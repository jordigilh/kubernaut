# Kubernaut Agent - Business Requirements

**Version**: 1.0
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

This is a catalog/index over KA's business requirement documents. Unlike some peer services
(e.g. Data Storage), KA's BRs are **individually filed** under `docs/requirements/BR-KA-*.md` rather
than embedded inline in this file — this document indexes those files with status, a one-line
summary, and a pointer to test coverage (see [BR_MAPPING.md](./BR_MAPPING.md) for the full
BR-to-test-file traceability).

---

## BR Catalog

| BR | Title | Status | Summary |
|---|---|---|---|
| [BR-KA-191](../../../requirements/BR-KA-191-workflow-parameter-validation.md) | Workflow Parameter Validation with Self-Correction | ✅ Approved (implemented) | KA validates the LLM's proposed workflow/parameters in Go against catalog constraints and re-prompts on failure — no LLM tool-calling involved. |
| [BR-KA-193](../../../requirements/BR-KA-193-execution-outcome-reporting.md) | Execution Outcome Reporting | ✅ Approved | Reports workflow-execution outcomes back through the investigation lifecycle. |
| [BR-KA-195](../../../requirements/BR-KA-195-cost-tracking-metrics.md) | LLM Cost Tracking Metrics | 🟡 Pending (V2.0 enhancements) | V1 cost-counter slice is approved under [BR-KA-OBSERVABILITY-001](../../../requirements/BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md); full cost-tracking is deferred. |
| [BR-KA-197](../../../requirements/BR-KA-197-needs-human-review-field.md) | Human Review Required Flag | ✅ Approved | LLM-reported confidence bands (0.5 inconclusive / 0.7 resolved) drive a `needs_human_review` flag in the parsed investigation outcome. |
| [BR-KA-200](../../../requirements/BR-KA-200-resolved-stale-signals.md) | Handling Inconclusive Investigations | ✅ Implemented | Defines KA/AIAnalysis behavior when an investigation cannot reach a confident conclusion (e.g. the signal's underlying condition has already resolved). |
| [BR-KA-211](../../../requirements/BR-KA-211-llm-input-sanitization.md) | LLM Input Sanitization | ✅ Implemented (Go), tracked gaps remain | Multi-stage credential/secret redaction pipeline before any data is sent to the LLM. |
| [BR-KA-212](../../../requirements/BR-KA-212-rca-target-resource.md) | RCA Target Resource in Root Cause Analysis | ✅ Approved | The LLM-determined remediation target is validated against the Kubernetes owner-reference chain and may differ from the original signal's resource. |
| [BR-KA-261](../../../requirements/BR-KA-261-llm-provided-affected-resource.md) | LLM-Provided Affected Resource with Owner Resolution | ✅ Approved | Owner-chain resolution for the LLM-provided affected resource. |
| [BR-KA-263](../../../requirements/BR-KA-263-conversation-continuity.md) | Conversation Continuity Across Investigation Phases | ✅ Approved | Preserves LLM conversation/session context across the RCA, workflow-discovery, and validation phases. |
| [BR-KA-264](../../../requirements/BR-KA-264-post-rca-label-detection.md) | Post-RCA Infrastructure Label Detection via EnrichmentService | ✅ Approved | Detects 12 infrastructure characteristics of the root-owner resource after RCA completes (see [DD-KA-018](../../../architecture/decisions/DD-KA-018-detected-labels-detection-specification.md)). |
| [BR-KA-265](../../../requirements/BR-KA-265-labels-in-workflow-discovery.md) | Infrastructure Labels in Workflow Discovery Context | ✅ Approved | Detected infrastructure labels are threaded into workflow-catalog discovery/filtering. |
| [BR-KA-OBSERVABILITY-001](../../../requirements/BR-KA-OBSERVABILITY-001-agent-prometheus-metrics.md) | Kubernaut Agent Prometheus Metrics | ✅ Approved | Baseline Prometheus metrics surface (`aiagent_*` namespace) — see [metrics-slos.md](./metrics-slos.md). |
| [BR-KA-OBSERVABILITY-002](../../../requirements/BR-KA-OBSERVABILITY-002-verification-step-events.md) | Verification Step Events for Console Activity Log | ✅ Approved | Emits verification-step events consumable by the operator-facing console activity log. |
| [BR-AUDIT-011](../../../requirements/BR-AUDIT-011-kubernautagent-secret-read-audit.md) | KubernautAgent Secret Read Audit (Detective Control) | ✅ Implemented | Every Kubernetes Secret Get/List by KA's tools emits a dedicated `aiagent.secret.accessed` audit event (detective control, RBAC intentionally not narrowed — see [security-configuration.md](./security-configuration.md)). |

**Not yet formalized as a standalone BR** (tracked elsewhere): confidence-band configurability is
tracked directly by [Issue #1826](https://github.com/jordigilh/kubernaut/issues/1826) (KA-side) and
[Issue #1828](https://github.com/jordigilh/kubernaut/issues/1828) (AIAnalysis-side) rather than a BR
document, as of this writing.

---

## Summary Statistics

| Metric | Value |
|---|---|
| Total BR documents | 14 |
| Approved / Implemented | 13 |
| Pending (deferred to V2.0) | 1 (BR-KA-195) |

---

## Related Documentation

- [BR_MAPPING.md](./BR_MAPPING.md) — BR-to-test-file traceability
- [overview.md](./overview.md) — architecture and design decisions referenced by these BRs
- `docs/requirements/` — full requirements catalog for all Kubernaut services
