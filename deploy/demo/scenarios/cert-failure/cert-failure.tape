# VHS tape: Certificate Failure Remediation (#P12)
#
# Three-act recording:
#   Act 1 -- "The Problem":       CA secret deleted, Certificate goes NotReady, alert fires
#   Act 2 -- "The Remediation":   RCA identifies broken issuer, FixCertificate workflow
#   Act 3 -- "The Result":        CA secret recreated, Certificate Ready
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - cert-manager installed
#   - cert-failure workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/cert-failure/cert-failure.tape

Output deploy/demo/scenarios/cert-failure/cert-failure.gif

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

Type `bash deploy/demo/scenarios/cert-failure/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/cert-failure/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/demo-app -n demo-cert-failure --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Certificate Failure Remediation"
Enter
Sleep 3s

Type "# CA secret is deleted, breaking cert-manager Certificate issuance"
Enter
Sleep 3s

Type "# Healthy workload with a valid TLS certificate"
Enter
Sleep 2s
Type "kubectl get certificate -n demo-cert-failure"
Enter
Sleep 5s

Type "# Injecting fault: deleting the CA secret to break the issuer..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/cert-failure/inject-broken-issuer.sh"
Enter
Sleep 4s

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

Type "kubectl get certificate -n demo-cert-failure"
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

Type "# Root cause analysis and recommended workflow:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-cert-failure"
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

Type "kubectl get certificate -n demo-cert-failure"
Enter
Sleep 5s

Type "kubectl get pods -n demo-cert-failure"
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-cert-failure"
Enter
Sleep 10s

Type "# Certificate restored -- CA secret recreated, cert is Ready"
Enter
Sleep 5s
