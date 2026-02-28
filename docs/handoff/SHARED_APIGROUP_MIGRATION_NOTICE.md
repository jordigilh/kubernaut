# SHARED: API Group Migration to Single `kubernaut.ai` Group

**Date**: December 13, 2025
**Priority**: 🔴 **BREAKING CHANGE - ACTION REQUIRED**
**Target Teams**: SignalProcessing, AIAnalysis, WorkflowExecution, RemediationOrchestrator, Notification
**Authoritative Standard**: [DD-CRD-001: CRD API Group Domain Selection](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)

---

## 🚨 Executive Summary

**What Changed**: All CRDs must migrate from resource-specific API groups to a **single API group** `kubernaut.ai/v1alpha1`

**Why**: Aligns with authoritative standard DD-CRD-001 (updated 2025-12-13), industry best practices, and original architectural decision

**Impact**: Breaking change - requires code changes, CRD regeneration, and E2E test updates

**Timeline**: Migration to be completed by December 20, 2025 (before RO E2E coordination begins)

---

## Migration Overview

### Current State (Resource-Specific Groups)

```yaml
# BEFORE - Resource-specific API groups
apiVersion: remediation.kubernaut.ai/v1alpha1
kind: RemediationRequest

apiVersion: signalprocessing.kubernaut.ai/v1alpha1
kind: SignalProcessing

apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIAnalysis

apiVersion: workflowexecution.kubernaut.ai/v1alpha1
kind: WorkflowExecution

apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest

apiVersion: remediationorchestrator.kubernaut.ai/v1alpha1
kind: RemediationOrchestrator

apiVersion: kubernetesexecution.kubernaut.io/v1alpha1  # Also needs .io → .ai (DEPRECATED - ADR-025)
kind: KubernetesExecution
```

### Target State (Single API Group)

```yaml
# AFTER - Single API group for all CRDs
apiVersion: kubernaut.ai/v1alpha1
kind: RemediationRequest

apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing

apiVersion: kubernaut.ai/v1alpha1
kind: AIAnalysis

apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution

apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest

apiVersion: kubernaut.ai/v1alpha1
kind: RemediationOrchestrator

apiVersion: kubernaut.ai/v1alpha1
kind: KubernetesExecution

apiVersion: kubernaut.ai/v1alpha1
kind: RemediationApprovalRequest
```

---

## Rationale

### Why Single API Group?

**Architectural Fit**:
- All CRDs are **tightly-coupled workflow phases** in a unified remediation pipeline
- They are **sequential phases**, not independent services
- No organizational benefit from separate API groups

**Industry Pattern**:
| Project | Type | API Group Strategy |
|---------|------|-------------------|
| K8sGPT | AI K8s diagnostics | `core.k8sgpt.ai` (single group) |
| Prometheus Operator | Monitoring | `monitoring.coreos.com` (single group) |
| Cert-Manager | Certificates | `cert-manager.io` (single group) |
| ArgoCD | GitOps | `argoproj.io` (single group) |
| **Istio** | Service Mesh | Multiple groups (distinct feature domains: networking vs security) |

**Kubernetes Best Practice** (2025):
> "Simplify to top-level domain when possible. Use subdomains only for large number of subresources requiring further categorization."

**Original Decision**:
- `001-crd-api-group-rationale.md` (October 2025): Single group `kubernaut.io` (95% confidence)
- Explicitly rejected subdomains: "Redundant, no need for subdomain"

**Benefits**:
- ✅ Simpler kubectl commands: `kubectl get remediationrequests.kubernaut.ai`
- ✅ Clear project identity: All resources under one umbrella
- ✅ Easier RBAC: Single API group for permissions
- ✅ Reduced cognitive load: 1 API group vs 7 resource-specific groups

---

## Team-Specific Migration Tasks

### 📋 SignalProcessing Team

**CRDs to Migrate**: `SignalProcessing`

**Files to Update**:
1. `api/signalprocessing/v1alpha1/groupversion_info.go`
2. `config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml` (regenerate)
3. `internal/controller/signalprocessing/signalprocessing_controller.go` (RBAC annotations)
4. E2E test manifests in `test/e2e/signalprocessing/`

