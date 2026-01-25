# DD-AUTH-005: DataStorage Client Authentication Pattern

**Date**: January 7, 2026
**Status**: ✅ **APPROVED** - Authoritative V1.0 Pattern
**Builds On**: DD-AUTH-004 (OpenShift OAuth-Proxy for Legal Hold)
**Confidence**: 95%
**Last Reviewed**: January 7, 2026

---

## 🎯 **DECISION**

**All services (7 Go + 1 Python) SHALL use transport layer injection to authenticate with DataStorage REST API. The transport layer SHALL automatically inject authentication headers based on environment (integration/E2E/production) WITHOUT modifying OpenAPI-generated clients.**

**Pattern Authority**: This DD is the AUTHORITATIVE blueprint for DataStorage client instantiation across all services and all environments.

**Scope**:
- **7 Go Services**: Gateway, AIAnalysis, WorkflowExecution, RemediationOrchestrator, Notification, AuthWebhook, SignalProcessing
- **1 Python Service**: holmesgpt-api
- **All DataStorage API Endpoints**: `/api/v1/audit/*`, `/api/v1/storage/*`
- **All Environments**: Integration tests, E2E tests, Production

---

## 📊 **Context & Problem**

### **Business Requirements**

1. **SOC2 CC6.1 (Access Controls)**: All DataStorage API calls require authenticated user identification
2. **OpenAPI Client Usage**: Services MUST use OpenAPI-generated clients (DD-HAPI-003)
3. **Testing Flexibility**: Integration tests mock headers, E2E tests use real tokens
4. **Production Simplicity**: Services automatically read ServiceAccount tokens from filesystem
5. **No Generated Code Modifications**: OpenAPI clients remain pristine (DD-HAPI-003, DD-HAPI-005)

### **Problem Statement**

**Current Implementation (Direct HTTP Calls)**:
- Services make raw HTTP requests to DataStorage
- Inconsistent authentication patterns across services
- Testing requires complex mocking infrastructure
- No standardized authentication layer

**Why This Doesn't Work**:
- ❌ **Inconsistency**: Each service implements auth differently
- ❌ **Testing Complexity**: Hard to switch between mock and real auth
- ❌ **OpenAPI Violation**: Not using generated clients (DD-HAPI-003)
- ❌ **Maintenance Burden**: Auth changes require updating 8 services
- ❌ **Production Risk**: Easy to forget token injection in new services

**Critical Constraint from User**:
> "We must use the OpenAPI audit client for integration and E2E tests. Do NOT modify the auto-generated code."

**Question**: How do we inject authentication headers into OpenAPI-generated clients without modifying the generated code?

**Answer**: Transport layer injection using `http.RoundTripper` (Go) and `requests.Session` (Python).

---

## 🔍 **Alternatives Considered**

### **Alternative 1: Modify Generated OpenAPI Clients** ❌ REJECTED

**Approach**: Add authentication logic directly in generated client code

```go
// BAD: Modifying generated code
func (c *ClientImpl) PlaceLegalHold(ctx context.Context, req PlaceLegalHoldRequest) (*LegalHoldResponse, error) {
    // Add authentication here
    token := readServiceAccountToken()
    headers := map[string]string{
        "Authorization": "Bearer " + token,
    }
    // ... rest of implementation
}
```

**Why Rejected**:
- ❌ **Violates DD-HAPI-003**: Generated clients must remain pristine
- ❌ **Violates DD-HAPI-005**: Breaks auto-regeneration workflow
- ❌ **Manual regeneration**: Every OpenAPI spec change requires manual patching
- ❌ **Testing nightmare**: Cannot switch between mock and real auth
- ❌ **User mandate**: "Do NOT modify the auto-generated code"

**Confidence**: 100% rejection (violates established patterns)

---

### **Alternative 2: Custom Wrapper Functions** ⚠️ PARTIAL SOLUTION

**Approach**: Create wrapper functions around OpenAPI client

```go
// Wrapper approach
func PlaceLegalHoldWithAuth(ctx context.Context, client *DataStorageClient, req PlaceLegalHoldRequest) (*LegalHoldResponse, error) {
    token := readServiceAccountToken()
    ctx = contextWithAuth(ctx, token)
    return client.PlaceLegalHold(ctx, req)
}
```

