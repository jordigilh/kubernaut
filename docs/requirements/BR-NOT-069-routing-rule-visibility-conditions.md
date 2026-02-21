# BR-NOT-069: Routing Rule Visibility via Kubernetes Conditions

**Service**: Notification Controller
**Category**: Channel Routing (Observability)
**Priority**: P1 (HIGH)
**Version**: 1.0
**Date**: 2025-12-11
**Status**: ✅ Approved (Kubernaut V1.0 - December 2025)
**Related ADRs**: None
**Related BRs**: BR-NOT-065 (Channel Routing), BR-NOT-066 (Alertmanager Config), BR-NOT-067 (Hot-Reload)

---

## Overview

The Notification Service MUST expose routing rule resolution status via Kubernetes Conditions, enabling operators to debug spec-field-based channel routing without accessing controller logs.

**Business Value**: Reduces routing debugging time from 15-30 minutes (log analysis) to <1 minute (kubectl describe), improving operator efficiency and reducing MTTR for misconfigured routing rules.

---

## BR-NOT-069: Routing Rule Visibility via Kubernetes Conditions

### Description

When NotificationRequest CRDs are processed, the Notification Service resolves delivery channels using spec-field-based routing rules (BR-NOT-065). Operators need visibility into *which routing rule matched* and *which channels were selected* to debug routing misconfigurations without accessing controller logs.

### Priority

**P1 (HIGH)** - Quality of Life Enhancement for V1.1

### Rationale

Spec-field-based routing (BR-NOT-065) is complex with multiple matchers:
- **Type**: `escalation`, `approval_required`, `completed`, `failed`
- **Severity**: `critical`, `high`, `medium`, `low`
- **Environment**: `production`, `staging`, `development`, `test`
- **Namespace**: Kubernetes namespace name
- **Skip-Reason**: `PreviousExecutionFailed`, `ExhaustedRetries`, etc.

**Without Conditions**:
- Operators must access controller logs to debug routing
- Log analysis takes 15-30 minutes
- No visibility via `kubectl describe`
- Difficult to validate routing rules work correctly

**With Conditions**:
- Routing decision visible via `kubectl describe` (<1 minute)
- Matched rule name shown in Condition message
- Fallback scenarios clearly indicated
- Easy validation of routing rule behavior

### Implementation

#### Kubernetes Condition

**Type**: `RoutingResolved`

**Status Values**:
- `True`: Routing successfully resolved (rule matched or fallback used)
- `False`: Routing failed (should be rare - fallback to console)

**Reason Values**:
- `RoutingRuleMatched`: A routing rule matched successfully
- `RoutingFallback`: No rules matched, using console fallback
- `RoutingFailed`: Routing resolution failed (error state)

**Condition Example** (Rule Matched):
```yaml
status:
  phase: Sending
  conditions:
  - type: RoutingResolved
    status: "True"
    reason: RoutingRuleMatched
    message: "Matched rule 'production-critical' (severity=critical, env=production) → channels: slack, email, pagerduty"
    lastTransitionTime: "2025-12-11T10:30:00Z"
    observedGeneration: 1
```

**Condition Example** (Fallback):
```yaml
status:
  phase: Sending
  conditions:
  - type: RoutingResolved
    status: "True"
    reason: RoutingFallback
    message: "No routing rules matched (labels: type=simple, severity=low), using console fallback"
    lastTransitionTime: "2025-12-11T10:30:00Z"
    observedGeneration: 1
```

#### Technical Implementation

1. **Helper Functions** (`pkg/notification/conditions.go`):
   ```go
   // SetRoutingResolved sets the RoutingResolved condition
   func SetRoutingResolved(notif *NotificationRequest, status metav1.ConditionStatus, reason, message string)

   // GetRoutingResolved returns the RoutingResolved condition
   func GetRoutingResolved(notif *NotificationRequest) *metav1.Condition

   // IsRoutingResolved checks if routing was successfully resolved
   func IsRoutingResolved(notif *NotificationRequest) bool
   ```

2. **Controller Integration** (`internal/controller/notification/notificationrequest_controller.go`):
   - Set condition after `resolveChannelsFromRouting()` call
   - Include matched rule name in condition message
   - Update CRD status to persist condition

3. **Routing Resolver Enhancement** (`pkg/notification/routing/resolver.go`):
   - Modify `ResolveChannelsForNotification` to return matched rule name
   - New function: `resolveChannelsFromRoutingWithDetails()` returns `([]Channel, string)`

### Acceptance Criteria

1. ✅ **RoutingResolved condition set during reconciliation**:
   - Condition created after routing resolution
   - Condition updated on each reconciliation
   - Condition follows Kubernetes API conventions

2. ✅ **Condition message includes matched rule name**:
   - Rule name displayed: `"Matched rule 'production-critical' → ..."`
   - Resulting channels listed: `"... → channels: slack, email, pagerduty"`
   - Labels used in matching shown: `"(severity=critical, env=production)"`

3. ✅ **Condition reason distinguishes scenarios**:
   - `RoutingRuleMatched` when rule matches successfully
   - `RoutingFallback` when no rules match (console fallback)
   - `RoutingFailed` when routing resolution errors

