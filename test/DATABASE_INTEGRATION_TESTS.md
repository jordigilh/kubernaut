# Database Integration Tests

This document describes the comprehensive database integration tests for the prometheus-alerts-slm project, which test the complete workflow including PostgreSQL database persistence, oscillation detection, and MCP server functionality.

## Overview

The database integration tests extend the existing integration test suite to include:

1. **Database Integration & Action History Storage** - PostgreSQL connectivity, schema migrations, and data persistence
2. **Oscillation Detection Integration** - Pattern detection algorithms with real database data
3. **MCP Server Integration with Database** - MCP tools with live database backend
4. **End-to-End Workflow with Database Persistence** - Complete workflow including database operations

## Test Architecture

### Test Structure

```
test/integration/
├── database_test_utils.go              # Shared database utilities
├── database_integration_test.go        # Context 1: Database & Action History
├── oscillation_integration_test.go     # Context 2: Oscillation Detection
├── mcp_database_integration_test.go    # Context 3: MCP Server + Database
├── workflow_database_integration_test.go # Context 4: End-to-End Workflow
└── DATABASE_INTEGRATION_TESTS.md       # This documentation
```

### Integration with Existing Tests

The new database tests are integrated into the existing `OllamaIntegrationSuite` and follow the same patterns:
- **Ginkgo/Gomega** testing framework
- **Resource monitoring** and performance measurement
- **Comprehensive reporting** with metrics
- **Test environment management** with cleanup

## Test Contexts

### Context 1: Database Integration & Action History Storage

**Purpose**: Test PostgreSQL integration, action history persistence, and data integrity.

#### Test Scenarios:

1. **TestDatabaseConnectivity**
   - Verifies PostgreSQL container deployment and connectivity
   - Tests connection string validation and environment configuration
   - Validates schema migration execution (001, 002, 003)

2. **TestActionHistoryPersistence**
   - Executes actions via existing workflow and verifies database storage
   - Tests action trace creation, parameter storage (JSONB), and metadata
   - Validates action effectiveness score updates
   - Tests action history retrieval with filtering

3. **TestActionHistoryRetention**
   - Generates multiple actions over time
   - Tests automatic retention policies
   - Verifies old action cleanup without data corruption

4. **TestConcurrentActionStorage**
   - Executes multiple actions simultaneously
   - Verifies database transaction integrity
   - Tests action history concurrency without conflicts

### Context 2: Oscillation Detection Integration

**Purpose**: Test oscillation detection algorithms with real data flow.

#### Test Scenarios:

1. **TestOscillationDetectionWorkflow**
   - Generates sequence of scaling actions that create oscillation patterns
   - Executes SLM → Action → Storage → Detection pipeline
   - Verifies oscillation detection triggers at appropriate thresholds

2. **TestScaleOscillationDetection**
   - Creates deployment scale-up/scale-down patterns
   - Stores action history via normal workflow
   - Triggers oscillation detection and verifies results match expected patterns

3. **TestResourceThrashingDetection**
   - Alternates between `scale_deployment` and `increase_resources` actions
   - Stores action sequences in database
   - Verifies thrashing detection identifies problematic patterns

4. **TestIneffectiveLoopDetection**
   - Repeats same action type with declining effectiveness scores
   - Stores effectiveness updates in database
   - Verifies loop detection identifies repetitive ineffective actions

### Context 3: MCP Server Integration with Database

**Purpose**: Test MCP tools with real database backend.

#### Test Scenarios:

1. **TestMCPActionHistoryRetrieval**
   - Populates database with action history via normal workflow
   - Tests MCP `get_action_history` tool with real database queries
   - Verifies filtering, pagination, and response formatting

2. **TestMCPOscillationAnalysis**
   - Generates oscillation patterns in database
   - Tests MCP `analyze_oscillation_patterns` tool with stored procedures
   - Verifies analysis results match database-stored patterns

3. **TestMCPActionSafetyChecks**
   - Stores historical action patterns that indicate risks
   - Tests MCP `check_action_safety` tool with real detection results
   - Verifies safety recommendations based on stored patterns

4. **TestMCPEffectivenessAnalysis**
   - Stores varied action effectiveness data
   - Tests MCP `get_action_effectiveness` tool with statistical analysis
   - Verifies effectiveness metrics calculation accuracy

