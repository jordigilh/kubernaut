# ADR-051: Operator-SDK Webhook Scaffolding Pattern

**Date**: December 20, 2025
**Status**: ✅ Approved
**Purpose**: Establish standard pattern for CRD webhooks using operator-sdk scaffolding
**Rationale**: Leverage production-ready operator-sdk code generation while enabling code reuse through shared libraries
**Authority**: AUTHORITATIVE for all CRD webhook implementations

---

## 🎯 **DECISION**

**All CRD webhooks in Kubernaut SHALL use operator-sdk (Kubebuilder) scaffolding with a shared authentication library pattern.**

**Enforcement**: Mandatory for all CRD webhooks requiring user authentication (starting with WE and RO services)

**Pattern**:
- **Independent webhooks** per CRD controller (each team owns their webhook)
- **Shared library** (`pkg/authwebhook`) for common authentication logic
- **Operator-SDK scaffolding** for webhook boilerplate code generation

---

## 📋 **CONTEXT**

### **Problem Statement**

Two services require authenticated user identity for CRD status updates:

1. **WorkflowExecution (BR-WE-013)**: Track WHO cleared a `PreviousExecutionFailed` block
2. **RemediationOrchestrator (ADR-040)**: Track WHO approved/rejected a remediation

**SOC2 Requirement**: CC8.1 (Attribution) mandates capturing **authenticated** user identity for all operational decisions.

**Key Challenge**: How to implement webhooks that:
- ✅ Extract real user identity from Kubernetes authentication context
- ✅ Follow standard Kubernetes operator patterns
- ✅ Enable code reuse between services
- ✅ Allow independent team ownership and deployment

### **Alternatives Considered**

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **A. Shared Webhook (1 deployment)** | Single deployment, full code reuse | Tight coupling, shared lifecycle, not K8s standard | ❌ Rejected |
| **B. Independent Webhooks with Shared Library** | Team autonomy, standard pattern, fault isolation | Minor code duplication (~200 LOC shared) | ✅ **SELECTED** |
| **C. Manual Webhook Implementation** | Full control | High manual work, no scaffolding, error-prone | ❌ Rejected |

**Decision**: **Option B** - Independent webhooks using operator-sdk scaffolding with shared library.

**Confidence**: 92%

---

## 🏗️ **ARCHITECTURE**

### **Pattern Overview**

```
┌─────────────────────────────────────────────────────────┐
│  cmd/workflowexecution/main.go                          │
│  └── Registers WE webhook with manager                  │
└─────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────┐
│  api/workflowexecution/v1alpha1/                        │
│  └── workflowexecution_webhook.go                       │
│      ├── Default() - mutation logic                     │
│      ├── ValidateUpdate() - validation logic            │
│      └── imports pkg/authwebhook                        │
└─────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────┐
│  pkg/authwebhook/ (Shared Library - ~200 LOC)          │
│  ├── types.go (AuthContext, interfaces)                │
│  ├── authenticator.go (ExtractUser from K8s auth)      │
│  ├── validator.go (ValidateReason, ValidateTimestamp)  │
│  └── audit.go (EmitAuthenticationEvent)                │
└─────────────────────────────────────────────────────────┘
                              ↑
┌─────────────────────────────────────────────────────────┐
│  api/remediationorchestrator/v1alpha1/                  │
│  └── remediationapprovalrequest_webhook.go              │
│      ├── Default() - mutation logic                     │
│      ├── ValidateUpdate() - validation logic            │
│      └── imports pkg/authwebhook                        │
└─────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────┐
│  cmd/remediationorchestrator/main.go                    │
│  └── Registers RO webhook with manager                  │
└─────────────────────────────────────────────────────────┘
```

### **Deployment Architecture**

```
┌───────────────────────────────────────────────────────┐
│  Kubernetes API Server                                │
│  ├── Authenticates user (OIDC/certs/SA token)        │
│  └── Sends admission request with req.UserInfo       │
└───────────────────────────────────────────────────────┘
                    ↓                    ↓
    ┌───────────────────────┐    ┌──────────────────────────┐
    │  WE Webhook Service   │    │  RO Webhook Service      │
    │  (WE Team owns)       │    │  (RO Team owns)          │
    │  Port: 9443           │    │  Port: 9443              │
    │  Cert: wfe-webhook    │    │  Cert: ro-webhook        │
    └───────────────────────┘    └──────────────────────────┘
```

