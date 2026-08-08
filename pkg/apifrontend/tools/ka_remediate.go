package tools

import (
	"context"
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
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
}

// RemediateResult is the output of kubernaut_remediate.
type RemediateResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
	SignalName     string `json:"signal_name,omitempty"`
	// Managed reports whether the target resource is within Kubernaut's
	// management scope (ADR-053). false means no RR was created — Message
	// explains why (#2022). true for the rr_id lookup path too, since no
	// scope check applies there (the RR already exists).
	Managed bool `json:"managed"`
}

// HandleRemediate creates a RemediationRequest CRD without creating an
// InvestigationSession. This is for autonomous remediation flows where
// the pipeline handles analysis without user interaction.
//
// If args.RRID is set, it looks up the existing RR status (deduplication path).
// Otherwise, it delegates to HandleCreateRR for CRD creation.
func HandleRemediate(ctx context.Context, client crclient.Client, dynClient dynamic.Interface, controllerNS string, args *RemediateArgs, username string, triager *severity.Triager, auditor audit.Emitter) (RemediateResult, error) {
	if client == nil {
		return RemediateResult{}, ErrK8sUnavailable
	}

	if args.RRID != "" {
		ns, name, parseErr := ParseRRID(args.RRID, controllerNS, "")
		if parseErr != nil {
			return RemediateResult{}, fmt.Errorf("lookup existing RR: %w", parseErr)
		}
		var rr remediationv1.RemediationRequest
		if getErr := client.Get(ctx, crclient.ObjectKey{Namespace: ns, Name: name}, &rr); getErr != nil {
			return RemediateResult{
				RRID:          args.RRID,
				Message:       "RemediationRequest not found",
				AlreadyExists: false,
				Managed:       true,
			}, nil
		}
		return RemediateResult{
			RRID:          rr.Name,
			Message:       fmt.Sprintf("RemediationRequest already exists (%s)", rr.Status.OverallPhase),
			AlreadyExists: true,
			Managed:       true,
		}, nil
	}

	if err := validate.APIVersion(args.APIVersion); err != nil {
		return RemediateResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	if managed, msg := checkRRScope(ctx, ScopeCheckerFromContext(ctx), auditor, username, args.Namespace, args.Kind, args.Name); !managed {
		return RemediateResult{Managed: false, Message: msg}, nil
	}

	createArgs := &CreateRRArgs{
		Namespace:     args.Namespace,
		Kind:          args.Kind,
		Name:          args.Name,
		Description:   args.Description,
		APIVersion:    args.APIVersion,
		ClusterScoped: args.Namespace == "",
	}

	result, err := HandleCreateRR(ctx, client, dynClient, controllerNS, createArgs, username, triager, auditor)
	if err != nil {
		return RemediateResult{}, err
	}

	launcher.SetRRContextSafe(ctx, &launcher.RRContext{
		RRID:      result.RRID,
		Namespace: args.Namespace,
		Kind:      args.Kind,
		Target:    args.Name,
		AlertName: result.SignalName,
		Phase:     "Investigating",
	})

	return RemediateResult{
		RRID:           result.RRID,
		Message:        result.Message,
		AlreadyExists:  result.AlreadyExists,
		Severity:       result.Severity,
		SeveritySource: result.SeveritySource,
		SignalName:     result.SignalName,
		Managed:        true,
	}, nil
}

// NewRemediateTool creates the kubernaut_remediate tool for autonomous remediation.
// It creates RRs without InvestigationSessions — the pipeline handles analysis
// autonomously without user interaction. checker is optional (nil skips scope
// validation, backward compat) and is threaded into HandleRemediate via context
// rather than widening its already-large positional signature (#2022).
func NewRemediateTool(client crclient.Client, dynClient dynamic.Interface, controllerNS string, triager *severity.Triager, auditor audit.Emitter, checker scope.ScopeChecker) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_remediate",
		Description: "Create a RemediationRequest for autonomous remediation. Use when fixing issues without interactive investigation. The pipeline will analyze and remediate automatically.",
	}, func(ctx tool.Context, args RemediateArgs) (RemediateResult, error) {
		toolCtx := ContextWithScopeChecker(ctx, checker)
		return HandleRemediate(toolCtx, client, dynClient, controllerNS, &args, usernameFromContext(ctx), triager, auditor)
	})
}
