# DD-PLATFORM-002: Fix ArgoCD PostSync/Health Deadlock for `post-install` Hook Jobs

**Date**: July 7, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 92%
**Last Reviewed**: July 7, 2026
**Related**: Issue #1617 (Helm chart GitOps/ArgoCD operational readiness), DD-PLATFORM-001, PR #1619, PR #1620

---

## 🎯 **DECISION**

**Every Helm hook resource annotated `"helm.sh/hook": post-install,post-upgrade`
that a *different* resource's runtime health depends on SHALL also carry an
explicit `argocd.argoproj.io/hook: Sync` annotation (with an equivalent
`argocd.argoproj.io/hook-delete-policy` in ArgoCD's `PascalCase` vocabulary).
This applies today to `db-migration`/`{{ fullname }}-migrations` (ConfigMap)
in `templates/hooks/migration-job.yaml` and
`{{ fullname }}-interservice-ca-sync` in
`templates/hooks/interservice-ca-sync-job.yaml`.**

**Scope**: Helm hook annotations only. No change to hook Job logic, RBAC, or
Helm CLI behavior (Helm does not recognize `argocd.argoproj.io/*` annotations
and ignores them; `helm install`/`helm upgrade` are unaffected).

---

## 📊 **Context & Problem**

### How this was found

While building the automated GitOps smoke-test workflow for issue #1617 (item
4: "a repeatable/scripted GitOps validation flow"), a live Kind + ArgoCD
v3.4.4 + cert-manager v1.20.3 dry run of `charts/kubernaut` — using
consistent (non-conflicting) prerequisite secrets, unlike the earlier #1617
spike which never got past a `PostgreSQL` password mismatch — surfaced a
previously-undetected deadlock: **DataStorage and several dependent services
never left `CrashLoopBackOff`, and the sync never completed, no matter how
long it ran.**

### Root cause

