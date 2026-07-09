package tools

// IsStatusEvent exposes isStatusEvent for external test packages.
var IsStatusEvent = isStatusEvent

// MapReasonToPhase exposes mapReasonToPhase for external test packages.
var MapReasonToPhase = mapReasonToPhase

// ResolveInvestigationRR exposes resolveInvestigationRR for external test
// packages, so its takeover-fetch decision logic (#1409) can be unit-tested
// directly without going through the full HandleInvestigationMCPWithRegistry
// await/signal machinery (which requires a real MCP/HTTP transport to test
// meaningfully — that wiring is proven separately at the IT tier).
var ResolveInvestigationRR = resolveInvestigationRR
