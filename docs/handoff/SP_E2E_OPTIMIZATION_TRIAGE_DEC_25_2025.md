# SignalProcessing E2E Optimization Triage

**Date**: December 25, 2025
**Engineer**: @jgil
**Context**: SignalProcessing E2E setup takes 507s (8.5 min) vs Gateway 298s (5 min) - a **70% performance gap (+209s)**

---

## 🎯 **Objective**

Systematically analyze the 3.5-minute (209-second) performance gap between SignalProcessing and Gateway E2E infrastructure setup to identify concrete optimization opportunities.

---

## 📊 **Performance Baseline**

| Service | Setup Time | Test Count | Status |
|---|---|---|---|
| **Gateway** | 298s (5.0 min) | 37/37 ✅ | Baseline |
| **SignalProcessing** | 507s (8.5 min) | 24/24 ✅ | +70% slower |
| **Difference** | +209s (3.5 min) | - | Target for optimization |

---

## 🔍 **Phase-by-Phase Analysis**

### Methodology

1. Compare SignalProcessing vs Gateway hybrid setup implementations
2. Identify unique SignalProcessing infrastructure requirements
3. Quantify time impact of each difference
4. Propose optimization strategies with expected ROI

### Comparison Matrix

| Phase | SignalProcessing | Gateway | Delta Analysis |
|---|---|---|---|
| **Phase 1: Image Builds** | SP controller (coverage) + DataStorage | Gateway (coverage) + DataStorage | **Equal complexity** (both build 2 images in parallel) |
| **Phase 2: Cluster Setup** | Create cluster + 2 CRDs + namespace + **4 Rego ConfigMaps** | Create cluster + 1 CRD + namespace | **+1 CRD, +4 ConfigMaps** |
| **Phase 3: Image Loading** | Load SP + DS images (parallel) | Load Gateway + DS images (parallel) | **Equal complexity** |
| **Phase 4: Service Deployment** | PostgreSQL + Redis → Migrations → DataStorage → **SP Controller** | PostgreSQL + Redis → DataStorage → **Gateway** | **+Audit migrations step** |

---

## 🚨 **Identified Bottlenecks**

### 1. **Rego Policy ConfigMap Deployment (Phase 2)**

**Current Implementation**:
```go
// test/infrastructure/signalprocessing.go:763
func deploySignalProcessingPolicies(kubeconfigPath string, writer io.Writer) error {
    // 1. Deploy environment classification policy (kubectl apply)
    // 2. Deploy priority assignment policy (kubectl apply)
    // 3. Deploy business classification policy (kubectl apply)
    // 4. Deploy custom labels extraction policy (kubectl apply)
}
```

**Impact**:
- **4 sequential `kubectl apply` commands** (estimated ~5-10s each = 20-40s total)
- Each command involves:
  - CLI invocation overhead
  - API server validation
  - etcd write + replication
  - ConfigMap data parsing (large Rego files)

**Optimization Opportunity #1**:
- **Batch all 4 ConfigMaps into a single YAML manifest**
- Apply with one `kubectl apply -f -` call
- **Expected savings**: 15-30 seconds

**Implementation**:
```go
func deploySignalProcessingPolicies(kubeconfigPath string, writer io.Writer) error {
    // Combine all 4 ConfigMaps into a single manifest
    combinedManifest := fmt.Sprintf(`
---
%s
---
%s
---
%s
---
%s
`, envPolicy, priorityPolicy, businessPolicy, customLabelsPolicy)

    cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
    cmd.Stdin = strings.NewReader(combinedManifest)
    cmd.Stdout = writer
    cmd.Stderr = writer
    return cmd.Run()
}
```

---

### 2. **Additional CRD Installation (Phase 2)**

**Current Implementation**:
- SignalProcessing installs **2 CRDs** (SignalProcessing + RemediationRequest)
- Gateway installs **1 CRD** (RemediationRequest only)

**Impact**:
- Additional `kubectl apply` for SignalProcessing CRD (estimated ~5-10s)
- CRD validation webhook configuration
- API server schema registration

**Optimization Opportunity #2**:
- **Batch both CRDs into a single `kubectl apply` call**
- **Expected savings**: 3-5 seconds

**Implementation**:
```go
// Create combined CRD manifest
crdManifest := fmt.Sprintf(`
---
%s
---
%s
`, signalProcessingCRD, remediationRequestCRD)

cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
cmd.Stdin = strings.NewReader(crdManifest)
```

---

### 3. **Audit Migrations Execution (Phase 4)**

