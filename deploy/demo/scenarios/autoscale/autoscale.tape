# VHS tape: Cluster Autoscale Remediation (#P9)
#
# Three-act recording:
#   Act 1 -- "The Problem":       scale-up exceeds node capacity, pods stuck Pending
#   Act 2 -- "The Remediation":   RCA identifies capacity gap, ProvisionNode workflow
#   Act 3 -- "The Result":        new node provisioned, pods scheduled
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - autoscale workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/autoscale/autoscale.tape

Output deploy/demo/scenarios/autoscale/autoscale.gif

Require kubectl
Require bash
Require python3
Require podman

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

Type `bash deploy/demo/scenarios/autoscale/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/autoscale/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/web-cluster -n demo-autoscale --timeout=120s >/dev/null 2>&1 && (nohup bash deploy/demo/scenarios/autoscale/provisioner.sh >/dev/null 2>&1 &) && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Cluster Autoscale Remediation"
Enter
Sleep 3s

Type "# Scaling up the deployment exceeds available node capacity"
Enter
Sleep 3s

Type "# Workload running in namespace demo-autoscale"
Enter
Sleep 2s
Type "kubectl get pods -n demo-autoscale"
Enter
Sleep 5s

Type "kubectl get nodes"
Enter
Sleep 5s

Type "# Injecting fault: scaling deployment to 8 replicas (exceeds node capacity)..."
Enter
Sleep 2s
Type "kubectl scale deployment/web-cluster --replicas=8 -n demo-autoscale"
Enter
Sleep 4s

Hide
Set TypingSpeed 1ms
Type `sleep 15 && clear && echo FAULT"_"VISIBLE`
Enter
Wait+Screen@30s /FAULT_VISIBLE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Some pods are Pending -- not enough node capacity"
Enter
Sleep 2s
Type "kubectl get pods -n demo-autoscale"
Enter
Sleep 5s

Type "# Waiting for the KubePodSchedulingFailed alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodSchedulingFailed --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- pods cannot be scheduled"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubePodSchedulingFailed"
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
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-autoscale"
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
Wait+Screen@180s /Completed/
Sleep 5s
Ctrl+C
Sleep 2s

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Workflow completed -- verifying recovery"
Enter
Sleep 2s

Type "kubectl get nodes"
Enter
Sleep 5s

Type "kubectl get pods -n demo-autoscale"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-autoscale"
Enter
Sleep 10s

Type "# Capacity restored -- new node provisioned, all pods scheduled"
Enter
Sleep 5s
