# ezHealthKonnect Operations Runbook

**Audience**: On-call engineers, DevOps  
**Last updated**: 2026-05-24

---

## Health Check Endpoints

| Endpoint | Purpose | Expected Response |
|---|---|---|
| `GET /healthz` | Liveness — is the process alive? | `200 {"status":"alive"}` |
| `GET /readyz` | Readiness — DB + storage + engine up? | `200 {"status":"ready"}` |
| `GET /api/system/health` | Detailed status (proxy-authenticated) | `200` with subsystem detail |
| `GET /metrics` | Prometheus scrape endpoint | `200` text/plain |

A `503` on `/readyz` means traffic should **not** be routed to this instance.

---

## Top 5 Failure Modes

### 1. PostgreSQL unreachable

**Symptoms**: `/readyz` returns `503`, `checks.database.ok = false`. All message processing halts.

**Diagnosis**:
```bash
docker compose logs db | tail -50
docker compose exec db pg_isready -U ezhealth_user -d ezhealthkonnect
```

**Resolution**:
1. If container crashed: `docker compose up -d db`
2. If disk full: free space, then `docker compose restart db`
3. If connection pool exhausted: check `DB_MAX_OPEN_CONNECTIONS` in `.env`; default is 25

**Recovery**: Once DB is healthy, `/readyz` returns 200 automatically. No restart needed.

---

### 2. Object storage unreachable (MinIO / S3)

**Symptoms**: `/readyz` returns `503`, `checks.object_storage.ok = false`. Messages are received and ACK'd, but raw HL7 and FHIR bundles cannot be stored. Pipeline will queue messages until storage recovers.

**Diagnosis**:
```bash
# MinIO
docker compose exec minio mc ready local

# S3 — check AWS credentials and bucket policy
aws s3 ls s3://<OBJECT_STORAGE_BUCKET> --region <AWS_REGION>
```

**Resolution**:
1. MinIO: `docker compose restart minio`
2. S3: verify `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION` in `.env`
3. If bucket was deleted: re-create and restart the app (bucket is auto-created on startup)

---

### 3. DLQ depth growing (delivery failures)

**Symptoms**: Admin UI shows DLQ depth > 0 on one or more interfaces. `interface_alert_thresholds.max_dlq_depth` exceeded → alert email/webhook fired.

**Diagnosis**:
```bash
# Via API (requires admin auth through proxy)
curl -s http://localhost:3000/api/fhir/dlq/stats | jq .

# Via database
psql -c "SELECT interface_id, COUNT(*), MAX(attempt_count) FROM delivery_dlq WHERE status='pending' GROUP BY 1 ORDER BY 2 DESC"
```

**Resolution**:
1. Identify the failing connector type from `connector_type` column.
2. Fix the downstream system (FHIR receiver, file share, database).
3. Use Admin UI > DLQ to bulk-redrive, or:
```bash
curl -X POST http://localhost:3000/api/fhir/dlq/bulk-redrive \
  -H "Content-Type: application/json" \
  -d '{"ids":["<uuid>","<uuid>"],"mode":"from_failed_step"}'
```
4. Rows that exceed `max_attempts` are marked `abandoned`. These must be manually reviewed and either re-driven or acknowledged.

---

### 4. Transformation quality flag rate > 20%

**Symptoms**: Alert fires indicating `messages_flagged / messages_scored > 20%` for an interface. Flagged messages have a score below threshold — missing FHIR resources, empty required fields, or unhandled segments.

**Diagnosis**:
```bash
# Review flagged records
curl http://localhost:3000/api/fhir/quality/stats
curl http://localhost:3000/api/fhir/quality/flagged?message_type=ORU%5ER01
```

**Resolution**:
1. Review the HL7 input and FHIR output side-by-side in the Admin > Mapping Review UI.
2. If the OOB template is wrong: update `hl7_fhir_templates` via the template editor.
3. If the issue is vendor-specific (Epic, Cerner quirk): add an interface override via the Mapping Override UI.
4. Mark reviewed via: `POST /api/fhir/quality/flagged/:id/review` with a resolution note.
5. Add the HL7/FHIR pair as a regression test case in `tests/hl7-scale-test.js`.

