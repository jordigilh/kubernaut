# ADR-042: Pre-Implementation Design Decision Pattern

**Status**: ✅ **APPROVED**
**Date**: 2025-11-28
**Related**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md (v2.6+)
**Applies To**: ALL Kubernaut services (process standard)
**Confidence**: 85%

---

## Context & Problem

During service implementations, we've encountered recurring situations where:

1. **Ambiguous requirements cause mid-implementation confusion**
   - API behavior with multiple valid interpretations
   - Unclear immutability/mutability constraints
   - Undefined delete behavior (hard vs soft)

2. **Design decisions made during implementation are not captured**
   - Decisions buried in PR comments
   - Lost knowledge when team members rotate
   - Inconsistent approaches across services

3. **Implementation delays from unresolved questions**
   - Developer blocks waiting for clarification
   - Back-and-forth on Slack/meetings
   - Rework when assumptions are wrong

**Examples from Previous Implementations**:
- **Workflow Service**: Unclear if PUT should update or be rejected (immutability)
- **Data Storage**: Unclear if DELETE should hard-delete or soft-delete (audit trail)
- **Signal Processing**: Unclear if Gateway should pass-through or validate signals

---

## Decision

**APPROVED: Mandatory Pre-Implementation Design Decisions Section**

Before starting Day 1 of any implementation:

1. **Identify ambiguous requirements** that have multiple valid interpretations
2. **Document decisions in standardized format** in the implementation plan
3. **Obtain stakeholder approval** before proceeding
4. **Reference decisions during implementation** for consistency

---

## When to Document Pre-Implementation Decisions

### Mandatory (MUST Document)

| Category | Trigger | Example |
|----------|---------|---------|
| **API Behavior** | Multiple valid interpretations | PUT: update vs reject vs version |
| **Immutability** | Not explicitly defined | Can resource be modified after creation? |
| **Delete Behavior** | Hard vs soft delete unclear | Archive with disabled_at or remove? |
| **Timing** | Sync vs async not specified | Block until complete or fire-and-forget? |
| **Validation** | Strictness not defined | Reject invalid or sanitize and accept? |
| **Defaults** | Not explicitly defined | What if optional field is omitted? |

### Optional (MAY Document)

| Category | Trigger | Example |
|----------|---------|---------|
| **Performance** | Trade-offs possible | Optimize for latency vs throughput? |
| **Caching** | Strategy unclear | Cache in memory, Redis, or no cache? |
| **Error Messages** | Format not specified | Technical detail vs user-friendly? |

---

## Decision Format

### Standard Template

```markdown
## 🎯 **Approved Design Decisions** - [Date]

### **DD-1: [Short Decision Name]**

| Question | [The ambiguous question that needs resolution] |
|----------|------------------------------------------------|
| **Options** | **A**: [Option A description] |
|            | **B**: [Option B description] |
|            | **C**: [Option C description] |
| **Decision** | **Option [X]**: [Chosen option] |
| **Rationale** | [Why this option was chosen - business/technical reasons] |
| **Implementation** | [How this affects code - specific files/functions] |
| **Cross-Reference** | [Related ADRs/DDs if any] |
```

### Example: Workflow CRUD Decisions

```markdown
## 🎯 **Approved Design Decisions** - 2025-11-28

### **DD-1: Workflow Immutability (PUT Behavior)**

| Question | Should PUT /api/v1/workflows/{id} update existing workflow versions? |
|----------|---------------------------------------------------------------------|
| **Options** | **A**: Allow full updates (replace all fields) |
|            | **B**: Allow partial updates (patch semantics) |
|            | **C**: Reject all updates (immutability enforced) |
| **Decision** | **Option C**: PUT is NOT allowed. Immutability enforced. |
| **Rationale** | DD-WORKFLOW-012 mandates immutability for audit trail integrity. Updates create new versions via POST. |
| **Implementation** | PUT returns `405 Method Not Allowed` with RFC 7807 error body. |
| **Cross-Reference** | DD-WORKFLOW-012, DD-004 (RFC 7807) |

### **DD-2: Delete Behavior**

| Question | Should DELETE remove workflows from database or disable them? |
|----------|--------------------------------------------------------------|
| **Options** | **A**: Hard delete (remove from database) |
|            | **B**: Soft delete (set deleted_at timestamp) |
|            | **C**: Disable mechanism (preserve full record with disabled_at, disabled_by, disabled_reason) |
| **Decision** | **Option C**: Use disable mechanism. |
| **Rationale** | Preserves complete audit trail per ADR-034. disabled_at mechanism allows reason tracking. |
| **Implementation** | DELETE sets `disabled_at=NOW()`, requires `reason` in JSON body per DD-API-001. |
| **Cross-Reference** | ADR-034 (Audit), DD-API-001 (HTTP patterns) |

### **DD-3: Version Numbering Strategy**

| Question | How should workflow version numbers be generated? |
|----------|--------------------------------------------------|
| **Options** | **A**: Auto-increment integers (1, 2, 3) |
|            | **B**: Semantic versioning (1.0.0, 1.0.1) |
|            | **C**: Timestamp-based (20251128-001) |
| **Decision** | **Option A**: Auto-increment integers. |
| **Rationale** | Simplest implementation, no parsing required, natural ordering. |
| **Implementation** | `SELECT MAX(version) + 1` on workflow creation in same workflow_id group. |
| **Cross-Reference** | None |
```

