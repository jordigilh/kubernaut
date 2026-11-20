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

// SignalInteractiveWithRetry exposes signalInteractiveWithRetry for external
// test packages (#2289's bounded-retry unit tests), decoupled from the full
// HandleInvestigationMCPWithRegistry/HandleCreateRRWithHooks machinery.
var SignalInteractiveWithRetry = signalInteractiveWithRetry

// SignalInteractiveRequest exposes signalInteractiveRequest for external
// test packages calling SignalInteractiveWithRetry (#2289).
type SignalInteractiveRequest = signalInteractiveRequest

// SignalInteractiveMaxAttempts exposes signalInteractiveMaxAttempts for
// external test packages asserting exact retry-exhaustion counts (#2289).
const SignalInteractiveMaxAttempts = signalInteractiveMaxAttempts

// CreateRRForInvestigation exposes createRRForInvestigation for external test
// packages so the #2658 confirmed_api_group mapping (InvestigatemcpArgs ->
// CreateRRArgs) can be proven without the full MCP await machinery
// (IT-AF-2658-104).
var CreateRRForInvestigation = createRRForInvestigation
