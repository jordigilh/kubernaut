# DD-AUTH-002: HTTP Authentication Middleware for REST API Operations

**Date**: January 6, 2026
**Status**: ❌ **SUPERSEDED** by [DD-AUTH-003](mdc:DD-AUTH-003-externalized-authorization-sidecar.md)
**Superseded Date**: January 6, 2026
**Reason**: Externalized authorization via sidecar pattern provides cleaner separation of concerns, easier testing, and zero-trust architecture
**Purpose**: [HISTORICAL] Define HTTP authentication middleware for capturing operator identity in REST API operations
**Authority**: [SUPERSEDED] See DD-AUTH-003 for current authentication pattern
**Scope**: HTTP endpoints requiring authenticated user identity (workflow CRUD, future REST API operations)
**Related**: DD-AUTH-001 (CRD webhooks), DD-AUTH-003 (current pattern), DD-WORKFLOW-005 v2.0, SOC2 CC8.1

---

## ⚠️ **DEPRECATION NOTICE**

**This design decision has been SUPERSEDED by DD-AUTH-003 (Externalized Authorization via Sidecar Pattern).**

**Migration Required**: Services implementing DD-AUTH-002 should migrate to DD-AUTH-003.

**Why Superseded**:
1. ❌ **Auth code pollution**: Application-level JWT validation mixes auth with business logic
2. ❌ **Testing complexity**: Requires K8s JWT token mocking in unit/integration tests
3. ❌ **Limited auth methods**: Only supports K8s ServiceAccounts (no OAuth/OIDC)
4. ❌ **Not zero-trust**: Application-level auth doesn't protect at network level
5. ✅ **DD-AUTH-003 is better**: Sidecar pattern provides clean separation, easy testing, OAuth support

