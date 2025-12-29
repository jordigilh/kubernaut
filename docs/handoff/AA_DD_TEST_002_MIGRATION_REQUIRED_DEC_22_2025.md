# AIAnalysis DD-TEST-002 Migration Required - CRITICAL

**Date**: December 22, 2025
**Severity**: 🚨 **CRITICAL - DD-TEST-002 VIOLATION**
**Impact**: Using deprecated podman-compose for multi-service dependencies
**Authority**: DD-TEST-002 (Integration Test Container Orchestration)

---

## 🚨 **CRITICAL VIOLATION**

AIAnalysis integration tests are using **`podman-compose`** for **multi-service dependencies**, which **VIOLATES DD-TEST-002** authoritative guidance.

**Per DD-TEST-002 Lines 79-83**:
```markdown
| **Multi-service with startup dependencies** | ✅ **Sequential `podman run`** | Eliminates race conditions |
```

**Per DD-TEST-002 Lines 245-250**:
```markdown
### ❌ Never Use `podman-compose` For:
- **Multi-service integration tests** with startup dependencies
- **CI/CD pipelines** where reliability is critical
- **Automated test suites** where failures block deployments
```

---

## 📋 **AIAnalysis Has Multi-Service Dependencies**

**Current Infrastructure** (`test/integration/aianalysis/podman-compose.yml`):

1. **PostgreSQL** (port 15434) - DataStorage backend
2. **Redis** (port 16380) - DataStorage DLQ
3. **DataStorage** (port 18091) - Audit events storage
4. **HolmesGPT API** (port 18120) - AI analysis capabilities
5. **Migrations** - Database schema setup

**Startup Dependency Chain**:
```
PostgreSQL → Migrations → Redis → DataStorage → HolmesGPT API
```

**Result**: AIAnalysis has **SAME dependency pattern** as:
- DataStorage (✅ migrated Dec 20, 2025)
- Gateway (✅ migrated Dec 22, 2025)
- WorkflowExecution (✅ migrated Dec 21, 2025)
- Notification (✅ migrated Dec 21, 2025)
- RemediationOrchestrator (✅ migrated)
- SignalProcessing (✅ migrated)

**Conclusion**: AIAnalysis **MUST** migrate to DD-TEST-002 sequential startup pattern.

---

## 🚨 **DD-TEST-002 Service Migration Status - INCORRECT**

### **DD-TEST-002 Lines 294-304** (OUTDATED):

```markdown
| **AIAnalysis** | N/A | - | Uses mocked dependencies |
```

**This is WRONG**:
- ❌ AIAnalysis does NOT use "mocked dependencies"
- ❌ AIAnalysis has **4 real services** (PostgreSQL + Redis + DataStorage + HolmesGPT API)
- ❌ AIAnalysis is using **podman-compose** (deprecated for multi-service)

### **CORRECT Status**:

```markdown
| **AIAnalysis** | ⚠️ **VIOLATION** | - | Using deprecated podman-compose for multi-service dependencies (MUST migrate to sequential startup) |
```

---

## 📊 **Problems with Current podman-compose Approach**

### **Problem 1: Race Conditions**

Per DD-TEST-002, `podman-compose up -d` starts all services **simultaneously**:

```bash
podman-compose up -d
  ├── PostgreSQL starts ⏱️ Takes 10-15 seconds to be ready
  ├── Redis starts ⏱️ Takes 2-3 seconds to be ready
  ├── DataStorage starts ⚡ Tries to connect IMMEDIATELY
  │   ↓
  │   ❌ Connection fails (PostgreSQL not ready yet)
  │   ↓
  │   🔄 Container crashes and restarts repeatedly
  └── HolmesGPT API starts ⚡ Tries to connect to DataStorage
      ↓
      ❌ DataStorage not ready yet
      ↓
      🔄 Container crashes
```

**Evidence**: Other services experienced **Exit 137 (SIGKILL)** failures before migration:
- RemediationOrchestrator (Dec 20, 2025)
- Notification (Dec 21, 2025)
- WorkflowExecution (Dec 21, 2025)

**AIAnalysis Risk**: Same pattern, same failures likely occurring intermittently.

---

### **Problem 2: Outdated Ports**

`podman-compose.yml` has **WRONG PORTS** that conflict with DD-TEST-001 v1.6:

