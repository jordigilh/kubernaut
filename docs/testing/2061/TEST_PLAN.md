# Test Plan: MCP Interactive-Session CRD-Fallback Drops ResourceAPIVersion

**Issue**: [#2061](https://github.com/jordigilh/kubernaut/issues/2061) (`v1.5.6`, `release/v1.5`)
**Clone**: [#2063](https://github.com/jordigilh/kubernaut/issues/2063) (`v1.6`, `main`)
**Related** (same root class, different layer): [#2064](https://github.com/jordigilh/kubernaut/issues/2064) (Track 2), [#2066](https://github.com/jordigilh/kubernaut/issues/2066) (Track 3)
**Branch**: `fix/2061-2064-ka-apiversion-propagation` (off `origin/release/v1.5`)
**Created**: 2026-08-10
**Status**: Implementation complete — build/vet/targeted UT+IT suites green

---

## 1. Purpose

`SessionSignalContextResolver.ResolveSignalContext` resolves the `SignalContext` KA's MCP
workflow-discovery tools (`list_available_actions`, `list_workflows`, `get_workflow`) filter
against. When an interactive session's in-memory signal provider has no entry for the
`RemediationRequest` (RR) ID — e.g. process restart, session eviction, or an autonomous
investigation resuming interactively — the resolver falls back to reading the RR CRD directly and
reconstructing a `SignalContext` from `Spec.TargetResource`. That fallback construction explicitly
omitted `TargetResource.APIVersion`, so `SignalContext.ResourceAPIVersion` was always empty on this
path, even though the CRD it was read from already carried a correct, validated `apiVersion`.

### Root Cause

`internal/kubernautagent/mcp/adapters/signal_resolver.go`'s CRD-fallback struct literal mapped
`Kind`/`Name`/`Namespace` from `rr.Spec.TargetResource` but had no line for `APIVersion` — a simple
field-mapping omission, not a design gap. `ComponentGVK()` (`pkg/kubernautagent/types/types.go`)
returns an empty string whenever `ResourceAPIVersion` is empty, so any exact-match GVK filtering in
workflow discovery silently degraded to Kind-only matching on this path.

## 2. Fix Design

One-line addition to the existing struct literal:

```go
return &katypes.SignalContext{
    Name:               rr.Spec.SignalName,
    Severity:           rr.Spec.Severity,
    RemediationID:      rrID,
    ResourceKind:       rr.Spec.TargetResource.Kind,
    ResourceName:       rr.Spec.TargetResource.Name,
    Namespace:          rr.Spec.TargetResource.Namespace,
    ResourceAPIVersion: rr.Spec.TargetResource.APIVersion,
}, nil
```

No new types, no new interfaces, no new callers — the RR CRD's `TargetResource.APIVersion` field
already existed and was already populated for this path's repro case (MCP/agent-driven RR creation,
`pkg/apifrontend/tools/af_create_rr.go`, already required + RESTMapper-validated). This fix only
stops the resolver from discarding a value that was already correct on the wire.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-10** | Information Input Validation | `ResourceAPIVersion` is part of the resource-identifying input to workflow discovery; the CRD-fallback path must preserve it with the same fidelity as the primary (session-provider) path. |
| **AU-2** | Auditable Events Defined | No new audit event type — this is a data-completeness fix on an existing, already-audited resolution path, not a new control surface. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control / BR | Test File |
|---|---|---|---|---|
| UT-KA-1175-SCR-005 | Unit | `SessionSignalContextResolver.ResolveSignalContext`, on session-provider miss, reads the RR CRD and returns a `SignalContext` whose `ResourceAPIVersion` matches `RR.Spec.TargetResource.APIVersion` | SI-10, BR-WORKFLOW-004 | `internal/kubernautagent/mcp/adapters/signal_resolver_test.go` |
| IT-KA-DISC-011 | Integration | The **production** `SessionSignalContextResolver`, constructed the same way as `cmd/kubernautagent/main.go`, round-trips `apiVersion` through a **real envtest** API server (not a fake client) — proves the value survives the K8s API server's structural-schema validation/pruning on the CRD-fallback branch, forced via an `alwaysMissSignalProvider` | SI-10, BR-WORKFLOW-004, BR-INTERACTIVE-010 | `test/integration/kubernautagent/mcp/discovery_flow_test.go` |

### Tier Coverage Rationale

- **UT** proves the field-mapping logic itself in isolation (fast, no envtest).
- **IT** proves the fix survives a real Kubernetes API server's CRD structural schema — a fake
  client (`client-go/fake` or an in-memory map) would not catch a schema that silently drops or
  defaults an unset/incorrectly-typed field the way a real `apiserver` (or envtest's real
  `kube-apiserver`) does. `IT-KA-DISC-011` deliberately bypasses the full MCP session-handshake flow
  and calls the resolver directly with a `SignalProvider` stub that always misses, isolating the
  CRD-fallback branch specifically — going through the full session flow would let KA's existing,
  unrelated `apiVersionValidationGate` (#1044/#1051) backfill `ResourceAPIVersion` from RCA output
  for an unambiguous kind, masking this exact defect.
- **E2E**: not added. This is a one-line, non-branching field-mapping fix on an existing,
  already-E2E-covered interactive-session code path (BR-INTERACTIVE-010's existing suites); no new
  user-facing journey is introduced.

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `SessionSignalContextResolver.ResolveSignalContext` | Constructed at `cmd/kubernautagent/main.go:1441` (`NewSessionSignalContextResolver(...)`) | `internal/kubernautagent/mcp/adapters/signal_resolver.go:90` | UT-KA-1175-SCR-005, IT-KA-DISC-011 |

**No new wiring points**: this fix is a struct-literal field addition inside an already-wired,
already-constructed component — CHECKPOINT W's concern (a component built but never called from
production) does not apply here; the component was already called from production before this fix,
it was simply returning an incomplete value.

## 6. CHECKPOINT W Evidence

```bash
$ grep -n "NewSessionSignalContextResolver" cmd/kubernautagent/main.go
cmd/kubernautagent/main.go:1441:	signalResolver := mcpadapters.NewSessionSignalContextResolver(autoMgr, ctrlCli, namespace)

$ grep -n "ResourceAPIVersion" internal/kubernautagent/mcp/adapters/signal_resolver.go
internal/kubernautagent/mcp/adapters/signal_resolver.go:90:		ResourceAPIVersion: rr.Spec.TargetResource.APIVersion,
```

## 7. Build Validation

```bash
$ go build ./...                                                          # exit 0
$ go vet ./...                                                            # exit 0
$ go test ./internal/kubernautagent/mcp/adapters/...                      # PASS
$ go test ./test/integration/kubernautagent/mcp/...                      # PASS (envtest)
```

## 8. Coverage Summary

| Metric | Target | Actual |
|---|---|---|
| BR/Control coverage (SI-10, AU-2, BR-WORKFLOW-004, BR-INTERACTIVE-010) | 100% | ✅ (Sections 3, 4) |
| Wiring Manifest rows with passing IT/UT evidence | 100% | ✅ (Section 5) |
| CHECKPOINT W (no orphaned code, pre-existing production caller confirmed) | Pass | ✅ (Section 6) |
| Build (`go build ./...`, `go vet ./...`) | Pass | ✅ (Section 7) |
| Real-envtest schema round-trip proof (not fake-client only) | Required | ✅ (IT-KA-DISC-011) |

## 9. Out of Scope

- **Track 2** (`AIAnalysis`→KA `IncidentRequest` wire contract missing `apiVersion` entirely): a
  distinct, larger defect on the *autonomous* investigation path — see
  [`docs/testing/2064/TEST_PLAN.md`](../2064/TEST_PLAN.md).
- **Track 3** (Gateway discarding/never capturing `apiVersion` for webhook-originated signals): the
  earliest point in the pipeline where this class of defect originates — see
  [`docs/testing/2066/TEST_PLAN.md`](../2066/TEST_PLAN.md).
- **KA-internal RCA-to-workflow-discovery propagation**: already correctly implemented
  (`apiVersionValidationGate`, #1044/#1051) — no gap found, no new work needed; Track 1/2/3 fix the
  *input* to this already-correct machinery.
- **Backport/cherry-pick to `main`/`v1.6`**: tracked separately via the cloned issue
  [#2063](https://github.com/jordigilh/kubernaut/issues/2063), not performed in this branch.

## 10. Sign-off

| Role | Name | Date | Signature |
|---|---|---|---|
| Author | AI Assistant | 2026-08-10 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
