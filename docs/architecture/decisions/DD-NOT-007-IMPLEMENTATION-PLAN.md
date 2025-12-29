# DD-NOT-007: Implementation Plan - Registration Pattern Refactoring

**Date**: December 22, 2025
**Status**: 🎯 **READY TO IMPLEMENT**
**Authority**: Implements DD-NOT-007 (Authoritative Channel Architecture)
**Methodology**: APDC-Enhanced TDD
**Estimated Effort**: 2-3 hours

---

## 📋 **APDC Methodology Application**

### **Analysis Phase** ✅ (Complete - 10 minutes)

**Business Context**:
- **BR-NOT-053**: At-least-once delivery (unaffected by refactoring)
- **BR-NOT-055**: Retry logic (unaffected by refactoring)
- **BR-NOT-034**: Audit trail (unaffected by refactoring)
- **Technical Debt**: Hardcoded channel constructor parameters limit extensibility

**Technical Context**:
- **Interface exists**: `DeliveryService` (DD-NOT-002 V3.0)
- **4 channels**: Console, Slack, File, Log
- **3 usage sites**: production + integration tests + E2E tests
- **Switch statement**: 4 cases in `DeliverToChannel()`

**Integration Context**:
- **Production**: `cmd/notification/main.go` (lines 295-304)
- **Integration**: `test/integration/notification/suite_test.go` (lines 282-290)
- **Controller**: `internal/controller/notification/notificationrequest_controller.go`

**Complexity Assessment**: 🟢 **LOW-MEDIUM**
- Well-defined scope (single package)
- Interface already exists
- Clear migration path
- No breaking business logic changes

---

### **Plan Phase** ✅ (Complete - 15 minutes)

#### **TDD Strategy**

