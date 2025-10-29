# BR-HAPI-046 to BR-HAPI-050: Context API Tool Integration

**Version**: 1.0
**Date**: October 22, 2025
**Related**: DD-CONTEXT-001 (LLM-Driven Context Tool Call Pattern)
**Implementation Plan**: IMPLEMENTATION_PLAN_V3.0.md v3.1

---

## Overview

These 5 business requirements define the Context API tool integration for HolmesGPT API Service. This integration implements DD-CONTEXT-001 (Approach B: LLM-Driven Tool Call Pattern), allowing the LLM to request historical context on-demand rather than forcing context in every investigation.

**Benefits**:
- **Cost Savings**: 36% token cost reduction ($910/year)
- **LLM Autonomy**: LLM decides when context is needed
- **Architectural Alignment**: Native HolmesGPT SDK tool call pattern
- **Flexibility**: LLM can request different context types

**Timeline**: +3 days (Day 1: Plan + RED, Day 2: GREEN + Integration, Day 3: REFACTOR + E2E)

---

## BR-HAPI-046: Define `get_context` Tool

**Category**: Context API Tool Integration
**Priority**: High
**Status**: ⏸️ PENDING (v3.1)

### Requirement

System must define a `get_context` tool that allows the LLM to retrieve historical context for similar incidents on-demand.

### Tool Definition

```python
{
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
```

### Acceptance Criteria

- [ ] Tool definition is valid JSON schema
- [ ] Tool is registered with HolmesGPT SDK
- [ ] Tool description emphasizes when context is valuable
- [ ] Parameters have correct types and defaults
- [ ] Required parameters are marked correctly

### Unit Test Coverage (3 tests)

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool.py::test_tool_definition_valid`
  - Verify tool definition is valid JSON schema
  - Verify all required fields are present

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool.py::test_parameter_validation`
  - Verify alert_fingerprint is required
  - Verify similarity_threshold has default 0.70
  - Verify context_types is optional array

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool.py::test_default_similarity_threshold`
  - Verify default similarity_threshold is applied when not provided

### Implementation

**File**: `holmesgpt-api/src/tools/context_tool.py`

```python
# Tool definition
CONTEXT_TOOL_DEFINITION = {
    "name": "get_context",
    "description": "Retrieve historical context for similar incidents...",
    "parameters": {...}
}

def register_context_tool(sdk_client):
    """Register context tool with HolmesGPT SDK"""
    sdk_client.register_tool(CONTEXT_TOOL_DEFINITION, handle_context_tool_call)
```

---

## BR-HAPI-047: Implement Context API Client

**Category**: Context API Tool Integration
**Priority**: High
**Status**: ⏸️ PENDING (v3.1)

### Requirement

System must implement a robust HTTP client for Context API with retry logic, circuit breaker, and caching.

### Client Requirements

- HTTP client for Context API REST endpoint (`/api/v1/context/enrich`)
- Retry logic with exponential backoff (max 3 retries)
- Circuit breaker (opens after 50% failure rate in 5-minute window)
- Caching of context results within investigation session (1h TTL)
- Timeout: 2s per request (Context API p95 latency is <500ms)

### Acceptance Criteria

- [ ] Client successfully calls Context API endpoint
- [ ] Retry logic implements exponential backoff
- [ ] Circuit breaker opens after 50% failure rate
- [ ] Cache hit returns cached result without API call
- [ ] Cache miss fetches from API and caches result
- [ ] Timeout is enforced (2s)

### Unit Test Coverage (5 tests)

- ✅ `holmesgpt-api/tests/unit/clients/test_context_api_client.py::test_successful_request`
  - Verify successful Context API call returns expected data

- ✅ `holmesgpt-api/tests/unit/clients/test_context_api_client.py::test_retry_on_timeout`
  - Verify retry logic with exponential backoff on timeout
  - Verify max 3 retries

- ✅ `holmesgpt-api/tests/unit/clients/test_context_api_client.py::test_circuit_breaker_opens`
  - Verify circuit breaker opens after 50% failure rate
  - Verify circuit breaker prevents requests when open

- ✅ `holmesgpt-api/tests/unit/clients/test_context_api_client.py::test_cache_hit`
  - Verify cache hit returns cached result
  - Verify no API call is made on cache hit

- ✅ `holmesgpt-api/tests/unit/clients/test_context_api_client.py::test_cache_miss`
  - Verify cache miss fetches from API
  - Verify result is cached with 1h TTL

### Integration Test Coverage (2 tests)

- ✅ `holmesgpt-api/tests/integration/test_context_api_integration.py::test_real_context_api_call`
  - Verify real Context API call with deployed service
  - Verify response format matches DD-HOLMESGPT-009

- ✅ `holmesgpt-api/tests/integration/test_context_api_integration.py::test_context_api_unavailable`
  - Verify graceful handling when Context API is unavailable
  - Verify circuit breaker opens after failures

### Implementation

**File**: `holmesgpt-api/src/clients/context_api_client.py`

```python
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
            failure_rate = len(self.failures) / (len(self.failures) + 1)
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

