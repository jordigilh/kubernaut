# Notification Service (NT) - E2E Test Coverage Plan for V1.0

**Date**: December 22, 2025
**Status**: 📋 **READY FOR REVIEW**
**Priority**: P1 (V1.0 Production Readiness)
**Target**: Comprehensive E2E test coverage for all critical user journeys

---

## 🎯 Objective

Expand E2E test coverage to validate all critical Notification service user journeys in a production-like environment (Kind cluster + real infrastructure).

**Current E2E Coverage**: 5 test files (<10% BR coverage - appropriate for E2E tier)
**Target E2E Coverage**: 8-10 test files (~15% BR coverage - comprehensive critical path validation)

---

## 📊 Current E2E Test Coverage

### ✅ **Existing E2E Tests** (5 files)

| Test File | BRs Validated | Status | LOC |
|---|---|---|---|
| `01_notification_lifecycle_audit_test.go` | BR-NOT-062, 063, 064 | ✅ Passing | 230 |
| `02_audit_correlation_test.go` | BR-NOT-064 (correlation) | ✅ Passing | 194 |
| `03_file_delivery_validation_test.go` | BR-NOT-053, 054, 056 | ✅ Passing | 280 |
| `04_failed_delivery_audit_test.go` | BR-NOT-062, 063 (failure path) | ✅ Passing | 220 |
| `04_metrics_validation_test.go` | BR-NOT-060 (observability) | ✅ Passing | 366 |

**Total**: 5 files, ~1,290 LOC

### 📈 **BR Coverage Matrix**

| BR Category | Unit Tests | Integration Tests | E2E Tests | Gap |
|---|---|---|---|---|
| **Data Integrity** | ✅ 100% | ✅ 90% | ✅ 80% | ✓ Sufficient |
| **Audit Trail** | ✅ 100% | ✅ 100% | ✅ 90% | ✓ Sufficient |
| **Retry & Recovery** | ✅ 95% | ✅ 85% | ❌ 30% | ⚠️ **GAP** |
| **Channel Delivery** | ✅ 90% | ✅ 80% | ❌ 40% | ⚠️ **GAP** |
| **Graceful Degradation** | ✅ 90% | ✅ 85% | ❌ 50% | ⚠️ **GAP** |
| **Routing & Priority** | ✅ 100% | ✅ 90% | ❌ 60% | ⚠️ **GAP** |
| **Observability** | ✅ 95% | ✅ 90% | ✅ 75% | ✓ Sufficient |

**Overall E2E BR Coverage**: ~60% (Target: ~80% for V1.0)

---

## 🚨 Identified E2E Test Gaps

### **Gap 1: Retry and Circuit Breaker E2E Validation** (P0 - CRITICAL)

**Missing Coverage**:
- [ ] End-to-end exponential backoff validation (30s → 480s)
- [ ] Circuit breaker state transitions in real cluster (closed → open → half-open)
- [ ] Channel isolation (console continues when Slack circuit opens)
- [ ] Transient vs permanent error classification with real Slack API

**Business Impact**: Without E2E validation, retry logic failures could cause:
- Duplicate notifications (backoff too short)
- Stuck notifications (circuit breaker never recovers)
- Cascade failures (circuit breaker isolation fails)

**BRs at Risk**: BR-NOT-052 (Automatic Retry), BR-NOT-055 (Graceful Degradation)

---

### **Gap 2: Multi-Channel Delivery E2E Validation** (P0 - CRITICAL)

**Missing Coverage**:
- [ ] Slack webhook E2E with real Slack API
- [ ] Console delivery E2E validation
- [ ] Multi-channel fanout (notification to console + slack simultaneously)
- [ ] Channel priority validation (high-priority to PagerDuty, low to console)

**Business Impact**: Without E2E validation, channel delivery failures could cause:
- Silent delivery failures (no real Slack validation)
- Priority routing failures (high-priority to wrong channel)
- Fanout failures (partial delivery not detected)

**BRs at Risk**: BR-NOT-053 (At-Least-Once Delivery), BR-NOT-056 (Priority Handling), BR-NOT-068 (Multi-Channel Fanout)

---

### **Gap 3: Spec-Field-Based Routing E2E Validation** (P1 - HIGH)

**Missing Coverage**:
- [ ] ConfigMap hot-reload without controller restart
- [ ] Spec-field-based routing rule evaluation (e.g., `severity: critical` → PagerDuty)
- [ ] Route conflict resolution (multiple matching rules)
- [ ] Invalid routing config graceful degradation

