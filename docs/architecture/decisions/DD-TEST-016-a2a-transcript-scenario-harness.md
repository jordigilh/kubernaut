# DD-TEST-016: Explicit Transcript Scenarios for A2A E2E Tests

**Status**: Approved and implementing
**Date**: 2026-09-12
**Version**: 1.0
**Scope**: Mock LLM A2A E2E test harness
**Related**: DD-TEST-011, DD-TEST-002, DD-AA-KA-001, DD-AF-004

## Business Requirements

- **BR-TESTING-001**: Mock LLM responses must faithfully model the tested interaction contract.
- **BR-INTERACTIVE-001**: Interactive investigations must expose the intended sequential tool flow.
- **BR-INTERACTIVE-004**: A fresh interactive investigation must establish interactive state before backend processing can race it.
- **BR-INTERACTIVE-011**: Interactive mode must pause after RCA and require explicit later workflow actions.

## Context

The Mock LLM currently uses keyword-matched scenarios for AF A2A conversations. This is useful
for simple single-turn tests, but it is not a reliable representation of a multi-turn protocol:

- Substring matching allows unrelated scenarios to shadow one another.
- Equal-confidence matches depend on registration order.
- `$from_tool` references encode hidden assumptions about earlier calls.
- A test can pass configuration parsing while exercising the wrong phase or the autonomous path.

Issue #2390 exposed the limit: `kubernaut_remediate` creates an RR that is immediately visible to
AA/RO/KA, while a later `kubernaut_investigate` call is only a best-effort attach. It cannot act as
a consent barrier after autonomous processing has started.

## Alternatives Considered

### A. Continue extending keyword scenarios

Rejected. It preserves the failure mode and requires more ordering comments, special keywords,
fallback arguments, and registration exceptions for every new journey.

### B. Use golden raw-response replay

Rejected for this change. Golden replay is appropriate for parser fidelity and signal-driven KA
responses under DD-TEST-011, but it does not model AF's user-turn protocol or dynamic RR references.

### C. Add explicit transcript scenarios

Chosen. A transcript declares the exact user message and expected tool call for each turn. The
Mock LLM matches the current last user message exactly and emits only that step's tool call.
Dynamic values such as the RR ID still use the existing `$from_tool` resolver, but the transcript
makes the phase and predecessor relationship explicit rather than relying on keyword overlap.

## Decision

Add a `transcript_scenarios` YAML section with this shape:

```yaml
transcript_scenarios:
  - name: af_gitops_interactive_2390
    steps:
      - user: "investigate GitOps remediation for deployment memory-eater"
        tool_call:
          name: kubernaut_investigate
          arguments:
            namespace: fp-a2a-interactive
            kind: Deployment
            name: memory-eater
            api_version: apps/v1
            interaction_mode: interactive
      - user: "discover available workflows"
        tool_call:
          name: kubernaut_discover_workflows
          arguments:
            rr_id: "$from_tool:kubernaut_investigate:rr_id"
```

Rules:

1. Matching is exact after trimming whitespace and case-folding. It never searches accumulated
   assistant text or tool arguments.
2. Each step is independently request-scoped and produces cloned argument maps, so concurrent
   requests cannot mutate transcript state.
3. Transcript steps use the existing repeat-call guard: the same step may be selected while the
   model processes its tool result, but the tool is not emitted again until a new user turn arrives.
4. Transcript scenarios have higher priority than keyword scenarios, below golden raw replay.
5. Existing `keyword_scenarios` remain supported unchanged and are migrated only when a journey
   requires protocol-level multi-turn guarantees.
6. A transcript step must contain a non-empty user message and tool name. Invalid transcript
   entries are rejected while loading the override file rather than silently becoming fallback text.

The Issue #2390 journey will use a fresh `kubernaut_investigate` call followed by explicit
discover, select, and watch turns. This is the API behavior described by DD-AA-KA-001 and avoids
using an autonomous RR creation as an interactive consent mechanism.

## Consequences

### Positive

- Multi-turn test protocols are readable as a single ordered artifact.
- Scenario selection is deterministic and independent of registration order.
- Tool-call predecessor relationships remain explicit and dynamically resolvable.
- Legacy scenarios and golden replay remain compatible.
- The harness can identify the matching transcript scenario when diagnosing a failed E2E run.

### Negative

- The Mock LLM supports one additional configuration model.
- Existing keyword scenarios are not automatically converted.
- Exact user messages must be updated when the A2A client changes its phrasing.

### Safety and Compliance

This is test infrastructure only. It does not change production authorization, session ownership,
audit behavior, or execution routing. E2E coverage remains mapped to **BR-TESTING-001**,
**BR-INTERACTIVE-001**, **BR-INTERACTIVE-004**, and **BR-INTERACTIVE-011**.

## Wiring Manifest

| Component | Production/Test Entry Point | Wiring Location | Test ID |
|---|---|---|---|
| Transcript YAML parser | Mock LLM startup override loading | `test/services/mock-llm/config/overrides.go` | `UT-ML-2410-001` |
| Transcript registry scenario | Mock LLM request detection | `test/services/mock-llm/scenarios/registry_default.go` | `UT-ML-2410-002` |
| Transcript step config | OpenAI/Gemini response builders | `test/services/mock-llm/scenarios/transcript.go` and existing handlers | `UT-ML-2410-003` |
| Issue #2390 transcript | FullPipeline Mock LLM deployment | `test/infrastructure/shared_e2e.go` | `E2E-FP-2390-001` |

## Validation

The implementation follows RED, GREEN, REFACTOR:

1. RED: add parser, exact-match, step-selection, repeat-guard, and malformed-entry tests.
2. GREEN: register transcript scenarios and wire the Issue #2390 journey.
3. REFACTOR: remove the dedicated GitOps keyword scenario and stale `$from_tool` override.
4. Validate with focused Mock LLM tests, `go build ./...`, lint, and FullPipeline CI.
