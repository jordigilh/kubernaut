# DD-PLATFORM-003: Infra-First ArgoCD Sync Wave (Phased Deployment)

**Date**: July 7, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 90%
**Last Reviewed**: July 7, 2026
**Related**: Issue #1617 (Helm chart GitOps/ArgoCD operational readiness), DD-PLATFORM-001, DD-PLATFORM-002, PR #1625

---

## 🎯 **DECISION**

**PostgreSQL, Valkey, the DataStorage service (Deployment + its ServiceAccount),
all `cert-manager.io` `Certificate`/`Issuer` resources used for inter-service
mTLS and the DataStorage signing cert, and the `db-migration` /
`interservice-ca-sync` hook Jobs SHALL carry
`argocd.argoproj.io/sync-wave: "-1"`. Every other chart resource (the
remaining ~12 application controllers) keeps the implicit default wave `"0"`
and is unchanged.**

Scope: ArgoCD-only annotations (`argocd.argoproj.io/sync-wave`; Helm CLI
ignores this key and is completely unaffected — `helm install`/`helm upgrade`
ordering and timing are unchanged). No change to hook Job logic, RBAC scope,
container images, or application code.

---

## 📊 **Context & Problem**

### How this was found

Building the automated GitOps CI validation for issue #1617 (PR #1625), the
first real GitHub Actions run of the `tls.mode=cert-manager` ArgoCD smoke
test failed at "Verify GitOps deployment": `gateway`, `authwebhook`,
`effectivenessmonitor`, `notification`, `signalprocessing`, and
`workflowexecution` were all stuck in `CrashLoopBackOff`, even after the
existing single-shot pod-recovery step (DD-PLATFORM-002's documented
ConfigMap-mount-race workaround) ran.

### Root cause

Must-gather logs showed all six *unrelated* services failing identically
during controller-runtime manager startup:

```
"error":"failed to get server groups: Get \"https://10.96.0.1:443/api\": dial tcp 10.96.0.1:443: i/o timeout"
```

— each exactly 30s (client-go's dial timeout) after their last successful
startup log line, all within the same ~45s wall-clock window, while
`kube-apiserver`'s own log went completely silent for the preceding ~4
minutes. This is **CI-environment resource contention, not a chart or
application defect**: a standard GitHub-hosted `ubuntu-latest` runner is 4
vCPU / 16GB RAM (per
[GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)),
and this single VM hosts Kind's control plane (kube-apiserver, etcd,
scheduler, controller-manager) *plus* ArgoCD's own ~9 pods, cert-manager's 3
pods, and the full chart's ~14 Go controllers + PostgreSQL + Valkey. Because
DD-PLATFORM-002 pins every hook Job to the same `sync-wave: "0"` as every
regular (unannotated, implicitly wave-`"0"`) chart resource, ArgoCD applies
essentially the *entire* chart — all ~14 Deployments, both Jobs, and every
`Certificate` — concurrently, in one wave. That simultaneous burst of Go
binaries each performing TLS handshakes, client-go informer cache syncs, and
REST discovery calls at startup is measurably heavier than what a plain
`helm install` produces (Helm has no equivalent "apply everything at once"
concentration point), and was sufficient to transiently starve the node's
network stack.

A **secondary, independent finding** during this investigation: several
application Deployments mount `Secret`s produced by the leaf `Certificate`
resources (`templates/interservice/leaf-certs.yaml`) as `optional: true`
volumes — the exact same "mounts empty before the producer exists, kubelet
never retroactively populates it" class of race that DD-PLATFORM-002 already
documented for the `inter-service-ca` `ConfigMap`. Because those
`Certificate`s currently share the default wave `"0"` with the Deployments
that consume their output Secrets, this race is possible for TLS leaf certs
too, not just the CA ConfigMap.

Both findings point to the same underlying gap: DD-PLATFORM-002 correctly
fixed the *Job-vs-Deployment-health* deadlock, but left every hook Job and
every "produces a resource that other Deployments consume" resource in the
*same* wave as every consumer — trading a hard deadlock for a soft race
(tolerable only via best-effort pod-recovery retries) and a large
concurrency burst.

---

## 🔍 **Alternatives Considered**

### **Option A: Two-wave "infra-first" split** ✅ **CHOSEN**

Move PostgreSQL, Valkey, DataStorage (+ its ServiceAccount), all
`cert-manager.io` `Certificate`/`Issuer` resources, and the two hook Jobs to
`sync-wave: "-1"`. Leave the remaining ~12 application controllers at the
default `"0"`. ArgoCD gates wave `"0"` on wave `"-1"` reaching `Healthy`
(Jobs: `Complete`; `Certificate`: `Ready`, via ArgoCD's built-in cert-manager
health check; Deployments: `Available`).

- ✅ Directly addresses the root cause (halves the peak simultaneous
  pod-startup count) instead of only tolerating the symptom with retries.
- ✅ **Eliminates** the TLS-leaf-cert-Secret mount race as a side effect: no
  application Deployment starts before its consumed `Certificate` Secret and
  the `inter-service-ca` ConfigMap already exist.
- ✅ ArgoCD-only (`argocd.argoproj.io/sync-wave`); zero effect on `helm
  install`/`helm upgrade`, which already applies all non-hook resources
  together regardless of wave annotations.
