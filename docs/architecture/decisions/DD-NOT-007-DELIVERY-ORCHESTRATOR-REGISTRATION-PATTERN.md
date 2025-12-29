# DD-NOT-007: Delivery Channel Registration Pattern - AUTHORITATIVE

**Date**: December 22, 2025
**Status**: ✅ **AUTHORITATIVE** - Mandatory for all delivery channels
**Priority**: 🔴 **HIGH** - Architecture standard for extensibility
**Authority**: Defines channel architecture for Notification service
**Compliance**: MANDATORY for all current and future delivery channels

---

## 🎯 **Authoritative Standard**

**MANDATE**: All delivery channels in the Notification service MUST use the registration pattern defined in this document.

**Scope**: This standard applies to:
- ✅ All existing delivery channels (Console, Slack, File, Log)
- ✅ All future delivery channels (Email, PagerDuty, Teams, Webhook, etc.)
- ✅ Integration test setup
- ✅ E2E test setup
- ✅ Production deployment

**Non-Compliance**: Adding channels through constructor parameters or switch statements is FORBIDDEN.

---

## 🚨 **Historical Context: Why This Standard Exists**

**Legacy Design Smell**: Delivery Orchestrator previously hardcoded channel services in constructor

```go
// ❌ CURRENT BAD DESIGN
func NewOrchestrator(
	consoleService DeliveryService,  // Hardcoded parameter #1
	slackService DeliveryService,    // Hardcoded parameter #2
	fileService DeliveryService,     // Hardcoded parameter #3
	logService DeliveryService,      // Hardcoded parameter #4
	sanitizer *sanitization.Sanitizer,
	metrics notificationmetrics.Recorder,
	statusManager *notificationstatus.Manager,
	logger logr.Logger,
) *Orchestrator
```

**Impact**:
- ❌ **Brittle constructor** - Adding new channel = breaking change
- ❌ **Switch statement coupling** - Channel routing hardcoded in `DeliverToChannel()`
- ❌ **Test friction** - Must pass `nil` for unused channels
- ❌ **Poor extensibility** - Can't dynamically register/unregister channels
- ❌ **Violates Open/Closed Principle** - Not open for extension without modification

---

## 🎯 **Quick Reference Card**

### **MANDATORY Pattern for All Channels**

```go
// ✅ STEP 1: Implement DeliveryService interface
type MyChannelService struct { ... }
func (s *MyChannelService) Deliver(ctx, notification) error { ... }

// ✅ STEP 2: Add enum to CRD
const ChannelMyChannel Channel = "mychannel"

// ✅ STEP 3: Register in production
deliveryOrchestrator.RegisterChannel(
    string(notificationv1alpha1.ChannelMyChannel),
    myChannelService,
)

// ✅ STEP 4: Test with registration
orchestrator.RegisterChannel("mychannel", mockService)
```

### **FORBIDDEN Patterns** ❌

```go
// ❌ DO NOT add constructor parameters
func NewOrchestrator(myChannel DeliveryService, ...) // FORBIDDEN

// ❌ DO NOT add channel fields
type Orchestrator struct {
    myChannelService DeliveryService  // FORBIDDEN
}

// ❌ DO NOT add switch cases
switch channel {
    case ChannelMyChannel:  // FORBIDDEN
        return o.deliverToMyChannel()
}

// ❌ DO NOT pass nil in tests
NewOrchestrator(console, nil, nil, ...)  // FORBIDDEN
```

---

## ✅ **What's Already Good**

**We DO have a common interface** (DD-NOT-002 V3.0):

```go
// ✅ GOOD: Common interface exists
type DeliveryService interface {
	Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}
```

**All channels implement this interface**:
- ✅ `ConsoleService`
- ✅ `SlackService`
- ✅ `FileService`
- ✅ `LogService`

**This is the foundation for a registration pattern!**

---

## 📋 **Mandatory Architecture: Registration Pattern**

### **Core Design Principles** (REQUIRED)

