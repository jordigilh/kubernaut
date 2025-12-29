# ADR-030 Configuration Management Violations - Fix Guide

**Date**: December 28, 2025
**Issue**: Non-compliance with ADR-030 Configuration Management Standard
**Severity**: ⚠️ **ARCHITECTURAL VIOLATION - MANDATORY FIX**
**Affected Services**: 2 services (DataStorage, AIAnalysis)
**Document For**: Service team leads to implement fixes

---

## 🎯 Executive Summary

**ADR-030 Requirement**: All services MUST use:
1. **Command-line flag** (`-config`) for configuration file path
2. **YAML ConfigMap** as source of truth for functional configuration
3. **Environment variables** ONLY for secrets (never for functional config)

**Current Status**:
- ✅ **5 services compliant**: Gateway, SignalProcessing, WorkflowExecution, RemediationOrchestrator, Notification
- ❌ **2 services NON-COMPLIANT**: DataStorage, AIAnalysis
- ⚠️ **1 service (HolmesGPT-API)**: Will be fixed by HAPI team separately

**This Document**: Detailed fix instructions for DataStorage and AIAnalysis teams

---

## Why This Matters

### Problems with Current Approach (Environment Variables)

1. **Kubernetes Anti-Pattern**
   - ConfigMaps exist specifically for configuration
   - Environment variables should be for secrets only
   - Mixing concerns makes configuration management harder

2. **Debugging Difficulty**
   - Config hidden in pod spec environment variables
   - Cannot easily inspect effective configuration
   - `kubectl describe pod` doesn't show full config
   - Hard to troubleshoot config issues in production

3. **Deployment Inflexibility**
   - Cannot override config without modifying deployment
   - Testing different configurations requires deployment changes
   - Config spread across multiple env vars (AIAnalysis)

4. **Operational Issues**
   - No single source of truth for configuration
   - Typos in env var names fail silently
   - Harder to version control effective configuration

### Benefits of ADR-030 Compliant Approach

1. **Kubernetes Native**
   - ✅ ConfigMaps designed for configuration
   - ✅ Can update ConfigMap without redeploying
   - ✅ `kubectl get configmap` shows full config
   - ✅ Clear separation: ConfigMaps for config, Secrets for secrets

2. **Operational Excellence**
   - ✅ `kubectl exec cat /etc/service/config.yaml` shows effective config
   - ✅ Single YAML file = single source of truth
   - ✅ Easy to review, version control, and audit
   - ✅ Clear configuration contract

3. **Development Velocity**
   - ✅ Override flag at deployment time for testing
   - ✅ No need to modify deployment for config changes
   - ✅ Clear args visible in `kubectl describe pod`

---

## 📋 Service-Specific Fix Instructions

---

## Service 1: DataStorage ❌

**Team**: DataStorage Team
**Priority**: 🔴 **CRITICAL - HIGH PRIORITY**
**Reason**: 6 other services depend on DataStorage
**Estimated Effort**: ~30 minutes
**Complexity**: LOW (simple change)

### Current Violation

**File**: `cmd/datastorage/main.go:61`

```go
// ❌ CURRENT (ADR-030 VIOLATION)
cfgPath := os.Getenv("CONFIG_PATH")
if cfgPath == "" {
    logger.Error(fmt.Errorf("CONFIG_PATH not set"),
        "CONFIG_PATH environment variable required (ADR-030)")
    os.Exit(1)
}

cfg, err := config.LoadFromFile(cfgPath)
```

**Problem**: Requires `CONFIG_PATH` environment variable instead of command-line flag

**Current Kubernetes Deployment**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    env:
    - name: CONFIG_PATH  # ❌ BAD: Config path in env var
      value: /etc/datastorage/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/datastorage
