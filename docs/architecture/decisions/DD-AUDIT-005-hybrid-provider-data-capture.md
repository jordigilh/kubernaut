# DD-AUDIT-005: Hybrid Provider Data Capture for AI Analysis Audit Trail

**Status**: Approved
**Created**: January 5, 2026
**Author**: AI Analysis + HolmesAPI Integration Team
**Business Requirement**: BR-AUDIT-005 v2.0 (Gap #4 - AI Provider Data)
**Related Documents**:
- [SOC2 Audit Test Plan](../../development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)
- [DD-AUDIT-002: Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md)

---

## Context

For SOC2 Type II compliance and RemediationRequest (RR) reconstruction, we need to capture complete AI provider response data in audit trails. The AI Analysis service depends on HolmesGPT API (HAPI) for root cause analysis and workflow recommendations.

**Problem**: Where should the audit event for AI provider data be emitted?
- **Option A**: AI Analysis Controller only (consumer captures response)
- **Option B**: HolmesAPI only (provider captures response)
- **Option C**: BOTH services (hybrid approach)

---

## Decision

**We will use a HYBRID approach where BOTH HolmesAPI and AI Analysis Controller emit audit events.**

### Event Distribution

| Service | Event Type | Purpose | Content |
|---------|-----------|---------|---------|
| **HolmesAPI** | `holmesgpt.response.complete` | Provider perspective | Full `IncidentResponse` structure |
| **AI Analysis** | `aianalysis.analysis.completed` | Consumer perspective | `provider_response_summary` + business context |

---

## Rationale

### Defense-in-Depth Auditing

**SOC2 Principle**: Multiple independent audit sources provide stronger compliance evidence than a single source.

1. **Provider Audit (HAPI)**:
   - ✅ Captures response **at the source** (API endpoint)
   - ✅ Complete `IncidentResponse` structure with all fields
   - ✅ Independent of consumer processing
   - ✅ Detects if AA receives incomplete data

2. **Consumer Audit (AA)**:
   - ✅ Captures **business context** (phase, approval, degraded mode)
   - ✅ Validates AA received the response
   - ✅ Summary format reduces audit storage
   - ✅ Confirms end-to-end flow

### Single Source of Truth

**HAPI owns the complete API response data** - AA references it by summary.

```sql
-- Full provider response (authoritative)
SELECT event_data->'response_data'
FROM audit_events
WHERE event_type = 'holmesgpt.response.complete'
  AND correlation_id = 'req-2025-01-05-abc123';

-- Business context (complementary)
SELECT event_data
FROM audit_events
WHERE event_type = 'aianalysis.analysis.completed'
  AND correlation_id = 'req-2025-01-05-abc123';
```

### Alternative Approaches Considered

#### Option A: AA Only (Consumer Captures)

**Pros**:
- ✅ Single audit event per analysis
- ✅ Rich business context
- ✅ Simpler query pattern

**Cons**:
- ❌ AA must store full response in CRD status (increases CRD size)
- ❌ No independent verification of HAPI → AA data transfer
- ❌ Duplicates HAPI data in AA service
- ❌ Single point of failure for audit capture

**Verdict**: ⛔ **REJECTED** - Violates single source of truth principle

---

#### Option B: HAPI Only (Provider Captures)

**Pros**:
- ✅ Single source of truth (HAPI owns response)
- ✅ No CRD status bloat
- ✅ Easy implementation (HAPI audit already exists)

**Cons**:
- ❌ No AA business context (phase, approval, degraded mode)
- ❌ Requires 2 queries for RR reconstruction (HAPI + AA events)
- ❌ Can't prove AA received the response

**Verdict**: ⚠️ **INSUFFICIENT** - Missing critical business context

---

#### Option C: HYBRID (Both Capture)

**Pros**:
- ✅ Defense-in-depth auditing
- ✅ Complete audit trail (provider + consumer)
- ✅ Each service owns its data
- ✅ Independent verification of data transfer
- ✅ Easier debugging (both perspectives)

**Cons**:
- ⚠️ 2 audit events per analysis (storage cost)
- ⚠️ Slightly more implementation work

**Verdict**: ✅ **SELECTED** - Best balance of compliance, debugging, and architecture

---

## Implementation

### HolmesAPI Changes

**File**: `holmesgpt-api/src/audit/events.py`

```python
def create_hapi_response_complete_event(
    incident_id: str,
    remediation_id: str,
    response_data: Dict[str, Any]  # Full IncidentResponse
) -> Dict[str, Any]:
    """
    Create Holmes API response completion audit event

    BR-AUDIT-005 Gap #4: Capture complete API response for SOC2 audit trail
    DD-AUDIT-005: Provider perspective audit (HAPI owns response data)
    """
    event_data_model = HAPIResponseEventData(
        event_id=str(uuid.uuid4()),
        incident_id=incident_id,
        response_data=response_data  # Full IncidentResponse
    )

    return _create_adr034_event(
        event_type="holmesgpt.response.complete",
        operation="response_sent",
        outcome="success",
        correlation_id=remediation_id,
        event_data=event_data_model.model_dump()
    )
```

**File**: `holmesgpt-api/src/extensions/incident/endpoint.py`

```python
@router.post("/incident/analyze")
async def incident_analyze_endpoint(request: IncidentRequest) -> IncidentResponse:
    result = await analyze_incident(request_data)

    # DD-AUDIT-005: Capture complete response for audit trail (provider perspective)
    audit_store = get_audit_store()
    audit_store.store_audit(create_hapi_response_complete_event(
        incident_id=request.incident_id,
        remediation_id=request.remediation_id,
        response_data=result.model_dump()
    ))

    return result
```

**Effort**: ~15 minutes

---

### AI Analysis Changes

**File**: `pkg/aianalysis/audit/event_types.go`

```go
// AnalysisCompletePayload is the structured payload for analysis completion events.
//
// Business Requirements:
// - BR-AI-001: Analysis completion tracking
// - BR-AUDIT-005 Gap #4: Provider data capture (DD-AUDIT-005 hybrid approach)
type AnalysisCompletePayload struct {
	// Core Status Fields
	Phase            string `json:"phase"`
	ApprovalRequired bool   `json:"approval_required"`
	ApprovalReason   string `json:"approval_reason,omitempty"`
	DegradedMode     bool   `json:"degraded_mode"`
	WarningsCount    int    `json:"warnings_count"`

	// Workflow Selection (conditional)
	Confidence         *float64 `json:"confidence,omitempty"`
	WorkflowID         *string  `json:"workflow_id,omitempty"`
	TargetInOwnerChain *bool    `json:"target_in_owner_chain,omitempty"`

	// Failure Information (conditional)
	Reason    string `json:"reason,omitempty"`
	SubReason string `json:"sub_reason,omitempty"`

	// DD-AUDIT-005: Provider response summary (consumer perspective)
	// NOTE: Full response is in holmesgpt.response.complete event (provider perspective)
	// This field provides AA-side context for what AA received
	ProviderResponseSummary *ProviderResponseSummary `json:"provider_response_summary,omitempty"`
}

// ProviderResponseSummary provides AA's perspective of Holmes response
// DD-AUDIT-005: Consumer-side summary (full data in holmesgpt.response.complete)
type ProviderResponseSummary struct {
	IncidentID         string  `json:"incident_id"`
	AnalysisPreview    string  `json:"analysis_preview"`          // First 500 chars
	SelectedWorkflowID *string `json:"selected_workflow_id,omitempty"`
	NeedsHumanReview   bool    `json:"needs_human_review"`
	WarningsCount      int     `json:"warnings_count"`
}
```

**File**: `pkg/aianalysis/audit/audit.go`

```go
func (c *AuditClient) RecordAnalysisComplete(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
	// Build structured payload
	payload := AnalysisCompletePayload{
		Phase:            analysis.Status.Phase,
		ApprovalRequired: analysis.Status.ApprovalRequired,
		// ... existing fields ...
	}

	// DD-AUDIT-005: Add provider response summary (consumer perspective)
	if analysis.Status.HolmesResponse != nil {
		summary := &ProviderResponseSummary{
			IncidentID:       analysis.Spec.IncidentID,
			AnalysisPreview:  truncateString(analysis.Status.HolmesResponse.Analysis, 500),
			NeedsHumanReview: analysis.Status.HolmesResponse.NeedsHumanReview,
			WarningsCount:    len(analysis.Status.HolmesResponse.Warnings),
		}
		if analysis.Status.SelectedWorkflow != nil {
			summary.SelectedWorkflowID = &analysis.Status.SelectedWorkflow.WorkflowID
		}
		payload.ProviderResponseSummary = summary
	}

	// ... rest of audit emission ...
}
```

**Effort**: ~30 minutes

---

## Testing Strategy

### Integration Test Validation

**File**: `test/integration/aianalysis/audit_provider_data_integration_test.go`

```go
It("should capture Holmes response in BOTH HAPI and AA audit events", func() {
    // Create AIAnalysis CRD
    aiAnalysis := createTestAIAnalysis()
    err := k8sClient.Create(ctx, aiAnalysis)
    Expect(err).ToNot(HaveOccurred())

    // Wait for completion
    Eventually(func() string {
        var updated aianalysisv1alpha1.AIAnalysis
        k8sClient.Get(ctx, client.ObjectKeyFromObject(aiAnalysis), &updated)
        return updated.Status.Phase
    }, 60*time.Second, 2*time.Second).Should(Equal("Completed"))

    correlationID := aiAnalysis.Spec.RemediationID

    // Verify HAPI audit event (provider perspective)
    hapiEvents := waitForAuditEvents(correlationID, "holmesgpt.response.complete", 1)
    hapiEventData := hapiEvents[0].EventData.(map[string]interface{})
    Expect(hapiEventData).To(HaveKey("response_data"))

    responseData := hapiEventData["response_data"].(map[string]interface{})
    Expect(responseData).To(HaveKey("root_cause_analysis"))
    Expect(responseData).To(HaveKey("selected_workflow"))
    Expect(responseData).To(HaveKey("alternative_workflows"))

    // Verify AA audit event (consumer perspective)
    aaEvents := waitForAuditEvents(correlationID, "aianalysis.analysis.completed", 1)
    aaEventData := aaEvents[0].EventData.(map[string]interface{})
    Expect(aaEventData).To(HaveKey("provider_response_summary"))

    summary := aaEventData["provider_response_summary"].(map[string]interface{})
    Expect(summary).To(HaveKey("analysis_preview"))
    Expect(summary).To(HaveKey("selected_workflow_id"))

    // Verify AA business context (not in HAPI event)
    Expect(aaEventData).To(HaveKey("phase"))
    Expect(aaEventData).To(HaveKey("approval_required"))
    Expect(aaEventData).To(HaveKey("degraded_mode"))
})
```

---

## Compliance Impact

### SOC2 Type II Audit Trail

**Requirement**: Complete RemediationRequest reconstruction from audit traces

**Compliance Score**: ✅ **ENHANCED**

| Audit Aspect | Single Event | Hybrid Events |
|--------------|-------------|---------------|
| **Provider Data Integrity** | ⚠️ Unverified | ✅ HAPI event proves source |
| **Consumer Processing** | ⚠️ Assumed | ✅ AA event proves receipt |
| **Data Transfer Validation** | ❌ No proof | ✅ Both events prove transfer |
| **Debugging** | 🔴 Limited context | ✅ Full provider + consumer views |
| **Audit Independence** | 🔴 Single source | ✅ Multiple independent sources |

---

## Performance Impact

### Storage Cost Analysis

**Assumptions**:
- Average IncidentResponse size: ~2 KB (full response)
- Average summary size: ~200 bytes (10% of full response)
- Analysis rate: 100 analyses/day

**Daily Storage**:
- **Single Event (AA only)**: 100 × 2 KB = 200 KB/day
- **Hybrid Events**: 100 × (2 KB + 200 bytes) = 220 KB/day
- **Overhead**: 20 KB/day (10% increase)

**Annual Storage Overhead**: ~7.3 MB/year

**Verdict**: ✅ **NEGLIGIBLE** - 10% storage increase is acceptable for enhanced compliance

---

## Consequences

### Positive

1. ✅ **Enhanced SOC2 Compliance**: Multiple independent audit sources
2. ✅ **Easier Debugging**: Both provider and consumer perspectives available
3. ✅ **Single Source of Truth**: HAPI owns complete response data
4. ✅ **Business Context**: AA provides phase, approval, degraded mode info
5. ✅ **Data Transfer Validation**: Can prove HAPI → AA data integrity

### Negative

1. ⚠️ **Additional Storage**: 10% increase in audit event storage
2. ⚠️ **Two Queries for RR Reconstruction**: Must query both event types
3. ⚠️ **Implementation Complexity**: Requires changes in both services

### Mitigation

- **Storage**: Negligible cost (~7.3 MB/year) vs. compliance benefit
- **Query Complexity**: Provide helper functions in DD-TESTING-001
- **Implementation**: Clear separation of concerns (provider vs. consumer)

---

## Alternatives

If storage cost becomes a concern in the future, we can:

1. **Compress HAPI response_data** using gzip before storing
2. **Archive old audit events** to cold storage after 90 days
3. **Sample audit events** for non-production environments

**Note**: These are FUTURE optimizations. Current cost is negligible.

---

## Related Decisions

- [DD-AUDIT-002: Audit Shared Library Design](./DD-AUDIT-002-audit-shared-library.md)
- [DD-AUDIT-003: Service Audit Trace Requirements](./DD-AUDIT-003-service-audit-trace-requirements.md)
- [DD-AUDIT-004: Structured Event Data Standards](./DD-AUDIT-004-structured-event-data-standards.md)

---

## References

- [BR-AUDIT-005 v2.0: Audit Event Gaps for RR Reconstruction](../../requirements/11_SECURITY_ACCESS_CONTROL.md)
- [SOC2 Audit Test Plan v2.1.0](../../development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)
- [ADR-034: Unified Audit Table Design](./ADR-034-unified-audit-table.md)
- [ADR-038: Asynchronous Buffered Audit Trace Ingestion](./ADR-038-async-buffered-audit-ingestion.md)