4. ✅ **Condition visible via kubectl**:
   - `kubectl describe notificationrequest` shows condition
   - Condition formatted per Kubernetes conventions
   - `lastTransitionTime` accurate

5. ✅ **Condition follows Kubernetes API conventions**:
   - Type, Status, Reason, Message fields populated
   - `observedGeneration` tracks CRD generation
   - Immutable `lastTransitionTime` on status change

### Test Coverage

#### Unit Tests (`test/unit/notification/conditions_test.go`)

**Coverage**: 5 test scenarios
1. Set RoutingResolved condition successfully
2. Update existing RoutingResolved condition
3. IsRoutingResolved returns true when condition is True
4. IsRoutingResolved returns false when condition is False
5. Handle fallback scenario with appropriate reason

**Test Framework**: Ginkgo + Gomega

#### Integration Tests (`test/integration/notification/routing_conditions_test.go`)

**Coverage**: 4 test scenarios
1. Routing rule matches → RoutingResolved = True (RuleMatched)
2. No routing rules → RoutingResolved = True (Fallback)
3. Multiple labels matched → Condition shows matched rule name
4. Hot-reload config → Condition updates with new rule

**Test Framework**: Ginkgo + Gomega + Real routing config

#### E2E Tests (`test/e2e/notification/conditions_kubectl_test.go`)

**Coverage**: 2 test scenarios
1. kubectl describe shows RoutingResolved condition
2. Condition message includes rule name and channels

**Test Framework**: Ginkgo + Gomega + Kind cluster

### kubectl Output Example

**Before** (no condition):
```bash
$ kubectl describe notificationrequest escalation-rr-001
Name:         escalation-rr-001
Namespace:    kubernaut-system
Status:
  Phase:      Sent
  Delivery Attempts:
    Channel:  slack
    Status:   success
  # ❌ No visibility: Why was Slack selected? Need controller logs
```

**After** (with RoutingResolved):
```bash
$ kubectl describe notificationrequest escalation-rr-001
Name:         escalation-rr-001
Namespace:    kubernaut-system
Status:
  Phase:      Sent
  Conditions:
    Type:                RoutingResolved
    Status:              True
    Reason:              RoutingRuleMatched
    Message:             Matched rule 'production-critical' (severity=critical, env=production, type=escalation) → channels: slack, email, pagerduty
    Last Transition Time: 2025-12-11T10:30:00Z
    Observed Generation:  1
  Delivery Attempts:
    Channel:  slack
    Status:   success
  # ✅ Clear: Production-critical rule matched because of severity=critical + env=production
```

### Related Business Requirements

- **BR-NOT-065**: Channel Routing Based on Spec Fields
  - Provides the routing functionality that BR-NOT-069 makes visible

- **BR-NOT-066**: Alertmanager-Compatible Configuration Format
  - Defines routing rule format that BR-NOT-069 displays

- **BR-NOT-067**: Routing Configuration Hot-Reload
  - Routing rules can change dynamically, BR-NOT-069 shows current match

### Related Documentation

- **Implementation Plan**: [RESPONSE_NO_CONDITIONS_IMPLEMENTATION.md](../handoff/RESPONSE_NO_CONDITIONS_IMPLEMENTATION.md)
- **Original Request**: [REQUEST_NO_KUBERNETES_CONDITIONS_IMPLEMENTATION.md](../handoff/REQUEST_NO_KUBERNETES_CONDITIONS_IMPLEMENTATION.md)
- **AIAnalysis Pattern**: [AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md](../handoff/AIANALYSIS_CONDITIONS_IMPLEMENTATION_STATUS.md)

### Deferred Conditions

**ChannelReachable** (Per-channel circuit breaker state visibility):
- **Reason**: Already exposed via Prometheus metric `notification_channel_circuit_breaker_state`
- **Status**: ⏸️ Deferred pending production operator feedback
- **Re-evaluation**: Q2 2026 based on user requests for kubectl-only access

### Business Impact

**Metrics**:
- **Routing Debug Time**: 15-30 min (logs) → <1 min (kubectl)
- **Operator Efficiency**: 95% improvement in routing troubleshooting
- **MTTR Reduction**: ~25 min saved per routing misconfiguration

**User Benefits**:
1. **Faster Debugging**: kubectl describe vs log analysis
2. **Routing Validation**: Confirm rules work as expected
3. **Label Troubleshooting**: Understand which labels triggered routing
4. **Fallback Detection**: Know when console fallback was used

---

## Implementation Status

**Status**: ✅ Approved for V1.1 (Q1 2026)

**Estimated Effort**: 3 hours
- Infrastructure (conditions.go): 30 min
- Controller integration: 1 hour
- Testing (unit + integration): 1 hour
- Documentation: 30 min

**Confidence**: 90%

**Next Steps**:
1. Create `pkg/notification/conditions.go` (copy from AIAnalysis pattern)
2. Update controller to set RoutingResolved condition
3. Add `resolveChannelsFromRoutingWithDetails` helper
4. Create unit tests for condition helpers
5. Create integration tests for routing scenarios
6. Update service documentation

---

**Document Version**: 1.0
**Last Updated**: December 11, 2025
**Maintained By**: Kubernaut Architecture Team
**File**: `docs/requirements/BR-NOT-069-routing-rule-visibility-conditions.md`
