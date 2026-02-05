#!/usr/bin/env bats
# BATS tests for gopls-helper.sh
# Tests focus on BEHAVIOR and CORRECTNESS, not implementation details

# Setup - runs before each test
setup() {
    SCRIPT_DIR="$(cd "$(dirname "${BATS_TEST_FILENAME}")" && pwd)"
    HELPER_SCRIPT="$SCRIPT_DIR/gopls-helper.sh"
    
    # Create temporary test directory
    TEST_DIR=$(mktemp -d)
    cd "$TEST_DIR"
    
    # Create a mock go.mod for testing
    cat > go.mod << 'EOF'
module example.com/test
go 1.23
EOF
    
    # Initialize git repo
    git init -q
    git config user.email "test@example.com"
    git config user.name "Test User"
    
    # Create mock Go file
    mkdir -p pkg/calc
    cat > pkg/calc/math.go << 'EOF'
package calc

// Add returns the sum of two integers
func Add(a, b int) int {
    return a + b
}

// Multiply returns the product
func Multiply(a, b int) int {
    return a * b
}
EOF
    
    git add -A
    git commit -q -m "Initial commit"
    
    # Setup PATH for mocks
    export PATH="$TEST_DIR/bin:$PATH"
    mkdir -p bin
    
    # Track whether gopls was called
    GOPLS_CALLED="$TEST_DIR/gopls_called"
    GO_BUILD_CALLED="$TEST_DIR/go_build_called"
    GO_TEST_CALLED="$TEST_DIR/go_test_called"
}

# Teardown - runs after each test
teardown() {
    cd /
    rm -rf "$TEST_DIR"
}

# Helper: Create mock gopls that tracks calls
create_mock_gopls() {
    local exit_code=${1:-0}
    local should_modify_file=${2:-false}
    
    cat > "$TEST_DIR/bin/gopls" << EOF
#!/bin/bash
echo "gopls was called" > "$GOPLS_CALLED"
echo "args: \$*" >> "$GOPLS_CALLED"

# Simulate file modification on successful rename/move
if [[ "$should_modify_file" == "true" && \$exit_code -eq 0 ]]; then
    echo "// Modified by gopls" >> pkg/calc/math.go 2>/dev/null || true
fi

exit $exit_code
EOF
    chmod +x "$TEST_DIR/bin/gopls"
}

# Helper: Create mock go that tracks calls
create_mock_go() {
    local build_exit=${1:-0}
    local test_exit=${2:-0}
    
    cat > "$TEST_DIR/bin/go" << EOF
#!/bin/bash
if [[ "\$1" == "build" ]]; then
    touch "$GO_BUILD_CALLED"
    exit $build_exit
elif [[ "\$1" == "test" ]]; then
    touch "$GO_TEST_CALLED"
    exit $test_exit
else
    exit 1
fi
EOF
    chmod +x "$TEST_DIR/bin/go"
}

# =============================================================================
# BEHAVIOR: Help System
# =============================================================================

@test "returns success when help requested" {
    run bash "$HELPER_SCRIPT" --help
    [ "$status" -eq 0 ]
}

@test "provides usage information when no command given" {
    run bash "$HELPER_SCRIPT"
    [ "$status" -eq 1 ]
    [[ "$output" =~ "Usage:" ]]
}

@test "rejects unknown commands" {
    run bash "$HELPER_SCRIPT" invalid-command arg1 arg2
    [ "$status" -eq 1 ]
}

@test "documents all available commands in help" {
    run bash "$HELPER_SCRIPT" --help
    
    # Contract: Help must list all commands
    [[ "$output" =~ "rename" ]]
    [[ "$output" =~ "move-package" ]]
    [[ "$output" =~ "inline-call" ]]
    [[ "$output" =~ "extract-function" ]]
    [[ "$output" =~ "remove-param" ]]
    [[ "$output" =~ "list-actions" ]]
    [[ "$output" =~ "validate" ]]
}

# =============================================================================
# BEHAVIOR: Prerequisites and Safety Checks
# =============================================================================

