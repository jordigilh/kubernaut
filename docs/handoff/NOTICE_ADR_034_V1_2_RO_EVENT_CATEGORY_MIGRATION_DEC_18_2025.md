# 🔔 NOTICE: Remediation Orchestrator event_category Migration Required

**Date**: December 18, 2025
**Type**: **INFORMATIONAL** (Best Practice Alignment)
**Priority**: **MEDIUM** - V1.0 alignment recommended (not blocking)
**Authority**: [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md)
**Affected Team**: Remediation Orchestrator
**Action Required By**: V1.0 Release (recommended, not mandatory)

---

## 📋 Executive Summary

**Context**: During Data Storage REST API investigation (NT Team bug report, Dec 18, 2025), we discovered that Remediation Orchestrator is the **ONLY service** using **operation-level** `event_category` values instead of **service-level** values.

**Current State**: Remediation Orchestrator uses 5 different `event_category` values:
- `"lifecycle"` - Orchestrator lifecycle events
- `"phase"` - Phase transitions
- `"approval"` - Approval requests
- `"remediation"` - Remediation operations
- `"routing"` - Routing decisions

**Standard Pattern**: All other services (Gateway, Notification, AI Analysis, SignalProcessing, Workflow, Execution) use **service-level** `event_category`:
- `"gateway"` - All Gateway events
- `"notification"` - All Notification events
- `"analysis"` - All AI Analysis events
- `"signalprocessing"` - All SignalProcessing events
- `"workflow"` - All Workflow events
- `"execution"` - All Execution events

**Recommendation**: Consolidate to `"orchestration"` for all Remediation Orchestrator events, following the service-level convention.

**Design Decision**: [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md) now standardizes `event_category` as service-level (not operation-level).

---

## 🎯 What This Means For Your Team

### This is NOT Blocking

**This is NOT**:
- ❌ A V1.0 release blocker
- ❌ A breaking change (your current code will continue to work)
- ❌ A mandatory immediate migration
- ❌ A critical bug fix

**This IS**:
- ✅ A best practice alignment recommendation
- ✅ A consistency improvement across services
- ✅ A query efficiency enhancement
- ✅ A V1.0 alignment opportunity

### Why This Matters

**Problem with Current Approach** (Operation-Level Categories):
```go
// Current RO pattern: 5 different categories for one service
audit.SetEventCategory(event, "lifecycle")
audit.SetEventCategory(event, "phase")
audit.SetEventCategory(event, "approval")
audit.SetEventCategory(event, "remediation")
audit.SetEventCategory(event, "routing")

// Result: To query ALL RO events, you need 5 filters
WHERE event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
```

**Solution with Service-Level Category**:
```go
// Recommended RO pattern: 1 category for entire service
audit.SetEventCategory(event, "orchestration")  // Service identifier
audit.SetEventAction(event, "started")         // Operation type
audit.SetEventAction(event, "transitioned")    // Operation type
audit.SetEventAction(event, "approval_requested") // Operation type

// Result: To query ALL RO events, you need 1 filter
WHERE event_category = 'orchestration'
```

**Benefits**:
1. **Consistency**: Matches all other 6 services (Gateway, Notification, AI, SP, Workflow, Execution)
2. **Query Efficiency**: One filter instead of five for "all orchestrator events"
3. **Service Analytics**: Track orchestrator event volume, success rate, performance in one query
4. **Compliance Auditing**: "Show all orchestrator operations for last 90 days" becomes trivial
5. **Operational Visibility**: Clear which service generated the event (orchestration, not generic "lifecycle")

---

## 📊 Current vs. Recommended Mapping

### Current Implementation (Operation-Level)

| event_category | event_action | event_type | Usage |
|---------------|--------------|------------|-------|
| `"lifecycle"` | `"started"` | `orchestrator.lifecycle.started` | Orchestrator started |
| `"lifecycle"` | `"completed"` | `orchestrator.lifecycle.completed` | Orchestrator completed |
| `"phase"` | `"transitioned"` | `orchestrator.phase.transitioned` | Phase changed |
| `"approval"` | `"approval_requested"` | `orchestrator.approval.requested` | Approval needed |
| `"approval"` | `"approved"` | `orchestrator.approval.approved` | Approval granted |
| `"approval"` | `"rejected"` | `orchestrator.approval.rejected` | Approval denied |
| `"remediation"` | `"manual_review"` | `orchestrator.remediation.manual_review` | Manual review |
| `"routing"` | `"blocked"` | `orchestrator.routing.blocked` | Routing blocked |

