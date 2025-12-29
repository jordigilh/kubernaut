# DD-TEST-002: Hybrid Parallel E2E Approach - Action Plan

**Date**: December 25, 2025
**Status**: 📋 **ACTION REQUIRED**
**Priority**: ⚠️ **HIGH** - Performance & Reliability
**Based On**: DD-TEST-002 v1.0 (Parallel Test Execution Standard)

---

## 🎯 **Objective**

Apply DD-TEST-002's **Hybrid Parallel E2E Infrastructure Setup** across all Kubernaut services to achieve:
- ✅ **4x faster builds** (parallel image builds)
- ✅ **100% reliability** (no Kind cluster timeouts)
- ✅ **~5-6 minutes** total E2E setup time (vs 20-25 minutes sequential)

---

## 📊 **Current State Analysis**

### ✅ **Services with Hybrid Pattern Implemented**

| Service | Status | File | Validated |
|---------|--------|------|-----------|
| **Gateway** | ✅ **COMPLETE** | `test/infrastructure/gateway_e2e_hybrid.go` | Dec 25, 2025 |

**Gateway Results** (with mandatory `dnf update`):
- Setup Time: **~5 minutes** (298 seconds)
- Build Strategy: Parallel (Gateway ‖ DataStorage)
- Cluster: Created after builds (no timeout)
- Reliability: **100% success rate**

### ⏳ **Services Needing Implementation**

| Service | E2E Tests Exist | Infrastructure File | Status |
|---------|----------------|---------------------|--------|
| **RemediationOrchestrator** | ✅ Yes (9 tests) | ❌ Missing | ⏳ **PENDING** |
| **SignalProcessing** | ✅ Yes | ❌ Missing | ⏳ **PENDING** |
| **AIAnalysis** | ✅ Yes | ❌ Missing | ⏳ **PENDING** |
| **WorkflowExecution** | ✅ Yes | ❌ Missing | ⏳ **PENDING** |
| **Notification** | ✅ Yes | ❌ Missing | ⏳ **PENDING** |
| **DataStorage** | ✅ Yes | ❌ Missing | ⏳ **PENDING** |

---

## 🔧 **The Hybrid Parallel Pattern** (Authoritative from DD-TEST-002)

### **4-Phase Strategy**

```
PHASE 1: Build images in PARALLEL ⚡
  ├── Service image (with coverage if enabled)
  ├── Dependencies (DataStorage, Redis, etc.)
  └── ⏳ Wait for ALL builds to complete

PHASE 2: Create Kind cluster 🎯 (after builds complete)
  ├── Install CRDs
  └── Create namespaces

PHASE 3: Load images into cluster 📦 (parallel)
  ├── Load service image
  └── Load dependency images

PHASE 4: Deploy services 🚀 (parallel)
  ├── Deploy dependencies (PostgreSQL, Redis, etc.)
  ├── Deploy service
  └── ⏳ Wait for all services ready
```

### **Why This Works**

| Aspect | Benefit |
|--------|---------|
| **Build Parallel** | Maximizes CPU → 4x faster than sequential |
| **Cluster After Builds** | No idle time → prevents Kind timeout |
| **Load Immediately** | Fresh cluster → reliable image loading |
| **Deploy Parallel** | Fastest service startup |

### **Validation Metrics** (Gateway, Dec 25, 2025)

- ✅ **Setup Time**: 298 seconds (~5 minutes) vs 25 minutes (sequential)
- ✅ **Reliability**: 100% success rate (no cluster timeouts)
- ✅ **Speed**: 4x faster builds with parallel execution

---

## 🚨 **Critical Finding: Dockerfile Optimization Required**

### **DD-TEST-002 Requirement**

> All service Dockerfiles MUST be optimized for fast E2E builds:
> **Standard**: Use latest UBI9 base images, **NO** `dnf update`

### **Current State: ❌ VIOLATIONS FOUND**

```bash
$ grep -r "dnf update" docker/ | wc -l
20  # ← 20 dnf update commands found!
```

**Services with `dnf update`**:
- ❌ storage-service.Dockerfile
- ❌ gateway-ubi9.Dockerfile
- ❌ data-storage.Dockerfile
- ❌ alert-service.Dockerfile
- ❌ workflow-service.Dockerfile
- ❌ notification-controller-ubi9.Dockerfile
- ❌ aianalysis.Dockerfile
- ❌ executor-service.Dockerfile
- ❌ notification-service.Dockerfile
- ❌ signalprocessing-controller.Dockerfile (assumed)

