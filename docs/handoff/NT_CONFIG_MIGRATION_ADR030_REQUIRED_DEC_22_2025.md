# NT Config Migration to ADR-030 - REQUIRED

**Date**: December 22, 2025
**Status**: 🔴 **ACTION REQUIRED**
**Issue**: Notification service violates ADR-030 configuration standard

---

## 🚨 Problem Identified

**Current State**: Notification service uses individual environment variables
**Authoritative Standard**: ADR-030 requires YAML ConfigMap configuration
**Compliance**: ❌ **NON-COMPLIANT**

### Current (Wrong) Pattern
```yaml
env:
  - name: FILE_OUTPUT_DIR
    value: "/tmp/notifications"
  - name: LOG_DELIVERY_ENABLED
    value: "true"
  - name: NOTIFICATION_SLACK_WEBHOOK_URL
    value: "http://mock-slack:8080/webhook"
  - name: DATA_STORAGE_URL
    value: "http://datastorage..."
```

### Required (ADR-030) Pattern
```yaml
# ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-controller-config
data:
  config.yaml: |
    delivery:
      file:
        output_dir: "/tmp/notifications"
      log:
        enabled: true
      slack:
        webhook_url: "http://mock-slack..."
    audit:
      data_storage_url: "http://datastorage..."

# Deployment
env:
  - name: CONFIG_PATH
    value: "/etc/notification/config.yaml"
volumeMounts:
  - name: config
    mountPath: /etc/notification
volumes:
  - name: config
    configMap:
      name: notification-controller-config
```

---

## 📋 Required Changes

### 1. Create Config Package ✅

**File**: `pkg/notification/config/config.go`

```go
package config

import (
    "fmt"
    "os"
    "gopkg.in/yaml.v3"
)

// Config is the top-level configuration for Notification service
// Follows ADR-030 configuration management standard
type Config struct {
    Controller ControllerSettings `yaml:"controller"`
    Delivery   DeliverySettings   `yaml:"delivery"`
    Audit      AuditSettings      `yaml:"audit"`
}

type ControllerSettings struct {
    MetricsAddr      string `yaml:"metrics_addr"`       // Default: ":9090"
    HealthProbeAddr  string `yaml:"health_probe_addr"`  // Default: ":8081"
    LeaderElection   bool   `yaml:"leader_election"`    // Default: false
    LeaderElectionID string `yaml:"leader_election_id"` // Default: "notification.kubernaut.ai"
}

type DeliverySettings struct {
    Console ConsoleSettings `yaml:"console"`
    File    FileSettings    `yaml:"file"`
    Log     LogSettings     `yaml:"log"`
    Slack   SlackSettings   `yaml:"slack"`
}

type ConsoleSettings struct {
    Enabled bool `yaml:"enabled"` // Default: true
}

type FileSettings struct {
    OutputDir string `yaml:"output_dir"` // Required when file channel used
    Format    string `yaml:"format"`     // Default: "json"
}

type LogSettings struct {
    Enabled bool `yaml:"enabled"` // Default: false
}

type SlackSettings struct {
    WebhookURL string `yaml:"webhook_url"` // Optional, from env or config
}

type AuditSettings struct {
    DataStorageURL string `yaml:"data_storage_url"` // Required (ADR-032)
}

// LoadFromFile loads configuration from YAML file
func LoadFromFile(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("failed to parse config YAML: %w", err)
    }

    return &cfg, nil
}

// LoadFromEnv overrides config with environment variables (secrets only)
func (c *Config) LoadFromEnv() {
    // Secrets only (never in ConfigMap)
    if webhookURL := os.Getenv("SLACK_WEBHOOK_URL"); webhookURL != "" {
        c.Delivery.Slack.WebhookURL = webhookURL
    }
}

// Validate validates configuration
func (c *Config) Validate() error {
    if c.Audit.DataStorageURL == "" {
        return fmt.Errorf("audit.data_storage_url is required (ADR-032)")
    }
    return nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
    return &Config{
        Controller: ControllerSettings{
            MetricsAddr:      ":9090",
            HealthProbeAddr:  ":8081",
            LeaderElection:   false,
            LeaderElectionID: "notification.kubernaut.ai",
        },
        Delivery: DeliverySettings{
            Console: ConsoleSettings{Enabled: true},
            File:    FileSettings{Format: "json"},
            Log:     LogSettings{Enabled: false},
        },
    }
}
```

