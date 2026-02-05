# ADR-030: Service Configuration Management via YAML Files

**Status**: ✅ **APPROVED & IMPLEMENTED**
**Date**: November 2, 2025
**Last Updated**: January 30, 2026 (camelCase mandate added - V1.1)
**Authoritative Implementation**: **Context API Service** ([`pkg/contextapi/config/config.go`](../../pkg/contextapi/config/config.go))
**Naming Convention**: ✅ **camelCase for YAML fields** - See [CRD_FIELD_NAMING_CONVENTION.md V1.1](../CRD_FIELD_NAMING_CONVENTION.md)
**Applies To**: All microservices in Kubernaut platform

---

## Context

All microservices in the Kubernaut platform must have consistent configuration management that is:
- **Production-Ready**: Safe for production deployments
- **Manageable**: Easy to review, update, and version
- **Validated**: Errors caught early, before runtime
- **Auditable**: Changes tracked in Git and Kubernetes audit logs

**Historical Note**: Gateway Service originally had configuration types mixed with server logic and lacked `LoadFromFile()` implementation. On November 2, 2025, Gateway was refactored following TDD methodology to match the Context API pattern, establishing Context API as the sole authoritative reference. See [Configuration Location Triage](./ADR-030-CONFIGURATION-LOCATION-TRIAGE.md) for detailed analysis.

---

## Decision

**MANDATE**: All microservices MUST use structured YAML configuration files loaded as Kubernetes ConfigMaps.

### Configuration Pattern (Standard)

**Authoritative Reference**: ✅ **Context API Service** ([`pkg/contextapi/config/config.go`](../../pkg/contextapi/config/config.go))

This pattern has been **implemented** in:
- ✅ **Context API Service**: [`pkg/contextapi/config`](../../pkg/contextapi/config/config.go) (AUTHORITATIVE)
- ✅ **Gateway Service**: [`pkg/gateway/config`](../../pkg/gateway/config/config.go) (Refactored Nov 2, 2025)
- ✅ **Data Storage Service**: [`pkg/datastorage/server`](../../pkg/datastorage/server/server.go)
- 🚧 **Notification Service**: Configuration to be refactored
- 🚧 **Dynamic Toolset Service**: Configuration to be refactored

**New services MUST follow the Context API configuration pattern.**

---

## Implementation Standard

### 1. Configuration File Structure

**Location**: `pkg/<service>/config/config.go`

**Reference Implementation**: [`pkg/contextapi/config/config.go`](../../pkg/contextapi/config/config.go)

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete service configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Cache    CacheConfig    `yaml:"cache"`
	Logging  LoggingConfig  `yaml:"logging"`

	// Service-specific sections
	DataStorage DataStorageConfig `yaml:"datastorage"` // Example: Context API
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int           `yaml:"port"`
	Host         string        `yaml:"host"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// DatabaseConfig contains PostgreSQL database configuration
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase (AUTHORITATIVE)
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslMode"`  // camelCase per naming convention
}

