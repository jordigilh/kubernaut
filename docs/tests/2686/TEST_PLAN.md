# Test Plan — #2686: Invalid managed-label revocation on the fenced lane

**Issue**: https://github.com/jordigilh/kubernaut/issues/2686
**Branch**: `work/2686-invalid-label-revocation` (base `main`)
**BR**: BR-SCOPE-001 (Resource Scope Management); DD-FLEET-016 amendment
**Date**: 2026-11-20

## Problem

On the fenced fleet lane, a target whose `kubernaut.ai/managed` label becomes **invalid**
(e.g. `"banana"`) keeps its previously committed authorization until a later repair replaces
the record or its TTL expires (45s default). The read side already maps a committed
`invalid` record to `ScopeInvalid`/`invalid_resource_label`
(`pkg/fleet/scopecache/client.go` `mapFencedRecord`, `checkResourceObservation`), but the
write side refuses to carry the observation, so the invalid state never reaches the cache:

- `pkg/fleet/fmc/syncer.go` `buildObservedResource` rejects `scope.LabelStateInvalid` (whole
  sync cycle fails, last-good record stays readable).
- `pkg/fleet/scopecache/lifecycle.go` `Observation.Validate` accepts only true/false/absent.
- `pkg/fleet/scopecache/repair_contract.go` `RepairObservation.Validate` same.
- `pkg/fleet/scopecache/status_store.go` `decodeReadVerified` rejects a stored `invalid`.
- `pkg/fleet/scopecache/scripts/fmc_v2_atomic.lua` `read_verified` rejects `label_state
  ~= true/false/absent` with `INVALID_LABEL_STATE`.

The local lane (`pkg/shared/scope/manager.go`) already treats an invalid label as a
decisive fail-closed outcome (`ScopeInvalid`/`invalid_resource_label`), and the direct ACM
lane already produces `LabelStateInvalid`. The fenced lane is the outlier.

## Design decision (CHECKPOINT DD)

**Option A (chosen): an invalid observation is a complete, decisive source observation.**
The FMC writer stages and commits it like any other label state; the committed record
carries `label_state=invalid`; readers — which already map it — fail closed immediately
with `ScopeInvalid`/`invalid_resource_label`. Local and fenced lanes end up with the same
rule.

- Option B (distinguish "rejected as invalid" from "repair could not be staged"): rejected —
  keeps two failure channels for one domain outcome and does not close the window for the
  invalid case, which is a *complete* observation, not a staging failure.
- Option C (accept the window, tighten TTL): rejected — accepts a bounded fail-open
  authorization window on an AC-6/SI-10 control surface; TTL tightening also penalizes
  every record for a per-item condition.

Rationale: the read-side contract already exists end-to-end (`valkey_reader` decodes
`invalid`; `client.go` maps it); Option A is the minimal change that makes the write side
stop discarding a decisive observation. Readiness/partition failures are untouched and keep
returning `partition_unavailable`/`partition_invalid` — they are never flattened into
`invalid`.

## Scope

Production changes (5 enforcement points):

| File | Change |
|---|---|
| `pkg/fleet/fmc/syncer.go` | `buildObservedResource` no longer rejects `LabelStateInvalid` |
| `pkg/fleet/scopecache/lifecycle.go` | `Observation.Validate` accepts `LabelStateInvalid` |
| `pkg/fleet/scopecache/repair_contract.go` | `RepairObservation.Validate` accepts `LabelStateInvalid` |
| `pkg/fleet/scopecache/status_store.go` | `decodeReadVerified` accepts a stored `invalid` |
| `pkg/fleet/scopecache/scripts/fmc_v2_atomic.lua` | `read_verified` label whitelist gains `invalid` |

No new components; the change relaxes validation on the existing wired chain.

## Control objectives

| Control | Application |
|---|---|
| FedRAMP AC-6 (least privilege) | An invalid opt-in label revokes a previously committed authorization at the next committed repair instead of retaining it until TTL |
| FedRAMP SI-10 (input validation) | An invalid label value is a completed fail-closed domain outcome (`ScopeInvalid`), not a retained last-good authorization and not a silent skip |
| SOC2 CC7.2 | Fail-closed revocation is observable through the existing scope-result contract |
| OWASP ASVS V5.1 (access control fail-safe defaults) / V4.1 (input validation) | Authorization fails closed on malformed opt-in input; unavailable evidence stays `unavailable`, never flattened to `invalid` or to `unmanaged` |

