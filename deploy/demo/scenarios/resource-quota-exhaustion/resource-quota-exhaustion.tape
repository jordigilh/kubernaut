# VHS tape: Resource Quota Exhaustion -- No Action Required Demo
#
# Three-act recording:
#   Act 1 -- "The Problem":       resource quota exhausted, deployments cannot scale
#   Act 2 -- "The Pipeline":      SP enrichment, AI analysis finds policy constraint
#   Act 3 -- "The Result":        RR completed with ManualReviewRequired, escalated to human
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - NO quota expansion workflow seeded (intentional -- policy constraint)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion.tape

Output deploy/demo/scenarios/resource-quota-exhaustion/resource-quota-exhaustion.gif

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

Type `bash deploy/demo/scenarios/resource-quota-exhaustion/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/resource-quota-exhaustion/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/api-server -n demo-quota --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Resource Quota Exhaustion -- Escalation to Human"
Enter
Sleep 3s

Type "# Namespace quota is exhausted -- no automated fix (policy constraint)"
Enter
Sleep 3s

Type "# Workload in namespace demo-quota with a strict ResourceQuota"
Enter
Sleep 2s
Type "kubectl get resourcequota -n demo-quota"
Enter
Sleep 5s

Type "kubectl get pods -n demo-quota"
Enter
Sleep 5s

Type "# Injecting fault: exhausting the resource quota..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/resource-quota-exhaustion/exhaust-quota.sh"
Enter
Sleep 4s

Type "# Waiting for the KubeResourceQuotaExhausted alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubeResourceQuotaExhausted --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- quota exhausted"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubeResourceQuotaExhausted"
Enter
Sleep 8s

Type "# Kubernaut Gateway received this alert and created a RemediationRequest"
Enter
Sleep 3s

# Wait for RR to exist
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q rr-; do sleep 5; done && clear && echo RR"_"CREATED`
Enter
Wait+Screen@120s /RR_CREATED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 2 -- The Pipeline
# ═════════════════════════════════════════

Type "# The pipeline is processing: SP -> AA (HAPI / LLM)"
Enter
Sleep 2s
Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# No quota expansion workflow exists -- this is a policy constraint"
Enter
Sleep 2s
Type "# The LLM will evaluate and conclude: manual review required"
Enter
Sleep 3s

# Wait for RR to reach terminal state
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo PIPELINE"_"DONE`
Enter
Wait+Screen@600s /PIPELINE_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Pipeline completed -- checking the outcome"
Enter
Sleep 2s

Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# RemediationRequest outcome:"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.overallPhase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 6s

Type "# AIAnalysis outcome:"
Enter
Sleep 2s
Type `kubectl get aa -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.phase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 6s

Type "# ResourceQuota status (limits vs used):"
Enter
Sleep 2s
Type "kubectl get resourcequota -n demo-quota -o wide"
Enter
Sleep 6s

Type "# Confirm: no WorkflowExecution was created"
Enter
Sleep 2s
Type "kubectl get wfe -n ${PLATFORM_NS}"
Enter
Sleep 5s

Type "# Kubernaut correctly escalated a policy-constrained issue to human review"
Enter
Sleep 2s
Type "# Outcome: ManualReviewRequired -- quota changes need platform team approval"
Enter
Sleep 5s
