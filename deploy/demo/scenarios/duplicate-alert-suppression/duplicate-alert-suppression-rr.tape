# VHS tape: duplicate-alert-suppression — RR Watcher (Tape B)
#
# Pure RemediationRequest watcher. Captures the full RR lifecycle.
# Started by record.sh BEFORE fault injection. The coordinator creates
# a sentinel file to signal this tape to stop recording cleanly.
#
# Run: vhs deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression-rr.tape
# (normally invoked by record.sh, not directly)

Output deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression-rr-raw.mp4

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

Type `STOP_FILE="/tmp/kubernaut-duplicate-alert-suppression-rr-stop"`
Enter
Sleep 200ms

Type `rr_watch() { rm -f "$STOP_FILE"; clear; set +m; kubectl get remediationrequests -n kubernaut-system -w & local pid=$!; set -m; while [ ! -f "$STOP_FILE" ]; do sleep 2; done; kill $pid 2>/dev/null; wait $pid 2>/dev/null; clear && echo RR"_"WATCH"_"DONE; }`
Enter
Sleep 200ms

Type "rr_watch"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Wait+Screen@1800s /RR_WATCH_DONE/