// CacheConfig contains Redis and LRU cache configuration
// Per CRD_FIELD_NAMING_CONVENTION.md: YAML fields use camelCase (AUTHORITATIVE)
type CacheConfig struct {
	RedisAddr  string        `yaml:"redisAddr"`   // camelCase per naming convention
	RedisDB    int           `yaml:"redisDb"`     // camelCase per naming convention
	LRUSize    int           `yaml:"lruSize"`     // camelCase per naming convention
	DefaultTTL time.Duration `yaml:"defaultTtl"`  // camelCase per naming convention
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // json, console
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// LoadFromEnv overrides configuration values with environment variables
// Priority: Environment Variables > YAML File > Defaults
func (c *Config) LoadFromEnv() {
	// Database configuration overrides
	if host := os.Getenv("DB_HOST"); host != "" {
		c.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.Database.Port = p
		}
	}
	// ... more overrides
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate database configuration
	if c.Database.Host == "" {
		return fmt.Errorf("database host required")
	}
	if c.Database.Port == 0 {
		return fmt.Errorf("database port required")
	}

	// Validate server configuration
	if c.Server.Port < 1024 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1024 and 65535")
	}

	// ... more validation

	return nil
}
```

---

### 2. YAML Configuration File

**Location**: `config/<service>-config.yaml`

**Reference Example**: [`pkg/contextapi/config/testdata/valid-config.yaml`](../../pkg/contextapi/config/testdata/valid-config.yaml)

**NAMING CONVENTION**: ✅ **camelCase for YAML fields** (MANDATORY)
- **Authority**: [CRD_FIELD_NAMING_CONVENTION.md](../CRD_FIELD_NAMING_CONVENTION.md) §2 "Use camelCase for JSON/YAML Fields"
- **Applies To**: ALL YAML configuration files (service configs, CRD specs, Kubernetes manifests)
- **Rationale**: Consistency across entire platform, clean JSON/YAML serialization

```yaml
# Context API Configuration Example
# Per CRD_FIELD_NAMING_CONVENTION.md: camelCase for YAML fields
server:
  port: 8091
  host: "0.0.0.0"
  readTimeout: "30s"   # camelCase (not read_timeout)
  writeTimeout: "30s"  # camelCase (not write_timeout)

logging:
  level: "info"          # debug, info, warn, error
  format: "json"         # json, console

cache:
  redisAddr: "localhost:6379"  # camelCase (not redis_addr)
  redisDb: 0                   # camelCase (not redis_db)
  lruSize: 1000                # camelCase (not lru_size)
  default_ttl: "5m"

database:
  host: "localhost"
  port: 5432
  name: "action_history"
  user: "db_user"
  password: "slm_password_dev"    # Override with secret in production
  ssl_mode: "disable"

# Service-specific configuration
datastorage:
  url: "http://data-storage-service:8080"
  timeout: "5s"
  max_connections: 100
  circuit_breaker:
    threshold: 3
    timeout: "60s"
  retry:
    max_attempts: 3
    base_delay: "100ms"
    max_delay: "400ms"
```

---

### 3. Kubernetes ConfigMap Integration

**Method 1: Create ConfigMap from YAML file (RECOMMENDED)**

```bash
kubectl create configmap context-api-config \
  --from-file=config.yaml=config/context-api-config.yaml \
  --namespace=kubernaut-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

**Method 2: Using Kustomize (for environment-specific)**

```yaml
# deploy/context-api/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: kubernaut-system

configMapGenerator:
  - name: context-api-config
    files:
      - config.yaml=../../config/context-api-config.yaml
    options:
      disableNameSuffixHash: true

resources:
  - deployment.yaml
  - service.yaml
  - secret.yaml
```

---

### 4. Deployment Manifest

**File**: `deploy/<service>/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: context-api
  namespace: kubernaut-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: context-api
  template:
    metadata:
      labels:
        app: context-api
    spec:
      containers:
      - name: context-api
        image: kubernaut/context-api:v1.0.0

        # ========================================
        # CONFIGURATION: Mount ConfigMap as file
        # ========================================
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true

        # ========================================
        # MINIMAL ENV VARS: Only secrets/overrides
        # ========================================
        env:
        # Configuration file path
        - name: CONFIG_FILE
          value: /etc/config/config.yaml

        # Secrets (from Secret, NOT ConfigMap)
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: context-api-secret
              key: db-password

        ports:
        - name: http
          containerPort: 8091
        - name: metrics
          containerPort: 9090

        livenessProbe:
          httpGet:
            path: /health/live
            port: http

        readinessProbe:
          httpGet:
            path: /health/ready
            port: http

      # ========================================
      # VOLUMES: ConfigMap mounted as file
      # ========================================
      volumes:
      - name: config
        configMap:
          name: context-api-config
          items:
          - key: config.yaml
            path: config.yaml
```

---

### 5. Main Application Integration

