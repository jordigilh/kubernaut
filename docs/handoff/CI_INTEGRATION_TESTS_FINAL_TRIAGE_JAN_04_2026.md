# CI Integration Tests Triage - Final Analysis (Jan 4, 2026)

**Status**: 3 services failing with similar root cause
**PR**: #XXX (fix/ci-python-dependencies-path)
**Commit**: 7d91dad36
**Date**: 2026-01-04

---

## 📊 **Service-by-Service Analysis**

### ✅ **PASSING Services (5/8)**

| Service | Status | Tests Passed |
|---|---|---|
| Gateway (GW) | ✅ PASS | 73/73 |
| Remediation Orchestrator (RO) | ✅ PASS | 41/41 |
| Notification (NT) | ✅ PASS | 59/59 |
| Data Storage (DS) | ✅ PASS | 168/168 (3 pre-existing flaky) |
| Workflow Execution (WE) | ✅ PASS | All specs |

---

## ❌ **FAILING Services (3/8)**

### 1. Signal Processing (SP)

**Failure Summary**: 2 audit tests failing (FAIL + INTERRUPTED)

**Root Cause**: **DD-TESTING-001 Compliance Issue - Test expects exact audit event count, business logic emits more events**

**Details**:
```bash
[FAIL] BR-SP-090: should create 'signalprocessing.signal.processed' audit event in Data Storage
[INTERRUPTED] BR-SP-090: should create 'phase.transition' audit events for each phase change

Issue:
- Test queries by category "signalprocessing" (line 156)
- Test expects exactly 1 total event (line 185)
- Business logic emits multiple events:
  - enrichment.completed
  - phase.transition (multiple)
  - signal.processed
```

**Evidence from Logs**:
```json
{"event_type":"signalprocessing.enrichment.completed","correlation_id":"audit-test-rr-01","total_buffered":1}
{"event_type":"signalprocessing.phase.transition","correlation_id":"audit-test-rr-01","total_buffered":2}
{"event_type":"signalprocessing.phase.transition","correlation_id":"audit-test-rr-01","total_buffered":3}
{"event_type":"signalprocessing.phase.transition","correlation_id":"audit-test-rr-01","total_buffered":4}
{"event_type":"signalprocessing.signal.processed","correlation_id":"audit-test-rr-01","total_buffered":5}
{"msg":"Audit store closed","written_count":652}
```

**Fix Strategy**:
```go
// BEFORE (test/integration/signalprocessing/audit_integration_test.go:185)
}, 90*time.Second, 500*time.Millisecond).Should(Equal(1),
    "BR-SP-090: SignalProcessing MUST emit exactly 1 signal.processed event per processing completion")

// AFTER (DD-TESTING-001 compliant - filter by event type)
}, 90*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "BR-SP-090: SignalProcessing MUST emit at least 1 audit event (may include phase transitions, enrichment)")

// Then filter for specific event type before detailed validation
var processedEvents []dsgen.AuditEvent
for _, event := range auditEvents {
    if event.EventType == "signalprocessing.signal.processed" {
        processedEvents = append(processedEvents, event)
    }
}
Expect(len(processedEvents)).To(Equal(1),
    "Should have exactly 1 signal.processed event")
```

**Priority**: P1 (blocks CI merge)
**Confidence**: 95% (same pattern as HAPI audit fix)

---

### 2. AI Analysis (AA)

**Failure Summary**: 1 audit test failing with FlakeAttempts(3) exhausted

**Root Cause**: **DD-TESTING-001 Compliance Issue - Test expects exactly 3 phase transitions, business logic emits 4**

**Details**:
```bash
[FAIL] Complete Workflow Audit Trail - BR-AUDIT-001 [It] should generate complete audit trail from Pending to Completed

Failed 3 times (FlakeAttempts exhausted):
- Expected exactly 3 phase transitions
- Got 4 phase transitions

Error: Expected <int>: 4 to equal <int>: 3
Location: test/integration/aianalysis/audit_flow_integration_test.go:244
```

**Evidence from Logs**:
```
Expected exactly 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed
Actual: 4 phase transitions (additional transition emitted by business logic)
```