**Migration Path**: See [DD-AUTH-003 Migration Section](mdc:DD-AUTH-003-externalized-authorization-sidecar.md#migration-path-from-dd-auth-002)

---

## 📋 **HISTORICAL CONTEXT** (Preserved for Reference)

---

## 🎯 **DECISION**

**REST API operations that modify critical business resources SHALL capture authenticated operator identity through HTTP authentication middleware using Kubernetes ServiceAccount JWT tokens.**

**When to Use This Pattern**:
1. **REST API operations** (not CRD operations - use DD-AUTH-001 webhooks instead)
2. **Manual operator actions** (not automated controller actions)
3. **SOC2 CC8.1 attribution required** (must record WHO performed the action)
4. **Critical resource modifications** (workflow CRUD, configuration changes, etc.)

---

## 📊 **REST API Operations Requiring Authentication**

### **Current Scope (V1.0)**

| REST API Endpoint | HTTP Method | Use Case | SOC2 Control | Implementation Owner | Timeline |
|------------------|-------------|----------|--------------|---------------------|----------|
| `/api/v1/workflows` | POST | Workflow Creation | CC8.1 (Attribution) | Data Storage Team | Week 2 |
| `/api/v1/workflows/{workflowID}` | PATCH | Workflow Update | CC8.1 (Attribution) | Data Storage Team | Week 2 |
| `/api/v1/workflows/{workflowID}/disable` | PATCH | Workflow Disable | CC8.1 (Attribution) | Data Storage Team | Week 2 |

### **Future Scope (V1.1+)**

**Extensible Pattern**: This middleware design is reusable for any future REST API requiring authentication:
- Configuration management APIs
- User management APIs
- Access control policy APIs
- Critical business resource APIs

---

## 🔧 **RECOMMENDED APPROACH: Kubernetes ServiceAccount JWT**

### **Why K8s JWT Authentication**

✅ **Consistency**: Same authentication source as CRD webhooks (DD-AUTH-001)
✅ **SOC2 CC8.1 Compliant**: Cryptographically verified identity (not forgeable)
✅ **Native Integration**: Leverages existing Kubernetes authentication infrastructure
✅ **RBAC Compatible**: Can enforce Kubernetes RBAC policies on REST API access
✅ **Zero Additional Infrastructure**: No separate auth service required
✅ **Operator-Friendly**: Works with `kubectl`, K8s API clients, and service accounts

---

## 🏗️ **Architecture**

### **Authentication Flow**

```
┌────────────────────────────────────────────────────────────────┐
│ 1. Operator Creates Workflow via kubectl or K8s API Client     │
│    curl -X POST http://datastorage:8080/api/v1/workflows \    │
│      -H "Authorization: Bearer <K8s-ServiceAccount-JWT>" \     │
│      -d @workflow.json                                         │
└───────────────────────┬────────────────────────────────────────┘
                        │
                        │ HTTP Request with JWT
                        ▼
┌────────────────────────────────────────────────────────────────┐
│ 2. Data Storage Service - HTTP Authentication Middleware       │
│    pkg/datastorage/middleware/auth.go                          │
│                                                                 │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 1: Extract JWT from Authorization Header          │ │
│    │   - Parse "Bearer <token>"                             │ │
│    │   - Validate token format                              │ │
│    └────────────────────────────────────────────────────────┘ │
│                        │                                        │
│                        │ JWT Token                              │
│                        ▼                                        │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 2: Validate JWT via K8s TokenReview API           │ │
│    │   POST /apis/authentication.k8s.io/v1/tokenreviews     │ │
│    │   {                                                     │ │
│    │     "spec": { "token": "<jwt>" }                       │ │
│    │   }                                                     │ │
│    └────────────────────────────────────────────────────────┘ │
│                        │                                        │
│                        │ TokenReview Response                   │
│                        ▼                                        │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 3: Extract User Identity from TokenReview         │ │
│    │   user_info = {                                         │ │
│    │     "username": "operator@example.com",                │ │
│    │     "uid": "k8s-user-uuid",                            │ │
│    │     "groups": ["platform-admins", "system:authenticated"] │ │
│    │   }                                                     │ │
│    └────────────────────────────────────────────────────────┘ │
│                        │                                        │
│                        │ Attach to request context             │
│                        ▼                                        │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 4: Pass to Business Logic Handler                 │ │
│    │   ctx = context.WithValue(r.Context(), authUserKey, userInfo) │ │
│    │   next.ServeHTTP(w, r.WithContext(ctx))                │ │
│    └────────────────────────────────────────────────────────┘ │
└─────────────────────────┬──────────────────────────────────────┘
                          │
                          │ Authenticated Request Context
                          ▼
┌────────────────────────────────────────────────────────────────┐
│ 3. Workflow Handler (HandleCreateWorkflow, HandleUpdateWorkflow) │
│                                                                 │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 1: Extract User Identity from Context             │ │
│    │   userInfo := auth.UserFromContext(r.Context())        │ │
│    └────────────────────────────────────────────────────────┘ │
│                        │                                        │
│                        ▼                                        │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 2: Execute Business Logic                          │ │
│    │   workflow := createWorkflowFromRequest(r)             │ │
│    │   err := repo.CreateWorkflow(ctx, workflow)            │ │
│    └────────────────────────────────────────────────────────┘ │
│                        │                                        │
│                        ▼                                        │
│    ┌────────────────────────────────────────────────────────┐ │
│    │ Step 3: Emit Audit Event with Authenticated Actor      │ │
│    │   auditStore.Store(ctx, audit.Event{                   │ │
│    │     EventType:     "datastorage.workflow.created",     │ │
│    │     EventCategory: "workflow",                          │ │
│    │     EventAction:   "create",                            │ │
│    │     EventOutcome:  "success",                           │ │
│    │     ActorID:       userInfo.Username,                  │ │
│    │     ActorType:     "user",                              │ │
│    │     EventData: map[string]interface{}{                 │ │
│    │       "workflow_id": workflow.ID,                      │ │
│    │       "created_by": {                                   │ │
│    │         "username": userInfo.Username,                 │ │
│    │         "uid":      userInfo.UID,                      │ │
│    │         "groups":   userInfo.Groups,                   │ │
│    │       },                                                │ │
│    │     },                                                  │ │
│    │   })                                                    │ │
│    └────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
```

---

## 💻 **Implementation Specification**

### **Phase 1: HTTP Authentication Middleware** (Day 1)

**File**: `pkg/datastorage/middleware/auth.go`

```go
package middleware

import (
    "context"
    "fmt"
    "net/http"
    "strings"

    authv1 "k8s.io/api/authentication/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "github.com/go-logr/logr"
)

// AuthUserKey is the context key for authenticated user information
type contextKey string

const authUserKey = contextKey("auth_user")

// UserInfo contains authenticated user information from K8s TokenReview
type UserInfo struct {
    Username string   // e.g., "operator@example.com" or "system:serviceaccount:default:operator-sa"
    UID      string   // K8s UID for the user/service account
    Groups   []string // K8s groups the user belongs to
}

// TokenValidator interface allows mocking for integration tests (NO Kind required)
// Production: Uses real K8s TokenReview API
// Testing: Uses mock validator with predefined test users
type TokenValidator interface {
    ValidateToken(ctx context.Context, token string) (*UserInfo, error)
}

// K8sTokenValidator validates JWTs using real K8s TokenReview API (production)
type K8sTokenValidator struct {
    k8sClient kubernetes.Interface
    logger    logr.Logger
}

// NewK8sTokenValidator creates a production token validator
func NewK8sTokenValidator(k8sClient kubernetes.Interface, logger logr.Logger) *K8sTokenValidator {
    return &K8sTokenValidator{
        k8sClient: k8sClient,
        logger:    logger.WithName("k8s-token-validator"),
    }
}

// ValidateToken validates JWT using K8s TokenReview API (production)
func (v *K8sTokenValidator) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
    // Create TokenReview request
    tr := &authv1.TokenReview{
        Spec: authv1.TokenReviewSpec{
            Token: token,
        },
    }

    // Call K8s TokenReview API
    result, err := v.k8sClient.AuthenticationV1().TokenReviews().Create(ctx, tr, metav1.CreateOptions{})
    if err != nil {
        return nil, fmt.Errorf("TokenReview API call failed: %w", err)
    }

    // Check if token is authenticated
    if !result.Status.Authenticated {
        return nil, fmt.Errorf("token not authenticated (K8s API rejected)")
    }

    // Extract user information
    userInfo := &UserInfo{
        Username: result.Status.User.Username,
        UID:      result.Status.User.UID,
        Groups:   result.Status.User.Groups,
    }

    v.logger.V(1).Info("Token validated successfully",
        "username", userInfo.Username,
        "uid", userInfo.UID)

    return userInfo, nil
}

// MockTokenValidator validates tokens using predefined test users (integration tests)
// CRITICAL: This allows integration tests to run WITHOUT Kind cluster
type MockTokenValidator struct {
    ValidUsers map[string]*UserInfo // Map of token -> user info for testing
    logger     logr.Logger
}

// NewMockTokenValidator creates a mock validator for integration tests
func NewMockTokenValidator(validUsers map[string]*UserInfo, logger logr.Logger) *MockTokenValidator {
    return &MockTokenValidator{
        ValidUsers: validUsers,
        logger:     logger.WithName("mock-token-validator"),
    }
}

// ValidateToken validates token against predefined test users (NO K8s API call)
func (v *MockTokenValidator) ValidateToken(ctx context.Context, token string) (*UserInfo, error) {
    userInfo, ok := v.ValidUsers[token]
    if !ok {
        return nil, fmt.Errorf("invalid test token")
    }

    v.logger.V(1).Info("Mock token validated",
        "username", userInfo.Username,
        "uid", userInfo.UID)

    return userInfo, nil
}

// AuthMiddleware validates Kubernetes ServiceAccount JWTs for REST API requests
// Uses TokenValidator interface for dependency injection (production or mock)
type AuthMiddleware struct {
    validator TokenValidator // Interface allows mocking for tests
    logger    logr.Logger
}

// NewAuthMiddleware creates a new authentication middleware
// validator can be K8sTokenValidator (production) or MockTokenValidator (testing)
func NewAuthMiddleware(validator TokenValidator, logger logr.Logger) *AuthMiddleware {
    return &AuthMiddleware{
        validator: validator,
        logger:    logger.WithName("auth-middleware"),
    }
}

// Middleware returns the HTTP middleware handler
func (m *AuthMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract JWT from Authorization header
        token, err := extractBearerToken(r)
        if err != nil {
            m.logger.V(1).Info("Missing or invalid Authorization header",
                "path", r.URL.Path,
                "error", err.Error())
            http.Error(w, "Unauthorized: Missing or invalid Authorization header", http.StatusUnauthorized)
            return
        }

        // Validate token using injected validator (production or mock)
        userInfo, err := m.validator.ValidateToken(r.Context(), token)
        if err != nil {
            m.logger.Error(err, "Token validation failed",
                "path", r.URL.Path)
            http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
            return
        }

        m.logger.V(1).Info("Request authenticated",
            "path", r.URL.Path,
            "username", userInfo.Username,
            "uid", userInfo.UID)

        // Attach authenticated user to request context
        ctx := context.WithValue(r.Context(), authUserKey, userInfo)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// extractBearerToken extracts the JWT token from the Authorization header
// Expected format: "Authorization: Bearer <token>"
func extractBearerToken(r *http.Request) (string, error) {
    authHeader := r.Header.Get("Authorization")
    if authHeader == "" {
        return "", fmt.Errorf("missing Authorization header")
    }

    parts := strings.Split(authHeader, " ")
    if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
        return "", fmt.Errorf("invalid Authorization header format (expected 'Bearer <token>')")
    }

    return parts[1], nil
}

// UserFromContext extracts the authenticated user from request context
// Returns nil if no authenticated user in context
func UserFromContext(ctx context.Context) *UserInfo {
    userInfo, ok := ctx.Value(authUserKey).(*UserInfo)
    if !ok {
        return nil
    }
    return userInfo
}
```

---

### **Production vs. Testing Configuration**

#### **Production: Real K8s TokenReview Validator**

```go
// pkg/datastorage/server/server.go
func NewServer(cfg *config.Config, logger logr.Logger) (*Server, error) {
    // ... existing database, repository setup ...

    // DD-AUTH-002: Create K8s client for JWT validation (production)
    k8sConfig, err := rest.InClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
    }
    k8sClient, err := kubernetes.NewForConfig(k8sConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create K8s client: %w", err)
    }

    // Production: Real K8s TokenReview validator
    tokenValidator := dsmiddleware.NewK8sTokenValidator(k8sClient, logger)
    authMiddleware := dsmiddleware.NewAuthMiddleware(tokenValidator, logger)

    srv := &Server{
        // ... existing fields ...
        authMiddleware: authMiddleware, // Store for use in Handler()
    }

    return srv, nil
}
```

#### **Integration Tests: Mock Validator (NO Kind Required)**

**CRITICAL**: Integration tests use `MockTokenValidator` - **NO Kubernetes cluster required!**

```go
// test/integration/datastorage/workflow_auth_integration_test.go
var _ = Describe("Workflow CRUD Authentication", func() {
    var (
        ctx            context.Context
        server         *httptest.Server
        datastorageURL string
        testToken      string
    )

    BeforeEach(func() {
        ctx = context.Background()
        testToken = "test-operator-token"

        // CRITICAL: Mock validator - NO Kind cluster needed!
        // Integration tests run with httptest.NewServer (pure Go, no K8s)
        mockValidator := dsmiddleware.NewMockTokenValidator(
            map[string]*dsmiddleware.UserInfo{
                testToken: {
                    Username: "test-operator@example.com",
                    UID:      "test-uuid-123",
                    Groups:   []string{"platform-admins", "system:authenticated"},
                },
                "test-admin-token": {
                    Username: "admin@example.com",
                    UID:      "admin-uuid-456",
                    Groups:   []string{"system:masters"},
                },
            },
            logger,
        )

        authMiddleware := dsmiddleware.NewAuthMiddleware(mockValidator, logger)

        // Create test handler with mock auth middleware
        handler := createTestHandler(authMiddleware)
        server = httptest.NewServer(handler) // NO Kind - just httptest!
        datastorageURL = server.URL
    })

    AfterEach(func() {
        server.Close()
    })

    It("should create workflow with authenticated user", func() {
        workflowJSON := `{"workflow_id": "test-workflow", "title": "Test Workflow"}`

        req, err := http.NewRequest(http.MethodPost, datastorageURL+"/api/v1/workflows",
            strings.NewReader(workflowJSON))
        Expect(err).ToNot(HaveOccurred())
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testToken))

        client := &http.Client{}
        resp, err := client.Do(req)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusCreated))

        // Verify audit event contains authenticated actor
        // Query audit events from Data Storage
        // Assert actor_id = "test-operator@example.com"
    })

    It("should reject workflow creation without authentication", func() {
        workflowJSON := `{"workflow_id": "test-workflow"}`

        req, err := http.NewRequest(http.MethodPost, datastorageURL+"/api/v1/workflows",
            strings.NewReader(workflowJSON))
        Expect(err).ToNot(HaveOccurred())
        req.Header.Set("Content-Type", "application/json")
        // NO Authorization header

        client := &http.Client{}
        resp, err := client.Do(req)
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
    })
})
```

---

### **Phase 2: Wire Middleware to Data Storage Server** (Day 1)

**File**: `pkg/datastorage/server/server.go`

```go
// Handler returns the configured HTTP handler for the server
func (s *Server) Handler() http.Handler {
    r := chi.NewRouter()

    // Existing middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(s.loggingMiddleware)
    r.Use(s.panicRecoveryMiddleware)
    r.Use(cors.Handler(/* ... */))

    // DD-AUTH-002: HTTP authentication middleware for workflow CRUD operations
    // Only apply to authenticated routes (not health/metrics)
    // authMiddleware created in NewServer() with production or mock validator
    r.Route("/api/v1", func(r chi.Router) {
        // Unauthenticated routes (audit ingestion, queries, etc.)
        r.Get("/incidents", s.handler.ListIncidents)
        r.Post("/audit/events", s.handleCreateAuditEvent)
        // ... other unauthenticated routes ...

        // DD-AUTH-002: Authenticated workflow CRUD routes (SOC2 CC8.1)
        r.Group(func(r chi.Router) {
            r.Use(authMiddleware.Middleware) // Apply authentication middleware

            // Workflow CRUD operations requiring authenticated user identity
            r.Post("/workflows", s.handler.HandleCreateWorkflow)
            r.Patch("/workflows/{workflowID}", s.handler.HandleUpdateWorkflow)
            r.Patch("/workflows/{workflowID}/disable", s.handler.HandleDisableWorkflow)
        })
    })

    return r
}
```

---

### **Phase 3: Update Workflow Handlers to Capture Authenticated Actor** (Day 2)

**File**: `pkg/datastorage/server/workflow_handlers.go`

```go
// HandleCreateWorkflow creates a new workflow in the catalog (DD-AUTH-002: SOC2 CC8.1)
func (h *Handler) HandleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // DD-AUTH-002: Extract authenticated user from middleware context
    userInfo := dsmiddleware.UserFromContext(ctx)
    if userInfo == nil {
        // This should never happen if middleware is properly configured
        h.logger.Error(nil, "Missing authenticated user in context (middleware misconfigured?)")
        http.Error(w, "Internal Server Error: Authentication context missing", http.StatusInternalServerError)
        return
    }

    h.logger.V(1).Info("Processing authenticated workflow creation",
        "username", userInfo.Username,
        "uid", userInfo.UID)

    // Parse and validate request body
    var workflow models.RemediationWorkflow
    if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
        h.logger.Error(err, "Failed to decode workflow request")
        http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
        return
    }

    // Business logic: Create workflow in repository
    if err := h.workflowRepo.CreateWorkflow(ctx, &workflow); err != nil {
        h.logger.Error(err, "Failed to create workflow",
            "workflow_id", workflow.WorkflowID)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    // DD-AUTH-002: Emit audit event with authenticated actor (SOC2 CC8.1)
    auditEvent := audit.Event{
        EventType:     "datastorage.workflow.created",
        EventCategory: audit.EventCategoryWorkflow,
        EventAction:   "create",
        EventOutcome:  audit.EventOutcomeSuccess,
        ActorID:       userInfo.Username,  // Authenticated user from JWT
        ActorType:     audit.ActorTypeUser,
        EventData: map[string]interface{}{
            "workflow_id":      workflow.WorkflowID,
            "workflow_version": workflow.Version,
            "signal_type":      workflow.SignalType,
            "created_by": map[string]interface{}{
                "username": userInfo.Username,
                "uid":      userInfo.UID,
                "groups":   userInfo.Groups,
            },
        },
    }

    if err := h.auditStore.Store(ctx, auditEvent); err != nil {
        // Non-fatal: Log error but continue (audit write failure should not block business operation)
        h.logger.Error(err, "Failed to emit workflow creation audit event",
            "workflow_id", workflow.WorkflowID)
    }

    // Return success response
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(workflow)
}

// HandleUpdateWorkflow updates mutable workflow fields (DD-AUTH-002: SOC2 CC8.1)
func (h *Handler) HandleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
    // Similar implementation with authenticated actor capture
    // ...
}

// HandleDisableWorkflow disables a workflow (DD-AUTH-002: SOC2 CC8.1)
func (h *Handler) HandleDisableWorkflow(w http.ResponseWriter, r *http.Request) {
    // Similar implementation with authenticated actor capture
    // ...
}
```

---

## 📋 **Configuration Requirements**

### **Data Storage Service Configuration**

**File**: `pkg/datastorage/config/config.go`

```go
type Config struct {
    // ... existing fields ...

    // DD-AUTH-002: Kubernetes client configuration for JWT validation
    KubeConfig string `yaml:"kube_config" env:"KUBE_CONFIG"`  // Path to kubeconfig (optional, uses in-cluster config if empty)
}
```

**Kubernetes Client Initialization**:
- **In-cluster**: Use `rest.InClusterConfig()` when running in K8s cluster (default)
- **Out-of-cluster**: Use `clientcmd.BuildConfigFromFlags()` for local development

---

## 🔐 **SOC2 Compliance Requirements**

### **SOC2 CC8.1: Attribution Control**

| Requirement | Implementation | Validation |
|-------------|----------------|------------|
| **Cryptographic Verification** | K8s TokenReview API validates JWT signature | JWT validation via K8s API server |
| **Unforgeable Identity** | JWT signed by K8s API server, cannot be forged | TokenReview ensures authenticity |
| **User Identity Captured** | `username`, `uid`, `groups` from TokenReview response | Audit events contain complete user identity |
| **Audit Trail** | All workflow CRUD operations emit audit events with authenticated actor | Audit events query-able via Data Storage API |
| **Completeness** | 100% workflow operations require authentication | Middleware enforces authentication for all workflow routes |

**Compliance Validation Checklist**:
- [ ] All workflow CRUD endpoints require `Authorization: Bearer <token>` header
- [ ] JWT validation succeeds for valid K8s ServiceAccount tokens
- [ ] JWT validation fails for invalid/expired tokens (returns HTTP 401)
- [ ] Audit events contain `actor_id` and `event_data.created_by` with user identity
- [ ] Unauthorized requests (missing/invalid token) are rejected

---

## 🧪 **Testing Strategy**

### **Testing Philosophy: Mock Validator for Integration Tests**

**CRITICAL DESIGN DECISION**: Integration tests use `MockTokenValidator` to **avoid requiring Kind cluster**.

| Test Tier | Validator Type | Infrastructure | Purpose |
|-----------|---------------|----------------|---------|
| **Unit Tests** | MockTokenValidator | None (pure Go) | Test middleware logic in isolation |
| **Integration Tests** | MockTokenValidator | httptest.NewServer | Test workflow CRUD + audit with fast execution |
| **E2E Tests** | K8sTokenValidator | Kind cluster | Final validation with real K8s JWT |

### **Unit Tests** (18 tests) - Mock Validator

**File**: `pkg/datastorage/middleware/auth_test.go`

```go
var _ = Describe("AuthMiddleware", func() {
    var (
        mockValidator *dsmiddleware.MockTokenValidator
        middleware    *dsmiddleware.AuthMiddleware
    )

    BeforeEach(func() {
        // Unit tests: Use mock validator (NO K8s dependency)
        mockValidator = dsmiddleware.NewMockTokenValidator(
            map[string]*dsmiddleware.UserInfo{
                "valid-token": {
                    Username: "test-user@example.com",
                    UID:      "test-uid",
                    Groups:   []string{"test-group"},
                },
            },
            logger,
        )
        middleware = dsmiddleware.NewAuthMiddleware(mockValidator, logger)
    })

    Describe("JWT Extraction", func() {
        It("should extract valid Bearer token", func() {
            // Test extractBearerToken() logic
        })
        It("should reject missing Authorization header", func() {})
        It("should reject invalid Authorization format", func() {})
    })

    Describe("Token Validation (Mock)", func() {
        It("should accept predefined valid token", func() {
            userInfo, err := mockValidator.ValidateToken(ctx, "valid-token")
            Expect(err).ToNot(HaveOccurred())
            Expect(userInfo.Username).To(Equal("test-user@example.com"))
        })
        It("should reject unknown token", func() {
            _, err := mockValidator.ValidateToken(ctx, "invalid-token")
            Expect(err).To(HaveOccurred())
        })
    })

    Describe("Context Management", func() {
        It("should attach user info to request context", func() {})
        It("should allow handlers to extract user from context", func() {})
    })
})
```

### **Integration Tests** (3 tests) - Mock Validator (NO Kind)

**File**: `test/integration/datastorage/workflow_auth_integration_test.go`

**CRITICAL**: Uses `httptest.NewServer` with `MockTokenValidator` - **NO Kind cluster required!**

```go
var _ = Describe("Workflow CRUD Authentication", func() {
    var (
        mockValidator *dsmiddleware.MockTokenValidator
        server        *httptest.Server
        testToken     string
    )

    BeforeEach(func() {
        testToken = "test-operator-token"

        // Integration tests: Mock validator (NO Kind dependency!)
        mockValidator = dsmiddleware.NewMockTokenValidator(
            map[string]*dsmiddleware.UserInfo{
                testToken: {
                    Username: "test-operator@example.com",
                    UID:      "test-uuid-123",
                    Groups:   []string{"platform-admins"},
                },
            },
            logger,
        )

        authMiddleware := dsmiddleware.NewAuthMiddleware(mockValidator, logger)
        handler := createTestHandler(authMiddleware)
        server = httptest.NewServer(handler) // httptest, NOT Kind!
    })

    It("should create workflow with authenticated user", func() {
        // POST /api/v1/workflows with Authorization: Bearer <testToken>
        // Verify workflow created
        // Verify audit event contains authenticated actor
        // NO Kind cluster needed!
    })

    It("should reject workflow creation without authentication", func() {
        // POST /api/v1/workflows without Authorization header
        // Verify HTTP 401 Unauthorized
    })

    It("should reject workflow creation with invalid token", func() {
        // POST /api/v1/workflows with unknown token
        // Verify HTTP 401 Unauthorized
    })
})
```

### **E2E Tests** (2 tests) - Real K8s TokenReview (Kind Required)

**File**: `test/e2e/datastorage/09_workflow_crud_auth_test.go`

**Infrastructure**: Requires Kind cluster (uses real K8s `TokenReview` API)

```go
var _ = Describe("E2E: Workflow CRUD Attribution", func() {
    var (
        k8sClient      kubernetes.Interface
        tokenValidator *dsmiddleware.K8sTokenValidator
        realJWT        string
    )

    BeforeEach(func() {
        // E2E: Use REAL K8s client and validator
        k8sConfig, err := rest.InClusterConfig()
        Expect(err).ToNot(HaveOccurred())

        k8sClient, err = kubernetes.NewForConfig(k8sConfig)
        Expect(err).ToNot(HaveOccurred())

        tokenValidator = dsmiddleware.NewK8sTokenValidator(k8sClient, logger)

        // Get real ServiceAccount JWT token from Kind cluster
        realJWT = getServiceAccountToken("default", "test-operator-sa")
    })

    It("should capture operator identity for workflow creation", func() {
        // E2E: Create workflow with REAL K8s JWT token
        // POST /api/v1/workflows with Authorization: Bearer <realJWT>
        // TokenReview API validates against REAL K8s API server
        // Verify workflow created
        // Verify audit event shows authenticated operator username
    })

    It("should capture operator identity for workflow disable", func() {
        // E2E: Disable workflow with REAL K8s JWT token
        // PATCH /api/v1/workflows/{id}/disable
        // Verify audit event shows authenticated operator username
    })
})
```

**Why E2E Needs Kind**:
- E2E tests validate the **complete production flow** with real K8s TokenReview API
- Integration tests already validated business logic (fast, no Kind)
- E2E provides final confidence that production auth works end-to-end

---

### **Testing Benefits: Interface-Based Mock Approach**

| Aspect | Without Mock (Always Real K8s) | With Mock Validator (This Design) |
|--------|--------------------------------|-----------------------------------|
| **Integration Test Infrastructure** | ❌ Requires Kind cluster | ✅ httptest.NewServer only |
| **Integration Test Startup Time** | ❌ +30-60s per suite | ✅ <1s per suite |
| **Integration Test Complexity** | ❌ High (K8s setup, teardown) | ✅ Low (pure Go) |
| **Developer Experience** | ❌ Slow feedback loop | ✅ Fast iterations |
| **CI/CD Pipeline** | ❌ Heavier (Kind for integration) | ✅ Lighter (Kind only for E2E) |
| **Production Security** | ✅ Real JWT validation | ✅ Real JWT validation (same) |
| **SOC2 Compliance** | ✅ CC8.1 compliant | ✅ CC8.1 compliant (same) |

**Key Insight**: Using `TokenValidator` interface provides **same production security** while making **integration tests 30-60x faster**.

---

## 📅 **Implementation Timeline**

### **Week 2: Days 12-14** (Data Storage Team)

**Day 12** (8 hours):
- Implement `pkg/datastorage/middleware/auth.go` (JWT middleware)
- Write 18 unit tests for middleware
- Wire middleware to Data Storage server

**Day 13** (8 hours):
- Update workflow handlers (`HandleCreateWorkflow`, `HandleUpdateWorkflow`, `HandleDisableWorkflow`)
- Populate audit events with authenticated actor
- Write 3 integration tests

**Day 14** (4 hours):
- Write 2 E2E tests
- SOC2 CC8.1 compliance validation
- Documentation

**Total Effort**: 2.5 days (20 hours)

---

## 🚫 **Anti-Patterns to Avoid**

### **❌ Anti-Pattern 1: Custom X-User-Identity Header**

**Why Wrong**: User identity can be forged (not cryptographically verified)

**Correct Approach**: Use K8s JWT with TokenReview API

---

### **❌ Anti-Pattern 2: Hardcoded Service Account**

**Why Wrong**: All operations appear to come from same user (no attribution)

**Correct Approach**: Each operator uses their own ServiceAccount or user credentials

---

### **❌ Anti-Pattern 3: Optional Authentication**

**Why Wrong**: Some workflow operations might bypass authentication

**Correct Approach**: Middleware enforces authentication for ALL workflow CRUD routes

---

## 🔗 **Integration with Other Systems**

### **CRD Webhooks (DD-AUTH-001)**

**Consistency**: Both patterns use K8s authentication as the source of truth
- **CRDs**: Extract user from `req.UserInfo` in admission webhook
- **REST API**: Validate JWT via TokenReview API

**Audit Events**: Same `actor_id` format in both patterns
- Example: `"operator@example.com"` or `"system:serviceaccount:default:operator-sa"`

---

### **RBAC Integration (Future Enhancement)**

**Optional**: Middleware can enforce K8s RBAC policies on REST API access

```go
// Optional RBAC check in middleware
func (m *AuthMiddleware) checkRBAC(ctx context.Context, userInfo *UserInfo, resource string, verb string) error {
    // Create SubjectAccessReview
    sar := &authorizationv1.SubjectAccessReview{
        Spec: authorizationv1.SubjectAccessReviewSpec{
            User:   userInfo.Username,
            Groups: userInfo.Groups,
            ResourceAttributes: &authorizationv1.ResourceAttributes{
                Namespace: "kubernaut-system",
                Verb:      verb,      // "create", "update", "delete"
                Resource:  resource,  // "workflows"
            },
        },
    }

    // Call K8s SubjectAccessReview API
    result, err := m.k8sClient.AuthorizationV1().SubjectAccessReviews().Create(ctx, sar, metav1.CreateOptions{})
    if err != nil {
        return fmt.Errorf("SubjectAccessReview API call failed: %w", err)
    }

    if !result.Status.Allowed {
        return fmt.Errorf("user %s is not authorized to %s %s", userInfo.Username, verb, resource)
    }

    return nil
}
```

---

## ✅ **Success Criteria**

### **Implementation Complete When**:
1. ✅ `pkg/datastorage/middleware/auth.go` implemented with JWT validation
2. ✅ Middleware wired to Data Storage server for workflow CRUD routes
3. ✅ Workflow handlers populate audit events with authenticated actor
4. ✅ 18 unit tests passing
5. ✅ 3 integration tests passing
6. ✅ 2 E2E tests passing
7. ✅ SOC2 CC8.1 compliance validated
8. ✅ Documentation complete

### **Acceptance Criteria**:
- ✅ Valid K8s ServiceAccount JWT → workflow creation succeeds + audit event with user identity
- ✅ Missing JWT → HTTP 401 Unauthorized
- ✅ Invalid JWT → HTTP 401 Unauthorized
- ✅ Audit events contain `actor_id` and `event_data.created_by` with complete user identity

---

## 📚 **References**

- [DD-AUTH-001](./DD-AUTH-001-shared-authentication-webhook.md) - CRD webhook authentication
- [DD-WORKFLOW-005 v2.0](./DD-WORKFLOW-005-automated-schema-extraction.md) - Workflow registration architecture
- [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) - Audit event schema
- [WEBHOOK_WORKFLOW_CRUD_ATTRIBUTION_TRIAGE.md](../../development/SOC2/WEBHOOK_WORKFLOW_CRUD_ATTRIBUTION_TRIAGE.md) - Architecture triage
- [Kubernetes TokenReview API](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#tokenreview-v1-authentication-k8s-io)

---

**Status**: ❌ **SUPERSEDED** by DD-AUTH-003
**Date**: January 6, 2026
**Superseded Date**: January 6, 2026
**Implementation**: ❌ DO NOT IMPLEMENT - Use DD-AUTH-003 instead
**Owner**: N/A (superseded)

**Compliance**: SOC2 CC8.1, DD-AUTH-001 (consistency), DD-WORKFLOW-005 v2.0

---

## 🔄 **Why This Approach Was Superseded**

### **Problems with DD-AUTH-002 (Application-Level JWT Validation)**

This design decision was superseded on the same day it was created because architectural review revealed fundamental issues:

1. **Auth Code Pollution** ❌
   - Every service needs auth middleware
   - Business logic mixed with authentication concerns
   - Violates separation of concerns principle

2. **Testing Complexity** ❌
   - Unit tests require K8s JWT token mocking
   - Integration tests need TokenReview API mocking
   - Makes testing harder than necessary
   - Example from the problem:
     > "the idea of adding k8s JWT tokens to the business layer was not working for me"

3. **Limited Authentication Methods** ❌
   - Only supports K8s ServiceAccounts
   - Cannot support OAuth/OIDC for external users
   - Hard to extend to new auth providers

4. **Not Zero-Trust** ❌
   - Authentication only at application level
   - Services can be accessed directly if network policy isn't perfect
   - No defense-in-depth

5. **Deployment Coupling** ❌
   - Auth changes require service redeployment
   - Can't update auth independently
   - Slower iteration on auth improvements

### **Solution: DD-AUTH-003 (Externalized Sidecar Pattern)**

DD-AUTH-003 solves all these problems by externalizing authorization:

| Aspect | DD-AUTH-002 (Old) | DD-AUTH-003 (New) |
|--------|-------------------|-------------------|
| **Where auth happens** | In application code | In sidecar proxy |
| **Services auth code** | ~200 lines per service | 0 lines (clean!) |
| **Unit test complexity** | K8s JWT mocking required | Just set headers |
| **Auth methods** | K8s only | OAuth/OIDC + K8s |
| **Zero-trust** | No (app-level only) | Yes (network-enforced) |
| **Separation of concerns** | Poor (mixed) | Excellent (external) |

### **Migration Impact**

**If DD-AUTH-002 was already implemented**:
- Remove auth middleware from services (~200 lines per service)
- Add OAuth2-Proxy sidecar to deployments
- Replace JWT validation with header reading
- Estimated effort: ~1 week per service
- **Result**: Simpler code, easier testing, better security

**Since DD-AUTH-002 was never implemented**:
- No migration needed ✅
- Start directly with DD-AUTH-003 ✅
- Avoid technical debt ✅

---

## 📚 **Historical Documentation Preserved**

The content above is preserved for historical reference and architectural learning.

**See [DD-AUTH-003](mdc:DD-AUTH-003-externalized-authorization-sidecar.md) for the current, approved authentication architecture.**

