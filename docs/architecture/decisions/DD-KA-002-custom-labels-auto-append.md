# DD-KA-002: Custom Labels Workflow-Matching Architecture

**Date**: November 30, 2025
**Status**: ✅ APPROVED
**Deciders**: Architecture Team
**Version**: 2.0
**Related**: DD-WORKFLOW-004 v1.7 (per-key custom-label boost), DD-WORKFLOW-019 (KA-owned workflow discovery), DD-KA-017 (Three-Step Workflow Discovery Integration)

> **Note (2026-08-01, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: Rewritten against the Go implementation and renumbered `DD-HAPI-001` → **`DD-KA-002`** (not `DD-KA-001` — that identifier was already taken by the unrelated `DD-KA-001-workflow-response-validation-architecture.md`, renamed from `DD-HAPI-002` earlier in this cleanup; `DD-KA-002` was the next free slot in the `DD-KA-*` range at the time of this rewrite). The original document described a Python `WorkflowCatalogToolset`/MCP auto-append mechanism that no longer exists — workflow discovery moved in-process into KA (DD-WORKFLOW-019). **This rewrite surfaced an apparent wiring gap** (see "Known Gap" below): `custom_labels` reaches KA over the wire but is not currently forwarded into workflow-discovery scoring. This is flagged for a separate issue, not fixed here.

---

## Context

### Problem Statement

Custom labels (`custom_labels`) are customer-defined filtering constraints extracted by SignalProcessing via Rego policies (e.g. `constraint: ["cost-constrained"]`, `team: ["name=payments"]`). Per DD-WORKFLOW-004 v1.7, workflows in the catalog can declare their own `CustomLabels` (which constraints/teams they satisfy), and the discovery/scoring engine boosts workflows whose custom labels match the incident's custom labels.

**The original (Python HAPI) design question**: should the LLM be responsible for including `custom_labels` in its MCP tool calls, or should the service auto-append them invisibly?

**Answer (unchanged in the Go rewrite)**: Auto-append — `custom_labels` are operational metadata, not investigation context; the LLM should not need to "think" about them.

---

## Decision

**Custom-label matching is scoring, not investigation input.** It happens entirely on the discovery/catalog side, using request-time context the platform already has — the LLM never sees or provides `custom_labels`.

### Current (Go) Architecture

Workflow discovery is owned by KA's in-process catalog (`internal/kubernautagent/workflowcatalog`, per DD-WORKFLOW-019), not by a remote Data Storage MCP call. The relevant pieces:

1. **Catalog-side matching** (`internal/kubernautagent/workflowcatalog/cache_filter.go`, `customLabelsBoost`): each cached `RemediationWorkflow` carries a `CustomLabels map[string][]string` (from the `RemediationWorkflow` CRD's `spec.customLabels`, converted by `crdCustomLabelsToModel`). `customLabelsBoost` computes a `DD-WORKFLOW-004 v1.7` boost (`customLabelsWeight = 0.15` per matching key/value pair, half-weight for a workflow-side wildcard `"*"`) against the query's `filterCustom` (the incident's custom labels).
2. **Query-side filter struct** (`pkg/datastorage/models/workflow_discovery.go`, `WorkflowDiscoveryFilters.CustomLabels`): the field that `customLabelsBoost` compares against — populated by whatever calls `ListActions`/`ListWorkflowsByActionType`/`GetWorkflowWithContextFilters`.
3. **LLM contract unchanged from the original decision**: the three discovery tools (`list_available_actions`, `list_workflows`, `get_workflow`) never expose `custom_labels` as an LLM-provided parameter (`internal/kubernautagent/tools/custom/tools.go`). Whatever supplies `WorkflowDiscoveryFilters.CustomLabels` must come from request context, not the LLM.

### Known Gap (discovered during this rewrite, not fixed here)

`filtersFromSignal` (`internal/kubernautagent/tools/custom/tools.go`), the function that builds `WorkflowDiscoveryFilters` for every `list_available_actions`/`list_workflows` call, populates `Severity`, `Component`, `Environment`, `Priority`, `RemediationID`, and `Cluster` from `katypes.SignalContext` — but **`katypes.SignalContext` has no `CustomLabels` field**, and `filtersFromSignal` does not set `filters.CustomLabels`. The `get_workflow` tool's best-effort filter construction has the same omission.

Tracing the data upstream: SignalProcessing computes `CustomLabels` via Rego (`internal/controller/signalprocessing/signalprocessing_enriching.go`) → AIAnalysis forwards it to KA over the wire (`pkg/aianalysis/handlers/request_builder.go`, `result.CustomLabels.SetTo(...)`) → but **no code under `internal/kubernautagent/api` reads `CustomLabels` off the incoming request**, so it never reaches `SignalContext` or the discovery filters.

**Net effect**: `customLabelsBoost` and its `0.15`-per-match weighting are fully implemented and unit-tested (`cache_filter_test.go`) against hand-constructed `WorkflowDiscoveryFilters{CustomLabels: ...}` fixtures, but in a real production call, `filters.CustomLabels` is always empty — the boost can never fire. This looks like a "built but not wired" gap under this project's Wiring Verification checkpoint (a `pkg/`-level feature with test coverage but no production caller supplying real data), rather than an intentional behavior change. Filing a follow-up issue to either (a) add `CustomLabels` to `SignalContext` and populate it from the incoming request in `internal/kubernautagent/api`, or (b) confirm this was intentionally descoped, is recommended.

---

## LLM Prompt Impact

Unchanged from the original decision: `custom_labels` is never mentioned in LLM prompts or the three discovery tools' parameter schemas. The LLM's discovery calls (`list_available_actions(action_type)`, `list_workflows(action_type, filters)`, `get_workflow(workflow_id)`) carry no `custom_labels` argument; scoring happens entirely inside the catalog, keyed off whatever request-time filters were supplied by the caller (see "Known Gap" above for the current state of that supply path).

---

## Rationale

### Why Auto-Append (Scoring-Side Matching) is Better

1. **100% Reliable** (when wired): Custom labels are always included by the platform, no LLM "forgetting"
2. **Simpler LLM Contract**: LLM focuses on RCA and action-type selection, not operational metadata
3. **No Prompt Bloat**: Don't need to explain the `map[string][]string` custom-labels structure to the LLM
4. **Consistent with other pass-through context** (e.g. `RemediationID`, `Cluster`): same request-context-derived pattern

### Why NOT LLM-Prompted

1. **Unreliable**: LLM might forget/omit custom_labels
2. **Cognitive Overhead**: LLM must track and reproduce custom_labels correctly, risking transformation/misinterpretation (violates the pass-through principle in DD-WORKFLOW-004)

---

## Cross-References

### DD-WORKFLOW-004 v1.7 Custom-Label Boost

DD-WORKFLOW-004 v1.7 establishes the per-key custom-label boost weighting (`customLabelsWeight = 0.15`) and the "Pass-Through Principle":

> Kubernaut does NOT validate DetectedLabels or CustomLabels values — they are passed through to the workflow catalog for matching.

This DD's scoring-side implementation (`customLabelsBoost`) is that principle's realization in the Go catalog.

### DD-WORKFLOW-019 (KA-Owned Workflow Discovery)

Workflow discovery — including all custom-label scoring — now runs in-process inside KA against its own cached catalog, not via a remote MCP call to Data Storage. This DD's original "auto-append into an MCP tool call" framing is obsolete; matching happens as a pure function (`filterAndScoreCachedWorkflows`) over already-fetched CRDs.

---

## Consequences

### Positive

- Custom-label scoring, when wired end-to-end, requires zero LLM cognitive load
- Scoring logic is a pure, well-unit-tested function (`customLabelsBoost`) independent of the discovery tools' plumbing

### Negative / Risk

- The current implementation gap (see "Known Gap") means production discovery calls never populate `CustomLabels`, silently disabling this scoring dimension until the gap above is addressed

---

## Testing Strategy

### Unit Tests (existing, verified against current code)

- `customLabelsBoost` full-weight match, half-weight wildcard match, and zero-boost cases (`internal/kubernautagent/workflowcatalog/cache_filter_test.go`)
- `crdCustomLabelsToModel` CRD→model conversion, including nil input (`cache_convert_test.go`)

### Gap Coverage (recommended follow-up, not yet written)

- An integration test asserting that a `CustomLabels`-bearing incident, routed end-to-end through KA's discovery tools, actually produces a boosted score — this test does not currently exist, consistent with the wiring gap identified above.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-30 | Initial version - Auto-append architecture approved (Python HAPI / MCP `search_workflow_catalog`) |
| 2.0 | 2026-08-01 | Rewritten against the Go implementation ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)). Renumbered DD-HAPI-001 → **DD-KA-002** (DD-KA-001 was already taken by the renamed workflow-response-validation-architecture doc). Replaced Python `WorkflowCatalogToolset` auto-append design with the current in-process `customLabelsBoost` catalog scoring. Documented the discovered CustomLabels wiring gap. |

---

**Document Version**: 2.0
**Last Updated**: 2026-08-01
**Status**: ✅ APPROVED (architecture); ⚠️ implementation gap flagged for follow-up
