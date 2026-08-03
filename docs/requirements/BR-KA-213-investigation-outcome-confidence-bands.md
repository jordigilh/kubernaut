# BR-KA-213: Operator-Configurable Investigation-Outcome Confidence Bands

**Business Requirement ID**: BR-KA-213
**Category**: AI (Kubernaut Agent)
**Priority**: P2
**Target Version**: V1.6
**Status**: ✅ Implemented
**Date**: 2026-07-29

---

## 📋 Business Need

### Problem Statement

[BR-KA-200](./BR-KA-200-resolved-stale-signals.md) defines KA's own, upstream investigation-quality
gate: before any workflow is ever selected, the LLM self-assesses whether its investigation reached
a credible conclusion at all, using two confidence bands:

- **Outcome A ("resolved")**: `investigation_outcome = "resolved"` when confidence `>= 0.7`
- **Outcome B ("inconclusive")**: `investigation_outcome = "inconclusive"` when confidence `< 0.5`

These bands were **hardcoded directly as literal prompt text** in
`internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl`, requiring a code change
and redeploy to adjust. This is distinct from
[BR-AI-088](./BR-AI-088-configurable-confidence-thresholds.md)'s AIAnalysis-owned, already
operator-configurable *approval* threshold, which applies later, to an already-selected workflow's
`confidence` — see BR-AI-088.4's note for how the two gates differ.

