# WorkflowExecution Team: API Group Migration Triage

**Document Type**: Migration Triage & Action Plan
**Status**: ⚠️ **ACTION REQUIRED** - Breaking change before V1.0 GA
**Priority**: P0 - BLOCKING for segmented E2E tests with RO
**Estimated Effort**: 2-3 hours
**Target Completion**: Before RO E2E coordination work
**Created**: 2025-12-13

**Source Document**: [SHARED_APIGROUP_MIGRATION_NOTICE.md](SHARED_APIGROUP_MIGRATION_NOTICE.md)
**Authoritative Standard**: [DD-CRD-001: CRD API Group Domain Selection](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)

---

## 📋 Executive Summary

**What**: Migrate WorkflowExecution CRD from `kubernaut.ai/v1alpha1` to `kubernaut.ai/v1alpha1`

**Why**:
- ✅ Aligns with DD-CRD-001 authoritative standard (updated 2025-12-13)
- ✅ Follows industry best practices (K8sGPT, Prometheus, Cert-Manager, ArgoCD)
- ✅ Simplifies kubectl commands and RBAC
- ✅ Required before RO E2E coordination work begins

**Impact**:
- ✅ **Minimal Risk**: Mechanical change with well-defined scope
- ⚠️ **Breaking Change**: Requires CRD regeneration and E2E test updates
- ⏰ **Timing**: Should complete AFTER BR-WE-006 (to avoid merge conflicts)

**Recommendation**: ✅ **PROCEED** - Migration is straightforward and necessary for V1.0 GA

---

## 🎯 Migration Overview

### Current State
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: wfe-example
spec:
  workflowRef:
    workflowID: increase-memory
    containerImage: ghcr.io/kubernaut/workflows/increase-memory:v1
  targetResource: production/deployment/payment-api
```

### Target State
```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: wfe-example
spec:
  workflowRef:
    workflowID: increase-memory
    containerImage: ghcr.io/kubernaut/workflows/increase-memory:v1
  targetResource: production/deployment/payment-api
