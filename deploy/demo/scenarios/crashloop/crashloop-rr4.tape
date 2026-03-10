# VHS tape: CrashLoopBackOff — RR Stream 4
#
# Focused capture: Verifying -> Completed
# Started after RR reaches Verifying. The 720s stabilization window
# means Completed won't appear for ~12 minutes, so timeout is 900s.
# Auto-terminates once "Completed" appears + 5s for readability.

Output deploy/demo/scenarios/crashloop/crashloop-rr4-raw.mp4

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

Wait+Screen@900s /Completed/
Sleep 5s
