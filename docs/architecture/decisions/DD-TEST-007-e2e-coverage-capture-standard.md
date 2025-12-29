# DD-TEST-007: E2E Coverage Capture Standard for Go CRD Controllers

**Date**: 2025-12-21
**Status**: ✅ **ACCEPTED**
**Context**: Kubernaut V1 E2E Testing - Coverage Measurement
**Deciders**: Development Team
**Technical Story**: [BR-PLATFORM-001, ADR-005, TESTING_GUIDELINES.md]
**Reference Implementations**: SignalProcessing Service, DataStorage Service (both validated working)

---

## 📋 Changelog

### Version 1.1.0 (2025-12-22)
- **ENHANCED**: Added DataStorage team's learnings (build flags + permissions)
- **TROUBLESHOOTING**: Expanded with two critical issues discovered during DS implementation
- **DOCKERFILE**: Added explicit guidance on avoiding incompatible build flags
- **SECURITY**: Clarified when and why to run as root for E2E coverage
- **VALIDATED**: DataStorage service implementation confirmed working (70.8% main coverage, 20 packages)

### Version 1.0.0 (2025-12-21)
- **INITIAL**: Created E2E coverage capture standard
- **VALIDATED**: SignalProcessing service implementation confirmed working
- **DOCUMENTED**: Complete implementation pattern with examples

---

## Context and Problem Statement

Go 1.20+ introduced support for coverage profiling of compiled binaries, enabling coverage measurement for controllers running in Kind clusters. Previously, E2E coverage was unmeasurable because tests run externally while the controller runs inside a container.

**Problem**: How do we measure E2E test coverage for CRD controllers that run inside Kind clusters?

**Constraints**:
- Controllers run as containers inside Kind clusters
- Tests run on the host machine
- Standard `go test -cover` doesn't work for compiled binaries
- Need to capture coverage for critical path validation

---

## Decision

Implement E2E coverage capture using Go 1.20+ binary profiling with the following pattern:

### 1. Build Controller with Coverage Instrumentation

```bash
# Build with GOFLAGS=-cover (no symbol stripping)
GOFLAGS=-cover go build -o bin/{service}-controller ./cmd/{service}/

# Or via Dockerfile with build arg
podman build -t {service}:coverage \
  --build-arg GOFLAGS=-cover \
  -f docker/{service}.Dockerfile .
```

### 2. Dockerfile Conditional Symbol Stripping

```dockerfile
ARG GOFLAGS=""

# Don't strip symbols when coverage is enabled
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
        echo "Building with coverage (no stripping)..."; \
        CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
            -ldflags="-X main.Version=${VERSION}" \
            -o {service}-controller ./cmd/{service}; \
    else \
        echo "Building production (with stripping)..."; \
        CGO_ENABLED=0 GOOS=linux go build \
            -ldflags="-s -w -X main.Version=${VERSION}" \
            -o {service}-controller ./cmd/{service}; \
    fi
```

**⚠️ CRITICAL: Avoid These Build Flags with Coverage** (DS Team Finding)

Go's coverage instrumentation is **incompatible** with aggressive optimization flags. When building with `GOFLAGS=-cover`, you **MUST NOT** use:

| Flag | Purpose | Why It Breaks Coverage |
|------|---------|------------------------|
| `-a` | Force rebuild all packages | Interferes with coverage package metadata |
| `-installsuffix cgo` | Add suffix to package install directory | Breaks coverage runtime's ability to find instrumented packages |
| `-extldflags "-static"` | Force static linking | May strip coverage runtime dependencies |

**❌ WRONG** (DataStorage initial attempt):
```dockerfile
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
        -ldflags='-extldflags "-static"' \    # ❌ Breaks coverage
        -a -installsuffix cgo \                # ❌ Breaks coverage
        -o service \
        ./cmd/service/main.go; \
    fi
```