**Pros**:
- ✅ Generated client remains pristine
- ✅ Centralized auth logic

**Cons**:
- ❌ **Function explosion**: Need wrapper for EVERY DataStorage endpoint
- ❌ **Maintenance burden**: Add wrapper for each new endpoint
- ❌ **Context abuse**: Storing headers in context is anti-pattern
- ❌ **Testing complexity**: Still need to mock context headers

**Why Rejected**: Doesn't scale to all endpoints, high maintenance burden

**Confidence**: 85% rejection (unscalable)

---

### **Alternative 3: Transport Layer Injection** ✅ APPROVED

**Approach**: Use `http.RoundTripper` (Go) and `requests.Session` (Python) to inject headers

**Go Implementation**:
```go
// Create HTTP client with custom transport
transport := auth.NewServiceAccountTransport()
httpClient := &http.Client{Transport: transport}

// Pass to OpenAPI client (NO MODIFICATION TO GENERATED CODE)
datastorageClient := datastorage.NewClientWithResponses(
    "http://datastorage:8080",
    datastorage.WithHTTPClient(httpClient),
)

// Use normally - transport injects headers automatically
resp, err := datastorageClient.PlaceLegalHoldWithResponse(ctx, req)
```

**Python Implementation**:
```python
# Create Requests session with custom auth
session = ServiceAccountAuthSession()

# Pass to OpenAPI client (NO MODIFICATION TO GENERATED CODE)
ds_client = DataStorageClient(
    base_url="http://datastorage:8080",
    session=session
)

# Use normally - session injects headers automatically
response = ds_client.place_legal_hold(correlation_id="...", reason="...")
```

**Pros**:
- ✅ **Zero generated code changes**: OpenAPI clients remain pristine
- ✅ **Environment-aware**: Different transports for integration/E2E/production
- ✅ **Transparent**: Services use clients normally, transport handles auth
- ✅ **Testable**: Easy to switch between mock and real auth
- ✅ **Scalable**: Works for ALL endpoints automatically
- ✅ **Standard pattern**: `http.RoundTripper` is idiomatic Go
- ✅ **Reusable**: Same pattern across all 8 services

**Cons**:
- ⚠️ **New abstraction**: Developers need to understand transport layer
- ⚠️ **Python learning curve**: `requests.Session` less familiar

**Why Approved**: Best balance of simplicity, scalability, and adherence to OpenAPI standards

**Confidence**: 95% approval (proven pattern)

---

## 🏗️ **Implementation Architecture**

### **Component 1: Go Auth Transport**

**File**: `pkg/shared/auth/transport.go`

