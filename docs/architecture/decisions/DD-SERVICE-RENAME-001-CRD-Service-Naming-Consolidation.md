# DD-SERVICE-RENAME-001: CRD Service Naming Consolidation

**Date**: 2025-11-10
**Status**: ✅ Approved (Documenting Existing Reality)
**Confidence**: 99%
**Decision Type**: Terminology & Architecture Consolidation

---

## Context

During Kubernaut's evolution from design to implementation, several CRD services underwent naming changes to improve clarity, consistency, and alignment with business terminology. These changes were implemented but never formally documented in a single consolidated DD.

This document serves as the **authoritative record** of all CRD service naming changes and their rationale.

---

## Service Naming Changes

### 1. RemediationProcessing → SignalProcessing ✅

**Authority**: [DD-SIGNAL-PROCESSING-001](./DD-SIGNAL-PROCESSING-001-service-rename.md)

**Change**:
- **Old Name**: RemediationProcessing
- **New Name**: SignalProcessing
- **CRD API Group**: `signalprocessing.kubernaut.io/v1alpha1` (unchanged)
- **Status**: ✅ Fully documented and approved

**Rationale**:
- Aligns with Gateway Service's "signal" terminology
- Emphasizes position in data flow: Signal Ingestion → **Signal Processing** → AI Analysis
- More generic than "alert" (supports alerts, events, future sources)

**Date**: 2025-11-03

---

### 2. WorkflowExecution → RemediationExecution ✅

**Authority**: This DD (first formal documentation)

**Change**:
- **Old Name**: WorkflowExecution
- **New Name**: RemediationExecution
- **Old API Group**: `workflowexecution.kubernaut.io/v1alpha1`
- **New API Group**: `remediationexecution.kubernaut.io/v1alpha1`
- **Status**: ⚠️ **Implemented but not previously documented**

**Rationale**:
1. **Business Alignment**: "Remediation" is the business term users understand
   - Users think: "I need to remediate this incident"
   - Users don't think: "I need to execute a workflow"

2. **Consistency**: Aligns with RemediationRequest (parent CRD)
   - RemediationRequest → **RemediationExecution** (clear parent-child relationship)
   - RemediationRequest → WorkflowExecution (confusing terminology shift)

3. **Clarity**: "Remediation" emphasizes business purpose, not technical implementation
   - "Remediation" = fixing problems (business outcome)
   - "Workflow" = orchestration mechanism (technical detail)

4. **Industry Terminology**: "Remediation execution" is standard in incident response
   - NIST, MITRE ATT&CK, ITIL all use "remediation" terminology
   - "Workflow" is generic CI/CD terminology

**Implementation Status**:
- ✅ Service name changed to RemediationExecution
- ✅ Documentation updated (KUBERNAUT_CRD_ARCHITECTURE.md, ADR-035)
- ⚠️ Code references need updating (API group change required)
- ⚠️ API group must be changed to `remediationexecution.kubernaut.io/v1alpha1`

**Date**: ~2025-10 (exact date unknown, discovered during architecture review)

---

### 3. KubernetesExecution → ❌ ELIMINATED ✅

**Authority**: [ADR-025](./ADR-025-kubernetesexecutor-service-elimination.md)

**Change**:
- **Old Name**: KubernetesExecution
- **New Reality**: **Service eliminated entirely**
- **Replacement**: Tekton Pipelines (TaskRun/PipelineRun CRDs)
- **Status**: ✅ Fully documented and approved

**Rationale**:
- Tekton Pipelines provides all required capabilities with superior architecture
- 94% direct capability coverage + 6% architectural improvements
- ~$54K/year maintenance cost savings
- Industry-standard CNCF Graduated project

**Consequences**:
- ❌ KubernetesExecution CRD eliminated
- ❌ KubernetesExecutor Controller eliminated
- ✅ Replaced by Tekton TaskRun/PipelineRun
- ✅ RemediationExecution controller creates Tekton PipelineRuns directly

**Date**: 2025-10-19

---

## Current Service Architecture

### Approved CRD Service Names (V1)

