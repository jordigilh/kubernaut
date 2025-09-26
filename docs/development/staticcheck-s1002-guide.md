# Staticcheck S1002 Violation Prevention Guide

## What is S1002?

S1002 is a staticcheck rule that detects redundant nil checks before calling `len()` on slices, maps, or strings. In Go, calling `len()` on a nil slice, map, or string returns 0, making explicit nil checks unnecessary.

## The Problem

❌ **Redundant Pattern (Violates S1002):**
```go
if slice != nil && len(slice) > 0 {
    // Process slice
}

if params != nil && len(params) > 3 {
    // Process parameters
}
```

✅ **Correct Pattern:**
```go
if len(slice) > 0 {
    // Process slice
}

if len(params) > 3 {
    // Process parameters
}
```

## Why This Matters

1. **Code Simplicity**: Removes unnecessary checks that don't add value
2. **Go Idioms**: Following Go's design where `len(nil)` returns 0
3. **Consistency**: Ensures codebase follows staticcheck best practices
4. **Build Quality**: Prevents lint failures in CI/CD

## How We Prevent S1002 Violations

### 1. Pre-commit Hooks
The project has pre-commit hooks that detect S1002 patterns:

```bash
# This pattern is automatically detected and blocks commits:
grep -q "!= nil && len(" staged_file.go
```

### 2. Enhanced Linting Configuration
`.golangci.yml` is configured to catch S1002 violations:

```yaml
linters:
  enable:
    - staticcheck
    - gosimple      # Includes S1002 detection

linters-settings:
  staticcheck:
    checks: ["all", "-SA1019"] # All checks including S1002
```

### 3. Automated Fix Script
Use the provided script to fix all S1002 violations:

```bash
# Fix all S1002 violations in the codebase
make fix-s1002

# Or run the script directly
./scripts/fix-s1002-violations.sh
```

### 4. CI/CD Integration
The build pipeline fails on staticcheck violations, including S1002.

## Common S1002 Patterns and Fixes

### Pattern 1: Slice Length Check
```go
// ❌ Before (S1002 violation)
if step.Action.Parameters != nil && len(step.Action.Parameters) > 3 {
    complexActionCount++
}

// ✅ After (Fixed)
if len(step.Action.Parameters) > 3 {
    complexActionCount++
}
```

### Pattern 2: Map Key Check
```go
// ❌ Before (S1002 violation)
if config != nil && len(config) > 0 {
    processConfig(config)
}

// ✅ After (Fixed)
if len(config) > 0 {
    processConfig(config)
}
```

### Pattern 3: String Length Check
```go
// ❌ Before (S1002 violation)
if str != nil && len(str) > 0 {
    return str
}

// ✅ After (Fixed)
if len(str) > 0 {
    return str
}
```

## Developer Workflow

### Before Committing
1. **Run pre-commit validation** (happens automatically):
   ```bash
   # Pre-commit hook runs automatically
   git commit -m "Your changes"
   ```

2. **If S1002 violations are detected**, fix them:
   ```bash
   # Auto-fix all S1002 violations
   make fix-s1002

   # Verify the fix
   make lint

   # Commit again
   git add .
   git commit -m "Fix S1002 violations and implement feature"
   ```

### During Development
1. **Use correct patterns from the start**:
   ```go
   // Always use this pattern
   if len(collection) > threshold {
       // Process collection
   }
   ```

2. **Run linting frequently**:
   ```bash
   make lint  # Catches S1002 and other issues early
   ```

## Testing the Fix

After applying S1002 fixes, always verify:

1. **Code still compiles**:
   ```bash
   go build ./...
   ```

2. **Tests still pass**:
   ```bash
   make test
   ```

3. **No new lint errors**:
   ```bash
   make lint
   ```

## Integration with Kubernaut Rules

This prevention strategy aligns with kubernaut project guidelines:

- **[00-project-guidelines.mdc]**: Mandatory code quality standards
- **[02-go-coding-standards.mdc]**: Go-specific patterns and practices
- **TDD Workflow**: Fix linting before implementing business logic

## Quick Reference

| Command | Purpose |
|---------|---------|
| `make fix-s1002` | Auto-fix all S1002 violations |
| `make lint` | Check for all linting issues including S1002 |
| `./scripts/fix-s1002-violations.sh` | Direct script execution |
| `git commit` | Pre-commit hook automatically checks for S1002 |

## Need Help?

- **Auto-fix**: Use `make fix-s1002` for most cases
- **Manual review**: Check complex patterns that auto-fix can't handle
- **Documentation**: This guide covers common patterns and solutions
- **Project rules**: See `.cursor/rules/` for broader coding standards