### Recommended Implementation (Service-Level)

| event_category | event_action | event_type | Change |
|---------------|--------------|------------|--------|
| `"orchestration"` | `"started"` | `orchestrator.lifecycle.started` | ✅ Category change only |
| `"orchestration"` | `"completed"` | `orchestrator.lifecycle.completed` | ✅ Category change only |
| `"orchestration"` | `"transitioned"` | `orchestrator.phase.transitioned` | ✅ Category change only |
| `"orchestration"` | `"approval_requested"` | `orchestrator.approval.requested` | ✅ Category change only |
| `"orchestration"` | `"approved"` | `orchestrator.approval.approved` | ✅ Category change only |
| `"orchestration"` | `"rejected"` | `orchestrator.approval.rejected` | ✅ Category change only |
| `"orchestration"` | `"manual_review"` | `orchestrator.remediation.manual_review` | ✅ Category change only |
| `"orchestration"` | `"blocked"` | `orchestrator.routing.blocked` | ✅ Category change only |

**Key Point**: Only `event_category` changes. `event_type` and `event_action` remain the same.

---

## 🔧 Implementation Guide (Optional)

### Step 1: Update Constants

**Current** (`pkg/remediationorchestrator/audit/helpers.go:38-61`):
```go
const (
    CategoryLifecycle   = "lifecycle"
    CategoryPhase       = "phase"
    CategoryApproval    = "approval"
    CategoryRemediation = "remediation"
    CategoryRouting     = "routing"
)
```

**Recommended**:
```go
const (
    // Service-level category (ADR-034 v1.2: Use service name, not operation type)
    CategoryOrchestration = "orchestration"

    // Action types (use event_action, not event_category, to differentiate operations)
    ActionStarted          = "started"
    ActionCompleted        = "completed"
    ActionTransitioned     = "transitioned"
    ActionApprovalRequested = "approval_requested"
    ActionApproved         = "approved"
    ActionRejected         = "rejected"
    ActionManualReview     = "manual_review"
    ActionBlocked          = "blocked"
)
```

### Step 2: Update Audit Functions

**Current**:
```go
audit.SetEventCategory(event, CategoryLifecycle)
audit.SetEventAction(event, ActionStarted)
```

**Recommended**:
```go
audit.SetEventCategory(event, CategoryOrchestration)  // Service-level
audit.SetEventAction(event, ActionStarted)           // Operation type
```

### Step 3: Update Tests

**Current** (if filtering by category):
```go
params := &dsgen.QueryAuditEventsParams{
    EventCategory: ptr.To("lifecycle"),  // Old: operation-level
}
```

**Recommended**:
```go
params := &dsgen.QueryAuditEventsParams{
    EventCategory: ptr.To("orchestration"),  // New: service-level
    // If needed, add event_action filter
}
```

### Step 4: Verify Metrics

**If you have Prometheus metrics** using `event_category` as a label:
```go
// Current: 5 separate metrics
orchestrator_events_total{category="lifecycle"} 100
orchestrator_events_total{category="phase"} 200
orchestrator_events_total{category="approval"} 50
orchestrator_events_total{category="remediation"} 30
orchestrator_events_total{category="routing"} 20

// Recommended: 1 unified metric (use event_action for breakdown)
orchestrator_events_total{category="orchestration",action="started"} 100
orchestrator_events_total{category="orchestration",action="transitioned"} 200
orchestrator_events_total{category="orchestration",action="approval_requested"} 50
```

**Estimated Effort**: 1-2 hours (update constants, ~8 audit function calls, update tests)

---

## 📋 Query Pattern Examples

### Before (Operation-Level)

**Query: "Get all orchestrator events for last 7 days"**
```sql
-- Current: Requires 5 OR conditions
SELECT * FROM audit_events
WHERE event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
  AND event_timestamp > NOW() - INTERVAL '7 days'
ORDER BY event_timestamp DESC;
```

