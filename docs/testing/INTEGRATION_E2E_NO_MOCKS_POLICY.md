# Integration and E2E Testing: NO MOCKS Policy

## 🚨 **MANDATORY POLICY: ZERO MOCKS IN INTEGRATION AND E2E TESTS**

### **Core Principle**

**Integration and End-to-End tests MUST use REAL business components and REAL business logic. NO MOCKS are permitted.**

This policy ensures that integration and E2E tests validate actual system behavior, not mock behavior.

---

## **What This Means**

### ✅ **REQUIRED: Use Real Components**

**Integration Tests MUST use:**
- ✅ **Real webhook handlers** (`pkg/integration/webhook/handler.go`)
- ✅ **Real alert processors** (`pkg/integration/processor/processor.go`)
- ✅ **Real Kubernetes clients** connecting to actual Kind clusters
- ✅ **Real database connections** to PostgreSQL with real schemas
- ✅ **Real business logic** for alert processing, filtering, and execution
- ✅ **Real HTTP servers** handling actual AlertManager webhooks
- ✅ **Real configuration loading** from actual config files
- ✅ **Real error handling** and logging systems

**E2E Tests MUST use:**
- ✅ **Complete real workflows** from alert ingestion to action execution
- ✅ **Real monitoring stack** (Prometheus, AlertManager) in Kind cluster
- ✅ **Real Kubernaut application** deployed and running
- ✅ **Real alert routing** through actual AlertManager configuration
- ✅ **Real Kubernetes operations** executed against actual cluster resources

### ❌ **FORBIDDEN: Mock Components**

**Integration and E2E Tests MUST NOT use:**
- ❌ **Mock processors** that simulate alert processing
- ❌ **Mock webhook handlers** that return fake responses
- ❌ **Mock Kubernetes clients** or fake cluster operations
- ❌ **Mock databases** or in-memory storage substitutes
- ❌ **Mock HTTP servers** that don't use real business logic
- ❌ **Stub implementations** that bypass business logic
- ❌ **Test doubles** that replace core business components

---

## **Acceptable External Service Configuration**

### **The ONLY Exception: External Service Endpoints**

The only acceptable "configuration" (not mocking) is pointing external services to integration-appropriate endpoints:

#### ✅ **Acceptable External Service Configuration:**
```go
// ✅ CORRECT: Configure LLM to use integration test endpoint
llmConfig := llm.Config{
    Provider: "localai",
    Endpoint: "http://localhost:8080",  // Integration test LLM endpoint
    Model:    "granite-3.0-8b-instruct",
    // ... real configuration
}

// ✅ CORRECT: Use real LLM client with integration endpoint
llmClient, err := llm.NewClient(llmConfig, log)
```

#### ❌ **FORBIDDEN: Mock LLM Client:**
```go
// ❌ WRONG: Mock LLM client that doesn't use real business logic
type MockLLMClient struct{}
func (m *MockLLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.LLMResponse, error) {
    return &types.LLMResponse{Action: "fake"}, nil  // FORBIDDEN
}
```

### **External Services vs Business Logic**

| Component | Status | Approach |
|-----------|--------|----------|
| **Webhook Handler** | ❌ **NEVER MOCK** | Use real `pkg/integration/webhook/handler.go` |
| **Alert Processor** | ❌ **NEVER MOCK** | Use real `pkg/integration/processor/processor.go` |
| **Kubernetes Client** | ❌ **NEVER MOCK** | Use real client connected to Kind cluster |
| **Database** | ❌ **NEVER MOCK** | Use real PostgreSQL with real schema |
| **LLM Service** | ✅ **Configure endpoint** | Point to integration LLM service |
| **External APIs** | ✅ **Configure endpoint** | Point to test endpoints if needed |

---

## **Implementation Examples**

### **✅ CORRECT Integration Test Setup**

```go
// ✅ CORRECT: Real components with real business logic
func setupIntegrationTest(t *testing.T) (*webhook.Handler, *processor.Processor) {
    // Real database connection
    db, err := database.NewConnection(integrationDBConfig)
    require.NoError(t, err)

    // Real action history repository
    actionRepo := actionhistory.NewRepository(db, log)

    // Real Kubernetes client
    k8sClient := k8s.NewClient(clientset, log)

    // Real executor with real K8s operations
    executor := executor.NewExecutor(k8sClient, executorConfig, log)

    // Real LLM client (configured for integration endpoint)
    llmClient, err := llm.NewClient(integrationLLMConfig, log)
    require.NoError(t, err)

    // Real alert processor with ALL real components
    processor := processor.NewProcessor(llmClient, executor, filters, actionRepo, log)

    // Real webhook handler using real processor
    handler := webhook.NewHandler(processor, webhookConfig, log)

    return handler, processor
}

func TestRealAlertProcessing(t *testing.T) {
    handler, processor := setupIntegrationTest(t)

    // Test with real AlertManager webhook payload
    alertPayload := `{
        "alerts": [{
            "status": "firing",
            "labels": {"alertname": "HighMemoryUsage", "severity": "critical"},
            "annotations": {"description": "Memory usage is above 90%"}
        }]
    }`

    // Send to REAL webhook handler
    req := httptest.NewRequest("POST", "/alerts", strings.NewReader(alertPayload))
    req.Header.Set("Content-Type", "application/json")

    recorder := httptest.NewRecorder()
    handler.HandleAlert(recorder, req)

    // Verify REAL business logic executed
    assert.Equal(t, http.StatusOK, recorder.Code)

    // Verify REAL database operations occurred
    actions, err := actionRepo.GetRecentActions(context.Background(), 1)
    require.NoError(t, err)
    assert.Len(t, actions, 1)
    assert.Equal(t, "HighMemoryUsage", actions[0].AlertName)
}
```

