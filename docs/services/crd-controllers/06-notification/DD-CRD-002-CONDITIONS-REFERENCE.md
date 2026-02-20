# DD-CRD-002: NotificationRequest Conditions Reference

**Service**: Notification
**CRD**: NotificationRequest
**Status**: ✅ **COMPLETE - V1.0 COMPLIANT**
**Pattern**: Minimal (Routing/Decision Point)
**Last Updated**: 2025-12-16
**Authority**: DD-CRD-002-kubernetes-conditions-standard.md

---

## 📋 Document Purpose

This document provides the **authoritative specification** for all Kubernetes Conditions used by the NotificationRequest CRD controller. It serves as:

1. **Implementation Reference**: Complete specification for all condition types and reasons
2. **Testing Guide**: Expected behavior for unit, integration, and E2E tests
3. **Operations Manual**: How operators use conditions for debugging and automation
4. **V1.0 Compliance**: Evidence of DD-CRD-002 requirement satisfaction

**Authority**: This document is the single source of truth for NotificationRequest conditions.

---

## 🎯 Conditions Overview

### Condition Strategy

**NotificationRequest implements the "Minimal Pattern"** as defined in DD-CRD-002:

- **Single Condition Type**: `RoutingResolved` (binary decision point)
- **Clear Semantics**: True = routing successful, False = routing failed
- **Detailed Messages**: Includes matched rule names and selected channels
- **Business Backing**: BR-NOT-069 (Routing Rule Visibility via Conditions)

**Why Minimal Pattern?**
- ✅ **Simple decision point**: Routing is a single-phase, binary outcome
- ✅ **Clear observability**: Operators need visibility into which routing rule matched
- ✅ **No complex state**: Unlike multi-phase workflows (e.g., AIAnalysis with 4 conditions)

### Reference Implementation

NotificationRequest serves as the **reference implementation** for services with:
- ✅ Simple routing or approval decision workflows
- ✅ Binary outcomes (success/failure)
- ✅ Single decision point (not multi-phase)

**Other services that should follow this pattern**: RemediationApprovalRequest (RO Team)

---

## 📊 Conditions Inventory

| Condition Type | Reasons | Purpose | Controller Phase |
|----------------|---------|---------|------------------|
| `Ready` | 2 | Aggregate: True on success terminal, False on failure terminal | All |
| `RoutingResolved` | 3 | Indicates routing rule evaluation outcome | Routing Phase |

**Total**: 2 Condition Types, 5 Condition Reasons

---

## 🔍 Condition Specifications

### Condition: `Ready`

**Type Constant**: `ConditionTypeReady = "Ready"`

**Purpose**: Aggregate condition indicating readiness. True on success terminal phases (Sent, PartiallySent); False on failure terminal.

**When Set**: On phase transitions to terminal state.

---

### Condition: `RoutingResolved`

**Type Constant**: `ConditionTypeRoutingResolved = "RoutingResolved"`

**Purpose**: Indicates whether notification routing rules have been successfully evaluated and channels selected.

**Business Requirement**: BR-NOT-069 (Routing Rule Visibility via Conditions)

**When Set**: During the **Routing Phase** of reconciliation (after spec validation, before channel sending)

