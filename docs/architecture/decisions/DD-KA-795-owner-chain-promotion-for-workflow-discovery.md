# DD-KA-795: Owner Chain Promotion for Workflow Discovery

**Status**: APPROVED
**Decision Date**: 2026-04-22
**Version**: 1.0
**Confidence**: 92%
**Applies To**: Kubernaut Agent (KA)
**Issue**: [#795](https://github.com/jordigilh/kubernaut/issues/795)

---

## Context & Problem

### Symptom

`list_available_actions` returns empty results for Pod signals even though matching workflows exist for the owning Deployment/StatefulSet.

### Root Cause

When the LLM's RCA output fails to parse (plain text instead of structured JSON) or provides no explicit `remediation_target`, `ResolveEnrichmentTarget()` falls back to `signal.ResourceKind` (Pod). The re-enrichment guard (`postRCAKind != signalKind`) evaluates to false, so `workflowSignal.ResourceKind` stays "Pod". The `list_available_actions` tool then queries DataStorage with `component=pod`, which matches no workflows since workflows are typically registered against higher-level controllers (Deployment, StatefulSet, DaemonSet, etc.).

### HAPI Precedent (v1.2.1)

HAPI solves this via `_effective_component()` in `WorkflowDiscoveryToolset`, which dynamically reads `session_state["root_owner"]["kind"]` -- populated by the enrichment phase from the owner chain root. Even when RCA parsing is incomplete, the enrichment's owner chain resolution provides the correct component for discovery.

```python
def _effective_component(self) -> str:
    if self._session_state:
        root_owner = self._session_state.get("root_owner")
        if isinstance(root_owner, dict):
            resolved = root_owner.get("kind", "")
            if resolved:
                return resolved
    return self._component or ""
```

---

## Decision

Promote the owner chain root to `workflowSignal` when re-enrichment did **not** run, the initial enrichment has a non-empty owner chain, and the chain root kind differs from the signal kind.

This is a **targeted fix** within `Investigate()` -- no new types, no new interfaces.

---

## Design

### Owner Chain Promotion Block

Inserted after the re-enrichment block in `Investigate()`, guarded by `reEnrichmentRan`:

```go
if !reEnrichmentRan && enrichData != nil && len(enrichData.OwnerChain) > 0 {
    root := enrichData.OwnerChain[len(enrichData.OwnerChain)-1]
    if root.Kind != "" && root.Kind != workflowSignal.ResourceKind {
        promotedNS := inv.normalizeNamespace(root.Kind, root.Namespace)
        workflowSignal.ResourceKind = root.Kind
        workflowSignal.ResourceName = root.Name
        workflowSignal.Namespace = promotedNS
        // Emit audit event with promotion_trigger="owner_chain_root"
    }
}
```

### Key Design Choices

1. **`reEnrichmentRan` boolean flag**: Explicitly tracks whether the re-enrichment block executed, avoiding fragile kind-equality checks that could fail when RCA identifies a same-kind but different-name resource.

2. **`normalizeNamespace`**: Applies to the promoted kind to handle cluster-scoped owner chain roots (e.g., Node, PersistentVolume) where namespace must be cleared.

3. **Audit event**: Emits `aiagent.enrichment.completed` with `promotion_trigger=owner_chain_root` and `signal_kind=<original>` to distinguish promotion events from standard enrichment events in the audit trail.

### OpenAPI Schema Extension

`AIAgentEnrichmentCompletedPayload` gains two optional nullable fields:

| Field | Type | Description |
|-------|------|-------------|
| `promotion_trigger` | `string?` | Trigger reason when event is from promotion (e.g., `"owner_chain_root"`) |
| `signal_kind` | `string?` | Original signal resource kind before promotion |

---

## Test Coverage

| Test ID | Scenario | Assertion |
|---------|----------|-----------|
| IT-KA-795-001 | Pod signal, RCA parse failure, owner chain has Deployment root | `component=deployment` sent to DS |
| IT-KA-795-002 | Pod signal, cluster-scoped root (Node), ScopeResolver configured | `component=node` with empty namespace |
| IT-KA-795-003 | RCA identifies different Pod, re-enrichment runs | Promotion does NOT fire; uses re-enriched target |

---

## Business Requirement

**BR-KA-795** (extends BR-HAPI-261): When RCA parsing fails or provides no remediation target, KA MUST use the owner chain root kind as the workflow discovery component, matching HAPI v1.2.1 `_effective_component()` behaviour.

---

## Blast Radius

| Component | File | Change |
|-----------|------|--------|
| KA Investigator | `internal/kubernautagent/investigator/investigator.go` | Add promotion block + `reEnrichmentRan` flag |
| OpenAPI Spec | `api/openapi/data-storage-v1.yaml` | Add `promotion_trigger`, `signal_kind` to enrichment payload |
| Ogen Client | `pkg/datastorage/ogen-client/oas_schemas_gen.go` | Regenerated |
| Audit DS Store | `internal/kubernautagent/audit/ds_store.go` | Map new fields in `buildEventData` |
| Integration Tests | `test/integration/kubernautagent/investigator/owner_chain_promotion_795_it_test.go` | New file: 3 test cases |

---

## Related Decisions

| Document | Relationship |
|----------|-------------|
| DD-HAPI-017 | KA-side equivalent of HAPI's `_effective_component()` in `WorkflowDiscoveryToolset` |
| ADR-056 | Re-enrichment architecture that this fix complements |
| DD-AUDIT-002 | Audit event schema that `promotion_trigger` / `signal_kind` extends |

---

**Document Version**: 1.0
**Last Updated**: April 22, 2026
**Authority**: KA workflow discovery correctness fix
