# HolmesGPT API Service - Implementation Checklist

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP Service (Python REST API)
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## 📚 Reference Documentation

**CRITICAL**: Read these documents before starting implementation:

- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md) - Python TDD with pytest (70%+ unit, >50% integration)
- **Security Configuration**: [security-configuration.md](./security-configuration.md) - TokenReviewer + prompt injection prevention
- **Integration Points**: [integration-points.md](./integration-points.md) - HolmesGPT SDK, LLM providers, K8s API
- **Core Methodology**: [00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc) - APDC-Enhanced TDD
- **AI/ML Guidelines**: [04-ai-ml-guidelines.mdc](../../.cursor/rules/04-ai-ml-guidelines.mdc) - AI-specific TDD patterns
- **Business Requirements**: Map all implementation to BR-HOLMES-001 through BR-HOLMES-180

---

## 📋 Implementation Overview

This checklist ensures complete and correct implementation of the HolmesGPT API Service following **mandatory** APDC-Enhanced TDD methodology (Analysis → Plan → Do-RED → Do-GREEN → Do-REFACTOR → Check) and project specifications.

**Note**: This is a **Python service**, so implementation uses Python/FastAPI with pytest instead of Go/Ginkgo.

---

## ✅ Phase 1: Core Infrastructure (Week 1)

### **1.1 Project Structure**
- [ ] Create `src/` directory structure for Python code
- [ ] Create `src/main.py` FastAPI application entry point
- [ ] Create `tests/unit/` for unit tests
- [ ] Create `tests/integration/` for integration tests
- [ ] Create `tests/e2e/` for end-to-end tests
- [ ] Create `deploy/holmesgpt-api/` for Kubernetes manifests
- [ ] Create `requirements.txt` with dependencies
- [ ] Create `Dockerfile` for container build

### **1.2 Python Dependencies**
- [ ] FastAPI for REST API framework
- [ ] HolmesGPT SDK for investigation engine
- [ ] kubernetes Python client for K8s API access
- [ ] httpx for async HTTP client
- [ ] prometheus-client for metrics
- [ ] pytest for testing framework
- [ ] pydantic for request/response validation

### **1.3 Configuration Management**
- [ ] Configuration structs defined (`src/config.py`)
- [ ] Environment variable overrides supported
- [ ] Kubernetes secrets for LLM API keys
- [ ] Configuration validation on startup

---

## ✅ Phase 2: Authentication & Authorization (Week 1-2)

### **2.1 Kubernetes TokenReviewer**
- [ ] TokenReviewer client implemented (`src/auth/token_reviewer.py`)
- [ ] FastAPI middleware for Bearer token extraction
- [ ] Token validation integrated with Kubernetes API
- [ ] Failed authentication logging implemented

### **2.2 RBAC Configuration**
- [ ] ServiceAccount created (`holmesgpt-api-sa`)
- [ ] ClusterRole created with read-only K8s access
- [ ] ClusterRoleBinding created linking SA to ClusterRole
- [ ] Authorization middleware implemented
- [ ] Service account validation for investigation requests

### **2.3 Security Testing**
- [ ] Unit tests for token extraction
- [ ] Integration tests for TokenReviewer validation
- [ ] Authorization tests for different service accounts
- [ ] Failed authentication tests

---

## ✅ Phase 3: Core Business Logic (Week 2-3)

### **3.1 Investigation Models** (TDD RED Phase)
- [ ] Write failing unit tests for investigation models
- [ ] `InvestigationRequest` Pydantic model defined
- [ ] `InvestigationResponse` Pydantic model defined
- [ ] JSON schema validation tested
- [ ] Field validation rules tested

### **3.2 Prompt Generation** (TDD RED → GREEN)
- [ ] Write failing unit tests for prompt generation (BR-HOLMES-003)
- [ ] Test prompt injection prevention
- [ ] Test prompt template parameterization
- [ ] `InvestigationPromptBuilder` class implemented
- [ ] Prompt sanitization implemented
- [ ] All prompt tests passing

