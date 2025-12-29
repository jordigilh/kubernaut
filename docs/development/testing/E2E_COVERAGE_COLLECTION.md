# E2E Coverage Collection for Go Services

**Version**: 1.0.0
**Last Updated**: 2025-12-21
**Status**: Active
**Applies To**: All Go CRD controllers and stateless services

---

## 📋 Changelog

### Version 1.0.0 (2025-12-21)
- **INITIAL**: Created E2E coverage collection guide for Go 1.20+ binary profiling
- **ADDED**: Kind cluster configuration with hostPath volume mounts
- **ADDED**: Controller deployment pattern with GOCOVERDIR
- **ADDED**: Coverage extraction and reporting commands
- **ADDED**: Combined coverage report generation (unit + integration + E2E)
- **ADDED**: Troubleshooting guide for common issues
- **ADDED**: CI/CD integration example (GitHub Actions)
- **REFERENCE**: Based on [Go 1.20 Coverage Profiling for Kubernetes Apps](https://www.mgasch.com/2023/02/go-e2e/)

---

## Overview

Go 1.20+ supports coverage profiling for compiled binaries, enabling E2E coverage measurement for controllers running in Kind clusters. This document describes the implementation pattern for kubernaut services.

**Reference**: [Go 1.20 Coverage Profiling for Kubernetes Apps](https://www.mgasch.com/2023/02/go-e2e/)

---

## Coverage Targets

| Tier | Target | Measured Via |
|------|--------|--------------|
| **Unit** | 70%+ (95% critical) | `go test -cover` |
| **Integration** | >50% (CRD controllers) | `go test -cover` |
| **E2E** | 10-15% | `go tool covdata` (this guide) |

---

## Implementation Pattern

### Prerequisites

- Go 1.20+
- Kind cluster
- Controller binary (not container image initially)

### Step 1: Create Coverage Directory

```bash
# Create directory for coverage data
mkdir -p coverdata
chmod 777 coverdata  # Ensure container can write
```

### Step 2: Build Controller with Coverage

```bash
# Build with coverage instrumentation
GOFLAGS=-cover go build -o bin/{service}-controller ./cmd/{service}/

# Verify coverage is enabled
file bin/{service}-controller
# Should show: "ELF 64-bit LSB executable" (not stripped)
```

### Step 3: Kind Cluster Configuration

Create or update `test/infrastructure/kind-{service}-config.yaml`:

```yaml
apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
nodes:
- role: control-plane
- role: worker
  extraMounts:
  # Mount coverage directory from host to Kind node
  - hostPath: ${PWD}/coverdata
    containerPath: /coverdata
    readOnly: false
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
  - containerPort: 30180
    hostPort: 9180
    protocol: TCP
```

### Step 4: Controller Deployment Manifest

Update the controller deployment to:
1. Set `GOCOVERDIR` environment variable
2. Mount the coverage volume

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {service}-controller
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: controller
        image: {service}-controller:e2e
        env:
        - name: GOCOVERDIR
          value: /coverdata
        volumeMounts:
        - name: coverdata
          mountPath: /coverdata
      volumes:
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
```

### Step 5: Load Image to Kind

```bash
# Build container image with coverage-enabled binary
docker build -t {service}-controller:e2e \
  --build-arg GOFLAGS=-cover \
  -f docker/{service}.Dockerfile .

# Load into Kind cluster
kind load docker-image {service}-controller:e2e --name {service}-e2e
```

### Step 6: Run E2E Tests

```bash
# Run E2E test suite
make test-e2e-{service}

# Or directly with Ginkgo
ginkgo -v ./test/e2e/{service}/...
```

### Step 7: Graceful Shutdown (CRITICAL)

Coverage data is written when the binary exits gracefully. Ensure proper shutdown:

```bash
# Scale down controller to trigger graceful exit
kubectl scale deployment {service}-controller -n kubernaut-system --replicas=0

# Wait for pod termination
kubectl wait --for=delete pod -l app={service}-controller -n kubernaut-system --timeout=60s
```

### Step 8: Extract Coverage Report

```bash
# Verify coverage files exist
ls -la coverdata/
# Should show: covcounters.* and covmeta.* files

# Generate summary
go tool covdata percent -i=./coverdata

# Generate detailed text report
go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt

# Generate HTML report
go tool cover -html=e2e-coverage.txt -o e2e-coverage.html

# Open in browser
open e2e-coverage.html  # macOS
xdg-open e2e-coverage.html  # Linux
```

---

## Integration with Test Infrastructure

### Updated E2E Suite Setup

```go
// test/e2e/{service}/suite_test.go

var _ = SynchronizedBeforeSuite(func() []byte {
    // Create coverage directory
    coverDir := filepath.Join(projectRoot, "coverdata")
    os.MkdirAll(coverDir, 0777)

    // Create Kind cluster with coverage mount
    err := infrastructure.CreateClusterWithCoverage(
        clusterName,
        kubeconfigPath,
        coverDir,
        GinkgoWriter,
    )
    Expect(err).ToNot(HaveOccurred())

    // ... rest of setup
}, func(data []byte) {
    // ...
})

var _ = SynchronizedAfterSuite(func() {
    // ... cleanup
}, func() {
    // Graceful shutdown to flush coverage
    GinkgoWriter.Println("Scaling down controller for coverage flush...")
    cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
        "scale", "deployment", "{service}-controller",
        "-n", "kubernaut-system", "--replicas=0")
    cmd.Run()

    // Wait for coverage files
    time.Sleep(5 * time.Second)

    // Generate coverage report
    GinkgoWriter.Println("Generating E2E coverage report...")
    cmd = exec.Command("go", "tool", "covdata", "percent", "-i=./coverdata")
    output, _ := cmd.CombinedOutput()
    GinkgoWriter.Println(string(output))

    // Cleanup cluster
    infrastructure.DeleteCluster(clusterName, kubeconfigPath, GinkgoWriter)
})
```

### Makefile Target

```makefile
# Makefile

.PHONY: test-e2e-{service}-coverage
test-e2e-{service}-coverage:
	@echo "Building controller with coverage..."
	GOFLAGS=-cover go build -o bin/{service}-controller ./cmd/{service}/
	@echo "Running E2E tests..."
	$(MAKE) test-e2e-{service}
	@echo "Generating coverage report..."
	go tool covdata percent -i=./coverdata
	go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt
	go tool cover -html=e2e-coverage.txt -o e2e-coverage.html
	@echo "Coverage report: e2e-coverage.html"
```

---

## Combining Coverage Reports

To get a unified coverage report across all tiers:

```bash
# Run all test tiers with coverage
go test ./test/unit/{service}/... -coverprofile=unit-coverage.out
go test ./test/integration/{service}/... -coverprofile=integration-coverage.out

# Convert E2E coverage to same format
go tool covdata textfmt -i=./coverdata -o e2e-coverage.out

# Merge all coverage files (requires gocovmerge)
go install github.com/wadey/gocovmerge@latest
gocovmerge unit-coverage.out integration-coverage.out e2e-coverage.out > combined-coverage.out

# Generate combined report
go tool cover -html=combined-coverage.out -o combined-coverage.html
go tool cover -func=combined-coverage.out | tail -1  # Total coverage
```

---

## Troubleshooting

### Coverage Files Not Created

**Symptom**: `coverdata/` directory is empty after tests

**Causes & Solutions**:

1. **Binary not built with coverage**
   ```bash
   # Verify: should show coverage metadata
   go tool nm bin/{service}-controller | grep cover
   ```

2. **GOCOVERDIR not set**
   ```bash
   kubectl exec -it {pod} -n kubernaut-system -- env | grep GOCOVERDIR
   ```

3. **Controller not gracefully terminated**
   ```bash
   # Must scale to 0 or send SIGTERM, not SIGKILL
   kubectl scale deployment {service}-controller -n kubernaut-system --replicas=0
   ```

4. **Volume not mounted correctly**
   ```bash
   kubectl exec -it {pod} -n kubernaut-system -- ls -la /coverdata
   ```

### Permission Denied

**Symptom**: `permission denied` writing to `/coverdata`

**Solution**: Ensure container runs as user with write access, or use `securityContext`:

```yaml
securityContext:
  runAsUser: 0  # Run as root for Kind (not for production!)
```

### Coverage Lower Than Expected

**Symptom**: E2E coverage is very low (e.g., <5%)

**Causes**:
- Not all code paths exercised by E2E tests (expected)
- Controller restarted during tests (coverage lost)
- Multiple replicas writing to same file (race condition)

**Solutions**:
- E2E tests should cover critical paths only (10-15% target)
- Use single replica during E2E tests
- Ensure graceful shutdown before extracting coverage

---

## CI/CD Integration

### GitHub Actions Example

```yaml
# .github/workflows/e2e.yml

jobs:
  e2e-coverage:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4

    - name: Setup Go
      uses: actions/setup-go@v5
      with:
        go-version: '1.22'

    - name: Setup Kind
      uses: helm/kind-action@v1
      with:
        config: test/infrastructure/kind-{service}-config.yaml
        cluster_name: {service}-e2e

    - name: Build with Coverage
      run: GOFLAGS=-cover go build -o bin/{service}-controller ./cmd/{service}/

    - name: Run E2E Tests
      run: make test-e2e-{service}

    - name: Extract Coverage
      run: |
        go tool covdata percent -i=./coverdata
        go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt

    - name: Upload Coverage
      uses: codecov/codecov-action@v4
      with:
        files: e2e-coverage.txt
        flags: e2e
```

---

## References

- [Go 1.20 Coverage Profiling for Kubernetes Apps](https://www.mgasch.com/2023/02/go-e2e/)
- [Go Coverage Documentation](https://go.dev/blog/integration-test-coverage)
- [TESTING_GUIDELINES.md](../business-requirements/TESTING_GUIDELINES.md)
- [DD-TEST-001: Port Allocation Strategy](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

