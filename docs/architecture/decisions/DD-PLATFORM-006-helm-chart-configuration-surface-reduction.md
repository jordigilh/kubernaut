# DD-PLATFORM-006: Helm Chart Configuration Surface Reduction

**Status**: 🟡 **PROPOSED** (pending user approval)
**Decision Date**: TBD (on approval)
**Version**: 5.7 (PR5/DA8 pre-implementation finding, new Decision Area 13: Decision Area 8's
Valkey TLS-only cutover would have permanently broken APIFrontend's replay-cache Valkey client,
which had zero TLS support in Go — closed with a small, in-scope Go change mirroring the fleet's
existing shared TLS pattern, not deferred or worked around with a dual-listener compromise)
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
  Field-census recount and file/line-reference verification against post-merge `main` completed —
  see Decision Area 12.

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

**Pre-merge gate — verified, not just documented**: `kubernaut.np.apiServerPeers`'s
`apiServerCIDR`/`apiServerCIDRs` auto-discovery is guarded by `kubernaut.hasClusterAccess`
(a `lookup "v1" "Namespace" "" "kube-system"` probe), which is false under `helm template`-only
rendering (the mode ArgoCD/Flux GitOps pipelines use) — so the `fail()` for "could not
auto-discover" is structurally unreachable in that mode regardless of whether
`apiServerCIDR`/`apiServerCIDRs` is set. Confirmed empirically: this repo's own ArgoCD GitOps CI
leg (`ci-pipeline.yml`'s `Create ArgoCD Application` step) does **not** set `apiServerCIDR`/
`apiServerCIDRs` in its `valuesObject` and relies on `networkPolicies.enabled`'s prior default of
`true` (never overridden to `false` there) — i.e. this exact code path (mandatory NetworkPolicies,
no CIDR override, GitOps rendering) is already exercised by that currently-green CI leg today.
Removing the `networkPolicies.enabled=false` escape hatch changes nothing for it. No CI change
required for this Decision Area.

### Decision Area 4 — Values.yaml Trim

**Decision**: trim the shipped `values.yaml` to mandatory fields (7) + feature-enable toggles
(~7-12) only; every already-defaulted field keeps its schema default and moves to an
**auto-generated** reference doc instead of a hand-authored README table (see "Configuration
reference generation" below). Acceptance gate: `helm lint` clean + `helm template` renders without
error for the trimmed file (and a helm-unittest fixture with the Fleet fields below set inline) —
a *different* rendered value than before is acceptable, an error or invalid manifest is not (no
backward-compatibility requirement, pre-GA).

**Configuration reference generation (revises the plan's original "moves to the README's
configuration reference table" framing, per direct user instruction)**: hand-transcribing ~394
fields into README tables was found to not scale. `charts/kubernaut/README.md` is already 895
lines, of which the existing "Configuration Reference" section (a curated, not exhaustive, subset)
is already 385 lines; exhaustively covering every trimmed field the same way would push the file
past 1,500 lines. Neither `helm show values` nor `helm show all` renders `values.schema.json`'s
`description` fields, so once a field's inline YAML comment is deleted from the trimmed
`values.yaml`, standard Helm tooling can no longer surface it at all — some external reference is
required, not optional.

The repo already has an established, working pattern for exactly this shape of problem:
`generate-crd-docs`/`gen-diff` (Makefile) auto-generates `docs/generated/crds.md` from Go API
types via `crd-ref-docs`, committed to git, staleness-checked in CI (`make gen-diff` fails the
build if the generated file doesn't match a fresh run). This decision mirrors that pattern instead
of introducing a new one: a new small Go generator (`hack/gen-helm-config-docs/`, no new external
dependency — pure `encoding/json` walking `values.schema.json`'s `properties`/`definitions`/
`$ref`/`required`) emits `docs/generated/helm-values-reference.md`, one table per top-level
service (mirroring the README's existing per-service `###` structure) with Parameter/Type/
Description/Default/Required columns sourced directly from the schema. Wired into the existing
`generate`/`gen-diff` targets — no new CI workflow needed. Because the generator walks the schema
itself rather than a hand-maintained list, it closes **both** directions of drift discussed above
for the Fleet fields: a renamed/removed field disappears from the next generated run (no stale
row), and a newly-added field appears automatically (no missing row) — stronger than the
`additionalProperties: false`-only protection available to hand-authored content.

README's existing Fleet section keeps its genuinely narrative content (the "sole source of truth"
architecture explanation, the `mcpGatewayEndpoint`-is-required-for-which-services callout); only
its exhaustive field-listing table is replaced with a pointer to
`docs/generated/helm-values-reference.md`'s Global section plus one short worked example.

Output location is `docs/generated/` (matching `crds.md`'s existing convention) rather than inside
`charts/kubernaut/` — deliberately different from the Fleet-overlay-file reasoning earlier in this
Decision Area, because this is a *passive reference* a user consults optionally (readable on
GitHub, or by browsing a git checkout), not a file required to *complete* an install via `-f`; the
OCI/airgap unreachability concern that ruled out a shipped `values-fleet.yaml` doesn't apply here.

**CI enforcement, corrected after checking the actual precedent (per direct user instruction —
"this should be part of the release phase: if any drift is found the release should fail")**: the
claim above that "the existing `gen-diff` CI gate covers it automatically" doesn't hold up under
verification and is retracted. `generate-crd-docs` is never invoked by any `.github/workflows/*`
file — `docs/generated/crds.md` can silently drift today with zero CI enforcement, a pre-existing
gap unrelated to this DD (out of scope to fix here, flagged for awareness only). Separately, simply
chaining the new target onto the plain `generate` target wouldn't be sufficient even if `generate`
itself were CI-enforced: `ci-pipeline.yml`'s Go-codegen step is cache-gated by a hash of
`api/**/*.go`/`pkg/shared/types/**/*.go`/OpenAPI files only, which excludes
`values.schema.json` — a schema-only change could get a false cache-hit and skip regeneration
silently. The actual design: a new, uncached, dedicated step in both `ci-pipeline.yml` (every PR —
`ci-pipeline.yml` has no path filter excluding chart changes; it already runs `helm lint --strict`/
`helm unittest` unconditionally) and `chart-release.yml` (currently has zero `make` steps of any
kind, on `chart-v*` tags only — a release-time backstop for a tag ever cut from a commit that
bypassed PR CI), each independently running `make generate-helm-config-docs` then `git diff
--exit-code -- docs/generated/helm-values-reference.md`, scoped to that one file rather than
piggy-backing on the cached Go-codegen infrastructure.

**Git hook considered and declined**: the generator itself is expected to be sub-second (pure
`encoding/json` over a single ~3,300-line file, no network calls), so a pre-commit/pre-push hook
wouldn't have a real performance problem. Declined anyway — a git hook is opt-in and bypassable
(`--no-verify`, or simply never installed), so it can never substitute for the CI gate above; add
it only as a convenience if the two CI gates prove insufficient in practice.

**`postgresql.enabled`/`valkey.enabled`**: included in the standard trim (both already default
`true`; the fully functional `--set postgresql.enabled=false --set postgresql.host=...` BYO
override, and the Valkey equivalent, remain unaffected) — covered by the auto-generated reference
(below), not shown in the example file; README keeps a short narrative callout for the BYO
override. This is a visibility-only change; no schema field is removed or renamed.

**`kubernautAgent.llmProfileRef` default (mandatory count 8→7)**: defaults to `"primary"` instead
of an unconditional `fail()`, grounded in `"primary"` already being the universal, 100%-consistent
convention across this repo's README, `quickstart.sh`, `helm-smoke-test.sh`, the CI GitOps step,
and essentially every `helm-unittest` fixture. `apifrontend.yaml`/`NOTES.txt`'s inherited
references need the same `| default "primary"` fallback-chain fix for consistency. A still-
undefined `global.llmProfiles.primary` doesn't go unguarded — `kubernaut.llm.resolveProfile`'s
existing `fail()` fires instead, with a more actionable message. The guarantee moves from a
template `fail()` to a schema-level requirement on whichever profile ends up referenced; it isn't
removed.

**No separate `values-fleet.yaml` overlay (reversed from an earlier draft of this decision, per
direct user instruction)**: a second overlay file only works cleanly for a *local checkout*
install (`-f charts/kubernaut/values-fleet.yaml`, a real path on disk). For a pure OCI install
(`helm install kubernaut oci://.../kubernaut --version X.Y.Z -f my-values.yaml`, the pattern
`DEVELOPER_GUIDE.md` documents as the production default) that file exists only inside the
packaged chart tarball and isn't reachable by `-f` without first `helm pull ... --untar`-ing the
chart to disk. A version-pinned `raw.githubusercontent.com` URL was considered (Helm's `-f`
officially accepts a URL, not just a path) and rejected for two independent reasons: (1)
Fleet-Federation edge clusters are exactly the population most likely to run in a
restricted-network or airgapped environment, where a live fetch from `github.com` at install time
is not a safe assumption; (2) a URL pinned to a specific git ref is one more artifact that can
silently drift out of sync with schema changes if a release forgets to update it — a maintenance
liability independent of the network concern. Instead, the ~19 Fleet-specific fields
(`global.fleet.*` + per-service `fleet.oauth2.credentialsSecretRef`/`namespace` overrides) are
covered automatically by the auto-generated reference (below) rather than a hand-copied README
block — README keeps only a short worked example (concrete values enabling Fleet) plus its
existing narrative explanation of the shared-vs-per-service semantics. A Fleet user adds the
fields directly to the single `values.yaml` they already maintain and pass via `-f`, identical
mechanism regardless of OCI vs. local-checkout, airgapped or not.

**Drift protection for the README's Fleet block**: `global.fleet` and every per-service `fleet`
block have `"additionalProperties": false` in the schema (verified) — if a documented field name
is ever renamed or removed, `helm template`/`helm lint` hard-fails with a schema violation rather
than silently ignoring the stale key, *provided* a helm-unittest fixture actually exercises the
README's exact field set (this DD's render-validity gate below requires exactly that). This closes
the "documentation goes stale and silently wrong" failure mode. It does **not** close the opposite
one — a new Fleet field added to the schema without anyone remembering to add it to the README
block — which is a human-process risk shared by any documentation-based approach, including a real
committed file (nothing auto-detects "a related field was added, should this mention it too").

**`values-airgap.yaml`'s existing usage docs have the same root-cause bug, fixed as part of this
DD**: its own header comment shows `-f charts/kubernaut/values-demo.yaml -f
charts/kubernaut/values-airgap.yaml` — `values-demo.yaml` does not exist anywhere in the chart, and
even if it did, `values.yaml` is loaded automatically as the chart's default layer and never needs
an explicit `-f`. Unlike Fleet, disconnected/airgapped installs are *not* well served by a raw-URL
fetch either (the whole premise of airgap is no live internet access), so the correct fix here is
different: document the actual working OCI-compatible sequence, `helm pull
oci://quay.io/kubernaut-ai/charts/kubernaut --version <X.Y.Z> --untar` followed by `helm install
kubernaut ./kubernaut -f ./kubernaut/values-airgap.yaml --set global.image.registry=...` — extract
first, then install from the now-local directory, consistent with an airgapped user needing a local
mirror step anyway.

**The 7 mandatory fields**: `aianalysis.policies.{content,existingConfigMap}` and
`signalprocessing.policies.{content,existingConfigMap}` (either/or pairs, unconditional `fail()`),
and whatever profile `kubernautAgent.llmProfileRef` resolves to (by default `"primary"`, or
explicitly) must itself supply `global.llmProfiles.<name>.{provider,model,
credentialsSecretName}` (schema `required` on `global.llmProfiles`'s `additionalProperties`,
enforced unconditionally by `helm install`/`helm template`/`helm upgrade` — JSON-schema validation
is not opt-in, unlike `helm lint --strict`, which only promotes lint *warnings* to errors) —
`global.llmProfiles` ships as `{}` with no default profile, so there is nothing for
`credentialsSecretName` in particular to fall back to.

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

**Correction found during implementation — `resource-policy: keep` is inert here, the real
mechanism is omission**: unlike `postgresql-data`/`valkey-data` (PVCs Helm itself creates and
tracks in its release manifest, where `resource-policy: keep` genuinely instructs Helm's own
uninstall logic to skip deletion), the audit-HMAC Secret is created out-of-band by `kubectl create
secret` inside a hook Job's shell script — Helm never tracks it as part of the release at all, so
`helm uninstall` was never going to touch it regardless of any annotation. The annotation is kept
anyway (cheap, matches repo convention, self-documents intent to a human running `kubectl get
secret -o yaml`), but the actual survival guarantee comes from deliberately never adding
`datastorage-audit-hmac-key` to `tls-cert-job.yaml`'s Job 4 (`tls-cleanup`, `post-delete`)
`kubectl delete secret ...` list — the same Job that already, correctly, deletes the *TLS* secrets
it created (which must not survive, since they rotate). A code comment at the omission point flags
this as deliberate, not an oversight, for future editors.

**Correction found during implementation — a second `lookup`+`fail()` timing bug, same shape as
Decision Area 3's**: `templates/infrastructure/secrets.yaml` already had an *opt-in* validation
block for `datastorage.config.auditHashKey` — `lookup` the Secret, `fail()` if absent — gated by a
local "does this look like a live cluster" canary (`lookup "v1" "Namespace" "" "kube-system"`,
functionally identical to Decision Area 3's `kubernaut.hasClusterAccess`). Helm renders **all**
templates (including this `lookup`) in a single pass *before* any `pre-install`/`pre-upgrade` hook
executes (confirmed against Helm's documented hook lifecycle) — so on every fresh `helm install`
against a real cluster, this check would see the not-yet-hook-created Secret as absent and `fail()`
the entire install, unconditionally defeating Section 4's auto-generation. This is a pre-existing
latent bug in the opt-in validation, now surfaced by Section 4 giving `tls.mode=hook` users a
working alternative to pre-creation. **Fix**: scope the existing check to `(existingSecret set) OR
(tls.mode != "hook")` — i.e. skip it exactly when Section 4 will handle auto-generation itself
(default name, hook mode), keep it in force for BYO (`existingSecret` set, same contract as
postgresql/valkey) and non-hook `tls.mode`s, where no auto-generation mechanism exists and
pre-creation is genuinely required. Verified empirically: this repo's own dev kubeconfig has no
reachable cluster, so this class of `lookup`-gated behavior (postgresql/valkey secret checks
included) is structurally untestable via `helm template`/`helm-unittest` in this environment or in
GitOps rendering — proof is necessarily deferred to the Kind-based CI smoke test (Validation
Strategy item 4).

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

### Decision Area 12 — Post-#1755 Regression Check & Field Census

PR #1755 raised the schema's total leaf-field count from ~375 to **404** (net +29 — new
Gateway/APIFrontend `ingress`/`nodePort` fields, NetworkPolicy `ingressCIDRs`/
`ingressNamespaceSelectors` + new `idp`/`llm`/`mcpGateway` blocks, and three new APIFrontend
config blocks: `mcp`, `interactive`, `rateLimit`). Verified directly against merged `main` that
none of these new fields conflict with or require redesigning Decision Areas 1-11:

- **DA1/DA3 structurally unaffected**: `pdb.yaml`'s shared per-service loop and all 14
  NetworkPolicy templates' `{{- if and .Values.networkPolicies.enabled
  .Values.networkPolicies.<service>.enabled ... }}` guards are unchanged in shape — #1755's new
  `ingressCIDRs`/`ingressNamespaceSelectors` fields are used *inside* the guarded block, not part
  of the enabling condition.
- **DA7/DA8 implementation detail**: `tls-cert-job.yaml`'s Section 2 per-service cert loop already
  includes `apifrontend` (added by #1755, independently of this DD) — Decision Area 8's "add
  `valkey-service` to the loop" step targets the *current* membership (`gateway-service
  data-storage-service kubernaut-agent fleetmetadatacache-service apifrontend`). No
  section-renumbering conflict — the file's Section 1/2/3 structure is unchanged, so Decision
  Area 7's planned "Section 4" is still a clean append.
- **DA9 guards confirmed present**, at shifted line numbers only (`console.yaml`'s three `fail()`
  guards now sit around lines 86-92) — the design is unaffected; exact lines should be re-grepped
  at implementation time rather than trusted from any prior write-up, including this one.
- **Three new #1755 toggles categorized, no new Decision Area needed**:
  - `apifrontend.config.mcp.enabled` (defaults `true`) — a genuine feature toggle for AF's own
    external-facing `/mcp` endpoint. Belongs in Decision Area 4's "feature-enable toggle" bucket
    (shown in the trimmed `values.yaml`), not a security control.
  - `apifrontend.config.interactive.enabled` (defaults `true`, own doc comment states "not a
    behavior-changing fix") — a Decision Area 4 "Bucket 98" trim candidate, hidden with its
    working default.
  - `apifrontend.config.rateLimit` (no `enabled` field at all — AF's own concurrent-session/
    request-rate limits, always on, four tuning fields each with a safe default) — Bucket 98 trim
    candidate, distinct from `datastorage.config.server.rateLimit.enabled` (Decision Area 6's
    target).

**Outcome**: no Decision Area's chosen alternative changes. The mandatory-field count (7) and this
DD's confidence levels are unaffected — this is a scope-expansion of Decision Area 4's trim
candidate set (+29 fields to categorize, following the same test-then-trim methodology already
established) plus a handful of implementation-detail corrections (current loop membership, current
line numbers), not a design change.

### Decision Area 13 — APIFrontend Replay-Cache TLS Client (prerequisite for Decision Area 8)

**Finding, discovered during PR5/Decision Area 8 pre-implementation analysis**: Decision Area 8
makes the in-chart Valkey Deployment TLS-only (disables the plaintext port entirely). Two Go
clients connect to it:

| Client | Go-side TLS support (before this Decision Area) |
|---|---|
| DataStorage (`pkg/datastorage/server/server_construction.go`) | Full — `appCfg.Redis.TLS.Enabled` → `RedisTLSConfig.BuildTLSConfig()` → `redisOpts.TLSConfig`, already wired |
| APIFrontend replay cache (`cmd/apifrontend/auth_wiring.go`'s `newValkeyReplayCache`, `pkg/apifrontend/config.ReplayCacheConfig`) | **None** — `redis.Options{Addr, Password, DB}` had no `TLSConfig` field anywhere, and `ReplayCacheConfig` had no CA/cert fields to even plumb one through |

Left unaddressed, Decision Area 8 landing would have permanently broken
`apifrontend.config.auth.replayCache.enabled=true` (TLS handshake against a plaintext client) —
not a hypothetical edge case, since Decision Area 6/PR6 makes that same toggle mandatory-on by
default, and `scripts/helm-smoke-test.sh` already exercises it today (template-only assertions
currently, so CI would not have caught the runtime break). This is a genuine cross-Decision-Area
gap: Decision Area 6's table justified mandating `replayCache.enabled` with "None — Valkey already
load-bearing," drafted before Decision Area 8's TLS-only change existed.

**Options considered** (escalated to the user given the architectural/scope implications):
1. Add minimal Go TLS support to the replay-cache client — **selected**.
2. Dual-listener Valkey (keep plaintext 6379 alongside TLS 6380) — rejected: only partially
   encrypts Valkey traffic (DataStorage's, not the replay cache's jti state), undermining Decision
   Area 8's stated intent for a security-hardening pass explicitly about closing exactly this kind
   of gap.
3. Defer Decision Area 8 (and Decision Area 6's `redis.tls` mandate) to a separate follow-up issue
   — rejected: delays needed hardening without technical justification once option 1 was shown to
   be small and low-risk.

**Decision**: implement option 1, explicitly requiring the result be "on par with the rest of the
services" (the user's words) rather than a stripped-down, replay-cache-specific implementation —
i.e. reuse the fleet's existing shared TLS hardening primitive, not a smaller ad-hoc one:

1. **`pkg/shared/tls`**: extract `BuildTLSConfig(caFile string, opts ...TLSTransportOption)
   (*tls.Config, error)` — the CA-verified, security-profile-hardened (`ApplyProfile`,
   `getDefaultSecurityProfile()`), optional-mTLS (`WithClientCert`) `*tls.Config` construction
   logic already inside `NewTLSTransport`, which now becomes a thin wrapper:
   `NewTLSTransport` = `BuildTLSConfig` + `&http.Transport{TLSClientConfig: ...}`. This is the same
   hardening every other outbound TLS client in the fleet gets (BR-ENC-001, SC-8), just exposed in
   a form usable by non-HTTP clients (go-redis's `redis.Options.TLSConfig` wants a raw
   `*tls.Config`, not an `*http.Transport`). Zero behavior change for existing `NewTLSTransport`
   callers (proven by UT-TLS-DA9-006, asserting output parity between the two functions).
2. **`pkg/apifrontend/config`**: add `ReplayCacheConfig.TLS *ReplayCacheTLSConfig`, a new type
   mirroring `pkg/datastorage/config.RedisTLSConfig`'s field shape (`Enabled`, `CAFile`,
   `CertFile`, `KeyFile`) for cross-service consistency — deliberately a separate type (not a
   cross-package import), consistent with each service owning its own config package, with only
   the `pkg/shared/tls` builder function shared. `CertFile`/`KeyFile` are unused against the
   chart's own Valkey (`--tls-auth-clients no`, one-way TLS per Decision Area 8), kept for BYO
   Valkey/Redis that does require mTLS — same optionality precedent as DataStorage's identical
   fields.
3. **`cmd/apifrontend/auth_wiring.go`**: `newValkeyReplayCache` calls `sharedtls.BuildTLSConfig`
   when `cfg.TLS != nil && cfg.TLS.Enabled` and sets `redis.Options.TLSConfig`. A failed TLS
   handshake (untrusted CA, unreachable host) falls into the same existing fail-open path as any
   other connection failure — `buildReplayCache` degrades to the in-memory cache rather than
   disabling replay protection outright (pre-existing behavior, proven unchanged by
   `TestBuildReplayCache_TLSEnabledWrongCA_FallsBackToInMemory`).
4. **Helm**: `apifrontend.config.auth.replayCache.tls.{enabled,caFile,certFile,keyFile}`, schema
   and `values.yaml` defaults mirroring `datastorage.config.redis.tls`'s existing shape exactly
   (minus `insecureSkipVerify`, a pre-existing dead field on DataStorage's side — present in its
   JSON schema but never read by `pkg/datastorage/config.RedisTLSConfig` — deliberately not
   propagated into a second config surface). `caFile` defaults to the already-distributed
   inter-service CA (`kubernaut.interServiceTLS.caFile`, `/etc/tls-ca/ca.crt` — APIFrontend's pod
   already mounts this via `kubernaut.tlsCaVolumeMount`/`kubernaut.tlsCaVolume` for unrelated
   REST-API-client purposes, confirmed by grep, no new mount needed) so `tls.enabled=true` works
   against the chart's own Valkey without any extra configuration; explicit override remains
   available for a BYO Valkey/Redis signed by a different CA.

**Not addressed here (separate, pre-existing, unrelated gap noted for awareness only)**:
`datastorage.config.redis.tls.caFile` has the identical "defaults to empty string, chart doesn't
mount `/etc/tls-ca` in `datastorage.yaml`" problem this Decision Area fixes for APIFrontend —
tracked as an explicit line item in Decision Area 8's own PR5 implementation (add the mount, fix
the default), not duplicated here.

**Confidence**: 95% — the Go-side change reuses an existing, already-tested primitive
(`NewTLSTransport`'s internals) behind a new exported name with a proven zero-behavior-change
refactor; the config/wiring shape directly mirrors DataStorage's already-shipped, already-verified
`RedisTLSConfig` pattern; the Helm CA mount is confirmed already present, not newly added. Full UT
coverage includes a real TLS handshake against a `miniredis.RunTLS` server (not just YAML parsing)
and its rejection path (wrong CA → fail-open fallback, confirmed via the same log line a real
production Valkey TLS misconfiguration would emit).

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
   toggles, from 404 total leaf fields today (see Decision Area 12).
2. Decision Areas 1-2 close real value-level duplication on top of an already-shared schema
   shape.
3. Decision Area 3's `enabled`-toggle removal closes a latent AC-4/SC-8 compliance gap as a
   byproduct of the same onboarding-friction-reduction pass, not a separate effort.
4. Fleet Federation's ~19 fields get a documented on-ramp (worked example in the README, full
   field list in the auto-generated reference) that works identically for OCI and local-checkout
   installs, airgapped or not — no second file to fetch. `values-airgap.yaml`'s own OCI usage
   docs get fixed as a byproduct (same root cause). The auto-generated
   `docs/generated/helm-values-reference.md` keeps README's size bounded regardless of how many
   fields the schema grows to, and can never drift from it (Decision Area 4's generation strategy).
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
   for the trimmed default `values.yaml` and for a helm-unittest fixture with the README's
   documented Fleet field set applied inline (no shipped `values-fleet.yaml` file — see Decision
   Area 4).
2. **helm-unittest additions**: per-service merge-behavior cases for Decision Areas 1-2; explicit
   "key entirely absent" cases for every shared helper touched by Decision Area 4's trim.
3. **Decision Area 6-specific**: `helm-unittest` asserting each of the four security-relevant
   config blocks renders unconditionally, with the `enabled` guard fully absent from the template.
4. **Decision Area 7-specific** (mandatory): a Kind-based `helm upgrade` test asserting the
   audit-HMAC Secret's value is byte-identical before and after an upgrade with no other changes.
5. **Decision Area 8-specific**: CI-integrated (not just locally-run) Kind-based `helm upgrade`
   test exercising the plaintext→TLS transition through the real `helm upgrade` command.
6. **Decision Area 4's generated reference**: dedicated, uncached `git diff --exit-code --
   docs/generated/helm-values-reference.md` steps added to both `ci-pipeline.yml` (every PR) and
   `chart-release.yml` (release-time backstop) — not the existing `gen-diff`/Go-codegen cache path,
   which excludes `values.schema.json` from its cache key and wouldn't reliably catch this.
7. See the implementation plan (`.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`)
   for the full RED/GREEN test breakdown and pre-assigned Test Scenario IDs per PR.

---

## References

- `charts/kubernaut/values.schema.json`, `charts/kubernaut/values.yaml`,
  `charts/kubernaut/values-airgap.yaml`, `charts/kubernaut/templates/**`
- `hack/gen-helm-config-docs/` (new), `docs/generated/helm-values-reference.md` (new, generated) —
  modeled on the existing `hack/crd-ref-docs/`/`docs/generated/crds.md` pattern
- DD-PLATFORM-004: Anti-Affinity and PDB Enabled by Default
- DD-PLATFORM-005: helm-unittest as a Dedicated Fast-Fail CI Gate
- BR-PLATFORM-007: Helm Chart Minimal-Configuration Onboarding
- BR-PLATFORM-009: Helm Chart Gateway/APIFrontend Ingress Parity with the Kubernaut Operator
- Issues #1725 (merged), #1730 (open), #1729 (open), #1737/#1755 (merged)
- Implementation plan: `.cursor/plans/dd-platform-006_full_implementation_10d3769d.plan.md`

---

**Document Version**: 5.7 (Decision Area 13: closed a PR5/Decision Area 8 blocker discovered during
implementation — APIFrontend's replay-cache Valkey client had zero TLS support in Go, which would
have permanently broken it once Valkey went TLS-only; fixed with a small Go change reusing the
fleet's existing shared TLS hardening primitive rather than a dual-listener compromise or deferral)
**Last Updated**: 2026-07-29
**Status**: 🟡 Proposed — awaiting user approval. Field-census recount and file/line-reference
verification against `main` post-PR #1755 completed (Decision Area 12) — no Decision Area's
chosen alternative changed. Decision Area 4's Fleet overlay approach reversed from a shipped
`values-fleet.yaml` file to fields folded into the user's own `values.yaml`, after identifying it
would have been unusable for pure-OCI/airgapped Fleet installs. The ~394 fields removed from
`values.yaml` are documented via a new auto-generated `docs/generated/helm-values-reference.md`
rather than hand-authored README tables, after finding README.md (895 lines) couldn't scale to an
exhaustive field-by-field transcription without becoming unusable. Its drift-freshness check is
enforced by dedicated new steps in both `ci-pipeline.yml` and `chart-release.yml` (verified the
`generate-crd-docs` precedent this was modeled on is actually unenforced anywhere in CI today, and
that the existing Go-codegen cache path excludes `values.schema.json` from its cache key — neither
could be relied on as-is). No local git hook — CI enforcement is sufficient, hooks are bypassable.
