# BR-WORKFLOW-002: Workflow Versioning & Deprecation Lifecycle

> **SUPERSEDED** by [DD-WORKFLOW-017](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) (Workflow Lifecycle Component Interactions, February 2026).
>
> DD-WORKFLOW-017 defines the authoritative status lifecycle (active/disabled/deprecated) and transition rules. The `draft` status, `promote` endpoint, automated deprecation based on success rate thresholds, and version comparison API defined in this document are no longer valid. Deprecation is an operator decision based on their observability, not an automated system action. Refer to DD-WORKFLOW-017 Phase 4 for the current operational management design.

**Business Requirement ID**: BR-WORKFLOW-002
**Category**: Workflow Catalog Service
**Priority**: P1
**Target Version**: V1
**Status**: **SUPERSEDED** by DD-WORKFLOW-017
**Date**: November 5, 2025

---

## 📋 **Business Need**

### **Problem Statement**

ADR-033 requires workflow versioning with effectiveness-based deprecation. The Workflow Catalog Service must manage playbook lifecycle transitions (draft → active → deprecated) based on effectiveness data from the Effectiveness Monitor, ensuring AI always selects the most effective playbook versions.

**Current Limitations**:
- ❌ No automated playbook deprecation based on effectiveness data
- ❌ Manual playbook lifecycle management (error-prone)
- ❌ AI may select deprecated or ineffective playbook versions
- ❌ No version comparison workflow for promoting new versions
- ❌ No audit trail for deprecation decisions

**Impact**:
- Degraded playbooks remain active (e.g., v1.1 with 40% success rate)
- Manual coordination required for version transitions
- AI effectiveness compromised by outdated playbooks
- No data-driven playbook lifecycle management

---

## 🎯 **Business Objective**

**Provide automated playbook lifecycle management with effectiveness-based deprecation, version comparison, and audit trails to ensure AI always uses the most effective playbook versions.**

### **Success Criteria**
1. ✅ Playbook lifecycle: draft → active → deprecated (state machine enforced)
2. ✅ API to promote playbook from draft to active
3. ✅ API to deprecate playbook with effectiveness-based justification
4. ✅ Automated deprecation recommendation (if success rate <50% for 7d)
5. ✅ Version comparison API (compare v1.1 vs v1.2 effectiveness)
6. ✅ Deprecation audit trail (who, when, why)
7. ✅ AI excludes deprecated playbooks from selection queries

---

## 📊 **Use Cases**

### **Use Case 1: Promote New Playbook Version**

**Scenario**: Team creates `pod-oom-recovery v1.3` and wants to promote it from draft to active after validation.

**Current Flow** (Without BR-WORKFLOW-002):
```
1. Team creates v1.3 (status: draft)
2. Team manually tests v1.3
3. ❌ Manual status update (no validation)
4. ❌ No comparison with existing active versions
5. ❌ Risk of activating untested playbook
```

**Desired Flow with BR-WORKFLOW-002**:
```
1. Team creates v1.3 (status: draft)
2. RemediationExecutor executes v1.3 in "testing" mode (10% of traffic)
3. Effectiveness Monitor tracks v1.3 success rate: 95% (20 executions)
4. Team queries version comparison:
   GET /api/v1/playbooks/pod-oom-recovery/compare?version1=v1.2&version2=v1.3
   Response:
   {
     "v1.2": {"success_rate": 0.89, "executions": 90, "status": "active"},
     "v1.3": {"success_rate": 0.95, "executions": 20, "status": "draft"},
     "recommendation": "v1.3 shows 6% improvement - promote to active"
   }
5. Team promotes v1.3:
   PATCH /api/v1/playbooks/pod-oom-recovery/versions/v1.3
   Body: {"status": "active", "reason": "95% success rate, 6% improvement over v1.2"}
6. ✅ v1.3 promoted to active
7. ✅ AI immediately starts selecting v1.3 (highest success rate)
8. ✅ Audit trail: User X promoted v1.3 on date Y with reason Z
```

---

### **Use Case 2: Automated Deprecation Recommendation**

**Scenario**: `pod-oom-recovery v1.1` has 40% success rate for 7 days. Effectiveness Monitor recommends deprecation.

**Current Flow**:
```
1. v1.1 success rate degrades to 40%
2. No automated recommendation
3. ❌ v1.1 remains active
4. ❌ AI continues selecting v1.1
5. ❌ Poor remediation outcomes
```

