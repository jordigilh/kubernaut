# HolmesGPT API Service - Business Requirements

**Service**: HolmesGPT API Service
**Service Type**: Stateless HTTP API (Python)
**Version**: v3.0 (Minimal Internal Service)
**Last Updated**: November 8, 2025
**Status**: ✅ Production-Ready (V1.0 Complete)

---

## 📋 Overview

The **HolmesGPT API Service** is a minimal internal Python service that wraps the HolmesGPT SDK, providing AI-powered Kubernetes investigation capabilities, recovery strategy analysis, and post-execution effectiveness analysis. It follows the DD-HOLMESGPT-012 minimal internal service architecture, focusing on core business value without enterprise API gateway features.

### Architecture

**Service Type**: Stateless HTTP API (Python Flask)

**Key Characteristics**:
- Internal-only service (network policies handle access control)
- Thin wrapper around HolmesGPT Python SDK
- Multi-provider LLM support (OpenAI, Claude, Ollama)
- Read-only Kubernetes access via RBAC
- ServiceAccount token authentication
- Circuit breaker and retry patterns for LLM calls

**Relationship with Other Services**:
- **AI Analysis Controller**: Consumes HolmesGPT API for AI-powered investigations
- **Context API**: Provides historical intelligence for investigation enrichment (pending integration)
- **Dynamic Toolset Service**: Provides observability tool discovery for investigations
- **External LLM Providers**: OpenAI, Anthropic Claude, Ollama

### Service Responsibilities

1. **AI-Powered Investigation**: Analyze Kubernetes issues using HolmesGPT SDK
2. **Recovery Strategy Analysis**: Generate recovery strategies with confidence scores
3. **Post-Execution Analysis**: Evaluate effectiveness of executed remediation actions
4. **Multi-Provider LLM**: Support multiple LLM providers with fallback
5. **Health & Readiness**: Kubernetes-native health checks
6. **Basic Authentication**: ServiceAccount token validation

---

## 🎯 Business Requirements

### 📊 Summary

**Total Business Requirements**: 55 essential BRs (139 deferred BRs for v2.0)
**Categories**: 9 (Investigation, Recovery, Post-Exec, SDK, Remediation History & Workflow Discovery, Health, Auth, HTTP, Observability)
**Priority Breakdown**:
- P0 (Critical): 49 BRs (core business logic + observability metrics + graceful shutdown + LLM sanitization)
- P1 (High): 6 BRs (RFC 7807 + hot-reload + mock mode + config reload metrics)

**Implementation Status**:
- ✅ Implemented: 52 BRs (100% of V1.0 scope)
- ⏸️ V1.1 Deferred: BR-HAPI-POSTEXEC-* (PostExec endpoint deferred to V1.1; EM Level 1 exists in V1.0 per DD-017 v2.0 but does not call PostExec; Level 2 (V1.1) is the PostExec consumer)
- BR-HAPI-192 (Recovery Context Consumption): ✅ Complete
- BR-HAPI-199 (ConfigMap Hot-Reload): ✅ Complete
- BR-HAPI-200 (Investigation Inconclusive): ✅ Complete
- BR-HAPI-211 (LLM Input Sanitization): ✅ **Complete** (Dec 10, 2025) - 46 unit tests
- BR-HAPI-212 (Mock LLM Mode): ✅ **Complete** (Dec 10, 2025) - 24 unit tests

**Test Coverage** (Updated Dec 10, 2025):
- Unit: 590+ tests (100% passing)
- Integration: 84 tests (100% passing)
- E2E: 53 tests (100% passing, runs against mock LLM)
- Mock Mode: 24 tests (100% passing)
- **Total: 750+ tests**

**V1.0 Endpoint Availability**:
| Endpoint | Status |
|----------|--------|
| `/api/v1/incident/analyze` | ✅ Available |
| `/api/v1/recovery/analyze` | ✅ Available |
| `/api/v1/postexec/analyze` | ⏸️ V1.1 (DD-017); EM Level 2 consumer |

**Deferred BRs**: 139 BRs deferred to v2.0 (advanced security, rate limiting, advanced configuration) - only needed if service becomes externally exposed

---

### Category 1: Investigation Endpoints (BR-HAPI-001 to 015)

#### BR-HAPI-001: AI-Powered Investigation Endpoint

**Description**: The HolmesGPT API Service MUST provide a `/investigate` POST endpoint that accepts Kubernetes alert data and returns AI-powered investigation results including root cause analysis, affected resources, and recommended actions.

**Priority**: P0 (CRITICAL)

**Rationale**: This is the core business capability - AI-powered investigation of Kubernetes issues. Without this, the service has no value.

**Implementation**:
- **Endpoint**: `POST /api/v1/investigate`
- **Input**: Alert data (name, namespace, labels, annotations, description)
- **Output**: Investigation result (root cause, affected resources, recommendations, confidence score)
- **LLM Integration**: Calls HolmesGPT SDK with alert context
- **Timeout**: 60 seconds (configurable)
- **Retry**: 3 attempts with exponential backoff

**Acceptance Criteria**:
- ✅ Accepts alert data in JSON format
- ✅ Returns structured investigation results
- ✅ Includes confidence score (0.0-1.0)
- ✅ Handles LLM timeouts gracefully
- ✅ Logs investigation requests and results

**Test Coverage**:
- Unit: `test_recovery.py`, `test_postexec.py` (investigation logic)
- Integration: `test_sdk_integration.py` (end-to-end investigation)
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-026 (SDK Integration), BR-HAPI-036 (HTTP Server)

---

#### BR-HAPI-002 to BR-HAPI-010: Investigation Endpoint Variations