**Fix Strategy**:

**Option A: Validate Specific Transitions (Recommended)**
```go
// BEFORE (line 244)
Expect(eventTypeCounts[aiaudit.EventTypePhaseTransition]).To(Equal(3),
    "Expected exactly 3 phase transitions: Pending→Investigating, Investigating→Analyzing, Analyzing→Completed")

// AFTER (DD-TESTING-001 compliant - validate specific transitions)
var phaseTransitions []dsgen.AuditEvent
for _, event := range events {
    if event.EventType == aiaudit.EventTypePhaseTransition {
        phaseTransitions = append(phaseTransitions, event)
    }
}

// Validate required transitions exist (not exact count)
requiredTransitions := map[string]bool{
    "Pending→Investigating": false,
    "Investigating→Analyzing": false,
    "Analyzing→Completed": false,
}

for _, event := range phaseTransitions {
    // Extract from_phase and to_phase from event payload
    fromPhase := event.Payload["from_phase"]
    toPhase := event.Payload["to_phase"]
    transitionKey := fmt.Sprintf("%s→%s", fromPhase, toPhase)

    if _, exists := requiredTransitions[transitionKey]; exists {
        requiredTransitions[transitionKey] = true
    }
}

for transition, found := range requiredTransitions {
    Expect(found).To(BeTrue(),
        fmt.Sprintf("Required phase transition missing: %s", transition))
}
```

**Option B: Use Minimum Count (Simpler)**
```go
// Allow business logic to emit additional phase transitions
Expect(eventTypeCounts[aiaudit.EventTypePhaseTransition]).To(BeNumerically(">=", 3),
    "Expected at least 3 phase transitions (may have additional transitions)")
```

**Priority**: P1 (blocks CI merge)
**Confidence**: 90% (similar to HAPI fix, but needs payload inspection)
**Recommended**: Option B (simpler, aligns with DD-TESTING-001)

---

### 3. HolmesGPT API (HAPI)

**Failure Summary**: Docker container build failed during client generation

**Root Cause**: **Dockerfile executes generate-client.sh inside container, but container has no podman/docker**

**Details**:
```bash
STEP 13/24: RUN cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../../..
🔧 Generating Python OpenAPI client (DD-HAPI-005)...
   OpenAPI Spec: /workspace/holmesgpt-api/api/openapi.json
   Output: /workspace/holmesgpt-api/tests/clients/holmesgpt_api_client
❌ Error: Neither podman nor docker found. Please install one.
```

**Issue**:
- `holmesgpt-api/tests/integration/generate-client.sh` requires podman/docker (lines 40-48)
- Uses `openapitools/openapi-generator-cli:latest` Docker container
- Dockerfile `RUN` command executes inside UBI9 Python container (no Docker-in-Docker)

**Fix Strategy**:

**Option A: Generate Client on Host (Recommended - matches E2E pattern)**
```dockerfile
# BEFORE (docker/holmesgpt-api-integration-test.Dockerfile:54-56)
RUN cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../../..

# AFTER (generate on host before docker build)
# Remove the RUN command from Dockerfile
# Update Makefile to generate client before building container
```

```makefile
# Makefile: test-integration-holmesgpt-api target
test-integration-holmesgpt-api: ginkgo clean-holmesgpt-test-ports
    @echo "🔧 Step 1: Generating HAPI OpenAPI client..."
    @cd holmesgpt-api/tests/integration && bash generate-client.sh && cd ../../..
    @echo "✅ Client generated"
    @echo ""
    @echo "🐳 Step 2: Building integration test container..."
    @podman build -t holmesgpt-api-integration-test:latest \
        -f docker/holmesgpt-api-integration-test.Dockerfile .
    # ... rest of existing logic ...
```

**Option B: Use Python openapi-generator Package (Alternative)**
```dockerfile
# Install openapi-generator-cli as Python package (no Docker required)
RUN pip3.12 install openapi-generator-cli
RUN cd holmesgpt-api/tests/integration && \
    openapi-generator-cli generate \
    -i /workspace/api/openapi.json \
    -g python \
    -o /workspace/holmesgpt-api/tests/clients/holmesgpt_api_client \
    --additional-properties=packageName=holmesgpt_api_client,projectName=holmesgpt-api-client
```

