#!/usr/bin/env bash
# ============================================================================
# V1.0 Service Maturity Validation Script
# ============================================================================
# Purpose: Dynamically validates all services meet V1.0 maturity requirements
# Usage: ./scripts/validate-service-maturity.sh [--ci]
#
# Features:
# - Dynamically discovers services from cmd/ directory
# - Checks for required maturity features per service type
# - Generates report in docs/reports/
# - Compatible with Bash 3.x (macOS) and Bash 4+ (Linux)
#
# CI Enforcement Strategy:
# - Feature branches: No enforcement (allows iterative development)
# - PRs to main: Enforced via GitHub Actions (see .github/workflows/service-maturity-validation.yml)
# - This allows new services to be developed incrementally
# - Maturity validation only blocks merge to main, not commits
#
# Reference: docs/development/business-requirements/TESTING_GUIDELINES.md
# ============================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CI_MODE="${1:-}"
REPORT_DIR="docs/reports"
REPORT_FILE="${REPORT_DIR}/maturity-status.md"

# Service lists (Bash 3.x compatible - no associative arrays)
CRD_CONTROLLERS=""
STATELESS_SERVICES=""
PYTHON_SERVICES=""

# Services to exclude from validation (not in V1.x scope)
EXCLUDED_SERVICES="dynamictoolset"

# ============================================================================
# Service Discovery
# ============================================================================