**Desired Flow with BR-WORKFLOW-002**:
```
1. Effectiveness Monitor detects: v1.1 success rate = 40% (7d window)
2. Effectiveness Monitor creates deprecation recommendation:
   POST /api/v1/playbooks/pod-oom-recovery/versions/v1.1/deprecation-recommendations
   Body: {
     "reason": "Success rate below 50% threshold for 7 days",
     "current_success_rate": 0.40,
     "recommended_alternative": "v1.2 (89% success rate)",
     "severity": "high"
   }
3. Dashboard displays alert: "v1.1 recommended for deprecation (40% success rate)"
4. Team reviews recommendation and approves:
   PATCH /api/v1/playbooks/pod-oom-recovery/versions/v1.1
   Body: {"status": "deprecated", "reason": "Low success rate (40%), superseded by v1.2"}
5. ✅ v1.1 deprecated
6. ✅ AI excludes v1.1 from queries (only selects active playbooks)
7. ✅ Audit trail: User Y deprecated v1.1 based on automated recommendation
```

---

### **Use Case 3: Version Comparison Before Deprecation**

**Scenario**: Team wants to deprecate v1.0 but needs to ensure v1.2 is a suitable replacement.

**Current Flow**:
```
1. Team wants to deprecate v1.0
2. No version comparison available
3. ❌ Manual effectiveness analysis required
4. ❌ Risk of deprecating without suitable alternative
```

**Desired Flow with BR-WORKFLOW-002**:
```
1. Team queries version comparison:
   GET /api/v1/playbooks/pod-oom-recovery/compare?version1=v1.0&version2=v1.2
   Response:
   {
     "v1.0": {
       "success_rate": 0.50,
       "executions": 100,
       "status": "active",
       "trend": "stable"
     },
     "v1.2": {
       "success_rate": 0.89,
       "executions": 90,
       "status": "active",
       "trend": "stable"
     },
     "comparison": {
       "success_rate_difference": +39%,
       "recommendation": "v1.2 is 39% more effective - safe to deprecate v1.0"
     }
   }
2. ✅ Team validates: v1.2 is significantly better (39% improvement)
3. Team deprecates v1.0:
   PATCH /api/v1/playbooks/pod-oom-recovery/versions/v1.0
   Body: {"status": "deprecated", "reason": "Superseded by v1.2 (39% improvement)"}
4. ✅ v1.0 deprecated with data-driven justification
5. ✅ Audit trail includes comparison data
```

---

## 🔧 **Functional Requirements**

### **FR-PLAYBOOK-002-01: Lifecycle State Machine**

**Requirement**: Workflow Catalog SHALL enforce playbook lifecycle state machine.

**State Transitions**:
```
draft → active (promotion)
active → deprecated (deprecation)
deprecated → (no transitions - immutable)
```

**Implementation**:
```go
package playbookcatalog

type PlaybookStatus string

const (
    PlaybookStatusDraft      PlaybookStatus = "draft"
    PlaybookStatusActive     PlaybookStatus = "active"
    PlaybookStatusDeprecated PlaybookStatus = "deprecated"
)

// ValidateStatusTransition validates playbook status transitions
func ValidateStatusTransition(currentStatus, newStatus PlaybookStatus) error {
    switch currentStatus {
    case PlaybookStatusDraft:
        if newStatus == PlaybookStatusActive {
            return nil // ✅ draft → active
        }
        return fmt.Errorf("invalid transition: draft can only transition to active")

    case PlaybookStatusActive:
        if newStatus == PlaybookStatusDeprecated {
            return nil // ✅ active → deprecated
        }
        return fmt.Errorf("invalid transition: active can only transition to deprecated")

    case PlaybookStatusDeprecated:
        return fmt.Errorf("invalid transition: deprecated playbooks cannot be reactivated")

    default:
        return fmt.Errorf("unknown status: %s", currentStatus)
    }
}
```

**Acceptance Criteria**:
- ✅ `draft → active` allowed
- ✅ `active → deprecated` allowed
- ✅ `deprecated → active` **FORBIDDEN** (no reactivation)
- ✅ `draft → deprecated` **FORBIDDEN** (must activate first)
- ✅ Returns error for invalid transitions

---

### **FR-PLAYBOOK-002-02: Promote Workflow API**

**Requirement**: Workflow Catalog SHALL provide API to promote playbook from draft to active.

