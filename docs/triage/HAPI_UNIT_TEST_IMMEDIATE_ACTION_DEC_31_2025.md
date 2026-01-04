# HAPI Unit Tests - IMMEDIATE ACTION REQUIRED

**Date**: December 31, 2025
**Status**: 🚨 **CRITICAL PERFORMANCE ISSUE**
**Duration**: 5m14s (CI) / 30min+ (local) - **UNACCEPTABLE**

---

## 🚨 CRITICAL SITUATION

### Current State
- **CI Duration**: 5m14s (314 seconds)
- **Local Duration**: 30+ minutes (extrapolated from 3min for 10%)
- **Test Count**: 557 tests
- **Root Cause**: 33 `time.sleep()` calls (documented in `HAPI_UNIT_TEST_PERFORMANCE_DEC_31_2025.md`)

### Why This Is CRITICAL
1. **Blocks CI/CD**: Every PR waits 5+ minutes for HAPI tests
2. **Developer Productivity**: Local testing is impossible (30 minutes!)
3. **Anti-Pattern Violation**: Explicitly forbidden by `TESTING_GUIDELINES.md`
4. **Same Issue We Just Fixed**: Notification tests had identical problem (251s → 5s)

---

## 📊 ROOT CAUSE (ALREADY ANALYZED)

From `docs/triage/HAPI_UNIT_TEST_PERFORMANCE_DEC_31_2025.md`:

### Identified `time.sleep()` Calls
| File | Count | Total Waste |
|------|-------|-------------|
| `test_file_watcher.py` | 14 | ~18s |
| `test_config_manager.py` | 10 | ~15s |
| `test_hot_reload.py` | 7 | ~10s |
| `test_errors.py` | 2 | ~2s |
| **TOTAL** | **33** | **~45s** |

### Worst Offenders
- `time.sleep(3)` - 8 occurrences = 24s
- `time.sleep(2)` - 2 occurrences = 4s
- `time.sleep(1.1)` - 2 occurrences = 2.2s
- `time.sleep(1)` - 1 occurrence = 1s
- `time.sleep(0.5)` - multiple occurrences = ~5s

---

## 🎯 IMMEDIATE FIX STRATEGY

### Option A: Quick Fix (Polling Helper) - 1 hour
**Target**: 53s → ~13s (4x faster)

1. Create `wait_for_condition()` helper in `tests/unit/conftest.py`
2. Replace all 33 `time.sleep()` calls with polling
3. Run tests to verify

**Pros**: Fast to implement, proven pattern
**Cons**: Still has some delays (polling overhead)

### Option B: Optimal Fix (Event-Driven) - 3 hours
**Target**: 53s → ~9s (6x faster)

1. Add event signals to file watcher
2. Use `threading.Event` for synchronization
3. Use `freezegun` for time-dependent tests

**Pros**: Maximum performance, best practice
**Cons**: More complex, requires refactoring

### Option C: Hybrid (RECOMMENDED) - 2 hours
**Target**: 53s → ~10s (5x faster)

1. **Phase 1** (30 min): Polling helper for file watcher tests (27 calls)
2. **Phase 2** (30 min): Fake time for circuit breaker tests (2 calls)
3. **Phase 3** (1 hour): Event-driven for most critical tests (optional)

**Pros**: Balanced approach, iterative improvement
**Cons**: None

---

## 🛠️ IMPLEMENTATION PLAN (HYBRID APPROACH)

### Step 1: Create Polling Helper (15 minutes)

**File**: `holmesgpt-api/tests/unit/conftest.py`

```python
import time
import pytest

def wait_for_condition(check_fn, timeout=1.0, interval=0.01, error_msg="Condition not met"):
    """
    Poll a condition with short intervals instead of blocking sleep.

    Args:
        check_fn: Callable that returns True when condition is met
        timeout: Maximum time to wait in seconds (default: 1.0)
        interval: Polling interval in seconds (default: 0.01 = 10ms)
        error_msg: Error message if timeout is reached

    Returns:
        True if condition met, raises AssertionError if timeout

    Example:
        wait_for_condition(lambda: config.value == 42, timeout=1.0)
    """
    start = time.time()
    while time.time() - start < timeout:
        if check_fn():
            return True
        time.sleep(interval)
    raise AssertionError(f"{error_msg} (timeout after {timeout}s)")

@pytest.fixture
def wait_for():
    """Fixture to provide wait_for_condition helper to tests."""
    return wait_for_condition
```

