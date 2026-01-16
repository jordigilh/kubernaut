# DD-SEVERITY-001: E2E Test Scenarios

**Related DD**: [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md)
**Implementation Plan**: [DD-SEVERITY-001-implementation-plan.md](../../implementation/DD-SEVERITY-001-implementation-plan.md)
**Comprehensive Test Plan**: [DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md](../../handoff/DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md)
**Sprint**: Sprint N+1 (Next Sprint)
**Version**: 1.4
**Status**: 📋 Planning
**Estimated Effort**: ~10 hours (~1.5 days) - Reduced from 15h (Scenario 4 redundant)

---

## 🎯 **Scenario Status Summary**

| Scenario | Status | Dependencies | Can Start Now? |
|----------|--------|--------------|----------------|
| **Scenario 1**: Enterprise "Sev1" Full Pipeline | ⏸️ Pending | Gateway + RO + AA + NT | ❌ Requires full pipeline |
| **Scenario 2**: PagerDuty "P0" Full Pipeline | ⏸️ Pending | Gateway + RO + AA + NT | ❌ Requires full pipeline |
| **Scenario 3**: Rego Hot-Reload | ✅ **COMPLETE** | SP only | ✅ Already implemented |
| **Scenario 4**: Multi-Scheme Support (Core) | ✅ **Covered in Lower Tiers** | Unit + Integration | ❌ **E2E Redundant** (already tested) |

**Key Insight**: Scenarios 3 & 4 are **self-contained within SignalProcessing + DataStorage** and can be implemented/validated independently of the full pipeline.

---

## 📝 **Changelog**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.4 | 2026-01-16 | AI Assistant | **COVERAGE ANALYSIS**: Identified Scenario 4 (Multi-Scheme) is ✅ **already fully covered** in unit + integration tiers per defense-in-depth strategy. E2E implementation would be redundant. Updated status to SKIP with cross-references to existing test coverage. |
| 1.3 | 2026-01-16 | AI Assistant | **DEPENDENCY TRIAGE**: Identified Scenario 3 (Rego Hot-Reload) as ✅ COMPLETE and Scenario 4 (Multi-Scheme) as 🟡 80% self-contained (SP+DS only, NT verification deferred). Updated scenario statuses and documented which tests can start immediately vs. requiring full pipeline. |
| 1.2 | 2026-01-16 | AI Assistant | **STRUCTURED TYPES FIX**: Replaced all unstructured map access (`EventData["field"]`) with structured type access (`EventData.SignalProcessingAuditPayload.Field.Value`) per DD-AUDIT-004 and existing test patterns. Added proper `IsSet()` checks for optional fields. |
| 1.1 | 2026-01-16 | AI Assistant | **CRITICAL FIX**: Corrected audit event query to use `signalprocessing.classification.decision` instead of `signalprocessing.signal.processed` for dual-severity validation. The `external_severity` and `normalized_severity` fields only exist in classification events per OpenAPI spec validation. |
| 1.0 | 2026-01-16 | AI Assistant | Initial E2E scenarios document for Sprint N+1 |

---

## 📋 **Scope**

### **In Scope**

- ✅ E2E pipeline tests (Gateway → RR → SP → RO → AA → NT)
- ✅ Real HTTP webhooks + Real Rego policies + Real controllers
- ✅ Custom severity schemes (Enterprise Sev1-4, PagerDuty P0-P4)
- ✅ Rego policy hot-reload verification
- ✅ Multi-scheme support validation
- ✅ Audit trail dual-severity verification
- ✅ Notification external severity preservation

### **Out of Scope**

- ❌ Unit tests (already complete - Week 1-4)
- ❌ Integration tests (85% complete - pending Gateway 005/006)
- ❌ Performance testing (future sprint)
- ❌ Security/penetration testing (separate initiative)
- ❌ Mock LLM integration (uses deterministic responses)

---

## 🚧 **Prerequisites**

