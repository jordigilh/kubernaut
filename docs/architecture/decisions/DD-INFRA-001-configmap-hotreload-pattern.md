# DD-INFRA-001: ConfigMap Hot-Reload Pattern

## Status
**✅ Approved** (2025-12-06)
**Last Reviewed**: 2025-12-06
**Confidence**: 92%

---

## Context & Problem

**Problem**: Multiple Kubernaut services need to dynamically reload configuration from Kubernetes ConfigMaps without requiring pod restarts. This includes:
- **Rego policies** (Priority Engine, Custom Labels, Safety Rules)
- **Feature flags** (A/B testing, gradual rollouts)
- **Environment mappings** (namespace-to-environment overrides)
- **Operational parameters** (thresholds, timeouts, rate limits)

**Why This Matters**:
- **Operational Agility**: Configuration changes should take effect within ~60 seconds, not minutes (pod restart time)
- **Zero Downtime**: Policy updates should not require service interruption
- **Consistency**: All services should use the same hot-reload mechanism to ensure predictable behavior
- **Safety**: Invalid configuration should not crash the service; fallback to previous valid configuration

**Key Requirements**:
1. **Standard Kubernetes Pattern**: Use ConfigMap volume mounts (canonical approach)
2. **Dynamic Callback**: Execute user-defined function when content changes
3. **Graceful Degradation**: Continue with old configuration if new content is invalid
4. **Concurrency Safety**: Thread-safe access to reloaded configuration
5. **Auditability**: Log configuration version (hash) for debugging and compliance
6. **Resource Efficiency**: Minimal memory footprint, no Kubernetes API calls at runtime

**Scope**: Shared infrastructure component used by:
- Signal Processing: Priority Engine (Rego), Environment Classifier (ConfigMap overrides)
- AI Analysis: Safety policies, context filtering rules
- Workflow Execution: Playbook selection policies, execution parameters
- Gateway: Rate limiting rules, adapter configurations

---

## Alternatives Considered

### Alternative 1: Polling-Based ConfigMap Reload

**Approach**: Periodically fetch ConfigMap via Kubernetes API and check for changes

```go
func (r *Reloader) startPolling(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            cm := &corev1.ConfigMap{}
            if err := r.client.Get(ctx, r.configMapName, cm); err != nil {
                continue
            }
            if cm.ResourceVersion != r.lastVersion {
                r.onUpdate(cm.Data[r.key])
                r.lastVersion = cm.ResourceVersion
            }
        }
    }
}
```

**Pros**:
- ✅ Simple implementation (~30 lines)
- ✅ Easy to understand and debug

**Cons**:
- ❌ **30-second latency** (average 15s for changes to be detected)
- ❌ **Unnecessary API calls** (polls even when no changes)
- ❌ **Not scalable** (100 services × 30s polling = 200 API calls/minute per ConfigMap)
- ❌ **RBAC overhead** (requires ConfigMap `get` permission)

**Confidence**: 50% (rejected - latency, scalability, and API overhead)

---

### Alternative 2: File-Based ConfigMap Mounting + fsnotify ⭐ **SELECTED**

**Approach**: Mount ConfigMap as Kubernetes volume, watch filesystem changes with fsnotify

```go
func (r *Reloader) watchFile(ctx context.Context, path string) error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return err
    }
    defer watcher.Close()

    // Watch the directory (ConfigMap mounts use symlinks)
    if err := watcher.Add(filepath.Dir(path)); err != nil {
        return err
    }

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case event := <-watcher.Events:
            // ConfigMap updates create new symlink targets
            if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
                content, err := os.ReadFile(path)
                if err != nil {
                    r.logger.Error(err, "Failed to read updated config")
                    continue
                }
                if err := r.callback(string(content)); err != nil {
                    r.logger.Error(err, "Callback rejected new config - keeping previous")
                    continue
                }
                r.logger.Info("Configuration reloaded", "hash", r.computeHash(content)[:8])
            }
        case err := <-watcher.Errors:
            r.logger.Error(err, "Filesystem watcher error")
        }
    }
}
```