### Context 4: End-to-End Workflow with Database Persistence

**Purpose**: Complete workflow integration including database operations.

#### Test Scenarios:

1. **TestEndToEndWithActionStorage**
   - Executes complete workflow: Alert → SLM → Action → Database Storage
   - Verifies action traces are properly stored with all metadata
   - Tests action effectiveness score updates after execution

2. **TestWorkflowResilience**
   - Tests workflow continuation after database connection issues
   - Verifies graceful degradation when database is unavailable
   - Tests action execution continues even with storage failures

3. **TestLongRunningWorkflowPatterns**
   - Executes extended sequences of actions over time
   - Verifies pattern detection emerges from accumulated data
   - Tests that oscillation detection prevents harmful loops

## Prerequisites

### Database Setup

The integration tests require a PostgreSQL database. The test runner automatically handles database deployment using Podman:

```bash
# Automated setup (recommended)
./scripts/run-database-integration-tests.sh

# Manual setup
./scripts/deploy-postgres.sh
```

### Environment Configuration

The tests use the following environment variables:

```bash
# Database Configuration
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_NAME="action_history"
export DB_USER="slm_user"
export DB_PASSWORD="slm_password_dev"
export DB_SSL_MODE="disable"

# Test Configuration
export OLLAMA_ENDPOINT="http://localhost:11434"
export OLLAMA_MODEL="granite3.1-dense:8b"
export TEST_TIMEOUT="120s"
export LOG_LEVEL="debug"
export SKIP_SLOW_TESTS="false"

# Kubernetes Test Environment
export KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.33.0-darwin-arm64"
```

### Dependencies

- **PostgreSQL**: Database server (deployed via Podman)
- **Ollama**: LLM server for SLM analysis
- **Podman**: Container runtime for PostgreSQL
- **Go 1.21+**: For running tests
- **Kubebuilder test assets**: For Kubernetes client simulation

## Running the Tests

### Using the Test Runner (Recommended)

The provided test runner script handles database setup, test execution, and cleanup:

```bash
# Run all database integration tests
./scripts/run-database-integration-tests.sh

# Run specific test contexts
./scripts/run-database-integration-tests.sh contexts

# Run with existing database (skip setup)
./scripts/run-database-integration-tests.sh test --skip-setup

# Run specific tests
./scripts/run-database-integration-tests.sh --specific="TestDatabaseConnectivity"

# Skip slow tests
./scripts/run-database-integration-tests.sh --skip-slow
```

### Manual Test Execution

```bash
# Setup database
./scripts/deploy-postgres.sh

# Set environment variables
export DB_HOST="localhost"
export DB_PORT="5432"
# ... (see Environment Configuration above)

# Run integration tests
go test -v -tags=integration ./test/integration/... -timeout=30m

# Run specific contexts
go test -v -tags=integration ./test/integration/... -run="TestDatabaseIntegration" -timeout=30m
```

### Test Categories

```bash
# Database connectivity and setup
go test -v -tags=integration ./test/integration/... -run="TestDatabaseConnectivity"

# Action history persistence
go test -v -tags=integration ./test/integration/... -run="TestActionHistoryPersistence"

# Oscillation detection
go test -v -tags=integration ./test/integration/... -run="TestOscillationDetection"

# MCP server integration
go test -v -tags=integration ./test/integration/... -run="TestMCPDatabase"

# End-to-end workflow
go test -v -tags=integration ./test/integration/... -run="TestWorkflowDatabase"
```

## Test Data and Patterns

### Test Action Patterns

The tests create realistic action patterns to trigger oscillation detection:

1. **Scale Oscillation Pattern**:
   ```
   replicas: 2 → 4 → 2 → 4 → 2
   time: 0min → 5min → 10min → 15min → 20min
   effectiveness: 0.8 → 0.65 → 0.5 → 0.35 → 0.2
   ```

2. **Resource Thrashing Pattern**:
   ```
   actions: scale_deployment → increase_resources → scale_deployment → increase_resources
   time gaps: 3 minutes between each action
   effectiveness: declining from 0.6 to 0.3
   ```

