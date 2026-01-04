# Notification Unit Tests - Remaining Performance Issues - Dec 31, 2025

## 🎯 **Status**

**✅ FIXED**: Critical O(n²) bugs (76s + 15s = 91s savings)
**⚠️ REMAINING**: Tests still take ~2+ minutes (target: 30-60 seconds)

---

## 📊 **Current State After Fixes**

### **Fixed Issues** ✅
- ✅ O(n²) 1MB string building: 76s → 0.003s
- ✅ O(n²) 15KB string building: 15s → 0.003s
- ✅ 6× `time.Sleep()` anti-patterns: ~3.4s total savings
- **Total fixed**: ~94 seconds

### **Remaining Issues** ⚠️
- **Observed**: Tests still taking 2+ minutes
- **Expected**: Unit tests should complete in 30-60 seconds
- **Gap**: ~60-90 seconds of unexplained slowness

---

## 🔍 **Suspected Remaining Issues**

### **1. File I/O Operations** (80% confidence)

**Evidence**:
```bash
$ find test/unit/notification -name "*_test.go" -exec grep -l "os.Create\|ioutil.WriteFile\|os.MkdirAll" {} \;
test/unit/notification/file_delivery_test.go
test/unit/notification/routing_config_test.go
```

**Hypothesis**: Tests doing real file system operations

**Files to investigate**:
- `file_delivery_test.go` (303 lines) - File creation tests
- `routing_config_test.go` (668 lines) - Config file operations
- `routing_hotreload_test.go` (403 lines) - File watching tests

**Potential Fix**: Use in-memory filesystem (`github.com/spf13/afero`)

```go
// ❌ CURRENT: Real file I/O
func TestFileDelivery(t *testing.T) {
    tmpDir := os.TempDir()
    service := NewFileDeliveryService(tmpDir)
    // Writes to actual disk
}

// ✅ BETTER: In-memory filesystem
func TestFileDelivery(t *testing.T) {
    fs := afero.NewMemMapFs()
    service := NewFileDeliveryService("/output", fs)
    // Writes to memory
}
```

---

### **2. Large Test Suite (239 specs in one file)** (60% confidence)

**Evidence**:
```
audit_test.go: 801 lines, likely 239 specs
```

**Impact**: Even fast tests add up when there are 239 of them

**Potential Fix**: Profile to identify slowest tests

```bash
cd test/unit/notification
ginkgo -v --cpuprofile=cpu.prof --slowSpecThreshold=0.1

# Analyze profile
go tool pprof -top cpu.prof
go tool pprof -list="." cpu.prof
```

---

### **3. Ginkgo Parallel Overhead** (40% confidence)

**Evidence**: Tests run with 4 parallel processes

**Hypothesis**: Parallel process coordination adds overhead for small, fast tests

**Potential Fix**: Try sequential execution for comparison

```bash
# Sequential (no parallelism)
time make test-unit-notification GINKGO_PROCS=1

# vs Current (4 processes)
time make test-unit-notification GINKGO_PROCS=4
```

If sequential is faster, the tests are too granular for parallel benefits.

---

### **4. Buffered Output Delay** (20% confidence)

**Evidence**: Tests appear "stuck" but are actually running

**Hypothesis**: Ginkgo output buffering makes tests seem slower than they are

**Not Actually Slow**: Just perception issue due to buffered output

---

## 🎯 **Recommended Investigation Steps**

### **Step 1: Profile the Tests** (Do First)

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Run with profiling
cd test/unit/notification
ginkgo -v --cpuprofile=cpu.prof --memprofile=mem.prof

# Analyze CPU profile
go tool pprof -top cpu.prof | head -20

# Analyze memory profile
go tool pprof -top mem.prof | head -20

