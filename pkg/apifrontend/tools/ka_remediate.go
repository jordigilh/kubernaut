package tools

import (
	"context"
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
)

// getNestedString safely extracts a nested string value from an unstructured map.
func getNestedString(obj map[string]interface{}, fields ...string) string {
	current := obj
	for i, f := range fields {
		if i == len(fields)-1 {
			if v, ok := current[f].(string); ok {
				return v
			}
			return ""
		}
		nested, ok := current[f].(map[string]interface{})
		if !ok {
			return ""
		}
		current = nested
	}
	return ""
}

// RemediateArgs defines the LLM-supplied input for kubernaut_remediate.
// Autonomous remediation: creates RR without creating an InvestigationSession.
type RemediateArgs struct {
	Namespace   string `json:"namespace,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	RRID        string `json:"rr_id,omitempty"`
}

// RemediateResult is the output of kubernaut_remediate.
type RemediateResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
}

// HandleRemediate creates a RemediationRequest CRD without creating an
// InvestigationSession. This is for autonomous remediation flows where
// the pipeline handles analysis without user interaction.
//
// If args.RRID is set, it looks up the existing RR status (deduplication path).
// Otherwise, it delegates to HandleCreateRR for CRD creation.
func HandleRemediate(ctx context.Context, client dynamic.Interface, controllerNS string, args *RemediateArgs, username string, triager *severity.Triager, auditor audit.Emitter) (RemediateResult, error) {
	if client == nil {
		return RemediateResult{}, ErrK8sUnavailable
	}

	if args.RRID != "" {
		ns, name, parseErr := ParseRRID(args.RRID, controllerNS, "")
		if parseErr != nil {
			return RemediateResult{}, fmt.Errorf("lookup existing RR: %w", parseErr)
		}
		rr, getErr := client.Resource(rrGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return RemediateResult{
				RRID:          args.RRID,
				Message:       "RemediationRequest not found",
				AlreadyExists: false,
			}, nil
		}
		phase := getNestedString(rr.Object, "status", "phase")
		return RemediateResult{
			RRID:          rr.GetName(),
			Message:       fmt.Sprintf("RemediationRequest already exists (%s)", phase),
			AlreadyExists: true,
		}, nil
	}

	createArgs := &CreateRRArgs{
		Namespace:   args.Namespace,
		Kind:        args.Kind,
		Name:        args.Name,
		Description: args.Description,
	}

	result, err := HandleCreateRR(ctx, client, controllerNS, createArgs, username, triager, auditor)
	if err != nil {
		return RemediateResult{}, err
	}

	return RemediateResult(result), nil
}

// NewRemediateTool creates the kubernaut_remediate tool for autonomous remediation.
// It creates RRs without InvestigationSessions — the pipeline handles analysis
// autonomously without user interaction.
func NewRemediateTool(client dynamic.Interface, controllerNS string, triager *severity.Triager, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_remediate",
		Description: "Create a RemediationRequest for autonomous remediation. Use when fixing issues without interactive investigation. The pipeline will analyze and remediate automatically.",
	}, func(ctx tool.Context, args RemediateArgs) (RemediateResult, error) {
		return HandleRemediate(ctx, client, controllerNS, &args, usernameFromContext(ctx), triager, auditor)
	})
}
