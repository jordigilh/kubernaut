# SHARED NOTIFICATION: HAPI-RO Integration Test Port Conflict

**Date**: 2025-12-24
**From**: RemediationOrchestrator Team
**To**: HolmesGPT-API (HAPI) Team
**Severity**: 🚨 **CRITICAL** - Blocking RO integration tests
**Authority**: DD-TEST-001-port-allocation-strategy.md

---

## 🚨 **Issue Summary**

**HAPI and RO integration tests are using IDENTICAL PORTS**, causing port conflicts that block RO test execution.

---

## 📊 **Port Conflict Matrix**

### **Current State (CONFLICT)**

| Service | PostgreSQL | Redis | DataStorage | Container Names |
|---------|------------|-------|-------------|-----------------|
| **HAPI Integration** | **15435** ❌ | **16381** ❌ | 18094 | kubernaut-hapi-postgres-integration<br>kubernaut-hapi-redis-integration |
| **RO Integration** | **15435** ❌ | **16381** ❌ | 18140 | ro-postgres-integration<br>ro-redis-integration |

**Result**: Port collision on 15435 (PostgreSQL) and 16381 (Redis)

---

## 🔍 **Evidence**

### **HAPI Integration Test Ports**

**File**: `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`

```yaml
postgres-integration:
  container_name: kubernaut-hapi-postgres-integration
  ports:
    - "15435:5432"  # Line 16: DD-TEST-001 comment says "uses 15435 (not 15433)"

redis-integration:
  container_name: kubernaut-hapi-redis-integration
  ports:
    - "16381:6379"  # Line 32: DD-TEST-001 comment says "uses 16381 (not 16379)"

data-storage-service:
  ports:
    - "18094:8080"  # Line 55: Correct, no conflict
```

---

### **RO Integration Test Ports**

**File**: `docs/handoff/RO_INTEGRATION_INFRASTRUCTURE_COMPLETE.md` (lines 24-30)

**Authority**: DD-TEST-001-port-allocation-strategy.md

```yaml
postgres:
  Port: 15435 (from DD-TEST-001 range 15433-15442)
  Container: ro-postgres-integration

redis:
  Port: 16381 (from DD-TEST-001 range 16379-16388)
  Container: ro-redis-integration

datastorage:
  Port: 18140 (after stateless services per DD-TEST-001)
  Container: ro-datastorage-integration
```

---

### **Actual Runtime Conflict**

**Command**: `podman ps -a | grep -E "15435|16381"`

**Output** (2025-12-24 14:43):
```
9d3d7312fed2  postgres:16-alpine   Up (healthy)   0.0.0.0:15435->5432/tcp   kubernaut-hapi-postgres-integration
51fba4668cf2  redis:7-alpine       Up (healthy)   0.0.0.0:16381->6379/tcp   kubernaut-hapi-redis-integration
```

**Error** when RO tries to start:
```
Error: cannot listen on the TCP port: listen tcp4 :15435: bind: address already in use
```

---

## 📋 **DD-TEST-001 Port Allocation Strategy**

### **Authoritative Document**

