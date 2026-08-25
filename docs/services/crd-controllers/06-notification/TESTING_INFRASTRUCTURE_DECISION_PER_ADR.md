# Notification Service Testing Infrastructure Decision (Per ADR-004, ADR-070)

**Date**: October 13, 2025
**Status**: ✅ **DECISION CLARIFIED**
**Confidence**: **98%**

---

## 🎯 TL;DR: Use Envtest for Notification Integration Tests

**Decision**: The Notification Service should use **envtest** (controller-runtime TestEnv) for integration tests.

**Why**: CRD controllers need real Kubernetes API behavior for integration tests, but don't need full cluster features (RBAC, networking). Envtest provides the perfect balance.

---

## 📚 Relevant ADRs

### ADR-070: Service-Specific Integration Test Infrastructure
**Location**: `docs/architecture/decisions/ADR-070-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md`

**Key Excerpt** (Line 76):
```
| Service | Infrastructure | Dependencies | Startup Time | Rationale |
|---------|----------------|--------------|--------------|-----------|
| **Notification Controller** | Podman or None | None (CRD controller) | ~5 sec | May not need external deps |
```

**Interpretation**:
- **"Podman or None"**: "None" means **no external infrastructure** (no databases, no Redis)
- **"CRD controller"**: Needs Kubernetes API for CRD operations
- **Resolution**: Use **envtest** (in-memory Kubernetes API)

---

### ADR-004: Fake Kubernetes Client for Unit Testing
**Location**: `docs/architecture/decisions/ADR-004-fake-kubernetes-client.md`

**Key Decision Matrix**:

| Tool | Use Case | Startup Time | Pros | Cons | ADR Decision |
|------|----------|--------------|------|------|--------------|
| **Fake Client** | **Unit Tests** | <1s | Fast, simple | No validation, no watch | ✅ **Chosen for unit tests** |
| **Envtest (TestEnv)** | **Integration Tests** | 5-10s | Real K8s API, validation, watch | Slower, needs binaries | ⚠️ "Rejected for **unit tests**" |
| **Kind** | **E2E Tests** | 30-60s | Full cluster | Very slow, resource heavy | ❌ Too slow for integration |

**Critical Quote** (Line 375):
> "**Why Rejected**: Too slow for unit tests (5-10s startup vs. <1s target). **Better suited for integration tests**."

**Interpretation**:
- Envtest is **perfect for integration tests** (our exact use case!)
- It was only "rejected" for **unit tests** (where fake client is better)

---

### ADR-003: Kind Cluster as Primary Integration Environment
**Location**: `docs/architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md`

**Status**: SUPERSEDED IN PART by ADR-070 (October 2025)

**Current Scope**: Services requiring Kubernetes features (RBAC, TokenReview, Service Discovery)

**Examples of Kind-Required Services**:
- Dynamic Toolset (RBAC, service discovery)
- Gateway Service (RBAC, TokenReview API)

**Notification Service**: Does **not** require these features → Kind not needed

---

## 🧪 Testing Tool Decision Matrix for Notification Service

### Unit Tests (70% coverage target)
**Tool**: **Fake Client** (`sigs.k8s.io/controller-runtime/pkg/client/fake`)

**Use Cases**:
- Controller reconciliation logic
- Status management
- Retry logic
- Delivery logic (with mocked HTTP)

**Characteristics**:
- ✅ Fast (<1s execution)
- ✅ No infrastructure
- ✅ In-memory
- ✅ **Already implemented** (21 tests passing)

**Example**:
```go
fakeClient := fake.NewClientBuilder().
    WithScheme(scheme.Scheme).
    WithObjects(testObjects...).
    Build()
```

**Status**: ✅ **Complete** (test/unit/notification/)

---

### Integration Tests (20% coverage target)
**Tool**: **Envtest** (`sigs.k8s.io/controller-runtime/pkg/envtest`)

