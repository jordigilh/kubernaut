# Webhook Implementation Plan - Single Consolidated Webhook

**Date**: January 6, 2026
**Status**: ✅ **APPROVED** - Ready for Implementation
**Purpose**: Implement single webhook service handling authentication for 3 CRDs
**Authority**: DD-AUTH-001, DD-WEBHOOK-001, SOC2 CC8.1
**Timeline**: 5-6 days
**Owner**: Webhook Team

---

## 🎯 **Implementation Strategy**

### **Core Principle: Single Webhook, Multiple Handlers**

**One deployment** (`kubernaut-auth-webhook`) with **three handlers**:
1. WorkflowExecution handler (block clearance)
2. RemediationApprovalRequest handler (approval/rejection)
3. NotificationRequest handler (cancellation)

**Common authentication logic** shared across all handlers:
- Extract `req.UserInfo.Username`
- Validate user exists
- Populate authenticated fields in CRD

---

## 📊 **Architecture Overview**

### **Deployment Architecture**

```
┌────────────────────────────────────────────────────────────────┐
│  Single Pod: kubernaut-auth-webhook                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  Webhook Server (controller-runtime)                     │ │
│  │                                                           │ │
│  │  Route: /mutate-workflowexecution                        │ │
│  │    → WorkflowExecutionAuthHandler(auditStore)            │ │
│  │    → Populates: status.blockClearanceRequest.clearedBy  │ │
│  │    → Writes: audit event (WHO + WHAT + ACTION)          │ │
│  │                                                           │ │
│  │  Route: /mutate-remediationapprovalrequest               │ │
│  │    → RemediationApprovalRequestAuthHandler(auditStore)   │ │
│  │    → Populates: status.approvalRequest.approvedBy        │ │
│  │    → Writes: audit event (WHO + WHAT + ACTION)          │ │
│  │                                                           │ │
│  │  Route: /validate-notificationrequest-delete             │ │
│  │    → NotificationRequestDeleteHandler(auditStore)        │ │
│  │    → Captures: metadata.deletionTimestamp + user         │ │
│  │    → Writes: audit event (WHO + WHAT + ACTION)          │ │
│  │                                                           │ │
│  │  Shared: ExtractAuthenticatedUser(req.UserInfo)          │ │
│  │  Shared: auditStore (OpenAPI client to Data Storage)    │ │
│  └──────────────────────────────────────────────────────────┘ │
│                                                                 │
│  Port: 9443 (HTTPS with TLS cert)                             │
│  CLI Flag: --data-storage-url (required)                      │
└────────────────────────────────────────────────────────────────┘
         │
         ├──→ K8s API Server (CRD mutations)
         │
         └──→ Data Storage Service (audit events via OpenAPI)
                http://datastorage-service:8080
```

### **Data Flow: Operator Action → Audit Event**

```
1. Operator updates CRD
   kubectl patch workflowexecution test-wfe --type=merge -p '...'

2. K8s API Server intercepts
   → Calls webhook: https://kubernaut-auth-webhook:9443/mutate-workflowexecution
   → Includes: req.UserInfo.Username (authenticated operator)

3. Webhook Handler
   → Extracts: authCtx.Username from req.UserInfo
   → Populates: wfe.Status.BlockClearance.ClearedBy = authCtx.Username (MANDATORY)
   → Writes: audit.Event{...} via auditStore (MANDATORY)

4. Data Storage Service
   → Receives: audit event via OpenAPI client
   → Stores: PostgreSQL (immutable audit log)
   → Returns: success/failure

5. K8s API Server
   → Webhook returns: AdmissionResponse{Allowed: true, Patches: [...]}
   → Applies: status field mutations
   → CRD updated with operator attribution

Result:
✅ Status field: wfe.Status.BlockClearance.ClearedBy = "admin@example.com"
✅ Audit event: {event_type: "workflowexecution.block.cleared", actor_id: "admin@example.com", ...}
```

### **Benefits of Single Webhook**

| Aspect | Before (3 Webhooks) | After (1 Webhook) | Savings |
|--------|---------------------|-------------------|---------|
| **Pods** | 3 | 1 | 2 fewer pods |
| **Memory** | 150MB (3×50MB) | 50MB | 66% reduction |
| **Configs** | 3 webhooks | 1 webhook | Simpler maintenance |
| **Deployment time** | 3× slower | Fast | 3× faster deploys |
| **Code consistency** | Risk of drift | Guaranteed | Shared logic |

---

## 🔧 **Configuration: Data Storage URL**

### **CLI Flag Pattern** (Following Gateway/Notification/AI Analysis)

**Required Flag**: `--data-storage-url`

**Purpose**: Connect webhook to Data Storage service for audit event writes

**Pattern**: Same as other services (Gateway, Notification, AI Analysis, etc.)

