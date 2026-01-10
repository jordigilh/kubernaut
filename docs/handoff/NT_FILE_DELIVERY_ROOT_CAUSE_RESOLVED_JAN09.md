# Notification File Delivery Root Cause RESOLVED - January 9, 2026

## 🎯 **ROOT CAUSE IDENTIFIED AND FIXED**

### **Problem Summary**
After removing `FileDeliveryConfig` from the NotificationRequest CRD (DD-NOT-006 v2), Notification E2E tests reported **0 files created** despite file delivery being enabled in the ConfigMap.

### **Root Cause** 🐛

**Issue**: ConfigMap had a **hardcoded namespace** that prevented file service initialization.

**Details**:
1. `test/e2e/notification/manifests/notification-configmap.yaml` had `namespace: notification-e2e` (line 27)
2. Infrastructure code runs `kubectl apply -f configmap.yaml -n <dynamic-namespace>`
3. **YAML's hardcoded namespace takes precedence** over `-n` flag
4. ConfigMap created in wrong namespace → Deployment can't find it
5. Controller loads empty config → `cfg.Delivery.File.OutputDir = ""`
6. File service initialization skipped (line 206: `if cfg.Delivery.File.OutputDir != ""`)
7. `fileService` stays `nil`
8. `RegisterChannel("file", nil)` silently skips registration (line 119-122 in orchestrator.go)
9. **Result**: File delivery never happens, tests find 0 files

---

## ✅ **FIX APPLIED**

### **1. ConfigMap Namespace Fix**

**File**: `test/e2e/notification/manifests/notification-configmap.yaml`

**Change**:
```yaml
# BEFORE:
metadata:
  name: notification-controller-config
  namespace: notification-e2e  # ← HARDCODED!

# AFTER:
metadata:
  name: notification-controller-config
  # NOTE: Namespace omitted - uses kubectl apply -n flag for dynamic namespace
  # This allows E2E tests to use any namespace (notification-e2e, test-*, etc.)
  # Authority: DD-NOT-006 v2, BR-NOTIFICATION-001
```

**Result**: ConfigMap now created in correct dynamic namespace, config loaded successfully.

### **2. Ogen Migration Fix**

**File**: `pkg/audit/openapi_client_adapter.go:210`

**Change**:
```go
// BEFORE:
return parseOgenError(err)  // ← UNDEFINED! Function removed during ogen migration

// AFTER:
return err  // Ogen already provides good error messages
```

**Result**: Code compiles successfully.

---

## 📊 **VERIFICATION - ROOT CAUSE CONFIRMED**

### **Evidence from Controller Logs**

**Before Fix** (ConfigMap not loaded):
- No "File delivery service initialized" message
- `cfg.Delivery.File.OutputDir` was empty
- fileService remained nil
- RegisterChannel silently skipped file channel

**After Fix** (ConfigMap loaded successfully):
```
2026-01-10T00:02:04.190Z  INFO  File delivery service initialized  
  {"output_dir": "/tmp/notifications", "format": "json", "timeout": "5s"}

2026-01-10T00:02:04Z  INFO  delivery-orchestrator  Registered delivery channel  
  {"channel": "file"}

2026-01-10T00:05:44Z  INFO  Delivering notification to file  
  {"notification": "e2e-priority-critical", "filename": "notification-e2e-priority-critical-20260110-000544.591908.json", "outputDir": "/tmp/notifications"}

2026-01-10T00:05:44Z  INFO  Notification delivered successfully to file  
  {"filePath": "/tmp/notifications/notification-e2e-priority-critical-20260110-000544.591908.json", "filesize": 2325}
```

**Conclusion**: ✅ **File service NOW initializing and creating files successfully!**

---

## 🔍 **REMAINING ISSUE - Volume Mount**

### **New Problem Discovered**

**Observation**: Files ARE being created in the pod (8+ files logged), but only **1 file** appears on the host.

