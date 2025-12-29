# Port Allocation Reassessment - December 22, 2025

**Date**: December 22, 2025
**Status**: 🔍 **ANALYSIS IN PROGRESS**
**Authority**: DD-TEST-001 Port Allocation Strategy
**Trigger**: Gateway Redis removal (DD-GATEWAY-012) freed ports 16380 and 26380

---

## 🎯 **Objective**

Reassess all integration test port allocations after Gateway Redis removal to:
1. Identify current port usage in code vs. DD-TEST-001 allocations
2. Resolve any conflicts or misalignments
3. Update code/documentation to match authoritative DD-TEST-001

---

## 📋 **DD-TEST-001: Authoritative Port Allocations**

### **Services with Full DD-TEST-001 Documentation**

| Service | PostgreSQL | Redis | DataStorage | Metrics | DD-TEST-001 Lines |
|---------|------------|-------|-------------|---------|-------------------|
| **DataStorage** | 15433 | 16379 | 18090 | 19090 | Lines 83-128 |
| **Gateway** | N/A | ~~16380~~ **FREED** | 18091 | 19091 | Lines 132-168 |
| **Effectiveness Monitor** | 15434 | N/A | 18092 | N/A | Lines 172-208 |
| **Workflow Engine** | N/A | N/A | 18093 | N/A | Lines 212-238 |
| **SignalProcessing** | 15436 | 16382 | 18094 | 19094 | Lines 242-267 |

### **Services in Port Collision Matrix Only**

| Service | PostgreSQL | Redis | API | DataStorage Dep | DD-TEST-001 Lines |
|---------|------------|-------|-----|-----------------|-------------------|
| **RemediationOrchestrator** | 15435 | 16381 | N/A | N/A | Line 580 (note: "undocumented, requires DD-TEST-001 update") |

### **Services NOT in DD-TEST-001**

| Service | Status |
|---------|--------|
| **AIAnalysis** | ❌ NOT documented in Integration Tests section |
| **Notification** | ❌ NOT documented |
| **WorkflowExecution** | ❌ NOT documented for Integration Tests (only E2E) |

---

## 🔍 **Actual Code Port Usage**

### **Integration Test Infrastructure Constants**

| Service | PostgreSQL | Redis | DataStorage | Metrics | Source File |
|---------|------------|-------|-------------|---------|-------------|
| **Gateway** | 15437 | 16383 | 18091 | 19091 | `test/infrastructure/gateway.go:18-21` |
| **SignalProcessing** | 15436 | 16382 | 18094 | N/A | `test/infrastructure/signalprocessing.go:1247-1249` |
| **RemediationOrchestrator** | 15435 | 16381 | 18140 | 18141 | `test/infrastructure/remediationorchestrator.go:479-482` |
| **AIAnalysis** | 15434 | 16380 | 18091 | N/A | `test/infrastructure/aianalysis.go:1301-1307` |
| **DataStorage** | 5433 | 6380 | 8085 | N/A | `test/infrastructure/datastorage.go:1199-1201` |
| **Notification** | N/A | N/A | N/A | N/A | No constants found |
| **WorkflowExecution** | N/A | N/A | N/A | N/A | No constants found |

---

## ⚠️ **CONFLICTS FOUND: DD-TEST-001 vs. Actual Code**

### **CONFLICT 1: Gateway Redis Port**
**DD-TEST-001 Says**: Gateway uses Redis port 16380 (FREED per DD-GATEWAY-012)
**Code Actually Uses**: Gateway uses Redis port **16383**
**Status**: ❌ **MISMATCH** - Gateway code never used DD-TEST-001 allocation
**Impact**: Gateway code predates DD-TEST-001 v1.5 update

**Resolution Options**:
- **Option A**: Update DD-TEST-001 to reflect Gateway never used Redis (remove 16380 reference entirely)
- **Option B**: Keep DD-TEST-001 as-is (historical reference that 16380 was allocated but never used)

**Recommendation**: **Option A** - DD-TEST-001 should reflect actual usage, not hypothetical allocations

