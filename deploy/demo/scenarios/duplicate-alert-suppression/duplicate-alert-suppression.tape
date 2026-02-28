# VHS tape: Duplicate Alert Suppression Demo
#
# Three-act recording:
#   Act 1 -- "The Problem":       5 replicas crash simultaneously, AlertManager sends 5 alerts
#   Act 2 -- "The Deduplication": Gateway deduplicates to a single RR with occurrenceCount=5
#   Act 3 -- "The Result":        single remediation fixes all pods, no duplicate RRs
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - duplicate-alert-suppression workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression.tape

Output deploy/demo/scenarios/duplicate-alert-suppression/duplicate-alert-suppression.gif

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

Type `bash deploy/demo/scenarios/duplicate-alert-suppression/cleanup.sh 2>/dev/null; true`
Enter
Sleep 5s

Type `kubectl apply -f deploy/demo/scenarios/duplicate-alert-suppression/manifests/ >/dev/null 2>&1 && kubectl wait --for=condition=Available deployment/api-gateway -n demo-alert-storm --timeout=120s >/dev/null 2>&1 && clear && echo SETUP"_"COMPLETE`
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

Type "# Kubernaut Demo: Duplicate Alert Suppression"
Enter
Sleep 3s

Type "# All 5 replicas crash at once -- AlertManager fires 5 alerts"
Enter
Sleep 3s

Type "# Healthy 5-replica deployment in namespace demo-alert-storm"
Enter
Sleep 2s
Type "kubectl get pods -n demo-alert-storm"
Enter
Sleep 5s

Type "# Injecting fault: broken config crashes all 5 replicas..."
Enter
Sleep 2s
Type "bash deploy/demo/scenarios/duplicate-alert-suppression/inject-bad-config.sh"
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

Type "# All pods are now in CrashLoopBackOff"
Enter
Sleep 2s
Type "kubectl get pods -n demo-alert-storm"
Enter
Sleep 5s

Type "# Waiting for the KubePodCrashLooping alerts to fire (expecting 5)..."
Enter
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=KubePodCrashLooping --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; a=json.load(sys.stdin); print(len([x for x in a if x.get(\"labels\",{}).get(\"namespace\")==\"demo-alert-storm\"]))' 2>/dev/null)" -lt "5" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Type "# Multiple alerts firing from demo-alert-storm namespace"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-alert.sh KubePodCrashLooping"
Enter
Sleep 8s

Type "# Kubernaut Gateway received all alerts -- deduplication in progress"
Enter
Sleep 3s

Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -q rr-; do sleep 10; done && sleep 15 && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@300s /PIPELINE_READY/
Type "clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

# ═════════════════════════════════════════
# ACT 2 -- The Deduplication
# ═════════════════════════════════════════

Type "# Gateway deduplicated 5 alerts into a single RemediationRequest"
Enter
Sleep 2s
Type "kubectl get rr -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Only 1 RR created despite 5 alerts -- check occurrenceCount:"
Enter
Sleep 2s
Type `kubectl get rr -n ${PLATFORM_NS} -o jsonpath='{.items[0].spec.occurrenceCount}' && echo " occurrences"`
Enter
Sleep 5s

Type "kubectl get rr,sp,aa -n ${PLATFORM_NS}"
Enter
Sleep 6s

Type "# Root cause analysis:"
Enter
Sleep 2s
Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-alert-storm"
Enter
Sleep 12s

Type "# Approving the single remediation..."
Enter
Sleep 2s
Type "bash deploy/demo/scripts/approve-rar.sh"
Enter
Sleep 4s

# Wait for RR to complete
Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get remediationrequests -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.overallPhase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo RR"_"DONE`
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

Type "# Single remediation fixed all 5 replicas"
Enter
Sleep 2s

Type "kubectl get pods -n demo-alert-storm"
Enter
Sleep 5s

Type "kubectl get rr -n ${PLATFORM_NS}"
Enter
Sleep 5s

Type "# Confirm: only 1 RR exists (not 5)"
Enter
Sleep 2s
Type `echo "Total RRs: $(kubectl get rr -n ${PLATFORM_NS} --no-headers 2>/dev/null | wc -l | tr -d ' ')"`
Enter
Sleep 5s

Type "# Kubernaut deduplicated 5 alerts into 1 RR -- efficient, no alert storm"
Enter
Sleep 5s
