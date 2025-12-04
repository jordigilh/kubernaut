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

### Progress Update (2025-12-04)

The following areas have been corrected:
- ✅ **RO code**: `pkg/remediationorchestrator/creator/*.go` - All 4 creator files
- ✅ **RO specs**: `docs/services/crd-controllers/05-remediationorchestrator/` - All 8 spec files
- ✅ **Global BRs**: `docs/requirements/` - All 11 BR files

**Remaining**: ~275 files across other services (Gateway, SP, AI, WE, DataStorage, etc.) - to be addressed in separate sessions per service.

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
- [x] Acknowledged
- [x] Label domain correction completed
- [x] No `kubernaut.io/` references found
- Notes: **COMPLETED 2025-12-04**. Updated 14 label references in `crd_creator.go`, 1 in `k8s/client.go`, 8 in `kubernetes_event_adapter.go`, 6 error type URLs in `rfc7807.go`. All unit tests (279 specs) passing. All production code now uses `kubernaut.ai/` domain consistently.

### SignalProcessing Team
- [x] Acknowledged
- [x] Label domain correction completed
- [ ] No `kubernaut.io/` references found
- Notes: Label domain corrected in `pkg/signalprocessing/classifier/business.go` (3 occurrences) and `test/unit/signalprocessing/business_classifier_test.go` (2 occurrences). All tests pass. Changes: `kubernaut.io/owner` → `kubernaut.ai/owner`, `kubernaut.io/sla` → `kubernaut.ai/sla`. Committed: `d468aca7` (RED), `26247161` (GREEN).

### AIAnalysis Team
- [x] Acknowledged (2025-12-04)
- [x] Label domain correction completed
- [x] No `kubernaut.io/` references found
- Notes:
  - Fixed `aianalysis.kubernaut.io/retry-count` → `aianalysis.kubernaut.ai/retry-count` in `pkg/aianalysis/handlers/investigating.go`
  - Updated test in `test/unit/aianalysis/investigating_handler_test.go`
  - Extracted constant `RetryCountAnnotation` for maintainability
  - Fixed 28 documentation references in `docs/services/crd-controllers/02-aianalysis/`

### WorkflowExecution Team
- [x] Acknowledged (2025-12-04)
- [x] Label domain correction completed - **Already using `kubernaut.ai/`**
- [x] No `kubernaut.io/` references found
- Notes:
  - Searched `internal/controller/workflowexecution/`, `api/workflowexecution/`, `cmd/workflowexecution/`, `test/integration/workflowexecution/`, `test/fixtures/tekton/`, `docs/services/crd-controllers/03-workflowexecution/` - **zero** `kubernaut.io/` references
  - Finalizer correctly uses: `workflowexecution.kubernaut.ai/finalizer`
  - Labels correctly use: `kubernaut.ai/workflow-execution`, `kubernaut.ai/source-namespace`, `kubernaut.ai/workflow-id`
  - **No code changes required** - WE was implemented correctly from the start

### Notification Team
- [x] Acknowledged (December 4, 2025)
- [x] Label domain correction completed - **No `kubernaut.io/` references found** in Notification service code
- [x] BR-NOT-065 routing rules implementation **✅ FULLY COMPLETED** for V1.0
- Notes:
  - Searched `pkg/notification/`, `internal/controller/notification/`, `api/notification/` - zero `kubernaut.io/` references
  - CRD schema already has optional Recipients/Channels (lines 156-179 in notificationrequest_types.go)
  - **FULLY IMPLEMENTED** (December 4, 2025) - Following strict TDD:
    - `pkg/notification/routing/config.go` - Alertmanager-compatible config (BR-NOT-066) ✅
    - `pkg/notification/routing/labels.go` - Standard label constants (kubernaut.ai/) ✅
    - `pkg/notification/routing/resolver.go` - Channel resolution from labels (BR-NOT-065) ✅
    - `test/unit/notification/routing_config_test.go` - 28 config parsing tests ✅
    - `test/unit/notification/routing_integration_test.go` - 9 controller integration tests ✅
  - **Features**:
    - `ParseConfig()` - Alertmanager-compatible YAML parsing
    - `FindReceiver(labels)` - First-match ordered routing
    - `ResolveChannelsForNotification()` - Label-based channel resolution
    - `GetEffectiveChannels()` - Explicit spec.channels OR routing rules
    - `DefaultConfig()` - Console fallback when no routing configured
  - **Test Results**: 177 tests passing (168 original + 9 new)

---

## 📚 Related Documents

- [BR-NOT-065: Channel Routing Based on Labels](../services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md)
- [BR-NOT-066: Alertmanager-Compatible Configuration Format](../services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md)
- [ADR-017: NotificationRequest CRD Creator Responsibility](../architecture/decisions/ADR-017-notification-crd-creator.md)

---

**Document Version**: 1.3
**Last Updated**: 2025-12-04
**Status**: ✅ ALL TEAMS ACKNOWLEDGED - COMPLETE
**Changelog**:
- V1.3: WorkflowExecution acknowledged - no changes needed (already using `kubernaut.ai/`)
- V1.2: Fixed `kubernaut.io/` → `kubernaut.ai/` in `docs/requirements/` (11 files)
- V1.1: Gateway, Notification, SignalProcessing acknowledged
- V1.0: Initial notice

