# Demo Scenario Validation Tracker

**Branch**: `feature/demo-scenarios-validation`
**Last updated**: 2026-02-27

---

## Validation Status

### Legend

| Status | Meaning |
|--------|---------|
| PASS | Fully validated end-to-end (alert → SP → AIA → WE → EA) |
| PARTIAL | Ran but hit issues; needs clean re-run |
| BLOCKED | Cannot run due to external dependency/bug |
| PENDING | Not yet attempted |
| N/A | No automated workflow (demo-only or dedup scenario) |

---

### Tier 1 — Core Value

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P1 | crashloop | #145 | PASS | Re-validated E2E on 2026-02-26. LLM 0.95 confidence, GracefulRestart rollback restored healthy pods (rev 3). validate.sh 10/10 pass. Fixed run.sh kind-config path. |
| P2 | stuck-rollout | #148 | PASS | Fully validated E2E on 2026-02-24. Auto-approved at 95% confidence after Rego policy update. Remediation succeeded (readiness poll confirmed all replicas ready). |
| P3 | pending-taint | #147 | PASS | Validated E2E on 2026-02-26. Fixed: kind-config path, inject script (taint single target worker only), deployment nodeSelector (demo-taint-target label). LLM 0.90 confidence, RemoveTaint. Auto-approved. WFE removed maintenance taint, pods scheduled successfully. |
| P4 | node-notready | #149 | PASS | Validated E2E on 2026-02-26. Fixed: kind-config path. LLM 0.90 confidence, CordonDrainNode. Auto-approved. WFE cordoned and drained paused worker node. Drain timed out on some evictions (expected, node paused), but pods rescheduled to healthy worker. |

### Tier 2 — Differentiated

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P5 | memory-leak | #150 | PASS | Validated E2E on 2026-02-25. predict_linear fired at ~5min, LLM identified linear memory growth with 0.90 confidence, selected GracefulRestart. Rolling restart reset memory from ~30Mi to baseline. validate.sh 8/8 pass. |
| P6 | slo-burn | #151 | BLOCKED | Validated infra E2E on 2026-02-26: blackbox-exporter installed, Probe CRD label fixed (release: kube-prometheus-stack), image rebuilt+pushed (v1.0.0), environment wildcard added. ErrorBudgetBurn alert fires correctly (100% error rate). Blocked by #217: LLM selects RestartDeployment instead of ProactiveRollback (classifies as config issue, not deployment regression). |
| P7 | hpa-maxed | #153 | PASS | Validated E2E on 2026-02-25. AIAnalysis 0.85 confidence, PatchHPA workflow executed (maxReplicas 3→5). Manual approval required due to #206 (confidence threshold inverted). Slack notifications failed due to #207 (routing not configured). EA completed. |
| P8 | pdb-deadlock | #154 | BLOCKED | LLM consistently selects `RemoveTaint` instead of `RelaxPDB`. Blocked by HAPI bugs: #196 (prompt engineering), #197 (DataStorage discovery SQL ignores detectedLabels), #198 (HAPI `_build_k8s_context` doesn't populate pod_details). Workflow image built (private on Quay). PrometheusRule label fix needed. |
| P9 | autoscale | #152 | PASS | Validated E2E on 2026-02-27. Fixed: provisioner.sh (Kind-specific container config — named volumes for /var, tmpfs for /run+/tmp, security-opt unmask=all, /lib/modules mount, entrypoint /sbin/init, kubeadm join --ignore-preflight-errors=SystemVerification, node registration wait loop), workflow-schema.yaml (signalName FailedScheduling→KubePodSchedulingFailed, environment wildcard, updated bundle tag demo-v1.2), deployment.yaml (memory 512Mi→2Gi to exhaust node), prometheus-rule.yaml (for 2m→1m). LLM 0.85 confidence, ProvisionNode. New Kind worker node joined cluster, pods rescheduled successfully. |

### Tier 3 — Advanced Integration

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P10 | gitops-drift | #158 | PASS | Validated E2E on 2026-02-27. Fixed: remediate.sh (Gitea auth via URL rewriting with credentials from gitea-repo-creds Secret, bash comparison bug in verification loop — multi-line grep output broke integer comparison). ArgoCD v3 annotation-based tracking detection added (#218). LLM 0.85 confidence, GitRevertCommit. Git revert pushed to Gitea, ArgoCD reconciled drift successfully. |
| P11 | crashloop-helm | #161 | PASS | Validated E2E on 2026-02-26, re-validated 2026-02-27. Built workflow image (helm+kubectl+jq). Fixed: kind-config path, signalName (CrashLoopBackOff→KubePodCrashLooping), severity (low→high), environment wildcard, PrometheusRule release label, pod template checksum annotation (trigger restart on ConfigMap change), kubernaut.ai/managed label on pod template, RBAC (namespaces, services, events for Helm operations). LLM 0.85-0.90 confidence, HelmRollback. WFE executed helm rollback (rev 2→3=Rollback to 1). |
| P12 | cert-failure | #159 | PASS | Validated E2E on 2026-02-26, re-validated 2026-02-27. Installed cert-manager v1.19.4 (+ ServiceMonitor with kube-prometheus-stack label). Fixed: signalName (CertificateNotReady→CertManagerCertNotReady), PromQL (namespace→exported_namespace), added namespace label override in alert, environment wildcard, remediate.sh resilient name lookup (secretName fallback). LLM 0.85-0.90 confidence, FixCertificate. WFE recreated CA Secret, cert re-issued. |
| P13 | orphaned-pvc-no-action (was disk-pressure) | #146 | PASS | Renamed from disk-pressure. Validated E2E on 2026-02-25. 5 orphaned PVCs created (housekeeping, not a real issue), LLM correctly dismissed as benign → NoActionRequired. validate.sh 8/8 pass. |
| P14 | statefulset-pvc-failure | #155 | PASS | Validated E2E on 2026-02-26. Fixed: kind-config path, signalName mismatch, PromQL (spec vs status replicas), inject script (broken-storage-class PVC for Kind), remediate.sh (handles non-Bound PVC). LLM 0.85 confidence, FixStatefulSetPVC. Manual approval (bug #206). RBAC patched for PVC perms. Images v1.0.2. |