**Query: "Count orchestrator events per operation"**
```sql
-- Current: Use event_category directly
SELECT event_category, COUNT(*) as count
FROM audit_events
WHERE event_category IN ('lifecycle', 'phase', 'approval', 'remediation', 'routing')
GROUP BY event_category;
```

### After (Service-Level)

**Query: "Get all orchestrator events for last 7 days"**
```sql
-- Recommended: Single condition
SELECT * FROM audit_events
WHERE event_category = 'orchestration'
  AND event_timestamp > NOW() - INTERVAL '7 days'
ORDER BY event_timestamp DESC;
```

**Query: "Count orchestrator events per operation"**
```sql
-- Recommended: Use event_action for breakdown
SELECT event_action, COUNT(*) as count
FROM audit_events
WHERE event_category = 'orchestration'
GROUP BY event_action;
```

**Query: "Service-level analytics across all services"**
```sql
-- After migration: RO included in service-level analytics
SELECT
    event_category as service,
    COUNT(*) as total_events,
    COUNT(*) FILTER (WHERE event_outcome = 'success') as success_count,
    COUNT(*) FILTER (WHERE event_outcome = 'failure') as failure_count
FROM audit_events
WHERE event_timestamp > NOW() - INTERVAL '30 days'
GROUP BY event_category
ORDER BY total_events DESC;

-- Result includes: gateway, notification, analysis, signalprocessing, workflow, execution, orchestration
```

---

## 🎯 Benefits Summary

### Operational Benefits

1. **Query Efficiency**:
   - Before: 5 categories to filter → `WHERE event_category IN ('lifecycle', 'phase', ...)`
   - After: 1 category to filter → `WHERE event_category = 'orchestration'`

2. **Service Analytics**:
   - Before: Can't easily track "all orchestrator events" without knowing all 5 categories
   - After: One query: `WHERE event_category = 'orchestration'`

3. **Consistency**:
   - Before: RO is the ONLY service using operation-level categories (confusing for new developers)
   - After: RO matches all 6 other services (predictable pattern)

4. **Compliance Auditing**:
   - Before: "Show all orchestrator operations" requires domain knowledge of categories
   - After: "Show all orchestrator operations" is `WHERE event_category = 'orchestration'`

### Technical Benefits

1. **Index Efficiency**:
   - Single `event_category` value per service → better cardinality for indexes
   - Easier to add index on `(event_category, event_timestamp)` for service-level queries

2. **Metrics Simplification**:
   - One service label instead of five operation labels
   - Use `event_action` for operation breakdown if needed

3. **Code Clarity**:
   - Clear that `event_category` identifies the **service** (orchestration)
   - Clear that `event_action` identifies the **operation** (started, transitioned, etc.)

---

## 🤔 Questions & Concerns

### Q1: Will this break existing queries?

**A**: **Potentially, yes** - If you have queries, dashboards, or alerts that filter by `event_category IN ('lifecycle', 'phase', ...)`, they will need to be updated. However, this is a **low-risk** change:
- **Audit data is append-only**: Old events with old categories will remain unchanged
- **New events** will use `"orchestration"` after migration
- **Transition period**: Both patterns can coexist temporarily

### Q2: Should we migrate old audit events in the database?

**A**: **No, not recommended** - Audit events are immutable per ADR-034. Old events should remain as-is for audit trail integrity. Only **new events** should use `"orchestration"`.

**Alternative**: If needed for analytics, create a view:
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

### Q3: Is this mandatory for V1.0?

**A**: **No** - This is a **best practice recommendation**, not a blocker. However, aligning before V1.0 avoids technical debt and makes the codebase more consistent.

**Recommendation**: Migrate for V1.0 if time permits (1-2 hours effort). If not, create a post-V1.0 task.

### Q4: Why is this coming up now?

**A**: **Root Cause**: NT Team's bug investigation (Dec 18, 2025) revealed incomplete OpenAPI spec for Data Storage. During triage, we audited all services' `event_category` usage and discovered RO is the only service not following service-level convention. This was documented in [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md).

**Timing**: V1.0 release preparation is the ideal time to align conventions across all services.

### Q5: What if we prefer keeping operation-level categories?

**A**: That's valid feedback! Please share your concerns:

**What we'd like to understand**:
1. What specific use cases benefit from operation-level categories?
2. Are there queries or dashboards that would break with service-level categories?
3. Would adding `event_action` filters satisfy your use cases?

