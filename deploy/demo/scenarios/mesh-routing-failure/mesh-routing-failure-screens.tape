# VHS tape: Mesh Routing Failure — Narration Screens (Tape C)
#
# Tier 2 AUTO-APPROVE scenario. 15 scenes (no approval scenes).
#
# Run: vhs deploy/demo/scenarios/mesh-routing-failure/mesh-routing-failure-screens.tape

Output deploy/demo/scenarios/mesh-routing-failure/mesh-routing-failure-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Mesh Routing Failure Remediation ━━\e[0m\n\n\e[38;5;245mA deny AuthorizationPolicy blocks service mesh traffic.\nKubernaut detects the high error rate,\nremoves the deny policy, and verifies mesh traffic resumes.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — deny policy blocks mesh, high error rate, Prometheus alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, auto-approved workflow runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — policy removed, mesh traffic resumes, EA confirms fix\e[0m\n'; }`
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
Type "narrate 'Healthy workload running in namespace demo-mesh-failure'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-mesh-failure"
Enter
Sleep 4s

Type "kubectl get authorizationpolicies -n demo-mesh-failure 2>/dev/null || echo 'No AuthorizationPolicies'"
Enter
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting deny AuthorizationPolicy to block mesh traffic...'"
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
Type "narrate 'Alert is now firing — Kubernaut deduplicates signals and creates a single remediation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-alert.sh LinkerdHighErrorRate"
Enter
Sleep 10s

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
Type "narrate 'Analysis complete. Auto-approved — executing remediation now.'"
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
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-mesh-failure"
Enter
Sleep 20s

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

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Deny policy removed and mesh traffic has resumed'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-mesh-failure"
Enter
Sleep 4s

Type "kubectl get authorizationpolicies -n demo-mesh-failure 2>/dev/null || echo 'No AuthorizationPolicies (removed)'"
Enter
Sleep 4s

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
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-mesh-failure"
Enter
Sleep 10s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected LinkerdHighErrorRate alert from Prometheus\n  • AI analysis identified: deny AuthorizationPolicy blocking mesh traffic\n  • Selected workflow: RemoveAuthPolicy (auto-approved)\n  • Workflow removed the deny policy\n  • Users notified via Slack and console\n  • EffectivenessAssessment confirmed: remediation successful\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
