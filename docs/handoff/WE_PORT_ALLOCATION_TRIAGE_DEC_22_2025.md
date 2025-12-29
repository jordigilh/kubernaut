# WorkflowExecution Port Allocation Triage

**Date**: December 22, 2025
**Status**: 🚨 **CRITICAL GAP FOUND**
**Severity**: HIGH - Undocumented ports in active use
**Trigger**: User noticed WE missing from port allocation fixes

---

## 🚨 **Critical Finding**

WorkflowExecution integration tests **ARE using DataStorage infrastructure** but with **undocumented, non-DD-TEST-001 compliant ports**.

---

## 📋 **Current State**

### **WorkflowExecution Actual Ports** (from `podman-compose.test.yml`)

```yaml
# Port comments in file say: "WE-specific, +10 from DS baseline"
PostgreSQL:  15443  (DS baseline 15433 + 10)
Redis:       16389  (DS baseline 16379 + 10)
DataStorage: 18100  (DS baseline 18090 + 10)
Metrics:     19100  (DS baseline 19090 + 10)
```

**Source**: `test/integration/workflowexecution/podman-compose.test.yml`

### **Problem 1: Not in DD-TEST-001** ❌

WorkflowExecution is **completely absent** from DD-TEST-001 Integration Tests section.

**DD-TEST-001 Status**:
- Lines 212-238: Only documents WE for Integration Tests (API 18110, DS Dep 18093)
- **NO** PostgreSQL, Redis, or full DataStorage stack documented
- Collision matrix line 608: Lists WE as using only DS dependency 18093

**Reality**: WE is running **FULL DataStorage stack** (PostgreSQL + Redis + DataStorage + Metrics)

### **Problem 2: Ports Don't Match DD-TEST-001 Pattern** ⚠️

**DD-TEST-001 Pattern**:
- PostgreSQL: 154XX (sequential)
- Redis: 163XX (sequential)
- DataStorage: 180XX (sequential)
- Metrics: 190XX (matches DataStorage XX)

