# VHS tape: <SCENARIO_TITLE> — Screens Tape (#<SCENARIO_NUMBER>)
#
# ── THREE-TAPE RECORDING ARCHITECTURE ──────────────────────────────────────
#
# Each scenario is recorded as THREE separate tapes run in parallel by record.sh:
#
#   1. *-screens.tape  (THIS FILE) — narration, analysis, notifications, EA
#      Recorded LAST, after the scenario completes, so all data is available.
#      Produces: <scenario>-screens-raw.mp4
#
#   2. *-rr.tape (or *-rr1..4.tape for focused streams)
#      Watches RemediationRequest phase transitions via 'kubectl get rr -w'.
#      Started by record.sh at appropriate pipeline phases.
#      Produces: <scenario>-rr-raw.mp4 (or rr1..4-raw.mp4)
#
#   3. *-pods.tape (or *-pods1..2.tape for focused streams)
#      Watches Pod/resource state transitions via 'kubectl get pods -w'.
#      Started by record.sh at appropriate times.
#      Produces: <scenario>-pods-raw.mp4 (or pods1..2-raw.mp4)
#
# After recording, splice.conf + splice-demo.sh assemble the final video by
# interleaving scenes from all raw MP4s in narrative order.
#
# ── SCREENS TAPE STRUCTURE ─────────────────────────────────────────────────
#
#   Title Card:                    scenario overview + 3-act roadmap
#   Act 1 -- "The Problem":       narration + show-alert + dedup explanation
#   Act 2 -- "The Remediation":   AI analysis + approval reason + RAR display
#   Act 3 -- "The Result":        pods recovered + notification + EA + summary
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - <ADDITIONAL_PREREQS>
#   - VHS installed (brew install charmbracelet/tap/vhs)
#   - Scenario must have COMPLETED its full pipeline before recording screens
#
# Run: Normally launched by record.sh, not directly.
#   Direct: vhs deploy/demo/scenarios/<SCENARIO_DIR>/<SCENARIO_DIR>-screens.tape
#
# ── SENTINEL PATTERN ───────────────────────────────────────────────────────
#
# Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# ── VISUAL HIERARCHY ───────────────────────────────────────────────────────
#
#   1. Bold yellow ━━ act headers      (announce)  — hidden typing, only output shown
#   2. Dim gray narration              (narrate)   — hidden typing, only output shown
#   3. Cyan ❯ prompt                   (typed commands visible)
#   4. Default foreground              (command output)
#
# Pattern for narrate/announce: Hide → fast-type → Enter → Show.
# The ANSI self-erase codes in the helpers clean the prompt line, but VHS
# captures frames during typing, so we must hide the typing itself.
#
# Persistent act titles:
#   announce() sets _act; act_clear() does clear + reprints the current act title.
#   Use act_clear instead of clear in hidden-wait blocks before Show so the
#   viewer always knows which act they are in.
#
# ── APPROVAL vs AUTO-APPROVE ──────────────────────────────────────────────
#
#   Scenarios in production namespaces wait for AwaitingApproval, show the
#   approval reason, and approve the RAR before watching execution.
#   Auto-approved scenarios wait for Executing, show AI analysis (Approval
#   Required: false), then watch execution directly.

Output deploy/demo/scenarios/<SCENARIO_DIR>/<SCENARIO_DIR>-screens-raw.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 28
Set Width 1600
Set Height 800
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

Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

