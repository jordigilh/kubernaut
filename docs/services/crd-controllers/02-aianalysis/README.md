# AI Analysis Service

**Version**: v2.6
**Status**: ✅ Design Complete (V1.0 scope) - Ready for Implementation
**Health/Ready Port**: 8081 (`/healthz`, `/readyz` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**Service Host Port**: 8084 (Kind extraPortMappings per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))
**CRD**: AIAnalysis
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Controller**: AIAnalysisReconciler
**Priority**: **P0 - HIGH**
**Effort**: 2 weeks
**Go Client**: `pkg/clients/holmesgpt/` (generated with `ogen` from OpenAPI 3.1.0)

---

## 📋 Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.6** | 2025-12-03 | **PodSecurityLevel Removed**: Removed `podSecurityLevel` from DetectedLabels (9→8 fields) per DD-WORKFLOW-001 v2.2; PSP deprecated in K8s 1.21, PSS is namespace-level | [DD-WORKFLOW-001 v2.2](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [NOTICE](../../handoff/NOTICE_PODSECURITYLEVEL_REMOVED.md) |
| v2.5 | 2025-12-02 | **FailedDetections**: Added `failedDetections` field to DetectedLabels per DD-WORKFLOW-001 v2.1; Updated crd-schema, integration-points, implementation-checklist, REGO_POLICY_EXAMPLES | [DD-WORKFLOW-001 v2.1](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) |
| v2.4 | 2025-12-02 | **SPEC ALIGNMENT**: Aligned with handoff Q&A; Fixed HolmesGPT-API port (8080), endpoints, schemas; Removed deprecated fields (RiskTolerance, BusinessCategory, EnrichmentQuality); Added TargetInOwnerChain/Warnings; Go client generated | [AIANALYSIS_TO_HOLMESGPT_API_TEAM.md](../../handoff/AIANALYSIS_TO_HOLMESGPT_API_TEAM.md) |
| v2.3 | 2025-11-30 | **V1.0 COMPLETE**: All spec files updated (finalizers, metrics, database, checklist); Legacy implementation plans archived | This session |
| v2.2 | 2025-11-30 | **FIXED**: Port allocation (8081 health, 8084 host per DD-TEST-001); BR count 31→31; Added TESTING_GUIDELINES reference | [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) |
| v2.0 | 2025-11-30 | **REGENERATED**: Complete spec from Go types; V1.0 scope clarifications; DetectedLabels, CustomLabels, OwnerChain; Recovery flow with PreviousExecutions slice | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md), [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) |
| v1.1 | 2025-10-20 | Added V1.0 approval notification integration | [ADR-018](../../../architecture/decisions/ADR-018-approval-notification-integration.md) |
| v1.0 | 2025-10-15 | Initial design specification | - |

---

## 🗂️ Documentation Index

| Document | Purpose | Status |
|----------|---------|--------|
| **[Overview](./overview.md)** | Service purpose, scope, architecture, key decisions | ✅ Complete (v2.0) |
| **[CRD Schema](./crd-schema.md)** | AIAnalysis CRD types, validation, examples | ✅ **Updated (v2.4)** |
| **[Controller Implementation](./controller-implementation.md)** | Reconciler logic, phase handling, owner references | ✅ Complete (v2.0) |
| **[Reconciliation Phases](./reconciliation-phases.md)** | Phase transitions, timeouts, coordination patterns | ✅ Complete (v2.0) |
| **[AI HolmesGPT & Approval](./ai-holmesgpt-approval.md)** | HolmesGPT integration, Rego policies, approval workflow | ✅ Complete (v2.0) |
| **[Finalizers & Lifecycle](./finalizers-lifecycle.md)** | Cleanup patterns, CRD lifecycle management | ✅ Complete (v2.0) |
| **[Testing Strategy](./testing-strategy.md)** | Unit/Integration/E2E tests, mock patterns | ✅ Complete (v2.0) |
| **[Security Configuration](./security-configuration.md)** | RBAC, network policies, secret handling | ✅ Ports Fixed |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, distributed tracing | ✅ Ports Fixed |
| **[Metrics & SLOs](./metrics-slos.md)** | Prometheus metrics, Grafana dashboards | ✅ Complete (v2.0) |
| **[Database Integration](./database-integration.md)** | Audit storage via Data Storage Service | ✅ Complete (v2.0) |
| **[Integration Points](./integration-points.md)** | Upstream/downstream services, HolmesGPT-API contract | ✅ **Updated (v2.2)** |
| **[Migration & Current State](./migration-current-state.md)** | Existing code, migration path | ✅ Ports Fixed |
| **[Implementation Checklist](./implementation-checklist.md)** | APDC-TDD phases, tasks | ✅ **Updated (v2.2)** |
| **[BR Mapping](./BR_MAPPING.md)** | Business requirements mapping | ✅ Authoritative (v1.3) |
| **[Rego Policy Examples](./REGO_POLICY_EXAMPLES.md)** | Approval policy input schema (v1.4) | ✅ **Updated** |

