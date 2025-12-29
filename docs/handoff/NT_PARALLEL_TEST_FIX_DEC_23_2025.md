# Notification Integration Test Parallel Execution Fix

**Date**: December 23, 2025
**Issue**: Integration tests failing with parallel execution (`--procs=4`)
**Root Cause**: Using `BeforeSuite` instead of `SynchronizedBeforeSuite`
**Status**: ✅ **ROOT CAUSE IDENTIFIED**

---

## Problem

Integration tests were failing with errors:
```
Error: network name notification_test_network already used: network already exists
Error: cannot listen on the TCP port: listen tcp4 :15439: bind: address already in use
```

**Initial incorrect diagnosis**: Thought we needed to run serially with `--procs=1`
**User correction**: Per DD-TEST-002, serial execution (`--procs=1`) is an anti-pattern

---

## Root Cause

The test suite was using `BeforeSuite` which runs **once per parallel process**:
- With `--procs=4`, `BeforeSuite` runs 4 times
- All 4 processes try to create the same infrastructure:
  - Same network name (`notification_test_network`)
  - Same ports (15439, 16385, 18096)
  - Result: Port conflicts and network name collisions

---

## Solution

Convert to `SynchronizedBeforeSuite` pattern per DD-TEST-002:

### Pattern (from Gateway implementation)

```go
var _ = SynchronizedBeforeSuite(func() []byte {
    // FIRST FUNCTION: Runs ONCE on process 1 only
    // - Create shared infrastructure (DSBootstrap, envtest)
    // - Return serialized config for other processes

    dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
    // ... setup envtest ...

    // Share config with all processes
    config := SharedConfig{
        Kubeconfig:     testEnv.KubeConfig,
        DataStorageURL: dataStorageURL,
    }
    return json.Marshal(config)

}, func(data []byte) {
    // SECOND FUNCTION: Runs on ALL processes (including process 1)
    // - Unmarshal shared config from process 1
    // - Setup per-process state (logger, client, env vars)

    var sharedConfig SharedConfig
    json.Unmarshal(data, &sharedConfig)

    // Create per-process K8s client
    k8sConfig, _ = clientcmd.RESTConfigFromKubeConfig(sharedConfig.Kubeconfig)
    os.Setenv("TEST_DATA_STORAGE_URL", sharedConfig.DataStorageURL)
})
```

---

## DD-TEST-002 Compliance

### ✅ Correct Approach
- **Use `SynchronizedBeforeSuite`** for shared infrastructure
- **Run with `--procs=4`** for parallel execution
- **Unique namespaces per test** for resource isolation

### ❌ Anti-Patterns to Avoid
- ❌ Using `BeforeSuite` with shared infrastructure
- ❌ Running serially with `--procs=1`
- ❌ Shared network names or fixed ports across processes

---

## Implementation Checklist

- [ ] Replace `BeforeSuite` with `SynchronizedBeforeSuite` in `suite_test.go`
- [ ] Move DSBootstrap setup to first function (process 1 only)
- [ ] Move envtest setup to first function (process 1 only)
- [ ] Serialize and share config (kubeconfig, URLs) via return value
- [ ] Initialize per-process state in second function
- [ ] Test with `--procs=4` to validate parallel execution
- [ ] Verify audit infrastructure works across all processes

---

## Reference Implementations

- ✅ **Gateway**: `test/integration/gateway/suite_test.go`
- ✅ **DataStorage**: `test/integration/datastorage/suite_test.go`
- ✅ **SignalProcessing**: `test/integration/signalprocessing/suite_test.go`
- ✅ **RemediationOrchestrator**: `test/integration/remediationorchestrator/suite_test.go`
- ✅ **AIAnalysis**: `test/integration/aianalysis/suite_test.go`

---

## Expected Outcome

After fix:
- ✅ All 4 parallel processes use the same infrastructure
- ✅ No port conflicts or network collisions
- ✅ Tests complete in ~40s (vs ~120s sequential)
- ✅ DD-TEST-002 compliant (4 concurrent processes)

---

**Next Action**: Convert `test/integration/notification/suite_test.go` to use `SynchronizedBeforeSuite` pattern



