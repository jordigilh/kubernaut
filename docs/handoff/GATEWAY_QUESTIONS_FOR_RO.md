# Gateway Team Questions for RO Team

**From**: Gateway Team
**To**: Remediation Orchestrator Team
**Date**: December 1, 2025
**Status**: ✅ **RESOLVED** (December 2, 2025)
**Context**: Follow-up to RO_CONTRACT_GAPS implementation

---

## Summary

Gateway has completed implementation of all three contract gaps (GAP-C1-02, GAP-C1-03, GAP-C1-04). The following questions require RO team confirmation before we consider the integration complete.

---

## Q1: Shared DeduplicationInfo Location

**Question**: Is the shared type location acceptable?

**Current Location**: `pkg/shared/types/deduplication.go`

```go
package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type DeduplicationInfo struct {
    IsDuplicate                   bool        `json:"isDuplicate,omitempty"`
    FirstOccurrence               metav1.Time `json:"firstOccurrence"`
    LastOccurrence                metav1.Time `json:"lastOccurrence"`
    OccurrenceCount               int         `json:"occurrenceCount"`
    CorrelationID                 string      `json:"correlationId,omitempty"`
    PreviousRemediationRequestRef string      `json:"previousRemediationRequestRef,omitempty"`
}
```

**Options**:
- [x] A) Location is acceptable as-is ✅
- [ ] B) Move to different location (please specify): _______________
- [ ] C) Other concerns: _______________

**RO Response**: ✅ **A) Location is acceptable as-is**
- `pkg/shared/types/` is the established location for cross-CRD shared types
- `enrichment.go` already exists here for `EnrichmentResults`
- Import path `sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"` is consistent
- Both RemediationRequest and SignalProcessing already import from this package

---

## Q2: Breaking JSON Field Changes

**Question**: Are there any existing consumers that need notification?

**Changes Made**:
| Old Field | New Field |
|-----------|-----------|
| `firstSeen` | `firstOccurrence` |
| `lastSeen` | `lastOccurrence` |

**Affected CRDs**:
- `RemediationRequest` (Gateway-owned)
- `SignalProcessing` (SP team notified separately)

**Response**:
- [x] A) No existing consumers - proceed ✅
- [ ] B) Notify these consumers: _______________
- [ ] C) Need migration strategy for: _______________

**RO Response**: ✅ **A) No existing consumers - proceed**
- Pre-release product (ADR-001): No backward compatibility required
- Only consumers are RO, SP, and tests (all under our control)
- Change is internal to Kubernaut ecosystem
- New naming (`*Occurrence`) is semantically clearer

---

## Q3: TargetResource Default Handling

**Question**: Is the default for signals without resource info acceptable?

**Current Behavior**: When a signal has no resource info (edge case), Gateway populates:

```go
TargetResource: ResourceIdentifier{
    Kind:      "Unknown",
    Name:      "unknown",
    Namespace: signal.Namespace, // Falls back to signal's namespace
}
```

**Use Cases**:
- Non-Kubernetes signals (future V2 support for AWS/Datadog)
- Malformed alerts missing resource labels

**Options**:
- [ ] A) Default is acceptable - RO can handle "Unknown" kind
- [x] B) Gateway should reject such signals (HTTP 400) ✅
- [ ] C) Use different default values: _______________
- [ ] D) Make TargetResource optional again for these cases

**RO Response**: ✅ **B) Gateway should reject such signals (HTTP 400)**
- V1.0 is **Kubernetes-only** (per project scope)
- Non-Kubernetes signals are explicitly out of scope until V2.0
- Malformed alerts missing resource labels indicate a configuration issue at the source
- Rejecting early provides clear feedback to alert authors
- "Unknown" kind would break downstream processing:
  - SignalProcessing enrichment expects valid K8s resources
  - AIAnalysis root cause analysis requires actual resource context
  - WorkflowExecution needs specific target for remediation

**V2.0 consideration**: When non-Kubernetes targets are added, this logic should be revisited.

---

### Gateway Implementation Status (December 2, 2025)

✅ **IMPLEMENTED** in `pkg/gateway/processing/crd_creator.go`:

