# gopls Refactoring Examples

Real-world examples of using gopls for common refactoring operations in kubernaut.

---

## Example 1: Rename Function Across Codebase

### Scenario
Function `ProcessAlert` in `pkg/processor/handler.go` should be renamed to `HandleAlert` for consistency.

### Before
```go
// pkg/processor/handler.go (line 45)
func ProcessAlert(alert *api.Alert) error {
    return validateAndProcess(alert)
}

// cmd/signalprocessing/main.go
func main() {
    err := processor.ProcessAlert(alert)
    // ...
}

// test/unit/processor_test.go
result := processor.ProcessAlert(testAlert)
```

### Steps

1. **Find the function definition**:
   ```bash
   rg "^func ProcessAlert" pkg/processor/ --type go
   # Output: pkg/processor/handler.go:45:func ProcessAlert(alert *api.Alert) error {
   ```

2. **Calculate position**: Line 45, column 5 (start of "ProcessAlert")

3. **Execute rename**:
   ```bash
   gopls rename -w pkg/processor/handler.go:45:5 HandleAlert
   ```

4. **Validate**:
   ```bash
   go build ./...
   go test ./pkg/processor/... -v
   git diff  # Review changes
   ```

### After
```go
// pkg/processor/handler.go (line 45)
func HandleAlert(alert *api.Alert) error {
    return validateAndProcess(alert)
}

// cmd/signalprocessing/main.go (automatically updated!)
func main() {
    err := processor.HandleAlert(alert)
    // ...
}

// test/unit/processor_test.go (automatically updated!)
result := processor.HandleAlert(testAlert)
```

### Result
✅ Function renamed in definition  
✅ All call sites updated (main.go, tests)  
✅ Build succeeds  
✅ Tests pass

---

## Example 2: Move Package to Better Location

### Scenario
Package `pkg/oldutil` should move to `pkg/common/utils` for better organization.

### Before
```
pkg/
  oldutil/
    helper.go
    validator.go
```

```go
// pkg/oldutil/helper.go
package oldutil

func FormatString(s string) string { ... }

// pkg/processor/handler.go
import "github.com/jordigilh/kubernaut/pkg/oldutil"

func process() {
    formatted := oldutil.FormatString(data)
}
```

### Steps

1. **Locate package declaration**:
   ```bash
   cat pkg/oldutil/helper.go | head -1
   # Output: package oldutil
   ```

2. **Position**: Line 1, column 8 (start of "oldutil" in `package oldutil`)

3. **Execute package rename**:
   ```bash
   gopls rename -w pkg/oldutil/helper.go:1:8 common/utils
   ```

4. **Verify the move**:
   ```bash
   # Check new location
   ls -la pkg/common/utils/
   # Output: helper.go, validator.go
   
   # Check imports updated
   rg "pkg/common/utils" --type go
   # Output: Multiple files now import new path
   
   # Validate build
   go build ./...
   ```

### After
```
pkg/
  common/
    utils/
      helper.go
      validator.go
```

```go
// pkg/common/utils/helper.go (moved!)
package utils

func FormatString(s string) string { ... }

// pkg/processor/handler.go (import automatically updated!)
import "github.com/jordigilh/kubernaut/pkg/common/utils"

func process() {
    formatted := utils.FormatString(data)
}
```

### Result
✅ All files moved to new directory  
✅ Package declaration updated  
✅ All imports updated across codebase  
✅ Build succeeds

---

## Example 3: Extract Function (Reduce Complexity)

### Scenario
Complex alert validation logic should be extracted into its own function.

### Before
```go
// pkg/processor/handler.go
func HandleAlert(alert *api.Alert) error {
    // Validation logic (lines 50-65)
    if alert.Name == "" {
        return fmt.Errorf("alert name is required")
    }
    if alert.Severity < 1 || alert.Severity > 5 {
        return fmt.Errorf("invalid severity: %d", alert.Severity)
    }
    if alert.Timestamp.IsZero() {
        return fmt.Errorf("timestamp is required")
    }
    
    // Process the alert (lines 67-70)
    return processValidatedAlert(alert)
}
```

### Steps

1. **Identify code to extract**: Lines 50-65 (validation block)

2. **Calculate byte offsets or line positions**:
   ```bash
   # For line range approach
   # Start: line 50, col 0
   # End: line 65, col 5
   ```

