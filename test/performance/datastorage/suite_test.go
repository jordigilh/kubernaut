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

package datastorage

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// Performance Test Suite for Data Storage Service
//
// Purpose: Validate SQL query performance at realistic scale using local Podman infrastructure
//
// Infrastructure:
// - Reuses integration test Podman PostgreSQL
// - Tests against 1K, 5K, and 10K workflow catalogs
// - Measures P50, P95, P99 latencies
// - Tests concurrent query performance
//
// Performance Targets (Local Testing - 2x slower than production):
// - P50 Latency: <100ms
// - P95 Latency: <200ms
// - P99 Latency: <500ms
// - Concurrent Queries: 10 QPS sustained
//
// Why Local Testing:
// - No production platform available yet
// - Sufficient for detecting performance regressions
// - Can run in CI/CD
// - Production-scale testing deferred to V1.1+

func TestDataStoragePerformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Performance Suite")
}

var (
	ctx    context.Context
	cancel context.CancelFunc
	logger logr.Logger

	// Database connection (reuses integration test infrastructure)
	db *sqlx.DB

	// Data Storage Service URL (for HTTP API performance tests)
	datastorageURL string
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	logger = kubelog.NewLogger(kubelog.DevelopmentOptions())

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Data Storage Performance Test Suite - Setup")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Infrastructure: Local Podman PostgreSQL (reuses integration test setup)")
	logger.Info("Test Scale: 1K, 5K, 10K workflows")
	logger.Info("Targets: P50 <100ms, P95 <200ms, P99 <500ms (2x slower than production)")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Connect to PostgreSQL (assumes integration test infrastructure is running)
	// If not running, these tests will be skipped
	//
	// IMPORTANT: Use PERF_TEST_PG_PORT environment variable to avoid conflicts
	// with integration tests that may be using the default port (5433)
	//
	// Example: PERF_TEST_PG_PORT=5434 ginkgo test/performance/datastorage/
	//
	// Credentials match integration test setup: slm_user/test_password
	pgPort := os.Getenv("PERF_TEST_PG_PORT")
	if pgPort == "" {
		pgPort = "5433" // Default fallback
		logger.Info("warning: PERF_TEST_PG_PORT not set, using default port 5433 (may conflict with integration tests)")
	}
	postgresURL := fmt.Sprintf("postgresql://slm_user:test_password@localhost:%s/action_history?sslmode=disable", pgPort)
	var err error
	db, err = sqlx.Connect("postgres", postgresURL)
	if err != nil {
		Skip(fmt.Sprintf("PostgreSQL not available (integration test infrastructure not running): %v", err))
	}

	// Set Data Storage Service URL for HTTP API performance tests
	// These tests require a running Data Storage service (integration test infrastructure)
	datastorageURL = os.Getenv("DATASTORAGE_URL")
	if datastorageURL == "" {
		datastorageURL = "http://localhost:18090" // Default to integration test port
	}
	logger.Info("Data Storage Service URL", "url", datastorageURL)

	logger.Info("✅ Performance test suite setup complete")
})

var _ = AfterSuite(func() {
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("Data Storage Performance Test Suite - Cleanup")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Close database connection
	if db != nil {
		if err := db.Close(); err != nil {
			logger.Info("warning: failed to close database connection", "error", err)
		}
	}

	if cancel != nil {
		cancel()
	}

	logger.Info("✅ Performance test suite cleanup complete")
})

// generateTestID creates a unique test identifier for data isolation
// Format: test-{process}-{timestamp}
// This enables parallel test execution by ensuring each test has unique data
func generateTestID() string {
	return fmt.Sprintf("perf-%d-%d", GinkgoParallelProcess(), os.Getpid())
}