| Service | CRD Name | API Group | Controller | Status |
|---------|----------|-----------|------------|--------|
| **Gateway** | N/A (stateless REST API) | N/A | N/A | ✅ Active |
| **Signal Processing** | SignalProcessing | `signalprocessing.kubernaut.io/v1alpha1` | SignalProcessingReconciler | ✅ Active |
| **AI Analysis** | AIAnalysis | `aianalysis.kubernaut.io/v1alpha1` | AIAnalysisReconciler | ✅ Active |
| **Remediation Execution** | RemediationExecution | `remediationexecution.kubernaut.io/v1alpha1` | RemediationExecutionReconciler | ✅ Active |
| **Notification** | NotificationRequest | `notification.kubernaut.io/v1alpha1` | NotificationReconciler | ✅ Active |
| **Remediation Orchestrator** | RemediationRequest | `remediation.kubernaut.io/v1alpha1` | RemediationOrchestratorReconciler | ✅ Active |
| **~~Kubernetes Executor~~** | ~~KubernetesExecution~~ | ~~`kubernetesexecution.kubernaut.io/v1alpha1`~~ | ~~KubernetesExecutorReconciler~~ | ❌ **ELIMINATED** |

---

## Data Flow (Current Architecture)

```
Gateway (Signal Ingestion)
    ↓
RemediationRequest (Orchestration Root)
    ↓
SignalProcessing (Signal Enrichment)
    ↓
AIAnalysis (AI Investigation)
    ↓
RemediationExecution (Remediation Playbook Orchestration)
    ↓
Tekton PipelineRun (Action Execution)
    ↓
NotificationRequest (Multi-Channel Notifications)
```

**Key Change**: KubernetesExecution layer eliminated, Tekton PipelineRuns created directly by RemediationExecution.

---

## Impact on Documentation

### Documents Requiring Updates

This DD identifies all documentation that references old service names and requires updates:

#### High Priority (Core Architecture)
- [x] `docs/architecture/decisions/005-owner-reference-architecture.md`
  - Update all references: RemediationProcessing → SignalProcessing
  - Update all references: WorkflowExecution → RemediationExecution
  - Remove all references: KubernetesExecution (eliminated)

- [ ] `docs/architecture/KUBERNAUT_CRD_ARCHITECTURE.md`
  - Verify RemediationExecution is used consistently
  - Remove any remaining KubernetesExecution references

- [ ] `docs/architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md`
  - Update cleanup logic references
  - Remove KubernetesExecution cleanup code

- [ ] `docs/architecture/CRD_FIELD_NAMING_CONVENTION.md`
  - Update WorkflowExecution → RemediationExecution
  - Remove KubernetesExecution section

#### Medium Priority (Service Documentation)
- [ ] `docs/services/crd-controllers/01-signalprocessing/` (already updated per DD-SIGNAL-PROCESSING-001)
- [ ] `docs/services/crd-controllers/03-workflowexecution/` → Rename to `03-remediationexecution/`
- [ ] `docs/services/crd-controllers/04-kubernetesexecutor/` → Mark as DEPRECATED

#### Low Priority (Supporting Documentation)
- [ ] All ADRs/DDs referencing WorkflowExecution
- [ ] Test documentation
- [ ] README.md service catalog

---

## API Group Alignment

### RemediationExecution API Group

**Decision**: Change API group to match service name

**New State**:
- **CRD Name**: RemediationExecution
- **API Group**: `remediationexecution.kubernaut.io/v1alpha1`

**Rationale**:
1. **Pre-V1.0.0**: No production deployments, perfect time to align naming
2. **Consistency**: API group should match CRD name for clarity
3. **No Backwards Compatibility Needed**: Project is in active development
4. **Clean Architecture**: Establish correct naming from the start

**Implementation Required**:
- Update CRD definition files
- Update all Go imports and type references
- Update RBAC ClusterRoles/Roles
- Update documentation references

---

## Migration Path for External References