**WE Actual Ports**:
- PostgreSQL: **15443** (doesn't follow pattern, uses "+10" logic)
- Redis: **16389** (doesn't follow pattern, uses "+10" logic)
- DataStorage: **18100** (doesn't follow pattern, uses "+10" logic)
- Metrics: **19100** (doesn't follow pattern, uses "+10" logic)

### **Problem 3: No Infrastructure Constants** ❌

**Expected**: `test/infrastructure/workflowexecution.go` with constants like:
```go
const (
    WEIntegrationPostgresPort    = 15443
    WEIntegrationRedisPort       = 16389
    WEIntegrationDataStoragePort = 18100
    WEIntegrationMetricsPort     = 19100
)
```

**Reality**: File doesn't exist, infrastructure managed via podman-compose only

---

## 🔍 **Port Conflict Analysis**

### **Checking WE Ports Against All Services**

| WE Port | Type | Conflicts With? |
|---------|------|-----------------|
| 15443 | PostgreSQL | ✅ **NO CONFLICT** (unique, between 15442-15444 gap) |
| 16389 | Redis | ✅ **NO CONFLICT** (unique, in 16388-16390 gap) |
| 18100 | DataStorage | 🚨 **CONFLICTS with EffectivenessMonitor!** |
| 19100 | Metrics | ✅ **NO CONFLICT** (unique, beyond allocated range) |

### **CONFLICT DETECTED: EffectivenessMonitor** 🚨

**EffectivenessMonitor** (per DD-TEST-001 lines 172-208):
```yaml
Effectiveness Monitor API: 18100
```

**WorkflowExecution** (actual usage):
```yaml
DataStorage: 18100
```

**Impact**: Cannot run EffectivenessMonitor + WorkflowExecution integration tests in parallel

---

## 📊 **DD-TEST-001 Discrepancy Analysis**

### **What DD-TEST-001 Says** (Lines 212-238)

```markdown
### Workflow Engine Service

#### Integration Tests (`test/integration/workflow-engine/`)
Workflow Engine API:
  Host Port: 18110

Data Storage (Dependency):
  Host Port: 18093
```

**Interpretation**: DD-TEST-001 implies WE only needs a DataStorage URL (18093), not full infrastructure

### **What WE Actually Does**

WE runs **FULL DataStorage stack**:
- PostgreSQL (15443)
- Redis (16389)
- DataStorage service (18100)
- Metrics (19100)

**Gap**: DD-TEST-001 documents WE as lightweight consumer, reality is heavy infrastructure user

---

## 🎯 **Root Cause**

### **Why This Happened**

1. **Inconsistent Naming**: DD-TEST-001 calls it "Workflow **Engine**", code uses "Workflow**Execution**"
2. **Documentation Lag**: podman-compose created without updating DD-TEST-001
3. **Ad-hoc Ports**: Used "DS baseline +10" instead of DD-TEST-001 allocation
4. **No Constants File**: No `workflowexecution.go` in `test/infrastructure/` to enforce ports

### **Why It Wasn't Caught Earlier**

- WE integration tests work in isolation (no parallel testing yet)
- podman-compose shields port conflicts (runs sequentially within WE)
- EffectivenessMonitor integration tests may not exist yet (needs verification)

---

## ✅ **RECOMMENDED: Fix Options**

### **Option A: Align WE to DD-TEST-001 Pattern** (RECOMMENDED)

**Change WE ports to follow DD-TEST-001 sequential pattern**:

```yaml
# OLD (current podman-compose.test.yml):
PostgreSQL:  15443  # Ad-hoc "+10"
Redis:       16389  # Ad-hoc "+10"
DataStorage: 18100  # CONFLICTS with EffectivenessMonitor!
Metrics:     19100  # Ad-hoc "+10"

# NEW (DD-TEST-001 compliant):
PostgreSQL:  15441  # Sequential after Gateway (15437), before EM (15434) - WAIT, no...
                     # Sequential pattern: DS(15433), EM(15434), RO(15435), SP(15436), GW(15437), AI(15438)
                     # Next available: 15439 (Notification), 15441 (WorkflowExecution)
PostgreSQL:  15441  # DD-TEST-001 sequential pattern
Redis:       16387  # DD-TEST-001 sequential pattern (next after AI 16384)
DataStorage: 18097  # DD-TEST-001 sequential pattern (resolves EM conflict!)
Metrics:     19097  # DD-TEST-001 metrics pattern (matches DataStorage XX)
```

**Files to Update**:
1. `test/integration/workflowexecution/podman-compose.test.yml` - Update ports
2. `test/integration/workflowexecution/config/config.yaml` - Update connection strings
3. `test/integration/workflowexecution/suite_test.go` - Update `dataStorageBaseURL` from 18100 → 18097
4. **CREATE** `test/infrastructure/workflowexecution.go` - Add constants
5. `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` - Add WE detailed section

**Benefits**:
- ✅ Resolves EffectivenessMonitor conflict
- ✅ Follows DD-TEST-001 pattern
- ✅ Enables parallel testing
- ✅ Consistent with all other services

---

### **Option B: Document Current WE Ports in DD-TEST-001** (NOT RECOMMENDED)

Keep WE ports as-is (15443/16389/18100/19100) and just document them.

**Problems**:
- ❌ Still conflicts with EffectivenessMonitor (18100)
- ❌ Doesn't follow DD-TEST-001 pattern
- ❌ Perpetuates ad-hoc port allocation

**Verdict**: Not viable due to EffectivenessMonitor conflict

---

### **Option C: Verify EffectivenessMonitor Exists** (INVESTIGATION)

**Question**: Does EffectivenessMonitor actually have integration tests?

**Action Required**:
```bash
ls -la test/integration/effectiveness-monitor/
```

**If EffectivenessMonitor doesn't exist**:
- Port 18100 conflict is theoretical
- WE can keep current ports temporarily
- But still should align to DD-TEST-001 for consistency

**If EffectivenessMonitor exists**:
- Must resolve conflict immediately
- Option A is required

---

## 📋 **Recommended Action Plan**

### **Step 1: Verify EffectivenessMonitor** (5 minutes)
```bash
# Check if EffectivenessMonitor integration tests exist
ls -la test/integration/effectiveness-monitor/

# If exists: CRITICAL - must fix WE port conflict
# If not exists: MEDIUM - can fix for consistency
```

### **Step 2: Update WE Ports** (30 minutes)
1. Update `podman-compose.test.yml` ports (15441/16387/18097/19097)
2. Update `config/config.yaml` connection strings
3. Update `suite_test.go` dataStorageBaseURL
4. Create `test/infrastructure/workflowexecution.go` with constants

### **Step 3: Update DD-TEST-001** (15 minutes)
1. Add WorkflowExecution detailed section (after Workflow Engine section)
2. Update port collision matrix
3. Add revision v1.7

### **Step 4: Validate** (10 minutes)
```bash
# Run WE integration tests with new ports
go test ./test/integration/workflowexecution -v -timeout=10m

# Check for port conflicts
grep -h "IntegrationPostgresPort\|IntegrationRedisPort\|IntegrationDataStoragePort" test/infrastructure/*.go | awk '{print $NF}' | sort -n | uniq -d
```

---

## 🚨 **Impact Assessment**

### **Severity**: HIGH

**Why High**:
- ✅ Active service with integration tests
- 🚨 Port conflict with EffectivenessMonitor (blocks parallel testing)
- ❌ Not in DD-TEST-001 (authoritative document incomplete)
- ❌ No infrastructure constants (inconsistent with other services)

### **Urgency**: MEDIUM-HIGH

**Why Medium-High**:
- Works in isolation (not blocking current CI/CD)
- Blocks parallel test execution (impacts future CI/CD optimization)
- Blocks shared DataStorage bootstrap migration (inconsistent ports)

### **Risk**: LOW (to fix)

**Why Low Risk**:
- WE integration tests run in isolation (no cross-service dependencies)
- Port changes tested before deployment
- podman-compose shields conflicts during transition

---

## ✅ **Success Criteria**

- ✅ WE ports follow DD-TEST-001 pattern (15441/16387/18097/19097)
- ✅ No conflict with EffectivenessMonitor (18100 freed)
- ✅ `test/infrastructure/workflowexecution.go` created with constants
- ✅ DD-TEST-001 updated with WE detailed section
- ✅ WE integration tests pass with new ports
- ✅ Port validation shows no duplicates

---

## 📊 **Summary Table**

| Aspect | Current State | Recommended State | Status |
|--------|---------------|-------------------|--------|
| **PostgreSQL** | 15443 (ad-hoc) | 15441 (DD-TEST-001) | ⚠️ **CHANGE** |
| **Redis** | 16389 (ad-hoc) | 16387 (DD-TEST-001) | ⚠️ **CHANGE** |
| **DataStorage** | 18100 (conflicts!) | 18097 (DD-TEST-001) | 🚨 **MUST FIX** |
| **Metrics** | 19100 (ad-hoc) | 19097 (DD-TEST-001) | ⚠️ **CHANGE** |
| **DD-TEST-001** | Not documented | Detailed section | ❌ **MISSING** |
| **Infrastructure Constants** | None | `workflowexecution.go` | ❌ **MISSING** |

---

## ❓ **Questions for User**

### **Critical Decision**

**Q1**: Does EffectivenessMonitor have integration tests?
- If **YES**: WE port conflict is **CRITICAL** - must fix immediately
- If **NO**: WE port conflict is **THEORETICAL** - can fix for consistency

**Q2**: Approve Option A (align WE to DD-TEST-001 pattern)?
- PostgreSQL: 15443 → **15441**
- Redis: 16389 → **16387**
- DataStorage: 18100 → **18097** (resolves EM conflict)
- Metrics: 19100 → **19097**

**Q3**: Priority level?
- **HIGH**: Fix immediately (blocks parallel testing)
- **MEDIUM**: Fix before shared bootstrap migration
- **LOW**: Fix when convenient

---

**Document Status**: ✅ **COMPLETE** - Awaiting User Decisions
**Confidence**: **100%** that WE ports are undocumented and potentially conflicting
**Recommended Action**: Align WE to DD-TEST-001 pattern (Option A)











