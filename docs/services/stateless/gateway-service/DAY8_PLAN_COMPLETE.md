# Day 8 APDC Plan - Critical Integration Tests

**Date:** October 22, 2025
**Phase:** APDC Plan (1 hour)
**Status:** ✅ Complete

---

## 🎯 Test Implementation Strategy

### **Overall Approach**:
1. Create test helper utilities FIRST (TDD infrastructure)
2. Write failing tests using helpers (DO-RED)
3. Implement minimal infrastructure to pass tests (DO-GREEN)
4. Extract common patterns and improve (DO-REFACTOR)

### **File Organization**:
```
test/integration/gateway/
├── suite_test.go                    (existing - Ginkgo suite)
├── helpers/
│   ├── concurrent_helpers.go        (NEW - concurrent request utilities)
│   ├── redis_helpers.go             (NEW - Redis load testing)
│   ├── k8s_helpers.go               (NEW - K8s API testing)
│   └── payload_generators.go        (NEW - realistic payloads)
├── concurrent_processing_test.go    (NEW - 6 tests)
├── redis_load_test.go               (NEW - 6 tests)
├── k8s_api_load_test.go             (NEW - 6 tests)
└── error_resilience_test.go         (NEW - 6 tests)
```

---

## 📋 Phase 1: Concurrent Processing Tests (6 tests)

### **Test File**: `concurrent_processing_test.go`

**BR Coverage**: BR-001, BR-002, BR-017, BR-018
**Priority**: CRITICAL
**Estimated Time**: 2 hours (RED + GREEN)

### **Test Scenarios**:

#### **Test 1: 100 Concurrent Prometheus Webhooks**
```go
It("should handle 100 concurrent Prometheus webhooks without data corruption", func() {
    // BUSINESS OUTCOME: Gateway handles production load without crashes

    // Setup
    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)
    prometheusPayload := helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
        AlertName: "HighCPU",
        Namespace: "production",
        Severity:  "critical",
    })

    // Execute: Send 100 concurrent requests
    responses := helpers.SendConcurrentWebhooks(100, gatewayURL+"/webhook/prometheus", prometheusPayload)

    // Verify: All requests succeeded
    Expect(responses).To(HaveLen(100))
    for _, resp := range responses {
        Expect(resp.StatusCode).To(Or(Equal(201), Equal(200))) // 201 Created or 200 Duplicate
    }

    // Verify: Correct number of CRDs created (deduplication working)
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(len(crds)).To(BeNumerically("<=", 100)) // May be fewer due to deduplication

    // Verify: No data corruption (all CRDs have valid data)
    for _, crd := range crds {
        Expect(crd.Spec.AlertName).To(Equal("HighCPU"))
        Expect(crd.Spec.Namespace).To(Equal("production"))
        Expect(crd.Spec.Priority).ToNot(BeEmpty())
    }
})
```

**Dependencies**:
- `helpers.GeneratePrometheusAlert()` - generate realistic Prometheus webhook payload
- `helpers.SendConcurrentWebhooks()` - send N concurrent HTTP requests
- `helpers.ListRemediationRequests()` - list all CRDs created
- `startTestGateway()` - start Gateway server with real Redis + K8s client

