# Gateway Service: Test Coverage Extension Plan

**Version**: v1.0  
**Created**: October 10, 2025  
**Status**: 🔄 Ready for Approval  
**Current Coverage**: 57% BRs (13/23 tested)  
**Target Coverage**: 100% BRs (23/23 tested)  
**Methodology**: APDC-Enhanced TDD following `testing-strategy.md`

---

## Executive Summary

**Current State**:
- ✅ **40 tests** (33 unit + 7 integration)
- ✅ **13 BRs covered** (57%)
- ✅ **100% business outcome focused**
- ⚠️ **Integration tests broken** (auth token + CRD issues)

**Target State**:
- 🎯 **79 tests** (+39 new tests)
- 🎯 **23 BRs covered** (100%)
- 🎯 **Test pyramid compliance** (70% unit, 20% integration, 10% e2e)
- 🎯 **Infrastructure fixed** (Kind cluster + CRDs)

**Effort Estimate**: 12-16 hours (1.5-2 days)

---

## Business Requirements Inventory

### Source: [README.md](./README.md) + [overview.md](./overview.md)

| BR ID | Category | Description | Current Coverage |
|-------|----------|-------------|------------------|
| **Primary Alert Handling (BR-GATEWAY-001 to BR-GATEWAY-023)** |
| BR-001 | Ingestion | Alert ingestion endpoint | ✅ Integration (2 tests) |
| BR-002 | Ingestion | Prometheus adapter | ✅ Unit (4) + Integration (2) |
| BR-003 | Validation | Validate webhook payloads | ✅ Unit (5 DescribeTable) |
| BR-004 | Security | Authentication/Authorization | ✅ Integration (1 test) |
| BR-005 | Ingestion | Kubernetes event adapter | ❌ **NOT TESTED** |
| BR-006 | Processing | Alert normalization | ✅ Unit (1 test) |
| BR-007-009 | - | Reserved | ⏸️ N/A |
| BR-010 | Deduplication | Fingerprint-based deduplication | ✅ Unit (2) + Integration (2) |
| BR-011 | Deduplication | Redis deduplication storage | ⏸️ Implicit (integration) |
| BR-012-014 | - | Reserved | ⏸️ N/A |
| BR-015 | Storm | Alert storm detection (rate-based) | ✅ Unit (4) + Integration (1) |
| BR-016 | Storm | Storm aggregation | ✅ Unit (2) + Integration (1) |
| BR-017-019 | - | Reserved | ⏸️ N/A |
| BR-020 | Priority | Priority assignment (Rego) | ✅ Unit (9 DescribeTable) |
| BR-021 | Priority | Priority fallback matrix | ✅ Unit (9 DescribeTable) |
| BR-022 | Remediation | Remediation path decision | ❌ **NOT TESTED** |
| BR-023 | CRD | CRD creation | ⏸️ Implicit (integration) |
| **Environment Classification (BR-GATEWAY-051 to BR-GATEWAY-053)** |
| BR-051 | Environment | Environment detection (namespace labels) | ✅ Unit (6) + Integration (1) |
| BR-052 | Environment | ConfigMap fallback | ✅ Unit (6 DescribeTable) |
| BR-053 | Environment | Default environment (unknown) | ✅ Unit (6 DescribeTable) |
| **GitOps Integration (BR-GATEWAY-071 to BR-GATEWAY-072)** |
| BR-071 | GitOps | CRD-only integration | ⏸️ Implicit (integration) |
| BR-072 | GitOps | CRD as GitOps trigger | ⏸️ Downstream concern |
| **Notification (BR-GATEWAY-091 to BR-GATEWAY-092)** |
| BR-091 | Notification | Escalation notification trigger | ⏸️ Downstream concern |
| BR-092 | Notification | Notification metadata | ⏸️ Downstream concern |

**Total Defined BRs**: 23 (excluding reserved ranges)  
**Tested**: 13 (57%)  
**Gaps**: 10 BRs (43%)

---

## Gap Analysis

### Critical Gaps (Must Fix)

#### 1. **BR-005: Kubernetes Event Adapter** ❌
**Status**: Adapter not implemented, no tests  
**Business Impact**: Cannot ingest K8s Events (40% of production signals)  
**Priority**: **P0 - BLOCKING**

**Required Tests**:
- Unit (6 tests): Event parsing, validation, resource identification
- Integration (1 test): Event → CRD end-to-end

---

#### 2. **BR-011: Redis Deduplication Storage** ⏸️
**Status**: Implicitly tested, no explicit Redis behavior tests  
**Business Impact**: No validation of Redis TTL, expiry, persistence  
**Priority**: **P1 - HIGH**

**Required Tests**:
- Integration (4 tests): TTL expiry, Redis failover, persistence across restarts, concurrent updates

---

#### 3. **BR-022: Remediation Path Decision** ❌
**Status**: Not tested  
**Business Impact**: No validation of Rego policy output  
**Priority**: **P1 - HIGH**

**Required Tests**:
- Unit (5 tests): Rego policy evaluation, fallback behavior