Both `db-migration` and `interservice-ca-sync` are annotated
`"helm.sh/hook": post-install,post-upgrade"`. Per
[ArgoCD's own Helm-hook mapping](https://argo-cd.readthedocs.io/en/stable/user-guide/helm/),
this is translated to `argocd.argoproj.io/hook: PostSync`. Per
[ArgoCD's documented hook semantics](https://argo-cd.readthedocs.io/en/stable/user-guide/resource_hooks/):

> `PostSync`: Executes after all `Sync` hooks completed and were successful,
> a successful application, **and all resources in a `Healthy` state**.

This is a circular dependency for this chart:

- DataStorage's pod can never become `Healthy` because its schema
  (`audit_events` and others) doesn't exist yet — confirmed via pod logs:
  `relation "audit_events" does not exist (SQLSTATE 42P01)`. The schema is
  only created by the `db-migration` Job.
- Gateway's pod can never become `Healthy` because its mandatory audit
  client (ADR-032 §1.5) can't establish mTLS to DataStorage without
  `/etc/tls-ca/ca.crt` — confirmed via pod logs:
  `failed to read CA certificate /etc/tls-ca/ca.crt: no such file or
  directory`. That file comes from the `inter-service-ca` ConfigMap, only
  created by the `interservice-ca-sync` Job.
- Both Jobs are `PostSync` hooks, which ArgoCD will not run until the
  **entire Application** — including DataStorage and Gateway — is `Healthy`.

Empirically confirmed via `kubectl get events`: after 13+ minutes, zero
`SuccessfulCreate` events exist for either Job (compare: the `crd-upgrade`
`PreSync` hook Job *did* fire and complete in seconds — `PreSync` has no such
gate). This is **not specific to `tls.mode=cert-manager`** — `db-migration`
runs unconditionally in every mode — and is **not** a #1619/#1620 regression;
it is a latent, pre-existing gap that the original #1617 spike never
exercised far enough to hit (it stopped at an unrelated password-mismatch
artifact of that spike's manual secret setup).

This is a well-documented, widely-hit class of issue, not specific to this
chart — see
[argoproj/argo-cd#17604](https://github.com/argoproj/argo-cd/issues/17604)
and equivalent reports from the Apache Airflow and OpenFGA Helm charts. The
ArgoCD maintainers' own guidance in that issue: *"The `post-install,
post-upgrade` Helm hook should really map to the `Sync` ArgoCD hook... this
maps to the ArgoCD `Sync` hook instead of the ArgoCD `PostSync` hook."*

---

## 🔍 **Alternatives Considered**

### **Option A: Add explicit `argocd.argoproj.io/hook: Sync` override** ✅ **CHOSEN**

Add `argocd.argoproj.io/hook: Sync` (+ matching `hook-delete-policy`)
alongside the existing `helm.sh/hook` annotation on the affected resources.
Per ArgoCD's docs, an explicit `argocd.argoproj.io/hook` annotation takes
precedence over the Helm-translated one. `Sync` hooks "execute... at the
same time as the application of the manifests" — not gated on any
resource's health.

- ✅ Purely additive: a brand-new annotation key. Zero effect on `helm
  install`/`helm upgrade` (Helm does not recognize `argocd.argoproj.io/*`).
- ✅ Zero risk to existing `helm-unittest` coverage (asserts on the
  `helm.sh/hook` key specifically, unaffected by an added key).
- ✅ `helm.sh/hook-weight` is separately, automatically translated to
  `argocd.argoproj.io/sync-wave` by ArgoCD, so existing relative ordering
  (ConfigMap before Job, etc.) is preserved without extra annotations.
- ✅ This is the ArgoCD-maintainer-endorsed workaround for this exact class
  of issue (see argoproj/argo-cd#17604 above).
- ➖ Two hook vocabularies now coexist on the same resource (`helm.sh/hook`
  for Helm CLI, `argocd.argoproj.io/hook` for ArgoCD) — a small ongoing
  cognitive cost, mitigated by an explanatory comment on each occurrence.

### **Option B: Drop Helm hook semantics entirely; use sync-wave only** ❌ REJECTED

Remove `helm.sh/hook`/`argocd.argoproj.io/hook` entirely and rely solely on
`argocd.argoproj.io/sync-wave` ordering for regular (non-hook) resources.

- ❌ Breaks the plain-`helm install`/`helm upgrade` path: without a hook
  annotation, Helm treats the Job as a normal immutable resource — a
  `helm upgrade` would fail with an immutable-field error on every release
  (Jobs' `spec.template` is immutable) unless further reworked (e.g.
  `generateName` + revision-based naming), a materially larger change.
- ❌ Loses `hook-delete-policy`'s "recreate on every sync" semantics, which
  the current design already gets for free from Helm's hook lifecycle.

### **Option C: Custom ArgoCD health checks so the app reports Healthy earlier** ❌ REJECTED

Author custom Lua health-check scripts (via ArgoCD's `resource.customizations`)
so DataStorage/Gateway are considered "healthy enough" before their real
readiness probes pass.

- ❌ Highest complexity: requires ArgoCD-instance-level configuration
  (`argocd-cm` ConfigMap) outside this chart's control — not portable,
  not something the chart itself can enforce for downstream GitOps users.
- ❌ Masks real health signals rather than fixing the ordering problem;
  risks hiding genuine startup failures behind a false-Healthy status.

---

## ✅ **Consequences**

- `charts/kubernaut/templates/hooks/migration-job.yaml`: `argocd.argoproj.io/hook:
  Sync` added to both the `db-migration` Job and the `{{ fullname
  }}-migrations` ConfigMap it mounts.
- `charts/kubernaut/templates/hooks/interservice-ca-sync-job.yaml`:
  `argocd.argoproj.io/hook: Sync` added to the `{{ fullname
  }}-interservice-ca-sync` Job.
- No change to Helm CLI behavior, RBAC, or hook Job logic.
- The `.github/workflows/gitops-smoke-test.yml` weekly smoke test (issue
  #1617 item 4) directly exercises this fix: it asserts the
  `inter-service-ca` ConfigMap and `db-migration`-created schema become
  available without the app ever needing to reach aggregate `Healthy` first.
- DD-PLATFORM-001's original claim that the sync-hook pattern was "already
  empirically validated in the #1617 spike" is superseded by this DD — that
  spike never actually exercised the PostSync-health interaction end-to-end
  (it stopped at an unrelated password-mismatch artifact first). This DD is
  the actual empirical validation of that interaction.
- Amendment (PR #1625, 2026-07-08): `hook-delete-policy` on both Jobs dropped
  `hook-succeeded` (Helm: `before-hook-creation,hook-succeeded` →
  `before-hook-creation`; ArgoCD: `BeforeHookCreation,HookSucceeded` →
  `BeforeHookCreation`). `before-hook-creation` alone already gives the
  "recreate on every sync" semantics this DD depends on (Option B's rejection
  above); `hook-succeeded` was an *additional*, non-required trigger that
  deletes the Job the instant ArgoCD observes it Healthy. Once the terminal
  sync-failure race from cert-manager health flakiness (see the CI fix in
  `ci-pipeline.yml`'s "Sync ArgoCD Application" step) stopped masking it, this
  was found to delete the Job before the GitOps smoke test's own verification
  step could inspect it, and to cost a real operator any post-hoc visibility
  into the last migration/CA-sync run. `ttlSecondsAfterFinished: 86400` on
  both Jobs still bounds their lifetime.

## 🔗 Related Decisions

- DD-PLATFORM-001: introduced the `interservice-ca-sync` hook whose ArgoCD
  incompatibility this DD fixes.
- DD-PLATFORM-003: changes the `argocd.argoproj.io/sync-wave` value on both
  Jobs from `"0"` (set here) to `"-1"`, as part of a broader infra-first
  phased-deployment split. The `argocd.argoproj.io/hook: Sync` fix in *this*
  DD -- which is what actually breaks the PostSync/health deadlock -- is
  unchanged and still in effect; only the wave *number* is superseded.
- Issue #334 / PR #1619: introduced the RBAC-scoping and
  `datastorage-signing-cert` auto-provisioning pattern.
- Issue #1617: umbrella GitOps/ArgoCD operational readiness tracking issue;
  this DD directly unblocks item 4 (repeatable GitOps validation flow).