3. **Ineffective Loop Pattern**:
   ```
   action_type: repeated "scale_deployment"
   repetitions: 6 times
   effectiveness: declining from 0.9 to 0.4
   ```

### Database Schema Validation

Tests verify that all required database objects exist:

- **Tables**: `resource_references`, `action_histories`, `resource_action_traces`, `oscillation_patterns`, `oscillation_detections`
- **Stored Procedures**: `detect_scale_oscillation`, `detect_resource_thrashing`, `detect_ineffective_loops`, `detect_cascading_failures`, `get_action_traces`, `get_action_effectiveness`, `store_oscillation_detection`
- **Indexes**: Performance indexes for action traces and effectiveness analysis

## Performance Expectations

### Response Time Targets

- **Database Connectivity**: < 10 seconds initial connection
- **Action Storage**: < 1 second per action
- **Oscillation Detection**: < 30 seconds for 50 actions
- **MCP Tool Calls**: < 20 seconds for complex queries
- **End-to-End Workflow**: < 35 seconds (SLM + Execution + Storage)

### Scalability Testing

The tests include performance scenarios with larger datasets:

- **Action History**: Up to 100 actions per resource
- **Concurrent Storage**: 10 simultaneous action writes
- **Pattern Detection**: Performance testing with 50+ actions
- **MCP Performance**: Large dataset queries (100+ actions)

## Troubleshooting

### Common Issues

1. **Database Connection Failures**
   ```bash
   # Check if PostgreSQL container is running
   podman ps | grep prometheus-alerts-slm-postgres
   
   # Check container logs
   podman logs prometheus-alerts-slm-postgres
   
   # Restart database
   ./scripts/deploy-postgres.sh
   ```

2. **Migration Failures**
   ```bash
   # Check migration files exist
   ls -la migrations/
   
   # Manual migration execution
   podman exec -it prometheus-alerts-slm-postgres psql -U slm_user -d action_history -f /path/to/migration.sql
   ```

3. **Test Timeouts**
   ```bash
   # Increase test timeout
   export TEST_TIMEOUT="300s"
   
   # Skip slow tests
   export SKIP_SLOW_TESTS="true"
   ```

4. **Ollama Connection Issues**
   ```bash
   # Check Ollama server
   curl http://localhost:11434/api/tags
   
   # Use different endpoint
   export OLLAMA_ENDPOINT="http://your-ollama-server:11434"
   ```

### Debugging

Enable debug logging for detailed information:

```bash
export LOG_LEVEL="debug"
./scripts/run-database-integration-tests.sh
```

Check test output for:
- Database connection details
- SQL query execution
- Action storage details
- Oscillation detection results
- MCP tool response content

## Integration with CI/CD

### GitHub Actions Integration

```yaml
name: Database Integration Tests
on: [push, pull_request]

jobs:
  database-integration:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: action_history
          POSTGRES_USER: slm_user
          POSTGRES_PASSWORD: slm_password_dev
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    
    - name: Run Database Integration Tests
      env:
        DB_HOST: localhost
        DB_PORT: 5432
        DB_NAME: action_history
        DB_USER: slm_user
        DB_PASSWORD: slm_password_dev
        SKIP_SLOW_TESTS: true
      run: |
        go test -v -tags=integration ./test/integration/... -timeout=30m
```

### Local Development

```bash
# Quick development cycle
make test-db-integration

# Or using the script
./scripts/run-database-integration-tests.sh contexts --skip-slow
```

## Contributing

When adding new database integration tests:

1. **Follow the existing context structure** - add tests to appropriate context files
2. **Use the DatabaseTestUtils** - leverage shared database utilities
3. **Include performance measurements** - use `s.monitor.RecordMeasurement()`
4. **Test both success and failure scenarios** - verify error handling
5. **Document any new test patterns** - update this documentation
6. **Verify cleanup** - ensure tests clean up database state

### Test Naming Conventions

- **Test functions**: `TestContextName` (e.g., `TestDatabaseIntegration`)
- **Sub-tests**: `Context_Specific_Function` (e.g., `Database_Connectivity`)
- **Test data**: Use descriptive resource names (e.g., `test-mcp-history-app`)

This comprehensive integration test suite ensures that the database functionality works correctly in realistic scenarios and provides confidence in the production deployment of the prometheus-alerts-slm system.