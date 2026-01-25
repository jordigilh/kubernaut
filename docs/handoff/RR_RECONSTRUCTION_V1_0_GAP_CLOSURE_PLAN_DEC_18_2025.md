# 🎯 RemediationRequest Reconstruction from Audit Traces - V1.0 Gap Closure Plan

**Date**: December 18, 2025, 17:30 UTC
**Status**: ⚠️ **SUPERSEDED** - Timeline outdated due to SOC2 work (see V1.1 plan)
**Original Approval**: ✅ User Decision (100% Coverage) - December 18, 2025
**Business Requirement**: **BR-AUDIT-005 v2.0** (Enterprise-Grade Audit Integrity and Compliance - RR Reconstruction)
**Priority**: **P0** - Must-have for V1.0 release
**Goal**: Enable exact RR CRD reconstruction from audit traces (excluding user-modified status fields)
**Current Coverage**: 70% → **Target Coverage**: 100% (ALL 8 GAPS CLOSED)

**⚠️ SUPERSEDED BY**: [RR_RECONSTRUCTION_V1_1_IMPLEMENTATION_PLAN_JAN10.md](../development/SOC2/RR_RECONSTRUCTION_V1_1_IMPLEMENTATION_PLAN_JAN10.md)

**Why Superseded**: SOC2 compliance work (December 20 - January 8, 2026) completed 60% of the infrastructure identified in this plan, reducing the 6.5-day estimate to 3 days. See [RR_RECONSTRUCTION_SOC2_OVERLAP_TRIAGE_JAN09.md](./RR_RECONSTRUCTION_SOC2_OVERLAP_TRIAGE_JAN09.md) for details.

**Authority**: This plan implements the "RR CRD Reconstruction" component of [BR-AUDIT-005 v2.0](../requirements/11_SECURITY_ACCESS_CONTROL.md)

---

## 🎯 **User Decision**

> **"We must have this feature for v1.0. We need to extend the audit BR to include the notion that the audit traces can be used to recreate the RR exactly as it was before it was deleted."**

**Acknowledgement**: User-made changes to `.status` fields will NOT be captured in audit traces (this is expected and acceptable).

**Focus**: Capture all `.spec` fields and system-managed `.status` fields that enable exact RR reconstruction.

---

## 📊 **Current State: What We Have vs. What We Need**

### **✅ Already Available (70% Coverage)**

**Spec Fields (7/10 field groups)**:
- ✅ `SignalFingerprint`, `SignalName`, `Severity`, `SignalType`, `SignalSource` (100%)
- ✅ `TargetResource` (kind, name, namespace) (90%)
- ✅ `FiringTime`, `ReceivedTime` (90-100%)
- ✅ Classification (environment, priority, criticality) (100%)

**Status Fields (4/10 fields)**:
- ✅ `Phase`, `PhaseStartedAt`, `LastTransitionTime` (100%)
- ✅ Lifecycle timeline (phase transitions) (100%)

---

### **❌ Critical Gaps (30% Missing Coverage)**

**HIGH PRIORITY - Blocking Exact Reconstruction**:

| Gap # | Field | Current Coverage | Impact | Service |
|-------|-------|-----------------|--------|---------|
| **1** | `OriginalPayload` | 0% | **CRITICAL** - Raw K8s event | Gateway |
| **2** | `ProviderData` | 0% | **CRITICAL** - Holmes analysis | AI Analysis |
| **3** | `SignalLabels` | 0% | **HIGH** - Full label map | Gateway |
| **4** | `SignalAnnotations` | 0% | **HIGH** - Full annotation map | Gateway |
| **5** | `SelectedWorkflowRef` | 0% | **HIGH** - Workflow selection | Workflow Engine |
| **6** | `ExecutionRef` | 0% | **MEDIUM** - WE CRD link | Remediation Execution |
| **7** | `Error` (detailed) | 0% | **MEDIUM** - Error messages | All services |
| **8** | `TimeoutConfig` | 0% | **LOW** - Timeout settings | Remediation Orchestrator |

**Total Gaps**: 8 fields across 5 services

---

## 🔧 **Gap Closure Strategy**

### **Phase 1: Critical Spec Fields (Gaps #1-4)** - **MUST CLOSE**

