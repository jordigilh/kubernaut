# Webhooks → AuthWebhook Refactoring Plan

**Date**: January 21, 2026
**Status**: 📋 **PLANNED - Awaiting Execution**
**Scope**: Rename `cmd/authwebhook/` → `cmd/authwebhook/` and `pkg/authwebhook/` → `pkg/authwebhook/`

---

## 🎯 **Objective**

Rename webhooks to authwebhook for naming consistency across the codebase.

**Current Inconsistency**:
- Code: `cmd/authwebhook/`, `pkg/authwebhook/`
- Tests: `test/unit/authwebhook/`, `test/integration/authwebhook/`, `test/e2e/authwebhook/`
- CI: `authwebhook`
- Makefile: `test-unit-authwebhook`, `test-integration-authwebhook`

**Goal**: Align code naming with test/CI naming → `authwebhook` everywhere

---

## 📊 **Impact Analysis**

| File Type | Count | Impact Level |
|-----------|-------|--------------|
| **Go files** | 2 | 🔴 **HIGH** - Imports need updates |
| **YAML files** | 3 | 🟡 **MEDIUM** - Deployment manifests |
| **Markdown files** | 65 | 🟢 **LOW** - Documentation updates |

**Total Files Affected**: ~70 files

---

## 🔧 **Refactoring Steps**

### **Phase 1: Move Directories**

```bash
# 1. Move pkg/authwebhook → pkg/authwebhook
git mv pkg/authwebhook pkg/authwebhook

# 2. Move cmd/authwebhook → cmd/authwebhook
git mv cmd/authwebhook cmd/authwebhook
```

**Risk**: Low - Git preserves history with `git mv`

---

### **Phase 2: Update Go Imports** (2 files)

#### **File 1: cmd/authwebhook/main.go** (formerly cmd/authwebhook/main.go)

```diff
--- a/cmd/authwebhook/main.go
+++ b/cmd/authwebhook/main.go
@@ -11,7 +11,7 @@ import (
 	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
 	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
 	"github.com/jordigilh/kubernaut/pkg/audit"
-	"github.com/jordigilh/kubernaut/pkg/authwebhook"
+	"github.com/jordigilh/kubernaut/pkg/authwebhook"

 	"k8s.io/apimachinery/pkg/runtime"
 	ctrl "sigs.k8s.io/controller-runtime"
@@ -126,7 +126,7 @@ func main() {
 	decoder := admission.NewDecoder(scheme)

 	// Register WorkflowExecution handler (DD-WEBHOOK-003: Complete audit events)
-	wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
+	wfeHandler := authwebhook.NewWorkflowExecutionAuthHandler(auditStore)
 	if err := wfeHandler.InjectDecoder(decoder); err != nil {
 		setupLog.Error(err, "failed to inject decoder into WorkflowExecution handler")
 		os.Exit(1)
@@ -135,7 +135,7 @@ func main() {
 	setupLog.Info("Registered WorkflowExecution webhook handler with audit store")

 	// Register RemediationApprovalRequest handler (DD-WEBHOOK-003: Complete audit events)
-	rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(auditStore)
+	rarHandler := authwebhook.NewRemediationApprovalRequestAuthHandler(auditStore)
 	// ... (3 more similar changes)
 ```

#### **File 2: test/integration/authwebhook/suite_test.go**

```diff
--- a/test/integration/authwebhook/suite_test.go
+++ b/test/integration/authwebhook/suite_test.go
@@ -8,7 +8,7 @@ import (
 	. "github.com/onsi/gomega"

 	"github.com/jordigilh/kubernaut/pkg/audit"
-	"github.com/jordigilh/kubernaut/pkg/authwebhook"
+	"github.com/jordigilh/kubernaut/pkg/authwebhook"
 	"github.com/jordigilh/kubernaut/test/infrastructure"
 )
```

**Tool**: `gofmt` and `goimports` will auto-fix formatting

---

### **Phase 3: Update YAML Manifests** (3 files)

Need to identify which YAML files reference webhooks:

```bash
grep -r "cmd/authwebhook\|pkg/authwebhook" --include="*.yaml" -l
```

Likely candidates:
- Deployment manifests
- Kustomize configurations
- Test manifests

---

### **Phase 4: Update Documentation** (65 files)

**Strategy**: Use `find` + `sed` for batch updates