**Pros**:
- ✅ **Standard Kubernetes pattern** - ConfigMap volume mounts are the canonical way to consume config
- ✅ **No Kubernetes API calls** at runtime (zero API overhead)
- ✅ **Minimal memory footprint** - No informer cache, just a file watcher
- ✅ **Simpler RBAC** - No ConfigMap `get`/`list`/`watch` permissions needed
- ✅ **Works in disconnected scenarios** - No API dependency after pod start
- ✅ **Battle-tested** - fsnotify is widely used (Viper, Hugo, Kubernetes itself)
- ✅ **Graceful degradation** - Callback rejection keeps old config

**Cons**:
- ⚠️ **~60 second update latency** - Kubernetes kubelet syncs ConfigMap volumes every 60s by default
  - **Mitigation**: Acceptable for configuration changes (not real-time events)
  - **Mitigation**: Can tune with `kubelet.configMapAndSecretChangeDetectionStrategy`
- ⚠️ **Requires volume mount** in deployment spec
  - **Mitigation**: Standard practice, not additional complexity

**Confidence**: 92% (approved - standard pattern, minimal overhead)

---

### Alternative 3: Kubernetes Informer-Based Watching

**Approach**: Use Kubernetes informers to watch ConfigMap changes via API

```go
type ConfigMapReloader struct {
    client       client.Client
    informer     cache.Informer
    logger       logr.Logger

    configMapRef types.NamespacedName
    key          string
    callback     func(string) error

    mu           sync.RWMutex
    lastContent  string
    lastHash     string
}

func (r *ConfigMapReloader) Start(ctx context.Context) error {
    informer, err := r.createInformer(ctx)
    if err != nil {
        return err
    }
    informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        UpdateFunc: func(_, new interface{}) { r.handleConfigMapChange(new) },
    })
    go informer.Run(ctx.Done())
    return nil
}
```

**Pros**:
- ✅ Near-instant detection (<1 second latency)
- ✅ No deployment changes needed (no volume mount)

**Cons**:
- ❌ **Overkill for config reload** - Informers are designed for controllers tracking many resources
- ❌ **Memory overhead** - Informers maintain local caches (~1MB+ per ConfigMap)
- ❌ **RBAC complexity** - Requires ConfigMap `get`, `list`, `watch` permissions
- ❌ **API dependency** - Requires continuous API connection
- ❌ **More complex lifecycle** - Must manage informer start/stop
- ❌ **Not standard pattern** - ConfigMap volume mounts are the canonical K8s approach

**Confidence**: 60% (rejected - overkill for configuration hot-reload use case)

**When Informers ARE Appropriate**:
- Controllers watching many resources (e.g., all Pods in a namespace)
- Resources that change frequently (real-time event streams)
- Cases where volume mounts aren't feasible (cross-namespace watching)

---

## Decision

**APPROVED: Alternative 2** - File-Based ConfigMap Mounting + fsnotify

**Rationale**:

1. **Standard Kubernetes Pattern**: ConfigMap volume mounts are the canonical way to consume configuration
   - Alternative 1: API polling (non-standard, wasteful) ❌
   - Alternative 2: Volume mount + fsnotify (standard K8s pattern) ✅
   - Alternative 3: Informers (overkill for config, designed for controllers) ❌

2. **Resource Efficiency**: Minimal memory and API overhead
   - Alternative 1: Continuous API calls ❌
   - Alternative 2: Zero API calls at runtime, minimal memory ✅
   - Alternative 3: Informer cache memory overhead (~1MB+) ❌

3. **RBAC Simplicity**: No special permissions needed
   - Alternative 1: Requires ConfigMap `get` permission ❌
   - Alternative 2: Just volume mount (no API permissions) ✅
   - Alternative 3: Requires ConfigMap `get`, `list`, `watch` permissions ❌