**Description**: Additional investigation endpoint capabilities including:
- BR-HAPI-002: Investigation with custom context
- BR-HAPI-003: Investigation with historical data
- BR-HAPI-004: Investigation with toolset integration
- BR-HAPI-005: Investigation result caching
- BR-HAPI-006: Investigation request validation
- BR-HAPI-007: Investigation error handling
- BR-HAPI-008: Investigation timeout configuration
- BR-HAPI-009: Investigation retry logic
- BR-HAPI-010: Investigation logging

**Priority**: P0 (CRITICAL)

**Implementation Status**: ✅ Implemented

**Related BRs**: BR-HAPI-001 (Core Investigation)

---

#### BR-HAPI-011: Investigation Metrics (Prometheus Observability)

**Description**: The HolmesGPT API Service MUST expose Prometheus metrics for investigation request observability, enabling SLO monitoring, performance tracking, and operational visibility.

**Priority**: P0 (CRITICAL) - Core observability capability

**Rationale**: 
- SLO monitoring: Track investigation latency and success rates
- Performance tracking: Identify slow investigations and optimize
- Operational visibility: Alert on investigation failures or degraded performance
- Business insights: Understand investigation workload patterns

**Metrics Specification**:

1. **Investigation Request Counter**
   ```
   Metric: holmesgpt_api_investigations_total
   Type: Counter
   Labels: status (success | error | needs_review)
   Description: Total number of investigation requests by outcome
   ```

2. **Investigation Duration Histogram**
   ```
   Metric: holmesgpt_api_investigations_duration_seconds
   Type: Histogram
   Labels: none
   Buckets: (0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0)
   Description: Time spent processing investigation requests (incident + recovery)
   ```

**Service Level Objectives (SLOs)**:
- **P95 Latency**: < 10 seconds (investigation requests)
- **Success Rate**: > 95% (non-needs_review outcomes)
- **Error Rate**: < 5%

**Acceptance Criteria**:
- ✅ Metrics exposed via `/metrics` endpoint
- ✅ Metrics follow DD-005 naming convention
- ✅ Metric name constants defined (DD-005 v3.0 compliance)
- ✅ Integration tests validate metric emission
- ✅ Grafana dashboard queries documented

**Implementation Notes**:
- Metrics incremented in business logic (not middleware) per DD-005 pattern
- Injectable metrics instance for integration test isolation
- Follows Go service pattern (Gateway, AIAnalysis)

**Related Standards**: DD-005 v3.0 (Observability Standards)

---

#### BR-HAPI-012 to 015: Investigation Advanced Features (Deferred)

**Description**: Advanced investigation capabilities:
- BR-HAPI-012: Investigation rate limiting (deferred to v2.0)
- BR-HAPI-013: Investigation authentication (handled by DD-AUTH-014 middleware)
- BR-HAPI-014: Investigation authorization (deferred to v2.0)
- BR-HAPI-015: Investigation audit trail (deferred to v2.0)

**Priority**: P1 (HIGH)

**Implementation Status**: ⏸️ Deferred to v2.0

**Related BRs**: BR-HAPI-001 (Core Investigation)

---

### Category 2: Recovery Analysis (BR-HAPI-RECOVERY-001 to 006)

#### BR-HAPI-RECOVERY-001: Recovery Strategy Generation

**Description**: The HolmesGPT API Service MUST analyze investigation results and generate recovery strategies with confidence scores, risk assessments, and step-by-step execution plans.

**Priority**: P0 (CRITICAL)

**Rationale**: Recovery strategy generation is the primary output of AI investigation - it provides actionable remediation steps for operators.

**Implementation**:
- **Endpoint**: `POST /api/v1/recovery/analyze`
- **Input**: Investigation result, cluster context, historical data
- **Output**: Recovery strategies (list of strategies with confidence, risk, steps)
- **LLM Integration**: Calls HolmesGPT SDK recovery analysis
- **Strategy Ranking**: Sorts strategies by confidence score (highest first)
- **Risk Assessment**: Low/Medium/High risk classification

**Acceptance Criteria**:
- ✅ Generates multiple recovery strategies (1-5)
- ✅ Each strategy includes confidence score (0.0-1.0)
- ✅ Each strategy includes risk assessment (Low/Medium/High)
- ✅ Each strategy includes step-by-step execution plan
- ✅ Strategies sorted by confidence (highest first)

**Test Coverage**:
- Unit: `test_recovery.py:27` (27 test scenarios)
- Integration: `test_sdk_integration.py` (recovery analysis)
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-001 (Investigation), BR-HAPI-POSTEXEC-001 (Post-Execution Analysis)

---

#### BR-HAPI-RECOVERY-002 to 006: Recovery Analysis Variations

**Description**: Additional recovery analysis capabilities:
- BR-HAPI-RECOVERY-002: Recovery strategy validation
- BR-HAPI-RECOVERY-003: Recovery strategy comparison
- BR-HAPI-RECOVERY-004: Recovery strategy rollback planning
- BR-HAPI-RECOVERY-005: Recovery strategy dry-run simulation
- BR-HAPI-RECOVERY-006: Recovery strategy approval workflow (deferred)

**Priority**: P0 (CRITICAL) for BR-HAPI-RECOVERY-002 to 005, P1 (HIGH) for BR-HAPI-RECOVERY-006

**Implementation Status**: ✅ Implemented (BR-HAPI-RECOVERY-002 to 005), ⏸️ Deferred (BR-HAPI-RECOVERY-006)

**Related BRs**: BR-HAPI-RECOVERY-001 (Core Recovery Analysis)

---

### Category 3: Post-Execution Analysis (BR-HAPI-POSTEXEC-001 to 005)

> ⏸️ **V1.1 DEFERRED**: Per [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) v2.0, the PostExec endpoint is deferred to V1.1. EM Level 1 exists in V1.0 but does not call PostExec. EM Level 2 (V1.1) is the PostExec consumer. Implementation exists but endpoint is not exposed.

