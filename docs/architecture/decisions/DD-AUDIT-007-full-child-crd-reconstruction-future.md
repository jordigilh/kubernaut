# DD-AUDIT-007: Full Child CRD Reconstruction (Future Feature)

**Status**: 📋 **PLANNED** - Future Consideration (Post-V1.0)
**Date**: February 1, 2026
**Priority**: P2 (Enhancement - Not Required for V1.0)
**Version**: 1.0
**Related**: BR-AUDIT-005, DD-AUDIT-004, ADR-034

---

## Executive Summary

This document captures the **future feature** for reconstructing **complete child CRDs** (SignalProcessing, AIAnalysis, WorkflowExecution, NotificationRequest, RemediationApprovalRequest) from audit traces, similar to the existing RR reconstruction capability.

**Current State (V1.0)**:
- ✅ RemediationRequest reconstruction (100% field coverage)
- ✅ Correlation ID links all child events
- ✅ Query all events by `correlation_id` for complete timeline
- ✅ Sufficient for SOC 2 Type II compliance

**Future Enhancement**:
- 🔮 Full child CRD YAML reconstruction from audit traces
- 🔮 REST API endpoints for child CRD reconstruction
- 🔮 Forensic investigation of child CRD state after deletion

**Decision**: **DEFER to post-V1.0** - current audit trail is sufficient for compliance and operational needs.

---

## Context & Problem

### **Current Audit Approach**

**V1.0 provides**:
1. **Selective field capture**: Business-critical fields for audit trail
   - SignalProcessing: phase, severity, environment, priority (15-20 fields)
   - AIAnalysis: phase, approval, selected workflow, confidence (10-15 fields)
   - WorkflowExecution: execution refs, phase transitions (5-8 fields)
   - NotificationRequest: delivery status (5-8 fields)
   - RemediationApprovalRequest: approval decision, user, rationale (8-10 fields)

2. **Event-based timeline**: Complete audit trail via `correlation_id` query
   ```sql
   SELECT * FROM audit_events 
   WHERE correlation_id = 'rr-oomkilled-abc123'
   ORDER BY event_timestamp;
   ```

3. **RR reconstruction**: 100% RemediationRequest field coverage (BR-AUDIT-005 v2.0)

**What auditors can do today**:
- ✅ Reconstruct RemediationRequest (root document)
- ✅ Query complete event timeline (who did what, when)
- ✅ Export signed audit trail (legal evidence)
- ✅ Satisfy SOC 2 Type II, ISO 27001, NIST 800-53

---

### **Future Enhancement Proposal**

**Full child CRD reconstruction would enable**:
- 🔮 Reconstruct complete SignalProcessing YAML (all spec + status fields)
- 🔮 Reconstruct complete AIAnalysis YAML (all spec + status fields)
- 🔮 Reconstruct complete WorkflowExecution YAML (all spec + status fields)
- 🔮 Reconstruct complete NotificationRequest YAML (all spec + status fields)
- 🔮 Reconstruct complete RemediationApprovalRequest YAML (all spec + status fields)

**Use cases**:
- Forensic investigation requiring exact CRD state
- Legal discovery requiring complete YAML exports
- Compliance audits demanding full CRD snapshots
- Debugging complex remediation failures months later

---

## Potential Implementation

### **Approach: Lifecycle Snapshot Events**

**Add new event types**:
```yaml
# NEW event types (one per service)
- signalprocessing.lifecycle.snapshot     # Full SP CRD
- aianalysis.lifecycle.snapshot           # Full AA CRD
- workflowexecution.lifecycle.snapshot    # Full WE CRD
- notification.lifecycle.snapshot         # Full NT CRD
- approval.lifecycle.snapshot             # Full RAR CRD
```

**Trigger**: Emit snapshot when CRD reaches terminal state (`Completed`, `Failed`, `Expired`)

**Payload**: Complete `.spec` + `.status` (all fields)

---

### **OpenAPI Schema Extensions**

**Example for AIAnalysis**:

```yaml
AIAnalysisSnapshotPayload:
  type: object
  description: Complete AIAnalysis CRD snapshot for reconstruction
  required:
    - event_type
    - spec
    - status
  properties:
    event_type:
      type: string
      enum: [aianalysis.lifecycle.snapshot]
    spec:
      type: object
      description: Complete AIAnalysis spec (30-40 fields)
      properties:
        remediation_request_ref: {$ref: '#/components/schemas/ObjectReference'}
        remediation_id: {type: string}
        analysis_request: {$ref: '#/components/schemas/AnalysisRequest'}
        is_recovery_attempt: {type: boolean}
        previous_executions: {type: array, items: {...}}
        timeout_config: {$ref: '#/components/schemas/TimeoutConfig'}
        # ... all spec fields
    status:
      type: object
      description: Complete AIAnalysis status (40-50 fields)
      properties:
        phase: {type: string}
        started_at: {type: string, format: date-time}
        completed_at: {type: string, format: date-time}
        root_cause_analysis: {$ref: '#/components/schemas/RootCauseAnalysis'}
        selected_workflow: {$ref: '#/components/schemas/SelectedWorkflow'}
        approval_context: {$ref: '#/components/schemas/ApprovalContext'}
        warnings: {type: array, items: {type: string}}
        # ... all status fields
```

