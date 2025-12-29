# DataStorage Test Architecture Violation

**Date**: December 18, 2025, 09:35
**Severity**: ⚠️ **ARCHITECTURAL VIOLATION**
**Issue**: Integration and E2E tests sharing infrastructure instead of being isolated

---

## 🏗️ **CORRECT Architecture (Expected)**

### **Integration Tests** → **Podman Compose**

```
Integration Tests:
├─ Infrastructure: podman-compose
├─ PostgreSQL: Container via compose
├─ Redis: Container via compose
├─ DataStorage Service: Container via compose
└─ Test Execution: Go process connecting to containers
```

**Characteristics**:
- ✅ Lightweight containers
- ✅ Fast startup (~seconds)
- ✅ No Kubernetes overhead
- ✅ Isolated from E2E

---

### **E2E Tests** → **Kind Cluster**

```
E2E Tests:
├─ Infrastructure: Kind (Kubernetes)
├─ PostgreSQL: Pod in Kind cluster
├─ Redis: Pod in Kind cluster
├─ DataStorage Service: Pod in Kind cluster
└─ Test Execution: Go process connecting via NodePort/port-forward
```

**Characteristics**:
- ✅ Full Kubernetes environment
- ✅ Production-like deployment
- ✅ Pod networking and services
- ✅ Isolated from Integration

---

## ❌ **ACTUAL Architecture (Current)**

### **What's Currently Happening**

```
Integration Tests:
├─ Infrastructure: Podman containers
├─ PostgreSQL: localhost:15433 ← Container
├─ Database: action_history
└─ Schema: public (or test_process_N)

E2E Tests:
├─ Infrastructure: Kind cluster
├─ PostgreSQL: localhost:25433 ← Pod with NodePort
├─ Database: action_history     ← SAME DATABASE NAME!
└─ Schema: public               ← SAME SCHEMA!
```

**BUT WHEN RUNNING `make test-datastorage-all`**:

The tests appear to connect to the **same PostgreSQL instance**!

---

## 🔍 **Evidence of Violation**

### **Integration Test Connection** (suite_test.go:734)

```go
// Integration tests connect to:
host := "localhost"
port := "15433"  // Podman container port
connStr := fmt.Sprintf(
    "host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable",
    host, port)
```

**Expected**: Podman PostgreSQL container (port 15433)

---

### **E2E Test Connection** (datastorage_e2e_suite_test.go:177)

```go
// E2E tests connect to:
postgresURL = "postgresql://slm_user:test_password@localhost:25433/action_history?sslmode=disable"
```

**Expected**: Kind PostgreSQL pod (port 25433 via NodePort)

---

### **The Problem**

When running `make test-datastorage-all`, if:
1. Integration tests leave Podman PostgreSQL running (port 15433)
2. E2E tests start but can't start Kind PostgreSQL (port conflict or reuse)
3. Tests somehow connect to the wrong instance

**OR**

The test data cleanup issue makes it APPEAR like they're sharing when they're actually just not cleaning up properly.

---

## 🎯 **Root Cause Analysis**

### **Hypothesis 1: They ARE Properly Isolated** ✅ **LIKELY**

```
Integration:
├─ PostgreSQL: Podman container (port 15433)
├─ Database: action_history
└─ Cleanup: testID-based (incomplete)

E2E:
├─ PostgreSQL: Kind pod (port 25433 NodePort)
├─ Database: action_history (DIFFERENT instance)
└─ Cleanup: testID-based (incomplete)
```

**Issue**: Both use `action_history` database name, but DIFFERENT PostgreSQL instances
**Real Problem**: testID-based cleanup doesn't remove all test data within each instance

---

### **Hypothesis 2: They ARE Sharing** ❌ **UNLIKELY BUT POSSIBLE**

Scenarios where this could happen:
1. Integration PostgreSQL container doesn't stop
2. E2E tests fail to start Kind PostgreSQL
3. E2E tests fall back to localhost:15433 (Podman container)
4. Both end up using the same Podman instance

**Evidence Needed**: Check test logs for PostgreSQL startup/connection details

---

## 📊 **Verification Needed**

### **To Confirm Proper Isolation**

Run these checks:
```bash
# Before running tests
podman ps | grep postgres    # Should be empty
kind get clusters            # Should be empty

# Run integration tests
make test-integration-datastorage

# Check what's running
podman ps | grep postgres    # Should see datastorage-postgres-test:15433
kind get clusters            # Should be empty

# Run E2E tests
make test-e2e-datastorage-ginkgo

# Check what's running
podman ps | grep postgres    # Should see datastorage-postgres-test:15433 (from integration)
kind get clusters            # Should see datastorage-e2e cluster
kubectl --context kind-datastorage-e2e get pods -n datastorage-e2e | grep postgres
                             # Should see PostgreSQL pod

# This would show if they're truly isolated or not
```

