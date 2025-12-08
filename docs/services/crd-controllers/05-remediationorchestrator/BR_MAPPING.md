# RemediationOrchestrator - Business Requirement Mapping

**Service**: RemediationOrchestrator Controller
**CRD**: RemediationRequest
**Version**: 1.1
**Last Updated**: December 8, 2025
**Status**: ✅ V1.0 Complete

---

## 📋 Overview

This document maps RemediationOrchestrator business requirements (BR-ORCH-XXX) to implementation code and test coverage, per [CRD Service Specification Template](../../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md).

---

## 🎯 Active Business Requirements

### Category 1: Approval & Notification (V1.0)

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-001** | Approval Notification Creation | P0 | ✅ Complete | [BR-ORCH-001](../../../requirements/BR-ORCH-001-approval-notification-creation.md) | `notification_creator_test.go` |

### Category 2: Workflow Data Pass-Through

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-025** | Workflow Data Pass-Through | P0 | ✅ Complete | [BR-ORCH-025/026](../../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md) | `workflowexecution_creator_test.go` |
| **BR-ORCH-026** | Approval Orchestration | P0 | ✅ Complete | [BR-ORCH-025/026](../../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md) | `approval_orchestration_test.go` |

### Category 3: Timeout Management

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-027** | Global Remediation Timeout | P0 | ✅ Complete | [BR-ORCH-027/028](../../../requirements/BR-ORCH-027-028-timeout-management.md) | `timeout_detector_test.go` |
| **BR-ORCH-028** | Per-Phase Timeouts | P1 | ✅ Complete | [BR-ORCH-027/028](../../../requirements/BR-ORCH-027-028-timeout-management.md) | `timeout_detector_test.go` |

### Category 4: Notification Handling

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-029** | User-Initiated Notification Cancellation | P1 | 📅 V1.1 | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | `notification_cancellation_test.go` |
| **BR-ORCH-030** | Notification Status Tracking | P2 | 📅 V1.1 | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | `notification_tracking_test.go` |
| **BR-ORCH-031** | Cascade Cleanup for Child NotificationRequest | P1 | ✅ Complete | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | All creator tests (owner refs) |

### Category 5: Resource Lock Deduplication (DD-RO-001)

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-032** | Handle WE Skipped Phase | P0 | ✅ Complete | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `workflowexecution_handler_test.go` |
| **BR-ORCH-033** | Track Duplicate Remediations | P1 | ✅ Complete | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `workflowexecution_handler_test.go` |
| **BR-ORCH-034** | Bulk Notification for Duplicates | P2 | 📅 V1.1 | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `bulk_notification_test.go` |

### Category 6: Manual Review & AIAnalysis Handling (V1.0 - Dec 2025)

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-035** | Notification Reference Tracking | P1 | ✅ Complete | [BR-ORCH-035](../../../requirements/BR-ORCH-035-notification-reference-tracking.md) | All creator tests (status updates) |
| **BR-ORCH-036** | Manual Review Notification | P0 | ✅ Complete | [BR-ORCH-036](../../../requirements/BR-ORCH-036-manual-review-notification.md) | `aianalysis_handler_test.go`, `notification_creator_test.go` |
| **BR-ORCH-037** | WorkflowNotNeeded Handling | P0 | ✅ Complete | [BR-ORCH-037](../../../requirements/BR-ORCH-037-workflow-not-needed.md) | `aianalysis_handler_test.go` |
| **BR-ORCH-038** | Preserve Gateway Deduplication | P1 | ✅ Complete | [BR-ORCH-038](../../../requirements/BR-ORCH-038-preserve-gateway-deduplication.md) | `workflowexecution_handler_test.go` |

---

## ⚠️ Deprecated/Superseded Business Requirements

The following BRs from `migration-current-state.md` are **implementation details** that have been superseded by the business-focused BR structure. They describe *how* the service is built, not *what business value* it provides.