1. **Dynamic Registration** - Channels MUST register via `RegisterChannel()`, not constructor
2. **Type-Safe Routing** - Map channel name → `DeliveryService` interface
3. **Optional Channels** - NOT all channels required at construction (test flexibility)
4. **Interface-Based** - All channels MUST implement `DeliveryService` interface
5. **Future-Proof** - New channels MUST NOT change constructor signature

### **Forbidden Patterns** ❌

The following patterns are STRICTLY FORBIDDEN:
- ❌ Hardcoding channel services as constructor parameters
- ❌ Using switch statements for channel routing
- ❌ Adding channel-specific fields to Orchestrator struct
- ❌ Creating channel-specific delivery methods (e.g., `deliverToConsole()`)
- ❌ Passing `nil` for unused channels in tests

---

## ✅ **Mandatory Implementation Pattern**

#### **1. Orchestrator with Registration**

```go
// ✅ IMPROVED DESIGN
type Orchestrator struct {
	// Channel registry (dynamic registration)
	channels map[string]DeliveryService  // NEW: Channel name → service mapping

	// Dependencies (unchanged)
	sanitizer     *sanitization.Sanitizer
	metrics       notificationmetrics.Recorder
	statusManager *notificationstatus.Manager
	logger        logr.Logger
}

// Simplified constructor (no channel parameters!)
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

// ✅ NEW: Register channel dynamically
func (o *Orchestrator) RegisterChannel(channelName string, service DeliveryService) {
	if service == nil {
		o.logger.Info("Skipping nil service registration", "channel", channelName)
		return
	}
	o.channels[channelName] = service
	o.logger.Info("Registered delivery channel", "channel", channelName)
}

// ✅ NEW: Unregister channel (for testing/runtime changes)
func (o *Orchestrator) UnregisterChannel(channelName string) {
	delete(o.channels, channelName)
	o.logger.Info("Unregistered delivery channel", "channel", channelName)
}

// ✅ NEW: Check if channel is registered
func (o *Orchestrator) HasChannel(channelName string) bool {
	_, exists := o.channels[channelName]
	return exists
}
```

#### **2. Simplified Channel Delivery (No Switch!)**

```go
// ✅ IMPROVED: No switch statement, no hardcoded channel routing
func (o *Orchestrator) DeliverToChannel(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
	channel notificationv1alpha1.Channel,
) error {
	// Look up channel service from registry
	service, exists := o.channels[string(channel)]
	if !exists {
		return fmt.Errorf("channel not registered: %s", channel)
	}

	// Sanitize before delivery
	sanitized := o.sanitizeNotification(notification)

	// Delegate to channel service
	return service.Deliver(ctx, sanitized)
}
```

**Benefits**:
- ✅ No switch statement - routing handled by map lookup
- ✅ No individual `deliverToConsole()`, `deliverToSlack()` methods
- ✅ Adding new channel = register it, no code changes in orchestrator
- ✅ Type-safe - map key is `string`, value is `DeliveryService` interface

#### **3. Production Usage (Improved)**

```go
// cmd/notification/main.go (BEFORE)
// ❌ OLD: Hardcoded 4 parameters
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

// ✅ NEW: Registration pattern
deliveryOrchestrator := delivery.NewOrchestrator(
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// Register channels based on configuration
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), fileService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelLog), logService)

// Future channels: Just register them!
// deliveryOrchestrator.RegisterChannel("email", emailService)
// deliveryOrchestrator.RegisterChannel("pagerduty", pagerDutyService)
```

#### **4. Test Usage (Much Cleaner!)**

```go
// test/integration/notification/suite_test.go (BEFORE)
// ❌ OLD: Must pass nil for unused channels
deliveryOrchestrator := delivery.NewOrchestrator(
	consoleService,
	slackService,
	nil, // fileService (E2E only, not needed in integration tests)
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// ✅ NEW: Register only needed channels
deliveryOrchestrator := delivery.NewOrchestrator(
	sanitizer,
	metricsRecorder,
	statusManager,
	ctrl.Log.WithName("delivery-orchestrator"),
)

// Only register what we need for this test
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
deliveryOrchestrator.RegisterChannel(string(notificationv1alpha1.ChannelSlack), slackService)
// fileService not registered - will return "channel not registered" error if used
```