```go
package auth

import (
	"net/http"
	"os"
	"sync"
	"time"
)

// AuthTransportMode defines how the transport handles authentication
type AuthTransportMode int

const (
	// ModeServiceAccount reads token from filesystem (for services and E2E)
	ModeServiceAccount AuthTransportMode = iota

	// ModeStaticToken uses a provided static token (for E2E tests with TokenRequest)
	ModeStaticToken

	// ModeMockUser directly injects X-Auth-Request-User header (for integration tests)
	ModeMockUser
)

// AuthTransport is an http.RoundTripper that handles authentication for DataStorage API calls.
//
// It supports three modes:
// 1. ModeServiceAccount: Reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
//    - Used by: Services in E2E/Production
//    - Injects: Authorization: Bearer <token>
//
// 2. ModeStaticToken: Uses provided token
//    - Used by: E2E tests (via TokenRequest API)
//    - Injects: Authorization: Bearer <token>
//
// 3. ModeMockUser: Directly injects X-Auth-Request-User header
//    - Used by: Integration tests (no oauth-proxy)
//    - Injects: X-Auth-Request-User: <mockUserID>
//
// Usage (Services):
//   transport := auth.NewServiceAccountTransport()
//   client := &http.Client{Transport: transport}
//   datastorageClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(client))
//
// Usage (E2E Tests):
//   token, _ := getServiceAccountToken(...)
//   transport := auth.NewStaticTokenTransport(token)
//   client := &http.Client{Transport: transport}
//   datastorageClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(client))
//
// Usage (Integration Tests):
//   transport := auth.NewMockUserTransport("test-user@example.com")
//   client := &http.Client{Transport: transport}
//   datastorageClient := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(client))
type AuthTransport struct {
	base      http.RoundTripper
	mode      AuthTransportMode
	tokenPath string

	// For ModeStaticToken
	staticToken string

	// For ModeMockUser
	mockUserID string

	// Token caching (for ModeServiceAccount)
	tokenCache      string
	tokenCacheTime  time.Time
	tokenCacheMutex sync.RWMutex
}

// NewServiceAccountTransport creates a transport that reads tokens from the ServiceAccount filesystem
// Used by services in E2E/Production environments
func NewServiceAccountTransport() *AuthTransport {
	return NewServiceAccountTransportWithBase(http.DefaultTransport)
}

func NewServiceAccountTransportWithBase(base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:      base,
		mode:      ModeServiceAccount,
		tokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
	}
}

// NewStaticTokenTransport creates a transport that uses a provided static token
// Used by E2E tests with TokenRequest API
func NewStaticTokenTransport(token string) *AuthTransport {
	return NewStaticTokenTransportWithBase(token, http.DefaultTransport)
}

func NewStaticTokenTransportWithBase(token string, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:        base,
		mode:        ModeStaticToken,
		staticToken: token,
	}
}

// NewMockUserTransport creates a transport that directly injects X-Auth-Request-User header
// Used by integration tests (no oauth-proxy, no token validation)
func NewMockUserTransport(userID string) *AuthTransport {
	return NewMockUserTransportWithBase(userID, http.DefaultTransport)
}

func NewMockUserTransportWithBase(userID string, base http.RoundTripper) *AuthTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &AuthTransport{
		base:       base,
		mode:       ModeMockUser,
		mockUserID: userID,
	}
}

// RoundTrip implements http.RoundTripper
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid mutating original
	reqClone := req.Clone(req.Context())

	switch t.mode {
	case ModeServiceAccount:
		// Read token from filesystem (with caching)
		token := t.getServiceAccountToken()
		if token != "" {
			reqClone.Header.Set("Authorization", "Bearer "+token)
		}

	case ModeStaticToken:
		// Use provided static token
		if t.staticToken != "" {
			reqClone.Header.Set("Authorization", "Bearer "+t.staticToken)
		}

	case ModeMockUser:
		// Directly inject X-Auth-Request-User header (bypass oauth-proxy)
		// This simulates what oauth-proxy would inject after validating the token
		if t.mockUserID != "" {
			reqClone.Header.Set("X-Auth-Request-User", t.mockUserID)
		}
	}

	return t.base.RoundTrip(reqClone)
}

// getServiceAccountToken retrieves the ServiceAccount token with 5-minute caching
func (t *AuthTransport) getServiceAccountToken() string {
	// Check cache first (read lock)
	t.tokenCacheMutex.RLock()
	if time.Since(t.tokenCacheTime) < 5*time.Minute && t.tokenCache != "" {
		cached := t.tokenCache
		t.tokenCacheMutex.RUnlock()
		return cached
	}
	t.tokenCacheMutex.RUnlock()

	// Cache miss or expired - read from filesystem (write lock)
	t.tokenCacheMutex.Lock()
	defer t.tokenCacheMutex.Unlock()

	// Double-check after acquiring write lock
	if time.Since(t.tokenCacheTime) < 5*time.Minute && t.tokenCache != "" {
		return t.tokenCache
	}

	// Read token from filesystem
	tokenBytes, err := os.ReadFile(t.tokenPath)
	if err != nil {
		// Token file doesn't exist or read error
		return ""
	}

	// Update cache
	t.tokenCache = string(tokenBytes)
	t.tokenCacheTime = time.Now()
	return t.tokenCache
}
```

**Key Design Decisions**:
1. **Three modes**: ServiceAccount (production/E2E), StaticToken (E2E tests), MockUser (integration tests)
2. **Token caching**: 5-minute cache to avoid filesystem reads on every request
3. **Idiomatic Go**: Implements `http.RoundTripper` interface (standard pattern)
4. **Transparent**: Services don't need to know about authentication logic