| Component | podman-compose.yml | Correct (DD-TEST-001) | Conflict |
|-----------|-------------------|----------------------|----------|
| PostgreSQL | **15434** | 15438 | ⚠️ EffectivenessMonitor |
| Redis | **16380** | 16384 | ⚠️ Freed Gateway port |
| DataStorage | **18091** | 18095 | 🚨 **Gateway conflict!** |
| HolmesGPT API | 18120 | 18120 | ✅ Correct |

**Impact**: Cannot run AIAnalysis + Gateway integration tests in parallel (port 18091 conflict).

---

### **Problem 3: Violates Authoritative Guidance**

**DD-TEST-002 is AUTHORITATIVE** (Line 326):
```markdown
**Document Status**: ✅ Authoritative
**Approved By**: DataStorage Team (validated), Infrastructure Team (approved)
```

**AIAnalysis violates this by**:
- Using `podman-compose` for multi-service dependencies
- Ignoring proven sequential startup pattern (6 other services migrated)
- Risking race conditions in CI/CD

---

## ✅ **REQUIRED: Migrate to DD-TEST-002 Sequential Startup**

### **Action Required**

**DEPRECATE**: `test/integration/aianalysis/podman-compose.yml`
**CREATE**: `test/integration/aianalysis/setup-infrastructure.sh`

---

### **Implementation Template**

**File**: `test/integration/aianalysis/setup-infrastructure.sh`