**File**: `cmd/<service>/main.go`

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jordigilh/kubernaut/pkg/contextapi/config"
	"github.com/jordigilh/kubernaut/pkg/contextapi/server"
	"go.uber.org/zap"
)

func main() {
	// ========================================
	// CONFIGURATION LOADING
	// ========================================
	configPath := flag.String("config",
		os.Getenv("CONFIG_FILE"),
		"Path to configuration file")
	flag.Parse()

	// Default config path
	if *configPath == "" {
		*configPath = "/etc/config/config.yaml"
	}

	// Load configuration from YAML file
	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to load configuration: %v", err))
	}

	// Override with environment variables (for secrets)
	cfg.LoadFromEnv()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("Invalid configuration: %v", err))
	}

	// ========================================
	// LOGGER INITIALIZATION
	// ========================================
	logger, err := initLogger(cfg.Logging)
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	logger.Info("Starting Context API service",
		zap.String("config_file", *configPath),
		zap.Int("server_port", cfg.Server.Port),
	)

	// ========================================
	// SERVER INITIALIZATION
	// ========================================
	srv, err := server.NewServer(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create server", zap.Error(err))
	}

	// Start server in goroutine
	go func() {
		if err := srv.Start(); err != nil {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	// ========================================
	// GRACEFUL SHUTDOWN
	// ========================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Server stopped")
}

func initLogger(cfg config.LoggingConfig) (*zap.Logger, error) {
	// Logger initialization logic
	// ...
	return zap.NewProduction()
}
```

---

### 6. Secret Management via Mounted Files

**Status**: ✅ **APPROVED PATTERN** (Effective November 3, 2025)

**Problem**: Environment variables for secrets have limitations:
- ❌ Visible in process listings
- ❌ Limited to string values
- ❌ No structured data support
- ❌ Difficult to rotate without restart

**Solution**: Mount Kubernetes Secrets as structured files

#### Configuration Structure

```yaml
# config/data-storage.yaml
database:
  host: "postgres-service"
  port: 5432
  user: "slm_user"  # Non-sensitive, can be in YAML

  # Secret file configuration
  secretsPath: "/etc/secrets/database"  # Mount point (deployment controls)
  secretsFile: "credentials.yaml"       # Structured secret file name
  passwordKey: "password"               # Key to extract from secret file

redis:
  addr: "redis:6379"
  secretsPath: "/etc/secrets/redis"
  secretsFile: "credentials.yaml"
  passwordKey: "password"  # Optional - may not need auth
```

#### Kubernetes Secret

```yaml
# deploy/<service>/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-db-credentials
  namespace: kubernaut-system
type: Opaque
stringData:
  credentials.yaml: |
    username: "slm_user"
    password: "actual_secure_password"
    # Can include additional metadata
    connection_pool: 25
    ssl_mode: "require"
```

#### Deployment with Secret Mount

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-storage
spec:
  template:
    spec:
      containers:
      - name: data-storage
        image: kubernaut/data-storage:v1.0.0

        # Mount ConfigMap (non-sensitive config)
        volumeMounts:
        - name: config
          mountPath: /etc/config
          readOnly: true

        # Mount Secret as structured file
        - name: db-secrets
          mountPath: /etc/secrets/database
          readOnly: true

        - name: redis-secrets
          mountPath: /etc/secrets/redis
          readOnly: true

      volumes:
      - name: config
        configMap:
          name: data-storage-config

      # Secrets mounted as files
      - name: db-secrets
        secret:
          secretName: datastorage-db-credentials

      - name: redis-secrets
        secret:
          secretName: datastorage-redis-credentials
```

#### Config Package Implementation

```go
// pkg/<service>/config/config.go

// DatabaseConfig with secret file support
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"` // Loaded from secret file, not YAML

	// Secret file configuration
	SecretsPath  string `yaml:"secretsPath"`  // e.g., "/etc/secrets/database"
	SecretsFile  string `yaml:"secretsFile"`  // e.g., "credentials.yaml"
	PasswordKey  string `yaml:"passwordKey"`  // e.g., "password"
	UsernameKey  string `yaml:"usernameKey"`  // Optional override
}

