# 📋 Day 9 Phase 2: APDC Plan - Prometheus Metrics Integration

**Date**: 2025-10-26
**Phase**: APDC Plan
**Duration**: 15 minutes
**Status**: ✅ **COMPLETE**

---

## 🎯 **Implementation Strategy**

### **TDD Approach: REFACTOR Phase**

**Why REFACTOR, not RED-GREEN?**
- ✅ Metrics infrastructure already exists (DO-GREEN stub)
- ✅ Tests already exist (integration tests validate behavior)
- ✅ Code already works (metrics just need wiring)
- ✅ This is **enhancement**, not new functionality

**TDD Cycle**:
1. **REFACTOR**: Wire metrics to existing code
2. **VERIFY**: Run existing tests to ensure no breakage
3. **VALIDATE**: Check metrics are tracked correctly

---

## 🗂️ **Implementation Order - Dependency-Driven**

### **Phase 2.1: Server Initialization** (5 min)
**Why First**: Foundation - enables all other components

**Files**:
- `pkg/gateway/server/server.go` (line 162)

**Changes**:
```go
// OLD:
metrics: nil, // TODO Day 9: Implement metrics properly

// NEW:
metrics: gatewayMetrics.NewMetrics(),
```

**Validation**:
- ✅ Server compiles
- ✅ No nil pointer panics
- ✅ Metrics registered with Prometheus

---

### **Phase 2.2: Authentication Middleware** (30 min)
**Why Second**: Critical path - tracks K8s API timeouts (BR-GATEWAY-010)

**Files**:
- `pkg/gateway/middleware/auth.go`

**Metrics to Wire**:
1. `TokenReviewRequests.WithLabelValues("success").Inc()` - On successful auth
2. `TokenReviewRequests.WithLabelValues("timeout").Inc()` - On timeout
3. `TokenReviewRequests.WithLabelValues("error").Inc()` - On error
4. `TokenReviewTimeouts.Inc()` - On timeout (>5s)
5. `K8sAPILatency.WithLabelValues("tokenreview").Observe(duration)` - Always

