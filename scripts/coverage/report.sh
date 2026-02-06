#!/usr/bin/env bash
# scripts/coverage/report.sh
# Generate comprehensive coverage report for Kubernaut services
#
# Usage: ./report.sh [OPTIONS]
#
# This script replaces the 150-line embedded shell+AWK logic in Makefile
# with a modular, testable, feature-rich coverage reporting tool.

set -euo pipefail

# Script directory (for finding AWK scripts and config)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG_FILE="${CONFIG_FILE:-$REPO_ROOT/.coverage-patterns.yaml}"

# Default options
OUTPUT_FORMAT="table"
FILTER_SERVICE=""
FILTER_TIER=""

# Color codes for terminal output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

usage() {
    cat <<EOF
Usage: $0 [OPTIONS]

Generate comprehensive coverage report for all Kubernaut services.

OPTIONS:
    --format FORMAT     Output format: table (default), json, markdown, csv
    --service SERVICE   Report for specific service only
    --tier TIER         Report for specific tier: unit, integration, e2e, all
    --config FILE       Coverage patterns config (default: .coverage-patterns.yaml)
    --no-color          Disable colored output
    --help              Show this help message

EXAMPLES:
    $0                              # Full table report
    $0 --service datastorage        # Just datastorage
    $0 --tier unit                  # Unit coverage only
    $0 --format json > coverage.json  # JSON output
    $0 --format markdown            # Markdown table for docs

EXIT CODES:
    0   Success
    1   General error
    2   Missing coverage files
    3   Invalid arguments
EOF
}

log_info() {
    if [[ "${NO_COLOR:-0}" -eq 0 ]]; then
        echo -e "${BLUE}ℹ${NC} $*" >&2
    else
        echo "INFO: $*" >&2
    fi
}

log_error() {
    if [[ "${NO_COLOR:-0}" -eq 0 ]]; then
        echo -e "${RED}✗${NC} $*" >&2
    else
        echo "ERROR: $*" >&2
    fi
}

log_success() {
    if [[ "${NO_COLOR:-0}" -eq 0 ]]; then
        echo -e "${GREEN}✓${NC} $*" >&2
    else
        echo "SUCCESS: $*" >&2
    fi
}

# Parse YAML config file (simple key extraction)
# Usage: get_yaml_value "go_services.aianalysis.pkg_pattern"
get_yaml_value() {
    local key="$1"
    local file="${2:-$CONFIG_FILE}"
    
    # Simple YAML parser for our specific structure
    # This works for our flat structure, not a general YAML parser
    grep -A1 "$key:" "$file" 2>/dev/null | tail -1 | sed 's/^[[:space:]]*//' | sed 's/"//g'
}

