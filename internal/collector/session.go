package collector

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var sessionCountDesc = prometheus.NewDesc(
	"oracle_session_count",
	"Number of Oracle sessions grouped by instance, PDB, status and type",
	[]string{"inst_id", "pdb", "status", "type"}, nil,
)

type SessionCollector struct {
	isCDB bool
}

func (c *SessionCollector) Name() string { return "session" }

func (c *SessionCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if c.isCDB {
		return c.updateCDB(ctx, db, ch)
	}
	return c.updateNonCDB(ctx, db, ch)
}

func (c *SessionCollector) updateCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.inst_id, NVL(con.name, ''), LOWER(s.status), LOWER(s.type), COUNT(*)
		FROM gv$session s
		LEFT JOIN v$containers con ON s.con_id = con.con_id
		GROUP BY s.inst_id, con.name, s.status, s.type
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var instID int
		var pdb, status, sessType string
		var count float64
		if err := rows.Scan(&instID, &pdb, &status, &sessType, &count); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(sessionCountDesc, prometheus.GaugeValue, count,
			strconv.Itoa(instID), pdb, status, sessType)
	}
	return rows.Err()
}

func (c *SessionCollector) updateNonCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
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
			strconv.Itoa(instID), "", status, sessType)
	}
	return rows.Err()
}
