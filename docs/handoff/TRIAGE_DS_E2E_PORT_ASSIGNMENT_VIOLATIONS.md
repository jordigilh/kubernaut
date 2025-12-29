# TRIAGE: DataStorage E2E Port Assignment Violations

**Date**: December 16, 2025
**Issue**: E2E tests and infrastructure using wrong ports
**Severity**: 🚨 **CRITICAL** - Violates authoritative documentation
**Status**: 🔍 **TRIAGE COMPLETE** - Implementation required

---

## 🚨 **Issue Summary**

DataStorage E2E tests are using **WRONG PORTS** that violate the authoritative port allocation strategy in **DD-TEST-001**.

### **Current (WRONG)**
```
PostgreSQL: localhost:5432  ❌ Wrong!
Data Storage: localhost:8081 ❌ Wrong!
```

### **Authoritative (CORRECT)**
Per [DD-TEST-001-port-allocation-strategy.md](../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) lines 106-127:

```yaml
DataStorage E2E Tests (test/e2e/datastorage/):
  PostgreSQL:
    Host Port: 25433          ✅ Correct
    Container Port: 5432
    Connection: localhost:25433

  Redis:
    Host Port: 26379          ✅ Correct
    Container Port: 6379
    Connection: localhost:26379

  Data Storage API:
    Host Port: 28090          ✅ Correct
    Container Port: 8080
    Connection: http://localhost:28090

  Embedding Service:
    Host Port: 28000          ✅ Correct
    Container Port: 8000
    Connection: http://localhost:28000
```

---

## 📋 **Files with Port Violations**

### **1. `test/infrastructure/kind-datastorage-config.yaml`**

**Current (WRONG)**:
```yaml
extraPortMappings:
- containerPort: 30081  # Data Storage NodePort in cluster
  hostPort: 8081        # ❌ WRONG: Should be 28090
  protocol: TCP
- containerPort: 30432  # PostgreSQL NodePort in cluster
  hostPort: 5432        # ❌ WRONG: Should be 25433
  protocol: TCP
```

**Should Be**:
```yaml
extraPortMappings:
- containerPort: 30081  # Data Storage NodePort in cluster
  hostPort: 28090       # ✅ CORRECT per DD-TEST-001
  protocol: TCP
- containerPort: 30432  # PostgreSQL NodePort in cluster
  hostPort: 25433       # ✅ CORRECT per DD-TEST-001
  protocol: TCP
```

---

### **2. `test/e2e/datastorage/datastorage_e2e_suite_test.go`**

**Current (WRONG)**:
```go
// Line 175-176
dataStorageURL = "http://localhost:8081"  // ❌ WRONG
postgresURL = "postgresql://slm_user:test_password@localhost:5432/action_history?sslmode=disable"  // ❌ WRONG

// Line 195-196 (port-forward fallback)
pgLocalPort := 5432 + (processID * 100)   // ❌ WRONG base
dsLocalPort := 8081 + (processID * 100)   // ❌ WRONG base
```

**Should Be**:
```go
// NodePort URLs (per DD-TEST-001)
dataStorageURL = "http://localhost:28090"  // ✅ CORRECT
postgresURL = "postgresql://slm_user:test_password@localhost:25433/action_history?sslmode=disable"  // ✅ CORRECT

// Port-forward fallback (per DD-TEST-001)
pgLocalPort := 25433 + (processID * 100)   // ✅ CORRECT base
dsLocalPort := 28090 + (processID * 100)   // ✅ CORRECT base
```

---

### **3. All E2E Test Files**

**Files Using Wrong Hardcoded Ports**:
- `test/e2e/datastorage/01_happy_path_test.go` - Line 113: `localhost port=5432`
- `test/e2e/datastorage/02_dlq_fallback_test.go` - Lines 109, 143: `localhost port=5432`
- `test/e2e/datastorage/04_workflow_search_test.go` - Line 110: `localhost port=5432`
- `test/e2e/datastorage/06_workflow_search_audit_test.go` - Line 94: `localhost port=5432`
- `test/e2e/datastorage/08_workflow_search_edge_cases_test.go` - Line 108: `localhost port=5432`

**All Should Change**:
```go
// BEFORE (WRONG)
connStr := "host=localhost port=5432 user=slm_user password=test_password dbname=action_history sslmode=disable"

// AFTER (CORRECT per DD-TEST-001)
connStr := "host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable"
```

---

### **4. Test Documentation**

**File**: `test/e2e/datastorage/README.md`

**Current (WRONG)**:
```markdown
## Port Allocation

- Data Storage: localhost:8081    ❌ Wrong
- PostgreSQL: localhost:5432       ❌ Wrong
- Redis: localhost:6379            ❌ Wrong
```