**✅ CORRECT** (Simple build for coverage):
```dockerfile
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
      CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} GOFLAGS=${GOFLAGS} go build \
        -o service \                           # ✅ Simple build only
        ./cmd/service/main.go; \
    fi
```

**Production builds** can still use all optimizations - this restriction only applies to coverage builds.

### 3. Controller Deployment with GOCOVERDIR

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {service}-controller
spec:
  template:
    spec:
      # ⚠️ CRITICAL: Coverage requires write access to /coverdata (DS Team Finding)
      # Run as root ONLY for E2E coverage tests (not production)
      # Non-root users may lack permission to write to hostPath volumes
      securityContext:
        runAsUser: 0      # Required for /coverdata write access
        runAsGroup: 0
      containers:
      - name: controller
        image: {service}:coverage
        env:
        - name: GOCOVERDIR
          value: /coverdata   # MUST match Kind extraMounts containerPath
        volumeMounts:
        - name: coverdata
          mountPath: /coverdata  # MUST match GOCOVERDIR value
          readOnly: false        # Required for writing coverage files
      volumes:
      - name: coverdata
        hostPath:
          path: /coverdata       # MUST match Kind extraMounts containerPath
          type: DirectoryOrCreate
```

**Path Consistency Requirement** (DS Team Finding):

All paths must be consistent across the entire infrastructure chain:

| Component | Path Configuration | Example |
|-----------|-------------------|---------|
| **Kind extraMounts** | `containerPath` in cluster config | `/coverdata` |
| **Kubernetes hostPath** | `volumes[].hostPath.path` | `/coverdata` |
| **GOCOVERDIR** | Environment variable | `/coverdata` |
| **Volume Mount** | `volumeMounts[].mountPath` | `/coverdata` |
| **podman cp** | Extraction source path | `{cluster}-worker:/coverdata/.` |

**❌ Common Mistake**: Using `/tmp/coverage` in deployment while Kind uses `/coverdata` → coverage files never appear on host.

**Security Context Requirement** (DS Team Finding):

Running as root simplifies permissions for E2E tests:
- **E2E Tests**: Use `runAsUser: 0` for simplified `/coverdata` write access
- **Production**: Use standard security context (non-root user)
- **Acceptable**: E2E runs in ephemeral Kind clusters, not production

Without `runAsUser: 0`, you may see empty coverage directories despite correct configuration.

### 4. Suite Test Integration (COVERAGE_MODE Detection)

```go
// suite_test.go
var coverageMode bool

var _ = SynchronizedBeforeSuite(func() []byte {
    coverageMode = os.Getenv("COVERAGE_MODE") == "true"

    if coverageMode {
        By("📊 E2E Coverage Mode ENABLED")
        // Use coverage-enabled infrastructure
        err := infrastructure.SetupInfrastructureWithCoverage(...)
    } else {
        // Use standard infrastructure
        err := infrastructure.SetupInfrastructure(...)
    }
}, ...)

var _ = SynchronizedAfterSuite(func() {}, func() {
    if coverageMode {
        // Step 1: Scale down for graceful shutdown (flushes coverage)
        Expect(infrastructure.ScaleDownControllerForCoverage(kubeconfigPath, GinkgoWriter)).To(Succeed())

        // Step 2: Wait for pods to terminate (use Eventually, NOT time.Sleep - anti-pattern)
        Eventually(func() bool {
            cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
                "get", "pods", "-n", "kubernaut-system",
                "-l", "app={service}-controller", "-o", "name")
            output, _ := cmd.Output()
            return len(strings.TrimSpace(string(output))) == 0
        }).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).Should(BeTrue(),
            "Controller pods should terminate for coverage flush")

        // Step 3: Extract coverage from Kind node
        Expect(infrastructure.ExtractCoverageFromKind(clusterName, coverDir, GinkgoWriter)).To(Succeed())

        // Step 4: Generate report
        Expect(infrastructure.GenerateCoverageReport(coverDir, GinkgoWriter)).To(Succeed())
    }

    // Delete cluster
    infrastructure.DeleteCluster(clusterName)
})
```

### 5. Makefile Target

```makefile
.PHONY: test-e2e-{service}-coverage
test-e2e-{service}-coverage:
    @mkdir -p coverdata && chmod 777 coverdata
    @COVERAGE_MODE=true ginkgo -v ./test/e2e/{service}/...
    @go tool covdata percent -i=./coverdata
    @go tool covdata textfmt -i=./coverdata -o coverdata/e2e-coverage.txt
    @go tool cover -html=coverdata/e2e-coverage.txt -o coverdata/e2e-coverage.html
