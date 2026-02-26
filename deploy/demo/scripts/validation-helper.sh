#!/usr/bin/env bash
# Shared validation helpers for demo scenario validate.sh scripts and VHS tapes.
# Source this from validate.sh or .tape hidden blocks:
#   source "$(dirname "$0")/../../scripts/validation-helper.sh"
#
# All display functions produce clean terminal output suitable for both
# interactive use and VHS recordings. Functions are individually callable:
#   source validation-helper.sh && show_ai_analysis demo-hpa

# ── Color support ────────────────────────────────────────────────────────────
# Respects NO_COLOR (https://no-color.org/) and --no-color flag.
_VALIDATION_NO_COLOR="${NO_COLOR:-}"

_c_reset=""
_c_green=""
_c_red=""
_c_blue=""
_c_cyan=""
_c_yellow=""
_c_bold=""
_c_dim=""

_init_colors() {
    if [ -n "$_VALIDATION_NO_COLOR" ]; then return; fi
    if [ ! -t 1 ]; then return; fi  # not a terminal
    _c_reset=$'\033[0m'
    _c_green=$'\033[32m'
    _c_red=$'\033[31m'
    _c_blue=$'\033[34m'
    _c_cyan=$'\033[36m'
    _c_yellow=$'\033[33m'
    _c_bold=$'\033[1m'
    _c_dim=$'\033[2m'
}
_init_colors

# ── Assertion tracking ───────────────────────────────────────────────────────
_ASSERT_PASS=0
_ASSERT_FAIL=0
_ASSERT_TOTAL=0

# ── Utilities ────────────────────────────────────────────────────────────────

_ts() {
    date '+%H:%M:%S'
}

log_phase() {
    printf '%s[%s]%s %s\n' "$_c_dim" "$(_ts)" "$_c_reset" "$1"
}

log_transition() {
    printf '%s[%s]%s %sPhase: %s -> %s%s\n' \
        "$_c_dim" "$(_ts)" "$_c_reset" "$_c_blue" "$1" "$2" "$_c_reset"
}

log_info() {
    printf '           %s\n' "$1"
}

log_success() {
    printf '%s[%s]%s %s%s%s\n' "$_c_dim" "$(_ts)" "$_c_reset" "$_c_green" "$1" "$_c_reset"
}

log_error() {
    printf '%s[%s]%s %s%s%s\n' "$_c_dim" "$(_ts)" "$_c_reset" "$_c_red" "$1" "$_c_reset"
}

log_warn() {
    printf '%s[%s]%s %s%s%s\n' "$_c_dim" "$(_ts)" "$_c_reset" "$_c_yellow" "$1" "$_c_reset"
}

# ── CRD namespace ────────────────────────────────────────────────────────────
# Pipeline CRDs (RR, SP, AA, WFE, EA, RAR) are created in the platform
# namespace, not in the scenario namespace. The scenario namespace is only
# used for workload assertions (pods, HPA, deployments, etc.).
PLATFORM_NS="${PLATFORM_NS:-kubernaut-system}"

# ── CRD field accessors ─────────────────────────────────────────────────────
# Thin wrappers over kubectl jsonpath. Return empty string on missing fields.
# $1 = scenario namespace (used to find the RR by target namespace label)
# When multiple RRs exist, we filter by the most recent one targeting $1.

_find_rr_name() {
    local target_ns="$1"
    # Find the most recent RR whose spec.targetResource references the target namespace
    kubectl get remediationrequests -n "$PLATFORM_NS" \
        -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.signalLabels.namespace}{"\n"}{end}' 2>/dev/null \
        | grep "$target_ns" | tail -1 | cut -f1
}

get_rr_phase() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    kubectl get remediationrequests "$rr_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.overallPhase}' 2>/dev/null || echo ""
}

get_rr_outcome() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    kubectl get remediationrequests "$rr_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.outcome}' 2>/dev/null || echo ""
}

get_rr_name() {
    _find_rr_name "$1"
}

get_sp_phase() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    local sp_name="sp-${rr_name}"
    kubectl get signalprocessings "$sp_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo ""
}

get_aa_phase() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    local aa_name="ai-${rr_name}"
    kubectl get aianalyses "$aa_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo ""
}

