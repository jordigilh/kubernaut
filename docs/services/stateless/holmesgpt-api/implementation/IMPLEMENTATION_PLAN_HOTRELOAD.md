# IMPLEMENTATION_PLAN_HOTRELOAD.md

## ConfigMap Hot-Reload Implementation Plan

**BR**: [BR-HAPI-199](../../../../requirements/BR-HAPI-199-configmap-hot-reload.md)
**DD**: [DD-HAPI-004](../../../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md)
**Status**: 🔄 IN PROGRESS
**Created**: 2025-12-06
**Target**: V1.0

---

## Overview

Implement file-based ConfigMap hot-reload for HolmesGPT API using Python `watchdog` library, following DD-INFRA-001 pattern.

### Scope

| Field | Hot-Reload | Use Case |
|-------|------------|----------|
| `llm.model` | ✅ | Cost/quality switching |
| `llm.provider` | ✅ | Provider failover |
| `llm.endpoint` | ✅ | Endpoint switching |
| `llm.max_retries` | ✅ | Retry tuning |
| `llm.timeout_seconds` | ✅ | Timeout adjustment |
| `llm.temperature` | ✅ | Response tuning |
| `toolsets.*` | ✅ | Feature toggles |
| `log_level` | ✅ | Debug enablement |

---

## Implementation Phases

### Phase 1: Dependencies & FileWatcher (TDD RED)
**Duration**: 1.5 hours
**Status**: ⏳ Pending

#### Tasks
- [ ] Add `watchdog>=3.0.0,<4.0.0` to `requirements.txt`
- [ ] Create `tests/unit/test_file_watcher.py` with failing tests
- [ ] Test cases:
  - `test_file_watcher_initial_load`
  - `test_file_watcher_detects_change`
  - `test_file_watcher_debounces_rapid_changes`
  - `test_file_watcher_tracks_hash`
  - `test_file_watcher_graceful_on_invalid_file`
  - `test_file_watcher_stop_cleanup`

#### Acceptance Criteria
- [ ] All tests written and failing (RED)
- [ ] Test coverage for happy path and error scenarios

---

### Phase 2: FileWatcher Implementation (TDD GREEN)
**Duration**: 2 hours
**Status**: ⏳ Pending

#### Tasks
- [ ] Create `src/config/hot_reload.py`
- [ ] Implement `FileWatcher` class:
  ```python
  class FileWatcher:
      def __init__(self, path: str, callback: Callable[[str], None], logger: logging.Logger)
      def start(self) -> None
      def stop(self) -> None
      @property
      def last_hash(self) -> str
      @property
      def reload_count(self) -> int
      @property
      def error_count(self) -> int
  ```
- [ ] Implement debouncing (200ms)
- [ ] Implement hash tracking (SHA256)
- [ ] Implement graceful degradation

#### Acceptance Criteria
- [ ] All FileWatcher tests passing (GREEN)
- [ ] Hash changes trigger callback
- [ ] Rapid changes debounced
- [ ] Invalid file doesn't crash

---

### Phase 3: ConfigManager (TDD RED → GREEN)
**Duration**: 2 hours
**Status**: ⏳ Pending

#### Tasks
- [ ] Create `tests/unit/test_config_manager.py` with failing tests
- [ ] Test cases:
  - `test_config_manager_get_llm_model`
  - `test_config_manager_get_llm_provider`
  - `test_config_manager_get_toolsets`
  - `test_config_manager_thread_safe_access`
  - `test_config_manager_reload_updates_values`
  - `test_config_manager_invalid_yaml_keeps_previous`
  - `test_config_manager_missing_field_uses_default`
- [ ] Implement `ConfigManager` class:
  ```python
  class ConfigManager:
      def __init__(self, path: str, logger: logging.Logger)
      def start(self) -> None
      def stop(self) -> None
      def get_llm_model(self) -> str
      def get_llm_provider(self) -> str
      def get_llm_endpoint(self) -> Optional[str]
      def get_llm_max_retries(self) -> int
      def get_llm_timeout(self) -> int
      def get_llm_temperature(self) -> float
      def get_toolsets(self) -> Dict[str, Any]
      def get_log_level(self) -> str
  ```
- [ ] Thread-safe with `threading.RLock`
- [ ] YAML validation on reload

#### Acceptance Criteria
- [ ] All ConfigManager tests passing
- [ ] Thread-safe concurrent access
- [ ] Graceful degradation on invalid YAML

---

### Phase 4: Integration with main.py (TDD GREEN)
**Duration**: 1 hour
**Status**: ⏳ Pending

#### Tasks
- [ ] Update `src/main.py`:
  - Replace static `load_config()` with `ConfigManager`
  - Store `config_manager` in `app.state`
  - Start watcher on startup, stop on shutdown
- [ ] Add startup/shutdown lifecycle hooks

