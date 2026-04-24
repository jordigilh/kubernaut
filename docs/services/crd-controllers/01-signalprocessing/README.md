# Signal Processing Service

**Version**: v1.5
**Status**: ✅ IMPLEMENTED - V1.0 Ready (94% - Day 14 docs complete)
**Health/Ready Port**: 8081 (`/health`, `/ready` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**CRD**: SignalProcessing
**CRD API Group**: `kubernaut.ai/v1alpha1` (DD-CRD-001)
**Controller**: SignalProcessingReconciler
**Priority**: **P0 - HIGH**
**Effort**: 14-17 days (quality-focused, includes label detection)

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| v1.5 | 2025-12-09 | V1.0 Implementation complete (17/17 BRs), Day 14 docs (BUILD, OPERATIONS, DEPLOYMENT) | [IMPLEMENTATION_PLAN_V1.31](IMPLEMENTATION_PLAN_V1.31.md), [V1.0_TRIAGE_REPORT](V1.0_TRIAGE_REPORT.md) |
| v1.4 | 2025-11-30 | Cross-team validation complete, API group updated to `.ai`, implementation plan v1.16 | [IMPLEMENTATION_PLAN_V1.16](IMPLEMENTATION_PLAN_V1.16.md), [DD-CRD-001](../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md) |
| v1.3 | 2025-11-30 | DD-WORKFLOW-001 v1.8: OwnerChain, DetectedLabels, CustomLabels, updated gap analysis | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) |
| v1.2 | 2025-11-28 | Port fix: 8080 → 8081, API group standardization, metrics naming, graceful shutdown, parallel testing | [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md), [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md), [ADR-019](../../../architecture/decisions/ADR-019-holmesgpt-circuit-breaker-retry-strategy.md) |
| v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
| v1.1 | 2025-11-27 | Terminology migration: Alert → Signal | [ADR-015](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md) |
| v1.1 | 2025-11-27 | Context API deprecated: Recovery context now embedded by Remediation Orchestrator | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md), [DD-001 Update](../../../architecture/decisions/DD-001-recovery-context-enrichment.md) |
| v1.1 | 2025-11-27 | Categorization consolidated: All categorization now in Signal Processing | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
| v1.0 | 2025-01-15 | Initial design specification | - |

---

## 🗂️ Documentation Index

| Document | Purpose | Lines | Status |
|----------|---------|-------|--------|
| **[Overview](./overview.md)** | Service purpose, scope, architecture, key decisions | ~350 | ✅ Complete |
| **[CRD Schema](./crd-schema.md)** | SignalProcessing CRD types, validation, examples | ~800 | ✅ Complete |
| **[Controller Implementation](./controller-implementation.md)** | Reconciler logic, phase handling, owner references | ~450 | ✅ Complete |
| **[Reconciliation Phases](./reconciliation-phases.md)** | Phase transitions, timeouts, coordination patterns | ~350 | ✅ Complete |
| **[Finalizers & Lifecycle](./finalizers-lifecycle.md)** | Cleanup patterns, CRD lifecycle management, monitoring | ~640 | ✅ Complete |
| **[Testing Strategy](./testing-strategy.md)** | Unit/Integration/E2E tests, mock patterns, anti-patterns | ~600 | ✅ Complete |
| **[Security Configuration](./security-configuration.md)** | RBAC, network policies, secret handling, security context | ~500 | ✅ Complete |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, distributed tracing, correlation IDs | ~460 | ✅ Complete |
| **[Metrics & SLOs](./metrics-slos.md)** | Prometheus metrics, Grafana dashboards, alert rules | ~420 | ✅ Complete |
| **[Database Integration](./database-integration.md)** | Audit storage via Data Storage Service REST API | ~240 | ✅ Complete |
| **[Integration Points](./integration-points.md)** | Upstream/downstream services, external dependencies | ~200 | ✅ Complete |
| **[Migration & Current State](./migration-current-state.md)** | Existing code, migration path, reusability analysis | ~290 | ✅ Complete |
| **[Implementation Checklist](./implementation-checklist.md)** | APDC-TDD phases, tasks, validation steps | ~220 | ✅ Complete |
| **[Implementation Plan v1.31](./IMPLEMENTATION_PLAN_V1.31.md)** | Day-by-day implementation guide (authoritative) | ~6,800 | ✅ Validated |
| **[BUILD.md](./BUILD.md)** | Build instructions, development environment | ~300 | ✅ Complete |
| **[OPERATIONS.md](./OPERATIONS.md)** | Operational procedures, monitoring, troubleshooting | ~400 | ✅ Complete |
| **[DEPLOYMENT.md](./DEPLOYMENT.md)** | Deployment instructions, RBAC, configuration | ~500 | ✅ Complete |
| **[V1.0 Triage Report](./V1.0_TRIAGE_REPORT.md)** | Final V1.0 readiness audit | ~230 | ✅ Complete |

