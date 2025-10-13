# DD-CONTEXT-001: REST API vs RAG Architecture

**Date**: 2025-10-13
**Status**: ✅ APPROVED
**Decision**: Context API will be a REST API service, NOT a RAG (Retrieval-Augmented Generation) system
**Confidence**: 98%

---

## Context

During implementation planning for Context API (Phase 2 - Intelligence Layer), the question arose:

**Should Context API be:**
- **Option A**: REST API providing raw historical incident data
- **Option B**: RAG system providing AI-analyzed recommendations

**Initial Consideration**: Since Workflow Engine has no LLM capacity, it seemed Context API should provide intelligent analysis via RAG.

**Critical Discovery**: Misunderstanding of the V1 CRD architecture. The **tool-based LLM architecture** requires Context API to be a simple, predictable REST endpoint.

---

## Decision

**APPROVED**: Context API will be a **REST API service** (Option A)

---

## Architectural Rationale

### 1. Tool-Based LLM Architecture (Critical Factor)

**V1 Architecture** (from [APPROVED_MICROSERVICES_ARCHITECTURE.md](../../../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md)):

```
AIAnalysis Service (Phase 4)
    ↓ Calls
HolmesGPT API Service (Phase 2)
    ↓ Orchestrates LLM + Tools
LLM decides to call tools:
    - query_historical_incidents() ← Context API endpoint
    - get_cluster_state() ← K8s API
    - search_logs() ← Logging system
    ↓ LLM receives raw data from tools
LLM synthesizes data and generates analysis
```

**Key Insight**: Context API is an **LLM tool endpoint**, not a standalone intelligence service.

**Why This Matters**:
- ✅ LLM must decide WHEN to query historical data
- ✅ LLM must decide WHAT parameters to use
- ✅ LLM must synthesize MULTIPLE tool results
- ✅ Context API provides RAW data, LLM does reasoning

**Example LLM Reasoning**:
```
LLM: "I need to understand if this pod crash is a pattern"
LLM: *calls tool* query_historical_incidents(alert_name="PodCrashLoopBackOff")
Context API: Returns [{incident1}, {incident2}, ...]  (RAW DATA)
LLM: "I see 10 similar incidents, all OOMKilled at 495Mi usage"
LLM: *calls tool* get_deployment_spec(name="app-xyz")
K8s API: Returns {memory_limit: "512Mi"}  (RAW DATA)
LLM: "Pattern detected: memory limit too low. Recommend: Increase to 1Gi"
```

### 2. Service Responsibility Separation

**Correct Service Boundaries** (V1 Architecture):

| Service | Responsibility | Has LLM? | Calls Context API? |
|---------|---------------|----------|-------------------|
| **Context API** | Provide historical data | ❌ No | N/A (is the endpoint) |
| **HolmesGPT API** | LLM tool orchestration | ✅ Yes (orchestrates) | ✅ Yes (as tool) |
| **AIAnalysis Service** | CRD-based AI workflow | ❌ No (delegates to HolmesGPT) | ❌ No (HolmesGPT calls it) |
| **WorkflowExecution** | Workflow orchestration | ❌ No | ❌ No (reads AIAnalysis CRD) |

**Single Responsibility Principle**:
- Context API: Data retrieval ✅
- HolmesGPT API: Tool orchestration ✅
- LLM: Reasoning and analysis ✅
- AIAnalysis Service: CRD lifecycle management ✅

**If Context API were RAG** (❌ Wrong):
- Context API: Data retrieval + AI analysis ❌ (violates SRP)
- HolmesGPT API: Tool orchestration + redundant analysis ❌
- LLM: Receives pre-analyzed data (loses autonomy) ❌

### 3. Tool Composability

**LLM Tool Orchestration Requirements**:

| Requirement | REST API | RAG | Impact |
|-------------|----------|-----|--------|
| **Deterministic output** | ✅ Same query → same data | ❌ LLM variance | CRITICAL |
| **Fast response** | ✅ <200ms | ❌ >2000ms | HIGH |
| **Structured data** | ✅ JSON array | ⚠️ Unstructured text | HIGH |
| **Composable** | ✅ LLM combines tools | ❌ Pre-analyzed | CRITICAL |
| **LLM autonomy** | ✅ LLM decides when to call | ❌ Always analyzes | CRITICAL |

**Tool Composition Example**:
```python
# LLM orchestrates multiple tools (only works with REST)
def investigate_pod_crash(signal):
    # LLM decides to gather context
    historical_data = query_historical_incidents(alert_name=signal.alert_name)
    cluster_state = get_cluster_state(namespace=signal.namespace)
    recent_logs = search_logs(pod_name=signal.pod_name, lines=100)

    # LLM synthesizes all raw data
    analysis = llm.analyze(
        signal=signal,
        historical=historical_data,  # RAW data from Context API
        cluster=cluster_state,       # RAW data from K8s API
        logs=recent_logs             # RAW data from Logs API
    )

    return analysis
```

**Why RAG Breaks Composition**:
- Context API RAG returns: "Root cause is OOMKilled, recommend increase memory"
- K8s API returns: "Memory limit is 512Mi" (raw data)
- Logs API returns: "OOM kill messages" (raw data)
- How does LLM combine pre-analyzed + raw data? ❌ Conflict

---

## Alternatives Considered

### Option B: RAG (Retrieval-Augmented Generation) ❌ REJECTED

**Description**: Context API would retrieve historical data AND generate AI analysis

**Advantages**:
- Single endpoint for intelligence
- Potentially faster for simple queries (no multi-tool orchestration)

**Critical Disadvantages** (Why Rejected):