4. **Acceptable Latency**: ~60 second update delay is fine for configuration changes
   - Configuration changes are infrequent (not real-time events)
   - 60 seconds is vastly better than pod restart (2-5 minutes)
   - Can tune kubelet sync period if faster updates needed

5. **Battle-Tested**: fsnotify is proven technology
   - Used by: Viper, Hugo, Kubernetes kubelet itself
   - Mature, well-maintained library

---

## Implementation

### Package Location

```
pkg/shared/hotreload/
├── file_watcher.go          # Main fsnotify implementation
├── file_watcher_test.go
└── doc.go                   # Package documentation
```

### Core Types

```go
package hotreload

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/go-logr/logr"
)

// ReloadCallback is called when file content changes.
// Return error to reject the new configuration (keeps previous).
type ReloadCallback func(newContent string) error

// FileWatcher watches a mounted ConfigMap file and triggers
// callbacks when content changes. Provides graceful degradation
// on callback errors.
//
// ConfigMap volume mounts use symlinks that change on updates,
// so we watch the directory for CREATE events (new symlink target).
type FileWatcher struct {
    path     string           // Path to the mounted ConfigMap file
    callback ReloadCallback   // Called when content changes
    logger   logr.Logger

    mu          sync.RWMutex
    lastContent string
    lastHash    string
    lastReload  time.Time
    reloadCount int64
    errorCount  int64

    watcher *fsnotify.Watcher
    stopCh  chan struct{}
}
```

### Constructor

```go
// NewFileWatcher creates a new hot-reloader for a mounted ConfigMap file.
//
// Parameters:
//   - path: Path to the mounted ConfigMap file (e.g., "/etc/config/priority.rego")
//   - callback: Function called when content changes (return error to reject)
//   - logger: Structured logger for audit trail
//
// Example:
//
//   watcher, err := hotreload.NewFileWatcher(
//       "/etc/kubernaut/policies/priority.rego",
//       func(content string) error {
//           newQuery, err := rego.New(...).PrepareForEval(ctx)
//           if err != nil {
//               return err // Keeps previous policy
//           }
//           engine.updatePolicy(newQuery)
//           return nil
//       },
//       logger,
//   )
func NewFileWatcher(path string, callback ReloadCallback, logger logr.Logger) (*FileWatcher, error) {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
    }

    return &FileWatcher{
        path:     path,
        callback: callback,
        logger:   logger.WithName("hot-reload").WithValues("path", path),
        watcher:  watcher,
        stopCh:   make(chan struct{}),
    }, nil
}
```

### Lifecycle Methods

```go
// Start begins watching the file. This method:
//   1. Loads the initial file content
//   2. Starts watching for changes in background
//   3. Returns error only if initial load fails
//
// The watch runs until Stop() is called or context is cancelled.
func (w *FileWatcher) Start(ctx context.Context) error {
    w.logger.Info("Starting file hot-reloader")

    // Initial load (blocking)
    if err := w.loadInitial(); err != nil {
        return fmt.Errorf("initial file load failed: %w", err)
    }

    // Watch the directory (ConfigMap mounts use symlinks)
    // When ConfigMap updates, kubelet creates new symlink target
    dir := filepath.Dir(w.path)
    if err := w.watcher.Add(dir); err != nil {
        return fmt.Errorf("failed to watch directory %s: %w", dir, err)
    }

    // Start watching in background
    go w.watchLoop(ctx)

    w.logger.Info("File hot-reloader started successfully",
        "initialHash", w.lastHash[:8])
    return nil
}

// Stop gracefully stops the file watcher.
func (w *FileWatcher) Stop() {
    w.logger.Info("Stopping file hot-reloader")
    close(w.stopCh)
    w.watcher.Close()
}

// loadInitial reads the file for the first time.
func (w *FileWatcher) loadInitial() error {
    content, err := os.ReadFile(w.path)
    if err != nil {
        return fmt.Errorf("failed to read file %s: %w", w.path, err)
    }

    // Validate via callback
    if err := w.callback(string(content)); err != nil {
        return fmt.Errorf("initial configuration validation failed: %w", err)
    }

    w.mu.Lock()
    w.lastContent = string(content)
    w.lastHash = w.computeHash(content)
    w.lastReload = time.Now()
    w.mu.Unlock()

    return nil
}

// watchLoop monitors filesystem events.
func (w *FileWatcher) watchLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            w.logger.Info("File watcher stopped (context cancelled)")
            return
        case <-w.stopCh:
            w.logger.Info("File watcher stopped")
            return
        case event, ok := <-w.watcher.Events:
            if !ok {
                return
            }
            // ConfigMap volume updates create new symlink targets
            // Watch for CREATE events on the symlink or target file
            if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
                w.handleFileChange()
            }
        case err, ok := <-w.watcher.Errors:
            if !ok {
                return
            }
            w.logger.Error(err, "Filesystem watcher error")
        }
    }
}
```