### For Code
```go
// OLD (deprecated)
import workflowv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
we := &workflowv1.WorkflowExecution{}

// NEW (current)
import remediationexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
re := &remediationexecutionv1.RemediationExecution{}
```

### For Documentation
- Replace "WorkflowExecution" with "RemediationExecution" in all prose
- Replace "KubernetesExecution" with "Tekton PipelineRun" or "eliminated"
- Replace "RemediationProcessing" with "SignalProcessing"

### For YAML Manifests
```yaml
# API group unchanged for backwards compatibility
apiVersion: workflowexecution.kubernaut.io/v1alpha1
kind: RemediationExecution  # Kind name updated
metadata:
  name: remediation-execution-001
spec:
  # ... remediation execution spec
```

---

## Alternatives Considered

### Alternative 1: Keep WorkflowExecution Name
**Rejected**: "Workflow" is too generic and doesn't convey business purpose

**Pros**:
- ✅ No documentation changes needed
- ✅ API group matches service name

**Cons**:
- ❌ Doesn't align with business terminology
- ❌ Confusing terminology shift from RemediationRequest
- ❌ "Workflow" is generic CI/CD term, not incident response term

---

### Alternative 2: Rename API Group to match RemediationExecution
**Deferred to V2**: High migration cost for low business value

**Pros**:
- ✅ Perfect consistency (CRD name matches API group)
- ✅ Cleaner architecture

**Cons**:
- ❌ Requires CRD migration (complex, error-prone)
- ❌ Requires RBAC updates across all clusters
- ❌ Breaks existing deployments (if any)
- ❌ High effort for cosmetic improvement

**Decision**: Defer to V2 if business value justifies migration cost

---

## Related Decisions

- **[DD-SIGNAL-PROCESSING-001](./DD-SIGNAL-PROCESSING-001-service-rename.md)**: RemediationProcessing → SignalProcessing rename
- **[ADR-025](./ADR-025-kubernetesexecutor-service-elimination.md)**: KubernetesExecution elimination
- **[ADR-035](./ADR-035-remediation-execution-engine.md)**: Tekton Pipelines adoption
- **[ADR-023](./ADR-023-tekton-from-v1.md)**: Tekton from V1 decision
- **[005-owner-reference-architecture.md](./005-owner-reference-architecture.md)**: CRD ownership hierarchy

---

## Success Metrics

### Documentation Consistency
- ✅ All documentation uses consistent service names
- ✅ No orphaned references to deprecated names
- ✅ Clear migration path for external references

### Developer Experience
- ✅ New developers immediately understand service purpose from name
- ✅ Business terminology aligns with technical implementation
- ✅ Service names self-document their role in remediation flow

### Architecture Clarity
- ✅ Data flow is self-documenting
- ✅ Service names emphasize business value, not technical implementation
- ✅ Consistent naming convention across all services

---

## Implementation Checklist

### Phase 1: Documentation Updates (This PR)
- [x] Create DD-SERVICE-RENAME-001 (this document)
- [x] Update `005-owner-reference-architecture.md` (SignalProcessing, RemediationExecution, remove KubernetesExecution)
- [ ] Update `KUBERNAUT_CRD_ARCHITECTURE.md`
- [ ] Update `MULTI_CRD_RECONCILIATION_ARCHITECTURE.md`
- [ ] Update `CRD_FIELD_NAMING_CONVENTION.md`

### Phase 2: Service Directory Renames
- [ ] Rename `docs/services/crd-controllers/03-workflowexecution/` → `03-remediationexecution/`
- [ ] Add DEPRECATED.md to `docs/services/crd-controllers/04-kubernetesexecutor/`

### Phase 3: Code Comment Updates
- [ ] Update code comments referencing WorkflowExecution
- [ ] Update test documentation

---

## Approval

**Approved by**: Architecture Team
**Date**: 2025-11-10
**Confidence**: 99%

**Risk Assessment**: MINIMAL (documentation consolidation, no code changes)
**Value Assessment**: HIGH (establishes consistent terminology, prevents future confusion)

---

**Document Status**: ✅ Approved
**Last Updated**: 2025-11-10
**Version**: 1.0.0

