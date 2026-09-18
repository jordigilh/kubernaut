# Test Plan: Fleet Boundary, Audit Attribution, and Remote Owner Resolution

**Test Plan Identifier**: TP-2417-v1
**Feature**: Close fleet fail-open, audit cluster-attribution, and remote owner-resolution gaps across issues #2419, #2422, #2426, #2430, plus the remaining remote WorkflowExecution E2E gap from issue #1504.
**Version**: 1.0
**Created**: 2026-09-17
**Author**: Kubernaut maintainers
**Status**: Active
**Branch**: `fix/2417-ka-fleet-overlay-all-flows`

---

## 1. Purpose and Success Criteria

These tests prove that fleet-targeted operations never silently use hub-local state,
that audit records retain authoritative remote-cluster provenance, and that remote
owner resolution fails closed when the gateway cannot provide a reader. The tests
must prove business outcomes, not merely that a helper was called.

Success requires:

1. A fleet investigation with no usable overlay is rejected before enrichment can
   query the hub cluster.
2. A NotificationRequest DELETE audit event contains the object cluster identity at
   the validator and persisted Data Storage boundaries.
3. Every target-associated AIAnalysis audit event retains `Spec.ClusterID` after
   persistence; hub-local events remain intentionally unset.
4. A remote Prometheus signal never falls back to the hub owner resolver when its
   remote reader cannot be created.
5. A real fleet WorkflowExecution collision exercises the remote `IsCompleted` GET
   through the MCP gateway and produces the expected business state.
6. The existing FMC real-sync journeys remain the E2E proof for the former
   `E2E-FLEET-014` gap; no duplicate FMC journey is added.

---

## 2. Authority and Control Objectives

### Business requirements

- `BR-INTEGRATION-054` and `BR-INTEGRATION-1489`: fleet MCP overlay is the
  cluster-scoped access boundary.
- `BR-FLEET-003` and `BR-FLEET-004`: cluster identity and registered-cluster
  dispatch are authoritative and cannot be replaced by local defaults.
- `BR-FLEET-054` / `ADR-068`: remote fleet routing uses registered MCP gateway
  clusters.
- `BR-AUDIT-005`: business-critical events remain reconstructable with structured
  cluster provenance and correlation identifiers.
- `BR-GATEWAY-069`: owner-based signal fingerprints remain cluster-aware.
- `BR-AUTH-001`: destructive NotificationRequest cancellation is attributed.

### FedRAMP controls

- `AC-3`: remote resource access is enforced at the MCP boundary.
- `AC-4`: information flow stays within the requested fleet cluster.
- `AC-6`: no fallback grants access to an unintended hub cluster.
- `AU-2` and `AU-3`: deletion and AIAnalysis audit events are emitted with
  structured actor, action, outcome, correlation, and cluster data.
- `SC-7`: remote access is constrained to the registered gateway boundary.
- `SI-10`: missing or invalid remote routing context fails closed.

### OWASP ASVS objectives

- `V4.1.1`: authorization is enforced by the trusted service boundary.
- `V4.1.3`: access remains limited to operator-provisioned clusters.
- `V4.1.5`: access-control exceptions fail securely rather than selecting a local
  substitute.
- `V5.1.x`: request and remote-response routing inputs are validated.
- `V7.1.1` and `V7.2.1`: security-relevant actions are observable without leaking
  credentials or workflow content.

---

## 3. Pyramid Strategy

The pyramid invariant is mandatory:

- **Unit tests prove logic**: empty-overlay validation, validator field mapping,
  audit event construction, and resolver error semantics.
- **Integration tests prove wiring**: production handler/controller paths persist
  the expected fields through the real service boundary or envtest reconciler.
- **E2E tests prove journeys**: fleet gateway routing and remote-cluster behavior
  are exercised against real services and Kubernetes clusters.

Every new or changed business requirement has at least UT + IT coverage. E2E is
required where the control objective depends on real gateway, controller, or
Kubernetes behavior. Tests use Ginkgo/Gomega and carry a BR or control reference.
No standard `testing.T`, `Skip`, `XIt`, or `PIt` is permitted.

---

## 4. Scope and Test Matrix

| Scope | Business outcome | Unit | Integration | E2E | Controls |
|---|---|---|---|---|---|
| #2419 | Fleet enrichment cannot fall back to hub state | `UT-KA-2419-001` | `IT-KA-2419-002` | Existing fleet journey regression | `AC-4`, `AC-6`, `SI-10`, ASVS `V4.1.5` |
| #2422 | DELETE audit retains NotificationRequest cluster provenance | `UT-AW-2422-001` | `IT-AW-2422-002` | Existing AuthWebhook journey regression | `AU-2`, `AU-3`, ASVS `V7.1.1`, `V7.2.1` |
| #2426 | All AIAnalysis target events persist authoritative cluster ID | `UT-AA-2426-001` | `IT-AA-2426-002` | Existing audit-flow regression | `AU-2`, `AU-3`, `AC-4`, ASVS `V7.1.1`, `V7.2.1` |
| #2430 | Remote owner resolution never falls back to hub | `UT-GW-2430-001` | `IT-GW-2430-002` | Existing fleet signal-ingestion regression | `AC-4`, `AC-6`, `SC-7`, ASVS `V4.1.1`, `V4.1.5` |
| #1504 gap | WE collision status is read from the remote cluster through MCP | Existing executor unit coverage | Existing WE wiring regression | `E2E-FLEET-012` | `AC-3`, `AC-4`, `AC-6`, ASVS `V4.1.1`, `V4.1.5` |
| #1504 FMC gap | Real FMC syncer writes fresh remote state to Valkey | Existing FMC syncer UTs | Existing FMC integration coverage | `E2E-FMC-054-010`, `E2E-FMC-EAIGW-054-010` | `AC-3`, `SC-7`, ASVS `V4.1.1`, `V4.1.5` |

