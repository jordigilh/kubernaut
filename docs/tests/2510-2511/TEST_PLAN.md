# Test Plan: Generic Recurrent RCA and Applied-State History

**ID**: TP-KA-2510-2511-v1.0
**Status**: Implemented — scoped validation complete; E2E runtime blocked by Podman capacity
**Date**: 2026-10-14
**BRs**: BR-KA-016, BR-AUDIT-005
**DD**: DD-KA-2511

## Business outcomes

- Prior target-scoped remediation evidence reaches final RCA before completion.
- Target changes continue investigation without an artificial target-change limit.
- Applied, not-executed and unknown evidence remain distinct.
- OOM and non-memory recurrence use the same production contract.
- Existing budgets, cancellation, no-workflow and safety gates remain authoritative.

## Scenario matrix

| ID | Outcome | Controls |
|---|---|---|
| UT-DS-2510-001 | Post hash=current serializes `confirmed`. | AU-3, CC7.2 |
| UT-DS-2510-002 | `NoExecution` serializes `not_confirmed`. | AU-3, SI-10 |
| UT-DS-2510-003 | Contradictory, missing and differing hashes remain `unknown`/not-confirmed with explicit statuses. | AU-3, CC7.2 |
| UT-KA-2511-001 | Initial RCA remains signal-only; history review is server supplied. | SI-10, AC-4 |
| UT-KA-2511-002 | Generic history review includes prior outcomes and mechanism status. | SI-10, CC7.2 |
| UT-KA-2511-003 | Changed target is re-enriched without escalation. | AC-4, ASVS V2.2.1 |
| UT-KA-2511-004 | Existing max-turn/tool budgets bound continuation. | SI-10, ASVS V2.3.1 |
| UT-KA-2511-005 | Failed acquisition/exhaustion follows existing review path. | AU-3, CC7.2 |
| UT-KA-2511-006 | Mechanism status enum survives parser/result/audit boundaries. | AU-3, ASVS V16.2.x |
| UT-MOCK-2511-001 | Mock provider models provisional RCA, history continuation and changed target generically. | BR-KA-016 |
| UT-MOCK-2511-002 | OOM and non-memory fixtures use the same provider conversation contract. | BR-KA-016 |
| IT-DS-2510-001 | Authenticated DS route/generated client preserves applied-state fields. | AU-3, CC7.2 |
| IT-KA-2511-001 | Real Investigator path continues after provisional RCA. | SI-10, AC-4 |
| IT-KA-2511-002 | Changed target is re-enriched and its history is used. | AC-4 |
| IT-KA-2511-003 | Existing max-turn/tool-budget exhaustion produces review. | SI-10, AU-3 |
| IT-KA-2511-004 | Final mechanism status reaches typed audit boundaries. | AU-2/AU-3, CC7.2 |
| IT-KA-2511-005 | History/spec-hash acquisition failure is observable and fail-closed. | SI-10, AU-3, CC7.2 |
| IT-KA-2511-006 | Cancellation before continuation prevents an ungrounded provider turn. | SI-10, ASVS V2.3.1 |
| IT-MOCK-2511-001 | Mock LLM HTTP ordering is proven. | BR-KA-016 |
| E2E-FP-2511-001 | OOM/informer recurrence uses history-aware RCA. | Scoped controls |
| E2E-FP-2511-002 | Non-memory recurrence uses the identical path. | Scoped controls |
| E2E-FP-2511-003 | Target change/no-workflow/review remain safe. | AC-4, AU-3 |

## Wiring manifest

| Component | Production entry | IT |
|---|---|---|
| Applied-state classification | DS logic → HTTP/Ogen → KA DTO | IT-DS-2510-001 |
| History evidence DTO | DSAdapter → Enricher → prompt | IT-KA-2511-001 |
| RCA continuation | Investigator → existing RCA loop/budget | IT-KA-2511-001 |
| Target re-enrichment | ResolveEnrichmentTarget/cache | IT-KA-2511-002 |
| Generic Mock LLM conversation | Mock handlers/scenarios | IT-MOCK-2511-001 |
| Audit projection | RCA completion/audit mapper | IT-KA-2511-004 |

Unit target: at least 90% of defined business scenarios and 100% of critical
scenarios; no tests are added solely for raw line coverage.

## Existing dependent tests updated in this PR

- `test/services/mock-llm/handlers/openai_2496_test.go`: the existing history
  fixture now exercises a provisional RCA/history-review turn.
- `test/e2e/fullpipeline/history_selection_2496_test.go`: the existing genuine
  lifecycle asserts applied-state/spec-change evidence.
- `test/e2e/fleet/history_selection_2496_test.go`: the existing fleet journey
  asserts origin/target-scoped applied-state evidence.

These updates are part of feature delivery, not post-CI remediation.

## Dedicated Make-target validation

| Target | Result |
|---|---|
| `make test-unit-datastorage` | Passed |
| `make test-unit-kubernautagent` | Passed |
| `make test-unit-mock-llm` | Passed |
| `make test-integration-datastorage` | Passed |
| `make test-integration-kubernautagent` | Blocked in setup: synchronized suite timed out while building the DataStorage image; 0 investigator specs executed |
| `make test-e2e-fullpipeline GINKGO_FOCUS=2496 TEST_PROCS=1` | Blocked in infrastructure setup: Podman `/var/tmp`/container storage exhausted; 0 specs executed |
| `make test-e2e-fleet` | Not run after the shared Podman capacity failure; it uses the same image-build prerequisite |

The full-pipeline focus-filter smoke invocation using `history-2496` ran zero
specs because that label is not part of the Ginkgo focus text; it is not counted
as validation. The corrected `2496` invocation reached image setup and was
blocked before test execution.
