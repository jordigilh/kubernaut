# Notification E2E Coverage Implementation - Progress Report

**Date**: December 23, 2025
**Status**: 🚧 **IN PROGRESS** - Image Loading Fixed, Pod Readiness Issue Remains
**Context**: Implementing E2E coverage collection for Notification service using DD-TEST-008

---

## 🎯 Objective

Run Notification E2E tests with coverage collection and generate comprehensive coverage reports broken down by package and function.

---

## ✅ Achievements

### 1. Reusable E2E Coverage Infrastructure Created
- ✅ Created `scripts/generate-e2e-coverage.sh` (reusable script)
- ✅ Created `Makefile.e2e-coverage.mk` (reusable template)
- ✅ Created `DD-TEST-008-reusable-e2e-coverage-infrastructure.md` (authoritative doc)
- ✅ Updated `DD-TEST-007` to reference DD-TEST-008 prominently
- ✅ Fixed filename case to match naming convention (`DD-TEST-008-reusable...` lowercase)

### 2. Makefile Integration
- ✅ Included `Makefile.e2e-coverage.mk` in main Makefile
- ✅ Added `test-e2e-notification-coverage` target using `define-e2e-coverage-target`
- ✅ Target properly generates coverage collection infrastructure

### 3. Image Build/Load Infrastructure Fixes
- ✅ Fixed image tag generation (was creating invalid tags like `kubernaut/datastorage:kubernaut/datastorage-...`)
  - **Root Cause**: Using full `ImageName` ("kubernaut/datastorage") instead of just `ServiceName` ("datastorage")
  - **Fix**: Use `ServiceName` for both parts of tag generation

- ✅ Fixed podman localhost prefix handling
  - **Root Cause**: Podman automatically adds "localhost/" prefix, but code wasn't using it consistently
  - **Fix**: Build with `localhost/kubernaut/datastorage:...` tag and use same for Kind load

- ✅ Fixed Kind podman provider integration
  - **Root Cause**: Kind's podman provider requires `KIND_EXPERIMENTAL_PROVIDER=podman` environment variable
  - **Fix 1**: Added environment variable to Kind load command
  - **Fix 2**: Switched from `kind load docker-image` to `kind load image-archive` for reliability
  - **Implementation**: Export image to `/tmp/` tar file, load via archive, cleanup tar file

### 4. Build Process Validation
- ✅ Image builds successfully with coverage instrumentation (`GOFLAGS=-cover`)
- ✅ Image loads successfully into Kind cluster
- ✅ PostgreSQL pod becomes ready
- ✅ Redis pod becomes ready

---

## ❌ Current Blocking Issue

### DataStorage Pod Readiness Timeout

**Symptom**:
```
⏳ Waiting for Data Storage Service pod to be ready...
[FAILED] Timed out after 300.001s.
Data Storage Service pod should be ready
```

**Timeline**:
- 00:00 - Start deployment
- ~02:00 - PostgreSQL ready ✅
- ~02:30 - Redis ready ✅
- ~02:45 - Start waiting for DataStorage
- ~07:45 - Timeout after 300s ❌

**Possible Root Causes**:
1. **Coverage Overhead**: Coverage-instrumented binary may be slower to start
2. **GOCOVERDIR Setup**: Coverage directory may not be writable or properly mounted
3. **Resource Constraints**: Kind cluster may need more resources for coverage builds
4. **Probe Timing**: Readiness/liveness probes may need adjustment for coverage mode
5. **Configuration Issue**: E2E-specific config may have problems

**Next Steps to Investigate**:
1. Check DataStorage pod logs: `kubectl logs -n notification-e2e <pod> --kubeconfig ~/.kube/notification-e2e-config`
2. Check pod events: `kubectl describe pod -n notification-e2e <pod> --kubeconfig ~/.kube/notification-e2e-config`
3. Verify GOCOVERDIR mount and permissions
4. Consider increasing readiness probe `initialDelaySeconds` for coverage builds
5. Check if binary actually starts or crashes

