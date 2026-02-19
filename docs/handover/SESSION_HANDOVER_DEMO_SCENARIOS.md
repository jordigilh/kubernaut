# Session Handover: Demo Scenarios (End-to-End Remediation Demos)

**Date**: 2026-02-19  
**Branch**: `feat/effectiveness-monitor-level1-v1.0`  
**Status**: 3 of 11 scenarios implemented, triaged, and fixes applied. **Uncommitted.**  
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

## 3. Implemented Scenarios (3 of 11)

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

## 4. Unimplemented Scenarios (8 of 11)

These directories exist with empty `manifests/` and `workflow/` scaffolding but contain NO files:

| # | Directory | GitHub Issue | Action Type | Signal Type | Description |
|---|-----------|-------------|-------------|-------------|-------------|
| #120 | `crashloop/` | #120 | `GracefulRestart` | CrashLoopBackOff | Predictive restart before crash (memory leak) |
| #121 | `disk-pressure/` | #121 | `CleanupPVC` | DiskPressure | PVC disk space cleanup |
| #122 | `pending-taint/` | #122 | `RemoveTaint` | FailedScheduling | Node taint prevents scheduling |
| #123 | `hpa-maxed/` | #123 | `PatchHPA` | HPAMaxedOut | HPA at maxReplicas, traffic still too high |
| #124 | `pdb-deadlock/` | #124 | `RelaxPDB` | DrainBlocked | PDB prevents node drain |
| #127 | `node-notready/` | #127 | `CordonDrainNode` | NodeNotReady | Node failing, drain pods to healthy nodes |
| #129 | `memory-leak/` | #129 | `GracefulRestart` | PredictiveMemoryExhaust | `predict_linear()` detects memory exhaustion before crash |
| #130 | `stuck-rollout/` | #130 | `GracefulRestart` | StuckRollout | Deployment stuck mid-rollout |

All 9 `actionType` taxonomy values are already pre-seeded via `migrations/026_demo_action_types.sql`.

---

## 5. Shared Infrastructure & Tooling

### 5.1 Kind Cluster Config
**File**: `deploy/demo/overlays/kind/kind-cluster-config.yaml`

2-node cluster (control-plane + worker). Port mappings expose Gateway (30080), DataStorage (30081), Prometheus (9190), AlertManager (9193), Grafana (3000).

### 5.2 Database Migration
**File**: `migrations/026_demo_action_types.sql`

Adds 9 `actionType` taxonomy values to `action_type_taxonomy` table: `GitRevertCommit`, `ProvisionNode`, `GracefulRestart`, `CleanupPVC`, `RemoveTaint`, `PatchHPA`, `RelaxPDB`, `ProactiveRollback`, `CordonDrainNode`. Uses `ON CONFLICT DO NOTHING` for idempotency.

### 5.3 Build Script
**File**: `deploy/demo/scripts/build-demo-workflows.sh`

Builds multi-architecture OCI images (amd64 + arm64) for all demo workflow bundles using Podman manifests and pushes to `quay.io/kubernaut-cicd/test-workflows/`.

```bash
./deploy/demo/scripts/build-demo-workflows.sh                        # Multi-arch build + push
./deploy/demo/scripts/build-demo-workflows.sh --local                # Local arch only, no push
./deploy/demo/scripts/build-demo-workflows.sh --scenario gitops-drift  # Single scenario
```

The script validates that each scenario has a `Dockerfile` and `workflow-schema.yaml` (ADR-043 compliance).

**Current workflow-to-image mappings:**
| Scenario | Image |
|----------|-------|
| `gitops-drift` | `git-revert-job:v1.0.0` |
| `autoscale` | `provision-node-job:v1.0.0` |
| `slo-burn` | `proactive-rollback-job:v1.0.0` |

When adding new scenarios, append to the `WORKFLOWS` array in the script.

### 5.4 Seed Script
**File**: `deploy/demo/scripts/seed-workflows.sh`

Registers workflow OCI images in the DataStorage catalog via REST API. DataStorage pulls the image, extracts `/workflow-schema.yaml`, and populates all catalog fields from it (DD-WORKFLOW-017).

```bash
./deploy/demo/scripts/seed-workflows.sh                  # Default: http://localhost:30081
./deploy/demo/scripts/seed-workflows.sh http://ds:8080    # Custom DataStorage URL
```

Registers 5 workflows (2 existing + 3 demo).

### 5.5 GitOps Setup Scripts
**Directory**: `deploy/demo/scenarios/gitops/scripts/`

| Script | Purpose |
|--------|---------|
| `setup-gitea.sh` | Installs Gitea (lightweight Git server) via Helm, creates admin user, creates `demo-gitops-repo` with healthy manifests |
| `setup-argocd.sh` | Installs ArgoCD core via Helm, registers Gitea repo credentials |

