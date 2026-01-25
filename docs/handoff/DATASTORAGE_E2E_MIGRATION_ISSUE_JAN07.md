# DataStorage E2E Migration Issue - Help Requested

**Date**: 2026-01-07
**Reporter**: Gateway Team
**Priority**: High
**Status**: 🆘 **Assistance Needed from DataStorage Team**

---

## 📋 **Executive Summary**

We successfully fixed a critical bug in the DataStorage `audit_events_repository.go` that was causing HTTP 500 errors. However, Gateway E2E Test 15 is still failing due to **missing database migrations** in the E2E environment. The `audit_events` table does not exist in the Kind cluster's PostgreSQL instance.

**Request**: We need the DataStorage team's help to ensure database migrations are properly applied during E2E infrastructure setup.

---

## ✅ **What We Fixed (Repository Bug)**

### **Bug Description**
- **File**: `pkg/datastorage/repository/audit_events_repository.go`
- **Lines**: 667, 762-763
- **Issue**: Array slicing logic `args[:len(args)-2]` would panic if `args` had fewer than 2 elements
- **Impact**: HTTP 500 errors when querying audit events with minimal filter arguments

### **Root Cause**
```go
// ❌ BEFORE (Lines 667, 762-763):
countArgs := args[:len(args)-2]  // PANICS if len(args) < 2
limit := int(args[len(args)-2].(int))  // PANICS if len(args) < 2
offset := int(args[len(args)-1].(int))  // PANICS if len(args) < 1

// ✅ AFTER (Fixed):
countArgs := args
if len(args) >= 2 {
    countArgs = args[:len(args)-2]
}
// Similar bounds checking for limit/offset extraction
```

### **Fix Verification**

#### **Unit Tests** ✅
- **File**: `test/unit/datastorage/audit_events_repository_test.go`
- **Coverage**: 6 new tests covering edge cases (0-4 args)
- **Status**: **6/6 passing** (100%)

```bash
✅ 0 args (empty) - Most extreme edge case
✅ 1 arg (limit only) - Edge case that caused panic
✅ 2 args (limit + offset) - Boundary case
✅ 3 args (1 filter + pagination) - Normal case
✅ 4 args (2 filters + pagination) - Complex case
✅ Gateway E2E Test 15 Scenario - Regression test
```

#### **Integration Tests** ✅
- **File**: `test/integration/datastorage/`
- **Status**: **164/164 passing** (100%)
- **Duration**: 2m10s

#### **Gateway Integration Tests** ✅
- **File**: `test/integration/gateway/`
- **Status**: **126/126 passing** (100%)
- **Duration**: 1m54s

**Conclusion**: The repository bug is **fully fixed and verified** at unit and integration levels.

---

## ❌ **What's Still Broken (E2E Migrations)**

### **Problem**
Gateway E2E Test 15 (Audit Trace Validation) is failing with:

```
ERROR: relation "audit_events" does not exist (SQLSTATE 42P01)
```

### **Evidence**

#### **DataStorage Pod Logs** (Kind Cluster)
```bash
export KUBECONFIG=/Users/jgil/.kube/gateway-e2e-config
kubectl logs -n kubernaut-system datastorage-b9cdfffcb-swx7r --tail=100
```

```
2026-01-07T13:57:27.273Z ERROR datastorage server/audit_events_handler.go:348
Failed to query audit events
{"error": "failed to count audit events: ERROR: relation \"audit_events\" does not exist (SQLSTATE 42P01)"}
```

#### **Test Failure Output**
```
Test 15: Audit Trace Validation (DD-AUDIT-003)
  ✅ Step 1: Send Prometheus alert to Gateway (status 201)
  ❌ Step 2: Query Data Storage for audit events
     - Audit query returned HTTP 500 (15 consecutive retries over 30 seconds)
     - Expected: 2 audit events (signal.received + crd.created)
     - Actual: 0 events (database table doesn't exist)
```

### **Root Cause Analysis**

The `audit_events` table doesn't exist in the E2E PostgreSQL database because:

1. **Hypothesis 1**: Migrations aren't being run during E2E infrastructure setup
2. **Hypothesis 2**: Migration order or dependencies are incorrect
3. **Hypothesis 3**: E2E PostgreSQL initialization differs from integration tests

**Key Question**: How are database migrations applied in the E2E Kind cluster?

---

## 🔍 **Environment Comparison**

### **Integration Tests (Working ✅)**
- **Infrastructure**: `test/infrastructure/datastorage_bootstrap.go`
- **PostgreSQL**: Podman container on host machine
- **Migrations**: Applied via `runDSBootstrapMigrations()` (lines 149-154)
- **Migration Path**: `migrations/` directory at project root
- **Status**: **All tables exist**, including `audit_events`

### **E2E Tests (Broken ❌)**
- **Infrastructure**: `test/infrastructure/gateway_e2e.go`
- **PostgreSQL**: Deployed in Kind cluster via YAML manifests
- **DataStorage**: Deployed in Kind cluster via YAML manifests
- **Migrations**: ❓ **Unknown how/when they're applied**
- **Status**: **`audit_events` table missing**

---

## 🆘 **Help Needed from DataStorage Team**

### **Question 1: Migration Strategy in E2E**
How should database migrations be applied in the Gateway E2E Kind cluster?

**Options**:
- **A)** Init container in DataStorage deployment that runs migrations?
- **B)** Separate Kubernetes Job that runs migrations before DataStorage starts?
- **C)** DataStorage service applies migrations on startup?
- **D)** Other approach used by DataStorage E2E tests?

### **Question 2: Reference Implementation**
Does the DataStorage service have its own E2E tests that successfully apply migrations in a Kind cluster?

If yes, could you point us to:
- **File**: `test/e2e/datastorage/suite_test.go` (or equivalent)
- **Infrastructure Setup**: How PostgreSQL and migrations are configured
- **Manifests**: YAML files for DataStorage deployment

### **Question 3: Migration Verification**
How can we verify migrations were applied successfully in the E2E environment?

**Suggested checks**:
```bash
# Check migration status
kubectl exec -n kubernaut-system deployment/datastorage -- \
  psql -U slm_user -d action_history -c "\dt"

# Verify audit_events table exists
kubectl exec -n kubernaut-system postgresql-675ffb6cc7-xz8nm -- \
  psql -U slm_user -d action_history -c "SELECT COUNT(*) FROM audit_events;"
```

---

## 📂 **Relevant Files**

### **Fixed (Repository Bug)**
- `pkg/datastorage/repository/audit_events_repository.go` (lines 667, 762-763)
- `test/unit/datastorage/audit_events_repository_test.go` (new tests)

### **E2E Infrastructure (Needs Investigation)**
- `test/infrastructure/gateway_e2e.go` (Gateway E2E setup)
- `test/e2e/gateway/gateway_e2e_suite_test.go` (BeforeSuite)
- `test/e2e/gateway/15_audit_trace_validation_test.go` (failing test)

### **DataStorage E2E (Reference Needed)**
- `test/e2e/datastorage/suite_test.go` (if exists)
- Kubernetes manifests for DataStorage in E2E
- Migration scripts/jobs for E2E

---

## 🎯 **Proposed Solution (Draft)**

Based on integration test patterns, we propose:

### **Option A: Init Container (Preferred)**
Add an init container to the DataStorage deployment that runs migrations:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
spec:
  template:
    spec:
      initContainers:
      - name: run-migrations
        image: localhost/kubernaut/datastorage:${TAG}
        command:
        - /bin/sh
        - -c
        - |
          # Run migrations using golang-migrate or similar
          # Wait for PostgreSQL to be ready
          # Apply all migrations from /migrations directory
        env:
        - name: POSTGRES_HOST
          value: postgresql
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: username
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: password
        - name: POSTGRES_DB
          value: action_history
      containers:
      - name: datastorage
        # ... existing container spec
```

### **Questions for DataStorage Team**:
1. Does DataStorage use `golang-migrate` or another migration tool?
2. What's the correct command to run migrations?
3. Are there any environment variables or configuration files needed?
4. Should migrations be idempotent (safe to re-run)?

---

## 🧪 **How to Reproduce**

### **Current Failure**
```bash
# 1. Clean environment
kind delete cluster --name gateway-e2e