---

### **Component 2: Update Audit Adapter (Zero Service Changes)**

**File**: `pkg/audit/openapi_client_adapter.go`

```go
package audit

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// NewOpenAPIClientAdapter creates an OpenAPIClientAdapter with automatic ServiceAccount authentication.
//
// ========================================
// DD-AUTH-005: Transport Layer Authentication
// ========================================
//
// This function injects ServiceAccount token authentication into the DataStorage OpenAPI client
// WITHOUT modifying the generated client code. All 7 Go services use this adapter:
//
// Services:
// - Gateway (pkg/gateway/server.go)
// - AIAnalysis (pkg/aianalysis/server.go)
// - WorkflowExecution (pkg/workflowexecution/controller.go)
// - RemediationOrchestrator (pkg/remediationorchestrator/controller.go)
// - Notification (pkg/notification/server.go)
// - AuthWebhook (cmd/authwebhook/main.go)
// - SignalProcessing (cmd/signalprocessing/main.go)
//
// Authentication Flow:
// 1. Service creates audit adapter via this function
// 2. Adapter injects auth.NewServiceAccountTransport() into HTTP client
// 3. Transport reads /var/run/secrets/kubernetes.io/serviceaccount/token
// 4. Transport injects "Authorization: Bearer <token>" header
// 5. oauth-proxy sidecar validates token and injects X-Auth-Request-User
// 6. DataStorage handler extracts user identity
//
// Benefits:
// - ZERO service code changes (all services use this adapter)
// - OpenAPI generated client remains pristine (DD-HAPI-003)
// - Automatic token refresh (5-minute cache in transport)
// - Environment-aware (production uses ServiceAccount, tests use mock)
//
// See: docs/architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md
// ========================================
func NewOpenAPIClientAdapter(baseURL string, timeout time.Duration) (*OpenAPIClientAdapter, error) {
	// ========================================
	// DD-AUTH-005: Inject ServiceAccount authentication transport
	// This change affects ALL 7 Go services automatically
	// ========================================
	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: auth.NewServiceAccountTransport(), // ← Inject auth transport
	}

	// Create DataStorage OpenAPI client with custom transport
	dsClient, err := client.NewClientWithResponses(
		baseURL,
		client.WithHTTPClient(httpClient), // ← Pass authenticated HTTP client
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	return &OpenAPIClientAdapter{
		client:  dsClient,
		baseURL: baseURL,
	}, nil
}
```

**Impact**: ALL 7 Go services get ServiceAccount authentication with this ONE change!

**Services Affected (Zero Code Changes Required)**:
1. ✅ Gateway - uses `audit.NewOpenAPIClientAdapter()`
2. ✅ AIAnalysis - uses `audit.NewOpenAPIClientAdapter()`
3. ✅ WorkflowExecution - uses `audit.NewOpenAPIClientAdapter()`
4. ✅ RemediationOrchestrator - uses `audit.NewOpenAPIClientAdapter()`
5. ✅ Notification - uses `audit.NewOpenAPIClientAdapter()`
6. ✅ AuthWebhook - uses `audit.NewOpenAPIClientAdapter()` (see `cmd/authwebhook/main.go:81`)
7. ✅ SignalProcessing - uses `audit.NewOpenAPIClientAdapter()` (see `cmd/signalprocessing/main.go:153`)

---

### **Component 3: Python Auth Session**

**File**: `holmesgpt-api/src/clients/datastorage_auth_session.py`