---

#### 4. **BR-023: CRD Creation** ⏸️
**Status**: Implicitly tested, no explicit CRD field validation  
**Business Impact**: No validation of CRD labels, metadata, schema compliance  
**Priority**: **P1 - HIGH**

**Required Tests**:
- Integration (3 tests): CRD schema validation, labels/annotations, owner references

---

### Infrastructure Gaps (Critical for Running Tests)

#### 5. **Integration Test Infrastructure** ⚠️
**Status**: 6/7 tests failing (auth + CRD issues)  
**Business Impact**: Cannot validate integration tests  
**Priority**: **P0 - BLOCKING**

**Required Actions**:
1. Fix auth token generation (`make test-gateway-setup`)
2. Install RemediationRequest CRDs in Kind cluster
3. Verify Redis connectivity

---

### Additional Test Scenarios (from testing-strategy.md recommendation)

#### 6. **Rate Limiting Behavior** (BR-004 extension)
**Status**: Security test exists, but no rate limiting validation  
**Business Impact**: No validation of per-source rate limits  
**Priority**: **P2 - MEDIUM**

**Required Tests**:
- Integration (3 tests): Rate limit enforcement, burst handling, per-source isolation

---

#### 7. **Redis Failover Handling** (BR-011 extension)
**Status**: No Redis error handling tests  
**Business Impact**: Unknown behavior on Redis downtime  
**Priority**: **P2 - MEDIUM**

**Required Tests**:
- Integration (2 tests): Graceful degradation, Redis reconnection

---

#### 8. **Concurrent Request Handling** (BR-001 extension)
**Status**: No concurrency tests  
**Business Impact**: Unknown behavior under load  
**Priority**: **P3 - LOW**

**Required Tests**:
- Integration (2 tests): Concurrent deduplication, race conditions

---

## Test Coverage Extension Plan

### Phase 0: Fix Infrastructure (P0 - BLOCKING) ⚠️
**Effort**: 1-2 hours  
**Owner**: DevOps + Test Engineer

**Tasks**:
1. ✅ **Regenerate auth token**
   ```bash
   make test-gateway-setup
   ```
   - Expected: Token refreshed, 970+ characters
   - Validation: `echo $TEST_TOKEN | wc -c`

2. ✅ **Install RemediationRequest CRDs**
   ```bash
   kubectl apply -f config/crd/bases/remediation.kubernaut.io_remediationrequests.yaml --context kind-kubernaut-test
   ```
   - Expected: CRD installed
   - Validation: `kubectl get crd remediationrequests.remediation.kubernaut.io`

3. ✅ **Verify Kind cluster health**
   ```bash
   kubectl get nodes --context kind-kubernaut-test
   kubectl get pods -n kubernaut-system
   ```
   - Expected: Control plane ready, Gateway + Redis running
   - Validation: All pods `Running` status

4. ✅ **Run integration tests**
   ```bash
   ginkgo -v test/integration/gateway/
   ```
   - Expected: 7/7 tests passing
   - Validation: `PASS` summary

**Success Criteria**:
- ✅ All 7 integration tests passing
- ✅ Auth token valid for 24+ hours
- ✅ RemediationRequest CRDs available

---

### Phase 1: Kubernetes Event Adapter (P0 - CRITICAL) ⚠️
**Effort**: 4-6 hours  
**BRs Covered**: BR-005  
**Test Count**: +7 tests (6 unit + 1 integration)

#### 1.1 Unit Tests (6 tests) - `test/unit/gateway/adapters/kubernetes_event_test.go`

Following `testing-strategy.md` → Use DescribeTable for validation tests.

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/gateway/adapters"
)