@test "fails when gopls is not available" {
    export PATH="/usr/bin:/bin"  # gopls won't be found
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    [ "$status" -eq 1 ]
}

@test "warns about missing git repository" {
    create_mock_gopls 0
    create_mock_go 0 0
    rm -rf .git
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    [[ "$output" =~ "git" ]] || [[ "$output" =~ "repository" ]]
}

@test "validates file existence before proceeding" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" rename nonexistent.go 1 1 NewName
    [ "$status" -eq 1 ]
    
    # BEHAVIOR: gopls should NOT have been called
    [ ! -f "$GOPLS_CALLED" ]
}

# =============================================================================
# BEHAVIOR: Rename Command
# =============================================================================

@test "rename: requires exactly 4 arguments" {
    create_mock_gopls 0
    
    # Too few arguments
    run bash "$HELPER_SCRIPT" rename
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" rename arg1
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" rename arg1 arg2
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" rename arg1 arg2 arg3
    [ "$status" -eq 1 ]
}

@test "rename: succeeds when gopls succeeds and validation passes" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Operation should succeed
    [ "$status" -eq 0 ]
    
    # BEHAVIOR: gopls should have been called
    [ -f "$GOPLS_CALLED" ]
    
    # BEHAVIOR: Validation should have run
    [ -f "$GO_BUILD_CALLED" ]
    [ -f "$GO_TEST_CALLED" ]
}

@test "rename: fails when gopls fails" {
    create_mock_gopls 1
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Operation should fail
    [ "$status" -eq 1 ]
}

@test "rename: skips validation when --no-validate flag provided" {
    create_mock_gopls 0 true
    # Intentionally NOT creating mock go commands
    
    run bash "$HELPER_SCRIPT" --no-validate rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Should succeed without validation
    [ "$status" -eq 0 ]
    
    # BEHAVIOR: gopls should have been called
    [ -f "$GOPLS_CALLED" ]
    
    # BEHAVIOR: Validation should NOT have run
    [ ! -f "$GO_BUILD_CALLED" ]
    [ ! -f "$GO_TEST_CALLED" ]
}

@test "rename: fails when validation fails" {
    create_mock_gopls 0 true
    create_mock_go 1 0  # Build will fail
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Operation should fail due to build failure
    [ "$status" -eq 1 ]
    
    # BEHAVIOR: gopls was called (refactoring attempted)
    [ -f "$GOPLS_CALLED" ]
    
    # BEHAVIOR: Build validation was attempted
    [ -f "$GO_BUILD_CALLED" ]
}

@test "rename: propagates gopls error status" {
    create_mock_gopls 42  # Custom exit code
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Should fail (not preserve specific exit code, just fail)
    [ "$status" -ne 0 ]
}

# =============================================================================
# BEHAVIOR: Move Package Command
# =============================================================================

@test "move-package: requires exactly 2 arguments" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" move-package
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" move-package arg1
    [ "$status" -eq 1 ]
}

@test "move-package: succeeds when gopls succeeds" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" move-package pkg/calc/math.go newpkg
    
    # BEHAVIOR: Operation should succeed
    [ "$status" -eq 0 ]
    
    # BEHAVIOR: gopls should have been called
    [ -f "$GOPLS_CALLED" ]
}

@test "move-package: validates file existence" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" move-package nonexistent.go newpkg
    
    # BEHAVIOR: Should fail before calling gopls
    [ "$status" -eq 1 ]
    [ ! -f "$GOPLS_CALLED" ]
}

# =============================================================================
# BEHAVIOR: Inline Call Command
# =============================================================================

@test "inline-call: requires exactly 3 arguments" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" inline-call
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" inline-call arg1
    [ "$status" -eq 1 ]
}

@test "inline-call: succeeds when gopls succeeds" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" inline-call pkg/calc/math.go 10 5
    
    # BEHAVIOR: Operation should succeed
    [ "$status" -eq 0 ]
    [ -f "$GOPLS_CALLED" ]
}

