# Kubernaut Agent - BR Mapping

**Version**: 1.0
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

Maps each BR in [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) to the test files that carry
its literal BR-ID citation, verified by repo-wide grep (not by feature inspection) as of this
writing. Per [AGENTS.md](../../../../AGENTS.md)'s Business Requirements Mandate, tests should
reference their BR — this document surfaces where that citation is present, absent, or only
present in another service's test suite (a real, cross-service dependency, not a doc gap).

---

## BR -> Test File Mapping

| BR | Unit Tests | Integration Tests | E2E Tests |
|---|---|---|---|
| BR-KA-191 | `internal/kubernautagent/parser/validator_selfcorrect_params_test.go`, `internal/kubernautagent/parser/validator_params_test.go` | `test/integration/kubernautagent/investigator/investigator_param_validation_test.go` | `test/e2e/kubernautagent/param_validation_e2e_test.go` |
| BR-KA-193 | *(none found — see note below)* | *(none found)* | *(none found)* |
| BR-KA-195 | *(none found — deferred to V2.0, see BUSINESS_REQUIREMENTS.md)* | *(none found)* | *(none found)* |
| BR-KA-197 | *(none found)* | *(none found)* | `test/e2e/kubernautagent/session_endpoints_test.go`, `test/e2e/kubernautagent/incident_analysis_test.go` |
| BR-KA-200 | `internal/kubernautagent/parser/parser_test.go`, `internal/kubernautagent/parser/schema_phase_separation_test.go`, `internal/kubernautagent/parser/adversarial_parser_test.go`, `pkg/aianalysis/investigating_handler_test.go`, `pkg/apifrontend/agent/prompt_test.go`, `pkg/notification/routing_config_test.go` | `test/integration/kubernautagent/investigator/investigator_1430_skip_discovery_test.go`, `test/integration/kubernautagent/investigator/investigator_phase_separation_test.go`, `test/integration/aianalysis/error_handling_integration_test.go`, `test/integration/aianalysis/approval_context_integration_test.go`, `test/integration/aianalysis/agentclient_integration_test.go` | `test/e2e/kubernautagent/incident_analysis_test.go`, `test/e2e/fullpipeline/12_problem_resolved_no_we_test.go`, `test/e2e/fullpipeline/af_helpers_test.go` |
| BR-KA-211 | `pkg/kubernautagent/tools/sanitization/credential_test.go` | *(none found)* | *(none found)* |
| BR-KA-212 | *(none found in KA)* | `pkg/remediationorchestrator/workflowexecution_creator_test.go` (cross-service — target resource consumed by RemediationOrchestrator's WorkflowExecution creation) | *(none found)* |
| BR-KA-261 | *(none found — see note below)* | *(none found)* | *(none found)* |
| BR-KA-263 | *(none found — see note below)* | *(none found)* | *(none found)* |
| BR-KA-264 | *(none found by literal BR-ID; see [DD-KA-018 evidence](#dd-ka-018-cross-reference-for-br-ka-264265) below)* | *(none found)* | *(none found)* |
| BR-KA-265 | *(none found by literal BR-ID; see [DD-KA-018 evidence](#dd-ka-018-cross-reference-for-br-ka-264265) below)* | *(none found)* | *(none found)* |
| BR-KA-OBSERVABILITY-001 | `internal/kubernautagent/metrics/metrics_test.go`, `internal/kubernautagent/server/http_metrics_test.go`, `internal/kubernautagent/audit/emitter_test.go` | `test/integration/kubernautagent/server/wiring_test.go` | `test/e2e/kubernautagent/observability_e2e_test.go` |
| BR-KA-OBSERVABILITY-002 | *(none found by literal BR-ID)* | *(none found)* | *(none found)* |
| BR-AUDIT-011 | *(none found by literal BR-ID)* | *(none found)* | *(none found)* |

### DD-KA-018 cross-reference (for BR-KA-264/265)

BR-KA-264 (post-RCA label detection) and BR-KA-265 (labels in workflow discovery) are implemented as
part of the same DetectedLabels feature governed by
[DD-KA-018](../../../architecture/decisions/DD-KA-018-detected-labels-detection-specification.md),
which has substantial dedicated test coverage — just not tagged with the literal `BR-KA-264`/
`BR-KA-265` strings:

- `internal/kubernautagent/tools/custom/detected_labels_1052_test.go`
- `internal/kubernautagent/investigator/detected_labels_cnv_it_test.go`
- `internal/kubernautagent/investigator/detected_labels_cnv_test.go`
- `internal/kubernautagent/enrichment/detected_labels_1378_it_test.go`
- `internal/kubernautagent/enrichment/detected_labels_test.go`
- `internal/kubernautagent/enrichment/detected_labels_1378_test.go`

---

## Coverage Gaps (Test-ID Hygiene)

Several approved, implemented BRs (BR-KA-193, BR-KA-261, BR-KA-263, BR-KA-OBSERVABILITY-002,
BR-AUDIT-011) have **no test file with a literal BR-ID citation** found via repo-wide grep. This
does not necessarily mean the underlying behavior is untested — KA has 242 unit + 102 integration +
29 E2E test files total (see [testing-strategy.md](./testing-strategy.md)) and some cover these
BRs' behavior without an inline BR-ID tag. It does mean BR-to-test traceability for these five BRs
cannot currently be verified by grep alone, which is itself the gap this document exists to surface
per the project's Business Requirements Mandate (all tests should reference their BR).

**Recommendation**: tag the relevant existing tests (or add missing ones) with their BR ID in a
future pass; out of scope for this documentation-consolidation PR.

---

## Related Documentation

- [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) — BR catalog with status and summaries
- [testing-strategy.md](./testing-strategy.md) — overall test pyramid and counts