These fields are part of the RR `.spec` and are IMMUTABLE once created. Without them, we cannot exactly recreate the RR.

#### **Gap #1: `OriginalPayload` (Gateway Service)**

**Problem**: Raw K8s event/webhook JSON not captured in audit traces.

**Solution**:

```yaml
# Gateway audit event enhancement
event_data:
  signal_fingerprint: "oomkilled-ns-web-pod-api-server"
  alert_name: "KubernetesPodOOMKilled"
  severity: "critical"
  # NEW: Add complete original payload
  original_payload:  # ← ADD THIS
    apiVersion: "v1"
    kind: "Event"
    metadata:
      name: "api-server.oom.12345"
      namespace: "web"
      uid: "abc-123"
    # ... full K8s Event structure
```

**Implementation**:
1. **File**: `pkg/gateway/signal_processor.go` (or equivalent)
2. **Change**: Add `original_payload` to `gateway.signal.received` audit event
3. **Size Impact**: ~2-5KB per audit event (compressed JSON)
4. **Testing**: Verify payload is complete and valid JSON

**Confidence**: **100%** - We have access to the original payload in Gateway

**Effort**: **2-3 hours**

---

#### **Gap #2: `ProviderData` (AI Analysis Service)**

**Problem**: Holmes/external provider structured analysis not captured.

**Solution**:

```yaml
# AI Analysis audit event enhancement
event_data:
  correlation_id: "rr-2025-001"
  analysis_provider: "holmesgpt"
  # NEW: Add complete provider data
  provider_data:  # ← ADD THIS
    provider: "holmesgpt"
    analysis_id: "holmes-abc123"
    confidence: 0.92
    root_cause_candidates:
      - category: "resource_exhaustion"
        likelihood: 0.85
    context_insights:
      recent_deployments: true
      memory_pressure: true
    # ... full Holmes response structure
```

**Implementation**:
1. **File**: `pkg/aianalysis/controller.go` (or equivalent)
2. **Change**: Add `provider_data` to `aianalysis.analysis.completed` audit event
3. **Size Impact**: ~1-3KB per audit event
4. **Testing**: Verify Holmes response is fully captured

**Confidence**: **100%** - We have access to Holmes response in AI Analysis

**Effort**: **2-3 hours**

---

#### **Gap #3 & #4: `SignalLabels` & `SignalAnnotations` (Gateway Service)**

**Problem**: Full label and annotation maps not captured (only select keys).

**Solution**:

```yaml
# Gateway audit event enhancement
event_data:
  signal_fingerprint: "oomkilled-ns-web-pod-api-server"
  # NEW: Add complete maps
  signal_labels:  # ← ADD THIS (full map)
    app: "api-server"
    tier: "backend"
    environment: "production"
    team: "platform"
    # ... ALL labels from original signal
  signal_annotations:  # ← ADD THIS (full map)
    prometheus.io/scrape: "true"
    deployment.kubernetes.io/revision: "42"
    last-applied-configuration: "{...}"
    # ... ALL annotations from original signal
```

**Implementation**:
1. **File**: `pkg/gateway/signal_processor.go`
2. **Change**: Add `signal_labels` and `signal_annotations` maps to audit event
3. **Size Impact**: ~500B-2KB per audit event (depends on label count)
4. **Testing**: Verify all labels/annotations captured

**Confidence**: **100%** - We have access to full label/annotation maps in Gateway

**Effort**: **1-2 hours**

---

### **Phase 2: Critical Status Fields (Gaps #5-7)** - **STRONGLY RECOMMENDED**

These fields are part of the RR `.status` (system-managed, not user-editable). They provide critical "what happened" narrative.

#### **Gap #5: `SelectedWorkflowRef` (Workflow Engine)**

**Problem**: Which workflow was selected is not captured.

**Solution**:

```yaml
# Workflow Engine audit event enhancement
event_data:
  correlation_id: "rr-2025-001"
  selected_workflow_name: "oomkilled-restart-v2"
  # NEW: Add complete workflow reference
  selected_workflow_ref:  # ← ADD THIS
    kind: "RemediationWorkflow"
    apiVersion: "remediation.io/v1alpha1"
    name: "oomkilled-restart-v2"
    namespace: "kubernaut-system"
    uid: "workflow-uid-123"
  selection_reason: "highest_label_match_score"
  match_score: 0.95
```

