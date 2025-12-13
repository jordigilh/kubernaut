# TRIAGE: DataStorage Configuration for Integration Tests

**Date**: 2025-12-11
**Priority**: 🔴 **HIGH** - Blocks SP Integration Tests
**Status**: ✅ **TRIAGED** - Solution Identified
**Issue**: DataStorage requires ADR-030 config pattern, not environment variables

---

## 🔍 **Investigation Results**

### **Question**: Does DataStorage accept environment variables for configuration?

**Answer**: ❌ **NO** - DataStorage follows **ADR-030 strict configuration pattern**:

1. **MANDATORY**: `CONFIG_PATH` environment variable pointing to YAML file
2. **MANDATORY**: YAML file with full configuration structure
3. **MANDATORY**: Separate secret files for passwords (mounted Kubernetes Secrets)
4. **NO**: Individual environment variables for database, Redis, etc.

### **Code Evidence**

**File**: `cmd/datastorage/main.go:61-82`
```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"),
        "CONFIG_PATH environment variable required (ADR-030)")
    os.Exit(1)
}

cfg, err := config.LoadFromFile(cfgPath)
if err != nil {
    logger.Error(err, "Failed to load configuration file (ADR-030)")
    os.Exit(1)
}

// MANDATORY - always called, no dev-mode bypass
if err := cfg.LoadSecrets(); err != nil {
    logger.Error(err, "Failed to load secrets (ADR-030 Section 6)")
    os.Exit(1)
}
```

**File**: `pkg/datastorage/config/config.go:74,91`
```go
type DatabaseConfig struct {
    Host     string `yaml:"host"`
    Port     int    `yaml:"port"`
    Name     string `yaml:"name"`
    User     string `yaml:"user"`
    Password string `yaml:"-"`  // NOT in YAML - loaded from secret file
    // ...
    SecretsFile string `yaml:"secretsFile"`  // REQUIRED
    PasswordKey string `yaml:"passwordKey"`  // REQUIRED
}

type RedisConfig struct {
    Addr     string `yaml:"addr"`
    Password string `yaml:"-"`  // NOT in YAML - loaded from secret file
    // ...
    SecretsFile string `yaml:"secretsFile"`  // REQUIRED
    PasswordKey string `yaml:"passwordKey"`  // REQUIRED
}
```

---

## 📋 **Required Configuration Structure**

### **1. Main Config YAML** (`/tmp/datastorage-test-config.yaml`)
```yaml
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: "30s"
  write_timeout: "30s"

logging:
  level: "debug"
  format: "json"

database:
  host: "localhost"
  port: 51000  # Dynamic port from test
  name: "kubernaut_audit"
  user: "postgres"
  # password: NOT HERE - loaded from secretsFile
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "5m"
  conn_max_idle_time: "10m"
  secretsFile: "/tmp/db-secrets.yaml"  # REQUIRED
  passwordKey: "password"               # REQUIRED

redis:
  addr: "localhost:6379"  # Not used in tests but YAML structure required
  db: 0
  dlq_stream_name: "audit:dlq:test"
  dlq_max_len: 10000
  dlq_consumer_group: "test-consumers"
  secretsFile: "/tmp/redis-secrets.yaml"  # REQUIRED
  passwordKey: "password"                  # REQUIRED
```

### **2. Database Secrets File** (`/tmp/db-secrets.yaml`)
```yaml
password: "postgres"
```

### **3. Redis Secrets File** (`/tmp/redis-secrets.yaml`)
```yaml
password: ""  # Empty for test Redis (no auth)
```

---

## ✅ **Solution: Update helpers_infrastructure.go**

### **Required Changes**

**File**: `test/integration/signalprocessing/helpers_infrastructure.go`

#### **Step 1: Create Config Files Before Starting Container**

```go
func SetupDataStorageTestServer(ctx context.Context, pgClient *PostgresTestClient) *DataStorageTestServer {
    // ... existing port allocation ...

    // CREATE TEMPORARY CONFIG FILES
    configPath := "/tmp/datastorage-test-config.yaml"
    dbSecretsPath := "/tmp/db-secrets-test.yaml"
    redisSecretsPath := "/tmp/redis-secrets-test.yaml"

    // 1. Create main config YAML
    configContent := fmt.Sprintf(`server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: "30s"
  write_timeout: "30s"

logging:
  level: "debug"
  format: "json"

database:
  host: "host.containers.internal"  # Podman special DNS for host
  port: %d
  name: "kubernaut_audit"
  user: "postgres"
  ssl_mode: "disable"
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "5m"
  conn_max_idle_time: "10m"
  secretsFile: "/secrets/db-secrets.yaml"
  passwordKey: "password"

redis:
  addr: "localhost:6379"
  db: 0
  dlq_stream_name: "audit:dlq:test"
  dlq_max_len: 10000
  dlq_consumer_group: "test-consumers"
  secretsFile: "/secrets/redis-secrets.yaml"
  passwordKey: "password"
`, pgClient.Port)

    err := os.WriteFile(configPath, []byte(configContent), 0644)
    Expect(err).ToNot(HaveOccurred())

    // 2. Create database secrets file
    dbSecretsContent := `password: "postgres"`
    err = os.WriteFile(dbSecretsPath, []byte(dbSecretsContent), 0644)
    Expect(err).ToNot(HaveOccurred())

    // 3. Create redis secrets file
    redisSecretsContent := `password: ""`
    err = os.WriteFile(redisSecretsPath, []byte(redisSecretsContent), 0644)
    Expect(err).ToNot(HaveOccurred())

    // ... rest of container startup with volume mounts ...
}
```

