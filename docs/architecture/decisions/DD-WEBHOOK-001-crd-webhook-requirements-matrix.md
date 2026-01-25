# DD-WEBHOOK-001: CRD Webhook Requirements Matrix

**Date**: January 6, 2026 (v1.2 - Single Consolidated Webhook Architecture)
**Status**: ✅ **AUTHORITATIVE**
**Purpose**: Define WHEN and WHY CRDs require webhooks for authenticated user operations
**Authority**: Decision criteria for all CRD webhook implementations
**Scope**: All Kubernetes CRDs in Kubernaut requiring user authentication

**Version History**:
- **v1.2** (January 6, 2026): **ARCHITECTURE UPDATE**: Single consolidated webhook deployment (`kubernaut-auth-webhook`) with multiple handlers. Updated implementation approach and timelines. Added references to comprehensive implementation and test plans.
- **v1.1** (January 6, 2026): Added NotificationRequest (DELETE attribution). **Note**: Workflow CRUD uses HTTP middleware, not CRD webhook (see DD-AUTH-003)
- **v1.0** (December 20, 2025): Initial version with WorkflowExecution + RemediationApprovalRequest

---

## 🎯 **DECISION**

**Not all CRDs require webhooks. Webhooks SHALL be implemented ONLY when CRD status updates require authenticated user identity for audit trail or operational decisions.**

**Decision Criteria**: A CRD requires a webhook if it meets ANY of these conditions:
1. **Manual Intervention**: Users manually modify status fields as operational decisions
2. **SOC2 Attribution**: Changes require capturing WHO made the decision (CC8.1)
3. **Approval Workflows**: Human operators approve/reject automated actions
4. **Override Actions**: Users override controller-determined state

---

## 🏗️ **CONSOLIDATED WEBHOOK ARCHITECTURE** (v1.2)

### **Single Webhook Deployment, Multiple Handlers**

**Key Principle**: One webhook service (`kubernaut-auth-webhook`) handles authentication for ALL CRDs requiring webhooks.

```
┌────────────────────────────────────────────────────────────────┐
│  Single Pod: kubernaut-auth-webhook                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  Webhook Server (controller-runtime)                     │ │
│  │                                                           │ │
│  │  Route: /mutate-workflowexecution                        │ │
│  │    → WorkflowExecutionAuthHandler                        │ │
│  │    → Populates: status.blockClearanceRequest.clearedBy  │ │
│  │                                                           │ │
│  │  Route: /mutate-remediationapprovalrequest               │ │
│  │    → RemediationApprovalRequestAuthHandler               │ │
│  │    → Populates: status.approvalRequest.approvedBy        │ │
│  │                                                           │ │
│  │  Route: /validate-notificationrequest-delete             │ │
│  │    → NotificationRequestDeleteHandler                    │ │
│  │    → Captures: metadata.deletionTimestamp + user         │ │
│  │                                                           │ │
│  │  Shared: ExtractAuthenticatedUser(req.UserInfo)          │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                 │
│  Port: 9443 (DEFAULT - HTTPS with TLS cert)                   │
│  Data Storage URL: http://datastorage-service:8080 (DEFAULT)  │
│  Metrics: ❌ NONE (audit traces sufficient)                   │
└────────────────────────────────────────────────────────────────┘
```

**Default Configuration**:
- **Webhook Port**: `9443` (standard Kubernetes webhook port - no configuration needed)
- **Data Storage URL**: `http://datastorage-service:8080` (K8s service name - no configuration needed)
- **Metrics**: ❌ **None** - Audit traces capture 100% of operations; K8s API server exposes webhook metrics
- **Override**: Via environment variables or CLI flags for dev/test environments

### **Benefits of Consolidated Approach**

| Aspect | Separate Webhooks | Consolidated Webhook ✅ | Improvement |
|--------|-------------------|-------------------------|-------------|
| **Pods** | 3 | 1 | 66% reduction |
| **Memory** | 150MB (3×50MB) | 50MB | 66% reduction |
| **Configs** | 3 webhooks | 1 webhook | Simpler maintenance |
| **Deployment time** | 3× slower | Fast | 3× faster |
| **Code consistency** | Risk of drift | Guaranteed | Shared logic |
| **Testing** | 3× test suites | 1 unified suite | Easier maintenance |

