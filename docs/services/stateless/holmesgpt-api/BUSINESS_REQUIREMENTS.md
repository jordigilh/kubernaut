# HolmesGPT API Service - Business Requirements

**Service**: HolmesGPT API Service
**Service Type**: Stateless HTTP API (Python)
**Version**: v3.0 (Minimal Internal Service)
**Last Updated**: November 8, 2025
**Status**: Production-Ready (with 2 pending enhancements)

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

**Total Business Requirements**: 45 essential BRs (140 deferred BRs for v2.0)
**Categories**: 7
**Priority Breakdown**:
- P0 (Critical): 43 BRs (core business logic)
- P1 (High): 2 BRs (pending enhancements)

**Implementation Status**:
- ✅ Implemented: 43 BRs (95.6%)
- ⏸️ Pending: 2 BRs (4.4%) - RFC 7807 errors, graceful shutdown

**Test Coverage**:
- Unit: 104 test specs (100% passing, 95% confidence)
- Integration: 3 test scenarios (SDK, Context API, Real LLM)
- E2E: Not yet implemented (planned for v2.0)

**Deferred BRs**: 140 BRs deferred to v2.0 (advanced security, rate limiting, advanced configuration) - only needed if service becomes externally exposed

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

#### BR-HAPI-002 to BR-HAPI-015: Investigation Endpoint Variations

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
- BR-HAPI-011: Investigation metrics
- BR-HAPI-012: Investigation rate limiting (deferred)
- BR-HAPI-013: Investigation authentication
- BR-HAPI-014: Investigation authorization (deferred)
- BR-HAPI-015: Investigation audit trail (deferred)

**Priority**: P0 (CRITICAL) for BR-HAPI-002 to 011, P1 (HIGH) for BR-HAPI-012 to 015

**Implementation Status**: ✅ Implemented (BR-HAPI-002 to 011), ⏸️ Deferred (BR-HAPI-012 to 015)

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
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Implemented (100%)

**Related BRs**: BR-HAPI-RECOVERY-001 (Recovery Analysis), BR-HAPI-001 (Investigation)

---

#### BR-HAPI-POSTEXEC-002 to 005: Post-Execution Analysis Variations

**Description**: Additional post-execution analysis capabilities:
- BR-HAPI-POSTEXEC-002: Post-execution metrics collection
- BR-HAPI-POSTEXEC-003: Post-execution trend analysis
- BR-HAPI-POSTEXEC-004: Post-execution learning feedback loop
- BR-HAPI-POSTEXEC-005: Post-execution report generation

**Priority**: P0 (CRITICAL)

**Implementation Status**: ✅ Implemented (100%)

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

### Category 5: Health & Status (BR-HAPI-016 to 017)

#### BR-HAPI-016: Kubernetes Health Probes

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

#### BR-HAPI-017: Configuration Endpoint

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

**Related BRs**: BR-HAPI-016 (Health Probes)

---

### Category 6: Basic Authentication (BR-HAPI-066 to 067)

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

### Category 7: HTTP Server (BR-HAPI-036 to 045)

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
- ✅ Flask server starts on port 8080
- ✅ Gunicorn with 4 workers for production
- ✅ Structured JSON request logging
- ⏸️ RFC 7807 error responses (pending - BR-HAPI-036-PENDING-1)
- ⏸️ Graceful shutdown (pending - BR-HAPI-036-PENDING-2)
- ✅ CORS enabled for internal services

**Test Coverage**:
- Unit: HTTP server configuration tests
- Integration: End-to-end HTTP request/response
- E2E: Deferred to v2.0

**Implementation Status**: ✅ Partially Implemented (90% - 2 pending enhancements)

**Pending Enhancements**:
1. **BR-HAPI-036-PENDING-1**: RFC 7807 Error Response Standard (DD-004)
2. **BR-HAPI-036-PENDING-2**: Kubernetes-Aware Graceful Shutdown (DD-007)

**Related BRs**: BR-HAPI-001 (Investigation), BR-HAPI-016 (Health Probes)

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

## 📊 Test Coverage Summary

### Unit Tests
- **Total**: 104 test specs (100% passing)
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

## 🚧 Pending Enhancements (2 Blockers)

### 1. RFC 7807 Error Response Standard (BR-HAPI-036-PENDING-1)
**Status**: ⏸️ Pending
**Priority**: P1 (HIGH)
**Estimated Effort**: 2-3 hours
**Design Reference**: [DD-004: RFC 7807 Error Response Standard](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md)

**Current State**: Service returns generic JSON error responses
**Target State**: Structured error responses following RFC 7807 standard

**Impact**: Consistent error handling across all Kubernaut services

---

### 2. Kubernetes-Aware Graceful Shutdown (BR-HAPI-036-PENDING-2)
**Status**: ⏸️ Pending
**Priority**: P1 (HIGH)
**Estimated Effort**: 3-4 hours
**Design Reference**: [DD-007: Kubernetes-Aware Graceful Shutdown](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md)

**Current State**: Service stops immediately on SIGTERM
**Target State**: Graceful termination with connection draining (4-step pattern)

**Impact**: Zero-downtime deployments, reliable rolling updates

---

## 📋 Deferred BRs (140 BRs for v2.0)

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

### Advanced Configuration (BR-HAPI-146 to 165) - 20 BRs
- Dynamic configuration reloading
- Feature flags
- A/B testing configuration
- Multi-environment config
- Configuration validation

**Reason for Deferral**: Minimal config sufficient for internal service

---

### Advanced Integration (BR-HAPI-166 to 179) - 14 BRs
- Webhook notifications
- External API integrations
- Message queue integration
- Event streaming
- Data export capabilities

**Reason for Deferral**: Simple integration sufficient for internal service

---

### Additional Health (BR-HAPI-018 to 025) - 8 BRs
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

- [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md) - Complete implementation plan
- [api-specification.md](./api-specification.md) - API specification with examples
- [overview.md](./overview.md) - Service architecture and design decisions
- [DD-HOLMESGPT-012](../../../architecture/decisions/DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md) - Minimal service architecture decision
- [DD-004](../../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md) - RFC 7807 error responses
- [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) - Graceful shutdown pattern

---

**Document Version**: 1.0
**Last Updated**: November 8, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: Production-Ready (with 2 pending enhancements)