---

## 🔒 **Mandatory Requirements for New Channels**

### **REQUIRED: DeliveryService Interface Implementation**

```go
// MANDATORY: All channels MUST implement this interface
type DeliveryService interface {
	// Deliver sends a notification through this delivery mechanism.
	//
	// REQUIREMENTS:
	// - MUST respect context cancellation
	// - MUST return descriptive errors
	// - SHOULD be idempotent (safe to retry)
	// - SHOULD log delivery attempts (structured logging)
	Deliver(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error
}
```

### **REQUIRED: Channel Registration in Production**

```go
// cmd/notification/main.go - MANDATORY PATTERN

// Step 1: Create channel service
newChannelService := newchannel.NewService(config.NewChannel, logger)

// Step 2: Register with orchestrator
deliveryOrchestrator.RegisterChannel(
	string(notificationv1alpha1.ChannelNewChannel),  // Channel enum from CRD
	newChannelService,
)
```

### **REQUIRED: Channel Enum in CRD**

```go
// api/notification/v1alpha1/notificationrequest_types.go

type Channel string

const (
	ChannelConsole Channel = "console"
	ChannelSlack   Channel = "slack"
	ChannelFile    Channel = "file"
	ChannelLog     Channel = "log"
	// REQUIRED: Add new channel enum here
	ChannelNewChannel Channel = "newchannel"  // Example
)
```

### **REQUIRED: Test Registration Pattern**

```go
// Integration/E2E tests - MANDATORY PATTERN

orchestrator := delivery.NewOrchestrator(
	sanitizer,
	metricsRecorder,
	statusManager,
	logger,
)

// Register ONLY channels needed for this test
orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), consoleService)
// Do NOT register unused channels - orchestrator will return descriptive error if used
```

---

## ✅ **Compliance Checklist for New Channels**

### **Before Implementing a New Channel**

**Mandatory Steps**:
- [ ] Channel implements `DeliveryService` interface
- [ ] Channel enum added to CRD (`notificationrequest_types.go`)
- [ ] Channel registered in production (`cmd/notification/main.go`)
- [ ] Channel has unit tests (70%+ coverage, per testing standards)
- [ ] Channel has integration tests (registration + delivery flow)
- [ ] Channel documented in service README
- [ ] NO constructor parameter added to `NewOrchestrator()`
- [ ] NO switch case added to `DeliverToChannel()`
- [ ] NO channel-specific field added to `Orchestrator` struct
- [ ] NO `nil` passing in tests

### **Code Review Checklist**

**Reviewers MUST verify**:
- ✅ Channel uses registration pattern (not constructor)
- ✅ No switch statement additions in orchestrator
- ✅ Interface implementation is complete
- ✅ Tests follow registration pattern
- ✅ Configuration is ADR-030 compliant

---

## 📋 **Standard Channel Implementation Template**

### **Step 1: Implement DeliveryService Interface**

```go
// pkg/notification/delivery/newchannel.go

package delivery

import (
	"context"
	"github.com/go-logr/logr"
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// NewChannelService implements DeliveryService for NewChannel delivery.
type NewChannelService struct {
	config NewChannelConfig
	logger logr.Logger
}

// NewService creates a new NewChannel delivery service.
func NewService(config NewChannelConfig, logger logr.Logger) *NewChannelService {
	return &NewChannelService{
		config: config,
		logger: logger,
	}
}

// Deliver implements DeliveryService interface.
func (s *NewChannelService) Deliver(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) error {
	log := s.logger.WithValues(
		"notification", notification.Name,
		"namespace", notification.Namespace,
		"channel", "newchannel",
	)

	// Implementation details...
	log.Info("Delivering notification via NewChannel")

	// Respect context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Delivery logic here
	}

	return nil
}
```

### **Step 2: Add Channel Enum to CRD**

