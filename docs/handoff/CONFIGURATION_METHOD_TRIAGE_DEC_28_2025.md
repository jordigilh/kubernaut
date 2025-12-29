# Configuration Method Triage - 8 Services

**Date**: December 28, 2025
**Issue**: Services using environment variables for configuration instead of YAML config files with command-line flags
**Severity**: ARCHITECTURAL VIOLATION
**Impact**: 3 of 8 services (37.5%)

---

## Executive Summary

**Requirement**: All services MUST:
1. Load configuration from a YAML file mounted via ConfigMap
2. Accept the config file path via command-line flag (e.g., `--config`)
3. NOT rely on environment variables for configuration (secrets are exception)

**Results**:
- ✅ **5 services COMPLIANT** (Gateway, SignalProcessing, WorkflowExecution, RemediationOrchestrator, Notification)
- ❌ **3 services NON-COMPLIANT** (DataStorage, AIAnalysis, HolmesGPT-API)

---

## Service-by-Service Analysis

### ✅ COMPLIANT SERVICES (5/8)

#### 1. Gateway ✅

**Configuration Method**: `--config` flag

```go
// cmd/gateway/main.go:45
configPath := flag.String("config", "config/gateway.yaml", "Path to configuration file")
flag.Parse()

// Load configuration from YAML file
serverCfg, err := config.LoadFromFile(*configPath)
```

**Why Compliant**:
- Uses command-line flag for config path
- YAML file mounted via ConfigMap
- Environment variables only override secrets (via `LoadFromEnv()`)

**Kubernetes Usage**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: gateway
    args:
    - --config=/etc/gateway/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/gateway
  volumes:
  - name: config
    configMap:
      name: gateway-config
```

---

#### 2. SignalProcessing ✅

**Configuration Method**: `--config` flag

```go
// cmd/signalprocessing/main.go:87
flag.StringVar(&configFile, "config", "/etc/signalprocessing/config.yaml", "Path to configuration file")
flag.Parse()

// Load configuration
cfg, err := config.LoadFromFile(configFile)
```

**Why Compliant**:
- Uses command-line flag for config path
- YAML file mounted via ConfigMap
- Falls back to defaults if config file not found (development mode)

---

#### 3. WorkflowExecution ✅

**Configuration Method**: `--config` flag

```go
// cmd/workflowexecution/main.go:63
flag.StringVar(&configPath, "config", "", "Path to configuration file (optional, uses defaults if not provided)")
flag.Parse()

if configPath != "" {
    cfg, err = weconfig.LoadFromFile(configPath)
}
```

**Why Compliant**:
- Uses command-line flag for config path
- YAML file mounted via ConfigMap
- Supports CLI flag overrides for backwards compatibility (acceptable)

---

#### 4. RemediationOrchestrator ✅

**Configuration Method**: `--config` flag

```go
// cmd/remediationorchestrator/main.go:79
flag.StringVar(&configPath, "config", "", "Path to YAML configuration file (optional, falls back to defaults)")
flag.Parse()

cfg, err := config.LoadFromFile(configPath)
```

**Why Compliant**:
- Uses command-line flag for config path
- YAML file mounted via ConfigMap
- Graceful degradation to defaults if config file not found

---

#### 5. Notification ✅

**Configuration Method**: `--config` flag

```go
// cmd/notification/main.go:101
flag.StringVar(&configPath, "config",
    "/etc/notification/config.yaml",
    "Path to configuration file (ADR-030)")
flag.Parse()

// ADR-030: Load configuration from YAML file
cfg, err := notificationconfig.LoadFromFile(configPath)
```

**Why Compliant**:
- Uses command-line flag for config path (ADR-030 compliant)
- YAML file mounted via ConfigMap
- Environment variables only override secrets (via `LoadFromEnv()`)

---

### ❌ NON-COMPLIANT SERVICES (3/8)

#### 6. DataStorage ❌

**Configuration Method**: `CONFIG_PATH` environment variable

```go
// cmd/datastorage/main.go:61
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"), "CONFIG_PATH environment variable required (ADR-030)")
    os.Exit(1)
}

cfg, err := config.LoadFromFile(cfgPath)
```

**Why Non-Compliant**:
- ❌ Uses `CONFIG_PATH` environment variable instead of command-line flag
- ❌ Cannot be overridden at deployment time via args
- ❌ Harder to debug (env vars hidden in pod spec)

**Current Kubernetes Usage**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    env:
    - name: CONFIG_PATH  # ❌ BAD: Should use --config flag
      value: /etc/datastorage/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/datastorage
```

**Required Fix**:
```go
// CORRECT approach
var configPath string
flag.StringVar(&configPath, "config", "/etc/datastorage/config.yaml", "Path to configuration file")
flag.Parse()

cfg, err := config.LoadFromFile(configPath)
```

