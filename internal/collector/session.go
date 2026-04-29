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
		SELECT i.inst_id, c.name, st.status, 'user', COUNT(s.sid)
		FROM (SELECT DISTINCT inst_id FROM gv$instance) i
		CROSS JOIN (
			SELECT con_id, name FROM v$containers
			WHERE open_mode != 'MOUNTED'
			  AND con_id > 0
			  AND name != 'PDB$SEED'
		) c
		CROSS JOIN (
			SELECT 'active'   AS status FROM dual
			UNION ALL
			SELECT 'inactive' FROM dual
		) st
		LEFT JOIN gv$session s
			ON  s.inst_id       = i.inst_id
			AND s.con_id        = c.con_id
			AND LOWER(s.status) = st.status
			AND LOWER(s.type)   = 'user'
		GROUP BY i.inst_id, c.name, st.status
		UNION ALL
		SELECT inst_id, '', LOWER(status), LOWER(type), COUNT(*)
		FROM gv$session
		WHERE LOWER(type) != 'user'
		GROUP BY inst_id, status, type
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
