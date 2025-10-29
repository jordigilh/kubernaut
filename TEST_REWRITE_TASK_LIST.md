# Gateway Test Rewrite Task List

**Date**: October 28, 2025
**Purpose**: Detailed task list for rewriting implementation logic tests to verify business outcomes
**Source**: TEST_TRIAGE_BUSINESS_OUTCOME_VS_IMPLEMENTATION.md

---

## 📋 **TESTS REQUIRING REWRITE**

### **1. test/unit/gateway/adapters/prometheus_adapter_test.go**

**Status**: ⏸️ FLAGGED AS PENDING (PIt/PContext)
**Estimated Effort**: 1-1.5 hours
**Priority**: HIGH (foundational adapter tests)

#### **Current State (WRONG - Implementation Logic)**

```go
// ❌ Tests verify struct field extraction
It("should extract alert name from labels", func() {
    signal, _ := adapter.Parse(ctx, payload)
    Expect(signal.AlertName).To(Equal("HighMemoryUsage"))  // ❌ Tests struct field
})

It("should generate unique fingerprint for deduplication", func() {
    signal, _ := adapter.Parse(ctx, payload)
    Expect(signal.Fingerprint).NotTo(BeEmpty())  // ❌ Tests struct field
    Expect(len(signal.Fingerprint)).To(Equal(64))  // ❌ Tests implementation detail
})
```

**Problem**:
- Tests verify `Parse()` method returns correct struct fields
- Does NOT verify business outcome: Can Gateway deduplicate using this fingerprint?
- Does NOT verify business outcome: Can Gateway classify environment using this namespace?

#### **Target State (CORRECT - Business Outcomes)**

**Rewrite as Integration Tests** in `test/integration/gateway/prometheus_adapter_integration_test.go`:

```go
// ✅ BUSINESS OUTCOME: Prometheus alerts enable deduplication
var _ = Describe("BR-GATEWAY-001-003: Prometheus Alert Processing - Integration Tests", func() {
    var (
        ctx           context.Context
        gatewayServer *gateway.Server
        testServer    *httptest.Server
        redisClient   *RedisTestClient
        k8sClient     *K8sTestClient
    )

    BeforeEach(func() {
        // Setup using StartTestGateway() helper
        ctx = context.Background()
        redisClient = SetupRedisTestClient(ctx)
        k8sClient = SetupK8sTestClient(ctx)
        gatewayServer, _ = StartTestGateway(ctx, redisClient, k8sClient)
        testServer = httptest.NewServer(gatewayServer.Handler())
    })

    Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation", func() {
        It("creates RemediationRequest CRD with correct business metadata", func() {
            // BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
            payload := []byte(`{
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "HighMemoryUsage",
                        "severity": "critical",
                        "namespace": "production",
                        "pod": "payment-api-123"
                    },
                    "annotations": {
                        "summary": "Pod memory usage at 95%"
                    },
                    "startsAt": "2025-10-22T10:00:00Z"
                }]
            }`)

            // Send webhook
            url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)
            resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            // BUSINESS OUTCOME 1: HTTP 201 Created
            Expect(resp.StatusCode).To(Equal(http.StatusCreated))

            // BUSINESS OUTCOME 2: CRD created in K8s with correct business metadata
            var crdList remediationv1alpha1.RemediationRequestList
            err = k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))
            Expect(err).NotTo(HaveOccurred())
            Expect(crdList.Items).To(HaveLen(1), "One CRD should be created")

            crd := crdList.Items[0]

            // Verify business metadata for AI analysis
            Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"),
                "AI needs alert name to understand failure type")
            Expect(crd.Spec.Priority).To(Equal("P0"),
                "critical + production = P0 (revenue-impacting)")
            Expect(crd.Spec.Environment).To(Equal("production"),
                "Environment classification for priority assignment")
            Expect(crd.Spec.Severity).To(Equal("critical"),
                "Severity for AI remediation strategy")
            Expect(crd.Namespace).To(Equal("production"),
                "Namespace for kubectl targeting")

            // BUSINESS OUTCOME 3: Fingerprint stored in Redis for deduplication
            fingerprint := crd.Labels["kubernaut.io/fingerprint"]
            Expect(fingerprint).NotTo(BeEmpty(), "Fingerprint label must exist")

            exists, _ := redisClient.Client.Exists(ctx, "alert:fingerprint:"+fingerprint).Result()
            Expect(exists).To(Equal(int64(1)),
                "Fingerprint must be stored in Redis for deduplication")

            // BUSINESS CAPABILITY VERIFIED:
            // ✅ Prometheus alert → Gateway → CRD created with business metadata
            // ✅ AI receives complete context (alert name, severity, priority, environment)
            // ✅ Fingerprint enables deduplication (stored in Redis)
        })
    })

    Context("BR-GATEWAY-005: Deduplication Using Prometheus Alert Fingerprint", func() {
        It("prevents duplicate CRDs for identical Prometheus alerts", func() {
            // BUSINESS SCENARIO: Same alert fires twice → Only 1 CRD created
            payload := []byte(`{
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "CPUThrottling",
                        "severity": "warning",
                        "namespace": "production",
                        "pod": "api-gateway-7"
                    },
                    "startsAt": "2025-10-22T12:00:00Z"
                }]
            }`)

            url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)

            // First alert: Creates CRD
            resp1, _ := http.Post(url, "application/json", bytes.NewReader(payload))
            defer resp1.Body.Close()
            Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

            // BUSINESS OUTCOME 1: First CRD created
            var crdList1 remediationv1alpha1.RemediationRequestList
            k8sClient.Client.List(ctx, &crdList1, client.InNamespace("production"))
            Expect(crdList1.Items).To(HaveLen(1), "First alert creates CRD")

            firstCRDName := crdList1.Items[0].Name

            // Second alert: Duplicate (within TTL)
            resp2, _ := http.Post(url, "application/json", bytes.NewReader(payload))
            defer resp2.Body.Close()
            Expect(resp2.StatusCode).To(Equal(http.StatusAccepted),
                "Duplicate alert returns 202 Accepted")

            // BUSINESS OUTCOME 2: NO new CRD created (deduplication works)
            var crdList2 remediationv1alpha1.RemediationRequestList
            k8sClient.Client.List(ctx, &crdList2, client.InNamespace("production"))
            Expect(crdList2.Items).To(HaveLen(1),
                "Duplicate alert must NOT create new CRD")
            Expect(crdList2.Items[0].Name).To(Equal(firstCRDName),
                "Same CRD name confirms deduplication")

            // BUSINESS OUTCOME 3: Redis metadata updated
            fingerprint := crdList2.Items[0].Labels["kubernaut.io/fingerprint"]
            count, _ := redisClient.Client.HGet(ctx, "alert:fingerprint:"+fingerprint, "count").Int()
            Expect(count).To(BeNumerically(">=", 2),
                "Duplicate count tracked in Redis")

            // BUSINESS CAPABILITY VERIFIED:
            // ✅ Fingerprint generation enables deduplication
            // ✅ Duplicate alerts don't create duplicate CRDs (prevents K8s API spam)
            // ✅ Redis tracks duplicate count for operational visibility
        })
    })

    Context("BR-GATEWAY-011: Environment Classification from Prometheus Alert", func() {
        It("classifies environment from namespace for priority assignment", func() {
            // BUSINESS SCENARIO: Namespace determines environment → Affects priority

            testCases := []struct {
                namespace   string
                severity    string
                expectedEnv string
                expectedPri string
            }{
                {"production", "critical", "production", "P0"},
                {"staging", "critical", "staging", "P1"},
                {"development", "critical", "development", "P2"},
            }

            for _, tc := range testCases {
                // Clean K8s before each test
                k8sClient.Client.DeleteAllOf(ctx, &remediationv1alpha1.RemediationRequest{},
                    client.InNamespace(tc.namespace))

                payload := []byte(fmt.Sprintf(`{
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "TestAlert",
                            "severity": "%s",
                            "namespace": "%s"
                        }
                    }]
                }`, tc.severity, tc.namespace))

                url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)
                resp, _ := http.Post(url, "application/json", bytes.NewReader(payload))
                resp.Body.Close()

                // BUSINESS OUTCOME: CRD has correct environment and priority
                var crdList remediationv1alpha1.RemediationRequestList
                k8sClient.Client.List(ctx, &crdList, client.InNamespace(tc.namespace))
                Expect(crdList.Items).To(HaveLen(1))

                crd := crdList.Items[0]
                Expect(crd.Spec.Environment).To(Equal(tc.expectedEnv),
                    "Namespace %s → Environment %s", tc.namespace, tc.expectedEnv)
                Expect(crd.Spec.Priority).To(Equal(tc.expectedPri),
                    "%s + %s → Priority %s", tc.severity, tc.expectedEnv, tc.expectedPri)
            }

            // BUSINESS CAPABILITY VERIFIED:
            // ✅ Environment classification from namespace works
            // ✅ Priority assignment uses environment (production critical = P0)
        })
    })
})
```

