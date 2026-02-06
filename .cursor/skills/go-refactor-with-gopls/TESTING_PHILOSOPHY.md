# Testing Philosophy: Behavior vs Implementation

## Summary of Refactoring

The test suite has been refactored to focus on **BEHAVIOR and CORRECTNESS** rather than **IMPLEMENTATION DETAILS**.

**All 40 tests pass** ✅

---

## What Changed

### ❌ Before: Testing Implementation Details

```bash
@test "rename command succeeds" {
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BAD: Testing specific log messages (implementation)
    [[ "$output" =~ "Renaming symbol" ]]
    [[ "$output" =~ "Validating build" ]]
    [[ "$output" =~ "Build successful" ]]
}
```

**Problems:**
- Brittle: Breaks when log messages change
- Over-specified: Tests HOW not WHAT
- Couples tests to internal logging
- No actual verification of behavior

### ✅ After: Testing Behavior

```bash
@test "rename: succeeds when gopls succeeds and validation passes" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # GOOD: Testing observable behavior
    [ "$status" -eq 0 ]              # Success outcome
    [ -f "$GOPLS_CALLED" ]           # gopls was invoked
    [ -f "$GO_BUILD_CALLED" ]        # Validation ran
    [ -f "$GO_TEST_CALLED" ]         # Tests ran
}
```

**Benefits:**
- Robust: Survives internal refactoring
- Clear: Tests the contract, not internals
- Maintainable: Fewer false failures
- Verifies actual behavior

---

## Testing Principles Applied

### 1. **Test Observable Outcomes**

Focus on what users can observe:

```bash
# GOOD: Observable behavior
[ "$status" -eq 0 ]              # Success/failure
[ -f "$GOPLS_CALLED" ]           # Tool was invoked
[ -f "$GO_BUILD_CALLED" ]        # Validation occurred

# BAD: Implementation details
[[ "$output" =~ "Calling gopls rename" ]]
[[ "$output" =~ "Running go build" ]]
```

### 2. **Track Side Effects, Not Logs**

Use marker files to verify behavior:

```bash
# Setup: Create tracking mechanism
GOPLS_CALLED="$TEST_DIR/gopls_called"
GO_BUILD_CALLED="$TEST_DIR/go_build_called"

# Mock that tracks calls
cat > "$TEST_DIR/bin/gopls" << EOF
#!/bin/bash
touch "$GOPLS_CALLED"
exit 0
EOF

# Test: Verify the side effect
[ -f "$GOPLS_CALLED" ]  # gopls was called
```

### 3. **Test Contracts, Not Messages**

Only check output when it's part of the user contract:

```bash
# GOOD: Part of contract (help text must list commands)
@test "documents all available commands in help" {
    run bash "$HELPER_SCRIPT" --help
    [[ "$output" =~ "rename" ]]
    [[ "$output" =~ "validate" ]]
}

# BAD: Internal logging (can change freely)
@test "logs progress messages" {
    [[ "$output" =~ "Step 1: Validating" ]]
    [[ "$output" =~ "Step 2: Running gopls" ]]
}
```

### 4. **Test State Changes**

Verify state transitions, not process steps:

```bash
# GOOD: State verification
@test "validates file existence before proceeding" {
    run bash "$HELPER_SCRIPT" rename nonexistent.go 1 1 Name
    
    [ "$status" -eq 1 ]           # Failed
    [ ! -f "$GOPLS_CALLED" ]      # gopls NOT called (important!)
}

# BAD: Process verification
[[ "$output" =~ "Checking if file exists" ]]
[[ "$output" =~ "File not found, aborting" ]]
```

### 5. **Use Descriptive Test Names**

Names describe behavior, not implementation:

```bash
# GOOD: Describes behavior
@test "rename: skips validation when --no-validate flag provided"
@test "validate: stops at build failure, doesn't run tests"
@test "contract: successful operations return exit code 0"

# BAD: Describes implementation
@test "calls validate_build function"
@test "prints success message to stdout"
```

---

## Test Organization

Tests are now organized by **behavioral categories**:

### 1. Help System Behavior
- Returns success for help requests
- Provides usage information
- Rejects invalid commands
- Documents all commands

### 2. Prerequisites and Safety
- Fails when gopls unavailable
- Warns about missing git
- Validates file existence

### 3. Command Behaviors (per command)
- Argument validation
- Success path (gopls succeeds + validation passes)
- Failure propagation (gopls fails)
- Special behaviors (flags, edge cases)

### 4. Edge Cases and Robustness
- Special characters (spaces in paths)
- Relative vs absolute paths
- Sequential operations
- Empty files

### 5. Validation Integration
- Default behavior (runs validation)
- Flag behavior (skips validation)
- Failure propagation

### 6. Contracts (User Guarantees)
- Success returns 0
- Failures return non-zero
- Invalid usage returns non-zero

---

## Verification Techniques

### Mock Tracking Pattern