**Validation Function Added**:
```go
// validateResourceInfo validates that signal has required resource info for V1.0 (Kubernetes-only)
// - Resource.Kind is REQUIRED (e.g., "Pod", "Deployment", "Node")
// - Resource.Name is REQUIRED (e.g., "payment-api-789", "worker-node-1")
// - Resource.Namespace may be empty for cluster-scoped resources (e.g., Node, ClusterRole)
func (c *CRDCreator) validateResourceInfo(signal *types.NormalizedSignal) error {
    var missingFields []string
    if signal.Resource.Kind == "" {
        missingFields = append(missingFields, "Kind")
    }
    if signal.Resource.Name == "" {
        missingFields = append(missingFields, "Name")
    }
    if len(missingFields) > 0 {
        return fmt.Errorf("resource validation failed: missing required fields [%s] - V1.0 requires valid Kubernetes resource info",
            strings.Join(missingFields, ", "))
    }
    return nil
}
```

**Business Outcome**:
- ✅ Alert sources receive clear HTTP 400 error with message identifying missing fields
- ✅ No more "Unknown" kind defaults polluting downstream processing
- ✅ SignalProcessing, AIAnalysis, and WorkflowExecution receive validated resource info
- ✅ Unit tests added validating rejection behavior (BR-GATEWAY-TARGET-RESOURCE-VALIDATION)
- ✅ Cluster-scoped resources (empty Namespace) are correctly accepted

---

## Q4: ProviderData Contents

**Question**: Does RO need any additional data from ProviderData?

**Current ProviderData Contents** (after removing resource info):
```json
{
  "namespace": "signal-namespace",
  "labels": {
    "alertname": "...",
    "severity": "...",
    // ... other provider labels
  }
}
```

**Removed from ProviderData** (now in `spec.targetResource`):
- `resource_kind`
- `resource_name`
- `resource_namespace`

**Response**:
- [x] A) Current contents are sufficient ✅
- [ ] B) Need additional fields: _______________
- [ ] C) Need to restructure ProviderData: _______________

**RO Response**: ✅ **A) Current contents are sufficient**
- Resource info is now in `spec.targetResource` (no duplication needed)
- Provider-specific labels are captured for audit/debugging
- External links (AlertmanagerURL, GrafanaURL) are available for notifications

---

## Response Template

### RO Team Response ✅ COMPLETE

**Date**: December 2, 2025
**Respondent**: Remediation Orchestrator Team

| Question | Response | Notes |
|----------|----------|-------|
| Q1 (Location) | **A** ✅ | Location is acceptable as-is |
| Q2 (Breaking Changes) | **A** ✅ | No existing consumers - proceed |
| Q3 (Default Handling) | **B** ✅ | Gateway should reject (HTTP 400) |
| Q4 (ProviderData) | **A** ✅ | Current contents are sufficient |
| Q5 (ProviderData Usage) | **A** ✅ | Audit only - no parsing needed |
| Q6 (Storm Detection) | **A** ✅ | Informational only for V1 |
| Q7 (Rejected Signals) | **A** ✅ | HTTP 400 + logs sufficient |

**Additional Comments**: Thank you for the thorough contract alignment work. All changes follow established patterns.

---

## Additional Questions (December 2, 2025)

### Q5: ProviderData Usage Clarification

**Context**: After the contract alignment, `ProviderData` now contains:
```json
{
  "namespace": "signal-namespace",
  "labels": {
    "alertname": "...",
    "severity": "...",
    // ... provider-specific labels
  }
}
```

**Question**: Does RO or any downstream service actively parse `ProviderData`, or is it primarily for audit/debugging?

**Why it matters**: If it's actively parsed, we should document the expected structure. If it's just audit data, we can keep it flexible.

**Options**:
- [x] A) Audit only - no parsing needed, keep flexible ✅
- [ ] B) Actively parsed - need to document structure
- [ ] C) Specific fields needed: _______________

**RO Response**: ✅ **A) Audit only - no parsing needed, keep flexible**
- RO parses ProviderData only for **notification context** (external links)
- These are **optional convenience fields**, not critical path
- No downstream service (SP, AIAnalysis, WE) parses ProviderData
- Primary structured data is in dedicated spec fields (`targetResource`, `signalLabels`, `deduplication`)

