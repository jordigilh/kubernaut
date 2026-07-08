# DD-PLATFORM-004: Anti-Affinity and PDB Enabled by Default

**Date**: July 8, 2026
**Status**: ✅ **APPROVED**
**Confidence**: 96%
**Last Reviewed**: July 8, 2026
**Related**: Issue #1617 (Helm chart GitOps/ArgoCD operational readiness), Kubernaut Operator
(`kubernaut-operator/internal/resources/deployments.go`, `pdb.go`), PR A (apifrontend/
fleetmetadatacache hardening gaps)

---

## 🎯 **DECISION**

**Every Kubernaut Helm chart service that renders a Deployment SHALL get a
default soft (preferred, weight 100) pod anti-affinity spreading its
replicas across nodes by its own selector labels, and `pdb.enabled` SHALL
default to `true` with `maxUnavailable: 1` for every service already
covered by `templates/pdb.yaml`. Both remain fully overridable per-service
via `values.yaml`.**

Both defaults are additive, soft, and safe at `replicas: 1` (the chart's
default everywhere): a `preferred` (not `required`) anti-affinity term never
blocks scheduling when there's no other replica to avoid, and
`maxUnavailable: 1` (never `minAvailable`) always permits a single-replica
pod to be voluntarily evicted rather than paradoxically blocking it forever.

---

## 📊 **Context & Problem**

Auditing the Helm chart against the Kubernaut Operator's defaults
(`kubernaut-operator/internal/resources/deployments.go`,
`preferredPodAntiAffinity`; `pdb.go`, `PodDisruptionBudgets`) found the
Operator ships **unconditional** soft anti-affinity and a PDB
(`MaxUnavailable: 1`) for every component, while the Helm chart shipped both
as opt-in, `enabled: false` (or the field didn't exist in some cases before
PR A). This is a meaningful operational-readiness gap: a chart deployed with
defaults has no protection against a single `kubectl drain` or a
co-scheduling accident taking out every replica of a service at once —
exactly the class of incident PDBs and anti-affinity exist to prevent — and
it's inconsistent with the Operator installation path that's supposed to be
at parity.

A **secondary, pre-existing bug** was found in `values.yaml` while making
this change: `datastorage.pdb` already defaulted to `enabled: true`, but
used `minAvailable: 1` rather than `maxUnavailable: 1`. With
`datastorage.replicas: 1` (the default), `minAvailable: 1` means "at least 1
of 1 must remain available" — i.e. **zero voluntary disruptions are ever
permitted** (`PodDisruptionBudgetAtLimit`), which silently blocks
`kubectl drain`, cluster-autoscaler node consolidation, and rolling
node upgrades indefinitely for that pod. This is exactly the failure mode
the Kubernaut Operator's own PDB test explicitly guards against:

```go
// kubernaut-operator/internal/resources/pdb_test.go
Expect(pdb.Spec.MinAvailable).To(BeNil(),
  "PDB %q should not have MinAvailable set (causes PodDisruptionBudgetAtLimit with 1 replica)", pdb.Name)
```

This DD's new uniform default (`maxUnavailable: 1`, never `minAvailable`)
fixes that latent bug as part of the same change, rather than leaving one
service on the broken pattern while introducing the correct one everywhere
else.

---

## 🔍 **Alternatives Considered**

### **Option A: Match the Operator — enable both, unconditionally, via helper defaults** ✅ **CHOSEN**

Change the `kubernaut.affinity` helper to inject a default
`podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution` term
(`weight: 100`, `topologyKey: kubernetes.io/hostname`, `matchLabels` = the
component's own pod selector labels), deep-merged with any user-supplied
`<service>.affinity` override via Sprig's `merge` (user keys win; sibling
keys merge additively — validated in a scratch-chart spike). Flip
`pdb.enabled` to `true` and set `maxUnavailable: 1` as the shipped default
for every one of the 13 services in `templates/pdb.yaml`'s loop.

- ✅ Matches the Operator's proven, already-in-production defaults —
  closes the parity gap this refactor pass is explicitly chasing.
- ✅ Safe at `replicas: 1`: `preferred` anti-affinity is a no-op with no
  peer replica to avoid; `maxUnavailable: 1` always permits eviction of the
  sole pod (unlike `minAvailable: 1`, see above).
- ✅ Fully overridable: a user can still set `<service>.affinity` to
  anything, and `<service>.pdb.enabled=false` to opt out of the PDB
  entirely, exactly as before.
