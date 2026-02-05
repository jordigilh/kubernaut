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

package infrastructure

import (
	"fmt"
	"io"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HolmesGPT API Integration Test Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic Podman Setup using Go
// Replaces: Python pytest fixtures calling docker-compose via subprocess
//
// Port Allocation (per DD-TEST-001 v1.8):
//   PostgreSQL:   15439  (HAPI-specific, unique - Notification now uses 15440)
//   Redis:        16387  (HAPI-specific, unique - all services have separate Redis ports)
//   DataStorage:  18098  (HAPI allocation)
//   HAPI:         18120  (HAPI service port)
//
// Dependencies:
//   HolmesGPT-API Tests → HAPI Service (HTTP API)
//   HAPI Service → Data Storage (workflow catalog, audit)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
//
// Migration: December 27, 2025
//   From: Python pytest fixtures → docker-compose via subprocess.run()
//   To:   Go programmatic setup → shared utilities from shared_integration_utils.go
//   Benefits: Consistency with other services, no subprocess calls, reuses 720 lines of shared code
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Port allocation per DD-TEST-001 v1.8
const (
	HAPIIntegrationPostgresPort    = 15439 // HAPI-specific port (unique - Notification moved to 15440)
	HAPIIntegrationRedisPort       = 16387 // HAPI-specific port (unique - all services have separate Redis ports)
	HAPIIntegrationDataStoragePort = 18098 // HAPI allocation per DD-TEST-001 v1.8
	HAPIIntegrationServicePort     = 18120 // HAPI service port (per DD-TEST-001 v1.8)
)

// Container names (unique to HAPI integration tests)
// Pattern: DSBootstrap programmatic setup uses {service}_{component}_test suffix
const (
	HAPIIntegrationPostgresContainer    = "holmesgptapi_postgres_test"    // Matches DSBootstrap pattern
	HAPIIntegrationRedisContainer       = "holmesgptapi_redis_test"       // Matches DSBootstrap pattern
	HAPIIntegrationDataStorageContainer = "holmesgptapi_datastorage_test" // Matches DSBootstrap pattern
	HAPIIntegrationHAPIContainer        = "holmesgptapi_hapi_1"
	HAPIIntegrationMigrationsContainer  = "holmesgptapi_migrations"
	HAPIIntegrationNetwork              = "holmesgptapi_test-network"
)

// Database configuration
const (
	HAPIIntegrationDBName     = "action_history"
	HAPIIntegrationDBUser     = "slm_user"
	HAPIIntegrationDBPassword = "test_password"
)

// HAPIIntegrationInfra holds infrastructure handles for HAPI integration tests
// Used for cleanup in SynchronizedAfterSuite
type HAPIIntegrationInfra struct {
	DSInfra *DSBootstrapInfra // DataStorage infrastructure (includes PostgreSQL, Redis)
}

// StartHolmesGPTAPIIntegrationInfrastructure starts the full Podman stack for HAPI integration tests
// This includes: envtest, PostgreSQL, Redis, DataStorage API (with auth), and HolmesGPT-API service
//
// Pattern: DD-TEST-002 Sequential Startup Pattern (using shared utilities)
// - Programmatic `podman run` commands (NOT docker-compose)
// - Explicit health checks after each service
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
// - podman must be installed
// - Ports 15439, 16387, 18098, 18120 must be available (per DD-TEST-001 v2.0 - all unique)
//
// Returns:
// - *HAPIIntegrationInfra: Infrastructure handles (for cleanup)
// - error: Any errors during infrastructure startup
//
// Authority: DD-AUTH-014 (DataStorage requires auth middleware)
func StartHolmesGPTAPIIntegrationInfrastructure(writer io.Writer, envtestKubeconfig string) (*HAPIIntegrationInfra, error) {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Starting HolmesGPT API Integration Test Infrastructure\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", HAPIIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:          localhost:%d\n", HAPIIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", HAPIIntegrationDataStoragePort)
	_, _ = fmt.Fprintf(writer, "  HAPI:           http://localhost:%d\n", HAPIIntegrationServicePort)
	_, _ = fmt.Fprintf(writer, "  Pattern:        DD-INTEGRATION-001 v2.0 (Programmatic Go)\n")
	_, _ = fmt.Fprintf(writer, "  Migration:      From Python subprocess → Go shared utilities\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// ============================================================================
	// STEP 1: Start DataStorage infrastructure (PostgreSQL + Redis + DataStorage)
	// ============================================================================
	// FIX: HAPI-INT-CONFIG-001 - Use standardized StartDSBootstrap helper
	// ROOT CAUSE: Manual DataStorage start was missing KUBECONFIG env var
	// SOLUTION: Use StartDSBootstrap which handles auth config properly
	//
	// Note: We don't need ServiceAccount token for HAPI tests (business logic tests)
	// but DataStorage MUST have KUBECONFIG to initialize its auth middleware (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "📦 Starting DataStorage with auth middleware (DD-AUTH-014)...\n")
	_, _ = fmt.Fprintf(writer, "   Using StartDSBootstrap helper (standardized pattern)\n\n")

	cfg := DSBootstrapConfig{
		ServiceName:       "holmesgptapi",
		PostgresPort:      HAPIIntegrationPostgresPort,
		RedisPort:         HAPIIntegrationRedisPort,
		DataStoragePort:   HAPIIntegrationDataStoragePort,
		MetricsPort:       19098, // Metrics port for DataStorage
		ConfigDir:         "test/integration/holmesgptapi/config",
		EnvtestKubeconfig: envtestKubeconfig, // DD-AUTH-014: Required for auth middleware
	}

	dsInfra, err := StartDSBootstrap(cfg, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to start DataStorage infrastructure: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ DataStorage ready at %s (with auth middleware)\n\n", dsInfra.ServiceURL)

	// ============================================================================
	// INTEGRATION TEST PATTERN: HAPI business logic called directly (no container)
	// ============================================================================
	// Architecture Decision (Jan 4, 2026):
	// - Integration tests: Call HAPI business logic DIRECTLY (no HTTP, no container)
	//   * Go pattern: controller.Reconcile(ctx, req)
	//   * Python pattern: analyze_incident(request_data)
	// - E2E tests: Use HTTP API + OpenAPI client (future implementation)
	//
	// Why Direct Business Logic Calls for Integration Tests?
	// ✅ Consistent with Go service testing (no HTTP in integration tests)
	// ✅ Faster (~2 min, no HTTP overhead or container startup)
	// ✅ Focused on business logic behavior (not HTTP routing)
	// ✅ Easier debugging (direct function calls, no network layer)
	//
	// External dependencies for integration tests:
	// - PostgreSQL (for Data Storage persistence)
	// - Redis (for Data Storage caching)
	// - Data Storage (for audit event validation - external dependency)
	//
	// HTTP API testing deferred to E2E test suite (future implementation)
	// See: docs/handoff/HAPI_INTEGRATION_TEST_ARCHITECTURE_FIX_JAN_04_2026.md
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "ℹ️  HAPI Integration Test Pattern:\n")
	_, _ = fmt.Fprintf(writer, "   • HAPI business logic called DIRECTLY (no HTTP, no container)\n")
	_, _ = fmt.Fprintf(writer, "   • Python tests import src.extensions.incident.llm_integration directly\n")
	_, _ = fmt.Fprintf(writer, "   • External dependencies: PostgreSQL, Redis, Data Storage only\n")
	_, _ = fmt.Fprintf(writer, "   • Pattern: Matches Go service testing (controller.Reconcile() direct calls)\n")
	_, _ = fmt.Fprintf(writer, "   • See: holmesgpt-api/tests/integration/test_hapi_audit_flow_integration.py\n\n")

	// ============================================================================
	// Success Summary
	// ============================================================================
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "✅ HolmesGPT API Integration Infrastructure Ready\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d (ready)\n", HAPIIntegrationPostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:          localhost:%d (ready)\n", HAPIIntegrationRedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage:    %s (healthy with auth)\n", dsInfra.ServiceURL)
	_, _ = fmt.Fprintf(writer, "  HAPI:           Business logic called directly (no HTTP, no container)\n")
	_, _ = fmt.Fprintf(writer, "  Pattern:        StartDSBootstrap (standardized)\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	return &HAPIIntegrationInfra{DSInfra: dsInfra}, nil
}

// StopHolmesGPTAPIIntegrationInfrastructure stops all HAPI integration test infrastructure
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic cleanup
// - Stops containers gracefully
// - Removes network
// - Prunes infrastructure images (composite tags)
//
// Migration Note:
//
//	Replaces holmesgpt-api/tests/integration/conftest.py cleanup_infrastructure_after_tests()
//	which used subprocess.run() to call docker-compose down.
func StopHolmesGPTAPIIntegrationInfrastructure(infra *HAPIIntegrationInfra, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "\n🛑 Stopping HolmesGPT API Integration Infrastructure...\n")

	// FIX: HAPI-INT-CONFIG-001 - Use standardized StopDSBootstrap helper
	// Stops: DataStorage, Redis, PostgreSQL (in correct order)
	if infra != nil && infra.DSInfra != nil {
		if err := StopDSBootstrap(infra.DSInfra, writer); err != nil {
			return fmt.Errorf("failed to stop DataStorage infrastructure: %w", err)
		}
	}

	_, _ = fmt.Fprintf(writer, "✅ Infrastructure cleaned up (standardized StopDSBootstrap pattern)\n\n")
	return nil
}