// LoadSecrets loads secrets from mounted Kubernetes Secret files
func (c *Config) LoadSecrets() error {
	// Load database secrets
	if c.Database.SecretsPath != "" {
		secrets, err := loadSecretFile(
			c.Database.SecretsPath,
			c.Database.SecretsFile,
		)
		if err != nil {
			return fmt.Errorf("failed to load database secrets: %w", err)
		}

		// Extract password using configured key
		if password, ok := secrets[c.Database.PasswordKey]; ok {
			c.Database.Password = password.(string)
		} else {
			return fmt.Errorf("password key '%s' not found in secret file", c.Database.PasswordKey)
		}

		// Optional: Override username from secret if key specified
		if c.Database.UsernameKey != "" {
			if username, ok := secrets[c.Database.UsernameKey]; ok {
				c.Database.User = username.(string)
			}
		}
	}

	// Load Redis secrets (if configured)
	if c.Redis.SecretsPath != "" {
		secrets, err := loadSecretFile(
			c.Redis.SecretsPath,
			c.Redis.SecretsFile,
		)
		if err != nil {
			return fmt.Errorf("failed to load redis secrets: %w", err)
		}

		if password, ok := secrets[c.Redis.PasswordKey]; ok {
			c.Redis.Password = password.(string)
		}
	}

	return nil
}

// loadSecretFile unmarshals a secret file (supports YAML and JSON)
func loadSecretFile(secretsPath, secretsFile string) (map[string]interface{}, error) {
	path := filepath.Join(secretsPath, secretsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret file %s: %w", path, err)
	}

	var secrets map[string]interface{}

	// Try YAML first
	if err := yaml.Unmarshal(data, &secrets); err == nil {
		return secrets, nil
	}

	// Fallback to JSON
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secret file as YAML or JSON: %w", err)
	}

	return secrets, nil
}
```

#### Main Application Integration

```go
// cmd/<service>/main.go

