# VHS tape: CrashLoopBackOff — Pods Stream 2
#
# Focused capture: Pod recovery after remediation workflow rollback.
# Started before approval is granted. Coordinator touches STOP_FILE
# once pods stabilize (shortly after RR reaches Verifying).
#
# Everything is wrapped in a function defined and called in Hide mode.
# The function clears the screen and uses set +m to suppress [1] PID.
# By the time Show activates, only clean kubectl output is on screen.

Output deploy/demo/scenarios/crashloop/recording/crashloop-pods2-raw.mp4

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

Type `__SF="/tmp/kubernaut-crashloop-pods2-stop"`
Enter
Sleep 200ms

Type `__M=PODS2; __M="${__M}_DONE"`
Enter
Sleep 200ms

Type `rm -f "$__SF"`
Enter
Sleep 200ms

Type `__run() { set +m; clear; kubectl get pods -n demo-crashloop -w & __P=$!; while [ ! -f "$__SF" ]; do sleep 2; done; kill $__P 2>/dev/null; wait $__P 2>/dev/null; clear; echo "$__M"; }`
Enter
Sleep 200ms

Type "__run"
Enter
Sleep 2s
Set TypingSpeed 50ms
Show

Wait+Screen@1800s /PODS2_DONE/
