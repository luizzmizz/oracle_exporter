package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sysTimeModelDesc = prometheus.NewDesc(
	"oracle_time_model_microseconds_total",
	"Cumulative time model statistic in microseconds from gv$sys_time_model (use rate() in queries)",
	[]string{"inst_id", "name"}, nil,
)

type SysTimeModelCollector struct{ filter *Filter }

func (c *SysTimeModelCollector) Name() string { return "systimemodel" }

func (c *SysTimeModelCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `SELECT inst_id, stat_name, value FROM gv$sys_time_model`)
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
		ch <- prometheus.MustNewConstMetric(sysTimeModelDesc, prometheus.CounterValue, value,
			strconv.Itoa(instID), name)
	}
	return rows.Err()
}
