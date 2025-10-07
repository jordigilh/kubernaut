# Gateway Service - Implementation Checklist

**Version**: v1.0
**Last Updated**: October 4, 2025
**Status**: ✅ Design Complete

---

## APDC-TDD Implementation Phases

Following `.cursor/rules/00-core-development-methodology.mdc`:

```
ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK
```

**Total Estimated Effort**: 46-60 hours (6-8 days for single developer)

---

## Phase 1: ANALYSIS & Foundation (Day 1 - 4h)

### ANALYSIS Phase (1h)

- [ ] **Search existing HTTP service patterns**
  ```bash
  codebase_search "HTTP server implementations"
  grep -r "http.Server" pkg/ --include="*.go"
  ```

- [ ] **Review authentication patterns**
  ```bash
  codebase_search "JWT authentication TokenReviewer"
  grep -r "TokenReview" pkg/ --include="*.go"
  ```

- [ ] **Identify reusable Redis patterns**
  ```bash
  codebase_search "Redis client connection pool"
  grep -r "redis.Client" pkg/ --include="*.go"
  ```

- [ ] **Map business requirements**
  - Read BR-GATEWAY-001 to BR-GATEWAY-023 (webhook handling)
  - Read BR-GATEWAY-051 to BR-GATEWAY-053 (environment classification)

### PLAN Phase (1h)

- [ ] **Define TDD strategy**
  - Unit tests: Adapters, deduplication, storm detection (70%+)
  - Integration tests: Redis, CRD creation (>50%)
  - E2E tests: Prometheus → CRD (<10%)

- [ ] **Plan integration points**
  - Main app: `cmd/gateway/main.go`
  - Business logic: `pkg/gateway/`
  - Tests: `test/unit/gateway/`, `test/integration/gateway/`

- [ ] **Establish success criteria**
  - HTTP 202 response < 50ms (p95)
  - 40-60% deduplication rate
  - RemediationRequest CRD created successfully

### DO-DISCOVERY (2h)

- [ ] **Setup project structure**
  ```bash
  mkdir -p cmd/gateway
  mkdir -p pkg/gateway/{adapters,processing}
  mkdir -p test/unit/gateway
  mkdir -p test/integration/gateway
  ```

- [ ] **Create main.go skeleton**
- [ ] **Initialize Go modules and dependencies**

---

## Phase 2: Alert Adapters (Day 1-2 - 8-10h)

### DO-RED: Prometheus Adapter Tests (2h)

- [ ] **Write failing tests** (`test/unit/gateway/prometheus_adapter_test.go`)
  ```go
  Describe("BR-GATEWAY-001: Prometheus Adapter", func() {
      It("should parse valid webhook", func() {
          // Test should fail - adapter not implemented yet
      })
  })
  ```

- [ ] **Run tests - confirm failures**
  ```bash
  go test ./test/unit/gateway/prometheus_adapter_test.go
  ```

### DO-GREEN: Prometheus Adapter Implementation (2-3h)

- [ ] **Implement PrometheusAdapter** (`pkg/gateway/adapters/prometheus_adapter.go`)
- [ ] **Define NormalizedSignal type** (`pkg/gateway/types.go`)
- [ ] **Implement fingerprint generation**
- [ ] **Run tests - confirm pass**

### DO-RED: Kubernetes Events Adapter Tests (2h)

- [ ] **Write failing tests** (`test/unit/gateway/kubernetes_adapter_test.go`)

### DO-GREEN: Kubernetes Events Adapter Implementation (3-4h)

- [ ] **Implement KubernetesEventAdapter**
- [ ] **Implement event watcher**
- [ ] **Map event reasons to alert names**

### DO-REFACTOR: Extract AlertAdapter Interface (1h)

- [ ] **Extract common interface from both adapters**
- [ ] **Refactor adapters to implement interface**
- [ ] **Create adapter factory**

---

## Phase 3: Deduplication & Storm Detection (Day 3 - 10-12h)

### DO-RED: Deduplication Tests (2h)

- [ ] **Write deduplication tests** (`test/unit/gateway/deduplication_test.go`)
- [ ] **Use miniredis for fast in-memory Redis mock**

### DO-GREEN: Deduplication Implementation (3-4h)

- [ ] **Implement DeduplicationService** (`pkg/gateway/processing/deduplication.go`)
- [ ] **Redis schema for fingerprints**
- [ ] **5-minute TTL expiration**

### DO-RED: Storm Detection Tests (2h)

- [ ] **Write storm detection tests** (`test/unit/gateway/storm_detection_test.go`)
- [ ] **Test rate-based and pattern-based detection**

### DO-GREEN: Storm Detection Implementation (3-4h)

- [ ] **Implement StormDetector** (`pkg/gateway/processing/storm_detection.go`)
- [ ] **Rate-based detection (>10 alerts/min)**
- [ ] **Pattern-based detection (>5 similar alerts)**

---

## Phase 4: Classification & Priority (Day 4 - 10-12h)

### DO-RED: Environment Classification Tests (2h)

- [ ] **Write classification tests** (`test/unit/gateway/classification_test.go`)
- [ ] **Test namespace labels, ConfigMap, alert labels**

