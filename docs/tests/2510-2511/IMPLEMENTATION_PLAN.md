# Implementation Plan: #2510/#2511

**Status**: Implemented; runtime integration/E2E re-run pending environment capacity
**Test plan**: [TEST_PLAN.md](TEST_PLAN.md)
**Design**: [DD-KA-2511](../../architecture/decisions/DD-KA-2511-generic-recurrent-rca-continuation.md)

## TDD sequence

### RED

Add DataStorage applied-state and serialization tests; parser/audit mechanism-
status tests; Investigator provisional-submit, continuation, target-change and
budget tests; generic Mock LLM scenarios and handler tests; and integration/E2E
recurrence journeys.

### GREEN

1. Extend the DataStorage OpenAPI history contract and regenerate Ogen/embedded
   specifications.
2. Map applied-state/spec-change evidence through DSAdapter and enrichment.
3. Continue RCA after provisional submit using the existing shared turn/tool
   budget, cancellation and sentinels.
4. Re-enrich changed targets through the existing cache without a target-change
   threshold.
5. Preserve mechanism status through parser, result, audit and response mapping.
6. Update generic Mock LLM scenarios and dependent integration/E2E fixtures.
7. Run CHECKPOINT W for every wiring-manifest row.

Existing dependent tests updated in the same PR include the #2496 Mock LLM
handler fixture and the fullpipeline/fleet history-selection journeys; these
are not deferred to CI follow-up.

### REFACTOR

Extract focused classification/continuation helpers, preserve sanitization and
error observability, and remove duplication without creating new components.

## Verification commands

- `go build ./...`
- `go test ./... -run=^$ -timeout=30s`
- `golangci-lint run --timeout=5m`
- `make test`

Then run affected DataStorage, KA, Mock LLM, integration, fleet and full-pipeline
targets. Unrelated baseline failures remain separately classified.