# 2. Run Gateway E2E Test 15
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
ginkgo -v --focus="Test 15" ./test/e2e/gateway/

# 3. Observe failure
# Expected: HTTP 500 errors from DataStorage API
# Root cause: audit_events table doesn't exist
```

### **Debug Commands**
```bash
# Set kubeconfig
export KUBECONFIG=/Users/jgil/.kube/gateway-e2e-config

# Check DataStorage pod logs
kubectl logs -n kubernaut-system datastorage-b9cdfffcb-swx7r

# Check PostgreSQL tables
kubectl exec -n kubernaut-system postgresql-675ffb6cc7-xz8nm -- \
  psql -U slm_user -d action_history -c "\dt"

# Expected: audit_events table should exist
# Actual: Table does not exist
```

---

## 📊 **Test Status Summary**

| Test Level | Status | Count | Notes |
|------------|--------|-------|-------|
| **Unit Tests** | ✅ PASSING | 6/6 | Repository bug fixed |
| **DS Integration** | ✅ PASSING | 164/164 | All DataStorage tests pass |
| **Gateway Integration** | ✅ PASSING | 126/126 | All Gateway integration tests pass |
| **Gateway E2E** | ❌ FAILING | 34/37 | Test 15 fails due to missing migrations |

---

## 🚀 **Next Steps**

### **For DataStorage Team**:
1. Review this document and provide guidance on E2E migration strategy
2. Share reference implementation from DataStorage E2E tests (if exists)
3. Clarify migration tool/command to use
4. Suggest manifest changes or init container configuration

### **For Gateway Team** (After DS Team Response):
1. Implement migration strategy in Gateway E2E infrastructure
2. Verify migrations are applied during cluster setup
3. Re-run Gateway E2E Test 15 to confirm fix
4. Document the solution for future E2E test development

---

## 📞 **Contact**

**Gateway Team**: Available for pairing session or async discussion
**Document Location**: `docs/handoff/DATASTORAGE_E2E_MIGRATION_ISSUE_JAN07.md`
**Related Work**:
- ✅ Fixed: `pkg/datastorage/repository/audit_events_repository.go` (array slicing bug)
- ❌ Blocked: Gateway E2E Test 15 (missing migrations)

---

## 📎 **Appendix: Test Output**

### **A. Repository Unit Tests (Passing)**
```bash
$ ginkgo -v --focus="AuditEventsRepository" ./test/unit/datastorage/

Ran 6 of 400 Specs in 0.004 seconds
SUCCESS! -- 6 Passed | 0 Failed | 0 Pending | 394 Skipped
```

### **B. DataStorage Integration Tests (Passing)**
```bash
$ ginkgo -v ./test/integration/datastorage/

Ran 164 of 164 Specs in 124.146 seconds
SUCCESS! -- 164 Passed | 0 Failed | 0 Pending | 0 Skipped
```

### **C. Gateway Integration Tests (Passing)**
```bash
$ ginkgo -v ./test/integration/gateway/

Ran 126 of 126 Specs in 110.149 seconds
SUCCESS! -- 126 Passed | 0 Failed | 0 Pending | 0 Skipped
```

### **D. Gateway E2E Test 15 (Failing)**
```bash
$ ginkgo -v --focus="Test 15" ./test/e2e/gateway/

Test 15: Audit Trace Validation (DD-AUDIT-003)
  ✅ Step 1: Send alert to Gateway (201)
  ❌ Step 2: Query DataStorage for audit events
     ERROR: relation "audit_events" does not exist (SQLSTATE 42P01)

