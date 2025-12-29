# NOTICE: Shared ConfigMap HotReloader Package

**Date**: 2025-12-06
**From**: SignalProcessing Team
**To**: All Service Teams
**Status**: ✅ **Implementation Complete** (Day 9 SignalProcessing - Rego Hot-Reload)
**Reference**: [DD-INFRA-001](../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md)

---

## Summary

A shared `HotReloader` package will be created at `pkg/shared/hotreload/` for **generic ConfigMap hot-reloading**. This package enables any service to dynamically reload configuration from ConfigMaps without pod restarts.

**Approach**: Uses **fsnotify** to watch ConfigMap volume mounts (standard Kubernetes pattern) - NOT informers.

**Key Benefits**:
- ✅ **Standard K8s pattern** - ConfigMap volume mounts are canonical
- ✅ **Zero API overhead** - No Kubernetes API calls at runtime
- ✅ **Minimal memory** - No informer cache, just a file watcher
- ✅ **No RBAC complexity** - Just volume mount, no API permissions
- ✅ **Generic** - Works with any ConfigMap data (YAML, JSON, Rego, etc.)

**Design Decision**: See [DD-INFRA-001: ConfigMap Hot-Reload Pattern](../architecture/decisions/DD-INFRA-001-configmap-hotreload-pattern.md) for the complete technical specification.

---

## Location

```
pkg/shared/hotreload/
├── file_watcher.go       # Core fsnotify-based watching logic
├── file_watcher_test.go
└── doc.go                # Package documentation
```

---

## Use Cases

| Service | Use Case | ConfigMap Data Type |
|---------|----------|---------------------|
| **SignalProcessing** | Hot-reload Rego policies | `.rego` files |
| **AIAnalysis** | Hot-reload approval policies | `.rego` files |
| **Gateway** | Hot-reload processing config | YAML configuration |
| **Any Service** | Feature flags, timeouts, limits | YAML/JSON |
| **Any Service** | Environment overrides | Key-value pairs |

---

## API

```go
package hotreload

// ReloadCallback is called when file content changes.
// Return error to reject the new configuration (keeps previous).
type ReloadCallback func(newContent string) error

// FileWatcher watches a mounted ConfigMap file and triggers
// callbacks when content changes. Uses fsnotify for efficient
// filesystem event detection.
type FileWatcher struct {
    // ... internal fields
}

// NewFileWatcher creates a new hot-reloader for a mounted ConfigMap file.
// path: Path to the mounted ConfigMap file (e.g., "/etc/config/priority.rego")
// callback: Function called when content changes (return error to reject)
// logger: Structured logger for audit trail
func NewFileWatcher(path string, callback ReloadCallback, logger logr.Logger) (*FileWatcher, error)

// Start begins watching the file. Loads initial content and starts
// watching for changes in background. Returns error only if initial load fails.
func (w *FileWatcher) Start(ctx context.Context) error

// Stop gracefully stops the file watcher.
func (w *FileWatcher) Stop()

// GetLastHash returns the hash of the currently active configuration.
func (w *FileWatcher) GetLastHash() string

// GetLastReloadTime returns when configuration was last successfully reloaded.
func (w *FileWatcher) GetLastReloadTime() time.Time

// GetReloadCount returns total successful reloads since start.
func (w *FileWatcher) GetReloadCount() int64

// GetErrorCount returns total failed reload attempts since start.
func (w *FileWatcher) GetErrorCount() int64
```

---

## Features

| Feature | Description |
|---------|-------------|
| **Standard K8s Pattern** | Uses ConfigMap volume mounts (canonical approach) |
| **Zero API Calls** | No Kubernetes API calls at runtime |
| **Minimal Memory** | Just a file watcher, no informer cache |
| **Thread-Safe** | `sync.RWMutex` prevents race conditions |
| **Version Tracking** | SHA256 hash for change detection |
| **Graceful Degradation** | Keeps old value if callback returns error |
| **~60s Latency** | Kubelet syncs ConfigMap volumes every ~60s |

---

## Usage Examples

### Example 1: Rego Policy Hot-Reload (SignalProcessing)