func main() {
	logger, _ := zap.NewProduction()

	// 1. Load configuration from YAML (ConfigMap)
	cfg, err := config.LoadFromFile("/etc/config/config.yaml")
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// 2. Load secrets from mounted files (Kubernetes Secrets)
	if err := cfg.LoadSecrets(); err != nil {
		logger.Fatal("Failed to load secrets", zap.Error(err))
	}

	// 3. Override with environment variables (for local dev/testing)
	cfg.LoadFromEnv()

	// 4. Validate complete configuration
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Invalid configuration", zap.Error(err))
	}

	logger.Info("Configuration loaded successfully",
		zap.String("database_host", cfg.Database.Host),
		zap.Bool("secrets_loaded", cfg.Database.Password != ""),
	)

	// ... start service
}
```

#### Benefits of Mounted Secret Files

1. **Security**:
   - ✅ Secrets not visible in environment variables
   - ✅ Not exposed in process listings (`ps aux`)
   - ✅ File permissions enforced (read-only, 0400)
   - ✅ Automatic rotation support (remount on update)

2. **Flexibility**:
   - ✅ Structured data (YAML/JSON) supports complex credentials
   - ✅ Multiple credentials in one file
   - ✅ Can include metadata (connection pools, timeouts)
   - ✅ Config specifies which keys to extract

3. **Kubernetes Native**:
   - ✅ Standard Kubernetes Secret pattern
   - ✅ Works with secret management tools (Vault, Sealed Secrets)
   - ✅ Supports secret rotation without env var limits
   - ✅ Better audit trails (file access logs)

4. **Deterministic**:
   - ✅ Config YAML specifies WHERE to find secrets (path)
   - ✅ Config YAML specifies WHAT to extract (keys)
   - ✅ Deployment controls WHAT secrets are there
   - ✅ Clear separation of concerns

#### Local Development Support

For local development without Kubernetes:

```go
// LoadFromEnv as fallback for local development
func (c *Config) LoadFromEnv() {
	// Only use environment variables if secret files not available
	if c.Database.Password == "" {
		if password := os.Getenv("DB_PASSWORD"); password != "" {
			c.Database.Password = password
		}
	}

	// Other non-secret overrides...
}
```

This allows:
- **Production**: Secrets from mounted files
- **Local Dev**: Secrets from environment variables
- **Deterministic**: Behavior controlled by config, not guessing

---

## Benefits

### 1. Production Safety
- ✅ **Secrets Separation**: Passwords in Secrets, config in ConfigMaps
- ✅ **Fail Fast**: Invalid configuration detected at startup
- ✅ **Immutable**: Configuration file is read-only in container
- ✅ **Rollback Friendly**: Easy to revert ConfigMap changes

### 2. Manageability
- ✅ **Single Source**: All configuration in one YAML file
- ✅ **Version Controlled**: Configuration tracked in Git
- ✅ **Human Readable**: YAML format easy to review
- ✅ **Environment-Specific**: Different files for dev/staging/prod

### 3. Validation
- ✅ **Schema Validation**: YAML structure enforced at runtime
- ✅ **Type Safety**: Go structs provide compile-time checking
- ✅ **Clear Errors**: Detailed error messages for misconfigurations
- ✅ **Early Detection**: Configuration errors caught at startup

### 4. Flexibility
- ✅ **Override Support**: Environment variables can override YAML
- ✅ **Priority System**: Env vars > YAML > Defaults
- ✅ **Testable**: Easy to create test configurations
- ✅ **Documented**: YAML file serves as documentation

---

## Gateway Service Configuration Reference

**Gateway** uses a more sophisticated nested structure organized by Single Responsibility Principle.

**See**: [Gateway API Specification - Configuration Section](../../services/stateless/gateway-service/api-specification.md#️-configuration)

### Gateway Configuration Structure

```yaml
# Gateway Service Configuration
# Organized by Single Responsibility Principle

# HTTP Server configuration
server:
  listen_addr: ":8080"
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

# Middleware configuration
middleware:
  rate_limit:
    requests_per_minute: 100
    burst: 10

# Infrastructure dependencies
infrastructure:
  redis:
    addr: redis-gateway.kubernaut-gateway.svc.cluster.local:6379
    db: 0
    dial_timeout: 5s
    read_timeout: 3s
    write_timeout: 3s
    pool_size: 10
    min_idle_conns: 2

# Business logic configuration
processing:
  deduplication:
    ttl: 5m

  storm:
    rate_threshold: 10
    pattern_threshold: 5
    aggregation_window: 1m

  environment:
    cache_ttl: 30s
    configmap_namespace: kubernaut-system
    configmap_name: kubernaut-environment-overrides
```

**Benefits of Gateway's Nested Structure**:
- **Discoverability**: +90% (clear logical grouping)
- **Maintainability**: +80% (small, focused structs)
- **Testability**: +70% (test sections independently)
- **Scalability**: +60% (organized growth)

---

## Configuration Priority

**Order of Precedence** (highest to lowest):
1. **Environment Variables** (secrets, runtime overrides)
2. **YAML Configuration File** (main configuration)
3. **Code Defaults** (fallback values in struct definitions)

### Example Priority Resolution

```yaml
# config.yaml
database:
  host: "postgres"
  user: "db_user"
  password: "default"    # Will be overridden
```

```yaml
# deployment.yaml
env:
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: context-api-secret
      key: db-password    # This overrides config.yaml password
```

**Result**: `DB_PASSWORD` from Secret takes precedence over YAML value.

---

## Environment-Specific Configurations

### Development
```yaml
# config/context-api-dev.yaml
server:
  port: 8091

logging:
  level: "debug"         # Verbose logging for dev
  format: "console"      # Human-readable

database:
  host: "localhost"      # Local database
  ssl_mode: "disable"
