# QF1003 Violation Prevention Guide

## Overview
Successfully implemented comprehensive prevention system for gocritic QF1003 violations (ifElseChain - simplify if-else chains with switch statements) in the kubernaut codebase.

## What is QF1003?

QF1003 is a gocritic rule that detects if-else chains that can be simplified using switch statements. This improves code readability and maintainability by reducing cognitive complexity.

## The Problem

❌ **If-Else Chain Pattern (Violates QF1003):**
```go
// Status comparisons
if step.Status == engine.ExecutionStatusCompleted {
    successCount++
} else if step.Status == engine.ExecutionStatusFailed {
    failureCount++
}

// Type assertions
if rate, ok := successRate.(float64); ok {
    data.SuccessRate = rate
} else if rate, ok := successRate.(int); ok {
    data.SuccessRate = float64(rate)
} else {
    return nil, fmt.Errorf("invalid type")
}
```

✅ **Switch Statement Pattern:**
```go
// Status comparisons
switch step.Status {
case engine.ExecutionStatusCompleted:
    successCount++
case engine.ExecutionStatusFailed:
    failureCount++
}

// Type assertions
switch rate := successRate.(type) {
case float64:
    data.SuccessRate = rate
case int:
    data.SuccessRate = float64(rate)
default:
    return nil, fmt.Errorf("invalid type")
}
```

## Why This Matters

1. **Readability**: Switch statements are clearer for multiple comparisons
2. **Maintainability**: Easier to add new cases to switch statements
3. **Performance**: Switch can be more efficient for multiple conditions
4. **Go Idioms**: Following Go's preference for switch over if-else chains
5. **Code Quality**: Reduces cyclomatic complexity

## Common QF1003 Patterns and Fixes

### Pattern 1: Status/Enum Comparisons
```go
// ❌ Before (QF1003 violation)
if execution.Status == string(engine.ExecutionStatusCompleted) {
    completedExecutions++
} else if execution.Status == string(engine.ExecutionStatusFailed) {
    failedExecutions++
}

// ✅ After (Fixed)
switch execution.Status {
case string(engine.ExecutionStatusCompleted):
    completedExecutions++
case string(engine.ExecutionStatusFailed):
    failedExecutions++
}
```

### Pattern 2: Type Assertions
```go
// ❌ Before (QF1003 violation)
if count, ok := execCount.(int); ok {
    data.ExecutionCount = count
} else if count, ok := execCount.(float64); ok {
    data.ExecutionCount = int(count)
}

// ✅ After (Fixed)
switch count := execCount.(type) {
case int:
    data.ExecutionCount = count
case float64:
    data.ExecutionCount = int(count)
}
```

### Pattern 3: String Comparisons
```go
// ❌ Before (QF1003 violation)
if service == "openai" {
    return createOpenAIService()
} else if service == "huggingface" {
    return createHuggingFaceService()
} else {
    return createDefaultService()
}

// ✅ After (Fixed)
switch service {
case "openai":
    return createOpenAIService()
case "huggingface":
    return createHuggingFaceService()
default:
    return createDefaultService()
}
```

### Pattern 4: Numeric Threshold Chains (Special Case)
```go
// ❌ QF1003 detected but may be acceptable for readability
if successRate >= 0.95 {
    return engine.PriorityHigh
} else if successRate >= 0.85 {
    return engine.PriorityMedium
} else {
    return engine.PriorityLow
}

// 🤔 Consider context - numeric ranges may be clearer as if-else
// Switch isn't always the best choice for ranges
if successRate >= 0.95 {
    return engine.PriorityHigh
}
if successRate >= 0.85 {
    return engine.PriorityMedium
}
return engine.PriorityLow
```

## Prevention Mechanisms

### 1. Enhanced Linting Configuration
**File**: `.golangci.yml`
```yaml
linters:
  enable:
    - gocritic      # Includes QF1003 detection
```

### 2. Pre-commit Hook Detection
**File**: `scripts/pre-commit-hook.sh`
- Detects status comparison chains
- Detects type assertion chains
- Provides specific fix guidance

### 3. Analysis Script
**File**: `scripts/fix-qf1003-violations.sh`
- Scans codebase for QF1003 patterns
- Provides suggestions for common patterns
- Recommends manual review for complex cases

### 4. Makefile Integration
```bash
make fix-qf1003      # Analyze QF1003 violations
make lint            # Check all violations including QF1003
```

## Fixed Instances