#### BR-HAPI-POSTEXEC-001: Post-Execution Effectiveness Analysis

**Description**: The HolmesGPT API Service MUST analyze the effectiveness of executed remediation actions by comparing pre-execution and post-execution cluster state, providing success metrics and improvement recommendations.

**Priority**: P0 (CRITICAL)

**Rationale**: Post-execution analysis enables continuous learning - understanding what worked and what didn't improves future remediation strategies.

**Implementation**:
- **Endpoint**: `POST /api/v1/postexec/analyze`
- **Input**: Execution result, pre-execution state, post-execution state
- **Output**: Effectiveness analysis (success rate, improvements, failures, recommendations)
- **LLM Integration**: Calls HolmesGPT SDK post-execution analysis
- **Success Metrics**: Resolution rate, time to resolution, resource impact
- **Learning**: Stores successful patterns for future investigations

**Acceptance Criteria**:
- ✅ Compares pre/post execution states
- ✅ Calculates success rate (0.0-1.0)
- ✅ Identifies improvements and failures
- ✅ Provides recommendations for future actions
- ✅ Logs effectiveness analysis for learning

**Test Coverage**:
- Unit: `test_postexec.py:24` (24 test scenarios)
- Integration: `test_sdk_integration.py` (post-execution analysis)
- E2E: ⏸️ Skipped in V1.0 (DD-017)

**Implementation Status**: ⏸️ **V1.1** - Logic implemented, endpoint not exposed in V1.0. EM Level 1 (V1.0) does not call PostExec; Level 2 (V1.1) is the PostExec consumer (DD-017 v2.0)

**Related BRs**: BR-HAPI-RECOVERY-001 (Recovery Analysis), BR-HAPI-001 (Investigation)

---

#### BR-HAPI-POSTEXEC-002 to 005: Post-Execution Analysis Variations

**Description**: Additional post-execution analysis capabilities:
- BR-HAPI-POSTEXEC-002: Post-execution metrics collection
- BR-HAPI-POSTEXEC-003: Post-execution trend analysis
- BR-HAPI-POSTEXEC-004: Post-execution learning feedback loop
- BR-HAPI-POSTEXEC-005: Post-execution report generation

**Priority**: P0 (CRITICAL)

**Implementation Status**: ⏸️ **V1.1** - Logic implemented, endpoint not exposed in V1.0. EM Level 1 (V1.0) does not call PostExec; Level 2 (V1.1) is the PostExec consumer (DD-017 v2.0)

**Related BRs**: BR-HAPI-POSTEXEC-001 (Core Post-Execution Analysis)

---

### Category 4: SDK Integration (BR-HAPI-026 to 030)

#### BR-HAPI-026: HolmesGPT SDK Integration

**Description**: The HolmesGPT API Service MUST integrate with the HolmesGPT Python SDK, providing a thin wrapper that handles authentication, error handling, and retry logic for SDK calls.

**Priority**: P0 (CRITICAL)

**Rationale**: The service is a wrapper around the HolmesGPT SDK - without SDK integration, the service cannot provide AI-powered investigation capabilities.

**Implementation**:
- **SDK Version**: HolmesGPT Python SDK v1.2+
- **Initialization**: SDK client initialized with LLM provider config
- **Error Handling**: Wraps SDK exceptions with HTTP error responses
- **Retry Logic**: 3 attempts with exponential backoff for transient failures
- **Circuit Breaker**: Opens after 5 consecutive SDK failures
- **Timeout**: 60 seconds per SDK call

**Acceptance Criteria**:
- ✅ SDK client initialized on service startup
- ✅ SDK calls wrapped with error handling
- ✅ Retry logic for transient failures
- ✅ Circuit breaker prevents cascade failures
- ✅ Timeout prevents hanging requests

**Test Coverage**:
- Unit: `test_models.py:23` (SDK data models)
- Integration: `test_sdk_integration.py` (end-to-end SDK calls)
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-001 (Investigation), BR-HAPI-RECOVERY-001 (Recovery Analysis)

---

#### BR-HAPI-027 to 030: SDK Integration Variations

**Description**: Additional SDK integration capabilities:
- BR-HAPI-027: Multi-provider LLM support (OpenAI, Claude, Ollama)
- BR-HAPI-028: SDK configuration management
- BR-HAPI-029: SDK health checks
- BR-HAPI-030: SDK version compatibility validation

**Priority**: P0 (CRITICAL)

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-026 (Core SDK Integration)

---

### Category 5: Remediation History & Workflow Discovery (BR-HAPI-016 to 017)

#### BR-HAPI-016: Remediation History Context Enrichment

**Description**: The HolmesGPT API Service MUST enrich LLM investigation prompts with remediation history context from the DataStorage service. This enables the LLM to avoid repeating ineffective remediations and to detect configuration regressions.

**Priority**: P0 (CRITICAL)

**Rationale**: When a signal fires for a target resource that has already been remediated, the LLM needs visibility into what was previously attempted. Without this context, the LLM may recommend ScaleUp for HighCPULoad without knowing ScaleUp was already tried twice and failed.

**Implementation**:
- **Data Source**: DataStorage `GET /api/v1/remediation-history/context` endpoint (DD-HAPI-016)
- **Integration**: HAPI fetches context before constructing incident/recovery prompts
- **Prompt Section**: Formatted remediation chain with effectiveness scores, hash match, regression detection
- **spec_drift Handling**: INCONCLUSIVE semantics per DD-EM-002 v1.1 (see DD-HAPI-016 spec_drift section)

