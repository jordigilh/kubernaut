# Dynamic Toolset Service - Implementation Checklist

**Version**: v1.0
**Last Updated**: October 6, 2025
**Service Type**: Stateless HTTP API + Kubernetes Controller
**Port**: 8080 (REST API + Health), 9090 (Metrics)

---

## 📚 Reference Documentation

**CRITICAL**: Read these documents before starting implementation:

- **Testing Strategy**: [testing-strategy.md](./testing-strategy.md) - Hybrid testing (70%+ unit, >50% integration, controller reconciliation)
- **Security Configuration**: [security-configuration.md](./security-configuration.md) - TokenReviewer + RBAC for service discovery
- **Integration Points**: [integration-points.md](./integration-points.md) - K8s Service watch, ConfigMap reconciliation, HolmesGPT API
- **Core Methodology**: [00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc) - APDC-Enhanced TDD
- **Business Requirements**: BR-TOOLSET-001 through BR-TOOLSET-180
  - **V1 Scope**: BR-TOOLSET-001 to BR-TOOLSET-010 (documented in testing-strategy.md)
  - **Reserved for Future**: BR-TOOLSET-011 to BR-TOOLSET-180 (V2, V3 expansions)

---

## 📋 Implementation Overview

This checklist ensures complete and correct implementation of the Dynamic Toolset Service following **mandatory** APDC-Enhanced TDD methodology (Analysis → Plan → Do-RED → Do-GREEN → Do-REFACTOR → Check) and project specifications.

**Note**: This is a **hybrid service** (HTTP API + Kubernetes Controller using controller-runtime), so implementation includes both REST API and controller reconciliation components.

---

## ✅ Phase 1: Core Infrastructure (Week 1)

### **1.1 Project Structure**
- [ ] Create `pkg/dynamictoolset/` directory structure
- [ ] Create `cmd/dynamic-toolset/main.go` entry point
- [ ] Create `test/unit/dynamictoolset/` for unit tests
- [ ] Create `test/integration/dynamictoolset/` for integration tests
- [ ] Create `deploy/dynamic-toolset/` for Kubernetes manifests

### **1.2 Controller-Runtime Setup**
- [ ] Initialize controller-runtime manager
- [ ] Configure leader election (for multi-replica deployments)
- [ ] Setup ConfigMap reconciler
- [ ] Setup Service watcher
- [ ] Configure logging with Zap (`sigs.k8s.io/controller-runtime/pkg/log/zap`)

### **1.3 Configuration Management**
- [ ] Configuration structs defined (`internal/config/config.go`)
- [ ] Environment variable overrides supported
- [ ] Configuration validation on startup
- [ ] Discovery intervals configurable

---

## ✅ Phase 2: Authentication & Authorization (Week 1-2)

### **2.1 Kubernetes TokenReviewer**
- [ ] TokenReviewer client implemented (`pkg/auth/tokenreviewer.go`)
- [ ] HTTP middleware for Bearer token extraction
- [ ] Token validation integrated with Kubernetes API
- [ ] Failed authentication logging implemented

### **2.2 RBAC Configuration**
- [ ] ServiceAccount created (`dynamic-toolset-sa`)
- [ ] ClusterRole created with read-only service discovery
- [ ] ClusterRole includes ConfigMap write access (specific ConfigMap)
- [ ] ClusterRole includes leader election permissions
- [ ] ClusterRoleBinding created linking SA to ClusterRole
- [ ] Authorization middleware implemented

### **2.3 Security Testing**
- [ ] Unit tests for token extraction
- [ ] Integration tests for TokenReviewer validation
- [ ] Authorization tests for different service accounts
- [ ] Failed authentication tests

---

## ✅ Phase 3: Service Discovery Logic (Week 2-3)

### **3.1 Service Detection** (TDD RED Phase)
- [ ] Write failing unit tests for service detection (BR-TOOLSET-001)
- [ ] Test Prometheus detection (by label and port)
- [ ] Test Grafana detection (by label and port)
- [ ] Test Jaeger detection
- [ ] Test Elasticsearch detection
- [ ] Test unknown service rejection

