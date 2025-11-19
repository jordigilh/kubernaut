# Dynamic Toolset Service - Implementation Checklist

**Version**: v1.2
**Last Updated**: November 10, 2025
**Service Type**: Stateless HTTP API (V1.0) → CRD Controller (V1.1+)
**Port**: 8080 (HTTP + Health), 9090 (Metrics)
**Status**: 🚨 **CRITICAL GAPS IDENTIFIED** - ~30% complete

---

## 📝 Changelog

### Version 1.2 (November 10, 2025)
**Major Update**: Integrated gap analysis and implementation guidelines from Context API and Gateway services

**Added**:
- ✅ **Current State Assessment** with implementation status table
- ✅ **Implementation Guidelines** (Do's and Don'Ts) integrated into each phase
- ✅ **Critical Gaps** section (P0, P1, P2 priorities)
- ✅ **Edge Case Requirements** for each component
- ✅ **Behavior Testing Examples** for validation
- ✅ **Anti-Pattern Prevention** checklist

**Changed**:
- 🔄 Restructured phases by priority (P0 - IMMEDIATE, P1 - HIGH, P2 - FUTURE)
- 🔄 Updated V1.0 scope (REST API deprecated per DD-TOOLSET-001)
- 🔄 Removed authentication middleware (not required per ADR-036)
- 🔄 Simplified reconciliation (callback pattern for V1.0, CRD controller for V1.1)

**Fixed**:
- ✅ Added missing ConfigMap integration steps
- ✅ Corrected testing strategy (defense-in-depth, not pyramid)
- ✅ Updated phase estimates based on actual implementation

### Version 1.0 (October 6, 2025)
**Initial Release**: Original implementation checklist

---

## 🚨 Current State Assessment

### **Implementation Status** (as of November 10, 2025)

| Component | Documented (Plan) | Implemented (Code) | % Complete | Status |
|---|---|---|---|---|
| **Service Discovery** | 275 lines | ~200 lines | 70% | ✅ Core logic exists |
| **Toolset Generation** | 100 lines | ~60 lines | 60% | ✅ Exists, different structure |
| **ConfigMap Builder** | 60 lines | ~40 lines | 70% | ✅ Exists, not wired |
| **ConfigMap Integration** | Required | **MISSING** | 0% | ❌ **P0 - CRITICAL** |
| **HTTP Server** | 160 lines | ~100 lines | 60% | ✅ Basic server |
| **Graceful Shutdown** | Required | Implemented | 100% | ✅ DD-007 compliant |
| **Unit Tests** | 70%+ | 70%+ | 100% | ✅ Passing |
| **Integration Tests** | >50% | ~30% | 60% | ⚠️ Missing business logic |
| **E2E Tests** | <10% | 0% | 0% | ❌ 0/13 passing |
| **Overall** | ~1500 lines | ~400 lines | **~30%** | 🚨 **Incomplete** |

### **V1.0 Scope** (Current Target)

**✅ In Scope**:
1. Service Discovery (with `kubernaut.io/toolset` annotations)
2. Health Validation (5-second timeout per service)
3. Toolset Generation (HolmesGPT-compatible JSON)
4. ConfigMap Builder (build ConfigMap from JSON)
5. ConfigMap Integration (create/update ConfigMap) - **MISSING - P0**
6. HTTP Server (health, readiness, metrics endpoints)
7. Graceful Shutdown (DD-007 compliant)

**❌ Out of Scope** (Deferred to V1.1+):
1. REST API Endpoints (deprecated per DD-TOOLSET-001)
2. Authentication Middleware (not required per ADR-036)
3. Dedicated Reconciliation Controller (simplified to callback for V1.0)
4. Leader Election (single replica for V1.0)
5. ToolsetConfig CRD (BR-TOOLSET-044 - V1.1)

---

## 📋 Implementation Guidelines (MANDATORY)

### **🎯 Core Principles** (from Context API and Gateway)

