package collector

import (
	"context"
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	asmUsedBytesDesc = prometheus.NewDesc(
		"oracle_asm_diskgroup_used_bytes",
		"ASM diskgroup used space in bytes (mirror-aware: based on effective capacity)",
		[]string{"diskgroup", "type", "state"}, nil,
	)
	asmTotalBytesDesc = prometheus.NewDesc(
		"oracle_asm_diskgroup_total_bytes",
		"ASM diskgroup effective total capacity in bytes (raw total minus mirror overhead, divided by redundancy factor)",
		[]string{"diskgroup", "type", "state"}, nil,
	)
	asmFreeBytesDesc = prometheus.NewDesc(
		"oracle_asm_diskgroup_free_bytes",
		"ASM diskgroup usable free space in bytes (usable_file_mb from V$ASM_DISKGROUP)",
		[]string{"diskgroup", "type", "state"}, nil,
	)
	asmOfflineDisksDesc = prometheus.NewDesc(
		"oracle_asm_diskgroup_offline_disks",
		"Number of offline disks in ASM diskgroup",
		[]string{"diskgroup", "type", "state"}, nil,
	)
)

type ASMDiskgroupCollector struct{}

func (c *ASMDiskgroupCollector) Name() string { return "asm_diskgroup" }

func (c *ASMDiskgroupCollector) Update(ctx context.Context, db *sql.DB, ch chan<- prometheus.Metric) error {
	rows, err := db.QueryContext(ctx, `
		SELECT
			name,
			LOWER(state),
			LOWER(type),
			total_mb,
			usable_file_mb,
			required_mirror_free_mb,
			offline_disks
		FROM V$ASM_DISKGROUP
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, state, dgType string
		var totalMB, usableFileMB, requiredMirrorFreeMB, offlineDisks float64
		if err := rows.Scan(&name, &state, &dgType, &totalMB, &usableFileMB, &requiredMirrorFreeMB, &offlineDisks); err != nil {
			return err
		}

		mirrorFactor := mirrorFactorFor(dgType)
		effectiveTotal := (totalMB - requiredMirrorFreeMB) / mirrorFactor
		usedMB := effectiveTotal - usableFileMB

		const mbToBytes = 1024 * 1024
		labels := []string{name, dgType, state}
		ch <- prometheus.MustNewConstMetric(asmUsedBytesDesc, prometheus.GaugeValue, usedMB*mbToBytes, labels...)
		ch <- prometheus.MustNewConstMetric(asmTotalBytesDesc, prometheus.GaugeValue, effectiveTotal*mbToBytes, labels...)
		ch <- prometheus.MustNewConstMetric(asmFreeBytesDesc, prometheus.GaugeValue, usableFileMB*mbToBytes, labels...)
		ch <- prometheus.MustNewConstMetric(asmOfflineDisksDesc, prometheus.GaugeValue, offlineDisks, labels...)
	}
	return rows.Err()
}

func mirrorFactorFor(dgType string) float64 {
	switch dgType {
	case "high":
		return 3
	case "normal":
		return 2
	default:
		return 1
	}
}