### **Impact of `dnf update`**

| Metric | With `dnf update` | Without `dnf update` | Improvement |
|--------|------------------|---------------------|-------------|
| **Build Time** | ~10 minutes | ~2 minutes | **81% faster** |
| **Package Upgrades** | ~58 packages | 0 packages | **100% fewer** |
| **Parallel Build Impact** | Amplified (multiple slow builds) | Fast | **Critical** |

### **Why `dnf update` is Problematic**

1. ⏱️ **Adds 5-10 minutes** to every build
2. 🔄 **E2E tests run frequently** - slow builds = slow feedback
3. 📦 **Latest base images** (`:1.25`, `:latest`) already have current packages
4. ⚡ **Parallel builds amplify** the problem (multiple slow builds competing)

### **Required Fix**

```dockerfile
# ❌ WRONG: Slow builds (~10 minutes)
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder
RUN dnf update -y && \  # ← REMOVE THIS
    dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

```dockerfile
# ✅ CORRECT: Fast builds (~2 minutes)
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
# Install build dependencies (NO dnf update)
RUN dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

---

## 📋 **Implementation Plan**

### **Phase 1: RemediationOrchestrator** (Priority: Immediate)

#### **Step 1: Create Hybrid Infrastructure File**

**File**: `test/infrastructure/remediationorchestrator_e2e_hybrid.go`

**Pattern**: Copy from `gateway_e2e_hybrid.go` and adapt

**Required Functions**:
```go
// Main setup function
func SetupROInfrastructureHybridWithCoverage(
    ctx context.Context,
    clusterName, kubeconfigPath string,
    writer io.Writer,
) error

// Phase 1: Build images in parallel
func BuildROImageWithCoverage(writer io.Writer) error
func buildDependencyImages(writer io.Writer) error  // PostgreSQL, Redis, DS

// Phase 2: Create cluster
func createROKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error

// Phase 3: Load images in parallel
func LoadROCoverageImage(clusterName string, writer io.Writer) error
func loadDependencyImages(clusterName string, writer io.Writer) error

// Phase 4: Deploy services in parallel
func deployROWithCoverage(kubeconfigPath string, writer io.Writer) error
func deployRODependencies(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error
```

#### **Step 2: Update RO E2E Suite**

**File**: `test/e2e/remediationorchestrator/suite_test.go`

**Changes**:
```go
// BEFORE (current - inline Kind cluster creation)
var _ = SynchronizedBeforeSuite(
    func() []byte {
        createKindCluster(clusterName, kubeconfigPath)
        installCRDs()
        // TODO: Deploy services
        return []byte(kubeconfigPath)
    },
    // ...
)

// AFTER (use hybrid infrastructure)
var _ = SynchronizedBeforeSuite(
    func() []byte {
        err := infrastructure.SetupROInfrastructureHybridWithCoverage(
            ctx, clusterName, kubeconfigPath, GinkgoWriter,
        )
        Expect(err).ToNot(HaveOccurred())
        return []byte(kubeconfigPath)
    },
    // ...
)
```

#### **Step 3: Fix RO Dockerfile** (Critical)

**File**: `docker/remediationorchestrator.Dockerfile` (or equivalent)

**Change**:
```dockerfile
# Remove dnf update -y from ALL RUN commands
# Use latest base image: ubi9/go-toolset:1.25
```

**Validation**:
```bash
grep "dnf update" docker/*remediationorchestrator* || echo "✅ Clean"
```

### **Phase 2: All Other Services** (Priority: High)

Repeat Phase 1 for each service:
1. Create `<service>_e2e_hybrid.go` in `test/infrastructure/`
2. Update `test/e2e/<service>/suite_test.go`
3. Fix `docker/<service>.Dockerfile` (remove `dnf update`)

### **Phase 3: Validation** (Priority: Critical)

**Per Service**:
```bash
# Run E2E tests with timing
time make test-e2e-<service>

# Expected:
# - Setup: ~5-6 minutes (not 20-25 minutes)
# - No Kind cluster timeouts
# - 100% success rate
```

**All Services**:
```bash
# Verify NO dnf update in any Dockerfile
grep -r "dnf update" docker/ && echo "❌ VIOLATIONS FOUND" || echo "✅ Clean"
```

---

## 📊 **Expected Performance Improvements**

### **Per Service (E2E Setup)**