**Key Properties**:
- ✅ **Independent Deployment**: Each webhook has its own pod, service, certificate
- ✅ **Team Ownership**: WE team owns WE webhook, RO team owns RO webhook
- ✅ **Fault Isolation**: WE webhook failure doesn't affect RO
- ✅ **Independent Scaling**: Scale each webhook based on its load

---

## 📝 **IMPLEMENTATION PATTERN**

### **Step 1: Scaffold Webhook (Operator-SDK)**

```bash
# For WorkflowExecution
kubebuilder create webhook \
    --group workflowexecution \
    --version v1alpha1 \
    --kind WorkflowExecution \
    --defaulting \
    --programmatic-validation

# For RemediationApprovalRequest
kubebuilder create webhook \
    --group remediationorchestrator \
    --version v1alpha1 \
    --kind RemediationApprovalRequest \
    --defaulting \
    --programmatic-validation
```

**Generated Files**:
```
api/{group}/v1alpha1/
├── {kind}_webhook.go           # Webhook implementation (edit this)
└── webhook_suite_test.go        # Test suite (add tests here)

config/webhook/
├── manifests.yaml               # MutatingWebhookConfiguration
├── service.yaml                 # Webhook service
└── kustomization.yaml           # Kustomize config

config/certmanager/
├── certificate.yaml             # cert-manager Certificate
└── kustomization.yaml
```

### **Step 2: Implement Shared Library**

**File**: `pkg/authwebhook/types.go` (~80 LOC)

```go
package authwebhook

import (
    "fmt"
    authenticationv1 "k8s.io/api/authentication/v1"
)

// AuthContext contains authenticated user information from Kubernetes
type AuthContext struct {
    Username string
    UID      string
    Groups   []string
    Extra    map[string]authenticationv1.ExtraValue
}

// String returns formatted authentication string for audit trail
// Format: "username (UID: uid)"
func (a *AuthContext) String() string {
    return fmt.Sprintf("%s (UID: %s)", a.Username, a.UID)
}
```

**File**: `pkg/authwebhook/authenticator.go` (~100 LOC)

```go
package authwebhook

import (
    "context"
    "fmt"
    admissionv1 "k8s.io/api/admission/v1"
)

// Authenticator extracts user identity from Kubernetes authentication context
type Authenticator struct {}

// NewAuthenticator creates a new Authenticator
func NewAuthenticator() *Authenticator {
    return &Authenticator{}
}

// ExtractUser extracts authenticated user from admission request
// This is the CORE authentication logic
func (a *Authenticator) ExtractUser(ctx context.Context, req *admissionv1.AdmissionRequest) (*AuthContext, error) {
    if req.UserInfo.Username == "" {
        return nil, fmt.Errorf("no user information in request")
    }

    if req.UserInfo.UID == "" {
        return nil, fmt.Errorf("no user UID in request")
    }

    return &AuthContext{
        Username: req.UserInfo.Username,
        UID:      req.UserInfo.UID,
        Groups:   req.UserInfo.Groups,
        Extra:    req.UserInfo.Extra,
    }, nil
}
```

**File**: `pkg/authwebhook/validator.go` (~60 LOC)

```go
package authwebhook

import (
    "fmt"
    "strings"
    "time"
)

// ValidateReason validates that a reason string meets minimum requirements
func ValidateReason(reason string, minLength int) error {
    if reason == "" {
        return fmt.Errorf("reason is required")
    }

    if len(reason) < minLength {
        return fmt.Errorf("reason must be at least %d characters, got %d", minLength, len(reason))
    }

    if strings.TrimSpace(reason) == "" {
        return fmt.Errorf("reason cannot be only whitespace")
    }

    return nil
}

// ValidateTimestamp validates that a timestamp is present and not in the future
func ValidateTimestamp(ts time.Time) error {
    if ts.IsZero() {
        return fmt.Errorf("timestamp is required")
    }

    if ts.After(time.Now()) {
        return fmt.Errorf("timestamp cannot be in the future")
    }

    return nil
}
```

**File**: `pkg/authwebhook/audit.go` (~60 LOC)

