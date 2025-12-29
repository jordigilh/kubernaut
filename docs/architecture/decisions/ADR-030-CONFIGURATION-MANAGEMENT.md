# ADR-030: Configuration Management Standard

**Date**: December 22, 2025
**Status**: ✅ **AUTHORITATIVE STANDARD - MANDATORY**
**Priority**: CRITICAL (Affects all services)
**Enforcement**: Non-negotiable - all services MUST comply

---

## 🎯 Mandatory Decision

**All Kubernaut services MUST follow this standardized configuration management pattern:**

1. **Command-line flag** (`-config`) for configuration file path
2. **Kubernetes env var substitution** (`$(CONFIG_PATH)`) in deployment args
3. **YAML ConfigMap** as the source of truth for functional configuration
4. **Environment variables** ONLY for secrets (never for functional config)
5. **Dedicated config package** at `pkg/{service}/config/`
6. **Validation** before service startup (fail-fast principle)
7. **Test fixtures** in `test/unit/{service}/config/testdata/` (NOT in `pkg/`)
8. **Standardized YAML structure** following the schema defined below

**NO EXCEPTIONS** - this is a foundational architectural requirement.

---

## Mandatory Pattern

### Service Code (REQUIRED)

```go
// cmd/{service}/main.go
package main

import (
    "flag"
    "os"

    "{module}/pkg/{service}/config"
    kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func main() {
    // MANDATORY: Use -config flag with sensible default
    var configPath string
    flag.StringVar(&configPath, "config",
        "/etc/{service}/config.yaml",  // Default path (Kubernetes convention)
        "Path to configuration file")

    flag.Parse()

    // MANDATORY: Initialize logger first (for config error reporting)
    logger := kubelog.NewLogger(kubelog.Options{
        Development: os.Getenv("ENV") != "production",
        Level:       0, // INFO
        ServiceName: "{service}",
    })
    defer kubelog.Sync(logger)

    logger.Info("Loading configuration", "config_path", configPath)

    // MANDATORY: Load configuration from YAML file
    cfg, err := config.LoadFromFile(configPath)
    if err != nil {
        logger.Error(err, "Failed to load configuration",
            "config_path", configPath)
        os.Exit(1)
    }

    // MANDATORY: Override with secrets from environment variables
    cfg.LoadFromEnv()

    // MANDATORY: Validate configuration (fail-fast)
    if err := cfg.Validate(); err != nil {
        logger.Error(err, "Invalid configuration")
        os.Exit(1)
    }

    logger.Info("Configuration loaded successfully",
        "metrics_addr", cfg.Controller.MetricsAddr)

    // Start service with validated configuration
    // ...
}
```

**Key Requirements**:
- ✅ MUST use flag named `config` (NOT `config-file`, `config-path`, etc.)
- ✅ MUST use default path `/etc/{service}/config.yaml`
- ✅ MUST use `kubelog.NewLogger()` (NOT `zap` directly)
- ✅ MUST log config path being loaded
- ✅ MUST call `LoadFromEnv()` for secrets
- ✅ MUST call `Validate()` before starting
- ✅ MUST exit with error if config invalid

---

### Kubernetes Deployment (REQUIRED)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {service}-controller
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: manager
        image: {service}:latest

        # MANDATORY: Define CONFIG_PATH environment variable
        env:
        - name: CONFIG_PATH
          value: "/etc/{service}/config.yaml"

        # MANDATORY: Use $(CONFIG_PATH) in args for Kubernetes substitution
        args:
        - "-config"
        - "$(CONFIG_PATH)"  # ✅ K8s substitutes this with env var value

        # MANDATORY: Mount ConfigMap as volume
        volumeMounts:
        - name: config
          mountPath: /etc/{service}
          readOnly: true

      # MANDATORY: ConfigMap volume
      volumes:
      - name: config
        configMap:
          name: {service}-config
```

**Key Requirements**:
- ✅ MUST define `CONFIG_PATH` environment variable
- ✅ MUST use `args: ["-config", "$(CONFIG_PATH)"]` for K8s env substitution
- ✅ MUST mount ConfigMap at `/etc/{service}/`
- ✅ MUST mount config volume as `readOnly: true`
- ❌ MUST NOT put functional configuration in env vars
- ❌ MUST NOT hardcode config paths in args (use $(CONFIG_PATH))

**Why This Pattern**:
1. **Flag is the interface** - service uses standard flag parsing
2. **Env var is the value** - deployment controls config location
3. **K8s substitutes** - `$(CONFIG_PATH)` replaced with env var value
4. **Single source of truth** - change CONFIG_PATH env var, everything updates
5. **Standard K8s pattern** - documented Kubernetes feature

---

## Mandatory YAML Configuration Structure

### Required Top-Level Sections

All service configurations MUST have these three top-level sections:

```yaml
# MANDATORY: Controller runtime settings
controller:
  metrics_addr: ":9090"               # Prometheus metrics endpoint
  health_probe_addr: ":8081"          # Health/readiness probes
  leader_election: false              # Enable for HA deployments
  leader_election_id: "{service}.kubernaut.ai"