### DO-GREEN: Environment Classification Implementation (4-5h)

- [ ] **Implement EnvironmentClassifier** (`pkg/gateway/processing/classification.go`)
- [ ] **Namespace label lookup with cache**
- [ ] **ConfigMap override loading**

### DO-RED: Priority Assignment Tests (1h)

- [ ] **Write priority tests** (`test/unit/gateway/priority_test.go`)
- [ ] **Test Rego evaluation + fallback**

### DO-GREEN: Priority Assignment Implementation (3-4h)

- [ ] **Implement PriorityEngine** (`pkg/gateway/processing/priority.go`)
- [ ] **Rego policy evaluation**
- [ ] **Severity+environment fallback matrix**

---

## Phase 5: HTTP Server & Handlers (Day 5 - 8-10h)

### DO-RED: HTTP Handler Tests (2h)

- [ ] **Write handler tests** (`test/unit/gateway/handlers_test.go`)
- [ ] **Mock dependencies (Redis, K8s client)**

### DO-GREEN: HTTP Server Implementation (4-5h)

- [ ] **Implement Server struct** (`pkg/gateway/server.go`)
- [ ] **Implement handlePrometheusWebhook**
- [ ] **Implement handleKubernetesEvent**
- [ ] **Implement processAlert pipeline**

### DO-REFACTOR: Middleware (2-3h)

- [ ] **Authentication middleware (JWT + TokenReviewer)**
- [ ] **Rate limiting middleware (token bucket)**
- [ ] **Request ID correlation middleware**

---

## Phase 6: CRD Integration (Day 6 - 4-6h)

### DO-RED: CRD Creation Tests (1-2h)

- [ ] **Write CRD creation tests** (`test/unit/gateway/crd_test.go`)

### DO-GREEN: CRD Creation Implementation (3-4h)

- [ ] **Implement createRemediationRequestCRD**
- [ ] **Normal alert CRD structure**
- [ ] **Storm alert CRD structure**

---

## Phase 7: Integration Tests (Day 7 - 8-10h)

### Integration Test Suite Setup (2h)

- [ ] **Setup Redis test environment**
- [ ] **Setup KIND cluster for K8s API**
- [ ] **Create test fixtures**

### Integration Test Implementation (6-8h)

- [ ] **Redis integration tests** (`test/integration/gateway/redis_integration_test.go`)
- [ ] **CRD creation tests** (`test/integration/gateway/crd_creation_test.go`)
- [ ] **End-to-end webhook flow** (`test/integration/gateway/webhook_flow_test.go`)

---

## Phase 8: Observability & Security (Day 8 - 4-6h)

### Observability (2-3h)

- [ ] **Implement structured logging**
- [ ] **Implement Prometheus metrics**
- [ ] **Implement OpenTelemetry tracing**
- [ ] **Health and readiness probes**

### Security Configuration (2-3h)

- [ ] **RBAC manifests**
- [ ] **Network policies**
- [ ] **Security context**

---

## CHECK Phase: Validation & Confidence Assessment

### Business Requirement Verification

- [ ] **BR-GATEWAY-001 to BR-GATEWAY-023**: Webhook handling validated
- [ ] **BR-GATEWAY-051 to BR-GATEWAY-053**: Environment classification validated
- [ ] **BR-GATEWAY-071 to BR-GATEWAY-072**: Environment field enables GitOps decisions

### Technical Validation

- [ ] **Build success**: `go build ./cmd/gateway`
- [ ] **Unit tests pass**: `go test ./test/unit/gateway/... -v`
- [ ] **Integration tests pass**: `make test-integration-kind`
- [ ] **Lint compliance**: `golangci-lint run ./...`

### Integration Confirmation

- [ ] **Gateway instantiated in cmd/gateway/main.go**
- [ ] **RemediationRequest CRD created successfully in integration tests**
- [ ] **Redis connection healthy**
- [ ] **K8s API accessible**

### Performance Validation

- [ ] **API latency**: p95 < 50ms, p99 < 100ms
- [ ] **Redis latency**: p95 < 5ms
- [ ] **Deduplication rate**: 40-60% (production-like load)

### Confidence Assessment

**Provide both percentage and justification**:

Example:
```
Confidence Assessment: 85%
Justification: Implementation follows proven HTTP service patterns from existing
codebase. Alert adapters reuse fingerprinting logic from Context API. Redis
deduplication is standard pattern. Risk: Rego policy integration moderate complexity,
but fallback matrix provides safety. Validation: 70% unit test coverage, >50%
integration test coverage achieved.
```

---

## Summary

**Total Estimated Effort**: 46-60 hours (6-8 days)

**Critical Path**:
1. Alert Adapters (Day 1-2)
2. Deduplication & Storm Detection (Day 3)
3. Classification & Priority (Day 4)
4. HTTP Server (Day 5)
5. CRD Integration (Day 6)
6. Integration Tests (Day 7)
7. Observability & Security (Day 8)

**Confidence**: 85% (moderate complexity for Rego and multi-source adapters)

**Next Steps**: Begin ANALYSIS phase, search existing HTTP service patterns