```go
package authwebhook

import (
    "context"
    "github.com/jordigilh/kubernaut/internal/client/datastorage/dsgen"
)

// AuditClient wraps Data Storage API for audit event emission
type AuditClient struct {
    client *dsgen.ClientWithResponses
}

// NewAuditClient creates a new AuditClient
func NewAuditClient(dsClient *dsgen.ClientWithResponses) *AuditClient {
    return &AuditClient{client: dsClient}
}

// EmitAuthenticationEvent records an authentication event
func (a *AuditClient) EmitAuthenticationEvent(ctx context.Context, event *AuthenticationEvent) error {
    auditEvent := dsgen.AuditEvent{
        EventType:     event.EventType,
        EventCategory: event.EventCategory,
        EventAction:   event.EventAction,
        EventOutcome:  event.EventOutcome,
        ActorType:     event.ActorType,
        ActorId:       event.ActorID,
        ResourceType:  event.ResourceType,
        ResourceName:  event.ResourceName,
        EventData:     event.EventData,
    }

    resp, err := a.client.CreateAuditEventWithResponse(ctx, auditEvent)
    if err != nil {
        return fmt.Errorf("failed to create audit event: %w", err)
    }

    if resp.StatusCode() != http.StatusCreated {
        return fmt.Errorf("audit event creation failed: %s", resp.Status())
    }

    return nil
}
```

### **Step 3: Implement CRD Webhook (Example: WorkflowExecution)**

**File**: `api/workflowexecution/v1alpha1/workflowexecution_webhook.go`

```go
package v1alpha1

import (
    "context"
    "fmt"
    "github.com/jordigilh/kubernaut/pkg/authwebhook"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/webhook"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:webhook:path=/mutate-kubernaut-ai-v1alpha1-workflowexecution,mutating=true,failurePolicy=fail,groups=kubernaut.ai,resources=workflowexecutions/status,verbs=update,versions=v1alpha1,name=mworkflowexecution.kubernaut.ai,admissionReviewVersions=v1;v1beta1,sideEffects=None

var (
    authenticator *authwebhook.Authenticator
    auditClient   *authwebhook.AuditClient
)

// SetupWebhookWithManager registers the webhook with the manager
func (r *WorkflowExecution) SetupWebhookWithManager(mgr ctrl.Manager, dsClient *dsgen.ClientWithResponses) error {
    authenticator = authwebhook.NewAuthenticator()
    auditClient = authwebhook.NewAuditClient(dsClient)

    return ctrl.NewWebhookManagedBy(mgr).
        For(r).
        Complete()
}

var _ webhook.Defaulter = &WorkflowExecution{}

// Default implements webhook.Defaulter for status subresource updates
func (r *WorkflowExecution) Default() {
    // Get admission request from context
    ctx := context.Background()
    req, err := admission.RequestFromContext(ctx)
    if err != nil {
        return // Can't get request context
    }

    // CRITICAL: Allow controller ServiceAccount to bypass webhook
    // The controller needs to update status fields (phase, message, etc.) without authentication
    if isControllerServiceAccount(req.AdmissionRequest.UserInfo) {
        return // Allow controller updates to pass through unchanged
    }

    // Only process if blockClearanceRequest exists (operator-initiated clearance)
    if r.Status.BlockClearanceRequest == nil {
        return // No clearance request, allow update to pass through
    }

    // Extract authenticated user
    authCtx, err := authenticator.ExtractUser(ctx, &req.AdmissionRequest)
    if err != nil {
        return // Error handled by ValidateUpdate
    }

    // Populate authenticated fields
    r.Status.BlockClearance = &BlockClearanceDetails{
        ClearedBy:   authCtx.String(),
        ClearedAt:   metav1.Now(),
        ClearReason: r.Status.BlockClearanceRequest.ClearReason,
        ClearMethod: "KubernetesAdmissionWebhook",
    }

    // Clear the request (consumed)
    r.Status.BlockClearanceRequest = nil

    // Emit audit event (best-effort)
    _ = auditClient.EmitAuthenticationEvent(ctx, &authwebhook.AuthenticationEvent{
        EventType:     "workflowexecution.block.cleared",
        EventCategory: "workflow",
        EventAction:   "block.cleared",
        EventOutcome:  "success",
        ActorType:     "user",
        ActorID:       authCtx.String(),
        ResourceType:  "WorkflowExecution",
        ResourceName:  r.Name,
        EventData: map[string]interface{}{
            "cleared_by":   authCtx.String(),
            "clear_reason": r.Status.BlockClearance.ClearReason,
            "clear_method": "KubernetesAdmissionWebhook",
        },
    })
}

var _ webhook.Validator = &WorkflowExecution{}

// ValidateUpdate implements webhook.Validator
func (r *WorkflowExecution) ValidateUpdate(ctx context.Context, old runtime.Object) (admission.Warnings, error) {
    // Only validate if blockClearanceRequest exists
    if r.Status.BlockClearanceRequest == nil {
        return nil, nil
    }

    // Validate clearance request
    if err := authwebhook.ValidateReason(r.Status.BlockClearanceRequest.ClearReason, 10); err != nil {
        return nil, fmt.Errorf("clearReason validation failed: %w", err)
    }

    if err := authwebhook.ValidateTimestamp(r.Status.BlockClearanceRequest.RequestedAt.Time); err != nil {
        return nil, fmt.Errorf("requestedAt validation failed: %w", err)
    }

    return nil, nil
}

// ValidateCreate implements webhook.Validator (no-op for status subresource)
func (r *WorkflowExecution) ValidateCreate(ctx context.Context) (admission.Warnings, error) {
    return nil, nil
}

// ValidateDelete implements webhook.Validator (no-op for status subresource)
func (r *WorkflowExecution) ValidateDelete(ctx context.Context) (admission.Warnings, error) {
    return nil, nil
}

// isControllerServiceAccount checks if the request is from the controller's ServiceAccount
// Controllers need to update status fields without triggering authentication
// This prevents webhook from interfering with normal reconciliation loop
func isControllerServiceAccount(userInfo authenticationv1.UserInfo) bool {
    // WorkflowExecution controller ServiceAccount pattern
    // Format: system:serviceaccount:{namespace}:workflowexecution-controller
    return strings.HasPrefix(userInfo.Username, "system:serviceaccount:") &&
           strings.Contains(userInfo.Username, "workflowexecution-controller")
}
```