```

---

### Required Fix

#### Step 1: Update `cmd/datastorage/main.go`

**Location**: Lines 48-70

**REPLACE**:
```go
func main() {
	// Initialize logger first (before config loading for error reporting)
	// DD-005 v2.0: Use pkg/log shared library with logr interface
	logger := kubelog.NewLogger(kubelog.Options{
		Development: os.Getenv("ENV") != "production",
		Level:       0, // Info level
		ServiceName: "datastorage",
	})
	defer kubelog.Sync(logger)

	// ADR-030: Load configuration from YAML file (ConfigMap)
	// CONFIG_PATH environment variable is MANDATORY
	// Deployment/environment is responsible for setting this
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		logger.Error(fmt.Errorf("CONFIG_PATH not set"), "CONFIG_PATH environment variable required (ADR-030)",
			"env_var", "CONFIG_PATH",
			"reason", "Service must not guess config file location - deployment controls this",
			"example_local", "export CONFIG_PATH=config/data-storage.yaml",
			"example_k8s", "Set in Deployment manifest",
		)
		os.Exit(1)
	}

	logger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", cfgPath,
	)

	cfg, err := config.LoadFromFile(cfgPath)
```

**WITH**:
```go
func main() {
	// ========================================
	// ADR-030: Configuration Management
	// MANDATORY: Use -config flag with K8s env substitution
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config",
		"/etc/datastorage/config.yaml",
		"Path to configuration file (ADR-030)")
	flag.Parse()

	// Initialize logger first (before config loading for error reporting)
	// DD-005 v2.0: Use pkg/log shared library with logr interface
	logger := kubelog.NewLogger(kubelog.Options{
		Development: os.Getenv("ENV") != "production",
		Level:       0, // Info level
		ServiceName: "datastorage",
	})
	defer kubelog.Sync(logger)

	logger.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", configPath)

	// ADR-030: Load configuration from YAML file
	cfg, err := config.LoadFromFile(configPath)
```

**Key Changes**:
1. ✅ Import `flag` package
2. ✅ Use `flag.StringVar()` for `-config` flag (NOT `config-file` or other names)
3. ✅ Default path: `/etc/datastorage/config.yaml`
4. ✅ Call `flag.Parse()` before using `configPath`
5. ✅ Remove `CONFIG_PATH` environment variable logic
6. ✅ Update log message to use `configPath` variable

#### Step 2: Add Import

**Location**: Top of file

**ADD**:
```go
import (
	"context"
	"flag"  // ✅ ADD THIS
	"fmt"
	// ... rest of imports
)
```

#### Step 3: Update Integration Test Infrastructure

**File**: `test/infrastructure/datastorage_bootstrap.go`

**Find and UPDATE** (search for `podman run` commands):

**BEFORE**:
```go
"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
```

**AFTER**:
```go
// Remove the CONFIG_PATH env var line completely
// DataStorage now uses -config flag
```

**File**: `test/infrastructure/shared_integration_utils.go`

**Location**: Around line 459

**REPLACE**:
```go
runArgs = append(runArgs,
	"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
	"-v", fmt.Sprintf("%s:/etc/datastorage-secrets:ro", configDir),
	"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
)
```

**WITH**:
```go
runArgs = append(runArgs,
	"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
	"-v", fmt.Sprintf("%s:/etc/datastorage-secrets:ro", configDir),
	// ADR-030: Use -config flag instead of CONFIG_PATH env var
	"-config", "/etc/datastorage/config.yaml",
)
```

#### Step 4: Update Kubernetes Manifests

**All deployment files** (search for `datastorage` deployments):

**BEFORE**:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    env:
    - name: CONFIG_PATH  # ❌ REMOVE THIS
      value: /etc/datastorage/config.yaml
    volumeMounts:
    - name: config
      mountPath: /etc/datastorage
```

**AFTER**:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  containers:
  - name: datastorage
    # ADR-030: Define CONFIG_PATH env var for Kubernetes substitution
    env:
    - name: CONFIG_PATH
      value: "/etc/datastorage/config.yaml"

    # ADR-030: Use -config flag with $(CONFIG_PATH) substitution
    args:
    - "-config"
    - "$(CONFIG_PATH)"  # ✅ Kubernetes substitutes this

    volumeMounts:
    - name: config
      mountPath: /etc/datastorage
      readOnly: true
```

**Key Points**:
- ✅ Keep `CONFIG_PATH` env var (used for K8s substitution)
- ✅ Add `args:` section with `-config` flag
- ✅ Use `$(CONFIG_PATH)` for Kubernetes env var substitution
- ✅ Add `readOnly: true` to volume mount

#### Step 5: Update Dockerfile

**File**: `docker/data-storage.Dockerfile`

**No changes needed** - the binary now accepts `-config` flag, which will be passed via `args:` in Kubernetes

---

### Testing the Fix

#### Test 1: Local Build and Run

```bash
# Build
cd /path/to/kubernaut
go build -o datastorage ./cmd/datastorage/

