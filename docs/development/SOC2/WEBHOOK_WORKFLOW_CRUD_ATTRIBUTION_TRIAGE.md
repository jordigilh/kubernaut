# Workflow CRUD Attribution Triage - SOC2 CC8.1 Compliance

**Date**: January 6, 2026
**Status**: 🚨 **CRITICAL CORRECTION REQUIRED**
**Purpose**: Correct workflow authentication approach for SOC2 CC8.1
**Trigger**: User identified RemediationWorkflow is REST API, not CRD

---

## 🚨 **CRITICAL FINDING: No RemediationWorkflow CRD Exists**

### **ERROR IN CURRENT DOCUMENTATION**

**Documents with INCORRECT assumption**:
1. ❌ `DD-WEBHOOK-001 v1.1` - Added "RemediationWorkflow CRD" (WRONG)
2. ❌ `WEBHOOK_CONSOLIDATION_TRIAGE_CORRECTED.md` - Listed as CRD requiring webhook (WRONG)
3. ❌ `TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md` - Mentions "RemediationWorkflow webhook" (WRONG)

**Root Cause**: Incorrect assumption that workflow catalog is managed via CRD, when it's actually REST API only.

---

## 📋 **AUTHORITATIVE ARCHITECTURE - Workflow Catalog**

### **Current Implementation (V1.0)**

**Authority**: `DD-WORKFLOW-005 v2.0` (Automated Schema Extraction)

**Workflow Registration Method**: **REST API ONLY** ✅

```bash
# V1.0: Direct REST API workflow registration
POST /api/v1/workflows
Content-Type: application/json

{
  "workflow_id": "restart-pod-workflow",
  "title": "Restart Pod Workflow",
  "description": "Restarts a failing pod",
  "signal_type": "CrashLoopBackOff",
  "container_image": "ghcr.io/kubernaut/workflows/restart-pod:v1.0.0",
  "container_digest": "sha256:abc123...",
  "labels": ["pod", "restart", "oomkill"]
}
```

**Update Operations**:
```bash
# Update mutable fields
PATCH /api/v1/workflows/{workflowID}

# Disable workflow (convenience endpoint)
PATCH /api/v1/workflows/{workflowID}/disable
```

**Authority**: `pkg/datastorage/server/server.go` lines 358-366
- `r.Post("/workflows", s.handler.HandleCreateWorkflow)`
- `r.Patch("/workflows/{workflowID}", s.handler.HandleUpdateWorkflow)`
- `r.Patch("/workflows/{workflowID}/disable", s.handler.HandleDisableWorkflow)`

---

### **Future Implementation (V1.1 - Not Now)**

**Authority**: `DD-WORKFLOW-005 v2.0` Section "V1.1 Approach"

**Planned Approach**: `WorkflowRegistration` CRD with automated controller
- **NOT IMPLEMENTED YET**
- **NOT IN SCOPE for current SOC2 work**
- V1.1 will automate workflow registration via CRD controller

---

## 🔍 **CORRECTED: CRD Webhook Requirements**

### **CRDs Actually Requiring Webhooks** ✅ (3 TOTAL, NOT 4)

| CRD | Use Case | Operation Type | SOC2 Control | Webhook Type |
|-----|----------|----------------|--------------|--------------|
| **WorkflowExecution** | Block Clearance | Status Update (manual) | CC8.1 (Attribution) | MutatingWebhookConfiguration |
| **RemediationApprovalRequest** | Approval Decisions | Status Update (manual) | CC8.1 (Attribution) | MutatingWebhookConfiguration |
| **NotificationRequest** | Cancellation Attribution | DELETE operation | CC8.1 (Attribution) | ValidatingWebhookConfiguration |

### **REST API Endpoints Requiring Authentication Middleware** ✅ (1 TOTAL)

| REST API | Use Case | HTTP Method | SOC2 Control | Authentication Method |
|----------|----------|-------------|--------------|----------------------|
| `/api/v1/workflows` | Workflow CRUD Attribution | POST/PATCH/DELETE | CC8.1 (Attribution) | HTTP Middleware + Header Extraction |

---

## 🏗️ **CORRECTED ARCHITECTURE: Two Authentication Patterns**

