# DD-CONTEXT-001 Quick Start Guide - Developer Reference

**Date**: 2025-10-22
**For**: Developers implementing DD-CONTEXT-001
**Timeline**: 5-6 days

---

## 🚀 Quick Start

### What You're Building

**LLM-driven Context Tool Call Pattern**: Allow the LLM in HolmesGPT API to request historical context from Context API on-demand, instead of pre-fetching context for every investigation.

**Why**: Saves 36% token costs ($910/year), leverages LLM intelligence, aligns with HolmesGPT SDK native tool pattern.

---

## 📋 Your Tasks (By Service)

### If You're Working on HolmesGPT API (3 days)

**Day 1: Plan + RED Phase**
1. ✅ Read [DD-CONTEXT-001](DD-CONTEXT-001-Context-Enrichment-Placement.md)
2. ✅ Update implementation plan with BR-HAPI-031 to BR-HAPI-035
3. ✅ Write 15 unit tests (must fail initially)

**Day 2: GREEN Phase**
1. ✅ Implement `holmesgpt-api/src/tools/context_tool.py` (tool definition + handler)
2. ✅ Implement `holmesgpt-api/src/clients/context_api_client.py` (HTTP client)
3. ✅ Write 10 integration tests (must pass)

**Day 3: REFACTOR Phase**
1. ✅ Add retry logic, circuit breaker, caching
2. ✅ Write 3 E2E tests with real LLM
3. ✅ Update documentation

**Key Files**:
- `holmesgpt-api/src/tools/context_tool.py` - Tool definition and handler
- `holmesgpt-api/src/clients/context_api_client.py` - Context API client
- `holmesgpt-api/tests/unit/tools/test_context_tool.py` - Unit tests
- `holmesgpt-api/tests/integration/test_context_api_integration.py` - Integration tests
- `holmesgpt-api/tests/e2e/test_context_tool_e2e.py` - E2E tests

---

### If You're Working on Context API (1 day)

**Day 1: Documentation Only**
1. ✅ Read [DD-CONTEXT-001](DD-CONTEXT-001-Context-Enrichment-Placement.md)
2. ✅ Update `context-api/README.md` with tool call section
3. ✅ Create `context-api/docs/examples/TOOL_CALL_EXAMPLE.md`
4. ✅ Update `context-api/docs/METRICS.md` with tool call metrics

**No Code Changes Required** ✅

**Key Files**:
- `context-api/README.md` - Add tool call integration section
- `context-api/docs/API_REFERENCE.md` - Document tool call pattern
- `context-api/docs/examples/TOOL_CALL_EXAMPLE.md` - Usage examples
- `context-api/docs/METRICS.md` - Tool call metrics

---

### If You're Working on AIAnalysis (Already Complete ✅)

**Status**: ✅ Already updated to v1.1.2

**No Further Action Required**

---

## 🔧 Implementation Checklist

### HolmesGPT API Developer

**Phase 1: Implementation Plan (2 hours)**
- [ ] Read DD-CONTEXT-001 and understand Approach B
- [ ] Create BR-HAPI-031 to BR-HAPI-035 in implementation plan
- [ ] Define test coverage matrix (15 unit, 10 integration, 3 E2E)

**Phase 2: Unit Tests - RED (4 hours)**
- [ ] Create `test_context_tool.py` (tool definition tests)
- [ ] Create `test_context_api_client.py` (client tests)
- [ ] Create `test_context_tool_handler.py` (handler tests)
- [ ] Create `test_context_tool_metrics.py` (metrics tests)
- [ ] Verify all tests fail (RED phase)

**Phase 3: Minimal Implementation - GREEN (6 hours)**
- [ ] Implement tool definition in `context_tool.py`
- [ ] Implement Context API client in `context_api_client.py`
- [ ] Implement tool call handler in `context_tool.py`
- [ ] Implement basic metrics recording
- [ ] Verify all unit tests pass (GREEN phase)

**Phase 4: Integration Tests (2 hours)**
- [ ] Create `test_context_api_integration.py` (real Context API calls)
- [ ] Create `test_context_tool_handler.py` (end-to-end tool call)
- [ ] Create `test_context_tool_observability.py` (metrics validation)
- [ ] Verify all integration tests pass