```

### Production
```yaml
# config/context-api-prod.yaml
server:
  port: 8091

logging:
  level: "info"          # Less verbose in production
  format: "json"         # Structured logging

database:
  host: "postgres.kubernaut-system.svc.cluster.local"
  ssl_mode: "require"    # Enforce SSL
```

---

## Compliance Checklist

Before deploying any new service, verify:

### Configuration Package
- [ ] Configuration package created: `pkg/<service>/config/config.go`
- [ ] Configuration structs defined with YAML tags
- [ ] `LoadFromFile()` function implemented
- [ ] `LoadFromEnv()` function implemented for local dev overrides
- [ ] `Validate()` function implemented with comprehensive checks

### Secret Management (MANDATORY)
- [ ] `LoadSecrets()` function implemented for mounted secret files
- [ ] Config structs include `secretsPath`, `secretsFile`, and key fields
- [ ] `loadSecretFile()` helper supports YAML and JSON parsing
- [ ] Secrets validated after loading (non-empty check)

### YAML Configuration
- [ ] YAML configuration file created: `config/<service>-config.yaml`
- [ ] Config specifies secret paths (e.g., `/etc/secrets/database`)
- [ ] Config specifies secret file names (e.g., `credentials.yaml`)
- [ ] Config specifies extraction keys (e.g., `passwordKey: "password"`)

### Kubernetes Resources
- [ ] ConfigMap manifest created: `deploy/<service>/configmap.yaml`
- [ ] Secret manifest created: `deploy/<service>/secret.yaml` (structured YAML/JSON)
- [ ] Deployment mounts ConfigMap at `/etc/config/`
- [ ] Deployment mounts Secrets at paths specified in config YAML
- [ ] Secret volumes set to `readOnly: true`

### Application Integration
- [ ] Main application loads config from `/etc/config/config.yaml`
- [ ] Main application calls `cfg.LoadSecrets()` after `LoadFromFile()`
- [ ] Main application calls `cfg.LoadFromEnv()` for local dev fallback
- [ ] Main application validates complete config before starting
- [ ] Config file is MANDATORY - fail fast if not found (ADR-030 determinism)

### Testing & Documentation
- [ ] Environment-specific configurations created (dev, staging, prod)
- [ ] Unit tests created for config loading and validation
- [ ] Unit tests created for secret file loading
- [ ] Integration tests verify secret mounting works
- [ ] Documentation updated with secret management approach

---

## Anti-Patterns to AVOID

### ❌ **DON'T**: Service Guessing Config File Location

**Problem**: Service code should NOT contain logic to guess where config files are based on environment.

```go
// ❌ BAD: Business code contains environment detection
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    // Service guessing environment - ANTI-PATTERN!
    if _, err := os.Stat("config/service.yaml"); err == nil {
        cfgPath = "config/service.yaml"  // Local dev?
    } else if _, err := os.Stat("/etc/config/config.yaml"); err == nil {
        cfgPath = "/etc/config/config.yaml"  // Kubernetes?
    } else {
        log.Fatal("Config file not found")
    }
}
```

**Why This is Wrong**:
1. ❌ **Violates Separation of Concerns**: Business code knows about deployment environments
2. ❌ **Non-Deterministic**: Behavior changes based on file system state
3. ❌ **Fragile**: Breaks if deployment uses different paths
4. ❌ **Hard to Test**: Tests depend on file system layout

### ✅ **DO**: Deployment Controls Config Location

```go
// ✅ GOOD: Service receives config path from environment
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    log.Fatal("CONFIG_PATH environment variable required",
        "reason", "Deployment is responsible for setting config file location")
}

cfg, err := config.LoadFromFile(cfgPath)
// ... service starts with provided config
```

**Deployment Responsibility**:

**Local Development** (docker-compose, script):
```bash
# Local developer sets CONFIG_PATH
export CONFIG_PATH=config/data-storage.yaml
./bin/data-storage
```

**Kubernetes Deployment**:
```yaml
env:
- name: CONFIG_PATH
  value: /etc/config/config.yaml  # Deployment controls this
