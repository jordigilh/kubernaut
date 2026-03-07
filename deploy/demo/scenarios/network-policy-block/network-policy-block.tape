# VHS tape: Network Policy Block Remediation (#P15)
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       healthy workload, deny-all policy injected, connectivity blocked, alert fires
#   Act 2 -- "The Remediation":   RCA, auto-approved (staging), policy removal runs
#   Act 3 -- "The Result":        policy removed, traffic resumes, EA assessment
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - network-policy-block workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/network-policy-block/network-policy-block.tape
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

Output deploy/demo/scenarios/network-policy-block/network-policy-block.gif
Output deploy/demo/scenarios/network-policy-block/network-policy-block.mp4

Require kubectl
Require bash
Require python3

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
Type `narrate() { printf '\e[3A\e[J\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

# announce sets _act so act_clear can re-render it after screen clears
Type `_act=""`
Enter
Sleep 100ms

Type `announce() { _act="$*"; printf '\e[3A\e[J\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

# title_card: scenario-specific intro rendered as a single self-erasing call
Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ Network Policy Block Remediation ━━\e[0m\n\n\e[38;5;245mA deny-all NetworkPolicy blocks all traffic in a staging environment.\nKubernaut detects the connectivity failure, diagnoses the policy with AI,\nremoves the blocking rule, and verifies traffic resumes.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mdeny-all policy applied, connectivity blocked, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mAI root cause analysis, auto-approved, policy removal runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mpolicy removed, traffic resumes, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/network-policy-block/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/namespace.yaml >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/web-frontend deployment/traffic-gen -n demo-netpol --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

Type "act_clear"
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
Type "act_clear"
Enter
Sleep 300ms
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Type "narrate 'A deny-all NetworkPolicy blocks all traffic in a staging environment.'"
Enter
Sleep 300ms
Type "narrate 'Connectivity is blocked, health checks fail, and the alert fires.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Healthy workload running in namespace demo-netpol'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-netpol"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting deny-all NetworkPolicy to block inter-pod traffic...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scenarios/network-policy-block/inject-deny-all-netpol.sh"
Enter
Sleep 4s

# Hidden wait for fault to become visible
Hide
Set TypingSpeed 1ms
Type `sleep 30 && clear && echo FAULT"_"VISIBLE`
Enter
Wait+Screen@60s /FAULT_VISIBLE/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Traffic blocked — pods may be crashing or failing health checks'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-netpol -w"
Enter
Sleep 15s
Ctrl+C
Sleep 2s

Type "kubectl get networkpolicies -n demo-netpol"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for the KubePodCrashLooping alert to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# Hidden wait: poll until alert appears in AlertManager
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodCrashLooping --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; a=json.load(sys.stdin); print(len([x for x in a if x.get(\"labels\",{}).get(\"namespace\")==\"demo-netpol\"]))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
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

Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
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
# ACT 2 -- The Remediation (AUTO-APPROVE PATH)
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

# Hidden wait: skip remaining LLM time to Executing|Verifying|Completed (auto-approve path)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Executing|Verifying|Completed'; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Auto-approved — executing remediation now.'"
Enter
Sleep 300ms
Type "narrate 'Watching the remediation execute...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-netpol -w"
Enter
Sleep 30s
Ctrl+C
Sleep 2s

# AI analysis
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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-netpol"
Enter
Sleep 24s

# Remediation status
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Remediation status'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

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

# Wait for RR completion
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Verifying|Completed'; do sleep 5; done && clear && echo WF"_"DONE`
Enter
Wait+Screen@300s /WF_DONE/
Type "act_clear"
Enter
Sleep 500ms
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
Type "narrate 'The policy removal completed. Verifying traffic resumes and pods are healthy.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms the fix.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get networkpolicies -n demo-netpol"
Enter
Sleep 5s

Type "kubectl get pods -n demo-netpol"
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

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-netpol"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Remediation verified — policy removed, traffic restored, pods healthy'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