**Phase 5: Enhanced Implementation - REFACTOR (6 hours)**
- [ ] Add retry logic with exponential backoff
- [ ] Add circuit breaker (opens after 50% failure rate)
- [ ] Add Redis-based caching (1h TTL)
- [ ] Add rate limiting (max 10 tool calls per investigation)
- [ ] Add comprehensive error handling
- [ ] Add structured JSON logging
- [ ] Add OpenTelemetry tracing (if enabled)
- [ ] Verify all tests still pass

**Phase 6: E2E Tests (2 hours)**
- [ ] Create `test_context_tool_e2e.py` (3 scenarios)
- [ ] Test: Simple investigation (no context requested)
- [ ] Test: Complex investigation (context requested)
- [ ] Test: Context API failure (degraded mode)
- [ ] Verify all E2E tests pass with real LLM

**Phase 7: Documentation (1 hour)**
- [ ] Update `holmesgpt-api/README.md` with tool call section
- [ ] Create `holmesgpt-api/docs/TOOLS.md` (tool documentation)
- [ ] Update `holmesgpt-api/docs/METRICS.md` (tool call metrics)

---

### Context API Developer

**Phase 1: Documentation (4 hours)**
- [ ] Read DD-CONTEXT-001 and understand tool call pattern
- [ ] Update `context-api/README.md` with tool call integration section
- [ ] Update `context-api/docs/API_REFERENCE.md` with tool call pattern
- [ ] Update `context-api/docs/INTEGRATION.md` with HolmesGPT example
- [ ] Create `context-api/docs/examples/TOOL_CALL_EXAMPLE.md`
- [ ] Update `context-api/docs/METRICS.md` with tool call metrics

---

## 📊 Code Examples

### HolmesGPT API: Tool Definition

```python
# holmesgpt-api/src/tools/context_tool.py

from typing import Dict, Any, List, Optional
import requests
from prometheus_client import Counter, Histogram, Gauge

# Metrics
context_tool_calls = Counter('holmesgpt_context_tool_call_total', 'Total context tool calls', ['status'])
context_tool_latency = Histogram('holmesgpt_context_tool_call_duration_seconds', 'Context tool call latency')
context_tool_call_rate = Gauge('holmesgpt_context_tool_call_rate', 'Context tool call rate')

# Tool definition
CONTEXT_TOOL_DEFINITION = {
    "name": "get_context",
    "description": "Retrieve historical context for similar incidents. Use when investigation requires understanding of past similar alerts, success rates, or patterns. Recommended for complex cascading failures or recurring issues.",
    "parameters": {
        "type": "object",
        "properties": {
            "alert_fingerprint": {
                "type": "string",
                "description": "Fingerprint of the current alert (required)"
            },
            "similarity_threshold": {
                "type": "number",
                "description": "Minimum similarity score (0.0-1.0), default 0.70",
                "default": 0.70
            },
            "context_types": {
                "type": "array",
                "items": {
                    "enum": ["historical_remediations", "cluster_patterns", "success_rates"]
                },
                "description": "Types of context to retrieve (optional)"
            }
        },
        "required": ["alert_fingerprint"]
    }
}

class ContextTool:
    def __init__(self, context_api_url: str, cache_ttl: int = 3600):
        self.context_api_url = context_api_url
        self.cache_ttl = cache_ttl
        self.client = ContextAPIClient(context_api_url)

    def handle_tool_call(self, parameters: Dict[str, Any]) -> Dict[str, Any]:
        """Handle LLM tool call request"""
        try:
            with context_tool_latency.time():
                # Validate parameters
                alert_fingerprint = parameters.get("alert_fingerprint")
                if not alert_fingerprint:
                    raise ValueError("alert_fingerprint is required")

                # Invoke Context API
                context = self.client.get_context(
                    alert_fingerprint=alert_fingerprint,
                    similarity_threshold=parameters.get("similarity_threshold", 0.70),
                    context_types=parameters.get("context_types")
                )

                # Format response for LLM (ultra-compact JSON)
                response = self._format_response(context)

                context_tool_calls.labels(status='success').inc()
                return response

        except Exception as e:
            context_tool_calls.labels(status='error').inc()
            # Graceful degradation
            return {"error": str(e), "fallback": "continue_without_context"}

    def _format_response(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """Format context for LLM consumption (ultra-compact JSON per DD-HOLMESGPT-009)"""
        return {
            "ctx": {
                "sim": context.get("similar_incidents", []),
                "pat": context.get("patterns", {}),
                "succ": context.get("success_rates", {})
            }
        }
```

---

### HolmesGPT API: Context API Client