#### **Test 2: Mixed Prometheus + K8s Event Concurrent**
```go
It("should handle mixed Prometheus and K8s Event webhooks concurrently", func() {
    // BUSINESS OUTCOME: Different signal types don't interfere

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Generate 50 Prometheus + 50 K8s Event payloads
    prometheusPayloads := make([][]byte, 50)
    k8sEventPayloads := make([][]byte, 50)

    for i := 0; i < 50; i++ {
        prometheusPayloads[i] = helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: fmt.Sprintf("Alert-%d", i),
            Namespace: "production",
        })
        k8sEventPayloads[i] = helpers.GenerateK8sEvent(helpers.K8sEventOptions{
            Reason:    fmt.Sprintf("Event-%d", i),
            Namespace: "production",
        })
    }

    // Send concurrently
    var wg sync.WaitGroup
    prometheusResponses := make([]helpers.Response, 50)
    k8sResponses := make([]helpers.Response, 50)

    for i := 0; i < 50; i++ {
        wg.Add(2)
        go func(idx int) {
            defer wg.Done()
            prometheusResponses[idx] = helpers.SendWebhook(gatewayURL+"/webhook/prometheus", prometheusPayloads[idx])
        }(i)
        go func(idx int) {
            defer wg.Done()
            k8sResponses[idx] = helpers.SendWebhook(gatewayURL+"/webhook/kubernetes", k8sEventPayloads[idx])
        }(i)
    }
    wg.Wait()

    // Verify: All requests succeeded
    for _, resp := range prometheusResponses {
        Expect(resp.StatusCode).To(Or(Equal(201), Equal(200)))
    }
    for _, resp := range k8sResponses {
        Expect(resp.StatusCode).To(Or(Equal(201), Equal(200)))
    }

    // Verify: Correct routing (Prometheus alerts have different fingerprints than K8s Events)
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    prometheusCount := 0
    k8sCount := 0
    for _, crd := range crds {
        if strings.HasPrefix(crd.Spec.AlertName, "Alert-") {
            prometheusCount++
        } else if strings.HasPrefix(crd.Spec.AlertName, "Event-") {
            k8sCount++
        }
    }
    Expect(prometheusCount).To(BeNumerically(">", 0))
    Expect(k8sCount).To(BeNumerically(">", 0))
})
```

#### **Test 3: Concurrent Same Alert (Deduplication Race)**
```go
It("should handle concurrent requests to same alert (deduplication race)", func() {
    // BUSINESS OUTCOME: Deduplication prevents duplicate CRDs under load

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Generate SAME alert payload
    payload := helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
        AlertName: "DatabaseDown",
        Namespace: "production",
        Resource:  helpers.ResourceIdentifier{Kind: "Pod", Name: "postgres-0"},
        Severity:  "critical",
    })

    // Send same alert 10 times concurrently
    responses := helpers.SendConcurrentWebhooks(10, gatewayURL+"/webhook/prometheus", payload)

    // Verify: All requests succeeded (some may be 200 Duplicate)
    createdCount := 0
    duplicateCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 {
            createdCount++
        } else if resp.StatusCode == 200 {
            duplicateCount++
        }
    }

    // BUSINESS OUTCOME: Only 1 CRD created, rest marked as duplicates
    Expect(createdCount).To(Equal(1), "exactly 1 CRD should be created")
    Expect(duplicateCount).To(Equal(9), "9 requests should be marked as duplicates")

    // Verify: Only 1 CRD exists in K8s
    crds := helpers.ListRemediationRequestsByFingerprint(ctx, k8sClient, "DatabaseDown-production-Pod-postgres-0")
    Expect(crds).To(HaveLen(1))

    // Verify: Duplicate count is correct
    Expect(crds[0].Status.DuplicateCount).To(Equal(9))
})
```

#### **Test 4: Request ID Propagation Under Load**
```go
It("should maintain request ID propagation under concurrent load", func() {
    // BUSINESS OUTCOME: Traceability maintained under load (BR-016)

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 100 concurrent requests with unique request IDs
    requestIDs := make([]string, 100)
    responses := make([]helpers.Response, 100)

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            requestID := fmt.Sprintf("req-%d-%s", idx, uuid.New().String())
            requestIDs[idx] = requestID

            payload := helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
                AlertName: fmt.Sprintf("Alert-%d", idx),
            })
            responses[idx] = helpers.SendWebhookWithHeaders(gatewayURL+"/webhook/prometheus", payload, map[string]string{
                "X-Request-ID": requestID,
            })
        }(i)
    }
    wg.Wait()

    // Verify: All responses contain correct request ID
    for i, resp := range responses {
        Expect(resp.Headers.Get("X-Request-ID")).To(Equal(requestIDs[i]))
    }

    // Verify: CRDs contain request IDs in metadata
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    for _, crd := range crds {
        Expect(crd.Annotations["kubernaut.io/request-id"]).ToNot(BeEmpty())
    }
})
```