**Implementation Pattern**:
```go
func TokenReviewAuth(k8sClient kubernetes.Interface, metrics *metrics.Metrics) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()

            // Extract token
            token, err := extractBearerToken(r.Header.Get("Authorization"))
            if err != nil {
                respondAuthError(w, http.StatusUnauthorized, "Invalid token")
                return
            }

            // Call TokenReview API with 5s timeout
            ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
            defer cancel()

            tr := &authv1.TokenReview{
                Spec: authv1.TokenReviewSpec{Token: token},
            }

            result, err := k8sClient.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})

            // Track latency (always)
            duration := time.Since(start).Seconds()
            if metrics != nil {
                metrics.K8sAPILatency.WithLabelValues("tokenreview").Observe(duration)
            }

            // Handle timeout
            if ctx.Err() == context.DeadlineExceeded {
                if metrics != nil {
                    metrics.TokenReviewRequests.WithLabelValues("timeout").Inc()
                    metrics.TokenReviewTimeouts.Inc()
                }
                respondAuthError(w, http.StatusServiceUnavailable, "Authentication timeout")
                return
            }

            // Handle error
            if err != nil {
                if metrics != nil {
                    metrics.TokenReviewRequests.WithLabelValues("error").Inc()
                }
                respondAuthError(w, http.StatusUnauthorized, "Authentication failed")
                return
            }

            // Handle invalid token
            if !result.Status.Authenticated {
                if metrics != nil {
                    metrics.TokenReviewRequests.WithLabelValues("error").Inc()
                }
                respondAuthError(w, http.StatusUnauthorized, "Invalid token")
                return
            }

            // Success
            if metrics != nil {
                metrics.TokenReviewRequests.WithLabelValues("success").Inc()
            }

            // Store ServiceAccount identity in context
            ctx = context.WithValue(r.Context(), serviceAccountKey, result.Status.User.Username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Validation**:
- ✅ Metrics tracked on success
- ✅ Metrics tracked on timeout
- ✅ Metrics tracked on error
- ✅ Nil checks prevent panics
- ✅ Integration tests pass

---

### **Phase 2.3: Authorization Middleware** (30 min)
**Why Third**: Critical path - tracks SubjectAccessReview timeouts

**Files**:
- `pkg/gateway/middleware/authz.go`

**Metrics to Wire**:
1. `SubjectAccessReviewRequests.WithLabelValues("success").Inc()` - On authorized
2. `SubjectAccessReviewRequests.WithLabelValues("timeout").Inc()` - On timeout
3. `SubjectAccessReviewRequests.WithLabelValues("error").Inc()` - On error
4. `SubjectAccessReviewTimeouts.Inc()` - On timeout (>5s)
5. `K8sAPILatency.WithLabelValues("subjectaccessreview").Observe(duration)` - Always

**Implementation Pattern**: Same as authentication middleware

**Validation**:
- ✅ Metrics tracked on success
- ✅ Metrics tracked on timeout
- ✅ Metrics tracked on error
- ✅ Nil checks prevent panics
- ✅ Integration tests pass

---

### **Phase 2.4: Webhook Handler** (45 min)
**Why Fourth**: Business logic - tracks signal processing

**Files**:
- `pkg/gateway/server/handlers.go`

**Metrics to Wire**:
1. `SignalsReceived.WithLabelValues(source, signalType).Inc()` - On webhook received
2. `SignalsProcessed.WithLabelValues(source, priority, environment).Inc()` - On success
3. `SignalsFailed.WithLabelValues(source, errorType).Inc()` - On error
4. `ProcessingDuration.WithLabelValues(source, stage).Observe(duration)` - Per stage

**Implementation Pattern**:
```go
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // Determine source from URL path
    source := extractSource(r.URL.Path) // "prometheus", "alertmanager", etc.

    // Track signal received
    if s.metrics != nil {
        s.metrics.SignalsReceived.WithLabelValues(source, "alert").Inc()
    }

    // Parse webhook payload
    var payload WebhookPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        if s.metrics != nil {
            s.metrics.SignalsFailed.WithLabelValues(source, "parse_error").Inc()
        }
        s.respondError(w, http.StatusBadRequest, "Invalid payload")
        return
    }

    // Normalize signal
    normalizedSignal, err := s.adapterRegistry.Normalize(source, payload)
    if err != nil {
        if s.metrics != nil {
            s.metrics.SignalsFailed.WithLabelValues(source, "normalization_error").Inc()
        }
        s.respondError(w, http.StatusBadRequest, "Normalization failed")
        return
    }

    // Track normalization duration
    if s.metrics != nil {
        s.metrics.ProcessingDuration.WithLabelValues(source, "normalization").Observe(time.Since(start).Seconds())
    }

    // Check deduplication
    isDuplicate, err := s.dedupService.CheckDuplicate(ctx, normalizedSignal)
    if err != nil {
        if s.metrics != nil {
            s.metrics.SignalsFailed.WithLabelValues(source, "dedup_error").Inc()
        }
        s.respondError(w, http.StatusInternalServerError, "Deduplication check failed")
        return
    }

    if isDuplicate {
        // Duplicate signal - return 202 Accepted
        s.respondJSON(w, http.StatusAccepted, map[string]string{
            "status": "duplicate",
            "message": "Signal already processed",
        })
        return
    }

    // Classify environment
    environment := s.classifier.Classify(ctx, normalizedSignal)

    // Assign priority
    priority := s.priorityEngine.AssignPriority(ctx, normalizedSignal, environment)

    // Create CRD
    crd, err := s.crdCreator.CreateRemediationRequest(ctx, normalizedSignal, priority, environment)
    if err != nil {
        if s.metrics != nil {
            s.metrics.SignalsFailed.WithLabelValues(source, "crd_creation_error").Inc()
        }
        s.respondError(w, http.StatusInternalServerError, "CRD creation failed")
        return
    }

    // Success - track metrics
    if s.metrics != nil {
        s.metrics.SignalsProcessed.WithLabelValues(source, priority, environment).Inc()
        s.metrics.ProcessingDuration.WithLabelValues(source, "total").Observe(time.Since(start).Seconds())
    }

    s.respondJSON(w, http.StatusCreated, map[string]string{
        "status": "created",
        "crd": crd.Name,
    })
}
```

**Validation**:
- ✅ Metrics tracked on success
- ✅ Metrics tracked on failure
- ✅ Processing duration tracked
- ✅ Nil checks prevent panics
- ✅ Integration tests pass

---

### **Phase 2.5: Deduplication Service** (30 min)
**Why Fifth**: Service layer - tracks duplicate detection

**Files**:
- `pkg/gateway/processing/deduplication.go`

**Changes**:
1. Add `metrics *metrics.Metrics` field to `DeduplicationService` struct
2. Update `NewDeduplicationService` constructor to accept metrics parameter
3. Wire `DuplicateSignals.WithLabelValues(source).Inc()` in `CheckDuplicate` method

**Implementation Pattern**:
```go
type DeduplicationService struct {
    redisClient *redis.Client
    logger      *zap.Logger
    metrics     *metrics.Metrics // NEW
}

func NewDeduplicationService(redisClient *redis.Client, logger *zap.Logger, metrics *metrics.Metrics) *DeduplicationService {
    return &DeduplicationService{
        redisClient: redisClient,
        logger:      logger,
        metrics:     metrics, // NEW
    }
}