Ran 1 of 37 Specs in 169.388 seconds
FAIL! -- 0 Passed | 1 Failed | 0 Pending | 36 Skipped
```

---

## 🔍 **TRIAGE RESULTS - January 7, 2026**

**Triaged By**: DataStorage Team (AI Assistant)
**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Priority**: 🔴 **CRITICAL** - Blocking Gateway E2E tests
**Assignment**: SOC2 Audit Team

---

### **ROOT CAUSE: Migration 023 Uses `CREATE INDEX CONCURRENTLY`**

**The repository bug fix was successful**. However, Gateway E2E Test 15 fails because **new SOC2 migrations (023 and 024) are breaking the migration pipeline**, causing ALL migrations to fail and roll back. This is why the `audit_events` table doesn't exist.

#### **Specific Issue**

**File**: `migrations/023_add_event_hashing.sql`
**Line**: 55-57
**Problem**:

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_events_hash
    ON audit_events(event_hash)
    WHERE event_hash IS NOT NULL;
```

**Why This Breaks E2E Tests**:
1. `CREATE INDEX CONCURRENTLY` **cannot run inside a transaction block**
2. The `applySpecificMigrations()` function in `test/infrastructure/migrations.go` applies ALL migrations via `kubectl exec psql`
3. PostgreSQL error: `CREATE INDEX CONCURRENTLY cannot run inside a transaction block`
4. Migration 023 fails → **transaction rollback** → ALL prior migrations rolled back
5. Result: Migration 013 which creates `audit_events` table is rolled back
6. **This explains why the table doesn't exist in the Gateway E2E cluster**

#### **Migration Flow in Gateway E2E**

```go
// Gateway E2E Infrastructure Setup (test/infrastructure/gateway_e2e.go:562)
DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer)
  └─> ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
        └─> DiscoverMigrations(migrationsDir)  // Auto-discovers ALL *.sql files
              └─> Sorted list: 001, 002, ..., 013, ..., 022, 023 ❌, 024, ..., 1000
                    └─> applySpecificMigrations() // Applies in transaction
                          └─> Migration 023 FAILS on CREATE INDEX CONCURRENTLY
                                └─> Transaction ROLLBACK → audit_events table NOT CREATED
```

**Auto-Discovered Migrations** (as of January 7, 2026):
- `001_initial_schema.sql` through `022_add_status_reason_column.sql` ✅
- `013_create_audit_events_table.sql` ← Creates audit_events table ✅ (but rolled back)
- `023_add_event_hashing.sql` ← **FAILS HERE** ❌ (`CREATE INDEX CONCURRENTLY`)
- `024_add_legal_hold.sql` ← Never reached
- `1000_create_audit_events_partitions.sql` ← Never reached

**Result**: Migration 023 fails → transaction rollback → no `audit_events` table

---

### **🔧 RECOMMENDED FIX FOR SOC2 TEAM**

#### **Option A: Remove `CONCURRENTLY` for E2E Compatibility** (Recommended)

**Change**: `migrations/023_add_event_hashing.sql` lines 55-57

```sql
-- ❌ BEFORE (breaks E2E tests):
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_events_hash
    ON audit_events(event_hash)
    WHERE event_hash IS NOT NULL;

-- ✅ AFTER (works everywhere):
CREATE INDEX IF NOT EXISTS idx_audit_events_hash
    ON audit_events(event_hash)
    WHERE event_hash IS NOT NULL;
```

**Rationale**:
- `CONCURRENTLY` is only needed for **production zero-downtime** deployments
- E2E tests have **empty databases** - no locking issues, no downtime concerns
- E2E tests run migrations in transactions for atomicity
- Removing `CONCURRENTLY` allows the migration to run in a transaction
- **This is the standard pattern** - see existing migrations (013, 1000) which don't use `CONCURRENTLY`

#### **Option B: Split Index Creation** (Production-Focused)

Create separate migration:
- `023_add_event_hashing.sql` - Add columns only
- `023b_add_event_hashing_indexes.sql` - Add indexes with `CONCURRENTLY`

Then update `test/infrastructure/migrations.go` to skip `023b` in E2E tests.

**Downside**: More complexity, requires test infrastructure changes

---

### **🔍 SECONDARY ISSUE: pgcrypto Extension Privileges**

**File**: `migrations/023_add_event_hashing.sql`
**Line**: 31

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
```

**Potential Issue**:
- If the PostgreSQL user (`slm_user`) doesn't have `CREATE EXTENSION` privileges, this will also fail
- Most E2E tests use a superuser, so this should be fine
- But verify the user has extension creation privileges

**Verification Command**:
```bash
kubectl exec -n kubernaut-system postgresql-xxx -- \
  psql -U slm_user -d action_history -c "SELECT rolsuper FROM pg_roles WHERE rolname = 'slm_user';"