get_wfe_phase() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    local wfe_name="we-${rr_name}"
    kubectl get workflowexecutions "$wfe_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo ""
}

get_ea_phase() {
    local rr_name
    rr_name=$(_find_rr_name "$1")
    if [ -z "$rr_name" ]; then echo ""; return; fi
    local ea_name="ea-${rr_name}"
    kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" \
        -o jsonpath='{.status.phase}' 2>/dev/null || echo ""
}

get_rar_count() {
    kubectl get remediationapprovalrequests -n "$PLATFORM_NS" --no-headers 2>/dev/null | wc -l | tr -d ' '
}

# ── Wait functions ───────────────────────────────────────────────────────────

# Wait for a Prometheus alert to appear in AlertManager.
# Args: $1=alert_name $2=namespace $3=timeout_seconds (default 300)
wait_for_alert() {
    local alert_name="$1"
    local namespace="$2"
    local timeout="${3:-300}"
    local am_pod="alertmanager-kube-prometheus-stack-alertmanager-0"
    local elapsed=0
    local interval=10

    log_phase "Waiting for ${_c_cyan}${alert_name}${_c_reset} alert (timeout: ${timeout}s)..."

    while [ "$elapsed" -lt "$timeout" ]; do
        local count
        count=$(kubectl exec -n monitoring "$am_pod" -- \
            amtool alert query "alertname=${alert_name}" \
            --alertmanager.url=http://localhost:9093 \
            --output=json 2>/dev/null \
            | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")

        if [ "$count" != "0" ] && [ "$count" != "" ]; then
            log_success "Alert ${alert_name} fired in AlertManager"
            return 0
        fi

        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "Timed out waiting for alert ${alert_name} after ${timeout}s"
    return 1
}

# Wait for at least one RemediationRequest to exist in the namespace.
# Args: $1=namespace $2=timeout_seconds (default 120)
wait_for_rr() {
    local target_ns="$1"
    local timeout="${2:-120}"
    local elapsed=0
    local interval=5

    log_phase "Waiting for RemediationRequest (target: ${target_ns})..."

    while [ "$elapsed" -lt "$timeout" ]; do
        local rr_name
        rr_name=$(_find_rr_name "$target_ns")
        if [ -n "$rr_name" ]; then
            local phase
            phase=$(get_rr_phase "$target_ns")
            log_success "RemediationRequest ${rr_name} created -> Phase: ${phase}"
            return 0
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "Timed out waiting for RemediationRequest after ${timeout}s"
    return 1
}

# Wait for RR to reach a specific phase (or terminal).
# Args: $1=namespace $2=target_phase $3=timeout (default 600)
wait_for_rr_phase() {
    local namespace="$1"
    local target="$2"
    local timeout="${3:-600}"
    local elapsed=0
    local interval=10

    while [ "$elapsed" -lt "$timeout" ]; do
        local phase
        phase=$(get_rr_phase "$namespace")
        if [ "$phase" = "$target" ]; then
            return 0
        fi
        # Terminal states -- stop waiting
        case "$phase" in
            Completed|Failed|TimedOut|Cancelled|Skipped)
                return 0
                ;;
        esac
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    return 1
}

# ── Display functions ────────────────────────────────────────────────────────
# Designed for both validate.sh progress output and VHS tape visibility.

show_alert() {
    local alert_name="$1"
    local am_pod="alertmanager-kube-prometheus-stack-alertmanager-0"

    local alerts_json
    alerts_json=$(kubectl exec -n monitoring "$am_pod" -- \
        amtool alert query "alertname=${alert_name}" \
        --alertmanager.url=http://localhost:9093 \
        --output=json 2>/dev/null || echo "[]")

    python3 -c "
import sys, json
alerts = json.loads('''${alerts_json}''')
for a in alerts:
    labels = a.get('labels', {})
    annots = a.get('annotations', {})
    state = a.get('status', {}).get('state', 'unknown')
    print(f'           Alert: {labels.get(\"alertname\", \"N/A\")} | Severity: {labels.get(\"severity\", \"N/A\")} | Namespace: {labels.get(\"namespace\", \"N/A\")}')
    summary = annots.get('summary', '')
    if summary:
        # Wrap long summaries
        import textwrap
        lines = textwrap.wrap(summary, 70)
        print(f'           Summary: {lines[0]}')
        for line in lines[1:]:
            print(f'                    {line}')
" 2>/dev/null || true
}

