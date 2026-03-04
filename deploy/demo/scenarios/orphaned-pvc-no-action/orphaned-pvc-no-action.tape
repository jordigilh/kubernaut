# VHS tape: No Action Required — Orphaned PVC Demo (#122)
# Derived from: deploy/demo/scenarios/template.tape
#
# Three-act recording:
#   Act 1 -- "The Problem":     healthy workload, orphaned PVCs, alert fires
#   Act 2 -- "The Pipeline":   AI analysis finds no matching workflow, escalation
#   Act 3 -- "The Result":     RR in ManualReviewRequired — human operator notified
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
# Visual hierarchy (color-driven, no symbols):
#   1. Bold yellow ━━ act headers      (announce)  — hidden typing, only output shown
#   2. Dim gray narration              (narrate)   — hidden typing, only output shown
#   3. Cyan ❯ prompt                   (typed commands visible)
#   4. Default foreground              (command output)
#
# Persistent act titles:
#   announce() sets _act; act_clear() does clear + reprints the current act title.
#   Every hidden-wait block uses act_clear before Show so the viewer always knows
#   which act they are in.

Output deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action.gif
Output deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action.mp4

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

Hide
Set TypingSpeed 1ms

# Colored prompt: bright cyan ❯
Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

# Self-erasing helpers: \e[1A moves up, \e[2K erases the typed command line
Type `narrate() { printf '\e[1A\e[2K\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `_act=""`
Enter
Sleep 100ms

Type `announce() { _act="$*"; printf '\e[1A\e[2K\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

# title_card: scenario-specific intro
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ No Action Required — Orphaned PVC Demo ━━\e[0m\n\n\e[38;5;245mOrphaned PVCs are detected, but no automated remedy exists.\nKubernaut detects the issue, analyzes it with AI, determines\nno safe action is available, and escalates to a human operator.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mPVCs orphaned, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mAI analysis, no matching workflow, escalation\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mManualReviewRequired — human operator notified\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/orphaned-pvc-no-action/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/namespace.yaml -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/deployment.yaml -f deploy/demo/scenarios/orphaned-pvc-no-action/manifests/prometheus-rule.yaml >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/data-processor -n demo-orphaned-pvc --timeout=120s >/dev/null 2>&1 && sleep 20 && clear && echo SETUP"_"COMPLETE`
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
# ACT 1 -- PVCs orphaned, alert fires
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: PVCs orphaned, alert fires'"
Enter
Sleep 300ms
Type "narrate 'Healthy workload running in namespace demo-orphaned-pvc'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-orphaned-pvc"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting orphaned PVCs from simulated completed batch jobs...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Hidden: run PVC injection and wait for alert
Hide
Set TypingSpeed 1ms
Type `bash deploy/demo/scenarios/orphaned-pvc-no-action/inject-orphan-pvcs.sh >/dev/null 2>&1 && clear && echo PVCS"_"CREATED`
Enter
Wait+Screen@300s /PVCS_CREATED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate '5 orphaned Bound PVCs now exist — no running pods use them'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pvc -n demo-orphaned-pvc"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubePersistentVolumeClaimOrphaned alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePersistentVolumeClaimOrphaned --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — orphaned PVCs detected'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh KubePersistentVolumeClaimOrphaned"
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

# Hidden wait: poll until RR exists
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} --no-headers 2>/dev/null | grep -q .; do sleep 5; done && clear && echo RR"_"EXISTS`
Enter
Wait+Screen@120s /RR_EXISTS/

# ═════════════════════════════════════════
# ACT 2 -- AI analysis, no matching workflow, escalation
# ═════════════════════════════════════════

Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: AI analysis, no matching workflow, escalation'"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the snapshots and searching for a matching workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 1: RR in Analyzing (early AGE)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden time skip — let the LLM work without the viewer waiting
Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo SKIP"_"DONE`
Enter
Wait+Screen@30s /SKIP_DONE/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the snapshots and searching for a matching workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 2: RR still Analyzing (later AGE — shows time has passed)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden wait: poll until RR reaches ManualReviewRequired or Failed
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'ManualReviewRequired|Failed'; do sleep 10; done && clear && echo PIPELINE"_"DONE`
Enter
Wait+Screen@600s /PIPELINE_DONE/
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'No matching workflow found — escalating to human operator'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 3: RR now in ManualReviewRequired
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-orphaned-pvc"
Enter
Sleep 24s

Hide
Set TypingSpeed 1ms
Type "narrate 'No matching workflow found — escalating to human operator'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ═════════════════════════════════════════
# ACT 3 -- ManualReviewRequired — human operator notified
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: ManualReviewRequired — human operator notified'"
Enter
Sleep 300ms
Type "narrate 'The RR completed with ManualReviewRequired. No workflow was executed.'"
Enter
Sleep 300ms
Type "narrate 'Notification was sent to alert the human operator.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
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
Type "narrate 'Kubernaut gracefully handled an alert with no matching workflow'"
Enter
Sleep 300ms
Type "narrate 'Outcome: ManualReviewRequired — human operator notified for escalation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
