# DD-HAPI-004: ConfigMap Hot-Reload for HolmesGPT API

## Status
**✅ APPROVED & IMPLEMENTED**
**Date**: 2025-12-06
**Last Reviewed**: 2025-12-07
**Implemented**: 2025-12-07
**Confidence**: 92%

---

## Context & Problem

**Problem**: HolmesGPT API service requires pod restart for configuration changes, causing:
- 2-5 minute downtime for config updates
- Slow response to LLM cost spikes (model switching)
- Delayed failover capability (provider switching)
- Operational friction for toolset configuration

**Key Requirements**:
1. Hot-reload configuration from mounted ConfigMap without pod restart
2. Graceful degradation (keep previous config if new config is invalid)
3. Thread-safe configuration access
4. Audit trail of configuration changes (hash logging)
5. ~60 second update latency (acceptable for config changes)

**Alignment**: Implements DD-INFRA-001 pattern for Python services.

---

## Decision

**PROPOSED**: Implement file-based ConfigMap hot-reload using Python `watchdog` library.

**Rationale**:
1. **Standard Kubernetes Pattern**: ConfigMap volume mounts are canonical
2. **Proven Approach**: DD-INFRA-001 pattern validated for Go services
3. **Python Equivalent**: `watchdog` library is mature (10+ years, 6k+ stars)
4. **Low Complexity**: ~100 lines of core code
5. **Graceful Degradation**: Invalid configs rejected, service continues

---

## Scope

### Fields Supporting Hot-Reload

| Field | Config Path | Hot-Reload | Justification |
|-------|-------------|------------|---------------|
| `llm.model` | `llm.model` | ✅ Yes | Cost/quality switching |
| `llm.provider` | `llm.provider` | ✅ Yes | Failover capability |
| `llm.endpoint` | `llm.endpoint` | ✅ Yes | Endpoint switching |
| `llm.max_retries` | `llm.max_retries` | ✅ Yes | Tune retry behavior |
| `llm.timeout_seconds` | `llm.timeout_seconds` | ✅ Yes | Adjust for slow models |
| `llm.temperature` | `llm.temperature` | ✅ Yes | Fine-tune responses |
| `llm.max_tokens_per_request` | `llm.max_tokens_per_request` | ✅ Yes | Cost control |
| `toolsets` | `toolsets` | ✅ Yes | Enable/disable toolsets |
| `log_level` | `log_level` | ✅ Yes | Debug production issues |

### Fields NOT Supporting Hot-Reload

| Field | Reason |
|-------|--------|
| `api_host`, `api_port` | Requires server restart |
| `auth_enabled` | Security-critical, restart acceptable |
| `kubernetes.*` | Infrastructure config, restart acceptable |
| `DATA_STORAGE_URL` | Core dependency, restart acceptable |

---

## Configuration File Format

### Location
```
/etc/holmesgpt/config.yaml  (mounted from ConfigMap)
```

### Example ConfigMap
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: kubernaut
data:
  config.yaml: |
    service_name: holmesgpt-api
    version: "1.0.0"
    log_level: INFO

    llm:
      provider: openai
      model: gpt-4
      endpoint: null  # Use default
      max_retries: 3
      timeout_seconds: 60
      max_tokens_per_request: 4096
      temperature: 0.7

    toolsets:
      kubernetes/core: {}
      kubernetes/logs: {}
      workflow/catalog:
        enabled: true