show_pipeline_status() {
    printf '\n'
    kubectl get remediationrequests,signalprocessings,aianalyses,workflowexecutions,effectivenessassessments -n "$PLATFORM_NS" 2>/dev/null | sed 's/^/           /'
    printf '\n'
}

# Pretty-print the AI Analysis CRD status fields.
# Callable standalone: source validation-helper.sh && show_ai_analysis NAMESPACE
show_ai_analysis() {
    local target_ns="$1"

    local rr_name aa_name
    rr_name=$(_find_rr_name "$target_ns")
    if [ -n "$rr_name" ]; then
        aa_name="ai-${rr_name}"
    else
        aa_name=$(kubectl get aianalyses -n "$PLATFORM_NS" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    fi
    if [ -z "$aa_name" ]; then
        log_info "(no AIAnalysis found)"
        return
    fi

    local ns="$PLATFORM_NS"
    local root_cause severity affected_kind affected_name affected_ns
    local confidence workflow_id action_type rationale approval approval_reason

    root_cause=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.rootCause}' 2>/dev/null)
    severity=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.rootCauseAnalysis.severity}' 2>/dev/null)
    affected_kind=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.kind}' 2>/dev/null)
    affected_name=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.name}' 2>/dev/null)
    affected_ns=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.rootCauseAnalysis.affectedResource.namespace}' 2>/dev/null)
    confidence=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.selectedWorkflow.confidence}' 2>/dev/null)
    workflow_id=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.selectedWorkflow.workflowId}' 2>/dev/null)
    action_type=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.selectedWorkflow.actionType}' 2>/dev/null)
    rationale=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.selectedWorkflow.rationale}' 2>/dev/null)
    approval=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.approvalRequired}' 2>/dev/null)
    approval_reason=$(kubectl get aianalyses "$aa_name" -n "$ns" -o jsonpath='{.status.approvalReason}' 2>/dev/null)

    printf '\n'
    printf '           %s%sAI Analysis%s\n' "$_c_bold" "$_c_cyan" "$_c_reset"
    printf '           %s──────────────────────────────────────────%s\n' "$_c_dim" "$_c_reset"
    printf '           Root Cause:    %s\n' "${root_cause:-N/A}"
    printf '           Severity:      %s\n' "${severity:-unknown}"
    if [ -n "$affected_kind" ]; then
        printf '           Target:        %s/%s (ns: %s)\n' "$affected_kind" "$affected_name" "$affected_ns"
    fi
    printf '           Workflow:      %s (%s)\n' "${workflow_id:-N/A}" "${action_type:-N/A}"
    printf '           Confidence:    %s\n' "${confidence:-N/A}"
    if [ -n "$rationale" ]; then
        # Wrap long rationale text
        local first_line rest
        first_line=$(echo "$rationale" | head -c 70)
        if [ ${#rationale} -gt 70 ]; then
            printf '           Rationale:     %s...\n' "$first_line"
        else
            printf '           Rationale:     %s\n' "$rationale"
        fi
    fi
    if [ "$approval" = "true" ]; then
        printf '           Approval:      %srequired%s (%s)\n' "$_c_yellow" "$_c_reset" "${approval_reason:-policy match}"
    else
        printf '           Approval:      %snot required%s\n' "$_c_green" "$_c_reset"
    fi
    printf '\n'
}

show_wfe_progress() {
    local target_ns="$1"

    local rr_name wfe_name phase
    rr_name=$(_find_rr_name "$target_ns")
    if [ -n "$rr_name" ]; then
        wfe_name="we-${rr_name}"
    else
        wfe_name=$(kubectl get workflowexecutions -n "$PLATFORM_NS" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    fi
    if [ -z "$wfe_name" ]; then
        log_info "(no WorkflowExecution found)"
        return
    fi

    phase=$(kubectl get workflowexecutions "$wfe_name" -n "$PLATFORM_NS" -o jsonpath='{.status.phase}' 2>/dev/null)
    local completed total duration
    completed=$(kubectl get workflowexecutions "$wfe_name" -n "$PLATFORM_NS" -o jsonpath='{.status.executionStatus.completedTasks}' 2>/dev/null)
    total=$(kubectl get workflowexecutions "$wfe_name" -n "$PLATFORM_NS" -o jsonpath='{.status.executionStatus.totalTasks}' 2>/dev/null)
    duration=$(kubectl get workflowexecutions "$wfe_name" -n "$PLATFORM_NS" -o jsonpath='{.status.duration}' 2>/dev/null)

    printf '           WFE Phase: %s' "$phase"
    if [ -n "$completed" ] && [ -n "$total" ]; then
        printf ' (%s/%s tasks)' "$completed" "$total"
    fi
    if [ -n "$duration" ]; then
        printf ' [%s]' "$duration"
    fi
    printf '\n'
}

# Pretty-print the EffectivenessAssessment status.
# Callable standalone: source validation-helper.sh && show_effectiveness NAMESPACE
show_effectiveness() {
    local target_ns="$1"

    local rr_name ea_name
    rr_name=$(_find_rr_name "$target_ns")
    if [ -n "$rr_name" ]; then
        ea_name="ea-${rr_name}"
    else
        ea_name=$(kubectl get effectivenessassessments -n "$PLATFORM_NS" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    fi
    if [ -z "$ea_name" ]; then
        log_info "(no EffectivenessAssessment found)"
        return
    fi

    local phase reason message alert_score health_score metrics_score
    phase=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.phase}' 2>/dev/null)
    reason=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.assessmentReason}' 2>/dev/null)
    message=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.message}' 2>/dev/null)
    alert_score=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.components.alertScore}' 2>/dev/null)
    health_score=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.components.healthScore}' 2>/dev/null)
    metrics_score=$(kubectl get effectivenessassessments "$ea_name" -n "$PLATFORM_NS" -o jsonpath='{.status.components.metricsScore}' 2>/dev/null)

    printf '\n'
    printf '           %s%sEffectiveness Assessment%s\n' "$_c_bold" "$_c_cyan" "$_c_reset"
    printf '           %s──────────────────────────────────────────%s\n' "$_c_dim" "$_c_reset"
    printf '           Phase:         %s\n' "${phase:-Pending}"
    printf '           Reason:        %s\n' "${reason:-N/A}"
    if [ -n "$message" ]; then
        printf '           Message:       %s\n' "$message"
    fi
    printf '           Alert Score:   %s\n' "${alert_score:-pending}"
    printf '           Health Score:  %s\n' "${health_score:-pending}"
    printf '           Metrics Score: %s\n' "${metrics_score:-pending}"
    printf '\n'
}