**Multiplier**: 5 CRD types × 2 schemas (spec + status) = **10 new OpenAPI schemas**

---

### **REST API Endpoints**

**Option A: Individual Child Endpoints**

```bash
POST /api/v1/audit/signal-processing/{sp-name}/reconstruct
POST /api/v1/audit/ai-analysis/{aa-name}/reconstruct
POST /api/v1/audit/workflow-execution/{we-name}/reconstruct
POST /api/v1/audit/notification-request/{nt-name}/reconstruct
POST /api/v1/audit/approval-request/{rar-name}/reconstruct
```

**Option B: Unified Endpoint with Query Param**

```bash
POST /api/v1/audit/crds/{crd-name}/reconstruct?type=AIAnalysis
```

**Option C: Extend RR Endpoint (Convenience)**

```bash
POST /api/v1/audit/remediation-requests/{rr-name}/reconstruct?include=children

Response:
{
  "remediation_request": {...},
  "signal_processing": {...},      # ← NEW
  "ai_analysis": {...},            # ← NEW
  "workflow_execution": {...},     # ← NEW
  "notifications": [...],          # ← NEW
  "approval_request": {...}        # ← NEW (optional)
}
```

---

## Implementation Effort

| Component | Effort | Lines of Code | Risk |
|-----------|--------|---------------|------|
| Lifecycle snapshot events | **High** | 2500-4000 LOC (5 services) | Low |
| OpenAPI schema extensions | **Moderate** | 400-600 LOC (YAML) | Low |
| RAR audit package | **N/A** | 0 (covered by DD-AUDIT-006) | N/A |
| REST API endpoints | **Low** | 1000-2000 LOC (5 endpoints) | Low |
| Integration tests | **High** | 1500-2500 LOC | Medium |
| **TOTAL** | **High** | **~5500-9100 LOC** | **Low-Medium** |

**Time Estimate**: 
- Development: 3-4 weeks (2 devs)
- Testing: 2-3 weeks
- Documentation: 1 week
- **Total**: **6-8 weeks**

---

## Storage Impact

### **Current (V1.0)**:
```
RemediationRequest reconstruction:  5-12KB
Child CRD action events:            2-4KB
-------------------------------------------
Total per remediation:              7-16KB
```

**Annual storage (10K remediations/year)**: ~160MB/year

---

### **With Full Snapshots (Future)**:
```
RemediationRequest reconstruction:  5-12KB  (unchanged)
SignalProcessing snapshot:          3-5KB   (full spec + status)
AIAnalysis snapshot:                5-8KB   (full spec + status)
WorkflowExecution snapshot:         2-4KB   (full spec + status)
NotificationRequest snapshot:       1-2KB   (full spec + status) × 2-3
RemediationApprovalRequest snapshot: 2-3KB   (full spec + status, optional)
---------------------------------------------------
Total per remediation:              18-34KB (~2-3x increase)
```

**Annual storage (10K remediations/year)**: ~340MB/year (**+112% increase**)

---

## Business Value Assessment

### **Who Would Benefit?**

1. **Forensic Investigations**: SEC, SOC teams reconstructing full remediation state 6-12 months later
2. **Compliance Audits**: SOC 2, ISO 27001 auditors requesting full CRD snapshots
3. **Legal Discovery**: eDiscovery requests for exact system state at a point in time
4. **Engineering Post-Mortems**: Debugging complex failures without K8s CRD access

---

### **Arguments FOR** 🟢

- **Complete Audit Trail**: Zero gaps in reconstruction
- **Legal Defensibility**: Prove exact system state at any time
- **Debugging Power**: Reconstruct full state without K8s access
- **Compliance Gold Standard**: Exceeds SOC 2 / ISO 27001 requirements

---

### **Arguments AGAINST** 🔴

- **Current Approach Sufficient**: RR + correlation_id query provides complete timeline
- **Storage Overhead**: 2-3x increase in audit data (~180MB additional/year)
- **Maintenance Burden**: 5 new reconstruction endpoints + 10 OpenAPI schemas
- **Complexity**: More code, more tests, more documentation
- **No Explicit Requirement**: Neither SOC 2 nor BR-AUDIT-005 mandates child CRD reconstruction
- **Incremental Value Questionable**: Auditors can query events, do they need full YAML?