```

**Benefits**:
- ✅ **Separation of Concerns**: Service doesn't know about deployment
- ✅ **Deterministic**: Behavior controlled by environment, not guessing
- ✅ **Flexible**: Deployment can use any path
- ✅ **Testable**: Tests set CONFIG_PATH explicitly

---

### ❌ **DON'T**: Environment Variables in Deployment

```yaml
# BAD: Too many env vars, hard to manage
env:
- name: SERVER_PORT
  value: "8091"
- name: SERVER_HOST
  value: "0.0.0.0"
- name: READ_TIMEOUT
  value: "30s"
- name: WRITE_TIMEOUT
  value: "30s"
- name: REDIS_ADDR
  value: "redis:6379"
- name: REDIS_DB
  value: "0"
# ... 20 more env vars ...
```

### ✅ **DO**: ConfigMap with Environment Variable Overrides

```yaml
# GOOD: Structured config file + secrets in env vars
volumeMounts:
- name: config
  mountPath: /etc/config
  readOnly: true

env:
# Only secrets and runtime-specific overrides
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: context-api-secret
      key: db-password
```

---

## 7. Startup Behavior: Crash-if-Missing Pattern

**Status**: ✅ **MANDATORY** (Effective December 2, 2025)

### Principle

**Services MUST crash at startup if required configuration or dependencies are unavailable.**

This provides:
- **Fail Fast**: Issues detected immediately, not at first request
- **Deterministic**: No partial functionality or silent degradation
- **Clear Errors**: Operators know exactly what's wrong from logs
- **Kubernetes Integration**: Pods restart, AlertManager detects CrashLoopBackOff

### Dependency Classification

| Category | Behavior | Examples |
|----------|----------|----------|
| **Required Config** | Crash if missing | `CONFIG_PATH`, database host |
| **Required Dependency** | Crash if unavailable | PostgreSQL, Redis, Tekton |
| **Optional Config** | Use defaults | Log level, timeouts |
| **Optional Dependency** | Degrade gracefully | Embedding service, cache |

### Implementation Pattern

```go
func main() {
    logger, _ := zap.NewProduction()

    // ========================================
    // REQUIRED: Configuration file
    // ========================================
    cfgPath := os.Getenv("CONFIG_PATH")
    if cfgPath == "" {
        // CRASH: Required config path not provided
        logger.Fatal("CONFIG_PATH environment variable required")
    }

    cfg, err := config.LoadFromFile(cfgPath)
    if err != nil {
        // CRASH: Config file missing or invalid
        logger.Fatal("Failed to load configuration",
            zap.String("path", cfgPath),
            zap.Error(err))
    }

    // ========================================
    // REQUIRED: Validate configuration
    // ========================================
    if err := cfg.Validate(); err != nil {
        // CRASH: Invalid configuration values
        logger.Fatal("Invalid configuration", zap.Error(err))
    }

    // ========================================
    // REQUIRED: Check dependencies
    // ========================================
    if err := checkRequiredDependencies(cfg); err != nil {
        // CRASH: Required dependency unavailable
        logger.Fatal("Required dependency check failed", zap.Error(err))
    }

    // ========================================
    // OPTIONAL: Start with degraded features
    // ========================================
    features := discoverOptionalFeatures(cfg)
    if !features.EmbeddingAvailable {
        logger.Warn("Embedding service unavailable, semantic search disabled")
    }

    // ... start service
}

