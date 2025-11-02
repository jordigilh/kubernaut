# Context API Infrastructure Issue - Data Storage Service Connection

**Date**: November 2, 2025  
**Status**: 🔴 **BLOCKED** - PostgreSQL authentication from host to container  
**Impact**: Integration tests cannot run (unit tests work fine)

---

## 🎯 **Current State**

### ✅ **What's Working**:
1. ✅ **Context API Unit Tests**: 111/116 tests passing (95.7%)
   - **Failing**: 5 PostgresClient tests (need DB infrastructure)
   - **Skipped**: 26 tests (expected)
   - **Result**: `go test ./test/unit/contextapi/... -v`

2. ✅ **PostgreSQL Container**: Running on port 5432
   - Container: `datastorage-postgres`
   - Image: `pgvector/pgvector:pg16`
   - Status: UP and healthy

3. ✅ **Database Created**: `action_history` database exists with `db_user`

4. ✅ **Internal Connectivity**: PostgreSQL works from inside container
   ```bash
   podman exec datastorage-postgres bash -c "PGPASSWORD=db_password psql -h localhost -U db_user -d action_history -c 'SELECT 1;'"
   # ✅ WORKS
   ```

### ❌ **What's NOT Working**:
1. ❌ **Data Storage Service**: Cannot connect to PostgreSQL from host
   - **Error**: `pq: password authentication failed for user "db_user"`
   - **Attempted Fixes**:
     - ✅ Added `host all all all md5` to `pg_hba.conf`
     - ✅ Restarted PostgreSQL
     - ✅ Recreated user with password
     - ❌ Still failing from host

2. ❌ **Context API Integration Tests**: Cannot run without Data Storage Service
   - **Requires**: Data Storage Service on port 8080
   - **Required Endpoints**: 
     - `GET /api/v1/incidents` (read incidents)
     - `POST /api/v1/audit/*` (write audit traces)

---

## 🔍 **Root Cause Analysis**

### **PostgreSQL Authentication from Host**

**Symptom**: Data Storage Service (running on host) cannot connect to PostgreSQL (running in Podman container).

**Configuration**:
```go
// cmd/datastorage/main.go:76
dbConnStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
    *dbHost, *dbPort, *dbName, *dbUser, *dbPassword)
```

**Attempted**:
- Connection string: `host=localhost port=5432 dbname=action_history user=db_user password=db_password sslmode=disable`
- PostgreSQL `pg_hba.conf` updated with `host all all all md5`
- User created with `CREATE USER db_user WITH PASSWORD 'db_password';`
- Database owned by `db_user`: `ALTER DATABASE action_history OWNER TO db_user;`

**Still Failing**:
```json
{"level":"fatal","msg":"Failed to create server","error":"failed to ping PostgreSQL: pq: password authentication failed for user \"db_user\""}
```

**Hypothesis**:
- Podman container network isolation
- PostgreSQL listening address configuration
- Authentication method mismatch (md5 vs scram-sha-256)
- Connection from `localhost` might be treated differently than from container IP

---

## 🔧 **Potential Solutions**

### **Option A: Run Data Storage Service in Container** (RECOMMENDED)
Run Data Storage Service in a Podman container on the same network as PostgreSQL.

**Pros**:
- ✅ Containers can communicate directly
- ✅ Standard Podman networking
- ✅ Matches production deployment

**Cons**:
- ⚠️ Requires container image for Data Storage Service
- ⚠️ More complex dev setup

**Implementation**:
```bash
# Build Data Storage Service container
podman build -t datastorage:dev -f deploy/Dockerfile.datastorage .

# Run on same network as PostgreSQL
podman run -d --name datastorage-service \
  --network container:datastorage-postgres \
  -e DB_HOST=localhost \
  -e DB_PORT=5432 \
  -e DB_USER=db_user \
  -e DB_PASSWORD=db_password \
  -e DB_NAME=action_history \
  -p 8080:8080 \
  datastorage:dev
```

---

### **Option B: Fix PostgreSQL Host Authentication**
Investigate PostgreSQL authentication configuration for host connections.

**Potential Fixes**:
1. Check PostgreSQL `listen_addresses` setting
2. Use PostgreSQL SCRAM-SHA-256 authentication instead of MD5
3. Check Podman port forwarding (might need `--network=host`)
4. Use PostgreSQL trust authentication for dev (insecure but works)

**Implementation**:
```bash
# Check current listen_addresses
podman exec datastorage-postgres psql -U postgres -c "SHOW listen_addresses;"

# Update to listen on all interfaces
podman exec datastorage-postgres psql -U postgres -c "ALTER SYSTEM SET listen_addresses = '*';"
podman restart datastorage-postgres
```

---

### **Option C: Use Test Database for Context API** (TEMPORARY WORKAROUND)
Run Context API integration tests against a test PostgreSQL database without Data Storage Service dependency.

**Pros**:
- ✅ Unblocks Context API testing
- ✅ Simple to implement

**Cons**:
- ❌ Tests direct PostgreSQL access (not production architecture per ADR-032)
- ❌ Violates Data Access Layer Isolation principle
- ❌ Technical debt

---

## 📊 **Impact Assessment**

### **What Can Be Done Without Fix**:
- ✅ Context API unit tests (111/116 passing)
- ✅ Code reviews and documentation
- ✅ Data Storage Service code changes (can't test)
- ✅ Other services (Effectiveness Monitor, WorkflowExecution, etc.)

### **What's Blocked**:
- ❌ Context API integration tests with Podman Redis + Data Storage Service
- ❌ Full Context API validation (100% test passing goal)
- ❌ Data Storage Service runtime verification
- ❌ End-to-end testing of Context API → Data Storage → PostgreSQL flow

---

## 🎯 **Recommendation**

**Recommended Approach**: **Option A - Run Data Storage Service in Container**

**Rationale**:
1. ✅ Matches production deployment (containers communicate over network)
2. ✅ Standard Podman/Docker pattern
3. ✅ Unblocks Context API integration tests
4. ✅ Avoids PostgreSQL authentication complexity

**Estimated Time**: 1-2 hours to:
1. Create Dockerfile for Data Storage Service (if doesn't exist)
2. Build container image
3. Configure container networking
4. Update Context API integration test infrastructure
5. Validate end-to-end flow

---

## 🚀 **Next Steps**

### **Immediate**:
1. Decide on approach (A, B, or C)
2. If Option A: Create Dockerfile and container setup
3. If Option B: Deep-dive PostgreSQL authentication
4. If Option C: Update Context API tests (temporary)

### **Long-term**:
1. Document infrastructure setup in `docs/development/INTEGRATION_TEST_SETUP.md`
2. Create `make` targets for easy dev environment setup
3. Add troubleshooting guide for common Podman/PostgreSQL issues
4. Consider Docker Compose / Podman Compose for multi-container setup

---

## 📝 **Session Notes**

**Time Spent**: ~1 hour on PostgreSQL authentication debugging  
**Attempts**: 10+ iterations of credential/config changes  
**Outcome**: PostgreSQL works internally, host connection blocked  
**Decision**: Document issue, recommend containerized approach  

**Confidence**: 90% that Option A (containerization) will solve the issue permanently.

