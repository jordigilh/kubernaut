# 📋 REQUEST: Unified Goose Migration Approach for Kind E2E Tests

**From**: HolmesGPT-API Team
**To**: Data Storage Team
**Date**: December 11, 2025
**Priority**: 🟡 MEDIUM (V1.0 nice-to-have, V1.1 recommended)
**Status**: 🔵 **PROPOSED**
**Response Deadline**: December 13, 2025 (or defer to V1.1)

---

## 📝 Executive Summary

We've updated `podman-compose.test.yml` to use **goose** for database migrations, replacing the bash script that had silent failures. This document proposes updating `test/infrastructure/migrations.go` to also use goose for **unified migration handling** across all test environments.

**Immediate Impact**: None - this is a proposal for consideration.
**Current State**: HAPI's podman-compose now uses goose; Kind E2E tests use custom Go code.

---

## 🔄 What Changed (Already Done)

### `podman-compose.test.yml` - HAPI Python Tests

**Before** (silent failures):
```yaml
migrate:
  image: quay.io/jordigilh/pgvector:pg16
  command:
    - |
      for f in /migrations/*.sql; do
        sed -e '/^-- +goose/d' "$$f" | psql ... 2>&1 || true  # ← Silent failures!
      done
```

**After** (proper error handling):
```yaml
migrate:
  image: ghcr.io/pressly/goose:3.18.0
  environment:
    - GOOSE_DRIVER=postgres
    - GOOSE_DBSTRING=postgres://slm_user:test_password@postgres:5432/action_history?sslmode=disable
  command: ["-dir", "/migrations", "up"]
```

**Benefits**:
- ✅ Proper error handling (goose fails loudly)
- ✅ Migration state tracking (`goose_db_version` table)
- ✅ Idempotent (won't re-apply already-applied migrations)
- ✅ Rollback support (`goose down`)

---

## 🎯 Proposal: Unify Kind E2E Tests on Goose

### Current State: `test/infrastructure/migrations.go`

```go
// Current approach: kubectl exec + psql + custom error handling
cmd := exec.Command("kubectl", "--kubeconfig", config.KubeconfigPath,
    "exec", "-i", "-n", config.Namespace, podName, "--",
    "psql", "-U", config.PostgresUser, "-d", config.PostgresDB)
cmd.Stdin = strings.NewReader(migrationSQL)

// Custom "already exists" detection
if strings.Contains(outputStr, "already exists") {
    fmt.Fprintf(writer, "   ✅ Migration %s already applied\n", migration)
    continue
}
```

### Proposed State: Use Goose in Kind

**Option A: Goose as Kubernetes Job**
```yaml
# Apply before E2E tests
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate
spec:
  template:
    spec:
      containers:
      - name: goose
        image: ghcr.io/pressly/goose:3.18.0
        env:
        - name: GOOSE_DRIVER
          value: postgres
        - name: GOOSE_DBSTRING
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: connection-string
        volumeMounts:
        - name: migrations
          mountPath: /migrations
        command: ["-dir", "/migrations", "up"]
      volumes:
      - name: migrations
        configMap:
          name: db-migrations
```

**Option B: Goose in Postgres Init Container**
```go
// In test/infrastructure/datastorage.go
func DeployPostgresWithMigrations(ctx context.Context, ...) error {
    // Deploy postgres with goose init container
    // Init container runs goose up before postgres starts accepting connections
}
```

**Option C: Keep Current for V1.0, Evaluate for V1.1**
- No changes needed for V1.0
- Current approach works, just less elegant
- Evaluate post-V1.0 if maintenance burden grows

---

## 📊 Comparison

| Aspect | Current (kubectl+psql) | Proposed (goose) |
|--------|----------------------|------------------|
| **Migration tracking** | ❌ None | ✅ `goose_db_version` |
| **Error handling** | 🟡 Custom code | ✅ Built-in |
| **Rollback support** | ❌ Manual | ✅ `goose down` |
| **Consistency with podman** | ❌ Different | ✅ Same tool |
| **Implementation effort** | ✅ Done | 🟡 ~4 hours |
| **Risk** | ✅ Proven | 🟡 New approach |

---

## 💡 Recommendation

### For V1.0 (December 2025)
```
Priority: LOW - Current approach works
Action: No changes required
Risk: None
```

### For V1.1 (January 2026)
```
Priority: MEDIUM - Unification benefit
Action: Evaluate Option A or B
Benefit: Single migration approach everywhere
```

---

## 🤔 Questions for DS Team

1. **V1.0 Scope**: Is there bandwidth to include this in V1.0, or should we defer to V1.1?

2. **Preferred Option**: If implementing, which approach do you prefer?
   - [ ] Option A: Goose as Kubernetes Job
   - [ ] Option B: Goose in Init Container
   - [ ] Option C: Defer to V1.1

3. **Goose Image**: Should we use the official `ghcr.io/pressly/goose` or build a custom image with goose pre-installed?

---

## 📎 Related Documents

| Document | Purpose |
|----------|---------|
| [REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md](./REQUEST_SHARED_E2E_MIGRATION_LIBRARY.md) | Original shared library request (IMPLEMENTED) |
| [DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md](./DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md) | DS implementation schedule |
| `test/infrastructure/migrations.go` | Current Kind E2E migration code |
| `podman-compose.test.yml` | Updated with goose (this change) |

---

## 📋 Response Template

```markdown
## Response: DS Team - Unified Goose Migrations

**Date**: [DATE]
**Decision**: [ACCEPT V1.0 / DEFER V1.1 / REJECT]

### Decision Rationale
[Why this decision]

### If Accepted for V1.0
- **Preferred Option**: [A / B]
- **ETA**: [Date]
- **Owner**: [Name]

### If Deferred to V1.1
- **Reason**: [Why defer]
- **Tentative V1.1 ETA**: [Date]

### Questions/Concerns
[Any questions for HAPI team]
```

---

## ✅ Confidence Assessment

| Decision | Confidence | Rationale |
|----------|------------|-----------|
| Defer to V1.1 | **85%** | Current Kind approach works, V1.0 timeline is tight |
| Include in V1.0 | **60%** | Benefit exists but implementation risk before GA |

**HAPI Team Recommendation**: Defer to V1.1 unless DS team has bandwidth and strong preference for unified approach in V1.0.

---

**Contact**: HolmesGPT-API Team
**Document Version**: 1.0

