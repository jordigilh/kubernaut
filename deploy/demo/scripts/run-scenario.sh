#!/usr/bin/env bash
# Scenario orchestrator -- chains run.sh + validate.sh + cleanup.sh
#
# Usage:
#   ./deploy/demo/scripts/run-scenario.sh --scenario hpa-maxed
#   ./deploy/demo/scripts/run-scenario.sh --scenario hpa-maxed --auto-approve --cleanup
#   ./deploy/demo/scripts/run-scenario.sh --scenario hpa-maxed,stuck-rollout
#   ./deploy/demo/scripts/run-scenario.sh --validate-only --scenario hpa-maxed
#   ./deploy/demo/scripts/run-scenario.sh --list
#
# Flags:
#   --scenario NAME[,NAME]   One or more scenarios (comma-separated)
#   --auto-approve           Auto-approve RemediationApprovalRequests (default)
#   --interactive            Pause for manual RAR approval
#   --cleanup                Run cleanup.sh after each scenario
#   --validate-only          Skip run.sh, only validate (scenario already deployed)
#   --skip-run               Alias for --validate-only
#   --no-color               Disable color output
#   --timeout SECONDS        Pipeline timeout per scenario (default: 600)
#   --list                   List available scenarios and exit
#   --help                   Show this help
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
SCENARIOS_DIR="${REPO_ROOT}/deploy/demo/scenarios"

# Defaults
SCENARIO_LIST=""
APPROVE_MODE="--auto-approve"
DO_CLEANUP=false
VALIDATE_ONLY=false
PIPELINE_TIMEOUT=600
NO_COLOR_FLAG=""
DS_PORT_FORWARD_PID=""

usage() {
    sed -n '2,/^set /{ /^#/s/^# \?//p }' "$0"
    exit 0
}

cleanup_port_forward() {
    if [ -n "$DS_PORT_FORWARD_PID" ]; then
        kill "$DS_PORT_FORWARD_PID" 2>/dev/null || true
        DS_PORT_FORWARD_PID=""
    fi
}
trap cleanup_port_forward EXIT