@test "inline-call: fails when gopls fails" {
    create_mock_gopls 1
    
    run bash "$HELPER_SCRIPT" inline-call pkg/calc/math.go 10 5
    
    # BEHAVIOR: Should propagate failure
    [ "$status" -eq 1 ]
}

# =============================================================================
# BEHAVIOR: Extract Function Command
# =============================================================================

@test "extract-function: requires exactly 5 arguments" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" extract-function
    [ "$status" -eq 1 ]
    
    run bash "$HELPER_SCRIPT" extract-function arg1 arg2 arg3 arg4
    [ "$status" -eq 1 ]
}

@test "extract-function: succeeds when gopls succeeds" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" extract-function pkg/calc/math.go 5 0 10 5
    
    # BEHAVIOR: Operation should succeed
    [ "$status" -eq 0 ]
    [ -f "$GOPLS_CALLED" ]
}

# =============================================================================
# BEHAVIOR: Remove Parameter Command
# =============================================================================

@test "remove-param: requires exactly 3 arguments" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" remove-param
    [ "$status" -eq 1 ]
}

@test "remove-param: succeeds when gopls succeeds" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" remove-param pkg/calc/math.go 5 15
    
    # BEHAVIOR: Operation should succeed
    [ "$status" -eq 0 ]
    [ -f "$GOPLS_CALLED" ]
}

@test "remove-param: fails when gopls reports parameter is in use" {
    create_mock_gopls 1
    
    run bash "$HELPER_SCRIPT" remove-param pkg/calc/math.go 5 15
    
    # BEHAVIOR: Should fail
    [ "$status" -eq 1 ]
}

# =============================================================================
# BEHAVIOR: List Actions Command
# =============================================================================

@test "list-actions: requires exactly 3 arguments" {
    create_mock_gopls 0
    
    run bash "$HELPER_SCRIPT" list-actions
    [ "$status" -eq 1 ]
}

@test "list-actions: calls gopls and returns its output" {
    create_mock_gopls 0
    
    # Override gopls to output specific actions
    cat > "$TEST_DIR/bin/gopls" << 'EOF'
#!/bin/bash
echo "command 'Rename' [refactor.rename]"
echo "command 'Extract' [refactor.extract]"
exit 0
EOF
    chmod +x "$TEST_DIR/bin/gopls"
    
    run bash "$HELPER_SCRIPT" list-actions pkg/calc/math.go 5 6
    
    # BEHAVIOR: Should succeed and pass through gopls output
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Rename" ]]
    [[ "$output" =~ "Extract" ]]
}

# =============================================================================
# BEHAVIOR: Validate Command
# =============================================================================

@test "validate: succeeds when both build and tests pass" {
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" validate
    
    # BEHAVIOR: Should succeed
    [ "$status" -eq 0 ]
    
    # BEHAVIOR: Both checks should have run
    [ -f "$GO_BUILD_CALLED" ]
    [ -f "$GO_TEST_CALLED" ]
}

@test "validate: fails when build fails" {
    create_mock_go 1 0
    
    run bash "$HELPER_SCRIPT" validate
    
    # BEHAVIOR: Should fail
    [ "$status" -ne 0 ]
    
    # BEHAVIOR: Build was attempted
    [ -f "$GO_BUILD_CALLED" ]
}

@test "validate: fails when tests fail" {
    create_mock_go 0 1
    
    run bash "$HELPER_SCRIPT" validate
    
    # BEHAVIOR: Should fail
    [ "$status" -ne 0 ]
    
    # BEHAVIOR: Tests were attempted
    [ -f "$GO_TEST_CALLED" ]
}

@test "validate: stops at build failure, doesn't run tests" {
    create_mock_go 1 0
    
    run bash "$HELPER_SCRIPT" validate
    
    # BEHAVIOR: Should fail fast
    [ "$status" -ne 0 ]
    [ -f "$GO_BUILD_CALLED" ]
    [ ! -f "$GO_TEST_CALLED" ]  # Tests should NOT run if build fails
}

