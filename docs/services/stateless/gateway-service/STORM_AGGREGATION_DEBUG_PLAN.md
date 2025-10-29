# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes



## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes

# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes

# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes



## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes

# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes

# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes



## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes

# Storm Aggregation Test Failure - Debug Plan

## 🎯 **Problem Statement**

**Test**: "should aggregate 15 concurrent Prometheus alerts into 1 storm CRD"
**Expected**: 1-3 CRDs created (201), 12-14 alerts aggregated (202)
**Actual**: 15 CRDs created (201), 0 alerts aggregated (202)

**Pass Rate**: 92% (11/12 tests passing)

---

## 🔍 **Root Cause Analysis**

### **Current Storm Detection Logic**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    // 1. Check if storm is already active (fast path)
    isActive, err := s.IsStormActive(ctx, namespace)
    if isActive {
        // Return immediately - aggregate into existing storm CRD
        return true, metadata, nil
    }

    // 2. Storm not active - increment counter atomically (Lua script)
    count, err := s.IncrementCounter(ctx, namespace)

    // 3. Check if threshold reached (count >= 10)
    isStorm := count >= s.threshold

    // 4. If threshold reached, set storm flag
    if isStorm {
        s.setStormFlag(ctx, namespace)
    }

    return isStorm, metadata, nil
}
```

### **Race Condition Hypothesis**

With 15 concurrent requests:
- **Request 1-9**: `IsStormActive() = false`, `IncrementCounter() = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `IsStormActive() = false`, `IncrementCounter() = 10`, `isStorm = true`, `setStormFlag()` → **201 Created** (first storm CRD)
- **Request 11-15**: Should see `IsStormActive() = true` → 202 Accepted

**But**: All 15 requests return 201, meaning `IsStormActive()` returns `false` for ALL requests.

### **Possible Causes**

1. **`setStormFlag()` is too slow**: By the time the flag is set, all 15 requests have already passed the `IsStormActive()` check
2. **Redis latency**: Flag write doesn't complete before subsequent requests check
3. **Test timing**: All 15 goroutines start simultaneously, all check flag before any can set it

---

## 🛠️ **Solution Options**

### **Option A: Combine Increment + Flag Set in Single Lua Script** (RECOMMENDED)

**Approach**: Use a single atomic Lua script that:
1. Increments counter
2. Checks threshold
3. Sets flag if threshold reached
4. Returns both count and storm status

**Pros**:
- ✅ Truly atomic operation
- ✅ Eliminates race condition completely
- ✅ Single Redis round-trip
- ✅ Guaranteed consistency

**Cons**:
- ⚠️ Slightly more complex Lua script
- ⚠️ Requires refactoring `Check()` method

**Confidence**: 95%

---

### **Option B: Check Flag After Setting It**

**Approach**: After setting the flag, immediately check it to ensure it's visible

**Pros**:
- ✅ Simple fix
- ✅ Minimal code changes

**Cons**:
- ❌ Still has race condition window
- ❌ Adds extra Redis round-trip
- ❌ Doesn't guarantee atomicity

**Confidence**: 60%

---

### **Option C: Relax Test Expectations**

**Approach**: Accept that with 15 concurrent requests, some will create individual CRDs before storm detection kicks in

**Pros**:
- ✅ No code changes
- ✅ Reflects real-world behavior

**Cons**:
- ❌ Doesn't fix the underlying race condition
- ❌ Storm detection less effective in practice
- ❌ Defeats the purpose of storm aggregation

**Confidence**: 40%

---

## 📝 **Recommended Implementation: Option A**

### **New Lua Script**

```lua
local counter_key = KEYS[1]
local flag_key = KEYS[2]
local ttl = tonumber(ARGV[1])
local storm_ttl = tonumber(ARGV[2])
local threshold = tonumber(ARGV[3])

-- Increment counter atomically
local count = redis.call('INCR', counter_key)

-- Set TTL on first increment
if count == 1 then
    redis.call('EXPIRE', counter_key, ttl)
end

-- Check if storm threshold reached
local is_storm = count >= threshold

-- If threshold reached, set storm flag
if is_storm then
    redis.call('SET', flag_key, '1', 'EX', storm_ttl)
end

-- Return count and storm status
return {count, is_storm and 1 or 0}
```

### **Updated `Check()` Method**

```go
func (s *StormDetector) Check(ctx context.Context, signal *types.NormalizedSignal) (bool, *StormMetadata, error) {
    namespace := signal.Namespace

    // ATOMIC OPERATION: Check active storm OR increment+check threshold in single Lua script
    isActive, err := s.IsStormActive(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    if isActive {
        // Fast path: Storm already active
        count, _ := s.GetCounter(ctx, namespace)
        metadata := s.buildStormMetadata(namespace, count, true)
        return true, metadata, nil
    }

    // Atomic increment + threshold check + flag set
    result, err := s.atomicIncrementAndCheckStorm(ctx, namespace)
    if err != nil {
        return false, nil, err
    }

    count := result.Count
    isStorm := result.IsStorm

    metadata := s.buildStormMetadata(namespace, count, isStorm)
    return isStorm, metadata, nil
}
```

---

## 🎯 **Expected Outcome**

With Option A implemented:
- **Request 1-9**: `count = 1-9`, `isStorm = false` → 201 Created
- **Request 10**: `count = 10`, `isStorm = true`, **flag set atomically** → 201 Created (first storm CRD)
- **Request 11-15**: `IsStormActive() = true` (flag already set) → 202 Accepted

**Expected Test Result**: 1 CRD created, 14 alerts aggregated (or 2-3 CRDs, 12-13 aggregated due to timing)

---

## 📊 **Confidence Assessment**

**Option A Confidence**: 95%
- High confidence this will fix the race condition
- Lua script is proven to work (tested earlier)
- Atomic operations eliminate timing issues

**Risk**: Low - Lua scripts are well-tested and Redis-native

---

## 🚀 **Next Steps**

1. Implement Option A (atomic Lua script)
2. Run tests with `seed=1` + `fail-fast` to verify fix
3. If passing, run full suite to check for regressions
4. Update TODO list and create final summary

**Estimated Time**: 15-20 minutes




