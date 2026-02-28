# VHS tape: StatefulSet PVC Failure Remediation (#P14)
#
# Three-act recording:
#   Act 1 -- "The Problem":       PVC deleted from StatefulSet, replicas mismatch, alert fires
#   Act 2 -- "The Remediation":   RCA identifies PVC issue, RecreatePVC workflow
#   Act 3 -- "The Result":        PVC recreated, StatefulSet replicas restored
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - statefulset-pvc-failure workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/statefulset-pvc-failure/statefulset-pvc-failure.tape

Output deploy/demo/scenarios/statefulset-pvc-failure/statefulset-pvc-failure.gif

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

Type `bash deploy/demo/scenarios/statefulset-pvc-failure/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/statefulset-pvc-failure/manifests/ >/dev/null 2>&1 && kubectl rollout status statefulset/kv-store -n demo-statefulset --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: StatefulSet PVC Failure Remediation"
Enter
Sleep 3s

Type "# A PVC is deleted from a StatefulSet, causing replica mismatch"
Enter
Sleep 3s

Type "# Healthy StatefulSet running in namespace demo-statefulset"
Enter
Sleep 2s
Type "kubectl get pods -n demo-statefulset"
Enter
Sleep 5s

Type "kubectl get pvc -n demo-statefulset"
Enter
Sleep 5s

Type "# Injecting fault: deleting a PVC to cause replica mismatch..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/statefulset-pvc-failure/inject-pvc-issue.sh"
Enter
Sleep 4s

Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo FAULT"_"VISIBLE`
Enter
Wait+Screen@60s /FAULT_VISIBLE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# StatefulSet replicas mismatch -- missing PVC"
Enter
Sleep 2s
Type "kubectl get statefulset -n demo-statefulset"
Enter
Sleep 5s

Type "kubectl get pvc -n demo-statefulset"
Enter
Sleep 5s

Type "# Waiting for the KubeStatefulSetReplicasMismatch alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubeStatefulSetReplicasMismatch --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- StatefulSet replicas mismatch"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubeStatefulSetReplicasMismatch"
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
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-statefulset"
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

Type "kubectl get statefulset -n demo-statefulset"
Enter
Sleep 5s

Type "kubectl get pvc -n demo-statefulset"
Enter
Sleep 5s

Type "kubectl get pods -n demo-statefulset"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-statefulset"
Enter
Sleep 10s

Type "# StatefulSet restored -- PVC recreated, all replicas running"
Enter
Sleep 5s