1. **❌ Violates Tool-Based Architecture**
   - LLM loses autonomous decision-making
   - Pre-analyzed data conflicts with LLM reasoning
   - Cannot compose with other tools

2. **❌ Service Responsibility Overlap**
   - Context API does analysis → Overlaps with AIAnalysis Service
   - HolmesGPT API receives pre-analyzed data → Redundant analysis
   - Two services doing the same job (AI analysis)

3. **❌ Breaks LLM Tool Composability**
   - LLM cannot combine Context API RAG with other tools
   - Pre-analyzed recommendations conflict with LLM's own analysis
   - Non-deterministic tool responses (LLM variance)

4. **❌ Performance Issues**
   - RAG latency: >2000ms (LLM generation)
   - REST latency: <200ms (database query)
   - LLM waits synchronously for tools

5. **❌ Testing Complexity**
   - RAG: Non-deterministic LLM outputs
   - REST: Deterministic data queries
   - LLM testing belongs in HolmesGPT API, not Context API

**Confidence in Rejection**: 99%

---

## Implementation Consequences

### ✅ Positive Consequences

1. **Clear Service Boundaries**
   - Context API: Data retrieval only
   - HolmesGPT API: LLM orchestration
   - AIAnalysis Service: CRD management
   - No service overlap

2. **LLM Autonomy Preserved**
   - LLM decides when to query Context API
   - LLM decides what parameters to use
   - LLM synthesizes multiple tool results
   - Context API doesn't make decisions

3. **Tool Composability Enabled**
   - LLM can call: Context API + K8s API + Logs API
   - All tools return raw data
   - LLM does intelligent synthesis
   - Clean tool orchestration

4. **Performance Optimized**
   - REST API: <200ms response time
   - Synchronous tool calls efficient
   - No LLM inference overhead in Context API
   - Caching optimized for data queries

5. **Testing Simplified**
   - REST API: Standard HTTP tests
   - Deterministic responses
   - No LLM output validation needed
   - Clear pass/fail criteria

6. **Optimal Timeline**
   - REST API: 12 days implementation
   - RAG: 18 days (wasted effort)
   - +6 days saved

### ⚠️ Trade-offs Accepted

1. **Multi-Hop Calls**
   - AIAnalysis → HolmesGPT API → LLM → Context API
   - Accepted: Clean architecture worth extra hop
   - Mitigation: Async tool calls, aggressive caching

2. **No Direct Analysis**
   - Context API doesn't provide recommendations
   - Accepted: LLM should do reasoning, not Context API
   - Mitigation: Fast data retrieval enables quick LLM analysis

---

## Integration with V1 Architecture

### Phase 2: Intelligence Layer

**Context API** (12 days):
- REST API with 5 endpoints
- Historical incident retrieval
- Semantic search (pgvector)
- Query aggregation
- Multi-tier caching (Redis + L2)

**HolmesGPT API** (10 days):
- Python wrapper for HolmesGPT SDK
- LLM tool orchestration
- Calls Context API as a tool
- Multi-LLM support

### Phase 4: AI Integration

**AIAnalysis Service** (CRD Controller):
- Watches AIAnalysis CRD
- Calls HolmesGPT API /investigate
- HolmesGPT API orchestrates LLM + tools
- LLM calls Context API when needed
- Updates AIAnalysis CRD status

### Tool Configuration (Dynamic Toolset Service)

**Tool Definition** (Phase 1):
```yaml
# ConfigMap: holmesgpt-tools
tools:
  - name: query_historical_incidents
    description: "Query historical incident data by alert name, namespace, or time range"
    endpoint: "http://context-api:8080/api/v1/incidents"
    method: GET
    parameters:
      - name: alert_name
        type: string
        description: "Kubernetes alert name"
        required: false
      - name: namespace
        type: string
        description: "Kubernetes namespace"
        required: false
      - name: limit
        type: integer
        description: "Maximum results"
        default: 10
    response_schema:
      type: array
      items:
        type: object
        properties:
          id: {type: string}
          alert_name: {type: string}
          namespace: {type: string}
          workflow_status: {type: string}
          error_message: {type: string}
```

---

## Success Metrics

### Context API (REST) Performance Targets

1. **Latency**: p95 <200ms
2. **Cache Hit Rate**: >80%
3. **Throughput**: >1000 req/s per replica
4. **Availability**: >99.9%

### LLM Tool Integration Success

1. **Tool Call Success Rate**: >95%
2. **LLM Analysis Quality**: >90% (human validation)
3. **Multi-Tool Composition**: 3+ tools per investigation
4. **End-to-End Latency**: <5 seconds (including LLM generation)

---

## Related Documents

- [APPROVED_MICROSERVICES_ARCHITECTURE.md](../../../architecture/APPROVED_MICROSERVICES_ARCHITECTURE.md) - V1 service boundaries
- [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md) - Context API implementation plan
- [SERVICE_DEVELOPMENT_ORDER_STRATEGY.md](../../../planning/SERVICE_DEVELOPMENT_ORDER_STRATEGY.md) - Phase 2 timeline

---

## Decision History

**2025-10-13**: Initial decision captured after architecture clarification

**Key Learnings**:
1. Context API is an LLM **tool endpoint**, not a standalone intelligence service
2. Tool-based LLM architecture requires simple, predictable REST APIs
3. LLM autonomy depends on receiving raw data, not pre-analyzed recommendations
4. Service responsibility separation is critical in microservices architecture

---

**Decision**: ✅ **APPROVED - REST API Architecture**
**Confidence**: 98%
**Status**: Ready for implementation (12-day timeline)