### Step 2: Fix File Watcher Tests (30 minutes)

**File**: `holmesgpt-api/tests/unit/test_file_watcher.py`

**Pattern**:
```python
# ❌ BEFORE (14 occurrences)
def test_file_watcher_detects_change():
    modify_config_file()
    time.sleep(3)  # Wait for file watcher
    assert config.value == 42

# ✅ AFTER
def test_file_watcher_detects_change(wait_for):
    modify_config_file()
    wait_for(lambda: config.value == 42, timeout=1.0)
```

**Commands**:
```bash
cd holmesgpt-api/tests/unit
# Find all time.sleep() calls
grep -n "time\.sleep" test_file_watcher.py

# Replace each with wait_for()
# (Manual replacement or sed script)
```

### Step 3: Fix Config Manager Tests (30 minutes)

**File**: `holmesgpt-api/tests/unit/test_config_manager.py`

**Same pattern as Step 2**, 10 occurrences to fix.

### Step 4: Fix Hot Reload Tests (20 minutes)

**File**: `holmesgpt-api/tests/unit/test_hot_reload.py`

**Same pattern as Step 2**, 7 occurrences to fix.

### Step 5: Fix Circuit Breaker Tests (20 minutes)

**File**: `holmesgpt-api/tests/unit/test_errors.py`

**Pattern**:
```python
# ❌ BEFORE
def test_half_open_after_recovery_timeout():
    circuit_breaker.open()
    time.sleep(1.1)  # Wait for recovery timeout
    assert circuit_breaker.is_half_open()

# ✅ AFTER (using freezegun)
from freezegun import freeze_time
import time

def test_half_open_after_recovery_timeout():
    circuit_breaker.open()
    with freeze_time(lambda: time.time() + 1.1):
        assert circuit_breaker.is_half_open()
```

**Note**: If `freezegun` not available, use polling helper as fallback.

### Step 6: Validate (10 minutes)

```bash
cd holmesgpt-api
python3 -m pytest tests/unit/ -v --durations=20

# Expected results:
# - Total time: <15 seconds
# - Slowest test: <0.5 seconds
# - All 557 tests passing
```

---

## 📋 ACCEPTANCE CRITERIA

- [ ] All 557 tests pass
- [ ] Total duration <15 seconds (currently 314s)
- [ ] No test takes >0.5 seconds (currently 4.5s max)
- [ ] Zero `time.sleep()` calls >0.1s in unit tests
- [ ] CI job completes in <1 minute (including container setup)

---

## 🚀 EXECUTION DECISION

**RECOMMENDATION**: **Option C (Hybrid)** - Start immediately

**Why**:
1. We have complete analysis already done
2. Pattern is identical to notification tests we just fixed
3. 2-hour investment saves 5+ minutes on EVERY CI run
4. Local development becomes feasible again

**Alternative**: If time-constrained, do **Option A (Quick Fix)** first, then optimize later.

---

## 📝 NEXT IMMEDIATE ACTIONS

1. **STOP** waiting for local test run (it will take 30 minutes)
2. **CREATE** `wait_for_condition()` helper in `conftest.py`
3. **FIX** `test_file_watcher.py` first (biggest impact: 18s → ~1s)
4. **RUN** tests after each file to verify
5. **COMMIT** and push when all tests pass

---

## 🔗 RELATED DOCUMENTS

- `docs/triage/HAPI_UNIT_TEST_PERFORMANCE_DEC_31_2025.md` - Complete analysis
- `docs/triage/CI_NOTIFICATION_TEST_PERFORMANCE_DEC_31_2025.md` - Similar fix (Go)
- `docs/development/business-requirements/TESTING_GUIDELINES.md` - Forbids `time.sleep()`

---

**Status**: Ready for immediate implementation
**Estimated Time**: 2 hours
**Expected Improvement**: 314s → ~10-15s (20x faster)