### **Implementation Structure**

```
cmd/authwebhook/main.go                    # Single webhook server entry point
pkg/authwebhook/auth/common.go             # Shared: ExtractAuthenticatedUser()
pkg/authwebhook/workflowexecution_handler.go     # WE-specific logic
pkg/authwebhook/remediationapprovalrequest_handler.go  # RAR-specific logic
pkg/authwebhook/notificationrequest_handler.go         # NR-specific logic
```

**Comprehensive Plans**:
- **[Implementation Plan](../../development/SOC2/WEBHOOK_IMPLEMENTATION_PLAN.md)**: 5-6 day roadmap with APDC-TDD methodology
- **[Test Plan](../../development/SOC2/WEBHOOK_TEST_PLAN.md)**: 95 tests (70 unit + 11 integration + 14 E2E)

---

## ⚙️ **Service Configuration (AUTHORITATIVE)**

### **Default Configuration** (Zero Config Production)

| Parameter | Default Value | Override Method | Purpose |
|-----------|---------------|-----------------|---------|
| **Webhook Port** | `9443` | `--webhook-port` or `WEBHOOK_PORT` | Standard K8s webhook HTTPS port |
| **Data Storage URL** | `http://datastorage-service:8080` | `--data-storage-url` or `WEBHOOK_DATA_STORAGE_URL` | Audit event API endpoint |
| **Cert Directory** | `/tmp/k8s-webhook-server/serving-certs` | `--cert-dir` | TLS certificate location |
| **Metrics** | ❌ **None** | N/A | Audit traces sufficient; K8s API server exposes webhook metrics |

### **Configuration Priority** (Highest to Lowest)

1. **CLI Flags**: Explicit command-line arguments
2. **Environment Variables**: `WEBHOOK_*` prefixed variables
3. **Default Values**: Sensible production defaults

### **Production Deployment** (No Configuration Needed)

```yaml
# deploy/webhooks/deployment.yaml
containers:
- name: webhook
  image: kubernaut/auth-webhook:latest
  # No args needed - all defaults work in production
  ports:
  - containerPort: 9443  # Webhook HTTPS port (only port needed)
    name: webhook
  # No metrics port - audit traces sufficient
```

**Result**: Webhook works out-of-box in standard Kubernetes environments with zero configuration.

### **Development/Test Overrides**

```yaml
# Integration tests - override Data Storage URL only
env:
- name: WEBHOOK_DATA_STORAGE_URL
  value: "http://localhost:18099"
# Webhook port 9443 uses default
```

```yaml
# Staging - override Data Storage URL only
env:
- name: WEBHOOK_DATA_STORAGE_URL
  value: "http://datastorage-staging:8080"
# Webhook port 9443 uses default
```

### **CLI Flag Reference**

```bash
./webhooks-controller \
  --webhook-port=9443 \                          # DEFAULT (can omit)
  --data-storage-url=http://datastorage-service:8080 \  # DEFAULT (can omit)
  --cert-dir=/tmp/k8s-webhook-server/serving-certs      # DEFAULT (can omit)

# Minimal production command (all defaults):
./webhooks-controller

# No metrics flag - audit traces sufficient
```

### **Why Port 9443?**

**Standard**: Port 9443 is the de facto standard for Kubernetes admission webhooks
- ✅ Used by cert-manager, OPA Gatekeeper, Istio, and other K8s webhooks
- ✅ Well-known port for webhook HTTPS traffic
- ✅ No conflicts with application ports (8080-8089 range)
- ✅ Firewall-friendly (standard HTTPS alternative port)

**Authority**: Kubernetes webhook best practices and community conventions

---

## 📊 **CRD Webhook Requirements Matrix**

### **CRDs Requiring Webhooks** ✅ (3 Total)

| CRD | Use Case | Status Fields Requiring Auth | SOC2 Control | Implementation Owner | Priority | Target Version |
|-----|----------|------------------------------|--------------|----------------------|----------|----------------|
| **WorkflowExecution** | Block Clearance | `status.blockClearanceRequest` | CC8.1 (Attribution) | WE Team | P0 | v1.0 |
| **RemediationApprovalRequest** | Approval Decisions | `status.approvalRequest` | CC8.1 (Attribution) | RO Team | P0 | v1.0 |
| **NotificationRequest** | Cancellation Attribution | `metadata.deletionTimestamp` (DELETE) | CC8.1 (Attribution) | Notification Team | P0 | v1.1 |