#### **Test 5: Concurrent Storm Detection**
```go
It("should handle concurrent storm detection without false positives", func() {
    // BUSINESS OUTCOME: Storm detection accurate under concurrent load (BR-007)

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 5 alerts concurrently (just below storm threshold of 10)
    responses := helpers.SendConcurrentWebhooks(5, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: "HighMemory",
            Namespace: "production",
        }))

    // Verify: No storm detected (all 201 Created)
    for _, resp := range responses {
        Expect(resp.StatusCode).To(Equal(201))
        var body map[string]interface{}
        _ = json.Unmarshal(resp.Body, &body)
        Expect(body["storm_detected"]).To(BeFalse())
    }

    // Send 10 more alerts concurrently (should trigger storm)
    responses = helpers.SendConcurrentWebhooks(10, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: "HighMemory",
            Namespace: "production",
        }))

    // Verify: Storm detected (at least some responses indicate storm)
    stormDetected := false
    for _, resp := range responses {
        var body map[string]interface{}
        _ = json.Unmarshal(resp.Body, &body)
        if body["storm_detected"] == true {
            stormDetected = true
            break
        }
    }
    Expect(stormDetected).To(BeTrue(), "storm should be detected after 15 total alerts")
})
```

#### **Test 6: Classification Accuracy Under Load**
```go
It("should maintain classification accuracy under concurrent load", func() {
    // BUSINESS OUTCOME: Priority assignment correct under load (BR-020)

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 50 production alerts + 50 staging alerts concurrently
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(2)
        go func(idx int) {
            defer wg.Done()
            _ = helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
                helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
                    AlertName: fmt.Sprintf("ProdAlert-%d", idx),
                    Namespace: "production",
                    Severity:  "critical",
                }))
        }(i)
        go func(idx int) {
            defer wg.Done()
            _ = helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
                helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
                    AlertName: fmt.Sprintf("StagingAlert-%d", idx),
                    Namespace: "staging",
                    Severity:  "warning",
                }))
        }(i)
    }
    wg.Wait()

    // Verify: Production alerts have P0/P1 priority
    prodCRDs := helpers.ListRemediationRequestsByNamespace(ctx, k8sClient, "production")
    for _, crd := range prodCRDs {
        Expect(crd.Spec.Priority).To(Or(Equal("P0"), Equal("P1")))
    }

    // Verify: Staging alerts have P2/P3 priority
    stagingCRDs := helpers.ListRemediationRequestsByNamespace(ctx, k8sClient, "staging")
    for _, crd := range stagingCRDs {
        Expect(crd.Spec.Priority).To(Or(Equal("P2"), Equal("P3")))
    }
})
```

---

## 📋 Phase 2: Redis Integration Tests (6 tests)

### **Test File**: `redis_load_test.go`

**BR Coverage**: BR-003, BR-004, BR-005, BR-008, BR-013
**Priority**: CRITICAL
**Estimated Time**: 2 hours (RED + GREEN)

### **Test Scenarios**:

#### **Test 1: Redis Connection Pool Exhaustion**
```go
It("should handle Redis connection pool exhaustion gracefully", func() {
    // BUSINESS OUTCOME: Gateway degrades gracefully, doesn't crash

    // Create Redis client with small pool (10 connections)
    smallPoolRedis := goredis.NewClient(&goredis.Options{
        Addr:     "localhost:6379",
        PoolSize: 10,
    })

    gatewayURL := startTestGateway(ctx, smallPoolRedis, k8sClient)

    // Send 100 concurrent requests (10x pool size)
    responses := helpers.SendConcurrentWebhooks(100, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: "PoolTest",
        }))

    // Verify: All requests completed (no crashes)
    Expect(responses).To(HaveLen(100))

    // Verify: Some requests may have failed gracefully with 503 Service Unavailable
    successCount := 0
    errorCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 || resp.StatusCode == 200 {
            successCount++
        } else if resp.StatusCode == 503 {
            errorCount++
        }
    }

    // BUSINESS OUTCOME: Gateway handled overload gracefully
    Expect(successCount + errorCount).To(Equal(100))
    Expect(successCount).To(BeNumerically(">", 50), "at least 50% requests should succeed")
})
```

