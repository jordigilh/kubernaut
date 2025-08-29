# Integration Test Setup

This document explains how to properly set up and run integration tests with **real components** and **fake infrastructure**.

## Architecture

Integration tests use **real application components** with **fake external infrastructure**:

```
Integration Test → Real containers/kubernetes-mcp-server → Fake K8s Cluster (envsetup)
                → Real Action History MCP Server → Real PostgreSQL (test database)
                → Real Application Code Under Test
```

## Why This Approach?

**✅ Correct Integration Testing:**
- Tests real component interactions and interfaces
- Validates actual HTTP/network communication
- Catches integration bugs that mocks would hide
- Tests real serialization/deserialization

**❌ What We DON'T Mock:**
- MCP servers (use real servers)
- Database (use real PostgreSQL)
- HTTP communication
- Application components

**✅ What We DO Mock/Fake:**
- External Kubernetes cluster (use fake cluster)
- External alerting systems (if any)
- External file systems (if any)

## Prerequisites

### 1. Database Setup
```bash
# Start PostgreSQL for testing
docker run -d --name test-postgres \
  -e POSTGRES_DB=action_history \
  -e POSTGRES_USER=slm_user \
  -e POSTGRES_PASSWORD=slm_password_dev \
  -p 5432:5432 \
  postgres:13

# Run database migrations
make migrate-database
```

### 2. Fake Kubernetes Cluster Setup
```bash
# Setup fake K8s environment for testing
make setup-test-cluster

# This creates a fake K8s cluster that the external MCP server can connect to
# The cluster has test pods, deployments, etc. for realistic testing
```

### 3. External K8s MCP Server Setup (Automatic)
```bash
# ✅ NO MANUAL SETUP REQUIRED!
# The K8s MCP server is automatically started and stopped by the integration test framework
# using Podman containers.

# Prerequisites: Ensure Podman is installed
podman --version

# Optional: Pre-pull the MCP server image to speed up tests
podman pull ghcr.io/containers/kubernetes-mcp-server:latest
```

## Running Integration Tests

### Environment Variables
```bash
# Required for integration tests
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=action_history
export DB_USER=slm_user
export DB_PASSWORD=slm_password_dev

# Optional: K8s MCP server configuration (automatic if not set)
export K8S_MCP_SERVER_ENDPOINT=http://localhost:8080      # Default endpoint
export K8S_MCP_SERVER_IMAGE=ghcr.io/containers/kubernetes-mcp-server:latest  # Image to use
export TEST_KUBECONFIG_PATH=/tmp/test-kubeconfig          # Path to test kubeconfig

# Optional: Skip slow tests
export SKIP_SLOW_TESTS=false
```

### Run Tests
```bash
# Run all integration tests
make test-integration

# Run specific integration test suite
go test -tags=integration ./test/integration/e2e/...

# Run with verbose output
go test -tags=integration -v ./test/integration/...
```

## Test Infrastructure Components

### 1. IntegrationTestUtils
- Sets up fake K8s cluster
- Creates real Action History MCP server
- Provides HTTP client for real K8s MCP server
- Manages PostgreSQL connections

### 2. HTTPKubernetesMCPClient
- Real HTTP client implementing `slm.K8sMCPServer` interface
- Makes actual HTTP requests to `containers/kubernetes-mcp-server`
- Handles real JSON serialization/deserialization
- Provides realistic error handling

### 3. Test Environment Lifecycle
```go
// Setup
testUtils, err := shared.NewIntegrationTestUtils(logger)
// Creates:
// - Real PostgreSQL connection
// - Fake K8s cluster
// - Real Action History MCP server
// - HTTP client for external K8s MCP server

// Usage
mcpClient := testUtils.CreateMCPClient(config)
// Returns client connected to REAL MCP servers

// Cleanup
defer testUtils.Close()
// Cleans up K8s cluster and database connections
```

## Troubleshooting

### K8s MCP Server Issues
```bash
# Check if server is accessible
curl http://localhost:8080/health

# Check running containers
podman ps | grep k8s-mcp-server-test

# Check container logs (replace CONTAINER_ID with actual ID)
podman logs CONTAINER_ID

# Manually stop test containers if needed
podman stop $(podman ps -q --filter "name=k8s-mcp-server-test")

# Check port usage
lsof -i :8080
```

### Database Connection Issues
```bash
# Check PostgreSQL is running
docker ps | grep postgres

# Test connection
psql -h localhost -p 5432 -U slm_user -d action_history
```

### Fake K8s Cluster Issues
```bash
# Reset test environment
make cleanup-test-cluster
make setup-test-cluster

# Check cluster status
kubectl --kubeconfig=./test/kubeconfig get nodes
```

## Integration Test Development

### Writing New Integration Tests
```go
func TestNewFeature(t *testing.T) {
    // Setup
    logger := logrus.New()
    testUtils, err := shared.NewIntegrationTestUtils(logger)
    require.NoError(t, err)
    defer testUtils.Close()

    // Get real MCP client (connects to real servers)
    config := shared.LoadConfig()
    mcpClient := testUtils.CreateMCPClient(config)

    // Test real interactions
    response, err := mcpClient.GetActionContext(ctx, testAlert)
    // This makes REAL HTTP requests to REAL MCP servers

    // Assertions on real responses
    assert.NoError(t, err)
    assert.NotNil(t, response.ClusterState)
}
```

### Best Practices
1. **Always use real components** - no mocks except for external I/O boundaries
2. **Test actual serialization** - use real JSON/HTTP communication
3. **Validate real errors** - test actual HTTP errors, timeouts, etc.
4. **Clean up properly** - always call `testUtils.Close()`
5. **Use realistic test data** - create realistic K8s resources in fake cluster

This approach ensures integration tests actually validate real component integration! 🚀
