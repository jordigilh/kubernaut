# Kubernaut Documentation Directory Structure Guide

**Date**: January 26, 2026  
**Status**: ✅ **AUTHORITATIVE GUIDE**  
**Purpose**: Reference for organizing documentation across the Kubernaut project

---

## 📋 **DIRECTORY STRUCTURE OVERVIEW**

### **Root-Level Directories**

```
docs/
├── architecture/               # Architecture decisions and patterns
│   ├── decisions/             # ADRs and Design Decisions (DD-*)
│   ├── patterns/              # Reusable architecture patterns
│   ├── diagrams/              # Architecture diagrams
│   └── specifications/        # Technical specifications
├── development/               # Development guides and standards
│   ├── methodology/           # APDC, TDD, testing strategy
│   ├── business-requirements/ # Business requirements (BR-*)
│   ├── testing/               # Test plans and strategies
│   └── templates/             # Document templates
├── handoff/                   # Session handoff documents (AI session summaries)
├── services/                  # Service-specific documentation
│   ├── datastorage/          # DataStorage service docs
│   ├── gateway/              # Gateway service docs
│   ├── holmesgpt-api/        # HAPI service docs
│   └── [service-name]/       # Other services
├── test/                      # Test documentation and scenarios
├── operations/                # Operational guides and runbooks
├── troubleshooting/           # Troubleshooting guides
└── guides/                    # User and developer guides
```

---

## 📚 **DIRECTORY USAGE GUIDE**

### **1. `docs/architecture/decisions/`**

**Purpose**: Authoritative design decisions and architecture decision records

**File Naming**: 
- `ADR-NNN-title.md` (Architecture Decision Records)
- `DD-[CATEGORY]-NNN-title.md` (Design Decisions)

**Categories**: AUTH, WORKFLOW, STORAGE, AI, TEST, API, SOC2, INTEGRATION

**Examples**:
- `ADR-036-authentication-authorization-strategy.md`
- `DD-AUTH-011-granular-rbac-sar-verb-mapping.md`
- `DD-WORKFLOW-002-mcp-workflow-catalog-architecture.md`

**When to Use**:
- ✅ Architectural decisions with long-term impact
- ✅ Technical choices affecting multiple services
- ✅ Security and compliance decisions
- ✅ API contracts and protocols
- ❌ Session summaries (use `handoff/`)
- ❌ Temporary implementation notes (use `handoff/`)

---

### **2. `docs/handoff/`**

**Purpose**: Session handoff documents - summaries of work completed in AI-assisted development sessions

**File Naming**: `[SERVICE]_[TOPIC]_[STATUS]_[DATE].md`

**Examples**:
- `DS_E2E_COVERAGE_SUCCESS_DEC_22_2025.md`
- `HAPI_ALL_TEST_TIERS_FINAL_STATUS_DEC_25_2025.md`
- `AA_UNIT_TEST_FAILURES_TRIAGE.md`
- `DD_AUTH_013_IMPLEMENTATION_COMPLETE_JAN_26_2026.md` ← NEW

**When to Use**:
- ✅ Session completion summaries
- ✅ Implementation status updates
- ✅ Triage reports from AI sessions
- ✅ "Handoff" between development sessions
- ✅ Work-in-progress snapshots
- ❌ Permanent design decisions (use `architecture/decisions/`)
- ❌ Ongoing documentation (use service-specific dirs)

**Content Pattern**:
```markdown
# [Service/Topic] - [Status/Action] ([Date])

**Date**: [Date]
**Status**: ✅ COMPLETE / ⏳ IN PROGRESS / 🚨 BLOCKED
**Service**: [Service Name]

## Executive Summary
[What was accomplished]

## Completed Tasks
[List of tasks]

## Next Steps
[What's next]
```

---

### **3. `docs/development/`**

**Purpose**: Developer guides, standards, and methodologies

**Subdirectories**:
- `methodology/` - APDC, TDD, development processes
- `business-requirements/` - BR-* requirements and guidelines
- `testing/` - Test plans, strategies, and standards
- `templates/` - Document templates
- `e2e-testing/` - E2E test documentation
- `integration-testing/` - Integration test patterns
- `SOC2/` - SOC2 compliance documentation

**Examples**:
- `methodology/APDC_FRAMEWORK.md`
- `business-requirements/TESTING_GUIDELINES.md`
- `testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md`