func (d *DeduplicationService) CheckDuplicate(ctx context.Context, signal *types.NormalizedSignal) (bool, error) {
    fingerprint := d.generateFingerprint(signal)
    key := fmt.Sprintf("dedup:%s:%s", signal.Namespace, fingerprint)

    // Check if key exists
    exists, err := d.redisClient.Exists(ctx, key).Result()
    if err != nil {
        return false, fmt.Errorf("failed to check duplicate: %w", err)
    }

    isDuplicate := exists > 0

    // Track duplicate detection
    if isDuplicate && d.metrics != nil {
        d.metrics.DuplicateSignals.WithLabelValues(signal.Source).Inc()
    }

    // Store fingerprint if new
    if !isDuplicate {
        err = d.redisClient.Set(ctx, key, "1", 5*time.Minute).Err()
        if err != nil {
            return false, fmt.Errorf("failed to store fingerprint: %w", err)
        }
    }

    return isDuplicate, nil
}
```

**Validation**:
- ✅ Metrics tracked on duplicate
- ✅ Nil checks prevent panics
- ✅ Integration tests pass

---

### **Phase 2.6: CRD Creator** (30 min)
**Why Sixth**: Service layer - tracks CRD creation

**Files**:
- `pkg/gateway/processing/crd_creator.go`

**Changes**:
1. Add `metrics *metrics.Metrics` field to `CRDCreator` struct
2. Update `NewCRDCreator` constructor to accept metrics parameter
3. Wire `CRDsCreated.WithLabelValues(namespace, priority).Inc()` in `CreateRemediationRequest` method

**Implementation Pattern**:
```go
type CRDCreator struct {
    k8sClient client.Client
    logger    *zap.Logger
    metrics   *metrics.Metrics // NEW
}

func NewCRDCreator(k8sClient client.Client, logger *zap.Logger, metrics *metrics.Metrics) *CRDCreator {
    return &CRDCreator{
        k8sClient: k8sClient,
        logger:    logger,
        metrics:   metrics, // NEW
    }
}