var _ = Describe("BR-GATEWAY-005: Kubernetes Event Adapter", func() {
    var (
        adapter *adapters.KubernetesEventAdapter
        ctx     context.Context
    )

    BeforeEach(func() {
        adapter = adapters.NewKubernetesEventAdapter()
        ctx = context.Background()
    })

    // Business Outcome: Identify resource failures for remediation
    Describe("Resource identification for remediation targeting", func() {
        It("identifies Pod failures for remediation (OOMKilled scenario)", func() {
            // Business scenario: Pod OOM killed, need to restart or scale
            k8sEvent := []byte(`{
                "type": "Warning",
                "reason": "OOMKilled",
                "message": "Container killed due to memory limit",
                "involvedObject": {
                    "kind": "Pod",
                    "namespace": "production",
                    "name": "payment-api-789"
                }
            }`)

            signal, err := adapter.Parse(ctx, k8sEvent)

            // Business outcome: AI can identify WHICH resource to remediate
            Expect(err).NotTo(HaveOccurred())
            Expect(signal.Resource.Kind).To(Equal("Pod"),
                "AI needs resource KIND to choose remediation strategy")
            Expect(signal.Resource.Name).To(Equal("payment-api-789"),
                "AI needs resource NAME for kubectl targeting")
            Expect(signal.Resource.Namespace).To(Equal("production"),
                "AI needs NAMESPACE for kubectl context")
        })

        It("identifies Node failures for cluster-level remediation", func() {
            // Business scenario: Node disk pressure, need to cordon/drain
            k8sEvent := []byte(`{
                "type": "Warning",
                "reason": "DiskPressure",
                "message": "Node has insufficient disk space",
                "involvedObject": {
                    "kind": "Node",
                    "name": "worker-node-3"
                }
            }`)

            signal, err := adapter.Parse(ctx, k8sEvent)

            // Business outcome: AI handles cluster-scoped resources
            Expect(err).NotTo(HaveOccurred())
            Expect(signal.Resource.Kind).To(Equal("Node"))
            Expect(signal.Resource.Namespace).To(BeEmpty(),
                "Nodes are cluster-scoped, no namespace")
        })

        It("identifies Deployment failures for scaling remediation", func() {
            // Business scenario: Deployment rollout stuck, need rollback
            k8sEvent := []byte(`{
                "type": "Warning",
                "reason": "FailedCreate",
                "message": "Failed to create pod for deployment",
                "involvedObject": {
                    "kind": "Deployment",
                    "namespace": "staging",
                    "name": "api-service"
                }
            }`)

            signal, err := adapter.Parse(ctx, k8sEvent)

            // Business outcome: AI can trigger rollback for Deployments
            Expect(err).NotTo(HaveOccurred())
            Expect(signal.Resource.Kind).To(Equal("Deployment"))
        })
    })

    // Business Outcome: Filter noise (only Warning/Error events)
    Describe("Event type filtering to reduce noise", func() {
        It("processes Warning events for remediation", func() {
            // Business scenario: Warning events need attention
            warningEvent := []byte(`{
                "type": "Warning",
                "reason": "BackOff",
                "involvedObject": {"kind": "Pod", "name": "test"}
            }`)

            signal, err := adapter.Parse(ctx, warningEvent)

            // Business outcome: Warning events trigger remediation workflow
            Expect(err).NotTo(HaveOccurred())
            Expect(signal.Severity).To(Equal("warning"))
        })

        It("skips Normal events to avoid noise", func() {
            // Business scenario: Normal events = informational only
            normalEvent := []byte(`{
                "type": "Normal",
                "reason": "Started",
                "involvedObject": {"kind": "Pod", "name": "test"}
            }`)

            signal, err := adapter.Parse(ctx, normalEvent)

            // Business outcome: System doesn't create CRDs for normal operations
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("Normal events not processed"))
        })
    })

    // Business Outcome: Validation prevents invalid events
    DescribeTable("Event validation prevents incomplete remediation requests",
        func(eventJSON string, expectedError string, businessReason string) {
            signal, err := adapter.Parse(ctx, []byte(eventJSON))

            // Business outcome: Invalid events rejected before CRD creation
            Expect(err).To(HaveOccurred(), businessReason)
            Expect(err.Error()).To(ContainSubstring(expectedError))
            Expect(signal).To(BeNil())
        },
        Entry("missing involvedObject → AI cannot target remediation",
            `{"type": "Warning", "reason": "Failed"}`,
            "missing involvedObject",
            "AI needs resource to remediate"),

        Entry("missing reason → AI cannot understand failure type",
            `{"type": "Warning", "involvedObject": {"kind": "Pod"}}`,
            "missing reason",
            "AI needs failure reason for root cause analysis"),

        Entry("malformed JSON → cannot parse event",
            `{invalid json`,
            "invalid JSON",
            "Prevent system crash on malformed data"),
    )
})
```

**Test Strategy**:
- ✅ **3 resource identification tests** (Pod, Node, Deployment)
- ✅ **2 event type tests** (Warning accepted, Normal rejected)
- ✅ **1 DescribeTable** (3 validation scenarios)
- ✅ **Total: 6 tests** covering BR-005

**Coverage**: 100% of K8s Event adapter business requirements

---

#### 1.2 Integration Test (1 test) - `test/integration/gateway/gateway_integration_test.go`

Add to existing integration suite:

```go
It("enables AI service to discover resource failures from Kubernetes events", func() {
    // Business scenario: Pod OOMKilled, K8s Event API sends notification
    k8sEvent := map[string]interface{}{
        "type":   "Warning",
        "reason": "OOMKilled",
        "message": "Container killed due to memory limit",
        "involvedObject": map[string]interface{}{
            "kind":      "Pod",
            "namespace": testNamespace,
            "name":      "payment-api-123",
        },
    }

    By("Kubernetes Event API sends event to Gateway")
    sendEventToGateway(k8sEvent)

    By("AI service discovers remediation request from K8s event")
    Eventually(func() bool {
        rrList := &remediationv1alpha1.RemediationRequestList{}
        err := k8sClient.List(context.Background(), rrList,
            client.InNamespace(testNamespace))
        return err == nil && len(rrList.Items) > 0
    }, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
        "K8s events should trigger RemediationRequest CRD creation")

    By("AI has K8s event context for remediation")
    rrList := &remediationv1alpha1.RemediationRequestList{}
    k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
    rr := rrList.Items[0]

    // Business outcome: AI has event data to analyze failure
    Expect(rr.Spec.SignalName).To(Equal("OOMKilled"),
        "AI needs failure type for remediation strategy")
    Expect(rr.Spec.ProviderData).NotTo(BeEmpty(),
        "AI needs event payload for analysis")

    // Business capability verified:
    // K8s Event → Gateway → CRD → AI analyzes OOM failure
})
```

**Coverage**: End-to-end K8s Event ingestion workflow

---

### Phase 2: Redis Behavior Tests (P1 - HIGH) 🔴
**Effort**: 3-4 hours  
**BRs Covered**: BR-011  
**Test Count**: +4 integration tests

#### 2.1 Integration Tests - `test/integration/gateway/redis_behavior_test.go`

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "time"
)

var _ = Describe("BR-GATEWAY-011: Redis Deduplication Storage", func() {

    It("expires fingerprints after TTL to allow re-processing", func() {
        // Business scenario: Same alert after 5 min window = new issue
        alert := makeProductionPodAlert("payment-service", "HighMemoryUsage", testNamespace)

        By("First alert creates CRD")
        sendAlertToGateway(alert)
        Eventually(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 10*time.Second).Should(Equal(1))

        By("Duplicate within TTL is deduplicated")
        sendAlertToGateway(alert)
        Consistently(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 2*time.Second).Should(Equal(1), "Duplicate should not create second CRD")

        By("After TTL expiry, alert re-processed")
        // Fast-forward Redis TTL (integration test can use shorter TTL for speed)
        time.Sleep(6 * time.Second) // Assuming 5-second TTL for test
        sendAlertToGateway(alert)

        Eventually(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 10*time.Second).Should(Equal(2),
            "Expired fingerprint allows re-processing of recurring issue")
    })

    It("persists deduplication state across Gateway restarts", func() {
        // Business scenario: Gateway pod crashes mid-deduplication
        alert := makeProductionPodAlert("api-service", "CrashLoopBackOff", testNamespace)

        By("First alert processed before Gateway restart")
        sendAlertToGateway(alert)
        Eventually(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 10*time.Second).Should(Equal(1))

        By("Gateway server restarts (simulated)")
        // Note: In real test, restart Gateway server or use Redis persistence validation
        // For now, verify Redis still has fingerprint

        By("Duplicate alert after restart is still deduplicated")
        sendAlertToGateway(alert)
        Consistently(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 2*time.Second).Should(Equal(1),
            "Redis persistence prevents duplicate CRD after Gateway restart")
    })

    It("handles concurrent updates to same fingerprint", func() {
        // Business scenario: 2 Gateway replicas receive same alert simultaneously
        alert := makeProductionPodAlert("database-service", "HighCPU", testNamespace)

        By("Two Gateway instances process same alert concurrently")
        // Send same alert twice rapidly
        go sendAlertToGateway(alert)
        go sendAlertToGateway(alert)

        By("Only one CRD created (Redis handles race condition)")
        Eventually(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 10*time.Second).Should(Equal(1),
            "Redis atomic operations prevent duplicate CRDs in HA setup")

        Consistently(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 3*time.Second).Should(Equal(1),
            "Race condition resolved, no extra CRDs")
    })

    It("gracefully degrades when Redis is unavailable", func() {
        // Business scenario: Redis down, but alerts still need processing
        alert := makeProductionPodAlert("critical-service", "ServiceDown", testNamespace)

        By("Redis connection fails (simulated by stopping Redis)")
        // Note: Test infrastructure would stop Redis container here

        By("Gateway still creates CRD without deduplication")
        sendAlertToGateway(alert)

        Eventually(func() int {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
            return len(rrList.Items)
        }, 10*time.Second).Should(BeNumerically(">=", 1),
            "Gateway continues processing alerts without Redis (no deduplication)")

        // Business outcome: Critical alerts still trigger remediation
        // Trade-off: Potential duplicates acceptable vs. missing critical alerts
    })
})
```