```python
# holmesgpt-api/src/clients/context_api_client.py

import requests
from typing import Dict, Any, Optional, List
import time
from functools import wraps
import redis

class CircuitBreaker:
    def __init__(self, failure_threshold: float = 0.5, window_seconds: int = 300):
        self.failure_threshold = failure_threshold
        self.window_seconds = window_seconds
        self.failures = []
        self.is_open = False

    def record_failure(self):
        self.failures.append(time.time())
        self._cleanup_old_failures()
        self._check_threshold()

    def record_success(self):
        self._cleanup_old_failures()

    def _cleanup_old_failures(self):
        cutoff = time.time() - self.window_seconds
        self.failures = [f for f in self.failures if f > cutoff]

    def _check_threshold(self):
        if len(self.failures) > 0:
            failure_rate = len(self.failures) / (len(self.failures) + 1)  # Simplified
            self.is_open = failure_rate > self.failure_threshold

class ContextAPIClient:
    def __init__(self, base_url: str, timeout: int = 2, max_retries: int = 3):
        self.base_url = base_url
        self.timeout = timeout
        self.max_retries = max_retries
        self.circuit_breaker = CircuitBreaker()
        self.cache = redis.Redis(host='localhost', port=6379, db=0, decode_responses=True)

    def get_context(
        self,
        alert_fingerprint: str,
        similarity_threshold: float = 0.70,
        context_types: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        """Get context from Context API with retry logic and circuit breaker"""

        # Check circuit breaker
        if self.circuit_breaker.is_open:
            raise Exception("Circuit breaker is open")

        # Check cache
        cache_key = f"context:{alert_fingerprint}:{similarity_threshold}"
        cached = self.cache.get(cache_key)
        if cached:
            return eval(cached)  # In production, use json.loads

        # Make request with retry
        for attempt in range(self.max_retries):
            try:
                response = requests.post(
                    f"{self.base_url}/api/v1/context/enrich",
                    json={
                        "alert_fingerprint": alert_fingerprint,
                        "similarity_threshold": similarity_threshold,
                        "context_types": context_types
                    },
                    timeout=self.timeout
                )
                response.raise_for_status()

                result = response.json()

                # Cache result
                self.cache.setex(cache_key, 3600, str(result))  # 1h TTL

                self.circuit_breaker.record_success()
                return result

            except requests.exceptions.Timeout:
                if attempt < self.max_retries - 1:
                    time.sleep(2 ** attempt)  # Exponential backoff
                    continue
                self.circuit_breaker.record_failure()
                raise

            except requests.exceptions.RequestException as e:
                self.circuit_breaker.record_failure()
                raise
```

---

### Context API: Documentation Example

```markdown
## Tool Call Integration

The Context API supports LLM-driven tool call patterns. HolmesGPT API can invoke Context API as a tool, allowing the LLM to request historical context on-demand.

### Tool Call Endpoint

**Endpoint**: `POST /api/v1/context/enrich`

**Request**:
```json
{
  "alert_fingerprint": "sha256:abc123...",
  "similarity_threshold": 0.70,
  "context_types": ["historical_remediations", "cluster_patterns"]
}
```

**Response** (Ultra-Compact JSON per DD-HOLMESGPT-009):
```json
{
  "ctx": {
    "sim": [
      {"fp": "sha256:def456...", "sim": 0.85, "res": "success", "act": ["restart_pod"]},
      {"fp": "sha256:ghi789...", "sim": 0.78, "res": "success", "act": ["scale_deployment"]}
    ],
    "pat": {
      "freq": 12,
      "succ_rate": 0.83,
      "avg_dur": 45
    }
  }
}
```

### Performance

- **Latency**: <500ms p95
- **Timeout**: 2s (HolmesGPT API timeout)
- **Caching**: HolmesGPT API caches results (1h TTL)
```

---

## ⚠️ Common Pitfalls

### HolmesGPT API

**Pitfall 1**: Forgetting to register tool with HolmesGPT SDK
- **Solution**: Ensure tool is registered in SDK initialization

**Pitfall 2**: Not handling Context API failures gracefully
- **Solution**: Implement try-catch with fallback to "continue without context"

**Pitfall 3**: Not implementing caching
- **Solution**: Use Redis with 1h TTL to reduce Context API load

**Pitfall 4**: Not implementing circuit breaker
- **Solution**: Circuit breaker opens after 50% failure rate in 5-minute window

---

### Context API

**Pitfall 1**: Documentation doesn't match actual API
- **Solution**: Test examples against real Context API before documenting

