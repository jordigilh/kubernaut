# DD-PLATFORM-006: Helm Chart Configuration Surface Reduction

**Status**: 🟡 **PROPOSED** (pending user approval)
**Decision Date**: TBD (on approval)
**Version**: 5.0 (lean reconstruction)
**Date**: 2026-07-29
**Deciders**: Kubernaut Platform (chart maintainers)
**Applies To**: `charts/kubernaut` (Helm chart) only — no Kubernaut Operator changes

**Related Business Requirements**:
- BR-PLATFORM-007: Helm Chart Minimal-Configuration Onboarding (this DD implements it)
- BR-PLATFORM-009: Helm Chart Gateway/APIFrontend Ingress Parity with the Kubernaut Operator
  (already Approved, landed via PR #1755 — Decision Area 9 reviews and defers to its
  `console.ingress.enabled` default decision rather than reopening it)

**Related Design Decisions**:
- DD-PLATFORM-004: Anti-Affinity and PDB Enabled by Default (the `kubernaut.affinity`/
  `kubernaut.pdb` helper pattern this DD's Decision Area 1 extends)
- DD-PLATFORM-005: helm-unittest as a Dedicated Fast-Fail CI Gate (the CI mechanism this DD's
  render-validity tests run in)
- DD-PLATFORM-007: LLM Profile Consolidation (`global.llmProfiles`/`llmProfileRef` — landed via
  #1735/#1736; this DD's Decision Area 4 builds on it)

**Tracking Issue**: [#1743](https://github.com/jordigilh/kubernaut/issues/1743)

**Implementation Plan**: `.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`
(6 sequential PRs, RED/GREEN test cases, Wiring Manifests)

**Related Issues** (tracked separately, not superseded by this DD):
- #1725 (merged): consolidated the four per-service `fleet.enabled` toggles into
  `global.fleet.enabled` — same "global shared default, per-service override" pattern this DD
  generalizes to pod-scheduling and resources.
- #1730 (open, unassigned): proposes `global.fleet.mcpGatewayNamespace` as a shared fallback for
  `signalprocessing.fleet.namespace`/`fleetmetadatacache.namespace` — a genuine, load-bearing
  duplication (RBAC least-privilege scoping, not a hardcode candidate). Proceeds independently.
- #1729 (open): orthogonal Helm/Operator parity gap in KubernautAgent's fleet MCP tool wiring.
  Non-blocking watch item, does not change any decision below.
- #1737 / PR #1755 (merged): E2E Helm migration work that added `nodePort`, `ingress`
  (Gateway/APIFrontend, new), and NetworkPolicy `ingressCIDRs`/`ingressNamespaceSelectors` fields,
  and flipped `console.ingress.enabled`'s default `true`→`false` (formalized as BR-PLATFORM-009).
  **A full field-census recount against these new schema fields, and verification that this DD's
  file/line references still match post-merge `main`, is an outstanding follow-up** — not yet
  completed as of this reconstruction.

---

## Document History Note

This document is a **lean reconstruction** (2026-07-29) of an earlier, more detailed version
(iterated v1.0→v4.7 across a long working session) that was never committed to git — it lived only
as an untracked working-tree file and was deleted during a `git rebase`'s untracked-file cleanup.
That earlier version's superseded-alternative narrative, two Valkey-TLS pre-implementation spike
write-ups, and version-by-version changelog are not reproduced here. **No decision changed as a
result of the rewrite** — every Decision Area below is grounded in the surviving implementation
plan and this session's conversation record, which independently confirms the same final choices.

---

## Context & Problem

A field-by-field audit of `charts/kubernaut/values.schema.json` (prior to PR #1755) found ~375
leaf fields, of which only ~7-8 are truly mandatory (no safe default) — the rest are either
feature-enable toggles or already have a working default a user never needs to touch. New users
copy `values.yaml` as their starting point and cannot distinguish the handful of fields they must
set from the hundreds they can ignore, inflating perceived complexity and support burden.

Separately, several toggles gate a security/stability control this project already commits to
enforcing (FedRAMP AC-4/SC-8/AU-9 per `AGENTS.md`), with no remaining technical reason to leave
them as a self-service opt-out.

### Constraints

- **No backward-compatibility requirement**: a fresh `helm install` with the new chart must
  produce a valid, working deployment — the bar is functional correctness, not exact-output
  preservation (pre-GA, matching the precedent #1725 already set). The accepted upgrade path for
  a breaking change is `helm uninstall` + `helm install` (reinstall), not an in-place
  `helm upgrade` — except where a Decision Area explicitly adds reinstall-survival (Decision
  Area 7's `resource-policy: keep`, matching the pre-existing PostgreSQL/Valkey PVC pattern).
- **Zero capability loss**, with deliberate, security-motivated exceptions carved out in Decision
  Areas 3 and 6 — every other field must remain settable somehow (directly, via a shared
  default + override, or via an overlay file).
- No Operator (`kubernaut-operator`) changes.
- Follow the existing `kubernaut.affinity`/`kubernaut.pdb`/`global.fleet.*` merge patterns rather
  than inventing a new mechanism.

---

## Decision Areas

### Decision Area 1 — Shared PDB Default

**Decision**: add `global.podDefaults.pdb.{enabled,maxUnavailable}`; `templates/pdb.yaml` falls
back to it via `coalesce` when a service sets no `pdb` block, mirroring the existing
`kubernaut.scheduling` coalesce pattern already used for `nodeSelector`/`tolerations`. Remove the
12 services' identical, duplicated `pdb` blocks from `values.yaml` (the only field found with
genuine 100%-identical literal duplication); keep the field in the schema with the default
declared once.

**Scope note**: `affinity`/`podSecurityContext`/`nodeSelector`/`tolerations`/
`topologySpreadConstraints` were investigated and found to already have adequate global-default
handling — either an existing coalesce helper (`nodeSelector`/`tolerations`), or a hardcoded
default with zero `values.yaml` duplication to remove (`affinity`/`podSecurityContext`), or no
default merge and no duplication at all (`topologySpreadConstraints`). Out of scope, not
overlooked.

**`postgresql`/`valkey` note**: their `podSecurityContext` overrides (UID 70/`fsGroup: 70`,
required for the `postgres:16-alpine` entrypoint's `gosu`/`setpriv` use) are functionally
load-bearing, not stylistic. The deep-merge's "per-service keys win" rule already protects them by
design, but this is the explicit test case for "per-service override actually wins" in this
Decision Area's test suite, given the concrete failure mode (container crash) if that precedence
were ever inverted.

### Decision Area 2 — `global.defaultResources` (Schema Fallback Only)

**Decision**: add `global.defaultResources` as a schema-declared fallback-of-last-resort (used
only if a future service is added without its own explicit `resources` block). Do **not** remove
any of the 14 services' existing `resources` blocks from `values.yaml` — every service's current
sizing is genuinely different and workload-appropriate (e.g. console 16Mi/10m vs. kubernautAgent
256Mi/200m, confirmed field-by-field); consolidating them would either be a no-op (every service
still overrides it) or a real, unapproved sizing change.

`postgresql`/`valkey` are excluded from `global.defaultResources` entirely — a different workload
class from the 13-service Kubernaut fleet, with independently dev-sized defaults not comparable to
a "typical service" baseline.

### Decision Area 3 — NetworkPolicy Mandatory

**Decision**: remove the global `networkPolicies.enabled` toggle and all 14 per-service
`networkPolicies.<service>.enabled` toggles. NetworkPolicy becomes an unconditional resource for
every service (net **-15 leaf fields**). Where a guard also checks the *component's own* existence
toggle (`postgresql.enabled`, `valkey.enabled`, `console.enabled`, `apifrontend.enabled` — 4 of the
14), that check is retained — not rendering a NetworkPolicy for a component that isn't deployed at
all is a different, legitimate guard.

**Rationale**: a NetworkPolicy object is inert, not breaking, on a CNI that doesn't enforce it —
the API server accepts and stores it, a non-enforcing CNI simply ignores it. The original opt-in
rationale (#285, "not all clusters have a CNI that enforces NetworkPolicies") doesn't hold up
technically once this is verified. A self-service toggle that lets any installer silently disable
this project's only AC-4-relevant control is a latent compliance gap, not a neutral convenience.

**Accepted risk**: a user with an undiscovered traffic-matrix gap on an *enforcing* CNI loses the
fastest self-service escape hatch and must `kubectl delete networkpolicy/<name>` directly.
Mitigated: the traffic matrix has shipped since #285 with no reported connectivity gaps; direct
`kubectl delete` remains available even without a Helm-managed toggle.

**Pre-merge gate**: `kubernaut.np.apiServerPeers`'s `apiServerCIDR`/`apiServerCIDRs`
auto-discovery uses the `lookup` function, which returns empty under `helm template`-only
rendering (the mode ArgoCD/Flux GitOps pipelines use). This change removes the
`networkPolicies.enabled=false` escape hatch a GitOps user previously had if they hit this —
verify this repo's own ArgoCD GitOps CI step already sets `apiServerCIDR`/`apiServerCIDRs`
explicitly before this Decision Area's PR merges, not just document the risk.

### Decision Area 4 — Values.yaml Trim

**Decision**: trim the shipped `values.yaml` to mandatory fields (7) + feature-enable toggles
(~7-12) only; every already-defaulted field keeps its schema default and moves to the README's
configuration reference table instead. Acceptance gate: `helm lint` clean + `helm template`
renders without error for the trimmed file (and `+ values-fleet.yaml`) — a *different* rendered
value than before is acceptable, an error or invalid manifest is not (no backward-compatibility
requirement, pre-GA).

**`postgresql.enabled`/`valkey.enabled`**: included in the standard trim (both already default
`true`; the fully functional `--set postgresql.enabled=false --set postgresql.host=...` BYO
override, and the Valkey equivalent, remain unaffected) — documented as one README row each, not
shown in the example file. This is a visibility-only change; no schema field is removed or
renamed.

**`kubernautAgent.llmProfileRef` default (mandatory count 8→7)**: defaults to `"primary"` instead
of an unconditional `fail()`, grounded in `"primary"` already being the universal, 100%-consistent
convention across this repo's README, `quickstart.sh`, `helm-smoke-test.sh`, the CI GitOps step,
and essentially every `helm-unittest` fixture. `apifrontend.yaml`/`NOTES.txt`'s inherited
references need the same `| default "primary"` fallback-chain fix for consistency. A still-
undefined `global.llmProfiles.primary` doesn't go unguarded — `kubernaut.llm.resolveProfile`'s
existing `fail()` fires instead, with a more actionable message. The guarantee moves from a
template `fail()` to a schema-level requirement on whichever profile ends up referenced; it isn't
removed.

**New `values-fleet.yaml`**: a Fleet-Federation-specific overlay (~19 fields: `global.fleet.*` +
per-service `fleet.oauth2.credentialsSecretRef`/`namespace` overrides), following
`values-airgap.yaml`'s header-comment style but with a working `-f values.yaml -f
values-fleet.yaml` example.

**The 7 mandatory fields**: `aianalysis.policies.{content,existingConfigMap}` and
`signalprocessing.policies.{content,existingConfigMap}` (either/or pairs, unconditional `fail()`),
and whatever profile `kubernautAgent.llmProfileRef` resolves to (by default `"primary"`, or
explicitly) must itself supply `global.llmProfiles.<name>.{provider,model,
credentialsSecretName}` (schema `required`, enforced by `helm lint --strict`) — `global.llmProfiles`
ships as `{}` with no default profile, so there is nothing for `credentialsSecretName` in
particular to fall back to.

### Decision Area 5 — Remove Dead/Non-Configurable Fields

**Decision**: remove two fields that imply a live control but aren't one:
- `tls.interService.enabled` — a repo-wide grep confirms no template ever reads this boolean;
  inter-service TLS for Gateway/DataStorage/FleetMetadataCache/KubernautAgent's REST APIs is
  unconditionally mandatory, hardcoded directly into the `kubernaut.<service>.url` helpers by
  issues #753 and #1683, predating this DD. `certDir`/`caFile` siblings are retained — those are
  genuinely consulted path overrides, unlike `enabled`.
- `gateway.config.cors.allowedMethods` — hardcoded 1:1 to the REST route surface; protocol-
  plumbing coupled to the API's actual routes, not an independent deployment choice. Replace the
  `{{- range .Values... }}` with a literal method list matching Gateway's actual routes.

### Decision Area 6 — Mandatory Security-Relevant Toggles

**Decision**: make four toggles unconditional (their sibling tuning fields remain user-adjustable
— only the `enabled` gate is removed):

| Field | Control | Prerequisite |
|---|---|---|
| `datastorage.config.server.rateLimit.enabled` | Per-IP DoS protection (GAP-09) | None — immediate |
| `apifrontend.config.auth.replayCache.enabled` | JWT replay-attack detection (GAP-08) | None — Valkey already load-bearing |
| `datastorage.config.auditHashKey.enabled` | Audit log tamper-evidence, AU-9 | Decision Area 7 |
| `datastorage.config.redis.tls.enabled` | DataStorage↔Valkey transport encryption, SC-8 | Decision Area 8 |

**Explicitly excluded** (feature/topology toggles, not security controls — no change):
`global.fleet.enabled`, `fleetmetadatacache.enabled` (see Decision Area 10), `console.enabled`,
`apifrontend.enabled`, all `<service>.autoscaling.enabled`, all `monitoring.*.enabled`,
`kubernautAgent.interactive.enabled` (see Decision Area 11), `workflowexecution.config.tekton`,
`datastorage.config.retention.enabled` (not purging is the *safer* default against AU-11's 7-year
floor), `kubernautAgent.alignmentCheck.enabled` (a genuine cost/benefit trade-off — 2x LLM calls —
left as an opt-in judgment call), `global.fleet.oauth2.enabled` (verified mandatory Go-side
already, via `pkg/fleet/config.go`'s `Validate()` and its test `UT-FLEET-CFG-061` — FMC uses a
stricter, separate config type per ADR-068; deliberate, tested design, not a bug). Also settled by
prior DDs, not reopened: `<service>.pdb.enabled` (DD-PLATFORM-004's own on-record rationale),
`postgresql.enabled`/`valkey.enabled` (topology choice, see Decision Area 4 and "Considered and
Declined" below).

### Decision Area 7 — Audit-HMAC Key Auto-Generation (prerequisite for Decision Area 6)

**Decision**: extend `templates/hooks/tls-cert-job.yaml`'s existing idempotent-generate Job with a
new section: if the audit-HMAC Secret is missing, generate it (`openssl rand -base64 32`) and
create it; if present, skip — **exists-only, never expiry-based** (unlike this file's TLS
sections — an HMAC key rotating invalidates the ability to re-verify the existing hash chain's
earlier entries; this distinction must not be lost to a copy-paste of the TLS sections' logic).

**Reinstall survival**: give the Secret `helm.sh/resource-policy: keep`, matching
`postgresql-data`/`valkey-data`'s existing pattern. This project's reinstall-not-upgrade model
means every Helm-managed Secret is normally deleted on `helm uninstall` and regenerated fresh on
the next `helm install` — fine for TLS certs, but not for this key: AU-9 hash chaining is a
cryptographically continuous sequence over Postgres audit records, which *do* survive reinstall
(their PVC already has `resource-policy: keep`). Without the same annotation here, a reinstall
would silently break hash-chain verifiability at the reinstall boundary. The exists-only check and
the annotation are both required together — either alone is insufficient.

### Decision Area 8 — Valkey Server-Side TLS (prerequisite for Decision Area 6)

**Decision**: add native TLS to the in-chart Valkey Deployment:
1. Extend `tls-cert-job.yaml`'s per-service CA-signed cert loop to include Valkey (same
   `inter-service-ca` trust root already distributed to every consumer).
2. `templates/infrastructure/valkey.yaml` disables the plaintext port (`--port 0`), enables
   `--tls-port 6380` with the generated cert/key/CA (Valkey/Redis has supported native TLS since
   Redis 6.0 — no third-party sidecar needed).
3. `readinessProbe`/`livenessProbe` switch to TLS-aware `valkey-cli --tls --cacert ... -p 6380
   ping` — bare `valkey-cli ping` would break immediately once the plaintext port is gone.
4. `kubernaut.valkey.addr` switches to the TLS port and the already-distributed `inter-service-ca`
   trust bundle — no new client-side cert provisioning needed; one-way TLS suffices, matching
   `pkg/datastorage/config`'s `RedisTLSConfig`, whose client cert/key are already optional.

**Validated by two pre-implementation spikes** against the real shipped `valkey/valkey:8-alpine`
image, the real chart template, and a real Kind cluster rolling update:
- **Spike 1 (fresh-install correctness)**: confirmed TLS is genuinely compiled into the shipped
  image (a config-completeness error, not an "unsupported" error, on a negative-control test); a
  CA-verified TLS client round-trip succeeds; the plaintext port is genuinely refused once
  disabled; a client presenting no certificate still succeeds (one-way TLS confirmed sufficient).
  Surfaced the `readinessProbe`/`livenessProbe` breakage above.
- **Spike 2 (`helm upgrade`-against-existing-data survival)**: on an isolated Kind cluster,
  installed the real, unmodified `valkey.yaml` plaintext, wrote and persisted a test key, then
  applied the prototyped TLS manifest to the same Deployment/PVC/Service names (the same mechanism
  `helm upgrade` uses). The new pod reached Ready with zero restarts; the pre-upgrade key returned
  unchanged through the new TLS listener; the PVC was confirmed `unchanged`, not recreated.

**Confidence**: 92% — both fresh-install correctness and upgrade-path data survival are proven
against the real image and real chart template, not merely designed.

### Decision Area 9 — Console External-Access Consistency

**Finding 1 — `console.ingress.enabled`'s default**: incoming PR #1755 flips this default from
`true` to `false`, formalized in the already-Approved BR-PLATFORM-009 (Helm Chart Gateway/
APIFrontend Ingress Parity). An initial counter-argument was raised during review — `console.
ingress.host` is unconditionally required whenever `console.enabled=true` regardless of
`ingress.enabled` (a pre-existing `fail()` guard, unchanged by #1755), so the "expose nothing by
default" rationale doesn't fully hold, and an Ingress object with no matching controller is inert,
not harmful (the same reasoning accepted for NetworkPolicy in Decision Area 3).

**Decision**: defer to BR-PLATFORM-009's rationale rather than override it. That BR's actual
justification is platform-consistency — Console is optional, replaceable UI tooling, not a
pipeline component a deployment depends on, and should follow the same opt-in exposure posture as
Gateway/APIFrontend's new Ingress resources (also introduced by the same BR). This is a legitimate
values trade-off between two reasonable defaults, and BR-PLATFORM-009 is another workstream's
already-approved decision — **no change to `console.ingress.enabled`'s default**. Onboarding
friction is mitigated instead via a `templates/NOTES.txt` hint, conditional on
`console.enabled=true` and `console.ingress.enabled=false`, pointing the user at
`--set console.ingress.enabled=true` or `kubectl port-forward` as their two options.

**Finding 2 — `console.enabled` has no dependency on `apifrontend.enabled`**: Console's nginx
sidecar hardcodes a reverse-proxy straight to APIFrontend's in-cluster Service
(`kubernaut.console.apifrontendURL`), with no other possible backend. The existing `fail()` guards
in `console.yaml` check that an OIDC issuer *value* is present, not that the APIFrontend
*workload* is actually deployed — a user can set `console.enabled=true` +
`apifrontend.enabled=false` and get a Helm render that succeeds but a Console deployment that is
completely non-functional.

**Decision**: add a fourth `fail()` guard to `console.yaml`, same style as the three existing
ones: `console.enabled=true` requires `apifrontend.enabled=true`. Verified no existing test sets
`apifrontend.enabled=false` alongside `console.enabled=true` — zero regression risk.

### Decision Area 10 — Derive `fleetmetadatacache.enabled`

**Finding**: `fleetmetadatacache.enabled` has no validation tying it to `global.fleet.backend`.
`global.fleet.backend` defaults to empty string, which the Gateway/RemediationOrchestrator
templates resolve via `default "fleetmetadatacache" $preamble.config.backend` — i.e. the *default*
fleet configuration already assumes FleetMetadataCache is the backend. But
`fleetmetadatacache.enabled` independently defaults to `false`, and nothing (chart or Go-side —
`pkg/fleet/config.go`'s validation has no cross-service topology visibility) catches the
combination `global.fleet.enabled=true` + backend resolving to `fleetmetadatacache` +
`fleetmetadatacache.enabled=false`. Today this surfaces as a silent runtime connection failure,
not a `helm template`-time error.

**Decision**: derive `fleetmetadatacache.enabled`'s default from `global.fleet.enabled` +
`global.fleet.backend` via a new `_helpers.tpl` helper
(`kubernaut.fleetmetadatacache.effectiveEnabled`) rather than just guarding the omission after the
fact — there is no scenario where the combination is true and FMC should not run, so removing the
manual step entirely (not just catching its absence) better serves the minimal-knob goal. An
explicit, contradictory override (`enabled: false` while the derived default would be `true`) is
detected via `hasKey` (not `coalesce`, which can't distinguish "unset" from "explicitly false")
and still fails loudly — that combination has no sane interpretation.

### Decision Area 11 — `kubernautAgent.interactive.enabled` requires `apifrontend.enabled`

**Finding**: `kubernautAgent.interactive.enabled` has no dependency on `apifrontend.enabled`, even
though APIFrontend's `pkg/apifrontend/ka/` (`ka.NewSDKMCPClient`) is the only MCP client anywhere
in the codebase that connects to KubernautAgent's interactive `/api/v1/mcp` endpoint (verified:
zero other callers).

**Decision**: add a `fail()` guard — `kubernautAgent.interactive.enabled=true` requires
`apifrontend.enabled=true`, since APIFrontend is currently the sole consumer, making the dependency
explicit rather than an unenforced possibility that renders successfully but can never be used.

---

## Considered and Declined: Removing `postgresql`/`valkey` from the Chart

Removing `postgresql`/`valkey` from the chart entirely — making an externally-provisioned
database/cache a hard deployment prerequisite instead of an in-chart option — was raised and
declined. Bundling a stateful dependency on-by-default for convenience, with a clearly-labeled BYO
toggle for production (exactly this chart's current `postgresql.enabled`/`valkey.enabled` + `.host`
pattern), is the dominant convention for "app + supporting stateful service" charts (GitLab,
Harbor, Keycloak, Airflow, and similar) — removing the bundled option entirely is the minority
pattern, more typical of Operator-centric platforms that deliberately don't own data-tier
lifecycle at all. Kubernaut's hand-rolled-templates approach (vs. a third-party sub-chart, e.g.
`bitnami/postgresql`) additionally avoids a real, current supply-chain risk (Bitnami's 2025
registry/licensing changes broke a number of downstream charts that depended on it). No ADR
needed; this is closed, not deferred.

---

## Consequences

### Positive
1. New-user-facing `values.yaml` shrinks to roughly 7 mandatory fields + ~7-12 feature-enable
   toggles, from ~375 total leaf fields today.
2. Decision Areas 1-2 close real value-level duplication on top of an already-shared schema
   shape.
3. Decision Area 3's `enabled`-toggle removal closes a latent AC-4/SC-8 compliance gap as a
   byproduct of the same onboarding-friction-reduction pass, not a separate effort.
4. `values-fleet.yaml` gives Fleet Federation its own minimal on-ramp.
5. Decision Areas 7-8 deliver two independently-useful capabilities (audit-HMAC-key
   auto-provisioning, Valkey transport encryption) as a byproduct of unblocking Decision Area 6.
6. Decision Areas 10-11 close two silent-misconfiguration gaps (FMC/backend mismatch,
   Console-without-APIFrontend) that previously rendered successfully but produced a
   non-functional or unreachable deployment.

### Negative / Accepted
1. Existing pre-GA Helm deployments may need a reinstall (`helm uninstall` + `helm install`)
   rather than an in-place `helm upgrade` — explicit, accepted trade-off, matching the #1725
   precedent.
2. A user who previously disabled NetworkPolicy or a Decision Area 6 toggle for an undiscovered
   reason loses that self-service escape hatch — mitigated by `kubectl delete
   networkpolicy/<name>` remaining available, and by each removed toggle's sibling tuning fields
   staying adjustable.

---

## Compliance

| Requirement | Status | Notes |
|---|---|---|
| BR-PLATFORM-007 | 🟡 Pending | This DD is the implementation design for that BR |

---

## Validation Strategy

1. **Render-validity gate** (all areas): `helm lint` clean, `helm template` renders without error
   for the trimmed default `values.yaml` and `values.yaml` + `values-fleet.yaml`.
2. **helm-unittest additions**: per-service merge-behavior cases for Decision Areas 1-2; explicit
   "key entirely absent" cases for every shared helper touched by Decision Area 4's trim.
3. **Decision Area 6-specific**: `helm-unittest` asserting each of the four security-relevant
   config blocks renders unconditionally, with the `enabled` guard fully absent from the template.
4. **Decision Area 7-specific** (mandatory): a Kind-based `helm upgrade` test asserting the
   audit-HMAC Secret's value is byte-identical before and after an upgrade with no other changes.
5. **Decision Area 8-specific**: CI-integrated (not just locally-run) Kind-based `helm upgrade`
   test exercising the plaintext→TLS transition through the real `helm upgrade` command.
6. See the implementation plan (`.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`)
   for the full RED/GREEN test breakdown and pre-assigned Test Scenario IDs per PR.

---

## References

- `charts/kubernaut/values.schema.json`, `charts/kubernaut/values.yaml`,
  `charts/kubernaut/values-airgap.yaml`, `charts/kubernaut/templates/**`
- DD-PLATFORM-004: Anti-Affinity and PDB Enabled by Default
- DD-PLATFORM-005: helm-unittest as a Dedicated Fast-Fail CI Gate
- BR-PLATFORM-007: Helm Chart Minimal-Configuration Onboarding
- BR-PLATFORM-009: Helm Chart Gateway/APIFrontend Ingress Parity with the Kubernaut Operator
- Issues #1725 (merged), #1730 (open), #1729 (open), #1737/#1755 (merged)
- Implementation plan: `.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`

---

**Document Version**: 5.0 (lean reconstruction)
**Last Updated**: 2026-07-29
**Status**: 🟡 Proposed — awaiting user approval. Outstanding follow-up: full field-census
recount and file/line-reference verification against `main` post-PR #1755 (see "Related Issues"
above) — not yet completed as of this reconstruction.
