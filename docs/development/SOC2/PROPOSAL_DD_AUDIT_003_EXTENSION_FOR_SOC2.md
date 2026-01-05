# Proposal: Extend DD-AUDIT-003 with SOC2 Event Types

**Status**: ✅ **APPROVED & IMPLEMENTED**
**Date**: January 4, 2026
**Authority**: Concern1 feedback from SOC2 implementation planning
**Related BR**: BR-AUDIT-005 v2.0 (100% RR reconstruction)
**Confidence**: 90%

---

## 🎯 **Context**

**Problem**: SOC2 implementation requires specific audit event types for 100% RR CRD reconstruction (per DD-AUDIT-004). These event types are either:
1. **Missing entirely** from DD-AUDIT-003
2. **Named differently** than required by SOC2 implementation
3. **More granular** than needed for RR reconstruction

**Discovery**: DD-AUDIT-003 v1.2 defines audit requirements for all services, but predates the SOC2 100% RR reconstruction requirement (DD-AUDIT-004, December 18, 2025).

**Impact**: Without extending DD-AUDIT-003, developers implementing SOC2 gaps won't have authoritative guidance on event types.

---

## 📋 **Gap Analysis**

### **Event Types Required by SOC2** (from SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)

| Event Type | Service | Purpose | DD-AUDIT-003 Status |
|---|---|---|---|
| `gateway.signal.received` | Gateway | Capture original payload, labels, annotations | ✅ **EXISTS** (line 143) |
| `aianalysis.analysis.completed` | AI Analysis | Capture provider_data (Holmes response) | ❌ **MISSING** (uses `ai-analysis.*` naming) |
| `workflow.selection.completed` | Workflow Execution | Capture selected_workflow_ref | ❌ **MISSING** (no workflow selection event) |
| `execution.workflow.started` | Workflow Execution | Capture execution_ref | ✅ **EXISTS** (line 198) |

### **Detailed Gap Breakdown**

#### **Gap 1: AI Analysis Event Naming** ⚠️

**DD-AUDIT-003 Current** (lines 168-175):
```markdown
| Event Type | Description | Priority |
|---|---|---|
| `ai-analysis.investigation.started` | AI investigation started | P0 |
| `ai-analysis.llm.request_sent` | LLM API request sent | P0 |
| `ai-analysis.llm.response_received` | LLM API response received | P0 |
| `ai-analysis.recommendation.generated` | Remediation recommendation generated | P0 |
| `ai-analysis.crd.updated` | AIAnalysis CRD updated | P0 |
| `ai-analysis.llm.request_failed` | LLM API request failed | P0 |
```

**SOC2 Requirement**:
- Event type: `aianalysis.analysis.completed` (NO hyphen)
- Purpose: Single event capturing complete Holmes `provider_data` for RR reconstruction
- Rationale: Simpler, focused on business outcome, not granular technical steps

**Issue**:
- ❌ Naming mismatch: `ai-analysis.*` (hyphenated) vs `aianalysis.*` (no hyphen)
- ❌ Event fragmentation: 6 granular events vs 1 business outcome event
- ⚠️ None of the existing events capture complete `provider_data` for RR reconstruction

#### **Gap 2: Workflow Selection Event** ❌

**DD-AUDIT-003 Current**:
- **Workflow Execution** section (lines 183-208) does NOT include workflow *selection* events
- Only execution events: `execution.workflow.started`, `execution.action.executed`, etc.
- **Remediation Orchestrator** section (lines 327-360) has lifecycle events but no selection event

**SOC2 Requirement**:
- Event type: `workflow.selection.completed`
- Purpose: Capture `selected_workflow_ref` (which workflow was chosen for remediation)
- Field captured: `.status.selectedWorkflowRef` from RR CRD

**Issue**: ❌ **Completely missing** - no service currently audits workflow selection decision

---

## 💡 **Proposed Changes to DD-AUDIT-003**

### **Proposal Option A: Add SOC2-Specific Events (RECOMMENDED)** ⭐

**Rationale**: Keep existing granular events for operational visibility, add SOC2-focused events for compliance

#### **Change 1: AI Analysis Controller** (Section 2, after line 175)

**ADD** new event type for SOC2 compliance:

```markdown
| `aianalysis.analysis.completed` | AI analysis completed with full Holmes response (SOC2) | **P0** |
```

**Explanation**:
- **Purpose**: Single event capturing complete `provider_data` for RR reconstruction (DD-AUDIT-004)
- **Distinction**: Complements existing granular events (`ai-analysis.*`) for operational visibility
- **Naming**: Matches SOC2 test plan convention (no hyphen: `aianalysis` not `ai-analysis`)
- **Priority**: P0 (required for BR-AUDIT-005 v2.0 compliance)

