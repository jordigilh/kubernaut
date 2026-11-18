# Issue #2650 — Implementation Plan (v4, ephemeral, uncommitted)

Include the API group in the fingerprint hash. Maintainer decision: NO migration,
NO backward compatibility — clean installs only. No dual-read, versioned
fingerprints, or transition windows.

## Business requirement & control objectives

- **BR-GATEWAY-004** (primary): cross-adapter deduplication consistency — the same
  ns/kind/name in two API groups (e.g. `route.openshift.io` Route vs
  `serving.knative.dev` Route) MUST produce distinct fingerprints.
- **SI-10** (input validation): new required `api_version` input on the
  `kubernaut_check_existing_remediation` tool MUST be validated.
- **AC-4** (scoped): GW/AF fingerprint parity for top-level-owner resources with
  identical apiVersion (DD-GATEWAY-019 Alternative 2 keeps both on the same hash).
- **AU-2/AU-12** note: the pre-existing prometheus ambiguous-group empty-apiVersion
  case is audit-logged and is a **known residual #2650 gap**, intentionally out of
  scope (DD-KA-006 "absence is not an error").

## Change 1 — group-aware hash (pkg/gateway/types/fingerprint.go)

`CalculateClusterAwareFingerprint` hash input becomes
`clusterID:group:namespace:kind:name` (and `group:namespace:kind:name` when
clusterID is empty). Group extraction uses the project's canonical parser
`scope.ParseAPIVersion` (pkg/shared/scope/gvk.go:47) — NOT bare
`schema.ParseGroupVersion`, which silently accepts slash-less group names
(`ParseGroupVersion("apps")` → `{Group:"", Version:"apps"}`, no error), collapsing
a group name into the empty-group segment. Import direction verified feasible:
no file under `pkg/shared/scope` imports `pkg/gateway` (no cycle); `af_create_rr.go`
already imports both packages side by side.

- empty apiVersion → short-circuit → empty group segment (legitimate production
  state on the prometheus ambiguous/zero-candidate registry path)
- `scope.ParseAPIVersion` error (malformed, version-less, slash-less group name)
  → use the raw apiVersion string as the segment (defense-in-depth; verified
  unreachable in production: AF validates via `validate.APIVersion` +
  `scope.ApplyAPIVersion`, k8s event adapter validates at parse per #2592,
  prometheus derives apiVersion from registry GVRs via `formatAPIVersion` —
  always well-formed or empty)

Update doc comments at `fingerprint.go:136,158-159` and
`pkg/gateway/types/types.go:37-38`.

## Change 2 — group seeding on the owner-resolution path (MAJOR-1 amendment)

`ResolveFingerprintWithCluster` (`fingerprint.go:81-103`) currently calls
`ResolveTopLevelOwnerWithAPIVersion(ctx, ns, kind, name)` — the input's
`resource.APIVersion` is discarded. Production wiring is discovery-backed
(`cmd/gateway/main.go:361-366` constructs `NewK8sOwnerResolver` with
`WithRegistry(apiRegistry)`, hard-fail startup at `main.go:331-344`), so the gap
is: nil-registry test paths, registry misses for kinds unknown to discovery
(new CRD before the 5-min refresh, transient discovery miss), where `resolveGVK`
falls back to the static `kindToGroup` map (`owner_resolver.go:570-582`) that
misses grouped kinds.

Fix: when `resolution.APIVersion == ""`, seed the owner `ResourceIdentifier`'s
`APIVersion` with the input `resource.APIVersion` (the owner's own apiVersion from
OwnerReferences still wins when present — cross-group owners carry
`controllerRef.APIVersion` per `owner_resolver.go:381,414`, so the seed only fires
for standalone resources where the resolution IS the resource). Log when the seed
fires after an owner was actually found (degenerate controllerRef-without-
apiVersion case — real Kubernetes populates it; a log line makes the attribution
observable rather than silent). Add an invariant comment/test that the
plain-resolver fallback (`fingerprint.go:105-124`, no APIVersion) is unreachable
in production today because `K8sOwnerResolver` implements
`OwnerResolverWithAPIVersion`. IT-GW-2650-001's standalone case must deliberately
construct the registry-miss gap (mock resolver returning empty APIVersion, or a
kind unknown to the test registry).

## Change 3 — AF create path (pkg/apifrontend/tools/af_create_rr.go)

`rrFingerprintWithCluster(clusterID, namespace, kind, name)` gains an
`apiVersion string` parameter; the call at `:270` passes `args.APIVersion`
(already validated at `:312` and applied to the scope identity at `:237-239`).
This covers all KA/AF tools (`ka_remediate.go:123`, `ka_investigate_mcp.go:594`,
`af_investigate_alert.go:154` — all funnel into `HandleCreateRR` with validated
APIVersion).