# Calculate Go service coverage
calculate_go_service_coverage() {
    local service="$1"
    local tier="$2"  # unit, integration, e2e
    
    local covfile="coverage_${tier}_${service}.out"
    
    # The "all" tier handles its own file discovery (merging unit+integration+e2e);
    # skip the single-file check so the all) case block runs.
    if [[ "$tier" != "all" ]]; then
        if [[ ! -f "$covfile" ]] || [[ ! -s "$covfile" ]]; then
            # CI fallback: artifacts only have summary percentages stored in .pct files
            local pctfile="coverage_${tier}_${service}.pct"
            if [[ -f "$pctfile" ]]; then
                local pct
                pct=$(tr -d '[:space:]' < "$pctfile")
                # Normalize: ensure % suffix (Go artifacts include %, Python may not)
                [[ "$pct" != *% ]] && pct="${pct}%"
                echo "$pct"
                return
            fi
            echo "-"
            return
        fi
    fi
    
    # Get patterns from config
    local pkg_pattern unit_exclude int_include
    
    # Read from YAML config (simplified - assumes standard structure)
    case "$service" in
        aianalysis)
            pkg_pattern="/pkg/aianalysis/"
            unit_exclude="/(handler\\.go|audit)/"
            int_include="/(handler\\.go|audit)/"
            ;;
        authwebhook)
            pkg_pattern="/pkg/authwebhook/"
            unit_exclude="/(notificationrequest_handler|remediationapprovalrequest_handler|remediationrequest_handler|workflowexecution_handler)\\.go/"
            int_include="/(notificationrequest_handler|remediationapprovalrequest_handler|remediationrequest_handler|workflowexecution_handler)\\.go/"
            ;;
        datastorage)
            pkg_pattern="/pkg/datastorage/"
            unit_exclude="/(server|repository|dlq|ogen-client|mocks)/"
            int_include="/(server|repository|dlq)/"
            ;;
        gateway)
            pkg_pattern="/pkg/gateway/"
            unit_exclude="/(server\\.go|k8s|processing/(crd_creator|distributed_lock|status_updater))/"
            int_include="/(server\\.go|k8s|processing/(crd_creator|distributed_lock|status_updater))/"
            ;;
        notification)
            pkg_pattern="/pkg/notification/"
            unit_exclude="/(client\\.go|delivery|phase|status)/"
            int_include="/(client\\.go|delivery|phase|status)/"
            ;;
        remediationorchestrator)
            pkg_pattern="/pkg/remediationorchestrator/"
            unit_exclude="/(creator|handler/(aianalysis|signalprocessing|workflowexecution)|aggregator|status)/"
            int_include="/(creator|handler/(aianalysis|signalprocessing|workflowexecution)|aggregator|status)/"
            ;;
        signalprocessing)
            pkg_pattern="/pkg/signalprocessing/"
            unit_exclude="/(audit|cache|enricher|handler|status)/"
            int_include="/(audit|cache|enricher|handler|status)/"
            ;;
        workflowexecution)
            pkg_pattern="/pkg/workflowexecution/"
            unit_exclude="/(audit|status)/"
            int_include="/(audit|status)/"
            ;;
        *)
            echo "0.0%"
            return
            ;;
    esac
    
    # Use appropriate AWK script based on tier
    case "$tier" in
        unit)
            awk -f "$SCRIPT_DIR/calculate_go_unit_testable.awk" \
                -v pkg_pattern="$pkg_pattern" \
                -v exclude_pattern="$unit_exclude" \
                "$covfile"
            ;;
        integration)
            awk -f "$SCRIPT_DIR/calculate_go_integration_testable.awk" \
                -v pkg_pattern="$pkg_pattern" \
                -v include_pattern="$int_include" \
                "$covfile"
            ;;
        e2e)
            # E2E uses full package coverage (no filtering)
            go tool cover -func="$covfile" 2>/dev/null | grep total | awk '{print $NF}' || echo "0.0%"
            ;;
        all)
            # Merge all tiers
            local files=()
            [[ -f "coverage_unit_${service}.out" ]] && [[ -s "coverage_unit_${service}.out" ]] && files+=("coverage_unit_${service}.out")
            [[ -f "coverage_integration_${service}.out" ]] && [[ -s "coverage_integration_${service}.out" ]] && files+=("coverage_integration_${service}.out")
            [[ -f "coverage_e2e_${service}.out" ]] && [[ -s "coverage_e2e_${service}.out" ]] && files+=("coverage_e2e_${service}.out")
            
            if [[ ${#files[@]} -eq 0 ]]; then
                # CI fallback: pick the highest percentage from .pct summary files
                # (line-level merge isn't possible with summary-only data)
                local max_pct=""
                local max_val=0
                for t in unit integration e2e; do
                    local pf="coverage_${t}_${service}.pct"
                    if [[ -f "$pf" ]]; then
                        local raw
                        raw=$(tr -d '%[:space:]' < "$pf")
                        if awk "BEGIN{exit (!($raw > $max_val))}"; then
                            max_val="$raw"
                            max_pct="$raw"
                        fi
                    fi
                done
                if [[ -n "$max_pct" ]]; then
                    echo "${max_pct}%"
                else
                    echo "-"
                fi
            elif [[ ${#files[@]} -eq 1 ]]; then
                # Only one file, just calculate from it
                calculate_go_service_coverage "$service" "unit"
            else
                # Merge multiple files
                awk -f "$SCRIPT_DIR/merge_go_coverage.awk" \
                    -v pkg_pattern="$pkg_pattern" \
                    "${files[@]}"
            fi
            ;;
    esac
}

# Calculate Python service coverage (holmesgpt-api)
# When CI only has summary data it creates a file with a single TOTAL line; AWK returns 0.0%.
# Fallback: if result is 0.0% and file has TOTAL line, use that percentage instead.
calculate_python_service_coverage() {
    local tier="$1"  # unit, integration
    
    case "$tier" in
        unit)
            local covfile="coverage_unit_holmesgpt-api.txt"
            if [[ ! -f "$covfile" ]]; then
                echo "-"
                return
            fi
            local result
            result=$(awk -f "$SCRIPT_DIR/calculate_python_unit_testable.awk" "$covfile")
            if [[ "$result" == "0.0%" ]] && grep -q "^TOTAL" "$covfile" 2>/dev/null; then
                grep "^TOTAL" "$covfile" | head -1 | awk '{gsub(/%/, "", $NF); printf "%.1f%%", $NF}'
            else
                echo "$result"
            fi
            ;;
        integration)
            local covfile="coverage_integration_holmesgpt-api_python.txt"
            if [[ ! -f "$covfile" ]]; then
                echo "-"
                return
            fi
            local result
            result=$(awk -f "$SCRIPT_DIR/calculate_python_integration_testable.awk" "$covfile")
            if [[ "$result" == "0.0%" ]] && grep -q "^TOTAL" "$covfile" 2>/dev/null; then
                grep "^TOTAL" "$covfile" | head -1 | awk '{gsub(/%/, "", $NF); printf "%.1f%%", $NF}'
            else
                echo "$result"
            fi
            ;;
        e2e)
            # holmesgpt-api E2E is Go-based (Ginkgo tests)
            local covfile="coverage_e2e_holmesgpt-api.out"
            if [[ ! -f "$covfile" ]] || [[ ! -s "$covfile" ]]; then
                # CI fallback: check for .pct summary percentage
                local pctfile="coverage_e2e_holmesgpt-api.pct"
                if [[ -f "$pctfile" ]]; then
                    local pct
                    pct=$(tr -d '[:space:]' < "$pctfile")
                    [[ "$pct" != *% ]] && pct="${pct}%"
                    echo "$pct"
                    return
                fi
                echo "-"
                return
            fi
            go tool cover -func="$covfile" 2>/dev/null | grep total | awk '{print $NF}' || echo "-"
            ;;
        all)
            # For Python, we can't easily merge Python and Go coverage
            # So just return unit test total
            local covfile="coverage_unit_holmesgpt-api.txt"
            if [[ ! -f "$covfile" ]]; then
                echo "-"
                return
            fi
            grep "^TOTAL" "$covfile" 2>/dev/null | head -1 | awk '{gsub(/%/, "", $NF); print $NF"%"}' || echo "-"
            ;;
    esac
}

# Generate table output
output_table() {
    echo "═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════"
    echo "📊 KUBERNAUT COMPREHENSIVE COVERAGE ANALYSIS (By Test Tier)"
    echo "═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════════"
    printf "%-25s %-15s %-15s %-15s %-15s\n" "Service" "Unit-Testable" "Integration" "E2E" "All Tiers"
    echo "───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────"
    
    # Python service first
    local svc="holmesgpt-api"
    if [[ -z "$FILTER_SERVICE" ]] || [[ "$FILTER_SERVICE" == "$svc" ]]; then
        local unit_cov=$(calculate_python_service_coverage "unit")
        local int_cov=$(calculate_python_service_coverage "integration")
        local e2e_cov=$(calculate_python_service_coverage "e2e")
        local all_cov=$(calculate_python_service_coverage "all")
        printf "%-25s %-15s %-15s %-15s %-15s\n" "$svc" "$unit_cov" "$int_cov" "$e2e_cov" "$all_cov"
    fi
    
    # Go services
    for service in aianalysis authwebhook datastorage gateway notification remediationorchestrator signalprocessing workflowexecution; do
        if [[ -n "$FILTER_SERVICE" ]] && [[ "$FILTER_SERVICE" != "$service" ]]; then
            continue
        fi
        
        local unit_cov=$(calculate_go_service_coverage "$service" "unit")
        local int_cov=$(calculate_go_service_coverage "$service" "integration")
        local e2e_cov=$(calculate_go_service_coverage "$service" "e2e")
        local all_cov=$(calculate_go_service_coverage "$service" "all")
        
        printf "%-25s %-15s %-15s %-15s %-15s\n" "$service" "$unit_cov" "$int_cov" "$e2e_cov" "$all_cov"
    done
    
    echo "───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────"
    echo ""
    echo "📝 COLUMN DEFINITIONS:"
    echo "   • Unit-Testable: Coverage of pure logic code (config, validators, builders, formatters, classifiers, etc.)"
    echo "   • Integration: Coverage of integration-only code (handlers, servers, DB adapters, K8s clients, workers, etc.)"
    echo "   • E2E: Coverage of any code from E2E tests (usually covers full workflows)"
    echo "   • All Tiers: Merged coverage where ANY tier covering a line counts (true total coverage)"
    echo ""
    echo "💡 INTERPRETING RESULTS:"
    echo "   - High Unit-Testable %: Great unit test coverage for business logic"
    echo "   - High Integration %: Integration tests cover handlers/servers/DB properly"
    echo "   - High All Tiers %: Excellent overall coverage across all test types"
    echo "   - '-' indicates no coverage file or no relevant code in that category"
    echo ""
    echo "🎯 QUALITY TARGETS:"
    echo "   - Unit-Testable: ≥70% (pure logic should be well-tested)"
    echo "   - Integration: ≥60% (handlers/servers should have good integration coverage)"
    echo "   - All Tiers: ≥80% (overall coverage goal)"
    echo ""
    echo "📋 SERVICE-SPECIFIC NOTES:"
    echo "   • holmesgpt-api (Hybrid): Unit/Integration are Python (pytest-cov), E2E is Go (Ginkgo)"
    echo "     - 'All Tiers' shows Python unit total (Python tiers can't merge with Go E2E easily)"
    echo "     - E2E column shows Go coverage from test/e2e/holmesgpt-api/ (Ginkgo tests)"
    echo "   • Go services: 'All Tiers' merges line-by-line coverage from all test tiers"
    echo ""
    echo "📈 Run 'make test-tier-unit test-tier-integration test-tier-e2e' to update all coverage files."
}

# Generate JSON output
output_json() {
    echo "{"
    echo '  "services": ['
    
    local first=true
    
    # Python service
    if [[ -z "$FILTER_SERVICE" ]] || [[ "$FILTER_SERVICE" == "holmesgpt-api" ]]; then
        echo "    {"
        echo '      "name": "holmesgpt-api",'
        echo '      "language": "python",'
        echo "      \"unit_testable\": \"$(calculate_python_service_coverage unit)\","
        echo "      \"integration\": \"$(calculate_python_service_coverage integration)\","
        echo "      \"e2e\": \"$(calculate_python_service_coverage e2e)\","
        echo "      \"all_tiers\": \"$(calculate_python_service_coverage all)\""
        echo "    },"
        first=false
    fi
    
    # Go services
    for service in aianalysis authwebhook datastorage gateway notification remediationorchestrator signalprocessing workflowexecution; do
        if [[ -n "$FILTER_SERVICE" ]] && [[ "$FILTER_SERVICE" != "$service" ]]; then
            continue
        fi
        
        [[ "$first" == false ]] && echo "," || first=false
        echo "    {"
        echo "      \"name\": \"$service\","
        echo "      \"language\": \"go\","
        echo "      \"unit_testable\": \"$(calculate_go_service_coverage "$service" unit)\","
        echo "      \"integration\": \"$(calculate_go_service_coverage "$service" integration)\","
        echo "      \"e2e\": \"$(calculate_go_service_coverage "$service" e2e)\","
        echo "      \"all_tiers\": \"$(calculate_go_service_coverage "$service" all)\""
        echo -n "    }"
    done
    
    echo ""
    echo "  ]"
    echo "}"
}

# Generate markdown output (for GitHub PR comments)
output_markdown() {
    echo "## 📊 Kubernaut Coverage Report (By Test Tier)"
    echo ""
    echo "| Service | Unit-Testable | Integration-Testable | E2E | All Tiers |"
    echo "|---------|---------------|----------------------|-----|-----------|"
    
    # Python service
    if [[ -z "$FILTER_SERVICE" ]] || [[ "$FILTER_SERVICE" == "holmesgpt-api" ]]; then
        local unit_cov=$(calculate_python_service_coverage "unit")
        local int_cov=$(calculate_python_service_coverage "integration")
        local e2e_cov=$(calculate_python_service_coverage "e2e")
        local all_cov=$(calculate_python_service_coverage "all")
        echo "| holmesgpt-api | $unit_cov | $int_cov | $e2e_cov | $all_cov |"
    fi
    
    # Go services
    for service in aianalysis authwebhook datastorage gateway notification remediationorchestrator signalprocessing workflowexecution; do
        if [[ -n "$FILTER_SERVICE" ]] && [[ "$FILTER_SERVICE" != "$service" ]]; then
            continue
        fi
        
        local unit_cov=$(calculate_go_service_coverage "$service" "unit")
        local int_cov=$(calculate_go_service_coverage "$service" "integration")
        local e2e_cov=$(calculate_go_service_coverage "$service" "e2e")
        local all_cov=$(calculate_go_service_coverage "$service" "all")
        
        echo "| $service | $unit_cov | $int_cov | $e2e_cov | $all_cov |"
    done
    
    echo ""
    echo "### 📝 Column Definitions"
    echo ""
    echo "- **Unit-Testable**: Pure logic code (config, validators, builders, formatters, classifiers)"
    echo "- **Integration-Testable**: Integration-only code (handlers, servers, DB adapters, K8s clients)"
    echo "- **E2E**: End-to-end test coverage (full workflows)"
    echo "- **All Tiers**: Merged coverage (any tier covering a line counts)"
    echo ""
    echo "### 🎯 Quality Targets"
    echo ""
    echo "- Unit-Testable: ≥70%"
    echo "- Integration: ≥60%"
    echo "- All Tiers: ≥80%"
    echo ""
    echo "---"
    echo ""
    echo "_Generated by \`make coverage-report-unit-testable\` | See [Coverage Analysis Report](docs/testing/COVERAGE_ANALYSIS_REPORT.md) for details_"
}

# Main execution
main() {
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --format)
                OUTPUT_FORMAT="$2"
                shift 2
                ;;
            --service)
                FILTER_SERVICE="$2"
                shift 2
                ;;
            --tier)
                FILTER_TIER="$2"
                shift 2
                ;;
            --config)
                CONFIG_FILE="$2"
                shift 2
                ;;
            --no-color)
                NO_COLOR=1
                shift
                ;;
            --help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                exit 3
                ;;
        esac
    done
    
    # Change to repo root for coverage file access
    cd "$REPO_ROOT" || exit 1
    
    # Generate report based on format
    case "$OUTPUT_FORMAT" in
        table)
            output_table
            ;;
        json)
            output_json
            ;;
        markdown)
            output_markdown
            ;;
        csv)
            log_error "CSV format not yet implemented"
            exit 1
            ;;
        *)
            log_error "Invalid format: $OUTPUT_FORMAT"
            exit 3
            ;;
    esac
}

main "$@"