**Current Limitation (pre-#1826)**:
- KA's investigation-quality bar (`0.7`/`0.5`) is fixed for all operators and environments
- Different operators have legitimately different risk tolerances — a team running mostly
  well-instrumented production clusters might want a stricter (e.g. `< 0.6`) inconclusive bar so
  more low-certainty investigations route to human review, while a lab/dev environment might
  accept a looser bar to reduce review overhead
- Tuning requires a KA binary rebuild, unlike BR-AI-088's already-configurable AIAnalysis gate

### Business Value

| Benefit | Impact |
|---------|--------|
| **Risk Tuning** | Operators can raise the resolved-floor / inconclusive-ceiling for higher-stakes fleets |
| **Consistency** | Mirrors BR-AI-088's operator-configurability pattern for KA's own upstream gate |
| **No Redeploy** | Config-driven instead of requiring a prompt-template code change |

---

## 🎯 Requirements

### BR-KA-213.1: Configurable Confidence Bands

**MUST**: KA SHALL expose two independently configurable investigation-outcome confidence bands:

```yaml
# Kubernaut Agent config.yaml
ai:
  investigation:
    maxTurns: 40
    resolvedConfidenceThreshold: 0.7      # default matches pre-#1826 hardcoded value
    inconclusiveConfidenceThreshold: 0.5  # default matches pre-#1826 hardcoded value
```

**MUST**: Zero/absent values in YAML default to the V1.0 bands (`0.7`/`0.5`), preserving
byte-identical prompt output for operators who do not override them.

### BR-KA-213.2: Prompt Templating

**MUST**: `internal/kubernautagent/prompt/templates/phase3_workflow_selection.tmpl` SHALL
interpolate the configured (or defaulted) values instead of hardcoded literals, so the LLM is
always instructed with the operator's actual configured bar:

```
### Outcome A: Problem Self-Resolved
Set `selected_workflow` to None, `investigation_outcome` to "resolved" (confidence >= {{ .ResolvedConfidenceThreshold }}).

### Outcome B: Investigation Inconclusive
Set `selected_workflow` to None, `investigation_outcome` to "inconclusive" (confidence < {{ .InconclusiveConfidenceThreshold }}).
```

### BR-KA-213.3: Range and Ordering Validation

**MUST**: Both thresholds SHALL be validated to lie in `(0.0, 1.0]`.

**MUST**: `inconclusiveConfidenceThreshold` SHALL be strictly less than `resolvedConfidenceThreshold`
(no gray-zone inversion where a confidence value could satisfy neither, or both, band definitions).

### BR-KA-213.4: Boot-Time Configuration (Non-Hot-Reloadable)

**MUST NOT**: These bands are consumed at `Investigator` construction time
(`cmd/kubernautagent/bootstrap.go`), mirroring `ai.investigation.maxTurns`'s existing boot-time-only
configuration pattern. They are explicitly **not** part of KA's hot-reloadable LLM runtime config
(`LLMRuntimeConfig`) — changing them requires a KA restart, consistent with `maxTurns`.

### BR-KA-213.5: Helm Exposure

**SHOULD**: The chart SHALL expose both fields under `kubernautAgent.investigation.*`, rendered
conditionally (only when explicitly set) so the Go binary's own `DefaultConfig()` defaults apply
when the operator leaves them unset — mirroring the `aianalysis.rego.confidenceThreshold` pattern.

---

## 🔗 Out of Scope

- **`parser.go`'s `confidenceFloor = 0.8` constant** (used to floor confidence up for "not
  actionable" outcomes) is a separate concern from this BR's escalation-classification bands and is
  not addressed here.
- **Consolidating this gate into AIAnalysis**: this remains a KA-owned investigation-quality gate,
  just made configurable rather than hardcoded (see [BR-KA-200](./BR-KA-200-resolved-stale-signals.md)'s
  cross-reference note for why the two gates are intentionally separate).
- **Hot-reload support**: deferred; see BR-KA-213.4.

---

## ✅ Acceptance Criteria

### AC-1: Defaults Preserve V1.0 Behavior

```gherkin
Given a Kubernaut Agent config with no investigation confidence bands set
When the config is loaded
Then resolvedConfidenceThreshold SHALL default to 0.7
And inconclusiveConfidenceThreshold SHALL default to 0.5
And the rendered Phase 3 prompt SHALL be byte-identical to the pre-#1826 hardcoded text
```

### AC-2: Operator Override Reaches the Prompt

```gherkin
Given a Kubernaut Agent config with resolvedConfidenceThreshold=0.85 and inconclusiveConfidenceThreshold=0.3
When the Phase 3 workflow-selection prompt is rendered for an investigation
Then the prompt SHALL instruct "resolved" at confidence >= 0.85
And the prompt SHALL instruct "inconclusive" at confidence < 0.3
```

### AC-3: Validation Rejects Invalid Configuration

```gherkin
Given a Kubernaut Agent config
When resolvedConfidenceThreshold or inconclusiveConfidenceThreshold is outside (0.0, 1.0]
Or inconclusiveConfidenceThreshold >= resolvedConfidenceThreshold
Then config validation SHALL fail with an actionable error message
```

---

## 📎 Related Documents

- [BR-KA-200: Handling Inconclusive Investigations](./BR-KA-200-resolved-stale-signals.md) — defines
  the semantics and decision-tree consumption of these bands; this BR only makes the two numeric
  thresholds operator-configurable, with no behavioral change to BR-KA-200's downstream AIAnalysis
  decision tree.
- [BR-AI-088: Operator-Configurable Confidence Thresholds](./BR-AI-088-configurable-confidence-thresholds.md) —
  the separate, downstream AIAnalysis approval-threshold gate.

---

## Test Coverage

| Test ID | Layer | Scenario |
|---------|-------|----------|
| UT-KA-1826-001 | Unit (prompt) | Default bands (0.7/0.5) render unchanged when unset |
| UT-KA-1826-002 | Unit (prompt) | Operator-configured bands are templated into the prompt |
| UT-KA-1826-003a | Unit (config) | `DefaultConfig` sets the 0.7/0.5 V1.0 defaults |
| UT-KA-1826-003b | Unit (config) | YAML loading parses operator-overridden bands |
| UT-KA-1826-003c | Unit (config) | Validation rejects out-of-range or inverted bands |
| IT-KA-1826-001 | Integration | `investigator.Config` bands reach the real LLM system prompt via the production entry point (`RunWorkflowDiscoveryFromRCA`) |
| IT-PLATFORM-1826-001/002 | Integration (Helm) | `kubernautAgent.investigation.*` renders into the KA ConfigMap |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-29 | Initial business requirement and implementation (Issue #1826): made KA's investigation-outcome confidence bands operator-configurable instead of hardcoded in `phase3_workflow_selection.tmpl`. |
