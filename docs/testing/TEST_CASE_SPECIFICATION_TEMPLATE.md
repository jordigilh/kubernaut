# Test Case Specification Template
**Version**: 1.0.0  
**Date**: January 14, 2026  
**Purpose**: Industry-standard template for writing stable, maintainable test specifications  
**Based On**: IEEE 829, ISO/IEC/IEEE 29119

---

## 🎯 Overview

This template provides a **business-focused, implementation-independent** format for test case specifications. Test specifications should remain stable even as implementation code changes.

### Key Principles

1. **Test ID in Description**: Use `[SERVICE-TIER-CATEGORY-NNN]` format in `It()` descriptions
2. **Business Focus**: Describe what's being tested and expected outcomes, not how
3. **Stable Fixtures**: Describe fixture requirements conceptually, not exact JSON
4. **Implementation-Independent**: Code can change, but test specifications stay stable

---

## 📋 Template Structure

### Test Case Format

```markdown
#### Test Case: [SERVICE]-[TIER]-[CATEGORY]-[NUMBER]

**Test ID**: [SERVICE]-[TIER]-[CATEGORY]-[NUMBER]
**Test Name**: [Clear, descriptive name]
**Category**: [Audit, Metrics, Adapters, Error Handling, etc.]
**Priority**: P0 (Critical) | P1 (High) | P2 (Medium) | P3 (Low)
**BR Reference**: BR-[CATEGORY]-[NUMBER]
**Tags**: [audit, soc2, prometheus, integration, etc.]

---

**What's Being Tested**:
[One clear sentence describing the functionality or behavior being validated]

**Business Value**:
[Why this test matters: SOC2 compliance, operational visibility, reliability, etc.]

**Description**:
[2-3 sentences providing context about test purpose, scope, and business importance]

---

**Preconditions**:
- [System state required before test execution]
- [Services that must be running]
- [Configuration or data setup requirements]
- [Infrastructure dependencies]

**Test Data / Fixtures**:
Fixture Type: [Conceptual description of test data]

Required Fields (for this test):
- `field_name`: [Expected value, range, or constraint that matters for THIS test]
- `field_name`: [Pattern or validation rule]
- `field_name`: [Business rule validation]

Fixture Reference: `test/fixtures/[fixture_file_name]` OR `Use [FixtureBuilderName]()`

**Notes on Fixtures**:
- Only list fields that are **validated by this specific test**
- Fixture files may contain additional fields (evolution flexibility)
- Prefer fixture builders over static files for robustness

---

**Test Steps**:
1. [Given: Initial state or setup]
2. [When: Action or event triggering behavior]
3. [Then: Observable outcome]
4. [Verify: Specific validations]

**Expected Results**:
1. [Observable outcome 1 with measurable criteria]
2. [Observable outcome 2 with specific values]
3. [Field validations with business rules]
4. [Performance or quality attributes]

**Acceptance Criteria**:
- ✅ [Specific, measurable criterion 1]
- ✅ [Specific, measurable criterion 2]
- ✅ [Business rule validation]
- ✅ [Non-functional requirement (performance, security, etc.)]

---

**Dependencies**:
- Test IDs: [Other test cases this depends on]
- Services: [External services required]
- Infrastructure: [Database, message queue, etc.]

**Related Tests**:
- `[TEST-ID]`: [How they relate - similar scenario, opposite case, etc.]
- `[TEST-ID]`: [Relationship description]

---

**Implementation Guidance** (Optional):
- Use helper: `[FunctionName()]` - [Purpose]
- Validate: [Key fields to check]
- Reference: `[path/to/example_test.go]` - [What to reference]

**Common Pitfalls** (Optional):
```go
// ❌ BAD: [Anti-pattern description]
badPattern()

// ✅ GOOD: [Correct pattern description]
goodPattern()
```

**Notes**:
[Additional context, edge cases, known issues, or historical information]
```

---

## 🏷️ Test ID Format

### Structure
```
[SERVICE]-[TIER]-[CATEGORY]-[NUMBER]
```

### Components

| Component | Description | Examples |
|-----------|-------------|----------|
| **SERVICE** | Service abbreviation (2-3 chars) | `GW`, `SP`, `RO`, `DS`, `AA`, `NO` |
| **TIER** | Test tier | `UNIT`, `INT`, `E2E` |
| **CATEGORY** | Functional area (3 chars) | `AUD`, `MET`, `ADP`, `ERR`, `CFG` |
| **NUMBER** | Sequential number (001-999) | `001`, `042`, `999` |

### Examples

| Test ID | Meaning |
|---------|---------|
| `GW-INT-AUD-001` | Gateway Integration test, Audit category, test #1 |
| `SP-UNIT-SEV-023` | SignalProcessing Unit test, Severity category, test #23 |
| `RO-E2E-WFL-005` | RemediationOrchestrator E2E test, Workflow category, test #5 |

### Service Abbreviations

| Abbreviation | Service |
|--------------|---------|
| `GW` | Gateway |
| `SP` | SignalProcessing |
| `RO` | RemediationOrchestrator |
| `DS` | DataStorage |
| `AA` | AIAnalysis |
| `NO` | Notification |
| `WE` | WorkflowExecution |