```

---

## Architecture

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    HolmesGPT API Service                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────────┐     ┌─────────────────────────────┐   │
│  │   ConfigManager     │     │     Business Logic          │   │
│  │   (Thread-Safe)     │────▶│   (incident.py, recovery.py)│   │
│  │                     │     │                             │   │
│  │  get_llm_config()   │     │   Uses config.get_llm_*()   │   │
│  │  get_toolsets()     │     │                             │   │
│  └─────────┬───────────┘     └─────────────────────────────┘   │
│            │                                                     │
│            │ Watches                                             │
│            ▼                                                     │
│  ┌─────────────────────┐                                        │
│  │   FileWatcher       │                                        │
│  │   (watchdog)        │                                        │
│  │                     │                                        │
│  │  - Debounced events │                                        │
│  │  - Hash tracking    │                                        │
│  │  - Graceful degrade │                                        │
│  └─────────┬───────────┘                                        │
│            │                                                     │
└────────────┼─────────────────────────────────────────────────────┘
             │
             │ Watches (fsnotify equivalent)
             ▼
┌─────────────────────────────────────────────────────────────────┐
│  /etc/holmesgpt/config.yaml (ConfigMap volume mount)            │
│                                                                  │
│  Kubelet syncs every ~60 seconds                                │
└─────────────────────────────────────────────────────────────────┘
```

### Sequence: Config Hot-Reload

```
ConfigMap Update                 Kubelet                    FileWatcher               ConfigManager
      │                            │                            │                           │
      │  kubectl apply             │                            │                           │
      ├───────────────────────────▶│                            │                           │
      │                            │                            │                           │
      │                            │  Sync (~60s)               │                           │
      │                            │  Update symlink            │                           │
      │                            ├───────────────────────────▶│                           │
      │                            │                            │                           │
      │                            │                            │  fsnotify event           │
      │                            │                            │  (CREATE/WRITE)           │
      │                            │                            ├─────────┐                 │
      │                            │                            │         │ Debounce 200ms  │
      │                            │                            │◀────────┘                 │
      │                            │                            │                           │
      │                            │                            │  Read file                │
      │                            │                            │  Compute hash             │
      │                            │                            │  If hash changed:         │
      │                            │                            │    Call callback          │
      │                            │                            ├──────────────────────────▶│
      │                            │                            │                           │
      │                            │                            │                     Validate YAML
      │                            │                            │                     If valid:
      │                            │                            │                       Update config
      │                            │                            │                       Log success
      │                            │                            │                     If invalid:
      │                            │                            │                       Log error
      │                            │                            │◀──────────────────────────│
      │                            │                            │                     Keep previous
      │                            │                            │                           │
```

---

## API Changes

### New Module: `src/config/hot_reload.py`

```python
class FileWatcher:
    """
    Hot-reload file watcher for ConfigMap-mounted configuration.

    Uses watchdog library (Python equivalent of fsnotify).
    Implements DD-INFRA-001 pattern for Python.
    """

    def __init__(
        self,
        path: str,
        callback: Callable[[str], None],
        logger: logging.Logger
    ) -> None: ...

    def start(self) -> None:
        """Start watching. Raises if initial load fails."""
        ...

    def stop(self) -> None:
        """Stop watching gracefully."""
        ...

    @property
    def last_hash(self) -> str:
        """Hash of current active configuration."""
        ...

    @property
    def reload_count(self) -> int:
        """Total successful reloads since start."""
        ...

    @property
    def error_count(self) -> int:
        """Total failed reload attempts since start."""
        ...


class ConfigManager:
    """
    Thread-safe configuration manager with hot-reload support.

    Usage:
        config = ConfigManager("/etc/holmesgpt/config.yaml", logger)
        config.start()

        # Access config (thread-safe)
        model = config.get_llm_model()
        toolsets = config.get_toolsets()
    """

    def get_llm_model(self) -> str: ...
    def get_llm_provider(self) -> str: ...
    def get_llm_endpoint(self) -> Optional[str]: ...
    def get_llm_max_retries(self) -> int: ...
    def get_llm_timeout(self) -> int: ...
    def get_llm_temperature(self) -> float: ...
    def get_toolsets(self) -> Dict[str, Any]: ...
    def get_log_level(self) -> str: ...
```

### Changes to `src/main.py`

```python
# Before (static config)
config = load_config()

# After (hot-reloadable config)
config_manager = ConfigManager(
    path=os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml"),
    logger=logger
)
config_manager.start()

# Access via manager (not direct dict)
app.state.config_manager = config_manager
```

### Changes to `src/extensions/incident.py` and `recovery.py`