3. **List available code actions**:
   ```bash
   gopls codeaction pkg/processor/handler.go:50:0-65:5
   # Output: Lists available refactorings including "Extract function"
   ```

4. **Execute extraction**:
   ```bash
   gopls codeaction -exec -kind refactor.extract.function \
     pkg/processor/handler.go:50:0-65:5
   ```

5. **Rename extracted function** (gopls uses default name):
   ```bash
   # Assuming gopls created "newFunction" at line 50
   gopls rename -w pkg/processor/handler.go:50:5 validateAlert
   ```

6. **Validate**:
   ```bash
   go build ./pkg/processor/
   go test ./pkg/processor/... -v
   ```

### After
```go
// pkg/processor/handler.go
func HandleAlert(alert *api.Alert) error {
    if err := validateAlert(alert); err != nil {
        return err
    }
    return processValidatedAlert(alert)
}

func validateAlert(alert *api.Alert) error {
    if alert.Name == "" {
        return fmt.Errorf("alert name is required")
    }
    if alert.Severity < 1 || alert.Severity > 5 {
        return fmt.Errorf("invalid severity: %d", alert.Severity)
    }
    if alert.Timestamp.IsZero() {
        return fmt.Errorf("timestamp is required")
    }
    return nil
}
```

### Result
✅ Validation logic extracted to separate function  
✅ Original function simplified  
✅ Code more testable (can test validation independently)  
✅ Build succeeds

---

## Example 4: Inline Unnecessary Abstraction

### Scenario
One-line helper function adds no value and should be inlined.

### Before
```go
// pkg/processor/helpers.go
func getAlertName(alert *api.Alert) string {
    return alert.Name
}

// pkg/processor/handler.go (multiple call sites)
func process1(alert *api.Alert) {
    name := getAlertName(alert)
    log.Printf("Processing: %s", name)
}

func process2(alert *api.Alert) {
    if getAlertName(alert) == "critical" {
        // ...
    }
}
```

### Steps

1. **Locate a call site**:
   ```bash
   rg "getAlertName\(" pkg/processor/handler.go
   # Output: Multiple occurrences
   ```

2. **Position on first call**: Line 15, column 10 (on function name in call)

3. **Inline the call**:
   ```bash
   gopls codeaction -exec -kind refactor.inline.call \
     pkg/processor/handler.go:15:10
   ```

4. **Repeat for other call sites** or inline all at once

5. **Remove now-unused function**:
   ```bash
   # gopls or manual deletion of getAlertName
   ```

6. **Validate**:
   ```bash
   go build ./pkg/processor/
   go test ./pkg/processor/... -v
   ```

### After
```go
// pkg/processor/handler.go (function inlined!)
func process1(alert *api.Alert) {
    name := alert.Name  // Inlined!
    log.Printf("Processing: %s", name)
}

func process2(alert *api.Alert) {
    if alert.Name == "critical" {  // Inlined!
        // ...
    }
}
```

### Result
✅ Unnecessary abstraction removed  
✅ Code more direct and readable  
✅ One less function to maintain  
✅ Build succeeds

---

## Example 5: Remove Unused Parameter

### Scenario
Function `ProcessWithContext` has an unused `ctx context.Context` parameter.

### Before
```go
// pkg/workflow/engine.go
func ProcessWithContext(ctx context.Context, wf *Workflow) error {
    // ctx is never used!
    return wf.Execute()
}

// Multiple call sites
func caller1() {
    err := ProcessWithContext(context.Background(), workflow)
}

func caller2() {
    err := ProcessWithContext(ctx, workflow)
}
```

### Steps

1. **Verify parameter is unused**:
   ```bash
   rg "\bctx\b" pkg/workflow/engine.go
   # Should only show the parameter declaration, no usage
   ```

2. **Position on the unused parameter**: Line 10, column 23 (on "ctx" in parameters)

3. **Execute removal**:
   ```bash
   gopls codeaction -exec -kind refactor.rewrite.removeUnusedParam \
     pkg/workflow/engine.go:10:23
   ```

4. **Validate**:
   ```bash
   go build ./pkg/workflow/
   go test ./pkg/workflow/... -v
   ```

### After
```go
// pkg/workflow/engine.go (parameter removed!)
func ProcessWithContext(wf *Workflow) error {
    return wf.Execute()
}

// Multiple call sites (automatically updated!)
func caller1() {
    err := ProcessWithContext(workflow)  // ctx argument removed!
}

func caller2() {
    err := ProcessWithContext(workflow)  // ctx argument removed!
}
```