**Note**: Workflow catalog CRUD operations use externalized authorization via sidecar (REST API), not CRD webhooks. See `DD-AUTH-003-externalized-authorization-sidecar.md` for workflow CRUD attribution.

### **CRDs NOT Requiring Webhooks** ❌

| CRD | Why No Webhook | Status Update Pattern |
|-----|----------------|----------------------|
| **SignalProcessing** | Controller-only status | Controller manages K8s context enrichment |
| **AIAnalysis** | Controller-only status | Controller manages AI investigation results |
| **RemediationRequest** | Controller-only status | RO controller manages routing logic |

**Note**: **KubernetesExecution** was deprecated 2025-10-19 (never implemented, replaced by Tekton Pipelines). See [DEPRECATED.md](../../services/crd-controllers/04-kubernetesexecutor/DEPRECATED.md)

---

## 🔍 **Decision Criteria - Detailed**

### **Criterion 1: Manual Intervention Required**

**When**: Human operators must manually modify CRD status as operational decisions.

**Examples**:
- ✅ **WorkflowExecution**: Operator clears `PreviousExecutionFailed` block after manual investigation
- ✅ **RemediationApprovalRequest**: Operator approves/rejects remediation after risk assessment
- ❌ **SignalProcessing**: Status updated automatically by controller (no manual intervention)

**Test**: Ask "Does a human operator need to manually patch this status field?"
- If YES → Webhook required
- If NO → No webhook needed

---

### **Criterion 2: SOC2 Attribution (CC8.1)**

**When**: Changes must capture authenticated user identity for compliance audits.

**SOC2 Control CC8.1**: "The entity identifies, captures, and retains sufficient, reliable information to achieve its service commitments and system requirements."

**Examples**:
- ✅ **WorkflowExecution Block Clearance**: Must record WHO cleared the block (SOC2 CC8.1)
- ✅ **RemediationApprovalRequest**: Must record WHO approved remediation (SOC2 CC8.1)
- ❌ **AIAnalysis Investigation Status**: No attribution needed (automated AI investigation)

**Test**: Ask "Does SOC2 auditor need to know WHO made this change?"
- If YES → Webhook required
- If NO → No webhook needed

---

### **Criterion 3: Approval Workflows**

**When**: Human operators approve/reject automated actions before execution.

**Examples**:
- ✅ **RemediationApprovalRequest**: Operator approves high-risk remediations
- ❌ **RemediationRequest**: Auto-generated by system (no approval needed)

**Test**: Ask "Does this require human approval before proceeding?"
- If YES → Webhook required
- If NO → No webhook needed

---

### **Criterion 4: Override Actions**

**When**: Users override controller-determined state based on operational judgment.

**Examples**:
- ✅ **WorkflowExecution**: Operator overrides failure block to retry execution
- ❌ **NotificationRequest**: No override capability (automated processing)

**Test**: Ask "Can operators override the controller's decision?"
- If YES → Webhook required
- If NO → No webhook needed

---

## 📋 **Detailed Use Case Specifications**

### **Use Case 1: WorkflowExecution Block Clearance**

**Business Requirement**: [BR-WE-013](../../requirements/BR-WE-013-audit-tracked-block-clearing.md)

**Scenario**: Workflow execution failed (`wasExecutionFailure: true`), blocking future executions. Operator investigates, fixes root cause, and clears block.

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually patches `status.blockClearanceRequest`
2. ✅ **SOC2 CC8.1**: Must record WHO cleared block for audit trail
3. ✅ **Override Action**: Operator overrides controller's failure block
4. ✅ **Operational Decision**: Clearance requires human judgment (not automated)

**Status Fields Requiring Authentication**:
- **Operator Input** (unauthenticated): `status.blockClearanceRequest`
  - `clearReason`: Operator's explanation
  - `requestedAt`: Request timestamp
- **Webhook Output** (authenticated): `status.blockClearance`
  - `clearedBy`: Authenticated user from K8s auth context
  - `clearedAt`: Server-side timestamp
  - `clearReason`: Copied from request
  - `clearMethod`: "KubernetesAdmissionWebhook"