**Estimated Effort**: 2-3 hours

---

### 📋 AIAnalysis Team

**CRDs to Migrate**: `AIAnalysis`

**Files to Update**:
1. `api/aianalysis/v1alpha1/groupversion_info.go`
2. `config/crd/bases/aianalysis.kubernaut.ai_aianalyses.yaml` (regenerate)
3. `internal/controller/aianalysis/aianalysis_controller.go` (RBAC annotations)
4. E2E test manifests in `test/e2e/aianalysis/`

**Estimated Effort**: 2-3 hours

---

### 📋 WorkflowExecution Team

**CRDs to Migrate**: `WorkflowExecution`

**Files to Update**:
1. `api/workflowexecution/v1alpha1/groupversion_info.go`
2. `config/crd/bases/workflowexecution.kubernaut.ai_workflowexecutions.yaml` (regenerate)
3. `internal/controller/workflowexecution/workflowexecution_controller.go` (RBAC annotations)
4. E2E test manifests in `test/e2e/workflowexecution/`

**Estimated Effort**: 2-3 hours

---

### 📋 RemediationOrchestrator Team

**CRDs to Migrate**:
- `RemediationRequest`
- `RemediationOrchestrator`
- `RemediationApprovalRequest`

**Files to Update**:
1. `api/remediation/v1alpha1/groupversion_info.go`
2. `api/remediationorchestrator/v1alpha1/groupversion_info.go`
3. `config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml` (regenerate)
4. `config/crd/bases/remediation.kubernaut.ai_remediationapprovalrequests.yaml` (regenerate)
5. `config/crd/bases/remediationorchestrator.kubernaut.ai_remediationorchestrators.yaml` (regenerate)
6. `internal/controller/remediation/remediationrequest_controller.go` (RBAC annotations)
7. `internal/controller/remediationorchestrator/remediationorchestrator_controller.go` (RBAC annotations)
8. E2E test manifests in `test/e2e/remediationorchestrator/`

**Estimated Effort**: 4-6 hours (3 CRDs)

---

### 📋 Notification Team

**CRDs to Migrate**: `NotificationRequest`

**Files to Update**:
1. `api/notification/v1alpha1/groupversion_info.go`
2. `config/crd/bases/notification.kubernaut.ai_notificationrequests.yaml` (regenerate)
3. `internal/controller/notification/notificationrequest_controller.go` (RBAC annotations)
4. E2E test manifests in `test/e2e/notification/`

**Estimated Effort**: 2-3 hours

**Status**: ✅ Notification team will migrate as part of BR-NOT-069 implementation

---

### 📋 KubernetesExecution (DEPRECATED - ADR-025) Team (Deferred Service)

**CRDs to Migrate**: `KubernetesExecution`

**Special Note**: This CRD is still using `.io` domain AND needs resource-specific group migration

**Files to Update**:
1. `api/kubernetesexecution/v1alpha1/groupversion_info.go`
   - Change: `kubernetesexecution.kubernaut.io` → `kubernaut.ai`
2. `config/crd/bases/kubernetesexecution.kubernaut.io_kubernetesexecutions.yaml` (regenerate)
3. `internal/controller/kubernetesexecution/kubernetesexecution_controller.go` (RBAC annotations)

**Estimated Effort**: 2-3 hours

**Priority**: Lower (deferred service, not actively used)

---

## Step-by-Step Migration Guide

### Step 1: Update Go API Definition (10 minutes)

**File**: `api/<resource>/v1alpha1/groupversion_info.go`

```diff
  // Package v1alpha1 contains API Schema definitions for the <resource> v1alpha1 API group
  // +kubebuilder:object:generate=true
- // +groupName=<resource>.kubernaut.ai
+ // +groupName=kubernaut.ai
  package v1alpha1

  import (
      "k8s.io/apimachinery/pkg/runtime/schema"
      "sigs.k8s.io/controller-runtime/pkg/scheme"
  )

  var (
      // GroupVersion is group version used to register these objects
-     GroupVersion = schema.GroupVersion{Group: "<resource>.kubernaut.ai", Version: "v1alpha1"}
+     GroupVersion = schema.GroupVersion{Group: "kubernaut.ai", Version: "v1alpha1"}

      // SchemeBuilder is used to add go types to the GroupVersionKind scheme
      SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

      // AddToScheme adds the types in this group-version to the given scheme.
      AddToScheme = SchemeBuilder.AddToScheme
  )
```

