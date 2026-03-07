# VHS tape: Cluster Autoscale Remediation (#P9)
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       scale-up exceeds capacity, pods stuck Pending, alert fires
#   Act 2 -- "The Remediation":   RCA identifies capacity gap, approval gate, ProvisionNode workflow
#   Act 3 -- "The Result":        new node provisioned, pods scheduled, EA confirms fix
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - autoscale workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/autoscale/autoscale.tape
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

Output deploy/demo/scenarios/autoscale/autoscale.gif
Output deploy/demo/scenarios/autoscale/autoscale.mp4

Require kubectl
Require bash
Require python3
Require podman

Set Shell "bash"
Set FontSize 28
Set Width 1600
Set Height 800
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
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Cluster Autoscale Remediation ━━\e[0m\n\n\e[38;5;245mA scale-up exceeds available node capacity, leaving pods Pending.\nKubernaut detects the scheduling failure, diagnoses the capacity gap\nwith AI, provisions a new node, and verifies all pods are scheduled.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — scale-up exceeds capacity, pods stuck Pending, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, approval gate, node provisioning runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — new node provisioned, pods scheduled, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/autoscale/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/autoscale/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/autoscale/manifests/ >/dev/null 2>&1; sleep 3; kubectl apply -f deploy/demo/scenarios/autoscale/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/web-cluster -n demo-autoscale --timeout=120s >/dev/null 2>&1 && (nohup bash deploy/demo/scenarios/autoscale/provisioner.sh >/dev/null 2>&1 &) && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

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
Type "narrate 'Scaling up the deployment exceeds available node capacity.'"
Enter
Sleep 300ms
Type "narrate 'Workload running in namespace demo-autoscale'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Initial state: web-cluster deployment'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-autoscale"
Enter
Sleep 5s

Type "kubectl get nodes"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting fault: scaling deployment to 8 replicas (exceeds node capacity)...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl scale deployment/web-cluster --replicas=8 -n demo-autoscale"
Enter
Sleep 4s

# Live pod watch — transition to failure state (Pending)
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

Type "kubectl get pods -n demo-autoscale -w"
Enter
Wait+Screen@120s /Pending/
Sleep 5s
Ctrl+C
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Node resources are maxed out'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl top nodes"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubePodSchedulingFailed alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodSchedulingFailed --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — pods cannot be scheduled'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh KubePodSchedulingFailed"
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-autoscale"
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

Type "bash deploy/demo/scripts/show-approval-reason.sh demo-autoscale"
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
Type "narrate 'Watching pods — the workflow will provision a new node'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-autoscale -w"
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

# Show RR completing (Verifying → Completed)
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
Type "narrate 'Workflow completed. Verifying new node provisioned and pods scheduled.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms the fix.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get nodes"
Enter
Sleep 5s

Type "kubectl get pods -n demo-autoscale -o wide"
Enter
Sleep 5s

# Hidden wait for metrics to be available on new worker node
Hide
Set TypingSpeed 1ms
Type `while ! kubectl top node 2>/dev/null | grep -q worker; do sleep 5; done && sleep 10 && clear && echo METRICS"_"READY`
Enter
Wait+Screen@120s /METRICS_READY/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Node resources after provisioning'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl top nodes"
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
Type "narrate 'Waiting for EffectivenessAssessment to complete...'"
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

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-autoscale"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Capacity restored — new node provisioned, all pods scheduled'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
