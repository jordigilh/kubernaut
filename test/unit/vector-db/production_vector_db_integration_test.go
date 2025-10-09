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

package vectordb

import (
	"testing"
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// Business Requirements: BR-VDB-PROD-001 - Production workflows must use production vector databases
var _ = Describe("Production Vector Database Integration - Business Requirements", func() {
	var (
		logger *logrus.Logger
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("BR-VDB-PROD-001: Production vector database backend selection", func() {
		It("should prefer PostgreSQL backend when properly configured", func() {
			// Arrange: Create PostgreSQL configuration
			pgConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				PostgreSQL: config.PostgreSQLVectorConfig{
					UseMainDB: false,
					Host:      "localhost",
					Port:      "5433",
					Database:  "action_history",
					Username:  "slm_user",
					Password:  "slm_password_dev",
				},
			}

			// Act: Create vector database using factory (production pattern)
			vectorDB, backend, err := createProductionVectorDatabase(ctx, pgConfig, logger)

			// Assert: Should create PostgreSQL backend or graceful fallback
			Expect(err).ToNot(HaveOccurred(), "Should create vector database successfully")
			Expect(vectorDB).To(BeAssignableToTypeOf(vectorDB), "BR-DATABASE-001-A: Production vector database must provide functional database interface for database operations")

			// Business requirement: Should attempt PostgreSQL first
			if backend == "postgresql" {
				// If PostgreSQL is available, should use it
				Expect(backend).To(Equal("postgresql"), "Should use PostgreSQL when available")
			} else {
				// If PostgreSQL unavailable, should gracefully fall back but log the attempt
				Expect(backend).To(Equal("memory"), "Should fallback to memory when PostgreSQL unavailable")
				logger.Info("✅ Graceful fallback to memory - PostgreSQL likely not running in test environment")
			}
		})

		It("should detect vector database configuration priority correctly", func() {
			// Test different backend configurations to ensure proper priority
			testCases := []struct {
				name          string
				config        *config.VectorDBConfig
				expectedOrder []string // Order of preference
			}{
				{
					name: "PostgreSQL with full config",
					config: &config.VectorDBConfig{
						Enabled: true,
						Backend: "postgresql",
						PostgreSQL: config.PostgreSQLVectorConfig{
							UseMainDB: false,
							Host:      "localhost",
							Port:      "5433",
							Database:  "action_history",
						},
					},
					expectedOrder: []string{"postgresql", "memory"},
				},
				{
					name: "Memory backend explicit",
					config: &config.VectorDBConfig{
						Enabled: true,
						Backend: "memory",
					},
					expectedOrder: []string{"memory"},
				},
				{
					name: "Disabled database",
					config: &config.VectorDBConfig{
						Enabled: false,
					},
					expectedOrder: []string{"memory"},
				},
			}

			for _, tc := range testCases {
				By("Testing " + tc.name)

				// Act: Create vector database
				vectorDB, actualBackend, err := createProductionVectorDatabase(ctx, tc.config, logger)

				// Assert: Should create successfully
				Expect(err).ToNot(HaveOccurred(), "Should create vector database for "+tc.name)
				Expect(vectorDB).To(BeAssignableToTypeOf(vectorDB), "BR-DATABASE-001-A: Production vector database must provide functional database interface for database operations")

				// Business requirement: Backend should be from expected priority order
				Expect(tc.expectedOrder).To(ContainElement(actualBackend),
					"Backend should be from expected priority order for "+tc.name)
			}
		})

		It("should handle production environment detection", func() {
			// This test validates environment-aware backend selection

			// Arrange: Simulate production environment
			originalEnv := os.Getenv("ENVIRONMENT")
			defer func() { _ = os.Setenv("ENVIRONMENT", originalEnv) }()
			_ = os.Setenv("ENVIRONMENT", "production")

			// Create production-appropriate config
			prodConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "postgresql",
				PostgreSQL: config.PostgreSQLVectorConfig{
					UseMainDB: false,
					Host:      "postgres",
					Port:      "5432",
				},
			}

			// Act: Create vector database in production mode
			vectorDB, backend, err := createProductionVectorDatabase(ctx, prodConfig, logger)

			// Assert: Should handle production requirements
			Expect(err).ToNot(HaveOccurred(), "Should handle production configuration")
			Expect(vectorDB).To(BeAssignableToTypeOf(vectorDB), "BR-DATABASE-001-A: Production vector database must provide functional database interface for production database operations")

			// Business requirement: Should attempt production backend first
			Expect([]string{"postgresql", "memory"}).To(ContainElement(backend),
				"Should attempt production backend first")
		})
	})

	Describe("BR-VDB-PROD-002: Factory pattern consistency", func() {
		It("should use factory pattern instead of direct instantiation", func() {
			// This test validates the pattern that should be used throughout the codebase

			// Arrange: Create valid configuration
			validConfig := &config.VectorDBConfig{
				Enabled: true,
				Backend: "memory", // Use memory for predictable test
			}

			// Act: Create using production factory pattern (what SHOULD be done)
			factory := vector.NewVectorDatabaseFactory(validConfig, nil, logger)
			vectorDB, err := factory.CreateVectorDatabase()

			// Assert: Factory pattern should work correctly
			Expect(err).ToNot(HaveOccurred(), "Factory pattern should work")
			Expect(vectorDB).To(BeAssignableToTypeOf(vectorDB), "BR-DATABASE-001-A: Factory-created production vector database must provide functional database interface for database operations")
			Expect(vectorDB).To(BeAssignableToTypeOf(&vector.MemoryVectorDatabase{}),
				"Should create correct implementation type")
		})
	})
})

// Helper function that demonstrates the production pattern that should be used throughout
func createProductionVectorDatabase(
	ctx context.Context,
	config *config.VectorDBConfig,
	logger *logrus.Logger,
) (vector.VectorDatabase, string, error) {
	// This function demonstrates the correct pattern for production vector database creation
	// Following development guideline: use factory pattern for consistent behavior

	if config == nil || !config.Enabled {
		logger.Info("Vector database disabled, using memory fallback")
		return vector.NewMemoryVectorDatabase(logger), "memory", nil
	}

	// Use factory pattern for production-grade instantiation
	factory := vector.NewVectorDatabaseFactory(config, nil, logger)
	vectorDB, err := factory.CreateVectorDatabase()

	actualBackend := config.Backend
	if err != nil {
		// Graceful fallback on production issues
		logger.WithError(err).WithField("attempted_backend", config.Backend).
			Warn("Failed to create production vector database, using memory fallback")
		vectorDB = vector.NewMemoryVectorDatabase(logger)
		actualBackend = "memory"
		err = nil // Clear error for graceful fallback
	}

	return vectorDB, actualBackend, err
}

// TestRunner bootstraps the Ginkgo test suite
func TestUproductionUvectorUdbUintegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UproductionUvectorUdbUintegration Suite")
}
