# ezHealthKonnect Operations Runbook

This runbook covers the most common production incidents for the ezHealthKonnect HL7→FHIR integration platform. Each section includes symptoms, root cause checks, and resolution steps.

---

## 1. Dead-Letter Queue (DLQ) Full

**Symptom:** `delivery_dlq` has many rows with `status = 'pending'`. Alert fires on flag rate or DLQ depth in `interface_alert_thresholds`.

**Check depth:**
```sql
SELECT interface_id, COUNT(*), MAX(attempt_count), MIN(created_at)
FROM delivery_dlq
WHERE status IN ('pending','retrying')
GROUP BY interface_id
ORDER BY COUNT(*) DESC;
```

**Resolution options:**

| Option | When | SQL |
|--------|------|-----|
| Re-queue all pending | Destination is back up | `UPDATE delivery_dlq SET next_retry_at = NOW(), status='pending' WHERE status='abandoned'` |
| Abandon specific interface | Destination is decommissioned | `UPDATE delivery_dlq SET status='abandoned' WHERE interface_id='<uuid>' AND status='pending'` |
| Re-route payload manually | One-off rescue | Copy `payload` from row, POST to correct endpoint, then mark resolved |
| Mark all resolved | Testing data | `UPDATE delivery_dlq SET status='resolved', resolved_at=NOW() WHERE interface_id='<uuid>'` |

**Prevent recurrence:** Increase `max_dlq_depth` threshold in `interface_alert_thresholds` if the depth is expected, or fix the downstream connector config.

---

## 2. Database Disk Full

**Symptom:** PostgreSQL writes fail; application logs show `could not extend file` or `no space left on device`.

**Identify large tables:**
```sql
SELECT relname AS table, pg_size_pretty(pg_total_relation_size(oid)) AS size
FROM pg_class WHERE relkind = 'r'
ORDER BY pg_total_relation_size(oid) DESC
LIMIT 20;
```

**Immediate relief:**
```sql
-- Remove old quality scores (keep last 30 days)
DELETE FROM transformation_quality_scores WHERE created_at < NOW() - INTERVAL '30 days';

-- Remove resolved/abandoned DLQ rows older than 7 days
DELETE FROM delivery_dlq WHERE status IN ('resolved','abandoned') AND created_at < NOW() - INTERVAL '7 days';

-- Remove old metrics rows older than 90 days
DELETE FROM interface_transformation_metrics WHERE period_start < NOW() - INTERVAL '90 days';

-- Reclaim space after bulk deletes
VACUUM ANALYZE;
```

**Expand volume:** If the above is not enough, expand the PostgreSQL data volume via your cloud/infrastructure provider, then restart PostgreSQL.

**Apply retention policy:** If `retention_days` is not set on an interface, the default is 90 days. To verify:
```sql
SELECT i.name, i.retention_days
FROM interfaces i
WHERE i.retention_days IS NULL OR i.retention_days > 90;
```

---

## 3. TLS Certificate Expired (MLLP Listener)

**Symptom:** Sending systems report TLS handshake failures. Go logs show `certificate has expired or is not yet valid`.

**Check expiry:**
```bash
openssl x509 -in /path/to/cert.pem -noout -dates
```

**Rotate without downtime:**

1. Generate or obtain a new certificate:
   ```bash
   # Self-signed (dev/test only)
   openssl req -x509 -newkey rsa:4096 -keyout new_key.pem -out new_cert.pem -days 365 -nodes
   ```

2. Update the interface config in the database:
   ```sql
   UPDATE interface_connectivity
   SET source_config = jsonb_set(
       source_config,
       '{tls_cert_path}', '"/new/path/to/cert.pem"'
   )
   WHERE interface_id = '<uuid>' AND connector_type = 'tcp_mllp_inbound';
   ```

3. Deactivate and reactivate the interface via the UI (Admin → Interfaces → Deactivate → Activate), which restarts the MLLP listener with the new cert.

4. Verify the new cert is in use:
   ```bash
   echo | openssl s_client -connect localhost:6613 2>/dev/null | openssl x509 -noout -dates
   ```

---

## 4. Interface Stuck (Pipeline Not Processing)

**Symptom:** Messages arrive at the MLLP listener (ACK is returned) but no FHIR output appears. The interface shows as "active" but message count is not growing.

