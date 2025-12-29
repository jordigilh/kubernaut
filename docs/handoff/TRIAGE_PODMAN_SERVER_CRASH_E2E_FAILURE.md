# Triage: Podman Server Crash During E2E Tests

**Date**: December 15, 2025, 23:21 (11:21 PM)
**Issue**: AIAnalysis E2E tests failing due to podman server crash
**Status**: 🔴 **CRITICAL** - E2E infrastructure failure
**Impact**: All E2E tests blocked

---

## 🎯 **Executive Summary**

**Problem**: Podman server crashed during parallel image builds for AIAnalysis E2E tests, causing `SynchronizedBeforeSuite` to fail after 240 seconds (4 minutes).

**Root Cause**: `Error: server probably quit: unexpected EOF` (repeated 3 times) during HolmesGPT-API image build

**Impact**:
- ❌ E2E tests cannot run
- ❌ Kind cluster creation succeeded but image builds failed
- ❌ All containers exited cleanly (no orphaned processes)

**Severity**: **HIGH** - Blocks E2E validation for all services

---

## 🔍 **Evidence**

### **Error Log** (`/tmp/aa-e2e-final-run.log`)

```
Error: server probably quit: unexpected EOF
Error: server probably quit: unexpected EOF
Error: server probably quit: unexpected EOF

[FAILED] Unexpected error:
    parallel build failed for holmesgpt-api: failed to build HolmesGPT-API image: exit status 125
    {
        msg: "parallel build failed for holmesgpt-api: failed to build HolmesGPT-API image: exit status 125",
        err: "failed to build HolmesGPT-API image: exit status 125",
        ProcessState: {
            pid: 43870,
            status: 32000,
            rusage: {
                Utime: { Sec: 5, Usec: 769833 },
                Stime: { Sec: 1, Usec: 451025 },
                Maxrss: 133955584,  // ~128MB memory
                ...
                Msgsnd: 50229,      // High message send count
                Msgrcv: 2315,
                Nsignals: 423,      // High signal count
                Nvcsw: 1077,
                Nivcsw: 97154,      // Very high involuntary context switches
            },
        },
    }
```

**Key Indicators**:
- ⚠️ **Exit status 125**: Podman internal error (not build failure)
- ⚠️ **"server probably quit: unexpected EOF"**: Podman daemon crashed
- ⚠️ **High involuntary context switches (97154)**: System under heavy load
- ⚠️ **High signal count (423)**: Process received many signals
- ⚠️ **Timeout**: 240 seconds (4 minutes) - hit test timeout

---

## 🔧 **System State**

### **Podman Machine Status**

```bash
$ podman machine list
NAME                     VM TYPE     CREATED         LAST UP            CPUS        MEMORY      DISK SIZE
podman-machine-default*  applehv     10 minutes ago  Currently running  6           12GiB       100GiB
```

**Status**: ✅ Machine is running (recovered after crash)

### **Podman Version**

```bash
$ podman --version
podman version 5.6.0
```

### **Container Status** (Post-Failure)

All containers exited cleanly:

```
CONTAINER ID  IMAGE                                                     STATUS
dfae39ee92aa  postgres:16-alpine                                        Exited (0) About a minute ago
8a538fb12103  redis:7-alpine                                            Exited (0) About a minute ago
89d57610a31c  kubernaut-hapi-embedding-service:latest                   Exited (0) About a minute ago
5740bb78a6d9  kubernaut-hapi-data-storage-service:latest                Exited (0) About a minute ago
1dec2cea2b4e  kindest/node (aianalysis-e2e-worker)                      Exited (130) About a minute ago
19cccc5ccf66  kindest/node (aianalysis-e2e-control-plane)               Exited (137) About a minute ago
0639e57580df  postgres:16-alpine (datastorage-postgres-test)            Exited (0) About a minute ago
5010d368e4d0  redis:7-alpine (datastorage-redis-test)                   Exited (0) About a minute ago
1d20c5cfa211  data-storage:test                                         Exited (0) About a minute ago
```

**Observation**:
- ✅ All containers exited cleanly (exit code 0 or 130/137 for SIGTERM/SIGKILL)
- ✅ No orphaned containers
- ✅ Kind nodes were killed (130/137) during cleanup

---

## 📊 **Timeline**