---

## Process

### Before Day 1

```
1. Review service specifications
       ↓
2. Identify ambiguous requirements
       ↓
3. List questions in decision format
       ↓
4. Propose options for each question
       ↓
5. Discuss with stakeholder
       ↓
6. Document approved decisions
       ↓
7. Add to implementation plan
       ↓
8. Start Day 1
```

### Approval Requirements

| Decision Type | Approver | Response Time |
|---------------|----------|---------------|
| **API behavior** | Product Owner | 1 business day |
| **Technical trade-offs** | Tech Lead | Same day |
| **Security-related** | Security Team | 2 business days |
| **Cross-service impact** | Architecture Team | 2 business days |

### Sign-Off Template

```markdown
---
**Pre-Implementation Decisions Sign-Off**

All design decisions have been documented and approved.

Stakeholder: _________________ Date: ___________
Developer:   _________________ Date: ___________
---
```

---

## Integration with Implementation Plan

### Location in Implementation Plan

```markdown
# [Service Name] - Implementation Plan

**Version**: vX.Y
...

---

## 🎯 **Pre-Implementation Design Decisions** ⭐ REQUIRED

> **BLOCKING**: Complete this section before starting Day 1.

[Decision content here]

---

## Prerequisites Checklist

Before starting Day 1, ensure:
- [ ] Pre-implementation design decisions documented
- [ ] Decisions approved by stakeholder
- [ ] Sign-off completed
...
```

### Reference During Implementation

During implementation, reference decisions:

```go
// DD-1: Workflow Immutability - PUT rejected per pre-implementation decision
func (h *WorkflowHandler) HandlePUT(w http.ResponseWriter, r *http.Request) {
    // Per DD-1: Immutability enforced, PUT not allowed
    h.writeError(w, http.StatusMethodNotAllowed, "Workflows are immutable; create new version via POST")
}
```

---

## Anti-Patterns

### ❌ Making Decisions During Implementation

```markdown
// ❌ WRONG: Decision made mid-implementation without documentation
// "I decided to make workflows immutable because it seemed right"
```

### ❌ Undocumented Assumptions

```markdown
// ❌ WRONG: Assumption not captured
// Code assumes DELETE is soft-delete, but no decision documented
```

### ❌ Decisions in Slack/Meetings Only

```markdown
// ❌ WRONG: Decision discussed in meeting but not in plan
// "We agreed in standup that PUT would be rejected"
```

---

## Success Criteria

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Pre-implementation completion** | 100% | All ambiguous items have decisions before Day 1 |
| **Decision documentation rate** | 100% | All decisions in standard format |
| **Implementation alignment** | 100% | Code matches documented decisions |
| **Rework reduction** | ≥50% | Fewer implementation changes vs historical |

---

## Benefits

1. **Reduced implementation delays** - No blocking on unclear requirements
2. **Consistent decisions** - Same approach across similar situations
3. **Knowledge preservation** - Decisions captured with rationale
4. **Faster onboarding** - New team members understand "why"
5. **Audit trail** - Historical record of design choices

---

## Cross-References

1. **SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md** (v2.6+): Contains pre-implementation section
2. **DD-API-001**: HTTP header vs JSON body pattern
3. **DD-WORKFLOW-012**: Workflow immutability constraints
4. **ADR-034**: Unified audit table design

---

**Document Owner**: Platform Architecture Team
**Last Updated**: 2025-11-28
**Next Review**: After 3 service implementations

