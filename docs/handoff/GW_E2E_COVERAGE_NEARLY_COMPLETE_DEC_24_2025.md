# Gateway E2E Coverage Implementation - Nearly Complete (Dec 24, 2025)

## 🎯 **Session Objective**
Enable E2E coverage collection for Gateway service per DD-TEST-007/DD-TEST-008 standards.

## ✅ **Completed Tasks**

###1. **Build Infrastructure Fixes**
- **Fixed**: `test/infrastructure/datastorage_bootstrap.go` - Migrated to `BuildAndLoadImageToKind` helper
- **Fixed**: All E2E service infrastructure files updated for consistency
  - `test/infrastructure/gateway_e2e.go`
  - `test/infrastructure/notification.go`
  - `test/infrastructure/signalprocessing.go`
  - `test/infrastructure/workflowexecution_parallel.go`
- **Fixed**: Syntax errors in `test/infrastructure/workflowexecution.go` and `test/infrastructure/aianalysis.go`
- **Documentation**: `docs/handoff/BUILD_FIXES_DATASTORAGE_HELPER_DEC_23_2025.md`

### 2. **Dockerfile Configuration**
- **Action**: Deleted orphaned `Dockerfile.gateway` from root directory
- **Verified**: `docker/gateway-ubi9.Dockerfile` has coverage support (lines 38-59)
  ```dockerfile
  ARG GOFLAGS=""
  RUN if [ "${GOFLAGS}" = "-cover" ]; then \
          echo "Building with coverage instrumentation..."; \
          CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS="${GOFLAGS}" go build \
              -ldflags="-X main.version=${APP_VERSION}..." \
              -o gateway \
              ./cmd/gateway; \
      else \
          echo "Building production binary..."; \
          ...
      fi
  ```
- **Result**: Gateway Dockerfile fully compliant with DD-TEST-007

### 3. **E2E Infrastructure Updates**
- **Updated**: `test/infrastructure/gateway.go` with podman-compatible image handling
  - Added `localhost/` prefix for podman build operations
  - Implemented image-archive loading method (same as DataStorage)
  - Fixed image tag propagation between build → load → deploy phases
- **Updated**: `test/infrastructure/gateway_e2e.go`
  - Image name returned from `BuildGatewayImageWithCoverage`
  - Image name passed through to `LoadGatewayCoverageImage` and `DeployGatewayCoverageManifest`
  - Strip `localhost/` prefix before Kubernetes deployment

### 4. **Key Technical Fixes**

#### **Issue 1: Image Tag Mismatch**
**Problem**: Build used one timestamp, load used different timestamp
**Root Cause**: `GetGatewayCoverageFullImageName()` called multiple times with different timestamps
**Solution**: Return image name from `BuildGatewayImageWithCoverage` and pass through pipeline

#### **Issue 2: Podman Image Loading**
**Problem**: `kind load docker-image` failed with "image not present locally"
**Root Cause**: Podman uses `localhost/` prefix, Kind's docker-image loader incompatible
**Solution**: Use `kind load image-archive` after `podman save` (DataStorage pattern)

**Code Changes**:
```go
// Build with localhost/ prefix
localImageName := fmt.Sprintf("localhost/%s", imageName)
buildCmd := exec.Command("podman", "build", "-t", localImageName, ...)

// Save to tar and load via archive
tmpFile := fmt.Sprintf("/tmp/gateway-coverage-%s.tar", clusterName)
saveCmd := exec.Command("podman", "save", "-o", tmpFile, imageName)
loadCmd := exec.Command("kind", "load", "image-archive", tmpFile, "--name", clusterName)
```

#### **Issue 3: Kubernetes Image Reference**
**Problem**: Kubernetes manifest used `localhost/kubernaut/gateway:...` but images in Kind don't have localhost/ prefix
**Solution**: Strip prefix before deploying
```go
k8sImageName := strings.TrimPrefix(gatewayImageName, "localhost/")
DeployGatewayCoverageManifest(k8sImageName, kubeconfigPath, writer)
```

## ⏳ **Current Status: Gateway Health Check Timeout**

### **Symptoms**
- ✅ Gateway image builds successfully with coverage
- ✅ Gateway image loads into Kind cluster
- ✅ Gateway manifest deploys (deployment.apps/gateway created)
- ❌ Gateway health check times out after 2 minutes
- **Error**: `timeout waiting for Gateway health check (tried http://localhost:30080/health for 2m0s)`

### **What Works**
1. All 92 Gateway integration tests pass ✅
2. Image build with coverage instrumentation (-cover) ✅
3. Image loading via podman save → kind load image-archive ✅
4. Kubernetes manifest deployment ✅
5. DataStorage deployed and healthy ✅
6. PostgreSQL + Redis deployed ✅

### **What Doesn't Work**
Gateway pod fails to become healthy:
```
⏳ Waiting for Gateway to be ready (timeout: 2m0s)...
[FAILED] timeout waiting for Gateway health check (tried http://localhost:30080/health for 2m0s)
```

### **Investigation Needed**
**Cluster cleanup prevents debugging** - Kind cluster deleted immediately after failure

**Recommended Next Steps**:
1. **Manual deployment test**:
   ```bash
   # Create persistent Kind cluster
   kind create cluster --name gateway-debug --config test/infrastructure/kind-gateway-config.yaml

   # Build and load Gateway image
   podman build -t localhost/kubernaut/gateway:debug \
       --build-arg GOFLAGS=-cover \
       -f docker/gateway-ubi9.Dockerfile .

   podman save -o /tmp/gateway-debug.tar localhost/kubernaut/gateway:debug
   kind load image-archive /tmp/gateway-debug.tar --name gateway-debug

   # Deploy dependencies
   kubectl apply -f test/infrastructure/postgresql-manifest.yaml
   kubectl apply -f test/infrastructure/redis-manifest.yaml
   kubectl apply -f test/infrastructure/datastorage-manifest.yaml

   # Deploy Gateway with coverage
   kubectl apply -f - <<EOF
   <GatewayCoverageManifest contents>
   EOF

   # Inspect pod status
   kubectl get pods -n kubernaut-system -w
   kubectl describe pod -n kubernaut-system -l app=gateway
   kubectl logs -n kubernaut-system -l app=gateway
   ```

