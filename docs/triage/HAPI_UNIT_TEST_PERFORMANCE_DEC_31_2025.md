# HolmesGPT API Unit Tests - Performance Analysis - Dec 31, 2025

## 🎯 **Current State**

**Test Duration**: 53.88 seconds (557 tests)  
**Target**: 10-15 seconds  
**Gap**: ~40 seconds of unnecessary delays

---

## 📊 **Slowest Tests Analysis**

### **Top 20 Slowest Tests**

| Duration | Test | File | Root Cause |
|----------|------|------|------------|
| 4.52s | `test_graceful_degradation_on_invalid_yaml` | test_hot_reload.py | `time.sleep()` |
| 4.51s | `test_file_watcher_graceful_on_callback_error` | test_file_watcher.py | `time.sleep()` |
| 3.01s | `test_last_hash_updates` | test_config_manager.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_config_reload_reflects_in_getters` | test_hot_reload.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_file_watcher_reload_count` | test_file_watcher.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_reload_count_increments` | test_config_manager.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_file_watcher_error_count` | test_file_watcher.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_config_reload_updates_values` | test_config_manager.py | **`time.sleep(3)`** ❌ |
| 3.01s | `test_invalid_yaml_keeps_previous_config` | test_config_manager.py | **`time.sleep(3)`** ❌ |
| 2.01s | `test_hot_reload_disabled_via_env` | test_hot_reload.py | **`time.sleep(2)`** ❌ |
| 2.01s | `test_disable_hot_reload` | test_config_manager.py | **`time.sleep(2)`** ❌ |
| 1.11s | `test_half_open_after_recovery_timeout` | test_errors.py | **`time.sleep(1.1)`** ❌ |
| 1.11s | `test_half_open_to_closed_on_success` | test_errors.py | **`time.sleep(1.1)`** ❌ |
| 1.08s | `test_file_watcher_debounces_rapid_changes` | test_file_watcher.py | **`time.sleep(1)`** ❌ |

**Pattern Identified**: Exact timings (3.01s, 2.01s, 1.11s) indicate synchronous `time.sleep()` calls!

---

## 🔍 **Root Cause Analysis**

### **`time.sleep()` Distribution**

| File | Count | Use Case | Impact |
|------|-------|----------|--------|
| **test_file_watcher.py** | 14 | File change detection | **~18s waste** |
| **test_config_manager.py** | 10 | Config hot-reload | **~15s waste** |
| **test_hot_reload.py** | 7 | Hot-reload integration | **~10s waste** |
| **test_errors.py** | 2 | Circuit breaker recovery | **~2s waste** |
| **TOTAL** | **33** | File watching/async waits | **~45s waste** |

### **Anti-Pattern Identified**

```python
# ❌ ANTI-PATTERN: Blocking sleep to wait for async file events
def test_config_reload_updates_values():
    # Modify config file
    with open(config_file, "w") as f:
        f.write("new_value: 42")
    
    # Wait for file watcher to detect change
    time.sleep(3)  # ❌ Blocking wait
    
    # Assert value was updated
    assert config_manager.get_value() == 42
```

**Problem**: These tests use **synchronous `time.sleep()`** to wait for **asynchronous file system events**.

---

## 🛠️ **Fix Strategy**

### **Solution 1: Polling with Short Intervals** (Python equivalent of Go's `Eventually()`)

```python
# ✅ BETTER: Poll with short intervals and timeout
def wait_for_condition(check_fn, timeout=1.0, interval=0.01):
    """Poll condition with short intervals instead of blocking sleep."""
    import time
    start = time.time()
    while time.time() - start < timeout:
        if check_fn():
            return True
        time.sleep(interval)  # Short interval (10ms) instead of 3s
    return False

def test_config_reload_updates_values():
    # Modify config file
    with open(config_file, "w") as f:
        f.write("new_value: 42")
    
    # Wait for value to update (max 1s instead of 3s)
    assert wait_for_condition(
        lambda: config_manager.get_value() == 42,
        timeout=1.0
    )
```

**Improvement**: 3 seconds → ~0.1 seconds (**30x faster**)

---

### **Solution 2: Event-Driven Testing** (Best for file watchers)

```python
# ✅ BEST: Event-driven with threading.Event
class MockFileWatcher:
    def __init__(self):
        self.change_detected = threading.Event()
    
    def on_file_change(self, path):
        self.process_change(path)
        self.change_detected.set()

def test_file_watcher_detects_change():
    watcher = MockFileWatcher()
    
    # Modify file
    with open(config_file, "w") as f:
        f.write("new_value: 42")
    
    # Wait for event (max 1s instead of 3s)
    assert watcher.change_detected.wait(timeout=1.0)
```

**Improvement**: 3 seconds → <0.001 seconds (**3000x faster**)

---

### **Solution 3: Fake Time** (Best for circuit breaker tests)

```python
# ✅ BEST: Fake time with freezegun or unittest.mock
from unittest.mock import patch
import time

