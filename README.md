# oracle_exporter

Prometheus exporter for Oracle Database. One binary, one config file, multiple targets.

Built with [godror](https://github.com/godror/godror) (ODPI-C) and the [Prometheus Go client](https://github.com/prometheus/client_golang).

---

## Features

- **Multi-target** — single process serves any number of Oracle instances via `/metrics?target=<name>`
- **Auto-detection** — connects at startup and identifies each target as CDB, non-CDB, or ASM; adjusts queries and labels automatically
- **RAC-aware** — uses `gv$` views; every instance appears as a separate `inst_id` label, no extra config needed
- **CDB/PDB-aware** — uses `CDB_*` views for multitenant; every PDB appears as a `pdb` label on tablespace metrics
- **ASM mirror-aware** — capacity metrics account for redundancy factor (EXTERN/NORMAL/HIGH) and `required_mirror_free_mb`
- **Per-target overrides** — any collector can be enabled/disabled or filtered per target
- **Include/exclude filters** — regex-based allowlist and denylist on high-cardinality collectors (sysstat, syswaitclass, systimemodel, event)
- **Wallet auth** — pass `wallet_location` instead of a password
- **Privilege support** — `sysdba`, `sysoper`, `sysasm` for ASM and admin connections

---

## Quick start

```bash
cp config.yaml.example config.yaml
# edit config.yaml with your targets

make                          # builds /tmp/oracle_exporter_oel9
scp /tmp/oracle_exporter_oel9 oracle-server:/opt/oracle_exporter/

# on the target server:
export ORACLE_HOME=/opt/instantclient
export LD_LIBRARY_PATH=/opt/instantclient
./oracle_exporter_oel9 --config config.yaml
```

---

## Build

The binary is dynamically linked against glibc. Always build via Docker to target OEL/RHEL 9 (glibc 2.34):

```bash
make
```

Output: `/tmp/oracle_exporter_oel9`

Prerequisites: Docker (the container handles Go, gcc, and ODPI-C).

```bash
make clean    # remove the Docker build image
```

### Runtime requirements

Oracle Instant Client must be present on the server where the exporter runs:

```bash
export ORACLE_HOME=/opt/instantclient
export LD_LIBRARY_PATH=/opt/instantclient
./oracle_exporter_oel9 --config config.yaml --web.listen-address :9161
```

---

## Configuration

```bash
cp config.yaml.example config.yaml
```

See `config.yaml.example` for a fully annotated reference. Key structure:

```yaml
scrape_timeout: 30s

collectors:          # global defaults
  tablespace:
    enabled: true
  sysstat:
    enabled: true
    include: [...]   # regex allowlist
    exclude: [...]   # regex denylist (applied after include)
  dataguard:
    enabled: false   # off globally, enable per-target

targets:
  prod-cdb:
    connect_string: "scan-vip:1521/PRODCDB"
    username: C##DBSNMP
    password: secret

  prod-asm:
    connect_string: "node1:1521/+ASM1"
    username: ASMSNMP
    password: secret
    privilege: sysdba       # asmsnmp requires sysdba, not sysasm

  legacy-db:
    host: 10.0.0.5
    service: LEGACYDB
    username: DBSNMP
    wallet_location: /opt/oracle/wallet/legacy

  prod-cdb-dg:
    connect_string: "scan-vip:1521/PRODCDB"
    username: C##DBSNMP
    password: secret
    collectors:             # per-target override
      dataguard:
        enabled: true
      event:
        exclude: ["^SQL\\*Net"]
```

### Connection fields

| Field | Required | Description |
|---|---|---|
| `connect_string` | yes* | Full Oracle connection string |
| `host` + `service` | yes* | Alternative; `port` defaults to 1521 |
| `username` | yes | Oracle account |
| `password` | yes** | Cleartext password |
| `wallet_location` | no | Path to wallet directory; replaces password |
| `privilege` | no | `sysdba`, `sysoper`, or `sysasm` |

\* Either `connect_string` or `host`+`service` is required.  
\*\* Required unless `wallet_location` is set.

---

## Auto-detection

On startup the exporter connects to each target and queries:

| Condition | Target type | Behaviour |
|---|---|---|
| Instance name starts with `+ASM` | ASM | Only `asm_diskgroup` collector runs |
| `v$database.cdb = YES` | CDB | Uses `CDB_*` views; `pdb` + `inst_id` labels |
| Otherwise | non-CDB | Uses `DBA_*` views; `inst_id` label, `pdb` is empty |

---

## Collectors

### `tablespace`

Space usage per tablespace. One bulk query covers all tablespaces.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_tablespace_used_bytes` | Gauge | `pdb`, `tablespace`, `contents`, `status` | Used bytes |
| `oracle_tablespace_total_bytes` | Gauge | `pdb`, `tablespace`, `contents`, `status` | Total (incl. autoextend) |
| `oracle_tablespace_free_bytes` | Gauge | `pdb`, `tablespace`, `contents`, `status` | Free bytes |

CDB: queries `CDB_TABLESPACE_USAGE_METRICS` + `CDB_TABLESPACES` + `v$containers` (skips MOUNTED PDBs).  
non-CDB: queries `dba_tablespace_usage_metrics` + `dba_tablespaces`.

---

### `asm_diskgroup`

Mirror-aware capacity metrics from `V$ASM_DISKGROUP`.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_asm_diskgroup_used_bytes` | Gauge | `diskgroup`, `type`, `state` | Used bytes (effective capacity) |
| `oracle_asm_diskgroup_total_bytes` | Gauge | `diskgroup`, `type`, `state` | Effective total (raw minus mirror overhead) |
| `oracle_asm_diskgroup_free_bytes` | Gauge | `diskgroup`, `type`, `state` | Usable free bytes (`usable_file_mb`) |
| `oracle_asm_diskgroup_offline_disks` | Gauge | `diskgroup`, `type`, `state` | Offline disk count |

Redundancy factors applied to `(total_mb - required_mirror_free_mb)`: EXTERN÷1, NORMAL÷2, HIGH÷3.

---

### `session`

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_session_count` | Gauge | `inst_id`, `status`, `type` | Sessions grouped by instance, status and type |

Source: `gv$session`.

---

### `sysstat`

Cumulative statistics from `gv$sysstat`. Supports `include`/`exclude` filters.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_sysstat_value_total` | Counter | `inst_id`, `name` | Cumulative statistic value |

Use `rate(oracle_sysstat_value_total[5m])` in Prometheus. Counter resets on instance restart are handled automatically.

---

### `syswaitclass`

Wait class aggregates from `gv$system_wait_class`. Supports `include`/`exclude` filters.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_wait_class_waits_total` | Counter | `inst_id`, `wait_class` | Total wait count |
| `oracle_wait_class_time_waited_centiseconds_total` | Counter | `inst_id`, `wait_class` | Total wait time in centiseconds |

---

### `systimemodel`

Time model statistics from `gv$sys_time_model`. Supports `include`/`exclude` filters.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_time_model_microseconds_total` | Counter | `inst_id`, `name` | Cumulative time in microseconds |

Key entries: `DB time`, `DB CPU`, `sql execute elapsed time`, `parse time elapsed`.

---

### `event`

Wait event statistics from `gv$system_event`. Supports `include`/`exclude` filters.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_event_waits_total` | Counter | `inst_id`, `event`, `wait_class` | Total wait count |
| `oracle_event_time_waited_centiseconds_total` | Counter | `inst_id`, `event`, `wait_class` | Total wait time in centiseconds |
| `oracle_event_timeouts_total` | Counter | `inst_id`, `event`, `wait_class` | Total timeout count |

---

### `flash_recovery_area`

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_fra_limit_bytes` | Gauge | — | Configured FRA size limit |
| `oracle_fra_used_bytes` | Gauge | — | Space used |
| `oracle_fra_reclaimable_bytes` | Gauge | — | Space reclaimable |

Source: `v$recovery_file_dest`.

---

### `uptime`

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_instance_uptime_seconds` | Gauge | `inst_id`, `instance_name`, `version`, `status` | Seconds since last startup |

Source: `gv$instance`. On RAC, one row per node.

---

### `dataguard`

Disabled by default. Enable on primary/standby targets.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_dataguard_lag_seconds` | Gauge | `name` | Transport lag and apply lag in seconds |

Source: `v$dataguard_stats`. Parses `+HH:MM:SS` and `+DD HH:MM:SS` formats.

---

### `sanity`

Detects datafiles whose current size has grown past their autoextend `maxsize`. Fires only when the condition exists — silence is good.

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oracle_datafile_over_maxsize` | Gauge | `pdb`, `tablespace`, `file_name` | 1 when datafile bytes > maxbytes (and autoextensible) |

CDB: `CDB_DATA_FILES`. non-CDB: `dba_data_files`.

---

### Scrape self-metrics

Emitted automatically for every collector, every scrape:

| Metric | Labels | Description |
|---|---|---|
| `oracle_scrape_collector_success` | `collector` | 1 = ok, 0 = error |
| `oracle_scrape_collector_duration_seconds` | `collector` | Query duration |

---

## HTTP endpoints

| Endpoint | Description |
|---|---|
| `/metrics?target=<name>` | Prometheus metrics for a named target |
| `/targets` | Tab-separated list of configured targets with db name, CDB flag, ASM flag |
| `/` | Landing page with links |

---

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

---

## RAC and CDB topology

A 2-node RAC CDB with 3 PDBs needs **one target entry**:

- `gv$` views expose per-instance rows → `inst_id` label distinguishes nodes
- `CDB_*` views expose per-PDB rows → `pdb` label distinguishes PDBs
- No extra config, no per-node connections

For an ASM cluster, add one target per ASM instance (or use the SCAN if ASM is accessible through it). ASM targets are auto-detected and run only the `asm_diskgroup` collector — add `privilege: sysdba` since `asmsnmp` holds SYSDBA, not SYSASM.