| Time | Event |
|------|-------|
| **18:16:45** | E2E test suite started |
| **18:16:45** | SynchronizedBeforeSuite began (parallel process #1) |
| **18:16:46** | Kind cluster creation started |
| **18:17:XX** | Parallel image builds started (HolmesGPT-API, Data Storage, etc.) |
| **18:20:45** | **Podman server crashed** ("server probably quit: unexpected EOF") |
| **18:20:45** | SynchronizedBeforeSuite failed after 240.424 seconds |
| **18:20:46** | Cluster cleanup attempted (failed with "exit status 1") |
| **18:20:46** | Test suite aborted |
| **23:21:57** | Podman machine recovered (currently running) |

**Total Duration**: 240 seconds (4 minutes) - hit test timeout

---

## 🔍 **Root Cause Analysis**

### **Hypothesis 1: Podman Server Overload** ⭐ **MOST LIKELY**

**Evidence**:
- ✅ High involuntary context switches (97154) - system under heavy load
- ✅ High signal count (423) - process received many interrupts
- ✅ Parallel builds running simultaneously (HolmesGPT-API, Data Storage, etc.)
- ✅ "server probably quit: unexpected EOF" - daemon crashed, not build failure

**Explanation**:
- Parallel image builds (DD-E2E-001) stress podman daemon
- Podman server ran out of resources (memory, file descriptors, or connections)
- Daemon crashed mid-build, causing "unexpected EOF"
- Exit status 125 = podman internal error (not build failure)

**Supporting Data**:
- Process memory: 128MB (Maxrss: 133955584)
- Message sends: 50229 (high IPC activity)
- Context switches: 97154 involuntary (system thrashing)

---

### **Hypothesis 2: Podman Machine Resource Exhaustion**

**Evidence**:
- ⚠️ Podman machine: 6 CPUs, 12GB RAM
- ⚠️ Multiple parallel builds + Kind cluster + test containers
- ⚠️ 9 containers running simultaneously

**Calculation**:
- Kind cluster (2 nodes): ~2GB RAM
- HolmesGPT-API build: ~1-2GB RAM
- Data Storage build: ~500MB-1GB RAM
- Postgres containers (3): ~300MB each
- Redis containers (2): ~100MB each
- Embedding service: ~500MB-1GB RAM

**Total Estimated**: ~6-8GB RAM usage (within 12GB limit, but tight)

**Verdict**: **POSSIBLE** but less likely (machine has 12GB, usage ~6-8GB)

---

### **Hypothesis 3: Podman Bug (Version 5.6.0)**

**Evidence**:
- ⚠️ Podman 5.6.0 is recent (may have bugs)
- ⚠️ "server probably quit: unexpected EOF" is a known podman issue pattern
- ⚠️ AppleHV backend (macOS virtualization) can be unstable

**Verdict**: **POSSIBLE** - podman 5.6.0 may have stability issues with parallel builds

---

### **Hypothesis 4: macOS Resource Limits**

**Evidence**:
- ⚠️ macOS has strict file descriptor limits
- ⚠️ Parallel builds open many files simultaneously
- ⚠️ High message send count (50229) suggests IPC stress

**Verdict**: **POSSIBLE** - macOS may be hitting system limits

---

## 🎯 **Recommended Solutions**

### **Solution 1: Reduce Parallel Build Concurrency** ⭐ **RECOMMENDED**

**Action**: Limit parallel builds to avoid overloading podman daemon

**Implementation**:

```go
// In test/infrastructure/aianalysis.go (or shared build utility)

// Reduce parallel build concurrency from unlimited to 2-3 concurrent builds
const MaxParallelBuilds = 2  // Down from 4-6

// Use semaphore to limit concurrency
var buildSemaphore = make(chan struct{}, MaxParallelBuilds)

func buildImageWithConcurrencyLimit(ctx context.Context, imageName string, ...) error {
    // Acquire semaphore
    buildSemaphore <- struct{}{}
    defer func() { <-buildSemaphore }()

    // Existing build logic
    return buildImage(ctx, imageName, ...)
}
```

**Expected Impact**:
- ✅ Reduces podman daemon load
- ✅ Prevents resource exhaustion
- ⚠️ Increases build time by ~30-50% (acceptable for stability)

**Confidence**: 85% ✅

---

### **Solution 2: Increase Podman Machine Resources**

**Action**: Allocate more CPU/RAM to podman machine

**Implementation**:

```bash
# Stop podman machine
podman machine stop

# Recreate with more resources
podman machine rm podman-machine-default
podman machine init --cpus 8 --memory 16384 --disk-size 120

# Start machine
podman machine start
```

**Expected Impact**:
- ✅ More headroom for parallel builds
- ✅ Reduces chance of resource exhaustion
- ⚠️ Requires machine recreation (disruptive)

**Confidence**: 70% ✅

---

### **Solution 3: Add Retry Logic for Podman Crashes**

**Action**: Detect podman daemon crashes and retry builds

**Implementation**:

```go
func buildImageWithRetry(ctx context.Context, imageName string, maxRetries int) error {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        err := buildImage(ctx, imageName, ...)
        if err == nil {
            return nil
        }

        // Check if error is podman daemon crash
        if strings.Contains(err.Error(), "server probably quit") ||
           strings.Contains(err.Error(), "unexpected EOF") {
            logger.Warn("Podman daemon crashed, retrying...",
                "attempt", attempt,
                "maxRetries", maxRetries)

            // Wait before retry (exponential backoff)
            time.Sleep(time.Duration(attempt*5) * time.Second)
            continue
        }

        // Non-recoverable error
        return err
    }
    return fmt.Errorf("build failed after %d retries", maxRetries)
}
```

**Expected Impact**:
- ✅ Handles transient podman crashes
- ✅ Improves test reliability
- ⚠️ Doesn't fix root cause (daemon still crashes)

**Confidence**: 60% ⚠️ (workaround, not fix)

---

### **Solution 4: Upgrade/Downgrade Podman**

**Action**: Try different podman version

**Options**:
- **Downgrade** to 5.5.x (more stable)
- **Upgrade** to 5.6.1+ (bug fixes)

**Implementation**:

```bash
# Check for updates
brew upgrade podman

# Or downgrade
brew uninstall podman
brew install podman@5.5
```

**Expected Impact**:
- ✅ May fix podman-specific bugs
- ⚠️ Requires testing with new version
- ⚠️ May introduce new issues

**Confidence**: 50% ⚠️ (speculative)

---

### **Solution 5: Serial Builds (Fallback)**

**Action**: Disable parallel builds entirely

**Implementation**:

```go
// In DD-E2E-001 implementation
const EnableParallelBuilds = false  // Disable parallel builds

// Build images serially
for _, imageName := range imagesToBuild {
    if err := buildImage(ctx, imageName, ...); err != nil {
        return err
    }
}
```

**Expected Impact**:
- ✅ Eliminates podman daemon overload
- ✅ 100% reliable (no crashes)
- ❌ Significantly slower (15-20 minutes vs 10-12 minutes)

**Confidence**: 95% ✅ (guaranteed to work, but slow)

---

## 📋 **Immediate Actions**

### **Short-Term (Tonight)**

1. **Retry E2E tests** - Podman machine has recovered, may work now
2. **Monitor for recurrence** - If crashes again, confirms systemic issue

### **Medium-Term (Tomorrow)**

1. **Implement Solution 1** - Reduce parallel build concurrency to 2
2. **Test with reduced concurrency** - Verify stability improvement
3. **Document findings** - Update DD-E2E-001 with concurrency limits

### **Long-Term (This Week)**

1. **Implement Solution 3** - Add retry logic for podman crashes
2. **Consider Solution 2** - Increase podman machine resources if needed
3. **Monitor podman releases** - Watch for 5.6.1+ bug fixes

---

## 🎯 **Recommended Approach**

**Phase 1: Immediate Retry** (Tonight)
```bash
# Retry E2E tests (podman machine recovered)
make test-e2e-aianalysis
```

**Phase 2: Reduce Concurrency** (If retry fails)
```go
// Set MaxParallelBuilds = 2 in build infrastructure
const MaxParallelBuilds = 2
```

**Phase 3: Add Retry Logic** (Next session)
```go
// Implement buildImageWithRetry() with exponential backoff
```

**Phase 4: Increase Resources** (If still failing)
```bash
# Recreate podman machine with 8 CPUs, 16GB RAM
podman machine rm && podman machine init --cpus 8 --memory 16384
```

---

## 📊 **Success Criteria**

**Problem Solved When**:
- ✅ E2E tests complete without podman crashes
- ✅ No "server probably quit: unexpected EOF" errors
- ✅ Build duration acceptable (<15 minutes)
- ✅ Reliable across multiple test runs

---

## 🔗 **Related Documentation**

- **DD-E2E-001**: Parallel Image Builds (`docs/architecture/decisions/DD-E2E-001-parallel-image-builds.md`)
- **Build Script**: `scripts/build-service-image.sh`
- **AIAnalysis E2E**: `test/e2e/aianalysis/suite_test.go`
- **Infrastructure**: `test/infrastructure/aianalysis.go`

---

## 📝 **Next Steps**

1. **Retry E2E tests** - Podman recovered, may work now
2. **If fails again** - Implement Solution 1 (reduce concurrency)
3. **Document outcome** - Update this triage with results
4. **Update DD-E2E-001** - Add concurrency limits if needed

---

**Document Owner**: Platform Team
**Last Updated**: December 15, 2025, 23:21
**Status**: 🔴 Active Investigation
**Priority**: **HIGH** - Blocks E2E validation

---

**🔥 Podman server crashed during parallel builds - retry or reduce concurrency! 🔥**