**Implementation**:
1. **File**: `pkg/workflowengine/workflow_selector.go`
2. **Change**: Add `selected_workflow_ref` to `workflow.selection.completed` audit event
3. **Size Impact**: ~200B per audit event
4. **Testing**: Verify workflow ref matches RR status field

**Confidence**: **100%** - Workflow selection data is available

**Effort**: **1-2 hours**

---

#### **Gap #6: `ExecutionRef` (Remediation Execution)**

**Problem**: Link to WorkflowExecution CRD not captured.

**Solution**:

```yaml
# Remediation Execution audit event enhancement
event_data:
  correlation_id: "rr-2025-001"
  execution_phase: "executing"
  # NEW: Add execution CRD reference
  execution_ref:  # ← ADD THIS
    kind: "WorkflowExecution"
    apiVersion: "remediation.io/v1alpha1"
    name: "we-rr-2025-001"
    namespace: "kubernaut-system"
    uid: "execution-uid-456"
```

**Implementation**:
1. **File**: `pkg/remediationexecution/controller.go`
2. **Change**: Add `execution_ref` to `execution.started` audit event
3. **Size Impact**: ~200B per audit event
4. **Testing**: Verify execution ref matches RR status field

**Confidence**: **100%** - Execution CRD is created by Remediation Execution

**Effort**: **1 hour**

---

#### **Gap #7: `Error` Detailed Messages (All Services)**

**Problem**: Only generic failure outcome captured, not detailed error messages.

**Solution**:

```yaml
# Enhanced error audit events (all services)
event_data:
  correlation_id: "rr-2025-001"
  event_outcome: "failure"
  # NEW: Add detailed error information
  error_details:  # ← ADD THIS
    message: "Workflow execution failed: pod 'api-server' not found in namespace 'web'"
    code: "ResourceNotFound"
    component: "workflow_execution"
    retry_possible: false
```

**Implementation**:
1. **Files**: All service error handling locations
2. **Change**: Add `error_details` to all `*.failure` audit events
3. **Size Impact**: ~500B per error event
4. **Testing**: Verify error messages match RR status field

**Confidence**: **95%** - Errors are already logged, just need to capture in audit

**Effort**: **3-4 hours** (across 5 services)

---

### **Phase 3: Optional Enhancement (Gap #8)** - **NICE TO HAVE**

#### **Gap #8: `TimeoutConfig` (Remediation Orchestrator)**

**Problem**: Timeout settings not captured (rarely populated, low impact).

**Solution**: Add `timeout_config` to `orchestrator.lifecycle.created` audit event.

**Effort**: **1 hour**
**Priority**: **P2** (can defer to post-V1.0 if time constrained)

---

## 📋 **Business Requirement Extension**

### **Proposed BR-AUDIT-005: Audit-Based RR Reconstruction**

**Category**: BR-AUDIT (Audit Trail)
**Priority**: **P0** (V1.0 blocker)
**Related**: BR-AUDIT-001 to BR-AUDIT-004

#### **Requirement Statement**

**BR-AUDIT-005**: The system **MUST** capture sufficient data in audit traces to enable exact reconstruction of RemediationRequest CRDs after TTL expiration (24 hours).

#### **Acceptance Criteria**

1. **Spec Field Coverage**: ✅ **100%** of RR `.spec` fields can be reconstructed from audit traces
   - ✅ `SignalFingerprint`, `SignalName`, `Severity`, `SignalType`, `SignalSource`
   - ✅ `TargetResource` (full structure)
   - ✅ `FiringTime`, `ReceivedTime`
   - ✅ `OriginalPayload` (complete JSON)
   - ✅ `ProviderData` (complete structure)
   - ✅ `SignalLabels`, `SignalAnnotations` (complete maps)
   - ✅ `TimeoutConfig` (optional field, captured when present)

