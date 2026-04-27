package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sysstatValueDesc = prometheus.NewDesc(
	"oracle_sysstat_value_total",
	"Cumulative system statistic value from gv$sysstat / gv$con_sysstat (use rate() in queries)",
	[]string{"inst_id", "pdb", "name"}, nil,
)

type SysstatCollector struct {
	isCDB  bool
	filter *Filter
}

func (c *SysstatCollector) Name() string { return "sysstat" }

func (c *SysstatCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if c.isCDB {
		return c.updateCDB(ctx, db, ch)
	}
	return c.updateNonCDB(ctx, db, ch)
}

func (c *SysstatCollector) updateCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.inst_id, NVL(con.name, ''), s.name, s.value
		FROM gv$con_sysstat s
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
		ch <- prometheus.MustNewConstMetric(sysstatValueDesc, prometheus.CounterValue, value,
			strconv.Itoa(instID), pdb, name)
	}
	return rows.Err()
}

func (c *SysstatCollector) updateNonCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
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
			strconv.Itoa(instID), "", name)
	}
	return rows.Err()
}