## Scenario matrix (requirements-based)

### Unit — `pkg/fleet/scopecache`

| ID | Scenario | Expected | Critical |
|----|----------|----------|----------|
| UT-FMC-2686-001 | `Observation.Validate` and `RepairObservation.Validate` accept `LabelStateInvalid` | no error (an invalid observation is writable) | yes |
| UT-FMC-2686-002 | staged repair commits an `invalid` observation over a previously committed `true` record; `ReadVerified` serves it | `ReadVerified` with `Record.LabelState == invalid` (Lua write + read whitelist + Go decode) | yes |
| UT-FMC-2686-003 | `Client.Check` (production constructor `NewClientWithStatusStore`) against the committed invalid record | first commit `true` → `ScopeVerifiedManaged`; after the invalid commit → `ScopeInvalid`/`invalid_resource_label` (the revocation) | yes |
| UT-FMC-2686-004 | committed invalid record exists but the partition is not in the readiness view | `ScopeUnavailable`/`partition_unavailable` — readiness failures are never flattened into `invalid` | yes |

### Unit — `pkg/fleet/fmc`

| ID | Scenario | Expected | Critical |
|----|----------|----------|----------|
| UT-FMC-2686-005 | legacy lane: source item with invalid label state | sync succeeds; `SetObservation` receives `LabelStateInvalid` (legacy readers fail closed) | yes |
| UT-FMC-2686-006 | repair lane: full lifecycle `SyncCluster` with an invalid-label item | sync succeeds (no error); the repair request carries the observation with `LabelState == invalid` | yes |

### Integration (wiring)

| ID | Proves |
|----|--------|
| IT-FMC-2686-001 | Full production chain (MCP observation → FMC syncer → Valkey Lua store → `Client.Check` → HTTP handler): a committed `true` authorization is revoked to `ScopeInvalid`/`invalid_resource_label` at the next sync after the label becomes invalid |

### Wiring manifest

| Component | Production entry point | Wiring code location | IT |
|---|---|---|---|
| Syncer invalid-observation staging | `Syncer.SyncCluster` → `repairPartition` → `collectRepairObservations` | `pkg/fleet/fmc/syncer.go` `buildObservedResource` | IT-FMC-2686-001 |
| Status store invalid record round-trip | `Client.Check` fenced lane → `ReadVerified` | `pkg/fleet/scopecache/status_store.go` + `scripts/fmc_v2_atomic.lua` | IT-FMC-2686-001 |

## Documentation changes

| File | Change |
|---|---|
| `docs/architecture/decisions/DD-FLEET-016-staged-fmc-repair.md` | New "Invalid-label observations (#2686)" section: an invalid observation is staged and committed; it revokes at commit, not at TTL |
| `docs/architecture/decisions/ADR-053-resource-scope-management.md` | Amendment note: fenced-lane invalid observations revoke (both lanes aligned) |
| `docs/UPGRADE.md` | Fleet scope lanes section: behavior change entry |

Note: ADR-053 §2a "Recognized kinds" / "Invalid-label observations on the fenced lane" and
the UPGRA.md cluster-scoped caveat describing the retention window live on the unmerged
PR #2673 branch (`work/2670-cluster-scoped-restore`, commit `0bcbbf965`) and are not
modified here; this PR amends the same documents on `main`, so a small doc reconciliation
is expected when both land.

## Validation

```bash
go build ./...
golangci-lint run --timeout=5m
go test ./pkg/fleet/scopecache/... ./pkg/fleet/fmc/... -count=1
go test ./test/integration/fleetmetadatacache/... -count=1   # requires envtest/valkey per suite README
```

## Out of scope

- Local-lane informer staleness (the shorter local window noted in the issue) — informer
  refresh semantics, not a cache-contract change.
- Kind allowlists, TTL changes (Option C), or the #2670 branch's own documentation.