#### **Test 2: Key Collision with Realistic Fingerprints**
```go
It("should handle Redis key collisions with realistic fingerprints", func() {
    // BUSINESS OUTCOME: Different alerts never treated as duplicates

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Generate 10,000 unique alerts with realistic fingerprints
    uniqueAlerts := helpers.GenerateUniqueAlerts(10000)

    // Send all alerts (in batches to avoid overwhelming Gateway)
    batchSize := 100
    for i := 0; i < len(uniqueAlerts); i += batchSize {
        end := i + batchSize
        if end > len(uniqueAlerts) {
            end = len(uniqueAlerts)
        }
        batch := uniqueAlerts[i:end]

        for _, alert := range batch {
            _ = helpers.SendWebhook(gatewayURL+"/webhook/prometheus", alert)
        }

        time.Sleep(100 * time.Millisecond) // Brief pause between batches
    }

    // Verify: All 10,000 alerts created unique CRDs (no collisions)
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(len(crds)).To(BeNumerically(">=", 9900), "at least 99% should be unique")

    // Verify: No duplicate fingerprints
    fingerprints := make(map[string]bool)
    for _, crd := range crds {
        fingerprint := crd.Spec.Fingerprint
        Expect(fingerprints[fingerprint]).To(BeFalse(), "fingerprint collision detected: %s", fingerprint)
        fingerprints[fingerprint] = true
    }
})
```

#### **Test 3: Deduplication Accuracy Under Sustained Load**
```go
It("should maintain deduplication accuracy under sustained load", func() {
    // BUSINESS OUTCOME: Deduplication >99% accurate in production

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 1000 alerts over 5 minutes (mix of unique and duplicate)
    // 500 unique alerts, each sent twice (1000 total requests)
    uniqueAlerts := helpers.GenerateUniqueAlerts(500)

    startTime := time.Now()
    requestCount := 0
    expectedDuplicates := 0

    for time.Since(startTime) < 5*time.Minute && requestCount < 1000 {
        // Send each alert twice
        for _, alert := range uniqueAlerts {
            // First request (should create CRD)
            resp1 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus", alert)
            Expect(resp1.StatusCode).To(Equal(201))
            requestCount++

            // Second request (should be duplicate)
            resp2 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus", alert)
            if resp2.StatusCode == 200 {
                expectedDuplicates++
            }
            requestCount++

            time.Sleep(600 * time.Millisecond) // ~1 req/s sustained load

            if requestCount >= 1000 {
                break
            }
        }
    }

    // Verify: Deduplication accuracy >99%
    accuracy := float64(expectedDuplicates) / 500.0 * 100.0
    Expect(accuracy).To(BeNumerically(">=", 99.0), "deduplication accuracy should be >99%%")

    // Verify: Only 500 CRDs created (not 1000)
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(len(crds)).To(BeNumerically("~", 500, 10), "should have ~500 CRDs, not 1000")
})
```

#### **Test 4: Redis Connection Loss and Recovery**
```go
It("should handle Redis connection loss and recovery", func() {
    // BUSINESS OUTCOME: Gateway recovers from Redis failures

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send request (should succeed)
    resp1 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "BeforeFailure"}))
    Expect(resp1.StatusCode).To(Equal(201))

    // Simulate Redis connection loss (close connection)
    _ = redisClient.Close()

    // Send request during Redis outage (should fail gracefully)
    resp2 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "DuringFailure"}))
    Expect(resp2.StatusCode).To(Equal(503), "should return 503 when Redis unavailable")

    // Reconnect Redis
    redisClient = goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DB: 1})
    _, err := redisClient.Ping(ctx).Result()
    Expect(err).ToNot(HaveOccurred())

    // Wait for Gateway to reconnect
    time.Sleep(2 * time.Second)

    // Send request after recovery (should succeed)
    resp3 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "AfterRecovery"}))
    Expect(resp3.StatusCode).To(Equal(201), "should succeed after Redis recovery")
})
```