## Change 4 — AF check path (pkg/apifrontend/tools/af_check_existing_rr.go)

`CheckExistingRRArgs` gains required `APIVersion string \`json:"api_version"\``
(validated with `validate.APIVersion`, non-empty), passed at `:67`. Tool schema is
auto-derived from the struct via `functiontool.New` — no manual schema edit.

Fallout (enumerated, non-exhaustive — the rule is "every construction site of
CheckExistingRRArgs and every doc table listing its args"):
- `pkg/apifrontend/agent/prompt.txt:173` — document `api_version` for
  `kubernaut_check_existing_remediation` (mirror `kubernaut_remediate`'s docs)
- `test/fixtures/kubernetes/apifrontend/e2e/mock-llm.yaml:479-487` — add
  `api_version` to the tool-call args
- `test/e2e/apifrontend/a2a_test.go:153` (TC-E2E-A2A-T18)
- 18 `HandleCheckExistingRR` test call sites (17 in
  `af_check_existing_rr_test.go` + `integration_wiring_test.go:49`)
- `docs/services/apifrontend/design/TOOL_EXECUTION_MODEL.md:146` — args table
  lists "namespace, kind, name"; sweep `docs/services/` for other
  `check_existing` arg tables (historical docs like `docs/tests/1409/` stay)

## Formal test plan (methodology mandate)

Before implementation, create `docs/tests/2650/TEST_PLAN.md` from the RED section
using `docs/testing/TEST_PLAN_TEMPLATE.md` (IEEE 829-2008 hybrid), matching the
sibling-issue convention (`docs/tests/2066/`, `docs/tests/2593/`).

## TDD phase mapping

### RED — write first, all must fail

Unit (pkg/gateway/types, Ginkgo/Gomega, alongside fingerprint_fleet_test.go):
- `UT-GW-2650-001`: same ns/kind/name, two groups
  (`route.openshift.io/v1` vs `serving.knative.dev/v1`, both `Route`) → distinct
  fingerprints (BR-GATEWAY-004)
- `UT-GW-2650-002`: same resource (same group) → deterministic identical hash
- `UT-GW-2650-003`: cluster+group combined — different cluster same group →
  distinct; different group same cluster → distinct
- `UT-GW-2650-004`: empty apiVersion → stable, distinct from any non-empty group
- `UT-GW-2650-005`: malformed non-empty apiVersion does not collapse to the
  empty-group hash (raw-string fallback). Vectors MUST include the slash-less
  group name `"apps"` (accepted silently by `schema.ParseGroupVersion`, rejected
  by `scope.ParseAPIVersion`) in addition to a multi-slash vector like `"a/b/c"`
- `UT-GW-2650-006`: `apps/v1` vs `apps/v2` (same group) → SAME fingerprint
  (group-only dimension per issue decision)
- `UT-GW-2650-007` (M1): resource apiVersion known, owner resolution returns
  empty APIVersion → fingerprint still carries the resource's group

Unit (pkg/apifrontend/tools):
- `UT-AF-2650-001`: `HandleCreateRR` with two targets differing only in group →
  two RRs with distinct `SignalFingerprint`
- `UT-AF-2650-002`: `HandleCheckExistingRR` matching ns/kind/name but different
  group → `Exists: false`
- `UT-AF-2650-003`: missing/invalid `api_version` on CheckExistingRRArgs →
  `ErrInvalidInput` (SI-10)

Integration:
- `IT-GW-2650-001`: two cross-group signals through `ProcessSignal` production
  dispatch (adapter → `ResolveFingerprintWithCluster` → `CRDCreator`) → two RRs.
  MUST use an `OwnerResolverWithAPIVersion`-capable resolver (real `ownerchain`
  per `test/integration/gateway/owner_chain_dedup_integration_test.go:203-258`
  pattern, or a mock implementing `ResolveTopLevelOwnerWithAPIVersion` returning
  distinct groups) — plain `mockOwnerResolver` bypasses the group dimension.
  Include a standalone-resource (no ownerRefs) cross-group case proving M1
  seeding, with the registry-miss gap deliberately constructed.
- `IT-AF-2650-001` (SI-10): through the production tool closure
  (`NewCheckExistingRemediationTool` → `functiontool.New`,
  `af_check_existing_rr.go:96-103`), missing `api_version` → `ErrInvalidInput`.
- GW/AF parity (AC-4,
  `test/integration/kubernautagent/fleet/fleet_wiring_test.go:542-551` pattern):
  identical cluster+GVK inputs → identical fingerprints from the GW path and
  `rrFingerprintWithCluster`.

### GREEN — minimal

1. Change 1 (hash + group extraction) — makes UT-GW-2650-001..006 pass
2. Change 2 (group seeding) — makes UT-GW-2650-007 pass
3. Change 3 (AF create) — makes UT-AF-2650-001 pass
4. Change 4 (AF check + fallout) — makes UT-AF-2650-002/003 pass
5. Fixture sweep per the rules below — makes existing suites pass

### REFACTOR

- Deduplicate the two `fmt.Sprintf` branches in `CalculateClusterAwareFingerprint`
  into a single input builder; extract a `groupSegment(apiVersion string) string`
  helper if it reads cleaner
- Improve doc comments: group dimension, raw-fallback rule, no-migration decision
  (#2650, clean installs only), plain-resolver-fallback unreachability invariant
- Post-refactor validation (mandatory): `go build ./...`,
  `golangci-lint run --timeout=5m`, `make test`, grep for stale
  `namespace:kind:name` / `clusterID:namespace:kind:name` doc references

## Fixture rules (two mechanisms, both required)

A fixture breaks **iff its production path supplies a group**:

1. **Gateway fixtures** — via resolver/registry: tests using the real
   `ownerchain.K8sOwnerResolver` with apiVersion-bearing OwnerReferences (e.g.
   `pkg/gateway/owner_resolver_k8s_test.go:639` needs `APIVersion: "apps/v1"`;
   chain fixtures already carry it at `:603,:612`), and prometheus tests whose
   registry resolves a single group. Tests using plain `mockOwnerResolver`
   (kind/name only → plain-resolver path → group `""`) or nil registry
   (`Unknown/unknown/""`) match unchanged.
2. **AF fixtures** — via args: seeded RRs constructed without `APIVersion`
   (`af_create_rr_test.go:463,684,797,948,1014,1062`,
   `af_check_existing_rr_test.go:19,141,157,193` — non-exhaustive; the rule is
   "seeded RR fingerprints and lookup args must agree on the group") while
   lookups hash with the args' group. Both the args AND the seeded
   `ResourceIdentifier`s need `APIVersion`.

Also: `test/e2e/fullpipeline/af_helpers_test.go:534` `rrFingerprint` helper gains
an apiVersion parameter (comment at `:530` updated).

## Wiring Manifest

| Component | Production entry point | Wiring code location | IT test ID |
|-----------|----------------------|---------------------|-----------|
| group-aware `CalculateClusterAwareFingerprint` | GW signal ingestion (both adapters) | `kubernetes_event_adapter.go:227`, `prometheus_adapter.go:392,418` | `IT-GW-2650-001` |
| `ResolveFingerprintWithCluster` group seeding (M1) | same | `fingerprint.go:93-103` | `IT-GW-2650-001` (standalone case) |
| `rrFingerprintWithCluster` + apiVersion | `HandleCreateRR` (all KA/AF tools) | `af_create_rr.go:270` | `UT-AF-2650-001` + `integration_wiring_test.go` |
| `CheckExistingRRArgs.APIVersion` | `kubernaut_check_existing_remediation` | `af_check_existing_rr.go:67,100` | `UT-AF-2650-002/003` + `IT-AF-2650-001` |

## Success criteria

1. All RED tests pass; cross-group collision proven at UT and IT level
2. `go build ./...`, lint, `make test` green; zero regressions in existing suites
3. CHECKPOINT W passes: both AF call sites carry the group; no fingerprint call
   site left without it
4. SI-10: new tool input validated; BR-GATEWAY-004: GW and AF produce identical
   fingerprints for identical cluster+GVK (parity test)
5. UT scenario coverage of the fingerprint group contract ≥ 90% (expected 100% —
   UT-GW-2650-001..007 cover the full group-segment decision table)

## Risk mitigation

| Risk | Mitigation |
|------|-----------|
| Fixture churn | Governed by the two deterministic rules above; RED-first makes each miss a mechanical fix pointed at by a failing assertion |
| Legacy persisted RRs never match | Accepted: clean installs only, no migration (maintainer decision) |
| Empty-group signals colliding with a real group | Impossible: real groups are non-empty; empty apiVersion is the prometheus ambiguity case, documented as residual gap |
| Lease/RR names drift | None: output remains 64-char hex; CRD schema `^[a-f0-9]{64}$` satisfied |
| GW hashes owner's group, AF hashes target's group | Pre-existing seam for owned resources; AC-4 claim scoped to top-level-owner parity, proven by the parity test |
| PR #2647 rebases | Trivial: fingerprint core + two AF call sites + seeding fallback |

## Out of scope (explicit)

- Prometheus ambiguous-group empty-apiVersion dedup collapse (pre-existing #2066,
  AU-2/AU-12 audit-logged, DD-KA-006 contract)
- Any migration/back-compat scheme
