package tools

import (
	"context"
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// CheckExistingRRArgs defines the LLM-supplied input for kubernaut_check_existing_remediation.
// Namespace is the workload namespace where the target resource lives (LLM-provided).
// controllerNS for CRD listing is injected separately at wiring time (ADR-057).
type CheckExistingRRArgs struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	// ClusterID scopes the dedup fingerprint to a specific fleet cluster
	// (#1409, AC-4). Empty string preserves pre-#1409 local-hub matching
	// behavior for backward compatibility.
	ClusterID string `json:"cluster_id,omitempty"`
}

// CheckExistingRRResult is the output of kubernaut_check_existing_remediation.
type CheckExistingRRResult struct {
	Exists   bool   `json:"exists"`
	RRID     string `json:"rr_id,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Severity string `json:"severity,omitempty"`
	// ClusterID attributes the existing RR to its cluster of origin (#1409).
	ClusterID string `json:"cluster_id,omitempty"`
}

// HandleCheckExistingRR checks whether a non-terminal RemediationRequest already
// exists for the given target fingerprint.
//
// controllerNS is where RR CRDs are stored (ADR-057) — used for listing.
// args.Namespace is the workload namespace — used for fingerprint computation.
func HandleCheckExistingRR(ctx context.Context, client crclient.Client, controllerNS string, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
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
	if err := validate.ClusterID(args.ClusterID); err != nil {
		return CheckExistingRRResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	fingerprint := rrFingerprintWithCluster(args.ClusterID, args.Namespace, args.Kind, args.Name)

	var list remediationv1.RemediationRequestList
	if err := client.List(ctx, &list, crclient.InNamespace(controllerNS)); err != nil {
		return CheckExistingRRResult{}, ToUserFriendlyError(err)
	}

	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.SignalFingerprint != fingerprint {
			continue
		}
		phase := string(item.Status.OverallPhase)
		if !IsTerminalPhase(phase) {
			return CheckExistingRRResult{
				Exists:    true,
				RRID:      item.Name,
				Phase:     phase,
				ClusterID: item.Spec.ClusterID,
			}, nil
		}
	}

	return CheckExistingRRResult{Exists: false}, nil
}

// NewCheckExistingRemediationTool creates the kubernaut_check_existing_remediation tool.
// controllerNS is injected at wiring time for CRD listing (ADR-057). The LLM
// provides the workload namespace via args.Namespace for fingerprint matching.
func NewCheckExistingRemediationTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_check_existing_remediation",
		Description: "Check if an active remediation already exists for a target resource (deduplication check)",
	}, func(ctx tool.Context, args CheckExistingRRArgs) (CheckExistingRRResult, error) {
		return HandleCheckExistingRR(ctx, client, controllerNS, args)
	})
}
