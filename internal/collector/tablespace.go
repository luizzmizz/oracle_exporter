package collector

import (
	"context"
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	tablespaceUsedBytesDesc = prometheus.NewDesc(
		"oracle_tablespace_used_bytes",
		"Tablespace used space in bytes",
		[]string{"pdb", "tablespace", "contents", "status"}, nil,
	)
	tablespaceTotalBytesDesc = prometheus.NewDesc(
		"oracle_tablespace_total_bytes",
		"Tablespace total size in bytes (including autoextend headroom)",
		[]string{"pdb", "tablespace", "contents", "status"}, nil,
	)
	tablespaceFreeBytesDesc = prometheus.NewDesc(
		"oracle_tablespace_free_bytes",
		"Tablespace free space in bytes",
		[]string{"pdb", "tablespace", "contents", "status"}, nil,
	)
	tablespaceAllocatedBytesDesc = prometheus.NewDesc(
		"oracle_tablespace_allocated_bytes",
		"Tablespace physically allocated bytes (sum of current datafile sizes)",
		[]string{"pdb", "tablespace", "contents", "status"}, nil,
	)
)

type TablespaceCollector struct {
	isCDB bool
}

func (c *TablespaceCollector) Name() string { return "tablespace" }

func (c *TablespaceCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if c.isCDB {
		return c.updateCDB(ctx, db, ch)
	}
	return c.updateNonCDB(ctx, db, ch)
}

// updateCDB queries CDB_* views and joins with v$containers for the PDB name.
// Skips PDBs in MOUNTED state (not open, data not accessible).
func (c *TablespaceCollector) updateCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			con.name,
			u.tablespace_name,
			t.contents,
			t.status,
			ROUND(u.used_space      * t.block_size),
			ROUND(u.tablespace_size * t.block_size),
			ROUND(NVL(f.allocated_bytes, 0))
		FROM CDB_TABLESPACE_USAGE_METRICS u
		JOIN CDB_TABLESPACES t
			ON  u.tablespace_name = t.tablespace_name
			AND u.con_id          = t.con_id
		JOIN v$containers con
			ON  u.con_id = con.con_id
		LEFT JOIN (
			SELECT con_id, tablespace_name, SUM(bytes) AS allocated_bytes
			FROM CDB_DATA_FILES
			GROUP BY con_id, tablespace_name
			UNION ALL
			SELECT con_id, tablespace_name, SUM(bytes) AS allocated_bytes
			FROM CDB_TEMP_FILES
			GROUP BY con_id, tablespace_name
		) f ON u.con_id = f.con_id AND u.tablespace_name = f.tablespace_name
		WHERE con.open_mode != 'MOUNTED'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pdb, name, contents, status string
		var usedBytes, totalBytes, allocatedBytes float64
		if err := rows.Scan(&pdb, &name, &contents, &status, &usedBytes, &totalBytes, &allocatedBytes); err != nil {
			return err
		}
		emitTablespace(ch, pdb, name, contents, status, usedBytes, totalBytes, allocatedBytes)
	}
	return rows.Err()
}

// updateNonCDB queries DBA_* views. pdb label is set to "" for non-CDB targets.
func (c *TablespaceCollector) updateNonCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			u.tablespace_name,
			t.contents,
			t.status,
			ROUND(u.used_space      * t.block_size),
			ROUND(u.tablespace_size * t.block_size),
			ROUND(NVL(f.allocated_bytes, 0))
		FROM dba_tablespace_usage_metrics u
		JOIN dba_tablespaces t ON u.tablespace_name = t.tablespace_name
		LEFT JOIN (
			SELECT tablespace_name, SUM(bytes) AS allocated_bytes
			FROM dba_data_files
			GROUP BY tablespace_name
			UNION ALL
			SELECT tablespace_name, SUM(bytes) AS allocated_bytes
			FROM dba_temp_files
			GROUP BY tablespace_name
		) f ON u.tablespace_name = f.tablespace_name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, contents, status string
		var usedBytes, totalBytes, allocatedBytes float64
		if err := rows.Scan(&name, &contents, &status, &usedBytes, &totalBytes, &allocatedBytes); err != nil {
			return err
		}
		emitTablespace(ch, "", name, contents, status, usedBytes, totalBytes, allocatedBytes)
	}
	return rows.Err()
}

func emitTablespace(ch chan<- prometheus.Metric, pdb, name, contents, status string, usedBytes, totalBytes, allocatedBytes float64) {
	labels := []string{pdb, name, contents, status}
	ch <- prometheus.MustNewConstMetric(tablespaceUsedBytesDesc, prometheus.GaugeValue, usedBytes, labels...)
	ch <- prometheus.MustNewConstMetric(tablespaceTotalBytesDesc, prometheus.GaugeValue, totalBytes, labels...)
	ch <- prometheus.MustNewConstMetric(tablespaceFreeBytesDesc, prometheus.GaugeValue, totalBytes-usedBytes, labels...)
	ch <- prometheus.MustNewConstMetric(tablespaceAllocatedBytesDesc, prometheus.GaugeValue, allocatedBytes, labels...)
}
