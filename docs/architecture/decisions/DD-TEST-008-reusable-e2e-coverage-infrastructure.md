# DD-TEST-008: Reusable E2E Coverage Infrastructure

**Date**: 2025-12-23
**Status**: ✅ **APPROVED**
**Context**: Eliminate duplication across E2E coverage implementations
**Supersedes**: Individual service implementations (DataStorage, WorkflowExecution, SignalProcessing)
**Reference**: DD-TEST-007 (E2E Coverage Capture Standard)

---

## Problem Statement

All Go services in Kubernaut need E2E coverage collection, but each service was implementing identical logic:

### ❌ Current State (Duplicated)
```
DataStorage:         test-e2e-datastorage-coverage (45 lines)
WorkflowExecution:   test-e2e-workflowexecution-coverage (20 lines)
SignalProcessing:    test-e2e-signalprocessing-coverage (similar)
Gateway:             test-e2e-gateway-coverage (similar)
```

**Issues**:
- 4+ services with identical coverage logic
- Inconsistent output formatting
- Difficult to maintain (fix once, update everywhere)
- No coverage for Notification, AIAnalysis, RemediationOrchestrator, Toolset
- Error handling differs between services

---

## Decision

Create **reusable E2E coverage infrastructure** with:

1. **Shared Script**: `scripts/generate-e2e-coverage.sh`
   - Handles coverage report generation
   - Consistent error handling
   - Beautiful output formatting
   - Validates coverage data exists

2. **Makefile Template**: `Makefile.e2e-coverage.mk`
   - Single function: `define-e2e-coverage-target`
   - Generates service-specific targets
   - Follows DD-TEST-007 standard

---

## Implementation

### 1. Shared Coverage Script

**Location**: `scripts/generate-e2e-coverage.sh`

```bash
#!/bin/bash
# Usage: ./scripts/generate-e2e-coverage.sh <service> <coverdata-dir> <output-dir>

SERVICE_NAME="$1"
COVERDATA_DIR="$2"
OUTPUT_DIR="$3"

# Validates coverage data exists
# Generates text, HTML, and function reports
# Shows coverage percentage summary
# Provides helpful error messages
```

**Features**:
- ✅ Input validation
- ✅ Directory existence checks
- ✅ Non-empty coverage data validation
- ✅ Colored output
- ✅ Helpful error messages
- ✅ Generates 3 report types (text, HTML, function)
- ✅ Shows coverage percentage

### 2. Makefile Template

**Location**: `Makefile.e2e-coverage.mk`

```makefile
# Include in main Makefile
include Makefile.e2e-coverage.mk

# Define targets (one line per service!)
$(eval $(call define-e2e-coverage-target,notification,notification,4))
$(eval $(call define-e2e-coverage-target,datastorage,datastorage,4))
$(eval $(call define-e2e-coverage-target,gateway,gateway,4))
```

**Generated Target** (for `notification`):
```makefile
test-e2e-notification-coverage:
    # 1. Build with E2E_COVERAGE=true
    # 2. Run E2E tests
    # 3. Generate coverage reports via script
    # 4. Show summary
```

---

## Usage

### Adding Coverage to a New Service

**Step 1**: Ensure Dockerfile supports coverage (DD-TEST-007)
```dockerfile
ARG GOFLAGS=""

RUN if [ "${GOFLAGS}" = "-cover" ]; then \
        CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
            -o service-controller ./cmd/service; \
    else \
        CGO_ENABLED=0 GOOS=linux go build \
            -ldflags="-s -w" \
            -o service-controller ./cmd/service; \
    fi
```

**Step 2**: Add to main Makefile
```makefile
# At top of Makefile
include Makefile.e2e-coverage.mk

# In E2E section
$(eval $(call define-e2e-coverage-target,myservice,myservice,4))
```

**Step 3**: Run coverage
```bash
make test-e2e-myservice-coverage
```

**That's it!** No custom logic needed.

---

## Migration Path

### Before (DataStorage - 45 lines of duplicated logic)
```makefile
test-e2e-datastorage-coverage:
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📊 Data Storage Service - E2E Coverage Collection"
	@echo "════════════════════════════════════════════════════════════════════════"
	@echo "📋 Collecting coverage from:"
	@echo "   • Binary profiling (Go 1.20+) during E2E execution"
	@echo "   • Graceful shutdown triggers coverage data write"
	@echo "   • Coverage directory: ./coverdata/"
	@echo ""
	@echo "🏗️  Building Docker image with coverage instrumentation..."
	@echo "   Setting E2E_COVERAGE=true to enable GOFLAGS=-cover in Dockerfile"
	@echo ""
	@$(MAKE) E2E_COVERAGE=true test-e2e-datastorage
	@echo ""
	@echo "📊 Step 3: Generating coverage reports..."
	@if [ -d "./coverdata" ] && [ -n "$$(ls -A ./coverdata 2>/dev/null)" ]; then \
		echo "   Generating text coverage report..."; \
		go tool covdata textfmt -i=./coverdata -o e2e-coverage.txt && \
		echo "   ✅ Coverage report: e2e-coverage.txt"; \
		# ... 30 more lines ...
```

### After (1 line!)
```makefile
$(eval $(call define-e2e-coverage-target,datastorage,datastorage,4))
```

**Reduction**: 45 lines → 1 line (97.8% reduction)

---

## Benefits

### ✅ Developer Experience
- **Add coverage in 1 line** instead of copying 45+ lines
- **Consistent behavior** across all services
- **Better error messages** with troubleshooting hints
- **Less maintenance** (fix once, applies everywhere)

