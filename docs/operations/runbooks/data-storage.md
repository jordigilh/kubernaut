# Data Storage Service - Production Runbook

**Version**: v1.1
**Last Updated**: 2026-05-13
**Status**: Production Ready
**Related**: [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md), [BR-AUDIT-007](../../requirements/BR-AUDIT-007-tamper-evident-exports.md), [BR-AUDIT-009](../../requirements/BR-AUDIT-009-retention-policies.md), [#1048](https://github.com/jordigilh/kubernaut/issues/1048) Phase 5

> **Naming note**: This runbook uses Helm resource names (`deploy/datastorage`, `datastorage-config`, `datastorage-signing`). The kustomize manifests in `deploy/data-storage/` use different names (`data-storage-service`, `data-storage-config`, `datastorage-signing-cert`). Adjust `kubectl` commands accordingly for your deployment method.

---

## Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-DS-001 | [DLQ Backpressure / RPO Risk](#rb-ds-001-dlq-backpressure--rpo-risk) | `datastorage_dlq_stream_xadd_total` rising while depth at `dlqMaxLen` | Manual |
| RB-DS-002 | [Retention Worker Not Purging](#rb-ds-002-retention-worker-not-purging) | `retention_operations` table shows no recent runs | Manual |
| RB-DS-003 | [Signing Certificate Rotation](#rb-ds-003-signing-certificate-rotation) | Certificate approaching expiry (30-day threshold) | Manual / cert-manager |
| RB-DS-004 | [Redis TLS Connection Failure](#rb-ds-004-redis-tls-connection-failure) | DataStorage pod failing readiness with Redis TLS errors | Manual |
| RB-DS-005 | [PEL Stuck Messages](#rb-ds-005-pel-stuck-messages) | DLQ messages not being processed after consumer restart | Manual |
| RB-DS-006 | [Shutdown DLQ Drain Issues](#rb-ds-006-shutdown-dlq-drain-issues) | `datastorage_shutdown_dlq_drain_errors_total` incrementing during rollouts | Manual |

---

## Architecture Overview

The Data Storage service is the audit trail backend for Kubernaut. It receives audit events via REST API, buffers them through a Redis/Valkey DLQ (Dead Letter Queue), and persists them to PostgreSQL.

```
Service Controllers → REST API → DLQ (Redis Streams) → PostgreSQL
                                      ↓
                                 PEL Recovery (XAUTOCLAIM)
                                      ↓
                                 Dead-Letter Stream (poison messages)
```

Key subsystems:
- **Audit Event API**: REST endpoints for writing/reading audit events
- **DLQ Retry Worker**: Consumes Redis streams, inserts into PostgreSQL, retries on failure
- **PEL Recovery**: Reclaims orphaned messages from the Pending Entries List via XAUTOCLAIM
- **Retention Worker**: Periodically purges expired audit events (configurable, disabled by default)
- **Signing Certificate**: RSA 2048-bit key pair for tamper-evident audit export signatures

---

## RB-DS-001: DLQ Backpressure / RPO Risk

### Context: Recovery Point Objective (RPO)

Redis Streams used for the DLQ are bounded by `dlqMaxLen` (default: 10,000 messages). When the stream reaches this limit, older messages are trimmed on each `XADD` via `MAXLEN ~`. This means audit events that have not yet been persisted to PostgreSQL may be lost.

**RPO Calculation**:

```
RPO = dlqMaxLen × average_message_size
```

With defaults:
- `dlqMaxLen`: 10,000 messages
- Average audit event: ~2-5 KB
- **RPO**: ~20-50 MB of unsynced audit data at risk

### Symptoms

- `datastorage_dlq_stream_xadd_total` counter is incrementing steadily
- DLQ depth (from `XLEN` on the stream) plateaus at or near `dlqMaxLen`
- Audit events submitted via API are accepted (HTTP 201) but not appearing in PostgreSQL queries

### Alert Threshold

```
DLQ depth / dlqMaxLen > 0.95 sustained for 5 minutes
```

### Diagnosis

1. Check DLQ stream depth:
   ```bash
   kubectl exec -n kubernaut-system deploy/valkey -- redis-cli XLEN audit:dlq:audit_events
   ```

2. Check DLQ retry worker logs for errors:
   ```bash
   kubectl logs -n kubernaut-system deploy/datastorage --tail=100 | grep -i "dlq\|retry\|failed"
   ```

3. Check PostgreSQL connectivity:
   ```bash
   kubectl exec -n kubernaut-system deploy/datastorage -- curl -s localhost:8081/readyz
   ```

### Resolution

1. **Investigate PostgreSQL availability** — the most common cause is PostgreSQL being down or slow, causing the DLQ retry worker to fail inserts.

2. **Increase `dlqMaxLen`** if backpressure is expected (e.g., during bulk operations):
   ```yaml
   # values.yaml or operator config
   datastorage:
     config:
       redis:
         dlqMaxLen: 50000
   ```

3. **Scale PostgreSQL resources** if the database is the bottleneck (connection pool exhaustion, slow queries).

4. **Check DLQ retry worker health** — if the worker goroutine has exited, a pod restart will recover it.

### Prevention

- Monitor `datastorage_dlq_stream_xadd_total` rate alongside DLQ depth
- Monitor `datastorage_dlq_pel_pending` for PEL backlog growth (Phase 7)
- Monitor `datastorage_retention_purge_total` to confirm retention worker is active (Phase 7)
- Set up alerting on `DLQ depth / dlqMaxLen > 0.95`
- Size `dlqMaxLen` based on expected peak write rate and acceptable RPO

---

## RB-DS-002: Retention Worker Not Purging

### Context

The retention worker periodically deletes audit events older than `retention_days` (default: 2555 days / ~7 years per ADR-034, SOC 2 / ISO 27001). It runs in batches and respects `legal_hold = TRUE` rows.

### Configuration

```yaml
retention:
  enabled: false          # Must be explicitly enabled
  interval: "24h"         # How often the worker runs
  batchSize: 1000         # Max rows per DELETE batch
  defaultDays: 2555       # Application-level default (clamped to max 2555)
```

### Symptoms

- `retention_operations` table has no recent entries
- Disk usage growing unboundedly despite retention being enabled
- Worker log line `"Retention worker disabled"` appearing at startup

### Diagnosis

1. Check if retention is enabled in the config:
   ```bash
   kubectl get cm datastorage-config -n kubernaut-system -o yaml | grep -A5 retention
   ```

2. Check retention operation history:
   ```sql
   SELECT run_id, status, rows_deleted, operation_start, operation_duration_ms
   FROM retention_operations
   ORDER BY operation_start DESC
   LIMIT 10;
   ```

3. Check for legal-held rows blocking purge:
   ```sql
   SELECT COUNT(*) FROM audit_events
   WHERE legal_hold = TRUE
   AND event_timestamp < NOW() - INTERVAL '2555 days';
   ```

### Resolution

1. **Enable retention** if it's disabled:
   ```yaml
   retention:
     enabled: true
   ```

2. **Check for failed runs** in `retention_operations` — a `status: failed` entry with `error_message` indicates the root cause.

3. **Adjust batch size** if purge runs are timing out on large tables.

---

## RB-DS-003: Signing Certificate Rotation

### Context

The Data Storage service requires an RSA 2048-bit signing certificate (`tls.crt`, `tls.key`) for tamper-evident audit export signatures (BR-AUDIT-007, FedRAMP AU-9). The service **will not start** without a valid certificate (fail-hard, no self-signed fallback).

### Certificate Location

- **Helm/dev**: Kubernetes Secret `datastorage-signing`, mounted at `signerCertDir` (default: `/etc/certs`)
- **Production (operator)**: Managed by `kubernaut-operator` ([#90](https://github.com/jordigilh/kubernaut-operator/issues/90))

### Symptoms

- DataStorage pod in `CrashLoopBackOff`
- Log message: `"failed to load signing certificate: signing certificate not found at /etc/certs/tls.crt"`
- Log message: `"signing certificate must have RSA private key"`

### Diagnosis

1. Check the signing secret exists:
   ```bash
   kubectl get secret datastorage-signing -n kubernaut-system
   ```

2. Check certificate expiry:
   ```bash
   kubectl get secret datastorage-signing -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | \
     base64 -d | openssl x509 -noout -enddate
   ```

3. Verify the key is RSA (not ECDSA):
   ```bash
   kubectl get secret datastorage-signing -n kubernaut-system -o jsonpath='{.data.tls\.key}' | \
     base64 -d | openssl rsa -check -noout
   ```

### Resolution

1. **Regenerate the certificate** (Helm environments):
   The `tls-cert-job` Helm hook automatically generates a new RSA 2048-bit certificate if the secret is missing or within 30 days of expiry. Force regeneration:
   ```bash
   kubectl delete secret datastorage-signing -n kubernaut-system
   helm upgrade kubernaut charts/kubernaut -n kubernaut-system
   ```

2. **Production (operator)**: Follow the operator's cert rotation procedure. The operator manages certificate lifecycle via cert-manager ([#84](https://github.com/jordigilh/kubernaut-operator/issues/84)).

### Important Notes

- The signing certificate is **separate** from the inter-service TLS certificate (`datastorage-tls`). Do not confuse the two.
- ECDSA keys are **not supported** for signing — the signer requires RSA.

---

## RB-DS-004: Redis TLS Connection Failure

### Context

When Redis TLS is enabled (`redis.tls.enabled: true`), the Data Storage service uses TLS for all Redis/Valkey connections. Misconfiguration causes connection failures at startup.

### Symptoms

- DataStorage pod failing readiness probe
- Log message: `"failed to connect to Redis"` with TLS handshake errors
- Log message: `"failed to read Redis CA file"` or `"failed to load Redis client certificate"`

### Diagnosis

1. Check TLS configuration:
   ```bash
   kubectl get cm datastorage-config -n kubernaut-system -o yaml | grep -A10 "redis:" | grep -A6 "tls:"
   ```

2. Verify the CA/cert files exist in the pod:
   ```bash
   kubectl exec -n kubernaut-system deploy/datastorage -- ls -la /etc/redis/
   ```

3. Test TLS connectivity directly:
   ```bash
   kubectl exec -n kubernaut-system deploy/datastorage -- \
     openssl s_client -connect valkey:6379 -CAfile /etc/redis/ca.crt
   ```

### Resolution

1. **Verify Valkey has TLS enabled** — the server must be configured for TLS ([kubernaut-operator#89](https://github.com/jordigilh/kubernaut-operator/issues/89)).

2. **Check certificate paths** match the mounted secret paths.

3. **For dev/test**, set `insecureSkipVerify: true` to bypass CA verification:
   ```yaml
   redis:
     tls:
       enabled: true
       insecureSkipVerify: true
   ```

---

## RB-DS-005: PEL Stuck Messages

### Context

The DLQ retry worker uses a Pending Entries List (PEL) recovery mechanism to reclaim orphaned messages after consumer crashes. XAUTOCLAIM runs every 30 seconds and claims messages idle for more than 60 seconds. Messages exceeding 5 delivery attempts are moved to the dead-letter stream.

### Tuning Constants

| Parameter | Value | Description |
|-----------|-------|-------------|
| `PelRecoveryClaimInterval` | 30s | How often the janitor runs XAUTOCLAIM |
| `PelRecoveryMinIdleTime` | 60s | Minimum idle time before a message is claimable |
| `PelRecoveryClaimCount` | 10 | Max messages claimed per sweep |
| `PelRecoveryMaxDeliveries` | 5 | Poison message threshold — dead-lettered after this count |

### Symptoms

- Messages visible in `XPENDING` but not being processed
- `datastorage_dlq_stream_xadd_total` not increasing but depth not decreasing
- Dead-letter stream growing (poison messages)

### Diagnosis

1. Check pending messages:
   ```bash
   kubectl exec -n kubernaut-system deploy/valkey -- \
     redis-cli XPENDING audit:dlq:audit_events datastorage-group - + 10
   ```

2. Check dead-letter stream:
   ```bash
   kubectl exec -n kubernaut-system deploy/valkey -- \
     redis-cli XLEN audit:dead-letter:audit_events
   ```

3. Look for poison message patterns in logs:
   ```bash
   kubectl logs -n kubernaut-system deploy/datastorage --tail=200 | grep -i "dead.letter\|poison\|max.deliver"
   ```

### Resolution

1. **Poison messages** (delivery count > 5) are automatically moved to the dead-letter stream. Investigate the root cause (e.g., malformed event data, PostgreSQL constraint violation).

2. **If XAUTOCLAIM is not supported** (Valkey < 6.2), the worker logs a warning at startup. Upgrade Valkey.

3. **Manual recovery** — acknowledge stuck messages if they are known to be processed:
   ```bash
   kubectl exec -n kubernaut-system deploy/valkey -- \
     redis-cli XACK audit:dlq:audit_events datastorage-group <message-id>
   ```

---

## RB-DS-006: Shutdown DLQ Drain Issues

### Context

During graceful shutdown (DD-007 + DD-008), the Data Storage service drains pending DLQ messages to PostgreSQL before closing connections. The drain runs with a 10-second timeout budget after the DLQ retry worker is stopped and HTTP connections are drained. Each shutdown is assigned a `shutdown_id` UUID for log correlation.

Shutdown step order:
1. Set readiness flag (Kubernetes removes pod from endpoints)
2. Wait for endpoint propagation (configurable, default 5s)
3. Drain in-flight HTTP connections (30s budget)
4. Stop DLQ retry worker, then drain DLQ to PostgreSQL (10s budget)
5. Stop retention worker, then close PostgreSQL and audit store

### Symptoms

- `datastorage_shutdown_dlq_drain_errors_total` is incrementing during rollouts
- `datastorage_dlq_drain_batch_total` increments but logs show errors with `dd=DD-008-step-4-error`
- Pod termination takes longer than expected (`terminationGracePeriodSeconds: 90`)
- Audit events appear in DLQ (`XLEN`) after pod restart, indicating incomplete drain

### Diagnosis

1. Correlate shutdown logs using `shutdown_id`:
   ```bash
   kubectl logs -n kubernaut-system deploy/datastorage --previous | grep "shutdown_id"
   ```

2. Check drain statistics in logs:
   ```bash
   kubectl logs -n kubernaut-system deploy/datastorage --previous | grep "DD-008-step-4"
   ```

3. Check DLQ depth after restart (non-zero means messages survived the drain):
   ```bash
   kubectl exec -n kubernaut-system deploy/valkey -- redis-cli XLEN audit:dlq:audit_events
   kubectl exec -n kubernaut-system deploy/valkey -- redis-cli XLEN audit:dlq:audit_notifications
   ```

4. Check PostgreSQL connectivity at shutdown time (drain fails if DB is unreachable):
   ```bash
   kubectl logs -n kubernaut-system deploy/datastorage --previous | grep "failed to close PostgreSQL\|DLQ drain failed"
   ```

### Resolution

1. **Drain timeout** — if `timed_out: true` appears in drain logs, the 10s budget was insufficient. Messages that were not drained remain in Redis and will be picked up by the next pod's retry worker on startup. This is safe (at-least-once semantics) but delays persistence.

2. **PostgreSQL unreachable** — if drain errors show connection failures, investigate PostgreSQL availability. The drain cannot persist messages if the database is down. Messages remain in Redis.

3. **Large DLQ backlog** — if DLQ depth is very high at shutdown time, the 10s drain budget may not be enough. Consider draining the backlog before initiating a rolling update:
   ```bash
   # Check current backlog
   kubectl exec -n kubernaut-system deploy/valkey -- redis-cli XLEN audit:dlq:audit_events
   # Wait for the retry worker to reduce it before deploying
   ```

4. **Repeated errors across rollouts** — check `datastorage_shutdown_dlq_drain_errors_total` rate. If errors are consistent, the root cause is likely a persistent PostgreSQL issue, not the drain itself.

### Metrics

| Metric | Type | Purpose |
|--------|------|---------|
| `datastorage_dlq_drain_batch_total` | Counter | Total drain attempts during shutdown |
| `datastorage_shutdown_dlq_drain_errors_total` | Counter | Drain errors during shutdown |
| `datastorage_dlq_pel_pending` | Gauge | PEL backlog depth (pre-drain) |
