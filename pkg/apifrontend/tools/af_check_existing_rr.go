package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// CheckExistingRRArgs defines the LLM-supplied input for kubernaut_check_existing_remediation.
// Namespace is the workload namespace where the target resource lives (LLM-provided).
// controllerNS for CRD listing is injected separately at wiring time (ADR-057).
type CheckExistingRRArgs struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// CheckExistingRRResult is the output of kubernaut_check_existing_remediation.
type CheckExistingRRResult struct {
	Exists bool   `json:"exists"`
	RRID   string `json:"rr_id,omitempty"`
	Phase  string `json:"phase,omitempty"`
}

// HandleCheckExistingRR checks whether a non-terminal RemediationRequest already
// exists for the given target fingerprint.
//
// controllerNS is where RR CRDs are stored (ADR-057) — used for listing.
// args.Namespace is the workload namespace — used for fingerprint computation.
func HandleCheckExistingRR(ctx context.Context, client dynamic.Interface, controllerNS string, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
	if client == nil {
		return CheckExistingRRResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(controllerNS); err != nil {
		return CheckExistingRRResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return CheckExistingRRResult{}, fmt.Errorf("%w: workload namespace: %w", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return CheckExistingRRResult{}, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}
	if err := validate.Kind(args.Kind); err != nil {
		return CheckExistingRRResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.Name == "" {
		return CheckExistingRRResult{}, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}

	fingerprint := rrFingerprint(args.Namespace, args.Kind, args.Name)

	list, err := client.Resource(rrGVR).Namespace(controllerNS).List(ctx, metav1.ListOptions{})
	if err != nil {
		return CheckExistingRRResult{}, ToUserFriendlyError(err)
	}

	for _, item := range list.Items {
		fp, _, _ := unstructured.NestedString(item.Object, "spec", "signalFingerprint")
		if fp != fingerprint {
			continue
		}
		phase, _, _ := unstructured.NestedString(item.Object, "status", "overallPhase")
		if !IsTerminalPhase(phase) {
			return CheckExistingRRResult{
				Exists: true,
				RRID:   item.GetName(),
				Phase:  phase,
			}, nil
		}
	}

	return CheckExistingRRResult{Exists: false}, nil
}

// NewCheckExistingRemediationTool creates the kubernaut_check_existing_remediation tool.
// controllerNS is injected at wiring time for CRD listing (ADR-057). The LLM
// provides the workload namespace via args.Namespace for fingerprint matching.
func NewCheckExistingRemediationTool(client dynamic.Interface, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_check_existing_remediation",
		Description: "Check if an active remediation already exists for a target resource (deduplication check)",
	}, func(ctx tool.Context, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
		return HandleCheckExistingRR(ctx, client, controllerNS, args)
	})
}
