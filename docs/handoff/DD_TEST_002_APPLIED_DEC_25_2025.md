# DD-TEST-002: Hybrid Parallel Approach - Applied & Action Plan

**Date**: December 25, 2025
**Status**: 📋 **ANALYSIS COMPLETE** → 🚀 **READY FOR IMPLEMENTATION**
**Priority**: ⚠️ **HIGH** - E2E Performance Improvement
**Related**: `DD_TEST_002_HYBRID_APPROACH_ACTION_PLAN_DEC_25_2025.md`

---

## 🎯 **What Was Done**

### **1. Read DD-TEST-002 Standard** ✅

**Analyzed**:
- `docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md`
- Lines 151-442: Hybrid Parallel E2E Infrastructure Setup (authoritative)
- Lines 228-270: Dockerfile Optimization Requirements

**Key Findings**:
1. **Hybrid Pattern**: Build images in parallel → Create cluster → Load images → Deploy services
2. **Performance**: 4x faster builds, ~5-6 minutes total E2E setup (vs 20-25 minutes sequential)
3. **Reliability**: 100% success rate (no Kind cluster timeouts)
4. **Dockerfile Rule**: **NO `dnf update`** in any Dockerfile (81% faster builds)

### **2. Analyzed Current State** ✅

**Gateway (Reference Implementation)**:
- ✅ **VALIDATED**: `test/infrastructure/gateway_e2e_hybrid.go`
- ✅ **METRICS**: 298 seconds (~5 minutes) setup time
- ✅ **RELIABILITY**: 100% success rate (Dec 25, 2025)
- ✅ **PATTERN**: Build parallel → Cluster → Load → Deploy

**RemediationOrchestrator**:
- ✅ E2E tests exist: `test/e2e/remediationorchestrator/` (9 test files)
- ❌ No hybrid infrastructure file
- ✅ Current setup: Simple Kind cluster creation (no service deployment yet)
- ✅ **Dockerfile**: No RO-specific Dockerfile found (likely uses controller pattern)

**Other Services**:
- ❌ SignalProcessing: No hybrid infrastructure
- ❌ AIAnalysis: No hybrid infrastructure
- ❌ WorkflowExecution: No hybrid infrastructure
- ❌ Notification: No hybrid infrastructure
- ❌ DataStorage: No hybrid infrastructure

### **3. Dockerfile Audit** ✅

**Command**:
```bash
grep -r "dnf update" docker/ | wc -l
```

**Result**: **20 violations found** ❌

**Services with `dnf update`**:
1. storage-service.Dockerfile (2 instances)
2. gateway-ubi9.Dockerfile (2 instances)
3. data-storage.Dockerfile (2 instances)
4. alert-service.Dockerfile (2 instances)
5. workflow-service.Dockerfile (2 instances)
6. notification-controller-ubi9.Dockerfile (2 instances)
7. aianalysis.Dockerfile (2 instances)
8. executor-service.Dockerfile (2 instances)
9. notification-service.Dockerfile (2 instances)
10. (other services assumed)

**Clean Services** ✅:
- ✅ `signalprocessing-controller.Dockerfile` (uses `:1.25` base, NO `dnf update`)

**Impact**:
- **With `dnf update`**: 10 minutes per build (58 package upgrades)
- **Without `dnf update`**: 2 minutes per build (0 package upgrades)
- **Improvement**: **81% faster builds**

### **4. Created Action Plan** ✅

**Document**: `docs/handoff/DD_TEST_002_HYBRID_APPROACH_ACTION_PLAN_DEC_25_2025.md`

**Includes**:
- ✅ Comprehensive analysis of current state
- ✅ Hybrid pattern explanation with Gateway reference
- ✅ Dockerfile optimization requirements
- ✅ Service-by-service implementation plan
- ✅ Performance improvement projections
- ✅ Success criteria and validation steps

---

## 🚀 **What Needs to Be Done Next**

### **Phase 1: RemediationOrchestrator** (Immediate Priority)

#### **Step 1.1: Create Hybrid Infrastructure File**

**Action**: Create `test/infrastructure/remediationorchestrator_e2e_hybrid.go`

**Pattern**: Copy from Gateway, adapt for RO dependencies

**Required Functions**:
```go
// Main setup
func SetupROInfrastructureHybridWithCoverage(
    ctx context.Context,
    clusterName, kubeconfigPath string,
    writer io.Writer,
) error {
    // PHASE 1: Build images in parallel
    //   - RO controller image (with coverage)
    //   - DataStorage image
    //   - PostgreSQL/Redis images

    // PHASE 2: Create Kind cluster
    //   - Install ALL CRDs (RR, SP, AA, WE, NR)
    //   - Create kubernaut-system namespace

    // PHASE 3: Load images in parallel
    //   - Load RO image
    //   - Load dependency images

    // PHASE 4: Deploy services in parallel
    //   - PostgreSQL + Redis
    //   - DataStorage
    //   - RO controller
}

// Build functions
func BuildROImageWithCoverage(writer io.Writer) error
func LoadROCoverageImage(clusterName string, writer io.Writer) error
func DeployROCoverageManifest(kubeconfigPath string, writer io.Writer) error
```

