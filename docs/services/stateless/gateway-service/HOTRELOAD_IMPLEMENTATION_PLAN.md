# Gateway ConfigMap Hot-Reload Implementation Plan

**Date**: 2025-12-06
**Status**: 📋 **PLANNED** (Day 8 Gateway)
**Related**: [NOTICE_SHARED_HOTRELOADER_PACKAGE.md](../../../handoff/NOTICE_SHARED_HOTRELOADER_PACKAGE.md)
**Package**: `pkg/shared/hotreload/`

---

## 📋 Summary

Enable Gateway to hot-reload operational configuration from ConfigMaps without pod restarts. Uses the shared `pkg/shared/hotreload/` package from Signal Processing team.

---

## 🎯 Business Value

| Scenario | Without Hot-Reload | With Hot-Reload |
|----------|-------------------|-----------------|
| Storm threshold tuning during incident | Pod restart (10-30s downtime) | ~60s latency, zero downtime |
| Dedup TTL adjustment | Pod restart | ~60s latency, zero downtime |
| Retry config tuning | Pod restart | ~60s latency, zero downtime |

---

## 📊 Hot-Reloadable Configuration

### Priority 1: Storm Detection Settings (P0)
**Use Case**: Adjust storm thresholds during alert storms

```yaml
# ConfigMap: kubernaut-gateway-storm-config
storm:
  rate_threshold: 10          # Hot-reloadable
  pattern_threshold: 5        # Hot-reloadable
  buffer_threshold: 5         # Hot-reloadable
  inactivity_timeout: 60s     # Hot-reloadable
  max_window_duration: 5m     # Hot-reloadable
  sampling_threshold: 0.95    # Hot-reloadable
  sampling_rate: 0.5          # Hot-reloadable
```

### Priority 2: Deduplication Settings (P1)
**Use Case**: Adjust dedup TTL for different alert patterns

```yaml
# ConfigMap: kubernaut-gateway-dedup-config
deduplication:
  ttl: 5m                     # Hot-reloadable
```

### Priority 3: Retry Settings (P2)
**Use Case**: Fine-tune retry behavior under K8s API pressure

```yaml
# ConfigMap: kubernaut-gateway-retry-config
retry:
  max_attempts: 3             # Hot-reloadable
  initial_backoff: 100ms      # Hot-reloadable
  max_backoff: 5s             # Hot-reloadable
```

---

## 🔧 Implementation Design

### Architecture

```
                    ┌──────────────────────────┐
                    │  ConfigMap Volume Mount  │
                    │  /etc/kubernaut/config/  │
                    └────────────┬─────────────┘
                                 │
                                 │ fsnotify (kubelet syncs ~60s)
                                 ▼
┌──────────────────────────────────────────────────────────────────┐
│                        Gateway Server                              │
│                                                                    │
│  ┌─────────────────┐    ┌─────────────────┐    ┌────────────────┐ │
│  │  StormWatcher   │    │   DedupWatcher  │    │  RetryWatcher  │ │
│  │ (FileWatcher)   │    │ (FileWatcher)   │    │ (FileWatcher)  │ │
│  └────────┬────────┘    └────────┬────────┘    └───────┬────────┘ │
│           │                      │                      │          │
│           │ callback             │ callback             │ callback │
│           ▼                      ▼                      ▼          │
│  ┌────────────────┐    ┌──────────────────┐    ┌───────────────┐  │
│  │ StormDetector  │    │ DeduplicationSvc │    │  CRDCreator   │  │
│  │ UpdateConfig() │    │ UpdateTTL()      │    │ UpdateRetry() │  │
│  └────────────────┘    └──────────────────┘    └───────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### Interface Changes

```go
// pkg/gateway/processing/storm_detector.go

type StormDetector struct {
    // ... existing fields ...

    mu           sync.RWMutex  // Protects config updates
    rateThreshold int
    patternThreshold int
    // ... other configurable fields
}

// UpdateConfig hot-reloads storm detection configuration
// Thread-safe via RWMutex
func (d *StormDetector) UpdateConfig(config StormSettings) error {
    if err := config.Validate(); err != nil {
        return fmt.Errorf("invalid storm config: %w", err)
    }

    d.mu.Lock()
    defer d.mu.Unlock()

    d.rateThreshold = config.RateThreshold
    d.patternThreshold = config.PatternThreshold
    // ... update other fields

    d.logger.Info("Storm detection config hot-reloaded",
        "rate_threshold", config.RateThreshold,
        "pattern_threshold", config.PatternThreshold)

    return nil
}
```

### Server Integration

```go
// pkg/gateway/server.go

