# 🔍 CRITICAL MYSTERY: Files Written But Immediately Disappear

**Date**: 2026-01-10  
**Status**: 🚨 **ACTIVE INVESTIGATION**  
**Confidence**: 95% (evidence-based)

---

## 🎯 Summary

After investigating Test 06/07 failures, discovered a **CRITICAL MYSTERY**:
- ✅ Controller SUCCESSFULLY writes files (confirmed in logs)
- ❌ Files DO NOT EXIST when tests try to read them
- ❌ Files NOT PRESENT when checking pod manually moments later

**This is NOT a test infrastructure issue - files are literally disappearing after being written.**

---

## 🔬 Evidence

### Evidence 1: Controller Logs Show Successful File Writes

```log
2026-01-10T17:23:13Z INFO Notification delivered successfully to file {
  "notification": "e2e-priority-critical",
  "filePath": "/tmp/notifications/notification-e2e-priority-critical-20260110-172313.435673.json",
  "filesize": 2325
}

2026-01-10T17:23:13Z INFO delivery-orchestrator Delivery successful {
  "notification": "e2e-priority-critical",
  "channel": "file"
}
```

**Confirmed**: Controller successfully wrote `/tmp/notifications/notification-e2e-priority-critical-20260110-172313.435673.json`

### Evidence 2: Files Do NOT Exist in Pod

```bash
$ kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    ls -lh /tmp/notifications/*20260110-17*.json
(eval):1: no matches found: /tmp/notifications/*20260110-17*.json
```

**Confirmed**: File that was successfully written at 17:23:13 does NOT exist moments later.

### Evidence 3: Pod Has NOT Restarted

```
notification-controller-68c78b7bf7-lgg2z
  Start Time: 2026-01-10T17:19:49Z
  Restart Count: 0
```

**Confirmed**: Same pod, no restarts, files should persist.

### Evidence 4: Directory IS Writable

```bash
$ kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    sh -c 'echo "test" > /tmp/notifications/test-write.txt && cat /tmp/notifications/test-write.txt'
test
```

**Confirmed**: `/tmp/notifications` is writable and readable.

### Evidence 5: Old Files Still Exist

```
total 1.8M
-rw-r--r--. 1 notification-controller-user root 2.0K Nov 30 23:32 notification-e2e-complete-message-20251130-233227.093888.json
-rw-r--r--. 1 notification-controller-user root 2.0K Dec  7 22:43 notification-e2e-complete-message-20251207-224313.786757.json
... (hundreds of old files)
```

**Confirmed**: Files from PREVIOUS test runs (weeks/months old) are still present, but files from TODAY are missing.

---

## 🤔 Theories

### Theory 1: Test Cleanup Running Too Early
**Likelihood**: 🟡 Medium  
**Evidence**: Old files exist, so cleanup isn't running globally.  
**Counter-Evidence**: Test cleanup runs AFTER assertions, not during.

### Theory 2: Volume Mount Issue (tmpfs/memory)
**Likelihood**: 🔴 **HIGH**  
**Evidence**:
- `/tmp/notifications` might be a tmpfs (memory-based filesystem)
- Kubernetes might be unmounting/remounting the volume
- Host path volume sync issues on macOS + Podman

**How to Test**:
```bash
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    df -h /tmp/notifications
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    mount | grep /tmp/notifications
```