def test_half_open_after_recovery_timeout():
    circuit_breaker = CircuitBreaker(recovery_timeout=1.0)
    circuit_breaker.open()  # Circuit is open
    
    # Fast-forward time instead of sleeping
    with patch('time.time', return_value=time.time() + 1.1):
        assert circuit_breaker.is_half_open()
```

**Improvement**: 1.1 seconds → <0.001 seconds (**1000x faster**)

---

## 📈 **Expected Performance Improvement**

### **Conservative Estimate** (Polling with short intervals)

| File | Current | After Fix | Improvement |
|------|---------|-----------|-------------|
| test_file_watcher.py | ~18s | ~1.5s | 12x faster |
| test_config_manager.py | ~15s | ~1.2s | 12.5x faster |
| test_hot_reload.py | ~10s | ~1.0s | 10x faster |
| test_errors.py | ~2s | ~0.1s | 20x faster |
| **TOTAL** | **~45s** | **~4s** | **11x faster** |

**Result**: 53.88s → **~13s** (4x faster, **75% reduction**)

### **Optimistic Estimate** (Event-driven + Fake time)

| File | Current | After Fix | Improvement |
|------|---------|-----------|-------------|
| test_file_watcher.py | ~18s | ~0.5s | 36x faster |
| test_config_manager.py | ~15s | ~0.4s | 37.5x faster |
| test_hot_reload.py | ~10s | ~0.3s | 33x faster |
| test_errors.py | ~2s | ~0.01s | 200x faster |
| **TOTAL** | **~45s** | **~1.2s** | **37x faster** |

**Result**: 53.88s → **~9s** (6x faster, **83% reduction**)

---

## 🎯 **Implementation Priority**

### **Phase 1: Quick Wins** (Polling helper)
1. Create `wait_for_condition()` helper utility
2. Replace all `time.sleep(3)` with `wait_for_condition()` calls
3. Target: 53s → ~13s (4x faster)

### **Phase 2: Event-Driven** (Optimal solution)
1. Add event signals to file watcher callbacks
2. Replace polling with event waits
3. Target: 13s → ~9s (1.4x additional speedup)

### **Phase 3: Fake Time** (Circuit breaker tests)
1. Use `freezegun` or `unittest.mock` for time-dependent tests
2. Target: Minimal additional improvement (~0.1s)

---

## 🔍 **Files to Modify**

### **Priority 1: test_file_watcher.py** (14 `time.sleep()` calls)
```bash
grep -n "time\.sleep" holmesgpt-api/tests/unit/test_file_watcher.py

# Replace each with wait_for_condition()
```

### **Priority 2: test_config_manager.py** (10 `time.sleep()` calls)
```bash
grep -n "time\.sleep" holmesgpt-api/tests/unit/test_config_manager.py

# Replace each with wait_for_condition()
```

### **Priority 3: test_hot_reload.py** (7 `time.sleep()` calls)
```bash
grep -n "time\.sleep" holmesgpt-api/tests/unit/test_hot_reload.py

# Replace each with wait_for_condition()
```

### **Priority 4: test_errors.py** (2 `time.sleep()` calls)
```bash
grep -n "time\.sleep" holmesgpt-api/tests/unit/test_errors.py

# Use freezegun or mock time
```

---

## 📝 **Testing Strategy**

### **Validation Commands**

```bash
# Run with duration reporting
cd holmesgpt-api
python3 -m pytest tests/unit/ -v --durations=20

# Focus on specific slow tests
python3 -m pytest tests/unit/test_file_watcher.py -v --durations=0
python3 -m pytest tests/unit/test_config_manager.py -v --durations=0

# Verify no time.sleep() anti-patterns remain
grep -r "time\.sleep" tests/unit/
```

---

## ✅ **Success Criteria**

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| Total runtime | 53.88s | **10-15s** | ⏳ Pending |
| Slowest test | 4.52s | **<0.5s** | ⏳ Pending |
| `time.sleep()` count | 33 | **0** (except simulating external delays) | ⏳ Pending |
| Test reliability | Good | Excellent | ⏳ Pending |

---

## 🚀 **CI Impact**

**Before**: 53.88 seconds  
**After (Conservative)**: ~13 seconds  
**After (Optimistic)**: ~9 seconds

**Improvement**: 4-6x faster, 75-83% time reduction

**Expected CI time** (with parallelization from notification service lessons):
- Sequential: ~9-13s
- Parallel (if needed): ~5-7s

---

## 📚 **References**

- **Similar Issue**: Notification unit tests (120s → 5s via time.sleep() removal)
- **Pattern**: `docs/triage/CI_NOTIFICATION_TEST_PERFORMANCE_FINAL_DEC_31_2025.md`
- **Python Polling Example**: pytest-eventually, tenacity library
- **Fake Time**: freezegun library for Python

---

**Analysis Date**: 2025-12-31  
**Status**: Analysis complete, implementation pending  
**Next Action**: Implement Phase 1 (polling helper) for quick wins