# Test with custom config path
./datastorage -config=/tmp/test-config.yaml

# Test with default path
./datastorage  # Should use /etc/datastorage/config.yaml
```

#### Test 2: Integration Tests

```bash
# Run DataStorage integration tests
ginkgo -v ./test/integration/datastorage/

# Should pass with new -config flag approach
```

#### Test 3: Kubernetes Deployment

```bash
# Apply updated manifest
kubectl apply -f manifests/datastorage-deployment.yaml

# Verify args are correct
kubectl describe pod -l app=datastorage | grep -A 5 "Args:"
# Should show:
#   Args:
#     -config
#     /etc/datastorage/config.yaml

# Verify config is loaded
kubectl logs -l app=datastorage | grep "Loading configuration"
# Should show: config_path=/etc/datastorage/config.yaml

# Verify service is healthy
kubectl exec datastorage-xxx -- cat /etc/datastorage/config.yaml
```

---

### Files to Update

| File | Action | Lines |
|------|--------|-------|
| `cmd/datastorage/main.go` | Modify | ~48-70 |
| `test/infrastructure/datastorage_bootstrap.go` | Modify | Search for CONFIG_PATH |
| `test/infrastructure/shared_integration_utils.go` | Modify | ~459 |
| All K8s deployment manifests | Modify | Search for datastorage |

---

### Verification Checklist

- [ ] `flag` package imported in `main.go`
- [ ] `-config` flag defined with default `/etc/datastorage/config.yaml`
- [ ] `flag.Parse()` called before using `configPath`
- [ ] All references to `os.Getenv("CONFIG_PATH")` removed
- [ ] Integration test infrastructure updated
- [ ] Kubernetes manifests updated with `args:` section
- [ ] Local build test passes
- [ ] Integration tests pass
- [ ] Kubernetes deployment works

---

## Service 2: AIAnalysis ❌

**Team**: AIAnalysis Team
**Priority**: 🔴 **HIGH PRIORITY**
**Reason**: Architectural issue - NO config file at all
**Estimated Effort**: ~2-3 hours
**Complexity**: MEDIUM (requires creating config infrastructure)

### Current Violation

**File**: `cmd/aianalysis/main.go:64-79`

```go
// ❌ CURRENT (ADR-030 VIOLATION - Multiple env vars, NO config file)
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var holmesGPTURL string
	var holmesGPTTimeout time.Duration
	var regoPolicyPath string
	var dataStorageURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "...")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "...")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "...")
	flag.StringVar(&holmesGPTURL, "holmesgpt-api-url",
		getEnvOrDefault("HOLMESGPT_API_URL", "http://holmesgpt-api:8080"), "...")
	flag.StringVar(&regoPolicyPath, "rego-policy-path",
		getEnvOrDefault("REGO_POLICY_PATH", "/etc/kubernaut/policies/approval.rego"), "...")
	flag.StringVar(&dataStorageURL, "datastorage-url",
		getEnvOrDefault("DATASTORAGE_URL", "http://datastorage:8080"), "...")
```

**Problems**:
1. ❌ NO config file - uses environment variables directly
2. ❌ Configuration scattered across multiple env vars
3. ❌ No single source of truth
4. ❌ Not ADR-030 compliant

**Current Kubernetes Deployment**:
```yaml
apiVersion: v1
kind: Deployment
spec:
  containers:
  - name: aianalysis
    env:  # ❌ BAD: Config spread across env vars
    - name: HOLMESGPT_API_URL
      value: http://holmesgpt-api:8080
    - name: REGO_POLICY_PATH
      value: /etc/kubernaut/policies/approval.rego
    - name: DATASTORAGE_URL
      value: http://datastorage:8080