**Controller-Managed Fields** (NO webhook):
- `phase`, `message`, `conditions`, `consecutiveFailures`, `nextAllowedExecution`

**Implementation Owner**: WE Team

**Timeline**: 3-4 days (including shared library development)

**Reference**: [ADR-051](./ADR-051-operator-sdk-webhook-scaffolding.md)

---

### **Use Case 2: RemediationApprovalRequest Approval Decisions**

**Business Requirement**: [ADR-040](./ADR-040-remediation-approval-request-architecture.md) (if exists)

**Scenario**: High-risk remediation requires human approval before execution. Operator reviews context, makes decision, approves/rejects.

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually patches `status.approvalRequest`
2. ✅ **SOC2 CC8.1**: Must record WHO approved/rejected remediation
3. ✅ **Approval Workflow**: Explicit human approval required for high-risk actions
4. ✅ **Operational Decision**: Approval requires human risk assessment

**Status Fields Requiring Authentication**:
- **Operator Input** (unauthenticated): `status.approvalRequest`
  - `decision`: "Approved" | "Rejected"
  - `decisionMessage`: Operator's rationale
  - `requestedAt`: Request timestamp
- **Webhook Output** (authenticated):
  - `decision`: Copied from request
  - `decidedBy`: Authenticated user from K8s auth context
  - `decidedAt`: Server-side timestamp
  - `decisionMessage`: Copied from request

**Controller-Managed Fields** (NO webhook):
- `phase`, `conditions`, `remediationStatus`, etc.

**Implementation Owner**: RO Team

**Timeline**: 2-3 days (reuses shared library from WE implementation)

**Reference**: [ADR-051](./ADR-051-operator-sdk-webhook-scaffolding.md)

---

### **Use Case 3: NotificationRequest Cancellation Attribution** (v1.1)

**Business Requirement**: SOC2 CC8.1 Attribution for notification cancellations

**Scenario**: Operator manually cancels notification delivery by deleting NotificationRequest CRD (e.g., issue manually resolved, notification no longer needed).

**Why Webhook Required**:
1. ✅ **Manual Intervention**: Operator manually deletes NotificationRequest CRD
2. ✅ **SOC2 CC8.1**: Must record WHO cancelled the notification
3. ✅ **Override Action**: Operator cancels automated notification delivery
4. ✅ **Operational Decision**: Cancellation requires human judgment

**Operation**:
```bash
kubectl delete notificationrequest <nr-name> -n <namespace>
```

**Metadata Fields Requiring Authentication**:
- **Kubernetes Sets**: `metadata.deletionTimestamp` (on DELETE operation)
- **Webhook Captures**: Authenticated user identity before allowing deletion
- **Finalizer**: Webhook uses finalizer to intercept DELETE and emit audit event

**Audit Event**: `notification.request.cancelled` (NEW event type - requires DD-AUDIT-003 v1.4)

**Event Data**:
```json
{
  "notification_request_id": "nr-approval-rr-123",
  "remediation_request_id": "rr-oomkill-abc123",
  "notification_type": "approval",
  "cancelled_by": {
    "username": "operator@example.com",
    "uid": "k8s-user-uuid",
    "groups": ["platform-admins"]
  },
  "cancellation_reason": "Issue manually resolved",
  "notification_phase": "Pending"
}
```

**Webhook Type**: **ValidatingWebhookConfiguration** (DELETE operation)

**Implementation Pattern**:
1. Webhook intercepts DELETE operation
2. Extract authenticated user from `req.UserInfo`
3. Emit `notification.request.cancelled` audit event with authenticated actor
4. Allow DELETE to proceed (remove finalizer)

**Implementation Owner**: Notification Team

**Timeline**: 1-2 days (reuses shared library)

**Reference**: [TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md](../../development/SOC2/TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md)

---

## 🚫 **Anti-Patterns - When NOT to Use Webhooks**

### **❌ Anti-Pattern 1: Controller-Only Status Updates**

**Scenario**: CRD status fields are ONLY updated by the controller (no manual intervention).