The FMC E2E scenarios are already registered in both CI matrix entries and are
the authoritative replacement for the stale issue comment's `E2E-FLEET-014`
label. That label is already used by the crashloop cross-cluster scenario and
must not be reused.

---

## 5. Wiring Manifest

| Component | Production entry point | Wiring location | Integration/E2E proof |
|---|---|---|---|
| Empty fleet overlay rejection | `SelectWorkflowTool.Handle` pre-selection enrichment | `internal/kubernautagent/mcp/tools/select_workflow.go:runEnrichment` | `IT-KA-2419-002` |
| DELETE cluster attribution | AuthWebhook NotificationRequest DELETE validation | `pkg/authwebhook/notificationrequest_validator.go:ValidateDelete` | `IT-AW-2422-002` |
| AIAnalysis cluster attribution | AIAnalysis audit client used by handlers | `pkg/aianalysis/audit/audit.go:Record*` | `IT-AA-2426-002` |
| Remote owner resolution error | Prometheus webhook Parse/ParseBatch | `pkg/gateway/adapters/prometheus_adapter.go` and `cmd/gateway/main.go` | `IT-GW-2430-002` |
| Remote WE completion check | WorkflowExecution collision reconciliation | `internal/controller/workflowexecution/workflowexecution_collision.go` -> `JobExecutor.IsCompleted` | `E2E-FLEET-012` |

Checkpoint W passes only when every row has a production caller and its listed
integration or E2E proof passes.

---

## 6. RED, GREEN, REFACTOR

### RED

1. Add the empty-overlay business assertion and production-dispatch integration
   assertion before changing `runEnrichment`.
2. Extend the existing AuthWebhook validator and persistence assertions to require
   `cluster_id` before changing validator code.
3. Add a persistence-level AIAnalysis audit assertion; retain the existing unit
   matrix for all target-associated event methods.
4. Change existing resolver tests that currently expect remote-to-hub fallback into
   fail-closed tests.
5. Add the remote WorkflowExecution collision E2E assertion before changing any
   production code.

### GREEN

1. Reject a nil or empty fleet overlay with a contextual error.
2. Set `nr.Spec.ClusterID` on the validator's DELETE audit event.
3. Keep the existing `setAnalysisClusterID` production helper and prove its real
   persistence path; no speculative production change is allowed for #2426.
4. Return an error from `resolverForCluster` for a remote cluster when the reader
   factory is unavailable or cannot create a reader; propagate it through Parse and
   ParseBatch so the alert is not attributed to the hub.
5. Wire no new E2E-only seam: the remote WE test must use the production fleet
   deployment and MCP client path.

Run Checkpoint W immediately after GREEN.

### REFACTOR

- Centralize the remote resolver error message and preserve structured logging.
- Keep the validator and handler audit field ordering consistent.
- Remove or update comments that describe the old hub fallback.
- Do not introduce new components or compatibility shims.

---

## 7. Pass/Fail Criteria

PASS requires all P0 tests and all listed P1 tests to pass, the wiring manifest to
be complete, business-level assertions to cover every mapped control objective,
and no regression in affected package suites. The full required validation is:

```text
go build ./...
golangci-lint run --timeout=5m
make test
affected package UT/IT suites
E2E-FLEET-012 in CI
existing FMC Kuadrant and EAIGW E2E suites in CI
```

Failure of a control-objective assertion, a production wiring proof, or a
fail-closed boundary is a release-blocking failure even if line coverage passes.

---

## 8. Known Risks and Tier Decisions

| Risk | Mitigation |
|---|---|
| Remote WE collision setup is timing-sensitive | Use deterministic Job naming, a real remote Job, and `Eventually`; assert the business state, not a fixed reconcile count. |
| Envtest cannot prove MCP gateway routing | Keep envtest/IT for controller wiring and require `E2E-FLEET-012` for the real gateway journey. |
| Audit writes are buffered | Flush before querying Data Storage and assert exact event counts and `cluster_id`. |
| Existing tests encode unsafe fallback behavior | Update them to the new fail-closed contract and retain a hub-local regression test. |
| FMC scenario ID is stale/conflicting | Reference the existing `E2E-FMC-*` IDs; do not reuse `E2E-FLEET-014`. |

No tier is skipped. Existing FMC E2E coverage is reused rather than duplicated.