**TDD RED Phase** (Write failing tests):
1. Add unit tests for `RegisterChannel()` method
2. Add unit tests for map-based routing
3. Add unit tests for channel not registered error
4. Tests will fail (methods don't exist yet)

**TDD GREEN Phase** (Minimal implementation):
1. Add `channels map[string]DeliveryService` field
2. Implement `RegisterChannel()`, `UnregisterChannel()`, `HasChannel()`
3. Update `DeliverToChannel()` to use map lookup
4. Update constructor to accept no channel parameters
5. Update production usage (`cmd/notification/main.go`)
6. Update test usage (`test/integration/notification/suite_test.go`)

**TDD REFACTOR Phase** (Clean up):
1. Remove old channel fields from struct
2. Remove individual `deliverToConsole()`, `deliverToSlack()`, etc. methods
3. Remove switch statement completely
4. Update documentation

#### **Integration Plan**

**Files to Modify**:
1. `pkg/notification/delivery/orchestrator.go` - Core refactoring
2. `cmd/notification/main.go` - Production usage
3. `test/integration/notification/suite_test.go` - Integration test usage
4. `test/e2e/notification/*_test.go` - E2E test usage (if needed)

**Success Criteria**:
- ✅ All existing tests pass
- ✅ New registration tests pass
- ✅ Production builds successfully
- ✅ No switch statement in orchestrator
- ✅ No channel fields in Orchestrator struct

#### **Risk Mitigation**

**Risk 1**: Breaking existing functionality
- **Mitigation**: Run full test suite after each TDD phase
- **Validation**: Integration tests + E2E tests

**Risk 2**: Forgetting to register a channel
- **Mitigation**: Clear error message "channel not registered: X"
- **Validation**: Unit tests verify error messages

**Risk 3**: Performance regression
- **Mitigation**: Map lookup is O(1), same as switch statement
- **Validation**: Benchmark if concerned (not expected to be needed)

#### **Timeline**

| Phase | Duration | Deliverable |
|-------|----------|------------|
| **TDD RED** | 30 min | Failing registration tests |
| **TDD GREEN** | 60 min | Registration implementation + usage updates |
| **TDD REFACTOR** | 30 min | Remove legacy code |
| **Validation** | 15 min | Full test suite run |
| **Total** | **2h 15min** | DD-NOT-007 compliant code |

---

## 🧪 **Detailed Implementation Plan**

### **Phase 1: TDD RED - Write Failing Tests** (30 min)

#### **1.1: Create Registration Test File**

**File**: `pkg/notification/delivery/orchestrator_registration_test.go`

```go
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package delivery_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

// Mock delivery service for testing
type mockDeliveryService struct {
	deliverFunc func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}

func (m *mockDeliveryService) Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	if m.deliverFunc != nil {
		return m.deliverFunc(ctx, notification)
	}
	return nil
}

var _ = Describe("Orchestrator Channel Registration (DD-NOT-007)", func() {
	var (
		orchestrator  *delivery.Orchestrator
		mockService   *mockDeliveryService
		sanitizer     *sanitization.Sanitizer
		metrics       notificationmetrics.Recorder
		statusManager *notificationstatus.Manager
		logger        = ctrl.Log.WithName("test-orchestrator")
		ctx           = context.Background()
	)

	BeforeEach(func() {
		// Create orchestrator WITHOUT channel parameters (DD-NOT-007)
		orchestrator = delivery.NewOrchestrator(
			sanitizer,
			metrics,
			statusManager,
			logger,
		)

		// Create mock service
		mockService = &mockDeliveryService{
			deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
				return nil // Success by default
			},
		}
	})

	Describe("RegisterChannel", func() {
		It("should register a new channel successfully", func() {
			// Register mock service
			orchestrator.RegisterChannel("test-channel", mockService)

			// Verify channel is registered
			Expect(orchestrator.HasChannel("test-channel")).To(BeTrue())
		})

		It("should skip registration if service is nil", func() {
			// Attempt to register nil service
			orchestrator.RegisterChannel("nil-channel", nil)

			// Verify channel is NOT registered
			Expect(orchestrator.HasChannel("nil-channel")).To(BeFalse())
		})

		It("should allow overwriting existing channel", func() {
			// Register first service
			firstService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("first service")
				},
			}
			orchestrator.RegisterChannel("overwrite-channel", firstService)

			// Register second service (overwrite)
			secondService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("second service")
				},
			}
			orchestrator.RegisterChannel("overwrite-channel", secondService)

			// Verify second service is used
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, "overwrite-channel")
			Expect(err).To(MatchError("second service"))
		})
	})

	Describe("UnregisterChannel", func() {
		It("should remove a registered channel", func() {
			// Register channel
			orchestrator.RegisterChannel("remove-me", mockService)
			Expect(orchestrator.HasChannel("remove-me")).To(BeTrue())

			// Unregister channel
			orchestrator.UnregisterChannel("remove-me")
			Expect(orchestrator.HasChannel("remove-me")).To(BeFalse())
		})

		It("should be safe to unregister non-existent channel", func() {
			// Unregister non-existent channel (should not panic)
			Expect(func() {
				orchestrator.UnregisterChannel("non-existent")
			}).NotTo(Panic())
		})
	})

	Describe("HasChannel", func() {
		It("should return true for registered channel", func() {
			orchestrator.RegisterChannel("exists", mockService)
			Expect(orchestrator.HasChannel("exists")).To(BeTrue())
		})

		It("should return false for unregistered channel", func() {
			Expect(orchestrator.HasChannel("does-not-exist")).To(BeFalse())
		})
	})

	Describe("DeliverToChannel with Registration (DD-NOT-007)", func() {
		It("should deliver to registered channel successfully", func() {
			// Register channel
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockService)

			// Attempt delivery
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return error if channel not registered", func() {
			// Do NOT register channel

			// Attempt delivery to unregistered channel
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)

			// Verify descriptive error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("channel not registered"))
			Expect(err.Error()).To(ContainSubstring("console"))
		})

		It("should delegate to registered service", func() {
			// Track if service was called
			called := false
			trackedService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					called = true
					return nil
				},
			}

			// Register tracked service
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), trackedService)

			// Deliver
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelSlack)

			// Verify service was called
			Expect(err).ToNot(HaveOccurred())
			Expect(called).To(BeTrue())
		})

		It("should propagate service errors", func() {
			// Service that returns error
			failingService := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return errors.New("delivery failed")
				},
			}

			// Register failing service
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), failingService)

			// Deliver
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelFile)

			// Verify error propagated
			Expect(err).To(MatchError("delivery failed"))
		})
	})

	Describe("Registration Flexibility for Tests", func() {
		It("should allow registering only needed channels", func() {
			// Test scenario: Only need console for this test
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), mockService)

			// Should succeed for registered channel
			notification := testutil.NewNotification("test")
			err := orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)
			Expect(err).ToNot(HaveOccurred())

			// Should fail for unregistered channels
			err = orchestrator.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelSlack)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("channel not registered: slack"))
		})
	})

	Describe("DD-NOT-007 Compliance", func() {
		It("should have no hardcoded channel fields in struct", func() {
			// This test verifies the refactoring removed hardcoded fields
			// If struct still has consoleService, slackService fields, this documents the issue

			// Create orchestrator
			o := delivery.NewOrchestrator(sanitizer, metrics, statusManager, logger)

			// Verify channels must be registered
			Expect(o.HasChannel("console")).To(BeFalse(), "console should not be hardcoded")
			Expect(o.HasChannel("slack")).To(BeFalse(), "slack should not be hardcoded")
			Expect(o.HasChannel("file")).To(BeFalse(), "file should not be hardcoded")
			Expect(o.HasChannel("log")).To(BeFalse(), "log should not be hardcoded")
		})
	})
})
```

**Expected Result**: ❌ **Tests FAIL** (methods don't exist yet)

**Validation**:
```bash
cd pkg/notification/delivery
go test -v -run "Orchestrator Channel Registration"
# Expected: compilation errors or test failures
```

---

### **Phase 2: TDD GREEN - Minimal Implementation** (60 min)

#### **2.1: Update Orchestrator Struct**

**File**: `pkg/notification/delivery/orchestrator.go`

**Changes**:

```go
// Orchestrator manages delivery orchestration across channels.
// DD-NOT-007: Refactored to use registration pattern instead of hardcoded channels
type Orchestrator struct {
	// Channel registry (DD-NOT-007: Dynamic registration)
	channels map[string]DeliveryService

	// Dependencies
	sanitizer     *sanitization.Sanitizer
	metrics       notificationmetrics.Recorder
	statusManager *notificationstatus.Manager

	// Logger
	logger logr.Logger
}
```

#### **2.2: Update Constructor**

```go
// NewOrchestrator creates a new delivery orchestrator.
//
// DD-NOT-007: Refactored to remove hardcoded channel parameters.
// Channels are now registered dynamically via RegisterChannel().
//
// Migration from legacy pattern:
//   OLD: NewOrchestrator(console, slack, file, log, sanitizer, metrics, status, logger)
//   NEW: orchestrator := NewOrchestrator(sanitizer, metrics, status, logger)
//        orchestrator.RegisterChannel("console", console)
//        orchestrator.RegisterChannel("slack", slack)
func NewOrchestrator(
	sanitizer *sanitization.Sanitizer,
	metrics notificationmetrics.Recorder,
	statusManager *notificationstatus.Manager,
	logger logr.Logger,
) *Orchestrator {
	return &Orchestrator{
		channels:      make(map[string]DeliveryService),
		sanitizer:     sanitizer,
		metrics:       metrics,
		statusManager: statusManager,
		logger:        logger,
	}
}
```

#### **2.3: Add Registration Methods**

```go
// RegisterChannel registers a delivery service for a specific channel.
//
// DD-NOT-007: Dynamic channel registration pattern.
// This allows channels to be added/removed without modifying the orchestrator.
//
// Parameters:
//   - channelName: The channel identifier (e.g., "console", "slack")
//   - service: The DeliveryService implementation for this channel
//
// Nil services are ignored with a log message.
// Registering the same channel twice overwrites the previous registration.
func (o *Orchestrator) RegisterChannel(channelName string, service DeliveryService) {
	if service == nil {
		o.logger.Info("Skipping nil service registration", "channel", channelName)
		return
	}
	o.channels[channelName] = service
	o.logger.V(1).Info("Registered delivery channel", "channel", channelName)
}

// UnregisterChannel removes a channel from the registry.
//
// DD-NOT-007: Useful for testing scenarios where channels need to be removed.
// Safe to call on non-existent channels (no-op).
func (o *Orchestrator) UnregisterChannel(channelName string) {
	delete(o.channels, channelName)
	o.logger.V(1).Info("Unregistered delivery channel", "channel", channelName)
}

// HasChannel checks if a channel is registered.
//
// DD-NOT-007: Validation helper for tests and runtime checks.
func (o *Orchestrator) HasChannel(channelName string) bool {
	_, exists := o.channels[channelName]
	return exists
}
```

#### **2.4: Update DeliverToChannel (Map-Based Routing)**

```go
// DeliverToChannel attempts delivery to a specific channel.
//
// DD-NOT-007: Refactored to use map-based routing instead of switch statement.
// Channels must be registered via RegisterChannel() before use.
//
// Returns:
//   - nil if delivery succeeds
//   - error if channel not registered or delivery fails
func (o *Orchestrator) DeliverToChannel(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
) error {
	// DD-NOT-007: Map-based routing (no switch statement)
	service, exists := o.channels[string(channel)]
	if !exists {
		return fmt.Errorf("channel not registered: %s", channel)
	}

	// Sanitize before delivery
	sanitized := o.sanitizeNotification(notification)

	// Delegate to registered service
	return service.Deliver(ctx, sanitized)
}
```

#### **2.5: Update Production Usage**

**File**: `cmd/notification/main.go`

**Find and replace** (lines ~295-304):

```go
// OLD (DELETE THIS):
deliveryOrchestrator := delivery.NewOrchestrator(
	consoleService,
	slackService,
	fileService,
	logService,
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// NEW (DD-NOT-007 COMPLIANT):
// ========================================
// Create delivery orchestrator (DD-NOT-007: Registration Pattern)
// Channels are registered dynamically for extensibility
// ========================================
deliveryOrchestrator := delivery.NewOrchestrator(
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// Register delivery channels (DD-NOT-007)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), fileService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), logService)
logger.Info("Delivery Orchestrator initialized with registration pattern (DD-NOT-007)")
```

#### **2.6: Update Integration Test Usage**

**File**: `test/integration/notification/suite_test.go`

**Find and replace** (lines ~282-290):

```go
// OLD (DELETE THIS):
deliveryOrchestrator := delivery.NewOrchestrator(
	consoleService,
	slackService,
	nil, // fileService (E2E only, not needed in integration tests)
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// NEW (DD-NOT-007 COMPLIANT):
// Pattern 3: Create Delivery Orchestrator (DD-NOT-007: Registration Pattern)
deliveryOrchestrator := delivery.NewOrchestrator(
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// Register only channels needed for integration tests (DD-NOT-007)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
// NOTE: fileService NOT registered (E2E only) - will return clear error if used
```

**Expected Result**: ✅ **Tests PASS** (registration works)

**Validation**:
```bash
# Unit tests
cd pkg/notification/delivery
go test -v -run "Orchestrator Channel Registration"

# Integration tests
make test-integration-notification

# Production build
make build-notification
```

---

### **Phase 3: TDD REFACTOR - Remove Legacy Code** (30 min)

#### **3.1: Remove Individual Delivery Methods**

**File**: `pkg/notification/delivery/orchestrator.go`

**Delete these methods** (no longer needed with map-based routing):

```go
// ❌ DELETE: deliverToConsole (lines ~212-224)
// ❌ DELETE: deliverToSlack (lines ~226-238)
// ❌ DELETE: deliverToFile (lines ~240-255)
// ❌ DELETE: deliverToLog (lines ~257-272)
```

**Rationale**: `DeliverToChannel()` now uses map lookup, these methods are redundant

#### **3.2: Verify No Switch Statement**

**File**: `pkg/notification/delivery/orchestrator.go`

**Ensure removed**:
```go
// ❌ OLD CODE (should be gone):
switch channel {
	case notificationv1alpha1.ChannelConsole:
		return o.deliverToConsole(ctx, notification)
	case notificationv1alpha1.ChannelSlack:
		return o.deliverToSlack(ctx, notification)
	// ...
}
```

**✅ NEW CODE (should exist)**:
```go
service, exists := o.channels[string(channel)]
if !exists {
	return fmt.Errorf("channel not registered: %s", channel)
}
return service.Deliver(ctx, sanitized)
```

#### **3.3: Update Comments and Documentation**

**File**: `pkg/notification/delivery/orchestrator.go`

**Update struct comment**:
```go
// Orchestrator manages delivery orchestration across channels.
// DD-NOT-007: Uses registration pattern for extensibility.
//
// Channels must be registered via RegisterChannel() before use.
// This eliminates the need for hardcoded channel parameters and switch statements.
//
// See: docs/architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md
```

**Expected Result**: ✅ **Clean code, no legacy patterns**

**Validation**:
```bash
# Verify no switch statements
grep -n "switch channel" pkg/notification/delivery/orchestrator.go
# Expected: no results

# Verify no deliverToX methods
grep -n "deliverToConsole\|deliverToSlack\|deliverToFile\|deliverToLog" pkg/notification/delivery/orchestrator.go
# Expected: no results

# Run full test suite
make test
```

---

## 🧪 **Comprehensive Test Plan**

### **Test Strategy Overview**

| Test Level | Coverage | Purpose | Tool |
|------------|----------|---------|------|
| **Unit Tests** | Registration logic | Verify registration methods work | Ginkgo |
| **Integration Tests** | End-to-end delivery | Verify delivery still works | Ginkgo + Kind |
| **E2E Tests** | Full system | Verify production behavior | Ginkgo + Kind |
| **Manual Tests** | Production deployment | Verify controller starts | kubectl |

---

### **Test Level 1: Unit Tests** (70%+ coverage)

#### **Registration Logic Tests**

**File**: `pkg/notification/delivery/orchestrator_registration_test.go` (created in Phase 1)

**Coverage**:
- ✅ `RegisterChannel()` - success case
- ✅ `RegisterChannel()` - nil service (should skip)
- ✅ `RegisterChannel()` - overwrite existing channel
- ✅ `UnregisterChannel()` - remove channel
- ✅ `UnregisterChannel()` - non-existent channel (safe)
- ✅ `HasChannel()` - registered channel (true)
- ✅ `HasChannel()` - unregistered channel (false)
- ✅ `DeliverToChannel()` - registered channel (success)
- ✅ `DeliverToChannel()` - unregistered channel (error)
- ✅ `DeliverToChannel()` - delegates to service
- ✅ `DeliverToChannel()` - propagates errors

**Run**:
```bash
cd pkg/notification/delivery
go test -v -run "Orchestrator Channel Registration" -coverprofile=coverage.out
go tool cover -html=coverage.out
# Target: >90% coverage for registration logic
```

---

### **Test Level 2: Integration Tests** (>50% coverage)

#### **Integration Test Cases**

**File**: `test/integration/notification/suite_test.go`

**Existing tests should pass unchanged**:
- ✅ Audit emission tests (BR-NOT-051, BR-NOT-052)
- ✅ Delivery flow tests (BR-NOT-053)
- ✅ Retry logic tests (BR-NOT-055)
- ✅ Sanitization tests (BR-NOT-054)

**New registration-specific tests** (optional, add if time permits):

```go
Describe("DD-NOT-007: Channel Registration in Integration Tests", func() {
	It("should allow registering only needed channels", func() {
		// Create orchestrator
		o := delivery.NewOrchestrator(sanitizer, metrics, status, logger)

		// Register only console
		o.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)

		// Verify console works
		notification := testutil.NewNotification("test")
		err := o.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelConsole)
		Expect(err).ToNot(HaveOccurred())

		// Verify unregistered channels fail gracefully
		err = o.DeliverToChannel(ctx, notification, notificationv1alpha1.ChannelSlack)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("channel not registered"))
	})
})
```

**Run**:
```bash
make test-integration-notification
# Expected: All existing tests pass + new registration tests pass
```

---

### **Test Level 3: E2E Tests** (10-15% coverage)

#### **E2E Test Validation**

**File**: `test/e2e/notification/*_test.go`

**Critical paths to verify**:
- ✅ Controller starts successfully with registration pattern
- ✅ File delivery works (E2E-specific channel)
- ✅ All 4 channels work end-to-end

**Changes needed** (if E2E tests directly instantiate orchestrator):

**Check if E2E tests use orchestrator directly**:
```bash
grep -r "NewOrchestrator" test/e2e/notification/
```

**If found, update to registration pattern** (same as integration tests)

**Run**:
```bash
make test-e2e-notification
# Expected: All E2E tests pass
```

---

### **Test Level 4: Manual Production Validation** (<10% coverage)

#### **Manual Test Checklist**

**Prerequisites**:
- Kind cluster running
- Notification controller deployed

**Test Cases**:

**1. Controller Startup**:
```bash
# Deploy controller with registration pattern
make deploy-notification

# Verify controller starts
kubectl get pods -n notification-system
# Expected: notification-controller-xxx Running

# Check logs for registration messages
kubectl logs -n notification-system deployment/notification-controller | grep "Registered delivery channel"
# Expected: 4 log lines (console, slack, file, log)
```

**2. Console Delivery** (simplest channel):
```bash
# Create notification requesting console delivery
cat <<EOF | kubectl apply -f -
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: test-console-registration
  namespace: default
spec:
  subject: "DD-NOT-007 Registration Test"
  body: "Testing console channel with registration pattern"
  channels:
    - console
  priority: medium
EOF

# Verify delivery
kubectl logs -n notification-system deployment/notification-controller | grep "test-console-registration"
# Expected: "Delivery successful" log entry

# Check status
kubectl get notificationrequest test-console-registration -o yaml
# Expected: status.successfulDeliveries: 1
```

**3. Multi-Channel Delivery**:
```bash
# Create notification requesting all channels
cat <<EOF | kubectl apply -f -
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: test-all-channels-registration
  namespace: default
spec:
  subject: "DD-NOT-007 Multi-Channel Test"
  body: "Testing all channels with registration pattern"
  channels:
    - console
    - slack
    - file
    - log
  priority: medium
EOF

# Verify all deliveries
kubectl get notificationrequest test-all-channels-registration -o yaml
# Expected: status.successfulDeliveries: 4
```

**4. Unregistered Channel Error** (negative test):
```bash
# Try to use hypothetical "email" channel (not registered)
# This would require modifying the CRD enum temporarily
# Skip this test if not worth the effort
```

---

### **Test Level 5: Performance Validation** (Optional)

#### **Benchmark Test** (if concerned about performance)

**File**: `pkg/notification/delivery/orchestrator_bench_test.go`

```go
package delivery_test

import (
	"context"
	"testing"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

func BenchmarkDeliverToChannel_MapLookup(b *testing.B) {
	// Setup
	orchestrator := delivery.NewOrchestrator(nil, nil, nil, logger)
	mockService := &mockDeliveryService{}
	orchestrator.RegisterChannel("console", mockService)
	notification := testutil.NewNotification("bench")
	ctx := context.Background()

	// Benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = orchestrator.DeliverToChannel(ctx, notification, "console")
	}
}
```

**Run**:
```bash
cd pkg/notification/delivery
go test -bench=BenchmarkDeliverToChannel -benchmem
# Expected: <100ns/op (map lookup is extremely fast)
```

---

## ✅ **Validation Checklist**

### **Code Quality Gates**

- [ ] All unit tests pass (orchestrator_registration_test.go)
- [ ] All integration tests pass (suite_test.go)
- [ ] All E2E tests pass (notification_e2e_test.go)
- [ ] No compilation errors
- [ ] No lint errors (`golangci-lint run`)
- [ ] Code coverage >70% for registration logic

### **DD-NOT-007 Compliance**

- [ ] No channel parameters in `NewOrchestrator()` signature
- [ ] No switch statement in `DeliverToChannel()`
- [ ] No channel-specific fields in `Orchestrator` struct
- [ ] No `deliverToConsole()`, `deliverToSlack()`, etc. methods
- [ ] `RegisterChannel()`, `UnregisterChannel()`, `HasChannel()` methods exist
- [ ] Map-based routing implemented
- [ ] Clear error for unregistered channels

### **Production Readiness**

- [ ] Controller starts successfully
- [ ] All 4 channels deliver successfully
- [ ] Logs show registration messages
- [ ] No performance regression (benchmark if needed)
- [ ] Documentation updated

---

## 🎯 **Success Metrics**

### **Before Refactoring** ❌

```bash
# Constructor complexity
NewOrchestrator parameters: 8 (4 channels + 4 dependencies)

# Adding new channel
- Modify constructor signature (breaking change)
- Add field to struct
- Add case to switch statement
- Update all 3 usage sites
- Pass nil in tests for unused channels
Effort: ~4 hours
```

### **After Refactoring** ✅

```bash
# Constructor complexity
NewOrchestrator parameters: 4 (dependencies only)

# Adding new channel
- Implement DeliveryService interface
- Register channel in production
- Register channel in tests (optional)
Effort: ~2 hours (50% improvement)
```

### **Test Results Target**

| Metric | Target | Validation |
|--------|--------|------------|
| **Unit test coverage** | >90% | `go test -cover` |
| **Integration tests** | All pass | `make test-integration-notification` |
| **E2E tests** | All pass | `make test-e2e-notification` |
| **Build success** | Clean | `make build-notification` |
| **Performance** | No regression | Benchmark (optional) |

---

## 📋 **Rollback Plan**

### **If Something Goes Wrong**

**Scenario 1**: Tests fail unexpectedly
```bash
# Revert orchestrator.go changes
git checkout pkg/notification/delivery/orchestrator.go

# Revert usage changes
git checkout cmd/notification/main.go test/integration/notification/suite_test.go

# Verify original tests pass
make test
```

**Scenario 2**: Production deployment fails
```bash
# Rollback to previous deployment
kubectl rollout undo deployment/notification-controller -n notification-system

# Verify controller is running
kubectl get pods -n notification-system
```

**Scenario 3**: Performance regression detected
```bash
# Profile the application
kubectl port-forward -n notification-system deployment/notification-controller 6060:6060
go tool pprof http://localhost:6060/debug/pprof/profile

# If confirmed regression, revert changes
git revert <commit-sha>
```

---

## 🚀 **Post-Implementation Tasks**

### **Documentation Updates**

- [ ] Update orchestrator.go godoc comments
- [ ] Update DD-NOT-007 with "IMPLEMENTED" status
- [ ] Add migration notes to CHANGELOG
- [ ] Update service README if needed

### **Knowledge Sharing**

- [ ] Share DD-NOT-007 with team
- [ ] Add registration pattern example to team docs
- [ ] Update code review checklist

### **Future Enhancements** (Not in scope)

- [ ] Config-driven channel registration (ADR-030 integration)
- [ ] Channel health checks
- [ ] Channel priorities
- [ ] Channel middleware

---

## 📚 **References**

- **[DD-NOT-007](mdc:docs/architecture/decisions/DD-NOT-007-DELIVERY-ORCHESTRATOR-REGISTRATION-PATTERN.md)** - Authoritative standard
- **[00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc)** - APDC + TDD methodology
- **[03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)** - Testing coverage standards
- **[DeliveryService Interface](mdc:pkg/notification/delivery/interface.go)** - Common interface

---

**Document Status**: ✅ **READY TO IMPLEMENT**
**Created**: December 22, 2025
**Methodology**: APDC-Enhanced TDD
**Estimated Effort**: 2-3 hours
**Risk Level**: 🟢 **LOW** (well-defined, clear rollback)
**Prepared by**: AI Assistant (NT Team)
**Approved by**: User (jgil)

**Next Action**: Start Phase 1 (TDD RED) - Write failing registration tests! 🚀



