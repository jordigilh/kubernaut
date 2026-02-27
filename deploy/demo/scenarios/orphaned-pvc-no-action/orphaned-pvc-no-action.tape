# VHS tape: No Action Required -- Disk Pressure Demo (#122)
# Derived from: deploy/demo/scenarios/template.tape
#
# Three-act recording:
#   Act 1 -- "The Problem":     healthy workload, orphaned PVCs, alert fires
#   Act 2 -- "The Pipeline":    SP enrichment, AI analysis finds no matching workflow
#   Act 3 -- "The Result":      RR completed with NoActionRequired, no WFE created
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - NO cleanup workflow seeded (intentionally absent)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action.tape
#
# NOTE: Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# Visible annotations use bash comments (# ...) instead of echo to avoid the
# visual artifact where the echo command text and its output both appear.

Output deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action.gif
Output deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set Padding 20
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────

Hide
Set TypingSpeed 1ms

Type `export PS1='$ '`
Enter
Sleep 500ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/orphaned-pvc-no-action/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/namespace.yaml -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/deployment.yaml -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/prometheus-rule.yaml >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/data-processor -n demo-orphaned-pvc --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms

Show

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════

Type "# Kubernaut Demo: No Action Required -- Disk Pressure (#122)"
Enter
Sleep 3s

Type "# Scenario: orphaned PVCs trigger an alert, but no remediation workflow exists"
Enter
Sleep 3s

Type "# Healthy workload running in a production-labeled namespace"
Enter
Sleep 2s
Type "kubectl get pods -n demo-orphaned-pvc"
Enter
Sleep 5s

Type "# Injecting 5 orphaned PVCs from simulated completed batch jobs..."
Enter
Sleep 2s

# Hidden: run PVC injection and wait for alert
Hide
Set TypingSpeed 1ms
Type `bash deploy/demo/scenarios/orphaned-pvc-no-action/inject-orphan-pvcs.sh >/dev/null 2>&1 && clear && echo PVCS"_"CREATED`
Enter
Wait+Screen@300s /PVCS_CREATED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# 5 orphaned Bound PVCs now exist -- no running pods use them"
Enter
Sleep 2s
Type "kubectl get pvc -n demo-orphaned-pvc"
Enter
Sleep 6s

Type "# Waiting for the KubePersistentVolumeClaimOrphaned alert to fire..."
Enter
Sleep 2s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePersistentVolumeClaimOrphaned --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- orphaned PVCs detected"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubePersistentVolumeClaimOrphaned"
Enter
Sleep 8s

Type "# Kubernaut Gateway received this alert and created a RemediationRequest"
Enter
Sleep 3s

# Hidden wait: poll until RR exists
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q rr-; do sleep 5; done && clear && echo RR"_"CREATED`
Enter
Wait+Screen@120s /RR_CREATED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 2 -- The Pipeline
# ═════════════════════════════════════════

Type "# The pipeline is processing: SP -> AA (HAPI / LLM)"
Enter
Sleep 2s
Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# No remediation workflow is seeded for this alert type"
Enter
Sleep 2s
Type "# The LLM will evaluate and conclude: no automated remediation available"
Enter
Sleep 3s

# Hidden wait: poll until RR reaches Completed or Failed
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo PIPELINE"_"DONE`
Enter
Wait+Screen@600s /PIPELINE_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Pipeline completed -- let's check the outcome"
Enter
Sleep 2s

Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# RemediationRequest outcome:"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.overallPhase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 6s

Type "# AIAnalysis outcome:"
Enter
Sleep 2s
Type `kubectl get aa -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.phase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 6s

Type "# Confirm: no WorkflowExecution was created (correct -- no action needed)"
Enter
Sleep 2s
Type "kubectl get wfe -n ${PLATFORM_NS}"
Enter
Sleep 5s

Type "# Kubernaut gracefully handled an alert with no matching workflow"
Enter
Sleep 2s
Type "# Outcome: NoActionRequired -- no unnecessary remediation was executed"
Enter
Sleep 5s