- ⚠️ **Known limitation, accepted**: the merge (Sprig `merge`, backed by
  `mergo`) treats an empty list or `null` as an unset zero-value, so an
  explicit `podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution:
  []` override does **not** suppress the default term — mergo falls back to
  the default's non-zero list regardless (verified via an isolated spike).
  A user who wants genuinely different anti-affinity behavior must supply
  their own **non-empty** `preferredDuringSchedulingIgnoredDuringExecution`
  list, which *does* fully replace the default (also verified). This is
  accepted rather than worked around because the default term is `preferred`
  (soft) and therefore never blocks scheduling — there is no scenario where
  a user's inability to fully null it out causes a deployment failure, only
  a harmless, ignorable scheduler preference.
- ✅ Fixes the pre-existing `datastorage` `minAvailable`/`maxUnavailable`
  inconsistency as a natural side effect of unifying on one pattern.
- ➖ Behavior change for existing installs that `helm upgrade` without
  pinning these values explicitly (mitigated: both changes are soft/
  best-effort at the replica counts the chart ships by default, and are
  exactly what the Operator path already does).

### **Option B: New explicit global toggle (`global.hardening.antiAffinityEnabled` / `.pdbEnabled`)** ❌ REJECTED

Add a chart-wide switch defaulting to `true`, with per-service overrides
layered on top.

- ❌ Adds a new configuration axis and merge-precedence rules
  (global-vs-per-service) for something the existing per-service
  `<service>.affinity` / `<service>.pdb.enabled` fields already fully
  express — pure added complexity with no behavioral benefit over Option A.
- ❌ Diverges further from the Operator, which has no such global toggle
  either.

### **Option C: Leave both opt-in, only fix the `datastorage` `minAvailable` bug** ❌ REJECTED

Minimal fix: just correct `datastorage.pdb` to use `maxUnavailable: 1`, and
leave every other service (plus anti-affinity for all 15) untouched.

- ❌ Doesn't close the actual gap this refactor pass set out to address
  (Operator parity, operational readiness by default) — leaves 12 more
  services with no PDB and every service with no anti-affinity by default.
- ➖ Lowest risk / smallest diff, but was rejected because the risk of
  Option A is already minimal (soft/best-effort, matches an existing
  production reference implementation) and the operational upside
  (protecting every default install, not just users who discover and flip
  these settings) is significant.

---

## ✅ **Consequences**

- `charts/kubernaut/templates/_helpers.tpl`: `kubernaut.affinity` signature
  changed from `(include "kubernaut.affinity" .Values.<service>)` to
  `(include "kubernaut.affinity" (dict "component" .Values.<service>
  "matchLabels" (dict "app" "<name>")))`, injecting the default
  `preferredDuringSchedulingIgnoredDuringExecution` anti-affinity term
  merged with any user override.
- All 15 call sites (13 pre-existing + `apifrontend`/`fleetmetadatacache`
  from PR A) updated to pass their own component's actual pod-selector
  `matchLabels` (`app: <name>` for 14 services; `app.kubernetes.io/name:
  fleetmetadatacache` for the one service using that selector scheme).
- `charts/kubernaut/values.yaml`: `pdb.enabled` flipped from `false` to
  `true`, and `maxUnavailable: 1` (previously commented out) uncommented,
  for all 12 services that previously had `pdb.enabled: false`.
  `datastorage.pdb` changed from `minAvailable: 1` to `maxUnavailable: 1`
  (bugfix, see Context above). `console`'s and `fleetmetadatacache`'s PDBs
  still only actually render when the service itself is enabled (existing
  `pdb.yaml` guard, unchanged).
- `charts/kubernaut/values.schema.json`: `#/definitions/pdb.properties.
  enabled.default` changed from `false` to `true` to match.
- No change to `postgresql`/`valkey`: they already had `kubernaut.affinity`
  wired (now also gain the default anti-affinity), but are intentionally
  **not** added to `templates/pdb.yaml`'s loop by this DD — they're
  typically deployed as a single, disable-and-bring-your-own-infra
  component (`postgresql.enabled` / `valkey.enabled`), and a PDB was not
  part of the Operator-parity gap identified for this pass. Can be
  revisited separately if needed.
- `helm-unittest` extended to cover: default anti-affinity rendering (and
  merge behavior with a user override) for a representative sample of
  services, default PDB `maxUnavailable: 1` rendering, and the
  `datastorage` `minAvailable`→`maxUnavailable` regression guard.
- Validated: `helm lint` and `helm template` clean across all three
  `tls.mode` values; full `helm-unittest` suite green.

## 🔗 Related Decisions

- PR A (apifrontend/fleetmetadatacache hardening gaps): added the
  `kubernaut.affinity` / `pdb` wiring to these two services that this DD's
  helper-signature change and default flip then apply to, alongside every
  other service.
- Issue #1617: umbrella GitOps/ArgoCD operational readiness work this
  chart-refactor pass (PRs A/B/C) is part of.
