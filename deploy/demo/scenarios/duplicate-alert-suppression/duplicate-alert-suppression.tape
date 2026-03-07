# VHS tape: Duplicate Alert Suppression Demo
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       healthy workload, bad config crashes 5 replicas, alert storm fires
#   Act 2 -- "The Remediation":   Gateway deduplicates to 1 RR, AI analysis, auto-approved rollback
#   Act 3 -- "The Result":        deployment rolled back, pods healthy, effectiveness result
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - duplicate-alert-suppression workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression.tape
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

Output deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression.gif
Output deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression.mp4

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

Hide
Set TypingSpeed 1ms

# Colored prompt: bright cyan ❯
Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

# Self-erasing helpers: \e[3A\e[J moves up 3 lines and clears to end of screen
Type `narrate() { printf '\e[3A\e[J\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

# announce sets _act so act_clear can re-render it after screen clears
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
Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Duplicate Alert Suppression Demo ━━\e[0m\n\n\e[38;5;245mA bad config crashes all 5 replicas, triggering a storm of alerts.\nKubernaut deduplicates the alerts into a single RemediationRequest,\ndiagnoses the root cause with AI, and executes an auto-approved rollback.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mbad config crashes 5 replicas, alert storm fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245malerts deduplicated, AI analysis, auto-approved rollback runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mdeployment rolled back, pods healthy, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/duplicate-alert-suppression/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/duplicate-alert-suppression/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/duplicate-alert-suppression/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/api-gateway -n demo-alert-storm --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

Type "act_clear"
Enter
Sleep 500ms

# ─────────────────────────────────────────
# Title Card
# ─────────────────────────────────────────
# Single function call — self-erases its command line, outputs styled title card.

Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 20s

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Type "narrate 'A bad config crashes all 5 replicas simultaneously.'"
Enter
Sleep 300ms
Type "narrate 'AlertManager fires multiple alerts — an alert storm.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Healthy 5-replica deployment in namespace demo-alert-storm'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-alert-storm"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting broken config to crash all 5 replicas...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scenarios/duplicate-alert-suppression/inject-bad-config.sh"
Enter
Sleep 4s

# Hidden wait for crash to become visible
Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo CRASH"_"VISIBLE`
Enter
Wait+Screen@60s /CRASH_VISIBLE/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'All pods are now in CrashLoopBackOff'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-alert-storm -w"
Enter
Sleep 15s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubePodCrashLooping alerts to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until at least 1 alert appears for demo-alert-storm
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodCrashLooping --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; a=json.load(sys.stdin); print(len([x for x in a if x.get(\"labels\",{}).get(\"namespace\")==\"demo-alert-storm\"]))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Multiple alerts firing from demo-alert-storm namespace'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
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

# Hidden wait: poll until ANY RR exists
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} --no-headers 2>/dev/null | grep -q .; do sleep 5; done && clear && echo RR"_"EXISTS`
Enter
Wait+Screen@600s /RR_EXISTS/

# ═════════════════════════════════════════
# ACT 2 -- The Remediation (AUTO-APPROVE PATH)
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

# Hidden wait: skip remaining LLM time to Executing|Verifying|Completed (auto-approve path)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Executing|Verifying|Completed'; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Auto-approved — executing remediation now.'"
Enter
Sleep 300ms
Type "narrate 'Watching the remediation execute...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-alert-storm -w"
Enter
Sleep 15s
Ctrl+C
Sleep 2s

# AI analysis
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-alert-storm"
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

Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Remediation verified, waiting for completion...'"
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

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Type "narrate 'The rollback completed. Verifying pods are healthy.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms the fix.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get pods -n demo-alert-storm"
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

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-alert-storm"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Remediation verified — single RR fixed all 5 replicas, no alert storm'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