```python
import os
from typing import Optional
from requests import Session
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

class ServiceAccountAuthSession(Session):
    """
    Requests Session that automatically injects Kubernetes ServiceAccount tokens.

    ========================================
    DD-AUTH-005: Transport Layer Authentication
    ========================================

    This session injects authentication headers for DataStorage API calls without
    modifying the OpenAPI-generated client code.

    Authentication Flow:
    1. holmesgpt-api creates DataStorage client with this session
    2. Session reads /var/run/secrets/kubernetes.io/serviceaccount/token
    3. Session injects "Authorization: Bearer <token>" header
    4. oauth-proxy sidecar validates token and injects X-Auth-Request-User
    5. DataStorage handler extracts user identity

    Usage (E2E/Production):
        session = ServiceAccountAuthSession()
        ds_client = DataStorageClient(base_url="http://datastorage:8080", session=session)

    Usage (Integration Tests):
        session = ServiceAccountAuthSession(mock_user="test-user@example.com")
        ds_client = DataStorageClient(base_url="http://localhost:8080", session=session)

    Benefits:
    - OpenAPI generated client remains pristine (DD-HAPI-003, DD-HAPI-005)
    - Automatic token refresh on every request
    - Environment-aware (production uses ServiceAccount, tests use mock)

    See: docs/architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md
    ========================================
    """

    def __init__(self, mock_user: Optional[str] = None, token_path: str = "/var/run/secrets/kubernetes.io/serviceaccount/token"):
        super().__init__()
        self.mock_user = mock_user
        self.token_path = token_path

        # Configure retries
        retry = Retry(
            total=3,
            backoff_factor=0.3,
            status_forcelist=[500, 502, 503, 504],
        )
        adapter = HTTPAdapter(max_retries=retry)
        self.mount("http://", adapter)
        self.mount("https://", adapter)

    def request(self, method, url, **kwargs):
        """Inject authentication headers before sending request."""

        if self.mock_user:
            # Integration test mode: Mock X-Auth-Request-User header
            kwargs.setdefault("headers", {})
            kwargs["headers"]["X-Auth-Request-User"] = self.mock_user
        else:
            # E2E/Production mode: Read ServiceAccount token from filesystem
            try:
                with open(self.token_path, "r") as f:
                    token = f.read().strip()
                    kwargs.setdefault("headers", {})
                    kwargs["headers"]["Authorization"] = f"Bearer {token}"
            except FileNotFoundError:
                # Token file doesn't exist (local dev without K8s)
                # Request proceeds without token (will fail if oauth-proxy is active)
                pass

        return super().request(method, url, **kwargs)
```

**Usage in holmesgpt-api**:

```python
# holmesgpt-api/src/services/audit_service.py

from src.clients.datastorage_auth_session import ServiceAccountAuthSession
from src.clients.datastorage import DataStorageClient

def create_datastorage_client(base_url: str, mock_user: Optional[str] = None) -> DataStorageClient:
    """
    Create DataStorage client with ServiceAccount authentication.

    DD-AUTH-005: Transport layer authentication for Python services

    Args:
        base_url: DataStorage service URL
        mock_user: If provided, mock X-Auth-Request-User header (integration tests)

    Returns:
        Authenticated DataStorage client
    """
    session = ServiceAccountAuthSession(mock_user=mock_user)
    return DataStorageClient(base_url=base_url, session=session)

# Usage in E2E/Production
ds_client = create_datastorage_client("http://datastorage:8080")

# Usage in Integration Tests
ds_client = create_datastorage_client("http://localhost:8080", mock_user="test-user@example.com")
```

---

## 🧪 **Testing Strategy**

### **Integration Tests (70%+ Coverage)** ✅ MOCK USER TRANSPORT

**Go Integration Tests**:
```go
// test/integration/datastorage/legal_hold_integration_test.go

var _ = Describe("DataStorage Integration Tests", func() {
	var (
		httpClient *http.Client
		dsClient   *datastorage.ClientWithResponses
	)

	BeforeEach(func() {
		// ========================================
		// DD-AUTH-005: Mock user transport for integration tests
		// ========================================
		transport := auth.NewMockUserTransport("test-operator@kubernaut.ai")
		httpClient = &http.Client{Transport: transport}

		dsClient, _ = datastorage.NewClientWithResponses(
			"http://localhost:8080",
			datastorage.WithHTTPClient(httpClient),
		)
	})

	It("should place legal hold with mocked user", func() {
		req := datastorage.PlaceLegalHoldJSONRequestBody{
			CorrelationId: "test-correlation-id",
			Reason:        "Integration test",
		}

		// Transport automatically injects: X-Auth-Request-User: test-operator@kubernaut.ai
		resp, err := dsClient.PlaceLegalHoldWithResponse(context.Background(), req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(200))
		Expect(resp.JSON200.PlacedBy).To(Equal("test-operator@kubernaut.ai"))
	})
})
```