2. **Status Field Coverage**: ✅ **90%** of system-managed `.status` fields can be reconstructed
   - ✅ `Phase`, `PhaseStartedAt`, `LastTransitionTime`
   - ✅ `Deduplication` (OccurrenceCount, FirstSeenAt, LastSeenAt)
   - ✅ `SelectedWorkflowRef`, `ExecutionRef`
   - ✅ `Error` (detailed messages)
   - ❌ **EXCLUDED**: User-modified status fields (acknowledged)

3. **Reconstruction Accuracy**: ✅ **100%** field-level accuracy (excluding user edits)

4. **Reconstruction Tool**: ✅ **API endpoint** for RR reconstruction (V1.0) - CLI optional post-V1.0

5. **Testing**: ✅ Integration tests validate full reconstruction lifecycle

#### **Out of Scope (Acknowledged)**

- ❌ User-made changes to `.status` fields (e.g., manual phase overrides)
- ❌ Fields modified after RR creation (immutable `.spec` only)
- ❌ Real-time reconstruction (post-TTL use case only)

---

## 🧪 **Testing Strategy**

### **Integration Test: Full Reconstruction Lifecycle**

```go
var _ = Describe("BR-AUDIT-005: RR Reconstruction from Audit Traces", func() {
    It("should reconstruct RR with 100% spec accuracy after TTL deletion", func() {
        // 1. Create and execute full remediation lifecycle
        originalRR := createRemediationRequest(ctx, "test-signal")
        Expect(originalRR).ToNot(BeNil())

        // 2. Wait for lifecycle completion (all audit events written)
        Eventually(func() string {
            return getRRPhase(ctx, originalRR.Name)
        }).Should(Equal("Completed"))

        // 3. Capture original RR state before deletion
        originalSpec := originalRR.Spec.DeepCopy()
        originalStatus := originalRR.Status.DeepCopy()

        // 4. Simulate TTL deletion (delete RR after 24h)
        deleteRemediationRequest(ctx, originalRR.Name)

        // 5. Reconstruct RR from audit traces
        reconstructedRR := reconstructRRFromAuditTraces(ctx, originalRR.Name)
        Expect(reconstructedRR).ToNot(BeNil())

        // 6. Validate spec field accuracy (100% target)
        validateSpecFields(originalSpec, reconstructedRR.Spec)

        // 7. Validate system-managed status fields (90% target)
        validateStatusFields(originalStatus, reconstructedRR.Status)
    })
})

func validateSpecFields(original, reconstructed *RemediationRequestSpec) {
    // Mandatory fields (100% accuracy)
    Expect(reconstructed.SignalFingerprint).To(Equal(original.SignalFingerprint))
    Expect(reconstructed.SignalName).To(Equal(original.SignalName))
    Expect(reconstructed.Severity).To(Equal(original.Severity))

    // Critical new fields (MUST be present)
    Expect(reconstructed.OriginalPayload).To(Equal(original.OriginalPayload))
    Expect(reconstructed.ProviderData).To(Equal(original.ProviderData))
    Expect(reconstructed.SignalLabels).To(Equal(original.SignalLabels))
    Expect(reconstructed.SignalAnnotations).To(Equal(original.SignalAnnotations))

    // Optional field (can be nil if not set)
    if original.TimeoutConfig != nil {
        Expect(reconstructed.TimeoutConfig).To(Equal(original.TimeoutConfig))
    }
}
```

### **E2E Test: Production-Like Scenario**

```go
var _ = Describe("BR-AUDIT-005: Post-Incident RR Reconstruction", func() {
    It("should enable compliance audit 30 days after incident", func() {
        // Simulate real-world compliance scenario:
        // 1. Production incident occurs
        // 2. Remediation executes and completes
        // 3. RR deleted after 24h TTL
        // 4. 30 days later: Compliance team needs RR details
        // 5. Reconstruct from audit traces for audit report
    })
})
```

---

## ⏱️ **Implementation Timeline**

### **Phase 1: Critical Spec Fields (10-12 hours)**

| Task | Service | Effort | Priority |
|------|---------|--------|----------|
| Add `OriginalPayload` to audit | Gateway | 3h | **P0** |
| Add `ProviderData` to audit | AI Analysis | 3h | **P0** |
| Add `SignalLabels` to audit | Gateway | 2h | **P0** |
| Add `SignalAnnotations` to audit | Gateway | 2h | **P0** |
| Integration tests | - | 2h | **P0** |