```bash
#!/bin/bash
# AIAnalysis Integration Test Infrastructure Setup
# Per DD-TEST-002: Sequential Container Orchestration Pattern
# Date: December 22, 2025

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Container and network names
POSTGRES_CONTAINER="aianalysis_postgres_1"
REDIS_CONTAINER="aianalysis_redis_1"
DATASTORAGE_CONTAINER="aianalysis_datastorage_1"
HOLMESGPT_CONTAINER="aianalysis_holmesgpt_1"
MIGRATIONS_CONTAINER="aianalysis_migrations"
NETWORK_NAME="aianalysis_test-network"

# Ports (per DD-TEST-001 v1.6 - December 2025)
POSTGRES_PORT="15438"      # DD-TEST-001 sequential pattern
REDIS_PORT="16384"         # DD-TEST-001 sequential pattern
DATASTORAGE_HTTP_PORT="18095"  # DD-TEST-001 sequential pattern (was 18091, conflicted with Gateway)
DATASTORAGE_METRICS_PORT="19095"  # DD-TEST-001 metrics pattern
HOLMESGPT_PORT="18120"     # HolmesGPT API port (correct)

# Database configuration
DB_NAME="action_history"
DB_USER="slm_user"
DB_PASSWORD="test_password"

echo -e "${BLUE}🚀 AIAnalysis Integration Test Infrastructure Setup${NC}"
echo -e "${BLUE}   Per DD-TEST-002: Sequential Startup Pattern${NC}"
echo ""

# ========================================
# STEP 1: Cleanup existing containers
# ========================================
echo -e "${YELLOW}🧹 Step 1: Cleaning up existing containers...${NC}"

podman stop $POSTGRES_CONTAINER 2>/dev/null || true
podman rm $POSTGRES_CONTAINER 2>/dev/null || true
echo "  ✓ PostgreSQL container cleaned"

podman stop $REDIS_CONTAINER 2>/dev/null || true
podman rm $REDIS_CONTAINER 2>/dev/null || true
echo "  ✓ Redis container cleaned"

podman stop $DATASTORAGE_CONTAINER 2>/dev/null || true
podman rm $DATASTORAGE_CONTAINER 2>/dev/null || true
echo "  ✓ DataStorage container cleaned"

podman stop $HOLMESGPT_CONTAINER 2>/dev/null || true
podman rm $HOLMESGPT_CONTAINER 2>/dev/null || true
echo "  ✓ HolmesGPT API container cleaned"

podman stop $MIGRATIONS_CONTAINER 2>/dev/null || true
podman rm $MIGRATIONS_CONTAINER 2>/dev/null || true
echo "  ✓ Migrations container cleaned"

echo -e "${GREEN}✅ Cleanup complete${NC}"
echo ""

# ========================================
# STEP 2: Create network
# ========================================
echo -e "${YELLOW}🌐 Step 2: Creating network...${NC}"

if ! podman network exists $NETWORK_NAME 2>/dev/null; then
  podman network create $NETWORK_NAME
  echo "  ✓ Network '$NETWORK_NAME' created"
else
  echo "  ✓ Network '$NETWORK_NAME' already exists (reusing)"
fi

echo -e "${GREEN}✅ Network ready${NC}"
echo ""

# ========================================
# STEP 3: Start PostgreSQL
# ========================================
echo -e "${YELLOW}🔵 Step 3: Starting PostgreSQL...${NC}"

podman run -d \
  --name $POSTGRES_CONTAINER \
  --network $NETWORK_NAME \
  -p ${POSTGRES_PORT}:5432 \
  -e POSTGRES_USER=$DB_USER \
  -e POSTGRES_PASSWORD=$DB_PASSWORD \
  -e POSTGRES_DB=$DB_NAME \
  postgres:16-alpine

echo "  ✓ PostgreSQL container started"

# Wait for PostgreSQL to be ready
echo "  ⏳ Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
  if podman exec $POSTGRES_CONTAINER pg_isready -U $DB_USER -d $DB_NAME > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓ PostgreSQL is ready (attempt $i/30)${NC}"
    break
  fi
  if [ $i -eq 30 ]; then
    echo -e "${RED}❌ PostgreSQL failed to start after 30 seconds${NC}"
    exit 1
  fi
  sleep 1
done

echo -e "${GREEN}✅ PostgreSQL ready${NC}"
echo ""

# ========================================
# STEP 4: Run migrations
# ========================================
echo -e "${YELLOW}🔄 Step 4: Running migrations...${NC}"

WORKSPACE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"

podman run --rm \
  --name $MIGRATIONS_CONTAINER \
  --network $NETWORK_NAME \
  -v ${WORKSPACE_ROOT}/migrations:/migrations:ro \
  -e PGHOST=$POSTGRES_CONTAINER \
  -e PGPORT=5432 \
  -e PGUSER=$DB_USER \
  -e PGPASSWORD=$DB_PASSWORD \
  -e PGDATABASE=$DB_NAME \
  postgres:16-alpine \
  bash -c '
    set -e
    echo "Waiting for PostgreSQL..."
    until pg_isready -h $PGHOST -U $PGUSER; do sleep 1; done
    echo "Applying migrations (Up sections only)..."
    find /migrations -maxdepth 1 -name "*.sql" -type f | sort | while read f; do
      echo "Applying $f..."
      sed -n "1,/^-- +goose Down/p" "$f" | grep -v "^-- +goose Down" | psql
    done
    echo "Migrations complete!"
  '

echo -e "${GREEN}✅ Migrations applied${NC}"
echo ""

# ========================================
# STEP 5: Start Redis
# ========================================
echo -e "${YELLOW}🔵 Step 5: Starting Redis...${NC}"

podman run -d \
  --name $REDIS_CONTAINER \
  --network $NETWORK_NAME \
  -p ${REDIS_PORT}:6379 \
  redis:7-alpine

echo "  ✓ Redis container started"

# Wait for Redis to be ready
echo "  ⏳ Waiting for Redis to be ready..."
for i in {1..10}; do
  if podman exec $REDIS_CONTAINER redis-cli ping 2>/dev/null | grep -q PONG; then
    echo -e "  ${GREEN}✓ Redis is ready (attempt $i/10)${NC}"
    break
  fi
  if [ $i -eq 10 ]; then
    echo -e "${RED}❌ Redis failed to start after 10 seconds${NC}"
    exit 1
  fi
  sleep 1
done

echo -e "${GREEN}✅ Redis ready${NC}"
echo ""

# ========================================
# STEP 6: Start DataStorage
# ========================================
echo -e "${YELLOW}🔵 Step 6: Starting DataStorage...${NC}"

# Build DataStorage image if needed
if ! podman image exists localhost/kubernaut-datastorage:latest; then
  echo "  🔨 Building DataStorage image..."
  podman build -t localhost/kubernaut-datastorage:latest -f ${WORKSPACE_ROOT}/docker/data-storage.Dockerfile ${WORKSPACE_ROOT}
fi

# Start DataStorage container
podman run -d \
  --name $DATASTORAGE_CONTAINER \
  --network $NETWORK_NAME \
  -p ${DATASTORAGE_HTTP_PORT}:8080 \
  -p ${DATASTORAGE_METRICS_PORT}:9090 \
  -v ${WORKSPACE_ROOT}/test/integration/aianalysis/config:/etc/datastorage:ro \
  -e CONFIG_PATH=/etc/datastorage/config.yaml \
  -e POSTGRES_HOST=$POSTGRES_CONTAINER \
  -e POSTGRES_PORT=5432 \
  -e REDIS_ADDR=${REDIS_CONTAINER}:6379 \
  localhost/kubernaut-datastorage:latest

echo "  ✓ DataStorage container started"

# Wait for DataStorage health check
echo "  ⏳ Waiting for DataStorage to be healthy..."
for i in {1..30}; do
  if curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health > /dev/null 2>&1; then
    HEALTH_STATUS=$(curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health | grep -o '"status":"[^"]*"' || true)
    if [ -n "$HEALTH_STATUS" ]; then
      echo -e "  ${GREEN}✓ DataStorage is healthy (attempt $i/30): $HEALTH_STATUS${NC}"
      break
    fi
  fi
  if [ $i -eq 30 ]; then
    echo -e "${RED}❌ DataStorage failed to become healthy after 30 seconds${NC}"
    echo "  Checking logs:"
    podman logs $DATASTORAGE_CONTAINER
    exit 1
  fi
  sleep 1
done

echo -e "${GREEN}✅ DataStorage ready${NC}"
echo ""

# ========================================
# STEP 7: Start HolmesGPT API
# ========================================
echo -e "${YELLOW}🔵 Step 7: Starting HolmesGPT API...${NC}"

# Build HolmesGPT API image if needed
if ! podman image exists localhost/kubernaut-holmesgpt-api:latest; then
  echo "  🔨 Building HolmesGPT API image..."
  podman build -t localhost/kubernaut-holmesgpt-api:latest -f ${WORKSPACE_ROOT}/holmesgpt-api/Dockerfile ${WORKSPACE_ROOT}
fi

# Start HolmesGPT API container
podman run -d \
  --name $HOLMESGPT_CONTAINER \
  --network $NETWORK_NAME \
  -p ${HOLMESGPT_PORT}:8080 \
  -e MOCK_LLM_MODE=true \
  -e DATASTORAGE_URL=http://${DATASTORAGE_CONTAINER}:8080 \
  localhost/kubernaut-holmesgpt-api:latest

echo "  ✓ HolmesGPT API container started"

# Wait for HolmesGPT API health check
echo "  ⏳ Waiting for HolmesGPT API to be healthy..."
for i in {1..30}; do
  if curl -s http://127.0.0.1:${HOLMESGPT_PORT}/health > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓ HolmesGPT API is healthy (attempt $i/30)${NC}"
    break
  fi
  if [ $i -eq 30 ]; then
    echo -e "${RED}❌ HolmesGPT API failed to become healthy after 30 seconds${NC}"
    echo "  Checking logs:"
    podman logs $HOLMESGPT_CONTAINER
    exit 1
  fi
  sleep 1
done

echo -e "${GREEN}✅ HolmesGPT API ready${NC}"
echo ""

# ========================================
# STEP 8: Final verification
# ========================================
echo -e "${YELLOW}🔍 Step 8: Final verification...${NC}"

HEALTH_RESPONSE=$(curl -s http://127.0.0.1:${DATASTORAGE_HTTP_PORT}/health)
HAPI_RESPONSE=$(curl -s http://127.0.0.1:${HOLMESGPT_PORT}/health)

echo "  DataStorage Health: $HEALTH_RESPONSE"
echo "  HolmesGPT API Health: $HAPI_RESPONSE"

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ AIAnalysis Integration Test Infrastructure Ready!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "Service Endpoints:"
echo "  • PostgreSQL:        127.0.0.1:$POSTGRES_PORT"
echo "  • Redis:             127.0.0.1:$REDIS_PORT"
echo "  • DataStorage:       http://127.0.0.1:$DATASTORAGE_HTTP_PORT (health: /health)"
echo "  • DataStorage Metrics: http://127.0.0.1:$DATASTORAGE_METRICS_PORT"
echo "  • HolmesGPT API:     http://127.0.0.1:$HOLMESGPT_PORT (health: /health)"
echo ""
echo "To stop infrastructure:"
echo "  podman stop $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER $HOLMESGPT_CONTAINER"
echo "  podman rm $POSTGRES_CONTAINER $REDIS_CONTAINER $DATASTORAGE_CONTAINER $HOLMESGPT_CONTAINER"
echo ""
```