### Update Handling

```go
// handleFileChange processes file content changes.
func (w *FileWatcher) handleFileChange() {
    // Small delay to let symlink update complete
    time.Sleep(100 * time.Millisecond)

    content, err := os.ReadFile(w.path)
    if err != nil {
        w.logger.Error(err, "Failed to read updated file")
        return
    }

    // Check if content actually changed
    hash := w.computeHash(content)
    w.mu.RLock()
    if hash == w.lastHash {
        w.mu.RUnlock()
        return // No change (spurious event)
    }
    oldHash := w.lastHash
    w.mu.RUnlock()

    w.logger.V(1).Info("File change detected",
        "oldHash", oldHash[:8], "newHash", hash[:8])

    // Attempt to apply new configuration
    if err := w.callback(string(content)); err != nil {
        w.mu.Lock()
        w.errorCount++
        w.mu.Unlock()

        w.logger.Error(err, "Failed to apply new configuration - keeping previous",
            "newHash", hash[:8], "errorCount", w.errorCount)
        return // Graceful degradation
    }

    // Success - update state
    w.mu.Lock()
    w.lastContent = string(content)
    w.lastHash = hash
    w.lastReload = time.Now()
    w.reloadCount++
    w.mu.Unlock()

    w.logger.Info("Configuration hot-reloaded successfully",
        "hash", hash[:8], "reloadCount", w.reloadCount)
}

// computeHash returns SHA-256 hash of content (for version tracking).
func (w *FileWatcher) computeHash(content []byte) string {
    h := sha256.Sum256(content)
    return hex.EncodeToString(h[:])
}
```

### Status Methods

```go
// GetLastHash returns the hash of the currently active configuration.
// Useful for debugging and metrics.
func (w *FileWatcher) GetLastHash() string {
    w.mu.RLock()
    defer w.mu.RUnlock()
    return w.lastHash
}

// GetLastReloadTime returns when configuration was last successfully reloaded.
func (w *FileWatcher) GetLastReloadTime() time.Time {
    w.mu.RLock()
    defer w.mu.RUnlock()
    return w.lastReload
}

// GetReloadCount returns total successful reloads since start.
func (w *FileWatcher) GetReloadCount() int64 {
    w.mu.RLock()
    defer w.mu.RUnlock()
    return w.reloadCount
}

// GetErrorCount returns total failed reload attempts since start.
func (w *FileWatcher) GetErrorCount() int64 {
    w.mu.RLock()
    defer w.mu.RUnlock()
    return w.errorCount
}
```

---

## Usage Examples

### Example 1: Rego Policy Hot-Reload (Signal Processing)