These are shared prerequisites for any scenario that uses GitOps (currently only #125).

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

### 9.1 Immediate: Commit the Demo Scenario Work

All changes are **uncommitted** on branch `feat/effectiveness-monitor-level1-v1.0`. Suggested commit groups:

1. **Kind cluster config**: `deploy/demo/overlays/kind/kind-cluster-config.yaml` (added worker node)
2. **Database migration**: `migrations/026_demo_action_types.sql` (9 new action types)
3. **Scenario #125**: `deploy/demo/scenarios/gitops-drift/` + `deploy/demo/scenarios/gitops/scripts/`
4. **Scenario #126**: `deploy/demo/scenarios/autoscale/`
5. **Scenario #128**: `deploy/demo/scenarios/slo-burn/`
6. **Shared tooling**: `deploy/demo/scripts/build-demo-workflows.sh`, `deploy/demo/scripts/seed-workflows.sh`

**Warning**: The branch has many other uncommitted changes unrelated to demo scenarios (see `git status`). Separate carefully.

### 9.2 Build and Push OCI Images

Images have NOT been built or pushed yet. The `workflow-schema.yaml` files all use `@sha256:placeholder` as the digest.

```bash
# Login to quay.io first
podman login quay.io

# Build and push all demo workflows
./deploy/demo/scripts/build-demo-workflows.sh

# Get actual digests
for img in git-revert-job provision-node-job proactive-rollback-job; do
  echo "$img: $(podman manifest inspect quay.io/kubernaut-cicd/test-workflows/${img}:v1.0.0 | jq -r '.digest')"
done

# Update workflow-schema.yaml files with real digests
# Then re-register in DataStorage
./deploy/demo/scripts/seed-workflows.sh
```

### 9.3 Implement Remaining 8 Scenarios

For each unimplemented scenario, the deliverables are:
1. `manifests/namespace.yaml` with appropriate business labels
2. `manifests/deployment.yaml` (and any supporting resources)
3. `manifests/prometheus-rule.yaml` (PrometheusRule CRD with `release: prometheus` label)
4. `workflow/workflow-schema.yaml` (DD-WORKFLOW-001 v2.7)
5. `workflow/Dockerfile` (follow ubi9-minimal convention from Section 6)
6. `workflow/remediate.sh` (Validate → Action → Verify)
7. `run.sh` (automated end-to-end runner)
8. `README.md` (BDD spec + manual steps)
9. `cleanup.sh`
10. Add entry to `build-demo-workflows.sh` `WORKFLOWS` array
11. Add entry to `seed-workflows.sh`

The `actionType` taxonomy values already exist in the migration. Use the 3 implemented scenarios as templates.

**Priority order** (suggested by previous session):
1. #129 memory-leak (predictive `predict_linear()`)
2. #120 crashloop (predictive restart)
3. #123 hpa-maxed (detected label: HPA)
4. #124 pdb-deadlock (detected label: PDB)
5. #122 pending-taint (node taint removal)
6. #121 disk-pressure (PVC cleanup)
7. #127 node-notready (cordon + drain)
8. #130 stuck-rollout (stuck deployment)

### 9.4 End-to-End Validation

No scenario has been tested end-to-end yet. Each needs validation:
1. Build OCI images with real digests
2. Create Kind cluster with the demo config
3. Deploy Kubernaut services with real LLM backend
4. Run `run.sh` and verify the full pipeline completes
5. Verify EM assessment shows successful remediation

### 9.5 Open GitHub Issues

| Issue | Title | Priority | Status |
|-------|-------|----------|--------|
| #114 | Demo Scenarios umbrella | High | In Progress |
| #125 | GitOps Drift Remediation | High | Implementation complete, untested |
| #126 | Cluster Autoscaling | High | Implementation complete, untested |
| #128 | SLO Error Budget Burn | High | Implementation complete, untested |
| #131 | ADR-043: Add detectedLabels field spec | Normal | Open |

---

## 10. Known Risks & Gotchas

1. **Gitea + ArgoCD memory footprint**: Combined ~1.5GB RAM. On a 12GB budget with all Kubernaut services, this is tight. If you hit OOM, consider reducing ArgoCD to `--server-replicas=1` or using ArgoCD Lite.

2. **ArgoCD sync delay**: Default sync interval is 3 minutes. For faster demo response, either:
   - Force sync via CLI: `argocd app sync web-frontend`
   - Reduce sync interval in ArgoCD config

3. **blackbox-exporter default modules**: The `Probe` CRD references `module: http_2xx`. The default blackbox-exporter config includes this module, but if you're using a custom ConfigMap for the exporter, ensure the `http_2xx` module is defined.

4. **provisioner.sh cleanup**: Scenario #126's provisioner creates real Podman containers that survive `kubectl delete`. The `cleanup.sh` script must also run `podman rm -f` on any nodes it created.

5. **Bundle digest `placeholder`**: All `workflow-schema.yaml` files currently have `@sha256:placeholder`. These MUST be replaced with real digests after building images, or DataStorage won't be able to pull them.

6. **HAPI LabelDetector dependency**: Scenarios #125 (GitOps detection) and any future detected-label scenarios rely on HAPI's LabelDetector being functional (ADR-056, DD-HAPI-018). If the LabelDetector is broken, the LLM won't get the infrastructure context needed to select the correct workflow.

7. **kube-state-metrics required**: The `KubePodCrashLooping` rule (#125) uses `kube_pod_container_status_restarts_total` and the `KubePodSchedulingFailed` rule (#126) uses `kube_pod_status_phase` -- both from kube-state-metrics. Ensure it's deployed in the demo cluster.

8. **ServiceAccount for seed script**: The `seed-workflows.sh` script creates a short-lived SA token for `holmesgpt-api-sa`. This SA must exist in `kubernaut-system` namespace, or the script falls back to no-auth mode.

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
    ├── crashloop/      (#120) 📁 EMPTY SCAFFOLD
    ├── disk-pressure/  (#121) 📁 EMPTY SCAFFOLD
    ├── pending-taint/  (#122) 📁 EMPTY SCAFFOLD
    ├── hpa-maxed/      (#123) 📁 EMPTY SCAFFOLD
    ├── pdb-deadlock/   (#124) 📁 EMPTY SCAFFOLD
    ├── node-notready/  (#127) 📁 EMPTY SCAFFOLD
    ├── memory-leak/    (#129) 📁 EMPTY SCAFFOLD
    └── stuck-rollout/  (#130) 📁 EMPTY SCAFFOLD

migrations/
└── 026_demo_action_types.sql                     # 9 action types (all scenarios)
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