---

### Step 2: Update Controller RBAC Annotations (10 minutes)

**File**: `internal/controller/<resource>/<resource>_controller.go`

```diff
- //+kubebuilder:rbac:groups=<resource>.kubernaut.ai,resources=<resources>,verbs=get;list;watch;create;update;patch;delete
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=<resources>,verbs=get;list;watch;create;update;patch;delete
- //+kubebuilder:rbac:groups=<resource>.kubernaut.ai,resources=<resources>/status,verbs=get;update;patch
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=<resources>/status,verbs=get;update;patch
- //+kubebuilder:rbac:groups=<resource>.kubernaut.ai,resources=<resources>/finalizers,verbs=update
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=<resources>/finalizers,verbs=update
```

**Example** (SignalProcessing):
```diff
- //+kubebuilder:rbac:groups=signalprocessing.kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
+ //+kubebuilder:rbac:groups=kubernaut.ai,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
```

---

### Step 3: Regenerate CRD Manifests (5 minutes)

```bash
# From project root
make manifests

# Verify new CRD manifest filename
ls -la config/crd/bases/kubernaut.ai_<resources>.yaml

# Example: config/crd/bases/kubernaut.ai_signalprocessings.yaml
```

**Expected Changes**:
- Old file: `config/crd/bases/<resource>.kubernaut.ai_<resources>.yaml`
- New file: `config/crd/bases/kubernaut.ai_<resources>.yaml`

**⚠️ Important**: Delete old CRD manifest files after verifying new ones are correct

---

### Step 4: Update E2E Test Manifests (30 minutes)

**Files**: `test/e2e/<resource>/*.go` and any YAML manifests

```diff
- apiVersion: <resource>.kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
  kind: <ResourceKind>
```

**Search Command**:
```bash
# Find all occurrences in your service's tests
grep -r "<resource>.kubernaut.ai" test/e2e/<resource>/ test/integration/<resource>/
```

---

### Step 5: Update Documentation (20 minutes)

**Files to Update**:
- Service README: `docs/services/crd-controllers/<number>-<resource>/README.md`
- Implementation plans
- API specifications
- Examples in handoff documents

```diff
- apiVersion: <resource>.kubernaut.ai/v1alpha1
+ apiVersion: kubernaut.ai/v1alpha1
```

---

### Step 6: Run Tests (30 minutes)

```bash
# Unit tests
make test

# Integration tests
make test-integration

# E2E tests (if applicable)
make test-e2e-<resource>
```

---

### Step 7: Commit Changes

```bash
git add .
git commit -m "refactor: migrate <Resource> CRD to single API group kubernaut.ai

- Update API group from <resource>.kubernaut.ai to kubernaut.ai
- Align with DD-CRD-001 authoritative standard (2025-12-13 update)
- Regenerate CRD manifests
- Update controller RBAC annotations
- Update E2E test manifests and documentation

Ref: docs/handoff/SHARED_APIGROUP_MIGRATION_NOTICE.md"
```

---

## Validation Checklist

After migration, verify the following:

### Go Code
- [ ] `groupversion_info.go` updated: `Group: "kubernaut.ai"`
- [ ] Kubebuilder annotation updated: `+groupName=kubernaut.ai`
- [ ] Controller RBAC annotations updated: `groups=kubernaut.ai`

### CRD Manifests
- [ ] New CRD manifest exists: `config/crd/bases/kubernaut.ai_<resources>.yaml`
- [ ] Old CRD manifest deleted: `config/crd/bases/<resource>.kubernaut.ai_<resources>.yaml`
- [ ] CRD manifest contains: `group: kubernaut.ai`