**Current Implementation**:
```go
// Phase 4: Sequential deployment
// 1. Deploy PostgreSQL + Redis (parallel)
// 2. Wait for infra ready
// 3. Apply audit migrations      ← UNIQUE TO SIGNALPROCESSING
// 4. Deploy DataStorage
// 5. Wait for DataStorage ready
// 6. Deploy SignalProcessing controller
```

**Impact**:
- `ApplyAuditMigrations` is a **sequential database operation**
- Cannot be parallelized due to schema dependencies
- Estimated time: ~10-20 seconds

**Optimization Opportunity #3**:
- **Pre-apply migrations during PostgreSQL deployment**
- Use Kubernetes Job that runs migrations as PostgreSQL init container
- **Expected savings**: 5-10 seconds (eliminates wait for migrations to complete)

**Implementation**:
```yaml
# Deploy PostgreSQL with migrations as Job
apiVersion: batch/v1
kind: Job
metadata:
  name: postgres-migrations
spec:
  template:
    spec:
      initContainers:
      - name: wait-for-postgres
        image: postgres:15
        command: ["/bin/sh", "-c", "until pg_isready -h postgresql -p 5432; do sleep 1; done"]
      containers:
      - name: migrations
        image: migrate/migrate
        command: ["/migrate", "-path", "/migrations", "-database", "postgres://...", "up"]
      restartPolicy: OnFailure
```

---

### 4. **Dockerfile Build Complexity**

**Analysis**:
```bash
$ wc -l docker/*.Dockerfile
  66 docker/signalprocessing-controller.Dockerfile  ← SMALLEST
 107 docker/gateway-ubi9.Dockerfile
 117 docker/data-storage.Dockerfile
```

**Verdict**: **NOT A BOTTLENECK**
- SignalProcessing Dockerfile is actually **38% smaller** than Gateway
- Both use cached layers during E2E runs
- Build times should be comparable

---

### 5. **Phase 4 Sequential Deployment Strategy**

**Current Implementation**:
```go
// Phase 4: Sequential deployment (BR-SP-090 compliance)
deployPostgreSQL()  → deployRedis() → applyMigrations() → deployDataStorage() → deployController()
```

**Impact**:
- **Forced sequential execution** due to dependency chain
- DataStorage **must wait** for PostgreSQL + migrations
- Controller **must wait** for DataStorage

**Optimization Opportunity #4**:
- **Parallelize Redis + PostgreSQL + DataStorage image load** (already done)
- **Pre-warm migrations** as part of PostgreSQL deployment
- **Reduce polling intervals** for readiness checks
- **Expected savings**: 10-20 seconds

**Current readiness checks**:
```go
// Example: waitForSPDataStorageReady
for {
    time.Sleep(5 * time.Second)  // ← Could be reduced to 2s
    // Check pod readiness
}
```

---

## 📈 **Optimization ROI Matrix**

| Optimization | Complexity | Expected Savings | Risk | Priority |
|---|---|---|---|---|
| **#1: Batch Rego ConfigMaps** | Low (30 min) | 15-30s | Low | **HIGH** |
| **#2: Batch CRD Installation** | Low (15 min) | 3-5s | Low | **HIGH** |
| **#3: Pre-apply Migrations** | Medium (2-3 hrs) | 5-10s | Medium | **MEDIUM** |
| **#4: Reduce Polling Intervals** | Low (30 min) | 10-20s | Low | **HIGH** |
| **Total Expected Savings** | - | **33-65s** | - | - |

---

## 🎯 **Recommended Implementation Plan**

### Phase 1: Low-Hanging Fruit (1-2 hours, 28-55s savings)

