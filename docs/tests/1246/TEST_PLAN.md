# Test Plan: Issue #1246 — AW Graceful Degradation for RW Re-Registration

## 1. Test Plan Identifier

**TP-AW-1246** | Version 1.0 | 2026-05-23

## 2. References

- **Issue**: [#1246](https://github.com/jordigilh/kubernaut/issues/1246)
- **BR**: BR-WORKFLOW-006 (Kubernetes-native workflow schema), BR-PLATFORM-012 (Graceful degradation)
- **ADR**: ADR-058 (CRD-based workflow registration)
- **Design**: Issue #548 (StartupReconciler PVC-wipe resilience)

## 3. Introduction

This test plan covers the graceful degradation behavior of AuthWebhook's `StartupReconciler` when individual RemediationWorkflow (RW) CRDs fail to re-register with DataStorage at startup. The feature ensures AW starts successfully even when some RWs have unresolvable dependencies.

## 4. Test Items

| Component | File | Description |
|-----------|------|-------------|
| CRD Types | `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` | Conditions field addition |
| DS Client | `pkg/authwebhook/ds_client.go` | Error classification (IsPermanentError) |
| StartupReconciler | `pkg/authwebhook/startup_reconciler.go` | Graceful degradation loop |
| Main wiring | `cmd/authwebhook/main.go` | EventRecorder + AuditStore injection |
| RW Webhook | `pkg/authwebhook/remediationworkflow_handler.go` | Ready=True condition on success |

## 5. Features to Be Tested

| ID | Feature | Acceptance Criteria |
|----|---------|---------------------|
| F-1 | Graceful continuation | AW starts even when 1+ RWs fail registration |
| F-2 | Status marking | Failed RWs get `CatalogStatus=Disabled` + condition `Ready=False` |
| F-3 | Error classification | Permanent errors (400, 404) skip retry; transient (5xx, network) retry |
| F-4 | K8s Event emission | Warning event emitted on RW CR for failures |
| F-5 | Audit event | Audit trail for each failed re-registration |
| F-6 | Structured logging | Error-level log with RW name, reason, remediation |
| F-7 | Self-healing | Re-applying RW CR triggers re-registration via webhook |
| F-8 | Conditions on success | Successful registration sets `Ready=True` |

## 6. Features Not Tested

- DS side validation logic (tested in DS service)
- TLS/auth transport (tested in shared/tls)
- Controller-runtime manager lifecycle (framework concern)

## 7. Approach

### 7.1 Test Pyramid (per Pyramid Invariant)

| Tier | Coverage Target | Scope |
|------|-----------------|-------|
| Unit (Ginkgo/Gomega) | >=80% | StartupReconciler logic, error classification, condition helpers |
| Integration (envtest) | >=80% | Full reconciler with fake DS, real K8s client, status patches |
| E2E | Deferred | Full Kind cluster (covered by existing E2E infrastructure) |

### 7.2 Strategy

- **TDD Red**: Write failing tests for all 8 features first
- **TDD Green**: Implement minimal code to pass each test
- **TDD Refactor**: Improve structure, validate against 100 Go Mistakes

## 8. Test Scenarios

### Unit Tests

| ID | Scenario | Expected |
|----|----------|----------|
| UT-AW-1246-001 | One RW fails with 400, others succeed | Start() returns nil, failed RW has Disabled+Ready=False |
| UT-AW-1246-002 | All RWs fail with permanent errors | Start() returns nil, all marked Disabled |
| UT-AW-1246-003 | IsPermanentError returns true for 400/404 | Helper correctly classifies |
| UT-AW-1246-004 | IsPermanentError returns false for network/5xx | Helper correctly classifies |
| UT-AW-1246-005 | Transient error retries then succeeds | RW eventually registered Active |
| UT-AW-1246-006 | K8s event emitted for failed RW | EventRecorder.Eventf called with Warning |
| UT-AW-1246-009 | Successful RW gets Ready=True condition | Condition set with correct reason |
| UT-AW-1246-010 | Audit event emitted on permanent failure | AuditStore.StoreAudit called with category=workflow, type=authwebhook.workflow.registration_failed |
| UT-AW-1246-011 | Nil AuditStore graceful handling | No panic when AuditStore is nil |
| UT-AW-1246-012 | Context cancellation marks workflow Disabled | Disabled status + audit emitted |
| UT-AW-1246-013 | Deadline exhaustion with audit emission | Disabled status + audit + K8s event |

### Integration Tests

| ID | Scenario | Expected |
|----|----------|----------|
| IT-AW-1246-001 | Mixed success/failure with fake DS returning 400 | Pod starts, statuses correct |
| IT-AW-1246-002 | DS unreachable then available (transient) | Retry succeeds, Active status |

## 9. Pass/Fail Criteria

- All unit tests pass with `go test -race`
- No lint errors (`golangci-lint run`)
- Coverage >=80% of new code per tier
- Build succeeds (`go build ./...`)

## 10. Environmental Needs

- Go 1.24+
- controller-runtime v0.23.3 (fake client)
- Ginkgo v2 / Gomega

## 11. Risks and Contingencies

| Risk | Mitigation |
|------|------------|
| Audit event type not in OpenAPI spec | Use escape hatch (empty EventData.Type) |
| Status patch race with webhook | Use RetryOnConflict in startup reconciler |
| Fake client doesn't support status subresource | WithStatusSubresource() already used |
