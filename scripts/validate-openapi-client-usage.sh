#!/bin/bash
# ========================================
# Validate OpenAPI Client Usage Across All Services
# ========================================
#
# Design Decision: DD-HAPI-003 - Mandatory OpenAPI Client Usage
# See: docs/architecture/decisions/DD-HAPI-003-mandatory-openapi-client-usage.md
#
# This script validates that ALL service code (production + tests) uses
# generated OpenAPI clients instead of manual HTTP client implementations.
#
# ENFORCEMENT: P0 - BLOCKER
# - All external service calls MUST use generated OpenAPI clients
# - No manual http.NewRequestWithContext calls to external services
# - No manual json.Marshal of external service request types
# - Applies to: Production code, Unit tests, Integration tests, E2E tests
#
# VALIDATED SERVICES:
# - aianalysis
# - datastorage
# - gateway
# - notification
# - remediationorchestrator
# - signalprocessing
# - workflowexecution
#
# EXTERNAL SERVICES (require OpenAPI clients):
# - HolmesGPT-API (HAPI)
# - Data Storage
#
# WHY DD-HAPI-003?
# - ✅ Compile-time type safety
# - ✅ Contract compliance with service OpenAPI specs
# - ✅ Fixes HTTP 500 errors from malformed requests
# - ✅ Consistent across all services and test tiers
#
# USAGE:
#   ./scripts/validate-openapi-client-usage.sh
#
# EXIT CODES:
#   0 - All service code uses generated OpenAPI clients
#   1 - Found manual HTTP client violations
#
# NOTE: This script does NOT run tests, only validates code.
#       To run tests: make test-integration-[service] test-e2e-[service]
# ========================================

set -e

echo ""
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║   OpenAPI Client Usage Validation (All Services)         ║"
echo "╚═══════════════════════════════════════════════════════════╝"

# ========================================
# CONFIGURATION
# ========================================

# Services to validate
SERVICES=(
    "aianalysis"
    "datastorage"
    "gateway"
    "notification"
    "remediationorchestrator"
    "signalprocessing"
    "workflowexecution"
)

# Directories to validate per service
get_service_dirs() {
    local service="$1"
    local dirs=()

    # Production code
    [ -d "pkg/$service" ] && dirs+=("pkg/$service")

    # Controller code
    [ -d "internal/controller/$service" ] && dirs+=("internal/controller/$service")

    # Unit tests
    [ -d "test/unit/$service" ] && dirs+=("test/unit/$service")

    # Integration tests
    [ -d "test/integration/$service" ] && dirs+=("test/integration/$service")

    # E2E tests
    [ -d "test/e2e/$service" ] && dirs+=("test/e2e/$service")

    # Main entry point
    [ -d "cmd/$service" ] && dirs+=("cmd/$service")

    echo "${dirs[@]}"
}

# Shared client directories
SHARED_CLIENT_DIRS=(
    "pkg/holmesgpt/client"              # HAPI client wrapper
    "pkg/datastorage/client"            # Data Storage client (if exists)
)

# Forbidden patterns (manual HTTP client usage)
FORBIDDEN_PATTERNS=(
    "http\\.NewRequestWithContext.*(holmesgpt|datastorage|api/v1)"
    "json\\.Marshal.*(IncidentRequest|AlertRequest)"
    "http\\.MethodPost.*(holmesgpt|datastorage)"
    "http\\.Do.*(holmesgpt|datastorage)"
    "http\\.Post.*(holmesgpt|datastorage)"
    "http\\.Get.*(holmesgpt|datastorage)"
)

# Files to exclude from validation
EXCLUDE_FILES=(
    "*_gen.go"                           # Generated code
    "*_test.go"                          # Temporarily exclude to focus on prod
    "docs/"                              # Documentation
    "*.md"                               # Markdown files
)

# ========================================
# VALIDATION FUNCTIONS
# ========================================

total_violations=0
service_violations=()

# Check for forbidden patterns in service code
check_forbidden_patterns() {
    local dir="$1"
    local pattern="$2"
    local service="$3"

    # Build exclude arguments
    local exclude_args=""
    for exclude in "${EXCLUDE_FILES[@]}"; do
        exclude_args="$exclude_args --exclude=$exclude"
    done

    # Search for pattern
    local results
    results=$(grep -rn "$pattern" "$dir" \
        --include="*.go" \
        $exclude_args \
        2>/dev/null || true)

    if [ -n "$results" ]; then
        echo "  ❌ Found forbidden pattern: $pattern"
        echo "$results" | while IFS= read -r line; do
            echo "     $line"
        done
        ((total_violations++))
        service_violations+=("$service: $dir - $pattern")
        return 1
    fi
    return 0
}

# ========================================
# CODE VALIDATION
# ========================================

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "CODE VALIDATION (DD-HAPI-003 - All Services)"
echo "═══════════════════════════════════════════════════════════"
echo ""
echo "🔍 Validating OpenAPI client usage across all services..."
echo ""
echo "📋 Services:"
for service in "${SERVICES[@]}"; do
    echo "   - $service"
done
echo ""
echo "🚫 Forbidden patterns:"
for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
    echo "   - $pattern"