**Business Impact**: Without E2E validation, routing failures could cause:
- Critical alerts to wrong channel (severity routing broken)
- Missed notifications (hot-reload fails silently)
- Configuration errors not detected until production

**BRs at Risk**: BR-NOT-065 (Channel Routing), BR-NOT-066 (Config Format), BR-NOT-067 (Hot-Reload)

---

### **Gap 4: Data Sanitization E2E Validation** (P1 - HIGH)

**Missing Coverage**:
- [ ] 22 secret patterns redacted in real Slack messages
- [ ] Sanitization applied before audit trail
- [ ] Large payloads (>10KB) sanitized without timeout

**Business Impact**: Without E2E validation, sanitization failures could cause:
- Secret leakage to Slack channels
- Audit trails containing sensitive data
- Compliance violations (PCI, HIPAA, SOC2)

**BRs at Risk**: BR-NOT-054 (Data Sanitization), BR-NOT-058 (Security)

---

### **Gap 5: Performance and Scalability E2E Validation** (P2 - MEDIUM)

**Missing Coverage**:
- [ ] Concurrent notification delivery (100+ notifications/minute)
- [ ] Large notification payloads (10KB+)
- [ ] Controller resource limits under load
- [ ] Delivery latency P99 validation (<10s)

**Business Impact**: Without E2E validation, performance issues could cause:
- Notification backlog during incident spikes
- OOM kills under high load
- Excessive delivery latency (>30s)

**BRs at Risk**: BR-NOT-053 (At-Least-Once), BR-NOT-060 (Observability)

---

## 📋 Proposed E2E Test Suite Expansion

### **Phase 1: Critical Path Validation** (P0 - Blocking V1.0)

#### **Test 1: Retry and Circuit Breaker E2E** (NEW)
**File**: `05_retry_circuit_breaker_e2e_test.go`
**Priority**: P0 (CRITICAL)
**Estimated LOC**: ~350 lines
**Estimated Time**: 2-3 days

**Test Scenarios**:
1. **Exponential Backoff Validation**:
   - Create NotificationRequest with invalid Slack webhook
   - Verify retry attempts at 30s, 60s, 120s, 240s intervals (use mocked Slack server with timestamps)
   - Verify max 5 attempts before marking failed

2. **Circuit Breaker State Transitions**:
   - Simulate 5+ Slack failures (circuit opens)
   - Verify circuit open for 60s (no requests sent)
   - Verify half-open state (1 test request)
   - Verify circuit closes on success

3. **Channel Isolation**:
   - Create NotificationRequest with console + slack channels
   - Simulate Slack circuit open
   - Verify console delivery continues (not blocked by Slack circuit)

**Success Criteria**:
- ✅ Retry intervals match specification (30s → 480s)
- ✅ Circuit breaker state transitions validated in real cluster
- ✅ Channel isolation prevents cascade failures
- ✅ CRD status reflects retry attempts and circuit breaker state

---

#### **Test 2: Slack Webhook E2E Validation** (NEW)
**File**: `06_slack_webhook_e2e_test.go`
**Priority**: P0 (CRITICAL)
**Estimated LOC**: ~300 lines
**Estimated Time**: 1-2 days

**Test Scenarios**:
1. **Real Slack Webhook Delivery**:
   - Use test Slack workspace webhook (env var: `E2E_SLACK_WEBHOOK_URL`)
   - Create NotificationRequest with subject + body
   - Verify message delivered to Slack (manual verification or Slack API query)

2. **Slack Block Formatting**:
   - Create NotificationRequest with priority: critical
   - Verify Slack blocks formatted correctly (header, divider, fields)
   - Verify priority badge color (critical=red, high=orange, medium=yellow, low=gray)

3. **Slack Rate Limiting**:
   - Send 10 notifications rapidly
   - Verify rate limiting does not cause failures (429 errors handled)

**Success Criteria**:
- ✅ Real Slack message delivered and visible in channel
- ✅ Slack blocks formatted per specification
- ✅ Rate limiting handled gracefully (retry after 429)
- ✅ Audit trail contains Slack delivery attempt

**Note**: Requires test Slack workspace + webhook for CI/CD. Optional for local development (can skip if webhook not configured).

---

#### **Test 3: Multi-Channel Fanout E2E** (NEW)
**File**: `07_multi_channel_fanout_e2e_test.go`
**Priority**: P0 (CRITICAL)
**Estimated LOC**: ~250 lines
**Estimated Time**: 1 day

**Test Scenarios**:
1. **Console + Slack Fanout**:
   - Create NotificationRequest with channels: [console, slack]
   - Verify both deliveries attempted
   - Verify CRD status shows 2 delivery attempts