**Updated Kubernetes Usage**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    args:  # ✅ GOOD: Use args instead of env
    - --config=/etc/datastorage/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/datastorage
```

---

#### 7. AIAnalysis ❌

**Configuration Method**: Environment variables + command-line flags (NO config file)

```go
// cmd/aianalysis/main.go:76-79
flag.StringVar(&holmesGPTURL, "holmesgpt-api-url",
    getEnvOrDefault("HOLMESGPT_API_URL", "http://holmesgpt-api:8080"),
    "HolmesGPT-API base URL.")
flag.StringVar(&regoPolicyPath, "rego-policy-path",
    getEnvOrDefault("REGO_POLICY_PATH", "/etc/kubernaut/policies/approval.rego"),
    "Path to Rego approval policy file.")
flag.StringVar(&dataStorageURL, "datastorage-url",
    getEnvOrDefault("DATASTORAGE_URL", "http://datastorage:8080"),
    "Data Storage Service URL for audit events.")
```

**Why Non-Compliant**:
- ❌ NO config file at all - uses environment variables directly
- ❌ Configuration scattered across env vars and flags
- ❌ Cannot centralize configuration in a single YAML file
- ❌ Hard to manage in Kubernetes (env vars spread across deployment)

**Current Kubernetes Usage** (scattered config):
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: aianalysis
    env:  # ❌ BAD: Configuration via env vars
    - name: HOLMESGPT_API_URL
      value: http://holmesgpt-api:8080
    - name: REGO_POLICY_PATH
      value: /etc/kubernaut/policies/approval.rego
    - name: DATASTORAGE_URL
      value: http://datastorage:8080
```

**Required Fix**:

1. Create config struct:
```go
// pkg/aianalysis/config/config.go
type Config struct {
    HolmesGPTURL   string `yaml:"holmesgpt_api_url"`
    RegoPolicyPath string `yaml:"rego_policy_path"`
    DataStorageURL string `yaml:"datastorage_url"`
    // ... other fields
}

func LoadFromFile(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

2. Update main.go:
```go
// cmd/aianalysis/main.go
var configPath string
flag.StringVar(&configPath, "config", "/etc/aianalysis/config.yaml", "Path to configuration file")
flag.Parse()

cfg, err := config.LoadFromFile(configPath)
if err != nil {
    setupLog.Error(err, "failed to load configuration")
    os.Exit(1)
}
```

3. Create config.yaml:
```yaml
# config/aianalysis.yaml
holmesgpt_api_url: http://holmesgpt-api:8080
rego_policy_path: /etc/kubernaut/policies/approval.rego
datastorage_url: http://datastorage:8080
metrics_addr: ":9090"
health_probe_addr: ":8081"
```

4. Update Kubernetes:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-config
data:
  config.yaml: |
    holmesgpt_api_url: http://holmesgpt-api:8080
    rego_policy_path: /etc/kubernaut/policies/approval.rego
    datastorage_url: http://datastorage:8080
---
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: aianalysis
    args:  # ✅ GOOD
    - --config=/etc/aianalysis/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/aianalysis
  volumes:
  - name: config
    configMap:
      name: aianalysis-config
```

---

#### 8. HolmesGPT-API ❌

**Configuration Method**: `CONFIG_FILE` environment variable

```python
# holmesgpt-api/src/main.py:120
config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
```

**Why Non-Compliant**:
- ❌ Uses `CONFIG_FILE` environment variable instead of command-line argument
- ❌ Python service - should use argparse for command-line args
- ❌ Cannot be overridden at deployment time via args

**Current Kubernetes Usage**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: holmesgpt-api
    env:
    - name: CONFIG_FILE  # ❌ BAD: Should use --config arg
      value: /etc/holmesgpt/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/holmesgpt
```

**Required Fix**:

1. Add argparse:
```python
# holmesgpt-api/src/main.py
import argparse

parser = argparse.ArgumentParser(description='HolmesGPT API Service')
parser.add_argument('--config',
                    type=str,
                    default='/etc/holmesgpt/config.yaml',
                    help='Path to configuration file')
args = parser.parse_args()

config_file = args.config
```

2. Update Dockerfile CMD:
```dockerfile
# holmesgpt-api/Dockerfile
CMD ["python", "-m", "uvicorn", "src.main:app",
     "--host", "0.0.0.0", "--port", "8080",
     "--config", "/etc/holmesgpt/config.yaml"]  # Pass as arg
```

3. Update Kubernetes:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: holmesgpt-api
    args:  # ✅ GOOD
    - --config=/etc/holmesgpt/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/holmesgpt
  volumes:
  - name: config
    configMap:
      name: holmesgpt-config
```

