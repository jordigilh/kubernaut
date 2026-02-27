# VHS tape: CrashLoopBackOff Remediation Demo (#120)
#
# Three-act recording:
#   Act 1 -- "The Problem":       healthy workload, fault injection, CrashLoopBackOff, alert fires
#   Act 2 -- "The Remediation":   RCA, rego policy, RAR approval, live WFE execution
#   Act 3 -- "The Result":        recovery verification, EA assessment with score explanations
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - crashloop workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/crashloop/crashloop.tape
#
# NOTE: Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# Visible annotations use bash comments (# ...) instead of echo to avoid the
# visual artifact where the echo command text and its output both appear.

Output deploy/demo/scenarios/crashloop/crashloop.gif
Output deploy/demo/scenarios/crashloop/crashloop.mp4

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

Type `bash deploy/demo/scenarios/crashloop/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl get configmap remediationorchestrator-config -n ${PLATFORM_NS} -o yaml | sed 's/stabilizationWindow: "5m"/stabilizationWindow: "1m"/' | kubectl apply -f - >/dev/null 2>&1 && kubectl rollout restart deployment/remediationorchestrator-controller -n ${PLATFORM_NS} >/dev/null 2>&1 && kubectl rollout status deployment/remediationorchestrator-controller -n ${PLATFORM_NS} --timeout=60s >/dev/null 2>&1`
Enter
Sleep 30s

Type `kubectl apply -f deploy/demo/scenarios/crashloop/manifests/namespace.yaml -f deploy/demo/scenarios/crashloop/manifests/configmap.yaml -f deploy/demo/scenarios/crashloop/manifests/deployment.yaml -f deploy/demo/scenarios/crashloop/manifests/prometheus-rule.yaml >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/worker -n demo-crashloop --timeout=120s >/dev/null 2>&1 && sleep 20 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: CrashLoopBackOff Remediation"
Enter
Sleep 3s

Type "# Demo timings: alert window shortened to 3m, EA stabilization to 1m"
Enter
Sleep 3s

Type "# Healthy workload running in a production-labeled namespace"
Enter
Sleep 2s
Type "kubectl get pods -n demo-crashloop"
Enter
Sleep 5s

Type "# Injecting invalid nginx config to trigger CrashLoopBackOff..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/crashloop/inject-bad-config.sh"
Enter
Sleep 4s

# Hidden wait for crash to become visible
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
Type "kubectl get pods -n demo-crashloop"
Enter
Sleep 5s

Type "# Waiting for the KubePodCrashLooping alert to fire in AlertManager..."
Enter
Sleep 2s

# Hidden wait: poll until alert appears in AlertManager
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

Type "# Alert is now firing -- querying AlertManager"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
Enter
Sleep 8s

Type "# Kubernaut Gateway received this alert and created a RemediationRequest"
Enter
Sleep 3s

# Hidden wait: poll until RR reaches AwaitingApproval (pipeline completed analysis)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q AwaitingApproval; do sleep 10; done && clear && echo PIPELINE"_"READY`
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
Sleep 3s

Type "kubectl get remediationrequests,signalprocessings,aianalyses -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Root cause analysis and recommended workflow:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-crashloop"
Enter
Sleep 12s

Type "# Approval required -- rego policy matched because environment=production"
Enter
Sleep 3s
Type "kubectl get rar -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Why is approval required? Let's check the rego policy"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-approval-reason.sh demo-crashloop"
Enter
Sleep 12s

Type "# Approving the RemediationApprovalRequest..."
Enter
Sleep 2s
Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

Type "# RAR status after approval:"
Enter
Sleep 2s
Type "kubectl get rar -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# The Remediation Orchestrator will create a WorkflowExecution -- watching live"
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

Type "kubectl get pods -n demo-crashloop"
Enter
Sleep 5s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

Type "# Waiting for the EffectivenessAssessment to complete (stabilization window: 1m)..."
Enter
Sleep 2s

# Hidden wait: poll until EA reaches Completed or Failed
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
Type "bash deploy/demo/scripts/show-effectiveness.sh demo-crashloop"
Enter
Sleep 10s

Type "# Remediation verified -- workload restored, alert cleared"
Enter
Sleep 4s
