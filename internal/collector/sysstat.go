package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sysstatValueDesc = prometheus.NewDesc(
	"oracle_sysstat_value_total",
	"Cumulative system statistic value from gv$sysstat (use rate() in queries)",
	[]string{"inst_id", "name"}, nil,
)

type SysstatCollector struct{ filter *Filter }

func (c *SysstatCollector) Name() string { return "sysstat" }

func (c *SysstatCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `SELECT inst_id, name, value FROM gv$sysstat`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var name string
		var value float64
		if err := rows.Scan(&instID, &name, &value); err != nil {
			return err
		}
		if !c.filter.Allow(name) {
			continue
		}
		ch <- prometheus.MustNewConstMetric(sysstatValueDesc, prometheus.CounterValue, value,
			strconv.Itoa(instID), name)
	}
	return rows.Err()
}
