# WorkflowExecution Port Allocation Reassessment

**Date**: December 22, 2025
**Status**: 🔍 **ANALYSIS COMPLETE** - Corrected after DD-TEST-002 verification
**Trigger**: User correction - WE moved to sequential startup Dec 21, 2025
**Authority**: DD-TEST-002, WE_INTEGRATION_TEST_SEQUENTIAL_STARTUP_FIX_DEC_21_2025.md

---

## ✅ **CORRECTED UNDERSTANDING**

WorkflowExecution **DID** migrate from `podman-compose` to DD-TEST-002 sequential startup on **December 21, 2025**.

**Current Infrastructure**: `test/integration/workflowexecution/setup-infrastructure.sh` (shell script, not podman-compose)

**Source**: `docs/handoff/WE_INTEGRATION_TEST_SEQUENTIAL_STARTUP_FIX_DEC_21_2025.md`

---

## 📋 **Current State (ACCURATE)**

### **WorkflowExecution Actual Ports** (from `setup-infrastructure.sh`)

```bash
# Lines 24-28 of setup-infrastructure.sh
POSTGRES_PORT="15443"
REDIS_PORT="16389"
DATASTORAGE_HTTP_PORT="18100"
DATASTORAGE_METRICS_PORT="19100"
```

**Implementation**: Sequential `podman run` commands (DD-TEST-002 compliant)
**Status**: ✅ **WORKING** (43/52 tests passing, infrastructure stable)

### **Migration Timeline**

| Date | Event | Status |
|------|-------|--------|
| Before Dec 21 | Using `podman-compose.test.yml` | ❌ Race conditions, Exit 137 failures |
| Dec 21, 2025 | Migrated to DD-TEST-002 sequential startup | ✅ Infrastructure stable, 47% fewer failures |
| Dec 22, 2025 | This reassessment | 🔍 Ports still non-DD-TEST-001 compliant |

---

## 🚨 **Problems (UPDATED)**

### **Problem 1: Not in DD-TEST-001** ❌

WorkflowExecution is **still absent** from DD-TEST-001 Integration Tests section despite having active infrastructure.

**DD-TEST-001 Status**:
- Lines 212-238: Only documents "Workflow Engine" (not WorkflowExecution)
- **NO** PostgreSQL, Redis, or full DataStorage stack documented for WE
- Collision matrix line 608: Lists WE as using only DS dependency 18093

**Reality**: WE is running **FULL DataStorage stack** via sequential startup script

---

### **Problem 2: Ports Don't Match DD-TEST-001 Pattern** ⚠️

**DD-TEST-001 Sequential Pattern**:
- PostgreSQL: 154XX (DS:15433, EM:15434, RO:15435, SP:15436, GW:15437, AI:15438, **Next: 15441**)
- Redis: 163XX (DS:16379, EM:N/A, RO:16381, SP:16382, GW:16383, AI:16384, **Next: 16387**)
- DataStorage: 180XX (DS:18090, GW:18091, EM:18092, **WE should be: 18097**)
- Metrics: 190XX (DS:19090, GW:19091, EM:19092, **WE should be: 19097**)

