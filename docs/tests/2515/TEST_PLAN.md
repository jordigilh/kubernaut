# Test Plan: Independent Complete LLM Profile Resolution

**Test Plan Identifier**: TP-2515-v1  
**Feature**: Complete, independent LLM configuration resolution for investigation, workflow discovery, and alignment  
**Version**: 1.0  
**Created**: 2026-10-15  
**Author**: Kubernaut Agent Team  
**Status**: Approved  
**Branch**: `issue-2515`

## 1. Purpose and Objectives

This plan proves that every sibling LLM agent receives a complete effective
`types.LLMConfig` from its selected profile and never silently inherits
profile-owned credentials or transport settings from investigation.

Objectives:

1. Prove complete profile resolution and full validation for all three agents.
2. Prove API-key, OAuth2, TLS/mTLS, Vertex, endpoint, reasoning, retry, and
   timeout isolation.
3. Prove workflow phase reload behavior and DD-LLM-008 identity-lock behavior.
4. Prove omitted/disabled siblings do not mutate another sibling's config.
5. Prove no secret contents are logged.

## 2. Authority and References

- BR-AI-090: Independent Complete LLM Profile Resolution for Sibling Agents
- BR-SECURITY-1726: Phase-Override `apiKeyFile` Resolution
- BR-PLATFORM-008: Helm Chart LLM Configuration Parity
- BR-AI-086: LLM reasoning/token support
- DD-LLM-012: Independent Complete LLM Profile Resolution for Sibling Agents
- DD-LLM-008: LLM Identity (Provider+Model) Requires a Restart to Change
- Issue #2515 and operator issue #419

## 3. Risks and Mitigations

| ID | Risk | Impact | Mitigation/tests |
|---|---|---|---|
| R1 | A sibling still inherits a base credential or transport field | Wrong-provider authentication or policy bypass | UT-KA-2515-001..003; IT-KA-2515-001..003 |
| R2 | A profile field is parsed but not wired to the client | Silent endpoint/TLS/OAuth2 misrouting | Wire-level HTTP/TLS/OAuth2 tests |
| R3 | Full validation differs by sibling | Invalid profile reaches runtime | UT-KA-2515-004..006 |
| R4 | Reload changes identity or leaks secrets | Signature/reasoning integrity or credential disclosure | IT-KA-2515-004; log-capture assertions |
| R5 | Chart emits an incomplete profile | Operator/Helm deployments regress | IT-PLATFORM-LLM-009..011 |

## 4. Scope

### In scope

- `internal/kubernautagent/config` complete profile types, resolver, and validation.
- `cmd/kubernautagent/bootstrap.go` startup wiring.
- `cmd/kubernautagent/llm_builder.go` reload wiring.
- `cmd/kubernautagent/main.go` credential setup.
- Helm profile rendering and its existing `helm-unittest` suite.
- Related configuration reference and design documentation.

### Out of scope

- Provider SDK implementation changes.
- Live external-provider E2E tests.
- Operator repository changes; operator parity is tracked by #419.
- Legacy partial `LLMOverrideConfig` behavior.

## 5. Approach and Pass/Fail Criteria

- **Unit**: Ginkgo/Gomega tests for resolver, deep-copy isolation, validation,
  credential selection, and secret-safe logging.
- **Integration**: Ginkgo/Gomega wire-level tests using isolated `httptest`
  servers, OAuth2 token doubles, generated CA/client certificates, and the real
  client builders.
- **Helm integration**: helm-unittest assertions for complete sibling profile
  blocks and credential/TLS paths.
- **E2E**: deferred; no live provider credentials are required for this config
  contract change.

PASS requires all P0 tests, all affected package tests, Helm tests, and no
regressions. FAIL is any P0 failure, secret leakage, sibling inheritance, or
identity-lock regression.

## 6. BR Coverage Matrix