# SERVICE-SPECIFIC: Business/processing logic settings
# Name this section based on service purpose:
# - delivery: (Notification)
# - processing: (Gateway, SignalProcessing)
# - execution: (WorkflowExecution)
# - storage: (DataStorage)
{service_logic}:
  # Service-specific settings
  # ...

# MANDATORY: Infrastructure dependencies
infrastructure:
  data_storage_url: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
  # Other service URLs, endpoints, etc.
```

### Controller Section (MANDATORY)

**All services MUST include**:

```yaml
controller:
  metrics_addr: string       # REQUIRED: Prometheus metrics bind address (default: ":9090")
  health_probe_addr: string  # REQUIRED: Health probe bind address (default: ":8081")
  leader_election: bool      # REQUIRED: Enable leader election (default: false)
  leader_election_id: string # REQUIRED: Leader election ID (default: "{service}.kubernaut.ai")
```

**Field Requirements**:
- `metrics_addr`: Port must not conflict with health_probe_addr
- `health_probe_addr`: Port must not conflict with metrics_addr
- `leader_election`: Set to `true` for multi-replica deployments
- `leader_election_id`: Must be unique per service, format: `{service}.kubernaut.ai`

---

### Infrastructure Section (MANDATORY)

**All services MUST include**:

```yaml
infrastructure:
  data_storage_url: string  # REQUIRED: Data Storage service URL for audit (ADR-032)
  # Optional: Other infrastructure dependencies
  # redis_url: string
  # database_url: string
  # etc.
```

**Field Requirements**:
- `data_storage_url`: MUST be a valid HTTP/HTTPS URL
- MUST include protocol (`http://` or `https://`)
- SHOULD use Kubernetes service DNS names in cluster deployments
- Example: `http://datastorage.kubernaut-system.svc.cluster.local:8080`

---

### Service-Specific Section (SERVICE-DEFINED)

**Name this section based on service purpose**:

| Service | Section Name | Purpose |
|---------|--------------|---------|
| Notification | `delivery` | Notification delivery settings |
| Gateway | `processing` | Signal processing settings |
| SignalProcessing | `processing` | Signal classification settings |
| WorkflowExecution | `execution` | Workflow execution settings |
| DataStorage | `storage` | Storage backend settings |

**Example: Notification Service**

```yaml
delivery:
  console:
    enabled: bool           # Enable console delivery

  file:
    output_dir: string      # Directory for file delivery
    format: string          # File format (json, yaml, text)
    timeout: duration       # Write timeout

  log:
    enabled: bool           # Enable structured log delivery
    format: string          # Log format (json, text)

  slack:
    webhook_url: string     # Slack webhook (from env via LoadFromEnv)
    timeout: duration       # HTTP timeout
```

---

### YAML Data Types

**Supported Types**:

```yaml
# String
key: "value"
key: value

# Integer
key: 42

# Boolean
key: true
key: false

# Duration (Go time.Duration format)
timeout: 30s
timeout: 5m
timeout: 1h
timeout: 100ms

# Array
items:
  - item1
  - item2

# Map/Object
settings:
  key1: value1
  key2: value2
```

**Duration Format** (MANDATORY):
- Must use Go duration strings: `ns`, `us`, `ms`, `s`, `m`, `h`
- Examples: `"30s"`, `"5m"`, `"100ms"`, `"1h30m"`
- Parse in Go with: `time.ParseDuration(value)`

---

## Configuration Package Structure

### Package Layout (MANDATORY)

```
pkg/{service}/
  ├── config/
  │   ├── config.go          # REQUIRED: Configuration types + loading functions
  │   └── config_test.go     # REQUIRED: Unit tests for config

test/unit/{service}/config/
  └── testdata/
      ├── valid-config.yaml      # REQUIRED: Valid configuration example
      └── invalid-config.yaml    # REQUIRED: Invalid for validation tests

cmd/{service}/
  └── main.go                # REQUIRED: Loads config via -config flag
```

---

### config.go (REQUIRED IMPLEMENTATION)

