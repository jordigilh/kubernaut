# Gateway Integration Test Port Allocation Triage

**Date**: January 15, 2026  
**Issue**: Port allocations in recently upgraded Gateway integration suite do NOT match DD-TEST-001  
**Authority**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md` (v2.5)  
**Status**: 🚨 VIOLATIONS FOUND - IMMEDIATE FIX REQUIRED

---

## 🚨 **Violations Detected**

### **Current State** (WRONG ❌)
File: `test/integration/gateway/suite_test.go`

```go
const (
	// PostgreSQL configuration
	gatewayPostgresPort     = 15439  // ❌ WRONG
	gatewayPostgresUser     = "gateway_test"
	gatewayPostgresPassword = "gateway_test_password"
	gatewayPostgresDB       = "gateway_test"
	gatewayPostgresContainer = "gateway-integration-postgres"
	
	// DataStorage configuration
	gatewayDataStoragePort      = 15440  // ❌ WRONG
	gatewayDataStorageContainer = "gateway-integration-datastorage"
)
```

**Missing Infrastructure**:
- ❌ NO Redis configuration
- ❌ NO Immudb configuration

---

### **Authoritative Standard** (DD-TEST-001 lines 363-392)

```yaml
Gateway Integration Tests:
  PostgreSQL (Data Storage dependency):
    Host Port: 15437  # ✅ CORRECT
    Container Port: 5432
    Connection: 127.0.0.1:15437
    Purpose: Workflow catalog for Data Storage

  Redis:
    Host Port: 16380  # ✅ MISSING - REQUIRED
    Container Port: 6379
    Connection: 127.0.0.1:16380
    Purpose: Gateway rate limiting + Data Storage DLQ

  Immudb (Data Storage dependency):
    Host Port: 13323  # ✅ MISSING - REQUIRED
    Container Port: 3322
    Connection: immudb://127.0.0.1:13323
    Purpose: Audit event storage via Data Storage

  Gateway API:
    Host Port: 18080  # ✅ NOT NEEDED (direct ProcessSignal calls)
    Container Port: 8080
    Connection: http://127.0.0.1:18080

  Data Storage (Dependency):
    Host Port: 18091  # ✅ CORRECT
    Container Port: 8080
    Connection: http://127.0.0.1:18091