**API Specification**:
```http
PATCH /api/v1/playbooks/{playbook_id}/versions/{version}/promote

Request Body:
{
  "reason": "95% success rate in testing, 6% improvement over v1.2",
  "promoted_by": "user@company.com"
}

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.3",
  "status": "active",
  "promoted_at": "2025-11-05T10:00:00Z",
  "promoted_by": "user@company.com",
  "reason": "95% success rate in testing, 6% improvement over v1.2"
}
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid promotion
- ✅ Returns 400 Bad Request if current status != draft
- ✅ Returns 400 Bad Request if reason missing
- ✅ Auto-populates `promoted_at` timestamp
- ✅ Logs promotion in audit trail

---

### **FR-PLAYBOOK-002-03: Deprecate Workflow API**

**Requirement**: Workflow Catalog SHALL provide API to deprecate playbook.

**API Specification**:
```http
PATCH /api/v1/playbooks/{playbook_id}/versions/{version}/deprecate

Request Body:
{
  "reason": "Low success rate (40%), superseded by v1.2",
  "deprecated_by": "user@company.com",
  "replacement_version": "v1.2"  // Optional
}

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version": "v1.1",
  "status": "deprecated",
  "deprecated_at": "2025-11-05T10:00:00Z",
  "deprecated_by": "user@company.com",
  "reason": "Low success rate (40%), superseded by v1.2",
  "replacement_version": "v1.2"
}
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid deprecation
- ✅ Returns 400 Bad Request if current status != active
- ✅ Returns 400 Bad Request if reason missing
- ✅ Auto-populates `deprecated_at` timestamp
- ✅ Logs deprecation in audit trail
- ✅ AI excludes deprecated playbooks from queries

---

### **FR-PLAYBOOK-002-04: Version Comparison API**

**Requirement**: Workflow Catalog SHALL provide API to compare two playbook versions.

**API Specification**:
```http
GET /api/v1/playbooks/{playbook_id}/compare

Query Parameters:
- version1 (string, required): First version to compare
- version2 (string, required): Second version to compare

Response (200 OK):
{
  "playbook_id": "pod-oom-recovery",
  "version1": {
    "version": "v1.1",
    "success_rate": 0.40,
    "executions": 10,
    "status": "active",
    "trend": "stable",
    "effectiveness_score": 0.35
  },
  "version2": {
    "version": "v1.2",
    "success_rate": 0.89,
    "executions": 90,
    "status": "active",
    "trend": "stable",
    "effectiveness_score": 0.96
  },
  "comparison": {
    "success_rate_difference_percent": +122.5,  // (0.89 - 0.40) / 0.40 * 100
    "execution_volume_difference": +80,
    "effectiveness_score_difference": +0.61,
    "recommendation": "v1.2 is significantly more effective - consider deprecating v1.1"
  }
}
```

**Acceptance Criteria**:
- ✅ Returns 200 OK for valid comparison
- ✅ Returns 404 Not Found if either version doesn't exist
- ✅ Queries effectiveness data from Effectiveness Monitor
- ✅ Calculates success_rate_difference_percent
- ✅ Provides actionable recommendation

---

### **FR-PLAYBOOK-002-05: Automated Deprecation Recommendation**

**Requirement**: Effectiveness Monitor SHALL automatically recommend deprecation for low-performing playbooks.

**Deprecation Criteria**:
- Success rate <50% for 7 consecutive days
- At least 20 executions in 7-day period (statistical significance)
- Alternative version exists with >10% higher success rate

**Implementation**:
```go
// GenerateDeprecationRecommendations identifies playbooks for deprecation
func (em *EffectivenessMonitor) GenerateDeprecationRecommendations(ctx context.Context) ([]DeprecationRecommendation, error) {
    var recommendations []DeprecationRecommendation

    // Query all active playbooks
    playbooks, err := em.playbookCatalogClient.ListPlaybooks(ctx, "", "active")
    if err != nil {
        return nil, err
    }

    for _, playbook := range playbooks {
        // Get 7-day success rate
        successRate, err := em.contextAPIClient.GetSuccessRateByPlaybook(ctx, playbook.PlaybookID, playbook.Version, "7d")
        if err != nil {
            continue
        }

        // Check deprecation criteria
        if successRate.SuccessRate < 0.50 && successRate.TotalExecutions >= 20 {
            // Find alternative version
            alternative, err := em.findBetterAlternative(ctx, playbook.PlaybookID, successRate.SuccessRate)
            if err == nil && alternative != nil {
                recommendations = append(recommendations, DeprecationRecommendation{
                    PlaybookID:            playbook.PlaybookID,
                    Version:               playbook.Version,
                    Reason:                fmt.Sprintf("Success rate %.1f%% below 50%% threshold for 7 days", successRate.SuccessRate*100),
                    CurrentSuccessRate:    successRate.SuccessRate,
                    RecommendedAlternative: alternative.Version,
                    AlternativeSuccessRate: alternative.SuccessRate,
                    Severity:              "high",
                })
            }
        }
    }

    return recommendations, nil
}
```

