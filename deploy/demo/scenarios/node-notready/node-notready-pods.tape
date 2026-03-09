# VHS tape: Node NotReady — Pod Watcher (Tape A)
#
# Pure resource watcher. Runs kubectl get pods -w with -o wide to capture
# the full pod lifecycle and node column for the node failure scenario.
#
# Run: vhs deploy/demo/scenarios/node-notready/node-notready-pods.tape
# (normally invoked by record.sh, not directly)

Output deploy/demo/scenarios/node-notready/node-notready-pods-raw.mp4

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

Type `STOP_FILE="/tmp/kubernaut-node-notready-resource-stop"`
Enter
Sleep 200ms

# Function runs kubectl -w in background, polls for stop file, then exits cleanly.
Type `resource_watch() { rm -f "$STOP_FILE"; clear; set +m; kubectl get pods -n demo-node -o wide -w & local pid=$!; set -m; while [ ! -f "$STOP_FILE" ]; do sleep 2; done; kill $pid 2>/dev/null; wait $pid 2>/dev/null; clear && echo RESOURCE"_"WATCH"_"DONE; }`
Enter
Sleep 200ms

Type "resource_watch"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Wait+Screen@1800s /RESOURCE_WATCH_DONE/