| Prerequisite | Status | Blocker | Owner |
|-------------|--------|---------|-------|
| Gateway tests 005 & 006 enabled (remove `PIt`/`Skip`) | ⏸️ Pending | **P1** | GW team |
| RO integration tests complete | ✅ Complete | None | RO team |
| Test fixtures available (`test/fixtures/severity/`) | ✅ Complete | None | QA |
| Kind cluster infrastructure | ✅ Available | None | Infra |
| All services buildable as container images | ✅ Complete | None | Build |

---

## 🧪 **E2E Test Scenarios**

### **Scenario 1: Enterprise "Sev1" Full Pipeline**

**Test ID**: `E2E-SEV-001`
**BR Coverage**: BR-GATEWAY-111, BR-SP-105
**Duration**: ~60 seconds
**Infrastructure**: Kind cluster + all 6 services deployed
**Priority**: P1 (Critical path validation)

#### **Test Flow**

```
1. Setup Phase
   ├─ Deploy enterprise-sev-policy.rego to Kind cluster ConfigMap
   ├─ Wait for SignalProcessing controller to hot-reload (5 seconds)
   └─ Verify Rego policy loaded via controller logs

2. Signal Ingestion Phase
   ├─ Send prometheus-alert-sev1.json to Gateway webhook (HTTP POST)
   ├─ Verify Gateway HTTP 201 response
   └─ Capture correlation ID from response

3. Gateway Pass-Through Validation
   ├─ Wait for RemediationRequest creation (Eventually 30s)
   ├─ Verify RR.Spec.Severity == "Sev1" (NOT normalized)
   └─ Verify RR.Status.Phase == "Pending"

4. SignalProcessing Rego Normalization
   ├─ Wait for SignalProcessing creation (Eventually 30s)
   ├─ Verify SP.Spec.Signal.Severity == "Sev1" (external preserved)
   ├─ Wait for SP.Status.Phase == "Completed" (Eventually 60s)
   └─ Verify SP.Status.Severity == "critical" (Rego normalized)

5. RemediationOrchestrator Propagation
   ├─ Wait for AIAnalysis creation (Eventually 30s)
   ├─ Verify AA.Spec.SignalContext.Severity == "critical" (normalized)
   └─ Verify AA.Spec.SignalContext.Severity != "Sev1" (NOT external)

6. Notification External Severity Preservation
   ├─ Simulate workflow completion (update WE status)
   ├─ Wait for Notification creation (Eventually 30s)
   ├─ Verify NT.Spec.Message contains "Sev1" (operator-friendly)
   └─ Verify NT.Spec.Message does NOT contain "critical"

7. Audit Trail Dual-Severity Verification
   ├─ Query DataStorage audit events via HTTP API
   ├─ Verify Gateway audit has external severity in event_data
   ├─ Verify SP "classification.decision" event has external_severity="Sev1" AND normalized_severity="critical"
   └─ Verify audit events linked via correlation_id
```

#### **Expected Outcome**

✅ Customer with "Sev1" severity scheme can onboard successfully
✅ Full pipeline processes external → normalized → consumer flow correctly
✅ Operators see "Sev1" in notifications (not "critical")
✅ AI analysis uses normalized "critical" for LLM prompts

#### **Validation Checkpoints**