**Total**: 12 hours (~1.5 days)

---

### **Phase 2: Critical Status Fields (8-10 hours)**

| Task | Service | Effort | Priority |
|------|---------|--------|----------|
| Add `SelectedWorkflowRef` to audit | Workflow Engine | 2h | **P0** |
| Add `ExecutionRef` to audit | Remediation Execution | 1h | **P0** |
| Add `error_details` to audit (all services) | All | 4h | **P0** |
| Integration tests | - | 2h | **P0** |

**Total**: 9 hours (~1 day)

---

### **Phase 3: Reconstruction Logic (12-16 hours)**

| Task | Component | Effort | Priority |
|------|-----------|--------|----------|
| Design reconstruction algorithm | - | 2h | **P0** |
| Implement RR spec reconstruction | CLI/API | 4h | **P0** |
| Implement RR status reconstruction | CLI/API | 3h | **P0** |
| Handle edge cases (missing events) | - | 2h | **P0** |
| **API endpoint** (reconstruction) | REST API | 3h | **P0** |
| E2E tests | - | 3h | **P0** |

**Total**: 17 hours (~2 days)

---

### **Phase 4: Documentation & BR Updates (4-6 hours)**

| Task | Effort | Priority |
|------|--------|----------|
| Update ADR-034 with new audit fields | 2h | **P0** |
| Create BR-AUDIT-005 documentation | 2h | **P0** |
| Update service audit documentation | 2h | **P0** |

**Total**: 6 hours (~0.75 days)

---

## 📊 **Total Effort: 6-6.5 Days** (100% Coverage)