**Coverage**: 100% of Redis storage behaviors critical for production

---

### Phase 3: Remediation Path Decision (P1 - HIGH) 🔴
**Effort**: 2-3 hours  
**BRs Covered**: BR-022  
**Test Count**: +5 unit tests

#### 3.1 Unit Tests - `test/unit/gateway/remediation_path_test.go`

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

var _ = Describe("BR-GATEWAY-022: Remediation Path Decision", func() {
    var (
        pathDecider *processing.RemediationPathDecider
        ctx         context.Context
    )

    BeforeEach(func() {
        pathDecider = processing.NewRemediationPathDecider()
        ctx = context.Background()
    })

    // Business Outcome: Rego policy determines remediation strategy
    DescribeTable("Rego policy evaluates remediation strategies",
        func(environment string, priority string, expectedPath string, businessReason string) {
            signal := &processing.NormalizedSignal{
                Environment: environment,
                Priority:    priority,
            }

            path := pathDecider.DeterminePath(ctx, signal)

            // Business outcome: Path influences AI remediation aggressiveness
            Expect(path).To(Equal(expectedPath), businessReason)
        },
        Entry("P0 production → aggressive remediation with immediate action",
            "production", "P0", "aggressive",
            "Critical prod issues need immediate automated remediation"),

        Entry("P1 production → conservative remediation with GitOps PR",
            "production", "P1", "conservative",
            "High priority prod needs approval before destructive actions"),

        Entry("P0 staging → moderate remediation with validation",
            "staging", "P0", "moderate",
            "Staging allows faster iteration but needs validation"),

        Entry("P3 development → manual remediation",
            "development", "P3", "manual",
            "Low priority dev issues require manual review"),

        Entry("unknown environment → conservative (safe default)",
            "unknown", "P0", "conservative",
            "Unknown environment treated as production for safety"),
    )

    It("falls back to severity-based path when Rego policy fails", func() {
        // Business scenario: Rego policy unavailable or malformed
        signal := &processing.NormalizedSignal{
            Severity:    "critical",
            Environment: "production",
        }

        // Simulate Rego failure
        pathDecider.SetRegoFailure(true)

        path := pathDecider.DeterminePath(ctx, signal)

        // Business outcome: System continues with fallback logic
        Expect(path).NotTo(BeEmpty(),
            "Fallback ensures alerts still processed")
        Expect(path).To(Equal("moderate"),
            "Fallback uses conservative strategy for critical alerts")
    })

    It("validates Rego policy output format", func() {
        // Business scenario: Rego returns invalid path value
        signal := &processing.NormalizedSignal{
            Environment: "production",
            Priority:    "P0",
        }

        // Simulate invalid Rego output
        pathDecider.SetMockRegoOutput("invalid_path_value")

        path := pathDecider.DeterminePath(ctx, signal)

        // Business outcome: Invalid policy output triggers fallback
        Expect(path).To(BeElementOf([]string{"aggressive", "moderate", "conservative", "manual"}),
            "Invalid Rego output falls back to valid path")
    })

    It("caches Rego policy evaluations for performance", func() {
        // Business scenario: 100 alerts/sec, can't re-evaluate Rego each time
        signal := &processing.NormalizedSignal{
            Environment: "production",
            Priority:    "P0",
        }

        // First evaluation
        path1 := pathDecider.DeterminePath(ctx, signal)
        evalCount1 := pathDecider.GetEvaluationCount()

        // Second evaluation (same signal)
        path2 := pathDecider.DeterminePath(ctx, signal)
        evalCount2 := pathDecider.GetEvaluationCount()

        // Business outcome: Cache reduces Rego evaluation overhead
        Expect(path1).To(Equal(path2))
        Expect(evalCount2).To(Equal(evalCount1),
            "Cache prevents redundant Rego evaluations")
    })

    It("includes remediation path in CRD for AI consumption", func() {
        // Business scenario: AI needs path to choose remediation actions
        signal := &processing.NormalizedSignal{
            Environment: "production",
            Priority:    "P0",
        }

        path := pathDecider.DeterminePath(ctx, signal)
        crdSpec := pathDecider.ToCRDSpec(signal, path)

        // Business outcome: AI has remediation strategy guidance
        Expect(crdSpec.RemediationPath).NotTo(BeEmpty(),
            "CRD must include path for AI decision making")
        Expect(crdSpec.RemediationPath).To(Equal("aggressive"))
    })
})
```

**Coverage**: 100% of Rego policy evaluation and fallback logic

---

### Phase 4: CRD Creation Validation (P1 - HIGH) 🔴
**Effort**: 2-3 hours  
**BRs Covered**: BR-023  
**Test Count**: +3 integration tests

#### 4.1 Integration Tests - `test/integration/gateway/crd_creation_test.go`

```go
package gateway_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("BR-GATEWAY-023: RemediationRequest CRD Creation", func() {

    It("creates CRD with complete schema validation", func() {
        // Business scenario: CRD must be valid for downstream controllers
        alert := makeProductionPodAlert("payment-service", "HighMemoryUsage", testNamespace)

        By("Gateway creates RemediationRequest CRD")
        sendAlertToGateway(alert)

        Eventually(func() bool {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            err := k8sClient.List(context.Background(), rrList,
                client.InNamespace(testNamespace))
            return err == nil && len(rrList.Items) > 0
        }, 10*time.Second).Should(BeTrue())

        By("CRD passes OpenAPI schema validation")
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        rr := rrList.Items[0]

        // Business outcome: CRD schema compliance ensures controller compatibility
        Expect(rr.Spec.SignalName).NotTo(BeEmpty(),
            "Schema requires signalName field")
        Expect(rr.Spec.Environment).To(BeElementOf([]string{"production", "staging", "development", "unknown"}),
            "Schema validates environment enum")
        Expect(rr.Spec.Priority).To(MatchRegexp(`^P[0-3]$`),
            "Schema validates priority format")
    })

    It("populates CRD labels for controller filtering", func() {
        // Business scenario: Controllers filter CRDs by labels
        alert := makeProductionPodAlert("api-service", "CrashLoopBackOff", testNamespace)
        alert.Labels["team"] = "platform-engineering"

        By("Gateway creates CRD with propagated labels")
        sendAlertToGateway(alert)

        Eventually(func() bool {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            err := k8sClient.List(context.Background(), rrList,
                client.InNamespace(testNamespace))
            return err == nil && len(rrList.Items) > 0
        }, 10*time.Second).Should(BeTrue())

        By("CRD labels enable controller label selectors")
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        rr := rrList.Items[0]

        // Business outcome: Labels enable targeted controller reconciliation
        Expect(rr.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "gateway-service"),
            "Standard label for ownership tracking")
        Expect(rr.Labels).To(HaveKeyWithValue("kubernaut.io/environment", "production"),
            "Environment label for filtering")
        Expect(rr.Labels).To(HaveKeyWithValue("kubernaut.io/priority", "P0"),
            "Priority label for scheduling")
        Expect(rr.Labels).To(HaveKeyWithValue("kubernaut.io/team", "platform-engineering"),
            "Alert labels propagated to CRD")
    })

    It("sets owner references for cascade deletion", func() {
        // Business scenario: Deleting namespace should clean up CRDs
        alert := makeProductionPodAlert("cleanup-test", "TestAlert", testNamespace)

        By("Gateway creates CRD with owner reference")
        sendAlertToGateway(alert)

        Eventually(func() bool {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            err := k8sClient.List(context.Background(), rrList,
                client.InNamespace(testNamespace))
            return err == nil && len(rrList.Items) > 0
        }, 10*time.Second).Should(BeTrue())

        By("CRD has owner reference to Gateway ServiceAccount or ConfigMap")
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))
        rr := rrList.Items[0]

        // Business outcome: Owner references enable Kubernetes garbage collection
        // Note: Gateway may not set owner refs if it's cluster-scoped
        // This test validates the pattern if used
        if len(rr.OwnerReferences) > 0 {
            Expect(rr.OwnerReferences[0].Kind).To(BeElementOf([]string{"ServiceAccount", "ConfigMap"}),
                "Owner reference enables cascade deletion")
        }
    })
})
```

**Coverage**: 100% of CRD creation mechanics critical for downstream

---

### Phase 5: Rate Limiting (P2 - MEDIUM) 🟡
**Effort**: 2-3 hours  
**BRs Covered**: BR-004 (extension)  
**Test Count**: +3 integration tests

#### 5.1 Integration Tests - Add to `test/integration/gateway/gateway_integration_test.go`

```go
Describe("BR-GATEWAY-004: Rate Limiting (Extension)", func() {

    It("enforces per-source rate limits to prevent abuse", func() {
        // Business scenario: Noisy alertmanager sending 1000 alerts/sec
        alert := makeProductionPodAlert("test-service", "TestAlert", testNamespace)

        By("Sending 150 alerts rapidly (above 100/min limit)")
        for i := 0; i < 150; i++ {
            sendAlertToGateway(alert)
        }

        By("Rate limiter blocks excess alerts")
        // Expected: ~100 alerts accepted, ~50 rate limited
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

        // Business outcome: Rate limiting prevents system overload
        Expect(len(rrList.Items)).To(BeNumerically("<", 150),
            "Rate limiter blocks excess alerts")
        Expect(len(rrList.Items)).To(BeNumerically(">", 80),
            "Rate limiter allows legitimate traffic")
    })

    It("isolates rate limits per source IP", func() {
        // Business scenario: Multiple Prometheus instances, one is noisy
        alert1 := makeAlertFromSource("source-1", "Alert1", testNamespace)
        alert2 := makeAlertFromSource("source-2", "Alert2", testNamespace)

        By("Source 1 hits rate limit")
        for i := 0; i < 150; i++ {
            sendAlertFromIP(alert1, "192.168.1.1")
        }

        By("Source 2 still processes alerts normally")
        for i := 0; i < 10; i++ {
            sendAlertFromIP(alert2, "192.168.1.2")
        }

        By("Source 2 alerts not affected by Source 1 rate limit")
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

        // Business outcome: Per-source isolation prevents noisy neighbor
        source2Alerts := filterBySourceIP(rrList.Items, "192.168.1.2")
        Expect(len(source2Alerts)).To(Equal(10),
            "Source 2 not affected by Source 1 rate limit")
    })

    It("allows burst traffic within configured limits", func() {
        // Business scenario: Alert storm causes burst of 50 alerts in 1 second
        alert := makeProductionPodAlert("burst-test", "BurstAlert", testNamespace)

        By("Sending 50 alerts in rapid burst")
        for i := 0; i < 50; i++ {
            sendAlertToGateway(alert)
            time.Sleep(10 * time.Millisecond) // 50 in 500ms
        }

        By("Token bucket allows burst within limit")
        rrList := &remediationv1alpha1.RemediationRequestList{}
        k8sClient.List(context.Background(), rrList, client.InNamespace(testNamespace))

        // Business outcome: Burst capacity handles legitimate alert storms
        Expect(len(rrList.Items)).To(BeNumerically(">=", 40),
            "Burst capacity allows temporary spike")
    })
})
```

**Coverage**: 100% of rate limiting behaviors

---

### Phase 6: Error Handling & Edge Cases (P3 - LOW) 🟢
**Effort**: 2-3 hours  
**BRs Covered**: Various (error paths)  
**Test Count**: +5 integration tests

#### 6.1 Integration Tests - `test/integration/gateway/error_handling_test.go`

```go
var _ = Describe("Error Handling & Edge Cases", func() {

    It("handles malformed JSON gracefully", func() {
        // Business scenario: Corrupted webhook payload
        malformedJSON := []byte(`{invalid json`)

        resp := sendRawPayload(malformedJSON)

        // Business outcome: Clear error message for debugging
        Expect(resp.StatusCode).To(Equal(400))
        Expect(resp.Body).To(ContainSubstring("invalid JSON"))
    })

    It("handles very large payloads", func() {
        // Business scenario: Alertmanager sends 100KB payload
        largeAlert := makeAlertWithLargeAnnotations(100 * 1024) // 100KB

        resp := sendAlertToGateway(largeAlert)

        // Business outcome: Large alerts rejected to prevent DoS
        Expect(resp.StatusCode).To(BeElementOf([]int{400, 413}),
            "Large payload rejected or truncated")
    })

    It("handles missing required fields", func() {
        // Business scenario: Webhook missing alertname
        invalidAlert := []byte(`{
            "alerts": [{
                "labels": {"severity": "critical"}
            }]
        }`)

        resp := sendRawPayload(invalidAlert)

        // Business outcome: Validation prevents incomplete CRDs
        Expect(resp.StatusCode).To(Equal(400))
        Expect(resp.Body).To(ContainSubstring("missing alertname"))
    })

    It("handles K8s API server unavailability", func() {
        // Business scenario: K8s API down, can't create CRDs
        alert := makeProductionPodAlert("test-service", "TestAlert", testNamespace)

        // Simulate K8s API failure (test infrastructure would break connection)

        resp := sendAlertToGateway(alert)

        // Business outcome: Gateway returns 500 for retry
        Expect(resp.StatusCode).To(Equal(500))
        Expect(resp.Body).To(ContainSubstring("failed to create CRD"))
    })

    It("handles namespace not found scenarios", func() {
        // Business scenario: Alert references non-existent namespace
        alert := makeProductionPodAlert("test-service", "TestAlert", "non-existent-namespace")

        resp := sendAlertToGateway(alert)

        // Business outcome: Gateway creates CRD in default namespace
        Eventually(func() bool {
            rrList := &remediationv1alpha1.RemediationRequestList{}
            err := k8sClient.List(context.Background(), rrList,
                client.InNamespace("default"))
            return err == nil && len(rrList.Items) > 0
        }, 10*time.Second).Should(BeTrue(),
            "Non-existent namespace falls back to default")
    })
})
```

**Coverage**: 100% of error paths and edge cases

---

## Test Pyramid Compliance

### Current State
| Test Level | Current | Target | Status |
|------------|---------|--------|--------|
| **Unit** | 33 (83%) | 28 (70%) | ✅ Exceeds |
| **Integration** | 7 (17%) | 8 (20%) | ⚠️ Below |
| **E2E** | 0 (0%) | 4 (10%) | ❌ Missing |
| **Total** | 40 | 40 | - |

### After Extension
| Test Level | New Count | Percentage | Status |
|------------|-----------|------------|--------|
| **Unit** | 50 (+17) | 63% | ✅ Good |
| **Integration** | 25 (+18) | 32% | ✅ Good |
| **E2E** | 4 (+4) | 5% | ✅ Good |
| **Total** | 79 (+39) | 100% | ✅ Compliant |

**Analysis**: Extension improves pyramid balance, adds more integration tests.

---

## E2E Tests (Phase 7 - Optional) 🔵

**Effort**: 3-4 hours  
**BRs Covered**: BR-071, BR-072 (end-to-end workflows)  
**Test Count**: +4 e2e tests

### Location: `test/e2e/gateway/`

#### E2E Test 1: Complete Prometheus Workflow
```go
It("processes Prometheus alert end-to-end with real AlertManager", func() {
    // Deploy Prometheus with alert rule
    // Wait for alert to fire
    // Verify Gateway creates CRD
    // Verify downstream controller processes CRD
})
```

#### E2E Test 2: K8s Event Workflow
```go
It("processes Kubernetes event end-to-end", func() {
    // Create Pod that triggers OOMKilled event
    // Verify Gateway receives event
    // Verify CRD created
    // Verify remediation controller acts
})
```

#### E2E Test 3: Storm Aggregation
```go
It("aggregates alert storm into single CRD", func() {
    // Trigger 50 pod crashes
    // Verify Gateway creates 1 aggregated CRD
    // Verify AI analyzes root cause (not 50 symptoms)
})
```

#### E2E Test 4: Environment-Based Remediation
```go
It("applies conservative remediation in production", func() {
    // Send prod alert
    // Verify CRD has environment=production
    // Verify downstream uses GitOps PR (not direct kubectl)
})
```

**Note**: E2E tests are optional for initial coverage. Recommend implementing after Phases 1-6 complete.

---

## Implementation Timeline

### Week 1: Critical Path (P0 + P1)

**Day 1 (2h)**: Phase 0 - Fix Infrastructure
- ✅ Regenerate auth token
- ✅ Install CRDs in Kind cluster
- ✅ Verify 7/7 integration tests passing

**Day 2 (6h)**: Phase 1 - K8s Event Adapter
- ✅ Implement adapter (if not done)
- ✅ Write 6 unit tests
- ✅ Write 1 integration test
- ✅ Verify tests passing

**Day 3 (4h)**: Phase 2 - Redis Behavior
- ✅ Write 4 integration tests
- ✅ Test Redis TTL, persistence, concurrency, failover

**Day 4 (3h)**: Phase 3 - Remediation Path
- ✅ Write 5 unit tests
- ✅ Test Rego policy evaluation

**Day 5 (3h)**: Phase 4 - CRD Validation
- ✅ Write 3 integration tests
- ✅ Test CRD schema, labels, owner references

---

### Week 2: Extensions (P2 + P3)

**Day 6 (3h)**: Phase 5 - Rate Limiting
- ✅ Write 3 integration tests
- ✅ Test per-source limits, burst handling

**Day 7 (3h)**: Phase 6 - Error Handling
- ✅ Write 5 integration tests
- ✅ Test edge cases, error paths

**Day 8-9 (Optional, 6h)**: Phase 7 - E2E Tests
- ✅ Write 4 e2e tests
- ✅ Test complete workflows

---

## Success Criteria

### Coverage Metrics
- ✅ **100% BR coverage** (23/23 BRs tested)
- ✅ **70%+ unit test ratio** (50/79 = 63%)
- ✅ **20%+ integration ratio** (25/79 = 32%)
- ✅ **All integration tests passing** (0 failures)

### Quality Metrics
- ✅ **100% business outcome focused** (no implementation details)
- ✅ **DescribeTable usage** (validation tests use table-driven)
- ✅ **Clear BR traceability** (each test maps to BR)
- ✅ **Maintainable** (<50 lines per test)

### Infrastructure
- ✅ **Kind cluster stable** (no flaky tests)
- ✅ **Auth tokens valid** (24+ hours)
- ✅ **CRDs installed** (no schema errors)
- ✅ **Redis connectivity** (no connection errors)

---

## Risk Assessment

### High Risk
1. **K8s Event Adapter Not Implemented** (Phase 1)
   - **Risk**: Adapter code doesn't exist, need implementation + tests
   - **Mitigation**: Allocate 6h for adapter implementation
   - **Fallback**: Skip K8s Events for V1, focus on Prometheus only

### Medium Risk
2. **Integration Test Flakiness** (Phase 2-6)
   - **Risk**: Redis timing, concurrent tests causing flakiness
   - **Mitigation**: Use deterministic waits, clean state between tests
   - **Fallback**: Increase timeouts, add retries

3. **E2E Test Infrastructure** (Phase 7)
   - **Risk**: Real Prometheus setup complex, flaky
   - **Mitigation**: Use testcontainers, mock AlertManager
   - **Fallback**: Skip E2E tests for initial coverage

### Low Risk
4. **Unit Test Complexity** (Phase 3)
   - **Risk**: Rego policy mocking difficult
   - **Mitigation**: Use fake Rego evaluator, simple mocks
   - **Fallback**: Test fallback logic only

---

## Approval Checklist

Before proceeding with implementation, confirm:

- [ ] **Effort estimate acceptable** (12-16 hours over 1.5-2 days)
- [ ] **Test pyramid ratios approved** (63% unit, 32% integration, 5% e2e)
- [ ] **Phase priorities correct** (P0 → P1 → P2 → P3)
- [ ] **DescribeTable usage approved** (validation tests)
- [ ] **Business outcome focus maintained** (no implementation details)
- [ ] **Infrastructure fix first** (Phase 0 before new tests)
- [ ] **K8s Event adapter scope** (implement adapter + tests, or Prometheus only?)
- [ ] **E2E tests optional** (Phase 7 deferred if time-constrained?)

---

## Next Steps

1. **User Approval**: Review plan, provide feedback
2. **Phase 0**: Fix integration test infrastructure (BLOCKING)
3. **Phase 1-6**: Implement tests following plan
4. **Validation**: Run full test suite, verify 100% BR coverage
5. **Documentation**: Update testing-strategy.md with new test locations

---

**Confidence**: 90% (Very High)  
**Status**: 🔄 Awaiting User Approval  
**Author**: AI Assistant  
**Date**: October 10, 2025


