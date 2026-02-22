# File Parser Architecture

## Overview

The File Parser executor parses structured files — CSV, TSV, fixed-width (CCLF, NACHA, X12),
Excel (.xlsx/.xls), Apache Avro, and Apache Parquet — into an array of `map[string]interface{}`
records for downstream pipeline steps.

It uses the **Strategy Pattern**: a `FormatParser` interface with one implementation per format.
Parsers self-register via `init()` — adding a new format requires no changes to the orchestrator.

```
FileParserExecutor (orchestrator)
    ├── resolves source content (field / local_path / field_as_path)
    ├── applies file-size gate (stat before read)
    ├── auto-detects format (magic bytes + heuristics)
    └── delegates parsing → FormatParserRegistry → ConcreteParser
```

---

## Source Types

| `sourceType` | Description |
|---|---|
| `field` (default) | Raw file content is already in a pipeline field (e.g., set by an Inbound Connector step) |
| `local_path` | Read directly from the server/container filesystem. Supports batch mode via glob pattern. |
| `field_as_path` | A pipeline field holds a **URI** — the executor resolves it remotely. |

### `field_as_path` URI Dispatch

When `sourceType = "field_as_path"`, the executor reads the URI from the named field and
dispatches to a protocol handler:

| URI Prefix | Handler | Notes |
|---|---|---|
| `s3://bucket/key` | AWS S3 SDK | Credentials from `interface_connectivity` row (AES-256-GCM decrypted) |
| `https://…` | HTTP GET | Plain HTTP GET, no auth |
| `file:///…` | Local filesystem | Same size gate as `local_path` |

#### S3 Credential Resolution Flow

```
step.config["interface_id"]
    ↓
SELECT source_config FROM interface_connectivity WHERE interface_id = ?
    ↓ (raw JSONB bytes)
configDecrypt(raw)                      ← AES-256-GCM via CredentialStore.DecryptConfigBytes
    ↓ (plaintext JSON)
json.Unmarshal → S3Config{Region, Bucket, AccessKeyID, SecretAccessKey}
    ↓
aws.NewSession → s3.GetObject(key)
    ↓ (file bytes)
return rawContent
```

`configDecrypt` is passed into `FileParserExecutor` from `ExecutorRegistry`, which gets it from
`CredentialStore.DecryptConfigBytes`. If `APP_CREDENTIAL_KEY` is unset, passthrough mode is
used (dev/test).

---

## Format Support Matrix

| Format | `fileFormat` value | Extensions | Detection |
|---|---|---|---|
| CSV | `csv` | .csv | delimiter heuristics |
| TSV | `tsv` | .tsv, .tab | delimiter heuristics |
| Fixed-Width | `fixed_width` | .dat, .txt, .fix, .pos | OOB template or column def |
| Excel (modern) | `xlsx` | .xlsx | magic bytes `PK\x03\x04` |
| Excel (legacy) | `xls` | .xls | magic bytes `\xD0\xCF\x11\xE0` |
| Apache Avro | `avro` | .avro | magic bytes `Obj\x01` |
| Apache Parquet | `parquet` | .parquet | magic bytes `PAR1` |

### Auto-Detection

Set `autoDetect: true` or `fileFormat: "auto"`. Detection priority:

1. **Binary magic bytes** — checked first; xlsx, xls, avro, parquet detected from first 4 bytes.
2. **Extension** — used for `local_path` when no magic bytes match (infers csv/tsv/etc.).
3. **Content heuristics** — counts common delimiters across first few lines to decide csv vs tsv.

Auto-detect also infers `hasHeader` from column name patterns (non-numeric first row → header).

---

## Format Parser Internals

### CSV / TSV Parser (streaming)

```go
// When MaxRecords > 0: O(MaxRecords) memory — row-by-row via csv.Reader.Read()
// When MaxRecords == 0: csv.ReadAll() — no streaming needed, already bounded by size gate
```

