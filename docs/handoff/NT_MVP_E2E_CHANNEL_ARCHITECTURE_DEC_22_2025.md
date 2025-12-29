# Notification (NT) Service - MVP E2E Test Plan Architectural Gap

**Date**: December 22, 2025
**From**: SP Team (SignalProcessing)
**To**: NT Team (Notification)
**Status**: 🚨 **BLOCKING MVP E2E TESTS** - API mismatch discovered
**Priority**: P0 - Blocks MVP E2E test execution

---

## 🎯 **Executive Summary**

The MVP E2E test plan ([TEST_PLAN_NT_V1_0_MVP.md](../services/crd-controllers/06-notification/TEST_PLAN_NT_V1_0_MVP.md)) references **file channel** and **FileDeliveryConfig** features that **do not exist** in the current NotificationRequest API. This blocks implementation of all 3 new MVP E2E tests.

**Impact**:
- ❌ Cannot implement E2E-1: Retry and Exponential Backoff (references file channel failures)
- ❌ Cannot implement E2E-2: Multi-Channel Fanout (references console + file channels)
- ❌ Cannot implement E2E-3: Priority Routing (references high → file, low → console)

**Root Cause**: Test plan assumes `ChannelFile` and `FileDeliveryConfig` exist as production features, but they are E2E testing utilities only.

---

## 🔍 **Problem Analysis**

### **What the Test Plan Assumes** (lines 222-296):

```go
// From test plan - does NOT compile
Channels: []notificationv1alpha1.Channel{
    notificationv1alpha1.ChannelConsole,
    notificationv1alpha1.ChannelFile,  // ❌ DOES NOT EXIST
}
FileDeliveryConfig: &notificationv1alpha1.FileDeliveryConfig{  // ❌ DOES NOT EXIST
    OutputDirectory: readOnlyDir,
}
```

### **What Actually Exists**:

**API (api/notification/v1alpha1/notificationrequest_types.go:52-62)**:
```go
type Channel string

const (
    ChannelEmail   Channel = "email"
    ChannelSlack   Channel = "slack"
    ChannelTeams   Channel = "teams"
    ChannelSMS     Channel = "sms"
    ChannelWebhook Channel = "webhook"
    ChannelConsole Channel = "console"
    // ❌ NO ChannelFile
)

// NotificationRequestSpec has NO FileDeliveryConfig field
```

**FileDeliveryService (pkg/notification/delivery/file.go)**:
- ✅ Exists as E2E testing utility
- ✅ Captures notifications to JSON files
- ⚠️ **NOT** a channel (transparent background operation)
- ⚠️ **NOT** configurable per-notification

---

## 📊 **Architectural Options**

### **Option A: Implement ChannelFile (File-Based Audit Trail)**

**Approach**: Elevate FileDeliveryService to production channel with per-notification configuration

**API Changes**:
```go
// api/notification/v1alpha1/notificationrequest_types.go

// Add to Channel enum
const (
    // ... existing channels ...
    ChannelFile    Channel = "file"     // NEW
)

// Add to NotificationRequestSpec
type NotificationRequestSpec struct {
    // ... existing fields ...

    // File delivery configuration (optional, required if ChannelFile specified)
    // +optional
    FileDeliveryConfig *FileDeliveryConfig `json:"fileDeliveryConfig,omitempty"`
}

// NEW: File delivery configuration
type FileDeliveryConfig struct {
    // Output directory for notification files
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    OutputDirectory string `json:"outputDirectory"`

    // File format (json only for MVP)
    // +kubebuilder:default=json
    // +optional
    Format string `json:"format,omitempty"`
}
```

**Orchestrator Changes (pkg/notification/delivery/orchestrator.go:182-199)**:
```go
func (o *Orchestrator) DeliverToChannel(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) error {
    switch channel {
    case notificationv1alpha1.ChannelConsole:
        return o.deliverToConsole(ctx, notification)
    case notificationv1alpha1.ChannelSlack:
        return o.deliverToSlack(ctx, notification)
    case notificationv1alpha1.ChannelFile:           // NEW
        return o.deliverToFile(ctx, notification)    // NEW
    default:
        return fmt.Errorf("unsupported channel: %s", channel)
    }
}

// NEW: File channel delivery
func (o *Orchestrator) deliverToFile(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) error {
    if o.fileService == nil {
        return fmt.Errorf("file service not configured")
    }

    // Validate FileDeliveryConfig exists when file channel specified
    if notification.Spec.FileDeliveryConfig == nil {
        return fmt.Errorf("FileDeliveryConfig required when using file channel")
    }

    // Sanitize before delivery
    sanitized := o.sanitizeNotification(notification)
    return o.fileService.Deliver(ctx, sanitized)
}
```