---

## 📊 Code Changes Summary

### Files Modified

1. **`Makefile`** (line ~949)
   - Added: `$(eval $(call define-e2e-coverage-target,notification,notification,4))`

2. **`test/infrastructure/datastorage_bootstrap.go`** (lines 774-834)
   - **Change 1**: Fixed image tag generation
     ```go
     // OLD: imageTag := generateInfrastructureImageTag(cfg.ImageName, cfg.ServiceName)
     // NEW: imageTag := generateInfrastructureImageTag(cfg.ServiceName, cfg.ServiceName)
     ```

   - **Change 2**: Fixed localhost prefix handling
     ```go
     localImageName := fmt.Sprintf("localhost/%s", fullImageName)
     ```

   - **Change 3**: Added image archive export/import
     ```go
     // Export to tar
     tmpFile := fmt.Sprintf("/tmp/%s-%s.tar", cfg.ServiceName, imageTag)
     saveCmd := exec.Command("podman", "save", "-o", tmpFile, localImageName)

     // Load from tar with KIND_EXPERIMENTAL_PROVIDER
     loadCmd := exec.Command("kind", "load", "image-archive", tmpFile, "--name", cfg.KindClusterName)
     loadCmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
     ```

3. **`docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md`**
   - Added prominent "Quick Start" section referencing DD-TEST-008
   - Updated "Implementation Checklist" to show DD-TEST-008 as recommended
   - Updated "Related Documents" to highlight DD-TEST-008
   - Bumped version to 1.2.0

4. **`docs/architecture/decisions/DD-TEST-008-reusable-e2e-coverage-infrastructure.md`**
   - Created comprehensive reusable E2E coverage infrastructure documentation

5. **`scripts/generate-e2e-coverage.sh`**
   - Created reusable script for generating coverage reports

6. **`Makefile.e2e-coverage.mk`**
   - Created reusable Makefile template with `define-e2e-coverage-target` function

---

## 🔍 Technical Deep Dive: Image Loading Issues

### Issue 1: Invalid Image Tag Format

**Error**:
```
Error: tag kubernaut/datastorage:kubernaut/datastorage-datastorage-18840220: invalid reference format
```

**Root Cause Analysis**:
The `generateInfrastructureImageTag` function expects two simple string parameters (e.g., "datastorage", "notification"), but was receiving the full image name with repository prefix ("kubernaut/datastorage"), causing the slash to appear in the tag.

**Solution**:
```go
// BEFORE:
imageTag := generateInfrastructureImageTag(cfg.ImageName, cfg.ServiceName)
// Resulted in: "kubernaut/datastorage-datastorage-12345"

// AFTER:
imageTag := generateInfrastructureImageTag(cfg.ServiceName, cfg.ServiceName)
// Results in: "datastorage-datastorage-12345"
```

### Issue 2: Podman Localhost Prefix Mismatch

**Error**:
```
ERROR: image: "kubernaut/datastorage:datastorage-datastorage-18840329" not present locally
```

**Root Cause Analysis**:
Podman automatically prefixes local images with "localhost/" when building, but the code was trying to load "kubernaut/datastorage:..." while the actual image was "localhost/kubernaut/datastorage:...".

**Solution**:
Use the "localhost/" prefix consistently for both build and load operations:
```go
localImageName := fmt.Sprintf("localhost/%s", fullImageName)
buildArgs = []string{"build", "-t", localImageName, ...}
loadCmd := exec.Command("kind", "load", "docker-image", localImageName, ...)
```

### Issue 3: Kind Podman Provider Image Resolution

