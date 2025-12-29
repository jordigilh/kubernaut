# AIAnalysis E2E - HAPI Timeout Investigation
**Date**: December 26, 2025 (21:05+)
**Service**: AIAnalysis E2E Infrastructure
**Issue**: HolmesGPT-API pod readiness timeout
**Author**: AI Assistant
**Status**: 🔍 INVESTIGATION IN PROGRESS

## 🎯 Investigation Objective

**Goal**: Determine why HolmesGPT-API pod fails to become ready within 2-minute timeout

**Context**:
- Namespace fix ✅ VALIDATED and working
- Infrastructure progresses successfully through DataStorage
- HAPI pod timeout blocks E2E test execution

## 🔬 Enhanced Debugging Implemented

### File Modified
**`test/infrastructure/aianalysis.go`** (lines 1772-1792)

### Debugging Enhancements Added

**1. Poll Count Tracking**:
```go
pollCount := 0
maxPolls := 24  // 2 minutes / 5 seconds = 24 polls
```

**2. Periodic Status Reporting** (every 20 seconds):
- Pod name and phase (Pending/Running/Failed)
- Ready condition status
- Container readiness details
- Restart counts
- Waiting/Terminated reasons with messages

**3. Detailed Error Diagnostics**:
```go
// Shows for non-ready containers:
Container 'holmesgpt-api': Ready=false, RestartCount=3
  Waiting: ImagePullBackOff (Failed to pull image)
  // OR
  Terminated: ExitCode=1, Reason=Error
```

**4. Error Handling**:
- Kubernetes API errors logged
- Missing pods detected and reported

### Expected Output Format

```
⏳ Waiting for HolmesGPT-API pod to be ready...
   [Poll 1/24] No HAPI pods found
   [Poll 4/24] HAPI pod 'holmesgpt-api-xxx': Phase=Pending, Ready=False (Reason: ContainersNotReady)
      Container 'holmesgpt-api': Ready=false, RestartCount=0, Waiting: ContainerCreating
   [Poll 8/24] HAPI pod 'holmesgpt-api-xxx': Phase=Running, Ready=False (Reason: ContainersNotReady)
      Container 'holmesgpt-api': Ready=false, RestartCount=2, Terminated: ExitCode=137, Reason=OOMKilled
   [Poll 12/24] ...
```

## 🔍 Diagnostic Scenarios

### Scenario A: Image Pull Issue
**Symptoms**:
- Pod stuck in `Pending` phase
- Waiting: `ImagePullBackOff` or `ErrImagePull`

**Solution**: Check image tag and Kind image loading

### Scenario B: Container Crash Loop
**Symptoms**:
- Pod reaches `Running` but Ready=False
- High `RestartCount`
- Terminated state with exit code

**Solution**: Check container logs, resource limits, configuration

### Scenario C: Failed Readiness Probe
**Symptoms**:
- Pod `Running`, containers ready, but Pod Ready=False
- Readiness probe failing

**Solution**: Check readiness probe configuration, health endpoint

### Scenario D: Resource Constraints
**Symptoms**:
- Pod stuck in `Pending`
- Events show: `Insufficient memory/cpu`

**Solution**: Adjust Kind node resources or pod requests

### Scenario E: Slow Startup (Coverage Overhead)
**Symptoms**:
- Pod eventually becomes ready after >2 minutes
- Normal progression, just slow

**Solution**: Increase timeout to 5 minutes for coverage builds

## 📋 Investigation Steps

### Step 1: Monitor Test Run (IN PROGRESS)
```bash
# Watch the enhanced debugging output
tail -f /tmp/aa-e2e-debug-run.log
```

**Expected Duration**: ~10 minutes to reach HAPI deployment

### Step 2: Capture Debugging Output
The test run will show:
- Poll progress (1/24, 2/24, etc.)
- Pod phase transitions
- Container status changes
- Error reasons and messages

### Step 3: Inspect Cluster (If Preserved)
If timeout occurs, cluster will be preserved for inspection:

```bash
# Check cluster exists
kind get clusters

# Check HAPI pod status
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config get pods -n kubernaut-system -l app=holmesgpt-api

# Get detailed pod description
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config describe pod -n kubernaut-system -l app=holmesgpt-api

# Get container logs
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config logs -n kubernaut-system -l app=holmesgpt-api --all-containers

# Check events
kubectl --kubeconfig ~/.kube/aianalysis-e2e-config get events -n kubernaut-system --sort-by='.lastTimestamp' | grep -i hapi
```

### Step 4: Analyze Root Cause
Based on debugging output and inspection, determine:
- Is this a configuration issue?
- Is this a resource issue?
- Is this a timing issue (coverage overhead)?
- Is this an infrastructure bug?

### Step 5: Implement Fix
Once root cause identified:
- Quick fix: Increase timeout if just slow startup
- Proper fix: Address underlying configuration/resource issue

## 🎯 Success Criteria

**Investigation Complete When**:
1. ✅ Root cause of HAPI timeout identified
2. ✅ Diagnostic data captured (pod status, logs, events)
3. ✅ Recommended fix documented
4. ✅ Fix validated with successful E2E test run

## 📊 Current Status

### Test Run Status
- **Started**: 21:05+ (approx)
- **Log File**: `/tmp/aa-e2e-debug-run.log`
- **Expected Failure Point**: ~10 minutes (at HAPI readiness check)

### Debugging Enhancements
- ✅ Code modified with enhanced logging
- ✅ Compilation verified
- ✅ Test execution started
- ⏳ Waiting for HAPI deployment phase

### Cluster Preservation
- ✅ Suite configured to preserve cluster on failure
- ✅ Manual cleanup instructions available
- ✅ Will allow post-failure inspection

## 📝 Next Actions

1. **Monitor test progress** (~10 minutes until HAPI phase)
2. **Capture debugging output** when HAPI deployment starts
3. **Analyze pod status** from enhanced logging
4. **Inspect cluster** if preserved on timeout
5. **Document findings** and recommend fix

---

**Status**: 🔍 Investigation in progress
**Expected Update**: When test reaches HAPI deployment phase (~10 minutes)
**Cluster**: Will be preserved on failure for inspection