```bash
# Setup tracking
GOPLS_CALLED="$TEST_DIR/gopls_called"

# Mock that tracks
cat > "$TEST_DIR/bin/gopls" << EOF
#!/bin/bash
echo "gopls was called" > "$GOPLS_CALLED"
echo "args: \$*" >> "$GOPLS_CALLED"
exit 0
EOF

# Verify behavior
[ -f "$GOPLS_CALLED" ]  # Called
[ ! -f "$GOPLS_CALLED" ]  # NOT called
```

### Exit Code Verification

```bash
# Success
[ "$status" -eq 0 ]

# Failure (any non-zero)
[ "$status" -ne 0 ]

# Specific exit code (only when part of contract)
[ "$status" -eq 1 ]
```

### State Verification

```bash
# Files created
[ -f "expected_file.txt" ]

# Files NOT created
[ ! -f "should_not_exist.txt" ]

# Git changes detected
git diff --quiet || true  # Changes exist
```

---

## What We DON'T Test

### ❌ Log Messages (Unless Part of Contract)

```bash
# DON'T TEST (implementation)
[[ "$output" =~ "Step 1: Preparing" ]]
[[ "$output" =~ "Step 2: Executing" ]]

# DO TEST (user-facing contract)
run bash "$HELPER_SCRIPT" --help
[[ "$output" =~ "Usage:" ]]  # Help text is contract
```

### ❌ Internal Function Calls

```bash
# DON'T TEST
# Can't check: "validate_build function was called"

# DO TEST
[ -f "$GO_BUILD_CALLED" ]  # Build tool was invoked
```

### ❌ Exact Output Formatting

```bash
# DON'T TEST
[[ "$output" == "✅ Success: Operation completed successfully" ]]

# DO TEST
[ "$status" -eq 0 ]  # Operation succeeded
```

### ❌ Order of Internal Operations (Unless It Matters)

```bash
# DON'T TEST
# Order of: check file → call gopls → validate

# DO TEST
[ ! -f "$GOPLS_CALLED" ]  # gopls NOT called when file missing
```

---

## Benefits of This Approach

### 1. **Refactoring Freedom**

Can change internal implementation without breaking tests:

```bash
# Can freely change:
- Log message wording
- Internal function names
- Progress indicators
- Output formatting

# Tests remain valid as long as behavior unchanged
```

### 2. **Clearer Intent**

Tests document the contract:

```bash
@test "rename: skips validation when --no-validate flag provided"
# Clear: Users can skip validation
# Contract: --no-validate flag exists and works

# vs old:
@test "no validation messages shown"
# Unclear: Why? What's being tested?
```

### 3. **Fewer False Failures**

```bash
# Old: Fails when changing log message
[[ "$output" =~ "Validating build..." ]]  # Brittle

# New: Only fails on behavior change
[ -f "$GO_BUILD_CALLED" ]  # Robust
```

### 4. **Better Test Names**

```bash
# Behavior-focused names tell a story:
@test "validate: stops at build failure, doesn't run tests"
@test "rename: propagates gopls error status"
@test "contract: successful operations return exit code 0"

# Easy to understand what's guaranteed
```

---

## Testing Checklist

When writing new tests, ask:

- [ ] **Does this test observable behavior?** (exit codes, files, side effects)
- [ ] **Is this part of the user contract?** (documented behavior)
- [ ] **Would this test break if I changed log messages?** (if yes, revise)
- [ ] **Does the test name describe behavior?** (not implementation steps)
- [ ] **Can I refactor the code without breaking this test?** (if no, revise)

If you answer "no" to any question, revise the test.

---

## Example Comparison

### ❌ Implementation-Focused Test

```bash
@test "rename processes correctly" {
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # Testing internal process
    [[ "$output" =~ "Checking prerequisites" ]]
    [[ "$output" =~ "Renaming symbol" ]]
    [[ "$output" =~ "Running validation" ]]
    [[ "$output" =~ "Build successful" ]]
    [[ "$output" =~ "Tests passed" ]]
    [ "$status" -eq 0 ]
}
```

**Problems:**
- 5 brittle assertions about logs
- Tests HOW rename works internally
- Breaks when changing messages
- Doesn't verify actual behavior

### ✅ Behavior-Focused Test

```bash
@test "rename: succeeds when gopls succeeds and validation passes" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # Testing observable behavior
    [ "$status" -eq 0 ]           # Operation succeeded
    [ -f "$GOPLS_CALLED" ]        # gopls was invoked
    [ -f "$GO_BUILD_CALLED" ]     # Build validated
    [ -f "$GO_TEST_CALLED" ]      # Tests validated
}
```

**Benefits:**
- 4 behavioral assertions
- Tests WHAT rename does
- Survives message changes
- Verifies actual outcomes

---

## Conclusion

**Before:** 40 tests, many brittle, testing implementation details  
**After:** 40 tests, robust, testing behavior and contracts

**Key Insight:** Test what users can observe, not how the code achieves it.

This makes tests:
- More robust (survive refactoring)
- More maintainable (fewer false failures)
- More clear (document contracts)
- More valuable (catch real bugs)

**All tests pass:** ✅ 40/40