**Recommendation**: Keep ProviderData flexible as `[]byte` (JSON blob). Document as "provider-specific audit data."

---

### Q6: Storm Detection Field Consumption

**Context**: Gateway populates these storm detection fields in `RemediationRequest.Spec`:
- `IsStorm` (bool)
- `StormType` (string: "same-resource", "cross-resource", "cascading")
- `StormWindow` (string)
- `StormAlertCount` (int)
- `AffectedResources` ([]string)

**Question**: Does RO handle storm-aggregated signals differently? Or are these fields purely informational?

**Options**:
- [x] A) Informational only - RO treats all signals the same ✅
- [ ] B) Special handling - describe: _______________
- [ ] C) Future feature - document expected behavior for V2

**RO Response**: ✅ **A) Informational only - RO treats all signals the same**
- Storm detection/aggregation is Gateway's responsibility (pre-CRD creation)
- RO receives one RemediationRequest per signal (or aggregated storm)
- Storm fields (`isStorm`, `stormType`, `stormWindow`, `stormAlertCount`) are:
  - Stored for audit trail
  - Included in notifications (context for operators)
  - NOT used for processing decisions

**V1.0 Processing Flow**:
```
Gateway:    Signal → [Storm Detection] → [Aggregation] → ONE RemediationRequest
RO:         RemediationRequest → [Standard Processing] → Child CRDs
```

**Future consideration (V2.0)**: If we need storm-specific handling (e.g., different timeout, batch workflow), we can add logic based on these fields.

---

### Q7: Rejected Signals Feedback Loop

**Context**: When Gateway receives malformed signals or signals that fail validation, we return HTTP 400 and log the error. However, there's no feedback mechanism to notify operators.

**Question**: Should we define a feedback mechanism for rejected signals?

**Options**:
- [x] A) Not needed - HTTP 400 + logs sufficient ✅
- [ ] B) Create `RejectedSignal` CRD for observability
- [ ] C) Emit Kubernetes Event on rejection
- [ ] D) Other: _______________

**RO Response**: ✅ **A) Not needed - HTTP 400 + logs sufficient**
- Alert sources (Alertmanager, etc.) receive HTTP 400 response
- Gateway logs include detailed error information
- Operators can monitor Gateway logs/metrics for rejection rates
- Creating CRDs for rejections adds complexity without clear benefit

**Already available observability**:
- HTTP 400 response to caller (immediate feedback)
- Gateway metrics: `gateway_signals_rejected_total` (by reason)
- Gateway logs: Structured logging with signal details

**Future consideration (V2.0)**: If operators need better visibility into rejected signals, consider **Option C (Kubernetes Event)** - provides kubectl-native visibility without CRD overhead.

---

## Suggestions (For Discussion)

### ~~S1: Contract Version Header~~ - WITHDRAWN

**Original Suggestion**: Add `contractVersion` field to CRD specs for runtime mismatch detection.

**Status**: ❌ **WITHDRAWN** after reviewing ADR-001 (CRD Spec Immutability Design Principle)

**Rationale**: ADR-001 establishes that CRD specs are **immutable by design**:
- Specs represent immutable events (not mutable resources)
- Immutability prevents race conditions, status-spec inconsistency, audit trail gaps
- Since specs can't change after creation, runtime schema negotiation is unnecessary

The existing immutability design adequately addresses schema consistency concerns.

---

### S2: Integration Test Suite - DEFERRED

**Original Suggestion**: Create a shared integration test validating full data flow:
```
Gateway → RemediationRequest → RO → SignalProcessing → AIAnalysis → WorkflowExecution
```

**Status**: ⏸️ **DEFERRED** - Good idea, but premature until services are implemented.

**Action**: Revisit when all CRD controllers are built.

---

## Reference

- **Implementation PR**: [Gateway RO Contract Gaps Implementation]
- **Related Documents**:
  - `pkg/shared/types/deduplication.go` (shared type)
  - `docs/architecture/decisions/ADR-001-crd-microservices-architecture.md` (CRD immutability)

---

**Document Version**: 1.1
**Last Updated**: December 2, 2025
**Changelog**:
- v1.1: Added Q5-Q7, S1 withdrawn (ADR-001), S2 deferred
- v1.0: Initial questions (Q1-Q4)