**FileDeliveryService Enhancement (pkg/notification/delivery/file.go:104-151)**:
```go
// Enhance Deliver() to use per-notification directory
func (s *FileDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
    log := ctrl.LoggerFrom(ctx)

    // NEW: Use per-notification output directory if specified
    outputDir := s.outputDir // Default E2E directory
    if notification.Spec.FileDeliveryConfig != nil && notification.Spec.FileDeliveryConfig.OutputDirectory != "" {
        outputDir = notification.Spec.FileDeliveryConfig.OutputDirectory
    }

    // Ensure output directory exists
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return fmt.Errorf("failed to create output directory: %w", err)
    }

    // ... rest of existing implementation ...
}
```

**Pros**:
- ✅ Matches MVP test plan exactly
- ✅ Enables audit trail use cases
- ✅ Supports E2E read-only directory tests (simulates failures)
- ✅ Reuses existing FileDeliveryService implementation

**Cons**:
- ⚠️ Adds CRD API complexity
- ⚠️ Requires CRD regeneration (`make manifests`)
- ⚠️ File system errors can fail notifications (operational risk)

**Estimated Effort**: 4-6 hours
- API changes: 1 hour
- Orchestrator integration: 1 hour
- FileDeliveryService enhancement: 1 hour
- CRD regeneration + deployment: 1 hour
- Testing + validation: 2 hours

---

### **Option B: Implement ChannelLog (Structured Log Channel)**

**Approach**: New log channel for JSON Lines output to stdout (observability-focused)

**API Changes**:
```go
// Add to Channel enum
const (
    // ... existing channels ...
    ChannelLog     Channel = "log"      // NEW - structured JSON logs
)

// NO FileDeliveryConfig needed - logs go to stdout
```

**New Service (pkg/notification/delivery/log.go)**:
```go
package delivery

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// LogDeliveryService outputs notifications as JSON Lines to stdout
// Use case: Log aggregation systems (Loki, Elasticsearch)
type LogDeliveryService struct{}

func NewLogDeliveryService() *LogDeliveryService {
    return &LogDeliveryService{}
}

// Deliver outputs notification as single-line JSON to stdout
func (s *LogDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
    if notification == nil {
        return fmt.Errorf("notification cannot be nil")
    }

    // Create structured log entry
    logEntry := map[string]interface{}{
        "timestamp":  time.Now().UTC().Format(time.RFC3339),
        "level":      "INFO",
        "component":  "notification-controller",
        "event_type": "notification.delivered",
        "notification": map[string]interface{}{
            "name":      notification.Name,
            "namespace": notification.Namespace,
            "type":      notification.Spec.Type,
            "priority":  notification.Spec.Priority,
            "subject":   notification.Spec.Subject,
            "body":      notification.Spec.Body,
        },
    }

    // Output as single-line JSON (JSON Lines format)
    jsonBytes, err := json.Marshal(logEntry)
    if err != nil {
        return fmt.Errorf("failed to marshal log entry: %w", err)
    }

    fmt.Println(string(jsonBytes))
    return nil
}
```

**Orchestrator Integration**: Same pattern as Option A switch case

**Pros**:
- ✅ No CRD changes required
- ✅ Leverages existing log infrastructure
- ✅ Perfect for observability stacks (Loki, ES)
- ✅ No file system operational risk

**Cons**:
- ❌ **Does NOT match MVP test plan** (plan references "file channel")
- ⚠️ Cannot simulate file write failures (test plan requirement)
- ⚠️ Test plan would need rewrite

**Estimated Effort**: 3-4 hours
- New LogDeliveryService: 1 hour
- Orchestrator integration: 1 hour
- Testing: 1-2 hours

---

### **Option C: Implement Both ChannelFile + ChannelLog**

**Approach**: Comprehensive channel architecture supporting both use cases

**Changes**: Combine Option A + Option B

**Pros**:
- ✅ Matches MVP test plan (ChannelFile)
- ✅ Provides observability path (ChannelLog)
- ✅ Future-proof architecture
- ✅ Clear separation of concerns

**Cons**:
- ⚠️ Larger scope (8-10 hours)
- ⚠️ More API surface to maintain

**Estimated Effort**: 8-10 hours
- All Option A work: 4-6 hours
- All Option B work: 3-4 hours

---

### **Option D: Fix Test Plan (No Implementation)**

**Approach**: Rewrite MVP E2E tests to use existing API (console + slack channels)

**Changes**:
- Remove `ChannelFile` references from E2E tests
- Use mock Slack server for retry/failure testing
- Deploy mock Slack in Kind cluster (similar to DataStorage PostgreSQL)
- Validate delivery via FileDeliveryService output (existing E2E pattern)

**Pros**:
- ✅ Zero API changes
- ✅ Uses existing integration test patterns
- ✅ Leverages existing mock infrastructure