### Tests
- [ ] Unit tests pass: `make test`
- [ ] Integration tests pass: `make test-integration`
- [ ] E2E tests pass: `make test-e2e-<resource>` (if applicable)
- [ ] E2E test manifests updated to `apiVersion: kubernaut.ai/v1alpha1`

### Documentation
- [ ] Service README updated with new API group
- [ ] Examples updated in implementation plans
- [ ] API specifications updated

### Build
- [ ] Code compiles: `make build`
- [ ] No lint errors: `make lint`
- [ ] CRDs install successfully: `kubectl apply -f config/crd/bases/kubernaut.ai_<resources>.yaml`

---

## kubectl Command Changes

### Before (Resource-Specific Groups)

```bash
# List resources - verbose
kubectl get signalprocessings.signalprocessing.kubernaut.ai
kubectl get aianalyses.aianalysis.kubernaut.ai
kubectl get workflowexecutions.workflowexecution.kubernaut.ai
kubectl get remediationrequests.remediation.kubernaut.ai
kubectl get notificationrequests.notification.kubernaut.ai

# List resources - short names (still work)
kubectl get sp
kubectl get aa
kubectl get we
kubectl get rr
kubectl get nr
```

### After (Single API Group)

```bash
# List resources - simpler syntax
kubectl get signalprocessings.kubernaut.ai
kubectl get aianalyses.kubernaut.ai
kubectl get workflowexecutions.kubernaut.ai
kubectl get remediationrequests.kubernaut.ai
kubectl get notificationrequests.kubernaut.ai

# List resources - short names (unchanged)
kubectl get sp
kubectl get aa
kubectl get we
kubectl get rr
kubectl get nr

# List ALL Kubernaut resources (new capability!)
kubectl api-resources | grep kubernaut.ai
```

---

## Cross-Team Coordination

### Dependencies

**No cross-team code dependencies** - each team can migrate independently.

**E2E Test Coordination**: If your E2E tests create CRDs from other services:
- Example: RemediationOrchestrator E2E tests create `SignalProcessing` CRDs
- Solution: Update test manifests after dependency team completes migration

### Migration Order Recommendation

**Suggested Order** (to minimize E2E test updates):

1. **Phase 1**: Independent CRDs (no E2E dependencies)
   - [ ] SignalProcessing
   - [ ] AIAnalysis
   - [ ] Notification

2. **Phase 2**: Orchestrator (creates other CRDs in E2E tests)
   - [ ] RemediationOrchestrator (update after Phase 1 completes)

3. **Phase 3**: Execution layer
   - [ ] WorkflowExecution

4. **Phase 4**: Deferred service
   - [ ] KubernetesExecution (low priority)

---

## Shared E2E Infrastructure Updates

**File**: `test/e2e/shared_functions.go` (if CRD creation helpers exist)

```diff
  func CreateSignalProcessing(namespace, name string) error {
      sp := &unstructured.Unstructured{
          Object: map[string]interface{}{
-             "apiVersion": "signalprocessing.kubernaut.ai/v1alpha1",
+             "apiVersion": "kubernaut.ai/v1alpha1",
              "kind":       "SignalProcessing",
              // ...
          },
      }
      return k8sClient.Create(ctx, sp)
  }
```

**Action Required**: If your team maintains shared E2E helpers, update them after migration.

---

## FAQ

### Q1: Why are we doing this now?

**A**: The authoritative standard DD-CRD-001 was updated (2025-12-13) to align with:
- Original architectural decision (October 2025, 95% confidence)
- Industry best practices (Kubernetes community guidance)
- Comparable projects (K8sGPT, Prometheus, Cert-Manager, ArgoCD)

### Q2: Is this a breaking change?

**A**: Yes, but acceptable:
- ✅ Pre-release product (no external customers)
- ✅ Mechanical change (well-defined scope)
- ✅ Improves long-term maintainability

### Q3: Can I keep the resource-specific group?

**A**: No. DD-CRD-001 is the authoritative standard. All CRDs must comply for consistency.

### Q4: What about KubernetesExecution's `.io` domain?

**A**: KubernetesExecution must migrate **both**:
1. Domain: `.io` → `.ai`
2. Group: `kubernetesexecution.kubernaut.ai` → `kubernaut.ai`

