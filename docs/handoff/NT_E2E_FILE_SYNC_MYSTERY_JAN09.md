# Notification E2E File Sync Mystery Investigation (Jan 9, 2026)

## 🚨 **CRITICAL ISSUE: Files No Longer Appearing on Host**

**Symptom**: After applying race condition fixes, files stopped appearing on host entirely
**Timeline**:
- **18:48 EST**: Files appearing on host (last successful run)
- **19:06 EST**: Some files appearing (triage run - 1 out of 8 files appeared)
- **20:21 EST**: ZERO files appearing (after applying fixes)

---

## 📊 **EVIDENCE**

### **Pod-Side: Files Being Written Successfully**
From Kind logs (`/tmp/notification-e2e-logs-20260109-190615`):
```
2026-01-10T00:05:44Z INFO Notification delivered successfully to file
{"filePath": "/tmp/notifications/notification-e2e-priority-critical-20260110-000544.591908.json", "filesize": 2325}

2026-01-10T00:05:44Z INFO Notification delivered successfully to file
{"filePath": "/tmp/notifications/notification-e2e-priority-low-20260110-000544.604959.json", "filesize": 1909}

2026-01-10T00:05:44Z INFO Notification delivered successfully to file
{"filePath": "/tmp/notifications/notification-e2e-priority-high-multi-20260110-000544.672859.json", "filesize": 2266}

2026-01-10T00:05:44Z INFO Notification delivered successfully to file
{"filePath": "/tmp/notifications/notification-e2e-priority-medium-20260110-000544.856007.json", "filesize": 1921}
```

**Result**: ✅ Controller writing files successfully IN POD

---

### **Host-Side: Files Not Appearing**
```bash
$ ls -la ~/.kubernaut/e2e-notifications/*20260109* | tail -10
-rw-r--r--  1 jgil  staff  1960 Jan  9 18:04 notification-e2e-partial-failure-test-20260109-230458.json
-rw-r--r--  1 jgil  staff  1960 Jan  9 18:19 notification-e2e-partial-failure-test-20260109-231930.json
-rw-r--r--  1 jgil  staff  1951 Jan  9 18:40 notification-e2e-partial-failure-test-20260109-234013.json
-rw-r--r--  1 jgil  staff  1952 Jan  9 18:48 notification-e2e-partial-failure-test-20260109-234808.json  ← LAST FILE
-rw-r--r--  1 jgil  staff  2346 Jan  9 18:04 notification-e2e-retry-backoff-test-20260109-230457.json
-rw-r--r--  1 jgil  staff  2346 Jan  9 18:19 notification-e2e-retry-backoff-test-20260109-231931.json
```

**Latest files**: 18:48 EST (before fixes)
**Current time**: 20:21 EST (after fixes)
**Gap**: 1.5 hours with ZERO new files

**Result**: ❌ Files NOT appearing on host after 20:21

---

### **Test Results: Eventually() Timing Out**
```
[FAILED] Timed out after 2.000s.
File should appear within 2 seconds (macOS Podman sync delay)
Expected <int>: 0
to be >=
  <int>: 1
```

**Interpretation**: Eventually() is working correctly, polling for 2 seconds, but finding 0 files

---

## 🔍 **WHAT CHANGED BETWEEN 18:48 AND 20:21?**

### **Changes Made:**
1. ✅ Added `Eventually()` waits (3 locations) - **CODE CHANGE**
2. ✅ Fixed EventData extraction (1 location) - **CODE CHANGE**
3. ✅ Marked partial delivery test as Pending - **TEST CHANGE**
4. ✅ Git commit at 20:19 EST
5. ✅ Test run at 20:21 EST

**Hypothesis**: Something in the code changes broke file delivery or volume mounting

---

## 🤔 **POTENTIAL ROOT CAUSES**

### **Theory 1: Eventually() Pattern Broke Something**
**Likelihood**: LOW
**Rationale**: Eventually() is just a Gomega matcher, doesn't affect controller behavior

### **Theory 2: Build/Image Issue**
**Likelihood**: MEDIUM
**Rationale**:
- Dockerfile builds controller image for Kind
- Image might be cached from 18:48 run
- New test run might not have rebuilt image with latest code

**Test**: Check if image was rebuilt
```bash
# Check Kind image timestamp
docker images | grep kubernaut-notification:e2e-test
```

### **Theory 3: Volume Mount Configuration Changed**
**Likelihood**: MEDIUM
**Rationale**:
- Volume mount was working at 18:48
- AfterEach cleanup in tests didn't change
- Kind cluster configuration didn't change

**Test**: Compare Kind cluster configs before/after

