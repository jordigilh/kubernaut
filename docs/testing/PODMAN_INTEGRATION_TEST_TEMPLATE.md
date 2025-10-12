# Podman Integration Test Template

**Date**: 2025-10-12
**Status**: ✅ READY FOR USE
**Use For**: Stateless services with ONLY external dependencies (databases, Redis, message queues)
**See Also**: [INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md](./INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md)

---

## Quick Start (5 minutes)

### When to Use This Template

**Use Podman/Testcontainers** when your service:
- ✅ Does NOT interact with Kubernetes APIs
- ✅ Does NOT use CRDs
- ✅ Does NOT require RBAC/ServiceAccount auth
- ✅ Only needs: PostgreSQL, Redis, MongoDB, RabbitMQ, etc.

**Examples**: Gateway Service, Data Storage Service, Context Optimization Service

---

## Complete Example: Service with PostgreSQL + Redis

```go
package myservice_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-redis/redis/v8"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/myservice"
)

func TestMyServiceIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "My Service Integration Suite (Podman)")
}

var (
	// Containers
	postgresContainer testcontainers.Container
	redisContainer    testcontainers.Container

	// Connections
	db          *sql.DB
	redisClient *redis.Client
	dbURL       string
	redisAddr   string

	// Service under test
	service *myservice.Service
	logger  *zap.Logger
)

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Setup logger
	logger, _ = zap.NewDevelopment()

	// =====================================
	// PostgreSQL Container Setup
	// =====================================
	By("Starting PostgreSQL container")
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithStartupTimeout(60 * time.Second),
	}

	var err error
	postgresContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to start PostgreSQL container")

	// Get PostgreSQL connection details
	pgHost, err := postgresContainer.Host(ctx)
	Expect(err).ToNot(HaveOccurred())

	pgPort, err := postgresContainer.MappedPort(ctx, "5432")
	Expect(err).ToNot(HaveOccurred())

	dbURL = fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable",
		pgHost, pgPort.Port())

	// Connect to PostgreSQL
	db, err = sql.Open("postgres", dbURL)
	Expect(err).ToNot(HaveOccurred(), "Failed to connect to PostgreSQL")

	// Verify connection
	err = db.Ping()
	Expect(err).ToNot(HaveOccurred(), "PostgreSQL not responding")

	// Initialize schema (if needed)
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_data (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			value TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	Expect(err).ToNot(HaveOccurred(), "Failed to initialize schema")

	GinkgoWriter.Println("✅ PostgreSQL container ready")

	// =====================================
	// Redis Container Setup
	// =====================================
	By("Starting Redis container")
	redisReq := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").
			WithStartupTimeout(30 * time.Second),
	}

	redisContainer, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: redisReq,
		Started:          true,
	})
	Expect(err).ToNot(HaveOccurred(), "Failed to start Redis container")

	// Get Redis connection details
	redisHost, err := redisContainer.Host(ctx)
	Expect(err).ToNot(HaveOccurred())

	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	Expect(err).ToNot(HaveOccurred())

	redisAddr = fmt.Sprintf("%s:%s", redisHost, redisPort.Port())

	// Connect to Redis
	redisClient = redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})

	// Verify connection
	err = redisClient.Ping(ctx).Err()
	Expect(err).ToNot(HaveOccurred(), "Redis not responding")

	GinkgoWriter.Println("✅ Redis container ready")

	// =====================================
	// Service Initialization
	// =====================================
	By("Initializing service under test")
	service = myservice.New(db, redisClient, logger)

	GinkgoWriter.Println("✅ Podman integration test environment ready!")
	GinkgoWriter.Printf("   PostgreSQL: %s\n", dbURL)
	GinkgoWriter.Printf("   Redis: %s\n", redisAddr)
})

var _ = AfterSuite(func() {
	ctx := context.Background()

	By("Cleaning up test environment")

	// Close connections
	if db != nil {
		_ = db.Close()
	}
	if redisClient != nil {
		_ = redisClient.Close()
	}

	// Terminate containers
	if postgresContainer != nil {
		_ = postgresContainer.Terminate(ctx)
	}
	if redisContainer != nil {
		_ = redisContainer.Terminate(ctx)
	}

	GinkgoWriter.Println("✅ Podman integration test environment cleaned up!")
})

// Helper: Clean database between tests
var _ = BeforeEach(func() {
	ctx := context.Background()

	// Clear Redis
	err := redisClient.FlushDB(ctx).Err()
	Expect(err).ToNot(HaveOccurred())

	// Clear PostgreSQL tables
	_, err = db.ExecContext(ctx, "TRUNCATE TABLE test_data RESTART IDENTITY CASCADE")
	Expect(err).ToNot(HaveOccurred())
})

// =====================================
// Example Integration Tests
// =====================================

var _ = Describe("BR-SERVICE-001: Data Persistence", func() {
	It("should persist data to PostgreSQL", func() {
		ctx := context.Background()

		// Create test data
		data := &myservice.TestData{
			Name:  "test-item",
			Value: "test-value",
		}

		// Service writes to database
		result, err := service.Create(ctx, data)
		Expect(err).ToNot(HaveOccurred())
		Expect(result.ID).To(BeNumerically(">", 0))

		// Verify in database directly
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_data WHERE name = $1", "test-item").Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1))
	})
})

var _ = Describe("BR-SERVICE-002: Cache Integration", func() {
	It("should cache frequently accessed data in Redis", func() {
		ctx := context.Background()

		// First access (cache miss)
		result1, err := service.GetWithCache(ctx, "test-key")
		Expect(err).ToNot(HaveOccurred())

		// Verify Redis was populated
		cached, err := redisClient.Get(ctx, "cache:test-key").Result()
		Expect(err).ToNot(HaveOccurred())
		Expect(cached).ToNot(BeEmpty())

		// Second access (cache hit)
		result2, err := service.GetWithCache(ctx, "test-key")
		Expect(err).ToNot(HaveOccurred())
		Expect(result2).To(Equal(result1))
	})
})

var _ = Describe("BR-SERVICE-003: Transaction Handling", func() {
	It("should rollback on error", func() {
		ctx := context.Background()

		// Attempt transaction that will fail
		err := service.CreateWithValidation(ctx, &myservice.TestData{
			Name: "", // Invalid - will trigger rollback
		})
		Expect(err).To(HaveOccurred())

		// Verify no data was persisted
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_data").Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(0), "Transaction should have rolled back")
	})
})
```