#### **✅ DO's**
1. **Discover Services Periodically**: 5-minute interval (configurable)
2. **Validate Annotations**: Require `kubernaut.io/toolset: "enabled"` and `kubernaut.io/toolset-type`
3. **Health Check with Timeout**: 5-second timeout per service, fail gracefully
4. **Generate ConfigMap Atomically**: Build entire ConfigMap before updating
5. **Preserve Manual Overrides**: Merge manual ConfigMap changes with discovered services
6. **Log Discovery Events**: Structured logging for service add/remove/update
7. **Use Callback Pattern**: Decouple discovery from ConfigMap generation
8. **Parallel Health Checks**: Use goroutines for concurrent health validation
9. **Retry ConfigMap Updates**: Exponential backoff for conflict resolution (3 attempts)
10. **Test Behavior, Not Implementation**: Focus on business outcomes in tests

#### **❌ DON'Ts**
1. **Don't Block Discovery Loop**: Use goroutines for health checks (parallel)
2. **Don't Fail on Single Service**: Continue discovery if one service health check fails
3. **Don't Update ConfigMap on Every Discovery**: Only update if services changed
4. **Don't Cache Health Status Forever**: Re-check health on every discovery cycle
5. **Don't Ignore ConfigMap Update Conflicts**: Retry with exponential backoff
6. **Don't Skip Validation**: Validate service annotations before including in toolset
7. **Don't Hardcode ConfigMap Name/Namespace**: Use configuration
8. **Don't Test Implementation Details**: Test business outcomes, not internal logic
9. **Don't Create New Components in REFACTOR**: Only enhance existing code
10. **Don't Skip Integration in GREEN Phase**: Wire components to main app immediately

### **🧪 Testing Requirements** (Defense-in-Depth)

#### **Unit Tests** (70%+ Coverage)
- **Focus**: Real business logic with external mocks only
- **Coverage**: Service detection, health checks, toolset generation, ConfigMap building
- **Edge Cases**: Malformed annotations, health timeouts, empty results
- **Validation**: Test business behavior (e.g., "only healthy services included"), not implementation

#### **Integration Tests** (>50% Coverage)
- **Focus**: Component interactions requiring infrastructure (microservices coordination)
- **Coverage**: Discovery → ConfigMap flow, ConfigMap updates, conflict resolution
- **Edge Cases**: Concurrent updates, large service counts (1000+), discovery failures
- **Validation**: Test end-to-end business flow with real Kubernetes client (fake or envtest)
- **Rationale**: Service discovery patterns and ConfigMap synchronization require real K8s API testing

#### **E2E Tests** (<10% Coverage)
- **Focus**: Critical user journeys in production-like environment
- **Coverage**: Full discovery lifecycle, service add/delete/update, ConfigMap synchronization
- **Edge Cases**: Kind cluster with real services, annotation changes, health failures
- **Validation**: Test complete system behavior in realistic environment

---

## 📚 Reference Documentation

**CRITICAL**: Read these documents before starting implementation:

### **Core Methodology**
- [00-core-development-methodology.mdc](../../.cursor/rules/00-core-development-methodology.mdc) - APDC-Enhanced TDD
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Defense-in-Depth testing

### **Service-Specific Documentation**
- [testing-strategy.md](./testing-strategy.md) - Dynamic Toolset testing approach
- [integration-points.md](./integration-points.md) - K8s Service watch, ConfigMap reconciliation
- [implementation.md](./implementation.md) - Detailed technical specification

### **Gap Analysis and Lessons Learned**
- [IMPLEMENTATION_PLAN_V1.1_UPDATED.md](./IMPLEMENTATION_PLAN_V1.1_UPDATED.md) - Comprehensive gap analysis with lessons from Context API and Gateway
- [E2E_FAILURE_ROOT_CAUSE_ANALYSIS.md](../../../test/e2e/toolset/E2E_FAILURE_ROOT_CAUSE_ANALYSIS.md) - Root cause of E2E test failures
- [IMPLEMENTATION_GAP_ANALYSIS.md](../../../test/e2e/toolset/IMPLEMENTATION_GAP_ANALYSIS.md) - Detailed gap comparison

### **Business Requirements**
- BR-TOOLSET-001 through BR-TOOLSET-043 (V1.0 scope)
- BR-TOOLSET-044 (ToolsetConfig CRD - V1.1)

### **Architecture Decisions**
- DD-TOOLSET-001: REST API Deprecation
- ADR-036: Authentication and Authorization Strategy (no auth middleware required)
- DD-007: Graceful Shutdown Pattern