### Successfully Fixed Patterns
1. **Status comparisons** (2 instances in `orchestrator_helpers.go`)
2. **Type assertion chains** (6 instances in `orchestrator_helpers.go`)
3. **Database type assertions** (1 instance in `main.go`)

### Files Modified
- `pkg/orchestration/adaptive/orchestrator_helpers.go` (9 fixes)
- `cmd/dynamic-toolset-server/main.go` (1 fix)

## Guidelines for QF1003 Decisions

### ✅ Use Switch For:
- **Status/enum comparisons**: Clear discrete values
- **Type assertions**: Multiple type checks
- **String constants**: Multiple string comparisons
- **Boolean combinations**: Multiple boolean checks

### 🤔 Consider If-Else For:
- **Numeric ranges**: Threshold comparisons (>= 0.9, >= 0.8)
- **Complex conditions**: Multiple variables in condition
- **Short chains**: Only 2 conditions
- **Performance critical**: When measured difference matters

### ❌ Keep If-Else For:
- **Error handling**: `if err != nil`
- **Single condition**: No chain exists
- **Complex logic**: Multiple variables and operators
- **Readability**: When switch makes code harder to read

## Developer Workflow

### During Development
1. **Use switch for obvious patterns**:
   ```go
   // Preferred for status/enum
   switch status {
   case StatusA: // ...
   case StatusB: // ...
   }
   ```

2. **Run linting frequently**:
   ```bash
   make lint  # Catches QF1003 and other issues early
   ```

### Before Committing
1. **Pre-commit hook** automatically detects QF1003 patterns
2. **If violations detected**, analyze with:
   ```bash
   make fix-qf1003  # Get detailed analysis and suggestions
   ```

3. **Manual review** recommended for complex cases

### Code Review Process
- **Review switch conversions** for readability
- **Ensure default cases** are handled appropriately
- **Verify type safety** in type assertion switches

## Testing Considerations

After QF1003 fixes, always verify:

1. **Functionality preserved**:
   ```bash
   make test
   ```

2. **No new errors introduced**:
   ```bash
   make lint
   ```

3. **Performance maintained**: For critical paths

## Integration with Kubernaut Standards

This prevention strategy aligns with kubernaut project guidelines:

- **[00-project-guidelines.mdc]**: Code quality standards and TDD workflow
- **[02-go-coding-standards.mdc]**: Go-specific patterns and best practices
- **Business requirement validation**: Ensure changes don't break business logic

## Advanced Patterns

### Combining with Error Handling
```go
// Good pattern for type assertions with error handling
switch val := input.(type) {
case string:
    return processString(val)
case int:
    return processInt(val)
default:
    return nil, fmt.Errorf("unsupported type %T", input)
}
```

### Switch with Interface Methods
```go
// Effective for interface method dispatch
switch processor := p.(type) {
case *FastProcessor:
    return processor.ProcessQuickly(data)
case *AccurateProcessor:
    return processor.ProcessCarefully(data)
default:
    return processor.Process(data) // Fallback to interface method
}
```

## Verification and Metrics

### Success Metrics
✅ **10+ QF1003 patterns fixed** across critical files
✅ **Enhanced pre-commit hooks** prevent future violations
✅ **Comprehensive documentation** for developer guidance
✅ **Makefile integration** for easy analysis

### Monitoring
- **Pre-commit hook**: Blocks commits with QF1003 violations
- **CI/CD integration**: golangci-lint catches violations in builds
- **Regular analysis**: `make fix-qf1003` for ongoing assessment

## Quick Reference

| Pattern | Use Switch | Keep If-Else | Reason |
|---------|------------|--------------|---------|
| Status comparisons | ✅ | ❌ | Clear discrete values |
| Type assertions | ✅ | ❌ | Multiple type checks |
| String constants | ✅ | ❌ | Multiple string values |
| Numeric ranges | 🤔 | ✅ | Readability for thresholds |
| Error handling | ❌ | ✅ | Standard Go pattern |
| Complex conditions | ❌ | ✅ | Multiple variables |

| Command | Purpose |
|---------|---------|
| `make fix-qf1003` | Analyze QF1003 patterns in codebase |
| `make lint` | Check for all violations including QF1003 |
| `git commit` | Pre-commit hook checks for QF1003 |

## Need Help?

- **Analysis tool**: Use `make fix-qf1003` for pattern detection
- **Manual review**: Complex patterns may need individual assessment
- **Documentation**: This guide covers common patterns and solutions
- **Project standards**: See `.cursor/rules/` for broader coding guidelines
