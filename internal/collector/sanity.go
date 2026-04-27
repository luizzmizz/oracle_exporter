package collector

import (
	"context"
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

var datafileOverMaxsizeDesc = prometheus.NewDesc(
	"oracle_datafile_over_maxsize",
	"Datafile whose current size exceeds its autoextend maxsize (1 = over maxsize)",
	[]string{"pdb", "tablespace", "file_name"}, nil,
)

type SanityCollector struct {
	isCDB bool
}

func (c *SanityCollector) Name() string { return "sanity" }

func (c *SanityCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	if c.isCDB {
		return c.updateCDB(ctx, db, ch)
	}
	return c.updateNonCDB(ctx, db, ch)
}

func (c *SanityCollector) updateCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT con.name, f.tablespace_name, f.file_name
		FROM CDB_DATA_FILES f
		JOIN v$containers con ON f.con_id = con.con_id
		WHERE f.autoextensible = 'YES'
		AND   f.bytes > f.maxbytes
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var pdb, tablespace, fileName string
		if err := rows.Scan(&pdb, &tablespace, &fileName); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(datafileOverMaxsizeDesc, prometheus.GaugeValue, 1, pdb, tablespace, fileName)
	}
	return rows.Err()
}

func (c *SanityCollector) updateNonCDB(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT tablespace_name, file_name
		FROM dba_data_files
		WHERE autoextensible = 'YES'
		AND   bytes > maxbytes
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tablespace, fileName string
		if err := rows.Scan(&tablespace, &fileName); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(datafileOverMaxsizeDesc, prometheus.GaugeValue, 1, "", tablespace, fileName)
	}
	return rows.Err()
}
