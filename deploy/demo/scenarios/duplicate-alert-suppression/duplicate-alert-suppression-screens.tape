# VHS tape: Duplicate Alert Suppression — Narration Screens (Tape C)
#
# Auto-approve scenario: no approval scenes. 15 scenes total.
# Emphasis on deduplication: 5 alerts -> 1 RR.
#
# Run: vhs deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression-screens.tape

Output deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Duplicate Alert Suppression Demo ━━\e[0m\n\n\e[38;5;245mA bad config crashes all 5 replicas simultaneously.\nMultiple alerts fire — Kubernaut deduplicates them\ninto a single RemediationRequest and fixes all pods.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — bad config, 5 crash loops, multiple alerts fire\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — Gateway deduplicates, AI analysis, auto-approved workflow\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — single RR fixes all, EA confirms, no alert storm\e[0m\n'; }`
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
Type "narrate 'Healthy workload with 5 replicas in namespace demo-alert-storm'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-alert-storm"
Enter
Sleep 8s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting bad config to crash all 5 replicas simultaneously...'"
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
Type "narrate 'Multiple alerts firing — Kubernaut deduplicates into a single RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
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
Type "narrate 'The Gateway receives multiple alerts, deduplicates into one RR, and creates a RemediationRequest'"
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-alert-storm"
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
Type "narrate 'All pods recovered — single RR fixed all 5 replicas'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-alert-storm"
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
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-alert-storm"
Enter
Sleep 10s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected multiple KubePodCrashLooping alerts from 5 replicas\n  • Gateway deduplicated into single RemediationRequest\n  • AI analysis identified: bad config causing crash loops\n  • Selected workflow: RollbackDeployment (auto-approved)\n  • Workflow rolled back — all pods recovered\n  • EffectivenessAssessment confirmed: remediation successful — no alert storm\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
