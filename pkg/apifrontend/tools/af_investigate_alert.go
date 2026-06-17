package tools

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	apiprom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
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
// nil Signaler skips IS CRD co-creation (backward compat).
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

	if err := validate.AlertName(args.AlertName); err != nil {
		incFailure("alert_name")
		return InvestigateAlertResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.APIVersion(args.APIVersion); err != nil {
		incFailure("api_version")
		return InvestigateAlertResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.Kind(args.Kind); err != nil {
		incFailure("kind")
		return InvestigateAlertResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.Name == "" {
		incFailure("name")
		return InvestigateAlertResult{}, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}

	clusterScoped := args.Namespace == ""

	if cfg.Mapper != nil {
		gv, parseErr := schema.ParseGroupVersion(args.APIVersion)
		if parseErr == nil {
			mapping, mapErr := cfg.Mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: args.Kind}, gv.Version)
			if mapErr == nil {
				isNamespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
				if isNamespaced && args.Namespace == "" {
					incFailure("scope_mismatch")
					return InvestigateAlertResult{}, fmt.Errorf(
						"%w: kind %q in apiVersion %q is namespaced but namespace was not provided",
						ErrInvalidInput, args.Kind, args.APIVersion)
				}
				if !isNamespaced && args.Namespace != "" {
					incFailure("scope_mismatch")
					return InvestigateAlertResult{}, fmt.Errorf(
						"%w: kind %q in apiVersion %q is cluster-scoped but namespace %q was provided",
						ErrInvalidInput, args.Kind, args.APIVersion, args.Namespace)
				}
				clusterScoped = !isNamespaced
			}
		}
	}

	if !clusterScoped {
		if err := validate.Namespace(args.Namespace); err != nil {
			incFailure("namespace")
			return InvestigateAlertResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
	}

	alertValidated := false
	if cfg.PromClient != nil {
		found, err := validateAlertExists(ctx, cfg.PromClient, args.AlertName)
		if err != nil {
			return InvestigateAlertResult{}, fmt.Errorf("alert validation failed: %w", err)
		}
		if !found {
			incFailure("alert_not_found")
			return InvestigateAlertResult{}, fmt.Errorf("%w: alert %q not found in active alerts or defined rules", ErrInvalidInput, args.AlertName)
		}
		alertValidated = true
	}

	createArgs := &CreateRRArgs{
		Namespace:          args.Namespace,
		Kind:               args.Kind,
		Name:               args.Name,
		APIVersion:         args.APIVersion,
		ClusterScoped:      clusterScoped,
		Description:        fmt.Sprintf("Alert-driven investigation: %s", args.AlertName),
		SignalNameOverride: args.AlertName,
	}

	result, err := HandleCreateRR(ctx, cfg.Client, cfg.DynClient, cfg.ControllerNS, createArgs, username, cfg.Triager, cfg.Auditor)
	if err != nil {
		return InvestigateAlertResult{}, fmt.Errorf("create RR for alert investigation: %w", err)
	}

	if cfg.Signaler != nil {
		rrName := extractRRNameFromID(result.RRID)
		taskID := "a2a-" + rrName
		signalerUsername, signalerGroups := resolveSignalerIdentity(ctx, username)
		if signalErr := cfg.Signaler.SignalInteractive(ctx, taskID, rrName, signalerUsername, signalerGroups); signalErr != nil {
			// Best-effort: do not fail RR creation (BR-INTERACTIVE-010 #1440).
			// The signaler adapter logs internally; suppressing here avoids
			// adding a logger dependency to this pure function.
			_ = signalErr
		}
	}

	launcher.SetRRContextSafe(ctx, &launcher.RRContext{
		RRID:      result.RRID,
		Namespace: args.Namespace,
		Kind:      args.Kind,
		Target:    args.Name,
		AlertName: args.AlertName,
		Phase:     "Investigating",
	})

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
			"The backend validates the alert exists and creates a RemediationRequest.",
	}, func(ctx tool.Context, args InvestigateAlertArgs) (InvestigateAlertResult, error) {
		return HandleInvestigateAlert(ctx, cfg, &args, usernameFromContext(ctx))
	})
}
