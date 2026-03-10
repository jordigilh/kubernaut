# VHS tape: Memory Escalation (Multi-Cycle) — Narration Screens (Tape C)
#
# All static narration and analysis screens. Recorded AFTER the scenario has
# fully completed (both RR cycles exist in the cluster). Alert data is read
# from ALERT_CACHE (set by record.sh) since the alert may have resolved.
#
# Each screen is separated by a `clear` command so detect-scenes.sh can find
# scene boundaries for splice-demo.sh.
#
# Screens (scene index):
#   0  Title card                     (15s)
#   1  Act 1 announce                 (4s)
#   2  Healthy workload + kubectl     (8s)
#   3  Narration: no injection needed (4s)
#   4  Alert firing + show-alert      (10s)
#   5  Act 2 announce                 (3s)
#   6  Gateway creates RR             (4s)
#   7  AI analyzing                  (5s)
#   8  Analysis complete + RR status  (8s)
#   9  Root cause + show-ai-analysis  (20s)
#  10  Approval reason                (12s)
#  11  Approval granted — Cycle 1     (4s)
#  12  Cycle 1 result — Completed    (8s)
#  13  Cycle 2 — OOM recurs, new RR   (8s)
#  14  Escalation — ManualReviewReq   (8s)
#  15  Act 3 announce                 (3s)
#  16  All RRs status                 (8s)
#  17  Notification                   (12s)
#  18  Narration: escalation logic   (8s)
#  19  Summary                        (15s)
#
# Run: vhs deploy/demo/scenarios/memory-escalation/memory-escalation-screens.tape
# (normally invoked by record.sh after scenario completion)

Output deploy/demo/scenarios/memory-escalation/memory-escalation-screens-raw.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 22
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

Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

Type `narrate() { printf '\e[3A\e[J\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `announce() { printf '\e[3A\e[J\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Memory Escalation (Multi-Cycle) Demo ━━\e[0m\n\n\e[38;5;245mA workload with a persistent memory leak causes repeated OOM kills.\nCycle 1: Kubernaut increases memory limits (approved).\nCycle 2: OOM recurs — Kubernaut escalates to human review.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mOOM kills begin, first alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mFirst fix applied, leak continues, escalation\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mManualReviewRequired — operator notified\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 200ms

Type "clear"
Enter
Sleep 300ms

Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show

# ─────────────────────────────────────────
# Scene 0: Title Card (15s)
# ─────────────────────────────────────────

Sleep 15s

# ─────────────────────────────────────────
# Scene 1: Act 1 — OOM Kills Begin (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 1: OOM Kills Begin'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 2: Healthy workload (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Healthy workload running in namespace demo-memory-escalation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-memory-escalation"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 3: No injection needed (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'The built-in memory leak will cause OOM kills — no injection needed'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 4: Alert is now firing (10s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Alert is now firing — Kubernaut deduplicates signals and creates a single remediation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-alert.sh ContainerOOMKilling"
Enter
Sleep 10s

# ─────────────────────────────────────────
# Scene 5: Act 2 — The Remediation Cycles (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 2: The Remediation Cycles'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ─────────────────────────────────────────
# Scene 6: Gateway creates RR (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'The Gateway receives the alert, deduplicates signals, and creates a RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 7: AI analyzing (5s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Cycle 1: The AI analyzes and selects a memory limit increase...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# ─────────────────────────────────────────
# Scene 8: Analysis complete + RR status (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Analysis complete. Approval is required because this is a production namespace.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 9: Root cause analysis (20s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-memory-escalation"
Enter
Sleep 20s

# ─────────────────────────────────────────
# Scene 10: Approval reason (12s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Checking why approval is required...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-approval-reason.sh demo-memory-escalation"
Enter
Sleep 12s

# ─────────────────────────────────────────
# Scene 11: Approval granted — Cycle 1 (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Approval granted — Cycle 1 workflow increases memory limits'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 12: Cycle 1 result — Completed (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Cycle 1 complete — memory limits increased. But the leak persists...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 13: Cycle 2 — OOM recurs, new RR (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'The memory leak continues. A second OOM triggers a new RR.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 14: Escalation (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Kubernaut detects consecutive OOM failures and escalates to human review'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 15: Act 3 — ManualReviewRequired (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 3: ManualReviewRequired'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ─────────────────────────────────────────
# Scene 16: All RRs status (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'All RemediationRequests — Cycle 1 Completed, Cycle 2 ManualReviewRequired'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 17: Notification (12s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Notification delivered to Slack and console'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-notification.sh ${PLATFORM_NS}"
Enter
Sleep 12s

# ─────────────────────────────────────────
# Scene 18: Escalation logic narration (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'After consecutive OOM kills with increased limits, Kubernaut correctly escalates to human review rather than looping indefinitely'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 8s

# ─────────────────────────────────────────
# Scene 19: Summary (15s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected ContainerOOMKilling alert from Prometheus\n  • Cycle 1: AI selected IncreaseMemoryLimits workflow (approval required)\n  • After approval, memory limits were raised — but the leak persisted\n  • Cycle 2: Consecutive OOM detected — Kubernaut escalated to ManualReviewRequired\n  • Operator notified — the system avoids infinite remediation loops\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