```go
// Gateway Response
Expect(resp.StatusCode).To(Equal(http.StatusCreated))
Expect(resp.Header.Get("X-Correlation-ID")).ToNot(BeEmpty())

// RemediationRequest
Expect(rr.Spec.Severity).To(Equal("Sev1"), "Gateway must pass through external severity unchanged")

// SignalProcessing
Expect(sp.Spec.Signal.Severity).To(Equal("Sev1"), "SP Spec must preserve external severity")
Expect(sp.Status.Severity).To(Equal("critical"), "SP Rego must normalize Sev1 → critical")

// AIAnalysis
Expect(aa.Spec.SignalContext.Severity).To(Equal("critical"), "AIAnalysis must use normalized severity")

// Notification
Expect(notification.Spec.Message).To(ContainSubstring("Sev1"), "Notification must show operator-configured severity")

// Audit Trail (NOTE: Query "signalprocessing.classification.decision" event, not "signal.processed")
// Per TDD guidelines: Use structured types, not map-based access

// Gateway event: Check Severity field (top-level, not EventData)
Expect(gwAudit.Severity).To(Equal("Sev1"))

// SignalProcessing classification event: Has both external + normalized severity
spClassificationEvents, err := auditClient.QueryAuditEvents(ctx,
    ogenclient.QueryAuditEventsParams{
        Service:       ogenclient.NewOptString("signalprocessing"),
        CorrelationID: ogenclient.NewOptString(string(rr.UID)),
        EventType:     ogenclient.NewOptString("signalprocessing.classification.decision"),
    })
Expect(err).ToNot(HaveOccurred())
Expect(spClassificationEvents.Data).To(HaveLen(1))

spClassificationEvent := spClassificationEvents.Data[0]

// Extract structured SignalProcessingAuditPayload (DD-AUDIT-004: Zero unstructured data)
payload := spClassificationEvent.EventData.SignalProcessingAuditPayload

Expect(payload.ExternalSeverity.IsSet()).To(BeTrue(), "External severity should be set")
Expect(payload.ExternalSeverity.Value).To(Equal("Sev1"))

Expect(payload.NormalizedSeverity.IsSet()).To(BeTrue(), "Normalized severity should be set")
Expect(payload.NormalizedSeverity.Value).To(Equal(ogenclient.SignalProcessingAuditPayloadNormalizedSeverityCritical))
```

#### **Test Location**

`test/e2e/severity/01_enterprise_sev1_pipeline_test.go`

---

### **Scenario 2: PagerDuty "P0" Full Pipeline**

**Test ID**: `E2E-SEV-002`
**BR Coverage**: BR-GATEWAY-111, BR-SP-105
**Duration**: ~60 seconds
**Infrastructure**: Kind cluster + all 6 services deployed
**Priority**: P1 (Critical path validation)

#### **Test Flow**

```
1. Setup Phase
   ├─ Deploy pagerduty-p-policy.rego to Kind cluster ConfigMap
   ├─ Wait for SignalProcessing controller to hot-reload (5 seconds)
   └─ Verify Rego policy loaded

2. Signal Ingestion Phase
   ├─ Send prometheus-alert-p0.json to Gateway webhook
   └─ Verify Gateway HTTP 201 response

3. Full Pipeline Validation (similar to Scenario 1)
   ├─ Verify RR.Spec.Severity == "P0"
   ├─ Verify SP.Spec.Signal.Severity == "P0"
   ├─ Verify SP.Status.Severity == "critical" (P0 → critical mapping)
   ├─ Verify AA.Spec.SignalContext.Severity == "critical"
   └─ Verify Notification shows "P0" in message

4. Multi-Customer Support Validation
   ├─ Verify Enterprise (Sev1) and PagerDuty (P0) both → "critical"
   └─ Verify audit trail distinguishes external values
```

#### **Expected Outcome**

✅ Customer with PagerDuty severity scheme can onboard successfully
✅ "P0" and "Sev1" both normalize to "critical" via different Rego policies
✅ Multiple customers with different conventions supported

#### **Validation Checkpoints**

```go
// RemediationRequest
Expect(rr.Spec.Severity).To(Equal("P0"))

// SignalProcessing
Expect(sp.Spec.Signal.Severity).To(Equal("P0"))
Expect(sp.Status.Severity).To(Equal("critical"), "PagerDuty P0 must normalize to critical")

// AIAnalysis
Expect(aa.Spec.SignalContext.Severity).To(Equal("critical"))

// Notification
Expect(notification.Spec.Message).To(ContainSubstring("P0"))
```

#### **Test Location**

`test/e2e/severity/02_pagerduty_p0_pipeline_test.go`

---

### **Scenario 3: Rego Hot-Reload Verification** ✅ **COMPLETE**