```

---

### Required Fix

This is a larger fix requiring config infrastructure creation.

#### Step 1: Create Config Package

**NEW FILE**: `pkg/aianalysis/config/config.go`

```go
/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package config provides configuration management for AIAnalysis service
// Per ADR-030: Configuration Management Standard
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents AIAnalysis service configuration
// ADR-030: Mandatory three-section structure
type Config struct {
	// MANDATORY: Controller runtime settings
	Controller ControllerSettings `yaml:"controller"`

	// SERVICE-SPECIFIC: AIAnalysis business logic settings
	Analysis AnalysisSettings `yaml:"analysis"`

	// MANDATORY: External service dependencies
	Infrastructure InfrastructureSettings `yaml:"infrastructure"`
}

// ControllerSettings configures controller-runtime manager
// ADR-030: Standard across all controller-based services
type ControllerSettings struct {
	MetricsAddr        string `yaml:"metrics_addr"`          // Prometheus metrics endpoint
	HealthProbeAddr    string `yaml:"health_probe_addr"`     // Health/readiness probes
	LeaderElection     bool   `yaml:"leader_election"`       // Enable for HA
	LeaderElectionID   string `yaml:"leader_election_id"`    // Unique election ID
}

// AnalysisSettings configures AIAnalysis business logic
type AnalysisSettings struct {
	// HolmesGPT API configuration
	HolmesGPTURL     string        `yaml:"holmesgpt_api_url"`     // HolmesGPT-API base URL
	HolmesGPTTimeout time.Duration `yaml:"holmesgpt_api_timeout"` // Request timeout

	// Rego policy configuration
	RegoPolicyPath string `yaml:"rego_policy_path"` // Path to approval policy
}

// InfrastructureSettings configures external service dependencies
type InfrastructureSettings struct {
	DataStorageURL string `yaml:"datastorage_url"` // Data Storage Service URL for audit
}

// LoadFromFile loads configuration from YAML file
// ADR-030 MANDATORY: Primary configuration loading method
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
// ADR-030: ONLY for secrets, NOT for functional configuration
func (c *Config) LoadFromEnv() {
	// ADR-030: Environment variables ONLY for secrets
	// Currently no secrets in AIAnalysis config
	// Add here if needed in the future (e.g., API keys)
}

// Validate checks configuration for errors
// ADR-030 MANDATORY: Fail-fast validation before service starts
func (c *Config) Validate() error {
	// Controller validation
	if c.Controller.MetricsAddr == "" {
		return fmt.Errorf("controller.metrics_addr is required")
	}
	if c.Controller.HealthProbeAddr == "" {
		return fmt.Errorf("controller.health_probe_addr is required")
	}

	// Analysis validation
	if c.Analysis.HolmesGPTURL == "" {
		return fmt.Errorf("analysis.holmesgpt_api_url is required")
	}
	if c.Analysis.RegoPolicyPath == "" {
		return fmt.Errorf("analysis.rego_policy_path is required")
	}
	if c.Analysis.HolmesGPTTimeout <= 0 {
		return fmt.Errorf("analysis.holmesgpt_api_timeout must be positive")
	}

	// Infrastructure validation
	if c.Infrastructure.DataStorageURL == "" {
		return fmt.Errorf("infrastructure.datastorage_url is required")
	}

	return nil
}

// applyDefaults sets default values for missing configuration
// ADR-030: Provide sensible defaults
func (c *Config) applyDefaults() {
	// Controller defaults
	if c.Controller.MetricsAddr == "" {
		c.Controller.MetricsAddr = ":9090"
	}
	if c.Controller.HealthProbeAddr == "" {
		c.Controller.HealthProbeAddr = ":8081"
	}
	if c.Controller.LeaderElectionID == "" {
		c.Controller.LeaderElectionID = "aianalysis.kubernaut.ai"
	}

	// Analysis defaults
	if c.Analysis.HolmesGPTURL == "" {
		c.Analysis.HolmesGPTURL = "http://holmesgpt-api:8080"
	}
	if c.Analysis.HolmesGPTTimeout == 0 {
		c.Analysis.HolmesGPTTimeout = 60 * time.Second
	}
	if c.Analysis.RegoPolicyPath == "" {
		c.Analysis.RegoPolicyPath = "/etc/kubernaut/policies/approval.rego"
	}

	// Infrastructure defaults
	if c.Infrastructure.DataStorageURL == "" {
		c.Infrastructure.DataStorageURL = "http://datastorage:8080"
	}
}
```

#### Step 2: Create Default Config File

**NEW FILE**: `config/aianalysis.yaml`

```yaml
# AIAnalysis Service Configuration
# Per ADR-030: Configuration Management Standard
#
# This file is the source of truth for AIAnalysis configuration
# Mount this via ConfigMap in Kubernetes