### **3.3 Response Parsing** (TDD RED → GREEN)
- [ ] Write failing unit tests for response parsing
- [ ] Test structured response extraction
- [ ] Test confidence score normalization
- [ ] `HolmesGPTResponseParser` class implemented
- [ ] All response parsing tests passing

---

## ✅ Phase 4: HolmesGPT SDK Integration (Week 3-4)

### **4.1 HolmesGPT SDK Client** (TDD RED → GREEN)
- [ ] Write failing integration tests for SDK (BR-HOLMES-001)
- [ ] Test investigation with real HolmesGPT SDK
- [ ] Test toolset discovery and loading
- [ ] `HolmesGPTClient` class implemented
- [ ] SDK error handling implemented
- [ ] All SDK integration tests passing

### **4.2 Toolset Management**
- [ ] ConfigMap polling for dynamic toolset configuration
- [ ] Toolset registration with HolmesGPT SDK
- [ ] Hot-reload of toolset configuration
- [ ] Toolset validation on load

### **4.3 LLM Provider Integration** (TDD RED → GREEN)
- [ ] Write failing integration tests for LLM providers (BR-HOLMES-002)
- [ ] OpenAI client implementation
- [ ] Anthropic (Claude) client implementation
- [ ] Local model client implementation (Ollama)
- [ ] Provider fallback logic
- [ ] All LLM integration tests passing

---

## ✅ Phase 5: Kubernetes API Integration (Week 4)

### **5.1 Kubernetes Inspector** (TDD RED → GREEN)
- [ ] Write failing integration tests for K8s API (BR-HOLMES-005)
- [ ] Test pod listing and inspection
- [ ] Test pod log retrieval
- [ ] Test deployment description
- [ ] `KubernetesInspector` class implemented
- [ ] Read-only K8s access enforced
- [ ] All K8s API tests passing

### **5.2 RBAC Testing**
- [ ] Verify read-only access (no write operations)
- [ ] Test access denial for unauthorized resources
- [ ] Test ClusterRole permissions

---

## ✅ Phase 6: HTTP API Implementation (Week 5)

### **6.1 API Endpoints** (TDD RED → GREEN)
- [ ] Write failing integration tests for API endpoints
- [ ] `POST /api/v1/investigate` implemented
- [ ] Request validation with Pydantic
- [ ] Response formatting (200 OK, error responses)
- [ ] All API endpoint tests passing

### **6.2 FastAPI Middleware**
- [ ] Authentication middleware applied to all routes
- [ ] Authorization middleware applied to all routes
- [ ] Rate limiting middleware (100 req/min per service)
- [ ] Request logging middleware with correlation IDs
- [ ] CORS configuration (if needed)

### **6.3 Error Handling**
- [ ] Structured error responses (JSON format)
- [ ] HTTP status codes (400, 401, 403, 429, 500, 503)
- [ ] Error logging with stack traces
- [ ] Graceful degradation for LLM provider failures

---

## ✅ Phase 7: Cross-Service Integration (Week 5-6)

### **7.1 Integration with AI Analysis Controller**
- [ ] AI Analysis Controller can call HolmesGPT API
- [ ] Integration test: AI Analysis → HolmesGPT API
- [ ] Error handling for invalid AI Analysis requests

### **7.2 Integration with Workflow Execution Controller**
- [ ] Workflow Execution can call HolmesGPT API
- [ ] Integration test: Workflow Execution → HolmesGPT API
- [ ] Error handling for invalid workflow requests

### **7.3 Integration with Context API**
- [ ] HolmesGPT API can retrieve historical context
- [ ] Integration test: HolmesGPT API → Context API
- [ ] Historical success rate integration

---

## ✅ Phase 8: Observability (Week 6)

### **8.1 Logging** (Python logging)
- [ ] Structured logging configured (JSON format)
- [ ] Log levels configurable (DEBUG, INFO, WARN, ERROR)
- [ ] Security event logging (authentication, authorization)
- [ ] Investigation request/response logging
- [ ] LLM API call logging (sanitized prompts/responses)

