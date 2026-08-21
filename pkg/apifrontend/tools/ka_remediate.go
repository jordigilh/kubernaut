package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"k8s.io/client-go/dynamic"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// RemediateArgs defines the LLM-supplied input for kubernaut_remediate.
// Autonomous remediation: creates RR without creating an InvestigationSession.
type RemediateArgs struct {
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	RRID        string `json:"rr_id,omitempty"`
	// APIVersion is the Kubernetes API group/version (e.g., "apps/v1", "v1").
	// Required when providing namespace/kind/name (#1372).
	APIVersion string `json:"api_version"`
	// ClusterID identifies the fleet cluster the target resource lives on
	// (#1409, ADR-065). Empty for the local hub cluster.
	ClusterID string `json:"cluster_id,omitempty"`
	// ConfirmedSignalName re-supplies a previously-surfaced ambiguous
	// candidate's alert name after the user has explicitly confirmed it
	// (DD-AF-012, #2027/#2028). Leave empty on the first call.
	ConfirmedSignalName string `json:"confirmed_signal_name,omitempty"`
}

// RemediateResult is the output of kubernaut_remediate.
type RemediateResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
	SignalName     string `json:"signal_name,omitempty"`
	// ClusterID attributes the RR to its cluster of origin (#1409).
	ClusterID string `json:"cluster_id,omitempty"`
	// Fingerprint (#2043), Ambiguous/CandidateSignalName/CandidateSeverity
	// (DD-AF-012, #2027/#2028), and ClusterID above must stay in this exact
	// field order/types, matching CreateRRResult's trailing fields, so the
	// RemediateResult(result) conversion below stays valid (Go struct
	// conversion requires identical field sequences; tags may differ).
	// Re-call with ConfirmedSignalName set to CandidateSignalName once the
	// user has confirmed an ambiguous candidate.
	Fingerprint         string `json:"-"`
	Ambiguous           bool   `json:"ambiguous,omitempty"`
	CandidateSignalName string `json:"candidate_signal_name,omitempty"`
	CandidateSeverity   string `json:"candidate_severity,omitempty"`
}

// HandleRemediate creates a RemediationRequest CRD without creating an
// InvestigationSession. This is for autonomous remediation flows where
// the pipeline handles analysis without user interaction.
//
// If args.RRID is set, it looks up the existing RR status (deduplication path).
// Otherwise, it delegates to HandleCreateRR for CRD creation.
func HandleRemediate(ctx context.Context, d *ToolDeps, args *RemediateArgs, username string) (RemediateResult, error) {
	if d.Client == nil {
		return RemediateResult{}, ErrK8sUnavailable
	}

	if args.RRID != "" {
		ns, name, parseErr := ParseRRID(args.RRID, d.ControllerNS, "")
		if parseErr != nil {
			return RemediateResult{}, fmt.Errorf("lookup existing RR: %w", parseErr)
		}
		var rr remediationv1.RemediationRequest
		if getErr := d.Client.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr); getErr != nil {
			// Any Get failure (not-found, transient API error, RBAC) is
			// treated as "no existing RR" so the LLM can proceed with
			// creation rather than getting stuck on a dedup check --
			// mirrors HandleReconnect's tolerant lookup (#1472, SI-10).
			logr.FromContextOrDiscard(ctx).Info("kubernaut_remediate: RR lookup failed, treating as not found",
				"rr_id", args.RRID, "error", getErr.Error())
			//nolint:nilerr // intentional soft-fail: any Get error becomes
			// AlreadyExists=false so the LLM proceeds with creation instead
			// of surfacing a raw K8s error on a best-effort dedup check.
			return RemediateResult{
				RRID:          args.RRID,
				Message:       "RemediationRequest not found",
				AlreadyExists: false,
			}, nil
		}
		return RemediateResult{
			RRID:          rr.Name,
			Message:       fmt.Sprintf("RemediationRequest already exists (%s)", rr.Status.OverallPhase),
			AlreadyExists: true,
			ClusterID:     rr.Spec.ClusterID,
		}, nil
	}

	if err := validate.APIVersion(args.APIVersion); err != nil {
		return RemediateResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	createArgs := &CreateRRArgs{
		Namespace:                    args.Namespace,
		Kind:                         args.Kind,
		Name:                         args.Name,
		Description:                  args.Description,
		APIVersion:                   args.APIVersion,
		ClusterScoped:                args.Namespace == "",
		ClusterID:                    args.ClusterID,
		ConfirmedAmbiguousSignalName: args.ConfirmedSignalName,
	}

	result, err := HandleCreateRR(ctx, d, createArgs, username)
	if err != nil {
		return RemediateResult{}, err
	}

	if result.Ambiguous {
		return RemediateResult{
			Ambiguous:           true,
			CandidateSignalName: result.CandidateSignalName,
			CandidateSeverity:   result.CandidateSeverity,
			Message:             result.Message,
		}, nil
	}

	launcher.SetRRContextSafe(ctx, newlyCreatedRRContext(result.RRID, args.Namespace, args.Kind, args.Name, result.SignalName, result.ClusterID))

	return RemediateResult(result), nil
}

// NewRemediateTool creates the kubernaut_remediate tool for autonomous remediation.
// It creates RRs without InvestigationSessions — the pipeline handles analysis
// autonomously without user interaction.
func NewRemediateTool(client crclient.Client, dynClient dynamic.Interface, controllerNS string, triager *severity.Triager, auditor audit.Emitter, scopeChecker scope.ScopeChecker) (tool.Tool, error) {
	d := &ToolDeps{
		Client:       client,
		DynClient:    dynClient,
		ControllerNS: controllerNS,
		Triager:      triager,
		Auditor:      auditor,
		ScopeChecker: scopeChecker,
	}
	return functiontool.New(functiontool.Config{
		Name: "kubernaut_remediate",
		Description: "Create a RemediationRequest for autonomous remediation. Use when fixing issues without interactive investigation. " +
			"The pipeline will analyze and remediate automatically. " +
			"For fleet (multi-cluster) deployments, also provide cluster_id to identify which cluster the resource lives on; omit for the local hub cluster.",
	}, func(ctx agent.Context, args RemediateArgs) (RemediateResult, error) {
		return HandleRemediate(ctx, d, &args, usernameFromContext(ctx))
	})
}
