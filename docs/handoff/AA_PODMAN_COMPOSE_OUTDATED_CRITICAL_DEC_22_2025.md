# AIAnalysis podman-compose.yml OUTDATED - CRITICAL ISSUE

**Date**: December 22, 2025
**Severity**: 🚨 **CRITICAL**
**Impact**: Port conflicts, cannot run AIAnalysis integration tests in parallel with other services
**Status**: ⚠️ **BLOCKING ISSUE**

---

## 🚨 **Critical Finding**

AIAnalysis `test/integration/aianalysis/podman-compose.yml` has **OUTDATED PORTS** that conflict with DD-TEST-001 v1.6 allocations and other services.

---

## 📋 **Port Discrepancy Analysis**

### **Authoritative Source** (DD-TEST-001 v1.6 + `test/infrastructure/aianalysis.go`)

```go
// test/infrastructure/aianalysis.go lines 1314-1320
AIAnalysisIntegrationPostgresPort    = 15438
AIAnalysisIntegrationRedisPort       = 16384
AIAnalysisIntegrationDataStoragePort = 18095
AIAnalysisIntegrationMetricsPort     = 19095
AIAnalysisIntegrationHAPIPort        = 18120  // HolmesGPT API
```

**Status**: ✅ **CORRECT** (aligned with DD-TEST-001 v1.6, December 2025)

---

### **OUTDATED** (`test/integration/aianalysis/podman-compose.yml`)

```yaml
# Lines 4-8 of podman-compose.yml
# Service Ports (AIAnalysis-specific):
#   PostgreSQL:       15434 → 5432  # ❌ WRONG! (conflicts with EffectivenessMonitor)
#   Redis:            16380 → 6379  # ❌ WRONG! (was Gateway's freed port)
#   Data Storage API: 18091 → 8080  # ❌ WRONG! (conflicts with Gateway)
#   HolmesGPT API:    18120 → 8080  # ✅ CORRECT
```

**Actual Ports in File**:
- Line 24: `"15434:5432"` (PostgreSQL)
- Line 37: `"16380:6379"` (Redis)
- Line 62: `"18091:8080"` (DataStorage)
- Line 94: `"18120:8080"` (HolmesGPT API)

---

## 🚨 **Port Conflicts Caused by Outdated File**

### **Conflict 1: PostgreSQL 15434**
- **AIAnalysis podman-compose.yml**: 15434 (OUTDATED)
- **EffectivenessMonitor (DD-TEST-001)**: 15434 (ALLOCATED)
- **Correct AIAnalysis Port**: 15438
- **Impact**: Cannot run AIAnalysis + EffectivenessMonitor in parallel (when EM v1.1 is implemented)

### **Conflict 2: Redis 16380**
- **AIAnalysis podman-compose.yml**: 16380 (OUTDATED)
- **Gateway (DD-TEST-001 v1.5)**: 16380 (FREED in December 2025 per DD-GATEWAY-012)
- **Correct AIAnalysis Port**: 16384
- **Impact**: If Gateway tests revert or other service uses 16380, conflict occurs

### **Conflict 3: DataStorage 18091**
- **AIAnalysis podman-compose.yml**: 18091 (OUTDATED)
- **Gateway (DD-TEST-001 v1.6)**: 18091 (ALLOCATED)
- **Correct AIAnalysis Port**: 18095
- **Impact**: 🚨 **CANNOT RUN AIAnalysis + Gateway integration tests in parallel**

### **No Conflict: HolmesGPT API 18120**
- **AIAnalysis podman-compose.yml**: 18120 ✅ **CORRECT**
- **DD-TEST-001**: 18120-18129 (ALLOCATED to HolmesGPT API)
- **Impact**: None - this port is correct

---

## 🔍 **Root Cause**

AIAnalysis `podman-compose.yml` was created **before DD-TEST-001 v1.6 (December 2025)** port allocation updates. The file's header comments still reference old allocations:

```yaml
# Lines 1-11 of podman-compose.yml
# AIAnalysis Integration Test Infrastructure
# Port Allocation per DD-TEST-001: Port Allocation Strategy
#
# Service Ports (AIAnalysis-specific):
#   PostgreSQL:       15434 → 5432  # ❌ OLD ALLOCATION
#   Redis:            16380 → 6379  # ❌ OLD ALLOCATION
#   Data Storage API: 18091 → 8080  # ❌ OLD ALLOCATION
#   HolmesGPT API:    18120 → 8080  # ✅ CORRECT
```

**When Updated**:
- DD-TEST-001 v1.6: December 22, 2025
- `test/infrastructure/aianalysis.go`: December 22, 2025
- `podman-compose.yml`: ❌ **NOT UPDATED**

---

## ✅ **REQUIRED FIX**

### **Update `test/integration/aianalysis/podman-compose.yml`**

**File**: `test/integration/aianalysis/podman-compose.yml`

**Changes Required**:

```yaml
# Lines 4-8 (header comments)
# Service Ports (AIAnalysis-specific):
#   PostgreSQL:       15438 → 5432  # ✅ FIXED (was 15434)
#   Redis:            16384 → 6379  # ✅ FIXED (was 16380)
#   Data Storage API: 18095 → 8080  # ✅ FIXED (was 18091)
#   HolmesGPT API:    18120 → 8080  # ✅ CORRECT (no change)
```