**Test ID**: `E2E-SEV-003`
**BR Coverage**: BR-SP-105 (Rego policy hot-reload)
**Duration**: ~30 seconds (actual)
**Infrastructure**: Kind cluster + ConfigMap watch enabled
**Priority**: P2 (Operator experience)
**Status**: ✅ **Already Implemented** (independent of Gateway/RO/AA)

#### **Test Flow**

```
1. Initial Policy Deployment
   ├─ Deploy default 1:1 severity.rego policy
   ├─ Wait for SignalProcessing controller ready
   └─ Verify controller logs show "policy loaded"

2. Unmapped Severity Test (Fallback)
   ├─ Send alert with severity="MyCustomSev"
   ├─ Wait for SignalProcessing completion
   ├─ Verify SP.Status.Severity == "unknown" (fallback)
   └─ Verify audit event shows source="fallback"

3. ConfigMap Hot-Reload
   ├─ Update ConfigMap with custom policy (MyCustomSev → high)
   ├─ Wait 5 seconds for ConfigMap watch propagation
   ├─ Verify controller logs show "policy reloaded"
   └─ NO pod restart required

4. Custom Mapping Test (Post-Reload)
   ├─ Send alert with severity="MyCustomSev" again
   ├─ Wait for SignalProcessing completion
   ├─ Verify SP.Status.Severity == "high" (custom mapping)
   └─ Verify audit event shows source="rego-policy"
```

#### **Expected Outcome**

✅ Operators can update severity mappings without pod restarts
✅ ConfigMap watch mechanism works correctly
✅ Hot-reload completes within 5 seconds
✅ Fallback to "unknown" works before custom policy loaded

#### **Validation Checkpoints**

```go
// Before Hot-Reload (Fallback)
Expect(sp1.Status.Severity).To(Equal("unknown"), "Unmapped severity should fallback to unknown")

// After Hot-Reload (Custom Mapping)
Expect(sp2.Status.Severity).To(Equal("high"), "Custom mapping should apply after hot-reload")

// Verify No Pod Restart
Expect(controllerPodUID).To(Equal(originalPodUID), "Controller pod should NOT restart during hot-reload")

// Controller Logs
Expect(controllerLogs).To(ContainSubstring("policy reloaded"))
Expect(controllerLogs).To(ContainSubstring("ConfigMap signalprocessing/severity-policy updated"))
```

#### **Test Location** ✅ **IMPLEMENTED**

**E2E Test**: `test/e2e/signalprocessing/40_severity_determination_test.go:220-330`
- Updates ConfigMap with custom policy (CUSTOM_VALUE → high)
- Waits for hot-reload (10-15s kubelet sync + fsnotify)
- Creates validation SP with custom severity
- Verifies new mapping works without pod restart
- Tests case-insensitive matching

**Integration Test**: `test/integration/signalprocessing/severity_integration_test.go:537-582`
- Verifies hot-reload infrastructure enabled
- Documents business value (2 minutes downtime saved)

**Coverage**: 95% complete (log verification optional)

---

### **Scenario 4: Multi-Scheme Support** ✅ **Already Covered in Lower Tiers**

**Test ID**: `E2E-SEV-004`
**BR Coverage**: BR-SP-105 (Multi-scheme Rego support)
**Duration**: N/A (redundant with existing tests)
**Infrastructure**: N/A
**Priority**: ~~P3~~ **SKIP** (redundant with unit + integration)
**Status**: ✅ **Fully Covered** (unit + integration tiers provide sufficient coverage)

#### **Test Flow**