```bash
# Find all markdown files with webhooks references
find . -name "*.md" -exec grep -l "cmd/authwebhook\|pkg/authwebhook" {} \;

# Batch replace cmd/authwebhook → cmd/authwebhook
find . -name "*.md" -exec sed -i '' 's|cmd/authwebhook|cmd/authwebhook|g' {} \;

# Batch replace pkg/authwebhook → pkg/authwebhook
find . -name "*.md" -exec sed -i '' 's|pkg/authwebhook|pkg/authwebhook|g' {} \;
```

**Manual Review Required**: Some docs may need context-aware updates

---

### **Phase 5: Verification**

```bash
# 1. Build succeeds
go build ./cmd/authwebhook

# 2. Tests pass
make test-unit-authwebhook
make test-integration-authwebhook

# 3. No lingering references
grep -r "pkg/authwebhook\|cmd/authwebhook" --include="*.go" .
# Should return 0 results in code

# 4. Run full unit test suite
./scripts/test-all-unit-tests.sh
```

---

## 🎯 **Expected Outcomes**

### **Before Refactoring**
```
cmd/authwebhook/main.go          ❌ Inconsistent with tests/CI
pkg/authwebhook/*.go             ❌ Inconsistent with tests/CI
test/unit/authwebhook/        ✅ Correct naming
test/integration/authwebhook/ ✅ Correct naming
CI: authwebhook               ✅ Correct naming
```

### **After Refactoring**
```
cmd/authwebhook/main.go          ✅ Consistent
pkg/authwebhook/*.go             ✅ Consistent
test/unit/authwebhook/           ✅ Consistent
test/integration/authwebhook/    ✅ Consistent
CI: authwebhook                  ✅ Consistent
```

---

## ⚠️ **Risk Assessment**

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| **Build failures** | Low | High | Automated build verification |
| **Import errors** | Low | High | Only 2 Go files affected |
| **Broken docs** | Medium | Low | Manual review of critical docs |
| **Deployment issues** | Low | Medium | Update manifests before deploy |
| **Git history loss** | Very Low | Medium | Use `git mv` (preserves history) |

**Overall Risk**: 🟢 **LOW** - Well-scoped refactoring with clear boundaries

---

## 📋 **Execution Checklist**

- [ ] **Phase 1**: Move directories with `git mv`
- [ ] **Phase 2**: Update Go imports (2 files)
- [ ] **Phase 3**: Update YAML manifests (3 files)
- [ ] **Phase 4**: Update documentation (65 files)
- [ ] **Phase 5**: Verification
  - [ ] `go build ./cmd/authwebhook` succeeds
  - [ ] `make test-unit-authwebhook` passes
  - [ ] `make test-integration-authwebhook` passes
  - [ ] No lingering `pkg/authwebhook` or `cmd/authwebhook` in Go code
  - [ ] Full unit test suite passes
- [ ] **Phase 6**: Commit with clear message

---

## 📝 **Commit Message Template**

```
refactor: rename webhooks → authwebhook for naming consistency

BREAKING CHANGE: Directory structure updated for semantic clarity

Changes:
- Rename cmd/authwebhook/ → cmd/authwebhook/
- Rename pkg/authwebhook/ → pkg/authwebhook/
- Update all imports (2 Go files)
- Update deployment manifests (3 YAML files)
- Update documentation references (65 MD files)

Rationale:
All webhooks are authentication/authorization webhooks. Previous naming
(cmd/authwebhook, pkg/authwebhook) was inconsistent with test directories
(test/unit/authwebhook/, test/integration/authwebhook/) and CI service
name (authwebhook). This refactoring aligns code naming with the
semantic purpose and existing test/CI infrastructure.

Related: docs/triage/WEBHOOKS_UNIT_TEST_TRIAGE.md
```

---

## 🔗 **Related Documents**

- **Triage**: [WEBHOOKS_UNIT_TEST_TRIAGE.md](./WEBHOOKS_UNIT_TEST_TRIAGE.md)
- **Service Code**: `cmd/authwebhook/main.go` (will become `cmd/authwebhook/main.go`)
- **Business Logic**: `pkg/authwebhook/*.go` (will become `pkg/authwebhook/*.go`)
- **CI Pipeline**: `.github/workflows/ci-pipeline.yml` (already uses `authwebhook`)

---

**Status**: 📋 **READY FOR EXECUTION**

**Estimated Time**: 15-20 minutes
**Confidence**: **95%** - Low-risk refactoring with clear scope

**Next Step**: Execute Phase 1 (Move directories)