---

## 📋 **Migration Checklist**

### **Phase 1: Create Sequential Startup Script** (1 hour)

- [ ] Create `test/integration/aianalysis/setup-infrastructure.sh`
- [ ] Use DD-TEST-001 v1.6 ports (15438/16384/18095/19095/18120)
- [ ] Follow DD-TEST-002 sequential pattern
- [ ] Add explicit health checks for each service
- [ ] Test: `./setup-infrastructure.sh && curl http://localhost:18095/health`

---

### **Phase 2: Update Test Suite** (30 minutes)

- [ ] Update `test/integration/aianalysis/suite_test.go`
  - [ ] Change `BeforeSuite` to call `setup-infrastructure.sh` (not podman-compose)
  - [ ] Update port references (18091→18095, 15434→15438, 16380→16384)
  - [ ] Use `Eventually()` for health checks (30s timeout)

- [ ] Update `test/integration/aianalysis/config/config.yaml`
  - [ ] Verify `database.host: aianalysis_postgres_1` (container name)
  - [ ] Verify `redis.addr: aianalysis_redis_1:6379` (container name)

---

### **Phase 3: Deprecate podman-compose** (10 minutes)

- [ ] Rename `podman-compose.yml` → `podman-compose.yml.deprecated`
- [ ] Add deprecation notice to file header
- [ ] Update any documentation referencing podman-compose

