# DD-KA-017: Three-Step Workflow Discovery Integration

**Status**: ✅ APPROVED
**Decision Date**: 2026-02-05
**Version**: 2.0 (Go rewrite)
**Confidence**: 90%
**Applies To**: Kubernaut Agent (KA), DataStorage Service (DS)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0–1.5 | 2026-02-05 to 2026-03-24 | Architecture Team | Historical evolution under the Python-era implementation: introduced the three-step tools, moved label detection from signal source to RCA target (ADR-056), surfaced labels as read-only `cluster_context`, added one-shot reassessment via `detected_infrastructure`, and split resource-context tools by scope (Issue #524). Superseded by v2.0 below; see git history for the original entries. |
| 2.0 | 2026-08-01 | — | Rewritten against the Go KA implementation as part of [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806). Replaced the Python "shared mutable `session_state` dict" mechanism with Go's actual mechanism: `SignalContext.DetectedLabelsJSON`, propagated via `context.Context`. Removed the dual incident/recovery-flow framing — Go KA has a single unified investigation flow, so the "recovery flow validation parity" rationale is historical only. Renamed `BR-HAPI-017-*` → `BR-KA-017-*`. Corrected the label count from 7 to the current 12 infrastructure characteristics (`internal/kubernautagent/enrichment/label_detector.go`). **Also corrected a factually wrong first draft**: the tools do not call DataStorage over REST — per [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) (Issue #1677, implemented), discovery/scoring ownership moved from DS into KA's own `workflowcatalog.Catalog` before this rewrite was even written; the DS REST surface this DD originally described is retired dead code. |

---

## Context & Problem

### Current State (Go KA)

KA exposes three LLM tools for workflow discovery, defined in `internal/kubernautagent/investigator/types.go` and implemented in `internal/kubernautagent/tools/custom/tools.go`:

| Tool | Catalog Call | Responsibility |
|------|-------------|-----------------|
| `list_available_actions` | `workflowcatalog.Catalog.ListActions` | Action type discovery from taxonomy |
| `list_workflows` | `workflowcatalog.Catalog.ListWorkflowsByActionType` | Workflow selection within an action type |
| `get_workflow` | `workflowcatalog.Catalog.GetWorkflow` (or equivalent by-ID lookup) | Single workflow parameter schema lookup |

**Important architectural correction vs. the historical (pre-#1677) design**: these tools do **not** call DataStorage (DS) over the network. Per [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) (implemented, Issue #1677), workflow/action-type discovery, search, and scoring logic — and the informer-backed cache that backs it — was relocated **from DS into KA itself** (`internal/kubernautagent/workflowcatalog`). DS's `/api/v1/workflows*` REST surface was retired as dead code in that migration. The tool table above reflects the current in-process call path; DD-WORKFLOW-019 is authoritative on *why* and *when* that ownership moved. DD-WORKFLOW-016 remains authoritative for the discovery protocol's conceptual shape (three steps, taxonomy-driven action types, pagination contract), independent of which service executes it.

Each tool passes signal context filters (severity, component, environment, priority, custom_labels, detected_labels) into the catalog call and passes `remediationId` through for audit correlation (BR-AUDIT-023) via KA's own buffered audit-write path to DS (`internal/kubernautagent/audit/ds_buffered_store.go`), not a DS-handler-side audit emission.

KA has a single, unified investigation flow (there is no separate "incident" vs "recovery" LLM flow in the Go implementation — the historical Python design's dual-flow-parity rationale, described in the v1.0–v1.5 changelog above, no longer applies).

### Problems (original motivation, still valid)

1. **Action/workflow conflation**: A single search call forces the LLM to select both the remediation action type and the specific workflow at once, with no structured way to first understand available action categories before drilling into specific workflows.
2. **No comprehensive review**: A ranked-list-returning search tool encourages the LLM to pick the first result rather than reviewing all available workflows.
3. **Semantic search noise**: Free-text semantic search is less reliable than the deterministic, curated `action_type` taxonomy (DD-WORKFLOW-016).

---

## Decision

Workflow discovery uses three purpose-built tools instead of a single free-text search tool, structured as: resolve target/enrichment context → discover action types → list workflows for the chosen action type → fetch the selected workflow's parameter schema → validate before execution.

**Rationale**:
1. **Reduced LLM cognitive load**: Three focused tools with clear responsibilities outperform one overloaded tool.
2. **Comprehensive workflow review**: The `list_workflows` step is designed so the LLM reviews all candidate workflows before selecting, preventing premature selection.
3. **Deterministic taxonomy over semantic noise**: `action_type` taxonomy (DD-WORKFLOW-016) replaces embedding-similarity search.

---

## Design

### 1. Label Injection into Discovery Queries

KA computes 12 infrastructure detection characteristics for the RCA target during enrichment (`internal/kubernautagent/enrichment/label_detector.go`), then attaches them to the outgoing discovery-tool calls via the signal context, not a shared mutable dict:

- `internal/kubernautagent/investigator/investigator_discovery.go` (`attachRCADetectedLabelsJSON`) marshals `enrichData.DetectedLabels` onto `SignalContext.DetectedLabelsJSON` after enrichment resolves the RCA target (#1374 — activates GitOps-aware DS scoring, parity with the autonomous investigation path).
- `SignalContext` (carrying `DetectedLabelsJSON`) is propagated through the request via `context.Context` (`katypes.WithSignalContext` / `katypes.SignalContextFromContext`).
- `internal/kubernautagent/tools/custom/tools.go`: `list_available_actions` and `list_workflows` read `SignalContext.DetectedLabelsJSON` from the context, unmarshal it, and set `filters.DetectedLabels` on the call into `workflowcatalog.Catalog`. When `DetectedLabelsJSON` is empty, no label filter is applied (graceful degradation) — no resource-context tool call is a hard prerequisite in the Go implementation.

This replaces the Python-era design's per-investigation shared mutable `session_state` dict (constructed once per request and passed by reference into two toolset constructors) with an idiomatic Go alternative: an immutable value carried on the request context.

### 2. Post-Selection Validation

Selected workflows are validated against a parameter schema before execution (`internal/kubernautagent/parser/validator.go`), with undeclared parameters stripped except for `kaManagedParams` (see [DD-KA-006](DD-KA-006-remediation-target-in-rca.md)). If the investigation cannot converge on a valid selection within its turn budget, KA sets `needs_human_review=true` with `human_review_reason=investigation_inconclusive` (`internal/kubernautagent/investigator/investigator_gates.go`) rather than executing an unvalidated workflow.

### 3. Remediation History Context (Causal Chains, Regression Detection)

The "no aggregate success metrics in the discovery prompt" principle still holds: `list_workflows` does not surface global `actualSuccessRate`/`totalExecutions` to the LLM, since aggregate stats measured across unrelated signals/environments have no predictive value for the current incident. Instead, the LLM receives **target-scoped** remediation history:

- Three-way spec-hash comparison (`preRemediationSpecHash` / `postRemediationSpecHash` / `currentSpecHash`) detects configuration regression, unchanged state, or drift for the specific target resource — see [Issue #1802](https://github.com/jordigilh/kubernaut/issues/1802) for the target-resource-scoped query fix.
- Causal-chain and declining-effectiveness-trend detection over that history is built in Go (`internal/kubernautagent/prompt/history.go`, `internal/kubernautagent/prompt/templates/remediation_history.tmpl`), replacing the old `_detect_spec_drift_causal_chains` Python helper.
- This contextual history is rendered into the investigation prompt (`internal/kubernautagent/prompt/builder.go`) ahead of the workflow discovery phase.

### 4. Discovery Logic Ownership

The discovery/scoring logic and its informer-backed cache are owned by KA (`internal/kubernautagent/workflowcatalog`), not DS — see [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md) for the full rationale and migration history (Issue #1677). DS retains the `RemediationWorkflow`/`ActionType` CRDs as the source of truth that KA's informer cache watches; DS itself no longer serves discovery queries. The three-step protocol's conceptual shape (steps, taxonomy, pagination contract) remains authoritatively defined in DD-WORKFLOW-016.

---

## Alternatives Considered

### Alternative 1: Coexistence (deploy new tools alongside old)
**Confidence**: 40% (rejected) — two code paths to maintain, increased LLM cognitive load from four tools instead of three.

### Alternative 2: Feature flag toggle
**Confidence**: 50% (rejected) — doubles the testing matrix, prompt builder must generate two instruction sets.

### Alternative 3: Direct replacement (APPROVED)
**Confidence**: 92% (approved) — single code path, unambiguous LLM prompt contract, no risk of the LLM reaching for a removed tool.

---

## Business Requirements

### BR-KA-017-001: Three-Step Tool Implementation
- **Category**: Kubernaut Agent (KA)
- **Priority**: P0
- **Description**: MUST implement three LLM tools (`list_available_actions`, `list_workflows`, `get_workflow`) per DD-WORKFLOW-016.
- **Acceptance Criteria**:
  - Three tool implementations, each with a correct `workflowcatalog.Catalog` call and parameter mapping
  - All tools pass signal context filters (severity, component, environment, priority, custom_labels) into the catalog call
  - `detected_labels` is forwarded from `SignalContext.DetectedLabelsJSON` when present; omitted when absent (graceful degradation, no hard prerequisite)
  - All tools propagate `remediationId` for KA's own audit-event correlation

### BR-KA-017-002: Prompt Instructions for Three-Step Discovery
- **Category**: Kubernaut Agent (KA)
- **Priority**: P0
- **Description**: The investigation prompt MUST instruct the LLM to use the three-step discovery protocol (review action types, review all workflows for the chosen action type, fetch the parameter schema) rather than free-text search.
- **Acceptance Criteria**:
  - Prompt explicitly requires reviewing all workflows returned by `list_workflows` before selecting

### BR-KA-017-003: Post-Selection Validation with Security Gate
- **Category**: Kubernaut Agent (KA)
- **Priority**: P0
- **Description**: MUST pass full signal context filters to `get_workflow` during post-selection validation, activating the DS security gate. A 404 response indicates the workflow does not match the signal context.
- **Acceptance Criteria**:
  - Validation calls `get_workflow` with all signal context filters
  - 404 from DS treated as a validation failure and drives self-correction

### BR-KA-017-004: Unified Validation Flow
- **Category**: Kubernaut Agent (KA)
- **Priority**: P0
- **Description**: The single unified investigation flow validates every LLM workflow selection; there is no unvalidated code path. (Historical BR-HAPI-017-004 required recovery-flow parity with the incident flow; that distinction no longer exists in Go KA.)
- **Acceptance Criteria**:
  - Every workflow selection passes through parameter validation before being returned to AIAnalysis
  - Validation failure triggers self-correction within the turn budget, then `investigation_inconclusive` if unresolved

### BR-KA-017-005: remediationId Audit Correlation
- **Category**: Kubernaut Agent (KA) / Audit
- **Priority**: P0
- **Description**: MUST propagate `remediationId` through KA's own discovery/selection audit-event emission (`BufferedDSAuditStore`), preserving BR-AUDIT-023 correlation now that KA — not DS — emits the discovery/selection audit events (per DD-WORKFLOW-019).
- **Acceptance Criteria**:
  - `remediationId` present on KA-emitted discovery/selection audit events
  - Empty `remediationId` handled gracefully (discovery proceeds, audit has empty correlation)

### BR-KA-017-006: Single Discovery Code Path
- **Category**: Kubernaut Agent (KA)
- **Priority**: P0
- **Description**: There is exactly one workflow-discovery tool set; no legacy free-text search tool exists.
- **Acceptance Criteria**:
  - No free-text `search_workflow_catalog`-style tool is registered anywhere in KA

### BR-KA-017-007: Label Context for Action Type Selection
- **Category**: Kubernaut Agent (KA)
- **Priority**: P1
- **Description**: Detected infrastructure labels for the RCA target MUST be available to inform action-type selection (via the catalog filters applied in `list_available_actions`/`list_workflows`, not necessarily surfaced verbatim to the LLM as prompt text).
- **Acceptance Criteria**:
  - `filters.DetectedLabels` is populated on the `workflowcatalog.Catalog` call when `SignalContext.DetectedLabelsJSON` is non-empty
- **Authority**: ADR-056, DD-KA-018

### BR-KA-017-008: Target-Scoped Remediation History in Discovery Prompt
- **Category**: Kubernaut Agent (KA)
- **Priority**: P1
- **Description**: The investigation prompt MUST include target-scoped remediation history (spec-hash regression/drift detection, causal chains, effectiveness trend) ahead of workflow discovery, instead of global aggregate success metrics.
- **Acceptance Criteria**:
  - `list_workflows` response excludes `actualSuccessRate`/`totalExecutions`
  - Remediation history section renders via `internal/kubernautagent/prompt/history.go` / `remediation_history.tmpl`
- **Authority**: [Issue #1802](https://github.com/jordigilh/kubernaut/issues/1802)

---

## Related Decisions

| Document | Relationship |
|----------|-------------|
| **DD-WORKFLOW-016** | Authoritative source for the three-step discovery protocol's conceptual shape (steps, taxonomy, pagination contract). This DD implements that protocol as KA's LLM-facing tool contract. |
| **DD-WORKFLOW-019** | Authoritative source for **where** the discovery/scoring logic and cache live (KA, not DS) and why (Issue #1677). This DD assumes that ownership as a given and focuses on the LLM tool contract on top of it. |
| **DD-WORKFLOW-017** | Workflow lifecycle component interactions. |
| **DD-WORKFLOW-014 v3.0** | Audit trail for workflow selection; KA propagates `remediationId` for correlation. |
| **DD-KA-006** | Remediation target resolution; `kaManagedParams` are the parameters this DD's `get_workflow`/validation step never strips. |
| **DD-KA-018** | DetectedLabels detection specification — the 12 infrastructure characteristics KA computes and forwards via `DetectedLabelsJSON`. |
| **ADR-056** | Post-RCA label computation relocation (from signal source to remediation target). |

---

## Review & Evolution

**When to Revisit**:
- If LLM tool-calling patterns show the three-step flow is too many round trips for simple cases
- If DD-WORKFLOW-016 protocol changes (new steps, changed pagination behavior)

**Success Metrics**:
- Workflow selection accuracy vs. single-tool baseline (effectiveness scores from EM)
- No increase in `needs_human_review` rate

---

**Document Version**: 2.0
**Last Updated**: August 1, 2026
**Status**: ✅ APPROVED
**Authority**: KA workflow discovery integration (implements DD-WORKFLOW-016 protocol + ADR-056 label relocation)
**Confidence**: 90%

**Confidence Gap (10%)**:
- This rewrite condenses several Python-era procedural details (tool-instance lifecycle, exact session-state failure-mode table) that don't have a direct Go analog worth preserving; if a future reader needs that level of detail for the current implementation, it should come from reading `internal/kubernautagent/tools/custom/tools.go` and `internal/kubernautagent/investigator/investigator_discovery.go` directly rather than this document.