### **3.2 Service Detection Implementation** (TDD GREEN Phase)
- [ ] `ServiceDetector` interface defined
- [ ] `DetectServiceType()` method implemented
- [ ] Label-based detection implemented
- [ ] Port-based detection implemented
- [ ] All service detection tests passing

### **3.3 Service Discovery Engine** (TDD RED → GREEN)
- [ ] Write failing integration tests for service discovery
- [ ] `DiscoveryService` class implemented
- [ ] `DiscoverServices()` method implemented
- [ ] Kubernetes client integration
- [ ] All service discovery tests passing

---

## ✅ Phase 4: Health Validation (Week 3)

### **4.1 Health Checker** (TDD RED → GREEN)
- [ ] Write failing unit tests for health validation (BR-TOOLSET-003)
- [ ] Test Prometheus health check (`/-/healthy`)
- [ ] Test Grafana health check (`/api/health`)
- [ ] Test Jaeger health check (`/`)
- [ ] Test timeout handling
- [ ] `ServiceHealthChecker` implemented
- [ ] All health validation tests passing

### **4.2 Health Check Integration**
- [ ] Optional credentials loading from secrets
- [ ] HTTP client with timeout configuration
- [ ] Parallel health checks for multiple services
- [ ] Health status caching (5-minute TTL)

---

## ✅ Phase 5: ConfigMap Generation (Week 3-4)

### **5.1 Toolset Generator** (TDD RED → GREEN)
- [ ] Write failing unit tests for ConfigMap generation (BR-TOOLSET-002)
- [ ] Test toolset YAML generation
- [ ] Test unhealthy service exclusion
- [ ] Test manual overrides preservation
- [ ] `ToolsetGenerator` class implemented
- [ ] YAML marshaling implemented
- [ ] All ConfigMap generation tests passing

### **5.2 ConfigMap Writer**
- [ ] ConfigMap creation logic implemented
- [ ] ConfigMap update logic implemented
- [ ] Conflict resolution for concurrent updates
- [ ] ConfigMap validation before write

---

## ✅ Phase 6: Kubernetes Controller (Week 4-5)

### **6.1 ConfigMap Reconciler** (TDD RED → GREEN)
- [ ] Write failing integration tests for reconciliation (BR-TOOLSET-004)
- [ ] Test ConfigMap creation if not exists
- [ ] Test ConfigMap recreation after deletion
- [ ] Test manual overrides preservation
- [ ] `ConfigMapReconciler` implemented
- [ ] Reconciliation loop logic implemented
- [ ] All reconciliation tests passing

### **6.2 Service Watcher**
- [ ] Service watch loop implemented
- [ ] Service add event handling
- [ ] Service delete event handling
- [ ] Service update event handling
- [ ] Debouncing for rapid service changes

### **6.3 Leader Election**
- [ ] Leader election configured
- [ ] Only leader performs reconciliation
- [ ] Leader failover tested
- [ ] Lease renewal logic

---

## ✅ Phase 7: HTTP API Implementation (Week 5)

### **7.1 API Endpoints** (TDD RED → GREEN)
- [ ] Write failing integration tests for API endpoints
- [ ] `GET /api/v1/toolset` implemented
- [ ] `GET /api/v1/services` implemented
- [ ] Request validation
- [ ] Response formatting (200 OK, error responses)
- [ ] All API endpoint tests passing

### **7.2 HTTP Middleware**
- [ ] Authentication middleware applied to all routes
- [ ] Authorization middleware applied to all routes
- [ ] Rate limiting middleware (100 req/min per service)
- [ ] Request logging middleware
- [ ] CORS configuration (if needed)

### **7.3 Error Handling**
- [ ] Structured error responses (JSON format)
- [ ] HTTP status codes (400, 401, 403, 429, 500, 503)
- [ ] Error logging with correlation IDs
- [ ] Graceful degradation for K8s API failures

