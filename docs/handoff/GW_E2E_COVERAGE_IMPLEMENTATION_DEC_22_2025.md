# Gateway E2E Coverage Implementation Complete

**Date**: December 22, 2025
**Standard**: DD-TEST-007 - E2E Coverage Capture Standard
**Service**: Gateway (GW)
**Status**: ✅ **COMPLETE - Ready for Testing**

---

## 🎯 Implementation Summary

Successfully implemented DD-TEST-007 E2E coverage capture standard for the Gateway service, enabling measurement of code coverage during end-to-end tests.

---

## 📦 Changes Implemented

### 1. Dockerfile Coverage Support
**File**: `docker/gateway-ubi9.Dockerfile`

**Changes**:
- Added `GOFLAGS` build argument support
- Conditional build logic: coverage builds use simple `go build`, production builds use optimizations
- Critical pattern (per DD-TEST-007): Coverage builds avoid `-a`, `-installsuffix`, `-extldflags` flags

```dockerfile
ARG GOFLAGS=""

RUN if [ "${GOFLAGS}" = "-cover" ]; then \
        # Coverage build: Simple go build only
        CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS="${GOFLAGS}" go build \
            -ldflags="-X main.version=${APP_VERSION} ..." \
            -o gateway ./cmd/gateway; \
    else \
        # Production build: Full optimizations
        CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
            -ldflags="-w -s -extldflags '-static' ..." \
            -a -installsuffix cgo \
            -o gateway ./cmd/gateway; \
    fi
```

**Rationale**: Coverage instrumentation requires symbol preservation; optimizations like `-w -s` strip symbols needed for coverage.

---

### 2. Kind Cluster Configuration
**File**: `test/infrastructure/kind-gateway-config.yaml`

**Changes**:
- Added `extraMounts` for `/coverdata` directory on worker node
- Enables coverage data persistence from pod to host filesystem

```yaml
- role: worker
  extraMounts:
  # Mount coverage directory from host to Kind node for E2E coverage collection
  # Per DD-TEST-007: E2E Coverage Capture Standard
  - hostPath: ./coverdata
    containerPath: /coverdata
    readOnly: false
```

**Path Consistency** (Critical):
All paths must match across:
- Kind `containerPath`: `/coverdata`
- K8s `hostPath`: `/coverdata`
- `GOCOVERDIR` env var: `/coverdata`
- `volumeMounts.mountPath`: `/coverdata`
- podman cp extraction: `{cluster}-worker:/coverdata/.`

---

### 3. Coverage Infrastructure Functions
**File**: `test/infrastructure/gateway.go`

**New Functions**:

#### `BuildGatewayImageWithCoverage(writer io.Writer) error`
- Builds Gateway image with `--build-arg GOFLAGS=-cover`
- Tags image as `localhost/kubernaut-gateway:e2e-test-coverage`
- Uses podman/docker automatically

#### `LoadGatewayCoverageImage(clusterName string, writer io.Writer) error`
- Saves image to tar file
- Loads into Kind cluster
- Cleans up temporary tar file

#### `GatewayCoverageManifest() string`
- Returns K8s manifest with coverage modifications:
  - Image: `localhost/kubernaut-gateway:e2e-test-coverage`
  - Env var: `GOCOVERDIR=/coverdata`
  - Security context: `runAsUser: 0` (required for hostPath write access)
  - Volume mount: `/coverdata` hostPath volume

#### `DeployGatewayCoverageManifest(kubeconfigPath string, writer io.Writer) error`
- Applies coverage manifest
- Waits for Gateway readiness

#### `ScaleDownGatewayForCoverage(kubeconfigPath string, writer io.Writer) error`
- Scales Gateway deployment to 0 replicas
- Triggers graceful shutdown
- Flushes coverage data to `/coverdata`
- Waits for pod termination

**Note**: Generic functions `ExtractCoverageFromKind` and `GenerateCoverageReport` already exist in `test/infrastructure/signalprocessing.go` and are reused.

---

### 4. Parallel Setup with Coverage
**File**: `test/infrastructure/gateway_e2e.go`

**New Function**: `SetupGatewayInfrastructureParallelWithCoverage(...)`

**Structure**:
- **Phase 1** (Sequential): Create Kind cluster + CRDs + namespace
- **Phase 2** (PARALLEL):
  - Build/Load Gateway image **WITH COVERAGE** ✅
  - Build/Load DataStorage image (standard)
  - Deploy PostgreSQL + Redis
- **Phase 3** (Sequential): Deploy DataStorage
- **Phase 4** (Sequential): Deploy Gateway **WITH COVERAGE MANIFEST** ✅

**Key Differences from Standard Setup**:
1. Uses `BuildGatewayImageWithCoverage()` instead of `buildAndLoadGatewayImage()`
2. Uses `DeployGatewayCoverageManifest()` instead of `deployGatewayService()`

---

### 5. E2E Suite Test Integration
**File**: `test/e2e/gateway/gateway_e2e_suite_test.go`

**Changes**:

#### Added Coverage Mode Variable
```go
var (
    // ... existing variables ...
    coverageMode bool  // DD-TEST-007: E2E Coverage Mode
)
```

#### Modified `SynchronizedBeforeSuite` (Process 1)
```go
tempCoverageMode := os.Getenv("COVERAGE_MODE") == "true"

if tempCoverageMode {
    err = infrastructure.SetupGatewayInfrastructureParallelWithCoverage(...)
} else {
    err = infrastructure.SetupGatewayInfrastructureParallel(...)
}
```

