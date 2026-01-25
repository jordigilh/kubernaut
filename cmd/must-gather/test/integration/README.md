# Kubernaut Must-Gather - Integration & E2E Tests

Integration and end-to-end tests for must-gather functionality.

## Prerequisites

### Unit Tests (Bats)

```bash
# Install bats-core
# macOS
brew install bats-core

# Linux (Ubuntu/Debian)
sudo apt-get install bats

# Or install from source
git clone https://github.com/bats-core/bats-core.git
cd bats-core
./install.sh /usr/local
```

### E2E Tests

**Required**:
- Running Kubernetes cluster (v1.28+)
- `kubectl` configured and connected
- Kubernaut V1.0 deployed in cluster
- RBAC permissions for must-gather

## Running Tests

### Unit Tests

Run all unit tests:

```bash
cd cmd/must-gather
bats test/
```

Run specific test file:

```bash
bats test/test_crds.bats
bats test/test_sanitize.bats
```

Run with verbose output:

```bash
bats --trace test/
```

### Integration Tests (E2E)

**Warning**: E2E tests execute against a real cluster and may take several minutes.

Enable and run E2E tests:

```bash
# Set E2E test flag
export KUBERNAUT_E2E_TESTS=1

# Verify cluster connection
kubectl cluster-info

# Run E2E tests
bats test/integration/test_e2e.bats
```

## Test Structure

```
test/
├── helpers.bash              # Shared test utilities
├── test_crds.bats           # CRD collection tests
├── test_logs.bats           # Logs collection tests
├── test_sanitize.bats       # Data sanitization tests
├── test_checksum.bats       # SHA256 checksum tests
├── test_datastorage.bats    # DataStorage API tests
├── test_gather_main.bats    # Main orchestration tests
└── integration/
    ├── README.md            # This file
    └── test_e2e.bats        # End-to-end cluster tests
```

## Test Coverage

| Component | Unit Tests | E2E Tests | Status |
|-----------|-----------|-----------|--------|
| CRD Collection | ✅ | ✅ | Complete |
| Log Collection | ✅ | ✅ | Complete |
| Event Collection | ⚠️  | ✅ | Partial |
| Cluster State | ⚠️  | ✅ | Partial |
| Tekton | ⚠️  | ✅ | Partial |
| DataStorage API | ✅ | ✅ | Complete |
| Database Infra | ⚠️  | ✅ | Partial |
| Metrics | ⚠️  | ✅ | Partial |
| Sanitization | ✅ | ✅ | Complete |
| Checksums | ✅ | ✅ | Complete |
| Main Orchestration | ✅ | ✅ | Complete |

## Test Scenarios

### Unit Tests

Unit tests use mocked `kubectl` and `curl` responses:

- **Positive Cases**: Normal operation with valid data
- **Negative Cases**: Missing resources, API failures, permission errors
- **Edge Cases**: Empty responses, malformed data, timeout scenarios

### E2E Tests

E2E tests execute against a real cluster:

- **Full Collection**: Complete must-gather execution
- **Output Validation**: Verify tarball structure and contents
- **Data Integrity**: Validate checksums
- **Sanitization**: Confirm sensitive data removal
- **Performance**: Verify execution time and size limits

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Must-Gather Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install bats
        run: sudo apt-get install -y bats
      - name: Run unit tests
        run: bats cmd/must-gather/test/

  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Create k8s Kind cluster
        uses: helm/kind-action@v1.5.0
      - name: Deploy Kubernaut
        run: make deploy
      - name: Install bats
        run: sudo apt-get install -y bats
      - name: Run E2E tests
        env:
          KUBERNAUT_E2E_TESTS: "1"
        run: bats cmd/must-gather/test/integration/
```

## Troubleshooting

### Unit Tests Fail

```bash
# Verify bats is installed
bats --version

# Run with verbose output
bats --trace test/test_crds.bats

# Check test environment
echo $BATS_TEST_DIRNAME
echo $BATS_TEST_TMPDIR
```

### E2E Tests Skip

E2E tests are skipped by default. Enable with:

```bash
export KUBERNAUT_E2E_TESTS=1
```

### E2E Tests Fail

```bash
# Verify cluster connection
kubectl cluster-info
kubectl get nodes

# Verify Kubernaut is deployed
kubectl get crds | grep kubernaut

# Check RBAC permissions
kubectl auth can-i get pods --all-namespaces

# Run single E2E test with verbose output
bats --trace test/integration/test_e2e.bats
```

## Performance Benchmarks

Expected test execution times:

| Test Suite | Duration | Notes |
|------------|----------|-------|
| Unit Tests (all) | <30s | Fast, no cluster required |
| E2E Tests (small cluster) | 2-5 min | Depends on cluster size |
| E2E Tests (large cluster) | 5-10 min | More resources to collect |

## Contributing

When adding new collectors, please:

1. ✅ Add unit tests in `test/test_<collector>.bats`
2. ✅ Add E2E validation in `test/integration/test_e2e.bats`
3. ✅ Update test coverage table above
4. ✅ Ensure tests pass before submitting PR

## References

- [Bats-core Documentation](https://bats-core.readthedocs.io/)
- [BR-PLATFORM-001](../../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)

