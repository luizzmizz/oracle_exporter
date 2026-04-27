package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	waitClassWaitsDesc = prometheus.NewDesc(
		"oracle_wait_class_waits_total",
		"Total number of waits by instance and wait class from gv$system_wait_class (use rate() in queries)",
		[]string{"inst_id", "wait_class"}, nil,
	)
	waitClassTimeWaitedDesc = prometheus.NewDesc(
		"oracle_wait_class_time_waited_centiseconds_total",
		"Total time waited in centiseconds by instance and wait class from gv$system_wait_class (use rate() in queries)",
		[]string{"inst_id", "wait_class"}, nil,
	)
)

type SysWaitClassCollector struct{ filter *Filter }

func (c *SysWaitClassCollector) Name() string { return "syswaitclass" }

func (c *SysWaitClassCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT inst_id, wait_class, total_waits, time_waited
		FROM gv$system_wait_class
		WHERE wait_class != 'Idle'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var waitClass string
		var totalWaits, timeWaited float64
		if err := rows.Scan(&instID, &waitClass, &totalWaits, &timeWaited); err != nil {
			return err
		}
		if !c.filter.Allow(waitClass) {
			continue
		}
		instStr := strconv.Itoa(instID)
		ch <- prometheus.MustNewConstMetric(waitClassWaitsDesc, prometheus.CounterValue, totalWaits, instStr, waitClass)
		ch <- prometheus.MustNewConstMetric(waitClassTimeWaitedDesc, prometheus.CounterValue, timeWaited, instStr, waitClass)
	}
	return rows.Err()
}