### ✅ Code Quality
- **DRY principle** (Don't Repeat Yourself)
- **Single source of truth** for coverage logic
- **Easier to test** (one script vs. N Makefile targets)
- **Easier to enhance** (add features once, benefit everywhere)

### ✅ Coverage Adoption
- **Lower barrier to entry** for new services
- **Standardized output** makes comparison easier
- **Encourages E2E testing** (easy to enable)

---

## Example: Adding Coverage to Notification Service

### Step 1: Add to Makefile (1 line)
```makefile
# After including Makefile.e2e-coverage.mk
$(eval $(call define-e2e-coverage-target,notification,notification,4))
```

### Step 2: Run
```bash
make test-e2e-notification-coverage
```

### Output
```
════════════════════════════════════════════════════════════════════════
📊 Notification Service - E2E Coverage Collection (DD-TEST-007)
════════════════════════════════════════════════════════════════════════
📋 Collecting coverage from:
   • Binary profiling (Go 1.20+) during E2E execution
   • Graceful shutdown triggers coverage data write
   • Coverage directory: ./test/e2e/notification/coverdata/

🏗️  Step 1: Building notification image with coverage instrumentation...
   Setting E2E_COVERAGE=true to enable GOFLAGS=-cover in Dockerfile

[... E2E tests run ...]

📊 Step 2: Generating coverage reports...
   ✅ Text report: ./test/e2e/notification/e2e-coverage.txt
   ✅ HTML report: ./test/e2e/notification/e2e-coverage.html
   ✅ Function report: ./test/e2e/notification/e2e-coverage-func.txt

📈 Coverage Summary for notification Service:
	main		coverage: 65.2% of statements
	pkg/notification/delivery	coverage: 78.9% of statements
	[... more packages ...]

✅ Coverage Reports Generated Successfully
  📄 Text:     ./test/e2e/notification/e2e-coverage.txt
  🌐 HTML:     ./test/e2e/notification/e2e-coverage.html
  📊 Function: ./test/e2e/notification/e2e-coverage-func.txt
  📁 Data:     ./test/e2e/notification/coverdata/

💡 View HTML report:
   open ./test/e2e/notification/e2e-coverage.html
════════════════════════════════════════════════════════════════════════
```

---

## Error Handling

The script provides **helpful error messages** for common issues:

### Empty Coverage Data
```
⚠️  Coverage data directory is empty: ./test/e2e/notification/coverdata/
Possible causes:
  • Controller built without GOFLAGS=-cover
  • GOCOVERDIR not set in deployment
  • Controller crashed before coverage flush
  • Permission issues (coverdata directory not writable)
```

### Missing Coverage Directory
```
⚠️  Coverage data directory not found: ./test/e2e/notification/coverdata/
Did the controller shut down gracefully to flush coverage data?
```

---

## Rollout Plan

### Phase 1: Create Infrastructure ✅ COMPLETE
- [x] Create `scripts/generate-e2e-coverage.sh`
- [x] Create `Makefile.e2e-coverage.mk`
- [x] Document in DD-TEST-008

### Phase 2: Migrate Existing Services
- [ ] Notification: Add `$(eval $(call define-e2e-coverage-target,notification,notification,4))`
- [ ] DataStorage: Replace 45-line target with 1-line call
- [ ] Gateway: Replace custom target with 1-line call
- [ ] WorkflowExecution: Replace custom target with 1-line call
- [ ] SignalProcessing: Replace custom target with 1-line call

### Phase 3: Enable New Services
- [ ] RemediationOrchestrator
- [ ] AIAnalysis
- [ ] Toolset

---

## Validation

### Testing the Script
```bash
# Test with a service that has coverage data
./scripts/generate-e2e-coverage.sh \
    datastorage \
    ./test/e2e/datastorage/coverdata \
    ./test/e2e/datastorage

# Should generate:
#   - e2e-coverage.txt
#   - e2e-coverage.html
#   - e2e-coverage-func.txt
```

### Testing the Makefile Function
```bash
# After adding to Makefile
make test-e2e-notification-coverage

# Should:
#   1. Build with coverage
#   2. Run E2E tests
#   3. Generate reports
#   4. Show coverage summary
```

---

## Compliance

### DD-TEST-007 Compliance
✅ Uses `GOFLAGS=-cover` for build
✅ Uses `GOCOVERDIR` in deployment
✅ Generates text, HTML, and function reports
✅ Shows coverage percentage
✅ Validates coverage data exists

### Code Quality Standards
✅ DRY principle (single source of truth)
✅ Consistent error handling
✅ Clear documentation
✅ Easy to test and maintain

---

## Future Enhancements

### Potential Additions
1. **Coverage Trending**: Compare with baseline
2. **Coverage Gates**: Fail if below threshold
3. **Coverage Diff**: Show change from previous run
4. **Parallel Report Generation**: Speed up for large codebases
5. **Slack/Email Notifications**: Alert on coverage changes

---

## Cross-References

- **DD-TEST-007**: E2E Coverage Capture Standard (technical foundation)
- **DD-TEST-002**: Parallel Test Execution Standard (E2E test patterns)
- **ADR-005**: Integration Test Coverage (>50% target)
- **TESTING_GUIDELINES.md**: Overall testing strategy

---

## Success Criteria

This decision is successful when:
- ✅ All services use the same E2E coverage infrastructure
- ✅ Adding coverage to a new service takes < 2 minutes
- ✅ Coverage reports are consistent across all services
- ✅ Developers understand how to use the system
- ✅ Maintenance burden is reduced (1 script vs. N targets)

---

**Document Owner**: Platform Team
**Last Updated**: 2025-12-23
**Next Review**: After V1.0 (validate with all 8 services)