- ✅ No deadlock risk: verified wave `"-1"` members have no dependency, direct
  or transitive, on any wave `"0"` member's health (DataStorage only needs
  PostgreSQL + its own `Certificate`; the sync Jobs only need PostgreSQL /
  the root CA Secret — never a peer application controller).
- ✅ Also benefits real (non-CI) GitOps installs: infra and prerequisite
  certs/secrets are guaranteed to exist before dependent application
  controllers are even created, not just "probably fast enough."
- ➖ First-install wall-clock time increases slightly (wave `"0"` waits for
  wave `"-1"` to be `Healthy` rather than applying concurrently) — an
  accepted, small trade-off for correctness and reduced CI flakiness.
- ➖ A second `argocd.argoproj.io/sync-wave` value now exists in the chart
  (`"-1"` and the implicit `"0"`), a small additional cognitive surface,
  mitigated by an explanatory comment at each occurrence and this DD.

### **Option B: Bigger GitHub Actions runner** ❌ REJECTED (for this problem)

Move the `cert-manager` matrix leg to a larger (8+ vCPU) hosted or
self-hosted runner.

- ❌ Treats the symptom (CI capacity), not the cause; a sufficiently large
  chart could still contend even on a bigger runner, and this fixes nothing
  for real GitOps users deploying to their own (potentially also
  resource-constrained) clusters.
- ❌ Cost/quota implications requiring a separate organizational decision,
  disproportionate to this problem.
- ➖ Does not address the independently-found TLS-leaf-cert mount race.

### **Option C: CI-only retry/stabilization loop** ✅ KEPT (as defense-in-depth, not a replacement)

Already implemented as an immediate, minimal fix on the same PR: turn the
single-shot "Recover pods" step into a bounded 5-pass loop (45s apart)
restarting any still-`CrashLoopBackOff` pod. This resolved the observed CI
failure on its own (full pipeline went green before Option A was designed).

- ✅ Zero chart changes; fastest possible mitigation once the CI failure was
  diagnosed.
- ➖ Purely reactive — tolerates contention/races after they occur rather
  than reducing their likelihood; does not benefit real GitOps installs at
  all (CI-workflow-only).
- **Decision**: retained alongside Option A as a safety net for any residual,
  lower-probability contention Option A doesn't eliminate (e.g. within a
  single wave, ~12 application controllers still start concurrently).

---

## ✅ **Consequences**

- `charts/kubernaut/templates/infrastructure/postgresql.yaml`: all resources
  (ServiceAccount, Service, Deployment, PVC) annotated `sync-wave: "-1"`.
- `charts/kubernaut/templates/infrastructure/valkey.yaml`: same, all
  resources annotated `sync-wave: "-1"`.
- `charts/kubernaut/templates/rbac/datastorage-rbac.yaml`: only the
  `data-storage-sa` `ServiceAccount` is annotated `sync-wave: "-1"` (a
  hard Kubernetes admission-time requirement: a Deployment's
  `serviceAccountName` must already exist or pod creation is rejected
  outright). The unrelated `ClusterRole`/`ClusterRoleBinding`/`RoleBinding`
  resources in the same file are RBAC authorization records, not startup
  dependencies of the DataStorage pod itself, so they are intentionally left
  at the default wave `"0"`.
- `charts/kubernaut/templates/datastorage/datastorage.yaml`: `ConfigMap`,
  `Deployment`, `Service` all annotated `sync-wave: "-1"`.
- `charts/kubernaut/templates/interservice/ca.yaml`,
  `charts/kubernaut/templates/interservice/leaf-certs.yaml`,
  `charts/kubernaut/templates/datastorage/certificate.yaml`: all
  `cert-manager.io` `Issuer`/`Certificate` resources annotated
  `sync-wave: "-1"` (cert-manager mode only; unchanged in `hook`/`manual`
  mode, which don't render these templates at all).
- `charts/kubernaut/templates/hooks/migration-job.yaml`,
  `charts/kubernaut/templates/hooks/interservice-ca-sync-job.yaml`: existing
  `argocd.argoproj.io/sync-wave` changed from `"0"` to `"-1"` (both were
  already ArgoCD `Sync`-phase per DD-PLATFORM-002; only the wave number
  changes).
- No change to Helm CLI behavior, RBAC scope, hook Job logic, or application
  code.
- `helm-unittest` extended to assert `sync-wave: "-1"` on every resource
  listed above, across `tls.mode` values where applicable.
- Validated via the same live Kind + ArgoCD v3.4.4 + cert-manager v1.20.3 CI
  job (`.github/workflows/ci-pipeline.yml`, `helm-smoke-test` /
  `tls_mode=cert-manager` leg) that originally surfaced the contention.

## 🔗 Related Decisions

- DD-PLATFORM-001: introduced the dedicated internal CA and leaf certs whose
  consumer-Deployment mount race this DD closes.
- DD-PLATFORM-002: introduced the `argocd.argoproj.io/hook: Sync` +
  `sync-wave` override this DD refines (changes the wave value, not the
  hook-phase fix).
- Issue #1617 / PR #1625: umbrella GitOps/ArgoCD operational readiness work;
  this DD directly reduces the CI resource-contention flakiness found while
  validating item 4 (repeatable GitOps validation flow).