**When to Use**:
- ✅ Development methodology and standards
- ✅ Business requirement definitions
- ✅ Test plan templates
- ✅ Coding standards and patterns
- ❌ Completed implementation summaries (use `handoff/`)

---

### **4. `docs/services/[service-name]/`**

**Purpose**: Service-specific documentation (design, implementation, guides)

**Structure**:
```
services/
└── datastorage/
    ├── design/              # Service design documents
    ├── implementation/      # Implementation guides
    ├── testing/             # Service-specific test docs
    └── operations/          # Service operations guides
```

**Examples**:
- `services/datastorage/design/AUDIT_STORAGE_ARCHITECTURE.md`
- `services/gateway/implementation/ROUTING_IMPLEMENTATION.md`

**When to Use**:
- ✅ Service-specific design documents
- ✅ Service implementation guides
- ✅ Service-specific testing strategies
- ❌ Cross-service architecture decisions (use `architecture/decisions/`)

---

### **5. `docs/operations/`**

**Purpose**: Operational guides and runbooks

**Subdirectories**:
- `runbooks/` - Service-specific runbooks

**Examples**:
- `operations/runbooks/workflowexecution-runbook.md`

**When to Use**:
- ✅ Production operational procedures
- ✅ Incident response runbooks
- ✅ Deployment guides
- ❌ Development documentation (use `development/`)

---

### **6. `docs/test/`**

**Purpose**: Test documentation and test scenarios

**Subdirectories**:
- `unit/` - Unit test documentation
- `integration/` - Integration test documentation
- `e2e/` - E2E test documentation

**When to Use**:
- ✅ Test scenario documentation
- ✅ Test data and fixtures
- ✅ Test architecture documentation
- ❌ Test plans (use `development/testing/`)

---

### **7. `docs/troubleshooting/`**

**Purpose**: Troubleshooting guides and known issues

**Subdirectories**:
- `service-specific/` - Service-specific issues

**Examples**:
- `troubleshooting/DATASTORAGE_VERSION_ERRORS.md`
- `troubleshooting/service-specific/workflowexecution-issues.md`

**When to Use**:
- ✅ Known issues and workarounds
- ✅ Debugging guides
- ✅ Common problems and solutions
- ❌ Architecture decisions (use `architecture/decisions/`)

---

### **8. `docs/triage/`**

**Purpose**: Temporary triage reports and analysis

**When to Use**:
- ✅ Issue analysis reports
- ✅ Bug investigation notes
- ✅ Temporary problem analysis
- ❌ Permanent documentation (consolidate into appropriate dirs)

**Note**: Documents here should eventually be:
- Resolved and moved to `architecture/decisions/`
- Consolidated into service docs
- Moved to `troubleshooting/` if recurring issue
- Archived or deleted if obsolete

---

## 🎯 **DECISION FLOWCHART**

```
Is this a design decision with long-term impact?
├─ YES → docs/architecture/decisions/ (ADR-* or DD-*)
└─ NO ↓

Is this a session handoff or implementation summary?
├─ YES → docs/handoff/ ([SERVICE]_[TOPIC]_[DATE].md)
└─ NO ↓

Is this service-specific documentation?
├─ YES → docs/services/[service-name]/
└─ NO ↓

Is this a development guide or standard?
├─ YES → docs/development/
└─ NO ↓

Is this an operational runbook?
├─ YES → docs/operations/runbooks/
└─ NO ↓

Is this a troubleshooting guide?
├─ YES → docs/troubleshooting/
└─ NO ↓

Is this a temporary triage report?
└─ YES → docs/triage/ (later consolidate or delete)
```

---

## 📝 **FILE NAMING CONVENTIONS**

### **Design Decisions**
```
DD-[CATEGORY]-NNN-descriptive-title.md

Examples:
DD-AUTH-011-granular-rbac-sar-verb-mapping.md
DD-WORKFLOW-002-mcp-workflow-catalog-architecture.md
DD-TEST-001-port-allocation-strategy.md
```

### **Handoff Documents**
```
[SERVICE]_[TOPIC]_[STATUS]_[DATE].md

Examples:
DS_E2E_COVERAGE_SUCCESS_DEC_22_2025.md
HAPI_INTEGRATION_TEST_COMPLETE_DEC_24_2025.md
DD_AUTH_013_IMPLEMENTATION_COMPLETE_JAN_26_2026.md
```