func NewServer(cfg *ServerConfig, logger logr.Logger) (*Server, error) {
    // ... existing initialization ...

    // Initialize hot-reload watchers (if paths configured)
    if cfg.Processing.Storm.ConfigPath != "" {
        stormWatcher, err := hotreload.NewFileWatcher(
            cfg.Processing.Storm.ConfigPath,
            func(content string) error {
                var stormCfg StormSettings
                if err := yaml.Unmarshal([]byte(content), &stormCfg); err != nil {
                    return fmt.Errorf("invalid storm YAML: %w", err)
                }
                return server.stormDetector.UpdateConfig(stormCfg)
            },
            logger.WithName("storm-hotreload"),
        )
        if err != nil {
            return nil, fmt.Errorf("failed to create storm watcher: %w", err)
        }
        server.stormWatcher = stormWatcher
    }

    // Similar for dedup and retry watchers...
}

func (s *Server) Start(ctx context.Context) error {
    // Start hot-reload watchers
    if s.stormWatcher != nil {
        if err := s.stormWatcher.Start(ctx); err != nil {
            return fmt.Errorf("failed to start storm watcher: %w", err)
        }
    }
    // ... start other watchers

    // ... existing server startup
}
```

---

## 📦 Deployment Configuration

### ConfigMap Definition

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-gateway-config
  namespace: kubernaut-system
data:
  storm.yaml: |
    rate_threshold: 10
    pattern_threshold: 5
    buffer_threshold: 5
    inactivity_timeout: 60s
    max_window_duration: 5m
    sampling_threshold: 0.95
    sampling_rate: 0.5

  dedup.yaml: |
    ttl: 5m

  retry.yaml: |
    max_attempts: 3
    initial_backoff: 100ms
    max_backoff: 5s
```

### Deployment Volume Mounts

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      containers:
      - name: gateway
        volumeMounts:
        - name: gateway-config
          mountPath: /etc/kubernaut/config
          readOnly: true
      volumes:
      - name: gateway-config
        configMap:
          name: kubernaut-gateway-config
```

---

## 🧪 Testing Strategy

### Unit Tests
- Test `UpdateConfig()` methods are thread-safe
- Test config validation rejects invalid values
- Test graceful degradation (keeps old config on error)

### Integration Tests
- Test file watcher triggers callback on file change
- Test config changes propagate to processing components
- Test metrics update after hot-reload

### E2E Tests
- Test ConfigMap update triggers hot-reload within ~60s
- Test Gateway continues processing during config reload
- Test invalid ConfigMap update is rejected (old config preserved)

---

## 📊 Metrics

```go
// New metrics for hot-reload observability
var (
    ConfigReloadTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "gateway_config_reload_total",
            Help: "Total number of configuration reloads",
        },
        []string{"config_type", "result"}, // config_type: storm|dedup|retry, result: success|error
    )

    ConfigReloadDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "gateway_config_reload_duration_seconds",
            Help: "Duration of configuration reload operations",
            Buckets: []float64{0.001, 0.01, 0.1, 1.0},
        },
        []string{"config_type"},
    )

    ConfigReloadLastSuccess = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "gateway_config_reload_last_success_timestamp",
            Help: "Timestamp of last successful config reload",
        },
        []string{"config_type"},
    )
)
```

---

## 📅 Implementation Timeline

| Phase | Tasks | Estimate |
|-------|-------|----------|
| **Day 8.1** | Add `UpdateConfig()` methods to StormDetector | 2 hours |
| **Day 8.2** | Add `UpdateTTL()` to DeduplicationService | 1 hour |
| **Day 8.3** | Add `UpdateRetry()` to CRDCreator | 1 hour |
| **Day 8.4** | Integrate FileWatcher in server.go | 2 hours |
| **Day 8.5** | Add metrics and logging | 1 hour |
| **Day 8.6** | Unit tests for UpdateConfig methods | 2 hours |
| **Day 8.7** | Integration tests for hot-reload | 2 hours |
| **Day 8.8** | Update deployment manifests | 1 hour |

**Total**: ~12 hours (Day 8 Gateway)

---

## ✅ Acceptance Criteria

- [ ] Storm thresholds can be updated via ConfigMap without pod restart
- [ ] Dedup TTL can be updated via ConfigMap without pod restart
- [ ] Retry settings can be updated via ConfigMap without pod restart
- [ ] Invalid config changes are rejected (old config preserved)
- [ ] Metrics track reload success/failure counts
- [ ] Logs include config reload audit trail
- [ ] Zero downtime during config reload
- [ ] Update latency is ~60s (kubelet sync interval)

---

## 🔗 Related Documents

| Document | Description |
|----------|-------------|
| [NOTICE_SHARED_HOTRELOADER_PACKAGE.md](../../../handoff/NOTICE_SHARED_HOTRELOADER_PACKAGE.md) | Shared package notification |
| [DD-INFRA-001](../../../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md) | Design decision for hot-reload pattern |
| [api-specification.md](./api-specification.md) | Gateway API specification |

---

**Document Version**: 1.0
**Last Updated**: 2025-12-06
**Author**: Gateway Team





