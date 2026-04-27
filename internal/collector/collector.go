package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/luizzmizz/oracle_exporter/internal/config"
	"github.com/luizzmizz/oracle_exporter/internal/target"
)

type Collector interface {
	Name() string
	Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error
}

var (
	scrapeSuccessDesc = prometheus.NewDesc(
		"oracle_scrape_collector_success",
		"Whether the last scrape of this collector succeeded (1=ok, 0=error)",
		[]string{"collector"}, nil,
	)
	scrapeDurationDesc = prometheus.NewDesc(
		"oracle_scrape_collector_duration_seconds",
		"Duration of the last scrape of this collector in seconds",
		[]string{"collector"}, nil,
	)
)

type OracleCollector struct {
	db         *sql.DB
	collectors []Collector
	timeout    time.Duration
	logger     *slog.Logger
}

func NewOracleCollector(db *sql.DB, collectors []Collector, timeout time.Duration, logger *slog.Logger) *OracleCollector {
	return &OracleCollector{db: db, collectors: collectors, timeout: timeout, logger: logger}
}

func (o *OracleCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeSuccessDesc
	ch <- scrapeDurationDesc
}

func (o *OracleCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()

	for _, c := range o.collectors {
		start := time.Now()
		err := c.Update(ctx, o.db, ch)
		dur := time.Since(start).Seconds()

		success := 1.0
		if err != nil {
			success = 0
			o.logger.Error("collector failed", "name", c.Name(), "err", err)
		}
		ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, c.Name())
		ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, dur, c.Name())
	}
}

// Build returns the set of collectors appropriate for the target's detected type.
// ASM targets only run the ASM diskgroup collector.
// DB targets (CDB or non-CDB) run all other enabled collectors.
func Build(cfg config.CollectorSet, meta target.Meta) ([]Collector, error) {
	if meta.IsASM {
		if cfg.ASMDiskgroup.IsEnabled() {
			return []Collector{&ASMDiskgroupCollector{}}, nil
		}
		return nil, nil
	}

	var collectors []Collector

	if cfg.Tablespace.IsEnabled() {
		collectors = append(collectors, &TablespaceCollector{isCDB: meta.IsCDB})
	}
	if cfg.ASMDiskgroup.IsEnabled() {
		collectors = append(collectors, &ASMDiskgroupCollector{})
	}
	if cfg.Session.IsEnabled() {
		collectors = append(collectors, &SessionCollector{})
	}
	if cfg.Sysstat.IsEnabled() {
		f, err := NewFilter(cfg.Sysstat.Include, cfg.Sysstat.Exclude)
		if err != nil {
			return nil, fmt.Errorf("sysstat filter: %w", err)
		}
		collectors = append(collectors, &SysstatCollector{filter: f})
	}
	if cfg.SysWaitClass.IsEnabled() {
		f, err := NewFilter(cfg.SysWaitClass.Include, cfg.SysWaitClass.Exclude)
		if err != nil {
			return nil, fmt.Errorf("syswaitclass filter: %w", err)
		}
		collectors = append(collectors, &SysWaitClassCollector{filter: f})
	}
	if cfg.SysTimeModel.IsEnabled() {
		f, err := NewFilter(cfg.SysTimeModel.Include, cfg.SysTimeModel.Exclude)
		if err != nil {
			return nil, fmt.Errorf("systimemodel filter: %w", err)
		}
		collectors = append(collectors, &SysTimeModelCollector{filter: f})
	}
	if cfg.Event.IsEnabled() {
		f, err := NewFilter(cfg.Event.Include, cfg.Event.Exclude)
		if err != nil {
			return nil, fmt.Errorf("event filter: %w", err)
		}
		collectors = append(collectors, &EventCollector{filter: f})
	}
	if cfg.FlashRecoveryArea.IsEnabled() {
		collectors = append(collectors, &FlashRecoveryAreaCollector{})
	}
	if cfg.Uptime.IsEnabled() {
		collectors = append(collectors, &UptimeCollector{})
	}
	if cfg.Dataguard.IsEnabled() {
		collectors = append(collectors, &DataguardCollector{})
	}
	if cfg.Sanity.IsEnabled() {
		collectors = append(collectors, &SanityCollector{isCDB: meta.IsCDB})
	}
	return collectors, nil
}