### Common Categories

| Category | Full Name | Typical Use Cases |
|----------|-----------|-------------------|
| `AUD` | Audit | Audit event emission, SOC2 compliance |
| `MET` | Metrics | Prometheus metrics, observability |
| `ADP` | Adapters | Signal parsing, external integrations |
| `ERR` | Error | Error handling, retry logic, circuit breakers |
| `CFG` | Config | Configuration validation, dynamic updates |
| `MID` | Middleware | Middleware chain, request processing |
| `WFL` | Workflow | Workflow orchestration, state machines |
| `SEV` | Severity | Severity determination, classification |
| `VAL` | Validation | Input validation, schema validation |

---

## 📝 Complete Example

### Example 1: Gateway Audit Event Test

```markdown
#### Test Case: GW-INT-AUD-001

**Test ID**: GW-INT-AUD-001
**Test Name**: Prometheus Signal Audit Event Emission
**Category**: Audit Event Emission
**Priority**: P0 (Critical)
**BR Reference**: BR-GATEWAY-055
**Tags**: audit, prometheus, rr-reconstruction, soc2

---

**What's Being Tested**:
Gateway's ability to emit SOC2-compliant audit events when receiving Prometheus alerts, ensuring all fields required for RemediationRequest reconstruction are captured.

**Business Value**:
- SOC2 Compliance: Audit trail for all signal ingestion
- Operational Debugging: Complete signal reconstruction capability
- Traceability: Correlation ID enables end-to-end tracking

**Description**:
When a Prometheus alert is parsed by the Gateway adapter, a `gateway.signal.received` audit event must be emitted containing all fields necessary to reconstruct the RemediationRequest. This includes the original payload, signal labels, annotations, and a unique correlation ID that enables tracing the signal through the entire remediation lifecycle.

---

**Preconditions**:
- Gateway service running with audit store enabled
- Prometheus adapter configured and initialized
- MockAuditStore collecting events in-memory

**Test Data / Fixtures**:
Fixture Type: Prometheus Alert (High CPU Usage)

Required Fields (for this test):
- `alertname`: Any value (presence validation)
- `labels.severity`: "critical" (business rule: alerts must have severity)
- `labels.team`: Any value (validates custom labels preserved)
- `annotations.summary`: Non-empty string (validates annotations preserved)

Fixture Reference: `Use PrometheusAlertBuilder().WithSeverity("critical").WithLabels(...)` OR `test/fixtures/prometheus_high_cpu_alert.json`

**Notes on Fixtures**:
- This test validates label/annotation preservation, not specific alert content
- Fixture file may evolve to include additional Prometheus fields
- Test only asserts on fields it explicitly validates

---

**Test Steps**:
1. **Given**: Prometheus alert payload with custom labels and annotations
2. **When**: Prometheus adapter parses the alert
3. **Then**: Audit event is emitted
4. **Verify**: 
   - Event type is `gateway.signal.received`
   - GatewayAuditPayload contains all RR reconstruction fields
   - Correlation ID follows format `rr-{12hex}-{10digit}`

**Expected Results**:
1. Parse operation succeeds without error
2. Exactly 1 audit event emitted to audit store
3. Event has type `gateway.signal.received` and action `received`
4. GatewayAuditPayload contains:
   - `signal_labels`: Map containing all alert labels (including custom ones)
   - `signal_annotations`: Map containing all alert annotations
   - `original_payload`: Full alert JSON for RR reconstruction
   - `signal_type`: "prometheus-alert"
5. Correlation ID matches regex `^rr-[a-f0-9]{12}-\d{10}$`
6. Correlation ID enables fingerprint extraction (first 12 hex chars)

**Acceptance Criteria**:
- ✅ All RR reconstruction fields present and non-empty
- ✅ Custom labels preserved (team, environment, etc.)
- ✅ Custom annotations preserved (runbook_url, dashboard, etc.)
- ✅ Correlation ID format enables downstream RR naming
- ✅ No PII (Personally Identifiable Information) in audit payload
- ✅ Audit emission does not block signal processing (< 10ms overhead)

---

**Dependencies**:
- Test IDs: None (foundational test)
- Services: Prometheus adapter, audit store
- Infrastructure: None (uses in-memory mock)

**Related Tests**:
- `GW-INT-AUD-002`: K8s Event signal audit (same pattern, different source)
- `GW-INT-AUD-003`: Correlation ID format validation (deeper validation)
- `GW-INT-AUD-004`: Signal labels preservation (focused on edge cases)

---

**Implementation Guidance**:
- Use helper: `ParseGatewayPayload(event)` - Extracts GatewayAuditPayload from EventData
- Use helper: `ExpectSignalLabels(payload, expected)` - Validates label map
- Use helper: `ExpectCorrelationIDFormat(correlationID)` - Validates regex pattern
- Reference: `test/e2e/gateway/23_audit_emission_test.go` - E2E example with real K8s

**Common Pitfalls**:
```go
// ❌ BAD: Accessing audit fields directly (unstructured data)
auditEvent.SignalLabels  // Field doesn't exist at top level
auditEvent.Metadata["signal_labels"]  // Unstructured access (DD-AUDIT-004 violation)