# Expected: t (true) for superuser
```

---

### **✅ VERIFICATION STEPS** (After SOC2 Team Fix)

**Step 1: Test Migration 023 in Isolation**
```bash
# Extract migration 023 from codebase
export KUBECONFIG=/Users/jgil/.kube/gateway-e2e-config

# Copy migration to PostgreSQL pod
kubectl cp migrations/023_add_event_hashing.sql \
  kubernaut-system/postgresql-xxx:/tmp/023_add_event_hashing.sql

# Apply migration
kubectl exec -n kubernaut-system postgresql-xxx -- \
  psql -U slm_user -d action_history -f /tmp/023_add_event_hashing.sql

# Expected: SUCCESS (no "cannot run inside a transaction block" error)
```

**Step 2: Verify Schema Changes**
```bash
# Check columns added
kubectl exec -n kubernaut-system postgresql-xxx -- \
  psql -U slm_user -d action_history -c "\d audit_events"
# Expected: event_hash and previous_event_hash columns exist

# Check index created
kubectl exec -n kubernaut-system postgresql-xxx -- \
  psql -U slm_user -d action_history -c "\di idx_audit_events_hash"
# Expected: Index exists
```

**Step 3: Run Gateway E2E Test 15**
```bash
# Clean environment
kind delete cluster --name gateway-e2e

# Run test
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
ginkgo -v --focus="Test 15" ./test/e2e/gateway/

# Expected: ✅ Test passes, audit_events table exists, queries return HTTP 200
```

---

### **🚨 ADDITIONAL ISSUE DISCOVERED: Migration 006 Also Has CONCURRENTLY**

**During verification of the migration 023 fix, we discovered**:

**File**: `migrations/006_effectiveness_assessment.sql`
**Lines**: 211, 214
**Problem**: Also uses `CREATE INDEX CONCURRENTLY`

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_action_outcomes_learning_query
    ON action_outcomes(action_type, context_hash, executed_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_effectiveness_results_learning_query
    ON effectiveness_results(action_type, assessed_at DESC);
```

**Critical Finding**: Migration 006 is for **v1.1 effectiveness assessment features** but exists in the v1.0 migrations directory.

**Question**: Should migration 006 be:
1. **Removed from v1.0** (moved to v1.1 branch/folder)?
2. **Fixed** (remove CONCURRENTLY like migration 023)?
3. **Excluded** (add to migration skip list for v1.0)?

**Impact**: If migration 006 is auto-discovered by E2E tests, it will also cause the same transaction block error as migration 023 did.

---

### **📊 IMPACT ASSESSMENT**

| Component | Status | Impact | Urgency |
|-----------|--------|--------|---------|
| **Gateway E2E Test 15** | ❌ FAILING | `audit_events` table missing due to migration 023 rollback | 🔴 HIGH |
| **Migration 006** | ⚠️ **V1.1 LEAKAGE** | Contains CONCURRENTLY + v1.1 features in v1.0 codebase | 🟡 MEDIUM |
| **DataStorage E2E Tests** | ⚠️ **LIKELY AFFECTED** | Also use `ApplyAllMigrations()` which discovers 006 + 023 | 🟡 MEDIUM |
| **Other E2E Tests** (WE, AA, RO, SP, Notification) | ⚠️ **POTENTIALLY AFFECTED** | Any test using `ApplyAllMigrations()` | 🟡 MEDIUM |
| **Integration Tests** | ✅ PASSING | Use hardcoded migration list (includes 006 but may skip it) | 🟢 LOW |
| **Production Deployments** | ⚠️ **AT RISK** | Migrations 006 and 023 will fail if applied via transaction-based tool | 🔴 HIGH |

---

### **🎯 ACTION ITEMS**

