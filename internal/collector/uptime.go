package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var uptimeSecondsDesc = prometheus.NewDesc(
	"oracle_instance_uptime_seconds",
	"Oracle instance uptime in seconds since last startup",
	[]string{"inst_id", "instance_name", "version", "status"}, nil,
)

type UptimeCollector struct{}

func (c *UptimeCollector) Name() string { return "uptime" }

func (c *UptimeCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			inst_id,
			instance_name,
			version,
			LOWER(status),
			ROUND((SYSDATE - startup_time) * 86400)
		FROM gv$instance
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var name, version, status string
		var uptime float64
		if err := rows.Scan(&instID, &name, &version, &status, &uptime); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(uptimeSecondsDesc, prometheus.GaugeValue, uptime,
			strconv.Itoa(instID), name, version, status)
	}
	return rows.Err()
}
