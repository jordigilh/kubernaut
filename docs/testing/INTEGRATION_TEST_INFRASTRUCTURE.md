# Integration Test Infrastructure

**Per ADR-016**: Service-Specific Integration Test Infrastructure
**Last Updated**: October 12, 2025

---

## 🚀 Quick Start

### Run All Integration Tests
```bash
make test-integration-service-all
```

**Duration**: ~6-10 minutes (sequential execution)

### Run Service-Specific Tests

#### Data Storage (Podman: PostgreSQL + pgvector) - ~30 seconds
```bash
make test-integration-datastorage
```

**Requirements**: Podman, port 5432 available

#### AI Service (Podman: Redis) - ~15 seconds
```bash
make test-integration-ai
```

**Requirements**: Podman, port 6379 available

#### Dynamic Toolset (Kind: Kubernetes) - ~3-5 minutes
```bash
make test-integration-toolset
```

**Requirements**: Kind, kubectl, Kubernetes cluster

#### Gateway Service (Kind: Kubernetes) - ~3-5 minutes
```bash
make test-integration-gateway-service
```

**Requirements**: Kind, kubectl, Kubernetes cluster

---

## 📊 Infrastructure Overview

### Service Classification

| Service | Infrastructure | Dependencies | Startup Time | Port(s) | Rationale |
|---------|----------------|--------------|--------------|---------|-----------|
| **Data Storage** | Podman | PostgreSQL + pgvector | ~15 sec | 5432 | No Kubernetes features needed |
| **AI Service** | Podman | Redis | ~5 sec | 6379 | No Kubernetes features needed |
| **Dynamic Toolset** | Kind | Kubernetes cluster | ~2-5 min | N/A | Service discovery, RBAC |
| **Gateway** | Kind | Kubernetes cluster | ~2-5 min | N/A | RBAC, TokenReview API |

### Container Images

**Podman-Based Services**:
- PostgreSQL: `pgvector/pgvector:pg15` (includes pgvector extension)
- Redis: `redis:7-alpine`

**Kind-Based Services**:
- Kind cluster: `kubernaut-integration` (default name)
- Uses local Kubernetes resources

---

## 📚 Detailed Usage

### Data Storage Service

**What It Tests**:
- PostgreSQL persistence with vector embeddings
- Schema initialization and migrations
- Dual-write transaction coordination
- Validation and sanitization pipelines
- Cross-service concurrent writes
- Context cancellation (KNOWN_ISSUE_001)

**Infrastructure**:
```bash
# Automatic via Makefile:
make test-integration-datastorage

# Manual (for debugging):
podman run -d --name datastorage-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres \
  pgvector/pgvector:pg15

sleep 5
podman exec datastorage-postgres pg_isready -U postgres

go test ./test/integration/datastorage/... -v -timeout 5m

podman stop datastorage-postgres
podman rm datastorage-postgres
```

**Expected Output**:
- 29 test scenarios
- ~11-15 scenarios passing (some test data issues expected in current version)
- 3 scenarios skipped (KNOWN_ISSUE_001 context cancellation)
- Total time: ~30 seconds

### AI Service

**What It Tests**:
- Redis cache integration
- Embedding cache hit/miss behavior
- TTL expiration
- Concurrent cache access

**Infrastructure**:
```bash
# Automatic via Makefile:
make test-integration-ai

# Manual (for debugging):
podman run -d --name ai-redis -p 6379:6379 redis:7-alpine

sleep 2

go test ./test/integration/ai/... -v -timeout 5m

podman stop ai-redis
podman rm ai-redis
```

**Expected Output**:
- Cache operations validate correctly
- Total time: ~15 seconds

### Dynamic Toolset Service

**What It Tests**:
- Kubernetes service discovery
- RBAC permissions
- ConfigMap reconciliation
- Cross-namespace service detection

**Infrastructure**:
```bash
# Automatic via Makefile:
make test-integration-toolset

# Manual (for debugging):
./scripts/ensure-kind-cluster.sh

go test ./test/integration/toolset/... -v -timeout 10m
```

**Expected Output**:
- Service discovery tests pass
- RBAC tests validate permissions
- Total time: ~3-5 minutes (including Kind startup if needed)

### Gateway Service

**What It Tests**:
- Kubernetes TokenReview API
- RBAC enforcement
- Storm detection and aggregation
- Rate limiting

**Infrastructure**:
```bash
# Automatic via Makefile:
make test-integration-gateway-service

# Uses existing test-gateway target
```

