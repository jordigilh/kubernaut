# HAPI Code Coverage Analysis - Business Outcome Focus

**Date**: December 24, 2025
**Team**: HAPI Service
**Status**: ✅ ANALYSIS COMPLETE
**Priority**: P1 - Testing Strategy

---

## 🎯 **Executive Summary**

Comprehensive code coverage analysis of HAPI service additions on top of HolmesGPT base. Total coverage: **53%** (6056 statements, 2571 covered). Analysis focuses on **business outcome validation** rather than implementation testing.

**Key Findings**:
- ✅ **High Coverage Areas**: Core models (98-100%), sanitization (80%), validation (92%)
- ⚠️  **Medium Coverage Areas**: Workflow catalog (74%), audit (71%), prompt builders (72-81%)
- ❌ **Low Coverage Areas**: LLM integration (12-31%), middleware auth (60%), API endpoints

---

## 📊 **Coverage by Business Capability**

### **1. Workflow Selection & Search (BR-WORKFLOW-*)**

**Business Outcome**: AI-driven workflow recommendation based on Kubernetes incidents

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `workflow_catalog.py` | 74% (233/43 miss) | Core search logic | **Gap**: Error recovery paths, edge cases |
| `workflow_response_validator.py` | 92% (131/8 miss) | Response validation | **Gap**: Validation failure scenarios |
| `mock_responses.py` | 91% (100/6 miss) | Test fixtures | ✅ Well covered |

**Business Outcomes Tested**:
- ✅ Semantic search with exact match
- ✅ Hybrid scoring with label boost
- ✅ Empty results handling
- ✅ Filter validation
- ✅ Top-k limiting
- ✅ DetectedLabels auto-append to filters

**Missing Business Outcomes**:
- ❌ **BR-WORKFLOW-001**: Workflow selection when multiple matches have equal confidence
- ❌ **BR-WORKFLOW-002**: Timeout handling for Data Storage Service (>5s)
- ❌ **BR-WORKFLOW-003**: Fallback behavior when embedding service unavailable
- ❌ **BR-WORKFLOW-004**: Workflow ranking validation with conflicting labels
- ❌ **BR-WORKFLOW-005**: Search behavior with partial label matches

**Recommended Test Scenarios**:
```python
# Business Outcome: System behavior under timeout
def test_workflow_search_timeout_fallback():
    """BR-WORKFLOW-002: System should return cached results or fail gracefully on timeout"""
    # Mock Data Storage timeout (>5s)
    # Verify: Returns cached results OR returns clear error message
    # Verify: Does NOT hang indefinitely

# Business Outcome: Ranking validation with conflicting signals
def test_workflow_ranking_with_conflicting_labels():
    """BR-WORKFLOW-004: System prioritizes severity over environment"""
    # Setup: 2 workflows, one matches environment but low severity, one mismatches env but critical
    # Verify: Critical workflow ranked higher (business logic: safety > convenience)
```

---

### **2. LLM Integration & Prompt Engineering (BR-AI-*, BR-HAPI-2*)**

**Business Outcome**: Accurate incident analysis and remediation recommendations

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `incident/llm_integration.py` | 12% (172/143 miss) | Incident analysis | **Critical Gap**: Almost no business validation |
| `recovery/llm_integration.py` | 31% (144/92 miss) | Recovery actions | **Critical Gap**: Low business outcome coverage |
| `incident/prompt_builder.py` | 72% (118/25 miss) | Prompt construction | **Gap**: Edge cases, context truncation |
| `recovery/prompt_builder.py` | 81% (155/20 miss) | Recovery prompts | **Gap**: Error scenarios |
| `incident/result_parser.py` | 65% (177/57 miss) | Analysis parsing | **Gap**: Malformed response handling |
| `recovery/result_parser.py` | 51% (94/47 miss) | Recovery parsing | **Critical Gap**: Half of parsing logic untested |

**Business Outcomes Tested**:
- ✅ Prompt template rendering with incident context
- ✅ Basic LLM response parsing
- ✅ DetectedLabels integration in prompts