Type `narrate() { printf '\e[3A\e[J\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `_act=""`
Enter
Sleep 100ms
Type `announce() { _act="$*"; printf '\e[3A\e[J\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

# title_card: scenario-specific intro rendered as a single self-erasing call
# CUSTOMIZE: replace title, description, and act summaries
Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ <SCENARIO_TITLE> ━━\e[0m\n\n\e[38;5;245m<SCENARIO_DESCRIPTION_LINE_1>\n<SCENARIO_DESCRIPTION_LINE_2>\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — <ACT1_SHORT>\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — <ACT2_SHORT>\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — <ACT3_SHORT>\e[0m\n'; }`
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
Type `kubectl apply -f deploy/demo/scenarios/<SCENARIO_DIR>/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/<SCENARIO_DIR>/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/<DEPLOYMENT_NAME> -n <NAMESPACE> --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

Type "clear"
Enter
Sleep 500ms

# ─────────────────────────────────────────
# Title Card
# ─────────────────────────────────────────

Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 20s

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════
# Show the healthy state, inject the fault, and wait for the alert.
# CUSTOMIZE: title, fault injection, and alert name.

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Type "narrate '<ONE_LINE_SCENARIO_DESCRIPTION>'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Healthy workload running in namespace <NAMESPACE>'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n <NAMESPACE>"
Enter
Sleep 5s

# CUSTOMIZE: fault injection step
Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting fault: <FAULT_DESCRIPTION>...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
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
# Type "act_clear"
# Enter
# Sleep 500ms
# Set TypingSpeed 50ms
# Show
#
# Hide
# Set TypingSpeed 1ms
# Type "narrate '<FAULT_VISIBLE_COMMENT>'"
# Enter
# Sleep 300ms
# Set TypingSpeed 50ms
# Show
# Sleep 2s
# Type "kubectl get pods -n <NAMESPACE>"
# Enter
# Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the <ALERT_NAME> alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll AlertManager until the alert fires
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=<ALERT_NAME> --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh <ALERT_NAME>"
Enter
Sleep 8s

Hide
Set TypingSpeed 1ms
Type "narrate 'Kubernaut Gateway received this alert and created a RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# Hidden wait: poll until ANY RR exists
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} --no-headers 2>/dev/null | grep -q .; do sleep 5; done && clear && echo RR"_"EXISTS`
Enter
Wait+Screen@600s /RR_EXISTS/

# ═════════════════════════════════════════
# ACT 2 -- The Remediation
# ═════════════════════════════════════════

Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: The Remediation'"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 1: RR in Analyzing (early AGE)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden time skip — let the LLM work for ~20s without the viewer waiting
Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo SKIP"_"DONE`
Enter
Wait+Screen@30s /SKIP_DONE/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 2: RR still Analyzing (later AGE — shows time has passed)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# ┌─────────────────────────────────────────────────────────┐
# │ APPROVAL PATH (production namespaces)                   │
# │ Use this block when the scenario requires RAR approval. │
# │ Delete the AUTO-APPROVE block below.                    │
# └─────────────────────────────────────────────────────────┘

# Hidden wait: skip remaining LLM time to AwaitingApproval
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q AwaitingApproval; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Approval is required because this is a production namespace.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 3: RR now in AwaitingApproval
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-ai-analysis.sh <NAMESPACE>"
Enter
Sleep 24s

Hide
Set TypingSpeed 1ms
Type "narrate 'Checking why approval is required...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-approval-reason.sh <NAMESPACE>"
Enter
Sleep 12s

# NOTE: Pod transitions are now captured in the *-pods.tape (separate stream).
# The approval action is triggered by record.sh at the appropriate time.
# The screens tape just shows narration about the approval.
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Approval granted — remediation workflow is executing...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ┌─────────────────────────────────────────────────────────────┐
# │ AUTO-APPROVE PATH (non-production namespaces)               │
# │ Use this block when the scenario auto-approves.             │
# │ Delete the APPROVAL PATH block above.                       │
# └─────────────────────────────────────────────────────────────┘
#
# # NOTE: Pod transitions and RR phase transitions are in separate tapes.
# # For auto-approve scenarios, this screens tape just shows:
# #   1. AI analysis (show-ai-analysis.sh)
# #   2. Narration that approval was auto-granted
# # The record.sh coordinator handles timing.
#
# Hide
# Set TypingSpeed 1ms
# Type "act_clear"
# Enter
# Sleep 300ms
# Type "narrate 'Analysis complete. Auto-approved — executing remediation now.'"
# Enter
# Sleep 300ms
# Set TypingSpeed 50ms
# Show
# Sleep 4s
#
# Hide
# Set TypingSpeed 1ms
# Type "act_clear"
# Enter
# Sleep 300ms
# Type "narrate 'Root cause analysis and recommended workflow'"
# Enter
# Sleep 300ms
# Set TypingSpeed 50ms
# Show
# Sleep 2s
#
# Type "bash deploy/demo/scripts/show-ai-analysis.sh <NAMESPACE>"
# Enter
# Sleep 24s

Hide
Set TypingSpeed 1ms
Type "narrate 'Workflow execution summary'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get workflowexecutions -n ${PLATFORM_NS}"
Enter
Sleep 6s

# NOTE: RR phase transitions (Verifying → Completed) are now captured in the
# *-rr.tape (separate stream). The screens tape shows narration only.
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Remediation verified — pipeline complete.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════
# Show the final RR outcome and verify the cluster state.
# CUSTOMIZE the verification steps for your scenario.

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Type "narrate '<RECOVERY_DESCRIPTION>'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# CUSTOMIZE: verification steps (e.g. kubectl get pods, kubectl get certificate)
Type "kubectl get pods -n <NAMESPACE>"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Notification delivered to Slack and console'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-notification.sh ${PLATFORM_NS}"
Enter
Sleep 12s

Hide
Set TypingSpeed 1ms
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-effectiveness.sh <NAMESPACE>"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate '<OUTCOME_SUMMARY>'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
