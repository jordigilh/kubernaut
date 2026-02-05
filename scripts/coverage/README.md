# Coverage Analysis Scripts

This directory contains modular, testable scripts for calculating code coverage across Kubernaut services.

## 📁 Directory Structure

```
scripts/coverage/
├── README.md                                    # This file
├── report.sh                                    # Main coverage report generator
├── calculate_go_unit_testable.awk              # Go unit-testable coverage
├── calculate_go_integration_testable.awk       # Go integration-testable coverage
├── merge_go_coverage.awk                       # Merge Go coverage from multiple tiers
├── calculate_python_unit_testable.awk          # Python unit-testable coverage
├── calculate_python_integration_testable.awk   # Python integration-testable coverage
└── test/
    ├── test_awk_scripts.sh                     # Unit tests for AWK scripts
    └── fixtures/
        ├── sample_go_coverage.out              # Sample Go coverage file
        └── sample_python_coverage.txt          # Sample Python coverage file
```

## 🚀 Quick Start

### Generate Coverage Report

```bash
# Full table report (all services, all tiers)
./scripts/coverage/report.sh

# Or via Make
make coverage-report-unit-testable

# JSON output for CI/CD
make coverage-report-json

# Filter by service
./scripts/coverage/report.sh --service datastorage

# No color output (for CI)
./scripts/coverage/report.sh --no-color
```

### Run Unit Tests

```bash
# Test all AWK scripts
./scripts/coverage/test/test_awk_scripts.sh
```

## 📊 Coverage Categorization

### Unit-Testable Code
Pure logic packages with no I/O dependencies:
- `config/` - Configuration parsing
- `validation/` - Validators and business rules
- `models/` - Data models and types
- `formatting/` - Formatters and builders
- `metrics/` - Metrics helpers (not collection)
- `classifier/` - Classification logic
- `detection/` - Detection logic
- `routing/` - Routing logic
- `retry/` - Retry policies
- `rego/` - Rego policy evaluation
- `types/`, `conditions/` - Type definitions

### Integration-Testable Code
Packages with I/O, external dependencies:
- `server/` - HTTP servers
- `handler/` - Request handlers
- `repository/` - Database adapters
- `k8s/` - Kubernetes clients
- `delivery/` - Notification delivery
- `client/` - External service clients
- `cache/` - Cache implementations
- `enricher/` - Enrichment with external data
- `status/` - Status managers (K8s API)
- `audit/` - Audit event infrastructure
- `phase/` - Phase managers (K8s API)
- `creator/` - CRD creators (K8s API)

## 🔧 AWK Scripts Reference

### calculate_go_unit_testable.awk

**Purpose**: Calculate coverage for unit-testable Go code

**Usage**:
```bash
awk -f calculate_go_unit_testable.awk \
    -v pkg_pattern="/pkg/aianalysis/" \
    -v exclude_pattern="/(handler\.go|audit)/" \
    coverage_unit_aianalysis.out
```

**Output**: `71.8%`

**Parameters**:
- `pkg_pattern`: Regex to match package (e.g., `/pkg/aianalysis/`)
- `exclude_pattern`: Regex for integration-only code to exclude

### calculate_go_integration_testable.awk

**Purpose**: Calculate coverage for integration-testable Go code

**Usage**:
```bash
awk -f calculate_go_integration_testable.awk \
    -v pkg_pattern="/pkg/aianalysis/" \
    -v include_pattern="/(handler\.go|audit)/" \
    coverage_integration_aianalysis.out
```

**Output**: `43.5%`

**Parameters**:
- `pkg_pattern`: Regex to match package
- `include_pattern`: Regex for integration-only code to include

### merge_go_coverage.awk

**Purpose**: Merge coverage from multiple tiers (unit, integration, E2E)

**Usage**:
```bash
awk -f merge_go_coverage.awk \
    -v pkg_pattern="/pkg/aianalysis/" \
    coverage_unit_aianalysis.out \
    coverage_integration_aianalysis.out \
    coverage_e2e_aianalysis.out
```