# MANDATORY: Controller runtime settings
controller:
  metrics_addr: ":9090"              # Prometheus metrics endpoint
  health_probe_addr: ":8081"         # Health/readiness probes
  leader_election: false             # Enable for HA deployments
  leader_election_id: "aianalysis.kubernaut.ai"

# SERVICE-SPECIFIC: AIAnalysis business logic
analysis:
  # HolmesGPT API configuration
  holmesgpt_api_url: "http://holmesgpt-api:8080"
  holmesgpt_api_timeout: 60s         # Request timeout

  # Rego policy configuration
  rego_policy_path: "/etc/kubernaut/policies/approval.rego"

# MANDATORY: External service dependencies
infrastructure:
  datastorage_url: "http://datastorage:8080"
```

#### Step 3: Update `cmd/aianalysis/main.go`

**Location**: Lines 64-106

**REPLACE**:
```go
func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var holmesGPTURL string
	var holmesGPTTimeout time.Duration
	var regoPolicyPath string
	var dataStorageURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "...")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "...")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "...")
	flag.StringVar(&holmesGPTURL, "holmesgpt-api-url",
		getEnvOrDefault("HOLMESGPT_API_URL", "http://holmesgpt-api:8080"), "...")
	flag.DurationVar(&holmesGPTTimeout, "holmesgpt-api-timeout", 60*time.Second, "...")
	flag.StringVar(&regoPolicyPath, "rego-policy-path",
		getEnvOrDefault("REGO_POLICY_PATH", "/etc/kubernaut/policies/approval.rego"), "...")
	flag.StringVar(&dataStorageURL, "datastorage-url",
		getEnvOrDefault("DATASTORAGE_URL", "http://datastorage:8080"), "...")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information at startup
	setupLog.Info("Starting AI Analysis Controller",
		"version", Version,
		"gitCommit", GitCommit,
		"buildTime", BuildTime,
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "aianalysis.kubernaut.ai",
	})
```

**WITH**:
```go
func main() {
	// ========================================
	// ADR-030: Configuration Management
	// MANDATORY: Use -config flag with K8s env substitution
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config",
		"/etc/aianalysis/config.yaml",
		"Path to configuration file (ADR-030)")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information at startup
	setupLog.Info("Starting AI Analysis Controller",
		"version", Version,
		"gitCommit", GitCommit,
		"buildTime", BuildTime,
	)

	// ADR-030: Load configuration from YAML file
	setupLog.Info("Loading configuration from YAML file (ADR-030)",
		"config_path", configPath)

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration file (ADR-030)",
			"config_path", configPath)
		os.Exit(1)
	}

	// ADR-030: Override with environment variables (secrets only)
	cfg.LoadFromEnv()

	// ADR-030: Validate configuration (fail-fast)
	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "Invalid configuration (ADR-030)")
		os.Exit(1)
	}

	setupLog.Info("Configuration loaded successfully (ADR-030)",
		"service", "aianalysis",
		"metrics_addr", cfg.Controller.MetricsAddr,
		"health_probe_addr", cfg.Controller.HealthProbeAddr,
		"holmesgpt_api_url", cfg.Analysis.HolmesGPTURL,
		"datastorage_url", cfg.Infrastructure.DataStorageURL)

	// ADR-030: Use configuration values for controller manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.Controller.MetricsAddr,
		},
		HealthProbeBindAddress: cfg.Controller.HealthProbeAddr,
		LeaderElection:         cfg.Controller.LeaderElection,
		LeaderElectionID:       cfg.Controller.LeaderElectionID,
	})
```

**Continue updating main.go** - find where variables are used:

**Line ~112-115** (HolmesGPT client creation):
```go
// BEFORE
setupLog.Info("Creating HolmesGPT-API client (generated)", "url", holmesGPTURL)
holmesGPTClient := client.NewHolmesGPTClient(client.Config{
	BaseURL: holmesGPTURL,
})