### Tier 4 — Niche/Combo

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P15 | network-policy-block | #156 | PASS | Validated E2E on 2026-02-26. Built+pushed workflow image (v1.0.0). Fixed: signalName (DeploymentUnavailable→KubePodCrashLooping), added environment wildcard. RBAC patched for networkpolicies delete. LLM 0.85 confidence, FixNetworkPolicy. Manual approval (bug #207). WFE auto-detected+removed deny-all-ingress policy via label selector. EA completed (health=0.75, alert still clearing). |
| P16 | mesh-routing-failure | #157 | PENDING | Needs Linkerd. Workflow image not yet built. PrometheusRule label fix needed. |
| P17 | cert-failure-gitops | #160 | BLOCKED | Validated infra E2E on 2026-02-27: workflow image built+loaded, workflow registered in catalog, PrometheusRule label+PromQL fixed (exported_namespace), CA Secret+Gitea repo+ArgoCD Application deployed, Certificate Ready then broken via bad git commit. LabelDetector correctly detected gitOpsManaged=true, gitOpsTool=argocd. Blocked by #219: LLM selects FixCertificate (direct kubectl) instead of GitRevertCommit (git revert) despite gitOpsManaged=true. Same class as #217. |

### Tier 5 — Newer Scenarios

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P18 | memory-escalation | #168 | BLOCKED | Design gap (#214): CheckConsecutiveFailures only counts Failed RRs, breaks chain on Completed. Scenario expects completed-but-ineffective remediations to trigger escalation, but code treats Completed as success. |
| P19 | concurrent-cross-namespace | #172 | N/A | Two teams, same issue, different risk tolerance. No workflow schema yet. |
| P20 | duplicate-alert-suppression | #170 | BLOCKED | Circular duplicate blocking deadlock (#209): both RRs with same fingerprint block each other. AA completed successfully (GracefulRestart) but RO retroactively blocked the original RR. Reuses crashloop workflow. |
| P21 | resource-quota-exhaustion | #171 | PASS | PromQL fixed: uses ReplicaSet spec vs status metrics instead of pod Pending (pods never created under quota rejection). Alert fires correctly. AA identifies no matching workflow, escalates to ManualReviewRequired. Fixed KSM label leak with `max by(replicaset, namespace)`. |

---

## Pre-requisites Tracker

