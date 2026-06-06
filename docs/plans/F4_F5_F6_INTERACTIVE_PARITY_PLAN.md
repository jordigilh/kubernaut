# F4/F5/F6 Interactive-Autonomous Parity Plan

**Issue**: #1374 follow-up — Remaining interactive path stubs for GA readiness
**Status**: Plan (pending approval)
**Priority**: GA-blocking — "We can't have stubs in GA"

---

## Problem Statement

Three stubs remain in the MCP interactive path that prevent full parity with the autonomous `Investigate()` pipeline:

| ID | Stub | Impact | Complexity |
|----|------|--------|------------|
| F4 | `handleStartAutonomous` passes no-op `InvestigateFunc` | `action=start_autonomous` creates a session that immediately completes with no result | Medium |
| F5 | `ResolveEnrichmentData` returns empty `&prompt.EnrichmentData{}` | Phase 3 LLM prompt lacks owner chain, labels, quota, remediation history | Medium |
| F6 | `DetectedLabelsJSON` never set on interactive signal | DS catalog GitOps-aware scoring/filtering inactive in `discover_workflows` | Medium (depends on F5) |

---

## Dependency Graph

```
F5 (enrichment) ──► F6 (DetectedLabelsJSON)
                         │
F4 (autonomous)    (independent)
```

**Recommended execution order**: F5 → F6 → F4

---

## F4: Wire Real Investigation in `handleStartAutonomous`

### Current State

```go
// internal/kubernautagent/mcp/tools/investigate.go L1005-1007
sessionID, err := t.autoMgr.StartInvestigation(ctx, func(ctx context.Context) (*katypes.InvestigationResult, error) {
    return nil, ctx.Err()  // ← no-op: exits immediately with (nil, nil)
}, metadata)
```

### Required Changes

1. **Resolve `SignalContext`** from RR via `t.signalResolver.ResolveSignalContext(ctx, input.RRID)`
2. **Build real `InvestigateFunc`** that calls the investigator:
   ```go
   investigateFn := func(bgCtx context.Context) (*katypes.InvestigationResult, error) {
       return t.runner.RunFullInvestigation(bgCtx, signal)
   }
   ```
3. **Extend `InvestigatorRunner` interface** with `RunFullInvestigation(ctx, signal) (*InvestigationResult, error)` — or add a new dependency interface (e.g., `AutonomousInvestigator`)
4. **Wire `EventLogBridge`** in `registration.go` for `autonomous_started` responses (similar to `action=start` + `InvestigationSessionID` path)

### Files Affected

| File | Change |
|------|--------|
| `internal/kubernautagent/mcp/tools/investigate.go` | `handleStartAutonomous`: resolve signal, build real `InvestigateFunc` |
| `internal/kubernautagent/mcp/tools/investigate.go` | `InvestigatorRunner` interface: add `RunFullInvestigation` or new interface |
| `internal/kubernautagent/mcp/adapters/investigator_runner.go` | Implement the new method (delegates to `investigator.Investigate`) |
| `cmd/kubernautagent/main.go` | Wire the investigator dependency |
| `internal/kubernautagent/mcp/registration.go` | Wire `EventLogBridge` for autonomous sessions |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `RunFullInvestigation` | `handleStartAutonomous()` | `investigate.go` | IT-KA-AUTO-001 |
| `EventLogBridge` for autonomous | `registration.go` | `registration.go` | IT-KA-AUTO-002 |

### Risk Assessment

- **Medium risk**: Requires new interface method and adapter wiring
- **Test coverage**: Existing `autonomous_test.go` tests validate dispatch, duplicate detection, Subscribe. New ITs needed for real investigation execution.
- **Mitigation**: Can be gated behind a feature flag if needed

---

## F5: Wire Real Enrichment in `ResolveEnrichmentData`

### Current State

```go
// internal/kubernautagent/mcp/adapters/signal_resolver.go L94-96
func (r *SessionSignalContextResolver) ResolveEnrichmentData(_ context.Context, _ string) (*prompt.EnrichmentData, error) {
    return &prompt.EnrichmentData{}, nil  // ← empty: no owner chain, labels, quota, history
}

func (r *SessionSignalContextResolver) ResolvePostRCAEnrichment(_ context.Context, _, _, _, _ string) (*prompt.EnrichmentData, error) {
    return nil, nil  // ← stub: no post-RCA re-enrichment
}
```

### Required Changes

1. **Add `*enrichment.Enricher` dependency** to `SessionSignalContextResolver`
2. **Implement `ResolveEnrichmentData`**:
   - Resolve signal via `GetSignalForRemediation(rrID)`
   - Call `enricher.Enrich(ctx, kind, name, namespace, "", "", incidentID)`
   - Convert via `toPromptEnrichment` (needs exporting or shared helper)
3. **Implement `ResolvePostRCAEnrichment`**:
   - Call `enricher.Enrich(ctx, kind, name, namespace, "", "", incidentID)` with RCA target
   - Convert via `toPromptEnrichment`
4. **Export `toPromptEnrichment`** from `investigator_phases.go` (or extract to shared package)
5. **Update `main.go`**: Pass `enricher` to `NewSessionSignalContextResolver`

### Files Affected