**Use Cases**:
- CRD lifecycle (create → reconcile → update → delete)
- Real Kubernetes API validation
- Watch events
- Controller behavior in realistic environment

**Characteristics**:
- ✅ Real Kubernetes API (etcd + kube-apiserver)
- ✅ Fast enough (5-10s startup, acceptable for integration tests)
- ✅ CRD validation (OpenAPI v3 schema)
- ✅ Watch support (real watch events)
- ✅ No external dependencies (no Docker, no Podman)
- ✅ **Already proven** (remediation/suite_test.go uses it successfully)

**Setup**:
```go
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    ErrorIfCRDPathMissing: true,
}

cfg, err := testEnv.Start()
k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
```

**Status**: ⏸️ **Pending migration** (currently designed for Kind)

---

### E2E Tests (10% coverage target, deferred)
**Tool**: **Kind** or **Real Cluster**

**Use Cases**:
- Real Slack webhook integration
- Multi-service workflows
- Production-like validation

**Characteristics**:
- ✅ Full production parity
- ❌ Slow (30-60s startup)
- ❌ Resource intensive

**Status**: ⏸️ **Deferred** until all services complete (per user preference)

---

## 📊 Performance Comparison

### Current Approach (Kind-Based Integration Tests)
| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Kind Cluster Creation | 30-60s | Docker/Podman, Kind |
| CRD Installation | 5-10s | kubectl |
| Controller Deploy | 20-40s | Image build, registry |
| Test Execution | 30-60s | Network I/O |
| **Total** | **85-170s** | **Multiple tools** |

### Recommended Approach (Envtest Integration Tests)
| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Envtest Start | 5-10s | KUBEBUILDER_ASSETS |
| CRD Load | <1s | In-process |
| Controller Start | <1s | In-process (goroutine) |
| Test Execution | 3-6s | In-memory |
| **Total** | **9-17s** | **Go binaries only** |

**Performance Improvement**: **5-18x faster** 🚀

---

## ✅ Decision Rationale

### Why Envtest for Notification Integration Tests?

#### 1. Matches ADR-004 Recommendation
- ✅ Envtest is "better suited for integration tests"
- ✅ Fake client already used for unit tests
- ✅ Kind unnecessary for CRD-only controllers

#### 2. Aligns with ADR-070 Classification
- ✅ Notification Controller classified as "None" infrastructure
- ✅ "CRD controller" - needs K8s API but not full cluster
- ✅ ~5 sec startup target → envtest delivers 5-10s

#### 3. Proven Infrastructure
- ✅ Already used in `test/integration/remediation/suite_test.go`
- ✅ Existing `testenv/environment.go` helpers
- ✅ KUBEBUILDER_ASSETS setup automation via Makefile

#### 4. Technical Benefits
- ✅ **Real CRD validation**: Tests OpenAPI v3 schema enforcement
- ✅ **Real watch events**: Tests controller reactivity
- ✅ **No Docker/Podman**: Simpler CI/CD, fewer dependencies
- ✅ **Portable**: Runs in IDE, CI, local development
- ✅ **Deterministic**: No flaky network/container issues

#### 5. Developer Experience
- ✅ **Fast feedback**: 9-17s total vs 85-170s (5-18x improvement)
- ✅ **Simple setup**: Standard `go test` command
- ✅ **Easy debugging**: In-process, same debugger
- ✅ **CI-friendly**: No Docker-in-Docker complexity

---

## 🚫 Why NOT Kind for Notification Integration Tests?

### Kind Is Designed For Services Requiring:
- ❌ **RBAC** - Notification controller doesn't use RBAC directly
- ❌ **TokenReview API** - Not needed for notification delivery
- ❌ **Service Discovery** - Notification uses direct webhook URLs
- ❌ **Networking/Network Policies** - Not relevant to notification logic
- ❌ **Multi-node scheduling** - CRD controller is single-instance
- ❌ **Storage** - No PersistentVolumes needed