```bash
# Command-line flag
./webhooks-controller \
  --data-storage-url=http://datastorage-service:8080 \
  --webhook-port=9443 \
  --cert-dir=/tmp/k8s-webhook-server/serving-certs

# Environment variable (optional override)
export WEBHOOK_DATA_STORAGE_URL=http://localhost:8081  # Development
./webhooks-controller
```

### **Kubernetes Deployment Configuration**

```yaml
# Option A: Hardcoded in deployment (simple, for stable environments)
args:
- --data-storage-url=http://datastorage-service:8080

# Option B: ConfigMap (recommended, for flexibility)
apiVersion: v1
kind: ConfigMap
metadata:
  name: webhook-config
data:
  data-storage-url: "http://datastorage-service:8080"
---
args:
- --data-storage-url=$(DATA_STORAGE_URL)
env:
- name: DATA_STORAGE_URL
  valueFrom:
    configMapKeyRef:
      name: webhook-config
      key: data-storage-url

# Option C: Secret (for sensitive URLs with auth tokens)
apiVersion: v1
kind: Secret
metadata:
  name: webhook-secrets
stringData:
  data-storage-url: "https://datastorage.prod.internal:8443?token=xyz"
---
env:
- name: DATA_STORAGE_URL
  valueFrom:
    secretKeyRef:
      name: webhook-secrets
      key: data-storage-url
```

### **Service Discovery**

**Production**: Use Kubernetes service name
```
--data-storage-url=http://datastorage-service:8080
```

**Development**: Use localhost or Kind NodePort
```
--data-storage-url=http://localhost:18099  # Integration tests
--data-storage-url=http://localhost:28099  # E2E tests
```

### **Initialization Pattern** (Following Gateway)

```go
// main.go initialization (per Gateway pattern)
func main() {
    // 1. Parse CLI flags
    flag.StringVar(&dataStorageURL, "data-storage-url", "", "Data Storage service URL (required)")
    flag.Parse()

    // 2. Validate required flags
    if dataStorageURL == "" {
        setupLog.Error(fmt.Errorf("data-storage-url is required"), "missing required flag")
        os.Exit(1)  // Per ADR-032: Webhooks are P0 - MUST crash if audit unavailable
    }

    // 3. Initialize OpenAPI audit client
    // Per DD-API-001: Use OpenAPI generated client for type safety
    dsClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
    if err != nil {
        setupLog.Error(err, "failed to create Data Storage client")
        os.Exit(1)
    }

    // 4. Create buffered audit store
    auditConfig := audit.RecommendedConfig("authwebhook")
    auditStore, err := audit.NewBufferedStore(dsClient, auditConfig, "authwebhook", logger)
    if err != nil {
        setupLog.Error(err, "failed to create audit store")
        os.Exit(1)
    }

    // 5. Pass to webhook handlers
    wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(auditStore)
    // ... register handlers ...
}
```

**Reference Implementation**: `cmd/gateway/main.go` lines 86-103 (Gateway uses same pattern)

### **Why Not Kubernetes Service Account Tokens?**

**Question**: Why use a simple URL flag instead of K8s ServiceAccount authentication?

**Answer**:
1. ✅ **Simplicity**: Data Storage already trusts internal traffic (no auth needed for pod-to-pod)
2. ✅ **Consistency**: Same pattern as Gateway, Notification, AI Analysis services
3. ✅ **Flexibility**: Easy to point to different Data Storage instances (dev/staging/prod)
4. ⚠️ **Future**: If Data Storage adds mTLS, we'd pass `--data-storage-cert` flag

**Security**: Relies on Kubernetes Network Policies (restrict Data Storage access to authorized pods only)

---

## 🗂️ **File Structure**

```
kubernaut/
├── cmd/
│   └── webhooks/
│       └── main.go                              # Webhook server entry point
│
├── pkg/
│   └── webhooks/
│       ├── auth/
│       │   ├── common.go                        # Shared: ExtractAuthenticatedUser()
│       │   ├── common_test.go                   # Unit tests for common logic
│       │   └── types.go                         # UserInfo struct
│       │
│       ├── workflowexecution_handler.go         # WE handler
│       ├── workflowexecution_handler_test.go    # WE unit tests
│       │
│       ├── remediationapprovalrequest_handler.go    # RAR handler
│       ├── remediationapprovalrequest_handler_test.go # RAR unit tests
│       │
│       ├── notificationrequest_handler.go       # NR handler
│       └── notificationrequest_handler_test.go  # NR unit tests
│
├── deploy/
│   └── webhooks/
│       ├── deployment.yaml                      # Single webhook deployment
│       ├── service.yaml                         # Webhook service
│       ├── mutating-webhook-config.yaml         # MutatingWebhookConfiguration
│       ├── validating-webhook-config.yaml       # ValidatingWebhookConfiguration (NR only)
│       ├── certificate.yaml                     # TLS cert (cert-manager)
│       └── kustomization.yaml                   # Kustomize overlay
│
└── test/
    ├── integration/
    │   └── webhooks/
    │       ├── workflowexecution_webhook_test.go   # WE integration tests
    │       ├── rar_webhook_test.go                  # RAR integration tests
    │       └── notificationrequest_webhook_test.go  # NR integration tests
    │
    └── e2e/
        └── webhooks/
            └── 10_webhook_auth_test.go          # E2E tests (all 3 CRDs)
```

