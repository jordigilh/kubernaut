# DD-CONTEXT-001: Context Enrichment Placement - LLM-Driven Tool Call Pattern

**Date**: 2025-10-22
**Status**: ✅ **APPROVED** (Approach B)
**Decision Maker**: User (Jordi Gil)
**Confidence**: 90%
**Impact**: High - Affects AIAnalysis, HolmesGPT API, and Context API integration

---

## 🎯 Decision Summary

**Context enrichment will be implemented as an LLM-driven tool call within the HolmesGPT API Service, NOT as pre-enrichment in the AIAnalysis Controller.**

**Pattern**: AIAnalysis → HolmesGPT API → Context API (LLM-driven tool call)

---

## 📋 Context

### Problem Statement

During implementation planning for the AIAnalysis Controller, a critical architectural question arose:

**Where should context enrichment happen?**

Two approaches were considered:
1. **Approach A**: AIAnalysis Controller pre-fetches context from Context API, then sends enriched request to HolmesGPT API
2. **Approach B**: HolmesGPT API exposes Context API as a tool, allowing the LLM to request context on-demand

### Business Requirements Affected

- **BR-AI-002**: Context Enrichment Integration (original scope)
- **BR-HAPI-XXX**: Context API Tool Integration (new scope for HolmesGPT API)
- **BR-CONTEXT-XXX**: Tool Call Support (Context API must support tool call pattern)

---

## 🔍 Analysis

### Approach A: Pre-Enrichment in AIAnalysis Controller

**Architecture**:
```
RemediationProcessing CRD
    ↓
AIAnalysis Controller
    ↓ (fetch context)
Context API Service
    ↓ (return enriched context)
AIAnalysis Controller
    ↓ (send investigation request with context)
HolmesGPT API Service
    ↓ (investigate with provided context)
LLM (Claude/Vertex AI)
```

**Pros**:
- ✅ Simpler implementation (no tool orchestration)
- ✅ Lower latency (parallel context fetch)
- ✅ Easier to test and debug
- ✅ Clearer observability (separate steps)
- ✅ Simpler failure handling

**Cons**:
- ❌ **Violates LLM autonomy principle** - Forces context even when not needed
- ❌ **Higher token cost** - Context in every request (~730 tokens per DD-HOLMESGPT-009)
- ❌ **Wastes LLM intelligence** - Pre-decides what LLM needs
- ❌ **Not aligned with HolmesGPT SDK design** - SDK supports tool calls natively
- ❌ **Inflexible** - Cannot adapt to different investigation types

**Confidence**: 65% ⚠️

---

### Approach B: LLM-Driven Tool Call in HolmesGPT API (APPROVED)

**Architecture**:
```
RemediationProcessing CRD
    ↓
AIAnalysis Controller
    ↓ (send investigation request)
HolmesGPT API Service
    ↓ (LLM decides if context needed)
LLM (Claude/Vertex AI)
    ↓ (tool call: get_context)
HolmesGPT API → Context API Service
    ↓ (return context to LLM)
LLM continues investigation with context
```

**Pros**:
- ✅ **LLM-driven decision making** - LLM decides if context needed
- ✅ **Token efficiency** - Context only when LLM requests it
- ✅ **Cost optimization** - Saves ~$1,000/year when context not needed
- ✅ **Aligned with HolmesGPT SDK** - Native tool call pattern
- ✅ **Flexible** - LLM can request different context types
- ✅ **Intelligent** - Leverages LLM's reasoning about what data it needs

**Cons**:
- ⚠️ Higher latency (+1-2s for tool call round-trip)
- ⚠️ More complex implementation (tool orchestration in HolmesGPT API)
- ⚠️ Harder to test (LLM non-determinism)
- ⚠️ More complex observability (tool call tracing)

**Confidence**: 90% ✅

---

## 📊 Comparative Analysis

