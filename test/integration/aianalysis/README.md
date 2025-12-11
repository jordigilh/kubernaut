# AIAnalysis Integration Test Infrastructure

## Overview

AIAnalysis integration tests use a **dedicated** `podman-compose.yml` with unique ports per DD-TEST-001 to prevent collisions with other services.

## Port Allocation (DD-TEST-001)

| Service | Port | Connection String |
|---------|------|-------------------|
| **PostgreSQL** | 15434 | `localhost:15434` (user: kubernaut, db: kubernaut) |
| **Redis** | 16380 | `localhost:16380` |
| **DataStorage API** | 18091 | `http://localhost:18091` |
| **HolmesGPT API** | 18120 | `http://localhost:18120` |

## Architecture

```
AIAnalysis Integration Test Stack:
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  PostgreSQL + pgvector (:15434)                             │
│        ↓                                                    │
│  Redis (:16380)                                             │
│        ↓                                                    │
│  DataStorage API (:18091) ← Goose migrations               │
│        ↓                                                    │
│  HolmesGPT API (:18120) [MOCK_LLM_MODE=true]               │
│        ↓                                                    │
│  AIAnalysis Controller (envtest + integration tests)       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### 1. Start Infrastructure

```bash
# From project root
podman-compose -f test/integration/aianalysis/podman-compose.yml up -d --build

# Wait for health checks (or use the infrastructure helper)
# The compose file includes health checks for all services
```

### 2. Run Integration Tests

```bash
# Run all AIAnalysis integration tests
make test-integration-aianalysis

# Or run specific test files
go test -v ./test/integration/aianalysis/recovery_integration_test.go
```

### 3. Stop Infrastructure

```bash
# Stop and remove containers + volumes
podman-compose -f test/integration/aianalysis/podman-compose.yml down -v
```

## Infrastructure Helpers

The `test/infrastructure/aianalysis.go` package provides programmatic helpers:

```go
import "github.com/jordigilh/kubernaut/test/infrastructure"

// In suite_test.go BeforeSuite
err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)
if err != nil {
    return err
}

// In suite_test.go AfterSuite
infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
```

## Parallel Execution

AIAnalysis uses **unique ports** (15434, 16380, 18091, 18120), so it can run integration tests in parallel with other services:

```bash
# Safe to run simultaneously
make test-integration-datastorage &  # Uses ports 15433, 16379, 18090
make test-integration-aianalysis &   # Uses ports 15434, 16380, 18091, 18120
make test-integration-gateway &      # Uses dynamic ports 50001-60000
wait
```

## Environment Variables

The compose file sets these automatically:

| Variable | Value | Purpose |
|----------|-------|---------|
| `POSTGRES_HOST` | postgres | Container network DNS |
| `POSTGRES_PORT` | 5432 | Internal container port |
| `POSTGRES_USER` | kubernaut | Database user |
| `POSTGRES_PASSWORD` | kubernaut-test-password | Test password |
| `POSTGRES_DB` | kubernaut | Database name |
| `REDIS_ADDR` | redis:6379 | Internal Redis address |
| `MOCK_LLM_ENABLED` | true | HAPI mock mode (no real LLM calls) |
| `DATASTORAGE_URL` | http://datastorage:8080 | HAPI → DS connection |

## Troubleshooting

### Ports Already in Use

If you see port binding errors, check what's using the ports:

```bash
lsof -i :15434  # PostgreSQL
lsof -i :16380  # Redis
lsof -i :18091  # DataStorage
lsof -i :18120  # HolmesGPT API
```

### Containers Won't Start

Check logs:

```bash
podman-compose -f test/integration/aianalysis/podman-compose.yml logs
```

### Health Checks Failing

Manually verify services:

```bash
# DataStorage health
curl http://localhost:18091/health

# HolmesGPT API health
curl http://localhost:18120/health

# PostgreSQL
psql -h localhost -p 15434 -U kubernaut -d kubernaut -c "SELECT 1;"

# Redis
redis-cli -p 16380 ping
```

## References

- **DD-TEST-001**: Port allocation strategy (authoritative)
- **TESTING_GUIDELINES.md**: Testing methodology
- **NOTICE_INTEGRATION_TEST_INFRASTRUCTURE_OWNERSHIP.md**: Infrastructure ownership clarification

