package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sysTimeModelDesc = prometheus.NewDesc(
	"oracle_time_model_microseconds_total",
	"Cumulative time model statistic in microseconds from gv$sys_time_model / gv$con_sys_time_model (use rate() in queries)",
	[]string{"inst_id", "pdb", "name"}, nil,
)

type SysTimeModelCollector struct {
	isCDB  bool
	filter *Filter
}

func (c *SysTimeModelCollector) Name() string { return "systimemodel" }

func (c *SysTimeModelCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if c.isCDB {
		return c.updateCDB(ctx, db, ch)
	}
	return c.updateNonCDB(ctx, db, ch)
}

func (c *SysTimeModelCollector) updateCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.inst_id, NVL(con.name, ''), s.stat_name, s.value
		FROM gv$con_sys_time_model s
		LEFT JOIN v$containers con ON s.con_id = con.con_id
		WHERE con.open_mode != 'MOUNTED' OR con.open_mode IS NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var pdb, name string
		var value float64
		if err := rows.Scan(&instID, &pdb, &name, &value); err != nil {
			return err
		}
		if !c.filter.Allow(name) {
			continue
		}
		ch <- prometheus.MustNewConstMetric(sysTimeModelDesc, prometheus.CounterValue, value,
			strconv.Itoa(instID), pdb, name)
	}
	return rows.Err()
}

func (c *SysTimeModelCollector) updateNonCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
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
			strconv.Itoa(instID), "", name)
	}
	return rows.Err()
}
