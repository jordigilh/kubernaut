# DD-CONTEXT-002: BR-AI-002 Ownership - Keep in AIAnalysis or Move to HolmesGPT API?

**Date**: 2025-10-22
**Status**: ✅ **DECISION: Keep BR-AI-002 in AIAnalysis (Revised Scope)**
**Confidence**: 85%
**Related**: DD-CONTEXT-001 (Context Enrichment Placement)

---

## 🎯 Question

After approving Approach B (LLM-driven tool call pattern) in DD-CONTEXT-001, should BR-AI-002 (Context Enrichment Integration) be:

**Option A**: Kept in AIAnalysis implementation plan (revised scope)
**Option B**: Moved to HolmesGPT API as BR-HAPI-031+

---

## 📊 Analysis

### Option A: Keep BR-AI-002 in AIAnalysis (Revised Scope)

**New Scope**: Tool Call Monitoring and Observability

**Rationale**:
- AIAnalysis Controller is responsible for investigation **outcomes**
- AIAnalysis Controller monitors investigation quality
- Tool call usage affects investigation quality → AIAnalysis should monitor it
- BR-AI-002 represents the **business requirement** for context integration, regardless of implementation location

**Revised BR-AI-002 Scope**:
```
BR-AI-002: Context Integration Monitoring

Requirement: System must monitor HolmesGPT tool calls to Context API and track investigation quality by context usage.

Responsibilities:
- Monitor context tool call rate (target: 50-70%)
- Track investigation confidence by context usage
- Alert on anomalous context usage patterns
- Ensure Context API integration improves investigation quality

Implementation: AIAnalysis Controller observability layer
```

**Confidence**: **85%** ✅

**Pros**:
- ✅ Maintains BR continuity (BR-AI-002 still exists, just revised scope)
- ✅ AIAnalysis owns investigation quality → should monitor factors affecting quality
- ✅ Separates concerns: HolmesGPT implements tool, AIAnalysis monitors outcomes
- ✅ Business requirement stays with business outcome owner (AIAnalysis)

**Cons**:
- ⚠️ BR-AI-002 no longer involves direct Context API integration
- ⚠️ May confuse readers expecting AIAnalysis to call Context API

---

### Option B: Move to HolmesGPT API as BR-HAPI-031+

**New Scope**: Context API Tool Implementation

**Rationale**:
- HolmesGPT API implements the actual Context API integration
- Tool definition, handler, and client all live in HolmesGPT API
- BR should be owned by the service that implements it

**New BR-HAPI-031 to BR-HAPI-035**:
```
BR-HAPI-031: Define get_context Tool
BR-HAPI-032: Implement Context API Client
BR-HAPI-033: Tool Call Handler
BR-HAPI-034: Tool Call Observability
BR-HAPI-035: Tool Call Testing
```

**Confidence**: **70%** ⚠️

**Pros**:
- ✅ BR ownership matches implementation ownership
- ✅ Clear that HolmesGPT API implements Context API integration
- ✅ All tool-related BRs in one place (HolmesGPT API)

**Cons**:
- ❌ Breaks BR continuity (BR-AI-002 disappears from AIAnalysis)
- ❌ AIAnalysis loses visibility into context integration requirement
- ❌ Business requirement split across two services (confusing)
- ❌ AIAnalysis still needs to monitor context usage → partial BR ownership

---

## 🎯 Decision: **Option A - Keep BR-AI-002 in AIAnalysis (Revised Scope)**

**Confidence**: **85%** ✅

### Rationale

**1. Business Requirement Ownership**
- BR-AI-002 represents the **business need** for context integration in AI investigations
- The business outcome (investigation quality) is owned by AIAnalysis Controller
- Implementation location (HolmesGPT API) is a technical detail, not a business concern

**2. Separation of Concerns**
- **HolmesGPT API**: Implements context tool (BR-HAPI-031 to BR-HAPI-035)
- **AIAnalysis Controller**: Monitors investigation outcomes including context usage (BR-AI-002)
- Clear separation: Implementation vs Monitoring

**3. BR Continuity**
- AIAnalysis implementation plan already references BR-AI-002
- Revising scope is less disruptive than removing and creating new BRs
- Maintains traceability: BR-AI-002 always meant "context integration for AI investigations"

**4. Practical Monitoring**
- AIAnalysis Controller needs to monitor context tool call rate anyway
- AIAnalysis Controller tracks investigation quality by context usage
- BR-AI-002 (revised) captures these monitoring requirements

---

## 📋 Implementation