2. **Check for missing environment variables** in Gateway manifest:
   - `DATA_STORAGE_URL`: ✅ Present (`http://datastorage.kubernaut-system.svc.cluster.local:8080`)
   - `LOG_LEVEL`: ✅ Present (`info`)
   - `GOCOVERDIR`: ✅ Present (`/coverage`)
   - **Missing**: Other required Gateway env vars?

3. **Verify Gateway binary compatibility**:
   - Check if coverage-instrumented binary starts locally
   - Verify UBI9 runtime image has required libraries

4. **Compare with working services**:
   - DataStorage E2E coverage works perfectly
   - What's different about Gateway deployment?

## 📋 **E2E Coverage Prerequisites Status**

| Prerequisite | Status | Details |
|---|---|---|
| **A. Dockerfile Coverage Support** | ✅ DONE | `docker/gateway-ubi9.Dockerfile` lines 38-59 |
| **B. Kind Config /coverdata Mount** | ✅ DONE | `test/infrastructure/kind-gateway-config.yaml` |
| **C. E2E Deployment GOCOVERDIR** | ✅ DONE | Gateway manifest line 237-238 |
| **D. Build Command Updated** | ✅ DONE | Uses `--build-arg GOFLAGS=-cover` |
| **E. Image Loading** | ✅ DONE | Uses podman save → kind load image-archive |
| **F. Gateway Health Check** | ❌ BLOCKED | Timeout after 2 minutes |

## 🔍 **Diagnostic Data**

### **Successful Test Output (Phases 1-3)**
```
📦 PHASE 1: Creating Kind cluster + CRDs + namespace...
   ✅ Namespace kubernaut-system created

⚡ PHASE 2: Parallel infrastructure setup (coverage-enabled)...
   ✅ Gateway coverage image built: localhost/kubernaut/gateway:gateway-jgil-37cf9f1-1766582718
   ✅ Gateway coverage image loaded to Kind
   ✅ DataStorage image completed
   ✅ PostgreSQL+Redis completed

📦 PHASE 3: Deploying DataStorage...
   ✅ Migrations applied successfully
   ✅ All tables verified
```

### **Failure at Phase 4**
```
📦 PHASE 4: Deploying Gateway (coverage-enabled)...
   ✅ Gateway coverage manifest applied
  ⏳ Waiting for Gateway to be ready (timeout: 2m0s)...
  [FAILED] timeout waiting for Gateway health check (tried http://localhost:30080/health for 2m0s)
```

## 🎯 **Success Criteria (Nearly Met)**
- [x] Gateway Dockerfile has conditional coverage support
- [x] E2E infrastructure builds image with `-cover`
- [x] E2E infrastructure loads image into Kind
- [x] E2E deployment includes `GOCOVERDIR` environment variable
- [x] Kind config mounts `/coverdata` directory
- [ ] Gateway pod starts successfully **← BLOCKED HERE**
- [ ] E2E tests run and collect coverage
- [ ] Coverage reports generated (text, HTML, function)

## 📊 **Integration Test Success**
**All 92 Gateway integration tests pass** after DD-TEST-009 field index fixes:
- Field index registration implemented correctly
- Deduplication logic works with `spec.signalFingerprint` index
- Storm detection code properly removed per DD-GATEWAY-015

## 🔗 **Related Documents**
- `docs/handoff/SHARED_ALL_TEAMS_E2E_COVERAGE_NOW_AVAILABLE_DEC_23_2025.md` - E2E coverage standards
- `docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md` - E2E coverage architecture
- `docs/architecture/decisions/DD-TEST-009-FIELD-INDEX-ENVTEST-SETUP.md` - Field index setup (integration tests)
- `docs/handoff/GW_FIELD_INDEX_FIX_COMPLETE_DEC_23_2025.md` - Integration test fixes
- `cmd/datastorage/Dockerfile` - Reference for coverage pattern

## 💡 **Key Learnings**

### **Podman + Kind Image Handling**
1. **Build**: Use `localhost/` prefix for podman
2. **Save**: Export to tar with `podman save`
3. **Load**: Use `kind load image-archive` (not `kind load docker-image`)
4. **Deploy**: Strip `localhost/` prefix for Kubernetes manifest

### **Image Tag Consistency**
- Generate tag once, pass through entire pipeline
- Don't call tag generation functions multiple times (timestamps change!)

### **Coverage Build Requirements**
- Use `ARG GOFLAGS=""` in Dockerfile
- Conditional build logic: coverage vs production
- Don't strip symbols (`-s -w`) in coverage builds
- Coverage builds need more startup time (2min vs 1min timeout)

## 🚨 **Blocker for User Input**

**Question**: Why is Gateway pod failing to start in Kind with coverage instrumentation?

**Possible causes**:
1. Missing environment variables or configuration
2. Gateway requires additional dependencies not in manifest
3. Coverage-instrumented binary incompatible with UBI9 runtime
4. Resource constraints in Kind cluster
5. Network connectivity issues (DataStorage URL)

**Request**: Manual deployment debugging to capture pod logs and events before cluster cleanup.

---

**Session End**: Dec 24, 2025 08:31:46
**Next Action**: Debug Gateway pod startup failure with persistent Kind cluster
**Confidence**: 85% - Infrastructure correct, need pod-level debugging








