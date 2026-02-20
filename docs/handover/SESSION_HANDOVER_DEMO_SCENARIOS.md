# Session Handover: Demo Scenarios (End-to-End Remediation Demos)

**Date**: 2026-02-20 (updated)  
**Branch**: `feat/effectiveness-monitor-level1-v1.0`  
**Status**: 17 scenarios implemented (11 original + 6 new label/CRD scenarios). Workflow OCI images have real SHA256 digests. None end-to-end tested yet.  
**Parent GitHub Issue**: #114 (Demo Scenarios umbrella)

---

## 1. What This Is

A set of end-to-end demo scenarios that showcase Kubernaut's full remediation pipeline against realistic SRE problems. Each scenario runs locally on **Kind + Podman with a real LLM** (not CI mock). The flow is always:

```
Prometheus Alert → Gateway → Signal Processing → AI Analysis (HAPI + LLM) → Remediation Orchestrator → Workflow Execution → Effectiveness Monitor
```

Each scenario provides:
- Kubernetes manifests (Namespace, Deployment, Service, ConfigMap, PrometheusRule)
- A workflow OCI bundle (Dockerfile + `workflow-schema.yaml` + `remediate.sh`)
- A `run.sh` script for one-command automated execution
- A `README.md` with manual step-by-step instructions and BDD specification
- A `cleanup.sh` script to tear down resources

---

## 2. Environment Constraints

| Resource | Budget |
|----------|--------|
| Memory | 12 GB total |
| CPUs | 11 |
| PV space | ≥10 GB |
| Container runtime | Podman |
| Cluster | Kind (multi-node) |
| LLM | Real (not mock) |

The Kind cluster config is at `deploy/demo/overlays/kind/kind-cluster-config.yaml` and includes:
- 1 control-plane node (with port mappings for Gateway, DataStorage, Prometheus, AlertManager, Grafana)
- 1 worker node (labeled `kubernaut.ai/workload-node: "true"`)

---

## 3. Originally Implemented Scenarios (detailed writeup)

### 3.1 Scenario #125: GitOps Drift Remediation

| Attribute | Value |
|-----------|-------|
| **Directory** | `deploy/demo/scenarios/gitops-drift/` |
| **GitHub Issue** | #125 |
| **Namespace** | `demo-gitops` |
| **Signal** | `KubePodCrashLooping` (pod restarts) |
| **Root Cause** | Broken ConfigMap pushed via Git commit |
| **Key Differentiator** | Signal resource (Pod) != RCA resource (ConfigMap). LLM must choose `GitRevertCommit` over `RollbackDeployment` because the environment is GitOps-managed. |
| **Workflow** | `git-revert-v1` / `GitRevertCommit` |
| **Bundle** | `quay.io/kubernaut-cicd/test-workflows/git-revert-job:v1.0.0` |
| **Infrastructure** | Gitea (lightweight Git server) + ArgoCD |
| **Detected Labels** | `gitOpsTool: "*"` (declared in `workflow-schema.yaml`) |

**How it works:**
1. ArgoCD manages an nginx Deployment via a Gitea Git repository
2. A bad commit changes a ConfigMap key to an invalid value → nginx can't start → CrashLoopBackOff
3. Prometheus fires `KubePodCrashLooping` alert
4. HAPI's LabelDetector discovers ArgoCD annotations → `gitOpsManaged=true`
5. LLM traces crash to ConfigMap (not Pod) and selects `GitRevertCommit` because of GitOps context
6. WE Job clones Gitea repo, runs `git revert HEAD`, pushes
7. ArgoCD auto-syncs the reverted ConfigMap → pods recover
8. EM verifies health restored

**Files:**
```
deploy/demo/scenarios/gitops-drift/
├── README.md                              # Full BDD spec + manual instructions
├── run.sh                                 # Automated runner (Steps 1-8)
├── cleanup.sh                             # Teardown
├── manifests/
│   ├── namespace.yaml                     # demo-gitops + business labels
│   ├── deployment.yaml                    # nginx Deployment + ConfigMap + Service
│   ├── prometheus-rule.yaml               # PrometheusRule CRD (KubePodCrashLooping)
│   └── argocd-application.yaml            # ArgoCD Application pointing to Gitea repo
└── workflow/
    ├── workflow-schema.yaml               # DD-WORKFLOW-001 v2.7 compliant
    ├── Dockerfile                         # ubi9-minimal + git + kubectl
    └── remediate.sh                       # Validate → git revert → Verify (ArgoCD sync)
```

**Prerequisites:** Gitea and ArgoCD must be installed first:
```bash
./deploy/demo/scenarios/gitops/scripts/setup-gitea.sh
./deploy/demo/scenarios/gitops/scripts/setup-argocd.sh
```

---

### 3.2 Scenario #126: Cluster Autoscaling