**Python Integration Tests**:
```python
# holmesgpt-api/tests/integration/test_datastorage.py

def test_place_legal_hold():
    """Integration test with mocked user header."""
    # DD-AUTH-005: Mock user session for integration tests
    ds_client = create_datastorage_client(
        base_url="http://localhost:8080",
        mock_user="test-operator@kubernaut.ai"
    )

    # Session automatically injects: X-Auth-Request-User: test-operator@kubernaut.ai
    response = ds_client.place_legal_hold(
        correlation_id="test-correlation-id",
        reason="Integration test"
    )

    assert response.status_code == 200
    assert response.json()["placed_by"] == "test-operator@kubernaut.ai"
```

---

### **E2E Tests** ✅ STATIC TOKEN TRANSPORT

**Go E2E Tests**:
```go
// test/e2e/datastorage/legal_hold_e2e_test.go

var _ = Describe("DataStorage E2E Tests", func() {
	It("should authenticate with real oauth-proxy sidecar", func() {
		// ========================================
		// DD-AUTH-005: Static token transport for E2E tests
		// ========================================

		// Get ServiceAccount token via TokenRequest API
		token, err := getServiceAccountToken("datastorage-e2e-client", "kubernaut", 3600)
		Expect(err).ToNot(HaveOccurred())

		// Create transport with static token
		transport := auth.NewStaticTokenTransport(token)
		httpClient := &http.Client{Transport: transport}

		// Create DataStorage client (connects to oauth-proxy sidecar port 8443)
		dsClient, _ := datastorage.NewClientWithResponses(
			"https://datastorage.kubernaut.svc.cluster.local:8443",
			datastorage.WithHTTPClient(httpClient),
		)

		// Use client normally
		req := datastorage.PlaceLegalHoldJSONRequestBody{
			CorrelationId: "e2e-correlation-id",
			Reason:        "E2E test",
		}

		// Transport injects: Authorization: Bearer <token>
		// oauth-proxy validates token, performs SAR, injects X-Auth-Request-User
		resp, err := dsClient.PlaceLegalHoldWithResponse(context.Background(), req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(200))
	})
})
```

**Python E2E Tests** (if applicable):
```python
# holmesgpt-api/tests/e2e/test_datastorage.py

def test_place_legal_hold_e2e():
    """E2E test with real oauth-proxy sidecar."""
    # DD-AUTH-005: ServiceAccount token from filesystem (E2E environment)
    ds_client = create_datastorage_client(
        base_url="https://datastorage.kubernaut.svc.cluster.local:8443"
    )

    # Session reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
    # oauth-proxy validates token, performs SAR, injects X-Auth-Request-User
    response = ds_client.place_legal_hold(
        correlation_id="e2e-correlation-id",
        reason="E2E test"
    )

    assert response.status_code == 200
```

---

### **Production Deployment** ✅ SERVICEACCOUNT TRANSPORT

**Go Services** (Gateway, AIAnalysis, WorkflowExecution, etc.):
```go
// All Go services use audit.NewOpenAPIClientAdapter()
// No changes needed - adapter already uses auth.NewServiceAccountTransport()

dsClient, err := audit.NewOpenAPIClientAdapter(dataStorageURL, 30*time.Second)
// Transport automatically reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
```

**Python Service** (holmesgpt-api):
```python
# holmesgpt-api/src/main.py

ds_client = create_datastorage_client("http://datastorage:8080")
# Session automatically reads token from /var/run/secrets/kubernetes.io/serviceaccount/token
```

---

## ✅ **Consequences**

### **Positive Impacts**

1. **Zero Service Code Changes** ✅
   - 7 Go services: Update `audit.NewOpenAPIClientAdapter()` ONCE
   - 1 Python service: Update client instantiation ONCE
   - All services automatically get authentication

2. **OpenAPI Client Compliance** ✅
   - Generated clients remain pristine (DD-HAPI-003)
   - Auto-regeneration workflow preserved (DD-HAPI-005)
   - No manual patching after spec updates

3. **Environment-Aware Authentication** ✅
   - Integration tests: Mock user headers (no oauth-proxy)
   - E2E tests: Real tokens with oauth-proxy validation
   - Production: Automatic ServiceAccount token from filesystem