### **Architecture Decision Records**
```
ADR-NNN-descriptive-title.md

Examples:
ADR-036-authentication-authorization-strategy.md
ADR-034-unified-audit-table.md
```

---

## 🔄 **MIGRATION GUIDELINES**

### **From Project Root to docs/**

**Rule**: Documents should NEVER remain in project root

**Migration Path**:
1. **Implementation summaries** → `docs/handoff/`
2. **Design decisions** → `docs/architecture/decisions/`
3. **Triage reports** → `docs/triage/` or `docs/handoff/`
4. **Service guides** → `docs/services/[service-name]/`

---

### **From docs/triage/ to Permanent Location**

**Triage is temporary**. When issue is resolved:
1. **Create DD-* if architectural decision** → `architecture/decisions/`
2. **Update service docs** → `services/[service-name]/`
3. **Add to troubleshooting** → `troubleshooting/` (if recurring)
4. **Archive or delete** → If obsolete

---

## 📋 **CURRENT SESSION DOCUMENTS**

### **Created in Project Root** (Need Migration)

| File | Type | Destination |
|------|------|-------------|
| `DD-AUTH-011-012-EXECUTION-SUMMARY.md` | Session handoff | `docs/handoff/` |
| `DD-AUTH-013-COMPLETE-IMPLEMENTATION-SUMMARY.md` | Session handoff | `docs/handoff/` |
| `DD-AUTH-013-FINAL-STATUS.md` | Session handoff | `docs/handoff/` |
| `DD-AUTH-013-HAPI-OPENAPI-TRIAGE.md` | Triage report | `docs/handoff/` |
| `DD-AUTH-013-OPENAPI-UPDATE-SUMMARY.md` | Implementation summary | `docs/handoff/` |

---

### **Created in docs/architecture/decisions/** ✅ CORRECT

| File | Type | Status |
|------|------|--------|
| `DD-AUTH-011-E2E-TESTING-GUIDE.md` | Design decision | ✅ Correct location |
| `DD-AUTH-011-012-COMPLETE-STATUS.md` | Design decision | ✅ Correct location |
| `DD-AUTH-012-ose-oauth-proxy-sar-rest-api-endpoints.md` | Design decision | ✅ Correct location |
| `DD-AUTH-012-IMPLEMENTATION-SUMMARY.md` | Design decision | ✅ Correct location |
| `DD-AUTH-013-http-status-codes-oauth-proxy.md` | Design decision | ✅ Correct location |

---

## ✅ **RECOMMENDATIONS**

### **For Current Session**

1. **Move** implementation summaries from project root → `docs/handoff/`
2. **Rename** to follow handoff naming convention:
   - `DD-AUTH-011-012-EXECUTION-SUMMARY.md` → `DD_AUTH_011_012_EXECUTION_SUMMARY_JAN_26_2026.md`
   - `DD-AUTH-013-COMPLETE-IMPLEMENTATION-SUMMARY.md` → `DD_AUTH_013_IMPLEMENTATION_COMPLETE_JAN_26_2026.md`
   - etc.

3. **Keep** DD-AUTH-013 in `docs/architecture/decisions/` (it's authoritative)

---

### **For Future Sessions**

1. **NEVER** create documents in project root
2. **Session summaries** → Immediately to `docs/handoff/`
3. **Design decisions** → Immediately to `docs/architecture/decisions/`
4. **Follow naming conventions** for easy searchability

---

## 🔍 **QUICK REFERENCE**

| Document Type | Location | Naming Pattern |
|--------------|----------|----------------|
| **Design Decision** | `architecture/decisions/` | `DD-[CAT]-NNN-title.md` |
| **ADR** | `architecture/decisions/` | `ADR-NNN-title.md` |
| **Session Handoff** | `handoff/` | `[SVC]_[TOPIC]_[DATE].md` |
| **Test Plan** | `development/testing/` | `*_TEST_PLAN*.md` |
| **Business Requirement** | `development/business-requirements/` | `BR-[CAT]-NNN.md` |
| **Runbook** | `operations/runbooks/` | `[service]-runbook.md` |
| **Troubleshooting** | `troubleshooting/` | `[ISSUE]-guide.md` |
| **Service Guide** | `services/[name]/` | Context-specific |

---

**Document Version**: 1.0  
**Last Updated**: January 26, 2026  
**Authority**: Reference for all future documentation placement