**Implementation Notes** (Issue #79):
- **Persistence fix**: `AtomicStatusUpdate`/`UpdatePhase` must pass `conditions` parameter so RoutingResolved is persisted when phase is updated.
- **Fallback reason fix**: When no routing rules match, use `ReasonRoutingFallback` (not `RoutingFailed`); fallback is a valid success state.

**Status Values**:
- ✅ **True**: Routing completed successfully (rule matched or fallback used)
- ❌ **False**: Routing failed (error condition)

---

#### Reason 1: `RoutingRuleMatched` ✅

**Constant**: `ReasonRoutingRuleMatched = "RoutingRuleMatched"`

**Status**: `True`

**When Set**: A routing rule successfully matched the notification's severity and environment

**Message Format**:
```
Matched rule '<RuleName>' (severity=<Severity>, env=<Env>) → channels: <Channel1>, <Channel2>, ...
```

**Example Messages**:
```
Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty
Matched rule 'dev-all' (severity=info, env=dev) → channels: console
Matched rule 'staging-high' (severity=high, env=staging) → channels: slack, email
```

**Controller Code Location**:
```go
// File: internal/controller/notification/routing.go
// Function: resolveRoutingChannels()

notification.SetRoutingResolved(notif, metav1.ConditionTrue,
    notification.ReasonRoutingRuleMatched,
    fmt.Sprintf("Matched rule '%s' (severity=%s, env=%s) → channels: %s",
        matchedRule.Name, notif.Spec.Severity, notif.Spec.Environment, channels))
```

**Testing**:
- ✅ Unit Tests: `test/unit/notification/conditions_test.go`
- ✅ Integration Tests: `test/integration/notification/routing_rules_test.go`
- ✅ E2E Tests: `test/e2e/notification/01_notification_lifecycle_audit_test.go`

**kubectl Output**:
```bash
$ kubectl describe notificationrequest prod-critical-alert

Status:
  Conditions:
    Last Transition Time:  2025-12-16T10:30:45Z
    Message:               Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty
    Observed Generation:   1
    Reason:                RoutingRuleMatched
    Status:                True
    Type:                  RoutingResolved
```

**Operational Impact**:
- ✅ **Debugging**: Operators immediately see which rule matched and what channels were selected
- ✅ **Audit Trail**: Routing decisions are recorded in Kubernetes history
- ✅ **Automation**: GitOps tools can wait for `RoutingResolved=True` before proceeding

---

#### Reason 2: `RoutingFallback` ✅

**Constant**: `ReasonRoutingFallback = "RoutingFallback"`

**Status**: `True`

**When Set**: No routing rules matched, so the notification falls back to console-only channel

**Message Format**:
```
No routing rules matched (severity=<Severity>, env=<Env>), using console fallback
```

**Example Messages**:
```
No routing rules matched (severity=info, env=dev), using console fallback
No routing rules matched (severity=medium, env=test), using console fallback
```

**Controller Code Location**:
```go
// File: internal/controller/notification/routing.go
// Function: resolveRoutingChannels()

notification.SetRoutingResolved(notif, metav1.ConditionTrue,
    notification.ReasonRoutingFallback,
    fmt.Sprintf("No routing rules matched (severity=%s, env=%s), using console fallback",
        notif.Spec.Severity, notif.Spec.Environment))
```

**Testing**:
- ✅ Unit Tests: `test/unit/notification/conditions_test.go`
- ✅ Integration Tests: `test/integration/notification/routing_rules_test.go` (fallback scenarios)
- ✅ E2E Tests: `test/e2e/notification/01_notification_lifecycle_audit_test.go`

**kubectl Output**:
```bash
$ kubectl describe notificationrequest low-priority-alert

Status:
  Conditions:
    Last Transition Time:  2025-12-16T10:31:20Z
    Message:               No routing rules matched (severity=low, env=dev), using console fallback
    Observed Generation:   1
    Reason:                RoutingFallback
    Status:                True
    Type:                  RoutingResolved
```

**Operational Impact**:
- ✅ **Visibility**: Operators understand why only console channel was used
- ✅ **Rule Tuning**: Helps identify gaps in routing rule coverage
- ✅ **Expected Behavior**: Fallback is a valid success state, not an error

**Note**: `RoutingFallback` is **not an error** - it's a valid success state when no rules match. The notification will still be sent via the console channel.

---

#### Reason 3: `RoutingFailed` ❌

**Constant**: `ReasonRoutingFailed = "RoutingFailed"`

**Status**: `False`

**When Set**: An error occurred during routing rule evaluation (e.g., invalid rule condition, system error)

**Message Format**:
```
Routing failed: <ErrorDetails>
```

**Example Messages**:
```
Routing failed: invalid rule condition 'severity >= invalid' in rule 'production-critical'
Routing failed: routing rule service temporarily unavailable
Routing failed: unable to parse environment label
```

**Controller Code Location**:
```go
// File: internal/controller/notification/routing.go
// Function: resolveRoutingChannels()

notification.SetRoutingResolved(notif, metav1.ConditionFalse,
    notification.ReasonRoutingFailed,
    fmt.Sprintf("Routing failed: %v", err))
```

**Testing**:
- ✅ Unit Tests: `test/unit/notification/conditions_test.go`
- ✅ Integration Tests: `test/integration/notification/routing_rules_test.go` (error scenarios)
- ✅ E2E Tests: `test/e2e/notification/04_failed_delivery_audit_test.go`

**kubectl Output**:
```bash
$ kubectl describe notificationrequest failed-routing

Status:
  Conditions:
    Last Transition Time:  2025-12-16T10:32:10Z
    Message:               Routing failed: invalid rule condition 'severity >= invalid' in rule 'production-critical'
    Observed Generation:   1
    Reason:                RoutingFailed
    Status:                False
    Type:                  RoutingResolved
  Phase:                   Failed
```

**Operational Impact**:
- ✅ **Immediate Alerting**: Operators know routing failed before checking logs
- ✅ **Error Details**: Condition message includes specific error information
- ✅ **Automation**: CI/CD pipelines can detect failures via `Status=False`

**Recovery**:
1. Review condition message for error details
2. Check routing rule definitions (`kubectl get routingrules`)
3. Validate rule conditions and labels
4. Update routing rules if needed
5. Notification will be reconciled automatically

---

## 🧪 Testing Requirements

### Unit Tests ✅ COMPLETE

**File**: `test/unit/notification/conditions_test.go`

**Coverage**: 100% of condition helper functions

**Test Structure**:
```go
var _ = Describe("Conditions", func() {
    Context("SetRoutingResolved", func() {
        It("should set condition to True on success", func() { /* ... */ })
        It("should set condition to False on failure", func() { /* ... */ })
        It("should preserve LastTransitionTime when status unchanged", func() { /* ... */ })
        It("should update LastTransitionTime when status changes", func() { /* ... */ })
        It("should set ObservedGeneration from object metadata", func() { /* ... */ })
    })

    Context("GetRoutingResolved", func() {
        It("should return condition when it exists", func() { /* ... */ })
        It("should return nil when condition does not exist", func() { /* ... */ })
    })

    Context("IsRoutingResolved", func() {
        It("should return true when condition is True", func() { /* ... */ })
        It("should return false when condition is False", func() { /* ... */ })
        It("should return false when condition does not exist", func() { /* ... */ })
    })
})
```

**Run Tests**:
```bash
cd test/unit/notification
ginkgo -v conditions_test.go
```

---

### Integration Tests ✅ COMPLETE

**File**: `test/integration/notification/routing_rules_test.go`

**Coverage**: Conditions populated during reconciliation for all routing scenarios

**Test Scenarios**:
1. ✅ **Rule Matched**: Condition set to `RoutingRuleMatched` when rule matches
2. ✅ **Fallback**: Condition set to `RoutingFallback` when no rules match
3. ✅ **Error**: Condition set to `RoutingFailed` on routing errors

**Example Test**:
```go
It("should set RoutingResolved condition when rule matches", func() {
    // Create NotificationRequest
    notif := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-routing-condition",
            Namespace: "default",
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Severity:    "critical",
            Environment: "production",
            Title:       "Test Notification",
            Description: "Testing routing condition",
        },
    }
    Expect(k8sClient.Create(ctx, notif)).To(Succeed())

    // Wait for condition to be set
    Eventually(func() bool {
        var updated notificationv1alpha1.NotificationRequest
        if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), &updated); err != nil {
            return false
        }
        cond := notification.GetRoutingResolved(&updated)
        return cond != nil && cond.Status == metav1.ConditionTrue
    }, timeout, interval).Should(BeTrue())

    // Verify condition details
    var final notificationv1alpha1.NotificationRequest
    Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(notif), &final)).To(Succeed())
    cond := notification.GetRoutingResolved(&final)
    Expect(cond.Reason).To(Equal(notification.ReasonRoutingRuleMatched))
    Expect(cond.Message).To(ContainSubstring("Matched rule"))
})
```

**Run Tests**:
```bash
cd test/integration/notification
ginkgo -v routing_rules_test.go
```

---

### E2E Tests ✅ COMPLETE

**Files**:
1. `test/e2e/notification/01_notification_lifecycle_audit_test.go` - Success scenarios
2. `test/e2e/notification/04_failed_delivery_audit_test.go` - Failure scenarios

**Coverage**: End-to-end condition population in real Kubernetes environment

**Validation**:
```go
// Verify condition is set and accessible via kubectl
Eventually(func() string {
    output, _ := kubectl.Describe("notificationrequest", notifName)
    return output
}, timeout, interval).Should(ContainSubstring("Type: RoutingResolved"))

// Verify kubectl wait works
Eventually(func() error {
    return kubectl.Wait("--for=condition=RoutingResolved",
        "notificationrequest/"+notifName, "--timeout=30s")
}, timeout, interval).Should(Succeed())
```

**Run Tests**:
```bash
cd test/e2e/notification
ginkgo -v 01_notification_lifecycle_audit_test.go
ginkgo -v 04_failed_delivery_audit_test.go
```

---

## 🔧 Implementation Details

### Code Structure

```
pkg/notification/
└── conditions.go (123 lines)
    ├── Condition Type Constants (lines 15-20)
    ├── Condition Reason Constants (lines 22-35)
    ├── SetRoutingResolved() (lines 40-75)
    ├── GetRoutingResolved() (lines 77-90)
    └── IsRoutingResolved() (lines 92-100)
```

### Helper Functions

#### `SetRoutingResolved()`

**Purpose**: Set the `RoutingResolved` condition with the specified status, reason, and message

**Signature**:
```go
func SetRoutingResolved(
    notif *v1alpha1.NotificationRequest,
    status metav1.ConditionStatus,
    reason string,
    message string,
)
```

**Behavior**:
1. ✅ **Idempotent**: Can be called multiple times with same status without side effects
2. ✅ **LastTransitionTime Preservation**: Only updates timestamp when status changes
3. ✅ **ObservedGeneration**: Automatically set from object metadata generation
4. ✅ **Meta Condition Management**: Uses `meta.SetStatusCondition()` for consistency

**Example Usage**:
```go
// Success: Rule matched
notification.SetRoutingResolved(notif, metav1.ConditionTrue,
    notification.ReasonRoutingRuleMatched,
    "Matched rule 'production-critical' → channels: slack, email, pagerduty")

// Success: Fallback
notification.SetRoutingResolved(notif, metav1.ConditionTrue,
    notification.ReasonRoutingFallback,
    "No routing rules matched, using console fallback")

// Failure: Error
notification.SetRoutingResolved(notif, metav1.ConditionFalse,
    notification.ReasonRoutingFailed,
    fmt.Sprintf("Routing failed: %v", err))
```

---

#### `GetRoutingResolved()`

**Purpose**: Retrieve the `RoutingResolved` condition from a NotificationRequest

**Signature**:
```go
func GetRoutingResolved(notif *v1alpha1.NotificationRequest) *metav1.Condition
```

**Returns**:
- ✅ `*metav1.Condition`: Pointer to condition if found
- ❌ `nil`: If condition does not exist

**Example Usage**:
```go
cond := notification.GetRoutingResolved(notif)
if cond != nil {
    fmt.Printf("Status: %s, Reason: %s, Message: %s\n",
        cond.Status, cond.Reason, cond.Message)
} else {
    fmt.Println("RoutingResolved condition not yet set")
}
```

---

#### `IsRoutingResolved()`

**Purpose**: Check if routing has been successfully resolved (condition exists with `Status=True`)

**Signature**:
```go
func IsRoutingResolved(notif *v1alpha1.NotificationRequest) bool
```

**Returns**:
- ✅ `true`: Condition exists and `Status=True` (rule matched or fallback)
- ❌ `false`: Condition does not exist or `Status=False` (routing failed)

**Example Usage**:
```go
if notification.IsRoutingResolved(notif) {
    // Proceed to channel sending phase
    sendToChannels(notif, selectedChannels)
} else {
    // Handle routing failure
    return reconcile.Result{Requeue: true}, err
}
```

---

### Controller Integration

**File**: `internal/controller/notification/routing.go`

**Integration Point**: During reconciliation, after spec validation and before channel sending

**Reconciliation Flow**:
```
1. Spec Validation
   ↓
2. Routing Phase ← **SetRoutingResolved() called here**
   ├── Evaluate routing rules
   ├── Match found? → SetRoutingResolved(True, RoutingRuleMatched, "Matched rule 'X' → channels: Y")
   ├── No match? → SetRoutingResolved(True, RoutingFallback, "No rules matched, using console")
   └── Error? → SetRoutingResolved(False, RoutingFailed, "Routing failed: error")
   ↓
3. Channel Sending Phase ← **IsRoutingResolved() guards this phase**
   ↓
4. Status Update (condition persisted to Kubernetes)
```

**Error Handling**:
- ✅ Routing errors set `RoutingResolved=False` with detailed error message
- ✅ Controller returns `RequeueAfter` to retry routing (temporary failures)
- ✅ Permanent failures (e.g., invalid rule syntax) set status to `Failed`

---

## 📚 Operator Guide

### Viewing Conditions

#### kubectl describe
```bash
# See all conditions for a NotificationRequest
kubectl describe notificationrequest my-notification

# Output includes:
# Status:
#   Conditions:
#     Type: RoutingResolved
#     Status: True
#     Reason: RoutingRuleMatched
#     Message: Matched rule 'production-critical' → channels: slack, email
#     Last Transition Time: 2025-12-16T10:30:45Z
```

#### kubectl get with JSONPath
```bash
# Get condition status
kubectl get notificationrequest my-notification \
  -o jsonpath='{.status.conditions[?(@.type=="RoutingResolved")].status}'

# Get condition reason
kubectl get notificationrequest my-notification \
  -o jsonpath='{.status.conditions[?(@.type=="RoutingResolved")].reason}'

# Get condition message
kubectl get notificationrequest my-notification \
  -o jsonpath='{.status.conditions[?(@.type=="RoutingResolved")].message}'
```

---

### Automation with kubectl wait

#### Wait for routing resolution
```bash
# Wait up to 30 seconds for routing to complete
kubectl wait --for=condition=RoutingResolved notificationrequest/my-notification --timeout=30s

# Exit code:
#   0 = Routing completed (True or False)
#   non-zero = Timeout (condition not set)
```

#### GitOps Integration

**ArgoCD Health Checks**:
```yaml
# Application health depends on RoutingResolved=True
apiVersion: argoproj.io/v1alpha1
kind: Application
spec:
  syncPolicy:
    automated:
      prune: true
    syncOptions:
      - CreateNamespace=true
  healthCheck:
    resources:
      - kind: NotificationRequest
        check: |
          hs = {}
          if obj.status ~= nil and obj.status.conditions ~= nil then
            for i, condition in ipairs(obj.status.conditions) do
              if condition.type == "RoutingResolved" and condition.status == "True" then
                hs.status = "Healthy"
                hs.message = condition.message
                return hs
              elseif condition.type == "RoutingResolved" and condition.status == "False" then
                hs.status = "Degraded"
                hs.message = condition.message
                return hs
              end
            end
          end
          hs.status = "Progressing"
          hs.message = "Routing not yet resolved"
          return hs
```

**Flux Kustomization**:
```bash
# Reconcile kustomization and wait for routing resolution
flux reconcile kustomization my-app --wait-for-condition=RoutingResolved
```

---

### Debugging Routing Issues

#### Scenario 1: Unexpected Fallback

**Symptom**: Notification uses console-only when you expected multi-channel

**Diagnosis**:
```bash
# Check condition details
kubectl describe notificationrequest my-notification | grep -A5 "Type: RoutingResolved"

# Output shows:
# Reason: RoutingFallback
# Message: No routing rules matched (severity=high, env=staging), using console fallback
```

**Resolution**:
1. Review routing rules:
   ```bash
   kubectl get routingrules -n default
   ```
2. Check rule conditions (severity, environment labels)
3. Verify notification labels match rule selectors
4. Update routing rules or notification spec as needed

---

#### Scenario 2: Routing Failed

**Symptom**: Notification stuck in `Failed` phase

**Diagnosis**:
```bash
# Check condition details
kubectl get notificationrequest my-notification \
  -o jsonpath='{.status.conditions[?(@.type=="RoutingResolved")]}'

# Output shows:
# {
#   "type": "RoutingResolved",
#   "status": "False",
#   "reason": "RoutingFailed",
#   "message": "Routing failed: invalid rule condition 'severity >= invalid'"
# }
```

**Resolution**:
1. Identify problematic routing rule from error message
2. Review and fix routing rule definition
3. Update routing rule:
   ```bash
   kubectl edit routingrule production-critical
   ```
4. Notification will be reconciled automatically

---

#### Scenario 3: Rule Not Matching as Expected

**Symptom**: Wrong routing rule matched

**Diagnosis**:
```bash
# Check which rule matched
kubectl get notificationrequest my-notification \
  -o jsonpath='{.status.conditions[?(@.type=="RoutingResolved")].message}'

# Output:
# Matched rule 'production-all' (severity=high, env=production) → channels: slack

# Expected rule 'production-critical' to match instead
```

**Resolution**:
1. Review routing rule priority (rules evaluated in order)
2. Check rule conditions and label selectors
3. Adjust rule ordering or conditions as needed
4. Test with new notification to verify fix

---

## 📊 Metrics and Observability

### Prometheus Metrics (Future Enhancement)

**Recommended Metrics** (V1.1+):
```prometheus
# Counter: Total routing resolutions by reason
notification_routing_resolved_total{reason="RoutingRuleMatched"} 1245
notification_routing_resolved_total{reason="RoutingFallback"} 67
notification_routing_resolved_total{reason="RoutingFailed"} 3

# Gauge: Current notifications with RoutingResolved=False
notification_routing_failed_current 2

# Histogram: Time to routing resolution
notification_routing_duration_seconds_bucket{le="0.1"} 890
notification_routing_duration_seconds_bucket{le="0.5"} 1203
notification_routing_duration_seconds_bucket{le="1.0"} 1245
```

**Grafana Dashboard** (V1.1+):
- Routing success rate (RoutingRuleMatched + RoutingFallback) vs. failures
- Fallback rate (indicates routing rule coverage gaps)
- Routing duration P50/P95/P99
- Failed routing alerts

---

## 🔗 Related Documents

### Authoritative References
- **Standard**: [DD-CRD-002: Kubernetes Conditions Standard](../../../architecture/decisions/DD-CRD-002-kubernetes-conditions-standard.md)
- **Inventory**: [KUBERNAUT_CONDITIONS_REFERENCE.md](../../../architecture/KUBERNAUT_CONDITIONS_REFERENCE.md)
- **Business Requirement**: [BR-NOT-069: Routing Rule Visibility via Conditions](../../../requirements/BR-NOT-069-routing-rule-visibility-conditions.md)

### Implementation Files
- **Infrastructure**: `pkg/notification/conditions.go` (123 lines)
- **Controller**: `internal/controller/notification/routing.go`
- **CRD Schema**: `api/notification/v1alpha1/notificationrequest_types.go`

### Testing Documentation
- **Unit Tests**: `test/unit/notification/conditions_test.go`
- **Integration Tests**: `test/integration/notification/routing_rules_test.go`
- **E2E Tests**:
  - `test/e2e/notification/01_notification_lifecycle_audit_test.go`
  - `test/e2e/notification/04_failed_delivery_audit_test.go`

### Service Documentation
- **Overview**: [overview.md](./overview.md)
- **Controller Implementation**: [controller-implementation.md](./controller-implementation.md)
- **CRD Schema**: [api-specification.md](./api-specification.md)
- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md)

