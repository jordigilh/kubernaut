# gopls Complete Reference

Comprehensive reference for all gopls refactoring and transformation capabilities.

**Source**: Based on [gopls v0.21.0 features documentation](https://go.dev/gopls/features/transformation)

---

## Overview

gopls (Go Language Server) provides type-safe, automated refactoring operations through the Language Server Protocol (LSP). All operations ensure:
- All references are updated across the entire codebase
- Type safety is maintained
- Build integrity is preserved
- Imports are automatically organized

---

## Primary Operations

### 1. Rename (`textDocument/rename`)

**Most common and powerful operation** - renames any Go symbol.

**Supports**:
- Functions and methods
- Types (structs, interfaces, type aliases)
- Variables and constants
- Package names (moves entire package!)
- Method receivers (renames across all methods)

**Command**:
```bash
gopls rename -w <file>:<line>:<column> <newname>
```

**Safety Features**:
- Detects shadowing conflicts
- Prevents interface method mismatches
- Validates package move constraints
- Reports errors if rename would break compilation

**Special Cases**:
- **Package rename**: Moves all files, updates imports, creates directories
- **Method receiver rename**: Attempts to rename all methods of the same type
- **Interface method rename**: Can rename across implementing types

---

## Code Actions (`textDocument/codeAction`)

Code actions are context-sensitive operations available at a specific code location.

### Extract Refactorings

#### `refactor.extract.function`
Extracts statements into a new function.

**Requirements**:
- Select one or more complete statements
- Must be fewer statements than entire function body
- Creates new function with inferred parameters/returns

**Example**:
```go
// Before: Select validation block
if x < 0 { return err }
if y < 0 { return err }

// After: Extracted
if err := validateInputs(x, y); err != nil { return err }

func validateInputs(x, y int) error { ... }
```

#### `refactor.extract.method`
Variant of extract function for methods - creates a new method on the same receiver type.

#### `refactor.extract.variable`
Extracts an expression into a local variable.

**Example**:
```go
// Before: Select expression
fmt.Println(strings.ToUpper(name))

// After: Extracted
upper := strings.ToUpper(name)
fmt.Println(upper)
```

#### `refactor.extract.constant`
Extracts a constant expression into a named constant.

#### `refactor.extract.variable-all` / `refactor.extract.constant-all`
Extracts ALL occurrences of the selected expression within the function.

**Example**:
```go
// Before: Select API_URL
fmt.Println("https://api.example.com/users")
http.Get("https://api.example.com/users")

// After: All occurrences extracted
const apiURL = "https://api.example.com/users"
fmt.Println(apiURL)
http.Get(apiURL)
```

#### `refactor.extract.toNewFile`
Moves selected top-level declarations to a new file.

**Requirements**:
- Select one or more complete top-level declarations
- New file name based on first symbol
- Imports created automatically

**Example**:
```go
// Before: Select functions in handlers.go
func HandleAlert() { ... }
func HandleNotification() { ... }

// After: Moved to handlealert.go
// handlers.go is now smaller
// handlealert.go contains the moved functions with proper imports
```

---

### Inline Refactorings

#### `refactor.inline.call`
Replaces a function call with the function's body.

**Requirements**:
- Must be a static call (not through interface or function value)
- Callee must be accessible (not unexported from another package)
- No generic functions (limitation as of v0.21)

**Use Cases**:
- Remove deprecated functions
- Eliminate unnecessary abstractions
- Copy function for modification

**Safety**:
- Preserves side effect order
- Handles control flow (`defer`, `return`, etc.)
- Adds temporary variables when needed

**Example**:
```go
// Before: Call to sum
result := sum(a, b)
func sum(x, y int) int { return x + y }

// After: Inlined
result := a + b
```

#### `refactor.inline.variable`
Replaces a variable usage with its initializer expression.

**Example**:
```go
// Before: Variable s
s := fmt.Sprintf("+%d", x)
println(s)

// After: Inlined
s := fmt.Sprintf("+%d", x)
println(fmt.Sprintf("+%d", x))  // s still defined but unused
```

---

### Rewrite Operations (`refactor.rewrite.*`)

#### `refactor.rewrite.removeUnusedParam`
Removes an unused parameter and updates all call sites.

**Analyzer**: `unusedparams`  
**Constraints**: Only for non-address-taken, non-exported functions

**Example**:
```go
// Before: x unused
func process(x, y int) { fmt.Println(y) }
process(getExpensiveValue(), 42)

// After: x removed, but call preserved!
func process(y int) { fmt.Println(y) }
process(getExpensiveValue(), 42)  // First arg kept for side effects!
```

#### `refactor.rewrite.moveParamLeft` / `refactor.rewrite.moveParamRight`
Reorders function parameters and updates all call sites.

**Example**:
```go
// Before
func foo(x, y, z int) { ... }
foo(1, 2, 3)

// After: Move y left
func foo(y, x, z int) { ... }
foo(2, 1, 3)
```

#### `refactor.rewrite.changeQuote`
Converts string literal between raw (backtick) and interpreted (quotes).

**Example**:
```go
// Before: "hello\nworld"
// After: `hello\nworld`
// Toggle back: "hello\nworld"
```

#### `refactor.rewrite.invertIf`
Inverts an if/else statement by negating the condition and swapping blocks.

**Example**:
```go
// Before
if x > 0 {
    doPositive()
} else {
    doNegative()
}

// After
if x <= 0 {
    doNegative()
} else {
    doPositive()
}
```

#### `refactor.rewrite.splitLines` / `refactor.rewrite.joinLines`
Splits/joins elements in lists onto separate lines.

**Applies to**:
- Composite literal elements
- Function call arguments
- Function parameters
- Function results

**Example**:
```go
// Before (joined)
foo(a, b, c)

// After (split)
foo(
    a,
    b,
    c,
)
```

#### `refactor.rewrite.fillStruct`
Populates a struct literal with all accessible fields.

**Heuristics**:
- Matches field names to variables/constants in scope
- Uses zero values for unmatched fields
- Considers only symbols defined above current point

**Example**:
```go
// Before: type Config struct { Host string; Port int; Debug bool }
cfg := Config{}

// After: Fill Config
cfg := Config{
    Host:  host,     // Matched from scope
    Port:  8080,     // Matched from scope
    Debug: false,    // Zero value
}
```

#### `refactor.rewrite.fillSwitch`
Adds cases for all values of an enum type or all types implementing an interface.

**For type switches**: Adds case for each concrete type
**For value switches**: Adds case for each named constant

**Example**:
```go
// Before: type Status int; const (Active Status = 1; Inactive Status = 2)
switch status {
}

// After: Fill switch
switch status {
case Active:
case Inactive:
default:
    panic(fmt.Sprintf("unexpected Status: %v", status))
}
```

#### `refactor.rewrite.addTags` / `refactor.rewrite.removeTags`
Adds or removes struct tags (e.g., `json:"field_name"`).

**Example**:
```go
// Before
type User struct {
    FirstName string
    LastName  string
}

// After: Add tags
type User struct {
    FirstName string `json:"first_name"`
    LastName  string `json:"last_name"`
}
```

---

## Formatting Operations

### `textDocument/formatting`
Formats Go code using `go fmt` (or `gofumpt` if configured).

**Command**:
```bash
gopls format file.go
```

**Settings**:
- `gofumpt`: Use alternative formatter (`github.com/mvdan/gofumpt`)

---

## Import Organization

### `source.organizeImports`
Automatically organizes imports:
- Removes unused imports
- Adds imports for undefined symbols
- Sorts into standard order (stdlib, third-party, local)

**Command**:
```bash
gopls fix -a file.go:#offset source.organizeImports
```

**Settings**:
- `local`: Comma-separated prefixes for "local" import paths

---

## Testing Support

### `source.addTest`
Generates a table-driven test for a function or method.

**Features**:
- Creates `_test.go` file if needed
- Uses `*_test` package when possible
- Generates test struct with parameters and results
- Handles context parameters and error returns
- Finds constructor for method receivers

**Example**:
```go
// Before: func Add(a, b int) int { return a + b }

// After: Generate test
func TestAdd(t *testing.T) {
    tests := []struct {
        name string
        a    int
        b    int
        want int
    }{
        // TODO: Add test cases.
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := Add(tt.a, tt.b); got != tt.want {
                t.Errorf("Add() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

---

## Diagnostic-Based Fixes

Many refactorings are triggered by diagnostics (linter/compiler errors):

### `quickfix`
Automatically suggested fixes for common issues:
- Simplify range loops: `for _ = range m` → `for range m`
- Fix imports
- Add missing error checks
- Convert types

### `source.fixAll`
Applies all unambiguously safe fixes in the file.

---

## Command-Line Interface

### Basic Usage

```bash
# Rename
gopls rename -w file.go:line:col newname

# Code action
gopls codeaction file.go:start-end

# Execute code action
gopls codeaction -exec -kind <kind> file.go:range

# Format
gopls format file.go

# Organize imports
gopls fix -a file.go:#offset source.organizeImports
```

### Position Formats

gopls accepts positions in multiple formats:

1. **Line:Column** (most common): `file.go:42:10`
   - Line: 1-indexed (first line = 1)
   - Column: 0-indexed (first char = 0)

2. **Byte Offset**: `file.go:#1234`
   - 0-indexed byte offset from file start

3. **Range**: `file.go:10:0-20:5` or `file.go:#100-#200`
   - For operations on code ranges

---

## Limitations and Known Issues

### Not Yet Supported

1. **Change Function Signature**
   - **Status**: Planned, but not yet implemented
   - **Issue**: [golang/go#38028](https://github.com/golang/go/issues/38028)
   - **Workaround**: Use parameter move/remove for partial signature changes

2. **Inline Method** (full support)
   - **Status**: Partial support
   - **Issue**: [golang/go#59243](https://github.com/golang/go/issues/59243)

3. **Generic Function Inlining**
   - **Status**: Not supported
   - **Issue**: [golang/go#63352](https://github.com/golang/go/issues/63352)
   - **Workaround**: Manual refactoring for generic functions

### Known Issues

1. **Comment Preservation**
   - **Issue**: Some operations may lose comments
   - **Root Cause**: Go's AST representation (issue [golang/go#20744](https://github.com/golang/go/issues/20744))
   - **Status**: High priority for Go team
   - **Workaround**: Manually restore comments from git diff

2. **Generated Files**
   - **Behavior**: Code actions are not offered for generated files
   - **Detection**: Files with conventional `DO NOT EDIT` comment

3. **Rename Safety**
   - **Behavior**: Very conservative to avoid breaking builds
   - **Trade-off**: May refuse valid renames if safety uncertain
   - **Note**: Better to be safe than introduce subtle bugs

---

## Client Support

### Command Line (Universal)
```bash
gopls <command> [options] file.go:position
```

### VS Code
- **Rename**: `F2` or right-click → "Rename Symbol"
- **Refactor**: `Ctrl+Shift+R` (macOS: `Cmd+Shift+R`)
- **Code Actions**: `Ctrl+.` (macOS: `Cmd+.`) or 💡 lightbulb icon

### Emacs (eglot)
- **Rename**: `M-x eglot-rename`
- **Code Actions**: `M-x eglot-code-actions`
- **Extract**: `M-x eglot-code-action-extract`
- **Inline**: `M-x eglot-code-action-inline`

### Vim (coc.nvim)
- **Rename**: `:call CocAction('rename')`
- **Code Actions**: `:call CocAction('codeAction')`

---

## Settings

gopls settings can be configured in `gopls.yaml` or editor settings:

```yaml
# Enable alternative formatter
gofumpt: true

# Local import prefixes
local: "github.com/jordigilh/kubernaut"

# Allow package moves to include subpackages
renameMovesSubpackages: false
```

---

## Best Practices

### 1. Always Validate After Refactoring
```bash
go build ./...
go test ./... -run=^$ -timeout=30s  # Quick compile check
go test ./... -v  # Full test suite
```

### 2. Work in Git Repository
```bash
git status  # Ensure tracked
git diff    # Review changes
git checkout -- .  # Revert if needed
```

### 3. Commit Before Large Refactorings
```bash
git add -A
git commit -m "Before refactoring: <description>"
# Now safe to refactor
```

### 4. Start Small, Then Scale
- Test rename on one symbol first
- Verify it works as expected
- Then apply to larger scope

### 5. Use Type-Safe Operations
- Prefer gopls rename over `sed`/`awk`
- Prefer gopls move over manual file moves
- Trust gopls to update all references

---

## Performance Considerations

gopls is fast, but large refactorings may take time:

- **Rename**: Usually < 1 second for most codebases
- **Package move**: 1-5 seconds depending on references
- **Extract/Inline**: < 1 second for single operation

For very large codebases (> 1M LOC), operations may take longer.

---

## Additional Resources

- **Official Documentation**: https://go.dev/gopls/features/transformation
- **LSP Specification**: https://microsoft.github.io/language-server-protocol/
- **gopls Repository**: https://github.com/golang/tools/tree/master/gopls
- **Issue Tracker**: https://github.com/golang/go/issues?q=is%3Aissue+label%3Agopls

---

## Version Information

This reference is based on **gopls v0.21.0** (February 2026).

For the latest features, check: `gopls version` and visit the official documentation.