### Cross-Team Validation Records

| Document | Team | Status |
|----------|------|--------|
| [RESPONSE_CUSTOM_LABELS_VALIDATION.md](./RESPONSE_CUSTOM_LABELS_VALIDATION.md) | HolmesGPT-API | ✅ Validated |
| [RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md](./RESPONSE_GATEWAY_LABEL_PASSTHROUGH.md) | Gateway | ✅ Validated |
| [RESPONSE_SIGNALPROCESSING_INTEGRATION_VALIDATION.md](../02-aianalysis/RESPONSE_SIGNALPROCESSING_INTEGRATION_VALIDATION.md) | AIAnalysis | ✅ Validated |

**Total**: ~5,800 lines across 14 documents + 3 validation records

---

## 📁 File Organization

```
01-signalprocessing/
├── 📄 README.md (you are here)              - Service index & navigation
├── 📘 overview.md                           - High-level architecture
├── 🔧 crd-schema.md                         - CRD type definitions
├── ⚙️  controller-implementation.md         - Reconciler logic
├── 🔄 reconciliation-phases.md              - Phase details & coordination
├── 🧹 finalizers-lifecycle.md               - Cleanup & lifecycle management
├── 🧪 testing-strategy.md                   - Test patterns (COMMON PATTERN)
├── 🔒 security-configuration.md             - Security patterns (COMMON PATTERN)
├── 📊 observability-logging.md              - Logging & tracing (COMMON PATTERN)
├── 📈 metrics-slos.md                       - Prometheus & Grafana (COMMON PATTERN)
├── 💾 database-integration.md               - Audit storage & schema
├── 🔗 integration-points.md                 - Service coordination
├── 🔀 migration-current-state.md            - Existing code & migration
└── ✅ implementation-checklist.md           - APDC-TDD phases & tasks
```

**Legend**:
- **(COMMON PATTERN)** = Duplicated across all CRD services with service-specific adaptations
- Service-specific files contain Signal Processing unique logic

---

## 🏗️ Implementation Structure

### **Binary Location**
- **Directory**: `cmd/signalprocessing/`
- **Entry Point**: `cmd/signalprocessing/main.go`
- **Build Command**: `go build -o bin/signal-processing ./cmd/signalprocessing`

### **Controller Location**
- **Controller**: `internal/controller/signalprocessing/signalprocessing_controller.go`
- **CRD Types**: `api/signalprocessing/v1alpha1/`

### **Business Logic**
- **Package**: `pkg/signalprocessing/`
- **Tests**: `test/unit/signalprocessing/`

**See Also**: [cmd/ directory structure](../../../../cmd/README.md) for complete binary organization.

---

## 🚀 Quick Start

**For New Developers**:
1. **Understand the Service**: Start with [Overview](./overview.md) (5 min read)
2. **Review the CRD**: See [CRD Schema](./crd-schema.md) (15 min read)
3. **Understand Phases**: Read [Reconciliation Phases](./reconciliation-phases.md) (10 min read)

**For Implementers**:
1. **Check Migration**: Start with [Migration & Current State](./migration-current-state.md)
2. **Follow Checklist**: Use [Implementation Checklist](./implementation-checklist.md)
3. **Review Patterns**: Reference [Controller Implementation](./controller-implementation.md)

**For Reviewers**:
1. **Security Review**: Check [Security Configuration](./security-configuration.md)
2. **Testing Review**: Verify [Testing Strategy](./testing-strategy.md)
3. **Observability**: Validate [Metrics & SLOs](./metrics-slos.md)

---

## 🔗 Related Services

| Service | Relationship | Purpose |
|---------|--------------|---------|
| **Gateway Service** | Upstream | Creates RemediationRequest CRD (duplicate detection already done, sets placeholder categorization) |
| **RemediationRequest Controller** | Parent | Creates SignalProcessing CRD (initial & recovery), embeds failure data for recovery attempts |
| **AIAnalysis Service** | Downstream | Receives complete enrichment data (monitoring + business context) |
| **Data Storage Service** | External | Provides audit trail persistence via REST API ([ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)) |

**Coordination Pattern**: CRD-based (no HTTP calls between controllers)