---

## ✅ DD-CRD-002 Compliance Checklist

**Notification Service - V1.0 Compliance**:

- [x] ✅ `pkg/notification/conditions.go` exists with all required elements (123 lines)
- [x] ✅ All condition types map to business requirements (BR-NOT-069)
- [x] ✅ Unit tests in `test/unit/notification/conditions_test.go` (100% coverage)
- [x] ✅ Integration tests verify conditions are set during reconciliation
- [x] ✅ E2E tests validate conditions in real Kubernetes environment
- [x] ✅ Controller code calls condition setters during routing phase
- [x] ✅ `kubectl describe notificationrequest` shows populated Conditions section
- [x] ✅ `kubectl wait --for=condition=RoutingResolved` works correctly
- [x] ✅ Documentation updated with complete condition reference (this document)
- [x] ✅ Listed as "Minimal Pattern" reference implementation in DD-CRD-002

**Status**: ✅ **100% COMPLIANT - V1.0 READY**

---

## 📝 Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-12-16 | Initial comprehensive conditions reference | Notification Team (@jgil) |

---

**Document Maintained By**: Notification Team
**Last Review**: 2025-12-16
**Next Review**: V1.1 Planning Phase
**File**: `docs/services/crd-controllers/06-notification/DD-CRD-002-CONDITIONS-REFERENCE.md`




