# ADR-061: Design Decision (DD) Template Standard

**Date**: March 4, 2026
**Status**: ✅ Approved
**Purpose**: Establish standard template for all Kubernaut design decisions
**Rationale**: Ensure consistency, traceability, and comprehensive documentation across all DDs — mirrors ADR-037 (BR template standard)

---

## 🎯 **DECISION**

**All design decisions in Kubernaut SHALL follow the standardized DD template format defined in this ADR.**

**Enforcement**: Mandatory for all new DDs starting March 4, 2026

---

## 📋 **DD TEMPLATE STRUCTURE**

### **Mandatory Sections**

All DD documents MUST include these sections in this order:

```markdown
# DD-{CATEGORY}-{NUMBER}: {Title}

**Status**: {Proposed/Approved/Deprecated/Superseded}
**Decision Date**: {YYYY-MM-DD}
**Version**: {X.Y}
**Confidence**: {0-100}%
**Deciders**: {Team/Role names}
**Applies To**: {Service(s) or component(s) affected}

**Related Business Requirements**:
- BR-{CATEGORY}-{NUMBER}: {Title}

**Related Design Decisions**:
- DD-{CATEGORY}-{NUMBER}: {Title}

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | {YYYY-MM-DD} | {Author} | Initial design |

---

## Context & Problem

### Current State
{Description of the current system state relevant to this decision}

### Problem Statement
{Clear description of the technical problem or gap being addressed}

### Constraints
- {Constraint 1}
- {Constraint 2}

---

## Decision Drivers

1. {Driver 1 — what forces influence the decision}
2. {Driver 2}
3. {Driver 3}

---

## Alternatives Considered

### Alternative A: {Title} {✅ CHOSEN / ❌ Rejected}

**Approach**: {Description of the approach}

**Pros**:
- {Pro 1}
- {Pro 2}

**Cons**:
- {Con 1}
- {Con 2}

**Confidence**: {0-100}% ({chosen/rejected})

### Alternative B: {Title} {✅ CHOSEN / ❌ Rejected}

{Same structure as Alternative A}

---

## Decision

### Chosen: Alternative {X} — {Title}

{Rationale for why this alternative was chosen over others}

### Architecture

{Diagrams, data flow, component interaction — use ASCII art or mermaid}

### Implementation Details

{Code examples, API specs, configuration, schema definitions as needed}

---

## Consequences

### Positive Consequences
1. {Positive consequence 1}
2. {Positive consequence 2}

### Negative Consequences
1. {Negative consequence 1}
   - **Mitigation**: {How this is addressed}

### Risks
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| {Risk 1} | {Low/Medium/High} | {Low/Medium/High} | {Mitigation} |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-{CATEGORY}-{NUMBER} | ✅ | {How this DD satisfies the BR} |

---

## Validation Strategy

1. {How will the decision be validated — tests, metrics, load tests, etc.}
2. {Success criteria for the decision}

---

## References

- {Link or path to related documents}
- {External references (specs, libraries, RFCs)}

---

**Document Version**: {X.Y}
**Last Updated**: {YYYY-MM-DD}
```

---

## 🏷️ **DD NAMING CONVENTION**

### **Format**: `DD-{CATEGORY}-{NUMBER}`

**Category Codes** (aligned with BR categories from ADR-037 + infrastructure categories):

| Category Code | Domain | Example |
|---|---|---|
| **HAPI** | HolmesGPT-API / AI Investigation Service | DD-HAPI-017 |
| **GATEWAY** | Gateway Service (signal ingestion) | DD-GATEWAY-011 |
| **WE** | Workflow Engine | DD-WE-006 |
| **RO** | Remediation Orchestrator | DD-RO-001 |
| **SP** | Signal Processing | DD-SP-001 |
| **EM** | Effectiveness Monitor | DD-EM-002 |
| **AUDIT** | Audit Trail | DD-AUDIT-003 |
| **WORKFLOW** | Workflow/Playbook Schema & Lifecycle | DD-WORKFLOW-004 |
| **PLAYBOOK** | Playbook Catalog | DD-PLAYBOOK-003 |
| **CONTRACT** | Cross-Service Integration Contracts | DD-CONTRACT-002 |
| **CRD** | CRD Design & API Groups | DD-CRD-001 |
| **WEBHOOK** | Admission Webhooks | DD-WEBHOOK-001 |
| **ACTIONTYPE** | ActionType CRD Lifecycle | DD-ACTIONTYPE-001 |
| **SHARED** | Shared Libraries | DD-SHARED-001 |
| **METRICS** | Observability & Metrics | DD-METRICS-001 |
| **SEVERITY** | Severity Classification | DD-SEVERITY-001 |
| **INFRA** | Infrastructure Patterns | DD-INFRA-001 |
| **PERF** | Performance | DD-PERF-001 |
| **TEST** | Testing Strategy & Patterns | DD-TEST-001 |
| **TESTING** | Testing Infrastructure | DD-TESTING-002 |
| **CICD** | CI/CD Pipeline | DD-CICD-001 |
| **BUILD** | Build System | DD-BUILD-001 |
| **DOCS** | Documentation Standards | DD-DOCS-001 |
| **API** | API Design | DD-API-002 |
| **AIANALYSIS** | AI Analysis Service | DD-AIANALYSIS-004 |
| **CONTROLLER** | Controller Patterns | DD-CONTROLLER-001 |
| **SCOPE** | Resource Scope Management | DD-SCOPE-001 |
| **TOOLSET** | Toolset Configuration | DD-TOOLSET-001 |
| **SOC2** | Compliance & Security | DD-SOC2-001 |
| **NOT** | Notification Controller | DD-NOT-001 |

