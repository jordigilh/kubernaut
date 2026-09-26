# Test Plan: Fleet-Scoped Notification Cluster Attribution

> **Template Version**: 2.0 - Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2449-v1.0
**Feature**: Use `RemediationRequest.spec.clusterID` for notification attribution
**Version**: 1.0
**Created**: 2026-09-20
**Author**: Kubernaut Team
**Status**: Active
**Branch**: `fix/2448-console-provider-logout`

## 1. Purpose and Business Outcome

Issue #2449 fixes a fleet attribution defect where RemediationOrchestrator
notifications displayed the hub cluster's boot-time Kubernetes identity instead of
the cluster that originated the RemediationRequest. The authoritative value is
`RemediationRequest.spec.clusterID`, populated by the MCP Gateway. Empty
`clusterID` remains the local-mode signal and must not produce a fabricated cluster
line.

This protects operator investigation and remediation reconstruction by ensuring the
cluster shown in every notification is the cluster attached to the request.

## 2. Authority and Controls

- BR-FLEET-001: fleet remediation requires cluster identity on every RR (defined in ADR-065 and existing coverage plans)
- [BR-ORCH-001](../../requirements/BR-ORCH-001-approval-notification-creation.md): approval NotificationRequest creation
- [BR-ORCH-045](../../requirements/BR-ORCH-045-completion-notification.md): completion NotificationRequest creation
- [ADR-065](../../architecture/decisions/ADR-065-fleet-cluster-identity-on-rr.md): `ClusterID` is the sole supported cluster identifier
- Issue [#2449](https://github.com/jordigilh/kubernaut/issues/2449)
- FedRAMP `AU-3` and `SI-4`: complete, attributable monitoring records
- SOC 2 `CC7.2`: incident monitoring and reconstruction evidence
- OWASP ASVS `V7.1.1`/`V7.2.1`: security-relevant actions are observable without false attribution

## 3. Scope and Risks

### In Scope

- `NotificationCreator` body formatting for approval, completion, duplicate,
  manual-review, self-resolved, global-timeout, and phase-timeout notifications.
- Reconciler timeout wiring from `RemediationRequest.Spec.ClusterID`.
- Removal of RemediationOrchestrator boot-time cluster discovery and setter wiring.
- Envtest proof that an RR-created NotificationRequest contains the authoritative
  fleet cluster ID and omits the line in local mode.
- E2E proof through the production RO controller path.

### Out of Scope

- MCP Gateway extraction and RR population; already covered by ADR-065 Gateway
  tests and `IT-GW-FLEET-001/003`.
- Notification delivery channels; they render `NotificationRequest.spec.body` and
  are not changed here.
- Removal of unrelated `ClusterName` fields used by AgentSession or other services.

### Risks and Mitigations

| ID | Risk | Mitigation |
|---|---|---|
| R1 | A body path still uses process-level identity | Unit coverage for all body builders, call-graph verification, and remote RR assertion through envtest |
| R2 | Local notifications gain an incorrect cluster line | Unit and envtest local-mode assertions require the line to be absent |
| R3 | Timeout notifications lose cluster attribution | Dedicated timeout unit tests plus direct production call-site inspection |
| R4 | Removing boot discovery leaves an orphaned or required package | Search all production references before deleting the package; compile all RO entry points |

## 4. TDD and Pyramid Plan

### RED

- `UT-RO-FLEET-001..011`: format and body-builder contract for fleet and local RRs.
- `IT-NOT-2449-001`: envtest approval notification uses RR `spec.clusterID`.
- `IT-NOT-2449-002`: envtest local approval notification omits cluster content.
- `E2E-RO-2449-001`: production RO completion path omits cluster content for a local
  RR, proving the removed boot-time identity is not fabricated.

### GREEN

- Read cluster identity from `rr.Spec.ClusterID` in all notification builders.
- Pass `rr.Spec.ClusterID` to timeout body builders.
- Remove RO boot discovery and `SetClusterIdentity` wiring.

### REFACTOR

- Remove the now-unused RO-only boot discovery package and update stale historical
  documentation to identify the superseding behavior.
- Keep notification formatting centralized in `FormatClusterLine`.

### Wiring Manifest

| Component | Production entry point | Wiring location | Test |
|---|---|---|---|
| Fleet notification body attribution | RO notification creation | `pkg/remediationorchestrator/creator/notification.go` | `IT-NOT-2449-001` |
| Local notification omission | RO notification creation | `pkg/remediationorchestrator/creator/notification.go` | `UT-RO-FLEET-002/011`, `IT-NOT-2449-002` |
| Phase timeout attribution | RO phase timeout handler | `internal/controller/remediationorchestrator/notification_creation.go` | `UT-RO-FLEET-009`, `UT-RO-FLEET-010` |
| Global timeout attribution | RO global timeout handler | `internal/controller/remediationorchestrator/timeout_handling.go` | `UT-RO-FLEET-008`, `UT-RO-FLEET-010` |
| Boot discovery removal | RO startup | `cmd/remediationorchestrator/main.go` | RO build plus E2E-RO-2449-001 |

## 5. BR and Control Coverage Matrix

| Requirement/control | Business outcome | Tier | Test ID | Status |
|---|---|---|---|---|
| BR-FLEET-001 / AU-3 | Fleet notification identifies the RR's originating cluster | Unit | `UT-RO-FLEET-001`, `UT-RO-FLEET-003..010` | Pass |
| BR-FLEET-001 / AU-3, CC7.2 | Created NotificationRequest retains authoritative cluster provenance | Integration | `IT-NOT-2449-001` | Implemented |
| BR-FLEET-001 / AU-3, CC7.2 | Local notification does not invent cluster provenance | Integration | `IT-NOT-2449-002` | Implemented |
| BR-ORCH-045 / ASVS V7.1.1/V7.2.1 | Production completion notification remains attributable | E2E | `E2E-RO-2449-001` | Implemented; pending runtime rerun |
| BR-ORCH-001 / AU-3 | Approval notification remains correctly created | Existing integration | `IT-NOT-453B-003` plus #2449 assertions | Pass |

## 6. Pass Criteria

The change passes when:

1. All affected unit tests pass.
2. Both #2449 envtest scenarios pass through real Kubernetes API persistence.
3. The RO E2E production path omits cluster content for a local RR, not a hub boot identity.
4. `go build ./...` and RO package compilation pass.
5. No production caller remains for the removed RO boot-discovery package.
6. No notification body contains a cluster line for an empty RR `clusterID`.

## 7. Validation Commands

```bash
go test ./pkg/remediationorchestrator/...
go test ./internal/controller/remediationorchestrator -run '^$'
go test ./cmd/remediationorchestrator -run '^$'
go test ./test/integration/remediationorchestrator -ginkgo.focus='2449'
go build ./...

# Make-managed pyramid runs, using all host CPU cores
make test-integration-remediationorchestrator GINKGO_FOCUS=2449 TEST_PROCS=$(sysctl -n hw.ncpu)
make test-e2e-remediationorchestrator GINKGO_FOCUS=2449 TEST_PROCS=$(sysctl -n hw.ncpu)
```

The full envtest and E2E suites require their existing Kubernetes/infrastructure
prerequisites and are reported separately when unavailable.
