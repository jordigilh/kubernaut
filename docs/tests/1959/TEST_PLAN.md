# Test Plan: Simplify ParseRARID to Bare-Name-Only (Issue #1959)

**Service**: apifrontend
**Version**: 1.0 (release/v1.5 port)
**Created**: 2026-08-06
**Author**: AI Assistant
**Status**: Approved
**Business Requirement**: BR-API-1493 (amends #1493)

---

## 1. Purpose

Remove the redundant `namespace/name` tolerance from `ParseRARID`, so
`rar_id` handling matches the bare-name-only precedent already established
for `rr_id` (`ParseRRID`, `2bfd24c31`).

## 2. Background

This is the `release/v1.5` port of the `main` fix. `release/v1.5` received
`#1493`'s `ParseRARID` (via #1957/#1958) but never received the sibling
commit (`cdbdf2452`, #1492) that added a namespace prefix to
`BuildPhaseMetadata`'s `approval_request_name` — so on `release/v1.5`,
`BuildPhaseMetadata` already emits a bare name and needs no change. Only
`ParseRARID` needs simplifying here.

Per **ADR-057** (CRD Namespace Consolidation), every `RemediationApprovalRequest`
is always created in the single controller namespace — there is no legitimate
scenario where a RAR's namespace differs from `controllerNS`. The
`namespace/name` branch in `ParseRARID` therefore could never resolve
anything the bare-name branch (using the injected, trusted `controllerNS`)
couldn't already resolve, and it introduced a minor trust-boundary
inconsistency: it used the caller-supplied namespace segment instead of the
injected `controllerNS`.

## 3. Objectives

- `ParseRARID` becomes structurally identical to `ParseRRID`: bare-name-only,
  namespace always sourced from the injected argument
- Preserve the legitimate bare-name-acceptance fix from #1493
- No regressions in existing approval request flows; no console changes

## 4. Scope

### In Scope

- `ParseRARID` (`helpers.go`): drop the `namespace/name` split branch
- Adversarial tests proving a `rar_id` containing `/` is now rejected as an
  invalid K8s resource name rather than reinterpreted as `namespace/name`

### Out of Scope

- `BuildPhaseMetadata` (already bare name on this branch — no change needed)
- Console-side changes (none required)

## 5. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-API-1493 | `ParseRARID`: bare name resolution (unchanged) | P0 | Unit | UT-AF-1493-001 | Pass |
| BR-API-1493 | `ParseRARID`: rar_id with slash is NOT split (name-only) | P0 | Unit | UT-AF-1959-002 | Pass |
| BR-API-1493 | `ParseRARID`: fallback to explicit ns+name (unchanged) | P1 | Unit | UT-AF-1493-003 | Pass |
| BR-API-1493 | `ParseRARID`: error on empty inputs (unchanged) | P1 | Unit | UT-AF-1493-004 | Pass |
| BR-API-1493 | Handler resolves bare rar_id shorthand | P0 | Unit | UT-AF-1959-003 | Pass |
| BR-API-1493 | rar_id with embedded namespace no longer overrides injected namespace | P0 | Adversarial | ADV-AF-1959-006 | Pass |

## 6. Risks

| Risk | Mitigation |
|------|-----------|
| A production caller relied on `namespace/name` shorthand | Confirmed only producer was `BuildPhaseMetadata` (already bare on this branch) and the console passes values through verbatim — no other producer exists |

## 7. Environment

- Unit tests: `go test ./pkg/apifrontend/...`
- `go build ./...` clean
