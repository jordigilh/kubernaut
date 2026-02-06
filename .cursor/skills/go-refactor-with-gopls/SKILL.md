---
name: go-refactor-with-gopls
description: Performs Go code refactoring operations using gopls for type-safe transformations. Use when renaming functions/types/packages/variables, extracting code, inlining calls, moving Go packages, or any Go refactoring operation. Ensures build integrity and updates all references across the codebase.
---

# Go Refactoring with gopls

Provides type-safe, automated Go refactoring using gopls (Go Language Server) to ensure all references are updated and builds remain valid.

## Core Principle

**MANDATORY: ALWAYS use gopls for Go refactoring operations**

❌ **NEVER**:
- Use manual find/replace for renaming
- Use `sed`/`awk` for code changes
- Manually update imports after moving packages
- Guess at all references to update

✅ **ALWAYS**:
- Use gopls for rename, extract, inline, move operations
- Validate with `go build ./...` after refactoring
- Work in a git repository (can revert if needed)
- Run tests after refactoring to ensure behavior unchanged

---

## When to Use This Skill

Trigger this skill when the user wants to:
- Rename functions, methods, types, variables, constants, packages
- Move packages to different locations
- Extract functions, methods, variables, or constants
- Inline function calls or variables
- Move function parameters
- Remove unused parameters
- Any structural Go code changes

**Examples of user requests that trigger this skill:**
- "Rename the Add function to Sum"
- "Move the oldpkg package to math/calculations"
- "Extract this code into a new function"
- "Inline this function call"
- "Remove the unused x parameter"

---

## Prerequisites Check

**Important**: This skill uses **gopls CLI mode** (not server mode). Each command starts a fresh gopls instance and exits. You do NOT need to start a gopls server - VS Code/Cursor's gopls server is separate and unrelated.

Before executing ANY gopls refactoring:

```bash
# 1. Verify gopls is installed
gopls version || go install golang.org/x/tools/gopls@latest

# 2. Verify working in git repo
git status || echo "❌ NOT in git repo - proceed with caution"

# 3. Verify clean working tree (recommended)
git diff --quiet || echo "⚠️ Uncommitted changes exist"
```

---

## Common Refactoring Operations

### 1. Rename Symbol (Function, Method, Type, Variable)

**Use Case**: Rename any Go symbol across the entire codebase

**Steps**:

1. **Locate the symbol definition** (not a usage):
   ```bash
   # Find where the symbol is defined
   rg "^func Add\(" --type go
   rg "^type OldType struct" --type go
   ```

2. **Calculate position** (line:column of the symbol name):
   - Line: The line number where the symbol is defined
   - Column: The starting column of the symbol name (0-indexed)
   - Example: `func Add(` at line 42, column 5 → `file.go:42:5`

3. **Execute rename**:
   ```bash
   gopls rename -w path/to/file.go:line:column NewName
   ```

4. **Validate**:
   ```bash
   go build ./...
   go test ./... -run=^$ -timeout=30s  # Quick compile check
   ```

**Example**:
```bash
# Rename function "Add" to "Sum" in pkg/calc/math.go at line 10, col 6
gopls rename -w pkg/calc/math.go:10:6 Sum

# Validate
go build ./...
```

---

### 2. Move/Rename Package

**Use Case**: Move a package to a different location or rename it

**IMPORTANT**: This moves ALL files in the package and updates ALL imports

**Steps**:

1. **Locate the package declaration**:
   ```bash
   # Find the package file
   ls -la path/to/oldpkg/
   ```

2. **Calculate position** of the package name in `package oldpkg` statement:
   - Usually at line 1, column 8 (after "package ")
   - Example: `package oldpkg` → `file.go:1:8`

3. **Execute rename** with new package path:
   ```bash
   gopls rename -w path/to/oldpkg/file.go:1:8 newpkg
   # OR for nested packages
   gopls rename -w path/to/oldpkg/file.go:1:8 math/calculations
   ```

4. **Verify the move**:
   ```bash
   # Check files moved
   ls -la path/to/newpkg/
   
   # Check imports updated
   rg "import.*newpkg" --type go
   
   # Validate build
   go build ./...
   ```

**Constraints**:
- ❌ Cannot move `package main`
- ❌ Cannot move `x_test` packages
- ❌ Cannot cross module boundaries
- ❌ Cannot merge into existing package

**Example**:
```bash
# Move pkg/oldutil to pkg/utils
gopls rename -w pkg/oldutil/helper.go:1:8 utils

# Validate
go build ./...
rg "pkg/utils" --type go  # Check imports updated
```

---

### 3. Extract Function/Method/Variable

**Use Case**: Extract selected code into a new function, method, or variable

**Steps**:

1. **Identify the code to extract** (start and end positions):
   - Note: gopls uses byte offsets or line:column ranges
   - Format: `file.go:#offset` or `file.go:startline:startcol-endline:endcol`

2. **List available code actions**:
   ```bash
   gopls codeaction path/to/file.go:#startoffset-#endoffset
   ```

3. **Execute extract refactoring**:
   ```bash
   # Extract function
   gopls codeaction -exec -kind refactor.extract.function file.go:#start-#end
   
   # Extract variable
   gopls codeaction -exec -kind refactor.extract.variable file.go:#start-#end
   
   # Extract to new file
   gopls codeaction -exec -kind refactor.extract.toNewFile file.go:#start-#end
   ```

4. **Validate**:
   ```bash
   go build ./...
   go test ./pkg/... -v  # Run relevant tests
   ```

**Note**: Extract operations may require manual adjustment of the generated name (default: `newFunction`, `newVar`)

---

### 4. Inline Function Call or Variable