```go
package priorityengine

import (
    "context"
    "fmt"
    "sync"

    "github.com/go-logr/logr"
    "github.com/open-policy-agent/opa/v1/rego"

    "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

type PriorityEngine struct {
    regoQuery   *rego.PreparedEvalQuery
    fileWatcher *hotreload.FileWatcher
    mu          sync.RWMutex
    logger      logr.Logger
}

// NewPriorityEngine creates a priority engine with hot-reloadable Rego policy.
// The policy file must be mounted from a ConfigMap volume.
func NewPriorityEngine(ctx context.Context, policyPath string, logger logr.Logger) (*PriorityEngine, error) {
    engine := &PriorityEngine{
        logger: logger,
    }

    // Create file watcher for Rego policy (mounted from ConfigMap)
    var err error
    engine.fileWatcher, err = hotreload.NewFileWatcher(
        policyPath, // e.g., "/etc/kubernaut/policies/priority.rego"
        func(content string) error {
            // Compile new Rego policy
            newQuery, err := rego.New(
                rego.Query("data.signalprocessing.priority.result"),
                rego.Module("priority.rego", content),
            ).PrepareForEval(ctx)
            if err != nil {
                return fmt.Errorf("Rego compilation failed: %w", err)
            }

            // Atomically swap policy
            engine.mu.Lock()
            engine.regoQuery = &newQuery
            engine.mu.Unlock()

            return nil
        },
        logger,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create file watcher: %w", err)
    }

    // Start watching (loads initial policy)
    if err := engine.fileWatcher.Start(ctx); err != nil {
        return nil, fmt.Errorf("failed to start Rego hot-reloader: %w", err)
    }

    return engine, nil
}

func (e *PriorityEngine) AssignPriority(ctx context.Context, input map[string]interface{}) (string, error) {
    e.mu.RLock()
    query := e.regoQuery
    e.mu.RUnlock()

    results, err := query.Eval(ctx, rego.EvalInput(input))
    // ... process results ...
    return "", err
}

func (e *PriorityEngine) Stop() {
    e.fileWatcher.Stop()
}
```

### Example 2: Environment Override Mapping (Signal Processing)

```go
package classifier

import (
    "context"
    "fmt"
    "strings"
    "sync"

    "github.com/go-logr/logr"
    "gopkg.in/yaml.v3"

    "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

type EnvironmentClassifier struct {
    mappings    map[string]string // namespace pattern -> environment
    fileWatcher *hotreload.FileWatcher
    mu          sync.RWMutex
}

// NewEnvironmentClassifier creates a classifier with hot-reloadable mappings.
// The mappings file must be mounted from a ConfigMap volume.
func NewEnvironmentClassifier(ctx context.Context, mappingsPath string, logger logr.Logger) (*EnvironmentClassifier, error) {
    classifier := &EnvironmentClassifier{
        mappings: make(map[string]string),
    }

    var err error
    classifier.fileWatcher, err = hotreload.NewFileWatcher(
        mappingsPath, // e.g., "/etc/kubernaut/config/mappings.yaml"
        func(content string) error {
            // Parse YAML mappings
            var newMappings map[string]string
            if err := yaml.Unmarshal([]byte(content), &newMappings); err != nil {
                return fmt.Errorf("invalid YAML: %w", err)
            }

            // Validate mappings
            validEnvs := map[string]bool{
                "production": true, "staging": true,
                "development": true, "test": true,
            }
            for pattern, env := range newMappings {
                if !validEnvs[strings.ToLower(env)] {
                    return fmt.Errorf("invalid environment %q for pattern %q", env, pattern)
                }
            }

            // Apply new mappings
            classifier.mu.Lock()
            classifier.mappings = newMappings
            classifier.mu.Unlock()

            return nil
        },
        logger,
    )
    if err != nil {
        return nil, err
    }

    if err := classifier.fileWatcher.Start(ctx); err != nil {
        return nil, err
    }

    return classifier, nil
}
```

### Example 3: Feature Flags (Any Service)

