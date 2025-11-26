# DD-TEST-001: Port Allocation Strategy for Integration & E2E Tests

**Status**: ✅ Approved
**Date**: 2025-11-26
**Author**: AI Assistant
**Reviewers**: TBD
**Related**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)

---

## Context

Integration and E2E tests require running multiple services (PostgreSQL, Redis, APIs) on the host machine using Podman containers. Without a coordinated port allocation strategy, tests experience port collisions when:

1. Multiple test suites run simultaneously (parallel execution)
2. Production services are running on default ports
3. Multiple developers run tests concurrently
4. CI/CD pipelines run multiple test jobs in parallel

**Problem Statement**: Port 8080 collision between Gateway service and Data Storage integration tests, plus potential conflicts with external PostgreSQL on port 15432.

---

## Decision

**Establish a structured port allocation strategy with dedicated port ranges for each service and test tier.**

### **Port Range Blocks**

| Service | Production | Integration Tests | E2E Tests | Reserved Range |
|---------|-----------|-------------------|-----------|----------------|
| **Gateway** | 8080 | 18080-18089 | 28080-28089 | 18080-28089 |
| **Data Storage** | 8081 | 18090-18099 | 28090-28099 | 18090-28099 |
| **Effectiveness Monitor** | 8082 | 18100-18109 | 28100-28109 | 18100-28109 |
| **Workflow Engine** | 8083 | 18110-18119 | 28110-28119 | 18110-28119 |
| **PostgreSQL** | 5432 | 15433-15442 | 25433-25442 | 15433-25442 |
| **Redis** | 6379 | 16379-16388 | 26379-26388 | 16379-26388 |
| **Embedding Service** | 8000 | 18000-18009 | 28000-28009 | 18000-28009 |

**Allocation Rules**:
- **Integration Tests**: 15433-18119 range
- **E2E Tests**: 25433-28119 range
- **Avoided Ports**: 15432 (external postgres-poc), 8080 (production Gateway)
- **Buffer**: 10 ports per service per tier (supports parallel processes + dependencies)

---

## Detailed Port Assignments

### **Data Storage Service**

#### **Integration Tests** (`test/integration/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 15433
  Container Port: 5432
  Connection: localhost:15433

Redis:
  Host Port: 16379
  Container Port: 6379
  Connection: localhost:16379

Data Storage API:
  Host Port: 18090
  Container Port: 8080
  Connection: http://localhost:18090

Embedding Service (Mock):
  Host Port: 18000
  Container Port: 8000
  Connection: http://localhost:18000
```

#### **E2E Tests** (`test/e2e/datastorage/`)
```yaml
PostgreSQL:
  Host Port: 25433
  Container Port: 5432
  Connection: localhost:25433

Redis:
  Host Port: 26379
  Container Port: 6379
  Connection: localhost:26379

Data Storage API:
  Host Port: 28090
  Container Port: 8080
  Connection: http://localhost:28090

Embedding Service:
  Host Port: 28000
  Container Port: 8000
  Connection: http://localhost:28000
```

---

### **Gateway Service**

#### **Integration Tests** (`test/integration/gateway/`)
```yaml
Redis:
  Host Port: 16380
  Container Port: 6379
  Connection: localhost:16380

Gateway API:
  Host Port: 18080
  Container Port: 8080
  Connection: http://localhost:18080

Data Storage (Dependency):
  Host Port: 18091
  Container Port: 8080
  Connection: http://localhost:18091
```

#### **E2E Tests** (`test/e2e/gateway/`)
```yaml
Redis:
  Host Port: 26380
  Container Port: 6379
  Connection: localhost:26380

Gateway API:
  Host Port: 28080
  Container Port: 8080
  Connection: http://localhost:28080

Data Storage (Dependency):
  Host Port: 28091
  Container Port: 8080
  Connection: http://localhost:28091
```

---

### **Effectiveness Monitor Service**

#### **Integration Tests** (`test/integration/effectiveness-monitor/`)
```yaml
PostgreSQL:
  Host Port: 15434
  Container Port: 5432
  Connection: localhost:15434