```go
// api/notification/v1alpha1/notificationrequest_types.go

const (
	// ... existing channels ...
	ChannelNewChannel Channel = "newchannel"  // ADD THIS
)
```

### **Step 3: Register in Production**

```go
// cmd/notification/main.go

// Create service
newChannelService := newchannel.NewService(cfg.NewChannel, logger)

// Register with orchestrator
deliveryOrchestrator.RegisterChannel(
	string(notificationv1alpha1.ChannelNewChannel),
	newChannelService,
)
```

### **Step 4: Write Tests**

```go
// pkg/notification/delivery/newchannel_test.go

var _ = Describe("NewChannel Delivery", func() {
	var (
		service      *NewChannelService
		orchestrator *Orchestrator
		notification *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		service = NewService(testConfig, logger)
		orchestrator = NewOrchestrator(sanitizer, metrics, status, logger)

		// MANDATORY: Use registration pattern
		orchestrator.RegisterChannel(
			string(notificationv1alpha1.ChannelNewChannel),
			service,
		)

		notification = testutil.NewNotification("test")
	})

	It("should deliver notification successfully", func() {
		err := orchestrator.DeliverToChannel(
			ctx,
			notification,
			notificationv1alpha1.ChannelNewChannel,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return error if channel not registered", func() {
		// Create new orchestrator without registration
		emptyOrchestrator := NewOrchestrator(sanitizer, metrics, status, logger)

		err := emptyOrchestrator.DeliverToChannel(
			ctx,
			notification,
			notificationv1alpha1.ChannelNewChannel,
		)
		Expect(err).To(MatchError(ContainSubstring("channel not registered")))
	})
})
```

---

## 📊 **Impact Analysis**

### **Files Requiring Changes**

| File | Change Type | Complexity | Risk |
|-----|-----|-----|-----|
| `pkg/notification/delivery/orchestrator.go` | Refactor constructor + delivery method | Medium | Low |
| `cmd/notification/main.go` | Update instantiation + registration | Low | Low |
| `test/integration/notification/suite_test.go` | Update instantiation + registration | Low | Low |
| `test/e2e/notification/*_test.go` | Update instantiation + registration | Low | Low |

**Total Impact**: 🟡 **Medium** - Limited to 4-5 files, well-defined scope

---

### **Breaking Changes**

**Constructor Signature**:
```go
// OLD
func NewOrchestrator(console, slack, file, log DeliveryService, ...) *Orchestrator

// NEW
func NewOrchestrator(...) *Orchestrator
```

**Migration Strategy**:
1. Update `orchestrator.go` implementation
2. Update production usage (`cmd/notification/main.go`)
3. Update integration test usage (`test/integration/notification/suite_test.go`)
4. Update E2E test usage (if any direct instantiation)
5. Run full test suite to validate

**Estimated Effort**: 🟢 **1-2 hours** (low risk, mechanical changes)

---

## 🎓 **Design Principles Satisfied**

### **Open/Closed Principle** ✅
- **Open for extension** - New channels register without modifying orchestrator
- **Closed for modification** - Core delivery logic unchanged

### **Dependency Inversion Principle** ✅
- **Interface-based** - All channels implement `DeliveryService`
- **Loose coupling** - Orchestrator depends on interface, not concrete types

### **Single Responsibility Principle** ✅
- **Orchestrator** - Manages delivery flow, not channel-specific logic
- **DeliveryService** - Each channel handles its own delivery mechanism

### **Liskov Substitution Principle** ✅
- **Polymorphic** - All `DeliveryService` implementations interchangeable
- **Uniform interface** - `Deliver(ctx, notification)` contract

---

## 🚦 **Comparison: Current vs Proposed**

| Aspect | Current Design ❌ | Proposed Design ✅ |
|-----|-----|-----|
| **Adding new channel** | Constructor change + switch statement | Just register it |
| **Test setup** | Pass `nil` for unused channels | Register only needed channels |
| **Runtime flexibility** | Fixed at construction | Dynamic registration |
| **Code coupling** | Orchestrator knows all channels | Orchestrator channel-agnostic |
| **Constructor parameters** | 8 parameters (4 channels + 4 deps) | 4 parameters (deps only) |
| **Channel routing** | Hardcoded switch statement | Map lookup |
| **Extensibility** | Closed (requires modification) | Open (no modification needed) |