#### **Rewrite Tasks**

1. **Create new file**: `test/integration/gateway/prometheus_adapter_integration_test.go`
2. **Test 1**: "creates RemediationRequest CRD with correct business metadata" (30 min)
   - Verify CRD created in K8s
   - Verify business metadata (signalName, priority, environment, severity)
   - Verify fingerprint stored in Redis
3. **Test 2**: "prevents duplicate CRDs for identical Prometheus alerts" (20 min)
   - First alert creates CRD
   - Second alert returns 202, NO new CRD
   - Redis duplicate count incremented
4. **Test 3**: "classifies environment from namespace for priority assignment" (20 min)
   - Test production/staging/development namespaces
   - Verify correct environment classification
   - Verify correct priority assignment
5. **Test 4**: "extracts resource information for AI targeting" (15 min)
   - Verify pod/node/deployment resource info in CRD
   - Verify AI can target specific resources
6. **Cleanup**: Remove flagged tests from `prometheus_adapter_test.go` (5 min)

**Total Effort**: 1.5 hours

---

### **2. test/integration/gateway/webhook_integration_test.go**

**Status**: ⏸️ FLAGGED AS PENDING (PDescribe)
**Estimated Effort**: 2-2.5 hours
**Priority**: CRITICAL (E2E webhook processing tests)

#### **Current State (WRONG - Implementation Logic)**

```go
// ❌ Tests verify HTTP response body structure
It("creates RemediationRequest CRD from Prometheus AlertManager webhook", func() {
    resp, _ := http.Post(testServer.URL+"/webhook/prometheus", "application/json", payload)

    var response map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&response)

    // ❌ Tests HTTP response JSON fields (implementation detail)
    Expect(response["status"]).To(Equal("created"))
    Expect(response["priority"]).To(Equal("P0"))
    Expect(response["resource_info"]).NotTo(BeNil())  // ❌ Field doesn't exist!
})
```

**Problem**:
- Tests verify HTTP response body structure
- Does NOT verify business outcome: Is CRD created in K8s?
- Does NOT verify business outcome: Is fingerprint stored in Redis?
- Guessed field names that don't exist (`resource_info`)

#### **Target State (CORRECT - Business Outcomes)**

**Complete Rewrite** in `test/integration/gateway/webhook_integration_test.go`:

```go
// ✅ BUSINESS OUTCOME: End-to-end webhook processing
var _ = Describe("BR-GATEWAY-001-015: End-to-End Webhook Processing - Integration Tests", func() {
    var (
        ctx           context.Context
        gatewayServer *gateway.Server
        testServer    *httptest.Server
        redisClient   *RedisTestClient
        k8sClient     *K8sTestClient
    )

    BeforeEach(func() {
        ctx = context.Background()
        redisClient = SetupRedisTestClient(ctx)
        k8sClient = SetupK8sTestClient(ctx)
        redisClient.Client.FlushDB(ctx)  // Clean Redis
        gatewayServer, _ = StartTestGateway(ctx, redisClient, k8sClient)
        testServer = httptest.NewServer(gatewayServer.Handler())
    })

    Context("BR-GATEWAY-001: Prometheus Alert → CRD Creation", func() {
        It("creates RemediationRequest CRD for production critical alert", func() {
            // BUSINESS SCENARIO: Production pod memory alert → AI analysis triggered
            payload := []byte(`{
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "HighMemoryUsage",
                        "severity": "critical",
                        "namespace": "production",
                        "pod": "payment-api-123"
                    },
                    "annotations": {
                        "summary": "Pod payment-api-123 using 95% memory"
                    },
                    "startsAt": "2025-10-22T10:00:00Z"
                }]
            }`)

            url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)
            resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()

            // BUSINESS OUTCOME 1: HTTP 201 Created
            Expect(resp.StatusCode).To(Equal(http.StatusCreated))

            // BUSINESS OUTCOME 2: CRD created in K8s
            var crdList remediationv1alpha1.RemediationRequestList
            err = k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))
            Expect(err).NotTo(HaveOccurred())
            Expect(crdList.Items).To(HaveLen(1))

            crd := crdList.Items[0]
            Expect(crd.Spec.Priority).To(Equal("P0"))
            Expect(crd.Spec.Environment).To(Equal("production"))
            Expect(crd.Spec.SignalName).To(Equal("HighMemoryUsage"))

            // BUSINESS OUTCOME 3: Fingerprint in Redis
            fingerprint := crd.Labels["kubernaut.io/fingerprint"]
            exists, _ := redisClient.Client.Exists(ctx, "alert:fingerprint:"+fingerprint).Result()
            Expect(exists).To(Equal(int64(1)))
        })
    })

    Context("BR-GATEWAY-003-005: Deduplication", func() {
        It("returns 202 Accepted for duplicate alerts", func() {
            // BUSINESS SCENARIO: Same alert fires 3 times → Only 1 CRD
            payload := []byte(`{
                "alerts": [{
                    "status": "firing",
                    "labels": {
                        "alertname": "CPUThrottling",
                        "severity": "warning",
                        "namespace": "production",
                        "pod": "api-gateway-7"
                    }
                }]
            }`)

            url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)

            // First alert
            resp1, _ := http.Post(url, "application/json", bytes.NewReader(payload))
            defer resp1.Body.Close()
            Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

            // Second alert (duplicate)
            resp2, _ := http.Post(url, "application/json", bytes.NewReader(payload))
            defer resp2.Body.Close()
            Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))

            // BUSINESS OUTCOME: Only 1 CRD created
            var crdList remediationv1alpha1.RemediationRequestList
            k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))
            Expect(crdList.Items).To(HaveLen(1))
        })
    })

    Context("BR-GATEWAY-013: Storm Detection", func() {
        It("aggregates multiple related alerts into single storm CRD", func() {
            // BUSINESS SCENARIO: Node failure → 15 pod alerts → 1 storm CRD
            url := fmt.Sprintf("%s/webhook/prometheus", testServer.URL)

            for i := 1; i <= 15; i++ {
                payload := []byte(fmt.Sprintf(`{
                    "alerts": [{
                        "status": "firing",
                        "labels": {
                            "alertname": "PodNotReady",
                            "severity": "critical",
                            "namespace": "production",
                            "pod": "app-pod-%d",
                            "node": "worker-node-03"
                        }
                    }]
                }`, i))

                resp, _ := http.Post(url, "application/json", bytes.NewReader(payload))
                resp.Body.Close()
            }

            // Wait for storm aggregation window
            time.Sleep(2 * time.Second)

            // BUSINESS OUTCOME: Storm CRD created (not 15 individual CRDs)
            var crdList remediationv1alpha1.RemediationRequestList
            k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))

            // Should have 1 storm CRD (not 15)
            Expect(crdList.Items).To(HaveLen(1))

            crd := crdList.Items[0]
            Expect(crd.Spec.StormAlertCount).To(BeNumerically(">=", 15))
            Expect(crd.Labels["kubernaut.io/storm"]).To(Equal("true"))
        })
    })

    Context("BR-GATEWAY-002: Kubernetes Event Webhooks", func() {
        It("creates CRD from Kubernetes Warning events", func() {
            // BUSINESS SCENARIO: Pod OOMKilled event → AI analyzes memory issue
            payload := []byte(`{
                "type": "Warning",
                "reason": "OOMKilled",
                "message": "Container killed due to out of memory",
                "involvedObject": {
                    "kind": "Pod",
                    "namespace": "production",
                    "name": "payment-processor-42"
                },
                "metadata": {
                    "namespace": "production"
                }
            }`)

            url := fmt.Sprintf("%s/webhook/kubernetes", testServer.URL)
            resp, _ := http.Post(url, "application/json", bytes.NewReader(payload))
            defer resp.Body.Close()

            // BUSINESS OUTCOME: CRD created for K8s event
            Expect(resp.StatusCode).To(Equal(http.StatusCreated))

            var crdList remediationv1alpha1.RemediationRequestList
            k8sClient.Client.List(ctx, &crdList, client.InNamespace("production"))
            Expect(crdList.Items).To(HaveLen(1))

            crd := crdList.Items[0]
            Expect(crd.Spec.SignalName).To(Equal("OOMKilled"))
            Expect(crd.Spec.SourceType).To(Equal("kubernetes-event"))
        })
    })
})
```