**File**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`

**Lines 39-40** (Port Range Blocks):
```
| PostgreSQL | 5432 | 15433-15442 | 25433-25442 | 15433-25442 |
| Redis      | 6379 | 16379-16388 | 26379-26388 | 16379-26388 |
```

**Allocation Rules** (Line 70-76):
- **Integration Tests**: 15433-18139 range (Podman containers)
- **Buffer**: 10 ports per service per tier (supports parallel processes + dependencies)

---

### **Service-Specific Port Allocations Found**

DD-TEST-001 shows these explicit assignments:

| Service | PostgreSQL | Redis | Source |
|---------|------------|-------|---------|
| **Data Storage** | 15433 | 16379 | DD-TEST-001 line 86-94 |
| **Gateway** | N/A | 16380 | DD-TEST-001 line 136-140 |
| **Effectiveness Monitor** | 15434 | N/A | DD-TEST-001 line 176-180 |
| **AIAnalysis** | 15434 | 16380 | `AIANALYSIS_INTEGRATION_INFRASTRUCTURE_SUMMARY.md` line 20 |
| **RemediationOrchestrator** | 15435 | 16381 | `RO_INTEGRATION_INFRASTRUCTURE_COMPLETE.md` line 65-66 |

**Note**: AIAnalysis also conflicts with EffectivenessMonitor (both use 15434) - pre-existing issue

---

## 🔍 **Root Cause Analysis**

### **Why HAPI Uses 15435/16381**

**Source**: `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`

**Lines 16, 32** show explicit comments:
```yaml
- "15435:5432"  # DD-TEST-001: HolmesGPT-API uses 15435 (not 15433)
- "16381:6379"  # DD-TEST-001: HolmesGPT-API uses 16381 (not 16379)
```

**Hypothesis**: HAPI documentation in `docs/handoff/HAPI_INTEGRATION_TEST_TRIAGE_DEC_23_2025.md` (lines 62-67) shows:
```
| PostgreSQL | 15435 | 15433 | Avoid conflict with DS tests |
| Redis      | 16381 | 16379 | Avoid conflict with DS tests |
```

**Problem**: HAPI chose 15435/16381 to avoid DS (15433/16379), but this conflicts with RO

---

### **Why RO Uses 15435/16381**

**Source**: `RO_INTEGRATION_INFRASTRUCTURE_COMPLETE.md` (2025-12-11)

**Lines 65-66**:
```
| **PostgreSQL** | 15435 | 5432 | ro-postgres-integration | Range 15433-15442 (Line 39) |
| **Redis**      | 16381 | 6379 | ro-redis-integration    | Range 16379-16388 (Line 40) |
```

**Rationale** (line 83):
> ✅ RO uses next sequential ports per DD-TEST-001 pattern

**Sequential Allocation Logic**:
- DS uses 15433 → HAPI skips to 15435 → RO assumes 15435 is next available
- DS uses 16379 → Gateway uses 16380 → HAPI skips to 16381 → RO assumes 16381 is next

---

## 📅 **Timeline**

| Date | Event | Team |
|------|-------|------|
| Unknown | HAPI integration tests created with ports 15435/16381 | HAPI |
| 2025-12-11 | RO integration infrastructure created with ports 15435/16381 | RO |
| 2025-12-23 | HAPI integration test infrastructure documented | HAPI |
| 2025-12-24 | Port conflict discovered - RO tests fail to start | RO |

---

## ✅ **Proposed Resolution**

### **Option A: HAPI Changes Ports** (RECOMMENDED)

**Rationale**: HAPI integration tests use **docker-compose** (not podman-compose), which is **NON-STANDARD** per DD-TEST-002.

**HAPI Should Use**:
```yaml
PostgreSQL: 15439  # Next available after RO (15435)
Redis:      16387  # Next available after RO (16381)
DataStorage: 18094 # Already correct, no change
```

**Changes Required**:
1. Update `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`
2. Update `holmesgpt-api/tests/integration/data-storage-integration.yaml` (database connection)
3. Update any test code referencing ports

**Impact**: Low - HAPI team changes 2 lines in compose file + config

---

### **Option B: RO Changes Ports** (NOT RECOMMENDED)

**Rationale**: RO infrastructure follows DD-TEST-002 (programmatic podman-compose), which is the **STANDARD PATTERN**.

**RO Would Use**:
```yaml
PostgreSQL: 15436  # After HAPI (15435)
Redis:      16382  # After HAPI (16381)
DataStorage: 18140 # Already unique, no change
```

**Changes Required**:
1. Update `test/infrastructure/remediationorchestrator.go` (port constants)
2. Update `RO_INTEGRATION_INFRASTRUCTURE_COMPLETE.md` (documentation)
3. Update `test/integration/remediationorchestrator/suite_test.go` (test setup)

**Impact**: Medium - RO team changes 3+ files + re-documents

---

### **Option C: Both Teams Use Different Ports**

**Comprehensive Port Allocation**:

| Service | PostgreSQL | Redis | DataStorage | Rationale |
|---------|------------|-------|-------------|-----------|
| **Data Storage** | 15433 | 16379 | 18090-18099 | First in range |
| **Effectiveness Monitor** | 15434 | 16382 | 18100-18109 | Sequential |
| **AIAnalysis** | 15437 | 16383 | 18091, 18120 | Resolve conflict with EM |
| **RemediationOrchestrator** | 15435 | 16381 | 18140-18141 | Keep current (standard pattern) |
| **HAPI Integration** | 15439 | 16387 | 18094 | Move to avoid conflicts |
| **Gateway** | N/A | 16380 | 18080-18089 | Keep current |

**Impact**: Medium-High - Multiple teams update ports

---

## 🎯 **Recommended Action**

### **For HAPI Team** (IMMEDIATE)

✅ **Option A: Change HAPI ports to 15439/16387**

**Justification**:
1. ✅ HAPI uses non-standard docker-compose (DD-TEST-002 violation)
2. ✅ RO uses standard programmatic pattern (DD-TEST-002 compliant)
3. ✅ RO infrastructure created first (2025-12-11)
4. ✅ HAPI port change is minimal (2 lines + config)
5. ✅ Resolves conflict without cascading changes

**Files to Update**:
1. `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`
   - Line 16: `"15439:5432"` (was 15435)
   - Line 32: `"16387:6379"` (was 16381)

2. `holmesgpt-api/tests/integration/data-storage-integration.yaml`
   - Update PostgreSQL connection (port 15439)
   - Update Redis connection (port 16387)

3. `docs/handoff/HAPI_INTEGRATION_TEST_TRIAGE_DEC_23_2025.md`
   - Update port documentation

---

## 📊 **Updated Port Allocation (After Fix)**

### **Conflict-Free Matrix**

| Service | PostgreSQL | Redis | DataStorage | Container Prefix | Status |
|---------|------------|-------|-------------|------------------|---------|
| **Data Storage** | 15433 | 16379 | 18090-18099 | ds-integration | ✅ No conflict |
| **Gateway** | N/A | 16380 | 18080-18089 | gw-integration | ✅ No conflict |
| **Effectiveness Monitor** | 15434 | 16382 | 18100-18109 | em-integration | ⚠️ Conflicts with AA (15434) |
| **AIAnalysis** | 15437 | 16383 | 18091, 18120 | aa-integration | ✅ After resolution |
| **RemediationOrchestrator** | 15435 | 16381 | 18140-18141 | ro-integration | ✅ No conflict |
| **HAPI Integration** | **15439** ✅ | **16387** ✅ | 18094 | kubernaut-hapi-integration | ✅ After fix |

---

## 🚀 **Verification Steps**

### **For HAPI Team (After Changes)**

```bash
# Stop old containers
podman stop kubernaut-hapi-postgres-integration kubernaut-hapi-redis-integration
podman rm -f kubernaut-hapi-postgres-integration kubernaut-hapi-redis-integration

