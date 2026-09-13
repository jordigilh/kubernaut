# DD-TEST-017: Structured Mock-LLM Scenario Selectors

**Status**: Approved and implementing
**Date**: 2026-09-12
**Version**: 1.0
**Scope**: Mock-LLM scenario detection and E2E scenario configuration
**Business Requirement**: BR-TESTING-001

## Context

The Mock-LLM scenario registry historically exposed separate keyword and signal
helper constructors. Those helpers performed broad substring matching and did
not make caller, phase, or matching surface visible in scenario metadata. New
scenarios could therefore copy the old helpers and reintroduce routing
collisions, especially in multi-turn AF and KA conversations.

Issue #2390 demonstrated this failure mode: an AF interactive request and a KA
investigation request could contain overlapping text while both scenarios had
the same confidence. Registration order then became part of the behavior.

## Decision

`scenarios.ScenarioSelector` is the canonical matcher for static scenarios. It
contains the selector constraints and evaluates keyword, signal, proactive,
last-user-turn, and caller/phase scope consistently. Static scenario metadata
publishes the same constraints used for detection.

The canonical YAML key is `scenario_selectors`. The existing
`keyword_scenarios` key remains accepted as a deprecated compatibility path so
existing test fixtures do not break, but all repository-owned generated and
static configuration uses `scenario_selectors`.

Custom matching remains an explicit escape hatch through `CustomMatch` for
stateful scenarios, golden transcript replay, dynamic resource extraction, or
conditions that cannot be expressed declaratively. It is not the default path
for new static scenarios.

## Alternatives Considered

### Keep the Existing Helpers

This has the smallest immediate diff, but future scenarios would continue to
inherit broad substring matching and hidden registration-order dependencies.
It does not address the stability concern.

### Require Every Scenario to Implement One New Matcher Interface

This would enforce a stronger boundary, but it would require rewriting
transcript, replay, fleet, and dynamic scenarios whose matching is intentionally
stateful or context-sensitive. It would increase migration risk without
improving those matchers.

### Adopt a Canonical Selector with an Explicit Custom Escape Hatch

This centralizes common behavior and makes simple scenarios declarative while
preserving specialized behavior where necessary. It also allows the old helper
constructors and their accidental reuse to be removed immediately.

**Selected.**

## Migration and Deprecation Policy

1. All simple built-in scenarios use `ScenarioSelector`.
2. New repository-owned YAML uses `scenario_selectors`.
3. `keyword_scenarios` remains supported only for compatibility and is validated
   before registration.
4. The old keyword/signal helper constructors are removed rather than wrapped,
   so new Go scenarios cannot reuse them.
5. New custom matchers must document why declarative selector fields are
   insufficient and should expose any stable constraints in metadata.
6. The compatibility YAML key can be removed after existing external fixtures
   have migrated and a release compatibility window has elapsed.

## Consequences

### Positive

- New simple scenarios have one obvious, reusable implementation path.
- Caller and phase constraints are visible in listings and tests.
- Malformed selector configuration fails during loading instead of silently
  falling through to the default scenario.
- Specialized scenarios retain the flexibility needed for protocol and
  state-machine tests.

### Negative

- The registry supports two YAML keys during the compatibility window.
- `CustomMatch` still permits bespoke logic, so review and tests remain needed
  for context-sensitive scenarios.
- Text-based phase inference remains a heuristic until the request protocol
  exposes stronger phase evidence; this decision does not add production
  correlation headers or alter service protocols.

## Validation

- Unit tests cover selector matching, metadata publication, YAML compatibility,
  and malformed selector rejection.
- The Mock-LLM suite must pass before removing any remaining legacy usage.
- Repository-wide searches must show no use of the removed helper constructors.