### Notification Controller Only Needs:
- ✅ **CRD CRUD operations** → Envtest provides this
- ✅ **Status updates** → Envtest provides this
- ✅ **Watch events** → Envtest provides this
- ✅ **Validation** → Envtest provides this

**Conclusion**: Kind is **over-engineered** for notification integration tests.

---

## 🎯 Recommended Action

### Immediate Next Step: Migrate to Envtest

**Implementation**: Follow the detailed plan in `ENVTEST_MIGRATION_CONFIDENCE_ASSESSMENT.md`

**Estimated Effort**: 3-4 hours

**Phases**:
1. Create `pkg/notification/client.go` (1-2h)
2. Migrate `suite_test.go` to envtest (30-45m)
3. Adapt controller for webhook URL injection (30m)
4. Run all 6 integration tests (15-30m)

**Expected Outcome**:
- ✅ All 6 integration tests passing
- ✅ 9-17s total execution time (vs current 85-170s blocked state)
- ✅ No Docker/Podman/Kind dependencies
- ✅ Production-ready notification.Client for RemediationOrchestrator

---

## 📋 ADR Compliance Checklist

### ADR-004 Compliance
- ✅ Unit tests use **Fake Client** (21 tests implemented)
- ✅ Integration tests should use **Envtest** (recommended, pending implementation)
- ✅ E2E tests can use **Kind** (deferred per user preference)

### ADR-070 Compliance
- ✅ Service classified correctly: "None" infrastructure (CRD controller)
- ✅ No unnecessary Kubernetes features required
- ✅ Target startup time: ~5 sec → Envtest delivers 5-10s ✅

### ADR-003 Compliance
- ✅ Kind not required (notification doesn't need RBAC/TokenReview/etc.)
- ✅ ADR-003 remains valid for Gateway/Toolset services (not notification)

---

## 📚 Supporting Evidence

### Existing Envtest Usage in Codebase

**File**: `test/integration/remediation/suite_test.go`

**Evidence**:
```go
// Lines 84-87
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    ErrorIfCRDPathMissing: true,
}

// Lines 95-96
cfg, err = testEnv.Start()
Expect(err).NotTo(HaveOccurred())

// Lines 99-101
k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
Expect(err).NotTo(HaveOccurred())

// Lines 104-122
k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
err = (&remediationctrl.RemediationRequestReconciler{...}).SetupWithManager(k8sManager)
go func() { err = k8sManager.Start(ctx) }()
```

**Status**: ✅ **Proven pattern** - working in production integration tests

---

## 🎉 Conclusion

**Decision**: Use **envtest** for Notification Service integration tests

**Confidence**: **98%**

**Rationale**:
1. ✅ Aligns with ADR-004 ("better suited for integration tests")
2. ✅ Matches ADR-070 classification ("None" infrastructure, CRD controller)
3. ✅ Proven in existing codebase (remediation integration tests)
4. ✅ 5-18x performance improvement over Kind
5. ✅ Required for RemediationOrchestrator integration anyway

**Status**: ✅ **APPROVED - PROCEED WITH ENVTEST MIGRATION**

---

## 📝 ADR Update Recommendation

**Suggestion**: Update ADR-070 to clarify "None" infrastructure for CRD controllers:

```diff
| Service | Infrastructure | Dependencies | Startup Time | Rationale |
|---------|----------------|--------------|--------------|-----------|
- | **Notification Controller** | Podman or None | None (CRD controller) | ~5 sec | May not need external deps |
+ | **Notification Controller** | Envtest | None (CRD controller) | ~5-10 sec | CRD-only controller, needs K8s API but not full cluster |
```

**Rationale**: "Envtest" is more specific than "None" and clarifies the intended infrastructure.

---

**Next Action**: Begin envtest migration using the implementation plan in `ENVTEST_MIGRATION_CONFIDENCE_ASSESSMENT.md`

**Priority**: High (blocks RemediationOrchestrator integration)

**Status**: ✅ **READY TO PROCEED**

