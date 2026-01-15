# Example: Fully Updated Test Scenario

## Before (Current Format)

```markdown
#### **Scenario 1.1: Signal Received Audit Event**
**BR**: BR-GATEWAY-055
**Priority**: P0 (Critical)
**Business Value**: SOC2 compliance - every signal must be auditable

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    // Test 1.1.1
    It("should emit gateway.signal.received audit event for Prometheus signal", func() {
        // Test implementation...
    })
})
```
```

---

## After (With Test ID + Metadata)

```markdown
#### **Test Case: GW-INT-AUD-001**

**Test ID**: GW-INT-AUD-001  
**Test Name**: Prometheus Signal Audit Event Emission  
**BR**: BR-GATEWAY-055  
**Priority**: P0 (Critical)  
**Category**: Audit Event Emission  
**Tags**: audit, prometheus, rr-reconstruction, soc2

---

**What's Being Tested**:
Gateway's ability to emit SOC2-compliant audit events when receiving Prometheus alerts, ensuring all fields required for RemediationRequest reconstruction are captured.

**Business Value**:
- SOC2 Compliance: Audit trail for all signal ingestion
- Operational Debugging: Complete signal reconstruction capability
- Traceability: Correlation ID enables end-to-end tracking

**Expected Outcome**:
When a Prometheus alert is parsed, a `gateway.signal.received` audit event is emitted with GatewayAuditPayload containing `signal_labels`, `signal_annotations`, `original_payload`, and properly formatted `correlation_id`.

---

**Fixtures**:
- **Type**: Prometheus alert with high CPU usage
- **Required Fields** (for these tests):
  - `alertname`: Any value (presence validation)
  - `labels.severity`: "critical" (business rule validation)
  - `labels.team`, `labels.environment`: Custom values (preservation test)
  - `annotations.summary`, `annotations.runbook_url`: Non-empty (presence test)
- **Reference**: `Use PrometheusAlertBuilder().WithSeverity("critical").WithLabels(...)` OR `test/fixtures/prometheus_high_cpu_alert.json`

**Dependencies**:
- Services: Prometheus adapter, MockAuditStore
- Infrastructure: None (in-memory mocks)
- Related Tests: GW-INT-AUD-002 (K8s Event - same pattern)

---

**Test Specifications**:

```go
var _ = Describe("BR-GATEWAY-055: Signal Received Audit Events", func() {
    var (
        adapter       adapters.SignalAdapter
        auditStore    *MockAuditStore
        ctx           context.Context
    )

    BeforeEach(func() {
        auditStore = NewMockAuditStore()
        ctx = context.Background()
    })

    // Test ID: GW-INT-AUD-001
    It("[GW-INT-AUD-001] should emit gateway.signal.received audit event for Prometheus signal", func() {
        // Given: Prometheus alert payload
        prometheusAlert := createTestPrometheusAlert()
        adapter = prometheus.NewAdapter(auditStore)

        // When: Adapter parses signal
        signal, err := adapter.Parse(ctx, prometheusAlert)

        // Then: Audit event emitted
        Expect(err).ToNot(HaveOccurred())
        Expect(auditStore.Events).To(HaveLen(1))

        auditEvent := auditStore.Events[0]
        Expect(auditEvent.EventType).To(Equal("gateway.signal.received"))
        Expect(auditEvent.EventAction).To(Equal("received"))
        Expect(auditEvent.CorrelationID).ToNot(BeEmpty())
        
        // Parse EventData to get GatewayAuditPayload
        gatewayPayload := auditEvent.EventData.GatewayAuditPayload
        
        // Access OriginalPayload (Optional field)
        originalPayload, ok := gatewayPayload.OriginalPayload.Get()
        Expect(ok).To(BeTrue(), "OriginalPayload should be present")
        payloadStr := fmt.Sprintf("%v", originalPayload)
        Expect(payloadStr).To(ContainSubstring("alertname"))
        
        // Access SignalLabels (Optional field)
        signalLabels, ok := gatewayPayload.SignalLabels.Get()
        Expect(ok).To(BeTrue(), "SignalLabels should be present")
        Expect(signalLabels).To(HaveKey("severity"))
        
        // Access SignalAnnotations (Optional field)
        signalAnnotations, ok := gatewayPayload.SignalAnnotations.Get()
        Expect(ok).To(BeTrue(), "SignalAnnotations should be present")
        Expect(signalAnnotations).To(HaveKey("summary"))
    })

    // Test ID: GW-INT-AUD-002
    It("[GW-INT-AUD-002] should emit gateway.signal.received audit event for K8s Event signal", func() {
        // Test implementation...
    })

    // Test ID: GW-INT-AUD-003
    It("[GW-INT-AUD-003] should include correlation_id in audit event for tracing", func() {
        // Test implementation...
    })

    // Test ID: GW-INT-AUD-004
    It("[GW-INT-AUD-004] should preserve signal_labels and signal_annotations in audit event", func() {
        // Test implementation...
    })

    // Test ID: GW-INT-AUD-005
    It("[GW-INT-AUD-005] should not block signal processing if audit emission fails", func() {
        // Test implementation...
    })
})
```