### Q5: Will short names (`sp`, `rr`, `aa`) still work?

**A**: Yes! Short names are independent of the API group. They are defined in CRD `spec.names.shortNames`.

### Q6: What if I find issues during migration?

**A**:
1. Document the issue in `docs/handoff/APIGROUP_MIGRATION_ISSUES_<TEAM>.md`
2. Ask questions in #kubernaut-api-group-migration Slack channel (if exists)
3. Reference this shared document in commit messages

### Q7: Do I need to update RBAC ClusterRoles?

**A**: RBAC is auto-generated from controller annotations. After updating annotations and running `make manifests`, RBAC manifests will update automatically.

### Q8: Will this affect deployed clusters?

**A**: Yes - requires CRD reinstallation:
```bash
# Delete old CRD
kubectl delete crd <resources>.<resource>.kubernaut.ai

# Install new CRD
kubectl apply -f config/crd/bases/kubernaut.ai_<resources>.yaml
```

**Note**: Pre-release, so no production impact.

---

## Timeline

**Migration Start**: December 13, 2025
**Target Completion**: **Before segmented E2E tests with RO and other services**

**Critical Dependency**: All CRD teams must complete API group migration before E2E test coordination work begins to avoid test manifest conflicts and ensure consistent API group usage across the platform.

**Estimated Total Effort by Team**:
- SignalProcessing: 2-3 hours
- AIAnalysis: 2-3 hours
- WorkflowExecution: 2-3 hours
- RemediationOrchestrator: 4-6 hours (3 CRDs)
- Notification: 2-3 hours
- KubernetesExecution: 2-3 hours (deferred, low priority)

**Total Platform Effort**: ~15-20 hours across all teams

---

## Support & Questions

**Primary Contact**: [Your Name/Team] - Notification team led migration

**Authoritative Documentation**:
- [DD-CRD-001: CRD API Group Domain Selection](../architecture/decisions/DD-CRD-001-api-group-domain-selection.md)
- [TRIAGE_API_GROUP_NAMING_STRATEGY.md](TRIAGE_API_GROUP_NAMING_STRATEGY.md) (detailed analysis)

**Shared Slack Channel**: #kubernaut-api-group-migration (if created)

**Questions**: Add questions to this document in the FAQ section or contact the Notification team.

---

## Acknowledgment

Please acknowledge receipt and estimated completion timeline:

### SignalProcessing Team
- [x] **Acknowledged**: SP Team (AI Assistant) on December 13, 2025
- [x] **Estimated Completion**: December 13, 2025 (Completed same day)
- [x] **Migration Status**: ✅ **COMPLETE** (2-3 hours as estimated)

### AIAnalysis Team
- [x] **Acknowledged**: AIAnalysis Team on December 13, 2025
- [x] **Estimated Completion**: December 13, 2025 (same day)
- [x] **Migration Status**: In Progress (executing now)

### WorkflowExecution Team
- [x] **Acknowledged**: WorkflowExecution Team on December 13, 2025
- [x] **Estimated Completion**: December 13, 2025 (COMPLETED)
- [x] **Migration Status**: ✅ **COMPLETE** (2.5 hours actual effort)

### RemediationOrchestrator Team
- [ ] **Acknowledged**: [Name] on [Date]
- [ ] **Estimated Completion**: [Date]
- [ ] **Migration Status**: Not Started / In Progress / Complete

### Notification Team
- [x] **Acknowledged**: Notification Team on December 13, 2025
- [ ] **Estimated Completion**: Before BR-NOT-069 implementation
- [ ] **Migration Status**: Not Started (separate task, will complete before BR-NOT-069)

### KubernetesExecution Team (Deferred)
- [ ] **Acknowledged**: [Name] on [Date]
- [ ] **Estimated Completion**: [Date]
- [ ] **Migration Status**: Deferred (service not actively used)

---

**Document Created**: December 13, 2025
**Last Updated**: December 13, 2025
**Status**: 🔴 **ACTION REQUIRED - AWAITING TEAM ACKNOWLEDGMENT**

