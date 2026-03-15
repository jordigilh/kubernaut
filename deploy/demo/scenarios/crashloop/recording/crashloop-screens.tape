# VHS tape: CrashLoopBackOff — Narration Screens (Tape C)
#
# All static narration and analysis screens. Recorded AFTER the scenario has
# fully completed (all CRs still exist in the cluster). Alert data is read
# from ALERT_CACHE (set by record.sh) since the alert may have resolved.
#
# Each screen is separated by a `clear` command so detect-scenes.sh can find
# scene boundaries for splice-demo.sh.
#
# Screens (scene index):
#   0  Title card                     (15s)
#   1  Act 1 announce                 (4s)
#   2  Healthy workload + kubectl     (8s)
#   3  Injecting bad config narration (4s)
#   4  Alert is now firing + show-alert (10s)
#   5  Act 2 announce                 (3s)
#   6  Gateway creates RR             (4s)
#   7  AI analyzing                   (5s)
#   8  Analysis complete narration    (5s)
#   9  Root cause + show-ai-analysis  (20s)
#  10  Approval reason                (12s)
#  11  RAR created + kubectl          (8s)
#  12  Approving narration            (4s)
#  13  Act 3 announce                 (3s)
#  14  Pods recovered + kubectl       (8s)
#  15  Notification                   (12s)
#  16  EA result                      (10s)
#  17  Summary                        (15s)
#
# Run: vhs deploy/demo/scenarios/crashloop/recording/crashloop-screens.tape
# (normally invoked by record.sh after scenario completion)

Output deploy/demo/scenarios/crashloop/recording/crashloop-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ CrashLoopBackOff Remediation ━━\e[0m\n\n\e[38;5;245mA production nginx deployment receives a bad config update.\nKubernaut detects the crash, diagnoses the root cause with AI,\nexecutes a rollback workflow, and verifies the fix.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — bad config crashes pods, Prometheus alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, approval gate, workflow runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — pods recover, effectiveness assessment confirms fix\e[0m\n'; }`
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
# Scene 1: Act 1 — The Problem (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 1: The Problem'"
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
Type "narrate 'Healthy workload running in namespace demo-crashloop'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-crashloop"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 3: Injecting bad config (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting invalid nginx config to trigger CrashLoopBackOff...'"
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

Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
Enter
Sleep 10s

# ─────────────────────────────────────────
# Scene 5: Act 2 — The Remediation (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 2: The Remediation'"
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
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# ─────────────────────────────────────────
# Scene 8: Analysis complete (5s)
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
Sleep 5s

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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-crashloop"
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

Type "bash deploy/demo/scripts/show-approval-reason.sh demo-crashloop"
Enter
Sleep 12s

# ─────────────────────────────────────────
# Scene 11: RAR created (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'A RemediationApprovalRequest was created — the workflow is paused until approved'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationapprovalrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 12: Approving (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Approval granted — the workflow will rollback to the previous good config'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 13: Act 3 — The Result (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ─────────────────────────────────────────
# Scene 14: Pods recovered (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'All pods are now running with the corrected configuration'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-crashloop"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 15: Notification (12s)
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
# Scene 16: Effectiveness assessment (10s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-crashloop"
Enter
Sleep 10s

# ─────────────────────────────────────────
# Scene 17: Summary (15s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected a KubePodCrashLooping alert from Prometheus\n  • AI analysis identified: CrashLoopBackOff caused by invalid nginx config\n  • Selected workflow: RollbackDeployment (approval required — production namespace)\n  • After approval, the workflow rolled back to the previous healthy revision\n  • Users notified via Slack and console\n  • EffectivenessAssessment confirmed: remediation successful\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