**Expected Output**:
- Authentication tests pass
- Storm detection validates
- Total time: ~3-5 minutes

---

## 🛠️ Troubleshooting

### Port Conflicts

#### PostgreSQL (Port 5432)
```bash
# Check what's using the port
lsof -i :5432

# Stop existing PostgreSQL containers
podman stop datastorage-postgres
podman rm datastorage-postgres

# If system PostgreSQL is running:
brew services stop postgresql  # macOS
sudo systemctl stop postgresql  # Linux
```

#### Redis (Port 6379)
```bash
# Check what's using the port
lsof -i :6379

# Stop existing Redis containers
podman stop ai-redis
podman rm ai-redis

# If system Redis is running:
brew services stop redis  # macOS
sudo systemctl stop redis  # Linux
```

### Container Cleanup

```bash
# List all containers
podman ps -a

# Stop all integration test containers
podman stop datastorage-postgres ai-redis 2>/dev/null || true
podman rm datastorage-postgres ai-redis 2>/dev/null || true

# Remove all stopped containers
podman container prune -f

# Remove all unused images (optional)
podman image prune -f
```

### Kind Cluster Issues

```bash
# List all Kind clusters
kind get clusters

# Delete and recreate integration cluster
kind delete cluster --name kubernaut-integration
./scripts/ensure-kind-cluster.sh

# Check cluster accessibility
kubectl cluster-info --context kind-kubernaut-integration

# Switch to Kind cluster context
kubectl config use-context kind-kubernaut-integration
```

### Podman Not Responding

```bash
# Restart Podman machine (macOS/Windows)
podman machine stop
podman machine start

# Check Podman status
podman info

# Reset Podman (nuclear option, removes all containers/images)
podman system reset
```

### Test Failures

#### Data Storage: pgvector Extension Missing
```
Error: extension "vector" is not available
```

**Solution**: Ensure you're using `pgvector/pgvector:pg15` image, not plain `postgres:15`

```bash
podman rm datastorage-postgres
make test-integration-datastorage  # Will pull correct image
```

#### Data Storage: Embedding Dimension Mismatch
```
Error: embedding dimension must be 384, got 3
```

**Solution**: This is a known test data issue (will be fixed in Day 8). Test infrastructure is working correctly.

#### Kind: Cluster Not Accessible
```
Error: unable to connect to cluster
```

**Solution**:
```bash
kind delete cluster --name kubernaut-integration
./scripts/ensure-kind-cluster.sh
```

---

## 🎯 Performance Expectations

### Target Times

| Service | Target Time | Actual Time | Status |
|---------|-------------|-------------|--------|
| **Data Storage** | <1 min | ~30 sec | ✅ Exceeded |
| **AI Service** | <30 sec | ~15 sec | ✅ Exceeded |
| **Dynamic Toolset** | <5 min | ~3-5 min | ✅ Met |
| **Gateway** | <5 min | ~3-5 min | ✅ Met |
| **All Services** | <10 min | ~6-10 min | ✅ Met |

### Resource Usage

| Service | Memory | CPU | Disk |
|---------|--------|-----|------|
| **Data Storage (Podman)** | 512 MB | 0.5-1 core | 1-2 GB |
| **AI Service (Podman)** | 256 MB | 0.2-0.5 core | 500 MB |
| **Dynamic Toolset (Kind)** | 2-4 GB | 1-2 cores | 5-10 GB |
| **Gateway (Kind)** | 2-4 GB | 1-2 cores | 5-10 GB |

---

## 🔐 Security Notes

### Credentials

**PostgreSQL**:
- User: `postgres`
- Password: `postgres`
- Database: `postgres`
- ⚠️ These are test-only credentials, never use in production

**Redis**:
- No authentication (test-only)
- Bound to localhost only

### Network Isolation

All Podman containers bind to `localhost` only and are not accessible from external networks.

---

## 🤝 CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  test-datastorage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - name: Install Podman
        run: |
          sudo apt-get update
          sudo apt-get install -y podman
      - name: Run Data Storage Tests
        run: make test-integration-datastorage

  test-ai:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - name: Install Podman
        run: |
          sudo apt-get update
          sudo apt-get install -y podman
      - name: Run AI Service Tests
        run: make test-integration-ai

  test-kind-services:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - name: Install Kind
        run: |
          curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.30.0/kind-linux-amd64
          chmod +x ./kind
          sudo mv ./kind /usr/local/bin/kind
      - name: Run Dynamic Toolset Tests
        run: make test-integration-toolset
      - name: Run Gateway Tests
        run: make test-integration-gateway-service