---

## ✅ Phase 8: Cross-Service Integration (Week 5-6)

### **8.1 Integration with HolmesGPT API**
- [ ] HolmesGPT API can poll ConfigMap for toolset
- [ ] Integration test: HolmesGPT API → Dynamic Toolset
- [ ] Hot-reload verified when ConfigMap changes

### **8.2 Integration with Prometheus**
- [ ] Prometheus service discovery working
- [ ] Health check validated
- [ ] Endpoint correctly formatted in toolset

### **8.3 Integration with Grafana**
- [ ] Grafana service discovery working
- [ ] Health check validated
- [ ] Endpoint correctly formatted in toolset

---

## ✅ Phase 9: Observability (Week 6)

### **9.1 Logging** (Zap - Controller-Runtime)
- [ ] Zap logger initialized with `ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))`
- [ ] Structured logging for all discovery operations
- [ ] Security event logging (authentication, authorization)
- [ ] ConfigMap reconciliation logging
- [ ] Service watch event logging

### **9.2 Metrics** (Prometheus)
- [ ] Metrics server on port 9090
- [ ] `dynamictoolset_services_discovered_total` counter (by type)
- [ ] `dynamictoolset_service_health_checks_total` counter (by type, status)
- [ ] `dynamictoolset_configmap_reconciliations_total` counter (by status)
- [ ] `dynamictoolset_discovery_duration_seconds` histogram
- [ ] Metrics endpoint secured with TokenReviewer

### **9.3 Health Checks**
- [ ] `/healthz` endpoint (liveness probe)
- [ ] `/readyz` endpoint (readiness probe)
- [ ] Kubernetes API connection check in readiness
- [ ] Leader election status in readiness
- [ ] Health checks on port 8080

### **9.4 Grafana Dashboard**
- [ ] Dashboard created for Dynamic Toolset metrics
- [ ] Panels: Services discovered, health check success rate
- [ ] Panels: ConfigMap reconciliation frequency
- [ ] Alert rules for discovery failures

---

## ✅ Phase 10: Testing & Quality (Week 7)

### **10.1 Unit Tests** (70%+ Coverage)
- [ ] Service detection tests (10+ scenarios)
- [ ] ConfigMap generation tests
- [ ] Health validation tests
- [ ] Unit test coverage ≥70%

### **10.2 Integration Tests** (>50% Coverage)
- [ ] Kubernetes service discovery tests (with envtest)
- [ ] ConfigMap reconciliation tests
- [ ] Cross-service integration tests (HolmesGPT API)
- [ ] Leader election tests
- [ ] Integration test coverage >50%

### **10.3 E2E Tests** (10-15% Coverage)
- [ ] Complete service discovery and toolset generation flow
- [ ] End-to-end with real Kind cluster
- [ ] E2E test coverage 10-15%

### **10.4 Load Testing**
- [ ] Load test: 1000 services discovered
- [ ] Load test: ConfigMap reconciliation under load
- [ ] Performance regression tests

---

## ✅ Phase 11: Deployment (Week 7-8)

### **11.1 Kubernetes Manifests**
- [ ] Deployment manifest with 1-2 replicas (leader election)
- [ ] Service manifest (ClusterIP)
- [ ] ServiceAccount, ClusterRole, ClusterRoleBinding manifests
- [ ] NetworkPolicy manifest
- [ ] PodDisruptionBudget manifest
- [ ] Leader election Lease resource

### **11.2 ConfigMaps & Secrets**
- [ ] ConfigMap for dynamic-toolset configuration
- [ ] (Optional) Secret for service health check credentials
- [ ] Environment-specific configurations (dev, staging, prod)

### **11.3 ServiceMonitor**
- [ ] ServiceMonitor for Prometheus scraping
- [ ] Metrics endpoint configured (port 9090)
- [ ] Label selectors correct