### **❌ FORBIDDEN Mock-Based Test**

```go
// ❌ WRONG: Mock processor that bypasses business logic
type MockProcessor struct{}
func (m *MockProcessor) ProcessAlert(ctx context.Context, alert types.Alert) error {
    // This bypasses ALL real business logic - FORBIDDEN
    return nil
}

func TestWithMocks(t *testing.T) {  // ❌ FORBIDDEN
    mockProcessor := &MockProcessor{}
    handler := webhook.NewHandler(mockProcessor, config, log)
    // This test validates nothing about real business behavior
}
```

---

## **E2E Test Requirements**

### **Complete Real System Testing**

E2E tests MUST validate the complete real system:

```go
func TestCompleteAlertWorkflow(t *testing.T) {
    // 1. REAL Prometheus generates alert
    // 2. REAL AlertManager receives alert
    // 3. REAL AlertManager sends webhook to REAL Kubernaut
    // 4. REAL Kubernaut processes alert with REAL business logic
    // 5. REAL Kubernaut executes REAL Kubernetes operations
    // 6. REAL database stores REAL action history

    // Deploy REAL alert rule to Prometheus
    alertRule := `
    groups:
    - name: integration-test
      rules:
      - alert: TestAlert
        expr: up == 0
        labels:
          severity: critical
    `

    // Apply to REAL Prometheus in Kind cluster
    err := applyPrometheusRule(alertRule)
    require.NoError(t, err)

    // Wait for REAL AlertManager to send webhook to REAL Kubernaut
    time.Sleep(30 * time.Second)

    // Verify REAL action was executed in REAL cluster
    pods, err := k8sClient.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
    require.NoError(t, err)

    // Verify REAL database recorded REAL action
    actions, err := actionRepo.GetActionsByAlert(context.Background(), "TestAlert")
    require.NoError(t, err)
    assert.NotEmpty(t, actions)
}
```

---

## **Validation and Enforcement**

### **Pre-commit Validation**

```bash
#!/bin/bash
# Validate no mocks in integration/e2e tests

echo "🔍 Validating NO MOCKS policy in integration and E2E tests..."

# Check for forbidden mock patterns
MOCK_VIOLATIONS=$(grep -r "Mock\|Stub\|Fake" test/integration/ test/e2e/ --include="*.go" | grep -v "// ✅" || true)

if [ -n "$MOCK_VIOLATIONS" ]; then
    echo "❌ POLICY VIOLATION: Mocks found in integration/e2e tests:"
    echo "$MOCK_VIOLATIONS"
    echo ""
    echo "Integration and E2E tests MUST use REAL business components."
    echo "See docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md"
    exit 1
fi

# Check for real component usage
REAL_COMPONENTS=$(grep -r "NewProcessor\|NewHandler\|NewClient\|NewExecutor" test/integration/ test/e2e/ --include="*.go" | wc -l)

if [ "$REAL_COMPONENTS" -eq 0 ]; then
    echo "❌ POLICY VIOLATION: No real business components found in tests"
    echo "Integration and E2E tests MUST use real business logic."
    exit 1
fi

echo "✅ NO MOCKS policy validation passed"
```

### **Code Review Checklist**

**For Integration Tests:**
- [ ] Uses real `webhook.NewHandler()` with real processor
- [ ] Uses real `processor.NewProcessor()` with real dependencies
- [ ] Connects to real database (PostgreSQL)
- [ ] Uses real Kubernetes client connected to Kind cluster
- [ ] No mock, stub, or fake implementations
- [ ] External services configured to integration endpoints (not mocked)

**For E2E Tests:**
- [ ] Tests complete workflow from alert generation to action execution
- [ ] Uses real Prometheus and AlertManager in Kind cluster
- [ ] Real Kubernaut application deployed and receiving webhooks
- [ ] Real Kubernetes operations executed against cluster
- [ ] Real database operations and persistence
- [ ] No simulation or mock of any business components

---

## **Benefits of NO MOCKS Policy**

### **Real System Validation**
- ✅ Tests validate actual business behavior
- ✅ Catches real integration issues
- ✅ Validates real performance characteristics
- ✅ Tests real error handling and edge cases

### **Confidence in Deployments**
- ✅ Integration tests prove system works end-to-end
- ✅ E2E tests validate complete user workflows
- ✅ Real database operations tested
- ✅ Real Kubernetes operations validated

### **Prevents Mock Drift**
- ✅ No risk of mocks becoming outdated
- ✅ No false confidence from passing mock tests
- ✅ Real business logic changes immediately affect tests

---

## **Summary**

**ZERO TOLERANCE for mocks in integration and E2E tests.**

- **Integration Tests**: Use real business components with real business logic
- **E2E Tests**: Test complete real workflows in real environments
- **External Services**: Configure endpoints, don't mock the services
- **Validation**: Automated checks prevent mock usage
- **Benefits**: Real system validation and deployment confidence

**This policy is NON-NEGOTIABLE and ensures our integration and E2E tests provide genuine validation of system behavior.**