```go
// pkg/{service}/config/config.go
package config

import (
    "fmt"
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

// ========================================
// {SERVICE} SERVICE CONFIGURATION (ADR-030)
// Authority: ConfigMap {service}-config
// ========================================

// Config is the top-level configuration structure
// MANDATORY: Must have Controller, {ServiceLogic}, Infrastructure sections
type Config struct {
    Controller     ControllerSettings     `yaml:"controller"`
    {ServiceLogic} {ServiceLogic}Settings `yaml:"{service_logic}"`
    Infrastructure InfrastructureSettings `yaml:"infrastructure"`
}

// ControllerSettings contains Kubernetes controller configuration
// MANDATORY: All services must have these exact fields
type ControllerSettings struct {
    MetricsAddr      string `yaml:"metrics_addr"`       // Default: ":9090"
    HealthProbeAddr  string `yaml:"health_probe_addr"`  // Default: ":8081"
    LeaderElection   bool   `yaml:"leader_election"`    // Default: false
    LeaderElectionID string `yaml:"leader_election_id"` // Default: "{service}.kubernaut.ai"
}

// {ServiceLogic}Settings contains service-specific configuration
// SERVICE-DEFINED: Define based on business logic needs
type {ServiceLogic}Settings struct {
    // Service-specific fields
    // ...
}

// InfrastructureSettings contains external dependency configuration
// MANDATORY: Must have DataStorageURL for audit (ADR-032)
type InfrastructureSettings struct {
    DataStorageURL string `yaml:"data_storage_url"` // REQUIRED
    // Optional: Other infrastructure dependencies
}

// ========================================
// MANDATORY FUNCTIONS
// ========================================

// LoadFromFile loads configuration from YAML file
// MANDATORY: This is the primary configuration loader
func LoadFromFile(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config YAML: %w", err)
    }

    // Apply defaults for missing values
    cfg.applyDefaults()

    return &cfg, nil
}

// LoadFromEnv overrides configuration with environment variables
// MANDATORY: Only for secrets - NEVER for functional configuration
func (c *Config) LoadFromEnv() {
    // ONLY secrets (API keys, passwords, tokens)
    // NEVER functional configuration

    // Example:
    // if apiKey := os.Getenv("API_KEY"); apiKey != "" {
    //     c.Infrastructure.APIKey = apiKey
    // }
}

// Validate checks if configuration is valid
// MANDATORY: Fail-fast validation before service startup
func (c *Config) Validate() error {
    // MANDATORY: Validate Controller section
    if c.Controller.MetricsAddr == "" {
        return fmt.Errorf("controller.metrics_addr required")
    }
    if c.Controller.HealthProbeAddr == "" {
        return fmt.Errorf("controller.health_probe_addr required")
    }
    if c.Controller.LeaderElectionID == "" {
        return fmt.Errorf("controller.leader_election_id required")
    }

    // MANDATORY: Validate Infrastructure section
    if c.Infrastructure.DataStorageURL == "" {
        return fmt.Errorf("infrastructure.data_storage_url required (ADR-032)")
    }

    // SERVICE-SPECIFIC: Validate service logic settings
    // ...

    return nil
}

// applyDefaults sets default values for missing configuration
// MANDATORY: Provide sensible defaults
func (c *Config) applyDefaults() {
    // Controller defaults
    if c.Controller.MetricsAddr == "" {
        c.Controller.MetricsAddr = ":9090"
    }
    if c.Controller.HealthProbeAddr == "" {
        c.Controller.HealthProbeAddr = ":8081"
    }
    if c.Controller.LeaderElectionID == "" {
        c.Controller.LeaderElectionID = "{service}.kubernaut.ai"
    }

    // SERVICE-SPECIFIC: Apply service logic defaults
    // ...
}
```

**Required Functions**:
- ✅ `LoadFromFile(path string) (*Config, error)` - YAML loader
- ✅ `LoadFromEnv()` - Secret overrides ONLY
- ✅ `Validate() error` - Configuration validation
- ✅ `applyDefaults()` - Default values (private)

---

## Configuration Priority (MANDATORY ORDER)

**Precedence (highest to lowest)**:

1. **Command-line flag** - `./service -config /path/to/config.yaml`
2. **Kubernetes env substitution** - `args: ["-config", "$(CONFIG_PATH)"]`
3. **Default value** - `/etc/{service}/config.yaml`

**For secrets within config**:

1. **Environment variable** - `LoadFromEnv()` overrides
2. **YAML file** - Initial value (⚠️ NOT RECOMMENDED for secrets)

---

## Real-World Examples

### Example 1: Notification Service

**Config Package**: `pkg/notification/config/config.go`