---

## 📁 File Organization

```
02-aianalysis/
├── 📄 README.md (you are here)              - Service index & navigation
├── 📘 overview.md                           - High-level architecture
├── 🔧 crd-schema.md                         - CRD type definitions ✅
├── ⚙️  controller-implementation.md         - Reconciler logic
├── 🔄 reconciliation-phases.md              - Phase details & coordination
├── 🤖 ai-holmesgpt-approval.md              - AI-specific: HolmesGPT, Rego, approval (SERVICE-SPECIFIC)
├── 🧹 finalizers-lifecycle.md               - Cleanup & lifecycle management
├── 🧪 testing-strategy.md                   - Test patterns (COMMON PATTERN)
├── 🔒 security-configuration.md             - Security patterns (COMMON PATTERN)
├── 📊 observability-logging.md              - Logging & tracing (COMMON PATTERN)
├── 📈 metrics-slos.md                       - Prometheus & Grafana (COMMON PATTERN)
├── 💾 database-integration.md               - Audit storage & schema
├── 🔗 integration-points.md                 - Service coordination
├── 🔀 migration-current-state.md            - Existing code & migration
├── ✅ implementation-checklist.md           - APDC-TDD phases & tasks
├── 📋 BR_MAPPING.md                         - Business requirements ✅
└── 🤖 REGO_POLICY_EXAMPLES.md               - Rego approval policies (SERVICE-SPECIFIC) ✅
```

**Legend**:
- **(COMMON PATTERN)** = Standard files duplicated across all CRD services with service-specific adaptations (per [DD-006](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md))
- **(SERVICE-SPECIFIC)** = Files unique to this service's domain (e.g., Rego policies for AIAnalysis, Tekton pipelines for WorkflowExecution)
- 🤖 = Service-specific domain files
- ✅ = Updated for v2.0

---

## 🏗️ Implementation Structure

### **Binary Location**
- **Directory**: `cmd/aianalysis/`
- **Entry Point**: `cmd/aianalysis/main.go`
- **Build Command**: `go build -o bin/ai-analysis ./cmd/aianalysis`

### **Controller Location**
- **Controller**: `internal/controller/aianalysis/aianalysis_controller.go`
- **CRD Types**: `api/aianalysis/v1alpha1/`

### **Business Logic**
- **Package**: `pkg/aianalysis/`
- **Tests**: `test/unit/aianalysis/`

---

## 🎯 V1.0 Scope

### ✅ In Scope