# ── Approval ─────────────────────────────────────────────────────────────────

# Auto-approve the first RAR in the namespace.
auto_approve_rar() {
    local _unused="$1"  # kept for API compat; always uses PLATFORM_NS

    local rar_name
    rar_name=$(kubectl get remediationapprovalrequests -n "$PLATFORM_NS" \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    if [ -z "$rar_name" ]; then
        log_warn "No RemediationApprovalRequest found to approve"
        return 1
    fi

    kubectl patch remediationapprovalrequest "$rar_name" -n "$PLATFORM_NS" \
        --type=merge --subresource=status \
        -p '{"status":{"decision":"Approved","decidedBy":"validate.sh","decisionMessage":"Auto-approved by validation script"}}' \
        >/dev/null 2>&1

    log_success "Approved RAR ${rar_name}"
}

# ── Main pipeline poller ─────────────────────────────────────────────────────
# Polls RR overallPhase, prints transitions, shows AI analysis and WFE inline.
#
# Args: $1=target_namespace $2=timeout (default 600) $3=--auto-approve|--interactive (default --auto-approve)
poll_pipeline() {
    local target_ns="$1"
    local timeout="${2:-600}"
    local approve_mode="${3:---auto-approve}"
    local elapsed=0
    local interval=10
    local prev_phase=""
    local aa_shown=false
    local ea_shown=false

    log_phase "Polling pipeline (timeout: ${timeout}s, approval: ${approve_mode})..."

    while [ "$elapsed" -lt "$timeout" ]; do
        local phase
        phase=$(get_rr_phase "$target_ns")

        # Print phase transitions
        if [ "$phase" != "$prev_phase" ] && [ -n "$phase" ]; then
            if [ -n "$prev_phase" ]; then
                log_transition "$prev_phase" "$phase"
            fi

            case "$phase" in
                Analyzing)
                    log_phase "AI Analysis in progress..."
                    ;;
                AwaitingApproval)
                    if [ "$aa_shown" = false ]; then
                        show_ai_analysis "$target_ns"
                        aa_shown=true
                    fi
                    if [ "$approve_mode" = "--auto-approve" ]; then
                        sleep 2
                        auto_approve_rar
                    else
                        log_warn "Awaiting manual approval. Approve with:"
                        log_info "  kubectl patch rar \$(kubectl get rar -n $PLATFORM_NS -o name | head -1) -n $PLATFORM_NS --type=merge --subresource=status -p '{\"status\":{\"decision\":\"Approved\"}}'"
                    fi
                    ;;
                Executing)
                    if [ "$aa_shown" = false ]; then
                        show_ai_analysis "$target_ns"
                        aa_shown=true
                    fi
                    log_phase "WorkflowExecution running..."
                    ;;
                Completed)
                    if [ "$aa_shown" = false ]; then
                        show_ai_analysis "$target_ns"
                        aa_shown=true
                    fi
                    local outcome
                    outcome=$(get_rr_outcome "$target_ns")
                    log_success "Pipeline completed (outcome: ${outcome})"
                    _wait_for_ea "$target_ns"
                    return 0
                    ;;
                Failed|TimedOut|Cancelled)
                    log_error "Pipeline terminated with phase: ${phase}"
                    local rr_name block_msg
                    rr_name=$(_find_rr_name "$target_ns")
                    block_msg=$(kubectl get remediationrequests "$rr_name" -n "$PLATFORM_NS" \
                        -o jsonpath='{.status.blockMessage}' 2>/dev/null)
                    if [ -n "$block_msg" ]; then
                        log_info "Block message: $block_msg"
                    fi
                    return 1
                    ;;
                Blocked|Skipped)
                    local rr_name block_reason
                    rr_name=$(_find_rr_name "$target_ns")
                    block_reason=$(kubectl get remediationrequests "$rr_name" -n "$PLATFORM_NS" \
                        -o jsonpath='{.status.blockReason}' 2>/dev/null)
                    log_warn "Pipeline ${phase} (reason: ${block_reason:-unknown})"
                    return 0
                    ;;
            esac

            prev_phase="$phase"
        fi

        # Show WFE progress during Executing phase
        if [ "$phase" = "Executing" ]; then
            show_wfe_progress "$target_ns"
        fi

        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "Pipeline timed out after ${timeout}s (last phase: ${prev_phase})"
    return 1
}