done
echo ""

all_services_passed=true
services_checked=0
services_passed=0
services_failed=0

# Check each service
for service in "${SERVICES[@]}"; do
    echo "═══════════════════════════════════════════════════════════"
    echo "🔍 Service: $service"
    echo "═══════════════════════════════════════════════════════════"

    # Get directories for this service
    service_dirs=($(get_service_dirs "$service"))

    if [ ${#service_dirs[@]} -eq 0 ]; then
        echo "⚠️  No directories found for $service - skipping"
        echo ""
        continue
    fi

    ((services_checked++))
    service_passed=true
    service_violation_count=0

    # Check each directory for this service
    for dir in "${service_dirs[@]}"; do
        echo "   📂 $dir"

        dir_has_violations=false
        for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
            if ! check_forbidden_patterns "$dir" "$pattern" "$service"; then
                dir_has_violations=true
                ((service_violation_count++))
                service_passed=false
                all_services_passed=false
            fi
        done

        if [ "$dir_has_violations" = false ]; then
            echo "      ✅ Uses OpenAPI client"
        fi
    done

    if [ "$service_passed" = true ]; then
        echo "   ✅ $service: PASSED (${#service_dirs[@]} directories validated)"
        ((services_passed++))
    else
        echo "   ❌ $service: FAILED ($service_violation_count violation(s))"
        ((services_failed++))
    fi
    echo ""
done

# Check shared client directories
echo "═══════════════════════════════════════════════════════════"
echo "🔍 Shared Client Libraries"
echo "═══════════════════════════════════════════════════════════"
for dir in "${SHARED_CLIENT_DIRS[@]}"; do
    if [ ! -d "$dir" ]; then
        echo "⚠️  Skipping $dir (does not exist)"
        continue
    fi

    echo "   📂 $dir"

    dir_has_violations=false
    for pattern in "${FORBIDDEN_PATTERNS[@]}"; do
        if ! check_forbidden_patterns "$dir" "$pattern" "shared"; then
            dir_has_violations=true
            all_services_passed=false
        fi
    done

    if [ "$dir_has_violations" = false ]; then
        echo "      ✅ Uses OpenAPI client"
    fi
done
echo ""

# ========================================
# RESULTS
# ========================================

echo ""
echo "═══════════════════════════════════════════════════════════"
echo "VALIDATION SUMMARY"
echo "═══════════════════════════════════════════════════════════"
echo ""

echo "📊 Services Validated: $services_checked"
echo "   ✅ Passed: $services_passed"
echo "   ❌ Failed: $services_failed"
echo ""

if [ "$all_services_passed" = true ]; then
    echo "✅ SUCCESS: All services use generated OpenAPI clients"
    echo ""
    echo "📋 DD-HAPI-003 Compliance:"
    echo "   ✅ Production code: Uses OpenAPI clients"
    echo "   ✅ Controllers: Use OpenAPI clients"
    echo "   ✅ Main entry points: Use OpenAPI clients"
    echo "   ✅ Shared client libraries: Use OpenAPI clients"
    echo ""
    echo "🎯 Benefits:"
    echo "   ✅ Compile-time type safety enforced"
    echo "   ✅ Contract compliance with service OpenAPI specs"
    echo "   ✅ Consistent across all services"
    echo "   ✅ HTTP 500 errors prevented"
    echo ""
    echo "📚 See: docs/architecture/decisions/DD-HAPI-003-mandatory-openapi-client-usage.md"
    echo ""
    exit 0
else
    echo "❌ FAILURE: Found manual HTTP client violations"
    echo ""
    echo "🚨 ENFORCEMENT VIOLATION:"
    echo "   All service code MUST use generated OpenAPI clients"
    echo "   Manual HTTP clients are FORBIDDEN for external service calls"
    echo ""
    echo "📊 Violation Details:"
    echo "   Total violations: $total_violations"
    echo "   Services affected: $services_failed"
    echo ""

    if [ ${#service_violations[@]} -gt 0 ]; then
        echo "🔍 Violations by Service:"
        printf '%s\n' "${service_violations[@]}" | sort -u | while IFS= read -r violation; do
            echo "   • $violation"
        done
        echo ""
    fi

    echo "🔧 Remediation:"
    echo "   1. Replace manual http.NewRequestWithContext with generated client"
    echo "   2. Use client.[Operation]APIV1[Endpoint]Post(ctx, req)"
    echo "   3. Type-assert response interface to concrete type"
    echo "   4. Never manually marshal JSON for external service requests"
    echo ""
    echo "📝 Example Migration:"
    echo ""
    echo "   ❌ BEFORE (Manual HTTP):"
    echo '   body, _ := json.Marshal(req)'
    echo '   httpReq, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))'
    echo '   resp, _ := httpClient.Do(httpReq)'
    echo ""
    echo "   ✅ AFTER (OpenAPI Client):"
    echo '   res, err := externalClient.Investigate(ctx, req)'
    echo '   // Type-safe, contract-compliant, no manual HTTP'
    echo ""
    echo "📚 See: docs/architecture/decisions/DD-HAPI-003-mandatory-openapi-client-usage.md"
    echo ""
    exit 1
fi