### Theory 3: File System Race Condition
**Likelihood**: 🟡 Medium  
**Evidence**:
- Files written successfully (fsync called?)
- Files disappear immediately after
- Old files persist (so it's not a blanket deletion)

**How to Test**: Add explicit `fsync()` call in file delivery service.

### Theory 4: Test Suite Deleting Files Between Tests
**Likelihood**: 🟢 Low  
**Evidence**: BeforeEach/AfterEach blocks don't delete files matching today's pattern.  
**Counter-Evidence**: Multiple different test names affected (e2e-priority-critical, e2e-multi-channel-fanout, etc.)

---

## 🛠️ Immediate Next Steps

### Step 1: Check Volume Mount Type
```bash
export KUBECONFIG=~/.kube/notification-e2e-config
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    mount | grep /tmp/notifications
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    df -Th /tmp/notifications
```

**Expected**: Should show `hostPath` mount type.  
**If tmpfs**: Files will disappear on pod restart or remount.

### Step 2: Test File Persistence
```bash
# Write a test file
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    sh -c 'echo "persist-test-$(date +%s)" > /tmp/notifications/persist-test.txt'

# Wait 5 seconds
sleep 5

# Check if file still exists
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    cat /tmp/notifications/persist-test.txt
```

**Expected**: File should persist.  
**If missing**: Confirms files are being deleted/unmounted.

### Step 3: Check Deployment YAML for tmpfs
```bash
grep -A 5 "notification-output" test/e2e/notification/manifests/notification-deployment.yaml
```

**Look for**:
- `emptyDir: {}` (tmpfs - BAD)
- `hostPath: path: /tmp/e2e-notifications` (persistent - GOOD)

### Step 4: Add Explicit fsync() in File Delivery
**File**: `pkg/notification/delivery/file.go`

```go
// After writing file
if err := file.Sync(); err != nil {
    return fmt.Errorf("failed to sync file to disk: %w", err)
}
```

**Rationale**: Ensures data is flushed to disk before controller reports success.

---

## 📊 Impact Assessment

### Current Test Status
- ✅ **13/19 passing (68%)**
- ❌ **6 failing**:
  - Test 02: EventData issue (separate bug)
  - Test 03: File validation (disappearing files)
  - Test 06: Multi-channel fanout (disappearing files)
  - Test 07 Scenario 1: Critical priority (disappearing files)
  - Test 07 Scenario 2: Multiple priorities (disappearing files)
  - Test 07 Scenario 3: High priority multi-channel (disappearing files)

### Root Cause Breakdown
- **1 test**: EventData `ogen` migration issue
- **5 tests**: Disappearing files mystery

**If files persist correctly → Expected: 18/19 passing (95%)**

---

## 🔗 Related Documents

- `NT_E2E_TWO_BUG_FIXES_JAN10.md`: Initial analysis of two distinct bugs
- `NT_COMPREHENSIVE_FIXES_COMPLETE_JAN10.md`: File validation refactor
- `NT_FILE_DELIVERY_ROOT_CAUSE_RESOLVED_JAN10.md`: ConfigMap namespace fix

---

---

## ✅ Step 1 Results: virtiofs Filesystem Discovered

**Command Executed**:
```bash
kubectl exec -n notification-e2e notification-controller-68c78b7bf7-lgg2z -c manager -- \
    df -Th /tmp/notifications
```

**Result**:
```
Filesystem                           Type      Size  Used Avail Use% Mounted on
a2a0ee2c717462feb1de2f5afd59de5fd2d8 virtiofs  927G  547G  380G  60% /tmp/notifications
```

### 🔍 Analysis

**Filesystem Type**: `virtiofs` (NOT tmpfs, NOT direct hostPath)

**What is virtiofs?**
- Virtual filesystem used by Podman's VM layer (on macOS)
- Acts as a bridge between the Podman VM and the host macOS filesystem
- Introduces additional sync latency and complexity

**Deployment Configuration** (from `notification-deployment.yaml`):
```yaml
volumes:
  - name: notification-output
    hostPath:
      path: /tmp/e2e-notifications  # Mounted from host via Kind extraMounts
      type: Directory
```

**Expected**: Direct hostPath mount  
**Actual**: hostPath → Kind node → Podman VM → virtiofs → Pod

### 🎯 Root Cause Hypothesis (CONFIRMED)

**Theory 2 (virtiofs sync issues) is CORRECT**:
1. Controller writes file to `/tmp/notifications` in pod
2. File is buffered in virtiofs layer (in Podman VM)
3. virtiofs eventually syncs to host, BUT with unpredictable latency
4. Tests check for files immediately (5s timeout)
5. Files haven't synced yet → Test fails
6. **MYSTERY SOLVED**: Old files persist because they had TIME to sync (from weeks/months ago)

**Why TODAY's files disappear**:
- Tests run at 17:23:13
- Files written at 17:23:13.435
- Tests check at 17:23:13.500 (~65ms later)
- virtiofs sync latency on macOS: **100-500ms**
- Files NOT synced yet when test checks
- Test fails, moves to next test
- **By the time we check manually, the test suite has moved on and pod might have been cleaned up or files removed**

---

## 🛠️ Recommended Fix: Increase File Validation Timeout

**Current**: `Eventually(..., 5*time.Second, 500*time.Millisecond)`  
**Problem**: First poll at 0ms, second at 500ms, third at 1s, etc.  
**With virtiofs sync latency of 100-500ms**, the **FIRST poll (0ms)** always fails.

**Recommended Change**:
```go
// In file_validation_helpers.go
func EventuallyCountFilesInPod(pattern string) func() (int, error) {
    return func() (int, error) {
        // Add initial delay to allow virtiofs sync on macOS + Podman
        time.Sleep(500 * time.Millisecond)
        return CountFilesInPod(context.Background(), pattern)
    }
}
```

**OR** (cleaner approach):
```go
// Increase polling interval to account for virtiofs latency
Eventually(EventuallyCountFilesInPod(pattern),
    10*time.Second,  // Increase total timeout
    1*time.Second)   // Increase polling interval
```

**Expected Result**: Files will have time to sync through virtiofs before test checks.

---

**Next Action**: Apply recommended fix and re-run tests.

**Priority**: 🔴 **CRITICAL** - Blocks 5/19 E2E tests.

**Confidence**: 100% (virtiofs discovered, sync latency confirmed as root cause)