| Aspect | Approach A (Pre-Enrichment) | Approach B (Tool Call) | Winner |
|---|---|---|---|
| **LLM Autonomy** | ❌ Low - Context forced | ✅ High - LLM decides | **B** |
| **Token Efficiency** | ⚠️ Medium - Always sends context | ✅ High - Context only when needed | **B** |
| **Cost** | ⚠️ Higher - Context in every request | ✅ Lower - Context on-demand | **B** |
| **Latency** | ✅ Lower - Parallel fetch | ⚠️ Higher - Sequential tool call | **A** |
| **Complexity** | ✅ Lower - Simple flow | ⚠️ Higher - Tool orchestration | **A** |
| **LLM Intelligence** | ❌ Bypassed - Pre-decided | ✅ Utilized - LLM decides | **B** |
| **Failure Handling** | ✅ Simpler - Fail before LLM | ⚠️ Complex - Mid-investigation failure | **A** |
| **Alignment with HolmesGPT** | ⚠️ Wrapper pattern | ✅ Native tool pattern | **B** |
| **Observability** | ✅ Clear - Separate steps | ⚠️ Complex - Tool call tracing | **A** |
| **Testability** | ✅ Easier - Separate concerns | ⚠️ Harder - LLM behavior testing | **A** |

**Score**: Approach B wins **6-4**

---

## 💰 Cost Analysis

### Token Cost Comparison

**Approach A (Pre-Enrichment)**:
- Context included in every investigation request
- Context size: ~730 tokens (per DD-HOLMESGPT-009 ultra-compact format)
- Annual investigations: ~10,000
- **Annual token cost**: 730 tokens × 10,000 = 7.3M tokens = **~$2,555/year** (at $0.35/1M tokens)

**Approach B (Tool Call)**:
- Context only when LLM requests it
- Estimated context request rate: 60% of investigations (LLM skips for simple cases)
- Annual context requests: ~6,000
- **Annual token cost**: 730 tokens × 6,000 = 4.4M tokens = **~$1,540/year**
- **Tool call overhead**: ~50 tokens per tool call × 6,000 = 300K tokens = **~$105/year**
- **Total**: ~$1,645/year

**Annual Savings**: $2,555 - $1,645 = **~$910/year** (36% reduction)

### Latency Analysis

**Approach A**:
- Context API call: ~200ms (p50), ~500ms (p95)
- Total investigation latency: 3-5s (parallel context fetch)

**Approach B**:
- Investigation start: 0ms (no pre-fetch)
- LLM decides context needed: ~500ms
- Context API tool call: ~200ms (p50), ~500ms (p95)
- LLM continues with context: ~2-3s
- **Total investigation latency**: 3.5-5.5s (sequential tool call)
- **Latency increase**: ~500ms-1s (p95)

**Trade-off**: +500ms-1s latency for 36% cost savings and LLM autonomy

---

## 🎯 Decision

**APPROVED: Approach B - LLM-Driven Tool Call Pattern**

**Rationale**:
1. **Architecturally Superior**: Leverages LLM intelligence, not bypasses it
2. **Cost Efficient**: Saves ~$910/year in token costs (36% reduction)
3. **Aligned with HolmesGPT SDK**: Native tool call pattern
4. **More Flexible**: LLM can request different context types (historical, cluster state, etc.)
5. **Better User Experience**: Faster for simple investigations (no context overhead)
6. **Scalable**: LLM can adaptively request context based on investigation complexity

**Acceptable Trade-offs**:
- +500ms-1s latency (still within 5s SLA for investigations)
- Slightly more complex implementation (tool orchestration)
- More complex observability (tool call tracing)

---

## 📋 Implementation Requirements

### 1. HolmesGPT API Service (New Scope)

**Add BR-HAPI-031 to BR-HAPI-040**: Context API Tool Integration

**BR-HAPI-031**: Define `get_context` Tool
```python
# HolmesGPT API tool definition
tools = [
    {
        "name": "get_context",
        "description": "Retrieve historical context for similar incidents. Use when investigation requires understanding of past similar alerts, success rates, or patterns.",
        "parameters": {
            "type": "object",
            "properties": {
                "alert_fingerprint": {
                    "type": "string",
                    "description": "Fingerprint of the current alert"
                },
                "similarity_threshold": {
                    "type": "number",
                    "description": "Minimum similarity score (0.0-1.0), default 0.70"
                },
                "context_types": {
                    "type": "array",
                    "items": {"enum": ["historical_remediations", "cluster_patterns", "success_rates"]},
                    "description": "Types of context to retrieve"
                }
            },
            "required": ["alert_fingerprint"]
        }
    }
]
```

**BR-HAPI-032**: Implement Context API Client
- HTTP client for Context API REST endpoint
- Retry logic with exponential backoff
- Circuit breaker for Context API failures
- Caching of context results within investigation session

**BR-HAPI-033**: Tool Call Handler
- Parse LLM tool call requests
- Invoke Context API with tool parameters
- Format context response for LLM consumption
- Handle tool call failures gracefully (degraded mode)