### **Step 4: Wire Webhook in main.go**

**File**: `cmd/workflowexecution/main.go`

```go
func main() {
    // ... existing setup ...

    // Create Data Storage client for audit events
    dsClient, err := dsgen.NewClientWithResponses(dataStorageURL)
    if err != nil {
        setupLog.Error(err, "unable to create Data Storage client")
        os.Exit(1)
    }

    // Setup WorkflowExecution webhook
    if err = (&workflowexecutionv1alpha1.WorkflowExecution{}).SetupWebhookWithManager(mgr, dsClient); err != nil {
        setupLog.Error(err, "unable to create webhook", "webhook", "WorkflowExecution")
        os.Exit(1)
    }

    // ... start manager ...
}
```

---

## 🔐 **CRITICAL: Controller ServiceAccount Bypass**

### **Problem**

Webhooks configured on `/status` subresource intercept **ALL** status updates, including:
- ❌ Controller's normal reconciliation updates (phase, message, conditions)
- ❌ Controller's routine status syncs
- ❌ Could slow down reconciliation loop with unnecessary webhook calls

**Without bypass**: Controller's status updates would trigger authentication, which is unnecessary and potentially problematic.

### **Solution: ServiceAccount Bypass Pattern**

**Pattern**: Webhook checks if request is from controller ServiceAccount and allows it to pass through unchanged.

```go
func (r *WorkflowExecution) Default() {
    // Get admission request from context
    ctx := context.Background()
    req, err := admission.RequestFromContext(ctx)
    if err != nil {
        return
    }

    // CRITICAL: Allow controller ServiceAccount to bypass webhook
    if isControllerServiceAccount(req.AdmissionRequest.UserInfo) {
        return // Controller updates pass through unchanged
    }

    // Only process operator-initiated clearance requests
    if r.Status.BlockClearanceRequest == nil {
        return
    }

    // ... authentication logic for operator requests ...
}

func isControllerServiceAccount(userInfo authenticationv1.UserInfo) bool {
    // Check for controller ServiceAccount pattern
    return strings.HasPrefix(userInfo.Username, "system:serviceaccount:") &&
           strings.Contains(userInfo.Username, "workflowexecution-controller")
}
```

### **How This Works**