```

---

## 📊 **Violation Summary**

| Component | Current | DD-TEST-001 | Status | Impact |
|-----------|---------|-------------|--------|--------|
| **PostgreSQL** | 15439 | 15437 | ❌ WRONG | Port conflict with HAPI |
| **DataStorage** | 15440 | 18091 | ❌ WRONG | Wrong port range (should be 180xx) |
| **Redis** | MISSING | 16380 | ❌ MISSING | DataStorage DLQ won't work |
| **Immudb** | MISSING | 13323 | ❌ MISSING | SOC2 audit trail won't work |
| **Gateway API** | N/A | 18080 | ✅ OK | Not needed (direct calls) |

---

## 🔍 **Root Cause Analysis**

### **Why PostgreSQL 15439 is Wrong**

From DD-TEST-001 Port Collision Matrix (lines 819-833):

```
| **Gateway** | 15437 | 16380 | **13323** | 18080 | Data Storage: 18091 |
| **HolmesGPT API (Python)** | 15439 | 16387 | **13329** | 18120 | Data Storage: 18098 |
```

**Port 15439 is officially allocated to HAPI integration tests!**

Using 15439 for Gateway will cause:
- ❌ Port collision if Gateway + HAPI integration tests run in parallel
- ❌ Violation of DD-TEST-001 authority
- ❌ Breaking the port allocation strategy

### **Why DataStorage 15440 is Wrong**

**Port 15440 is in the PostgreSQL range (15433-15442)**

From DD-TEST-001 (lines 33-46):
```
| Service | Production | Integration Tests | E2E Tests |
| Data Storage | 8081 | 18090-18099 | 28090-28099 |
| PostgreSQL | 5432 | 15433-15442 | 25433-25442 |
```

**DataStorage should use 18091 (from 180xx range for services)**

### **Why Redis is Missing**

DataStorage **REQUIRES** Redis for DLQ (Dead Letter Queue):
- Audit events that fail to write go to DLQ
- Integration tests need real DLQ behavior
- All other services (AIAnalysis, SignalProcessing, etc.) include Redis

From DD-TEST-001 Port Collision Matrix:
```
| **Gateway** | 15437 | 16380 | **13323** | 18080 | Data Storage: 18091 |
```

**Redis 16380 is officially allocated and REQUIRED**

### **Why Immudb is Missing**

Immudb provides **SOC2-compliant immutable audit trails**:
- Integration tests need to verify audit event immutability
- All services using DataStorage now include Immudb (SOC2 Gap #9)
- Added in DD-TEST-001 v2.2 (2026-01-06)

From DD-TEST-001:
```
| **Gateway** | 15437 | 16380 | **13323** | 18080 | Data Storage: 18091 |
```

**Immudb 13323 is officially allocated and REQUIRED for SOC2**

---

## ✅ **Required Fixes**

### **Fix 1: Update Port Constants**

```go
const (
	// PostgreSQL configuration (DataStorage dependency)
	gatewayPostgresPort     = 15437  // FIXED: Was 15439 (HAPI conflict)
	gatewayPostgresUser     = "gateway_test"
	gatewayPostgresPassword = "gateway_test_password"
	gatewayPostgresDB       = "gateway_test"
	gatewayPostgresContainer = "gateway-integration-postgres"
	
	// Redis configuration (DataStorage DLQ)
	gatewayRedisPort      = 16380  // NEW: Required for DataStorage DLQ
	gatewayRedisContainer = "gateway-integration-redis"
	
	// Immudb configuration (SOC2 immutable audit)
	gatewayImmudbPort      = 13323  // NEW: Required for SOC2 compliance
	gatewayImmudbContainer = "gateway-integration-immudb"
	
	// DataStorage configuration
	gatewayDataStoragePort      = 18091  // FIXED: Was 15440 (wrong range)
	gatewayDataStorageContainer = "gateway-integration-datastorage"
)
```

### **Fix 2: Add Redis Infrastructure**

**Function**: `startRedis()`

```go
func startRedis() error {
	// Remove existing container if any
	_ = exec.Command("podman", "rm", "-f", gatewayRedisContainer).Run()
	
	cmd := exec.Command("podman", "run", "-d",
		"--name", gatewayRedisContainer,
		"--network", "gateway-integration-net",
		"-p", fmt.Sprintf("%d:6379", gatewayRedisPort),
		"redis:7-alpine",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Redis: %w: %s", err, output)
	}
	
	// Wait for Redis to be ready
	time.Sleep(2 * time.Second)
	return nil
}
```

### **Fix 3: Add Immudb Infrastructure**

**Function**: `startImmudb()`

```go
func startImmudb() error {
	// Remove existing container if any
	_ = exec.Command("podman", "rm", "-f", gatewayImmudbContainer).Run()
	
	cmd := exec.Command("podman", "run", "-d",
		"--name", gatewayImmudbContainer,
		"--network", "gateway-integration-net",
		"-p", fmt.Sprintf("%d:3322", gatewayImmudbPort),
		"-e", "IMMUDB_ADMIN_PASSWORD=immudb",
		"codenotary/immudb:latest",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Immudb: %w: %s", err, output)
	}
	
	// Wait for Immudb to be ready
	time.Sleep(3 * time.Second)
	return nil
}
```

### **Fix 4: Update SynchronizedBeforeSuite Phase 1**

```go
// 3. Start PostgreSQL
logger.Info("[Process 1] Step 3: Start PostgreSQL container")
err = startPostgreSQL()
Expect(err).ToNot(HaveOccurred(), "PostgreSQL start must succeed")

// 4. Start Redis  // NEW
logger.Info("[Process 1] Step 4: Start Redis container")
err = startRedis()
Expect(err).ToNot(HaveOccurred(), "Redis start must succeed")

// 5. Start Immudb  // NEW
logger.Info("[Process 1] Step 5: Start Immudb container")
err = startImmudb()
Expect(err).ToNot(HaveOccurred(), "Immudb start must succeed")

// 6. Apply migrations to PUBLIC schema  // Was step 4
logger.Info("[Process 1] Step 6: Apply database migrations")
// ... existing code ...

