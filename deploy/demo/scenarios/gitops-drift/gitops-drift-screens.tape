# VHS tape: GitOps Drift — Narration Screens (Tape C)
#
# Auto-approve GitOps scenario. 16 scenes total.
#
# Screens (scene index):
#   0  Title card                     (15s)
#   1  Act 1 announce                 (4s)
#   2  Healthy workload + ArgoCD      (8s)
#   3  Injecting fault via git + show-git-log (8s)
#   4  Alert firing + show-alert      (10s)
#   5  Act 2 announce                 (3s)
#   6  Gateway creates RR             (4s)
#   7  AI analyzing                   (5s)
#   8  Analysis complete, auto-approved + kubectl get rr (8s)
#   9  Root cause + show-ai-analysis  (20s)
#  10  Act 3 announce                 (3s)
#  11  Pods recovered + ArgoCD synced (8s)
#  12  Git log showing revert commit  (8s)
#  13  Notification                   (12s)
#  14  EA result                      (10s)
#  15  Summary                        (15s)
#
# Run: vhs deploy/demo/scenarios/gitops-drift/gitops-drift-screens.tape

Output deploy/demo/scenarios/gitops-drift/gitops-drift-screens-raw.mp4

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

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ GitOps Drift Remediation ━━\e[0m\n\n\e[38;5;245mA bad git commit pushes broken config — ArgoCD syncs it.\nPods crash. Kubernaut detects the drift,\nreverts the bad commit, and verifies ArgoCD resyncs healthy state.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — broken config synced via git, pods crash\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, auto-approved GitRevert workflow\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — pods recovered, ArgoCD synced, EA confirmed\e[0m\n'; }`
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
Type "narrate 'Healthy workload and ArgoCD sync status'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-gitops"
Enter
Sleep 4s

Type "kubectl get app web-frontend -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting broken config via git push...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-git-log.sh 3"
Enter
Sleep 8s

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
Type "narrate 'Analysis complete. Auto-approved — executing GitRevert workflow now.'"
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-gitops"
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
Type "narrate 'Pods recovered and ArgoCD synced'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-gitops"
Enter
Sleep 4s

Type "kubectl get app web-frontend -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 4s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Git log showing revert commit'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-git-log.sh 3"
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

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-gitops"
Enter
Sleep 10s

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected KubePodCrashLooping alert (ArgoCD-synced broken config)\n  • AI analysis identified: bad git commit\n  • Selected workflow: GitRevert (auto-approved)\n  • Workflow pushed revert commit via git\n  • Pods recovered, ArgoCD synced\n  • EffectivenessAssessment confirmed: remediation successful\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