---

### 5. Go API process crash / OOM

**Symptoms**: Docker restarts the `app` container (`restart: always` policy). Requests return `502` for a few seconds. The restart loop in `CMD` (`while true; do ./go-api; done`) brings it back automatically.

**Diagnosis**:
```bash
docker compose logs app --tail=100 | grep -E "panic:|FATAL|OOM|killed"
docker stats --no-stream app  # check memory usage
```

**Resolution**:
1. If OOM: increase container memory limit in `docker-compose.yml`.
2. If panic: capture the stack trace from logs and file a bug.
3. If crash loop: `docker compose exec app cat /tmp/go-api-crash.log` (if crash handler writes one).
4. Emergency: `docker compose restart app` — the DLQ will hold failed messages for redrive.

---

## Maintenance Procedures

### Applying Flyway migrations
```bash
docker compose run --rm flyway migrate
```

### Rolling back a migration (V137–V141)
```bash
# 1. Stop the app to prevent new writes
docker compose stop app

# 2. Apply the rollback script
psql -U ezhealth_user -d ezhealthkonnect -f database/rollbacks/U141__Interface_DLQ_Config.sql

# 3. Remove the Flyway history entry
psql -U ezhealth_user -d ezhealthkonnect \
  -c "DELETE FROM flyway_schema_history WHERE version = '141'"

# 4. Restart on the previous code version
docker compose up -d app
```
See `database/rollbacks/` for rollback scripts per migration.

### Manually purging old messages (if retention enforcement is lagging)
```bash
# Purge messages older than N days from all interface tables
psql -U ezhealth_user -d ezhealthkonnect << 'EOF'
DO $$
DECLARE t text;
BEGIN
  FOR t IN SELECT table_name FROM interface_table_metadata LOOP
    EXECUTE format('DELETE FROM %I WHERE received_at < NOW() - INTERVAL ''90 days''', t);
  END LOOP;
END $$;
EOF
```

### Restarting the Go API without restarting Node.js
```bash
# The go-api process runs in the same container as Node.js
# Kill go-api — the restart loop brings it back in ~1 second
docker compose exec app pkill go-api
```

---

## Alerting Configuration

Alert thresholds are configured per interface in the Admin UI:
- **Max flag rate %**: trigger if > X% of messages flagged (default 20%)
- **Max DLQ depth**: trigger if DLQ has > N pending rows (default 10)
- **Alert email**: SMTP address for notifications
- **Alert webhook URL**: POST target (Slack incoming webhook, PagerDuty, etc.)

SMTP credentials are set in Admin > Settings > Email.

---

## Escalation

| Severity | Response time | Action |
|---|---|---|
| DB down | < 15 min | Page on-call, attempt DB restart |
| DLQ depth > 100 | < 1 hour | Page on-call, investigate connector |
| Quality flag rate > 50% | < 4 hours | Notify integration team, freeze new onboarding |
| Go API crash loop | < 15 min | Page on-call, check logs, restart if needed |

---

## Customer Onboarding Checklist (per new interface)

- [ ] Send 10 sample HL7 messages through the interface
- [ ] Verify FHIR bundle output at `/api/fhir/transform` or via Admin > Message Viewer
- [ ] Run `node tests/hl7-scale-test.js --types "<message_type>" --level strict`
- [ ] Confirm quality score ≥ 90 for all test messages
- [ ] Confirm TLS configured on MLLP listener (`enable_tls: true` + cert files)
- [ ] Set `dlq_config` retention and retry parameters (Admin > Interface > Recovery Queue)
- [ ] Configure alert thresholds (Admin > Interface > Alerts)
- [ ] Confirm data retention period matches customer BAA requirement
- [ ] Run FHIR validator against first 10 bundles: `npx fhir-validator bundle.json --profile us-core`
- [ ] Add first 5 HL7/FHIR pairs as regression tests in `tests/hl7-scale-test.js`
