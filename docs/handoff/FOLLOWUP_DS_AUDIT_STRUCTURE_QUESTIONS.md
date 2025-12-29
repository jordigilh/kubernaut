# Follow-Up Questions: Audit Event Data Structure

**From**: Notification Team (NT)
**To**: Data Services Team (DS)
**Date**: December 17, 2025
**Context**: Response to `DS_AUDIT_EVENT_DATA_STRUCTURE_RESPONSE.md`
**Priority**: 🟡 MEDIUM (refinements, not blockers)

---

## 📋 Response Acknowledgment

✅ **Thank you for the authoritative response!** Your guidance on Pattern 2 with `audit.StructToMap()` is clear and unblocks NT implementation.

**Status**: We can proceed with implementation using reasonable defaults, but have follow-up questions for consistency and long-term maintainability.

---

## ❓ Follow-Up Questions

### 🔴 Q1: Where Should Structured Types Be Defined? (HIGH PRIORITY)

**Context**: DS response shows example type `MessageSentEventData` but doesn't specify file location.

**Options for NT Implementation**:

| Option | Path | Visibility | Pros | Cons |
|--------|------|-----------|------|------|
| **A** | `pkg/notification/audit/event_types.go` | Public (pkg/) | Service encapsulation, reusable in tests | Creates new package |
| **B** | `pkg/audit/notification_events.go` | Public (shared) | Centralized, cross-service visibility | Violates service encapsulation |
| **C** | `internal/controller/notification/audit_types.go` | Private (internal/) | Controller-local, minimal exposure | Not testable from integration/e2e tests |

**Current Service Patterns**:
- WorkflowExecution: `pkg/workflowexecution/audit_types.go` (Option A equivalent)
- AIAnalysis: `pkg/aianalysis/audit/audit.go` (Option A equivalent)

**NT Team Preference**: **Option A** (`pkg/notification/audit/event_types.go`)

**Question**: Does this align with DS team's vision for service audit type organization?

---

### 🟡 Q2: Error Handling Pattern for `audit.StructToMap()` (MEDIUM PRIORITY)

**Context**: `audit.StructToMap()` returns `(map[string]interface{}, error)` but DS example doesn't show full error handling.

**Current NT Audit Function Signature**:
```go
func (r *NotificationRequestReconciler) auditMessageSent(
    ctx context.Context,
    notification *notificationv1.NotificationRequest,
) error
```

**Scenario**: `audit.StructToMap()` fails during reconciliation

**Options**:

| Option | Behavior | ADR-032 §1 Compliance | Production Impact |
|--------|----------|----------------------|-------------------|
| **A** | Return error immediately (fail reconciliation) | ✅ YES ("No Audit Loss") | Reconciliation fails until audit fixed |
| **B** | Log error and continue (degrade gracefully) | ❌ NO (allows audit loss) | Reconciliation continues, audit lost |
| **C** | Panic (marshal should never fail) | ⚠️ PARTIAL (prevents loss via crash) | Pod restart, reconciliation retried |

**NT Team Preference**: **Option A** (return error)

**Rationale**: Aligns with ADR-032 §1 "No Audit Loss" mandate

**Question**: Does this error handling strategy align with DS team's expectations for production audit robustness?

---

### 🟡 Q3: Migration Scope - NT Only or Coordinated? (MEDIUM PRIORITY)

**Context**: DS response identifies 4 services needing migration:
- Notification (Pattern 1 → Pattern 2 with `audit.StructToMap()`)
- SignalProcessing (Pattern 1 → Pattern 2 with `audit.StructToMap()`)
- WorkflowExecution (Pattern 2 custom `ToMap()` → Pattern 2 with `audit.StructToMap()`)
- AIAnalysis (Pattern 2 custom `ToMap()` → Pattern 2 with `audit.StructToMap()`)

**Migration Options**:

| Option | Approach | Benefits | Risks |
|--------|----------|----------|-------|
| **A** | Independent per service (NT now, others later) | Non-blocking, service teams control timing | Inconsistent patterns during transition |
| **B** | Coordinated cross-service (all at once) | Consistent cutover, single DD update | Requires coordination, potential delays |
| **C** | Phase 1 (NT), Phase 2 (others in V1.1) | NT proves pattern, others follow | Inconsistent until V1.1 |

**NT Team Preference**: **Option A** (independent)

**Rationale**: NT is unblocked and ready to implement; other services can migrate on their own schedules

**Question**: Does DS team recommend coordinated migration, or is independent service migration acceptable?

---

### 🟢 Q4: Should Structured Types Be Exported? (LOW PRIORITY)

**Context**: Types will be in `pkg/notification/audit/`, but should they be public?

**Options**:
- **Option A**: Exported (`MessageSentEventData`) - testable from integration/e2e tests
- **Option B**: Unexported (`messageSentEventData`) - internal implementation detail

**Current Service Patterns**:
- WorkflowExecution: `WorkflowExecutionAuditPayload` (exported)
- AIAnalysis: Need to verify

**NT Team Preference**: **Exported** - enables test validation from external test packages

**Question**: Any guidance on type visibility for audit event structures?

---

### 🟢 Q5: JSON Tag Naming Convention (LOW PRIORITY)

