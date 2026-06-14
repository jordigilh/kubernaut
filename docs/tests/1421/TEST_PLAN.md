# Test Plan: #1421 Cascade Terminal Phase from RR to Child CRDs

**Version**: 1.0
**Date**: 2026-06-13
**Issue**: [#1421](https://github.com/jordigilh/kubernaut/issues/1421)
**Design**: Kubernetes-native parent-manages-children cascade (inline in plan)
**Business Requirements**: BR-ORCH-1421

## FedRAMP Control Mapping

| Control | Requirement | Verified By |
|---------|-------------|-------------|
| IR-4 (Incident Handling) | Cancelled remediation MUST terminate all downstream processing (SP, AI, WE) promptly | UT-RO-1421-001, UT-RO-1421-004, UT-RO-1421-006, IT-RO-1421-001 |
| IR-4(1) (Automated Response) | Automated cascade terminates active investigation sessions when parent is cancelled | UT-AA-1421-001, IT-RO-1421-001 |
| AC-6 (Least Privilege) | Active child CRDs holding elevated cluster access (KA sessions, PipelineRuns) must be revoked on cancellation | UT-RO-1421-005, UT-AA-1421-002, IT-RO-1421-003 |
| SI-4 (Information System Monitoring) | All state transitions in remediation chain must be observable; no orphaned resources creating monitoring blind spots | UT-RO-1421-003, UT-AA-1421-003 |
| CM-3 (Configuration Change Control) | Cascade is idempotent — repeated reconciles do not corrupt already-terminal children | UT-RO-1421-002, IT-RO-1421-002 |
| AU-12 (Audit Generation) | Status patches produce observable K8s events auditable via standard watch mechanisms | IT-RO-1421-001, IT-RO-1421-003 |

## Test Scenarios

### Unit Tests — RO Cascade Logic

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-RO-1421-001 | AI patched to Failed when RR is Cancelled and AI is Investigating | RR(Cancelled) + AI(Investigating) | AI.Phase=Failed, AI.Reason=ParentCancelled, AI.CompletedAt set | IR-4(1) |
| UT-RO-1421-002 | Already-terminal children are skipped (idempotent) | RR(Cancelled) + AI(Completed) | AI.Phase unchanged (Completed), Reason unchanged | CM-3 |
| UT-RO-1421-003 | Missing child refs handled gracefully (no panic) | RR(Cancelled) + refs to nonexistent CRDs | Reconcile succeeds, no error | SI-4 |
| UT-RO-1421-004 | SP patched to Failed when RR is Cancelled | RR(Cancelled) + SP(Enriching) | SP.Phase=Failed | IR-4(1) |
| UT-RO-1421-005 | WE patched to Failed when RR is Cancelled | RR(Cancelled) + WE(Running) | WE.Phase=Failed, WE.FailureReason contains "terminal phase" | AC-6 |
| UT-RO-1421-006 | All three child types cascaded simultaneously | RR(Cancelled) + SP + AI + WE (all non-terminal) | All three children Failed | IR-4 |

### Unit Tests — AA Controller IS Cleanup

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-AA-1421-001 | SetTerminalPhase(Cancelled) called when AA is Failed/ParentCancelled | AA.Phase=Failed, AA.Reason=ParentCancelled | ISPhaseUpdater.SetTerminalPhase called with (rrName, Cancelled) | IR-4(1) |
| UT-AA-1421-002 | Normal failures do NOT trigger IS cascade | AA.Phase=Failed, AA.Reason=TransientError | ISPhaseUpdater.SetTerminalPhase NOT called | AC-6 |
| UT-AA-1421-003 | Nil ISPhaseUpdater handled gracefully | AA.Phase=Failed, AA.Reason=ParentCancelled, ISPhaseUpdater=nil | No panic, reconcile succeeds | SI-4 |

### Integration Tests — RO Cascade via envtest

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-RO-1421-001 | RR cancelled → AI+SP transition to Failed | Create RR + AI(Investigating) + SP(Enriching), patch RR to Cancelled | AI.Phase=Failed, AI.Reason=ParentCancelled; SP.Phase=Failed | IR-4(1), AU-12 |
| IT-RO-1421-002 | Already-terminal AI not overwritten | Create RR + AI(Completed), patch RR to Cancelled | AI.Phase remains Completed, Reason unchanged (Consistently) | CM-3 |
| IT-RO-1421-003 | WE transitions to Failed | Create RR + WE(Running), patch RR to Cancelled | WE.Phase=Failed, WE.FailureReason set | AC-6, AU-12 |

### Integration Tests — AA IS Cascade via envtest

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-AA-1421-001 | AA externally patched to Failed/ParentCancelled → IS transitions to Cancelled | Create AI + IS(Active), patch AI to Failed/ParentCancelled | IS.Phase=Cancelled | IR-4(1) |

> **Note**: IT-AA-1421-001 requires the full AA integration suite (envtest + controller manager).
> It exercises the production dispatch path: `Reconcile → terminal branch → cascadeCancelToIS → K8sISPhaseUpdater.SetTerminalPhase → IS status update`.

## Acceptance Criteria

1. All UT-RO-1421 tests pass (cascade logic correct for all child types)
2. All UT-AA-1421 tests pass (IS cleanup triggered only for ParentCancelled)
3. All IT-RO-1421 tests pass (envtest proves real K8s API interactions)
4. `ParentCancelled` reason added to AIAnalysisReason enum + kubebuilder validation
5. Cascade is non-fatal: errors logged but reconcile continues (operator resilience)
6. Cascade is idempotent: repeated reconciles produce same final state (CM-3)
7. Full build passes (`go build ./...`)
8. No regressions in existing RO (367 specs) and AA (430 specs) unit test suites

## Architecture

```
Console → RR(Cancelled) → RO Reconcile
                            ├── cascadeToAIAnalysis → AI(Failed/ParentCancelled)
                            │                          └── AA Reconcile → cascadeCancelToIS → IS(Cancelled)
                            │                                                                  └── AF/KA detects → session teardown
                            ├── cascadeToSignalProcessing → SP(Failed)
                            └── cascadeToWorkflowExecution → WE(Failed)
```

**Kubernetes-native pattern**: Parent controller (RO) manages child lifecycle.
Children do NOT watch the parent's phase — parent pushes terminal state down.
This mirrors Deployment→ReplicaSet→Pod cascade behavior.