**Reference**: `test/infrastructure/gateway_e2e_hybrid.go` (lines 24-223)

#### **Step 1.2: Update RO E2E Suite**

**File**: `test/e2e/remediationorchestrator/suite_test.go`

**Change**:
```go
// Lines 91-136: Replace manual Kind cluster creation with hybrid setup

// CURRENT (manual):
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // ... setup kubeconfig ...
        if !clusterExists(clusterName) {
            createKindCluster(clusterName, tempKubeconfigPath)
        }
        installCRDs()
        // TODO: Deploy services when teams respond
        return []byte(tempKubeconfigPath)
    },
    // ...
)

// NEW (hybrid):
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // ... setup kubeconfig ...
        err := infrastructure.SetupROInfrastructureHybridWithCoverage(
            ctx, clusterName, tempKubeconfigPath, GinkgoWriter,
        )
        Expect(err).ToNot(HaveOccurred())
        return []byte(tempKubeconfigPath)
    },
    // ...
)
```

#### **Step 1.3: Create or Fix RO Dockerfile** (Critical)

**Current State**: No RO-specific Dockerfile found

**Options**:
1. **If using shared controller Dockerfile**: Verify it follows pattern
2. **If creating new RO Dockerfile**: Use SignalProcessing pattern as template

**Pattern** (from `docker/signalprocessing-controller.Dockerfile`):
```dockerfile
# ✅ CORRECT: Fast builds
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder  # Latest base image

USER root
USER 1001

WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
RUN go mod download

COPY --chown=1001:0 . .

# Build with optional coverage support
ARG GOFLAGS=""
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
    CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
    -o remediationorchestrator-controller ./cmd/remediationorchestrator; \
    else \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o remediationorchestrator-controller ./cmd/remediationorchestrator; \
    fi

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /
COPY --from=builder /opt/app-root/src/remediationorchestrator-controller /remediationorchestrator-controller
RUN useradd -r -u 65532 -g root nonroot
USER nonroot
EXPOSE 9090 8081
ENTRYPOINT ["/remediationorchestrator-controller"]
```

**Validation**:
```bash
# Verify NO dnf update
grep "dnf update" docker/*remediation* && echo "❌ VIOLATION" || echo "✅ Clean"
```

#### **Step 1.4: Test & Validate**

**Command**:
```bash
cd test/e2e/remediationorchestrator
time ginkgo -p --procs=4 -v ./...
```

**Success Criteria**:
- ✅ **Setup Time**: ≤6 minutes (not 20-25 minutes)
- ✅ **Reliability**: 100% (no Kind cluster timeouts)
- ✅ **Build Time**: ≤3 minutes (parallel builds)
- ✅ **Tests Pass**: All E2E tests pass

### **Phase 2: Fix All Dockerfiles** (High Priority)

**Target**: Remove `dnf update` from **20 Dockerfiles**

**Services to Fix**:
1. storage-service.Dockerfile
2. gateway-ubi9.Dockerfile
3. data-storage.Dockerfile
4. alert-service.Dockerfile
5. workflow-service.Dockerfile
6. notification-controller-ubi9.Dockerfile
7. aianalysis.Dockerfile
8. executor-service.Dockerfile
9. notification-service.Dockerfile
10. (other services with `dnf update`)

**Pattern for Each**:
```dockerfile
# BEFORE (slow):
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder
RUN dnf update -y && \  # ← REMOVE THIS LINE
    dnf install -y git ca-certificates tzdata && \
    dnf clean all

# AFTER (fast):
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder  # ← Update to :1.25
# Install build dependencies (NO dnf update per DD-TEST-002)
RUN dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

**Validation After Each Fix**:
```bash
# Check specific service
grep "dnf update" docker/<service>.Dockerfile && echo "❌ Still present" || echo "✅ Fixed"