// 7. Start DataStorage service  // Was step 5
logger.Info("[Process 1] Step 7: Start DataStorage service")
// ... existing code ...
```

### **Fix 5: Update DataStorage Service Startup**

DataStorage needs Redis and Immudb connections:

```go
cmd = exec.Command("podman", "run", "-d",
	"--name", gatewayDataStorageContainer,
	"--network", "gateway-integration-net",
	"-p", fmt.Sprintf("%d:8080", gatewayDataStoragePort),
	"-e", fmt.Sprintf("POSTGRES_HOST=%s", gatewayPostgresContainer),
	"-e", "POSTGRES_PORT=5432",
	"-e", fmt.Sprintf("POSTGRES_USER=%s", gatewayPostgresUser),
	"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", gatewayPostgresPassword),
	"-e", fmt.Sprintf("POSTGRES_DB=%s", gatewayPostgresDB),
	"-e", fmt.Sprintf("REDIS_HOST=%s", gatewayRedisContainer),  // NEW
	"-e", "REDIS_PORT=6379",                                     // NEW
	"-e", fmt.Sprintf("IMMUDB_HOST=%s", gatewayImmudbContainer), // NEW
	"-e", "IMMUDB_PORT=3322",                                    // NEW
	"-e", "IMMUDB_USERNAME=immudb",                              // NEW
	"-e", "IMMUDB_PASSWORD=immudb",                              // NEW
	"-e", "IMMUDB_DATABASE=defaultdb",                           // NEW
	"kubernaut-datastorage:latest",
)
```

### **Fix 6: Update Cleanup**

```go
func cleanupInfrastructure() {
	// Stop containers
	_ = exec.Command("podman", "rm", "-f", gatewayDataStorageContainer).Run()
	_ = exec.Command("podman", "rm", "-f", gatewayImmudbContainer).Run()  // NEW
	_ = exec.Command("podman", "rm", "-f", gatewayRedisContainer).Run()   // NEW
	_ = exec.Command("podman", "rm", "-f", gatewayPostgresContainer).Run()
	
	// Remove network
	_ = exec.Command("podman", "network", "rm", "gateway-integration-net").Run()
}
```

---

## 📋 **Compliance Checklist**

After fixes:

- [x] PostgreSQL: 15437 ✅ (was 15439)
- [x] Redis: 16380 ✅ (was missing)
- [x] Immudb: 13323 ✅ (was missing)
- [x] DataStorage: 18091 ✅ (was 15440)
- [x] Matches DD-TEST-001 Port Collision Matrix
- [x] No conflicts with HAPI (15439)
- [x] All DataStorage dependencies present
- [x] SOC2 compliance (Immudb)

---

## 🎯 **Why This Matters**

### **Parallel Execution Safety**
Gateway + HAPI integration tests can now run in parallel without port conflicts

### **SOC2 Compliance**
Immudb provides immutable audit trails (SOC2 Gap #9 resolution)

### **DataStorage Functionality**
Redis DLQ ensures audit events don't get lost during failures

### **Consistency**
Gateway now matches the pattern used by all other services:
- AIAnalysis: PostgreSQL + Redis + Immudb + DataStorage
- SignalProcessing: PostgreSQL + Redis + Immudb + DataStorage
- RemediationOrchestrator: PostgreSQL + Redis + Immudb + DataStorage
- Notification: PostgreSQL + Redis + Immudb + DataStorage
- WorkflowExecution: PostgreSQL + Redis + Immudb + DataStorage
- AuthWebhook: PostgreSQL + Redis + Immudb + DataStorage

---

## 📖 **Authority Reference**

**Document**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`  
**Version**: 2.5 (2026-01-11)  
**Section**: Lines 363-392 (Gateway Integration Tests)  
**Port Collision Matrix**: Lines 819-833  
**Status**: ✅ AUTHORITATIVE - MUST COMPLY

---

**Document Status**: 🚨 Active - Violations Found  
**Created**: 2026-01-15  
**Purpose**: Triage and fix Gateway integration test port allocation violations  
**Next**: Apply all fixes to `test/integration/gateway/suite_test.go`
