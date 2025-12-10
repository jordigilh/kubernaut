# ✅ NOTICE: Data Storage Migrations Not Auto-Applied in Test Infrastructure

**From**: WorkflowExecution Team
**To**: Data Storage Team
**Date**: December 10, 2025
**Priority**: 🟡 MEDIUM
**Status**: ✅ **RESOLVED** (see [RESPONSE_DATASTORAGE_MIGRATION_FIXED.md](./RESPONSE_DATASTORAGE_MIGRATION_FIXED.md))
**Resolution Date**: December 10, 2025

---

## 📋 Problem Summary

The `podman-compose.test.yml` test infrastructure starts the Data Storage service successfully, but **database migrations are not automatically applied**. This causes all services that depend on the `audit_events` table to fail with HTTP 500 errors.

---

## 🔍 Root Cause Analysis

### What Happens

1. `podman-compose -f podman-compose.test.yml up -d` starts:
   - ✅ PostgreSQL (healthy)
   - ✅ Redis (healthy)
   - ✅ Data Storage service (healthy)
   - ✅ HolmesGPT API (healthy)

2. Services report healthy, but the database is **empty** (no tables)

3. When services call the batch audit endpoint:
   ```
   POST /api/v1/audit/events/batch
   ```

4. Data Storage returns HTTP 500:
   ```
   ERROR: relation "audit_events" does not exist (SQLSTATE 42P01)
   ```

### Evidence from Logs

```
2025-12-10T01:11:22.368Z ERROR datastorage server/audit_events_batch_handler.go:164
Batch database write failed
{"count": 1, "error": "failed to prepare statement: ERROR: relation \"audit_events\" does not exist (SQLSTATE 42P01)"}
```

---

## 🛠️ Current Workaround

We manually applied migrations by:

```bash
# Copy migrations into postgres container
podman cp migrations kubernaut_postgres_1:/tmp/

# Apply migrations (note: goose directives ignored, some migrations fail)
podman exec kubernaut_postgres_1 bash -c 'for f in /tmp/migrations/*.sql; do psql -U slm_user -d action_history -f "$f"; done'

# Manually create audit_events table (migration 013 failed due to goose syntax)
podman exec kubernaut_postgres_1 psql -U slm_user -d action_history -c "
CREATE TABLE IF NOT EXISTS audit_events (
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    event_version VARCHAR(10) NOT NULL DEFAULT '1.0',
    event_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    event_date DATE NOT NULL DEFAULT CURRENT_DATE,
    event_type VARCHAR(100) NOT NULL,
    event_category VARCHAR(50) NOT NULL,
    event_action VARCHAR(50) NOT NULL,
    event_outcome VARCHAR(20) NOT NULL,
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    actor_ip INET,
    resource_type VARCHAR(100) NOT NULL,
    resource_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),
    correlation_id VARCHAR(255) NOT NULL,
    parent_event_id UUID,
    parent_event_date DATE,
    trace_id VARCHAR(255),
    span_id VARCHAR(255),
    namespace VARCHAR(253),
    cluster_name VARCHAR(255),
    event_data JSONB NOT NULL,
    event_metadata JSONB,
    severity VARCHAR(20),
    duration_ms INTEGER,
    error_code VARCHAR(50),
    error_message TEXT,
    retention_days INTEGER DEFAULT 2555,
    is_sensitive BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (event_id, event_date)
) PARTITION BY RANGE (event_date);

CREATE TABLE audit_events_y2025m12 PARTITION OF audit_events
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
CREATE TABLE audit_events_y2026m01 PARTITION OF audit_events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
"
```

After this, all tests pass.

---

## 💡 Recommended Solutions

### Option A: Auto-Migrate on DS Startup (Recommended)

Add migration logic to Data Storage service startup:

```go
// cmd/datastorage/main.go
func main() {
    // ... existing config loading ...

    // Auto-apply migrations if enabled
    if cfg.AutoMigrate {
        if err := db.RunMigrations(cfg.MigrationsPath); err != nil {
            log.Fatal("Failed to run migrations", zap.Error(err))
        }
    }

    // ... rest of startup ...
}
```

**Pros**: Zero manual steps, works in all environments
**Cons**: Requires code change to DS service

### Option B: Init Container in podman-compose.test.yml

Add an init container that runs migrations before DS starts:

```yaml
services:
  migrate:
    image: migrate/migrate:v4.16.2
    volumes:
      - ./migrations:/migrations
    command: ["-path", "/migrations", "-database", "postgres://slm_user:test_password@postgres:5432/action_history?sslmode=disable", "up"]
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - test-network

  datastorage:
    depends_on:
      migrate:
        condition: service_completed_successfully  # Wait for migrations
      postgres:
        condition: service_healthy
```

**Pros**: No code changes to DS service
**Cons**: Requires `migrate` tool image, adds complexity to compose file

### Option C: Document Manual Migration Step

Add to test setup documentation:

```bash
# After starting test infrastructure
podman-compose -f podman-compose.test.yml up -d

# Apply migrations (REQUIRED before running tests)
make test-db-migrate  # New Makefile target

# Then run tests
go test ./test/integration/...
```

**Pros**: Simplest to implement
**Cons**: Easy to forget, CI will fail without explicit step

---

## 📊 Impact

| Service | Impact Without Fix |
|---------|-------------------|
| **WorkflowExecution** | ❌ All audit integration tests fail (HTTP 500) |
| **AIAnalysis** | ❌ Audit tests will fail (same dependency) |
| **Gateway** | ❌ Audit tests will fail (same dependency) |
| **Notification** | ❌ Audit tests will fail (same dependency) |
| **RemediationOrchestrator** | ❌ Audit tests will fail (same dependency) |

---

## ✅ Verification

After fix is applied, verify with:

```bash
# Start infrastructure
podman-compose -f podman-compose.test.yml up -d

# Verify audit_events table exists
podman exec kubernaut_postgres_1 psql -U slm_user -d action_history -c "\dt audit_events*"

# Expected output:
#  Schema |         Name          |       Type        |  Owner
# --------+-----------------------+-------------------+----------
#  public | audit_events          | partitioned table | slm_user
#  public | audit_events_y2025m12 | table             | slm_user
#  ...

# Run integration tests
go test ./test/integration/workflowexecution/... -v --ginkgo.focus="Real Data Storage"
# Expected: 6 PASSED
```

---

## 🔗 Related Documents

| Document | Purpose |
|----------|---------|
| [NOTICE_DATASTORAGE_BATCH_AUDIT_ENDPOINT_COMPLETE.md](./NOTICE_DATASTORAGE_BATCH_AUDIT_ENDPOINT_COMPLETE.md) | Batch endpoint implementation confirmation |
| [ADR-034](../architecture/decisions/ADR-034-unified-audit-table-design.md) | Audit table schema specification |
| [013_create_audit_events_table.sql](../../migrations/013_create_audit_events_table.sql) | Migration that creates audit_events table |

---

## 📞 Questions?

Contact: WorkflowExecution Team

---

**Document Version**: 1.0
**Created**: December 10, 2025
**Maintained By**: WorkflowExecution Team

