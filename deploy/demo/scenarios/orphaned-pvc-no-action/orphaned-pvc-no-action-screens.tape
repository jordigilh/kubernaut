# VHS tape: Orphaned PVC No Action — Narration Screens (Tape C)
#
# Tier 4 Escalation scenario. No workflow execution, no EA, no approval.
# 13 scenes total (0-12).
#
# Screens (scene index):
#   0  Title card                     (15s)
#   1  Act 1 announce                 (4s)
#   2  Healthy workload + kubectl     (8s)
#   3  Injecting fault narration      (4s)
#   4  Alert firing + show-alert      (10s)
#   5  Act 2 announce — AI Analysis & Escalation (3s)
#   6  Gateway creates RR             (4s)
#   7  AI analyzing                   (5s)
#   8  No workflow found — escalating + kubectl get rr (8s)
#   9  Root cause + show-ai-analysis  (20s)
#  10  Act 3 announce — ManualReviewRequired (3s)
#  11  Notification                   (12s)
#  12  Summary                        (15s)
#
# Run: vhs deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action-screens.tape

Output deploy/demo/scenarios/orphaned-pvc-no-action/orphaned-pvc-no-action-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Orphaned PVC — No Action Required ━━\e[0m\n\n\e[38;5;245mOrphaned PVCs are detected after batch jobs complete.\nKubernaut finds no matching workflow and correctly\nescalates to human review rather than taking risky action.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — orphaned PVCs detected, Prometheus alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mAI Analysis     — AI analyzes but finds no safe workflow\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mEscalation      — ManualReviewRequired, operator notified\e[0m\n'; }`
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

Sleep 15s

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

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Healthy workload — PVCs and pods in namespace demo-orphaned-pvc'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pvc -n demo-orphaned-pvc"
Enter
Sleep 4s

Type "kubectl get pods -n demo-orphaned-pvc"
Enter
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting orphaned PVCs (batch jobs completed, PVCs left behind)...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Alert is now firing — Kubernaut deduplicates signals and creates a RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-alert.sh KubePersistentVolumeClaimOrphaned"
Enter
Sleep 10s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 2: AI Analysis & Escalation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

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

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'No matching workflow found — escalating to human operator'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Root cause analysis — no safe workflow available'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-orphaned-pvc"
Enter
Sleep 20s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 3: ManualReviewRequired — Operator Notified'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

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

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected KubePersistentVolumeClaimOrphaned alert from Prometheus\n  • AI analysis identified orphaned PVCs but found no safe workflow\n  • Correctly escalated to ManualReviewRequired\n  • Operator notified via Slack\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