#### **Test 5: Redis Memory Pressure**
```go
It("should handle Redis memory pressure gracefully", func() {
    // BUSINESS OUTCOME: Gateway doesn't crash when Redis is full

    // Fill Redis to near capacity (simulate memory pressure)
    helpers.FillRedisToCapacity(redisClient, 90) // 90% full

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send requests (should handle Redis memory errors gracefully)
    responses := helpers.SendConcurrentWebhooks(100, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "MemoryPressure"}))

    // Verify: Gateway didn't crash (all requests completed)
    Expect(responses).To(HaveLen(100))

    // Verify: Some requests may have failed with 503, but Gateway remained available
    availableCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 || resp.StatusCode == 200 || resp.StatusCode == 503 {
            availableCount++
        }
    }
    Expect(availableCount).To(Equal(100), "Gateway should remain available under memory pressure")
})
```

#### **Test 6: Storm State Across Redis Reconnections**
```go
It("should maintain storm detection state across Redis reconnections", func() {
    // BUSINESS OUTCOME: Storm detection survives Redis failures (BR-007)

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 5 alerts (build up storm state)
    for i := 0; i < 5; i++ {
        resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
            helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "StormTest"}))
        Expect(resp.StatusCode).To(Equal(201))
    }

    // Simulate Redis reconnection
    _ = redisClient.Close()
    time.Sleep(1 * time.Second)
    redisClient = goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DB: 1})

    // Send 10 more alerts (should trigger storm)
    for i := 0; i < 10; i++ {
        _ = helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
            helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "StormTest"}))
    }

    // Verify: Storm was detected (state persisted through reconnection)
    // Note: This test verifies storm state is stored in Redis, not just in-memory
    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "StormTest"}))

    var body map[string]interface{}
    _ = json.Unmarshal(resp.Body, &body)
    Expect(body["storm_detected"]).To(BeTrue(), "storm state should persist across Redis reconnections")
})
```

---

## 📋 Phase 3: K8s API Integration Tests (6 tests)

### **Test File**: `k8s_api_load_test.go`

**BR Coverage**: BR-015, BR-019
**Priority**: HIGH
**Estimated Time**: 2 hours (RED + GREEN)

### **Test Scenarios**:

#### **Test 1: K8s API Rate Limiting**
```go
It("should handle K8s API rate limiting gracefully", func() {
    // BUSINESS OUTCOME: Gateway retries on rate limit, eventual success

    // Create rate-limited K8s client (100 req/s limit)
    rateLimitedClient := helpers.NewRateLimitedK8sClient(k8sClient, 100)

    gatewayURL := startTestGateway(ctx, redisClient, rateLimitedClient)

    // Send 200 CRD creation requests rapidly (2x rate limit)
    responses := helpers.SendConcurrentWebhooks(200, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "RateLimitTest"}))

    // Verify: All requests eventually succeeded (with retries)
    successCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 || resp.StatusCode == 200 {
            successCount++
        }
    }

    // BUSINESS OUTCOME: Rate limiting handled gracefully, eventual consistency
    Expect(successCount).To(BeNumerically(">=", 190), "at least 95% should succeed with retries")
})
```

#### **Test 2: CRD Schema Validation**
```go
It("should validate CRD schema with real K8s API", func() {
    // BUSINESS OUTCOME: Invalid CRDs rejected before API call

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send alert with invalid data (missing required fields)
    invalidPayload := helpers.GenerateInvalidPrometheusAlert(helpers.InvalidOptions{
        MissingFields: []string{"alertname"},
    })

    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus", invalidPayload)

    // Verify: Request rejected with 400 Bad Request
    Expect(resp.StatusCode).To(Equal(400))

    var body map[string]interface{}
    _ = json.Unmarshal(resp.Body, &body)
    Expect(body["error"]).To(ContainSubstring("alertname"))

    // Verify: No CRD created
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(crds).To(HaveLen(0))
})
```