**Number Format**: Zero-padded 3 digits within each category (001, 002, ..., 999)

**File naming**: `DD-{CATEGORY}-{NUMBER}-{title-slug}.md`
Example: `DD-HAPI-017-three-step-workflow-discovery-integration.md`

---

## 📂 **DD DOCUMENT LOCATION**

### **Standard Location**

```
docs/architecture/decisions/DD-{CATEGORY}-{NUMBER}-{title-slug}.md
```

All DDs live in a single flat directory alongside ADRs. The `DD-` prefix distinguishes them from `ADR-` documents.

---

## 🔍 **DD vs ADR — WHEN TO USE WHICH**

| Criterion | DD (Design Decision) | ADR (Architecture Decision Record) |
|-----------|---------------------|--------------------------------------|
| **Scope** | Internal implementation choices | System-level architectural changes |
| **External contract change** | No — same API, same CRDs, same topology | Yes — new APIs, new CRDs, topology changes |
| **Examples** | Framework selection, algorithm choice, async model, toolset implementation strategy | New service introduction, CRD schema changes, cross-service contract changes |
| **Reversibility** | Typically reversible within a service | Harder to reverse — affects multiple consumers |
| **Approval** | Team-level | Architecture team |

**Rule of thumb**: If consumers of the service see no behavioral difference, it's a DD. If the change requires consumers to adapt, it's an ADR.

---

## 🔍 **DD VALIDATION CHECKLIST**

Before approving any DD document, verify:

### **Completeness**
- ✅ All mandatory sections present
- ✅ DD ID follows naming convention (DD-{CATEGORY}-{NUMBER})
- ✅ Status, date, version, and confidence populated
- ✅ At least one BR referenced (business backing)
- ✅ Changelog present with initial entry

### **Quality**
- ✅ Problem statement is specific and evidence-based
- ✅ At least two alternatives considered (including the chosen one)
- ✅ Each alternative has Pros, Cons, and Confidence rating
- ✅ Decision rationale explains why the chosen option wins
- ✅ Consequences include both positive and negative
- ✅ Negative consequences have mitigations

### **Traceability**
- ✅ Related BRs referenced
- ✅ Related DDs cross-referenced
- ✅ Impacted services identified in "Applies To"
- ✅ Validation strategy defined

### **Approval**
- ✅ Confidence rating is ≥60% (minimum for approval)
- ✅ Deciders are identified
- ✅ Status reflects current state

---

## 🔄 **DD LIFECYCLE STATES**

### **State Transitions**

```
Proposed → Approved → Implemented → Deprecated
   ↓                                    ↓
Rejected                           Superseded (by DD-{NEW})
```

**State Definitions**:

| State | Meaning | Next Actions |
|---|---|---|
| **Proposed** | DD drafted, awaiting review | Team review, stakeholder approval |
| **Approved** | DD approved for implementation | Begin implementation |
| **Implemented** | DD fully implemented and validated | Monitor, update version if evolved |
| **Rejected** | DD rejected after review | Document rejection reason |
| **Deprecated** | DD no longer applicable | Reference reason, archive |
| **Superseded** | DD replaced by newer DD | Reference replacement DD |

---

## 🔗 **INTEGRATION WITH OTHER ARTIFACTS**

### **DD → BR Relationship**

Every DD MUST reference at least one BR that provides its business justification. DDs without business backing violate the core rule: "EVERY code change MUST be backed by at least ONE business requirement."

### **DD → ADR Relationship**

When a DD reveals that the chosen approach requires architectural changes (new APIs, new CRDs, cross-service contract changes), an ADR MUST be created to capture the architectural decision separately.

### **DD → Test Plan Relationship**

The Validation Strategy section in the DD informs the test plan. Test scenarios SHOULD reference the DD they validate:

```go
// DD-HAPI-017: Three-step workflow discovery
It("should list available actions before listing workflows", func() {
    // Validates DD-HAPI-017 enforcement flow
})
```

---

## ✅ **APPROVAL**

**Status**: ✅ **APPROVED**
**Date**: March 4, 2026
**Decision**: Establish DD template as mandatory standard for all new design decisions
**Rationale**: 214 existing DDs follow inconsistent formats. A standard template ensures consistency, traceability, and quality — mirroring the success of ADR-037 for BRs.
**Approved By**: Architecture Team
**Effective Date**: March 4, 2026 (all new DDs)
**Migration Plan**: Existing DDs grandfathered, new DDs must follow template

---

## 📚 **REFERENCES**

### **Related ADRs**
- **ADR-037**: Business Requirement Template Standard (sibling standard for BRs)

### **Example DD Documents (pre-template, good exemplars)**
- **DD-HAPI-017**: Three-Step Workflow Discovery Integration (most complete existing DD)
- **DD-HAPI-015**: Single-Worker Async Architecture (clean options analysis)
- **DD-HAPI-005**: LLM Input Sanitization (good security DD with architecture diagram)

---

**Document Version**: 1.0
**Last Updated**: March 4, 2026
**Status**: ✅ Approved — Mandatory for all new DDs