**How to provide feedback**:
- **Slack**: `#architecture-decisions` channel
- **Document**: Add comments to [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md)
- **Meeting**: Request architecture review session

---

## 🔗 References

### Authoritative Documentation

- **[ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md)**: Unified Audit Table Design (Event Category Naming Convention added)
- **[DD-AUDIT-003](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)**: Service Audit Trace Requirements
- **[DD-API-001](../architecture/decisions/DD-API-001-openapi-client-mandatory-v1.md)**: OpenAPI Client Mandatory (context for this discovery)

### Related Issues

- **[NT_DS_API_QUERY_ISSUE_DEC_18_2025.md](./NT_DS_API_QUERY_ISSUE_DEC_18_2025.md)**: Root cause investigation that led to this discovery

### Implementation References

- **Gateway**: `pkg/gateway/server.go:1129` - Uses `"gateway"`
- **Notification**: `test/integration/notification/audit_integration_test.go:131` - Uses `"notification"`
- **AI Analysis**: `pkg/aianalysis/audit/audit.go:98` - Uses `"analysis"`
- **SignalProcessing**: `pkg/signalprocessing/audit/client.go:135` - Uses `"signalprocessing"`
- **Workflow**: `pkg/datastorage/audit/workflow_search_event.go:174` - Uses `"workflow"`
- **Remediation Orchestrator**: `pkg/remediationorchestrator/audit/helpers.go:38-61` - **Uses 5 operation-level categories** (current pattern)

---

## 📊 Service Compliance Status

| Service | event_category Pattern | Status | Action |
|---------|----------------------|--------|--------|
| **Gateway** | `"gateway"` | ✅ **COMPLIANT** | None |
| **Notification** | `"notification"` | ✅ **COMPLIANT** | None |
| **AI Analysis** | `"analysis"` | ✅ **COMPLIANT** | None |
| **SignalProcessing** | `"signalprocessing"` | ✅ **COMPLIANT** | None |
| **Workflow** | `"workflow"` | ✅ **COMPLIANT** | None |
| **Execution** | `"execution"` | ✅ **COMPLIANT** | None |
| **Remediation Orchestrator** | `"lifecycle"`, `"phase"`, `"approval"`, `"remediation"`, `"routing"` | ⚠️ **NON-COMPLIANT** | **Migration recommended** |

---

## 🎯 Bottom Line

### For Team Leads

**What**: Consolidate 5 operation-level `event_category` values to 1 service-level value (`"orchestration"`)
**Why**: Consistency with all other 6 services, query efficiency, service-level analytics
**When**: V1.0 release (recommended, not mandatory)
**Effort**: 1-2 hours (update constants, ~8 audit function calls, update tests)
**Risk**: Low (append-only audit, no data migration needed)
**Authority**: [ADR-034 v1.2](../architecture/decisions/ADR-034-unified-audit-table-design.md) (Service-Level Event Category Convention)

### For Developers

