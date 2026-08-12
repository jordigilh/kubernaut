# Test Plan: #2118 — Same-Kind Validation Gate Retry Silently Drops RCA Confidence

## 1. Purpose

`sameKindValidationGate` (`internal/kubernautagent/investigator/investigator_gates.go`)
fires when the LLM's initial RCA `remediation_target.kind` equals the input
signal's own resource kind (a signal often propagating upward from a child
resource, e.g. a Pod OOM manifesting as Node `DiskPressure`). It sends a
correction prompt asking the LLM to re-evaluate whether a child resource is
the true root cause, then retries the LLM call with only `submit_result`
declared as a tool.

The gate already had a "lost field, keep original" guard for
`RemediationTarget.Kind` (added for a prior issue): if the retry response
drops the target entirely, the original result is kept rather than
discarded. It had **no equivalent guard for `Confidence`**. Because the
correction prompt reads as a narrow yes/no question ("is the target
correct?"), the LLM frequently responds with a minimal tool call that
answers only that question and omits (or zeroes) the separately-required
`confidence` field. Since `sameKindValidationGate` accepted
`retryResult.Confidence` unconditionally whenever `RemediationTarget.Kind`
was non-empty, this silently overwrote a real, previously-validated
confidence score with `0.0` -- corrupting the audit-visible RCA record and,
by extension, `investigator_phases.go`'s `MergePhase1Fallbacks`, which only
backfills Phase 3's confidence from Phase 1's (`p1.Confidence`) when Phase 3
itself is `0` -- it has no way to know Phase 1's own value was already
corrupted by the gate before it ever reached that merge step.

### Live-LLM Spike (completed prior to implementation)

Before implementing, a throwaway Go program (`cmd/spike2118main/`, deleted
after the run) probed the real `claude-sonnet-5` model on Vertex AI (the
same auth path production uses) with the actual `sameKindCorrectionMessage`
wording, a realistic multi-turn synthetic investigation history, and only
`submitOnlyRCATools()` declared -- matching production's exact request
shape -- across 8 trials per wording variant (current vs. a strengthened
candidate).

**Finding 1 (confirms the bug is real, not theoretical):** one old-wording
trial reproduced the exact #2118 symptom live: the model's `submit_result`
tool call omitted `confidence` entirely while `due_diligence.target_accuracy`
was fully and coherently populated -- i.e. the model treated the retry as
"just confirm the target," exactly as the root-cause theory predicted.

**Finding 2 (limits how much weight prompt wording alone can carry):** in
both variants, the model frequently (4/8 old, 7/8 new trials) called a tool
name that was not in the declared `Tools` list at all, instead of
`submit_result`. This is a separate, pre-existing reliability gap (filed
separately, see Scope below), but it also means the clean, comparable
sample for the wording A/B was too small to validate the wording change on
its own. **Conclusion**: ship the strengthened wording as a best-effort,
defense-in-depth improvement, but treat the deterministic code-level
backfill guard as the actual, load-bearing fix -- mirroring the project's
dual-layer approach to #2110/#2111 (fix the source, then guard the
boundary).

### Scope

A secondary finding from the spike -- the same-kind (and API-version) gate
retry sometimes calls an undeclared tool instead of `submit_result`, so the
correction is silently skipped entirely -- is out of scope for this fix and
tracked separately: [#2120](https://github.com/jordigilh/kubernaut/issues/2120)
(v1.5.6) / [#2121](https://github.com/jordigilh/kubernaut/issues/2121) (v1.6
clone).

## 2. Chosen Fix (defense-in-depth)

1. **Strengthened prompt** (`sameKindValidationGate`'s `correctionMsg`):
   added a second paragraph instructing the LLM to resubmit a *complete*
   RCA result -- confidence genuinely recomputed (not a placeholder or
   default), severity, and causal_chain at the same rigor as the original
   submission -- regardless of which target it concludes is correct.
2. **Confidence-backfill guard** (the deterministic fix, applied
   immediately after the existing `RemediationTarget.Kind` guard):

```go
if retryResult.Confidence <= 0 && result.Confidence > 0 {
    inv.logger.Info("same-kind validation gate: retry lost confidence, keeping original",
        "original_confidence", result.Confidence, "correlation_id", correlationID)
    retryResult.Confidence = result.Confidence
}
```

   Unlike the `RemediationTarget.Kind` guard (which discards the entire
   retry on loss), this backfills only the regressed field -- other
   genuinely new retry content (e.g. an updated
   `due_diligence.target_accuracy` narrative) is preserved rather than
   discarded.

No Wiring Manifest row: this modifies existing, already-wired gate logic
(`Investigate()` -> `sameKindValidationGate`); no new production entry point.

## 3. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-10** | Information Input/Output Validation | A retry's `confidence` value must be validated against regression, not blindly trusted just because it satisfies schema bounds (`[0,1]`, `parser/schema.go`). |
| **AU-3** | Content of Audit Records | Confidence surfaced to operators/Console and persisted in the audit-visible RCA record must reflect genuine RCA certainty, not a silently-dropped placeholder. |

## 4. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-KA-2118-001 (regression) | Unit (exercised through the real `Investigate()` production path) | Same-kind gate retry confirms the same target but the tool call arguments omit confidence entirely; the pre-retry confidence (0.88) must be backfilled through to the final merged result, not silently dropped to 0 | SI-10, AU-3 | `internal/kubernautagent/investigator/samekind_confidence_2118_test.go` |
| UT-KA-2118-002 | Unit | Retry genuinely recomputes a different, non-zero confidence (0.55); the gate must preserve it unchanged, not overwrite it with the pre-retry value | SI-10 | same file |
| UT-KA-2118-003 (edge case) | Unit | The pre-retry confidence was itself never set (0) and the retry also omits it; the guard must not fabricate a non-zero confidence out of another zero value | SI-10 | same file |
| UT-KA-2118-004 | Unit | The correction message sent to the LLM must explicitly instruct it to restate confidence as part of a full RCA restatement, not just confirm/deny the target | AU-3 | same file |

### Tier Coverage Rationale

- **UT** (via the real `Investigate()` entry point, following this
  package's established pattern in `apiversion_gate_test.go` and
  `gate_history_propagation_1935_test.go`) covers this fix completely: the
  bug is a pure business-logic property of `sameKindValidationGate`'s own
  retry-acceptance branch, fully reproducible with a mock `llm.Client` and
  no external dependencies. UT-KA-2118-001/003 specifically route the
  synthetic workflow-selection response's own confidence to `0` (via
  `gateWfToolResp` omitting `confidence`) so the assertion exercises
  `investigator_phases.go`'s `MergePhase1Fallbacks` backfill from Phase 1's
  post-gate `p1.Confidence` -- isolating whether the gate itself preserved
  or corrupted that value, independent of Phase 3's own confidence.
- **IT/E2E**: not added net-new. The gate's wiring into the production
  `Investigate()` path already exists and is already exercised by the
  existing `apiversion_gate_test.go`/`gate_history_propagation_1935_test.go`
  suites plus this fix's own UT specs (which call through that same real
  entry point); this change modifies the gate's internal retry-acceptance
  behavior without adding a new wiring point.

## 5. Validation Results

- `go test ./internal/kubernautagent/investigator/...` -- pass (including
  the 4 new UT-KA-2118-XXX specs above; UT-KA-2118-001/004 failed against
  the unmodified gate, confirming RED before the fix was applied).
- `go test ./internal/kubernautagent/...` (full package) -- pass, no
  regressions.
- `golangci-lint run internal/kubernautagent/investigator/...` -- 0 issues.
- `gofmt -l` -- clean.
