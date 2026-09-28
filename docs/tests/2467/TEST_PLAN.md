# Test Plan: SP-Owned Classification Authority Across RO, AIA, KA, and AF

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2467-v1.0<br>
**Issue**: [#2467](https://github.com/jordigilh/kubernaut/issues/2467)<br>
**Feature**: Keep SignalProcessing-owned classifications authoritative through AIAnalysis, Kubernaut Agent, and API Frontend RCA contracts<br>
**Created**: 2026-09-27<br>
**Author**: Kubernaut Team<br>
**Status**: Implemented; local unit validation complete; runtime integration/E2E blocked by Podman storage<br>
**Branch**: `fix/2467-sp-classification-source-of-truth` (stacked on `fix/2459-workflow-discovery-audit`)

---

## 1. Purpose and boundaries

### 1.1 Purpose

SP is the source of truth for normalized signal classifications. This plan proves that an adversarial or malformed model response cannot create or replace an SP-owned classification as it crosses RO → AIA → KA → AF. Tests must prove business outcomes—not just field copying—including SP failure when required classifications are empty or `unknown`, fail-closed RO handling of invalid completed SP state, correct final presentation, and preservation of the distinct manual/pre-SP severity path tracked by #1017.

### 1.2 Classification inventory and test contract

Required SP-derived values for the normal SP-backed path:

| SP status source | Downstream contract | Required behavior |
|---|---|---|
| `SignalClassification.Severity` | AIA `SignalContext.Severity` → AgentSession `Severity` → KA/server RCA → AF presentation | Preserve the exact SP value. It is read-only context to the LLM, not an LLM output field. |
| `EnvironmentClassification.Environment` | AIA `SignalContext.Environment` → AgentSession `Environment` → KA context | Preserve the exact SP value; never synthesize it in RO or read it from RCA output. |
| `PriorityAssignment.Priority` | AIA `SignalContext.BusinessPriority` → AgentSession `Priority` → KA context | Preserve the exact SP value; never synthesize it in RO or read it from RCA output. |
| `SignalClassification.SignalName` | AIA `SignalContext.SignalName` → AgentSession signal name → KA context | Preserve SP's normalized input name. Do not confuse it with the LLM RCA's `signal_name`, which may describe an inferred effect and remains a separate RCA finding. |
| `SignalClassification.SignalMode` | AIA `SignalContext.SignalMode` → AgentSession `SignalMode` → KA prompt strategy | Preserve the SP value (`reactive` or `proactive`). |
| `SignalClassification.ClusterClassification` | AIA `SignalContext.Cluster` → AgentSession `Cluster` | Preserve exactly when SP supplies it; absence is a normal optional result and remains omitted. |
| `BusinessClassification` (`BusinessUnit`, `ServiceOwner`, `Criticality`, `SLARequirement`) | AIA `EnrichmentResults` → AgentSession enrichment → KA context | Preserve the complete optional SP value; absence remains absent. LLM response extras cannot create it. |

`SignalClassification.PolicyHash` and `SourceSignalName` are SP-owned lineage/audit metadata, not currently part of the AIA/AgentSession classification context. They must not be synthesized from RCA output; this plan does not add new propagation fields for them. `KubernetesContext` remains SP enrichment/context, not an LLM-authored classification.

For required values, the user has clarified the contract: SP fails classification if a workflow-driving output (severity, normalized signal name, mode, environment, or priority) is empty or uses the `unknown` sentinel. A concrete Rego catch-all value such as `warning`, `Development`, or `P3` is a valid operator policy result, not an SP-generated fallback. RO also rejects creation of an SP-backed AIA if a `Completed` SP has a missing or unusable required output; optional cluster and business classifications are omitted. RO-local `Unknown`/`P2` defaults and RR signal-name fallback are not valid substitutes. BR-SP-105 and related severity/environment/priority requirement language has been amended in this worktree. AF's source-severity gate is additionally authoritative under DD-AF-016.

**Authority note:** BR-SP-106 requires downstream consumers to use SP's normalized `SignalName`/`SignalMode`; BR-SP-105 and BR-SEVERITY-001 have been amended for #2467 so `unknown` remains enum-compatible but is not a successful required classification. Environment/priority defaults belong in operator Rego rather than Go fallbacks. The chart example's current defaults of severity `unknown`, environment `Unknown`, and priority `P3` show why the example must be updated; evaluator code currently accepts the first two as successful results. Under the clarified contract, SP transitions to `Failed` for empty/`unknown` required outputs; concrete defaults such as `P3` remain valid when explicitly defined by an operator's Rego policy. BRs do not explicitly specify RO's response to malformed `Completed` SP state; RO's fail-closed validation is a defensive boundary check. Business-classification dimensions remain optional per BR-SP-081; when supplied, preserve them verbatim, but they do not satisfy missing required workflow classifications.

AF's alert/rule-grounded severity triage before SP completes remains separate from SP's later normalized classification. Per DD-AF-016, AF creates an RR only from explicit alert/rule severity and passes that raw value unchanged; it does not ask an LLM to infer severity or map unknown source values to `warning`. For rule-only potential-issue triage, all relevant candidates must have the same non-empty severity. Missing/conflicting values, or no correlating alert/rule, produce no RR; the user may retry once an alert is pending/firing. The pre-SP value remains distinct from SP's normalized classification and is marked provisional in AF presentation. #1017 manual-signal severity behavior is not converted into SP ownership.

### 1.3 Out of scope

- Changing the classification taxonomy or substantive signal-to-classification mappings. In scope: reject empty/`unknown` required workflow classifications in SP and update the chart's example Rego catch-all outputs/comments.
- Promoting SP policy hashes or pre-normalized source names into AIA/KA/AF contracts.
- Removing the LLM RCA's distinct `signal_name` finding where it describes an inferred effect. It must never overwrite the normalized input `SignalContext.SignalName`.
- Creating new audit event types. Existing correlated SP classification and AIA/KA/AF events are used for reconstruction evidence.
- Changing manual-signal severity behavior in #1017.

## 2. Requirements and control objectives

### 2.1 Business requirements

- BR-SP-105 and BR-SEVERITY-001: SP Rego owns normalized severity; downstream consumers preserve it; `unknown` remains schema-valid but is not a successful required classification.
- BR-SP-106 / BR-AI-084: SP owns normalized signal name and reactive/proactive mode.
- BR-SP-051–053, BR-SP-070–072, BR-SP-002, BR-SP-080, BR-SP-081: SP owns environment, priority, and business classification outputs.
- BR-ORCH-025: RO passes SP-derived context to AIA.
- BR-AI-008: AIA stores the RCA while retaining authoritative request context.
- #2467 acceptance criteria; #1017, #2034, #2071, and #2068 define adjacent manual and AF grounding boundaries.

### 2.1.1 Requirement alignment notes

The previous BR-SEVERITY-001 language requiring KA to copy severity into RCA/response fields has been amended in this worktree: severity is read-only context and is not an LLM-authored output. #2467 tests must prove that contract. The existing taxonomy mismatch remains (`critical/high/medium/low/unknown` in BR-SP-105/BR-SEVERITY-001 vs. `critical/high/warning/info/unknown` in current SP/AIA enums); taxonomy changes remain out of scope, and tests use values admitted by current CRD contracts.

BR-SP-071 has a similar documentation drift: the standalone `BR-SP-071-priority-fallback-matrix.md` is deprecated in favor of operator Rego defaults/no Go fallbacks. The stale service summary and archived matrix have been annotated as superseded in this worktree. The chart example still needs its catch-all values/comments updated to explain that Rego defaults apply when specific mappings do not match and are operator-owned policy choices.

### 2.2 Security/compliance objectives

| Control | Objective in this change | Proving evidence |
|---|---|---|
| FedRAMP AC-4 | Prevent untrusted LLM/RCA data from flowing into server-owned SP classification fields across service boundaries. | Conflicting-value unit and integration tests at RO, AIA, KA, and AF; end-to-end parity assertion. |
| FedRAMP SI-10 | Validate the structured LLM/tool boundary and ensure hostile, unknown, or absent classification properties cannot become authoritative values. | Schema/DTO assertions, adversarial parser and AF callback tests, and required-SP-missing tests. |
| FedRAMP AU-3 | Existing SP classification/audit payload retains structured classification and correlation content; no new audit schema is introduced. | Correlation-ID audit integration/E2E assertion. |
| SOC 2 CC7.2 | Operators can correlate the SP classification decision with downstream analysis/presentation outcomes for investigation. | Existing audit-trail query by remediation correlation ID, alongside final-value parity assertion. |
| OWASP ASVS 5.0.0 V2.2.1 | Validate security/business-decision input against the expected structure and values. | Tests assert classification properties are absent from model-output schemas and unknown model fields do not populate typed results. |
| OWASP ASVS 5.0.0 V2.2.2 | Enforce validation and authority at trusted service layers rather than trusting the model/tool caller. | AIA response processor and AF server projection overwrite or omit untrusted values using trusted SP-derived state. |

SOC 2 CC8.1 applies to the project's change approval and verification process, not to the runtime classification data; it is handled by this plan's approval/TDD gates rather than claimed as a runtime test result.

## 3. Pyramid strategy

The pyramid invariant applies: UT proves behavior, IT proves each production wiring point, and E2E proves the cross-service user journey.

### 3.1 Unit tests

| ID | Business-level assertion | Controls |
|---|---|---|
| `UT-SP-2467-001` | The chart example's Rego catch-all rules return explicit concrete severity, environment, and priority values for otherwise-unmatched inputs; comments explain that defaults are operator policy decisions, not SP fallbacks. | AC-4, BR-SP-105/051/070, SI-10 |
| `UT-SP-2467-002` | An empty or `unknown` required policy result is unusable and causes SP classification failure; an explicit concrete Rego default such as `P3` remains a valid result. | AC-4, BR-SP-105/051/070, SI-10 |
| `UT-RO-2467-001` | With conflicting RR values and a fully classified SP object, RO creates AIA with exact SP severity, environment, priority, normalized signal name/mode, and optional business/cluster values. RR external severity remains unchanged. | AC-4, BR-SP-105/106, BR-ORCH-025 |
| `UT-RO-2467-002` | Each missing or `unknown` required SP result fails closed; no AIA is persisted and no `Unknown`, `P2`, or RR signal-name fallback is substituted. Explicit concrete Rego defaults already present in SP status are copied verbatim. | AC-4, SI-10, V2.2.1/V2.2.2 |
| `UT-RO-2467-003` | Missing optional `ClusterClassification` or `BusinessClassification` remains omitted without inventing values. | AC-4, BR-FLEET-003, BR-SP-080/081 |
| `UT-KA-2467-001` | All KA response schemas and instructions keep SP values as read-only input context but do not request severity or other SP categories in model output; parser DTOs do not expose those fields. The distinct RCA `signal_name` remains distinct from input `SignalName`. | SI-10, V2.2.1, BR-SEVERITY-001, BR-SP-106 |
| `UT-KA-2467-002` | An adversarial model response containing conflicting top-level/nested severity and extra category keys cannot set KA result severity; KA result uses its trusted signal context. | AC-4, SI-10, V2.2.2 |
| `UT-AA-2467-001` | AIA processing of free-form AgentSession RCA JSON ignores model-supplied category fields and persists the SP-derived severity from AIA spec; unknown extra category keys cannot bypass the typed boundary. An RCA `signal_name` finding does not replace input `SignalContext.SignalName`. | AC-4, SI-10, V2.2.2 |
| `UT-AA-2467-002` | If required SP severity is absent in an invalid AIA input, AIA does not substitute model severity; the chosen fail-closed/omit behavior is explicit and tested. | AC-4, SI-10, V2.2.2 |
| `UT-AF-2467-001` | Generated `present_decision` LLM schema does not expose SP-owned category fields; server-side presentation still emits only authoritative categories. | AC-4, SI-10, V2.2.1/V2.2.2 |
| `UT-AF-2467-002` | Conflicting category values cannot survive grounded, summary-only, missing-structured-RCA, malformed-RCA, or ungrounded `present_decision` paths; categories are server-populated from trusted state or omitted. | AC-4, SI-10, V2.2.2 |
| `UT-AF-2467-003` | Alert/rule-grounded pre-SP triage remains visibly provisional and separate from SP-backed final categories; no-alert/no-rule LLM inference cannot create an RR; #1017/manual severity does not become an SP classification. | AC-4, BR #1017, DD-AF-010 |
| `UT-AF-2467-004` | A firing/pending alert or eligible rule-only candidate supplies a raw severity label that AF preserves byte-for-byte; unrecognized source labels are not rewritten to `warning`, and no LLM is invoked. | AC-8, BR-SEVERITY-001, DD-AF-016 |
| `UT-AF-2467-005` | When no alert is active, matching rule candidates are accepted only if every relevant rule has the same non-empty raw severity; missing or conflicting labels return `ErrSeverityUndetermined`. | AC-8, DD-AF-016 |
| `UT-AF-2467-006` | No correlated alert/rule prevents RR creation; SP remains responsible for mapping a supplied raw value or failing its own required classification. | AC-4/8, BR-SEVERITY-001, DD-AF-016 |
| `UT-AF-2467-007` | A matching rule candidate with no severity label returns `ErrSeverityUndetermined`; AF creates no RR. | AC-8, DD-AF-016 |
| `UT-AF-2467-008` | Matching rule candidates with conflicting raw severity labels return `ErrSeverityUndetermined`; AF creates no RR rather than ranking or selecting one. | AC-8, DD-AF-016 |
| `UT-AF-2467-009` | An unfamiliar explicit firing-alert label such as `P0` is passed through unchanged and is not normalized to `warning`. | AC-8, BR-SEVERITY-001, DD-AF-016 |
| `UT-AF-2467-010` | A correlating firing alert with a missing/blank severity label returns `ErrSeverityUndetermined`; AF creates no RR. | AC-8, DD-AF-016 |
| `UT-AF-2467-011` | Multiple firing alerts in the selected specificity/state bucket with conflicting raw severity labels fail closed; AF does not rank external values. | AC-8, DD-AF-016 |

### 3.2 Integration tests

| ID | Production path exercised | Business assertion | Controls |
|---|---|---|---|
| `IT-SP-2467-001` | Production SignalProcessing `PhaseClassifying` reconciliation with a policy returning `unknown` for a mandatory workflow classification | SP transitions to terminal `PhaseFailed`, sets classification/processing conditions false, and uses its existing error event/audit path; concrete policy catch-alls complete with their exact values. | AC-4, SI-10 |
| `IT-RO-2467-001` | Envtest RO reconcile: completed SP → `ProcessingHandler.handleSPCompleted` → `AIAnalysisCreator.Create` → persisted AIA | Required SP values survive with adversarially different RR fields; missing required SP classification prevents AIA creation; optional classifications are omitted. Extend `test/integration/remediationorchestrator/severity_normalization_integration_test.go`. | AC-4, SI-10 |
| `IT-KA-2467-001` | Production KA investigator/request path from AgentSession signal context through LLM parser and server result construction | Mock model supplies conflicting categories; result severity comes from AgentSession input, while inferred RCA `signal_name` remains a separate finding. Extend `test/integration/kubernautagent/investigator/signal_context_779_it_test.go` or `signal_mode_it_test.go`. | AC-4, SI-10 |
| `IT-AA-2467-001` | AIA controller reconciliation of completed AgentSession | Conflicting and unknown fields in raw RCA JSON cannot override the AIA spec's SP severity or populate other typed SP classifications. Extend `test/integration/aianalysis/reconciliation_test.go`. | AC-4, SI-10 |
| `IT-AF-2467-001` | Real AF `present_decision` function tool through model callback/phase guard and emitted decision artifact | The strict tool schema cannot accept SP-owned categories from the model; all groundedness branches use server-owned values or omit them; provisional data stays marked provisional. Extend existing AF agent/tool integration tests. | AC-4, SI-10, V2.2.1/V2.2.2 |
| `IT-AF-2467-002` | Production `HandleCreateRR` → severity triager → persisted RemediationRequest | AF persists the exact explicit alert severity unchanged; missing alert severity and conflicting rule-only candidates return an error and create no RR; no severity LLM or local fallback is invoked. | AC-8, SI-10 |

### 3.3 End-to-end tests

| ID | Journey | Business assertion | Controls |
|---|---|---|---|
| `E2E-2467-001` | SP classification → RO-created AIA → AgentSession/KA with adversarial model response → AF presentation | For a unique remediation/correlation ID, AIA, KA result, and AF final artifact retain exact SP-owned values; no conflicting or unknown LLM category survives. AF's pre-SP severity is an explicit raw alert/rule value, never LLM-inferred or locally normalized; no-rule, missing, or ambiguous rule-only severity creates no RR. Query existing audit events by correlation ID and verify the SP classification decision and downstream lifecycle events remain reconstructable. Extend the existing full-pipeline and AF E2E harnesses where possible. | AC-4/8, SI-10, AU-3, CC7.2, V2.2.2 |

E2E is required for the full journey; if the current harnesses cannot inject an adversarial model response without adding production-only seams, report that constraint and propose an isolated test-only mock before implementation rather than silently downgrading this tier to UT-only.

## 4. Risks and mitigations

| Risk | Mitigation / proving test |
|---|---|
| SP currently accepts enum-valid `unknown` defaults and can complete classification with them. | Reject empty/`unknown` required outputs in SP's Classifying phase, align BR-SP-105, and prove terminal failure; retain RO validation as defense-in-depth (`UT/IT-SP-2467-002/001`, `UT/IT-RO-2467-002`). |
| Example OPA catch-all values may be mistaken for platform defaults. | Explain that Rego defaults apply when specific rules do not match and that operators choose workflow-appropriate concrete values; SP itself does not supply `P3` or other classification fallbacks. |
| Removing severity from AF's LLM schema also removes it from the user-facing artifact. | Split model-writable tool arguments from the server-owned artifact projection; test schema and emitted artifact independently (`UT/IT-AF-2467-001`). |
| Removing model RCA severity accidentally erases severity from KA output. | Keep severity in KA's server-owned result populated from AgentSession signal context; test conflicting LLM values (`UT/IT-KA-2467-002/001`). |
| Free-form AgentSession JSON carries unknown category keys. | Use adversarial unknown-key fixtures; assert AIA typed output only uses the SP spec (`UT-AA-2467-001`). |
| Normalized SP input signal name is conflated with LLM RCA `signal_name`. | Keep separate names/contracts and add a fixture where the LLM inferred effect differs (`UT-KA-2467-001`, `UT-AA-2467-001`). |
| AF reintroduces LLM inference or rewrites raw source severity as a local canonical/default value; missing/conflicting alert/rule labels are treated as sufficient evidence. | Preserve only explicit, unambiguous alert/rule source values, keep them distinct from later SP normalization, and assert no RR on absent/conflicting evidence (`UT-AF-2467-003..011`, `IT-AF-2467-002`, `E2E-2467-001`). |

## 5. Test completion criteria

- All listed UT and IT scenarios pass; all four production boundaries have a passing IT.
- The E2E journey proves cross-service parity and correlation-based audit reconstruction.
- Zero LLM-facing KA/AF output schemas, examples, or correction prompts request SP-owned categories.
- No raw model category is parsed into an authoritative typed field; unknown free-form RCA properties cannot bypass the boundary.
- Required SP values have no RO fallback; empty/`unknown` required outputs fail in SP and are rejected defensively in RO; concrete policy-defined catch-alls remain attributable to Rego; optional classifications remain optional.
- AF never invokes an LLM or local default to derive RR severity; only explicit, unambiguous raw alert/rule severity is passed to SP, and missing/conflicting evidence creates no RR.
- Existing `#1017` manual/provisional severity behavior and the distinct RCA `signal_name` behavior remain covered.
- Build, lint, affected unit/integration suites, and repository-required validation gates pass.
