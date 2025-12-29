# Integration Test Infrastructure Setup Guide

**Version**: 1.0.0
**Last Updated**: 2025-12-21
**Authoritative Reference**: DD-TEST-002 (Integration Test Container Orchestration)

---

## 🎯 Purpose

This guide provides step-by-step instructions for setting up reliable integration test infrastructure using the **sequential startup pattern** to avoid `podman-compose` race conditions.

**Use this guide if your service**:
- Requires PostgreSQL, Redis, or DataStorage for integration tests
- Experiences "exit 137" (SIGKILL) container failures
- Has DNS resolution errors ("lookup postgres: no such host")
- Has BeforeSuite failures due to infrastructure unavailability

---

## 📋 Prerequisites

### Required Tools
```bash
# Verify Podman is installed
podman --version  # Should be >= 4.0

# Verify Go is installed
go version  # Should be >= 1.21

# Verify Ginkgo is installed
ginkgo version  # Should be >= 2.13
```

### Port Allocation
Ensure your service has dedicated ports per **DD-TEST-001**:

| Service | PostgreSQL | Redis | DataStorage HTTP | DataStorage Metrics |
|---------|------------|-------|------------------|---------------------|
| DataStorage | 15432 | 16379 | 18080 | 18090 |
| Notification | 15453 | 16399 | 18110 | 19110 |
| RemediationOrchestrator | 15454 | 16400 | 18111 | 19111 |
| *(Your Service)* | *(Next available)* | *(Next available)* | *(Next available)* | *(Next available)* |

---

## 🚀 Step-by-Step Setup

### Step 1: Create Setup Script

Create `test/integration/{service}/setup-infrastructure.sh`:

