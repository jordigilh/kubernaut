# VHS tape: Resource Quota Exhaustion — Narration Screens (Tape C)
#
# Tier 4 Escalation scenario. No workflow execution, no EA, no approval.
# Policy blocks automated remediation. 14 scenes total (0-13).
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
#   8  Policy blocks — escalating + kubectl get rr (8s)
#   9  Root cause + show-ai-analysis  (20s)
#  10  Act 3 announce — ManualReviewRequired (3s)
#  11  ResourceQuota still exhausted  (8s)
#  12  Notification                   (12s)
#  13  Summary                        (15s)
#
# Run: vhs deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion-screens.tape

Output deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Resource Quota Exhaustion — Escalation ━━\e[0m\n\n\e[38;5;245mA deployment exhausts its namespace ResourceQuota.\nPolicy blocks automated remediation.\nKubernaut correctly escalates to human review.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — ResourceQuota exhausted, Prometheus alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mAI Analysis     — AI analyzes but policy blocks automation\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mEscalation      — ManualReviewRequired, platform team notified\e[0m\n'; }`
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
Type "narrate 'Healthy workload — ResourceQuota and pods in namespace demo-quota'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get resourcequota -n demo-quota"
Enter
Sleep 4s

Type "kubectl get pods -n demo-quota"
Enter
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Exhausting ResourceQuota (deployment scales beyond quota limits)...'"
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

Type "bash deploy/demo/scripts/show-alert.sh KubeResourceQuotaExhausted"
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
Type "narrate 'Policy blocks automation — escalating to human operator'"
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
Type "narrate 'Root cause analysis — policy blocks automated quota changes'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-quota"
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
Type "narrate 'ResourceQuota still exhausted — human operator must decide'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get resourcequota -n demo-quota -o wide"
Enter
Sleep 8s

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
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected KubeResourceQuotaExhausted alert from Prometheus\n  • AI analysis identified exhausted ResourceQuota but policy blocks automated changes\n  • Correctly escalated to ManualReviewRequired\n  • Platform team notified via Slack\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