// ✅ GOOD: Using OpenAPI structures
gatewayPayload := auditEvent.EventData.GatewayAuditPayload
signalLabels, ok := gatewayPayload.SignalLabels.Get()
Expect(ok).To(BeTrue(), "SignalLabels should be present")
```

**Notes**:
- This test focuses on happy path (successful parsing)
- Error cases (malformed alerts) covered in GW-INT-AUD-005
- Performance benchmark: Audit emission adds < 10ms to signal processing
```

---

## 🎓 Best Practices

### DO ✅

1. **Use Test IDs in `It()` Descriptions**:
   ```go
   It("[GW-INT-AUD-001] should emit gateway.signal.received audit event", func() {
       // Test ID visible in output and reports
   })
   ```

2. **Focus on Business Outcomes**:
   ```markdown
   **What's Being Tested**: Gateway emits SOC2-compliant audit events
   **NOT**: "Gateway calls auditStore.Emit() with correct parameters"
   ```

3. **Describe Fixtures Conceptually**:
   ```markdown
   Required Fields:
   - severity: "critical" (business rule validation)
   **NOT**: Full JSON payload that becomes outdated
   ```

4. **Include Business Value**:
   ```markdown
   **Business Value**: SOC2 compliance for audit trail
   **NOT**: "Tests that code works correctly"
   ```

5. **Use Stable References**:
   ```markdown
   Fixture Reference: `Use PrometheusAlertBuilder()`
   **NOT**: Hardcoded JSON that breaks on schema changes
   ```

### DON'T ❌

1. **Don't Put Implementation Code in Specs**:
   - ❌ Full test functions with `Expect()` statements
   - ✅ Implementation guidance with helper references

2. **Don't Hardcode Full Fixtures**:
   - ❌ 100-line JSON payloads in spec
   - ✅ Required fields + fixture builder reference

3. **Don't Describe Implementation Details**:
   - ❌ "Should call repository.Save() with transaction"
   - ✅ "Should persist data atomically with rollback on error"

4. **Don't Make Specs Brittle**:
   - ❌ "Field X must be exactly 'value123'"
   - ✅ "Field X must match pattern ^value\\d+$"

5. **Don't Skip Business Context**:
   - ❌ Only technical acceptance criteria
   - ✅ Business value + acceptance criteria

---

## 📊 Test Case Registry Format

### Registry Table

```markdown
## Test Case Registry

| Test ID | Test Name | Category | BR | Priority | Status | Section |
|---------|-----------|----------|-----|----------|--------|---------|
| GW-INT-AUD-001 | Prometheus Signal Audit | Audit | 055 | P0 | 📝 Spec | 1.1.1 |
| GW-INT-AUD-002 | K8s Event Signal Audit | Audit | 055 | P0 | 📝 Spec | 1.1.2 |
| GW-INT-AUD-003 | Correlation ID Format | Audit | 055 | P0 | ✅ Pass | 1.1.3 |
| GW-INT-AUD-004 | Signal Labels Preservation | Audit | 055 | P0 | 🚧 Dev | 1.1.4 |

### Status Legend
- 📝 **Spec**: Specification complete, implementation pending
- 🚧 **Dev**: Implementation in progress
- ✅ **Pass**: Implemented and passing
- ❌ **Fail**: Implemented but failing
- ⏸️ **Hold**: Blocked or on hold
- 🗑️ **Deprecated**: No longer needed
```

### Category Summary

```markdown
### Category Breakdown

| Category | Count | Test ID Range | Priority Distribution |
|----------|-------|---------------|----------------------|
| **AUD** (Audit) | 20 | GW-INT-AUD-001 to 020 | P0: 15, P1: 5 |
| **MET** (Metrics) | 15 | GW-INT-MET-001 to 015 | P0: 10, P1: 5 |
| **ADP** (Adapters) | 15 | GW-INT-ADP-001 to 015 | P0: 8, P1: 7 |
```

---

## 🔗 Related Documents

- [PODMAN_INTEGRATION_TEST_TEMPLATE.md](./PODMAN_INTEGRATION_TEST_TEMPLATE.md) - Infrastructure setup for Podman tests
- [KIND_CLUSTER_TEST_TEMPLATE.md](./KIND_CLUSTER_TEST_TEMPLATE.md) - Infrastructure setup for Kind tests
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Overall testing strategy
- [INTEGRATION_E2E_NO_MOCKS_POLICY.md](./INTEGRATION_E2E_NO_MOCKS_POLICY.md) - Testing philosophy

---

## 📚 References

- **IEEE 829-2008**: Standard for Software and System Test Documentation
- **ISO/IEC/IEEE 29119**: Software Testing Standards
- **ISTQB Foundation**: Test case design techniques
- **BDD (Behavior-Driven Development)**: Given-When-Then specification format

---

**Version**: 1.0.0  
**Status**: ✅ READY FOR USE  
**Created**: January 14, 2026  
**Use For**: Test case specifications (not infrastructure setup)
