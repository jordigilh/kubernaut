# ADR-037: Business Requirement (BR) Template Standard

**Date**: November 5, 2025
**Status**: ✅ Approved
**Purpose**: Establish standard template for all Kubernaut business requirements
**Rationale**: Ensure consistency, traceability, and comprehensive documentation across all BRs

---

## 🎯 **DECISION**

**All business requirements in Kubernaut SHALL follow the standardized BR template format defined in this ADR.**

**Enforcement**: Mandatory for all new BRs starting November 5, 2025

---

## 📋 **BR TEMPLATE STRUCTURE**

### **Mandatory Sections**

All BR documents MUST include these sections in this order:

```markdown
# BR-{CATEGORY}-{NUMBER}: {Title}

**Business Requirement ID**: BR-{CATEGORY}-{NUMBER}
**Category**: {Service/Domain Name}
**Priority**: {P0/P1/P2/P3}
**Target Version**: {V1/V2/etc}
**Status**: {Pending/Approved/Implemented/Deprecated}
**Date**: {YYYY-MM-DD}

---

## 📋 **Business Need**

### **Problem Statement**
{Clear description of the business problem or gap}

**Current Limitations**:
- ❌ {Limitation 1}
- ❌ {Limitation 2}

**Impact**:
- {Business impact 1}
- {Business impact 2}

---

## 🎯 **Business Objective**

{One sentence objective statement}

### **Success Criteria**
1. ✅ {Measurable success criterion 1}
2. ✅ {Measurable success criterion 2}

---

## 📊 **Use Cases**

### **Use Case 1: {Title}**

**Scenario**: {Description}

**Current Flow**:
```
1. {Step 1}
2. {Step 2}
3. ❌ {Problem/Gap}
```

**Desired Flow with {BR-XXX-YYY}**:
```
1. {Step 1}
2. {Step 2}
3. ✅ {Solution}
```

---

## 🔧 **Functional Requirements**

### **FR-{BR-ID}-01: {Title}**

**Requirement**: {SHALL/SHOULD/MAY statement}

**Implementation Details**:
{Code examples, API specs, schema definitions}

**Acceptance Criteria**:
- ✅ {Testable criterion 1}
- ✅ {Testable criterion 2}

---

## 📈 **Non-Functional Requirements**

### **NFR-{BR-ID}-01: {Category}**
{Performance/Security/Compliance/Scalability requirements}

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ {Dependency 1}

### **Downstream Impacts**
- ✅ {Impacted service/component 1}

---

## 🚀 **Implementation Phases**

### **Phase 1: {Title}** ({Duration})
- {Task 1}
- {Task 2}

**Total Estimated Effort**: {Duration}

---

## 📊 **Success Metrics**

### **{Metric Category}**
- **Target**: {Quantifiable target}
- **Measure**: {How to measure}

---

## 🔄 **Alternatives Considered**

### **Alternative 1: {Title}**
**Approach**: {Description}
**Rejected Because**: {Reason}

---

## ✅ **Approval**

**Status**: {Approved/Pending/Rejected}
**Date**: {YYYY-MM-DD}
**Decision**: {Summary}
**Approved By**: {Role/Team}
**Related ADR**: {Link to architectural decision if applicable}

---

## 📚 **References**

### **Related Business Requirements**
- BR-XXX-YYY: {Description}

### **Related Documents**
- {Document path}: {Description}

---

**Document Version**: 1.0
**Last Updated**: {YYYY-MM-DD}
**Status**: {Current status}
```

---

## 🏷️ **BR NAMING CONVENTION**

### **Format**: `BR-{CATEGORY}-{NUMBER}`

**Category Codes** (Standard across Kubernaut):

| Category Code | Service/Domain | Example |
|---|---|---|
| **WORKFLOW** | Deprecated - see REMEDIATION | BR-WORKFLOW-001 ❌ |
| **REMEDIATION** | RemediationExecutor Service | BR-REMEDIATION-001 ✅ |
| **PLAYBOOK** | Playbook Catalog Service | BR-PLAYBOOK-001 ✅ |
| **AI** | AI/LLM Service | BR-AI-001 |
| **INTEGRATION** | Context API / Cross-Service | BR-INTEGRATION-001 |
| **SECURITY** | Security features and access controls | BR-SECURITY-001 |
| **PLATFORM** | Kubernetes and infrastructure platform | BR-PLATFORM-001 |
| **API** | API Gateway Service | BR-API-001 |
| **STORAGE** | Data Storage Service | BR-STORAGE-001 |
| **MONITORING** | Observability, metrics, monitoring | BR-MONITORING-001 |
| **SAFETY** | Safety frameworks and validation | BR-SAFETY-001 |
| **PERFORMANCE** | Performance optimization | BR-PERFORMANCE-001 |
| **GATEWAY** | Gateway Service (signal ingestion) | BR-GATEWAY-001 |
| **EFFECTIVENESS** | Effectiveness Monitor Service | BR-EFFECTIVENESS-001 ✅ |

**Number Format**: Zero-padded 3 digits (001, 002, 003, ..., 999)

**Examples**:
- ✅ `BR-STORAGE-031`: Data Storage Service requirement #31
- ✅ `BR-REMEDIATION-015`: RemediationExecutor requirement #15
- ✅ `BR-PLAYBOOK-001`: Playbook Catalog requirement #1
- ✅ `BR-EFFECTIVENESS-001`: Effectiveness Monitor requirement #1
- ❌ `BR-WORKFLOW-001`: DEPRECATED (use BR-REMEDIATION instead)

---

## 📂 **BR DOCUMENT LOCATION**

### **Standard Locations**

