# BR-TESTING-001: Golden Transcript Mock LLM Fidelity

**Business Requirement ID**: BR-TESTING-001
**Category**: Testing Infrastructure
**Priority**: P1
**Target Version**: V1.3
**Status**: Approved
**Date**: 2026-03-04
**Related Issue**: [kubernaut-demo-scenarios#296](https://github.com/jordigilh/kubernaut-demo-scenarios/issues/296)

---

## Business Need

### Problem Statement

The Mock LLM used in CI/CD E2E tests produces static, hardcoded responses that diverge from real Claude behavior. In v1.3 development, at least 6 distinct CI failures were caused by mock LLM fidelity gaps:

- `remediation_target` with non-existent resources (`Deployment/worker`, `Deployment/api-server`)
- Missing `service_account_name` in workflow selection
- Missing `TARGET_RESOURCE_*` parameters
- Wrong `execution_engine` (`tekton` default vs actual `job`)
- Confidence value mismatches (0.88 vs 0.95)
- `problem_resolved` with contradictory actionability flags

These failures are undetectable until E2E tests run because the mock LLM's substring-based scenario detection and static config produce responses that pass superficial checks but fail contract validation.

### Business Objective

Introduce golden transcript replay mode in the Mock LLM. Golden transcripts are structured JSON files captured from real HAPI+Claude interactions that record the exact LLM response content. The mock LLM replays these verbatim, ensuring:

1. KA's parser is tested against real Claude output format and structure
2. Response field values (remediation targets, workflow IDs, confidence scores) match real LLM behavior
3. Regressions in KA's response processing are caught immediately in CI/CD
4. New scenarios can be added by capturing more transcripts without code changes

---

## Acceptance Criteria

1. Mock LLM supports loading golden transcript JSON files from a configurable directory (`MOCK_LLM_GOLDEN_DIR`)
2. Replay scenarios match on `signalName` from the transcript and take priority over static scenarios (confidence > 1.0)
3. In `FORCE_TEXT=true` mode (current E2E), replay scenarios return the exact analysis text from the transcript
4. In `FORCE_TEXT=false` mode (future high-fidelity), replay scenarios provide per-step tool call arguments from the transcript
5. Static scenarios remain as fallback when no golden transcript matches
6. Golden transcript JSON format is compatible with `kubernaut-demo-scenarios/scripts/capture-transcript.sh` output
7. Unit tests validate transcript loading, scenario matching, config population, and response generation

---

## Scope

### In Scope

- Golden transcript Go types for JSON deserialization
- `ReplayScenario` implementing the `Scenario` interface
- Config extension for `MOCK_LLM_GOLDEN_DIR`
- Registry integration with priority-based fallback
- `ExactAnalysisText` support for verbatim response replay
- Unit tests for all replay components

### Out of Scope

- Capturing golden transcripts (tracked by kubernaut-demo-scenarios#296)
- E2E infrastructure changes for mounting transcripts (Phase 4, separate PR)
- Multi-model replay (same transcript format works regardless of model)

---

## Technical Context

- Architecture Decision: DD-LLM-003 (mock-first development strategy)
- KA is an OpenAPI-compliant drop-in replacement for HAPI; HAPI transcripts are directly usable
- Current E2E uses `MOCK_LLM_FORCE_TEXT=true` (single-turn text responses)
- Mock LLM scenario detection uses confidence-based priority (highest wins)

---

## Dependencies

| Dependency | Type | Status | Impact |
|-----------|------|--------|--------|
| kubernaut-demo-scenarios#296 | External | Requested | Golden transcripts needed for CI integration |
| capture-transcript.sh enhancement | External | Requested | Raw LLM response bodies for full-fidelity replay |