### **8.2 Metrics** (Prometheus)
- [ ] Metrics server on port 9090
- [ ] `holmesgpt_investigations_total` counter (by status)
- [ ] `holmesgpt_investigation_duration_seconds` histogram
- [ ] `holmesgpt_llm_api_calls_total` counter (by provider)
- [ ] `holmesgpt_llm_api_duration_seconds` histogram
- [ ] `holmesgpt_rate_limit_exceeded_total` counter
- [ ] Metrics endpoint secured with TokenReviewer

### **8.3 Health Checks**
- [ ] `/healthz` endpoint (liveness probe)
- [ ] `/readyz` endpoint (readiness probe)
- [ ] HolmesGPT SDK health check
- [ ] LLM provider health check
- [ ] Kubernetes API health check
- [ ] Health checks on port 8080

### **8.4 Grafana Dashboard**
- [ ] Dashboard created for HolmesGPT API metrics
- [ ] Panels: Investigation rate, success rate, p95 latency
- [ ] Panels: LLM API calls, cost tracking
- [ ] Panels: K8s API usage
- [ ] Alert rules for high error rates, LLM failures

---

## ✅ Phase 9: Testing & Quality (Week 7)

### **9.1 Unit Tests** (70%+ Coverage)
- [ ] Prompt generation tests (10+ scenarios)
- [ ] Response parsing tests
- [ ] Input sanitization tests (prompt injection)
- [ ] Unit test coverage ≥70%

### **9.2 Integration Tests** (>50% Coverage)
- [ ] HolmesGPT SDK integration tests
- [ ] LLM provider integration tests (OpenAI, Anthropic)
- [ ] Kubernetes API integration tests
- [ ] Cross-service integration tests (AI Analysis, Workflow Execution, Context API)
- [ ] Rate limiting tests
- [ ] Integration test coverage >50%

### **9.3 E2E Tests** (10-15% Coverage)
- [ ] Complete investigation flow test
- [ ] End-to-end with real LLM provider
- [ ] End-to-end with real K8s cluster
- [ ] E2E test coverage 10-15%

### **9.4 Load Testing**
- [ ] Load test: 100 investigations/minute sustained
- [ ] Load test: 500 investigations/minute burst
- [ ] Load test: 1000 concurrent connections
- [ ] LLM API rate limit testing

---

## ✅ Phase 10: Deployment (Week 7-8)

### **10.1 Container Build**
- [ ] Dockerfile optimized for Python (multi-stage build)
- [ ] Container image size < 500 MB
- [ ] Non-root user in container
- [ ] Security scanning passed (no high CVEs)

### **10.2 Kubernetes Manifests**
- [ ] Deployment manifest with 2-3 replicas
- [ ] HorizontalPodAutoscaler (2-10 replicas)
- [ ] Service manifest (ClusterIP)
- [ ] ServiceAccount, ClusterRole, ClusterRoleBinding manifests
- [ ] NetworkPolicy manifest
- [ ] PodDisruptionBudget manifest

### **10.3 ConfigMaps & Secrets**
- [ ] ConfigMap for HolmesGPT API configuration
- [ ] ConfigMap for dynamic toolset configuration
- [ ] Secret for LLM API keys (OpenAI, Anthropic)
- [ ] Environment-specific configurations (dev, staging, prod)

### **10.4 ServiceMonitor**
- [ ] ServiceMonitor for Prometheus scraping
- [ ] Metrics endpoint configured (port 9090)
- [ ] Label selectors correct

### **10.5 Deployment Validation**
- [ ] Deploy to dev environment
- [ ] Health checks passing
- [ ] Metrics scraped by Prometheus
- [ ] Logs visible in centralized logging
- [ ] Integration tests passing against deployed service

---

## ✅ Phase 11: Documentation (Week 8)

### **11.1 API Documentation**
- [ ] OpenAPI 3.0 spec generated from FastAPI
- [ ] API examples for each endpoint
- [ ] Error response documentation
- [ ] Authentication/authorization requirements documented

### **11.2 Operational Documentation**
- [ ] Runbook for common issues
- [ ] Troubleshooting guide
- [ ] LLM provider configuration guide
- [ ] Toolset configuration guide

