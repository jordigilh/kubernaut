# VHS tape: Memory Escalation (Multi-Cycle) Demo
#
# Three-act recording:
#   Act 1 -- "The Problem":       container OOMKilled repeatedly, alert fires
#   Act 2 -- "The Remediation":   multiple cycles increase limits, eventual escalation to human
#   Act 3 -- "The Result":        escalated to manual review after consecutive failures
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - memory-escalation workflows seeded (IncreaseMemoryLimits + CheckConsecutiveFailures)
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/memory-escalation/memory-escalation.tape

Output deploy/demo/scenarios/memory-escalation/memory-escalation.gif

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

Type `bash deploy/demo/scenarios/memory-escalation/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/memory-escalation/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/ml-worker -n demo-memory-escalation --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Memory Escalation (Multi-Cycle)"
Enter
Sleep 3s

Type "# Container repeatedly OOMKilled -- Kubernaut increases limits then escalates"
Enter
Sleep 3s

Type "# Workload running in namespace demo-memory-escalation"
Enter
Sleep 2s
Type "kubectl get pods -n demo-memory-escalation"
Enter
Sleep 5s

Type "# The ml-worker container has a built-in memory leak that exceeds its 64Mi limit"
Enter
Sleep 2s
Type "# No manual injection needed -- OOMKill will happen naturally"
Enter
Sleep 3s

Type "# Waiting for the ContainerOOMKilling alert to fire..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=ContainerOOMKilling --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Alert is now firing -- container OOMKilled"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh ContainerOOMKilling"
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
# ACT 2 -- The Remediation (Multi-Cycle Escalation)
# ═════════════════════════════════════════

Type "# Cycle 1: Kubernaut attempts to increase memory limits"
Enter
Sleep 2s
Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Root cause analysis:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-memory-escalation"
Enter
Sleep 12s

Type "# Approving Cycle 1..."
Enter
Sleep 2s
Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

# Wait for first cycle to complete -- it will likely fail (OOM recurs)
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo CYCLE1"_"DONE`
Enter
Wait+Screen@600s /CYCLE1_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Cycle 1 result:"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS} -o jsonpath='Phase: {.items[0].status.overallPhase}  Outcome: {.items[0].status.outcome}' && echo`
Enter
Sleep 5s

Type "# Kubernaut detects consecutive failures and escalates to human review"
Enter
Sleep 3s

# Wait for final pipeline state
Hide
Set TypingSpeed 1ms
Type `while true; do RR_COUNT=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{range .items[*]}{.status.overallPhase}{"\n"}{end}' 2>/dev/null | grep -c "Completed\|Failed" || true); [ "$RR_COUNT" -ge "2" ] && break; sleep 10; done && clear && echo ESCALATION"_"DONE`
Enter
Wait+Screen@120s /ESCALATION_DONE/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Type "# Checking all RemediationRequests:"
Enter
Sleep 2s
Type "kubectl get rr -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "kubectl get rr,sp,aa,wfe,ea -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Kubernaut escalation: after consecutive OOMKills with increased limits,"
Enter
Sleep 2s
Type "# the system correctly escalates to human review rather than looping indefinitely"
Enter
Sleep 5s