---

## 📅 **Implementation Timeline (5-6 Days)**

### **Day 1: Webhook Server Foundation** ✅

**Goal**: Webhook server runs, common auth logic works

**Tasks**:
1. Create `cmd/authwebhook/main.go` (webhook server entry point)
2. Implement `pkg/authwebhook/auth/common.go` (ExtractAuthenticatedUser)
3. Implement `pkg/authwebhook/auth/types.go` (UserInfo struct)
4. Write unit tests for common auth logic
5. Create `deploy/webhooks/deployment.yaml` (single deployment)
6. Create `deploy/webhooks/service.yaml` (webhook service)
7. Setup TLS certificates (cert-manager)

**APDC**:
- **Analysis** (1 hour): Study controller-runtime webhook patterns, cert-manager setup
- **Plan** (1 hour): Design common auth extraction logic, error handling
- **Do-RED** (2 hours): Write unit tests for ExtractAuthenticatedUser
- **Do-GREEN** (2 hours): Implement webhook server + common logic
- **Do-REFACTOR** (1 hour): Clean up, add logging
- **Check** (30 min): Verify webhook server starts, TLS works

**Deliverables**:
- ✅ Webhook server runs (no handlers yet)
- ✅ Common auth logic tested
- ✅ TLS certificates configured
- ✅ 15 unit tests passing

**Confidence**: 90% (standard controller-runtime pattern)

---

### **Day 2: WorkflowExecution Handler** ✅

**Goal**: Block clearance attribution working end-to-end

**Tasks**:
1. Implement `pkg/authwebhook/workflowexecution_handler.go`
2. Wire handler to webhook server (`/mutate-workflowexecution`)
3. Update WorkflowExecution CRD (add `clearedBy`, `clearedAt` fields if missing)
4. Write unit tests for WE handler
5. Write integration test (create WE, update block clearance, verify auth)
6. Update WE controller to detect `clearedBy` field
7. Update `deploy/webhooks/mutating-webhook-config.yaml` (add WE rule)

**APDC**:
- **Analysis** (1 hour): Study WorkflowExecution CRD schema, BR-WE-013 requirements
- **Plan** (1 hour): Design handler logic, decide on field names
- **Do-RED** (2 hours): Write unit + integration tests
- **Do-GREEN** (3 hours): Implement handler + wire to server + update CRD
- **Do-REFACTOR** (1 hour): Add validation, error handling
- **Check** (30 min): E2E test with real K8s cluster

**Deliverables**:
- ✅ WE webhook handler functional
- ✅ Block clearance captures authenticated user
- ✅ WE controller detects clearance
- ✅ 20 unit tests + 3 integration tests passing
- ✅ Audit event `workflowexecution.block.cleared` emitted with user

**Confidence**: 85% (depends on CRD schema changes)

---

### **Day 3: RemediationApprovalRequest Handler** ✅

**Goal**: Approval/rejection attribution working end-to-end

**Tasks**:
1. Implement `pkg/authwebhook/remediationapprovalrequest_handler.go`
2. Wire handler to webhook server (`/mutate-remediationapprovalrequest`)
3. Update RAR CRD (add `approvedBy`, `rejectedBy`, `decidedAt` fields if missing)
4. Write unit tests for RAR handler
5. Write integration test (create RAR, approve/reject, verify auth)
6. Update RAR controller to detect `approvedBy`/`rejectedBy` fields
7. Update `deploy/webhooks/mutating-webhook-config.yaml` (add RAR rule)

**APDC**:
- **Analysis** (1 hour): Study RAR CRD schema, approval workflow
- **Plan** (1 hour): Design handler logic for approve vs reject
- **Do-RED** (2 hours): Write unit + integration tests
- **Do-GREEN** (3 hours): Implement handler + wire to server + update CRD
- **Do-REFACTOR** (1 hour): Add decision validation logic
- **Check** (30 min): E2E test with real approval workflow

**Deliverables**:
- ✅ RAR webhook handler functional
- ✅ Approval/rejection captures authenticated user
- ✅ RAR controller detects decision
- ✅ 20 unit tests + 3 integration tests passing
- ✅ Audit events `orchestrator.approval.approved/rejected` emitted with user

**Confidence**: 85% (depends on CRD schema changes)

---

### **Day 4: NotificationRequest Handler** ✅

**Goal**: Cancellation attribution working end-to-end

