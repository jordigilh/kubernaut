# Integration Test `setup-envtest` Inconsistency Triage

**Date**: 2026-01-22
**Issue**: Inconsistent `KUBEBUILDER_ASSETS` / `setup-envtest` patterns across CRD controller integration test suites
**Severity**: Low (functional but inconsistent)
**Impact**: Developer experience, test reliability, IDE compatibility

---

## 🔍 **Root Cause Analysis**

### **Discovered Inconsistency**

CRD controller integration test suites use **TWO DIFFERENT PATTERNS** for setting up envtest binaries:

#### **Pattern 1: Dynamic CLI (NEWER)** - 2 services
```go
// Runtime download/discovery using setup-envtest CLI
if os.Getenv("KUBEBUILDER_ASSETS") == "" {
    cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
    output, err := cmd.Output()
    if err != nil {
        logf.Log.Error(err, "Failed to get KUBEBUILDER_ASSETS path")
        Expect(err).ToNot(HaveOccurred(), "Should get KUBEBUILDER_ASSETS path from setup-envtest")
    }
    assetsPath := strings.TrimSpace(string(output))
    _ = os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
    GinkgoWriter.Printf("[Process %d] 📍 Set KUBEBUILDER_ASSETS: %s\n", GinkgoParallelProcess(), assetsPath)
}
```

**Services using this**:
1. ✅ **AuthWebhook** (`test/integration/authwebhook/suite_test.go:152-162`)
2. ✅ **Gateway** (`test/integration/gateway/suite_test.go:164-173`)
3. ✅ **Gateway/Processing** (`test/integration/gateway/processing/suite_test.go:74-84`)

#### **Pattern 2: Static Pre-installed (OLDER)** - 5 services
```go
// Uses pre-installed binaries in bin/k8s/
testEnv = &envtest.Environment{
    CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
    ErrorIfCRDPathMissing: true,
}

// Retrieve the first found binary directory to allow running tests from IDEs
if getFirstFoundEnvTestBinaryDir() != "" {
    testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
}
```

**Supporting function**:
```go
func getFirstFoundEnvTestBinaryDir() string {
    basePath := filepath.Join("..", "..", "..", "bin", "k8s")
    entries, err := os.ReadDir(basePath)
    if err != nil {
        logf.Log.Error(err, "Failed to read directory", "path", basePath)
        return ""
    }
    for _, entry := range entries {
        if entry.IsDir() {
            return filepath.Join(basePath, entry.Name())
        }
    }
    return ""
}
```

**Services using this**:
1. ✅ **AIAnalysis** (`test/integration/aianalysis/suite_test.go:363-371`)
2. ✅ **SignalProcessing** (`test/integration/signalprocessing/suite_test.go:263-271`)
3. ✅ **RemediationOrchestrator** (`test/integration/remediationorchestrator/suite_test.go:211-219`)
4. ✅ **WorkflowExecution** (`test/integration/workflowexecution/suite_test.go:158-169`)
5. ✅ **Notification** (`test/integration/notification/suite_test.go:557-575`)

---

## 📊 **Pattern Comparison**

| Aspect | Pattern 1: Dynamic CLI | Pattern 2: Static `bin/k8s/` |
|---|---|---|
| **Setup Requirement** | None (auto-downloads) | `make setup-envtest` first |
| **IDE Compatibility** | ✅ Excellent (auto-setup) | ⚠️ Requires pre-setup |
| **CI Compatibility** | ✅ Excellent (self-contained) | ✅ Good (Makefile handles it) |
| **Cold Start Time** | ⚠️ Slower (downloads once) | ✅ Faster (pre-installed) |
| **Parallel Safety** | ✅ Safe (per-process download) | ✅ Safe (shared readonly binaries) |
| **Failure Mode** | Network failure, GitHub rate limit | Missing `bin/k8s/` directory |
| **Code Complexity** | Simple (5 lines) | Moderate (helper function) |
| **Version Control** | CLI version controlled via `@latest` | Version controlled via Makefile |
| **Maintenance** | ✅ Self-maintaining | ⚠️ Requires Makefile updates |

---

## 🕵️ **Historical Context**

### **Timeline of Pattern Evolution**

1. **Original Pattern (2024-2025)**: All services used `getFirstFoundEnvTestBinaryDir()` (Pattern 2)
   - Required `make setup-envtest` before running tests
   - Good for CI, poor for IDE development

2. **Gateway Migration (Late 2025)**: Gateway switched to dynamic CLI (Pattern 1)
   - Improved IDE experience (zero setup)
   - Introduced first inconsistency

3. **AuthWebhook Migration (Jan 2026)**: AuthWebhook adopted Gateway's pattern
   - Followed Gateway's lead for consistency
   - But other services remained unchanged

### **Why the Inconsistency Exists**

1. **Incremental Migration**: Services migrated to new pattern as they were refactored
2. **No Enforcement**: No rule/guideline documented for which pattern to use
3. **Both Work**: Neither pattern is "broken", so no urgency to unify
4. **Developer Preference**: Different developers favor different approaches

---

## 🎯 **Recommended Solution**

### **Option A: Standardize on Dynamic CLI (Pattern 1)** ⭐ **RECOMMENDED**

**Rationale**:
- ✅ **Best IDE Experience**: Zero-setup for developers
- ✅ **Self-Contained**: No dependency on Makefile targets
- ✅ **Modern Approach**: Aligns with kubebuilder best practices
- ✅ **Parallel-Safe**: Each process downloads to its own cache
- ⚠️ **Slightly Slower**: First run downloads binaries (~5-10s)
- ⚠️ **Network Dependency**: Requires internet on first run

