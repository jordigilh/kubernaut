# AIAnalysis Integration Test Infrastructure

## Overview

AIAnalysis integration tests use a **dedicated** `podman-compose.yml` with unique ports per DD-TEST-001 to prevent collisions with other services.

## Port Allocation (DD-TEST-001)

| Service | Port | Connection String |
|---------|------|-------------------|
| **PostgreSQL** | 15438 | `localhost:15438` (user: kubernaut, db: kubernaut) |
| **Redis** | 16384 | `localhost:16384` |
| **DataStorage API** | 18095 | `http://localhost:18095` |
| **HolmesGPT API** | 18120 | `http://localhost:18120` |

## Architecture

```
AIAnalysis Integration Test Stack:
┌─────────────────────────────────────────────────────────────┐
│                                                             │
│  PostgreSQL + pgvector (:15438)                             │
│        ↓                                                    │
│  Redis (:16384)                                             │
│        ↓                                                    │
│  DataStorage API (:18095) ← Goose migrations               │
│        ↓                                                    │
│  HolmesGPT API (:18120) [MOCK_LLM_MODE=true]               │
│        ↓                                                    │
│  AIAnalysis Controller (envtest + integration tests)       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Quick Start

### Run Integration Tests (Infrastructure Auto-Started)

```bash
# From project root - infrastructure starts automatically
make test-integration-aianalysis

# Or run directly with go test
go test -v ./test/integration/aianalysis/...
```

**What happens automatically**:
1. ✅ PostgreSQL + pgvector starts (port 15438)
2. ✅ Redis starts (port 16384)
3. ✅ Data Storage API starts (port 18095)
4. ✅ HolmesGPT API starts (port 18120, MOCK_LLM_MODE=true)
5. ✅ Tests run
6. ✅ All services stop automatically after tests complete

**No manual `podman-compose` required!** The test suite manages infrastructure lifecycle automatically.

### Manual Infrastructure Control (Advanced)

If you need to start/stop services manually for debugging:

```bash
# Start infrastructure manually (optional)
podman-compose -f test/integration/aianalysis/podman-compose.yml up -d --build

# Stop infrastructure manually (optional)
podman-compose -f test/integration/aianalysis/podman-compose.yml down -v
```

## Infrastructure Helpers

The `test/infrastructure/aianalysis.go` package provides programmatic helpers that are **automatically called** in `suite_test.go`:

```go
import "github.com/jordigilh/kubernaut/test/infrastructure"

// Called automatically in SynchronizedBeforeSuite (process 1 only)
err := infrastructure.StartAIAnalysisIntegrationInfrastructure(GinkgoWriter)

// Called automatically in SynchronizedAfterSuite (last process only)
infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
```

**Result**: Infrastructure lifecycle is fully managed by the test suite following Gateway/Notification pattern.

## Parallel Execution

AIAnalysis uses **unique ports** (15438, 16384, 18095, 18120), so it can run integration tests in parallel with other services:

```bash
# Safe to run simultaneously
make test-integration-datastorage &  # Uses ports 15433, 16379, 18090
make test-integration-aianalysis &   # Uses ports 15438, 16384, 18095, 18120
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
lsof -i :15438  # PostgreSQL
lsof -i :16384  # Redis
lsof -i :18095  # DataStorage
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
curl http://localhost:18095/health

# HolmesGPT API health
curl http://localhost:18120/health

# PostgreSQL
psql -h localhost -p 15438 -U kubernaut -d kubernaut -c "SELECT 1;"

# Redis
redis-cli -p 16384 ping
```

## References

- **DD-TEST-001**: Port allocation strategy (authoritative)
- **TESTING_GUIDELINES.md**: Testing methodology
- **NOTICE_INTEGRATION_TEST_INFRASTRUCTURE_OWNERSHIP.md**: Infrastructure ownership clarification