---

## Container Configurations

### PostgreSQL

```go
postgresReq := testcontainers.ContainerRequest{
	Image:        "postgres:15-alpine",
	ExposedPorts: []string{"5432/tcp"},
	Env: map[string]string{
		"POSTGRES_USER":     "testuser",
		"POSTGRES_PASSWORD": "testpass",
		"POSTGRES_DB":       "testdb",
	},
	WaitingFor: wait.ForLog("database system is ready to accept connections").
		WithStartupTimeout(60 * time.Second),
}
```

### Redis

```go
redisReq := testcontainers.ContainerRequest{
	Image:        "redis:7-alpine",
	ExposedPorts: []string{"6379/tcp"},
	WaitingFor:   wait.ForLog("Ready to accept connections").
		WithStartupTimeout(30 * time.Second),
}
```

### MongoDB

```go
mongoReq := testcontainers.ContainerRequest{
	Image:        "mongo:6",
	ExposedPorts: []string{"27017/tcp"},
	Env: map[string]string{
		"MONGO_INITDB_ROOT_USERNAME": "testuser",
		"MONGO_INITDB_ROOT_PASSWORD": "testpass",
	},
	WaitingFor: wait.ForLog("Waiting for connections").
		WithStartupTimeout(60 * time.Second),
}
```

### RabbitMQ

```go
rabbitReq := testcontainers.ContainerRequest{
	Image:        "rabbitmq:3-management-alpine",
	ExposedPorts: []string{"5672/tcp", "15672/tcp"},
	Env: map[string]string{
		"RABBITMQ_DEFAULT_USER": "testuser",
		"RABBITMQ_DEFAULT_PASS": "testpass",
	},
	WaitingFor: wait.ForLog("Server startup complete").
		WithStartupTimeout(90 * time.Second),
}
```

### Elasticsearch

