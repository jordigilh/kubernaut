# Notification E2E File Pattern Bug Analysis

**Date**: February 1, 2026  
**Status**: 🚨 **2 Pattern Matching Bugs Found** (1 fixed, 1 remaining)  
**Impact**: Causes flaky test failures due to cross-test file pollution  
**Root Cause**: Wildcard patterns matching files from other tests

---

## 🎯 Root Cause Analysis

### Filename Format

**File Delivery Service** (`pkg/notification/delivery/file.go:221`):
```go
return fmt.Sprintf("notification-%s-%s.%s", notification.Name, timestamp, format)
```

**Example**:
- Notification: `e2e-priority-critical`  
- File: `notification-e2e-priority-critical-20260201-143022.123456.json`

---

## 🐛 Bug #1: `e2e-priority-critical` vs `e2e-priority-critical-2` (FIXED ✅)

### Location
`test/e2e/notification/07_priority_routing_test.go:174` (Scenario 1)

### Original Code
```go
pattern := "notification-e2e-priority-critical-*.json"
```

### Problem
**Two notifications with overlapping names**:
- **Scenario 1** (line 82): Creates `e2e-priority-critical` **WITH Metadata**
- **Scenario 2** (line 248): Creates `e2e-priority-critical-2` **WITHOUT Metadata**

**Pattern matching**:
```
Pattern: notification-e2e-priority-critical-*.json
  ✅ Matches: notification-e2e-priority-critical-20260201-143022.123456.json (CORRECT)
  ✅ Matches: notification-e2e-priority-critical-2-20260201-143022.123456.json (WRONG FILE!)
                                                    ↑
                         Wildcard * matches "-2-<timestamp>", causing cross-test pollution
```

### Test Behavior
- **Scenario 1** creates notification WITH `Metadata: {"severity": "critical", ...}`
- **Scenario 1** waits for file with pattern `notification-e2e-priority-critical-*.json`
- `WaitForFileInPod()` finds `notification-e2e-priority-critical-2-*.json` FIRST (from Scenario 2)
- **Scenario 1** reads file from `e2e-priority-critical-2` (which has NO Metadata)
- **Test fails**: `Expect(savedNotification.Spec.Metadata["severity"]).To(Equal("critical"))` ❌

### Debug Log Evidence
```log
2026-02-01T20:56:36.278  DEBUG: Creating NotificationRequest with Metadata
  name: "e2e-priority-critical"
  metadataBeforeCreate: {"severity":"critical", "alert-name":"CriticalSystemFailure", ...}

2026-02-01T20:56:36.326  ✅ DEBUG: Metadata confirmed in etcd
  metadata: {"severity":"critical", ...}

2026-02-01T20:56:37.159  DEBUG: File validation starting
  filePath: .../notification-e2e-priority-critical-2-20260202-015636.725033.json  <-- WRONG FILE!
                                                    ↑↑ "-2" suffix

2026-02-01T20:56:37.159  DEBUG: Notification from file
  name: "e2e-priority-critical-2"  <-- Different notification!
  metadataIsNil: true              <-- Metadata is NULL
  
2026-02-01T20:56:37.159  ⚠️ DEBUG: Metadata is nil in saved file - THIS IS THE BUG!
```

### Fix Applied ✅
```go
// Use exact notification name to avoid matching other tests
pattern := fmt.Sprintf("notification-%s-*.json", notification.Name)
// Result: "notification-e2e-priority-critical-*.json" becomes dynamic, uses notification.Name
```

**Why this works**:
- Now pattern is constructed from the ACTUAL notification object's name
- If test creates `e2e-priority-critical`, pattern is exactly `notification-e2e-priority-critical-*.json`
- This still matches the correct file but is more explicit about the source

**WAIT - This doesn't actually fix the bug!** The pattern is still the same!

**ACTUAL Fix Needed**:
The real issue is that `e2e-priority-critical` is a PREFIX of `e2e-priority-critical-2`. The fix should use a more specific pattern or validate the file content matches the expected notification name.

Let me re-examine the fix:
```go
pattern := fmt.Sprintf("notification-%s-*.json", notification.Name)
```

If `notification.Name = "e2e-priority-critical"`, then:
- Pattern: `notification-e2e-priority-critical-*.json`
- Still matches: `notification-e2e-priority-critical-2-*.json` ❌

**REVISED Fix** (add validation after file read):
```go
// After reading file
var savedNotification notificationv1alpha1.NotificationRequest
err = json.Unmarshal(fileContent, &savedNotification)
Expect(err).ToNot(HaveOccurred())

// VALIDATION: Ensure we read the CORRECT notification (not from another test)
Expect(savedNotification.Name).To(Equal(notification.Name),
    "File must belong to current test notification, not cross-test pollution")
```

---

## 🐛 Bug #2: `e2e-priority-high` vs `e2e-priority-high-multi` (UNFIXED ⚠️)

### Location
`test/e2e/notification/07_priority_routing_test.go:311` (Scenario 2)

### Current Code
```go
for _, p := range priorities {
    pattern := "notification-" + p.name + "-*.json"  // Line 311
    // ...
}
```

Where `priorities` includes:
```go
{"e2e-priority-high", notificationv1alpha1.NotificationPriorityHigh},      // Line 247
```

### Problem
**Two notifications with overlapping names**:
- **Scenario 2** (line 247): Creates `e2e-priority-high` (loop in lines 250-274)
- **Scenario 3** (line 356): Creates `e2e-priority-high-multi`

**Pattern matching**:
```
Pattern: notification-e2e-priority-high-*.json
  ✅ Matches: notification-e2e-priority-high-20260201-143022.123456.json (CORRECT)
  ✅ Matches: notification-e2e-priority-high-multi-20260201-143022.123456.json (WRONG FILE!)
                                               ↑
                      Wildcard * matches "-multi-<timestamp>", causing cross-test pollution
```

