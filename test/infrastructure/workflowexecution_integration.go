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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// WorkflowExecution Integration Test Infrastructure Constants
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Per DD-TEST-001 v1.7 (December 2025): Port Allocation Strategy
// Per DD-TEST-002: Integration Test Container Orchestration Pattern
//
// WorkflowExecution integration tests use sequential `podman run` commands
// via `test/integration/workflowexecution/setup-infrastructure.sh`.
//
// Migration History:
// - Before Dec 21, 2025: Used podman-compose (race conditions, Exit 137 failures)
// - Dec 21, 2025: Migrated to DD-TEST-002 sequential startup (47% fewer failures)
// - Dec 22, 2025: Aligned ports to DD-TEST-001 sequential pattern
//
// Related:
// - workflowexecution.go: E2E infrastructure (Kind cluster + Tekton)
// - workflowexecution_parallel.go: E2E parallel test helpers
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// WorkflowExecution Integration Test Ports (per DD-TEST-001 v1.7 - December 2025)
// Sequential allocation after AIAnalysis (15438/16384/18095/19095)
const (
	// PostgreSQL port for WorkflowExecution integration tests
	// Changed from 15443 (ad-hoc "+10") to 15441 (DD-TEST-001 sequential) on Dec 22, 2025
	WEIntegrationPostgresPort = 15441

	// Redis port for WorkflowExecution integration tests
	// Changed from 16389 (ad-hoc "+10") to 16387 (DD-TEST-001 sequential) on Dec 22, 2025
	// Changed from 16387 (conflicted with HAPI) to 16388 (unique) on Dec 25, 2025 per DD-TEST-001 v1.9
	WEIntegrationRedisPort = 16388

	// DataStorage HTTP API port for WorkflowExecution integration tests
	// Changed from 18100 (conflicted with EffectivenessMonitor) to 18097 (DD-TEST-001 sequential) on Dec 22, 2025
	WEIntegrationDataStoragePort = 18097

	// DataStorage Metrics port for WorkflowExecution integration tests
	// Changed from 19100 (ad-hoc "+10") to 19097 (DD-TEST-001 metrics pattern) on Dec 22, 2025
	WEIntegrationMetricsPort = 19097
)

// WorkflowExecution Integration Test Container Names
// These match the container names in setup-infrastructure.sh for sequential startup
const (
	// PostgreSQL container name (matches setup-infrastructure.sh line 18)
	WEIntegrationPostgresContainer = "workflowexecution_postgres_1"

	// Redis container name (matches setup-infrastructure.sh line 19)
	WEIntegrationRedisContainer = "workflowexecution_redis_1"

	// DataStorage container name (matches setup-infrastructure.sh line 20)
	WEIntegrationDataStorageContainer = "workflowexecution_datastorage_1"

	// Migrations container name (matches setup-infrastructure.sh line 21)
	WEIntegrationMigrationsContainer = "workflowexecution_migrations"

	// Network name for container communication (matches setup-infrastructure.sh line 22)
	WEIntegrationNetwork = "workflowexecution_test-network"
)

// WorkflowExecution Integration Test Database Configuration
// These match the database settings in setup-infrastructure.sh
const (
	// Database name for audit events (matches setup-infrastructure.sh line 31)
	WEIntegrationDBName = "action_history"

	// Database user (matches setup-infrastructure.sh line 32)
	WEIntegrationDBUser = "slm_user"

	// Database password (matches setup-infrastructure.sh line 33)
	WEIntegrationDBPassword = "test_password"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// USAGE NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Starting Infrastructure:
//   cd test/integration/workflowexecution
//   ./setup-infrastructure.sh
//
// Stopping Infrastructure:
//   cd test/integration/workflowexecution
//   podman stop workflowexecution_postgres_1 workflowexecution_redis_1 workflowexecution_datastorage_1
//   podman rm workflowexecution_postgres_1 workflowexecution_redis_1 workflowexecution_datastorage_1
//
// Health Check:
//   curl http://localhost:18097/health  # Should return 200 OK
//
// Running Tests:
//   go test ./test/integration/workflowexecution/... -v -timeout=10m
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━