```go
esReq := testcontainers.ContainerRequest{
	Image:        "elasticsearch:8.11.0",
	ExposedPorts: []string{"9200/tcp"},
	Env: map[string]string{
		"discovery.type":         "single-node",
		"xpack.security.enabled": "false",
		"ES_JAVA_OPTS":          "-Xms512m -Xmx512m",
	},
	WaitingFor: wait.ForHTTP("/").
		WithPort("9200/tcp").
		WithStartupTimeout(90 * time.Second),
}
```

---

## Common Patterns

### Pattern 1: Database Schema Initialization

```go
var _ = BeforeSuite(func() {
	// ... container setup ...

	// Initialize schema
	schema := `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(255) UNIQUE NOT NULL,
			email VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS orders (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id),
			total DECIMAL(10,2) NOT NULL,
			status VARCHAR(50) NOT NULL,
			created_at TIMESTAMP DEFAULT NOW()
		);
	`

	_, err := db.ExecContext(context.Background(), schema)
	Expect(err).ToNot(HaveOccurred(), "Failed to initialize schema")
})
```

### Pattern 2: Test Data Cleanup

```go
// Option A: Truncate tables (fast)
var _ = BeforeEach(func() {
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `
		TRUNCATE TABLE orders, users RESTART IDENTITY CASCADE
	`)
	Expect(err).ToNot(HaveOccurred())

	// Clear Redis
	err = redisClient.FlushDB(ctx).Err()
	Expect(err).ToNot(HaveOccurred())
})

// Option B: Drop and recreate database (thorough but slower)
var _ = BeforeEach(func() {
	ctx := context.Background()

	// Drop all tables
	_, err := db.ExecContext(ctx, `
		DROP SCHEMA public CASCADE;
		CREATE SCHEMA public;
	`)
	Expect(err).ToNot(HaveOccurred())

	// Reinitialize schema
	initializeSchema(ctx, db)
})
```

### Pattern 3: Container Health Checks

```go
func waitForPostgreSQL(db *sql.DB, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for PostgreSQL")
		case <-ticker.C:
			if err := db.Ping(); err == nil {
				return nil
			}
		}
	}
}

var _ = BeforeSuite(func() {
	// ... container setup ...

	// Additional health check
	err := waitForPostgreSQL(db, 30*time.Second)
	Expect(err).ToNot(HaveOccurred())
})
```

### Pattern 4: Connection Pooling

```go
var _ = BeforeSuite(func() {
	// ... container setup and connection ...

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Verify pool
	err := db.Ping()
	Expect(err).ToNot(HaveOccurred())
})
```

---

## Performance Comparison

### Startup Time Comparison

| Environment | Startup Time | Per-Test Overhead | CI/CD Time |
|-------------|-------------|-------------------|------------|
| **Podman** | 2-5 seconds | < 1 second | 3-6 seconds |
| **Kind** | 30-60 seconds | 5-10 seconds | 45-90 seconds |
| **Improvement** | **6-12x faster** | **5-10x faster** | **15x faster** |

### Example: Gateway Service

**Before (Kind)**:
```
$ make test-integration-gateway
[Kind cluster startup: 45s]
[Test execution: 12s]
[Kind cleanup: 8s]
Total: 65 seconds
```

**After (Podman)**:
```
$ make test-integration-gateway
[Podman containers: 3s]
[Test execution: 12s]
[Container cleanup: 1s]
Total: 16 seconds
```

**Improvement**: **75% faster** (49 seconds saved)

---

## Migration Guide

### Migrating from Kind to Podman

**Step 1: Update go.mod**
```bash
go get github.com/testcontainers/testcontainers-go@latest
```

**Step 2: Replace Kind suite setup**

**Before (Kind)**:
```go
var suite *kind.IntegrationSuite

var _ = BeforeSuite(func() {
	suite = kind.Setup("myservice-test")
	suite.WaitForPostgreSQLReady(60 * time.Second)
})
```

**After (Podman)**:
```go
var postgresContainer testcontainers.Container
var db *sql.DB

var _ = BeforeSuite(func() {
	ctx := context.Background()

	// Start PostgreSQL container
	postgresReq := testcontainers.ContainerRequest{
		Image: "postgres:15-alpine",
		// ... config ...
	}

	postgresContainer, err := testcontainers.GenericContainer(ctx, ...)
	Expect(err).ToNot(HaveOccurred())

	// Connect to database
	// ...
})
```

