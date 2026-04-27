package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sessionCountDesc = prometheus.NewDesc(
	"oracle_session_count",
	"Number of Oracle sessions grouped by instance, status and type",
	[]string{"inst_id", "status", "type"}, nil,
)

type SessionCollector struct{}

func (c *SessionCollector) Name() string { return "session" }

func (c *SessionCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT inst_id, LOWER(status), LOWER(type), COUNT(*)
		FROM gv$session
		GROUP BY inst_id, status, type
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var status, sessType string
		var count float64
		if err := rows.Scan(&instID, &status, &sessType, &count); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(sessionCountDesc, prometheus.GaugeValue, count,
			strconv.Itoa(instID), status, sessType)
	}
	return rows.Err()
}