**Missing Business Outcomes**:
- ❌ **BR-AI-001**: LLM returns no analysis → System behavior
- ❌ **BR-AI-002**: LLM returns malformed JSON → Error recovery
- ❌ **BR-AI-003**: LLM suggests dangerous action (e.g., delete production data) → Safety validation
- ❌ **BR-AI-004**: Context exceeds token limit → Truncation strategy
- ❌ **BR-AI-005**: Multiple LLM failures in succession → Circuit breaker behavior
- ❌ **BR-HAPI-211**: PII in incident data → Sanitization before LLM
- ❌ **BR-HAPI-212**: Secrets in logs → Redaction verification

**Recommended Test Scenarios**:
```python
# Business Outcome: Safety validation for dangerous LLM suggestions
def test_llm_dangerous_action_rejected():
    """BR-AI-003: System rejects LLM suggestions that could cause data loss"""
    # Mock LLM response: "kubectl delete namespace production"
    # Verify: Action is flagged as dangerous
    # Verify: Requires explicit human approval
    # Verify: Logged for audit

# Business Outcome: Graceful degradation on LLM failure
def test_llm_repeated_failures_circuit_breaker():
    """BR-AI-005: System opens circuit breaker after 3 consecutive LLM failures"""
    # Simulate 3 LLM timeouts
    # Verify: 4th request immediately fails with circuit open error
    # Verify: User gets actionable error message (not hanging)
    # Verify: Circuit auto-closes after 60 seconds

# Business Outcome: Context truncation preserves critical information
def test_context_truncation_preserves_signal_type():
    """BR-AI-004: When context exceeds 100K tokens, signal type is preserved"""
    # Setup: Incident with 150K tokens of log data
    # Verify: Signal type (OOMKilled) still in truncated context
    # Verify: Most recent logs are included (last 50K tokens)
    # Verify: Truncation is logged for debugging
```

---

### **3. Audit Trail & Observability (BR-AUDIT-*, ADR-032, ADR-034)**

**Business Outcome**: Complete audit trail for compliance and debugging

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `audit/events.py` | 100% (33/0 miss) | Event creation | ✅ Fully covered |
| `audit/buffered_store.py` | 71% (129/33 miss) | Async buffering | **Gap**: Buffer overflow, flush failures |
| `audit/factory.py` | 45% (18/9 miss) | Event factories | **Gap**: Factory error handling |

**Business Outcomes Tested**:
- ✅ LLM request events are created with correct ADR-034 schema
- ✅ LLM response events capture analysis preview
- ✅ Tool call events log workflow search
- ✅ Validation attempt events track retry attempts

**Missing Business Outcomes**:
- ❌ **BR-AUDIT-001**: Audit buffer full → Backpressure behavior
- ❌ **BR-AUDIT-002**: Data Storage unavailable → Dead letter queue
- ❌ **BR-AUDIT-003**: Audit write fails → Retry logic
- ❌ **BR-AUDIT-004**: Compliance requirement: 100% audit coverage → Verification test
- ❌ **ADR-032**: Service fails if audit unavailable → Startup validation

**Recommended Test Scenarios**:
```python
# Business Outcome: System handles audit buffer overflow
def test_audit_buffer_full_backpressure():
    """BR-AUDIT-001: When audit buffer is full, system applies backpressure"""
    # Fill audit buffer to capacity (10000 events)
    # Attempt to log 10001st event
    # Verify: Event is queued OR oldest event is flushed first
    # Verify: No events are dropped silently
    # Verify: Warning logged for ops team

# Business Outcome: Service fails fast if audit is unavailable
def test_service_fails_if_audit_unavailable_at_startup():
    """ADR-032: HAPI is P0 service, MUST have audit. Service fails if DS unavailable"""
    # Start HAPI with Data Storage offline
    # Verify: Service exits with error code 1
    # Verify: Error message explains audit is required
    # Verify: Kubernetes restart policy will retry
```

---

### **4. Input Sanitization & Security (BR-HAPI-211, BR-HAPI-212)**