| Task | Status | Details |
|------|--------|---------|
| Fix PrometheusRule labels (7 scenarios) | IN PROGRESS | hpa-maxed, node-notready, mesh-routing-failure, cert-failure, cert-failure-gitops, pdb-deadlock, slo-burn |
| Investigate workflow image build failure (pending-taint) | PENDING | Previous `build-demo-workflows.sh` failed at scenario 8 |
| Build all exec+schema image pairs | PENDING | ~30-60 min background task. 4 built+public, 4 built+private, 11 not built |
| Make schema image repos public on Quay.io | PENDING | User action required |
| Confidence-based Rego auto-approval | DONE | Implemented and tested. Commit pending (cherry-pick `7bc12225f` after #180 settles). |

---

## Bugs Discovered During Validation

| Issue | Title | Status | Impact |
|-------|-------|--------|--------|
| #196 | HAPI prompt engineering: LLM misclassifies PDB-deadlock | OPEN | Blocks pdb-deadlock scenario |
| #197 | DataStorage discovery SQL ignores detectedLabels | OPEN | Blocks pdb-deadlock scenario |
| #198 | HAPI `_build_k8s_context` doesn't populate pod_details | OPEN | Blocks pdb-deadlock scenario |
| #199 | stuck-rollout remediate.sh reports failure despite success | FIXED | `kubectl rollout status` replaced with readiness poll |
| #200 | DataStorage 500 on workflow listing (pgx cached plan) | FIXED | Switched to `QueryExecModeDescribeExec` |
| — | Corrupt `pdbProtected` string→bool in DB | FIXED | Manual SQL fix. Root cause likely related to #197 |
| — | AIAnalysis `approvalRequired=true` despite high confidence | FIXED | Rego policy updated with confidence-based gating |
| #205 | RO creates EA for Failed/ManualReviewRequired RR with no WFE | OPEN | EA created when no remediation was executed |
| #206 | RO confidence threshold comparison inverted — RAR created at 85% > 80% | OPEN | Auto-approval never triggers; all RRs require manual approval |
| #207 | Notification Slack delivery not enabled / routing ConfigMap missing | OPEN | No Slack notifications sent; requires `notification-routing-config` ConfigMap in `kubernaut-notifications` namespace |
| #209 | RO circular duplicate blocking deadlock — both RRs block each other | OPEN | Deduplication deadlock: RRs with same fingerprint each mark the other as `duplicateOf`, neither progresses. Blocks duplicate-alert-suppression scenario. |
| #212 | Custom labels not scored in workflow discovery (3 gaps in DS pipeline) | OPEN | Schema customLabels not persisted to DB; discovery endpoints ignore custom_labels; SearchByLabels not exposed via API. |
| #214 | RO CheckConsecutiveFailures ignores completed-but-ineffective remediations | OPEN | Blocks memory-escalation scenario (#168). Consecutive failure chain breaks on Completed RR even if remediation was ineffective. |
| #215 | DataStorage wildcard support gaps in severity and detectedLabels | OPEN | `severity` JSONB array missing `OR ? '*'` in SQL; `detectedLabels` string fields have inverted wildcard logic (check query instead of schema). Affects workflow scoring accuracy. |
| #216 | Hardcoded PostgreSQL/Redis credentials in Helm chart values.yaml | OPEN | Plaintext `demo-password` for PostgreSQL, empty password for Redis in `charts/kubernaut/values.yaml`. Should use `existingSecret` pattern. |
| #217 | HAPI prompt engineering: LLM misclassifies SLO burn rate as config issue | OPEN | LLM selects RestartDeployment instead of ProactiveRollback for ErrorBudgetBurn. Fails to correlate ConfigMap swap (new deployment revision) with error spike. Blocks P6 slo-burn. |
| #218 | HAPI LabelDetector doesn't detect ArgoCD v3 annotation-based tracking | FIXED | ArgoCD v3 uses `argocd.argoproj.io/tracking-id` annotation instead of label. LabelDetector updated to check both v2 labels and v3 annotations. Mutual exclusion enforced (argocdV2 vs argocdV3 vs fluxCD). |
| #219 | HAPI prompt engineering: LLM selects FixCertificate instead of GitRevertCommit for GitOps-managed cert-failure | OPEN | LLM ignores gitOpsManaged=true context and chooses direct kubectl fix over git-based fix. Blocks P17. Same class as #217. |

---

## Fixes Applied on This Branch

| Commit | Description |
|--------|-------------|
| `458e5b192` | fix(demo): correct workflow signalNames and multi-node taint scripts |
| `354e1e4ca` | fix(demo): replace kubectl rollout status with readiness poll (#199) |
| `da7062151` | fix(datastorage): use QueryExecModeDescribeExec to prevent stale plan caching (#200) |
| `26b2776ec` | fix(demo): add environment wildcard and update stuck-rollout image digest |
| `7bc12225f` | feat(rego): confidence-based auto-approval for production (#197) — *in reflog, pending cherry-pick* |
| `48ce5d3ee` | feat(hapi): add ArgoCD v3 annotation-based tracking detection (#218) |
| `4b7afbc43` | docs: update DD-HAPI-018 v1.3, BRs for ArgoCD v3 detection (#218) |
| `87ca49ec7` | fix(hotreload): add timer-based debouncing to FileWatcher |
| `f6bc679c6` | fix(demo): tune gitops-drift thresholds and add ArgoCD v3 regression tests |
| `22b9ef959` | fix(demo): validate P9 autoscale scenario end-to-end (#126) |
| `727a220e1` | fix(test): fix e2e and integration test stability issues |

---

## Cluster Group Validation Plan

| Group | Cluster Type | Scenarios | Extra Deps | Status |
|-------|-------------|-----------|------------|--------|
| **A** | Single node | crashloop, orphaned-pvc-no-action, stuck-rollout, gitops-drift, crashloop-helm, network-policy-block, statefulset-pvc-failure, pdb-deadlock, memory-leak | None | 8/9 (crashloop PASS, stuck-rollout PASS, orphaned-pvc-no-action PASS, memory-leak PASS, statefulset-pvc-failure PASS, network-policy-block PASS, crashloop-helm PASS, gitops-drift PASS, pdb-deadlock BLOCKED) |
| **B** | Multi-node | node-notready, pending-taint | None | 2/2 (node-notready PASS, pending-taint PASS) |
| **C** | Multi-node | hpa-maxed, autoscale | metrics-server | 2/2 (hpa-maxed PASS, autoscale PASS) |
| **D** | Single node | cert-failure, cert-failure-gitops | cert-manager | 1/2 (cert-failure PASS, cert-failure-gitops BLOCKED #219) |
| **E** | Single node | mesh-routing-failure | Linkerd | 0/1 |
| **F** | Single node | slo-burn | blackbox-exporter | 0/1 |