**Acceptance Criteria**:
- ✅ Fetches remediation history from DS when configured
- ✅ Gracefully degrades when DS unavailable (prompt valid without history section)
- ✅ Prompt includes Tier 1 (24h) and Tier 2 (90d) chains when applicable
- ✅ spec_drift entries rendered as INCONCLUSIVE with appropriate guidance

**Test Coverage**:
- Unit: `test_remediation_history_prompt.py` (28 tests)
- Integration: `test_remediation_history_integration.py` (spec_drift flow, graceful degradation)

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: DD-HAPI-016, DD-EM-002 v1.1, BR-HAPI-017 (workflow discovery)

---

#### BR-HAPI-017: Three-Step Workflow Discovery Protocol

**Description**: The HolmesGPT API Service MUST provide a three-step workflow discovery protocol (list_available_actions → list_workflows → get_workflow) replacing the legacy search_workflow_catalog tool. This enables structured action-type-first discovery aligned with DD-WORKFLOW-016 taxonomy.

**Priority**: P0 (CRITICAL)

**Rationale**: The LLM must first understand what categories of remediation actions are available, then drill into specific workflows within a category. The three-step protocol forces comprehensive review before selection.

**Implementation**:
- **BR-HAPI-017-001**: ListAvailableActionsTool, ListWorkflowsTool, GetWorkflowTool
- **BR-HAPI-017-002**: Prompt template update with three-step instructions
- **BR-HAPI-017-003**: Post-selection validation with context filter security gate
- **BR-HAPI-017-004**: Recovery flow validation loop (self-correction)
- **BR-HAPI-017-005**: remediationId propagation for audit correlation
- **BR-HAPI-017-006**: search_workflow_catalog removal

**Acceptance Criteria**:
- ✅ Three tools registered in incident and recovery flows
- ✅ LLM instructed to review ALL workflows before selecting
- ✅ GetWorkflow returns 404 when context filters mismatch (security gate)
- ✅ Recovery flow has validation loop matching incident flow

**Test Coverage**:
- Unit: `test_workflow_discovery_tools.py`
- Integration: `test_three_step_discovery_integration.py`, `test_workflow_validation_integration.py`, `test_recovery_validation_integration.py`
- E2E: `three_step_discovery_test.go`

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: DD-HAPI-017, DD-WORKFLOW-016, BR-HAPI-016 (remediation history)

---

### Category 6: Health & Status (BR-HAPI-018 to 019)

#### BR-HAPI-018: Kubernetes Health Probes

**Description**: The HolmesGPT API Service MUST provide `/healthz` (liveness) and `/readyz` (readiness) endpoints following Kubernetes health check patterns.

**Priority**: P0 (CRITICAL)

**Rationale**: Kubernetes health probes are mandatory for production services - they enable zero-downtime deployments and automatic recovery from failures.

**Implementation**:
- **Liveness Probe**: `GET /healthz` - Returns 200 if service is alive
- **Readiness Probe**: `GET /readyz` - Returns 200 if service is ready to accept traffic
- **Readiness Checks**:
  - HolmesGPT SDK initialized
  - LLM provider reachable
  - Configuration loaded
- **Startup Time**: Service ready within 10 seconds

**Acceptance Criteria**:
- ✅ `/healthz` returns 200 OK when service is alive
- ✅ `/readyz` returns 200 OK when service is ready
- ✅ `/readyz` returns 503 Service Unavailable when not ready
- ✅ Readiness checks validate SDK and LLM provider
- ✅ Service ready within 10 seconds of startup

**Test Coverage**:
- Unit: `test_health.py:30` (30 test scenarios)
- Integration: Kubernetes liveness/readiness probes
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-026 (SDK Integration), BR-HAPI-036 (HTTP Server)

---

#### BR-HAPI-019: Configuration Endpoint

**Description**: The HolmesGPT API Service MUST provide a `/config` endpoint that returns current service configuration (non-sensitive) for debugging and observability.

**Priority**: P0 (CRITICAL)

**Implementation**:
- **Endpoint**: `GET /config`
- **Output**: Service configuration (LLM provider, SDK version, timeouts, retry config)
- **Security**: Redacts sensitive values (API keys, tokens)

**Acceptance Criteria**:
- ✅ Returns current configuration in JSON format
- ✅ Redacts sensitive values (API keys, tokens)
- ✅ Includes LLM provider, SDK version, timeouts

**Test Coverage**:
- Unit: `test_health.py` (configuration endpoint tests)
- Integration: Configuration validation
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-018 (Health Probes)

---

### Category 7: Basic Authentication (BR-HAPI-066 to 067)

#### BR-HAPI-066: ServiceAccount Token Authentication

**Description**: The HolmesGPT API Service MUST validate Kubernetes ServiceAccount tokens for authentication, following DD-GATEWAY-006 sidecar-based authentication strategy.

**Priority**: P0 (CRITICAL)

**Rationale**: ServiceAccount token authentication is the Kubernetes-native authentication mechanism for internal services.

**Implementation**:
- **Authentication Method**: Kubernetes ServiceAccount tokens (Bearer tokens)
- **Token Validation**: Validates token signature using Kubernetes TokenReview API
- **Authorization**: Delegates to Kubernetes RBAC (not app-level)
- **Sidecar Support**: Compatible with Envoy/Istio sidecar authentication
- **Token Extraction**: Reads `Authorization: Bearer <token>` header

**Acceptance Criteria**:
- ✅ Validates ServiceAccount tokens via TokenReview API
- ✅ Rejects invalid or expired tokens (401 Unauthorized)
- ✅ Delegates authorization to Kubernetes RBAC
- ✅ Compatible with sidecar authentication (Envoy/Istio)
- ✅ Logs authentication failures for security auditing

**Test Coverage**:
- Unit: `test_auth_middleware.py` (authentication middleware tests)
- Integration: ServiceAccount token validation
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-067 (Authorization), BR-HAPI-036 (HTTP Server)