Effectiveness Monitor API:
  Host Port: 18100
  Container Port: 8080
  Connection: http://localhost:18100

Data Storage (Dependency):
  Host Port: 18092
  Container Port: 8080
  Connection: http://localhost:18092
```

#### **E2E Tests** (`test/e2e/effectiveness-monitor/`)
```yaml
PostgreSQL:
  Host Port: 25434
  Container Port: 5432
  Connection: localhost:25434

Effectiveness Monitor API:
  Host Port: 28100
  Container Port: 8080
  Connection: http://localhost:28100

Data Storage (Dependency):
  Host Port: 28092
  Container Port: 8080
  Connection: http://localhost:28092
```

---

### **Workflow Engine Service**

#### **Integration Tests** (`test/integration/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 18110
  Container Port: 8080
  Connection: http://localhost:18110

Data Storage (Dependency):
  Host Port: 18093
  Container Port: 8080
  Connection: http://localhost:18093
```

#### **E2E Tests** (`test/e2e/workflow-engine/`)
```yaml
Workflow Engine API:
  Host Port: 28110
  Container Port: 8080
  Connection: http://localhost:28110

Data Storage (Dependency):
  Host Port: 28093
  Container Port: 8080
  Connection: http://localhost:28093
```

---

## Rationale

### **Why Separate Port Ranges for Integration vs E2E?**
- **Parallel Execution**: Integration and E2E tests can run simultaneously without conflicts
- **Clear Separation**: Easy to identify which test tier is using which port
- **CI/CD Optimization**: Different test tiers can run in parallel pipelines

### **Why 10-Port Buffers per Service?**
- **Parallel Processes**: Ginkgo runs 4 parallel processes by default
- **Dependencies**: Services may need multiple instances (e.g., Data Storage as dependency)
- **Future Growth**: Room for additional parallel processes or test scenarios

### **Why Start at 15433 for PostgreSQL?**
- **Avoid 15432**: External postgres-poc uses this port
- **Sequential**: Easy to remember (15433, 15434, 15435...)
- **Standard Offset**: +10000 from production port (5432 → 15432 range)

### **Why Start at 18000 for Services?**
- **Above Ephemeral Range**: Avoids conflicts with OS-assigned ports (32768-60999)
- **Below Well-Known Ports**: Stays clear of common service ports
- **Memorable Pattern**: 18xxx for integration, 28xxx for E2E

---

## Consequences

### **Positive**
- ✅ **No Port Collisions**: Each test tier has dedicated, non-overlapping port ranges
- ✅ **Parallel Execution**: Multiple test suites can run simultaneously
- ✅ **Developer Friendly**: Tests don't interfere with production services
- ✅ **CI/CD Ready**: Parallel pipelines won't conflict
- ✅ **Scalable**: Room for 10 services × 2 tiers × 10 ports = 200 ports allocated
- ✅ **Predictable**: Easy to calculate port for any service/tier combination

### **Negative**
- ⚠️ **Non-Standard Ports**: Developers must remember test-specific ports
- ⚠️ **Configuration Overhead**: Each test suite needs port configuration
- ⚠️ **Documentation Burden**: Must keep port assignments up-to-date

### **Mitigation**
- 📝 **Centralized Documentation**: This DD serves as single source of truth
- 🔧 **Constants in Code**: Define ports as constants in test suites
- 📋 **Test READMEs**: Document ports in service-specific test documentation
- 🤖 **Validation Scripts**: Add pre-test port availability checks

---

## Implementation Checklist

### **Phase 1: Data Storage (Immediate)**
- [ ] Update `test/integration/datastorage/suite_test.go`
  - [ ] PostgreSQL: 5433 → 15433
  - [ ] Redis: 6379 → 16379
  - [ ] Data Storage API: 8080 → 18090
  - [ ] Embedding Service: 8000 → 18000
- [ ] Update `test/integration/datastorage/config/config.yaml`
- [ ] Update `test/integration/datastorage/config_integration_test.go`
- [ ] Update `test/e2e/datastorage/` (ports: 25433, 26379, 28090, 28000)
- [ ] Test parallel execution: `ginkgo -p -procs=4 test/integration/datastorage/`

### **Phase 2: Gateway**
- [ ] Update `test/integration/gateway/suite_test.go` (ports: 16380, 18080, 18091)
- [ ] Update `test/e2e/gateway/` (ports: 26380, 28080, 28091)

### **Phase 3: Effectiveness Monitor**
- [ ] Update `test/integration/effectiveness-monitor/` (ports: 15434, 18100, 18092)
- [ ] Update `test/e2e/effectiveness-monitor/` (ports: 25434, 28100, 28092)

### **Phase 4: Workflow Engine**
- [ ] Update `test/integration/workflow-engine/` (ports: 18110, 18093)
- [ ] Update `test/e2e/workflow-engine/` (ports: 28110, 28093)

### **Phase 5: Documentation**
- [ ] Update `test/integration/README.md` with port allocation table
- [ ] Update `test/e2e/README.md` with port allocation table
- [ ] Update `.cursor/rules/03-testing-strategy.mdc` with DD-TEST-001 reference
- [ ] Add port allocation section to each service's test README

---

## Port Collision Matrix

### **Integration Tests** (Can run simultaneously)

| Service | PostgreSQL | Redis | API | Dependencies |
|---------|-----------|-------|-----|--------------|
| **Data Storage** | 15433 | 16379 | 18090 | Embedding: 18000 |
| **Gateway** | N/A | 16380 | 18080 | Data Storage: 18091 |
| **Effectiveness Monitor** | 15434 | N/A | 18100 | Data Storage: 18092 |
| **Workflow Engine** | N/A | N/A | 18110 | Data Storage: 18093 |

✅ **No Conflicts** - All services can run integration tests in parallel

### **E2E Tests** (Can run simultaneously)

| Service | PostgreSQL | Redis | API | Dependencies |
|---------|-----------|-------|-----|--------------|
| **Data Storage** | 25433 | 26379 | 28090 | Embedding: 28000 |
| **Gateway** | N/A | 26380 | 28080 | Data Storage: 28091 |
| **Effectiveness Monitor** | 25434 | N/A | 28100 | Data Storage: 28092 |
| **Workflow Engine** | N/A | N/A | 28110 | Data Storage: 28093 |

✅ **No Conflicts** - All services can run E2E tests in parallel

---

## Example Usage

### **Running Data Storage Integration Tests**
```bash
# Ports used:
# - PostgreSQL: 15433
# - Redis: 16379
# - Data Storage API: 18090
# - Embedding Service: 18000

ginkgo -p -procs=4 test/integration/datastorage/

# Access services:
psql -h localhost -p 15433 -U postgres -d kubernaut
redis-cli -h localhost -p 16379
curl http://localhost:18090/health
```

### **Running Multiple Test Suites in Parallel**
```bash
# Terminal 1: Data Storage integration tests (ports: 15433, 16379, 18090, 18000)
ginkgo -p -procs=4 test/integration/datastorage/

# Terminal 2: Gateway integration tests (ports: 16380, 18080, 18091)
ginkgo -p -procs=4 test/integration/gateway/

# Terminal 3: Data Storage E2E tests (ports: 25433, 26379, 28090, 28000)
ginkgo -p -procs=4 test/e2e/datastorage/

# No port conflicts! ✅
```

---

## References

- **Testing Strategy**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **ADR-016**: Podman-based integration testing infrastructure
- **ADR-027**: Data Storage service containerization
- **ADR-030**: Configuration management for tests

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-11-26 | AI Assistant | Initial port allocation strategy |

---

**Authority**: This design decision is **AUTHORITATIVE** for all test port allocations.
**Scope**: All integration and E2E tests across all services.
**Enforcement**: Port allocations MUST follow this strategy to prevent conflicts.

