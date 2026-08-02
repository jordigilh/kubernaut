# DD-AF-010: Remove Ungrounded LLM Severity Inference (Tier 3)

**Status**: ✅ Accepted
**Date**: 2026-08-01
**Author**: AI Assistant
**Related**: Issue [#1839](https://github.com/jordigilh/kubernaut/issues/1839), issue #92 (original three-tier design), PR #1830

---

## Context

API Frontend's severity-triage pipeline (`pkg/apifrontend/severity/triage.go`)
resolves the severity of a `RemediationRequest` before creation via a tiered
pipeline:

- **Tier 1 / 1.5**: a real Prometheus alert (firing or pending) correlates to
  the target resource.
- **Tier 2**: a Prometheus rule correlates to the resource and its expression
  is live-evaluated against current metrics.
- **Tier 2.5**: a Prometheus rule correlates to the resource, but the data
  needed to evaluate it is unavailable — the LLM classifies severity *using
  that rule's name, expression, and annotations as context*.
- **Tier 3 (removed by this decision)**: no alert and no rule correlate to
  the resource at all. The LLM was asked to classify severity from
  `namespace/kind/name/description` alone — zero confirming evidence of any
  kind.

Tier 3 fired whenever a `kubernaut_investigate`/`kubernaut_remediate` call
targeted a resource with no matching alert or rule, which is the common case
for direct-resource investigation (not an edge case) in any realistic
deployment with Prometheus and an agent LLM configured — i.e. the default.

This was surfaced during IT-coverage triage for PR #1830 (severity triage's
"no alert investigation" path lacked integration coverage), which led to
tracing `severityTriage.enabled`/`PromClient` coupling end-to-end and
discovering Tier 3's design.

### Why this is a problem, not just a design quirk

1. **The output is indistinguishable from a real signal.** `resolveCreateRRSeverity`
   writes the Tier 3 result into `RemediationRequest.Spec.Severity` with
   `SignalType: "alert"` hardcoded regardless of tier. Nothing downstream —
   the CRD, KA, or the workflow catalog — can tell a Tier 3 guess apart from
   a genuine Alertmanager-sourced severity (only an internal
   `signalLabels["severity_source"]` annotation differs, which is not part of
   workflow-selection logic).
2. **It drives workflow selection.** KA's workflow-catalog filter
   (`internal/kubernautagent/workflowcatalog/cache_filter.go`) matches
   candidate workflows against `RemediationRequest.Spec.Severity`. Workflow
   label-selectors are authored by users against *real* alert semantics
   (their own Prometheus rules' `severity` labels and naming conventions).
   An LLM given only a resource's namespace/kind/name/description has no
   grounds to reconstruct semantics that only exist in rules it has never
   seen (because none matched). A wrong guess can steer remediation toward
   the wrong workflow for the actual incident.
3. **It was confirmed as intentional, not an oversight.** The original
   test plan (`docs/services/apifrontend/tests/92/test_plan.md`, `BAC-T-06`)
   documents Tier 3 as "Pure LLM (Tier 3) used when no rules cover the
   target resource", and `NewTriager` explicitly panics if `llm` is nil with
   the stated rationale "the pipeline requires an LLM fallback to guarantee a
   result" — i.e. Tier 3 was designed to *always* produce an answer. The
   downstream workflow-selection risk was not accounted for at design time.

## Alternatives Considered

### Alternative A — Config toggle, default off — ❌ REJECTED

Add a `severityTriage.allowUngroundedInference` (or similar) flag, defaulting
to `false`, gating Tier 3. Operators who accept the risk could re-enable it.

**Rejected because**: this was the first proposal, but on reflection there is
no scenario where inventing a severity with zero evidence is a *safe*
capability to keep available, even opt-in. Workflow selection depends on
severity being a faithful signal, not a plausible-sounding LLM guess.
Keeping the code path alive (even off by default) preserves the risk for
any operator who flips it, and adds a permanent maintenance/test surface for
a capability with no safe use case. The user explicitly rejected this
framing: *"removing the code is the safest bet... I don't see how we can let
an LLM determine the initial categories on the fly if none exist to derive
from."*

### Alternative B — Default severity instead of an LLM guess — ❌ REJECTED

When Tier 1/1.5/2/2.5 all miss, fall back to a fixed default (e.g.
`"warning"`) instead of calling the LLM.

**Rejected because**: a silent fixed default is just a *different* kind of
fabrication — it still writes a confident-looking severity onto the RR with
no evidence behind it, and downstream workflow selection still can't tell it
apart from a real signal. It trades "LLM hallucination" for "silent
mislabeling," without solving the actual problem (deciding remediation
priority/workflow without real grounds).

### Alternative C — Remove Tier 3 entirely; fail closed with a clear error — ✅ CHOSEN

Delete Tier 3 (`runTier3`, and `TriagePure` from the `LLMTriager` interface
and all four implementations). When Tier 1/1.5/2/2.5 all miss, `Triage()`
returns `severity.ErrSeverityUndetermined` instead of a result.
`resolveCreateRRSeverity`/`HandleCreateRR` already abort RR creation on any
`Triage()` error (pre-existing code path, unchanged), and per issue #1658's
prompt mandate the agent's LLM translates the tool error into a
natural-language explanation for the user instead of silently creating an RR
with a fabricated severity.

**Pros**: Closes the workflow-mismatch risk completely rather than bounding
it. No new config surface, no new maintenance burden, no "safe until someone
flips the flag" residual risk. The user gets an honest, actionable answer
("I can't determine severity for this resource — no alert or rule
correlates to it") instead of either a hallucination or a silent guess.

**Cons**: `kubernaut_investigate`/`kubernaut_remediate` on a resource with no
alert or rule coverage now fails outright instead of producing a
(low-confidence) result. This is a deliberate behavior change — see
Consequences.

## Decision

**Alternative C**: Tier 3 is removed. Tier 1/1.5/2 (real alert/rule
evidence) and Tier 2.5 (a real, label-correlated Prometheus rule exists,
just not currently true — there is something concrete to derive from) are
unaffected. `severity.ErrSeverityUndetermined` is the new fail-closed
signal when nothing correlates.

## Consequences

**Positive**:
- Eliminates a real, currently-shipping risk of LLM-fabricated severities
  silently steering remediation-workflow selection.
- Simpler pipeline (four tiers instead of five) and interface (`LLMTriager`
  now has one method).
- Failure is explicit and user-facing rather than a silent low-confidence
  guess baked into a CRD.

**Negative**:
- `kubernaut_investigate`/`kubernaut_remediate` against a resource with no
  Prometheus alert or rule coverage now fails instead of creating an RR with
  a best-effort severity. Operators whose resources lack any Prometheus rule
  coverage will need to add one (even a non-firing rule with a `severity`
  label) for AF-driven remediation to work, or invoke the K8s-native tools
  directly without going through severity-gated RR creation.
- `NoopLLMTriager` (used when no LLM is configured at all) still satisfies
  `LLMTriager` for Tier 2.5, but Tier 2.5 now can't be exercised without at
  least one correlated rule — this was already true before this change.

**Neutral**:
- `BuildTriagePrompt`'s no-rules code path (`types.go`) is retained as a
  general-purpose prompt-builder capability, even though production no
  longer calls it with empty rules (only Tier 2.5 calls it now, always with
  non-empty matched rules). Low-risk to keep; removing it added no safety
  value.

## Authority

Issue #1839, issue #92 (original three-tier test plan, `BAC-T-06`),
`pkg/apifrontend/severity/triage.go`, `pkg/apifrontend/tools/af_create_rr.go`.