### **Pattern 1: Kubernetes Admission Webhooks** (for CRDs)

**Use Case**: CRD status updates or deletion
**Technology**: `MutatingWebhookConfiguration` / `ValidatingWebhookConfiguration`
**Authentication Source**: `req.UserInfo` from Kubernetes API server
**Shared Library**: `pkg/authwebhook/` ✅ (already exists)

**Applies To**:
1. WorkflowExecution (Status Update)
2. RemediationApprovalRequest (Status Update)
3. NotificationRequest (DELETE)

**Consolidated Service**: `kubernaut-auth-webhook` (single deployment)

---

### **Pattern 2: HTTP Authentication Middleware** (for REST APIs)

**Use Case**: REST API workflow catalog operations
**Technology**: HTTP middleware extracting authenticated user from request headers
**Authentication Source**: TBD (options below)
**Shared Library**: TBD (needs creation)

**Applies To**:
1. `POST /api/v1/workflows` (create)
2. `PATCH /api/v1/workflows/{workflowID}` (update)
3. `PATCH /api/v1/workflows/{workflowID}/disable` (disable)

---

## 🔧 **HTTP Authentication Options for Data Storage API**

### **Option 1: Kubernetes Service Account Token (JWT)** ⭐ RECOMMENDED

**How It Works**:
1. Operator creates workflow via `kubectl` or API
2. Request includes K8s ServiceAccount JWT token in `Authorization: Bearer <token>` header
3. Data Storage middleware validates JWT against K8s API server
4. Extract `sub` claim (e.g., `system:serviceaccount:default:operator-sa`)
5. Populate audit event with authenticated actor

**Pros**:
- ✅ Native Kubernetes authentication
- ✅ No external auth system required
- ✅ Consistent with CRD webhook pattern (same auth source)
- ✅ Works with `kubectl proxy` or direct API calls
- ✅ RBAC integration (validate user has permission)

**Cons**:
- ⚠️ Requires JWT validation middleware (new code)
- ⚠️ Token expiration handling needed

