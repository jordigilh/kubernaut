# DD-KA-004: V1.0 Confidence Scoring Methodology

**Design Decision ID**: DD-KA-004
**Status**: ✅ APPROVED (superseded methodology) / 🔄 IMPLEMENTED DIFFERENTLY
**Version**: 2.0
**Created**: December 2, 2025
**Last Updated**: 2026-08-01

---

> **Note (2026-08-01, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: Renamed `DD-HAPI-003` → `DD-KA-004` (the ID `DD-HAPI-003` was reused by two unrelated Python-era documents — the other one, on mandatory OpenAPI client usage, is now [DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md)). This document originally proposed a `0.7 * semantic_similarity + 0.3 * label_match_ratio` formula. **That formula was never implemented in the Go rewrite.** What actually shipped is Alternative 2 from this very document ("LLM Self-Assessed Confidence") — the exact alternative this DD rejected in December 2025. This rewrite documents the current Go reality and keeps the original decision record below for historical context.

---

## Current Implementation (Go, KA) — Supersedes the V1.0 Methodology Below

**Confidence is 100% LLM self-reported.** There is no semantic-similarity or label-matching formula in the Go codebase. KA parses `confidence` directly out of the LLM's structured JSON tool-call output and passes it through with only bounds validation:

1. The prompt template instructs the LLM to self-calibrate: *"Confidence Calibration: How was your confidence score determined? Start at 1.0 and list each factor that reduced it..."* (`internal/kubernautagent/prompt/templates/incident_investigation.tmpl`).
2. The LLM returns `confidence` (top-level, for RCA) and `selected_workflow.confidence` in its JSON response (`internal/kubernautagent/parser/parser_llm_types.go`).
3. `parser.applyLLMWorkflow` copies it verbatim onto `katypes.InvestigationResult.Confidence` (`internal/kubernautagent/parser/parser_format.go`) — no derivation logic.
4. `validator.Validate` only checks `0 <= confidence <= 1` (`internal/kubernautagent/parser/validator.go`); a defense-in-depth floor of 0.8 is applied only when the LLM marks the result `actionable=false` (issue #607, `parser_format.go`).
5. The value flows unchanged through `mcp.WorkflowDiscoveryResult`, `AIAnalysis.Status.SelectedWorkflow.Confidence`, and into Rego as `input.confidence` (`pkg/aianalysis/handlers/response_processor.go`, `pkg/aianalysis/handlers/analyzing.go`).
6. Exception: the interactive MCP path where a human operator manually selects a workflow hardcodes `Confidence: 1.0` (`internal/kubernautagent/investigator/select_workflow.go`), since a human decision replaces LLM uncertainty.

**Semantic similarity / embeddings**: never implemented. [DD-WORKFLOW-015](DD-WORKFLOW-015-v1-0-label-only-architecture.md) explicitly approved "label-only matching" for V1.0 and deferred embedding-based semantic search indefinitely. There is no `pgvector`, no embedding generation, and no cosine-similarity code anywhere in `internal/kubernautagent/`.

**Label matching**: a label-based score does exist (`finalScore` in `internal/kubernautagent/workflowcatalog/cache_filter.go`, formula `(5.0 + detectedBoost + customBoost - penalty) / 10.0` per [DD-WORKFLOW-004](DD-WORKFLOW-004-hybrid-weighted-label-scoring.md)), but it is **not** the LLM-reported `Confidence`. It is used only to sort/order the workflow candidate list KA hands to the LLM during discovery (`internal/kubernautagent/workflowcatalog/discovery_cache.go`); the score is discarded after sorting and never reaches the final `Confidence` field.

**Historical success rate**: confirmed excluded from scoring (as this document originally intended), but more thoroughly than described below — the "human display only" telemetry fields (`TotalExecutions`, `SuccessfulExecutions`, `ActualSuccessRate`) were deleted from the workflow catalog model entirely, not merely excluded from the confidence formula (see [DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md)).

**Approval thresholds — two gates, not one**:
- **Gate 1 (hard floor, Go, non-configurable in V1.0)**: AIAnalysis rejects workflow resolution before Rego ever runs if `confidence < 0.7` ([BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md) AC-4, `pkg/aianalysis/handlers/response_processor.go`). A TODO in the code notes this should become configurable per [BR-KA-198](../../requirements/BR-KA-198-configurable-confidence-thresholds.md) in V1.1.
- **Gate 2 (soft, Rego, operator-configurable)**: if Gate 1 passes, AIAnalysis forwards `confidence` to whatever Rego policy the operator deploys via Helm (`aianalysis.policies.content`). The repository's reference/test policy (`pkg/aianalysis/testdata/policies/approval.rego`, referenced by [REGO_POLICY_EXAMPLES.md](../../services/crd-controllers/02-aianalysis/REGO_POLICY_EXAMPLES.md)) defaults `confidence_threshold` to 0.80, overridable via `input.confidence_threshold`.
- **"Production always requires approval regardless of confidence" is no longer accurate.** In the reference policy, production only forces approval when confidence is below threshold, the target is a sensitive kind (`Node`/`StatefulSet`), the action is infrastructure-provisioning, or the target is missing — a high-confidence, non-sensitive production workflow can auto-approve. This is policy-defined, not hardcoded, so it is technically deployment-specific, but the shipped reference implementation diverges from this document's original "always" claim.

---

## Original V1.0 Decision Record (Historical — Superseded by the Go Implementation Above)

The following sections describe the **originally proposed methodology** as approved for the (Python) HolmesGPT-API in December 2025. They are preserved for historical traceability. They do **not** describe the current Go behavior — see "Current Implementation" above.

### Context & Problem (as originally written)

HolmesGPT-API needed to return a confidence score for workflow recommendations. The question was: what factors should contribute to this confidence score in V1.0?

**Key Requirements** (as originally written):
- Score must be meaningful for AIAnalysis Rego policy evaluation
- Score must be explainable in the `rationale` field
- Implementation must be achievable in V1.0 timeline

### Originally Proposed V1.0 Formula (Never Implemented)

**Originally approved** (but not carried into the Go rewrite): V1.0 confidence scoring using only two factors:

1. **Semantic Similarity** (primary factor) — cosine similarity between incident embedding and workflow description embedding, weight 70%
2. **Label Matching** (secondary factor) — percentage of `DetectedLabels` matching workflow requirements, weight 30%

```
confidence = (semantic_similarity * 0.7) + (label_match_ratio * 0.3)
```

### What Was NOT Included in V1.0 (Still True Today)

**Historical Success Rate** was explicitly excluded from the confidence score, for reasons that still hold in the Go implementation:
1. No execution data at initial deployment
2. Circular dependency — can't have success rates without executions
3. Avoiding bias toward frequently-used (vs. newer, potentially better) workflows

This exclusion held; see "Current Implementation" above for how much further the Go rewrite took it (the display-only telemetry fields were deleted, not just excluded from scoring).

### Alternatives Considered (Historical)

**Alternative 1: Include Historical Success Rate** — Rejected (no historical data in V1.0, adds complexity, creates workflow bias). Still true in Go.

**Alternative 2: LLM Self-Assessed Confidence** — Rejected at the time as "unreliable, not reproducible, hard to threshold." **This is exactly what the Go implementation does today** — see "Current Implementation" above. The original rejection rationale was not revisited in writing when the Go rewrite adopted this approach; if the tradeoff is intentional (e.g., LLM self-assessment proved adequate with the calibration prompt language and the 0.7/0.80 gates as guardrails), that decision should be documented explicitly rather than left as an undocumented reversal. Tracked as a gap for follow-up rather than resolved here.

**Alternative 3: Static Confidence (Always 0.85)** — Rejected (not useful for Rego policies, doesn't differentiate recommendations). Still not used.

### Original Rationale Field Guidance

The `rationale`/reasoning explanation is still expected to explain the confidence score in business terms (now driven by the LLM's own calibration narrative per the prompt template, rather than a fixed formula description).

---

## Related Documents

- **[DD-KA-003](DD-KA-003-mandatory-openapi-client-usage.md)**: Mandatory OpenAPI Client Usage (the other, unrelated document that used to share the `DD-HAPI-003` ID)
- **[DD-WORKFLOW-015](DD-WORKFLOW-015-v1-0-label-only-architecture.md)**: V1.0 label-only architecture (formally deferred embedding-based semantic search)
- **[DD-WORKFLOW-004](DD-WORKFLOW-004-hybrid-weighted-label-scoring.md)**: Hybrid weighted label scoring (the catalog-ranking heuristic, distinct from `Confidence`)
- **[DD-WORKFLOW-019](DD-WORKFLOW-019-ka-owned-workflow-discovery.md)**: KA-owned workflow discovery (confirms removal of success-rate telemetry)
- **[BR-KA-197](../../requirements/BR-KA-197-needs-human-review-field.md)**: Needs-human-review field (AC-4, the 0.7 hard floor)
- **[BR-KA-198](../../requirements/BR-KA-198-configurable-confidence-thresholds.md)**: Configurable confidence thresholds (V1.1 TODO to make the 0.7 floor configurable)
- **[REGO_POLICY_EXAMPLES.md](../../services/crd-controllers/02-aianalysis/REGO_POLICY_EXAMPLES.md)**: Points to `pkg/aianalysis/testdata/policies/approval.rego` as the canonical reference policy
- **DD-WORKFLOW-001**: Mandatory label schema

---

## Version History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-02 | HAPI Team | Initial creation — V1.0 semantic-similarity + label-match methodology proposed |
| 2.0 | 2026-08-01 | Docs cleanup ([Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806)) | Renamed `DD-HAPI-003` → `DD-KA-004`; documented that the Go implementation never adopted the proposed formula and instead uses LLM self-reported confidence (the originally rejected Alternative 2); corrected the approval-threshold and production-always-approval claims against the current two-gate (0.7 hard floor + 0.80 Rego default) reality |