```
┌─────────────────────────────────────────────────────────┐
│  Controller Status Update                               │
│  User: system:serviceaccount:kubernaut:wfe-controller   │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Check UserInfo       │
        │  Is ServiceAccount?   │
        └───────────────────────┘
                    ↓
            ✅ YES: Bypass
                    ↓
    ┌───────────────────────────────┐
    │  Status Update Proceeds       │
    │  (phase, message, conditions) │
    └───────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│  Operator Block Clearance                               │
│  User: operator@example.com                             │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Check UserInfo       │
        │  Is ServiceAccount?   │
        └───────────────────────┘
                    ↓
            ❌ NO: Authenticate
                    ↓
    ┌───────────────────────────────┐
    │  Extract User Identity        │
    │  Populate blockClearance      │
    │  Emit Audit Event             │
    └───────────────────────────────┘
```

### **Benefits**

| Benefit | Description |
|---------|-------------|
| ✅ **No Reconciliation Impact** | Controller updates bypass webhook (fast) |
| ✅ **Operator Authentication** | Human operators still authenticated |
| ✅ **Clear Separation** | Controller = automated, Operator = manual |
| ✅ **Performance** | No unnecessary webhook calls during reconciliation |

### **TR-6: Mutual Exclusion - Prevent Field Modification Across Actor Boundaries**

**Problem**: Without validation, actors could accidentally or maliciously modify fields they don't own:

**Without validation**:
- ❌ **Users** could manually set `status.phase` to "Completed" (bypassing execution)
- ❌ **Users** could forge `status.message` to hide failures
- ❌ **Controller** could accidentally modify `status.blockClearanceRequest` (corrupts auth flow)

**Solution**: Webhook enforces mutual exclusion - each actor can ONLY modify their designated fields.

```go
func (r *WorkflowExecution) ValidateUpdate(ctx context.Context, old runtime.Object) (admission.Warnings, error) {
    oldWFE := old.(*WorkflowExecution)

    // Get admission request to check user
    req, err := admission.RequestFromContext(ctx)
    if err != nil {
        return nil, err
    }

    isController := isControllerServiceAccount(req.AdmissionRequest.UserInfo)

    if isController {
        // ⭐ MUTUAL EXCLUSION: Controller CANNOT modify operator-managed fields
        // This prevents accidental programming errors in controller code
        if !reflect.DeepEqual(oldWFE.Status.BlockClearanceRequest, r.Status.BlockClearanceRequest) {
            return nil, fmt.Errorf("controller cannot modify status.blockClearanceRequest (operator-managed field)")
        }

        if !reflect.DeepEqual(oldWFE.Status.BlockClearance, r.Status.BlockClearance) {
            return nil, fmt.Errorf("controller cannot modify status.blockClearance (webhook-managed field)")
        }

        // Controller CAN modify all other status fields (phase, message, conditions, etc.)
        return nil, nil
    } else {
        // ⭐ MUTUAL EXCLUSION: Operators CANNOT modify controller-managed fields
        // This prevents status field forgery
        if !reflect.DeepEqual(oldWFE.Status.Phase, r.Status.Phase) {
            return nil, fmt.Errorf("users cannot modify status.phase (controller-managed field)")
        }

        if !reflect.DeepEqual(oldWFE.Status.Message, r.Status.Message) {
            return nil, fmt.Errorf("users cannot modify status.message (controller-managed field)")
        }

        if !reflect.DeepEqual(oldWFE.Status.Conditions, r.Status.Conditions) {
            return nil, fmt.Errorf("users cannot modify status.conditions (controller-managed field)")
        }

        if !reflect.DeepEqual(oldWFE.Status.ConsecutiveFailures, r.Status.ConsecutiveFailures) {
            return nil, fmt.Errorf("users cannot modify status.consecutiveFailures (controller-managed field)")
        }

        if !reflect.DeepEqual(oldWFE.Status.NextAllowedExecution, r.Status.NextAllowedExecution) {
            return nil, fmt.Errorf("users cannot modify status.nextAllowedExecution (controller-managed field)")
        }

        // Operators CANNOT modify blockClearance (webhook populates this)
        if !reflect.DeepEqual(oldWFE.Status.BlockClearance, r.Status.BlockClearance) {
            return nil, fmt.Errorf("users cannot modify status.blockClearance (webhook-managed field)")
        }

        // Operators CAN modify blockClearanceRequest - validate it
        if r.Status.BlockClearanceRequest != nil {
            if err := authwebhook.ValidateReason(r.Status.BlockClearanceRequest.ClearReason, 10); err != nil {
                return nil, fmt.Errorf("clearReason validation failed: %w", err)
            }

            if err := authwebhook.ValidateTimestamp(r.Status.BlockClearanceRequest.RequestedAt.Time); err != nil {
                return nil, fmt.Errorf("requestedAt validation failed: %w", err)
            }
        }

        return nil, nil
    }
}
```