**Implementation**:
```go
// pkg/datastorage/middleware/auth.go
func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract JWT from Authorization header
        token := extractBearerToken(r)
        if token == "" {
            http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
            return
        }

        // Validate token against K8s API server
        userInfo, err := m.k8sTokenReviewer.Review(r.Context(), token)
        if err != nil {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }

        // Attach authenticated user to request context
        ctx := context.WithValue(r.Context(), authUserKey, userInfo)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

**Audit Event Population**:
```go
// In HandleCreateWorkflow, HandleUpdateWorkflow, HandleDisableWorkflow
userInfo := auth.UserFromContext(r.Context())
event := audit.Event{
    EventType:     "datastorage.workflow.created",
    EventCategory: "workflow",
    EventAction:   "create",
    EventOutcome:  "success",
    ActorID:       userInfo.Username,  // e.g., "operator@example.com"
    ActorType:     "user",
    EventData: map[string]interface{}{
        "workflow_id": workflow.WorkflowID,
        "created_by": map[string]interface{}{
            "username": userInfo.Username,
            "uid":      userInfo.UID,
            "groups":   userInfo.Groups,
        },
    },
}
```

---

### **Option 2: Custom X-User-Identity Header**

**How It Works**:
1. Operator sets `X-User-Identity` header manually
2. Data Storage middleware extracts header value
3. Populate audit event with user identity

**Pros**:
- ✅ Simple implementation
- ✅ No JWT validation needed

**Cons**:
- ❌ **INSECURE** - User identity can be forged
- ❌ **NOT SOC2 COMPLIANT** - No cryptographic verification
- ❌ Violates CC8.1 (requires authenticated identity)

**Verdict**: ❌ **DO NOT USE** (fails SOC2 requirements)

---

### **Option 3: API Key Authentication**

**How It Works**:
1. Generate API keys per operator
2. Operator includes `X-API-Key` header
3. Data Storage validates key against database
4. Populate audit event with associated user identity

**Pros**:
- ✅ Simple key-based auth
- ✅ No K8s dependency

**Cons**:
- ❌ API key management overhead
- ❌ Key rotation complexity
- ❌ Separate auth system (not aligned with CRD webhooks)

**Verdict**: ⚠️ **NOT RECOMMENDED** (adds complexity)

---

## ✅ **RECOMMENDED APPROACH: Kubernetes JWT + HTTP Middleware**

### **Why This Approach**:
1. ✅ **Consistent with CRD webhooks**: Both use K8s authentication
2. ✅ **SOC2 CC8.1 compliant**: Cryptographically verified identity
3. ✅ **No additional infrastructure**: Reuses K8s API server
4. ✅ **RBAC integration**: Can validate workflow CRUD permissions
5. ✅ **Operator-friendly**: Works with `kubectl` and K8s API

---

## 📊 **CORRECTED: Implementation Scope**

### **Kubernetes Admission Webhooks** (3 CRDs)

**Service**: `kubernaut-auth-webhook` (consolidated)
**Timeline**: 6-8 days
**Handlers**:
1. WorkflowExecution (mutating)
2. RemediationApprovalRequest (mutating)
3. NotificationRequest (validating)

**Shared Library**: `pkg/authwebhook/` ✅ (already exists)

---

### **HTTP Authentication Middleware** (1 REST API)

**Service**: `datastorage` (add middleware)
**Timeline**: 2-3 days
**Endpoints**:
1. `POST /api/v1/workflows`
2. `PATCH /api/v1/workflows/{workflowID}`
3. `PATCH /api/v1/workflows/{workflowID}/disable`

**New Library**: `pkg/datastorage/middleware/auth.go` (needs creation)

**Dependencies**:
- K8s TokenReview API client
- JWT validation logic
- Context-based user info passing

---

## 📅 **CORRECTED TIMELINE: Week 2-3**

### **Phase 1: Webhook Infrastructure** (4-5 days)

**Days 7-8: Shared Library + WE Webhook** (WE Team):
- `pkg/authwebhook/` shared library (already ✅ done)
- WorkflowExecution block clearance webhook

**Days 9-10: RAR Webhook** (RO Team):
- RemediationApprovalRequest approval/rejection webhook

**Days 11: NotificationRequest Webhook** (Notification Team):
- NotificationRequest DELETE webhook

---

### **Phase 2: HTTP Middleware for Workflow CRUD** (2-3 days)

**Days 12-13: HTTP Authentication Middleware** (Data Storage Team):
- Create `pkg/datastorage/middleware/auth.go`
- Implement K8s JWT validation
- Wire to workflow CRUD handlers
- Update audit events with authenticated actor

**Day 14: Integration Testing**:
- Test workflow create/update/disable with authenticated user
- Verify audit events contain user identity
- Test unauthorized access rejection

---

## 🎯 **CORRECTED: Operator Actions SOC2 Coverage**

| Operator Action | Authentication Method | Audit Event | SOC2 Control |
|----------------|----------------------|-------------|--------------|
| **WorkflowExecution Block Clearance** | K8s Admission Webhook | `workflowexecution.block.cleared` | CC8.1 |
| **RemediationApprovalRequest Approval** | K8s Admission Webhook | `orchestrator.approval.approved` | CC8.1 |
| **NotificationRequest Cancellation** | K8s Admission Webhook | `notification.request.cancelled` | CC8.1 |
| **Workflow CREATE** | HTTP JWT Middleware | `datastorage.workflow.created` | CC8.1 |
| **Workflow UPDATE/DISABLE** | HTTP JWT Middleware | `datastorage.workflow.updated` | CC8.1 |

**Total**: 5 operator actions (3 via webhook, 2 via HTTP middleware)

---

## 🔧 **ACTION ITEMS: Fix Incorrect Documentation**

### **1. Revert DD-WEBHOOK-001 v1.1 Changes**

**File**: `docs/architecture/decisions/DD-WEBHOOK-001-crd-webhook-requirements-matrix.md`

**Changes Needed**:
- ❌ **REMOVE** RemediationWorkflow from "CRDs Requiring Webhooks" table
- ❌ **REMOVE** "Use Case 4: RemediationWorkflow Catalog CRUD Attribution"
- ❌ **REMOVE** "Sprint 4: RemediationWorkflow Webhook Implementation"
- ✅ **UPDATE** "CRDs Requiring Webhooks" to show **3 total** (not 4)
- ✅ **ADD** note: "Workflow CRUD uses HTTP middleware, not CRD webhook"

---

### **2. Update WEBHOOK_CONSOLIDATION_TRIAGE_CORRECTED.md**

**File**: `docs/development/SOC2/WEBHOOK_CONSOLIDATION_TRIAGE_CORRECTED.md`

**Changes Needed**:
- ❌ **REMOVE** RemediationWorkflow from webhook requirements
- ✅ **UPDATE** architecture diagram (3 webhook handlers, not 4)
- ✅ **ADD** section: "HTTP Middleware for REST APIs"
- ✅ **UPDATE** "4 CRDs" → "3 CRDs + 1 REST API"

---

### **3. Create New Architecture Decision**

**File**: `docs/architecture/decisions/DD-AUTH-002-http-authentication-middleware.md` (NEW)

**Purpose**: Document HTTP authentication middleware for Data Storage API workflow CRUD operations

**Contents**:
- K8s JWT validation approach
- Middleware implementation pattern
- Audit event population with authenticated actor
- RBAC integration

---

### **4. Update TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md**

**File**: `docs/development/SOC2/TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md`

**Changes Needed**:
- ✅ **UPDATE** "Day 11-12: RemediationWorkflow Webhook" → "Day 11-13: Workflow CRUD HTTP Middleware"
- ✅ **CHANGE** "Scaffold RemediationWorkflow webhook" → "Implement HTTP JWT middleware"
- ✅ **UPDATE** effort from "16h" → "20h" (HTTP middleware is more complex than webhook)

---

## 📈 **CORRECTED EFFORT ANALYSIS**

### **Original Estimate (WRONG)**:
- 4 CRD webhooks × 2 days = 8 days
- Total: 8-10 days (64-80 hours)

### **Corrected Estimate (RIGHT)**:
- 3 CRD webhooks × 1.5 days = 4.5 days
- 1 HTTP middleware × 2.5 days = 2.5 days
- Integration testing: 1 day
- **Total: 8 days (64 hours)**

**Impact**: Same total effort, but work distribution changes:
- Webhook work: 6 days (was 8 days) -2 days
- HTTP middleware work: 2.5 days (was 0 days) +2.5 days
- Net: +0.5 days (more complex HTTP auth vs. simpler webhook)

---

## ✅ **RECOMMENDATION**

### **Immediate Actions**:
1. ✅ **REVERT** DD-WEBHOOK-001 v1.1 commit (RemediationWorkflow changes)
2. ✅ **CREATE** this triage document as authoritative source
3. ✅ **CREATE** DD-AUTH-002 for HTTP middleware pattern
4. ✅ **UPDATE** WEBHOOK_CONSOLIDATION_TRIAGE_CORRECTED.md
5. ✅ **UPDATE** TRIAGE_OPERATOR_ACTIONS_SOC2_EXTENSION.md

### **Implementation Plan**:
- **Week 2 (Days 7-11)**: 3 CRD webhooks (WE, RAR, NR)
- **Week 2 (Days 12-13)**: HTTP middleware for workflow CRUD
- **Week 2 (Day 14)**: Integration testing + SOC2 validation

---

## 🎯 **CORRECTED FINAL STATUS**

**Authentication Patterns**: 2 (Webhooks + HTTP Middleware)

**Kubernetes Admission Webhooks**: 3 CRDs
1. ✅ WorkflowExecution (Status Update)
2. ✅ RemediationApprovalRequest (Status Update)
3. ✅ NotificationRequest (DELETE)

**HTTP Authentication Middleware**: 1 REST API
1. ✅ Data Storage `/api/v1/workflows` (CREATE/PATCH)

**Total Timeline**: 8 days (64 hours)

**SOC2 CC8.1 Coverage**: ✅ 5 operator actions (all authenticated)

---

**Status**: ✅ **TRIAGE COMPLETE - READY FOR IMPLEMENTATION**
**Date**: January 6, 2026
**Approval**: User identified critical error - thank you!
**Next**: Revert DD-WEBHOOK-001 v1.1 + create DD-AUTH-002

**Compliance**: SOC2 CC8.1, DD-WORKFLOW-005 v2.0, DD-AUTH-001 (webhooks only)