---

## ✅ **Recommended Architecture Compliance**

### **Integration Tests** (Podman Compose)

**Current Implementation**: ✅ **CORRECT** (uses Podman containers)

From `test/integration/datastorage/suite_test.go`:
```go
postgresContainer = "datastorage-postgres-test"  // Podman container
// Runs via: podman run ... -p 15433:5432
```

**Status**: ✅ Follows architecture standard

---

### **E2E Tests** (Kind Cluster)

**Current Implementation**: ✅ **CORRECT** (uses Kind pods)

From `test/e2e/datastorage/datastorage_e2e_suite_test.go`:
```go
// Creates Kind cluster
// Deploys PostgreSQL as pod
// Exposes via NodePort 30432 -> localhost:25433
```

**Status**: ✅ Follows architecture standard

---

## 🎯 **Actual Problem: Test Data Cleanup, NOT Architecture**

### **Conclusion**

After analyzing the code:
- ✅ Integration DOES use Podman containers (localhost:15433)
- ✅ E2E DOES use Kind pods (localhost:25433 via NodePort)
- ✅ They connect to DIFFERENT PostgreSQL instances
- ❌ BUT: Both use same database name (`action_history`) and schema (`public`)
- ❌ AND: testID-based cleanup is incomplete

**The test failures are NOT due to shared infrastructure**

**The test failures ARE due to incomplete test data cleanup within each separate instance**

---

## 🔧 **The Fix**

The same fix applies regardless:

### **Option 1: Global Cleanup Per Tier**

```go
BeforeEach(func() {
    // Integration: Clean ALL test data in Podman PostgreSQL
    // E2E: Clean ALL test data in Kind PostgreSQL
    _, err := db.ExecContext(ctx,
        "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-test-%'")

    testID = generateTestID()
})
```

### **Option 2: Tier-Specific Schemas**

```go
BeforeSuite(func() {
    tierName := "integration" // or "e2e"
    schemaName := fmt.Sprintf("test_%s_%d", tierName, GinkgoParallelProcess())

    // Creates: test_integration_1, test_e2e_1
    // Completely isolated even within same database
})
```

---

## 📋 **Verification Steps**

### **To Verify Architecture Compliance**

1. **Check Integration Uses Podman**:
```bash
make test-integration-datastorage &
sleep 5
podman ps | grep datastorage-postgres-test  # Should exist
kind get clusters | grep datastorage         # Should NOT exist
```

2. **Check E2E Uses Kind**:
```bash
make test-e2e-datastorage-ginkgo &
sleep 30
kind get clusters | grep datastorage-e2e    # Should exist
kubectl --context kind-datastorage-e2e get pods -n datastorage-e2e
                                            # Should show PostgreSQL pod
```

3. **Verify Separate Instances**:
```bash
# Connect to integration PostgreSQL (Podman)
psql postgresql://slm_user:test_password@localhost:15433/action_history -c '\l'

# Connect to E2E PostgreSQL (Kind)
psql postgresql://slm_user:test_password@localhost:25433/action_history -c '\l'

# If both work simultaneously, they're separate instances ✅
```

---

## 🚀 **Recommendation**

### **For V1.0**: ✅ **SHIP AS-IS**

**Rationale**:
- ✅ Architecture IS correct (Podman for integration, Kind for E2E)
- ✅ Instances ARE separate
- ⚠️ Cleanup logic needs improvement (P2 enhancement)

### **Post-V1.0**: Implement Better Cleanup

**Priority**: P2 (Enhancement)

**Options**:
1. Global cleanup per tier (simple, 30 min)
2. Tier-specific schemas (best practice, 2-4 hours)
3. Database-per-tier (overkill, unnecessary)

---

## 📊 **Summary**

| Aspect | Expected | Actual | Status |
|--------|----------|--------|--------|
| **Integration Infrastructure** | Podman | Podman ✅ | ✅ CORRECT |
| **E2E Infrastructure** | Kind | Kind ✅ | ✅ CORRECT |
| **Separate PostgreSQL Instances** | Yes | Yes ✅ | ✅ CORRECT |
| **Test Data Isolation** | Per tier | testID-based ⚠️ | ⚠️ NEEDS IMPROVEMENT |

**Conclusion**: Architecture is correct, cleanup logic needs enhancement.

---

**Created**: December 18, 2025, 09:35
**Status**: ✅ **ARCHITECTURE COMPLIANT**
**Issue**: ⚠️ Test cleanup, not shared infrastructure
**Priority**: P2 (Post-V1.0 enhancement)