### **11.4 Deployment Validation**
- [ ] Deploy to dev environment
- [ ] Health checks passing
- [ ] Metrics scraped by Prometheus
- [ ] Logs visible in centralized logging
- [ ] Integration tests passing against deployed service
- [ ] Leader election working correctly

---

## ✅ Phase 12: Documentation (Week 8)

### **12.1 API Documentation**
- [ ] OpenAPI 3.0 spec generated
- [ ] API examples for each endpoint
- [ ] Error response documentation
- [ ] Authentication/authorization requirements documented

### **12.2 Operational Documentation**
- [ ] Runbook for common issues
- [ ] Troubleshooting guide
- [ ] Service discovery configuration guide
- [ ] Manual override configuration guide

### **12.3 Architecture Decision Records**
- [ ] ADR: Why automatic service discovery
- [ ] ADR: ConfigMap as storage mechanism
- [ ] ADR: Leader election strategy
- [ ] ADR: Health validation approach

---

## 🎯 Definition of Done

### **Service is production-ready when:**

- ✅ All unit tests passing (≥70% coverage)
- ✅ All integration tests passing (>50% coverage)
- ✅ All E2E tests passing (10-15% coverage)
- ✅ Load tests passing (1000 services discovered)
- ✅ Deployed to staging environment successfully
- ✅ Health checks passing in staging
- ✅ Metrics visible in Prometheus
- ✅ Logs visible in centralized logging
- ✅ Leader election working correctly
- ✅ ConfigMap reconciliation verified
- ✅ Security review completed
- ✅ Documentation complete
- ✅ Operational runbook reviewed

---

## 🚨 Critical Path Items

### **Must be completed before production:**

1. **Authentication**: TokenReviewer authentication implemented and tested
2. **Authorization**: RBAC enforced for all operations
3. **Kubernetes RBAC**: Read-only service discovery, ConfigMap write access
4. **Leader Election**: Multi-replica coordination working
5. **ConfigMap Reconciliation**: Prevents accidental deletion
6. **Monitoring**: Prometheus metrics and Grafana dashboards operational
7. **Testing**: All test suites passing with required coverage

---

## 📊 Progress Tracking

| Phase | Status | Completion Date |
|-------|--------|----------------|
| Phase 1: Core Infrastructure | ⏸️ Not Started | TBD |
| Phase 2: Authentication & Authorization | ⏸️ Not Started | TBD |
| Phase 3: Service Discovery Logic | ⏸️ Not Started | TBD |
| Phase 4: Health Validation | ⏸️ Not Started | TBD |
| Phase 5: ConfigMap Generation | ⏸️ Not Started | TBD |
| Phase 6: Kubernetes Controller | ⏸️ Not Started | TBD |
| Phase 7: HTTP API Implementation | ⏸️ Not Started | TBD |
| Phase 8: Cross-Service Integration | ⏸️ Not Started | TBD |
| Phase 9: Observability | ⏸️ Not Started | TBD |
| Phase 10: Testing & Quality | ⏸️ Not Started | TBD |
| Phase 11: Deployment | ⏸️ Not Started | TBD |
| Phase 12: Documentation | ⏸️ Not Started | TBD |

**Overall Progress**: 0% (Design phase complete, implementation pending)

---

## 🔗 Reference Documentation

- **Overview**: `docs/services/stateless/dynamic-toolset/overview.md`
- **API Specification**: `docs/services/stateless/dynamic-toolset/api-specification.md`
- **Testing Strategy**: `docs/services/stateless/dynamic-toolset/testing-strategy.md`
- **Security Configuration**: `docs/services/stateless/dynamic-toolset/security-configuration.md`
- **Integration Points**: `docs/services/stateless/dynamic-toolset/integration-points.md`
- **APDC Methodology**: `.cursor/rules/00-core-development-methodology.mdc`
- **Testing Standards**: `.cursor/rules/03-testing-strategy.mdc`

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Implementation Status**: ⏸️ **Pending** (Design phase complete)
**Language**: Go 1.21+
**Framework**: controller-runtime + HTTP API