---

## 📋 **Migration Checklist**

### **Phase 1: Orchestrator Refactoring** (TDD RED)
- [ ] Add `channels map[string]DeliveryService` field to `Orchestrator`
- [ ] Update `NewOrchestrator()` to remove channel parameters
- [ ] Add `RegisterChannel(name string, service DeliveryService)`
- [ ] Add `UnregisterChannel(name string)` (for testing)
- [ ] Add `HasChannel(name string) bool` (for validation)
- [ ] Update `DeliverToChannel()` to use map lookup instead of switch
- [ ] Remove individual `deliverToConsole()`, `deliverToSlack()`, etc. methods
- [ ] Write unit tests for registration pattern

### **Phase 2: Production Code Update** (TDD GREEN)
- [ ] Update `cmd/notification/main.go` instantiation
- [ ] Add registration calls for all production channels
- [ ] Verify production deployment still works

### **Phase 3: Test Code Update** (TDD REFACTOR)
- [ ] Update integration test suite instantiation
- [ ] Update E2E test suite instantiation (if needed)
- [ ] Run full test suite to validate
- [ ] Check for any documentation references

### **Phase 4: Documentation Update**
- [ ] Update DD-NOT-006 to reference new registration pattern
- [ ] Update orchestrator godoc comments
- [ ] Add usage examples in code comments
- [ ] Create ADR if needed (if considered architectural change)

---

## 🎯 **Business Requirements Validation**

### **BR-NOT-053: At-Least-Once Delivery** ✅
**Impact**: None - Registration pattern doesn't affect delivery guarantees

### **BR-NOT-055: Retry Logic** ✅
**Impact**: None - Retry logic happens at orchestrator level (unchanged)

### **BR-NOT-034: Audit Trail** ✅
**Impact**: None - Audit hooks unchanged (callback functions)

### **DD-NOT-006: ChannelFile + ChannelLog** ✅
**Impact**: Positive - Easier to add/remove channels dynamically

---

## 🔧 **Implementation Strategy**

### **Option A: Gradual Refactoring** (Recommended)
1. Add registration methods alongside current constructor
2. Update production code to use registration
3. Update tests to use registration
4. Remove old hardcoded channel fields
5. Remove switch statement

**Pros**: Lower risk, incremental validation
**Cons**: More commits
**Effort**: ~2-3 hours

---

### **Option B: Big Bang Refactoring**
1. Refactor orchestrator in one commit
2. Update all usage sites in same commit
3. Run full test suite

**Pros**: Cleaner git history, faster completion
**Cons**: Higher risk if tests fail
**Effort**: ~1-2 hours

---

### **Recommendation**: **Option A** (Gradual)
- ✅ Lower risk for production code
- ✅ Easier to debug if issues arise
- ✅ Aligns with TDD RED-GREEN-REFACTOR approach

---

## 🚨 **Risks & Mitigation**

### **Risk 1: Breaking Existing Tests** 🟡 Medium
**Mitigation**: Update tests incrementally, validate after each change

### **Risk 2: Channel Registration Forgotten** 🟢 Low
**Mitigation**: Add validation to fail fast if unregistered channel used

### **Risk 3: Performance Impact (Map Lookup)** 🟢 Low
**Impact**: Negligible - map lookup is O(1), similar to field access
**Validation**: Benchmark if concerned

---

## 📚 **References**

### **Design Patterns**
- **Strategy Pattern** - DeliveryService interface for pluggable strategies
- **Registry Pattern** - Channel registration for dynamic service discovery
- **Facade Pattern** - Orchestrator simplifies complex delivery coordination

