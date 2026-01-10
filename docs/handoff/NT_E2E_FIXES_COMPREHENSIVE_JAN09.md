# Notification E2E Comprehensive Fixes (Jan 9, 2026)

## 🎯 **EXECUTIVE SUMMARY**

**Current**: 15/20 PASSING (75%)
**Target**: 19/20 PASSING (95%) - 1 test marked Pending
**Time**: ~30 minutes
**Confidence**: 95%

---

## 🔧 **FIX 1: Race Condition - File Sync Timing (3 tests)**

### **Problem**
Files written in pod take 200-600ms to sync to macOS host (Podman VM overhead).
Tests check immediately and fail.

### **Root Cause** 
```go
// CURRENT (BROKEN):
files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-*.json"))
Expect(len(files)).To(BeNumerically(">=", 1)) // ❌ Fails - checks too soon
```

### **Solution Pattern**
```go
// FIXED:
Eventually(func() int {
    files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-*.json"))
    return len(files)
}, 2*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
    "File should appear on host within 2 seconds (macOS Podman sync delay)")
```

### **Files to Fix**

#### **File 1: test/e2e/notification/03_file_delivery_validation_test.go:277**
**Test**: "should preserve priority field in delivered notification file"

**Find:**
```go
By("Verifying file channel created notification file")
files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-file-critical-*.json"))
Expect(err).ToNot(HaveOccurred())
Expect(len(files)).To(BeNumerically(">=", 1),
    "File channel should create at least one notification file in shared directory")
```

**Replace:**
```go
By("Verifying file channel created notification file")
// DD-NOT-006 v2: Add explicit wait for file sync (macOS Podman VM delay)
// Files take 200-600ms to sync from pod to host via hostPath volume
Eventually(func() int {
    files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-file-critical-*.json"))
    return len(files)
}, 2*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
    "File should appear on host within 2 seconds (macOS Podman sync delay)")

files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-file-critical-*.json"))
Expect(err).ToNot(HaveOccurred())
```

---

#### **File 2: test/e2e/notification/07_priority_routing_test.go:236**
**Test**: "should deliver notifications in priority order"

**Find** (around line 236):
```go
By("Verifying file audit trail was created for all notifications")
files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-priority-*.json"))
Expect(err).ToNot(HaveOccurred())
Expect(len(files)).To(BeNumerically(">=", 4),
    "File channel should create audit trail for all 4 priority levels in shared directory")
```

**Replace:**
```go
By("Verifying file audit trail was created for all notifications")
// DD-NOT-006 v2: Add explicit wait for file sync (macOS Podman VM delay)
Eventually(func() int {
    files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-priority-*.json"))
    return len(files)
}, 2*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 4),
    "All 4 files should appear within 2 seconds (macOS Podman sync delay)")

files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-priority-*.json"))
Expect(err).ToNot(HaveOccurred())
```

---

#### **File 3: test/e2e/notification/07_priority_routing_test.go:331**
**Test**: "should deliver high priority notification to all channels"

**Find** (around line 331):
```go
By("Verifying file delivery created notification file")
files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-high-priority-*.json"))
Expect(err).ToNot(HaveOccurred())
Expect(len(files)).To(BeNumerically(">=", 1),
    "File channel should create notification file in shared directory")
```

**Replace:**
```go
By("Verifying file delivery created notification file")
// DD-NOT-006 v2: Add explicit wait for file sync (macOS Podman VM delay)
Eventually(func() int {
    files, _ := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-high-priority-*.json"))
    return len(files)
}, 2*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 1),
    "File should appear within 2 seconds (macOS Podman sync delay)")

files, err := filepath.Glob(filepath.Join(e2eFileOutputDir, "notification-e2e-high-priority-*.json"))
Expect(err).ToNot(HaveOccurred())
```

---

## 🔧 **FIX 2: ogen Discriminated Union - EventData Access (1 test)**

### **Problem**
Test tries to access `event.EventData.NotificationMessageSentPayload` but this field doesn't exist.
The correct field is `NotificationAuditPayload` and it uses `OptString` wrappers.

### **Root Cause**
```go
// CURRENT (BROKEN):
notificationID := ""
if event.EventData.Type != "" {
    // ❌ Wrong field name
    notificationID = event.EventData.NotificationMessageSentPayload.NotificationID
}
```

### **Solution**
```go
// FIXED:
notificationID := ""
if event.EventData.Type != "" && event.EventData.NotificationAuditPayload.NotificationID.IsSet() {
    notificationID = event.EventData.NotificationAuditPayload.NotificationID.Value
}
```

### **File to Fix**

#### **File: test/e2e/notification/02_audit_correlation_test.go:226-230**
**Test**: "should generate correlated audit events"

**Find:**
```go
		for _, event := range events {
		// Extract notification_id from EventData discriminated union
		// ogen: Access specific payload type from discriminated union
		notificationID := ""
		if event.EventData.Type != "" {
			// Access NotificationMessageSentPayload from discriminated union
			notificationID = event.EventData.NotificationMessageSentPayload.NotificationID
		}
```

