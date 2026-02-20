# Controller Refactoring Pattern Library

**Version**: 1.1.0
**Status**: 🎯 **AUTHORITATIVE REFERENCE**
**Last Updated**: December 21, 2025
**Reference Implementations**:
- RemediationOrchestrator (RO) Service - 86% pattern adoption (6/7 patterns)
- Notification (NT) Service - 57% pattern adoption (4/7 patterns, validated Dec 2025)

---

## 📚 Purpose

This document provides **production-proven refactoring patterns** extracted from the RemediationOrchestrator (RO) service, which achieved 86% (6/7) pattern adoption and serves as kubernaut's architectural gold standard.

**Target Audience**:
- Service teams refactoring monolithic controllers (NT, SP, WE)
- Developers implementing new CRD controllers
- Technical leads establishing architectural standards

**Source Analysis**: See [NT Refactoring Case Study](../case-studies/NT_REFACTORING_2025.md) for production-validated lessons learned.

---

## 🎯 When to Apply These Patterns

### Refactoring Triggers
Your controller needs refactoring if it exhibits **any 3** of these symptoms:

1. ❌ **Size**: Main controller file > 800 lines
2. ❌ **Duplication**: Same logic appears in 3+ locations
3. ❌ **Complexity**: Adding new features requires changes across entire file
4. ❌ **Testing**: Tests require complex setup with many mocks
5. ❌ **Phase Logic**: Phase transitions scattered across multiple methods
6. ❌ **Status Updates**: Inconsistent retry patterns or error handling
7. ❌ **Delivery/Execution**: Orchestration logic mixed with business logic

**Severity Guide**:
- 3-4 symptoms: **Refactoring recommended** (Plan for next quarter)
- 5-6 symptoms: **Refactoring strongly recommended** (Plan for next sprint)
- 7 symptoms: **Refactoring mandatory** (Address immediately)

---

## 📦 Pattern Catalog

### Pattern Overview

