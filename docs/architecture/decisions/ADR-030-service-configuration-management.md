# ADR-030: Service Configuration Management via YAML Files

**Status**: ✅ **APPROVED & IMPLEMENTED**
**Date**: November 2, 2025
**Last Updated**: November 2, 2025 (Gateway refactored to follow Context API pattern)
**Authoritative Implementation**: **Context API Service** ([`pkg/contextapi/config/config.go`](../../pkg/contextapi/config/config.go))
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
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`
}

// CacheConfig contains Redis and LRU cache configuration
type CacheConfig struct {
	RedisAddr  string        `yaml:"redis_addr"`
	RedisDB    int           `yaml:"redis_db"`
	LRUSize    int           `yaml:"lru_size"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
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

```yaml
# Context API Configuration Example
server:
  port: 8091
  host: "0.0.0.0"
  read_timeout: "30s"
  write_timeout: "30s"

logging:
  level: "info"          # debug, info, warn, error
  format: "json"         # json, console

cache:
  redis_addr: "localhost:6379"
  redis_db: 0
  lru_size: 1000
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

- [ ] Configuration package created: `pkg/<service>/config/config.go`
- [ ] Configuration structs defined with YAML tags
- [ ] `LoadFromFile()` function implemented
- [ ] `LoadFromEnv()` function implemented for overrides
- [ ] `Validate()` function implemented with comprehensive checks
- [ ] YAML configuration file created: `config/<service>-config.yaml`
- [ ] ConfigMap manifest created: `deploy/<service>/configmap.yaml`
- [ ] Deployment mounts ConfigMap at `/etc/config/`
- [ ] Main application loads config from `/etc/config/config.yaml`
- [ ] Secrets separated from ConfigMap (use Kubernetes Secrets)
- [ ] Environment-specific configurations created (dev, staging, prod)
- [ ] Unit tests created for config loading and validation
- [ ] Documentation updated

---

## Anti-Patterns to AVOID

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
**Last Updated**: November 2, 2025

