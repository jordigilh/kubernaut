# VHS tape: GitOps Drift Remediation (#P10)
#
# Three-act recording:
#   Act 1 -- "The Problem":       bad git commit pushes broken config, ArgoCD syncs it, pods crash
#   Act 2 -- "The Remediation":   RCA identifies GitOps drift, GitRevertCommit workflow
#   Act 3 -- "The Result":        bad commit reverted, ArgoCD syncs healthy state, pods recover
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Gitea + ArgoCD configured for this scenario
#   - gitops-drift workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/gitops-drift/gitops-drift.tape

Output deploy/demo/scenarios/gitops-drift/gitops-drift.gif

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

Type `bash deploy/demo/scenarios/gitops-drift/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

# Full setup: deploy manifests + Gitea repo + ArgoCD app
Type `bash deploy/demo/scenarios/gitops-drift/run.sh setup 2>/dev/null && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: GitOps Drift Remediation"
Enter
Sleep 3s

Type "# A bad git commit pushes broken nginx config -- ArgoCD syncs it, pods crash"
Enter
Sleep 3s

Type "# Healthy workload managed by ArgoCD in namespace demo-gitops"
Enter
Sleep 2s
Type "kubectl get pods -n demo-gitops"
Enter
Sleep 5s

Type "# ArgoCD application status:"
Enter
Sleep 2s
Type "kubectl get app web-frontend -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 5s

Type "# Injecting fault: pushing broken config via git..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/gitops-drift/run.sh inject"
Enter
Sleep 8s

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

Type "# Pods are crashing after ArgoCD synced the broken config"
Enter
Sleep 2s
Type "kubectl get pods -n demo-gitops"
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

Type "# Root cause analysis -- GitOps-aware: detected ArgoCD management"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-gitops"
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

Type "# Watching the WorkflowExecution live (git revert + ArgoCD sync)..."
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

Type "# ArgoCD application status:"
Enter
Sleep 2s
Type "kubectl get app web-frontend -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 5s

Type "kubectl get pods -n demo-gitops"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-gitops"
Enter
Sleep 10s

Type "# GitOps drift remediated -- bad commit reverted, ArgoCD synced healthy state"
Enter
Sleep 5s