## BR-HAPI-048: Tool Call Handler

**Category**: Context API Tool Integration
**Priority**: High
**Status**: ⏸️ PENDING (v3.1)

### Requirement

System must implement a tool call handler that parses LLM tool call requests, invokes Context API, and formats responses for LLM consumption.

### Handler Requirements

- Parse LLM tool call requests (JSON format)
- Validate tool parameters (alert_fingerprint required)
- Invoke Context API client with parameters
- Format context response for LLM consumption (ultra-compact JSON per DD-HOLMESGPT-009)
- Handle tool call failures gracefully (degraded mode)
- Rate limiting: Max 10 tool calls per investigation

### Acceptance Criteria

- [ ] Handler parses LLM tool call requests correctly
- [ ] Handler validates required parameters
- [ ] Handler invokes Context API client
- [ ] Handler formats response per DD-HOLMESGPT-009
- [ ] Handler handles failures gracefully (returns error, allows LLM to continue)
- [ ] Handler enforces rate limiting (max 10 calls per investigation)

### Unit Test Coverage (5 tests)

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py::test_parse_tool_call`
  - Verify handler parses LLM tool call request
  - Verify parameters are extracted correctly

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py::test_validate_parameters`
  - Verify alert_fingerprint is required
  - Verify error is raised if missing

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py::test_format_response`
  - Verify response is formatted per DD-HOLMESGPT-009 (ultra-compact JSON)
  - Verify response includes context data

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py::test_handle_failure_gracefully`
  - Verify handler returns error on Context API failure
  - Verify error allows LLM to continue (degraded mode)

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py::test_rate_limiting`
  - Verify handler enforces max 10 tool calls per investigation
  - Verify error is returned after limit exceeded

### Integration Test Coverage (2 tests)

- ✅ `holmesgpt-api/tests/integration/test_context_tool_handler.py::test_end_to_end_tool_call`
  - Verify complete tool call flow (parse → invoke → format)
  - Verify response is correctly formatted

- ✅ `holmesgpt-api/tests/integration/test_context_tool_handler.py::test_tool_call_with_real_context_api`
  - Verify tool call with real Context API service
  - Verify LLM receives formatted context

### Implementation

**File**: `holmesgpt-api/src/tools/context_tool.py` (handler methods)

```python
from prometheus_client import Counter, Histogram, Gauge

# Metrics
context_tool_calls = Counter('holmesgpt_context_tool_call_total', 'Total context tool calls', ['status'])
context_tool_latency = Histogram('holmesgpt_context_tool_call_duration_seconds', 'Context tool call latency')
context_tool_call_rate = Gauge('holmesgpt_context_tool_call_rate', 'Context tool call rate')

