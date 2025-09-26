# Fix Build Issues Following TDD Methodology

## Purpose
Systematically fix build errors while enforcing core development methodology and preventing cascade failures.

## Prompt
```
You are tasked with fixing build errors following the mandatory TDD REFACTOR methodology from @00-core-development-methodology.mdc.

CRITICAL: You MUST follow this exact sequence to prevent cascade failures:

## PHASE 1: COMPREHENSIVE BUILD ERROR ANALYSIS (CHECKPOINT D)
1. **MANDATORY**: Run comprehensive lint check to identify ALL build errors:
   ```bash
   golangci-lint run --timeout=10m --max-issues-per-linter=0 --max-same-issues=0
   ```

2. **MANDATORY**: For EACH undefined symbol, execute CHECKPOINT D analysis:
   ```bash
   # HALT: Execute comprehensive symbol analysis
   codebase_search "[undefined_symbol] usage patterns and dependencies"
   grep -r "[undefined_symbol]" . --include="*.go" -n
   # Find constructor patterns
   grep -r "New[SymbolName]\|Create[SymbolName]" . --include="*.go"
   ```

3. **MANDATORY**: Present complete analysis in this format:
   ```
   🚨 UNDEFINED SYMBOL ANALYSIS:
   Symbol: [undefined_symbol]
   References found: [N files]
   Dependent infrastructure: [list missing types/functions]
   Scope: [minimal/medium/extensive]

   OPTIONS:
   A) Implement complete infrastructure ([X] files affected)
   B) Create minimal stub (may break [Y] files)
   C) Alternative approach: [suggest]
   ```

4. **MANDATORY**: Ask for user approval before ANY implementation.

## PHASE 2: SYSTEMATIC VALIDATION (CHECKPOINTS A, B, C)

### CHECKPOINT A: Type Reference Validation
**TRIGGER**: About to reference any struct field or type
**MANDATORY ACTION**:
```bash
# HALT: Validate type definition exists
read_file [target_file]
# RULE: Verify all imports and type definitions exist before referencing
```

### CHECKPOINT B: Function Reference Validation
**TRIGGER**: About to call any function
**MANDATORY ACTION**:
```bash
# HALT: Validate function signature
grep -r "func.*[FunctionName]" . --include="*.go" -A 3
# RULE: Verify function exists and signature matches before calling
```

### CHECKPOINT C: Import Validation
**TRIGGER**: About to use any package
**MANDATORY ACTION**:
```bash
# HALT: Validate import exists
grep -r "import.*[PackageName]" [target_file]
# RULE: Verify import statement exists before using package types
```

## PHASE 3: TDD REFACTOR COMPLIANCE

### MANDATORY REFACTOR RULES:
1. **ENHANCE existing code only** - NO new types/methods/files
2. **REUSE existing functions** - NO duplication across files
3. **PRESERVE integration** - Main application usage MUST be maintained
4. **VALIDATE before change** - Use tools to verify before editing

### FORBIDDEN ACTIONS:
- ❌ Creating new interfaces without business requirement
- ❌ Duplicating functions across test files
- ❌ Using `interface{}` instead of proper types
- ❌ Removing files without dependency analysis
- ❌ Assuming imports/types exist without validation

## PHASE 4: SYSTEMATIC REMEDIATION

### For Missing Imports:
1. **VALIDATE**: Check if import should exist: `grep -r "import.*[package]" [file]`
2. **ADD**: Only if missing: Add proper import statement
3. **VERIFY**: Confirm package is available: `go list [package]`

### For Missing Functions:
1. **SEARCH**: Find existing equivalent: `codebase_search "similar [function] implementations"`
2. **REUSE**: Use existing function if available
3. **MINIMAL**: Create only if no alternative exists

### For Type Mismatches:
1. **READ**: Actual function signature: `grep -r "func.*[FunctionName]" . -A 3`
2. **MATCH**: Update call to match signature exactly
3. **VALIDATE**: Confirm types are compatible

## PHASE 5: VERIFICATION

### MANDATORY CHECKS:
1. **Build Validation**: `go build ./...`
2. **Lint Compliance**: `golangci-lint run --timeout=5m`
3. **Test Compilation**: `go test -c ./test/...`
4. **Integration Preserved**: `grep -r "[ComponentName]" cmd/ --include="*.go"`

### SUCCESS CRITERIA:
- ✅ All build errors resolved
- ✅ No new lint errors introduced
- ✅ All tests compile successfully
- ✅ Main application integration preserved
- ✅ No function duplication across files

## EMERGENCY PROTOCOLS:

### If Cascade Failures Occur:
1. **STOP** immediately
2. **RESTORE** any deleted files: `git checkout HEAD -- [file]`
3. **ANALYZE** full dependency chain before proceeding
4. **ASK** for user guidance on approach

### If Methodology Violations Detected:
1. **HALT** current approach
2. **REPORT** specific violation
3. **REQUEST** approval for corrective action
4. **RESTART** from PHASE 1 with proper methodology

## CONFIDENCE ASSESSMENT (REQUIRED):
After completion, provide:
```
Build Fix Confidence: [60-100]%
Methodology Compliance: ✅/❌ All checkpoints executed
Integration Preserved: ✅/❌ Main application usage maintained
Risk Assessment: [Description of remaining risks]
```

Remember: The goal is not just to fix build errors, but to do so while maintaining code quality, following TDD methodology, and preventing future cascade failures.
```

## Usage
Use this shortcut when you encounter build errors and need systematic remediation that follows the core development methodology.

## Key Differences from `/investigate-build`
- **Enforces methodology compliance** through mandatory checkpoints
- **Prevents cascade failures** through comprehensive dependency analysis
- **Requires user approval** before making changes
- **Validates integration preservation** throughout the process
- **Provides systematic remediation** rather than just error identification