### AIAnalysis Implementation Plan

**BR-AI-002 (Revised): Context Integration Monitoring**

**Requirement**: System must monitor HolmesGPT tool calls to Context API and ensure context integration improves investigation quality.

**Unit Test Coverage**:
- ✅ `test/unit/aianalysis/monitoring_test.go::MonitorContextToolCallRate`
- ✅ `test/unit/aianalysis/monitoring_test.go::TrackInvestigationQualityByContext`
- ✅ `test/unit/aianalysis/monitoring_test.go::AlertOnAnomalousContextUsage`

**Integration Test Coverage**:
- ✅ `test/integration/aianalysis/context_monitoring_test.go::ContextToolCallRateMonitoring`
- ✅ `test/integration/aianalysis/context_monitoring_test.go::InvestigationQualityByContextUsage`

**Implementation**: `pkg/aianalysis/monitoring/context_monitor.go`

**Edge Cases Covered**:
- Context tool call rate too low (<40%) → Business outcome: Alert ops team, investigate tool description
- Context tool call rate too high (>80%) → Business outcome: Investigate if LLM over-relying on context
- Investigation quality lower without context → Business outcome: Validate context tool is providing value
- Investigation quality same with/without context → Business outcome: Question if context tool is needed
- HolmesGPT API tool call metrics unavailable → Business outcome: Fallback to investigation outcome tracking only
- Context API failures visible in tool call errors → Business outcome: Correlate investigation failures with Context API health

**Metrics**:
- `aianalysis_context_tool_call_rate` (gauge) - % of investigations using context tool
- `aianalysis_investigation_confidence_by_context` (histogram, labeled by `context_used=true/false`)
- `aianalysis_context_tool_call_anomaly_alerts` (counter)

**Responsibilities**:
1. Query HolmesGPT API metrics for tool call rates
2. Track investigation confidence by context usage
3. Alert on anomalous patterns (too low/high context usage)
4. Validate context tool improves investigation quality
5. Provide observability dashboard for context integration health

---

### HolmesGPT API Implementation Plan

**BR-HAPI-031 to BR-HAPI-035: Context API Tool Integration**

**BR-HAPI-031**: Define `get_context` Tool
- Tool definition with parameters (alert_fingerprint, similarity_threshold, context_types)
- Tool description emphasizing when context is valuable
- Parameter validation

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
- Metrics: `holmesgpt_context_tool_call_rate`, `holmesgpt_context_tool_call_latency`, `holmesgpt_context_tool_call_errors`
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

## 📊 Comparison

| Aspect | Option A (Keep in AIAnalysis) | Option B (Move to HolmesGPT) |
|---|---|---|
| **BR Continuity** | ✅ Maintained | ❌ Broken |
| **Business Ownership** | ✅ Clear (AIAnalysis owns outcomes) | ⚠️ Split (implementation vs monitoring) |
| **Implementation Clarity** | ✅ Clear (HolmesGPT implements, AIAnalysis monitors) | ⚠️ Confusing (BR in HolmesGPT, monitoring in AIAnalysis) |
| **Traceability** | ✅ BR-AI-002 always meant context integration | ❌ BR-AI-002 disappears |
| **Separation of Concerns** | ✅ Implementation vs Monitoring | ⚠️ Implementation only |
| **Monitoring Requirements** | ✅ Captured in BR-AI-002 | ❌ Not captured in BR-HAPI-031+ |

**Winner**: Option A (Keep in AIAnalysis) - **85% confidence**

---

## 🎯 Final Decision

**Keep BR-AI-002 in AIAnalysis Implementation Plan with Revised Scope**

**Revised BR-AI-002**: Context Integration Monitoring

**Rationale**:
1. Business requirement ownership stays with business outcome owner (AIAnalysis)
2. Maintains BR continuity and traceability
3. Clear separation: HolmesGPT implements (BR-HAPI-031+), AIAnalysis monitors (BR-AI-002)
4. AIAnalysis needs to monitor context usage anyway → BR-AI-002 captures this requirement

**Action Items**:
1. ✅ Update AIAnalysis plan: Revise BR-AI-002 scope to monitoring
2. ✅ Create BR-HAPI-031 to BR-HAPI-035 in HolmesGPT API plan
3. ✅ Remove Approach A (pre-enrichment) from AIAnalysis plan
4. ✅ Update edge cases to reflect tool call monitoring pattern

---

**Document Version**: 1.0
**Last Updated**: 2025-10-22
**Status**: ✅ **APPROVED**
**Confidence**: 85%