**Business Outcome**: No sensitive data reaches external LLMs

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `sanitization/llm_sanitizer.py` | 80% (96/17 miss) | PII/secret redaction | **Gap**: Edge cases, new secret patterns |

**Business Outcomes Tested**:
- ✅ Bearer tokens are redacted
- ✅ JWT tokens are redacted
- ✅ GitHub PAT tokens are redacted
- ✅ GitHub OAuth tokens are redacted
- ✅ API keys are redacted
- ✅ Email addresses are redacted
- ✅ IP addresses are redacted

**Missing Business Outcomes**:
- ❌ **BR-HAPI-211**: New secret pattern (e.g., AWS keys) → Auto-detection
- ❌ **BR-HAPI-212**: PII in nested JSON → Recursive sanitization
- ❌ **BR-HAPI-213**: Sanitization performance with large payloads (>1MB)
- ❌ **BR-HAPI-214**: Sanitization doesn't break JSON structure

**Recommended Test Scenarios**:
```python
# Business Outcome: Sanitization handles large payloads efficiently
def test_sanitization_performance_1mb_payload():
    """BR-HAPI-213: Sanitization of 1MB payload completes in <100ms"""
    # Create 1MB incident payload with 100 secrets scattered
    # Measure sanitization time
    # Verify: Completes in <100ms (p99)
    # Verify: All 100 secrets redacted

# Business Outcome: Sanitization preserves JSON structure
def test_sanitization_preserves_json_structure():
    """BR-HAPI-214: Sanitized output is still valid JSON"""
    # Input: Valid JSON with secrets
    # Sanitize
    # Verify: Output is valid JSON (json.loads succeeds)
    # Verify: Structure preserved (keys still present)
```

---

### **5. RFC 7807 Error Responses (BR-HAPI-200, DD-004)**

**Business Outcome**: Consistent, actionable error responses for clients

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `errors.py` | 97% (102/2 miss) | Error models | ✅ Excellent coverage |
| `middleware/rfc7807.py` | 63% (37/11 miss) | Error middleware | **Gap**: Error transformation edge cases |