// AFTER
setupLog.Info("Creating HolmesGPT-API client (generated)", "url", cfg.Analysis.HolmesGPTURL)
holmesGPTClient := client.NewHolmesGPTClient(client.Config{
	BaseURL: cfg.Analysis.HolmesGPTURL,
})
```

**Line ~123-126** (Rego evaluator creation):
```go
// BEFORE
setupLog.Info("Creating Rego evaluator", "policyPath", regoPolicyPath)
regoEvaluator := rego.NewEvaluator(rego.Config{
	PolicyPath: regoPolicyPath,
}, ctrl.Log.WithName("rego"))

// AFTER
setupLog.Info("Creating Rego evaluator", "policyPath", cfg.Analysis.RegoPolicyPath)
regoEvaluator := rego.NewEvaluator(rego.Config{
	PolicyPath: cfg.Analysis.RegoPolicyPath,
}, ctrl.Log.WithName("rego"))
```

**Search for `dataStorageURL` variable** and replace with `cfg.Infrastructure.DataStorageURL`

#### Step 4: Add Config Import

**Location**: Top of `cmd/aianalysis/main.go`

**ADD**:
```go
import (
	// ... existing imports

	// ADR-030: Configuration management
	"github.com/jordigilh/kubernaut/pkg/aianalysis/config"

	// ... rest of imports
)
```

#### Step 5: Remove `getEnvOrDefault` Function

**Location**: Bottom of `cmd/aianalysis/main.go`

**REMOVE** this entire function (no longer needed):
```go
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
```

#### Step 6: Create ConfigMap Manifest

**NEW FILE**: `manifests/aianalysis-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-config
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: aianalysis
    app.kubernetes.io/component: controller
    app.kubernetes.io/part-of: kubernaut
data:
  config.yaml: |
    # AIAnalysis Service Configuration
    # Per ADR-030: Configuration Management Standard

    controller:
      metrics_addr: ":9090"
      health_probe_addr: ":8081"
      leader_election: true  # Enable in production
      leader_election_id: "aianalysis.kubernaut.ai"

    analysis:
      holmesgpt_api_url: "http://holmesgpt-api:8080"
      holmesgpt_api_timeout: 60s
      rego_policy_path: "/etc/kubernaut/policies/approval.rego"

    infrastructure:
      datastorage_url: "http://datastorage:8080"
```

#### Step 7: Update Kubernetes Deployment

**File**: `manifests/aianalysis-deployment.yaml` (or similar)

**BEFORE**:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  containers:
  - name: aianalysis
    env:  # ❌ REMOVE ALL THESE
    - name: HOLMESGPT_API_URL
      value: http://holmesgpt-api:8080
    - name: REGO_POLICY_PATH
      value: /etc/kubernaut/policies/approval.rego
    - name: DATASTORAGE_URL
      value: http://datastorage:8080
```

**AFTER**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: manager
        image: aianalysis:latest

        # ADR-030: Define CONFIG_PATH env var for Kubernetes substitution
        env:
        - name: CONFIG_PATH
          value: "/etc/aianalysis/config.yaml"

        # ADR-030: Use -config flag with $(CONFIG_PATH) substitution
        args:
        - "-config"
        - "$(CONFIG_PATH)"

        # ADR-030: Mount ConfigMap
        volumeMounts:
        - name: config
          mountPath: /etc/aianalysis
          readOnly: true
        - name: rego-policy
          mountPath: /etc/kubernaut/policies
          readOnly: true

      # ADR-030: ConfigMap volumes
      volumes:
      - name: config
        configMap:
          name: aianalysis-config
      - name: rego-policy
        configMap:
          name: approval-policy  # Existing Rego policy ConfigMap
```

---

### Testing the Fix

#### Test 1: Build and Unit Tests

```bash
# Build config package
go build ./pkg/aianalysis/config/

# Test config loading
cat > /tmp/test-ai-config.yaml << EOF
controller:
  metrics_addr: ":9999"
  health_probe_addr: ":8888"
analysis:
  holmesgpt_api_url: "http://test:8080"
  holmesgpt_api_timeout: 30s
  rego_policy_path: "/tmp/test.rego"
infrastructure:
  datastorage_url: "http://test-ds:8080"
EOF