```go
package featureflags

import (
    "context"
    "encoding/json"
    "fmt"
    "sync"

    "github.com/go-logr/logr"

    "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

type FeatureFlags struct {
    flags       map[string]bool
    fileWatcher *hotreload.FileWatcher
    mu          sync.RWMutex
}

// NewFeatureFlags creates a feature flag manager with hot-reloadable config.
// The flags file must be mounted from a ConfigMap volume.
func NewFeatureFlags(ctx context.Context, flagsPath string, logger logr.Logger) (*FeatureFlags, error) {
    ff := &FeatureFlags{
        flags: make(map[string]bool),
    }

    var err error
    ff.fileWatcher, err = hotreload.NewFileWatcher(
        flagsPath, // e.g., "/etc/kubernaut/config/flags.json"
        func(content string) error {
            var newFlags map[string]bool
            if err := json.Unmarshal([]byte(content), &newFlags); err != nil {
                return fmt.Errorf("invalid JSON: %w", err)
            }

            ff.mu.Lock()
            ff.flags = newFlags
            ff.mu.Unlock()

            return nil
        },
        logger,
    )
    if err != nil {
        return nil, err
    }

    if err := ff.fileWatcher.Start(ctx); err != nil {
        return nil, err
    }

    return ff, nil
}

func (ff *FeatureFlags) IsEnabled(flag string) bool {
    ff.mu.RLock()
    defer ff.mu.RUnlock()
    return ff.flags[flag]
}
```

---

## Deployment Configuration

Since this approach uses ConfigMap volume mounts, **no special RBAC is required** beyond the standard volume mount permissions. The pod just needs the ConfigMap mounted as a volume:

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

