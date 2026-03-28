# Workflow Execution Service

**Version**: v4.1
**Status**: ✅ Implementation Complete (Days 1-10)
**Health/Ready Port**: 8081 (`/health`, `/ready` - no auth required)
**Metrics Port**: 9090 (`/metrics` - with auth filter)
**CRD**: WorkflowExecution
**CRD API Group**: `kubernaut.ai/v1alpha1` ([DD-CRD-001](../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md))
**Controller**: WorkflowExecutionReconciler
**Priority**: **P0 - HIGH**
**Implementation Date**: 2025-12-04

---

## 📋 Prerequisites

### Required

| Dependency | Version | Purpose |
|------------|---------|---------|
| **Tekton Pipelines** | Latest stable | Workflow execution engine |
| **Bundle Resolver** | Built-in | Resolves OCI bundle references |
| **kubernaut-workflows** namespace | - | Dedicated namespace for all PipelineRuns |

### Dedicated Execution Namespace Setup

**All PipelineRuns execute in `kubernaut-workflows` namespace** (industry standard pattern).

**DD-WE-005 v2.0**: Kubernaut does **not** create a shared runner ServiceAccount via Helm. Operators pre-create **per-workflow** ServiceAccounts (example name `my-workflow-sa`) and RBAC, then reference them from the workflow schema (`serviceAccountName`). If omitted, Kubernetes uses the namespace default ServiceAccount.

```yaml
# 1. Create dedicated namespace for workflow execution
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-workflows
---
# 2. Example: ServiceAccount for one workflow (you choose names and rules)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-workflow-sa
  namespace: kubernaut-workflows
---
# 3. Example ClusterRole — scope to what this workflow needs (least privilege)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-workflow-remediation
rules:
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "patch", "update"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "delete"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "patch", "cordon", "uncordon"]
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["get", "list"]
---
# 4. Bind ClusterRole to ServiceAccount
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: my-workflow-sa-remediation
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: my-workflow-remediation
subjects:
- kind: ServiceAccount
  name: my-workflow-sa
  namespace: kubernaut-workflows
```

**Benefits**:
- ✅ All remediation activity in one namespace (audit clarity)
- ✅ Per-workflow ServiceAccounts and RBAC (no shared platform runner SA)
- ✅ Easy PipelineRun cleanup and resource quota management
- ✅ No pollution of application namespaces

### Optional: Signed Bundle Verification (V1.0)

To require signed OCI bundles, deploy a Tekton VerificationPolicy:

```yaml
apiVersion: tekton.dev/v1alpha1
kind: VerificationPolicy
metadata:
  name: require-signed-bundles
  namespace: kubernaut-system
spec:
  resources:
    - pattern: "ghcr.io/kubernaut/workflows/*"
  authorities:
    - key:
        secretRef:
          name: cosign-public-key
          namespace: kubernaut-system
```

**Dependencies for signed bundles**:
- Tekton Pipelines with Trusted Resources enabled (`enable-tekton-oci-bundles: "true"`)
- Cosign public key in Secret `cosign-public-key`
- Workflows signed with `cosign sign`

---

## 🗂️ Documentation Index

| Document | Purpose | Status |
|----------|---------|--------|
| **[Overview](./overview.md)** | Service purpose, scope, architecture, key decisions | ✅ Complete |
| **[CRD Schema](./crd-schema.md)** | WorkflowExecution CRD types, validation, examples | ✅ Complete |
| **[Controller Implementation](./controller-implementation.md)** | Reconciler logic, phase handling, owner references | ✅ Complete |
| **[Reconciliation Phases](./reconciliation-phases.md)** | Phase transitions, Tekton status sync | ✅ Complete |
| **[Finalizers & Lifecycle](./finalizers-lifecycle.md)** | Cleanup patterns, CRD lifecycle management | ✅ Complete |
| **[Testing Strategy](./testing-strategy.md)** | Unit/Integration/E2E tests, mock patterns | ✅ Complete |
| **[Security Configuration](./security-configuration.md)** | RBAC, network policies, secret handling | ✅ Complete |
| **[Observability & Logging](./observability-logging.md)** | Structured logging, distributed tracing | ✅ Complete |
| **[Metrics & SLOs](./metrics-slos.md)** | Prometheus metrics, Grafana dashboards | ✅ Complete |
| **[Database Integration](./database-integration.md)** | Audit storage, PostgreSQL schema | ✅ Complete |
| **[Integration Points](./integration-points.md)** | Upstream/downstream services | ✅ Complete |
| **[Implementation Checklist](./implementation-checklist.md)** | APDC-TDD phases, tasks, validation | ✅ Complete |

---

## 📁 File Organization

