# DD-TEST-001: Port Allocation - Quick Reference Card

**Authority**: [DD-TEST-001-port-allocation-strategy.md](./DD-TEST-001-port-allocation-strategy.md)
**Version**: 1.2 (2025-12-06)

---

## 📊 Stateless Services - Port Allocation Table

| Service | Component | Integration | E2E | Production |
|---------|-----------|-------------|-----|------------|
| **Data Storage** | PostgreSQL | **15433** | **25433** | 5432 |
| | Redis | **16379** | **26379** | 6379 |
| | API | **18090** | **28090** | 8081 |
| | Embedding | **18000** | **28000** | 8000 |
| **Gateway** | Redis | **16380** | **26380** | 6379 |
| | API | **18080** | **28080** | 8080 |
| | Data Storage (dep) | **18091** | **28091** | 8081 |
| **Effectiveness Monitor** | PostgreSQL | **15434** | **25434** | 5432 |
| | API | **18100** | **28100** | 8082 |
| | Data Storage (dep) | **18092** | **28092** | 8081 |
| **Workflow Engine** | API | **18110** | **28110** | 8083 |
| | Data Storage (dep) | **18093** | **28093** | 8081 |

---

## 🎯 Kind NodePort - CRD Controllers (E2E)

| Controller | Host Port | NodePort | Metrics NodePort |
|------------|-----------|----------|------------------|
| **Gateway** | 8080 | 30080 | 30090 |
| **Data Storage** | 8081 | 30081 | 30181 |
| **Signal Processing** | 8082 | 30082 | 30182 |
| **Remediation Orchestrator** | 8083 | 30083 | 30183 |
| **AIAnalysis** | 8084 | 30084 | 30184 |
| **WorkflowExecution** | 8085 | 30085 | 30185 |
| **Notification** | 8086 | 30086 | 30186 |
| **Toolset** | 8087 | 30087 | 30187 |
| **Kubernaut Agent (KA)** | 8088 | 30088 | 30188* |

\* *Per the real `test/infrastructure/kind-kubernautagent-config.yaml`, 30188/28088 is KA's **health**
NodePort; KA's metrics NodePort is actually 30988 (host 9088) — this table's "Metrics NodePort" column
heading does not hold for the KA row. Not corrected further here (out of scope for this pass); treat
the Kind config YAML as ground truth over this table for exact port semantics.*

---

## 🤖 Kubernaut Agent (KA) E2E Dependencies (Dedicated Kind Cluster)

> **Corrected (2026-08-02, [Issue #1806](https://github.com/jordigilh/kubernaut/issues/1806))**: The
> pgvector-backed workflow catalog storage and standalone Embedding Service below were **retired**
> (`33c2053a3`, `eb5185b73` — see [DD-CONTRACT-002](DD-CONTRACT-002-service-integration-contracts.md)
> changelog). KA's real E2E Kind config (`test/infrastructure/kind-kubernautagent-config.yaml`) has no
> dedicated pgvector/embedding ports — it shares generic PostgreSQL (30439) and Redis (30387) NodePorts
> instead of the dedicated 5488/8188/6388 below. Table retained for historical reference only.

| Dependency | Host Port | NodePort | Purpose |
|------------|-----------|----------|---------|
| ~~PostgreSQL + pgvector~~ | ~~5488~~ | ~~30488~~ | ~~Workflow catalog storage~~ — retired |
| ~~Embedding Service~~ | ~~8188~~ | ~~30288~~ | ~~Vector embeddings~~ — retired |
| Data Storage | 8089 | 30089 | Audit trail, catalog API |
| ~~Redis~~ | ~~6388~~ | ~~30388~~ | ~~Data Storage DLQ~~ — superseded by shared Redis NodePort 30387 |

---

## 🎯 Port Ranges

- **Integration Tests**: 15433-18139 (Podman containers)
- **E2E Tests (Podman)**: 25433-28139
- **E2E Tests (Kind NodePort)**: 30080-30099 (API), 30180-30199 (Metrics)
- **Host Port Mapping**: 8080-8089 (Kind extraPortMappings)
- **Avoided**: 15432 (external postgres-poc), 8080 (production Gateway)

---

## 💡 Quick Lookup

### **"I'm working on Data Storage integration tests"**
```
PostgreSQL: localhost:15433
Redis: localhost:16379
API: http://localhost:18090
Embedding: http://localhost:18000
```

### **"I'm working on Data Storage E2E tests"**
```
PostgreSQL: localhost:25433
Redis: localhost:26379
API: http://localhost:28090
Embedding: http://localhost:28000
```

### **"I'm working on Kubernaut Agent (KA) E2E tests"**
```
Kubernaut Agent: http://localhost:8088
Data Storage: http://localhost:8089
PostgreSQL: localhost:30439   # shared, not a dedicated KA port
Redis: localhost:30387        # shared, not a dedicated KA port
```

### **"I'm working on Gateway integration tests"**
```
Redis: localhost:16380
API: http://localhost:18080
Data Storage: http://localhost:18091
```

---

## 🔧 Common Commands

```bash
# Check if ports are available
lsof -i :15433  # Data Storage PostgreSQL (integration)
lsof -i :18090  # Data Storage API (integration)
lsof -i :8088   # Kubernaut Agent (Kind E2E)

# Connect to test databases
psql -h localhost -p 15433 -U postgres -d kubernaut  # Integration
psql -h localhost -p 25433 -U postgres -d kubernaut  # E2E

# Connect to test Redis
redis-cli -h localhost -p 16379  # Data Storage integration
redis-cli -h localhost -p 16380  # Gateway integration

# Test API health
curl http://localhost:18090/health  # Data Storage integration
curl http://localhost:18080/health  # Gateway integration
curl http://localhost:8088/health   # Kubernaut Agent E2E (Kind)
curl http://localhost:8089/health   # Data Storage E2E (KA Kind cluster)

# Kind cluster management (KA) -- exact cluster name is set by the test setup helper
# in test/infrastructure/kubernautagent.go; check `kind get clusters` output to confirm
kind get clusters
kind delete cluster --name <ka-e2e-cluster-name-from-above>
```

---

**Full Documentation**: [DD-TEST-001-port-allocation-strategy.md](./DD-TEST-001-port-allocation-strategy.md)