**Context**: DS example shows snake_case JSON tags (`notification_id`, `message_type`).

**DS Example**:
```go
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`  // snake_case
    Channel        string `json:"channel"`          // snake_case
    MessageType    string `json:"message_type"`     // snake_case
    RecipientCount int    `json:"recipient_count"`  // snake_case
}
```

**Question**: Is snake_case JSON tag convention mandatory for audit event data fields, or service-defined?

**Observation**: Snake_case aligns with PostgreSQL JSONB column naming conventions

**NT Team Preference**: Follow DS example (snake_case) for consistency

---

### 🟢 Q6: DD-AUDIT-004 Update Responsibility (LOW PRIORITY)

**Context**: DS response (line 182) mentions:
> ⏸️ Update DD-AUDIT-004 with recommended pattern examples

**Question**: Should:
- **Option A**: NT team update DD-AUDIT-004 with NT implementation as reference example (after proving pattern)
- **Option B**: DS team update DD-AUDIT-004 centrally with canonical examples
- **Option C**: Create separate implementation guide referencing DD-AUDIT-004

**NT Team Preference**: **Option A** - NT updates DD-AUDIT-004 after successful implementation

**Question**: Does DS team prefer a specific approach for DD-AUDIT-004 updates?

---

### 🟢 Q7: Validation Strategy for Structured Types (LOW PRIORITY)

**Context**: Structured types enable compile-time validation, but runtime validation might still be needed.

**Question**: Should structured types include validation tags (e.g., `validate:"required"`) or rely solely on OpenAPI validation at API boundary?

**Current Pattern** (`pkg/audit/openapi_validator.go`):
- Validation happens at API boundary using OpenAPI spec

**NT Team Preference**: No validation tags - rely on OpenAPI validator

**Question**: Any guidance on validation strategy for structured audit event types?

---

### 🟢 Q8: Backward Compatibility for Field Names (LOW PRIORITY)

**Context**: Migration from Pattern 1 (direct map) → Pattern 2 (structured types) changes internal implementation but should not break API contract.

**Concern**: Existing audit event consumers (dashboards, queries) might rely on specific field names in `event_data` JSONB column.

**Question**: Are there any audit event consumers that NT should be aware of when migrating field structures?

**NT Team Approach**: Maintain exact same field names during migration (no breaking changes to JSONB schema)

**Question**: Does DS team maintain a registry of audit event field schemas per service?

---

## 🎯 NT Team Next Steps

**Immediate Action** (proceeding with reasonable defaults):

1. ✅ **Create structured types** in `pkg/notification/audit/event_types.go` (Q1 → Option A)
2. ✅ **Use `audit.StructToMap()`** for all conversions
3. ✅ **Return errors** if conversion fails (Q2 → Option A, ADR-032 compliance)
4. ✅ **Export types** for test validation (Q4 → Option A)
5. ✅ **Use snake_case JSON tags** per DS example (Q5 → Follow DS pattern)
6. ✅ **Independent migration** (Q3 → Option A, NT proceeds now)
7. ✅ **No validation tags** (Q7 → Rely on OpenAPI validator)
8. ✅ **Maintain field names** (Q8 → Backward compatible)

**Pending DS Team Clarification**:
- Q6: DD-AUDIT-004 update responsibility (can defer until after implementation)

---

## ✅ Summary

**Blocking Issues**: **NONE** - NT team can proceed with implementation

**Priority Clarifications** (for consistency):
- 🔴 **Q1**: Type definition location (using `pkg/notification/audit/` as default)
- 🟡 **Q2**: Error handling pattern (using "return error" as default)
- 🟡 **Q3**: Migration scope (proceeding independently)

**Low Priority Refinements** (Q4-Q8):
- Can be addressed during implementation review or post-implementation

---

## 📊 Confidence Assessment

**Confidence in Proceeding**: **95%**

**Justification**:
- DS team provided clear authoritative pattern
- Infrastructure exists (`audit.StructToMap()`, `audit.SetEventData()`)
- Follow-up questions are refinements, not blockers
- Reasonable defaults available for all questions

**Risk Assessment**:
- ⚠️ **Low Risk**: Type definition location choice might need refactoring if DS team prefers different approach
- ✅ **No Risk**: Core pattern is authoritative and well-documented
- ✅ **No Risk**: Backward compatible at API level (JSONB field names preserved)

---

## 🔗 Related Documents

- **DS Team Response**: `docs/handoff/DS_AUDIT_EVENT_DATA_STRUCTURE_RESPONSE.md`
- **NT Triage**: `docs/handoff/NT_DS_RESPONSE_TRIAGE_DEC_17_2025.md`
- **Original Question**: `docs/handoff/QUESTION_DS_AUDIT_EVENT_DATA_STRUCTURE.md`
- **Helper Implementation**: `pkg/audit/helpers.go:127-153`
- **DD-AUDIT-004**: `docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md`

---

**Requested Response**: Clarification on Q1-Q3 (high/medium priority) would be helpful for long-term consistency, but NT team can proceed with reasonable defaults if DS team is unavailable.

**Timeline**: NT team will proceed with implementation using defaults within **1 hour** unless DS team provides alternative guidance.