```

### Parallel Execution

Podman services can run in parallel (different ports):
```bash
# Terminal 1
make test-integration-datastorage

# Terminal 2 (simultaneously)
make test-integration-ai
```

---

## 📖 Related Documentation

- [ADR-016: Service-Specific Integration Test Infrastructure](../architecture/decisions/ADR-016-SERVICE-SPECIFIC-INTEGRATION-TEST-INFRASTRUCTURE.md) - Primary decision document
- [ADR-003: Kind Cluster as Primary Integration Environment](../architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md) - Original Kind decision (partially superseded)
- [Data Storage Implementation Plan](../services/stateless/data-storage/implementation/IMPLEMENTATION_PLAN_V4.1.md) - Data Storage service details
- [Dynamic Toolset Testing Strategy](../services/stateless/dynamic-toolset/testing-strategy.md) - Dynamic Toolset service details

---

## 💡 Tips & Best Practices

### 1. Fast Iteration During Development

For fastest TDD cycle, run only the service you're working on:
```bash
# Developing Data Storage? Run only that:
make test-integration-datastorage  # 30 seconds

# Don't run all services unless necessary:
# make test-integration-service-all  # 6-10 minutes
```

### 2. Keep Containers Running for Multiple Test Runs

```bash
# Start PostgreSQL manually
podman run -d --name datastorage-postgres -p 5432:5432 \
  -e POSTGRES_PASSWORD=postgres \
  pgvector/pgvector:pg15

# Run tests multiple times (no startup overhead)
go test ./test/integration/datastorage/... -v
go test ./test/integration/datastorage/... -v  # Instant startup!

# Cleanup when done
podman stop datastorage-postgres && podman rm datastorage-postgres
```

### 3. Debug Individual Test Scenarios

```bash
# Run specific test file
go test ./test/integration/datastorage/basic_persistence_test.go -v

# Run specific test scenario (Ginkgo)
go test ./test/integration/datastorage/... -v -ginkgo.focus="should write remediation audit"
```

### 4. Verbose Logging

```bash
# Enable verbose test output
make test-integration-datastorage | tee test-output.log

# Check logs from PostgreSQL
podman logs datastorage-postgres
```

---

## 🔧 Troubleshooting

### Podman "proxy already running" Error

**Symptom**:
```
❌ Failed to start PostgreSQL: Error: "proxy already running"
```

Or:
```
⚠️  Ports 15433 or 16379 may be in use:
gvproxy 30754 jgil   16u  IPv6  TCP *:16379 (LISTEN)
```

**Root Cause**: On macOS, Podman uses `gvproxy` to forward ports from containers to the host. When a test is interrupted (Ctrl+C, crash, timeout), containers may be removed but `gvproxy` keeps the port bindings, causing conflicts on the next run.

**Solutions**:

#### Option 1: Use Stale Container Cleanup Target (Recommended)

The Makefile includes a safe cleanup target that only removes stale containers (not running ones):

```bash
make clean-stale-datastorage-containers
```

This is automatically called by `test-integration-datastorage`. It's safe for parallel test runs because it only removes containers that exist but aren't running.

#### Option 2: Restart Podman Machine (Heavy-handed)

If cleanup doesn't work, restart the Podman VM:

```bash
podman machine stop && podman machine start
```

This takes ~30 seconds but guarantees a clean state. **Warning**: This will stop ALL running containers.

#### Option 3: Kill Specific Port (Last Resort)

Only use if you're certain no other tests are using the port:

```bash
# Find and kill process holding port
lsof -ti:5432 | xargs kill -9
```

**Warning**: This may break parallel test runs from other services!

### Container Already Exists

**Symptom**:
```
⚠️  PostgreSQL container already exists or failed to start
```

**Solution**: The Makefile handles this automatically by attempting to start the existing container. If issues persist, run:

```bash
podman rm -f datastorage-postgres
make test-integration-datastorage
```

### Port Already in Use by Another Service

**Symptom**: Port conflict when running multiple services' integration tests in parallel.

**Solution**: Each service should use unique ports. Standard port assignments:

| Service | PostgreSQL | Redis | Notes |
|---------|------------|-------|-------|
| Data Storage | 5432 | - | Default ports |
| AI Service | - | 6379 | Default ports |
| Suite Tests | 15433 | 16379 | Offset ports for isolation |

If you need to run the same service tests in parallel, use different port offsets via environment variables.

---

**Last Updated**: December 10, 2025
**Maintained By**: Kubernaut Team
**Decision Document**: ADR-016