**Step 3: Update test cleanup**

**Before (Kind)**:
```go
var _ = AfterSuite(func() {
	suite.Cleanup()
})
```

**After (Podman)**:
```go
var _ = AfterSuite(func() {
	ctx := context.Background()

	if db != nil {
		_ = db.Close()
	}
	if postgresContainer != nil {
		_ = postgresContainer.Terminate(ctx)
	}
})
```

**Step 4: Update tests (minimal changes)**

Tests themselves often require **no changes** - just the setup/teardown!

---

## Troubleshooting

### Issue 1: Container Startup Timeout

**Symptom**: "timeout waiting for container"

**Solution**: Increase timeout or check wait condition
```go
WaitingFor: wait.ForLog("ready to accept connections").
	WithStartupTimeout(120 * time.Second), // Increase timeout
```

### Issue 2: Port Already in Use

**Symptom**: "bind: address already in use"

**Solution**: Let testcontainers assign random ports (default behavior)
```go
// DON'T do this:
// HostConfigModifier: func(hc *container.HostConfig) {
//     hc.PortBindings = nat.PortMap{"5432/tcp": []nat.PortBinding{{HostPort: "5432"}}}
// }

// DO this (automatic port assignment):
ExposedPorts: []string{"5432/tcp"}, // testcontainers assigns random host port
```

### Issue 3: Slow Container Startup

**Symptom**: Tests take > 10 seconds to start

**Solution**: Use alpine images and optimize wait conditions
```go
// Prefer alpine images (smaller, faster)
Image: "postgres:15-alpine", // Not postgres:15

// Optimize wait condition (don't wait longer than needed)
WaitingFor: wait.ForLog("ready").WithStartupTimeout(30 * time.Second),
```

---

## Best Practices

### DO ✅
1. **Use alpine images** - Smaller, faster startup
2. **Let testcontainers assign ports** - Avoids conflicts
3. **Clean data between tests** - Use TRUNCATE or FlushDB
4. **Configure connection pools** - Improve performance
5. **Add container health checks** - Verify connectivity

### DON'T ❌
1. **Don't hardcode ports** - Use dynamic port assignment
2. **Don't skip wait conditions** - Containers may not be ready
3. **Don't reuse containers across test suites** - Isolation is important
4. **Don't forget to close connections** - Memory leaks in tests
5. **Don't use full images unnecessarily** - alpine is faster

---

## go.mod Dependencies

```go
require (
	github.com/testcontainers/testcontainers-go v0.27.0
	github.com/onsi/ginkgo/v2 v2.13.2
	github.com/onsi/gomega v1.30.0
	github.com/lib/pq v1.10.9 // PostgreSQL driver
	github.com/go-redis/redis/v8 v8.11.5 // Redis client
	go.uber.org/zap v1.26.0 // Logging
)
```

---

## Makefile Integration

```makefile
# Podman integration tests
.PHONY: test-integration-podman
test-integration-podman:
	@echo "Running Podman integration tests..."
	go test -v -timeout 5m ./test/integration/... -tags=integration

# With coverage
.PHONY: test-integration-coverage
test-integration-coverage:
	go test -v -timeout 5m -coverprofile=coverage.out ./test/integration/... -tags=integration
	go tool cover -html=coverage.out -o coverage.html
```

---

## Related Documents

- [INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md](./INTEGRATION_TEST_ENVIRONMENT_DECISION_TREE.md) - When to use Podman vs Kind
- [KIND_CLUSTER_TEST_TEMPLATE.md](./KIND_CLUSTER_TEST_TEMPLATE.md) - Kind template for K8s services
- [03-testing-strategy.mdc](../../.cursor/rules/03-testing-strategy.mdc) - Overall testing strategy
- [INTEGRATION_E2E_NO_MOCKS_POLICY.md](./INTEGRATION_E2E_NO_MOCKS_POLICY.md) - No mocks policy

---

**Template Status**: ✅ READY FOR USE
**Use For**: Gateway, Data Storage, Context Optimization, and other stateless services
**Performance**: 6-12x faster than Kind for non-K8s services
**Simplicity**: 5-10 lines setup vs 80+ for custom Kind setup

