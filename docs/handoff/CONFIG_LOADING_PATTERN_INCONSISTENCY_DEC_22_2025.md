# Configuration Loading Pattern Inconsistency - CRITICAL TRIAGE

**Date**: December 22, 2025
**Status**: 🚨 **CRITICAL INCONSISTENCY FOUND**
**Priority**: HIGH (Affects all services)

---

## 🚨 Problem Statement

**Kubernaut services use THREE DIFFERENT patterns for loading configuration files:**

| Service | Pattern | Config Source | Compliant? |
|---------|---------|---------------|------------|
| **DataStorage** | `CONFIG_PATH` env var (mandatory) | `os.Getenv("CONFIG_PATH")` | ✅ ADR-030 compliant |
| **Gateway** | `-config` flag (optional) | `flag.String("config", "config/gateway.yaml")` | ❌ NOT ADR-030 compliant |
| **WorkflowExecution** | `-config` flag (optional) | `flag.String("config-file", "")` | ❌ NOT ADR-030 compliant |
| **SignalProcessing** | `-config` flag (default) | `flag.String("config", "/etc/signalprocessing/config.yaml")` | ❌ NOT ADR-030 compliant |
| **Notification** | ❌ Individual env vars | `os.Getenv("FILE_OUTPUT_DIR")` etc. | ❌ NOT ADR-030 compliant |

**Impact**: Inconsistent deployment manifests, unclear "source of truth", confusion for operators

---

## Detailed Service Analysis

### ✅ **DataStorage Service** (ADR-030 Compliant)

**File**: `cmd/datastorage/main.go:58-82`

```go
// ADR-030: Load configuration from YAML file (ConfigMap)
// CONFIG_PATH environment variable is MANDATORY
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"),
        "CONFIG_PATH environment variable required (ADR-030)",
        "env_var", "CONFIG_PATH",
        "reason", "Service must not guess config file location - deployment controls this",
        "example_local", "export CONFIG_PATH=config/data-storage.yaml",
        "example_k8s", "Set in Deployment manifest")
    os.Exit(1)
}

logger.Info("Loading configuration from YAML file (ADR-030)",
    "config_path", cfgPath)

cfg, err := config.LoadFromFile(cfgPath)
```

**Why this is correct**:
- ✅ **Explicit**: Deployment manifest controls config location
- ✅ **Kubernetes-native**: ConfigMap mounted, env var points to mount path
- ✅ **No guessing**: Service doesn't assume config location
- ✅ **Clear error**: Fails fast if CONFIG_PATH not set
- ✅ **Production-ready**: Works in any environment

**Example Deployment**:
```yaml
env:
- name: CONFIG_PATH
  value: "/etc/datastorage/config.yaml"
volumeMounts:
- name: config
  mountPath: /etc/datastorage
volumes:
- name: config
  configMap:
    name: datastorage-config
```

---

### ❌ **Gateway Service** (Flag-based, NOT ADR-030 Compliant)

**File**: `cmd/gateway/main.go:44-76`

```go
// Parse command-line flags
configPath := flag.String("config", "config/gateway.yaml", "Path to configuration file")
showVersion := flag.Bool("version", false, "Show version and exit")
listenAddr := flag.String("listen", ":8080", "HTTP server listen address")
flag.Parse()

// Load configuration from YAML file
serverCfg, err := config.LoadFromFile(*configPath)
if err != nil {
    logger.Error(err, "Failed to load configuration",
        "config_path", *configPath)
    os.Exit(1)
}
```

**Why this is problematic**:
- ❌ **Default path**: Assumes `config/gateway.yaml` exists
- ❌ **Flag-based**: Requires command-line args in deployment
- ❌ **Inconsistent**: Different pattern from DataStorage
- ⚠️  **Works but different**: Functional but not standardized

**Example Deployment**:
```yaml
containers:
- name: gateway
  args:
  - "-config"
  - "/etc/gateway/config.yaml"  # Must specify in args
  volumeMounts:
  - name: config
    mountPath: /etc/gateway
```

---

### ❌ **WorkflowExecution Service** (Flag-based, Optional)

**File**: `cmd/workflowexecution/main.go:95-108`