### 2. Update main.go ✅

**File**: `cmd/notification/main.go`

```go
func main() {
    // ADR-030: Load configuration from YAML file (ConfigMap)
    cfgPath := os.Getenv("CONFIG_PATH")
    if cfgPath == "" {
        setupLog.Error(fmt.Errorf("CONFIG_PATH not set"),
            "CONFIG_PATH environment variable required (ADR-030)")
        os.Exit(1)
    }

    cfg, err := notificationconfig.LoadFromFile(cfgPath)
    if err != nil {
        setupLog.Error(err, "Failed to load configuration", "path", cfgPath)
        os.Exit(1)
    }

    // ADR-030: Override with environment variables (secrets only)
    cfg.LoadFromEnv()

    // Validate configuration
    if err := cfg.Validate(); err != nil {
        setupLog.Error(err, "Invalid configuration")
        os.Exit(1)
    }

    setupLog.Info("Configuration loaded",
        "metrics_addr", cfg.Controller.MetricsAddr,
        "data_storage_url", cfg.Audit.DataStorageURL)

    // Use cfg.Delivery.File.OutputDir instead of os.Getenv("FILE_OUTPUT_DIR")
    // Use cfg.Delivery.Log.Enabled instead of os.Getenv("LOG_DELIVERY_ENABLED")
    // etc.
}
```

### 3. Create ConfigMap ✅

**File**: `test/e2e/notification/manifests/notification-configmap.yaml`

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-controller-config
  namespace: notification-e2e
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

      log:
        enabled: true

      slack:
        webhook_url: "http://mock-slack:8080/webhook"

    audit:
      data_storage_url: "http://datastorage.notification-e2e.svc.cluster.local:8080"
```

### 4. Update Deployment ✅

**File**: `test/e2e/notification/manifests/notification-deployment.yaml`

**Remove** env vars block, **Add**:

```yaml
containers:
- name: manager
  env:
  - name: CONFIG_PATH
    value: "/etc/notification/config.yaml"
  volumeMounts:
  - name: config
    mountPath: /etc/notification
    readOnly: true
  - name: notification-output
    mountPath: /tmp/notifications

volumes:
- name: config
  configMap:
    name: notification-controller-config
- name: notification-output
  hostPath:
    path: /tmp/e2e-notifications
    type: Directory
```

### 5. Update initContainer Image ✅

**Already Done**: Change `busybox:latest` → `quay.io/jordigilh/kubernaut-busybox:latest`

---

## 📚 Authoritative References

1. **ADR-030**: Configuration Management (all services must follow)
2. **Reference Implementation**: `pkg/datastorage/config/config.go`
3. **E2E Pattern**: `test/e2e/gateway/gateway-deployment.yaml`
4. **Main Pattern**: `cmd/datastorage/main.go` (lines 58-95)

---

## ✅ Acceptance Criteria

- [ ] Config package created (`pkg/notification/config/config.go`)
- [ ] `main.go` loads from `CONFIG_PATH`
- [ ] ConfigMap created with YAML config
- [ ] Deployment uses ConfigMap volume mount
- [ ] Individual env vars removed
- [ ] `quay.io/jordigilh/kubernaut-busybox:latest` used for initContainer
- [ ] E2E tests pass
- [ ] Documentation updated

---

## 🎯 Priority

**CRITICAL**: This is a compliance issue with authoritative ADR-030 standard

**Timeline**: Should be fixed before merging DD-NOT-006

---

**Next Action**: Implement all 5 changes listed above

