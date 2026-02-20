# Signal Processing Service - Implementation Plan

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed per ADR-056 and ADR-055.

**Filename**: `IMPLEMENTATION_PLAN_V1.31.md`
**Version**: v1.31
**Last Updated**: 2025-12-16
**Timeline**: 14-17 days (quality-focused, includes label detection)
**Status**: ✅ 100% COMPLETE - All BRs implemented
**Quality Level**: Production-Ready Standard (19/19 BRs Implemented - BR-SP-110, BR-SP-111 Complete)

---

## 📋 Feature Extension Plans

This plan has been extended with standalone feature plans for specific enhancements:

| Plan | Feature | Status | BR Reference |
|------|---------|--------|--------------|
| [IMPLEMENTATION_PLAN_CONDITIONS_V1.0.md](./IMPLEMENTATION_PLAN_CONDITIONS_V1.0.md) | Kubernetes Conditions | ✅ COMPLETE | BR-SP-110 |
| Shared Backoff Integration | Exponential Backoff | ✅ COMPLETE | BR-SP-111 |

**Note**: Feature extension plans are standalone documents that add to this main plan. See [Naming Convention](../../SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#-naming-convention) for the pattern.

---

**Change Log**:
- **v1.31** (2025-12-09): Day-by-Day Triage - Documentation Fixes + BR-SP-090 Implementation
  - ✅ **BR-SP-090 Implemented**: `pkg/signalprocessing/audit/client.go` created (272 LOC)
  - ✅ **Audit Integration**: Controller integrates AuditClient for completion + classification events
  - ✅ **Unit Tests**: 10 tests in `test/unit/signalprocessing/audit_client_test.go`
  - ✅ **Field Name Fix**: `signal.Resource.Kind` → `signal.TargetResource.Kind` (matches actual code)
  - ✅ **DD-005 Compliance**: Updated imports from `go.uber.org/zap` to `github.com/go-logr/logr`
  - 📏 **Triage Source**: DAY_BY_DAY_TRIAGE.md against TESTING_GUIDELINES.md, DD-005, actual code
- **v1.30** (2025-12-07): Day 10 Triage - Test Coverage Expansion + Integration Test Matrix
  - 🔴 **Test Count Correction**: Plan said ~20 unit tests but actual is 184 (Days 1-9 implementation)
  - 🔴 **Integration Test Gap**: 0 integration tests exist, target 50-80 per plan
  - ✅ **Coverage Alignment**: Targets now match Gateway (~440) and DataStorage (~714) scale
  - ✅ **Test Count Table Updated**: Reflects actual counts + realistic targets
  - ✅ **Day 10 Integration Test Matrix**: Expanded from 1 example to 50+ specific tests
  - ✅ **Component Integration Tests**: Added K8sEnricher, EnvironmentClassifier, PriorityEngine tests
  - ✅ **Reconciler Integration Tests**: Full phase transition, error recovery, concurrent tests
  - ✅ **Rego Integration Tests**: Policy evaluation, hot-reload, fallback tests
  - ✅ **Controller Setup Added**: ENVTEST example now includes manager + reconciler setup
  - ✅ **Parallel Execution Pattern**: Unique namespace generation for isolation
  - 📏 **Triage Source**: Cross-service comparison (Gateway, DataStorage) + TESTING_GUIDELINES.md
- **v1.29** (2025-12-07): Day 9 Triage - CustomLabels Rego Extraction Gap Fixes
  - 🔴 **CRD Type Fix**: `CustomLabels map[string]string` → `map[string][]string` (DD-WORKFLOW-001 v1.9)
  - 🔴 **Label Domain Fix**: Security wrapper updated from `kubernaut.io/` to `kubernaut.ai/`
  - ✅ **Test Matrix Expanded**: 6 → 16 tests (Happy Path 5 + Edge Cases 6 + Error Handling 3 + Security 2)
  - ✅ **Test Files Specified**: `rego_engine_test.go` (BR-SP-102), `rego_security_wrapper_test.go` (BR-SP-104)
  - ✅ **Test Naming**: TC-CL-XXX → CL-HP-XX/CL-EC-XX/CL-ER-XX/CL-SEC-XX
  - ✅ **Sandbox Config**: Added 5s timeout, 128MB memory, StrictBuiltinErrors per DD-WORKFLOW-001 v1.9
  - ✅ **Input Types**: RegoInput now uses sharedtypes.KubernetesContext (authoritative source)
  - ✅ **Hot-Reload Integration**: Uses pkg/shared/hotreload/FileWatcher (Day 5 pattern)
  - ✅ **OPA v1 Syntax**: Updated Rego examples to use `import rego.v1` and `if` keyword
  - ✅ **ConfigMap Example**: Updated to use correct domain and OPA v1 syntax
  - 📏 **Triage Source**: Day 9 pre-implementation triage against DD-WORKFLOW-001 v1.9, TESTING_GUIDELINES.md
- **v1.28** (2025-12-07): Day 8 Post-Implementation Triage - Plan-to-Code Alignment
  - ✅ **Helper Function Signatures**: Updated to match implementation (detectPDB, detectHPA, etc.)
  - ✅ **API Call Clarity**: Functions that don't make API calls (detectGitOps, detectHelm, detectServiceMesh) now pass result directly, no error return
  - ✅ **DD-WORKFLOW-001 v2.3 Reference**: Updated comments to reference v2.3 (was v2.2)
  - ✅ **BR-SP-101 Cache TTL**: Clarified as deferred to V1.1 in BUSINESS_REQUIREMENTS.md
  - 📏 **Triage Source**: Day 8 post-implementation triage against actual code
- **v1.27** (2025-12-06): Day 8 Triage - DetectedLabels Test Matrix Expansion + Gap Fixes
  - ✅ **Test File Location**: Added `test/unit/signalprocessing/label_detector_test.go` per BR-SP-101
  - ✅ **Test Matrix Expanded**: 7 → 16 tests (Happy Path 9 + Edge Cases 3 + Error Handling 4)
  - ✅ **BR-SP-103 Coverage**: Added 4 FailedDetections error handling tests
  - ✅ **ServiceMesh Tests**: DL-HP-08 (Istio), DL-HP-09 (Linkerd) with specific annotations
  - ✅ **Stateful Detection**: Updated to use `ownerChain` parameter (no API call needed)
  - ✅ **Function Signature**: `DetectLabels(ctx, k8sCtx, ownerChain)` - added owner chain param
  - ✅ **DD-WORKFLOW-001 v2.3**: Updated authoritative doc with detailed detection methods
  - ✅ **Comment Fixed**: "9 label types" → "8 label types" (PodSecurityLevel removed v2.2)
  - ✅ **Code Fixed**: Removed PodSecurityLevel detection (deprecated PSP, inconsistent PSS)
  - ✅ **Import Clarified**: Added explicit `sharedtypes` import to plan code
  - ✅ **Test ID Naming**: TC-DL-XXX → DL-HP-XX/DL-EC-XX/DL-ER-XX (consistency with Day 7)
  - ✅ **SLA Tier Documentation**: Added industry-standard metallic tier explanation (ITIL, MSP)
  - 📏 **Triage Source**: Day 8 pre-implementation triage against TESTING_GUIDELINES.md
  - 📏 **DD Update**: DD-WORKFLOW-001 v2.2 → v2.3 (detection methods documented)
- **v1.26** (2025-12-06): Day 7 Triage - OwnerChain Schema Alignment + Test Matrix
  - ✅ **OwnerChainEntry Schema**: Fixed to include Namespace, exclude APIVersion/UID (DD-WORKFLOW-001 v1.8)
  - ✅ **Max Depth**: Specified MaxOwnerChainDepth = 5 per BR-SP-100
  - ✅ **Source Not In Chain**: Clarified chain contains owners only (source not included)
  - ✅ **Test Matrix Expanded**: 5 → 14 tests (Happy Path 4 + Edge Cases 4 + Error Handling 4 + Bonus 2)
  - ✅ **CRD Types Updated**: `api/signalprocessing/v1alpha1/signalprocessing_types.go` aligned
  - ✅ **K8s Enricher Fixed**: `buildOwnerChain` now uses correct schema
  - 📏 **Triage Source**: Day 7 pre-implementation triage against DD-WORKFLOW-001 v1.8
- **v1.25** (2025-12-06): Day 6 Triage - Business Classifier Complete Compliance
  - ✅ **Incremental Fill**: Classification uses all 4 tiers progressively (not early return)
  - ✅ **Default Values**: Criticality=medium, SLARequirement=bronze (per plan)
  - ✅ **Per-Field Confidence**: Added internal tracking for weighted average
  - ✅ **collectLabels**: Merges namespace + deployment labels
  - ✅ **Test Matrix**: 23 tests (Happy Path 7 + Edge Cases 8 + Confidence 4 + Error 4)
- **v1.24** (2025-12-06): Day 5 Complete + RO/Gateway Coordination
  - ✅ **Priority Engine**: Rego-based priority assignment complete
  - ✅ **HotReloader**: pkg/shared/hotreload/FileWatcher implemented
  - ✅ **Gateway Notice Updated**: SP environment + priority ready, Gateway unblocked
  - ✅ **RO Notice Updated**: SP Day 5 complete, RO may proceed with schema updates
- **v1.21** (2025-12-04): Template v3.0 Full Compliance + Implementation Start
  - ✅ **Template Reference Updated**: v2.8 → v3.0 in "Based on" section
  - ✅ **HANDOFF/RESPONSE Pattern**: Added documentation section (Template v3.0)
  - ✅ **Pre-Implementation ADR/DD Validation**: Added checklist section (Template v3.0)
  - ✅ **testing-strategy.md Aligned**: TC-DL-008 removed (podSecurityLevel), BR range updated
  - 📏 **Implementation**: Day 1 ready to start
- **v1.20** (2025-12-03): DD-WORKFLOW-001 v2.2 - PodSecurityLevel Removed + All 8 Detections in V1.0
  - ✅ **PodSecurityLevel REMOVED**: PSP deprecated K8s 1.21, removed 1.25; PSS is namespace-level (inconsistent)
  - ✅ **DetectedLabels Scope**: All 8 detections in V1.0 (NetworkIsolated, ServiceMesh no longer deferred)
  - ✅ **DD-WORKFLOW-001 Updated**: v2.1 → v2.2 (schema change)
  - ✅ **Go Type Updated**: `pkg/shared/types/enrichment.go` - PodSecurityLevel field removed
  - 📏 **Notification**: See `docs/handoff/NOTICE_PODSECURITYLEVEL_REMOVED.md`
- **v1.19** (2025-12-03): Rego Policy Testing + Timeline Optimization + RBAC Documentation
  - ✅ **Rego Policy Testing Strategy**: Dedicated section with 4 integration test scenarios
  - ✅ **Timeline Optimized**: Gateway migration moved earlier (Days 4-5, 10) instead of last day
  - ✅ **RBAC Documentation**: Extended permissions for DetectedLabels documented in Deployment Guide
  - ✅ **User Documentation**: Added Day 14 task for operator/user-facing docs
  - ✅ **Performance Testing**: Deferred to V1.1 (KIND API server limits >100 concurrent signals)
  - ✅ **Missing RBAC Markers**: Added PDB, HPA, NetworkPolicy permissions for DetectedLabels
  - 📏 **AIAnalysis Feedback**: Rego policy integration testing uniqueness addressed
- **v1.18** (2025-12-02): DD-WORKFLOW-001 v2.1 + Template v3.0 Compliance + All Gaps Fixed + Gateway Triage
  - ✅ **DD-WORKFLOW-001 Updated**: v1.9 → v2.1 (DetectedLabels schema change)
  - ✅ **FailedDetections Field**: New `[]string` field tracks query failures (RBAC, timeout)
  - ✅ **Go Type Updated**: `pkg/shared/types/enrichment.go` now includes FailedDetections
  - ✅ **BR-SP-100 to BR-SP-104**: Added to Primary Business Requirements table
  - ✅ **BR Coverage Matrix**: Updated to 17/17 BRs (100%)
  - ✅ **Cross-Team Validation Section**: Added dedicated section (Template v3.0)
  - ✅ **Risk Mitigation Table**: Added "Day" column (Template v3.0 format)
  - ✅ **Gateway Migration VERIFIED**: Actual line counts confirmed (478 LOC + 857 tests = 1,335 total)
  - ✅ **OPA Library Confirmed**: `github.com/open-policy-agent/opa/v1/rego` (official, already used by Gateway)
  - ✅ **Audit Library Documented**: `pkg/audit/store.go` - shared `AuditStore` interface
  - ✅ **Infrastructure to Create**: Kind config, Rego policies documented
  - ✅ **Dead Links Fixed**: Fixed 14+ broken relative paths (`../../` → `../../../` for handoff, architecture, templates)
  - 📏 **Reference**: [DD-WORKFLOW-001 v2.1](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md)
- **v1.17** (2025-12-02): DD-WORKFLOW-001 v1.9 + Shared Types + RO Contract Gaps + Data Flow
  - ✅ **DD-WORKFLOW-001 Updated**: v1.8 → v1.9 (CustomLabels validation limits now authoritative)
  - ✅ **Validation Limits**: max 10 keys, 5 values/key, 63 char keys, 100 char values
  - ✅ **Security Measures**: Sandboxed OPA Runtime (no network, no filesystem, 5s timeout, 128MB)
  - ✅ **Mandatory Label Protection**: Security wrapper strips 5 system labels from customer Rego output
  - ✅ **Shared Types Section**: Added `pkg/shared/types/` as authoritative source for enrichment types
  - ✅ **Data Flow Section**: Documented RO copying to AIAnalysis (per NOTICE_AIANALYSIS_PATH_CORRECTION.md)
  - ✅ **RO Contract Gaps**: Documented GAP-C1-01/02/05/06 fixes (Environment, Priority, StormType, StormWindow)
  - ✅ **EnrichmentQuality Decision**: Documented NOT implementing (deterministic lookups don't need quality scores)
  - ✅ **Code Examples Updated**: OwnerChainEntry now uses `sharedtypes.OwnerChainEntry`
  - 📏 **Reference**: [DD-WORKFLOW-001 v1.9](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [NOTICE_AIANALYSIS_PATH_CORRECTION.md](../../../handoff/NOTICE_AIANALYSIS_PATH_CORRECTION.md)
- **v1.14** (2025-11-30): DD-WORKFLOW-001 v1.9 - OwnerChain, DetectedLabels, CustomLabels
  - ✅ **Added Phase 3.25**: Owner Chain & Label Detection (2-3 days)
  - ✅ **OwnerChain Implementation**: K8s ownerReference traversal for DetectedLabels validation
  - ✅ **DetectedLabels Auto-Detection**: 8 detection types (GitOps, PDB, HPA, StatefulSet, Helm, NetworkPolicy, ServiceMesh) - PSS removed v1.20
  - ✅ **CustomLabels Rego Extraction**: Customer Rego policies with security wrapper
  - ✅ **Security Wrapper**: Blocks override of 5 mandatory labels
  - ✅ **New Business Requirements**: BR-SP-100 to BR-SP-104 for label detection
  - ✅ **New Test Scenarios**: 12 label detection test cases added
  - ✅ **Timeline Updated**: 12-14 days → 14-17 days (+2-3 days for label detection)
  - ✅ **Prerequisites Updated**: DD-WORKFLOW-001 v1.9 added as mandatory
  - 📏 **Reference**: [DD-WORKFLOW-001 v1.9](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)
- **v1.13** (2025-11-30): DD-005 v2.0 code example compliance
  - ✅ **Code Examples Fixed**: All `*zap.Logger` → `logr.Logger` in code examples
  - ✅ **Logging Syntax Fixed**: All `zap.Error()`, `zap.String()` → key-value pairs
  - ✅ **Constructor Patterns Fixed**: `.Named()` → `.WithName()` for logr
  - ✅ **Reconciler Logger Fixed**: `*zap.Logger` → `logr.Logger` (native from ctrl.Log)
  - ✅ **Audit Client Fixed**: Accept `logr.Logger` per DD-005 shared library standard
  - 📏 **Impact**: 10 code blocks updated for DD-005 v2.0 compliance
- **v1.12** (2025-11-28): Template v2.8 alignment - comprehensive new sections
  - ✅ **Logging Framework Decision Matrix**: DD-005 v2.0 compliance with `ctrl.Log` for CRD controllers
  - ✅ **Pre-Implementation Design Decisions**: DD format for ambiguous requirements
  - ✅ **Risk Assessment Matrix**: 6 identified risks with mitigations
  - ✅ **Files Affected Section**: Complete inventory of new/modified/deleted files
  - ✅ **Enhancement Application Checklist**: Day-by-day pattern tracking
  - ✅ **Metrics Cardinality Audit**: Prometheus memory protection (<1,000 target)
  - ✅ **Rollback Plan Template**: Production deployment recovery procedure
  - ✅ **Critical Issues Resolved Section**: Knowledge capture template
  - ✅ **Pre-Day Validation Checklists**: Formal checkpoints for Days 7, 10, 12
  - ✅ **Template Reference Updated**: v2.3 → v2.8
  - ✅ **Package Name Fixed**: `signalprocessing_test` → `signalprocessing` (per 03-testing-strategy.mdc)
  - ✅ **MockK8sClient Fixed**: Replaced with `fake.NewClientBuilder()` + `WithInterceptorFuncs()` (ADR-004 compliance)
  - 📏 **Plan size**: ~4,900 lines (growth for template v2.8 compliance)
- **v1.11** (2025-11-28): Gateway migration triage + ADR/DD link validation
  - ✅ **Gateway Migration Triage**: Complete inventory of code to migrate (~1,400 lines, ~48 tests)
  - ✅ **Detailed File List**: Source → Target mapping for all migration files
  - ✅ **Effort Estimate**: ~8 hours (1 full day) for complete migration
  - ✅ **ADR/DD Links Validated**: All 15 referenced documents confirmed to exist
  - ✅ **Old Plan Versions Removed**: Only v1.11 retained (authoritative)
  - ✅ **Template Aligned**: v2.5 with pre-implementation ADR/DD checklist
- **v1.10** (2025-11-28): Document structure fix - Error Handling Philosophy nesting
  - ✅ **Heading Levels Fixed**: Core Principles, Retry Strategy, Error Wrapping, Logging → `####` level
  - ✅ **TOC Updated**: Error Handling sections nested under Days 3-6 EOD deliverable
  - ✅ **Context Preserved**: Sections now clearly part of Day 6 Error Handling Philosophy document
  - ✅ **Developer Flow**: Day-by-day implementation no longer interrupted by standalone sections
- **v1.9** (2025-11-28): ADR/DD triage - comprehensive template compliance
  - ✅ **Universal Standards Added**: DD-005 (Observability), DD-013 (K8s Client), ADR-015 (Signal naming)
  - ✅ **CRD Standards Added**: ADR-004 (Fake K8s Client - MANDATORY for unit tests)
  - ✅ **Audit Standards Added**: DD-014, DD-AUDIT-003, ADR-032, ADR-034
  - ✅ **Prerequisites Restructured**: Categorized by Universal/CRD/Audit/Testing/Service-Specific
  - ✅ **References Section Restructured**: Categorized with MANDATORY markers
  - ✅ **K8s Client Mandate Section**: Added fake client pattern per ADR-004
  - ✅ **Aligned with Template v2.4**: Full ADR/DD reference matrix compliance
  - ✅ **DD-009 Triaged**: DLQ pattern deferred to V2 (ADR-038 fire-and-forget sufficient for V1)
- **v1.8** (2025-11-28): E2E NodePort infrastructure
  - ✅ **E2E NodePort Configuration**: Complete Kind config with `extraPortMappings` (no port-forward)
  - ✅ **DD-TEST-001 Compliance**: Port allocation per authoritative document (30182 for metrics)
  - ✅ **Controller Service YAML**: NodePort service configuration for E2E tests
  - ✅ **Test Suite Pattern**: NodePort access without kubectl port-forward
  - ✅ **Template Reference Updated**: Now based on SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.3
- **v1.7** (2025-11-28): Testing methodology alignment + parallel execution standard
  - ✅ **Testing Order Alignment**: Standard Unit → Integration → E2E methodology (removed "Integration-First")
  - ✅ **Parallel Test Execution**: **4 concurrent processes** standard for all test tiers
  - ✅ **Parallel Execution Section**: Complete configuration with `go test -p 4` and `ginkgo -procs=4`
  - ✅ **Test Isolation Patterns**: Unique namespace per test for parallel safety
  - ✅ **Parallel Anti-Patterns**: Common mistakes to avoid (hardcoded namespaces, shared state)
  - ✅ **Makefile Targets Updated**: All test targets now include `-p 4` flag
  - ✅ **Quick Reference Updated**: Methodology and parallel execution standards added
  - ✅ **Template Reference Updated**: Now based on SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v2.2
- **v1.6** (2025-11-28): Test scenarios, operational sections, Gateway code migration
  - ✅ Defined 138 concrete test scenarios upfront (per TDD methodology)
  - ✅ Test scenarios organized by component with ID, input, expected outcome
  - ✅ Fixed E2E test path from `test/e2e/` to `test/e2e/signalprocessing/`
  - ✅ Updated configuration to separate controller config (hardcoded) from business config (YAML)
  - ✅ Removed configurable `leader_election` (always enabled in production)
  - ✅ Removed configurable `audit.enabled` (audit is mandatory per ADR-032)
  - ✅ Fixed port allocation: metrics=`:9090`, health=`:8081` (per DD-TEST-001)
  - ✅ Added `audit.buffer_size` and `audit.flush_interval` config (per ADR-038)
  - ✅ Added DD-007 (Graceful Shutdown) and DD-017 (K8s Enrichment Depth) to references
  - ✅ Added Service-Specific Error Categories (A-E) with code examples
  - ✅ Added Production Runbooks section (3 runbooks)
  - ✅ Added Edge Case Categories section (5 categories)
  - ✅ Added Metrics Validation Commands section
  - ✅ Added Blockers Section template
  - ✅ Added Lessons Learned / Technical Debt templates
  - ✅ Added Team Handoff Notes with debugging tips
  - ✅ **Gateway Code Migration**: Detailed step-by-step migration plan
    - Source files: `classification.go` (267 LOC), `priority.go` (222 LOC), `priority.rego` (74 LOC)
    - Unit tests: 34 tests (~860 LOC) to migrate
    - Integration tests: 6 files affected
    - E2E tests: 1 file affected
  - ✅ Ported all operational sections to `SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md`
- **v1.5** (2025-11-28): Template compliance and documentation completeness
  - ✅ Added Table of Contents with section links
  - ✅ Added Common Pitfalls section (Signal Processing specific)
  - ✅ Added Success Criteria with completion checklist
  - ✅ Added Makefile Targets for development workflow
  - ✅ Expanded Table-Driven Testing patterns (4 patterns with examples)
  - ✅ Fixed CRD group from `signalprocessing.kubernaut.io` to `kubernaut.io`
  - ✅ Fixed package name from `signalprocessing_test` to `signalprocessing`
  - ✅ Added package declarations and imports to all code snippets
- **v1.4** (2025-11-28): Metrics triage and test location fixes
  - ✅ Triaged metrics for business value (12 → 6 metrics)
  - ✅ Removed redundant metrics (Enrichment/Classification/Audit totals)
  - ✅ Added business value documentation for each metric
  - ✅ Fixed test location: moved from `api/.../types_test.go` to `test/unit/signalprocessing/`
  - ✅ Tests now focus on business outcomes, not struct validation
- **v1.3** (2025-11-27): DD-017 documentation for enrichment depth strategy
  - ✅ Created DD-017: K8s Enrichment Depth Strategy (signal-driven, standard depth, no config)
  - ✅ Added DD-017 to key decisions and references sections
  - ✅ Cross-referenced DD-017 in ADR-041
- **v1.2** (2025-11-27): Signal-driven K8s enrichment strategy
  - ✅ K8s Enricher now uses signal-driven enrichment based on `signal.TargetResource.Kind`
  - ✅ Standard depth (hardcoded, no configuration): Pod→Ns+Pod+Node+Owner, Deploy→Ns+Deploy, Node→Node only
  - ✅ Updated architecture diagram with signal-driven enrichment description
  - ✅ Updated K8s Enricher implementation with `enrichPodSignal()`, `enrichDeploymentSignal()`, `enrichNodeSignal()`
  - ✅ Design decision: No configurable depth (YAGNI, avoid configuration complexity)
- **v1.1** (2025-11-27): Template compliance enhancements
  - ✅ Added ADR-041: Rego Policy Data Fetching Separation (K8s Enricher + Rego architecture)
  - ✅ Added Critical Checkpoints section (5 checkpoints from Gateway learnings)
  - ✅ Added Error Handling Philosophy section (Day 6 EOD deliverable)
  - ✅ Added Appendix A: EOD Documentation Templates (Day 1, 4, 7)
  - ✅ Added Appendix B: Production Readiness Report Template
  - ✅ Added Appendix C: Handoff Summary Template
  - ✅ Added "Why K8s Enricher + Rego" architecture explanation with ADR-041 reference
  - ✅ Updated References section with ADR-041
- **v1.0** (2025-11-27): Initial implementation plan created
  - ✅ Full CRD controller implementation using DD-006 templates
  - ✅ Complete categorization ownership (migrated from Gateway)
  - ✅ Rego policy engine integration (ConfigMap hot-reload)
  - ✅ K8s context enrichment and business classification
  - ✅ Gateway migration tasks (Days 4-5, 10 per v1.19 optimization)
  - ✅ ENVTEST integration test environment

---

## 🎯 **Quick Reference**

**Use this plan for**: Signal Processing CRD Controller implementation
**Based on**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0 + DD-006 Controller Scaffolding
**Methodology**: APDC-TDD with Defense-in-Depth Testing (Unit → Integration → E2E)
**Parallel Execution**: **4 concurrent processes** for all test tiers (standard)
**Test Environment**: ENVTEST (confirmed)
**Logging Framework**: `logr.Logger` via `ctrl.Log.WithName()` (DD-005 v2.0)
**Target Success Rate**:
- Unit Test Coverage: 70%+
- Integration Test Coverage: 50%+
- BR Coverage: 100% (12/12 BRs)
- Overall Confidence: 90%+

**Quality Standard**: V3.0 - Production-ready with comprehensive examples

---

## 📑 **Table of Contents**

| Section | Purpose |
|---------|---------|
| [Quick Reference](#-quick-reference) | Template overview and success metrics |
| [Critical: Read This First](#-critical-read-this-first) | Key decisions and prerequisites |
| [CRD API Group Standard](#-crd-api-group-standard-authoritative) | Unified API group rationale |
| [Business Requirements](#-business-requirements) | BR-SP-001 through BR-SP-012 |
| [Authoritative References](#-authoritative-references) | DDs and ADRs |
| [Prerequisites Checklist](#-prerequisites-checklist) | Pre-Day 1 requirements |
| [Logging Framework Decision Matrix](#-logging-framework-decision-matrix-dd-005-v20) | `logr.Logger` usage for CRD controllers ⭐ v1.12 |
| [Pre-Implementation Design Decisions](#-pre-implementation-design-decisions) | DD format for ambiguous requirements ⭐ v1.12 |
| [Risk Assessment Matrix](#️-risk-assessment-matrix) | Risk identification and mitigation ⭐ v1.12 |
| [Files Affected](#-files-affected) | New/modified/deleted file inventory ⭐ v1.12 |
| [Enhancement Application Checklist](#-enhancement-application-checklist) | Pattern tracking by day ⭐ v1.12 |
| [Critical Checkpoints](#-critical-checkpoints-from-gateway-learnings) | 5 checkpoints from Gateway learnings |
| [Timeline Overview](#-timeline-overview) | Phase breakdown (14 days) |
| [Architecture](#-architecture) | Component diagram and data flow |
| **Day-by-Day Breakdown** | |
| ├─ [Day 0: ANALYSIS + PLAN](#day-0-analysis--plan-pre-work-) | Pre-work planning |
| ├─ [Day 1: Foundation](#day-1-foundation---dd-006-scaffolding) | DD-006 scaffolding, types |
| ├─ [Day 2: CRD Types](#day-2-foundation---crd-types) | API types and CRD generation |
| ├─ [Days 3-6: Core Logic](#days-3-6-core-logic-components) | Enricher, classifiers, Rego (priority) |
|    ├─ Day 6 EOD: Error Handling Philosophy | Document deliverable |
|       ├─ [Core Principles](#-core-principles) | Error classification |
|       ├─ [Retry Strategy](#-retry-strategy-for-crd-controller) | Backoff and requeue |
|       ├─ [Error Wrapping](#-error-wrapping-pattern) | Standard error patterns |
|       └─ [Logging Best Practices](#-logging-best-practices) | Structured logging |
| ├─ [Days 7-9: Label Detection](#days-7-9-label-detection--new-dd-workflow-001-v19) | OwnerChain, DetectedLabels, CustomLabels (v1.9 validation) ⭐ NEW |
| ├─ [Days 10-11: Integration](#days-10-11-integration) | Metrics, server, controller |
| ├─ [Days 12-13: Testing](#days-12-13-testing) | Unit, integration, E2E |
| └─ [Days 14-15: Finalization](#days-14-15-finalization) | Docs, cleanup, handoff |
| [BR Coverage Matrix](#-business-requirements-coverage-matrix) | Requirements to tests mapping |
| [Rego Policy Testing Strategy](#-rego-policy-testing-strategy) | ConfigMap, hot-reload, fallback tests |
| [Key Files and Locations](#-key-files-and-locations) | Directory structure |
| [Production Readiness Checklist](#-production-readiness-checklist) | Deployment validation |
| [Confidence Assessment](#-confidence-assessment) | Overall confidence rating |
| [Common Pitfalls](#-common-pitfalls-signal-processing-specific) | Do's and don'ts |
| [Success Criteria](#-success-criteria) | Completion checklist |
| [Makefile Targets](#-makefile-targets) | Development commands |
| **Appendices** | |
| ├─ [Appendix A: EOD Templates](#-appendix-a-eod-documentation-templates) | Days 1, 4, 7 templates |
| ├─ [Appendix B: Production Readiness](#-appendix-b-production-readiness-report-template) | Report template |
| └─ [Appendix C: Handoff Summary](#-appendix-c-handoff-summary-template) | Handoff template |

---

## 🚨 **CRITICAL: Read This First**

**Before starting implementation, you MUST review these critical decisions:**

1. **DD-CATEGORIZATION-001**: Signal Processing owns ALL categorization (environment + priority)
2. **DD-006**: Use CRD controller scaffolding templates from `docs/templates/crd-controller-gap-remediation/`
3. **DD-001**: Recovery context from embedded RemediationRequest data (no Context API)
4. **ADR-032**: Use Data Storage Service REST API for audit writes (no direct DB access)
5. **ADR-015**: Use "Signal" terminology throughout (not "Alert")
6. **DD-SIGNAL-PROCESSING-001**: Service renamed from RemediationProcessor to SignalProcessing
7. **DD-CRD-001**: Use `*.kubernaut.ai/v1alpha1` API group for ALL CRDs (AIOps branding)
8. **ADR-041**: K8s Enricher fetches data, Rego policies evaluate classification (no `http.send` in Rego)
9. **DD-017**: Signal-driven K8s enrichment with standard depth (hardcoded, no configuration)
10. **DD-WORKFLOW-001 v2.1**: Label schema with OwnerChain, DetectedLabels (FailedDetections), CustomLabels ⭐ UPDATED
11. **Shared Types**: Use `pkg/shared/types/` for `EnrichmentResults`, `DetectedLabels`, `OwnerChainEntry`, `DeduplicationInfo` ⭐ NEW
12. **RO Contract Alignment**: GAP-C1-01/02/05/06 fixes applied to CRD types ⭐ NEW

⚠️ **Build Fresh**: No legacy code migration. Use DD-006 templates and deprecate existing `pkg/signalprocessing/` code.

---

## 🔷 **CRD API Group Standard (AUTHORITATIVE)**

**API Group**: `kubernaut.ai/v1alpha1`
**Reference**: `docs/architecture/decisions/DD-CRD-001-api-group-domain-selection.md`

### **Decision**
All Kubernaut CRDs use the **`.ai` domain** for AIOps branding:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing
```

### **Rationale** (per DD-CRD-001)
1. **K8sGPT Precedent**: AI K8s projects use `.ai` (e.g., `core.k8sgpt.ai`)
2. **Brand Alignment**: AIOps is the core value proposition - domain reflects this
3. **Differentiation**: Stands out from traditional infrastructure tooling (`.io`)
4. **Industry Trend**: AI-native platforms increasingly adopt `.ai`

**Note**: Label keys still use `kubernaut.io/` prefix (K8s label convention, not CRD API group).

### **Industry Best Practices Analysis**

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

### **Confidence Assessment**

| Factor | Assessment | Confidence |
|--------|------------|------------|
| Industry alignment | Matches 5/6 major CNCF projects | 95% |
| Simplicity benefit | Reduced complexity for users/operators | 95% |
| Future scalability | V2 features can add fields, not new groups | 90% |
| Migration risk | Low - pre-release product | 98% |

**Overall Confidence**: **95%** - Strong alignment with industry best practices

### **CRD Inventory (Unified API Group)**

| CRD Kind | Full Resource Name | Controller |
|----------|-------------------|------------|
| `RemediationRequest` | `remediationrequests.remediation.kubernaut.ai` | RemediationOrchestrator |
| `SignalProcessing` | `signalprocessings.kubernaut.ai` | SignalProcessing |
| `AIAnalysis` | `aianalyses.kubernaut.ai` | AIAnalysis |
| `WorkflowExecution` | `workflowexecutions.kubernaut.ai` | RemediationExecution |
| `NotificationRequest` | `notificationrequests.notification.kubernaut.ai` | Notification |

---

## 🔗 **Shared Types (AUTHORITATIVE)** ⭐ NEW (v1.17)

**Location**: `pkg/shared/types/`
**Reference**: DD-CONTRACT-002, DD-WORKFLOW-001 v1.9

SignalProcessing MUST use shared types from `pkg/shared/types/` for API contract alignment:

### **Import Pattern**
```go
import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
```

### **Shared Types Usage**

| Type | Location | Used In |
|------|----------|---------|
| `sharedtypes.EnrichmentResults` | `pkg/shared/types/enrichment.go` | `SignalProcessingStatus.EnrichmentResults` |
| `sharedtypes.DetectedLabels` | `pkg/shared/types/enrichment.go` | `EnrichmentResults.DetectedLabels` |
| `sharedtypes.OwnerChainEntry` | `pkg/shared/types/enrichment.go` | `EnrichmentResults.OwnerChain` |
| `sharedtypes.KubernetesContext` | `pkg/shared/types/enrichment.go` | `EnrichmentResults.KubernetesContext` |
| `sharedtypes.DeduplicationInfo` | `pkg/shared/types/deduplication.go` | `SignalProcessingSpec.Deduplication` |

### **Why Shared Types?**
- ✅ **Single source of truth**: All services use identical type definitions
- ✅ **API contract alignment**: RO, AIAnalysis, HolmesGPT-API consume same schema
- ✅ **Type safety**: Go compiler enforces contract compliance
- ✅ **Maintenance**: Schema changes propagate automatically

### **EnrichmentQuality Field - NOT IMPLEMENTED**

> ⚠️ **Decision (Dec 2, 2025)**: `EnrichmentQuality` field is NOT implemented.
>
> **Rationale**: SignalProcessing's `DetectedLabels` are **deterministic lookups**:
> - Detection succeeds → explicit `true`/`false` values
> - Detection fails (RBAC, timeout) → `false` + error log
>
> No "partial success" concept exists. Error logs provide observability for failed detections.
>
> **Reference**: `pkg/shared/types/enrichment.go` (lines 58-62)

---

## 📊 **Data Flow to Downstream Services** ⭐ NEW (v1.17)

**Reference**: [NOTICE_AIANALYSIS_PATH_CORRECTION.md](../../../handoff/NOTICE_AIANALYSIS_PATH_CORRECTION.md)

### **SignalProcessing Output**

SignalProcessing populates `status.enrichmentResults`:
```go
// SignalProcessing controller reconciliation output
sp.Status.EnrichmentResults = sharedtypes.EnrichmentResults{
    KubernetesContext: kubeCtx,
    DetectedLabels:    detectedLabels,
    OwnerChain:        ownerChain,
    CustomLabels:      customLabels,
}
```

### **RO Copies to AIAnalysis**

Remediation Orchestrator (RO) copies data when creating AIAnalysis CRD:
```go
// RO creates AIAnalysis with enrichment data from SignalProcessing
aiAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults = sharedtypes.EnrichmentResults{
    KubernetesContext: signalProcessing.Status.EnrichmentResults.KubernetesContext,
    DetectedLabels:    signalProcessing.Status.EnrichmentResults.DetectedLabels,
    OwnerChain:        signalProcessing.Status.EnrichmentResults.OwnerChain,
    CustomLabels:      signalProcessing.Status.EnrichmentResults.CustomLabels,
}
```

### **Data Flow Diagram**
```
SignalProcessing.Status.EnrichmentResults
        │
        │ (RO copies)
        ▼
AIAnalysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults
        │
        │ (AIAnalysis passes to)
        ▼
HolmesGPT-API (workflow filtering + LLM context)
        │
        │ (searches)
        ▼
Data Storage (workflow catalog)
```

### **SignalProcessing Responsibility**
- ✅ Populate `status.enrichmentResults` with all enrichment data
- ❌ Do NOT create AIAnalysis CRD (RO's responsibility)
- ❌ Do NOT call HolmesGPT-API (AIAnalysis's responsibility)

---

## 📝 **RO Contract Gap Fixes** ⭐ NEW (v1.17)

The following contract gaps (identified by RO team) were fixed in `api/signalprocessing/v1alpha1/signalprocessing_types.go`:

| GAP ID | Issue | Fix | Status |
|--------|-------|-----|--------|
| **GAP-C1-01** | `Environment` had enum constraint | Changed to free-text (MinLength=1, MaxLength=63) | ✅ Fixed |
| **GAP-C1-02** | `Priority` had enum + pattern constraint | Changed to free-text (MinLength=1, MaxLength=63) | ✅ Fixed |
| **GAP-C1-05** | Missing `StormType` field | Added field for contract alignment | ✅ Fixed |
| **GAP-C1-06** | Missing `StormWindow` field | Added field for contract alignment | ✅ Fixed |

### **Why Free-Text for Environment/Priority?**

**Rationale** (per DD-WORKFLOW-001 v1.9):
- Customers define environment meaning for risk (e.g., "uat" = high risk for one team, low for another)
- Rego policies assign environment and priority values
- Enum constraints prevent valid customer-defined values like "qa-eu", "canary", "dr-site"

**Best Practice Examples**:
- Environment: `production`, `staging`, `development`, `qa-eu`, `canary`, `dr-site`
- Priority: `P0` (critical), `P1` (high), `P2` (normal), `P3` (low), `critical`, `high`

---

## 📋 **Version History**

| Version | Date | Changes | Status |
|---------|------|---------|--------|
| **v1.26** | 2025-12-06 | Day 7 triage fixes: CRD OwnerChainEntry schema corrected (add Namespace, remove APIVersion/UID per DD-WORKFLOW-001 v1.8), max depth 10→5 per BR-SP-100, source NOT in chain (owners only), expanded test matrix 5→12, use CRD type v1alpha1.OwnerChainEntry, k8s_enricher.go aligned with new schema | ✅ **CURRENT** |
| **v1.25** | 2025-12-06 | Day 6 triage fixes: Added BR-SP-002 coverage, 4-tier confidence scoring (labels→pattern→Rego→default), removed priority from Classify input, NewBusinessClassifier constructor, fixed K8s label limit (63 chars), timeout 200ms, BC-CF-* tests, safe type assertions | ✅ |
| **v1.24** | 2025-12-06 | Day 5 triage fixes: PE-ER-01 (construction error, not fallback), PE-ER-06 (fallback per BR-SP-071), `fallbackBySeverity` signature (no error return) | ✅ |
| **v1.23** | 2025-12-06 | Day 5 gap fixes: Created `pkg/shared/hotreload/FileWatcher`, `priority.rego` policy, NewPriorityEngine constructor, 100ms timeout, nil checks, priority validation, integration test matrix (PE-HR-*), CRD Source comment update | ✅ |
| **v1.22** | 2025-12-06 | Day 5 updates: fsnotify hot-reload (DD-INFRA-001), severity-based fallback (BR-SP-071), BR-SP-070 schema compliance, P0-P3 only. Day 4 fixes: struct alignment, EC-EC-02 test, hot-reload test, EC-ER-03/04 clarifications | ✅ |
| **v1.21** | 2025-12-05 | Day 4 Rego-based environment classifier, Gateway coordination | ✅ |
| **v1.18** | 2025-12-02 | DD-WORKFLOW-001 v2.1, Template v3.0 compliance, all gaps fixed | ✅ |
| **v1.17** | 2025-12-02 | Shared types, data flow, RO contract gaps, DD-WORKFLOW-001 v1.9 | ✅ |
| **v1.16** | 2025-11-30 | Cross-team validation complete (100% confidence) | ✅ |
| **v1.6** | 2025-11-28 | Test scenarios (138), Gateway code migration (489 LOC + 34 tests), operational sections, DD-007 | ✅ |
| **v1.5** | 2025-11-28 | Template compliance: TOC, Common Pitfalls, Success Criteria, Makefile, Table-Driven Tests | ✅ |
| **v1.4** | 2025-11-28 | Metrics triage (12→6), test location fix (api/ → test/unit/), null-testing removal | ✅ |
| **v1.3** | 2025-11-27 | DD-017: K8s Enrichment Depth Strategy documentation | ✅ |
| **v1.2** | 2025-11-27 | Signal-driven K8s enrichment with standard depth (no config) | ✅ |
| **v1.1** | 2025-11-27 | ADR-041, Critical Checkpoints, Error Handling, EOD Templates, Appendices | ✅ |
| **v1.0** | 2025-11-27 | Initial implementation plan: CRD controller with full categorization ownership | ✅ |

---

## 🎯 **Business Requirements**

> **Authoritative Source**: [BUSINESS_REQUIREMENTS.md](BUSINESS_REQUIREMENTS.md) - Full requirement definitions with acceptance criteria

### **Primary Business Requirements**

| BR ID | Description | Success Criteria |
|-------|-------------|------------------|
| **BR-SP-001** | K8s Context Enrichment | Fetch deployment, pod, node context within 2 seconds |
| **BR-SP-002** | Business Classification | Classify signals by business criticality |
| **BR-SP-003** | Recovery Context Integration | Embed recovery data from RemediationRequest (DD-001) |
| **BR-SP-051** | Environment Classification (Primary) | Classify from namespace labels with 95%+ confidence |
| **BR-SP-052** | Environment Classification (Fallback) | ConfigMap override when labels unavailable |
| **BR-SP-053** | Environment Classification (Default) | Graceful degradation to "unknown" |
| **BR-SP-070** | Priority Assignment (Rego) | Rego policies with K8s + business context |
| **BR-SP-071** | Priority Fallback Matrix | Severity + environment fallback when Rego fails |
| **BR-SP-072** | Rego Hot-Reload | ConfigMap-based policy updates without restart |
| **BR-SP-080** | Confidence Scoring | 0.0-1.0 confidence score for all categorization |
| **BR-SP-081** | Multi-dimensional Categorization | businessUnit, serviceOwner, criticality, sla |
| **BR-SP-090** | Categorization Audit Trail | Log all decisions via Data Storage API |
| **BR-SP-100** | OwnerChain Traversal | Build K8s ownership chain (Pod → ReplicaSet → Deployment) |
| **BR-SP-101** | DetectedLabels Auto-Detection | Detect 8 cluster characteristics from K8s resources |
| **BR-SP-102** | CustomLabels Rego Extraction | Extract customer labels via sandboxed OPA policies |
| **BR-SP-103** | FailedDetections Tracking | Track query failures (RBAC, timeout) per DD-WORKFLOW-001 v2.1 |
| **BR-SP-104** | Mandatory Label Protection | Block customer Rego from overriding 5 system labels |

### **Success Metrics**

**Format**: `[Metric]: [Target] - *Justification: [Why this target?]*`

- **K8s Enrichment Latency**: <2 seconds P95 - *Justification: Acceptable delay for rich context*
- **Categorization Accuracy**: 95%+ confidence - *Justification: Enables AI analysis reliability*
- **Rego Policy Evaluation**: <100ms P95 - *Justification: Fast policy decisions*
- **Audit Write Latency**: <1ms P95 - *Justification: Fire-and-forget pattern per ADR-038 (non-blocking)*
- **Owner Chain Build**: <500ms P95 - *Justification: Fast K8s API traversal (DD-WORKFLOW-001 v1.9)*
- **DetectedLabels Detection**: <200ms P95 - *Justification: Parallel K8s queries for 8 detection types*
- **Rego Policy Evaluation**: <100ms P95 - *Justification: Fast policy decisions*
- **Test Coverage**: 70%+ unit, 50%+ integration - *Justification: Defense-in-depth testing*

---

## 📚 **Authoritative References**

### **Design Decisions (DD)**

| DD ID | Title | Impact on Implementation |
|-------|-------|--------------------------|
| **DD-006** | Controller Scaffolding Strategy | Use templates from `docs/templates/crd-controller-gap-remediation/` |
| **DD-007** | Kubernetes-Aware Graceful Shutdown | Implement 4-step shutdown with readiness probe coordination |
| **DD-017** | K8s Enrichment Depth Strategy | Standard depth enrichment (hardcoded, no config knobs) |
| **DD-001** | Recovery Context Enrichment | Recovery data from embedded RemediationRequest, not Context API |
| **DD-CATEGORIZATION-001** | Gateway vs Signal Processing Split | Signal Processing owns ALL categorization |
| **DD-SIGNAL-PROCESSING-001** | Service Rename | Use SignalProcessing (not RemediationProcessor) |
| **DD-CONTEXT-006** | Context API Deprecation | No Context API integration |
| **DD-WORKFLOW-001 v1.9** | Mandatory Label Schema | OwnerChain, DetectedLabels, CustomLabels for workflow filtering ⭐ NEW |

### **Architecture Decision Records (ADR)**

| ADR ID | Title | Impact on Implementation |
|--------|-------|--------------------------|
| **ADR-015** | Alert-to-Signal Naming | Use "Signal" terminology throughout |
| **ADR-032** | Data Access Layer Isolation | Audit writes via Data Storage REST API |
| **ADR-034** | Unified Audit Table | Audit events: `signalprocessing.enrichment.*`, `signalprocessing.categorization.*` |
| **ADR-038** | Async Buffered Audit Ingestion | Fire-and-forget audit writes (<1ms overhead, non-blocking) |
| **ADR-041** | Rego Policy Data Fetching Separation | K8s Enricher fetches data, Rego evaluates policies (no `http.send`) |

---

## ✅ **Prerequisites Checklist**

**Before starting Day 1, ensure all items are checked:**

### **Documentation Prerequisites**
- [ ] Service specifications complete:
  - [ ] `overview.md` - Service overview and responsibilities
  - [ ] `crd-schema.md` - SignalProcessing CRD schema
  - [ ] `controller-implementation.md` - Reconciler design
  - [ ] `reconciliation-phases.md` - Phase state machine
  - [ ] `integration-points.md` - Upstream/downstream services
- [ ] Business requirements documented (BR-SP-XXX format) - 12 BRs defined ✅
- [ ] Architecture decisions approved:
  - **Universal Standards (ALL services)**:
    - [ ] DD-005: Observability Standards (**MANDATORY** - metrics/logging) ✅
    - [ ] DD-007: Graceful Shutdown (**MANDATORY**) ✅
    - [ ] DD-014: Binary Version Logging (**MANDATORY**) ✅
    - [ ] ADR-015: Signal Naming (**MANDATORY** - use "Signal" terminology) ✅
  - **CRD Controller Standards**:
    - [ ] DD-006: Controller Scaffolding ✅
    - [ ] DD-013: K8s Client Initialization ✅
    - [ ] ADR-004: Fake K8s Client (**MANDATORY for unit tests**) ✅
  - **Audit Standards (Signal Processing is P1)**:
    - [ ] DD-AUDIT-003: Service Audit Requirements ✅
    - [ ] ADR-032: Data Access Layer Isolation (**MANDATORY**) ✅
    - [ ] ADR-034: Unified Audit Table Design ✅
    - [ ] ADR-038: Async Buffered Audit Ingestion ✅
  - **Testing Standards**:
    - [ ] DD-TEST-001: Port Allocation (**MANDATORY for E2E**) ✅
  - **Service-Specific**:
    - [ ] DD-CATEGORIZATION-001: Categorization Ownership ✅
    - [ ] DD-017: K8s Enrichment Depth ✅
    - [ ] DD-001: Recovery Context Enrichment ✅
  - **Label Detection (DD-WORKFLOW-001 v1.9)**: ⭐ NEW
    - [ ] DD-WORKFLOW-001 v1.9: OwnerChain, DetectedLabels, CustomLabels (**MANDATORY**)
    - [ ] HANDOFF v3.2: Rego Label Extraction specification
    - [ ] Security wrapper for 5 mandatory labels
    - [ ] ADR-041: Rego Policy Data Fetching Separation ✅

### **Dependency Prerequisites**
- [ ] Data Storage Service REST API available (`/api/v1/audit/events`)
- [ ] Kubernetes API access configured
- [ ] RemediationRequest CRD deployed (Gateway dependency)

### **Test Infrastructure Prerequisites**
- [ ] **Integration test environment determined**: ENVTEST ✅
- [ ] **envtest binaries installed**: `setup-envtest use 1.31.0`
- [ ] **Test framework available**: Ginkgo/Gomega

### **Infrastructure to Create** ⭐ v1.18 VERIFIED
> **Note**: These files do NOT exist yet and must be created during implementation.

| File | Purpose | Day |
|------|---------|-----|
| `test/infrastructure/kind-signalprocessing-config.yaml` | Kind cluster config with NodePort mappings | Day 11 |
| `config.app/signalprocessing/policies/priority.rego` | Copied from Gateway, adapted for SP | Day 5 |
| `config.app/signalprocessing/policies/customlabels.rego` | New Rego for CustomLabels extraction | Day 9 |

### **Shared Libraries to Use** ⭐ v1.18 VERIFIED

| Library | Location | Purpose |
|---------|----------|---------|
| **Audit Store** | `pkg/audit/store.go` | `AuditStore` interface for async audit writes |
| **Audit HTTP Client** | `pkg/audit/http_client.go` | HTTP client for Data Storage API |
| **OPA Library** | `github.com/open-policy-agent/opa/v1/rego` | Official OPA library (already used by Gateway) |
| **DataStorage OpenAPI** | `docs/services/stateless/data-storage/api/audit-write-api.openapi.yaml` | API spec for audit writes |

### **Template Sections Review** (V2.1 Compliance)
- [ ] Error Handling Philosophy Template reviewed (create on Day 6)
- [ ] BR Coverage Matrix Methodology reviewed (create on Day 9)
- [ ] EOD Documentation Templates reviewed (Appendix A)
- [ ] CRD Controller Variant patterns reviewed (Appendix B)
- [ ] Complete Integration Test Examples reviewed (Day 9)
- [ ] Production Readiness Report template reviewed (Day 12)
- [ ] Handoff Summary template reviewed (Day 12)
- [ ] Confidence Assessment Methodology reviewed (Day 12)

### **Success Criteria Defined**
- [ ] K8s Enrichment Latency: <2 seconds P95
- [ ] Categorization Accuracy: 95%+ confidence
- [ ] Rego Policy Evaluation: <100ms P95
- [ ] Test Coverage: 70%+ unit, 50%+ integration

---

## 🤝 **Cross-Team Validation** ⭐ v1.18 NEW (Template v3.0)

**Purpose**: Formally validate all cross-team dependencies before starting implementation.

### **Cross-Team Validation Status**

> **Validation Status**: ✅ **VALIDATED** - Ready for Implementation

| Team | Validation Topic | Status | Record |
|------|-----------------|--------|--------|
| **HolmesGPT-API** | CustomLabels pass-through | ✅ Complete | [RESPONSE_CUSTOM_LABELS_VALIDATION.md](RESPONSE_CUSTOM_LABELS_VALIDATION.md) |
| **AIAnalysis** | EnrichmentResults data path | ✅ Complete | [NOTICE_AIANALYSIS_PATH_CORRECTION.md](../../../handoff/NOTICE_AIANALYSIS_PATH_CORRECTION.md) |
| **AIAnalysis** | DetectedLabels FailedDetections schema | ✅ Complete | [AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md](../../../handoff/AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md) |
| **Gateway** | Label passthrough behavior | ✅ Complete | [RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md](RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md) |
| **Data Storage** | JSONB query structure | ✅ Complete | Confirmed via handoff |
| **RO** | Contract gaps (Environment, Priority, Storm fields) | ✅ Complete | CRD types updated |

### **Pre-Implementation Validation Gate**

> ✅ **ALL cross-team validations COMPLETE** - Proceed to Day 1

**Validation Checklist**:
- [x] All upstream data contracts validated (RemediationRequest → SignalProcessing)
- [x] All downstream data contracts validated (SignalProcessing → AIAnalysis)
- [x] Shared type definitions aligned (`pkg/shared/types/enrichment.go`)
- [x] Naming conventions agreed (JSON camelCase, K8s kebab-case)
- [x] Field paths confirmed (`status.enrichmentResults.*`)
- [x] Integration points documented with examples

**Confidence**: 100% (all contracts verified)

### **HANDOFF/RESPONSE Pattern** (Template v3.0)

**File Naming Convention**:
- `HANDOFF_REQUEST_[TOPIC].md` - Request sent to another team
- `RESPONSE_[TOPIC].md` - Response received from that team
- Handoff documents are in `docs/handoff/` directory

**SignalProcessing Handoff Files**:
```
docs/handoff/
├── AIANALYSIS_TO_SIGNALPROCESSING_TEAM.md    ← From AIAnalysis team
├── NOTICE_AIANALYSIS_PATH_CORRECTION.md       ← Path correction notice
├── QUESTIONS_FOR_SIGNALPROCESSING_TEAM.md     ← HolmesGPT-API questions
└── V1.0-TIMELINE-QUESTIONS.md                 ← Timeline coordination
```

### 📋 **Pre-Implementation ADR/DD Validation** (Template v3.0)

**Validation Status**: ✅ **VALIDATED** - All referenced documents exist

**CRD Controller Standards**:
- [x] DD-006: Controller Scaffolding Strategy
- [x] DD-013: K8s Client Initialization Standard
- [x] DD-CRD-001: API Group Domain Selection (`.kubernaut.ai`)
- [x] ADR-004: Fake K8s Client (for unit tests)

**Universal Standards**:
- [x] DD-004: RFC 7807 Error Responses
- [x] DD-005: Observability Standards (v2.0)
- [x] DD-007: Kubernetes-Aware Graceful Shutdown
- [x] DD-014: Binary Version Logging Standard
- [x] ADR-015: Alert to Signal Naming Migration

**Testing Standards**:
- [x] DD-TEST-001: Port Allocation Strategy
- [x] DD-WORKFLOW-001: Mandatory Label Schema (v2.2)

**Audit Standards** (P1 per DD-AUDIT-003):
- [x] DD-AUDIT-003: Service Audit Trace Requirements
- [x] ADR-032: Data Access Layer Isolation
- [x] ADR-034: Unified Audit Table Design
- [x] ADR-038: Async Buffered Audit Ingestion

---

## 📝 **Logging Framework Decision Matrix (DD-005 v2.0)** ⭐ v1.12 NEW

**Authority**: [DD-005-OBSERVABILITY-STANDARDS.md](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) v2.0

### **Unified Logging Interface**

**MANDATORY**: Signal Processing Controller uses `logr.Logger` as the unified logging interface.

| Component | Logger Creation | Usage |
|-----------|----------------|-------|
| **Controller (main.go)** | `ctrl.Log.WithName("signalprocessing")` | Native logr from controller-runtime |
| **Reconciler** | `log.FromContext(ctx)` | Request-scoped logger with correlation |
| **Shared Libraries** | Accept `logr.Logger` parameter | Passed by caller |

### **Implementation Pattern (CRD Controller)**

```go
package main

import (
    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
    // Setup logr via controller-runtime (native, no adapter needed)
    opts := zap.Options{Development: false}
    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    // Get named logger for this controller
    logger := ctrl.Log.WithName("signalprocessing-controller")

    // Pass to shared libraries
    auditStore, _ := audit.NewBufferedStore(client, config, "signalprocessing", logger.WithName("audit"))
    enricher := enricher.New(k8sClient, logger.WithName("enricher"))
}
```

### **Logging Syntax (logr)**

```go
// INFO level (V=0, always shown)
logger.Info("Signal received", "fingerprint", fp, "phase", "enriching")

// DEBUG level (V=1, shown when verbosity >= 1)
logger.V(1).Info("Fetching K8s context", "namespace", ns, "resource", res)

// ERROR level (error as first argument)
logger.Error(err, "Failed to enrich signal", "fingerprint", fp, "phase", phase)

// Named sub-logger for component
enricherLog := logger.WithName("enricher")
classifierLog := logger.WithName("classifier")
```

### **❌ FORBIDDEN Patterns**

```go
// ❌ WRONG: Using *zap.Logger directly in shared libraries
func NewEnricher(..., logger *zap.Logger) // FORBIDDEN

// ❌ WRONG: Using zap.String() helpers with logr
logger.Info("Message", zap.String("key", "value")) // FORBIDDEN

// ❌ WRONG: Creating separate zap logger in CRD controllers
zapLogger, _ := zap.NewProduction() // FORBIDDEN in CRD controllers

// ✅ CORRECT: Accept logr.Logger in shared libraries
func NewEnricher(..., logger logr.Logger) *Enricher // CORRECT
```

---

## 🔍 **Pre-Implementation Design Decisions** ⭐ v1.12 NEW

**Purpose**: Document decisions made during ANALYSIS phase for ambiguous requirements.

### **DD-1: Reconciliation Trigger Strategy**

| Question | Should reconciliation be triggered by all field changes or specific fields only? |
|----------|----------------------------------------------------------------------------------|
| **Decision** | **Option B**: Specific fields only (spec changes, not status). |
| **Rationale** | Prevents reconciliation loops from status updates. |
| **Implementation** | Use `GenerationChangedPredicate` in controller builder. |

```go
// In SetupWithManager
ctrl.NewControllerManagedBy(mgr).
    For(&kubernautv1alpha1.SignalProcessing{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
    Complete(r)
```

### **DD-2: Finalizer Strategy**

| Question | Should the controller use finalizers for cleanup? |
|----------|--------------------------------------------------|
| **Decision** | **Option A**: Yes, use finalizers for audit trail completion. |
| **Rationale** | Ensures audit data persisted before CRD deletion. |
| **Implementation** | Add finalizer on first reconcile, remove after audit write in delete handler. |

### **DD-3: Rego Policy Reload Strategy**

| Question | Should Rego policies reload on every reconciliation or watch ConfigMap changes? |
|----------|--------------------------------------------------------------------------------|
| **Decision** | **Option B**: Watch ConfigMap changes (hot-reload). |
| **Rationale** | Avoids re-parsing policies on every reconciliation (~50ms savings). |
| **Implementation** | Use informer to watch `kubernaut-rego-policies` ConfigMap, cache compiled policy. |

### **DD-4: K8s Enrichment Failure Handling**

| Question | Should enrichment failure block processing or use degraded mode? |
|----------|------------------------------------------------------------------|
| **Decision** | **Option B**: Degraded mode with partial context. |
| **Rationale** | Availability over completeness - signal processing continues. |
| **Implementation** | Set `status.degradedMode = true`, use signal labels as fallback context. |

### **Pre-Implementation Checklist**

- [x] All ambiguous requirements have documented decisions (DD-1 to DD-4)
- [x] Each decision has clear rationale
- [x] Implementation impact is documented
- [x] Decisions approved by stakeholder

---

## ⚠️ **Risk Assessment Matrix** ⭐ v1.12 NEW

**Purpose**: Identify and mitigate risks before implementation begins.

### **Identified Risks**

| # | Risk | Probability | Impact | Mitigation | Owner |
|---|------|-------------|--------|------------|-------|
| 1 | **K8s API unavailable** | Medium | High | Degraded mode with signal labels fallback | Dev |
| 2 | **Rego policy evaluation timeout** | Low | Medium | 100ms timeout, fallback to severity+environment matrix | Dev |
| 3 | **Data Storage Service unavailable** | Medium | Medium | Async buffered audit (ADR-038), retry on failure | Dev |
| 4 | **Gateway migration breaks existing tests** | Low | High | Feature flag for gradual rollout, comprehensive test migration | Dev |
| 5 | **ConfigMap hot-reload race condition** | Low | Medium | Mutex protection on policy reload, version tracking | Dev |
| 6 | **Memory pressure from large K8s contexts** | Low | Medium | Context size limit (100KB), pagination for large resources | Dev |

### **Risk Severity Matrix**

| Probability ↓ / Impact → | Low | Medium | High | Critical |
|---------------------------|-----|--------|------|----------|
| **High** | Monitor | Mitigate | Mitigate | Block |
| **Medium** | Accept | Monitor | Mitigate | Mitigate |
| **Low** | Accept | Accept | Monitor | Mitigate |

### **Mitigation Status** (Template v3.0 Format)

| Risk # | Action Required | Day | Status |
|--------|-----------------|-----|--------|
| 1 | Implement `buildDegradedContext()` | Day 3 | ⬜ Pending |
| 2 | Add Rego timeout and fallback matrix | Day 5 | ⬜ Pending |
| 3 | Use `audit.NewBufferedStore()` with retry | Day 8 | ⬜ Pending |
| 4 | Create Gateway test migration checklist | Day 12 | ⬜ Pending |
| 5 | Add `sync.RWMutex` to policy cache | Day 5 | ⬜ Pending |
| 6 | Add context size validation | Day 3 | ⬜ Pending |
| 7 | Implement FailedDetections tracking | Day 8 | ⬜ Pending |

**Status Legend**:
- ⬜ Pending: Not yet implemented
- 🔄 In Progress: Currently being addressed
- ✅ Complete: Mitigation implemented and tested
- ❌ Blocked: Cannot proceed (escalate)

---

## 📋 **Files Affected** ⭐ v1.12 NEW

**Purpose**: Document all files that will be created, modified, or deleted during implementation.

### **New Files** (to be created)

| File | Purpose | Day |
|------|---------|-----|
| `cmd/signalprocessing/main.go` | Controller entry point | Day 1 |
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | CRD type definitions | Day 2 |
| `api/signalprocessing/v1alpha1/zz_generated.deepcopy.go` | Generated deepcopy | Day 2 |
| `internal/controller/signalprocessing/reconciler.go` | Main reconciler | Day 7 |
| `internal/controller/signalprocessing/phases.go` | Phase handlers | Day 7 |
| `pkg/signalprocessing/enricher/enricher.go` | K8s context enricher | Day 3 |
| `pkg/signalprocessing/enricher/degraded.go` | Degraded mode fallback | Day 3 |
| `pkg/signalprocessing/classifier/environment.go` | Environment classifier | Day 4 |
| `pkg/signalprocessing/classifier/business.go` | Business classifier | Day 6 |
| `pkg/signalprocessing/categorizer/priority.go` | Priority engine | Day 5 |
| `pkg/signalprocessing/categorizer/rego.go` | Rego policy engine | Day 5 |
| `pkg/signalprocessing/config/config.go` | Configuration types | Day 1 |
| `pkg/signalprocessing/metrics/metrics.go` | Prometheus metrics | Day 8 |
| `config/signalprocessing/policies/priority.rego` | Priority Rego policy | Day 5 |
| `config/signalprocessing/policies/environment.rego` | Environment Rego policy | Day 4 |
| `test/unit/signalprocessing/enricher_test.go` | Enricher unit tests | Day 9 |
| `test/unit/signalprocessing/classifier_test.go` | Classifier unit tests | Day 9 |
| `test/unit/signalprocessing/categorizer_test.go` | Categorizer unit tests | Day 9 |
| `test/integration/signalprocessing/reconciler_test.go` | Integration tests | Day 10 |
| `test/e2e/signalprocessing/e2e_test.go` | E2E tests | Day 11 |

### **Modified Files** (existing files to update)

| File | Changes | Day |
|------|---------|-----|
| `pkg/gateway/processing/crd_creator.go` | Remove classification, pass through raw values | Day 12 |
| `pkg/gateway/server.go` | Remove classifier/categorizer instantiation | Day 12 |
| `pkg/gateway/config/config.go` | Remove classification config section | Day 12 |
| `config/crd/bases/kubernaut.ai_signalprocessings.yaml` | Generated CRD manifest | Day 2 |
| `Makefile` | Add signalprocessing targets | Day 1 |

### **Deleted Files** (obsolete files to remove)

| File | Reason | Day |
|------|--------|-----|
| `pkg/gateway/processing/classification.go` | Moved to Signal Processing | Day 12 |
| `pkg/gateway/processing/priority.go` | Moved to Signal Processing | Day 12 |
| `config.app/gateway/policies/priority.rego` | Moved to Signal Processing | Day 12 |
| `test/unit/gateway/processing/environment_classification_test.go` | Moved to Signal Processing | Day 12 |
| `test/unit/gateway/priority_classification_test.go` | Moved to Signal Processing | Day 12 |

**Validation**: Run `git status` at end of each day to verify file changes match plan.

---

## 🔄 **Enhancement Application Checklist** ⭐ v1.12 NEW

**Purpose**: Track which patterns and enhancements have been applied to which implementation days.

### **Enhancement Tracking**

| Enhancement | Applied To | Status | Notes |
|-------------|------------|--------|-------|
| **Error Handling Philosophy** | Day 6 EOD | ⬜ Pending | Document 5 error categories (A-E) |
| **Service-Specific Error Categories** | Day 6 EOD | ⬜ Pending | CRD Not Found, K8s API, Rego, Status Conflict, Audit |
| **Retry with Exponential Backoff** | Day 7 | ⬜ Pending | K8s API calls, audit writes |
| **Graceful Degradation** | Day 3 | ⬜ Pending | Degraded enrichment fallback |
| **Metrics Cardinality Audit** | Day 8 EOD | ⬜ Pending | Per DD-005 |
| **Integration Test Anti-Flaky** | Day 10 | ⬜ Pending | `Eventually()` pattern, 30s timeout |
| **Production Runbooks** | Day 12 | ⬜ Pending | 3 runbooks |

### **Day-by-Day Enhancement Application**

**Day 3** (K8s Enricher):
- [ ] Apply graceful degradation for K8s API failures (Category E)
- [ ] Implement context size validation (Risk #6 mitigation)

**Day 5** (Priority Engine):
- [ ] Implement Rego timeout and fallback (Risk #2 mitigation)
- [ ] Add mutex protection for policy hot-reload (Risk #5 mitigation)

**Day 6** (Error Handling EOD):
- [ ] Document all 5 error categories in Error Handling Philosophy
- [ ] Create error classification helper functions

**Day 7** (Reconciler):
- [ ] Implement exponential backoff for transient errors (Category B)
- [ ] Add optimistic locking for status updates (Category D)

**Day 8** (Metrics EOD):
- [ ] Complete Metrics Cardinality Audit per DD-005
- [ ] Verify all metrics have bounded label cardinality

**Day 10** (Testing):
- [ ] Apply anti-flaky patterns (`Eventually()`, 30s timeout)
- [ ] Test all edge case categories

**Day 12** (Production Readiness):
- [ ] Create 3 production runbooks
- [ ] Add Prometheus metrics for runbook automation

---

## 🔍 **Critical Checkpoints (From Gateway Learnings)**

### ✅ Checkpoint 1: Defense-in-Depth Testing (Days 8-10)
**Why**: Catches architectural issues early (2 days cheaper to fix)
**Action**: Write 5 integration tests before unit tests
**Evidence**: Gateway caught function signature mismatches early
**Signal Processing Application**:
- Test SignalProcessing CRD lifecycle (Pending → Enriching → Categorizing → Complete)
- Test Rego policy evaluation with real K8s context
- Test audit event creation via Data Storage Service

### ✅ Checkpoint 2: Schema Validation (Day 7 EOD)
**Why**: Prevents test failures from schema mismatches
**Action**: Validate 100% field alignment before testing
**Evidence**: Gateway added missing CRD fields, avoided test failures
**Signal Processing Application**:
- Validate SignalProcessingSpec matches CRD YAML
- Validate SignalProcessingStatus has all enrichment/classification fields
- Confirm KubernetesContext struct matches what K8s Enricher populates

### ✅ Checkpoint 3: BR Coverage Matrix (Day 10 EOD)
**Why**: Ensures all requirements have test coverage
**Action**: Map every BR to tests, justify any skipped
**Evidence**: Gateway achieved 100% BR coverage
**Signal Processing Application**:
- Map all 12 BRs to specific test files
- Ensure BR-SP-051/052/053 (environment) have dedicated tests
- Ensure BR-SP-070/071/072 (priority) have dedicated tests

### ✅ Checkpoint 4: Production Readiness (Day 12)
**Why**: Reduces production deployment issues
**Action**: Complete comprehensive readiness checklist
**Evidence**: Gateway deployment went smoothly
**Signal Processing Application**:
- Complete Production Readiness Report (Appendix C)
- Validate metrics endpoint serves 10+ metrics
- Verify graceful shutdown handles in-flight reconciliations

### ✅ Checkpoint 5: Daily Status Docs (Days 1, 4, 7, 12)
**Why**: Better progress tracking and handoffs
**Action**: Create progress documentation at key milestones
**Evidence**: Gateway handoff was smooth
**Signal Processing Application**:
- Day 1: `01-day1-complete.md` - Package structure, CRD types
- Day 4: `02-day4-midpoint.md` - Enricher + Environment Classifier
- Day 7: `03-day7-complete.md` - All classifiers + Reconciler
- Day 12: `00-HANDOFF-SUMMARY.md` - Complete handoff

---

## 📅 **Timeline Overview**

### **Phase Breakdown**

| Phase | Duration | Days | Purpose | Key Deliverables |
|-------|----------|------|---------|------------------|
| **ANALYSIS** | 4 hours | Day 0 | Context understanding | Analysis document, existing pattern review |
| **PLAN** | 4 hours | Day 0 | Implementation strategy | This document, TDD phase mapping |
| **Foundation** | 2 days | Days 1-2 | Package structure, CRD types | DD-006 scaffolding, API types |
| **Core Logic** | 4 days | Days 3-6 | Business logic components | Enrichment, classification, Rego (priority) |
| **Label Detection** | 2-3 days | Days 7-9 | OwnerChain, DetectedLabels, CustomLabels | DD-WORKFLOW-001 v1.9 ⭐ NEW |
| **Integration** | 2 days | Days 10-11 | Controller, metrics, audit | Complete CRD controller |
| **Testing** | 2 days | Days 12-13 | Unit → Integration → E2E | 70%+ coverage |
| **Finalization** | 2 days | Days 14-15 | Docs, user guides, Gateway cleanup | Production-ready |

### **14-17 Day Implementation Timeline** (Updated for DD-WORKFLOW-001 v1.9 + Gateway Migration v1.19)

| Day | Phase | Focus | Hours | Key Milestones |
|-----|-------|-------|-------|----------------|
| **Day 0** | ANALYSIS + PLAN | Pre-work | 8h | ✅ Analysis complete, Plan approved |
| **Day 1** | Foundation | DD-006 scaffolding | 8h | Package structure, main.go, config |
| **Day 2** | Foundation | CRD types, API | 8h | SignalProcessing CRD, types_test.go |
| **Day 3** | Core Logic | K8s Enricher | 8h | Kubernetes context fetching |
| **Day 4** | Core Logic | Environment Classifier | 8h | **PORT from Gateway** (478 LOC), Namespace labels, ConfigMap |
| **Day 5** | Core Logic | Priority Engine (Rego) | 8h | Fresh implementation (not Gateway port), uses `pkg/shared/hotreload/FileWatcher` per DD-INFRA-001 |
| **Day 6** | Core Logic | Business Classifier | 8h | Confidence scoring, multi-dimensional |
| **Day 7** | Label Detection ⭐ | OwnerChain | 8h | K8s ownerReference traversal |
| **Day 8** | Label Detection ⭐ | DetectedLabels | 8h | 8 auto-detection types (all in V1.0, PSS removed) |
| **Day 9** | Label Detection ⭐ | CustomLabels Rego | 8h | Rego extraction, security wrapper, ConfigMap |
| **Day 10** | Integration | Reconciler + Gateway Tests | 8h | SignalProcessingReconciler, **PORT Gateway tests** (857 LOC) |
| **Day 11** | Integration | Metrics, Audit | 8h | Prometheus metrics, audit client |
| **Day 12** | Testing | Unit Tests | 8h | 70%+ unit coverage (including ported tests) |
| **Day 13** | Testing | Integration + E2E | 8h | ENVTEST integration, Rego policy tests |
| **Day 14** | Finalization | Documentation | 8h | Service docs, **User documentation**, deployment guide |
| **Day 15** | Finalization | Gateway Cleanup + Buffer | 8h | Remove Gateway classification code, polish |

**Note**: Days 7-9 (Label Detection) added per DD-WORKFLOW-001 v1.9. Gateway migration moved earlier (Days 4-5, 10) per v1.19 timeline optimization.

**⚠️ Performance Testing**: Deferred to V1.1 (KIND API server limits prevent >100 concurrent signals).

---

## 📐 **Architecture**

### **Component Diagram**

```
                            ┌───────────────────────────────────────────┐
                            │       Remediation Orchestrator            │
                            │  (Creates SignalProcessing CRD)           │
                            └───────────────────┬───────────────────────┘
                                                │ Creates
                                                ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      Signal Processing Controller                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────┐                                                   │
│  │ SignalProcessing │  (Input CRD - created by RemediationOrchestrator) │
│  │   kubernaut.ai/  │                                                   │
│  │   v1alpha1       │                                                   │
│  └────────┬─────────┘                                                   │
│           │ Watches                                                      │
│           ▼                                                              │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │              SignalProcessingReconciler (Sequential Pipeline)     │   │
│  │                                                                   │   │
│  │  STEP 1: K8s Data Fetching (Signal-Driven, Standard Depth)       │   │
│  │  ┌────────────────────────────────────────────────────────────┐  │   │
│  │  │ K8s Enricher (based on signal.resourceKind)                │  │   │
│  │  │                                                            │  │   │
│  │  │  Pod signal    → Namespace + Pod + Node + Owner            │  │   │
│  │  │  Deploy signal → Namespace + Deployment                    │  │   │
│  │  │  SS signal     → Namespace + StatefulSet                   │  │   │
│  │  │  DS signal     → Namespace + DaemonSet                     │  │   │
│  │  │  Node signal   → Node only (no namespace)                  │  │   │
│  │  │  Unknown       → Namespace only (graceful fallback)        │  │   │
│  │  │                                                            │  │   │
│  │  │  OUTPUT: Raw KubernetesContext (no interpretation)         │  │   │
│  │  └──────────────────────────┬─────────────────────────────────┘  │   │
│  │                              │                                    │   │
│  │                              ▼                                    │   │
│  │  STEP 2-4: Customer-Defined Rego Policies (ALL classification)   │   │
│  │  ┌────────────────────────────────────────────────────────────┐  │   │
│  │  │                   Rego Policy Engine                        │  │   │
│  │  │  ┌─────────────────────────────────────────────────────┐   │  │   │
│  │  │  │ ConfigMap: kubernaut-rego-policies                   │   │  │   │
│  │  │  │  (Customer-defined, hot-reloadable)                  │   │  │   │
│  │  │  └─────────────────────────────────────────────────────┘   │  │   │
│  │  │                              │                              │  │   │
│  │  │      ┌───────────────────────┼───────────────────────┐     │  │   │
│  │  │      │                       │                       │     │  │   │
│  │  │      ▼                       ▼                       ▼     │  │   │
│  │  │ ┌──────────────┐   ┌──────────────┐   ┌──────────────┐    │  │   │
│  │  │ │ environment. │   │  priority.   │   │  business.   │    │  │   │
│  │  │ │    rego      │   │    rego      │   │    rego      │    │  │   │
│  │  │ │              │   │              │   │              │    │  │   │
│  │  │ │ Customer     │   │ Customer     │   │ Customer     │    │  │   │
│  │  │ │ defines how  │   │ defines how  │   │ defines how  │    │  │   │
│  │  │ │ to classify  │   │ to assign    │   │ to extract   │    │  │   │
│  │  │ │ environment  │   │ P0/P1/P2/P3  │   │ businessUnit │    │  │   │
│  │  │ └──────┬───────┘   └──────┬───────┘   └──────┬───────┘    │  │   │
│  │  │        │                  │                  │             │  │   │
│  │  │        ▼                  ▼                  ▼             │  │   │
│  │  │   Environment       Priority           Business           │  │   │
│  │  │   Classification    Assignment         Classification     │  │   │
│  │  │   + Confidence      + Confidence       + Confidence       │  │   │
│  │  └────────────────────────────────────────────────────────────┘  │   │
│  │                                                                   │   │
│  │  STEP 5: Recovery Context (from RemediationRequest embedded data) │   │
│  │  ┌────────────────────────────────────────────────────────────┐  │   │
│  │  │ Recovery Context Extractor                                  │  │   │
│  │  │  • Extract historical failure data from parent CRD         │  │   │
│  │  │  • No external API calls (DD-001)                          │  │   │
│  │  └────────────────────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
│  ┌──────────────────┐     ┌──────────────────┐                          │
│  │   Audit Client   │     │      Metrics     │                          │
│  │ (Data Storage)   │     │   (Prometheus)   │                          │
│  └────────┬─────────┘     └──────────────────┘                          │
│           │                                                              │
└───────────┼──────────────────────────────────────────────────────────────┘
            │                              │
            ▼                              │ Updates Status
    ┌───────────────┐                      ▼
    │ Data Storage  │        ┌───────────────────────────────────────────┐
    │  REST API     │        │       Remediation Orchestrator            │
    │ /api/v1/audit │        │  (Watches SignalProcessing completion)    │
    └───────────────┘        └───────────────────────────────────────────┘
```

### **Key Architecture Principle: Customer-Defined Classification**

**⚠️ CRITICAL**: Kubernaut does NOT hardcode classification logic. ALL classification is defined by the customer via Rego policies.

| Component | What it does | What it does NOT do |
|-----------|--------------|---------------------|
| **K8s Enricher** | Fetches raw K8s objects based on signal type (standard depth) | No classification, no interpretation, no configurable depth |
| **Rego Policies** | Customer-defined classification rules | No hardcoded labels or keywords |

**Why Rego Policies?**
1. **Customer-specific**: Every organization has different labeling conventions
2. **Hot-reloadable**: Update policies without restarting the controller
3. **Auditable**: Policy decisions can be logged and traced
4. **Testable**: Policies can be unit tested independently

**Why K8s Enricher + Rego (Not Rego Alone)?**

Per [ADR-041: Rego Policy Data Fetching Separation](../../../architecture/decisions/ADR-041-rego-policy-data-fetching-separation.md):

| Concern | K8s Enricher (Go) | Rego Policy Engine |
|---------|-------------------|-------------------|
| **Data Fetching** | ✅ Fetches K8s objects via client-go | ❌ Never fetches data |
| **Authentication** | ✅ Uses ServiceAccount + RBAC | ❌ No auth concerns |
| **Caching** | ✅ TTL cache for repeated lookups | ❌ Stateless evaluation |
| **Policy Evaluation** | ❌ No business logic | ✅ All classification rules |

Rego CAN make HTTP calls via `http.send`, but this was **rejected** for security (customer policies would have raw K8s API access), performance (no caching), and complexity (auth token management).

### **Key Components**

| Component | Purpose | Key Files |
|-----------|---------|-----------|
| **Reconciler** | Watches SignalProcessing CRD, orchestrates enrichment pipeline | `internal/controller/signalprocessing/reconciler.go` |
| **K8s Enricher** | Signal-driven K8s object fetching (standard depth, hardcoded) | `pkg/signalprocessing/enricher/k8s_enricher.go` |
| **Environment Classifier** | Classifies environment using Rego policies | `pkg/signalprocessing/classifier/environment.go` |
| **Priority Engine** | Assigns priority using Rego policies | `pkg/signalprocessing/classifier/priority.go` |
| **Business Classifier** | Multi-dimensional categorization with confidence | `pkg/signalprocessing/classifier/business.go` |
| **Audit Client** | Writes audit events to Data Storage Service | `pkg/signalprocessing/audit/client.go` |
| **Metrics** | Prometheus metrics (DD-005 compliant) | `pkg/signalprocessing/metrics/metrics.go` |

### **Data Flow**

**Upstream Flow** (before Signal Processing):
1. **Gateway**: Receives signal (Prometheus alert, K8s event, webhook)
2. **Gateway**: Creates RemediationRequest CRD with raw signal data
3. **RemediationOrchestrator**: Watches RemediationRequest CRD
4. **RemediationOrchestrator**: Creates SignalProcessing CRD in "Pending" phase

**Signal Processing Controller Flow**:
5. **Watch**: SignalProcessingReconciler watches SignalProcessing CRDs
6. **Enrich**: Fetch K8s context (namespace, deployment, pod, node)
7. **Classify**: Environment classification using Rego policies
8. **Prioritize**: Priority assignment using Rego policies with K8s context
9. **Business**: Multi-dimensional categorization with confidence scores
10. **Audit**: Write categorization audit event to Data Storage Service
11. **Status**: Update SignalProcessing CRD status to "Complete"

**Downstream Flow** (after Signal Processing):
12. **RemediationOrchestrator**: Watches SignalProcessing completion
13. **RemediationOrchestrator**: Creates AIAnalysis CRD for next phase

---

## 📆 **Day-by-Day Implementation Breakdown**

### **Day 0: ANALYSIS + PLAN (Pre-Work) ✅**

**Phase**: ANALYSIS + PLAN
**Duration**: 8 hours
**Status**: ✅ COMPLETE (this document represents Day 0 completion)

**Deliverables**:
- ✅ Analysis of existing Gateway classification code
- ✅ DD-CATEGORIZATION-001 migration assessment
- ✅ Implementation plan (this document v1.0)
- ✅ BR coverage matrix (12 BRs mapped)
- ✅ Architecture diagram and component design

---

### **Day 1: Foundation - DD-006 Scaffolding**

**Phase**: DO-DISCOVERY
**Duration**: 8 hours
**TDD Focus**: Create package structure using DD-006 templates

**Morning Tasks (4 hours)**:

**Hour 1-2: Scaffold Package Structure**
```bash
# Create directory structure
mkdir -p cmd/signalprocessing
mkdir -p pkg/signalprocessing/{config,metrics,enricher,classifier,audit}
mkdir -p internal/controller/signalprocessing
mkdir -p api/signalprocessing/v1alpha1
mkdir -p test/unit/signalprocessing
mkdir -p test/integration/signalprocessing
mkdir -p deploy/signalprocessing
```

**Hour 3-4: Copy and Customize DD-006 Templates**
1. **Copy** `docs/templates/crd-controller-gap-remediation/cmd-main-template.go.template` → `cmd/signalprocessing/main.go`
2. **Replace placeholders**:
   - `{{CONTROLLER_NAME}}` → `signalprocessing`
   - `{{PACKAGE_PATH}}` → `github.com/jordigilh/kubernaut/pkg/signalprocessing`
   - `{{CRD_GROUP}}` → `kubernaut.ai`
   - `{{CRD_VERSION}}` → `v1alpha1`
   - `{{CRD_KIND}}` → `SignalProcessing`

**Afternoon Tasks (4 hours)**:

**Hour 5-6: Configuration Package**
1. **Copy** `config-template.go.template` → `pkg/signalprocessing/config/config.go`
2. **Add Signal Processing specific config**:
```go
// Package config provides configuration types for Signal Processing controller.
package config

import (
    "time"

    "github.com/go-playground/validator/v10"
)

// Config holds all configuration for the Signal Processing controller.
// Note: MetricsAddr, HealthProbeAddr, and LeaderElection are NOT configurable
// in YAML - they are hardcoded or set via CLI flags for safety.
type Config struct {
    // Signal Processing specific configuration
    Enrichment EnrichmentConfig `yaml:"enrichment" validate:"required"`
    Classifier ClassifierConfig `yaml:"classifier" validate:"required"`
    Audit      AuditConfig      `yaml:"audit" validate:"required"`
}

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

// EnrichmentConfig configures K8s context enrichment behavior.
type EnrichmentConfig struct {
    // CacheTTL is how long to cache K8s context lookups.
    CacheTTL time.Duration `yaml:"cache_ttl" validate:"min=0"`

    // Timeout is the maximum time for enrichment operations.
    Timeout time.Duration `yaml:"timeout" validate:"required,min=1s"`
}

// ClassifierConfig configures Rego policy-based classification.
type ClassifierConfig struct {
    // RegoPolicyPath is the path to Rego policy files (for local development).
    RegoPolicyPath string `yaml:"rego_policy_path"`

    // RegoConfigMapName is the ConfigMap containing Rego policies.
    RegoConfigMapName string `yaml:"rego_configmap_name" validate:"required"`

    // RegoConfigMapKey is the key within the ConfigMap for policy content.
    RegoConfigMapKey string `yaml:"rego_configmap_key" validate:"required"`

    // HotReloadInterval is how often to check for policy updates.
    HotReloadInterval time.Duration `yaml:"hot_reload_interval" validate:"min=10s"`
}

// AuditConfig configures audit trail persistence via Data Storage Service.
// ADR-032, ADR-034: Audit is MANDATORY - no "enabled" flag.
type AuditConfig struct {
    // DataStorageURL is the base URL for Data Storage Service REST API.
    // ADR-032: All audit writes go through Data Storage Service (no direct DB access).
    DataStorageURL string `yaml:"data_storage_url" validate:"required,url"`

    // Timeout is the maximum time for audit write operations.
    // ADR-038: Fire-and-forget pattern means this is for buffer flush, not blocking.
    Timeout time.Duration `yaml:"timeout" validate:"required,min=1s"`

    // BufferSize is the in-memory buffer size for fire-and-forget audit writes.
    // ADR-038: Events buffered locally, flushed asynchronously.
    BufferSize int `yaml:"buffer_size" validate:"min=100,max=10000"`

    // FlushInterval is how often to flush buffered audit events.
    FlushInterval time.Duration `yaml:"flush_interval" validate:"min=1s,max=30s"`
}

// Validate validates the configuration using struct tags.
func (c *Config) Validate() error {
    validate := validator.New()
    return validate.Struct(c)
}
```

**Hour 7-8: Metrics Package**
1. **Copy** `metrics-template.go.template` → `pkg/signalprocessing/metrics/metrics.go`
2. **Add Signal Processing specific metrics** (triaged for business value):

> ⚠️ **Metrics Triage**: Reduced from 12 metrics to 6 based on business value analysis.
> Removed redundant metrics (Enrichment/Classification/Audit totals) - use ReconciliationTotal with labels instead.

```go
// Package metrics provides Prometheus metrics for Signal Processing controller.
// DD-005 compliant metrics implementation.
// Metrics triaged for business value - see implementation plan v1.4.
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

const (
    namespace = "kubernaut"
    subsystem = "signalprocessing"
)

// Metrics holds all Prometheus metrics for Signal Processing.
// Triaged for business value:
// - Core business metrics: ReconciliationTotal, ReconciliationDuration, CategorizationConfidence
// - SLO metrics: EnrichmentDuration (<2s P95), RegoEvaluationDuration (<100ms P95)
// - Operational metrics: RegoHotReloadTotal
type Metrics struct {
    // === CORE BUSINESS METRICS ===

    // ReconciliationTotal tracks all reconciliation operations.
    // Labels: phase (enriching, classifying, categorizing, complete, failed), status (success, failure)
    // Business value: Core throughput and success rate metric.
    ReconciliationTotal *prometheus.CounterVec

    // ReconciliationDuration measures end-to-end processing time.
    // Labels: phase
    // Business value: End-to-end latency for SLO tracking.
    ReconciliationDuration *prometheus.HistogramVec

    // CategorizationConfidence tracks confidence scores for all classifications.
    // Labels: classifier (environment, priority, business)
    // Business value: Are classifications reliable? Low confidence = review Rego policies.
    CategorizationConfidence *prometheus.HistogramVec

    // === SLO METRICS ===

    // EnrichmentDuration measures K8s API enrichment latency.
    // Labels: resource_kind (Pod, Deployment, Node, etc.)
    // SLO: <2 seconds P95 (BR-SP-001)
    EnrichmentDuration *prometheus.HistogramVec

    // RegoEvaluationDuration measures Rego policy evaluation time.
    // Labels: policy (environment, priority, business)
    // SLO: <100ms P95
    RegoEvaluationDuration *prometheus.HistogramVec

    // === OPERATIONAL METRICS ===

    // RegoHotReloadTotal tracks Rego policy hot-reload events.
    // Labels: status (success, failure)
    // Operational: Did policy updates succeed?
    RegoHotReloadTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all Prometheus metrics.
func NewMetrics() *Metrics {
    return &Metrics{
        // Core business metrics
        ReconciliationTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "reconciliation_total",
                Help:      "Total number of reconciliation operations by phase and status",
            },
            []string{"phase", "status"},
        ),
        ReconciliationDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "reconciliation_duration_seconds",
                Help:      "Duration of reconciliation operations in seconds",
                Buckets:   []float64{0.5, 1, 2, 5, 10, 30},
            },
            []string{"phase"},
        ),
        CategorizationConfidence: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "categorization_confidence",
                Help:      "Confidence scores for categorization decisions (0.0-1.0)",
                Buckets:   []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
            },
            []string{"classifier"},
        ),

        // SLO metrics
        EnrichmentDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "enrichment_duration_seconds",
                Help:      "Duration of K8s context enrichment operations (SLO: <2s P95)",
                Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5},
            },
            []string{"resource_kind"},
        ),
        RegoEvaluationDuration: promauto.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "rego_evaluation_duration_seconds",
                Help:      "Duration of Rego policy evaluations (SLO: <100ms P95)",
                Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
            },
            []string{"policy"},
        ),

        // Operational metrics
        RegoHotReloadTotal: promauto.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Subsystem: subsystem,
                Name:      "rego_hot_reload_total",
                Help:      "Total number of Rego policy hot-reload events",
            },
            []string{"status"},
        ),
    }
}
```

**Metrics Summary** (6 metrics, triaged for business value):

| Metric | Type | Business Value | SLO |
|--------|------|----------------|-----|
| `reconciliation_total` | Counter | ✅ Throughput, success rate | - |
| `reconciliation_duration_seconds` | Histogram | ✅ End-to-end latency | <5s P95 |
| `categorization_confidence` | Histogram | ✅ Classification reliability | >0.8 avg |
| `enrichment_duration_seconds` | Histogram | ✅ K8s API latency | <2s P95 |
| `rego_evaluation_duration_seconds` | Histogram | ✅ Policy evaluation speed | <100ms P95 |
| `rego_hot_reload_total` | Counter | ⚠️ Operational health | - |

**EOD Day 1 Checklist**:
- [ ] Package structure created
- [ ] `main.go` compiles (basic skeleton)
- [ ] `config.go` with validation
- [ ] `metrics.go` with DD-005 compliant metrics
- [ ] Zero lint errors

---

### **Day 2: Foundation - CRD Types**

**Phase**: DO-RED → DO-GREEN
**Duration**: 8 hours
**TDD Focus**: Define CRD types, then write business outcome tests in `test/unit/`

**Morning Tasks (4 hours)**:

**Hour 1-2: Define CRD Types (Pure Data Structures)**

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

> ⚠️ **IMPORTANT**: API types packages contain ONLY struct definitions - no validation methods, no business logic.
> Tests reside in `test/unit/signalprocessing/`, not alongside API types.

**Hour 3-4: CRD Types - No Separate Tests Required**

> **⚠️ IMPORTANT: CRD Types Do NOT Need Separate Unit Tests**
>
> Per [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md) and [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc):
>
> **Why No `types_test.go`:**
> 1. **Compile-time safety**: Go guarantees struct field existence at compile time
> 2. **Schema validation**: Kubernetes API server validates CRDs via OpenAPI spec
> 3. **Null-testing anti-pattern**: Testing struct initialization has zero business value
> 4. **Controller tests**: Business behavior is tested through controller reconciliation
>
> **What Gets Tested Instead:**
> - **Day 3-6**: Controller behavior tests (reconciliation, status updates)
> - **Day 8**: Unit tests - all components (parallel: 4 procs)
> - **Day 9**: Integration tests with real K8s API (envtest) (parallel: 4 procs)
> - **Day 10**: E2E tests with full workflow validation (parallel: 4 procs)
>
> **Validation Responsibilities:**
> | Concern | Mechanism | Location |
> |---------|-----------|----------|
> | Required fields | `// +kubebuilder:validation:Required` | CRD OpenAPI spec |
> | Field formats | `// +kubebuilder:validation:Pattern` | CRD OpenAPI spec |
> | Enum values | `// +kubebuilder:validation:Enum` | CRD OpenAPI spec |
> | Phase transitions | Reconciler logic | Controller tests |
> | Business outcomes | Controller behavior | Integration tests |

**Hour 5-6: Implement CRD Types (DO-GREEN)**

**File**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Environment",type=string,JSONPath=`.status.environmentClassification.environment`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.status.priorityAssignment.priority`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type SignalProcessing struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   SignalProcessingSpec   `json:"spec,omitempty"`
    Status SignalProcessingStatus `json:"status,omitempty"`
}

type SignalProcessingSpec struct {
    // Reference to parent RemediationRequest
    RemediationRequestRef ObjectReference `json:"remediationRequestRef"`

    // Signal data (copied from RemediationRequest for processing)
    Signal SignalData `json:"signal"`

    // Configuration for processing
    EnrichmentConfig EnrichmentConfig `json:"enrichmentConfig,omitempty"`
}

type SignalProcessingStatus struct {
    // Phase: Pending, Enriching, Classifying, Categorizing, Complete, Failed
    Phase SignalProcessingPhase `json:"phase"`

    // Processing timestamps
    StartTime      *metav1.Time `json:"startTime,omitempty"`
    CompletionTime *metav1.Time `json:"completionTime,omitempty"`

    // Enrichment results
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`
    RecoveryContext   *RecoveryContext   `json:"recoveryContext,omitempty"`

    // Categorization results (DD-CATEGORIZATION-001)
    EnvironmentClassification *EnvironmentClassification `json:"environmentClassification,omitempty"`
    PriorityAssignment        *PriorityAssignment        `json:"priorityAssignment,omitempty"`
    BusinessClassification    *BusinessClassification    `json:"businessClassification,omitempty"`

    // Conditions for detailed status
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // Error information
    Error string `json:"error,omitempty"`
}

// EnvironmentClassification from DD-CATEGORIZATION-001
type EnvironmentClassification struct {
    Environment      string  `json:"environment"`       // production, staging, development
    Confidence       float64 `json:"confidence"`        // 0.0-1.0 [DEPRECATED - Remove per DD-SP-001 V1.1]
    Source           string  `json:"source"`            // namespace-labels, rego-inference, default [signal-labels REMOVED - security per BR-SP-080 V2.0]
    ClassifiedAt     metav1.Time `json:"classifiedAt"`
}

// PriorityAssignment from DD-CATEGORIZATION-001
type PriorityAssignment struct {
    Priority         string  `json:"priority"`          // P0, P1, P2, P3
    Confidence       float64 `json:"confidence"`        // 0.0-1.0
    Source           string  `json:"source"`            // rego-policy, fallback-matrix
    PolicyName       string  `json:"policyName,omitempty"` // Which Rego rule matched
    AssignedAt       metav1.Time `json:"assignedAt"`
}

// BusinessClassification for multi-dimensional categorization
type BusinessClassification struct {
    BusinessUnit     string  `json:"businessUnit,omitempty"`
    ServiceOwner     string  `json:"serviceOwner,omitempty"`
    Criticality      string  `json:"criticality,omitempty"`  // critical, high, medium, low
    SLARequirement   string  `json:"slaRequirement,omitempty"` // 5m, 15m, 1h
    OverallConfidence float64 `json:"overallConfidence"`
}
```

**Afternoon Tasks (4 hours)**:

**Hour 5-6: Deep Copy and Register**
1. Run `make generate` to create `zz_generated.deepcopy.go`
2. Register types in scheme

**Hour 7-8: CRD Manifest Generation**
1. Run `make manifests` to generate CRD YAML
2. Verify CRD in `config/crd/bases/kubernaut.ai_signalprocessings.yaml`

**EOD Day 2 Checklist**:
- [ ] CRD types defined with all fields
- [ ] Types tests passing
- [ ] Deep copy generated
- [ ] CRD manifest generated
- [ ] Phase state machine validated

---

### **Days 3-6: Core Logic Components**

#### **Day 3: K8s Enricher**

**BR Coverage**: BR-SP-001 (K8s Context Enrichment)

**File**: `pkg/signalprocessing/enricher/k8s_enricher.go`

```go
// Package enricher provides Kubernetes context enrichment for signals.
package enricher

import (
    "context"
    "fmt"
    "time"

    "github.com/go-logr/logr"
    corev1 "k8s.io/api/core/v1"
    appsv1 "k8s.io/api/apps/v1"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"

    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/signalprocessing/cache"
    "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// K8sEnricher fetches Kubernetes context for signal enrichment.
// DD-005 v2.0: Uses logr.Logger (unified interface for all Kubernaut services)
type K8sEnricher struct {
    client  client.Client
    logger  logr.Logger      // DD-005 v2.0: logr.Logger (not *zap.Logger)
    cache   *cache.TTLCache  // TTL-based cache for repeated lookups
    metrics *metrics.Metrics
    timeout time.Duration
}

// NewK8sEnricher creates a new K8s context enricher.
// DD-005 v2.0: Accept logr.Logger from caller (CRD controller passes ctrl.Log)
func NewK8sEnricher(c client.Client, logger logr.Logger, m *metrics.Metrics, timeout time.Duration) *K8sEnricher {
    return &K8sEnricher{
        client:  c,
        logger:  logger.WithName("k8s-enricher"), // DD-005: .WithName() not .Named()
        cache:   cache.NewTTLCache(5 * time.Minute),
        metrics: m,
        timeout: timeout,
    }
}

// Enrich fetches Kubernetes context based on signal type (standard depth, hardcoded).
// BR-SP-001: <2 seconds P95
//
// Standard Depth Strategy (no configuration):
//   Pod signal    → Namespace + Pod + Node + Owner (Deployment/SS/DS)
//   Deploy signal → Namespace + Deployment
//   SS signal     → Namespace + StatefulSet
//   DS signal     → Namespace + DaemonSet
//   Node signal   → Node only (no namespace)
//   Unknown       → Namespace only (graceful fallback)
func (e *K8sEnricher) Enrich(ctx context.Context, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.KubernetesContext, error) {
    startTime := time.Now()
    defer func() {
        e.metrics.EnrichmentDuration.WithLabelValues("k8s_context").Observe(time.Since(startTime).Seconds())
    }()

    // Apply timeout
    ctx, cancel := context.WithTimeout(ctx, e.timeout)
    defer cancel()

    result := &signalprocessingv1alpha1.KubernetesContext{}

    // Signal-driven enrichment based on resource kind
    switch signal.TargetResource.Kind {
    case "Pod":
        return e.enrichPodSignal(ctx, signal, result)
    case "Deployment":
        return e.enrichDeploymentSignal(ctx, signal, result)
    case "StatefulSet":
        return e.enrichStatefulSetSignal(ctx, signal, result)
    case "DaemonSet":
        return e.enrichDaemonSetSignal(ctx, signal, result)
    case "ReplicaSet":
        return e.enrichReplicaSetSignal(ctx, signal, result)
    case "Node":
        return e.enrichNodeSignal(ctx, signal, result)
    default:
        // Graceful fallback: namespace only for unknown resource types
        return e.enrichNamespaceOnly(ctx, signal, result)
    }
}

// enrichPodSignal fetches Namespace + Pod + Node + Owner (standard depth).
func (e *K8sEnricher) enrichPodSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
    // 1. Fetch namespace
    ns, err := e.getNamespace(ctx, signal.Namespace)
    if err != nil {
        e.metrics.EnrichmentTotal.WithLabelValues("failure").Inc()
        return nil, fmt.Errorf("failed to get namespace %s: %w", signal.Namespace, err)
    }
    result.Namespace = ns

    // 2. Fetch pod
    pod, err := e.getPod(ctx, signal.Namespace, signal.Resource.Name)
    if err != nil {
        e.logger.Info("Pod not found, continuing with partial context", "error", err) // DD-005: key-value pairs
    } else {
        result.Pod = pod

        // 3. Fetch node where pod runs (standard depth)
        if pod.NodeName != "" {
            node, err := e.getNode(ctx, pod.NodeName)
            if err == nil {
                result.Node = node
            }
        }

        // 4. Fetch owner workload (Deployment/StatefulSet/DaemonSet)
        owner, err := e.getOwnerWorkload(ctx, signal.Namespace, pod.OwnerReferences)
        if err == nil {
            result.Owner = owner
        }
    }

    e.metrics.EnrichmentTotal.WithLabelValues("success").Inc()
    return result, nil
}

// enrichDeploymentSignal fetches Namespace + Deployment (no pods - standard depth).
func (e *K8sEnricher) enrichDeploymentSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
    // 1. Fetch namespace
    ns, err := e.getNamespace(ctx, signal.Namespace)
    if err != nil {
        e.metrics.EnrichmentTotal.WithLabelValues("failure").Inc()
        return nil, fmt.Errorf("failed to get namespace %s: %w", signal.Namespace, err)
    }
    result.Namespace = ns

    // 2. Fetch deployment
    deployment, err := e.getDeployment(ctx, signal.Namespace, signal.Resource.Name)
    if err != nil {
        e.logger.Info("Deployment not found, continuing with namespace only", "error", err) // DD-005: key-value pairs
    } else {
        result.Deployment = deployment
    }

    // NO pods fetched - ephemeral, expensive (standard depth decision)

    e.metrics.EnrichmentTotal.WithLabelValues("success").Inc()
    return result, nil
}

// enrichNodeSignal fetches Node only (no namespace for node signals).
func (e *K8sEnricher) enrichNodeSignal(ctx context.Context, signal *signalprocessingv1alpha1.SignalData, result *signalprocessingv1alpha1.KubernetesContext) (*signalprocessingv1alpha1.KubernetesContext, error) {
    // Node signals have no namespace
    node, err := e.getNode(ctx, signal.Resource.Name)
    if err != nil {
        e.metrics.EnrichmentTotal.WithLabelValues("failure").Inc()
        return nil, fmt.Errorf("failed to get node %s: %w", signal.Resource.Name, err)
    }
    result.Node = node

    e.metrics.EnrichmentTotal.WithLabelValues("success").Inc()
    return result, nil
}

// getNamespace fetches namespace with caching.
func (e *K8sEnricher) getNamespace(ctx context.Context, name string) (*signalprocessingv1alpha1.NamespaceContext, error) {
    // Check cache first
    if cached, ok := e.cache.Get("ns:" + name); ok {
        return cached.(*signalprocessingv1alpha1.NamespaceContext), nil
    }

    ns := &corev1.Namespace{}
    if err := e.client.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
        return nil, err
    }

    result := &signalprocessingv1alpha1.NamespaceContext{
        Name:        ns.Name,
        Labels:      ns.Labels,
        Annotations: ns.Annotations,
    }

    e.cache.Set("ns:"+name, result)
    return result, nil
}
```

**Tests**: `test/unit/signalprocessing/enricher_test.go`

---

#### **Day 4: Environment Classifier (Rego)**

**BR Coverage**: BR-SP-051, BR-SP-052, BR-SP-053

**File**: `pkg/signalprocessing/classifier/environment.go`

```go
type EnvironmentClassifier struct {
    regoQuery        *rego.PreparedEvalQuery // Prepared query for performance
    k8sClient        client.Client           // For ConfigMap fallback (BR-SP-052)
    logger           logr.Logger             // DD-005 v2.0: logr.Logger (not *zap.Logger)
    configMapMu      sync.RWMutex            // Thread safety for ConfigMap cache
    configMapMapping map[string]string       // Namespace pattern → environment mapping
}

// Classify determines environment using Rego policy
// 🚨 SECURITY UPDATE (BR-SP-080 V2.0): Priority order: namespace labels → Rego pattern matching → default
// REMOVED: signal labels (security vulnerability - untrusted external source per DD-SP-001 V1.1)
func (c *EnvironmentClassifier) Classify(ctx context.Context, k8sCtx *KubernetesContext, signal *SignalData) (*EnvironmentClassification, error) {
    input := map[string]interface{}{
        "namespace": map[string]interface{}{
            "name":   k8sCtx.Namespace.Name,
            "labels": k8sCtx.Namespace.Labels,
        },
        "signal": map[string]interface{}{
            "labels": signal.Labels,
        },
    }

    results, err := c.regoQuery.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        // Fallback to default
        return &EnvironmentClassification{
            Environment: "unknown",
            Confidence:  0.0,
            Source:      "default",
        }, nil
    }

    // Extract result from Rego
    env := results[0].Expressions[0].Value.(map[string]interface{})
    return &EnvironmentClassification{
        Environment: env["environment"].(string),
        Confidence:  env["confidence"].(float64),
        Source:      env["source"].(string),
    }, nil
}
```

**Rego Policy**: `deploy/signalprocessing/policies/environment.rego`
```rego
package signalprocessing.environment

# Primary: Namespace labels (kubernaut.ai/environment)
# Per BR-SP-051: Only kubernaut.ai/ prefixed labels
# Per BR-SP-051: Case-insensitive matching via lower() function
result := {"environment": lower(env), "confidence": 0.95, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
}

# Default fallback (when namespace label not present)
# Returns "unknown" so Go code can try ConfigMap (BR-SP-052) and signal labels
result := {"environment": "unknown", "confidence": 0.0, "source": "default"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
}
```

**Note**: Signal labels fallback (confidence 0.80) is handled in Go code after ConfigMap check
to maintain correct priority order: namespace labels → ConfigMap → signal labels → default

---

#### **Day 5: Priority Engine (Rego)**

**BR Coverage**: BR-SP-070, BR-SP-071, BR-SP-072

**File**: `pkg/signalprocessing/classifier/priority.go`

**Dependencies**:
- `pkg/shared/hotreload/FileWatcher` - Shared fsnotify-based hot-reloader (DD-INFRA-001)
- `github.com/open-policy-agent/opa/v1/rego` - Rego policy evaluation

**Imports**:
```go
import (
    "context"
    "fmt"
    "strings"
    "sync"

    "github.com/go-logr/logr"
    "github.com/open-policy-agent/opa/v1/rego"

    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)
```

```go
const (
    // Per BR-SP-070: P95 evaluation latency < 100ms
    regoEvalTimeout = 100 * time.Millisecond
    // Valid priority levels per BR-SP-071
    validPriorities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
)

type PriorityEngine struct {
    regoQuery   *rego.PreparedEvalQuery
    fileWatcher *hotreload.FileWatcher // Per DD-INFRA-001: fsnotify-based
    policyPath  string                 // Path to mounted ConfigMap file
    logger      logr.Logger            // DD-005 v2.0: logr.Logger (not *zap.Logger)
    mu          sync.RWMutex           // Protects regoQuery during hot-reload
}

// NewPriorityEngine creates a new Rego-based priority engine.
// Per BR-SP-070, BR-SP-071, BR-SP-072 specifications.
func NewPriorityEngine(ctx context.Context, policyPath string, logger logr.Logger) (*PriorityEngine, error) {
    log := logger.WithName("priority-engine")

    // Read and compile initial policy
    policyContent, err := os.ReadFile(policyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
    }

    query, err := rego.New(
        rego.Query("data.signalprocessing.priority.result"),
        rego.Module(filepath.Base(policyPath), string(policyContent)),
    ).PrepareForEval(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to compile Rego policy: %w", err)
    }

    return &PriorityEngine{
        regoQuery:  &query,
        policyPath: policyPath,
        logger:     log,
    }, nil
}

// Assign determines priority using Rego policy with K8s + business context
// BR-SP-070: Rego policies with rich context
// BR-SP-071: Fallback on timeout (>100ms) or error
// Input schema per BR-SP-070 (no replicas/minReplicas/conditions)
func (p *PriorityEngine) Assign(ctx context.Context, k8sCtx *KubernetesContext, envClass *EnvironmentClassification, signal *SignalData) (*PriorityAssignment, error) {
    // Validate inputs (PE-ER-03)
    if envClass == nil {
        return nil, fmt.Errorf("environment classification is required")
    }
    if signal == nil {
        return nil, fmt.Errorf("signal data is required")
    }

    // Build input per BR-SP-070 schema with nil checks
    input := p.buildRegoInput(k8sCtx, envClass, signal)

    // Add timeout per BR-SP-071 (>100ms triggers fallback)
    timeoutCtx, cancel := context.WithTimeout(ctx, regoEvalTimeout)
    defer cancel()

    p.mu.RLock()
    query := p.regoQuery
    p.mu.RUnlock()

    results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
    if err != nil {
        // BR-SP-071: Fallback based on severity ONLY (not environment)
        p.logger.Info("Rego evaluation failed, using fallback", "error", err)
        return p.fallbackBySeverity(signal.Severity), nil
    }

    // Check for empty results
    if len(results) == 0 || len(results[0].Expressions) == 0 {
        p.logger.Info("Rego returned no results, using fallback")
        return p.fallbackBySeverity(signal.Severity), nil
    }

    // Extract and validate Rego output
    return p.extractAndValidateResult(results, signal.Severity)
}

// buildRegoInput constructs the input map with nil checks.
func (p *PriorityEngine) buildRegoInput(k8sCtx *KubernetesContext, envClass *EnvironmentClassification, signal *SignalData) map[string]interface{} {
    input := map[string]interface{}{
        "signal": map[string]interface{}{
            "severity": signal.Severity,
            "source":   signal.Source,
        },
        "environment": envClass.Environment,
    }

    // Nil checks for nested K8s context
    if k8sCtx != nil {
        if k8sCtx.Namespace != nil {
            input["namespace_labels"] = ensureLabelsMap(k8sCtx.Namespace.Labels)
        } else {
            input["namespace_labels"] = map[string]interface{}{}
        }
        if k8sCtx.Deployment != nil {
            input["deployment_labels"] = ensureLabelsMap(k8sCtx.Deployment.Labels)
        } else {
            input["deployment_labels"] = map[string]interface{}{}
        }
    } else {
        input["namespace_labels"] = map[string]interface{}{}
        input["deployment_labels"] = map[string]interface{}{}
    }

    return input
}

// extractAndValidateResult extracts and validates Rego output.
// PE-ER-04, PE-ER-05: Validate priority is P0-P3
func (p *PriorityEngine) extractAndValidateResult(results rego.ResultSet, severity string) (*PriorityAssignment, error) {
    resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
    if !ok {
        p.logger.Info("Invalid Rego output type, using fallback")
        return p.fallbackBySeverity(severity), nil
    }

    priority, _ := resultMap["priority"].(string)
    policyName, _ := resultMap["policy_name"].(string)
    confidence := extractConfidence(resultMap["confidence"])

    // Validate priority is P0-P3 (PE-ER-04, PE-ER-05)
    if !validPriorities[priority] {
        return nil, fmt.Errorf("invalid priority value: %s (must be P0, P1, P2, or P3)", priority)
    }

    return &PriorityAssignment{
        Priority:   priority,
        Confidence: confidence,
        Source:     "rego-policy",
        PolicyName: policyName,
    }, nil
}

// fallbackBySeverity returns priority based on severity only (BR-SP-071)
// Used when Rego fails - environment is NOT considered in fallback
func (p *PriorityEngine) fallbackBySeverity(severity string) *PriorityAssignment {
    var priority string
    switch strings.ToLower(severity) {
    case "critical":
        priority = "P1" // Conservative - high but not highest without context
    case "warning":
        priority = "P2"
    case "info":
        priority = "P3"
    default:
        priority = "P2" // Default when severity unknown
    }
    p.logger.Info("Using severity-based fallback", "severity", severity, "priority", priority)
    return &PriorityAssignment{
        Priority:   priority,
        Confidence: 0.6, // Reduced confidence for fallback
        Source:     "fallback-severity",
    }
}

// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
// Per DD-INFRA-001: Uses shared FileWatcher component
func (p *PriorityEngine) StartHotReload(ctx context.Context) error {
    var err error
    p.fileWatcher, err = hotreload.NewFileWatcher(
        p.policyPath, // e.g., "/etc/kubernaut/policies/priority.rego"
        func(content string) error {
            // Compile new Rego policy
            newQuery, err := rego.New(
                rego.Query("data.signalprocessing.priority.result"),
                rego.Module("priority.rego", content),
            ).PrepareForEval(ctx)
            if err != nil {
                return fmt.Errorf("Rego compilation failed: %w", err)
            }

            // Atomically swap policy
            p.mu.Lock()
            p.regoQuery = &newQuery
            p.mu.Unlock()

            p.logger.Info("Rego policy hot-reloaded successfully")
            return nil
        },
        p.logger,
    )
    if err != nil {
        return fmt.Errorf("failed to create file watcher: %w", err)
    }

    return p.fileWatcher.Start(ctx)
}

// Stop gracefully stops the hot-reloader
func (p *PriorityEngine) Stop() {
    if p.fileWatcher != nil {
        p.fileWatcher.Stop()
    }
}
```

---

#### **Day 6: Business Classifier**

**BR Coverage**: BR-SP-002, BR-SP-080, BR-SP-081

**File**: `pkg/signalprocessing/classifier/business.go`

**Dependencies**:
- `github.com/open-policy-agent/opa/v1/rego` - Rego policy evaluation

**Label Keys** (for direct label detection per BR-SP-002):
- `kubernaut.ai/business-unit` → BusinessUnit (confidence 1.0)
- `kubernaut.ai/service-owner` → ServiceOwner (confidence 1.0)
- `kubernaut.ai/criticality` → Criticality (confidence 1.0)
- `kubernaut.ai/sla-tier` → SLARequirement (confidence 1.0)

```go
const (
    // Per BR-SP-080: Rego evaluation timeout
    businessRegoTimeout = 200 * time.Millisecond

    // BR-SP-080: Confidence levels by detection method
    confidenceExplicitLabel = 1.0  // Explicit label match
    confidencePatternMatch  = 0.8  // Pattern match (namespace prefix)
    confidenceRegoInference = 0.6  // Rego policy inference
    confidenceDefault       = 0.4  // Default fallback

    // Label keys per BR-SP-002
    labelBusinessUnit  = "kubernaut.ai/business-unit"
    labelServiceOwner  = "kubernaut.ai/service-owner"
    labelCriticality   = "kubernaut.ai/criticality"
    labelSLATier       = "kubernaut.ai/sla-tier"
)

// Valid enum values per BR-SP-081
//
// Criticality levels: standard incident/alert severity classification
//
// SLA Tier naming: Industry-standard "metallic tier" model (ITIL, MSP, Insurance)
// Used by: ITIL frameworks, managed service providers, enterprise SLAs
// Order: platinum (highest) > gold > silver > bronze (lowest/default)
//
// Alternative naming conventions (not used here, documented for reference):
// - Cloud Providers: Basic, Standard, Premium, Enterprise (AWS, Azure, GCP support tiers)
// - Numeric: Tier 1, Tier 2, Tier 3, Tier 4
// - Priority: Critical, High, Medium, Low
//
// Kubernaut uses metallic tiers as they're widely recognized and imply service quality expectations.
var (
    validCriticality = map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
    validSLATier     = map[string]bool{"platinum": true, "gold": true, "silver": true, "bronze": true}
)

type BusinessClassifier struct {
    regoQuery  *rego.PreparedEvalQuery
    policyPath string
    logger     logr.Logger // DD-005 v2.0: logr.Logger (not *zap.Logger)
    mu         sync.RWMutex
}

// NewBusinessClassifier creates a new business classifier.
// Per BR-SP-002, BR-SP-080, BR-SP-081 specifications.
func NewBusinessClassifier(ctx context.Context, policyPath string, logger logr.Logger) (*BusinessClassifier, error) {
    log := logger.WithName("business-classifier")

    policyContent, err := os.ReadFile(policyPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
    }

    query, err := rego.New(
        rego.Query("data.signalprocessing.business.result"),
        rego.Module(filepath.Base(policyPath), string(policyContent)),
    ).PrepareForEval(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to compile Rego policy: %w", err)
    }

    return &BusinessClassifier{
        regoQuery:  &query,
        policyPath: policyPath,
        logger:     log,
    }, nil
}

// Classify performs multi-dimensional business categorization.
// Per BR-SP-002: Classification from namespace/deployment labels OR Rego policies.
// Per BR-SP-080: 4-tier confidence scoring (1.0 label → 0.8 pattern → 0.6 Rego → 0.4 default)
// Per BR-SP-081: businessUnit, serviceOwner, criticality, sla dimensions
//
// NOTE: priority is NOT an input - business classification is independent of priority assignment.
func (b *BusinessClassifier) Classify(ctx context.Context, k8sCtx *KubernetesContext, envClass *EnvironmentClassification) (*BusinessClassification, error) {
    // Validate inputs
    if k8sCtx == nil {
        return nil, fmt.Errorf("kubernetes context is required")
    }

    result := &BusinessClassification{}

    // BR-SP-080: 4-tier detection with confidence scoring
    // Tier 1: Explicit label match (confidence 1.0)
    b.classifyFromLabels(k8sCtx, result)

    // Tier 2: Pattern match (confidence 0.8) - if label detection incomplete
    if result.BusinessUnit == "" || result.ServiceOwner == "" {
        b.classifyFromPatterns(k8sCtx, result)
    }

    // Tier 3: Rego inference (confidence 0.6) - for remaining fields
    if b.needsRegoClassification(result) {
        if err := b.classifyFromRego(ctx, k8sCtx, envClass, result); err != nil {
            b.logger.Info("Rego classification failed, using defaults", "error", err)
        }
    }

    // Tier 4: Default fallback (confidence 0.4) - for any remaining unknown fields
    b.applyDefaults(result)

    // Calculate overall confidence (weighted average)
    result.OverallConfidence = b.calculateOverallConfidence(result)

    return result, nil
}

// classifyFromLabels extracts business fields from explicit kubernaut.ai/ labels.
// Per BR-SP-002 + BR-SP-080: Label-based classification (confidence 1.0)
func (b *BusinessClassifier) classifyFromLabels(k8sCtx *KubernetesContext, result *BusinessClassification) {
    labels := b.collectLabels(k8sCtx)

    if val, ok := labels[labelBusinessUnit]; ok && val != "" {
        result.BusinessUnit = val
        result.businessUnitConfidence = confidenceExplicitLabel
    }
    if val, ok := labels[labelServiceOwner]; ok && val != "" {
        result.ServiceOwner = val
        result.serviceOwnerConfidence = confidenceExplicitLabel
    }
    if val, ok := labels[labelCriticality]; ok && val != "" {
        if validCriticality[strings.ToLower(val)] {
            result.Criticality = strings.ToLower(val)
            result.criticalityConfidence = confidenceExplicitLabel
        }
    }
    if val, ok := labels[labelSLATier]; ok && val != "" {
        if validSLATier[strings.ToLower(val)] {
            result.SLARequirement = strings.ToLower(val)
            result.slaConfidence = confidenceExplicitLabel
        }
    }
}

// classifyFromPatterns uses namespace naming patterns.
// Per BR-SP-080: Pattern match (confidence 0.8)
// Examples: "payments-prod" → business_unit="payments", "billing-staging" → "billing"
func (b *BusinessClassifier) classifyFromPatterns(k8sCtx *KubernetesContext, result *BusinessClassification) {
    if k8sCtx.Namespace == nil {
        return
    }

    nsName := k8sCtx.Namespace.Name
    parts := strings.Split(nsName, "-")
    if len(parts) > 0 && result.BusinessUnit == "" {
        // First segment before hyphen as potential business unit
        potentialUnit := parts[0]
        if len(potentialUnit) > 2 { // Avoid short prefixes like "ns"
            result.BusinessUnit = potentialUnit
            result.businessUnitConfidence = confidencePatternMatch
        }
    }
}

// classifyFromRego evaluates business Rego policy for inference.
// Per BR-SP-080: Rego inference (confidence 0.6)
func (b *BusinessClassifier) classifyFromRego(ctx context.Context, k8sCtx *KubernetesContext, envClass *EnvironmentClassification, result *BusinessClassification) error {
    input := b.buildRegoInput(k8sCtx, envClass)

    timeoutCtx, cancel := context.WithTimeout(ctx, businessRegoTimeout)
    defer cancel()

    b.mu.RLock()
    query := b.regoQuery
    b.mu.RUnlock()

    results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
    if err != nil {
        return err
    }

    if len(results) == 0 || len(results[0].Expressions) == 0 {
        return nil // No results, will use defaults
    }

    return b.extractRegoResults(results, result)
}

// extractRegoResults safely extracts Rego output with type checking.
func (b *BusinessClassifier) extractRegoResults(results rego.ResultSet, result *BusinessClassification) error {
    resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
    if !ok {
        return fmt.Errorf("invalid Rego output type")
    }

    // Safe extraction with validation
    if val, ok := resultMap["business_unit"].(string); ok && val != "" && result.BusinessUnit == "" {
        result.BusinessUnit = val
        result.businessUnitConfidence = confidenceRegoInference
    }
    if val, ok := resultMap["service_owner"].(string); ok && val != "" && result.ServiceOwner == "" {
        result.ServiceOwner = val
        result.serviceOwnerConfidence = confidenceRegoInference
    }
    if val, ok := resultMap["criticality"].(string); ok && val != "" && result.Criticality == "" {
        if validCriticality[strings.ToLower(val)] {
            result.Criticality = strings.ToLower(val)
            result.criticalityConfidence = confidenceRegoInference
        }
    }
    if val, ok := resultMap["sla"].(string); ok && val != "" && result.SLARequirement == "" {
        if validSLATier[strings.ToLower(val)] {
            result.SLARequirement = strings.ToLower(val)
            result.slaConfidence = confidenceRegoInference
        }
    }

    return nil
}

// applyDefaults sets "unknown" for any unclassified fields.
// Per BR-SP-081: "unknown" if not determinable
func (b *BusinessClassifier) applyDefaults(result *BusinessClassification) {
    if result.BusinessUnit == "" {
        result.BusinessUnit = "unknown"
        result.businessUnitConfidence = confidenceDefault
    }
    if result.ServiceOwner == "" {
        result.ServiceOwner = "unknown"
        result.serviceOwnerConfidence = confidenceDefault
    }
    if result.Criticality == "" {
        result.Criticality = "medium" // Safe default
        result.criticalityConfidence = confidenceDefault
    }
    if result.SLARequirement == "" {
        result.SLARequirement = "bronze" // Lowest tier default
        result.slaConfidence = confidenceDefault
    }
}

// calculateOverallConfidence computes weighted average confidence.
func (b *BusinessClassifier) calculateOverallConfidence(result *BusinessClassification) float64 {
    sum := result.businessUnitConfidence + result.serviceOwnerConfidence +
           result.criticalityConfidence + result.slaConfidence
    return sum / 4.0
}

// Helper methods
func (b *BusinessClassifier) collectLabels(k8sCtx *KubernetesContext) map[string]string {
    labels := make(map[string]string)
    if k8sCtx.Namespace != nil {
        for k, v := range k8sCtx.Namespace.Labels {
            labels[k] = v
        }
    }
    if k8sCtx.Deployment != nil {
        for k, v := range k8sCtx.Deployment.Labels {
            labels[k] = v
        }
    }
    return labels
}

func (b *BusinessClassifier) needsRegoClassification(result *BusinessClassification) bool {
    return result.BusinessUnit == "" || result.ServiceOwner == "" ||
           result.Criticality == "" || result.SLARequirement == ""
}

func (b *BusinessClassifier) buildRegoInput(k8sCtx *KubernetesContext, envClass *EnvironmentClassification) map[string]interface{} {
    input := map[string]interface{}{
        "environment": "",
    }
    if envClass != nil {
        input["environment"] = envClass.Environment
    }
    if k8sCtx != nil && k8sCtx.Namespace != nil {
        input["namespace"] = map[string]interface{}{
            "name":        k8sCtx.Namespace.Name,
            "labels":      ensureLabelsMap(k8sCtx.Namespace.Labels),
            "annotations": ensureLabelsMap(k8sCtx.Namespace.Annotations),
        }
    }
    if k8sCtx != nil && k8sCtx.Deployment != nil {
        input["deployment"] = map[string]interface{}{
            "labels":      ensureLabelsMap(k8sCtx.Deployment.Labels),
            "annotations": ensureLabelsMap(k8sCtx.Deployment.Annotations),
        }
    }
    return input
}
```

**NOTE**: The `BusinessClassification` struct needs internal confidence tracking fields (not in CRD, just for calculation):
```go
// Internal tracking (not exported to CRD)
type classificationWithConfidence struct {
    *BusinessClassification
    businessUnitConfidence  float64
    serviceOwnerConfidence  float64
    criticalityConfidence   float64
    slaConfidence           float64
}
```

---

#### **Day 6 EOD: Error Handling Philosophy Document** ⭐

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/design/ERROR_HANDLING_PHILOSOPHY.md`

**MANDATORY**: Create this document at end of Day 6 to establish consistent error handling across all Signal Processing components.

```markdown
# Error Handling Philosophy - Signal Processing Service

**Date**: [Implementation Date]
**Status**: ✅ Authoritative Guide
**Version**: 1.0

---

#### 🎯 **Core Principles**

##### 1. **Error Classification**

#### **Transient Errors** (Retry-able)
- **Definition**: Temporary failures that may succeed on retry
- **Examples**: K8s API timeouts, Data Storage Service 503, network errors
- **Strategy**: Exponential backoff with jitter (requeue with delay)
- **Max Retries**: 5 attempts (30s, 60s, 120s, 240s, 480s)

#### **Permanent Errors** (Non-retry-able)
- **Definition**: Failures that will not succeed on retry
- **Examples**: Invalid CRD spec, missing required fields, Rego policy syntax error
- **Strategy**: Fail immediately, update status.phase = Failed, log error
- **Max Retries**: 0 (no retry)

#### **Partial Errors** (Graceful Degradation)
- **Definition**: Some operations succeed while others fail
- **Examples**: K8s API returns partial namespace data, one classifier fails
- **Strategy**: Continue with available data, set confidence = 0.5 for failed components
- **Max Retries**: N/A (proceed with degraded results)

---

##### 2. **Signal Processing-Specific Error Categories (A-E)**

> **Source**: Adapted from Notification Controller v3.2 patterns

#### **Category A: SignalProcessing CR Not Found**
- **When**: CRD deleted during reconciliation (race condition)
- **Action**: Log deletion, return without error (normal cleanup)
- **Recovery**: Automatic (Kubernetes garbage collection)
- **Metric**: `signalprocessing_reconciliation_total{result="not_found"}`

```go
// Category A: Handle CR deleted during reconciliation
if apierrors.IsNotFound(err) {
    log.Info("SignalProcessing CR deleted during reconciliation, skipping")
    return ctrl.Result{}, nil // No requeue, normal cleanup
}
```

#### **Category B: K8s API Errors** (Retry with Backoff)
- **When**: K8s API timeouts, 503 errors, rate limiting (429)
- **Action**: Exponential backoff (30s → 60s → 120s → 240s → 480s)
- **Recovery**: Automatic retry up to 5 attempts, then mark as failed
- **Metric**: `signalprocessing_k8s_api_errors_total{error_type="..."}`

```go
// Category B: K8s API transient error
if isTransientK8sError(err) {
    log.Error(err, "K8s API transient error, will retry",
        "attempt", attemptCount, "backoff", CalculateBackoff(attemptCount))
    return HandleTransientError(attemptCount), nil
}
```

#### **Category C: Rego Policy Errors** (User Configuration Error)
- **When**: Rego syntax error, invalid policy output, policy not found
- **Action**: Mark as failed immediately, create Kubernetes Event
- **Recovery**: Manual (fix Rego policy in ConfigMap, controller will re-evaluate)
- **Metric**: `signalprocessing_rego_policy_errors_total{policy="...",error_type="..."}`

```go
// Category C: Rego policy user error (permanent)
if isRegoPolicyError(err) {
    log.Error(err, "Rego policy configuration error - manual intervention required",
        "policy", policyName, "error", err.Error())
    r.recorder.Event(sp, corev1.EventTypeWarning, "RegoPolicyError",
        fmt.Sprintf("Rego policy %s has configuration error: %v", policyName, err))
    return HandlePermanentError(), r.updateStatusFailed(ctx, sp, err)
}
```

#### **Category D: Status Update Conflicts** (Optimistic Locking)
- **When**: Multiple reconcile attempts updating status simultaneously
- **Action**: Retry status update with fresh resource version (3 attempts)
- **Recovery**: Automatic (retry with latest version)
- **Metric**: `signalprocessing_status_update_conflicts_total`

```go
// Category D: Status update with retry for conflicts
func (r *SignalProcessingReconciler) updateStatusWithRetry(ctx context.Context, sp *v1alpha1.SignalProcessing) error {
    return retry.RetryOnConflict(retry.DefaultRetry, func() error {
        // Get fresh version
        fresh := &v1alpha1.SignalProcessing{}
        if err := r.Get(ctx, client.ObjectKeyFromObject(sp), fresh); err != nil {
            return err
        }
        fresh.Status = sp.Status
        return r.Status().Update(ctx, fresh)
    })
}
```

#### **Category E: Enrichment/Classification Failures** (Partial Data)
- **When**: K8s enrichment partially succeeds, one classifier fails while others succeed
- **Action**: Continue with available data, set component confidence to 0.0
- **Recovery**: Automatic (degraded results are acceptable for non-critical data)
- **Metric**: `signalprocessing_partial_success_total{component="..."}`

```go
// Category E: Partial enrichment (graceful degradation)
k8sContext, err := r.enricher.EnrichSignal(ctx, signal)
if err != nil {
    if isPartialEnrichmentError(err) {
        log.Info("Partial K8s enrichment - proceeding with degraded context",
            "missing_fields", err.(*PartialEnrichmentError).MissingFields)
        k8sContext = err.(*PartialEnrichmentError).PartialContext
        k8sContext.Confidence = 0.5 // Reduced confidence
    } else {
        return HandleTransientError(attemptCount), err
    }
}
```

---

#### 🔄 **Retry Strategy for CRD Controller**

### Requeue with Backoff

```go
package controller

import (
    "math"
    "math/rand"
    "time"

    ctrl "sigs.k8s.io/controller-runtime"
)

// CalculateBackoff returns exponential backoff duration for controller requeue.
// Attempts: 0→30s, 1→60s, 2→120s, 3→240s, 4+→480s (capped)
func CalculateBackoff(attemptCount int) time.Duration {
    baseDelay := 30 * time.Second
    maxDelay := 480 * time.Second

    delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attemptCount)))
    if delay > maxDelay {
        delay = maxDelay
    }

    // Add jitter (±10%) to prevent thundering herd
    jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))
    return jitter
}

// HandleTransientError returns a requeue result with backoff.
func HandleTransientError(attemptCount int) ctrl.Result {
    return ctrl.Result{
        RequeueAfter: CalculateBackoff(attemptCount),
    }
}

// HandlePermanentError returns a result that does NOT requeue.
func HandlePermanentError() ctrl.Result {
    return ctrl.Result{Requeue: false}
}
```

### Retry Decision Matrix

| Error Type | HTTP/K8s Status | Retry? | Backoff | Max Attempts | Example |
|-----------|-----------------|--------|---------|--------------|---------|
| Transient | K8s API timeout | ✅ Yes | Exponential | 5 | `context deadline exceeded` |
| Transient | Data Storage 503 | ✅ Yes | Exponential | 5 | Service temporarily unavailable |
| Transient | K8s API 429 | ✅ Yes | Exponential | 5 | Rate limited |
| Permanent | K8s API 404 | ❌ No | N/A | 0 | Namespace not found |
| Permanent | Validation | ❌ No | N/A | 0 | Missing required field |
| Permanent | Rego syntax | ❌ No | N/A | 0 | Invalid policy syntax |
| Partial | K8s partial | ⚠️ Continue | N/A | N/A | Some fields unavailable |

---

### Backoff Progression Table (Signal Processing Controller)

| Attempt | Backoff (base) | With Jitter (±10%) | Cumulative Time |
|---------|----------------|---------------------|-----------------|
| 0 (initial fail) | 30s | 27s - 33s | ~30s |
| 1 | 60s | 54s - 66s | ~90s |
| 2 | 120s | 108s - 132s | ~210s |
| 3 | 240s | 216s - 264s | ~450s |
| 4+ | 480s (capped) | 432s - 528s | ~930s (~15.5 min) |

**Total retry window**: ~15 minutes before permanent failure status
**K8s Requeue**: Uses `ctrl.Result{RequeueAfter: backoff}` (native K8s pattern)

---

### ⚠️ Circuit Breaker: NOT APPLICABLE for CRD Controllers

**Why Notification uses Circuit Breaker**:
- External API calls (Slack, email SMTP) can overload external services
- Channel-specific isolation (Slack failure shouldn't affect console)
- Thundering herd protection during external outages

**Why Signal Processing does NOT use Circuit Breaker**:
- ✅ **K8s API is internal** - not an external service to protect
- ✅ **K8s has built-in rate limiting** - 429 responses handled via requeue
- ✅ **Controller-runtime handles backpressure** - work queue manages concurrency
- ✅ **Data Storage API uses fire-and-forget** (ADR-038) - audit writes don't block

**Pattern Comparison**:
| Service | External Dependencies | Circuit Breaker Needed? |
|---------|----------------------|------------------------|
| Notification | Slack API, Email SMTP | ✅ Yes - external APIs |
| Signal Processing | K8s API, Data Storage API | ❌ No - internal services |
| Gateway | External webhooks | ⚠️ Optional - depends on webhook targets |

**Reference**: See ADR-038 (fire-and-forget audit) for async pattern details

---

#### 📝 **Error Wrapping Pattern**

### Standard Error Wrapping

```go
package enricher

import (
    "context"
    "fmt"
)

// EnrichSignal wraps errors with context for debugging.
// DD-005 v2.0: Uses logr.Logger with key-value pairs
func (e *K8sEnricher) EnrichSignal(ctx context.Context, signal *Signal) (*KubernetesContext, error) {
    ns, err := e.getNamespace(ctx, signal.Namespace)
    if err != nil {
        return nil, fmt.Errorf("failed to enrich namespace %s: %w", signal.Namespace, err)
    }

    deploy, err := e.getDeployment(ctx, signal.Namespace, signal.DeploymentName)
    if err != nil {
        // Graceful degradation: continue without deployment context
        // DD-005 v2.0: logr syntax (key-value pairs, no zap helpers)
        e.logger.Info("deployment not found, continuing with partial context",
            "namespace", signal.Namespace,
            "deployment", signal.DeploymentName,
            "error", err)
        deploy = &DeploymentContext{} // Empty context
    }

    return &KubernetesContext{Namespace: ns, Deployment: deploy}, nil
}
```

---

#### 📊 **Logging Best Practices**

### Structured Logging for Signal Processing

```go
package reconciler

import (
    "github.com/go-logr/logr"
    ctrl "sigs.k8s.io/controller-runtime"
)

// Reconcile logs errors with full context.
func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := r.Log.WithValues(
        "signalProcessing", req.NamespacedName,
        "reconcileID", uuid.New().String(),
    )

    // Transient error
    if isTransient(err) {
        logger.Error(err, "transient error during enrichment, will retry",
            "attemptCount", sp.Status.AttemptCount,
            "nextRetry", CalculateBackoff(sp.Status.AttemptCount),
        )
        return HandleTransientError(sp.Status.AttemptCount), nil
    }

    // Permanent error
    if isPermanent(err) {
        logger.Error(err, "permanent error, marking as failed",
            "phase", "Failed",
            "reason", err.Error(),
        )
        sp.Status.Phase = "Failed"
        sp.Status.Message = err.Error()
        return HandlePermanentError(), r.Status().Update(ctx, sp)
    }

    return ctrl.Result{}, nil
}
```
```

---

### **Days 7-9: Label Detection** ⭐ NEW (DD-WORKFLOW-001 v1.9)

**Reference**: [DD-WORKFLOW-001 v1.9](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md)

#### **Day 7: Owner Chain**

**Purpose**: Build K8s ownership chain for DetectedLabels validation by HolmesGPT-API

**File**: `pkg/signalprocessing/ownerchain/builder.go`

**Business Requirement**: BR-SP-100 (OwnerChain Traversal)

**Authoritative Reference**: DD-WORKFLOW-001 v1.8

> ⚠️ **CRITICAL SCHEMA REQUIREMENTS (DD-WORKFLOW-001 v1.8)**:
> - OwnerChainEntry MUST have: `Namespace`, `Kind`, `Name` ONLY
> - Do NOT include `APIVersion` or `UID` - not used by HolmesGPT-API validation
> - Chain contains **OWNERS ONLY** - source resource is NOT included
> - Max depth: **5** levels (per BR-SP-100)

```go
package ownerchain

import (
    "context"

    "github.com/go-logr/logr"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
    "sigs.k8s.io/controller-runtime/pkg/client"

    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// MaxOwnerChainDepth per BR-SP-100: Stop at max depth 5
const MaxOwnerChainDepth = 5

// Builder constructs the K8s ownership chain
// Used by HolmesGPT-API to validate DetectedLabels applicability
// DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY (no APIVersion/UID)
// BR-SP-100: Max depth 5, owners only (source not included)
type Builder struct {
    client client.Client
    logger logr.Logger
}

func NewBuilder(c client.Client, logger logr.Logger) *Builder {
    return &Builder{
        client: c,
        logger: logger.WithName("ownerchain"),
    }
}

// Build traverses K8s ownerReferences to construct ownership chain
// Algorithm: Follow first `controller: true` ownerReference at each level
// Example: Pod → ReplicaSet → Deployment (Pod is NOT in chain, only owners)
// DD-WORKFLOW-001 v1.8: Chain contains OWNERS ONLY
// BR-SP-100: Max depth 5
func (b *Builder) Build(ctx context.Context, namespace, kind, name string) ([]signalprocessingv1alpha1.OwnerChainEntry, error) {
    var chain []signalprocessingv1alpha1.OwnerChainEntry

    currentNamespace := namespace
    currentKind := kind
    currentName := name

    // NOTE: Source resource is NOT added to chain (DD-WORKFLOW-001 v1.8)
    // Chain contains owners only

    // Traverse ownerReferences (max 5 levels per BR-SP-100)
    for i := 0; i < MaxOwnerChainDepth; i++ {
        ownerRef, err := b.getControllerOwner(ctx, currentNamespace, currentKind, currentName)
        if err != nil {
            // Log error but return partial chain (graceful degradation)
            b.logger.V(1).Info("Error fetching owner, returning partial chain",
                "error", err, "currentKind", currentKind, "currentName", currentName)
            break
        }
        if ownerRef == nil {
            break // No more owners - chain complete
        }

        // Cluster-scoped resources have empty namespace
        ownerNamespace := currentNamespace
        if isClusterScoped(ownerRef.Kind) {
            ownerNamespace = ""
        }

        // DD-WORKFLOW-001 v1.8: Namespace, Kind, Name ONLY
        chain = append(chain, signalprocessingv1alpha1.OwnerChainEntry{
            Namespace: ownerNamespace,
            Kind:      ownerRef.Kind,
            Name:      ownerRef.Name,
        })

        currentNamespace = ownerNamespace
        currentKind = ownerRef.Kind
        currentName = ownerRef.Name
    }

    b.logger.Info("Owner chain built",
        "length", len(chain),
        "source", kind+"/"+name)

    return chain, nil
}

// getControllerOwner fetches a resource and returns its controller owner reference.
// Returns nil, nil if no controller owner found.
// Returns nil, error for K8s API errors (RBAC, timeout, not found).
func (b *Builder) getControllerOwner(ctx context.Context, namespace, kind, name string) (*metav1.OwnerReference, error) {
    // Use unstructured for dynamic resource fetching
    gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
    if group, ok := kindToGroup[kind]; ok {
        gvk.Group = group
        gvk.Version = "v1"
    }

    obj := &unstructured.Unstructured{}
    obj.SetGroupVersionKind(gvk)

    key := client.ObjectKey{Namespace: namespace, Name: name}
    if err := b.client.Get(ctx, key, obj); err != nil {
        return nil, err // Caller handles error
    }

    // Find controller owner (controller: true)
    for _, ref := range obj.GetOwnerReferences() {
        if ref.Controller != nil && *ref.Controller {
            return &ref, nil
        }
    }

    return nil, nil // No controller owner
}

// kindToGroup maps K8s kinds to their API groups for unstructured fetching
var kindToGroup = map[string]string{
    "Pod":         "",
    "ReplicaSet":  "apps",
    "Deployment":  "apps",
    "StatefulSet": "apps",
    "DaemonSet":   "apps",
    "Job":         "batch",
    "CronJob":     "batch",
    "Node":        "",
    "Service":     "",
}

func isClusterScoped(kind string) bool {
    clusterScoped := map[string]bool{
        "Node": true, "PersistentVolume": true, "Namespace": true,
        "ClusterRole": true, "ClusterRoleBinding": true,
    }
    return clusterScoped[kind]
}
```

**Test Scenarios (Day 7)** - 12 tests per TESTING_GUIDELINES.md:

| ID | Category | Input | Expected Outcome |
|----|----------|-------|------------------|
| **OC-HP-01** | Happy Path | Pod owned by ReplicaSet owned by Deployment | Chain: [RS, Deployment] (2 entries) |
| **OC-HP-02** | Happy Path | Pod owned by StatefulSet | Chain: [StatefulSet] (1 entry) |
| **OC-HP-03** | Happy Path | Pod owned by DaemonSet | Chain: [DaemonSet] (1 entry) |
| **OC-HP-04** | Happy Path | Pod owned by Job owned by CronJob | Chain: [Job, CronJob] (2 entries) |
| **OC-EC-01** | Edge Case | Orphan Pod (no owner) | Empty chain [] |
| **OC-EC-02** | Edge Case | Node (cluster-scoped) | Single entry with empty namespace |
| **OC-EC-03** | Edge Case | Max depth reached (5 levels) | Truncated chain (5 entries max) |
| **OC-EC-04** | Edge Case | ReplicaSet without Deployment | Chain: [RS] (1 entry) |
| **OC-ER-01** | Error | K8s API timeout | Partial chain + logged error |
| **OC-ER-02** | Error | RBAC forbidden (403) | Partial chain + logged error |
| **OC-ER-03** | Error | Resource not found | Graceful termination, partial chain |
| **OC-ER-04** | Error | Context cancelled | Return current chain |

**Test File**: `test/unit/signalprocessing/ownerchain_builder_test.go`

---

#### **Day 8: DetectedLabels Auto-Detection**

**Purpose**: Auto-detect 8 cluster characteristics from K8s resources

**Business Requirements**: BR-SP-101 (DetectedLabels Auto-Detection), BR-SP-103 (FailedDetections Tracking)

**Authoritative Reference**: DD-WORKFLOW-001 v2.3

**File**: `pkg/signalprocessing/detection/labels.go`

**Test File**: `test/unit/signalprocessing/label_detector_test.go`

> ⚠️ **CRITICAL DETECTION FAILURE HANDLING (DD-WORKFLOW-001 v2.2)**:
> - Resource doesn't exist (no PDB) → `false` value, NOT in FailedDetections
> - Query failed (RBAC denied, timeout) → `false` value, field name IN FailedDetections
> - Use plain `bool` fields (NOT `*bool`) + `FailedDetections []string` array

```go
package detection

import (
    "context"

    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"

    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// LabelDetector auto-detects 8 cluster characteristics from K8s resources.
// BR-SP-101: DetectedLabels Auto-Detection
// BR-SP-103: FailedDetections Tracking
// DD-WORKFLOW-001 v2.2: Plain bool fields + FailedDetections array
type LabelDetector struct {
    client client.Client
    logger logr.Logger
}

// NewLabelDetector creates a new LabelDetector.
// Per BR-SP-101: Auto-detect cluster characteristics without customer configuration.
func NewLabelDetector(c client.Client, logger logr.Logger) *LabelDetector {
    return &LabelDetector{
        client: c,
        logger: logger.WithName("detection"),
    }
}

// DetectLabels detects 8 label types from K8s context.
// Per DD-WORKFLOW-001 v2.3: Tracks QUERY FAILURES in FailedDetections field.
//
// 8 Detection Types:
//   1. gitOpsManaged/gitOpsTool (ArgoCD/Flux annotations)
//   2. pdbProtected (PodDisruptionBudget - K8s API query)
//   3. hpaEnabled (HorizontalPodAutoscaler - K8s API query)
//   4. stateful (StatefulSet in owner chain - NO API call)
//   5. helmManaged (Helm labels - existing data)
//   6. networkIsolated (NetworkPolicy - K8s API query)
//   7. serviceMesh (Istio/Linkerd pod annotations - existing data)
//
// IMPORTANT DISTINCTION (BR-SP-103):
// - Resource doesn't exist (PDB not found) → false (normal, NOT an error)
// - Can't query resource (RBAC denied, timeout) → false + FailedDetections + warn log
//
// Parameters:
// - ctx: Context for K8s API calls
// - k8sCtx: Kubernetes context with namespace, pod, deployment details
// - ownerChain: Owner chain from Day 7 OwnerChain Builder (for stateful detection)
func (d *LabelDetector) DetectLabels(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, ownerChain []sharedtypes.OwnerChainEntry) *sharedtypes.DetectedLabels {
    if k8sCtx == nil {
        return nil
    }

    labels := &sharedtypes.DetectedLabels{}
    var failedDetections []string  // Track QUERY failures only (DD-WORKFLOW-001 v2.3)

    // 1. GitOps detection (ArgoCD/Flux) - NO API call needed
    d.detectGitOps(k8sCtx, labels)

    // 2. PDB protection detection - K8s API query
    if err := d.detectPDB(ctx, k8sCtx, labels); err != nil {
        d.logger.V(1).Info("Could not query PodDisruptionBudgets", "error", err)
        failedDetections = append(failedDetections, "pdbProtected")
    }

    // 3. HPA detection - K8s API query
    if err := d.detectHPA(ctx, k8sCtx, labels); err != nil {
        d.logger.V(1).Info("Could not query HorizontalPodAutoscalers", "error", err)
        failedDetections = append(failedDetections, "hpaEnabled")
    }

    // 4. StatefulSet detection - uses owner chain (NO API call)
    labels.Stateful = d.isStateful(ownerChain)

    // 5. Helm managed detection - NO API call needed
    d.detectHelm(k8sCtx, labels)

    // 6. Network isolation detection - K8s API query
    if err := d.detectNetworkPolicy(ctx, k8sCtx, labels); err != nil {
        d.logger.V(1).Info("Could not query NetworkPolicies", "error", err)
        failedDetections = append(failedDetections, "networkIsolated")
    }

    // 7. Service Mesh detection (Istio/Linkerd) - NO API call needed
    d.detectServiceMesh(k8sCtx, labels)

    // Set FailedDetections only if we had QUERY failures (DD-WORKFLOW-001 v2.3)
    if len(failedDetections) > 0 {
        labels.FailedDetections = failedDetections
        d.logger.Info("Some label detections failed (RBAC or timeout)",
            "failedDetections", failedDetections)
    }

    return labels
}

// Helper functions:
// - detectGitOps, detectHelm, detectServiceMesh: NO API call, no error return
// - detectPDB, detectHPA, detectNetworkPolicy: K8s API call, returns error on query failure

func (d *LabelDetector) detectGitOps(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
    // Check for ArgoCD: argocd.argoproj.io/instance annotation (pod, deployment, namespace)
    // Check for Flux: fluxcd.io/sync-gc-mark label (deployment, namespace)
    // NO API call - uses existing data from KubernetesContext
    // Sets result.GitOpsManaged and result.GitOpsTool
}

func (d *LabelDetector) detectPDB(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
    // List PodDisruptionBudgets, check if selector matches pod labels
    // Sets result.PDBProtected = true/false
    // Returns nil on success (even if no PDB found), error on query failure
    return nil
}

func (d *LabelDetector) detectHPA(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
    // List HorizontalPodAutoscalers, check if scaleTargetRef matches deployment
    // Sets result.HPAEnabled = true/false
    // Returns nil on success (even if no HPA found), error on query failure
    return nil
}

func (d *LabelDetector) isStateful(ownerChain []sharedtypes.OwnerChainEntry) bool {
    // Per DD-WORKFLOW-001 v2.3: Check if owner chain includes StatefulSet
    // NO K8s API call needed - uses owner chain from Day 7
    for _, owner := range ownerChain {
        if owner.Kind == "StatefulSet" {
            return true
        }
    }
    return false
}

func (d *LabelDetector) detectHelm(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
    // Check for app.kubernetes.io/managed-by: Helm or helm.sh/chart label
    // NO API call - uses existing data from KubernetesContext
    // Sets result.HelmManaged = true/false
}

func (d *LabelDetector) detectNetworkPolicy(ctx context.Context, k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) error {
    // List NetworkPolicies in namespace
    // Sets result.NetworkIsolated = true if any exist, false otherwise
    // Returns nil on success (even if no NetworkPolicy found), error on query failure
    return nil
}

func (d *LabelDetector) detectServiceMesh(k8sCtx *sharedtypes.KubernetesContext, result *sharedtypes.DetectedLabels) {
    // Per DD-WORKFLOW-001 v2.3: Check pod annotations for service mesh sidecars
    // NO K8s API call needed - uses existing pod annotation data
    //
    // Istio: sidecar.istio.io/status (present after sidecar injection)
    // Linkerd: linkerd.io/proxy-version (present after proxy injection)
    //
    // Sets result.ServiceMesh = "istio" | "linkerd" | ""
    if k8sCtx.PodDetails == nil || k8sCtx.PodDetails.Annotations == nil {
        result.ServiceMesh = ""
        return
    }
    annotations := k8sCtx.PodDetails.Annotations
    if _, ok := annotations["sidecar.istio.io/status"]; ok {
        result.ServiceMesh = "istio"
        return
    }
    if _, ok := annotations["linkerd.io/proxy-version"]; ok {
        result.ServiceMesh = "linkerd"
        return
    }
    result.ServiceMesh = ""
}
```

**Test Scenarios (Day 8)** - 16 tests per TESTING_GUIDELINES.md:

| ID | Category | Input | Expected Outcome | BR |
|----|----------|-------|------------------|-----|
| **DL-HP-01** | Happy Path | ArgoCD-annotated Deployment | `gitOpsManaged: true, gitOpsTool: "argocd"` | BR-SP-101 |
| **DL-HP-02** | Happy Path | Flux-labeled Deployment | `gitOpsManaged: true, gitOpsTool: "flux"` | BR-SP-101 |
| **DL-HP-03** | Happy Path | Deployment with PDB | `pdbProtected: true` | BR-SP-101 |
| **DL-HP-04** | Happy Path | Deployment with HPA | `hpaEnabled: true` | BR-SP-101 |
| **DL-HP-05** | Happy Path | Owner chain contains StatefulSet | `stateful: true` (uses ownerChain param) | BR-SP-101 |
| **DL-HP-06** | Happy Path | Helm-managed Deployment | `helmManaged: true` | BR-SP-101 |
| **DL-HP-07** | Happy Path | Namespace with NetworkPolicy | `networkIsolated: true` | BR-SP-101 |
| **DL-HP-08** | Happy Path | Istio sidecar-injected Pod (`sidecar.istio.io/status`) | `serviceMesh: "istio"` | BR-SP-101 |
| **DL-HP-09** | Happy Path | Linkerd proxy-injected Pod (`linkerd.io/proxy-version`) | `serviceMesh: "linkerd"` | BR-SP-101 |
| **DL-EC-01** | Edge Case | Clean deployment (no detections) | All fields `false`, no FailedDetections | BR-SP-101 |
| **DL-EC-02** | Edge Case | Nil KubernetesContext | Return `nil` | BR-SP-101 |
| **DL-EC-03** | Edge Case | Multiple detections true | GitOps + PDB + HPA all true simultaneously | BR-SP-101 |
| **DL-ER-01** | Error | RBAC denied (PDB query) | `pdbProtected: false`, `FailedDetections: ["pdbProtected"]` | BR-SP-103 |
| **DL-ER-02** | Error | API timeout (HPA query) | `hpaEnabled: false`, `FailedDetections: ["hpaEnabled"]` | BR-SP-103 |
| **DL-ER-03** | Error | Multiple query failures | `FailedDetections: ["pdbProtected", "hpaEnabled", "networkIsolated"]` | BR-SP-103 |
| **DL-ER-04** | Error | Context cancellation | Return partial results with detected labels so far | BR-SP-103 |

---

#### **Day 9: CustomLabels Rego Extraction**

**Purpose**: Extract user-defined labels via Rego policies with security wrapper

**Business Requirements**: BR-SP-102 (CustomLabels Rego Extraction), BR-SP-104 (Mandatory Label Protection)

**Authoritative Reference**: DD-WORKFLOW-001 v1.9

**Files**:
- `pkg/signalprocessing/rego/engine.go` - Main Rego engine
- `pkg/signalprocessing/rego/security.go` - Security wrapper (BR-SP-104)

**Test Files** (per BR mapping):
- `test/unit/signalprocessing/rego_engine_test.go` - BR-SP-102 tests
- `test/unit/signalprocessing/rego_security_wrapper_test.go` - BR-SP-104 tests

> ⚠️ **SANDBOX REQUIREMENTS (DD-WORKFLOW-001 v1.9)**:
> - Network access: ❌ Disabled
> - Filesystem access: ❌ Disabled
> - Evaluation timeout: **5 seconds**
> - Memory limit: **128 MB**
> - External data: ❌ Disabled (V1.0)

```go
package rego

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/go-logr/logr"
    "github.com/open-policy-agent/opa/rego"

    "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Sandbox configuration per DD-WORKFLOW-001 v1.9
const (
    evaluationTimeout = 5 * time.Second  // Max Rego evaluation time
    maxKeys           = 10               // Max keys (subdomains)
    maxValuesPerKey   = 5                // Max values per key
    maxKeyLength      = 63               // K8s label key compatibility
    maxValueLength    = 100              // Prompt efficiency
)

// Engine evaluates customer Rego policies for CustomLabels
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-104: Mandatory Label Protection (via security wrapper)
// DD-WORKFLOW-001 v1.9: Sandboxed OPA Runtime
type Engine struct {
    logger       logr.Logger
    policyModule string    // Compiled policy with security wrapper
    policyPath   string    // Path to labels.rego (for hot-reload)
    mu           sync.RWMutex
}

// NewEngine creates a new CustomLabels Rego engine.
// Per BR-SP-102: Extract customer labels via sandboxed OPA policies.
func NewEngine(logger logr.Logger, policyPath string) *Engine {
    return &Engine{
        logger:     logger.WithName("rego"),
        policyPath: policyPath,
    }
}

// SetupHotReload configures hot-reload using pkg/shared/hotreload/FileWatcher.
// Per Day 5 pattern: FileWatcher watches for ConfigMap changes.
func (e *Engine) SetupHotReload(ctx context.Context) error {
    watcher, err := hotreload.NewFileWatcher(e.policyPath, e.logger)
    if err != nil {
        return fmt.Errorf("failed to create file watcher: %w", err)
    }

    watcher.OnChange(func(data []byte) {
        e.mu.Lock()
        defer e.mu.Unlock()
        e.policyModule = e.wrapWithSecurityPolicy(string(data))
        e.logger.Info("Rego policy hot-reloaded", "policySize", len(data))
    })

    go watcher.Start(ctx)
    return nil
}

// LoadPolicy loads customer policy from file and wraps with security policy.
// Called at startup, then hot-reloaded via SetupHotReload.
func (e *Engine) LoadPolicy(policyContent string) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    e.policyModule = e.wrapWithSecurityPolicy(policyContent)
    e.logger.Info("Rego policy loaded", "policySize", len(policyContent))
    return nil
}

// RegoInput wraps shared types for Rego policy evaluation.
// Uses sharedtypes.KubernetesContext (authoritative source).
type RegoInput struct {
    Kubernetes     *sharedtypes.KubernetesContext `json:"kubernetes"`
    Signal         SignalContext                   `json:"signal"`
    DetectedLabels *sharedtypes.DetectedLabels     `json:"detected_labels,omitempty"`
}

// SignalContext contains signal-specific data for Rego policies.
type SignalContext struct {
    Type     string `json:"type"`
    Severity string `json:"severity"`
    Source   string `json:"source"`
}

// EvaluatePolicy evaluates the policy and returns CustomLabels.
// Output format: map[string][]string (subdomain → list of values)
// DD-WORKFLOW-001 v1.9: 5s timeout, sandboxed execution
func (e *Engine) EvaluatePolicy(ctx context.Context, input *RegoInput) (map[string][]string, error) {
    e.mu.RLock()
    policyModule := e.policyModule
    e.mu.RUnlock()

    if policyModule == "" {
        e.logger.V(1).Info("No policy loaded, returning empty labels")
        return make(map[string][]string), nil
    }

    // Sandboxed execution: 5s timeout per DD-WORKFLOW-001 v1.9
    evalCtx, cancel := context.WithTimeout(ctx, evaluationTimeout)
    defer cancel()

    r := rego.New(
        rego.Query("data.signalprocessing.security.labels"),
        rego.Module("policy.rego", policyModule),
        rego.Input(input),
        rego.StrictBuiltinErrors(true),     // Strict mode for safety
        rego.EnablePrintStatements(false),  // Disable debugging in prod
    )

    rs, err := r.Eval(evalCtx)
    if err != nil {
        return nil, fmt.Errorf("rego evaluation failed: %w", err)
    }

    if len(rs) == 0 || len(rs[0].Expressions) == 0 {
        return make(map[string][]string), nil
    }

    // Convert result to map[string][]string
    result, err := e.convertResult(rs[0].Expressions[0].Value)
    if err != nil {
        return nil, fmt.Errorf("invalid rego output type: %w", err)
    }

    // Validate and sanitize (DD-WORKFLOW-001 v1.9)
    result = e.validateAndSanitize(result)

    e.logger.Info("CustomLabels evaluated", "labelCount", len(result))
    return result, nil
}

// convertResult converts OPA output to map[string][]string.
func (e *Engine) convertResult(value interface{}) (map[string][]string, error) {
    result := make(map[string][]string)

    valueMap, ok := value.(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("expected map[string]interface{}, got %T", value)
    }

    for key, val := range valueMap {
        switch v := val.(type) {
        case []interface{}:
            var strValues []string
            for _, item := range v {
                if strVal, ok := item.(string); ok {
                    strValues = append(strValues, strVal)
                }
            }
            result[key] = strValues
        case string:
            result[key] = []string{v}
        }
    }

    return result, nil
}

// validateAndSanitize enforces validation limits per DD-WORKFLOW-001 v1.9.
// Strips reserved prefixes (BR-SP-104) and enforces size limits.
func (e *Engine) validateAndSanitize(labels map[string][]string) map[string][]string {
    result := make(map[string][]string)

    // Reserved prefixes - strip these for security (BR-SP-104)
    reservedPrefixes := []string{"kubernaut.ai/", "system/"}

    keyCount := 0
    for key, values := range labels {
        // Check key count limit
        if keyCount >= maxKeys {
            e.logger.Info("CustomLabels key limit reached, truncating",
                "maxKeys", maxKeys, "totalKeys", len(labels))
            break
        }

        // Skip reserved prefixes (BR-SP-104: Mandatory Label Protection)
        if hasReservedPrefix(key, reservedPrefixes) {
            e.logger.Info("CustomLabels reserved prefix stripped", "key", key)
            continue
        }

        // Truncate key if too long
        if len(key) > maxKeyLength {
            e.logger.Info("CustomLabels key truncated",
                "key", key, "maxLength", maxKeyLength)
            key = key[:maxKeyLength]
        }

        // Validate and truncate values
        var validValues []string
        for i, value := range values {
            if i >= maxValuesPerKey {
                e.logger.Info("CustomLabels values limit reached",
                    "key", key, "maxValues", maxValuesPerKey)
                break
            }
            if len(value) > maxValueLength {
                e.logger.Info("CustomLabels value truncated",
                    "key", key, "maxLength", maxValueLength)
                value = value[:maxValueLength]
            }
            validValues = append(validValues, value)
        }

        result[key] = validValues
        keyCount++
    }

    return result
}

func hasReservedPrefix(key string, prefixes []string) bool {
    for _, prefix := range prefixes {
        if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
            return true
        }
    }
    return false
}

// wrapWithSecurityPolicy wraps customer policy with security wrapper.
// BR-SP-104: Security wrapper blocks override of 5 mandatory labels.
// Uses kubernaut.ai/ domain (corrected from kubernaut.io/).
func (e *Engine) wrapWithSecurityPolicy(customerPolicy string) string {
    securityWrapper := `
package signalprocessing.security

import rego.v1
import data.signalprocessing.labels as customer_labels

# 5 mandatory system labels (DD-WORKFLOW-001 v1.9) - cannot be overridden
# BR-SP-104: Mandatory Label Protection
system_prefixes := {"kubernaut.ai/", "system/"}

# Final output: customer labels minus system labels
labels[key] := value if {
    some key, value in customer_labels.labels
    not has_reserved_prefix(key)
}

has_reserved_prefix(key) if {
    some prefix in system_prefixes
    startswith(key, prefix)
}
`
    return securityWrapper + "\n\n" + customerPolicy
}
```

**Test Scenarios (Day 9)** - 16 tests per TESTING_GUIDELINES.md:

**BR-SP-102 Tests** (`test/unit/signalprocessing/rego_engine_test.go`):

| ID | Category | Input | Expected Outcome | BR |
|----|----------|-------|------------------|-----|
| **CL-HP-01** | Happy Path | Rego extracts `team` from ns label | `{"team": ["payments"]}` | BR-SP-102 |
| **CL-HP-02** | Happy Path | Rego sets `risk_tolerance` | `{"risk_tolerance": ["low"]}` | BR-SP-102 |
| **CL-HP-03** | Happy Path | Multi-subdomain extraction | Multiple keys in result | BR-SP-102 |
| **CL-HP-04** | Happy Path | Multi-value per subdomain | `{"constraint": ["cost", "stateful"]}` | BR-SP-102 |
| **CL-HP-05** | Happy Path | Hot-reload triggers callback | Policy refreshed | BR-SP-102 |
| **CL-EC-01** | Edge Case | Empty policy | Empty map returned | BR-SP-102 |
| **CL-EC-02** | Edge Case | Policy returns nil | Empty map returned | BR-SP-102 |
| **CL-EC-03** | Edge Case | Max keys (10) exceeded | Truncated to 10 keys | BR-SP-102 |
| **CL-EC-04** | Edge Case | Max values (5) exceeded | Truncated to 5 values | BR-SP-102 |
| **CL-EC-05** | Edge Case | Key length > 63 chars | Key truncated | BR-SP-102 |
| **CL-EC-06** | Edge Case | Value length > 100 chars | Value truncated | BR-SP-102 |
| **CL-ER-01** | Error | Rego syntax error | Error returned | BR-SP-102 |
| **CL-ER-02** | Error | Rego timeout (>5s) | Context deadline error | BR-SP-102 |
| **CL-ER-03** | Error | Invalid output type | Type validation error | BR-SP-102 |

**BR-SP-104 Tests** (`test/unit/signalprocessing/rego_security_wrapper_test.go`):

| ID | Category | Input | Expected Outcome | BR |
|----|----------|-------|------------------|-----|
| **CL-SEC-01** | Security | Policy sets `kubernaut.ai/environment` | Label stripped | BR-SP-104 |
| **CL-SEC-02** | Security | Policy sets `system/internal` | Label stripped | BR-SP-104 |

**ConfigMap Example** (uses correct `kubernaut.ai/` domain):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: signal-processing-policies
  namespace: kubernaut-system
data:
  labels.rego: |
    package signalprocessing.labels

    import rego.v1

    # Extract team from namespace label
    labels["team"] := input.kubernetes.namespaceLabels["kubernaut.ai/team"] if {
      input.kubernetes.namespaceLabels["kubernaut.ai/team"]
    }

    # Derive risk_tolerance from environment classification
    labels["risk_tolerance"] := "low" if {
      input.signal.severity == "critical"
    }
    labels["risk_tolerance"] := "high" if {
      input.signal.severity != "critical"
    }

    # Multi-value constraint example
    labels["constraint"] := constraints if {
      constraints := array.concat(
        ["cost-aware"],
        _stateful_constraint
      )
    }

    _stateful_constraint := ["stateful-safe"] if {
      input.detected_labels.stateful == true
    } else := []
```

---

### **Days 10-11: Integration**

#### **Day 10: Reconciler** (Updated with Label Detection)

**File**: `internal/controller/signalprocessing/reconciler.go`

**Note**: Reconciler now includes OwnerChain, DetectedLabels, and CustomLabels integration.

```go
// Updated reconciler struct with label detection components
type SignalProcessingReconciler struct {
    client.Client
    Scheme              *runtime.Scheme
    Logger              logr.Logger
    K8sEnricher         *enricher.K8sEnricher
    EnvironmentClassifier *classifier.EnvironmentClassifier
    PriorityEngine      *classifier.PriorityEngine
    BusinessClassifier  *classifier.BusinessClassifier
    AuditClient         *audit.Client
    Metrics             *metrics.Metrics
    // NEW: Label Detection components (DD-WORKFLOW-001 v1.9)
    OwnerChainBuilder   *ownerchain.Builder
    LabelDetector       *detection.LabelDetector
    RegoEngine          *rego.Engine
}

// handleEnrichingPhase now includes label detection
func (r *SignalProcessingReconciler) handleEnrichingPhase(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    // ... existing K8s enrichment ...

    // NEW: Build owner chain (DD-WORKFLOW-001 v1.9)
    ownerChain, err := r.OwnerChainBuilder.Build(ctx,
        sp.Status.EnrichmentResults.KubernetesContext.Namespace,
        "Pod",
        sp.Status.EnrichmentResults.KubernetesContext.PodDetails.Name)
    if err != nil {
        r.Logger.Error(err, "owner chain build failed (non-fatal)")
    }
    sp.Status.EnrichmentResults.OwnerChain = ownerChain

    // NEW: Detect labels (V1.0)
    detectedLabels := r.LabelDetector.DetectLabels(ctx, sp.Status.EnrichmentResults.KubernetesContext)
    sp.Status.EnrichmentResults.DetectedLabels = detectedLabels

    // NEW: Evaluate Rego for custom labels (V1.0)
    regoInput := r.buildRegoInput(ctx, sp, detectedLabels)
    customLabels, err := r.RegoEngine.EvaluatePolicy(ctx, regoInput)
    if err != nil {
        r.Logger.Error(err, "rego evaluation failed (non-fatal)")
        customLabels = make(map[string][]string)
    }
    sp.Status.EnrichmentResults.CustomLabels = customLabels

    // ... continue to classifying phase ...
}
```

---

**Original Reconciler Code** (for reference):

```go
// SignalProcessingReconciler reconciles SignalProcessing CRDs
// DD-005 v2.0: Uses logr.Logger (native from controller-runtime)
type SignalProcessingReconciler struct {
    client.Client
    Scheme              *runtime.Scheme
    Logger              logr.Logger // DD-005 v2.0: logr.Logger (native ctrl.Log)
    K8sEnricher         *enricher.K8sEnricher
    EnvironmentClassifier *classifier.EnvironmentClassifier
    PriorityEngine      *classifier.PriorityEngine
    BusinessClassifier  *classifier.BusinessClassifier
    AuditClient         *audit.Client
    Metrics             *metrics.Metrics
}

// RBAC: Own CRD - SignalProcessing (created by RemediationOrchestrator)
//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings/finalizers,verbs=update

// RBAC: K8s resources for enrichment (read-only)
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch

// RBAC: DetectedLabels auto-detection (DD-WORKFLOW-001 v2.1)
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch

// RBAC: ConfigMaps for Rego policy hot-reload
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := r.Logger.WithValues("signalprocessing", req.NamespacedName.String()) // DD-005: .WithValues() not .With(zap.String())

    // 1. Fetch SignalProcessing CRD
    sp := &signalprocessingv1alpha1.SignalProcessing{}
    if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Check terminal state
    if sp.Status.Phase == signalprocessingv1alpha1.PhaseComplete ||
       sp.Status.Phase == signalprocessingv1alpha1.PhaseFailed {
        return ctrl.Result{}, nil
    }

    // 3. Phase state machine
    switch sp.Status.Phase {
    case "", signalprocessingv1alpha1.PhasePending:
        return r.handlePendingPhase(ctx, sp)
    case signalprocessingv1alpha1.PhaseEnriching:
        return r.handleEnrichingPhase(ctx, sp)
    case signalprocessingv1alpha1.PhaseClassifying:
        return r.handleClassifyingPhase(ctx, sp)
    case signalprocessingv1alpha1.PhaseCategorizing:
        return r.handleCategorizingPhase(ctx, sp)
    default:
        log.Error(nil, "Unknown phase", "phase", string(sp.Status.Phase)) // DD-005: logr syntax
        return ctrl.Result{}, nil
    }
}

func (r *SignalProcessingReconciler) handleEnrichingPhase(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) (ctrl.Result, error) {
    // K8s context enrichment
    k8sCtx, err := r.K8sEnricher.Enrich(ctx, &sp.Spec.Signal)
    if err != nil {
        return r.handleError(ctx, sp, err)
    }

    // Update status
    sp.Status.KubernetesContext = k8sCtx
    sp.Status.Phase = signalprocessingv1alpha1.PhaseClassifying

    if err := r.Status().Update(ctx, sp); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{Requeue: true}, nil
}
```

#### **Day 8: Metrics and Audit**

**BR Coverage**: BR-SP-090 (Categorization Audit Trail)

**File**: `pkg/signalprocessing/audit/client.go`

**Pattern**: Fire-and-forget with buffered writes (ADR-038)

```go
package audit

import (
    "context"

    "github.com/go-logr/logr"

    "github.com/jordigilh/kubernaut/pkg/audit" // Shared audit library (ADR-038)
    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

// Client wraps the shared audit store for Signal Processing events.
// ADR-038: Uses fire-and-forget pattern with buffered writes (<1ms overhead).
// DD-005 v2.0: Accepts logr.Logger (unified interface for all Kubernaut services)
type Client struct {
    store  audit.AuditStore // Shared library from pkg/audit/
    logger logr.Logger      // DD-005 v2.0: logr.Logger (not *zap.Logger)
}

// NewClient creates an audit client using the shared buffered audit store.
// DD-005 v2.0: Accept logr.Logger from caller
func NewClient(store audit.AuditStore, logger logr.Logger) *Client {
    return &Client{
        store:  store,
        logger: logger,
    }
}

// WriteCategorizationAudit writes categorization decisions using fire-and-forget pattern.
// ADR-034: signalprocessing.categorization.completed event
// ADR-038: Non-blocking, returns immediately (<1ms), event buffered for async write
func (c *Client) WriteCategorizationAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
    event := audit.Event{
        Version:       "1.0",
        Service:       "signalprocessing",
        EventType:     "signalprocessing.categorization.completed",
        EventCategory: "categorization",
        EventAction:   "categorized",
        EventOutcome:  "success",
        CorrelationID: sp.Spec.RemediationRequestRef.Name,
        ResourceType:  "SignalProcessing",
        ResourceID:    sp.Name,
        EventData: map[string]interface{}{
            "environment": sp.Status.EnvironmentClassification,
            "priority":    sp.Status.PriorityAssignment,
            "business":    sp.Status.BusinessClassification,
        },
    }

    // Fire-and-forget: Returns immediately, event added to buffer
    // Background worker flushes buffer to Data Storage Service asynchronously
    // Business logic NEVER waits for audit writes (ADR-038)
    if err := c.store.StoreAudit(ctx, event); err != nil {
        // Log and continue - audit failures don't block business operations
        // DD-005 v2.0: logr syntax (key-value pairs, no zap helpers)
        c.logger.Info("audit event buffering failed",
            "error", err,
            "correlation_id", sp.Spec.RemediationRequestRef.Name,
        )
    }
    // Returns immediately - no wait for actual write
}

// WriteEnrichmentAudit writes enrichment completion events.
// ADR-034: signalprocessing.enrichment.completed event
func (c *Client) WriteEnrichmentAudit(ctx context.Context, sp *signalprocessingv1alpha1.SignalProcessing) {
    event := audit.Event{
        Version:       "1.0",
        Service:       "signalprocessing",
        EventType:     "signalprocessing.enrichment.completed",
        EventCategory: "enrichment",
        EventAction:   "enriched",
        EventOutcome:  "success",
        CorrelationID: sp.Spec.RemediationRequestRef.Name,
        ResourceType:  "SignalProcessing",
        ResourceID:    sp.Name,
        EventData: map[string]interface{}{
            "k8s_context": sp.Status.KubernetesContext != nil,
        },
    }

    // Fire-and-forget (ADR-038)
    if err := c.store.StoreAudit(ctx, event); err != nil {
        c.logger.Info("audit event buffering failed", "error", err) // DD-005: key-value pairs
    }
}
```

**Key ADR-038 Principles**:
- ✅ **Returns immediately** (<1ms) - no wait for HTTP response
- ✅ **Event buffered** - added to in-memory buffer
- ✅ **Background worker** - flushes buffer asynchronously
- ✅ **Business never waits** - reconciliation continues regardless of audit
- ✅ **Graceful degradation** - audit failures logged, not propagated

---

#### 📊 **Metrics Cardinality Audit** (Day 8 EOD) ⭐ v1.12 NEW

**⚠️ MANDATORY**: Audit all Prometheus metrics for high-cardinality label combinations per DD-005.

**Target**: Keep total unique metric combinations < 1,000 (ideal) or < 5,000 (acceptable)

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/METRICS_CARDINALITY_AUDIT.md`

##### **Signal Processing Metrics Inventory**

| Metric | Labels | Cardinality | Status |
|--------|--------|-------------|--------|
| `signalprocessing_reconciliation_total` | `phase`, `result` | **Low** (5×2=10) | ✅ SAFE |
| `signalprocessing_reconciliation_duration_seconds` | `phase` | **Low** (5) | ✅ SAFE |
| `signalprocessing_categorization_confidence` | (none) | **Low** (1) | ✅ SAFE |
| `signalprocessing_enrichment_duration_seconds` | (none) | **Low** (1) | ✅ SAFE |
| `signalprocessing_rego_evaluation_duration_seconds` | `policy` | **Low** (3) | ✅ SAFE |
| `signalprocessing_rego_hot_reload_total` | `status` | **Low** (2) | ✅ SAFE |

##### **Cardinality Analysis**

| Label | Possible Values | Bounded? |
|-------|-----------------|----------|
| `phase` | `pending`, `enriching`, `categorizing`, `completed`, `failed` | ✅ Fixed (5 values) |
| `result` | `success`, `error` | ✅ Fixed (2 values) |
| `policy` | `priority`, `environment`, `business` | ✅ Fixed (3 values) |
| `status` | `success`, `error` | ✅ Fixed (2 values) |

**Total Combinations**: 10 + 5 + 1 + 1 + 3 + 2 = **22 unique metrics** ✅ EXCELLENT

##### **Cardinality Protection Applied**

- ✅ **No unbounded labels** (no user IDs, signal fingerprints, or dynamic values)
- ✅ **Phase labels fixed** (enum-based, not string input)
- ✅ **Policy labels fixed** (known set of policy names)
- ✅ **No HTTP path metrics** (CRD controller, not HTTP service)

##### **Audit Status**: ✅ **SAFE** (22 combinations << 1,000 threshold)

---

### **Days 9-10: Testing**

#### **Defense-in-Depth Testing Strategy** (per `03-testing-strategy.mdc`)

**Standard Methodology**: Unit → Integration → E2E

**Reference**: Actual test counts from production services:
- Gateway: **275 unit** + **143 integration** = 418 tests
- Data Storage: **392 unit** + **160 integration** = 552 tests

**Signal Processing Expected** (CRD Controller, medium complexity):
- Unit Tests: **250-300** tests (current: 184 from Days 1-9)
- Integration Tests: **50-80** tests (current: 0 - Day 10 focus)
- E2E Tests: **5-10** tests (current: 0 - Day 11 focus)

| Test Type | Current | Target | Purpose | Location |
|-----------|---------|--------|---------|----------|
| **Unit** | 184 ✅ | 250-300 | Component logic, edge cases, error handling | `test/unit/signalprocessing/` |
| **Integration** | 0 ❌ | 50-80 | CRD reconciliation, real K8s API (envtest) | `test/integration/signalprocessing/` |
| **E2E** | 0 ❌ | 5-10 | Full workflow validation | `test/e2e/signalprocessing/` |

**Note**: Unit tests created during Days 1-9 cover component logic (enricher, classifier, priority, business, ownerchain, detection, rego). Integration and E2E tests are the Day 10-11 focus.

---

#### ⚡ **Parallel Test Execution** (MANDATORY)

**Standard**: **4 concurrent processes** for all test tiers.

**Configuration**:
```bash
# Unit tests - parallel execution
go test -v -p 4 ./test/unit/signalprocessing/...
ginkgo -p -procs=4 -v ./test/unit/signalprocessing/...

# Integration tests - parallel with shared envtest
go test -v -p 4 ./test/integration/signalprocessing/...
ginkgo -p -procs=4 -v ./test/integration/signalprocessing/...

# E2E tests - parallel with isolated namespaces
go test -v -p 4 ./test/e2e/signalprocessing/...
ginkgo -p -procs=4 -v ./test/e2e/signalprocessing/...

# All tests at once
ginkgo -p -procs=4 -v ./test/unit/signalprocessing/... ./test/integration/signalprocessing/... ./test/e2e/signalprocessing/...
```

**Parallel Test Requirements**:
| Tier | Isolation Strategy | Shared Resources | Port Allocation |
|------|-------------------|------------------|-----------------|
| **Unit** | No shared state between tests | Mock/fake clients | N/A |
| **Integration** | Unique namespace per test | Shared envtest API server | Per DD-TEST-001 |
| **E2E** | Unique namespace per test | Shared cluster | Per DD-TEST-001 |

---

#### ⚠️ **K8s Client Mandate** (per ADR-004 - MANDATORY)

**Reference**: [ADR-004: Fake Kubernetes Client](../../../architecture/decisions/ADR-004-fake-kubernetes-client.md)

| Test Tier | MANDATORY Interface | Package |
|-----------|---------------------|---------|
| **Unit Tests** | **Fake K8s Client** | `sigs.k8s.io/controller-runtime/pkg/client/fake` |
| **Integration** | Real K8s API (envtest) | `sigs.k8s.io/controller-runtime/pkg/client` |
| **E2E** | Real K8s API (KIND) | `sigs.k8s.io/controller-runtime/pkg/client` |

**❌ FORBIDDEN**: Custom `MockK8sClient` implementations
**✅ APPROVED**: `fake.NewClientBuilder()` for all unit tests

**Unit Test Setup Pattern**:
```go
package signalprocessing

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
)

var _ = Describe("K8s Enricher", func() {
    var (
        ctx        context.Context
        fakeClient client.Client  // ADR-004: Use fake client for unit tests
    )

    BeforeEach(func() {
        ctx = context.Background()

        // ADR-004: Create fake client with scheme
        scheme := runtime.NewScheme()
        Expect(kubernautv1alpha1.AddToScheme(scheme)).To(Succeed())
        Expect(corev1.AddToScheme(scheme)).To(Succeed())

        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            Build()
    })

    It("should enrich pod signal", func() {
        // Use fakeClient for K8s operations
        // NO custom MockK8sClient allowed
    })
})
```

---

#### 🔄 **Retry Strategy Tests** (Unit Tests)

**File**: `test/unit/signalprocessing/retry_test.go`

```go
package signalprocessing

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime/schema"

    "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
)

var _ = Describe("Retry Strategy", func() {
    // TABLE-DRIVEN: Backoff calculation with jitter
    DescribeTable("should calculate correct backoff",
        func(attempt int, minBackoff, maxBackoff time.Duration) {
            backoff := signalprocessing.CalculateBackoff(attempt)
            Expect(backoff).To(BeNumerically(">=", minBackoff), "backoff too low for attempt %d", attempt)
            Expect(backoff).To(BeNumerically("<=", maxBackoff), "backoff too high for attempt %d", attempt)
        },
        Entry("attempt 0 (first fail)", 0, 27*time.Second, 33*time.Second),
        Entry("attempt 1", 1, 54*time.Second, 66*time.Second),
        Entry("attempt 2", 2, 108*time.Second, 132*time.Second),
        Entry("attempt 3", 3, 216*time.Second, 264*time.Second),
        Entry("attempt 4 (cap)", 4, 432*time.Second, 528*time.Second),
        Entry("attempt 10 (still capped)", 10, 432*time.Second, 528*time.Second),
    )

    // TABLE-DRIVEN: Error classification
    DescribeTable("should classify errors correctly",
        func(err error, expectedRetryable bool, description string) {
            result := signalprocessing.IsRetryableError(err)
            Expect(result).To(Equal(expectedRetryable), description)
        },
        Entry("context timeout is retryable", context.DeadlineExceeded, true, "timeouts should retry"),
        Entry("K8s API 429 is retryable", apierrors.NewTooManyRequests("rate limited", 30), true, "rate limits should retry"),
        Entry("K8s API 503 is retryable", apierrors.NewServiceUnavailable("unavailable"), true, "503 should retry"),
        Entry("K8s API 500 is retryable", apierrors.NewInternalError(nil), true, "500 should retry"),
        Entry("K8s API 404 is permanent", apierrors.NewNotFound(schema.GroupResource{}, "test"), false, "404 should not retry"),
        Entry("K8s API 400 is permanent", apierrors.NewBadRequest("invalid"), false, "400 should not retry"),
        Entry("K8s API 401 is permanent", apierrors.NewUnauthorized("denied"), false, "401 should not retry"),
        Entry("K8s API 403 is permanent", apierrors.NewForbidden(schema.GroupResource{}, "", nil), false, "403 should not retry"),
    )

    Context("max attempts enforcement", func() {
        It("should allow retries up to max attempts (5)", func() {
            for attempt := 0; attempt < 5; attempt++ {
                shouldRetry := signalprocessing.ShouldRetry(attempt, context.DeadlineExceeded)
                Expect(shouldRetry).To(BeTrue(), "attempt %d should be allowed", attempt)
            }
        })

        It("should stop retrying after max attempts", func() {
            shouldRetry := signalprocessing.ShouldRetry(5, context.DeadlineExceeded)
            Expect(shouldRetry).To(BeFalse(), "should stop after 5 attempts")
        })

        It("should not retry permanent errors even on first attempt", func() {
            err := apierrors.NewNotFound(schema.GroupResource{}, "test")
            shouldRetry := signalprocessing.ShouldRetry(0, err)
            Expect(shouldRetry).To(BeFalse(), "permanent errors should not retry")
        })
    })
})
```

---

**Test Isolation Pattern** (Required for Integration/E2E):
```go
package signalprocessing

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SignalProcessing Controller", func() {
    var (
        ctx           context.Context
        testNamespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        // Unique namespace enables parallel execution (4 concurrent tests)
        testNamespace = fmt.Sprintf("test-sp-%s", uuid.New().String()[:8])
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

    It("should process signal successfully", func() {
        // Test uses unique namespace - safe for parallel execution
    })
})
```

**⚠️ Parallel Test Anti-Patterns** (AVOID):
- ❌ Hardcoded namespace names (`test-namespace`, `default`)
- ❌ Shared mutable state between tests (global vars modified during tests)
- ❌ Fixed port numbers without DD-TEST-001 allocation
- ❌ Tests that depend on execution order
- ❌ Tests that modify cluster-scoped resources

---

**Test Scenarios by Component** (Defined Upfront per TDD):

---

##### **K8s Enricher** (`test/unit/signalprocessing/enricher_test.go`) - 25-33 tests

**Happy Path (5-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-HP-01 | Pod signal enrichment | Signal with Pod resource | Returns Pod, Node, OwnerReference data |
| E-HP-02 | Deployment signal enrichment | Signal with Deployment resource | Returns Namespace, Deployment, ReplicaSet data |
| E-HP-03 | StatefulSet signal enrichment | Signal with StatefulSet resource | Returns Namespace, StatefulSet, PVC data |
| E-HP-04 | Service signal enrichment | Signal with Service resource | Returns Namespace, Service, Endpoints data |
| E-HP-05 | Node signal enrichment | Signal with Node resource | Returns Node details only |
| E-HP-06 | Standard depth fetching | Any valid signal | Fetches exactly standard depth objects (DD-017) |

**Edge Cases (12-15 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-EC-01 | Pod without owner | Orphan Pod signal | Returns Pod + Node, empty OwnerReference |
| E-EC-02 | Pod on deleted node | Pod signal, node doesn't exist | Returns Pod, Node=nil, continues without error |
| E-EC-03 | Deployment with 0 replicas | Deployment signal, no pods | Returns Deployment, empty pod list |
| E-EC-04 | Namespace not found | Signal in non-existent namespace | Returns partial context, logs warning |
| E-EC-05 | Cross-namespace owner | Pod owned by cluster-scoped resource | Returns Pod + cluster-scoped owner |
| E-EC-06 | Multiple owner references | Pod with multiple owners | Returns first owner (controller=true) |
| E-EC-07 | Resource name with special chars | Signal with `my-app_v2.0` name | Handles URL encoding correctly |
| E-EC-08 | Very long resource name | 253-char resource name | Handles within K8s limits |
| E-EC-09 | Resource in kube-system | Signal in kube-system namespace | Enriches normally (no special filtering) |
| E-EC-10 | Empty labels on resource | Resource without any labels | Returns resource with empty Labels map |
| E-EC-11 | Empty annotations on resource | Resource without annotations | Returns resource with empty Annotations map |
| E-EC-12 | Resource being deleted | Resource with DeletionTimestamp set | Returns resource with deletion status |

**Error Handling (8-10 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| E-ER-01 | K8s API timeout | Signal, API times out | Returns error with timeout code |
| E-ER-02 | K8s API 403 forbidden | Signal, RBAC denies access | Returns error with forbidden code, logs RBAC hint |
| E-ER-03 | K8s API 404 not found | Signal, resource deleted | Returns NotFound error, allows graceful handling |
| E-ER-04 | K8s API 500 server error | Signal, API server error | Returns error with retry hint |
| E-ER-05 | Invalid resource kind | Signal with unknown Kind | Returns validation error |
| E-ER-06 | Empty signal resource | Signal with empty resource ref | Returns validation error |
| E-ER-07 | Context cancelled | Signal, context cancelled mid-fetch | Returns context.Canceled error |
| E-ER-08 | Rate limited by API | Signal, API returns 429 | Returns error with backoff hint |

---

##### **Environment Classifier** (`test/unit/signalprocessing/environment_classifier_test.go`) - 21-28 tests

**Happy Path (5-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| EC-HP-01 | Production via namespace label | K8sContext with `environment: production` | Returns "production", confidence ≥0.95 |
| EC-HP-02 | Staging via namespace label | K8sContext with `environment: staging` | Returns "staging", confidence ≥0.90 |
| EC-HP-03 | Development via namespace label | K8sContext with `environment: development` | Returns "development", confidence ≥0.90 |
| EC-HP-04 | Production via namespace name | Namespace "prod-payments" | Returns "production", confidence ≥0.85 |
| EC-HP-05 | Staging via namespace name | Namespace "staging-api" | Returns "staging", confidence ≥0.80 |
| EC-HP-06 | Custom environment via Rego | K8sContext matching custom Rego rule | Returns custom env, confidence from Rego |

**Edge Cases (10-12 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| EC-EC-01 | Conflicting labels | `environment: prod` + `env: staging` | Uses primary label (`environment`), logs conflict |
| EC-EC-02 | Partial match namespace name | Namespace "production-like-test" | Returns "unknown" (avoid false positive) |
| EC-EC-03 | Case sensitivity | `Environment: PRODUCTION` | Normalizes to "production" |
| EC-EC-04 | Empty K8sContext | No namespace data | Returns "unknown", confidence 0.0 |
| EC-EC-05 | Nil namespace in context | K8sContext with namespace=nil | Returns "unknown", doesn't panic |
| EC-EC-06 | Very long label value | 253-char environment label | Handles within K8s limits |
| EC-EC-07 | Non-standard environment value | `environment: uat` | Returns "uat" as-is (customer-defined) |
| EC-EC-08 | Numeric label value | `environment: 1` | Returns "1" as string environment |
| EC-EC-09 | Multiple Rego rules match | Context matches 2 rules | Uses highest confidence rule |
| EC-EC-10 | No Rego rules match | Context doesn't match any rule | Returns "unknown", confidence 0.0 |

**Error Handling (6-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| EC-ER-01 | Rego policy syntax error | Malformed Rego policy | Returns error, logs policy issue |
| EC-ER-02 | Rego policy timeout | Policy takes >5s to evaluate | Returns error with timeout |
| EC-ER-03 | Rego policy runtime panic | Policy causes OPA panic | Returns error, doesn't crash service |
| EC-ER-04 | ConfigMap not found | ConfigMap not available in cluster | Graceful degradation, logs warning, continues without ConfigMap |
| EC-ER-05 | Invalid Rego output type | Rego returns number instead of string | Returns type validation error |
| EC-ER-06 | Nil input to classifier | Classify(ctx, nil, nil) | Returns validation error, doesn't panic |

---

##### **Priority Engine** (`test/unit/signalprocessing/priority_engine_test.go`) - 21-28 tests

**Happy Path (5-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PE-HP-01 | P0 - Critical production | Production env, critical severity | Returns "P0", confidence ≥0.95 |
| PE-HP-02 | P1 - Warning production | Production env, warning severity | Returns "P1", confidence ≥0.90 |
| PE-HP-03 | P2 - Info production | Production env, info severity | Returns "P2", confidence ≥0.85 |
| PE-HP-04 | P1 - Critical staging | Staging env, critical severity | Returns "P1", confidence ≥0.90 |
| PE-HP-05 | P3 - Development info | Development env, info severity | Returns "P3", confidence ≥0.85 |
| PE-HP-06 | Custom priority via Rego | Matching custom Rego rule | Returns custom priority (P0-P3) |

**Edge Cases (10-12 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PE-EC-01 | Empty string environment | Environment "", critical severity | Rego handles, returns priority based on severity |
| PE-EC-02 | Empty string severity | Production env, severity "" | Fallback to P2 (unknown severity) |
| PE-EC-03 | Both empty | Environment "" and severity "" | Fallback to P2 (severity-based) |
| PE-EC-04 | Case normalization | "PRODUCTION", "CRITICAL" | Normalizes before evaluation |
| PE-EC-05 | Custom severity value | Production env, "urgent" | Handles customer-defined severity |
| PE-EC-06 | Multiple Rego rules match | Context matches 2 priority rules | Uses highest priority (lowest P-number) |
| PE-EC-07 | Boundary condition | Exactly on severity threshold | Rounds up to higher priority |
| PE-EC-08 | Namespace labels missing | No namespace_labels in context | Returns priority with reduced confidence |
| PE-EC-09 | Deployment labels missing | No deployment_labels in context | Returns priority with reduced confidence |
| PE-EC-10 | All labels present | Complete namespace + deployment labels | Returns highest confidence |

**Error Handling (6-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PE-ER-01 | Rego policy syntax error | Malformed priority Rego policy | Returns construction error (cannot fallback - no valid engine exists) |
| PE-ER-02 | Rego policy timeout | Policy takes >100ms | Fallback to severity-based (BR-SP-071) |
| PE-ER-03 | Nil environment classification | AssignPriority with nil env | Returns validation error |
| PE-ER-04 | Invalid Rego output | Rego returns "PX" (invalid) | Returns validation error |
| PE-ER-05 | Rego returns out of range | Rego returns "P5" (not P0-P3) | Returns validation error |
| PE-ER-06 | Context cancelled | Context cancelled mid-evaluation | Fallback to severity-based (BR-SP-071 - never fail) |

**Fallback Tests (4 tests)** - Per BR-SP-071 severity-based fallback:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PE-FB-01 | Fallback critical | Rego fails, severity="critical" | Returns "P1", source="fallback-severity" |
| PE-FB-02 | Fallback warning | Rego fails, severity="warning" | Returns "P2", source="fallback-severity" |
| PE-FB-03 | Fallback info | Rego fails, severity="info" | Returns "P3", source="fallback-severity" |
| PE-FB-04 | Fallback unknown | Rego fails, severity="" | Returns "P2", source="fallback-severity" |

**Hot-Reload Integration Tests** - Per BR-SP-072 (`test/integration/signalprocessing/hot_reloader_test.go`):
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| PE-HR-01 | Policy file change detection | Update priority.rego file | FileWatcher triggers callback |
| PE-HR-02 | Policy hot-reload success | Valid new policy content | New policy takes effect without restart |
| PE-HR-03 | Policy hot-reload graceful degradation | Invalid new policy content | Old policy retained, error logged |
| PE-HR-04 | Concurrent requests during reload | Requests during policy swap | All requests complete (no race conditions) |

---

##### **Business Classifier** (`test/unit/signalprocessing/business_classifier_test.go`) - 18-25 tests

**Happy Path (5-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| BC-HP-01 | Payment service classification | Signal from payment-service pod | Returns business_domain="payments", criticality="high" |
| BC-HP-02 | API gateway classification | Signal from api-gateway pod | Returns business_domain="platform", criticality="critical" |
| BC-HP-03 | Background job classification | Signal from worker-job pod | Returns business_domain="processing", criticality="low" |
| BC-HP-04 | Classification via labels | Pod with `team: checkout` label | Returns team="checkout" in context |
| BC-HP-05 | Classification via namespace | Namespace "billing-prod" | Returns business_domain="billing" |
| BC-HP-06 | Custom Rego business rules | Matching custom business Rego | Returns customer-defined classification |

**Edge Cases (8-10 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| BC-EC-01 | No business context available | Generic pod with no business labels | Returns empty BusinessContext, confidence 0.0 |
| BC-EC-02 | Conflicting business labels | `domain: payments` + `service: orders` | Uses primary label hierarchy |
| BC-EC-03 | Unknown business domain | Pod in "misc" namespace | Returns business_domain="unknown" |
| BC-EC-04 | Multiple team labels | `team: a` + `owner: b` | Returns both in context |
| BC-EC-05 | Very long business label | 63-char business label value (K8s limit) | Handles within K8s label value limits |
| BC-EC-06 | Non-ASCII business name | `team: 支付团队` | Handles UTF-8 correctly |
| BC-EC-07 | Whitespace in labels | `team: " checkout "` | Trims whitespace |
| BC-EC-08 | No Rego rules match | Context doesn't match any business rule | Returns minimal context |

**Confidence Tier Tests (BR-SP-080)** (4 tests):
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| BC-CF-01 | Explicit label detection | `kubernaut.ai/business-unit=payments` | Returns confidence 1.0, source="label" |
| BC-CF-02 | Pattern match detection | Namespace "billing-prod" (no labels) | Returns confidence 0.8, source="pattern" |
| BC-CF-03 | Rego inference | Only Rego rule matches | Returns confidence 0.6, source="rego" |
| BC-CF-04 | Default fallback | No detection possible | Returns confidence 0.4, source="default" |

**Error Handling (5-7 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| BC-ER-01 | Rego policy syntax error | Malformed business Rego | Returns error, logs policy issue |
| BC-ER-02 | Rego policy timeout | Policy takes >200ms | Uses default fallback (0.4 confidence) |
| BC-ER-03 | Nil K8sContext | Classify with nil context | Returns validation error |
| BC-ER-04 | Invalid Rego output structure | Rego returns string instead of object | Returns type validation error |
| BC-ER-05 | Context cancelled | Context cancelled mid-evaluation | Uses default fallback (graceful degradation) |

---

##### **Audit Client** (`test/unit/signalprocessing/audit_client_test.go`) - 14-20 tests

**Happy Path (3-5 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| AC-HP-01 | Successful audit write | Valid categorization result | Fire-and-forget, no error returned |
| AC-HP-02 | Audit buffering | Multiple rapid writes | All buffered, flushed in batch |
| AC-HP-03 | Audit with full context | Complete SignalProcessing status | All fields serialized correctly |
| AC-HP-04 | Async write verification | Write audit, check buffer | Audit in buffer within 1ms |

**Edge Cases (6-8 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| AC-EC-01 | Large audit payload | 1MB audit data | Handles large payload |
| AC-EC-02 | Unicode in audit data | Japanese/Chinese characters | Correctly encoded |
| AC-EC-03 | Empty optional fields | Audit with nil BusinessContext | Writes with empty fields |
| AC-EC-04 | Buffer at capacity | Buffer full (1000 items) | Oldest items flushed first |
| AC-EC-05 | Flush interval trigger | Time-based flush | Flushes after configured interval |
| AC-EC-06 | Special characters in IDs | Signal ID with `/` or `:` | Correctly escaped |

**Error Handling (5-7 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| AC-ER-01 | Data Storage unavailable | Write, Data Storage down | Logs error, doesn't block business (ADR-038) |
| AC-ER-02 | Data Storage timeout | Write, Data Storage slow | Timeout after configured period, logs warning |
| AC-ER-03 | Data Storage 4xx error | Write, validation error | Logs error with response body |
| AC-ER-04 | Data Storage 5xx error | Write, server error | Logs error, may retry based on config |
| AC-ER-05 | Network partition | Write, network unreachable | Logs error, doesn't block business |
| AC-ER-06 | Invalid audit data | Nil categorization result | Returns validation error |

---

##### **Reconciler** (`test/unit/signalprocessing/reconciler_test.go`) - 26-39 tests

**Happy Path (8-12 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-HP-01 | Full happy path | Valid SignalProcessing CR | Status=Completed, all fields populated |
| R-HP-02 | Phase: Pending → Enriching | New CR | Status.Phase="Enriching" |
| R-HP-03 | Phase: Enriching → Classifying | Enriched CR | Status.Phase="Classifying" |
| R-HP-04 | Phase: Classifying → Completed | Classified CR | Status.Phase="Completed" |
| R-HP-05 | Requeue on transient error | API timeout during enrichment | Requeued with backoff |
| R-HP-06 | No requeue on permanent error | Invalid signal (validation) | Status=Failed, no requeue |
| R-HP-07 | Finalizer added | New CR | Finalizer present |
| R-HP-08 | Finalizer cleanup | CR deleted | Finalizer removed after cleanup |
| R-HP-09 | Metrics updated | Any reconciliation | Metrics recorded |
| R-HP-10 | Audit written | Completed reconciliation | Audit sent to Data Storage |

**Edge Cases (10-15 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-EC-01 | CR deleted during enrichment | CR deleted mid-reconcile | Graceful termination, no error |
| R-EC-02 | CR modified during reconcile | CR updated mid-reconcile | Detects conflict, requeues |
| R-EC-03 | Already completed CR | CR with Status=Completed | No-op, no requeue |
| R-EC-04 | Already failed CR | CR with Status=Failed | No-op, no requeue |
| R-EC-05 | Empty signal in spec | CR with empty Signal field | Status=Failed, validation error |
| R-EC-06 | Unknown resource kind | Signal with Kind="Unknown" | Status=Failed, validation error |
| R-EC-07 | Very old CR | CR created 24h ago | Still processes (no timeout) |
| R-EC-08 | Concurrent reconciles | Same CR reconciled twice | Only one succeeds (optimistic locking) |
| R-EC-09 | Owner reference present | CR with owner reference | Respects owner, no orphan cleanup |
| R-EC-10 | No owner reference | Orphan CR | Still processes normally |
| R-EC-11 | Namespace being deleted | CR in terminating namespace | Graceful handling |
| R-EC-12 | Low confidence classification | Environment confidence <0.5 | Logs warning, continues |
| R-EC-13 | All classifiers return unknown | No matching Rego rules | Status=Completed with unknowns |

**Error Handling (8-12 tests)**:
| ID | Scenario | Input | Expected Outcome |
|----|----------|-------|------------------|
| R-ER-01 | K8s API unavailable | CR, API server down | Requeue with exponential backoff |
| R-ER-02 | Enricher fails | CR, enrichment error | Status=Failed, error in conditions |
| R-ER-03 | Environment classifier fails | CR, classification error | Status=Failed, error in conditions |
| R-ER-04 | Priority engine fails | CR, priority error | Status=Failed, error in conditions |
| R-ER-05 | Business classifier fails | CR, business error | Status=Failed, error in conditions |
| R-ER-06 | Audit write fails | CR, audit error | Logs warning, Status=Completed (ADR-038) |
| R-ER-07 | Status update conflict | CR, concurrent update | Retries with fresh version |
| R-ER-08 | Context timeout | CR, slow processing | Returns error, requeues |
| R-ER-09 | Panic recovery | CR, internal panic | Recovers, logs stack trace, Status=Failed |
| R-ER-10 | RBAC permission denied | CR, missing RBAC | Status=Failed, logs RBAC hint |

---

**Test Count Summary**:

| Component | Happy Path | Edge Cases | Error Handling | **Total** |
|-----------|------------|------------|----------------|-----------|
| K8s Enricher | 6 | 12 | 8 | **26** |
| Environment Classifier | 6 | 10 | 6 | **22** |
| Priority Engine | 6 | 10 | 6 | **22** |
| Business Classifier | 6 | 8 | 5 | **19** |
| Audit Client | 4 | 6 | 6 | **16** |
| Reconciler | 10 | 13 | 10 | **33** |
| **Total** | **38** | **59** | **41** | **138** |

**Key Principle**: Mock ONLY external dependencies (K8s API, Data Storage, LLM). Use REAL business logic (Enricher, Classifiers, Rego Engine).

---

#### **Day 9: Unit Tests (70%+ Coverage)** - FOUNDATION LAYER

**Why Unit Tests First?**
- Validates component logic before integration
- Fast feedback loop during development
- 70%+ of BRs covered at this layer
- Foundation for integration and E2E tests

**Unit Test Files**:

**Test Files**:
- `test/unit/signalprocessing/enricher_test.go`
- `test/unit/signalprocessing/environment_classifier_test.go`
- `test/unit/signalprocessing/priority_engine_test.go`
- `test/unit/signalprocessing/business_classifier_test.go`
- `test/unit/signalprocessing/audit_client_test.go`

**Table-Driven Testing Patterns (Recommended)**:

Why table-driven tests?
- **38% code reduction** (from Gateway experience)
- **25-40% faster** to add new test cases
- **Better maintainability**: Change logic once, all entries benefit
- **Clearer coverage**: Easy to see all scenarios at a glance

**Pattern 1: Classification Success Scenarios**
```go
package signalprocessing

import (
    "context"
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

var _ = Describe("BR-SP-003: EnvironmentClassifier", func() {
    var (
        classifier *signalprocessing.EnvironmentClassifier
        ctx        context.Context
    )

    BeforeEach(func() {
        classifier = signalprocessing.NewEnvironmentClassifier(policy, logger)
        ctx = context.Background()
    })

    DescribeTable("should classify environment correctly based on labels",
        func(namespaceLabels map[string]string, signalLabels map[string]string, expectedEnv string, expectedConfidence float64) {
            k8sCtx := buildK8sContext(namespaceLabels)
            signal := buildSignal(signalLabels)

            result, err := classifier.Classify(ctx, k8sCtx, signal)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Environment).To(Equal(expectedEnv))
            Expect(result.Confidence).To(BeNumerically(">=", expectedConfidence))
        },
        Entry("production via namespace label",
            map[string]string{"environment": "production"},
            nil,
            "production", 0.95),
        Entry("staging via signal label fallback",
            nil,
            map[string]string{"environment": "staging"},
            "staging", 0.80),
        Entry("unknown when no labels present",
            nil,
            nil,
            "unknown", 0.0),
        Entry("production namespace overrides signal staging",
            map[string]string{"environment": "production"},
            map[string]string{"environment": "staging"},
            "production", 0.95),
    )
})
```

**Pattern 2: Error Scenarios**
```go
package signalprocessing

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    "sigs.k8s.io/controller-runtime/pkg/client/interceptor"

    "github.com/jordigilh/kubernaut/pkg/signalprocessing"
)

var _ = Describe("BR-SP-002: K8sEnricher Error Handling", func() {
    // ✅ CORRECT: Use fake.NewClientBuilder() with WithInterceptorFuncs() for error simulation
    // ❌ FORBIDDEN: Custom MockK8sClient implementations (per ADR-004)

    DescribeTable("should handle enrichment errors gracefully",
        func(interceptor interceptor.Funcs, expectedError string) {
            // Use fake client with interceptor for error injection
            fakeClient := fake.NewClientBuilder().
                WithScheme(scheme).
                WithInterceptorFuncs(interceptor).
                Build()

            enricher := signalprocessing.NewK8sEnricher(fakeClient, logger)

            result, err := enricher.EnrichSignal(ctx, signal)

            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring(expectedError))
            Expect(result).To(BeNil())
        },
        Entry("namespace not found",
            interceptor.Funcs{
                Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
                    if _, ok := obj.(*corev1.Namespace); ok {
                        return apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, key.Name)
                    }
                    return client.Get(ctx, key, obj, opts...)
                },
            },
            "namespace not found"),
        Entry("pod access forbidden",
            interceptor.Funcs{
                Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
                    if _, ok := obj.(*corev1.Pod); ok {
                        return apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, key.Name, nil)
                    }
                    return client.Get(ctx, key, obj, opts...)
                },
            },
            "forbidden"),
        Entry("API server timeout",
            interceptor.Funcs{
                Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
                    if _, ok := obj.(*corev1.Pod); ok {
                        return context.DeadlineExceeded
                    }
                    return client.Get(ctx, key, obj, opts...)
                },
            },
            "deadline exceeded"),
    )
})
```

**Pattern 3: Enrichment Depth Scenarios (DD-017)**
```go
package signalprocessing

var _ = Describe("BR-SP-002: Signal-Driven Enrichment Depth", func() {
    DescribeTable("should enrich based on signal resource kind (DD-017)",
        func(resourceKind string, expectedContextFields []string, unexpectedContextFields []string) {
            signal := &signalprocessingv1alpha1.SignalData{
                Resource: signalprocessingv1alpha1.ResourceReference{
                    Kind:      resourceKind,
                    Name:      "test-resource",
                    Namespace: "test-ns",
                },
            }

            result, err := enricher.EnrichSignal(ctx, signal)

            Expect(err).ToNot(HaveOccurred())
            for _, field := range expectedContextFields {
                Expect(result).To(HaveField(field, Not(BeNil())), "expected %s to be present", field)
            }
            for _, field := range unexpectedContextFields {
                Expect(result).To(HaveField(field, BeNil()), "expected %s to be nil", field)
            }
        },
        Entry("Pod signal fetches Namespace+Pod+Node+Owner",
            "Pod",
            []string{"Namespace", "Pod", "Node", "Owner"},
            []string{}),
        Entry("Deployment signal fetches Namespace+Workload only",
            "Deployment",
            []string{"Namespace", "Workload"},
            []string{"Pod", "Node"}),
        Entry("Node signal fetches Node only",
            "Node",
            []string{"Node"},
            []string{"Namespace", "Pod", "Workload"}),
    )
})
```

**Pattern 4: Rego Policy Evaluation**
```go
package signalprocessing

var _ = Describe("BR-SP-006: Rego Policy Evaluation", func() {
    DescribeTable("should evaluate Rego policies correctly",
        func(policyName string, inputData map[string]interface{}, expectedDecision string, expectedConfidence float64) {
            engine := signalprocessing.NewRegoEngine(policies, logger)

            result, err := engine.Evaluate(ctx, policyName, inputData)

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Decision).To(Equal(expectedDecision))
            Expect(result.Confidence).To(BeNumerically(">=", expectedConfidence))
        },
        Entry("production environment via namespace label",
            "environment_classification",
            map[string]interface{}{"namespace_labels": map[string]string{"env": "prod"}},
            "production", 0.95),
        Entry("critical priority for production outage",
            "priority_assignment",
            map[string]interface{}{"environment": "production", "signal_severity": "critical"},
            "P0", 0.90),
        Entry("pci business classification for payment namespace",
            "business_classification",
            map[string]interface{}{"namespace": "payment-processing"},
            "pci-compliant", 0.85),
    )
})
```

**Best Practices for Table-Driven Tests**:
1. Use descriptive Entry names that document the scenario
2. Keep table logic simple and consistent
3. Use traditional `It()` for truly unique scenarios
4. Group related scenarios in same `DescribeTable`
5. Add new scenarios by just adding `Entry()` (no code duplication)
6. All entries must map to a BR-SP-XXX requirement

#### **Day 10: Integration Tests (ENVTEST)**

**Test Environment**: ENVTEST (confirmed)
**Target**: 50-80 integration tests across 4 test files
**Test Pattern**: Real K8s API (envtest) + real controller manager + isolated namespaces

---

##### **Integration Test File Structure**

| File | Purpose | Test Count |
|------|---------|------------|
| `reconciler_integration_test.go` | Reconciler phase transitions, status updates | ~25 |
| `component_integration_test.go` | K8sEnricher, Classifiers with real K8s | ~20 |
| `rego_integration_test.go` | Rego policy evaluation with real ConfigMap | ~15 |
| `hot_reloader_test.go` | Hot-reload + concurrent access | ~5 |

---

##### **File 1: Reconciler Integration Tests** (`test/integration/signalprocessing/reconciler_integration_test.go`)

**CRITICAL**: ENVTEST requires controller manager setup (not just client):

```go
package signalprocessing_test

import (
    "context"
    "fmt"
    "path/filepath"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/rest"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"
    metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

    signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
    controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
)

const (
    timeout  = 30 * time.Second
    interval = 250 * time.Millisecond
)

var (
    testEnv   *envtest.Environment
    cfg       *rest.Config
    k8sClient client.Client
    scheme    *runtime.Scheme
    ctx       context.Context
    cancel    context.CancelFunc
)

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    // Setup scheme
    scheme = runtime.NewScheme()
    Expect(corev1.AddToScheme(scheme)).To(Succeed())
    Expect(signalprocessingv1alpha1.AddToScheme(scheme)).To(Succeed())

    // Start envtest
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "..", "config", "crd", "bases"),
        },
    }
    var err error
    cfg, err = testEnv.Start()
    Expect(err).ToNot(HaveOccurred())

    // Create client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
    Expect(err).ToNot(HaveOccurred())

    // Start controller manager with reconciler
    mgr, err := ctrl.NewManager(cfg, ctrl.Options{
        Scheme: scheme,
        Metrics: metricsserver.Options{BindAddress: "0"}, // Disable metrics
    })
    Expect(err).ToNot(HaveOccurred())

    // Register reconciler
    reconciler := &controller.SignalProcessingReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Logger: ctrl.Log.WithName("test"),
        // ... other dependencies injected
    }
    Expect(reconciler.SetupWithManager(mgr)).To(Succeed())

    // Start manager in background
    go func() {
        defer GinkgoRecover()
        Expect(mgr.Start(ctx)).To(Succeed())
    }()
})

var _ = AfterSuite(func() {
    cancel()
    Expect(testEnv.Stop()).To(Succeed())
})

// Helper: Create unique namespace for test isolation (PARALLEL EXECUTION)
func createTestNamespace(prefix string) string {
    ns := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
    namespace := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: ns},
    }
    Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
    return ns
}

var _ = Describe("SignalProcessing Reconciler Integration", func() {
    // ========================================
    // HAPPY PATH TESTS (10 tests)
    // ========================================

    Context("Happy Path - Phase Transitions", func() {
        It("BR-SP-070, BR-SP-051: should process production pod signal and assign P0 priority", func() {
            ns := createTestNamespace("it-hp-01")
            defer k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})

            sp := &signalprocessingv1alpha1.SignalProcessing{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-signal",
                    Namespace: ns,
                },
                Spec: signalprocessingv1alpha1.SignalProcessingSpec{
                    Signal: signalprocessingv1alpha1.SignalData{
                        Name:      "HighCPU",
                        Severity:  "critical",
                        Namespace: "production",
                        Resource: signalprocessingv1alpha1.ResourceReference{
                            Kind: "Pod",
                            Name: "api-server-xyz",
                        },
                    },
                },
            }
            Expect(k8sClient.Create(ctx, sp)).To(Succeed())

            Eventually(func() string {
                var updated signalprocessingv1alpha1.SignalProcessing
                k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &updated)
                return string(updated.Status.Phase)
            }, timeout, interval).Should(Equal("Complete"))

            var final signalprocessingv1alpha1.SignalProcessing
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(sp), &final)).To(Succeed())
            Expect(final.Status.EnvironmentClassification.Environment).To(Equal("production"))
            Expect(final.Status.PriorityAssignment.Priority).To(Equal("P0"))
        })

        It("BR-SP-070, BR-SP-051: should process staging deployment signal and assign P1 priority", func() {
            // Similar structure with staging namespace
        })

        It("BR-SP-070, BR-SP-051: should process dev service signal and assign P3 priority", func() {
            // Similar structure with dev namespace
        })

        It("BR-SP-051: should detect environment from namespace label", func() {
            // Create namespace with kubernaut.ai/environment label
        })

        It("BR-SP-052: should fallback to ConfigMap for environment", func() {
            // Create ConfigMap with environment mapping
        })

        It("BR-SP-002: should classify business context with labels", func() {
            // Create namespace with business labels
        })

        It("BR-SP-100: should build owner chain (Pod→RS→Deployment)", func() {
            // Create Pod with owner references
        })

        It("BR-SP-101: should detect PDB protection", func() {
            // Create PDB matching pod labels
        })

        It("BR-SP-101: should detect HPA enabled", func() {
            // Create HPA targeting deployment
        })

        It("BR-SP-102: should populate CustomLabels from Rego policy", func() {
            // Create ConfigMap with labels.rego policy
        })
    })

    // ========================================
    // EDGE CASE TESTS (8 tests)
    // ========================================

    Context("Edge Cases", func() {
        It("BR-SP-053: should default to unknown environment", func() {
            // No namespace labels, no ConfigMap mapping
        })

        It("BR-SP-001: should use degraded mode when pod not found", func() {
            // Signal references non-existent pod
        })

        It("Controller: should handle concurrent reconciliation", func() {
            // Create 10 SignalProcessing CRs simultaneously
        })

        It("Robustness: should handle empty signal data gracefully", func() {
            // Minimal valid SignalProcessing spec
        })

        It("Robustness: should handle namespace with special characters", func() {
            // Namespace with dashes and numbers
        })

        It("BR-SP-100: should handle max owner chain depth (5 levels)", func() {
            // Create deep owner chain (Pod→RS→Deployment→...)
        })

        It("BR-SP-103: should handle empty FailedDetections on success", func() {
            // Verify FailedDetections is empty when all queries succeed
        })

        It("BR-SP-102: should handle multiple Rego policy keys", func() {
            // Policy returns multiple custom labels
        })
    })

    // ========================================
    // ERROR HANDLING TESTS (7 tests)
    // ========================================

    Context("Error Handling", func() {
        It("Error Cat. B: should retry on K8s API timeout", func() {
            // Simulate transient API error
        })

        It("Error Cat. D: should retry on status update conflict", func() {
            // Simulate optimistic locking conflict
        })

        It("Error Cat. B: should handle context cancellation gracefully", func() {
            // Cancel context during reconciliation
        })

        It("Error Cat. C: should log Rego policy syntax error", func() {
            // Create ConfigMap with invalid Rego
        })

        It("BR-SP-103: should track PDB query failure in FailedDetections", func() {
            // Simulate RBAC denial for PDB list
        })

        It("ADR-038: should complete even if audit write fails", func() {
            // Audit client returns error
        })

        It("Error Cat. A: should mark failed for permanent errors", func() {
            // Invalid CRD spec that can't be processed
        })
    })
})
```

---

##### **Reconciler Integration Test Matrix** (25 tests)

| BR | Category | Scenario | Input | Expected |
|----|----------|----------|-------|----------|
| **BR-SP-070, BR-SP-051** | Happy Path | Production pod → P0 | `namespace: production, severity: critical` | P0, env: production |
| **BR-SP-070, BR-SP-051** | Happy Path | Staging deployment → P1 | `namespace: staging, severity: warning` | P1, env: staging |
| **BR-SP-070, BR-SP-051** | Happy Path | Dev service → P3 | `namespace: dev, severity: info` | P3, env: development |
| **BR-SP-051** | Happy Path | Environment from label | `namespace.labels[kubernaut.ai/environment]=prod` | env: production |
| **BR-SP-052** | Happy Path | ConfigMap fallback | ConfigMap mapping `test-ns→staging` | env: staging |
| **BR-SP-002** | Happy Path | Business classification | `namespace.labels[kubernaut.ai/team]=payments` | businessUnit: payments |
| **BR-SP-100** | Happy Path | Owner chain traversal | Pod with RS→Deployment owners | Chain: [RS, Deployment] |
| **BR-SP-101** | Happy Path | PDB detection | PDB selector matches pod | pdbProtected: true |
| **BR-SP-101** | Happy Path | HPA detection | HPA targets deployment | hpaEnabled: true |
| **BR-SP-102** | Happy Path | CustomLabels from Rego | ConfigMap with labels.rego | CustomLabels populated |
| **BR-SP-053** | Edge Case | Default environment | No labels, no ConfigMap | env: unknown |
| **BR-SP-001** | Edge Case | Degraded mode | Pod not found | DegradedMode: true |
| **Controller** | Edge Case | Concurrent reconciliation | 10 CRs at once | All complete |
| **Robustness** | Edge Case | Minimal spec | Empty labels | Default values |
| **Robustness** | Edge Case | Special namespace | `my-ns-123` | Handles correctly |
| **BR-SP-100** | Edge Case | Max owner depth | 5+ levels | Stops at 5 |
| **BR-SP-103** | Edge Case | No failed detections | All queries succeed | FailedDetections: [] |
| **BR-SP-102** | Edge Case | Multi-key Rego | Policy returns 3 keys | All 3 in CustomLabels |
| **Error Cat. B** | Error | K8s API timeout | Transient 503 | Retry + succeed |
| **Error Cat. D** | Error | Status conflict | Concurrent update | Retry + succeed |
| **Error Cat. B** | Error | Context cancelled | Cancel during reconcile | Clean exit |
| **Error Cat. C** | Error | Rego syntax error | Invalid policy | Log error, use defaults |
| **BR-SP-103** | Error | PDB RBAC denied | No list permission | FailedDetections: [pdb] |
| **ADR-038** | Error | Audit write failure | Audit returns error | Continue processing |
| **Error Cat. A** | Error | Permanent error | Invalid spec | Phase: Failed |

---

##### **File 2: Component Integration Tests** (`test/integration/signalprocessing/component_integration_test.go`)

**Test Matrix** (20 tests):

| BR | Category | Component | Scenario |
|----|----------|-----------|----------|
| **BR-SP-001** | K8sEnricher | Enricher | Pod enrichment with real K8s |
| **BR-SP-001** | K8sEnricher | Enricher | Deployment enrichment |
| **BR-SP-001** | K8sEnricher | Enricher | Node enrichment |
| **BR-SP-001** | K8sEnricher | Enricher | StatefulSet enrichment |
| **BR-SP-001** | K8sEnricher | Enricher | Service enrichment |
| **BR-SP-001** | K8sEnricher | Enricher | Namespace context |
| **BR-SP-001** | K8sEnricher | Enricher | Degraded mode fallback |
| **BR-SP-052** | Environment | Classifier | Real ConfigMap lookup |
| **BR-SP-051** | Environment | Classifier | Namespace label priority |
| **BR-SP-072** | Environment | Classifier | Hot-reload policy change |
| **BR-SP-070** | Priority | Engine | Real Rego evaluation |
| **BR-SP-071** | Priority | Engine | Severity fallback |
| **BR-SP-072** | Priority | Engine | ConfigMap policy load |
| **BR-SP-002** | Business | Classifier | Label-based classification |
| **BR-SP-002** | Business | Classifier | Pattern-based classification |
| **BR-SP-100** | OwnerChain | Builder | Real K8s traversal |
| **BR-SP-100** | OwnerChain | Builder | Cross-namespace owner |
| **BR-SP-101** | Detection | LabelDetector | Real PDB query |
| **BR-SP-101** | Detection | LabelDetector | Real HPA query |
| **BR-SP-101** | Detection | LabelDetector | Real NetworkPolicy query |

---

##### **File 3: Rego Integration Tests** (`test/integration/signalprocessing/rego_integration_test.go`)

**Test Matrix** (15 tests):

| BR | Category | Scenario |
|----|----------|----------|
| **BR-SP-051** | Policy Load | ConfigMap environment.rego |
| **BR-SP-070** | Policy Load | ConfigMap priority.rego |
| **BR-SP-102** | Policy Load | ConfigMap labels.rego |
| **BR-SP-051** | Evaluation | Environment classification |
| **BR-SP-070** | Evaluation | Priority assignment |
| **BR-SP-102** | Evaluation | CustomLabels extraction |
| **BR-SP-104** | Security | System prefix stripping |
| **BR-SP-071** | Fallback | Invalid policy → defaults |
| **BR-SP-053** | Fallback | Missing ConfigMap → defaults |
| **Stability** | Concurrent | 10 parallel evaluations |
| **BR-SP-072** | Concurrent | Policy update during eval |
| **DD-WORKFLOW-001** | Timeout | 5s timeout enforcement |
| **DD-WORKFLOW-001** | Validation | Key truncation (63 chars) |
| **DD-WORKFLOW-001** | Validation | Value truncation (100 chars) |
| **DD-WORKFLOW-001** | Validation | Max keys truncation (10) |

---

##### **File 4: Hot-Reload Integration Tests** (`test/integration/signalprocessing/hot_reloader_test.go`)

**Test Matrix** (5 tests):

| BR | Category | Scenario |
|----|----------|----------|
| **BR-SP-072** | File Watch | Policy file change detected |
| **BR-SP-072** | Reload | Valid policy takes effect |
| **BR-SP-072** | Graceful | Invalid policy → old retained |
| **BR-SP-072** | Concurrent | Update during active reconciliation |
| **BR-SP-072** | Recovery | Watcher restart after error |

---

### **Days 11-12: Finalization**

#### **Day 11: E2E Tests and Documentation**

**Reference**: [DD-TEST-001: Port Allocation Strategy](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

**E2E Infrastructure Setup** (Kind NodePort - NO port-forward):

**Step 1: Kind Config** (`test/infrastructure/kind-signalprocessing-config.yaml`)
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  # Metrics endpoint (controller doesn't have HTTP API)
  - containerPort: 30182    # Signal Processing Metrics NodePort
    hostPort: 9182          # localhost:9182/metrics
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

**Step 2: Controller Service** (`test/e2e/signalprocessing/deployment.yaml`)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller-metrics
spec:
  type: NodePort
  selector:
    app: signalprocessing-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30182  # From DD-TEST-001
```

**Step 3: Test Suite** (NodePort access - no port-forward)
```go
// All parallel processes use same NodePort URL
metricsURL = "http://localhost:9182/metrics"

// Wait for controller readiness via NodePort
Eventually(func() error {
    resp, err := http.Get(metricsURL)
    if err != nil { return err }
    defer resp.Body.Close()
    return nil
}, 60*time.Second, 2*time.Second).Should(Succeed())
```

**Port Allocation** (from DD-TEST-001 - AUTHORITATIVE):

| Port Type | Port | Purpose |
|-----------|------|---------|
| **Internal Metrics** | `9090` | Prometheus metrics endpoint (container) |
| **Internal Health** | `8081` | Health/Ready probes (container) |
| **Host Port** | `8082` | Kind extraPortMappings (localhost access) |
| **NodePort (API)** | `30082` | K8s NodePort for service access |
| **NodePort (Metrics)** | `30182` | K8s NodePort for metrics scraping |
| **Host Metrics** | `9182` | localhost:9182 → 30182 (Kind mapping) |

**Kind Config**: `test/infrastructure/kind-signalprocessing-config.yaml`

**Full DD-TEST-001 Reference**:
```
Signal Processing Service:
├── Internal Ports (Container)
│   ├── 9090 - Metrics (/metrics)
│   └── 8081 - Health (/healthz, /readyz)
├── E2E Tests (Kind NodePort)
│   ├── Host Port: 8082 (extraPortMappings)
│   ├── NodePort: 30082 (API)
│   └── Metrics NodePort: 30182
└── Config: test/infrastructure/kind-signalprocessing-config.yaml
```

**E2E Test Scenarios**: `test/e2e/signalprocessing/`
- Happy path: RemediationRequest → SignalProcessing → Complete
- Error recovery: Transient K8s API failure → Retry → Success
- Timeout: Enrichment timeout → Failed with error message

**Documentation Updates**:
- Update `docs/services/crd-controllers/01-signalprocessing/` with implementation details
- Create `BUILD.md`, `OPERATIONS.md`, `DEPLOYMENT.md` from DD-006 templates

#### **Day 12: Gateway Code Migration (CRITICAL)**

> **KEY PRINCIPLE**: We are MOVING code from Gateway to Signal Processing, not rewriting it.
> The existing Gateway code is tested and working - reuse it!

---

### **📊 Gateway Migration Triage Summary (v1.18 - VERIFIED)**

> **Last Verified**: December 2, 2025 (actual line counts confirmed)

| Category | Source | Lines/Tests | Effort |
|----------|--------|-------------|--------|
| **Production Code** | `pkg/gateway/processing/classification.go` | **259 lines** ✅ | 1h |
| **Production Code** | `pkg/gateway/processing/priority.go` | **219 lines** ✅ | 1h |
| **Unit Tests** | `test/unit/gateway/processing/environment_classification_test.go` | **385 lines** ✅ | 1.5h |
| **Unit Tests** | `test/unit/gateway/priority_classification_test.go` | **472 lines** ✅ | 1.5h |
| **Rego Policy** | `config.app/gateway/policies/priority.rego` | ~73 lines | 30m |
| **Gateway Updates** | Remove classification from server, crd_creator, config, metrics | ~10 files | 2h |
| **Integration Tests** | Update 5 Gateway integration test files | ~5 files | 1h |
| **E2E Tests** | Update 1 Gateway E2E test file | 1 file | 30m |
| **TOTAL** | | **~1,335 lines, ~48 tests** | **~8h (1 day)** |

**OPA Library**: `github.com/open-policy-agent/opa/v1/rego` (already used by Gateway - official OPA library)

**Files to MOVE** (copy to Signal Processing, then delete from Gateway):
- `pkg/gateway/processing/classification.go` → `pkg/signalprocessing/classifier/environment.go`
- `pkg/gateway/processing/priority.go` → `pkg/signalprocessing/classifier/priority.go`
- `test/unit/gateway/processing/environment_classification_test.go` → `test/unit/signalprocessing/environment_classifier_test.go`
- `test/unit/gateway/priority_classification_test.go` → `test/unit/signalprocessing/priority_engine_test.go`
- `config.app/gateway/policies/priority.rego` → `config.app/signalprocessing/policies/priority.rego`

**Files to UPDATE in Gateway** (remove classification logic):
- `pkg/gateway/server.go` - Remove classifier/priorityEngine initialization
- `pkg/gateway/processing/crd_creator.go` - Pass-through raw values
- `pkg/gateway/processing/crd_updater.go` - Remove classification updates
- `pkg/gateway/config/config.go` - Remove classification config
- `pkg/gateway/metrics/metrics.go` - Remove classification metrics
- `test/integration/gateway/k8s_api_integration_test.go`
- `test/integration/gateway/k8s_api_failure_test.go`
- `test/integration/gateway/prometheus_adapter_integration_test.go`
- `test/integration/gateway/helpers.go`
- `test/e2e/gateway/09_signal_validation_test.go`

---

### **Step 1: Copy Gateway Categorization Code to Signal Processing**

**Production Code to Copy** (~487 lines):

| Source (Gateway) | Target (Signal Processing) | Lines |
|------------------|----------------------------|-------|
| `pkg/gateway/processing/classification.go` | `pkg/signalprocessing/classifier/environment.go` | 267 |
| `pkg/gateway/processing/priority.go` | `pkg/signalprocessing/classifier/priority.go` | 222 |

**Rego Policy to Copy**:

| Source | Target | Lines |
|--------|--------|-------|
| `config.app/gateway/policies/priority.rego` | `config.app/signalprocessing/policies/priority.rego` | 74 |

**Refactoring Notes**:
- Change package from `processing` to `classifier`
- Update Rego package path: `kubernaut.gateway.priority` → `kubernaut.signalprocessing.priority`
- Replace `types.NormalizedSignal` with `signalprocessingv1alpha1.SignalData`
- Add metrics integration (use Signal Processing metrics)

---

### **Step 2: Copy Tests (Adapt Package Paths)**

**Unit Tests to Copy** (~860 lines, 34 test cases):

| Source | Target | Tests |
|--------|--------|-------|
| `test/unit/gateway/processing/environment_classification_test.go` | `test/unit/signalprocessing/environment_classifier_test.go` | 15 |
| `test/unit/gateway/priority_classification_test.go` | `test/unit/signalprocessing/priority_engine_test.go` | 19 |

**Test Adaptations Needed**:
```go
// Change imports
- "github.com/jordigilh/kubernaut/pkg/gateway/processing"
- "github.com/jordigilh/kubernaut/pkg/gateway/types"
+ "github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
+ signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"

// Change type references
- signal := &types.NormalizedSignal{Namespace: "test-ns"}
+ signal := &signalprocessingv1alpha1.SignalData{Namespace: "test-ns"}

// Change struct references
- classifier = processing.NewEnvironmentClassifier(k8sClient, logger)
+ classifier = classifier.NewEnvironmentClassifier(k8sClient, logger)
```

**Integration Tests Affected** (in `test/integration/gateway/`):
- Files referencing `EnvironmentClassifier` or `PriorityEngine`:
  - `adapter_interaction_test.go`
  - `k8s_api_integration_test.go`
  - `k8s_api_interaction_test.go`
  - `observability_test.go`
  - `prometheus_adapter_integration_test.go`

**E2E Tests Affected**:
- `test/e2e/gateway/09_signal_validation_test.go`

---

### **Step 3: Remove from Gateway (After Signal Processing is Working)**

**Files to DELETE from Gateway**:
```bash
# Production code
rm pkg/gateway/processing/classification.go
rm pkg/gateway/processing/priority.go

# Tests (after migrating)
rm test/unit/gateway/processing/environment_classification_test.go
rm test/unit/gateway/priority_classification_test.go

# Note: Rego policy can stay in gateway/policies/ if used as shared config
# OR move to signalprocessing/policies/ if Signal Processing specific
```

**Files to MODIFY in Gateway**:
1. `pkg/gateway/server.go` - Remove classifier initialization and usage
2. `pkg/gateway/processing/crd_creator.go` - Pass-through signal values:
```go
// BEFORE (current - Gateway classifies):
environment := g.classifier.Classify(ctx, signal.Namespace)
priority := g.priorityEngine.Assign(ctx, signal.Severity, environment)

// AFTER (pass-through - Signal Processing classifies):
// Gateway does NOT classify - it passes raw signal data
// Signal Processing will classify after K8s enrichment
```

**Gateway Tests to Update**:
- Update integration tests to expect raw values (not classified)
- Remove classification-specific test assertions

---

### **Step 4: Update Documentation**

**Business Requirements to DEPRECATE** (in `docs/requirements/`):
- BR-GATEWAY-051: Environment Classification (Primary) → **DEPRECATED, moved to BR-SP-051**
- BR-GATEWAY-052: Environment Classification (Fallback) → **DEPRECATED, moved to BR-SP-052**
- BR-GATEWAY-053: Environment Classification (Default) → **DEPRECATED, moved to BR-SP-053**
- BR-GATEWAY-020: Priority Assignment (Rego) → **DEPRECATED, moved to BR-SP-070**
- BR-GATEWAY-021: Priority Fallback → **DEPRECATED, moved to BR-SP-071**

**Reference**: DD-CATEGORIZATION-001 for migration rationale

---

### **Verification Checklist**

**Before Removing from Gateway**:
- [ ] Signal Processing compiles with moved code
- [ ] All 34 unit tests pass in Signal Processing location
- [ ] Rego policy loads correctly in Signal Processing
- [ ] Signal Processing reconciler uses new classifiers
- [ ] Integration test: SignalProcessing CRD → Classified result

**After Removing from Gateway**:
- [ ] Gateway compiles without classification code
- [ ] Gateway unit tests pass (classification tests removed)
- [ ] Gateway integration tests pass (updated expectations)
- [ ] E2E test: Gateway → RemediationOrchestrator → SignalProcessing → Classified result

**Final Verification**:
- [ ] No duplicate classification code in codebase
- [ ] BR-GATEWAY-05x marked as DEPRECATED with migration reference
- [ ] DD-CATEGORIZATION-001 updated with completion status

---

#### 🔄 **Rollback Plan** (Day 12 EOD) ⭐ v1.12 NEW

**Purpose**: Document rollback procedures for Signal Processing deployment.

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/ROLLBACK_PLAN.md`

##### **Rollback Triggers**

| Trigger | Threshold | Action |
|---------|-----------|--------|
| Error rate spike | >5% of reconciliations failing | Initiate rollback |
| Latency degradation | P95 >10s (2x target) | Investigate, consider rollback |
| CRD stuck in phase | >50% stuck in enriching/categorizing | Immediate rollback |
| Gateway tests failing | Any regression after migration | Rollback Gateway changes |

##### **Rollback Procedure**

**Step 1: Verify Rollback Need** (5 min)
- [ ] Confirm issue is related to Signal Processing deployment
- [ ] Check `signalprocessing_reconciliation_total{result="error"}` metrics
- [ ] Review controller logs for errors
- [ ] Notify on-call team

**Step 2: Execute Rollback** (10 min)

```bash
# Option A: Rollback Signal Processing controller
kubectl rollout undo deployment/signalprocessing-controller -n kubernaut-system

# Option B: Rollback to specific revision
kubectl rollout undo deployment/signalprocessing-controller -n kubernaut-system --to-revision=X

# Verify rollback
kubectl rollout status deployment/signalprocessing-controller -n kubernaut-system
```

**Step 3: Gateway Rollback (if migration was applied)**

```bash
# If Gateway changes were deployed, rollback Gateway too
kubectl rollout undo deployment/gateway -n kubernaut-system

# Verify Gateway is using old classification code
kubectl logs deployment/gateway -n kubernaut-system | grep "Classification"
```

**Step 4: Verify Rollback Success** (15 min)
- [ ] Pods running previous version
- [ ] Error rate returned to baseline
- [ ] CRDs processing successfully
- [ ] Gateway classification working (if rolled back)

##### **Rollback Safeguards**

| Safeguard | Implementation |
|-----------|----------------|
| **Feature flag** | `ENABLE_SIGNAL_PROCESSING_CATEGORIZATION=false` disables new categorization |
| **Canary deployment** | Deploy to 10% of cluster first, monitor for 30 min |
| **CRD version** | `v1alpha1` allows schema changes without migration |
| **Dual-write period** | Keep Gateway classification active for 7 days post-migration |

---

#### 🔧 **Critical Issues Resolved** (Fill During Implementation) ⭐ v1.12 NEW

**Purpose**: Document critical issues encountered during implementation.

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/CRITICAL_ISSUES_RESOLVED.md`

##### **Issue Summary** (Fill during implementation)

| Issue # | Title | Severity | Resolution Time | Status |
|---------|-------|----------|-----------------|--------|
| 1 | [TBD] | Critical/High | Xh | ⬜ |
| 2 | [TBD] | Critical/High | Xh | ⬜ |

##### **Issue Template**

```markdown
### **Issue #N: [Issue Title]**

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
```

---

#### 📋 **Pre-Day Validation Checklists** ⭐ v1.12 NEW

##### **Pre-Day 7 Validation** (Before Integration Phase)

| Category | Items | Status |
|----------|-------|--------|
| **Core Logic Complete** | Enricher, Environment Classifier, Priority Engine, Business Classifier | ⬜ |
| **Unit Tests Written** | Tests exist for all 4 core components | ⬜ |
| **Tests Passing** | All unit tests pass (0 failures) | ⬜ |
| **Error Handling** | Error categories A-E implemented | ⬜ |
| **DD-4 Decisions** | All pre-implementation decisions resolved | ⬜ |

**Ready for Day 7**: ⬜ YES

##### **Pre-Day 10 Validation** (Before E2E Testing)

| Category | Items | Status |
|----------|-------|--------|
| **Integration Tests** | 50-80 integration tests written | ⬜ |
| **Integration Passing** | All integration tests pass | ⬜ |
| **BR Coverage** | All 12 BRs have at least 1 test | ⬜ |
| **Metrics Working** | Prometheus endpoint returns metrics | ⬜ |
| **Audit Working** | Audit events created in Data Storage | ⬜ |

**Ready for Day 10**: ⬜ YES

##### **Pre-Day 12 Validation** (Before Production Readiness)

| Category | Items | Status |
|----------|-------|--------|
| **E2E Tests** | 5-10 E2E tests written | ⬜ |
| **E2E Passing** | All E2E tests pass | ⬜ |
| **Documentation** | All docs updated | ⬜ |
| **Production Readiness** | Checklist 90%+ complete | ⬜ |
| **Gateway Migration Plan** | Detailed migration steps documented | ⬜ |

**Ready for Day 12**: ⬜ YES

---

## 📊 **Business Requirements Coverage Matrix**

| BR ID | Description | Test File | Test Type | Coverage |
|-------|-------------|-----------|-----------|----------|
| **BR-SP-001** | K8s Context Enrichment | `enricher_test.go` | Unit + Integration | ✅ |
| **BR-SP-002** | Business Classification | `business_classifier_test.go` | Unit | ✅ |
| **BR-SP-003** | Recovery Context Integration | `reconciler_test.go` | Integration | ✅ |
| **BR-SP-051** | Environment Classification (Primary) | `environment_classifier_test.go` | Unit | ✅ |
| **BR-SP-052** | Environment Classification (Fallback) | `environment_classifier_test.go` | Unit | ✅ |
| **BR-SP-053** | Environment Classification (Default) | `environment_classifier_test.go` | Unit | ✅ |
| **BR-SP-070** | Priority Assignment (Rego) | `priority_engine_test.go` | Unit | ✅ |
| **BR-SP-071** | Priority Fallback Matrix | `priority_engine_test.go` | Unit | ✅ |
| **BR-SP-072** | Rego Hot-Reload | `hot_reloader_test.go` | Integration | ✅ |
| **BR-SP-080** | Confidence Scoring | All classifier tests | Unit | ✅ |
| **BR-SP-081** | Multi-dimensional Categorization | `business_classifier_test.go` | Unit | ✅ |
| **BR-SP-090** | Categorization Audit Trail | `audit_client_test.go` | Unit + Integration | ✅ |
| **BR-SP-100** | OwnerChain Traversal | `ownerchain_builder_test.go` | Unit + Integration | ✅ |
| **BR-SP-101** | DetectedLabels Auto-Detection | `label_detector_test.go` | Unit + Integration | ✅ |
| **BR-SP-102** | CustomLabels Rego Extraction | `rego_engine_test.go` | Unit + Integration | ✅ |
| **BR-SP-103** | FailedDetections Tracking | `label_detector_test.go` | Unit | ✅ |
| **BR-SP-104** | Mandatory Label Protection | `rego_security_wrapper_test.go` | Unit | ✅ |

**Coverage**: 17/17 BRs (100%) - All business requirements implemented

---

## 🧪 **Rego Policy Testing Strategy**

> **Context**: Rego policy integration testing is unique to services that use OPA for classification (SignalProcessing, AIAnalysis). This section documents the dedicated testing approach.

### **Why Dedicated Rego Testing?**

Unlike typical unit tests that mock the Rego engine, **integration tests must validate the full policy lifecycle**:

1. **ConfigMap Loading**: K8s ConfigMap → policy string extraction
2. **Policy Compilation**: OPA `rego.New()` → `PreparedEvalQuery`
3. **Policy Evaluation**: Input data → Rego evaluation → structured output
4. **Hot-Reload**: ConfigMap update → policy recompilation without restart
5. **Graceful Degradation**: Invalid policy → fallback to default behavior

### **Test File: `rego_integration_test.go`**

| Test Scenario | BR Coverage | Description |
|---------------|-------------|-------------|
| **ConfigMap → Policy Load** | BR-SP-070 | Create ConfigMap, verify policy loads correctly |
| **Hot-Reload Under Load** | BR-SP-072 | Update ConfigMap during active reconciliation, verify no race |
| **Invalid Policy Fallback** | BR-SP-071 | Invalid Rego syntax → fallback matrix used |
| **Policy Version Tracking** | BR-SP-090 | Audit trail includes policy version hash |

### **Integration Test Pattern**

```go
var _ = Describe("Rego Policy Integration", func() {
    var (
        ctx       context.Context
        k8sClient client.Client
        configMap *corev1.ConfigMap
        engine    *PriorityEngine
    )

    BeforeEach(func() {
        ctx = context.Background()
        // Use ENVTEST k8sClient (real K8s API, not mocked)
        k8sClient = envTestClient

        // Create ConfigMap with valid Rego policy
        configMap = &corev1.ConfigMap{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "signalprocessing-rego-policies",
                Namespace: "kubernaut-system",
            },
            Data: map[string]string{
                "priority.rego": validPriorityPolicy,
            },
        }
        Expect(k8sClient.Create(ctx, configMap)).To(Succeed())
    })

    AfterEach(func() {
        Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
    })

    // Test 1: ConfigMap → Policy Load (BR-SP-070)
    It("should load policy from ConfigMap", func() {
        engine, err := NewPriorityEngineFromConfigMap(ctx, k8sClient, "kubernaut-system")
        Expect(err).ToNot(HaveOccurred())
        Expect(engine.IsReady()).To(BeTrue())
    })

    // Test 2: Hot-Reload Under Load (BR-SP-072)
    It("should hot-reload policy without race condition", func() {
        engine, _ := NewPriorityEngineFromConfigMap(ctx, k8sClient, "kubernaut-system")
        engine.StartHotReload(ctx, k8sClient)

        // Simulate concurrent evaluation
        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
            wg.Add(1)
            go func() {
                defer wg.Done()
                _, err := engine.Assign(ctx, testInput)
                Expect(err).ToNot(HaveOccurred())
            }()
        }

        // Update ConfigMap mid-evaluation
        configMap.Data["priority.rego"] = updatedPriorityPolicy
        Expect(k8sClient.Update(ctx, configMap)).To(Succeed())

        wg.Wait()
        // No panic, no race = success
    })

    // Test 3: Invalid Policy Fallback (BR-SP-071)
    It("should fallback to matrix when policy is invalid", func() {
        configMap.Data["priority.rego"] = "invalid { rego syntax"
        Expect(k8sClient.Update(ctx, configMap)).To(Succeed())

        engine, err := NewPriorityEngineFromConfigMap(ctx, k8sClient, "kubernaut-system")
        Expect(err).To(HaveOccurred()) // Policy compilation fails

        // Engine should use fallback
        priority := engine.AssignWithFallback(ctx, testInput)
        Expect(priority).To(Equal(PriorityFromMatrix(testInput.Severity, testInput.Environment)))
    })

    // Test 4: Policy Version in Audit (BR-SP-090)
    It("should include policy version in audit trail", func() {
        engine, _ := NewPriorityEngineFromConfigMap(ctx, k8sClient, "kubernaut-system")
        result, _ := engine.Assign(ctx, testInput)

        Expect(result.PolicyVersion).ToNot(BeEmpty())
        Expect(result.PolicyVersion).To(HavePrefix("sha256:"))
    })
})
```

### **Risk #5 Mitigation: Hot-Reload Race Condition**

The hot-reload test specifically validates **Risk #5** from the Risk Mitigation table:

| Risk | Mitigation | Test Validation |
|------|------------|-----------------|
| ConfigMap hot-reload race condition | Mutex protection on policy reload, version tracking | Test 2: 10 concurrent evaluations + ConfigMap update |

### **Test Infrastructure Requirements**

| Requirement | Implementation |
|-------------|----------------|
| **ENVTEST** | Real K8s API for ConfigMap CRUD |
| **Mutex** | `sync.RWMutex` in `PriorityEngine` |
| **Version Hash** | SHA256 of policy content |
| **Fallback Matrix** | Hardcoded severity × environment → priority |

### **Day 9-10 Focus**

These integration tests are scheduled for **Days 9-10 (Testing phase)**:

- [ ] Create `test/integration/signalprocessing/rego_integration_test.go`
- [ ] Implement 4 test scenarios above
- [ ] Verify Risk #5 mitigation with concurrent access test
- [ ] Add policy version hash to audit trail

---

## 🔧 **Key Files and Locations**

### **Production Code**

| File | Purpose |
|------|---------|
| `cmd/signalprocessing/main.go` | Main entry point |
| `pkg/signalprocessing/config/config.go` | Configuration |
| `pkg/signalprocessing/metrics/metrics.go` | Prometheus metrics |
| `pkg/signalprocessing/enricher/k8s_enricher.go` | K8s context enrichment |
| `pkg/signalprocessing/classifier/environment.go` | Environment classification |
| `pkg/signalprocessing/classifier/priority.go` | Priority assignment |
| `pkg/signalprocessing/classifier/business.go` | Business classification |
| `pkg/signalprocessing/audit/client.go` | Audit client |
| `internal/controller/signalprocessing/reconciler.go` | CRD controller |
| `api/signalprocessing/v1alpha1/signalprocessing_types.go` | CRD types |

### **Rego Policies**

| File | Purpose |
|------|---------|
| `deploy/signalprocessing/policies/environment.rego` | Environment classification |
| `deploy/signalprocessing/policies/priority.rego` | Priority assignment |
| `deploy/signalprocessing/policies/business.rego` | Business classification |

### **Tests**

| Directory | Type | Current | Target |
|-----------|------|---------|--------|
| `test/unit/signalprocessing/` | Unit | **184** ✅ | 250-300 |
| `test/integration/signalprocessing/reconciler_integration_test.go` | Reconciler Integration | 0 | ~25 |
| `test/integration/signalprocessing/component_integration_test.go` | Component Integration | 0 | ~20 |
| `test/integration/signalprocessing/rego_integration_test.go` | Rego Policy Integration | 0 | ~15 |
| `test/integration/signalprocessing/hot_reloader_test.go` | Hot-Reload Integration | 0 | ~5 |
| `test/e2e/signalprocessing/` | E2E | 0 | 5-10 |

**Integration Test Breakdown** (Target: 50-80 tests):
- Reconciler phase transitions + status updates: ~25 tests
- Component integration (Enricher, Classifier, Priority): ~20 tests
- Rego policy evaluation + ConfigMap: ~15 tests
- Hot-reload + concurrent access: ~5 tests

---

## ✅ **Production Readiness Checklist**

### **Code Quality**
- [ ] Zero lint errors (`golangci-lint run`)
- [ ] Zero compilation errors
- [ ] 70%+ unit test coverage
- [ ] 50%+ integration test coverage
- [ ] All BRs covered by tests

### **CRD Controller**
- [ ] Reconciliation loop handles all phases
- [ ] Status updates work correctly
- [ ] Finalizer implemented for cleanup
- [ ] RBAC rules complete

### **Observability**
- [ ] Prometheus metrics exposed (DD-005 compliant)
- [ ] Structured logging (Zap)
- [ ] Health checks (/healthz, /readyz)
- [ ] Audit trail to Data Storage Service

### **Configuration**
- [ ] ConfigMap for Rego policies
- [ ] Environment variable overrides
- [ ] Validation for all required fields
- [ ] Hot-reload for Rego policies

### **Gateway Migration**
- [ ] Classification code removed from Gateway
- [ ] Gateway tests updated
- [ ] BRs marked as DEPRECATED
- [ ] E2E validation complete

---

## 📈 **Confidence Assessment**

### **Calculation**

```
Confidence = (Implementation Accuracy × 0.30) +
             (Test Coverage × 0.25) +
             (BR Coverage × 0.20) +
             (Production Readiness × 0.15) +
             (Documentation Quality × 0.10)
```

### **Target**

| Component | Target | Weight |
|-----------|--------|--------|
| Implementation Accuracy | 95% | 30% |
| Test Coverage | 70% | 25% |
| BR Coverage | 100% | 20% |
| Production Readiness | 90% | 15% |
| Documentation Quality | 90% | 10% |

**Overall Target Confidence**: 90%+

---

## ⚠️ **Common Pitfalls (Signal Processing Specific)**

### ❌ **Don't Do This**

| Anti-Pattern | Impact | Correct Approach |
|--------------|--------|------------------|
| **Run tests sequentially** | Slow feedback (10x slower) | Parallel execution: 4 concurrent procs |
| **Test CRD struct fields exist** | Null-testing, zero value | Test controller behavior instead |
| **Create `types_test.go` for API types** | Useless tests | Controller tests validate behavior |
| **Use `http.send` in Rego policies** | Security + performance issues | K8s Enricher fetches data first |
| **Configure enrichment depth** | SRE nightmare | Standard depth (DD-017) |
| **Direct database access for audits** | ADR-032 violation | Data Storage Service REST API |
| **Use "Alert" terminology** | ADR-015 violation | Use "Signal" everywhere |
| **Copy existing `pkg/signalprocessing/`** | Legacy code risk | Build fresh with DD-006 |
| **Hardcode Rego policies in code** | No hot-reload | ConfigMap-based policies |
| **Skip BR coverage matrix** | Untested requirements | Map all tests to BR-SP-XXX |

### ✅ **Do This Instead**

| Best Practice | Benefit | Implementation |
|---------------|---------|----------------|
| **Parallel test execution** | 4x faster feedback | `ginkgo -p -procs=4` or `go test -p 4` |
| **Table-driven tests** | 38% code reduction | `DescribeTable` with `Entry` patterns |
| **Business outcome tests** | Real validation | Test controller reconciliation results |
| **Standard enrichment depth** | Predictable, simple | Pod→Ns+Pod+Node+Owner (DD-017) |
| **Rego ConfigMap hot-reload** | Policy updates without restart | fsnotify-based policy watcher |
| **Daily status docs** | Clear handoffs | Days 1, 4, 7, 12 EOD templates |
| **BR coverage matrix** | Complete validation | Map all 12 BRs to specific tests |
| **envtest for integration** | Fast, reliable | No KIND/Podman complexity |

### ⚠️ **Signal Processing Specific Gotchas**

1. **Remediation Orchestrator Creates SignalProcessing CRD**
   - Gateway does NOT create SignalProcessing directly
   - Gateway creates RemediationRequest → RO creates SignalProcessing
   - Correct RBAC: watch `kubernaut.io` group for `remediationrequests`

2. **K8s Enricher vs Rego Policy Engine**
   - K8s Enricher: Fetches raw K8s data (ADR-041)
   - Rego Engine: Evaluates classification policies with pre-fetched data
   - NO `http.send` in Rego (security, performance, testing)

3. **Signal-Driven Enrichment (DD-017)**
   - Pod signals: Fetch Namespace + Pod + Node + Owner
   - Deployment signals: Fetch Namespace + Deployment
   - Node signals: Fetch Node only
   - NO configurable depth (avoid complexity)

4. **Unified CRD API Group**
   - All CRDs use `kubernaut.io/v1alpha1`
   - NOT `signalprocessing.kubernaut.io` (legacy pattern)
   - All RBAC rules must reference `kubernaut.io` group

---

## 📚 **Production Runbooks**

> **IMPORTANT**: Create these runbooks in `docs/services/crd-controllers/01-signalprocessing/runbooks/` during Day 12.

### **Runbook 1: High Classification Failure Rate** (>10%)

**Symptoms**:
- `signalprocessing_reconciliation_total{result="failed"}` spike
- Multiple `SignalProcessing` CRDs stuck in `Failed` phase
- Increased error logs with `phase=classifying`

**Investigation Steps**:
```bash
# 1. Check failed SignalProcessing CRDs
kubectl get signalprocessings -A -o json | jq '.items[] | select(.status.phase=="Failed") | {name: .metadata.name, error: .status.conditions[-1].message}'

# 2. Check Rego policy ConfigMap
kubectl get configmap signalprocessing-rego-policies -n kubernaut-system -o yaml

# 3. Check controller logs for policy errors
kubectl logs -l app=signalprocessing-controller -n kubernaut-system --since=10m | grep -i "rego\|policy\|classify"

# 4. Check K8s API availability
kubectl get --raw /healthz
```

**Resolution**:
1. If Rego policy syntax error → Fix ConfigMap, controller will hot-reload
2. If K8s API unavailable → Wait for API recovery (transient)
3. If RBAC issue → Update ClusterRole with missing permissions

**Escalation**: If >20% failure rate for >10 minutes

---

### **Runbook 2: Stuck Signal Processing** (>5min in Enriching/Classifying)

**Symptoms**:
- `signalprocessing_phase_duration_seconds{phase="enriching"}` histogram shows >5m
- SignalProcessing CRDs not transitioning to `Completed`
- Controller reconcile queue growing

**Investigation Steps**:
```bash
# 1. Find stuck CRDs
kubectl get signalprocessings -A -o json | jq '.items[] | select(.status.phase=="Enriching" or .status.phase=="Classifying") | {name: .metadata.name, phase: .status.phase, since: .status.conditions[-1].lastTransitionTime}'

# 2. Check controller reconcile rate
curl -s localhost:9090/metrics | grep signalprocessing_reconciliation_duration

# 3. Check K8s API latency
kubectl get --raw /readyz?verbose

# 4. Check controller memory/CPU
kubectl top pod -l app=signalprocessing-controller -n kubernaut-system
```

**Resolution**:
1. If K8s API slow → Check API server health, consider rate limit increase
2. If controller OOM → Increase memory limits
3. If ConfigMap mount missing → Verify Rego policy ConfigMap mount

**Escalation**: If >10 stuck for >5 minutes

---

### **Runbook 3: Rego Policy Errors**

**Symptoms**:
- Events with `RegoPolicyError` on SignalProcessing CRDs
- `signalprocessing_rego_policy_errors_total` increasing
- Classification results show `environment=unknown`, `priority=unknown`

**Investigation Steps**:
```bash
# 1. Check Rego policy ConfigMap syntax
kubectl get configmap signalprocessing-rego-policies -n kubernaut-system -o jsonpath='{.data.environment\.rego}' | opa check -

# 2. Validate policy with test input
echo '{"namespace_labels": {"environment": "production"}}' | opa eval -d /tmp/env.rego -I 'data.signalprocessing.classify_environment'

# 3. Check controller policy reload logs
kubectl logs -l app=signalprocessing-controller -n kubernaut-system | grep -i "policy.*reload\|configmap.*watch"
```

**Resolution**:
1. Fix Rego syntax in ConfigMap
2. Test policy locally with OPA before applying
3. Restart controller if hot-reload fails (rare)

---

## 🎯 **Edge Case Categories**

> **Apply to**: Days 9-10 (Testing phase)

### **Category 1: Rego Policy Hot-Reload During Evaluation**
- **Scenario**: Policy ConfigMap updated while classification in progress
- **Expected**: Current evaluation completes with old policy, next uses new
- **Test**: Start long classification, update ConfigMap, verify next uses new policy

### **Category 2: K8s API Rate Limiting**
- **Scenario**: K8s Enricher hits API rate limits during high signal volume
- **Expected**: Exponential backoff, eventual success
- **Test**: Mock 429 responses, verify backoff behavior

### **Category 3: Large Enrichment Payloads**
- **Scenario**: Deployment with 100+ pods causes large K8s context
- **Expected**: Enricher completes within timeout, no OOM
- **Test**: Create deployment with high replica count, verify enrichment

### **Category 4: Concurrent Classification Attempts**
- **Scenario**: Same SignalProcessing reconciled by two controller replicas
- **Expected**: Optimistic locking prevents duplicate work
- **Test**: Simulate concurrent status updates, verify conflict handling

### **Category 5: Partial K8s API Response**
- **Scenario**: Namespace exists but Pod deleted during enrichment
- **Expected**: Graceful degradation with partial context (confidence 0.5)
- **Test**: Delete Pod mid-enrichment, verify partial result

---

## 📊 **Metrics Validation Commands**

```bash
# Start controller locally (for validation)
go run ./cmd/signalprocessing/main.go \
    --metrics-bind-address=:9090 \
    --health-probe-bind-address=:8081

# Verify metrics endpoint
curl -s localhost:9090/metrics | grep signalprocessing_

# Expected metrics:
# signalprocessing_reconciliation_total{result="success",phase="completed"} 0
# signalprocessing_reconciliation_duration_seconds_bucket{phase="enriching",le="1"} 0
# signalprocessing_k8s_api_errors_total{error_type="timeout"} 0
# signalprocessing_rego_evaluation_duration_seconds_bucket{policy="environment",le="0.1"} 0
# signalprocessing_categorization_confidence{type="environment"} 0
# signalprocessing_audit_write_total{status="success"} 0

# Verify health endpoints
curl -s localhost:8081/healthz  # Should return 200
curl -s localhost:8081/readyz   # Should return 200

# Create test SignalProcessing CRD
kubectl apply -f config/samples/signalprocessing_v1alpha1_signalprocessing.yaml

# Verify reconciliation metric increments
watch -n 1 'curl -s localhost:9090/metrics | grep signalprocessing_reconciliation_total'
```

---

## 🚧 **Blockers Section**

> **Status**: Updated during implementation. Track any blocking issues here.

| ID | Description | Status | Owner | Resolution Date |
|----|-------------|--------|-------|-----------------|
| _None at start_ | | | | |

**Blocker Template**:
```markdown
| B-001 | [Description] | 🔴 Blocked / 🟡 In Progress / ✅ Resolved | [Name] | [Date] |
```

---

## 📝 **Lessons Learned** (Fill During Implementation)

> **Update after each phase completion**

### **What Worked Well**
1. _[To be filled during Day 4 review]_
2. _[To be filled during Day 7 review]_
3. _[To be filled during Day 12 review]_

### **Technical Wins**
1. _[To be filled]_
2. _[To be filled]_

### **Challenges Overcome**
1. _[To be filled]_
2. _[To be filled]_

---

## 🔧 **Technical Debt** (Fill During Implementation)

> **Track items to address post-V1**

### **Minor Issues (Non-Blocking)**
1. _[To be identified during implementation]_

### **Future Enhancements (Post-V1)**
1. Configurable Rego policy location (currently hardcoded ConfigMap name)
2. Multi-tenant policy support (namespace-scoped rules)
3. Classification audit dashboard (Grafana)
4. Rego policy versioning and rollback

---

## 🤝 **Team Handoff Notes** (Fill Day 12)

### **Key Files to Review**
1. `api/kubernaut.io/v1alpha1/signalprocessing_types.go` - CRD definition
2. `internal/controller/signalprocessing/reconciler.go` - Main reconciliation logic
3. `pkg/signalprocessing/enricher/k8s_enricher.go` - K8s context enrichment
4. `pkg/signalprocessing/classifier/*.go` - Rego-based classifiers
5. `pkg/signalprocessing/rego/engine.go` - OPA policy engine
6. `docs/.../ERROR_HANDLING_PHILOSOPHY.md` - Error handling guide

### **Running Locally**
```bash
# Terminal 1: Start KIND cluster with CRDs
make kind-create
make install

# Terminal 2: Start controller
make run-signalprocessing

# Terminal 3: Create test resources
kubectl apply -f config/samples/signalprocessing_v1alpha1_signalprocessing.yaml
kubectl get signalprocessings -w

# Terminal 4: Watch logs
kubectl logs -f -l app=signalprocessing-controller -n kubernaut-system
```

### **Debugging Tips**
```bash
# Watch SignalProcessing CRD status changes
kubectl get signalprocessings -A -w -o custom-columns=NAME:.metadata.name,PHASE:.status.phase,ENV:.status.environmentClassification.environment,PRIORITY:.status.priorityAssignment.priority

# Check Rego policy loaded correctly
kubectl logs -l app=signalprocessing-controller -n kubernaut-system | grep -i "policy.*loaded\|rego.*ready"

# Force re-reconciliation
kubectl annotate signalprocessing <name> force-reconcile=$(date +%s) --overwrite

# Check controller leader election status
kubectl get lease signalprocessing-controller-leader -n kubernaut-system -o yaml
```

---

## ✅ **Success Criteria**

### **Completion Checklist**

#### **Functional Validation**
- [ ] All 12 BRs covered by tests (BR-SP-001 through BR-SP-012)
- [ ] K8s Enricher correctly enriches Pod, Deployment, and Node signals
- [ ] Rego policy engine evaluates environment, priority, and business classification
- [ ] Controller correctly updates SignalProcessing CRD status
- [ ] ConfigMap hot-reload works for Rego policy updates
- [ ] All phase transitions validated (Pending → Enriching → Classifying → Completed)

#### **Quality Validation**
- [ ] Unit test coverage ≥70%
- [ ] Integration test coverage ≥50%
- [ ] BR coverage = 100% (12/12 BRs)
- [ ] No linter errors (`golangci-lint run ./...`)
- [ ] All tests pass (`go test ./...`)
- [ ] Confidence assessment ≥90%

#### **Operational Validation**
- [ ] Metrics exposed and validated (6 core metrics)
- [ ] Health/ready endpoints functional
- [ ] Structured logging with correlation IDs
- [ ] Error handling with proper requeue strategy
- [ ] RBAC rules correctly scoped

#### **Documentation Validation**
- [ ] EOD templates completed (Days 1, 4, 7, 12)
- [ ] Production readiness report generated
- [ ] Handoff summary completed
- [ ] All design decisions documented and cross-referenced

### **Definition of Done**

| Criteria | Target | Evidence |
|----------|--------|----------|
| BR Coverage | 100% | BR coverage matrix with test links |
| Unit Tests | 70%+ | `go tool cover` output |
| Integration Tests | 50%+ | envtest results |
| Build Success | 100% | `go build ./...` passes |
| Lint Clean | 100% | `golangci-lint run ./...` passes |
| Confidence | 90%+ | Evidence-based assessment |

---

## 🔧 **Makefile Targets**

```makefile
# Signal Processing Service Development Targets
# Standard: 4 concurrent processes for all test tiers

# Build
.PHONY: build-signalprocessing
build-signalprocessing:
	go build -o bin/signalprocessing ./cmd/signalprocessing

# Unit tests (parallel: 4 procs)
.PHONY: test-unit-signalprocessing
test-unit-signalprocessing:
	go test -v -p 4 ./test/unit/signalprocessing/...

# Integration tests (parallel: 4 procs)
.PHONY: test-integration-signalprocessing
test-integration-signalprocessing:
	go test -v -p 4 ./test/integration/signalprocessing/...

# E2E tests (parallel: 4 procs)
.PHONY: test-e2e-signalprocessing
test-e2e-signalprocessing:
	go test -v -p 4 ./test/e2e/signalprocessing/... --tags=e2e

# Ginkgo targets (preferred - parallel with 4 procs)
.PHONY: test-unit-ginkgo-signalprocessing
test-unit-ginkgo-signalprocessing:
	ginkgo -p -procs=4 -v ./test/unit/signalprocessing/...

.PHONY: test-integration-ginkgo-signalprocessing
test-integration-ginkgo-signalprocessing:
	ginkgo -p -procs=4 -v ./test/integration/signalprocessing/...

.PHONY: test-e2e-ginkgo-signalprocessing
test-e2e-ginkgo-signalprocessing:
	ginkgo -p -procs=4 -v ./test/e2e/signalprocessing/...

# All tests with parallel execution
.PHONY: test-all-signalprocessing
test-all-signalprocessing:
	ginkgo -p -procs=4 -v ./test/unit/signalprocessing/... ./test/integration/signalprocessing/... ./test/e2e/signalprocessing/...

# Coverage (parallel: 4 procs)
.PHONY: coverage-signalprocessing
coverage-signalprocessing:
	go test -v -p 4 -coverprofile=coverage-signalprocessing.out ./test/unit/signalprocessing/...
	go tool cover -html=coverage-signalprocessing.out -o coverage-signalprocessing.html
	@echo "Coverage report: coverage-signalprocessing.html"

# Lint
.PHONY: lint-signalprocessing
lint-signalprocessing:
	golangci-lint run ./pkg/signalprocessing/... ./cmd/signalprocessing/... ./internal/controller/signalprocessing/...

# CRD Generation
.PHONY: generate-signalprocessing-crd
generate-signalprocessing-crd:
	controller-gen crd:trivialVersions=true paths="./api/signalprocessing/..." output:crd:artifacts:config=config/crd/bases

# All validations (parallel)
.PHONY: validate-signalprocessing
validate-signalprocessing: lint-signalprocessing test-unit-signalprocessing test-integration-signalprocessing
	@echo "✅ All validations passed for Signal Processing"

# Development cycle (quick)
.PHONY: dev-signalprocessing
dev-signalprocessing: build-signalprocessing lint-signalprocessing test-signalprocessing
	@echo "✅ Dev cycle complete"

# Full validation (before commit)
.PHONY: precommit-signalprocessing
precommit-signalprocessing: validate-signalprocessing coverage-signalprocessing
	@echo "✅ Ready for commit"
```

### **Daily Workflow**

```bash
# Morning: Build and quick test
make dev-signalprocessing

# After implementation: Full validation
make validate-signalprocessing

# Before commit: Coverage and lint
make precommit-signalprocessing

# CRD changes: Regenerate
make generate-signalprocessing-crd
```

---

## 📎 **Appendix A: EOD Documentation Templates**

### **Day 1 EOD: `01-day1-complete.md`**

```markdown
# Signal Processing Service - Day 1 Complete

**Date**: [YYYY-MM-DD]
**Phase**: Foundation
**Status**: ✅ Complete

---

## Package Structure Created

- [ ] `cmd/signalprocessing/main.go` - Main entry point
- [ ] `pkg/signalprocessing/config/config.go` - Configuration
- [ ] `pkg/signalprocessing/metrics/metrics.go` - Prometheus metrics
- [ ] `api/signalprocessing/v1alpha1/signalprocessing_types.go` - CRD types

## Types and Interfaces Defined

- [ ] `SignalProcessingSpec` struct
- [ ] `SignalProcessingStatus` struct
- [ ] `KubernetesContext` struct
- [ ] `EnvironmentClassification` struct
- [ ] `PriorityAssignment` struct
- [ ] `BusinessClassification` struct

## Build Validation

```bash
go build ./cmd/signalprocessing/...
# Expected: Build successful with no errors
```

## Confidence Assessment

| Component | Status | Confidence |
|-----------|--------|------------|
| Package structure | ✅ | 95% |
| CRD types | ✅ | 90% |
| Configuration | ✅ | 85% |
| Build success | ✅ | 100% |

**Day 1 Confidence**: 92%

---

## Blockers

- None identified

## Tomorrow's Focus

- Day 2: CRD types test, DD-006 scaffolding complete
```

---

### **Day 4 EOD: `02-day4-midpoint.md`**

```markdown
# Signal Processing Service - Day 4 Midpoint

**Date**: [YYYY-MM-DD]
**Phase**: Core Logic (Midpoint)
**Status**: ✅ In Progress

---

## Components Completed (Days 1-4)

### Foundation (Days 1-2)
- [x] Package structure created
- [x] CRD types defined and tested
- [x] Configuration structure established
- [x] DD-006 scaffolding applied

### Core Logic (Days 3-4)
- [x] K8s Enricher implemented
- [x] K8s Enricher unit tests
- [x] Environment Classifier (Rego) implemented
- [x] Environment Rego policies created
- [x] Environment Classifier unit tests

## Integration Status

| Integration Point | Status | Notes |
|-------------------|--------|-------|
| K8s API (via client-go) | ✅ Working | Cache implemented |
| Rego Policy Engine | ✅ Working | Hot-reload pending |
| Data Storage Service | ⏳ Pending | Day 8 |

## Test Coverage So Far

| Component | Unit Tests | Integration Tests | Coverage |
|-----------|------------|-------------------|----------|
| K8s Enricher | 8 | 0 | 75% |
| Environment Classifier | 6 | 0 | 80% |
| **Total** | 14 | 0 | ~77% |

## Blockers

- [Blocker 1 if any]
- [Blocker 2 if any]

## Confidence Assessment

| Component | Status | Confidence |
|-----------|--------|------------|
| K8s Enricher | ✅ Complete | 90% |
| Environment Classifier | ✅ Complete | 88% |
| Integration readiness | 🟡 Partial | 75% |

**Day 4 Confidence**: 85%

---

## Days 5-6 Focus

- Day 5: Priority Engine (Rego)
- Day 6: Business Classifier + Error Handling Philosophy
```

---

### **Day 7 EOD: `03-day7-complete.md`**

```markdown
# Signal Processing Service - Day 7 Complete

**Date**: [YYYY-MM-DD]
**Phase**: Integration
**Status**: ✅ Complete

---

## Core Implementation Complete

### All Classifiers Implemented
- [x] K8s Enricher with caching
- [x] Environment Classifier (Rego)
- [x] Priority Engine (Rego + hot-reload)
- [x] Business Classifier (Rego)

### Reconciler Implemented
- [x] SignalProcessingReconciler
- [x] Phase state machine (Pending → Enriching → Categorizing → Complete)
- [x] Status updates
- [x] Error handling with exponential backoff

### Metrics Implemented (DD-005 Compliant)
- [x] `signalprocessing_reconciliations_total`
- [x] `signalprocessing_reconciliation_duration_seconds`
- [x] `signalprocessing_enrichment_duration_seconds`
- [x] `signalprocessing_classification_duration_seconds`
- [x] `signalprocessing_errors_total`

## Schema Validation (Checkpoint 2) ✅

| CRD Field | Go Struct Field | Validated |
|-----------|-----------------|-----------|
| `spec.remediationRequestRef` | `RemediationRequestRef` | ✅ |
| `spec.signal` | `Signal` | ✅ |
| `status.phase` | `Phase` | ✅ |
| `status.kubernetesContext` | `KubernetesContext` | ✅ |
| `status.environmentClassification` | `EnvironmentClassification` | ✅ |
| `status.priorityAssignment` | `PriorityAssignment` | ✅ |
| `status.businessClassification` | `BusinessClassification` | ✅ |

**Schema Validation**: 100% field alignment confirmed

## Test Infrastructure Ready

- [x] ENVTEST configured
- [x] Ginkgo/Gomega test suites created
- [x] Mock Data Storage client ready
- [x] Test Rego policies ready

## Confidence Assessment

| Component | Status | Confidence |
|-----------|--------|------------|
| All classifiers | ✅ Complete | 92% |
| Reconciler | ✅ Complete | 90% |
| Metrics | ✅ Complete | 95% |
| Schema validation | ✅ Complete | 100% |
| Test infrastructure | ✅ Ready | 85% |

**Day 7 Confidence**: 92%

---

## Days 8-10 Focus

- Day 8: Audit client, metrics integration
- Day 9: Integration tests (Checkpoint 1)
- Day 10: Unit tests, BR coverage matrix (Checkpoint 3)
```

---

## 📎 **Appendix B: Production Readiness Report Template**

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/PRODUCTION_READINESS_REPORT.md`

```markdown
# Signal Processing Service - Production Readiness Assessment

**Assessment Date**: [YYYY-MM-DD]
**Assessment Status**: ✅ Production-Ready | 🚧 Partially Ready | ❌ Not Ready
**Overall Score**: XX/119 (target 95+)

---

## 1. Functional Validation (Weight: 30%)

### 1.1 Critical Path Testing
- [ ] **Happy path** - Complete workflow from SignalProcessing CRD creation to Complete phase
  - **Test**: `test/integration/signalprocessing/workflow_test.go`
  - **Evidence**: Phases transition correctly (Pending → Enriching → Categorizing → Complete)
  - **Score**: X/10

- [ ] **Error recovery** - Transient K8s API failure with automatic retry
  - **Test**: `test/integration/signalprocessing/failure_recovery_test.go`
  - **Evidence**: Exponential backoff working, retries succeed after transient errors
  - **Score**: X/10

- [ ] **Permanent failure** - Failure after max retries
  - **Test**: `test/integration/signalprocessing/failure_recovery_test.go`
  - **Evidence**: Fails gracefully after 5 retry attempts, status.phase = Failed
  - **Score**: X/10

### 1.2 Edge Cases and Boundary Conditions
- [ ] **Missing namespace** - Handles namespace not found
  - **Test**: `test/unit/signalprocessing/enricher_test.go`
  - **Score**: X/5

- [ ] **Invalid Rego policy** - Handles policy syntax errors gracefully
  - **Test**: `test/unit/signalprocessing/classifier_test.go`
  - **Score**: X/5

### Functional Validation Score: XX/35 (Target: 32+)

---

## 2. Operational Validation (Weight: 25%)

### 2.1 Observability - Metrics
- [ ] **10+ Prometheus metrics** defined and exported
  - **File**: `pkg/signalprocessing/metrics/metrics.go`
  - **Endpoint**: `:9090/metrics`
  - **Score**: X/5

- [ ] **Metrics recorded** in all reconciliation paths
  - **Score**: X/5

### 2.2 Observability - Logging
- [ ] **Structured logging** using logr/zap throughout
  - **Score**: X/4

- [ ] **Log levels** appropriate
  - **Score**: X/3

### 2.3 Health Checks
- [ ] **Liveness probe** - `GET /healthz`
  - **Score**: X/3

- [ ] **Readiness probe** - `GET /readyz`
  - **Score**: X/3

### 2.4 Graceful Shutdown
- [ ] **Signal handling** - SIGTERM/SIGINT handled gracefully
  - **Score**: X/3

### Operational Validation Score: XX/29 (Target: 27+)

---

## 3. Security Validation (Weight: 15%)

### 3.1 RBAC Permissions
- [ ] **Minimal permissions** - Only required Kubernetes permissions
  - **File**: `config/rbac/role.yaml`
  - **Score**: X/5

- [ ] **ServiceAccount** properly configured
  - **Score**: X/3

### 3.2 Secret Management
- [ ] **No hardcoded secrets** in code
  - **Score**: X/4

### Security Validation Score: XX/15 (Target: 14+)

---

## 4. Performance Validation (Weight: 15%)

### 4.1 Latency
- [ ] **K8s Enrichment P95** < 2s
  - **Score**: X/5

- [ ] **Rego Evaluation P95** < 100ms
  - **Score**: X/5

### 4.2 Throughput
- [ ] **Reconciliation rate** meets requirements
  - **Score**: X/5

### Performance Validation Score: XX/15 (Target: 13+)

---

## 5. Deployment Validation (Weight: 15%)

### 5.1 Kubernetes Manifests
- [ ] **Deployment manifest** complete
  - **Score**: X/4

- [ ] **ConfigMap** for Rego policies
  - **Score**: X/3

- [ ] **RBAC manifests** complete
  - **Score**: X/3

### 5.2 Probes Configuration
- [ ] **Probes configured** with appropriate thresholds
  - **Score**: X/5

### Deployment Validation Score: XX/15 (Target: 14+)

---

## 6. Documentation Quality (Weight: 10% bonus)

- [ ] **Service README** comprehensive
  - **Score**: X/3

- [ ] **Design Decisions** documented
  - **Score**: X/2

- [ ] **Testing Strategy** documented
  - **Score**: X/2

- [ ] **Troubleshooting Guide** included
  - **Score**: X/3

### Documentation Score: XX/10 (Bonus)

---

## Overall Production Readiness Assessment

**Total Score**: XX/109
**With Documentation Bonus**: XX/119

**Production Readiness Level**:
- **95-100%** (113+): ✅ **Production-Ready**
- **85-94%** (101-112): 🚧 **Mostly Ready**
- **75-84%** (89-100): ⚠️ **Needs Work**
- **<75%** (<89): ❌ **Not Ready**

**Current Level**: [Status]
```

---

## 📎 **Appendix C: Handoff Summary Template**

**File**: `docs/services/crd-controllers/01-signalprocessing/implementation/00-HANDOFF-SUMMARY.md`

```markdown
# Signal Processing Service - Implementation Handoff Summary

**Service Name**: Signal Processing CRD Controller
**Implementation Dates**: [Start Date] - [End Date]
**Handoff Date**: [YYYY-MM-DD]
**Document Status**: ✅ Complete

---

## Executive Summary

**What Was Built**:
The Signal Processing CRD Controller is a Kubernetes controller that enriches incoming signals with Kubernetes context (namespace, deployment, pod, node data) and performs multi-dimensional categorization using customer-configurable Rego policies. It processes signals created by the Remediation Orchestrator and prepares them for AI analysis.

**Current Status**: ✅ Production-Ready

**Production Readiness Score**: XX/119 (XX%)

**Key Achievement**: Complete categorization ownership migrated from Gateway to Signal Processing with 100% BR coverage.

---

## Implementation Overview

### Scope Accomplished
✅ **Phase 1 (Days 1-2)**: Foundation
- Package structure created with DD-006 scaffolding
- CRD types defined (SignalProcessing, KubernetesContext, classifications)
- Configuration structure established

✅ **Phase 2 (Days 3-6)**: Core Logic
- K8s Enricher with caching (ADR-041 compliant)
- Environment Classifier (Rego policies)
- Priority Engine (Rego + hot-reload)
- Business Classifier (multi-dimensional)

✅ **Phase 3 (Days 7-8)**: Integration
- SignalProcessingReconciler (phase state machine)
- Prometheus metrics (DD-005 compliant)
- Audit client (ADR-032 compliant)

✅ **Phase 4 (Days 9-10)**: Testing
- Integration tests with ENVTEST
- Unit tests (70%+ coverage)
- BR coverage matrix (100% coverage)

✅ **Phase 5 (Days 14-15)**: Finalization
- Documentation (service docs, user guides, deployment guide)
- Gateway cleanup (remove old classification code)
- Production readiness validation

---

## Architecture Summary

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│ RemediationOrchestrator creates SignalProcessing CRD             │
└──────────────────────────┬───────────────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────────────┐
│ SignalProcessingReconciler                                        │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ K8s Enricher (Go) → Rego Policies (OPA)                  │    │
│  │                                                          │    │
│  │ 1. Fetch K8s objects (namespace, deployment, pod, node)  │    │
│  │ 2. Environment Classifier → environment.rego             │    │
│  │ 3. Priority Engine → priority.rego                       │    │
│  │ 4. Business Classifier → business.rego                   │    │
│  └──────────────────────────────────────────────────────────┘    │
│                           │                                       │
│                           ▼                                       │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ Update SignalProcessing status.phase = Complete          │    │
│  │ Write audit event to Data Storage Service                │    │
│  └──────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
```

---

## Business Requirements Coverage

### Implemented Requirements
- ✅ **BR-SP-001**: K8s Context Enrichment - Fully implemented
- ✅ **BR-SP-002**: Business Classification - Fully implemented
- ✅ **BR-SP-003**: Recovery Context Integration - Fully implemented
- ✅ **BR-SP-051**: Environment Classification (Primary) - Fully implemented
- ✅ **BR-SP-052**: Environment Classification (Fallback) - Fully implemented
- ✅ **BR-SP-053**: Environment Classification (Default) - Fully implemented
- ✅ **BR-SP-070**: Priority Assignment (Rego) - Fully implemented
- ✅ **BR-SP-071**: Priority Fallback Matrix - Fully implemented
- ✅ **BR-SP-072**: Rego Hot-Reload - Fully implemented
- ✅ **BR-SP-080**: Confidence Scoring - Fully implemented
- ✅ **BR-SP-081**: Multi-dimensional Categorization - Fully implemented
- ✅ **BR-SP-090**: Categorization Audit Trail - Fully implemented

**Total**: 12/12 business requirements implemented (100%)

---

## Key Design Decisions

### ADR-041: Rego Policy Data Fetching Separation
**Decision**: K8s Enricher (Go) fetches data, Rego policies evaluate classification
**Rationale**: Security (no raw K8s API access for customer policies), performance (caching), separation of concerns
**Impact**: All classifiers receive pre-fetched K8s context as input

### DD-CATEGORIZATION-001: Gateway/Signal Processing Split
**Decision**: Signal Processing owns ALL categorization
**Rationale**: Richer K8s context available, simplifies Gateway
**Impact**: Gateway code simplified, categorization tests moved

---

## Key Files and Locations

### Production Code
- **Main Entry Point**: `cmd/signalprocessing/main.go`
- **Reconciler**: `internal/controller/signalprocessing/reconciler.go`
- **K8s Enricher**: `pkg/signalprocessing/enricher/k8s_enricher.go`
- **Classifiers**: `pkg/signalprocessing/classifier/*.go`
- **CRD Types**: `api/signalprocessing/v1alpha1/signalprocessing_types.go`

### Tests
- **Integration**: `test/integration/signalprocessing/` (~10 tests)
- **Unit**: `test/unit/signalprocessing/` (~20 tests)
- **E2E**: `test/e2e/signalprocessing/` (~3 tests)

### Configuration
- **CRD Schema**: `config/crd/bases/kubernaut.io_signalprocessings.yaml`
- **RBAC**: `config/rbac/signalprocessing_role.yaml`
- **Rego Policies**: `deploy/signalprocessing/policies/*.rego`

---

## Testing Summary

### Test Coverage Breakdown
| Test Type | Count | Coverage | Confidence |
|-----------|-------|----------|------------|
| **Integration** | 10 | 50%+ | 85% |
| **Unit** | 20 | 70%+ | 90% |
| **E2E** | 3 | <10% | 95% |

### Key Test Scenarios Covered
✅ Happy path (Pending → Enriching → Categorizing → Complete)
✅ Error recovery with exponential backoff
✅ Permanent failure after max retries
✅ Partial enrichment (graceful degradation)
✅ Rego policy hot-reload
✅ Audit event creation

---

## Deployment Guide

### Quick Deployment
```bash
# Build
make build-signalprocessing

# Deploy to cluster
kubectl apply -f config/crd/bases/
kubectl apply -f config/rbac/
kubectl apply -f deploy/signalprocessing/

# Verify
kubectl get pods -n kubernaut | grep signalprocessing
```

### RBAC Requirements for DetectedLabels (DD-WORKFLOW-001 v2.1)

> **📋 TODO (Day 14)**: Expand this section with detailed RBAC documentation for operators.

The SignalProcessing controller requires expanded RBAC permissions to auto-detect cluster characteristics:

| Resource | API Group | Verbs | Used For |
|----------|-----------|-------|----------|
| pods | "" (core) | get, list, watch | Pod context enrichment |
| nodes | "" (core) | get, list, watch | Node context enrichment |
| namespaces | "" (core) | get, list, watch | Namespace labels for environment detection |
| configmaps | "" (core) | get, list, watch | Rego policy hot-reload |
| deployments | apps | get, list, watch | OwnerChain traversal |
| replicasets | apps | get, list, watch | OwnerChain traversal |
| statefulsets | apps | get, list, watch | Stateful detection |
| **poddisruptionbudgets** | policy | get, list, watch | PDBProtected detection |
| **horizontalpodautoscalers** | autoscaling | get, list, watch | HPAEnabled detection |
| **networkpolicies** | networking.k8s.io | get, list, watch | NetworkIsolated detection |

**⚠️ Important for Operators**:
- If RBAC permissions are denied, the detection will fail gracefully
- Failed detections are tracked in `status.enrichmentResults.detectedLabels.failedDetections`
- Check controller logs for RBAC-related errors: `Failed to query [resource] (RBAC denied or API error)`

**Minimal RBAC (Core Functionality Only)**:
```yaml
# If you cannot grant extended permissions, use this minimal set:
# DetectedLabels will populate FailedDetections for missing permissions
rules:
- apiGroups: [""]
  resources: ["pods", "nodes", "namespaces", "configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch"]
```

### User Documentation (Day 14)

> **📋 TODO (Day 14)**: Create user-facing documentation covering:

1. **Operator Guide**: How to deploy and configure SignalProcessing
2. **Custom Rego Policies**: How to write and deploy custom classification policies
3. **Troubleshooting Guide**: Common issues and solutions
4. **Monitoring Dashboard**: Grafana dashboard JSON for SignalProcessing metrics

---

## Operational Considerations

### Monitoring
**Key Metrics**:
- `signalprocessing_reconciliations_total{status="success|failure"}`
- `signalprocessing_reconciliation_duration_seconds` (P50, P95, P99)
- `signalprocessing_enrichment_duration_seconds`
- `signalprocessing_classification_duration_seconds`
- `signalprocessing_errors_total{type="transient|permanent"}`

### Alerting Recommendations
- Error rate > 5% for 5 minutes → Page on-call
- Enrichment latency P95 > 5s → Investigate
- Rego evaluation failures → Check policy syntax

---

## Lessons Learned

### What Went Well ✅
1. **ADR-041 separation**: Clean architecture between data fetching and policy evaluation
2. **Parallel test execution**: 4 concurrent processes cut test time by 75%
3. **Rego hot-reload**: Zero-downtime policy updates
4. **DD-006 scaffolding**: Consistent project structure

### Challenges Encountered ⚠️
1. **Rego debugging**: Policy evaluation errors can be cryptic
   - **Resolution**: Added comprehensive logging for policy inputs/outputs
   - **Lesson**: Always log Rego inputs for debugging

---

## Next Steps

### Immediate (Week 1-2)
- [ ] Monitor production deployment for 72 hours
- [ ] Create Grafana dashboard for key metrics
- [ ] Brief on-call team on troubleshooting

### Short-Term (Month 1-3)
- [ ] Performance optimization for high-volume signals
- [ ] Additional Rego policy templates for common use cases

---

**Handoff Complete**: ✅ [Date]
```

---

## 📚 **References**

### **Design Decisions**

**Universal Standards**:
- [DD-005: Observability Standards](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) **MANDATORY** - metrics/logging
- [DD-007: Kubernetes-Aware Graceful Shutdown](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) **MANDATORY**
- [DD-014: Binary Version Logging Standard](../../../architecture/decisions/DD-014-binary-version-logging-standard.md) **MANDATORY**

**CRD Controller Standards**:
- [DD-006: Controller Scaffolding Strategy](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md)
- [DD-013: K8s Client Initialization Standard](../../../architecture/decisions/DD-013-kubernetes-client-initialization-standard.md)

**Testing Standards**:
- [DD-TEST-001: Port Allocation Strategy](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) **MANDATORY for E2E**

**Audit Standards**:
- [DD-AUDIT-003: Service Audit Trace Requirements](../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)

**Service-Specific**:
- [DD-001: Recovery Context Enrichment](../../../architecture/decisions/DD-001-recovery-context-enrichment.md)
- [DD-CATEGORIZATION-001: Gateway vs Signal Processing Split](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- [DD-SIGNAL-PROCESSING-001: Service Rename](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md)
- [DD-017: K8s Enrichment Depth Strategy](../../../architecture/decisions/DD-017-k8s-enrichment-depth-strategy.md)

### **Architecture Decision Records**

**Universal Standards**:
- [ADR-015: Alert-to-Signal Naming Migration](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) **MANDATORY** - use "Signal" terminology

**CRD Controller Standards**:
- [ADR-004: Fake Kubernetes Client](../../../architecture/decisions/ADR-004-fake-kubernetes-client.md) **MANDATORY for unit tests**

**Audit Standards**:
- [ADR-032: Data Access Layer Isolation](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) **MANDATORY** - use Data Storage API
- [ADR-034: Unified Audit Table Design](../../../architecture/decisions/ADR-034-unified-audit-table-design.md) - audit schema
- [ADR-038: Async Buffered Audit Ingestion](../../../architecture/decisions/ADR-038-async-buffered-audit-ingestion.md) - fire-and-forget

**Service-Specific**:
- [ADR-041: Rego Policy Data Fetching Separation](../../../architecture/decisions/ADR-041-rego-policy-data-fetching-separation.md)

### **Templates**
- [DD-006 Controller Templates](../../../templates/crd-controller-gap-remediation/)
- [Service Implementation Plan Template](../../SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)

---

**Document Status**: 📋 DRAFT
**Version**: v1.11
**Last Updated**: 2025-11-28
**Author**: AI Assistant (Cursor)
**Approved By**: Pending
**Next Review**: After Day 0 approval

