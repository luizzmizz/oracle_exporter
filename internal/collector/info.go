package collector

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	instanceInfoDesc = prometheus.NewDesc(
		"oracle_instance_info",
		"Oracle instance metadata; value is uptime in seconds",
		[]string{"inst_id", "instance_name", "version", "status", "is_cdb"}, nil,
	)
	pdbInfoDesc = prometheus.NewDesc(
		"oracle_pdb_info",
		"Oracle PDB state (1 = present)",
		[]string{"pdb", "open_mode", "restricted"}, nil,
	)
	sqlpatchInfoDesc = prometheus.NewDesc(
		"oracle_sqlpatch_info",
		"Oracle SQL patch history (1 = applied)",
		[]string{"patch_id", "description", "status", "action", "action_time"}, nil,
	)
)

type InfoCollector struct {
	isCDB bool
}

func NewInfoCollector(isCDB bool) *InfoCollector {
	return &InfoCollector{isCDB: isCDB}
}

func (c *InfoCollector) Name() string { return "info" }

func (c *InfoCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if err := c.collectInstances(ctx, db, ch); err != nil {
		return err
	}
	if c.isCDB {
		if err := c.collectPDBs(ctx, db, ch); err != nil {
			slog.Warn("pdb info collection failed", "err", err)
		}
	}
	if err := c.collectSQLPatch(ctx, db, ch); err != nil {
		slog.Warn("sqlpatch info collection failed", "err", err)
	}
	return nil
}

func (c *InfoCollector) collectInstances(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	isCDB := "false"
	if c.isCDB {
		isCDB = "true"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT inst_id, instance_name, version, LOWER(status),
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
		ch <- prometheus.MustNewConstMetric(instanceInfoDesc, prometheus.GaugeValue, uptime,
			strconv.Itoa(instID), name, version, status, isCDB)
	}
	return rows.Err()
}

func (c *InfoCollector) collectPDBs(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT name, open_mode, NVL(restricted, 'NO')
		FROM v$pdbs
		WHERE name != 'PDB$SEED'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, openMode, restricted string
		if err := rows.Scan(&name, &openMode, &restricted); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(pdbInfoDesc, prometheus.GaugeValue, 1,
			name, openMode, restricted)
	}
	return rows.Err()
}

func (c *InfoCollector) collectSQLPatch(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT TO_CHAR(patch_id),
		       SUBSTR(description, 1, 200),
		       LOWER(status),
		       LOWER(action),
		       TO_CHAR(action_time, 'YYYY-MM-DD HH24:MI:SS')
		FROM dba_registry_sqlpatch
		ORDER BY action_time
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var patchID, description, status, action, actionTime string
		if err := rows.Scan(&patchID, &description, &status, &action, &actionTime); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(sqlpatchInfoDesc, prometheus.GaugeValue, 1,
			patchID, description, status, action, actionTime)
	}
	return rows.Err()
}