| Attribute | Value |
|-----------|-------|
| **Directory** | `deploy/demo/scenarios/autoscale/` |
| **GitHub Issue** | #126 |
| **Namespace** | `demo-autoscale` |
| **Signal** | `KubePodSchedulingFailed` (pods stuck Pending) |
| **Root Cause** | Node capacity exhausted -- pods can't be scheduled |
| **Key Differentiator** | "Split responsibility" architecture: WE Job creates a `ScaleRequest` ConfigMap, a host-side agent (`provisioner.sh`) provisions the actual Kind node via Podman + kubeadm. |
| **Workflow** | `provision-node-v1` / `ProvisionNode` |
| **Bundle** | `quay.io/kubernaut-cicd/test-workflows/provision-node-job:v1.0.0` |

**How it works:**
1. An nginx Deployment starts with 2 replicas (512Mi each) on the worker node
2. `run.sh` scales to 8 replicas → worker can't fit all → pods go Pending
3. Prometheus fires `KubePodSchedulingFailed` after 2 minutes
4. LLM diagnoses resource exhaustion, selects `ProvisionNode` workflow
5. WE Job creates a `scale-request` ConfigMap in `kubernaut-system` namespace with `status: pending`
6. `provisioner.sh` (host-side, started by `run.sh`) watches for the ConfigMap, then:
   - Creates a new Kind node container via `podman run`
   - Gets a `kubeadm join` command from the control plane
   - Joins the node and labels it `kubernaut.ai/workload-node=true`
   - Patches ConfigMap to `status: fulfilled`
7. WE Job detects fulfillment, waits for pods to schedule on the new node
8. EM verifies all pods are Running

**Architecture decision:** `kubeadm join` must run on the machine becoming the node, not from inside a K8s Job. The "split responsibility" model avoids mounting the Podman socket into a Job container, mirroring how production cloud autoscalers (Karpenter/NAP) work as external agents.

**Files:**
```
deploy/demo/scenarios/autoscale/
├── README.md
├── run.sh                                 # Automated runner (starts provisioner in background)
├── cleanup.sh
├── provisioner.sh                         # Host-side agent (watches ScaleRequest, provisions node)
├── manifests/
│   ├── namespace.yaml                     # demo-autoscale + business labels
│   ├── deployment.yaml                    # nginx Deployment + Service (512Mi/pod, with health probes)
│   └── prometheus-rule.yaml               # PrometheusRule CRD (KubePodSchedulingFailed)
└── workflow/
    ├── workflow-schema.yaml               # DD-WORKFLOW-001 v2.7 compliant
    ├── Dockerfile                         # ubi9-minimal + kubectl
    └── remediate.sh                       # Validate → Create ScaleRequest → Verify fulfillment
```

---

### 3.3 Scenario #128: SLO Error Budget Burn

| Attribute | Value |
|-----------|-------|
| **Directory** | `deploy/demo/scenarios/slo-burn/` |
| **GitHub Issue** | #128 |
| **Namespace** | `demo-slo` |
| **Signal** | `KubernautSLOBudgetBurning` (error rate > 14.4x sustainable burn) |
| **Root Cause** | Bad ConfigMap swap causes /api/ to return 500s, but /healthz passes |
| **Key Differentiator** | Subtle failure mode -- readiness probes pass, deployment looks healthy, but the service is broken. Predictive SLO burn rate math detects the issue. `rollout undo` reverts the Deployment's volume reference back to the healthy ConfigMap. |
| **Workflow** | `proactive-rollback-v1` / `ProactiveRollback` |
| **Bundle** | `quay.io/kubernaut-cicd/test-workflows/proactive-rollback-job:v1.0.0` |

**How it works:**
1. An nginx Deployment serves `/api/` (200 OK) and `/healthz` (200 OK) via a ConfigMap-mounted config
2. A traffic generator hits `/api/status` every 200ms to generate baseline metrics
3. A **blackbox-exporter** + `Probe` CRD probes the `/api/status` endpoint every 10s
4. Prometheus records `probe_success` and computes `1 - avg_over_time(probe_success[5m])` as the error rate
5. `inject-bad-config.sh` creates `api-config-bad` ConfigMap (returns 500 on /api/) and patches the Deployment's volume reference to point at it
6. Health checks still pass (/healthz returns 200), but /api/ returns 500
7. Blackbox probes start failing → error rate rises → `KubernautSLOBudgetBurning` fires after 2m
8. LLM correlates the error spike with the recent deployment revision change
9. WE Job runs `kubectl rollout undo` → reverts volume reference back to `api-config` → /api/ works again
10. EM verifies error rate drops below SLO threshold

**Critical design decisions applied during this session:**
- **Injection creates a separate ConfigMap** (`api-config-bad`) and patches the Deployment volume reference, NOT the original ConfigMap in-place. This is what makes `rollout undo` work -- it reverts the volume reference.
- **Blackbox-exporter for metrics**, not nginx stub_status. The `stub_status` module only provides aggregate request counts without status-code breakdown, making it impossible to detect error rates. The blackbox-exporter probes the actual endpoint and exposes `probe_success` (0 or 1).
- **Revision fetched via jsonpath** (`deployment.kubernetes.io/revision` annotation), not `kubectl rollout history` text parsing.