#### **Test 3: K8s API Intermittent Failures**
```go
It("should handle K8s API intermittent failures", func() {
    // BUSINESS OUTCOME: Gateway retries on transient failures

    // Create flaky K8s client (30% failure rate)
    flakyClient := helpers.NewFlakyK8sClient(k8sClient, 0.3)

    gatewayURL := startTestGateway(ctx, redisClient, flakyClient)

    // Send 100 requests
    responses := helpers.SendConcurrentWebhooks(100, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "FlakyTest"}))

    // Verify: Most requests eventually succeeded (with retries)
    successCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 {
            successCount++
        }
    }

    // BUSINESS OUTCOME: Retry logic handles transient failures
    Expect(successCount).To(BeNumerically(">=", 95), "at least 95% should succeed with retries")
})
```

#### **Test 4: K8s API Version Skew**
```go
It("should handle K8s API version skew", func() {
    // BUSINESS OUTCOME: Gateway works across K8s versions

    // Test against multiple K8s API versions (if available in test environment)
    // This test may be skipped if only one K8s version available

    if os.Getenv("SKIP_VERSION_SKEW_TEST") == "true" {
        Skip("K8s API version skew test skipped (single version environment)")
    }

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send request
    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "VersionSkewTest"}))

    // Verify: Request succeeded regardless of K8s version
    Expect(resp.StatusCode).To(Equal(201))

    // Verify: CRD created with correct API version
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(crds).To(HaveLen(1))
    Expect(crds[0].APIVersion).To(Equal("remediation.kubernaut.io/v1alpha1"))
})
```

#### **Test 5: CRD Admission Webhook Rejections**
```go
It("should handle CRD admission webhook rejections", func() {
    // BUSINESS OUTCOME: Gateway handles webhook validation errors

    // Create K8s client with admission webhook that rejects certain CRDs
    webhookClient := helpers.NewK8sClientWithAdmissionWebhook(k8sClient, func(crd *remediationv1alpha1.RemediationRequest) error {
        if crd.Spec.Priority == "P0" && crd.Spec.Namespace != "production" {
            return fmt.Errorf("P0 priority only allowed in production namespace")
        }
        return nil
    })

    gatewayURL := startTestGateway(ctx, redisClient, webhookClient)

    // Send alert that will be rejected by admission webhook
    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: "AdmissionTest",
            Namespace: "staging",
            Severity:  "critical", // Will result in P0 priority
        }))

    // Verify: Request failed with 500 Internal Server Error
    Expect(resp.StatusCode).To(Equal(500))

    var body map[string]interface{}
    _ = json.Unmarshal(resp.Body, &body)
    Expect(body["error"]).To(ContainSubstring("admission webhook"))
})
```

#### **Test 6: CRD Creation Accuracy Under Pressure**
```go
It("should maintain CRD creation accuracy under API pressure", func() {
    // BUSINESS OUTCOME: All CRDs created correctly under load

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 500 CRD creation requests rapidly
    responses := helpers.SendConcurrentWebhooks(500, gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "PressureTest"}))

    // Verify: All requests succeeded
    successCount := 0
    for _, resp := range responses {
        if resp.StatusCode == 201 {
            successCount++
        }
    }
    Expect(successCount).To(Equal(500), "100% success rate expected")

    // Verify: All CRDs have correct data (no corruption)
    crds := helpers.ListRemediationRequests(ctx, k8sClient)
    Expect(crds).To(HaveLen(500))

    for _, crd := range crds {
        Expect(crd.Spec.AlertName).To(Equal("PressureTest"))
        Expect(crd.Spec.Priority).ToNot(BeEmpty())
        Expect(crd.Spec.Fingerprint).ToNot(BeEmpty())
    }
})
```

---

## 📋 Phase 4: Error Handling & Resilience Tests (6 tests)

### **Test File**: `error_resilience_test.go`

**BR Coverage**: BR-019, BR-092
**Priority**: HIGH
**Estimated Time**: 2 hours (RED + GREEN)

### **Test Scenarios**:

#### **Test 1: Consistent Error Format Across Endpoints**
```go
It("should return consistent error format across all endpoints", func() {
    // BUSINESS OUTCOME: Clients parse errors reliably

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Trigger errors on all endpoints
    endpoints := []string{
        "/webhook/prometheus",
        "/webhook/kubernetes",
        "/health",
        "/ready",
    }

    for _, endpoint := range endpoints {
        // Send invalid request
        resp := helpers.SendWebhook(gatewayURL+endpoint, []byte("invalid json"))

        // Verify: Error response has consistent format
        var body map[string]interface{}
        err := json.Unmarshal(resp.Body, &body)
        Expect(err).ToNot(HaveOccurred(), "error response should be valid JSON")

        // Verify: RFC 7807 Problem Details format
        Expect(body).To(HaveKey("type"))
        Expect(body).To(HaveKey("title"))
        Expect(body).To(HaveKey("status"))
        Expect(body).To(HaveKey("detail"))
    }
})
```

#### **Test 2: Memory Pressure Handling**
```go
It("should handle memory pressure gracefully", func() {
    // BUSINESS OUTCOME: Gateway doesn't OOM crash

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send large payloads to exhaust memory (10MB each)
    largePayload := helpers.GenerateLargePrometheusAlert(10 * 1024 * 1024) // 10MB

    responses := helpers.SendConcurrentWebhooks(10, gatewayURL+"/webhook/prometheus", largePayload)

    // Verify: Gateway rejected oversized payloads (413 Payload Too Large)
    for _, resp := range responses {
        Expect(resp.StatusCode).To(Equal(413))
    }

    // Verify: Gateway still responsive after memory pressure
    healthResp := helpers.SendWebhook(gatewayURL+"/health", nil)
    Expect(healthResp.StatusCode).To(Equal(200))
})
```

#### **Test 3: Panic Recovery in Middleware**
```go
It("should recover from panic in middleware chain", func() {
    // BUSINESS OUTCOME: Single request panic doesn't crash Gateway

    // Create Gateway with middleware that panics on specific input
    gatewayURL := startTestGatewayWithPanicMiddleware(ctx, redisClient, k8sClient)

    // Send request that triggers panic
    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{
            AlertName: "PANIC_TRIGGER", // Special value that triggers panic
        }))

    // Verify: Request failed with 500 Internal Server Error (not crash)
    Expect(resp.StatusCode).To(Equal(500))

    // Verify: Gateway still responsive after panic
    healthResp := helpers.SendWebhook(gatewayURL+"/health", nil)
    Expect(healthResp.StatusCode).To(Equal(200))

    // Verify: Subsequent requests work normally
    normalResp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "Normal"}))
    Expect(normalResp.StatusCode).To(Equal(201))
})
```

#### **Test 4: Malformed JSON Payloads**
```go
It("should handle malformed JSON payloads without crash", func() {
    // BUSINESS OUTCOME: Invalid payloads don't crash Gateway

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 100 malformed JSON payloads
    malformedPayloads := [][]byte{
        []byte("{invalid json"),
        []byte("not json at all"),
        []byte("{\"incomplete\":"),
        []byte("null"),
        []byte(""),
        // ... 95 more variations
    }

    for _, payload := range malformedPayloads {
        resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus", payload)

        // Verify: Request rejected with 400 Bad Request
        Expect(resp.StatusCode).To(Equal(400))
    }

    // Verify: Gateway still responsive after malformed payloads
    healthResp := helpers.SendWebhook(gatewayURL+"/health", nil)
    Expect(healthResp.StatusCode).To(Equal(200))
})
```

#### **Test 5: Extremely Large Payloads**
```go
It("should handle extremely large payloads (>10MB)", func() {
    // BUSINESS OUTCOME: Gateway rejects oversized payloads

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send 20MB payload
    largePayload := helpers.GenerateLargePrometheusAlert(20 * 1024 * 1024)

    resp := helpers.SendWebhook(gatewayURL+"/webhook/prometheus", largePayload)

    // Verify: Request rejected with 413 Payload Too Large
    Expect(resp.StatusCode).To(Equal(413))

    var body map[string]interface{}
    _ = json.Unmarshal(resp.Body, &body)
    Expect(body["error"]).To(ContainSubstring("payload too large"))
})
```