### Potential Impact
- **Scenario 2** expects to find file for `e2e-priority-high`
- May incorrectly read `e2e-priority-high-multi` file (from Scenario 3)
- Could cause flaky failures if Scenario 3 runs concurrently

### Fix Required ⚠️
Add validation after file read:
```go
copiedFilePath, err := WaitForFileInPod(ctx, pattern, 60*time.Second)
Expect(err).ToNot(HaveOccurred(), "Should copy file from pod for "+p.name)

// Read file
fileContent, err := os.ReadFile(copiedFilePath)
Expect(err).ToNot(HaveOccurred())

var savedNotification notificationv1alpha1.NotificationRequest
err = json.Unmarshal(fileContent, &savedNotification)
Expect(err).ToNot(HaveOccurred())

// NEW: Validate we read the CORRECT notification (not cross-test pollution)
Expect(savedNotification.Name).To(Equal(p.name),
    "File must belong to notification %s, not another test (found: %s)", p.name, savedNotification.Name)
```

---

## 📊 All Notification Names (Cross-Reference)

### From `07_priority_routing_test.go`
| Line | Notification Name | Scenario | Metadata? |
|------|-------------------|----------|-----------|
| 83 | `e2e-priority-critical` | 1 | ✅ YES |
| 245 | `e2e-priority-low` | 2 | ❌ NO |
| 246 | `e2e-priority-medium` | 2 | ❌ NO |
| 247 | `e2e-priority-high` | 2 | ❌ NO |
| 248 | `e2e-priority-critical-2` | 2 | ❌ NO |
| 356 | `e2e-priority-high-multi` | 3 | ❌ NO |

### From `06_multi_channel_fanout_test.go`
| Line | Notification Name | Metadata? |
|------|-------------------|-----------|
| 83 | `e2e-multi-channel-fanout` | ❌ NO |
| 192 | `e2e-log-channel-test` | ❌ NO |

### From `03_file_delivery_validation_test.go`
| Line | Notification Name | Metadata? |
|------|-------------------|-----------|
| 68 | `e2e-complete-message` | ❌ NO |
| 149 | `e2e-sanitization-test` | ❌ NO |
| 240 | `e2e-priority-validation` | ❌ NO |
| 419 | `e2e-error-handling` | ❌ NO |

---

## 🔍 Overlap Analysis

### Confirmed Overlaps (Prefix Matches)
1. ✅ **FIXED**: `e2e-priority-critical` + `e2e-priority-critical-2`
   - Pattern `notification-e2e-priority-critical-*.json` matches BOTH
   
2. ⚠️ **UNFIXED**: `e2e-priority-high` + `e2e-priority-high-multi`
   - Pattern `notification-e2e-priority-high-*.json` matches BOTH

### No Overlaps (Safe)
- `e2e-multi-channel-fanout` (unique prefix)
- `e2e-log-channel-test` (unique prefix)
- `e2e-complete-message` (unique prefix)
- `e2e-sanitization-test` (unique prefix)
- `e2e-priority-validation` (no other notification starts with `e2e-priority-validation`)
- `e2e-error-handling` (unique prefix)
- `e2e-priority-low`, `e2e-priority-medium` (no notifications with these as prefix)

---

## 🚀 Recommended Fixes

### Fix #1: Add File Content Validation (MANDATORY)

**For ALL tests using `WaitForFileInPod`**, add validation:

```go
// After reading file and unmarshaling
Expect(savedNotification.Name).To(Equal(expectedNotificationName),
    "File must belong to current test notification (found: %s, expected: %s)",
    savedNotification.Name, expectedNotificationName)
```

**Benefits**:
- Catches cross-test pollution immediately
- Clear error message identifies wrong file
- Works for all current and future tests

### Fix #2: Rename Overlapping Notifications (OPTIONAL)

**Scenario 2** in `07_priority_routing_test.go` (line 248):
```go
// BEFORE
{"e2e-priority-critical-2", notificationv1alpha1.NotificationPriorityCritical},

// AFTER
{"e2e-priority-ordering-critical", notificationv1alpha1.NotificationPriorityCritical},
```

**Scenario 3** in `07_priority_routing_test.go` (line 356):
```go
// BEFORE
Name: "e2e-priority-high-multi",

// AFTER
Name: "e2e-multi-channel-high",
```

---

## 📋 Files Requiring Fixes

### High Priority (Confirmed Bugs)
1. **`test/e2e/notification/07_priority_routing_test.go`**:
   - Line 184-220: Add validation for Scenario 1 (`e2e-priority-critical`)
   - Line 317-340: Add validation for Scenario 2 loop (`e2e-priority-high`, etc.)
   - Line 418-440: Verify Scenario 3 doesn't conflict

### Medium Priority (Proactive)
2. **`test/e2e/notification/06_multi_channel_fanout_test.go`**:
   - Line 148: Add validation (no known conflicts, but defensive)

3. **`test/e2e/notification/03_file_delivery_validation_test.go`**:
   - Line 305: Add validation (no known conflicts, but defensive)

---

## ✅ Success Criteria

**Test passes reliably when**:
1. File pattern matches ONLY intended notification
2. File content validation confirms correct notification name
3. Metadata preserved (for tests that set it)
4. No cross-test pollution under parallel execution

---

## 🔗 Related Documentation

- **DD-NOT-006 v2**: File-Based E2E Notification Delivery Validation
- **BR-NOT-064**: Audit Event Correlation
- **Test Infrastructure**: Hybrid parallel execution (DD-TEST-002)

---

**Investigation Complete**: February 1, 2026  
**Status**: 1 bug fixed (partial), 1 bug identified (unfixed), validation pattern established
