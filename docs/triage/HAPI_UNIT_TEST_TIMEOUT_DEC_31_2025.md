# HolmesGPT API Unit Test Performance Issue - CRITICAL

**Date**: December 31, 2025  
**Issue**: Unit tests taking 5m14s (314 seconds) - UNACCEPTABLE  
**Expected**: <10 seconds for 557 unit tests  
**Root Cause**: Same `time.sleep()` anti-pattern we just fixed in Go notification tests

---

## 🚨 CRITICAL PERFORMANCE ISSUE

### Current State
- **CI Duration**: 5m14s (314 seconds)
- **Test Count**: 557 unit tests
- **Average per test**: 0.56 seconds (should be <0.02s)
- **Status**: Tests passing but TOO SLOW

### Evidence from CI Logs
```
Unit Tests (holmesgpt-api)	Run unit tests (Python)	2025-12-31T19:33:55.9395737Z ============================= test session starts ==============================
Unit Tests (holmesgpt-api)	Run unit tests (Python)	2025-12-31T19:33:55.9396806Z platform linux -- Python 3.12.12, pytest-7.4.3, pluggy-1.6.0
...
[Tests running for ~5 minutes]
```

### Comparison to Fixed Go Tests
| Service | Before Fix | After Fix | Improvement |
|---------|-----------|-----------|-------------|
| **Notification (Go)** | 251s | 5s | **98% faster** |
| **HAPI (Python)** | 314s | ??? | **NEEDS FIX** |

---

## 📊 ROOT CAUSE ANALYSIS

### Known Anti-Patterns from Previous Analysis
From `docs/triage/HAPI_UNIT_TEST_PERFORMANCE_DEC_31_2025.md`:

**33 `time.sleep()` calls identified**:
1. `test_mock_llm_responses.py` - 15 calls (0.1s each = 1.5s)
2. `test_recovery_endpoint.py` - 8 calls (0.1-0.5s each = ~2s)
3. `test_incident_endpoint.py` - 6 calls (0.1s each = 0.6s)
4. `test_workflow_endpoint.py` - 4 calls (0.1s each = 0.4s)

**Total identified sleep time**: ~4.5 seconds  
**Actual total time**: 314 seconds  
**Conclusion**: There are MORE `time.sleep()` calls we haven't found yet, OR tests are doing expensive I/O operations.

---

## 🔍 INVESTIGATION NEEDED

### Step 1: Run with `--durations=50` to identify slowest tests
```bash
make test-unit-holmesgpt-api
# Already includes --durations=20, need to check output
```

### Step 2: Search for ALL `time.sleep()` calls
```bash
cd holmesgpt-api
grep -r "time.sleep" tests/unit/ --include="*.py" -n | wc -l
grep -r "asyncio.sleep" tests/unit/ --include="*.py" -n | wc -l
grep -r "sleep(" tests/unit/ --include="*.py" -n | wc -l
```

### Step 3: Identify I/O-heavy tests
- File system operations
- Network calls (even mocked ones with delays)
- Database operations (even in-memory)

---

## 🎯 PROPOSED SOLUTIONS

### Solution 1: Remove ALL `time.sleep()` calls (PRIORITY 1)
**Target**: Eliminate all 33+ identified `time.sleep()` calls

**Pattern**: Replace synchronous waits with:
```python
# ❌ BAD: time.sleep() anti-pattern
time.sleep(0.1)
assert response.status == 200

# ✅ GOOD: Direct assertion (unit tests should be synchronous)
assert response.status == 200

# ✅ GOOD: For async tests, use pytest-asyncio properly
@pytest.mark.asyncio
async def test_async_behavior():
    result = await async_function()
    assert result == expected
```

**Expected Impact**: 4.5s → 0s (98% reduction in identified sleeps)

### Solution 2: Mock expensive operations (PRIORITY 2)
**Target**: Replace real I/O with instant mocks

**Pattern**:
```python
# ❌ BAD: Real file I/O in unit tests
def test_config_loading():
    config = load_config_from_file("config.yaml")
    assert config["key"] == "value"

# ✅ GOOD: Mock file I/O
@patch("builtins.open", mock_open(read_data="key: value"))
def test_config_loading():
    config = load_config_from_file("config.yaml")
    assert config["key"] == "value"
```

**Expected Impact**: Variable, but could save 10-50s

### Solution 3: Use pytest-benchmark for timing-sensitive tests (PRIORITY 3)
**Target**: Tests that validate timing behavior without actual delays

**Pattern**:
```python
# ❌ BAD: Testing retry timing with real sleeps
def test_retry_backoff():
    start = time.time()
    retry_with_backoff()
    duration = time.time() - start
    assert duration > 0.5  # Waits 0.5s in test

# ✅ GOOD: Mock time and test logic
@patch("time.sleep")
def test_retry_backoff(mock_sleep):
    retry_with_backoff()
    mock_sleep.assert_called_with(0.5)  # Instant test
```

**Expected Impact**: 50-100s reduction

---

## 📋 ACTION PLAN

### Phase 1: Quick Wins (Target: <30s total)
1. ✅ Run `make test-unit-holmesgpt-api` locally with `--durations=50`
2. ✅ Identify top 10 slowest tests
3. ✅ Fix `time.sleep()` in those tests first
4. ✅ Re-run and measure improvement

### Phase 2: Systematic Cleanup (Target: <10s total)
1. ✅ Search and replace ALL `time.sleep()` calls
2. ✅ Mock all file I/O operations
3. ✅ Mock all network calls (even to mock LLM)
4. ✅ Verify no database I/O in unit tests

### Phase 3: Validation (Target: <5s total)
1. ✅ Run full suite: `make test-unit-holmesgpt-api`
2. ✅ Verify all 557 tests pass
3. ✅ Verify total time <10s
4. ✅ Commit and push

---

## 🎯 SUCCESS CRITERIA

- [ ] **All 557 tests pass**
- [ ] **Total duration <10 seconds** (currently 314s)
- [ ] **No `time.sleep()` calls in unit tests** (except for mocking)
- [ ] **No real I/O operations in unit tests**
- [ ] **CI job completes in <30s** (including container setup)

---

## 📝 NEXT STEPS

**IMMEDIATE**: Run local tests with detailed timing to identify the worst offenders:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-unit-holmesgpt-api 2>&1 | tee hapi-unit-timing.log
```

**THEN**: Triage the slowest tests and fix them systematically.

---

## 🔗 RELATED DOCUMENTS

- `docs/triage/CI_NOTIFICATION_TEST_PERFORMANCE_DEC_31_2025.md` - Go notification test fixes (same anti-pattern)
- `docs/triage/HAPI_UNIT_TEST_PERFORMANCE_DEC_31_2025.md` - Initial HAPI analysis (33 `time.sleep()` calls)
- `docs/development/business-requirements/TESTING_GUIDELINES.md` - Explicitly forbids `time.sleep()` in tests

