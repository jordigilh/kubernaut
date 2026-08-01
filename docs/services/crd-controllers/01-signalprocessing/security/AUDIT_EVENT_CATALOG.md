# Audit Event Catalog — Signal Processing (SP)

Authoritative reference for all structured audit events emitted by the `signalprocessing` controller.

**Source of truth:** `pkg/signalprocessing/audit/client.go` (`EventType*` const block, lines 38-50). No `AllEventTypes`-style exported slice exists.
**Payload mapping:** `pkg/signalprocessing/audit/client.go` (`build*Payload` functions), `pkg/signalprocessing/audit/manager.go` (`Record*` wrappers)
**Predecessor doc:** [DD-AUDIT-003](../../../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) §"Signal Processing Controller" documents 4 events under an incorrect `signal-processing.` (hyphenated) prefix; the real prefix is `signalprocessing.` (no hyphen), and SP actually emits 6 events, only one of which overlaps conceptually with the baseline. This catalog is the current, code-verified reference.

**Schema:** `EventCategory = "signalprocessing"` (`CategorySignalProcessing`) for all events. Actor: `service`/`signalprocessing-controller`. Resource: `SignalProcessing`/name. CorrelationID: `RemediationRequestRef.Name`. Fleet provenance: `cluster_id` set conditionally per DD-AUDIT-003 v2.2 (CC8.1).

---

## Events

| Event Type | Constant | Action | Trigger | Data Fields |
|-----------|----------|--------|---------|--------------|
| `signalprocessing.signal.processed` | `EventTypeSignalProcessed` | `processed` | Signal processing finishes the Categorizing phase and transitions to `Completed`/`Failed` | `Phase`, `Signal`; optional `Severity`, `SignalMode`, `SourceSignalName`, `Environment`/`EnvironmentSource`, `Priority`/`PrioritySource`, `Criticality`, `SLARequirement`, `HasOwnerChain`, `OwnerChainLength`, `DegradedMode`, `Error`. Outcome: `failure` if `Status.Phase == PhaseFailed`, else `success` |
| `signalprocessing.phase.transition` | `EventTypePhaseTransition` | `phase_transition` | Every phase change (`""→Pending→Enriching→Classifying→Categorizing→Completed`); no-op transitions are skipped | `Phase` (current), `FromPhase`, `ToPhase` |
| `signalprocessing.classification.decision` | `EventTypeClassificationDecision` | `classification` | Environment/priority/severity/business classification computed during the Classifying phase, when a severity result exists (ADR-034 Table 3) | `DurationMs`, `Severity`, `ExternalSeverity`, `NormalizedSeverity`, `DeterminationSource="rego-policy"`, `PolicyHash`, `SignalMode`, `SourceSignalName`, `Environment`/`EnvironmentSource`, `Priority`/`PrioritySource`, `Criticality`, `SLARequirement` (SOC2 CC7.4) |
| `signalprocessing.business.classified` | `EventTypeBusinessClassified` | `classification` | Final completion, only if `Status.BusinessClassification != nil` (AUDIT-06) | `Severity`, `BusinessUnit`, `Criticality`, `SLARequirement` |
| `signalprocessing.enrichment.completed` | `EventTypeEnrichmentComplete` | `enrichment` | K8s context enrichment finishes successfully (idempotency-guarded, SP-BUG-ENRICHMENT-001) | `DurationMs`, `HasNamespace`, `HasPod`, `HasDeployment`, `OwnerChainLength`, `DegradedMode` |
| `signalprocessing.error.occurred` | `EventTypeError` | `error` | Hard failure in either the Enriching phase (K8sEnricher error) or the Classifying phase (Rego-policy-evaluation error) — a single generic error event covers both | `Phase` (`"Enriching"` or `"Classifying"`), `Signal`, `Error`. Outcome always `failure`. **No `RemediationRequestRef.Name` guard** — unlike every other SP event, this one always attempts to write even with an empty correlation ID |

**Emitted from:** `pkg/signalprocessing/audit/client.go` (`RecordSignalProcessed`, `RecordPhaseTransition`, `RecordClassificationDecision`, `RecordBusinessClassification`, `RecordEnrichmentComplete`, `RecordError`), called from `internal/controller/signalprocessing/*.go` (`signalprocessing_controller.go`, `signalprocessing_enriching.go`, `signalprocessing_classifying.go`, `signalprocessing_categorizing.go`). Production wiring confirmed at `cmd/signalprocessing/main.go` (`spaudit.NewAuditClient`/`NewManager`).

---

## Known Gaps (tracked, not fixed by this catalog)

1. **Prefix mismatch corrected**: DD-AUDIT-003 documents all SP events under `signal-processing.` (hyphenated); the real, consistent, code-verified prefix is `signalprocessing.` (no hyphen) — confirmed in the Go constant, the emission call sites, and repo-wide grep.
2. **`signal-processing.enrichment.started`, `.enrichment.failed`, and `.crd.updated` (DD-AUDIT-003 baseline) do not exist.** There is no "started" event for enrichment; enrichment failures are folded into the generic `signalprocessing.error.occurred` (with `Phase="Enriching"`), not a dedicated `enrichment.failed` type; and `crd.updated` has no code equivalent anywhere in the repo.
3. **`signalprocessing.error.occurred` has no correlation-ID guard**, unlike the other 5 events — it can be stored with an empty `CorrelationID` if `RemediationRequestRef.Name` is unset. Worth a follow-up consistency fix, not addressed here.

---

## Adding New Events

1. Define the `EventType`/`Action` constants in `pkg/signalprocessing/audit/client.go`
2. Add a `build*Payload` function and a `Record*` method, wired via `pkg/signalprocessing/audit/manager.go`
3. Wire the emit call at the production reconciler entry point (`internal/controller/signalprocessing/`), never only in a test
4. Update this catalog with the new event's trigger, fields, and NIST/SOC2 control mapping
5. Ensure a UT proves the emission decision and an IT proves it fires through the reconciler entry point (Pyramid Invariant)

---

*Last updated: 2026-07-31 | QE readiness audit follow-up (DD-AUDIT-003 single-source-of-truth migration) | Covers all 6 event types in the `pkg/signalprocessing/audit/client.go` const block*