### **11.3 Architecture Decision Records**
- [ ] ADR: Why Python/FastAPI for HolmesGPT wrapper
- [ ] ADR: LLM provider selection and fallback strategy
- [ ] ADR: Dynamic toolset configuration approach
- [ ] ADR: Read-only Kubernetes access

---

## 🎯 Definition of Done

### **Service is production-ready when:**

- ✅ All unit tests passing (≥70% coverage)
- ✅ All integration tests passing (>50% coverage)
- ✅ All E2E tests passing (10-15% coverage)
- ✅ Load tests passing (100 investigations/minute sustained)
- ✅ Deployed to staging environment successfully
- ✅ Health checks passing in staging
- ✅ Metrics visible in Prometheus
- ✅ Logs visible in centralized logging
- ✅ Security review completed
- ✅ LLM API cost monitoring operational
- ✅ Documentation complete
- ✅ Operational runbook reviewed

---

## 🚨 Critical Path Items

### **Must be completed before production:**

1. **Authentication**: TokenReviewer authentication implemented and tested
2. **Authorization**: RBAC enforced for all investigation requests
3. **LLM Security**: API keys in secrets, prompt injection prevention
4. **K8s RBAC**: Read-only cluster access enforced
5. **Monitoring**: Prometheus metrics and Grafana dashboards operational
6. **Cost Control**: LLM API rate limiting and cost monitoring
7. **Testing**: All test suites passing with required coverage

---

## 📊 Progress Tracking

| Phase | Status | Completion Date |
|-------|--------|----------------|
| Phase 1: Core Infrastructure | ⏸️ Not Started | TBD |
| Phase 2: Authentication & Authorization | ⏸️ Not Started | TBD |
| Phase 3: Core Business Logic | ⏸️ Not Started | TBD |
| Phase 4: HolmesGPT SDK Integration | ⏸️ Not Started | TBD |
| Phase 5: Kubernetes API Integration | ⏸️ Not Started | TBD |
| Phase 6: HTTP API Implementation | ⏸️ Not Started | TBD |
| Phase 7: Cross-Service Integration | ⏸️ Not Started | TBD |
| Phase 8: Observability | ⏸️ Not Started | TBD |
| Phase 9: Testing & Quality | ⏸️ Not Started | TBD |
| Phase 10: Deployment | ⏸️ Not Started | TBD |
| Phase 11: Documentation | ⏸️ Not Started | TBD |

**Overall Progress**: 0% (Design phase complete, implementation pending)

---

## 🐍 Python-Specific Considerations

### **Virtual Environment**
```bash
python -m venv venv
source venv/bin/activate  # Linux/Mac
venv\Scripts\activate     # Windows
pip install -r requirements.txt
```

### **Code Quality Tools**
- [ ] black for code formatting
- [ ] flake8 for linting
- [ ] mypy for type checking
- [ ] bandit for security scanning

### **Development Workflow**
```bash
# Run tests
pytest tests/unit/ -v
pytest tests/integration/ -v

# Run with coverage
pytest --cov=src --cov-report=html

# Format code
black src/ tests/

# Lint
flake8 src/ tests/

# Type check
mypy src/
```

---

## 🔗 Reference Documentation

- **Overview**: `docs/services/stateless/holmesgpt-api/overview.md`
- **API Specification**: `docs/services/stateless/holmesgpt-api/api-specification.md`
- **Testing Strategy**: `docs/services/stateless/holmesgpt-api/testing-strategy.md`
- **Security Configuration**: `docs/services/stateless/holmesgpt-api/security-configuration.md`
- **Integration Points**: `docs/services/stateless/holmesgpt-api/integration-points.md`
- **APDC Methodology**: `.cursor/rules/00-core-development-methodology.mdc`
- **Testing Standards**: `.cursor/rules/03-testing-strategy.mdc`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Implementation Status**: ⏸️ **Pending** (Design phase complete)
**Language**: Python 3.11+
**Framework**: FastAPI 0.104+