---

#### BR-HAPI-067: Kubernetes RBAC Authorization

**Description**: The HolmesGPT API Service MUST delegate authorization to Kubernetes RBAC, ensuring only authorized ServiceAccounts can access investigation endpoints.

**Priority**: P0 (CRITICAL)

**Implementation**:
- **Authorization Method**: Kubernetes RBAC (Role/RoleBinding)
- **Required Permissions**: `holmesgpt-api:investigate` custom resource access
- **Delegation**: Service does NOT implement app-level RBAC
- **Network Policies**: Restrict access to authorized namespaces

**Acceptance Criteria**:
- ✅ Delegates authorization to Kubernetes RBAC
- ✅ Rejects unauthorized ServiceAccounts (403 Forbidden)
- ✅ Network policies restrict access to authorized namespaces
- ✅ Logs authorization failures for security auditing

**Test Coverage**:
- Unit: `test_auth_middleware.py` (authorization tests)
- Integration: RBAC validation
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-066 (Authentication)

---

### Category 8: HTTP Server (BR-HAPI-036 to 045)

#### BR-HAPI-036: Flask HTTP Server

**Description**: The HolmesGPT API Service MUST provide a Flask-based HTTP server with production-ready configuration including CORS, request logging, error handling, and graceful shutdown.

**Priority**: P0 (CRITICAL)

**Rationale**: Flask is the Python HTTP framework - production-ready configuration ensures reliability and observability.

**Implementation**:
- **Framework**: Flask 3.0+
- **WSGI Server**: Gunicorn with 4 workers
- **Port**: 8080 (configurable)
- **Request Logging**: Structured JSON logs for all requests
- **Error Handling**: RFC 7807 Problem Details (pending implementation)
- **Graceful Shutdown**: DD-007 4-step shutdown pattern (pending implementation)
- **CORS**: Enabled for internal services

**Acceptance Criteria**:
- ✅ FastAPI server starts on port 8080
- ✅ uvicorn with production configuration
- ✅ Structured JSON request logging
- ✅ RFC 7807 error responses (BR-HAPI-200)
- ✅ Graceful shutdown with DD-007 pattern (BR-HAPI-201)
- ✅ CORS enabled for internal services

**Test Coverage**:
- Unit: HTTP server configuration tests + 7 RFC 7807 tests
- Integration: End-to-end HTTP request/response + 2 graceful shutdown tests
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Fully Implemented (100%)

**Recent Enhancements** (November 9, 2025):
1. **BR-HAPI-200**: RFC 7807 Error Response Standard (DD-004) - ✅ Implemented
2. **BR-HAPI-201**: Kubernetes-Aware Graceful Shutdown (DD-007) - ✅ Implemented

**Related BRs**: BR-HAPI-001 (Investigation), BR-HAPI-018 (Health Probes)

---

#### BR-HAPI-037 to 045: HTTP Server Variations

**Description**: Additional HTTP server capabilities:
- BR-HAPI-037: Request/response logging
- BR-HAPI-038: Error handling middleware
- BR-HAPI-039: CORS configuration
- BR-HAPI-040: Request timeout configuration
- BR-HAPI-041: Connection pooling
- BR-HAPI-042: HTTP/2 support (deferred)
- BR-HAPI-043: TLS configuration (delegated to service mesh)
- BR-HAPI-044: Request validation middleware
- BR-HAPI-045: Response compression (deferred)

**Priority**: P0 (CRITICAL) for BR-HAPI-037 to 041, P1 (HIGH) for BR-HAPI-042 to 045

**Implementation Status**: ✅ Implemented (BR-HAPI-037 to 041), ⏸️ Deferred (BR-HAPI-042 to 045)

**Related BRs**: BR-HAPI-036 (Core HTTP Server)

---

### Category 9: Observability & Metrics (BR-HAPI-301 to 303)

#### BR-HAPI-301: LLM Observability Metrics

**Description**: The HolmesGPT API Service MUST expose Prometheus metrics for LLM API call observability, including call counts, latency, token usage, and provider/model breakdown.

**Priority**: P0 (CRITICAL) - LLM is core business capability

**Rationale**:
- **Cost monitoring**: Track token usage for billing forecasting and cost optimization
- **Performance SLOs**: Monitor LLM latency by provider (OpenAI vs Claude vs Ollama)
- **Error tracking**: Alert on LLM API failures or degraded performance
- **Capacity planning**: Understand LLM workload patterns and provider distribution
- **Business intelligence**: Correlate model selection with investigation quality

**Metrics Specification**:

1. **LLM Call Counter**
   ```
   Metric: holmesgpt_api_llm_calls_total
   Type: Counter
   Labels: provider (openai | anthropic | ollama), model (gpt-4 | claude-3 | ...), status (success | error | timeout)
   Description: Total number of LLM API calls by provider, model, and outcome
   ```

2. **LLM Call Duration Histogram**
   ```
   Metric: holmesgpt_api_llm_call_duration_seconds
   Type: Histogram
   Labels: provider, model
   Buckets: (0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0)
   Description: LLM API call latency distribution (streaming excluded)
   ```

3. **LLM Token Usage Counter**
   ```
   Metric: holmesgpt_api_llm_token_usage_total
   Type: Counter
   Labels: provider, model, type (prompt | completion)
   Description: Total tokens consumed by LLM calls (for cost tracking)
   ```

**Service Level Objectives (SLOs)**:
- **OpenAI P95 Latency**: < 5 seconds
- **Claude P95 Latency**: < 10 seconds
- **Ollama P95 Latency**: < 2 seconds (local LLM)
- **LLM Error Rate**: < 1% (excluding rate limits)
- **Token Cost Alert**: > $100/day

