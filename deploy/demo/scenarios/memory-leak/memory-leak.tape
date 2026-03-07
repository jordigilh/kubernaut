# VHS tape: Memory Leak Remediation (#P5)
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       memory leak grows, exhaustion predicted, alert fires,
#                                  dedup explained (2 pods → 1 RR)
#   Act 2 -- "The Remediation":   pod restart shown live, then AI analysis + WE summary
#   Act 3 -- "The Result":        pod restarted, memory reset, EA confirms fix
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - memory-leak workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/memory-leak/memory-leak.tape
#
# NOTE: AUTO-APPROVE PATH (staging namespace, no RAR required)
# Sentinel defenses: string concatenation + clear before echo
#
# Visual hierarchy (color-driven, no symbols):
#   1. Bold yellow ━━ act headers      (announce)  — hidden typing, only output shown
#   2. Dim gray narration              (narrate)   — hidden typing, only output shown
#   3. Cyan ❯ prompt                   (typed commands visible)
#   4. Default foreground              (command output)

Output deploy/demo/scenarios/memory-leak/memory-leak.gif
Output deploy/demo/scenarios/memory-leak/memory-leak.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 28
Set Width 1600
Set Height 800
Set Padding 40
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────

Hide
Set TypingSpeed 1ms

Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

Type `narrate() { printf '\e[1A\e[2K\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `_act=""`
Enter
Sleep 100ms

Type `announce() { _act="$*"; printf '\e[1A\e[2K\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Memory Leak Remediation ━━\e[0m\n\n\e[38;5;245mA leaker sidecar steadily consumes memory in a staging environment.\nPrometheus predicts memory exhaustion using linear extrapolation.\nKubernaut ingests the proactive alert, diagnoses the leak with AI,\nrestarts the pod to reset memory, and verifies the fix.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — memory leak grows, Prometheus predicts exhaustion, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, auto-approved, pod restart runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — pod restarted, memory reset, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 500ms

Type `bash deploy/demo/scenarios/memory-leak/tape-setup.sh && clear && echo SETUP"_"COMPLETE`
Enter
Wait+Screen@600s /SETUP_COMPLETE/

Type "clear"
Enter
Sleep 500ms

# ─────────────────────────────────────────
# Title Card
# ─────────────────────────────────────────

Type `date +%s.%N > /tmp/vhs-marker-video-start`
Enter
Sleep 100ms
Type "clear"
Enter
Sleep 300ms
Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 20s

# ═════════════════════════════════════════
# ACT 1 -- The Problem
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Type "narrate 'A leaker sidecar steadily consumes memory in a staging environment.'"
Enter
Sleep 300ms
Type "narrate 'Prometheus predicts memory exhaustion using linear extrapolation (proactive alert).'"
Enter
Sleep 300ms
Type "narrate 'Kubernaut ingests the alert and remediates automatically.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Workload running in namespace demo-memory-leak'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get pods -n demo-memory-leak"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'The leaker sidecar is already allocating ~1MB every 5 seconds'"
Enter
Sleep 300ms
Type "narrate 'No manual injection needed — the leak is built into the deployment'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for Prometheus predictive alert ContainerMemoryExhaustionPredicted to fire...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=ContainerMemoryExhaustionPredicted --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — memory exhaustion predicted'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh ContainerMemoryExhaustionPredicted"
Enter
Sleep 8s

Hide
Set TypingSpeed 1ms
Type "narrate 'Both pods fire alerts — Kubernaut deduplicates them into a single RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'The Gateway ingested the alert and created one RemediationRequest for the Deployment'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} --no-headers 2>/dev/null | grep -q .; do sleep 5; done && clear && echo RR"_"EXISTS`
Enter
Wait+Screen@600s /RR_EXISTS/

# ═════════════════════════════════════════
# ACT 2 -- The Remediation (AUTO-APPROVE PATH)
# ═════════════════════════════════════════

Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 2: The Remediation'"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 1: RR in Analyzing (early AGE)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden time skip — let the LLM work for ~20s without the viewer waiting
Hide
Set TypingSpeed 1ms
Type `sleep 20 && clear && echo SKIP"_"DONE`
Enter
Wait+Screen@30s /SKIP_DONE/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 2: RR still Analyzing (later AGE — shows time has passed)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

# Hidden wait: skip to Executing (catch early so we see the pod transition live)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Executing|Verifying|Completed'; do sleep 5; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Auto-approved — executing remediation now.'"
Enter
Sleep 300ms
Type "narrate 'Watching pods — the workflow will restart them to clear the leak'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Pod transition — shown FIRST so the viewer sees the live rollout restart
Type "kubectl get pods -n demo-memory-leak -w"
Enter

# Wait for Terminating to appear — rollout restart is happening
Wait+Screen@300s /Terminating/
Sleep 10s

# Exit watch — viewer saw the full transition
Ctrl+C
Sleep 2s

# AI analysis — explain WHAT the AI recommended (after seeing the visual result)
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-memory-leak"
Enter
Sleep 24s

# Remediation status — show RR and WE summaries
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Remediation status'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Workflow execution summary'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get workflowexecutions -n ${PLATFORM_NS}"
Enter
Sleep 6s

# Wait for RR to reach Verifying/Completed (may already be there)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Verifying|Completed'; do sleep 5; done && clear && echo WF"_"DONE`
Enter
Wait+Screen@300s /WF_DONE/
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Remediation verified, waiting for completion...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Show RR completing (Verifying → Completed)
Type "kubectl get remediationrequests -n ${PLATFORM_NS} -w"
Enter
Wait+Screen@300s /Completed/
Sleep 5s
Ctrl+C
Sleep 2s

# ═════════════════════════════════════════
# ACT 3 -- The Result
# ═════════════════════════════════════════

Hide
Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Type "narrate 'The pod restart completed. Verifying memory usage is reset.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms the fix.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get pods -n demo-memory-leak"
Enter
Sleep 5s

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Notification delivered to Slack and console'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-notification.sh ${PLATFORM_NS}"
Enter
Sleep 12s

Hide
Set TypingSpeed 1ms
Type "narrate 'Waiting for EffectivenessAssessment to complete...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Hide
Set TypingSpeed 1ms
Type `while true; do PHASE=$(kubectl get effectivenessassessments -n ${PLATFORM_NS} -o jsonpath='{.items[0].status.phase}' 2>/dev/null); [ "$PHASE" = "Completed" ] || [ "$PHASE" = "Failed" ] && break; sleep 10; done && clear && echo EA"_"DONE`
Enter
Wait+Screen@600s /EA_DONE/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-memory-leak"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Memory leak mitigated — pod restarted, memory usage reset'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