**Acceptance Criteria**:
- ✅ Runs daily at midnight (batch job)
- ✅ Recommends deprecation if success rate <50% (7d, ≥20 executions)
- ✅ Includes alternative version recommendation
- ✅ Creates notification for dashboard
- ✅ Does NOT auto-deprecate (requires human approval)

---

## 📈 **Non-Functional Requirements**

### **NFR-PLAYBOOK-002-01: Audit Trail**

- ✅ All status transitions logged with timestamp, user, reason
- ✅ Audit log retained for 1 year
- ✅ Immutable audit records (no updates/deletes)

### **NFR-PLAYBOOK-002-02: Consistency**

- ✅ AI queries exclude deprecated playbooks (status=active only)
- ✅ Version comparison queries Effectiveness Monitor (real-time data)
- ✅ State transitions atomic (no partial updates)

### **NFR-PLAYBOOK-002-03: Performance**

- ✅ Version comparison response time <300ms
- ✅ Deprecation recommendation batch job completes <5 minutes
- ✅ Status update response time <100ms

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ ADR-033: Playbook lifecycle management
- ✅ BR-WORKFLOW-001: Playbook registry (provides playbook storage)
- ✅ BR-EFFECTIVENESS-002: Provides effectiveness data for recommendations

### **Downstream Impacts**
- ✅ BR-AI-057: AI queries only active playbooks
- ✅ BR-EFFECTIVENESS-003: Uses deprecation recommendations for feedback loops

---

## 🚀 **Implementation Phases**

### **Phase 1: State Machine** (Day 8 - 3 hours)
- Implement `ValidateStatusTransition()` function
- Add status validation to update API
- Unit tests (10+ test cases)

### **Phase 2: Promote/Deprecate APIs** (Day 8 - 4 hours)
- Implement promote endpoint
- Implement deprecate endpoint
- Add audit trail logging
- Integration tests

### **Phase 3: Version Comparison** (Day 9 - 4 hours)
- Implement comparison API
- Integrate with Effectiveness Monitor
- Calculate comparison metrics
- Unit + integration tests

### **Phase 4: Automated Recommendations** (Day 9 - 3 hours)
- Implement deprecation recommendation batch job
- Add daily scheduler
- Add notification generation

**Total Estimated Effort**: 14 hours (1.75 days)

---

## 📊 **Success Metrics**

### **Deprecation Accuracy**
- **Target**: 90%+ of deprecated playbooks have <50% success rate
- **Measure**: Track success rate at deprecation time

### **Lifecycle Compliance**
- **Target**: 100% of status transitions follow state machine
- **Measure**: Zero invalid transition attempts

### **Recommendation Adoption**
- **Target**: 70%+ of automated recommendations result in deprecation
- **Measure**: Track recommendation → deprecation rate

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Auto-Deprecation (No Human Approval)**

**Approach**: Automatically deprecate playbooks without human approval

**Rejected Because**:
- ❌ Risk of false positives (e.g., temporary infrastructure issue)
- ❌ No human review of impact
- ❌ Cannot override recommendation for business reasons

---

### **Alternative 2: No State Machine (Free-Form Status)**

**Approach**: Allow any status transition

**Rejected Because**:
- ❌ Risk of reactivating deprecated playbooks
- ❌ Inconsistent lifecycle management
- ❌ No audit trail enforcement

---

## ✅ **Approval**

**Status**: ✅ **APPROVED FOR V1**
**Date**: November 5, 2025
**Decision**: Implement as P1 priority (enables effectiveness-based lifecycle management)
**Rationale**: Required for continuous playbook improvement and automated deprecation
**Approved By**: Architecture Team
**Related ADR**: [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)

---

## 📚 **References**

### **Related Business Requirements**
- BR-WORKFLOW-001: Playbook registry management
- BR-EFFECTIVENESS-002: Provides effectiveness data for deprecation decisions
- BR-AI-057: AI queries only active playbooks

### **Related Documents**
- [ADR-033: Remediation Workflow Catalog](../architecture/decisions/ADR-033-remediation-playbook-catalog.md)
- [ADR-033: Cross-Service BRs](../architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md)
- [ADR-037: BR Template Standard](../architecture/decisions/ADR-037-business-requirement-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved for V1 Implementation

