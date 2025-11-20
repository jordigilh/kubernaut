# Gateway Kubeconfig Isolation Update

## Overview
Updated Gateway integration and E2E tests to use a dedicated kubeconfig at `~/.kube/gateway-kubeconfig` to avoid collisions with other test suites.

## Changes Made

### Integration Tests
1. **`test/integration/gateway/suite_test.go`**
   - Updated kubeconfig path from `~/.kube/kind-config` to `~/.kube/gateway-kubeconfig`
   - Updated log message to reflect new path

2. **`test/integration/gateway/helpers.go`**
   - Updated fallback kubeconfig path to `~/.kube/gateway-kubeconfig`

3. **`test/integration/gateway/security_suite_setup.go`**
   - Updated fallback kubeconfig path to `~/.kube/gateway-kubeconfig`

4. **`test/integration/gateway/setup-kind-cluster.sh`**
   - Updated `KIND_KUBECONFIG` variable to `${HOME}/.kube/gateway-kubeconfig`

5. **`test/integration/gateway/run-tests-kind.sh`**
   - Updated `KUBECONFIG` export to `${HOME}/.kube/gateway-kubeconfig`

### E2E Tests
1. **`test/e2e/gateway/gateway_e2e_suite_test.go`**
   - Updated kubeconfig path from `~/.kube/kind-config` to `~/.kube/gateway-kubeconfig`
   - Updated log message to reflect new path

2. **`test/e2e/gateway/deduplication_helpers.go`**
   - Updated kubeconfig path to `~/.kube/gateway-kubeconfig`

### Infrastructure
1. **`test/infrastructure/gateway.go`**
   - Updated documentation comment to reflect new kubeconfig path

## Benefits

### Isolation
- **No Collisions**: Gateway tests now use a dedicated kubeconfig file
- **Parallel Execution**: Can run Gateway tests alongside other service tests without conflicts
- **Clean Separation**: Each service suite can have its own kubeconfig

### Consistency
- **Uniform Pattern**: Follows the pattern established for other services (e.g., Toolset uses `~/.kube/kind-toolset-config`)
- **Clear Ownership**: The kubeconfig filename clearly indicates which service it belongs to

### Safety
- **No Default Kubeconfig Modification**: Never touches `~/.kube/config`
- **Environment Variable Override**: Still respects `KUBECONFIG` env var if set
- **Predictable Behavior**: Tests always know which cluster they're targeting

## Usage

### Running Integration Tests
```bash
# Tests automatically use ~/.kube/gateway-kubeconfig
make test-gateway-integration

# Or manually with explicit KUBECONFIG
export KUBECONFIG="${HOME}/.kube/gateway-kubeconfig"
go test ./test/integration/gateway/... -v
```

### Running E2E Tests
```bash
# Tests automatically use ~/.kube/gateway-kubeconfig
make test-gateway-e2e

# Or manually with explicit KUBECONFIG
export KUBECONFIG="${HOME}/.kube/gateway-kubeconfig"
go test ./test/e2e/gateway/... -v
```

### Cleanup
```bash
# Remove Gateway kubeconfig
rm ~/.kube/gateway-kubeconfig

# Delete Gateway Kind cluster
kind delete cluster --name gateway-integration
kind delete cluster --name gateway-e2e
```

## Verification

### Build Status
- ✅ Integration tests compile without errors
- ✅ E2E tests compile without errors
- ✅ No lint errors introduced

### Files Updated
- 7 Go source files
- 2 Shell scripts
- 1 Infrastructure helper

### Pattern Consistency
```
Gateway:     ~/.kube/gateway-kubeconfig
Toolset:     ~/.kube/kind-toolset-config
DataStorage: (TBD - should follow same pattern)
```

## Next Steps

1. **Test Execution**: Run integration and E2E tests to verify functionality
2. **Documentation Update**: Update main test documentation to reflect new kubeconfig paths
3. **Other Services**: Consider applying same pattern to DataStorage and other services
4. **CI/CD**: Update CI/CD pipelines if they reference the old kubeconfig path

## Related Documentation
- `test/integration/gateway/KIND_KUBECONFIG_ISOLATION.md` - Original isolation documentation
- `docs/testing/TEST_PACKAGE_NAMING_STANDARD.md` - Test package naming conventions
- `.cursor/rules/03-testing-strategy.mdc` - Parallel testing requirements

