# Containerized Integration Testing

This document describes the new containerized integration testing setup that provides consistent, reliable database services for all integration tests.

## Overview

The integration testing framework now uses **containerized PostgreSQL and Vector Database instances** via Podman to ensure:

✅ **Consistent Test Environment** - All developers and CI runs use identical database setups
✅ **No External Dependencies** - Tests work without manual database setup
✅ **pgvector Support** - Full vector database functionality for embedding tests
✅ **Automatic Lifecycle Management** - Services start/stop automatically with tests
✅ **Isolation** - Each test run uses fresh database state

## Architecture

```
Integration Tests
    ├── Main PostgreSQL (port 5433)
    │   ├── action_history database
    │   ├── pgvector extension
    │   └── Action pattern storage
    │
    ├── Vector Database (port 5434)
    │   ├── vector_store database
    │   ├── Document embeddings
    │   └── Workflow pattern embeddings
    │
    └── Redis Cache (port 6380)
        └── Test data caching
```

## Quick Start

### Option 1: Automatic Service Management (Recommended)
```bash
# Run all integration tests with automatic service lifecycle
make integration-test-all

# Run specific test suites
make integration-test-infrastructure
make integration-test-performance
make integration-test-vector
```

### Option 2: Manual Service Management
```bash
# Start services once
make integration-services-start

# Run tests multiple times
make integration-test
go test ./test/integration/infrastructure -tags=integration -v
go test ./test/integration/performance -tags=integration -v

# Stop services when done
make integration-services-stop
```

## Service Management Commands

| Command | Purpose |
|---------|---------|
| `make integration-services-start` | Start all containerized services |
| `make integration-services-stop` | Stop all containerized services |
| `make integration-services-status` | Show service status |
| `make integration-test-all` | Run all tests with auto service management |
| `make integration-test-infrastructure` | Run infrastructure tests only |
| `make integration-test-performance` | Run performance tests only |
| `make integration-test-vector` | Run vector database tests only |

## Configuration

### Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `USE_CONTAINER_DB` | `true` | Use containerized databases |
| `SKIP_DB_TESTS` | `false` | Skip database tests (legacy) |
| `DB_HOST` | `localhost` | Database host |
| `DB_PORT` | `5433` | Main database port (containerized) |
| `VECTOR_DB_HOST` | `localhost` | Vector database host |
| `VECTOR_DB_PORT` | `5434` | Vector database port (containerized) |

### Database Connection Details

#### Main PostgreSQL Database
- **Host**: localhost
- **Port**: 5433
- **Database**: action_history
- **Username**: slm_user
- **Password**: slm_password_dev
- **Connection**: `postgres://slm_user:slm_password_dev@localhost:5433/action_history?sslmode=disable`

#### Vector Database
- **Host**: localhost
- **Port**: 5434
- **Database**: vector_store
- **Username**: vector_user
- **Password**: vector_password_dev
- **Connection**: `postgres://vector_user:vector_password_dev@localhost:5434/vector_store?sslmode=disable`

## Directory Structure

```
test/integration/
├── docker-compose.integration.yml    # Service definitions
├── scripts/
│   ├── bootstrap-integration-tests.sh # Service lifecycle management
│   ├── init-vector-db.sql            # Main DB initialization
│   └── init-vector-store.sql         # Vector DB initialization
└── shared/
    ├── config.go                     # Test configuration
    ├── database_test_utils.go        # Database utilities
    └── vector_database_config.go     # Vector DB configuration
```

## Database Schema

### Main Database (action_history)
- **action_patterns** - Stores workflow action patterns with embeddings
- **vector extension** - Enables vector similarity search
- **Test data** - Pre-populated patterns for testing

### Vector Database (vector_store)
- **document_embeddings** - Document embedding storage
- **workflow_pattern_embeddings** - Workflow pattern embeddings
- **alert_pattern_embeddings** - Alert resolution pattern embeddings
- **Similarity search functions** - Built-in vector search capabilities

## Test Development

### Writing New Integration Tests