**Event Data Fields**:
```json
{
  "provider_data": {
    "provider": "HolmesGPT",
    "analysis_id": "holmes-abc123",
    "recommendations": [...],
    "confidence_score": 0.95
  }
}
```

#### **Change 2: Workflow Execution Controller** (Section 3, after line 198)

**ADD** new event type for workflow selection:

```markdown
| `workflow.selection.completed` | Workflow selected for remediation (SOC2) | **P0** |
```

**Explanation**:
- **Purpose**: Capture which workflow was chosen for execution
- **Distinction**: New event type (no existing equivalent)
- **RR Field**: Maps to `.status.selectedWorkflowRef` in RR CRD
- **Priority**: P0 (required for 100% RR reconstruction)

**Event Data Fields**:
```json
{
  "selected_workflow_ref": {
    "name": "restart-pod-workflow",
    "version": "v1.2.3",
    "namespace": "kubernaut-system"
  },
  "selection_reason": "Best match for OOMKill incident",
  "alternatives_considered": 3
}
```

**Note**: Existing `execution.workflow.started` event (DD-AUDIT-003 line 198) already captures `execution_ref` for SOC2 - no changes needed.

---

### **Proposal Option B: Replace Granular Events (NOT RECOMMENDED)** ❌

**NOT RECOMMENDED** because:
- ❌ Breaks existing operational visibility (lose granular LLM tracking)
- ❌ Requires refactoring existing audit implementations
- ❌ Higher risk of breaking existing integrations
- ❌ Loses valuable debugging information (LLM request/response granularity)

**Option A is superior**: Additive, low-risk, preserves existing value.

---

## 📊 **Impact Analysis**

### **Services Affected**

| Service | Change | Effort | Risk |
|---|---|---|---|
| **AI Analysis Controller** | Add `aianalysis.analysis.completed` event | 2h | LOW (additive) |
| **Workflow Execution Controller** | Add `workflow.selection.completed` event | 2h | LOW (new feature) |

**Total Effort**: 4 hours (already included in SOC2 Day 1-5 implementation)

### **DD-AUDIT-003 Version Update**

**Proposed Version**: 1.3 (minor addition)

**Version History Addition**:
```markdown
**Recent Changes** (v1.3 - January 4, 2026):
- **AI Analysis**: Added `aianalysis.analysis.completed` event for SOC2 compliance (BR-AUDIT-005 v2.0)
- **Workflow Execution**: Added `workflow.selection.completed` event for RR reconstruction (DD-AUDIT-004)
- **Rationale**: 100% RR CRD reconstruction from audit traces (enterprise compliance)
```

### **Event Volume Impact**

**Additional Events per Remediation**:
- `aianalysis.analysis.completed`: +1 event (already counted in AI Analysis volume)
- `workflow.selection.completed`: +1 event (new, adds ~300 events/day)

**Updated Volume Estimate**:
| Service | Events/Day (Before) | Events/Day (After) | Delta |
|---|---|---|---|
| AI Analysis | 500 | 500 | 0 (replaces existing) |
| Workflow Execution | 2,000 | 2,300 | +300 (+15%) |
| **TOTAL** | 11,700 | 12,000 | +300 (+2.6%) |

**Storage Impact**: Negligible (+0.9 MB/month, $0.0009/month)

---

## 🔗 **Related Documents to Update**

After DD-AUDIT-003 is updated:

1. **DD-AUDIT-004 (RR Reconstruction Field Mapping)**:
   - Update event type references to match DD-AUDIT-003 v1.3
   - Confirm field mappings align with new event definitions

2. **SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md**:
   - Reference DD-AUDIT-003 v1.3 as authority for event types
   - Add cross-reference links to DD-AUDIT-003 sections

3. **SOC2_AUDIT_IMPLEMENTATION_PLAN.md**:
   - Note DD-AUDIT-003 v1.3 as updated authority
   - Confirm implementation aligns with approved event types

4. **Service Implementation**:
   - `pkg/aianalysis/audit/audit.go`: Implement `aianalysis.analysis.completed`
   - `pkg/workflow/audit/audit.go`: Implement `workflow.selection.completed`
   - `pkg/workflow/audit/audit.go`: Verify `execution.workflow.started` already captures `execution_ref` (existing event)

---

## ✅ **Recommendation**

### **APPROVE: Proposal Option A (Add SOC2-Specific Events)**