#### **Test 6: Partial Failure Availability**
```go
It("should maintain service availability during partial failures", func() {
    // BUSINESS OUTCOME: Redis failure doesn't block K8s operations

    gatewayURL := startTestGateway(ctx, redisClient, k8sClient)

    // Send request (should succeed with Redis)
    resp1 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "BeforeRedisFailure"}))
    Expect(resp1.StatusCode).To(Equal(201))

    // Kill Redis
    _ = redisClient.Close()

    // Send request (should still create CRD, but deduplication disabled)
    resp2 := helpers.SendWebhook(gatewayURL+"/webhook/prometheus",
        helpers.GeneratePrometheusAlert(helpers.PrometheusAlertOptions{AlertName: "DuringRedisFailure"}))

    // BUSINESS OUTCOME: Gateway degraded but still functional
    // May return 201 (CRD created, dedup disabled) or 503 (Redis required for this endpoint)
    Expect(resp2.StatusCode).To(Or(Equal(201), Equal(503)))

    // Verify: Health endpoint indicates degraded state
    healthResp := helpers.SendWebhook(gatewayURL+"/health", nil)
    var healthBody map[string]interface{}
    _ = json.Unmarshal(healthResp.Body, &healthBody)
    Expect(healthBody["redis"]).To(Equal("unhealthy"))
})
```

---

## 🛠️ Test Helper Utilities (Priority Order)

### **1. `helpers/concurrent_helpers.go`** (CRITICAL)
```go
package helpers

type Response struct {
    StatusCode int
    Headers    http.Header
    Body       []byte
}

func SendConcurrentWebhooks(count int, url string, payload []byte) []Response
func SendWebhook(url string, payload []byte) Response
func SendWebhookWithHeaders(url string, payload []byte, headers map[string]string) Response
func ListRemediationRequests(ctx context.Context, client client.Client) []remediationv1alpha1.RemediationRequest
func ListRemediationRequestsByFingerprint(ctx context.Context, client client.Client, fingerprint string) []remediationv1alpha1.RemediationRequest
func ListRemediationRequestsByNamespace(ctx context.Context, client client.Client, namespace string) []remediationv1alpha1.RemediationRequest
```

### **2. `helpers/payload_generators.go`** (CRITICAL)
```go
package helpers

type PrometheusAlertOptions struct {
    AlertName string
    Namespace string
    Resource  ResourceIdentifier
    Severity  string
    Labels    map[string]string
}

type K8sEventOptions struct {
    Reason    string
    Namespace string
    Resource  ResourceIdentifier
}

type ResourceIdentifier struct {
    Kind string
    Name string
}

func GeneratePrometheusAlert(opts PrometheusAlertOptions) []byte
func GenerateK8sEvent(opts K8sEventOptions) []byte
func GenerateUniqueAlerts(count int) [][]byte
func GenerateInvalidPrometheusAlert(opts InvalidOptions) []byte
func GenerateLargePrometheusAlert(sizeBytes int) []byte
```

### **3. `helpers/redis_helpers.go`** (HIGH)
```go
package helpers

func FillRedisToCapacity(client *goredis.Client, percentFull int) error
func GenerateUniqueFingerprints(count int) []string
```

### **4. `helpers/k8s_helpers.go`** (HIGH)
```go
package helpers

func NewRateLimitedK8sClient(baseClient client.Client, rateLimit int) client.Client
func NewFlakyK8sClient(baseClient client.Client, failureRate float64) client.Client
func NewK8sClientWithAdmissionWebhook(baseClient client.Client, validator func(*remediationv1alpha1.RemediationRequest) error) client.Client
```

---

## ✅ Plan Phase Complete

### **Implementation Order**:

1. **DO-RED Phase** (2 hours):
   - Create helper utilities (concurrent, payload generators)
   - Write 24 failing tests (6 per phase)
   - Verify tests fail for correct reasons

2. **DO-GREEN Phase** (2 hours):
   - Implement minimal test infrastructure
   - Make tests pass with simplest implementation
   - Verify all 24 tests passing

3. **DO-REFACTOR Phase** (1 hour):
   - Extract common patterns
   - Improve test readability
   - Consolidate helper utilities

4. **CHECK Phase** (1 hour):
   - Verify all critical scenarios covered
   - Run full test suite
   - Document coverage improvements

**Total Estimated Time**: 8 hours (Day 8)

**Next Steps**: Proceed to DO-RED phase