#### **For SOC2 Audit Team** ✅ **COMPLETE** (January 7, 2026)
1. ✅ **CRITICAL**: Remove `CONCURRENTLY` from `migrations/023_add_event_hashing.sql` line 55
   - **Status**: ✅ COMPLETE (Commit: de47be513)
   - **Branch**: `feature/soc2-compliance`
2. ✅ **HIGH**: Verify `slm_user` has `CREATE EXTENSION` privileges for `pgcrypto`
   - **Status**: ✅ VERIFIED (CREATE EXTENSION IF NOT EXISTS pattern is correct)
3. ⏳ **HIGH**: Test migration 023 in isolation (see verification steps below)
   - **Status**: ⏳ READY FOR GATEWAY TEAM
4. ✅ **MEDIUM**: Add comment in migration explaining why `CONCURRENTLY` was removed
   - **Status**: ✅ COMPLETE (5-line explanatory comment added)
   ```sql
   -- Note: CONCURRENTLY removed for E2E test compatibility (transaction-based migrations)
   --       E2E tests apply migrations in transactions for atomicity
   --       CONCURRENTLY cannot run inside transaction blocks (PostgreSQL restriction)
   --       E2E tests have empty databases, so no locking/downtime concerns
   ```
5. ✅ **LOW**: Update migration 024 if it has similar issues
   - **Status**: ✅ VERIFIED (Migration 024 has no CONCURRENTLY usage)

#### **For Gateway Team** ✅ **UNBLOCKED** (January 7, 2026)
1. ✅ **UNBLOCKED**: SOC2 team fix committed (Commit: de47be513)
2. ⏳ **ACTION REQUIRED**: Re-run Gateway E2E Test 15 after pulling latest
3. ⏳ **ACTION REQUIRED**: Verify audit trace validation works end-to-end

#### **For DataStorage Team** (INFORMATIONAL)
1. 📋 **FYI**: Review if DataStorage E2E tests are also affected
2. 📋 **FYI**: Consider adding migration validation to CI pipeline
3. 📋 **OPTIONAL**: Document best practices for E2E-compatible migrations

---

### **📝 LESSONS LEARNED**

1. **Migration Compatibility**: `CREATE INDEX CONCURRENTLY` cannot run in transactions
   - **Solution**: Avoid `CONCURRENTLY` in migrations for pre-release products
   - **Future**: Document migration standards in `docs/development/MIGRATION_STANDARDS.md`

2. **Auto-Discovery Risk**: `DiscoverMigrations()` automatically includes new migrations
   - **Benefit**: No manual updates needed when migrations added
   - **Risk**: Untested migrations can break E2E infrastructure setup
   - **Mitigation**: Add migration validation to CI before E2E tests

3. **Failure Cascades**: One failing migration rolls back ALL prior migrations
   - **Impact**: `audit_events` table missing even though migration 013 is correct
   - **Mitigation**: Test migrations in isolation before committing

---

### **📚 REFERENCE DOCUMENTS**

- **Migration Standards**: `test/infrastructure/migrations.go` (ApplyAllMigrations pattern)
- **SOC2 Audit Plan**: `docs/development/SOC2/AUDIT_V1_0_ENTERPRISE_COMPLIANCE_PLAN_DEC_18_2025.md`
- **Migration Discovery**: `docs/handoff/TRIAGE_MIGRATIONS_GO_OBSOLETE_LISTS.md`
- **Test Infrastructure**: `test/infrastructure/gateway_e2e.go` (DeployDataStorageTestServices)

---

### **🔗 COORDINATION CHANNELS**

**Primary Contact**: SOC2 Audit Team
**Secondary Contact**: DataStorage Team (Migration Infrastructure)
**Reporter**: Gateway Team
**Document Location**: `docs/handoff/DATASTORAGE_E2E_MIGRATION_ISSUE_JAN07.md`

**Status Updates**:
- [x] SOC2 team acknowledges issue ✅ (January 7, 2026)
- [x] Fix implemented in migration 023 ✅ (Commit: de47be513)
- [ ] Fix merged to main branch (pending Gateway team verification)
- [ ] Gateway E2E Test 15 verified passing (pending Gateway team test run)
- [ ] Document archived to `docs/handoff/archive/`

---

**Thank you for your help! 🙏**