| Metric | Sequential | Old Parallel | Hybrid Parallel | Improvement |
|--------|-----------|--------------|----------------|-------------|
| **Build Time** | 20-25 min | ~12 min (TIMEOUT) | **~5 min** | **4-5x faster** |
| **Reliability** | 100% | 0% (crash) | **100%** | **Perfect** |
| **Total E2E** | ~30-35 min | N/A | **~10 min** | **3-4x faster** |

### **CI/CD Pipeline (All Services)**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **E2E Setup** | ~180 min (9 services × 20 min) | **~45 min** (9 services × 5 min) | **4x faster** |
| **Total CI/CD** | ~200 min | **~65 min** | **3x faster** |
| **Developer Feedback** | 30+ min | **~10 min** | **3x faster** |

---

## ✅ **Success Criteria**

| Metric | Target | Measurement |
|--------|--------|-------------|
| **E2E Setup Time** | ≤6 minutes | Time from start to "services ready" |
| **Reliability** | 100% | No Kind cluster timeouts |
| **Build Speed** | ≤3 minutes | Parallel image builds |
| **Dockerfile Compliance** | 0 `dnf update` | `grep -r "dnf update" docker/` returns 0 |

---

## 🚀 **Action Items**

### **Immediate** (Week of Dec 30, 2025)

- [ ] **RO Team**: Create `remediationorchestrator_e2e_hybrid.go`
- [ ] **RO Team**: Update `test/e2e/remediationorchestrator/suite_test.go`
- [ ] **RO Team**: Fix RO Dockerfile (remove `dnf update`)
- [ ] **RO Team**: Validate E2E setup time ≤6 minutes

### **Short-Term** (Week of Jan 6, 2026)

- [ ] **SP Team**: Implement hybrid pattern + fix Dockerfile
- [ ] **AA Team**: Implement hybrid pattern + fix Dockerfile
- [ ] **WE Team**: Implement hybrid pattern + fix Dockerfile
- [ ] **NT Team**: Implement hybrid pattern + fix Dockerfile
- [ ] **DS Team**: Implement hybrid pattern + fix Dockerfile

### **Medium-Term** (Week of Jan 13, 2026)

- [ ] **Platform Team**: Add pre-commit hook to detect `dnf update` in Dockerfiles
- [ ] **Platform Team**: Update CI/CD pipelines to use hybrid pattern
- [ ] **Platform Team**: Document hybrid pattern in service templates

---

## 📚 **Reference Documentation**

1. **DD-TEST-002**: Parallel Test Execution Standard (authoritative)
   - `docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md`
2. **Gateway Implementation**: Reference example
   - `test/infrastructure/gateway_e2e_hybrid.go`
3. **DD-TEST-007**: E2E Coverage Capture Standard (integration with hybrid)
4. **DD-TEST-001**: Port Allocation Strategy (NodePort assignments)

---

## 🎓 **Lessons Learned** (Gateway Validation, Dec 25, 2025)

### **What Works**
1. ✅ Parallel image builds (even with `dnf update`) prevent timeouts
2. ✅ Create cluster AFTER builds complete (no idle time)
3. ✅ Load images immediately (fresh cluster, reliable)
4. ✅ Parallel service deployment (fastest startup)

### **What Doesn't Work**
1. ❌ Old parallel approach (cluster + builds simultaneously) → 0% success
2. ❌ Sequential builds → 4x slower
3. ❌ `dnf update` in Dockerfiles → 81% slower builds

### **Critical Insight**
> "The hybrid approach is not just faster, it's MORE RELIABLE. By building in parallel BEFORE creating the cluster, we eliminate idle timeout issues entirely while maximizing build speed."

---

## 📞 **Support**

### **For Implementation Help**
- **Reference**: `test/infrastructure/gateway_e2e_hybrid.go` (validated example)
- **Contact**: Platform Team
- **Pattern**: Copy Gateway, adapt service-specific details

### **For Dockerfile Optimization**
- **Rule**: NO `dnf update` in ANY Dockerfile
- **Rule**: Use latest base images (`:1.25`, `:latest`)
- **Validation**: `grep -r "dnf update" docker/ | wc -l` should return `0`

---

**Document Status**: ✅ **Ready for Implementation**
**Priority**: High - Blocks E2E performance improvements
**Owner**: Platform Team (architecture), Service Teams (implementation)
**Next Review**: After RO implementation (Week of Jan 6, 2026)