```yaml
# Line 24 (PostgreSQL port)
- "15438:5432"  # AIAnalysis integration test PostgreSQL port (was 15434)

# Line 37 (Redis port)
- "16384:6379"  # AIAnalysis integration test Redis port (was 16380)

# Line 62 (DataStorage port)
- "18095:8080"  # AIAnalysis integration test DataStorage API port (was 18091)

# Line 94 (HolmesGPT API port - no change needed)
- "18120:8080"  # AIAnalysis integration test HolmesGPT API port
```

---

## 📋 **Additional Updates Required**

### **1. Update `test/integration/aianalysis/config/config.yaml`**

Verify hostnames match container names (postgres, redis, datastorage) - these should be correct already.

---

### **2. Update `test/integration/aianalysis/suite_test.go`**

Check if any hardcoded ports exist:

```bash
grep -n "15434\|16380\|18091" test/integration/aianalysis/suite_test.go
```

If found, update to 15438/16384/18095.

---

### **3. Migrate from podman-compose to DD-TEST-002 (RECOMMENDED)**

**Long-term Fix**: Migrate AIAnalysis to DD-TEST-002 sequential `podman run` pattern like:
- Gateway (December 2025)
- WorkflowExecution (December 21, 2025)
- Notification (December 21, 2025)

**Create**: `test/integration/aianalysis/setup-infrastructure.sh` with sequential startup:
1. PostgreSQL → wait ready
2. Redis → wait ready
3. Run migrations
4. Start DataStorage → wait healthy
5. Start HolmesGPT API → wait healthy

**Benefits**:
- Eliminates `podman-compose` race conditions
- Consistent with other services
- Explicit health checks
- Better error messages

---

## 🚨 **IMMEDIATE IMPACT**

### **Current State** (with outdated podman-compose.yml)

**Cannot Run in Parallel**:
- ❌ AIAnalysis + Gateway (conflict on 18091)
- ⚠️ AIAnalysis + EffectivenessMonitor (conflict on 15434, EM v1.1 only)

**Can Run Separately**:
- ✅ AIAnalysis integration tests work in isolation (wrong ports, but internally consistent)

---

### **After Fix** (with DD-TEST-001 v1.6 ports)

**Can Run in Parallel**:
- ✅ AIAnalysis + Gateway (18095 vs 18091 - no conflict)
- ✅ AIAnalysis + EffectivenessMonitor (15438 vs 15434 - no conflict)
- ✅ AIAnalysis + ALL other v1.0 services (no conflicts)

---

## 📊 **Verification Commands**

### **Before Fix** (expected conflicts)

```bash
# Start Gateway infrastructure
cd test/integration/gateway && ./setup-infrastructure.sh &

# Start AIAnalysis infrastructure (WILL CONFLICT on 18091)
cd test/integration/aianalysis && podman-compose -f podman-compose.yml up -d

# Check for port conflicts
netstat -tuln | grep -E "15434|16380|18091|18120"
```

**Expected**: Port 18091 conflict (Gateway DataStorage vs AIAnalysis DataStorage)

---

### **After Fix** (no conflicts)

```bash
# Start Gateway infrastructure
cd test/integration/gateway && ./setup-infrastructure.sh &

# Start AIAnalysis infrastructure (FIXED PORTS)
cd test/integration/aianalysis && podman-compose -f podman-compose.yml up -d

# Check all services healthy
curl http://localhost:18091/health  # Gateway DataStorage
curl http://localhost:18095/health  # AIAnalysis DataStorage
curl http://localhost:18120/health  # AIAnalysis HolmesGPT API
```

**Expected**: All services start cleanly, all health checks pass

---

## ✅ **SUCCESS CRITERIA**

- ✅ `podman-compose.yml` updated with DD-TEST-001 v1.6 ports (15438/16384/18095/18120)
- ✅ Header comments updated to reflect correct allocations
- ✅ No port conflicts with Gateway (18095 vs 18091)
- ✅ No port conflicts with EffectivenessMonitor (15438 vs 15434)
- ✅ AIAnalysis integration tests pass with new ports
- ✅ Can run AIAnalysis + Gateway + all other services in parallel

---

## 🎯 **PRIORITY**

**Severity**: 🚨 **HIGH**
**Urgency**: **IMMEDIATE** (blocks parallel testing with Gateway)
**Effort**: **LOW** (15 minutes - just port updates in podman-compose.yml)

---

## 📋 **Recommended Action Plan**

### **Quick Fix** (15 minutes)
1. Update `podman-compose.yml` ports (15438/16384/18095/18120)
2. Update header comments
3. Test: `podman-compose up -d && curl http://localhost:18095/health`
4. Verify: Run AIAnalysis integration tests

### **Complete Fix** (1-2 hours)
1. Apply Quick Fix
2. Migrate to DD-TEST-002 sequential startup (`setup-infrastructure.sh`)
3. Create `test/infrastructure/aianalysis_integration.go` constants file (if not exists)
4. Update DD-TEST-001 to note AIAnalysis migration status

---

## 📝 **Related Documents**

- `DD-TEST-001-port-allocation-strategy.md` v1.6 - Authoritative port allocation
- `DD-TEST-002-integration-test-container-orchestration.md` - Sequential startup pattern
- `test/infrastructure/aianalysis.go` - Authoritative constants (correct ports)
- `ALL_SERVICES_DS_INFRASTRUCTURE_AUDIT_DEC_22_2025.md` - Noted AIAnalysis uses podman-compose

---

**Document Status**: ✅ **COMPLETE**
**Confidence**: **100%** that podman-compose.yml has outdated ports causing conflicts
**Recommended Action**: Apply Quick Fix immediately to unblock parallel testing











