# Test Plan: Issue #2444 SOC2 Audit Gaps

**Feature:** OpenAI-compatible stream failures and audit reconstruction
**Business requirements:** BR-AUDIT-005 v2.0, BR-INTEGRATION-1254
**Controls:** SOC2 CC7.2, CC8.1; FedRAMP AU-2, AU-3
**Status:** In progress

## Objective

Ensure malformed or incomplete provider responses fail closed, reach the
existing AF/KA audit boundaries, and remain reconstructable using stable
correlation identifiers and typed error details.

## Scenarios

| ID | Level | Scenario | Expected result |
|----|-------|----------|-----------------|
| UT-SHARED-2444-001 | Unit | Malformed SSE JSON | Stream returns a decode error and emits no successful terminal response |
| UT-SHARED-2444-002 | Unit | Provider EOF without `[DONE]` | Stream returns an incomplete-stream error |
| UT-AF-2444-003 | Unit | Streaming adapter receives provider error | ADK iterator receives the error rather than silent completion |
| UT-AF-2444-004 | Unit | Invalid tool-call arguments | Adapter returns a decoding error rather than a nil argument map |
| IT-AF-2444-005 | Integration | A2A execution fails after an LLM error | Started and failed events share a stable correlation ID; no completed event is emitted |
| UT-AF-2444-006 | Unit | AF typed failure payload | A2A and severity failures persist `error_details` with code, component, and retry guidance |
| UT-KA-2444-007 | Unit | KA response failure payload | `aiagent.response.failed` preserves typed `ErrorDetails` through the Data Storage mapper |

## Wiring Manifest

| Component | Production entry point | Wiring location | Test |
|-----------|------------------------|-----------------|------|
| Strict stream errors | AF ADK model iterator | `pkg/apifrontend/launcher/openai/adapter.go:GenerateContent` | UT-AF-2444-003 |
| A2A failure audit | A2A executor callbacks | `pkg/apifrontend/launcher/launcher.go:buildAfterExecuteCallback` | IT-AF-2444-005 |
| Typed AF failure data | AF StoreAdapter | `pkg/apifrontend/audit/store_adapter.go` | UT-AF-2444-006 |
| Typed KA failure data | KA Data Storage audit store | `internal/kubernautagent/audit/ds_payloads.go` | UT-KA-2444-007 |

## Acceptance Criteria

- Provider parse errors, incomplete streams, and invalid tool arguments are
  observable failures.
- AF task lifecycle events use the A2A context/session ID, falling back to the
  task ID only when no context ID exists.
- Failure payloads include standardized `error_details` without raw provider
  payloads or secrets.
- Existing consumer-requested early stream termination remains successful.
- Focused tests, build, lint, and full affected-package tests pass.