**Key Architectural Changes**:
- **Context API DEPRECATED**: Recovery context no longer queried from Context API. Remediation Orchestrator embeds current failure data from WorkflowExecution CRD. See [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)
- **Categorization Consolidated**: All categorization (environment classification + priority assignment) performed by Signal Processing after K8s context enrichment. Gateway sets placeholder values. See [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- **Data Access Layer Isolation**: Signal Processing uses Data Storage Service REST API for audit writes (no direct PostgreSQL). See [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md)

**Design Decisions**:
- [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) - Service rename
- [DD-001](../../../architecture/decisions/DD-001-recovery-context-enrichment.md) - Recovery context enrichment (updated: embedded by Remediation Orchestrator)

---

## 📋 Business Requirements Coverage

| Category | Range | Description |
|----------|-------|-------------|
| **Primary** | BR-SP-001 to BR-SP-050 | Signal processing and enrichment logic |
| **Environment** | BR-SP-051 to BR-SP-053 | Environment classification (production/staging/dev) |
| **Enrichment** | BR-SP-060 to BR-SP-062 | Signal enrichment, correlation, timeout handling |
| **Categorization** | BR-SP-070 to BR-SP-075 | Priority assignment after K8s context enrichment ([DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)) |
| **Tracking** | BR-SP-021 | Signal lifecycle state tracking |
| **Deduplication** | BR-WH-008 | Gateway Service responsibility (NOT Signal Processing) |

**Notes**:
- Duplicate signal handling is a Gateway Service responsibility
- Recovery context is embedded by Remediation Orchestrator (Context API deprecated per [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md))
- All categorization consolidated in Signal Processing per [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)

---

## 🎯 Key Architectural Decisions

| Decision | Choice | Rationale | Document |
|----------|--------|-----------|----------|
| **Service Name** | SignalProcessing | Alignment with Gateway's "signal" terminology | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
| **CRD API Group** | `kubernaut.io/v1alpha1` | Unified API group for all Kubernaut CRDs | [001-crd-api-group-rationale.md](../../../architecture/decisions/001-crd-api-group-rationale.md) |
| **Processing Model** | Single-phase synchronous | Fast operations (<5s), no multi-phase complexity | [Reconciliation Phases](./reconciliation-phases.md) |
| **State Management** | CRD-based with watch | Watch-based coordination, no HTTP polling | [Controller Implementation](./controller-implementation.md) |
| **Recovery Context** | Embedded by Remediation Orchestrator | Context API deprecated; failure data from WorkflowExecution CRD | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md), [DD-001](../../../architecture/decisions/DD-001-recovery-context-enrichment.md) |
| **Categorization** | Consolidated in Signal Processing | Richer K8s context available after enrichment | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
| **Degraded Mode** | Multi-level fallback | Signal labels + minimal context fallback | [Reconciliation Phases](./reconciliation-phases.md) |
| **Duplicate Detection** | Gateway Service | Already handled upstream (BR-WH-008) | [Overview](./overview.md#deduplication) |
| **Owner Reference** | RemediationRequest owns this | Cascade deletion with 24h retention | [Finalizers & Lifecycle](./finalizers-lifecycle.md) |
| **Secret Handling** | Never log verbatim | Sanitize all secrets before storage/logging | [Security Configuration](./security-configuration.md) |
| **Audit Storage** | Data Storage Service REST API | No direct PostgreSQL access per ADR-032 | [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) |
| **Graceful Shutdown** | 4-step K8s-aware pattern | Zero request failures during rolling updates | [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md) |
| **Retry Strategy** | K8s requeue (no circuit breaker) | Internal dependencies only, K8s handles backpressure | [ADR-019](../../../architecture/decisions/ADR-019-holmesgpt-circuit-breaker-retry-strategy.md) |
| **K8s Enrichment** | Standard depth (hardcoded) | Avoids SRE configuration complexity | [DD-017](../../../architecture/decisions/DD-017-k8s-enrichment-depth-strategy.md) |
| **Rego Data Fetching** | K8s Enricher + Rego Engine | Separation of concerns for security/performance | [ADR-041](../../../architecture/decisions/ADR-041-rego-policy-data-fetching-separation.md) |
| **Label Detection** ⭐ | OwnerChain + DetectedLabels + CustomLabels | Workflow filtering via DD-WORKFLOW-001 v1.8 | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) |
| **CustomLabels Rego** ⭐ | Customer policies with security wrapper | 5 mandatory labels protected from override | [HANDOFF v3.2](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) |

---

## 🏗️ Implementation Status

### Existing Code (Verified)
- **Location**: `pkg/signalprocessing/` (migrated from `pkg/remediationprocessing/`)
- **Reusability**: 85-95% (see [Migration & Current State](./migration-current-state.md))
- **Tests**: `test/unit/signalprocessing/`