| # | Pattern | Priority | Effort | ROI | Reference |
|---|---------|----------|--------|-----|-----------|
| 1 | Phase State Machine | P0 | 2-3 days | High | [§1](#1-phase-state-machine-pattern-p0) |
| 2 | Terminal State Logic | P1 | 4-6 hours | Very High | [§2](#2-terminal-state-logic-pattern-p1) |
| 3 | Creator/Orchestrator | P0 | 2-3 days | High | [§3](#3-creatororchestrator-pattern-p0) |
| 4 | Status Manager | P1 | 4-6 hours | Very High | [§4](#4-status-manager-pattern-p1) |
| 5 | Controller Decomposition | P2 | 1-2 weeks | Medium | [§5](#5-controller-decomposition-pattern-p2) |
| 6 | Interface-Based Services | P2 | 1-2 days | Medium | [§6](#6-interface-based-services-pattern-p2) |
| 7 | Audit Manager | P3 | 1-2 days | Low | [§7](#7-audit-manager-pattern-p3) |

**Priority Guide**:
- **P0**: Critical for maintainability (do first)
- **P1**: Quick wins with high ROI (do early)
- **P2**: Significant improvements (do after P0/P1)
- **P3**: Polish and consistency (do when time allows)

---

## 1. Phase State Machine Pattern (P0)

### 📋 Problem Statement

**Symptoms**:
- Phase constants scattered across multiple files
- No validation of phase transitions
- Duplicate phase checking logic
- Terminal state checks repeated everywhere
- Phase transitions happen without validation

**Example Bad Code**:
```go
// ❌ BAD: Direct phase assignment without validation
notification.Status.Phase = notificationv1alpha1.NotificationPhaseFailed

// ❌ BAD: No validation of transition validity
if notification.Status.Phase == "Pending" {
    notification.Status.Phase = "Sending"  // What if already Failed?
}
```

---

### 🎯 Solution: Phase Package with State Machine

**Reference**: `pkg/remediationorchestrator/phase/`

#### Step 1: Create Phase Package Structure

```bash
# Create directory
mkdir -p pkg/[service]/phase

# Create files
touch pkg/[service]/phase/types.go
touch pkg/[service]/phase/manager.go
```

#### Step 2: Define Phase Types (`types.go`)

```go
// pkg/[service]/phase/types.go

package phase

import (
    "fmt"
    [servicev1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// Phase is an alias for the API-exported phase type
type Phase = [servicev1].[Service]Phase

// Re-export phase constants for internal convenience
const (
    Pending   = [servicev1].PhasePending
    Processing = [servicev1].PhaseProcessing
    Completed = [servicev1].PhaseCompleted
    Failed    = [servicev1].PhaseFailed
    // ... add your service-specific phases
)

// IsTerminal returns true if the phase is a terminal state.
func IsTerminal(p Phase) bool {
    switch p {
    case Completed, Failed:  // Add your terminal phases
        return true
    default:
        return false
    }
}

// ValidTransitions defines the state machine.
// Key: current phase, Value: list of valid target phases
var ValidTransitions = map[Phase][]Phase{
    Pending:    {Processing, Failed},
    Processing: {Completed, Failed},
    // Terminal states - no transitions allowed
    Completed: {},
    Failed:    {},
}

// CanTransition checks if transition from current to target is valid.
func CanTransition(current, target Phase) bool {
    validTargets, ok := ValidTransitions[current]
    if !ok {
        return false
    }
    for _, v := range validTargets {
        if v == target {
            return true
        }
    }
    return false
}

// Validate checks if a phase value is valid.
func Validate(p Phase) error {
    switch p {
    case Pending, Processing, Completed, Failed:  // Add all valid phases
        return nil
    default:
        return fmt.Errorf("invalid phase: %s", p)
    }
}

// GetTerminalPhases returns all terminal phases.
func GetTerminalPhases() []Phase {
    return []Phase{Completed, Failed}  // Add your terminal phases
}
```

#### Step 3: Create Phase Manager (`manager.go`)

```go
// pkg/[service]/phase/manager.go

package phase

import (
    "fmt"
    [servicev1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// Manager implements phase state machine logic.
type Manager struct{}

// NewManager creates a new phase manager.
func NewManager() *Manager {
    return &Manager{}
}

// CurrentPhase returns the current phase.
// Returns Pending if phase is empty (initial state).
func (m *Manager) CurrentPhase(obj *[servicev1].[ServiceCRD]) Phase {
    if obj.Status.Phase == "" {
        return Pending
    }
    return Phase(obj.Status.Phase)
}

// TransitionTo transitions to the target phase.
// Returns an error if the transition is invalid per the state machine.
func (m *Manager) TransitionTo(obj *[servicev1].[ServiceCRD], target Phase) error {
    current := m.CurrentPhase(obj)

    if !CanTransition(current, target) {
        return fmt.Errorf("invalid phase transition from %s to %s", current, target)
    }

    obj.Status.Phase = target
    return nil
}
```

---

### 📝 Migration Steps

#### Before Migration: Typical Controller Code
```go
// ❌ BEFORE: Phase logic scattered in controller

func (r *Reconciler) Reconcile(...) {
    // No validation
    if notification.Status.Phase == "" {
        notification.Status.Phase = notificationv1alpha1.NotificationPhasePending
    }

    // Terminal check duplicated
    if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent ||
       notification.Status.Phase == notificationv1alpha1.NotificationPhasePartiallySent ||
       notification.Status.Phase == notificationv1alpha1.NotificationPhaseFailed {
        return ctrl.Result{}, nil
    }

    // Direct phase assignment (no validation)
    notification.Status.Phase = notificationv1alpha1.NotificationPhaseSending
}
```

#### After Migration: Clean Controller Code
```go
// ✅ AFTER: Phase logic centralized in phase package

import (
    "[service]phase" "github.com/jordigilh/kubernaut/pkg/[service]/phase"
)

type Reconciler struct {
    // ... other fields
    phaseManager *[service]phase.Manager
}

func (r *Reconciler) Reconcile(...) {
    // Get current phase
    currentPhase := r.phaseManager.CurrentPhase(notification)

    // Single terminal check
    if [service]phase.IsTerminal(currentPhase) {
        return ctrl.Result{}, nil
    }

    // Validated phase transition
    if err := r.phaseManager.TransitionTo(notification, [service]phase.Sending); err != nil {
        logger.Error(err, "Invalid phase transition")
        return ctrl.Result{}, err
    }
}
```

---

### ✅ Benefits

- ✅ **Single Source of Truth**: Phase logic in one place
- ✅ **Runtime Validation**: Invalid transitions caught immediately
- ✅ **Testability**: Test state machine independently
- ✅ **Documentation**: State machine is self-documenting
- ✅ **Reusability**: Use in CLI tools, webhooks, tests

---

### 🧪 Testing Pattern

```go
// test/unit/[service]/phase/state_machine_test.go

var _ = Describe("Phase State Machine", func() {
    Context("Terminal States", func() {
        It("should identify terminal phases correctly", func() {
            Expect([service]phase.IsTerminal([service]phase.Completed)).To(BeTrue())
            Expect([service]phase.IsTerminal([service]phase.Failed)).To(BeTrue())
            Expect([service]phase.IsTerminal([service]phase.Pending)).To(BeFalse())
        })
    })

    Context("Phase Transitions", func() {
        It("should allow valid transitions", func() {
            Expect([service]phase.CanTransition(
                [service]phase.Pending,
                [service]phase.Processing,
            )).To(BeTrue())
        })

        It("should reject invalid transitions", func() {
            Expect([service]phase.CanTransition(
                [service]phase.Completed,
                [service]phase.Processing,
            )).To(BeFalse())
        })
    })
})
```

---

## 2. Terminal State Logic Pattern (P1)

### 📋 Problem Statement

**Symptoms**:
- Terminal state checks duplicated 3+ times
- Inconsistent terminal state definitions
- Risk of missing terminal phases in checks
- Each duplication is 10-20 lines of code

**Example Bad Code**:
```go
// ❌ BAD: Duplicated in 4 locations in NT controller

// Location 1: handleTerminalStateCheck()
if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent {
    return true
}
if notification.Status.Phase == notificationv1alpha1.NotificationPhasePartiallySent {
    return true
}
if notification.Status.Phase == notificationv1alpha1.NotificationPhaseFailed {
    return true
}

// Location 2: Reconcile() line 145
if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent ||
    notification.Status.Phase == notificationv1alpha1.NotificationPhasePartiallySent {
    return ctrl.Result{}, nil
}

// Location 3: Reconcile() line 160 (duplicate of location 2!)
// ... same code again

// Location 4: determinePhaseTransition()
// ... same logic in different form
```

---

### 🎯 Solution: Single IsTerminal() Function

**Reference**: `pkg/remediationorchestrator/phase/types.go` lines 74-82

#### Implementation (Part of Phase Package)

```go
// pkg/[service]/phase/types.go

// IsTerminal returns true if the phase is a terminal state.
func IsTerminal(p Phase) bool {
    switch p {
    case Completed, Failed, TimedOut, Skipped:  // RR: TimedOut; NR: PartiallySent
        return true
    default:
        return false
    }
}

// GetTerminalPhases returns all terminal phases for documentation/testing.
func GetTerminalPhases() []Phase {
    return []Phase{Completed, Failed, TimedOut, Skipped}
}
```

---

### 📝 Migration Steps (Quick Win!)

**Estimated Effort**: 4-6 hours
**Lines Saved**: 50-60 lines
**Risk**: Very Low (pure refactoring)

#### Step 1: Identify All Terminal State Checks

```bash
# Search for terminal state checks
grep -n "Phase.*Sent\|Phase.*Failed\|Phase.*Completed" \
    internal/controller/[service]/*.go

# Look for patterns like:
# - if status.Phase == PhaseFailed
# - switch status.Phase { case Completed: ...}
# - phase == "Sent" || phase == "Failed"
```

#### Step 2: Replace with IsTerminal()

```go
// ❌ BEFORE: Location 1
if notification.Status.Phase == notificationv1alpha1.NotificationPhaseSent ||
   notification.Status.Phase == notificationv1alpha1.NotificationPhasePartiallySent ||
   notification.Status.Phase == notificationv1alpha1.NotificationPhaseFailed {
    return ctrl.Result{}, nil
}

// ✅ AFTER: Single line
if [service]phase.IsTerminal([service]phase.Phase(notification.Status.Phase)) {
    return ctrl.Result{}, nil
}

// Or with phase manager:
if [service]phase.IsTerminal(r.phaseManager.CurrentPhase(notification)) {
    return ctrl.Result{}, nil
}
```

#### Step 3: Run Tests

```bash
# Verify no behavior change
make test-unit-[service]
make test-integration-[service]
```

---

### ✅ Benefits

- ✅ **Quick Win**: 4-6 hours for 50+ line reduction
- ✅ **Zero Risk**: Pure refactoring, no behavior change
- ✅ **Prevents Bugs**: Adding terminal phase only requires one change
- ✅ **Self-Documenting**: `IsTerminal()` is clearer than scattered checks

---

## 3. Creator/Orchestrator Pattern (P0)

### 📋 Problem Statement

**Symptoms**:
- Controller has 100+ line methods for creating/delivering/executing
- Creation logic mixed with orchestration logic
- Difficult to test delivery/execution independently
- Adding new channels/workflows requires controller changes

**Example Bad Code** (NT Service):
```go
// ❌ BAD: 79-line handleDeliveryLoop() method in controller
// ❌ BAD: 124-line recordDeliveryAttempt() method in controller
// ❌ BAD: Channel-specific logic in controller (deliverToSlack, deliverToConsole)
```

---

### 🎯 Solution: Extract to Creator/Orchestrator Package

**Reference**: `pkg/remediationorchestrator/creator/` (5 files, 1200+ lines extracted)

#### Pattern A: Creator Pattern (for CRD creation)

**Use When**: Service creates child CRDs (like RO creating SignalProcessing, AIAnalysis)

```go
// pkg/[service]/creator/[childcrd].go

package creator

import (
    "context"
    [childv1] "github.com/jordigilh/kubernaut/api/[child]/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Create[ChildCRD] creates a [ChildCRD] CRD for the given parent.
func Create[ChildCRD](
    ctx context.Context,
    client client.Client,
    parent *[parentv1].[ParentCRD],
    correlationID string,
) (*[childv1].[ChildCRD], error) {
    // 1. Construct child CRD
    child := &[childv1].[ChildCRD]{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-child", parent.Name),
            Namespace: parent.Namespace,
            Labels: map[string]string{
                "parent": parent.Name,
            },
        },
        Spec: [childv1].[ChildCRD]Spec{
            // Copy relevant fields from parent
        },
    }

    // 2. Set owner reference (for cascade deletion)
    if err := ctrl.SetControllerReference(parent, child, scheme); err != nil {
        return nil, fmt.Errorf("failed to set owner reference: %w", err)
    }

    // 3. Create in Kubernetes
    if err := client.Create(ctx, child); err != nil {
        return nil, fmt.Errorf("failed to create child CRD: %w", err)
    }

    return child, nil
}
```

**RO Example**: `pkg/remediationorchestrator/creator/signalprocessing.go`
```go
// RO creates SignalProcessing CRD from RemediationRequest
func CreateSignalProcessing(
    ctx context.Context,
    client client.Client,
    rr *remediationv1.RemediationRequest,
    correlationID string,
) (*signalprocessingv1.SignalProcessing, error) {
    // Construct CRD, set owner, create
    // Returns created CRD or error
}
```

---

#### Pattern B: Orchestrator Pattern (for delivery/execution)

**Use When**: Service orchestrates delivery, execution, or external operations (like NT delivering notifications)

```go
// pkg/[service]/delivery/orchestrator.go (or execution/orchestrator.go)

package delivery

import (
    "context"
    [servicev1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// Orchestrator manages delivery/execution orchestration.
type Orchestrator struct {
    services map[Channel]DeliveryService  // Or executors, handlers, etc.
    sanitizer *sanitization.Sanitizer
    circuitBreaker *retry.CircuitBreaker
    logger logr.Logger
}

// DeliveryResult represents the outcome of a delivery attempt.
type DeliveryResult struct {
    Channel Channel
    Success bool
    Error   error
    Attempt DeliveryAttempt
}

// DeliverToChannels orchestrates delivery to multiple channels.
func (o *Orchestrator) DeliverToChannels(
    ctx context.Context,
    notification *[servicev1].NotificationRequest,
    channels []Channel,
) ([]DeliveryResult, error) {
    results := make([]DeliveryResult, 0, len(channels))

    for _, channel := range channels {
        result, err := o.DeliverToChannel(ctx, notification, channel)
        if err != nil {
            return results, fmt.Errorf("orchestration failed: %w", err)
        }
        results = append(results, *result)
    }

    return results, nil
}

// DeliverToChannel handles delivery to a single channel.
func (o *Orchestrator) DeliverToChannel(
    ctx context.Context,
    notification *[servicev1].NotificationRequest,
    channel Channel,
) (*DeliveryResult, error) {
    // 1. Get service for channel
    service, ok := o.services[channel]
    if !ok {
        return nil, fmt.Errorf("no service registered for channel: %s", channel)
    }

    // 2. Sanitize before delivery
    sanitized := o.sanitizer.Sanitize(notification)

    // 3. Attempt delivery with circuit breaker
    err := o.circuitBreaker.Execute(func() error {
        return service.Deliver(ctx, sanitized)
    })

    // 4. Return result
    return &DeliveryResult{
        Channel: channel,
        Success: err == nil,
        Error:   err,
    }, nil
}
```

---

### 📝 Migration Steps

#### Step 1: Create Package Structure

```bash
# For CRD creators
mkdir -p pkg/[service]/creator

# For delivery/execution orchestration
mkdir -p pkg/[service]/delivery  # or execution/
```

#### Step 2: Extract Large Methods

**Identify Candidates**:
```bash
# Find methods > 50 lines in controller
grep -n "^func.*Reconciler.*{" internal/controller/[service]/*.go | \
    while read line; do
        # Manual inspection needed
        echo "$line"
    done
```

**Common Patterns to Extract**:
- `handleDeliveryLoop()` → `Orchestrator.DeliverToChannels()`
- `createChildCRD()` → `creator.CreateChildCRD()`
- `recordDeliveryAttempt()` → `Orchestrator.RecordAttempt()`

#### Step 3: Update Controller to Use Creator/Orchestrator

```go
// ❌ BEFORE: Controller has delivery logic
func (r *Reconciler) handleDeliveryLoop(...) {
    // 79 lines of delivery orchestration
}

// ✅ AFTER: Controller delegates to orchestrator
type Reconciler struct {
    // ... other fields
    deliveryOrchestrator *delivery.Orchestrator
}

func (r *Reconciler) handleDeliveryLoop(...) {
    results, err := r.deliveryOrchestrator.DeliverToChannels(
        ctx,
        notification,
        notification.Spec.Channels,
    )
    if err != nil {
        return r.handleDeliveryError(ctx, notification, err)
    }
    return r.handleDeliverySuccess(ctx, notification, results)
}
```

---

### ✅ Benefits

- ✅ **Separation of Concerns**: Controller orchestrates, creators/orchestrators execute
- ✅ **Testability**: Test creators/orchestrators independently
- ✅ **Reusability**: Use in webhooks, CLI tools, other controllers
- ✅ **Extensibility**: Add new channels/CRDs without controller changes
- ✅ **Line Reduction**: 200-400 lines out of controller

---

## 4. Status Manager Pattern (P1)

### ⚠️ **CRITICAL: Check for Existing Status Manager First!**

**Before creating a new status manager, search the codebase:**

```bash
# Search for existing status manager implementations
codebase_search "existing StatusManager implementations"
grep -r "type.*Manager.*struct" pkg/[service]/status/

# Check if already wired in main app
grep -r "StatusManager\|status.NewManager" cmd/[service]/
```

**NT Lesson Learned**: Status manager existed at `pkg/notification/status/manager.go` but wasn't wired into the controller. Saved **4 hours** by discovering and wiring existing code instead of creating duplicate implementation.

**If status manager exists**:
1. ✅ **Wire it into controller** (see Step 2 below)
2. ✅ **Remove controller's custom status update methods**
3. ✅ **Update all status.Update() calls to use manager**

**If status manager doesn't exist**:
1. Create new status manager (follow template below)
2. Wire into controller
3. Document why new manager was needed

---

### 📋 Problem Statement

**Symptoms**:
- Status update logic scattered across controller
- Inconsistent retry patterns
- `pkg/[service]/status/manager.go` exists but unused ⚠️ **CHECK FIRST!**
- Controller has its own `updateStatusWithRetry()` method

**Example Bad Code** (NT Service):
```go
// ❌ BAD: Controller has custom updateStatusWithRetry()
// ❌ BAD: pkg/notification/status/manager.go exists but unused
// ❌ BAD: Direct status.Update() calls scattered everywhere
```

---

### 🎯 Solution: Centralized Status Manager

**Reference**: `pkg/remediationorchestrator/phase/manager.go`

#### Implementation

```go
// pkg/[service]/status/manager.go

package status

import (
    "context"
    "fmt"
    "time"

    [servicev1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager manages status updates with retry logic.
type Manager struct {
    client client.Client
    scheme *runtime.Scheme
}

// NewManager creates a new status manager.
func NewManager(client client.Client, scheme *runtime.Scheme) *Manager {
    return &Manager{
        client: client,
        scheme: scheme,
    }
}

// UpdatePhaseWithRetry updates the phase with retry logic.
func (m *Manager) UpdatePhaseWithRetry(
    ctx context.Context,
    obj *[servicev1].[ServiceCRD],
    newPhase [servicev1].[Service]Phase,
    reason, message string,
    maxRetries int,
) error {
    return m.retryUpdate(ctx, obj, maxRetries, func() error {
        // Validate transition
        if !isValidPhaseTransition(obj.Status.Phase, newPhase) {
            return fmt.Errorf("invalid phase transition from %s to %s",
                obj.Status.Phase, newPhase)
        }

        // Update phase
        obj.Status.Phase = newPhase
        obj.Status.LastTransitionTime = metav1.Now()

        // Set condition
        meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
            Type:    "PhaseTransition",
            Status:  metav1.ConditionTrue,
            Reason:  reason,
            Message: message,
        })

        return nil
    })
}

// UpdateStatusWithRetry updates status with retry logic.
func (m *Manager) UpdateStatusWithRetry(
    ctx context.Context,
    obj *[servicev1].[ServiceCRD],
    maxRetries int,
) error {
    return m.retryUpdate(ctx, obj, maxRetries, func() error {
        return nil  // Changes already applied to obj
    })
}

// retryUpdate implements retry logic for status updates.
func (m *Manager) retryUpdate(
    ctx context.Context,
    obj *[servicev1].[ServiceCRD],
    maxRetries int,
    updateFunc func() error,
) error {
    for i := 0; i < maxRetries; i++ {
        // Apply update function
        if err := updateFunc(); err != nil {
            return err
        }

        // Attempt status update
        if err := m.client.Status().Update(ctx, obj); err != nil {
            if errors.IsConflict(err) {
                // Conflict: re-fetch and retry
                if refetchErr := m.client.Get(ctx,
                    client.ObjectKeyFromObject(obj), obj); refetchErr != nil {
                    return fmt.Errorf("refetch failed: %w", refetchErr)
                }
                time.Sleep(time.Millisecond * 100)  // Backoff
                continue
            }
            return fmt.Errorf("status update failed: %w", err)
        }

        // Success
        return nil
    }

    return fmt.Errorf("status update failed after %d retries", maxRetries)
}

// isValidPhaseTransition validates phase transitions.
func isValidPhaseTransition(current, new [servicev1].[Service]Phase) bool {
    // Implement your service's phase transition rules
    // Can delegate to phase.CanTransition() if using Phase State Machine pattern
    return true  // Placeholder
}
```

---

### 📝 Migration Steps (Quick Win!)

**Estimated Effort**: 4-6 hours
**Lines Saved**: 100+ lines
**Risk**: Low

#### Step 1: Check if Status Manager Already Exists

```bash
# Many services already have a status manager!
ls pkg/[service]/status/manager.go
```

If it exists: **Adopt it!** (NT has this problem - manager exists but unused)

#### Step 2: Replace Controller's Status Update Logic

```go
// ❌ BEFORE: Controller has custom updateStatusWithRetry()
type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *Reconciler) updateStatusWithRetry(...) error {
    // 30+ lines of retry logic
}

// ✅ AFTER: Controller uses status manager
import (
    [service]status "github.com/jordigilh/kubernaut/pkg/[service]/status"
)

type Reconciler struct {
    client.Client
    Scheme        *runtime.Scheme
    statusManager *[service]status.Manager  // ADD THIS
}

// In SetupWithManager or constructor:
func NewReconciler(...) *Reconciler {
    return &Reconciler{
        // ... other fields
        statusManager: [service]status.NewManager(client, scheme),
    }
}

// Replace all updateStatusWithRetry() calls:
func (r *Reconciler) someMethod(...) error {
    // BEFORE: r.updateStatusWithRetry(ctx, obj, 3)
    // AFTER:
    if err := r.statusManager.UpdateStatusWithRetry(ctx, obj, 3); err != nil {
        return err
    }
}
```

#### Step 3: Remove Controller's Status Update Methods

```go
// DELETE from controller:
func (r *Reconciler) updateStatusWithRetry(...) { ... }
func (r *Reconciler) retryStatusUpdate(...) { ... }
```

---

### ✅ Benefits

- ✅ **Quick Win**: 4-6 hours for 100+ line reduction
- ✅ **Consistency**: All status updates use same retry pattern
- ✅ **Testability**: Test status manager independently
- ✅ **Reduced Duplication**: Single source of truth for status updates

---

## 5. Controller Decomposition Pattern (P2)

### 📋 Problem Statement

**Symptoms**:
- Main controller file > 1000 lines
- Mixed concerns (reconciliation + specialized logic)
- Difficult to navigate
- High cognitive load for new developers

**Example**:
- NT: 1558 lines in single file
- SP: 1287 lines in single file
- WE: 1118 lines in single file

---

### 🎯 Solution: Multi-File Controller Structure

**Reference**: `pkg/remediationorchestrator/controller/` (5 files, 2751 lines)

#### RO's Structure (Gold Standard)

```
pkg/remediationorchestrator/controller/
├── reconciler.go (1754 lines)           # Main reconciliation loop
├── blocking.go (291 lines)              # Blocked phase handling (BR-ORCH-042)
├── consecutive_failure.go (257 lines)   # Consecutive failure tracking
├── notification_handler.go (290 lines)  # Notification creation
└── notification_tracking.go (159 lines) # Duplicate notification management
```

**Pattern**: Main reconciler + specialized handler files

---

### 📝 Recommended Decomposition Strategy

#### Option A: By Concern (Recommended for NT, SP, WE)

```
internal/controller/[service]/
├── [service]_controller.go       # Main reconciliation loop
├── phase_handlers.go              # Phase transition handlers
├── delivery_handlers.go           # Delivery/execution logic
├── audit_handlers.go              # Audit event emission
└── error_handlers.go              # Error handling and retry logic
```

**When to Use**: Service has distinct functional areas (delivery, execution, audit)

#### Option B: By Phase (Alternative for complex state machines)

```
internal/controller/[service]/
├── [service]_controller.go       # Main reconciliation loop
├── phase_pending.go              # handlePendingPhase()
├── phase_processing.go           # handleProcessingPhase()
├── phase_executing.go            # handleExecutingPhase()
└── phase_completed.go            # handleCompletedPhase()
```

**When to Use**: Service has 5+ phases with complex phase-specific logic

#### Option C: By Functional Domain (NT Lesson Learned)

```
internal/controller/[service]/
├── [service]_controller.go       # Main reconciliation loop + core phase logic
├── routing_handler.go             # Routing/channel resolution logic
├── retry_circuit_breaker_handler.go  # Retry and circuit breaker logic
└── audit_handlers.go              # Audit event emission (if needed)
```

**When to Use**:
- Controller has distinct functional domains (routing, retry, circuit breaking)
- Phase extraction would create excessive fragmentation
- Functional concerns are orthogonal to phases

**NT Success Metrics**:
- Main controller: 1133 lines (down from 1472, -23%)
- Routing handler: 196 lines (7 methods)
- Retry/CB handler: 187 lines (8 methods)
- Clean separation of concerns without over-fragmentation

**Decision Rule**:
- **Choose Functional Domain** when you can identify 2-4 clear functional areas that span multiple phases
- **Choose Phase-Based** when phase logic is highly distinct and doesn't share concerns
- **Choose Concern-Based** (Option A) when neither applies clearly

---

### 📝 Migration Steps

**Estimated Effort**: 1-2 weeks
**Lines Saved**: 400-600 lines from main controller
**Risk**: Medium (requires careful refactoring)

#### Step 1: Identify Extraction Candidates

**Look for**:
- Methods > 50 lines
- Logically grouped methods (all audit methods, all delivery methods)
- Methods only called from 1-2 locations

**Example Analysis** (NT Service):
```
Phase Transitions: 6 methods, ~400 lines → phase_handlers.go
Delivery Logic:    8 methods, ~300 lines → delivery_handlers.go
Audit Events:      8 methods, ~300 lines → audit_handlers.go
Status Updates:    4 methods, ~150 lines → (use Status Manager instead)
```

#### Step 2: Create Handler Files

```bash
cd internal/controller/[service]
touch phase_handlers.go
touch delivery_handlers.go
touch audit_handlers.go
```

#### Step 3: Move Methods (Keep Reconciler Receiver)

```go
// internal/controller/[service]/delivery_handlers.go

package [service]

// IMPORTANT: Keep the same receiver type!
// This allows methods to access r.Client, r.Scheme, etc.

// handleDeliveryLoop orchestrates delivery to configured channels.
func (r *[Service]RequestReconciler) handleDeliveryLoop(
    ctx context.Context,
    obj *[servicev1].[ServiceCRD],
) (ctrl.Result, error) {
    // Original method body moved here
}

// deliverToChannel attempts delivery to a single channel.
func (r *[Service]RequestReconciler) deliverToChannel(
    ctx context.Context,
    obj *[servicev1].[ServiceCRD],
    channel Channel,
) error {
    // Original method body moved here
}
```

#### Step 4: Keep Main Controller Clean

```go
// internal/controller/[service]/[service]_controller.go

// Main controller should only have:
// 1. Reconciler struct definition
// 2. Reconcile() method (main loop)
// 3. SetupWithManager()
// 4. High-level orchestration logic

func (r *[Service]RequestReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    // High-level flow only
    logger := log.FromContext(ctx)

    // 1. Fetch object
    obj := &[servicev1].[ServiceCRD]{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Check terminal state
    if [service]phase.IsTerminal(r.phaseManager.CurrentPhase(obj)) {
        return ctrl.Result{}, nil
    }

    // 3. Delegate to phase handler
    switch r.phaseManager.CurrentPhase(obj) {
    case [service]phase.Pending:
        return r.handlePendingPhase(ctx, obj)  // In phase_handlers.go
    case [service]phase.Processing:
        return r.handleProcessingPhase(ctx, obj)  // In phase_handlers.go
    // ... more phases
    }

    return ctrl.Result{}, nil
}
```

---

### ✅ Benefits

- ✅ **Easier Navigation**: Jump to specific handler file
- ✅ **Reduced Cognitive Load**: Understand one concern at a time
- ✅ **Better Git Diffs**: Changes to audit don't conflict with delivery changes
- ✅ **Team Parallelization**: Multiple developers can work on different files
- ✅ **Maintainability**: Main controller stays < 300 lines

---

## 6. Interface-Based Services Pattern (P2)

### 📋 Problem Statement

**Symptoms**:
- Controller tightly coupled to concrete service types
- Cannot use polymorphism for delivery/execution
- Adding new channels/executors requires controller changes
- Interface exists but only 1 implementation uses it

**Example Bad Code** (NT Service):
```go
// ❌ BAD: Concrete types in reconciler
type NotificationRequestReconciler struct {
    ConsoleService *delivery.ConsoleDeliveryService  // Concrete
    SlackService   *delivery.SlackDeliveryService    // Concrete
    FileService    *delivery.FileDeliveryService     // Concrete
}

// ❌ BAD: Type-specific logic in controller
func (r *Reconciler) deliverToChannel(..., channel Channel) error {
    switch channel {
    case "console":
        return r.ConsoleService.Deliver(ctx, notification)
    case "slack":
        return r.SlackService.Deliver(ctx, notification)
    // Adding "email" requires controller change!
    }
}
```

---

### 🎯 Solution: Interface-Based Service Registry

#### Step 1: Define Common Interface

```go
// pkg/[service]/delivery/interface.go (or execution/interface.go)

package delivery

type DeliveryService interface {
    Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}

// Or for execution services:
type ExecutionService interface {
    Execute(ctx context.Context, workflow *workflowv1alpha1.WorkflowExecution) error
}
```

#### Step 2: Implement Interface for All Services

```go
// pkg/notification/delivery/slack.go

type SlackDeliveryService struct {
    webhookURL string
    httpClient *http.Client
}

// Implement DeliveryService interface
func (s *SlackDeliveryService) Deliver(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) error {
    // Implementation
}

// pkg/notification/delivery/console.go

type ConsoleDeliveryService struct {
    writer io.Writer
}

// Implement DeliveryService interface
func (c *ConsoleDeliveryService) Deliver(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) error {
    // Implementation
}
```

#### Step 3: Use Map in Controller

```go
// internal/controller/notification/notificationrequest_controller.go

type NotificationRequestReconciler struct {
    client.Client
    Scheme *runtime.Scheme

    // ✅ GOOD: Interface-based registry
    DeliveryServices map[notificationv1alpha1.Channel]delivery.DeliveryService

    // ... other fields
}

// Polymorphic delivery
func (r *Reconciler) deliverToChannel(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
    channel notificationv1alpha1.Channel,
) error {
    service, ok := r.DeliveryServices[channel]
    if !ok {
        return fmt.Errorf("no delivery service registered for channel: %s", channel)
    }

    // Polymorphic call - works for any channel!
    return service.Deliver(ctx, notification)
}
```

#### Step 4: Register Services in main.go

```go
// cmd/notification/main.go

func main() {
    // ... setup code

    // Create services
    consoleService := delivery.NewConsoleDeliveryService(os.Stdout)
    slackService := delivery.NewSlackDeliveryService(slackWebhookURL)
    emailService := delivery.NewEmailDeliveryService(smtpConfig)  // NEW!

    // Register in map
    deliveryServices := map[notificationv1alpha1.Channel]delivery.DeliveryService{
        notificationv1alpha1.ChannelConsole: consoleService,
        notificationv1alpha1.ChannelSlack:   slackService,
        notificationv1alpha1.ChannelEmail:   emailService,  // No controller change!
    }

    // Create controller
    if err = (&controller.NotificationRequestReconciler{
        Client:           mgr.GetClient(),
        Scheme:           mgr.GetScheme(),
        DeliveryServices: deliveryServices,  // Inject map
        // ... other fields
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller")
        os.Exit(1)
    }
}
```

---

### ✅ Benefits

- ✅ **Polymorphism**: Single code path for all channels
- ✅ **Extensibility**: Add new channels without controller changes
- ✅ **Testability**: Mock entire delivery layer with single mock
- ✅ **Type Safety**: Compile-time checking of interface compliance

---

## 7. Audit Manager Pattern (P3)

### 📋 Problem Statement

**Symptoms**:
- Audit logic (200+ lines) in controller directory
- Idempotency tracking mixed with controller state
- Difficult to test audit logic independently

**Example**: NT has 289-line `audit.go` in `internal/controller/notification/`

---

### 🎯 Solution: Extract to pkg/[service]/audit/

**Reference**: `pkg/remediationorchestrator/audit/helpers.go`

#### Implementation

```go
// pkg/[service]/audit/manager.go

package audit

import (
    "context"
    "sync"

    "github.com/jordigilh/kubernaut/pkg/audit"
    [servicev1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// EventManager manages audit event emission with idempotency.
type EventManager struct {
    store         audit.AuditStore
    helpers       *AuditHelpers
    emittedEvents sync.Map  // Idempotency tracking
    logger        logr.Logger
}

// NewEventManager creates a new audit event manager.
func NewEventManager(
    store audit.AuditStore,
    helpers *AuditHelpers,
    logger logr.Logger,
) *EventManager {
    return &EventManager{
        store:   store,
        helpers: helpers,
        logger:  logger,
    }
}

// EmitMessageSent emits a message sent audit event.
func (em *EventManager) EmitMessageSent(
    ctx context.Context,
    notification *[servicev1].NotificationRequest,
    channel string,
) error {
    // Check idempotency
    key := string(notification.UID)
    eventType := "notification.message.sent"
    if !em.shouldEmit(key, eventType) {
        return nil  // Already emitted
    }

    // Create event
    event := em.helpers.CreateMessageSentEvent(notification, channel)

    // Store event
    if err := em.store.Store(ctx, event); err != nil {
        return fmt.Errorf("failed to store audit event: %w", err)
    }

    // Mark as emitted
    em.markEmitted(key, eventType)

    return nil
}

// shouldEmit checks if event should be emitted (idempotency).
func (em *EventManager) shouldEmit(key, eventType string) bool {
    emittedMap, ok := em.emittedEvents.Load(key)
    if !ok {
        return true
    }

    events := emittedMap.(map[string]bool)
    return !events[eventType]
}

// markEmitted marks event as emitted.
func (em *EventManager) markEmitted(key, eventType string) {
    emittedMap, _ := em.emittedEvents.LoadOrStore(key, make(map[string]bool))
    events := emittedMap.(map[string]bool)
    events[eventType] = true
}

// Cleanup removes idempotency tracking for deleted objects.
func (em *EventManager) Cleanup(key string) {
    em.emittedEvents.Delete(key)
}
```

---

### 📝 Migration Steps

**Estimated Effort**: 1-2 days
**Lines Saved**: 200-300 lines from controller
**Risk**: Low

#### Step 1: Create Audit Package

```bash
mkdir -p pkg/[service]/audit
touch pkg/[service]/audit/manager.go
touch pkg/[service]/audit/helpers.go
```

#### Step 2: Move Audit Methods

```go
// Move from internal/controller/[service]/audit.go
// To pkg/[service]/audit/manager.go

// Move idempotency tracking from controller to manager
```

#### Step 3: Update Controller

```go
// ❌ BEFORE: Controller has audit methods and idempotency map
type Reconciler struct {
    // ... fields
    AuditStore         audit.AuditStore
    emittedAuditEvents sync.Map  // Idempotency tracking
}

func (r *Reconciler) auditMessageSent(...) error {
    // 50+ lines of audit logic
}

// ✅ AFTER: Controller delegates to audit manager
import (
    [service]audit "github.com/jordigilh/kubernaut/pkg/[service]/audit"
)

type Reconciler struct {
    // ... fields
    auditManager *[service]audit.EventManager  // Single field
}

func (r *Reconciler) handleDeliverySuccess(...) error {
    // Delegate to audit manager
    if err := r.auditManager.EmitMessageSent(ctx, notification, channel); err != nil {
        logger.Error(err, "Failed to emit audit event")
        // Continue (fire-and-forget per DD-AUDIT-003)
    }
}
```

---

### ✅ Benefits

- ✅ **Separation**: Audit logic in pkg/, not controller
- ✅ **Testability**: Test audit manager independently
- ✅ **Reusability**: Use in webhooks, CLI tools
- ✅ **Line Reduction**: 200-300 lines out of controller

---

## 🧪 Testing Strategy During Refactoring

### Phase 1: Extract with Tests First (TDD)

**Before extracting code**:

1. **Ensure existing tests pass** (establish baseline):
   ```bash
   make test-unit-[service]
   make test-integration-[service]
   ```

2. **Identify code to extract**:
   ```bash
   # Example: Identify large methods
   grep -n "^func.*Reconciler.*{" internal/controller/[service]/*.go
   ```

3. **Write tests for extracted component** (RED phase):
   ```go
   // test/unit/[service]/phase/state_machine_test.go

   var _ = Describe("Phase State Machine", func() {
       It("should validate phase transitions", func() {
           Expect([service]phase.CanTransition(
               [service]phase.Pending,
               [service]phase.Processing,
           )).To(BeTrue())
       })
   })
   ```

   **Run tests** (should fail - no implementation yet):
   ```bash
   make test-unit-[service]
   ```

4. **Extract code to new package** (GREEN phase):
   ```bash
   # Create new package
   mkdir -p pkg/[service]/phase

   # Move code
   # ... extract code to pkg/[service]/phase/types.go
   ```

5. **Run all tests** (should pass):
   ```bash
   make test-unit-[service]
   make test-integration-[service]
   ```

6. **Refactor** if needed (REFACTOR phase):
   - Improve naming
   - Add documentation
   - Optimize logic

---

### Phase 2: Integration Test Coverage

**After each pattern adoption**:

```go
// test/integration/[service]/refactoring_validation_test.go

var _ = Describe("[Service] Refactoring Validation", func() {
    Context("Phase State Machine", func() {
        It("should handle invalid transitions gracefully", func() {
            // Create object in Completed phase
            obj := createTestObject(phase.Completed)

            // Attempt invalid transition
            err := phaseManager.TransitionTo(obj, phase.Pending)

            // Should reject
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("invalid phase transition"))
        })
    })

    Context("Status Manager", func() {
        It("should retry on conflict", func() {
            // Create object
            obj := createTestObject()

            // Simulate conflict by updating in background
            go func() {
                time.Sleep(50 * time.Millisecond)
                obj.ResourceVersion = "updated"
            }()

            // Update should retry and succeed
            err := statusManager.UpdateStatusWithRetry(ctx, obj, 3)
            Expect(err).ToNot(HaveOccurred())
        })
    })
})
```

---

### Phase 3: End-to-End Validation

**Run full test suite**:

```bash
# Unit tests
make test-unit-[service]

# Integration tests
make test-integration-[service]

# E2E tests
make test-e2e-[service]

# All tiers
make test-[service]
```

**Verify metrics**:
- ✅ 100% test pass rate maintained
- ✅ No performance regression
- ✅ Code coverage ≥ current baseline

---

## 📊 Progress Tracking Template

Use this template to track refactoring progress:

```markdown
# [Service] Refactoring Progress

## Phase 1: Quick Wins (P1) - Target: Week 1

- [ ] Terminal State Logic (4-6 hours)
  - [ ] Create pkg/[service]/phase/types.go with IsTerminal()
  - [ ] Replace 4 duplications in controller
  - [ ] Run tests (make test-unit-[service])
  - [ ] **Lines saved**: ~50

- [ ] Status Manager Adoption (4-6 hours)
  - [ ] Enhance existing pkg/[service]/status/manager.go
  - [ ] Replace controller's updateStatusWithRetry()
  - [ ] Run tests (make test-unit-[service])
  - [ ] **Lines saved**: ~100

**Week 1 Goal**: 150 lines saved, 100% test pass rate

---

## Phase 2: High-Impact (P0) - Target: Week 2-3

- [ ] Phase State Machine (2-3 days)
  - [ ] Create pkg/[service]/phase/manager.go
  - [ ] Define ValidTransitions map
  - [ ] Implement CanTransition(), Validate()
  - [ ] Update controller to use phase.Manager
  - [ ] Run tests (make test-unit-[service])
  - [ ] **Lines saved**: ~400

- [ ] Creator/Orchestrator (2-3 days)
  - [ ] Create pkg/[service]/creator/ or delivery/orchestrator.go
  - [ ] Extract large methods from controller
  - [ ] Update controller to delegate
  - [ ] Run tests (make test-integration-[service])
  - [ ] **Lines saved**: ~200

**Week 2-3 Goal**: 600 lines saved, 100% test pass rate

---

## Phase 3: Architecture Improvements (P2) - Target: Week 4-5

- [ ] Controller Decomposition (1 week)
  - [ ] Create phase_handlers.go
  - [ ] Create delivery_handlers.go
  - [ ] Move methods to appropriate files
  - [ ] Run tests (make test-[service])
  - [ ] **Lines saved**: Main controller < 300 lines

- [ ] Interface-Based Services (3 days)
  - [ ] Update all services to implement interface
  - [ ] Change controller to use map[Channel]Service
  - [ ] Update main.go to register services
  - [ ] Run tests (make test-[service])
  - [ ] **Benefit**: Easier to add channels

**Week 4-5 Goal**: Controller < 700 lines, 100% test pass rate

---

## Phase 4: Polish (P3) - Target: Week 6

- [ ] Audit Manager (2 days)
  - [ ] Create pkg/[service]/audit/manager.go
  - [ ] Move audit methods from controller
  - [ ] Update controller to use EventManager
  - [ ] Run tests (make test-[service])
  - [ ] **Lines saved**: ~300

**Week 6 Goal**: Controller < 690 lines (-54%), 100% test pass rate

---

## Final Validation

- [ ] All tests pass (unit + integration + e2e)
- [ ] No performance regression
- [ ] Code coverage maintained or improved
- [ ] Documentation updated
- [ ] Team review completed
```

---

## 🚨 Common Pitfalls and How to Avoid Them

### 1. Breaking Tests During Extraction

**Problem**: Extracting code causes test failures

**Solution**:
- ✅ Run tests BEFORE extraction (establish baseline)
- ✅ Extract small pieces (one pattern at a time)
- ✅ Run tests AFTER each extraction
- ✅ Use git branches for each phase

### 2. Over-Engineering

**Problem**: Creating too many abstractions

**Solution**:
- ✅ Follow YAGNI (You Aren't Gonna Need It)
- ✅ Only extract when clear benefit exists
- ✅ Start with P0/P1 patterns (proven high ROI)
- ✅ Avoid premature abstraction

### 3. Incomplete Migration

**Problem**: Old and new patterns coexist, causing confusion

**Solution**:
- ✅ Complete one pattern fully before starting next
- ✅ Remove old code after extraction
- ✅ Update all call sites
- ✅ Search for duplicate logic

### 4. Losing Business Context

**Problem**: Extracted code loses connection to business requirements

**Solution**:
- ✅ Keep BR-XXX-XXX references in extracted code
- ✅ Document why extraction happened
- ✅ Maintain DD-XXX design decision docs
- ✅ Update architecture documentation

### 5. Test Complexity Increases

**Problem**: Extracted code requires more complex test setup

**Solution**:
- ✅ Provide test helpers in extracted package
- ✅ Use testutil patterns
- ✅ Mock at package boundaries, not internal methods
- ✅ Follow RO's test patterns

### 6. Metrics Naming Misalignment (NT Lesson)

**Problem**: Service metrics don't follow DD-005 naming standards, causing E2E test failures

**NT Case Study**:
- NT metrics used flat prefix: `notification_reconciler_requests_total`
- DD-005 requires: `kubernaut_notification_reconciler_requests_total`
- Root cause: Missing `Namespace` and `Subsystem` in metric definitions
- Time lost: 2 hours debugging E2E failures

**Solution**:
- ✅ **Before metrics refactoring**: Consult Gateway team on DD-005 compliance
- ✅ Always use `Namespace: "kubernaut"` and `Subsystem: "[service]"` in metrics
- ✅ Compare with working reference (RO service) before implementing
- ✅ Cross-team expert consultation resolves domain issues 80% faster than solo debugging

**Pattern**: When refactoring shared infrastructure (metrics, audit, observability):
1. Identify domain expert team (Gateway for metrics, Audit for events)
2. Review their reference implementation first
3. Ask for 15-min consultation before implementation
4. Result: **Faster resolution, correct-by-design implementation**

---

## 🤝 Cross-Team Collaboration Patterns

### When to Consult Other Teams

| Domain | Expert Team | Consult Before | Time Saved |
|--------|-------------|----------------|------------|
| **Metrics** | Gateway | Refactoring metrics code | 80% (2h → 20min) |
| **Audit Events** | Audit Service | Changing event structure | 70% |
| **OpenAPI Clients** | API Platform | Adopting new API patterns | 60% |
| **Phase State Machines** | Remediation Orchestrator | Implementing state logic | 50% |
| **Circuit Breakers** | Platform/SRE | Retry pattern changes | 40% |

### Consultation Protocol

**15-Minute Expert Consultation Template**:

1. **Context** (2 min): "I'm refactoring [X] in [Service]"
2. **Question** (3 min): "Your team has [Y], should I reuse or create new?"
3. **Reference** (5 min): Expert shows reference implementation
4. **Decision** (5 min): Agree on approach (reuse/adapt/create new)

**NT Metrics Example**:
- **Without consultation**: 2 hours debugging E2E failures
- **With consultation**: 20 minutes following RO pattern
- **ROI**: 6x time savings + correct implementation

### Knowledge Transfer Artifacts

After refactoring, create:

1. **Case Study**: `docs/architecture/case-studies/[SERVICE]_REFACTORING_[YEAR].md`
   - Results achieved
   - Lessons learned
   - Recommendations for next service
   - **Permanent reference** (won't be archived)

2. **Pattern Library Update**: This document (add lessons to relevant sections)

3. **Brown Bag Session**: 30-min team presentation on key insights

4. **Handoff Document** (Optional): `docs/handoff/[SERVICE]_REFACTORING_COMPLETE_[DATE].md`
   - Detailed play-by-play for immediate team reference
   - Will be archived, so extract key lessons to case study

**NT Example**: Created permanent case study covering 12.5-hour journey, benefiting SP/WE teams

---

## 📚 References

### Primary References

1. **[NT Refactoring Case Study 2025](../case-studies/NT_REFACTORING_2025.md)** ⭐ **PERMANENT REFERENCE**
   - Production-validated lessons learned (12.5 hours, 100% test pass rate)
   - Pattern application results and metrics
   - Cross-team collaboration insights
   - Best practices and recommendations

2. **`pkg/remediationorchestrator/`** (Reference Implementation - Gold Standard)
   - `phase/types.go` - Phase State Machine
   - `phase/manager.go` - Phase Manager
   - `creator/` - Creator Pattern (5 files)
   - `controller/` - Decomposed Controller (5 files)
   - `audit/helpers.go` - Audit Helpers

5. **`internal/controller/notification/`** (Reference Implementation - Recent Validation)
   - `notificationrequest_controller.go` - Main controller (1133 lines, -23%)
   - `routing_handler.go` - Routing logic extraction (196 lines)
   - `retry_circuit_breaker_handler.go` - Retry/CB extraction (187 lines)
   - Demonstrates functional domain extraction strategy

### Business Requirements

- **BR-NOT-050 through BR-NOT-068**: Notification service requirements
- **BR-ORCH-001 through BR-ORCH-046**: Remediation Orchestrator requirements
- **BR-SP-001 through BR-SP-072**: Signal Processing requirements
- **BR-WE-001 through BR-WE-045**: Workflow Execution requirements

### Design Decisions

- **DD-NOT-002 V3.0**: Interface-First Approach
- **DD-API-001**: OpenAPI Client Adoption
- **DD-AUDIT-003**: Real Service Integration
- **DD-METRICS-001**: Metrics Wiring Standards
- **DD-005**: Observability Standards

### Testing Standards

- **[TESTING_GUIDELINES.md](../../../docs/development/business-requirements/TESTING_GUIDELINES.md)**: V2.1.0
- **[SERVICE_MATURITY_REQUIREMENTS.md](../../services/SERVICE_MATURITY_REQUIREMENTS.md)**: V1.2.0

---

## 🎯 Quick Start Guide

### For NT Service Team

**✅ COMPLETED** (December 2025):

NT has successfully applied 4/7 patterns:
- ✅ Pattern 2: Terminal State Logic
- ✅ Pattern 4: Status Manager (wired existing infrastructure)
- ✅ Pattern 5: Controller Decomposition (functional domain approach)
- ✅ Cross-team collaboration (Gateway team for metrics)

**Results**:
- Main controller: 1133 lines (down from 1472, -23%)
- 100% integration test pass rate (129/129)
- 100% E2E test pass rate (14/14)

**Next Steps for Other Services**: Review NT lessons learned in [NT Refactoring Case Study](../case-studies/NT_REFACTORING_2025.md)

---

### For SP Service Team

**Recommended Focus**:

1. **Week 1**: Terminal State Logic + Status Manager (if exists)
2. **Week 2-3**: Extract Classification Orchestrator
3. **Week 4-5**: Extract Enrichment Orchestrator

---

### For WE Service Team

**Recommended Focus**:

1. **Week 1**: Terminal State Logic
2. **Week 2-3**: Extract Pipeline Executor
3. **Week 4**: Extract Resource Lock Manager

---

## 💡 Success Criteria

### Per-Pattern Success

After adopting each pattern, verify:

- ✅ **Tests Pass**: 100% test pass rate maintained
- ✅ **Lines Reduced**: Expected line reduction achieved
- ✅ **No Regressions**: No performance degradation
- ✅ **Documentation**: Pattern adoption documented
- ✅ **Team Review**: Code review completed and approved

### Overall Refactoring Success

After completing all phases:

- ✅ **Controller Size**: Main controller < 800 lines
- ✅ **Code Quality**: Maintainability score > 85/100
- ✅ **Test Coverage**: Maintained or improved
- ✅ **Team Velocity**: No significant slowdown
- ✅ **New Features**: Easier to add (demonstrated)

---

## 🤝 Getting Help

### Questions or Issues?

1. **Reference Implementation**: Study `pkg/remediationorchestrator/`
2. **Analysis Documents**: See [References](#references) section
3. **Team Discussion**: Bring questions to architecture review
4. **Code Review**: Request review from RO team members

### Contributing Improvements

Found a better pattern? Improved the approach?

1. Document your improvement
2. Share with architecture team
3. Update this pattern library
4. Help other services adopt

---

**Document Version**: 1.0.0
**Last Updated**: December 20, 2025
**Owner**: Architecture Team
**Status**: ✅ AUTHORITATIVE REFERENCE - Ready for team adoption
**Next Review**: After first service completes Phase 1 refactoring