discover_services() {
    echo -e "${BLUE}Discovering services...${NC}"

    # Discover Go services from cmd/
    for service_dir in cmd/*/; do
        service=$(basename "$service_dir")

        # Skip non-service directories
        [ "$service" = "README.md" ] && continue
        [ ! -f "${service_dir}main.go" ] && continue

        # Skip excluded services
        if echo "$EXCLUDED_SERVICES" | grep -qw "$service"; then
            echo "  Skipped: ${service} (not in V1.x scope)"
            continue
        fi

        # Detect service type
        if [ -d "internal/controller/${service}" ]; then
            CRD_CONTROLLERS="${CRD_CONTROLLERS} ${service}"
            echo "  Found: ${service} (crd-controller)"
        elif grep -q "Reconcile" "cmd/${service}/main.go" 2>/dev/null; then
            CRD_CONTROLLERS="${CRD_CONTROLLERS} ${service}"
            echo "  Found: ${service} (crd-controller)"
        else
            STATELESS_SERVICES="${STATELESS_SERVICES} ${service}"
            echo "  Found: ${service} (stateless-go)"
        fi
    done

    # Discover Python services (holmesgpt-api)
    # Main entry point is at holmesgpt-api/src/main.py
    if [ -d "holmesgpt-api" ] && [ -f "holmesgpt-api/src/main.py" ]; then
        PYTHON_SERVICES="${PYTHON_SERVICES} holmesgpt-api"
        echo "  Found: holmesgpt-api (stateless-python)"
    fi

    echo ""
}

# ============================================================================
# CRD Controller Checks
# ============================================================================

check_crd_metrics_wired() {
    local service=$1
    local controller_path="internal/controller/${service}"

    # Check for Metrics field in reconciler struct
    # DD-METRICS-001 supports two patterns:
    # 1. Pointer to concrete type: `Metrics *metrics.Metrics`
    # 2. Interface type: `Metrics notificationmetrics.Recorder`
    if [ -d "$controller_path" ]; then
        # Pattern 1: Pointer to metrics struct (e.g., *metrics.Metrics)
        if grep -r "Metrics.*\*metrics\." "$controller_path" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
        # Pattern 2: Interface from metrics package (e.g., notificationmetrics.Recorder)
        if grep -r "Metrics.*metrics\.Recorder\|Metrics.*metrics\.Interface" "$controller_path" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Check in pkg/ as fallback
    if grep -r "Metrics.*\*metrics\.\|Metrics.*metrics\.Recorder\|Metrics.*metrics\.Interface" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
        return 0
    fi

    return 1
}

check_crd_metrics_registered() {
    local service=$1

    # Check for metrics.Registry.MustRegister in init()
    if [ -d "internal/controller/${service}" ]; then
        if grep -r "metrics\.Registry\.MustRegister\|MustRegister" "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Check in pkg/
    if [ -d "pkg/${service}/metrics" ]; then
        if grep -r "metrics\.Registry\.MustRegister\|MustRegister" "pkg/${service}/metrics" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_crd_metrics_test_registry() {
    local service=$1

    # DD-METRICS-001: Check for NewMetricsWithRegistry() function
    # This enables test isolation by using custom registries
    if [ -d "pkg/${service}/metrics" ]; then
        if grep -r "func NewMetricsWithRegistry" "pkg/${service}/metrics" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Some services may have metrics in internal/
    if [ -d "internal/controller/${service}/metrics" ]; then
        if grep -r "func NewMetricsWithRegistry" "internal/controller/${service}/metrics" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_crd_event_recorder() {
    local service=$1

    # Check for EventRecorder field
    if [ -d "internal/controller/${service}" ]; then
        if grep -r "Recorder.*record\.EventRecorder" "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    if [ -d "pkg/${service}" ]; then
        if grep -r "Recorder.*record\.EventRecorder" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_crd_predicates() {
    local service=$1

    # Check for predicate usage
    if [ -d "internal/controller/${service}" ]; then
        if grep -r "predicate\." "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    if [ -d "pkg/${service}" ]; then
        if grep -r "predicate\." "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

# ============================================================================
# Controller Refactoring Pattern Library Checks
# Per: docs/architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md
# ============================================================================

# Pattern 1: Phase State Machine (P0)
check_pattern_phase_state_machine() {
    local service=$1

    # Check for phase package with types.go and manager.go
    if [ -f "pkg/${service}/phase/types.go" ] && [ -f "pkg/${service}/phase/manager.go" ]; then
        # Verify it has ValidTransitions map
        if grep -q "ValidTransitions" "pkg/${service}/phase/types.go" 2>/dev/null; then
            return 0
        fi
    fi

    return 1
}

# Pattern 2: Terminal State Logic (P1)
check_pattern_terminal_state_logic() {
    local service=$1

    # Check for IsTerminal() function in phase package
    if [ -f "pkg/${service}/phase/types.go" ]; then
        if grep -q "func IsTerminal" "pkg/${service}/phase/types.go" 2>/dev/null; then
            return 0
        fi
    fi

    return 1
}

# Pattern 3: Creator/Orchestrator (P0)
check_pattern_creator_orchestrator() {
    local service=$1

    # Check for creator/, delivery/, or execution/ packages
    if [ -d "pkg/${service}/creator" ] || [ -d "pkg/${service}/delivery" ] || [ -d "pkg/${service}/execution" ]; then
        return 0
    fi

    return 1
}

# Pattern 4: Status Manager (P1)
check_pattern_status_manager() {
    local service=$1

    # Check if status manager exists AND is used in controller
    if [ -f "pkg/${service}/status/manager.go" ]; then
        # Verify it's actually used in controller (not just exists)
        if [ -d "internal/controller/${service}" ]; then
            if grep -r "status\.Manager\|statusManager" "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
                return 0
            fi
        fi
    fi

    return 1
}

# Pattern 5: Controller Decomposition (P2)
check_pattern_controller_decomposition() {
    local service=$1

    # Check for multiple files in controller directory (excluding suite_test.go and main controller)
    # Standard pattern: internal/controller/{service}/ with multiple handler files
    # RO example: blocking.go, consecutive_failure.go, notification_handler.go, notification_tracking.go
    # NT/SP/WE example: phase_handlers.go, delivery_handlers.go, etc.

    if [ ! -d "internal/controller/${service}" ]; then
        return 1
    fi

    # Count non-test .go files (excluding main controller file)
    local go_files
    go_files=$(find "internal/controller/${service}" -maxdepth 1 -name "*.go" ! -name "*_test.go" ! -name "suite_test.go" ! -name "reconciler.go" ! -name "${service}_controller.go" ! -name "${service}request_controller.go" 2>/dev/null | wc -l)

    # Decomposed if 2+ additional files beyond main controller
    if [ "$go_files" -ge 2 ]; then
        return 0
    fi

    return 1
}

# Pattern 6: Interface-Based Services (P2)
check_pattern_interface_based_services() {
    local service=$1

    # EXCEPTION: RO uses Sequential Orchestration pattern, not Interface-Based Services
    # Rationale: RO orchestrates child CRDs in a fixed sequence with data dependencies
    # (SP → AI → WE), where each creator has unique signatures and parameters.
    # Interface-Based Services is designed for independent, pluggable services with
    # common interfaces (like SignalProcessing's delivery channels).
    # See: docs/handoff/RO_INTERFACE_BASED_SERVICES_PATTERN_TRIAGE_DEC_28_2025.md
    if [ "$service" = "remediationorchestrator" ]; then
        return 2  # Special return code for "N/A" (pattern not applicable)
    fi

    # Check for service interfaces (DeliveryService, ExecutionService, etc.)
    # and map-based registry pattern in controller
    if [ -d "pkg/${service}" ]; then
        # Look for interface definitions
        if grep -r "type.*Service.*interface" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            # Look for map-based registry in controller
            if [ -d "internal/controller/${service}" ]; then
                if grep -r "map\[.*\].*Service\|Services.*map" "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
                    return 0
                fi
            fi
        fi
    fi

    return 1
}

# Pattern 7: Audit Manager (P3)
check_pattern_audit_manager() {
    local service=$1

    # Check for audit manager package (P3: Audit Manager)
    # Per CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7
    # All CRD controllers now use manager.go naming convention
    if [ -f "pkg/${service}/audit/manager.go" ]; then
        return 0
    fi

    return 1
}

# Check if Creator/Orchestrator pattern is applicable to this service
is_creator_orchestrator_applicable() {
    local service=$1

    # Only applicable to services that create child CRDs or orchestrate external delivery
    case "$service" in
        remediationorchestrator)
            # RO creates SignalProcessing, AIAnalysis, WorkflowExecution CRDs
            return 0
            ;;
        notification)
            # NT orchestrates delivery to multiple external channels (Slack, Email, Webhook, PagerDuty)
            return 0
            ;;
        *)
            # All other services (SP, WE, AIA) don't create child CRDs or orchestrate external delivery
            return 1
            ;;
    esac
}

# Check if Interface-Based Services pattern is applicable to this service
is_interface_based_services_applicable() {
    local service=$1

    # RO uses Sequential Orchestration, not Interface-Based Services
    # Rationale: RO orchestrates child CRDs in fixed sequence (SP → AI → WE) with data
    # dependencies. Interface-Based Services is for independent, pluggable services
    # (like Notification's delivery channels: Slack, Email, Console, etc.)
    if [ "$service" = "remediationorchestrator" ]; then
        return 1  # Not applicable
    fi

    # AIAnalysis uses Phase Handlers, not Interface-Based Services
    # Rationale: AIAnalysis has a single linear flow through phases (Pending → Investigating
    # → Analyzing → Completed/Failed). Each phase is handled by a dedicated handler, but
    # there's no need for pluggable service interfaces. Interface-Based Services is for
    # services that orchestrate multiple independent, swappable implementations of a common
    # interface (like Notification's delivery channels: Slack, Email, Console, etc.)
    if [ "$service" = "aianalysis" ]; then
        return 1  # Not applicable
    fi

    # WorkflowExecution is a pure executor with single execution backend (Tekton)
    # Rationale: WorkflowExecution has a single, fixed execution path (Tekton PipelineRuns only).
    # Interface-Based Services is for services that orchestrate multiple independent, pluggable
    # implementations (like Notification's delivery channels or SignalProcessing's delivery services).
    # WE does not orchestrate multiple backends or services - it has one job: execute Tekton workflows.
    if [ "$service" = "workflowexecution" ]; then
        return 1  # Not applicable
    fi

    return 0  # Applicable to all other services
}

# Get total applicable patterns for a service
get_total_applicable_patterns() {
    local service=$1
    local total=5  # Base patterns always applicable (Phase SM, Terminal, Status Mgr, Decomp, Audit Mgr)

    # Add Creator/Orchestrator if applicable (RO, NT only)
    is_creator_orchestrator_applicable "$service" && total=$((total + 1))

    # Add Interface-Based Services if applicable (all except RO)
    is_interface_based_services_applicable "$service" && total=$((total + 1))

    echo "$total"
}

# Check all patterns and return count of adopted patterns
count_adopted_patterns() {
    local service=$1
    local count=0

    check_pattern_phase_state_machine "$service" && count=$((count + 1))
    check_pattern_terminal_state_logic "$service" && count=$((count + 1))

    # Only check Creator/Orchestrator if applicable
    if is_creator_orchestrator_applicable "$service"; then
        check_pattern_creator_orchestrator "$service" && count=$((count + 1))
    fi

    check_pattern_status_manager "$service" && count=$((count + 1))
    check_pattern_controller_decomposition "$service" && count=$((count + 1))

    # Only check Interface-Based Services if applicable
    if is_interface_based_services_applicable "$service"; then
        check_pattern_interface_based_services "$service" && count=$((count + 1))
    fi

    check_pattern_audit_manager "$service" && count=$((count + 1))

    echo "$count"
}

# ============================================================================
# Common Checks (Both Service Types)
# ============================================================================

check_graceful_shutdown() {
    local service=$1

    # Check for signal handling or Close() calls
    if grep -r "signal\|Close()\|Shutdown\|SIGTERM" "cmd/${service}/main.go" >/dev/null 2>&1; then
        return 0
    fi

    return 1
}

check_healthz_probes() {
    local service=$1

    # Check for healthz/readyz
    if grep -r "healthz\|readyz\|HealthProbe\|AddHealthzCheck" "cmd/${service}/main.go" >/dev/null 2>&1; then
        return 0
    fi

    return 1
}

check_audit_integration() {
    local service=$1

    # DataStorage IS the audit service - automatically passes
    if [ "$service" = "datastorage" ]; then
        return 0
    fi

    # Check for audit usage in cmd/
    if grep -r "audit\.\|AuditStore\|AuditClient" "cmd/${service}/main.go" >/dev/null 2>&1; then
        return 0
    fi

    # Check for audit usage in internal/controller/ (CRD controllers)
    if [ -d "internal/controller/${service}" ]; then
        if grep -r "audit\.\|AuditStore\|AuditClient" "internal/controller/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Check for audit usage in pkg/ (stateless services)
    if [ -d "pkg/${service}" ]; then
        if grep -r "audit\.\|AuditStore\|AuditClient" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

# ============================================================================
# Stateless Service Checks
# ============================================================================

check_stateless_metrics() {
    local service=$1

    # Check for prometheus/metrics usage
    if [ -d "pkg/${service}" ]; then
        if grep -r "prometheus\|/metrics\|Metrics" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_stateless_metrics_test_registry() {
    local service=$1

    # Check for NewMetricsWithRegistry() function (test isolation support)
    # Stateless services should also support test-specific registries
    if [ -d "pkg/${service}/metrics" ]; then
        if grep -r "func NewMetricsWithRegistry" "pkg/${service}/metrics" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Some services may have metrics elsewhere
    if [ -d "pkg/${service}" ]; then
        if grep -r "func NewMetricsWithRegistry" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_stateless_health() {
    local service=$1

    # Check for health endpoint
    if [ -d "pkg/${service}" ]; then
        if grep -r "/health\|/healthz\|HealthCheck" "pkg/${service}" --include="*.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

# ============================================================================
# Python Service Checks (holmesgpt-api)
# ============================================================================

check_python_metrics() {
    local service=$1

    # Check for Prometheus metrics in Python service
    if [ -d "$service" ]; then
        if grep -r "prometheus_client\|/metrics\|Counter\|Histogram\|Gauge" "$service" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_health() {
    local service=$1

    # Check for health endpoint in Python service
    if [ -d "$service" ]; then
        if grep -r "/health\|/healthz\|health_check\|@app.get.*health" "$service" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_graceful_shutdown() {
    local service=$1

    # Check for graceful shutdown in Python service
    if [ -d "$service" ]; then
        if grep -r "signal\|SIGTERM\|shutdown\|atexit\|on_shutdown" "$service" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_audit() {
    local service=$1

    # Check for audit integration in Python service
    if [ -d "$service" ]; then
        if grep -r "audit\|AuditClient\|data_storage" "$service" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_openapi_client() {
    local service=$1

    # Check for OpenAPI client usage in Python tests (MANDATORY per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
    # Python services use generated clients from src/clients/datastorage/ or tests/clients/
    if [ -d "${service}/tests" ]; then
        # Look for OpenAPI client imports (datastorage, api_client, ApiClient)
        if grep -r "from.*clients.*import\|ApiClient\|datastorage\.api\|datastorage\.models\|from.*datastorage.*import" "${service}/tests" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_audit_validator() {
    local service=$1

    # Check for structured audit validation in Python tests
    # Python equivalent of testutil.ValidateAuditEvent would be structured assertion patterns
    if [ -d "${service}/tests" ]; then
        # Look for structured audit event validation patterns
        if grep -r "AuditEvent\|audit_event\.\|assert.*event_type\|assert.*correlation_id\|\.event_category\|\.event_type\|\.event_action" "${service}/tests" --include="*.py" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_python_raw_http() {
    local service=$1

    # Check if Python tests use raw HTTP (anti-pattern - should use OpenAPI client)
    if [ -d "${service}/tests" ]; then
        # Look for requests.get/post patterns that query audit events directly
        if grep -r "requests\.get.*audit\|requests\.post.*audit\|httpx\.get.*audit\|httpx\.post.*audit" "${service}/tests" --include="*.py" >/dev/null 2>&1; then
            return 0  # Found raw HTTP - bad
        fi
    fi

    return 1  # No raw HTTP found - good
}

# ============================================================================
# Test Coverage Checks
# ============================================================================

check_metrics_integration_tests() {
    local service=$1

    # Check for metrics tests in integration
    if [ -f "test/integration/${service}/metrics_test.go" ]; then
        return 0
    fi

    if [ -d "test/integration/${service}" ]; then
        if grep -r "metrics\|Metrics" "test/integration/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_metrics_e2e_tests() {
    local service=$1

    # Check for metrics E2E tests
    if [ -f "test/e2e/${service}/metrics_test.go" ]; then
        return 0
    fi

    if [ -d "test/e2e/${service}" ]; then
        if grep -r "/metrics\|metricsURL" "test/e2e/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_audit_tests() {
    local service=$1

    # Check for audit tests
    if [ -d "test/integration/${service}" ]; then
        if grep -r "audit\|Audit" "test/integration/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

# ============================================================================
# V1.0 Mandatory Pattern Checks (OpenAPI Client + testutil.ValidateAuditEvent)
# ============================================================================

check_audit_openapi_client() {
    local service=$1

    # Check for OpenAPI client usage in audit tests (MANDATORY per V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
    # Must use dsgen.APIClient or dsgen.NewConfiguration, NOT raw http.Get
    # SPECIAL CASE: DataStorage uses dsclient. (its own generated OpenAPI client)
    if [ -d "test/integration/${service}" ]; then
        if grep -r "dsgen\.\|dsgen\.APIClient\|dsgen\.NewConfiguration\|dsclient\." "test/integration/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Also check E2E tests
    if [ -d "test/e2e/${service}" ]; then
        if grep -r "dsgen\.\|dsgen\.APIClient\|dsgen\.NewConfiguration\|dsclient\." "test/e2e/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_audit_testutil_validator() {
    local service=$1

    # Check for testutil.ValidateAuditEvent usage (P0 - MANDATORY per SERVICE_MATURITY_REQUIREMENTS.md v1.2.0)
    if [ -d "test/integration/${service}" ]; then
        if grep -r "testutil\.ValidateAuditEvent\|testutil\.ExpectedAuditEvent" "test/integration/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    # Also check E2E tests
    if [ -d "test/e2e/${service}" ]; then
        if grep -r "testutil\.ValidateAuditEvent\|testutil\.ExpectedAuditEvent" "test/e2e/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0
        fi
    fi

    return 1
}

check_audit_raw_http() {
    local service=$1

    # Check if tests use raw HTTP (anti-pattern - should use OpenAPI client)
    # Returns 0 if raw HTTP is found (bad), 1 if not found (good)

    # EXCEPTION: DataStorage graceful_shutdown_test.go tests HTTP connection behavior
    # Rationale: Connection draining and in-flight request testing requires raw HTTP
    # See: docs/handoff/DS_MATURITY_VALIDATION_ISSUES_TRIAGE_DEC_20_2025.md
    if [ "$service" = "datastorage" ]; then
        # Check for raw HTTP OUTSIDE of graceful shutdown tests
        if [ -d "test/integration/${service}" ]; then
            if grep -r "http\.Get.*audit\|http\.Get.*api/v1/audit" \
               "test/integration/${service}" \
               --include="*_test.go" \
               --exclude="graceful_shutdown_test.go" >/dev/null 2>&1; then
                return 0  # Found inappropriate raw HTTP - bad
            fi
        fi
        return 1  # Only graceful shutdown uses raw HTTP (acceptable)
    fi

    # Standard check for other services (no exceptions)
    if [ -d "test/integration/${service}" ]; then
        # Look for http.Get patterns that query audit events
        if grep -r "http\.Get.*audit\|http\.Get.*api/v1/audit" "test/integration/${service}" --include="*_test.go" >/dev/null 2>&1; then
            return 0  # Found raw HTTP - bad
        fi
    fi

    return 1  # No raw HTTP found - good
}

# ============================================================================
# Reporting
# ============================================================================

generate_report() {
    mkdir -p "$REPORT_DIR"

    cat > "$REPORT_FILE" << 'EOF'
# V1.0 Service Maturity Status

> **Auto-generated**: This report is generated by `scripts/validate-service-maturity.sh`

## Legend

| Icon | Meaning |
|------|---------|
| ✅ | Requirement met |
| ❌ | Requirement NOT met |
| ⬜ | Not applicable |

---

## CRD Controllers

| Service | Metrics Wired | Metrics Registered | EventRecorder | Predicates | Graceful Shutdown | Healthz | Audit |
|---------|---------------|--------------------|---------------|------------|-------------------|---------|-------|
EOF

    # Process CRD Controllers
    for service in $CRD_CONTROLLERS; do
        metrics_wired=$(check_crd_metrics_wired "$service" && echo "✅" || echo "❌")
        metrics_reg=$(check_crd_metrics_registered "$service" && echo "✅" || echo "❌")
        event_rec=$(check_crd_event_recorder "$service" && echo "✅" || echo "❌")
        predicates=$(check_crd_predicates "$service" && echo "✅" || echo "❌")
        shutdown=$(check_graceful_shutdown "$service" && echo "✅" || echo "❌")
        healthz=$(check_healthz_probes "$service" && echo "✅" || echo "❌")
        audit=$(check_audit_integration "$service" && echo "✅" || echo "❌")

        echo "| ${service} | ${metrics_wired} | ${metrics_reg} | ${event_rec} | ${predicates} | ${shutdown} | ${healthz} | ${audit} |" >> "$REPORT_FILE"
    done

    cat >> "$REPORT_FILE" << 'EOF'

---

## Stateless HTTP Services

| Service | Prometheus Metrics | Health Endpoint | Graceful Shutdown | Audit |
|---------|--------------------|-----------------|--------------------|-------|
EOF

    # Process Stateless Go Services
    for service in $STATELESS_SERVICES; do
        metrics=$(check_stateless_metrics "$service" && echo "✅" || echo "❌")
        health=$(check_stateless_health "$service" && echo "✅" || echo "❌")
        shutdown=$(check_graceful_shutdown "$service" && echo "✅" || echo "❌")
        audit=$(check_audit_integration "$service" && echo "✅" || echo "❌")

        echo "| ${service} | ${metrics} | ${health} | ${shutdown} | ${audit} |" >> "$REPORT_FILE"
    done

    # Process Python Services
    for service in $PYTHON_SERVICES; do
        metrics=$(check_python_metrics "$service" && echo "✅" || echo "❌")
        health=$(check_python_health "$service" && echo "✅" || echo "❌")
        shutdown=$(check_python_graceful_shutdown "$service" && echo "✅" || echo "❌")
        audit=$(check_python_audit "$service" && echo "✅" || echo "❌")

        echo "| ${service} (Python) | ${metrics} | ${health} | ${shutdown} | ${audit} |" >> "$REPORT_FILE"
    done

    # Controller Refactoring Pattern Library Compliance (CRD Controllers only)
    cat >> "$REPORT_FILE" << 'EOF'

---

## Controller Refactoring Pattern Library Compliance

> Per `CONTROLLER_REFACTORING_PATTERN_LIBRARY.md`: Production-proven patterns from RemediationOrchestrator service

| Service | P0: Phase SM | P1: Terminal | P0: Creator | P1: Status Mgr | P2: Decomp | P2: Interfaces | P3: Audit Mgr | Total |
|---------|--------------|--------------|-------------|----------------|------------|----------------|---------------|-------|
EOF

    for service in $CRD_CONTROLLERS; do
        p1=$(check_pattern_phase_state_machine "$service" && echo "✅" || echo "❌")
        p2=$(check_pattern_terminal_state_logic "$service" && echo "✅" || echo "❌")

        # Creator/Orchestrator - only applicable to RO and NT
        if is_creator_orchestrator_applicable "$service"; then
            p3=$(check_pattern_creator_orchestrator "$service" && echo "✅" || echo "❌")
        else
            p3="N/A"
        fi

        p4=$(check_pattern_status_manager "$service" && echo "✅" || echo "❌")
        p5=$(check_pattern_controller_decomposition "$service" && echo "✅" || echo "❌")

        # Interface-Based Services - only applicable to services other than RO
        if is_interface_based_services_applicable "$service"; then
            p6=$(check_pattern_interface_based_services "$service" && echo "✅" || echo "❌")
        else
            p6="N/A"
        fi

        p7=$(check_pattern_audit_manager "$service" && echo "✅" || echo "❌")
        total=$(count_adopted_patterns "$service")
        max_patterns=$(get_total_applicable_patterns "$service")

        echo "| ${service} | ${p1} | ${p2} | ${p3} | ${p4} | ${p5} | ${p6} | ${p7} | ${total}/${max_patterns} |" >> "$REPORT_FILE"
    done

    cat >> "$REPORT_FILE" << 'EOF'

**Legend**:
- **Phase SM**: Phase State Machine with ValidTransitions map
- **Terminal**: IsTerminal() function for terminal state checks
- **Creator**: Creator/Orchestrator/Delivery/Execution package extraction (only applicable to RO/NT)
- **Status Mgr**: Status Manager adopted (not just existing)
- **Decomp**: Controller decomposed into handler files
- **Interfaces**: Interface-based service registry pattern
- **Audit Mgr**: Audit Manager package

**Pattern Applicability**:
- **Creator/Orchestrator**: Only applicable to services that create child CRDs (RO) or orchestrate external delivery (NT)
- Services without child CRD creation or external orchestration show **N/A** for this pattern
- Total patterns: 7 for RO/NT, 6 for all other services

**Priority Guide**:
- **P0**: Critical for maintainability (Phase SM, Creator*)
- **P1**: Quick wins with high ROI (Terminal, Status Mgr)
- **P2**: Significant improvements (Decomp, Interfaces)
- **P3**: Polish and consistency (Audit Mgr)

*Creator only P0 for services where applicable (RO, NT)

**Reference Implementation**: RemediationOrchestrator (6/7 patterns)

---

## Test Coverage

| Service | Type | Metrics Integration | Metrics E2E | Audit Tests |
|---------|------|---------------------|-------------|-------------|
EOF

    for service in $CRD_CONTROLLERS; do
        metrics_int=$(check_metrics_integration_tests "$service" && echo "✅" || echo "❌")
        metrics_e2e=$(check_metrics_e2e_tests "$service" && echo "✅" || echo "❌")
        audit_tests=$(check_audit_tests "$service" && echo "✅" || echo "❌")
        echo "| ${service} | CRD | ${metrics_int} | ${metrics_e2e} | ${audit_tests} |" >> "$REPORT_FILE"
    done

    for service in $STATELESS_SERVICES; do
        metrics_int=$(check_metrics_integration_tests "$service" && echo "✅" || echo "❌")
        metrics_e2e=$(check_metrics_e2e_tests "$service" && echo "✅" || echo "❌")
        audit_tests=$(check_audit_tests "$service" && echo "✅" || echo "❌")
        echo "| ${service} | HTTP-Go | ${metrics_int} | ${metrics_e2e} | ${audit_tests} |" >> "$REPORT_FILE"
    done

    for service in $PYTHON_SERVICES; do
        # Python service test checks
        if [ -d "${service}/tests" ]; then
            metrics_int=$(grep -r "metrics\|prometheus" "${service}/tests" --include="*.py" >/dev/null 2>&1 && echo "✅" || echo "❌")
            audit_tests=$(grep -r "audit\|data_storage" "${service}/tests" --include="*.py" >/dev/null 2>&1 && echo "✅" || echo "❌")
        else
            metrics_int="❌"
            audit_tests="❌"
        fi
        # E2E tests for Python services
        metrics_e2e=$([ -d "test/e2e/${service}" ] && echo "✅" || echo "❌")
        echo "| ${service} | HTTP-Python | ${metrics_int} | ${metrics_e2e} | ${audit_tests} |" >> "$REPORT_FILE"
    done

    # V1.0 Mandatory Testing Patterns
    cat >> "$REPORT_FILE" << 'EOF'

---

## V1.0 P0 Mandatory Testing Patterns

> Per `SERVICE_MATURITY_REQUIREMENTS.md v1.2.0`: Audit tests MUST use OpenAPI client and testutil.ValidateAuditEvent (P0 - BLOCKERS)

| Service | Type | OpenAPI Client | testutil.ValidateAuditEvent | Uses Raw HTTP (Bad) |
|---------|------|----------------|------------------------------|---------------------|
EOF

    for service in $CRD_CONTROLLERS; do
        openapi=$(check_audit_openapi_client "$service" && echo "✅" || echo "❌")
        validator=$(check_audit_testutil_validator "$service" && echo "✅" || echo "❌")
        raw_http=$(check_audit_raw_http "$service" && echo "⚠️ YES" || echo "✅ No")
        echo "| ${service} | CRD | ${openapi} | ${validator} | ${raw_http} |" >> "$REPORT_FILE"
    done

    for service in $STATELESS_SERVICES; do
        openapi=$(check_audit_openapi_client "$service" && echo "✅" || echo "❌")
        validator=$(check_audit_testutil_validator "$service" && echo "✅" || echo "❌")
        raw_http=$(check_audit_raw_http "$service" && echo "⚠️ YES" || echo "✅ No")
        echo "| ${service} | HTTP-Go | ${openapi} | ${validator} | ${raw_http} |" >> "$REPORT_FILE"
    done

    for service in $PYTHON_SERVICES; do
        openapi=$(check_python_openapi_client "$service" && echo "✅" || echo "❌")
        validator=$(check_python_audit_validator "$service" && echo "✅" || echo "❌")
        raw_http=$(check_python_raw_http "$service" && echo "⚠️ YES" || echo "✅ No")
        echo "| ${service} | Python | ${openapi} | ${validator} | ${raw_http} |" >> "$REPORT_FILE"
    done

    cat >> "$REPORT_FILE" << 'EOF'

---

## References

- [TESTING_GUIDELINES.md](../development/business-requirements/TESTING_GUIDELINES.md)
- [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md)
- [DD-METRICS-001: Controller Metrics Wiring Pattern](../architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md)
- [DD-005: Observability Standards](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [CONTROLLER_REFACTORING_PATTERN_LIBRARY.md](../architecture/patterns/CONTROLLER_REFACTORING_PATTERN_LIBRARY.md)
EOF

    echo -e "${GREEN}Report generated: ${REPORT_FILE}${NC}"
}

# ============================================================================
# Validation
# ============================================================================

validate_all() {
    local failed=0

    echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║         V1.0 SERVICE MATURITY VALIDATION                     ║${NC}"
    echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    # Validate CRD Controllers
    for service in $CRD_CONTROLLERS; do
        local service_failed=0
        local P0_VIOLATIONS=0
        echo -e "${YELLOW}Checking: ${service} (crd-controller)${NC}"

        # P0 checks for CRD controllers
        if ! check_crd_metrics_wired "$service"; then
            echo -e "  ${RED}❌ Metrics not wired to controller${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Metrics wired${NC}"
        fi

        if ! check_crd_metrics_registered "$service"; then
            echo -e "  ${RED}❌ Metrics not registered with controller-runtime${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Metrics registered${NC}"
        fi

        # DD-METRICS-001: Check for test isolation support
        if ! check_crd_metrics_test_registry "$service"; then
            echo -e "  ${YELLOW}⚠️  No NewMetricsWithRegistry() for test isolation (DD-METRICS-001) (P1)${NC}"
        else
            echo -e "  ${GREEN}✅ Metrics test isolation (NewMetricsWithRegistry)${NC}"
        fi

        if ! check_crd_event_recorder "$service"; then
            echo -e "  ${YELLOW}⚠️  No EventRecorder (P1)${NC}"
        else
            echo -e "  ${GREEN}✅ EventRecorder present${NC}"
        fi

        # Common checks
        if ! check_graceful_shutdown "$service"; then
            echo -e "  ${RED}❌ Graceful shutdown not implemented${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Graceful shutdown${NC}"
        fi

        if ! check_audit_integration "$service"; then
            echo -e "  ${RED}❌ Audit integration not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Audit integration${NC}"
        fi

        # V1.0 Mandatory Testing Patterns (P1 checks)
        if check_audit_tests "$service"; then
            if ! check_audit_openapi_client "$service"; then
                echo -e "  ${YELLOW}⚠️  Audit tests don't use OpenAPI client (P1)${NC}"
            else
                echo -e "  ${GREEN}✅ Audit uses OpenAPI client${NC}"
            fi

            if ! check_audit_testutil_validator "$service"; then
                echo -e "  ${RED}❌ Audit tests don't use testutil.ValidateAuditEvent (P0 - MANDATORY)${NC}"
                P0_VIOLATIONS=$((P0_VIOLATIONS + 1))
                service_failed=1
            else
                echo -e "  ${GREEN}✅ Audit uses testutil validator${NC}"
            fi

            if check_audit_raw_http "$service"; then
                echo -e "  ${YELLOW}⚠️  Audit tests use raw HTTP (refactor to OpenAPI) (P1)${NC}"
            fi
        fi

        # Controller Refactoring Pattern Library Checks (CRD Controllers only)
        echo -e "  ${BLUE}Controller Refactoring Patterns:${NC}"
        local pattern_count=0

        if check_pattern_phase_state_machine "$service"; then
            echo -e "    ${GREEN}✅ Phase State Machine (P0)${NC}"
            pattern_count=$((pattern_count + 1))
        else
            echo -e "    ${YELLOW}⚠️  Phase State Machine not adopted (P0 - recommended)${NC}"
        fi

        if check_pattern_terminal_state_logic "$service"; then
            echo -e "    ${GREEN}✅ Terminal State Logic (P1)${NC}"
            pattern_count=$((pattern_count + 1))
        else
            echo -e "    ${YELLOW}⚠️  Terminal State Logic not adopted (P1 - quick win)${NC}"
        fi

        # Creator/Orchestrator - only applicable to RO and NT
        if is_creator_orchestrator_applicable "$service"; then
            if check_pattern_creator_orchestrator "$service"; then
                echo -e "    ${GREEN}✅ Creator/Orchestrator Pattern (P0)${NC}"
                pattern_count=$((pattern_count + 1))
            else
                echo -e "    ${YELLOW}⚠️  Creator/Orchestrator not adopted (P0 - recommended)${NC}"
            fi
        else
            echo -e "    ${BLUE}ℹ️  Creator/Orchestrator N/A (service doesn't create child CRDs or orchestrate delivery)${NC}"
        fi

        if check_pattern_status_manager "$service"; then
            echo -e "    ${GREEN}✅ Status Manager adopted (P1)${NC}"
            pattern_count=$((pattern_count + 1))
        else
            echo -e "    ${YELLOW}⚠️  Status Manager not adopted (P1 - quick win)${NC}"
        fi

        if check_pattern_controller_decomposition "$service"; then
            echo -e "    ${GREEN}✅ Controller Decomposition (P2)${NC}"
            pattern_count=$((pattern_count + 1))
        else
            echo -e "    ${YELLOW}⚠️  Controller not decomposed (P2)${NC}"
        fi

        if is_interface_based_services_applicable "$service"; then
            if check_pattern_interface_based_services "$service"; then
                echo -e "    ${GREEN}✅ Interface-Based Services (P2)${NC}"
                pattern_count=$((pattern_count + 1))
            else
                echo -e "    ${YELLOW}⚠️  Interface-Based Services not adopted (P2)${NC}"
            fi
        else
            echo -e "    ${BLUE}ℹ️  Interface-Based Services N/A (service uses Sequential Orchestration)${NC}"
        fi

        if check_pattern_audit_manager "$service"; then
            echo -e "    ${GREEN}✅ Audit Manager (P3)${NC}"
            pattern_count=$((pattern_count + 1))
        else
            echo -e "    ${YELLOW}⚠️  Audit Manager not adopted (P3)${NC}"
        fi

        max_patterns=$(get_total_applicable_patterns "$service")
        echo -e "  ${BLUE}Pattern Adoption: ${pattern_count}/${max_patterns} patterns${NC}"

        if [ $service_failed -eq 1 ]; then
            failed=1
        fi

        echo ""
    done

    # Validate Stateless Services
    for service in $STATELESS_SERVICES; do
        local service_failed=0
        local P0_VIOLATIONS=0
        echo -e "${YELLOW}Checking: ${service} (stateless)${NC}"

        if ! check_stateless_metrics "$service"; then
            echo -e "  ${RED}❌ Prometheus metrics not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Prometheus metrics${NC}"
        fi

        # Check for test isolation support (similar to CRD controllers)
        if ! check_stateless_metrics_test_registry "$service"; then
            echo -e "  ${YELLOW}⚠️  No NewMetricsWithRegistry() for test isolation (P1)${NC}"
        else
            echo -e "  ${GREEN}✅ Metrics test isolation (NewMetricsWithRegistry)${NC}"
        fi

        if ! check_stateless_health "$service"; then
            echo -e "  ${RED}❌ Health endpoint not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Health endpoint${NC}"
        fi

        if ! check_graceful_shutdown "$service"; then
            echo -e "  ${RED}❌ Graceful shutdown not implemented${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Graceful shutdown${NC}"
        fi

        if ! check_audit_integration "$service"; then
            echo -e "  ${RED}❌ Audit integration not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Audit integration${NC}"
        fi

        # V1.0 Mandatory Testing Patterns (P1 checks)
        if check_audit_tests "$service"; then
            if ! check_audit_openapi_client "$service"; then
                echo -e "  ${YELLOW}⚠️  Audit tests don't use OpenAPI client (P1)${NC}"
            else
                echo -e "  ${GREEN}✅ Audit uses OpenAPI client${NC}"
            fi

            if ! check_audit_testutil_validator "$service"; then
                echo -e "  ${RED}❌ Audit tests don't use testutil.ValidateAuditEvent (P0 - MANDATORY)${NC}"
                P0_VIOLATIONS=$((P0_VIOLATIONS + 1))
                service_failed=1
            else
                echo -e "  ${GREEN}✅ Audit uses testutil validator${NC}"
            fi

            if check_audit_raw_http "$service"; then
                echo -e "  ${YELLOW}⚠️  Audit tests use raw HTTP (refactor to OpenAPI) (P1)${NC}"
            fi
        fi

        if [ $service_failed -eq 1 ]; then
            failed=1
        fi

        echo ""
    done

    # Validate Python Services
    for service in $PYTHON_SERVICES; do
        local service_failed=0
        local P0_VIOLATIONS=0
        echo -e "${YELLOW}Checking: ${service} (stateless-python)${NC}"

        if ! check_python_metrics "$service"; then
            echo -e "  ${RED}❌ Prometheus metrics not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Prometheus metrics${NC}"
        fi

        if ! check_python_health "$service"; then
            echo -e "  ${RED}❌ Health endpoint not found${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Health endpoint${NC}"
        fi

        if ! check_python_graceful_shutdown "$service"; then
            echo -e "  ${RED}❌ Graceful shutdown not implemented${NC}"
            service_failed=1
        else
            echo -e "  ${GREEN}✅ Graceful shutdown${NC}"
        fi

        if ! check_python_audit "$service"; then
            echo -e "  ${YELLOW}⚠️  Audit integration not found (P1)${NC}"
        else
            echo -e "  ${GREEN}✅ Audit integration${NC}"
        fi

        # V1.0 Mandatory Testing Patterns for Python (P1 checks)
        if [ -d "${service}/tests" ]; then
            if ! check_python_openapi_client "$service"; then
                echo -e "  ${YELLOW}⚠️  Tests don't use OpenAPI client (P1)${NC}"
            else
                echo -e "  ${GREEN}✅ Tests use OpenAPI client${NC}"
            fi

            if ! check_python_audit_validator "$service"; then
                echo -e "  ${YELLOW}⚠️  Tests don't use structured audit validation (P1)${NC}"
            else
                echo -e "  ${GREEN}✅ Tests use structured audit validation${NC}"
            fi

            if check_python_raw_http "$service"; then
                echo -e "  ${YELLOW}⚠️  Tests use raw HTTP for audit (refactor to OpenAPI) (P1)${NC}"
            fi
        fi

        if [ $service_failed -eq 1 ]; then
            failed=1
        fi

        echo ""
    done

    return $failed
}

# ============================================================================
# Main
# ============================================================================

main() {
    discover_services

    # Check if any services were found
    if [ -z "$CRD_CONTROLLERS" ] && [ -z "$STATELESS_SERVICES" ] && [ -z "$PYTHON_SERVICES" ]; then
        echo -e "${RED}No services found${NC}"
        exit 1
    fi

    generate_report

    if [ "$CI_MODE" = "--ci" ]; then
        echo ""
        echo -e "${BLUE}Running CI validation...${NC}"

        if ! validate_all; then
            echo ""
            echo -e "${RED}╔══════════════════════════════════════════════════════════════╗${NC}"
            echo -e "${RED}║  VALIDATION FAILED: Some services missing P0 requirements    ║${NC}"
            echo -e "${RED}╚══════════════════════════════════════════════════════════════╝${NC}"
            echo ""
            echo "See: ${REPORT_FILE}"
            echo "Fix issues before merging to main branch."
            exit 1
        fi

        echo ""
        echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${GREEN}║  VALIDATION PASSED: All P0 requirements met                  ║${NC}"
        echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
    else
        validate_all || true
        echo ""
        echo "To run in CI mode (fails on P0 violations): $0 --ci"
    fi
}

main "$@"