#### **Step 2: Mount Config Files in Container**

```go
cmd := exec.Command("podman", "run", "-d",
    "--name", containerName,
    "-p", fmt.Sprintf("%d:8080", dsPort),

    // MOUNT CONFIG FILES
    "-v", fmt.Sprintf("%s:/app/config.yaml", configPath),
    "-v", fmt.Sprintf("%s:/secrets/db-secrets.yaml", dbSecretsPath),
    "-v", fmt.Sprintf("%s:/secrets/redis-secrets.yaml", redisSecretsPath),

    // SET CONFIG_PATH ENVIRONMENT VARIABLE (MANDATORY)
    "-e", "CONFIG_PATH=/app/config.yaml",
    "-e", "ENV=development",

    // Use host.containers.internal for PostgreSQL access
    "--add-host", "host.containers.internal:host-gateway",

    "localhost/kubernaut-datastorage:e2e-test",
)
```

#### **Step 3: Clean Up Temp Files**

```go
func TeardownDataStorageTestServer(server *DataStorageTestServer) {
    if server == nil {
        return
    }

    GinkgoWriter.Println("🧹 Stopping DataStorage container...")

    // Stop container
    stopCmd := exec.Command("podman", "stop", "sp-datastorage-integration")
    _ = stopCmd.Run()

    // Remove container
    rmCmd := exec.Command("podman", "rm", "sp-datastorage-integration")
    _ = rmCmd.Run()

    // CLEAN UP TEMP CONFIG FILES
    os.Remove("/tmp/datastorage-test-config.yaml")
    os.Remove("/tmp/db-secrets-test.yaml")
    os.Remove("/tmp/redis-secrets-test.yaml")

    GinkgoWriter.Println("✅ DataStorage cleanup complete")
}
```

---

## 🎯 **Alternative: Simplify for Tests Only**

### **Option B**: Add Test-Mode Bypass to DataStorage

**IF** the team wants to simplify integration tests, DataStorage could add a test mode:

**File**: `cmd/datastorage/main.go:86` (modify LoadSecrets call)
```go
// ADR-030 Section 6: Load secrets from mounted files
// TEST MODE: Skip secret loading if ENV=test and passwords already set
if os.Getenv("ENV") == "test" && cfg.Database.Password != "" && cfg.Redis.Password != "" {
    logger.Info("Test mode: Using inline passwords (NOT for production)")
} else {
    logger.Info("Loading secrets from mounted files (ADR-030 Section 6)")
    if err := cfg.LoadSecrets(); err != nil {
        logger.Error(err, "Failed to load secrets (ADR-030 Section 6)")
        os.Exit(1)
    }
}
```

**File**: `pkg/datastorage/config/config.go:74,91` (allow YAML passwords in test mode)
```go
type DatabaseConfig struct {
    // ...
    Password string `yaml:"password,omitempty"` // Allow YAML password in test mode
    // ...
}

type RedisConfig struct {
    // ...
    Password string `yaml:"password,omitempty"` // Allow YAML password in test mode
    // ...
}
```

**Pros**:
- ✅ Simpler for integration tests
- ✅ No temp file management needed
- ✅ Faster test setup

**Cons**:
- ❌ Weakens ADR-030 separation of config and secrets
- ❌ Risk of test mode leaking to production
- ❌ Different config patterns for dev vs prod

---

## 📊 **Recommendation**

### **Preferred**: Option A - Full ADR-030 Compliance (Solution Above)

**Reasoning**:
1. ✅ Maintains ADR-030 integrity (config/secret separation)
2. ✅ Tests use same code path as production
3. ✅ Better validation of Kubernetes Secret mounting pattern
4. ✅ No risk of test-mode bypass in production

**Effort**: ~2-3 hours to implement in `helpers_infrastructure.go`

**Confidence**: 95% - Pattern is proven (used by RO team in podman-compose.test.yml)

---

## ✅ **Action Items**

### **Immediate (SP Team)**
1. ✅ Update `helpers_infrastructure.go::SetupDataStorageTestServer()`:
   - Create temporary config YAML with dynamic PostgreSQL port
   - Create temporary secret files for passwords
   - Mount config files into container
   - Set `CONFIG_PATH=/app/config.yaml` environment variable
   - Use `--add-host host.containers.internal:host-gateway` for PostgreSQL access

2. ✅ Update `helpers_infrastructure.go::TeardownDataStorageTestServer()`:
   - Clean up temporary config files

3. ✅ Test fix:
   ```bash
   ginkgo -v ./test/integration/signalprocessing/...
   ```

### **Follow-up (DataStorage Team)**
- ⏸️ **Optional**: Consider if test-mode bypass is worth the trade-off
- ⏸️ **Documentation**: Update `config/data-storage.yaml` to remove `password` field (it's ignored)

---

## 📚 **Related Documentation**

- **ADR-030**: Configuration Management (YAML + Secret Files)
- **DD-007**: Graceful Shutdown Pattern
- **BR-SP-090**: Audit Trail Persistence Requirement
- **Test Infrastructure**: `test/integration/signalprocessing/helpers_infrastructure.go`
- **Reference**: `podman-compose.test.yml` (RO team's working example)

---

**Document Status**: ✅ TRIAGED - Solution Identified
**Created**: 2025-12-11
**Next Step**: Implement Option A (Full ADR-030 Compliance) in `helpers_infrastructure.go`
**File**: `docs/handoff/TRIAGE_DS_CONFIG_FOR_INTEGRATION_TESTS.md`