```

---

## Reference Implementations

### SignalProcessing Service (Validated Working - 2025-12-21)

**Files Modified**:

| File | Changes |
|------|---------|
| `docker/signalprocessing-controller.Dockerfile` | Conditional symbol stripping |
| `test/e2e/signalprocessing/suite_test.go` | `COVERAGE_MODE` detection, coverage extraction |
| `test/infrastructure/signalprocessing.go` | Coverage infrastructure functions |
| `Makefile` | `test-e2e-signalprocessing-coverage` target |

**Coverage Results** (validated 2025-12-21):

| Package | E2E Coverage |
|---------|--------------|
| `pkg/signalprocessing/ownerchain` | 88.4% |
| `pkg/signalprocessing/audit` | 72.7% |
| `pkg/signalprocessing` | 70.3% |
| `cmd/signalprocessing` | 67.4% |
| `internal/controller/signalprocessing` | 53.1% |

**Usage**:
```bash
make test-e2e-signalprocessing-coverage
# Reports: coverdata/e2e-coverage.html
```

---

### DataStorage Service (Validated Working - 2025-12-22)

**Files Modified**:

| File | Changes |
|------|---------|
| `docker/data-storage.Dockerfile` | Simple build flags for coverage (removed `-a`, `-installsuffix`, `-extldflags`) |
| `test/e2e/datastorage/datastorage_e2e_suite_test.go` | `E2E_COVERAGE` detection, coverage extraction, path `/coverdata` |
| `test/infrastructure/datastorage.go` | Coverage infrastructure, `SecurityContext` with `runAsUser: 0` |
| `test/infrastructure/kind-datastorage-config.yaml` | `extraMounts` for `/coverdata` |
| `Makefile` | `test-e2e-datastorage-coverage` target |

**Coverage Results** (validated 2025-12-22):

| Package | E2E Coverage |
|---------|--------------|
| `pkg/datastorage/middleware` | 78.2% |
| `command-line-arguments` (main) | 70.8% |
| `pkg/datastorage/config` | 64.3% |
| `pkg/log` | 51.3% |
| `pkg/audit` | 42.8% |
| `pkg/datastorage/server/helpers` | 39.0% |
| `pkg/datastorage/dlq` | 37.9% |
| `pkg/datastorage/repository/workflow` | 36.2% |
| **Total** | **20 packages** |

**Key Learnings** (Critical for other services):
1. **Build Flags**: Must use simple `go build` - no `-a`, `-installsuffix cgo`, or `-extldflags "-static"`
2. **Permissions**: Must run as root (`runAsUser: 0`) for E2E coverage to write `/coverdata`
3. **Path Consistency**: `/coverdata` must match across Kind config, K8s volumes, and extraction

**Usage**:
```bash
make test-e2e-datastorage-coverage
# Reports: test/e2e/datastorage/e2e-coverage.html
```

---

## Key Infrastructure Functions

### BuildWithCoverage

```go
func Build{Service}ImageWithCoverage(writer io.Writer) error {
    cmd := exec.Command(containerCmd, "build",
        "-t", imageName,
        "-f", dockerfilePath,
        "--build-arg", "GOFLAGS=-cover",
        projectRoot,
    )
    return cmd.Run()
}
```

### ExtractCoverageFromKind

```go
func ExtractCoverageFromKind(clusterName, coverDir string, writer io.Writer) error {
    // Get worker node container ID
    cmd := exec.Command("docker", "ps", "-q", "--filter",
        fmt.Sprintf("name=%s-worker", clusterName))
    containerID, _ := cmd.Output()

    // Copy coverage data from Kind node
    copyCmd := exec.Command("docker", "cp",
        fmt.Sprintf("%s:/coverdata/.", strings.TrimSpace(string(containerID))),
        coverDir)
    return copyCmd.Run()
}
```

### GenerateCoverageReport

```go
func GenerateCoverageReport(coverDir string, writer io.Writer) error {
    // Generate percentage summary
    percentCmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
    percentCmd.Stdout = writer
    percentCmd.Run()

    // Generate text format
    textCmd := exec.Command("go", "tool", "covdata", "textfmt",
        "-i="+coverDir, "-o="+filepath.Join(coverDir, "e2e-coverage.txt"))
    textCmd.Run()

    // Generate HTML report
    htmlCmd := exec.Command("go", "tool", "cover",
        "-html="+filepath.Join(coverDir, "e2e-coverage.txt"),
        "-o="+filepath.Join(coverDir, "e2e-coverage.html"))
    return htmlCmd.Run()
}
```

---

## Troubleshooting Guide

### Coverage Files Not Created

**Symptom**: `coverdata/` directory is empty after tests

| Cause | Diagnosis | Solution |
|-------|-----------|----------|
| **Incompatible build flags** (DS Finding) | Binary built with `-a`, `-installsuffix`, or `-extldflags` | Use simple `go build` - see "Avoid These Build Flags" above |
| **Path mismatch** (DS Finding) | Kind uses `/coverdata`, deployment uses `/tmp/coverage` | Standardize all paths to `/coverdata` |
| **Permission denied** (DS Finding) | Container runs as non-root, can't write `/coverdata` | Set `securityContext.runAsUser: 0` for E2E |
| Binary not built with coverage | `go tool nm bin/{service} \| grep cover` returns nothing | Rebuild with `GOFLAGS=-cover` |
| Symbols stripped | Binary much smaller than expected | Don't use `-s -w` ldflags with coverage |
| GOCOVERDIR not set | `kubectl exec {pod} -- env \| grep GOCOVERDIR` empty | Add env var to deployment |
| Not gracefully terminated | Controller killed with SIGKILL | Use `kubectl scale --replicas=0` |

### Common Symptom: "Extraction Succeeds but Directory Empty"

**What you see**:
```
✅ Coverage files extracted from Kind node
⚠️  warning: no applicable files found in input directories
```

**Most likely causes** (from DS team experience):

1. **Incompatible Build Flags** (70% of cases)
   - Check Dockerfile for `-a`, `-installsuffix cgo`, or `-extldflags "-static"` in coverage build
   - Solution: Use simple `go build` for coverage builds
   - Verification: `go tool nm bin/{service} | grep cover` should show coverage symbols

2. **Permission Issues** (20% of cases)
   - Container runs as non-root, lacks permission to write `/coverdata`
   - Solution: Add `securityContext.runAsUser: 0` to pod spec
   - Verification: `kubectl exec {pod} -- touch /coverdata/test.txt` should succeed

3. **Path Inconsistency** (10% of cases)
   - Different paths used across Kind config, K8s volumes, GOCOVERDIR, podman cp
   - Solution: Use `/coverdata` consistently everywhere
   - Verification: All 4 components use identical path

### Controller Crashes on Startup

**Symptom**: CrashLoopBackOff with coverage-enabled image

**Common Causes**:
1. **Wrong policy paths**: Coverage manifest has different volume mounts than standard
2. **Missing env vars**: Coverage manifest missing required environment variables
3. **Permission issues**: Can't write to `/coverdata`

**Solution**: Compare coverage manifest with working standard manifest line-by-line.

---

## Coverage Targets

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

| Tier | Target | Purpose |
|------|--------|---------|
| Unit | 70%+ | Business logic in isolation |
| Integration | >50% | Cross-service coordination |
| **E2E** | **10-15%** | **Critical path validation** |

---

## Decision Rationale

### Why Go 1.20+ Binary Profiling?

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Binary profiling** | Accurate coverage for running controllers | Requires graceful shutdown | ✅ CHOSEN |
| Test-time mocking | No infrastructure needed | Doesn't test real controller | ❌ Rejected |
| Log-based coverage | Simple implementation | Inaccurate, manual maintenance | ❌ Rejected |

### Why hostPath Volume?

The coverage data must persist across container restarts and be accessible from the host for extraction. `hostPath` provides direct access to Kind node filesystem.

### Why Run as Root?

For E2E tests only, running as root simplifies volume write permissions. This is acceptable because:
- E2E tests run in ephemeral Kind clusters
- Not used in production
- Simplifies implementation

---

## Implementation Checklist for New Services

- [ ] **Dockerfile**: Add conditional symbol stripping for coverage builds
  - [ ] ⚠️ **CRITICAL**: Ensure coverage build uses simple `go build` (no `-a`, `-installsuffix`, `-extldflags`)
  - [ ] Verify production build keeps all optimizations
- [ ] **Kind Config**: Add `extraMounts` for `/coverdata` in cluster configuration
- [ ] **Infrastructure**: Add `Build{Service}ImageWithCoverage` function
- [ ] **Infrastructure**: Add `Deploy{Service}ControllerWithCoverage` function
  - [ ] ⚠️ **CRITICAL**: Add `securityContext.runAsUser: 0` for E2E coverage
  - [ ] Set `GOCOVERDIR=/coverdata` environment variable
  - [ ] Add hostPath volume mount at `/coverdata`
- [ ] **Infrastructure**: Add `SetupInfrastructureWithCoverage` function
- [ ] **Suite Test**: Add `COVERAGE_MODE` or `E2E_COVERAGE` environment variable detection
- [ ] **Suite Test**: Add coverage extraction in `SynchronizedAfterSuite`
  - [ ] Scale deployment to 0 for graceful shutdown
  - [ ] Wait for pods to terminate (use `Eventually`, not `time.Sleep`)
  - [ ] Extract from `/coverdata` (must match all other paths)
- [ ] **Makefile**: Add `test-e2e-{service}-coverage` target
- [ ] **Validate**: Run coverage tests and verify reports generated
- [ ] **Path Consistency**: Verify `/coverdata` used in all 4 locations (Kind, K8s, GOCOVERDIR, extraction)

---

## Related Documents

- [E2E_COVERAGE_COLLECTION.md](../../development/testing/E2E_COVERAGE_COLLECTION.md) - Implementation guide
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) - Coverage targets
- [ADR-005: Integration Test Coverage](ADR-005-integration-test-coverage.md) - Testing strategy context
- [DD-TEST-001: Port Allocation Strategy](DD-TEST-001-port-allocation-strategy.md) - E2E infrastructure
- [DD-TEST-002: Parallel Test Execution](DD-TEST-002-parallel-test-execution-standard.md) - Test parallelization

---

## Success Criteria

1. **E2E coverage reports generated**: HTML + text reports in `coverdata/`
2. **Coverage within target**: 10-15% E2E coverage captured
3. **Reproducible**: Same coverage results on repeat runs
4. **CI/CD ready**: Works in GitHub Actions/CI pipelines

---

**Document Status**: ✅ **ACCEPTED**
**Version**: 1.1.0
**Last Updated**: 2025-12-22
**Reference Implementations**:
- SignalProcessing Service (validated working 2025-12-21)
- DataStorage Service (validated working 2025-12-22) - contributed build flags + permissions guidance