**Check pipeline status:**
```sql
SELECT p.id, p.name, p.status, p.last_executed_at
FROM transformation_pipelines p
JOIN interfaces i ON i.id = p.interface_id
WHERE i.id = '<interface-uuid>';
```

**Check for stuck steps:**
```sql
SELECT te.id, te.status, te.started_at, te.error_message
FROM transformation_executions te
WHERE te.interface_id = '<interface-uuid>'
  AND te.status = 'running'
ORDER BY te.started_at DESC
LIMIT 10;
```

**Reset a stuck execution:**
```sql
UPDATE transformation_executions
SET status = 'failed', error_message = 'Reset by operator — was stuck in running state'
WHERE status = 'running' AND started_at < NOW() - INTERVAL '10 minutes';
```

**Full pipeline reset:**
1. Deactivate the interface via UI or API: `POST /api/processing/interfaces/<uuid>/deactivate`
2. Wait 5 seconds
3. Reactivate: `POST /api/processing/interfaces/<uuid>/activate`

**Check processing engine health:**
```bash
curl http://localhost:8080/readyz
```
If the engine is not `"ok"`, restart the Go service: `docker restart go-api`

---

## 5. MinIO Unreachable

**Symptom:** Transformations succeed but raw HL7 and FHIR JSON cannot be retrieved from the UI. Logs show `connection refused` or `bucket not found` errors.

**Check connectivity:**
```bash
curl -s http://localhost:9000/minio/health/live
# Should return 200 OK
```

**Check environment:**
```bash
echo $OBJECT_STORAGE_DRIVER   # should be 'minio' or 's3'
echo $MINIO_ENDPOINT
echo $MINIO_BUCKET
```

**If MinIO is down:**
1. Restart the MinIO container: `docker restart minio`
2. Wait for health check: `curl -s http://localhost:9000/minio/health/ready`
3. Transformations continue to work (MinIO is storage-only, not in the critical path for HL7 delivery)
4. Messages that failed storage writes will be missing from the content viewer — these are not re-stored automatically

**Fallback to local storage:**
If MinIO will be unavailable for an extended period, switch to local file storage:
```bash
# In .env
OBJECT_STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=/data/ezhealthkonnect-storage
```
Restart the Go service after changing the env var.

---

## 6. Mapping Quality Alerts (Integration Team)

**Symptom:** Integration team is notified of flagged transformations in the Mapping Review queue (`/admin-mapping-review`).

**Review workflow:**
1. Navigate to Admin → Mapping Review
2. Filter by message type using the dropdown
3. Select a flagged record — HL7 and FHIR appear side-by-side
4. Identify the mapping gap (missing resource, empty required field, wrong code)
5. Click **OOB Template** to open the pipeline builder for that interface/message type
6. Fix the mapping rule in the pipeline
7. Return to the review queue and click **Mark Reviewed** with a resolution note
8. If the fix is a general OOB template change (not interface-specific), trigger a template rebuild:
   - Admin → Settings → OOB Templates → Rebuild for message type
   - Or: `POST /api/fhir/templates/rebuild-oob` (superadmin)

**Prevent recurrence:** After fixing, re-run the scale test with `--level field` to verify the mapping:
```powershell
node tests\hl7-scale-test.js --types "ORU^R01" --level field --wait 12
```
All field-level checks should pass before the fix is considered complete.

---

## 7. Scale Test Reference

Run the full corpus test (41 messages, 14 types):
```powershell
# Standard validation
node tests\hl7-scale-test.js --wait 12

# Field-level semantic validation
node tests\hl7-scale-test.js --level field --wait 15

# Concurrent load test (5 parallel connections)
node tests\hl7-scale-test.js --concurrent 5 --wait 20

# Specific message type only
node tests\hl7-scale-test.js --types "ADT^A01,ORU^R01" --wait 10

# Repeat each message 3 times (stress test)
node tests\hl7-scale-test.js --repeat 3 --concurrent 3 --wait 30
```

Results are written to `tests/hl7-scale-test-results/`. Open the HTML report for a visual summary.

---

## 8. Emergency Contacts & Escalation

| Role | Responsible for |
|------|----------------|
| Integration Engineer | Mapping fixes, OOB template updates, scale test sign-off |
| Platform Engineer | Database, MinIO, Docker infrastructure |
| Security Officer | TLS cert rotation, PHI exposure incidents |

For PHI exposure incidents: immediately isolate the affected interface (deactivate via UI), notify the Security Officer, and preserve logs before any remediation.
