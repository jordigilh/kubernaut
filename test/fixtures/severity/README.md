# Custom Severity Test Fixtures

This directory contains test fixtures for validating DD-SEVERITY-001 v1.1 custom severity scheme support.

## Purpose

These fixtures enable end-to-end testing of:
- Non-standard severity values (Sev1-4, P0-P4) flowing through Gateway
- SignalProcessing Rego-based severity normalization
- Normalized severity propagation to AIAnalysis and downstream consumers

## Test Fixtures

### Rego Policies

1. **`enterprise-sev-policy.rego`** - Enterprise "Sev1-4" scheme
   - Sev1 → critical
   - Sev2 → high
   - Sev3 → medium
   - Sev4 → low

2. **`pagerduty-p-policy.rego`** - PagerDuty "P0-P4" scheme
   - P0, P1 → critical
   - P2 → high
   - P3 → medium
   - P4 → low

### Prometheus Alerts

1. **`prometheus-alert-sev1.json`** - Production outage with severity="Sev1"
   - Tests: Gateway pass-through of "Sev1" → RemediationRequest
   - Expected: RR.Spec.Severity = "Sev1" (not transformed by Gateway)

2. **`prometheus-alert-p0.json`** - Database outage with severity="P0" (TODO)
   - Tests: PagerDuty priority scheme support
   - Expected: RR.Spec.Severity = "P0" (not transformed by Gateway)

## Usage in Tests

### Unit Tests (SignalProcessing Classifier)

```go
// Load Enterprise policy
policyContent, _ := os.ReadFile("test/fixtures/severity/enterprise-sev-policy.rego")
classifier.LoadRegoPolicy(string(policyContent))

// Test classification
sp := &signalprocessingv1alpha1.SignalProcessing{
    Spec: signalprocessingv1alpha1.SignalProcessingSpec{
        Signal: signalprocessingv1alpha1.SignalData{
            Severity: "Sev1", // External value
        },
    },
}

result, err := classifier.ClassifySeverity(ctx, sp)
Expect(err).ToNot(HaveOccurred())
Expect(result.Severity).To(Equal("critical")) // Normalized value
```

### Integration Tests (Gateway → SP → AA)

```go
// Send Prometheus alert with Sev1
alertPayload, _ := os.ReadFile("test/fixtures/severity/prometheus-alert-sev1.json")
resp := sendToGateway("/webhook/prometheus", alertPayload)

// Verify RemediationRequest has external severity
Eventually(func() string {
    rr := getRemediationRequest("ProductionOutage")
    return rr.Spec.Severity
}).Should(Equal("Sev1")) // Gateway did NOT transform

// Verify SignalProcessing normalized severity
Eventually(func() string {
    sp := getSignalProcessing("ProductionOutage")
    return sp.Status.Severity
}).Should(Equal("critical")) // Rego policy normalized

// Verify AIAnalysis received normalized severity
Eventually(func() string {
    aa := getAIAnalysis("ProductionOutage")
    return aa.Spec.SignalContextInput.Severity
}).Should(Equal("critical")) // Propagated from SP.Status
```

### E2E Tests (Full Pipeline)

```go
It("should handle Enterprise Sev1 severity end-to-end", func() {
    // 1. Deploy Enterprise Rego policy to SP
    applyRegoPolicy("enterprise-sev-policy.rego")

    // 2. Send Prometheus alert
    sendPrometheusAlert("prometheus-alert-sev1.json")

    // 3. Verify full pipeline
    verifyRemediationRequestSeverity("Sev1")           // External
    verifySignalProcessingSeverity("critical")          // Normalized
    verifyAIAnalysisSeverity("critical")                // Normalized
    verifyAuditEventHasBothSeverities("Sev1", "critical") // Dual tracking
})
```

## Expected Behavior (DD-SEVERITY-001 v1.1)

### Gateway (Week 3 - Pending)
```
Input:  Prometheus alert with severity="Sev1"
Output: RemediationRequest.Spec.Severity = "Sev1"
Rule:   Gateway passes through raw severity (NO transformation)
```

### SignalProcessing (Week 2 - Complete)
```
Input:  SignalProcessing.Spec.Signal.Severity = "Sev1"
Rego:   enterprise-sev-policy.rego maps "Sev1" → "critical"
Output: SignalProcessing.Status.Severity = "critical"
```

### AIAnalysis (Week 4 - Complete)
```
Input:  SignalProcessing.Status.Severity = "critical"
Output: AIAnalysis.Spec.SignalContextInput.Severity = "critical"
Rule:   Uses normalized severity from SP.Status (not external RR.Spec)
```

## Validation Checklist

- [ ] Gateway accepts "Sev1" without transformation (Week 3 pending)
- [ ] Gateway accepts "P0" without transformation (Week 3 pending)
- [x] SignalProcessing Rego normalizes "Sev1" → "critical"
- [x] SignalProcessing Rego normalizes "P0" → "critical"
- [x] AIAnalysis receives normalized severity from SP.Status
- [x] Audit events include both external + normalized severity
- [ ] E2E test covers full "Sev1" → "critical" pipeline (pending GW)
- [ ] E2E test covers full "P0" → "critical" pipeline (pending GW)

## Related Documentation

- [DD-SEVERITY-001 v1.1](../../../docs/architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
- [SignalProcessing Controller](../../../docs/services/crd-controllers/01-signalprocessing/)
- [Gateway Service](../../../docs/services/stateless/gateway-service/)