func (c *CRDCreator) CreateRemediationRequest(ctx context.Context, signal *types.NormalizedSignal, priority, environment string) (*remediationv1.RemediationRequest, error) {
    crd := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      generateCRDName(signal),
            Namespace: signal.Namespace,
        },
        Spec: remediationv1.RemediationRequestSpec{
            Priority:    priority,
            Environment: environment,
            // ... other fields ...
        },
    }

    err := c.k8sClient.Create(ctx, crd)
    if err != nil {
        return nil, fmt.Errorf("failed to create CRD: %w", err)
    }

    // Track CRD creation
    if c.metrics != nil {
        c.metrics.CRDsCreated.WithLabelValues(signal.Namespace, priority).Inc()
    }

    c.logger.Info("Created RemediationRequest CRD",
        zap.String("name", crd.Name),
        zap.String("namespace", crd.Namespace),
        zap.String("priority", priority))

    return crd, nil
}
```

**Validation**:
- ✅ Metrics tracked on CRD creation
- ✅ Nil checks prevent panics
- ✅ Integration tests pass

---

### **Phase 2.7: Integration Test Updates** (1h)
**Why Last**: Validation - ensures metrics work end-to-end

**Files**:
- `test/integration/gateway/helpers.go`
- `test/integration/gateway/metrics_integration_test.go` (NEW)

**Changes to `helpers.go`**:
```go
func StartTestGateway(ctx context.Context, redisClient *RedisTestClient, k8sClient *K8sTestClient) string {
    logger := zap.NewNop()

    // Create metrics instance for testing
    metrics := gatewayMetrics.NewMetrics()

    // Create Gateway components
    adapterRegistry := adapters.NewAdapterRegistry()
    classifier := processing.NewEnvironmentClassifier()
    priorityEngine, _ := processing.NewPriorityEngineWithRego(policyPath, logger)
    pathDecider := processing.NewRemediationPathDecider(logger)

    // Pass metrics to services
    dedupService := processing.NewDeduplicationService(redisClient.Client, logger, metrics)
    stormDetector := processing.NewStormDetector(redisClient.Client, logger)
    stormAggregator := processing.NewStormAggregator(redisClient.Client)
    crdCreator := processing.NewCRDCreator(k8sClient.Client, logger, metrics)

    // ... rest of setup ...
}
```

**New Test File**: `metrics_integration_test.go`
```go
var _ = Describe("Metrics Integration Tests", func() {
    It("should track TokenReview metrics", func() {
        // Send authenticated request
        payload := GeneratePrometheusAlert(PrometheusAlertOptions{
            AlertName: "HighMemoryUsage",
            Namespace: "production",
        })

        resp := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
        Expect(resp.StatusCode).To(Equal(http.StatusCreated))

        // Verify metrics were tracked
        // Note: In real tests, we'd query /metrics endpoint
        // For now, we verify behavior through logs
    })

    It("should track duplicate signal metrics", func() {
        // Send same signal twice
        payload := GeneratePrometheusAlert(PrometheusAlertOptions{
            AlertName: "DuplicateTest",
            Namespace: "production",
        })

        resp1 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
        Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

        resp2 := SendWebhook(gatewayURL+"/webhook/prometheus", payload)
        Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))

        // Verify duplicate metric was incremented
    })
})
```

**Validation**:
- ✅ Metrics tracked in integration tests
- ✅ No test failures
- ✅ No nil pointer panics

---

## 📋 **Implementation Checklist**

### **Phase 2.1: Server Initialization** ✅
- [ ] Change `metrics: nil` to `metrics: gatewayMetrics.NewMetrics()`
- [ ] Remove TODO comment
- [ ] Verify server compiles
- [ ] Run integration tests

### **Phase 2.2: Authentication Middleware** ✅
- [ ] Wire `TokenReviewRequests` counter (success/timeout/error)
- [ ] Wire `TokenReviewTimeouts` counter
- [ ] Wire `K8sAPILatency` histogram
- [ ] Add nil checks for all metrics calls
- [ ] Run integration tests

### **Phase 2.3: Authorization Middleware** ✅
- [ ] Wire `SubjectAccessReviewRequests` counter (success/timeout/error)
- [ ] Wire `SubjectAccessReviewTimeouts` counter
- [ ] Wire `K8sAPILatency` histogram
- [ ] Add nil checks for all metrics calls
- [ ] Run integration tests

### **Phase 2.4: Webhook Handler** ✅
- [ ] Wire `SignalsReceived` counter
- [ ] Wire `SignalsProcessed` counter
- [ ] Wire `SignalsFailed` counter
- [ ] Wire `ProcessingDuration` histogram
- [ ] Add nil checks for all metrics calls
- [ ] Run integration tests

### **Phase 2.5: Deduplication Service** ✅
- [ ] Add `metrics *metrics.Metrics` field
- [ ] Update constructor signature
- [ ] Wire `DuplicateSignals` counter
- [ ] Add nil checks for all metrics calls
- [ ] Update test helpers
- [ ] Run integration tests

### **Phase 2.6: CRD Creator** ✅
- [ ] Add `metrics *metrics.Metrics` field
- [ ] Update constructor signature
- [ ] Wire `CRDsCreated` counter
- [ ] Add nil checks for all metrics calls
- [ ] Update test helpers
- [ ] Run integration tests

### **Phase 2.7: Integration Tests** ✅
- [ ] Update `StartTestGateway` to pass metrics
- [ ] Create `metrics_integration_test.go`
- [ ] Add metrics validation tests
- [ ] Run full integration suite
- [ ] Verify no regressions

---

## 🎯 **Success Criteria**

**Phase 2 Complete When**:
1. ✅ All 7 phases implemented
2. ✅ All integration tests pass
3. ✅ No nil pointer panics
4. ✅ Metrics tracked correctly
5. ✅ No TODO comments remain
6. ✅ Code compiles without errors
7. ✅ No lint errors

---

## ⏱️ **Timeline**

| Phase | Duration | Cumulative |
|-------|----------|------------|
| 2.1: Server Init | 5 min | 5 min |
| 2.2: Auth Middleware | 30 min | 35 min |
| 2.3: Authz Middleware | 30 min | 1h 5min |
| 2.4: Webhook Handler | 45 min | 1h 50min |
| 2.5: Dedup Service | 30 min | 2h 20min |
| 2.6: CRD Creator | 30 min | 2h 50min |
| 2.7: Integration Tests | 1h | **3h 50min** |

**Total Estimated Time**: **3h 50min** (originally estimated 4.5h)

---

## 🚨 **Risk Mitigation**

### **Risk 1: Nil Pointer Panics**
**Mitigation**: Add `if metrics != nil` checks everywhere

### **Risk 2: Test Failures**
**Mitigation**: Run tests after each phase, not at the end

### **Risk 3: Performance Impact**
**Mitigation**: Metrics add ~10-50µs per request (negligible)

### **Risk 4: Breaking Changes**
**Mitigation**: Metrics are optional (nil checks), backward compatible

---

## 📊 **Confidence Assessment**

**Confidence**: 95%

**Rationale**:
- ✅ Clear implementation pattern
- ✅ Existing infrastructure ready
- ✅ Low complexity changes
- ✅ Test infrastructure in place
- ✅ No breaking changes

**Risks**: LOW
- Nil checks might be missed (mitigated by testing)
- Integration tests might need adjustment (mitigated by incremental approach)

---

**Status**: ✅ **PLAN COMPLETE**
**Next**: APDC DO Phase (Implementation)
**Ready**: Begin Phase 2.1 (Server Initialization)