```
03-workflowexecution/
├── 📄 README.md (you are here)              - Service index & navigation
├── 📘 overview.md                           - High-level architecture
├── 🔧 crd-schema.md                         - CRD type definitions
├── ⚙️  controller-implementation.md         - Reconciler logic
├── 🔄 reconciliation-phases.md              - Phase details & Tekton sync
├── 🧹 finalizers-lifecycle.md               - Cleanup & lifecycle management
├── 🧪 testing-strategy.md                   - Test patterns (COMMON PATTERN)
├── 🔒 security-configuration.md             - Security patterns (COMMON PATTERN)
├── 📊 observability-logging.md              - Logging & tracing (COMMON PATTERN)
├── 📈 metrics-slos.md                       - Prometheus & Grafana (COMMON PATTERN)
├── 💾 database-integration.md               - Audit storage & schema
├── 🔗 integration-points.md                 - Service coordination
└── ✅ implementation-checklist.md           - APDC-TDD phases & tasks
```

**Legend**:
- **(COMMON PATTERN)** = Duplicated across all CRD services with service-specific adaptations

---

## 🏗️ Implementation Structure

### **Binary Location**
- **Directory**: `cmd/workflowexecution/`
- **Entry Point**: `cmd/workflowexecution/main.go`
- **Build Command**: `go build -o bin/workflow-execution ./cmd/workflowexecution`

### **Controller Location**
- **Controller**: `internal/controller/workflowexecution/workflowexecution_controller.go`
- **CRD Types**: `api/workflowexecution/v1alpha1/`

### **Business Logic**
- **Package**: `pkg/workflowexecution/`
- **Tests**: `test/unit/workflowexecution/`

**See Also**: [cmd/ directory structure](../../../../cmd/README.md) for complete binary organization.

---

## 🚀 Quick Start

**For New Developers**:
1. **Understand the Service**: Start with [Overview](./overview.md) (5 min read)
2. **Review the CRD**: See [CRD Schema](./crd-schema.md) (20 min read)
3. **Understand Phases**: Read [Reconciliation Phases](./reconciliation-phases.md) (10 min read)

**For Implementers**:
1. **Follow Checklist**: Use [Implementation Checklist](./implementation-checklist.md)
2. **Review Patterns**: Reference [Controller Implementation](./controller-implementation.md)
3. **Test Patterns**: See [Testing Strategy](./testing-strategy.md)

**For Reviewers**:
1. **Security Review**: Check [Security Configuration](./security-configuration.md)
2. **Testing Review**: Verify [Testing Strategy](./testing-strategy.md)
3. **Observability**: Validate [Metrics & SLOs](./metrics-slos.md)

---

## 🔗 Related Services

| Service | Relationship | Purpose |
|---------|--------------|---------|
| **RemediationOrchestrator** | Parent | Creates WorkflowExecution CRD, watches for completion |
| **AIAnalysis Service** | Upstream | Provides selected workflow and parameters |
| **Tekton Pipelines** | Downstream | Executes workflows via PipelineRun (ADR-044) |
| **Data Storage Service** | External | Persists audit trail for compliance |

**Coordination Pattern**: CRD-based (no HTTP calls between controllers)

---

## 🎯 Service Responsibilities

1. **Create PipelineRun** - Create Tekton PipelineRun from user-provided workflow OCI bundle
2. **Pass Parameters** - Forward workflow parameters to PipelineRun
3. **Check Resource Locks** - Prevent parallel/redundant execution (DD-WE-001)
4. **Sync Status** - Map PipelineRun conditions to WorkflowExecution status
5. **Extract Failures** - Build FailureDetails from TaskRun errors for recovery context

---

## 📋 Business Requirements Coverage

| Category | Range | Description |
|----------|-------|-------------|
| **Core Execution** | BR-WE-001 to BR-WE-008 | Tekton PipelineRun creation, status monitoring, audit |
| **Resource Locking** | BR-WE-009 to BR-WE-011 | **Resource locking safety** ([DD-WE-001](../../../architecture/decisions/DD-WE-001-resource-locking-safety.md)) |

**Key Safety Features (v4.0)**:
- **BR-WE-009**: Prevent parallel execution on same target resource
- **BR-WE-010**: Cooldown period prevents redundant sequential execution
- **BR-WE-011**: Target resource identification for locking

See: [BR-WE-009-011-resource-locking.md](../../../requirements/BR-WE-009-011-resource-locking.md)

---

## 🎯 Key Architectural Decisions