### Gap Analysis
- ❌ SignalProcessing CRD schema (need to create)
- ❌ SignalProcessingReconciler controller (need to create)
- ❌ CRD lifecycle management (owner refs, finalizers)
- ❌ Watch-based status coordination with RemediationRequest
- ❌ OwnerChain builder (DD-WORKFLOW-001 v1.8) ⭐ NEW
- ❌ DetectedLabels auto-detection (9 types) ⭐ NEW
- ❌ CustomLabels Rego engine with security wrapper ⭐ NEW

### Migration Effort
- **Package Migration**: Complete - renamed to `pkg/signalprocessing/`
- **CRD Controller**: 3-4 days (new implementation)
- **Label Detection**: 2-3 days (DD-WORKFLOW-001 v1.8) ⭐ NEW
- **Testing**: 2 days (migrate tests, add integration tests, label detection tests)
- **Total**: ~2 weeks (extended for label detection)

---

## 🎓 Development Methodology

**Mandatory Process**: Follow APDC-Enhanced TDD workflow per [.cursor/rules/00-core-development-methodology.mdc](../../../../.cursor/rules/00-core-development-methodology.mdc)

**Quick Reference**:
```
ANALYSIS → PLAN → DO-RED → DO-GREEN → DO-REFACTOR → CHECK
```

**See**: [Implementation Checklist](./implementation-checklist.md) for complete APDC phase breakdown.

---

## 📊 Performance Targets

| Metric | Target | Business Impact |
|--------|--------|----------------|
| **Total Processing (Initial)** | <5s | Rapid remediation start (SLO target) |
| **Enrichment** | <2s | Fast K8s context gathering |
| **Classification** | <1s | Quick environment detection |
| **Categorization** | <1s | Priority assignment after K8s context enrichment |
| **Audit Write** | <1ms P95 | Fire-and-forget pattern (ADR-038) |
| **Accuracy** | >99% for production | Correct priority routing |
| **Degraded Mode** | <5% of signals | Most signals fully enriched |

**Note**: Performance targets aligned with [IMPLEMENTATION_PLAN_V1.11.md](./IMPLEMENTATION_PLAN_V1.11.md).

**Monitoring**: See [Metrics & SLOs](./metrics-slos.md) for Prometheus metrics and Grafana dashboards.

---

## 🔍 Common Pitfalls & Best Practices

**Don't**:
- ❌ Query Context API for recovery context (DEPRECATED per [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md))
- ❌ Access PostgreSQL directly (use Data Storage Service REST API per [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md))
- ❌ Create AIAnalysis CRD directly (RemediationRequest does this)
- ❌ Log secrets verbatim (sanitize all sensitive data)
- ❌ Skip owner reference (needed for cascade deletion)
- ❌ Perform categorization in Gateway (consolidated in Signal Processing per [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md))

**Do**:
- ✅ Use degraded mode for enrichment service failures
- ✅ Read recovery context from `spec.failureData` (embedded by Remediation Orchestrator)
- ✅ Perform all categorization after K8s context enrichment
- ✅ Emit Kubernetes events for visibility
- ✅ Implement phase timeouts (5s for enrichment, 2s for classification)
- ✅ Cache environment classification (5 min TTL)
- ✅ Use Data Storage Service REST API for audit writes

**See**: Each document's "Common Pitfalls" section for detailed guidance.

---

## 📞 Support & Documentation

- **Binary Structure**: [cmd/ directory structure](../../../../cmd/README.md)
- **Build & Deploy**: [cmd/ build guide](../../../../cmd/README.md#building-services)
- **Architecture Overview**: [docs/architecture/KUBERNAUT_CRD_ARCHITECTURE.md](../../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md)
- **CRD Design Spec**: [docs/design/CRD/02_SIGNAL_PROCESSING_CRD.md](../../../design/CRD/02_SIGNAL_PROCESSING_CRD.md)
- **Testing Strategy Rule**: [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)
- **AI Assistant Rules**: [.cursor/rules/10-ai-assistant-behavioral-constraints.mdc](../../../../.cursor/rules/10-ai-assistant-behavioral-constraints.mdc)

---

## 📝 Document Maintenance

**Last Updated**: 2025-11-27
**Document Structure Version**: 1.1
**Status**: ✅ Production Ready (98% Confidence)

**Common Pattern Updates**: When updating common patterns (testing, security, observability, metrics), update:
1. This service (01-signalprocessing/)
2. AI Analysis (02-aianalysis/)
3. Workflow Execution (03-workflowexecution/)
4. Remediation Orchestrator (05-remediationorchestrator/)

---

**Ready to implement?** Start with [Implementation Checklist](./implementation-checklist.md) 🚀