| Feature | Description | Reference |
|---------|-------------|-----------|
| **HolmesGPT-API Integration** | Single AI provider for investigation | DD-CONTRACT-002 |
| **Workflow Selection** | Select workflow from catalog | DD-WORKFLOW-001 v1.8 |
| **Rego Approval Policies** | ConfigMap-based policy evaluation | DD-AIANALYSIS-001 |
| **Recovery Flow** | [Deprecated - Issue #180] Handle failed workflow retries | DD-RECOVERY-002 |
| **DetectedLabels** (ADR-056: removed from EnrichmentResults) | Auto-detected cluster characteristics | DD-WORKFLOW-001 v1.8 |
| **CustomLabels** | Customer-defined via Rego | DD-WORKFLOW-001 v1.5 |
| **OwnerChain** (ADR-055: removed from EnrichmentResults) | K8s ownership for DetectedLabels validation | DD-WORKFLOW-001 v1.8 |
| **Approval Signaling** | Set `approvalRequired=true` → RO notifies | ADR-040 |

### ❌ Out of Scope (V1.1+)

| Feature | Deferred Reason | Target Version |
|---------|-----------------|----------------|
| **AIApprovalRequest CRD** | Approval orchestration via CRD | V1.1 |
| **Multi-provider LLM** | OpenAI, Anthropic, etc. | V2.0 |
| **LLM Fallback Chains** | Model-specific routing | V2.0 |
| **AI Conditions Engine** | Advanced condition evaluation | V2.0 |

---

## 🔗 Related Services

| Service | Relationship | Purpose |
|---------|--------------|---------|
| **Remediation Orchestrator** | Parent | Creates AIAnalysis CRD, watches for completion |
| **SignalProcessing** | Upstream | Provides EnrichmentResults, DetectedLabels, CustomLabels, OwnerChain |
| **WorkflowExecution** | Downstream | Receives workflow definition from RO |
| **HolmesGPT-API** | External | Provides AI investigation, workflow selection |
| **Data Storage** | External | Workflow catalog, historical success rates |
| **Notification** | External | Approval notifications (V1.0: RO triggers) |

**Coordination Pattern**: CRD-based (no HTTP calls between controllers)

---

## 📋 Business Requirements Coverage

**Status**: ✅ 31 V1.0 BRs Mapped (per [BR_MAPPING.md v1.1](./BR_MAPPING.md))

| Category | BR Count | Description |
|----------|----------|-------------|
| **Core AI Analysis** | 15 | Investigation, RCA, recommendations |
| **Approval & Policy** | 5 | Rego policies, approval signaling |
| **Data Management** | 3 | Payload handling, timeouts, fallback |
| **Quality Assurance** | 5 | Catalog validation, schema validation |
| **Workflow Selection** | 2 | Output format, approval context |
| **Recovery Flow** | [Deprecated - Issue #180] | Recovery attempt handling |
| ~~Dependency Validation~~ | ~~3~~ | ~~Deferred to V2.0+ (predefined workflows)~~ |

**See**: [BR_MAPPING.md](./BR_MAPPING.md) for complete mapping.

---

## 🎯 Key Architectural Decisions

| Decision | Choice | Rationale | Document |
|----------|--------|-----------|----------|
| **AI Provider** | HolmesGPT-API only | Specialized K8s analysis, V1.0 simplicity | DD-CONTRACT-002 |
| **Approval Mechanism** | Rego policies + signaling | Flexible policies, RO orchestrates | DD-AIANALYSIS-001 |
| **State Management** | CRD-based with watch | Watch-based coordination | [Controller Impl](./controller-implementation.md) |
| **Recovery Pattern** | [Deprecated - Issue #180] | RO creates new AIAnalysis for recovery | DD-RECOVERY-002 |
| **Labels Architecture** | DetectedLabels + CustomLabels + OwnerChain | Dual-use: LLM context + workflow filtering | DD-WORKFLOW-001 v1.8 |
| **V1.0 Approval Flow** | `approvalRequired=true` → RO notifies | No AIApprovalRequest CRD in V1.0 | ADR-040 |

---

## 📊 Performance Targets

| Metric | Target | Business Impact |
|--------|--------|----------------|
| **HolmesGPT Analysis** | <30s | AI investigation time |
| **Rego Policy Evaluation** | <2s | Approval decision time |
| **Total Processing** | <60s (auto-approve) | Rapid workflow generation |
| **Confidence Threshold** | >80% for auto-approve | High-quality recommendations |

---

## 🚀 Quick Start

**For New Developers**:
1. **Understand the Service**: Start with [Overview](./overview.md) (5 min read)
2. **Review the CRD**: See [CRD Schema](./crd-schema.md) (10 min read)
3. **Understand AI Integration**: Read [AI HolmesGPT & Approval](./ai-holmesgpt-approval.md)

**For Implementers**:
1. **Check BRs**: Start with [BR_MAPPING.md](./BR_MAPPING.md)
2. **Follow Checklist**: Use [Implementation Checklist](./implementation-checklist.md)
3. **Review Patterns**: Reference [Controller Implementation](./controller-implementation.md)

---

## 🔍 Common Pitfalls & Best Practices

**Don't**:
- ❌ Log HolmesGPT API keys or full responses
- ❌ Create WorkflowExecution CRD directly (RO does this)
- ❌ Skip approval for production actions
- ❌ Hard-code Rego policies (use ConfigMap)
- ❌ Include `HistoricalContext` in LLM prompts (operators only)

**Do**:
- ✅ Use `approvalRequired=true` signaling (V1.0)
- ✅ Include DetectedLabels + CustomLabels + OwnerChain in HolmesGPT-API request
- ✅ Track ALL previous executions in recovery (slice, not single)
- ✅ Use Kubernetes reason codes for failure (not natural language)
- ✅ Emit Kubernetes events for visibility

---

## 📞 Support & Documentation

- **Architecture Overview**: [docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **CRD Design Spec**: [docs/design/CRD/03_AI_ANALYSIS_CRD.md](../../design/CRD/03_AI_ANALYSIS_CRD.md)
- **Port Allocation**: [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md) (AUTHORITATIVE for ports)
- **Testing Guidelines**: [TESTING_GUIDELINES.md](../../../development/business-requirements/TESTING_GUIDELINES.md) (BR vs Unit test decisions)
- **Testing Strategy Rule**: [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)
- **AI/ML Guidelines**: [.cursor/rules/04-ai-ml-guidelines.mdc](../../../../.cursor/rules/04-ai-ml-guidelines.mdc)
- **Documentation Structure**: [DD-006](../../../architecture/decisions/DD-006-controller-scaffolding-strategy.md) (COMMON PATTERN vs SERVICE-SPECIFIC)

---

## 📝 Version History

### **Version 2.3** (2025-11-30) - CURRENT
- ✅ **V1.0 COMPLETE**: All specification files updated and aligned
- ✅ **Updated**: finalizers-lifecycle.md (v2.0) - No AIApprovalRequest in V1.0
- ✅ **Updated**: metrics-slos.md (v2.0) - Approval signaling metrics
- ✅ **Updated**: database-integration.md (v2.0) - DetectedLabels/CustomLabels columns
- ✅ **Updated**: implementation-checklist.md (v2.0) - 31 BRs, 4-phase flow
- ✅ **Archived**: Legacy implementation plans (V1.0, V1.1, V1.2)

### **Version 2.0** (2025-11-30)
- ✅ **REGENERATED** all specifications from Go types
- ✅ **V1.0 Scope Clarifications**: HolmesGPT-API only, approval signaling (no CRD)
- ✅ **Added**: DetectedLabels, CustomLabels, OwnerChain (DD-WORKFLOW-001 v1.8)
- ✅ **Removed**: businessContext, investigationScope, HistoricalContext (for LLM)
- ✅ **Updated**: PreviousExecutions as slice (tracks ALL recovery attempts)
- ✅ **Updated**: Rego policy input schema (v1.2)

### **Version 1.1** (2025-10-20)
- Added V1.0 approval notification integration (ADR-018)

### **Version 1.0** (2025-10-15)
- Initial AI Analysis Service specification

---

**Document Maintenance**:
- **Last Updated**: 2025-11-30
- **Maintained By**: AIAnalysis Service Team
- **Source of Truth**: `api/aianalysis/v1alpha1/aianalysis_types.go`

---

**Ready to implement?** Start with [Implementation Checklist](./implementation-checklist.md) 🚀
