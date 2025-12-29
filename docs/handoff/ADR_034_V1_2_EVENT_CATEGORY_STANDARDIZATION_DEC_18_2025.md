# ADR-034 v1.2: event_category Naming Convention Standardization

**Date**: December 18, 2025
**Type**: **ARCHITECTURAL IMPROVEMENT**
**Priority**: **MEDIUM** - Best practice alignment (RO migration recommended for V1.0)
**Authority**: [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md)
**Related**: [NT DS API Query Investigation](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)

---

## 📋 Executive Summary

**Context**: During Data Storage REST API investigation (NT Team bug report, Dec 18, 2025), we audited all services' `event_category` usage and discovered **Remediation Orchestrator** is the **ONLY service** using **operation-level** categories instead of **service-level** categories.

**Discovery**:
- ✅ **6 services** follow service-level pattern: Gateway, Notification, AI Analysis, SignalProcessing, Workflow, Execution
- ❌ **1 service** uses operation-level pattern: RemediationOrchestrator (5 different categories)

**Action Taken**:
1. ✅ Updated ADR-034 from v1.1 → v1.2 (added event_category naming convention)
2. ✅ Documented complete list of valid `event_category` values
3. ✅ Created notice for RemediationOrchestrator team (migration recommended, not mandatory)

**Outcome**:
- **ADR-034 v1.2**: Now explicitly requires service-level `event_category` (e.g., `"orchestration"`, not `"lifecycle"`)
- **RO Team**: Notified of best practice misalignment, migration recommended for V1.0

---

## 🔍 What Was Discovered

### Service-Level vs. Operation-Level Categories

**Discovered Pattern** (6 out of 7 services):
```go
// Gateway: Service-level category
audit.SetEventCategory(event, "gateway")  // All gateway events
audit.SetEventAction(event, "received")   // Operation type

// Notification: Service-level category
audit.SetEventCategory(event, "notification")  // All notification events
audit.SetEventAction(event, "sent")           // Operation type

// AI Analysis: Service-level category
audit.SetEventCategory(event, "analysis")  // All analysis events
audit.SetEventAction(event, "completed")   // Operation type
```

**Outlier Pattern** (1 out of 7 services):
```go
// RemediationOrchestrator: Operation-level categories (ANTI-PATTERN)
audit.SetEventCategory(event, "lifecycle")    // Operation type (wrong)
audit.SetEventCategory(event, "phase")        // Operation type (wrong)
audit.SetEventCategory(event, "approval")     // Operation type (wrong)
audit.SetEventCategory(event, "remediation")  // Operation type (wrong)
audit.SetEventCategory(event, "routing")      // Operation type (wrong)
```

---

## 📊 Service Compliance Analysis

| Service | event_category Value(s) | Pattern | Status |
|---------|------------------------|---------|--------|
| **Gateway** | `"gateway"` | Service-level | ✅ **COMPLIANT** |
| **Notification** | `"notification"` | Service-level | ✅ **COMPLIANT** |
| **AI Analysis** | `"analysis"` | Service-level | ✅ **COMPLIANT** |
| **SignalProcessing** | `"signalprocessing"` | Service-level | ✅ **COMPLIANT** |
| **Workflow** | `"workflow"` | Service-level | ✅ **COMPLIANT** |
| **Execution** | `"execution"` | Service-level | ✅ **COMPLIANT** |
| **RemediationOrchestrator** | `"lifecycle"`, `"phase"`, `"approval"`, `"remediation"`, `"routing"` | Operation-level | ⚠️ **NON-COMPLIANT** |

**Compliance Rate**: 6/7 services (85.7%) already follow service-level convention
**Non-Compliance**: Only RemediationOrchestrator uses operation-level categories

---

## 📝 ADR-034 v1.2 Changes

### Version Update

**Before** (v1.1):
- **Version**: 1.1
- **Last Updated**: 2025-11-27
- **event_category Documentation**: Only schema comment: `-- 'signal', 'remediation', 'workflow'` (examples only, incomplete)

**After** (v1.2):
- **Version**: 1.2
- **Last Updated**: 2025-12-18
- **event_category Documentation**: New section 1.1 with complete naming convention

### Key Changes

**1. Updated Schema Comment** (Line 58):
```sql
-- Before (v1.1):
event_category VARCHAR(50) NOT NULL,     -- 'signal', 'remediation', 'workflow'

-- After (v1.2):
event_category VARCHAR(50) NOT NULL,     -- Service identifier: 'gateway', 'notification', 'analysis', 'signalprocessing', 'workflow', 'execution', 'orchestration' (see Event Category Naming Convention below)
```