# Build and test
go build -o aianalysis ./cmd/aianalysis/
./aianalysis -config=/tmp/test-ai-config.yaml
```

#### Test 2: Kubernetes Deployment

```bash
# Apply ConfigMap
kubectl apply -f manifests/aianalysis-configmap.yaml

# Apply updated deployment
kubectl apply -f manifests/aianalysis-deployment.yaml

# Verify args
kubectl describe pod -l app=aianalysis | grep -A 5 "Args:"
# Should show:
#   Args:
#     -config
#     /etc/aianalysis/config.yaml

# Verify config loaded
kubectl logs -l app=aianalysis | grep "Configuration loaded"

# Verify effective config
kubectl exec aianalysis-xxx -- cat /etc/aianalysis/config.yaml
```

---

### Files to Create/Update

| File | Action | Purpose |
|------|--------|---------|
| `pkg/aianalysis/config/config.go` | **CREATE** | Config struct and loading logic |
| `config/aianalysis.yaml` | **CREATE** | Default configuration template |
| `manifests/aianalysis-configmap.yaml` | **CREATE** | Kubernetes ConfigMap |
| `cmd/aianalysis/main.go` | **MODIFY** | Use config package instead of env vars |
| All K8s deployment manifests | **MODIFY** | Add args and volume mounts |

---

### Verification Checklist

- [ ] Config package created at `pkg/aianalysis/config/config.go`
- [ ] `LoadFromFile()`, `LoadFromEnv()`, `Validate()` implemented
- [ ] Default config file created at `config/aianalysis.yaml`
- [ ] `main.go` uses `-config` flag
- [ ] All env var references removed from `main.go`
- [ ] `getEnvOrDefault()` function removed
- [ ] Config import added to `main.go`
- [ ] All variable usages updated (e.g., `cfg.Analysis.HolmesGPTURL`)
- [ ] ConfigMap manifest created
- [ ] Kubernetes deployment updated with `args:` and volumes
- [ ] Local build test passes
- [ ] Kubernetes deployment works
- [ ] Config validation works (try invalid config)

---

## 📊 Compliance Summary

### Before Fixes

| Service | Config Method | ADR-030 Compliant |
|---------|--------------|-------------------|
| Gateway | `--config` flag | ✅ YES |
| SignalProcessing | `--config` flag | ✅ YES |
| WorkflowExecution | `--config` flag | ✅ YES |
| RemediationOrchestrator | `--config` flag | ✅ YES |
| Notification | `--config` flag | ✅ YES |
| **DataStorage** | `CONFIG_PATH` env var | ❌ **NO** |
| **AIAnalysis** | Multiple env vars | ❌ **NO** |

### After Fixes

| Service | Config Method | ADR-030 Compliant |
|---------|--------------|-------------------|
| All 7 services | `--config` flag | ✅ **YES** |

---

## 🎯 Implementation Timeline

### Recommended Order

1. **DataStorage** (Day 1 - 30 minutes)
   - Simple fix, high impact
   - All other services depend on it
   - Low risk

2. **AIAnalysis** (Day 2 - 2-3 hours)
   - More complex (requires config infrastructure)
   - Medium risk
   - Can be done after DataStorage is complete

---

## 🔗 References

- **ADR-030**: Configuration Management Standard
  - File: `docs/architecture/decisions/ADR-030-CONFIGURATION-MANAGEMENT.md`
  - Mandatory patterns, examples, anti-patterns

- **Compliant Examples**:
  - Gateway: `cmd/gateway/main.go`
  - Notification: `cmd/notification/main.go` + `pkg/notification/config/`
  - SignalProcessing: `cmd/signalprocessing/main.go` + `pkg/signalprocessing/config/`

- **Kubernetes ConfigMap Documentation**:
  - https://kubernetes.io/docs/concepts/configuration/configmap/

---

## 💬 Questions or Issues?

If you encounter any issues implementing these fixes, please:

1. Review ADR-030 document for detailed requirements
2. Reference compliant services (Gateway, Notification) as examples
3. Check Kubernetes manifest syntax for `args:` and volume mounts
4. Reach out to architecture team for clarification

---

**Document Status**: ✅ **READY FOR TEAM IMPLEMENTATION**
**Date**: December 28, 2025
**Prepared By**: Architecture Team
**Distribution**: DataStorage Team, AIAnalysis Team

