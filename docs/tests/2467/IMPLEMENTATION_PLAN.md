# Implementation Plan: Issue #2467 — SP Classification Source of Truth

**Status**: Implemented; local verification recorded below; runtime integration/E2E remain environment-blocked; Option A selected for AF<br>
**Issue**: [#2467](https://github.com/jordigilh/kubernaut/issues/2467)<br>
**Branch**: `fix/2467-sp-classification-source-of-truth`<br>
**Stack base / future PR target**: `fix/2459-workflow-discovery-audit` at `579234d5cf3dd004168a1a98c79eb4c3777d30f0`<br>
**Business requirements**: BR-SP-105, BR-SP-106, BR-SEVERITY-001, BR-SP-051–053, BR-SP-070–072, BR-SP-002/080/081, BR-ORCH-025, BR-AI-008, BR-AI-084<br>
**Test plan**: [TP-2467-v1.0](TEST_PLAN.md)

## 1. Preflight findings

### 1.1 Repository and branch state

- Dedicated worktree: `/Users/jgil/go/src/github.com/jordigilh/kubernaut-issue-2467`.
- Branch is clean at PR #2468 / `fix/2459-workflow-discovery-audit` head `579234d5cf3dd004168a1a98c79eb4c3777d30f0`; no implementation changes existed when preflight began.
- The shared checkout remains untouched. The future stacked PR must target `fix/2459-workflow-discovery-audit`, not `main`.
- Engram's index is pinned to the registered Kubernaut project; the sibling worktree has no workspace manifest. Engram recall and indexed search were used against the registered project, with source-level verification in this worktree. This did not block preflight.

### 1.2 Component/data-flow map

| Stage | Existing behavior and gap | Candidate files |
|---|---|---|
| SP → RO → AIA | `buildSignalContext` copies severity, signal mode/name, cluster, business enrichment. It locally supplies `Environment="Unknown"` and `Priority="P2"` and falls back from missing normalized SP `SignalName` to RR signal type. | `pkg/remediationorchestrator/creator/aianalysis.go`; `pkg/remediationorchestrator/aianalysis_creator_test.go`; `test/integration/remediationorchestrator/severity_normalization_integration_test.go`; `api/aianalysis/v1alpha1/aianalysis_types.go` |
| AIA → AgentSession | Request builder copies severity, environment, priority, signal name/mode, cluster, and enrichment into AgentSession spec. | `pkg/aianalysis/handlers/request_builder.go`; `pkg/aianalysis/handlers/request_builder_agentsession_test.go` |
| KA LLM contract | Prompt/schema requests nested and top-level severity; parser DTOs accept it (and nested RCA `signal_name`). Investigator later overwrites severity from signal context, but this still lets LLM output cross the contract boundary. | `internal/kubernautagent/prompt/templates/incident_investigation.tmpl`; `internal/kubernautagent/investigator/investigator_rca.go`; `internal/kubernautagent/parser/schema.go`; `parser_llm_types.go`; `parser_format.go`; `investigator.go` |
| KA → AIA | AIA parses raw RCA severity and then `extractRootCauseAnalysisWithSPSeverity` overwrites it from AIA spec. Unknown free-form JSON keys are already outside typed AIA mappings, but severity is unnecessarily captured first. | `pkg/aianalysis/handlers/response_processor.go`; `pkg/aianalysis/response_processor_no_matching_test.go`; `pkg/aianalysis/investigating_handler_test.go` |
| AIA/KA → AF | `present_decision` currently exposes required severity to the model. The canonical grounded path substitutes KA RCA, but summary-only/missing-structured-RCA and provisional branches can leave LLM-authored RCA values untouched. AF also creates provisional local-triage RCA before SP completes. | `pkg/apifrontend/tools/ka_tools.go`; `pkg/apifrontend/agent/phase_guard.go`; `pkg/apifrontend/tools/ka_investigate_mcp.go`; `pkg/apifrontend/tools/af_create_rr.go`; corresponding AF tool/agent tests |
| Audit evidence | SP already emits classification-decision audit data; downstream events use remediation correlation. AIA's analysis-complete payload does not add category fields. No new event type is currently required for this fix. | `pkg/signalprocessing/audit/client.go`; `pkg/aianalysis/audit/audit.go`; `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` |

### 1.3.1 Authoritative requirement gaps

- **Missing/unusable SP classifications:** BR-SP-106 requires downstream use of SP's normalized signal name/mode, and severity/environment/priority policy belongs to SP/Rego. The chart example currently has Rego catch-alls for severity (`unknown`), environment (`Unknown`), and priority (`P3`); evaluator code accepts the unknown sentinels today. User clarification for #2467: an empty or `unknown` required workflow classification is not usable, and SP should fail the Classifying phase instead of completing. Concrete catch-all values such as `P3` are valid only when explicitly returned by operator Rego. RO should also reject a malformed `Completed` SP defensively and must not synthesize a replacement. BR-SP-105, BR-SEVERITY-001, the SP service requirements, and DD-SEVERITY-001 have been amended in this worktree to record the fail-closed contract. BR-SP-081 allows optional business-classification dimensions; those remain optional here.
- **Severity prompt contradiction:** BR-SEVERITY-001 previously told the LLM to copy SP severity into RCA output, conflicting with #2467. Its requirement and acceptance criteria have been amended in this worktree: severity is read-only model context, not model-authored output. The implementation still needs tests proving the prompt/schema/parser contract.
- **Existing taxonomy mismatch:** BR-SEVERITY-001/BR-SP-105 document `critical/high/medium/low/unknown`; current SP/AIA CRD field enums are `critical/high/warning/info/unknown`. Do not silently widen this issue into a taxonomy migration. Tests should use currently admitted values and assert literal provenance. Record the mismatch as an existing follow-up if not separately resolved.
- **Priority requirement drift:** The standalone BR-SP-071 is explicitly deprecated in favor of Rego defaults/no Go fallback. Its archived matrix and the old service summary have been explicitly marked superseded; the active text now says empty/`unknown` required results fail and concrete values must come from OPA.

### 1.4 Static spike result

**Question:** Can the normal pipeline preserve all listed categories without treating LLM RCA output as an authority, fail unusable SP classifications at their source, preserve AF's artifact/manual path, and prevent AF from inventing or locally normalizing alert severity?

**Result: YES.** The normal AIA → AgentSession path already transports SP context. KA can retain severity as a server-populated result field while removing it from LLM response DTOs and schemas. AIA can map narrative RCA fields without reading category values from raw RCA JSON. AF must separate model-writable tool arguments from final server-owned category projection; simply deleting severity from the existing shared RCA struct risks deleting it from the final artifact, while keeping it in the LLM schema violates #2467. DD-AF-016 now makes the AF source boundary authoritative: only explicit, unambiguous alert/rule labels can seed an RR; AF does not ask an LLM or map raw values locally. #1017's manual/pre-SP value remains distinct and cannot substitute for a missing SP classification.

No prototype/code spike was needed: the relevant production path, DTOs, schema generation, parser behavior, and existing AF grounding tests are present and inspected. During RED, add a schema-level test proving the generated tool schema excludes all LLM-writable SP categories before choosing the server-projection seam.

## 2. Approved behavior and decisions

1. **Confirmed direction: fail closed at SP for unusable required classifications; validate again at RO.** SP must fail the Classifying phase when a required workflow-driving classification is empty or `unknown`, rather than complete classification and allow downstream routing. OPA catch-all rules may return concrete values (for example `warning`, `Development`, or `P3`); these are policy outputs, not SP fallbacks. Before creating AIA, RO must require usable SP severity, normalized signal name, signal mode, environment, and priority, and create no AIA if a `Completed` SP violates that contract. Optional business and cluster classifications remain omitted. RO must not recreate defaults locally.
2. **Split AF's model input from the final artifact projection (Option A approved).** Remove SP category fields, especially severity, from the LLM-facing `present_decision` argument schema. Populate the final artifact/presentation from server-held grounded KA/SP context; when authoritative structured RCA is unavailable, omit SP categories instead of keeping model values. Keep only qualifying alert/rule-grounded pre-SP triage in the distinct provisional path, including its provisional marker; do not restore no-rule LLM severity inference or RR creation.
3. **Keep the two signal-name concepts distinct.** SP normalized `SignalContext.SignalName` remains immutable input context; the LLM RCA's `signal_name` may remain an inferred effect finding and must never overwrite that input field.
4. **No backwards compatibility.** Do not preserve obsolete model-output fields or RR/default fallbacks. Deploy the RO/AIA/KA/AF contract changes as one compatible release unit.
5. **AF alert severity is source data, not an inference result (DD-AF-016 approved).** AF may create an RR only when a correlated alert/rule supplies an explicit raw severity. For rule-only potential-issue triage, all relevant matching rules must carry the same non-empty severity; missing or conflicting values fail closed. AF passes the value unchanged to SP—no Tier 2.5 LLM inference, `warning` substitution, or local canonicalization. SP Rego maps the raw value or SP fails.

### AF alternatives (Option A selected)

| Option | Approach | Assessment |
|---|---|---|
| **A — recommended** | Separate LLM tool arguments from the server-owned artifact projection; inject trusted categories only after model argument validation and use trusted state for final presentation. | Meets schema-removal requirement while preserving grounded output and provisional/manual UX. Requires targeted callback/artifact wiring tests. |
| B | Remove severity from both the model schema and all AF presentation artifacts. | Simple, but needlessly loses trusted grounded severity and weakens current user-facing/provisional UX. Not recommended. |
| C | Keep severity in the model schema and overwrite it after tool execution. | Rejected: the LLM still receives write authority and can emit the field; fails #2467 even if the final value is overwritten. |

The user approved implementation of this plan on 2026-09-27. Option A is selected for the AF model-input/server-output boundary; B and C are not in scope. The AF severity-source behavior was further clarified and approved on 2026-09-27 and is recorded in DD-AF-016.

### AF alert-severity source (DD-AF-016 selected)

| Option | Approach | Assessment |
|---|---|---|
| A | Let Tier 2.5's LLM infer severity from rule prose. | Rejected: severity controls workflow selection, and a plausible inference can choose the wrong workflow. |
| B | Require an already-firing alert and reject all rule-only potential investigations. | Rejected: a matching rule with explicit severity can ground a potential-issue RR before it fires. |
| **C — selected** | Use only explicit raw alert/rule severity. For multiple rule-only candidates, proceed only when every relevant candidate has the same non-empty value; otherwise fail closed. | No LLM/default/AF normalization; retry after an alert becomes pending/firing when the rule candidates are ambiguous. |

## 3. Scope and affected contracts

### In scope

- RO no-fallback mapping and required-SP validation.
- AIA/AgentSession API contract requiredness where needed (notably mode if current generated CRD schema leaves it optional), plus generated CRD artifacts if markers change.
- KA prompt examples/instructions, all response schemas, parser DTOs/conversion, and correction/retry prompts: SP-owned classifications remain read-only context and are absent from model response contracts.
- KA server-side population of severity from trusted AgentSession signal context.
- AIA response parsing: no read/capture of raw model category values; use AIA's SP-derived spec for any persisted category.
- AF LLM-facing schema, phase-guard branches, and server-side artifact projection; provisional local severity remains explicitly separate.
- AF's direct alert/rule severity gate, raw-value pass-through to the RR, no LLM/default/AF normalization, and fail-closed behavior for missing or ambiguous rule candidates.
- Align BR-SEVERITY-001's prompt/acceptance language and BR-SP-105's `unknown` handling with #2467, and annotate the deprecated BR-SP-071 summary drift. Do not alter the severity taxonomy in this issue.
- Existing unit, integration, and E2E cases that encode the old contract.

### Out of scope

- DataStorage schema or new audit event types; adding PolicyHash/SourceSignalName fields downstream; #1017 manual-signal classification rules; changing the semantics of the LLM RCA `signal_name` finding. SP-side rejection of empty/`unknown` required classifications and the chart's example Rego catch-all rules are in scope; broader classification-mapping changes are not.

## 4. Wiring manifest

| Component/boundary | Production entry point | Wiring location | Integration proof |
|---|---|---|---|
| SP required-classification validation | SignalProcessing `PhaseClassifying` reconciliation | `internal/controller/signalprocessing/signalprocessing_classifying.go` → required classification evaluators/failure transition | `IT-SP-2467-001`, extending `test/integration/signalprocessing/reconciler_integration_test.go` |
| RO SP→AIA classification validator/mapper | Completed-SP handling | `internal/controller/remediationorchestrator/processing_handler.go` → `creator.AIAnalysisCreator.Create` in `pkg/remediationorchestrator/creator/aianalysis.go` | `IT-RO-2467-001`, extending `test/integration/remediationorchestrator/severity_normalization_integration_test.go` |
| KA input-to-result classification authority | Production investigator run and result assembly | `internal/kubernautagent/investigator/investigator.go` invokes parser and applies signal context | `IT-KA-2467-001`, extending `test/integration/kubernautagent/investigator/signal_context_779_it_test.go` or `signal_mode_it_test.go` |
| AIA AgentSession-result boundary | Completed AgentSession handling | `pkg/aianalysis/handlers/response_processor.go` via the AIA investigating handler | `IT-AA-2467-001`, extending `test/integration/aianalysis/reconciliation_test.go` |
| AF model-to-presentation boundary | `present_decision` callback/tool and decision artifact projection | `pkg/apifrontend/agent/phase_guard.go`, `pkg/apifrontend/tools/ka_tools.go`, and the existing AF launcher/part-converter path | `IT-AF-2467-001`, extending AF agent/tool integration tests |
| AF source-severity gate | `af_create_rr` production handler and Prometheus severity triager | `pkg/apifrontend/tools/af_create_rr.go` and `pkg/apifrontend/severity/triage.go` | `IT-AF-2467-002`, proving raw source pass-through and no RR on missing/conflicting severity |
| Full cross-service journey | SP classification to final AF decision | Existing full-pipeline and AF E2E harnesses | `E2E-2467-001`, extending `test/e2e/fullpipeline/01_full_remediation_lifecycle_test.go` and `test/e2e/apifrontend/interactive_wiring_e2e_test.go` as harness boundaries permit |

## 5. TDD phase plan

Estimated effort assumes existing test harnesses can inject mock responses and excludes any deployment wait for Kind E2E.

### RED — define the business contract (0.75–1 day)

1. Add Ginkgo/Gomega SP tests proving concrete OPA catch-alls classify unmatched inputs while empty/`unknown` required outputs transition to `PhaseFailed` (`UT-SP-2467-001..002`, `IT-SP-2467-001`); assert the chart example documents that catch-alls are operator-owned policy decisions.
2. Add Ginkgo/Gomega RO adversarial pass-through and missing/`unknown`-field tests (`UT-RO-2467-001..003`); add/extend envtest production dispatch (`IT-RO-2467-001`).
3. Add schema/prompt/parser tests proving KA has no classification response fields, while context remains and output `signal_name` stays semantically distinct (`UT-KA-2467-001..002`).
4. Add AIA adversarial free-form AgentSession RCA tests proving model categories cannot influence stored classifications (`UT-AA-2467-001..002`, `IT-AA-2467-001`).
5. Add AF generated-schema and groundedness-branch matrix tests, including provisional/manual path (`UT-AF-2467-001..003`, `IT-AF-2467-001`).
6. Add AF source-severity tests for exact raw-label preservation, alert/rule candidate agreement, no LLM invocation, and no RR on absent/conflicting evidence (`UT-AF-2467-004..011`, `IT-AF-2467-002`).
7. Add end-to-end conflict fixture and correlated audit assertion (`E2E-2467-001`).
8. Run the focused new tests and record expected failures before implementation.

### GREEN — minimal end-to-end wiring (1.5–2 days)

1. Fail SP classification when a required workflow-driving output is empty/`unknown`; update the chart example's Rego catch-alls to concrete illustrative values and explain their purpose/ownership. Validate completed SP outputs before RO persists AIA; remove RO's `Unknown`/`P2` and RR signal-name fallbacks. Keep truly optional classifications optional.
2. Keep required SP classification context in AIA/AgentSession contracts and ensure generated CRD schema matches approved requiredness.
3. Remove SP-owned categories from KA's LLM response schemas, examples, parser DTOs, and retry prompts. Populate the KA result's severity from trusted signal context after parsing.
4. Stop extracting model severity from AgentSession RCA in AIA; source persisted severity solely from AIA spec and ignore unknown category properties.
5. Implement the approved AF model-input/server-output split; ensure every grounding branch overwrites categories from trusted state or omits them. Keep pre-SP triage separately provisional.
6. Remove the severity LLM fallback and AF's local severity normalization/default. Use only explicit correlated alert/rule labels; require rule-only candidates to agree on one non-empty raw value and fail before RR creation otherwise.
7. Wire each change at its existing production entry point and make every Wiring Manifest IT pass.

### REFACTOR — name concrete improvements (0.5–1 day)

- Consolidate duplicated KA schema assertions/fixtures so all response-schema variants are checked consistently.
- Make authority and provenance explicit in helper/type names and comments; avoid ambiguous use of “severity” or “signal name” between input context and RCA findings.
- Simplify AF branch handling so model-authored category values are stripped/ignored in one server-owned projection path; make raw-source versus SP-normalized severity provenance explicit without changing the established provisional UX semantics.
- Apply the Go anti-pattern checklist to changed code; do not create new components during REFACTOR.

**Post-refactor validation is mandatory and is not itself the REFACTOR work:** run `go build ./...`, `go test ./... -run=^$ -timeout=30s`, affected unit/integration suites, repository lint, and `make test` where feasible. Run the focused Kind E2E after communicating its runtime/cost.

## 6. Success criteria and verification

- All TP-2467 UT, IT, and E2E cases pass with conflicting model categories and unknown raw RCA keys; SP rejects empty/`unknown` required outputs and the example OPA catch-alls return explicit concrete values.
- No LLM response schema, example, initial prompt, or correction prompt requests an SP-owned category.
- The output field corresponding to each SP classification is either exact SP data or omitted/fail-closed; it is never sourced from model RCA JSON or RO local defaults.
- AF's final artifact has no model-authored category value in grounded, summary-only, missing-structured-RCA, malformed, or ungrounded paths. AF RR severity is the exact explicit alert/rule value; missing/conflicting evidence produces no RR, and no LLM or `warning` fallback is used.
- Pre-SP local triage remains explicitly provisional and distinct from SP-backed classification; manual-signal #1017 behavior remains intact.
- Existing SP classification audit data and downstream completion events remain queryable by remediation correlation ID; no raw provider response is newly added to audit storage.
- CHECKPOINT W passes for every manifest row; build, lint, affected tests, and required post-refactor validation pass.

## 7. Risks, rollback, and confidence

| Risk | Mitigation |
|---|---|
| Treating `unknown` as unclassified changes current SP behavior and may expose policies that rely on enum-valid `unknown` defaults. | Update the example policy and BR-SP-105 to make catch-all behavior explicit; fail the SP object with a clear condition/error when a required result is empty/`unknown`; retain RO validation as a boundary guard. |
| Concrete OPA catch-all values in the example may be copied without operator review. | Label the values illustrative and explain that each operator must choose catch-alls appropriate to its workflow-routing policy; no SP-side default is implied. |
| AF tool schema and artifact are coupled today. | Approve and test the DTO/projection separation before changing the LLM schema. If no safe server projection seam is found, stop and present evidence/options rather than retaining model fields. |
| Existing Prometheus rules may omit severity or yield conflicting source values. | Require explicit non-empty values and exact agreement for rule-only candidates; fail closed and direct operators/callers to correct rule labels or retry when a specific alert is pending/firing (`UT-AF-2467-005/006`, `IT-AF-2467-002`). |
| Removing KA response severity may affect downstream consumers of KA's result DTO. | Search/update all parser/result consumers; keep the server-populated result field and prove the real AgentSession completion path. |
| Incompatible contract rollout across independently deployed services. | Release RO, AIA, KA, and AF changes together; no legacy parsing or field fallback. Roll back the code release as a unit if any contract validation fails. |

**Preflight confidence: 90%.**<br>
**Justification:** The issue body and current production path are clear, and the four data boundaries, current fallbacks, schema/parser behavior, existing tests, and audit correlation path have been inspected. Source requirements reveal that RO fail-closed behavior is not explicitly specified and that BR-SEVERITY-001 currently contradicts #2467's model-output boundary; the deprecated priority fallback is also summarized inconsistently. AF's server-owned artifact projection still needs the first schema RED test to validate the seam. These were surfaced as approval/document-alignment gates rather than silently treated as settled requirements. At the time of preflight, no source implementation or tests had been run.

## 8. Implementation and verification outcomes (2026-09-28)

### Implementation

- Implemented the approved cross-service classification contract: SP rejects unusable required Rego classifications; RO validates completed SP status before constructing AIA; AIA ignores model-authored severity; KA excludes SP-owned severity from response schemas/parser authority and reapplies trusted input; AF requires explicit, unambiguous raw alert/rule severity and projects trusted severity only into its server-built decision artifact.
- Added DD-AF-016 and reconciled the severity, priority-fallback, SignalProcessing, and AF operational/test documentation with the approved behavior.
- Regenerated AgentSession and AIAnalysis CRDs with pinned `controller-gen` v0.19.0. For each changed CRD, the `config/crd/bases/`, Helm chart, chart-file, and embedded copies have matching SHA-256 hashes.
- Wiring manifest tests were added at each existing production seam. No new audit event type was introduced; existing correlated triage/classification audit paths remain in use.

### Local verification

| Check | Result |
|---|---|
| `go build ./...` | PASS |
| `go test ./... -run=^$ -timeout=30s` | PASS (all packages compile) |
| `make test TEST_SUITE_PROCS=2 TEST_PROCS=4` | PASS (full unit-test matrix) |
| Focused changed-package tests (SP evaluator/controller, RO creator/controller, KA parser/investigator/prompt, AF severity/agent/launcher/tools/handler, and command wiring) | PASS |
| `golangci-lint run --new-from-rev=HEAD --timeout=5m` | PASS (0 new issues) |
| `git diff --check` | PASS |
| CRD copy consistency | PASS (matching hashes across all four copies for both CRDs) |

The default-parallelism `make test` attempt had one load-sensitive failure in `UT-SP-2467-001`: the chart policy's priority evaluation exceeded the evaluator's existing 100 ms Rego deadline while all service suites were running concurrently. The evaluator package passed in isolation, and the complete unit matrix passed with reduced service/Ginkgo parallelism; the production timeout was not relaxed.

### Environment limitations / repository-wide checks

- Focused envtest execution was attempted, but suite setup could not build its Podman dependency images (`no space left on device`). Integration packages compile successfully with `go test -run=^$`; runtime integration and E2E journeys were not completed locally. No Podman images or volumes were pruned.
- Repository-wide `golangci-lint run` still reports findings in existing spike, validation-script, and hack-tool files; the diff-aware lint reports zero new issues.
- `make lint-test-patterns` and `make lint-business-integration` report broad repository-wide findings. The new integration assertions were checked against the scanner's literal-pattern rule, and no new business type/component was introduced.

**Implementation confidence: 95%.** The changed production paths build, the full unit matrix passes with reduced concurrency, the focused service tests pass, diff-aware lint is clean, and generated CRD copies are synchronized. Remaining risk is limited to runtime integration/E2E coverage blocked by local Podman storage; CI must provide that final wiring/environment assessment.
