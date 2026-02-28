# VHS tape: Certificate Failure GitOps Remediation (#P17)
#
# Three-act recording:
#   Act 1 -- "The Problem":       bad ClusterIssuer pushed via git, cert becomes NotReady
#   Act 2 -- "The Remediation":   RCA identifies GitOps cert failure, GitRevertCommit workflow
#   Act 3 -- "The Result":        bad commit reverted, ArgoCD syncs, certificate Ready
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Gitea + ArgoCD configured for this scenario
#   - cert-manager installed
#   - cert-failure-gitops workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/cert-failure-gitops/cert-failure-gitops.tape

Output deploy/demo/scenarios/cert-failure-gitops/cert-failure-gitops.gif

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

Type `bash deploy/demo/scenarios/cert-failure-gitops/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

# Full setup: deploy manifests + Gitea repo + ArgoCD app + cert-manager resources
Type `bash deploy/demo/scenarios/cert-failure-gitops/run.sh setup 2>/dev/null && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Certificate Failure -- GitOps Remediation"
Enter
Sleep 3s

Type "# A broken ClusterIssuer is pushed via git, ArgoCD syncs it, cert goes NotReady"
Enter
Sleep 3s

Type "# Healthy certificate in namespace demo-cert-gitops"
Enter
Sleep 2s
Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

Type "# ArgoCD application status:"
Enter
Sleep 2s
Type "kubectl get app demo-cert-gitops -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 5s

Type "# Injecting fault: pushing broken ClusterIssuer via git..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/cert-failure-gitops/run.sh inject"
Enter
Sleep 8s

Type "# Waiting for the CertManagerCertNotReady alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=CertManagerCertNotReady --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- Certificate is not ready"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh CertManagerCertNotReady"
Enter
Sleep 8s

Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

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
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-cert-gitops"
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

Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

Type "# ArgoCD application status:"
Enter
Sleep 2s
Type "kubectl get app demo-cert-gitops -n argocd -o jsonpath='{.status.sync.status}  {.status.health.status}' && echo"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-cert-gitops"
Enter
Sleep 10s

Type "# Certificate restored via GitOps -- bad commit reverted, ArgoCD synced"
Enter
Sleep 5s
