# VHS tape: Network Policy Block Remediation (#P15)
#
# Three-act recording:
#   Act 1 -- "The Problem":       deny-all NetworkPolicy injected, inter-pod traffic blocked
#   Act 2 -- "The Remediation":   RCA identifies deny policy, RemoveNetworkPolicy workflow
#   Act 3 -- "The Result":        deny policy removed, traffic restored
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - network-policy-block workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/network-policy-block/network-policy-block.tape

Output deploy/demo/scenarios/network-policy-block/network-policy-block.gif

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 14
Set Width 1200
Set Height 600
Set Padding 20
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────

Hide
Set TypingSpeed 1ms

Type `export PS1='$ '`
Enter
Sleep 500ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/network-policy-block/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/network-policy-block/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/web-frontend deployment/traffic-gen -n demo-netpol --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@300s /SETUP_COMPLETE/

Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms

Show

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════

Type "# Kubernaut Demo: Network Policy Block Remediation"
Enter
Sleep 3s

Type "# A deny-all NetworkPolicy is injected, blocking inter-pod traffic"
Enter
Sleep 3s

Type "# Healthy workload in namespace demo-netpol"
Enter
Sleep 2s
Type "kubectl get pods -n demo-netpol"
Enter
Sleep 5s

Type "# Injecting fault: applying deny-all NetworkPolicy..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/network-policy-block/inject-deny-all-netpol.sh"
Enter
Sleep 4s

Hide
Set TypingSpeed 1ms
Type `sleep 30 && clear && echo FAULT"_"VISIBLE`
Enter
Wait+Screen@60s /FAULT_VISIBLE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Traffic blocked -- pods may be crashing or failing health checks"
Enter
Sleep 2s
Type "kubectl get pods -n demo-netpol"
Enter
Sleep 5s

Type "kubectl get networkpolicies -n demo-netpol"
Enter
Sleep 5s

Type "# Waiting for the alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodCrashLooping --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; a=json.load(sys.stdin); print(len([x for x in a if x.get(\"labels\",{}).get(\"namespace\")==\"demo-netpol\"]))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
Enter
Sleep 8s

Type "# Kubernaut Gateway received this alert and created a RemediationRequest"
Enter
Sleep 3s

Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'AwaitingApproval|Executing'; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 2 -- The Remediation
# ═════════════════════════════════════════

Type "# The pipeline completed AI-driven analysis"
Enter
Sleep 2s
Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Root cause analysis and recommended workflow:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-netpol"
Enter
Sleep 12s

Type "# Approval required -- approving the RemediationApprovalRequest"
Enter
Sleep 2s
Type "kubectl get rar -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

Type "# Watching the WorkflowExecution live..."
Enter
Sleep 2s
Type "kubectl get wfe -n ${PLATFORM_NS} -w"
Enter
Wait+Screen@120s /Completed/
Sleep 5s
Ctrl+C
Sleep 2s

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Workflow completed -- verifying recovery"
Enter
Sleep 2s

Type "kubectl get networkpolicies -n demo-netpol"
Enter
Sleep 5s

Type "kubectl get pods -n demo-netpol"
Enter
Sleep 5s

Type "kubectl get rr,sp,aa,wfe,ea -n ${PLATFORM_NS}"
Enter
Sleep 6s

Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get effectivenessassessments -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.phase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo EA"_"DONE`
Enter
Wait+Screen@600s /EA_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# EffectivenessAssessment result:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-netpol"
Enter
Sleep 10s

Type "# Network policy removed -- inter-pod traffic restored, pods healthy"
Enter
Sleep 5s
