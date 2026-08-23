package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	apiprom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// AlertISSignaler creates an InvestigationSession CRD to signal interactive
// intent. Used by HandleInvestigateAlert to co-create the IS alongside the RR,
// closing the temporal gap where AA might submit autonomously
// (BR-INTERACTIVE-010, #1440). Best-effort: errors are logged but do not fail
// the RR creation.
type AlertISSignaler interface {
	SignalInteractive(ctx context.Context, taskID, rrName, username string, groups []string) error
}

// InvestigateAlertConfig holds dependencies for kubernaut_investigate_alert.
// All nil-safe: nil PromClient skips alert validation, nil Mapper skips
// RESTMapper scope checks, nil ValidationFailures skips metric emission,
// nil Signaler skips IS CRD co-creation (backward compat), nil ScopeChecker
// skips management-scope validation (backward compat; ADR-053 Addendum
// "Point 3", #2025/#2022).
type InvestigateAlertConfig struct {
	Client             crclient.Client
	DynClient          dynamic.Interface
	ControllerNS       string
	Triager            *severity.Triager
	PromClient         apiprom.Client
	Auditor            audit.Emitter
	ValidationFailures *prometheus.CounterVec
	Mapper             meta.RESTMapper
	Signaler           AlertISSignaler
	ScopeChecker       scope.ScopeChecker
}

// InvestigateAlertArgs defines the LLM-supplied input for kubernaut_investigate_alert.
// All fields except Namespace are required. Namespace is optional for cluster-scoped
// resources (e.g., Node). The backend validates the alert exists in Prometheus and
// correlates it to the specified resource.
type InvestigateAlertArgs struct {
	AlertName  string `json:"alert_name"`
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	// ClusterID identifies the fleet cluster the target resource lives on
	// (#1409, ADR-065). Empty for the local hub cluster.
	ClusterID string `json:"cluster_id,omitempty"`
	// ConfirmedSignalName re-supplies a previously-surfaced ambiguous
	// severity candidate's alert name after the user has explicitly
	// confirmed it (DD-AF-012, #2027/#2028). Leave empty on the first call.
	// Note AlertName above is always the RR's signalName here (already
	// user-confirmed) -- this field only affects severity resolution.
	ConfirmedSignalName string `json:"confirmed_signal_name,omitempty"`
}