### **How Mutual Exclusion Works**

**Scenario 1: Operator Attempts to Modify Controller-Managed Field (DENIED)**

```
┌─────────────────────────────────────────────────────────┐
│  Operator Attempts to Modify status.phase              │
│  User: operator@example.com                             │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  ValidateUpdate()     │
        │  isController?        │
        └───────────────────────┘
                    ↓
            ❌ NO (User)
                    ↓
        ┌───────────────────────┐
        │  Compare Old vs New   │
        │  status.phase changed?│
        └───────────────────────┘
                    ↓
            ✅ YES: DENY
                    ↓
    ┌───────────────────────────────┐
    │  Reject: "users cannot modify │
    │   status.phase"               │
    └───────────────────────────────┘
```

**Scenario 2: Operator Modifies Operator-Managed Field (ALLOWED)**

```
┌─────────────────────────────────────────────────────────┐
│  Operator Modifies blockClearanceRequest               │
│  User: operator@example.com                             │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  ValidateUpdate()     │
        │  isController?        │
        └───────────────────────┘
                    ↓
            ❌ NO (User)
                    ↓
        ┌───────────────────────┐
        │  Check controller     │
        │  fields unchanged?    │
        └───────────────────────┘
                    ↓
            ✅ YES: Validate
                    ↓
    ┌───────────────────────────────┐
    │  Validation passes            │
    │  Default() mutates request    │
    └───────────────────────────────┘
```

**Scenario 3: Controller Attempts to Modify Operator-Managed Field (DENIED)**

```
┌─────────────────────────────────────────────────────────┐
│  Controller Bug: Modifies blockClearanceRequest        │
│  User: system:serviceaccount:...:wfe-controller         │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  ValidateUpdate()     │
        │  isController?        │
        └───────────────────────┘
                    ↓
            ✅ YES (Controller)
                    ↓
        ┌──────────────────────────────────┐
        │  Compare Old vs New              │
        │  blockClearanceRequest changed?  │
        └──────────────────────────────────┘
                    ↓
            ✅ YES: DENY
                    ↓
    ┌───────────────────────────────────────┐
    │  Reject: "controller cannot modify   │
    │   status.blockClearanceRequest"      │
    └───────────────────────────────────────┘
```

**Scenario 4: Controller Modifies Controller-Managed Fields (ALLOWED)**

```
┌─────────────────────────────────────────────────────────┐
│  Controller Reconciliation: Updates status.phase        │
│  User: system:serviceaccount:...:wfe-controller         │
└─────────────────────────────────────────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  Webhook Intercepted  │
        └───────────────────────┘
                    ↓
        ┌───────────────────────┐
        │  ValidateUpdate()     │
        │  isController?        │
        └───────────────────────┘
                    ↓
            ✅ YES (Controller)
                    ↓
        ┌──────────────────────────────────┐
        │  Check operator fields unchanged?│
        └──────────────────────────────────┘
                    ↓
            ✅ YES: ALLOW
                    ↓
    ┌───────────────────────────────┐
    │  Validation passes            │
    │  Status update proceeds       │
    └───────────────────────────────┘
```

### **Benefits**

|| Benefit | Description |
||---------|-------------|
|| ✅ **Bidirectional Protection** | Controller AND operators cannot modify each other's fields |
|| ✅ **Fail-Fast** | Programming errors caught immediately (controller bugs rejected) |
|| ✅ **Defense-in-Depth** | Controller-managed fields protected from user tampering |
|| ✅ **Explicit Allow-List** | Only `blockClearanceRequest` allowed for operators |
|| ✅ **SOC2 Integrity** | Prevents status field forgery (CC6.1) |
|| ✅ **Clear Ownership** | Each actor has designated fields they can modify |

### **Critical Design Choice**

**Why validation, not mutation?**
- ✅ **Validation** (`ValidateUpdate`) can DENY requests with clear error messages
- ✅ **Mutation** (`Default`) only modifies allowed fields
- ✅ **Separation of concerns**: Validation rejects bad requests, mutation adds authentication