### Result
✅ Unused parameter removed from signature  
✅ All call sites updated automatically  
✅ Cleaner API  
✅ Build succeeds

---

## Example 6: Rename Type Across Interfaces

### Scenario
Type `AlertProcessor` should be renamed to `AlertHandler` for consistency.

### Before
```go
// pkg/processor/types.go
type AlertProcessor interface {
    Process(alert *api.Alert) error
}

type alertProcessorImpl struct {}

func (a *alertProcessorImpl) Process(alert *api.Alert) error { ... }

// pkg/processor/factory.go
func NewAlertProcessor() AlertProcessor {
    return &alertProcessorImpl{}
}

// cmd/signalprocessing/main.go
var processor processor.AlertProcessor
```

### Steps

1. **Find type definition**:
   ```bash
   rg "^type AlertProcessor" pkg/processor/ --type go
   # Output: pkg/processor/types.go:15:type AlertProcessor interface {
   ```

2. **Position**: Line 15, column 5 (start of "AlertProcessor")

3. **Execute rename**:
   ```bash
   gopls rename -w pkg/processor/types.go:15:5 AlertHandler
   ```

4. **Validate**:
   ```bash
   go build ./...
   rg "AlertHandler" --type go  # Verify all occurrences updated
   go test ./... -v
   ```

### After
```go
// pkg/processor/types.go
type AlertHandler interface {
    Process(alert *api.Alert) error
}

type alertHandlerImpl struct {}

func (a *alertHandlerImpl) Process(alert *api.Alert) error { ... }

// pkg/processor/factory.go
func NewAlertHandler() AlertHandler {
    return &alertHandlerImpl{}
}

// cmd/signalprocessing/main.go
var processor processor.AlertHandler
```

### Result
✅ Interface renamed  
✅ Implementation struct renamed  
✅ Factory function updated  
✅ All variable declarations updated  
✅ Build succeeds

---

## Example 7: TDD REFACTOR Phase Usage

### Scenario
After GREEN phase, refactor with gopls for better code quality.

### TDD Flow

**RED Phase**: Write failing test
```go
// test/unit/calculator_test.go
var _ = Describe("Calculator", func() {
    It("should add two numbers", func() {
        result := calc.Add(2, 3)
        Expect(result).To(Equal(5))
    })
})
```

**GREEN Phase**: Minimal implementation (poor naming)
```go
// pkg/calc/calculator.go
func Add(x, y int) int { return x + y }  // Works but could be better
```

**REFACTOR Phase**: Improve with gopls
```bash
# 1. Rename for clarity (already good in this case)

# 2. Extract common validation (if needed)
# If we had: func Add(x, y int) (int, error) { if x < 0 { return 0, err } ... }
# Extract validation:
gopls codeaction -exec -kind refactor.extract.function \
  pkg/calc/calculator.go:10:0-15:5

# 3. Validate tests still pass
go test ./pkg/calc/... -v
```

**Result**: Better code structure, tests still pass, TDD cycle complete!

---

## Troubleshooting Real Issues

### Issue: Wrong position causes "no identifier found"

```bash
# Wrong: Column too early
gopls rename -w pkg/calc/math.go:10:0 NewName
# Error: no identifier found

# Fix: Count column to start of identifier
# "func Add(" - 'A' starts at column 5 (0-indexed)
gopls rename -w pkg/calc/math.go:10:5 NewName
# Success!
```

### Issue: Package move creates empty directory

```bash
# If move partially fails
ls pkg/oldpkg/  # Still has files
ls pkg/newpkg/  # Empty!

# Fix: Revert and retry
git checkout -- .
gopls rename -w pkg/oldpkg/file.go:1:8 newpkg  # Try again
```

---

## Quick Command Reference

```bash
# Rename symbol
gopls rename -w file.go:line:col NewName

# Move package
gopls rename -w pkg/old/file.go:1:8 new

# Extract function
gopls codeaction -exec -kind refactor.extract.function file.go:start-end

# Inline call
gopls codeaction -exec -kind refactor.inline.call file.go:line:col

# Remove unused parameter
gopls codeaction -exec -kind refactor.rewrite.removeUnusedParam file.go:line:col

# List available actions
gopls codeaction file.go:range

# Validate after refactoring
go build ./...
go test ./... -run=^$ -timeout=30s
```