**Implementation Guidance**:
- Use helper: `ParseGatewayPayload(event)` - Extracts GatewayAuditPayload
- Use helper: `ExpectSignalLabels(payload, expected)` - Validates label map
- Use helper: `ExpectCorrelationIDFormat(correlationID)` - Validates format
- Reference: `test/e2e/gateway/23_audit_emission_test.go` - E2E example

**Common Pitfalls**:
```go
// ❌ BAD: Accessing undefined fields
auditEvent.SignalLabels  // Field doesn't exist at top level

// ✅ GOOD: Using OpenAPI structures
gatewayPayload := auditEvent.EventData.GatewayAuditPayload
signalLabels, ok := gatewayPayload.SignalLabels.Get()
```
```

---

## Key Changes

### 1. Test Case Header
```markdown
#### **Test Case: GW-INT-AUD-001**

**Test ID**: GW-INT-AUD-001
**Test Name**: Prometheus Signal Audit Event Emission
```

### 2. Metadata Section
```markdown
**What's Being Tested**: [One sentence]
**Business Value**: [Bullet points]
**Expected Outcome**: [Clear outcome description]
```

### 3. Fixtures Section
```markdown
**Fixtures**:
- **Type**: Prometheus alert with high CPU usage
- **Required Fields** (for these tests): [Only fields validated]
- **Reference**: Builder or fixture file path
```

### 4. Test IDs in Descriptions
```go
It("[GW-INT-AUD-001] should emit gateway.signal.received...", func() {
```

### 5. Keep All Implementation Code
- All existing code preserved (needed for Phase 2 implementation)
- Helper function references added
- Pitfalls section added

---

## Benefits

✅ **Test ID Visible in Output**:
```
Running Suite: Gateway Integration Test Suite
  [GW-INT-AUD-001] should emit gateway.signal.received audit event ✓
  [GW-INT-AUD-002] should emit gateway.signal.received audit event for K8s Event ✓
```

✅ **Grep-able**:
```bash
grep "GW-INT-AUD-001" test/integration/gateway/
```

✅ **Business Context Clear**:
- What's being tested
- Why it matters
- Expected outcomes

✅ **Stable Specification**:
- Fixture requirements conceptual
- Implementation code can evolve
- Business requirements stable

---

## Questions for You

1. **Format Approval**: Does this format meet your requirements?
2. **Fixture Detail**: Is the "Required Fields" approach sufficient?
3. **Full Update**: Should I proceed to update all 77 test scenarios with this format?
   - Estimated time: 3-4 hours
   - 77 scenarios × 5 tests/scenario = ~385 test IDs to add
4. **Phased Approach**: Would you prefer:
   - **Option A**: Update all 77 scenarios now (complete but long)
   - **Option B**: Update Phase 1 scenarios only (20 tests) as proof-of-concept
   - **Option C**: Keep current format, add metadata during implementation (Phase 2)