---

### **Phase 4: Update DD-TEST-002** (10 minutes)

- [ ] Update service migration status table (Line 296):
  ```markdown
  | **AIAnalysis** | ✅ Migrated | 2025-12-22 | Sequential startup (PostgreSQL + Redis + DataStorage + HolmesGPT API), ports aligned to DD-TEST-001 v1.6 |
  ```

---

### **Phase 5: Validate** (20 minutes)

- [ ] Run `./setup-infrastructure.sh` - all services start cleanly
- [ ] Run AIAnalysis integration tests - all pass
- [ ] Test parallel execution with Gateway - no port conflicts
- [ ] Verify port validation shows no duplicates

---

## ✅ **Benefits of Migration**

1. ✅ **Eliminates race conditions** - Sequential startup ensures dependencies are ready
2. ✅ **DD-TEST-002 compliance** - Follows authoritative guidance
3. ✅ **Correct ports** - Aligns with DD-TEST-001 v1.6 (resolves Gateway conflict)
4. ✅ **Parallel testing** - Can run with all other services simultaneously
5. ✅ **Consistent pattern** - Matches 6 other services already migrated
6. ✅ **Better error messages** - Know exactly which service failed to start
7. ✅ **CI/CD reliability** - Deterministic startup behavior

---

## 🚨 **Priority**

**Severity**: 🚨 **HIGH** (DD-TEST-002 violation)
**Urgency**: **IMMEDIATE** (blocks parallel testing, violates authoritative guidance)
**Effort**: **MEDIUM** (2 hours total)

**Blocking**:
- Parallel testing with Gateway (port 18091 conflict)
- DD-TEST-002 compliance for all services
- Future EffectivenessMonitor integration tests (port 15434 conflict)

---

## 📚 **Related Documents**

- **Authoritative**:
  - `DD-TEST-002-integration-test-container-orchestration.md` - VIOLATED by current approach
  - `DD-TEST-001-port-allocation-strategy.md` v1.6 - Correct port allocations

- **Reference Implementations**:
  - `test/integration/gateway/setup-infrastructure.sh` - Sequential startup (Go)
  - `test/integration/workflowexecution/setup-infrastructure.sh` - Sequential startup (Shell)
  - `test/integration/notification/setup-infrastructure.sh` - Sequential startup (Shell)

- **Handoff Documents**:
  - `AA_PODMAN_COMPOSE_OUTDATED_CRITICAL_DEC_22_2025.md` - Port conflict analysis (superseded by this document)
  - `ALL_SERVICES_DS_INFRASTRUCTURE_AUDIT_DEC_22_2025.md` - Noted AIAnalysis uses podman-compose

---

**Document Status**: ✅ **COMPLETE**
**Confidence**: **100%** that AIAnalysis violates DD-TEST-002 and must migrate
**Recommended Action**: Migrate to DD-TEST-002 sequential startup pattern immediately