**Should Be** (per DD-TEST-001):
```markdown
## Port Allocation (per DD-TEST-001)

- Data Storage: localhost:28090    ✅ Correct
- PostgreSQL: localhost:25433      ✅ Correct
- Redis: localhost:26379           ✅ Correct
- Embedding: localhost:28000       ✅ Correct
```

---

### **5. Handoff Document**

**File**: `docs/handoff/DS_E2E_PODMAN_PORT_FORWARD_FIX.md`

**Current (WRONG)** - Lines 20-22:
```markdown
**Actual Issue**: Tests were trying to connect to `localhost:5432`, but:
1. **Kind with Docker**: `extraPortMappings` expose NodePorts to localhost ✅
2. **Kind with Podman**: `extraPortMappings` DON'T expose NodePorts to localhost ❌
```

**Should Document** (per DD-TEST-001):
```markdown
**Actual Issue**: Tests were trying to connect to `localhost:25433` (per DD-TEST-001), but:
1. **Kind with Docker**: `extraPortMappings` expose NodePorts to localhost ✅
2. **Kind with Podman**: `extraPortMappings` DON'T expose NodePorts to localhost ❌

**Port Assignments per DD-TEST-001**:
- DataStorage: localhost:28090 (E2E), localhost:18090 (Integration)
- PostgreSQL: localhost:25433 (E2E), localhost:15433 (Integration)
```

---

## 📚 **Authoritative Documentation**

### **DD-TEST-001: Port Allocation Strategy**

**File**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`

**Key Sections**:
- **Lines 67-76**: Allocation rules and ranges
- **Lines 80-127**: DataStorage specific port assignments
- **Lines 106-127**: E2E test ports (THE AUTHORITY)

**Port Allocation Philosophy** (Lines 69-75):
```
Integration Tests: 15433-18139 range (Podman containers)
E2E Tests (Podman): 25433-28139 range
E2E Tests (Kind NodePort): 30080-30099 (API), 30180-30199 (Metrics)
Host Port Mapping: 8080-8089 (for Kind extraPortMappings)  ← ❌ IGNORED!
```

**CRITICAL INSIGHT**: The document says `Host Port Mapping: 8080-8089` BUT the detailed DataStorage section (lines 106-127) says `28090`. This is an inconsistency in DD-TEST-001 itself!

---

## 🔍 **Root Cause Analysis**

### **Why This Happened**

1. **Port Assignment Confusion**: DD-TEST-001 has conflicting information:
   - **Line 73**: "Host Port Mapping: 8080-8089 (for Kind extraPortMappings)"
   - **Line 119**: "Data Storage API: Host Port: 28090"

2. **Implementation Followed Line 73**: The Kind config and tests used `8081` and `5432` following the "Host Port Mapping" section

3. **Missed Detailed Assignments**: The detailed DataStorage section (lines 106-127) was not referenced during implementation

### **Which is Authoritative?**

**ANSWER**: **Lines 106-127** (Detailed DataStorage assignments)

**Rationale**:
- More specific than general rules
- Explicitly labeled "Data Storage Service" section
- Provides complete service breakdown
- Matches pattern for other services (Gateway, Notification, etc.)

---

## ✅ **Correct Port Assignments (AUTHORITATIVE)**

### **DataStorage E2E Tests**

| Service | Authoritative Port (DD-TEST-001) | Current Port (WRONG) | Status |
|---------|----------------------------------|----------------------|--------|
| **Data Storage API** | **28090** | 8081 | ❌ WRONG |
| **PostgreSQL** | **25433** | 5432 | ❌ WRONG |
| **Redis** | **26379** | 6379 | ❌ WRONG |
| **Embedding** | **28000** | (not used in V1.0) | N/A |

### **Process-Specific Ports (Parallel Execution)**

For parallel test execution (process ID 1, 2, 3...):

```go
// Port-forward fallback (Podman)
processID := GinkgoParallelProcess()

// PostgreSQL
pgBasePort := 25433  // ✅ Per DD-TEST-001
pgLocalPort := pgBasePort + (processID * 100)
// Process 1: 25533
// Process 2: 25633
// Process 3: 25733

// Data Storage
dsBasePort := 28090  // ✅ Per DD-TEST-001
dsLocalPort := dsBasePort + (processID * 100)
// Process 1: 28190
// Process 2: 28290
// Process 3: 28390
```

---

## 🔧 **Required Changes**

### **Priority 1: Infrastructure Config**

**File**: `test/infrastructure/kind-datastorage-config.yaml`

```yaml
extraPortMappings:
- containerPort: 30081
  hostPort: 28090      # Changed from 8081
  protocol: TCP
- containerPort: 30432
  hostPort: 25433      # Changed from 5432
  protocol: TCP
