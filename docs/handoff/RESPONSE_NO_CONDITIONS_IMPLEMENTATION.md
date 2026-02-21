# RESPONSE: Notification - Kubernetes Conditions Implementation

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**Date**: December 11, 2025
**Version**: 1.0
**From**: Notification Team
**To**: AIAnalysis Team
**Status**: ✅ **APPROVED - OPTION A (ROUTING ONLY)**
**Priority**: LOW

---

## 📋 Response Summary

**Decision**: ✅ **APPROVED WITH MODIFICATIONS**

**Approved**: Implement **RoutingResolved** condition only
**Deferred**: ChannelReachable condition (Prometheus metrics sufficient)
**Reason**: RoutingResolved provides unique routing visibility; ChannelReachable duplicates Prometheus metrics

---

## 🔍 **Triage Analysis Results**

### **Condition 1: RoutingResolved** ✅ **APPROVED**

**Current Gap Identified**:
- ✅ Channels resolved from routing rules logged but NOT in CRD status
- ✅ No visibility into which routing rule matched
- ✅ No way to debug routing fallback via `kubectl`

**Unique Value**:
1. **Routing Rule Visibility**: Operators can see which rule matched
2. **Label-Based Debugging**: Understand why certain channels were selected
3. **Fallback Detection**: Know when console fallback was used
4. **kubectl UX**: Routing diagnostics via `kubectl describe` without log access

**Example Operator Value**:
```bash
# BEFORE (no condition):
$ kubectl describe notif notif-123
Status:
  Phase: Sending
  # ❌ WHY did PagerDuty get paged? Need to check logs

# AFTER (with RoutingResolved):
$ kubectl describe notif notif-123
Status:
  Phase: Sending
  Conditions:
    Type:     RoutingResolved
    Status:   True
    Reason:   RoutingRuleMatched
    Message:  Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty
    # ✅ CLEAR: Production-critical rule matched, that's why PagerDuty was notified
```

**Verdict**: ✅ **IMPLEMENT** - High value, no overlap

---

### **Condition 2: ChannelReachable** ⏸️ **DEFERRED**

**Overlap Identified**:
- ✅ Circuit breaker state already tracked internally
- ✅ Prometheus metric `notification_channel_circuit_breaker_state` exposes circuit state
- ✅ `DeliveryAttempts[]` shows failure history

**Decision Rationale**:
- Prometheus provides circuit state visibility for operators with monitoring access
- Adding to CRD status would duplicate existing metric
- No compelling operator use case requiring kubectl-only access
- Can revisit if user feedback indicates kubectl visibility is needed

**Verdict**: ⏸️ **DEFER** - Re-evaluate based on production feedback

---

## 🎯 **Approved Business Requirement**

### **BR-NOT-069: Routing Rule Visibility via Kubernetes Conditions**

**Full Specification**: [BR-NOT-069-routing-rule-visibility-conditions.md](../requirements/BR-NOT-069-routing-rule-visibility-conditions.md)

**Category**: Observability (BR-NOT-XXX series)
**Priority**: P1 (HIGH) - Quality of Life Enhancement
**Status**: ✅ Approved (Kubernaut V1.0 - December 2025)

**Description**: The Notification Service MUST expose routing rule resolution status via Kubernetes Conditions, enabling operators to debug label-based channel routing without accessing controller logs.

**Rationale**:
- Label-based routing (BR-NOT-065) is complex with multiple matchers (type, severity, environment, namespace, skip-reason)
- Operators need to understand *why* specific channels were selected for debugging misconfigurations
- kubectl-based debugging is faster than log analysis for routing issues

**Acceptance Criteria**:
- ✅ `RoutingResolved` condition set after channel resolution
- ✅ Condition message includes matched rule name and resulting channels
- ✅ Condition reason indicates success (`RoutingRuleMatched`) or fallback (`RoutingFallback`)
- ✅ Condition visible via `kubectl describe notificationrequest`

**Related Requirements**:
- **BR-NOT-065**: Channel Routing Based on Labels (provides routing functionality)
- **BR-NOT-066**: Alertmanager-Compatible Configuration (routing rule format)
- **BR-NOT-067**: Routing Configuration Hot-Reload (routing rule updates)

**Test Coverage**:
- Unit: Condition helper functions (set/get/clear)
- Integration: Condition updates during routing resolution
- E2E: kubectl describe validation

---

## 🛠️ **Implementation Plan - Option A (RoutingResolved Only)**

### **Scope**: Single Condition Implementation