# Wait for EA after pipeline completion (if applicable).
_wait_for_ea() {
    local target_ns="$1"
    local timeout=300
    local elapsed=0
    local interval=15

    local ea_phase
    ea_phase=$(get_ea_phase "$target_ns")
    if [ -z "$ea_phase" ]; then
        log_phase "Waiting for EffectivenessAssessment..."
        while [ "$elapsed" -lt 60 ]; do
            ea_phase=$(get_ea_phase "$target_ns")
            if [ -n "$ea_phase" ]; then break; fi
            sleep 5
            elapsed=$((elapsed + 5))
        done
    fi

    if [ -z "$ea_phase" ]; then
        log_info "(no EffectivenessAssessment created)"
        return
    fi

    log_phase "Waiting for EffectivenessAssessment to complete..."
    elapsed=0
    while [ "$elapsed" -lt "$timeout" ]; do
        ea_phase=$(get_ea_phase "$target_ns")
        case "$ea_phase" in
            Completed|Failed)
                show_effectiveness "$target_ns"
                return
                ;;
        esac
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_warn "EA did not complete within ${timeout}s"
    show_effectiveness "$target_ns"
}

# ── Assertion helpers ────────────────────────────────────────────────────────

assert_eq() {
    local actual="$1"
    local expected="$2"
    local label="$3"
    _ASSERT_TOTAL=$((_ASSERT_TOTAL + 1))

    if [ "$actual" = "$expected" ]; then
        _ASSERT_PASS=$((_ASSERT_PASS + 1))
        printf '           %s[PASS]%s %s = %s\n' "$_c_green" "$_c_reset" "$label" "$actual"
    else
        _ASSERT_FAIL=$((_ASSERT_FAIL + 1))
        printf '           %s[FAIL]%s %s = %s (expected: %s)\n' "$_c_red" "$_c_reset" "$label" "$actual" "$expected"
    fi
}

