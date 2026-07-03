# DD-AUDIT-008: Audit Event Builder Registry Pattern

**Status**: ✅ **APPROVED & IMPLEMENTED** (2026-07-01)
**Priority**: P2 (Code Quality / Maintainability)
**Owner**: KubernautAgent Team
**Scope**: `internal/kubernautagent/audit` package (private, single call site)
**Related**: [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) (audit trace requirements), [DD-AUDIT-004-structured-types-for-audit-event-payloads.md](./DD-AUDIT-004-structured-types-for-audit-event-payloads.md) (typed payload mandate this function fulfills), [GO-ANTIPATTERN-AUDIT-2026-07-01](../audits/GO-ANTIPATTERN-AUDIT-2026-07-01.md) Phase 4

---

## 📋 Context & Problem

### Problem Statement

`buildEventData` in [internal/kubernautagent/audit/ds_store.go](../../../internal/kubernautagent/audit/ds_store.go) maps a `KubernautAgent`-internal `AuditEvent` to one of 29 typed `ogenclient.AuditEventRequestEventData` payload variants (one per `EventType*` constant), via a single `switch event.EventType { case ...: ...; case ...: ... }` with one case per event type. This is the sole caller of `StoreAudit`'s event-data construction step.

The Go anti-pattern audit ([GO-ANTIPATTERN-AUDIT-2026-07-01](../audits/GO-ANTIPATTERN-AUDIT-2026-07-01.md)) flagged `buildEventData` as the highest-complexity function in the repository by raw case count: cyclomatic complexity 88, cognitive complexity 117, driven entirely by the 29-way type switch (not by nested branching — each case body is a flat, linear struct literal).

### Why This Needs a Documented Decision

Per `AGENTS.md`'s REFACTOR-phase rule ("NEVER create new types or components in REFACTOR — enhance existing only") and CHECKPOINT DD, introducing a **new dispatch mechanism** (a function type + a package-level map) rather than simply extracting the existing `switch` cases into helper functions is a new pattern for this codebase's event-handling code, not a pure "enhance existing" change. It therefore requires a documented alternatives analysis and user approval before implementation.

## Alternatives Considered

### Alternative A — Extract each case into a helper function, keep the `switch` (rejected)

```go
func buildEventData(event *AuditEvent) (ogenclient.AuditEventRequestEventData, bool) {
	switch event.EventType {
	case EventTypeEnrichmentCompleted:
		return buildEnrichmentCompletedPayload(event), true
	case EventTypeEnrichmentFailed:
		return buildEnrichmentFailedPayload(event), true
	// ... 27 more cases ...
	default:
		return ogenclient.AuditEventRequestEventData{}, false
	}
}
```

- **Pros**: Minimal-diff change; keeps the compiler's exhaustiveness intuition (a `switch` on a `string`-backed type has no exhaustiveness checking either way, but the shape is familiar); zero new types.
- **Cons**: Cyclomatic complexity of `buildEventData` itself barely improves (88 → ~29, one branch per case, still the single largest function in the repo by branch count); every new event type still requires touching the dispatch function itself (adding a `case` line) in addition to writing the builder — two edits for one conceptual change.

### Alternative B — Registry / lookup-table (chosen)

```go
type eventDataBuilder func(event *AuditEvent) ogenclient.AuditEventRequestEventData

var eventDataBuilders = map[string]eventDataBuilder{
	EventTypeEnrichmentCompleted: buildEnrichmentCompletedPayload,
	EventTypeEnrichmentFailed:    buildEnrichmentFailedPayload,
	// ... 27 more entries ...
}

func buildEventData(event *AuditEvent) (ogenclient.AuditEventRequestEventData, bool) {
	builder, ok := eventDataBuilders[event.EventType]
	if !ok {
		return ogenclient.AuditEventRequestEventData{}, false
	}
	return builder(event), true
}
```

- **Pros**: `buildEventData` itself drops to cyclomatic complexity 2 (a single `ok` check) — it is no longer "the most complex function in the repo" by any measure; adding a new event type is a single map entry (data), not a code-path edit (control flow), which better matches the "one event type = one row" mental model already used elsewhere in this codebase (e.g. `AllEventTypes []string` in the same package); each builder function remains independently testable exactly as it was in Alternative A.
- **Cons**: Introduces one new function type (`eventDataBuilder`) and one new package-level `map` — technically "new components," but both are private to `internal/kubernautagent/audit`, have a single call site (`buildEventData`), and do not change the exported `AuditStore`/`StoreAudit` API surface; map lookup has a marginally higher constant-time cost than a jump-table `switch`, but this function is called at most once per audit event on a non-hot-path (async-buffered per ADR-038), so the perf delta is immaterial; a typo'd map key silently falls through to the `!ok` branch (same silent-drop risk a `default:` case in Alternative A would have) — mitigated by the existing `UT-KA-PR9-001` test asserting every `AllEventTypes` entry produces non-zero `EventData`, which fails loudly if a builder is missing from the map.

### Decision

**Alternative B (registry/lookup-table)** was selected by explicit user decision during Phase 4 planning of the Go anti-pattern remediation (2026-07-01), prioritizing the larger complexity reduction and the "add a row, not a branch" extensibility property over the marginal cost of one new private function type + map.

## Consequences

- **Positive**: `buildEventData` cyclomatic complexity drops from 88 to ~2; adding event type #30 in the future requires (a) a new `build<EventType>Payload` function and (b) one `eventDataBuilders` map entry — no edit to `buildEventData` itself.
- **Positive**: Each of the 29 extracted `build<EventType>Payload` functions is independently unit-testable (already partially true — see per-event-type tests in `internal/kubernautagent/audit`), and the existing `UT-KA-PR9-001` all-event-types smoke test continues to prove no event type is silently dropped from the registry.
- **Negative (accepted)**: Two new package-private symbols (`eventDataBuilder` type, `eventDataBuilders` map) exist that didn't before. Scope is contained: private to `internal/kubernautagent/audit`, single call site, no change to the `AuditStore` interface or any exported type.
- **No behavior change**: `StoreAudit` (the sole caller of `buildEventData`) and its request/response contract with DataStorage are unchanged; this is a pure internal refactor of event-data construction dispatch.

## Implementation

Implemented in Phase 4d of [GO-ANTIPATTERN-AUDIT-2026-07-01](../audits/GO-ANTIPATTERN-AUDIT-2026-07-01.md) — see `internal/kubernautagent/audit/ds_store.go`. Verified via the existing `UT-KA-PR9-001` all-event-types coverage test plus the full `internal/kubernautagent/audit` suite, with identical assertions before and after (same-package, same-behavior move).