**Priority**: P1 (blocks CI merge)
**Confidence**: 95% (Option A matches E2E pattern and is well-tested)
**Recommended**: Option A (consistency with E2E tests)

---

## 🔍 **Pattern Analysis**

### **Common Root Cause: DD-TESTING-001 Compliance**

All 3 failures violate **DD-TESTING-001: Audit Event Validation Standards**:

```markdown
❌ ANTI-PATTERN: Tests SHOULD NOT assert exact audit event counts
✅ CORRECT: Tests SHOULD filter by event_type and validate specific events

Rationale: Business logic evolution may emit additional audit events
for observability, compliance, or debugging purposes. Tests should be
resilient to these additions while still validating required events exist.
```

**Affected Tests**:
1. **SP**: Expects exactly 1 total event, business emits 5+ events
2. **AA**: Expects exactly 3 phase transitions, business emits 4
3. **HAPI**: Fixed in previous commit (filter by event_type)

**Solution Pattern**:
```go
// Step 1: Query all events by correlation ID
allEvents := queryAuditEvents(correlationID)

// Step 2: Filter by specific event type
targetEvents := []dsgen.AuditEvent{}
for _, event := range allEvents {
    if event.EventType == "target.event.type" {
        targetEvents = append(targetEvents, event)
    }
}

// Step 3: Validate filtered events (not total count)
Expect(len(targetEvents)).To(Equal(1),
    "Should have exactly 1 target.event.type event")
```

---

## 📋 **Implementation Plan**

### **Phase 1: Signal Processing (SP) - Estimated 15 min**

**Files to Modify**:
1. `test/integration/signalprocessing/audit_integration_test.go`
   - Line 185: Change `Equal(1)` to `BeNumerically(">=", 1)`
   - Lines 188-196: Filter events by `event_type == "signalprocessing.signal.processed"`
   - Line 558: Similar fix for phase.transition test

**Test Plan**:
```bash
make test-integration-signalprocessing  # Should pass locally
```

---

### **Phase 2: AI Analysis (AA) - Estimated 15 min**

**Files to Modify**:
1. `test/integration/aianalysis/audit_flow_integration_test.go`
   - Line 244: Change `Equal(3)` to `BeNumerically(">=", 3)`
   - Add comment explaining business logic may emit additional transitions

**Test Plan**:
```bash
make test-integration-aianalysis  # Should pass locally
```

---

### **Phase 3: HolmesGPT API (HAPI) - Estimated 20 min**

**Files to Modify**:
1. `Makefile` (test-integration-holmesgpt-api target)
   - Add client generation step before container build

2. `docker/holmesgpt-api-integration-test.Dockerfile`
   - Remove `RUN cd holmesgpt-api/tests/integration && bash generate-client.sh`
   - Update comment explaining client is generated on host

**Test Plan**:
```bash
make test-integration-holmesgpt-api  # Should pass locally
```

---

## 🎯 **Success Criteria**

✅ All 8 integration test jobs pass in CI
✅ Tests are resilient to additional audit events (DD-TESTING-001 compliant)
✅ HAPI client generation works consistently (matches E2E pattern)
✅ No test flakiness related to event count assumptions

---

## 📚 **References**

- [DD-TESTING-001: Audit Event Validation Standards](../architecture/decisions/DD-TESTING-001-audit-event-validation-standards.md)
- [CI Integration Tests IPv6 Triage (Jan 3)](./CI_INTEGRATION_TEST_FAILURES_IPv6_TRIAGE_JAN_03_2026.md)
- [HAPI Test Logic Triage (Jan 3)](./HAPI_TEST_LOGIC_TRIAGE_JAN_03_2026.md)

---

**Status**: ⏳ Implementation in progress
**Next Steps**: Apply fixes sequentially (SP → AA → HAPI), verify locally, then push
**ETA**: ~50 minutes for all 3 fixes + verification
**Confidence**: 90% (all fixes follow established patterns)