```bash
#!/bin/bash
# Integration Test Infrastructure Setup
# Service: {SERVICE_NAME}
# Per DD-TEST-002: Sequential startup pattern for multi-service dependencies

set -e

# Configuration (per DD-TEST-001 port allocation)
SERVICE_NAME="{service}"
POSTGRES_PORT={POSTGRES_PORT}
REDIS_PORT={REDIS_PORT}
DS_HTTP_PORT={DS_HTTP_PORT}
DS_METRICS_PORT={DS_METRICS_PORT}

# Container names (standardized format)
POSTGRES_CONTAINER="${SERVICE_NAME}_postgres_1"
REDIS_CONTAINER="${SERVICE_NAME}_redis_1"
DATASTORAGE_CONTAINER="${SERVICE_NAME}_datastorage_1"
NETWORK_NAME="${SERVICE_NAME}_test-network"

echo "🚀 Starting ${SERVICE_NAME} integration test infrastructure..."

# ========================================
# STEP 1: Cleanup existing containers
# ========================================
echo "🧹 Cleaning up existing containers..."
podman stop $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER 2>/dev/null || true
podman rm $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER 2>/dev/null || true

# ========================================
# STEP 2: Create network
# ========================================
echo "🌐 Creating test network..."
podman network create $NETWORK_NAME 2>/dev/null || echo "  Network already exists"

# ========================================
# STEP 3: Start PostgreSQL FIRST
# ========================================
echo "🔵 Starting PostgreSQL..."
podman run -d \
  --name $POSTGRES_CONTAINER \
  --network $NETWORK_NAME \
  -p $POSTGRES_PORT:5432 \
  -e POSTGRES_USER=slm_user \
  -e POSTGRES_PASSWORD=test_password \
  -e POSTGRES_DB=action_history \
  postgres:16-alpine

# WAIT for PostgreSQL to be ready (critical!)
echo "⏳ Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
  if podman exec $POSTGRES_CONTAINER pg_isready -U slm_user >/dev/null 2>&1; then
    echo "  ✅ PostgreSQL is ready"
    break
  fi
  if [ $i -eq 30 ]; then
    echo "  ❌ PostgreSQL failed to start within 30 seconds"
    podman logs $POSTGRES_CONTAINER
    exit 1
  fi
  sleep 1
done

# ========================================
# STEP 4: Run database migrations
# ========================================
echo "🔄 Running database migrations..."
# Option A: If you have a migrations container
# podman run --rm \
#   --network $NETWORK_NAME \
#   -e DB_HOST=$POSTGRES_CONTAINER \
#   -e DB_PORT=5432 \
#   -e DB_USER=slm_user \
#   -e DB_PASSWORD=test_password \
#   -e DB_NAME=action_history \
#   ${SERVICE_NAME}_migrations:latest

# Option B: If migrations are built into your service
echo "  (Migrations will run when DataStorage starts)"

# ========================================
# STEP 5: Start Redis SECOND
# ========================================
echo "🔵 Starting Redis..."
podman run -d \
  --name $REDIS_CONTAINER \
  --network $NETWORK_NAME \
  -p $REDIS_PORT:6379 \
  redis:7-alpine

# WAIT for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
  if podman exec $REDIS_CONTAINER redis-cli ping 2>/dev/null | grep -q PONG; then
    echo "  ✅ Redis is ready"
    break
  fi
  if [ $i -eq 10 ]; then
    echo "  ❌ Redis failed to start within 10 seconds"
    podman logs $REDIS_CONTAINER
    exit 1
  fi
  sleep 1
done

# ========================================
# STEP 6: Start DataStorage LAST
# ========================================
echo "🔵 Starting DataStorage..."
podman run -d \
  --name $DATASTORAGE_CONTAINER \
  --network $NETWORK_NAME \
  -p $DS_HTTP_PORT:8080 \
  -p $DS_METRICS_PORT:9090 \
  -e DB_HOST=$POSTGRES_CONTAINER \
  -e DB_PORT=5432 \
  -e DB_USER=slm_user \
  -e DB_PASSWORD=test_password \
  -e DB_NAME=action_history \
  -e REDIS_HOST=$REDIS_CONTAINER \
  -e REDIS_PORT=6379 \
  datastorage:latest

# WAIT for DataStorage health check
echo "⏳ Waiting for DataStorage to be healthy..."
for i in {1..30}; do
  if curl -s http://127.0.0.1:$DS_HTTP_PORT/health 2>/dev/null | grep -q "ok"; then
    echo "  ✅ DataStorage is healthy"
    break
  fi
  if [ $i -eq 30 ]; then
    echo "  ❌ DataStorage failed to become healthy within 30 seconds"
    echo "  Container logs:"
    podman logs $DATASTORAGE_CONTAINER
    exit 1
  fi
  sleep 1
done

# ========================================
# SUCCESS
# ========================================
echo ""
echo "✅ Infrastructure is ready!"
echo ""
echo "Service Status:"
echo "  - PostgreSQL: http://127.0.0.1:$POSTGRES_PORT"
echo "  - Redis: http://127.0.0.1:$REDIS_PORT"
echo "  - DataStorage: http://127.0.0.1:$DS_HTTP_PORT"
echo "  - DataStorage Metrics: http://127.0.0.1:$DS_METRICS_PORT/metrics"
echo ""
echo "You can now run integration tests:"
echo "  cd test/integration/${SERVICE_NAME}"
echo "  ginkgo -v"
echo ""
```

Make the script executable:
```bash
chmod +x test/integration/{service}/setup-infrastructure.sh
```

---

### Step 2: Create Teardown Script

Create `test/integration/{service}/teardown-infrastructure.sh`:

```bash
#!/bin/bash
# Integration Test Infrastructure Teardown
# Service: {SERVICE_NAME}

set -e

SERVICE_NAME="{service}"
POSTGRES_CONTAINER="${SERVICE_NAME}_postgres_1"
REDIS_CONTAINER="${SERVICE_NAME}_redis_1"
DATASTORAGE_CONTAINER="${SERVICE_NAME}_datastorage_1"
NETWORK_NAME="${SERVICE_NAME}_test-network"

echo "🧹 Tearing down ${SERVICE_NAME} integration test infrastructure..."

# Stop containers
echo "⏸️  Stopping containers..."
podman stop $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER 2>/dev/null || true

# Remove containers
echo "🗑️  Removing containers..."
podman rm $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER 2>/dev/null || true

# Remove network
echo "🌐 Removing network..."
podman network rm $NETWORK_NAME 2>/dev/null || true

echo "✅ Teardown complete!"
```