**Replace:**
```go
		for _, event := range events {
		// Extract notification_id from EventData discriminated union
		// ogen: NotificationAuditPayload is the correct field for notification events
		// NotificationID is wrapped in OptString (ogen optional type)
		notificationID := ""
		if event.EventData.Type != "" && event.EventData.NotificationAuditPayload.NotificationID.IsSet() {
			notificationID = event.EventData.NotificationAuditPayload.NotificationID.Value
		}
```

---

## 🔧 **FIX 3: Mark Partial Delivery Test as Pending (1 test)**

### **Problem**
Test expects file delivery to fail by specifying invalid directory, but after DD-NOT-006 v2:
- `FileDeliveryConfig` removed from CRD
- File delivery now uses service-level configuration (`/tmp/notifications`)
- No way to simulate file write failure in E2E tests

### **Root Cause**
```go
// Test assumes:
Channels: []Channel{
    ChannelFile, // Expected to fail - but actually succeeds
}
// Expected: Phase = Retrying
// Actual:   Phase = Sent (file delivery succeeds)
```

### **Solution**
Mark test as `PIt` (Pending) with clear explanation, similar to `05_retry_exponential_backoff_test.go`.

### **File to Fix**

#### **File: test/e2e/notification/06_multi_channel_fanout_test.go:176**
**Test**: "should mark as PartiallySent when file delivery fails"

**Find:**
```go
	// ========================================
	// Scenario 2: Partial Failure Handling
	// ========================================
	Context("Scenario 2: One channel fails, others succeed", func() {
		It("should mark as PartiallySent when file delivery fails but console/log succeed", func() {
```

**Replace:**
```go
	// ========================================
	// Scenario 2: Partial Failure Handling
	// ========================================
	Context("Scenario 2: One channel fails, others succeed", func() {
		// PIt: This test is currently pending because the FileDeliveryConfig field
		// was removed from the CRD (DD-NOT-006 v2).
		// The controller now writes to a fixed, configured output directory,
		// making it impossible for E2E tests to specify an invalid directory
		// to simulate file delivery failures.
		//
		// Re-enable this test if a new mechanism for simulating file write failures
		// (e.g., a mock filesystem, in-memory adapter, or test-only configuration) 
		// is introduced.
		//
		// Related: 05_retry_exponential_backoff_test.go (also pending for same reason)
		PIt("should mark as PartiallySent when file delivery fails but console/log succeed", func() {
```

---

## 📊 **EXPECTED RESULTS AFTER FIXES**

```
BEFORE: 15/20 PASSING (75%)
        ↓
Fix 1:  Race conditions fixed (3 tests)
        18/20 PASSING (90%)
        ↓
Fix 2:  EventData extraction fixed (1 test)
        19/20 PASSING (95%)
        ↓
Fix 3:  Partial delivery marked Pending (1 test)
        19/19 PASSING (100% of active tests)
        2 PENDING (05_retry, 06_multi_channel_fanout scenario 2)
```

---

## 🎯 **VALIDATION COMMANDS**

### **Apply all fixes:**
```bash
# Apply Fix 1 (race conditions)
# Apply Fix 2 (EventData)
# Apply Fix 3 (mark pending)
```

### **Run tests:**
```bash
make test-e2e-notification
```

### **Expected output:**
```
✅ 19 Passed | ❌ 0 Failed | ⏸️  2 Pending | ⏭️  0 Skipped
```

---

## 📝 **TECHNICAL NOTES**

### **ogen Discriminated Union Pattern**
```go
// Discriminated union structure:
type AuditEventEventData struct {
    Type                      AuditEventEventDataType // Enum: "notification.message.sent"
    NotificationAuditPayload  NotificationAuditPayload // Payload for notification events
    // ... other service payloads ...
}

type NotificationAuditPayload struct {
    NotificationID  OptString  // Optional wrapper (ogen pattern)
    // ... other fields ...
}

// Correct access pattern:
if event.EventData.NotificationAuditPayload.NotificationID.IsSet() {
    id := event.EventData.NotificationAuditPayload.NotificationID.Value
}
```

### **macOS Podman File Sync Timing**
```
Write in container:  T+0ms    (instant)
Sync to Kind node:   T+100ms  (overlay FS)
Sync to Podman VM:   T+400ms  (VM + FUSE overhead)
Visible on host:     T+200-600ms (total)
```

**Solution**: `Eventually()` with 2s timeout, 200ms polling
**Impact**: +1.6s per file check (acceptable for E2E)

---

## 🔗 **RELATED DOCUMENTS**

- **Design**: `docs/handoff/NT_FILE_DELIVERY_CONFIG_DESIGN_ISSUE_JAN08.md`
- **Root Cause**: `docs/handoff/NT_FILE_DELIVERY_ROOT_CAUSE_RESOLVED_JAN09.md`
- **Timing Analysis**: `docs/handoff/NT_KIND_LOGS_TRIAGE_JAN09.md`
- **Triage**: `docs/handoff/NT_E2E_TRIAGE_FINAL_JAN09.md`

---

**Authority**: DD-NOT-006 v2, BR-NOTIFICATION-001
**Status**: Fixes documented, ready to apply
**Next**: Apply fixes sequentially, run tests after each fix to verify