**2. Added Section 1.1: Event Category Naming Convention**

New section includes:
- **RULE**: `event_category` MUST match the service name (not operation type)
- **Complete List**: All 7 valid categories documented
- **Rationale**: 5 benefits (filtering, analytics, compliance, cost, correlation)
- **Query Patterns**: SQL examples for common use cases
- **Anti-Pattern**: Explicitly calls out operation-level categories as forbidden
- **Migration Note**: RemediationOrchestrator MUST consolidate for V1.0

**3. Updated Changelog** (Version History):
```markdown
| **v1.2** | 2025-12-18 | **BREAKING**: Standardized `event_category` naming convention (service-level, not operation-level). Added complete list of valid categories. RemediationOrchestrator MUST consolidate to `"orchestration"` category. Discovered during NT Team DS API query investigation (DD-API-001). Cross-references DD-AUDIT-003. | Architecture Team |
```

---

## 📨 Remediation Orchestrator Notice

### Notice Document Created

**File**: `docs/handoff/NOTICE_ADR_034_V1_2_RO_EVENT_CATEGORY_MIGRATION_DEC_18_2025.md`

**Type**: **INFORMATIONAL** (Best Practice Alignment)
**Priority**: **MEDIUM** - V1.0 alignment recommended (not blocking)

### Key Points in Notice

1. **Context**: RO is the only service using operation-level categories
2. **Recommendation**: Consolidate 5 categories to 1 (`"orchestration"`)
3. **Benefits**: Query efficiency, consistency, service analytics
4. **Implementation**: 1-2 hours effort (update constants, ~8 function calls)
5. **Migration**: NOT blocking V1.0 (recommended, but optional)

### Current vs. Recommended Mapping

**Current Implementation**:
```go
// 5 operation-level categories
const (
    CategoryLifecycle   = "lifecycle"
    CategoryPhase       = "phase"
    CategoryApproval    = "approval"
    CategoryRemediation = "remediation"
    CategoryRouting     = "routing"
)
```

**Recommended Implementation**:
```go
// 1 service-level category
const (
    CategoryOrchestration = "orchestration"  // Service identifier

    // Use event_action for operation differentiation
    ActionStarted          = "started"
    ActionCompleted        = "completed"
    ActionTransitioned     = "transitioned"
    ActionApprovalRequested = "approval_requested"
    // ...etc
)
```

---

## 🎯 Benefits of Standardization

### 1. Query Efficiency

**Before** (Operation-Level):
```sql
-- Get all RemediationOrchestrator events: 5 OR conditions
SELECT * FROM audit_events
WHERE event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
  AND event_timestamp > NOW() - INTERVAL '7 days';
```

**After** (Service-Level):
```sql
-- Get all RemediationOrchestrator events: 1 condition
SELECT * FROM audit_events
WHERE event_category = 'orchestration'
  AND event_timestamp > NOW() - INTERVAL '7 days';
```

### 2. Service Analytics

**Before**:
- Can't easily track "all orchestrator events" without knowing all 5 categories
- Service-level metrics require manual aggregation

**After**:
- One query: `WHERE event_category = 'orchestration'`
- Service-level SLOs: success rate, event volume, performance

### 3. Consistency

**Before**:
- 6 services use service-level pattern
- 1 service (RO) uses operation-level pattern
- New developers confused by inconsistency

**After**:
- 7 services use service-level pattern
- Predictable, consistent pattern across all services

### 4. Compliance Auditing

**Before**:
```sql
-- "Show all orchestrator operations for compliance review"
-- Requires domain knowledge of all 5 categories
WHERE event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
```

**After**:
```sql
-- "Show all orchestrator operations for compliance review"
-- Intuitive, no domain knowledge needed
WHERE event_category = 'orchestration'
```

---

## 📋 Complete Event Category List (v1.2)

| event_category | Service | Example Events |
|---------------|---------|----------------|
| `gateway` | Gateway Service | `gateway.signal.received`, `gateway.crd.created` |
| `notification` | Notification Service | `notification.message.sent`, `notification.delivery.failed` |
| `analysis` | AI Analysis Service | `aianalysis.investigation.started`, `aianalysis.recommendation.generated` |
| `signalprocessing` | Signal Processing Service | `signalprocessing.enrichment.completed`, `signalprocessing.classification.decision` |
| `workflow` | Workflow Catalog Service | `workflow.catalog.search_completed` |
| `execution` | Remediation Execution Service | `execution.workflow.started`, `execution.action.executed` |
| `orchestration` | Remediation Orchestrator Service | `orchestrator.lifecycle.started`, `orchestrator.phase.transitioned` |

---