**Approved Condition**:
- ✅ `RoutingResolved` - Routing rule resolution status

**Deferred**:
- ⏸️ `ChannelReachable` - Per-channel circuit breaker state (Prometheus sufficient)

---

### **Phase 1: Infrastructure** (~30 minutes)

#### **File**: `pkg/notification/conditions.go`

**Implementation**: Copy pattern from AIAnalysis with notification-specific logic

```go
package notification

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

const (
	// ConditionTypeRoutingResolved indicates routing rule resolution completed
	ConditionTypeRoutingResolved = "RoutingResolved"

	// Routing success reasons
	ReasonRoutingRuleMatched = "RoutingRuleMatched"
	ReasonRoutingFallback    = "RoutingFallback"
	ReasonRoutingFailed      = "RoutingFailed"
)

// SetRoutingResolved sets the RoutingResolved condition
// BR-NOT-069: Routing Rule Visibility
func SetRoutingResolved(notif *notificationv1alpha1.NotificationRequest,
	status metav1.ConditionStatus, reason, message string) {
	setCondition(notif, ConditionTypeRoutingResolved, status, reason, message)
}

// GetRoutingResolved returns the RoutingResolved condition
func GetRoutingResolved(notif *notificationv1alpha1.NotificationRequest) *metav1.Condition {
	return getCondition(notif, ConditionTypeRoutingResolved)
}

// IsRoutingResolved checks if routing was successfully resolved
func IsRoutingResolved(notif *notificationv1alpha1.NotificationRequest) bool {
	cond := GetRoutingResolved(notif)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// Helper functions (private)

func setCondition(notif *notificationv1alpha1.NotificationRequest,
	condType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	for i, cond := range notif.Status.Conditions {
		if cond.Type == condType {
			// Update existing condition
			notif.Status.Conditions[i].Status = status
			notif.Status.Conditions[i].Reason = reason
			notif.Status.Conditions[i].Message = message
			notif.Status.Conditions[i].LastTransitionTime = now
			notif.Status.Conditions[i].ObservedGeneration = notif.Generation
			return
		}
	}

	// Add new condition
	notif.Status.Conditions = append(notif.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: notif.Generation,
	})
}

func getCondition(notif *notificationv1alpha1.NotificationRequest, condType string) *metav1.Condition {
	for i, cond := range notif.Status.Conditions {
		if cond.Type == condType {
			return &notif.Status.Conditions[i]
		}
	}
	return nil
}
```

**Confidence**: 95% (Copy from AIAnalysis pattern)

---

### **Phase 2: Controller Integration** (~1 hour)

#### **File**: `internal/controller/notification/notificationrequest_controller.go`

**Integration Point**: After routing resolution (line ~212)

**Implementation**:

```go
// BEFORE (line 204-212):
channels := notification.Spec.Channels
if len(channels) == 0 {
	channels = r.resolveChannelsFromRouting(ctx, notification)
	log.Info("Resolved channels from routing rules",
		"notification", notification.Name,
		"channels", channels,
		"labels", notification.Labels)
}

// AFTER (with condition):
channels := notification.Spec.Channels
if len(channels) == 0 {
	// BR-NOT-065: Resolve from routing rules
	channels, matchedRule := r.resolveChannelsFromRoutingWithDetails(ctx, notification)

	// BR-NOT-069: Set RoutingResolved condition
	if matchedRule != "" {
		message := fmt.Sprintf("Matched rule '%s' → channels: %v", matchedRule, channels)
		notification.SetRoutingResolved(notification, metav1.ConditionTrue,
			notification.ReasonRoutingRuleMatched, message)
		log.Info("Routing resolved via rule", "rule", matchedRule, "channels", channels)
	} else {
		// Fallback to console (no rules matched)
		message := "No routing rules matched, using console fallback"
		notification.SetRoutingResolved(notification, metav1.ConditionTrue,
			notification.ReasonRoutingFallback, message)
		log.Info("Routing fallback to console", "labels", notification.Labels)
	}

	// Update status to persist condition
	if err := r.updateStatusWithRetry(ctx, notification, 3); err != nil {
		log.Error(err, "Failed to update RoutingResolved condition")
		// Non-fatal - continue with delivery
	}
}
```

**New Helper Function**:

