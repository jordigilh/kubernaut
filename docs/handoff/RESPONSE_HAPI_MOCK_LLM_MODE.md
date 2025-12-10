# RESPONSE: HAPI Mock LLM Mode for Integration Testing

**Date**: 2025-12-10
**From**: HAPI Team
**To**: AIAnalysis Team
**Re**: REQUEST_HAPI_MOCK_LLM_MODE.md
**Status**: ✅ **IMPLEMENTED - Option B**

---

## Decision

**Selected Approach**: Option B - HAPI provides internal mock mode that returns deterministic responses

**Rationale**:
- Same endpoint paths as production (no `/test/` prefix needed)
- Same request validation (catches schema errors early)
- Same response structure (guaranteed contract compliance)
- Only the LLM call is bypassed
- Simplest integration for consumers

---

## Implementation Details

### Environment Variable

```bash
# Enable mock LLM mode
MOCK_LLM_MODE=true
```

When `MOCK_LLM_MODE=true`:
1. HAPI validates incoming requests normally (schema validation still runs)
2. Instead of calling HolmesGPT SDK → LLM, returns deterministic mock response
3. Mock responses are schema-compliant and based on input `signal_type`
4. No API keys required
5. No external LLM calls made

### Mock Response Behavior

| Input Signal Type | Mock Workflow ID | Mock Confidence | Mock Severity |
|-------------------|------------------|-----------------|---------------|
| `OOMKilled` | `mock-oomkill-increase-memory-v1` | 0.92 | critical |
| `CrashLoopBackOff` | `mock-crashloop-config-fix-v1` | 0.88 | high |
| `NodeNotReady` | `mock-node-drain-reboot-v1` | 0.90 | critical |
| `ImagePullBackOff` | `mock-image-fix-v1` | 0.85 | high |
| Other | `mock-generic-restart-v1` | 0.75 | medium |

### Sample Response (Mock Mode)

```json
{
  "incident_id": "test-incident-123",
  "analysis": "Mock analysis for OOMKilled signal",
  "root_cause_analysis": {
    "summary": "Container exceeded memory limits (MOCK)",
    "severity": "critical",
    "contributing_factors": ["mock_factor_1", "mock_factor_2"]
  },
  "selected_workflow": {
    "workflow_id": "mock-oomkill-increase-memory-v1",
    "version": "1.0.0",
    "confidence": 0.92,
    "rationale": "Mock selection based on OOMKilled signal type",
    "parameters": {
      "NAMESPACE": "from-request",
      "MEMORY_LIMIT": "1Gi"
    }
  },
  "alternative_workflows": [],
  "confidence": 0.92,
  "timestamp": "2025-12-10T12:00:00Z",
  "target_in_owner_chain": true,
  "warnings": ["MOCK_MODE: This response is deterministic for testing"],
  "needs_human_review": false,
  "human_review_reason": null,
  "validation_attempts_history": []
}
```

### Configuration for AIAnalysis Integration Tests

```yaml
# podman-compose.test.yml
holmesgpt-api:
  environment:
    - MOCK_LLM_MODE=true
    # No LLM_PROVIDER, LLM_MODEL, or API keys needed
```

### What Gets Validated (Even in Mock Mode)

1. ✅ Request schema (all 14+ required fields for IncidentRequest)
2. ✅ Pydantic model validation
3. ✅ Authentication middleware (if enabled)
4. ✅ Response schema compliance

### What Gets Bypassed

1. ❌ HolmesGPT SDK initialization
2. ❌ LLM API calls
3. ❌ Tool execution (kubectl, etc.)
4. ❌ Workflow catalog MCP search

---

## Business Requirement

**BR-HAPI-212**: Mock LLM Mode for Integration Testing

**Priority**: P1 (HIGH) - Blocks AIAnalysis integration testing

**Acceptance Criteria**:
- [x] `MOCK_LLM_MODE=true` environment variable enables mock mode
- [x] Mock responses are schema-compliant (pass IncidentResponse validation)
- [x] Mock responses are deterministic based on input signal_type
- [x] Request validation still runs (catches invalid requests)
- [x] No LLM API calls made when mock mode enabled
- [x] Works with both `/incident/analyze` and `/recovery/analyze` endpoints

**Note**: `/postexec/analyze` endpoint is **not exposed in V1.0** per DD-017 (Effectiveness Monitor V1.1 Deferral). Mock mode for postexec will be added in V1.1.

---

## Timeline

- **Implementation**: 2025-12-10 (today)
- **Available for testing**: Immediately after merge

---

## Usage Example for AIAnalysis

```go
// In AIAnalysis integration test setup
func setupHAPIContainer() {
    // Start HAPI with mock mode
    hapiContainer := testcontainers.NewContainer(
        Image: "holmesgpt-api:latest",
        Env: map[string]string{
            "MOCK_LLM_MODE": "true",
        },
        ExposedPorts: []string{"8080/tcp"},
    )
}

func TestIncidentAnalysis_SchemaCompliance(t *testing.T) {
    // This test validates AIAnalysis sends correct schema to HAPI
    // HAPI mock mode returns deterministic response without LLM

    request := &IncidentRequest{
        IncidentID: "test-123",
        SignalType: "OOMKilled",
        // ... all 14 required fields ...
    }

    response, err := hapiClient.AnalyzeIncident(ctx, request)
    require.NoError(t, err)

    // Verify response structure
    assert.NotNil(t, response.SelectedWorkflow)
    assert.Equal(t, "mock-oomkill-increase-memory-v1", response.SelectedWorkflow.WorkflowID)
    assert.Equal(t, 0.92, response.Confidence)
}
```

---

## Files Modified

1. `holmesgpt-api/src/extensions/incident.py` - Add mock mode check (BR-HAPI-212)
2. `holmesgpt-api/src/extensions/recovery.py` - Add mock mode check (BR-HAPI-212)
3. `holmesgpt-api/src/mock_responses.py` - NEW: Mock response generator
4. `holmesgpt-api/src/main.py` - Disable postexec endpoint for V1.0 (DD-017)
5. `holmesgpt-api/tests/unit/test_mock_mode.py` - NEW: 20+ unit tests for mock mode
6. `holmesgpt-api/tests/e2e/test_real_llm_integration.py` - Skip TestRealPostExecAnalysis class (DD-017)

---

## V1.0 Endpoint Availability

| Endpoint | V1.0 Status | Mock Mode |
|----------|-------------|-----------|
| `POST /api/v1/incident/analyze` | ✅ Available | ✅ Supported |
| `POST /api/v1/recovery/analyze` | ✅ Available | ✅ Supported |
| `POST /api/v1/postexec/analyze` | ⏸️ Deferred to V1.1 | N/A |
| `GET /health`, `/ready`, `/metrics` | ✅ Available | N/A (no LLM) |

**Reason for postexec deferral**: Per DD-017, the Effectiveness Monitor service (the only consumer of `/postexec/analyze`) is deferred to V1.1. Rather than expose an untested/unused endpoint, HAPI disables it until V1.1.

---

**Contact**: HAPI Team
**Status**: ✅ **COMPLETE** - Ready for AIAnalysis integration testing


