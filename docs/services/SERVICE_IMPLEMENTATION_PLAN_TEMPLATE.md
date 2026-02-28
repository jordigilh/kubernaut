# [Service Name] - Implementation Plan Template

**Filename Convention**: `IMPLEMENTATION_PLAN_V<semantic_version>.md` (e.g., `IMPLEMENTATION_PLAN_V1.3.md`)
**Version**: v3.1 - V1.0 MANDATORY MATURITY CHECKLIST
**Last Updated**: 2025-12-19
**Timeline**: [X] days (11-12 days typical)
**Status**: 📋 DRAFT | ⏳ IN REVIEW | ✅ VALIDATED - Ready for Implementation
**Quality Level**: Matches Data Storage v4.1 and Notification V3.0 standards

**Change Log**:
- **v3.1** (2025-12-19): 🎯 **V1.0 MANDATORY MATURITY CHECKLIST**
  - ✅ **V1.0 Maturity Checklist Section**: Mandatory features for CRD controllers and stateless services ⭐ NEW
  - ✅ **Test Requirements Integration**: Links to TESTING_GUIDELINES.md maturity testing requirements ⭐ NEW
  - ✅ **Test Plan Template Reference**: Standardized test plan for maturity validation ⭐ NEW
  - ✅ **Living Document Pattern**: Guidance for maintaining requirements as new ADRs/DDs are created ⭐ NEW
  - 📏 **Template size**: ~8,300 lines (growth for maturity checklist)
  - 📏 **Source**: V1_0_SERVICE_MATURITY_TRIAGE learnings (December 2025 maturity audit)
- **v3.0** (2025-12-01): 🎯 **CROSS-TEAM VALIDATION + CRD API GROUP STANDARD**
  - ✅ **Cross-Team Validation Section**: New section for multi-team dependency sign-off ⭐ NEW
  - ✅ **HANDOFF/RESPONSE Pattern**: Documentation for cross-team validation records ⭐ NEW
  - ✅ **Pre-Implementation Validation Gate**: Formal checklist before Day 1 ⭐ NEW
  - ✅ **CRD API Group Standard**: DD-CRD-001 unified `.ai` domain for CRD controllers ⭐ NEW
  - ✅ **Industry Best Practices Table**: Reusable format for architectural justification ⭐ NEW
  - ✅ **Risk Mitigation Status Tracking**: Links risks to specific days with status tracking ⭐ NEW
  - ✅ **Port Allocation Consolidated Table**: Enhanced DD-TEST-001 compliance format ⭐ NEW
  - 📏 **Template size**: ~7,600 lines (growth for cross-team validation patterns)
  - 📏 **Source**: SignalProcessing IMPLEMENTATION_PLAN_V1.16 learnings (100% validation confidence)
- **v2.9** (2025-11-30): 🎯 **CODE EXAMPLE DD-005 COMPLIANCE**
  - ✅ **Shared Library Extraction Examples**: Updated to use `logr.Logger` (per DD-005 v2.0)
  - ✅ **Redis Cache Example**: Changed from `*zap.Logger` to `logr.Logger` in "After" section
  - ✅ **Logging Syntax**: Updated to use key-value pairs (not zap helpers)
  - ✅ **DD-005 Reference**: Added explicit reference to authoritative document
  - 📏 **Template size**: ~7,400 lines (no growth, code example fixes only)
- **v2.8** (2025-11-28): 🎯 **UNIFIED LOGGING FRAMEWORK (DD-005 v2.0)**
  - ✅ **Logging Framework Decision Matrix**: Added comprehensive section for `logr.Logger` usage
  - ✅ **Implementation Patterns**: Stateless services use `zapr.NewLogger()`, CRD controllers use native `ctrl.Log`
  - ✅ **Shared Library Standard**: All `pkg/*` libraries MUST accept `logr.Logger` (not `*zap.Logger`)
  - ✅ **Forbidden Patterns**: Documented anti-patterns to avoid
  - 📏 **Template size**: ~7,400 lines (growth for logging framework guidance)
- **v2.7** (2025-11-28): 🎯 **SCOPE ANNOTATIONS + TEST HELPERS + OPENAPI PRE-PHASE**
  - ✅ **Scope Annotations**: All new sections clearly marked as COMMON, STATELESS, or CRD CONTROLLER
  - ✅ **OpenAPI Pre-Phase Step** (Day 1): Update OpenAPI spec before implementation (STATELESS)
  - ✅ **E2E Test Helper Patterns**: Reusable CRUD helpers for HTTP services (STATELESS)
  - ✅ **CRD Controller E2E Helper Patterns**: Reusable K8s client helpers (CRD CONTROLLER)
  - ✅ **Async Timing Example**: Added DD-3 example for sync vs async decisions (COMMON)
  - ✅ **CRD Controller Decision Examples**: Added reconciliation trigger and finalizer decisions (CRD CONTROLLER)
  - 📏 **Template size**: ~7,300 lines (growth for test helpers and scope clarity)
- **v2.6** (2025-11-28): 🎯 **PRE-IMPLEMENTATION DESIGN DECISIONS + API PATTERNS**
  - ✅ **Pre-Implementation Design Decisions Section**: Template for documenting ambiguous requirements before Day 1
  - ✅ **API Design Patterns Section**: DD-API-001 (HTTP header vs JSON body pattern)
  - ✅ **ADR/DD Validation Script Updated**: Added DD-API-001 to validation
  - ✅ **Checklist Updated**: Added API design standards checklist item
  - 📏 **Template size**: ~7,100 lines (growth for design decision guidance)
- **v2.5** (2025-11-28): 🎯 **DOCUMENT STRUCTURE FIX + PRE-IMPLEMENTATION CHECKLIST**
  - ✅ **8 Sections Fixed**: Changed from `##` to `###` to nest under respective days
  - ✅ **Error Handling Philosophy**: Now nested under Days 2-6 (Day 6 EOD deliverable)
  - ✅ **Production Runbooks**: Now nested under Day 12
  - ✅ **Edge Case Categories**: Now nested under Days 9-10
  - ✅ **Metrics Validation**: Now nested under Day 7
  - ✅ **Lessons Learned, Technical Debt, Team Handoff**: Now nested under Day 12
  - ✅ **Blockers Section**: Now at `###` level for consistency
  - ✅ **TOC Updated**: Shows proper day-to-template relationship
  - ✅ **NEW: Pre-Implementation ADR/DD Validation Checklist**: Bash script + sign-off for ADR/DD validation
  - 📏 **Impact**: Developers see templates in context, validate docs before Day 1
- **v2.4** (2025-11-28): 🎯 **COMPREHENSIVE ADR/DD REFERENCE INTEGRATION**
  - ✅ **Prerequisites Expanded**: Added DD-004, DD-005, DD-013, DD-014, ADR-015, ADR-032, ADR-034, DD-AUDIT-003
  - ✅ **New Reference Matrix**: Which ADRs/DDs apply to which service type (HTTP, CRD, Audit)
  - ✅ **Universal Standards Section**: DD-004 (RFC 7807), DD-005 (Observability), DD-013 (K8s Client), DD-014 (Version Logging), ADR-015 (Signal naming)
  - ✅ **Audit Standards**: DD-AUDIT-003 (audit requirements), ADR-032 (data access isolation), ADR-034 (audit table design)
  - ✅ **CRD Standards**: ADR-004 (fake K8s client), DD-006 (controller scaffolding), DD-007 (graceful shutdown)
  - ✅ **Testing Standards**: DD-TEST-001 (port allocation), ADR-038 (async audit)
  - 📏 **Template size**: ~6,900 lines (growth for ADR/DD reference matrix)
- **v2.3** (2025-11-28): 🎯 **E2E NODEPORT INFRASTRUCTURE** - Eliminate port-forward instability
  - ✅ **Kind NodePort Pattern**: Complete E2E test setup using NodePort (no kubectl port-forward)
  - ✅ **DD-TEST-001 Reference**: Authoritative port allocation for all services
  - ✅ **Kind Config Template**: `extraPortMappings` configuration for concurrent E2E execution
  - ✅ **Service NodePort Config**: NodePort service YAML patterns
  - ✅ **Test Suite Pattern**: SynchronizedBeforeSuite with NodePort URL (no port-forward)
  - ✅ **Port Allocation Table**: Quick reference for all services (Gateway, Signal Processing, etc.)
  - 📏 **Template size**: ~6,200 lines (growth for E2E infrastructure guidance)
- **v2.2** (2025-11-28): 🎯 **TESTING METHODOLOGY** - Standard order + parallel execution
  - ✅ **Testing Order Alignment**: Standard Unit → Integration → E2E methodology (removed "Integration-First")
  - ✅ **Parallel Test Execution**: **4 concurrent processes** standard for all test tiers
  - ✅ **Parallel Execution Section**: Complete configuration with `go test -p 4` and `ginkgo -procs=4`
  - ✅ **Test Isolation Patterns**: Unique namespace per test for parallel safety
  - ✅ **Parallel Anti-Patterns**: Common mistakes to avoid (hardcoded namespaces, shared state)
  - ✅ **Makefile Targets Updated**: All test targets now include `-p 4` flag
  - ✅ **Quick Reference Updated**: Methodology and parallel execution standards added
  - 📏 **Template size**: ~6,000 lines (minor growth for parallel execution guidance)
- **v2.1** (2025-11-23): 🎯 **ENHANCEMENTS** - Multi-language and deployment pattern support
  - ✅ **Python Service Adaptation** (~200 lines, complete Python patterns)
  - ✅ **Sidecar Deployment Pattern** (~150 lines, Kubernetes examples)
  - ✅ **Shared Library Extraction** (~200 lines, ROI analysis, Redis example)
  - ✅ **Multi-Language Services** (~150 lines, Go+Python structure)
  - ✅ **Enhanced Error Handling Philosophy** (~200 lines, graceful degradation patterns)
  - 📏 **Template size**: ~5,900 lines (16% growth from v2.0 for polyglot support)
- **v2.0** (2025-10-12): 🎯 **MAJOR UPDATE** - Comprehensive production-ready enhancements
  - ✅ **60+ complete code examples** (zero TODO placeholders, V3.0 standard)
  - ✅ **Error Handling Philosophy Template** (280 lines, complete methodology)
  - ✅ **Enhanced BR Coverage Matrix** (calculation methodology, 97%+ target)
  - ✅ **3 Complete EOD Templates** (Days 1, 4, 7 - ~450 lines total)
  - ✅ **CRD Controller Variant Section** (~400 lines, reconciliation patterns)
  - ✅ **Enhanced Prometheus Metrics** (10+ metrics with recording patterns)
  - ✅ **Complete Integration Test Examples** (2-3 tests, ~400 lines)
  - ✅ **Phase 4 Documentation Templates** (Handoff, Production, Confidence)
  - ✅ **Confidence Assessment Methodology** (evidence-based calculation)
  - ✅ **Production-ready code quality** (error handling, logging, metrics in all examples)
  - 📏 **Template size**: ~4,500 lines (3x growth from v1.3 for comprehensive guidance)
- v1.3: Added Integration Test Environment Decision Tree (KIND/envtest/Podman/Mocks)
- v1.2: Added Kind Cluster Test Template for integration tests
- v1.1: Added table-driven testing patterns

---

## 🎯 Quick Reference

**Use this template for**: All Kubernaut stateless services and CRD controllers
**Based on**: Gateway Service + Dynamic Toolset + Notification Controller (proven success)
**Methodology**: APDC-TDD with Defense-in-Depth Testing (Unit → Integration → E2E)
**Parallel Execution**: **4 concurrent processes** for all test tiers (standard)
**Success Rate**:
- Gateway: 95% test coverage, 100% BR coverage, 98% confidence
- Notification: 97.2% BR coverage, 95% test coverage, 98% confidence
**Quality Standard**: V3.0 - Production-ready with comprehensive examples

---

## 📑 **Table of Contents**