**Pitfall 2**: Missing tool call metrics in documentation
- **Solution**: Document all metrics exposed by Context API

---

## 📊 Testing Strategy

### Unit Tests (15 tests)

**Tool Definition** (3 tests):
- Tool definition is valid JSON
- Parameters have correct types
- Required parameters are marked

**Context API Client** (5 tests):
- Successful request returns context
- Retry on timeout (exponential backoff)
- Circuit breaker opens after failures
- Cache hit returns cached result
- Cache miss fetches from API

**Tool Call Handler** (5 tests):
- Parse tool call request
- Validate parameters (alert_fingerprint required)
- Format response for LLM (ultra-compact JSON)
- Handle failure gracefully (degraded mode)
- Rate limiting (max 10 calls per investigation)

**Metrics** (2 tests):
- Metrics are recorded correctly
- Metrics have correct labels

---

### Integration Tests (10 tests)

**Real Context API Calls** (5 tests):
- Successful context retrieval
- Context API unavailable (timeout)
- Context API returns error (500)
- Context API returns empty result
- Context API returns stale data

**End-to-End Tool Call** (3 tests):
- LLM requests context via tool call
- Tool call handler invokes Context API
- Response formatted correctly for LLM

**Observability** (2 tests):
- Metrics endpoint exposes tool call metrics
- Metrics cardinality is reasonable (<1000 unique labels)

---

### E2E Tests (3 tests)

**Scenario 1: Simple Investigation (No Context)**
- LLM investigates simple pod restart
- LLM does not request context (not needed)
- Investigation completes successfully

**Scenario 2: Complex Investigation (Context Requested)**
- LLM investigates cascading failure
- LLM requests context via tool call
- Context API returns historical data
- LLM uses context to improve recommendation

**Scenario 3: Context API Failure (Degraded Mode)**
- LLM investigates complex issue
- LLM requests context via tool call
- Context API is unavailable (timeout)
- LLM continues without context (degraded mode)
- Investigation completes with lower confidence

---

## 📋 Acceptance Criteria

### HolmesGPT API

- [ ] All 15 unit tests pass
- [ ] All 10 integration tests pass
- [ ] All 3 E2E tests pass
- [ ] Code coverage >80%
- [ ] No linter errors
- [ ] Tool call latency <500ms p95
- [ ] Cache hit rate >60% after 1 hour
- [ ] Circuit breaker opens after 50% failure rate
- [ ] Documentation complete (README, TOOLS.md, METRICS.md)

---

### Context API

- [ ] README includes tool call integration section
- [ ] API_REFERENCE.md documents tool call pattern
- [ ] INTEGRATION.md includes HolmesGPT API example
- [ ] TOOL_CALL_EXAMPLE.md provides complete example
- [ ] METRICS.md documents tool call metrics

---

## 🎯 Success Metrics

**Performance**:
- Context tool call latency p95: <500ms ✅
- Context tool call success rate: >95% ✅
- Investigation latency p95: <5s ✅

**Cost**:
- Token cost reduction: >30% ✅
- Context tool call rate: 50-70% ✅

**Quality**:
- Investigation confidence with context: >85% ✅
- Investigation confidence without context: >75% ✅

---

## 📚 Reference Documents

**Must Read**:
1. [DD-CONTEXT-001: Context Enrichment Placement](DD-CONTEXT-001-Context-Enrichment-Placement.md) - Architectural decision
2. [DD-CONTEXT-002: BR-AI-002 Ownership](DD-CONTEXT-002-BR-AI-002-Ownership.md) - BR ownership decision
3. [DD-CONTEXT-001-ACTION_PLAN](DD-CONTEXT-001-ACTION_PLAN.md) - Detailed action plan

**Optional**:
- [DD-HOLMESGPT-009: Ultra-Compact JSON Format](DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md) - Context response format
- [AIAnalysis Implementation Plan v1.1.2](../services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md) - Monitoring requirements

---

## 🚀 Getting Started

**Step 1**: Read DD-CONTEXT-001 (15 minutes)
**Step 2**: Choose your service (HolmesGPT API or Context API)
**Step 3**: Follow the checklist for your service
**Step 4**: Submit PR when all acceptance criteria met

**Questions?** Contact project lead or refer to [DD-CONTEXT-001-ACTION_PLAN](DD-CONTEXT-001-ACTION_PLAN.md)

---

**Document Version**: 1.0
**Last Updated**: 2025-10-22
**Status**: 🚀 **READY FOR USE**










