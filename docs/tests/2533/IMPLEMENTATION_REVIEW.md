# Issue #2533 Implementation Review

## Scope

Issue #2533 adds historical decision context for recurrent remediation without
changing Remediation Orchestrator ownership or workflow-created events.

Data lineage:

```text
AIAnalysis analysis.completed audit payload
  -> one bounded, cluster-scoped batch association by existing history correlation IDs
  -> DataStorage Tier 1/Tier 2 DTOs
  -> KA enrichment
  -> frozen evidence handoff
  -> labelled, sanitized Phase 3 historical-decision context
```

The association is not a Phase 3 lookup and does not use parameters, execution
bundles, raw transcripts, secrets, or private chain-of-thought. The frozen
handoff and complete rendered Phase 3 prompt each fail closed at the configured
byte budget before provider dispatch.

## Wiring manifest

| Boundary | Production path | Proving scenario |
|---|---|---|
| AIAnalysis audit -> DS history | `pkg/aianalysis/audit/audit.go` -> `pkg/datastorage/repository/remediation_history_repository.go` -> `pkg/datastorage/server/remediation_history_handler.go` | `UT-AA-2533-001`, `UT-DS-2533-001`, `IT-DS-2533-001` |
| DS DTO -> KA Tier 1/Tier 2 | `internal/kubernautagent/enrichment/ds_adapter.go` | `IT-KA-2533-002` |
| KA enrichment -> frozen handoff | `investigator.runWorkflowSelection` -> `evidence_handoff.go` | `UT-KA-2533-004/005/006` |
| Frozen handoff -> Phase 3 provider request | `prompt.Builder.RenderWorkflowSelection` plus complete rendered-prompt byte guard before dispatch | `IT-KA-2533-003/004` |
| Full recurrence journey | existing full-pipeline history journey with #2533 context assertions | `E2E-FP-2533-001/002` |

The production adapter has a compile-time assertion for the supplemental batch
querier. Missing AI rows or fields remain absent/unknown; database/query
failures return the existing RFC7807 fail-closed history error path.

## RED / GREEN / REFACTOR evidence

- **RED:** New Ginkgo/Gomega source, DS, adapter, handoff, prompt, and
  provider-path tests failed before the schema/projection implementation.
- **GREEN:** Generated OpenAPI/ogen fields, bounded batch query, DTO mapping,
  handoff projection, and prompt rendering made the focused suites pass.
- **REFACTOR:** Shared `HistoricalDecisionContext` validation centralizes
  sanitization and no-truncation behavior across DS, KA, handoff, and prompt;
  production wiring remains additive and model-agnostic.

## Control mapping

| Objective | Evidence |
|---|---|
| FedRAMP AU-2/AU-3, SOC2 CC7.2 | Typed AIAnalysis audit source, correlation-key batch association, Tier 1/Tier 2 reconstruction |
| FedRAMP AC-4/AC-6 | Target/cluster-scoped history, allow-listed non-executable fields, unchanged trusted gates |
| FedRAMP SI-10 | UTF-8/blank/partial/oversized rejection, sanitization, explicit unknowns, fail-closed budget |
| OWASP ASVS V2.2.1/V2.2.2/V2.2.3/V2.3.1 | Bounded and validated historical text; no silent truncation; no authority transfer to identity or execution |
| OWASP ASVS V16.2.1/V16.2.4/V16.2.5/V16.3.4/V16.4.1 | Correlation isolation, secret/prompt-injection sanitization, no parameters/transcripts/private reasoning, frozen-packet budget |
| SOC2 CC8.1 | Approved plan, RED/GREEN/REFACTOR evidence, generated-artifact review, wiring manifest, and scoped validation |

AU-9, AU-11, and SC-8 are inherited controls and are unchanged by this slice.

## Validation status

Passed: affected unit suites, focused ITs, compile-only all-package tests,
`go build ./...`, `go vet ./...`, generated-artifact regeneration, and
new-code lint.

Environment blockers: the investigator integration suite requires
`/usr/local/kubebuilder/bin/etcd`; full-pipeline E2E requires Kind image-build
infrastructure. Both compiled successfully, but execution was unavailable in
the current environment.
