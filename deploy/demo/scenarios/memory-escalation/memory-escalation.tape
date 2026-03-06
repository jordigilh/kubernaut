# VHS tape: Memory Escalation (Multi-Cycle) Demo
#
# Three-act recording:
#   Act 1 -- "The Problem":       OOM kills begin, first alert fires
#   Act 2 -- "The Remediation":   first fix applied, leak continues, escalation after repeated failures
#   Act 3 -- "The Result":        ManualReviewRequired — operator notified of persistent issue
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - memory-escalation workflows seeded (IncreaseMemoryLimits + CheckConsecutiveFailures)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/memory-escalation/memory-escalation.tape
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

Output deploy/demo/scenarios/memory-escalation/memory-escalation.gif
Output deploy/demo/scenarios/memory-escalation/memory-escalation.mp4

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
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Memory Escalation (Multi-Cycle) Demo ━━\e[0m\n\n\e[38;5;245mA memory leak triggers repeated OOM kills in production.\nKubernaut attempts automated fixes, detects the pattern of repeated\nfailures, and escalates to a human operator after exhausting retries.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mOOM kills begin, first alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mfirst fix applied, leak continues, escalation after repeated failures\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mManualReviewRequired — operator notified of persistent issue\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/memory-escalation/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/memory-escalation/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/memory-escalation/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/ml-worker -n demo-memory-escalation --timeout=120s >/dev/null 2>&1 && sleep 20 && clear && echo SETUP"_"COMPLETE`
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
# ACT 1 -- OOM kills begin, first alert fires
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: OOM kills begin, first alert fires'"
Enter
Sleep 300ms
Type "narrate 'Workload running in namespace demo-memory-escalation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-memory-escalation"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Watching pods transition to failure state...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-memory-escalation -o wide -w"
Enter
Wait+Screen@120s /OOMKilled/
Sleep 5s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the ContainerOOMKilling alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=ContainerOOMKilling --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — container OOMKilled'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh ContainerOOMKilling"
Enter
Sleep 8s

Hide
Set TypingSpeed 1ms
Type "narrate 'The Gateway deduplicates matching signals and creates a single RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'The Gateway ingested the alert and created one RemediationRequest for the owner resource'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# Hidden wait: poll until RR reaches AwaitingApproval or Executing (first cycle ready)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'AwaitingApproval|Executing'; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 2 -- first fix applied, leak continues, escalation after repeated failures
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: first fix applied, leak continues, escalation after repeated failures'"
Enter
Sleep 300ms
Type "narrate 'Cycle 1: Kubernaut attempts to increase memory limits'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "narrate 'Analysis complete. Approving Cycle 1 — executing remediation now.'"
Enter
Sleep 300ms
Type "narrate 'Watching pods as the remediation executes...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

# Pod transition — shown FIRST so the viewer sees the live change
Type "kubectl get pods -n demo-memory-escalation -o wide -w"
Enter
Sleep 45s
Ctrl+C
Sleep 2s

# AI analysis — explain WHAT the AI recommended (after seeing the visual result)
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-memory-escalation"
Enter
Sleep 24s

# Remediation status
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Remediation status'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

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

# Wait for RR to reach Verifying/Completed (Cycle 1)
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo CYCLE1"_"DONE`
Enter
Wait+Screen@600s /CYCLE1_DONE/
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Cycle 1 verified, waiting for completion...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS} -w"
Enter
Wait+Screen@300s /Completed/
Sleep 5s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Cycle 1 result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type `kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.overallPhase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Kubernaut detects consecutive failures and escalates to human review'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# Hidden wait: poll until escalation complete (2+ RRs or ManualReviewRequired)
Hide
Set TypingSpeed 1ms
Type `while true; do RR_COUNT=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{range .items[*]}{.status.overallPhase}{"\n"}{end}' 2>/dev/null | grep -cE 'Completed|Failed' || true); [ "$RR_COUNT" -ge "2" ] && break; sleep 10; done && clear && echo ESCALATION"_"DONE`
Enter
Wait+Screen@120s /ESCALATION_DONE/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 3 -- ManualReviewRequired — operator notified of persistent issue
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: ManualReviewRequired — operator notified of persistent issue'"
Enter
Sleep 300ms
Type "narrate 'Checking all RemediationRequests'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 6s

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
Type "narrate 'Kubernaut escalation: after consecutive OOMKills with increased limits,'"
Enter
Sleep 300ms
Type "narrate 'the system correctly escalates to human review rather than looping indefinitely'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