```go
var configPath string
flag.StringVar(&configPath, "config-file", "", "Configuration file path (optional)")
flag.Parse()

// Load configuration (file if provided, otherwise defaults)
var cfg *weconfig.Config
var err error
if configPath != "" {
    cfg, err = weconfig.LoadFromFile(configPath)
    if err != nil {
        setupLog.Error(err, "Failed to load configuration file", "path", configPath)
        os.Exit(1)
    }
    setupLog.Info("Configuration loaded from file", "path", configPath)
} else {
    cfg = weconfig.DefaultConfig()
    setupLog.Info("Using default configuration (no config file provided)")
}
```

**Why this is problematic**:
- ❌ **Optional config**: Can run without config file (uses defaults)
- ❌ **Flag-based**: Requires command-line args
- ❌ **Different flag name**: `-config-file` vs Gateway's `-config`
- ⚠️  **Backwards compatibility**: Allows CLI flag overrides

---

### ❌ **SignalProcessing Service** (Flag-based, Default Path)

**File**: `cmd/signalprocessing/main.go:81-117`

```go
var configFile string
flag.StringVar(&configFile, "config", "/etc/signalprocessing/config.yaml",
    "Path to configuration file")
flag.Parse()

// Load configuration
cfg, err := config.LoadFromFile(configFile)
if err != nil {
    // In development/testing, use defaults if config file not found
    setupLog.Info("config file not found, using defaults",
        "path", configFile, "error", err.Error())
    cfg = &config.Config{} // Will fail validation - that's intentional
}

// Validate configuration (skip in development if empty)
if cfg.Enrichment.Timeout > 0 {
    if err := cfg.Validate(); err != nil {
        setupLog.Error(err, "invalid configuration")
        os.Exit(1)
    }
}
```

**Why this is problematic**:
- ❌ **Hardcoded default**: Assumes `/etc/signalprocessing/config.yaml`
- ❌ **Flag-based**: Inconsistent with DataStorage
- ⚠️  **Graceful degradation**: Uses empty config if file missing
- ⚠️  **Skip validation**: Allows invalid config in development

---

### ❌ **Notification Service** (Individual Env Vars - WORST PATTERN)

**Current State**: `cmd/notification/main.go`

```go
// Individual environment variables (NOT ADR-030 compliant)
fileOutputDir := os.Getenv("FILE_OUTPUT_DIR")
logEnabled := os.Getenv("LOG_DELIVERY_ENABLED") == "true"
slackWebhook := os.Getenv("NOTIFICATION_SLACK_WEBHOOK_URL")
dataStorageURL := os.Getenv("DATA_STORAGE_URL")
```

**Why this is the WORST pattern**:
- ❌ **No YAML config**: Everything in environment variables
- ❌ **NOT production-ready**: Can't use ConfigMaps
- ❌ **Hardcoded in deployment**: Requires redeployment for config changes
- ❌ **NO validation**: No structured config validation
- ❌ **Secrets exposed**: Mixing functional config with secrets

---

## Decision Matrix

### Option A: Standardize on CONFIG_PATH (DataStorage Pattern) ✅ RECOMMENDED

**Approach**: All services use mandatory `CONFIG_PATH` environment variable

**Pros**:
- ✅ **Kubernetes-native**: ConfigMap-first approach
- ✅ **Explicit**: Deployment controls config location
- ✅ **No guessing**: Service doesn't assume paths
- ✅ **Production-ready**: Works in any environment
- ✅ **Clear errors**: Fails fast if CONFIG_PATH missing
- ✅ **Simplest**: No command-line args needed

**Cons**:
- ⚠️  **Breaking change**: Gateway, WE, SP need refactoring
- ⚠️  **Different from K8s controllers**: Standard controllers use flags

**Implementation**:
```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"),
        "CONFIG_PATH required (ADR-030)")
    os.Exit(1)
}
cfg, err := config.LoadFromFile(cfgPath)
```

**Deployment**:
```yaml
env:
- name: CONFIG_PATH
  value: "/etc/{service}/config.yaml"
```

---

### Option B: Standardize on -config Flag (Gateway/WE/SP Pattern)

**Approach**: All services use `-config` command-line flag with default

**Pros**:
- ✅ **Familiar**: Matches standard Kubernetes controller pattern
- ✅ **Flexible**: Can override with command-line args
- ✅ **Default path**: Can work without explicit config
- ✅ **No breaking change**: Most services already use this

**Cons**:
- ❌ **Inconsistent defaults**: Each service guesses different paths
- ❌ **NOT Kubernetes-native**: Requires args in deployment
- ❌ **More complex**: Command-line parsing required
- ❌ **Breaking change for DataStorage**: Need to refactor