#### **Rewrite Tasks**

1. **Test 1**: "creates RemediationRequest CRD for production critical alert" (30 min)
   - Verify CRD created in K8s with correct spec
   - Verify fingerprint in Redis
2. **Test 2**: "returns 202 Accepted for duplicate alerts" (20 min)
   - Verify duplicate detection works
   - Verify NO duplicate CRD created
3. **Test 3**: "tracks duplicate count and timestamps" (20 min)
   - Verify Redis metadata updated
   - Verify duplicate count incremented
4. **Test 4**: "aggregates multiple related alerts into single storm CRD" (30 min)
   - Send 15 alerts
   - Verify 1 storm CRD created (not 15)
   - Verify storm metadata
5. **Test 5**: "creates CRD from Kubernetes Warning events" (20 min)
   - Test K8s event webhook
   - Verify CRD created with event details
6. **Cleanup**: Remove old implementation logic tests (10 min)

**Total Effort**: 2.5 hours

---

## 📊 **SUMMARY**

| File | Tests to Rewrite | Effort | Priority |
|------|------------------|--------|----------|
| **prometheus_adapter_test.go** | 8 tests | 1.5h | HIGH |
| **webhook_integration_test.go** | 5 tests | 2.5h | CRITICAL |
| **TOTAL** | 13 tests | 4h | - |

---

## ✅ **COMPLETION CRITERIA**

### **For Each Rewritten Test**

1. ✅ **Verifies business outcome** (CRD in K8s, data in Redis)
2. ✅ **Does NOT test implementation details** (struct fields, HTTP response body)
3. ✅ **Maps to specific BR-GATEWAY-XXX** business requirement
4. ✅ **Uses real infrastructure** (Redis, K8s client)
5. ✅ **Clear business scenario** in test description
6. ✅ **Compiles and passes** when implementation is correct

### **Overall Success**

- ✅ All 13 tests rewritten to verify business outcomes
- ✅ All tests compile and pass
- ✅ No implementation logic tests remain
- ✅ 100% business outcome test coverage for critical flows

---

## 🎯 **NEXT STEPS**

1. **Review this task list** with user for approval
2. **Begin rewriting tests** following the patterns above
3. **Verify each test** verifies business outcome, not implementation
4. **Run full test suite** to ensure no regressions
5. **Update TEST_TRIAGE document** with completion status