The streaming path stops at `MaxRecords` without reading further — essential for multi-GB
files where you only need to preview or sample records.

### Fixed-Width Parser

Parses each line by byte position using `ColumnDef{Name, Start, Length}`. Supports:
- Manual column definition via `columns` array in config
- OOB template selection (see below)
- Leading/trailing whitespace trimming per field

### Excel (xlsx / xls)

Uses `github.com/xuri/excelize/v2` (xlsx) and `github.com/extrame/xls` (xls). Sheet selection
by name (`sheetName`) or 0-based index (`sheetIndex`). First row treated as header when
`hasHeader: true`.

### Apache Avro

Uses `github.com/linkedin/goavro/v2`. Reads an OCF (Object Container File) stream via
`goavro.NewOCFReader`. Avro union types (`{"typeName": value}`) are automatically flattened
to their underlying value. Column names come from the embedded Avro schema — `hasHeader` is
ignored.

### Apache Parquet

Uses `github.com/parquet-go/parquet-go`. Reads via `parquet.OpenFile` on the in-memory byte
slice. Column names come from `file.Schema().Fields()` — `hasHeader` is ignored. Row values
are converted from `parquet.Value` to Go primitives.

---

## File Size Gate

Applies to `local_path` and `field_as_path` with `file://` URIs.

```go
const (
    defaultMaxFileSizeMB = 100  // used when MaxFileSizeMB == 0
    hardCapFileSizeMB    = 500  // enforced regardless of config
)
```

`os.Stat()` is called **before** `os.ReadFile()`. Files exceeding the limit are rejected with a
descriptive error that includes the actual size, the limit, and a suggestion to use `maxRecords`
for sampling.

Configure via `maxFileSizeMB` in step config (0 = use default 100 MB).

---

## OOB Healthcare Templates

Pre-built column definitions for common fixed-width healthcare and financial formats.
Users select a template; columns are auto-populated. No manual column mapping needed.

| Template Key | Name | Source |
|---|---|---|
| `cclf1` | CCLF1 — Part A Claims Header | CMS CCLF Information Packet |
| `cclf2` | CCLF2 — Part A Claims Revenue | CMS CCLF Information Packet |
| `cclf3` | CCLF3 — Part A PPS / SNF | CMS CCLF Information Packet |
| `cclf4` | CCLF4 — Part B Physicians | CMS CCLF Information Packet |
| `cclf5` | CCLF5 — Part B DME | CMS CCLF Information Packet |
| `cclf6` | CCLF6 — Part D Drug Events | CMS CCLF Information Packet |
| `cclf7` | CCLF7 — Beneficiary Demographics | CMS CCLF Information Packet |
| `cclf8` | CCLF8 — Beneficiary XREF | CMS CCLF Information Packet |
| `nacha_entry` | NACHA ACH Entry Detail Record | NACHA Operating Rules |
| `era_835_header` | ERA 835 Interchange Control Header | ASC X12 TR3 |

Templates are stored in `services/executors/enrichment/file_parser_templates.go` and served
via the `/api/pipeline/file-parser/templates` endpoint.

---

## Batch Mode

When `sourceType = "local_path"` and `batchMode: true`, the executor processes **all files**
matching a glob pattern and returns a combined array of results. Each result includes the
filename and per-file record count. Useful for processing an entire directory of daily feed files.

```json
{
    "filePath": "/data/cclf/",
    "filePattern": "PARTA*.T.*.D2501*.T*",
    "sourceType": "local_path",
    "fileFormat": "fixed_width",
    "template": "cclf1",
    "batchMode": true
}
```

---

## Content Encoding

Set `contentEncoding: "base64"` when binary file content (Excel, Avro, Parquet) has been
base64-encoded before storing in a pipeline field (common when binary data passes through
JSON-based APIs or message queues).

The executor decodes before passing to the format parser.

---

## Output Structure