| BR/control | Scenario | Test IDs |
|---|---|---|
| BR-AI-090 | Complete independent investigation config | UT-KA-2515-001, IT-KA-2515-001 |
| BR-AI-090 | Complete independent workflow-discovery config | UT-KA-2515-002, IT-KA-2515-002 |
| BR-AI-090 | Complete independent alignment config | UT-KA-2515-003, IT-KA-2515-003 |
| BR-SECURITY-1726 / IA-5 | Per-profile API-key and OAuth2 credentials | UT-KA-2515-004, IT-KA-2515-001..003 |
| BR-AI-089 / AC-4 | Endpoint isolation | IT-KA-2515-001..003 |
| SC-8 / SC-12 | TLS/mTLS isolation | UT-KA-2515-005, IT-KA-2515-001..003 |
| SI-10 | Full validation and invalid profile rejection | UT-KA-2515-006 |
| DD-LLM-008 | Reload identity lock and safe tuning | IT-KA-2515-004 |
| BR-PLATFORM-008 | Complete Helm profile rendering and provider-specific credential paths | IT-PLATFORM-LLM-009..012 |

## 7. Test Scenarios

### Unit

| ID | Business outcome |
|---|---|
| UT-KA-2515-001 | Base investigation profile resolves all complete fields. |
| UT-KA-2515-002 | Workflow profile does not inherit any unset investigation field. |
| UT-KA-2515-003 | Alignment profile does not inherit any unset investigation field. |
| UT-KA-2515-004 | API-key and OAuth2 credentials resolve from the selected profile only. |
| UT-KA-2515-005 | TLS CA and mTLS cert/key fields remain profile-local. |
| UT-KA-2515-006 | Invalid provider, endpoint, OAuth2, TLS, and required-field profiles fail consistently. |
| UT-KA-2515-007 | Omitted/disabled sibling leaves another sibling unchanged and no secrets are logged. |

### Integration/wiring

| ID | Business outcome |
|---|---|
| IT-KA-2515-001 | Investigation and workflow requests reach distinct endpoints with distinct auth/transport. |
| IT-KA-2515-002 | Alignment requests reach its selected endpoint with its own auth/transport. |
| IT-KA-2515-003 | Vertex/OAuth2/TLS profile settings reach the real built client for each sibling path. |
| IT-KA-2515-004 | Phase reload applies safe complete-profile tuning and rejects provider/model identity changes. |
| IT-KA-2515-005 | Errors are observable and logs contain paths/metadata only, never secret contents. |

### Helm integration

| ID | Business outcome |
|---|---|
| IT-PLATFORM-LLM-009 | Workflow and alignment rendered blocks contain complete profile fields. |
| IT-PLATFORM-LLM-010 | Shared profile paths are explicit; distinct credential/TLS mounts remain isolated. |
| IT-PLATFORM-LLM-011 | Invalid/incomplete profile rendering fails clearly. |
| IT-PLATFORM-LLM-012 | A shared Secret uses the selected sibling provider's credential filename. |

## 8. Wiring Manifest

| Component | Production entry point | Wiring location | IT test |
|---|---|---|---|
| Complete resolver — investigation | Base client construction | `cmd/kubernautagent/bootstrap.go:buildLLMClients` | IT-KA-2515-001 |
| Complete resolver — workflow discovery | Phase client construction | `cmd/kubernautagent/bootstrap.go:buildLLMClients` | IT-KA-2515-002 |
| Complete resolver — alignment | Shadow client construction | `cmd/kubernautagent/bootstrap.go:buildAlignmentStack` | IT-KA-2515-003 |
| Complete resolver — reload | Runtime phase update | `cmd/kubernautagent/llm_builder.go:reloadSinglePhaseClient` | IT-KA-2515-004 |

## 9. TDD Status

| Phase | Status |
|---|---|
| Discovery/preflight | Complete; baseline relevant tests pass |
| RED | Complete; resolver and production-wiring tests were added and initially failed against the removed partial contract |
| GREEN | Complete; focused Go and Helm suites pass |
| REFACTOR | Complete; shared credential/validation path, chart mounts, docs, and lint cleanup applied |
