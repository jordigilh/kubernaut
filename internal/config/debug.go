package config

// DebugConfig holds developer/operator diagnostic toggles shared by all
// services. The Go zero value is the secure default (AC-6 least privilege):
// every field here must default to "off" so that a service booted with an
// empty or partial config never exposes a diagnostic surface unintentionally.
//
// Issue #2275: replaces the per-service negative-polarity DisableProfiling
// (default true = OFF) with a positive-polarity, shared field so that
// kubernaut-operator's CRD spec.<component>.debug.pprofEnabled maps 1:1 to
// this struct with no negation-translation layer at the operator boundary.
type DebugConfig struct {
	// PprofEnabled gates the /debug/pprof/* endpoints (net/http/pprof) on
	// the service's health/metrics listener, or a dedicated pprof listener
	// for controller-runtime-managed services. Defaults to false (profiling
	// OFF) -- an operator must explicitly opt in per BR-PLATFORM-012.
	PprofEnabled bool `yaml:"pprofEnabled"`
}

// DefaultDebugConfig returns the secure-by-default DebugConfig. Provided so
// callers building a DefaultConfig() can be explicit about the choice
// rather than relying on an implicit zero value.
func DefaultDebugConfig() DebugConfig {
	return DebugConfig{PprofEnabled: false}
}

// PprofBindAddress returns the controller-runtime pprof listener address
// for ctrl.Options.PprofBindAddress (BR-PLATFORM-012, Issue #2275), shared
// by the 7 controller-runtime-managed services (aianalysis, authwebhook,
// effectivenessmonitor, notification, remediationorchestrator,
// signalprocessing, workflowexecution) to avoid duplicating this ternary
// across each service's build*Manager. An empty string disables the
// listener entirely, per manager.Options.PprofBindAddress's own doc
// comment -- controller-runtime only starts a pprof server when non-empty.
func PprofBindAddress(enabled bool) string {
	if enabled {
		return ":6060"
	}
	return ""
}