## 🔗 Cross-References

### Related Documents

- **[ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md)**: Updated with event_category naming convention
- **[DD-AUDIT-003](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)**: Service Audit Trace Requirements
- **[NOTICE: RO event_category Migration](./NOTICE_ADR_034_V1_2_RO_EVENT_CATEGORY_MIGRATION_DEC_18_2025.md)**: RO Team migration notice
- **[NT DS API Query Issue](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)**: Root cause investigation that led to this discovery

### Related Design Decisions

- **[DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)**: OpenAPI Client Mandatory (context for this discovery)

---

## 📊 Impact Assessment

### Services Affected

| Service | Impact | Action Required |
|---------|--------|-----------------|
| **Gateway** | ✅ None | Already compliant |
| **Notification** | ✅ None | Already compliant |
| **AI Analysis** | ✅ None | Already compliant |
| **SignalProcessing** | ✅ None | Already compliant |
| **Workflow** | ✅ None | Already compliant |
| **Execution** | ✅ None | Already compliant |
| **RemediationOrchestrator** | ⚠️ **RECOMMENDED MIGRATION** | Consolidate to `"orchestration"` (1-2 hours) |

### Database Impact

**Schema Changes**: ❌ **NONE**
**Data Migration**: ❌ **NOT RECOMMENDED** (audit events are immutable)
**Query Impact**: ⚠️ **QUERIES THAT FILTER BY RO CATEGORIES MAY BREAK** (if RO migrates)

**Mitigation**: If RO migrates, create a database view for backward compatibility:
```sql
CREATE VIEW audit_events_normalized AS
SELECT
    *,
    CASE
        WHEN event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
        THEN 'orchestration'
        ELSE event_category
    END AS normalized_category
FROM audit_events;
```

---

## 🎯 Next Steps

### For Architecture Team
- ✅ **COMPLETE**: ADR-034 updated to v1.2
- ✅ **COMPLETE**: RO Team notified via notice document
- ⏳ **PENDING**: Monitor RO Team acknowledgment and decision

### For Remediation Orchestrator Team
- ⏳ **PENDING**: Review notice document
- ⏳ **PENDING**: Decide: Migrate for V1.0 or defer to post-V1.0
- ⏳ **PENDING**: If migrating: Update constants, audit functions, tests (1-2 hours)
- ⏳ **PENDING**: Acknowledge in notice document (optional)

### For Release Coordinator
- ✅ **NOTED**: RO migration is **NOT** V1.0 blocking
- ⏳ **TRACK**: If RO migrates, verify tests pass
- ⏳ **TRACK**: If RO defers, create post-V1.0 task for technical debt

---

## 📈 Success Metrics

### Immediate Success (v1.2)
- ✅ ADR-034 updated with event_category naming convention
- ✅ Complete list of valid categories documented
- ✅ RO Team notified with clear guidance and examples
- ✅ Anti-pattern (operation-level categories) explicitly forbidden

### Long-Term Success (Post-Migration)
- ⏳ All 7 services use service-level `event_category` (currently 6/7 = 85.7%)
- ⏳ Query efficiency improved (1 filter vs. 5 filters for "all RO events")
- ⏳ Service-level analytics simplified across all services
- ⏳ Compliance auditing standardized (predictable query patterns)

---

## 🔐 Confidence Assessment

**Confidence**: **95%** ✅ **STRONGLY RECOMMEND**

**Justification**:
- ✅ **Evidence-Based**: 6 out of 7 services already follow service-level pattern
- ✅ **Industry Alignment**: Service-level categorization is standard practice (AWS, Google, Azure)
- ✅ **Query Efficiency**: Proven benefit (1 filter vs. 5 filters)
- ✅ **Consistency**: Reduces cognitive load for developers
- ✅ **Low Risk**: RO migration is optional for V1.0, no database changes

**Remaining 5% Uncertainty**: RO Team may have valid use cases for operation-level categories that we haven't considered. Open to feedback.

---

## 💬 Questions & Feedback

**For Architecture Team**:
- **Slack**: `#architecture-decisions`
- **Document**: Add comments to ADR-034 v1.2

**For RemediationOrchestrator Team**:
- **Slack**: `#v1-migration-support` (if migrating)
- **Document**: Add questions to [RO Notice](./NOTICE_ADR_034_V1_2_RO_EVENT_CATEGORY_MIGRATION_DEC_18_2025.md)

---

**Status**: ✅ **COMPLETE** - ADR-034 v1.2 published, RO Team notified
**Next Review**: After RO Team acknowledgment or V1.0 release (whichever comes first)
**Last Updated**: December 18, 2025, 15:30 UTC