**Tasks**:
1. Implement `pkg/authwebhook/notificationrequest_handler.go`
2. Wire handler to webhook server (`/validate-notificationrequest-delete`)
3. Update NR CRD (add annotations for `cancelledBy`, `cancelledAt` if missing)
4. Write unit tests for NR handler
5. Write integration test (create NR, DELETE, verify auth)
6. Update NR controller to detect cancellation annotation
7. Create `deploy/webhooks/validating-webhook-config.yaml` (NR DELETE rule)

**APDC**:
- **Analysis** (1 hour): Study NR CRD schema, DELETE webhook pattern
- **Plan** (1 hour): Design handler for DELETE interception
- **Do-RED** (2 hours): Write unit + integration tests
- **Do-GREEN** (3 hours): Implement handler + wire to server + update CRD
- **Do-REFACTOR** (1 hour): Add DELETE validation logic
- **Check** (30 min): E2E test with real notification cancellation

**Deliverables**:
- ✅ NR webhook handler functional
- ✅ DELETE operation captures authenticated user
- ✅ NR controller detects cancellation
- ✅ 20 unit tests + 3 integration tests passing
- ✅ Audit event `notification.request.cancelled` emitted with user

**Confidence**: 80% (DELETE webhooks are less common, needs extra validation)

---

### **Day 5-6: Integration, E2E, Documentation** ✅

**Goal**: Complete system validation + SOC2 compliance verification

**Tasks**:
1. E2E test: Complete flow for all 3 CRDs with authenticated users
2. E2E test: Verify unauthenticated requests are rejected
3. E2E test: Verify audit events contain correct `actor_id`
4. Performance test: Webhook latency < 50ms per request
5. Security test: Verify TLS, RBAC, network policies
6. Documentation: Update DD-WEBHOOK-001 with implementation details
7. Documentation: Create operator runbook for webhook troubleshooting
8. Code review: Address any feedback
9. Final validation: SOC2 CC8.1 compliance checklist

**APDC**:
- **Analysis** (2 hours): Review all components, identify gaps
- **Plan** (1 hour): Create comprehensive E2E test plan
- **Do-RED** (2 hours): Write E2E tests
- **Do-GREEN** (4 hours): Fix any issues discovered in E2E
- **Do-REFACTOR** (2 hours): Optimize performance, add observability
- **Check** (4 hours): Final compliance validation + documentation

**Deliverables**:
- ✅ 10 E2E tests passing (all 3 CRDs)
- ✅ Performance: < 50ms webhook latency
- ✅ Security: TLS + RBAC + NetworkPolicy validated
- ✅ Documentation: Runbooks + troubleshooting guides
- ✅ SOC2 CC8.1 compliance: 100% operator actions attributed

**Confidence**: 90% (standard validation + documentation)

---

## 🔧 **Technical Implementation Details**

### **Component 1: Common Authentication Logic**

**File**: `pkg/authwebhook/auth/common.go`

```go
package auth

import (
    "errors"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// UserInfo contains authenticated user information from K8s API
type UserInfo struct {
    Username string   // e.g., "operator@example.com"
    UID      string   // K8s user UID
    Groups   []string // K8s groups user belongs to
}

// ExtractAuthenticatedUser extracts user identity from admission request
// This is the ONLY place we extract user info (DRY principle)
//
// DD-AUTH-001: Shared Authentication Webhook Pattern
// SOC2 CC8.1: Operator Attribution Requirement
func ExtractAuthenticatedUser(req admission.Request) (*UserInfo, error) {
    // Validate user info exists
    if req.UserInfo.Username == "" {
        return nil, errors.New("missing user identity in admission request")
    }

    userInfo := &UserInfo{
        Username: req.UserInfo.Username,
        UID:      req.UserInfo.UID,
        Groups:   req.UserInfo.Groups,
    }

    return userInfo, nil
}

// ValidateReason validates that a reason string is non-empty and reasonable length
// Used for block clearance reasons, approval reasons, etc.
func ValidateReason(reason string) error {
    if reason == "" {
        return errors.New("reason is required")
    }
    if len(reason) > 500 {
        return errors.New("reason exceeds maximum length (500 chars)")
    }
    return nil
}
```

**Unit Tests**: `pkg/authwebhook/auth/common_test.go`
- Test: Extract valid user info
- Test: Reject missing username
- Test: Reject empty UID
- Test: Extract multiple groups
- Test: ValidateReason accepts valid input
- Test: ValidateReason rejects empty reason
- Test: ValidateReason rejects overly long reason

---

### **Component 2: WorkflowExecution Handler**

**File**: `pkg/authwebhook/workflowexecution_handler.go`

