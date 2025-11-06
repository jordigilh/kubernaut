package contextapi

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // DD-010: Migrated from lib/pq
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// "github.com/jordigilh/kubernaut/pkg/contextapi/client"
)

func TestContextAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API Integration Suite (ADR-016: Podman Redis + Data Storage Service + PostgreSQL)")
}

var (
	db         *sql.DB
	sqlxDB     *sqlx.DB
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	testSchema string
	// TODO: Remove direct DB access - use Data Storage REST API per ADR-032
	// dbClient     *client.PostgresClient
	cacheManager     cache.CacheManager                        // ADR-016: Real Redis cache for integration tests
	dataStorageInfra *infrastructure.DataStorageInfrastructure // Shared infrastructure
)

const (
	redisPort       = 6379                    // Standard Redis port for Context API cache
	dataStoragePort = 8085                    // Data Storage Service port
	redisContainer  = "contextapi-redis-test" // Context API's own Redis
)

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Setup logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).ToNot(HaveOccurred())

	// Start Data Storage Service infrastructure using shared helper
	GinkgoWriter.Println("🚀 Starting Data Storage Service infrastructure (shared helper)...")
	dsConfig := &infrastructure.DataStorageConfig{
		PostgresPort: "5433",
		RedisPort:    "6380",
		ServicePort:  fmt.Sprintf("%d", dataStoragePort),
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password",
	}

	dataStorageInfra, err = infrastructure.StartDataStorageInfrastructure(dsConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Data Storage infrastructure should start successfully")

	// Start Context API's own Redis for caching (separate from Data Storage Redis)
	GinkgoWriter.Println("🚀 Starting Context API Redis cache (ADR-016)...")

	// Stop and remove existing Redis container (cleanup from previous runs)
	exec.Command("podman", "stop", redisContainer).Run()
	exec.Command("podman", "rm", redisContainer).Run()

	// Start Redis container
	cmd := exec.Command("podman", "run", "-d",
		"--name", redisContainer,
		"-p", fmt.Sprintf("%d:6379", redisPort),
		"redis:7-alpine")
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Failed to start Redis container: %s", string(output))

	// Wait for Redis to be ready
	time.Sleep(2 * time.Second)

	// Verify Redis is accessible
	redisAddr := fmt.Sprintf("localhost:%d", redisPort)
	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", redisContainer, "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	GinkgoWriter.Printf("✅ Context API Redis started successfully at %s\n", redisAddr)

	// Create real cache manager with Redis
	cacheConfig := &cache.Config{
		RedisAddr:  redisAddr,
		LRUSize:    1000, // Test cache size
		DefaultTTL: 5 * time.Minute,
	}
	cacheManager, err = cache.NewCacheManager(cacheConfig, logger)
	Expect(err).ToNot(HaveOccurred(), "Failed to create cache manager")

	GinkgoWriter.Println("✅ Cache manager initialized with real Redis")

	// Use Data Storage infrastructure's database connection
	db = dataStorageInfra.DB
	sqlxDB = sqlx.NewDb(db, "pgx")
	testSchema = "public" // Use Data Storage schema

	GinkgoWriter.Println("✅ Context API integration test environment ready!")
	GinkgoWriter.Printf("   - Data Storage Service: %s\n", dataStorageInfra.ServiceURL)
	GinkgoWriter.Printf("   - PostgreSQL: localhost:%s\n", dsConfig.PostgresPort)
	GinkgoWriter.Printf("   - Context API Redis: localhost:%d\n", redisPort)
	GinkgoWriter.Println("   - Schema: public (Data Storage schema - shared infrastructure)")
})

var _ = AfterSuite(func() {
	defer cancel()

	// Close cache manager
	if cacheManager != nil {
		GinkgoWriter.Println("Closing cache manager...")
		err := cacheManager.Close()
		if err != nil {
			GinkgoWriter.Printf("⚠️  Cache manager close failed (non-fatal): %v\n", err)
		}
	}

	// Stop and remove Context API Redis container
	GinkgoWriter.Println("Stopping Context API Redis container...")
	if output, err := exec.Command("podman", "stop", redisContainer).CombinedOutput(); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to stop Redis (non-fatal): %v, output: %s\n", err, string(output))
	}
	if output, err := exec.Command("podman", "rm", redisContainer).CombinedOutput(); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to remove Redis (non-fatal): %v, output: %s\n", err, string(output))
	}
	GinkgoWriter.Println("✅ Context API Redis container cleaned up")

	// Stop Data Storage infrastructure using shared helper
	if dataStorageInfra != nil {
		dataStorageInfra.Stop(GinkgoWriter)
	}

	if logger != nil {
		logger.Sync()
	}

	GinkgoWriter.Println("✅ Context API integration test cleanup complete")
})