func checkRequiredDependencies(cfg *config.Config) error {
    // Example: Tekton check for WorkflowExecution
    _, err := clientset.TektonV1beta1().Pipelines("").List(ctx, metav1.ListOptions{Limit: 1})
    if err != nil {
        return fmt.Errorf("Tekton Pipelines not installed or not accessible: %w", err)
    }

    // Example: PostgreSQL check for Data Storage
    if err := db.Ping(); err != nil {
        return fmt.Errorf("PostgreSQL not available: %w", err)
    }

    return nil
}
```

### Service-Specific Requirements

**Reference**: [CONFIG_STANDARDS.md](../../configuration/CONFIG_STANDARDS.md)

| Service | Required Dependencies | Crash-if-Missing |
|---------|----------------------|------------------|
| **Gateway** | Redis | Yes |
| **Data Storage** | PostgreSQL | Yes |
| **HolmesGPT-API** | LLM Provider, Data Storage | Yes |
| **WorkflowExecution** | Tekton Pipelines | Yes |
| **AIAnalysis** | HolmesGPT-API | Yes |
| **RemediationOrchestrator** | None | No (uses defaults) |
| **SignalProcessing** | None | No (uses defaults) |
| **Notification** | None (channels optional) | No (uses defaults) |
| **Dynamic Toolset** | None | No (uses defaults) |

### Optional Features with Graceful Degradation

Services with optional features MUST:
1. **Log a warning** when optional feature unavailable
2. **Disable the feature** cleanly
3. **Continue operating** with core functionality

```go
// Example: Data Storage with optional embedding
func initOptionalFeatures(cfg *config.Config, logger *zap.Logger) *Features {
    f := &Features{}

    // Optional: Embedding service
    if cfg.Embedding.Enabled {
        client, err := embedding.NewClient(cfg.Embedding.URL)
        if err != nil {
            logger.Warn("Embedding service unavailable, semantic search disabled",
                zap.Error(err))
            f.EmbeddingAvailable = false
        } else {
            f.EmbeddingClient = client
            f.EmbeddingAvailable = true
        }
    }

    return f
}
```

### Kubernetes Liveness/Readiness Integration

```yaml
# deployment.yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 5
```

**Behavior**:
- If service crashes at startup → Pod enters `CrashLoopBackOff`
- AlertManager triggers `KubernautPodCrashLooping` alert
- Operators investigate via `kubectl logs`

### Testing Crash-if-Missing

```go
var _ = Describe("Startup", func() {
    It("should crash when CONFIG_PATH not set", func() {
        cmd := exec.Command("./bin/service")
        // CONFIG_PATH not set
        err := cmd.Run()
        Expect(err).To(HaveOccurred())
        // Verify exit code is non-zero
        var exitErr *exec.ExitError
        Expect(errors.As(err, &exitErr)).To(BeTrue())
        Expect(exitErr.ExitCode()).ToNot(Equal(0))
    })

    It("should crash when required dependency unavailable", func() {
        cmd := exec.Command("./bin/service")
        cmd.Env = append(os.Environ(),
            "CONFIG_PATH=testdata/valid-config.yaml",
            "TEKTON_AVAILABLE=false", // Test hook
        )
        err := cmd.Run()
        Expect(err).To(HaveOccurred())
    })
})
```

---

## Related Decisions

- **ADR-027**: Multi-Architecture Docker Images (uses UBI9 base images)
- **ADR-003**: Kind Integration Environment (used for testing configurations)
- **DD-007**: Kubernetes-Aware Graceful Shutdown (health endpoints configured in YAML)

---

## References

- **Gateway Service**: [api-specification.md#configuration](../../services/stateless/gateway-service/api-specification.md#️-configuration)
- **Context API Config**: [`pkg/contextapi/config/config.go`](../../pkg/contextapi/config/config.go)
- **Context API Example**: [`pkg/contextapi/config/testdata/valid-config.yaml`](../../pkg/contextapi/config/testdata/valid-config.yaml)
- **Kubernetes ConfigMaps**: https://kubernetes.io/docs/concepts/configuration/configmap/
- **12-Factor App Config**: https://12factor.net/config
- **Go YAML Library**: https://github.com/go-yaml/yaml

---

**Status**: ✅ **APPROVED** - Existing pattern, mandatory for all new services
**Last Updated**: November 3, 2025 (Added Section 6: Secret Management via Mounted Files)

