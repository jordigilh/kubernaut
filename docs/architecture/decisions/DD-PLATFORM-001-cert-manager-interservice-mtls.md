# DD-PLATFORM-001: Inter-Service mTLS Auto-Provisioning in cert-manager Mode

**Date**: July 7, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 90%
**Last Reviewed**: July 7, 2026
**Related**: Issue #1617 (Helm chart GitOps/ArgoCD operational readiness), Issue #334, PR #1619

---

## 🎯 **DECISION**

**When `tls.mode=cert-manager`, the chart SHALL provision a dedicated, internal
Certificate Authority (root `Certificate` + `Issuer`) exclusively for
inter-service mTLS, decoupled from whatever `tls.certManager.issuerRef` is
configured for public-facing/webhook certificates. Leaf certificates for
`gateway-tls`, `datastorage-tls`, and `kubernautagent-tls` SHALL be issued from
this internal CA. The CA's public certificate SHALL be published to the
`inter-service-ca` ConfigMap via a Helm hook Job, matching the existing
hook-mode contract that ~11 consumer services already read from.**

**Scope**: `charts/kubernaut/templates/interservice/` (new), inter-service mTLS
for cert-manager mode only. `hook` mode (self-generated CA via shell script)
and `manual` mode (documented user-managed secrets) are unaffected.

---

## 📊 **Context & Problem**

### Business Requirement

FedRAMP **SC-8** (Transmission confidentiality) requires TLS enforcement for
all service-to-service communication. `hook` mode satisfies this today by
generating a shared internal CA and per-service leaf certs via a Helm hook
shell script (`charts/kubernaut/templates/hooks/tls-cert-job.yaml`).

### Problem Statement

In `tls.mode=cert-manager` and `tls.mode=manual`, **no equivalent mechanism
exists**. Confirmed in code:

- Server side (`pkg/shared/tls/tls.go` `ConfigureConditionalTLS`): if
  `tls.crt`/`tls.key` are absent from the mounted volume, the server silently
  falls back to plain HTTP — no error, no TLS.
- Client side (`DefaultBaseTransport`): if `TLS_CA_FILE` is unset/empty, the
  client silently falls back to a plain `http.Transport` — no verification.

Both sides degrade consistently (not broken, just off), but this is a **silent
SC-8 compliance regression**: any `cert-manager`/`manual`-mode deployment gets
zero inter-service encryption for `gateway-tls`/`datastorage-tls`/
`kubernautagent-tls` traffic, even though the chart's own PR (#1619) auto-fixes
the equivalent gap for the DataStorage signing certificate and webhook
certificate. Roughly 11 services (signalprocessing, aianalysis,
workflowexecution, remediationorchestrator, notification, console,
effectivenessmonitor, apifrontend, authwebhook, gateway, kubernaut-agent)
mount the `inter-service-ca` ConfigMap as `optional: true`, which is only ever
populated by the `hook`-mode script today.

---

## 🔍 **Alternatives Considered**

### **Option A: Dedicated internal CA + Issuer + sync hook** ✅ **CHOSEN**

Bootstrap a self-signed root `Certificate` and a namespaced `Issuer` (type
`ca`) exclusively for inter-service mTLS. Issue the three leaf certs
(`gateway-tls`, `datastorage-tls`, `kubernautagent-tls`, ECDSA P-256, matching
hook mode's algorithm and SANs) from this dedicated Issuer — not from
`tls.certManager.issuerRef`. A `post-install,post-upgrade` Helm hook Job
copies the root CA's public cert into the `inter-service-ca` ConfigMap.

- ✅ Exact security-model parity with `hook` mode: one isolated internal trust
  boundary, decoupled from whatever real-world PKI (e.g. a public ACME issuer)
  is configured for externally-facing certs.
- ✅ 100% GitOps-compatible: `Certificate`/`Issuer` are declarative CRs
  (render fine under `helm template`, no `lookup` needed); the sync hook uses
  the same Helm-hook-to-ArgoCD-sync-phase mapping already empirically
  validated in the #1617 spike.
- ➖ More moving parts than Option B (1 root cert + 1 issuer + 3 leaf certs +
  1 sync hook vs. 3 leaf certs alone).

### **Option B: Reuse `tls.certManager.issuerRef` directly** ❌ REJECTED

Add three more `Certificate` blocks issued by the same issuer already used
for `authwebhook-cert`/`datastorage-signing-cert` — mechanically identical to
the #334 pattern.

- ✅ Zero new concepts; consistent with the precedent just merged in #1619.
- ❌ **Does not actually reduce complexity**: most consumers (signalprocessing,
  aianalysis, workflowexecution, etc.) are pure mTLS *clients* with no leaf
  cert of their own — they still need `ca.crt` synced into a shared
  ConfigMap, so the sync hook is still required.
- ❌ Couples internal, cluster-only service-to-service trust to whatever
  issuer the operator configured for public-facing certs. Architecturally
  sound if that issuer is already a private/internal CA (the common case),
  but wrong if it is a public ACME issuer (e.g. Let's Encrypt) — the entire
  public web PKI would become the cluster's internal mTLS trust root.

### **Option C: Document as a known gap, defer to service mesh** ❌ REJECTED

Leave `cert-manager`/`manual` mode without inter-service TLS; document that
operators needing mTLS should adopt Istio/Linkerd.

- ✅ Zero implementation cost.
- ❌ Regression relative to what `hook` mode already provides out of the box.
- ❌ Leaves SC-8 unmet for `cert-manager` mode with no in-chart mitigation.

---

## ✅ **Consequences**

- New template directory `charts/kubernaut/templates/interservice/` housing
  the bootstrap `Issuer`, root `Certificate`, and 3 leaf `Certificate`
  resources (cert-manager mode only).
- New hook Job (`charts/kubernaut/templates/hooks/interservice-ca-sync-job.yaml`)
  to sync `ca.crt` from the root CA's Secret into the `inter-service-ca`
  ConfigMap. Reuses the existing `{{ fullname }}-hook-sa` ServiceAccount and
  `{{ fullname }}-hook-ns-role` namespaced Role (already unconditional across
  all `tls.mode` values, already granting `get/create/update/patch/delete` on
  `secrets`/`configmaps` in the release namespace) — zero new RBAC surface.
- `manual` mode documentation updated (README, values.yaml, NOTES.txt) to
  list `gateway-tls`, `datastorage-tls`, `kubernautagent-tls`, and the
  `inter-service-ca` ConfigMap as additional user-managed prerequisites,
  mirroring the `datastorage-signing-cert` precedent from #334.
- No changes required to the ~11 consumer service Deployments — they already
  mount `inter-service-ca` as `optional: true` and will pick it up
  automatically once it exists.
- `helm-unittest` coverage for RBAC scoping, Certificate rendering, and
  correct non-rendering in `hook` mode (mirroring `tls_mode_test.yaml` from
  #334).

## 🔗 Related Decisions

- Issue #334 / PR #1619: established the `Certificate`-auto-provisioning
  pattern this DD extends.
- Issue #1617: umbrella GitOps/ArgoCD operational readiness tracking issue.