**Migration Steps**:
1. Remove `getFirstFoundEnvTestBinaryDir()` function from each service
2. Add dynamic CLI setup code before `testEnv.Start()`
3. Remove `BinaryAssetsDirectory` assignment
4. Update service documentation

**Files to Modify**:
- `test/integration/aianalysis/suite_test.go`
- `test/integration/signalprocessing/suite_test.go`
- `test/integration/remediationorchestrator/suite_test.go`
- `test/integration/workflowexecution/suite_test.go`
- `test/integration/notification/suite_test.go`

### **Option B: Standardize on Static `bin/k8s/` (Pattern 2)**

**Rationale**:
- ✅ **Faster Cold Start**: No network calls
- ✅ **Offline Friendly**: Works without internet after setup
- ✅ **Version Controlled**: Makefile controls exact version
- ⚠️ **Poor IDE Experience**: Requires manual setup step
- ⚠️ **Maintenance Overhead**: Makefile updates needed

**Migration Steps**:
1. Add `getFirstFoundEnvTestBinaryDir()` to AuthWebhook and Gateway
2. Replace dynamic CLI code with static setup
3. Update documentation to require `make setup-envtest` first

**Files to Modify**:
- `test/integration/authwebhook/suite_test.go`
- `test/integration/gateway/suite_test.go`
- `test/integration/gateway/processing/suite_test.go`

### **Option C: Hybrid Approach (Fallback Pattern)**

**Rationale**:
- ✅ **Best of Both**: Try static first, fallback to dynamic
- ✅ **Maximum Compatibility**: Works in all scenarios
- ⚠️ **Complexity**: More code to maintain
- ⚠️ **Unclear Pattern**: What's the "right" way?

**Implementation**:
```go
// Try static binaries first (fast path)
if getFirstFoundEnvTestBinaryDir() != "" {
    testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
} else if os.Getenv("KUBEBUILDER_ASSETS") == "" {
    // Fallback to dynamic CLI (slow path)
    cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
    // ... setup code
}
```

---

## 🚀 **Implementation Plan (Option A - Recommended)**

### **Phase 1: Create Shared Helper** (15 min)
1. Create `test/shared/envtest/setup.go` with shared setup function:
```go
package envtest

import (
    "os"
    "os/exec"
    "strings"

    . "github.com/onsi/gomega"
    logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// EnsureKubebuilderAssets sets KUBEBUILDER_ASSETS if not already set
// Uses setup-envtest CLI to discover/download binaries automatically
func EnsureKubebuilderAssets() {
    if os.Getenv("KUBEBUILDER_ASSETS") != "" {
        return // Already set
    }

    cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
    output, err := cmd.Output()
    if err != nil {
        logf.Log.Error(err, "Failed to get KUBEBUILDER_ASSETS path")
        Expect(err).ToNot(HaveOccurred(), "Should get KUBEBUILDER_ASSETS path from setup-envtest")
    }
    assetsPath := strings.TrimSpace(string(output))
    _ = os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
}
```

### **Phase 2: Migrate Services** (60 min)

**For each service**:
1. Add import: `testenvtest "github.com/jordigilh/kubernaut/test/shared/envtest"`
2. Replace setup code with: `testenvtest.EnsureKubebuilderAssets()`
3. Remove `getFirstFoundEnvTestBinaryDir()` function
4. Remove `testEnv.BinaryAssetsDirectory` assignment
5. Run tests to verify

**Service Migration Order** (by dependency):
1. ✅ AIAnalysis (no dependencies)
2. ✅ SignalProcessing (no dependencies)
3. ✅ RemediationOrchestrator (depends on all)
4. ✅ WorkflowExecution (no dependencies)
5. ✅ Notification (no dependencies)

### **Phase 3: Documentation** (15 min)
1. Update `README.md` testing section
2. Document pattern in `.cursor/rules/03-testing-strategy.mdc`
3. Add note to service READMEs

### **Phase 4: Validation** (30 min)
1. Run all integration tests in parallel: `make test-integration`
2. Test IDE execution (GoLand, VS Code)
3. Test fresh clone scenario (no `bin/k8s/` directory)

---

## 📝 **Success Criteria**

✅ **Consistency**: All CRD controller integration tests use the same pattern
✅ **IDE Compatibility**: Tests run from IDE without manual setup
✅ **CI Compatibility**: Tests pass in CI without regression
✅ **Parallel Safety**: Tests run safely with `-p --procs=4`
✅ **Documentation**: Pattern documented in rules and READMEs

---

## 🔗 **Related Documentation**

- [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc) - Defense-in-depth testing approach
- [DD-TEST-010](mdc:docs/architecture/decisions/DD-TEST-010-multi-controller-architecture.md) - Multi-controller architecture
- [Gateway suite_test.go](mdc:test/integration/gateway/suite_test.go) - Reference implementation (Pattern 1)
- [Kubebuilder Envtest Docs](https://book.kubebuilder.io/reference/envtest.html)

---

## 🎯 **Decision Required**

**Recommendation**: **Option A - Standardize on Dynamic CLI (Pattern 1)**

**Approvals Needed**:
- [ ] User confirms recommendation
- [ ] User approves implementation plan
- [ ] User approves service migration order

---

**Next Steps**: Awaiting user decision on which option to implement.