**BR-HAPI-034**: Tool Call Observability
- Metrics: `context_tool_call_rate`, `context_tool_call_latency`, `context_tool_call_errors`
- Logging: Tool call requests, responses, and failures
- Tracing: OpenTelemetry spans for tool calls

**BR-HAPI-035**: Tool Call Testing
- Unit tests: Tool definition, parameter validation
- Integration tests: Real Context API tool calls
- E2E tests: LLM-driven tool call scenarios

**Implementation Files**:
- `holmesgpt-api/src/tools/context_tool.py` - Tool definition and handler
- `holmesgpt-api/src/clients/context_api_client.py` - Context API HTTP client
- `holmesgpt-api/tests/integration/test_context_tool.py` - Integration tests

---

### 2. AIAnalysis Controller (Scope Reduction)

**Remove BR-AI-002**: Context Enrichment Integration (moved to HolmesGPT API)

**Add BR-AI-002 (Revised)**: Tool Call Monitoring and Observability
- Monitor HolmesGPT tool calls to Context API (via HolmesGPT API metrics)
- Track investigation quality with/without context
- Alert on low context usage rate (<40%) or high context usage rate (>80%)
- Metrics: `investigation_with_context_rate`, `investigation_quality_by_context_usage`

**Remove**:
- ❌ Context API client in AIAnalysis controller
- ❌ Pre-enrichment logic before investigation
- ❌ Context caching in AIAnalysis controller

**Simplify**:
- ✅ AIAnalysis controller only sends investigation request to HolmesGPT API
- ✅ HolmesGPT API handles all tool orchestration
- ✅ AIAnalysis controller monitors investigation outcomes

**Implementation Files**:
- Remove: `pkg/aianalysis/context/enricher.go`
- Remove: `pkg/aianalysis/context/client.go`
- Update: `pkg/aianalysis/investigation/investigator.go` (remove pre-enrichment)

---

### 3. Context API Service (No Changes Required)

**Existing REST API Already Supports Tool Call Pattern**:
- ✅ `/api/v1/context/enrich` endpoint accepts alert fingerprint
- ✅ Returns enriched context in JSON format
- ✅ Supports similarity threshold parameter
- ✅ <500ms p95 latency (meets tool call requirements)

**No Implementation Changes Needed**:
- Context API is already designed as a stateless REST service
- Tool call pattern is just another HTTP client (HolmesGPT API instead of AIAnalysis)

---

## ⚠️ Risk Mitigation

### Risk 1: Increased Latency (+500ms-1s)

**Mitigation**:
- Context API must maintain <500ms p95 latency (already in design)
- HolmesGPT API caches recent context lookups (1h TTL)
- Monitor p95 investigation latency, alert if >5s

**Fallback**:
- If Context API latency exceeds 1s, HolmesGPT API returns cached context
- If no cache available, LLM continues without context (degraded mode)

---

### Risk 2: Tool Call Failures

**Mitigation**:
- HolmesGPT API handles tool failures gracefully (try-catch around tool calls)
- Circuit breaker opens after 50% failure rate (5-minute window)
- LLM receives error message and continues investigation without context

**Fallback**:
- LLM can retry tool call with different parameters
- LLM can proceed without context (investigation quality may be lower)
- AIAnalysis controller monitors investigation quality, escalates if <60% confidence

---

### Risk 3: LLM May Not Request Context When Needed

**Mitigation**:
- Tool description emphasizes when context is valuable:
  - "Use when investigation requires understanding of past similar alerts"
  - "Recommended for complex cascading failures or recurring issues"
- Monitor investigation quality with/without context tool calls
- Alert if investigations without context have <70% confidence rate

**Monitoring**:
- Track: `investigation_quality_by_context_usage`
- Alert: If investigations without context have >30% low confidence rate
- Action: Refine tool description to encourage context usage

---

### Risk 4: Context API Overload from Tool Calls

**Mitigation**:
- Context API already designed for high throughput (>100 req/s)
- HolmesGPT API caches context results (reduces duplicate requests)
- Rate limiting in HolmesGPT API (max 10 tool calls per investigation)

**Monitoring**:
- Track: `context_api_request_rate`, `context_api_p95_latency`
- Alert: If Context API p95 latency >500ms or error rate >5%
- Action: Scale Context API horizontally or increase cache TTL

---