**Acceptance Criteria**:
- ✅ Metrics exposed via `/metrics` endpoint
- ✅ Metrics follow DD-005 naming convention
- ✅ Metric name constants defined (DD-005 v3.0 compliance)
- ✅ Integration tests validate metric emission
- ✅ Cost dashboard queries documented

**Implementation Notes**:
- Metrics recorded in business logic (LLM client wrapper)
- Injectable metrics instance for integration test isolation
- Token usage updated after each successful LLM call
- Cost calculation: `tokens * model_price_per_1k` (external dashboard)

**Related Standards**: DD-005 v3.0 (Observability Standards)

---

#### BR-HAPI-302: HTTP Request Metrics (DD-005 Standard)

**Description**: The HolmesGPT API Service MUST expose standard HTTP request metrics per DD-005 observability standards.

**Priority**: P0 (CRITICAL) - Required by DD-005

**Rationale**:
- **Compliance**: DD-005 mandates HTTP metrics for all stateless services
- **API health**: Monitor endpoint availability and error rates
- **Performance**: Track request latency and throughput
- **Troubleshooting**: Correlate HTTP errors with business logic failures

**Metrics Specification**:

1. **HTTP Request Counter**
   ```
   Metric: holmesgpt_api_http_requests_total
   Type: Counter
   Labels: method (GET | POST), endpoint (/api/v1/incident/analyze | ...), status (200 | 400 | 500)
   Description: Total HTTP requests by method, endpoint, and status code
   ```

2. **HTTP Request Duration Histogram**
   ```
   Metric: holmesgpt_api_http_request_duration_seconds
   Type: Histogram
   Labels: method, endpoint
   Buckets: (0.01, 0.05, 0.1, 0.5, 1.0, 2.0, 5.0, 10.0) - Per DD-005 standard
   Description: HTTP request latency distribution (excluding LLM time)
   ```

**Service Level Objectives (SLOs)**:
- **P95 Latency**: < 100ms (HTTP overhead only, excludes LLM)
- **Availability**: > 99.9%
- **Error Rate**: < 0.1% (5xx errors)

**Acceptance Criteria**:
- ✅ Metrics exposed via `/metrics` endpoint
- ✅ Path normalization prevents cardinality explosion
- ✅ Buckets match DD-005 specification
- ✅ Integration tests validate metric emission

**Implementation Notes**:
- Metrics recorded in HTTP middleware (FastAPI/Starlette)
- Path normalization: `/api/v1/incidents/<id>` → `/api/v1/incidents/:id`
- Follows DD-005 Section 3.1 (Path Normalization)

**Related Standards**: DD-005 v3.0 Section 3.1 (HTTP Metrics)

---

#### BR-HAPI-303: Config Hot-Reload Metrics (Operational Visibility)

**Description**: The HolmesGPT API Service MUST expose metrics for ConfigMap hot-reload operations (BR-HAPI-199 compliance).

**Priority**: P1 (HIGH) - Operational visibility

**Rationale**: Operators need visibility into config reload events for troubleshooting and audit.

**Metrics Specification**:

1. **Config Reload Success Counter**
   ```
   Metric: holmesgpt_api_config_reload_total
   Type: Counter
   Labels: none
   Description: Total successful configuration reloads
   ```

2. **Config Reload Error Counter**
   ```
   Metric: holmesgpt_api_config_reload_errors_total
   Type: Counter
   Labels: none
   Description: Total failed configuration reload attempts
   ```

3. **Last Reload Timestamp**
   ```
   Metric: holmesgpt_api_config_last_reload_timestamp
   Type: Gauge
   Labels: none
   Description: Unix timestamp of last successful config reload
   ```

**Acceptance Criteria**:
- ✅ Metrics exposed via `/metrics` endpoint
- ✅ Metrics update on ConfigMap change events
- ✅ Integration tests validate reload metrics

**Implementation Status**: ✅ Implemented (BR-HAPI-199)

**Related BRs**: BR-HAPI-199 (ConfigMap Hot-Reload)

---

## 📊 Test Coverage Summary

### Unit Tests
- **Total**: 492 test specs (377 unit + 71 integration + 40 E2E + 4 smoke) - 100% passing

> **Note**: Test count updated December 2025 after multiple feature additions.
- **Coverage**: 95% confidence
- **Files**:
  - `test_recovery.py`: 27 tests (Recovery strategy analysis)
  - `test_postexec.py`: 24 tests (Post-execution effectiveness)
  - `test_models.py`: 23 tests (Pydantic data validation)
  - `test_health.py`: 30 tests (Health/readiness/config endpoints)

### Integration Tests
- **Total**: 3 test scenarios
- **Coverage**: 90% confidence
- **Files**:
  - `test_sdk_integration.py`: HolmesGPT SDK integration
  - `test_context_api_integration.py`: Context API integration (pending)
  - `test_real_llm_integration.py`: Real LLM provider integration

### E2E Tests
- **Status**: Deferred to v2.0
- **Reason**: Internal service with comprehensive unit/integration coverage

---

## ✅ Recent Enhancements (November 9, 2025)

### BR-HAPI-200: RFC 7807 Error Response Standard
**Status**: ✅ Implemented
**Priority**: P1 (HIGH)
**Implementation Time**: 2 hours
**Design Reference**: [DD-004: RFC 7807 Error Response Standard](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md)

