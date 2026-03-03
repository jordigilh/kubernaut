# VHS tape: <SCENARIO_TITLE> (#<SCENARIO_NUMBER>)
#
# Three-act recording:
#   Act 1 -- "The Problem":       <ACT1_SUMMARY>
#   Act 2 -- "The Remediation":   <ACT2_SUMMARY>
#   Act 3 -- "The Result":        <ACT3_SUMMARY>
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - <ADDITIONAL_PREREQS>
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/<SCENARIO_DIR>/<SCENARIO_DIR>.tape
#
# NOTE: Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# Visual hierarchy (most prominent → least):
#   1. Bold yellow ━━ act headers      (announce)
#   2. Cyan ❯ prompt                   (typed commands)
#   3. Default foreground              (command output / boxed show-* output)
#   4. Dim gray # narration            (narrate)

Output deploy/demo/scenarios/<SCENARIO_DIR>/<SCENARIO_DIR>.gif
Output deploy/demo/scenarios/<SCENARIO_DIR>/<SCENARIO_DIR>.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 28
Set Width 2400
Set Height 1200
Set Padding 40
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────
# Runs cleanup, deploys manifests, and waits for the workload to become ready.
# Everything here is hidden from the recording.

Hide
Set TypingSpeed 1ms

# Colored prompt: bright cyan ❯ distinguishes command lines from output
Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

# Narration helper: dim gray for contextual annotations
Type `narrate() { printf '\e[38;5;245m# %s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

# Act header helper: bold yellow with separator lines
Type `announce() { printf '\e[1;33m━━ %s ━━\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

# Clean up any previous run
Type `bash deploy/demo/scenarios/<SCENARIO_DIR>/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

# [OPTIONAL] Platform config tweaks for demo speed (e.g. shorter stabilization window)
# Uncomment and adapt if the scenario needs non-default platform settings:
#
# Type `kubectl get configmap remediationorchestrator-config -n ${PLATFORM_NS} -o yaml | sed 's/stabilizationWindow: "5m"/stabilizationWindow: "1m"/' | kubectl apply -f - >/dev/null 2>&1 && kubectl rollout restart deployment/remediationorchestrator-controller -n ${PLATFORM_NS} >/dev/null 2>&1 && kubectl rollout status deployment/remediationorchestrator-controller -n ${PLATFORM_NS} --timeout=60s >/dev/null 2>&1`
# Enter
# Sleep 30s

# Deploy manifests and wait for readiness
# CUSTOMIZE: list all manifest files and the deployment/resource to wait on
Type `kubectl apply -f deploy/demo/scenarios/<SCENARIO_DIR>/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/<DEPLOYMENT_NAME> -n <NAMESPACE> --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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
# Show the healthy state, inject the fault, and wait for the alert.
# CUSTOMIZE: title, fault injection, and alert name.

Type "announce 'Act 1: The Problem — <SCENARIO_TITLE>'"
Enter
Sleep 3s

Type "narrate '<ONE_LINE_SCENARIO_DESCRIPTION>'"
Enter
Sleep 3s

Type "narrate 'Healthy workload running in namespace <NAMESPACE>'"
Enter
Sleep 2s
Type "kubectl get pods -n <NAMESPACE>"
Enter
Sleep 5s

# CUSTOMIZE: fault injection step
Type "narrate 'Injecting fault: <FAULT_DESCRIPTION>...'"
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/<SCENARIO_DIR>/<INJECT_SCRIPT>"
Enter
Sleep 4s

# [OPTIONAL] Hidden wait for the fault to become visible before showing pods
# Uncomment if the fault needs time to propagate (e.g. CrashLoopBackOff):
#
# Hide
# Set TypingSpeed 1ms
# Type `sleep 20 && clear && echo FAULT"_"VISIBLE`
# Enter
# Wait+Screen@60s /FAULT_VISIBLE/
# Type "clear"
# Enter
# Sleep 500ms
# Set TypingSpeed 50ms
# Show
#
# Type "narrate '<FAULT_VISIBLE_COMMENT>'"
# Enter
# Sleep 2s
# Type "kubectl get pods -n <NAMESPACE>"
# Enter
# Sleep 5s

Type "narrate 'Waiting for the <ALERT_NAME> alert to fire...'"
Enter
Sleep 2s

# Hidden wait: poll AlertManager until the alert fires
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=<ALERT_NAME> --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "narrate 'Alert is now firing'"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh <ALERT_NAME>"
Enter
Sleep 8s

Type "narrate 'Kubernaut Gateway received this alert and created a RemediationRequest'"
Enter
Sleep 3s

# ═════════════════════════════════════════
# ACT 2 -- The Remediation
# ═════════════════════════════════════════
# Watch the RemediationRequest transition through phases in real time.
# CUSTOMIZE: add approval steps or AI analysis display as needed.
#
# Common building blocks (copy/uncomment as needed):
#
# --- Show AI analysis details ---
# Type "bash deploy/demo/scripts/show-ai-analysis.sh <NAMESPACE>"
# Enter
# Sleep 12s
#
# --- Show approval reason ---
# Type "bash deploy/demo/scripts/show-approval-reason.sh <NAMESPACE>"
# Enter
# Sleep 8s
#
# --- Show and approve RAR (approval scenarios) ---
# Type "kubectl get rar -n ${PLATFORM_NS}"
# Enter
# Sleep 6s
# Type "bash deploy/demo/scripts/approve-rar.sh"
# Enter
# Sleep 4s

Type "announce 'Act 2: The Remediation'"
Enter
Sleep 2s

Type "narrate 'Watching the RemediationRequest lifecycle...'"
Enter
Sleep 2s

# Watch the RR transition through phases (Analyzing → Executing → Completed)
Type "kubectl get rr -n ${PLATFORM_NS} -w"
Enter

# Hidden wait: poll until the RR reaches a terminal state, then Ctrl+C the watch
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 5; done && sleep 3 && clear && echo RR"_"DONE`
Enter
Wait+Screen@600s /RR_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# CUSTOMIZE: add scenario-specific Act 2 content here (e.g. show-ai-analysis.sh)

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════
# Show the final RR outcome and verify the cluster state.
# CUSTOMIZE the verification steps for your scenario.

Type "announce 'Act 3: The Result'"
Enter
Sleep 2s

Type "narrate 'RemediationRequest outcome:'"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS}`
Enter
Sleep 6s

# CUSTOMIZE: verification steps (e.g. kubectl get pods, show-effectiveness.sh)

# [OPTIONAL] Show EffectivenessAssessment (scenarios with WFE)
# Uncomment if the scenario includes workflow execution + EA:
#
# Type "narrate 'Waiting for the EffectivenessAssessment to complete...'"
# Enter
# Sleep 2s
#
# Hide
# Set TypingSpeed 1ms
# Type `while true; do PHASE=$(kubectl get effectivenessassessments -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.phase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo EA"_"DONE`
# Enter
# Wait+Screen@600s /EA_DONE/
# Type "clear"
# Enter
# Sleep 500ms
# Set TypingSpeed 50ms
# Show
#
# Type "bash deploy/demo/scripts/show-effectiveness.sh <NAMESPACE>"
# Enter
# Sleep 10s

# CUSTOMIZE: final annotation summarizing the outcome
Type "narrate '<OUTCOME_SUMMARY>'"
Enter
Sleep 5s