**Example**: `SignalProcessing` status
- `phase`: Controller manages signal processing lifecycle
- `lastProcessedTime`: Controller records processing timestamp
- `conditions`: Controller reports validation status

**Why No Webhook**: No human operator interaction → No authentication needed

**Correct Approach**: Controller updates status directly (no webhook)

---

### **❌ Anti-Pattern 2: Spec-Only CRDs (No Status Subresource)**

**Scenario**: CRD has no status subresource or status is managed entirely by controller.

**Example**: Configuration-only CRDs
- No user-modifiable status fields
- Only spec fields (configuration)
- Status (if present) is read-only and controller-managed

**Why No Webhook**: No manual status updates → No authentication needed

**Correct Approach**: No webhook (status is controller-managed or absent)

---

### **❌ Anti-Pattern 3: System-Generated Fields**

**Scenario**: Status fields are system-generated (not user-provided).

**Example**: `AIAnalysis` investigation results
- `investigationSummary`: Auto-generated by HolmesGPT AI
- `rootCauseAnalysis`: AI-generated analysis
- `workflowRecommendation`: AI-selected workflow

**Why No Webhook**: No human input → No authentication needed (automated AI investigation)

**Correct Approach**: Controller updates status directly (no webhook)

---

### **❌ Anti-Pattern 4: Using Webhooks for Validation Only**

**Scenario**: Webhook used ONLY for field validation (not authentication).

**Example**: Validating `clearReason` length without capturing user identity

**Why Wrong**: Use CRD OpenAPI validation or ValidatingWebhookConfiguration instead

**Correct Approach**:
- **Simple validation**: Use OpenAPI schema validation in CRD
- **Complex validation**: Use ValidatingWebhookConfiguration (no mutation)
- **Authentication**: Use MutatingWebhookConfiguration (our pattern)

---

## 🔧 **Implementation Decision Tree**

```
Does CRD have a status subresource?
    ↓ NO → No webhook needed
    ↓ YES
    ↓
Do human operators manually modify status fields?
    ↓ NO → No webhook needed (controller-only)
    ↓ YES
    ↓
Does change require capturing authenticated user identity?
    ↓ NO → Consider annotations or spec fields instead
    ↓ YES
    ↓
Is this for SOC2 compliance (CC8.1 Attribution)?
    ↓ YES → Webhook REQUIRED ✅
    ↓ NO
    ↓
Is this an approval workflow or override action?
    ↓ YES → Webhook REQUIRED ✅
    ↓ NO
    ↓
    → Reconsider design - webhook may not be necessary
```

---

## 📚 **Implementation Checklist for Teams**

### **Phase 1: Requirements Validation** (1 hour)

- [ ] Verify CRD meets at least ONE decision criterion
- [ ] Identify specific status fields requiring authentication
- [ ] Document business requirement (BR-XXX-XXX format)
- [ ] Map to SOC2 controls if applicable
- [ ] Review [DD-WEBHOOK-001](./DD-WEBHOOK-001-crd-webhook-requirements-matrix.md) (this document)

### **Phase 2: Shared Library Review** (1 hour)

- [ ] Review `pkg/authwebhook` library implementation
- [ ] Understand `Authenticator.ExtractUser()` interface
- [ ] Understand `ValidateReason()` and `ValidateTimestamp()` helpers
- [ ] Review [ADR-051](./ADR-051-operator-sdk-webhook-scaffolding.md)

### **Phase 3: CRD Schema Changes** (2 hours)

- [ ] Add operator input field (e.g., `status.approvalRequest`)
- [ ] Add webhook output fields (e.g., `decidedBy`, `decidedAt`)
- [ ] Regenerate CRD manifests (`make manifests`)
- [ ] Update API documentation

### **Phase 4: Webhook Implementation** (8 hours)

- [ ] Scaffold webhook using operator-sdk
  ```bash
  kubebuilder create webhook \
      --group <group> \
      --version v1alpha1 \
      --kind <Kind> \
      --defaulting \
      --programmatic-validation
  ```
- [ ] Implement `Default()` method (mutation + authentication)
- [ ] Implement `ValidateUpdate()` method (mutual exclusion validation)
- [ ] Implement `isControllerServiceAccount()` helper
- [ ] Add controller bypass logic (both methods)
- [ ] Add mutual exclusion validation
- [ ] Wire webhook in `main.go`

