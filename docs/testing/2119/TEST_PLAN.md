# Test Plan: #2119 — Same-Kind Validation Gate Retry Silently Drops RCA Confidence (v1.6 port of #2118)

## 1. Purpose

`sameKindValidationGate` (`internal/kubernautagent/investigator/investigator_gates.go`)
fires when the LLM's initial RCA `remediation_target.kind` equals the input
signal's own resource kind (a signal often propagating upward from a child
resource, e.g. a Pod OOM manifesting as Node `DiskPressure`). It sends a
correction prompt asking the LLM to re-evaluate whether a child resource is
the true root cause, then retries the LLM call with only `submit_result`
declared as a tool.

The gate already had a "lost field, keep original" guard for
`RemediationTarget.Kind`: if the retry response drops the target entirely,
the original result is kept rather than discarded. It had **no equivalent
guard for `Confidence`**. Because the correction prompt reads as a narrow
yes/no question ("is the target correct?"), the LLM frequently responds with
a minimal tool call that answers only that question and omits (or zeroes)
the separately-required `confidence` field. Since `sameKindValidationGate`
accepted `retryResult.Confidence` unconditionally whenever
`RemediationTarget.Kind` was non-empty, this silently overwrote a real,
previously-validated confidence score with `0.0` -- corrupting the
audit-visible RCA record.

This is the v1.6 port of the fix originally shipped for
[#2118](https://github.com/jordigilh/kubernaut/issues/2118) on
`release/v1.5` (v1.5.6, PR #2122). Root-cause analysis, the live-LLM spike,
and the chosen fix are identical; see #2118's own test plan for the spike
details. This document tracks the v1.6 port and its adapted test
scenarios (main's gate structure at port time differs from v1.5.6's:
`LLMInvocationContext` plumbing, a split `sameKindValidationGate`/
`retryForSameKind`, and post-#1578 Reasoning-block capture that #2118/#2120
predate).

### Scope

The same-kind (and API-version) gate retry occasionally calling an
undeclared tool instead of `submit_result` is a separate, related defect,
tracked and ported together with this fix as
[#2121](https://github.com/jordigilh/kubernaut/issues/2121) (v1.6 clone of
[#2120](https://github.com/jordigilh/kubernaut/issues/2120)).

## 2. Chosen Fix (defense-in-depth)

1. **Strengthened prompt** (`sameKindValidationGate`'s `correctionMsg`,
   built by `sameKindCorrectionMessage`): added a second paragraph
   instructing the LLM to resubmit a *complete* RCA result -- confidence
   genuinely recomputed (not a placeholder or default), severity, and
   causal_chain at the same rigor as the original submission -- regardless
   of which target it concludes is correct.
2. **Confidence-backfill guard** (the deterministic fix, applied in
   `finalizeSameKindRetry` immediately after the existing
   `RemediationTarget.Kind` guard):

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
(`Investigate()` -> `sameKindValidationGate` -> `retryForSameKind`); no new
production entry point.

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

(Test/scenario IDs retain the original `UT-KA-2118-*` numbering from the
upstream fix rather than being renumbered to `2119`, per this package's
convention of keeping scenario IDs stable across backports/ports so the same
spec can be cross-referenced from either issue.)

### Tier Coverage Rationale

- **UT** (via the real `Investigate()` entry point, following this
  package's established pattern in `apiversion_gate_test.go`) covers this
  fix completely: the bug is a pure business-logic property of
  `sameKindValidationGate`/`retryForSameKind`'s own retry-acceptance branch,
  fully reproducible with a mock `llm.Client` and no external dependencies.
- **IT/E2E**: not added net-new. The gate's wiring into the production
  `Investigate()` path already exists and is already exercised by the
  existing `apiversion_gate_test.go` suite plus this fix's own UT specs
  (which call through that same real entry point); this change modifies
  the gate's internal retry-acceptance behavior without adding a new wiring
  point.

## 5. Validation Results

- `go test ./internal/kubernautagent/investigator/...` -- pass, 366/366
  specs (including the 4 new UT-KA-2118-XXX specs above plus 6
  UT-KA-2120-XXX specs from the co-ported #2121 fix).
- `go test ./internal/kubernautagent/...` (full package) -- pass, no
  regressions.
- `golangci-lint run ./internal/kubernautagent/investigator/...` -- 0
  issues.
- `go build ./...` -- clean.