**Files:**
```
deploy/demo/scenarios/slo-burn/
├── README.md
├── run.sh                                 # Automated runner (deploy, baseline, inject, monitor)
├── cleanup.sh
├── inject-bad-config.sh                   # Creates bad ConfigMap + patches Deployment volume ref
├── manifests/
│   ├── namespace.yaml                     # demo-slo (production, tier-1)
│   ├── configmap.yaml                     # Healthy nginx config (/api/ returns 200)
│   ├── deployment.yaml                    # nginx Deployment + Service + traffic-gen
│   ├── blackbox-exporter.yaml             # Deployment + Service + Probe CRD
│   └── prometheus-rule.yaml               # PrometheusRule CRD (SLO burn rate from probe_success)
└── workflow/
    ├── workflow-schema.yaml               # DD-WORKFLOW-001 v2.7 compliant
    ├── Dockerfile                         # ubi9-minimal + kubectl
    └── remediate.sh                       # Validate (revision check) → rollout undo → Verify
```

---

## 4. Additionally Implemented Scenarios (8 of 11)

All 8 remaining scenarios are now fully implemented with manifests, workflows, run.sh, README, and cleanup.sh.

| # | Directory | Issue | Action Type | Signal | Kind Config | Key Details |
|---|-----------|-------|-------------|--------|-------------|-------------|
| #120 | `crashloop/` | #120 | `GracefulRestart` | CrashLoopBackOff | singlenode | Bad ConfigMap injection → rollback. `inject-bad-config.sh` patches ConfigMap. |
| #121 | `disk-pressure/` | #121 | `CleanupPVC` | DiskPressure | singlenode | `inject-orphan-pvcs.sh` creates orphaned PVCs. Workflow deletes unmounted PVCs. |
| #122 | `pending-taint/` | #122 | `RemoveTaint` | FailedScheduling | **multinode** | `inject-taint.sh` applies `maintenance=scheduled:NoSchedule` to worker. Workflow removes taint. |
| #123 | `hpa-maxed/` | #123 | `PatchHPA` | HPAMaxedOut | singlenode | `inject-load.sh` generates CPU load via busybox pod. Workflow patches `maxReplicas`. |
| #124 | `pdb-deadlock/` | #124 | `RelaxPDB` | DrainBlocked | singlenode | `inject-rolling-update.sh` triggers blocked rollout. Workflow reduces `minAvailable`. |
| #127 | `node-notready/` | #127 | `CordonDrainNode` | NodeNotReady | **multinode** | `inject-node-failure.sh` uses `podman pause` on worker. Workflow cordons + drains. |
| #129 | `memory-leak/` | #129 | `GracefulRestart` | PredictiveMemoryExhaust | singlenode | Sidecar leaks ~1MB/15s. `predict_linear()` fires alert. Workflow does rolling restart. |
| #130 | `stuck-rollout/` | #130 | `GracefulRestart` | StuckRollout | singlenode | `inject-bad-image.sh` patches to non-existent image tag. Workflow does `rollout undo`. |

All 9 `actionType` taxonomy values are pre-seeded via `migrations/026_demo_action_types.sql`.

Per-scenario `kind-config.yaml` files are symlinks to shared configs at the `scenarios/` level:
- `kind-config-singlenode.yaml` (8 scenarios): single control-plane node
- `kind-config-multinode.yaml` (3 scenarios: autoscale, node-notready, pending-taint): control-plane + worker with `kubernaut.ai/workload-node=true`

---

## 4b. New Label Coverage & CRD/Operator Scenarios (6 of 17)

These 6 scenarios ensure every detected label (DD-HAPI-018) has demo coverage, plus introduce CRD/operator remediation via cert-manager.

| # | Directory | Issue | Action Type | Signal | Detected Labels | Kind Config | Key Details |
|---|-----------|-------|-------------|--------|-----------------|-------------|-------------|
| #133 | `cert-failure/` | #133 | `FixCertificate` | CertificateNotReady | (none -- plain operator) | singlenode | cert-manager Certificate NotReady after CA Secret deleted. `inject-broken-issuer.sh` deletes CA Secret. Workflow regenerates CA. |
| #134 | `cert-failure-gitops/` | #134 | `GitRevertCommit` | CertificateNotReady | `gitOpsTool: "*"` | singlenode | GitOps variant of #133. ArgoCD manages Certificate/Issuer via Gitea. `run.sh` pushes broken ClusterIssuer via git. Workflow does git revert. |
| #135 | `crashloop-helm/` | #135 | `HelmRollback` | CrashLoopBackOff | `helmManaged: "true"` | singlenode | Helm variant of #120. Workload deployed via Helm chart. `inject-bad-config.sh` runs `helm upgrade` with bad values. Workflow does `helm rollback`. |
| #136 | `mesh-routing-failure/` | #136 | `FixAuthorizationPolicy` | HighErrorRate | `serviceMesh: "*"` | singlenode | Linkerd-meshed workload blocked by AuthorizationPolicy. `inject-deny-policy.sh` applies deny-all Server+AuthorizationPolicy. Workflow removes policy. |
| #137 | `statefulset-pvc-failure/` | #137 | `FixStatefulSetPVC` | StatefulSetReplicasMismatch | `stateful: "true"` | singlenode | 3-replica StatefulSet with PVC deleted. `inject-pvc-issue.sh` deletes pod+PVC+PV. Workflow recreates PVC + restarts pod. |
| #138 | `network-policy-block/` | #138 | `FixNetworkPolicy` | DeploymentUnavailable | `networkIsolated: "true"` | singlenode | Deny-all NetworkPolicy blocks health checks. `inject-deny-all-netpol.sh` applies deny-all ingress. Workflow removes offending policy. |