#### Code Changes
```python
# src/main.py
from src.config.hot_reload import ConfigManager

# Global config manager
config_manager: Optional[ConfigManager] = None

@app.on_event("startup")
async def startup_event():
    global config_manager
    config_manager = ConfigManager(
        path=os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml"),
        logger=logger
    )
    config_manager.start()
    app.state.config_manager = config_manager

@app.on_event("shutdown")
async def shutdown_event():
    if config_manager:
        config_manager.stop()
```

#### Acceptance Criteria
- [ ] ConfigManager starts with app
- [ ] ConfigManager stops on shutdown
- [ ] Config accessible via `app.state.config_manager`

---

### Phase 5: Integration with Business Logic (TDD GREEN)
**Duration**: 1 hour
**Status**: ⏳ Pending

#### Tasks
- [ ] Update `src/extensions/incident.py`:
  - Use `config_manager.get_llm_model()` instead of env/dict
  - Use `config_manager.get_toolsets()`
- [ ] Update `src/extensions/recovery.py`:
  - Same changes as incident.py
- [ ] Update `src/extensions/llm_config.py`:
  - Accept `ConfigManager` instead of dict

#### Code Changes
```python
# src/extensions/incident.py
def analyze_incident(request: Request, ...):
    config_manager = request.app.state.config_manager
    model_name = config_manager.get_llm_model()
    toolsets = config_manager.get_toolsets()
    # ...
```

#### Acceptance Criteria
- [ ] Business logic uses ConfigManager
- [ ] Hot-reload affects new requests
- [ ] In-flight requests use config at request time

---

### Phase 6: Metrics (TDD GREEN)
**Duration**: 0.5 hours
**Status**: ⏳ Pending

#### Tasks
- [ ] Add Prometheus metrics to `src/middleware/metrics.py`:
  ```python
  holmesgpt_config_reload_total = Counter(
      "holmesgpt_config_reload_total",
      "Total successful config reloads"
  )
  holmesgpt_config_reload_errors_total = Counter(
      "holmesgpt_config_reload_errors_total",
      "Total failed reload attempts"
  )
  holmesgpt_config_last_reload_timestamp = Gauge(
      "holmesgpt_config_last_reload_timestamp",
      "Unix timestamp of last successful reload"
  )
  ```
- [ ] Wire metrics to ConfigManager callbacks

#### Acceptance Criteria
- [ ] Metrics exposed at `/metrics`
- [ ] Reload count increments on success
- [ ] Error count increments on failure

---

### Phase 7: Integration Tests
**Duration**: 1 hour
**Status**: ⏳ Pending

#### Tasks
- [ ] Create `tests/integration/test_hot_reload_integration.py`:
  - `test_config_reload_affects_new_requests`
  - `test_invalid_config_graceful_degradation`
  - `test_rapid_config_updates_debounced`
  - `test_metrics_exposed_on_reload`

#### Acceptance Criteria
- [ ] Integration tests pass
- [ ] End-to-end reload flow validated

---

## Test Summary

| Phase | Test File | Test Count |
|-------|-----------|------------|
| 1-2 | `test_file_watcher.py` | 6 |
| 3 | `test_config_manager.py` | 7 |
| 7 | `test_hot_reload_integration.py` | 4 |
| **Total** | | **17** |

---

## Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `requirements.txt` | Modify | Add `watchdog>=3.0.0,<4.0.0` |
| `src/config/hot_reload.py` | Create | FileWatcher, ConfigManager classes |
| `src/main.py` | Modify | Use ConfigManager, lifecycle hooks |
| `src/extensions/incident.py` | Modify | Use ConfigManager |
| `src/extensions/recovery.py` | Modify | Use ConfigManager |
| `src/extensions/llm_config.py` | Modify | Accept ConfigManager |
| `src/middleware/metrics.py` | Modify | Add reload metrics |
| `tests/unit/test_file_watcher.py` | Create | FileWatcher unit tests |
| `tests/unit/test_config_manager.py` | Create | ConfigManager unit tests |
| `tests/integration/test_hot_reload_integration.py` | Create | Integration tests |

---

## Rollback Plan

If hot-reload causes issues:
1. Set `HOT_RELOAD_ENABLED=false` env var (feature flag)
2. ConfigManager falls back to static load
3. No code changes required for rollback

---

## Completion Checklist

- [ ] Phase 1: Dependencies & FileWatcher tests (RED)
- [ ] Phase 2: FileWatcher implementation (GREEN)
- [ ] Phase 3: ConfigManager (RED → GREEN)
- [ ] Phase 4: Integration with main.py
- [ ] Phase 5: Integration with business logic
- [ ] Phase 6: Metrics
- [ ] Phase 7: Integration tests
- [ ] All unit tests passing
- [ ] All integration tests passing
- [ ] Documentation updated
- [ ] Code committed

---

**Estimated Total**: ~9 hours
**Confidence**: 88%


