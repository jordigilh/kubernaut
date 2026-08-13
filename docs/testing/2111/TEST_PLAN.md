# Test Plan: #2111 — `EmitArtifact` gob-safety Boundary Hardening (follow-up to #2110's fix)

## 1. Purpose

[#2110](https://github.com/jordigilh/kubernaut/issues/2110) (main clone
[#2111](https://github.com/jordigilh/kubernaut/issues/2111)) was a
release-blocking regression: every `kubernaut_present_decision` call with
grounded RCA content crashed the a2a task with `gob: type not registered for
interface: tools.RCAData`, because `canonicalGroundedRCA` returned a raw
`*tools.RCAData` struct pointer that reached `a2a-go@v0.3.15`'s gob-encoded
task-manager deep-copy pipeline unregistered.

[#2112](https://github.com/jordigilh/kubernaut/pull/2112)/[#2113](https://github.com/jordigilh/kubernaut/pull/2113)
fixed that specific call site (changed `canonicalGroundedRCA`'s return type
to `map[string]any`, added `gob.Register(&RCAData{})` as defense-in-depth).
That fix was correct but not sufficient: it left the underlying structural
gap open. `EventBridge.EmitArtifact`
(`pkg/apifrontend/launcher/event_bridge.go`) is the single choke point all
three current artifact producers share --
`part_converter.go`'s `emitDecisionEvent` (`present_decision`),
`crd_tools.go`'s progress snapshot, and `ka_investigate_mcp.go`'s RCA
artifact -- before their `data` reaches that same gob pipeline, and
`EmitArtifact` performed zero validation of `data`'s gob-safety. Nothing
stopped (and, on `release/v1.5`, still doesn't stop) a future contributor
from reintroducing the identical crash class by assigning any other struct
into any nested field of an artifact's `data` map. `gob.Register(&RCAData{})`
is a manually-maintained per-type whitelist, not a structural guarantee --
the #2112 PR's own comments say as much.

This follow-up (`main` only; `release/v1.5` keeps the already-shipped #2112
fix as-is since it's mid-release) hardens `EmitArtifact` itself so the
guarantee holds for all current and future callers, not just the one that
broke.

### Root Cause Detail (recap, see #2110's own `docs/testing/2110/TEST_PLAN.md` for the original incident)

`encoding/gob` requires any concrete type stored behind an `interface{}` to
be `gob.Register`'d before it can be encoded, UNLESS it's one of a small set
of types Go's own `encoding/gob` package pre-registers for itself at
`init()` (`type.go`): the basic numeric/string/bool types and a handful of
their unnamed slice forms (`[]string`, `[]int`, `[]byte`, etc.). Named
struct types, `map[string]any`, and `[]any` are never in that built-in set --
`a2a-go@v0.3.15`'s own `internal/taskstore/store.go` registers
`map[string]any` and `[]any` itself at `init()` for exactly this reason.

### Design Consideration: Why Not Unconditional JSON Normalization

An earlier iteration of this fix unconditionally JSON-marshaled/unmarshaled
every call's `data`, which trivially guarantees gob-safety but silently
coerces already-compliant values into different Go types on the way through
-- `[]string` becomes `[]any`, `int` becomes `float64`. This broke existing
callers that type-assert on the original shape
(`pkg/apifrontend/tools/ka_investigate_wiring_test.go`'s
UT-AF-1922-001/002 and UT-AF-WIRE-SESSION-003 all asserted
`rcaData["causal_chain"].([]string)` and an `int`-typed
`tool_calls_count`), i.e. it introduced new, avoidable breakage in the name
of hardening.

**Chosen fix**: probe `data` with the real `encoding/gob` encoder first
(`gobProbe`, writing to `io.Discard`) rather than reimplementing gob's own
type-safety rules as a hand-maintained list. Data that already gob-encodes
successfully -- true for every existing caller in this package -- is
returned completely unchanged, type-for-type. Only when the probe fails
(an actually gob-unsafe value, e.g. a bare struct pointer -- exactly #2110's
mistake) does `sanitizeArtifactData` fall back to a JSON marshal/unmarshal
round-trip, which normalizes the offending value into the same
`map[string]any`/`[]any`/primitive shape every compliant caller already
produces by convention, then re-probes the result before returning it.

Also added: an explicit `gob.Register(map[string]any{})` /
`gob.Register([]any{})` in `event_bridge.go`'s own `init()`. This duplicates
what `a2a-go`'s `internal/taskstore/store.go` already does, but removes this
package's gob-safety guarantee from depending on that package happening to
be transitively imported by *something else* in the binary -- an implicit
coupling to an unrelated import-graph decision, not a guarantee this
codebase controls. `gob.Register` is idempotent, so the duplicate
registration is harmless.

## 2. FedRAMP Control Mapping

| Control | Title | Relevance |
|---|---|---|
| **SI-10** | Information Input/Output Validation | The core control, same as #2110: the SSE artifact pipeline must be able to actually deliver the structured artifact it constructs, for every current and future producer, not just the one already known to be compliant. |
| **AU-3** | Content of Audit Records | `kubernaut_present_decision`'s SSE artifact is the audit-visible decision record (#1408); a boundary-level guard prevents any future artifact producer from silently dropping that record the way #2110 did. |
| **SI-11** | Error Handling | When data is genuinely unsanitizable (fails even after JSON normalization, or isn't JSON-marshalable at all), `EmitArtifact` degrades to a text-only artifact and logs + increments a failure metric, rather than propagating an error that kills the whole a2a task the way the original bug did. |

## 3. Pyramid Invariant — Test Scenario Inventory

| ID | Tier | Business-Level Behavior Description | Control | Test File |
|---|---|---|---|---|
| UT-AF-2111-001 (regression) | Unit | A struct pointer nested anywhere in `EmitArtifact`'s `data` must not reach a2a-go's real gob deep-copy pipeline unsanitized -- reproduces the actual gob round-trip, not just a call to `EmitArtifact` | SI-10 | `pkg/apifrontend/launcher/event_bridge_gob_boundary_2111_test.go` |
| UT-AF-2111-002 | Unit | Legitimate JSON-shaped values (strings, floats, bools, nested maps/slices, nil) are preserved byte-for-byte, type-for-type through `EmitArtifact` -- no new normalization for already-compliant callers | SI-10 | same file |
| UT-AF-2111-003 (defense-in-depth) | Unit | Even without `phase_guard.go`'s `canonicalGroundedRCA` fix, an `EmitArtifact` call carrying a `*tools.RCAData`-shaped struct pointer in `rca` is still made gob-safe by the boundary guard alone -- proves the fix is independent of any one caller staying correct | SI-10, AU-3 | same file |
| UT-AF-2111-004 | Unit | Data that cannot be made JSON-safe at all (e.g. a channel value) degrades to a text-only artifact instead of failing `EmitArtifact` -- observable via the existing `IncBridgeWriteFailures` counter | SI-11 | same file |

### Tier Coverage Rationale

- **UT** covers this fix completely, for the same reason #2110's did: the
  actual failure mode is a pure `encoding/gob` property of the data these
  functions produce, fully reproducible without any of `a2a-go`'s
  unexported internals -- these tests gob-encode/decode the exact
  `DataPart.Data` shape `EmitArtifact` hands to `a2a.DataPart`, the literal
  operation that crashed in production.
- **IT/E2E**: not added net-new. `EmitArtifact`'s wiring into all three
  production callers (`emitDecisionEvent`, `crd_tools.go`'s progress
  snapshot, `ka_investigate_mcp.go`'s RCA artifact) already exists and is
  already exercised by each of those callers' own IT/E2E coverage; this
  change modifies `EmitArtifact`'s internal behavior without touching any
  wiring point, so no new Wiring Manifest row applies (per
  `.cursor/rules/10-wiring-verification.mdc`, this is hardening existing
  code, not introducing a new component).

## 4. Validation Results

- `go test ./pkg/apifrontend/launcher/...` -- pass (including the 4 new
  specs above).
- `go test ./pkg/apifrontend/...` (full package, all 22 sub-packages) --
  pass, including the pre-existing `pkg/apifrontend/tools` UT-AF-1922-001/
  002 and UT-AF-WIRE-SESSION-003 specs that caught the unconditional-JSON-
  normalization regression during development of this fix.
- `golangci-lint run pkg/apifrontend/launcher/...` -- 0 issues.
- `gofmt -l` -- clean.
