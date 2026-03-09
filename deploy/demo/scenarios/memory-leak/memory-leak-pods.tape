# VHS tape: Memory Leak — Pod Watcher (Tape A)
#
# Pure resource watcher. Runs kubectl get pods -w continuously.
# No fault injection — memory leak is built into the workload.
#
# Run: vhs deploy/demo/scenarios/memory-leak/memory-leak-pods.tape
# (normally invoked by record.sh, not directly)

Output deploy/demo/scenarios/memory-leak/memory-leak-pods-raw.mp4

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

Type `STOP_FILE="/tmp/kubernaut-memory-leak-resource-stop"`
Enter
Sleep 200ms

Type `resource_watch() { rm -f "$STOP_FILE"; clear; set +m; kubectl get pods -n demo-memory-leak -w & local pid=$!; set -m; while [ ! -f "$STOP_FILE" ]; do sleep 2; done; kill $pid 2>/dev/null; wait $pid 2>/dev/null; clear && echo RESOURCE"_"WATCH"_"DONE; }`
Enter
Sleep 200ms

Type "resource_watch"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Wait+Screen@1800s /RESOURCE_WATCH_DONE/