**Why ServiceAccount check in both methods?**
- ✅ **Default()**: Bypass authentication logic (controller doesn't need it)
- ✅ **ValidateUpdate()**: Bypass field protection (controller owns all fields)

---

## 🧪 **TESTING STRATEGY**

### **Shared Library Tests** (~300 LOC total)

**Unit Tests** (`pkg/authwebhook/*_test.go`):
- 8 tests for `Authenticator.ExtractUser()`
- 4 tests for `ValidateReason()`
- 2 tests for `ValidateTimestamp()`
- 4 tests for `AuditClient.EmitAuthenticationEvent()`

### **Webhook Tests** (~400 LOC per webhook)

**Unit Tests** (`api/{group}/v1alpha1/{kind}_webhook_test.go`):
- 18 tests per webhook
- Test authentication extraction
- Test validation logic
- Test mutation logic
- **Test controller ServiceAccount bypass in Default()** ⭐ CRITICAL
- **Test controller ServiceAccount bypasses validation for controller-managed fields** ⭐ CRITICAL
- **Test operator requests trigger authentication** ⭐ CRITICAL
- **Test users CANNOT modify status.phase** ⭐ MUTUAL EXCLUSION
- **Test users CANNOT modify status.message** ⭐ MUTUAL EXCLUSION
- **Test users CANNOT modify status.conditions** ⭐ MUTUAL EXCLUSION
- **Test users CANNOT modify status.consecutiveFailures** ⭐ MUTUAL EXCLUSION
- **Test users CANNOT modify status.blockClearance** ⭐ MUTUAL EXCLUSION
- **Test users CAN modify blockClearanceRequest** ⭐ MUTUAL EXCLUSION
- **Test controller CANNOT modify status.blockClearanceRequest** ⭐ NEW - MUTUAL EXCLUSION
- **Test controller CANNOT modify status.blockClearance** ⭐ NEW - MUTUAL EXCLUSION
- **Test controller CAN modify all controller-managed fields** ⭐ MUTUAL EXCLUSION
- **Test validation error messages are clear** ⭐ UX
- **Test mutual exclusion applies ONLY to Update operations** ⭐ OPERATION-SPECIFIC

**Integration Tests** (`test/integration/{service}/webhook_test.go`):
- 3 tests per webhook using envtest
- Test complete webhook flow with real K8s API
- Verify CRD status updates
- Verify audit events emitted

**E2E Tests** (`test/e2e/{service}/webhook_test.go`):
- 2 tests per webhook using Kind cluster
- Test with real `kubectl patch` commands
- Verify end-to-end authentication
- Verify audit trail completeness

---

## 📊 **BENEFITS**

### **Technical Benefits**

| Benefit | Description | Impact |
|---------|-------------|--------|
| **Production-Ready Code** | Operator-SDK generates battle-tested boilerplate | 33% less manual code |
| **Standard Pattern** | Follows Kubernetes operator best practices | Lower learning curve |
| **Code Reuse** | Shared library (~200 LOC) used by all webhooks | Consistency enforced |
| **Team Autonomy** | Each team owns its webhook lifecycle | Independent iteration |
| **Fault Isolation** | Webhook failures isolated per service | Higher availability |
| **Simplified RBAC** | Minimal permissions per webhook | Better security |
| **Independent Scaling** | Scale each webhook independently | Resource efficiency |

### **Operational Benefits**

| Benefit | Description | Impact |
|---------|-------------|--------|
| **Independent Deployment** | No cross-team blocking | Faster iteration |
| **Clear Ownership** | WE team owns WE webhook, RO team owns RO | Accountability |
| **Independent Rollback** | Rollback one webhook without affecting others | Lower risk |
| **Separate Logs/Metrics** | Per-webhook observability | Easier troubleshooting |

---

## 🚫 **ANTI-PATTERNS TO AVOID**

### **❌ DO NOT: Create Shared Webhook Service**

```
❌ WRONG:
kubernaut-auth-webhook (single deployment)
├── Handler 1: /authenticate/workflowexecution
└── Handler 2: /authenticate/remediationapproval
```

**Why Wrong**: Tight coupling, shared lifecycle, non-standard pattern, single point of failure

### **❌ DO NOT: Implement Webhook Manually**

```go
❌ WRONG: Manual webhook implementation
// Don't write webhook server from scratch
http.HandleFunc("/mutate", func(w http.ResponseWriter, r *http.Request) {
    // ... manual admission request parsing ...
})
```

**Why Wrong**: Error-prone, no scaffolding, violates operator-SDK patterns

### **❌ DO NOT: Duplicate Authentication Logic**

```go
❌ WRONG: Copy-paste authentication logic
// In WE webhook
func extractUser(req *admissionv1.AdmissionRequest) string {
    return req.UserInfo.Username + " (UID: " + req.UserInfo.UID + ")"
}

// In RO webhook (duplicated!)
func extractUser(req *admissionv1.AdmissionRequest) string {
    return req.UserInfo.Username + " (UID: " + req.UserInfo.UID + ")"
}
```

**Why Wrong**: Use `pkg/authwebhook` library instead!

---

## 📚 **REFERENCES**

### **Authoritative Documents**

1. **BR-WE-013**: [Audit-Tracked Block Clearing](../../requirements/BR-WE-013-audit-tracked-block-clearing.md) - WE use case
2. **ADR-040**: [RemediationApprovalRequest Architecture](./ADR-040-remediation-approval-request-architecture.md) - RO use case
3. **Webhook Triage**: [WEBHOOK_ARCHITECTURE_TRIAGE_OPERATOR_SDK_VS_SHARED_DEC_20_2025.md](../../handoff/WEBHOOK_ARCHITECTURE_TRIAGE_OPERATOR_SDK_VS_SHARED_DEC_20_2025.md)

### **External References**

4. [Kubebuilder Webhook Guide](https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html)
5. [Kubernetes Admission Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/)
6. [controller-runtime Webhook API](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/webhook)

---

## ✅ **ACCEPTANCE CRITERIA**

This ADR is successfully implemented when:

- ✅ `pkg/authwebhook` shared library exists with 18+ unit tests
- ✅ WE webhook implemented using operator-SDK scaffolding
- ✅ RO webhook implemented using operator-SDK scaffolding
- ✅ Both webhooks use `pkg/authwebhook` library (no code duplication)
- ✅ Each webhook has independent deployment (separate pods, services, certs)
- ✅ 18+ unit tests per webhook (including mutual exclusion tests)
- ✅ 3+ integration tests per webhook
- ✅ 2+ E2E tests per webhook
- ✅ **Controller ServiceAccount bypass implemented** ⭐ CRITICAL
- ✅ **Mutual exclusion validation implemented (bidirectional)** ⭐ CRITICAL
- ✅ **Tests verify controller can modify controller-managed fields** ⭐ CRITICAL
- ✅ **Tests verify controller CANNOT modify operator-managed fields** ⭐ NEW - CRITICAL
- ✅ **Tests verify users CANNOT modify controller-managed fields** ⭐ CRITICAL
- ✅ **Tests verify users CAN modify operator-managed fields** ⭐ CRITICAL
- ✅ **Validation applies ONLY to Update operations** ⭐ CRITICAL
- ✅ Documentation updated with webhook patterns

---

## 📅 **TIMELINE**

| Phase | Duration | Deliverables |
|-------|----------|--------------|
| **Phase 1** | 4 hours | Shared library (`pkg/authwebhook`) + 18 unit tests |
| **Phase 2** | 8 hours | WE webhook scaffolding + implementation + tests |
| **Phase 3** | 8 hours | RO webhook scaffolding + implementation + tests |
| **Phase 4** | 4 hours | E2E tests for both webhooks |
| **Total** | **3-4 days** | Complete webhook implementation |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2025-12-20 | Added TR-6: Mutual exclusion validation - bidirectional field protection (controller CANNOT modify operator-managed fields, operators CANNOT modify controller-managed fields). Added 3 new unit tests for mutual exclusion. Updated acceptance criteria to include mutual exclusion requirements. Clarified that validation applies ONLY to Update operations. |
| 1.0 | 2025-12-20 | Initial ADR: Operator-SDK webhook scaffolding pattern with shared library. Includes TR-5: Controller ServiceAccount bypass pattern. |

---

**Document Status**: ✅ **APPROVED**
**Version**: 1.1
**Authority**: **AUTHORITATIVE** for all CRD webhook implementations
**Next Review**: 2026-06-20 (6 months)

