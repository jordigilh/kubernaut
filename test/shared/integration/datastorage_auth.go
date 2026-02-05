/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package integration provides shared helpers for integration tests.
//
// DD-AUTH-014: This package provides centralized DataStorage authentication helpers
// that automatically use ServiceAccount tokens from envtest, eliminating the need
// for each test to manually configure authentication.
package integration

import (
	"net/http"
	"time"

	"github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// AuthenticatedDataStorageClients holds both the audit client and OpenAPI client
// with automatic ServiceAccount authentication (DD-AUTH-014).
//
// Usage in integration test suite:
//
//	var dsClients *integration.AuthenticatedDataStorageClients
//
//	var _ = SynchronizedBeforeSuite(func() []byte {
//	    // Phase 1: Create ServiceAccount + start DataStorage
//	    authConfig, _ := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(...)
//	    return []byte(authConfig.Token)
//	}, func(data []byte) {
//	    // Phase 2: Create authenticated clients
//	    token := string(data)
//	    dsClients = integration.NewAuthenticatedDataStorageClients(
//	        "http://localhost:18140",
//	        token,
//	        5*time.Second,
//	    )
//	})
type AuthenticatedDataStorageClients struct {
	// AuditClient is used by controllers for audit event emission
	// (BufferedStore, batch writes, automatic retries)
	AuditClient audit.DataStorageClient

	// OpenAPIClient is used by tests for direct DataStorage queries
	// (workflow searches, audit queries, etc.)
	OpenAPIClient *ogenclient.Client

	// HTTPClient is the underlying authenticated HTTP client
	// Can be used for custom HTTP requests if needed
	HTTPClient *http.Client
}

// NewAuthenticatedDataStorageClients creates DataStorage clients with automatic
// ServiceAccount authentication via Bearer token (DD-AUTH-014).
//
// This function centralizes authentication setup so that:
//   - All DataStorage requests use the same ServiceAccount token
//   - Tests don't need to manually configure authentication
//   - Audit stores automatically use authenticated requests
//   - Easy to reuse across all service integration tests
//
// Parameters:
//   - baseURL: DataStorage API URL (e.g., "http://localhost:18140")
//   - token: ServiceAccount Bearer token from envtest (from Phase 1)
//   - timeout: HTTP client timeout (e.g., 5*time.Second)
//
// Returns:
//   - AuthenticatedDataStorageClients with both audit and OpenAPI clients
//
// Example:
//
//	dsClients := integration.NewAuthenticatedDataStorageClients(
//	    dataStorageBaseURL,
//	    token,  // from Phase 1
//	    5*time.Second,
//	)
//
//	// Use in controller setup
//	auditStore, _ := audit.NewBufferedStore(
//	    dsClients.AuditClient,  // ← Automatically authenticated
//	    auditConfig,
//	    "remediation-orchestrator",
//	    auditLogger,
//	)
//
//	// Use in tests for queries
//	workflows, _ := dsClients.OpenAPIClient.WorkflowSearch(ctx, ...)
func NewAuthenticatedDataStorageClients(baseURL, token string, timeout time.Duration) *AuthenticatedDataStorageClients {
	// Create ServiceAccount transport (injects Bearer token in Authorization header)
	saTransport := testauth.NewServiceAccountTransport(token)

	// Create authenticated HTTP client
	httpClient := &http.Client{
		Transport: saTransport,
		Timeout:   timeout,
	}

	// Create audit client adapter (for controllers)
	auditClient, err := audit.NewOpenAPIClientAdapterWithTransport(
		baseURL,
		timeout,
		saTransport,
	)
	if err != nil {
		panic(err) // Should never fail - indicates infrastructure issue
	}

	// Create OpenAPI client (for test queries)
	openAPIClient, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		panic(err) // Should never fail - indicates infrastructure issue
	}

	return &AuthenticatedDataStorageClients{
		AuditClient:   auditClient,
		OpenAPIClient: openAPIClient,
		HTTPClient:    httpClient,
	}
}

// Note: NewAuthenticatedAuditStore convenience wrapper removed to avoid import cycles.
// Use NewAuthenticatedDataStorageClients() + audit.NewBufferedStore() directly instead.