# Check all Dockerfiles
grep -r "dnf update" docker/ | wc -l  # Should be 0
```

### **Phase 3: Implement Hybrid for All Services** (Medium Priority)

**For Each Service** (SignalProcessing, AIAnalysis, WorkflowExecution, Notification, DataStorage):

1. Create `test/infrastructure/<service>_e2e_hybrid.go`
2. Update `test/e2e/<service>/suite_test.go`
3. Fix `docker/<service>.Dockerfile` (remove `dnf update`, use `:1.25`)
4. Test & validate (≤6 minutes setup, 100% reliability)

---

## 📊 **Expected Impact**

### **Per Service (E2E Setup)**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Build Time** | 20-25 min | **~5 min** | **4-5x faster** |
| **Reliability** | Variable | **100%** | **Perfect** |
| **Total E2E** | 30-35 min | **~10 min** | **3-4x faster** |

### **CI/CD Pipeline (All Services)**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **E2E Setup** | ~180 min (9 × 20 min) | **~45 min** (9 × 5 min) | **4x faster** |
| **Total CI/CD** | ~200 min | **~65 min** | **3x faster** |
| **Developer Feedback** | 30+ min | **~10 min** | **3x faster** |

---

## 🎓 **Key Insights from Gateway Validation** (Dec 25, 2025)

### **What Works** ✅
1. **Parallel Builds Before Cluster**: No idle time → No timeouts
2. **Latest Base Images (`:1.25`)**: No `dnf update` needed
3. **Parallel Image Loading**: Fast, reliable
4. **Parallel Service Deployment**: Fastest startup

### **What Doesn't Work** ❌
1. **Old Parallel (Cluster + Builds Simultaneously)**: 0% success (cluster timeouts)
2. **Sequential Builds**: 4x slower
3. **`dnf update` in Dockerfiles**: 81% slower builds

### **Critical Learning**
> "The hybrid approach achieves both speed AND reliability. By building images in parallel BEFORE creating the Kind cluster, we eliminate idle timeout issues while maximizing build performance."

---

## ✅ **Checklist for RO Implementation**

**Infrastructure**:
- [ ] Create `test/infrastructure/remediationorchestrator_e2e_hybrid.go`
- [ ] Implement `SetupROInfrastructureHybridWithCoverage()`
- [ ] Implement build/load/deploy functions

**E2E Suite**:
- [ ] Update `test/e2e/remediationorchestrator/suite_test.go`
- [ ] Replace manual cluster creation with hybrid setup
- [ ] Remove TODO comment about service deployment

**Dockerfile**:
- [ ] Create or verify RO Dockerfile exists
- [ ] Ensure uses `:1.25` base image (latest)
- [ ] Ensure NO `dnf update` in any RUN command
- [ ] Add optional coverage support (`GOFLAGS=-cover`)

**Validation**:
- [ ] Run E2E tests: `time ginkgo -p --procs=4 -v ./test/e2e/remediationorchestrator/...`
- [ ] Verify setup time ≤6 minutes
- [ ] Verify 100% reliability (no timeouts)
- [ ] Verify all tests pass

**Documentation**:
- [ ] Update handoff documents with results
- [ ] Note any service-specific variations

---

## 📚 **Reference Documentation**

### **Authoritative**
1. **DD-TEST-002**: Parallel Test Execution Standard
   - `docs/architecture/decisions/DD-TEST-002-parallel-test-execution-standard.md`
   - Lines 151-442: Hybrid Pattern (authoritative)
   - Lines 228-270: Dockerfile Optimization (required)

### **Implementation References**
1. **Gateway Hybrid Implementation**: Validated Dec 25, 2025
   - `test/infrastructure/gateway_e2e_hybrid.go`
2. **SignalProcessing Dockerfile**: Clean example
   - `docker/signalprocessing-controller.Dockerfile`

### **Action Plan**
1. **Comprehensive Plan**: This session's output
   - `docs/handoff/DD_TEST_002_HYBRID_APPROACH_ACTION_PLAN_DEC_25_2025.md`

---

## 🚦 **Next Steps**

### **Immediate** (This Week)
1. 🚀 **Start RO Hybrid Implementation**
   - Create infrastructure file
   - Update E2E suite
   - Verify/create Dockerfile

2. 📋 **Fix Dockerfiles** (High Impact)
   - Remove all `dnf update` commands
   - Update to `:1.25` base images
   - Validate: `grep -r "dnf update" docker/ | wc -l` should be `0`

### **Short-Term** (Next 2 Weeks)
1. 🔄 **Implement Hybrid for Other Services**
   - SignalProcessing, AIAnalysis, WorkflowExecution, Notification, DataStorage
2. ✅ **Validate All Services**
   - E2E setup ≤6 minutes
   - 100% reliability

### **Medium-Term** (Month of January)
1. 📊 **Measure CI/CD Impact**
   - Track total pipeline time reduction
2. 🔧 **Add Pre-Commit Hooks**
   - Detect `dnf update` in Dockerfiles
3. 📖 **Update Service Templates**
   - Include hybrid pattern by default

---

**Document Status**: ✅ **COMPLETE** - Ready for Implementation
**Owner**: Platform Team (guidance), Service Teams (implementation)
**Priority**: High - E2E performance improvement (4x faster)
**Next Review**: After RO implementation complete

