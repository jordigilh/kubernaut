# Test Plan: OpenAI Structured-Output JSON Schema Envelope

**Test Plan Identifier**: TP-2438-v1.0
**Feature**: Include the required named nested `json_schema` envelope for AF and KA
**Version**: 1.0
**Created**: 2026-09-19
**Author**: Kubernaut Team
**Status**: Active
**Branch**: `fix/fleet-e2e-logger-init`

## 1. Purpose and Business Outcome

This plan validates issue #2438 and BR-AI-086. Structured-output requests sent through
the shared OpenAI Chat Completions client must use the provider contract
`response_format.type=json_schema` with a stable `json_schema.name` and the caller's
schema nested under `json_schema.schema`. The contract must hold for non-streaming and
streaming requests and remain correct through both AF and KA adapters.

## 2. Authority and Scope

- [BR-AI-086](../../requirements/BR-AI-086-llm-reasoning-token-support.md)
- [DD-LLM-005](../../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md)
- Issue #2438
- `pkg/shared/llm/openaicompat/client.go`
- `pkg/apifrontend/launcher/openai/adapter.go`
- `pkg/kubernautagent/llm/openai/client.go`

In scope: shared request serialization, AF and KA adapter paths, streaming and
non-streaming HTTP requests, and regression protection. Out of scope: OpenAI Responses
API structured outputs and provider-side schema validation semantics.

## 3. Pyramid Coverage

| Tier | Test IDs | Proof |
|---|---|---|
| Unit/contract | `UT-KA-1581-018`, `UT-KA-1581-019` | Shared client emits the named nested envelope for Chat and StreamChat |
| Integration | AF and KA structured-output adapter assertions | Production adapters send the shared contract to an HTTP provider test server |
| E2E | `E2E-PLATFORM-2438-001` | The post-setup fleet remediation journey completes using the deployed AF/KA clients and structured investigation responses |

## 4. Wiring Manifest

| Component | Production entry point | Wiring location | Test |
|---|---|---|---|
| Shared JSON schema envelope | AF and KA OpenAI-compatible clients | `openaicompat.Client.Chat` and `StreamChat` | `UT-KA-1581-018/019` |
| AF adapter | API Frontend model dispatch | `pkg/apifrontend/launcher/openai/adapter.go` | AF structured-output regression test |
| KA adapter | Kubernaut Agent client dispatch | `pkg/kubernautagent/llm/openai/client.go` | KA structured-output regression test |

## 5. E2E Procedure

1. Complete the #2437 fleet demo setup.
2. Run the fleet crashloop scenario and request investigation through Console.
3. Verify the investigation reaches AIAnalysis completion and workflow execution.
4. Confirm no structured-output request fails due to a missing `json_schema.name` or
   incorrectly shaped schema envelope.

## 6. Pass Criteria

- Shared non-streaming and streaming envelope tests pass.
- AF and KA adapter regressions pass.
- Build, lint, and full unit test targets pass.
- The final fleet remediation journey completes without a structured-output HTTP 400.