# Start with new ports
cd holmesgpt-api/tests/integration
docker-compose -f docker-compose.workflow-catalog.yml up -d

# Verify new ports
docker ps | grep hapi
# Expected: 15439:5432 and 16387:6379
```

### **For RO Team (Verification)**

```bash
# Clean any stale RO containers
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Run RO integration tests (should now work)
make test-integration-remediationorchestrator
```

---

## 📋 **Success Criteria**

### **HAPI Team**
- [ ] Update docker-compose.workflow-catalog.yml (ports 15439, 16387)
- [ ] Update data-storage-integration.yaml (connection strings)
- [ ] Run HAPI workflow catalog integration tests → PASS
- [ ] Verify no port conflicts (`podman ps -a`)

### **RO Team**
- [ ] Verify HAPI containers stopped/removed
- [ ] Run RO integration tests → infrastructure starts successfully
- [ ] Verify no port conflicts during test execution
- [ ] CF-INT-1 validation completes

### **Both Teams**
- [ ] Can run tests in parallel without conflicts
- [ ] Documentation updated (DD-TEST-001 clarification if needed)

---

## 🔗 **Related Documentation**

| Document | Purpose | Authority |
|----------|---------|-----------|
| **DD-TEST-001** | Port allocation strategy | ✅ AUTHORITATIVE |
| **DD-TEST-002** | Integration test container orchestration | ✅ AUTHORITATIVE |
| **RO_INTEGRATION_INFRASTRUCTURE_COMPLETE.md** | RO port documentation | ✅ Complete |
| **HAPI_INTEGRATION_TEST_TRIAGE_DEC_23_2025.md** | HAPI port documentation | ⚠️ Needs update |
| **RO_SESSION_FINAL_SUMMARY_DEC_24_2025.md** | Context for this issue | ✅ Complete |

---

## 📝 **Summary**

**Issue**: HAPI and RO integration tests use identical ports (15435 PostgreSQL, 16381 Redis)
**Impact**: RO integration tests cannot start (port conflict error)
**Root Cause**: Independent port selection without coordination
**Solution**: HAPI changes to ports 15439 (PostgreSQL) and 16387 (Redis)
**Justification**: HAPI uses non-standard docker-compose, RO uses standard pattern
**Effort**: Low (2 lines in compose file + config update)

---

**Status**: 🚨 **AWAITING HAPI TEAM ACTION**
**Priority**: HIGH - Blocks RO integration test validation
**Estimated Fix Time**: 15 minutes
**Coordination**: None required after HAPI fix

---

**Created**: 2025-12-24 15:00
**From**: RemediationOrchestrator Team
**To**: HolmesGPT-API Team
**Next**: HAPI team updates ports, both teams verify parallel execution

---

## ✅ **HAPI TEAM RESPONSE** (2025-12-24 23:00 UTC)

**Status**: ✅ **RESOLVED** - Port conflict fixed

### **Actions Taken**

HAPI team has completed the recommended solution (Option 1: HAPI Changes Ports):

1. ✅ Updated `docker-compose.workflow-catalog.yml`:
   - PostgreSQL: 15435 → **15439**
   - Redis: 16381 → **16387**

2. ✅ Updated `conftest.py` port constants:
   - `POSTGRES_PORT = "15439"`
   - `REDIS_PORT = "16387"`

3. ✅ Updated `data-storage-integration.yaml` connection strings

4. ✅ Removed deprecated embedding service references

5. ✅ Enforced Podman-only container runtime

### **Verification**

```bash
$ podman ps --filter "name=hapi"
kubernaut-hapi-postgres-integration      0.0.0.0:15439->5432/tcp  ✅
kubernaut-hapi-redis-integration         0.0.0.0:16387->6379/tcp  ✅
kubernaut-hapi-data-storage-integration  0.0.0.0:18094->8080/tcp  ✅
```

**Result**: Ports 15435/16381 are now FREE for RO integration tests

### **Documentation**

- Created: `HAPI_PORT_CONFLICT_RESOLVED_DEC_24_2025.md`
- Details complete port change implementation and verification

### **RO Team: Ready to Proceed**

✅ RO can now run integration tests without port conflicts
✅ Both teams can run integration tests in parallel
✅ No coordination needed for test execution

---

## 🚨 **UPDATE: SignalProcessing Port Conflict (2025-12-25 11:00 EST)**

**Status**: ✅ **RESOLVED**

### **New Issue Discovered**

SignalProcessing team reported that HAPI was using port **18094**, which is officially allocated to SignalProcessing per DD-TEST-001 v1.4.

**Evidence**:
```
Container: kubernaut-hapi-data-storage-integration
Port Binding: 0.0.0.0:18094->8080/tcp
Error: listen tcp4 :18094: bind: address already in use
```

### **Resolution**

HAPI team migrated Data Storage dependency port from **18094** to **18098**:

**Files Updated**:
- `holmesgpt-api/tests/integration/conftest.py`
- `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`
- `holmesgpt-api/tests/integration/setup_workflow_catalog_integration.sh`
- `holmesgpt-api/tests/integration/bootstrap-workflows.sh`

**New HAPI Port Allocation** (DD-TEST-001 v1.8):
- PostgreSQL: **15439** (shared with Notification/WE)
- Redis: **16387** (shared with Notification/WE)
- Data Storage: **18098** (CHANGED from 18094)
- HAPI API: **18120**

**Verification**:
```bash
$ podman ps --filter "name=kubernaut-hapi"
kubernaut-hapi-data-storage-integration  0.0.0.0:18098->8080/tcp  Up (healthy) ✅
```

**Test Results**: 27/27 integration tests passing (zero impact)

**Detailed Resolution**: See `docs/handoff/HAPI_PORT_MIGRATION_18094_TO_18098_DEC_25_2025.md`

**Conclusion**: HAPI, RO, and SignalProcessing integration tests can now all run in parallel without port conflicts.

---

**Document Updated**: 2025-12-25 11:00 EST
**Final Status**: ✅ **RESOLVED** - HAPI ports changed (twice), RO and SP unblocked