**Business Outcomes Tested**:
- ✅ ValidationError returns RFC 7807 response
- ✅ Error includes `type` URI (https://kubernaut.ai/problems/*)
- ✅ Error includes `title`, `status`, `detail` fields

**Missing Business Outcomes**:
- ❌ **BR-HAPI-201**: Client can programmatically handle errors (type URI is actionable)
- ❌ **BR-HAPI-202**: Unhandled exceptions return RFC 7807 (not stack traces)
- ❌ **BR-HAPI-203**: Error responses include correlation ID for support

**Recommended Test Scenarios**:
```python
# Business Outcome: Unhandled exceptions don't leak stack traces
def test_unhandled_exception_returns_rfc7807():
    """BR-HAPI-202: Even unhandled exceptions return RFC 7807, not stack traces"""
    # Trigger unexpected exception (ZeroDivisionError)
    # Verify: Response is RFC 7807 format
    # Verify: No stack trace in response body
    # Verify: Stack trace logged server-side only
```

---

### **6. Health & Monitoring (BR-HAPI-100)**

**Business Outcome**: Operations team can monitor service health

| Component | Coverage | Statements | Business Gap |
|-----------|----------|------------|--------------|
| `extensions/health.py` | 78% (50/11 miss) | Health endpoint | **Gap**: Dependency health checks |
| `middleware/metrics.py` | 72% (103/25 miss) | Prometheus metrics | **Gap**: Metric edge cases |

**Business Outcomes Tested**:
- ✅ `/health` endpoint returns 200 when healthy

**Missing Business Outcomes**:
- ❌ **BR-HAPI-101**: `/health` returns 503 when Data Storage unavailable
- ❌ **BR-HAPI-102**: `/health` includes dependency status (Data Storage, LLM)
- ❌ **BR-HAPI-103**: Prometheus metrics track LLM latency percentiles
- ❌ **BR-HAPI-104**: Metrics track workflow search success rate

**Recommended Test Scenarios**:
```python
# Business Outcome: Health endpoint reflects dependency health
def test_health_unhealthy_when_data_storage_down():
    """BR-HAPI-101: /health returns 503 when Data Storage is unreachable"""
    # Stop Data Storage service
    # Call /health endpoint
    # Verify: Returns 503
    # Verify: Response body explains Data Storage unavailable
    # Verify: Kubernetes readiness probe fails (pod removed from service)
```

---

## 📈 **Coverage Summary by Package**

### **High Priority - Business Logic (Need >70% Coverage)**

| Package | Coverage | Priority | Status |
|---------|----------|----------|--------|
| `src/models/` | 98-100% | ✅ EXCELLENT | Well covered |
| `src/validation/` | 92% | ✅ EXCELLENT | Minor gaps |
| `src/toolsets/` | 74% | ✅ GOOD | Edge cases needed |
| `src/sanitization/` | 80% | ✅ GOOD | Edge cases needed |
| `src/audit/events.py` | 100% | ✅ EXCELLENT | Perfect |

### **Medium Priority - Integration (Need >60% Coverage)**

| Package | Coverage | Priority | Status |
|---------|----------|----------|--------|
| `src/middleware/metrics.py` | 72% | ✅ GOOD | Acceptable |
| `src/audit/buffered_store.py` | 71% | ✅ GOOD | Buffer edge cases needed |
| `src/extensions/postexec.py` | 75% | ✅ GOOD | Acceptable |
| `src/middleware/auth.py` | 60% | ⚠️  NEEDS WORK | Auth failure scenarios |
| `src/middleware/rfc7807.py` | 63% | ⚠️  NEEDS WORK | Error edge cases |

### **Low Priority - External Integrations (Integration tests cover these)**

| Package | Coverage | Priority | Status |
|---------|----------|----------|--------|
| `src/extensions/incident/llm_integration.py` | 12% | ❌ CRITICAL | Needs integration tests |
| `src/extensions/recovery/llm_integration.py` | 31% | ❌ CRITICAL | Needs integration tests |
| `src/clients/datastorage/` | 28-68% | ⚠️  LOW | OpenAPI generated code |

**Note**: LLM integration modules have low unit test coverage because they require real LLM calls. These are covered by integration and E2E tests.

---

## 🎯 **Priority Test Scenarios - Business Outcome Focus**

### **P0 - Safety & Security (Must Have)**

1. **Dangerous LLM Action Rejection** (BR-AI-003)
   - **Business Risk**: LLM suggests `kubectl delete namespace production`
   - **Test**: Verify action is flagged, requires human approval, audited
   - **Coverage Gap**: 0% → Target: 100%

2. **Secret Leakage Prevention** (BR-HAPI-211)
   - **Business Risk**: API keys sent to external LLM
   - **Test**: Verify all secret patterns are redacted before LLM
   - **Coverage Gap**: 80% → Target: 95%

3. **Audit Completeness** (ADR-032, BR-AUDIT-004)
   - **Business Risk**: Compliance violation if audit gaps exist
   - **Test**: Verify 100% of LLM calls are audited
   - **Coverage Gap**: 71% → Target: 100%

### **P1 - Reliability (Should Have)**

4. **LLM Timeout Handling** (BR-AI-005)
   - **Business Risk**: System hangs when LLM is slow (>30s)
   - **Test**: Verify circuit breaker after 3 failures
   - **Coverage Gap**: 12% → Target: 80%

5. **Data Storage Unavailable** (BR-WORKFLOW-002)
   - **Business Risk**: Workflow search fails, no fallback
   - **Test**: Verify graceful degradation or cached results
   - **Coverage Gap**: 74% → Target: 90%

6. **Malformed LLM Response** (BR-AI-002)
   - **Business Risk**: Invalid JSON breaks the system
   - **Test**: Verify error recovery and retry logic
   - **Coverage Gap**: 65% → Target: 90%

### **P2 - Observability (Nice to Have)**

7. **Health Check Dependency Status** (BR-HAPI-102)
   - **Business Risk**: Ops team can't diagnose issues
   - **Test**: Verify `/health` includes Data Storage status
   - **Coverage Gap**: 78% → Target: 90%

8. **Prometheus Metrics Completeness** (BR-HAPI-103-104)
   - **Business Risk**: Can't measure LLM latency or success rate
   - **Test**: Verify metrics track key business outcomes
   - **Coverage Gap**: 72% → Target: 85%

---

## 📊 **Current vs. Target Coverage**

| Area | Current | Target | Priority |
|------|---------|--------|----------|
| **Overall** | 53% | 70% | Medium |
| **Business Logic** | 74-100% | 80%+ | High |
| **LLM Integration** | 12-31% | 60%+ | **Critical** |
| **Audit** | 71-100% | 90%+ | High |
| **Sanitization** | 80% | 95%+ | High |
| **Error Handling** | 63-97% | 80%+ | Medium |

---

## 🛠️ **Recommended Actions**

### **Immediate (This Sprint)**

1. **Add P0 Safety Tests**
   - Dangerous action rejection
   - Secret leakage prevention
   - Audit completeness validation

2. **Add LLM Failure Scenarios**
   - Timeout handling
   - Malformed response recovery
   - Circuit breaker behavior

### **Next Sprint**

3. **Add Data Storage Failure Scenarios**
   - Unavailable service
   - Timeout handling
   - Fallback/cache behavior

4. **Add Sanitization Edge Cases**
   - Large payload performance
   - Nested JSON PII
   - JSON structure preservation

### **Ongoing**

5. **Integration Test Coverage**
   - LLM integration modules (currently 12-31%)
   - End-to-end user journeys
   - Cross-service interactions

6. **Business Outcome Validation**
   - Every new feature must have business outcome test
   - Test what the system should do, not how it does it
   - Focus on user-visible behavior and compliance requirements

---

## 📚 **Testing Principles - Business Outcome Focus**

### **✅ DO: Test Business Outcomes**

```python
# GOOD: Tests business outcome (what should happen)
def test_critical_incident_gets_highest_priority_workflow():
    """BR-WORKFLOW-001: Critical incidents must get P0 workflows first"""
    # Business context: Production is down
    # Expected outcome: System recommends fastest remediation
    # Verification: P0 workflow returned, not P1/P2

# GOOD: Tests safety requirement
def test_dangerous_kubectl_command_requires_approval():
    """BR-AI-003: Dangerous actions must not execute automatically"""
    # Business context: LLM suggests deleting production data
    # Expected outcome: System blocks auto-execution
    # Verification: Approval required + audit log created
```

### **❌ DON'T: Test Implementation Details**

```python
# BAD: Tests implementation (how it works)
def test_prompt_builder_uses_jinja2_template():
    """Tests that we use Jinja2 (implementation detail)"""
    # This test will break if we switch to f-strings
    # Business outcome is unchanged

# BAD: Tests internal data structure
def test_workflow_catalog_stores_results_in_dict():
    """Tests internal storage mechanism"""
    # Business doesn't care if it's a dict, list, or database
    # Test the business behavior instead
```

### **Key Questions for Every Test**

1. **Business Value**: "If this fails in production, would a user notice?"
2. **Compliance**: "Does this test verify a regulatory requirement?"
3. **Safety**: "Does this test prevent data loss or security breach?"
4. **Stability**: "Is this test stable and not flaky?"

---

## 🎯 **Success Metrics**

**Target for v1.0 Release**:
- ✅ Overall coverage: 70%+ (currently 53%)
- ✅ Business logic: 80%+ (currently 74-100%, mixed)
- ✅ P0 safety tests: 100% (currently missing)
- ✅ LLM integration: 60%+ (currently 12-31%)
- ✅ All BR-* requirements have test coverage mapping

---

**Document Version**: 1.0
**Last Updated**: December 24, 2025
**Owner**: HAPI Team
**Next Review**: Post-v1.0 Release



