package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	eventWaitsDesc = prometheus.NewDesc(
		"oracle_event_waits_total",
		"Total waits for non-idle system events from gv$system_event (use rate() in queries)",
		[]string{"inst_id", "event", "wait_class"}, nil,
	)
	eventTimeoutsDesc = prometheus.NewDesc(
		"oracle_event_timeouts_total",
		"Total timeouts for non-idle system events from gv$system_event (use rate() in queries)",
		[]string{"inst_id", "event", "wait_class"}, nil,
	)
	eventTimeWaitedDesc = prometheus.NewDesc(
		"oracle_event_time_waited_centiseconds_total",
		"Total time waited in centiseconds for non-idle system events from gv$system_event (use rate() in queries)",
		[]string{"inst_id", "event", "wait_class"}, nil,
	)
)

type EventCollector struct{ filter *Filter }

func (c *EventCollector) Name() string { return "event" }

func (c *EventCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT inst_id, event, wait_class, total_waits, total_timeouts, time_waited
		FROM gv$system_event
		WHERE wait_class != 'Idle'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var event, waitClass string
		var totalWaits, totalTimeouts, timeWaited float64
		if err := rows.Scan(&instID, &event, &waitClass, &totalWaits, &totalTimeouts, &timeWaited); err != nil {
			return err
		}
		if !c.filter.Allow(event) {
			continue
		}
		instStr := strconv.Itoa(instID)
		ch <- prometheus.MustNewConstMetric(eventWaitsDesc, prometheus.CounterValue, totalWaits, instStr, event, waitClass)
		ch <- prometheus.MustNewConstMetric(eventTimeoutsDesc, prometheus.CounterValue, totalTimeouts, instStr, event, waitClass)
		ch <- prometheus.MustNewConstMetric(eventTimeWaitedDesc, prometheus.CounterValue, timeWaited, instStr, event, waitClass)
	}
	return rows.Err()
}