```go
// ConfigMap must be mounted as a volume
// Volume mount: /etc/kubernaut/policies/priority.rego

watcher, err := hotreload.NewFileWatcher(
    "/etc/kubernaut/policies/priority.rego",
    func(content string) error {
        // Recompile Rego query
        newQuery, err := rego.New(
            rego.Query("data.signalprocessing.priority.result"),
            rego.Module("priority.rego", content),
        ).PrepareForEval(ctx)
        if err != nil {
            return fmt.Errorf("invalid Rego policy: %w", err)
        }

        // Update engine's query (thread-safe)
        engine.mu.Lock()
        engine.regoQuery = &newQuery
        engine.mu.Unlock()

        logger.Info("Priority Rego policy hot-reloaded")
        return nil
    },
    logger,
)
if err != nil {
    return err
}

// Start watching (blocks until context cancelled)
if err := watcher.Start(ctx); err != nil {
    return fmt.Errorf("failed to start watcher: %w", err)
}
```

### Example 2: YAML Configuration Hot-Reload (Gateway)

```go
// ConfigMap must be mounted as a volume
// Volume mount: /etc/kubernaut/config/rate_limits.yaml

watcher, err := hotreload.NewFileWatcher(
    "/etc/kubernaut/config/rate_limits.yaml",
    func(content string) error {
        var limits RateLimitConfig
        if err := yaml.Unmarshal([]byte(content), &limits); err != nil {
            return fmt.Errorf("invalid YAML: %w", err)
        }

        rateLimiter.UpdateLimits(limits)
        logger.Info("Rate limits hot-reloaded", "rpm", limits.RequestsPerMinute)
        return nil
    },
    logger,
)
if err != nil {
    return err
}

if err := watcher.Start(ctx); err != nil {
    return err
}
```

### Example 3: JSON Feature Flags (Any Service)

```go
// ConfigMap must be mounted as a volume
// Volume mount: /etc/kubernaut/config/flags.json

watcher, err := hotreload.NewFileWatcher(
    "/etc/kubernaut/config/flags.json",
    func(content string) error {
        var flags map[string]bool
        if err := json.Unmarshal([]byte(content), &flags); err != nil {
            return fmt.Errorf("invalid JSON: %w", err)
        }

        featureManager.Update(flags)
        return nil
    },
    logger,
)
```

---

## Deployment Configuration

**Key Requirement**: ConfigMaps must be mounted as volumes in your deployment.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signal-processing
spec:
  template:
    spec:
      containers:
      - name: controller
        volumeMounts:
        - name: rego-policies
          mountPath: /etc/kubernaut/policies
          readOnly: true
        - name: config
          mountPath: /etc/kubernaut/config
          readOnly: true
      volumes:
      - name: rego-policies
        configMap:
          name: kubernaut-rego-policies
      - name: config
        configMap:
          name: kubernaut-environment-config
```

**No RBAC Changes Required** - Volume mounts use kubelet, not pod's service account.

---

## Benefits Over Pod Restart

| Aspect | Pod Restart | ConfigMap Hot-Reload |
|--------|-------------|---------------------|
| **Downtime** | Brief interruption | Zero downtime |
| **State Loss** | In-memory caches cleared | State preserved |
| **Speed** | 10-30 seconds | ~60 seconds (kubelet sync) |
| **Blast Radius** | Entire pod restarts | Only config changes |
| **Rollback** | Requires another restart | Update ConfigMap |

---

## Update Latency

**Important**: ConfigMap volume mounts are synced by kubelet every ~60 seconds by default. This means:
- Changes to ConfigMaps take ~60 seconds to appear in the mounted files
- This is acceptable for configuration changes (not real-time events)
- Still vastly faster than pod restarts (2-5 minutes)

If faster updates are required, you can tune `kubelet.configMapAndSecretChangeDetectionStrategy`.

---

## Implementation Timeline

| Phase | Status |
|-------|--------|
| Package creation | Day 5 SignalProcessing |
| Unit tests | Day 5 SignalProcessing |
| Integration in PriorityEngine | Day 5 SignalProcessing |
| Available for all services | After Day 5 |

---

## Acknowledgment Required

Please acknowledge receipt of this notice by updating the section below:

### Acknowledgments

| Team | Acknowledged | Date | Notes |
|------|--------------|------|-------|
| AIAnalysis | ⬜ Pending | - | Rego policy hot-reload |
| Gateway | ✅ Acknowledged | 2025-12-06 | Storm thresholds, dedup TTL, retry config. Implementation plan: Day 8 Gateway |
| WorkflowExecution | ⬜ Pending | - | Timeout/retry config |
| RemediationOrchestrator | ⬜ Pending | - | Policy config |
| KubernetesExecutor | ⬜ Pending | - | Safety config |
| HolmesGPT-API | ⬜ Pending | - | Model config |

---

## Questions / Feedback

Contact SignalProcessing team or raise in `#kubernaut-dev` channel.