list_scenarios() {
    echo "Available scenarios:"
    echo ""
    for dir in "${SCENARIOS_DIR}"/*/; do
        local name
        name=$(basename "$dir")
        local has_run="" has_validate="" has_cleanup=""
        [ -f "${dir}/run.sh" ] && has_run="run"
        [ -f "${dir}/validate.sh" ] && has_validate="validate"
        [ -f "${dir}/cleanup.sh" ] && has_cleanup="cleanup"
        printf "  %-30s  [%s]\n" "$name" "${has_run:+run }${has_validate:+validate }${has_cleanup:+cleanup}"
    done
    echo ""
}

# ── Parse arguments ──────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scenario)
            SCENARIO_LIST="$2"
            shift 2
            ;;
        --auto-approve)
            APPROVE_MODE="--auto-approve"
            shift
            ;;
        --interactive)
            APPROVE_MODE="--interactive"
            shift
            ;;
        --cleanup)
            DO_CLEANUP=true
            shift
            ;;
        --validate-only|--skip-run)
            VALIDATE_ONLY=true
            shift
            ;;
        --no-color)
            export NO_COLOR=1
            NO_COLOR_FLAG="--no-color"
            shift
            ;;
        --timeout)
            PIPELINE_TIMEOUT="$2"
            shift 2
            ;;
        --list)
            list_scenarios
            exit 0
            ;;
        --help|-h)
            usage
            ;;
        *)
            echo "Unknown option: $1 (try --help)"
            exit 1
            ;;
    esac
done

if [ -z "$SCENARIO_LIST" ]; then
    echo "ERROR: --scenario is required (or use --list to see available scenarios)"
    exit 1
fi

# Source color support
source "${SCRIPT_DIR}/validation-helper.sh"

# ── Port-forward management ──────────────────────────────────────────────────

ensure_datastorage_port_forward() {
    if curl -sf -o /dev/null --connect-timeout 2 "http://localhost:30081/health" 2>/dev/null; then
        return 0
    fi

    log_phase "Starting DataStorage port-forward (localhost:30081 -> svc/data-storage-service:8080)..."
    kubectl port-forward -n kubernaut-system svc/data-storage-service 30081:8080 >/dev/null 2>&1 &
    DS_PORT_FORWARD_PID=$!

    local retries=0
    while [ "$retries" -lt 15 ]; do
        if curl -sf -o /dev/null --connect-timeout 1 "http://localhost:30081/health" 2>/dev/null; then
            log_success "DataStorage port-forward ready"
            return 0
        fi
        sleep 1
        retries=$((retries + 1))
    done

    log_error "Failed to establish DataStorage port-forward"
    return 1
}

ensure_prometheus_port_forward() {
    if curl -sf -o /dev/null --connect-timeout 2 "http://localhost:9090/-/ready" 2>/dev/null; then
        return 0
    fi

    log_phase "Starting Prometheus port-forward (localhost:9090)..."
    kubectl port-forward -n monitoring svc/kube-prometheus-stack-prometheus 9090:9090 >/dev/null 2>&1 &

    local retries=0
    while [ "$retries" -lt 10 ]; do
        if curl -sf -o /dev/null --connect-timeout 1 "http://localhost:9090/-/ready" 2>/dev/null; then
            log_success "Prometheus port-forward ready"
            return 0
        fi
        sleep 1
        retries=$((retries + 1))
    done

    log_warn "Prometheus port-forward may not be ready (non-critical)"
}

# ── Run scenarios ────────────────────────────────────────────────────────────

IFS=',' read -ra SCENARIOS <<< "$SCENARIO_LIST"

RESULTS=()
TOTAL_START=$(date +%s)

# Ensure port-forwards before any scenario
ensure_datastorage_port_forward
ensure_prometheus_port_forward

for scenario in "${SCENARIOS[@]}"; do
    scenario=$(echo "$scenario" | tr -d ' ')  # trim whitespace
    scenario_dir="${SCENARIOS_DIR}/${scenario}"

    if [ ! -d "$scenario_dir" ]; then
        echo "ERROR: Scenario '${scenario}' not found at ${scenario_dir}"
        RESULTS+=("${scenario}:ERROR:0")
        continue
    fi

    echo ""
    echo "  ${_c_bold}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${_c_reset}"
    echo "  ${_c_bold}  Scenario: ${scenario}${_c_reset}"
    echo "  ${_c_bold}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${_c_reset}"
    echo ""

    SCENARIO_START=$(date +%s)
    scenario_result="PASS"

    # Step 1: run.sh (unless --validate-only)
    if [ "$VALIDATE_ONLY" = false ]; then
        if [ -f "${scenario_dir}/run.sh" ]; then
            log_phase "Running ${scenario}/run.sh..."
            if ! bash "${scenario_dir}/run.sh"; then
                log_error "run.sh failed for ${scenario}"
                scenario_result="FAIL"
            fi
        else
            log_error "No run.sh found for ${scenario}"
            scenario_result="SKIP"
        fi
    else
        log_phase "Skipping run.sh (--validate-only)"
    fi

    # Step 2: validate.sh
    if [ "$scenario_result" != "FAIL" ]; then
        if [ -f "${scenario_dir}/validate.sh" ]; then
            log_phase "Running ${scenario}/validate.sh..."
            reset_assertions
            if ! bash "${scenario_dir}/validate.sh" "$APPROVE_MODE" $NO_COLOR_FLAG; then
                scenario_result="FAIL"
            fi
        else
            log_warn "No validate.sh found for ${scenario} -- skipping validation"
            scenario_result="SKIP"
        fi
    fi

    SCENARIO_END=$(date +%s)
    SCENARIO_DURATION=$((SCENARIO_END - SCENARIO_START))

    RESULTS+=("${scenario}:${scenario_result}:${SCENARIO_DURATION}")

    # Step 3: cleanup.sh (if --cleanup)
    if [ "$DO_CLEANUP" = true ]; then
        if [ -f "${scenario_dir}/cleanup.sh" ]; then
            log_phase "Running ${scenario}/cleanup.sh..."
            bash "${scenario_dir}/cleanup.sh" || true
        fi
    fi
done

# ── Summary table ────────────────────────────────────────────────────────────

TOTAL_END=$(date +%s)
TOTAL_DURATION=$((TOTAL_END - TOTAL_START))

echo ""
echo "  ${_c_bold}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${_c_reset}"
echo "  ${_c_bold}  Summary${_c_reset}"
echo "  ${_c_bold}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${_c_reset}"
echo ""
printf "  %-30s  %-8s  %s\n" "Scenario" "Result" "Duration"
printf "  %-30s  %-8s  %s\n" "──────────────────────────────" "────────" "────────"

PASS_COUNT=0
FAIL_COUNT=0

for entry in "${RESULTS[@]}"; do
    IFS=':' read -r name result duration <<< "$entry"
    local_mins=$((duration / 60))
    local_secs=$((duration % 60))
    duration_str=$(printf "%dm %02ds" "$local_mins" "$local_secs")

    case "$result" in
        PASS)
            result_color="${_c_green}"
            PASS_COUNT=$((PASS_COUNT + 1))
            ;;
        FAIL|ERROR)
            result_color="${_c_red}"
            FAIL_COUNT=$((FAIL_COUNT + 1))
            ;;
        *)
            result_color="${_c_yellow}"
            ;;
    esac

    printf "  %-30s  %s%-8s%s  %s\n" "$name" "$result_color" "$result" "$_c_reset" "$duration_str"
done

total_mins=$((TOTAL_DURATION / 60))
total_secs=$((TOTAL_DURATION % 60))
echo ""
printf "  Total: %dm %02ds | %s%d passed%s, %s%d failed%s\n" \
    "$total_mins" "$total_secs" \
    "$_c_green" "$PASS_COUNT" "$_c_reset" \
    "$_c_red" "$FAIL_COUNT" "$_c_reset"
echo ""

# Exit with failure if any scenario failed
[ "$FAIL_COUNT" -eq 0 ]