---

## 🔧 Phase 1: Critical Gap Closure (P0 - IMMEDIATE)

**Priority**: P0 - BLOCKING
**Estimated Effort**: 8-12 hours
**Target**: Fix E2E test failures and complete core integration

---

### **1.1 ConfigMap Integration** (4-6 hours)

**Status**: ✅ **IN PROGRESS** (2025-11-10)
**Confidence**: 95%

#### **Implementation Steps**
- [x] Add `ServiceDiscoveryCallback` to `ServiceDiscoverer` interface
- [x] Implement `reconcileConfigMap()` method in `server.go`
- [x] Wire callback in `NewServer()` (add `s.discoverer.SetCallback(s.reconcileConfigMap)`)
- [ ] Add unit tests for `reconcileConfigMap()`
- [ ] Run unit tests to verify changes
- [ ] Run E2E tests to verify fix (target: 13/13 passing)

#### **Implementation Guidelines** (MANDATORY)
**✅ DO**:
- Use callback pattern to decouple discovery from ConfigMap generation
- Generate toolset JSON atomically before updating ConfigMap
- Retry ConfigMap updates with exponential backoff (3 attempts, 100ms base delay)
- Log all ConfigMap operations with structured fields (name, namespace, service_count)
- Compare existing ConfigMap data before updating (skip if unchanged)

**❌ DON'T**:
- Don't block discovery loop on ConfigMap updates
- Don't fail entire reconciliation if ConfigMap update fails once
- Don't update ConfigMap without checking if data changed
- Don't ignore ConfigMap update conflicts (retry with backoff)

#### **Edge Cases to Handle**
1. **ConfigMap Already Exists**: Handle `AlreadyExists` error during creation
2. **ConfigMap Update Conflict**: Retry with exponential backoff (3 attempts)
3. **ConfigMap Not Found**: Create new ConfigMap
4. **Kubernetes API Failure**: Log error and continue (don't crash service)
5. **Empty Services List**: Create ConfigMap with empty toolset (valid state)

#### **Validation Checklist**
- [ ] Unit test: `reconcileConfigMap` creates ConfigMap when not exists
- [ ] Unit test: `reconcileConfigMap` updates ConfigMap when exists
- [ ] Unit test: `reconcileConfigMap` handles update conflicts with retry
- [ ] Unit test: `reconcileConfigMap` skips update if data unchanged
- [ ] Unit test: `reconcileConfigMap` logs all operations
- [ ] E2E test: Discovery → ConfigMap creation flow works end-to-end
- [ ] E2E test: ConfigMap updates when services change
- [ ] E2E test: ConfigMap updates when services deleted

---

### **1.2 Integration Tests for Business Logic** (4-6 hours)

**Status**: ⏳ **PENDING**
**Confidence**: 90%

#### **Test Files to Create**
1. `test/integration/toolset/discovery_configmap_integration_test.go`

#### **Test Coverage Required**

See [IMPLEMENTATION_PLAN_V1.1_UPDATED.md](./IMPLEMENTATION_PLAN_V1.1_UPDATED.md) Section 5.2 for detailed test examples.

**Key Tests**:
1. Discovery → ConfigMap Creation (happy path)
2. ConfigMap Updates on Service Changes
3. ConfigMap Updates on Service Deletion
4. ConfigMap Update Conflict Resolution
5. Parallel Health Checks Performance

#### **Validation Checklist**
- [ ] Test: Discovery → ConfigMap creation (happy path)
- [ ] Test: ConfigMap updates on service add
- [ ] Test: ConfigMap updates on service delete
- [ ] Test: ConfigMap update conflict resolution (retry logic)
- [ ] Test: Parallel health checks performance (< 10s for 50 services)
- [ ] Test: Malformed service annotations (graceful skip)
- [ ] Test: Empty services list (valid ConfigMap with empty toolset)
- [ ] All integration tests pass with fake Kubernetes client

---

## ✅ Phase 1: Core Infrastructure (Week 1) - DEPRECATED

**NOTE**: This phase is from the original V1.0 plan. Most of this is now out of scope or already implemented. See Phase 1 (P0 - IMMEDIATE) above for current priorities.

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

