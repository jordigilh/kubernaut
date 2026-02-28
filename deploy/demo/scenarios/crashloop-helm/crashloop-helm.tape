# VHS tape: CrashLoop Helm Rollback Remediation (#P11)
#
# Three-act recording:
#   Act 1 -- "The Problem":       bad config injected into Helm release, pods crash
#   Act 2 -- "The Remediation":   RCA identifies Helm regression, HelmRollback workflow
#   Act 3 -- "The Result":        Helm rolled back, pods healthy
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - crashloop-helm workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/crashloop-helm/crashloop-helm.tape

Output deploy/demo/scenarios/crashloop-helm/crashloop-helm.gif

Require kubectl
Require bash
Require python3
Require helm

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

Type `bash deploy/demo/scenarios/crashloop-helm/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `helm upgrade --install demo-crashloop-helm deploy/demo/scenarios/crashloop-helm/chart -n demo-crashloop-helm --create-namespace --wait --timeout 120s >/dev/null 2>&1 && kubectl apply -f deploy/demo/scenarios/crashloop-helm/manifests/prometheus-rule.yaml >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: CrashLoop Helm Rollback"
Enter
Sleep 3s

Type "# A bad config upgrade causes Helm-managed pods to crash"
Enter
Sleep 3s

Type "# Helm-managed workload running in namespace demo-crashloop-helm"
Enter
Sleep 2s
Type "kubectl get pods -n demo-crashloop-helm"
Enter
Sleep 5s

Type "# Injecting fault: pushing bad config via Helm upgrade..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/crashloop-helm/inject-bad-config.sh"
Enter
Sleep 4s

Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo CRASH"_"VISIBLE`
Enter
Wait+Screen@60s /CRASH_VISIBLE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Pods are now in CrashLoopBackOff"
Enter
Sleep 2s
Type "kubectl get pods -n demo-crashloop-helm"
Enter
Sleep 5s

Type "# Waiting for the KubePodCrashLooping alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodCrashLooping --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
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
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-crashloop-helm"
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

Type "# Watching the WorkflowExecution live (Helm rollback)..."
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

Type "kubectl get pods -n demo-crashloop-helm"
Enter
Sleep 5s

Type "# Helm release history (should show rollback):"
Enter
Sleep 2s
Type "helm history demo-crashloop-helm -n demo-crashloop-helm"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-crashloop-helm"
Enter
Sleep 10s

Type "# Helm rollback completed -- pods restored to previous healthy revision"
Enter
Sleep 5s