### **Phase 5: Testing** (8 hours)

- [ ] Write 18+ unit tests
  - Authentication extraction
  - Controller bypass (both methods)
  - Mutual exclusion (bidirectional)
  - Validation logic
- [ ] Write 3+ integration tests (envtest)
- [ ] Write 2+ E2E tests (Kind cluster)
- [ ] Verify audit events emitted

### **Phase 6: SOC2 Compliance Validation** (4 hours)

- [ ] Document SOC2 control mapping (CC8.1, CC7.3, CC7.4, CC4.2)
- [ ] Verify audit trail completeness
- [ ] Test unauthorized access rejection
- [ ] Document compliance evidence

### **Phase 7: Documentation** (2 hours)

- [ ] Update service BUSINESS_REQUIREMENTS.md
- [ ] Create implementation plan document
- [ ] Update operator runbooks
- [ ] Document troubleshooting steps

---

## 🎯 **Team Responsibility Matrix (v1.2 - Consolidated Webhook)**

| CRD | Webhook Needed? | Handler Owner | Implementation Phase | Timeline | Dependencies |
|-----|----------------|---------------|----------------------|----------|--------------|
| **WorkflowExecution** | ✅ YES | Webhook Team | Phase 2 (Day 2) | 1 day | Phase 1 complete |
| **RemediationApprovalRequest** | ✅ YES | Webhook Team | Phase 3 (Day 3) | 1 day | Phase 1 complete |
| **NotificationRequest** (v1.1) | ✅ YES | Webhook Team | Phase 4 (Day 4) | 1 day | Phase 1 complete |
| **SignalProcessing** | ❌ NO | N/A | N/A | N/A | N/A (K8s enrichment automated) |
| **AIAnalysis** | ❌ NO | N/A | N/A | N/A | N/A (AI investigation automated) |
| **RemediationRequest** | ❌ NO | N/A | N/A | N/A | N/A (routing automated) |

**Note**: Workflow CRUD attribution (Data Storage Team) uses externalized authorization via sidecar, not CRD webhook. See `DD-AUTH-003` for implementation.

**Single Webhook Service Ownership**:
- **Webhook Team**: Implements unified `kubernaut-auth-webhook` service
- **Phase 1 (Day 1)**: Common authentication logic (`pkg/authwebhook/auth/common.go`)
- **Phase 2-4 (Days 2-4)**: CRD-specific handlers (one per day)
- **Phase 5-6 (Days 5-6)**: Integration + E2E testing + documentation

---

## 📅 **Implementation Timeline - V1.2 (Consolidated Webhook)**

**V1.0 (December 2025)**: WorkflowExecution + RemediationApprovalRequest
**V1.1 (January 2026)**: NotificationRequest (Week 2-3)
**V1.2 (January 2026)**: **ARCHITECTURE UPDATE** - Single consolidated webhook deployment

**Note**: Workflow CRUD attribution uses externalized authorization via sidecar (DD-AUTH-003), not CRD webhook

### **Consolidated Implementation (5-6 days) - v1.2**

**See Comprehensive Plans**:
- **[WEBHOOK_IMPLEMENTATION_PLAN.md](../../development/SOC2/WEBHOOK_IMPLEMENTATION_PLAN.md)**: Detailed roadmap with APDC-TDD methodology
- **[WEBHOOK_TEST_PLAN.md](../../development/SOC2/WEBHOOK_TEST_PLAN.md)**: 95 tests (70 unit + 11 integration + 14 E2E)

### **High-Level Timeline**

**Day 1: Webhook Server Foundation** (Webhook Team)
- Create `cmd/authwebhook/main.go` (single webhook server)
- Implement `pkg/authwebhook/auth/common.go` (shared authentication logic)
- Write 10 unit tests for common auth
- Setup TLS certificates (cert-manager)
- **Deliverable**: Webhook server runs, common auth tested ✅

**Day 2: WorkflowExecution Handler** (Webhook Team)
- Implement `pkg/authwebhook/workflowexecution_handler.go`
- Wire handler to webhook server (`/mutate-workflowexecution`)
- Write 20 unit tests + 3 integration tests
- Update WE controller to detect `clearedBy` field
- **Deliverable**: WE attribution working end-to-end ✅