```

---

### **Priority 2: E2E Suite**

**File**: `test/e2e/datastorage/datastorage_e2e_suite_test.go`

```go
// Try NodePort first (works with Docker)
dataStorageURL = "http://localhost:28090"  // Changed from 8081
postgresURL = "postgresql://slm_user:test_password@localhost:25433/action_history?sslmode=disable"  // Changed from 5432

// Port-forward fallback (Podman)
pgLocalPort := 25433 + (processID * 100)   // Changed from 5432
dsLocalPort := 28090 + (processID * 100)   // Changed from 8081
```

---

### **Priority 3: Individual Test Files**

**All E2E test files** need connection string updates:

```go
// OLD (WRONG)
connStr := "host=localhost port=5432 user=slm_user ..."

// NEW (CORRECT)
connStr := "host=localhost port=25433 user=slm_user ..."
```

**Files to Update**:
1. `01_happy_path_test.go` - Line 113
2. `02_dlq_fallback_test.go` - Lines 109, 143
3. `04_workflow_search_test.go` - Line 110
4. `06_workflow_search_audit_test.go` - Line 94
5. `08_workflow_search_edge_cases_test.go` - Line 108

---

### **Priority 4: Documentation**

1. **`test/e2e/datastorage/README.md`** - Update port assignments
2. **`docs/handoff/DS_E2E_PODMAN_PORT_FORWARD_FIX.md`** - Correct port references
3. **DD-TEST-001** - Clarify "Host Port Mapping: 8080-8089" vs detailed assignments

---

## 📋 **Verification Steps**

After applying fixes:

```bash
# 1. Verify Kind config has correct ports
grep -A2 "extraPortMappings" test/infrastructure/kind-datastorage-config.yaml
# Should show: hostPort: 28090 and hostPort: 25433

# 2. Verify suite test has correct ports
grep "localhost:2" test/e2e/datastorage/datastorage_e2e_suite_test.go
# Should show: localhost:28090 and localhost:25433

# 3. Verify individual tests have correct ports
grep -r "port=5432\|port=8081" test/e2e/datastorage/*.go
# Should return ZERO matches

# 4. Verify DD-TEST-001 compliance
grep "25433\|28090" test/e2e/datastorage/*.go
# Should show multiple matches in all test files
```

---

## 🎯 **Impact Assessment**

### **Why This Matters**

1. **Port Collision Risk**: Using production-like ports (`5432`, `8081`) increases collision risk with local services
2. **Parallel Execution**: Wrong base ports affect process-specific port calculation
3. **Documentation Compliance**: Violates DD-TEST-001 authoritative standard
4. **Team Coordination**: Other teams follow DD-TEST-001; DataStorage should too

### **What Works Despite Wrong Ports**

- ✅ Tests pass because ports are **consistently** wrong everywhere
- ✅ No actual collisions (yet) because tests run in isolation
- ✅ Kind cluster works because it's internal to the cluster

### **What Could Break**

- ❌ Parallel execution with other services' E2E tests
- ❌ Running E2E tests while DataStorage integration tests are running
- ❌ Developer confusion (docs say 28090, tests use 8081)
- ❌ CI/CD pipeline port conflicts

---

## 📚 **Related Documentation**

### **Authoritative Standards**
- **DD-TEST-001** - Port allocation strategy (PRIMARY AUTHORITY)
- **TESTING_GUIDELINES.md** - E2E test best practices

### **Implementation References**
- **Gateway E2E** - Follows DD-TEST-001 correctly (use as reference)
- **SignalProcessing E2E** - Follows DD-TEST-001 correctly (use as reference)

### **Handoff Documents**
- **DS_E2E_PODMAN_PORT_FORWARD_FIX.md** - Needs port corrections
- **TEAM_ANNOUNCEMENT_MIGRATION_AUTO_DISCOVERY.md** - Migration pattern reference

---

## ✅ **Sign-Off**

**Issue**: DataStorage E2E tests violate DD-TEST-001 port allocations
**Status**: ✅ **TRIAGE COMPLETE**
**Severity**: 🚨 **CRITICAL** - Compliance violation
**Authoritative Source**: DD-TEST-001 lines 106-127

**Required Action**:
1. Update Kind config (`kind-datastorage-config.yaml`)
2. Update E2E suite (`datastorage_e2e_suite_test.go`)
3. Update all E2E test files (6 files total)
4. Update documentation (README, handoff docs)
5. Re-run E2E tests to verify

**Priority**: **HIGH** - Should be fixed before V1.0 release to ensure DD-TEST-001 compliance

---

**Date**: December 16, 2025
**Triaged By**: AI Assistant (After user identified port assignment issue)
**Authority**: DD-TEST-001-port-allocation-strategy.md lines 106-127