Make the script executable:
```bash
chmod +x test/integration/{service}/teardown-infrastructure.sh
```

---

### Step 3: Update Test Suite BeforeSuite

Update `test/integration/{service}/suite_test.go`:

```go
package {service}

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	ctx             context.Context
	cancel          context.CancelFunc
	dataStorageURL  string
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "{Service} Integration Test Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	// Start infrastructure using sequential startup script
	// Per DD-TEST-002: This eliminates podman-compose race conditions
	GinkgoWriter.Println("🚀 Starting infrastructure with sequential startup...")

	cmd := exec.Command("./setup-infrastructure.sh")
	cmd.Dir = "."
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	err := cmd.Run()
	if err != nil {
		Fail(fmt.Sprintf("Failed to start infrastructure: %v\n"+
			"See DD-TEST-002 for troubleshooting guidance", err))
	}

	// Set DataStorage URL (per DD-TEST-001 port allocation)
	dataStorageURL = "http://127.0.0.1:{DS_HTTP_PORT}"

	// Verify DataStorage is healthy using Eventually()
	// Per DD-TEST-002: Use 30s timeout for cold start on macOS
	GinkgoWriter.Println("⏳ Verifying DataStorage health...")
	Eventually(func() int {
		resp, err := http.Get(dataStorageURL + "/health")
		if err != nil {
			GinkgoWriter.Printf("  Health check failed: %v\n", err)
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}, "30s", "1s").Should(Equal(http.StatusOK),
		"DataStorage should be healthy within 30 seconds")

	GinkgoWriter.Println("✅ Infrastructure ready!")
})

var _ = AfterSuite(func() {
	cancel()

	// Teardown infrastructure
	GinkgoWriter.Println("🧹 Tearing down infrastructure...")

	cmd := exec.Command("./teardown-infrastructure.sh")
	cmd.Dir = "."
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	_ = cmd.Run() // Best effort cleanup
})
```

---

### Step 4: Update Makefile

Add integration test targets to your `Makefile`:

```makefile
.PHONY: test-integration-{service}
test-integration-{service}:
	@echo "🧪 Running {Service} integration tests..."
	@cd test/integration/{service} && ./setup-infrastructure.sh
	@cd test/integration/{service} && ginkgo -v --trace --progress
	@cd test/integration/{service} && ./teardown-infrastructure.sh

.PHONY: test-integration-{service}-watch
test-integration-{service}-watch:
	@echo "👀 Watching {Service} integration tests..."
	@cd test/integration/{service} && ./setup-infrastructure.sh
	@cd test/integration/{service} && ginkgo watch -v
	# Note: teardown runs when you stop the watch (Ctrl+C)

.PHONY: clean-integration-{service}
clean-integration-{service}:
	@echo "🧹 Cleaning {Service} integration test infrastructure..."
	@cd test/integration/{service} && ./teardown-infrastructure.sh
```

---

## ✅ Verification

### Step 1: Test Script Execution

```bash
# Start infrastructure
cd test/integration/{service}
./setup-infrastructure.sh

# Verify containers are running
podman ps | grep {service}

# Expected output:
# {service}_postgres_1      Up (healthy)
# {service}_redis_1         Up (healthy)
# {service}_datastorage_1   Up (healthy)
```

### Step 2: Test Health Endpoints

```bash
# PostgreSQL
podman exec {service}_postgres_1 pg_isready -U slm_user

# Redis
podman exec {service}_redis_1 redis-cli ping

# DataStorage
curl http://127.0.0.1:{DS_HTTP_PORT}/health
# Expected: {"status":"ok"}
```

### Step 3: Run Integration Tests

```bash
# From project root
make test-integration-{service}

# Or directly
cd test/integration/{service}
ginkgo -v
```

### Step 4: Verify Teardown

```bash
cd test/integration/{service}
./teardown-infrastructure.sh

# Verify containers are removed
podman ps -a | grep {service}
# Expected: No output
```

---

## 🔧 Troubleshooting

### Issue: PostgreSQL fails to start