```go
// resolveChannelsFromRoutingWithDetails returns channels AND matched rule name
func (r *NotificationRequestReconciler) resolveChannelsFromRoutingWithDetails(
	ctx context.Context,
	notification *notificationv1alpha1.NotificationRequest,
) ([]notificationv1alpha1.Channel, string) {

	if r.Router == nil {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}, ""
	}

	// Get routing config
	r.routingMu.RLock()
	config := r.routingConfig
	r.routingMu.RUnlock()

	if config == nil {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}, ""
	}

	// Extract labels
	labels := notification.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	// Find matching receiver
	receiverName := config.Route.FindReceiver(labels)

	// Get receiver configuration
	receiver := config.GetReceiver(receiverName)
	if receiver == nil {
		return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}, ""
	}

	// Convert string channels to typed channels
	channelStrings := receiver.GetChannels()
	channels := routing.ConvertToTypedChannels(channelStrings)

	// Return channels + matched rule name (receiver name serves as rule identifier)
	return channels, receiverName
}
```

**Confidence**: 85% (Need to verify Router/Config access patterns)

---

### **Phase 3: Testing** (~1 hour)

#### **Unit Tests**: `test/unit/notification/conditions_test.go`

**Test Coverage** (BR-NOT-069):

```go
package notification_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification"
)

var _ = Describe("Notification Conditions (BR-NOT-069)", func() {
	var notif *notificationv1alpha1.NotificationRequest

	BeforeEach(func() {
		notif = &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-notification",
				Namespace: "default",
				Generation: 1,
			},
			Status: notificationv1alpha1.NotificationRequestStatus{
				Conditions: []metav1.Condition{},
			},
		}
	})

	Context("RoutingResolved Condition", func() {
		It("should set RoutingResolved condition successfully", func() {
			notification.SetRoutingResolved(notif, metav1.ConditionTrue,
				notification.ReasonRoutingRuleMatched,
				"Matched rule 'production-critical' → channels: slack, email")

			cond := notification.GetRoutingResolved(notif)
			Expect(cond).ToNot(BeNil())
			Expect(cond.Type).To(Equal(notification.ConditionTypeRoutingResolved))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(notification.ReasonRoutingRuleMatched))
			Expect(cond.Message).To(ContainSubstring("production-critical"))
		})

		It("should update existing RoutingResolved condition", func() {
			// Set initial condition
			notification.SetRoutingResolved(notif, metav1.ConditionTrue,
				notification.ReasonRoutingRuleMatched, "Rule 1")

			// Update condition
			notification.SetRoutingResolved(notif, metav1.ConditionTrue,
				notification.ReasonRoutingFallback, "Fallback to console")

			// Should have only one condition
			Expect(notif.Status.Conditions).To(HaveLen(1))

			cond := notification.GetRoutingResolved(notif)
			Expect(cond.Reason).To(Equal(notification.ReasonRoutingFallback))
			Expect(cond.Message).To(Equal("Fallback to console"))
		})

		It("should return true for IsRoutingResolved when condition is True", func() {
			notification.SetRoutingResolved(notif, metav1.ConditionTrue,
				notification.ReasonRoutingRuleMatched, "Test")

			Expect(notification.IsRoutingResolved(notif)).To(BeTrue())
		})

		It("should return false for IsRoutingResolved when condition is False", func() {
			notification.SetRoutingResolved(notif, metav1.ConditionFalse,
				notification.ReasonRoutingFailed, "No rules matched")

			Expect(notification.IsRoutingResolved(notif)).To(BeFalse())
		})

		It("should handle fallback scenario", func() {
			notification.SetRoutingResolved(notif, metav1.ConditionTrue,
				notification.ReasonRoutingFallback,
				"No routing rules matched, using console fallback")

			cond := notification.GetRoutingResolved(notif)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(notification.ReasonRoutingFallback))
			Expect(cond.Message).To(ContainSubstring("console fallback"))
		})
	})
})
```

#### **Integration Tests**: `test/integration/notification/routing_conditions_test.go`

**Test Scenarios**:
1. Routing rule matches → RoutingResolved = True (RuleMatched)
2. No routing rules → RoutingResolved = True (Fallback)
3. Multiple labels matched → Condition shows matched rule name
4. Hot-reload config → Condition updates with new rule

**Confidence**: 90% (Standard Ginkgo/Gomega patterns)

---

### **Phase 4: Documentation** (~30 minutes)

#### **Files to Update**:

1. **`docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md`**
   - Add BR-NOT-069 to Category 9 (Channel Routing)
   - Document RoutingResolved condition

2. **`docs/services/crd-controllers/06-notification/api-specification.md`**
   - Add Conditions section showing RoutingResolved example