| Old BR ID | Description | Status | Notes |
|-----------|-------------|--------|-------|
| BR-ORCH-015 | AIAnalysis CRD Creation | ⛔ SUPERSEDED | Implementation detail - covered by BR-ORCH-025/026 |
| BR-ORCH-016 | WorkflowExecution CRD Creation | ⛔ SUPERSEDED | Implementation detail - covered by BR-ORCH-025 |
| BR-ORCH-017 | Status Watching & Phase Progression | ⛔ SUPERSEDED | Core reconciler logic - not a BR |
| BR-ORCH-018 | CRD Watch Controller Setup | ⛔ SUPERSEDED | Infrastructure - not a BR |
| BR-ORCH-019 | RBAC Permissions | ⛔ SUPERSEDED | Infrastructure - covered by `security-configuration.md` |
| BR-ORCH-020 | Error Handling & Retries | ⛔ SUPERSEDED | Technical pattern - not a BR |
| BR-ORCH-021 | Monitoring & Metrics | ⛔ SUPERSEDED | Observability - covered by `metrics-slos.md` |

**Note**: The gap between BR-ORCH-001 and BR-ORCH-025 (BR-ORCH-002 to BR-ORCH-024) is reserved for future business requirements.

---

## 📅 V1.1 Deferred Requirements

The following requirements are deferred to V1.1:

| BR ID | Title | Priority | Reason for Deferral |
|-------|-------|----------|---------------------|
| **BR-ORCH-029** | User-Initiated Notification Cancellation | P1 | Requires Notification Service enhancements |
| **BR-ORCH-030** | Notification Status Tracking | P2 | Nice-to-have, not blocking V1.0 |
| **BR-ORCH-034** | Bulk Notification for Duplicates | P2 | Optimization, individual notifications work |

---

## 📊 Test Coverage by BR

### Unit Tests (Target: 70%+)

```
test/unit/remediationorchestrator/
├── aianalysis_creator_test.go        # BR-ORCH-025 (data pass-through)
├── aianalysis_handler_test.go        # BR-ORCH-036, BR-ORCH-037
├── approval_orchestration_test.go    # BR-ORCH-026
├── controller_test.go                # Core reconciler
├── notification_creator_test.go      # BR-ORCH-001, BR-ORCH-036
├── signalprocessing_creator_test.go  # BR-ORCH-025
├── timeout_detector_test.go          # BR-ORCH-027, BR-ORCH-028
├── workflowexecution_creator_test.go # BR-ORCH-025
├── workflowexecution_handler_test.go # BR-ORCH-032, BR-ORCH-033, BR-ORCH-038
└── suite_test.go
```

### Integration Tests (Target: 20%)

```
test/integration/remediationorchestrator/
├── full_lifecycle_test.go            # BR-ORCH-025, BR-ORCH-027
├── approval_workflow_test.go         # BR-ORCH-001, BR-ORCH-026
├── manual_review_test.go             # BR-ORCH-036, BR-ORCH-037
├── resource_lock_test.go             # BR-ORCH-032, BR-ORCH-033
└── suite_test.go
```

### E2E Tests (Target: 10%)

```
test/e2e/remediationorchestrator/
├── happy_path_test.go                # Full workflow
├── approval_required_test.go         # BR-ORCH-001, BR-ORCH-026
├── manual_review_test.go             # BR-ORCH-036
└── suite_test.go
```

---

## 📋 BR Coverage Matrix

| Test Level | BRs Covered | Coverage Target |
|------------|-------------|-----------------|
| Unit | BR-ORCH-001, BR-ORCH-025-028, BR-ORCH-031-033, BR-ORCH-035-038 | 70%+ of each BR |
| Integration | BR-ORCH-001, BR-ORCH-025-027, BR-ORCH-032-033, BR-ORCH-036-037 | 20% (cross-component) |
| E2E | BR-ORCH-001, BR-ORCH-026, BR-ORCH-036 | 10% (critical paths) |

---

## 🔗 Related Documentation

- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) - Full BR definitions
- [testing-strategy.md](./testing-strategy.md) - Testing approach
- [implementation-checklist.md](./implementation-checklist.md) - APDC-TDD phases
- [DD-RO-001](../../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md) - Resource lock deduplication
- [ADR-040](../../architecture/decisions/ADR-040-remediation-approval-request-architecture.md) - Approval Request CRD

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-12-08 | Added BR-ORCH-036/037/038 (Manual Review, WorkflowNotNeeded, Gateway Dedup). Updated status to ✅ Complete for V1.0 BRs. Marked BR-ORCH-029/030/034 as V1.1. |
| 1.0 | 2025-12-02 | Initial BR_MAPPING.md with all active BRs (BR-ORCH-001, BR-ORCH-025-034) |

---

**Document Version**: 1.1
**Last Updated**: December 8, 2025
**Maintained By**: Kubernaut Architecture Team