### **Kubernaut Standards**
- **[00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc)** - TDD workflow
- **[02-technical-implementation.mdc](mdc:.cursor/rules/02-technical-implementation.mdc)** - Go patterns
- **[DD-NOT-002 V3.0](mdc:docs/services/crd-controllers/06-notification/DD-NOT-002-FILE-BASED-E2E-TESTS_IMPLEMENTATION_PLAN_V3.0.md)** - DeliveryService interface definition

### **Related Design Decisions**
- **DD-NOT-006** - ChannelFile + ChannelLog features (would benefit from registration pattern)
- **ADR-030** - Configuration management (registration could be config-driven)

---

## ✅ **Success Criteria**

### **Code Quality**
- ✅ Orchestrator constructor has ≤5 parameters (down from 8)
- ✅ No switch statement in `DeliverToChannel()`
- ✅ Tests only register needed channels (no `nil` passing)

### **Extensibility**
- ✅ Adding new channel requires zero orchestrator code changes
- ✅ Channels can be registered/unregistered dynamically
- ✅ Test setup is cleaner and more explicit

### **Validation**
- ✅ All existing tests pass with new design
- ✅ Integration tests run successfully
- ✅ E2E tests run successfully
- ✅ Production build succeeds

---

## 🚨 **Enforcement Mechanisms**

### **Automated Detection**

**Pre-commit Hook** (add to `.git/hooks/pre-commit`):

```bash
#!/bin/bash
# Detect forbidden channel patterns in orchestrator

echo "🔍 Checking delivery orchestrator compliance with DD-NOT-007..."

# Check 1: No new channel parameters in NewOrchestrator constructor
CONSTRUCTOR_PARAMS=$(grep -A 10 "func NewOrchestrator" pkg/notification/delivery/orchestrator.go | grep "Service DeliveryService" | wc -l)
if [ "$CONSTRUCTOR_PARAMS" -gt 0 ]; then
    echo "❌ VIOLATION: NewOrchestrator has channel parameters (DD-NOT-007)"
    echo "   Required: Remove channel parameters, use RegisterChannel() instead"
    exit 1
fi

# Check 2: No switch statement in DeliverToChannel
SWITCH_STATEMENT=$(grep -A 20 "func.*DeliverToChannel" pkg/notification/delivery/orchestrator.go | grep "switch channel" | wc -l)
if [ "$SWITCH_STATEMENT" -gt 0 ]; then
    echo "❌ VIOLATION: DeliverToChannel uses switch statement (DD-NOT-007)"
    echo "   Required: Use map lookup (o.channels[string(channel)])"
    exit 1
fi

# Check 3: No channel-specific fields in Orchestrator struct
CHANNEL_FIELDS=$(grep -A 15 "type Orchestrator struct" pkg/notification/delivery/orchestrator.go | grep "Service.*DeliveryService" | wc -l)
if [ "$CHANNEL_FIELDS" -gt 0 ]; then
    echo "❌ VIOLATION: Orchestrator has channel-specific fields (DD-NOT-007)"
    echo "   Required: Use channels map[string]DeliveryService instead"
    exit 1
fi

echo "✅ DD-NOT-007 compliance verified"
```

### **Code Review Guidelines**

**MANDATORY Review Checklist**:
- [ ] Does PR add new channel using registration pattern?
- [ ] No constructor parameters added to `NewOrchestrator()`?
- [ ] No switch cases added to `DeliverToChannel()`?
- [ ] Channel implements `DeliveryService` interface?
- [ ] Tests use registration pattern (no `nil` passing)?
- [ ] Channel enum added to CRD types?
- [ ] Documentation updated?

**Auto-reject conditions** (CI/CD):
- ❌ Constructor signature changed
- ❌ Switch statement detected in orchestrator
- ❌ Channel-specific fields added to Orchestrator struct
- ❌ Tests pass `nil` for channels

---

## 🎓 **Design Rationale**

### **Why Registration Over Constructor Injection?**

