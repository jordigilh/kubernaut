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

package contextapi

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	
	// Import PostgreSQL driver (required for database connections)
	_ "github.com/jackc/pgx/v5/stdlib"
)

// E2E Test Suite for Context API Aggregation (Podman Infrastructure)
// Tests the complete flow: PostgreSQL → Data Storage Service → Context API
//
// Infrastructure (4 components):
// 1. PostgreSQL (database)
// 2. Redis (cache)
// 3. Data Storage Service (REST API)
// 4. Context API (REST API)
//
// Related: Day 12 - E2E Tests + Documentation

var (
	// Infrastructure components
	dataStorageInfra *infrastructure.DataStorageInfrastructure
	contextAPIInfra  *infrastructure.ContextAPIInfrastructure

	// Service ports (avoid conflicts with integration tests)
	postgresPort    = "5434"
	redisPort       = "6381"
	dataStoragePort = "8087"
	contextAPIPort  = "8088"

	// Service URLs
	dataStorageBaseURL string
	contextAPIBaseURL  string

	// Test context
	ctx    context.Context
	cancel context.CancelFunc
	logger *zap.Logger
)

func TestContextAPIAggregationE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API Aggregation E2E Suite (Podman)")
}

var _ = BeforeSuite(func() {
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("🚀 Starting Context API Aggregation E2E Test Infrastructure (Podman)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred(), "Failed to create logger")

	// Start Data Storage Infrastructure (PostgreSQL + Redis + Data Storage Service)
	GinkgoWriter.Println("📦 Starting Data Storage Infrastructure (3 services)...")
	cfg := &infrastructure.DataStorageConfig{
		PostgresPort: postgresPort,
		RedisPort:    redisPort,
		ServicePort:  dataStoragePort,
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password_e2e",
	}

	dataStorageInfra, err = infrastructure.StartDataStorageInfrastructure(cfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Data Storage infrastructure should start successfully")

	dataStorageBaseURL = dataStorageInfra.ServiceURL
	GinkgoWriter.Printf("✅ Data Storage Infrastructure ready: %s\n", dataStorageBaseURL)

	// Start Context API Service
	GinkgoWriter.Println("📦 Starting Context API Service...")
	contextAPICfg := &infrastructure.ContextAPIConfig{
		RedisPort:       redisPort,
		DataStoragePort: dataStoragePort,
		ServicePort:     contextAPIPort,
	}

	contextAPIInfra, err = infrastructure.StartContextAPIInfrastructure(contextAPICfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Context API should start successfully")

	contextAPIBaseURL = contextAPIInfra.ServiceURL
	GinkgoWriter.Printf("✅ Context API ready: %s\n", contextAPIBaseURL)

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ E2E Infrastructure Ready - All 4 services running")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Printf("   PostgreSQL:     localhost:%s\n", postgresPort)
	GinkgoWriter.Printf("   Redis:          localhost:%s\n", redisPort)
	GinkgoWriter.Printf("   Data Storage:   %s\n", dataStorageBaseURL)
	GinkgoWriter.Printf("   Context API:    %s\n", contextAPIBaseURL)
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})

var _ = AfterSuite(func() {
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("🧹 Cleaning up E2E test infrastructure...")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Stop Context API Service
	if contextAPIInfra != nil {
		GinkgoWriter.Println("🛑 Stopping Context API Service...")
		contextAPIInfra.Stop(GinkgoWriter)
		GinkgoWriter.Println("✅ Context API Service stopped")
	}

	// Stop Data Storage Infrastructure (PostgreSQL + Redis + Data Storage Service)
	if dataStorageInfra != nil {
		GinkgoWriter.Println("🛑 Stopping Data Storage Infrastructure...")
		dataStorageInfra.Stop(GinkgoWriter)
		GinkgoWriter.Println("✅ Data Storage Infrastructure stopped")
	}

	// Cancel context
	if cancel != nil {
		cancel()
	}

	// Sync logger
	if logger != nil {
		_ = logger.Sync()
	}

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ E2E cleanup complete")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
})
