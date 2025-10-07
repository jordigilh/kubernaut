## Current State & Migration Path

### Existing Business Logic (Verified)

**Current Location**: `pkg/platform/executor/` (implementation exists)
**Target Location**: `pkg/kubernetesexecution/` (after refactor)

**Existing Components**:
```
pkg/platform/executor/
├── executor.go (245 lines)          ✅ Action execution interface
├── kubernetes_executor.go (418 lines) ✅ K8s action implementations
└── actions.go (182 lines)           ✅ Action type definitions
```

**Existing Tests**:
- `test/unit/platform/executor/` → `test/unit/kubernetesexecution/`
- `test/integration/kubernetes_operations/` → `test/integration/kubernetesexecution/`

### Component Reuse Mapping

| Existing Component | CRD Controller Usage | Reusability | Migration Effort | Notes |
|-------------------|---------------------|-------------|-----------------|-------|
| **Action Interface** | Action type definitions | 85% | Low | ✅ Adapt to typed parameters |
| **K8s Executor** | Job creation and monitoring | 60% | Medium | ⚠️ Refactor for Job-based execution |
| **Action Implementations** | Command building logic | 90% | Low | ✅ Reuse kubectl command generation |
| **Validation Logic** | Pre-execution validation | 75% | Medium | ✅ Add Rego policy integration |

### Implementation Gap Analysis

**What Exists (Verified)**:
- ✅ Basic action execution framework (pkg/platform/executor/)
- ✅ Kubernetes client integration
- ✅ Action type definitions
- ✅ Command building for common actions

**What's Missing (CRD V1 Requirements)**:
- ❌ KubernetesExecution CRD schema
- ❌ KubernetesExecutionReconciler controller
- ❌ Native Kubernetes Job creation and lifecycle management
- ❌ Per-action ServiceAccount creation and RBAC
- ❌ Rego policy integration for safety validation
- ❌ Dry-run validation Jobs
- ❌ Rollback information extraction and storage
- ❌ Approval gate handling
- ❌ Comprehensive audit trail to database

**Code Quality Issues to Address**:
- ⚠️ **Refactor for Job-Based Execution**: Current implementation uses direct kubectl calls
  - Need to wrap all actions in Kubernetes Job specifications
  - Add Job monitoring and status tracking
  - Implement Job cleanup with TTL
  - Estimated effort: 3-4 days

**Estimated Migration Effort**: 10-15 days (2-3 weeks)
- Day 1-2: CRD schema + controller skeleton
- Day 3-5: Job creation and monitoring logic
- Day 6-8: Rego policy integration + validation
- Day 9-10: Per-action ServiceAccounts + RBAC
- Day 11-12: Testing and refinement
- Day 13-15: Integration with WorkflowExecution + audit

---

