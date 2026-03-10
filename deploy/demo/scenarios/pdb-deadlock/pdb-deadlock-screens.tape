# VHS tape: PDB Deadlock — Narration Screens (Tape C)
#
# All static narration and analysis screens. Recorded AFTER the scenario has
# fully completed (all CRs still exist in the cluster).
#
# Run: vhs deploy/demo/scenarios/pdb-deadlock/pdb-deadlock-screens.tape
# (normally invoked by record.sh after scenario completion)

Output deploy/demo/scenarios/pdb-deadlock/pdb-deadlock-screens-raw.mp4

Require kubectl
Require bash
Require python3

Set Shell "bash"
Set FontSize 22
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

Type `narrate() { printf '\e[3A\e[J\e[38;5;245m%s\e[0m\n' "$*"; }`
Enter
Sleep 200ms

Type `announce() { printf '\e[3A\e[J\n\e[1;33m━━ %s ━━\e[0m\n\n' "$*"; }`
Enter
Sleep 200ms

Type `title_card() { printf '\e[3A\e[J\n\e[1;33m━━ PDB Deadlock Remediation ━━\e[0m\n\n\e[38;5;245mA PodDisruptionBudget blocks node drain, creating a deadlock.\nKubernaut detects the stuck disruption budget,\nrelaxes the PDB to unblock disruptions, and verifies recovery.\e[0m\n\n\e[1;33mAct 1\e[0m  \e[38;5;245mThe Problem     — PDB blocks drain, alert fires\e[0m\n\e[1;33mAct 2\e[0m  \e[38;5;245mThe Remediation — AI root cause analysis, approval gate, RelaxPDB workflow\e[0m\n\e[1;33mAct 3\e[0m  \e[38;5;245mThe Result      — PDB relaxed, EA confirms fix\e[0m\n'; }`
Enter
Sleep 200ms

Type `export PLATFORM_NS=kubernaut-system`
Enter
Sleep 200ms

Type "clear"
Enter
Sleep 300ms

Type "title_card"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show

# ─────────────────────────────────────────
# Scene 0: Title Card (15s)
# ─────────────────────────────────────────

Sleep 15s

# ─────────────────────────────────────────
# Scene 1: Act 1 — The Problem (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 1: The Problem'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 2: Healthy workload (8s) — with PDB
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Healthy workload running in namespace demo-pdb'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-pdb"
Enter
Sleep 4s

Type "kubectl get pdb -n demo-pdb"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 3: Injecting fault (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Injecting node drain to trigger PDB deadlock...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 4: Alert is now firing (10s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Alert is now firing — Kubernaut deduplicates signals and creates a single remediation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-alert.sh KubePodDisruptionBudgetAtLimit"
Enter
Sleep 10s

# ─────────────────────────────────────────
# Scene 5: Act 2 — The Remediation (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 2: The Remediation'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ─────────────────────────────────────────
# Scene 6: Gateway creates RR (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'The Gateway receives the alert, deduplicates signals, and creates a RemediationRequest'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 7: AI analyzing (5s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'The AI is analyzing the root cause and selecting a remediation workflow...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 5s

# ─────────────────────────────────────────
# Scene 8: Analysis complete + RR status (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Analysis complete. Approval is required because this is a production namespace.'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 9: Root cause analysis (20s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Root cause analysis and recommended workflow'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-ai-analysis.sh demo-pdb"
Enter
Sleep 20s

# ─────────────────────────────────────────
# Scene 10: Approval reason (12s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Checking why approval is required...'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-approval-reason.sh demo-pdb"
Enter
Sleep 12s


# ─────────────────────────────────────────
# Scene 11: RAR created (8s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'A RemediationApprovalRequest was created — the workflow is paused until approved'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get remediationapprovalrequests -n ${PLATFORM_NS}"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 12: Approval granted (4s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Approval granted — the workflow will relax the PDB to unblock disruptions'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 4s

# ─────────────────────────────────────────
# Scene 13: Act 3 — The Result (3s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "announce 'Act 3: The Result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 3s

# ─────────────────────────────────────────
# Scene 14: Resource recovered (8s) — with PDB
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Pods and PDB are now in a healthy state'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "kubectl get pods -n demo-pdb"
Enter
Sleep 4s

Type "kubectl get pdb -n demo-pdb"
Enter
Sleep 8s

# ─────────────────────────────────────────
# Scene 15: Notification (12s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'Notification delivered to Slack and console'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-notification.sh ${PLATFORM_NS}"
Enter
Sleep 12s

# ─────────────────────────────────────────
# Scene 16: Effectiveness assessment (10s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type "narrate 'EffectivenessAssessment result'"
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 500ms

Type "bash deploy/demo/scripts/show-effectiveness.sh demo-pdb"
Enter
Sleep 10s

# ─────────────────────────────────────────
# Scene 17: Summary (15s)
# ─────────────────────────────────────────

Set TypingSpeed 1ms
Type "clear"
Enter
Sleep 300ms
Hide
Type `printf '\e[H\e[J\n\e[1;33m━━ Summary ━━\e[0m\n\n\e[38;5;245m  • Kubernaut detected KubePodDisruptionBudgetAtLimit alert from Prometheus\n  • AI analysis identified: PDB blocking node drain\n  • Selected workflow: RelaxPDB (approval required — production namespace)\n  • After approval, the workflow relaxed the PDB\n  • Users notified via Slack and console\n  • EffectivenessAssessment confirmed: remediation successful\e[0m\n'`
Enter
Sleep 300ms
Set TypingSpeed 50ms
Show
Sleep 15s
