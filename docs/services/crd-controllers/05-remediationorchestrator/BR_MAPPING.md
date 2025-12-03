# RemediationOrchestrator - Business Requirement Mapping

**Service**: RemediationOrchestrator Controller
**CRD**: RemediationRequest
**Version**: 1.0
**Last Updated**: December 2, 2025
**Status**: ✅ Complete

---

## 📋 Overview

This document maps RemediationOrchestrator business requirements (BR-ORCH-XXX) to implementation code and test coverage, per [CRD Service Specification Template](../../../development/templates/CRD_SERVICE_SPECIFICATION_TEMPLATE.md).

---

## 🎯 Active Business Requirements

### Category 1: Approval & Notification (V1.0)

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-001** | Approval Notification Creation | P0 | 🚧 Planned | [BR-ORCH-001](../../../requirements/BR-ORCH-001-approval-notification-creation.md) | `test/unit/remediationorchestrator/approval_notification_test.go` |

### Category 2: Workflow Data Pass-Through

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-025** | Workflow Data Pass-Through | P0 | 🚧 Planned | [BR-ORCH-025/026](../../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md) | `test/unit/remediationorchestrator/workflow_passthrough_test.go` |
| **BR-ORCH-026** | Approval Orchestration | P0 | 🚧 Planned | [BR-ORCH-025/026](../../../requirements/BR-ORCH-025-026-workflow-approval-orchestration.md) | `test/unit/remediationorchestrator/approval_orchestration_test.go` |

### Category 3: Timeout Management

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-027** | Global Remediation Timeout | P0 | 🚧 Planned | [BR-ORCH-027/028](../../../requirements/BR-ORCH-027-028-timeout-management.md) | `test/unit/remediationorchestrator/timeout_test.go` |
| **BR-ORCH-028** | Per-Phase Timeouts | P1 | 🚧 Planned | [BR-ORCH-027/028](../../../requirements/BR-ORCH-027-028-timeout-management.md) | `test/unit/remediationorchestrator/timeout_test.go` |

### Category 4: Notification Handling

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-029** | User-Initiated Notification Cancellation | P0 | 🚧 Planned | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | `test/unit/remediationorchestrator/notification_cancellation_test.go` |
| **BR-ORCH-030** | Notification Status Tracking | P1 | 🚧 Planned | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | `test/unit/remediationorchestrator/notification_tracking_test.go` |
| **BR-ORCH-031** | Cascade Cleanup for Child NotificationRequest | P1 | 🚧 Planned | [BR-ORCH-029/030/031](../../../requirements/BR-ORCH-029-031-notification-handling.md) | `test/unit/remediationorchestrator/cascade_cleanup_test.go` |

### Category 5: Resource Lock Deduplication (DD-RO-001)

| BR ID | Title | Priority | Status | BR File | Test File |
|-------|-------|----------|--------|---------|-----------|
| **BR-ORCH-032** | Handle WE Skipped Phase | P0 | 🚧 Planned | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `test/unit/remediationorchestrator/skipped_phase_test.go` |
| **BR-ORCH-033** | Track Duplicate Remediations | P1 | 🚧 Planned | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `test/unit/remediationorchestrator/duplicate_tracking_test.go` |
| **BR-ORCH-034** | Bulk Notification for Duplicates | P1 | 🚧 Planned | [BR-ORCH-032/033/034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) | `test/unit/remediationorchestrator/bulk_notification_test.go` |

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

## 📊 Test Coverage by BR

### Unit Tests (Target: 70%+)

```
test/unit/remediationorchestrator/
├── approval_notification_test.go     # BR-ORCH-001
├── workflow_passthrough_test.go      # BR-ORCH-025
├── approval_orchestration_test.go    # BR-ORCH-026
├── timeout_test.go                   # BR-ORCH-027, BR-ORCH-028
├── notification_cancellation_test.go # BR-ORCH-029
├── notification_tracking_test.go     # BR-ORCH-030
├── cascade_cleanup_test.go           # BR-ORCH-031
├── skipped_phase_test.go             # BR-ORCH-032
├── duplicate_tracking_test.go        # BR-ORCH-033
├── bulk_notification_test.go         # BR-ORCH-034
└── suite_test.go
```

### Integration Tests (Target: 20%)

```
test/integration/remediationorchestrator/
├── full_lifecycle_test.go            # BR-ORCH-025, BR-ORCH-027
├── approval_workflow_test.go         # BR-ORCH-001, BR-ORCH-026
├── resource_lock_test.go             # BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
└── suite_test.go
```

### E2E Tests (Target: 10%)

```
test/e2e/remediationorchestrator/
├── happy_path_test.go                # Full workflow
├── approval_required_test.go         # BR-ORCH-001, BR-ORCH-026
├── storm_deduplication_test.go       # BR-ORCH-032, BR-ORCH-033, BR-ORCH-034
└── suite_test.go
```

---

## 📋 BR Coverage Matrix

| Test Level | BRs Covered | Coverage Target |
|------------|-------------|-----------------|
| Unit | BR-ORCH-001, BR-ORCH-025-034 | 70%+ of each BR |
| Integration | BR-ORCH-001, BR-ORCH-025, BR-ORCH-026, BR-ORCH-027, BR-ORCH-032-034 | 20% (cross-component) |
| E2E | BR-ORCH-001, BR-ORCH-026, BR-ORCH-032-034 | 10% (critical paths) |

---

## 🔗 Related Documentation

- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) - Full BR definitions
- [testing-strategy.md](./testing-strategy.md) - Testing approach
- [implementation-checklist.md](./implementation-checklist.md) - APDC-TDD phases
- [DD-RO-001](../../architecture/decisions/DD-RO-001-resource-lock-deduplication-handling.md) - Resource lock deduplication

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-02 | Initial BR_MAPPING.md with all active BRs (BR-ORCH-001, BR-ORCH-025-034) |

---

**Document Version**: 1.0
**Last Updated**: December 2, 2025
**Maintained By**: Kubernaut Architecture Team