**Cons**:
- ⚠️ Test plan doesn't match original vision
- ⚠️ Cannot test file-specific failure modes (read-only directories)
- ⚠️ MVP test plan document becomes misleading

**Estimated Effort**: 2-3 hours
- Rewrite 3 E2E test files: 2 hours
- Update test plan document: 1 hour

---

## 🎯 **SP Team Recommendation**

**Recommended Approach**: **Option C** (Implement Both Channels)

**Rationale**:
1. **Matches MVP Vision**: Test plan clearly describes file channel use cases (audit, compliance)
2. **Complete Architecture**: Both channels serve legitimate production needs
3. **Long-term Value**: Observability (log) + compliance (file) are both critical
4. **Clean Implementation**: Existing patterns make this straightforward

**Alternative**: **Option A** (File Only) if time-constrained, add Log channel in V1.1

---

## 📋 **Implementation Checklist** (for Option C)

### **Phase 1: API Changes** (2 hours)
- [ ] Add `ChannelFile` to Channel enum
- [ ] Add `ChannelLog` to Channel enum
- [ ] Add `FileDeliveryConfig` struct to types
- [ ] Run `make manifests` to regenerate CRDs
- [ ] Update API documentation

### **Phase 2: Delivery Services** (3 hours)
- [ ] Enhance FileDeliveryService with per-notification directory support
- [ ] Create LogDeliveryService (new file)
- [ ] Add both services to Orchestrator struct
- [ ] Update Orchestrator.DeliverToChannel switch
- [ ] Add deliverToFile() and deliverToLog() methods

### **Phase 3: Controller Integration** (2 hours)
- [ ] Update cmd/notification/main.go to wire services
- [ ] Update controller instantiation
- [ ] Test manual notification delivery

### **Phase 4: E2E Tests** (2 hours)
- [ ] Fix compilation errors in 3 new E2E test files
- [ ] Run tests locally (Kind cluster)
- [ ] Validate file channel retry behavior
- [ ] Validate multi-channel fanout
- [ ] Validate priority routing

### **Phase 5: Documentation** (1 hour)
- [ ] Create DD-XXX design decision entry
- [ ] Update Notification service documentation
- [ ] Update E2E test plan with final implementation details

**Total Estimated Effort**: 10 hours (1.5 days)

---

## 📚 **Reference Files**

### **Files SP Team Created** (Handoff to NT Team):
- `test/e2e/notification/05_retry_exponential_backoff_test.go` (250 LOC) - ❌ Does not compile
- `test/e2e/notification/06_multi_channel_fanout_test.go` (370 LOC) - ❌ Does not compile
- `test/e2e/notification/07_priority_routing_test.go` (380 LOC) - ❌ Does not compile

### **Files NT Team Needs to Modify**:
- `api/notification/v1alpha1/notificationrequest_types.go` - Add ChannelFile, ChannelLog, FileDeliveryConfig
- `pkg/notification/delivery/orchestrator.go` - Add channel routing
- `pkg/notification/delivery/file.go` - Enhance with per-notification directory
- `pkg/notification/delivery/log.go` - **NEW FILE** - Create log service
- `cmd/notification/main.go` - Wire new channels
- `internal/controller/notification/notificationrequest_controller.go` - (No changes needed - uses orchestrator)

### **Reference Patterns**:
- **Existing Console Channel**: `pkg/notification/delivery/console.go` (58 lines)
- **Existing Slack Channel**: `pkg/notification/delivery/slack.go`
- **Existing File Service**: `pkg/notification/delivery/file.go` (164 lines) - **REUSE THIS**
- **Orchestrator Pattern**: `pkg/notification/delivery/orchestrator.go:182-227`

---

## 🤝 **Handoff Actions**

**SP Team** (Done):
- ✅ Analyzed API mismatch
- ✅ Created 3 MVP E2E test files (compilation blocked)
- ✅ Documented architectural options
- ✅ Provided implementation guidance

**NT Team** (Next Steps):
1. **Decide**: Choose Option A, B, C, or D (recommendation: C)
2. **Create DD-XXX**: Document design decision per [14-design-decisions-documentation.mdc](../../.cursor/rules/14-design-decisions-documentation.mdc)
3. **Implement**: Follow checklist above
4. **Validate**: Run E2E tests and confirm MVP coverage
5. **Notify SP Team**: When implementation complete (we can assist with test validation)

---

## 📞 **Contact**

**Questions?**
- Slack: #kubernaut-notification-service
- Document: This handoff doc
- Code Review: Tag @nt-team when ready

**SP Team Status**:
- ✅ SignalProcessing E2E tests: 100% passing (all unit/integration/e2e)
- ⏸️ Notification E2E tests: Blocked on API implementation
- 🎯 Next: Awaiting NT team decision on channel architecture

---

**End of Handoff Document**