**Error** (even after Fix #2):
```
using podman due to KIND_EXPERIMENTAL_PROVIDER
ERROR: image: "localhost/kubernaut/datastorage:datastorage-datastorage-18840329" not present locally
```

**Root Cause Analysis**:
Kind's experimental podman provider has issues with `kind load docker-image` even when the image exists in podman's local storage. This appears to be a bug or limitation in Kind's podman integration.

**Solution**:
Use `kind load image-archive` instead, which is more reliable:
1. Export image to tar file: `podman save -o /tmp/image.tar localhost/image:tag`
2. Load tar into Kind: `kind load image-archive /tmp/image.tar --name cluster-name`
3. Cleanup tar file: `os.Remove(tmpFile)`

This approach bypasses Kind's image resolution issues and works reliably with podman.

---

## 📋 Next Actions

### Immediate (To Unblock E2E Coverage)

1. **Investigate DataStorage Pod Failure**
   ```bash
   # Get pod name
   kubectl get pods -n notification-e2e --kubeconfig ~/.kube/notification-e2e-config

   # Check logs
   kubectl logs -n notification-e2e <datastorage-pod> --kubeconfig ~/.kube/notification-e2e-config

   # Check events
   kubectl describe pod -n notification-e2e <datastorage-pod> --kubeconfig ~/.kube/notification-e2e-config
   ```

2. **Verify Coverage Directory Mount**
   ```bash
   kubectl exec -n notification-e2e <datastorage-pod> --kubeconfig ~/.kube/notification-e2e-config -- ls -la /coverdata
   ```

3. **Adjust Probe Timings** (if needed)
   - Increase `initialDelaySeconds` for readiness probe
   - Increase `timeoutSeconds` and `failureThreshold`

4. **Consider Increasing Timeout** (temporary workaround)
   - Current: 300s (5 minutes)
   - Coverage builds may need: 600s (10 minutes)?

### Medium Term (After E2E Coverage Works)

1. **Apply Reusable Infrastructure to Other Services**
   - DataStorage: Replace 45-line custom target
   - WorkflowExecution: Replace custom target
   - SignalProcessing: Replace custom target
   - Gateway: Add coverage target

2. **Create Migration Guide**
   - Document how to migrate from custom to reusable infrastructure
   - Provide before/after examples

3. **Validate Coverage Reports**
   - Ensure text, HTML, and function reports are generated correctly
   - Verify coverage percentages are accurate
   - Test "open e2e-coverage.html" workflow

---

## 🎓 Lessons Learned

### 1. Podman/Kind Integration is Complex
- Podman's localhost prefix behavior differs from Docker
- Kind's podman provider support is experimental and has limitations
- Image archive approach is more reliable than direct image loading

### 2. Environment Variables Matter
- `KIND_EXPERIMENTAL_PROVIDER=podman` is required for Kind to use podman
- Must be set on the specific command, not globally (unless exported)

### 3. Tag Generation Requires Care
- Image names vs. service names are different concepts
- Slashes in tags are invalid and cause build failures
- Use simple strings for tag components

### 4. Reusable Infrastructure Pays Off
- 97% code reduction (45 lines → 1 line per service)
- Single place to fix bugs (benefits all services)
- Consistent behavior and error messages

### 5. Documentation is Critical
- Prominent signposting (Quick Start sections) helps adoption
- Cross-references between related documents prevent confusion
- Naming conventions (lowercase filenames) matter for consistency

---

## 📚 Related Documents

- **DD-TEST-007**: E2E Coverage Capture Standard (technical foundation)
- **DD-TEST-008**: Reusable E2E Coverage Infrastructure (implementation guide)
- **REUSABLE_E2E_COVERAGE_INFRASTRUCTURE_DEC_23_2025.md**: Handoff summary
- **DD_TEST_007_UPDATED_WITH_DD_TEST_008_DEC_23_2025.md**: DD-TEST-007 update summary
- **DD_TEST_008_FILENAME_STANDARDIZATION_DEC_23_2025.md**: Filename case fix summary

---

**Status**: 🚧 Waiting for DataStorage pod readiness issue resolution to proceed with coverage collection



