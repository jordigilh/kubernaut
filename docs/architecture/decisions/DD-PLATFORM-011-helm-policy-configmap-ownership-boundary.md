# DD-PLATFORM-011: Helm Policy ConfigMap Ownership Boundary

**Date**: 2026-09-28
**Status**: ✅ **APPROVED**
**Confidence**: 94% (deterministic chart behavior and the Helm prune path were
validated on a temporary Kind cluster; full application rollout health remains
pending CI image setup)
**Related**: Issue #2460, BR-PLATFORM-005, BR-PLATFORM-010, DD-PLATFORM-006

---

## 🎯 Decision

The Helm chart SHALL keep a stable, chart-owned identity for each policy ConfigMap:

| Policy | Reserved chart-owned identity | Required external key |
|---|---|---|
| AIAnalysis approval policy | `aianalysis-policies` | `approval.rego` |
| SignalProcessing classification policy | `signalprocessing-policy` | `policy.rego` |

When `content` is supplied, the chart renders and owns the reserved ConfigMap. When
`existingConfigMap` is supplied, its value MUST be a different, valid ConfigMap name.
The chart mounts that external ConfigMap and does not render the reserved ConfigMap.

`values.schema.json` is the authoritative ownership guard. It rejects an
`existingConfigMap` equal to the corresponding reserved identity before Helm renders
or applies any resources. The guard is declarative and deterministic; it does not
depend on `lookup`, ArgoCD live state, labels, or field-manager ordering.

Same-name ownership transfer is not supported as a one-step upgrade. For a healthy
chart-managed installation, the safe handoff is two phases: create the external
ConfigMap under a distinct name, then upgrade with that name and wait for both
controller rollouts. If an earlier unsafe upgrade already removed a chart-managed
ConfigMap, first restore chart ownership with valid `content` and an empty
`existingConfigMap`, wait for the controllers to recover, and then perform the
two-phase handoff.

---

## Context

The previous chart behavior allowed an upgrade to change a policy value from chart
content to `existingConfigMap` while retaining the same ConfigMap name. The new
manifest then omitted the ConfigMap, but the Deployment continued mounting that
identity. Helm correctly treated the previously chart-managed object as absent from
the target manifest and pruned it, leaving the controller without its policy volume.

Labels do not change Helm's resource identity or deletion decision. A
`helm.sh/resource-policy: keep` annotation alone would preserve an object but would
not establish a valid ownership contract or prevent a Deployment from mounting a
missing/incorrect policy. A live `lookup` ownership check is also unsuitable as the
primary guard because GitOps renderers must be able to validate the desired state
without cluster access.

---

## Alternatives considered

### A. Reserved-name schema guard plus distinct external names — **chosen**

- ✅ Deterministic under `helm template`, Helm CLI, and ArgoCD.
- ✅ Prevents the unsafe same-identity ownership switch before apply.
- ✅ Preserves the existing flexible externalization contract; this is not a blanket
  fixed-name restriction.
- ✅ Helm may safely prune the old chart-owned ConfigMap during a valid migration,
  because the Deployment switches to a separately named, pre-created external object.
- ➖ Existing same-name externalization requires the documented two-phase recovery.

### B. Live `lookup` ownership validation

- ❌ GitOps rendering cannot rely on live cluster state.
- ❌ Behavior differs between `helm template`, ArgoCD dry-run, and live Helm.
- ✅ Useful only as optional operational diagnostics, not as the safety boundary.

### C. `helm.sh/resource-policy: keep` on policy ConfigMaps

- ❌ Does not prevent the Deployment from continuing to reference a resource whose
  ownership contract changed.
- ❌ Leaves stale chart-managed objects behind and obscures lifecycle ownership.
- ➖ Could be a supplementary retention choice for a separately justified recovery
  resource, but is not the fix for Issue #2460.

### D. Same-name takeover with a hook/bridge/retention sequence

- ✅ Could preserve compatibility for operators that require the reserved name.
- ❌ Requires multi-step ordering and ownership/field-manager coordination that is
  fragile under GitOps reconciliation and rollback.
- ❌ Adds lifecycle machinery when distinct names solve the safety problem directly.
- **Deferred** unless a later requirement makes same-name externalization mandatory.

---

## Consequences

- Existing chart-managed installs keep their reserved names and default behavior.
- External policy ConfigMaps must use distinct names and must exist before the upgrade.
- A valid migration can prune the old chart-managed ConfigMap without wedging either
  controller; structural tests prove the rendered mount and live smoke tests prove
  Helm's upgrade/prune path and rollout health.
- Operators attempting the previously unsafe same-name transition receive a schema
  failure before resources are changed.
- The chart cannot automatically validate the contents or existence of an external
  ConfigMap during a GitOps render. Documentation and post-upgrade rollout checks
  make that prerequisite explicit.

---

## Implementation and verification

- Add draft-07 `not`/`required`/`const` guards for the two policy fields in
  `charts/kubernaut/values.schema.json`.
- Add helm-unittest cases proving both reserved names fail at schema level and
  distinct names still mount successfully and are reflected in SignalProcessing's
  generated policy configuration.
- Add a live smoke flow covering rejected same-name upgrade, preservation of the
  chart-managed ConfigMaps after rejection, and successful distinct-name migration
  with external ConfigMaps and healthy rollouts.
- Update chart values, generated values reference, and upgrade/recovery documentation.
- Map the validation to SI-10 and Helm lifecycle requirements in the test plan.