4. **Consistent Pattern Across Services** ✅
   - All 8 services use same authentication pattern
   - Single implementation to maintain
   - Easy to add new services (just use the adapter/session)

5. **Testing Simplicity** ✅
   - Integration tests: `auth.NewMockUserTransport("test-user")`
   - E2E tests: `auth.NewStaticTokenTransport(token)`
   - No complex mocking infrastructure

6. **Automatic Token Refresh** ✅
   - Go: 5-minute cache in transport layer
   - Python: Read on every request (stateless)
   - No manual token management

---

### **Negative Impacts** (Mitigated)

1. **New Abstraction Layer** ⚠️
   - **Impact**: Developers need to understand transport layer
   - **Mitigation**: Comprehensive documentation in this DD
   - **Severity**: LOW (standard pattern)

2. **Python Learning Curve** ⚠️
   - **Impact**: `requests.Session` less familiar than Go `http.RoundTripper`
   - **Mitigation**: Simple API, well-documented pattern
   - **Severity**: LOW

3. **Token Caching Complexity** ⚠️
   - **Impact**: Go transport caches tokens (5-minute TTL)
   - **Mitigation**: Thread-safe implementation with RWMutex
   - **Severity**: LOW (performance optimization)

---

## 📋 **Implementation Checklist**

### **Phase 1: Go Foundation** (2 hours)
- [ ] **Task 1.1**: Create `pkg/shared/auth/transport.go`
  - [ ] Implement `AuthTransport` struct with 3 modes
  - [ ] Implement `NewServiceAccountTransport()`
  - [ ] Implement `NewStaticTokenTransport(token)`
  - [ ] Implement `NewMockUserTransport(userID)`
  - [ ] Implement `RoundTrip()` with header injection
  - [ ] Implement `getServiceAccountToken()` with caching

- [ ] **Task 1.2**: Update `pkg/audit/openapi_client_adapter.go`
  - [ ] Add `auth.NewServiceAccountTransport()` to HTTP client
  - [ ] **Result**: ALL 7 Go services get authentication automatically

### **Phase 2: Python Foundation** (1.5 hours)
- [ ] **Task 2.1**: Create `holmesgpt-api/src/clients/datastorage_auth_session.py`
  - [ ] Implement `ServiceAccountAuthSession` class
  - [ ] Implement token injection from filesystem
  - [ ] Implement mock user header for integration tests

- [ ] **Task 2.2**: Update holmesgpt-api DataStorage client instantiation
  - [ ] Find where DataStorage client is created
  - [ ] Use `ServiceAccountAuthSession`

### **Phase 3: Integration Tests** (2 hours)
- [ ] **Task 3.1**: Update Go integration tests
  - [ ] `test/integration/datastorage/legal_hold_integration_test.go`
  - [ ] Use `auth.NewMockUserTransport("test-user")`

- [ ] **Task 3.2**: Update Python integration tests (if any)
  - [ ] Use `ServiceAccountAuthSession(mock_user="test-user")`

### **Phase 4: E2E Tests** (1.5 hours)
- [ ] **Task 4.1**: Create `test/e2e/datastorage/helpers.go`
  - [ ] Implement `getServiceAccountToken()` using TokenRequest API

- [ ] **Task 4.2**: Update Go E2E tests
  - [ ] Use `auth.NewStaticTokenTransport(token)`

- [ ] **Task 4.3**: Update Python E2E tests (if any)
  - [ ] Use `ServiceAccountAuthSession()` (reads token from filesystem)

### **Phase 5: E2E Infrastructure** (2 hours)
- [ ] **Task 5.1**: Update DataStorage deployment
  - [ ] Add oauth-proxy sidecar (already done in DD-AUTH-004)

- [ ] **Task 5.2**: Create RBAC for all 8 services
  - [ ] Create ClusterRole for DataStorage access
  - [ ] Create RoleBindings for all 8 ServiceAccounts

- [ ] **Task 5.3**: Create test ServiceAccount
  - [ ] `datastorage-e2e-client` with required permissions