# Find slow specs (>100ms)
ginkgo -v --slowSpecThreshold=0.1 | grep "seconds\]" | sort -t'[' -k2 -rn | head -20
```

**What to look for**:
- File I/O functions: `os.Create`, `os.Open`, `ioutil.WriteFile`
- Syscalls: `syscall.Read`, `syscall.Write`
- Time-related functions: Actual sleep or wait operations

---

### **Step 2: Test Timing Breakdown**

```bash
# Run tests with timing for each spec
ginkgo -v --slowSpecThreshold=0 test/unit/notification/ 2>&1 | \
  grep "\[.*seconds\]" | \
  awk '{print $2, $0}' | \
  sort -rn | \
  head -30 > slow_tests.txt

# Analyze results
cat slow_tests.txt
```

**Expected output**: List of slowest 30 tests with timing

---

### **Step 3: File I/O Investigation**

```bash
# Find all file operations in tests
grep -r "os\.Create\|os\.Open\|ioutil\.\|os\.MkdirAll" \
  test/unit/notification/ --include="*_test.go" -n

# Count file operations
grep -r "os\.Create\|os\.Open\|ioutil\.\|os\.MkdirAll" \
  test/unit/notification/ --include="*_test.go" | wc -l
```

---

### **Step 4: Compare Sequential vs Parallel**

```bash
# Sequential
time (cd test/unit/notification && ginkgo -v --procs=1) 2>&1 | grep "real"

# Parallel (current)
time (cd test/unit/notification && ginkgo -v --procs=4) 2>&1 | grep "real"
```

**Decision**:
- If sequential is **faster**: Tests too small for parallel benefit → reduce procs
- If parallel is **much faster**: Keep current approach, investigate other issues

---

## 📈 **Expected Outcomes After Further Fixes**

### **Conservative Estimate** (file I/O fixes only)
- Current: ~2 minutes (120s)
- After file I/O fixes: ~60-90s
- Improvement: 30-50% faster

### **Optimistic Estimate** (all fixes)
- Current: ~2 minutes (120s)
- After all fixes: ~30-45s
- Improvement: 60-75% faster
- **Matches other services**: Similar to 50-60s target

---

## 🚀 **Action Plan**

### **Immediate** (This Session)
- ✅ Fix O(n²) bugs (DONE - 91s saved)
- ✅ Fix time.Sleep() anti-patterns (DONE - 3.4s saved)
- ✅ Document remaining issues (THIS DOCUMENT)
- ⏳ **Run profiling** to identify actual bottleneck

### **Next Steps** (Follow-Up PR)
1. **Profile tests** to identify top 10 slowest specs
2. **Investigate file I/O** operations
3. **Consider** in-memory filesystem for file tests
4. **Target**: 30-60 second total runtime

---

## 📝 **Quick Commands for User**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Profile notification tests (RECOMMENDED - DO THIS FIRST)
cd test/unit/notification && \
  ginkgo -v --cpuprofile=cpu.prof --slowSpecThreshold=0.1 && \
  go tool pprof -top cpu.prof | head -30

# Quick timing check
time make test-unit-notification 2>&1 | tail -20

# Find slow specs (>100ms)
make test-unit-notification 2>&1 | \
  grep "\[.*seconds\]" | \
  grep -v "\[0.0" | \
  sort -t'[' -k2 -rn | \
  head -20
```

---

## 🎯 **Success Criteria**

**Target**: Unit tests complete in **30-60 seconds total**

**Current**: ~120 seconds (after O(n²) fixes)
**Target**: **30-60 seconds**
**Gap**: 60-90 seconds to optimize

---

## 📚 **References**

- **Fixed Issues**: `docs/triage/CI_NOTIFICATION_TEST_FIXES_DEC_31_2025.md`
- **Original Analysis**: `docs/triage/CI_NOTIFICATION_TEST_PERFORMANCE_DEC_31_2025.md`
- **Commit**: `d841c4a2b` - "fix(notification): Remove O(n²) string building and time.Sleep() anti-patterns"

---

**Analysis Date**: 2025-12-31
**Status**: Partial fix completed, profiling investigation needed
**Next Action**: Run CPU/memory profiling to identify remaining bottleneck