**Symptoms**:
```
❌ PostgreSQL failed to start within 30 seconds
```

**Solution**:
```bash
# Check logs
podman logs {service}_postgres_1

# Common issues:
# 1. Port already in use
lsof -i :{POSTGRES_PORT}
# Kill conflicting process or change port in DD-TEST-001

# 2. Disk space full
df -h
podman system df
podman system prune -a
```

---

### Issue: DataStorage cannot connect to PostgreSQL

**Symptoms**:
```
lookup postgres on 10.89.1.1:53: no such host
```

**Solution**:
```bash
# This means PostgreSQL wasn't ready when DataStorage started
# The sequential startup pattern should prevent this

# Verify PostgreSQL is healthy BEFORE starting DataStorage
podman exec {service}_postgres_1 pg_isready -U slm_user

# If PostgreSQL is slow, increase wait time in setup script:
for i in {1..60}; do  # Increase from 30 to 60
  podman exec $POSTGRES_CONTAINER pg_isready -U slm_user && break
  sleep 1
done
```

---

### Issue: Containers get SIGKILL (exit 137)

**Symptoms**:
```
podman ps -a | grep {service}
# Shows: Exited (137) X minutes ago
```

**Solution**:
```bash
# This indicates resource exhaustion or repeated crashes

# Check Podman VM resources
podman machine inspect

# Increase resources if needed
podman machine stop
podman machine set --memory 8192 --cpus 4
podman machine start

# Check for disk quota issues
podman system df
podman system prune -a -f

# Check container logs for crash reasons
podman logs {service}_datastorage_1
```

---

### Issue: Health check timeout (30s not enough)

**Symptoms**:
```
❌ DataStorage failed to become healthy within 30 seconds
```

**Solution**:
```bash
# macOS Podman cold start can be slower
# Increase timeout in setup script:
for i in {1..60}; do  # Increase from 30 to 60
  curl -s http://127.0.0.1:$DS_HTTP_PORT/health | grep -q "ok" && break
  sleep 1
done

# Also update test suite Eventually() timeout:
Eventually(..., "60s", "1s").Should(...)  # Increase from 30s to 60s
```

---

## 📚 Reference Documents

- **DD-TEST-002**: Integration Test Container Orchestration (Authoritative decision)
- **DD-TEST-001**: Integration Test Port Allocation
- **DD-AUDIT-003**: Audit Infrastructure Requirements
- **TESTING_GUIDELINES.md**: Testing standards and patterns

---

## 🎯 Service-Specific Examples

### DataStorage (Reference Implementation)
- **Location**: `test/integration/datastorage/suite_test.go`
- **Status**: ✅ 100% tests passing (818 tests)
- **Port Allocation**: PostgreSQL 15432, Redis 16379, HTTP 18080, Metrics 18090

### Notification (Pending Migration)
- **Port Allocation**: PostgreSQL 15453, Redis 16399, HTTP 18110, Metrics 19110
- **Script Location**: `test/integration/notification/setup-infrastructure.sh`

### RemediationOrchestrator (Pending Migration)
- **Port Allocation**: PostgreSQL 15454, Redis 16400, HTTP 18111, Metrics 19111
- **Script Location**: `test/integration/remediationorchestrator/setup-infrastructure.sh`

---

## ✅ Checklist

Before marking your service as complete:

- [ ] Created `setup-infrastructure.sh` with sequential startup
- [ ] Created `teardown-infrastructure.sh` for cleanup
- [ ] Updated `suite_test.go` BeforeSuite/AfterSuite
- [ ] Added Makefile targets (`test-integration-{service}`)
- [ ] Verified health checks work (PostgreSQL, Redis, DataStorage)
- [ ] Ran full integration test suite successfully
- [ ] Documented service-specific port allocation in DD-TEST-001
- [ ] Removed any `podman-compose.yml` files (if migrating)
- [ ] Updated CI/CD pipeline to use new setup scripts

---

**Document Status**: ✅ Living Document
**Last Updated**: 2025-12-21
**Maintainer**: Infrastructure Team
**Questions**: See DD-TEST-002 or ask in #infrastructure channel