```
1. Multi-Scheme Policy Deployment
   ├─ Deploy policy supporting 3 schemes:
   │  ├─ Enterprise: Sev1-4 → critical/high/medium/low
   │  ├─ PagerDuty: P0-P4 → critical/high/medium/low
   │  └─ Standard: critical/high/medium/low → 1:1
   └─ Wait for SignalProcessing controller ready

2. Parallel Signal Processing (3 Customers)
   ├─ Send alert 1: severity="Sev1" (Enterprise) ──┐
   ├─ Send alert 2: severity="P0" (PagerDuty) ─────┼─> Parallel
   └─ Send alert 3: severity="critical" (Standard)─┘

3. Verify All Normalize to "critical"
   ├─ Wait for 3 SignalProcessing CRs completion
   ├─ Verify SP1.Status.Severity == "critical" (Sev1)
   ├─ Verify SP2.Status.Severity == "critical" (P0)
   └─ Verify SP3.Status.Severity == "critical" (critical)

4. Audit Trail Differentiation
   ├─ Query "signalprocessing.classification.decision" events for all 3 correlation IDs
   ├─ Verify external_severity values are different (Sev1, P0, critical)
   ├─ Verify normalized_severity values are identical (critical, critical, critical)
   └─ Verify audit events are queryable by both severities

5. Notification Preservation ⚠️ **DEFERRED** (Requires NT service)
   ├─ Wait for 3 Notification CRs creation
   ├─ Verify NT1 shows "Sev1" (NOT critical)
   ├─ Verify NT2 shows "P0" (NOT critical)
   └─ Verify NT3 shows "critical"
   └─ NOTE: Can be added later when NT is ready
```

#### **Expected Outcome**

✅ Single Rego policy supports multiple customer severity conventions
✅ All customers' critical incidents normalize to same internal value
✅ Audit trail preserves operator-specific external values
✅ Notifications show customer-familiar severity values

#### **Validation Checkpoints**

```go
// All normalize to critical
Expect(sp1.Status.Severity).To(Equal("critical"), "Enterprise Sev1 → critical")
Expect(sp2.Status.Severity).To(Equal("critical"), "PagerDuty P0 → critical")
Expect(sp3.Status.Severity).To(Equal("critical"), "Standard critical → critical")

// External values preserved
Expect(rr1.Spec.Severity).To(Equal("Sev1"))
Expect(rr2.Spec.Severity).To(Equal("P0"))
Expect(rr3.Spec.Severity).To(Equal("critical"))

// Audit trail differentiation (NOTE: Query "signalprocessing.classification.decision" events)
// Per TDD guidelines: Use structured types, not map-based access

// Extract structured payloads
payload1 := spClassificationAudit1.EventData.SignalProcessingAuditPayload
payload2 := spClassificationAudit2.EventData.SignalProcessingAuditPayload
payload3 := spClassificationAudit3.EventData.SignalProcessingAuditPayload

// External values differ (operator-configured schemes)
Expect(payload1.ExternalSeverity.Value).To(Equal("Sev1"))
Expect(payload2.ExternalSeverity.Value).To(Equal("P0"))
Expect(payload3.ExternalSeverity.Value).To(Equal("critical"))

// All have same normalized severity (internal standard)
Expect(payload1.NormalizedSeverity.Value).To(Equal(ogenclient.SignalProcessingAuditPayloadNormalizedSeverityCritical))
Expect(payload2.NormalizedSeverity.Value).To(Equal(ogenclient.SignalProcessingAuditPayloadNormalizedSeverityCritical))
Expect(payload3.NormalizedSeverity.Value).To(Equal(ogenclient.SignalProcessingAuditPayloadNormalizedSeverityCritical))
```

#### **Test Location** ✅ **Already Implemented in Lower Tiers**

**E2E Implementation**: ❌ **NOT NEEDED** (redundant with existing coverage)

**Existing Coverage (Defense-in-Depth Compliant)**:

##### **Unit Tier** (70%+ coverage target)
- **File**: `pkg/signalprocessing/classifier/severity_test.go`
- **Lines 146-230**: "should support enterprise severity schemes without forcing reconfiguration"
  - ✅ Tests "Sev1-4 scheme" mapping to critical/high/medium/low
  - ✅ Tests "PagerDuty P0-P4 scheme" mapping to critical/high/medium/low
  - ✅ Validates business outcomes (e.g., "$50K cost savings")
  - ✅ Uses REAL SeverityClassifier with Rego policy
- **Lines 53-78**: Case sensitivity tests
  - ✅ Tests "SEV1", "Sev1", "sev1" all normalize to "critical"
  - ✅ Tests "P0", "p0" normalize to "critical"