| Section | Line | Purpose |
|---------|------|---------|
| [Quick Reference](#-quick-reference) | ~36 | Template overview and success metrics |
| [Naming Convention](#-naming-convention) | ~48 | File naming patterns and anti-patterns |
| [Document Purpose](#document-purpose) | ~175 | Template history and key improvements |
| [Prerequisites Checklist](#prerequisites-checklist) | ~209 | Pre-Day 1 requirements |
| [V1.0 Maturity Checklist](#-v10-mandatory-maturity-checklist--v31-new---scope-by-service-type) | ~378 | Mandatory maturity features ⭐ V3.1 |
| [Cross-Team Validation](#-cross-team-validation--v30-new---scope-common-all-services) | ~520 | Multi-team dependency sign-off ⭐ V3.0 |
| [Integration Test Environment Decision](#-integration-test-environment-decision-v13-) | ~430 | KIND/envtest/Podman/Mocks decision tree |
| [Risk Assessment Matrix](#️-risk-assessment-matrix--v28-new---scope-common-all-services) | ~760 | Risk identification and mitigation |
|    └─ [Risk Mitigation Status Tracking](#risk-mitigation-status-tracking--v30-new) | ~795 | Day-linked risk tracking ⭐ V3.0 |
| [Timeline Overview](#timeline-overview) | ~535 | Phase breakdown (11-12 days) |
| **Day-by-Day Breakdown** | | |
| ├─ [Day 1: Foundation](#day-1-foundation-8h) | ~551 | Types, interfaces, K8s client |
| ├─ [Days 2-6: Core Implementation](#days-2-6-core-implementation-5-days-8h-each) | ~614 | Business logic components |
|    └─ [Error Handling Philosophy](#-error-handling-philosophy-template--v20) | ~894 | Day 6 EOD deliverable |
| ├─ [Day 7: Server + API + Metrics](#day-7-server--api--metrics-8h) | ~1223 | Integration, metrics (10+) |
|    └─ [Metrics Validation Commands](#-metrics-validation-commands-template-day-7) | ~4194 | Day 7 validation |
| ├─ [Day 8: Unit Tests](#day-8-unit-tests-8h) | ~1586 | All component unit tests (parallel: 4 procs) |
| ├─ [Days 9-10: Testing](#day-9-unit-tests-part-2-8h) | ~2476 | Integration + E2E (parallel: 4 procs) |
|    └─ [Edge Case Categories](#-edge-case-categories-template-days-9-10) | ~4162 | Days 9-10 test coverage |
| ├─ [Day 11: Documentation](#day-11-comprehensive-documentation-8h--v20-enhanced) | ~2855 | README, design decisions |
| └─ [Day 12: Production Readiness](#day-12-check-phase--production-readiness--v20-comprehensive-8h) | ~3365 | Checklist, handoff |
|    ├─ [Production Runbooks](#-production-runbooks-template-day-12-deliverable) | ~4095 | Day 12 deliverable |
|    ├─ [Lessons Learned](#-lessons-learned-template-day-12) | ~4241 | Day 12 deliverable |
|    ├─ [Technical Debt](#-technical-debt-template-day-12) | ~4271 | Day 12 deliverable |
|    └─ [Team Handoff Notes](#-team-handoff-notes-template-day-12) | ~4291 | Day 12 deliverable |
| [Critical Checkpoints](#critical-checkpoints-from-gateway-learnings) | ~3718 | 5 gateway learnings |
| [Documentation Standards](#documentation-standards) | ~3747 | Daily status docs, DD format |
| [Testing Strategy](#testing-strategy) | ~3810 | Test distribution, table-driven patterns |
| [Performance Targets](#performance-targets) | ~3991 | Latency, throughput metrics |
| [Common Pitfalls](#common-pitfalls-to-avoid) | ~4008 | Do's and don'ts |
| [Success Criteria](#success-criteria) | ~4032 | Completion checklist |
| [Makefile Targets](#makefile-targets) | ~4056 | Development commands |
| **Appendices** | | |
| ├─ [Appendix A: EOD Templates](#-appendix-a-complete-eod-documentation-templates--v20) | ~4106 | Days 1, 4, 7 templates |
| ├─ [Appendix B: CRD Controller](#-appendix-b-crd-controller-variant--v20) | ~4501 | Reconciliation patterns |
|    └─ [CRD API Group Standard](#-crd-api-group-standard--v30-new) | ~4510 | DD-CRD-001 unified `.ai` domain ⭐ V3.0 |
| └─ [Appendix C: Confidence Assessment](#-appendix-c-confidence-assessment-methodology--v20) | ~4846 | Evidence-based calculation |
| **Language/Deployment Patterns** | | |
| ├─ [Python Service Adaptation](#-python-service-adaptation) | ~5225 | Python-specific patterns |
| ├─ [Sidecar Deployment Pattern](#-sidecar-deployment-pattern) | ~5350 | K8s sidecar examples |
| ├─ [Shared Library Extraction](#-shared-library-extraction) | ~5505 | ROI analysis, Redis example |
| └─ [Multi-Language Services](#-multi-language-services) | ~5671 | Go+Python structure |
| [Enhanced Error Handling](#-enhanced-error-handling-philosophy) | ~5820 | Graceful degradation |
| [Version History](#version-history) | ~5988 | Template changelog |

---

## 📝 **Naming Convention**

**Filename Format**: `[CONTEXT_PREFIX_]IMPLEMENTATION_PLAN[_FEATURE_SUFFIX]_V<semantic_version>.md`

**🚨 CRITICAL**: Version (`_V<semantic_version>`) MUST be the **last suffix** for consistency.

### **Standard Patterns**

**1. Full Service Implementation** (no prefix needed):
- ✅ `IMPLEMENTATION_PLAN_V1.0.md` - Complete service implementation
- ✅ `IMPLEMENTATION_PLAN_V2.0.md` - Major version rewrite

**2. Feature-Specific Implementation** (use descriptive prefix):
- ✅ `AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.3.md` - Specific features
- ✅ `RFC7807_IMPLEMENTATION_PLAN_V2.1.md` - RFC 7807 compliance
- ✅ `GRACEFUL_SHUTDOWN_IMPLEMENTATION_PLAN_V1.0.md` - Graceful shutdown feature

**3. Extension/Enhancement** (use feature suffix, version LAST):
- ✅ `IMPLEMENTATION_PLAN_RFC7807_V2.1.md` - Adding RFC 7807 to existing plan
- ✅ `IMPLEMENTATION_PLAN_PARALLEL_LIMITS_EXTENSION_V1.2.md` - Extending with new feature

**4. E2E or Specialized Tests**:
- ✅ `E2E_TEST_IMPLEMENTATION_PLAN_V1.1.md` - E2E test implementation

### **Anti-Patterns** (Don't Use)
- ❌ `DATA-STORAGE-V1.0-MVP-IMPLEMENTATION-PLAN.md` - Service name in filename (redundant)
- ❌ `IMPLEMENTATION_PLAN_V2.1_RFC7807.md` - Version not last (should be `IMPLEMENTATION_PLAN_RFC7807_V2.1.md`)
- ❌ `IMPLEMENTATION_PLAN.md` - Missing version
- ❌ `IMPLEMENTATION_PLAN_V1.md` - Incomplete version (use V1.0)
- ❌ `MY_FEATURE_PLAN.md` - Non-standard format

### **Semantic Versioning**
- **Major (X.0.0)**: Significant scope changes, architectural shifts
- **Minor (1.X.0)**: Feature additions, timeline extensions
- **Patch (1.0.X)**: Bug fixes, clarifications, template compliance updates

### **Context Prefix Guidelines**

**When to Use Prefix**:
- ✅ Implementing specific features (not full service)
- ✅ Multiple implementation plans for same service
- ✅ Feature-specific extensions or enhancements
- ✅ Need to distinguish from main implementation plan
- ✅ **Splitting large plans into manageable standalone documents**

**When to Split Plans** (Important Pattern):
When an implementation plan grows too large (>3,000 lines), split it into:
1. **Main Plan**: Core service implementation (`IMPLEMENTATION_PLAN_V1.0.md`)
2. **Feature Extensions**: Standalone plans with context prefixes

**🚨 CRITICAL: Cross-Referencing Requirement**:
- **Main plan MUST reference all feature plans** in "Related Documents" section
- **Feature plans MUST reference main plan** in their metadata
- **Use explicit links** to ensure traceability
- **Update cross-references** when plans are added/removed

**Benefits of Splitting**:
- ✅ Easier to navigate and maintain
- ✅ Clear separation of concerns
- ✅ Independent versioning for features
- ✅ Parallel development possible
- ✅ Reduces cognitive load

**Prefix Format**:
- Use `UPPERCASE_WITH_UNDERSCORES`
- Be descriptive but concise (2-4 words max)
- Focus on WHAT is being implemented, not service name

**Examples**:

**Pattern 1: Feature-Specific Plans** (Splitting large plans)
```
Service: Data Storage
├── IMPLEMENTATION_PLAN_V1.0.md                               ← Main service (foundation)
├── AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.3.md  ← Standalone feature
├── PLAYBOOK_CRUD_API_IMPLEMENTATION_PLAN_V1.1.md            ← Standalone feature
└── CACHE_OPTIMIZATION_IMPLEMENTATION_PLAN_V1.0.md           ← Standalone feature

Main plan (IMPLEMENTATION_PLAN_V1.0.md) MUST include:
  ## Related Documents
  - [Audit Trace & Semantic Search](./AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.3.md)
  - [Playbook CRUD API](./PLAYBOOK_CRUD_API_IMPLEMENTATION_PLAN_V1.1.md)
  - [Cache Optimization](./CACHE_OPTIMIZATION_IMPLEMENTATION_PLAN_V1.0.md)

Feature plan (AUDIT_TRACE_SEMANTIC_SEARCH_IMPLEMENTATION_PLAN_V1.3.md) MUST include:
  **Parent Plan**: [Data Storage V1.0](./IMPLEMENTATION_PLAN_V1.0.md)
  **Scope**: Audit trail persistence and playbook semantic search (subset of V1.0)
```

**Pattern 2: Extension Plans** (Adding to existing)
```
Service: HolmesGPT API
├── IMPLEMENTATION_PLAN_V3.0.md                               ← Full service
└── IMPLEMENTATION_PLAN_RFC7807_GRACEFUL_SHUTDOWN_V3.1.md    ← Extension (adds features)

Extension plan (IMPLEMENTATION_PLAN_RFC7807_GRACEFUL_SHUTDOWN_V3.1.md) MUST include:
  **Extends**: [HolmesGPT API V3.0](./IMPLEMENTATION_PLAN_V3.0.md)
  **Scope**: Adds RFC 7807 error handling and graceful shutdown to existing service
```

**Pattern 3: Incremental Development** (Gateway Service example)
```
Service: Gateway
├── IMPLEMENTATION_PLAN_V2.24.md                              ← Superseded
├── IMPLEMENTATION_PLAN_V2.25.md                              ← Superseded
├── IMPLEMENTATION_PLAN_V2.26.md                              ← Superseded
└── IMPLEMENTATION_PLAN_V2.27.md                              ← Current (references previous)

Each plan includes:
  **Supersedes**: [Gateway V2.26](./IMPLEMENTATION_PLAN_V2.26.md)
  **Changelog**: Lists what changed from previous version
```

### **Archived Plans**
Move superseded versions to `implementation/archive/` directory

### **Rationale**
Consistent naming enables:
- Easy version identification across all services
- Clear context about what's being implemented
- Automated tooling and scripts
- Clear historical tracking
- Distinguishing between full service and feature implementations
- Standard file organization

---

## Document Purpose

This template incorporates lessons learned from:
1. **Gateway Service**: Production-ready (21/22 tests, 95% coverage, 98% confidence)
2. **Dynamic Toolset Service**: Enhanced with additional best practices
3. **Notification Controller**: CRD controller standard (97.2% BR coverage, 98% confidence)
4. **Data Storage Service**: Comprehensive v4.1 standard (complete error handling, metrics)

**🎯 V2.0 Enhancement Highlights** (Major Update):
- **60+ Complete Code Examples**: Zero TODO placeholders, production-ready quality
- **Error Handling Philosophy**: 280-line methodology template included
- **Enhanced BR Coverage Matrix**: Calculation methodology, 97%+ target
- **3 Complete EOD Templates**: Days 1, 4, 7 with checklists and confidence assessments
- **CRD Controller Variant**: 400-line section with reconciliation patterns
- **Enhanced Prometheus Metrics**: 10+ metrics with recording patterns and testing
- **Complete Integration Tests**: 2-3 full examples (~400 lines) from proven implementations
- **Phase 4 Templates**: Handoff Summary, Production Readiness, Confidence Assessment
- **Evidence-Based Confidence**: Calculation methodology with formula

**Key Improvements Over Ad-Hoc Planning** (Retained from v1.x):
- Integration-first testing (catches issues 2 days earlier)
- Schema validation before testing (prevents test failures)
- Daily progress tracking (EOD documentation templates) ⭐ **v2.0 ENHANCED**
- BR coverage matrix (calculation methodology included) ⭐ **v2.0 ENHANCED**
- Production readiness checklist (comprehensive templates) ⭐ **v2.0 ENHANCED**
- File organization strategy (cleaner git history)
- Table-driven testing pattern (25-40% less test code)
- Kind cluster test template (15 lines vs 80+)
- Integration test decision tree (KIND/envtest/Podman/Mocks)
- Error handling philosophy (complete template) ⭐ **v2.0 NEW**
- CRD controller patterns (reconciliation, status updates) ⭐ **v2.0 NEW**
- HTTP header vs JSON body pattern (DD-API-001) ⭐ **v2.6 NEW**
- Pre-implementation design decisions section ⭐ **v2.6 NEW**
- OpenAPI pre-phase step for stateless services ⭐ **v2.7 NEW**
- E2E test helper patterns (STATELESS + CRD CONTROLLER) ⭐ **v2.7 NEW**
- Scope annotations (COMMON, STATELESS, CRD CONTROLLER) ⭐ **v2.7 NEW**

---

## Prerequisites Checklist

Before starting Day 1, ensure:
- [ ] Service specifications complete (overview, API spec, implementation docs)
- [ ] Business requirements documented (BR-[CATEGORY]-XXX format)
- [ ] Architecture decisions approved:
  - **Universal Standards (ALL services)**:
    - [ ] DD-004: RFC 7807 Error Responses (**MANDATORY** for HTTP APIs)
    - [ ] DD-005: Observability Standards (**MANDATORY** - metrics/logging)
    - [ ] DD-007: Kubernetes-Aware Graceful Shutdown (**MANDATORY**)
    - [ ] DD-014: Binary Version Logging (**MANDATORY** - production troubleshooting)
    - [ ] ADR-015: Alert-to-Signal Naming Migration (**MANDATORY** - use "Signal" terminology)
  - **K8s-Aware Services**:
    - [ ] DD-013: K8s Client Initialization Standard (shared `pkg/k8sutil`)
  - **CRD Controllers**:
    - [ ] DD-006: Controller Scaffolding (templates and patterns)
    - [ ] ADR-004: Fake K8s Client (**MANDATORY** for unit tests)
  - **Audit-Required Services** (check DD-AUDIT-003):
    - [ ] DD-AUDIT-003: Service Audit Trace Requirements (determines if audit needed)
    - [ ] ADR-032: Data Access Layer Isolation (**MANDATORY** - use Data Storage API)
    - [ ] ADR-034: Unified Audit Table Design (audit schema)
    - [ ] ADR-038: Async Buffered Audit Ingestion (fire-and-forget pattern)
  - **Testing**:
    - [ ] DD-TEST-001: Port Allocation Strategy (**MANDATORY** for E2E tests)
  - [ ] Service-specific DDs documented
- [ ] Dependencies identified
- [ ] Success criteria defined
- [ ] **Integration test environment determined** (see decision tree below)
- [ ] **Required test infrastructure available** (KIND/envtest/Podman/none)
- [ ] **E2E NodePort allocation reserved** (see [DD-TEST-001](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))
- [ ] **V2.0 Template sections reviewed**: ⭐ **NEW**
  - [ ] Error Handling Philosophy Template (Section after Day 6)
  - [ ] BR Coverage Matrix Methodology (Day 9 Enhanced)
  - [ ] EOD Documentation Templates (Appendix A)
  - [ ] CRD Controller Variant (Appendix B, if applicable)
  - [ ] Complete Integration Test Examples (Day 8 Enhanced)
  - [ ] Phase 4 Documentation Templates (Days 10-12 Enhanced)
  - [ ] Confidence Assessment Methodology (Day 12 Enhanced)
- [ ] **Cross-team dependencies validated** (see Cross-Team Validation section below) ⭐ V3.0 NEW
- [ ] **V1.0 Maturity Requirements validated** (see V1.0 Mandatory Maturity Checklist below) ⭐ V3.1 NEW

---

## ✅ **V1.0 Mandatory Maturity Checklist** ⭐ V3.1 NEW - **SCOPE: BY SERVICE TYPE**

**Purpose**: Ensure all services meet V1.0 production-readiness standards.

**Reference**: [V1_0_SERVICE_MATURITY_TRIAGE_DEC_19_2025.md](../handoff/V1_0_SERVICE_MATURITY_TRIAGE_DEC_19_2025.md)
**Testing Requirements**: [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)
**Test Plan Template**: [V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)

> ⚠️ **MANDATORY**: A service is NOT considered V1.0 ready until ALL applicable checkboxes are ✅.
> This checklist is a **living document** - triage new ADRs/DDs for additional requirements.

---

### 📋 **CRD Controller Maturity Checklist**

**Applies to**: SignalProcessing, WorkflowExecution, AIAnalysis, Notification, RemediationOrchestrator

#### Core Infrastructure (P0 - Blockers)

| Feature | Requirement | Test Requirement |
|---------|-------------|------------------|
| **Metrics wired to controller** | Controller struct has `Metrics` field | Integration: Verify metric values after operations |
| **Metrics registered with CR** | Uses `metrics.Registry.MustRegister()` in `init()` | E2E: Verify `/metrics` endpoint accessible |
| **EventRecorder** | Controller struct has `Recorder record.EventRecorder` | E2E: Verify events emitted via `kubectl describe` |
| **Graceful shutdown (DD-007)** | Flushes audit/state before exit | Integration: Verify flush called on SIGTERM |
| **Audit integration (DD-AUDIT-003)** | Uses `pkg/audit` store | Integration: Verify all audit traces via OpenAPI client |

#### Observability (P1 - High Priority)

| Feature | Requirement | Test Requirement |
|---------|-------------|------------------|
| **Predicates (event filtering)** | Uses `predicate.GenerationChangedPredicate{}` | Unit: Verify predicate applied |
| **Logger field** | Struct has `Log logr.Logger` | N/A (code review) |
| **Healthz probes** | Manager exposes healthz/readyz | E2E: Verify probe endpoints |
| **Config validation (ADR-030)** | Config validated at startup | Unit: Verify validation errors |

#### Implementation Checklist

```markdown
## V1.0 Maturity Checklist - [Service Name]

### P0 - Blockers (MUST pass before V1.0)
- [ ] **Metrics wired to controller**
  - [ ] `Metrics *metrics.Metrics` field in reconciler struct
  - [ ] Metrics recorded in reconciliation phases
  - [ ] Integration test: Verify metric values after reconciliation
- [ ] **Metrics registered with controller-runtime**
  - [ ] `metrics.Registry.MustRegister()` in `init()` function
  - [ ] E2E test: Verify `/metrics` endpoint returns expected metrics
- [ ] **EventRecorder**
  - [ ] `Recorder record.EventRecorder` field in reconciler struct
  - [ ] `mgr.GetEventRecorderFor("service-controller")` in main.go
  - [ ] Events emitted on phase transitions and errors
  - [ ] E2E test: Verify `kubectl describe <crd>` shows events
- [ ] **Graceful shutdown (DD-007)**
  - [ ] Audit store `.Close()` called before exit
  - [ ] Integration test: Verify flush on SIGTERM
- [ ] **Audit integration (DD-AUDIT-003)**
  - [ ] All required audit traces emitted
  - [ ] Integration test: Each trace verified via OpenAPI audit client
  - [ ] E2E test: Audit client wired to main controller

### P1 - High Priority (SHOULD pass before V1.0)
- [ ] **Predicates**
  - [ ] `WithEventFilter(predicate.GenerationChangedPredicate{})` in SetupWithManager
  - [ ] Unit test: Verify predicate filtering
- [ ] **Logger field**
  - [ ] `Log logr.Logger` field in reconciler struct (code review)
- [ ] **Healthz probes**
  - [ ] Manager exposes `/healthz` and `/readyz`
  - [ ] E2E test: Verify probe endpoints return 200

### P2 - Medium Priority (CAN defer to V1.1)
- [ ] **Config validation (ADR-030)**
  - [ ] Config validated at startup with clear error messages
  - [ ] Unit test: Verify validation errors for invalid config
```

---

### 📋 **Stateless Service (HTTP API) Maturity Checklist**

**Applies to**: Gateway, DataStorage, HolmesGPT-API

#### Core Infrastructure (P0 - Blockers)

| Feature | Requirement | Test Requirement |
|---------|-------------|------------------|
| **Prometheus metrics** | `/metrics` endpoint exposed | Integration: Verify metric values |
| **Health endpoints** | `/health` or `/healthz` endpoint | E2E: Verify 200 response |
| **Graceful shutdown (DD-007)** | Handles SIGTERM, flushes state | Integration: Verify shutdown sequence |
| **RFC 7807 errors (DD-004)** | All errors use RFC 7807 format | Integration: Verify error response format |
| **Audit integration (if required)** | Uses `pkg/audit` or Python equivalent | Integration: Verify traces via OpenAPI |

#### Observability (P1 - High Priority)

| Feature | Requirement | Test Requirement |
|---------|-------------|------------------|
| **Request logging** | All requests logged with correlation ID | Integration: Verify log output |
| **OpenAPI spec** | Spec matches implementation | Integration: Contract testing |
| **Config validation (ADR-030)** | Config validated at startup | Unit: Verify validation errors |

#### Implementation Checklist

```markdown
## V1.0 Maturity Checklist - [Service Name]

### P0 - Blockers (MUST pass before V1.0)
- [ ] **Prometheus metrics**
  - [ ] `/metrics` endpoint exposed
  - [ ] Business metrics registered (request count, latency, etc.)
  - [ ] Integration test: Verify metric values after operations
  - [ ] E2E test: Verify `/metrics` endpoint accessible
- [ ] **Health endpoints**
  - [ ] `/health` or `/healthz` endpoint implemented
  - [ ] E2E test: Verify 200 response
- [ ] **Graceful shutdown (DD-007)**
  - [ ] SIGTERM handler implemented
  - [ ] Connections drained before exit
  - [ ] Integration test: Verify shutdown sequence
- [ ] **RFC 7807 errors (DD-004)**
  - [ ] All error responses use RFC 7807 format
  - [ ] Integration test: Verify error response structure
- [ ] **Audit integration (if applicable)**
  - [ ] All required audit traces emitted
  - [ ] Integration test: Each trace verified via OpenAPI audit client

### P1 - High Priority (SHOULD pass before V1.0)
- [ ] **Request logging**
  - [ ] All requests logged with method, path, status, duration
  - [ ] Correlation ID propagated
- [ ] **OpenAPI spec**
  - [ ] Spec matches implementation
  - [ ] Integration test: Contract validation
- [ ] **Config validation (ADR-030)**
  - [ ] Config validated at startup
  - [ ] Unit test: Verify validation errors
```

---

### 🔄 **Living Document Notice**

> **This checklist is a living document.** When new ADRs or DDs are created that affect service maturity requirements:
>
> 1. **Triage** the ADR/DD for impact on this checklist
> 2. **Add** new requirements to the appropriate section
> 3. **Update** the version and changelog
> 4. **Notify** all service teams via handoff document
>
> **Last Updated**: 2025-12-19 (V3.1)
> **Next Review**: When any new ADR/DD is created

---

## 🤝 **Cross-Team Validation** ⭐ V3.0 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Formally validate all cross-team dependencies before starting implementation.

### **When to Use This Section**

Use cross-team validation when your service:
- Consumes data from another service (e.g., CRD status fields)
- Produces data consumed by another service (e.g., API responses)
- Has shared type definitions (e.g., `EnrichmentResults`)
- Requires coordination on naming conventions, field paths, or schemas

### **Cross-Team Validation Status Template**

> **Validation Status**: 📋 DRAFT | ⏳ IN REVIEW | ✅ VALIDATED
>
> | Team | Validation Topic | Status | Record |
> |------|-----------------|--------|--------|
> | [Team Name] | [What needs validation] | ⬜ Pending | - |
> | [Team Name] | [What needs validation] | ⏳ In Review | [HANDOFF_REQUEST_*.md] |
> | [Team Name] | [What needs validation] | ✅ Complete | [RESPONSE_*.md] |

### **HANDOFF/RESPONSE Pattern**

For cross-team validations, use the HANDOFF/RESPONSE document pattern:

**File Naming Convention**:
- `HANDOFF_REQUEST_[TOPIC].md` - Request sent to another team
- `RESPONSE_[TOPIC].md` - Response received from that team

**Validation Workflow**:
1. **Create** `HANDOFF_REQUEST_*.md` with questions, proposals, or validation needs
2. **Send** to dependent team for review
3. **Team responds** in `RESPONSE_*.md` (or updates the handoff inline)
4. **Update status** from ⬜ Pending → ✅ Complete when confirmed
5. **Plan status** changes to ✅ VALIDATED when ALL dependencies are complete

**Example**:
```
Service: Signal Processing
├── HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md    ← Sent to AI Analysis team
├── HANDOFF_REQUEST_GATEWAY_LABEL_PASSTHROUGH.md ← Sent to Gateway team
├── RESPONSE_CUSTOM_LABELS_VALIDATION.md        ← From HolmesGPT-API team
└── RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md       ← From Gateway team
```

### **Pre-Implementation Validation Gate**

> **🚨 BLOCKING REQUIREMENT**: Do NOT start Day 1 until ALL cross-team validations are ✅ Complete.

**Validation Checklist**:
- [ ] All upstream data contracts validated (data you CONSUME)
- [ ] All downstream data contracts validated (data you PRODUCE)
- [ ] Shared type definitions aligned across teams
- [ ] Naming conventions agreed (snake_case vs camelCase, kebab-case for K8s)
- [ ] Field paths confirmed (e.g., `spec.analysisRequest.signalContext.enrichmentResults`)
- [ ] Integration points documented with examples

**Confidence Impact**:
- Without validation: Maximum 85% confidence (integration risk)
- With validation: 100% confidence achievable (contracts verified)

### 📋 **Pre-Implementation ADR/DD Validation Checklist** ⭐ V2.5 NEW

> **MANDATORY**: Before starting Day 1, validate ALL referenced ADRs/DDs exist and have been read.

**Run this validation script to confirm all referenced documents exist**:

```bash
#!/bin/bash
# Pre-Implementation ADR/DD Validation
# Run from repository root before starting implementation

echo "🔍 Validating ADR/DD references..."

# Universal Standards (ALL services)
UNIVERSAL=(
  "DD-004-RFC7807-ERROR-RESPONSES.md"
  "DD-005-OBSERVABILITY-STANDARDS.md"
  "DD-007-kubernetes-aware-graceful-shutdown.md"
  "DD-014-binary-version-logging-standard.md"
  "ADR-015-alert-to-signal-naming-migration.md"
)

# CRD Controller Standards (if CRD controller)
CRD_CONTROLLER=(
  "DD-006-controller-scaffolding-strategy.md"
  "DD-013-kubernetes-client-initialization-standard.md"
  "DD-CRD-001-api-group-domain-selection.md"
  "ADR-004-fake-kubernetes-client.md"
)

# Audit Standards (if P0/P1 audit service per DD-AUDIT-003)
AUDIT=(
  "DD-AUDIT-003-service-audit-trace-requirements.md"
  "ADR-032-data-access-layer-isolation.md"
  "ADR-034-unified-audit-table-design.md"
  "ADR-038-async-buffered-audit-ingestion.md"
)

# API Design Standards (ALL HTTP services)
API_DESIGN=(
  "DD-API-001-http-header-vs-json-body-pattern.md"
)

# Testing Standards (ALL services)
TESTING=(
  "DD-TEST-001-port-allocation-strategy.md"
)

ERRORS=0
for doc in "${UNIVERSAL[@]}" "${CRD_CONTROLLER[@]}" "${AUDIT[@]}" "${API_DESIGN[@]}" "${TESTING[@]}"; do
  if [ -f "docs/architecture/decisions/$doc" ]; then
    echo "✅ $doc"
  else
    echo "❌ MISSING: $doc"
    ERRORS=$((ERRORS + 1))
  fi
done

if [ $ERRORS -gt 0 ]; then
  echo ""
  echo "❌ $ERRORS documents missing. Fix before starting implementation."
  exit 1
else
  echo ""
  echo "✅ All ADR/DD references validated. Ready to start Day 1."
fi
```

**Checklist (mark after reading each document)**:

- [ ] **Universal Standards READ**:
  - [ ] DD-004: Understood RFC 7807 error format
  - [ ] DD-005: Understood metrics naming and logging format
  - [ ] DD-007: Understood 4-step graceful shutdown
  - [ ] DD-014: Understood version logging requirements
  - [ ] ADR-015: Understood "Signal" terminology mandate

- [ ] **CRD Controller Standards READ** (if CRD controller):
  - [ ] DD-006: Reviewed controller scaffolding templates
  - [ ] DD-013: Reviewed K8s client initialization pattern
  - [ ] ADR-004: Understood fake client mandate for unit tests

- [ ] **Audit Standards READ** (if audit-required):
  - [ ] DD-AUDIT-003: Confirmed service audit tier (P0/P1/P2/none)
  - [ ] ADR-032: Understood Data Storage API requirement
  - [ ] ADR-034: Reviewed audit event schema
  - [ ] ADR-038: Understood fire-and-forget pattern

- [ ] **API Design Standards READ** (if HTTP service):
  - [ ] DD-API-001: Understood HTTP header vs JSON body pattern
    - Business data → JSON body (audit trail, proxy safety)
    - Infrastructure data → HTTP headers (X-Request-ID, X-Correlation-ID)

- [ ] **Testing Standards READ**:
  - [ ] DD-TEST-001: Reviewed port allocation for this service

**Sign-off**:
```
I have read and understood all applicable ADRs/DDs for this service.
Developer: _________________ Date: ___________
```

---

## 📝 Logging Framework Decision Matrix (DD-005 v2.0) ⭐ V2.8 NEW

**Authority**: [DD-005-OBSERVABILITY-STANDARDS.md](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) v2.0

### Unified Logging Interface

**MANDATORY**: All Kubernaut services use `logr.Logger` as the unified logging interface.

| Service Type | Primary Logger | How to Create | Shared Library Interface |
|--------------|----------------|---------------|--------------------------|
| **Stateless HTTP Services** (Gateway, Data Storage, Context API) | `logr.Logger` | `zapr.NewLogger(zapLogger)` | `logr.Logger` |
| **CRD Controllers** (Signal Processing, Notification, Workflow Execution) | `logr.Logger` | `ctrl.Log.WithName("component")` | `logr.Logger` |
| **Shared Libraries** (`pkg/*`) | N/A (accepts) | Passed by caller | `logr.Logger` |

### Implementation Patterns

#### Stateless HTTP Services

```go
import (
    "github.com/go-logr/logr"
    "github.com/go-logr/zapr"
    "go.uber.org/zap"
)

func main() {
    // Create zap logger (for performance)
    zapLogger, _ := zap.NewProduction()
    defer zapLogger.Sync()

    // Convert to logr interface (for consistency)
    logger := zapr.NewLogger(zapLogger)

    // Pass to shared libraries
    auditStore, _ := audit.NewBufferedStore(client, config, "gateway", logger.WithName("audit"))
    server := gateway.NewServer(cfg, logger)
}
```

#### CRD Controllers

```go
import (
    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    // Use native logr from controller-runtime (no adapter needed)
    logger := ctrl.Log.WithName("notification-controller")

    // Pass to shared libraries
    auditStore, _ := audit.NewBufferedStore(client, config, "notification", logger.WithName("audit"))
}
```

### Logging Syntax (logr)

```go
// INFO level (V=0, always shown)
logger.Info("Signal received", "source", "prometheus", "fingerprint", fp)

// DEBUG level (V=1, shown when verbosity >= 1)
logger.V(1).Info("Parsing signal payload", "fingerprint", fp)

// ERROR level (error as first argument)
logger.Error(err, "Failed to create CRD", "request_id", requestID)

// Named sub-logger
auditLogger := logger.WithName("audit")
```

### ❌ FORBIDDEN Patterns

```go
// ❌ WRONG: Using *zap.Logger directly in shared libraries
func NewBufferedStore(..., logger *zap.Logger) // FORBIDDEN

// ❌ WRONG: Using zap.String() helpers with logr
logger.Info("Message", zap.String("key", "value")) // FORBIDDEN

// ❌ WRONG: Creating separate zap logger in CRD controllers
zapLogger, _ := zap.NewProduction() // FORBIDDEN in CRD controllers
```

---

## 🔍 Integration Test Environment Decision (v1.3) ⭐ NEW

**CRITICAL**: Determine your integration test environment **before Day 1** using this decision tree.

### Decision Tree

```
Does your service WRITE to Kubernetes (create/modify CRDs or resources)?
├─ YES → Does it need RBAC or TokenReview API?
│        ├─ YES → Use KIND (full K8s cluster)
│        └─ NO → Use ENVTEST (API server only)
│
└─ NO → Does it READ from Kubernetes?
         ├─ YES → Need field selectors or CRDs?
         │        ├─ YES → Use ENVTEST
         │        └─ NO → Use FAKE CLIENT
         │
         └─ NO → Use PODMAN (external services only)
                 or HTTP MOCKS (if no external deps)
```

### Classification Guide

#### 🔴 KIND Required
**Use When**:
- Writes CRDs or Kubernetes resources
- Needs RBAC enforcement
- Uses TokenReview API for authentication
- Requires ServiceAccount permissions testing

**Examples**: Gateway Service, Dynamic Toolset Service (V2)

**Prerequisites**:
- [ ] KIND cluster available (`make bootstrap-dev`)
- [ ] Kind template documentation reviewed ([KIND_CLUSTER_TEST_TEMPLATE.md](../testing/KIND_CLUSTER_TEST_TEMPLATE.md))

---

#### 🟡 ENVTEST Required
**Use When**:
- Reads from Kubernetes (logs, events, resources)
- Needs field selectors (e.g., `.spec.nodeName=worker`)
- Writes ConfigMaps/Services (but no RBAC needed)
- Testing with CRDs (no RBAC validation)

**Examples**: Dynamic Toolset Service (V1), HolmesGPT API Service

**Prerequisites**:
- [ ] `setup-envtest` installed (`go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest`)
- [ ] Binaries downloaded (`setup-envtest use 1.31.0`)

---

#### 🟢 PODMAN Required
**Use When**:
- No Kubernetes operations
- Needs PostgreSQL, Redis, or other databases
- External service dependencies

**Examples**: Data Storage Service, Context API Service

**Prerequisites**:
- [ ] Docker/Podman available
- [ ] testcontainers-go configured

---

#### ⚪ HTTP MOCKS Only
**Use When**:
- No Kubernetes operations
- No database dependencies
- Only HTTP API calls to other services

**Examples**: Effectiveness Monitor Service, Notification Service

**Prerequisites**:
- [ ] None (uses Go stdlib `net/http/httptest`)

---

### Quick Classification Examples

| Service Type | Kubernetes Ops | Databases | Test Env |
|--------------|---------------|-----------|----------|
| Writes CRDs + RBAC | ✅ Write + RBAC | ❌ | 🔴 KIND |
| Writes ConfigMaps only | ✅ Write (no RBAC) | ❌ | 🟡 ENVTEST |
| Reads K8s (field selectors) | ✅ Read (complex) | ❌ | 🟡 ENVTEST |
| Reads K8s (simple) | ✅ Read (simple) | ❌ | Fake Client |
| HTTP API + PostgreSQL | ❌ | ✅ | 🟢 PODMAN |
| HTTP API only | ❌ | ❌ | ⚪ HTTP MOCKS |

### Update Your Plan

Once determined, update all instances of `[TEST_ENVIRONMENT]` in this plan with your choice:
- Replace `[TEST_ENVIRONMENT]` with: **KIND** | **ENVTEST** | **PODMAN** | **HTTP_MOCKS**
- Update prerequisites checklist above
- Review setup requirements in Day 8 (Integration Test Setup)

**Reference Documentation**:
- [Integration Test Environment Decision Tree](../../testing/INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)
- [Stateless Services Integration Test Strategy](../stateless/INTEGRATION_TEST_STRATEGY.md)
- [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

---

## Timeline Overview

| Phase | Days | Focus | Key Deliverables |
|-------|------|-------|------------------|
| **Foundation** | 1 | Types, interfaces, K8s client | Package structure, interfaces |
| **Core Logic** | 2-6 | Business logic components | All components implemented |
| **Integration** | 7 | Server, API, metrics | Complete service |
| **Testing** | 8-10 | Integration + Unit tests | 70%+ coverage |
| **Finalization** | 11-12 | E2E, docs, production readiness | Ready for deployment |

**Total**: 11-12 days (with buffer)

---

## 🎯 **Pre-Implementation Design Decisions** ⭐ V2.6 NEW

**BLOCKING**: Before starting Day 1, document decisions for ambiguous requirements.

### **Purpose**
Prevent implementation delays by resolving design ambiguities upfront. This section captures decisions that would otherwise cause mid-implementation confusion.

### **When to Use**
- API behavior has multiple valid interpretations
- Immutability/mutability constraints are unclear
- Delete behavior (hard vs soft) is not specified
- Timing behavior (sync vs async) is not specified
- External system constraints affect design

### **Template**

```markdown
## 🎯 **Approved Design Decisions** - [Date]

### **DD-1: [Decision Name]**

| Question | [The ambiguous question] |
|----------|--------------------------|
| **Decision** | **Option [X]**: [Chosen option] |
| **Rationale** | [Why this option was chosen] |
| **Implementation** | [How this affects implementation] |

### **DD-2: [Decision Name]**
...
```

### **Example: Workflow CRUD Decisions** - **SCOPE: STATELESS HTTP SERVICES**

```markdown
### **DD-1: Workflow Immutability (PUT Behavior)**

| Question | Should PUT /api/v1/workflows/{id} update existing versions? |
|----------|-------------------------------------------------------------|
| **Decision** | **Option C**: PUT is NOT allowed. Immutability enforced. |
| **Rationale** | DD-WORKFLOW-012 mandates immutability. Updates = new version via POST. |
| **Implementation** | PUT returns 405 Method Not Allowed. |

### **DD-2: Delete Behavior**

| Question | Should DELETE remove workflows or disable them? |
|----------|------------------------------------------------|
| **Decision** | **Option C**: Use disabled_at mechanism (preserve audit trail). |
| **Rationale** | Preserves audit trail, aligns with immutability. |
| **Implementation** | DELETE sets disabled_at=NOW(), disabled_by, disabled_reason. |

### **DD-3: Expensive Operation Timing (Sync vs Async)**

| Question | Should [expensive operation, e.g., embedding generation] be synchronous or asynchronous? |
|----------|------------------------------------------------------------------------------------------|
| **Decision** | **Option A**: Synchronous for V1.0 (accept ~2-3s latency). |
| **Rationale** | Correctness over performance. Async introduces race conditions where newly created resources aren't immediately available. |
| **Implementation** | POST blocks until operation completes. Async deferred to V1.1 if needed. |
```

### **Example: CRD Controller Decisions** - **SCOPE: CRD CONTROLLERS**

```markdown
### **DD-1: Reconciliation Trigger Strategy**

| Question | Should reconciliation be triggered by all field changes or specific fields only? |
|----------|----------------------------------------------------------------------------------|
| **Decision** | **Option B**: Specific fields only (spec changes, not status). |
| **Rationale** | Prevents reconciliation loops from status updates. |
| **Implementation** | Use `GenerationChangedPredicate` in controller builder. |

### **DD-2: Finalizer Strategy**

| Question | Should the controller use finalizers for cleanup? |
|----------|--------------------------------------------------|
| **Decision** | **Option A**: Yes, use finalizers for external resource cleanup. |
| **Rationale** | Ensures external resources (e.g., Redis keys) are cleaned up on CRD deletion. |
| **Implementation** | Add finalizer in reconcile, remove after cleanup in delete handler. |
```

### **Pre-Implementation Checklist** - **SCOPE: COMMON (ALL SERVICES)**

Before starting Day 1:
- [ ] All ambiguous requirements have documented decisions
- [ ] Each decision has clear rationale
- [ ] Implementation impact is documented
- [ ] Decisions are approved by stakeholder

---

## ⚠️ **Risk Assessment Matrix** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Identify and mitigate risks before implementation begins.

### **Risk Assessment Template**

| Risk | Probability | Impact | Mitigation | Owner |
|------|-------------|--------|------------|-------|
| [Risk 1: e.g., External API dependency unavailable] | Medium | High | Circuit breaker + graceful degradation | Dev |
| [Risk 2: e.g., Database schema migration failure] | Low | Critical | Rollback script + staging validation | Dev |
| [Risk 3: e.g., Performance regression] | Medium | Medium | Benchmark tests + performance budget | Dev |

### **Risk Categories**

| Category | Examples | Standard Mitigations |
|----------|----------|---------------------|
| **External Dependencies** | API unavailability, rate limiting | Circuit breaker, retry, graceful degradation |
| **Data Integrity** | Schema migration, data corruption | Rollback scripts, staging validation, backups |
| **Performance** | Latency regression, memory leaks | Benchmarks, load tests, performance budgets |
| **Security** | Auth bypass, data exposure | Security review, penetration testing |
| **Operational** | Deployment failure, config drift | Rollback plan, canary deployment |

### **Risk Severity Matrix**

| Probability ↓ / Impact → | Low | Medium | High | Critical |
|---------------------------|-----|--------|------|----------|
| **High** | Monitor | Mitigate | Mitigate | Block |
| **Medium** | Accept | Monitor | Mitigate | Mitigate |
| **Low** | Accept | Accept | Monitor | Mitigate |

**Actions**:
- **Block**: Cannot proceed until risk is eliminated
- **Mitigate**: Must have mitigation plan before proceeding
- **Monitor**: Proceed with monitoring plan
- **Accept**: Proceed with documented acceptance

### **Risk Mitigation Status Tracking** ⭐ V3.0 NEW

**Purpose**: Link identified risks to specific implementation days and track mitigation status.

| Risk # | Action Required | Day | Status |
|--------|-----------------|-----|--------|
| 1 | [Specific mitigation implementation] | Day X | ⬜ Pending |
| 2 | [Specific mitigation implementation] | Day Y | ⬜ Pending |
| 3 | [Specific mitigation implementation] | Day Z | ⬜ Pending |

**Status Legend**:
- ⬜ Pending: Not yet implemented
- 🔄 In Progress: Currently being addressed
- ✅ Complete: Mitigation implemented and tested
- ❌ Blocked: Cannot proceed (escalate)

**Example** (from Signal Processing):
| Risk # | Action Required | Day | Status |
|--------|-----------------|-----|--------|
| 1 | Implement `buildDegradedContext()` for K8s API failures | Day 3 | ⬜ Pending |
| 2 | Add Rego timeout (100ms) and fallback matrix | Day 5 | ⬜ Pending |
| 3 | Use `audit.NewBufferedStore()` with retry | Day 8 | ⬜ Pending |

**Validation**: Update status to ✅ Complete when mitigation is implemented AND tested.

---

## 📋 **Files Affected Section** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Document all files that will be created, modified, or deleted during implementation.

### **Files Affected Template**

#### **New Files** (to be created)
| File | Purpose | Day |
|------|---------|-----|
| `pkg/[service]/types.go` | Core types and interfaces | Day 1 |
| `pkg/[service]/[component].go` | [Component] implementation | Day 2-3 |
| `test/unit/[service]/[component]_test.go` | Unit tests | Day 8 |
| `test/integration/[service]/[feature]_test.go` | Integration tests | Day 9 |
| `test/e2e/[service]/[workflow]_test.go` | E2E tests | Day 10 |

#### **Modified Files** (existing files to update)
| File | Changes | Day |
|------|---------|-----|
| `cmd/[service]/main.go` | Add new component initialization | Day 7 |
| `pkg/[service]/server.go` | Register new handlers | Day 7 |
| `config/[service]/config.yaml` | Add new configuration options | Day 1 |

#### **Deleted Files** (obsolete files to remove)
| File | Reason | Day |
|------|--------|-----|
| `pkg/[service]/deprecated_*.go` | Replaced by new implementation | Day 6 |

**Validation**: Run `git status` at end of each day to verify file changes match plan.

---

## 🔄 **Enhancement Application Checklist** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Track which patterns and enhancements have been applied to which implementation days.

### **Enhancement Tracking Template**

| Enhancement | Applied To | Status | Notes |
|-------------|------------|--------|-------|
| **Error Handling Philosophy** | Days 2-6 | ⬜ Pending | Apply error categories A-E |
| **Service-Specific Error Categories** | Day 6 EOD | ⬜ Pending | Document 5 error categories |
| **Retry with Exponential Backoff** | Day 3 | ⬜ Pending | External API calls |
| **Circuit Breaker Pattern** | Day 4 | ⬜ Pending | External dependencies |
| **Graceful Degradation** | Day 5 | ⬜ Pending | Cache fallback |
| **Metrics Cardinality Audit** | Day 7 | ⬜ Pending | Per DD-005 |
| **Integration Test Anti-Flaky** | Day 9 | ⬜ Pending | Eventually() pattern |
| **Production Runbooks** | Day 12 | ⬜ Pending | 2-3 runbooks |

### **Day-by-Day Enhancement Application**

**Day 2** (Core Logic Start):
- [ ] Apply error classification for primary component (Category A, D)

**Day 3** (External Dependencies):
- [ ] Implement retry with exponential backoff (Category B)
- [ ] Add auth error handling (Category C)

**Day 4** (Status Management):
- [ ] Add optimistic locking for status updates (Category D)

**Day 5** (Resilience):
- [ ] Add graceful degradation for failures (Category E)

**Day 6** (Error Handling EOD):
- [ ] Document all 5 error categories in Error Handling Philosophy

**Day 7** (Metrics):
- [ ] Complete Metrics Cardinality Audit per DD-005

**Day 8-9** (Testing):
- [ ] Apply anti-flaky patterns (Eventually(), 30s timeout)
- [ ] Test all edge case categories

**Day 12** (Production Readiness):
- [ ] Create 2-3 production runbooks
- [ ] Add Prometheus metrics for runbook automation

---

## 📋 **API Design Patterns** ⭐ V2.6 NEW

### **DD-API-001: HTTP Header vs JSON Body Pattern**

**PRINCIPLE**: Business data goes in JSON body; infrastructure data goes in HTTP headers.

| Data Type | Transport | Examples |
|-----------|-----------|----------|
| **Business Logic** | JSON Body | `reason`, `workflow_id`, `event_type`, `resource_id` |
| **Infrastructure** | HTTP Headers | `X-Request-ID`, `X-Correlation-ID`, `X-Trace-ID` |
| **Security** | HTTP Headers | `Authorization`, `X-API-Key` |

**Rationale**:
1. **Audit Trail Integrity**: JSON body preserved through proxies; headers can be stripped
2. **Consistency**: All mutations use JSON body
3. **SDK Generation**: OpenAPI generators handle JSON body automatically
4. **Logging**: Request body logged as single unit

**Example - Correct Pattern**:
```bash
# ✅ Business data in JSON body
curl -X DELETE http://api/v1/workflows/wf-123 \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: req-456" \           # Infrastructure - OK in header
  -H "X-Correlation-ID: corr-789" \      # Infrastructure - OK in header
  -d '{
    "reason": "Deprecated - replaced by v2"  # Business data - MUST be in body
  }'
```

**Example - Incorrect Pattern**:
```bash
# ❌ Business data in HTTP header
curl -X DELETE http://api/v1/workflows/wf-123 \
  -H "X-Disable-Reason: Deprecated"  # Business data - should be in body
```

**Cross-Reference**: [DD-API-001](../../architecture/decisions/DD-API-001-http-header-vs-json-body-pattern.md)

---

## Day-by-Day Breakdown

### Day 1: Foundation (8h)

#### **Pre-Phase: OpenAPI Spec Update** (30 min) ⭐ V2.6 NEW - **SCOPE: STATELESS HTTP SERVICES**

> **APPLIES TO**: Stateless HTTP services only. Skip for CRD controllers.

Before implementing handlers, update OpenAPI spec with new endpoints:

**File**: `docs/services/stateless/[service]/openapi/v1.yaml`

**Checklist**:
- [ ] Define request schemas (JSON body per DD-API-001)
- [ ] Define response schemas (success + error)
- [ ] Document error responses (RFC 7807 per DD-004)
- [ ] Validate business data is in JSON body (not headers)
- [ ] Add examples for each endpoint

**Example OpenAPI Snippet**:
```yaml
paths:
  /api/v1/[resources]:
    post:
      summary: Create a new [resource]
      operationId: create[Resource]
      tags: [[Resource] Management]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Create[Resource]Request'
      responses:
        '201':
          description: [Resource] created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/[Resource]Response'
        '400':
          description: Invalid request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ProblemDetails'
        '409':
          description: [Resource] already exists
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ProblemDetails'
```

**Benefits**:
- Documents API contract before implementation
- Enables client SDK generation for tests
- Validates request/response schemas early
- Ensures DD-API-001 compliance (business data in body)

---

#### ANALYSIS Phase (1h)
**Search existing patterns:**
```bash
codebase_search "Kubernetes client initialization in-cluster config"
codebase_search "[Service functionality] implementations"
grep -r "relevant patterns" pkg/ cmd/ --include="*.go"
```

**Map business requirements:**
- List all BR-[CATEGORY]-XXX requirements
- Identify critical path requirements
- Note any missing specifications

#### PLAN Phase (1h)
**TDD Strategy:**
- Unit tests: [Component list] (70%+ coverage target)
- Integration tests: [Scenario list] (>50% coverage target)
- E2E tests: [Workflow list] (<10% coverage target)

**Integration points:**
- Main app: `cmd/[service]/main.go`
- Business logic: `pkg/[service]/`
- Tests: `test/unit/[service]/`, `test/integration/[service]/`, `test/e2e/[service]/`

**Success criteria:**
- [Performance metric 1] (target: X)
- [Performance metric 2] (target: Y)
- [Functional requirement] verified

---

### **Test Scenarios by Component** (Define Upfront per TDD)

> **IMPORTANT**: Define concrete test scenarios BEFORE implementation. This aligns with TDD - know what you're testing before writing code.

#### **[Component 1]** (`test/unit/[service]/[component1]_test.go`)

**Happy Path (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C1-HP-01 | [Primary success case] | [Valid input description] | [Expected result] |
| C1-HP-02 | [Secondary success case] | [Valid input description] | [Expected result] |
| C1-HP-03 | [Variation success case] | [Valid input description] | [Expected result] |

**Edge Cases (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C1-EC-01 | [Boundary condition] | [Edge input] | [Expected handling] |
| C1-EC-02 | [Empty/nil handling] | Empty/nil value | [Graceful handling, no panic] |
| C1-EC-03 | [Maximum size handling] | Max size input | [Handles within limits] |
| C1-EC-04 | [Concurrent access] | Parallel requests | [Thread-safe operation] |

**Error Handling (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C1-ER-01 | [External dependency failure] | Valid input, dependency down | [Error with appropriate code] |
| C1-ER-02 | [Timeout scenario] | Valid input, slow response | [Timeout error, no hang] |
| C1-ER-03 | [Validation failure] | Invalid input | [Validation error returned] |
| C1-ER-04 | [Context cancellation] | Valid input, context cancelled | [Returns context.Canceled] |

---

#### **[Component 2]** (`test/unit/[service]/[component2]_test.go`)

**Happy Path (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C2-HP-01 | [Primary success case] | [Valid input] | [Expected result] |

**Edge Cases (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C2-EC-01 | [Boundary condition] | [Edge input] | [Expected handling] |

**Error Handling (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| C2-ER-01 | [Failure scenario] | [Problem input] | [Error handling] |

---

#### **[Reconciler/Controller]** (`test/unit/[service]/reconciler_test.go`) - FOR CRD CONTROLLERS

**Happy Path (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-HP-01 | Full happy path | Valid CR | Status=Completed, all fields populated |
| R-HP-02 | Phase transition Pending→Processing | New CR | Status.Phase="Processing" |
| R-HP-03 | Phase transition Processing→Complete | Processed CR | Status.Phase="Completed" |
| R-HP-04 | Finalizer lifecycle | New CR, then delete | Finalizer added, then removed after cleanup |

**Edge Cases (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-EC-01 | CR deleted during processing | CR deleted mid-reconcile | Graceful termination |
| R-EC-02 | Already completed CR | CR with Status=Completed | No-op, no requeue |
| R-EC-03 | Concurrent reconciles | Same CR reconciled twice | Only one succeeds (optimistic locking) |

**Error Handling (X tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-ER-01 | K8s API unavailable | CR, API server down | Requeue with exponential backoff |
| R-ER-02 | Component failure | CR, internal error | Status=Failed, error in conditions |
| R-ER-03 | Status update conflict | CR, concurrent update | Retries with fresh version |

---

**Test Count Summary Template**:

| Component | Happy Path | Edge Cases | Error Handling | **Total** |
|-----------|------------|------------|----------------|-----------|
| [Component1] | X | X | X | **X** |
| [Component2] | X | X | X | **X** |
| [Component3] | X | X | X | **X** |
| [Reconciler] | X | X | X | **X** |
| **Total** | **X** | **X** | **X** | **X** |

> **Note**: Realistic test counts based on existing services:
> - Gateway: 275 unit, 143 integration tests
> - Data Storage: 392 unit, 160 integration tests
> - CRD Controller (medium complexity): 100-150 unit, 50-80 integration tests

#### DO-DISCOVERY (6h)
**Create package structure:**
```bash
mkdir -p cmd/[service]
mkdir -p pkg/[service]/{component1,component2,component3}
mkdir -p internal/[service]/{helpers}
mkdir -p test/unit/[service]
mkdir -p test/integration/[service]
mkdir -p test/e2e/[service]
```

**Create foundational files:**
- `pkg/[service]/types.go` - Core type definitions
- `pkg/[service]/[interface1].go` - Primary interface
- `pkg/[service]/[interface2].go` - Secondary interface
- `internal/[service]/k8s/client.go` - Kubernetes client wrapper (if needed)
- `cmd/[service]/main.go` - Basic skeleton

**Validation:**
- [ ] All packages created
- [ ] Types defined
- [ ] Interfaces defined
- [ ] Main.go compiles
- [ ] Zero lint errors

**EOD Documentation:**
- [ ] Create `implementation/phase0/01-day1-complete.md`
- [ ] Document architecture decisions
- [ ] Note any deviations from plan

---

### Days 2-6: Core Implementation (5 days, 8h each)

**Pattern for Each Component:**

#### DO-RED: Write Tests First (1.5-2h per component)
**File**: `test/unit/[service]/[component]_test.go`

**⭐ RECOMMENDED: Use Table-Driven Tests (DescribeTable) whenever possible**

**Pattern 1: Table-Driven Tests for Multiple Similar Scenarios** (Preferred)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    // Use DescribeTable for multiple test cases with same logic
    DescribeTable("should handle various input scenarios",
        func(input InputType, expectedOutput OutputType, expectError bool) {
            result, err := component.Method(input)

            if expectError {
                Expect(err).To(HaveOccurred())
            } else {
                Expect(err).ToNot(HaveOccurred())
                Expect(result).To(Equal(expectedOutput))
            }
        },
        Entry("scenario 1 description", input1, output1, false),
        Entry("scenario 2 description", input2, output2, false),
        Entry("scenario 3 with error", input3, nil, true),
        // Easy to add more scenarios - just add Entry lines!
    )
})
```

**Pattern 2: Traditional Tests for Unique Logic** (When needed)
```go
var _ = Describe("BR-[CATEGORY]-XXX: [Component Name]", func() {
    Context("when [unique condition]", func() {
        It("should [behavior]", func() {
            // Test implementation for unique scenario
        })
    })
})
```

**When to Use Table-Driven Tests**:
- ✅ Testing same logic with different inputs/outputs
- ✅ Testing multiple detection/validation scenarios
- ✅ Testing various error conditions
- ✅ Testing different configuration permutations
- ✅ Testing boundary conditions and edge cases

**When to Use Traditional Tests**:
- Complex setup that varies significantly per test
- Unique test logic that doesn't fit table pattern
- One-off tests with complex assertions

**Benefits**:
- 25-40% less code through elimination of duplication
- Easier to add new test cases (just add Entry)
- Better test organization and readability
- Consistent assertion patterns

**Reference**: See Dynamic Toolset detector tests for examples:
- `test/unit/toolset/prometheus_detector_test.go`
- `test/unit/toolset/grafana_detector_test.go`

**Validation:**
- [ ] Tests written (prefer table-driven where applicable)
- [ ] Tests fail (expected)
- [ ] Business requirements referenced (BR-XXX-XXX)
- [ ] Entry names clearly describe scenarios

#### DO-GREEN: Minimal Implementation (1.5-2h per component)
**File**: `pkg/[service]/[component].go`

**Validation:**
- [ ] Tests pass
- [ ] No extra features
- [ ] Integration point identified

#### DO-REFACTOR: Extract Common Patterns (2-3h per day)
**Common Refactorings:**
- Extract shared utilities
- Standardize error handling
- Extract validation logic
- Create helper functions

**Validation:**
- [ ] Code DRY (Don't Repeat Yourself)
- [ ] Patterns consistent
- [ ] Tests still pass

**Day-Specific Focus:**
- **Day 2**: [Component set 1]
- **Day 3**: [Component set 2]
- **Day 4**: [Component set 3] + **EOD: Create 02-day4-midpoint.md** ⭐
- **Day 5**: [Component set 4]
- **Day 6**: [Component set 5] + **DO-REFACTOR: Error handling philosophy doc** ⭐

---

### 📖 Error Handling Philosophy Template ⭐ V2.0 + V2.8 ENHANCED

**⚠️ MANDATORY**: Create this document at end of Day 6 to establish consistent error handling across all components.

**File**: `docs/services/[service-type]/[service-name]/implementation/design/ERROR_HANDLING_PHILOSOPHY.md`

**Purpose**: Define authoritative error handling patterns for this service, ensuring consistency and reliability.

---

### Complete Template (Copy This)

```markdown
# Error Handling Philosophy - [Service Name]

**Date**: YYYY-MM-DD
**Status**: ✅ Authoritative Guide
**Version**: 1.0

---

## 🎯 **Core Principles**

### 1. **Error Classification**
All errors fall into three categories:

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: Network timeouts, 503 Service Unavailable, database connection errors
- **Strategy**: Exponential backoff with jitter
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: 401 Unauthorized, 404 Not Found, validation failures, malformed input
- **Strategy**: Fail immediately, log error, update status
- **Max Retries**: 0 (no retry)

#### **User Errors** (Input Validation)
- **Definition**: Invalid user input or configuration
- **Examples**: Missing required fields, invalid formats, out-of-range values
- **Strategy**: Return validation error immediately, do not retry
- **Max Retries**: 0 (no retry)

---

## 🏷️ **Service-Specific Error Categories** ⭐ V2.8 NEW

> **MANDATORY**: Define 5 error categories (A-E) specific to your service. These categories map to the generic classification above but provide service-specific context.

### **Category Template** (Customize for your service)

#### **Category A: [Resource] Not Found**
- **When**: [Describe when this error occurs]
- **Action**: Log deletion, remove from retry queue
- **Recovery**: Normal (no action needed)
- **Example**: CRD deleted during reconciliation

#### **Category B: [External API] Errors** (Retry with Backoff)
- **When**: [External service] timeout, rate limiting, 5xx errors
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as failed
- **Example**: Slack webhook timeout, Data Storage API unavailable

#### **Category C: [Authentication/Authorization] Errors** (User Error)
- **When**: 401/403 auth errors, invalid credentials
- **Action**: Mark as failed immediately, create event
- **Recovery**: Manual (fix configuration)
- **Example**: Invalid API key, expired token

#### **Category D: [Status/State] Update Conflicts**
- **When**: Multiple processes updating same resource simultaneously
- **Action**: Retry with optimistic locking
- **Recovery**: Automatic (retry status update)
- **Example**: CRD status update conflict, database row lock

#### **Category E: [Data Processing] Failures**
- **When**: Data transformation error, malformed input
- **Action**: Log error, apply graceful degradation
- **Recovery**: Automatic (degraded operation)
- **Example**: Redaction logic error, JSON parsing failure

### **Service-Specific Examples**

**Notification Controller**:
| Category | Specific Error | Action |
|----------|----------------|--------|
| A | NotificationRequest deleted | Log, remove from queue |
| B | Slack API 5xx | Retry with backoff |
| C | Invalid Slack webhook | Fail immediately |
| D | Status update conflict | Retry with optimistic locking |
| E | Sanitization failure | Send with "[REDACTED]" placeholder |

**Gateway Service**:
| Category | Specific Error | Action |
|----------|----------------|--------|
| A | Signal already processed | Skip (idempotent) |
| B | K8s API 503 | Retry with backoff |
| C | Invalid webhook signature | Reject immediately |
| D | Redis write conflict | Retry |
| E | Payload parsing failure | Return 400 Bad Request |

**Data Storage Service**:
| Category | Specific Error | Action |
|----------|----------------|--------|
| A | Workflow not found | Return 404 |
| B | PostgreSQL connection timeout | Retry with backoff |
| C | Invalid API token | Return 401 |
| D | Row lock timeout | Retry transaction |
| E | Embedding generation failure | Return 500 with details |

---

## 🔄 **Retry Strategy**

### Exponential Backoff Implementation

\`\`\`go
// CalculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func CalculateBackoff(attemptCount int) time.Duration {
	baseDelay := 30 * time.Second
	maxDelay := 480 * time.Second

	// Calculate exponential backoff: baseDelay * 2^attemptCount
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

	// Cap at maximum delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

	return jitter
}
\`\`\`

### Retry Decision Matrix

| Error Type | HTTP Status | Retry? | Backoff | Max Attempts | Example |
|-----------|-------------|--------|---------|--------------|---------|
| Transient | 500, 502, 503, 504 | ✅ Yes | Exponential | 5 | Service temporarily unavailable |
| Transient | Timeout | ✅ Yes | Exponential | 5 | Network timeout |
| Transient | Connection refused | ✅ Yes | Exponential | 3 | Service restarting |
| Permanent | 401, 403 | ❌ No | N/A | 0 | Authentication failure |
| Permanent | 404 | ❌ No | N/A | 0 | Resource not found |
| Permanent | 400 | ❌ No | N/A | 0 | Bad request format |
| User Error | Validation | ❌ No | N/A | 0 | Missing required field |

---

## 🔐 **Circuit Breaker Pattern**

### Circuit Breaker States

\`\`\`
CLOSED (Normal Operation) → OPEN (Failing) → HALF_OPEN (Testing) → CLOSED
     ↓                           ↓                    ↓
   Normal                   Fast-fail          Limited retries
\`\`\`

### Implementation Guidance

\`\`\`go
// CircuitBreaker tracks failure rates and manages state transitions
type CircuitBreaker struct {
	State              CircuitState
	FailureCount       int
	FailureThreshold   int // e.g., 5 failures
	SuccessCount       int
	SuccessThreshold   int // e.g., 2 successes in HALF_OPEN
	Timeout            time.Duration // e.g., 60 seconds
	LastFailureTime    time.Time
}

// ShouldAllowRequest determines if request should proceed
func (cb *CircuitBreaker) ShouldAllowRequest() bool {
	switch cb.State {
	case CircuitStateClosed:
		return true // Normal operation
	case CircuitStateOpen:
		// Check if timeout elapsed
		if time.Since(cb.LastFailureTime) > cb.Timeout {
			cb.State = CircuitStateHalfOpen
			cb.SuccessCount = 0
			return true // Try one request
		}
		return false // Fast-fail
	case CircuitStateHalfOpen:
		return true // Allow limited requests
	default:
		return false
	}
}
\`\`\`

**When to Use**:
- External API calls (Slack, email providers)
- Database connections
- Network-dependent operations

**Benefits**:
- Prevents cascade failures
- Reduces load on failing services
- Graceful degradation

---

## 📝 **Error Wrapping & Context**

### Standard Error Wrapping Pattern

\`\`\`go
// Good: Error wrapping with context
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	data, err := s.fetchData(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch data for request %s: %w", req.ID, err)
	}

	if err := s.validate(data); err != nil {
		return fmt.Errorf("validation failed for request %s: %w", req.ID, err)
	}

	return nil
}

// Bad: Error swallowing
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	data, _ := s.fetchData(ctx, req.ID) // ❌ Error ignored!
	s.validate(data)
	return nil
}
\`\`\`

### Context Propagation

Always include:
- **Request ID**: Unique identifier for tracing
- **Resource ID**: Affected resource (pod name, deployment name)
- **Operation**: What was being attempted
- **Timestamp**: When error occurred

---

## 📊 **Logging Best Practices**

### Structured Logging Pattern

\`\`\`go
// Production-ready error logging
func (s *Service) handleError(ctx context.Context, operation string, err error) {
	log := log.FromContext(ctx)

	// Classify error
	errorType := classifyError(err)

	// Structured logging with context
	log.Error(err, "Operation failed",
		"operation", operation,
		"error_type", errorType,
		"retry_able", isRetryable(errorType),
		"request_id", getRequestID(ctx),
		"resource", getResourceName(ctx),
		"timestamp", time.Now().Format(time.RFC3339),
	)

	// Emit metric
	s.metrics.ErrorsTotal.With(prometheus.Labels{
		"operation":  operation,
		"error_type": string(errorType),
	}).Inc()
}
\`\`\`

### Log Levels

| Level | Use When | Example |
|-------|----------|---------|
| ERROR | Permanent failures, requires intervention | Authentication failure, CRD validation error |
| WARN | Transient failures, will retry | Network timeout on attempt 1/5 |
| INFO | Normal operation events | Request processed successfully |
| DEBUG | Detailed troubleshooting | Retry attempt 3/5 with 120s backoff |

---

## 🚨 **Error Recovery Strategies**

### Graceful Degradation

\`\`\`go
// Example: Notification service with graceful degradation
func (s *Service) SendNotifications(ctx context.Context, notif *Notification) error {
	errors := make([]error, 0)

	// Try console (always succeeds)
	if err := s.sendToConsole(ctx, notif); err != nil {
		errors = append(errors, fmt.Errorf("console delivery failed: %w", err))
	}

	// Try Slack (may fail - graceful degradation)
	if err := s.sendToSlack(ctx, notif); err != nil {
		log.Warn("Slack delivery failed, continuing", "error", err)
		errors = append(errors, fmt.Errorf("slack delivery failed: %w", err))
	}

	// Partial success handling
	if len(errors) == 0 {
		return nil // All succeeded
	} else if len(errors) < 2 {
		return nil // At least one succeeded (graceful degradation)
	} else {
		return fmt.Errorf("all deliveries failed: %v", errors)
	}
}
\`\`\`

### Rollback Strategies

For CRD controllers and stateful operations:

\`\`\`go
// Example: Kubernetes action with rollback
func (e *Executor) ExecuteAction(ctx context.Context, action *Action) error {
	// Save original state
	originalState, err := e.captureState(ctx, action.Resource)
	if err != nil {
		return fmt.Errorf("failed to capture original state: %w", err)
	}

	// Attempt action
	if err := e.applyAction(ctx, action); err != nil {
		log.Error(err, "Action failed, attempting rollback")

		// Rollback to original state
		if rollbackErr := e.restoreState(ctx, originalState); rollbackErr != nil {
			return fmt.Errorf("action failed and rollback failed: %w (rollback: %v)", err, rollbackErr)
		}

		return fmt.Errorf("action failed, rolled back successfully: %w", err)
	}

	return nil
}
\`\`\`

---

## ✅ **Implementation Checklist**

Use this checklist when implementing error handling for each component:

- [ ] **Error Classification**: All errors classified as transient/permanent/user
- [ ] **Retry Logic**: Exponential backoff implemented for transient errors
- [ ] **Circuit Breaker**: Implemented for external dependencies
- [ ] **Error Wrapping**: All errors wrapped with context using `fmt.Errorf("%w")`
- [ ] **Structured Logging**: All errors logged with structured fields
- [ ] **Metrics**: Error counters emitted for all error types
- [ ] **Graceful Degradation**: Partial failure handling implemented
- [ ] **Rollback**: State recovery implemented for stateful operations
- [ ] **Testing**: Error scenarios tested (transient, permanent, user)
- [ ] **Documentation**: Error handling patterns documented in code

---

## 📚 **References**

- [Error Handling Standard](../../../architecture/ERROR_HANDLING_STANDARD.md)
- [Logging Standard](../../../architecture/LOGGING_STANDARD.md)
- [Testing Strategy](../../../testing/README.md)

---

**Status**: ✅ Complete Error Handling Philosophy
**Confidence**: 98% - Production-ready error handling patterns
**Next**: Apply this philosophy to all service components
```

---

**Validation After Creation**:
- [ ] Error classification guide complete
- [ ] Retry strategy with exponential backoff defined
- [ ] Circuit breaker pattern documented
- [ ] Error wrapping patterns established
- [ ] Logging best practices defined
- [ ] Graceful degradation examples provided
- [ ] Rollback strategies documented
- [ ] Implementation checklist included

**Impact**: Ensures consistent, production-ready error handling across all components, preventing common pitfalls like error swallowing, infinite retries, and cascade failures.

---

### Day 7: Server + API + Metrics (8h)

#### HTTP Server Implementation (3h)
- Server struct with router
- Route registration
- Middleware stack
- Health/readiness endpoints

#### Metrics Implementation (2h) ⭐ V2.0 ENHANCED

**Target**: 10+ production-ready Prometheus metrics with complete recording patterns

---

**Complete Metrics Definition**:

```go
// pkg/[service]/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics contains all Prometheus metrics for the service
type Metrics struct {
	// Operations metrics
	OperationsTotal   *prometheus.CounterVec
	OperationDuration *prometheus.HistogramVec

	// Error metrics
	ErrorsTotal       *prometheus.CounterVec

	// Resource metrics (for CRD controllers)
	ResourcesProcessed *prometheus.CounterVec
	ReconciliationDuration *prometheus.HistogramVec
	ReconciliationErrors *prometheus.CounterVec

	// Queue metrics (if applicable)
	QueueDepth *prometheus.GaugeVec
	QueueLatency *prometheus.HistogramVec

	// Business-specific metrics
	[CustomMetric1] *prometheus.CounterVec
	[CustomMetric2] *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics(namespace, subsystem string) *Metrics {
	return &Metrics{
		// 1. Operations Counter (tracks all operations)
		OperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "operations_total",
				Help:      "Total number of operations processed",
			},
			[]string{"operation", "status"}, // labels: operation type, success/failure
		),

		// 2. Operation Duration Histogram (latency tracking)
		OperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "operation_duration_seconds",
				Help:      "Duration of operations in seconds",
				Buckets:   prometheus.DefBuckets, // or custom: []float64{.001, .01, .1, .5, 1, 2.5, 5, 10}
			},
			[]string{"operation"},
		),

		// 3. Errors Counter (detailed error tracking)
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "errors_total",
				Help:      "Total number of errors by type",
			},
			[]string{"error_type", "operation"}, // transient/permanent/user
		),

		// 4. Resources Processed (CRD controllers)
		ResourcesProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "resources_processed_total",
				Help:      "Total number of resources processed",
			},
			[]string{"phase", "result"}, // phase: pending/processing/complete, result: success/failure
		),

		// 5. Reconciliation Duration (CRD controllers)
		ReconciliationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_duration_seconds",
				Help:      "Duration of reconciliation loops",
				Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30},
			},
			[]string{"controller"},
		),

		// 6. Reconciliation Errors (CRD controllers)
		ReconciliationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "reconciliation_errors_total",
				Help:      "Total number of reconciliation errors",
			},
			[]string{"controller", "error_type"},
		),

		// 7. Queue Depth (if applicable)
		QueueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_depth",
				Help:      "Current depth of processing queue",
			},
			[]string{"queue_name"},
		),

		// 8. Queue Latency (if applicable)
		QueueLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "queue_latency_seconds",
				Help:      "Time items spend in queue before processing",
				Buckets:   []float64{.5, 1, 5, 10, 30, 60, 300},
			},
			[]string{"queue_name"},
		),

		// 9-10+. Business-specific metrics (customize per service)
		// Example: Notification delivery metrics
		// [CustomMetric1]: promauto.NewCounterVec(...)
		// [CustomMetric2]: promauto.NewGaugeVec(...)
	}
}
```

---

**Metric Recording Patterns in Business Logic**:

```go
// Example 1: Recording operation metrics
func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
	// Start timer for duration
	timer := prometheus.NewTimer(s.metrics.OperationDuration.WithLabelValues("process_request"))
	defer timer.ObserveDuration()

	// Process request
	err := s.doProcess(ctx, req)

	// Record result
	if err != nil {
		s.metrics.OperationsTotal.WithLabelValues("process_request", "failure").Inc()
		s.metrics.ErrorsTotal.WithLabelValues(classifyError(err), "process_request").Inc()
		return err
	}

	s.metrics.OperationsTotal.WithLabelValues("process_request", "success").Inc()
	return nil
}

// Example 2: Recording CRD reconciliation metrics
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Start timer
	timer := prometheus.NewTimer(r.metrics.ReconciliationDuration.WithLabelValues("my-controller"))
	defer timer.ObserveDuration()

	// Fetch resource
	resource := &Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		r.metrics.ReconciliationErrors.WithLabelValues("my-controller", "fetch_error").Inc()
		return ctrl.Result{}, err
	}

	// Process
	if err := r.process(ctx, resource); err != nil {
		r.metrics.ResourcesProcessed.WithLabelValues(string(resource.Status.Phase), "failure").Inc()
		r.metrics.ReconciliationErrors.WithLabelValues("my-controller", "processing_error").Inc()
		return ctrl.Result{}, err
	}

	r.metrics.ResourcesProcessed.WithLabelValues(string(resource.Status.Phase), "success").Inc()
	return ctrl.Result{}, nil
}

// Example 3: Recording gauge metrics (queue depth)
func (s *Service) EnqueueItem(item *Item) error {
	s.queue.Add(item)
	s.metrics.QueueDepth.WithLabelValues("main").Set(float64(s.queue.Len()))
	return nil
}
```

---

**Metrics Endpoint Exposure**:

**For HTTP Services**:
```go
// cmd/[service]/main.go
import (
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

func main() {
	// Initialize metrics
	metrics := metrics.NewMetrics("kubernaut", "[service]")

	// Create service with metrics
	service := NewService(metrics)

	// Expose metrics endpoint
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":9090", nil)

	// Start main service on port 8080
	// ...
}
```

**For CRD Controllers (controller-runtime)**:
```go
// cmd/[service]/main.go
import (
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func main() {
	// Metrics automatically exposed by controller-runtime
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		MetricsBindAddress: ":9090", // Metrics endpoint
		// ...
	})

	// Initialize custom metrics
	customMetrics := metrics.NewMetrics("kubernaut", "[service]")

	// Pass to reconciler
	if err = (&Reconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Metrics: customMetrics,
	}).SetupWithManager(mgr); err != nil {
		// ...
	}
}
```

---

**Testing Metrics**:

```go
// test/unit/[service]/metrics_test.go
package [service]

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/jordigilh/kubernaut/pkg/[service]/metrics"
)

var _ = Describe("BR-XXX-XXX: Metrics Recording", func() {
	var (
		ctx     context.Context
		svc     *[service].Service
		m       *metrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create custom registry for testing (isolated)
		registry := prometheus.NewRegistry()
		m = metrics.NewMetricsWithRegistry(registry)
		svc = [service].NewService(m)
	})

	It("should record successful operations", func() {
		// Execute operation
		err := svc.ProcessRequest(ctx, &Request{})
		Expect(err).ToNot(HaveOccurred())

		// Verify metric recorded
		count := testutil.ToFloat64(m.OperationsTotal.WithLabelValues("process_request", "success"))
		Expect(count).To(Equal(1.0))
	})

	It("should record operation duration", func() {
		// Execute operation
		err := svc.ProcessRequest(ctx, &Request{})
		Expect(err).ToNot(HaveOccurred())

		// Verify histogram recorded (count > 0)
		count := testutil.ToFloat64(m.OperationDuration.WithLabelValues("process_request"))
		Expect(count).To(BeNumerically(">", 0))
	})

	It("should record error metrics", func() {
		// Execute operation that fails
		err := svc.ProcessRequest(ctx, &Request{Invalid: true})
		Expect(err).To(HaveOccurred())

		// Verify error metric recorded
		errorCount := testutil.ToFloat64(m.ErrorsTotal.WithLabelValues("user_error", "process_request"))
		Expect(errorCount).To(Equal(1.0))
	})
})
```

---

**Validation Checklist**:
- [ ] Metrics defined for each business-critical operation (not a fixed count - driven by business value)
- [ ] Metrics registered with promauto (automatic registration)
- [ ] Labels used for dimension breakdown (operation, status, error_type)
- [ ] Histograms for duration/latency (with appropriate buckets)
- [ ] Counters for operations/errors
- [ ] Gauges for current state (queue depth, active connections)
- [ ] Metrics recorded in all business logic paths
- [ ] Metrics endpoint exposed (`:9090/metrics`)
- [ ] Metrics tested with prometheus/testutil
- [ ] ServiceMonitor created (Kubernetes Prometheus Operator)
- [ ] Cardinality audit completed (see Metrics Cardinality Audit section below)

---

### 📊 **Metrics Cardinality Audit** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**⚠️ MANDATORY**: Audit all Prometheus metrics for high-cardinality label combinations per DD-005.

**Purpose**: Prevent Prometheus memory explosion from unbounded label values.

**Target**: Keep total unique metric combinations < 10,000 per service (per DD-005 § 3.1)

**File**: `docs/services/[service-type]/[service-name]/METRICS_CARDINALITY_AUDIT.md`

#### **Cardinality Audit Template**

```markdown
# [Service Name] Metrics Cardinality Audit

**Date**: YYYY-MM-DD
**Status**: ✅ SAFE | ⚠️ MITIGATED | ❌ RISK
**Design Decision**: DD-005 § 3.1 - Metrics Cardinality Management

---

## 📊 **Metrics Inventory**

### **[Category 1] Metrics**

| Metric | Labels | Cardinality | Status |
|--------|--------|-------------|--------|
| `[service]_[metric1]_total` | `operation`, `status` | **Low** (N×M) | ✅ SAFE |
| `[service]_[metric2]_seconds` | `operation` | **Low** (N) | ✅ SAFE |

**Analysis**:
- `operation`: Fixed set ([list values]) → **N values**
- `status`: Fixed set ("success", "error") → **2 values**
- **Total combinations**: N × 2 = **X unique metrics** ✅

---

### **HTTP Metrics** ⚠️ **HIGHEST RISK**

| Metric | Labels | Cardinality | Status |
|--------|--------|-------------|--------|
| `[service]_http_requests_total` | `method`, `path`, `status` | **Medium** | ✅ **MITIGATED** |

**Risk Analysis BEFORE Mitigation**:
- `method`: Fixed set ("GET", "POST", "PUT", "DELETE") → **4 values**
- `path`: **UNBOUNDED** (could include query params or IDs) → **∞ values** ❌
- `status`: Fixed set (HTTP status codes) → **~20 values**
- **Worst case**: 4 × ∞ × 20 = **UNBOUNDED** ❌ **CARDINALITY EXPLOSION RISK**

**Mitigation Applied**:
- Path normalization: `/api/v1/resources/123` → `/api/v1/resources/{id}`
- Query parameter stripping: `/api/v1/search?q=foo` → `/api/v1/search`

**After Mitigation**:
- `path`: Fixed set (normalized endpoints) → **~10 values**
- **Total combinations**: 4 × 10 × 20 = **800 unique metrics** ✅

---

## 🛡️ **Cardinality Protection Patterns**

### **Pattern 1: Path Normalization** (HTTP Services)

**File**: `pkg/[service]/server/path_normalizer.go`

**Implementation**:
- Remove query parameters before recording metric
- Replace UUIDs with `{id}` placeholder
- Replace numeric IDs with `{id}` placeholder

**Example**: `/api/v1/resources/123?page=2` → `/api/v1/resources/{id}`

### **Pattern 2: Label Value Allowlist**

**Implementation**:
- Define fixed set of allowed label values
- Bucket unknown values as "other"
- Never use user input directly as label values

**Example**: Only allow `create`, `read`, `update`, `delete` operations; anything else → `other`

---

## 📈 **Total Cardinality Summary**

| Category | Metrics | Max Cardinality | Status |
|----------|---------|-----------------|--------|
| Operations | 3 | 50 | ✅ |
| HTTP | 2 | 800 | ✅ |
| Cache | 2 | 6 | ✅ |
| Database | 2 | 20 | ✅ |
| **TOTAL** | **9** | **~900** | ✅ **ACCEPTABLE** |

**Cardinality Thresholds** (per DD-005):
- **< 1,000**: ✅ Excellent - no concerns
- **1,000 - 5,000**: ⚠️ Acceptable - monitor growth
- **5,000 - 10,000**: ⚠️ Warning - review and optimize
- **> 10,000**: ❌ Critical - must reduce before production

**Note**: Most well-designed services should have < 1,000 unique metric combinations. The 10,000 limit in DD-005 is a hard ceiling, not a target.

---

## ✅ **Audit Checklist**

- [ ] All metrics inventoried
- [ ] Label values are bounded (no user input, no IDs)
- [ ] Path normalization implemented (HTTP services)
- [ ] Total cardinality < 1,000 (ideal) or < 5,000 (acceptable)
- [ ] High-risk metrics (HTTP, user-facing) mitigated
```

---

#### Main Application Integration (2h)
- Component wiring in main.go
- Configuration loading
- Graceful shutdown

#### Critical EOD Checkpoints (1h) ⭐
- [ ] **Schema Validation**: Create `design/01-[schema]-validation.md`
- [ ] **Test Infrastructure Setup**: Create test suite skeleton
- [ ] **Status Documentation**: Create `03-day7-complete.md`
- [ ] **Testing Strategy**: Create `testing/01-integration-first-rationale.md`

**Why These Matter**: Gateway found these prevented 2+ days of debugging

---

### Day 8: Unit Tests (8h)

**Standard Methodology**: Unit tests first, validating component logic in isolation.
**Parallel Execution**: 4 concurrent processes (`ginkgo -p -procs=4` or `go test -p 4`)

#### All Component Unit Tests (8h)

**Test Infrastructure Setup**: Choose based on your `[TEST_ENVIRONMENT]` decision ⭐ **v1.3**

<details>
<summary><strong>🔴 KIND Setup (if [TEST_ENVIRONMENT] = KIND)</strong></summary>

Use **Kind Cluster Test Template** for standardized integration tests:

**Documentation**: [Kind Cluster Test Template Guide](../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

```go
package myservice

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/testutil/kind"
	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Kind)")
}

var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	// Use Kind template for standardized test setup
	// See: docs/testing/KIND_CLUSTER_TEST_TEMPLATE.md
	suite = kind.Setup("[service]-test", "kubernaut-system")

	// Additional setup if needed (PostgreSQL, Redis, etc.)
	// suite.WaitForPostgreSQLReady(60 * time.Second)
})

var _ = AfterSuite(func() {
	suite.Cleanup()
})

// Integration Test Pattern
Describe("Integration Test [N]: [Scenario]", func() {
	var component *[service].Component

	BeforeEach(func() {
		// Setup real components using Kind cluster resources
		// Example: Deploy test services
		// svc, err := suite.DeployPrometheusService("[service]-test")

		// Initialize component with real dependencies
		component = [service].NewComponent(suite.Client, logger)
	})

	It("should [end-to-end behavior]", func() {
		// Complete workflow test using real Kind cluster resources
		// Example test assertion
		result, err := component.Process(suite.Context, input)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Status).To(Equal("success"))
	})
})
```

**Key Benefits of Kind Template**:
- ✅ **15 lines setup** vs 80+ custom (81% reduction)
- ✅ **Complete imports** (copy-pasteable)
- ✅ **Kind cluster DNS** (no port-forwarding)
- ✅ **Automatic cleanup** (`suite.Cleanup()`)
- ✅ **Consistent pattern** (aligned with Gateway, Dynamic Toolset V2)
- ✅ **30+ helper methods** (services, ConfigMaps, database, wait utilities)

</details>

---

<details>
<summary><strong>🟡 ENVTEST Setup (if [TEST_ENVIRONMENT] = ENVTEST)</strong></summary>

Use **envtest** for Kubernetes API server testing without full cluster:

**Prerequisites**:
```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
setup-envtest use 1.31.0
```

**Documentation**: [envtest Setup Requirements](../../testing/ENVTEST_SETUP_REQUIREMENTS.md)

```go
package myservice

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (envtest)")
}

var (
	cfg       *rest.Config
	k8sClient kubernetes.Interface
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Start envtest with CRDs if needed
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd")},
		ErrorIfCRDPathMissing: false, // Set true if CRDs required
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test K8s API operations]", func() {
		// Create test resources
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}
		_, err := k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		// Test your service logic
		component := [service].NewComponent(k8sClient)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of envtest**:
- ✅ **Real API server** validation (schema, field selectors)
- ✅ **CRD support** (register definitions + use controller-runtime client)
- ✅ **Fast setup** (~3 seconds vs ~60 seconds for KIND)
- ✅ **Standard K8s client** (same as production)
- ⚠️ **No RBAC/TokenReview** (use KIND if needed)

</details>

---

<details>
<summary><strong>🟢 PODMAN Setup (if [TEST_ENVIRONMENT] = PODMAN)</strong></summary>

Use **testcontainers-go** for PostgreSQL/Redis/database testing:

**Prerequisites**: Docker or Podman installed

**Documentation**: [Podman Integration Test Template](../../testing/PODMAN_INTEGRATION_TEST_TEMPLATE.md)

```go
package myservice

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (Podman)")
}

var (
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container
	dbURL             string
	redisAddr         string
)

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections"),
	}
	var err error
	postgresContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	Expect(err).NotTo(HaveOccurred())

	// Get database URL
	host, _ := postgresContainer.Host(ctx)
	port, _ := postgresContainer.MappedPort(ctx, "5432")
	dbURL = fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	// Start Redis container (if needed)
	// ... similar pattern
})

var _ = AfterSuite(func() {
	ctx := context.Background()
	if postgresContainer != nil {
		postgresContainer.Terminate(ctx)
	}
	if redisContainer != nil {
		redisContainer.Terminate(ctx)
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test database operations]", func() {
		// Test your service with real database
		component := [service].NewComponent(dbURL, redisAddr)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of Podman**:
- ✅ **Real databases** (PostgreSQL, Redis, etc.)
- ✅ **Fast startup** (~1-2 seconds)
- ✅ **Automatic cleanup** (testcontainers-go)
- ✅ **No Kubernetes** (simpler for pure HTTP APIs)

</details>

---

<details>
<summary><strong>⚪ HTTP MOCKS Setup (if [TEST_ENVIRONMENT] = HTTP_MOCKS)</strong></summary>

Use **net/http/httptest** for mocking external HTTP APIs:

**Prerequisites**: None (Go stdlib)

```go
package myservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/[service]"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "[Service] Integration Suite (HTTP Mocks)")
}

var (
	mockDataStorageAPI    *httptest.Server
	mockMonitoringAPI     *httptest.Server
)

var _ = BeforeSuite(func() {
	// Mock Data Storage API
	mockDataStorageAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/audit/actions" {
			json.NewEncoder(w).Encode(mockActions)
		}
	}))

	// Mock Monitoring API
	mockMonitoringAPI = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/metrics" {
			json.NewEncoder(w).Encode(mockMetrics)
		}
	}))
})

var _ = AfterSuite(func() {
	if mockDataStorageAPI != nil {
		mockDataStorageAPI.Close()
	}
	if mockMonitoringAPI != nil {
		mockMonitoringAPI.Close()
	}
})

Describe("Integration Test [N]: [Scenario]", func() {
	It("should [test HTTP API interactions]", func() {
		// Test your service with mocked APIs
		component := [service].NewComponent(mockDataStorageAPI.URL, mockMonitoringAPI.URL)
		result, err := component.Process(ctx, input)
		Expect(err).ToNot(HaveOccurred())
	})
})
```

**Key Benefits of HTTP Mocks**:
- ✅ **Zero infrastructure** (no KIND, databases, containers)
- ✅ **Instant startup** (milliseconds)
- ✅ **Easy failure simulation** (return errors, timeouts)
- ✅ **Perfect for pure HTTP API services**

</details>

---

**Required Tests**:
1. **Test 1**: Basic flow (input → processing → output) - 90 min
2. **Test 2**: Deduplication/Caching logic - 45 min
3. **Test 3**: Error recovery scenario - 60 min
4. **Test 4**: Data persistence/state management - 45 min
5. **Test 5**: Authentication/Authorization - 30 min

---

### 📋 Complete Integration Test Examples ⭐ V2.0

**Purpose**: Provide production-ready integration test templates to accelerate Day 8 implementation

---

### **📦 Package Naming Conventions - MANDATORY**

**AUTHORITY**: [TEST_PACKAGE_NAMING_STANDARD.md](../../testing/TEST_PACKAGE_NAMING_STANDARD.md)

**CRITICAL**: ALL tests use same package name as code under test (white-box testing).

| Test Type | Package Name | NO Exceptions |
|-----------|--------------|---------------|
| **Unit Tests** | `package [service]` | ✅ |
| **Integration Tests** | `package [service]` | ✅ |
| **E2E Tests** | `package [service]` | ✅ |

**Key Rules**:
- ✅ **ALWAYS** use `package [service]` (same package as code under test)
- ❌ **NEVER** use `package [service]_test` (violates standard)
- ✅ **White-box testing**: Access to internal state and unexported functions
- ✅ **Simpler imports**: No need to import the package being tested

**Example**:
```go
// ✅ CORRECT: All test types use same package
package myservice  // Unit, Integration, AND E2E tests

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    // No need to import myservice package - we're already in it
)
```

```go
// ❌ WRONG: Never use _test suffix
package myservice_test  // DO NOT USE - violates TEST_PACKAGE_NAMING_STANDARD.md
```

---

#### **Integration Test Example 1: Complete Workflow (CRD Controller)**

**File**: `test/integration/[service]/workflow_test.go`

**BR Coverage**: BR-XXX-001 (Complete workflow), BR-XXX-002 (Status tracking)

```go
package [service]

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 1: Complete Workflow (Pending → Complete)", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-workflow-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup: Delete resource
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should complete full workflow from creation to completion", func() {
		By("Creating resource in Pending state")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "high",
				Data:     map[string]string{"key": "value"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for status to transition to Processing")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal([service]v1alpha1.PhaseProcessing))

		By("Verifying processing timestamps are set")
		processing := &[service]v1alpha1.[Resource]{}
		err = suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, processing)
		Expect(err).ToNot(HaveOccurred())
		Expect(processing.Status.QueuedAt).ToNot(BeNil())
		Expect(processing.Status.ProcessingStartedAt).ToNot(BeNil())

		By("Waiting for status to transition to Complete")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying completion timestamp and results")
		completed := &[service]v1alpha1.[Resource]{}
		err = suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)
		Expect(err).ToNot(HaveOccurred())
		Expect(completed.Status.CompletionTime).ToNot(BeNil())
		Expect(completed.Status.SuccessCount).To(BeNumerically(">", 0))
		Expect(completed.Status.ObservedGeneration).To(Equal(completed.Generation))

		By("Verifying conditions are set correctly")
		readyCondition := meta.FindStatusCondition(completed.Status.Conditions, "Ready")
		Expect(readyCondition).ToNot(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCondition.Reason).To(Equal("ProcessingComplete"))
	})
})
```

---

#### **Integration Test Example 2: Failure Recovery with Retry**

**File**: `test/integration/[service]/failure_recovery_test.go`

**BR Coverage**: BR-XXX-003 (Error recovery), BR-XXX-004 (Exponential backoff)

```go
package [service]

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 2: Failure Recovery with Automatic Retry", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-retry-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should retry on transient failures with exponential backoff", func() {
		By("Creating resource that will initially fail")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/simulate-transient-error": "true",
					"test.kubernaut.ai/fail-attempts":            "2", // Fail first 2 attempts
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "medium",
				Data:     map[string]string{"test": "retry"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for first failure attempt")
		Eventually(func() int {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.AttemptCount
		}, 15*time.Second, 1*time.Second).Should(BeNumerically(">=", 1))

		By("Verifying transient error is recorded")
		firstAttempt := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, firstAttempt)
		Expect(firstAttempt.Status.LastError).To(ContainSubstring("transient"))
		Expect(firstAttempt.Status.AttemptCount).To(BeNumerically(">=", 1))

		By("Waiting for automatic retry and eventual success")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 90*time.Second, 2*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying retry attempts were made with exponential backoff")
		completed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)

		// Should have 3 total attempts (2 failures + 1 success)
		Expect(completed.Status.AttemptCount).To(Equal(3))

		// Verify backoff timing between attempts
		if len(completed.Status.AttemptHistory) >= 2 {
			attempt1Time := completed.Status.AttemptHistory[0].Timestamp.Time
			attempt2Time := completed.Status.AttemptHistory[1].Timestamp.Time
			backoffDuration := attempt2Time.Sub(attempt1Time)

			// First retry should be ~30s (with jitter: 27s-33s)
			Expect(backoffDuration.Seconds()).To(BeNumerically(">=", 25))
			Expect(backoffDuration.Seconds()).To(BeNumerically("<=", 35))
		}
	})

	It("should fail permanently after max retry attempts exceeded", func() {
		By("Creating resource that always fails")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-permanent",
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/simulate-permanent-error": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "[type]",
				Priority: "low",
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for status to reach Failed after max retries")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-permanent",
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 180*time.Second, 5*time.Second).Should(Equal([service]v1alpha1.PhaseFailed))

		By("Verifying max retry attempts reached")
		failed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName + "-permanent",
			Namespace: namespace,
		}, failed)
		Expect(failed.Status.AttemptCount).To(Equal(5)) // Max retry attempts
		Expect(failed.Status.LastError).To(ContainSubstring("max retry attempts exceeded"))
	})
})
```

---

#### **Integration Test Example 3: Graceful Degradation (Multi-Channel)**

**File**: `test/integration/[service]/graceful_degradation_test.go`

**BR Coverage**: BR-XXX-005 (Graceful degradation), BR-XXX-006 (Partial success)

```go
package [service]

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	[service]v1alpha1 "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

var _ = Describe("Integration Test 3: Graceful Degradation (Partial Failure)", func() {
	var (
		ctx          context.Context
		resourceName string
		namespace    string
	)

	BeforeEach(func() {
		ctx = context.Background()
		resourceName = "test-degradation-" + randomString(5)
		namespace = "kubernaut-test"
	})

	AfterEach(func() {
		// Cleanup
		resource := &[service]v1alpha1.[Resource]{}
		if err := suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, resource); err == nil {
			suite.Client.Delete(ctx, resource)
		}
	})

	It("should complete successfully when at least one channel succeeds", func() {
		By("Creating resource with multiple delivery channels")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName,
				Namespace: namespace,
				Annotations: map[string]string{
					// Simulate Slack failure, Console success
					"test.kubernaut.ai/fail-slack": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "notification",
				Priority: "high",
				Channels: []string{"console", "slack"},
				Data:     map[string]string{"message": "Test notification"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for resource to complete (graceful degradation)")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 30*time.Second, 1*time.Second).Should(Equal([service]v1alpha1.PhaseComplete))

		By("Verifying partial success status")
		completed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: namespace,
		}, completed)

		// At least one channel succeeded
		Expect(completed.Status.SuccessfulDeliveries).To(BeNumerically(">=", 1))

		// Some channels failed
		Expect(completed.Status.FailedDeliveries).To(BeNumerically(">=", 1))

		// Total attempts = success + failures
		totalExpected := completed.Status.SuccessfulDeliveries + completed.Status.FailedDeliveries
		Expect(completed.Status.TotalAttempts).To(Equal(totalExpected))

		By("Verifying delivery attempts are recorded")
		Expect(len(completed.Status.DeliveryAttempts)).To(BeNumerically(">=", 2))

		// Find console delivery (should succeed)
		consoleDelivery := findDeliveryAttempt(completed.Status.DeliveryAttempts, "console")
		Expect(consoleDelivery).ToNot(BeNil())
		Expect(consoleDelivery.Status).To(Equal("success"))

		// Find Slack delivery (should fail)
		slackDelivery := findDeliveryAttempt(completed.Status.DeliveryAttempts, "slack")
		Expect(slackDelivery).ToNot(BeNil())
		Expect(slackDelivery.Status).To(Equal("failed"))
		Expect(slackDelivery.Error).To(ContainSubstring("slack"))
	})

	It("should fail when all channels fail", func() {
		By("Creating resource where all channels fail")
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceName + "-all-fail",
				Namespace: namespace,
				Annotations: map[string]string{
					"test.kubernaut.ai/fail-all-channels": "true",
				},
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				Type:     "notification",
				Priority: "medium",
				Channels: []string{"console", "slack"},
			},
		}

		err := suite.Client.Create(ctx, resource)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for resource to fail (all channels failed)")
		Eventually(func() [service]v1alpha1.Phase {
			updated := &[service]v1alpha1.[Resource]{}
			suite.Client.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-all-fail",
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 60*time.Second, 2*time.Second).Should(Equal([service]v1alpha1.PhaseFailed))

		By("Verifying all deliveries failed")
		failed := &[service]v1alpha1.[Resource]{}
		suite.Client.Get(ctx, types.NamespacedName{
			Name:      resourceName + "-all-fail",
			Namespace: namespace,
		}, failed)

		Expect(failed.Status.SuccessfulDeliveries).To(Equal(0))
		Expect(failed.Status.FailedDeliveries).To(BeNumerically(">=", 2))
		Expect(failed.Status.LastError).To(ContainSubstring("all channels failed"))
	})
})

// Helper function
func findDeliveryAttempt(attempts []v1alpha1.DeliveryAttempt, channel string) *v1alpha1.DeliveryAttempt {
	for i := range attempts {
		if attempts[i].Channel == channel {
			return &attempts[i]
		}
	}
	return nil
}

func randomString(length int) string {
	// Simple random string generator for test names
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
```

---

**Integration Test Best Practices Applied**:

✅ **Complete Examples**: All 3 tests are production-ready, copy-pasteable code
✅ **Real Infrastructure**: Tests use actual Kind cluster, not mocks
✅ **Comprehensive Assertions**: Multiple verification points per test
✅ **Proper Cleanup**: AfterEach ensures no resource leaks
✅ **Timing Considerations**: Eventually() with appropriate timeouts
✅ **BR Coverage**: Each test maps to specific business requirements
✅ **Failure Scenarios**: Tests cover success, transient failure, permanent failure, partial success
✅ **Production-Ready**: Error handling, contexts, proper resource management

**Total Integration Test Code**: ~400 lines of complete, production-ready examples

---

**Validation After Integration Tests**:
- [ ] Architecture validated
- [ ] Integration issues found early
- [ ] Timing/concurrency issues identified
- [ ] Ready for unit test details

#### Afternoon: Unit Tests Part 1 (4h)
- Focus on components tested in integration tests
- Fill in edge cases
- Add negative test cases

**Metrics Validation Checkpoint**:
```bash
curl http://localhost:9090/metrics | grep [service]_
```

---

### Day 9: Unit Tests Part 2 (8h)

#### Morning: Unit Tests - [Component Group 1] (4h)
- Edge cases
- Error conditions
- Boundary values

#### Afternoon: Unit Tests - [Component Group 2] (4h)
- Mock dependencies
- Timeout scenarios
- Concurrent access

**EOD: Create BR Coverage Matrix** ⭐ **V2.0 ENHANCED**
**File**: `implementation/testing/BR-COVERAGE-MATRIX.md`

---

### Enhanced BR Coverage Matrix Template (Complete)

```markdown
# BR Coverage Matrix - [Service Name]

**Date**: YYYY-MM-DD
**Status**: Complete coverage validation
**Target Coverage**: ≥97% (V3.0 standard)

---

## 📊 **Coverage Summary**

### Overall Coverage Calculation

**Formula**: `(BRs with tests / Total BRs) * 100 = Coverage %`

**Example**:
- Total BRs: 9
- BRs with unit tests: 9 (100%)
- BRs with integration tests: 8 (89%)
- BRs with E2E tests: 4 (44%)
- **Overall BR Coverage**: 9/9 = **100%** ✅

### Coverage By Test Type

| Test Type | BR Coverage | Test Count | Code Coverage | Status |
|-----------|-------------|------------|---------------|--------|
| **Unit Tests** | 100% (9/9 BRs) | 50+ tests | ~75% | ✅ **Target: >70%** |
| **Integration Tests** | 89% (8/9 BRs) | 5 critical tests | ~60% | ✅ **Target: >50%** |
| **E2E Tests** | 44% (4/9 BRs) | 1 comprehensive test | ~15% | ✅ **Target: <10%** |

**Overall Test Quality**: **97.2% BR coverage** ✅

---

## 🔍 **Per-BR Coverage Breakdown**

### **BR-[CATEGORY]-001: [Requirement Name]**

**Requirement**: [Full requirement description]

#### Unit Tests
- **File**: `test/unit/[service]/[component]_test.go`
- **Tests**:
  - `It("should [behavior 1]")` - Lines 45-62
  - `It("should [behavior 2]")` - Lines 64-78
  - `DescribeTable("should handle multiple scenarios")` - Lines 80-95 (5 scenarios)
- **Coverage**: 3 tests + 5 table entries = **8 test cases** ✅

#### Integration Tests
- **File**: `test/integration/[service]/[scenario]_test.go`
- **Tests**:
  - `It("should [integration scenario]")` - Lines 120-155
- **Coverage**: 1 integration test ✅

#### E2E Tests
- **File**: `test/e2e/[service]/[workflow]_test.go`
- **Tests**:
  - Covered in comprehensive E2E workflow
- **Coverage**: Implicit ✅

**Status**: ✅ **100% Coverage** (unit + integration + E2E)

---

### **BR-[CATEGORY]-002: [Requirement Name]**

**Requirement**: [Full requirement description]

#### Unit Tests
- **File**: `test/unit/[service]/[component]_test.go`
- **Tests**:
  - `DescribeTable("should handle [scenarios]")` - Lines 100-125 (7 entries)
- **Coverage**: 7 table-driven test cases ✅

#### Integration Tests
- **File**: `test/integration/[service]/[scenario]_test.go`
- **Tests**:
  - `It("should [integration scenario]")` - Lines 200-240
- **Coverage**: 1 integration test ✅

**Status**: ✅ **100% Coverage** (unit + integration)

---

### **BR-[CATEGORY]-003: [Requirement Name]**

[Repeat pattern for each BR...]

---

## 📈 **Coverage Gap Analysis**

### ✅ **Fully Covered BRs** (100% coverage)

| BR | Requirement | Unit | Integration | E2E | Status |
|----|-------------|------|-------------|-----|--------|
| BR-XXX-001 | [Req 1] | 8 tests | 1 test | ✅ | ✅ Complete |
| BR-XXX-002 | [Req 2] | 7 tests | 1 test | ❌ | ✅ Complete |
| BR-XXX-003 | [Req 3] | 5 tests | 1 test | ✅ | ✅ Complete |
| ...

**Count**: 9/9 BRs (100%) ✅

### ⚠️ **Partially Covered BRs** (50-99% coverage)

**None** - All BRs fully covered ✅

### ❌ **Uncovered BRs** (0-49% coverage)

**None** - All BRs fully covered ✅

---

## 🎯 **Testing Strategy Validation**

### Unit Test Coverage (Target: >70%)

**Achieved**: ~75% code coverage ✅

**Coverage By Component**:
- [Component 1]: 80% coverage (50 tests)
- [Component 2]: 75% coverage (35 tests)
- [Component 3]: 70% coverage (25 tests)

**Status**: ✅ **Exceeds target**

### Integration Test Coverage (Target: >50%)

**Achieved**: ~60% scenario coverage ✅

**Critical Scenarios Covered**:
1. Complete workflow (Pending → Sent)
2. Failure recovery with retry
3. Graceful degradation (partial failure)
4. Status tracking (multiple attempts)
5. Priority handling (critical vs low)

**Status**: ✅ **Exceeds target**

### E2E Test Coverage (Target: <10%)

**Achieved**: ~15% production scenarios ✅

**Critical Paths Covered**:
- End-to-end workflow with real external dependencies

**Status**: ✅ **Within acceptable range**

---

## 📊 **Test Distribution Analysis**

### Test Count By Type

| Type | Count | Percentage | Target | Status |
|------|-------|------------|--------|--------|
| Unit Tests | 50+ | ~70% | 70%+ | ✅ Met |
| Integration Tests | 5 | ~20% | >50% coverage | ✅ Met |
| E2E Tests | 1 | ~10% | <10% | ✅ Met |

**Total Tests**: 56+ tests covering 9 business requirements

---

## ✅ **Validation Checklist**

Before releasing:
- [ ] All BRs mapped to tests ✅
- [ ] Unit test coverage >70% ✅
- [ ] Integration test coverage >50% ✅
- [ ] E2E test coverage >10% (but <20%) ✅
- [ ] No BRs with 0% coverage ✅
- [ ] Critical paths tested ✅
- [ ] Failure scenarios tested ✅
- [ ] Table-driven tests used where applicable ✅
- [ ] All test files documented in this matrix ✅

**Status**: ✅ **Ready for Production** (97.2% BR coverage)

---

## 📝 **Test File Reference Index**

### Unit Tests
- `test/unit/[service]/[component1]_test.go` - BR-XXX-001, BR-XXX-002
- `test/unit/[service]/[component2]_test.go` - BR-XXX-003, BR-XXX-004
- `test/unit/[service]/[component3]_test.go` - BR-XXX-005, BR-XXX-006

### Integration Tests
- `test/integration/[service]/suite_test.go` - Setup and teardown
- `test/integration/[service]/workflow_test.go` - BR-XXX-001 integration
- `test/integration/[service]/failure_test.go` - BR-XXX-002, BR-XXX-003 integration
- `test/integration/[service]/degradation_test.go` - BR-XXX-004 integration

### E2E Tests
- `test/e2e/[service]/end_to_end_test.go` - BR-XXX-001, BR-XXX-003, BR-XXX-005, BR-XXX-007

---

## 🔄 **Coverage Maintenance**

### When to Update This Matrix
- After adding new business requirements
- After implementing new tests
- Before release (validation checkpoint)
- During code reviews

### Coverage Targets
- **Unit**: Maintain >70% (increase to 75%+ for complex services)
- **Integration**: Maintain >50% (increase to 60%+ for CRD controllers)
- **E2E**: Keep <10% (only critical user journeys)

### Quality Indicators
- ✅ **Excellent**: >95% BR coverage (V3.0 standard)
- ✅ **Good**: 90-95% BR coverage
- ⚠️ **Acceptable**: 85-90% BR coverage
- ❌ **Insufficient**: <85% BR coverage

**Current Status**: **97.2%** = ✅ **Excellent** (V3.0 standard achieved)

---

**Confidence Assessment**: 98%
- **Evidence**: Complete BR-to-test mapping, all coverage targets exceeded
- **Risk**: Minimal - comprehensive test coverage validated
- **Next Steps**: Monitor coverage during future development, update matrix when adding new BRs
```

---

**Validation**:
- [ ] BR Coverage Matrix complete with calculation methodology
- [ ] All BRs mapped to specific test files and line numbers
- [ ] Coverage calculation formula documented
- [ ] Per-BR breakdown included
- [ ] Coverage gap analysis completed
- [ ] Test distribution analysis validated
- [ ] Coverage maintenance plan documented
- [ ] Unit test coverage >70% validated
- [ ] Integration test coverage >50% validated
- [ ] Overall BR coverage ≥97% achieved

---

### Day 10: Advanced Integration + E2E Tests (8h)

#### Advanced Integration Tests (4h)
- Concurrent request scenarios
- Resource exhaustion
- Long-running operations
- Failure recovery

#### E2E Test Setup (2h)

**Reference**: [DD-TEST-001: Port Allocation Strategy](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) (AUTHORITATIVE)

**⚠️ MANDATORY**: Use Kind NodePort instead of kubectl port-forward for E2E tests.

**Why NodePort?**
| Aspect | Port-Forward | NodePort |
|--------|--------------|----------|
| **Stability** | Crashes under concurrent load | 100% stable |
| **Performance** | Slow (proxy overhead) | Fast (direct connection) |
| **Parallelism** | Limited to ~4 processes | Unlimited (all CPUs) |
| **Evidence** | Gateway: 17% failure rate at 12 procs | Gateway: 0% failure rate, 6.4x speedup |

**Step 1: Create Kind Config** (`test/infrastructure/kind-[service]-config.yaml`)
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  # Service API (if HTTP endpoints) - use port from DD-TEST-001
  - containerPort: {{NODEPORT}}     # e.g., 30082 for Signal Processing
    hostPort: {{HOST_PORT}}         # e.g., 8082
    protocol: TCP
  # Metrics endpoint (always needed for controllers)
  - containerPort: {{METRICS_NODEPORT}}  # e.g., 30182
    hostPort: {{METRICS_HOST_PORT}}      # e.g., 9182
    protocol: TCP
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
- role: worker
```

**Step 2: Configure Service as NodePort** (`test/e2e/[service]/deployment.yaml`)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: [service]-service
spec:
  type: NodePort  # ← MANDATORY: NodePort, NOT ClusterIP
  selector:
    app: [service]
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: {{METRICS_NODEPORT}}  # From DD-TEST-001
```

**Step 3: Test Suite Uses NodePort Directly** (NO port-forward)
```go
// SynchronizedBeforeSuite - NodePort pattern (Gateway reference)
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // Process 1: Create Kind cluster and deploy service (ONCE)
        err := infrastructure.CreateCluster(clusterName, kubeconfigPath, GinkgoWriter)
        Expect(err).ToNot(HaveOccurred())

        err = infrastructure.DeployService(ctx, namespace, kubeconfigPath, GinkgoWriter)
        Expect(err).ToNot(HaveOccurred())

        return []byte(kubeconfigPath)
    },
    func(data []byte) {
        // ALL processes: Use NodePort directly (no port-forward)
        kubeconfigPath = string(data)

        // NodePort URL - same for all parallel processes
        serviceURL = "http://localhost:{{HOST_PORT}}"     // From DD-TEST-001
        metricsURL = "http://localhost:{{METRICS_HOST_PORT}}/metrics"

        // Wait for service readiness via NodePort
        Eventually(func() error {
            resp, err := http.Get(metricsURL)
            if err != nil {
                return err
            }
            defer resp.Body.Close()
            return nil
        }, 60*time.Second, 2*time.Second).Should(Succeed())
    },
)
```

**Port Allocation Consolidated Reference** ⭐ V3.0 ENHANCED (from DD-TEST-001):

| Service | Internal Health | Internal Metrics | Host Port | NodePort | Metrics NodePort |
|---------|-----------------|------------------|-----------|----------|------------------|
| Gateway | 8081 | 9090 | 8080 | 30080 | 30090 |
| Data Storage | 8081 | 9090 | 8081 | 30081 | 30181 |
| Signal Processing | 8081 | 9090 | 8082 | 30082 | 30182 |
| AI Analysis | 8081 | 9090 | 8083 | 30083 | 30183 |
| Notification | 8081 | 9090 | 8086 | 30086 | 30186 |
| Remediation Orchestrator | 8081 | 9090 | 8087 | 30087 | 30187 |

**Port Type Definitions**:
- **Internal Health**: Pod-internal health/readiness probes (`:8081/health`, `:8081/ready`)
- **Internal Metrics**: Pod-internal Prometheus metrics (`:9090/metrics`)
- **Host Port**: Docker/Podman host mapping for integration tests
- **NodePort**: Kind cluster NodePort for E2E tests (no port-forward needed)
- **Metrics NodePort**: Prometheus scraping in Kind cluster

**Full table and rationale**: See [DD-TEST-001](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

---

#### **E2E Test Helper Patterns** ⭐ V2.6 NEW - **SCOPE: STATELESS HTTP SERVICES**

> **APPLIES TO**: Stateless HTTP services with CRUD operations. CRD controllers use K8s client helpers instead.

Create reusable test helpers for API operations to reduce E2E test duplication and ensure consistent API usage.

**File**: `test/e2e/[service]/[service]_helpers.go`

```go
package [service]

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

// CreateTestResource creates a resource via REST API
// Returns response for assertion (caller checks status code)
func CreateTestResource(httpClient *http.Client, baseURL string, resource map[string]interface{}) (*http.Response, error) {
    body, err := json.Marshal(resource)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal resource: %w", err)
    }
    return httpClient.Post(baseURL+"/api/v1/[resources]", "application/json", bytes.NewBuffer(body))
}

// GetTestResource retrieves a resource by ID
func GetTestResource(httpClient *http.Client, baseURL, resourceID string) (*http.Response, error) {
    return httpClient.Get(fmt.Sprintf("%s/api/v1/[resources]/%s", baseURL, resourceID))
}

// DeleteTestResource disables/deletes a resource
// Reason is passed in JSON body per DD-API-001 (not HTTP header)
func DeleteTestResource(httpClient *http.Client, baseURL, resourceID, reason string) (*http.Response, error) {
    body, _ := json.Marshal(map[string]string{"reason": reason})
    req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/api/v1/[resources]/%s", baseURL, resourceID), bytes.NewBuffer(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    return httpClient.Do(req)
}

// SearchTestResources searches resources with filters
func SearchTestResources(httpClient *http.Client, baseURL string, filters map[string]interface{}) (*http.Response, error) {
    body, err := json.Marshal(filters)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal filters: %w", err)
    }
    return httpClient.Post(baseURL+"/api/v1/[resources]/search", "application/json", bytes.NewBuffer(body))
}

// WaitForResourceReady waits until a resource reaches expected state
func WaitForResourceReady(httpClient *http.Client, baseURL, resourceID string, timeout time.Duration) error {
    return Eventually(func() error {
        resp, err := GetTestResource(httpClient, baseURL, resourceID)
        if err != nil {
            return err
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
            return fmt.Errorf("resource not ready: status %d", resp.StatusCode)
        }
        return nil
    }, timeout, 2*time.Second).Should(Succeed())
}
```

**Benefits**:
- **Consistency**: All tests use same API patterns
- **DD-API-001 Compliance**: Helpers enforce JSON body for business data
- **Maintainability**: API changes require updating one file
- **Readability**: Tests focus on assertions, not HTTP mechanics

**Usage in E2E Tests**:
```go
var _ = Describe("Resource E2E", func() {
    It("should create and retrieve resource", func() {
        // Create
        resp, err := CreateTestResource(httpClient, serviceURL, map[string]interface{}{
            "id": "test-001",
            "name": "Test Resource",
        })
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusCreated))

        // Retrieve
        resp, err = GetTestResource(httpClient, serviceURL, "test-001")
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })
})
```

---

#### **CRD Controller E2E Helper Patterns** ⭐ V2.6 NEW - **SCOPE: CRD CONTROLLERS**

> **APPLIES TO**: CRD controllers. Uses K8s client instead of HTTP client.

**File**: `test/e2e/[controller]/[controller]_helpers.go`

```go
package [controller]

import (
    "context"
    "fmt"
    "time"

    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    [group]v1 "[module]/api/[group]/v1"
)

// CreateTestCR creates a CR for testing
func CreateTestCR(ctx context.Context, k8sClient client.Client, namespace, name string, spec [group]v1.[Resource]Spec) (*[group]v1.[Resource], error) {
    cr := &[group]v1.[Resource]{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
        Spec: spec,
    }
    err := k8sClient.Create(ctx, cr)
    return cr, err
}

// WaitForCRStatus waits until CR reaches expected status phase
func WaitForCRStatus(ctx context.Context, k8sClient client.Client, namespace, name string, expectedPhase string, timeout time.Duration) error {
    return Eventually(func() string {
        cr := &[group]v1.[Resource]{}
        err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cr)
        if err != nil {
            return ""
        }
        return cr.Status.Phase
    }, timeout, 2*time.Second).Should(Equal(expectedPhase))
}

// DeleteTestCR deletes a CR and waits for cleanup
func DeleteTestCR(ctx context.Context, k8sClient client.Client, namespace, name string) error {
    cr := &[group]v1.[Resource]{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: namespace,
        },
    }
    return k8sClient.Delete(ctx, cr)
}
```

---

#### E2E Test Execution (2h)
- Complete workflow tests
- Real environment validation
- Parallel execution with 4 processes (`ginkgo -p -procs=4`)

---

### Day 11: Comprehensive Documentation (8h) ⭐ V2.0 ENHANCED

**Purpose**: Create production-ready documentation that enables smooth handoffs and operational excellence

---

#### Implementation Documentation (4h)

**File 1**: `docs/services/[service]/README.md` (Complete Service Overview)

```markdown
# [Service Name] Service

## Overview
**Service Type**: [CRD Controller | Stateless Service]
**Purpose**: [One-sentence business purpose]
**Dependencies**: [List critical dependencies]
**Status**: ✅ Production-Ready | 🚧 In Development

## Quick Start
\`\`\`bash
# Build
make build-[service]

# Run locally
./_bin/[service] --config config/development.yaml

# Run tests
make test-[service]
\`\`\`

## Architecture

### Component Diagram
\`\`\`
[External Input] → [Main Handler] → [Business Logic] → [External Output]
                       ↓                    ↓
                  [Metrics]          [Database/Cache]
\`\`\`

### Key Components
- **[Component1]**: [Purpose and responsibility]
- **[Component2]**: [Purpose and responsibility]
- **[Component3]**: [Purpose and responsibility]

### Data Flow
1. [Step 1 description]
2. [Step 2 description]
3. [Step 3 description]

## Configuration

### Controller Manager Options (CLI flags - NOT configurable in YAML)

> **IMPORTANT**: These are hardcoded or set via CLI flags for safety reasons.
> Reference: [DD-TEST-001](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) for port allocation.

| Option | Default | Description | Rationale |
|--------|---------|-------------|-----------|
| `metrics-bind-address` | `:9090` | Prometheus metrics endpoint | Standard port for all services |
| `health-probe-bind-address` | `:8081` | Liveness/readiness probes | Standard health port |
| `leader-elect` | `true` (production) | Leader election for HA | **Always enabled** in production to prevent split-brain |
| `leader-election-id` | `[service].kubernaut.ai` | Unique leader election ID | Must be unique per controller (see [DD-CRD-001](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)) |

```go
// pkg/[service]/config/controller.go
package config

// ControllerConfig holds controller-manager options (CLI flags, not YAML).
// These are NOT exposed in config.yaml for safety reasons.
type ControllerConfig struct {
    // MetricsAddr is the address for Prometheus metrics endpoint.
    // Default: ":9090" - hardcoded, not configurable.
    MetricsAddr string

    // HealthProbeAddr is the address for health probe endpoint.
    // Default: ":8081" - hardcoded, not configurable.
    HealthProbeAddr string

    // LeaderElection is ALWAYS enabled for CRD controllers in production.
    // This prevents split-brain scenarios. Not configurable.
    LeaderElection bool

    // LeaderElectionID uniquely identifies this controller for leader election.
    LeaderElectionID string
}

// DefaultControllerConfig returns safe defaults for CRD controllers.
func DefaultControllerConfig() ControllerConfig {
    return ControllerConfig{
        MetricsAddr:      ":9090",
        HealthProbeAddr:  ":8081",
        LeaderElection:   true, // ALWAYS true in production
        LeaderElectionID: "[service].kubernaut.ai", // DD-CRD-001: Use .ai domain
    }
}
```

### Business Configuration (config.yaml)

> **IMPORTANT**: Only business logic configuration goes in YAML. Controller infrastructure is hardcoded.

\`\`\`yaml
# config/[service].yaml example
[service]:
  # Business-specific settings
  setting1: "value1"      # [Description of what this controls]
  setting2: 30            # [Default: 30, range: 10-300]

  # External dependencies
  dependencies:
    data_storage_url: "http://data-storage.kubernaut-system.svc.cluster.local:8080"
    timeout: 5s

  # Audit configuration (per ADR-038)
  audit:
    buffer_size: 1000     # In-memory buffer size for fire-and-forget writes
    flush_interval: 5s    # Background flush interval
\`\`\`

```go
// pkg/[service]/config/config.go
package config

import "time"

// Config holds business configuration for the service.
// Note: Controller infrastructure (ports, leader election) is NOT here.
type Config struct {
    // Business-specific settings
    Setting1 string `yaml:"setting1" validate:"required"`
    Setting2 int    `yaml:"setting2" validate:"min=10,max=300"`

    // Dependencies
    Dependencies DependencyConfig `yaml:"dependencies" validate:"required"`

    // Audit (per ADR-038 - fire-and-forget pattern)
    Audit AuditConfig `yaml:"audit" validate:"required"`
}

type DependencyConfig struct {
    DataStorageURL string        `yaml:"data_storage_url" validate:"required,url"`
    Timeout        time.Duration `yaml:"timeout" validate:"required"`
}

type AuditConfig struct {
    BufferSize    int           `yaml:"buffer_size" validate:"min=100,max=10000"`
    FlushInterval time.Duration `yaml:"flush_interval" validate:"required"`
}
```

### Configuration Options
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `setting1` | string | required | [What it does] |
| `setting2` | int | 30 | [Valid range and impact] |
| `dependencies.data_storage_url` | string | required | Data Storage Service URL |
| `dependencies.timeout` | duration | 5s | HTTP client timeout |
| `audit.buffer_size` | int | 1000 | Fire-and-forget buffer size (ADR-038) |
| `audit.flush_interval` | duration | 5s | Background flush interval |

## API Reference (for HTTP services)

### Endpoints

#### POST /api/v1/[resource]
**Purpose**: [What this endpoint does]

**Request**:
\`\`\`json
{
  "field1": "value1",
  "field2": 123
}
\`\`\`

**Response**:
\`\`\`json
{
  "id": "generated-id",
  "status": "processing"
}
\`\`\`

**Error Codes**:
- `400`: [When this happens]
- `500`: [When this happens]

## CRD Reference (for CRD controllers)

### Resource Spec
\`\`\`yaml
apiVersion: [group]/v1alpha1
kind: [ResourceKind]
metadata:
  name: example
  namespace: kubernaut
spec:
  field1: "value1"     # [Description]
  field2: 123          # [Valid range]
  field3:              # [Complex field description]
    nestedField: "value"
\`\`\`

### Status Fields
| Field | Type | Description |
|-------|------|-------------|
| `phase` | Phase | Current processing phase |
| `conditions` | []Condition | Detailed status conditions |
| `observedGeneration` | int64 | Last processed generation |

### Phase Lifecycle
\`\`\`
Pending → Processing → Complete
            ↓
          Failed (with retry)
\`\`\`

## Integration Guide

### Integrating with [Service]

**Step 1**: [First integration step]
\`\`\`go
// Example code
import "[service]"

service := [service].New(config)
\`\`\`

**Step 2**: [Second integration step]
\`\`\`go
// Example usage
result, err := service.Process(ctx, request)
\`\`\`

## Observability

### Metrics
Service exposes Prometheus metrics at `:9090/metrics`:

| Metric | Type | Description |
|--------|------|-------------|
| `[service]_operations_total` | Counter | Total operations processed |
| `[service]_operation_duration_seconds` | Histogram | Operation latency |
| `[service]_errors_total` | Counter | Total errors by type |

### Health Checks
- **Liveness**: `GET /healthz` - Always returns 200 if process is running
- **Readiness**: `GET /readyz` - Returns 200 when ready to serve traffic

### Logging
Structured logging using `logr`:
\`\`\`json
{
  "level": "info",
  "ts": "2025-10-12T10:30:00Z",
  "msg": "Processing request",
  "request_id": "uuid",
  "operation": "process"
}
\`\`\`

## Troubleshooting

### Common Issues

**Issue**: [Problem description]
- **Symptom**: [What you see]
- **Cause**: [Why it happens]
- **Fix**: [How to resolve]

**Issue**: [Another problem]
- **Symptom**: [What you see]
- **Cause**: [Why it happens]
- **Fix**: [How to resolve]

## Development

### Running Tests
\`\`\`bash
# Unit tests
make test-unit-[service]

# Integration tests (requires Kind cluster)
make test-integration-[service]

# E2E tests
make test-e2e-[service]
\`\`\`

### Local Development Setup
1. [Setup step 1]
2. [Setup step 2]
3. [Setup step 3]

## References
- [Business Requirements](../../requirements/BR-[SERVICE]-REQUIREMENTS.md)
- [Design Decisions](./implementation/DESIGN_DECISIONS.md)
- [Implementation Plan](./implementation/IMPLEMENTATION_PLAN_V3.0.md)
- [Production Readiness](./implementation/PRODUCTION_READINESS_REPORT.md)
\`\`\`

---

#### Design Decision Documentation (2h) ⭐ V2.0 ENHANCED

**File 2**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`

```markdown
# [Service] Design Decisions

## DD-[SERVICE]-001: [Decision Title]

**Date**: 2025-10-12
**Status**: ✅ Accepted | 🚧 Proposed | ❌ Rejected
**Deciders**: [Names]

### Context
[Describe the problem or situation requiring a decision]

### Decision
[State the decision clearly]

### Alternatives Considered

#### Option A: [Alternative 1]
**Pros**:
- [Advantage 1]
- [Advantage 2]

**Cons**:
- [Disadvantage 1]
- [Disadvantage 2]

#### Option B: [Alternative 2]
**Pros**:
- [Advantage 1]

**Cons**:
- [Disadvantage 1]

#### Option C: Selected - [Chosen Option]
**Pros**:
- [Why this is best]
- [Specific advantage]

**Cons**:
- [Acknowledged tradeoffs]

### Rationale
[Explain why Option C was chosen despite tradeoffs]

### Consequences
**Positive**:
- [Expected benefit 1]
- [Expected benefit 2]

**Negative**:
- [Accepted limitation 1]
- [Mitigation strategy]

### Implementation Notes
- [Key implementation detail 1]
- [Key implementation detail 2]

### References
- [Related BR-XXX-XXX]
- [Related code: pkg/[service]/[file].go]
```

---

#### Testing Documentation (2h) ⭐ V2.0 ENHANCED

**File 3**: `docs/services/[service]/implementation/TESTING_STRATEGY.md`

```markdown
# [Service] Testing Strategy

## Test Coverage Overview

### Coverage by Test Type
| Test Type | Count | Coverage | Confidence |
|-----------|-------|----------|------------|
| Unit Tests | [N] | 70-75% | 85-90% |
| Integration Tests | [M] | 15-20% | 80-85% |
| E2E Tests | [K] | 5-10% | 90-95% |

### Coverage by Component
| Component | Unit | Integration | E2E | Total Coverage |
|-----------|------|-------------|-----|----------------|
| [Component1] | 80% | 15% | 5% | 100% |
| [Component2] | 75% | 20% | 5% | 100% |
| [Component3] | 70% | 25% | 5% | 100% |

## Test Infrastructure

### Unit Tests
**Framework**: Ginkgo/Gomega
**Mocks**: Real business logic, mock external dependencies only
**Location**: `test/unit/[service]/`

### Integration Tests
**Framework**: Ginkgo/Gomega + Kind cluster
**Infrastructure**: Real Kubernetes API (Kind), real database (Testcontainers)
**Location**: `test/integration/[service]/`

### E2E Tests
**Framework**: Ginkgo/Gomega + full deployment
**Infrastructure**: Complete Kind cluster with all services
**Location**: `test/e2e/[service]/`

## Test Scenarios

### Critical Paths (Must Test)
1. [Happy path scenario]
   - **Coverage**: Unit + Integration + E2E
   - **Files**: `test/*/[service]/happy_path_test.go`

2. [Error recovery scenario]
   - **Coverage**: Unit + Integration
   - **Files**: `test/*/[service]/error_recovery_test.go`

3. [Concurrent operations scenario]
   - **Coverage**: Integration + E2E
   - **Files**: `test/*/[service]/concurrent_test.go`

### Edge Cases (Unit Tests)
- [Edge case 1]
- [Edge case 2]
- [Edge case 3]

## Known Limitations

### Test Coverage Gaps
- **Gap 1**: [Description of what's not tested]
  - **Reason**: [Why it's not tested]
  - **Mitigation**: [How risk is managed]
  - **Future Work**: [Plan to address]

- **Gap 2**: [Another untested scenario]
  - **Reason**: [Justification]
  - **Mitigation**: [Risk management]

### Test Infrastructure Limitations
- **Limitation 1**: [Infrastructure constraint]
  - **Impact**: [What this affects]
  - **Workaround**: [How we handle it]

## Running Tests

### Quick Test Commands
\`\`\`bash
# All tests
make test-[service]

# By type
make test-unit-[service]
make test-integration-[service]
make test-e2e-[service]

# Specific test
go test -v ./test/unit/[service]/ -ginkgo.focus="specific test"
\`\`\`

### CI/CD Integration
\`\`\`yaml
# .github/workflows/[service]-tests.yml
- name: Unit Tests
  run: make test-unit-[service]

- name: Integration Tests
  run: make test-integration-[service]
  # Requires Kind cluster setup
\`\`\`

## Test Maintenance

### Adding New Tests
1. Identify BR-XXX-XXX requirement
2. Determine appropriate test type (unit/integration/E2E)
3. Follow table-driven pattern if applicable
4. Update BR Coverage Matrix

### Updating Existing Tests
1. Maintain BR-XXX-XXX mapping in test description
2. Keep test names descriptive
3. Update documentation if behavior changes

## References
- [BR Coverage Matrix](./BR_COVERAGE_MATRIX.md)
- [Integration Test Examples](../../../test/integration/[service]/)
```

---

**Documentation Validation Checklist**:
- [ ] README.md complete with all sections
- [ ] Configuration examples tested and accurate
- [ ] API/CRD reference matches implementation
- [ ] Design decisions documented with DD-XXX format
- [ ] Testing strategy reflects actual test coverage
- [ ] Known limitations documented with mitigation
- [ ] Troubleshooting guide includes common issues
- [ ] All code examples are production-ready
- [ ] References to other docs are valid links

---

### Day 12: CHECK Phase + Production Readiness ⭐ V2.0 COMPREHENSIVE (8h)

#### CHECK Phase Validation (2h)
**Checklist**:
- [ ] All business requirements met
- [ ] Build passes without errors
- [ ] All tests passing
- [ ] Metrics exposed and validated
- [ ] Health checks functional
- [ ] Authentication working
- [ ] Documentation complete
- [ ] No lint errors

#### Production Readiness Checklist (2h) ⭐ V2.0 COMPREHENSIVE
**File**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`

```markdown
# [Service] Production Readiness Assessment

**Assessment Date**: 2025-10-12
**Assessment Status**: ✅ Production-Ready | 🚧 Partially Ready | ❌ Not Ready
**Overall Score**: XX/100 (target 95+)

---

## 1. Functional Validation (Weight: 30%)

### 1.1 Critical Path Testing
- [ ] **Happy path** - Complete workflow from input to success output
  - **Test**: `test/integration/[service]/workflow_test.go`
  - **Evidence**: All phases transition correctly (Pending → Processing → Complete)
  - **Score**: X/10

- [ ] **Error recovery** - Transient failure with automatic retry
  - **Test**: `test/integration/[service]/failure_recovery_test.go`
  - **Evidence**: Exponential backoff working, retries succeed after transient errors
  - **Score**: X/10

- [ ] **Permanent failure** - Failure after max retries
  - **Test**: `test/integration/[service]/failure_recovery_test.go`
  - **Evidence**: Fails gracefully after 5 retry attempts, status reflects failure
  - **Score**: X/10

### 1.2 Edge Cases and Boundary Conditions
- [ ] **Empty/nil inputs** - Handles missing or invalid data
  - **Test**: `test/unit/[service]/validation_test.go`
  - **Evidence**: Proper validation errors, no panics
  - **Score**: X/5

- [ ] **Large payloads** - Handles maximum expected data size
  - **Test**: `test/unit/[service]/large_payload_test.go`
  - **Evidence**: Processes 10MB payloads without memory issues
  - **Score**: X/5

- [ ] **Concurrent operations** - Thread-safe under concurrent load
  - **Test**: `test/integration/[service]/concurrent_test.go`
  - **Evidence**: 100 concurrent operations complete successfully
  - **Score**: X/5

### 1.3 Graceful Degradation
- [ ] **Partial success** - System continues with partial functionality
  - **Test**: `test/integration/[service]/graceful_degradation_test.go`
  - **Evidence**: Completes when at least 1 channel succeeds, records failures
  - **Score**: X/5

### Functional Validation Score: XX/35 (Target: 32+)

---

## 2. Operational Validation (Weight: 25%)

### 2.1 Observability - Metrics
- [ ] **Business-critical Prometheus metrics** defined and exported (count driven by business value, not arbitrary threshold)
  - **File**: `pkg/[service]/metrics/metrics.go`
  - **Endpoint**: `:9090/metrics`
  - **Evidence**: `curl localhost:9090/metrics | grep [service]_ | wc -l` returns 10+
  - **Score**: X/5

- [ ] **Metrics recorded** in all business logic paths
  - **Test**: `test/unit/[service]/metrics_test.go`
  - **Evidence**: All operations increment counters, duration histograms populated
  - **Score**: X/5

- [ ] **Metric labels** provide useful dimension breakdown
  - **Evidence**: Labels include operation, status, error_type for debugging
  - **Score**: X/3

### 2.2 Observability - Logging
- [ ] **Structured logging** using logr throughout
  - **Evidence**: All log entries include context (request_id, resource, operation)
  - **Score**: X/4

- [ ] **Log levels** appropriate (Info for normal, Error for failures)
  - **Evidence**: No Debug logs in production code, errors always logged
  - **Score**: X/3

### 2.3 Observability - Health Checks
- [ ] **Liveness probe** - Returns 200 when process is alive
  - **Endpoint**: `GET /healthz`
  - **Test**: `curl localhost:8081/healthz` returns 200 (health probe port per DD-TEST-001)
  - **Score**: X/3

- [ ] **Readiness probe** - Returns 200 when ready to serve traffic
  - **Endpoint**: `GET /readyz`
  - **Test**: Checks database connectivity, returns 503 if unhealthy
  - **Score**: X/3

### 2.4 Graceful Shutdown
- [ ] **Signal handling** - SIGTERM/SIGINT handled gracefully
  - **Evidence**: In-flight requests complete before shutdown (30s grace period)
  - **Test**: Manual test with `kill -TERM <pid>`
  - **Score**: X/3

### Operational Validation Score: XX/29 (Target: 27+)

---

## 3. Security Validation (Weight: 15%)

### 3.1 RBAC Permissions
- [ ] **Minimal permissions** - Service has only required Kubernetes permissions
  - **File**: `config/rbac/role.yaml`
  - **Evidence**: No wildcard permissions (`*`), no cluster-admin
  - **Score**: X/5

- [ ] **ServiceAccount** properly configured with RBAC
  - **File**: `deploy/manifests/[service]-deployment.yaml`
  - **Evidence**: Custom ServiceAccount with role binding
  - **Score**: X/3

### 3.2 Secret Management
- [ ] **No hardcoded secrets** in code or configuration
  - **Evidence**: Code review confirms all secrets from Kubernetes Secrets
  - **Score**: X/4

- [ ] **Secrets documented** with examples (not actual values)
  - **File**: `deploy/manifests/[service]-secret.yaml.example`
  - **Score**: X/3

### Security Validation Score: XX/15 (Target: 14+)

---

## 4. Performance Validation (Weight: 15%)

### 4.1 Latency
- [ ] **P50 latency** < 100ms for normal operations
  - **Test**: `go test -bench=BenchmarkProcess -benchmem`
  - **Evidence**: Benchmark results show 50ms P50
  - **Score**: X/5

- [ ] **P99 latency** < 500ms for normal operations
  - **Evidence**: Benchmark results show 300ms P99
  - **Score**: X/5

### 4.2 Throughput
- [ ] **Throughput** meets requirements (e.g., 100 ops/sec)
  - **Test**: Load test with 100 concurrent requests
  - **Evidence**: Sustains 150 ops/sec without errors
  - **Score**: X/5

### Performance Validation Score: XX/15 (Target: 13+)

---

## 5. Deployment Validation (Weight: 15%)

### 5.1 Kubernetes Manifests
- [ ] **Deployment manifest** complete with resource limits
  - **File**: `deploy/manifests/[service]-deployment.yaml`
  - **Evidence**: CPU (100m-500m), Memory (128Mi-512Mi) limits set
  - **Score**: X/4

- [ ] **ConfigMap** for configuration management
  - **File**: `deploy/manifests/[service]-configmap.yaml`
  - **Evidence**: All runtime config externalized
  - **Score**: X/3

- [ ] **Service manifest** (if applicable)
  - **File**: `deploy/manifests/[service]-service.yaml`
  - **Evidence**: Service exposes ports for health (8081) and metrics (9090). API port (8080) if HTTP service.
  - **Score**: X/3

### 5.2 Probes Configuration
- [ ] **Liveness probe** configured with appropriate thresholds
  - **Evidence**: `periodSeconds: 10, failureThreshold: 3`
  - **Score**: X/3

- [ ] **Readiness probe** configured with appropriate thresholds
  - **Evidence**: `periodSeconds: 5, failureThreshold: 3`
  - **Score**: X/2

### Deployment Validation Score: XX/15 (Target: 14+)

---

## 6. Documentation Quality (Weight: 10% bonus, not in total)

- [ ] **README.md** comprehensive with all sections
  - **Score**: X/3

- [ ] **Design Decisions** documented with DD-XXX format
  - **Score**: X/2

- [ ] **Testing Strategy** reflects actual implementation
  - **Score**: X/2

- [ ] **Troubleshooting Guide** includes common issues
  - **Score**: X/3

### Documentation Score: XX/10 (Bonus: adds to overall score)

---

## Overall Production Readiness Assessment

**Total Score**: XX/109 (Functional:35 + Operational:29 + Security:15 + Performance:15 + Deployment:15)
**With Documentation Bonus**: XX/119

**Production Readiness Level**:
- **95-100%** (113+): ✅ **Production-Ready** - Deploy to production immediately
- **85-94%** (101-112): 🚧 **Mostly Ready** - Minor improvements needed
- **75-84%** (89-100): ⚠️ **Needs Work** - Address gaps before production
- **<75%** (<89): ❌ **Not Ready** - Significant work required

**Current Level**: [✅ Production-Ready | 🚧 Mostly Ready | ⚠️ Needs Work | ❌ Not Ready]

---

## Critical Gaps (Score < Target)

### Gap 1: [Area where score is below target]
- **Current Score**: X/Y (Target: Z)
- **Missing**: [What's missing]
- **Impact**: [Risk if not addressed]
- **Mitigation**: [Plan to address]

### Gap 2: [Another gap]
- **Current Score**: X/Y
- **Missing**: [Description]
- **Impact**: [Risk assessment]
- **Mitigation**: [Action plan]

---

## Risks and Mitigations

### Risk 1: [Identified risk]
- **Probability**: Low | Medium | High
- **Impact**: Low | Medium | High
- **Mitigation**: [Specific mitigation strategy]
- **Owner**: [Responsible person/team]

### Risk 2: [Another risk]
- **Probability**: [Level]
- **Impact**: [Level]
- **Mitigation**: [Strategy]

---

## Production Deployment Recommendation

### Go/No-Go Decision
**Recommendation**: ✅ GO | 🚧 GO with caveats | ❌ NO-GO

**Justification**:
[Explain the recommendation based on scores, gaps, and risks]

### Pre-Deployment Checklist
- [ ] All critical gaps addressed
- [ ] High-priority risks mitigated
- [ ] Deployment manifests reviewed
- [ ] Rollback plan documented
- [ ] Monitoring dashboards configured
- [ ] On-call team briefed

### Post-Deployment Monitoring
- Monitor metrics dashboard for 24 hours
- Watch for error rate spikes in `[service]_errors_total`
- Track latency in `[service]_operation_duration_seconds`
- Review logs for unexpected ERROR entries

**Monitoring Dashboard**: [Link to Grafana/Prometheus dashboard]
```

---

### 🔄 **Rollback Plan Template** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Document rollback procedures for significant feature implementations.

**File**: `docs/services/[service]/implementation/ROLLBACK_PLAN.md`

```markdown
# Rollback Plan - [Feature Name]

**Date**: YYYY-MM-DD
**Feature**: [Feature being implemented]
**Rollback Window**: 24 hours post-deployment

---

## 🚨 **Rollback Triggers**

| Trigger | Threshold | Action |
|---------|-----------|--------|
| Error rate spike | >5% increase from baseline | Initiate rollback |
| Latency degradation | P99 >2x baseline | Investigate, consider rollback |
| Critical functionality broken | Any | Immediate rollback |
| Data corruption detected | Any | Immediate rollback + incident |

---

## 📋 **Rollback Procedure**

### **Step 1: Verify Rollback Need** (5 min)
- [ ] Confirm issue is related to new deployment
- [ ] Check metrics dashboard for anomalies
- [ ] Review recent logs for errors
- [ ] Notify on-call team

### **Step 2: Execute Rollback** (10 min)

**For Kubernetes Deployments**:
\`\`\`bash
# Option A: Rollback to previous revision
kubectl rollout undo deployment/[service] -n [namespace]

# Option B: Rollback to specific revision
kubectl rollout undo deployment/[service] -n [namespace] --to-revision=X

# Verify rollback
kubectl rollout status deployment/[service] -n [namespace]
\`\`\`

**For Database Migrations**:
\`\`\`bash
# Run down migration
goose -dir migrations postgres "$DATABASE_URL" down

# Verify schema
psql "$DATABASE_URL" -c "\d [table_name]"
\`\`\`

### **Step 3: Verify Rollback Success** (15 min)
- [ ] Pods running previous version
- [ ] Error rate returned to baseline
- [ ] Health checks passing
- [ ] Critical functionality working

### **Step 4: Post-Rollback Actions**
- [ ] Create incident report
- [ ] Notify stakeholders
- [ ] Schedule root cause analysis
- [ ] Plan fix and re-deployment

---

## 🛡️ **Rollback Safeguards**

| Safeguard | Implementation |
|-----------|----------------|
| **Feature flags** | Disable feature without rollback |
| **Canary deployment** | Rollback only affected pods |
| **Database versioning** | Goose migrations with down scripts |
| **Configuration rollback** | ConfigMap revision history |

---

## 📊 **Rollback Metrics**

Track these metrics during rollback:
- `deployment_rollback_total` - Number of rollbacks
- `deployment_rollback_duration_seconds` - Time to complete rollback
- `service_availability_during_rollback` - Uptime during rollback
```

---

### 🔧 **Critical Issues Resolved Section** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Document critical issues encountered during implementation and their resolutions for future reference.

**File**: `docs/services/[service]/implementation/CRITICAL_ISSUES_RESOLVED.md`

```markdown
# Critical Issues Resolved - [Service Name]

**Date**: YYYY-MM-DD
**Implementation Phase**: Day X

---

## 🔧 **Issue Summary**

| Issue # | Title | Severity | Resolution Time | Status |
|---------|-------|----------|-----------------|--------|
| 1 | [Issue title] | Critical | Xh | ✅ Resolved |
| 2 | [Issue title] | High | Xh | ✅ Resolved |

---

## 📋 **Issue Details**

### **Issue #1: [Issue Title]**

**Severity**: Critical | High | Medium
**Time to Resolve**: X hours
**Impact**: [What was broken/blocked]

**Problem**:
[Describe the problem in detail]

**Error Message**:
\`\`\`
[Paste actual error message]
\`\`\`

**Root Cause**:
[Explain why this happened]

**Solution**:
[Describe the fix applied]

**Lesson Learned**:
> **[Key takeaway for future implementations]**

**Prevention**:
- [ ] Add to pre-implementation checklist
- [ ] Create automated check
- [ ] Update documentation

---

### **Issue #2: [Architecture Mismatch Example]**

**Severity**: Critical
**Time to Resolve**: 2 hours
**Impact**: Service completely non-functional

**Problem**:
Segmentation fault (SIGSEGV) during network operations on arm64 (M1 Mac).

**Error Message**:
\`\`\`
runtime: unexpected return pc for netpoll_epoll.go
fatal error: unknown caller pc
\`\`\`

**Root Cause**:
Dockerfile hardcoded `ARG GOARCH=amd64`, but running on arm64 architecture.

**Solution**:
Rebuilt image with `--build-arg GOARCH=arm64` or use multi-arch build.

**Lesson Learned**:
> **Never hardcode GOARCH in Dockerfiles for multi-arch support. Use `--platform` flag or multi-arch manifests.**

**Prevention**:
- [x] Added to ADR-027 (Multi-Architecture Build Strategy)
- [x] CI/CD builds both amd64 and arm64

---

### **Issue #3: [Security Context Example]**

**Severity**: High
**Time to Resolve**: 30 minutes
**Impact**: Pod failed to start

**Problem**:
Kubernetes couldn't verify non-root user.

**Error Message**:
\`\`\`
container has runAsNonRoot and image has non-numeric user (context-api-user),
cannot verify user is non-root
\`\`\`

**Root Cause**:
Used string username instead of numeric UID in security context.

**Solution**:
Added `runAsUser: 1001` to security context.

**Lesson Learned**:
> **Always specify numeric UIDs when `runAsNonRoot: true`**

**Prevention**:
- [x] Added to deployment checklist
- [x] Updated Dockerfile template
```

---

### 📋 **Pre-Day Validation Checklist** ⭐ V2.8 NEW - **SCOPE: COMMON (ALL SERVICES)**

**Purpose**: Formal validation checkpoint before milestone days (Day 7, Day 10, Day 12).

**File**: `docs/services/[service]/implementation/PRE_DAY_X_VALIDATION.md`

```markdown
# Pre-Day [X] Validation Results

**Date**: YYYY-MM-DD
**Validator**: [Name]
**Status**: ✅ READY | ⚠️ ISSUES | ❌ BLOCKED

---

## 🎯 **Executive Summary**

**Overall Confidence**: XX% (Target: 99%)

| Category | Items Validated | Passed | Status |
|----------|----------------|--------|--------|
| **Mandatory Test Compliance** | X files | X | ✅/❌ |
| **Test Suite** | X tests | X | ✅/❌ |
| **Business Requirements** | X BRs | X | ✅/❌ |
| **Performance** | X areas | X | ✅/❌ |
| **Security** | X areas | X | ✅/❌ |
| **Documentation** | X docs | X | ✅/❌ |
| **TOTAL** | **X items** | **X** | ✅/❌ |

**Gap Analysis**: X% confidence gap is due to:
- [List any gaps and their impact]

**Ready for Day [X]**: ✅ YES | ❌ NO

---

## ✅ **Validation Phases**

### **Phase 1: Test Suite Execution**

**All Tests Passing**: X/X (100%)
- **Unit Tests**: X/X passing
- **Integration Tests**: X/X passing
- **E2E Tests**: X/X passing (if applicable)

**Evidence**:
\`\`\`
[Paste test output summary]
\`\`\`

---

### **Phase 2: Mandatory Compliance**

**Ginkgo/Gomega Compliance**: ✅ 100%
- [ ] All test files use Ginkgo framework
- [ ] DescribeTable pattern used where appropriate
- [ ] No standard Go `testing.T` tests

**Refactoring Completed**:
| File | Before | After | Lines Saved |
|------|--------|-------|-------------|
| `[file]_test.go` | Go table-driven | Ginkgo DescribeTable | X lines |

---

### **Phase 3: Business Requirement Coverage**

**BR Coverage**: X/X BRs (100%)

| BR | Description | Unit | Integration | E2E | Status |
|----|-------------|------|-------------|-----|--------|
| BR-XXX-001 | [Description] | ✅ | ✅ | ✅ | ✅ 100% |
| BR-XXX-002 | [Description] | ✅ | ⬜ | ⬜ | ⚠️ 50% |

---

### **Phase 4: Performance Validation**

| Metric | Target | Actual | Status |
|--------|--------|--------|--------|
| P99 Latency | <100ms | Xms | ✅/❌ |
| Throughput | >1000 RPS | X RPS | ✅/❌ |
| Memory Usage | <256MB | XMB | ✅/❌ |

---

### **Phase 5: Security Validation**

- [ ] Authentication working
- [ ] Authorization rules enforced
- [ ] Input validation complete
- [ ] No sensitive data in logs

---

## 🚨 **Blocking Issues**

| Issue | Severity | Resolution Required | Owner |
|-------|----------|---------------------|-------|
| [None] | - | - | - |

---

## ✅ **Sign-off**

**Validation Complete**: ✅ YES
**Ready for Day [X]**: ✅ YES

**Validator**: _________________ Date: ___________
```

---

#### File Organization (1h) ⭐
**File**: `implementation/FILE_ORGANIZATION_PLAN.md`

Categorize all files:
- Production implementation (pkg/, cmd/)
- Unit tests (test/unit/)
- Integration tests (test/integration/)
- E2E tests (test/e2e/)
- Configuration (deploy/)
- Documentation (docs/)

**Git commit strategy**:
```
Commit 1: Foundation (types, interfaces)
Commit 2: Component 1
Commit 3: Component 2
...
Commit N: Tests
Commit N+1: Documentation
Commit N+2: Deployment manifests
```

#### Performance Benchmarking (1h) ⭐
**File**: `implementation/PERFORMANCE_REPORT.md`

```bash
go test -bench=. -benchmem ./pkg/[service]/...
```

Validate:
- [ ] Latency targets met
- [ ] Throughput targets met
- [ ] Memory usage acceptable
- [ ] CPU usage acceptable

#### Troubleshooting Guide (1h) ⭐
**File**: `implementation/TROUBLESHOOTING_GUIDE.md`

For each common issue:
- **Symptoms**: What the user sees
- **Diagnosis**: How to investigate
- **Resolution**: How to fix

#### Confidence Assessment (1h) ⭐
**File**: `implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
## Confidence Assessment

**Implementation Accuracy**: X% (target 90%+)
**Evidence**: [spec compliance, code review results]

**Test Coverage**:
- Unit: X% (target 70%+)
- Integration: X% (target 50%+)
- E2E: X% (target <10%)

**Business Requirement Coverage**: X% (target 100%)
**Mapped BRs**: [count]
**Untested BRs**: [count with justification]

**Production Readiness**: X% (target 95%+)
**Risks**: [list with mitigations]
```

#### Handoff Summary (Last Step) ⭐ V2.0 COMPREHENSIVE
**File**: `docs/services/[service]/implementation/00-HANDOFF-SUMMARY.md`

```markdown
# [Service] Implementation Handoff Summary

**Service Name**: [Service Full Name]
**Implementation Dates**: [Start Date] - [End Date]
**Implementation Team**: [Team Members]
**Handoff Date**: 2025-10-12
**Document Status**: ✅ Complete

---

## Executive Summary

**What Was Built**:
[2-3 sentence summary of what was accomplished and why it matters]

**Current Status**: ✅ Production-Ready | 🚧 Beta | 🔬 Experimental

**Production Readiness Score**: XX/119 ([Percentage]%)

**Key Achievement**: [One sentence highlighting the main accomplishment]

---

## Implementation Overview

### Scope Accomplished
✅ **Phase 1 (Days 1-3)**: Foundation and types
- [X] packages created with [Y] interfaces
- CRD schema defined and generated
- Configuration structure established

✅ **Phase 2 (Days 4-7)**: Business logic implementation
- [N] core components implemented
- Error handling with exponential backoff retry
- Business-critical Prometheus metrics integrated
- Health checks and graceful shutdown

✅ **Phase 3 (Days 8-10)**: Testing
- [N] integration tests (covering happy path, error recovery, graceful degradation)
- [M] unit tests (70-75% coverage)
- [K] E2E tests (<10% coverage)

✅ **Phase 4 (Days 11-12)**: Documentation + Production Readiness
- Complete README with API reference
- Design decisions documented (DD-XXX format)
- Production readiness assessment completed
- Deployment manifests finalized

### Scope Deferred (If Any)
- [Feature 1]: Deferred to [Version/Date] - [Reason]
- [Feature 2]: Deferred to [Version/Date] - [Reason]

---

## Architecture Summary

### Component Diagram
\`\`\`
[High-level component diagram showing key components and interactions]

Example:
┌──────────────┐
│  Controller  │ (watches CRs)
└──────┬───────┘
       │
       ▼
┌──────────────┐         ┌──────────────┐
│   Manager    │────────▶│  Processor   │
└──────────────┘         └──────┬───────┘
                                │
                                ▼
                         ┌──────────────┐
                         │   Delivery   │ (external API calls)
                         └──────────────┘
\`\`\`

### Key Components
| Component | Purpose | Key Files |
|-----------|---------|-----------|
| **Controller** | Reconciles CRs | `pkg/[service]/controller/reconciler.go` |
| **Manager** | Orchestrates processing | `pkg/[service]/manager/manager.go` |
| **Processor** | Business logic | `pkg/[service]/processor/processor.go` |
| **Delivery** | External integrations | `pkg/[service]/delivery/*.go` |

### Data Flow
1. **Trigger**: User creates CR / HTTP request arrives
2. **Validation**: Input validated, defaults applied
3. **Processing**: Business logic executes with retry on transient errors
4. **Delivery**: Results delivered to external systems
5. **Status Update**: CR status updated with results

---

## Business Requirements Coverage

### Implemented Requirements
- ✅ **BR-[SERVICE]-001**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-002**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-003**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-004**: [Requirement description] - Fully implemented
- ✅ **BR-[SERVICE]-005**: [Requirement description] - Fully implemented

**Total**: [N]/[M] business requirements implemented (100%)

### Deferred/Out-of-Scope Requirements
- ⏸️ **BR-[SERVICE]-XXX**: [Requirement] - Deferred to v2.0 ([Justification])

---

## Key Design Decisions

### DD-[SERVICE]-001: [Major Decision Title]
**Decision**: [What was decided]
**Rationale**: [Why this approach was chosen]
**Alternatives**: [What other options were considered]
**Impact**: [How this affects the system]

### DD-[SERVICE]-002**: [Another Decision]
**Decision**: [What was decided]
**Rationale**: [Why]
**Impact**: [Effects]

[Refer to full document: `DESIGN_DECISIONS.md`]

---

## Key Files and Locations

### Production Code
- **Main Entry Point**: `cmd/[service]/main.go`
- **Core Business Logic**: `pkg/[service]/processor/processor.go`
- **CRD Controller**: `pkg/[service]/controller/reconciler.go`
- **API Types**: `api/[service]/v1alpha1/*_types.go`
- **Metrics**: `pkg/[service]/metrics/metrics.go`

### Tests
- **Integration Tests**: `test/integration/[service]/` ([N] tests)
- **Unit Tests**: `test/unit/[service]/` ([M] tests)
- **E2E Tests**: `test/e2e/[service]/` ([K] tests)

### Configuration
- **CRD Schema**: `config/crd/bases/[group]_[resources].yaml`
- **RBAC**: `config/rbac/role.yaml`
- **Deployment**: `deploy/manifests/[service]-deployment.yaml`
- **ConfigMap**: `deploy/manifests/[service]-configmap.yaml`

### Documentation
- **Service README**: `docs/services/[service]/README.md`
- **Implementation Plan**: `docs/services/[service]/implementation/IMPLEMENTATION_PLAN_V3.0.md`
- **Design Decisions**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`
- **Production Readiness**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`

---

## Testing Summary

### Test Coverage Breakdown
| Test Type | Count | Coverage | Confidence | Files |
|-----------|-------|----------|------------|-------|
| **Integration** | [N] | 15-20% | 80-85% | `test/integration/[service]/` |
| **Unit** | [M] | 70-75% | 85-90% | `test/unit/[service]/` |
| **E2E** | [K] | 5-10% | 90-95% | `test/e2e/[service]/` |

### Key Test Scenarios Covered
✅ Happy path (Pending → Processing → Complete)
✅ Error recovery with exponential backoff retry
✅ Permanent failure after max retries
✅ Graceful degradation (partial success)
✅ Concurrent operations (100 concurrent requests)
✅ Edge cases (nil inputs, large payloads, invalid data)

### Known Test Gaps
- [Gap 1]: [Description] - Mitigated by [Strategy]
- [Gap 2]: [Description] - Accepted risk because [Justification]

---

## Deployment Guide

### Quick Deployment (Development)
\`\`\`bash
# Build
make build-[service]

# Deploy to Kind cluster
kubectl apply -f deploy/manifests/[service]-deployment.yaml

# Verify deployment
kubectl get pods -n kubernaut | grep [service]
kubectl logs -f deployment/[service] -n kubernaut
\`\`\`

### Production Deployment
\`\`\`bash
# Apply CRDs (first-time only)
kubectl apply -f config/crd/bases/

# Apply RBAC
kubectl apply -f config/rbac/

# Create ConfigMap and Secrets
kubectl apply -f deploy/manifests/[service]-configmap.yaml
kubectl create secret generic [service]-secrets --from-literal=api-key=xxx

# Deploy service
kubectl apply -f deploy/manifests/[service]-deployment.yaml

# Verify
kubectl rollout status deployment/[service] -n kubernaut
curl http://[service]:8081/healthz  # Health probe port (per DD-TEST-001)
curl http://[service]:9090/metrics  # Prometheus metrics port
\`\`\`

### Configuration
**Required Environment Variables**:
- `CONFIG_FILE`: Path to configuration YAML (default: `/etc/[service]/config.yaml`)
- `LOG_LEVEL`: Logging level (default: `info`, options: `debug`, `info`, `warn`, `error`)

**Fixed Ports (NOT configurable)**:
> Per [DD-TEST-001](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md), ports are standardized and hardcoded.

- `METRICS_PORT`: `:9090` (Prometheus metrics - hardcoded)
- `HEALTH_PROBE_PORT`: `:8081` (Liveness/readiness probes - hardcoded)
- `SERVER_PORT`: `:8080` (HTTP API - only for stateless HTTP services)

---

## Operational Considerations

### Monitoring
**Prometheus Metrics Endpoint**: `:9090/metrics`

**Key Metrics to Watch**:
- `[service]_operations_total{status="failure"}` - Error rate
- `[service]_operation_duration_seconds` - Latency (P50, P99)
- `[service]_resources_processed_total{result="failure"}` - Processing failures
- `[service]_queue_depth` - Queue backlog (if applicable)

**Alert Recommendations**:
- Error rate > 5% for 5 minutes → Page on-call
- P99 latency > 5s for 10 minutes → Ticket
- Queue depth > 1000 for 15 minutes → Investigate

### Logging
**Log Level**: Configured via `LOG_LEVEL` environment variable

**Key Log Patterns**:
- `"Processing request"` - Normal operation
- `"Transient error, retrying"` - Retry logic triggered
- `"Max retry attempts exceeded"` - Permanent failure
- `"Graceful shutdown initiated"` - Shutdown in progress

**Log Location**: stdout (captured by Kubernetes)

### Health Checks
> **Reference**: [DD-TEST-001](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) for port allocation.

- **Liveness**: `GET :8081/healthz` - Returns 200 if process is alive
- **Readiness**: `GET :8081/readyz` - Returns 200 if ready to serve traffic, 503 if unhealthy
- **Metrics**: `GET :9090/metrics` - Prometheus metrics endpoint

---

## Troubleshooting Guide

### Common Issues

#### Issue 1: Service fails to start
**Symptoms**: Pod in CrashLoopBackOff, logs show configuration error
**Diagnosis**: Check ConfigMap exists and is mounted correctly
**Resolution**:
\`\`\`bash
kubectl describe pod [service]-xxx
kubectl get configmap [service]-config -o yaml
\`\`\`

#### Issue 2: High error rate in metrics
**Symptoms**: `[service]_operations_total{status="failure"}` increasing rapidly
**Diagnosis**: Check logs for error messages, verify external service availability
**Resolution**:
\`\`\`bash
kubectl logs -f deployment/[service] | grep ERROR
# Check external service connectivity
kubectl exec -it [service]-xxx -- curl http://external-service/health
\`\`\`

#### Issue 3: Processing stuck in queue
**Symptoms**: `[service]_queue_depth` growing, resources stuck in Processing phase
**Diagnosis**: Check for resource exhaustion or deadlock
**Resolution**:
\`\`\`bash
# Check resource usage
kubectl top pod [service]-xxx
# Restart if needed
kubectl rollout restart deployment/[service]
\`\`\`

[Full troubleshooting guide: `TROUBLESHOOTING_GUIDE.md`]

---

## Lessons Learned

### What Went Well ✅
1. **Integration-first testing**: Caught architectural issues early (Day 8), saved 2-3 days of rework
2. **Table-driven tests**: Reduced test code by 25-40%, easier to add new scenarios
3. **Error handling philosophy**: Consistent retry/backoff logic across all components
4. **Daily EOD docs**: Smooth handoffs, clear progress tracking

### Challenges Encountered ⚠️
1. **[Challenge 1]**: [Description]
   - **Resolution**: [How it was solved]
   - **Lesson**: [What to do differently next time]

2. **[Challenge 2]**: [Description]
   - **Resolution**: [Solution]
   - **Lesson**: [Takeaway]

### Recommendations for Future Services
1. [Recommendation 1]: [Why this matters]
2. [Recommendation 2]: [Impact]
3. [Recommendation 3]: [Benefit]

---

## Next Steps and Future Work

### Immediate Next Steps (Week 1-2)
- [ ] Monitor production deployment for 72 hours
- [ ] Create Grafana dashboard for key metrics
- [ ] Set up alerts in Prometheus AlertManager
- [ ] Brief on-call team on troubleshooting procedures

### Short-Term Enhancements (Month 1-3)
- [ ] [Enhancement 1]: [Description and priority]
- [ ] [Enhancement 2]: [Description and priority]
- [ ] Performance optimization: [Specific area]

### Long-Term Roadmap (Quarter 2-4)
- [ ] v2.0: [Major feature addition]
- [ ] [Scalability improvement]
- [ ] [Additional integration]

---

## Team and Contacts

**Implementation Team**:
- Lead: [Name] - [Role]
- Developer: [Name] - [Role]
- Reviewer: [Name] - [Role]

**Operational Contacts**:
- On-Call: [Team/Rotation]
- SME: [Name/Team] for [specific area]
- Documentation: [Link to wiki/runbook]

---

## References

- **Business Requirements**: `docs/requirements/BR-[SERVICE]-REQUIREMENTS.md`
- **Implementation Plan**: `docs/services/[service]/implementation/IMPLEMENTATION_PLAN_V3.0.md`
- **Design Decisions**: `docs/services/[service]/implementation/DESIGN_DECISIONS.md`
- **Production Readiness**: `docs/services/[service]/implementation/PRODUCTION_READINESS_REPORT.md`
- **API Documentation**: `docs/services/[service]/README.md#api-reference`
- **Testing Strategy**: `docs/services/[service]/implementation/TESTING_STRATEGY.md`

---

**Handoff Complete**: ✅ [Date]
**Next Review**: [Date for follow-up review]
```

---

### 📚 **Production Runbooks Template** (Day 12 Deliverable)

> **Location**: `docs/services/[service]/runbooks/`
> **Reference**: Notification Controller v3.2 patterns

### **Runbook Template: [Issue Type]**

```markdown
# Runbook: [Issue Name]

**Trigger**: [What condition triggers this runbook]
**Severity**: P1 | P2 | P3 | P4
**SLA**: [Resolution time target]

## Symptoms
- [Observable symptom 1]
- [Observable symptom 2]
- Metric: `[service]_[metric]` above/below threshold

## Investigation Steps

### Step 1: Verify the Issue
\`\`\`bash
# Check metric/logs
kubectl logs -l app=[service]-controller -n kubernaut-system --since=10m | grep -i "error\|fail"
curl -s localhost:9090/metrics | grep [service]_
\`\`\`

### Step 2: Identify Root Cause
\`\`\`bash
# Specific diagnostic commands
kubectl get [resources] -A -o json | jq '[filter]'
\`\`\`

### Step 3: Resolution Options
**Option A: [Quick fix]**
\`\`\`bash
# Commands for quick fix
\`\`\`

**Option B: [If Option A fails]**
\`\`\`bash
# Alternative commands
\`\`\`

## Resolution Verification
\`\`\`bash
# Verify issue is resolved
watch -n 5 'kubectl get [resources] -A | grep [filter]'
\`\`\`

## Escalation
- If unresolved after [X] minutes → Contact [Team/Person]
- If data loss suspected → [Escalation procedure]

## Prevention
- [How to prevent recurrence]
- [Monitoring improvements]
```

### **Required Runbooks for CRD Controllers**
1. **High Failure Rate** (>10% reconciliation failures)
2. **Stuck Resources** (>5min in non-terminal phase)
3. **Policy/Configuration Errors** (user misconfiguration)

---

### 🎯 **Edge Case Categories Template** (Days 9-10)

> **Purpose**: Ensure comprehensive edge case testing
> **Reference**: Notification Controller v3.2 patterns

| Category | Description | Test Pattern |
|----------|-------------|--------------|
| **Configuration Changes** | Config updated during operation | Start operation, update config, verify behavior |
| **Rate Limiting** | External service rate limits | Mock 429 responses, verify backoff |
| **Large Payloads** | Data exceeds normal size | Create large input, verify no OOM |
| **Concurrent Operations** | Race conditions | Parallel requests, verify consistency |
| **Partial Failures** | Some operations succeed, others fail | Mock partial success, verify graceful degradation |
| **Context Cancellation** | Request cancelled mid-operation | Cancel context, verify cleanup |

### **Edge Case Test Pattern**
```go
var _ = Describe("Edge Cases", func() {
    Context("when [edge case condition]", func() {
        BeforeEach(func() {
            // Setup edge case scenario
        })

        It("should [expected behavior]", func() {
            // Trigger edge case
            // Verify expected outcome
        })
    })
})
```

---

### 📊 **Metrics Validation Commands Template** (Day 7)

```bash
# Start service locally (for validation)
go run ./cmd/[service]/main.go \
    --metrics-bind-address=:9090 \
    --health-probe-bind-address=:8081

# Verify metrics endpoint
curl -s localhost:9090/metrics | grep [service]_

# Expected metrics (customize per service):
# [service]_operations_total{status="success",operation="..."} 0
# [service]_operation_duration_seconds_bucket{operation="...",le="1"} 0
# [service]_errors_total{error_type="..."} 0

# Verify health endpoints
curl -s localhost:8081/healthz  # Should return 200
curl -s localhost:8081/readyz   # Should return 200

# Create test resource (for CRD controllers)
kubectl apply -f config/samples/[service]_v1alpha1_[resource].yaml

# Verify metrics increment
watch -n 1 'curl -s localhost:9090/metrics | grep [service]_operations_total'
```

---

### 🚧 **Blockers Section Template**

> **Purpose**: Track blocking issues during implementation
> **Update**: During daily standups and EOD documentation

| ID | Description | Status | Owner | Created | Resolved | Notes |
|----|-------------|--------|-------|---------|----------|-------|
| B-001 | [Description] | 🔴 Blocked | [Name] | [Date] | - | [Details] |
| B-002 | [Description] | 🟡 In Progress | [Name] | [Date] | - | [Details] |
| B-003 | [Description] | ✅ Resolved | [Name] | [Date] | [Date] | [Resolution] |

**Status Legend**:
- 🔴 **Blocked**: Actively blocking progress
- 🟡 **In Progress**: Being worked on
- ✅ **Resolved**: No longer blocking

---

### 📝 **Lessons Learned Template** (Day 12)

> **Purpose**: Capture insights for future implementations
> **Location**: Include in handoff summary or separate document

### **What Worked Well**
1. [Approach/decision that worked]
   - **Evidence**: [How we know it worked]
   - **Recommendation**: [Should we continue/expand this?]

2. [Another success]
   - **Evidence**: [...]
   - **Recommendation**: [...]

### **Technical Wins**
1. [Technical achievement]
   - **Impact**: [Quantifiable impact if possible]

### **Challenges Overcome**
1. [Challenge faced]
   - **Solution**: [How we solved it]
   - **Lesson**: [What we learned]

### **What Would We Do Differently**
1. [Change we would make]
   - **Reason**: [Why this would be better]
   - **Impact**: [Expected improvement]

---

### 🔧 **Technical Debt Template** (Day 12)

> **Purpose**: Track known issues for future resolution
> **Location**: Include in handoff summary

### **Minor Issues (Non-Blocking)**
| Issue | Impact | Estimated Effort | Priority |
|-------|--------|------------------|----------|
| [Description] | [Impact] | [Hours/Days] | P3 |

### **Future Enhancements (Post-V1)**
| Enhancement | Business Value | Estimated Effort | Target Version |
|-------------|---------------|------------------|----------------|
| [Feature] | [Value] | [Effort] | V1.1 |

### **Known Limitations**
1. **[Limitation]**: [Description and workaround if any]

---

### 🤝 **Team Handoff Notes Template** (Day 12)

### **Key Files to Review**
| File | Purpose | Priority |
|------|---------|----------|
| `cmd/[service]/main.go` | Entry point, signal handling | High |
| `internal/controller/[service]/reconciler.go` | Main business logic | High |
| `pkg/[service]/types.go` | Core type definitions | Medium |
| `docs/.../ERROR_HANDLING_PHILOSOPHY.md` | Error handling guide | Medium |

### **Running Locally**
```bash
# Terminal 1: Start dependencies (if any)
[commands for dependencies]

# Terminal 2: Start service
make run-[service]

# Terminal 3: Test
[commands for testing]

# Terminal 4: Watch logs/metrics
[commands for observability]
```

### **Debugging Tips**
```bash
# Common debugging commands
kubectl logs -l app=[service]-controller -n kubernaut-system --tail=100

# Force re-reconciliation (CRD controllers)
kubectl annotate [resource] <name> force-reconcile=$(date +%s) --overwrite

# Check leader election
kubectl get lease [service]-controller-leader -n kubernaut-system -o yaml

# Profile memory/CPU
kubectl top pod -l app=[service]-controller -n kubernaut-system
```

### **Common Issues and Solutions**
| Issue | Symptom | Solution |
|-------|---------|----------|
| [Issue 1] | [Symptom] | [Solution] |
| [Issue 2] | [Symptom] | [Solution] |

---

## Critical Checkpoints (From Gateway Learnings)

### ✅ Checkpoint 1: Parallel Test Execution (Days 8-10)
**Why**: 4x faster feedback with 4 concurrent processes
**Action**: Run all tests with `-p 4` or `-procs=4`, ensure test isolation
**Evidence**: Gateway test suite runs in 25% of original time with parallel execution

### ✅ Checkpoint 2: Schema Validation (Day 7 EOD)
**Why**: Prevents test failures from schema mismatches
**Action**: Validate 100% field alignment before testing
**Evidence**: Gateway added missing CRD fields, avoided test failures

### ✅ Checkpoint 3: BR Coverage Matrix (Day 9 EOD)
**Why**: Ensures all requirements have test coverage
**Action**: Map every BR to tests, justify any skipped
**Evidence**: Gateway achieved 100% BR coverage

### ✅ Checkpoint 4: Production Readiness (Day 12)
**Why**: Reduces production deployment issues
**Action**: Complete comprehensive readiness checklist
**Evidence**: Gateway deployment went smoothly

### ✅ Checkpoint 5: Daily Status Docs (Days 1, 4, 7, 12)
**Why**: Better progress tracking and handoffs
**Action**: Create progress documentation at key milestones
**Evidence**: Gateway handoff was smooth

---

## Documentation Standards

### Daily Status Documents

**Day 1**: `01-day1-complete.md`
- Package structure created
- Types and interfaces defined
- Build successful
- Confidence assessment

**Day 4**: `02-day4-midpoint.md`
- Components completed so far
- Integration status
- Any blockers
- Confidence assessment

**Day 7**: `03-day7-complete.md`
- Core implementation complete
- Server and metrics implemented
- Schema validation complete
- Test infrastructure ready
- Confidence assessment

**Day 12**: `00-HANDOFF-SUMMARY.md`
- Executive summary
- Complete file inventory
- Key decisions
- Lessons learned
- Next steps

### Design Decision Documents

**Pattern**: Create DD-XXX entries for significant decisions

**Template**:
```markdown
## DD-XXX: [Decision Title]

### Status
**[Status Emoji] [Status]** (YYYY-MM-DD)

### Context & Problem
[What problem are we solving?]

### Alternatives Considered
1. **Alternative A**: [Pros/Cons]
2. **Alternative B**: [Pros/Cons]
3. **Alternative C**: [Pros/Cons]

### Decision
**APPROVED: Alternative X**

**Rationale**:
1. [Reason 1]
2. [Reason 2]

### Consequences
**Positive**: [Benefits]
**Negative**: [Trade-offs + Mitigations]
```

---

## Testing Strategy

### Defense-in-Depth Testing Approach

**Reference**: [03-testing-strategy.mdc](.cursor/rules/03-testing-strategy.mdc) (AUTHORITATIVE)

Kubernaut implements a **defense-in-depth testing strategy** with **90% overall confidence**.

**Standard Methodology**: Unit → Integration → E2E

### Test Distribution (From Gateway/Data Storage Success)

| Type | Coverage | Purpose | Mock Strategy |
|------|----------|---------|---------------|
| **Unit** | 70%+ of BRs | Component logic, edge cases | Mock external deps ONLY (K8s API, DB, LLM) |
| **Integration** | >50% of BRs | CRD coordination, real K8s API | Real components + envtest/KIND |
| **E2E** | <10% of BRs | Complete workflows, production-like | Minimal mocking |

### Mock Strategy Decision Matrix (AUTHORITATIVE)

**Core Principle**: Mock ONLY external dependencies. Use REAL business logic.

| Component Type | Unit Tests | Integration | E2E | Justification |
|----------------|------------|-------------|-----|---------------|
| **External AI APIs** (HolmesGPT, OpenAI) | MOCK | MOCK (CI) | REAL | External service |
| **Kubernetes API** | **FAKE CLIENT** ⚠️ | REAL (envtest) | REAL | See K8s mandate below |
| **Database** | MOCK | REAL | REAL | External infrastructure |
| **Business Logic Components** | **REAL** | REAL | REAL | Core business value |
| **Internal Services** | REAL | REAL | REAL | Business logic |

### ⚠️ K8s Client Mandate (MANDATORY)

**AUTHORITATIVE**: All K8s interaction MUST use approved interfaces:

| Test Tier | MANDATORY Interface | Package |
|-----------|---------------------|---------|
| **Unit Tests** | **Fake K8s Client** | `sigs.k8s.io/controller-runtime/pkg/client/fake` |
| **Integration** | Real K8s API (envtest/KIND) | `sigs.k8s.io/controller-runtime/pkg/client` |
| **E2E** | Real K8s API (OCP/KIND) | `sigs.k8s.io/controller-runtime/pkg/client` |

**❌ FORBIDDEN**: Custom `MockK8sClient` implementations
**✅ APPROVED**: `fake.NewClientBuilder()` for all unit tests

**Reference**: [ADR-004: Fake Kubernetes Client](docs/architecture/decisions/ADR-004-fake-kubernetes-client.md)

### Business vs Unit Test Decision

**Reference**: [TESTING_GUIDELINES.md](docs/development/business-requirements/TESTING_GUIDELINES.md)

```
📝 QUESTION: What are you trying to validate?

├─ 💼 "Does it solve the business problem?"
│  └─► BUSINESS REQUIREMENT TEST (maps to BR-XXX-XXX)
│
└─ 🔧 "Does the code work correctly?"
   └─► UNIT TEST (component behavior, edge cases)
```

| Test Purpose | Test Type | Example |
|--------------|-----------|---------|
| User-facing functionality | Business Requirement | "Should reduce alert noise by 80%" |
| Performance SLA compliance | Business Requirement | "Should complete within 30s" |
| Function behavior | Unit Test | "Should detect circular dependencies" |
| Error handling | Unit Test | "Should return error for invalid input" |

### ⚠️ Edge Case Testing Requirements (MANDATORY)

**Reference**: Gateway and Data Storage services have 96+ unit test files including dedicated edge case files.

**Required Edge Case Test Files** (create for each component):
```
test/unit/[service]/
├── [component]_test.go           # Happy path tests
├── [component]_edge_cases_test.go # Edge cases (MANDATORY)
└── [component]_errors_test.go     # Error handling
```

**Edge Case Categories (per Gateway patterns)**:

| Category | Examples | Test Strategy |
|----------|----------|---------------|
| **Empty/Nil Values** | Empty string, nil pointer, zero value | Graceful error with actionable message |
| **Boundary Values** | Min, max, just-over-limit | Validate boundary enforcement |
| **Invalid Input** | Malformed data, wrong types | Clear validation errors |
| **Collision Handling** | Same fingerprint, duplicate keys | Document expected behavior |
| **Concurrent Access** | Race conditions, mutex contention | Use `sync.WaitGroup`, parallel entries |
| **Infrastructure Failures** | Redis down, K8s API timeout, DB connection lost | Graceful degradation |
| **Partial Data** | Some fields missing, incomplete responses | Handle partial enrichment |
| **State Transitions** | Invalid phase transitions, stuck states | Reject invalid, recover stuck |

**Example from Gateway** (`edge_cases_test.go`):
```go
var _ = Describe("BR-001, BR-008: Edge Case Handling", func() {
    Context("Fingerprint Validation Edge Cases", func() {
        It("should reject empty fingerprint with clear error message", func() {
            // BUSINESS OUTCOME: Clear validation error for operators
            // WHY: Empty fingerprint would break deduplication
            signal := &types.NormalizedSignal{
                Fingerprint: "", // Edge case: empty fingerprint
                AlertName:   "TestAlert",
            }
            err := adapter.Validate(signal)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("fingerprint"))
        })
    })
})
```

**Signal Processing Edge Cases (Required)**:

| Component | Edge Cases to Test |
|-----------|-------------------|
| **K8s Enricher** | Namespace not found, Pod terminating, Node NotReady, Owner chain broken, API timeout, RBAC forbidden, Partial enrichment |
| **Environment Classifier** | Missing namespace labels, Conflicting labels, Rego policy syntax error, Policy timeout, Unknown environment |
| **Priority Engine** | Unknown environment, Invalid severity, Confidence below threshold, Override conflicts |
| **Business Classifier** | Multi-label conflicts, Zero confidence, Unknown category, Rego evaluation failure |
| **Audit Client** | Buffer overflow, Data Storage timeout, Fire-and-forget failure, Malformed event |
| **Reconciler** | Invalid phase transition, Stuck in phase, Finalizer cleanup failure, Requeue exhaustion |

**Minimum Edge Case Tests per Component**: 8-12 dedicated edge case scenarios

### 📊 Realistic Test Counts (From Production Services)

**Reference**: Actual test counts from Gateway and Data Storage services (verified 2025-11-28)

| Service | Unit Tests | Integration Tests | Total |
|---------|------------|-------------------|-------|
| **Gateway** | **275** | **143** | 418 |
| **Data Storage** | **392** | **160** | 552 |
| **Notification** | ~50 | ~30 | ~80 |

**Expected Test Counts for New Services**:

| Service Type | Unit Tests | Integration Tests | E2E Tests |
|--------------|------------|-------------------|-----------|
| **CRD Controller** (simple) | 80-120 | 40-60 | 5-10 |
| **CRD Controller** (complex) | 150-250 | 80-120 | 10-20 |
| **Stateless Service** (simple) | 100-150 | 50-80 | 5-10 |
| **Stateless Service** (complex) | 250-400 | 100-160 | 15-25 |

**Signal Processing Expected** (CRD Controller, medium complexity):
- Unit Tests: **100-150** (not 10)
- Integration Tests: **50-80** (not 5)
- E2E Tests: **5-10**

**Test Distribution by Component**:

| Component | Happy Path | Edge Cases | Error Handling | Total |
|-----------|------------|------------|----------------|-------|
| K8s Enricher | 5-8 | 12-15 | 8-10 | 25-33 |
| Environment Classifier | 5-8 | 10-12 | 6-8 | 21-28 |
| Priority Engine | 5-8 | 10-12 | 6-8 | 21-28 |
| Business Classifier | 5-8 | 8-10 | 5-7 | 18-25 |
| Audit Client | 3-5 | 6-8 | 5-7 | 14-20 |
| Reconciler | 8-12 | 10-15 | 8-12 | 26-39 |
| **Total** | 31-49 | 56-72 | 38-52 | **125-173** |

### Standard Test Order (Unit → Integration → E2E) ✅

**MANDATORY**: Follow standard testing methodology in this order:

```
Day 8: Unit tests - all components (100-150 tests)
Day 9: Integration tests - CRD reconciliation with envtest (50-80 tests)
Day 10: E2E tests - full workflow validation (5-10 tests)
```

**Why This Order**:
- Unit tests validate component logic in isolation (fastest feedback)
- Integration tests validate component interactions with real K8s API
- E2E tests validate complete business workflows
- Follows defense-in-depth testing approach

### ⚡ Parallel Test Execution (MANDATORY)

**Standard**: **4 concurrent processes** for all test tiers.

**Configuration**:
```bash
# Unit tests - parallel by default
go test -v -p 4 ./test/unit/[service]/...

# Integration tests - parallel with shared envtest
go test -v -p 4 ./test/integration/[service]/...

# E2E tests - parallel with isolated namespaces
go test -v -p 4 ./test/e2e/[service]/...

# Ginkgo parallel execution
ginkgo -p -procs=4 ./test/unit/[service]/...
ginkgo -p -procs=4 ./test/integration/[service]/...
ginkgo -p -procs=4 ./test/e2e/[service]/...
```

**Parallel Test Requirements**:
| Tier | Isolation Strategy | Shared Resources | Port Allocation |
|------|-------------------|------------------|-----------------|
| **Unit** | No shared state between tests | Mock clients | N/A |
| **Integration** | Unique namespace per test | Shared envtest API server | Per DD-TEST-001 |
| **E2E** | Unique namespace per test | Shared cluster | Per DD-TEST-001 |

**Test Isolation Patterns**:
```go
// Integration/E2E: Generate unique namespace per test
var _ = Describe("Component", func() {
    var testNamespace string

    BeforeEach(func() {
        // Unique namespace enables parallel execution
        testNamespace = fmt.Sprintf("test-%s", uuid.New().String()[:8])
        Expect(k8sClient.Create(ctx, &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
        })).To(Succeed())
    })

    AfterEach(func() {
        // Cleanup after test
        Expect(k8sClient.Delete(ctx, &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
        })).To(Succeed())
    })
})
```

**⚠️ Parallel Test Anti-Patterns** (AVOID):
- ❌ Hardcoded namespace names (`test-namespace`)
- ❌ Shared mutable state between tests
- ❌ Fixed port numbers without DD-TEST-001 allocation
- ❌ Tests that depend on execution order
- ❌ Global variables modified during tests

### Table-Driven Testing Pattern ⭐ (RECOMMENDED)

**Why Table-Driven Tests?**
Based on Dynamic Toolset Service implementation:
- **38% code reduction** in test files (1,612 lines → 1,001 lines)
- **25-40% faster** to add new test cases
- **Better maintainability**: Change logic once, all entries benefit
- **Clearer coverage**: Easy to see all scenarios at a glance

**Implementation Pattern**:

#### Pattern 1: Success Scenarios
```go
DescribeTable("should detect [Service] services",
    func(name, namespace string, labels map[string]string, ports []corev1.ServicePort, expectedEndpoint string) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:      name,
                Namespace: namespace,
                Labels:    labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).ToNot(BeNil())
        Expect(result.Endpoint).To(Equal(expectedEndpoint))
    },
    Entry("with standard label", "svc-1", "ns-1",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{{Port: 8080}},
        "http://svc-1.ns-1.svc.cluster.local:8080"),
    Entry("with name-based detection", "myapp-server", "ns-2",
        nil,
        []corev1.ServicePort{{Port: 8080}},
        "http://myapp-server.ns-2.svc.cluster.local:8080"),
    // Easy to add more - just add Entry!
)
```

#### Pattern 2: Negative Scenarios
```go
DescribeTable("should NOT detect non-matching services",
    func(name string, labels map[string]string, ports []corev1.ServicePort) {
        service := &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:   name,
                Labels: labels,
            },
            Spec: corev1.ServiceSpec{Ports: ports},
        }

        result, err := detector.Detect(ctx, service)
        Expect(err).ToNot(HaveOccurred())
        Expect(result).To(BeNil())
    },
    Entry("for different service type", "other-svc",
        map[string]string{"app": "other"},
        []corev1.ServicePort{{Port: 9090}}),
    Entry("for service without ports", "no-ports",
        map[string]string{"app": "myapp"},
        []corev1.ServicePort{}),
)
```

#### Pattern 3: Health Check Scenarios
```go
DescribeTable("should validate health status",
    func(statusCode int, body string, expectSuccess bool) {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(statusCode)
            w.Write([]byte(body))
        }))
        defer server.Close()

        err := checker.HealthCheck(ctx, server.URL)
        if expectSuccess {
            Expect(err).ToNot(HaveOccurred())
        } else {
            Expect(err).To(HaveOccurred())
        }
    },
    Entry("with 200 OK", http.StatusOK, "", true),
    Entry("with 204 No Content", http.StatusNoContent, "", true),
    Entry("with 503 Unavailable", http.StatusServiceUnavailable, "", false),
)
```

#### Pattern 4: Setup Functions for Complex Cases
```go
DescribeTable("should handle error conditions",
    func(setupServer func() string) {
        endpoint := setupServer()
        err := component.Process(endpoint)
        Expect(err).To(HaveOccurred())
    },
    Entry("for connection refused", func() string {
        return "http://localhost:9999"
    }),
    Entry("for timeout", func() string {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            time.Sleep(10 * time.Second)
        }))
        DeferCleanup(server.Close)
        return server.URL
    }),
    Entry("for invalid URL", func() string {
        return "not-a-valid-url"
    }),
)
```

**Best Practices**:
1. Use descriptive Entry names that document the scenario
2. Keep table logic simple and consistent
3. Use traditional It() for truly unique scenarios
4. Group related scenarios in same DescribeTable
5. Use DeferCleanup for resource cleanup in Entry setup functions

**Reference Examples**:
- Excellent examples in `test/unit/toolset/*_detector_test.go`
- 73 tests consolidated from 77, 38% less code
- All tests passing with 100% coverage maintained

### ⚠️ CRD Types Testing Guidance (Anti-Pattern Alert)

**DO NOT create `types_test.go` for CRD struct field validation.** This is a null-testing anti-pattern.

**Why Not?**
| Concern | Who Handles It | Not Your Test |
|---------|----------------|---------------|
| Field exists | **Go compiler** | Compile-time error if missing |
| Field type correct | **Go compiler** | Type mismatch = compilation failure |
| Required vs Optional | **OpenAPI schema + kubebuilder** | API validation at apply time |
| Default values | **kubebuilder `+kubebuilder:default`** | CRD webhook validation |
| Phase transitions | **Controller business logic** | Controller tests cover this |

**Anti-Pattern Example** (DON'T DO THIS):
```go
// ❌ ZERO business value - testing Go compiler and own test setup
sp := &SignalProcessing{}
Expect(sp.Spec.Signal).To(BeZero())     // Proves nothing
Expect(sp.Status.Phase).To(BeEmpty())   // Go already guarantees this
Expect(sp.Status.Context).To(BeNil())   // Null-testing anti-pattern
```

**Where CRD Behavior IS Tested**:
| Test Type | Location | Tests |
|-----------|----------|-------|
| Controller unit tests | `test/unit/[service]/` | Phase transitions, enrichment logic |
| Integration tests | `test/integration/[service]/` | Full reconciliation with envtest |
| E2E tests | `test/e2e/` | Cross-service CRD workflows |
| Schema validation | `config/crd/` | OpenAPI validation at apply time |

**Reference**: Signal Processing implementation learned this from Gateway patterns.

### Test Naming Convention

```go
// Business requirement reference in test description
Describe("BR-[CATEGORY]-XXX: [Requirement]", func() {
    // Prefer table-driven tests for multiple scenarios
    DescribeTable("should [behavior]",
        func(params...) {
            // Test logic
        },
        Entry("scenario 1", ...),
        Entry("scenario 2", ...),
    )

    // Use traditional It() for unique scenarios
    Context("when [unique condition]", func() {
        It("should [expected behavior]", func() {
            // Test implementation
        })
    })
})
```

---

## Performance Targets

Define service-specific targets:

| Metric | Target | Measurement |
|--------|--------|-------------|
| API Latency (p95) | < Xms | HTTP request duration |
| API Latency (p99) | < Yms | HTTP request duration |
| Throughput | > Z req/s | Requests per second |
| Memory Usage | < XMB | Per replica |
| CPU Usage | < X cores | Average |
| [Service-specific] | [Target] | [How measured] |

---

## Common Pitfalls to Avoid

### ❌ Don't Do This:
1. **Skip integration tests until end**: Costs 2+ days in debugging
2. **Write all unit tests first**: Wastes time on wrong details
3. **Skip schema validation**: Causes test failures later
4. **No daily status docs**: Makes handoffs difficult
5. **Skip BR coverage matrix**: Results in untested requirements
6. **No production readiness check**: Causes deployment issues
7. **Repetitive test code**: Copy-paste It blocks for similar scenarios
8. **No table-driven tests**: Results in 25-40% more code

### ✅ Do This Instead:
1. **Integration-first testing**: Validates architecture early
2. **5 critical integration tests Day 8**: Proves core functionality
3. **Schema validation Day 7**: Prevents test failures
4. **Daily progress docs**: Smooth handoffs and communication
5. **BR coverage matrix Day 9**: Ensures 100% requirement coverage
6. **Production checklist Day 12**: Smooth deployment
7. **Table-driven tests**: Use DescribeTable for multiple similar scenarios ⭐
8. **DRY test code**: Extract common test logic, parameterize with Entry

---

## Success Criteria

### Implementation Complete When:
- [ ] All business requirements implemented
- [ ] Build passes without errors
- [ ] Zero lint errors
- [ ] Unit test coverage > 70%
- [ ] Integration test coverage > 50%
- [ ] E2E tests passing
- [ ] All metrics exposed
- [ ] Health checks functional
- [ ] Documentation complete
- [ ] Production readiness validated

### Quality Indicators:
- **Code Quality**: No lint errors, follows Go idioms
- **Test Quality**: BDD style, clear assertions, business requirement references
- **Test Organization**: Table-driven tests for similar scenarios, 25-40% less test code
- **Test Maintainability**: Easy to add new cases (just add Entry), consistent patterns
- **Documentation Quality**: Complete, accurate, helpful
- **Production Readiness**: Deployment manifests complete, observability comprehensive

---

## Makefile Targets

Create consistent development commands:

```makefile
# Testing (with parallel execution - 4 concurrent processes standard)
.PHONY: test-unit-[service]
test-unit-[service]:
	go test -v -p 4 ./test/unit/[service]/...

.PHONY: test-integration-[service]
test-integration-[service]:
	go test -v -p 4 ./test/integration/[service]/...

.PHONY: test-e2e-[service]
test-e2e-[service]:
	go test -v -p 4 ./test/e2e/[service]/...

# Testing with Ginkgo (preferred - parallel with 4 procs)
.PHONY: test-unit-ginkgo-[service]
test-unit-ginkgo-[service]:
	ginkgo -p -procs=4 -v ./test/unit/[service]/...

.PHONY: test-integration-ginkgo-[service]
test-integration-ginkgo-[service]:
	ginkgo -p -procs=4 -v ./test/integration/[service]/...

.PHONY: test-e2e-ginkgo-[service]
test-e2e-ginkgo-[service]:
	ginkgo -p -procs=4 -v ./test/e2e/[service]/...

# All tests with parallel execution
.PHONY: test-all-[service]
test-all-[service]:
	ginkgo -p -procs=4 -v ./test/unit/[service]/... ./test/integration/[service]/... ./test/e2e/[service]/...

# Coverage
.PHONY: test-coverage-[service]
test-coverage-[service]:
	go test -cover -coverprofile=coverage.out -p 4 ./pkg/[service]/...
	go tool cover -html=coverage.out

# Build
.PHONY: build-[service]
build-[service]:
	go build -o bin/[service] ./cmd/[service]

# Linting
.PHONY: lint-[service]
lint-[service]:
	golangci-lint run ./pkg/[service]/... ./cmd/[service]/...

# Deployment
.PHONY: deploy-kind-[service]
deploy-kind-[service]:
	kubectl apply -f deploy/[service]/
```

---

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2025-10-11 | Initial template based on Gateway + Dynamic Toolset learnings | AI Assistant |

---

## 📚 Appendix A: Complete EOD Documentation Templates ⭐ V2.0

### Day 1 Complete Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/01-day1-complete.md`

```markdown
# Day 1 Complete: Foundation Setup

**Date**: YYYY-MM-DD
**Phase**: Foundation
**Status**: ✅ Complete
**Confidence**: 90%

---

## ✅ Completed Tasks

### Package Structure
- [x] Controller/Service skeleton created
- [x] Business logic packages created (`pkg/[service]/`)
- [x] Test directories established (`test/unit/`, `test/integration/`, `test/e2e/`)
- [x] Deployment directory created (`deploy/[service]/`)

### Foundational Files
- [x] Main application entry point (`cmd/[service]/main.go`)
- [x] Core type definitions (`pkg/[service]/types.go`)
- [x] Primary interfaces defined
- [x] [List specific files created]

### Build Validation
- [x] Code compiles successfully (`go build ./cmd/[service]/`)
- [x] Zero lint errors (`golangci-lint run`)
- [x] Imports resolve correctly
- [x] CRD manifests generated (if applicable)

---

## 🏗️ Architecture Decisions

### Decision 1: [Decision Name]
- **Chosen Approach**: [What was decided]
- **Rationale**: [Why this approach]
- **Alternatives Considered**: [What other options were evaluated]
- **Impact**: [How this affects implementation]

### Decision 2: [Decision Name]
[Repeat pattern]

---

## 📊 Progress Metrics

- **Hours Spent**: 8h
- **Files Created**: [X] files
- **Lines of Code**: ~[Y] lines (skeleton)
- **Tests Written**: 0 (foundation only)

---

## 🚧 Known Issues / Blockers

### Issue 1: [Issue Description]
- **Status**: [Resolved / In Progress / Blocked]
- **Impact**: [High / Medium / Low]
- **Resolution Plan**: [How it will be addressed]

**Current Status**: No blockers ✅

---

## 📝 Next Steps (Day 2)

### Immediate Priorities
1. [Priority 1 - specific task]
2. [Priority 2 - specific task]
3. [Priority 3 - specific task]

### Success Criteria for Day 2
- [ ] [Specific deliverable 1]
- [ ] [Specific deliverable 2]
- [ ] [Specific deliverable 3]

---

## 🎯 Confidence Assessment

**Overall Confidence**: 90%

**Evidence**:
- Foundation is solid and follows established patterns
- All skeleton code compiles without errors
- Package structure aligns with project standards
- Architecture decisions validated against business requirements

**Risks**:
- None identified at foundation stage

**Mitigation**:
- N/A

---

**Prepared By**: [Name]
**Reviewed By**: [Name] (if applicable)
**Next Review**: End of Day 2
```

---

### Day 4 Midpoint Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/02-day4-midpoint.md`

```markdown
# Day 4 Complete: Midpoint Review

**Date**: YYYY-MM-DD
**Phase**: Core Implementation (Midpoint)
**Status**: ✅ 50% Complete
**Confidence**: 85%

---

## ✅ Completed Components (Days 2-4)

### Component 1: [Component Name]
- [x] Unit tests written (RED phase) - BR-XXX-001, BR-XXX-002
- [x] Implementation complete (GREEN phase)
- [x] Refactored for production quality
- **Test Coverage**: [X]% (target >70%)
- **BR Coverage**: BR-XXX-001 ✅, BR-XXX-002 ✅

### Component 2: [Component Name]
[Repeat pattern for each completed component]

---

## 🧪 Testing Summary

### Unit Tests
- **Total Tests Written**: [X] tests
- **Tests Passing**: [Y]/[X] (Z% passing)
- **Coverage**: [Coverage %] (target >70%)
- **Table-Driven Tests**: [N] DescribeTable blocks

### Business Requirement Coverage
- **Total BRs**: [N] requirements
- **BRs Tested**: [M]/[N] ([%] coverage)
- **Remaining BRs**: [N-M] (to be covered Days 5-6)

---

## 🏗️ Architecture Refinements

### Refinement 1: [Refinement Description]
- **Reason**: [Why refinement was needed]
- **Change**: [What was changed]
- **Impact**: [How it improves design]

**Total Refinements**: [N] (expected at midpoint)

---

## 🚧 Current Blockers

### Blocker 1: [Blocker Description]
- **Discovered**: Day [X]
- **Impact**: [High / Medium / Low]
- **Resolution Plan**: [Specific plan]
- **Expected Resolution**: Day [Y]

**Current Status**: [No blockers / X blockers identified]

---

## 📊 Progress Metrics

### Velocity
- **Days Elapsed**: 4/12 (33%)
- **Components Complete**: [X]/[Y] ([Z]%)
- **On Track**: [Yes / No / At Risk]

### Code Quality
- **Lint Errors**: 0 ✅
- **Build Errors**: 0 ✅
- **Test Failures**: [N] (should be 0)

---

## 📝 Remaining Work (Days 5-7)

### Day 5 Priorities
1. [Component X] - BR-XXX-XXX
2. [Component Y] - BR-XXX-XXX
3. [Component Z] - BR-XXX-XXX

### Day 6 Priorities
1. [Error handling philosophy doc]
2. [Final component]
3. [Code refactoring]

### Day 7 Priorities
1. [Server integration]
2. [Metrics implementation]
3. [Health checks]

---

## 🎯 Confidence Assessment

**Midpoint Confidence**: 85%

**Evidence**:
- [X] components complete with passing tests
- Test coverage at [Y]% (target >70%)
- Architecture decisions validated through implementation
- No critical blockers identified

**Risks**:
- [Risk 1]: [Description]
  - **Mitigation**: [How it will be addressed]
- [Risk 2]: [Description]
  - **Mitigation**: [How it will be addressed]

**Adjustment Plan**:
- [Any timeline or scope adjustments needed]

---

**Status**: ✅ On Track for Day 12 Completion
**Next Checkpoint**: Day 7 EOD (Integration Ready)
```

---

### Day 7 Complete Template

**File**: `docs/services/[service-type]/[service-name]/implementation/phase0/03-day7-complete.md`

```markdown
# Day 7 Complete: Integration Ready

**Date**: YYYY-MM-DD
**Phase**: Core Implementation Complete + Integration
**Status**: ✅ Integration Ready
**Confidence**: 92%

---

## ✅ Core Implementation Complete

### All Components Implemented
- [x] Component 1: [Name] - BR-XXX-001, BR-XXX-002
- [x] Component 2: [Name] - BR-XXX-003, BR-XXX-004
- [x] Component 3: [Name] - BR-XXX-005, BR-XXX-006
- [x] Component 4: [Name] - BR-XXX-007
- [x] Component 5: [Name] - BR-XXX-008, BR-XXX-009

**Total Components**: [X]/[X] (100% complete) ✅

---

## 🔗 Integration Complete

### Server Implementation
- [x] HTTP server struct created
- [x] Route registration complete
- [x] Middleware stack implemented
- [x] Health endpoints functional (`/healthz`, `/readyz`)
- **Ports**: 8080 (API - HTTP services only), 8081 (health probes), 9090 (metrics)

### Main Application Wiring
- [x] All components wired in `main.go`
- [x] Configuration loading implemented
- [x] Graceful shutdown handling
- [x] Manager setup (for CRD controllers)

### Metrics Implementation
- [x] Business-critical Prometheus metrics defined
- [x] Metric recording in business logic
- [x] Metrics endpoint exposed (`:9090/metrics`)
- **Metrics Count**: [X] metrics (target: 10+) ✅

**Example Metrics**:
```
[service]_operations_total{operation="create",status="success"} 142
[service]_operations_duration_seconds{operation="create",quantile="0.95"} 0.234
[service]_errors_total{error_type="transient"} 5
```

---

## 🧪 Test Infrastructure Ready

### Schema Validation Complete
- [x] All CRD fields validated (if applicable)
- [x] API schemas validated
- [x] No field mismatches identified
- **Validation Document**: `design/01-[schema]-validation.md` ✅

### Test Suite Skeleton
- [x] Integration test suite created
- [x] Kind cluster setup validated (if applicable)
- [x] Test infrastructure imports verified
- [x] BeforeSuite/AfterSuite scaffolding complete

**Integration Test Files**:
- `test/integration/[service]/suite_test.go` ✅
- `test/integration/[service]/[scenario]_test.go` (placeholder) ✅

---

## 📋 Documentation Complete

### Error Handling Philosophy
- [x] Complete error handling document created
- [x] Error classification defined (transient/permanent/user)
- [x] Retry strategy documented
- [x] Circuit breaker patterns included
- **Document**: `design/ERROR_HANDLING_PHILOSOPHY.md` ✅

### Testing Strategy
- [x] Testing approach documented
- [x] Integration-first rationale explained
- [x] Test environment decision documented
- **Document**: `testing/01-integration-first-rationale.md` ✅

---

## 📊 Progress Metrics

### Implementation Progress
- **Days Elapsed**: 7/12 (58%)
- **Components Complete**: [X]/[X] (100%) ✅
- **Integration**: Complete ✅
- **Metrics**: [Y] metrics implemented (target 10+) ✅

### Code Quality
- **Build Status**: ✅ Passing
- **Lint Errors**: 0 ✅
- **Test Coverage**: [Z]% (preliminary, full validation Day 9)
- **BR Coverage**: [N]/[M] BRs implemented ([%]%)

---

## 🚧 Remaining Work (Days 8-12)

### Day 8: Unit Tests (Parallel: 4 procs)
- [ ] All component unit tests (`ginkgo -p -procs=4`)
- [ ] Table-driven tests for similar scenarios
- [ ] Edge case coverage

### Day 9: Integration Tests (Parallel: 4 procs)
- [ ] CRD reconciliation tests with envtest
- [ ] BR Coverage Matrix
- [ ] Test isolation (unique namespace per test)

### Day 10: E2E Tests (Parallel: 4 procs)
- [ ] E2E test scenarios
- [ ] Production environment setup

### Days 11-12: Documentation + Production Readiness
- [ ] Complete documentation
- [ ] Production readiness checklist
- [ ] Handoff summary

---

## 🎯 Confidence Assessment

**Day 7 Confidence**: 92%

**Evidence**:
- All components implemented with passing unit tests
- Server integration complete and functional
- Business-critical Prometheus metrics implemented and tested
- Health checks operational
- Error handling philosophy documented
- Test infrastructure validated

**Risks**:
- Integration test complexity (Day 8) - **Medium**
  - **Mitigation**: Use Kind cluster template, start with 5 critical tests
- BR coverage validation (Day 9) - **Low**
  - **Mitigation**: BR coverage matrix template prepared

**Status**: ✅ **Ready for Integration Testing Phase**

---

**Next Milestone**: Day 8 EOD - Integration Tests Complete
**Expected Confidence After Day 9**: 95%+
```

---

## 📚 Appendix B: CRD Controller Variant ⭐ V2.0

**When to Use**: If your service is a CRD controller (5 out of 12 V1 services are CRD controllers)

**CRD Controllers in V1**:
- SignalProcessing (renamed from RemediationProcessor)
- AIAnalysis
- WorkflowExecution
- KubernetesExecutor (DEPRECATED - ADR-025)
- RemediationOrchestrator

---

### 🔷 **CRD API Group Standard** ⭐ V3.0 NEW

**Reference**: [DD-CRD-001: API Group Domain Selection](../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)

All Kubernaut CRDs use the **`.ai` domain** for AIOps branding:

```yaml
apiVersion: [servicename].kubernaut.ai/v1alpha1
kind: [ServiceName]
```

**Decision Rationale** (per DD-CRD-001):
1. **K8sGPT Precedent**: AI K8s projects use `.ai` (e.g., `core.k8sgpt.ai`)
2. **Brand Alignment**: AIOps is the core value proposition - domain reflects this
3. **Differentiation**: Stands out from traditional infrastructure tooling (`.io`)
4. **Industry Trend**: AI-native platforms increasingly adopt `.ai`

**Note**: Label keys still use `kubernaut.io/` prefix (K8s label convention, not CRD API group).

#### **Industry Best Practices Analysis**

| Project | API Group Strategy | Pattern |
|---------|-------------------|---------|
| **Tekton** | `tekton.dev/v1` | ✅ Unified - all CRDs under single domain |
| **Istio** | `istio.io/v1` | ✅ Unified - network, security, config all under `istio.io` |
| **Cert-Manager** | `cert-manager.io/v1` | ✅ Unified - certificates, issuers, challenges |
| **ArgoCD** | `argoproj.io/v1alpha1` | ✅ Unified - applications, projects, rollouts |
| **Crossplane** | `crossplane.io/v1` | ✅ Unified - compositions, providers |
| **Knative** | Multiple: `serving.knative.dev`, `eventing.knative.dev` | ⚠️ Split by domain |

**Conclusion**: 5/6 major CNCF projects use unified API groups. Splitting is only justified when:
- Projects have **distinct product lines** (Knative Serving vs Eventing)
- Projects have **independent release cycles**
- Projects may be **deployed separately**

Kubernaut's CRD controllers are **tightly coupled** in a single remediation workflow, making unified grouping the correct choice.

#### **CRD Inventory (Unified API Group)**

| CRD | API Group | Purpose |
|-----|-----------|---------|
| SignalProcessing | `signalprocessing.kubernaut.ai/v1alpha1` | Context enrichment, classification |
| AIAnalysis | `kubernaut.ai/v1alpha1` | HolmesGPT RCA + workflow selection |
| WorkflowExecution | `kubernaut.ai/v1alpha1` | Ansible/K8s workflow execution |
| RemediationRequest | `remediation.kubernaut.ai/v1alpha1` | User-facing remediation entry point |

#### **RBAC Template for CRD Controllers**

```yaml
# kubebuilder markers for CRD controller
//+kubebuilder:rbac:groups=[servicename].kubernaut.ai,resources=[resources],verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=[servicename].kubernaut.ai,resources=[resources]/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=[servicename].kubernaut.ai,resources=[resources]/finalizers,verbs=update
```

---

### CRD Controller-Specific Patterns

#### 1. Reconciliation Loop Pattern

**Standard Controller Structure**:
```go
package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	[servicev1alpha1] "github.com/jordigilh/kubernaut/api/[service]/v1alpha1"
)

// Reconciler reconciles a [Resource] object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources],verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources]/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=[service].kubernaut.ai,resources=[resources]/finalizers,verbs=update

// Reconcile implements the reconciliation loop
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// 1. FETCH RESOURCE
	resource := &[servicev1alpha1].[Resource]{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource deleted, nothing to do
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// 2. CHECK TERMINAL STATES
	if resource.Status.Phase == [servicev1alpha1].[PhaseComplete] {
		log.Info("Resource already complete", "name", resource.Name)
		return ctrl.Result{}, nil
	}

	// 3. INITIALIZE STATUS
	if resource.Status.Phase == "" || resource.Status.Phase == [servicev1alpha1].[PhasePending] {
		resource.Status.Phase = [servicev1alpha1].[PhaseProcessing]
		now := metav1.Now()
		resource.Status.StartTime = &now

		if err := r.Status().Update(ctx, resource); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
	}

	// 4. BUSINESS LOGIC
	result, err := r.processResource(ctx, resource)
	if err != nil {
		// Update status with error
		resource.Status.Phase = [servicev1alpha1].[PhaseFailed]
		resource.Status.Error = err.Error()
		r.Status().Update(ctx, resource)

		// Determine if error is retryable
		if isRetryable(err) {
			backoff := calculateBackoff(resource.Status.AttemptCount)
			return ctrl.Result{RequeueAfter: backoff}, nil
		}

		return ctrl.Result{}, err
	}

	// 5. UPDATE STATUS ON SUCCESS
	return r.updateStatusAndRequeue(ctx, resource, result)
}

// SetupWithManager sets up the controller with the Manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&[servicev1alpha1].[Resource]{}).
		Complete(r)
}
```

---

#### 2. Status Update Patterns

**Complete Status Update with Conditions**:
```go
// updateStatusAndRequeue updates resource status and determines requeue strategy
func (r *Reconciler) updateStatusAndRequeue(ctx context.Context, resource *Resource, result *ProcessResult) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Update phase
	resource.Status.Phase = PhaseComplete
	now := metav1.Now()
	resource.Status.CompletionTime = &now
	resource.Status.ObservedGeneration = resource.Generation

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: resource.Generation,
		LastTransitionTime: now,
		Reason:             "ProcessingComplete",
		Message:            "Resource processed successfully",
	}
	meta.SetStatusCondition(&resource.Status.Conditions, condition)

	// Update status subresource
	if err := r.Status().Update(ctx, resource); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Resource processing complete", "name", resource.Name)
	return ctrl.Result{}, nil
}
```

---

#### 3. Finalizer Pattern

**Finalizer Implementation for Cleanup**:
```go
const finalizerName = "[service].kubernaut.ai/finalizer"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	resource := &Resource{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if resource is being deleted
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(resource, finalizerName) {
			// Perform cleanup
			if err := r.cleanupExternalResources(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(resource, finalizerName)
			if err := r.Update(ctx, resource); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(resource, finalizerName) {
		controllerutil.AddFinalizer(resource, finalizerName)
		if err := r.Update(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Continue with reconciliation...
	return ctrl.Result{}, nil
}
```

---

#### 4. Exponential Backoff Requeue

**Production-Ready Backoff Implementation**:
```go
// calculateBackoff returns exponential backoff duration
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func calculateBackoff(attemptCount int) time.Duration {
	baseDelay := 30 * time.Second
	maxDelay := 480 * time.Second

	// Calculate exponential backoff: baseDelay * 2^attemptCount
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))

	// Cap at maximum delay
	if delay > maxDelay {
		delay = maxDelay
	}

	// Add jitter (±10%) to prevent thundering herd
	jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))

	return jitter
}

// Usage in Reconcile
if isRetryable(err) {
	resource.Status.AttemptCount++
	backoff := calculateBackoff(resource.Status.AttemptCount)
	log.Info("Transient error, requeueing", "attempt", resource.Status.AttemptCount, "backoff", backoff)
	return ctrl.Result{RequeueAfter: backoff}, nil
}
```

---

#### 5. Phase State Machine Pattern

**Common CRD Phase Transitions**:
```go
// Phase definitions
const (
	PhasePending    Phase = "Pending"
	PhaseProcessing Phase = "Processing"
	PhaseComplete   Phase = "Complete"
	PhaseFailed     Phase = "Failed"
)

// Phase transition validation
func (r *Reconciler) validatePhaseTransition(current, next Phase) error {
	validTransitions := map[Phase][]Phase{
		PhasePending:    {PhaseProcessing},
		PhaseProcessing: {PhaseComplete, PhaseFailed},
		PhaseComplete:   {}, // Terminal state
		PhaseFailed:     {}, // Terminal state
	}

	allowed, ok := validTransitions[current]
	if !ok {
		return fmt.Errorf("unknown current phase: %s", current)
	}

	for _, validNext := range allowed {
		if validNext == next {
			return nil
		}
	}

	return fmt.Errorf("invalid phase transition: %s → %s", current, next)
}
```

---

#### 6. CRD Testing Patterns

**Fake Client Testing for Controllers**:
```go
var _ = Describe("Controller Tests", func() {
	var (
		ctx        context.Context
		reconciler *Reconciler
		scheme     *runtime.Scheme
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = [service]v1alpha1.AddToScheme(scheme)

		// Create fake client with test resources
		resource := &[service]v1alpha1.[Resource]{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-resource",
				Namespace: "default",
			},
			Spec: [service]v1alpha1.[Resource]Spec{
				// Test spec
			},
		}

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(resource).
			WithStatusSubresource(&[service]v1alpha1.[Resource]{}).
			Build()

		reconciler = &Reconciler{
			Client: fakeClient,
			Scheme: scheme,
		}
	})

	It("should transition from Pending to Processing", func() {
		result, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test-resource",
				Namespace: "default",
			},
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result.Requeue).To(BeFalse())

		// Verify status updated
		updated := &[service]v1alpha1.[Resource]{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-resource", Namespace: "default"}, updated)
		Expect(err).ToNot(HaveOccurred())
		Expect(updated.Status.Phase).To(Equal([service]v1alpha1.PhaseProcessing))
	})
})
```

---

### CRD Controller Checklist

Use this checklist when implementing CRD controllers:

- [ ] **Reconciliation Loop**: Standard pattern with fetch → validate → process → update
- [ ] **Status Updates**: Status subresource updates with conditions
- [ ] **Finalizers**: Cleanup logic implemented for resource deletion
- [ ] **Exponential Backoff**: Transient error retry with jitter
- [ ] **Phase State Machine**: Valid phase transitions enforced
- [ ] **RBAC Annotations**: Kubebuilder RBAC markers complete
- [ ] **Fake Client Tests**: Unit tests use fake.NewClientBuilder()
- [ ] **Integration Tests**: Kind cluster tests with real CRDs
- [ ] **Manager Setup**: SetupWithManager() implemented
- [ ] **Metrics**: Controller-specific metrics (reconciliations, errors, duration)

---

## 📚 Appendix C: Confidence Assessment Methodology ⭐ V2.0

**Purpose**: Evidence-based methodology for calculating implementation plan confidence

---

### Confidence Assessment Framework

**Confidence Range**: 60% (minimum viable) to 100% (perfect certainty)

**Target Confidence**: 90-95% for production-ready implementations

---

### Calculation Formula

```
Confidence = (Implementation Accuracy × 0.30) +
             (Test Coverage × 0.25) +
             (BR Coverage × 0.20) +
             (Production Readiness × 0.15) +
             (Documentation Quality × 0.10)
```

**Component Breakdown**:

1. **Implementation Accuracy (30% weight)**: How well does the implementation match specifications?
2. **Test Coverage (25% weight)**: How comprehensive is the test suite?
3. **BR Coverage (20% weight)**: Percentage of business requirements implemented
4. **Production Readiness (15% weight)**: Deployment and operational readiness
5. **Documentation Quality (10% weight)**: Completeness and accuracy of documentation

---

### Component Scoring Methodology

#### 1. Implementation Accuracy (30% weight)

**Score Calculation**:
```
Implementation Accuracy = (Spec Compliance + Code Quality + Error Handling) / 3
```

**Spec Compliance** (0-100):
- 100: All specified features implemented exactly as designed
- 80-99: Minor deviations from spec, documented with justification
- 60-79: Some features simplified or deferred
- <60: Significant gaps between spec and implementation

**Code Quality** (0-100):
- 100: Zero lint errors, all code reviewed, follows patterns
- 80-99: Minor lint warnings, mostly follows patterns
- 60-79: Some code quality issues, inconsistent patterns
- <60: Significant code quality problems

**Error Handling** (0-100):
- 100: Comprehensive error handling, retry logic, circuit breakers
- 80-99: Good error handling, some edge cases missed
- 60-79: Basic error handling, missing retry/recovery
- <60: Inadequate error handling

**Example**:
```
Implementation Accuracy = (95 + 90 + 92) / 3 = 92.3%
Weighted Contribution = 92.3% × 0.30 = 27.7 points
```

---

#### 2. Test Coverage (25% weight)

**Score Calculation**:
```
Test Coverage = (Unit Test Quality × 0.40) +
                (Integration Test Quality × 0.40) +
                (E2E Test Quality × 0.20)
```

**Unit Test Quality** (0-100):
- **Coverage Target**: 70-75% code coverage
- **Quality Factors**:
  - Tests use real business logic (not just mocks)
  - Table-driven tests for similar scenarios
  - Edge cases covered (nil, empty, invalid inputs)
  - Error paths tested

**Scoring**:
- 100: 75%+ coverage, comprehensive edge cases, production-ready tests
- 85: 70-75% coverage, most edge cases covered
- 70: 60-70% coverage, basic edge cases
- <70: <60% coverage or poor test quality

**Integration Test Quality** (0-100):
- **Coverage Target**: 15-20% of overall coverage
- **Quality Factors**:
  - Tests use real infrastructure (Kind/envtest)
  - Critical paths tested (happy path + error recovery)
  - Timing/concurrency issues validated
  - External dependency integration tested

**Scoring**:
- 100: 5+ comprehensive integration tests, all critical paths
- 85: 3-5 integration tests, most critical paths
- 70: 1-2 integration tests, basic coverage
- <70: No integration tests or poor quality

**E2E Test Quality** (0-100):
- **Coverage Target**: 5-10% of overall coverage
- **Quality Factors**:
  - Complete workflow scenarios
  - Production-like environment
  - Deployment validation

**Scoring**:
- 100: 2+ E2E tests covering complete workflows
- 85: 1 E2E test covering main workflow
- 70: E2E test planned but not implemented
- <70: No E2E tests

**Example**:
```
Test Coverage = (90 × 0.40) + (85 × 0.40) + (80 × 0.20)
              = 36 + 34 + 16 = 86%
Weighted Contribution = 86% × 0.25 = 21.5 points
```

---

#### 3. BR Coverage (20% weight)

**Score Calculation**:
```
BR Coverage = (Implemented BRs / Total BRs) × 100
```

**Scoring**:
- 100: All business requirements implemented and tested
- 90-99: 1-2 non-critical BRs deferred with justification
- 80-89: 3-5 BRs deferred, clear roadmap for implementation
- <80: Significant BR gaps affecting core functionality

**BR Mapping Requirement**:
- Each implemented BR must have:
  - At least one test validating the requirement
  - Documentation explaining the implementation
  - Evidence of successful validation

**Example**:
```
Total BRs: 15
Implemented BRs: 14
Deferred BRs: 1 (BR-SERVICE-015: Advanced analytics - v2.0 feature)

BR Coverage = (14 / 15) × 100 = 93.3%
Weighted Contribution = 93.3% × 0.20 = 18.7 points
```

---

#### 4. Production Readiness (15% weight)

**Score Calculation**: Based on Production Readiness Assessment scoring (from Appendix)

**Scoring Components**:
- Functional Validation (35 points)
- Operational Validation (29 points)
- Security Validation (15 points)
- Performance Validation (15 points)
- Deployment Validation (15 points)

**Total Possible**: 109 points (+ 10 bonus for documentation)

**Conversion to Percentage**:
```
Production Readiness = (Total Score / 109) × 100
```

**Scoring**:
- 95-100: Production-ready, deploy immediately
- 85-94: Mostly ready, minor improvements needed
- 75-84: Needs work before production
- <75: Not ready for production

**Example**:
```
Production Readiness Score: 103/109 = 94.5%
Weighted Contribution = 94.5% × 0.15 = 14.2 points
```

---

#### 5. Documentation Quality (10% weight)

**Score Calculation**:
```
Documentation Quality = (README + Design Decisions + Testing Docs + Troubleshooting) / 4
```

**README Quality** (0-100):
- 100: Complete with all sections, tested examples, accurate references
- 85: All sections present, minor gaps in examples
- 70: Basic README, missing integration guide or troubleshooting
- <70: Incomplete or inaccurate

**Design Decisions** (0-100):
- 100: All major decisions documented with DD-XXX format, alternatives considered
- 85: Most decisions documented, some missing rationale
- 70: Basic decisions documented, missing alternatives
- <70: Minimal or no design decision documentation

**Testing Documentation** (0-100):
- 100: Complete testing strategy, coverage matrix, known limitations documented
- 85: Good testing docs, minor gaps in coverage breakdown
- 70: Basic test documentation
- <70: Minimal test documentation

**Troubleshooting Guide** (0-100):
- 100: Comprehensive guide with common issues, symptoms, diagnosis, resolution
- 85: Good coverage of common issues
- 70: Basic troubleshooting info
- <70: Minimal or no troubleshooting guide

**Example**:
```
Documentation Quality = (95 + 90 + 88 + 85) / 4 = 89.5%
Weighted Contribution = 89.5% × 0.10 = 9.0 points
```

---

### Overall Confidence Calculation Example

**Component Scores**:
1. Implementation Accuracy: 92.3% → 27.7 points (30% weight)
2. Test Coverage: 86.0% → 21.5 points (25% weight)
3. BR Coverage: 93.3% → 18.7 points (20% weight)
4. Production Readiness: 94.5% → 14.2 points (15% weight)
5. Documentation Quality: 89.5% → 9.0 points (10% weight)

**Total Confidence Score**: 27.7 + 21.5 + 18.7 + 14.2 + 9.0 = **91.1%**

---

### Confidence Level Interpretation

| Score | Level | Interpretation | Action |
|-------|-------|----------------|--------|
| **95-100%** | ✅ **Exceptional** | Production-ready, comprehensive implementation | Deploy immediately |
| **90-94%** | ✅ **Excellent** | Production-ready with minor gaps | Deploy with confidence |
| **85-89%** | 🚧 **Good** | Mostly ready, some improvements needed | Address gaps, then deploy |
| **80-84%** | 🚧 **Acceptable** | Functional but needs work | Improve before production |
| **75-79%** | ⚠️ **Needs Improvement** | Significant gaps | Address critical issues |
| **<75%** | ❌ **Insufficient** | Not production-ready | Major work required |

---

### Confidence Assessment Template

**File**: `docs/services/[service]/implementation/CONFIDENCE_ASSESSMENT.md`

```markdown
# [Service] Confidence Assessment

**Assessment Date**: 2025-10-12
**Assessor**: [Name]
**Overall Confidence**: XX.X% ⭐

---

## Component Scores

### 1. Implementation Accuracy (30% weight)
- **Spec Compliance**: XX/100
- **Code Quality**: XX/100
- **Error Handling**: XX/100
- **Average**: XX.X%
- **Weighted Score**: XX.X points

**Evidence**:
- [Specific evidence for scores]

### 2. Test Coverage (25% weight)
- **Unit Tests**: XX/100
- **Integration Tests**: XX/100
- **E2E Tests**: XX/100
- **Weighted Average**: XX.X%
- **Weighted Score**: XX.X points

**Evidence**:
- Unit test coverage: XX% (`go test -cover`)
- [N] integration tests covering [list critical paths]
- [M] E2E tests validating [workflows]

### 3. BR Coverage (20% weight)
- **Implemented**: [N]/[M] BRs
- **Deferred**: [K] BRs with justification
- **Coverage**: XX.X%
- **Weighted Score**: XX.X points

**Deferred BRs**:
- BR-XXX-XXX: [Reason for deferral]

### 4. Production Readiness (15% weight)
- **Score**: [X]/109 points
- **Percentage**: XX.X%
- **Weighted Score**: XX.X points

**Reference**: [Production Readiness Report](./PRODUCTION_READINESS_REPORT.md)

### 5. Documentation Quality (10% weight)
- **README**: XX/100
- **Design Decisions**: XX/100
- **Testing Docs**: XX/100
- **Troubleshooting**: XX/100
- **Average**: XX.X%
- **Weighted Score**: XX.X points

---

## Overall Confidence Score

**Total**: XX.X / 100 points = **XX.X% Confidence**

**Confidence Level**: ✅ Exceptional | ✅ Excellent | 🚧 Good | ⚠️ Needs Improvement | ❌ Insufficient

---

## Strengths

1. [Specific strength with evidence]
2. [Another strength]
3. [Third strength]

---

## Areas for Improvement

1. **[Area]**: Current XX%, target YY%
   - **Gap**: [Description]
   - **Plan**: [Improvement strategy]

2. **[Another area]**: Current XX%, target YY%
   - **Gap**: [Description]
   - **Plan**: [Strategy]

---

## Recommendations

### Before Production Deployment
- [ ] [Critical recommendation]
- [ ] [Important recommendation]

### Post-Deployment
- [ ] [Monitoring recommendation]
- [ ] [Future improvement]

---

**Assessment Valid Until**: [Date]
**Next Assessment**: [Date for re-evaluation]
```

---

### Using Confidence Assessments

**When to Perform Assessments**:
1. **Day 7 (Mid-Implementation)**: Initial assessment to identify risks early
2. **Day 12 (Pre-Production)**: Final assessment before deployment
3. **Post-Production**: Periodic reassessments based on operational experience

**Confidence-Driven Decisions**:
- **<80%**: Do not deploy to production, address gaps first
- **80-89%**: Consider beta/staging deployment, plan improvements
- **90%+**: Production-ready, proceed with deployment

---

## 🐍 **Python Service Adaptation**

**When implementing Python services, adapt these sections**:

| Go Section | Python Equivalent | Notes |
|------------|-------------------|-------|
| **Unit Tests** | `pytest` with `pytest-asyncio` | Use `tests/test_*.py` structure |
| **Package Naming** | Same module structure | No `_test` suffix, use `tests/` directory |
| **Coverage Gates** | `pytest --cov=src --cov-fail-under=70` | Same 70% threshold |
| **Linting** | `black`, `flake8`, `mypy` | Instead of `golangci-lint` |
| **Dependencies** | `requirements.txt`, `requirements-test.txt` | Instead of `go.mod` |
| **Build** | `Dockerfile` with multi-stage build | For smaller images |

**Python-Specific Sections to Add**:

### **1. Virtual Environment Setup**
```bash
# Development environment
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
pip install -r requirements-test.txt
```

### **2. Docker Multi-Stage Build** (for smaller images)
```dockerfile
# Stage 1: Builder
FROM python:3.11-slim as builder
WORKDIR /app
COPY requirements.txt .
RUN pip install --user --no-cache-dir -r requirements.txt

# Stage 2: Runtime
FROM python:3.11-slim
WORKDIR /app
COPY --from=builder /root/.local /root/.local
COPY src/ ./src/
ENV PATH=/root/.local/bin:$PATH
CMD ["python", "-m", "src.main"]
```

### **3. FastAPI-Specific Patterns** (if REST API)
```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

app = FastAPI()

class Request(BaseModel):
    field1: str
    field2: int

@app.post("/api/v1/endpoint")
async def endpoint(request: Request):
    # Business logic
    return {"status": "success"}
```

### **4. Async/Await Patterns** (if applicable)
```python
import asyncio
from typing import List

async def process_batch(items: List[str]) -> List[str]:
    tasks = [process_item(item) for item in items]
    return await asyncio.gather(*tasks)

async def process_item(item: str) -> str:
    # Async processing
    await asyncio.sleep(0.1)
    return f"processed_{item}"
```

### **5. Python Testing Structure**
```
service-name/
├── src/
│   ├── __init__.py
│   ├── main.py
│   └── service/
│       ├── __init__.py
│       └── handler.py
├── tests/
│   ├── __init__.py
│   ├── test_handler.py        # Unit tests
│   └── integration/
│       └── test_api.py         # Integration tests
├── requirements.txt            # Production dependencies
├── requirements-test.txt        # Development dependencies
└── pytest.ini                  # Pytest configuration
```

**Python Testing Example** (pytest):
```python
# tests/test_handler.py
import pytest
from src.service.handler import Handler

@pytest.fixture
def handler():
    return Handler(config={"key": "value"})

def test_handler_success(handler):
    # ARRANGE
    input_data = {"field": "value"}

    # ACT
    result = handler.process(input_data)

    # ASSERT
    assert result["status"] == "success"
    assert result["field"] == "value"

@pytest.mark.asyncio
async def test_handler_async(handler):
    # ARRANGE
    input_data = {"field": "value"}

    # ACT
    result = await handler.process_async(input_data)

    # ASSERT
    assert result["status"] == "success"
```

---

## 🔗 **Sidecar Deployment Pattern**

**When to Use Sidecar**:
- Service needs to be co-located with main application (low latency)
- Service is internal-only (not exposed externally)
- Service shares lifecycle with main application
- Service provides supporting functionality (logging, metrics, caching)

**Sidecar Implementation Checklist**:
- [ ] Sidecar listens on `localhost` (not `0.0.0.0`)
- [ ] No Kubernetes Service for sidecar (pod-internal only)
- [ ] Startup probes account for sidecar initialization
- [ ] Readiness probes check both containers
- [ ] Resource limits set for both containers
- [ ] Network policy restricts access to pod only
- [ ] Shared volume for inter-container communication (if needed)
- [ ] Graceful shutdown coordination between containers

**Example Deployment** (Embedding Service Sidecar):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
spec:
  template:
    spec:
      containers:
      # Container 1: Main Application (Go)
      - name: datastorage
        image: datastorage:v1.0
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: EMBEDDING_SERVICE_URL
          value: "http://localhost:8086"  # Sidecar on localhost
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5

      # Container 2: Embedding Service (Python) - SIDECAR
      - name: embedding-sidecar
        image: embedding-service:v1.0
        ports:
        - containerPort: 8086  # Internal to pod only
          name: embedding
        env:
        - name: MODEL_NAME
          value: "all-mpnet-base-v2"
        - name: LISTEN_ADDR
          value: "127.0.0.1:8086"  # Localhost only
        resources:
          requests:
            memory: "1Gi"      # Model B needs more memory
            cpu: "500m"        # Model B needs more CPU
          limits:
            memory: "2Gi"
            cpu: "1000m"
        readinessProbe:
          httpGet:
            path: /health
            port: 8086
          initialDelaySeconds: 15  # Model loading time
          periodSeconds: 5
---
# No Service for sidecar - pod-internal only
apiVersion: v1
kind: Service
metadata:
  name: datastorage
spec:
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  # Note: No port 8086 exposed - sidecar is internal
```

**Network Policy** (restrict sidecar access):
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: embedding-sidecar-policy
spec:
  podSelector:
    matchLabels:
      app: datastorage
  policyTypes:
  - Ingress
  ingress:
  # Allow only pod-internal traffic to sidecar
  - from:
    - podSelector:
        matchLabels:
          app: datastorage
    ports:
    - protocol: TCP
      port: 8086
```

**Sidecar Communication Pattern** (Go client):
```go
// pkg/datastorage/embedding/client.go
type Client struct {
    baseURL string  // http://localhost:8086
    client  *http.Client
}

func NewClient(baseURL string) *Client {
    return &Client{
        baseURL: baseURL,
        client: &http.Client{
            Timeout: 5 * time.Second,
        },
    }
}

func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
    req := EmbeddingRequest{Text: text}
    body, _ := json.Marshal(req)

    resp, err := c.client.Post(
        fmt.Sprintf("%s/embed", c.baseURL),
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        return nil, fmt.Errorf("sidecar request failed: %w", err)
    }
    defer resp.Body.Close()

    var result EmbeddingResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode sidecar response: %w", err)
    }

    return result.Embedding, nil
}
```

---

## 📦 **Shared Library Extraction**

**When to Extract Shared Library**:
- Pattern used in **3+ services** (break-even point)
- Code is stable and well-tested
- Benefits outweigh refactoring cost
- Pattern is reusable across service types

**Extraction Checklist**:
- [ ] Identify reusable components (connection management, config, utilities)
- [ ] Create `pkg/[library]/` package structure
- [ ] Extract interfaces first, then implementations
- [ ] Add unit tests for shared library (≥70% coverage)
- [ ] Refactor original service to use shared library
- [ ] Verify original service tests still pass
- [ ] Update at least 2 other services to use shared library
- [ ] Document shared library in README with usage examples

**ROI Analysis**:

| Services | Copy-Paste Effort | Shared Library Effort | Break-Even | Decision |
|----------|-------------------|----------------------|------------|----------|
| 1 | 1x | 1.5x (overhead) | ❌ Not worth it | Copy-paste |
| 2 | 2x | 1.5x | ⚠️ Marginal | Case-by-case |
| 3 | 3x | 1.5x | ✅ Break-even | Extract |
| 4+ | 4x+ | 1.5x | ✅ Clear win | Extract |

**Extraction Example** (Redis Cache from Gateway):

**Before** (duplicated in Gateway, Data Storage, Signal Processing):
```go
// pkg/gateway/cache/redis.go (duplicated)
// ❌ LEGACY: Uses *zap.Logger directly (violates DD-005 v2.0)
type RedisCache struct {
    client    *redis.Client
    logger    *zap.Logger  // ❌ DD-005 violation
    connected atomic.Bool
    connCheckMu sync.Mutex
}

func (c *RedisCache) ensureConnection(ctx context.Context) error {
    if c.connected.Load() {
        return nil
    }
    // ... connection logic ...
}
```

**After** (shared library - DD-005 v2.0 compliant):
```go
// pkg/cache/redis/client.go (shared)
// DD-005 v2.0: Uses logr.Logger (unified interface for all Kubernaut services)
package redis

import "github.com/go-logr/logr"

type Client struct {
    client      *redis.Client
    logger      logr.Logger  // ✅ DD-005 v2.0: logr.Logger (not *zap.Logger)
    connected   atomic.Bool
    connCheckMu sync.Mutex
}

// NewClient creates a Redis client.
// DD-005 v2.0: Accept logr.Logger from caller (stateless services pass zapr.NewLogger(),
// CRD controllers pass ctrl.Log)
func NewClient(opts *redis.Options, logger logr.Logger) *Client {
    return &Client{
        client: redis.NewClient(opts),
        logger: logger,
    }
}

func (c *Client) EnsureConnection(ctx context.Context) error {
    // Fast path: already connected
    if c.connected.Load() {
        return nil
    }

    // Slow path: double-checked locking
    c.connCheckMu.Lock()
    defer c.connCheckMu.Unlock()

    if c.connected.Load() {
        return nil
    }

    // Try to connect
    if err := c.client.Ping(ctx).Err(); err != nil {
        return fmt.Errorf("redis unavailable: %w", err)
    }

    c.connected.Store(true)
    c.logger.Info("Redis connection established") // ✅ DD-005: logr syntax
    return nil
}
```

**Usage in Services**:
```go
// pkg/gateway/cache/deduplication.go
import "github.com/jordigilh/kubernaut/pkg/cache/redis"

type DeduplicationCache struct {
    redisClient *redis.Client
    ttl         time.Duration
}

func NewDeduplicationCache(redisClient *redis.Client, ttl time.Duration) *DeduplicationCache {
    return &DeduplicationCache{
        redisClient: redisClient,
        ttl:         ttl,
    }
}
```

**Shared Library Structure**:
```
pkg/cache/
├── redis/
│   ├── client.go          # Core Redis client with connection management
│   ├── config.go          # RedisOptions configuration struct
│   ├── cache.go           # Generic Cache[T] interface
│   └── client_test.go     # Unit tests (≥70% coverage)
├── memory/
│   ├── cache.go           # In-memory cache implementation
│   └── cache_test.go
└── README.md              # Usage examples, configuration guide
```

**Documentation Requirements** (`pkg/cache/redis/README.md`):
```markdown
# Redis Cache Library

## Overview
Shared Redis client with connection management, graceful degradation, and generic caching.

## Usage

### Basic Client
```go
import "github.com/jordigilh/kubernaut/pkg/cache/redis"

client := redis.NewClient(&redis.Options{
    Addr: "localhost:6379",
    DB: 0,
}, logger)

if err := client.EnsureConnection(ctx); err != nil {
    log.Fatal(err)
}
```

### Generic Cache
```go
cache := redis.NewCache[MyStruct](client, "prefix:", 1*time.Hour)

// Set
cache.Set(ctx, "key", &MyStruct{Field: "value"})

// Get
value, err := cache.Get(ctx, "key")
```

## Services Using This Library
- Gateway (deduplication cache)
- Data Storage (embedding cache)
- Signal Processing (alert cache)
```

---

## 🌐 **Multi-Language Services**

**When service uses multiple languages** (e.g., Go + Python):

**Directory Structure**:
```
service-name/
├── pkg/                    # Go code
│   ├── client/            # Go client for Python service
│   │   ├── client.go
│   │   └── client_test.go
│   └── service/           # Go business logic
│       ├── handler.go
│       └── handler_test.go
├── python-service/        # Python code (if sidecar)
│   ├── src/
│   │   ├── __init__.py
│   │   └── main.py
│   ├── tests/
│   │   └── test_main.py
│   ├── Dockerfile
│   └── requirements.txt
└── test/
    ├── unit/
    │   ├── go/           # Go unit tests
    │   │   └── handler_test.go
    │   └── python/       # Python unit tests
    │       └── test_main.py
    ├── integration/       # Cross-language integration tests
    │   └── embedding_integration_test.go
    └── e2e/              # End-to-end tests
        └── workflow_e2e_test.go
```

**Testing Strategy**:

### **1. Unit Tests** (in each language's directory)
- Go unit tests: `test/unit/go/`
- Python unit tests: `test/unit/python/`
- Each language tests its own business logic

### **2. Integration Tests** (cross-language communication)
```go
// test/integration/embedding_integration_test.go
var _ = Describe("Embedding Service Integration", func() {
    var (
        embeddingClient *client.EmbeddingClient
        testServer      *httptest.Server
    )

    BeforeEach(func() {
        // Start Python embedding service (or mock)
        testServer = startEmbeddingService()
        embeddingClient = client.NewEmbeddingClient(testServer.URL)
    })

    It("should generate embeddings via Python service", func() {
        embedding, err := embeddingClient.GenerateEmbedding(ctx, "test text")
        Expect(err).ToNot(HaveOccurred())
        Expect(embedding).To(HaveLen(768))  // Model B dimensions
    })
})
```

### **3. E2E Tests** (complete system validation)
- Test complete workflow (Go → Python → Go)
- Validate business outcomes, not implementation details

**CI/CD Considerations**:

### **1. Separate Coverage Gates**
```yaml
# .github/workflows/test.yml
jobs:
  test-go:
    runs-on: ubuntu-latest
    steps:
    - name: Run Go tests
      run: go test ./... -coverprofile=coverage.out
    - name: Check Go coverage
      run: |
        COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        if (( $(echo "$COVERAGE < 70" | bc -l) )); then
          echo "❌ Go coverage ($COVERAGE%) below 70%"
          exit 1
        fi

  test-python:
    runs-on: ubuntu-latest
    steps:
    - name: Run Python tests
      run: pytest --cov=src --cov-fail-under=70
```

### **2. Language-Specific Linting**
```yaml
  lint-go:
    runs-on: ubuntu-latest
    steps:
    - name: golangci-lint
      run: golangci-lint run ./...

  lint-python:
    runs-on: ubuntu-latest
    steps:
    - name: flake8
      run: flake8 src/ tests/
    - name: black
      run: black --check src/ tests/
    - name: mypy
      run: mypy src/
```

### **3. Multi-Stage Docker Builds**
```dockerfile
# Dockerfile (multi-language)

# Stage 1: Go Builder
FROM golang:1.21 as go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY pkg/ ./pkg/
RUN CGO_ENABLED=0 go build -o /app/service ./pkg/main.go

# Stage 2: Python Builder
FROM python:3.11-slim as python-builder
WORKDIR /app
COPY python-service/requirements.txt .
RUN pip install --user --no-cache-dir -r requirements.txt

# Stage 3: Runtime
FROM python:3.11-slim
WORKDIR /app

# Copy Go binary
COPY --from=go-builder /app/service /app/service

# Copy Python dependencies and code
COPY --from=python-builder /root/.local /root/.local
COPY python-service/src/ ./python-service/src/

ENV PATH=/root/.local/bin:$PATH

# Run both services (if not sidecar)
CMD ["/app/service"]
```

---

## 🔧 **Enhanced Error Handling Philosophy**

### **Graceful Degradation Principles**

**Core Principle**: Services should degrade gracefully when dependencies fail, not crash or block.

**Error Handling Hierarchy**:
1. **Retry with Exponential Backoff**: For transient errors (network, temporary unavailability)
2. **Cache Fallback**: Use cached data when primary source fails
3. **Graceful Degradation**: Return partial results or default behavior
4. **User-Friendly Messages**: Provide actionable error messages

### **Error Categories**

**Category 1: Transient Errors** (Retry)
- Network timeouts
- Temporary service unavailability
- Rate limiting (429)
- Database connection failures

**Handling**:
```go
func (c *Client) CallWithRetry(ctx context.Context, req *Request) (*Response, error) {
    backoff := time.Second
    maxBackoff := 30 * time.Second
    maxRetries := 3

    for attempt := 0; attempt < maxRetries; attempt++ {
        resp, err := c.call(ctx, req)
        if err == nil {
            return resp, nil
        }

        if !isTransientError(err) {
            return nil, fmt.Errorf("permanent error: %w", err)
        }

        c.logger.Warn("Transient error, retrying",
            zap.Int("attempt", attempt+1),
            zap.Duration("backoff", backoff),
            zap.Error(err))

        select {
        case <-time.After(backoff):
            backoff = min(backoff*2, maxBackoff)
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }

    return nil, fmt.Errorf("max retries exceeded")
}
```

**Category 2: Permanent Errors** (Fail Fast)
- Invalid input (400)
- Authentication failures (401)
- Authorization failures (403)
- Resource not found (404)

**Handling**:
```go
func (h *Handler) Process(ctx context.Context, req *Request) (*Response, error) {
    // Validate input early
    if err := req.Validate(); err != nil {
        return nil, &ValidationError{
            Field:   err.Field,
            Message: err.Message,
        }
    }

    // Fail fast on permanent errors
    if !h.authz.Authorize(ctx, req.User, req.Resource) {
        return nil, &AuthorizationError{
            User:     req.User,
            Resource: req.Resource,
        }
    }

    // Process request
    return h.process(ctx, req)
}
```

**Category 3: Cache Errors** (Degrade Gracefully)
- Redis unavailable
- Cache miss
- Cache corruption

**Handling**:
```go
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
    // Try cache first
    if err := c.ensureConnection(ctx); err != nil {
        c.logger.Warn("Cache unavailable, skipping", zap.Error(err))
        return nil, ErrCacheUnavailable  // Caller handles gracefully
    }

    val, err := c.client.Get(ctx, key).Result()
    if err == redis.Nil {
        return nil, ErrCacheMiss  // Not an error, just a miss
    }
    if err != nil {
        c.logger.Warn("Cache get failed", zap.Error(err))
        return nil, ErrCacheUnavailable  // Degrade gracefully
    }

    return []byte(val), nil
}
```

### **Logging Strategy**

**Log Levels**:

**ERROR**: Actionable errors requiring immediate attention
```go
logger.Error("Failed to process request",
    zap.String("request_id", req.ID),
    zap.Error(err),
    zap.String("action", "investigate_immediately"))
```

**WARN**: Degraded functionality, but service continues
```go
logger.Warn("Cache unavailable, using direct database access",
    zap.Error(err),
    zap.String("fallback", "database"))
```

**INFO**: Normal operations, state changes
```go
logger.Info("Request processed successfully",
    zap.String("request_id", req.ID),
    zap.Duration("latency", latency))
```

**DEBUG**: Detailed diagnostic information
```go
logger.Debug("Cache hit",
    zap.String("key", key),
    zap.Int("size", len(value)))
```

---

## Related Documents

- [00-core-development-methodology.mdc](.cursor/rules/00-core-development-methodology.mdc) - APDC-TDD methodology
- [Gateway Implementation](docs/services/stateless/gateway-service/implementation/) - Reference implementation
- [Dynamic Toolset Implementation](docs/services/stateless/dynamic-toolset/implementation/) - Enhanced patterns
- [Notification Controller Implementation](docs/services/crd-controllers/06-notification/implementation/IMPLEMENTATION_PLAN_V3.0.md) - CRD controller standard (V3.0, 98% confidence)
- [Data Storage Implementation](docs/services/stateless/data-storage/implementation/) - v4.1 standard

---

**Template Status**: ✅ **Production-Ready** (V2.5)
**Quality Standard**: Matches Notification V3.0 and Data Storage v4.1 standards + Multi-language support
**Success Rate**:
- Gateway: 95% test coverage, 100% BR coverage, 98% confidence
- Notification: 97.2% BR coverage, 95% test coverage, 98% confidence
- Data Storage (Go+Python): 100% test pass rate, hybrid architecture
**Estimated Effort Savings**: 3-5 days per service (comprehensive guidance prevents rework)

---

## 📚 ADR/DD Reference Matrix (v2.4)

This section provides a comprehensive reference of all ADRs and DDs that should be considered when implementing a new service.

### Quick Reference: Which Documents Apply?

| Service Type | MANDATORY | RECOMMENDED | OPTIONAL |
|--------------|-----------|-------------|----------|
| **HTTP Service** | DD-004, DD-005, DD-007, DD-014, ADR-015 | DD-013 | - |
| **CRD Controller** | DD-005, DD-006, DD-007, DD-014, ADR-004, ADR-015 | DD-013 | - |
| **Audit-Required** | ADR-032, ADR-034, ADR-038, DD-AUDIT-003 | - | DD-009 (V2) |
| **All Services (E2E)** | DD-TEST-001 | - | - |

### Universal Standards (ALL Services)

| Document | Purpose | Applicability |
|----------|---------|---------------|
| [DD-004: RFC 7807 Error Responses](../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md) | Standardized error format for HTTP APIs | **MANDATORY** for HTTP services |
| [DD-005: Observability Standards](../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | Metrics naming, logging format, tracing | **MANDATORY** for all services |
| [DD-007: Graceful Shutdown](../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) | 4-step shutdown for zero request failures | **MANDATORY** for all services |
| [DD-014: Binary Version Logging](../../architecture/decisions/DD-014-binary-version-logging-standard.md) | Version info at startup for troubleshooting | **MANDATORY** for all services |
| [ADR-015: Signal Naming](../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) | Use "Signal" terminology (not "Alert") | **MANDATORY** for all new code |

### Kubernetes-Aware Services

| Document | Purpose | Applicability |
|----------|---------|---------------|
| [DD-013: K8s Client Initialization](../../architecture/decisions/DD-013-kubernetes-client-initialization-standard.md) | Shared `pkg/k8sutil` for K8s client creation | **RECOMMENDED** for K8s-aware services |

### CRD Controller Standards

| Document | Purpose | Applicability |
|----------|---------|---------------|
| [DD-006: Controller Scaffolding](../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) | Templates for `cmd/`, config, metrics | **MANDATORY** for CRD controllers |
| [ADR-004: Fake K8s Client](../../architecture/decisions/ADR-004-fake-kubernetes-client.md) | Use `fake.NewClientBuilder()` for unit tests | **MANDATORY** for unit tests |

### Audit Standards

| Document | Purpose | Applicability |
|----------|---------|---------------|
| [DD-AUDIT-003: Service Audit Requirements](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) | Which services MUST generate audit traces | **CHECK FIRST** - determines if audit needed |
| [ADR-032: Data Access Layer Isolation](../../architecture/decisions/ADR-032-data-access-layer-isolation.md) | All audit writes via Data Storage REST API | **MANDATORY** for audit services |
| [ADR-034: Unified Audit Table Design](../../architecture/decisions/ADR-034-unified-audit-table-design.md) | Audit table schema and event format | **MANDATORY** for audit services |
| [ADR-038: Async Buffered Audit Ingestion](../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) | Fire-and-forget audit pattern | **MANDATORY** for audit services |
| [DD-009: Audit Write Error Recovery](../../architecture/decisions/DD-009-audit-write-error-recovery.md) | DLQ pattern for audit failures | **V2 ONLY** - deferred for V1 |

### Testing Standards

| Document | Purpose | Applicability |
|----------|---------|---------------|
| [DD-TEST-001: Port Allocation](../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) | Unique ports per service, NodePort for E2E | **MANDATORY** for E2E tests |

### Service-Specific Documents (NOT in Template)

The following document categories are **service-specific** and should NOT be referenced in the generic template:
- `DD-GATEWAY-*` - Gateway service only
- `DD-WORKFLOW-*` / `DD-PLAYBOOK-*` - Workflow/Playbook services only
- `DD-HOLMESGPT-*` - HolmesGPT API only
- `DD-STORAGE-*` - Data Storage service only
- `DD-EMBEDDING-*` - Embedding service only
- `DD-LLM-*` - LLM/AI services only
- `DD-AIANALYSIS-*` - AIAnalysis controller only
- `DD-ORCHESTRATOR-*` - Remediation Orchestrator only
- `DD-TOOLSET-*` - Dynamic Toolset only
- `DD-EFFECTIVENESS-*` - Effectiveness Monitor only

### How to Use This Matrix

1. **Before Day 0**: Identify your service type (HTTP, CRD, Audit)
2. **Prerequisites Checklist**: Mark all MANDATORY documents as reviewed
3. **Day 1-2**: Reference DD-006 (CRD) or DD-004/DD-005 (HTTP) for scaffolding
4. **Day 3-4**: Reference DD-007 for shutdown, DD-014 for version logging
5. **Day 5-6**: Reference ADR-032/ADR-038 for audit integration (if P0/P1 audit service)
6. **Day 8-10**: Reference DD-TEST-001 for test port allocation
7. **Day 12**: Reference all documents in Production Readiness Report

---

## Version History

### v2.1 (2025-11-23)
**Added: Multi-Language and Deployment Pattern Support** ⭐

**Changes**:
- Added comprehensive Python service adaptation guidance
- Added sidecar deployment pattern with Kubernetes examples
- Added shared library extraction guide with ROI analysis
- Added multi-language service structure (Go+Python)
- Enhanced error handling philosophy with graceful degradation patterns
- Added complete code examples for all new sections

**Impact**:
- Supports polyglot microservices (Go, Python, hybrid)
- Reduces sidecar implementation time by 50% (proven patterns)
- Prevents premature library extraction (ROI analysis)
- Comprehensive error handling reduces production incidents

**Reference**: Based on Data Storage embedding service implementation (Go+Python sidecar)

---

### v2.0 (2025-10-12)
**Added: Table-Driven Testing Pattern** ⭐

**Changes**:
- Added comprehensive table-driven testing guidance in DO-RED section
- Added "Table-Driven Testing Pattern" subsection in Testing Strategy
- Provided 4 complete code pattern examples (success, negative, health checks, setup functions)
- Updated Common Pitfalls section with table-driven testing guidance
- Updated Success Criteria with test organization quality indicators
- Added references to Dynamic Toolset detector test examples

**Impact**:
- 25-40% less test code expected
- Better test maintainability
- Easier to extend test coverage

**Reference**: [TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md](./TEMPLATE_UPDATE_TABLE_DRIVEN_TESTS.md)

---

### v1.0 (Initial Release)
**Base Template from Gateway + Dynamic Toolset Learnings**

**Included**:
- APDC-TDD methodology integration
- Integration-first testing strategy
- 12-day implementation timeline
- 5 critical checkpoints
- Daily progress documentation
- BR coverage matrix
- Production readiness checklist
- Performance benchmarking guidance

**Based On**:
- Gateway Service (proven success: 95% test coverage)
- Dynamic Toolset enhancements
- Gateway post-implementation triage