3. **`docs/services/crd-controllers/06-notification/testing-strategy.md`**
   - Document condition testing strategy (unit + integration)

4. **`api/notification/v1alpha1/notificationrequest_types.go`**
   - Add godoc comments for Conditions field

**Confidence**: 95% (Documentation structure exists)

---

## 📊 **Effort Summary**

| Phase | Duration | Confidence |
|---|---|---|
| Create `conditions.go` | 30 min | 95% |
| Controller integration | 1 hour | 85% |
| Unit + integration tests | 1 hour | 90% |
| Documentation | 30 min | 95% |
| **Total** | **3 hours** | **90%** |

**Why 3 hours** (vs original 2-3 hours):
- Only implementing 1 condition (RoutingResolved)
- Deferring ChannelReachable saves 1 hour
- Router integration needs testing

---

## ✅ **Benefits**

### **Routing Debugging Improvements**

**Before** (no condition):
```bash
$ kubectl describe notif escalation-rr-001
Status:
  Phase: Sent
  Delivery Attempts:
    - Channel: slack
      Status: success
    - Channel: pagerduty
      Status: success
  # ❌ Why was PagerDuty notified? Need controller logs
```

**After** (with RoutingResolved):
```bash
$ kubectl describe notif escalation-rr-001
Status:
  Phase: Sent
  Conditions:
    Type:     RoutingResolved
    Status:   True
    Reason:   RoutingRuleMatched
    Message:  Matched rule 'production-critical' (severity=critical, env=production, type=escalation) → channels: slack, email, pagerduty
  Delivery Attempts:
    - Channel: slack
      Status: success
    - Channel: pagerduty
      Status: success
  # ✅ Clear: Production-critical rule matched because of severity=critical + env=production
```

### **Operator Benefits**

1. **Routing Rule Validation**: Confirm rules work as expected
2. **Label Debugging**: Understand which labels triggered routing
3. **Fallback Detection**: Know when console fallback was used (config issue)
4. **kubectl UX**: No need to access controller logs for routing issues

---

## 🔗 **Related Documentation**

- **AIAnalysis Implementation**: `docs/handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md`
- **AIAnalysis Code**: `pkg/aianalysis/conditions.go`
- **BR-NOT-065**: Channel Routing Based on Labels
- **BR-NOT-066**: Alertmanager-Compatible Configuration Format
- **BR-NOT-067**: Routing Configuration Hot-Reload

---

## 📝 **Implementation Checklist**

- [ ] Create `pkg/notification/conditions.go` infrastructure
- [ ] Update controller to set RoutingResolved condition
- [ ] Add `resolveChannelsFromRoutingWithDetails` helper function
- [ ] Create unit tests (`test/unit/notification/conditions_test.go`)
- [ ] Create integration tests (`test/integration/notification/routing_conditions_test.go`)
- [ ] Update `BUSINESS_REQUIREMENTS.md` with BR-NOT-069
- [ ] Update `api-specification.md` with Conditions documentation
- [ ] Update `testing-strategy.md` with condition testing approach
- [ ] Verify `kubectl describe` output format
- [ ] Run full test suite (unit + integration + e2e)

---

## 🎯 **Success Metrics**

- ✅ RoutingResolved condition appears in `kubectl describe` output
- ✅ Condition message includes matched rule name and channels
- ✅ Unit tests achieve 90%+ coverage of condition helpers
- ✅ Integration tests validate condition lifecycle
- ✅ Zero build or lint errors
- ✅ Documentation updated with BR-NOT-069

---

## 🔄 **Future Considerations**

### **Deferred: ChannelReachable Condition**

**Reason for Deferral**:
- Circuit breaker state already visible via Prometheus metric `notification_channel_circuit_breaker_state`
- No operator feedback indicating kubectl-only access is needed
- Would duplicate existing monitoring infrastructure

**Re-evaluation Triggers**:
1. **User Feedback**: Operators request kubectl-based circuit state visibility
2. **Prometheus Access Issues**: Teams without Prometheus access need circuit diagnostics
3. **Alternative Design**: Implement as status field instead of Condition (simpler)

**Estimated Re-implementation Effort**: 1.5 hours (if approved later)

---

**Document Status**: ✅ Approved - Ready for Implementation
**Target Version**: Kubernaut V1.0 (December 2025)
**Priority**: P1 (HIGH) - Quality of Life Enhancement
**Business Requirement**: BR-NOT-069 (Proposed)

**File**: `docs/handoff/RESPONSE_NO_CONDITIONS_IMPLEMENTATION.md`