| Concern | Constructor Injection ❌ | Registration Pattern ✅ |
|---------|-------------------------|------------------------|
| **Adding channels** | Breaking change | Non-breaking |
| **Test flexibility** | Must pass all channels | Register only needed |
| **Runtime changes** | Impossible | Supported |
| **Code coupling** | Tight (orchestrator knows all) | Loose (interface-based) |
| **Open/Closed** | Violates (modification needed) | Satisfies (extension only) |

### **Why Map-Based Routing?**

**Switch statement problems**:
- ❌ Must modify orchestrator for each new channel
- ❌ Violates Open/Closed Principle
- ❌ Creates tight coupling
- ❌ Harder to test in isolation

**Map-based routing benefits**:
- ✅ O(1) lookup performance (same as switch)
- ✅ No code modification for new channels
- ✅ Dynamic registration/unregistration
- ✅ Clearer separation of concerns

---

## 📚 **Integration with Kubernaut Standards**

### **Related Standards**

| Standard | Relationship | Reference |
|---------|-------------|-----------|
| **[00-core-development-methodology.mdc](mdc:.cursor/rules/00-core-development-methodology.mdc)** | TDD workflow for implementation | Foundational |
| **[02-technical-implementation.mdc](mdc:.cursor/rules/02-technical-implementation.mdc)** | Go interface patterns | Technical |
| **[ADR-030](mdc:docs/architecture/decisions/ADR-030-CONFIGURATION-MANAGEMENT.md)** | Configuration-driven registration | Configuration |
| **[DD-NOT-002 V3.0](mdc:docs/services/crd-controllers/06-notification/DD-NOT-002-FILE-BASED-E2E-TESTS_IMPLEMENTATION_PLAN_V3.0.md)** | DeliveryService interface origin | Historical |
| **[DD-NOT-006](mdc:docs/services/crd-controllers/06-notification/DD-NOT-006-CHANNEL-FILE-LOG-PRODUCTION-FEATURES.md)** | ChannelFile + ChannelLog implementation | Feature |

### **Authority Hierarchy**

```
Priority 1: [00-core-development-methodology.mdc] - TDD methodology
Priority 2: [02-technical-implementation.mdc] - Technical patterns
Priority 3: [DD-NOT-007] (THIS DOCUMENT) - Channel architecture
Priority 4: Service-specific patterns
```

**Conflict Resolution**: If service-specific patterns conflict with this DD, this DD takes precedence for Notification service channel architecture.

---

## 📋 **Migration from Legacy Pattern**

### **For Existing Deployments**

**Phase 1: Add Registration Methods** (Non-breaking)
```go
// Add to orchestrator.go (keeps old constructor for now)
func (o *Orchestrator) RegisterChannel(name string, service DeliveryService) {
	o.channels[name] = service
}
```

**Phase 2: Update Usage Sites** (Incremental)
```go
// Update production code to use registration
orchestrator := NewOrchestrator(...existing params...)
orchestrator.RegisterChannel("console", consoleService)  // Add alongside old params
```

**Phase 3: Remove Legacy Constructor** (Breaking change)
```go
// Remove channel parameters from constructor
// All usage sites now use registration
```

**Timeline**: 2-3 days (gradual rollout)

---

## ✅ **Authoritative Decision**

### **Status**: ✅ **ACCEPTED & MANDATORY**

**Effective Date**: December 22, 2025

**Scope**:
- ✅ MANDATORY for all new channels
- ✅ MANDATORY for existing channel refactoring (when touching code)
- ✅ MANDATORY for all tests (integration + E2E)
- ✅ ENFORCED by pre-commit hooks and code review

**Non-Compliance**:
- ❌ PRs violating this standard will be rejected
- ❌ Legacy pattern is deprecated (but exists temporarily in existing code)
- ❌ No exceptions without explicit architectural review

---

## 📋 **Future Enhancements**

### **Potential Improvements** (Not required now)

1. **Config-Driven Registration** (ADR-030 integration)
   ```yaml
   channels:
     console:
       enabled: true
     slack:
       enabled: true
       webhook_url: ${SLACK_WEBHOOK}
   ```

2. **Channel Health Checks**
   ```go
   type DeliveryService interface {
       Deliver(ctx, notification) error
       HealthCheck(ctx) error  // Optional future enhancement
   }
   ```

