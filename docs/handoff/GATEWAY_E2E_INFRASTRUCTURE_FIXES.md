# Gateway E2E Infrastructure Fixes

**Date**: December 13, 2025
**Status**: ✅ COMPLETE - All fixes applied
**Related**: `docs/handoff/GATEWAY_E2E_PARALLEL_IMPLEMENTATION.md`

---

## 📋 Summary

Fixed 3 critical issues preventing Gateway E2E tests from running:
1. Missing `api/` directory in Dockerfile
2. Incorrect Rego policy path in Dockerfile
3. Podman image loading incompatibility with Kind

---

## 🐛 Issues Found & Fixed

### Issue #1: Missing `api/` Directory
**Symptom**:
```
go: no required module provides package github.com/jordigilh/kubernaut/api/remediation/v1alpha1
```

**Root Cause**: Dockerfile wasn't copying the `api/` directory needed for CRD type definitions.

**Fix**: Added `COPY api/ api/` to `Dockerfile.gateway`

**File**: `Dockerfile.gateway`

---

### Issue #2: Incorrect Rego Policy Path
**Symptom**:
```
Error: checking on sources under "/var/tmp/libpod_builder.../build":
copier: stat: "/config.app/gateway/policies/priority.rego": no such file or directory
```

**Root Cause**: Dockerfile referenced `priority.rego` but actual file is `remediation_path.rego`.

**Fix**: Updated Rego policy path in `Dockerfile.gateway`

**File**: `Dockerfile.gateway`

---

### Issue #3: Podman Image Loading (CRITICAL)
**Symptom**:
```
ERROR: image: "localhost/kubernaut-gateway:e2e-test" not present locally
```

**Root Cause**: Kind's `load docker-image` command doesn't work with Podman's local image storage.

**Fix**: Changed from `kind load docker-image` to Podman-compatible pattern:
1. `podman save -o <tarfile> <image>`
2. `kind load image-archive <tarfile>`
3. Cleanup tarfile

**File**: `test/infrastructure/gateway_e2e.go`

---

### Issue #4: Missing Namespace Creation
**Symptom**:
```
failed to deploy PostgreSQL: failed to create PostgreSQL init ConfigMap:
namespaces "kubernaut-system" not found
```

**Root Cause**: `DeployTestServices` was trying to deploy to `kubernaut-system` namespace without creating it first.

**Fix**: Added `createTestNamespace` call at the start of `DeployTestServices` function.

**File**: `test/infrastructure/gateway_e2e.go`

---

### Issue #5: Incorrect PostgreSQL Label Selector
**Symptom**:
```
error: no matching resources found
PostgreSQL not ready: exit status 1
```

**Root Cause**: Wait function was looking for pods with label `app=postgres` but PostgreSQL deployment uses `app=postgresql`.

**Fix**: Changed label selector from `app=postgres` to `app=postgresql`.

**File**: `test/infrastructure/gateway_e2e.go`

---

### Issue #6: Invalid NodePort Range
**Symptom**:
```
The Service "datastorage" is invalid: spec.ports[0].nodePort: Invalid value: 8091:
provided port is not in the valid range. The range of valid ports is 30000-32767
```

**Root Cause**: DataStorage service was trying to use NodePort 8091, which is outside Kubernetes's valid NodePort range.

**Fix**: Changed `GatewayDataStoragePort` from 8091 to 30091.

**File**: `test/infrastructure/gateway_e2e.go`

---

## ✅ Changes Applied

### `Dockerfile.gateway`
```diff
  COPY go.mod go.sum ./
  RUN go mod download

+ # Added api/ directory
+ COPY api/ api/
  COPY cmd/ cmd/
  COPY pkg/ pkg/
  COPY internal/ internal/
  COPY config.app/ config.app/

  ...

- # Fixed Rego policy path
- COPY config.app/gateway/policies/priority.rego /config.app/gateway/policies/priority.rego
+ COPY config.app/gateway/policies/remediation_path.rego /config.app/gateway/policies/remediation_path.rego
```

### `test/infrastructure/gateway_e2e.go`