**Reference**: [ADR-034 v1.2 Section 1.1](../architecture/decisions/ADR-034-unified-audit-table-design.md#11-event-category-naming-convention-v12)
**Pattern**: Replace `CategoryLifecycle`, `CategoryPhase`, etc. with `CategoryOrchestration`
**Benefit**: Matches all other 6 services, simpler queries, better analytics
**Support**: `#architecture-decisions` Slack channel (for questions or feedback)

### For Release Coordinator

**Status**: **INFORMATIONAL** (not blocking V1.0)
**Recommendation**: If RO team has bandwidth, migrate for V1.0 (1-2 hours)
**If not migrated**: Create post-V1.0 task for technical debt reduction
**Tracking**: Monitor this document for RO team acknowledgment (optional)

---

## ✅ Team Acknowledgment (Optional)

**Instructions**: If your team decides to migrate, update this section with progress.

### Remediation Orchestrator Team
- [x] **Acknowledged**: RO Team, December 18, 2025, 14:30 UTC
- [x] **Decision**: **Migrate for V1.0** (User Override: "let's do it now before we continue with the fixes")
- [x] **Migration Started**: December 18, 2025, 14:35 UTC
- [x] **Code Migration Complete**: December 18, 2025, 14:40 UTC (commit `3048bc5b`)
- [ ] **Tests Passing**: ⏸️ BLOCKED - Waiting on DS OpenAPI schema update

**User Direction**: User prioritized this migration before continuing with remaining test fixes (RAR conditions, lifecycle, audit trace).

**🚨 BLOCKER DISCOVERED** (December 18, 2025, 16:10 UTC):
- **Issue**: Data Storage OpenAPI schema missing `"orchestration"` enum value
- **Impact**: RO audit events rejected with 400 Bad Request
- **Test Results**: 12 passed / 14 failed (all failures due to audit write errors)
- **Status**: DS Team notified (Dec 18, 16:30 UTC)
- **Expected Resolution**: 30 minutes (DS team effort)
- **Details**: See `docs/handoff/NT_SECOND_OPENAPI_BUG_DEC_18_2025.md` (Third OpenAPI Gap section)

**Migration Status**:
- ✅ Production code: 8 locations updated to `CategoryOrchestration`
- ✅ Test code: 3 locations updated to expect `"orchestration"`
- ✅ Lint validation: 0 errors
- ❌ Integration tests: BLOCKED by DS schema (400 Bad Request errors)
- ⏸️ **Waiting on**: DS team to add `"orchestration"` to `event_category` enum

**Feedback/Questions**:

### 📊 **RO Team Assessment** (December 18, 2025)

**Priority Decision**: **P3 - Defer to post-V1.0**

**Rationale**:
1. **Current Priority**: Fixing 12 failing integration tests (63% → >70% pass rate target)
2. **Migration Effort**: 1-2 hours (8 code locations + 3 test locations)
3. **Risk**: Low (append-only audit, no data migration)
4. **V1.0 Impact**: None (this is best practice alignment, not functional change)

**Migration Scope Assessment**:
- **Production Code**: 8 locations in `pkg/remediationorchestrator/audit/helpers.go`
  - 5 constant definitions (lines 38-42, 61)
  - 8 `audit.SetEventCategory()` calls (lines 95, 136, 181, 217, 269, 335, 381, 437)
- **Test Code**: 3 locations in `test/integration/remediationorchestrator/audit_trace_integration_test.go`
  - 2 test assertions (lines 214, 274)
  - 1 comment reference (line 318)
- **Estimated Effort**: 1.5 hours (30 min code + 30 min test validation + 30 min documentation)

**Current Context**:
- ✅ RO production code is 90% compliant with DD-API-001 (OpenAPI client mandate)
- 🎯 Active work: Fixing 12 failing tests (RAR conditions, lifecycle, audit trace)
- 📊 Current pass rate: 63% (20/32 tests)
- 🎯 Target pass rate: >70% (23/32 tests)

**Recommendation**:
- **V1.0 Release**: Proceed without migration (not blocking)
- **Post-V1.0**: Create technical debt task for event_category consolidation
- **Priority**: P3 (after test fixes, after DD-API-001 migration if needed)

**Why This Makes Sense**:
1. ✅ **Functional Correctness**: Current implementation works correctly
2. ✅ **Query Performance**: RO doesn't query its own audit events (no performance impact)
3. ✅ **V1.0 Release**: Not blocking, purely a consistency improvement
4. ⚠️ **Technical Debt**: Acknowledged and tracked for post-V1.0 cleanup
5. 🎯 **Focus**: Team bandwidth better spent on test fixes (user-visible quality)

**Questions for Architecture Team**:
1. **Transition Plan**: Should we use a feature flag or immediate cutover when we do migrate?
2. **Analytics Impact**: Are there existing dashboards filtering by operation-level categories?
3. **Documentation**: Should we document the current pattern in ADR-034 as "legacy pattern"?

**Tracking**:
- **Technical Debt Task**: Create post-V1.0 story for event_category consolidation
- **Estimated Post-V1.0 Effort**: 1.5-2 hours (includes full test validation)
- **Confidence**: 95% this is the right prioritization decision

---

**Type**: **INFORMATIONAL** (Best Practice Alignment)
**Priority**: **MEDIUM** - V1.0 alignment recommended (not blocking)
**Questions**: `#architecture-decisions` Slack channel

✅ **This is NOT a V1.0 blocker - Your team can proceed with V1.0 release regardless of migration status**