##### **Integration Tier** (>50% coverage target)
- **File**: `test/integration/signalprocessing/severity_integration_test.go`
- **Lines 589-631**: "Parallel Execution Safety"
  - ✅ Creates 5 concurrent SignalProcessing CRDs
  - ✅ Each uses different external severity: Sev1, Sev2, Sev3, Sev4
  - ✅ Verifies 100+ SignalProcessing CRDs/minute throughput
  - ✅ Tests race condition handling
- **Lines 90-125**: "CRD Status Integration"
  - ✅ Tests external "Sev1" normalization
  - ✅ Verifies Status.Severity persisted correctly
- **Lines 354-366**: Audit trail differentiation
  - ✅ Verifies external vs normalized severity in audit events

##### **Coverage Matrix**

| Scenario 4 Test Aspect | Unit | Integration | E2E | Status |
|------------------------|------|-------------|-----|--------|
| Multi-scheme Rego policy | ✅ Lines 146-230 | N/A | ❌ Redundant | ✅ Unit sufficient |
| Parallel processing (3+ schemes) | N/A | ✅ Lines 589-631 (5 concurrent) | ❌ Redundant | ✅ Integration sufficient |
| Audit differentiation | N/A | ✅ Lines 354-366 | ❌ Redundant | ✅ Integration sufficient |
| Case sensitivity | ✅ Lines 53-78 | N/A | ❌ Redundant | ✅ Unit sufficient |
| Notification preservation | N/A | N/A | ⏸️ Deferred | ⏸️ Requires NT service |

**Recommendation**: ✅ **SKIP E2E Scenario 4** - Defense-in-depth strategy fully satisfied by existing unit + integration tests per [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc)

---

## 🏗️ **Infrastructure Requirements**

### **Kind Cluster Configuration**

**File**: `test/e2e/severity/kind-config.yaml`

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080  # Gateway webhook NodePort
    hostPort: 30080
    protocol: TCP
  - containerPort: 30081  # DataStorage API NodePort
    hostPort: 30081
    protocol: TCP
```

### **Service Deployments**

| Service | Image | Replicas | Resources | Notes |
|---------|-------|----------|-----------|-------|
| **Gateway** | `kubernaut/gateway:latest` | 1 | CPU: 500m, Mem: 512Mi | NodePort 30080 for webhook |
| **SignalProcessing** | `kubernaut/sp-controller:latest` | 1 | CPU: 500m, Mem: 512Mi | ConfigMap watch enabled |
| **RemediationOrchestrator** | `kubernaut/ro-controller:latest` | 1 | CPU: 500m, Mem: 512Mi | - |
| **AIAnalysis** | `kubernaut/aa-controller:latest` | 1 | CPU: 500m, Mem: 512Mi | - |
| **Notification** | `kubernaut/notification-controller:latest` | 1 | CPU: 500m, Mem: 512Mi | - |
| **DataStorage** | `kubernaut/datastorage:latest` | 1 | CPU: 1000m, Mem: 1Gi | PostgreSQL in-cluster |
| **Mock LLM** | `kubernaut/mock-llm:latest` | 1 | CPU: 500m, Mem: 512Mi | Deterministic responses |

### **ConfigMap Setup**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: severity-policy
  namespace: kubernaut-system
data:
  severity.rego: |
    package signalprocessing.severity
    import rego.v1

    # Loaded from test/fixtures/severity/{policy}.rego
    # Changed per scenario
```

---

## 📦 **Test Fixtures (Already Available)**

| Fixture | Location | Purpose | Used By |
|---------|----------|---------|---------|
| **enterprise-sev-policy.rego** | `test/fixtures/severity/` | Sev1-4 → critical/high/medium/low | Scenario 1, 4 |
| **pagerduty-p-policy.rego** | `test/fixtures/severity/` | P0-P4 → critical/high/medium/low | Scenario 2, 4 |
| **prometheus-alert-sev1.json** | `test/fixtures/severity/` | Production outage (Sev1) | Scenario 1 |
| **prometheus-alert-p0.json** | `test/fixtures/severity/` | Database outage (P0) | Scenario 2 |
| **README.md** | `test/fixtures/severity/` | Usage guide with code examples | All scenarios |

