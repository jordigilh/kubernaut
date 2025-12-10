# REQUEST: HAPI Mock LLM Mode for Integration Testing

**Date**: 2025-12-10
**From**: AIAnalysis Team
**To**: HAPI Team
**Priority**: Medium (blocks AIAnalysis integration testing)

## Problem Statement

AIAnalysis integration tests require calling HAPI `/api/v1/incident/analyze` and `/api/v1/recovery/analyze` endpoints with a mock LLM to avoid:
1. Cost: Real LLM calls incur API charges
2. Flakiness: LLM responses are non-deterministic
3. CI/CD: Automated tests shouldn't require API keys

## Current Configuration Attempt

```yaml
# podman-compose.test.yml
holmesgpt-api:
  environment:
    - LLM_PROVIDER=openai
    - LLM_MODEL=openai/gpt-4o-mini
    - OPENAI_API_KEY=sk-mock-test-key-for-integration
    - MOCK_LLM_ENABLED=true
```

## Observed Behavior

Despite `MOCK_LLM_ENABLED=true`, HAPI still attempts real LLM calls:

```
litellm.exceptions.AuthenticationError: AuthenticationError: OpenAIException -
Incorrect API key provided: sk-mock-********************tion
```

The mock flag doesn't intercept calls before they reach litellm.

## Questions for HAPI Team

1. **How should `MOCK_LLM_ENABLED` be configured to actually mock LLM calls?**
   - Is there a specific model format required?
   - Does it need additional config.yaml settings?

2. **Is there a test mode endpoint like `/api/v1/test/incident/analyze`?**
   - That bypasses LLM and returns deterministic responses

3. **Alternative: Does HAPI support a mock provider in litellm?**
   - Like `LLM_MODEL=mock/gpt-4` or `LLM_PROVIDER=mock`

## Required for AIAnalysis V1.0

Integration tests need to verify:
- `IncidentRequest` schema compliance (14 required fields)
- `RecoveryRequest` schema compliance
- Response parsing and error handling
- Contract compliance with HAPI OpenAPI spec

## Workaround in Place

Currently, integration tests skip gracefully when HAPI mock mode unavailable:
```go
if err != nil {
    Skip("HAPI not available - skipping integration tests")
}
```

Unit tests with `MockHolmesGPTClient` provide coverage, but integration tests with real HAPI are blocked.

## Proposed Solutions

**Option A**: HAPI adds true mock mode that returns deterministic responses
**Option B**: HAPI provides test fixtures/stubs for integration testing
**Option C**: AIAnalysis uses WireMock/similar to mock HAPI HTTP responses

Which approach does HAPI team recommend?

---

**Status**: Awaiting HAPI team response