| File | Change |
|------|--------|
| `internal/kubernautagent/mcp/adapters/signal_resolver.go` | Wire enricher, implement both methods |
| `internal/kubernautagent/investigator/investigator_phases.go` | Export `toPromptEnrichment` → `ToPromptEnrichment` |
| `cmd/kubernautagent/main.go` | Pass enricher to resolver constructor |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `ResolveEnrichmentData` (real) | `handleDiscoverWorkflows()` | `investigate.go:841` | IT-KA-ENRICH-001 |
| `ResolvePostRCAEnrichment` (real) | `handleDiscoverWorkflows()` | `investigate.go:849-858` | IT-KA-ENRICH-002 |
| `ToPromptEnrichment` export | `signal_resolver.go` | `investigator_phases.go` | UT-KA-ENRICH-001 |

### Risk Assessment

- **Low-medium risk**: Enrichment pipeline already exists and is battle-tested in autonomous path
- **Key concern**: `toPromptEnrichment` export — must not break existing callers
- **Performance**: Enrichment calls K8s API (owner chain) — adds latency to `discover_workflows`

---

## F6: Propagate `DetectedLabelsJSON` in Interactive Path

### Current State

The autonomous path sets `DetectedLabelsJSON` on the signal before Phase 3:

```go
// investigator.go L521-537
if enrichData != nil && enrichData.DetectedLabels != nil {
    if dlJSON, err := json.Marshal(enrichData.DetectedLabels); err == nil {
        workflowSignal.DetectedLabelsJSON = string(dlJSON)
    }
}
```

The interactive path never sets this field. DS catalog scoring is inactive.

### Required Changes (depends on F5)

1. **In `RunWorkflowDiscoveryFromRCA`**: After enrichment is available (F5), marshal `DetectedLabels` onto the signal before `runWorkflowSelection`
2. **Extract shared helper**: `AttachDetectedLabelsJSON(signal *SignalContext, enrichData *enrichment.EnrichmentResult) error`
3. **Update `FinalizeWorkflowResult`**: Accept `*enrichment.EnrichmentResult` instead of `nil` when enrichment is available

### Files Affected

| File | Change |
|------|--------|
| `internal/kubernautagent/investigator/investigator.go` | `RunWorkflowDiscoveryFromRCA`: marshal labels onto signal |
| `internal/kubernautagent/investigator/investigator_phases.go` | Extract `AttachDetectedLabelsJSON` helper |
| `internal/kubernautagent/investigator/investigator.go` | Autonomous path: use shared helper |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `AttachDetectedLabelsJSON` | `RunWorkflowDiscoveryFromRCA()` | `investigator.go` | IT-KA-LABELS-001 |
| `AttachDetectedLabelsJSON` | `Investigate()` | `investigator.go` (refactor) | IT-KA-1052-001 (existing) |

### Risk Assessment

- **Low risk**: Depends on F5 enrichment being wired first
- **Graceful degradation**: If labels are nil, DS scoring already falls back to baseline
- **Test coverage**: Existing IT-KA-1052-* tests cover autonomous; new IT for interactive

---

## TDD Execution Plan

### Phase 1: F5 (Enrichment Wiring)

| Step | Action |
|------|--------|
| RED | Write UT for `ToPromptEnrichment` (exported). Write IT-KA-ENRICH-001/002 that assert enrichment data flows through `discover_workflows` |
| GREEN | Export `toPromptEnrichment`, wire enricher into `SessionSignalContextResolver`, implement both methods, update `main.go` |
| CHECKPOINT W | Verify production callers + passing ITs |
| REFACTOR | Extract shared conversion helpers |

### Phase 2: F6 (DetectedLabelsJSON)

| Step | Action |
|------|--------|
| RED | Write IT-KA-LABELS-001 asserting `DetectedLabelsJSON` is set on interactive discovery signal |
| GREEN | Extract `AttachDetectedLabelsJSON`, call in `RunWorkflowDiscoveryFromRCA`, refactor autonomous to use shared helper |
| CHECKPOINT W | Verify production callers + passing ITs |
| REFACTOR | DRY up autonomous/interactive label attachment |

### Phase 3: F4 (Autonomous Investigation)

| Step | Action |
|------|--------|
| RED | Write IT-KA-AUTO-001 asserting `handleStartAutonomous` runs a real investigation |
| GREEN | Add `RunFullInvestigation` to interface, implement adapter, wire in `handleStartAutonomous` |
| CHECKPOINT W | Verify production callers + passing ITs |
| REFACTOR | Clean up, add EventLogBridge wiring |

---

## Business Requirements Mapping

| BR | Description | Coverage |
|----|-------------|----------|
| BR-INTERACTIVE-010 SC-6 | MCP parity with autonomous pipeline | F4, F5, F6 |
| BR-AI-056 | GitOps-aware workflow scoring | F6 |
| BR-MCP-002 | Autonomous investigation via MCP | F4 |
| BR-WORKFLOW-004 | GVK format correctness | F5, F6 |

## FedRAMP Control Mapping

| Control | Requirement | Impact |
|---------|-------------|--------|
| SI-10 | Input validation completeness | F5: enrichment validates resource identity |
| AU-2/AU-12 | Audit completeness | F4: autonomous sessions must produce auditable results |

---

## Estimated Effort

| Item | Estimate |
|------|----------|
| F5 (enrichment) | 2-3 hours |
| F6 (labels) | 1-2 hours |
| F4 (autonomous) | 2-3 hours |
| **Total** | **5-8 hours** |

---

## Decisions (Closed)

1. **No feature flag for F4.** Behavior should be derived from the prompt or the agent should ask when unclear — no default autonomous/interactive assumption.
2. **No cache for F5.** Direct K8s API calls. Creating a cache is over-engineering without production volume metrics to justify the complexity.
3. **Full autonomous parity for F5.** Pre-RCA + post-RCA re-enrichment with HardFail/merge. No reason for a different behavior between paths.
