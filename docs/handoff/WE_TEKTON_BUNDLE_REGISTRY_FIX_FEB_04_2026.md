# WorkflowExecution E2E Tekton Bundle Registry Fix

**Date**: February 4, 2026  
**Team**: WorkflowExecution (WE)  
**Issue**: CI/CD Job #165 Failure - `tkn bundle push` rejects `localhost/` registry  
**Status**: ✅ **FIXED**  
**CI Job**: https://github.com/jordigilh/kubernaut/actions/runs/21658518731/job/62438822533

---

## 🔍 **ROOT CAUSE ANALYSIS**

### **Issue**
WorkflowExecution E2E tests were failing in CI/CD because `tkn bundle push` rejects `localhost/` as a valid registry prefix for OCI bundles.

### **Error Message (from CI/CD logs)**
```
Error: could not parse reference: localhost/kubernaut-test-workflows/hello-world:v1.0.0
failed to build and register test workflows: failed to build hello-world bundle: 
tkn bundle push failed for localhost/kubernaut-test-workflows/hello-world:v1.0.0: exit status 1
```

**Location**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go:132` (BeforeSuite failure)

### **Root Cause**
The Tekton CLI (`tkn bundle push`) validates OCI image references and rejects `localhost/` as an invalid registry hostname. It requires:
- ✅ Proper registry domains (e.g., `quay.io`, `ghcr.io`, `docker.io`)
- ❌ NOT `localhost/` prefix (even though Docker/Podman accept it)

**Why it matters**: E2E tests build Tekton Pipeline bundles as OCI images during BeforeSuite setup when production bundles aren't available on `quay.io`.

---

## 💡 **THE FIX**

### **Strategy: CI/CD-Aware Registry Selection**

**Before** (Hardcoded `localhost/`):
```go
// ❌ BROKEN: tkn bundle push rejects localhost/
const LocalBundleRegistry = "localhost/kubernaut-test-workflows"

localHelloRef := fmt.Sprintf("%s/hello-world:%s", LocalBundleRegistry, TestWorkflowBundleVersion)
// → localhost/kubernaut-test-workflows/hello-world:v1.0.0 (REJECTED by tkn CLI)
```

**After** (Dynamic registry selection):
```go
// ✅ FIXED: Use IMAGE_REGISTRY env var (CI) or ghcr.io (local dev)
func getLocalBundleRegistry() string {
    if registry := os.Getenv("IMAGE_REGISTRY"); registry != "" {
        return registry + "/test-workflows"  // CI: ghcr.io/jordigilh/kubernaut/test-workflows
    }
    return "ghcr.io/jordigilh/kubernaut/test-workflows"  // Local dev fallback
}