```

**Change**: `kubernaut.ai/v1alpha1` → `kubernaut.ai/v1alpha1`

---

## 📊 Impact Assessment

### Files Affected (10 files)

| Category | Files | Changes |
|----------|-------|---------|
| **API Definition** | `api/workflowexecution/v1alpha1/groupversion_info.go` | 2 lines (Group + annotation) |
| **Controller** | `internal/controller/workflowexecution/workflowexecution_controller.go` | 3 RBAC annotations |
| **CRD Manifest** | `config/crd/bases/` | Regenerate + delete old |
| **E2E Tests** | `test/e2e/workflowexecution/*.go` | ~10-15 occurrences |
| **Integration Tests** | `test/integration/workflowexecution/*.go` | ~5-10 occurrences |
| **Documentation** | `docs/services/crd-controllers/03-workflowexecution/` | ~5-10 occurrences |

**Total Lines Changed**: ~30-50 lines across 10 files

### Risk Assessment

| Risk Factor | Level | Mitigation |
|-------------|-------|------------|
| **Code Complexity** | 🟢 LOW | Mechanical find-replace changes |
| **Test Impact** | 🟡 MEDIUM | E2E tests need manifest updates |
| **Build Impact** | 🟢 LOW | CRD regeneration is automated |
| **Integration Risk** | 🟢 LOW | No cross-team code dependencies |
| **Rollback Complexity** | 🟢 LOW | Git revert possible if needed |

**Overall Risk**: 🟢 **LOW** - Straightforward mechanical change

### Conflict Analysis with Current Work

**BR-WE-006 Kubernetes Conditions** (70% complete):
- ✅ **No Direct Conflicts**: Conditions work doesn't touch API group definitions
- ⚠️ **Merge Conflict Risk**: Both change `workflowexecution_controller.go` (different sections)
- ✅ **Recommendation**: Complete BR-WE-006 first, then migrate API group

**Timing Strategy**:
1. ✅ Finish BR-WE-006 Phases 4-5 (1 hour remaining)
2. ⏸️ Commit BR-WE-006 changes
3. 🔄 Start API group migration (2-3 hours)
4. ✅ Complete before RO E2E coordination

---

## 📝 Detailed Action Plan

### Phase 1: Code Changes (45 minutes)

#### Step 1.1: Update API Definition (5 min)
**File**: `api/workflowexecution/v1alpha1/groupversion_info.go`

```diff
  // Package v1alpha1 contains API Schema definitions for the workflowexecution v1alpha1 API group
  // +kubebuilder:object:generate=true
- // +groupName=kubernaut.ai
+ // +groupName=kubernaut.ai
  package v1alpha1

  import (
      "k8s.io/apimachinery/pkg/runtime/schema"
      "sigs.k8s.io/controller-runtime/pkg/scheme"
  )

  var (
      // GroupVersion is group version used to register these objects
-     GroupVersion = schema.GroupVersion{Group: "kubernaut.ai", Version: "v1alpha1"}
+     GroupVersion = schema.GroupVersion{Group: "kubernaut.ai", Version: "v1alpha1"}

      // SchemeBuilder is used to add go types to the GroupVersionKind scheme
      SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

      // AddToScheme adds the types in this group-version to the given scheme.
      AddToScheme = SchemeBuilder.AddToScheme
  )
```

**Validation**: `go build ./api/workflowexecution/v1alpha1/`

#### Step 1.2: Update Controller RBAC (10 min)
**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`

**Search Pattern**: `kubernaut.ai`

```diff
- //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions,verbs=get;list;watch;create;update;patch;delete
- //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/status,verbs=get;update;patch
- //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=workflowexecutions/finalizers,verbs=update
```

**Validation**: `go build ./internal/controller/workflowexecution/`

#### Step 1.3: Regenerate CRD Manifests (5 min)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Regenerate manifests
make manifests

# Verify new CRD created
ls -la config/crd/bases/kubernaut.ai_workflowexecutions.yaml

# Delete old CRD
rm config/crd/bases/kubernaut.ai_workflowexecutions.yaml
```

**Expected Output**:
- ✅ New file: `config/crd/bases/kubernaut.ai_workflowexecutions.yaml`
- ❌ Old file deleted: `config/crd/bases/kubernaut.ai_workflowexecutions.yaml`

#### Step 1.4: Update E2E Test Manifests (15 min)

**Files to Update**:
```bash
# Find all occurrences
grep -r "kubernaut.ai" test/e2e/workflowexecution/ test/integration/workflowexecution/

# Expected files:
# - test/e2e/workflowexecution/01_lifecycle_test.go
# - test/e2e/workflowexecution/backoff_test.go
# - test/e2e/workflowexecution/resource_locking_test.go
# - test/integration/workflowexecution/*.go
```

**Change Pattern**:
```diff
- apiVersion: kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
```

#### Step 1.5: Update Documentation (10 min)

**Files to Update**:
1. `docs/services/crd-controllers/03-workflowexecution/README.md`
2. `docs/services/crd-controllers/03-workflowexecution/crd-schema.md`
3. `docs/services/crd-controllers/03-workflowexecution/testing-strategy.md`
4. `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`
5. `docs/handoff/SHARED_RO_E2E_TEAM_COORDINATION.md` (WE section)

**Change Pattern**:
```diff
- apiVersion: kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
```

**Search Command**:
```bash
grep -r "kubernaut.ai" docs/services/crd-controllers/03-workflowexecution/ docs/handoff/
```

---

### Phase 2: Testing & Validation (45 minutes)

#### Step 2.1: Build Verification (5 min)
```bash
# Verify all code compiles
make build

# Expected: No errors
```

#### Step 2.2: Unit Tests (10 min)
```bash
# Run unit tests
make test-unit-workflowexecution

# Expected: All tests pass (23 conditions tests + existing tests)
```

#### Step 2.3: Integration Tests (15 min)
```bash
# Run integration tests
make test-integration-workflowexecution

# Expected: All tests pass (7 conditions tests + existing tests)
```

#### Step 2.4: E2E Tests (15 min)
```bash
# Run E2E tests
make test-e2e-workflowexecution

# Expected: All tests pass (lifecycle, backoff, resource locking)
```

**Note**: E2E tests will create new CRD with `kubernaut.ai` group in Kind cluster

---

### Phase 3: Documentation & Commit (30 minutes)

#### Step 3.1: Update Handoff Documents (15 min)

**File 1**: `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`

Add to "Recent Changes" section:
```markdown
### API Group Migration (December 13, 2025)
- ✅ Migrated from `kubernaut.ai/v1alpha1` to `kubernaut.ai/v1alpha1`
- ✅ Aligned with DD-CRD-001 authoritative standard
- ✅ Regenerated CRD manifests and updated all tests
- ✅ Simplified kubectl commands: `kubectl get workflowexecutions.kubernaut.ai`
```

**File 2**: `docs/handoff/SHARED_RO_E2E_TEAM_COORDINATION.md`

Update WorkflowExecution section examples to use `kubernaut.ai/v1alpha1`

#### Step 3.2: Commit Changes (15 min)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Stage all changes
git add .

# Commit with detailed message
git commit -m "refactor: migrate WorkflowExecution CRD to single API group kubernaut.ai

- Update API group from kubernaut.ai to kubernaut.ai
- Align with DD-CRD-001 authoritative standard (2025-12-13 update)
- Regenerate CRD manifests (delete old, create new)
- Update controller RBAC annotations (3 annotations)
- Update E2E test manifests (~15 occurrences)
- Update integration test manifests (~10 occurrences)
- Update documentation (5 files)

Changes:
- api/workflowexecution/v1alpha1/groupversion_info.go (2 lines)
- internal/controller/workflowexecution/workflowexecution_controller.go (3 RBAC annotations)
- config/crd/bases/kubernaut.ai_workflowexecutions.yaml (new)
- config/crd/bases/kubernaut.ai_workflowexecutions.yaml (deleted)
- test/e2e/workflowexecution/*.go (15 occurrences)
- test/integration/workflowexecution/*.go (10 occurrences)
- docs/services/crd-controllers/03-workflowexecution/*.md (5 files)

Testing:
- ✅ Unit tests: All passing
- ✅ Integration tests: All passing
- ✅ E2E tests: All passing
- ✅ Build: Successful
- ✅ Lint: No errors

Ref: docs/handoff/SHARED_APIGROUP_MIGRATION_NOTICE.md
Ref: docs/handoff/WE_TRIAGE_API_GROUP_MIGRATION.md
Closes: API Group Migration for WorkflowExecution"
```

---

## ✅ Validation Checklist

### Go Code
- [ ] `api/workflowexecution/v1alpha1/groupversion_info.go` updated: `Group: "kubernaut.ai"`
- [ ] Kubebuilder annotation updated: `+groupName=kubernaut.ai`
- [ ] Controller RBAC annotations updated: `groups=kubernaut.ai` (3 annotations)
- [ ] Code compiles: `make build`

### CRD Manifests
- [ ] New CRD manifest exists: `config/crd/bases/kubernaut.ai_workflowexecutions.yaml`
- [ ] Old CRD manifest deleted: `config/crd/bases/kubernaut.ai_workflowexecutions.yaml`
- [ ] CRD manifest contains: `group: kubernaut.ai`
- [ ] CRD manifest contains: `spec.group: kubernaut.ai`

### Tests
- [ ] Unit tests pass: `make test-unit-workflowexecution`
- [ ] Integration tests pass: `make test-integration-workflowexecution`
- [ ] E2E tests pass: `make test-e2e-workflowexecution`
- [ ] E2E test manifests updated to `apiVersion: kubernaut.ai/v1alpha1`
- [ ] Integration test manifests updated to `apiVersion: kubernaut.ai/v1alpha1`

### Documentation
- [ ] Service README updated: `docs/services/crd-controllers/03-workflowexecution/README.md`
- [ ] CRD schema doc updated: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md`
- [ ] Testing strategy updated: `docs/services/crd-controllers/03-workflowexecution/testing-strategy.md`
- [ ] Handoff doc updated: `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`
- [ ] RO coordination doc updated: `docs/handoff/SHARED_RO_E2E_TEAM_COORDINATION.md`

### Build & Lint
- [ ] Code compiles: `go build ./...`
- [ ] No lint errors: `golangci-lint run`
- [ ] Manifests regenerate: `make manifests`

---

## 📊 kubectl Command Changes

### Before Migration
```bash
# Verbose syntax
kubectl get workflowexecutions.kubernaut.ai
kubectl describe workflowexecution wfe-example

# Short name (unchanged)
kubectl get we
```

### After Migration
```bash
# Simpler syntax (shorter API group)
kubectl get workflowexecutions.kubernaut.ai
kubectl describe workflowexecution wfe-example

# Short name (unchanged)
kubectl get we

# List all Kubernaut resources (new capability!)
kubectl api-resources | grep kubernaut.ai
# Output will show: workflowexecutions, signalprocessings, aianalyses, etc.
```

---

## 🔗 Dependencies & Coordination

### No Blocking Dependencies
- ✅ **Independent Migration**: WE can migrate without waiting for other teams
- ✅ **No Code Dependencies**: No cross-service Go code affected
- ✅ **E2E Test Independence**: WE E2E tests don't create other service CRDs

### Coordination Points

#### 1. RO E2E Tests (Future)
**When**: RO creates `WorkflowExecution` CRDs in their E2E tests
**Impact**: RO team will need to update their test manifests to use `kubernaut.ai/v1alpha1`
**Timing**: WE migration should complete BEFORE RO E2E coordination work begins

**Action**: Update `docs/handoff/SHARED_RO_E2E_TEAM_COORDINATION.md` to reflect new API group in WE examples

#### 2. Integration Tests (Potential)
**When**: If other services have integration tests that create `WorkflowExecution` CRDs
**Impact**: Minimal - integration tests typically use real controllers
**Action**: Verify no other services' integration tests hardcode `kubernaut.ai`

```bash
# Check for cross-service references
grep -r "kubernaut.ai" test/integration/ | grep -v workflowexecution/

# Expected: No results (WE is typically the end of the pipeline)
```

---

## ⏰ Timeline & Effort

### Estimated Effort Breakdown

| Phase | Task | Estimated Time | Complexity |
|-------|------|----------------|------------|
| **Phase 1** | Code changes | 45 min | 🟢 LOW |
| - | API definition update | 5 min | 🟢 LOW |
| - | Controller RBAC update | 10 min | 🟢 LOW |
| - | CRD regeneration | 5 min | 🟢 LOW |
| - | E2E test manifests | 15 min | 🟡 MEDIUM |
| - | Documentation | 10 min | 🟢 LOW |
| **Phase 2** | Testing & validation | 45 min | 🟡 MEDIUM |
| - | Build verification | 5 min | 🟢 LOW |
| - | Unit tests | 10 min | 🟢 LOW |
| - | Integration tests | 15 min | 🟡 MEDIUM |
| - | E2E tests | 15 min | 🟡 MEDIUM |
| **Phase 3** | Documentation & commit | 30 min | 🟢 LOW |
| - | Handoff docs | 15 min | 🟢 LOW |
| - | Commit & push | 15 min | 🟢 LOW |
| **TOTAL** | | **2 hours** | 🟢 **LOW** |

**Buffer**: +30 minutes for unexpected issues = **2.5 hours total**

### Recommended Timeline

**Option 1: Immediate (After BR-WE-006)**
- ✅ Complete BR-WE-006 Phases 4-5 (1 hour)
- ✅ Commit BR-WE-006
- 🔄 Start API group migration (2.5 hours)
- ✅ Complete migration same day

**Total Time**: ~3.5 hours

**Option 2: Separate Day**
- ✅ Complete BR-WE-006 first (Day 1)
- 🔄 API group migration (Day 2, 2.5 hours)

**Recommendation**: **Option 1** - Complete both in one session to minimize context switching

---

## 🚨 Risk Mitigation

### Potential Issues & Solutions

#### Issue 1: E2E Tests Fail After Migration
**Symptom**: E2E tests can't find CRD or create resources
**Cause**: Kind cluster has old CRD installed
**Solution**:
```bash
# Delete old CRD from Kind cluster
kubectl delete crd workflowexecutions.kubernaut.ai --kubeconfig ~/.kube/workflowexecution-e2e-config

# E2E tests will install new CRD automatically
make test-e2e-workflowexecution
```

#### Issue 2: Integration Tests Reference Old API Group
**Symptom**: Integration tests fail with "no kind is registered for version"
**Cause**: Test code still references old API group
**Solution**:
```bash
# Find all references
grep -r "kubernaut.ai" test/integration/workflowexecution/

# Update to kubernaut.ai/v1alpha1
```

#### Issue 3: Merge Conflicts with BR-WE-006
**Symptom**: Git merge conflicts in `workflowexecution_controller.go`
**Cause**: Both BR-WE-006 and API migration modify same file
**Solution**:
- ✅ Complete BR-WE-006 first
- ✅ Commit and push BR-WE-006
- ✅ Start API migration on clean branch
- ✅ RBAC annotations are at top of file, conditions are in functions (minimal overlap)

#### Issue 4: Documentation Out of Sync
**Symptom**: Examples in docs show old API group
**Cause**: Missed grep results
**Solution**:
```bash
# Comprehensive search
grep -r "kubernaut.ai" docs/ test/ api/ internal/ config/

# Update ALL occurrences to kubernaut.ai
```

---

## 📈 Benefits of Migration

### Immediate Benefits
- ✅ **Simpler kubectl commands**: `kubectl get workflowexecutions.kubernaut.ai` (shorter)
- ✅ **Unified RBAC**: Single API group for all Kubernaut permissions
- ✅ **Clear project identity**: All resources under `kubernaut.ai` umbrella
- ✅ **Easier discovery**: `kubectl api-resources | grep kubernaut.ai` shows all CRDs

### Long-Term Benefits
- ✅ **Industry alignment**: Matches K8sGPT, Prometheus, Cert-Manager patterns
- ✅ **Reduced cognitive load**: 1 API group instead of 7 resource-specific groups
- ✅ **Simplified onboarding**: New developers see unified API group
- ✅ **V1.0 GA readiness**: Aligns with authoritative standards before release

---

## 📞 Support & Escalation

### Questions & Issues

**Primary Contact**: WorkflowExecution Team Lead

**Reference Documents**:
- **Authoritative Standard**: [DD-CRD-001: CRD API Group Domain Selection](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)
- **Shared Migration Notice**: [SHARED_APIGROUP_MIGRATION_NOTICE.md](SHARED_APIGROUP_MIGRATION_NOTICE.md)
- **Detailed Analysis**: [TRIAGE_API_GROUP_NAMING_STRATEGY.md](TRIAGE_API_GROUP_NAMING_STRATEGY.md)

**Escalation Path**:
1. Review this triage document
2. Check shared migration notice FAQ
3. Consult DD-CRD-001 for authoritative guidance
4. Ask questions in team channel

---

## ✅ Acknowledgment

**WorkflowExecution Team Response**:

- [x] **Acknowledged**: WE Team (AI Assistant) on December 13, 2025
- [ ] **Estimated Completion**: After BR-WE-006 complete (Dec 13, 2025 EOD)
- [ ] **Migration Status**: ⏸️ **Not Started** - Will start after BR-WE-006 Phases 4-5

**Rationale for Timing**:
1. ✅ BR-WE-006 is 70% complete (Phases 1-3 done)
2. ⏸️ BR-WE-006 Phases 4-5 remaining (~1 hour)
3. 🔄 API group migration after BR-WE-006 (~2.5 hours)
4. ✅ Total effort: ~3.5 hours, can complete in one session
5. ✅ Minimizes merge conflicts by sequential completion

**Commitment**: ✅ **Will complete both BR-WE-006 and API group migration before RO E2E coordination work**

---

## 📝 Summary

### Decision: ✅ PROCEED with API Group Migration

**Confidence**: 95%

**Justification**:
- ✅ Aligns with DD-CRD-001 authoritative standard
- ✅ Low risk mechanical change (~30-50 lines)
- ✅ Well-defined scope and clear migration path
- ✅ No blocking dependencies on other teams
- ✅ Required for V1.0 GA consistency
- ✅ Timing works with current workload (after BR-WE-006)

**Action Items**:
1. ✅ Complete BR-WE-006 Phases 4-5 first (~1 hour)
2. 🔄 Execute API group migration (~2.5 hours)
3. ✅ Update acknowledgment in SHARED_APIGROUP_MIGRATION_NOTICE.md
4. ✅ Coordinate with RO team on updated API group in E2E scenarios

**Target**: Complete by December 13, 2025 EOD (same day as BR-WE-006)

---

**Document Status**: ✅ Triage Complete, Awaiting Execution
**Created**: 2025-12-13
**Triaged By**: WE Team (AI Assistant)
**Recommendation**: ✅ **PROCEED** - Low risk, high value migration
**File**: `docs/handoff/WE_TRIAGE_API_GROUP_MIGRATION.md`


