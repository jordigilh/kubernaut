# Workflow Catalog Integration Tests

**Business Requirement**: BR-STORAGE-013 - Semantic Search for Remediation Workflows
**Design Decisions**: DD-WORKFLOW-002, DD-STORAGE-008, DD-WORKFLOW-004, DD-TEST-001

---

## 🎯 **Overview**

Comprehensive integration tests for the Workflow Catalog toolset with real Data Storage Service, PostgreSQL, pgvector, and Embedding Service.

### **What These Tests Validate:**

1. ✅ **End-to-End Workflow Search** - Complete search flow with semantic similarity
2. ✅ **Hybrid Scoring** - Label boost/penalty calculations (DD-WORKFLOW-004)
3. ✅ **Empty Results Handling** - Graceful handling when no workflows match
4. ✅ **Filter Validation** - Mandatory and optional label filtering (DD-LLM-001)
5. ✅ **Top-K Limiting** - Result count validation
6. ✅ **Error Handling** - Service unavailable scenarios

---

## 🏗️ **Architecture**

```
┌─────────────────────────────────────────────────────────────┐
│ HolmesGPT API Workflow Catalog Toolset (Python)            │
│ - Tests in: test_workflow_catalog_data_storage_integration.py │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ HTTP POST /api/v1/workflows/search
                        │ Port 18090 (DD-TEST-001)
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ Data Storage Service (Go)                                   │
│ - Two-phase semantic search                                 │
│ - Hybrid weighted scoring                                   │
└───────────────────────┬─────────────────────────────────────┘
                        │
        ┌───────────────┴───────────────┐
        │                               │
        ▼                               ▼
┌──────────────────┐          ┌──────────────────┐
│ PostgreSQL 16    │          │ Embedding Service│
│ + pgvector       │          │ (Python)         │
│ Port: 15433      │          │ Port: 18000      │
│ 5 test workflows │          │ all-MiniLM-L6-v2 │
└──────────────────┘          └──────────────────┘
```

---

## 📋 **Prerequisites**

### **Required:**
- Docker or Podman installed and running
- Python 3.8+ with pytest
- 4GB+ RAM available for containers
- Ports available: 15433, 16380, 18000, 18090

### **Optional:**
- `curl` for manual API testing
- `psql` for database inspection

---

## 🚀 **Quick Start**

### **1. Setup Test Environment**

```bash
cd holmesgpt-api/tests/integration
./setup_workflow_catalog_integration.sh
```

This script will:
- ✅ Start PostgreSQL with pgvector extension
- ✅ Start Redis for caching
- ✅ Start Embedding Service (sentence-transformers)
- ✅ Start Data Storage Service
- ✅ Bootstrap 5 test workflows in database
- ✅ Validate all services are healthy

**Expected output:**
```
=========================================
✅ Integration Test Environment Ready
=========================================

Services:
  - PostgreSQL:           localhost:15433
  - Redis:                localhost:16380
  - Embedding Service:    http://localhost:18000
  - Data Storage Service: http://localhost:18090

Test data: 5 workflows
```

### **2. Run Integration Tests**

```bash
cd holmesgpt-api
python3 -m pytest tests/integration/test_workflow_catalog_data_storage_integration.py -v
```

**Expected output:**
```
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_semantic_search_with_exact_match_br_storage_013 PASSED
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_hybrid_scoring_with_label_boost_dd_workflow_004 PASSED
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_empty_results_handling_br_hapi_250 PASSED
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_filter_validation_dd_llm_001 PASSED
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_top_k_limiting_br_hapi_250 PASSED
tests/integration/test_workflow_catalog_data_storage_integration.py::TestWorkflowCatalogEndToEnd::test_error_handling_service_unavailable_br_storage_013 PASSED

======================== 6 passed ========================
```

### **3. Teardown Test Environment**

```bash
cd holmesgpt-api/tests/integration
./teardown_workflow_catalog_integration.sh
```

This will stop and remove all containers and volumes.

---

## 🧪 **Test Data**

The integration tests use 5 pre-configured workflows:

| Workflow ID | Signal Type | Severity | Resource Mgmt | Success Rate |
|-------------|-------------|----------|---------------|--------------|
| `oomkill-increase-memory-limits` | OOMKilled | critical | gitops (argocd) | 85% |
| `oomkill-scale-down-replicas` | OOMKilled | high | manual | 80% |
| `crashloop-fix-configuration` | CrashLoopBackOff | high | gitops (flux) | 75% |
| `node-not-ready-drain-and-reboot` | NodeNotReady | critical | manual | 90% |
| `image-pull-backoff-fix-credentials` | ImagePullBackOff | high | gitops (argocd) | 88% |

### **Test Data Schema (DD-STORAGE-008):**

```json
{
  "workflow_id": "oomkill-increase-memory-limits",
  "version": "1.0.0",
  "title": "OOMKill Remediation - Increase Memory Limits",
  "description": "Increases memory limits for pods...",
  "labels": {
    "signal-type": "OOMKilled",
    "severity": "critical",
    "resource-management": "gitops",
    "gitops-tool": "argocd",
    "environment": "production",
    "business-category": "infrastructure",
    "priority": "p0",
    "risk-tolerance": "low"
  },
  "parameters": {...},
  "steps": [...],
  "estimated_duration": "10 minutes",
  "success_rate": 0.85,
  "embedding": [768-dimensional vector],
  "enabled": true
}
```