**Evidence**:
```bash
# Controller logs show 8+ files created:
- notification-e2e-priority-critical-20260110-000544.591908.json (2325 bytes)
- notification-e2e-priority-low-20260110-000544.604959.json (1909 bytes)
- notification-e2e-priority-high-multi-20260110-000544.672859.json (2266 bytes)
- notification-e2e-priority-medium-20260110-000544.856007.json (1921 bytes)
- notification-e2e-priority-high-20260110-000544.864897.json (1921 bytes)
- notification-e2e-priority-critical-2-20260110-000544.873873.json (1939 bytes)
- notification-e2e-multi-channel-fanout-20260110-000545.199426.json (1966 bytes)
- notification-e2e-partial-failure-test-20260110-000545.243836.json (1960 bytes)

# Host directory shows only 1 file:
$ ls ~/.kubernaut/e2e-notifications/*.json | grep "20260110-0005"
notification-e2e-partial-failure-test-20260110-000545.243836.json  # ← Only 1 file!
```

### **Suspected Cause**

**Volume Mount Path Mapping**:
```
Host:       ~/.kubernaut/e2e-notifications
   ↓ (Kind extraMount)
Kind Node:  /tmp/e2e-notifications
   ↓ (hostPath volume)
Pod:        /tmp/notifications (where controller writes)
```

**Hypothesis**: The volume mount is correctly configured but:
1. **Race condition**: Tests check before files are fully synced to host
2. **Permissions issue**: Some files written but not readable by host UID
3. **Sync delay**: Container filesystem not immediately visible on host
4. **Cleanup timing**: Files deleted by AfterEach before tests can read them

---

## 🎯 **NEXT STEPS**

### **Immediate Actions**

1. **Add debug logging** to tests:
   ```go
   BeforeEach(func() {
       files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "*.json"))
       logger.Info("Files before test", "count", len(files), "pattern", e2eFileOutputDir+"/*.json")
   })
   ```

2. **Add explicit wait** after file creation:
   ```go
   // Wait for file to appear on host (sync delay)
   Eventually(func() int {
       files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-*.json"))
       return len(files)
   }, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1))
   ```

3. **Check file timestamps** on host vs pod:
   ```bash
   # In test
   kubectl exec -n notification-e2e deployment/notification-controller -- ls -la /tmp/notifications/
   # On host
   ls -la ~/.kubernaut/e2e-notifications/
   ```

### **Alternative Approaches**

1. **Use ConfigMap for file output** (read-only, simpler):
   - Write files to `/tmp/notifications`
   - Mount ConfigMap volume for output
   - Tests read from ConfigMap instead of hostPath

2. **Use PersistentVolume** instead of hostPath:
   - More reliable sync
   - Better for E2E tests
   - Works across node restarts

3. **Direct pod file access** via `kubectl cp`:
   ```bash
   kubectl cp notification-e2e/notification-controller-xxx:/tmp/notifications/ ~/.kubernaut/e2e-notifications/
   ```

---

## 📝 **SUMMARY**

### **RESOLVED** ✅
- **Root Cause**: Hardcoded namespace in ConfigMap prevented file service initialization
- **Fix**: Removed hardcoded namespace, config now loads correctly
- **Verification**: Controller logs confirm file service initializing and files being created

### **IN PROGRESS** ⏸️
- **Volume Mount Sync**: Files created in pod but only partially visible on host
- **Next**: Add debug logging and explicit waits to diagnose sync timing

### **TEST STATUS**
- **Before Fix**: 16/20 passing (80%), 4 file delivery failures
- **After Fix**: Cannot complete test (Docker build fixed, need to rerun)
- **Expected**: Should improve file delivery test pass rate once volume mount sync resolved

---

## 🔗 **RELATED DOCUMENTS**

- **Design Change**: `docs/handoff/NT_FILE_DELIVERY_CONFIG_DESIGN_ISSUE_JAN08.md`
- **Investigation**: `docs/handoff/NT_FILE_DELIVERY_FIX_STATUS_JAN09.md`
- **E2E Success**: `docs/handoff/NT_E2E_SUCCESS_WE_TEAM_FIX_JAN09.md`

---

**Authority**: DD-NOT-006 v2, BR-NOTIFICATION-001
**Created**: 2026-01-09  
**Status**: ✅ **ROOT CAUSE RESOLVED** - Volume mount sync issue remains

**Key Insight**: Always verify ConfigMap namespace matches deployment namespace in dynamic E2E environments. Hardcoded namespaces in YAML override `kubectl apply -n` flag.