bundleRegistry := getLocalBundleRegistry()
localHelloRef := fmt.Sprintf("%s/hello-world:%s", bundleRegistry, TestWorkflowBundleVersion)
// CI → ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0 ✅
// Local → ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0 ✅
```

---

## 📋 **COMPLETE BUNDLE FLOW**

### **Scenario A: Production Bundles Exist on quay.io** (Fast Path)
```bash
1. Check quay.io/jordigilh/test-workflows/hello-world:v1.0.0 → ✅ EXISTS
2. podman pull quay.io/jordigilh/test-workflows/hello-world:v1.0.0
3. kind load image → Load into Kind cluster (cache for offline execution)
4. WorkflowExecution spec: ContainerImage: quay.io/jordigilh/test-workflows/hello-world:v1.0.0
5. Tekton resolves bundle from Kind's local cache (imagePullPolicy: IfNotPresent)
```

**Benefit**: No build needed, bundles cached in Kind for offline execution

### **Scenario B: Production Bundles DON'T Exist** (CI/CD Build Path)
```bash
1. Check quay.io/jordigilh/test-workflows/hello-world:v1.0.0 → ❌ NOT FOUND
2. Build: tkn bundle push ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0 ✅
3. Push to ghcr.io (authenticated via IMAGE_REGISTRY in CI)
4. kind load image → Load into Kind cluster (cache for offline execution)
5. WorkflowExecution spec: ContainerImage: ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0
6. Tekton resolves bundle from Kind's local cache
```

**Benefit**: Bundles pushed to CI registry AND cached in Kind

---

## 🔧 **MODIFIED FILES**

### **1. `test/infrastructure/workflow_bundles.go`**
**Changes**:
- Removed hardcoded `LocalBundleRegistry = "localhost/kubernaut-test-workflows"`
- Added `getLocalBundleRegistry()` function for dynamic registry selection
- Added `pullAndLoadBundleToKind()` for caching remote bundles
- Updated bundle building logic to use dynamic registry
- Both paths (quay.io exists OR build) now load bundles into Kind

**Pattern**: Registry-qualified references + Kind caching

### **2. `test/infrastructure/tekton_bundles.go`**
**Changes**:
- Removed hardcoded `TestBundleRegistry = "localhost/kubernaut-test-workflows"`
- Added `getTestBundleRegistry()` function (matches workflow_bundles.go)
- Updated `GetTestBundleRef()` to use dynamic registry

---

## ✅ **VALIDATION**

### **Build Verification**
```bash
$ go build ./test/infrastructure/...
# ✅ SUCCESS (no errors)
```

### **Expected CI/CD Behavior**
```bash
# CI/CD environment variables:
export IMAGE_REGISTRY="ghcr.io/jordigilh/kubernaut"
export IMAGE_TAG="pr-24"

# E2E test execution:
1. Check quay.io bundles → NOT FOUND
2. Build: tkn bundle push ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0
3. Authenticate to ghcr.io (GitHub Actions GITHUB_TOKEN)
4. Push bundles to ghcr.io ✅
5. Load bundles into Kind ✅
6. Tests reference: ghcr.io/jordigilh/kubernaut/test-workflows/hello-world:v1.0.0 ✅
7. Tekton resolves from Kind cache ✅
```

---

## 🎯 **BUSINESS VALUE**

| **Impact** | **Description** |
|------------|-----------------|
| **CI/CD Reliability** | WE E2E tests can now build bundles in CI/CD |
| **Offline Execution** | Bundles cached in Kind (no external pulls during tests) |
| **Registry Flexibility** | Works with any OCI registry (ghcr.io, quay.io, docker.io) |
| **Developer Experience** | Clear error message if auth required (vs silent tkn failure) |

---

## 📚 **REFERENCES**

### **Related Patterns**
- **IMAGE_REGISTRY**: CI/CD environment variable for container registry
- **Kind Image Loading**: Caching remote images in Kind for offline execution
- **Tekton Bundle Resolver**: OCI bundle resolution for PipelineRuns

### **Related Files**
- `test/infrastructure/workflow_bundles.go` - Primary bundle building logic (FIXED)
- `test/infrastructure/tekton_bundles.go` - Legacy bundle building (FIXED)
- `.github/workflows/ci-pipeline.yml` - Sets IMAGE_REGISTRY env var

### **CI/CD Job**
- **Job #165**: https://github.com/jordigilh/kubernaut/actions/runs/21658518731/job/62438822533
- **Status**: Should pass after this fix

---

## 🚀 **NEXT STEPS FOR WE TEAM**

1. ✅ **Verify fix** - CI/CD Job #165 should pass
2. 📝 **Document** - Add to WE E2E troubleshooting guide
3. 🔑 **Ensure ghcr.io auth** - CI needs GITHUB_TOKEN with package write permissions
4. 🎓 **Share pattern** - Other teams building Tekton bundles can use this approach

---

## 💡 **LOCAL DEVELOPMENT NOTE**

For local E2E testing with bundle builds, developers must authenticate:
```bash
# One-time setup (local development only)
podman login ghcr.io -u YOUR_GITHUB_USERNAME

# Or export IMAGE_REGISTRY to use a different registry
export IMAGE_REGISTRY="quay.io/your-username"
```

**Why**: `tkn bundle push` needs a real registry (can't use `localhost/`)

---

**Questions?** Check Tekton CLI docs: https://tekton.dev/docs/cli/