**Justification**:
1. ✅ **Low Risk**: Additive only, no breaking changes
2. ✅ **Preserves Value**: Keeps existing granular events for operations
3. ✅ **Compliance-Focused**: Clear separation between operational and compliance events
4. ✅ **Minimal Effort**: 4-5 hours (already budgeted in SOC2 plan)
5. ✅ **Industry Standard**: Many systems have both operational and compliance audit trails

**Implementation Timing**:
- **Update DD-AUDIT-003**: Before Day 1 implementation (1 hour)
- **Implement Events**: During Days 1-5 per SOC2 plan
- **Version**: DD-AUDIT-003 v1.3

---

## 📋 **Proposed DD-AUDIT-003 v1.3 Diff**

### **Section 2: AI Analysis Controller** (after line 175)

```diff
 | `ai-analysis.llm.request_failed` | LLM API request failed | P0 |
+| `aianalysis.analysis.completed` | AI analysis completed with full Holmes response (SOC2) | **P0** |

**Industry Precedent**: OpenAI API logs, Anthropic Claude logs, AWS Bedrock audit logs

**Expected Volume**: 500 events/day, 15 MB/month

+**SOC2 Compliance** (January 2026):
+- `aianalysis.analysis.completed` captures complete `provider_data` for RR reconstruction
+- Complements granular `ai-analysis.*` events for operational visibility
+- Required for BR-AUDIT-005 v2.0 (100% RR reconstruction accuracy)
+
---
```

### **Section 3: Workflow Execution Controller** (after line 198)

```diff
 | `execution.workflow.started` | Tekton workflow started | P0 |
+| `workflow.selection.completed` | Workflow selected for remediation (SOC2) | **P0** |
 | `execution.action.executed` | Kubernetes action executed | P0 |
```

**Note**: Existing `execution.workflow.started` event already serves SOC2 need (captures `execution_ref`) - no change required.

### **Version History** (top of document)

```diff
 **Recent Changes** (v1.2):
 - **Gateway**: Removed deprecated `gateway.signal.storm_detected` event
 - **Remediation Orchestrator**: Added `orchestrator.routing.blocked` event
 - **Remediation Orchestrator**: Added approval lifecycle events
 - **Remediation Orchestrator**: Updated expected volume: 1,000 → 1,200 events/day
 - **Data Storage**: Removed meta-auditing events per DD-AUDIT-002 V2.0.1

+**Recent Changes** (v1.3 - January 4, 2026):
+- **AI Analysis**: Added `aianalysis.analysis.completed` event for SOC2 compliance
+- **Workflow Execution**: Added `workflow.selection.completed` event for RR reconstruction
+- **Rationale**: BR-AUDIT-005 v2.0 (100% RR CRD reconstruction from audit traces)
+- **Expected Volume**: +300 events/day (workflow selection tracking)
```

---

## 🎯 **Confidence Assessment**

**Confidence**: 90%

**Justification**:
- ✅ Clear business requirement (BR-AUDIT-005 v2.0)
- ✅ Aligns with DD-AUDIT-004 field mapping specification
- ✅ Low implementation risk (additive changes only)
- ✅ Industry precedent (separate operational vs compliance events)
- ⚠️ Minor uncertainty on exact event_data field structure (to be refined during implementation)

**Risk Assessment**:
- **Risk**: Event naming conflicts with existing conventions
- **Mitigation**: Clear documentation of SOC2-specific naming (`aianalysis.*` vs `ai-analysis.*`)
- **Probability**: LOW (naming conventions are clearly documented)

---

**Recommendation**: **Approve DD-AUDIT-003 v1.3 with Proposal Option A**
**Estimated Effort**: 1 hour (documentation update) + 4 hours (implementation, already budgeted)
**Timeline**: Update DD-AUDIT-003 before Day 1 SOC2 implementation
**Authority**: This proposal becomes authoritative once approved and DD-AUDIT-003 is updated

---

## ✅ **Implementation Status**

**Date Approved**: January 4, 2026
**Implementation**: ✅ **COMPLETE**

**Changes Made**:
1. ✅ Updated DD-AUDIT-003 to v1.3
2. ✅ Added `aianalysis.analysis.completed` event to AI Analysis Controller section
3. ✅ Added `workflow.selection.completed` event to Remediation Execution Controller section
4. ✅ Updated event volume estimates: 12,000 events/day (was 11,700)
5. ✅ Updated storage cost: $0.36/month (was $0.35/month)

**Authority**: DD-AUDIT-003 v1.3 is now the authoritative reference for all SOC2 audit event types

---

**Document Status**: ✅ **APPROVED & IMPLEMENTED**
**Next Action**: Proceed with SOC2 Day 1 implementation (Gateway signal data capture)
**Timeline Impact**: Zero (completed within proposal timeframe)