1. **Formal BR Documents** (Full template):
   ```
   docs/requirements/BR-{CATEGORY}-{NUMBER}-{title-slug}.md
   ```
   Example: `docs/requirements/BR-STORAGE-031-multi-dimensional-success-tracking.md`

2. **BR Coverage Matrices** (Testing):
   ```
   docs/services/{service-type}/{service-name}/testing/BR-COVERAGE-MATRIX.md
   ```

3. **BR Implementation Plans** (Service-specific):
   ```
   docs/services/{service-type}/{service-name}/implementation/IMPLEMENTATION_PLAN_VX.Y.md
   ```

4. **BR Cross-Service Summaries** (Architecture-level):
   ```
   docs/architecture/decisions/ADR-XXX-CROSS-SERVICE-BRS.md
   ```
   Example: `docs/architecture/decisions/ADR-033-CROSS-SERVICE-BRS.md`

---

## 🔍 **BR VALIDATION CHECKLIST**

Before approving any BR document, verify:

### **Completeness**
- ✅ All mandatory sections present
- ✅ BR ID follows naming convention (BR-{CATEGORY}-{NUMBER})
- ✅ Priority assigned (P0/P1/P2/P3)
- ✅ Status and dates populated
- ✅ Business objective is clear and measurable

### **Quality**
- ✅ Problem statement is specific and evidence-based
- ✅ Success criteria are measurable and testable
- ✅ Use cases demonstrate real-world scenarios
- ✅ Functional requirements use SHALL/SHOULD/MAY
- ✅ Acceptance criteria are specific and verifiable

### **Traceability**
- ✅ Related ADRs referenced
- ✅ Dependencies documented
- ✅ Impacted services identified
- ✅ Implementation phases defined

### **Approval**
- ✅ Approval status documented
- ✅ Approval date recorded
- ✅ Approving authority identified

---

## 🔄 **BR LIFECYCLE STATES**

### **State Transitions**

```
Pending → Approved → Implemented → Deprecated
   ↓         ↓
Rejected  Deferred
```

**State Definitions**:

| State | Meaning | Next Actions |
|---|---|---|
| **Pending** | BR drafted, awaiting approval | Architecture review, stakeholder approval |
| **Approved** | BR approved for implementation | Begin implementation planning |
| **Implemented** | BR fully implemented and tested | Monitor success metrics, close ticket |
| **Rejected** | BR rejected after review | Document rejection reason, archive |
| **Deferred** | BR approved but postponed to future version | Document deferral reason, revisit in target version |
| **Deprecated** | BR superseded by new requirements | Reference replacement BR, archive |

---

## 📊 **BR PRIORITY LEVELS**

| Priority | Meaning | Timeline | Examples |
|---|---|---|---|
| **P0** | Critical - Blocks release | Must implement immediately | Core API functionality, critical bug fixes |
| **P1** | High - Impacts major feature | Implement in current sprint | New features, performance improvements |
| **P2** | Medium - Nice to have | Implement in next 1-2 sprints | UX enhancements, non-critical optimizations |
| **P3** | Low - Future consideration | Backlog, future versions | Speculative features, experimental capabilities |

---

## 🔗 **INTEGRATION WITH OTHER ARTIFACTS**

### **BR → ADR Relationship**

**When BR requires ADR**:
- Architectural impact (multi-service changes)
- Technology selection decisions
- Design pattern establishment
- Non-functional requirement trade-offs

**Example**:
- **BR-STORAGE-031**: Multi-dimensional success tracking (business need)
- **ADR-033**: Remediation Playbook Catalog (architectural solution)

### **BR → Implementation Plan Relationship**

All implementation plans MUST reference BRs they address:

```markdown
### **BR-STORAGE-031-01: Incident-Type Success Rate API**
**Implementation**: Day 13-14
**Test Coverage**: TC-ADR033-01 to TC-ADR033-06
**Confidence**: 95%
```

### **BR → Test Coverage Relationship**

All tests MUST map to specific BRs:

```go
// BR-STORAGE-031-01: Calculate success rate by incident type
It("should calculate incident-type success rate with exact counts", func() {
    // BEHAVIOR: Endpoint returns incident-type aggregation
    // CORRECTNESS: Success rate is exactly 0.80
})
```

---

## ✅ **APPROVAL**

**Status**: ✅ **APPROVED**
**Date**: November 5, 2025
**Decision**: Establish BR template as mandatory standard for all new business requirements
**Rationale**: Ensures consistency, traceability, and comprehensive documentation
**Approved By**: Architecture Team
**Effective Date**: November 5, 2025 (all new BRs)
**Migration Plan**: Existing BRs grandfathered, new BRs must follow template

---

## 📚 **REFERENCES**

### **Example BR Documents**

1. **BR-RR-001**: Forced Recommendation and Manual Override
   - Location: `docs/requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md`
   - Example of fully compliant BR document

2. **BR-PA-008**: AI Effectiveness Assessment Test
   - Location: `docs/test/integration/test_suites/02_ai_decision_making/BR-PA-008_effectiveness_assessment_test.md`
   - Example of BR-to-test mapping

### **Related ADRs**

- **ADR-033**: Remediation Playbook Catalog (multi-service BRs)
- **ADR-033-A**: Cross-Service BRs index

### **BR Tools**

- **BR Validation Script**: `scripts/validate_br_format.sh`
- **BR Template Generator**: `scripts/generate_br_template.sh`
- **BR Coverage Report**: `scripts/br_coverage_report.sh`

---

**Document Version**: 1.0
**Last Updated**: November 5, 2025
**Status**: ✅ Approved - Mandatory for all new BRs