2. **Partial Failure Handling**:
   - Create NotificationRequest with console + invalid-slack
   - Verify console succeeds, Slack fails
   - Verify phase: PartiallySent (not Sent)

3. **Priority-Based Channel Selection**:
   - Create NotificationRequest with priority: critical (routing to console + slack)
   - Create NotificationRequest with priority: low (routing to console only)
   - Verify routing rules applied correctly

**Success Criteria**:
- ✅ Multi-channel fanout delivers to all channels
- ✅ Partial failures set phase to PartiallySent
- ✅ Priority-based routing selects correct channels
- ✅ Audit trail contains all delivery attempts

---

### **Phase 2: Advanced Validation** (P1 - Post-V1.0)

#### **Test 4: Spec-Field-Based Routing E2E** (NEW)
**File**: `08_label_routing_e2e_test.go`
**Priority**: P1 (HIGH)
**Estimated LOC**: ~400 lines
**Estimated Time**: 2 days

**Test Scenarios**:
1. **ConfigMap Hot-Reload**:
   - Deploy controller with initial routing config
   - Create NotificationRequest (verify initial routing)
   - Update ConfigMap with new routing rules
   - Wait 30s (ConfigMap sync interval)
   - Create NotificationRequest (verify new routing)
   - Verify no controller restart

2. **Severity-Based Routing**:
   - ConfigMap: `severity: critical` → slack + pagerduty
   - ConfigMap: `severity: low` → console
   - Create NotificationRequests with both severities
   - Verify correct channels selected

3. **Invalid Routing Config**:
   - Update ConfigMap with invalid YAML
   - Verify controller logs error but continues processing
   - Verify default routing (all to console) used as fallback

**Success Criteria**:
- ✅ ConfigMap hot-reload without restart
- ✅ Routing rules evaluated correctly
- ✅ Invalid config does not crash controller
- ✅ Default routing used as fallback

---

#### **Test 5: Data Sanitization E2E** (NEW)
**File**: `09_data_sanitization_e2e_test.go`
**Priority**: P1 (HIGH)
**Estimated LOC**: ~280 lines
**Estimated Time**: 1 day

**Test Scenarios**:
1. **Secret Pattern Redaction**:
   - Create NotificationRequest with body containing AWS keys, passwords, tokens
   - Verify file delivery shows redacted content (`[REDACTED-AWS-ACCESS-KEY]`)
   - Query audit trail, verify audit contains redacted content

2. **22 Pattern Coverage**:
   - Create NotificationRequests with all 22 secret patterns
   - Verify each pattern redacted correctly

3. **Large Payload Sanitization**:
   - Create NotificationRequest with 10KB body containing secrets
   - Verify sanitization completes without timeout (<500ms)

**Success Criteria**:
- ✅ All 22 secret patterns redacted in delivery
- ✅ Audit trail contains sanitized content
- ✅ Large payloads sanitized without timeout
- ✅ No secret leakage to Slack or audit trail

---

### **Phase 3: Performance and Scalability** (P2 - Future)

#### **Test 6: Performance and Concurrency E2E** (FUTURE)
**File**: `10_performance_concurrency_e2e_test.go`
**Priority**: P2 (MEDIUM)
**Estimated LOC**: ~350 lines
**Estimated Time**: 2-3 days

**Test Scenarios**:
1. **Concurrent Notification Delivery**:
   - Create 100 NotificationRequests simultaneously
   - Verify all processed within 60s
   - Verify no resource exhaustion (OOM, CPU throttling)

2. **Delivery Latency P99**:
   - Create 50 NotificationRequests
   - Measure delivery latency (creation → phase: Sent)
   - Verify P99 latency <10s

3. **Resource Limits Under Load**:
   - Set controller CPU limit: 200m, memory: 128Mi
   - Create 200 NotificationRequests
   - Verify no OOM kills or CPU throttling

**Success Criteria**:
- ✅ 100 notifications processed in <60s
- ✅ P99 latency <10s
- ✅ No OOM kills under load
- ✅ Controller stable with resource limits

---

## 📊 E2E Test Suite Summary

| Phase | Test File | Priority | LOC | Time | Status |
|---|---|---|---|---|---|
| **Phase 1** | `05_retry_circuit_breaker_e2e_test.go` | P0 | 350 | 2-3 days | ⏸️ Pending |
| **Phase 1** | `06_slack_webhook_e2e_test.go` | P0 | 300 | 1-2 days | ⏸️ Pending |
| **Phase 1** | `07_multi_channel_fanout_e2e_test.go` | P0 | 250 | 1 day | ⏸️ Pending |
| **Phase 2** | `08_label_routing_e2e_test.go` | P1 | 400 | 2 days | ⏸️ Pending |
| **Phase 2** | `09_data_sanitization_e2e_test.go` | P1 | 280 | 1 day | ⏸️ Pending |
| **Phase 3** | `10_performance_concurrency_e2e_test.go` | P2 | 350 | 2-3 days | 🔮 Future |
| **TOTAL** | **6 new tests** | - | **1,930 LOC** | **9-11 days** | - |