---

## Summary Table

| Service | Config Method | Compliant | Fix Required |
|---------|--------------|-----------|--------------|
| Gateway | `--config` flag | ✅ YES | None |
| SignalProcessing | `--config` flag | ✅ YES | None |
| WorkflowExecution | `--config` flag | ✅ YES | None |
| RemediationOrchestrator | `--config` flag | ✅ YES | None |
| Notification | `--config` flag | ✅ YES | None |
| **DataStorage** | `CONFIG_PATH` env var | ❌ **NO** | **Change to --config flag** |
| **AIAnalysis** | Multiple env vars | ❌ **NO** | **Add config file + --config flag** |
| **HolmesGPT-API** | `CONFIG_FILE` env var | ❌ **NO** | **Add argparse + --config arg** |

---

## Why This Matters

### Problems with Environment Variables

1. **Debugging Difficulty**
   - Env vars hidden in pod spec
   - Harder to inspect running configuration
   - kubectl describe doesn't show effective config

2. **Deployment Complexity**
   - Cannot override config without changing deployment
   - Harder to test different configurations
   - Risk of env var typos (silent failures)

3. **Kubernetes Anti-Pattern**
   - ConfigMaps exist for configuration
   - Env vars should be for secrets only
   - Mixing concerns (config + secrets)

4. **Maintenance Issues**
   - Configuration spread across multiple env vars
   - No single source of truth
   - Harder to version control effective configuration

### Benefits of ConfigMap + Flag Approach

1. **Single Source of Truth**
   - All configuration in one YAML file
   - Easy to review and version
   - Clear configuration contract

2. **Kubernetes Native**
   - ConfigMaps designed for configuration
   - Can update ConfigMap without redeploying
   - kubectl get configmap shows full config

3. **Easy Override**
   - Can override flag at deployment time
   - Test different configs easily
   - No need to modify deployment for config changes

4. **Debugging**
   - kubectl exec cat /etc/service/config.yaml shows effective config
   - args visible in kubectl describe pod
   - Clear configuration path

---

## Recommended Fix Priority

### High Priority

1. **DataStorage** (most used service)
   - Used by 6 other services
   - Minimal fix required (just change env var to flag)
   - Impact: All services depend on it

2. **AIAnalysis** (architectural issue)
   - No config file at all
   - Needs config struct + YAML file
   - More complex fix (~2-3 hours)

### Medium Priority

3. **HolmesGPT-API** (Python service)
   - Python-specific fix (argparse)
   - Isolated service (lower impact)
   - Moderate complexity (~1 hour)

---

## Implementation Guide

### For Go Services (DataStorage)

**Before**:
```go
cfgPath := os.Getenv("CONFIG_PATH")
```

**After**:
```go
var configPath string
flag.StringVar(&configPath, "config", "/etc/datastorage/config.yaml", "Path to configuration file")
flag.Parse()
cfg, err := config.LoadFromFile(configPath)
```

### For Python Services (HolmesGPT-API)

**Before**:
```python
config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
```

**After**:
```python
import argparse
parser = argparse.ArgumentParser()
parser.add_argument('--config', default='/etc/holmesgpt/config.yaml')
args = parser.parse_args()
config_file = args.config
```

---

## Testing After Fix

### Verify Flag Works

```bash
# Test with custom config path
kubectl run test-datastorage --image=datastorage:latest \
  --rm -it --restart=Never -- \
  --config=/custom/path/config.yaml
```

### Verify ConfigMap Mount

```bash
# Check config is mounted correctly
kubectl exec datastorage-pod -- cat /etc/datastorage/config.yaml
```

### Verify Override

```yaml
# Test deployment-time override
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    args:
    - --config=/etc/datastorage/config.yaml  # Can change without rebuild
```

---

## Compliance Checklist

For each service, verify:

- [ ] Configuration loaded from YAML file
- [ ] Config path accepted via command-line flag (not env var)
- [ ] Flag has sensible default (e.g., `/etc/service/config.yaml`)
- [ ] ConfigMap mounted to contain configuration
- [ ] Environment variables ONLY used for secrets
- [ ] Documentation updated with --config flag usage
- [ ] Kubernetes manifests use `args:` not `env:` for config path

---

## References

- **ADR-030**: Service Configuration Management
- **DD-INTEGRATION-001 v2.0**: Programmatic Infrastructure Pattern
- **Kubernetes Best Practices**: ConfigMaps for Configuration

---

**Triage Status**: ✅ **COMPLETE**
**Violations Found**: 3 services (37.5%)
**Recommended Action**: Fix DataStorage first (highest impact), then AIAnalysis, then HolmesGPT-API




