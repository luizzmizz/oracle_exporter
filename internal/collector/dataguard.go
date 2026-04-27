package collector

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

var dataguardLagSecondsDesc = prometheus.NewDesc(
	"oracle_dataguard_lag_seconds",
	"Oracle Data Guard lag in seconds (transport lag and apply lag)",
	[]string{"name"}, nil,
)

type DataguardCollector struct{}

func (c *DataguardCollector) Name() string { return "dataguard" }

func (c *DataguardCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT name, value
		FROM v$dataguard_stats
		WHERE name IN ('transport lag', 'apply lag')
		AND value IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(dataguardLagSecondsDesc, prometheus.GaugeValue, parseLagInterval(value), name)
	}
	return rows.Err()
}

// parseLagInterval parses Oracle Data Guard interval strings.
// Handles "+HH:MM:SS" and "+DD HH:MM:SS" formats.
func parseLagInterval(s string) float64 {
	var days, hours, minutes, seconds int
	if n, _ := fmt.Sscanf(s, "+%d %d:%d:%d", &days, &hours, &minutes, &seconds); n == 4 {
		return float64(days*86400 + hours*3600 + minutes*60 + seconds)
	}
	if n, _ := fmt.Sscanf(s, "+%d:%d:%d", &hours, &minutes, &seconds); n == 3 {
		return float64(hours*3600 + minutes*60 + seconds)
	}
	return 0
}