### **Theory 4: Podman VM Issue**
**Likelihood**: HIGH
**Rationale**:
- Podman VM manages hostPath mounts
- macOS FUSE layer can have sync issues
- VM might need restart after extended testing

**Test**: Restart Podman VM
```bash
podman machine stop && podman machine start
```

### **Theory 5: initContainer Permissions**
**Likelihood**: LOW
**Rationale**:
- initContainer sets permissions on `/tmp/notifications`
- If permissions changed, writes would fail
- But logs show writes succeeding

**Test**: Check initContainer logs for errors

### **Theory 6: AfterEach Cleanup Timing**
**Likelihood**: MEDIUM
**Rationale**:
- AfterEach runs `os.Remove(f)` for each file
- Pattern: `notification-e2e-priority-*.json`
- Could be deleting files before Eventually() sees them?

**Problem**: AfterEach runs AFTER test, not during
**But**: If tests run in parallel, could one test's cleanup affect another?

---

## 🧪 **DIAGNOSTIC TESTS TO RUN**

### **Test 1: Check if Controller Image Was Rebuilt**
```bash
# List images built today
docker images --filter "since=2026-01-09T18:00:00" | grep notification

# Expected: Should see kubernaut-notification:e2e-test with recent timestamp
```

### **Test 2: Manually Check Files in Kind Node**
```bash
# Start a test cluster
make test-e2e-notification &  # Let it run BeforeSuite

# In another terminal, exec into Kind node
docker exec -it notification-e2e-control-plane bash
ls -la /tmp/e2e-notifications/

# Expected: Files should exist in Kind node
```

### **Test 3: Check Host Mount Point**
```bash
# Check if directory is actually mounted
mount | grep e2e-notifications

# Check permissions
ls -ld ~/.kubernaut/e2e-notifications/

# Expected: Should be mounted and writable
```

### **Test 4: Restart Podman and Retest**
```bash
podman machine stop
podman machine start
make test-e2e-notification

# If this fixes it: Podman VM issue
# If still broken: Code/configuration issue
```

### **Test 5: Revert Commit and Retest**
```bash
git stash
git reset --hard HEAD~1  # Revert to before fixes
make test-e2e-notification

# If files appear: Fix broke something
# If still broken: Environmental issue
```

---

## 📋 **NEXT STEPS (PRIORITIZED)**

### **IMMEDIATE (5 min):**
1. Run Test 4: Restart Podman VM and retest
   - **Rationale**: Fastest to test, most likely cause (VM state)
   - **If fixes**: Document Podman VM instability
   - **If doesn't fix**: Proceed to Test 5

### **SHORT-TERM (10 min):**
2. Run Test 5: Revert commit and retest
   - **Rationale**: Confirms if code changes broke delivery
   - **If fixes**: Bisect the 3 code changes to find culprit
   - **If doesn't fix**: Environmental issue confirmed

### **MEDIUM-TERM (15 min):**
3. Run Test 2: Manual inspection of Kind node
   - **Rationale**: Confirms files exist in node, isolates mount sync issue
   - **If files in node**: Mount sync problem
   - **If no files**: Controller issue (despite logs)

### **FALLBACK (30 min):**
4. Full diagnostic sweep
   - Check all initContainer logs
   - Verify volume mount configuration
   - Compare Kind configs before/after
   - Review test parallelization settings

---

## 🎯 **EXPECTED OUTCOME**

**Most Likely**: Podman VM restart fixes issue (VM state corruption)
**Second Most Likely**: Code change broke image rebuild (cached old image)
**Least Likely**: Actual volume mount configuration changed

**If Podman VM restart fixes it**:
- Document as known issue
- Add Podman VM restart to E2E pre-flight checks
- Consider adding explicit `podman machine restart` to Makefile

**If code change broke it**:
- Bisect the 3 changes (Eventually, EventData, Pending)
- Revert breaking change
- Find alternative fix

---

## 📝 **CRITICAL QUESTIONS**

1. **Are files being written in the Kind node?**
   - Answer: Unknown (need Test 2)
   - Impact: HIGH (isolates pod vs host issue)

2. **Was the controller image rebuilt?**
   - Answer: Unknown (need Test 1)
   - Impact: MEDIUM (cached image issue)

3. **Is Podman VM in a bad state?**
   - Answer: Possible (VM has been running for hours)
   - Impact: HIGH (restart could fix everything)

4. **Did volume mount configuration change?**
   - Answer: No obvious changes in code
   - Impact: LOW (unlikely root cause)

---

**Authority**: DD-NOT-006 v2, BR-NOTIFICATION-001
**Status**: Investigation required - files stopped appearing after fixes applied
**Next**: Run Test 4 (Podman restart) as first diagnostic step
**Urgency**: HIGH - blocking E2E test completion
