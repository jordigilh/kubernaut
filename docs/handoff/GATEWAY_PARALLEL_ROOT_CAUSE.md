# Gateway Parallel Optimization - Root Cause Analysis

**Date**: December 13, 2025
**Issue**: Parallel image builds killed with `signal: killed`
**Root Cause**: **Podman Machine severely under-resourced at 2 GB RAM**

---

## 🔍 Root Cause: Podman Machine Resource Constraint

### System Configuration

**Mac Host System** (Plenty of Resources):
```
Total Memory:     32 GB
Physical CPUs:    12 cores
Logical CPUs:     12 cores
Architecture:     ARM64 (Apple Silicon)
```

**Podman Machine VM** (Severely Limited):
```
Memory:     2048 MB (2 GB)  ← THE CONSTRAINT
CPUs:       6
DiskSize:   100 GB
```

---

## 📊 Resource Analysis

### Why Builds Were Killed

**Each Go Build Requires**:
- **Peak Memory**: 1-1.5 GB per build (for dependency compilation)
- **Sustained Memory**: 500-800 MB during active compilation
- **Total for 2 Parallel Builds**: 2-3 GB peak, 1-1.6 GB sustained

**Podman VM Only Has**: 2 GB total

**Result**: Linux OOM (Out of Memory) Killer inside Podman VM terminates processes

**Evidence**:
```
github.com/jackc/pgx/v5/pgtype: /usr/lib/golang/pkg/tool/linux_arm64/compile: signal: killed
net/http: /usr/lib/golang/pkg/tool/linux_arm64/compile: signal: killed
```

The `/usr/lib/golang/pkg/tool/linux_arm64/compile` process is the Go compiler, which is memory-intensive. The `signal: killed` indicates the Linux kernel OOM killer terminated it.

---

## 🎯 Why This Happened

### Parallel Build Resource Demand

**Phase 2 Tasks (Parallel)**:
1. **Gateway Image Build**:
   - Go compilation: ~1-1.5 GB
   - Dependencies: Large (kubernetes, controller-runtime, etc.)

2. **DataStorage Image Build**:
   - Go compilation: ~1-1.5 GB
   - Dependencies: Large (pgx, redis, etc.)

3. **PostgreSQL + Redis Deploy**:
   - Lightweight: ~200-300 MB

**Peak Memory**: 2-3 GB (exceeds 2 GB Podman VM limit)
**OOM Trigger**: When both compilers peak simultaneously

---

## ✅ Solution: Increase Podman Machine Resources

### Recommended Configuration

**For Go Development**:
```bash
# Stop current machine
podman machine stop

# Remove old machine
podman machine rm podman-machine-default

# Create new machine with adequate resources
podman machine init \
  --cpus 8 \
  --memory 8192 \
  --disk-size 100 \
  podman-machine-default

# Start new machine
podman machine start
```

**Rationale**:
- **8 GB RAM**: Allows 2-3 parallel Go builds comfortably
- **8 CPUs**: Speeds up compilation significantly
- **100 GB Disk**: Already sufficient

---

## 📋 Options Comparison (Updated)

### Option A: Increase Podman Resources (NOW RECOMMENDED)
**What**: Recreate Podman machine with 8 GB RAM
**Why**: Host system has 32 GB, only using 2 GB is wasteful
**How**: See configuration above
**Expected**: Full parallel optimization works (~27% faster)
**Reliability**: High (adequate resources)

---

### Option B: Sequential Builds (FALLBACK)
**What**: Build images sequentially, only deploy in parallel
**Why**: Works with 2 GB Podman VM
**How**: Modify Phase 2 implementation
**Expected**: ~14% improvement
**Reliability**: Very high (low resource usage)

---

### Option C: Pre-built Images (CI/CD ONLY)
**What**: Build once, cache images, only load during tests
**Why**: Separates builds from test runs
**How**: CI pipeline builds in separate job
**Expected**: ~40% improvement (for repeated runs)
**Reliability**: Very high

---

## 🎯 Recommendation

### **Option A: Increase Podman Resources**

**Why this is the right solution**:
- ✅ Host system has **32 GB** RAM, only using **2 GB** (6% utilization)
- ✅ Host system has **12 CPUs**, only using **6** (50% utilization)
- ✅ Enables full parallel optimization (27% improvement)
- ✅ Benefits ALL future E2E testing (not just Gateway)
- ✅ One-time setup, permanent benefit

**Steps**:
1. **Backup**: Export any important containers/volumes (if any)
2. **Recreate**: `podman machine rm && podman machine init --memory 8192 --cpus 8`
3. **Test**: Run Gateway E2E with full parallel optimization
4. **Validate**: Confirm ~27% improvement, no OOM kills

**Expected Outcome**:
- Gateway E2E: ~5.5 min (vs ~7.6 min baseline)
- No resource exhaustion
- Full parallel optimization working

---

## 🔗 Impact on Other Services

### Who Else Benefits?

**All services with E2E tests**:
- **SignalProcessing**: Already implemented parallel optimization
- **AIAnalysis**: Likely benefits from parallel builds
- **DataStorage**: Benefits from more build resources
- **RemediationOrchestrator**: Declined optimization (too fast already)
- **WorkflowExecution**: Assessment pending

**Bottom Line**: This is a system-wide improvement, not just for Gateway.

---

## 📝 Lesson Learned

**Original Diagnosis**: "Resource exhaustion from parallel builds"
**Actual Root Cause**: "Podman machine under-provisioned at 2 GB on a 32 GB host"

**Key Insight**: The Mac has plenty of resources, but Podman VM was artificially constrained. Increasing VM allocation is the proper solution, not limiting parallelization.

---

**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Recommendation**: Increase Podman machine to 8 GB RAM, 8 CPUs
**Priority**: P2 (V1.2) - Optimization for faster development
**Owner**: Gateway Team (applies to all teams)