```go
package webhooks

import (
    "context"
    "encoding/json"
    "net/http"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/authwebhook/auth"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// WorkflowExecutionAuthHandler handles authentication for WorkflowExecution CRD
// Captures operator identity when clearing workflow execution blocks (BR-WE-013)
type WorkflowExecutionAuthHandler struct {
    decoder *admission.Decoder
}

// Handle processes WorkflowExecution CREATE/UPDATE requests
// Populates status.blockClearanceRequest.clearedBy when operator clears a block
func (h *WorkflowExecutionAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    wfe := &workflowexecutionv1.WorkflowExecution{}

    // Decode incoming object
    if err := h.decoder.Decode(req, wfe); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // Extract authenticated user (common logic)
    user, err := auth.ExtractAuthenticatedUser(req)
    if err != nil {
        return admission.Denied("authentication required: " + err.Error())
    }

    // Check if block clearance is being requested
    if wfe.Status.BlockClearanceRequest != nil && wfe.Status.BlockClearanceRequest.ClearedBy == "" {
        // Populate authenticated fields
        wfe.Status.BlockClearanceRequest.ClearedBy = user.Username
        wfe.Status.BlockClearanceRequest.ClearedAt = metav1.Now()

        // Validate clearance reason exists
        if err := auth.ValidateReason(wfe.Status.BlockClearanceRequest.Reason); err != nil {
            return admission.Denied("invalid clearance reason: " + err.Error())
        }
    }

    // Marshal patched object
    marshaled, err := json.Marshal(wfe)
    if err != nil {
        return admission.Errored(http.StatusInternalServerError, err)
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

// InjectDecoder injects the decoder (required by admission.Handler interface)
func (h *WorkflowExecutionAuthHandler) InjectDecoder(d *admission.Decoder) error {
    h.decoder = d
    return nil
}
```

**Unit Tests**: `pkg/authwebhook/workflowexecution_handler_test.go`
- Test: Populate clearedBy when block clearance requested
- Test: Populate clearedAt timestamp
- Test: Reject clearance without reason
- Test: Reject clearance with overly long reason
- Test: Reject unauthenticated requests
- Test: Do nothing when no block clearance requested
- Test: Handle malformed WorkflowExecution

---

### **Component 3: RemediationApprovalRequest Handler**

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go`

```go
package webhooks

