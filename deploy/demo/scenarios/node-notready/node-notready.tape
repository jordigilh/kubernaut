# VHS tape: Node NotReady Remediation (#P4)
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       healthy cluster, node failure, node goes NotReady, alert fires
#   Act 2 -- "The Remediation":   RCA, approval gate, RAR approval, live RR phase transitions
#   Act 3 -- "The Result":        node drained, workloads rescheduled, EA assessment
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed (multi-node)
#   - node-notready workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/node-notready/node-notready.tape
#
# NOTE: Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# Visual hierarchy (color-driven, no symbols):
#   1. Bold yellow ━━ act headers      (announce)  — hidden typing, only output shown
#   2. Dim gray narration              (narrate)   — hidden typing, only output shown
#   3. Cyan ❯ prompt                   (typed commands visible)
#   4. Default foreground              (command output)
#
# Persistent act titles:
#   announce() sets _act; act_clear() does clear + reprints the current act title.
#   Every hidden-wait block uses act_clear before Show so the viewer always knows
#   which act they are in.

Output deploy/demo/scenarios/node-notready/node-notready.gif
Output deploy/demo/scenarios/node-notready/node-notready.mp4

Require kubectl
Require bash
Require python3
Require podman

Set Shell "bash"
Set FontSize 28
Set Width 2400
Set Height 1200
Set Padding 40
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────

Hide
Set TypingSpeed 1ms

# Colored prompt: bright cyan ❯
Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

# Self-erasing helpers: \e[1A moves up, \e[2K erases the typed command line
Type `narrate() { printf '\e[1A\e[2K\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

# announce sets _act so act_clear can re-render it after screen clears
Type `_act=""`
Enter
Sleep 100ms

Type `announce() { _act="$*"; printf '\e[1A\e[2K\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

# title_card: scenario-specific intro rendered as a single self-erasing call
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Node NotReady Remediation ━━\e[0m\n\n\e[38;5;245mA cluster node becomes NotReady, threatening workloads.\nKubernaut detects the unhealthy node, diagnoses the issue with AI,\ncordons and drains the node, and verifies workload recovery.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — node failure, workloads affected, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, approval gate, cordon/drain runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — node drained, workloads rescheduled, EA confirms recovery\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/node-notready/cleanup.sh 2>/dev/null; true && clear && echo CLEAN"_"DONE`
Enter
Wait+Screen@120s /CLEAN_DONE/

Type `kubectl get configmap remediationorchestrator-config -n ${PLATFORM_NS} -o yaml | sed 's/stabilizationWindow: "5m"/stabilizationWindow: "1m"/' | kubectl apply -f - >/dev/null 2>&1 && kubectl rollout restart deploy/remediationorchestrator-controller -n ${PLATFORM_NS} >/dev/null 2>&1 && kubectl rollout status deploy/remediationorchestrator-controller -n ${PLATFORM_NS} --timeout=120s >/dev/null 2>&1 && clear && echo CONFIG"_"DONE`
Enter
Wait+Screen@180s /CONFIG_DONE/

Type `kubectl apply -f deploy/demo/scenarios/node-notready/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/node-notready/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/web-service -n demo-node --timeout=180s >/dev/null 2>&1 && sleep 20 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@600s /SETUP_COMPLETE/

Type "clear"
Enter
Sleep 500ms

# ─────────────────────────────────────────
# Title Card
# ─────────────────────────────────────────
# Single function call — self-erases its command line, outputs styled title card.

Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 20s

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Type "narrate 'A cluster node becomes NotReady, threatening workloads.'"
Enter
Sleep 300ms
Type "narrate 'Node failure is simulated, workloads are affected, and the alert fires.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Healthy cluster — all nodes Ready'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get nodes"
Enter
Sleep 5s

Type "kubectl get pods -n demo-node -o wide"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting fault: simulating node failure (podman pause)...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scenarios/node-notready/inject-node-failure.sh"
Enter
Sleep 4s

# Live pod watch — transition to failure state (Terminating)
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Watching pods transition to failure state...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-node -o wide -w"
Enter
Wait+Screen@120s /Terminating/
Sleep 5s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubeNodeNotReady alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubeNodeNotReady --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh KubeNodeNotReady"
Enter
Sleep 8s

Hide
Set TypingSpeed 1ms
Type "narrate 'The Gateway deduplicates matching signals and creates a single RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'The Gateway ingested the alert and created one RemediationRequest for the owner resource'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# Hidden wait: poll until ANY RR exists
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} --no-headers 2>/dev/null | grep -q .; do sleep 5; done && clear && echo RR"_"EXISTS`
Enter
Wait+Screen@600s /RR_EXISTS/

# ═════════════════════════════════════════
# ACT 2 -- The Remediation
# ═════════════════════════════════════════

Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: The Remediation'"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 1: RR in Analyzing (early AGE)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden time skip — let the LLM work for ~20s without the viewer waiting
Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo SKIP"_"DONE`
Enter
Wait+Screen@30s /SKIP_DONE/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 2: RR still Analyzing (later AGE — shows time has passed)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden wait: skip remaining LLM time to AwaitingApproval
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q AwaitingApproval; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Approval is required because this is a production namespace.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 3: RR now in AwaitingApproval
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-node"
Enter
Sleep 24s

Hide
Set TypingSpeed 1ms
Type "narrate 'Checking why approval is required...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-approval-reason.sh demo-node"
Enter
Sleep 12s

Hide
Set TypingSpeed 1ms
Type "narrate 'Approving the remediation...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

# Live pod transition during remediation
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Watching pods — the workflow will cordon, drain, and reschedule'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-node -o wide -w"
Enter
Sleep 60s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Workflow execution summary'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get workflowexecutions -n ${PLATFORM_NS}"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type "narrate 'Remediation verified, waiting for completion...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS} -w"
Enter
Wait+Screen@300s /Completed/
Sleep 5s
Ctrl+C
Sleep 2s

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Type "narrate 'The cordon/drain completed. Verifying node status and workload recovery.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms recovery.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# Node-specific verification in Act 3
Type "kubectl get nodes"
Enter
Sleep 5s

Type "kubectl get pods -n demo-node -o wide"
Enter
Sleep 5s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Notification delivered to Slack and console'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-notification.sh ${PLATFORM_NS}"
Enter
Sleep 12s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for EffectivenessAssessment to complete (stabilization: 1m)...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Hidden wait: poll until EA reaches Completed or Failed
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get effectivenessassessments -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.phase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo EA"_"DONE`
Enter
Wait+Screen@600s /EA_DONE/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-node"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Node recovered — workloads rescheduled, EA confirms recovery'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