```python
# Before
model_name = os.getenv("LLM_MODEL") or app_config.get("llm", {}).get("model")

# After
config_manager = request.app.state.config_manager
model_name = config_manager.get_llm_model()
```

---

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `holmesgpt_config_reload_total` | Counter | Total successful config reloads |
| `holmesgpt_config_reload_errors_total` | Counter | Total failed reload attempts |
| `holmesgpt_config_last_reload_timestamp` | Gauge | Unix timestamp of last successful reload |
| `holmesgpt_config_hash` | Info | Hash of current active configuration |

---

## Graceful Degradation

| Scenario | Behavior | Impact |
|----------|----------|--------|
| ConfigMap deleted | Keeps last known config | Service continues |
| Invalid YAML syntax | Logs error, keeps previous | Service continues |
| Missing required field | Callback fails, keeps previous | Service continues |
| File unreadable | Logs error, keeps previous | Service continues |
| watchdog error | Logs error, retries on next event | Brief detection gap |

**Key Principle**: Service NEVER crashes due to ConfigMap issues after initial startup.

---

## Dependencies

### New Python Dependencies

```
# requirements.txt
watchdog>=3.0.0,<4.0.0  # File system events (fsnotify equivalent)
```

**Risk Assessment**:
- `watchdog` is mature (10+ years), well-maintained
- Zero external dependencies
- Used by Django dev server, pytest-watch, Hugo

---

## Deployment Changes

### Pod Spec Update

```yaml
spec:
  containers:
  - name: holmesgpt-api
    volumeMounts:
    - name: config
      mountPath: /etc/holmesgpt
      readOnly: true
  volumes:
  - name: config
    configMap:
      name: holmesgpt-api-config
```

**Note**: No RBAC changes required - volume mounts use kubelet, not pod's service account.

---

## Testing Strategy

### Unit Tests
- `test_file_watcher.py` - FileWatcher lifecycle, debouncing, hash tracking
- `test_config_manager.py` - Thread-safe access, validation, graceful degradation

### Integration Tests
- Config reload with valid YAML
- Config reload with invalid YAML (graceful degradation)
- Rapid config updates (debouncing)

### E2E Tests (Kind cluster)
- ConfigMap update triggers reload
- Symlink handling works correctly
- ~60 second latency verification

---

## Consequences

### Positive
- ✅ Config changes take effect in ~60 seconds (vs 2-5 min restart)
- ✅ LLM model switching without downtime
- ✅ Provider failover capability
- ✅ Graceful degradation on invalid config
- ✅ Audit trail via hash logging

### Negative
- ⚠️ ~60 second latency (kubelet sync period)
  - **Mitigation**: Acceptable for configuration changes
- ⚠️ New dependency (`watchdog`)
  - **Mitigation**: Mature, zero-dependency library
- ⚠️ Slight code complexity increase
  - **Mitigation**: ~100 lines of well-tested code

---

## Implementation Estimate

| Task | Effort |
|------|--------|
| `FileWatcher` class | 2 hours |
| `ConfigManager` class | 1 hour |
| Integration with `main.py` | 1 hour |
| Integration with `incident.py`, `recovery.py` | 1 hour |
| Unit tests | 2 hours |
| Integration tests | 1 hour |
| Documentation | 1 hour |
| **Total** | **~9 hours** |

---

## Related Documents

| Document | Relationship |
|----------|-------------|
| [DD-INFRA-001](DD-INFRA-001-configmap-hotreload-pattern.md) | Parent pattern (Go implementation) |
| [DD-HOLMESGPT-012](DD-HOLMESGPT-012-Minimal-Internal-Service-Architecture.md) | Service architecture |
| [DD-005](DD-005-OBSERVABILITY-STANDARDS.md) | Metrics standards |

---

## Approval

| Role | Name | Date | Decision |
|------|------|------|----------|
| Architecture | TBD | ⏳ Pending | |
| HolmesGPT-API Team | TBD | ⏳ Pending | |

---

**Last Updated**: December 6, 2025
**Next Review**: After approval

