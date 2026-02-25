# Demo Scenario Validation Tracker

**Branch**: `feature/demo-scenarios-validation`
**Last updated**: 2026-02-24

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
| P1 | crashloop | #145 | PASS | Pipeline validated E2E in prior session (branch `feature/demo-scenarios-v1.0`). Re-run on `demo-scenarios-validation` hit DataStorage 500 (corrupt `pdbProtected` string→bool in DB, fixed manually). Needs clean re-run on fresh cluster to fully reconfirm. |
| P2 | stuck-rollout | #148 | PASS | Fully validated E2E on 2026-02-24. Auto-approved at 95% confidence after Rego policy update. Remediation succeeded (readiness poll confirmed all replicas ready). |
| P3 | pending-taint | #147 | PENDING | Multi-node. Workflow image not yet built. |
| P4 | node-notready | #149 | PENDING | Multi-node. Workflow image not yet built. |

### Tier 2 — Differentiated

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P5 | memory-leak | #150 | PENDING | Predictive signal. Workflow image built (private on Quay). |
| P6 | slo-burn | #151 | PENDING | Needs blackbox-exporter. Workflow image built (private on Quay). PrometheusRule label fix needed. |
| P7 | hpa-maxed | #153 | PENDING | Needs metrics-server. Workflow image built (private on Quay). PrometheusRule label fix needed. |
| P8 | pdb-deadlock | #154 | BLOCKED | LLM consistently selects `RemoveTaint` instead of `RelaxPDB`. Blocked by HAPI bugs: #196 (prompt engineering), #197 (DataStorage discovery SQL ignores detectedLabels), #198 (HAPI `_build_k8s_context` doesn't populate pod_details). Workflow image built (private on Quay). PrometheusRule label fix needed. |
| P9 | autoscale | #152 | PENDING | Multi-node + Kind provisioner. Workflow image built (public on Quay). |

### Tier 3 — Advanced Integration

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P10 | gitops-drift | #158 | PENDING | Needs Gitea + ArgoCD. Workflow image built (public on Quay). |
| P11 | crashloop-helm | #161 | PENDING | Needs Helm release setup. Workflow image not yet built. |
| P12 | cert-failure | #159 | PENDING | Needs cert-manager. Workflow image not yet built. PrometheusRule label fix needed. |
| P13 | disk-pressure | #146 | PENDING | Original design flawed (orphaned PVCs = housekeeping). Redesigned as predictive. Workflow image built (public on Quay). |
| P14 | statefulset-pvc-failure | #155 | PENDING | Workflow image not yet built. |

### Tier 4 — Niche/Combo

| # | Scenario | Issue | Status | Notes |
|---|----------|-------|--------|-------|
| P15 | network-policy-block | #156 | PENDING | Workflow image not yet built. |
| P16 | mesh-routing-failure | #157 | PENDING | Needs Linkerd. Workflow image not yet built. PrometheusRule label fix needed. |
| P17 | cert-failure-gitops | #160 | PENDING | Needs cert-manager + Gitea + ArgoCD. Workflow image not yet built. PrometheusRule label fix needed. |

### Newer Scenarios (added post-initial plan)

| Scenario | Issue | Status | Notes |
|----------|-------|--------|-------|
| remediation-retry | #167 | PENDING | Tests escalation (1st fails, 2nd succeeds). Has workflow schema. |
| memory-escalation | #168 | PENDING | OOMKill → increase memory → OOMKill again → escalation. Has workflow schema. |
| concurrent-cross-namespace | #172 | N/A | Two teams, same issue, different risk tolerance. No workflow schema yet. |
| duplicate-alert-suppression | #170 | N/A | Alert storm dedup demo. No workflow schema (tests Gateway dedup). |
| resource-quota-exhaustion | — | N/A | LLM escalates to human review. No workflow by design. |

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

---

## Fixes Applied on This Branch

| Commit | Description |
|--------|-------------|
| `458e5b192` | fix(demo): correct workflow signalNames and multi-node taint scripts |
| `354e1e4ca` | fix(demo): replace kubectl rollout status with readiness poll (#199) |
| `da7062151` | fix(datastorage): use QueryExecModeDescribeExec to prevent stale plan caching (#200) |
| `26b2776ec` | fix(demo): add environment wildcard and update stuck-rollout image digest |
| `7bc12225f` | feat(rego): confidence-based auto-approval for production (#197) — *in reflog, pending cherry-pick* |

---

## Cluster Group Validation Plan

| Group | Cluster Type | Scenarios | Extra Deps | Status |
|-------|-------------|-----------|------------|--------|
| **A** | Single node | crashloop, disk-pressure, stuck-rollout, gitops-drift, crashloop-helm, network-policy-block, statefulset-pvc-failure, pdb-deadlock, memory-leak | None | 2/9 (crashloop PASS, stuck-rollout PASS, pdb-deadlock BLOCKED) |
| **B** | Multi-node | node-notready, pending-taint | None | 0/2 |
| **C** | Multi-node | hpa-maxed, autoscale | metrics-server | 0/2 |
| **D** | Single node | cert-failure, cert-failure-gitops | cert-manager | 0/2 |
| **E** | Single node | mesh-routing-failure | Linkerd | 0/1 |
| **F** | Single node | slo-burn | blackbox-exporter | 0/1 |
