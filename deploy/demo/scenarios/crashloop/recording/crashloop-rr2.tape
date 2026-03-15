# VHS tape: CrashLoopBackOff — RR Stream 2
#
# Focused capture: Analyzing -> AwaitingApproval
# Started after RR reaches Analyzing. Auto-terminates once
# "AwaitingApproval" appears + 5s for readability.

Output deploy/demo/scenarios/crashloop/recording/crashloop-rr2-raw.mp4

Require kubectl
Require bash

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

Type "clear"
Enter
Sleep 200ms
Set TypingSpeed 50ms
Show

Type `kubectl get remediationrequests -n kubernaut-system -w`
Enter

Wait+Screen@300s /AwaitingApproval/
Sleep 5s
