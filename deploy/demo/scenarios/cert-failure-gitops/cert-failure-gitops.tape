# VHS tape: Certificate Failure GitOps Remediation (#P17)
#
# Recording structure:
#   Title Card:                    scenario overview + 3-act roadmap (20s hold)
#   Act 1 -- "The Problem":       broken ClusterIssuer pushed, cert NotReady, alert fires
#   Act 2 -- "The Remediation":   RCA, auto-approved (no RAR), git revert runs
#   Act 3 -- "The Result":        commit reverted, cert Ready, EA confirms fix
#
# Pre-requisites:
#   - Kind cluster with Kubernaut platform deployed
#   - Gitea + ArgoCD configured for this scenario
#   - cert-manager installed
#   - cert-failure-gitops workflow seeded in DataStorage
#   - VHS installed (brew install charmbracelet/tap/vhs)
#
# Run: vhs deploy/demo/scenarios/cert-failure-gitops/cert-failure-gitops.tape
#
# NOTE: Each sentinel uses TWO defenses:
#   1. String concatenation (e.g. echo SETUP"_"COMPLETE) so the TYPED command text
#      does NOT contain the literal sentinel -- prevents Wait+Screen from matching
#      the command line itself before the command runs.
#   2. `clear` before the echo so the terminal screen contains ONLY the sentinel
#      output -- prevents stale matches from previous screen content.
#
# Visual hierarchy (color-driven, no symbols):
#   1. Bold yellow ━━ act headers      (announce)  — hidden typing, only output shown
#   2. Dim gray narration              (narrate)   — hidden typing, only output shown
#   3. Cyan ❯ prompt                   (typed commands visible)
#   4. Default foreground              (command output)
#
# Persistent act titles:
#   announce() sets _act; act_clear() does clear + reprints the current act title.
#   Every hidden-wait block uses act_clear before Show so the viewer always knows
#   which act they are in.

Output deploy/demo/scenarios/cert-failure-gitops/cert-failure-gitops.gif
Output deploy/demo/scenarios/cert-failure-gitops/cert-failure-gitops.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 28
Set Width 2400
Set Height 1200
Set Padding 40
Set TypingSpeed 50ms
Set Theme "Catppuccin Mocha"

# ─────────────────────────────────────────
# Hidden Setup
# ─────────────────────────────────────────

Hide
Set TypingSpeed 1ms

# Colored prompt: bright cyan ❯
Type `export PS1='\[\e[1;36m\]❯ \[\e[0m\]'`
Enter
Sleep 500ms

Type `export KUBECONFIG=~/.kube/kubernaut-demo-config`
Enter
Sleep 500ms

# Self-erasing helpers: \e[1A moves up, \e[2K erases the typed command line
Type `narrate() { printf '\e[1A\e[2K\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

# announce sets _act so act_clear can re-render it after screen clears
Type `_act=""`
Enter
Sleep 100ms

Type `announce() { _act="$*"; printf '\e[1A\e[2K\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `act_clear() { clear; [ -n "$_act" ] && printf '\e[1;33m━━ %s ━━\e[0m\n\n' "$_act"; }`
Enter
Sleep 200ms

# title_card: scenario-specific intro rendered as a single self-erasing call
Type `title_card() { printf '\e[1A\e[2K\n\e[1;33m━━ Certificate Failure GitOps Remediation ━━\e[0m\n\n\e[38;5;245mA broken ClusterIssuer is pushed via git, ArgoCD syncs it.\nKubernaut detects the cert failure, diagnoses the broken issuer,\nreverts the commit, and ArgoCD restores the healthy state.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — broken ClusterIssuer pushed, cert NotReady, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, auto-approved, git revert runs\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — commit reverted, cert Ready, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

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

# ─────────────────────────────────────────
# Title Card
# ─────────────────────────────────────────
# Single function call — self-erases its command line, outputs styled title card.

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
Type "narrate 'A broken ClusterIssuer is pushed via git, ArgoCD syncs it, cert goes NotReady.'"
Enter
Sleep 300ms
Type "narrate 'Healthy certificate in namespace demo-cert-gitops.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Hide
Set TypingSpeed 1ms
Type "narrate 'Certificate and ArgoCD application status'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

Type "kubectl get app demo-cert-gitops -n argocd -o jsonpath='{.status.sync.status}' && echo"
Enter
Sleep 3s

Hide
Set TypingSpeed 1ms
Type "narrate 'Injecting fault: pushing broken ClusterIssuer via git...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scenarios/cert-failure-gitops/run.sh inject"
Enter
Sleep 4s

# Hidden wait for CertManagerCertNotReady alert to fire
Hide
Set TypingSpeed 1ms
Type `while [ "$(kubectl exec -n monitoring alertmanager-kube-prometheus-stack-alertmanager-0 -- amtool alert query alertname=CertManagerCertNotReady --alertmanager.url=http://localhost:9093 --output=json 2>/dev/null | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null)" = "0" ]; do sleep 10; done && clear && echo ALERT"_"FIRED`
Enter
Wait+Screen@600s /ALERT_FIRED/
Type "act_clear"
Enter
Sleep 500ms
Set TypingSpeed 50ms
Show

Hide
Set TypingSpeed 1ms
Type "narrate 'Alert is now firing — Certificate is not ready'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

Type "bash deploy/demo/scripts/show-alert.sh CertManagerCertNotReady"
Enter
Sleep 8s

Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

Hide
Set TypingSpeed 1ms
Type "narrate 'Kubernaut Gateway received this alert and created a RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# Hidden wait: poll until ANY RR exists
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

# Hidden wait: skip remaining LLM time to Executing|Verifying|Completed (AUTO-APPROVE)
Hide
Set TypingSpeed 1ms
Type `while ! kubectl get remediationrequests -n ${PLATFORM_NS} 2>/dev/null | grep -qE 'Executing|Verifying|Completed'; do sleep 10; done && clear && echo PIPELINE"_"READY`
Enter
Wait+Screen@600s /PIPELINE_READY/
Type "act_clear"
Enter
Sleep 300ms
Type "narrate 'Analysis complete. Auto-approved — no manual approval required.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

# Snapshot 3: RR now in Executing (auto-approved)
Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 6s

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

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-cert-gitops"
Enter
Sleep 24s

# Second watch: captures execution transitions live
# (Executing → Verifying → Completed)
Hide
Set TypingSpeed 1ms
Type "act_clear"
Enter
Sleep 500ms
Type "narrate 'Watching the execution phase transitions...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 2s

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
Type "narrate 'The git revert completed. Verifying certificate is Ready and ArgoCD synced.'"
Enter
Sleep 300ms
Type "narrate 'Notifications were sent, and EffectivenessAssessment confirms the fix.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

Type "kubectl get certificate -n demo-cert-gitops"
Enter
Sleep 5s

Type "kubectl get app demo-cert-gitops -n argocd -o jsonpath='{.status.sync.status}  {.status.health.status}' && echo"
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

# Hidden wait: poll until EA reaches Completed or Failed
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

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-cert-gitops"
Enter
Sleep 10s

Hide
Set TypingSpeed 1ms
Type "narrate 'Certificate restored via GitOps — bad commit reverted, ArgoCD synced'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s