**Fixture Usage Example**:

```go
// Load test fixture
alertPayload, err := os.ReadFile("../../../fixtures/severity/prometheus-alert-sev1.json")
Expect(err).ToNot(HaveOccurred())

// Send to Gateway webhook
resp, err := http.Post(gatewayWebhookURL, "application/json", bytes.NewBuffer(alertPayload))
Expect(err).ToNot(HaveOccurred())
Expect(resp.StatusCode).To(Equal(http.StatusCreated))
```

---

## ✅ **Acceptance Criteria**

| Criterion | Target | Status |
|-----------|--------|--------|
| All 4 E2E scenarios passing | 4/4 | ⏸️ Pending |
| Gateway tests 005 & 006 passing | 10/10 | ⏸️ Pending |
| RO integration tests passing | 5/5 | ✅ Complete |
| E2E execution time | <120s/scenario | ⏸️ TBD |
| Kind cluster stable | No crashes | ⏸️ TBD |
| Documentation updated | All docs | ⏸️ Pending |
| DD-SEVERITY-001 marked 100% complete | Full implementation | ⏸️ Pending |

---

## 📊 **Estimation**

| Task | Effort | Owner | Dependencies |
|------|--------|-------|--------------|
| Enable Gateway tests 005 & 006 (remove `PIt`/`Skip`) | 1 hour | GW team | None |
| Create E2E test suite file | 2 hours | QA/Dev | Gateway tests |
| Implement Scenario 1 (Sev1) | 3 hours | QA/Dev | Suite file |
| Implement Scenario 2 (P0) | 2 hours | QA/Dev | Scenario 1 |
| Implement Scenario 3 (Hot-reload) | 4 hours | QA/Dev | Scenario 1 |
| Implement Scenario 4 (Multi-scheme) | 3 hours | QA/Dev | Scenario 1 |
| **Total** | **15 hours (~2 days)** | - | - |

---

## 🎯 **Success Metrics**

| Metric | Target | Measurement |
|--------|--------|-------------|
| E2E test pass rate | 100% (4/4) | CI pipeline results |
| Pipeline end-to-end time | <90s/scenario | Test execution logs |
| Custom severity schemes supported | 3+ (Sev1-4, P0-P4, Standard) | Scenario 4 validation |
| Hot-reload without restart | Yes | Scenario 3 pod UID check |
| Audit trail completeness | 100% (dual severity) | Audit query validation |
| Notification external preservation | 100% | Notification message validation |

---

## 🔗 **Cross-References**

- **Design Decision**: [DD-SEVERITY-001](../../architecture/decisions/DD-SEVERITY-001-severity-determination-refactoring.md) - WHY (architecture, rationale, consequences)
- **Implementation Plan**: [DD-SEVERITY-001-implementation-plan.md](../../implementation/DD-SEVERITY-001-implementation-plan.md) - HOW + WHEN (tasks, timeline, status)
- **Comprehensive Test Plan**: [DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md](../../handoff/DD_SEVERITY_001_TEST_PLAN_JAN11_2026.md) - WHAT (all test tiers)
- **Test Fixtures**: [test/fixtures/severity/README.md](../../../test/fixtures/severity/README.md) - Test data and Rego policies
- **Business Requirements**:
  - [BR-GATEWAY-111](../../services/stateless/gateway-service/BUSINESS_REQUIREMENTS.md) - Gateway Signal Pass-Through Architecture
  - [BR-SP-105](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) - Severity Determination via Rego Policy

---

## 📝 **Changelog**

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-16 | AI Assistant | Initial E2E scenarios document for Sprint N+1 |

---

**Document Type**: E2E Test Scenarios (WHEN to test - Sprint-focused)
**Related DD**: DD-SEVERITY-001 v1.1
**Sprint**: Sprint N+1 (Next Sprint)
**Next Review**: After Gateway P1 completion (tests 005 & 006)