class ContextTool:
    def __init__(self, context_api_url: str, cache_ttl: int = 3600):
        self.context_api_url = context_api_url
        self.cache_ttl = cache_ttl
        self.client = ContextAPIClient(context_api_url)
        self.call_count = {}  # Track calls per investigation

    def handle_tool_call(self, investigation_id: str, parameters: Dict[str, Any]) -> Dict[str, Any]:
        """Handle LLM tool call request"""
        try:
            # Rate limiting
            if self.call_count.get(investigation_id, 0) >= 10:
                raise ValueError("Max 10 context tool calls per investigation exceeded")

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

                # Track call count
                self.call_count[investigation_id] = self.call_count.get(investigation_id, 0) + 1

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

## BR-HAPI-049: Tool Call Observability

**Category**: Context API Tool Integration
**Priority**: Medium
**Status**: ⏸️ PENDING (v3.1)

### Requirement

System must expose comprehensive observability for Context API tool calls including metrics, logging, and tracing.

### Observability Requirements

**Metrics**:
- `holmesgpt_context_tool_call_rate` (gauge) - % of investigations using context tool
- `holmesgpt_context_tool_call_duration_seconds` (histogram) - Tool call latency
- `holmesgpt_context_tool_call_errors_total` (counter) - Tool call failures
- `holmesgpt_context_tool_call_cache_hit_rate` (gauge) - Cache effectiveness

**Logging**: Structured JSON logging for tool call requests, responses, and failures

**Tracing**: OpenTelemetry spans for tool calls (if tracing enabled)

### Acceptance Criteria

- [ ] Metrics are exposed on `/metrics` endpoint
- [ ] Metrics have correct labels and types
- [ ] Logging is structured JSON format
- [ ] Tracing spans are created for tool calls (if enabled)
- [ ] Metrics cardinality is reasonable (<1000 unique labels)

### Unit Test Coverage (3 tests)

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_metrics.py::test_metrics_recording`
  - Verify metrics are recorded correctly
  - Verify counters increment, histograms record values

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_metrics.py::test_metrics_labels`
  - Verify metrics have correct labels
  - Verify label values are correct

- ✅ `holmesgpt-api/tests/unit/tools/test_context_tool_metrics.py::test_logging_format`
  - Verify logging is structured JSON
  - Verify log entries include required fields

### Integration Test Coverage (2 tests)

- ✅ `holmesgpt-api/tests/integration/test_context_tool_observability.py::test_metrics_endpoint`
  - Verify metrics are exposed on `/metrics` endpoint
  - Verify Prometheus scrape format

- ✅ `holmesgpt-api/tests/integration/test_context_tool_observability.py::test_metrics_cardinality`
  - Verify metrics cardinality is reasonable
  - Verify no label explosion

### Implementation

**File**: `holmesgpt-api/src/tools/context_tool.py` (metrics methods)

```python
import logging
import json
from prometheus_client import Counter, Histogram, Gauge

# Metrics
context_tool_calls = Counter('holmesgpt_context_tool_call_total', 'Total context tool calls', ['status'])
context_tool_latency = Histogram('holmesgpt_context_tool_call_duration_seconds', 'Context tool call latency')
context_tool_call_rate = Gauge('holmesgpt_context_tool_call_rate', 'Context tool call rate')
context_tool_cache_hit_rate = Gauge('holmesgpt_context_tool_call_cache_hit_rate', 'Cache hit rate')

# Structured logging
logger = logging.getLogger(__name__)

def log_tool_call(investigation_id: str, parameters: Dict[str, Any], result: Dict[str, Any], duration: float):
    """Log tool call in structured JSON format"""
    logger.info(json.dumps({
        "event": "context_tool_call",
        "investigation_id": investigation_id,
        "parameters": parameters,
        "result_size": len(json.dumps(result)),
        "duration_seconds": duration,
        "status": "success" if "error" not in result else "error"
    }))
```

---

## BR-HAPI-050: Tool Call Testing

**Category**: Context API Tool Integration
**Priority**: High
**Status**: ⏸️ PENDING (v3.1)

### Requirement

System must have comprehensive test coverage for Context API tool integration including unit, integration, and E2E tests.

### Test Requirements

- **Unit Tests**: Tool definition, parameter validation, handler logic (15 tests)
- **Integration Tests**: Real Context API tool calls, failure scenarios (10 tests)
- **E2E Tests**: LLM-driven tool call scenarios (3 tests)