After execution, the step output is written to `enriched.file_parser` (or the step's alias):

```json
{
    "record_count": 1250,
    "column_count": 14,
    "columns": ["claim_id", "bene_id", "from_dt", ...],
    "records": [
        { "claim_id": "A001", "bene_id": "123456789A", "from_dt": "20250101" },
        ...
    ]
}
```

Metadata stored in step details:
- `format` — detected or configured format
- `parse_time_ms` — total parse duration
- `skipped_rows` — rows skipped from top
- `has_header` — whether header row was consumed
- `source_type` — field / local_path / field_as_path
- `auto_detected` — true if format was auto-detected
- `template` — OOB template key if used

---

## Configuration Reference

| Field | Type | Default | Description |
|---|---|---|---|
| `sourceType` | enum | `field` | `field`, `local_path`, `field_as_path` |
| `sourceField` | string | — | Pipeline field with content (sourceType=field or field_as_path) |
| `filePath` | string | — | Local filesystem path or glob (sourceType=local_path) |
| `filePattern` | string | — | Glob filename pattern within filePath directory (batch mode) |
| `fileFormat` | enum | `auto` | `auto`, `csv`, `tsv`, `fixed_width`, `xlsx`, `xls`, `avro`, `parquet` |
| `autoDetect` | bool | false | Enable magic-byte + heuristic format detection |
| `delimiter` | string | `,` | Field delimiter for CSV/TSV |
| `hasHeader` | bool | true | Whether first row is a header (text formats) |
| `template` | string | — | OOB template key for fixed-width parsing |
| `columns` | array | — | Manual column defs: `[{name, start, length}]` |
| `sheetName` | string | — | Excel sheet name (first sheet if empty) |
| `sheetIndex` | number | 0 | Excel 0-based sheet index (fallback if sheetName empty) |
| `contentEncoding` | enum | — | `base64` — decode before parsing |
| `trimFields` | bool | true | Trim whitespace from string values |
| `skipRows` | number | 0 | Rows/records to skip from top |
| `maxRecords` | number | 0 | Max records to parse (0 = unlimited) |
| `maxFileSizeMB` | number | 0 | File size limit (0 = 100 MB default, hard cap 500 MB) |
| `batchMode` | bool | false | Process all matching files and combine results |
| `interface_id` | string | — | Interface ID for S3 credential lookup (field_as_path + s3://) |

---

## Credential Encryption

Field-level credential encryption applies to pipeline step configs. If a step config contains
a sensitive key (e.g., a manually entered API key for a custom S3 config), it is encrypted via
`pipelineController.js:encryptSensitiveConfigFields()` before being saved to the database.

The Go executor's `configDecrypt` function decrypts connectivity credentials from the JSONB
column before using them. Both use AES-256-GCM with the `APP_CREDENTIAL_KEY` environment
variable (32-byte base64-encoded key).

See: `services/credential_store.go`

---

## File Paths

| Component | Path |
|---|---|
| Executor (orchestrator) | `services/executors/enrichment/file_parser_executor.go` |
| Format interface + registry | `services/executors/enrichment/format_parsers.go` |
| CSV + TSV parser | `services/executors/enrichment/csv_parser.go` |
| Fixed-width parser | `services/executors/enrichment/fixed_width_parser.go` |
| Excel parser (xlsx + xls) | `services/executors/enrichment/excel_parser.go` |
| Avro parser | `services/executors/enrichment/avro_parser.go` |
| Parquet parser | `services/executors/enrichment/parquet_parser.go` |
| OOB templates | `services/executors/enrichment/file_parser_templates.go` |
| Remote (S3 / HTTP / file://) | `services/executors/enrichment/file_parser_remote.go` |
| Go unit tests | `services/executors/enrichment/file_parser_executor_test.go` |
| API/integration tests | `services/executors/enrichment/file_parser_api_test.go` |
| Remote tests | `services/executors/enrichment/file_parser_remote_test.go` |