**Day 3: RemediationApprovalRequest Handler** (Webhook Team)
- Implement `pkg/authwebhook/remediationapprovalrequest_handler.go`
- Wire handler to webhook server (`/mutate-remediationapprovalrequest`)
- Write 20 unit tests + 3 integration tests
- Update RAR controller to detect `approvedBy`/`rejectedBy` fields
- **Deliverable**: RAR attribution working end-to-end ✅

**Day 4: NotificationRequest Handler** (Webhook Team)
- Implement `pkg/authwebhook/notificationrequest_handler.go`
- Wire handler to webhook server (`/validate-notificationrequest-delete`)
- Write 20 unit tests + 3 integration tests
- Update NR controller to detect cancellation annotations
- **Deliverable**: NR attribution working end-to-end ✅

**Day 5-6: Integration + E2E + Documentation** (Webhook Team)
- E2E tests: Complete flows for all 3 CRDs (14 tests)
- Performance validation: Webhook latency < 50ms
- Security validation: TLS + RBAC + NetworkPolicy
- Documentation: Runbooks, troubleshooting guides
- SOC2 compliance validation: 100% attribution
- **Deliverable**: Production-ready webhook service ✅

**Note**: Workflow CRUD attribution (Data Storage Team) requires externalized authorization via sidecar. See `DD-AUTH-003-externalized-authorization-sidecar.md` for implementation plan.

---

## 🔐 **SOC2 Compliance Requirements**

### **For CRDs Requiring Webhooks**

All webhooks MUST satisfy these SOC2 controls:

| Control | Requirement | Implementation | Validation |
|---------|-------------|----------------|------------|
| **CC8.1** - Attribution | Capture authenticated user identity | `req.UserInfo.Username` + `req.UserInfo.UID` | Audit events show real user |
| **CC7.3** - Immutability | Preserve original records (no deletion) | Original failed CRD preserved | Records not deleted on clearance |
| **CC7.4** - Completeness | No gaps in audit trail | All auth events recorded | 100% audit event coverage |
| **CC4.2** - Change Tracking | Track WHO made changes | Authenticated actor in audit event | User identity in all events |
| **CC6.1** - Integrity | Prevent status field forgery | Mutual exclusion validation | Users cannot modify controller fields |

**Compliance Validation Checklist**:
- [ ] All authenticated fields populated from K8s auth context (not user input)
- [ ] Audit events emitted for every authenticated operation
- [ ] Original CRD records preserved (not deleted)
- [ ] Unauthorized users rejected via RBAC
- [ ] Mutual exclusion prevents field forgery

---

## 🚨 **Must Gather - Webhook Troubleshooting**

### **Diagnostic Information for Webhook Issues**

**When webhook authentication fails, collect**:

1. **Webhook Logs**:
   ```bash
   kubectl logs -n kubernaut-system deployment/workflowexecution-controller | grep "webhook"
   ```

2. **Admission Request Details**:
   ```bash
   kubectl get events -n <namespace> --field-selector involvedObject.name=<crd-name>
   ```

3. **User Authentication Context**:
   ```bash
   kubectl auth can-i update workflowexecutions/status --as=<username> -n <namespace>
   ```

4. **Webhook Configuration**:
   ```bash
   kubectl get mutatingwebhookconfigurations -o yaml | grep workflowexecution
   ```

5. **Certificate Validity**:
   ```bash
   kubectl get certificate -n kubernaut-system <webhook-cert-name> -o yaml
   ```

6. **ServiceAccount Bypass Check**:
   ```bash
   # Check if request is from controller SA
   kubectl get events -n <namespace> | grep "Bypassing webhook for controller ServiceAccount"
   ```

7. **Audit Events**:
   ```bash
   # Verify audit events recorded
   kubectl get auditevents -l event_type=workflowexecution.block.cleared
   ```