---

## 🔧 **Manual Testing**

### **Check Service Health:**

```bash
# Data Storage Service
curl http://localhost:18090/health

# Embedding Service
curl http://localhost:18000/health
```

### **Query Database Directly:**

```bash
docker exec -it kubernaut-postgres-integration psql -U kubernaut -d kubernaut_test

# List workflows
SELECT workflow_id, version, title, labels->>'signal-type'
FROM workflow_catalog;

# Check embeddings
SELECT workflow_id, array_length(embedding, 1) as embedding_dim
FROM workflow_catalog;
```

### **Test Data Storage API:**

```bash
curl -X POST http://localhost:18090/api/v1/workflows/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "OOMKilled critical",
    "filters": {
      "signal-type": "OOMKilled",
      "severity": "critical"
    },
    "top_k": 5,
    "min_similarity": 0.7
  }' | jq
```

---

## 📊 **Port Allocation (DD-TEST-001)**

Integration tests use dedicated port ranges to avoid conflicts:

| Service | Internal Port | External Port | Purpose |
|---------|---------------|---------------|---------|
| PostgreSQL | 5432 | 15433 | Database with pgvector |
| Redis | 6379 | 16380 | Caching |
| Embedding Service | 8000 | 18000 | Embedding generation |
| Data Storage Service | 8080 | 18090 | Workflow search API |

---

## 🐛 **Troubleshooting**

### **Services Won't Start:**

```bash
# Check if ports are in use
lsof -i :15433
lsof -i :16380
lsof -i :18000
lsof -i :18090

# View container logs
docker-compose -f docker-compose.workflow-catalog.yml logs -f
```

### **Tests Fail with Connection Error:**

```bash
# Verify services are running
docker ps | grep kubernaut

# Check service health
curl http://localhost:18090/health
curl http://localhost:18000/health
```

### **No Test Data Found:**

```bash
# Check database
docker exec kubernaut-postgres-integration psql -U kubernaut -d kubernaut_test -c "SELECT COUNT(*) FROM workflow_catalog;"

# Re-run init script
docker exec kubernaut-postgres-integration psql -U kubernaut -d kubernaut_test -f /docker-entrypoint-initdb.d/init-db.sql
```

### **Embedding Generation Slow:**

First-time embedding generation can take 30-60 seconds as the model downloads and initializes. Subsequent searches use cached embeddings.

---

## 📁 **File Structure**

```
holmesgpt-api/tests/integration/
├── test_workflow_catalog_data_storage_integration.py  # Integration tests
├── docker-compose.workflow-catalog.yml                # Docker services
├── data-storage-integration.yaml                      # Data Storage config
├── init-db.sql                                        # Database schema + test data
├── setup_workflow_catalog_integration.sh              # Setup script
├── teardown_workflow_catalog_integration.sh           # Teardown script
└── WORKFLOW_CATALOG_INTEGRATION_TESTS.md             # This file
```

---

## 🎓 **Design Decisions**

- **DD-WORKFLOW-002 v2.0**: MCP Workflow Catalog Architecture
- **DD-LLM-001**: MCP Workflow Search Parameter Taxonomy
- **DD-STORAGE-008**: Workflow Catalog Schema
- **DD-WORKFLOW-004**: Hybrid Weighted Label Scoring
- **DD-TEST-001**: Port Allocation Strategy

---

## ✅ **Success Criteria**

Integration tests are considered successful when:

- ✅ All 6 tests pass
- ✅ Services start within 60 seconds
- ✅ Test data is bootstrapped (5 workflows)
- ✅ Semantic search returns relevant results
- ✅ Hybrid scoring calculations are correct
- ✅ Empty results handled gracefully
- ✅ Filters applied correctly
- ✅ Top-k limiting works
- ✅ Error handling is robust

---

## 🚀 **CI/CD Integration**

### **GitHub Actions Example:**

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: |
          cd holmesgpt-api
          pip install -r requirements.txt
          pip install -r requirements-test.txt

      - name: Setup integration environment
        run: |
          cd holmesgpt-api/tests/integration
          ./setup_workflow_catalog_integration.sh

      - name: Run integration tests
        run: |
          cd holmesgpt-api
          python3 -m pytest tests/integration/test_workflow_catalog_data_storage_integration.py -v

      - name: Teardown
        if: always()
        run: |
          cd holmesgpt-api/tests/integration
          ./teardown_workflow_catalog_integration.sh
```

---

## 📊 **Performance Benchmarks**

Expected performance (on modern laptop):

| Metric | Target | Typical |
|--------|--------|---------|
| Service startup | < 60s | ~45s |
| First search (with embedding gen) | < 10s | ~5s |
| Subsequent searches | < 2s | ~500ms |
| Test suite execution | < 30s | ~20s |

---

## 🎉 **Summary**

These integration tests provide comprehensive validation of the Workflow Catalog toolset with real services, ensuring production-ready quality and reliability.

**Confidence**: 95%
**Coverage**: 6 critical test scenarios
**Automation**: Fully automated setup/teardown

