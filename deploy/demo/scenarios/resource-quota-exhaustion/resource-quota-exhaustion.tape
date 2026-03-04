# VHS tape: Resource Quota Exhaustion — Escalation to Human
#
# Three-act recording:
#   Act 1 -- "The Problem":     quota exhausted, pods cannot scale, alert fires
#   Act 2 -- "The Pipeline":   AI analysis, policy blocks automation, escalation
#   Act 3 -- "The Result":     RR in ManualReviewRequired — operator notified
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - NO quota expansion workflow seeded (intentional — policy constraint)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion.tape
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

Output deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion.gif
Output deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion.mp4

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
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Resource Quota Exhaustion — Escalation to Human ━━\e[0m\n\n\e[38;5;245mA deployment exhausts its resource quota with no room to scale.\nKubernaut detects the quota exhaustion, analyzes it with AI, determines\nautomated action is blocked by policy, and escalates to a human operator.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mquota exhausted, pods cannot scale, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mAI analysis, policy blocks automation, escalation\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mManualReviewRequired — operator notified\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/resource-quota-exhaustion/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/resource-quota-exhaustion/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/api-server -n demo-quota --timeout=120s >/dev/null 2>&1 && sleep 20 && clear && echo SETUP"_"COMPLETE`
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
# ACT 1 -- quota exhausted, pods cannot scale, alert fires
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: quota exhausted, pods cannot scale, alert fires'"
Enter
Sleep 300ms
Type "narrate 'Workload in namespace demo-quota with a strict ResourceQuota'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get resourcequota -n demo-quota"
Enter
Sleep 5s

Type "kubectl get pods -n demo-quota"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting fault: exhausting the resource quota...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scenarios/resource-quota-exhaustion/exhaust-quota.sh"
Enter
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubeResourceQuotaExhausted alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubeResourceQuotaExhausted --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — quota exhausted'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh KubeResourceQuotaExhausted"
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
# ACT 2 -- AI analysis, policy blocks automation, escalation
# ═════════════════════════════════════════

Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: AI analysis, policy blocks automation, escalation'"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the snapshots and evaluating remediation options...'"
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
Type "narrate 'The AI is analyzing the snapshots and evaluating remediation options...'"
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
Type "narrate 'Policy blocks automation — escalating to human operator'"
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-quota"
Enter
Sleep 24s

Hide
Set TypingSpeed 1ms
Type "narrate 'Policy blocks automation — escalating to human operator'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ═════════════════════════════════════════
# ACT 3 -- ManualReviewRequired — operator notified
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: ManualReviewRequired — operator notified'"
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
Type "narrate 'ResourceQuota status (limits vs used)'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get resourcequota -n demo-quota -o wide"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "narrate 'Kubernaut correctly escalated a policy-constrained issue to human review'"
Enter
Sleep 300ms
Type "narrate 'Outcome: ManualReviewRequired — quota changes need platform team approval'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