```go
type Config struct {
    Controller     ControllerSettings     `yaml:"controller"`
    Delivery       DeliverySettings       `yaml:"delivery"`
    Infrastructure InfrastructureSettings `yaml:"infrastructure"`
}

type DeliverySettings struct {
    Console ConsoleSettings `yaml:"console"`
    File    FileSettings    `yaml:"file"`
    Log     LogSettings     `yaml:"log"`
    Slack   SlackSettings   `yaml:"slack"`
}

type FileSettings struct {
    OutputDir string        `yaml:"output_dir"`
    Format    string        `yaml:"format"`  // json, yaml, text
    Timeout   time.Duration `yaml:"timeout"`
}
```

**ConfigMap**: `test/e2e/notification/manifests/notification-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
data:
  config.yaml: |
    controller:
      metrics_addr: ":9090"
      health_probe_addr: ":8081"
      leader_election: false
      leader_election_id: "notification.kubernaut.ai"

    delivery:
      console:
        enabled: true

      file:
        output_dir: "/tmp/notifications"
        format: "json"
        timeout: 5s

      log:
        enabled: true
        format: "json"

      slack:
        timeout: 10s

    infrastructure:
      data_storage_url: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
```

**Deployment**: `test/e2e/notification/manifests/notification-deployment.yaml`

```yaml
containers:
- name: manager
  image: notification:latest
  env:
  - name: CONFIG_PATH
    value: "/etc/notification/config.yaml"
  - name: SLACK_WEBHOOK_URL  # Secret from env
    valueFrom:
      secretKeyRef:
        name: notification-secrets
        key: slack-webhook-url
  args:
  - "-config"
  - "$(CONFIG_PATH)"
  volumeMounts:
  - name: config
    mountPath: /etc/notification
    readOnly: true
volumes:
- name: config
  configMap:
    name: notification-config
```

---

### Example 2: Gateway Service

**Config Package**: `pkg/gateway/config/config.go`

```go
type ServerConfig struct {
    Server         ServerSettings         `yaml:"server"`
    Middleware     MiddlewareSettings     `yaml:"middleware"`
    Infrastructure InfrastructureSettings `yaml:"infrastructure"`
    Processing     ProcessingSettings     `yaml:"processing"`
}
```

**ConfigMap**: `test/e2e/gateway/gateway-deployment.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
data:
  config.yaml: |
    server:
      listen_addr: ":8080"
      read_timeout: 30s
      write_timeout: 30s

    infrastructure:
      data_storage_url: "http://datastorage.kubernaut-system.svc.cluster.local:8080"

    processing:
      deduplication:
        ttl: 5m
```

---

## Anti-Patterns (FORBIDDEN)

### ❌ **Anti-Pattern 1: Individual Environment Variables**

**DON'T DO THIS**:
```go
fileOutputDir := os.Getenv("FILE_OUTPUT_DIR")
logEnabled := os.Getenv("LOG_DELIVERY_ENABLED")
timeout := os.Getenv("TIMEOUT")
```

**WHY**: Not production-ready, can't use ConfigMaps, mixing config with secrets

**DO THIS**:
```yaml
# ConfigMap
delivery:
  file:
    output_dir: "/tmp/notifications"
  log:
    enabled: true
  timeout: 30s
```

---

### ❌ **Anti-Pattern 2: Hardcoded Configuration**

**DON'T DO THIS**:
```go
cfg := &Config{
    Port:    8080,
    Timeout: 30 * time.Second,
    URL:     "http://localhost:9090",
}
```

**WHY**: Requires recompilation to change

---

### ❌ **Anti-Pattern 3: Different Flag Names**

**DON'T DO THIS**:
```go
flag.StringVar(&configPath, "config-file", ...)  // ❌ Wrong
flag.StringVar(&configPath, "cfg", ...)          // ❌ Wrong
flag.StringVar(&configPath, "configuration", ...)// ❌ Wrong
```

**DO THIS**:
```go
flag.StringVar(&configPath, "config", ...)       // ✅ Correct
```

---

### ❌ **Anti-Pattern 4: Skip Validation**

**DON'T DO THIS**:
```go
cfg, _ := config.LoadFromFile(configPath)
// No validation - may crash at runtime
```

**DO THIS**:
```go
cfg, err := config.LoadFromFile(configPath)
if err != nil {
    logger.Error(err, "Failed to load config")
    os.Exit(1)
}
if err := cfg.Validate(); err != nil {
    logger.Error(err, "Invalid configuration")
    os.Exit(1)
}
```

---

## Compliance Checklist

Before merging configuration changes, verify ALL items:

### Code Requirements
- [ ] Config package created at `pkg/{service}/config/config.go`
- [ ] `LoadFromFile(path string) (*Config, error)` implemented
- [ ] `LoadFromEnv()` implemented (secrets ONLY)
- [ ] `Validate() error` implemented with comprehensive checks
- [ ] `applyDefaults()` implemented with sensible defaults
- [ ] `main.go` uses `-config` flag (NOT other names)
- [ ] `main.go` uses `kubelog.NewLogger()` (NOT zap directly)
- [ ] `main.go` calls `LoadFromEnv()` after `LoadFromFile()`
- [ ] `main.go` calls `Validate()` before starting service
- [ ] `main.go` exits with error if config invalid

### YAML Structure Requirements
- [ ] ConfigMap has `config.yaml` key with YAML content
- [ ] YAML has `controller` section with all required fields
- [ ] YAML has service-specific section (delivery/processing/execution/storage)
- [ ] YAML has `infrastructure` section with `data_storage_url`
- [ ] All durations use Go format (`30s`, `5m`, `1h`)
- [ ] No secrets in ConfigMap YAML

### Deployment Requirements
- [ ] Deployment defines `CONFIG_PATH` environment variable
- [ ] Deployment uses `args: ["-config", "$(CONFIG_PATH)"]`
- [ ] ConfigMap mounted at `/etc/{service}/`
- [ ] Config volume mounted as `readOnly: true`
- [ ] Secrets (if any) in environment variables with valueFrom
- [ ] No functional configuration in env vars

### Test Requirements
- [ ] Test fixtures in `test/unit/{service}/config/testdata/`
- [ ] `valid-config.yaml` exists and is complete
- [ ] `invalid-config.yaml` exists for validation tests
- [ ] Unit tests for `LoadFromFile()` success/failure
- [ ] Unit tests for `Validate()` with invalid configs

---

## Migration Timeline

### For New Services
- ✅ Implement this pattern from day one
- Timeline: Part of initial service creation

### For Existing Services

| Service | Current State | Action Required | ETA |
|---------|---------------|-----------------|-----|
| DataStorage | Env var only | Migrate to flag + K8s substitution | 30 min |
| Gateway | Flag (different style) | Standardize flag name, add K8s substitution | 20 min |
| WorkflowExecution | Flag (different name) | Rename flag to `config`, add K8s substitution | 20 min |
| SignalProcessing | Flag (different default) | Standardize default, add K8s substitution | 20 min |
| Notification | Individual env vars | Full migration (IN PROGRESS) | 2-3 hours |

**Total Migration Effort**: ~4 hours for all existing services

---

## Authoritative References

### Current Compliant Implementations
1. **DataStorage**: `pkg/datastorage/config/config.go` - Comprehensive example
2. **Gateway**: `pkg/gateway/config/config.go` - Multi-section structure
3. **Notification**: `pkg/notification/config/config.go` - NEW reference (in progress)

### Deployment Examples
1. **Gateway E2E**: `test/e2e/gateway/gateway-deployment.yaml` - ConfigMap + deployment
2. **Notification E2E**: `test/e2e/notification/manifests/` - Complete example (in progress)

### Related Decisions
- **ADR-032**: Audit Trail (requires `data_storage_url`)
- **DD-005**: Observability (metrics configuration in Controller)

---

## FAQ

### Q: Must I use exactly `-config` for the flag name?
**A**: YES. Standardization requires exact flag name: `-config`

### Q: Can I skip the Controller section if my service doesn't need it?
**A**: NO. All services MUST have Controller, ServiceLogic, Infrastructure sections.

### Q: Where do secrets go?
**A**: Environment variables ONLY. Load them in `LoadFromEnv()`, NEVER in ConfigMap.

### Q: Can I put functional config in environment variables?
**A**: NO. ONLY secrets in env vars. Functional config goes in YAML ConfigMap.

### Q: What if I need a different YAML structure?
**A**: The three top-level sections are MANDATORY. Add service-specific sub-sections as needed.

### Q: Must test fixtures be in `test/unit/` directory?
**A**: YES. NEVER in `pkg/` directory. Always `test/unit/{service}/config/testdata/`.

### Q: Can I use a different logging library?
**A**: NO. MUST use `kubelog.NewLogger()`, NOT `zap` directly.

### Q: What about backwards compatibility?
**A**: Migration plan provided above. 4 hours total for all services.

---

**Status**: ✅ **AUTHORITATIVE STANDARD - MANDATORY**
**Exceptions**: NONE - all services must comply
**Last Updated**: December 22, 2025
**Enforcement**: Non-negotiable architectural requirement