| Decision | Choice | Document |
|----------|--------|----------|
| **Execution Model** | Tekton PipelineRun | [ADR-044](../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md) |
| **Workflow Source** | User-provided OCI bundles | [ADR-043](../../../architecture/decisions/ADR-043-workflow-schema-definition-standard.md) |
| **Resource Locking** | Target-scoped locking | [DD-WE-001](../../../architecture/decisions/DD-WE-001-resource-locking-safety.md) |
| **Owner Reference** | RemediationRequest owns this | [Finalizers & Lifecycle](./finalizers-lifecycle.md) |
| **Failure Recovery** | Rich failure details for LLM | [CRD Schema](./crd-schema.md) |

---

## 🏗️ Implementation Status

### Existing Code (Verified)
- **Location**: `pkg/workflowexecution/` (new package)
- **Reusability**: Tekton PipelineRun patterns from existing services
- **Tests**: `test/unit/workflowexecution/`

### Gap Analysis
- ❌ WorkflowExecution CRD schema (need to create)
- ❌ WorkflowExecutionReconciler controller (need to create)
- ❌ Tekton PipelineRun creation logic
- ❌ Resource locking implementation (DD-WE-001)
- ❌ FailureDetails extraction from Tekton TaskRun
- ❌ CRD lifecycle management (owner refs, finalizers)

### Migration Effort
- **Controller Scaffolding**: 2 days (CRD types + controller)
- **Tekton Integration**: 3-4 days (PipelineRun creation + status watching)
- **Resource Locking**: 2-3 days (DD-WE-001 implementation)
- **Testing**: 2-3 days (unit + integration + E2E)
- **Total**: ~2 weeks

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
| **PipelineRun Creation** | <5s | Fast workflow initiation |
| **Status Sync** | <10s | Quick phase updates from Tekton |
| **Resource Lock Check** | <100ms | Fast parallel execution prevention |
| **Total Workflow** | <30min | Configurable timeout per workflow |

**Monitoring**: See [Metrics & SLOs](./metrics-slos.md) for Prometheus metrics and Grafana dashboards.

---

## 🔍 Best Practices

- ✅ Check resource lock before creating PipelineRun
- ✅ Pass all parameters from AIAnalysis to PipelineRun
- ✅ Extract rich failure details from TaskRun for recovery
- ✅ Emit Kubernetes events for visibility
- ✅ Write audit trail for all executions (including Skipped)
- ✅ Set owner reference for cascade deletion

**See**: Each document's "Best Practices" section for detailed guidance.

---

## 📞 Support & Documentation

### User & Operations Guides
- **Workflow Author's Guide**: [docs/development/guides/user/workflow-authoring.md](../../../development/guides/user/workflow-authoring.md) - How to create Tekton workflows
- **Troubleshooting Guide**: [docs/operations/troubleshooting/service-specific/workflowexecution-issues.md](../../../operations/troubleshooting/service-specific/workflowexecution-issues.md) - Common issues and solutions
- **Production Runbook**: [docs/operations/runbooks/workflowexecution-runbook.md](../../../operations/runbooks/workflowexecution-runbook.md) - Operational procedures

### Technical References
- **Binary Structure**: [cmd/ directory structure](../../../../cmd/README.md)
- **Build & Deploy**: [cmd/ build guide](../../../../cmd/README.md#building-services)
- **Architecture Overview**: [docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)
- **CRD Design Spec**: [docs/design/CRD/04_WORKFLOW_EXECUTION_CRD.md](../../../design/CRD/04_WORKFLOW_EXECUTION_CRD.md)
- **Testing Strategy Rule**: [.cursor/rules/03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc)

---

## 📝 Document Maintenance

**Last Updated**: 2025-12-06
**Document Structure Version**: 4.2
**Status**: ✅ Implementation Complete

**Changelog**:
| Version | Date | Changes |
|---------|------|---------|
| 4.4 | 2026-03-21 | **DD-WE-005 v2.0**: README setup uses operator-managed `my-workflow-sa` + example bindings; removed Helm-provisioned shared runner SA. |
| 4.3 | 2026-02-18 | **Issue #91**: Removed `kubernaut.ai/component` label from Namespace example (ownerRef sufficient for CRD ownership) |
| 4.2 | 2025-12-06 | Added links to new user guides, troubleshooting, and runbook in centralized docs/ |
| 4.1 | 2025-12-04 | **Implementation Complete** - Full controller implemented with tests |
| 4.0 | 2025-12-02 | Simplified documentation, updated architecture section |
| 3.1 | 2025-12-02 | Updated API group to `.ai`, port to 8081, BR-WE-* prefix, Tekton architecture |
| 3.0 | 2025-12-01 | Added resource locking (DD-WE-001), enhanced failure details |
| 2.0 | 2025-11-28 | Simplified schema per ADR-044 (Tekton delegation) |

**Common Pattern Updates**: When updating common patterns (testing, security, observability, metrics), update all CRD services.

---

**Ready to implement?** Start with [Implementation Checklist](./implementation-checklist.md) 🚀