### E2E Test Scenarios

1. **Simple Investigation (No Context Needed)**: LLM investigates simple pod restart, does not request context
2. **Complex Investigation (Context Requested)**: LLM investigates cascading failure, requests context via tool call
3. **Context API Failure (Degraded Mode)**: Context API unavailable, LLM continues without context

### Acceptance Criteria

- [ ] All unit tests pass (15 tests)
- [ ] All integration tests pass (10 tests)
- [ ] All E2E tests pass (3 tests)
- [ ] Code coverage >80% for new code
- [ ] No linter errors

### E2E Test Coverage (3 tests)

- ✅ `holmesgpt-api/tests/e2e/test_context_tool_e2e.py::test_simple_investigation_no_context`
  - Verify LLM investigates simple pod restart
  - Verify LLM does not request context (not needed)
  - Verify investigation completes successfully

- ✅ `holmesgpt-api/tests/e2e/test_context_tool_e2e.py::test_complex_investigation_with_context`
  - Verify LLM investigates cascading failure
  - Verify LLM requests context via tool call
  - Verify Context API returns historical data
  - Verify LLM uses context to improve recommendation

- ✅ `holmesgpt-api/tests/e2e/test_context_tool_e2e.py::test_context_api_failure_degraded_mode`
  - Verify LLM investigates complex issue
  - Verify LLM requests context via tool call
  - Verify Context API is unavailable (timeout)
  - Verify LLM continues without context (degraded mode)
  - Verify investigation completes with lower confidence

### Implementation

**Files**:
- `holmesgpt-api/tests/unit/tools/test_context_tool.py` (3 tests)
- `holmesgpt-api/tests/unit/clients/test_context_api_client.py` (5 tests)
- `holmesgpt-api/tests/unit/tools/test_context_tool_handler.py` (5 tests)
- `holmesgpt-api/tests/unit/tools/test_context_tool_metrics.py` (2 tests)
- `holmesgpt-api/tests/integration/test_context_api_integration.py` (2 tests)
- `holmesgpt-api/tests/integration/test_context_tool_handler.py` (2 tests)
- `holmesgpt-api/tests/integration/test_context_tool_observability.py` (2 tests)
- `holmesgpt-api/tests/e2e/test_context_tool_e2e.py` (3 tests)

**Total**: 28 tests (15 unit + 10 integration + 3 E2E)

---

## Test Coverage Summary

| Test Level | Tests | Purpose |
|---|---|---|
| **Unit Tests** | 15 | Tool definition, client, handler, metrics |
| **Integration Tests** | 10 | Real Context API calls, failure scenarios |
| **E2E Tests** | 3 | LLM-driven tool call scenarios |
| **Total** | 28 | Comprehensive coverage |

---

## Implementation Timeline

**Day 1: Plan + RED Phase** (6 hours):
- Update implementation plan with BR-HAPI-046 to BR-HAPI-050
- Write 15 unit tests (must fail initially)

**Day 2: GREEN Phase** (8 hours):
- Implement Context API tool and client (minimal)
- Write 10 integration tests (must pass)

**Day 3: REFACTOR Phase** (8 hours):
- Add retry logic, circuit breaker, caching
- Write 3 E2E tests with real LLM
- Update documentation

**Total**: +3 days

---

## Related Documents

- [DD-CONTEXT-001: Context Enrichment Placement](../../../../architecture/decisions/DD-CONTEXT-001-Context-Enrichment-Placement.md)
- [DD-CONTEXT-001-ACTION_PLAN](../../../../architecture/decisions/DD-CONTEXT-001-ACTION_PLAN.md)
- [DD-CONTEXT-001-QUICK_START](../../../../architecture/decisions/DD-CONTEXT-001-QUICK_START.md)
- [IMPLEMENTATION_PLAN_V3.0.md](IMPLEMENTATION_PLAN_V3.0.md) - v3.1

---

**Document Version**: 1.0
**Last Updated**: October 22, 2025
**Status**: ✅ **READY FOR IMPLEMENTATION**