## 📊 Success Metrics

### Performance Metrics

**Target**:
- Investigation latency p95: <5s (acceptable with +500ms tool call overhead)
- Context tool call latency p95: <500ms
- Context tool call success rate: >95%

**Monitoring**:
- `holmesgpt_investigation_duration_seconds` (histogram)
- `holmesgpt_context_tool_call_duration_seconds` (histogram)
- `holmesgpt_context_tool_call_success_rate` (gauge)

---

### Cost Metrics

**Target**:
- Token cost reduction: >30% compared to Approach A
- Annual savings: >$900/year

**Monitoring**:
- `holmesgpt_tokens_used_total` (counter, labeled by `context_included=true/false`)
- `holmesgpt_investigation_cost_dollars` (counter)

---

### Quality Metrics

**Target**:
- Investigation confidence: >80% average
- Context usage rate: 50-70% (LLM requests context when needed)
- Investigation quality with context: >85% confidence
- Investigation quality without context: >75% confidence

**Monitoring**:
- `aianalysis_investigation_confidence_score` (histogram, labeled by `context_used=true/false`)
- `holmesgpt_context_tool_call_rate` (gauge)

---

## 📅 Timeline Impact

**HolmesGPT API**:
- +1 day: Implement `get_context` tool definition and handler
- +1 day: Implement Context API client with retry/circuit breaker
- +1 day: Integration tests for tool call scenarios
- **Total**: +3 days

**AIAnalysis Controller**:
- -1 day: Remove Context API client and pre-enrichment logic
- +0.5 day: Update investigation flow (simpler)
- +0.5 day: Add tool call monitoring metrics
- **Total**: Net zero (simplification offsets monitoring work)

**Net Timeline Impact**: +3 days (HolmesGPT API only)

---

## 🔗 Related Decisions

- **DD-HOLMESGPT-009**: Ultra-Compact JSON Format (context format when returned to LLM)
- **DD-CONTEXT-XXX**: Context API REST Design (already supports tool call pattern)
- **ADR-019**: HolmesGPT Retry Strategy (applies to tool call retries)

---

## 📋 Action Items

### Immediate (Before Implementation)

1. ✅ **Update AIAnalysis Implementation Plan v1.1.1**:
   - Remove BR-AI-002 (Context Enrichment Integration)
   - Add BR-AI-002 (Revised): Tool Call Monitoring
   - Remove context enrichment from Day 2 implementation
   - Update edge cases to reflect tool call pattern

2. ✅ **Create BR-HAPI-031 to BR-HAPI-035** for HolmesGPT API:
   - Document Context API tool integration requirements
   - Add to HolmesGPT API implementation plan

3. ⏸️ **Update Context API Documentation**:
   - Document tool call usage pattern
   - Add example tool call request/response

### During Implementation

4. ⏸️ **Implement HolmesGPT API Context Tool** (Days 1-3):
   - Tool definition and handler
   - Context API client
   - Integration tests

5. ⏸️ **Update AIAnalysis Controller** (Day 1):
   - Remove context enrichment logic
   - Simplify investigation flow
   - Add tool call monitoring

6. ⏸️ **Integration Testing** (Day 4):
   - E2E test: LLM-driven context tool call
   - Verify tool call latency <500ms p95
   - Verify investigation quality with/without context

### Post-Implementation

7. ⏸️ **Monitor and Tune** (Weeks 1-4):
   - Track context tool call rate (target: 50-70%)
   - Track investigation quality by context usage
   - Tune tool description if needed

8. ⏸️ **Cost Validation** (Month 1):
   - Verify token cost reduction >30%
   - Verify annual savings >$900/year

---

## 📝 Approval

**Decision**: ✅ **APPROVED - Approach B (LLM-Driven Tool Call Pattern)**

**Approved By**: Jordi Gil (User)
**Date**: 2025-10-22
**Confidence**: 90%

**Rationale**: Approach B is architecturally superior, cost-efficient, and aligned with LLM-native tool call patterns. The +500ms-1s latency trade-off is acceptable for 36% cost savings and LLM autonomy.

**Next Steps**:
1. Update AIAnalysis implementation plan (remove Approach A)
2. Create HolmesGPT API BRs for Context API tool integration
3. Begin implementation with HolmesGPT API tool development

---

**Document Version**: 1.0
**Last Updated**: 2025-10-22
**Status**: ✅ **APPROVED AND ACTIVE**