**Must Gather Script** (for support tickets):
```bash
#!/bin/bash
# must-gather-webhook.sh
NAMESPACE=${1:-kubernaut-system}
CRD_NAME=${2}

echo "=== Webhook Logs ==="
kubectl logs -n $NAMESPACE deployment/workflowexecution-controller --tail=100

echo "=== CRD Events ==="
kubectl get events -n $NAMESPACE --field-selector involvedObject.name=$CRD_NAME

echo "=== Webhook Config ==="
kubectl get mutatingwebhookconfigurations -o yaml | grep -A 20 workflowexecution

echo "=== Certificate Status ==="
kubectl get certificate -n $NAMESPACE -o wide

echo "=== Audit Events ==="
kubectl get auditevents -l event_type=workflowexecution.block.cleared --sort-by=.metadata.creationTimestamp | tail -10
```

---

## 📚 **References**

### **Authoritative Documents**

1. **[ADR-051: Operator-SDK Webhook Scaffolding](./ADR-051-operator-sdk-webhook-scaffolding.md)** - HOW to implement webhooks
2. **[WEBHOOK_CONTROLLER_BYPASS_PATTERN_DEC_20_2025.md](../handoff/WEBHOOK_CONTROLLER_BYPASS_PATTERN_DEC_20_2025.md)** - Controller bypass pattern
3. **[BR-WE-013: Audit-Tracked Block Clearing](../requirements/BR-WE-013-audit-tracked-block-clearing.md)** - WE use case
4. **[INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md](../handoff/INDEPENDENT_WEBHOOK_NOTIFICATION_TO_RO_TEAM_DEC_20_2025.md)** - RO team notification

### **External References**

5. [SOC2 Trust Services Criteria](https://www.aicpa.org/interestareas/frc/assuranceadvisoryservices/socforserviceorganizations)
6. [Kubernetes Admission Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)

---

## ✅ **Success Criteria**

This DD is successfully implemented when:

- ✅ All teams understand WHEN webhooks are required (decision criteria)
- ✅ All teams know HOW to implement webhooks (ADR-051 reference)
- ✅ WE webhook implemented and SOC2 compliant
- ✅ RO webhook implemented and SOC2 compliant
- ✅ Shared library (`pkg/authwebhook`) reused across both webhooks
- ✅ Must gather procedures documented and tested
- ✅ No CRDs have unnecessary webhooks (anti-patterns avoided)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| **1.2** | **2026-01-06** | **ARCHITECTURE UPDATE**: Single consolidated webhook deployment (`kubernaut-auth-webhook`) with multiple handlers instead of 3 separate webhooks. Added consolidated architecture section with benefits (66% memory reduction, 3× faster deployments, guaranteed consistency). Updated team responsibility matrix for unified Webhook Team ownership. Updated implementation timeline to 5-6 days consolidated approach. Added comprehensive [implementation plan](../../development/SOC2/WEBHOOK_IMPLEMENTATION_PLAN.md) and [test plan](../../development/SOC2/WEBHOOK_TEST_PLAN.md) references. Updated all DD-AUTH-002 references to DD-AUTH-003 (sidecar pattern supersedes middleware). |
| 1.1 | 2026-01-06 | Added NotificationRequest (DELETE attribution). Workflow CRUD uses externalized authorization via sidecar (DD-AUTH-003), not CRD webhook. |
| 1.0.2 | 2025-12-20 | **CRITICAL**: Removed KubernetesExecution CRD (deprecated 2025-10-19, never implemented, replaced by Tekton Pipelines). Verified all CRDs against authoritative BR documents. Added deprecation note referencing [DEPRECATED.md](../../services/crd-controllers/04-kubernetesexecutor/DEPRECATED.md). Updated examples with actual CRDs: SignalProcessing, AIAnalysis, RemediationRequest, NotificationRequest. |
| 1.0.1 | 2025-12-20 | Fixed CRD names to match actual kubernaut CRDs: KubernetesExecution (not KubernetesExecutor), SignalProcessing, AIAnalysis, NotificationRequest (removed invented CRDs: WorkflowDefinition, AlertForwarder, DataStorage). |
| 1.0 | 2025-12-20 | Initial DD: CRD webhook requirements matrix. Decision criteria for WHEN/WHY webhooks are needed. Team responsibility matrix. Implementation checklist. SOC2 compliance requirements. Must gather procedures. |

---

**Document Status**: ✅ **AUTHORITATIVE**
**Version**: 1.2
**Authority**: Decision criteria for all CRD webhook implementations
**Next Review**: 2026-02-06 (1 month - after consolidated webhook v1.2 deployment)

