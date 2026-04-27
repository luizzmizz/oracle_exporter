package collector

import (
	"context"
	"database/sql"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	fraLimitBytesDesc = prometheus.NewDesc(
		"oracle_fra_limit_bytes",
		"Flash Recovery Area configured size limit in bytes",
		nil, nil,
	)
	fraUsedBytesDesc = prometheus.NewDesc(
		"oracle_fra_used_bytes",
		"Flash Recovery Area space used in bytes",
		nil, nil,
	)
	fraReclaimableBytesDesc = prometheus.NewDesc(
		"oracle_fra_reclaimable_bytes",
		"Flash Recovery Area space reclaimable in bytes",
		nil, nil,
	)
)

type FlashRecoveryAreaCollector struct{}

func (c *FlashRecoveryAreaCollector) Name() string { return "flash_recovery_area" }

func (c *FlashRecoveryAreaCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	row := db.QueryRowContext(ctx, `
		SELECT space_limit, space_used, space_reclaimable
		FROM v$recovery_file_dest
	`)
	var limit, used, reclaimable float64
	if err := row.Scan(&limit, &used, &reclaimable); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	ch <- prometheus.MustNewConstMetric(fraLimitBytesDesc, prometheus.GaugeValue, limit)
	ch <- prometheus.MustNewConstMetric(fraUsedBytesDesc, prometheus.GaugeValue, used)
	ch <- prometheus.MustNewConstMetric(fraReclaimableBytesDesc, prometheus.GaugeValue, reclaimable)
	return nil
}
