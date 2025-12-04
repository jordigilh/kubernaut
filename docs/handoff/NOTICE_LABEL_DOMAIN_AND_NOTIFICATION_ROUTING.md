# NOTICE: Label Domain Correction & Notification Routing Changes

**Date**: 2025-12-04
**From**: Remediation Orchestrator Team
**To**: All Service Teams (Gateway, SignalProcessing, AIAnalysis, WorkflowExecution, Notification)
**Priority**: HIGH
**Status**: REQUIRES ACKNOWLEDGMENT

---

## 📋 Summary

Two changes have been implemented that may affect your service:

1. **Label Domain Correction**: `kubernaut.io/` → `kubernaut.ai/`
2. **NotificationRequest Routing**: BR-NOT-065 moved from V1.1+ to V1.0

---

## 🔄 Change 1: Label Domain Correction

### What Changed

All Kubernetes labels using the `kubernaut.io/` domain have been corrected to use `kubernaut.ai/`:

| Before | After |
|--------|-------|
| `kubernaut.io/remediation-request` | `kubernaut.ai/remediation-request` |
| `kubernaut.io/component` | `kubernaut.ai/component` |
| `kubernaut.io/notification-type` | `kubernaut.ai/notification-type` |
| `kubernaut.io/severity` | `kubernaut.ai/severity` |
| `kubernaut.io/environment` | `kubernaut.ai/environment` |
| `kubernaut.io/priority` | `kubernaut.ai/priority` |

### Why

The project standard domain is `kubernaut.ai`, consistent with:
- API groups: `signalprocessing.kubernaut.ai/v1alpha1`
- Existing labels: `kubernaut.ai/workflow-execution`
- Finalizers: `workflowexecution.kubernaut.ai/finalizer`

### Impact Assessment

| Service | Impact | Action Required |
|---------|--------|-----------------|
| **Gateway** | LOW | Check if any label selectors use `kubernaut.io/` |
| **SignalProcessing** | LOW | Check if any label selectors use `kubernaut.io/` |
| **AIAnalysis** | LOW | Check if any label selectors use `kubernaut.io/` |
| **WorkflowExecution** | NONE | Already uses `kubernaut.ai/` |
| **Notification** | MEDIUM | Routing rules must use `kubernaut.ai/` labels |

### Action Required

**Please search your codebase for `kubernaut.io/` and correct to `kubernaut.ai/`:**

```bash
grep -r "kubernaut\.io/" pkg/ internal/ --include="*.go"
grep -r "kubernaut\.io/" docs/ --include="*.md"
```

---

## 🔔 Change 2: NotificationRequest Routing (BR-NOT-065)

### What Changed

**BR-NOT-065 (Channel Routing Based on Labels)** has been moved from V1.1+ to **V1.0**.

This means:
- `NotificationRequest.Spec.Recipients` is now **OPTIONAL**
- `NotificationRequest.Spec.Channels` is now **OPTIONAL**
- Notification Service routing rules determine recipients/channels based on labels

### CRD Schema Update

```go
// Before (V1.0 - OLD)
// +kubebuilder:validation:Required
// +kubebuilder:validation:MinItems=1
Recipients []Recipient `json:"recipients"`

// After (V1.0 - NEW)
// +optional
Recipients []Recipient `json:"recipients,omitempty"`
```

### How RO Creates NotificationRequests Now

```yaml
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: nr-approval-rr-12345
  labels:
    # Routing labels for BR-NOT-065
    kubernaut.ai/notification-type: approval_required
    kubernaut.ai/severity: critical
    kubernaut.ai/environment: production
    kubernaut.ai/priority: P1
spec:
  type: escalation
  priority: critical  # Enum: critical|high|medium|low
  subject: "🔔 Approval Required: Remediation for pod-crash-loop"
  body: "..."
  metadata:
    remediationRequestName: rr-12345
    originalPriority: P1  # Preserves original free-text priority
  # Recipients and Channels NOT specified - determined by routing rules
```

### Notification Service Routing Configuration

The Notification Service must implement routing rules (Alertmanager-compatible format per BR-NOT-066):

