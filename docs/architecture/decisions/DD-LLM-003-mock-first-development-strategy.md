# DD-LLM-003: Mock-First Development Strategy for LLM Integration

## Status
**✅ APPROVED** (2025-12-11)
**Last Reviewed**: 2025-12-11
**Confidence**: 90%

## Context & Problem

**Problem**: Current GPU hardware performance makes iterative LLM testing impractical for development cycles.

**Performance Analysis** (Remote Ollama via SSH Tunnel):
- Model: Devstral-Small-2-24B (33 GB, Q4_K_M quantization)
- GPU Utilization: 31% CPU / 69% GPU
- Context Window: 65,536 tokens maximum

**Observed Response Times**:
- 2K tokens:  ~5-8 minutes
- 10K tokens: ~25-35 minutes
- 40K tokens: ~2-2.5 hours ⚠️
- 65K tokens: ~4 hours

**Key Issues**:
1. 2.5-hour test iterations for realistic 40K token contexts
2. SSH tunnel adds 50-100ms latency per API call
3. 33 GB model size creates memory-bound workload
4. Cannot afford multi-hour waits for each code change

**Business Impact**: Holmesgpt-api development blocked by LLM response times.

## Decision

**APPROVED**: Mock-First Development Strategy

Use mock LLM responses during development, validate with real LLM only for final integration testing.

**Rationale**:
1. **Unblocks Development**: Instant feedback (<100ms) vs 2.5-hour waits
2. **Cost-Effective**: Zero API costs during development
3. **Privacy-Safe**: No external data transmission
4. **Flexible**: Defers production LLM decision (cloud vs self-hosted)
5. **Quality Gate**: Final validation ensures production readiness

## Implementation

### Phase 1: Mock LLM Development (Week 1-2)
- Use MOCK_WORKFLOWS in workflow_catalog.py (already implemented)
- Mock LLM responses match HolmesGPT SDK InvestigationResult structure
- Unit/integration tests use mocks exclusively

### Phase 2: Component Integration (Week 2-3)
- Test RemediationRequest CRD payload generation with mocks
- Validate workflow catalog toolset invocation
- Test end-to-end pipeline with predictable responses

### Phase 3: Final Validation with Real LLM (Week 4)
- Single comprehensive test with Claude 3.5 Haiku (recommended) or Ollama
- Verify workflow catalog toolset invoked by real LLM
- Document performance metrics for production planning

## Business Requirements

### BR-HAPI-251: Mock-First Development Velocity
**Requirement**: Development MUST NOT be blocked by LLM infrastructure performance.

**Acceptance Criteria**:
- ✅ Mock responses <100ms
- ✅ Mock structure matches real LLM format
- ✅ Developers iterate without GPU access

**Priority**: P0 (Critical)

### BR-HAPI-252: Production LLM Validation Gate
**Requirement**: All features MUST be validated with real LLM before production.

**Acceptance Criteria**:
- ✅ Comprehensive integration test with real LLM passes
- ✅ Workflow catalog toolset invoked successfully
- ✅ Performance metrics documented

**Priority**: P0 (Critical)

### BR-HAPI-253: LLM Provider Decision Deferral
**Requirement**: Production LLM decision deferred until validation complete.

**Acceptance Criteria**:
- ✅ Architecture supports both cloud and self-hosted
- ✅ Configuration allows switching providers
- ✅ Performance benchmarks documented

**Priority**: P1 (Important)

## Consequences

### Positive
- ✅ Fast development: <100ms vs 2.5 hours
- ✅ Zero API costs during development
- ✅ Privacy-safe (no external transmission)
- ✅ Parallel testing enabled
- ✅ Deferred LLM decision

### Negative
- ⚠️ Mock drift risk - **Mitigation**: Final validation catches drift
- ⚠️ Delayed real-world validation - **Mitigation**: Comprehensive final test

## Validation Results

### Performance Comparison
| Test Type | Mock LLM | Real Ollama | Real Claude |
|-----------|----------|-------------|-------------|
| 2K tokens  | <100ms   | 5-8 min     | 10-15s      |
| 10K tokens | <100ms   | 25-35 min   | 15-25s      |
| 40K tokens | <100ms   | 2-2.5 hours | 20-30s      |

### Hardware Analysis
**Current GPU**: ❌ Not suitable for production (2.5 hours for 40K tokens)

**Production Options** (decide after final validation):
1. **Cloud API**: 20-30s for 40K tokens, ~$0.25-0.50 per incident
2. **Self-Hosted GPU**: 8-12 minutes, hardware cost ~$1,600
3. **Context Reduction**: Limit to 10K tokens, 25-35 min (acceptable for MVP)

## Related Decisions
- **Builds On**: ADR-041 (LLM Prompt and Response Contract)
- **Builds On**: DD-WORKFLOW-002 (MCP Architecture - superseded by native toolset)
- **Supports**: BR-HAPI-250 (Workflow Catalog Search Tool)

## Quick Reference

### Development Testing
```bash
# Fast tests with mocks
pytest holmesgpt-api/tests/unit/ -v
pytest holmesgpt-api/tests/integration/ -v
```

### Final Validation (Week 4)
```bash
# Real LLM test (2.5 hours Ollama / 30s Claude)
export ANTHROPIC_API_KEY="sk-..."
pytest holmesgpt-api/tests/validation/test_real_llm_integration.py -v
```

---

**Priority**: CRITICAL - Unblocks MVP development

**Next Steps**:
1. Continue with mock LLM responses
2. Complete unit/integration tests
3. Schedule final validation (Week 4)
4. Decide production LLM provider