**Key Points**:
- No RBAC changes needed (volume mounts use kubelet, not pod's service account)
- Standard Kubernetes pattern for configuration
- Works with any ConfigMap in the same namespace

---

## Metrics & Observability

The hot-reloader exposes metrics for monitoring:

| Metric | Type | Description |
|--------|------|-------------|
| `kubernaut_hotreload_success_total` | Counter | Successful configuration reloads |
| `kubernaut_hotreload_error_total` | Counter | Failed reload attempts |
| `kubernaut_hotreload_last_success_timestamp` | Gauge | Unix timestamp of last successful reload |
| `kubernaut_hotreload_content_hash` | Gauge (labeled) | Hash of current active configuration |

**Alerting Recommendations**:
```yaml
- alert: HotReloadFailureRate
  expr: rate(kubernaut_hotreload_error_total[5m]) > 0.1
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "ConfigMap hot-reload failures detected"
    description: "{{ $labels.configmap }} has {{ $value }} failures/second"
```

---

## Graceful Degradation

**Error Scenarios and Behavior**:

| Scenario | Behavior | Impact |
|----------|----------|--------|
| ConfigMap deleted | Keeps last known configuration | Service continues with stale config |
| File becomes unreadable | Logs error, keeps previous | Service continues with stale config |
| Callback returns error | Logs error, keeps previous | Service continues with valid config |
| Invalid content format | Callback error (keep previous) | Service continues with valid config |
| fsnotify watcher error | Logs error, retries on next event | Brief gap in change detection |

**Key Principle**: The service NEVER crashes due to ConfigMap issues after initial startup.

---

## Consequences

### Positive

- ✅ **Standard Kubernetes pattern** - ConfigMap volume mounts are canonical
- ✅ **Zero API overhead** - No Kubernetes API calls at runtime
- ✅ **Minimal memory footprint** - Just a file watcher, no informer cache
- ✅ **No RBAC complexity** - Just volume mount, no API permissions needed
- ✅ **Graceful degradation** - Invalid configs rejected, service continues
- ✅ **Auditability** - Hash logging for compliance
- ✅ **Standardized pattern** across all Kubernaut services
- ✅ **Battle-tested** - fsnotify is used by Viper, Hugo, Kubernetes itself

### Negative

- ⚠️ **~60 second update latency** - Kubelet ConfigMap sync period
  - **Mitigation**: Acceptable for configuration changes (not real-time events)
  - **Mitigation**: Can tune `kubelet.configMapAndSecretChangeDetectionStrategy`

- ⚠️ **Requires volume mount** in deployment spec
  - **Mitigation**: Standard practice, adds minimal complexity

### Neutral

- 🔄 **Depends on kubelet sync** - Update latency controlled by kubelet, not service
  - Expected behavior for ConfigMap volumes

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| [BR-SP-072](../../services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md) | Primary driver: Rego Hot-Reload requirement |
| [DD-007](DD-007-kubernetes-aware-graceful-shutdown.md) | Shutdown coordination pattern |
| [ADR-030](ADR-030-service-configuration-management.md) | Configuration management strategy |
| [NOTICE_SHARED_HOTRELOADER_PACKAGE](../../handoff/NOTICE_SHARED_HOTRELOADER_PACKAGE.md) | Handoff notice to other teams |

**Services Using This Pattern**:
- Signal Processing: Priority Engine (Rego), Environment Classifier (mappings)
- AI Analysis: Safety policies, context filtering (planned)
- Workflow Execution: Playbook selection policies (planned)
- Gateway: Rate limiting rules (planned)

**Services NOT Using This Pattern** (by design):
- **Data Storage**: Excluded - see rationale below

---

## Appendix: Data Storage Exclusion Rationale

**Decision**: Data Storage does NOT implement ConfigMap hot-reload.

**Confidence**: 98%

**Rationale**:

Hot-reload provides value when services have **runtime-configurable business logic**. Data Storage lacks this characteristic:

| Data Storage Component | Core Service? | Hot-Reloadable? |
|------------------------|---------------|-----------------|
| PostgreSQL connection pool | ✅ Core | ❌ No (stateful) |
| Redis DLQ operations | ✅ Core | ❌ No (stateful) |
| Audit event storage (ADR-034) | ✅ Core | ❌ No (stateful) |
| Workflow semantic search | ✅ Core | ❌ No (stateful) |
| HTTP server timeouts | ❌ Peripheral | ✅ Yes (only this) |

**Key Insight**: The only hot-reloadable configuration in Data Storage is HTTP server timeouts (`ReadTimeout`, `WriteTimeout`), which:
1. Are peripheral HTTP plumbing, not core business logic
2. Rarely need runtime adjustment (typically set once)
3. Provide marginal benefit (~30s saved) vs pod restart (~30s)
4. Would require 4-6 hours implementation for near-zero ROI

**Comparison with Services That DO Benefit**:

| Service | Hot-Reloadable Asset | Business Value |
|---------|---------------------|----------------|
| Signal Processing | Rego policies | **HIGH** - Policy tuning without downtime |
| Gateway | Rate limiting rules | **MEDIUM** - Operational tuning |
| Data Storage | HTTP timeouts only | **NEAR-ZERO** - Core unchanged |

**Conclusion**: Data Storage configuration changes should use standard Kubernetes restart-based deployment updates. This is the canonical pattern for services without runtime-configurable business logic.

---

## Review & Evolution

**When to Revisit**:
- If kubelet ConfigMap sync becomes significantly faster by default
- If <10 second latency becomes a requirement (consider informer approach)
- If non-ConfigMap sources need hot-reload (e.g., Secrets, external systems)
- If fsnotify library has breaking changes

**Success Metrics**:
- **Reload Latency**: <90 seconds (P99) - kubelet sync + fsnotify detection
- **Reload Success Rate**: >99.9%
- **Memory Overhead**: <100KB per watcher
- **API Call Efficiency**: Zero API calls at runtime

---

**Last Updated**: December 6, 2025
**Next Review**: June 6, 2026 (6 months)

---

## Change Log

| Date | Change | Author |
|------|--------|--------|
| 2025-12-06 | Added Data Storage exclusion rationale - hot-reload provides no value for stateless DB proxy services | AI Assistant |
| 2025-12-06 | Initial approval | Architecture Team |

