# HAPI Missing Integration Infrastructure

**Date**: December 24, 2025
**Team**: HAPI Service
**Priority**: 🔴 **CRITICAL** - Blocking 18/53 integration tests
**Impact**: 34% of integration tests cannot run

---

## 🚨 **ISSUE**

HAPI integration tests expect a containerized HAPI service, but it's not included in the integration test infrastructure (`docker-compose.workflow-catalog.yml`).

**Current State**: Only Data Storage infrastructure is available
**Expected State**: HAPI service should be containerized and running on port 18120

---

## 📊 **IMPACT**

### **Tests Blocked (18/53)**

| Test File | Tests | Status |
|-----------|-------|--------|
| `test_custom_labels_integration_dd_hapi_001.py` | 5 | 🔴 BLOCKED |
| `test_mock_llm_mode_integration.py` | 13 | 🔴 BLOCKED |

**Total**: 18 tests (34% of integration tests) cannot run

### **Error Message**

```
Failed: REQUIRED: HolmesGPT API not available at http://127.0.0.1:18120
  Per TESTING_GUIDELINES.md: Integration tests MUST use real services

  Start with: ./tests/integration/setup_workflow_catalog_integration.sh
```

---

## 🎯 **REQUIRED WORK**

### **Step 1: Create HAPI Dockerfile**

**File**: `holmesgpt-api/Dockerfile`

```dockerfile
FROM python:3.12-slim

WORKDIR /app

# Copy requirements
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY src/ ./src/

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD python -c "import requests; requests.get('http://localhost:8080/health')"

# Run service
CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8080"]
```

### **Step 2: Add HAPI to docker-compose**

**File**: `holmesgpt-api/tests/integration/docker-compose.workflow-catalog.yml`

Add service:

```yaml
holmesgpt-api:
  container_name: kubernaut-hapi-service-integration
  build:
    context: ../..
    dockerfile: Dockerfile
  ports:
    - "18120:8080"  # DD-TEST-001: HAPI integration port
  environment:
    - MOCK_LLM=true
    - DATA_STORAGE_URL=http://data-storage-service:8080
    - POSTGRES_HOST=postgres-integration
    - POSTGRES_PORT=5432
    - REDIS_HOST=redis-integration
    - REDIS_PORT=6379
  depends_on:
    data-storage-service:
      condition: service_healthy
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
    interval: 10s
    timeout: 5s
    retries: 5
    start_period: 30s
  networks:
    - hapi-integration
```

### **Step 3: Update setup script**

**File**: `holmesgpt-api/tests/integration/setup_workflow_catalog_integration.sh`

Add HAPI health check:

```bash
# Wait for HAPI Service
echo "⏳ Waiting for HAPI Service to be ready..."
for i in {1..60}; do
    if curl -sf http://localhost:18120/health &> /dev/null; then
        echo -e "${GREEN}✅ HAPI Service is ready${NC}"
        break
    fi
    if [ $i -eq 60 ]; then
        echo -e "${RED}❌ HAPI Service failed to start after 60 seconds${NC}"
        $COMPOSE_CMD -f "$COMPOSE_FILE" -p "$PROJECT_NAME" logs holmesgpt-api
        exit 1
    fi
    sleep 1
done
```

### **Step 4: Update conftest.py**

**File**: `holmesgpt-api/tests/integration/conftest.py`

Update `is_integration_infra_available()` to check HAPI:

```python
def is_integration_infra_available() -> bool:
    """Check if integration infrastructure is running (PostgreSQL, Redis, DS, HAPI)."""

    # Check Data Storage
    if not is_service_available(DATA_STORAGE_URL):
        return False

    # Check HAPI Service
    hapi_url = f"http://127.0.0.1:{HOLMESGPT_API_PORT}"
    if not is_service_available(hapi_url):
        return False

    return True
```

---

## 🔍 **ROOT CAUSE**

**Why This Happened**:
- HAPI integration tests were written to test the full HAPI → Data Storage flow
- Infrastructure setup only included Data Storage dependencies (PostgreSQL, Redis, DS)
- HAPI service itself was assumed to be running separately (manual startup)

**Why This Is Wrong**:
- Per DD-TEST-002: Integration tests should use sequential `podman run` or docker-compose
- All services under test should be part of the infrastructure
- Tests should be self-contained and reproducible

---

## 📋 **ACCEPTANCE CRITERIA**

✅ **Done When**:
1. HAPI Dockerfile exists and builds successfully
2. HAPI service added to `docker-compose.workflow-catalog.yml`
3. `setup_workflow_catalog_integration.sh` starts HAPI and waits for health
4. All 53 integration tests (100%) run successfully:
   ```bash
   cd holmesgpt-api
   MOCK_LLM=true python3 -m pytest tests/integration/ -v
   # Expected: 53 passed, 0 errors
   ```

---

## ⏱️ **EFFORT ESTIMATE**

| Task | Effort | Priority |
|------|--------|----------|
| Create Dockerfile | 30 min | 🔴 Critical |
| Update docker-compose | 15 min | 🔴 Critical |
| Update setup script | 15 min | 🔴 Critical |
| Update conftest.py | 10 min | 🔴 Critical |
| Test and debug | 30 min | 🔴 Critical |
| **Total** | **1.5-2 hours** | 🔴 **Critical** |

---

## 🔗 **RELATED WORK**

### **E2E Tests (Separate Task)**

For E2E tests, HAPI needs to be:
1. Built as a container image
2. Deployed to Kind cluster
3. Accessible via Kubernetes Service

**File**: `test/e2e/aianalysis/suite_test.go` (following Go E2E pattern)

---

## 📝 **SUMMARY**

**Problem**: HAPI service not containerized for integration tests
**Impact**: 18/53 integration tests (34%) cannot run
**Root Cause**: Infrastructure setup incomplete
**Solution**: Add HAPI to docker-compose.workflow-catalog.yml
**Effort**: 1.5-2 hours
**Priority**: 🔴 **CRITICAL** - Blocking V1.0 integration test completion

---

**Document Version**: 1.0
**Last Updated**: December 24, 2025
**Owner**: HAPI Team
**Status**: 🔴 **ACTION REQUIRED** - HAPI service containerization needed