```yaml
route:
  group_by: ['kubernaut.ai/environment', 'kubernaut.ai/severity']
  routes:
    - match:
        kubernaut.ai/notification-type: approval_required
        kubernaut.ai/severity: critical
      receiver: pagerduty-oncall
    - match:
        kubernaut.ai/notification-type: completed
      receiver: slack-ops
    - match:
        kubernaut.ai/notification-type: failed
      receiver: pagerduty-oncall

receivers:
  - name: pagerduty-oncall
    pagerduty_configs:
      - service_key: ${PAGERDUTY_KEY}
  - name: slack-ops
    slack_configs:
      - channel: '#kubernaut-alerts'
```

### Impact Assessment

| Service | Impact | Action Required |
|---------|--------|-----------------|
| **Notification** | HIGH | Implement BR-NOT-065 routing rules in V1.0 |
| **RO** | DONE | Already updated to use routing labels |
| **Others** | NONE | No direct impact |

---

## ✅ Acknowledgment Required

Please acknowledge this notice by adding your team's response below:

### Gateway Team
- [x] Acknowledged (December 4, 2025)
- [x] Label domain correction completed
- [x] No `kubernaut.io/` label references remain in Gateway code
- Notes:
  - **Change 1 (Label Domain Correction)**: ✅ COMPLETED
    - `pkg/gateway/processing/crd_creator.go`: Updated 14 label references (`signal-type`, `signal-fingerprint`, `severity`, `environment`, `priority`, `created-at`, `storm`, `origin-namespace`, `cluster-scoped`)
    - Unit tests: Updated 4 test files to expect `kubernaut.ai/` labels
    - Integration tests: Updated 4 test files to use `kubernaut.ai/` labels
    - Documentation: Updated `overview.md`, `integration-points.md`
    - Verification: 333 unit tests pass ✅, 144/145 integration tests pass (1 pre-existing flaky test)
  - **Change 2 (BR-NOT-065 Routing)**: No impact on Gateway
    - Gateway creates `RemediationRequest` CRDs, not `NotificationRequest` CRDs
    - RO is responsible for creating `NotificationRequest` CRDs with routing labels
    - Gateway correctly populates labels (`kubernaut.ai/severity`, `kubernaut.ai/environment`, `kubernaut.ai/priority`) that downstream services can use

### SignalProcessing Team
- [x] Acknowledged
- [x] Label domain correction completed
- [x] No `kubernaut.io/` label references remain in SignalProcessing code
- Notes:
  - **Change 1 (Label Domain Correction)**: ✅ COMPLETED
    - `pkg/signalprocessing/classifier/business.go`: Updated 3 label references (`kubernaut.ai/owner`, `kubernaut.ai/sla`)
    - `test/unit/signalprocessing/business_classifier_test.go`: Updated 2 test expectations
    - Verification: All 115 unit tests pass ✅
    - Commits: `d468aca7` (RED), `26247161` (GREEN), `498e8904` (REFACTOR)
  - **Change 2 (BR-NOT-065 Routing)**: No impact on SignalProcessing
    - SignalProcessing enriches/classifies signals, does not create NotificationRequest CRDs
    - RO is responsible for NotificationRequest creation with routing labels

### AIAnalysis Team
- [ ] Acknowledged
- [ ] Label domain correction completed
- [ ] No `kubernaut.io/` references found
- Notes: _____

### WorkflowExecution Team
- [ ] Acknowledged
- [ ] Label domain correction completed
- [ ] No `kubernaut.io/` references found
- Notes: _____

### Notification Team
- [x] Acknowledged (December 4, 2025)
- [x] Label domain correction completed - **No `kubernaut.io/` references found** in Notification service code
- [x] BR-NOT-065 routing rules implementation planned for V1.0
- Notes:
  - Searched `pkg/notification/`, `internal/controller/notification/`, `api/notification/` - zero `kubernaut.io/` references
  - CRD schema already updated with optional Recipients/Channels (lines 156-179 in notificationrequest_types.go)
  - Will implement BR-NOT-065 (Label-based routing) and BR-NOT-066 (Alertmanager-compatible config) following TDD
  - Implementation scope: New `pkg/notification/routing/` package with Alertmanager config parsing

---

## 📚 Related Documents

- [BR-NOT-065: Channel Routing Based on Labels](../services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md)
- [BR-NOT-066: Alertmanager-Compatible Configuration Format](../services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md)
- [ADR-017: NotificationRequest CRD Creator Responsibility](../architecture/decisions/ADR-017-notification-crd-creator.md)

---

**Document Version**: 1.0
**Last Updated**: 2025-12-04
**Status**: PENDING ACKNOWLEDGMENTS

