# Test Plan: Simplify ParseRARID to Bare-Name-Only (Issue #1959)

**Service**: apifrontend
**Version**: 1.0
**Created**: 2026-08-06
**Author**: AI Assistant
**Status**: Approved
**Business Requirement**: BR-API-1493 (amends #1493)

---

## 1. Purpose

Remove the redundant `namespace/name` tolerance from `ParseRARID` and the
redundant namespace prefix from `BuildPhaseMetadata`'s `approval_request_name`,
so `rar_id` handling matches the bare-name-only precedent already established
for `rr_id` (`ParseRRID`, `2bfd24c31`).

## 2. Background

`#1493` (`c083673f5`) fixed a real bug (tool rejected bare RAR names) but also
added `namespace/name` tolerance "for backward compatibility." A sibling
commit (`cdbdf2452`, #1492) bundled in a change making `BuildPhaseMetadata`
emit `approval_request_name` as `namespace/name` "for defense-in-depth."

Per **ADR-057** (CRD Namespace Consolidation), every `RemediationApprovalRequest`
is always created in the single controller namespace — there is no legitimate
scenario where a RAR's namespace differs from `controllerNS`. The
`namespace/name` branch in `ParseRARID` therefore could never resolve
anything the bare-name branch (using the injected, trusted `controllerNS`)
couldn't already resolve, and it introduced a minor trust-boundary
inconsistency: it used the caller-supplied namespace segment instead of the
injected `controllerNS`.

The kubernaut-console `ChatContainer.tsx` passes `approval_request_name`
through to `rar_id` verbatim (no parsing), so this change requires **no
console-side update** — see jordigilh/kubernaut-console#57 for the console's
own, unrelated follow-up (error-type differentiation in the graceful
degradation card).

## 3. Objectives

- `ParseRARID` becomes structurally identical to `ParseRRID`: bare-name-only,
  namespace always sourced from the injected argument
- `BuildPhaseMetadata` emits `approval_request_name` as a bare name
- Preserve the legitimate bare-name-acceptance fix from #1493
- No regressions in existing approval request flows; no console changes

## 4. Scope

### In Scope

- `ParseRARID` (`helpers.go`): drop the `namespace/name` split branch
- `BuildPhaseMetadata` (`status_types.go`): revert `approval_request_name` to
  bare name (only this one line from `cdbdf2452`/#1492 — its unrelated
  Kind/Name `target` display format change is untouched)
- Adversarial tests proving a `rar_id` containing `/` is now rejected as an
  invalid K8s resource name rather than reinterpreted as `namespace/name`

### Out of Scope

- Console-side changes (none required)
- `cdbdf2452`'s Kind/Name display format work (unrelated, stays as-is)

## 5. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-API-1493 | `ParseRARID`: bare name resolution (unchanged) | P0 | Unit | UT-AF-1493-001 | Pass |
| BR-API-1493 | `ParseRARID`: rar_id with slash is NOT split (name-only) | P0 | Unit | UT-AF-1959-002 | Pass |
| BR-API-1493 | `ParseRARID`: fallback to explicit ns+name (unchanged) | P1 | Unit | UT-AF-1493-003 | Pass |
| BR-API-1493 | `ParseRARID`: error on empty inputs (unchanged) | P1 | Unit | UT-AF-1493-004 | Pass |
| BR-API-1493 | Metadata `approval_request_name` is bare name | P0 | Unit | UT-AF-1959-001 | Pass |
| BR-API-1493 | Bare name holds regardless of RR namespace | P1 | Unit | UT-AF-1959-002 (handler) | Pass |
| BR-API-1493 | Handler resolves bare rar_id shorthand | P0 | Unit | UT-AF-1959-003 | Pass |
| BR-API-1493 | rar_id with embedded namespace no longer overrides injected namespace | P0 | Adversarial | ADV-AF-1959-006 | Pass |

## 6. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| `ParseRARID` (simplified) | `HandleGetApprovalRequest` | `crd_tools.go` | UT-AF-1959-003 |
| `BuildPhaseMetadata` (bare name) | SSE status subscription | `status_types.go` | UT-AF-1959-001 |

## 7. Risks

| Risk | Mitigation |
|------|-----------|
| A production caller relied on `namespace/name` shorthand | Confirmed only producer was `BuildPhaseMetadata` (being fixed) and the console passes values through verbatim (confirmed by reading `ChatContainer.tsx`) — no other producer exists |
| release/v1.5 divergence | `release/v1.5` never received `cdbdf2452`'s metadata change, so only needs the `ParseRARID` simplification (tracked separately) |

## 8. Environment

- Unit tests: `go test ./pkg/apifrontend/...`
- `go build ./...` clean
