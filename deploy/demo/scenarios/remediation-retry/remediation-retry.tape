# VHS tape: Remediation Retry (Two-Cycle) Demo
#
# Three-act recording:
#   Act 1 -- "The Problem":       bad config injected, pods crash
#   Act 2 -- "The Remediation":   Cycle 1 (RestartDeployment) fails, Cycle 2 (RollbackDeployment) succeeds
#   Act 3 -- "The Result":        workload recovered via rollback after failed restart
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Two workflows seeded: restart (will fail) and rollback (will succeed)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/remediation-retry/remediation-retry.tape

Output deploy/demo/scenarios/remediation-retry/remediation-retry.gif

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

Type `bash deploy/demo/scenarios/remediation-retry/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/remediation-retry/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/worker -n demo-remediation-retry --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Remediation Retry (Two-Cycle)"
Enter
Sleep 3s

Type "# When the first remediation fails, Kubernaut retries with an alternative workflow"
Enter
Sleep 3s

Type "# Healthy workload running in namespace demo-remediation-retry"
Enter
Sleep 2s
Type "kubectl get pods -n demo-remediation-retry"
Enter
Sleep 5s

Type "# Injecting fault: bad config to trigger CrashLoopBackOff..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/remediation-retry/inject-bad-config.sh"
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
Type "kubectl get pods -n demo-remediation-retry"
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

# Wait for Cycle 1 to reach AwaitingApproval or Executing
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
# ACT 2 -- The Remediation (Two Cycles)
# ═════════════════════════════════════════

Type "# Cycle 1: first remediation attempt (RestartDeployment)"
Enter
Sleep 2s
Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Root cause analysis -- Cycle 1:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-remediation-retry"
Enter
Sleep 12s

Type "# Approving Cycle 1 RAR..."
Enter
Sleep 2s
Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

Type "# Watching Cycle 1 WorkflowExecution (restart -- expected to fail)..."
Enter
Sleep 2s

# Wait for Cycle 1 to complete (expected: Failed)
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] || [ "$PHASE" = "RetryPending" ] && break; sleep 10; done && clear && echo CYCLE1"_"DONE`
Enter
Wait+Screen@600s /CYCLE1_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Cycle 1 failed -- restart did not fix the config issue"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.overallPhase}' && echo`
Enter
Sleep 5s

Type "# Kubernaut automatically retries with the next available workflow"
Enter
Sleep 3s

# Wait for Cycle 2 RR to appear
Hide
Set TypingSpeed 1ms
Type `sleep 30 && while [ "$(kubectl get remediationrequests -n ${PLATFORM_NS} -o json 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print(len(d.get(\"items\",[])))')" -lt "2" ]; do sleep 10; done && clear && echo CYCLE2"_"READY`
Enter
Wait+Screen@600s /CYCLE2_READY/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Cycle 2: alternative remediation (RollbackDeployment)"
Enter
Sleep 2s
Type "kubectl get rr -n ${PLATFORM_NS}"
Enter
Sleep 6s

# Approve Cycle 2 RAR if needed
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationapprovalrequests -n ${PLATFORM_NS} -o jsonpath='{.items[-1].status.decision}' 2>/dev/null | grep -q Approved; do if kubectl get remediationapprovalrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q Pending; then kubectl patch remediationapprovalrequest $(kubectl get remediationapprovalrequests -n ${PLATFORM_NS} -o jsonpath='{.items[-1].metadata.name}') -n ${PLATFORM_NS} --subresource=status --type=merge -p '{"status":{"decision":"Approved","decidedBy":"demo-operator","decisionMessage":"Approved for demo"}}' 2>/dev/null; break; fi; sleep 5; done && clear && echo CYCLE2"_"APPROVED`
Enter
Wait+Screen@120s /CYCLE2_APPROVED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Cycle 2 approved -- watching the rollback workflow..."
Enter
Sleep 2s

# Wait for Cycle 2 RR to complete
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[-1].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] && break; sleep 10; done && clear && echo RR"_"DONE`
Enter
Wait+Screen@600s /RR_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Cycle 2 completed -- verifying recovery"
Enter
Sleep 2s

Type "kubectl get pods -n demo-remediation-retry"
Enter
Sleep 5s

Type "# Both RemediationRequests:"
Enter
Sleep 2s
Type "kubectl get rr -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "kubectl get rr,sp,aa,wfe,ea -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Kubernaut retry mechanism: first attempt failed, second succeeded"
Enter
Sleep 3s
Type "# Workload recovered via RollbackDeployment after RestartDeployment failed"
Enter
Sleep 5s