// InvestigateAlertResult is the output of kubernaut_investigate_alert.
type InvestigateAlertResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	AlertValidated bool   `json:"alert_validated"`
	SignalName     string `json:"signal_name"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
	// Ambiguous/CandidateSignalName/CandidateSeverity: see CreateRRResult
	// (DD-AF-012, #2027/#2028). Re-call with ConfirmedSignalName set to
	// CandidateSignalName once the user has confirmed it.
	Ambiguous           bool   `json:"ambiguous,omitempty"`
	CandidateSignalName string `json:"candidate_signal_name,omitempty"`
	CandidateSeverity   string `json:"candidate_severity,omitempty"`
}

// HandleInvestigateAlert creates a RemediationRequest for a specific alert+resource pair.
// It validates that:
//  1. All required fields are present and well-formed (FedRAMP input validation)
//  2. The alert exists in Prometheus (active alerts or defined rules)
//  3. The RR is created with the alert name as signalName
//
// When cfg.PromClient is nil, alert validation is skipped (graceful degradation).
func HandleInvestigateAlert(
	ctx context.Context,
	cfg InvestigateAlertConfig,
	args *InvestigateAlertArgs,
	username string,
) (InvestigateAlertResult, error) {
	incFailure := func(reason string) {
		if cfg.ValidationFailures != nil {
			cfg.ValidationFailures.WithLabelValues(reason).Inc()
		}
	}

	if cfg.Client == nil {
		return InvestigateAlertResult{}, ErrK8sUnavailable
	}

	if err := validateInvestigateAlertFields(args, incFailure); err != nil {
		return InvestigateAlertResult{}, err
	}

	clusterScoped, err := resolveInvestigateAlertScope(ctx, cfg.Mapper, args, incFailure)
	if err != nil {
		return InvestigateAlertResult{}, err
	}

	if !clusterScoped {
		if err := validate.Namespace(args.Namespace); err != nil {
			incFailure("namespace")
			return InvestigateAlertResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
	}

	alertValidated, err := validateAlertIfConfigured(ctx, cfg.PromClient, args.AlertName, incFailure)
	if err != nil {
		return InvestigateAlertResult{}, err
	}

	createArgs := &CreateRRArgs{
		Namespace:                    args.Namespace,
		Kind:                         args.Kind,
		Name:                         args.Name,
		APIVersion:                   args.APIVersion,
		ClusterScoped:                clusterScoped,
		Description:                  fmt.Sprintf("Alert-driven investigation: %s", args.AlertName),
		SignalNameOverride:           args.AlertName,
		ClusterID:                    args.ClusterID,
		ConfirmedAmbiguousSignalName: args.ConfirmedSignalName,
	}

	hooks, hookFired := buildAlertPreCreateISHooks(cfg.Signaler, username)
	result, err := HandleCreateRRWithHooks(ctx, &ToolDeps{Client: cfg.Client, DynClient: cfg.DynClient, ControllerNS: cfg.ControllerNS, Triager: cfg.Triager, Auditor: cfg.Auditor, ScopeChecker: cfg.ScopeChecker}, createArgs, username, hooks)
	if err != nil {
		return InvestigateAlertResult{}, fmt.Errorf("create RR for alert investigation: %w", err)
	}

	if result.Ambiguous {
		return InvestigateAlertResult{
			AlertValidated:      alertValidated,
			SignalName:          args.AlertName,
			Ambiguous:           true,
			CandidateSignalName: result.CandidateSignalName,
			CandidateSeverity:   result.CandidateSeverity,
			Message:             result.Message,
		}, nil
	}

	// #2265: the dedup (AlreadyExists) branch predates this call -- its RR
	// was never at risk of this ordering race, so it still goes through the
	// original post-hoc signal below. A genuinely-new RR was already
	// signaled from BeforeCreate, strictly before it became visible to any
	// other component; signaling it again here would just be a duplicate.
	if !*hookFired {
		signalInteractiveIfConfigured(ctx, cfg.Signaler, result.RRID, username)
	}

	launcher.SetRRContextSafe(ctx, newlyCreatedRRContext(result.RRID, args.Namespace, args.Kind, args.Name, args.AlertName, result.ClusterID))

	return InvestigateAlertResult{
		RRID:           result.RRID,
		Message:        result.Message,
		AlreadyExists:  result.AlreadyExists,
		AlertValidated: alertValidated,
		SignalName:     args.AlertName,
		Severity:       result.Severity,
		SeveritySource: result.SeveritySource,
	}, nil
}

// validateInvestigateAlertFields validates the required, well-formed-input
// checks for kubernaut_investigate_alert (FedRAMP input validation),
// incrementing incFailure with the appropriate reason on the first violation.
func validateInvestigateAlertFields(args *InvestigateAlertArgs, incFailure func(string)) error {
	if err := validate.AlertName(args.AlertName); err != nil {
		incFailure("alert_name")
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.APIVersion(args.APIVersion); err != nil {
		incFailure("api_version")
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.Kind(args.Kind); err != nil {
		incFailure("kind")
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.Name == "" {
		incFailure("name")
		return fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}
	return nil
}

// resolveInvestigateAlertScope determines whether the target resource is
// cluster-scoped, using the RESTMapper (when available) as the authoritative
// source and falling back to "empty namespace means cluster-scoped" when the
// mapper is unavailable or the kind/apiVersion can't be resolved. When the
// mapper disagrees with the caller-supplied namespace, it either rejects the
// request (namespaced kind, no namespace given) or strips a superfluous
// namespace (cluster-scoped kind, namespace given) rather than trusting the
// LLM-supplied value.
func resolveInvestigateAlertScope(ctx context.Context, mapper meta.RESTMapper, args *InvestigateAlertArgs, incFailure func(string)) (bool, error) {
	clusterScoped := args.Namespace == ""
	if mapper == nil {
		return clusterScoped, nil
	}

	gv, parseErr := schema.ParseGroupVersion(args.APIVersion)
	if parseErr != nil {
		logr.FromContextOrDiscard(ctx).Info("cannot parse apiVersion, falling back to namespace-based scope heuristic",
			"apiVersion", args.APIVersion, "error", parseErr.Error())
		// nolint:nilerr // intentional: already documented above ("falling
		// back to ... when the mapper is unavailable or the kind/apiVersion
		// can't be resolved") -- an unparseable apiVersion degrades to the
		// heuristic, it doesn't fail the tool call (Issue #1546 Tier 3).
		return clusterScoped, nil
	}
	mapping, mapErr := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: args.Kind}, gv.Version)
	if mapErr != nil {
		logr.FromContextOrDiscard(ctx).Info("RESTMapping failed, falling back to namespace-based scope heuristic",
			"kind", args.Kind, "apiVersion", args.APIVersion, "error", mapErr.Error())
		// nolint:nilerr // same documented fallback idiom as above (Issue
		// #1546 Tier 3).
		return clusterScoped, nil
	}

	isNamespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
	if isNamespaced && args.Namespace == "" {
		incFailure("scope_mismatch")
		return false, fmt.Errorf(
			"%w: kind %q in apiVersion %q is namespaced but namespace was not provided",
			ErrInvalidInput, args.Kind, args.APIVersion)
	}
	if !isNamespaced && args.Namespace != "" {
		logr.FromContextOrDiscard(ctx).Info("stripping namespace for cluster-scoped resource",
			"kind", args.Kind,
			"apiVersion", args.APIVersion,
			"stripped_namespace", args.Namespace,
		)
		args.Namespace = ""
	}
	return !isNamespaced, nil
}

// validateAlertIfConfigured checks alertName exists in Prometheus when
// promClient is configured (graceful degradation when nil, validation is
// skipped and alertValidated is reported false).
func validateAlertIfConfigured(ctx context.Context, promClient apiprom.Client, alertName string, incFailure func(string)) (bool, error) {
	if promClient == nil {
		return false, nil
	}
	found, err := validateAlertExists(ctx, promClient, alertName)
	if err != nil {
		return false, fmt.Errorf("alert validation failed: %w", err)
	}
	if !found {
		incFailure("alert_not_found")
		return false, fmt.Errorf("%w: alert %q not found in active alerts or defined rules", ErrInvalidInput, alertName)
	}
	return true, nil
}

// signalInteractiveIfConfigured co-creates the InvestigationSession CRD via
// signaler, when configured, to signal interactive intent (BR-INTERACTIVE-010,
// #1440). Best-effort: errors are swallowed here (the signaler adapter logs
// internally) so a signaling failure never fails RR creation.
func signalInteractiveIfConfigured(ctx context.Context, signaler AlertISSignaler, rrID, username string) {
	if signaler == nil {
		return
	}
	rrName := extractRRNameFromID(rrID)
	taskID := "a2a-" + rrName
	signalerUsername, signalerGroups := resolveSignalerIdentity(ctx, username)
	_ = signaler.SignalInteractive(ctx, taskID, rrName, signalerUsername, signalerGroups)
}

// buildAlertPreCreateISHooks builds the CreateRRHooks (#2265) that let
// HandleInvestigateAlert signal interactive intent (and later back-fill the
// resulting IS CRD's OwnerReference) strictly between "the new RR's name is
// known" and "the RR becomes visible to any other component" -- closing the
// same ordering race documented on buildPreCreateISHooks in
// ka_investigate_mcp.go. Unlike that path, this one preserves the existing
// fire-and-forget contract exactly (#1440, SC-24): BeforeCreate never
// aborts RR creation regardless of the signaler's outcome. The returned
// *bool reports whether BeforeCreate fired at all (regardless of success),
// so the caller knows whether to fall back to the original post-hoc
// signalInteractiveIfConfigured call.
func buildAlertPreCreateISHooks(signaler AlertISSignaler, username string) (CreateRRHooks, *bool) {
	hookFired := new(bool)
	if signaler == nil {
		return CreateRRHooks{}, hookFired
	}

	signalSucceeded := false
	hooks := CreateRRHooks{
		BeforeCreate: func(ctx context.Context, rrName string) error {
			*hookFired = true
			taskID := "a2a-" + rrName
			signalerUsername, signalerGroups := resolveSignalerIdentity(ctx, username)
			if sigErr := signaler.SignalInteractive(ctx, taskID, rrName, signalerUsername, signalerGroups); sigErr != nil {
				logr.FromContextOrDiscard(ctx).Error(sigErr, "failed to create IS CRD before RR (proceeding without IS signal)", "rr_name", rrName)
				return nil
			}
			signalSucceeded = true
			return nil
		},
		AfterCreate: func(ctx context.Context, rr *remediationv1.RemediationRequest) {
			if !signalSucceeded {
				return
			}
			if backfiller, ok := signaler.(OwnerReferenceBackfiller); ok {
				backfiller.BackfillOwnerReference(ctx, rr.Namespace, rr.Name, rr.UID)
			}
		},
	}
	return hooks, hookFired
}

// extractRRNameFromID extracts the RR name from an RRID like "namespace/name".
func extractRRNameFromID(rrid string) string {
	for i := len(rrid) - 1; i >= 0; i-- {
		if rrid[i] == '/' {
			return rrid[i+1:]
		}
	}
	return rrid
}

// resolveSignalerIdentity extracts the username and groups from auth context,
// falling back to the provided username if no identity is in context.
func resolveSignalerIdentity(ctx context.Context, fallbackUsername string) (string, []string) {
	if identity := auth.UserIdentityFromContext(ctx); identity != nil && identity.Username != "" {
		return identity.Username, identity.Groups
	}
	return fallbackUsername, nil
}

// validateAlertExists checks if the given alert name exists in Prometheus active
// alerts (firing or pending) or defined alerting rules. Returns true if found.
func validateAlertExists(ctx context.Context, promClient apiprom.Client, alertName string) (bool, error) {
	alerts, err := promClient.GetAlerts(ctx)
	if err != nil {
		return false, fmt.Errorf("get alerts: %w", err)
	}
	for _, a := range alerts {
		if a.Labels["alertname"] == alertName {
			return true, nil
		}
	}

	rules, err := promClient.GetRules(ctx)
	if err != nil {
		return false, fmt.Errorf("get rules: %w", err)
	}
	for _, g := range rules {
		for _, r := range g.Rules {
			if r.Type == "alerting" && r.Name == alertName {
				return true, nil
			}
		}
	}

	return false, nil
}

// NewInvestigateAlertTool creates the kubernaut_investigate_alert ADK tool.
// This tool allows the LLM to explicitly specify an alert name when creating
// an investigation, rather than relying on the backend's deterministic triage.
func NewInvestigateAlertTool(cfg InvestigateAlertConfig) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_investigate_alert",
		Description: "Create an investigation for a specific Prometheus alert targeting a Kubernetes resource. " +
			"Provide alert_name (the Prometheus alert name), api_version, kind, and name of the target resource. " +
			"For namespaced resources, also provide namespace. " +
			"For fleet (multi-cluster) deployments, also provide cluster_id to identify which cluster the " +
			"resource lives on; omit for the local hub cluster. " +
			"The backend validates the alert exists and creates a RemediationRequest.",
	}, func(ctx agent.Context, args InvestigateAlertArgs) (InvestigateAlertResult, error) {
		return HandleInvestigateAlert(ctx, cfg, &args, usernameFromContext(ctx))
	})
}