# =============================================================================
# BEHAVIOR: Edge Cases and Robustness
# =============================================================================

@test "handles file paths with spaces" {
    create_mock_gopls 0
    create_mock_go 0 0
    
    mkdir -p "pkg/my calc"
    cat > "pkg/my calc/math.go" << 'EOF'
package calc
func Add(a, b int) int { return a + b }
EOF
    
    run bash "$HELPER_SCRIPT" rename "pkg/my calc/math.go" 2 6 Sum
    
    # BEHAVIOR: Should handle spaces correctly
    [ "$status" -eq 0 ]
}

@test "handles relative paths" {
    create_mock_gopls 0
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename ./pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Should accept relative paths
    [ "$status" -eq 0 ]
}

@test "detects git changes after refactoring" {
    create_mock_gopls 0 true  # Will modify file
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Should detect and report changes
    [ "$status" -eq 0 ]
    [[ "$output" =~ "Changes" ]] || [[ "$output" =~ "changes" ]]
}

@test "handles empty files without crashing" {
    create_mock_gopls 0
    create_mock_go 0 0
    
    touch empty.go
    
    run bash "$HELPER_SCRIPT" rename empty.go 1 1 Name
    
    # BEHAVIOR: Should not crash (gopls handles this)
    # Exit code may be 0 or 1 depending on gopls behavior
    [ "$status" -eq 0 ] || [ "$status" -eq 1 ]
}

@test "can execute multiple operations sequentially" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    # First operation
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    [ "$status" -eq 0 ]
    
    # Second operation (in same environment)
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 10 6 Product
    [ "$status" -eq 0 ]
}

# =============================================================================
# BEHAVIOR: Validation Integration
# =============================================================================

@test "runs validation by default after refactoring" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    
    # BEHAVIOR: Validation should be part of default workflow
    [ "$status" -eq 0 ]
    [ -f "$GO_BUILD_CALLED" ]
    [ -f "$GO_TEST_CALLED" ]
}

@test "validation flag globally applies to all commands" {
    create_mock_gopls 0 true
    
    # Test with move-package
    run bash "$HELPER_SCRIPT" --no-validate move-package pkg/calc/math.go newpkg
    [ "$status" -eq 0 ]
    [ ! -f "$GO_BUILD_CALLED" ]
    
    # Reset tracking
    rm -f "$GOPLS_CALLED"
    
    # Test with inline-call
    run bash "$HELPER_SCRIPT" --no-validate inline-call pkg/calc/math.go 5 6
    [ "$status" -eq 0 ]
    [ ! -f "$GO_BUILD_CALLED" ]
}

# =============================================================================
# CONTRACT: User-Facing Behavior Guarantees
# =============================================================================

@test "contract: successful operations return exit code 0" {
    create_mock_gopls 0 true
    create_mock_go 0 0
    
    # Test multiple commands
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    [ "$status" -eq 0 ]
    
    run bash "$HELPER_SCRIPT" --no-validate inline-call pkg/calc/math.go 5 6
    [ "$status" -eq 0 ]
    
    run bash "$HELPER_SCRIPT" validate
    [ "$status" -eq 0 ]
}

@test "contract: failed operations return non-zero exit code" {
    create_mock_gopls 1
    
    # Test multiple failure scenarios
    run bash "$HELPER_SCRIPT" rename pkg/calc/math.go 5 6 Sum
    [ "$status" -ne 0 ]
    
    run bash "$HELPER_SCRIPT" inline-call pkg/calc/math.go 5 6
    [ "$status" -ne 0 ]
}

@test "contract: invalid usage returns non-zero exit code" {
    # Test multiple invalid usage patterns
    run bash "$HELPER_SCRIPT" rename  # Missing arguments
    [ "$status" -ne 0 ]
    
    run bash "$HELPER_SCRIPT" invalid-command
    [ "$status" -ne 0 ]
    
    run bash "$HELPER_SCRIPT" rename nonexistent.go 1 1 Name
    [ "$status" -ne 0 ]
}