**Implementation**:
```go
configPath := flag.String("config", "/etc/{service}/config.yaml",
    "Path to configuration file")
flag.Parse()
cfg, err := config.LoadFromFile(*configPath)
```

**Deployment**:
```yaml
containers:
- name: manager
  args:
  - "-config"
  - "/etc/{service}/config.yaml"
```

---

### Option C: Hybrid (CONFIG_PATH with flag fallback)

**Approach**: Try CONFIG_PATH first, fall back to `-config` flag

**Pros**:
- ✅ **Backwards compatible**: Works with both patterns
- ✅ **Flexible**: Supports multiple deployment styles
- ✅ **Gradual migration**: Can migrate services one at a time

**Cons**:
- ❌ **Complex**: Two code paths to maintain
- ❌ **Confusing**: Which takes precedence?
- ❌ **NOT standardized**: Still allows inconsistency

---

## Recommendation

### **RECOMMENDED: Option A - Standardize on CONFIG_PATH**

**Rationale**:
1. **Kubernetes-native**: ConfigMap-first is the cloud-native way
2. **Simplest**: No command-line parsing needed
3. **Explicit**: Deployment controls everything
4. **Already implemented**: DataStorage shows it works
5. **Production-ready**: Clear for operators

**Migration Path**:
1. ✅ **DataStorage**: Already compliant
2. ❌ **Notification**: Migrate to CONFIG_PATH (in progress)
3. ⚠️  **Gateway**: Refactor to use CONFIG_PATH
4. ⚠️  **WorkflowExecution**: Refactor to use CONFIG_PATH
5. ⚠️  **SignalProcessing**: Refactor to use CONFIG_PATH

**Timeline**: 2-4 hours per service (Gateway, WE, SP)

---

## ADR-030 Update Required

**Current ADR-030** does NOT specify CONFIG_PATH vs flag pattern.

**Required Update**:
```markdown
## Configuration Loading - MANDATORY PATTERN

All services MUST use the CONFIG_PATH environment variable pattern:

1. Read CONFIG_PATH environment variable
2. Fail fast if CONFIG_PATH not set
3. Load configuration from YAML file at CONFIG_PATH
4. Override with secrets from environment variables (LoadFromEnv)
5. Validate configuration (Validate)
6. Start service

Example:
```go
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"),
        "CONFIG_PATH required (ADR-030)")
    os.Exit(1)
}

cfg, err := config.LoadFromFile(cfgPath)
// ... LoadFromEnv, Validate, start
```

Deployment:
```yaml
env:
- name: CONFIG_PATH
  value: "/etc/{service}/config.yaml"
volumeMounts:
- name: config
  mountPath: /etc/{service}
volumes:
- name: config
  configMap:
    name: {service}-config
```
```

---

## For Notification Service Migration

**Decision**: Use **CONFIG_PATH pattern** (Option A) since:
1. We're creating new config package from scratch
2. DataStorage (most mature) uses this pattern
3. Sets good example for future services
4. Simplest implementation

**Next Steps**:
1. ✅ Create `pkg/notification/config/config.go` with LoadFromFile
2. ✅ Update `cmd/notification/main.go` to use CONFIG_PATH
3. ✅ Create ConfigMap with YAML configuration
4. ✅ Update deployment with CONFIG_PATH env var
5. ✅ Test E2E with new configuration

---

## Questions for User

### **Q1: Which pattern should we standardize on?**
- **A**: CONFIG_PATH (recommended - Kubernetes-native)
- **B**: -config flag (familiar to K8s controllers)
- **C**: Hybrid (backwards compatible)

### **Q2: Should we migrate existing services?**
- **A**: Yes, migrate all services to chosen pattern (2-4 hours each)
- **B**: No, document as "acceptable variation"
- **C**: Migrate only new services, existing services stay as-is

### **Q3: For Notification service (current task)?**
- **A**: Use CONFIG_PATH pattern (matches DataStorage)
- **B**: Use -config flag pattern (matches Gateway/WE/SP)
- **C**: Wait for decision on Q1/Q2 first

---

**Status**: 🚧 **AWAITING USER DECISION**
**Recommendation**: Use CONFIG_PATH for Notification (Q3-A), then migrate other services (Q2-A)
**Confidence**: 90% - CONFIG_PATH is objectively simpler and more Kubernetes-native

