# DD-AF-016: Explicit Alert-Rule Severity Source for AF Remediation Requests

**Status**: ✅ Accepted
**Date**: 2026-09-27
**Issue**: #2467
**Related**: [BR-SEVERITY-001](../../requirements/BR-SEVERITY-001-standardized-severity-levels.md), [DD-SEVERITY-001](DD-SEVERITY-001-severity-determination-refactoring.md), [DD-AF-010](DD-AF-010-remove-ungrounded-severity-inference.md), [DD-AF-012](DD-AF-012-confidence-gated-severity-correlation.md)

---

## Context

API Frontend (AF) must put a source severity on a `RemediationRequest` before
SignalProcessing (SP) can apply the operator's Rego policy. That source value
selects the signal's eventual workflow, so AF must not invent or silently
replace it.

Prometheus alerts and alerting rules can carry external severity labels in
operator-defined schemes. SP Rego, not AF, owns mapping those source labels to
the canonical severity used by downstream workflow selection. The AF triage
implementation historically violated this boundary in two ways:

- Tier 2.5 asked an LLM to infer severity from a matching rule's name,
  expression, and annotations when the rule did not yield evaluable data.
- AF's `NormalizeSeverity` converted unrecognized non-empty values to
  `warning`, masking a source value that the operator's Rego policy should
  handle (or reject).

DD-AF-010 removed Tier 3, the ungrounded LLM fallback with no alert or rule,
but explicitly retained Tier 2.5. This decision closes that remaining
LLM-inference path while retaining grounded, rule-backed investigation when
the source severity is explicit and unambiguous.

## Alternatives Considered

### Alternative A — Keep Tier 2.5 LLM inference with matching-rule context — ❌ Rejected

Continue allowing the LLM to derive severity from rule text when no explicit
severity value is available.

**Rejected because** the model cannot reliably reconstruct the operator's
severity-to-workflow policy from rule prose. A plausible but incorrect value
can select the wrong remediation workflow. Rule context is not a substitute
for the severity label that the operator chose.

### Alternative B — Require an already-firing alert; reject all rule-only investigations — ❌ Rejected

Permit RR creation only when a firing or pending alert instance exists.

**Rejected because** an operator may intentionally investigate a potential
issue before an alert fires when a matching alerting rule already defines its
severity. The rule-only path is safe when its explicit source value is
unambiguous; ambiguous candidates can fail closed until the alert becomes
pending or firing.

### Alternative C — Use explicit alert/rule values only; fail closed otherwise — ✅ Chosen

AF uses a non-empty severity label carried by a correlated firing alert,
pending rule, or matching Prometheus rule. For the rule-only potential-issue
path, every relevant candidate rule must carry the same non-empty raw severity
value. AF passes that exact value to the RR without LLM inference, local
canonicalization, or a default.

If no alert or rule correlates, a required severity label is missing, or
candidate rules disagree, AF returns `ErrSeverityUndetermined` and creates no
RR. The user may retry when a specific alert becomes pending or firing. SP
Rego later maps the raw value to canonical classifications or fails SP
classification if it cannot produce usable required outputs.

## Decision

1. **Severity provenance is explicit.** AF may resolve RR severity only from
   the `severity` label on a correlated Prometheus alert or rule. All matching
   alerts in the selected specificity/state bucket must agree on one non-empty
   raw value, just as matching rule candidates must; AF does not rank external
   labels using its canonical severity table. The LLM is not a severity source.
2. **Rule-only candidates must agree.** When no firing/pending alert instance
   identifies the source and AF is using matching rules for a potential-issue
   investigation, every relevant matching rule must have a non-empty severity
   label and all values must match exactly. Missing or conflicting labels are
   ambiguous; AF fails closed rather than choosing a rule or value.
3. **No local normalization or fallback.** AF preserves the raw source label
   unchanged. It must not map arbitrary values to `warning`, infer from
   descriptions, or synthesize a value. An absent source severity means AF
   cannot create an RR.
4. **SP owns canonical classification.** The RR's raw external severity is
   input to SP. Operator Rego maps it to canonical severity and the other
   workflow-driving classifications. If SP cannot produce usable required
   outputs, SP fails classification; downstream services do not repair the
   result.
5. **Scope remains bounded.** The #1017 manual/pre-SP severity path remains
   distinct. This decision does not change KA's RCA `signal_name` finding or
   DD-AF-012's guard for cluster-scoped ambiguous alert correlation.

## Supersession and Authority

- This decision **supersedes only DD-AF-010's allowance for Tier 2.5 LLM
  severity inference**. DD-AF-010's removal of Tier 3 and its no-alert/no-rule
  fail-closed behavior remain in force.
- DD-AF-012's cluster-scoped ambiguity and user-confirmation behavior remains
  in force. Its historical note that confidence came from a Tier 2.5 LLM is
  superseded; confidence is not an authority for severity.
- The issue #92 test plan's Tier 2.5 LLM acceptance criteria are historical
  and superseded for current behavior. The active contract is this decision,
  BR-SEVERITY-001, and the #2467 test plan.
- BR-SEVERITY-001 and DD-SEVERITY-001 remain authoritative for canonical
  severity semantics and SP/Rego normalization.

## Consequences

**Positive**:
- Prevents LLM- or Go-invented severity from steering workflow selection.
- Preserves the operator's exact source value for the SP policy that owns
  normalization.
- Makes missing, unmapped, and ambiguous policy inputs observable instead of
  disguising them as `warning`.
- Allows a potential-issue investigation when a matching rule supplies one
  unambiguous explicit severity.

**Negative**:
- AF-driven RR creation fails when an alert/rule lacks a severity label or
  when candidates in the selected bucket disagree. Operators must correct
  alert/rule metadata or the caller must retry after a specific alert becomes
  pending/firing.
- The LLM-based Tier 2.5 severity path, its confidence threshold, and its
  associated provider dependency are no longer part of severity triage.

## Verification

The #2467 test plan adds unit and integration coverage proving exact raw-label
pass-through, no warning fallback, no LLM call, no RR for missing or
conflicting rule severity, and successful SP normalization/failure after the
RR crosses the AF→SP boundary.
