# BR-AA-HAPI-064: Session-Based Pull Design for AA-HAPI Communication

**Status**: APPROVED
**Version**: 1.1
**Created**: 2026-02-09
**Category**: AI
**Priority**: P1 - High
**Services Affected**: AIAnalysis Controller, HolmesGPT-API (HAPI)
**GitHub Issue**: #64

---

## Context

The current architecture has the AA controller make a blocking HTTP call to HAPI's `/api/v1/incident/analyze` endpoint and wait for the full LLM investigation to complete. With real LLM providers (Vertex AI, Anthropic), this takes 2-3 minutes per investigation, creating:

- Timeout fragility: HTTP client timeout hardcoded to 10 minutes (BR-ORCH-036 v3.0 workaround)
- Retry inefficiency: timeout kills the entire LLM investigation, must restart from scratch
- Resource waste: AA controller goroutine blocked for full LLM duration
- No progress visibility: no insight until full response or timeout

---

## Requirements

### BR-AA-HAPI-064.1: Async Submit Endpoint
HAPI MUST accept investigation requests and return a session identifier immediately (HTTP 202 Accepted) without waiting for LLM completion.

### BR-AA-HAPI-064.2: Session Status Polling
HAPI MUST provide an endpoint to check investigation session status (pending, investigating, completed, failed).

### BR-AA-HAPI-064.3: Session Result Retrieval
HAPI MUST provide an endpoint to retrieve the completed investigation result by session ID.

### BR-AA-HAPI-064.4: AA Controller Session Tracking (InvestigationSession)
The AA controller MUST store session state in the AIAnalysis CRD status using an `InvestigationSession` sub-structure:

```go
type InvestigationSession struct {
    ID         string       `json:"id,omitempty"`
    Generation int32        `json:"generation,omitempty"`
    LastPolled *metav1.Time `json:"lastPolled,omitempty"`
    CreatedAt  *metav1.Time `json:"createdAt,omitempty"`
}
```

### BR-AA-HAPI-064.5: Session Regeneration on HAPI Restart
When a poll returns 404 (session not found, typically due to HAPI restart), the AA controller MUST:
1. Increment `InvestigationSession.Generation`
2. Clear `InvestigationSession.ID`
3. Resubmit the investigation request to get a new session
4. Update `InvestigationSession.CreatedAt`

### BR-AA-HAPI-064.6: Session Regeneration Cap
The AA controller MUST cap session regenerations at 5. After 5 regenerations, the AA MUST transition to Failed with:
- `SubReason: "SessionRegenerationExceeded"`
- Escalation notification to operators
- Warning K8s Event per DD-EVENT-001

### BR-AA-HAPI-064.7: InvestigationSessionReady Condition
The AA controller MUST maintain an `InvestigationSessionReady` Condition on the AIAnalysis CRD:
- True/SessionCreated when a new session is submitted
- True/SessionActive when polls succeed
- False/SessionLost when session is lost (404)
- True/SessionRegenerated when resubmit after loss succeeds
- False/SessionRegenerationExceeded when cap is exceeded

### BR-AA-HAPI-064.8: Polling with Controller-Runtime Requeue
The AA controller MUST use controller-runtime `RequeueAfter` for polling, not blocking waits. A constant poll interval of 15s is used (configurable via `--session-poll-interval` flag or `WithSessionPollInterval` option). The original recommended backoff (10s, 20s, 30s) was replaced with a constant interval for simplicity and predictable load patterns.

### BR-AA-HAPI-064.9: Recovery Flow Support
~~The same async pattern MUST apply to recovery investigations (`/api/v1/recovery/analyze`).~~

**DEPRECATED for v1.0**: Recovery investigations are deprecated. Instead, when a remediation is ineffective, the alert re-fires through the Gateway and the existing AI analysis results (from the prior Effectiveness Assessment) are included in the HAPI prompt context. The RO routing engine has logic to prevent this from becoming an endless cycle. Recovery flow may be revisited in future versions.

### BR-AA-HAPI-064.10: Timeout Removal
Once the async design is validated, the 10-minute hardcoded timeout workaround in `cmd/aianalysis/main.go` MUST be removed. All HTTP calls become short-lived (~30s timeout).

---

## Acceptance Criteria

- [ ] HAPI exposes async session endpoints (submit, poll, result) for incident analysis (recovery deprecated for v1.0)
- [ ] AA controller stores InvestigationSession in CRD status
- [ ] AA controller polls with constant requeue interval (not blocking)
- [ ] Stale session detection and regeneration works (Generation counter increments)
- [ ] Generation cap at 5 triggers Failed + escalation
- [ ] InvestigationSessionReady Condition reflects session lifecycle
- [ ] Warning Event emitted on SessionRegenerationExceeded per DD-EVENT-001
- [ ] Existing error classification and escalation (BR-ORCH-036 v3.0) preserved
- [ ] Unit tests for submit/poll logic
- [ ] Integration tests for async flow
- [ ] E2E test validates full pipeline with async design
- [ ] 10m timeout removed after validation

---

## Dependencies

- DD-EVENT-001: Controller K8s Event Registry (prerequisite for Warning Event)
- BR-ORCH-036 v3.0: Existing escalation notification pattern (preserved)
- DD-AA-HAPI-064: Detailed design document (implementation guidance)

---

## Changelog

### v1.1 (2026-03-04)
- BR-AA-HAPI-064.8: Updated to reflect constant 15s poll interval design decision (replaces 10s/20s/30s backoff)
- BR-AA-HAPI-064.9: Marked recovery flow as deprecated for v1.0 (alert re-fire through Gateway replaces dedicated recovery endpoint)

### v1.0 (2026-02-09)
- Initial version based on GitHub issue #64 analysis
- Added InvestigationSession sub-structure (ID, Generation, LastPolled, CreatedAt)
- Added InvestigationSessionReady Condition with 5 reason states
- Added session regeneration cap of 5 with escalation
- Added Warning Event requirement per DD-EVENT-001