**WE Actual Ports** (from shell script):
- PostgreSQL: **15443** (doesn't follow sequential pattern)
- Redis: **16389** (doesn't follow sequential pattern)
- DataStorage: **18100** (doesn't follow pattern, **CONFLICTS with EffectivenessMonitor!**)
- Metrics: **19100** (doesn't follow pattern)

**Pattern Origin**: "+10 from DS baseline" (outdated, predates DD-TEST-001)

---

### **Problem 3: No Infrastructure Constants** ❌

**Expected**: `test/infrastructure/workflowexecution.go` should have **Integration Test constants** like:
```go
const (
    WEIntegrationPostgresPort    = 15443 // or 15441 if aligned
    WEIntegrationRedisPort       = 16389 // or 16387 if aligned
    WEIntegrationDataStoragePort = 18100 // or 18097 if aligned
    WEIntegrationMetricsPort     = 19100 // or 19097 if aligned
)
```

**Reality**: `test/infrastructure/workflowexecution.go` **ONLY** has E2E infrastructure (Kind cluster setup)

**Current State**: Ports hardcoded in shell script (`setup-infrastructure.sh`)

---

### **Problem 4: Suite Test Documentation Outdated** ⚠️

**File**: `test/integration/workflowexecution/suite_test.go` (lines 215-217)

**Current Documentation** (OUTDATED):
```go
"To run these tests, start infrastructure:\n"+
"  cd test/integration/workflowexecution\n"+
"  podman-compose -f podman-compose.test.yml up -d\n\n"+  // ❌ WRONG!
```

**Should Be**:
```go
"To run these tests, start infrastructure:\n"+
"  cd test/integration/workflowexecution\n"+
"  ./setup-infrastructure.sh\n\n"+  // ✅ CORRECT (sequential startup)
```

---

## 🔍 **Port Conflict Analysis** (CONFIRMED)

### **CONFLICT: EffectivenessMonitor** 🚨

**EffectivenessMonitor** (per DD-TEST-001 lines 172-208):
```yaml
Effectiveness Monitor API: 18100
```

**WorkflowExecution** (actual usage in setup-infrastructure.sh):
```yaml
DataStorage HTTP: 18100
```

**Impact**:
- ❌ Cannot run EffectivenessMonitor + WorkflowExecution integration tests in parallel
- ⚠️ **Theoretical conflict** (EffectivenessMonitor integration tests don't exist yet)
- 🚨 **Will block** future EffectivenessMonitor integration test implementation

---

## ✅ **RECOMMENDED: Align WE to DD-TEST-001 Pattern**

### **Step 1: Update Port Allocations**

**Change `setup-infrastructure.sh` ports to DD-TEST-001 compliant**:

```bash
# OLD (lines 24-28 of setup-infrastructure.sh):
POSTGRES_PORT="15443"
REDIS_PORT="16389"
DATASTORAGE_HTTP_PORT="18100"  # CONFLICTS with EffectivenessMonitor!
DATASTORAGE_METRICS_PORT="19100"

# NEW (DD-TEST-001 sequential pattern):
POSTGRES_PORT="15441"           # Sequential after AIAnalysis (15438)
REDIS_PORT="16387"              # Sequential after AIAnalysis (16384)
DATASTORAGE_HTTP_PORT="18097"   # Sequential (resolves EM conflict!)
DATASTORAGE_METRICS_PORT="19097" # Matches DataStorage XX pattern
```

---

### **Step 2: Update Configuration Files**

**Files to Update**:

1. `test/integration/workflowexecution/setup-infrastructure.sh` (lines 24-28)
   - Update port constants

2. `test/integration/workflowexecution/config/config.yaml`
   - Update connection strings to match container hostnames
   - Verify `database.host`, `redis.addr` use container names

3. `test/integration/workflowexecution/suite_test.go` (line 85)
   ```go
   // OLD:
   dataStorageBaseURL string = "http://localhost:18100"

   // NEW:
   dataStorageBaseURL string = "http://localhost:18097"  // DD-TEST-001 compliant
   ```

4. `test/integration/workflowexecution/suite_test.go` (lines 215-217)
   - Update documentation to reference `setup-infrastructure.sh` not `podman-compose`

---

### **Step 3: Create Infrastructure Constants** ✨ **NEW**

**CREATE**: `test/infrastructure/workflowexecution_integration.go`

```go
package infrastructure

// WorkflowExecution Integration Test Ports (per DD-TEST-001 v1.7 - December 2025)
// Sequential startup pattern per DD-TEST-002
const (
	// PostgreSQL port for WorkflowExecution integration tests
	WEIntegrationPostgresPort = 15441 // Sequential after AIAnalysis (15438)

	// Redis port for WorkflowExecution integration tests
	WEIntegrationRedisPort = 16387 // Sequential after AIAnalysis (16384)

	// DataStorage HTTP API port for WorkflowExecution integration tests
	WEIntegrationDataStoragePort = 18097 // Sequential, resolves EM conflict

	// DataStorage Metrics port for WorkflowExecution integration tests
	WEIntegrationMetricsPort = 19097 // DD-TEST-001 metrics pattern

	// Container Names (match setup-infrastructure.sh)
	WEIntegrationPostgresContainer    = "workflowexecution_postgres_1"
	WEIntegrationRedisContainer       = "workflowexecution_redis_1"
	WEIntegrationDataStorageContainer = "workflowexecution_datastorage_1"
	WEIntegrationNetwork              = "workflowexecution_test-network"
)
```

**Rationale**:
- Other services (Gateway, RO, SP, AI) have integration constants in `test/infrastructure/`
- Separate file prevents confusion with E2E infrastructure (`workflowexecution.go`)
- Enables programmatic access to ports (future shared DS bootstrap migration)

---

### **Step 4: Update DD-TEST-001**

**Add WorkflowExecution detailed section** (after SignalProcessing, before Effectiveness Monitor):

```markdown
### WorkflowExecution Service

**Note**: WorkflowExecution integration tests use DD-TEST-002 sequential startup pattern (December 2025). See [WE_INTEGRATION_TEST_SEQUENTIAL_STARTUP_FIX_DEC_21_2025.md](../../handoff/WE_INTEGRATION_TEST_SEQUENTIAL_STARTUP_FIX_DEC_21_2025.md) for migration details.

#### Integration Tests (`test/integration/workflowexecution/`)
\`\`\`yaml
PostgreSQL:
  Host Port: 15441
  Container Port: 5432
  Connection: localhost:15441
  Purpose: DataStorage backend

Redis:
  Host Port: 16387
  Container Port: 6379
  Connection: localhost:16387
  Purpose: DataStorage DLQ

Data Storage (Dependency):
  Host Port: 18097
  Container Port: 8080
  Connection: http://localhost:18097
  Purpose: Audit events storage (BR-WE-005)

Data Storage Metrics:
  Host Port: 19097
  Container Port: 9090
  Connection: http://localhost:19097
  Purpose: DataStorage metrics
\`\`\`

**Infrastructure**: Sequential `podman run` via `setup-infrastructure.sh` (DD-TEST-002 pattern)
**Status**: ✅ Stable (43/52 tests passing as of Dec 21, 2025)
```

**Update Port Collision Matrix**:
```markdown
| Service | PostgreSQL | Redis | DataStorage Dep | Metrics |
|---------|------------|-------|-----------------|---------|
| **WorkflowExecution** | 15441 | 16387 | 18097 | 19097 |
```

**Add Revision v1.7**:
```markdown
| 1.7 | 2025-12-22 | AI Assistant | **WorkflowExecution Added**: 15441/16387/18097/19097 (DD-TEST-002 sequential startup, resolves EM conflict); Updated suite_test.go documentation to reference setup-infrastructure.sh not podman-compose |
```

---

## 📊 **Impact Assessment**

### **Current Risk**: LOW-MEDIUM

**Why Low-Medium**:
- ✅ WE infrastructure **IS working** (DD-TEST-002 implemented Dec 21)
- ✅ Tests passing (43/52, remaining failures not infrastructure-related)
- ⚠️ Port conflict with EffectivenessMonitor is **theoretical** (EM integration tests don't exist)
- ⚠️ Blocks future EffectivenessMonitor integration test development
- ⚠️ Non-DD-TEST-001 pattern prevents shared DS bootstrap migration

### **Urgency**: MEDIUM

**Fix Now**:
- Consistency with DD-TEST-001
- Enable future EM integration tests
- Prepare for shared DS bootstrap migration
- Document sequential startup pattern

**Can Wait**:
- Current infrastructure stable
- No immediate failures

---

## ✅ **Success Criteria**

- ✅ WE ports follow DD-TEST-001 pattern (15441/16387/18097/19097)
- ✅ No conflict with EffectivenessMonitor (18100 freed)
- ✅ `test/infrastructure/workflowexecution_integration.go` created with constants
- ✅ `setup-infrastructure.sh` updated with new ports
- ✅ `config/config.yaml` updated with correct hostnames
- ✅ `suite_test.go` updated with new DataStorage URL and correct infrastructure instructions
- ✅ DD-TEST-001 v1.7 documents WE with sequential startup pattern
- ✅ WE integration tests pass with new ports (43/52 or better)
- ✅ Port validation shows no duplicates

---

## 📋 **Migration Checklist**

### **Phase 1: Port Updates** (30 minutes)
- [ ] Update `setup-infrastructure.sh` ports (15441/16387/18097/19097)
- [ ] Update `config/config.yaml` connection strings
- [ ] Update `suite_test.go` dataStorageBaseURL (18100 → 18097)
- [ ] Update `suite_test.go` documentation (podman-compose → setup-infrastructure.sh)

### **Phase 2: Infrastructure Constants** (15 minutes)
- [ ] Create `test/infrastructure/workflowexecution_integration.go`
- [ ] Add port constants
- [ ] Add container name constants
- [ ] Validate no linter errors

### **Phase 3: DD-TEST-001 Update** (20 minutes)
- [ ] Add WorkflowExecution detailed section
- [ ] Update port collision matrix
- [ ] Add revision v1.7
- [ ] Cross-reference sequential startup handoff document

### **Phase 4: Validation** (15 minutes)
- [ ] Run `./setup-infrastructure.sh` with new ports
- [ ] Run WE integration tests: `go test ./test/integration/workflowexecution -v -timeout=10m`
- [ ] Verify 43/52+ tests passing
- [ ] Check for port conflicts: `grep -h "Integration.*Port\s*=" test/infrastructure/*.go | awk '{print $NF}' | sort -n | uniq -d`

**Total Estimated Time**: ~80 minutes

---

## 🎯 **Comparison: Before vs. After**

| Aspect | Current (Dec 22) | After Fix | Status |
|--------|------------------|-----------|--------|
| **Infrastructure** | ✅ Sequential startup | ✅ Sequential startup | **NO CHANGE** (already DD-TEST-002) |
| **PostgreSQL** | 15443 (ad-hoc) | 15441 (DD-TEST-001) | ⚠️ **CHANGE** |
| **Redis** | 16389 (ad-hoc) | 16387 (DD-TEST-001) | ⚠️ **CHANGE** |
| **DataStorage** | 18100 (conflicts!) | 18097 (DD-TEST-001) | 🚨 **MUST FIX** |
| **Metrics** | 19100 (ad-hoc) | 19097 (DD-TEST-001) | ⚠️ **CHANGE** |
| **DD-TEST-001** | Not documented | Detailed section v1.7 | ❌ **MISSING** |
| **Infrastructure Constants** | None | `workflowexecution_integration.go` | ❌ **MISSING** |
| **Suite Docs** | "podman-compose" (outdated) | "setup-infrastructure.sh" | ⚠️ **MISLEADING** |

---

## ❓ **Questions for User**

### **Critical Decision**

**Q1**: Approve Option A (align WE to DD-TEST-001 pattern)?
- PostgreSQL: 15443 → **15441**
- Redis: 16389 → **16387**
- DataStorage: 18100 → **18097** (resolves future EM conflict)
- Metrics: 19100 → **19097**

**Q2**: Priority level?
- **HIGH**: Fix with other port allocation fixes (batch update)
- **MEDIUM**: Fix separately after other services validated
- **LOW**: Fix when implementing shared DS bootstrap migration

**Q3**: Create `workflowexecution_integration.go` constants file?
- **YES**: Consistency with other services (Gateway, RO, SP, AI)
- **NO**: Keep ports in shell script only

---

**Document Status**: ✅ **COMPLETE** - Reassessed after DD-TEST-002 verification
**Confidence**: **100%** that WE is using DD-TEST-002 but non-DD-TEST-001 ports
**Recommended Action**: Align WE to DD-TEST-001 pattern (batch update with other fixes)