**Use Case**: Replace a function call with its body, or replace a variable with its value

**Steps**:

1. **Position cursor on the call/variable to inline**:
   ```bash
   # Find the usage location
   rg "functionName\(" path/to/file.go
   ```

2. **Execute inline**:
   ```bash
   # Inline function call
   gopls codeaction -exec -kind refactor.inline.call file.go:line:col
   
   # Inline variable
   gopls codeaction -exec -kind refactor.inline.variable file.go:line:col
   ```

3. **Validate**:
   ```bash
   go build ./...
   go test ./pkg/... -v
   ```

---

### 5. Remove Unused Parameter

**Use Case**: Remove a parameter from a function and update all call sites

**Steps**:

1. **Ensure parameter is truly unused** (gopls will validate):
   ```bash
   # Check parameter usage
   rg "paramName" path/to/file.go
   ```

2. **Get code action on the parameter**:
   ```bash
   gopls codeaction path/to/file.go:line:col
   ```

3. **Execute removal**:
   ```bash
   gopls codeaction -exec -kind refactor.rewrite.removeUnusedParam file.go:line:col
   ```

4. **Validate**:
   - gopls updates all callers automatically
   - Check with: `go build ./...`

---

## Safety Workflow (MANDATORY)

Follow this workflow for EVERY gopls refactoring:

### Pre-Refactoring Checklist

```
- [ ] Working in git repository
- [ ] All changes committed (clean working tree recommended)
- [ ] gopls installed and up to date
- [ ] Identified exact position (file:line:column)
- [ ] Understand scope of change (symbol usages across codebase)
```

### Post-Refactoring Validation

```
- [ ] Build succeeds: go build ./...
- [ ] Quick test compile: go test ./... -run=^$ -timeout=30s
- [ ] Run affected tests: go test ./pkg/[affected]/... -v
- [ ] Check for unintended changes: git diff
- [ ] Verify imports organized: go build ./... (will catch import errors)
- [ ] TDD validation: All tests still pass
```

---

## Integration with Kubernaut TDD Workflow

### During REFACTOR Phase

gopls refactoring is **most commonly used in the REFACTOR phase** of TDD:

**RED → GREEN → REFACTOR (with gopls)**

1. **RED**: Tests written (no refactoring yet)
2. **GREEN**: Minimal implementation (may have poor naming/structure)
3. **REFACTOR**: Use gopls to improve code quality:
   - Rename poorly named symbols
   - Extract duplicate code into functions
   - Inline unnecessary abstractions
   - Move code to better packages

**Example**:
```bash
# GREEN phase: Function works but poorly named
func calc(x, y int) int { return x + y }

# REFACTOR phase: Rename for clarity
gopls rename -w pkg/math/operations.go:10:6 Add

# Result: Better named function
func Add(x, y int) int { return x + y }

# Validate: Tests still pass
go test ./pkg/math/... -v
```

### During Structural Changes

When moving code between packages (architectural refactoring):

1. **Ensure tests exist** for the code being moved
2. **Use gopls to move the package**
3. **Validate tests still pass** in new location
4. **Update integration in cmd/** if needed

---

## Troubleshooting

### Issue: "no identifier found"

**Cause**: Incorrect position (line:column)

**Fix**: 
- Verify line number with `cat -n file.go | grep "func Name"`
- Count column carefully (0-indexed, starts at beginning of identifier)
- Try adjacent columns if unsure

### Issue: "cannot rename across module boundary"

**Cause**: Trying to move package outside current module

**Fix**:
- Can only move packages within the same go.mod module
- Use different approach if crossing module boundaries

### Issue: "package already exists"

**Cause**: Target package directory already has files

**Fix**:
- gopls won't merge packages automatically
- Choose different package name or manually merge first

### Issue: Build fails after rename

**Cause**: Possible gopls edge case or complex refactoring

**Fix**:
```bash
# Revert and investigate
git diff  # Review what changed
git checkout -- .  # Revert if needed

# Try more targeted refactoring
# Or report as gopls issue
```

---

## Limitations and Workarounds

### Known gopls Limitations

1. **Change signature** - Not yet supported (as of 2026)
   - Workaround: Remove unused params individually, or manual change + gopls rename

2. **Generic function inlining** - Not yet supported
   - Workaround: Manual refactoring for generics

3. **Comment preservation** - Some operations may lose comments
   - Workaround: Manually restore comments from git diff

4. **Complex package moves** - Edge cases exist
   - Workaround: Test in separate branch first

---

## Quick Reference Card

| Operation | Command Pattern | Example |
|-----------|----------------|---------|
| Rename symbol | `gopls rename -w file:line:col Name` | `gopls rename -w calc.go:10:6 Add` |
| Move package | `gopls rename -w file:1:8 newpkg` | `gopls rename -w old/f.go:1:8 new` |
| Extract function | `gopls codeaction -exec -kind refactor.extract.function file:range` | See examples.md |
| Inline call | `gopls codeaction -exec -kind refactor.inline.call file:line:col` | See examples.md |
| Remove param | `gopls codeaction -exec -kind refactor.rewrite.removeUnusedParam file:line:col` | See examples.md |

---

## Examples

For detailed examples, see [EXAMPLES.md](EXAMPLES.md)

For complete gopls capabilities, see [GOPLS_REFERENCE.md](GOPLS_REFERENCE.md)

---

## Success Criteria

A successful gopls refactoring:
- ✅ All references updated across codebase
- ✅ Build succeeds: `go build ./...`
- ✅ Tests pass: `go test ./...`
- ✅ No manual find/replace needed
- ✅ Git diff shows only intended changes
- ✅ TDD RED-GREEN-REFACTOR flow maintained