### **Phase 6: Handler Updates** (1 hour)
- [ ] **Task 6.1**: Update `pkg/datastorage/server/legal_hold_handler.go`
  - [ ] Replace `X-User-ID` → `X-Auth-Request-User`
  - [ ] Keep defense-in-depth 401 validation

### **Phase 7: OpenAPI Spec** (30 min)
- [ ] **Task 7.1**: Update `api/openapi/data-storage-v1.yaml`
  - [ ] Document oauth-proxy authentication flow
  - [ ] Regenerate Go and Python clients

### **Phase 8: Verification** (1 hour)
- [ ] **Task 8.1**: Run integration tests
  - [ ] Go: `make test-integration-datastorage`
  - [ ] Python: `pytest tests/integration/`

- [ ] **Task 8.2**: Run E2E tests
  - [ ] Go: `make test-e2e-datastorage`
  - [ ] Python: `pytest tests/e2e/`

---

## 📊 **Service Implementation Matrix**

| Service | Language | Integration | Uses Adapter/Session | Code Changes |
|---------|----------|-------------|----------------------|--------------|
| **Gateway** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **AIAnalysis** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **WorkflowExecution** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **RemediationOrchestrator** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **Notification** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **AuthWebhook** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **SignalProcessing** | Go | audit.NewOpenAPIClientAdapter() | ✅ Yes | ✅ Zero (uses adapter) |
| **holmesgpt-api** | Python | Direct OpenAPI client | ✅ Yes | ⚠️ One function (client instantiation) |

**Total Service Code Changes**: **1 function** (holmesgpt-api client instantiation)

---

## 🔗 **Related Decisions**

- **Builds On**: [DD-AUTH-004](DD-AUTH-004-openshift-oauth-proxy-legal-hold.md) (OpenShift OAuth-Proxy)
- **Enforces**: [DD-HAPI-003](DD-HAPI-003-mandatory-openapi-client-usage.md) (Mandatory OpenAPI Client Usage)
- **Enforces**: [DD-HAPI-005](DD-HAPI-005-python-openapi-client-regeneration.md) (Python OpenAPI Client Auto-Regeneration)
- **Supports**: SOC2 Gap #8 (Legal Hold), SOC2 CC6.1 (Access Controls)
- **Authoritative Pattern**: ALL services MUST follow this pattern for DataStorage interactions

---

## 📈 **Success Metrics**

| Metric | Target | Status |
|--------|--------|--------|
| Go services using auth transport | 7/7 (100%) | ⬜ Not Started |
| Python services using auth session | 1/1 (100%) | ⬜ Not Started |
| Integration tests with mock user | 100% passing | ⬜ Not Started |
| E2E tests with real tokens | 100% passing | ⬜ Not Started |
| Generated clients pristine | 100% (no modifications) | ⬜ Not Started |
| Service code changes | ≤2 files (adapter + session) | ⬜ Not Started |

---

## 📚 **Quick Reference**

### **Go Services (Production/E2E)**
```go
// Just use audit.NewOpenAPIClientAdapter() - it handles everything
dsClient, _ := audit.NewOpenAPIClientAdapter("http://datastorage:8080", 30*time.Second)
```

### **Go Integration Tests**
```go
transport := auth.NewMockUserTransport("test-user@example.com")
httpClient := &http.Client{Transport: transport}
dsClient, _ := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
```

### **Go E2E Tests**
```go
token, _ := getServiceAccountToken("test-sa", "kubernaut", 3600)
transport := auth.NewStaticTokenTransport(token)
httpClient := &http.Client{Transport: transport}
dsClient, _ := datastorage.NewClientWithResponses(url, datastorage.WithHTTPClient(httpClient))
```

### **Python Service (Production/E2E)**
```python
ds_client = create_datastorage_client("http://datastorage:8080")
```

### **Python Integration Tests**
```python
ds_client = create_datastorage_client("http://localhost:8080", mock_user="test-user@example.com")
```

---

**Document Status**: ✅ APPROVED
**Implementation Status**: ⬜ NOT STARTED
**Target V1.0**: Yes (All 8 services: 7 Go + 1 Python)
**Confidence**: 95%
**Authority**: AUTHORITATIVE - All services MUST follow this pattern