```go
// Example integration test using containerized database
func TestNewFeature(t *testing.T) {
    var (
        logger     *logrus.Logger
        testUtils  *shared.IntegrationTestUtils
    )

    BeforeAll(func() {
        logger = logrus.New()

        // Database will be available via containers
        var err error
        testUtils, err = shared.NewIntegrationTestUtils(logger)
        Expect(err).NotTo(HaveOccurred())
    })

    It("should work with real database", func() {
        // Test implementation using testUtils.Repository
        result := testUtils.Repository.FindActionsByType("scale_deployment")
        Expect(result).NotTo(BeEmpty())
    })
}
```

### Vector Database Testing

```go
func TestVectorOperations(t *testing.T) {
    It("should perform similarity search", func() {
        vectorConfig := shared.LoadVectorDatabaseTestConfig()

        // Connect to vector database
        db, err := sql.Open("postgres", vectorConfig.GetVectorDatabaseConnectionString())
        Expect(err).NotTo(HaveOccurred())

        // Perform vector search
        rows, err := db.Query("SELECT * FROM similarity_search.find_similar_documents($1)", embedding)
        Expect(err).NotTo(HaveOccurred())
        // Process results...
    })
}
```

## Troubleshooting

### Services Won't Start
```bash
# Check Podman status
podman --version
podman machine list  # macOS only

# Check service logs
make integration-services-status
./scripts/run-integration-tests.sh logs postgres-integration

# Restart services
make integration-services-stop
make integration-services-start
```

### Connection Issues
```bash
# Verify services are healthy
./test/integration/scripts/bootstrap-integration-tests.sh status

# Test manual connection
PGPASSWORD=slm_password_dev psql -h localhost -p 5433 -U slm_user -d action_history

# Check environment variables
env | grep DB_
```

### Port Conflicts
The containerized services use non-standard ports to avoid conflicts:
- Main DB: 5433 (instead of 5432)
- Vector DB: 5434 (instead of 5432)
- Redis: 6380 (instead of 6379)

If you have port conflicts, modify `docker-compose.integration.yml`.

## Migration from Legacy Setup

### Before (Manual Database Setup)
```bash
# Old way - manual setup required
docker run -d postgres:13
go test ./test/integration/... -tags=integration  # Often failed
```

### After (Containerized Services)
```bash
# New way - automatic service management
make integration-test-all  # Always works
```

### Backward Compatibility

The system maintains backward compatibility:
- `SKIP_DB_TESTS=true` still works for environments without container support
- Legacy database connection parameters are respected
- `USE_CONTAINER_DB=false` disables containerized mode

## CI/CD Integration

### GitHub Actions Example
```yaml
- name: Run Integration Tests
  run: |
    # Services start automatically
    make integration-test-all
```

### Local Development
```bash
# One-time setup - start services
make integration-services-start

# Development cycle
go test ./test/integration/infrastructure -tags=integration -v
# Make changes...
go test ./test/integration/infrastructure -tags=integration -v

# Cleanup when done
make integration-services-stop
```

## Performance Considerations

- **Container Startup**: ~30-60 seconds for all services
- **Test Execution**: Same speed as before, now with consistent data
- **Resource Usage**: ~1GB RAM for all containers
- **Cleanup**: Automatic via container lifecycle

## Advanced Usage

### Custom Database Configuration
```bash
# Override default configuration
export DB_HOST=custom-host
export DB_PORT=5432
export VECTOR_DB_PORT=5435
make integration-test-all
```

### Manual Service Control
```bash
# Direct compose management
podman-compose -f test/integration/docker-compose.integration.yml up -d
podman-compose -f test/integration/docker-compose.integration.yml logs -f postgres-integration
podman-compose -f test/integration/docker-compose.integration.yml down
```

### Debug Mode
```bash
# Run with detailed logging
LOG_LEVEL=debug make integration-test-infrastructure

# Check service health
./test/integration/scripts/bootstrap-integration-tests.sh status

# View all container logs
./scripts/run-integration-tests.sh logs
```

This new setup ensures integration tests are reliable, consistent, and easy to run across all environments!