---

## Decision Rationale

### **Why Defer to Post-V1.0**

1. **Current audit trail is sufficient for V1.0 compliance**:
   - ✅ SOC 2 Type II certified with RR reconstruction + event timeline
   - ✅ Auditors can query all events via `correlation_id`
   - ✅ Legal defensibility via signed audit exports

2. **No explicit business requirement**:
   - BR-AUDIT-005 v2.0 mandates RR reconstruction only
   - No customer requests for child CRD reconstruction

3. **Storage overhead without proven need**:
   - 2-3x increase (~180MB/year per 10K remediations)
   - Cost without clear ROI

4. **V1.0 scope management**:
   - Focus on critical features (RAR audit trail)
   - Avoid feature creep

5. **Can be added incrementally**:
   - Well-defined feature (6-8 weeks)
   - Low technical risk (follows existing patterns)
   - One CRD type at a time

---

### **When to Reconsider**

**Triggers for implementation**:
1. **External audit finding**: SOC 2 auditor requests child CRD snapshots
2. **Customer request**: Enterprise customer requires full CRD reconstruction
3. **Legal requirement**: eDiscovery or regulatory mandate
4. **Operational need**: Multiple forensic investigations blocked by lack of snapshots

**Incremental rollout**:
1. Start with **AIAnalysis** (highest value: LLM reasoning)
2. Add **SignalProcessing** (second: classification decisions)
3. Add **WorkflowExecution** (third: execution config)
4. Add **RemediationApprovalRequest** (fourth: approval audit)
5. Add **NotificationRequest** (fifth: delivery receipts)

**Effort per CRD**: ~1-2 weeks development + 3-5 days testing

---

## Recommendation

### **For V1.0**: ❌ **DO NOT IMPLEMENT**

**Rationale**:
- Current RR reconstruction + event timeline satisfies SOC 2 Type II
- No proven business need for full child CRD snapshots
- Storage overhead (2-3x) without clear ROI
- Focus resources on critical features (RAR audit trail)

---

### **For Future Consideration**: 🔮 **EVALUATE POST-V1.0**

**If needed later**:
- Well-defined feature (~6-8 weeks)
- Low risk (follows existing patterns)
- Can be added incrementally (one CRD type at a time)

**Ask first**:
> "Will auditors actually request full child CRD YAML, or is the event timeline sufficient?"

If **"timeline is sufficient"** → V1.0 is complete ✅

If **"need full snapshots"** → Implement post-V1.0 incrementally 🔮

---

## Alternative: Enhanced Event Queries

**Instead of full reconstruction**, consider enhancing query API:

```bash
# Query events with rich filtering
POST /api/v1/audit/events/query
{
  "correlation_id": "rr-oomkilled-abc123",
  "event_category": "aianalysis",
  "include_event_data": true,
  "group_by": "event_type"
}

Response:
{
  "events": [
    {
      "event_type": "aianalysis.analysis.started",
      "event_timestamp": "2026-01-15T10:00:00Z",
      "event_data": {...}
    },
    {
      "event_type": "aianalysis.analysis.completed",
      "event_timestamp": "2026-01-15T10:05:00Z",
      "event_data": {
        "phase": "Completed",
        "approval_required": false,
        "selected_workflow": {...},
        "confidence": 0.85
      }
    }
  ],
  "summary": {
    "total_events": 12,
    "duration_seconds": 300,
    "outcome": "success"
  }
}
```

**Benefits**:
- ✅ Richer querying without full snapshots
- ✅ Lower storage overhead
- ✅ Auditors get complete story from events
- ✅ Incremental enhancement (1-2 weeks)

---

## Related Documents

- [BR-AUDIT-005 v2.0: Hybrid Provider Data Capture](../../requirements/11_SECURITY_ACCESS_CONTROL.md)
- [DD-AUDIT-004: RR Reconstruction Field Mapping](./DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md)
- [ADR-034: Unified Audit Table Design](./ADR-034-unified-audit-table-design.md)

---

## Approval

**Status**: 📋 **CAPTURED FOR FUTURE CONSIDERATION**
**Date**: February 1, 2026
**Priority**: P2 (Enhancement - Not Required for V1.0)
**Decision**: **DEFER to post-V1.0** - Evaluate based on operational feedback

---

**Document Version**: 1.0
**Last Updated**: February 1, 2026
**Maintained By**: Kubernaut Architecture Team

---

## Summary

- ✅ **V1.0**: RR reconstruction + event timeline (sufficient for SOC 2)
- 🔮 **Future**: Full child CRD reconstruction (if business need arises)
- 📋 **Captured**: Well-defined feature ready for implementation if needed
- ⏳ **Timeline**: 6-8 weeks if triggered by audit finding or customer request