3. **Channel Priorities**
   ```go
   orchestrator.RegisterChannelWithPriority("critical", pagerDutyService, 1)
   orchestrator.RegisterChannelWithPriority("normal", slackService, 2)
   ```

4. **Channel Middleware**
   ```go
   orchestrator.RegisterMiddleware("rate-limiting", rateLimitMiddleware)
   orchestrator.RegisterMiddleware("retry", retryMiddleware)
   ```

---

---

## 📝 **Summary: What Developers Need to Know**

### **Adding a New Delivery Channel? Follow These 4 Steps**

1. ✅ **Implement `DeliveryService` interface** (`pkg/notification/delivery/mychannel.go`)
2. ✅ **Add enum to CRD** (`api/notification/v1alpha1/notificationrequest_types.go`)
3. ✅ **Register in production** (`cmd/notification/main.go`)
4. ✅ **Write tests with registration** (`pkg/notification/delivery/mychannel_test.go`)

### **What You Must NOT Do** ❌

1. ❌ Add parameters to `NewOrchestrator()` constructor
2. ❌ Add channel fields to `Orchestrator` struct
3. ❌ Add switch cases to `DeliverToChannel()`
4. ❌ Create channel-specific delivery methods (e.g., `deliverToMyChannel()`)
5. ❌ Pass `nil` for unused channels in tests

### **Where to Find Examples**

- **Interface**: `pkg/notification/delivery/interface.go`
- **Registration**: `cmd/notification/main.go` (lines ~295-302)
- **Test Usage**: `test/integration/notification/suite_test.go` (lines ~282-290)
- **Channel Implementation**: `pkg/notification/delivery/console.go` (simplest example)

### **Need Help?**

- **Full Template**: See "Standard Channel Implementation Template" section above
- **Compliance Checklist**: See "Compliance Checklist for New Channels" section
- **Code Review**: Reviewers will check DD-NOT-007 compliance

---

## 🎯 **TL;DR for Busy Developers**

**One Sentence**: Register delivery channels dynamically via `RegisterChannel()` instead of hardcoding them in the constructor.

**Why It Matters**: Makes adding new channels trivial (no breaking changes), simplifies tests, and follows Open/Closed Principle.

**What to Do**: Copy the template above, implement `DeliveryService`, and register your channel.

**What NOT to Do**: Don't touch `NewOrchestrator()` signature or add switch cases.

---

**Document Status**: ✅ **AUTHORITATIVE**
**Created**: December 22, 2025
**Last Updated**: December 22, 2025
**Authority**: Notification Service Channel Architecture
**Enforcement**: MANDATORY via pre-commit hooks + code review
**Prepared by**: AI Assistant (NT Team)
**Approved by**: User (jgil) - Architectural design review

**Compliance**: ALL delivery channels MUST follow this pattern. No exceptions.

---

## 📎 **Appendix: Related Documents**

- **[DeliveryService Interface](mdc:pkg/notification/delivery/interface.go)** - The common interface all channels implement
- **[Orchestrator Implementation](mdc:pkg/notification/delivery/orchestrator.go)** - Registration pattern implementation
- **[Production Usage](mdc:cmd/notification/main.go)** - How channels are registered in production
- **[Test Usage](mdc:test/integration/notification/suite_test.go)** - How channels are registered in tests
- **[DD-NOT-002 V3.0](mdc:docs/services/crd-controllers/06-notification/DD-NOT-002-FILE-BASED-E2E-TESTS_IMPLEMENTATION_PLAN_V3.0.md)** - Interface-first design origin
- **[DD-NOT-006](mdc:docs/services/crd-controllers/06-notification/DD-NOT-006-CHANNEL-FILE-LOG-PRODUCTION-FEATURES.md)** - Recent channel additions (File, Log)
- **[ADR-030](mdc:docs/architecture/decisions/ADR-030-CONFIGURATION-MANAGEMENT.md)** - Configuration management (for future config-driven registration)