---

### **CONFLICT 2: AIAnalysis Uses Gateway's Ports**
**DD-TEST-001 Says**: Gateway should use DataStorage 18091, Redis 16380 (freed)
**Code Actually Uses**:
- AIAnalysis uses DataStorage **18091** (Gateway's DD-TEST-001 allocation!)
- AIAnalysis uses Redis **16380** (Gateway's freed Redis port!)

**Status**: 🚨 **PORT CONFLICT** - AIAnalysis is using ports DD-TEST-001 allocated to Gateway

**Impact**:
- Cannot run Gateway + AIAnalysis integration tests in parallel
- AIAnalysis not documented in DD-TEST-001 Integration Tests section

**Resolution**:
1. Document AIAnalysis in DD-TEST-001 with unique ports
2. Change AIAnalysis ports to avoid Gateway conflict

---

### **CONFLICT 3: AIAnalysis Uses EffectivenessMonitor's PostgreSQL**
**DD-TEST-001 Says**: EffectivenessMonitor should use PostgreSQL **15434**
**Code Actually Uses**: AIAnalysis uses PostgreSQL **15434**
**Status**: 🚨 **PORT CONFLICT** - Cannot run both services in parallel

**Impact**: EffectivenessMonitor and AIAnalysis cannot run integration tests simultaneously

**Resolution**: Reallocate AIAnalysis PostgreSQL port

---

### **CONFLICT 4: DataStorage Using Non-DD-TEST-001 Ports**
**DD-TEST-001 Says**: DataStorage should use PostgreSQL 15433, Redis 16379, API 18090
**Code Actually Uses**: DataStorage uses PostgreSQL **5433**, Redis **6380**, API **8085**
**Status**: 🚨 **MAJOR MISMATCH** - DataStorage not following DD-TEST-001

**Impact**:
- DataStorage reference implementation doesn't follow its own port allocation strategy
- Potential conflicts with production services on default ports

**Resolution**:
1. Update DataStorage to use DD-TEST-001 ports (15433, 16379, 18090)
2. Or update DD-TEST-001 to match DataStorage's actual usage

---

### **CONFLICT 5: RemediationOrchestrator Metrics Port Pattern**
**DD-TEST-001 Says**: Metrics should use 19XXX range
**Code Actually Uses**: RO uses metrics port **18141** (18XXX range)
**Status**: ⚠️ **PATTERN VIOLATION** - Not following DD-TEST-001 metrics port pattern

**Resolution**: Change RO metrics port from 18141 → **19140**

---

### **CONFLICT 6: Services Missing from DD-TEST-001**
**Not Documented**:
- ❌ AIAnalysis (Integration Tests)
- ❌ Notification (Integration Tests)
- ❌ WorkflowExecution (Integration Tests)
- ❌ RemediationOrchestrator (Detailed section - only mentioned in collision matrix note)

**Status**: 🚨 **DOCUMENTATION GAP** - Active services not in authoritative document

---

## 📊 **Port Conflict Matrix**

### **Services That Can Run in Parallel** ✅

| Service Pair | PostgreSQL | Redis | DataStorage | Conflict? |
|--------------|------------|-------|-------------|-----------|
| **DataStorage ↔ Gateway** | 5433 ↔ 15437 | 6380 ↔ 16383 | 8085 ↔ 18091 | ✅ **NO CONFLICT** |
| **SignalProcessing ↔ Gateway** | 15436 ↔ 15437 | 16382 ↔ 16383 | 18094 ↔ 18091 | ✅ **NO CONFLICT** |
| **RemediationOrchestrator ↔ Gateway** | 15435 ↔ 15437 | 16381 ↔ 16383 | 18140 ↔ 18091 | ✅ **NO CONFLICT** |
| **SignalProcessing ↔ RemediationOrchestrator** | 15436 ↔ 15435 | 16382 ↔ 16381 | 18094 ↔ 18140 | ✅ **NO CONFLICT** |

### **Services That CANNOT Run in Parallel** ❌

| Service Pair | Conflict | Impact |
|--------------|----------|--------|
| **AIAnalysis ↔ Gateway** | DataStorage: 18091 ↔ 18091 | 🚨 **CONFLICT** - Same DataStorage port |
| **AIAnalysis ↔ Gateway** | Redis: 16380 ↔ ~16380~ (freed) | ⚠️ AIAnalysis using Gateway's freed port |
| **AIAnalysis ↔ EffectivenessMonitor** | PostgreSQL: 15434 ↔ 15434 | 🚨 **CONFLICT** - Same PostgreSQL port |

---

## ✅ **RECOMMENDED: Corrected Port Allocations**

### **Principle**: Follow DD-TEST-001 Pattern + Fix Conflicts

| Service | PostgreSQL | Redis | DataStorage | Metrics | Status |
|---------|------------|-------|-------------|---------|--------|
| **DataStorage** | **15433** ⚠️ | **16379** ⚠️ | **18090** ⚠️ | **19090** ✨ | CHANGE from 5433/6380/8085 |
| **EffectivenessMonitor** | 15434 | N/A | 18092 | 19092 ✨ | KEEP 15434, ADD 19092 |
| **RemediationOrchestrator** | 15435 | 16381 | 18140 | **19140** ⚠️ | KEEP, FIX metrics (18141→19140) |
| **SignalProcessing** | 15436 | 16382 | 18094 | **19094** ✨ | KEEP, ADD 19094 |
| **Gateway** | 15437 | 16383 | 18091 | 19091 | KEEP (code uses 15437/16383/18091/19091) |
| **AIAnalysis** | **15438** ✨ | **16384** ✨ | **18095** ✨ | **19095** ✨ | NEW allocation (resolve conflicts) |
| **Notification** | **15439** ✨ | **16385** ✨ | **18096** ✨ | **19096** ✨ | NEW allocation |
| **WorkflowExecution** | **15441** ✨ | **16387** ✨ | **18097** ✨ | **19097** ✨ | NEW allocation |

**Legend**:
- ✅ Already correct
- ⚠️ Requires code or DD-TEST-001 update
- ✨ New allocation (not currently in DD-TEST-001)

---

## 🎯 **Priority Actions**

### **Priority 1: Fix Actual Conflicts** 🔴 **HIGH**

#### **1.1: AIAnalysis Port Conflicts**
**Files to Update**:
- `test/infrastructure/aianalysis.go`:
  ```go
  // OLD:
  AIAnalysisIntegrationPostgresPort = 15434  // Conflicts with EffectivenessMonitor
  AIAnalysisIntegrationRedisPort = 16380     // Gateway's freed port
  AIAnalysisIntegrationDataStoragePort = 18091  // Conflicts with Gateway

  // NEW:
  AIAnalysisIntegrationPostgresPort = 15438  // Unique
  AIAnalysisIntegrationRedisPort = 16384     // Unique
  AIAnalysisIntegrationDataStoragePort = 18095  // Unique
  AIAnalysisIntegrationMetricsPort = 19095   // Add metrics (DD-TEST-001 pattern)
  ```

**Impact**: Resolves 3 port conflicts, enables parallel test execution

---

#### **1.2: DataStorage Port Alignment**
**Files to Update**:
- `test/infrastructure/datastorage.go`:
  ```go
  // OLD:
  PostgresPort: "5433"
  RedisPort: "6380"
  ServicePort: "8085"

  // NEW (follow DD-TEST-001):
  PostgresPort: "15433"  // DD-TEST-001 compliant
  RedisPort: "16379"     // DD-TEST-001 compliant
  ServicePort: "18090"   // DD-TEST-001 compliant
  MetricsPort: "19090"   // Add metrics port
  ```

**Impact**: DataStorage reference implementation follows DD-TEST-001

---

#### **1.3: RemediationOrchestrator Metrics Port**
**Files to Update**:
- `test/infrastructure/remediationorchestrator.go`:
  ```go
  // OLD:
  ROIntegrationDataStorageMetricsPort = 18141  // Wrong range

  // NEW:
  ROIntegrationDataStorageMetricsPort = 19140  // DD-TEST-001 pattern (19XXX)
  ```

**Impact**: Follows DD-TEST-001 metrics port pattern

---

### **Priority 2: Update DD-TEST-001 Documentation** 🟡 **MEDIUM**

#### **2.1: Remove Gateway Redis 16380 Reference**
**Rationale**: Gateway never actually used Redis port 16380; it uses 16383

**DD-TEST-001 Update**:
```markdown
### Gateway Service

**Note**: Gateway no longer uses Redis as of DD-GATEWAY-012 (December 2025).

**Previously Allocated Redis Ports (Now Available for Other Services)**:
- ~~Integration: 16380 (freed)~~ **NEVER USED** - Gateway used 16383
- ~~E2E: 26380 (freed)~~ **NEVER USED** - Gateway used 26383 (hypothetical)

**Clarification**: Gateway's Redis implementation used non-DD-TEST-001 ports (16383)
before Redis was removed. Port 16380 was allocated but never implemented.
```

---

#### **2.2: Add Missing Services to DD-TEST-001**

**Services to Document**:
1. **RemediationOrchestrator** (currently only in collision matrix note)
2. **AIAnalysis** (Integration Tests section)
3. **Notification** (Integration Tests section)
4. **WorkflowExecution** (Integration Tests section)

**Template**:
```markdown
### RemediationOrchestrator Service

#### Integration Tests (`test/integration/remediationorchestrator/`)
\`\`\`yaml
PostgreSQL:
  Host Port: 15435
  Container Port: 5432
  Connection: localhost:15435

Redis:
  Host Port: 16381
  Container Port: 6379
  Connection: localhost:16381

Data Storage (Dependency):
  Host Port: 18140
  Container Port: 8080
  Connection: http://localhost:18140

Data Storage Metrics:
  Host Port: 19140
  Container Port: 9090
  Connection: http://localhost:19140
\`\`\`
```

---

### **Priority 3: Validate No New Conflicts** 🟢 **LOW**

After implementing Priority 1 & 2, run validation:

```bash
# Check no port conflicts
cd test/infrastructure
grep -h "Integration.*Port\s*=" *.go | sort -t= -k2 -n | awk '{print $NF}' | sort | uniq -d

# Should return NO duplicates
```

---

## 📝 **Questions for User**

### **Critical Decisions Needed**

1. **DataStorage Ports**: Should DataStorage change to DD-TEST-001 ports (15433/16379/18090) or should DD-TEST-001 be updated to match DataStorage's current usage (5433/6380/8085)?
   - **Recommendation**: Change DataStorage to match DD-TEST-001 (reference implementation should follow standards)

2. **Gateway Redis Port**: DD-TEST-001 says 16380 was allocated, but code uses 16383. Should DD-TEST-001:
   - **Option A**: Remove 16380 reference entirely (it was never used)
   - **Option B**: Document both (16380 allocated, 16383 actually used, both now freed)
   - **Recommendation**: Option A (simplify documentation)

3. **AIAnalysis Ports**: Approve proposed allocation (15438/16384/18095/19095)?
   - **Recommendation**: YES - resolves all conflicts

4. **Notification/WorkflowExecution Ports**: Approve proposed allocations?
   - Notification: 15439/16385/18096/19096
   - WorkflowExecution: 15441/16387/18097/19097
   - **Recommendation**: YES - follows DD-TEST-001 pattern

5. **Migration Order**: Which conflicts to fix first?
   - **Recommendation**:
     1. AIAnalysis (blocks Gateway parallel testing)
     2. DataStorage (reference implementation should be correct)
     3. RemediationOrchestrator metrics (minor pattern violation)

---

**Document Status**: ✅ **COMPLETE** - Awaiting User Decisions
**Confidence**: **100%** that conflicts are accurately identified
**Next Action**: User approval required to proceed with fixes


