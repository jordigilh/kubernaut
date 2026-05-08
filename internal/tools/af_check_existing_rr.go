package tools

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

// CheckExistingRRArgs defines the input for af_check_existing_rr.
type CheckExistingRRArgs struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
}

// CheckExistingRRResult is the output of af_check_existing_rr.
type CheckExistingRRResult struct {
	Exists bool   `json:"exists"`
	RRID   string `json:"rr_id,omitempty"`
	Phase  string `json:"phase,omitempty"`
}

// HandleCheckExistingRR checks whether a non-terminal RemediationRequest already
// exists for the given target fingerprint (namespace+kind+name).
func HandleCheckExistingRR(ctx context.Context, client dynamic.Interface, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
	if client == nil {
		return CheckExistingRRResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return CheckExistingRRResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return CheckExistingRRResult{}, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}
	if args.Name == "" {
		return CheckExistingRRResult{}, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}

	labelSel := fmt.Sprintf("kubernaut.ai/target-kind=%s,kubernaut.ai/target-name=%s", args.Kind, args.Name)
	list, err := client.Resource(rrGVR).Namespace(args.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSel,
	})
	if err != nil {
		return CheckExistingRRResult{}, ToUserFriendlyError(err)
	}

	for _, item := range list.Items {
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		if !IsTerminalPhase(phase) {
			return CheckExistingRRResult{
				Exists: true,
				RRID:   item.GetNamespace() + "/" + item.GetName(),
				Phase:  phase,
			}, nil
		}
	}

	return CheckExistingRRResult{Exists: false}, nil
}

// NewCheckExistingRRTool creates the af_check_existing_rr tool.
func NewCheckExistingRRTool(client dynamic.Interface) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_check_existing_rr",
		Description: "Check if a non-terminal RemediationRequest already exists for a target resource (deduplication check)",
	}, func(ctx tool.Context, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
		return HandleCheckExistingRR(ctx, client, args)
	})
}