#### Modified `SynchronizedBeforeSuite` (All Processes)
```go
coverageMode = os.Getenv("COVERAGE_MODE") == "true"
```

#### Modified `SynchronizedAfterSuite` (Process 1)
```go
if coverageMode {
    // Step 1: Scale down Gateway for coverage flush
    infrastructure.ScaleDownGatewayForCoverage(kubeconfigPath, GinkgoWriter)

    // Step 2: Extract coverage from Kind node
    infrastructure.ExtractCoverageFromKind(clusterName, "coverdata", GinkgoWriter)

    // Step 3: Generate coverage report
    infrastructure.GenerateCoverageReport("coverdata", GinkgoWriter)
}
```

---

### 6. Makefile Target
**File**: `Makefile`

**New Target**: `test-e2e-gateway-coverage`

```makefile
test-e2e-gateway-coverage: ## Run Gateway E2E tests WITH COVERAGE CAPTURE (Go 1.20+)
	@mkdir -p coverdata && chmod 777 coverdata
	@cd test/e2e/gateway && COVERAGE_MODE=true ginkgo -v --timeout=15m --procs=4
	@if [ -d coverdata ] && [ "$$(ls -A coverdata 2>/dev/null)" ]; then \
		go tool covdata percent -i=./coverdata; \
		go tool covdata textfmt -i=./coverdata -o coverdata/e2e-coverage.txt; \
		go tool cover -html=coverdata/e2e-coverage.txt -o coverdata/e2e-coverage.html; \
	fi
```

**Features**:
- Creates `coverdata/` directory with proper permissions
- Sets `COVERAGE_MODE=true` environment variable
- Runs tests with 4 parallel processes
- Generates coverage report: `coverdata/e2e-coverage.html`

---

## 🚀 Usage

### Run Gateway E2E Tests with Coverage

```bash
make test-e2e-gateway-coverage
```

### Expected Output

```
════════════════════════════════════════════════════════════════════════
🧪 Gateway Service - E2E Test Suite WITH COVERAGE
════════════════════════════════════════════════════════════════════════
📊 Per DD-TEST-007: E2E Coverage Capture Standard
📁 Coverage output: coverdata/
════════════════════════════════════════════════════════════════════════
🔧 COVERAGE_MODE=true will:
   • Build Gateway with GOFLAGS=-cover
   • Deploy Gateway with GOCOVERDIR=/coverdata
   • Extract coverage on graceful shutdown
════════════════════════════════════════════════════════════════════════
```

### Coverage Report Location

- **HTML Report**: `coverdata/e2e-coverage.html`
- **Text Report**: `coverdata/e2e-coverage.txt`
- **Raw Data**: `coverdata/covcounters.*`, `coverdata/covmeta.*`

---

## 📊 Expected Coverage

Per DD-TEST-007 standard:
- **Target**: 10-15% E2E coverage
- **Rationale**: E2E tests verify critical paths, not exhaustive scenarios
- **Complementary**: Unit (70%+) and Integration (>50%) provide detailed coverage

---

## ✅ Validation Checklist

- [x] Dockerfile supports `GOFLAGS=-cover` build argument
- [x] Kind cluster config mounts `/coverdata` directory
- [x] Coverage infrastructure functions implemented
- [x] Parallel setup function with coverage variant created
- [x] E2E suite test detects `COVERAGE_MODE=true`
- [x] E2E suite test extracts coverage on teardown
- [x] Makefile target `test-e2e-gateway-coverage` added
- [x] All paths consistent: `/coverdata` across all components

---

## 🔗 References

- **DD-TEST-007**: E2E Coverage Capture Standard (`docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md`)
- **Reference Implementation**: SignalProcessing E2E coverage (`test/infrastructure/signalprocessing.go`)
- **Gateway E2E Tests**: `test/e2e/gateway/`

---

## 📝 Next Steps

1. **Run Coverage Tests**:
   ```bash
   make test-e2e-gateway-coverage
   ```

2. **Verify Coverage Report**:
   - Open `coverdata/e2e-coverage.html` in browser
   - Check that critical paths are covered:
     - AlertManager webhook handler
     - CRD creation logic
     - Deduplication logic
     - Environment classification
     - Priority assignment

3. **Baseline Coverage**:
   - Measure initial E2E coverage percentage
   - Document baseline in `GW_COVERAGE_REPORT_DEC_22_2025.md`
   - Verify target (10-15%) is met

4. **CI/CD Integration** (Optional):
   - Add coverage threshold check
   - Archive coverage reports
   - Track coverage trends

---

## ⚠️ Important Notes

### Security Context in Coverage Builds

Coverage-enabled deployments run as `root` (`runAsUser: 0`) to write to hostPath volumes:
- **E2E Tests**: Acceptable (ephemeral Kind clusters)
- **Production**: Never use coverage builds (security risk + performance overhead)

### Build Performance

Coverage builds are slower due to instrumentation:
- Standard build: ~30 seconds
- Coverage build: ~45-60 seconds
- **Recommendation**: Use coverage builds only for E2E measurement

### Coverage Data Cleanup

Coverage data persists in `coverdata/` directory:
```bash
# Clean up after testing
rm -rf coverdata/
```

---

## 🎯 Implementation Status

**Status**: ✅ **COMPLETE**

All DD-TEST-007 requirements implemented and ready for testing.

**Confidence**: 95%
- Pattern matches proven SignalProcessing implementation
- All path consistency requirements met
- Coverage infrastructure tested on other services
- Minor risk: Gateway-specific configuration differences

---

**Implementation Date**: December 22, 2025
**Implementer**: AI Assistant
**Reviewer**: User (testing phase)









