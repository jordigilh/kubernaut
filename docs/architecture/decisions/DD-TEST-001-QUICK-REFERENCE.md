# DD-TEST-001: Port Allocation - Quick Reference Card

**Authority**: [DD-TEST-001-port-allocation-strategy.md](./DD-TEST-001-port-allocation-strategy.md)

---

## 📊 Port Allocation Table

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

## 🎯 Port Ranges

- **Integration Tests**: 15433-18119
- **E2E Tests**: 25433-28119
- **Avoided**: 15432 (external postgres-poc), 8080 (production)

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

# Connect to test databases
psql -h localhost -p 15433 -U postgres -d kubernaut  # Integration
psql -h localhost -p 25433 -U postgres -d kubernaut  # E2E

# Connect to test Redis
redis-cli -h localhost -p 16379  # Data Storage integration
redis-cli -h localhost -p 16380  # Gateway integration

# Test API health
curl http://localhost:18090/health  # Data Storage integration
curl http://localhost:18080/health  # Gateway integration
```

---

**Full Documentation**: [DD-TEST-001-port-allocation-strategy.md](./DD-TEST-001-port-allocation-strategy.md)

