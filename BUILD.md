# Building oracle_exporter

## Prerequisites

- Go 1.22+
- gcc (required by godror for CGO/ODPI-C)
- Docker (for cross-OS builds)

## Build

The target platform is OEL/RHEL 9. The exporter uses CGO (via godror/ODPI-C) so
the binary is dynamically linked against glibc — building directly on a different
OS produces a binary that won't run on OEL/RHEL 9.

Always build via Docker:

```bash
make
```

This compiles inside an OEL 9 container (glibc 2.34) and extracts the binary as
`oracle_exporter_oel9`, ready to copy to the target server.

## Runtime requirements on the target server

The exporter uses Oracle's ODPI-C to connect to Oracle databases, which requires
Oracle Instant Client at runtime:

```bash
export ORACLE_HOME=/opt/instantclient
export LD_LIBRARY_PATH=/opt/instantclient
./oracle_exporter --config config.yaml
```

## Configuration

Copy `config.yaml.example` to `config.yaml` and edit:

```bash
cp config.yaml.example config.yaml
```

Key fields per target:

| Field | Required | Description |
|---|---|---|
| `connect_string` | yes* | Full Oracle connect string |
| `host` + `service` | yes* | Alternative to connect_string |
| `port` | no | Default: 1521 |
| `username` | yes | Oracle username |
| `password` | yes** | Oracle password |
| `wallet_location` | no | Path to Oracle wallet directory |

\* Either `connect_string` or `host`+`service` required.  
\*\* Required unless `wallet_location` is set.

## Prometheus scrape config

```yaml
scrape_configs:
  - job_name: oracle
    static_configs:
      - targets: [prod-cdb, prod-asm, legacy-db]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: localhost:9161
```

## Endpoints

| Endpoint | Description |
|---|---|
| `/metrics?target=<name>` | Prometheus metrics for a named target |
| `/targets` | List configured targets and their detected type (CDB/ASM/non-CDB) |
| `/` | Landing page |

## Target auto-detection

On startup the exporter connects to each target and detects its type:

- Instance name starts with `+ASM` → ASM target, only runs `asm_diskgroup` collector
- `v$database.cdb = YES` → CDB target, uses `CDB_*` views and `gv$` views with `pdb` and `inst_id` labels
- Otherwise → non-CDB target, uses `DBA_*` views and `gv$` views with `inst_id` label, `pdb` label is empty

On RAC, `gv$` views automatically expose per-instance data with `inst_id` as a label.
No extra configuration needed for RAC — one connection per CDB covers all nodes.