**Output**: `76.9%`

**Algorithm**: For each code location (file:lines), mark as covered if ANY input file shows count > 0

### calculate_python_unit_testable.awk

**Purpose**: Calculate unit-testable coverage from pytest-cov report

**Usage**:
```bash
awk -f calculate_python_unit_testable.awk coverage_unit_holmesgpt-api.txt
```

**Output**: `76.0%`

**Includes**: `models/`, `validation/`, `sanitization/`, `toolsets/`, `config/`, `audit/buffered_store.py`, `errors.py`

### calculate_python_integration_testable.awk

**Purpose**: Calculate integration-testable coverage from pytest-cov report

**Usage**:
```bash
awk -f calculate_python_integration_testable.awk coverage_integration_holmesgpt-api_python.txt
```

**Output**: `43.5%`

**Includes**: `extensions/`, `middleware/`, `auth/`, `clients/`, `main.py`, audit/DB infrastructure

## 🧪 Testing

### Running Tests

```bash
cd scripts/coverage/test
./test_awk_scripts.sh
```

### Expected Output

```
═══════════════════════════════════════════
AWK Coverage Script Unit Tests
═══════════════════════════════════════════

Testing calculate_go_unit_testable.awk...
✓ PASS: Go unit-testable coverage (excludes handler)
✓ PASS: Go unit-testable coverage (excludes handler and validation)

Testing calculate_go_integration_testable.awk...
✓ PASS: Go integration-testable coverage (handler only)

Testing calculate_python_unit_testable.awk...
✓ PASS: Python unit-testable coverage

Testing calculate_python_integration_testable.awk...
✓ PASS: Python integration-testable coverage

Testing merge_go_coverage.awk...
✓ PASS: merge_go_coverage.awk runs without error

═══════════════════════════════════════════
Test Summary
═══════════════════════════════════════════
Tests run:    6
Tests passed: 6
Tests failed: 0

✓ All tests passed!
```

## 📝 Configuration

Coverage categorization patterns are defined in `.coverage-patterns.yaml` at the repository root.

### Adding a New Service

1. Add service categorization to `.coverage-patterns.yaml`:
   ```yaml
   go_services:
     mynewservice:
       pkg_pattern: "/pkg/mynewservice/"
       unit_exclude: "/(handler|server)/"
       integration_include: "/(handler|server)/"
   ```

2. The `report.sh` script will automatically pick up the new service

3. No Makefile changes needed!

### Modifying Categorization

Edit `.coverage-patterns.yaml` to adjust which packages are unit-testable vs integration-testable.

## 🎯 Quality Targets

- **Unit-Testable**: ≥70% (pure logic should be well-tested)
- **Integration**: ≥60% (handlers/servers should have good integration coverage)
- **All Tiers**: ≥80% (overall coverage goal)

## 📚 References

- **Coverage Analysis Report**: `docs/testing/COVERAGE_ANALYSIS_REPORT.md`
- **Refactoring Analysis**: `docs/development/MAKEFILE_REFACTORING_ANALYSIS.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`
- **Config File**: `.coverage-patterns.yaml`

## 🔧 Troubleshooting

### AWK Script Fails

```bash
# Test script manually
awk -f calculate_go_unit_testable.awk \
    -v pkg_pattern="/pkg/aianalysis/" \
    -v exclude_pattern="/(handler\.go|audit)/" \
    coverage_unit_aianalysis.out

# Run unit tests
./test/test_awk_scripts.sh
```

### Coverage Report Shows Unexpected Values

```bash
# Check coverage file exists and has data
ls -lh coverage_unit_aianalysis.out
head -10 coverage_unit_aianalysis.out

# Verify patterns in config
cat .coverage-patterns.yaml | grep -A5 "aianalysis:"

# Run with debug output
bash -x ./scripts/coverage/report.sh --service aianalysis
```

### Missing Coverage Files

```bash
# Regenerate coverage files
make test-tier-unit
make test-tier-integration
make test-tier-e2e

# Then re-run report
make coverage-report-unit-testable
```