**Implementation**:
- RFC7807Error Pydantic model with all required fields
- Error type URI constants (kubernaut.io/errors/*)
- FastAPI exception handlers for all error types
- Request ID propagation for tracing
- Content-Type: application/problem+json

**Test Coverage**:
- Unit: 7 tests (test_rfc7807_errors.py)
- Validates error model structure, URI format, and HTTP status codes

**Impact**: ✅ Consistent error handling across all Kubernaut services

---

### BR-HAPI-201: Kubernetes-Aware Graceful Shutdown
**Status**: ✅ Implemented
**Priority**: P0 (CRITICAL)
**Implementation Time**: 3 hours
**Design Reference**: [DD-007: Kubernetes-Aware Graceful Shutdown](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)

**Implementation**:
- Global is_shutting_down flag for readiness coordination
- SIGTERM/SIGINT signal handlers
- /ready endpoint returns 503 during shutdown
- /health stays 200 during shutdown (liveness probe)
- uvicorn handles in-flight request completion automatically

**Test Coverage**:
- Integration: 2 tests (test_graceful_shutdown.py)
- Test 1: Readiness probe coordination (P0)
- Test 2: In-flight request completion (documents uvicorn behavior)

**Impact**: ✅ Zero-downtime deployments during rolling updates

**Impact**: Zero-downtime deployments, reliable rolling updates

---

### BR-HAPI-199: ConfigMap Hot-Reload (V1.0)
**Status**: ✅ **Complete**
**Priority**: P1 (HIGH)
**Implementation Time**: ~9 hours
**Design Reference**: [DD-HAPI-004: ConfigMap Hot-Reload](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md)

**Description**: The HolmesGPT API Service MUST support hot-reload of configuration from mounted ConfigMap without pod restart.

**Hot-Reloadable Fields**:
| Field | Business Use Case |
|-------|-------------------|
| `llm.model` | Cost/quality switching |
| `llm.provider` | Provider failover |
| `llm.endpoint` | Endpoint switching |
| `llm.max_retries` | Retry tuning |
| `llm.timeout_seconds` | Timeout adjustment |
| `llm.temperature` | Response tuning |
| `toolsets.*` | Feature toggles |
| `log_level` | Debug enablement |

**Acceptance Criteria**:
- [x] Service reloads config from ConfigMap within 90 seconds
- [x] Invalid config gracefully degrades (keeps previous)
- [x] Configuration hash logged on reload for audit
- [x] Metrics exposed for reload count and errors

**Implementation** (December 7, 2025):
- `FileWatcher` class with `watchdog` library for file system monitoring
- `ConfigManager` class for thread-safe config access
- Prometheus metrics: `hapi_config_reload_total`, `hapi_config_reload_errors_total`, `hapi_config_last_reload_timestamp_seconds`
- Graceful degradation on invalid config
- Integration with `main.py` startup

**Full Specification**: [BR-HAPI-199](../../../requirements/BR-HAPI-199-configmap-hot-reload.md)

---

### BR-HAPI-211: LLM Input Sanitization (V1.0)
**Status**: ✅ **IMPLEMENTED** (December 10, 2025)
**Priority**: P0 (CRITICAL)
**Implementation Time**: ~5 hours (actual)
**Design Reference**: [DD-HAPI-005: LLM Input Sanitization](../../../architecture/decisions/DD-HAPI-005-llm-input-sanitization.md)

**Description**: The HolmesGPT API Service MUST sanitize ALL data sent to external LLM providers to prevent credential leakage.

**Business Justification**:
- External LLM providers (OpenAI, Anthropic) receive prompts and tool results
- This data may contain credentials from logs, error messages, or workflow parameters
- Without sanitization, credentials leak to external services

**Sanitization Scope**:

| Data Flow | Risk Level | Mitigation |
|-----------|------------|------------|
| Initial prompts (`error_message`, `description`) | 🔴 HIGH | `sanitize_for_llm()` before prompt construction |
| Tool results (`kubectl logs`, `kubectl get`) | 🔴 HIGH | Wrap `Tool.invoke()` to sanitize results |
| Error messages from tool execution | 🟡 MEDIUM | Sanitize `StructuredToolResult.error` field |

**Implementation**:
- `src/sanitization/llm_sanitizer.py` - Core sanitizer with 28 regex patterns
- `src/sanitization/__init__.py` - Public API (`sanitize_for_llm`, `sanitize_with_fallback`)
- `src/extensions/llm_config.py` - `_wrap_tool_results_with_sanitization()` tool wrapper
- `src/extensions/incident.py` - Prompt sanitization before LLM call
- `src/extensions/recovery.py` - Prompt sanitization before LLM call

**Sanitization Patterns** (DD-005 Compliant, 28 rules):
- Passwords (JSON, URL, plain text, *_password variants)
- API keys (`api_key=`, `OPENAI_API_KEY=`, `sk-*` OpenAI keys)
- Tokens (`Bearer`, JWT, GitHub PATs `ghp_*`, `gho_*`)
- Database URLs (PostgreSQL, MySQL, MongoDB, Redis)
- AWS credentials (access keys `AKIA*`, secret keys)
- Private keys (PEM format, RSA, EC)
- Kubernetes Secret data (base64)
- Certificates (PEM format)
- Authorization headers

**Acceptance Criteria**:
- [x] Design decision documented (DD-HAPI-005)
- [x] Business requirement specified (BR-HAPI-211)
- [x] `src/sanitization/llm_sanitizer.py` implemented (28 patterns)
- [x] Tool.invoke() wrapper implemented in `llm_config.py`
- [x] Prompt sanitization in `incident.py` and `recovery.py`
- [x] **46 unit tests** validating all patterns (exceeds 15+ requirement)
- [x] Data type handling (str, dict, list, None)
- [x] Fallback sanitization for regex errors

**Security Guarantees**:
| Guarantee | Verification |
|-----------|--------------|
| No passwords leak to LLM | 46 unit tests + pattern coverage |
| No API keys leak to LLM | 46 unit tests + pattern coverage |
| No DB credentials leak to LLM | 46 unit tests + URL parsing |
| No K8s secrets leak to LLM | 46 unit tests + base64 detection |

**Test Coverage**:
- Unit: `tests/unit/test_llm_sanitizer.py` (46 tests, 100% passing)

**Full Specification**: [BR-HAPI-211](../../../requirements/BR-HAPI-211-llm-input-sanitization.md)

---

### BR-HAPI-212: Mock LLM Mode for Integration Testing (V1.0)
**Status**: ✅ **IMPLEMENTED** (December 10, 2025)
**Priority**: P1 (HIGH)
**Implementation Time**: ~3 hours
**Design Reference**: [RESPONSE_HAPI_MOCK_LLM_MODE.md](../../../../handoff/RESPONSE_HAPI_MOCK_LLM_MODE.md)

**Description**: The HolmesGPT API Service MUST provide a mock LLM mode for integration testing, allowing consumer services (AIAnalysis, etc.) to run tests without:
- LLM API costs
- Non-deterministic responses
- API key requirements in CI/CD

**Business Justification**:
- AIAnalysis team requested mock mode for integration testing (REQUEST_HAPI_MOCK_LLM_MODE.md)
- CI/CD pipelines should not require LLM API keys
- Deterministic responses enable reliable assertions in tests

**Implementation**:
- **Environment Variable**: `MOCK_LLM_MODE=true`
- **Mock Response Generator**: `src/mock_responses.py`
- **Signal Type Mapping**: 6 pre-defined scenarios (OOMKilled, CrashLoopBackOff, NodeNotReady, ImagePullBackOff, Evicted, FailedScheduling)
- **Endpoints Supported**: `/incident/analyze`, `/recovery/analyze`

**Acceptance Criteria**:
- [x] `MOCK_LLM_MODE=true` environment variable enables mock mode
- [x] Mock responses are schema-compliant (pass IncidentResponse/RecoveryResponse validation)
- [x] Mock responses are deterministic based on input `signal_type`
- [x] Request validation still runs (catches invalid requests)
- [x] No LLM API calls made when mock mode enabled
- [x] Works with both `/incident/analyze` and `/recovery/analyze` endpoints
- [x] 24+ unit tests validating mock mode behavior

**Test Coverage**:
- Unit: `tests/unit/test_mock_mode.py` (24 tests)

---

## 📋 Deferred BRs (139 BRs for v2.0)

The following 140 BRs are deferred to v2.0 and only needed if the service becomes externally exposed:

### Advanced Security (BR-HAPI-068 to 125) - 58 BRs
- API key authentication
- OAuth2/OIDC integration
- mTLS client authentication
- Advanced RBAC policies
- Rate limiting per user/tenant
- IP allowlisting/denylisting

**Reason for Deferral**: K8s RBAC + network policies sufficient for internal service

---

### Performance/Rate Limiting (BR-HAPI-126 to 145) - 20 BRs
- Request rate limiting
- Concurrent request limits
- Token bucket algorithm
- Sliding window rate limiting
- Per-endpoint rate limits

**Reason for Deferral**: Network policies + K8s quotas handle this for internal service

---

### Advanced Configuration (BR-HAPI-146 to 165) - 19 BRs (1 moved to V1.0)
- ~~Dynamic configuration reloading~~ → **Moved to V1.0 as BR-HAPI-199**
- Feature flags
- A/B testing configuration
- Multi-environment config
- Configuration validation

**Reason for Deferral**: Advanced config features not needed for internal service (basic hot-reload in V1.0)

---

### Advanced Integration (BR-HAPI-166 to 179) - 14 BRs
- Webhook notifications
- External API integrations
- Message queue integration
- Event streaming
- Data export capabilities

**Reason for Deferral**: Simple integration sufficient for internal service

---

### Additional Health (BR-HAPI-020 to 027) - 8 BRs
- Detailed health metrics
- Dependency health checks
- Health check aggregation
- Custom health indicators

**Reason for Deferral**: Basic probes sufficient for internal service

---

### Container/Deployment (BR-HAPI-046 to 065) - 20 BRs
- Multi-stage Docker builds
- Image scanning
- Helm chart configuration
- Resource limits tuning
- Horizontal Pod Autoscaling

**Reason for Deferral**: Standard K8s patterns handle deployment

---

## 🔗 Related Documentation

- [IMPLEMENTATION_PLAN_V3.0.md](./implementation/IMPLEMENTATION_PLAN_V3.0.md) - Complete implementation plan
- [api-specification.md](./api-specification.md) - API specification with examples
- [overview.md](./overview.md) - Service architecture and design decisions
- [DD-HOLMESGPT-012](../../../architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md) - Minimal service architecture decision
- [DD-HAPI-004](../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md) - ConfigMap hot-reload (V1.0)
- [DD-HAPI-005](../../../architecture/decisions/DD-HAPI-005-llm-input-sanitization.md) - LLM Input Sanitization (V1.0) 📋
- [DD-004](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md) - RFC 7807 error responses
- [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) - Graceful shutdown pattern
- [BR-HAPI-199](../../../requirements/BR-HAPI-199-configmap-hot-reload.md) - Hot-reload business requirements
- [BR-HAPI-211](../../../requirements/BR-HAPI-211-llm-input-sanitization.md) - LLM Input Sanitization 📋

---

**Document Version**: 1.4
**Last Updated**: 2026-02-12
**Changelog (v1.4)**: DRIFT-HAPI-2: Resolved BR number collision. BR-HAPI-016/017 reassigned to Remediation History Context and Three-Step Workflow Discovery (per codebase usage). Health Probes renumbered to BR-HAPI-018, Configuration Endpoint to BR-HAPI-019. Deferred Additional Health range updated to BR-HAPI-020 to 027.
**Maintained By**: Kubernaut Architecture Team
**Status**: 📋 V1.0 In Progress (50 BRs implemented, 1 planned)