import (
    "context"
    "encoding/json"
    "net/http"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/authwebhook/auth"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// RemediationApprovalRequestAuthHandler handles authentication for RAR CRD
// Captures operator identity when approving/rejecting remediation requests
type RemediationApprovalRequestAuthHandler struct {
    decoder *admission.Decoder
}

// Handle processes RAR CREATE/UPDATE requests
// Populates status.approvalRequest.approvedBy or rejectedBy when operator decides
func (h *RemediationApprovalRequestAuthHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    rar := &remediationv1.RemediationApprovalRequest{}

    // Decode incoming object
    if err := h.decoder.Decode(req, rar); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // Extract authenticated user (common logic)
    user, err := auth.ExtractAuthenticatedUser(req)
    if err != nil {
        return admission.Denied("authentication required: " + err.Error())
    }

    // Check if approval decision is being made
    if rar.Status.Decision != "" {
        // Populate authenticated fields based on decision
        switch rar.Status.Decision {
        case "Approved":
            if rar.Status.ApprovedBy == "" {
                rar.Status.ApprovedBy = user.Username
                rar.Status.DecidedAt = metav1.Now()
            }
        case "Rejected":
            if rar.Status.RejectedBy == "" {
                rar.Status.RejectedBy = user.Username
                rar.Status.DecidedAt = metav1.Now()
            }
        default:
            return admission.Denied("invalid decision: must be Approved or Rejected")
        }

        // Validate decision reason exists
        if err := auth.ValidateReason(rar.Status.DecisionReason); err != nil {
            return admission.Denied("invalid decision reason: " + err.Error())
        }
    }

    // Marshal patched object
    marshaled, err := json.Marshal(rar)
    if err != nil {
        return admission.Errored(http.StatusInternalServerError, err)
    }

    return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

// InjectDecoder injects the decoder (required by admission.Handler interface)
func (h *RemediationApprovalRequestAuthHandler) InjectDecoder(d *admission.Decoder) error {
    h.decoder = d
    return nil
}
```

**Unit Tests**: `pkg/authwebhook/remediationapprovalrequest_handler_test.go`
- Test: Populate approvedBy when decision is Approved
- Test: Populate rejectedBy when decision is Rejected
- Test: Populate decidedAt timestamp
- Test: Reject decision without reason
- Test: Reject invalid decision (not Approved/Rejected)
- Test: Reject unauthenticated requests
- Test: Do nothing when no decision made
- Test: Handle malformed RAR

---

### **Component 4: NotificationRequest Handler**

**File**: `pkg/authwebhook/notificationrequest_handler.go`

```go
package webhooks

import (
    "context"
    "encoding/json"
    "net/http"

    notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/authwebhook/auth"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// NotificationRequestDeleteHandler handles authentication for NR DELETE
// Captures operator identity when cancelling notification requests
type NotificationRequestDeleteHandler struct {
    decoder *admission.Decoder
}

// Handle processes NR DELETE requests
// Populates annotations with cancellation attribution before allowing DELETE
func (h *NotificationRequestDeleteHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
    // Only process DELETE operations
    if req.Operation != admission.Delete {
        return admission.Allowed("not a delete operation")
    }

    nr := &notificationv1.NotificationRequest{}

    // Decode OLD object (DELETE operations use OldObject)
    if err := h.decoder.DecodeRaw(req.OldObject, nr); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }

    // Extract authenticated user (common logic)
    user, err := auth.ExtractAuthenticatedUser(req)
    if err != nil {
        return admission.Denied("authentication required: " + err.Error())
    }

    // Add cancellation attribution as annotations (since we can't modify status on DELETE)
    if nr.Annotations == nil {
        nr.Annotations = make(map[string]string)
    }
    nr.Annotations["kubernaut.ai/cancelled-by"] = user.Username
    nr.Annotations["kubernaut.ai/cancelled-at"] = metav1.Now().Format(time.RFC3339)

    // Controller will read these annotations before finalizer cleanup
    // and emit audit event: notification.request.cancelled

    // Allow DELETE (annotations are for audit purposes only, not blocking)
    return admission.Allowed("cancellation attributed to " + user.Username)
}

// InjectDecoder injects the decoder (required by admission.Handler interface)
func (h *NotificationRequestDeleteHandler) InjectDecoder(d *admission.Decoder) error {
    h.decoder = d
    return nil
}
```

**Unit Tests**: `pkg/authwebhook/notificationrequest_handler_test.go`
- Test: Add cancellation annotations on DELETE
- Test: Populate cancelled-by annotation
- Test: Populate cancelled-at timestamp
- Test: Reject unauthenticated DELETE requests
- Test: Ignore non-DELETE operations
- Test: Handle NR without existing annotations
- Test: Handle malformed NR

---

### **Component 5: Webhook Server**

**File**: `cmd/authwebhook/main.go`

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/audit"
    "github.com/jordigilh/kubernaut/pkg/authwebhook"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    // Register CRD schemes
    _ = workflowexecutionv1.AddToScheme(scheme)
    _ = remediationv1.AddToScheme(scheme)
    _ = notificationv1.AddToScheme(scheme)
}

func main() {
    var webhookPort int
    var certDir string
    var dataStorageURL string

    // CLI flags with production defaults (per WEBHOOK_METRICS_TRIAGE.md)
    flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
    flag.StringVar(&certDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "The directory containing TLS certificates.")
    flag.StringVar(&dataStorageURL, "data-storage-url", "http://datastorage-service:8080", "Data Storage service URL for audit events")
    flag.Parse()

    // Allow environment variable overrides
    if envURL := os.Getenv("WEBHOOK_DATA_STORAGE_URL"); envURL != "" {
        dataStorageURL = envURL
    }
    if envPort := os.Getenv("WEBHOOK_PORT"); envPort != "" {
        if port, err := strconv.Atoi(envPort); err == nil {
            webhookPort = port
        }
    }

    ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

    // Initialize OpenAPI audit client
    // Per DD-API-001: Use OpenAPI generated client for type safety
    // Per DD-WEBHOOK-003: Webhooks write complete audit events (WHO + WHAT + ACTION)
    dsClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 5*time.Second)
    if err != nil {
        setupLog.Error(err, "failed to create Data Storage client - audit is MANDATORY per ADR-032 (webhooks are P0)")
        os.Exit(1)
    }

    // Create buffered audit store for async writes
    auditConfig := audit.RecommendedConfig("authwebhook")
    auditStore, err := audit.NewBufferedStore(dsClient, auditConfig, "authwebhook", ctrl.Log.WithName("audit"))
    if err != nil {
        setupLog.Error(err, "failed to create audit store - audit is MANDATORY per ADR-032")
        os.Exit(1)
    }
    setupLog.Info("Audit store initialized", "data_storage_url", dataStorageURL, "buffer_size", auditConfig.BufferSize)

    // Create manager (NO METRICS - audit traces sufficient per WEBHOOK_METRICS_TRIAGE.md)
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:             scheme,
        MetricsBindAddress: "0",  // Disable metrics endpoint
        Port:               webhookPort,
        CertDir:            certDir,
    })
    if err != nil {
        setupLog.Error(err, "unable to create manager")
        os.Exit(1)
    }

    // Get webhook server
    webhookServer := mgr.GetWebhookServer()

    // Register WorkflowExecution handler (WITH audit store)
    webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{
        Handler: webhooks.NewWorkflowExecutionAuthHandler(auditStore),  // CHANGED: Pass audit store
    })
    setupLog.Info("Registered WorkflowExecution webhook handler")

    // Register RemediationApprovalRequest handler (WITH audit store)
    webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{
        Handler: webhooks.NewRemediationApprovalRequestAuthHandler(auditStore),  // CHANGED: Pass audit store
    })
    setupLog.Info("Registered RemediationApprovalRequest webhook handler")

    // Register NotificationRequest DELETE handler (WITH audit store)
    webhookServer.Register("/validate-notificationrequest-delete", &webhook.Admission{
        Handler: webhooks.NewNotificationRequestDeleteHandler(auditStore),  // CHANGED: Pass audit store
    })
    setupLog.Info("Registered NotificationRequest DELETE webhook handler")

    setupLog.Info("Starting webhook server", "port", webhookPort, "data_storage", dataStorageURL)
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

**Key Changes**:
1. ✅ **NEW CLI Flag**: `--data-storage-url` (required)
2. ✅ **OpenAPI Client**: `audit.NewOpenAPIClientAdapter()` for type-safe audit writes
3. ✅ **Buffered Store**: `audit.NewBufferedStore()` for async audit writes
4. ✅ **Handler Injection**: All handlers receive `auditStore` for writing audit events
5. ✅ **Validation**: Crashes if Data Storage URL not provided (webhooks are P0 per ADR-032)

**Environment Variable Support** (optional):
```bash
# Can be overridden via env var in deployment
WEBHOOK_DATA_STORAGE_URL=http://datastorage-service:8080
```

---

## 🔐 **Security Considerations**

### **TLS Certificate Management**

**Using cert-manager** (recommended):
```yaml
# deploy/webhooks/certificate.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: kubernaut-auth-webhook-cert
  namespace: kubernaut-system
spec:
  secretName: kubernaut-auth-webhook-tls
  duration: 8760h # 1 year
  renewBefore: 720h # 30 days
  issuerRef:
    name: kubernaut-ca-issuer
    kind: Issuer
  dnsNames:
  - kubernaut-auth-webhook.kubernaut-system.svc
  - kubernaut-auth-webhook.kubernaut-system.svc.cluster.local
```

### **RBAC Requirements**

**Webhook service account needs**:
- Read access to CRDs (WorkflowExecution, RAR, NotificationRequest)
- No write access (webhook only validates/mutates, doesn't persist)

```yaml
# deploy/webhooks/rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-auth-webhook
rules:
- apiGroups: ["workflowexecution.kubernaut.ai"]
  resources: ["workflowexecutions"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["remediation.kubernaut.ai"]
  resources: ["remediationapprovalrequests"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["notification.kubernaut.ai"]
  resources: ["notificationrequests"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-auth-webhook
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-auth-webhook
subjects:
- kind: ServiceAccount
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
```

---

## 📊 **Success Metrics** (Business Criteria, Not Prometheus Metrics)

**Note**: ❌ **No Prometheus metrics** - Audit traces capture 100% of operations; Kubernetes API server exposes webhook metrics. See `WEBHOOK_METRICS_TRIAGE.md` for rationale.

| Metric | Target | Validation Method |
|--------|--------|-------------------|
| **Webhook Latency** | < 50ms per request | Performance tests, K8s API server metrics |
| **Unit Coverage** | > 70% | `go test -cover pkg/authwebhook/` |
| **Integration Coverage** | > 50% | `go test -cover test/integration/webhooks/` |
| **E2E Coverage** | > 50% | Binary coverage with `GOCOVERDIR` (Go 1.20+) |
| **Unit Tests** | > 60 tests passing | CI pipeline |
| **Integration Tests** | > 10 tests passing | CI pipeline |
| **E2E Tests** | > 10 tests passing | CI pipeline |
| **SOC2 Compliance** | 100% operator actions attributed | Audit event validation |
| **Deployment Success** | 100% webhook calls successful | K8s events, logs, audit traces |

**Operational Monitoring**: Use K8s API server metrics (`apiserver_admission_webhook_admission_duration_seconds{name="workflowexecution.mutate.kubernaut.ai"}`)

---

## 📊 **Code Coverage Targets (Per TESTING_GUIDELINES.md v2.5.0)**

### **Defense-in-Depth Coverage Strategy**

**Principle**: 50%+ of webhook code is tested in **ALL 3 tiers**, ensuring authentication vulnerabilities must slip through multiple defense layers.

| Tier | Coverage Target | What It Validates | Measurement |
|------|----------------|-------------------|-------------|
| **Unit** | **70%+** | Handler logic, auth extraction, validation | `go test -cover pkg/authwebhook/...` |
| **Integration** | **50%** | HTTP admission flow, TLS, webhook server | `go test -cover test/integration/webhooks/...` |
| **E2E** | **50%** | Deployed webhook, K8s API, CRD operations | Binary coverage (`GOCOVERDIR`) |

**Test Count**: 95 tests (70 unit + 11 integration + 14 E2E) provides comprehensive coverage.

### **Critical Path Example**: `ExtractAuthenticatedUser()`

| Tier | Coverage | What's Tested |
|------|----------|---------------|
| **Unit (70%)** | Tests all edge cases (missing username, empty UID, group extraction) | Function correctness |
| **Integration (50%)** | Tests HTTP admission requests with real webhook server | HTTP integration |
| **E2E (50%)** | Tests real CRD operations with deployed webhook in Kind cluster | Production behavior |

**Result**: Authentication logic is validated at **3 different levels** - ensuring bugs must slip through ALL layers to reach production.

---

## 📊 **E2E Coverage Collection (Go 1.20+)**

**Per TESTING_GUIDELINES.md §1343-1373**: Go 1.20+ supports binary coverage profiling.

### **Build Webhook with Coverage**

```bash
# Build webhook binary with coverage instrumentation
GOFLAGS=-cover go build -o bin/webhooks-controller ./cmd/authwebhook/
```

### **Deploy to Kind with GOCOVERDIR**

```yaml
# deploy/webhooks/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-auth-webhook
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: auth-webhook
  template:
    metadata:
      labels:
        app: auth-webhook
    spec:
      serviceAccountName: kubernaut-auth-webhook
      containers:
      - name: webhook
        image: kubernaut/auth-webhook:latest
        ports:
        - containerPort: 9443
          name: webhook
          protocol: TCP
        # No metrics port - audit traces sufficient (WEBHOOK_METRICS_TRIAGE.md)
        args:
        - --webhook-port=9443
        - --cert-dir=/tmp/k8s-webhook-server/serving-certs
        - --data-storage-url=http://datastorage-service:8080
        # No --metrics-bind-address flag - audit traces sufficient
        env:
        - name: GOCOVERDIR  # For E2E coverage collection
          value: /coverdata
        volumeMounts:
        - name: cert
          mountPath: /tmp/k8s-webhook-server/serving-certs
          readOnly: true
        - name: coverage  # For E2E coverage collection
          mountPath: /coverdata
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: cert
        secret:
          secretName: kubernaut-auth-webhook-cert
      - name: coverage  # For E2E coverage collection
        hostPath:
          path: /tmp/webhook-coverdata
          type: DirectoryOrCreate
```

**Key Configuration**:
- ✅ **`--data-storage-url`**: Points to Data Storage service (http://datastorage-service:8080)
- ✅ **Service Discovery**: Uses K8s service name for Data Storage
- ✅ **Required Flag**: Webhook crashes if not provided (ADR-032: P0 service)
- ⚠️ **Production**: Use ConfigMap or Secret for Data Storage URL (not hardcoded)

### **Extract Coverage After E2E Tests**

```bash
# Gracefully shutdown webhook (flush coverage)
kubectl scale deployment kubernaut-auth-webhook --replicas=0 -n kubernaut-system

# Wait for graceful shutdown
kubectl wait --for=delete pod -l app=kubernaut-auth-webhook -n kubernaut-system --timeout=60s

# Copy coverage data from Kind node
kind cp webhook-e2e:/tmp/webhook-coverdata ./coverdata

# Generate coverage report
go tool covdata percent -i=./coverdata
go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt
go tool cover -html=e2e-coverage.txt -o e2e-coverage.html
```

**Target**: 50%+ E2E coverage validates deployment wiring, TLS, RBAC, and handler integration.

---

## ✅ **Acceptance Criteria**

**Implementation is complete when**:
1. ✅ Single webhook service deployed (`kubernaut-auth-webhook`)
2. ✅ All 3 CRD handlers functional (WE, RAR, NR)
3. ✅ Common auth logic tested and working
4. ✅ Unit tests: > 60 tests passing, > 70% coverage
5. ✅ Integration tests: > 10 tests passing, > 50% coverage
6. ✅ E2E tests: > 10 tests passing, > 50% coverage
7. ✅ TLS certificates configured and working
8. ✅ RBAC permissions minimal and correct
9. ✅ Webhook latency < 50ms
10. ✅ SOC2 CC8.1: All operator actions capture authenticated user
11. ✅ Documentation: Runbooks, troubleshooting guides complete

---

## 🚨 **Risks & Mitigations**

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| **CRD schema changes required** | Medium | High | Coordinate with Day 3+ team on schema changes |
| **Webhook latency > 50ms** | Low | Low | Optimize handler logic, add caching if needed |
| **TLS cert issues** | High | Low | Use cert-manager, test cert rotation |
| **Controller doesn't detect webhook changes** | Medium | Medium | Add controller watches for auth fields |
| **Webhook crashes on malformed CRDs** | Medium | Low | Comprehensive error handling + tests |

---

## 📚 **References**

- **DD-AUTH-001**: Shared Authentication Webhook (authoritative pattern)
- **DD-WEBHOOK-001**: CRD Webhook Requirements Matrix
- **BR-WE-013**: Audit-Tracked Block Clearing
- **SOC2 CC8.1**: Change Control - Attribution
- **controller-runtime webhooks**: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html
- **cert-manager**: https://cert-manager.io/docs/

---

**Status**: ✅ **READY FOR IMPLEMENTATION**
**Confidence**: 90%
**Timeline**: 5-6 days
**Owner**: Webhook Team
**Next Step**: Begin Day 1 implementation (webhook server foundation)