5 new `actionType` values are seeded via `migrations/029_new_demo_action_types.sql`: `FixCertificate`, `HelmRollback`, `FixAuthorizationPolicy`, `FixStatefulSetPVC`, `FixNetworkPolicy`.

### Detected Label Coverage Matrix

| Detected Label | Detection Source (DD-HAPI-018) | Demo Scenario(s) |
|----------------|-------------------------------|-------------------|
| `gitOpsManaged` / `gitOpsTool` | ArgoCD/Flux annotations | `gitops-drift` (#125), `cert-failure-gitops` (#134) |
| `pdbProtected` | PDB selector match | `pdb-deadlock` (#124) |
| `hpaEnabled` | HPA scaleTargetRef match | `hpa-maxed` (#123) |
| `helmManaged` | `app.kubernetes.io/managed-by: Helm` label | `crashloop-helm` (#135) |
| `serviceMesh` | Linkerd/Istio pod annotations | `mesh-routing-failure` (#136) |
| `stateful` | StatefulSet in owner chain | `statefulset-pvc-failure` (#137) |
| `networkIsolated` | NetworkPolicy in namespace | `network-policy-block` (#138) |

### External Dependencies (New Scenarios)

| Scenario | External Dep | Install Method | RAM Footprint |
|----------|-------------|----------------|---------------|
| #133, #134 | cert-manager | `kubectl apply -f cert-manager.yaml` (auto in run.sh) | ~100MB |
| #134 | Gitea + ArgoCD | Shared scripts (`../gitops/scripts/`) | ~1.5GB |
| #135 | Helm 3 | Pre-installed on host | 0 (CLI only) |
| #136 | Linkerd | `linkerd install` (auto in run.sh) | ~200MB |
| #137 | local-path provisioner | Built into Kind | 0 |
| #138 | kindnet CNI | Built into Kind | 0 |

---

## 5. Shared Infrastructure & Tooling

### 5.1 Kind Cluster Config
**Files**: `deploy/demo/scenarios/kind-config-singlenode.yaml` and `deploy/demo/scenarios/kind-config-multinode.yaml`

Each scenario directory has a `kind-config.yaml` symlink pointing to the appropriate shared config. Port mappings expose Gateway (30080), DataStorage (30081), Prometheus (9190), AlertManager (9193), Grafana (3000). Multinode config adds a worker node labeled `kubernaut.ai/workload-node=true`.

### 5.2 Database Migrations
**Files**: `migrations/026_demo_action_types.sql` and `migrations/029_new_demo_action_types.sql`

- **026**: 9 original action types: `GitRevertCommit`, `ProvisionNode`, `GracefulRestart`, `CleanupPVC`, `RemoveTaint`, `PatchHPA`, `RelaxPDB`, `ProactiveRollback`, `CordonDrainNode`
- **029**: 5 new action types: `FixCertificate`, `HelmRollback`, `FixAuthorizationPolicy`, `FixStatefulSetPVC`, `FixNetworkPolicy`

Both use `ON CONFLICT DO NOTHING` for idempotency.

### 5.3 Build Script
**File**: `deploy/demo/scripts/build-demo-workflows.sh`

Builds multi-architecture OCI images (amd64 + arm64) for all demo workflow bundles using Podman manifests and pushes to `quay.io/kubernaut-cicd/test-workflows/`.

```bash
./deploy/demo/scripts/build-demo-workflows.sh                        # Multi-arch build + push
./deploy/demo/scripts/build-demo-workflows.sh --local                # Local arch only, no push
./deploy/demo/scripts/build-demo-workflows.sh --scenario gitops-drift  # Single scenario
```

The script validates that each scenario has a `Dockerfile` and `workflow-schema.yaml` (ADR-043 compliance).

**Current workflow-to-image mappings (all with real SHA256 digests):**
| Scenario | Image |
|----------|-------|
| `autoscale` | `provision-node-job:v1.0.0` |
| `crashloop` | `crashloop-rollback-job:v1.0.0` |
| `disk-pressure` | `cleanup-pvc-job:v1.0.0` |
| `gitops-drift` | `git-revert-job:v1.0.0` |
| `hpa-maxed` | `patch-hpa-job:v1.0.0` |
| `memory-leak` | `graceful-restart-job:v1.0.0` |
| `node-notready` | `cordon-drain-job:v1.0.0` |
| `pdb-deadlock` | `relax-pdb-job:v1.0.0` |
| `pending-taint` | `remove-taint-job:v1.0.0` |
| `slo-burn` | `proactive-rollback-job:v1.0.0` |
| `stuck-rollout` | `rollback-deployment-job:v1.0.0` |
| `cert-failure` | `fix-certificate-job:v1.0.0` |
| `cert-failure-gitops` | `fix-certificate-gitops-job:v1.0.0` |
| `crashloop-helm` | `helm-rollback-job:v1.0.0` |
| `mesh-routing-failure` | `fix-authz-policy-job:v1.0.0` |
| `statefulset-pvc-failure` | `fix-statefulset-pvc-job:v1.0.0` |
| `network-policy-block` | `fix-network-policy-job:v1.0.0` |

### 5.4 Seed Script
**File**: `deploy/demo/scripts/seed-workflows.sh`

Registers workflow OCI images in the DataStorage catalog via REST API. DataStorage pulls the image, extracts `/workflow-schema.yaml`, and populates all catalog fields from it (DD-WORKFLOW-017).

```bash
./deploy/demo/scripts/seed-workflows.sh                  # Default: http://localhost:30081
./deploy/demo/scripts/seed-workflows.sh http://ds:8080    # Custom DataStorage URL
```

Registers 19 workflows (2 existing + 11 original demo + 6 new label/CRD demo).

### 5.5 GitOps Setup Scripts
**Directory**: `deploy/demo/scenarios/gitops/scripts/`

| Script | Purpose |
|--------|---------|
| `setup-gitea.sh` | Installs Gitea (lightweight Git server) via Helm, creates admin user, creates `demo-gitops-repo` with healthy manifests |
| `setup-argocd.sh` | Installs ArgoCD core via Helm, registers Gitea repo credentials |

These are shared prerequisites for any scenario that uses GitOps (#125, #134).

---

## 6. Workflow Bundle Convention

Every workflow follows this structure (per ADR-043, DD-WORKFLOW-003):

```
workflow/
├── workflow-schema.yaml     # DD-WORKFLOW-001 v2.7 schema (registered in DataStorage catalog)
├── Dockerfile               # ubi9-minimal base, multi-arch (ARG TARGETARCH), USER 1001
└── remediate.sh             # Validate → Action → Verify pattern
```

**Dockerfile conventions:**
- Base: `registry.access.redhat.com/ubi9/ubi-minimal:latest`
- Multi-arch: `ARG TARGETARCH` → kubectl binary selected by `${TARGETARCH}`
- Schema embedded: `COPY workflow-schema.yaml /workflow-schema.yaml` (DataStorage OCI extractor reads this)
- Non-root: `USER 1001`
- Entrypoint: `/scripts/remediate.sh`

**workflow-schema.yaml conventions:**
- `metadata.workflowId`, `metadata.version`, `metadata.description` (what/whenToUse/whenNotToUse/preconditions)
- `actionType`: Must match a value in `action_type_taxonomy` table
- `labels`: Signal matching criteria (signalType, severity, environment, component, priority)
- `detectedLabels` (optional): Infrastructure requirements for HAPI LabelDetector matching
- `execution.engine: job`, `execution.bundle`: Full quay.io image reference
- `parameters`: Env vars injected into the WE Job container

**remediate.sh conventions:**
- 3-phase structure: `Phase 1: Validate`, `Phase 2: Action`, `Phase 3: Verify`
- Uses `kubectl` from within the Job container
- Exits 0 on success, non-zero on failure
- Parameters received as environment variables

---

## 7. Prometheus Alerting Convention

All scenarios use **PrometheusRule CRD** (`monitoring.coreos.com/v1`), NOT ConfigMap-based rules.

Required labels for prometheus-operator to pick up the rule:
```yaml
labels:
  release: prometheus
```

The PrometheusRule is deployed in the same namespace as the scenario (e.g., `demo-gitops`, `demo-slo`) so that PromQL namespace selectors are straightforward.

---

## 8. Design Decisions & Lessons Learned

### 8.1 Signal != RCA (Scenario #125)
The alert fires on Pod restarts, but the root cause is a ConfigMap. The LLM must trace from the signal resource (Pod) to the actual root cause resource (ConfigMap). This is a common real-world pattern where Prometheus alerts on symptoms, not causes.

### 8.2 Split Responsibility for Node Provisioning (Scenario #126)
`kubeadm join` must run on the machine becoming the node, not from inside a K8s Job. The WE Job can't provision a node -- it creates a `ScaleRequest` ConfigMap and a host-side agent (`provisioner.sh`) fulfills it. This mirrors production cloud autoscalers (Karpenter, GKE NAP).

### 8.3 ConfigMap Volume Swap for Rollback (Scenario #128)
If you patch a ConfigMap in-place and then `rollout undo`, the undo only reverts the Deployment's pod template -- the ConfigMap remains broken. The fix: create a **separate** bad ConfigMap and patch the Deployment's volume reference. `rollout undo` then reverts the volume reference, pointing back to the healthy ConfigMap.

### 8.4 Blackbox Probe for Error Rate (Scenario #128)
nginx's `stub_status` module provides aggregate request counts but NO per-status-code breakdown. You cannot compute error rates from it. Solution: deploy a `blackbox-exporter` with a `Probe` CRD that probes the actual endpoint. `probe_success` (0 or 1) probed every 10s gives a usable error rate via `1 - avg_over_time(probe_success[5m])`.

### 8.5 Detected Labels Are NOT Namespace Labels
`kubernaut.ai/gitops-managed: "true"` is NOT a valid Kubernaut label for GitOps detection. GitOps detection is performed at runtime by HAPI's LabelDetector, which checks for ArgoCD/Flux native annotations on the workload. The `workflow-schema.yaml` declares requirements via `detectedLabels: { gitOpsTool: "*" }` for workflow catalog filtering -- this is matched against the incident's runtime-detected labels, not static namespace labels.

### 8.6 GitHub Issue for Missing ADR-043 Field
Issue #131 tracks the documentation gap where `detectedLabels` is not formally defined in ADR-043 (workflow schema specification). The field is used in practice by the implemented scenarios and HAPI's matching logic (ADR-056, DD-HAPI-018).

---

## 9. What Needs to Be Done

### 9.1 End-to-End Validation

No scenario has been tested end-to-end yet. Each needs validation:
1. Create Kind cluster with the per-scenario kind-config (symlinks handle singlenode vs multinode)
2. Deploy Kubernaut services with real LLM backend
3. Run `run.sh` and verify the full pipeline completes
4. Verify EM assessment shows successful remediation
5. Run `cleanup.sh` and verify clean teardown

### 9.2 Open GitHub Issues

| Issue | Title | Priority | Status |
|-------|-------|----------|--------|
| #114 | Demo Scenarios umbrella | High | All 17 scenarios implemented, pending E2E validation |
| #131 | ADR-043: Add detectedLabels field spec | Normal | Open |
| #132 | GitOps causality evidence chain and CRD safety guardrails | Normal | Open |
| #133 | cert-manager Certificate failure (CRD/operator) | Normal | Implemented |
| #134 | cert-manager + GitOps (ArgoCD) | Normal | Implemented |
| #135 | CrashLoopBackOff with Helm-managed workload | Normal | Implemented |
| #136 | Linkerd AuthorizationPolicy block (mesh) | Normal | Implemented |
| #137 | StatefulSet PVC failure (stateful) | Normal | Implemented |
| #138 | NetworkPolicy traffic block (networkIsolated) | Normal | Implemented |

---

## 10. Known Risks & Gotchas

1. **Gitea + ArgoCD memory footprint**: Combined ~1.5GB RAM. On a 12GB budget with all Kubernaut services, this is tight. If you hit OOM, consider reducing ArgoCD to `--server-replicas=1` or using ArgoCD Lite.

2. **ArgoCD sync delay**: Default sync interval is 3 minutes. For faster demo response, either:
   - Force sync via CLI: `argocd app sync web-frontend`
   - Reduce sync interval in ArgoCD config

3. **blackbox-exporter default modules**: The `Probe` CRD references `module: http_2xx`. The default blackbox-exporter config includes this module, but if you're using a custom ConfigMap for the exporter, ensure the `http_2xx` module is defined.

4. **provisioner.sh cleanup**: Scenario #126's provisioner creates real Podman containers that survive `kubectl delete`. The `cleanup.sh` script must also run `podman rm -f` on any nodes it created.

5. **HAPI LabelDetector dependency**: Scenarios #125 (GitOps detection) and any future detected-label scenarios rely on HAPI's LabelDetector being functional (ADR-056, DD-HAPI-018). If the LabelDetector is broken, the LLM won't get the infrastructure context needed to select the correct workflow.

6. **kube-state-metrics required**: Most PrometheusRules use `kube_pod_container_status_restarts_total`, `kube_pod_status_phase`, `kube_deployment_status_condition`, etc. -- all from kube-state-metrics. Ensure it's deployed in the demo cluster.

7. **ServiceAccount for seed script**: The `seed-workflows.sh` script creates a short-lived SA token for `holmesgpt-api-sa`. This SA must exist in `kubernaut-system` namespace, or the script falls back to no-auth mode.

8. **Node-notready cleanup**: Scenario #127 pauses a Kind node via `podman pause`. If the demo is interrupted, the node remains paused. `cleanup.sh` handles `podman unpause` + `kubectl uncordon`, but manual intervention may be needed if cleanup fails.

9. **cert-manager CRD readiness**: cert-manager webhook takes ~10s after deployment to become ready. Scenario #133/#134 run.sh includes a sleep after install. If Certificate creation fails with webhook errors, wait longer.

10. **Linkerd installation**: Scenario #136 requires `linkerd` CLI on the host. Install via `curl --proto '=https' --tlsv1.2 -sSfL https://run.linkerd.io/install | sh`. Linkerd requires ~200MB RAM in Kind.

11. **Helm 3 required**: Scenario #135 requires `helm` CLI on the host. The inject script uses `helm upgrade` and the workflow uses `helm rollback`.

12. **NetworkPolicy CNI support**: Scenario #138 relies on Kind's kindnet CNI supporting NetworkPolicy enforcement. This works with default Kind configuration but may not work with custom CNI configurations.

13. **StatefulSet PVC recreation**: Scenario #137's inject script force-deletes PVC and PV. If the PVC has a `Retain` reclaim policy, the PV may not be cleanly deleted. Kind's local-path provisioner uses `Delete` by default.

14. **cert-manager metrics**: Scenarios #133/#134 require Prometheus to scrape cert-manager metrics (`certmanager_certificate_ready_status`). Ensure cert-manager ServiceMonitor is deployed or Prometheus is configured to scrape the cert-manager namespace.

---

## 11. File Inventory (Complete)

```
deploy/demo/
├── overlays/kind/
│   └── kind-cluster-config.yaml                  # 2-node Kind cluster
├── scripts/
│   ├── build-demo-workflows.sh                   # Multi-arch OCI image builder
│   └── seed-workflows.sh                         # DataStorage workflow registration
└── scenarios/
    ├── gitops/
    │   └── scripts/
    │       ├── setup-gitea.sh                    # Gitea installation
    │       └── setup-argocd.sh                   # ArgoCD installation
    ├── gitops-drift/   (#125) ✅ IMPLEMENTED
    │   ├── README.md
    │   ├── run.sh
    │   ├── cleanup.sh
    │   ├── manifests/
    │   │   ├── namespace.yaml
    │   │   ├── deployment.yaml                   # nginx + ConfigMap + Service
    │   │   ├── prometheus-rule.yaml              # PrometheusRule: KubePodCrashLooping
    │   │   └── argocd-application.yaml           # ArgoCD Application
    │   └── workflow/
    │       ├── workflow-schema.yaml              # GitRevertCommit, detectedLabels: gitOpsTool
    │       ├── Dockerfile                        # ubi9 + git + kubectl
    │       └── remediate.sh                      # git revert HEAD → push → wait ArgoCD sync
    ├── autoscale/      (#126) ✅ IMPLEMENTED
    │   ├── README.md
    │   ├── run.sh
    │   ├── cleanup.sh
    │   ├── provisioner.sh                        # Host-side agent (Podman + kubeadm)
    │   ├── manifests/
    │   │   ├── namespace.yaml
    │   │   ├── deployment.yaml                   # nginx 512Mi/pod + Service, health probes
    │   │   └── prometheus-rule.yaml              # PrometheusRule: KubePodSchedulingFailed
    │   └── workflow/
    │       ├── workflow-schema.yaml              # ProvisionNode
    │       ├── Dockerfile                        # ubi9 + kubectl
    │       └── remediate.sh                      # Create ScaleRequest CM → wait fulfillment
    ├── slo-burn/       (#128) ✅ IMPLEMENTED
    │   ├── README.md
    │   ├── run.sh
    │   ├── cleanup.sh
    │   ├── inject-bad-config.sh                  # Creates api-config-bad CM + patches volume ref
    │   ├── manifests/
    │   │   ├── namespace.yaml
    │   │   ├── configmap.yaml                    # Healthy nginx config
    │   │   ├── deployment.yaml                   # nginx + traffic-gen + Service
    │   │   ├── blackbox-exporter.yaml            # Deployment + Service + Probe CRD
    │   │   └── prometheus-rule.yaml              # PrometheusRule: SLO burn rate from probe_success
    │   └── workflow/
    │       ├── workflow-schema.yaml              # ProactiveRollback
    │       ├── Dockerfile                        # ubi9 + kubectl
    │       └── remediate.sh                      # Validate revision → rollout undo → verify CM ref
    ├── crashloop/      (#120) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-bad-config.sh
    │   ├── manifests/ (namespace, configmap, deployment, prometheus-rule)
    │   └── workflow/ (GracefulRestart, crashloop-rollback-job)
    ├── disk-pressure/  (#121) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-orphan-pvcs.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (CleanupPVC, cleanup-pvc-job)
    ├── pending-taint/  (#122) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-taint.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (RemoveTaint, remove-taint-job)
    ├── hpa-maxed/      (#123) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-load.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (PatchHPA, patch-hpa-job)
    ├── pdb-deadlock/   (#124) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-rolling-update.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (RelaxPDB, relax-pdb-job)
    ├── node-notready/  (#127) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-node-failure.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (CordonDrainNode, cordon-drain-job)
    ├── memory-leak/    (#129) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (GracefulRestart, graceful-restart-job)
    ├── stuck-rollout/  (#130) ✅ IMPLEMENTED
    │   ├── README.md, run.sh, cleanup.sh, inject-bad-image.sh
    │   ├── manifests/ (namespace, deployment, prometheus-rule)
    │   └── workflow/ (GracefulRestart, rollback-deployment-job)
    ├── cert-failure/  (#133) ✅ NEW -- CRD/operator scenario
    │   ├── README.md, run.sh, cleanup.sh, inject-broken-issuer.sh
    │   ├── manifests/ (namespace, clusterissuer, certificate, ca-secret, deployment, prometheus-rule)
    │   └── workflow/ (FixCertificate, fix-certificate-job) -- includes openssl for CA regeneration
    ├── cert-failure-gitops/  (#134) ✅ NEW -- CRD + GitOps
    │   ├── README.md, run.sh, cleanup.sh
    │   ├── manifests/ (argocd-application, prometheus-rule)
    │   └── workflow/ (GitRevertCommit, fix-certificate-gitops-job) -- git + kubectl
    ├── crashloop-helm/  (#135) ✅ NEW -- helmManaged label
    │   ├── README.md, run.sh, cleanup.sh, inject-bad-config.sh
    │   ├── chart/ (Chart.yaml, values.yaml, templates/) -- Helm chart for workload
    │   ├── manifests/ (prometheus-rule)
    │   └── workflow/ (HelmRollback, helm-rollback-job) -- kubectl + helm
    ├── mesh-routing-failure/  (#136) ✅ NEW -- serviceMesh label
    │   ├── README.md, run.sh, cleanup.sh, inject-deny-policy.sh
    │   ├── manifests/ (namespace, deployment, deny-policy, prometheus-rule)
    │   └── workflow/ (FixAuthorizationPolicy, fix-authz-policy-job)
    ├── statefulset-pvc-failure/  (#137) ✅ NEW -- stateful label
    │   ├── README.md, run.sh, cleanup.sh, inject-pvc-issue.sh
    │   ├── manifests/ (namespace, statefulset, prometheus-rule)
    │   └── workflow/ (FixStatefulSetPVC, fix-statefulset-pvc-job)
    └── network-policy-block/  (#138) ✅ NEW -- networkIsolated label
        ├── README.md, run.sh, cleanup.sh, inject-deny-all-netpol.sh
        ├── manifests/ (namespace, deployment, networkpolicy-allow, deny-all-netpol, prometheus-rule)
        └── workflow/ (FixNetworkPolicy, fix-network-policy-job)

migrations/
├── 026_demo_action_types.sql                     # 9 original action types
└── 029_new_demo_action_types.sql                 # 5 new action types (#133-#138)
```

---

## 12. Quick Start for New Team

```bash
# 1. Create the Kind cluster
kind create cluster --name kubernaut-demo \
  --config deploy/demo/overlays/kind/kind-cluster-config.yaml

# 2. Deploy Kubernaut services (with real LLM backend configured)
# [your existing deployment process]

# 3. Run the database migration
goose -dir migrations postgres "$DATABASE_URL" up

# 4. Build and push workflow images
podman login quay.io
./deploy/demo/scripts/build-demo-workflows.sh

# 5. Seed workflow catalog
./deploy/demo/scripts/seed-workflows.sh

# 6. Run a scenario
./deploy/demo/scenarios/slo-burn/run.sh          # SLO burn (simplest, no GitOps prereqs)
./deploy/demo/scenarios/autoscale/run.sh          # Autoscaling (needs Podman on host)
./deploy/demo/scenarios/gitops-drift/run.sh       # GitOps (auto-installs Gitea + ArgoCD)

# 7. Monitor the pipeline
kubectl get rr,sp,aa,we,ea -n <scenario-namespace> -w
```

---

## 13. Authoritative Documentation References

| Document | Path | Relevance |
|----------|------|-----------|
| DD-WORKFLOW-001 v2.7 | `docs/architecture/decisions/DD-WORKFLOW-001*` | Workflow schema definition standard |
| ADR-043 | `docs/architecture/decisions/ADR-043*` | OCI bundle + /workflow-schema.yaml standard |
| ADR-056 | (in docs/) | DetectedLabels computation moved to HAPI |
| DD-HAPI-018 | `docs/architecture/decisions/DD-HAPI-018*` | LabelDetector for infrastructure characteristics |
| DD-WORKFLOW-003 | (in docs/) | Parameterized remediation actions (Validate/Action/Verify) |
| DD-WORKFLOW-016 | `docs/architecture/decisions/DD-WORKFLOW-016*` | Action-type taxonomy indexing |
| DD-WORKFLOW-017 | `docs/architecture/decisions/DD-WORKFLOW-017*` | Workflow lifecycle + component interactions |
| BR-WE-014 | (in docs/) | Kubernetes Job execution backend |

---

## 14. Session Transcript

The full plain-text transcript of this session:
```
/Users/jgil/.cursor/projects/Users-jgil-go-src-github-com-jordigilh-kubernaut/agent-transcripts/d633a50e-f329-41a9-b792-b642b5890d4a.txt
```

Search for keywords: `blackbox-exporter`, `api-config-bad`, `PrometheusRule`, `provisioner.sh`, `split responsibility`, `detectedLabels`, `seed-workflows`, `build-demo-workflows`, `triage` to find relevant sections.