**Breakdown**:
- Phase 1 (Spec fields - Gaps #1-4): 1.5 days
- Phase 2 (Status fields - Gaps #5-7): 1 day
- Phase 3 (TimeoutConfig - Gap #8): 0.5 days ← **NEW (100% coverage)**
- Phase 4 (Reconstruction logic): 2 days
- Phase 5 (Documentation + CLI): 0.75 days
- **Buffer**: 0.75 days (for unexpected issues)

**Recommended Sprint**: 1.5 weeks (assuming 1 developer full-time)

**Note**: User chose Option B (100% coverage) to include TimeoutConfig capture (+0.5 days)

---

## 🎯 **Success Criteria**

### **Must-Have (V1.0)**

1. ✅ **100% Spec Coverage**: ALL `.spec` fields captured in audit traces (including TimeoutConfig)
2. ✅ **90% Status Coverage**: All system-managed `.status` fields captured
3. ✅ **Reconstruction Tool**: **API endpoint** for RR reconstruction (V1.0) - automation-ready
4. ✅ **Integration Tests**: Validate full reconstruction lifecycle
5. ✅ **BR-AUDIT-005 v2.0**: Documented and accepted (100% reconstruction accuracy)

### **Nice-to-Have (Post-V1.0)**

1. ⏳ **CLI wrapper** (1-2 days) - `kubernaut rr reconstruct <id>` - thin wrapper around API
2. ⏳ Real-time reconstruction (for live RRs, not just post-TTL)
3. ⏳ Bulk reconstruction API (reconstruct multiple RRs at once)
4. ⏳ Web UI for reconstruction visualization
5. ⏳ Diff view (compare reconstructed vs live RR)

---

## 🚨 **Risks & Mitigations**

### **Risk #1: Audit Event Size Increase**

**Risk**: Adding `OriginalPayload` and `ProviderData` may significantly increase audit event size.

**Impact**: **MEDIUM** - Increased database storage and network bandwidth.

**Mitigation**:
1. **Compression**: Store JSON payloads compressed (gzip)
2. **Sampling**: Only capture full payload for P0/P1 incidents
3. **Partitioning**: ADR-034 already uses date-based partitioning for scalability

**Confidence**: **85%** - With compression, size increase should be manageable (<5KB per event)

---

### **Risk #2: Holmes API Response Changes**

**Risk**: Holmes/provider API responses may change format over time, breaking reconstruction.

**Impact**: **LOW** - Only affects older audit events if Holmes API changes.

**Mitigation**:
1. **Versioning**: Capture Holmes API version in `provider_data`
2. **Schema Evolution**: Use flexible JSON structure
3. **Fallback**: Reconstruction handles missing/invalid provider data gracefully

**Confidence**: **90%** - Schema evolution is common practice

---

### **Risk #3: Reconstruction Edge Cases**

**Risk**: Missing audit events (dropped, delayed, or incomplete) may prevent reconstruction.

**Impact**: **MEDIUM** - Reconstruction fails if critical events are missing.

**Mitigation**:
1. **Partial Reconstruction**: Return RR with partial data + confidence score
2. **Validation**: Reconstruction tool reports missing fields explicitly
3. **Monitoring**: Alert on audit event gaps (BR-AUDIT-001 already requires this)

**Confidence**: **80%** - Robust error handling can mitigate most edge cases

---

## 🔄 **Alternative Approaches Considered**

### **Alternative 1: Snapshot RR to Separate Store (Rejected)**

**Approach**: Periodically snapshot RRs to a separate long-term storage (S3, etc.) before TTL deletion.

**Pros**:
- ✅ Exact RR preservation (no reconstruction needed)
- ✅ Simple to implement

**Cons**:
- ❌ Redundant storage (audit traces + snapshots)
- ❌ No single source of truth
- ❌ Snapshot timing issues (what if RR changes after snapshot?)

**Reason for Rejection**: Violates "audit traces as single source of truth" principle (ADR-034).

---

### **Alternative 2: Extend RR TTL to 90 Days (Rejected)**

**Approach**: Keep RRs in Kubernetes for 90 days instead of 24 hours.

**Pros**:
- ✅ No reconstruction needed
- ✅ Simple to implement

**Cons**:
- ❌ etcd storage explosion (thousands of RRs)
- ❌ K8s performance degradation (list operations)
- ❌ Still need long-term audit trail (>90 days for compliance)

**Reason for Rejection**: Does not scale for high-volume production environments.

---

## 📚 **Related Documentation**

- [RR_CRD_RECONSTRUCTION_FROM_AUDIT_TRACES_ASSESSMENT_DEC_18_2025.md](./RR_CRD_RECONSTRUCTION_FROM_AUDIT_TRACES_ASSESSMENT_DEC_18_2025.md) - Initial feasibility assessment
- [RR_RECONSTRUCTION_OPERATIONAL_VALUE_ASSESSMENT_DEC_18_2025.md](./RR_RECONSTRUCTION_OPERATIONAL_VALUE_ASSESSMENT_DEC_18_2025.md) - Operational value analysis
- [ADR-034: Unified Audit Table Design](../architecture/decisions/ADR-034-unified-audit-table-design.md) - Audit table schema
- [DD-AUDIT-003: Service Audit Trace Requirements](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) - Audit requirements

---

## 🎯 **Next Steps**

### **Immediate Actions** (This Sprint)

1. **User Approval**: Get explicit approval on this plan and timeline
2. **Sprint Planning**: Allocate 1 developer for 5-6 days
3. **Backlog Prioritization**: Move BR-AUDIT-005 to P0

### **Implementation Order** (Recommended)

1. **Week 1**: Phase 1 (Spec fields) + Phase 2 (Status fields)
2. **Week 2**: Phase 3 (Reconstruction logic) + Phase 4 (Documentation)
3. **Week 3**: Testing, validation, and bug fixes

---

**Status**: 🚀 **READY FOR IMPLEMENTATION** - Awaiting user approval
**Confidence**: **90%** - Plan is comprehensive, risks are manageable, timeline is realistic
**Recommendation**: **APPROVE** - High operational value, reasonable effort, critical for V1.0 compliance use cases

---

## 💬 **Questions for User**

1. **Timeline**: Is 5-6 days acceptable for V1.0, or do we need to compress?
2. **Scope**: Should we include Phase 3 (reconstruction tool) in V1.0, or just audit enhancements (Phases 1-2)?
3. **Priority**: If time is constrained, which gaps are most critical? (I recommend #1, #2, #5 as minimum)
4. **Testing**: Do you want E2E reconstruction tests in V1.0, or defer to post-V1.0?
5. **Deployment**: Should reconstruction tool be CLI-only, or also API endpoint?