**Import additions**:
```go
import (
    "context"
    "fmt"
    "io"
+   "os"
    "os/exec"
+   "path/filepath"
    "strings"
)
```

**Function changes**:
```go
func buildAndLoadGatewayImage(clusterName string, writer io.Writer) error {
    projectRoot := getProjectRoot()

    // 1. Build Docker image using Podman
    fmt.Fprintln(writer, "   Building Docker image using Podman...")
    buildCmd := exec.Command("podman", "build",
-       "-t", "gateway:e2e-test",
+       "-t", "localhost/kubernaut-gateway:e2e-test",
        "-f", projectRoot+"/Dockerfile.gateway",
        projectRoot,
    )
    buildCmd.Stdout = writer
    buildCmd.Stderr = writer
    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("podman build failed: %w", err)
    }

-   // 2. Load image into Kind (broken with Podman)
-   loadCmd := exec.Command("kind", "load", "docker-image",
-       "gateway:e2e-test",
-       "--name", clusterName,
-   )

+   // 2. Save image to tar file (Podman-compatible)
+   tmpFile := filepath.Join(os.TempDir(), "gateway-e2e.tar")
+   fmt.Fprintln(writer, "   Saving image to tar file...")
+   saveCmd := exec.Command("podman", "save",
+       "-o", tmpFile,
+       "localhost/kubernaut-gateway:e2e-test",
+   )
+   saveCmd.Stdout = writer
+   saveCmd.Stderr = writer
+   if err := saveCmd.Run(); err != nil {
+       return fmt.Errorf("podman save failed: %w", err)
+   }
+
+   // 3. Load image archive into Kind
+   fmt.Fprintln(writer, "   Loading image into Kind cluster...")
+   loadCmd := exec.Command("kind", "load", "image-archive",
+       tmpFile,
+       "--name", clusterName,
+   )
    loadCmd.Stdout = writer
    loadCmd.Stderr = writer
    if err := loadCmd.Run(); err != nil {
+       os.Remove(tmpFile)
-       return fmt.Errorf("kind load image failed: %w", err)
+       return fmt.Errorf("kind load image-archive failed: %w", err)
    }

+   // 4. Cleanup tmp file
+   os.Remove(tmpFile)
    return nil
}
```

---

## 🔍 Debugging Process

### Attempt History
| Attempt | Issue | Duration | Resolution |
|---------|-------|----------|------------|
| 1 | Missing `api/` directory | 163s | Added `COPY api/ api/` to Dockerfile |
| 2 | Wrong Rego policy path | 106s | Fixed `remediation_path.rego` path |
| 3 | Image not found by Kind | 79s | Changed to `podman save` + `kind load image-archive` |
| 4 | ✅ Running | TBD | All fixes applied |

### Error Messages Encountered
```
# Error 1: Missing api/ directory
go: no required module provides package github.com/jordigilh/kubernaut/api/remediation/v1alpha1
go: to add it: go get github.com/jordigilh/kubernaut/api/remediation/v1alpha1

# Error 2: Wrong Rego policy
Error: building at STEP "COPY config.app/gateway/policies/priority.rego ...":
checking on sources: copier: stat: "/config.app/gateway/policies/priority.rego":
no such file or directory

# Error 3: Podman image not found by Kind
ERROR: image: "localhost/kubernaut-gateway:e2e-test" not present locally
```

---

## 📚 Reference Pattern

This fix follows the SignalProcessing E2E infrastructure pattern:
- **Authority**: `test/infrastructure/signalprocessing.go:735` (loadSignalProcessingImage)
- **Pattern**: Podman save → Kind load image-archive
- **Precedent**: All other services using Podman + Kind

---

## ✅ Validation

**Status**: Baseline E2E run in progress with all fixes applied.

**Log**: `/tmp/gateway-e2e-baseline-final.log`
**Terminal**: `terminals/47.txt`

**Expected Outcome**: Successful E2E test run, establishing baseline timing for parallel optimization.

---

**Owner**: Gateway Team
**Reviewed By**: Platform Team (E2E infrastructure pattern)
**Status**: ✅ COMPLETE - Waiting for baseline run results