assert_neq() {
    local actual="$1"
    local unexpected="$2"
    local label="$3"
    _ASSERT_TOTAL=$((_ASSERT_TOTAL + 1))

    if [ "$actual" != "$unexpected" ]; then
        _ASSERT_PASS=$((_ASSERT_PASS + 1))
        printf '           %s[PASS]%s %s = %s (not %s)\n' "$_c_green" "$_c_reset" "$label" "$actual" "$unexpected"
    else
        _ASSERT_FAIL=$((_ASSERT_FAIL + 1))
        printf '           %s[FAIL]%s %s = %s (should not be %s)\n' "$_c_red" "$_c_reset" "$label" "$actual" "$unexpected"
    fi
}

assert_gt() {
    local actual="$1"
    local threshold="$2"
    local label="$3"
    _ASSERT_TOTAL=$((_ASSERT_TOTAL + 1))

    if [ "$actual" -gt "$threshold" ] 2>/dev/null; then
        _ASSERT_PASS=$((_ASSERT_PASS + 1))
        printf '           %s[PASS]%s %s = %s (> %s)\n' "$_c_green" "$_c_reset" "$label" "$actual" "$threshold"
    else
        _ASSERT_FAIL=$((_ASSERT_FAIL + 1))
        printf '           %s[FAIL]%s %s = %s (expected > %s)\n' "$_c_red" "$_c_reset" "$label" "$actual" "$threshold"
    fi
}

assert_contains() {
    local haystack="$1"
    local needle="$2"
    local label="$3"
    _ASSERT_TOTAL=$((_ASSERT_TOTAL + 1))

    if echo "$haystack" | grep -q "$needle" 2>/dev/null; then
        _ASSERT_PASS=$((_ASSERT_PASS + 1))
        printf '           %s[PASS]%s %s contains "%s"\n' "$_c_green" "$_c_reset" "$label" "$needle"
    else
        _ASSERT_FAIL=$((_ASSERT_FAIL + 1))
        printf '           %s[FAIL]%s %s does not contain "%s"\n' "$_c_red" "$_c_reset" "$label" "$needle"
    fi
}

# Print final result summary. Returns 0 on all-pass, 1 on any failure.
print_result() {
    local scenario_name="${1:-scenario}"
    printf '\n'
    printf '           %s%sScenario Assertions%s\n' "$_c_bold" "$_c_cyan" "$_c_reset"
    printf '           %s──────────────────────────────────────────%s\n' "$_c_dim" "$_c_reset"

    if [ "$_ASSERT_FAIL" -eq 0 ]; then
        printf '\n           %s%s============================================%s\n' "$_c_bold" "$_c_green" "$_c_reset"
        printf '           %s%s RESULT: PASS (%d/%d assertions passed)%s\n' "$_c_bold" "$_c_green" "$_ASSERT_PASS" "$_ASSERT_TOTAL" "$_c_reset"
        printf '           %s%s============================================%s\n\n' "$_c_bold" "$_c_green" "$_c_reset"
        return 0
    else
        printf '\n           %s%s============================================%s\n' "$_c_bold" "$_c_red" "$_c_reset"
        printf '           %s%s RESULT: FAIL (%d/%d passed, %d failed)%s\n' "$_c_bold" "$_c_red" "$_ASSERT_PASS" "$_ASSERT_TOTAL" "$_ASSERT_FAIL" "$_c_reset"
        printf '           %s%s============================================%s\n\n' "$_c_bold" "$_c_red" "$_c_reset"
        return 1
    fi
}

# Reset assertion counters (useful when running multiple scenarios).
reset_assertions() {
    _ASSERT_PASS=0
    _ASSERT_FAIL=0
    _ASSERT_TOTAL=0
}