1. **Batch Rego ConfigMaps** (Optimization #1)
   - Combine 4 sequential `kubectl apply` into 1
   - Expected: 15-30s savings
   - Risk: None (YAML concatenation)

2. **Batch CRD Installation** (Optimization #2)
   - Combine 2 sequential `kubectl apply` into 1
   - Expected: 3-5s savings
   - Risk: None (CRD batching is standard)

3. **Reduce Readiness Polling** (Optimization #4)
   - Change `time.Sleep(5*time.Second)` to `2*time.Second`
   - Add max retry limit to prevent infinite loops
   - Expected: 10-20s savings
   - Risk: Low (shorter intervals are common in E2E tests)

### Phase 2: Structural Improvement (2-3 hours, 5-10s savings)

4. **Pre-apply Migrations as Job** (Optimization #3)
   - Requires refactoring PostgreSQL deployment
   - Expected: 5-10s savings
   - Risk: Medium (dependency on PostgreSQL init timing)

---

## 🔬 **Root Cause Hypothesis**

Based on this analysis, the **primary contributors** to the 70% performance gap are:

1. **Rego Policy Deployment** (20-40s) - **4 sequential kubectl calls**
2. **Readiness Check Overhead** (10-20s) - **Conservative 5s polling intervals**
3. **Additional CRD** (5-10s) - **SignalProcessing CRD installation**
4. **Audit Migrations** (5-10s) - **Sequential database schema updates**

**Total Identified**: **40-80 seconds** of the 209-second gap

---

## ⚠️ **Unidentified Time Budget**

**Known optimizations**: 40-80s of 209s gap = **19-38% explained**

**Remaining unaccounted time**: **129-169 seconds (62-81%)**

### Possible Contributing Factors

1. **DataStorage Build Time**:
   - Both services build identical DataStorage image
   - Possible: SignalProcessing builds with different cache state?
   - **Action**: Add phase-level timestamps to measure actual build duration

2. **PostgreSQL Deployment**:
   - Both services deploy PostgreSQL identically
   - Possible: Non-deterministic container pull times?
   - **Action**: Profile PostgreSQL deployment time separately

3. **Kind Cluster Creation**:
   - Both services use same Kind configuration
   - Possible: Non-deterministic Podman/Kind interaction?
   - **Action**: Measure cluster creation time in isolation

4. **Image Loading**:
   - Both services load 2 images in parallel
   - Possible: SignalProcessing images are larger?
   - **Action**: Compare image sizes (`podman images --format "{{.Repository}}:{{.Tag}} {{.Size}}"`)

---

## 🧪 **Validation Strategy**

### Pre-Optimization Baseline

```bash
# Run SignalProcessing E2E with timestamps
time make test-e2e-signalprocessing 2>&1 | tee /tmp/sp-before.log

# Extract phase timings
grep "PHASE\|✅" /tmp/sp-before.log | grep -E "@ [0-9]{2}:[0-9]{2}:[0-9]{2}"
```

### Post-Optimization Validation

```bash
# After implementing optimizations #1, #2, #4
time make test-e2e-signalprocessing 2>&1 | tee /tmp/sp-after.log

# Compare timings
diff <(grep "PHASE" /tmp/sp-before.log) <(grep "PHASE" /tmp/sp-after.log)
```

### Success Criteria

- **Target**: Reduce setup time from 507s to **470s or less** (7.8 min)
- **Savings**: At least **37 seconds** (minimum from low-hanging fruit)
- **Stretch Goal**: Reduce to **440s** (7.3 min) with all 4 optimizations

---

## 📋 **Action Items**

### Immediate (This Session)

- [x] Complete triage analysis
- [ ] Implement Optimization #1 (Batch Rego ConfigMaps)
- [ ] Implement Optimization #2 (Batch CRD Installation)
- [ ] Implement Optimization #4 (Reduce Polling Intervals)
- [ ] Run validation tests
- [ ] Update DD-TEST-002 with optimized timings

### Follow-Up (Future Session)

- [ ] Add phase-level timestamps to hybrid setup functions
- [ ] Profile individual phase durations
- [ ] Compare image sizes (SP vs Gateway)
- [ ] Investigate remaining 62-81% unaccounted time
- [ ] Consider Optimization #3 (Pre-apply Migrations) if needed

---

## 🎯 **Expected Outcome**

**Current State**:
- SignalProcessing: 507s (8.5 min)
- Gateway: 298s (5 min)
- Gap: 209s (70% slower)

**After Low-Hanging Fruit Optimizations**:
- SignalProcessing: **470s (7.8 min)** *(conservative)* or **442s (7.4 min)** *(optimistic)*
- Gap: **172s (58% slower)** *(conservative)* or **144s (48% slower)** *(optimistic)*
- Improvement: **37-65 seconds (7-13%)**

**Next Milestone**:
- Profile remaining gap with granular timestamps
- Target: **<60 seconds (20%) gap** as acceptable due to inherent complexity (4 Rego policies, audit migrations, additional CRD)

---

## 📚 **References**

- **DD-TEST-002**: Parallel Test Execution Standard
- **BR-SP-090**: Audit Event Persistence Requirement
- **BR-SP-051/070/071**: Rego Policy Requirements
- `/test/infrastructure/signalprocessing_e2e_hybrid.go`: Current implementation
- `/test/infrastructure/gateway_e2e_hybrid.go`: Gateway baseline

---

**Status**: ✅ Triage Complete - Ready for Implementation
**Next Step**: Implement Optimizations #1, #2, #4 (estimated 1-2 hours)