**Combined with Existing**: 11 E2E test files, ~3,220 LOC total

---

## ✅ Success Criteria for V1.0

### **Phase 1 (Blocking V1.0)**:
- [ ] **Test 1**: Retry and circuit breaker validated in real cluster
- [ ] **Test 2**: Real Slack webhook delivery E2E
- [ ] **Test 3**: Multi-channel fanout with partial failure handling

**Estimated Time**: 4-6 days
**V1.0 Impact**: ✅ CRITICAL - These tests validate core delivery guarantees

### **Phase 2 (Post-V1.0)**:
- [ ] **Test 4**: Spec-field-based routing with hot-reload
- [ ] **Test 5**: Data sanitization E2E validation

**Estimated Time**: 3 days
**V1.0 Impact**: ⚠️ HIGH - Recommended for V1.0 but not blocking

### **Phase 3 (Future)**:
- [ ] **Test 6**: Performance and concurrency validation

**Estimated Time**: 2-3 days
**V1.0 Impact**: ℹ️ MEDIUM - Baseline for future scaling decisions

---

## 🛠️ Implementation Considerations

### **CI/CD Requirements**:
1. **Slack Webhook Secret**:
   - Required for Test 2 (Slack webhook E2E)
   - Environment variable: `E2E_SLACK_WEBHOOK_URL`
   - Optional: Skip test if not configured (local development)
   - CI/CD: Use test Slack workspace webhook

2. **Kind Cluster Resources**:
   - Current: 2 nodes (1 control-plane + 1 worker)
   - Recommended for Phase 1-2: Same (sufficient)
   - Required for Phase 3: 3-4 nodes (concurrency testing)

3. **Test Execution Time**:
   - Current: ~5 minutes (5 tests)
   - Phase 1: ~15 minutes (8 tests - retry delays)
   - Phase 2: ~20 minutes (10 tests - hot-reload delays)
   - Phase 3: ~30 minutes (11 tests - load testing)

### **Test Isolation**:
- Each test creates unique NotificationRequest (namespace + name)
- Retry tests use mocked Slack server (controlled timing)
- Circuit breaker tests use separate test ConfigMaps
- No global state shared between tests

### **Maintenance Burden**:
- E2E tests are more brittle than unit/integration tests
- External dependencies (Slack API) can cause flakiness
- Recommended: Make Slack webhook tests optional (skip if not configured)
- Recommended: Monitor E2E test failure rate (target <5%)

---

## 📚 References

### Authoritative Documents
- `docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md` - 18 BRs with acceptance criteria
- `docs/services/crd-controllers/06-notification/testing-strategy.md` - Defense-in-depth testing pyramid
- `.cursor/rules/03-testing-strategy.mdc` - E2E test requirements (<10% BR coverage)

### Current E2E Tests
- `test/e2e/notification/01_notification_lifecycle_audit_test.go` - Audit lifecycle
- `test/e2e/notification/02_audit_correlation_test.go` - Audit correlation
- `test/e2e/notification/03_file_delivery_validation_test.go` - File delivery
- `test/e2e/notification/04_failed_delivery_audit_test.go` - Failed delivery audit
- `test/e2e/notification/04_metrics_validation_test.go` - Metrics exposure

---

## 🎯 Recommendation

### **For V1.0 Production Readiness**:
**Execute Phase 1** (P0 tests) - ~4-6 days
- Critical path validation (retry, Slack, fanout)
- Confidence: 95% → 99% for production deployment
- Risk mitigation: Retry logic failures, Slack integration issues

### **Post-V1.0 Enhancements**:
**Execute Phase 2** (P1 tests) - ~3 days
- Advanced feature validation (routing, sanitization)
- Confidence: 99% → 99.5% for enterprise deployments

### **Future Work**:
**Execute Phase 3** (P2 tests) - ~2-3 days
- Performance baseline and scaling decisions
- Not blocking for V1.0

---

**Status**: 📋 **READY FOR REVIEW**
**Next Steps**: Review with team, prioritize Phase 1 tests, estimate capacity
**Owner**: NT Team
**Estimated Total Time**: 9-11 days (all phases)

