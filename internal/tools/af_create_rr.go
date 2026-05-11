package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut-apifrontend/internal/severity"
	"github.com/jordigilh/kubernaut-apifrontend/internal/validate"
)

// maxDescriptionLen is the maximum length for RR description (truncated, not rejected).
const maxDescriptionLen = 2048

// validSeverities is the allowlist of accepted severity values for RemediationRequests.
var validSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
	"info":     true,
}

// CreateRRArgs defines the input for af_create_rr.
type CreateRRArgs struct {
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description"`
}

// CreateRRResult is the output of af_create_rr.
type CreateRRResult struct {
	RRID          string `json:"rr_id"`
	Message       string `json:"message"`
	AlreadyExists bool   `json:"already_exists,omitempty"`
}

// rrCreateGroup provides singleflight deduplication per fingerprint.
// Dedup is intentionally user-agnostic: concurrent RR creation for the same
// target resource is deduplicated regardless of which user initiated it.
// This is acceptable because RR ownership is tracked via labels (reported-by),
// and the check_existing_rr safety net prevents duplicate CRDs regardless.
// Note: parallel tests with the same fingerprint may share flights (by design).
var rrCreateGroup singleflight.Group

func rrFingerprint(namespace, kind, name string) string {
	h := sha256.Sum256([]byte(namespace + "/" + kind + "/" + name))
	return fmt.Sprintf("%x", h[:16])
}

// HandleCreateRR creates a RemediationRequest CRD with singleflight deduplication.
// Concurrent calls with the same fingerprint are deduplicated — only one creation executes.
// If severity is empty and a triager is available, severity is determined via triage pipeline.
func HandleCreateRR(ctx context.Context, client dynamic.Interface, args *CreateRRArgs, username string, triager *severity.Triager) (CreateRRResult, error) {
	if client == nil {
		return CreateRRResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return CreateRRResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return CreateRRResult{}, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}
	if args.Name == "" {
		return CreateRRResult{}, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}
	if args.Severity != "" && !validSeverities[args.Severity] {
		return CreateRRResult{}, fmt.Errorf("%w: severity must be one of critical, high, medium, low, info", ErrInvalidInput)
	}

	if len(args.Description) > maxDescriptionLen {
		args.Description = args.Description[:maxDescriptionLen]
	}

	// Invoke triage pipeline when severity is not user-supplied
	var triageResult *severity.TriageResult
	if args.Severity == "" && triager != nil {
		input := severity.TriageInput{
			Namespace:   args.Namespace,
			Kind:        args.Kind,
			Name:        args.Name,
			Description: args.Description,
			Labels:      map[string]string{"namespace": args.Namespace, "kind": args.Kind, "name": args.Name},
		}
		result, err := triager.Triage(ctx, input)
		if err != nil {
			return CreateRRResult{}, fmt.Errorf("severity triage failed: %w", err)
		}
		if result.Severity != "" {
			args.Severity = result.Severity
			triageResult = &result
		}
	}

	fingerprint := rrFingerprint(args.Namespace, args.Kind, args.Name)

	result, err, _ := rrCreateGroup.Do(fingerprint, func() (interface{}, error) {
		existing, checkErr := HandleCheckExistingRR(ctx, client, CheckExistingRRArgs{
			Namespace: args.Namespace,
			Kind:      args.Kind,
			Name:      args.Name,
		})
		if checkErr != nil {
			return nil, checkErr
		}
		if existing.Exists {
			return &CreateRRResult{
				RRID:          existing.RRID,
				Message:       fmt.Sprintf("RemediationRequest already exists (%s)", existing.Phase),
				AlreadyExists: true,
			}, nil
		}

		rrName := fmt.Sprintf("rr-%s-%s-%d", args.Kind, args.Name, time.Now().UnixMilli())
		if len(rrName) > 63 {
			rrName = rrName[:63]
		}

		spec := map[string]interface{}{
			"targetRef": map[string]interface{}{
				"kind":      args.Kind,
				"name":      args.Name,
				"namespace": args.Namespace,
			},
			"severity":    args.Severity,
			"description": args.Description,
			"reportedBy":  username,
		}

		if triageResult != nil {
			signalLabels := map[string]interface{}{
				"severity_source": string(triageResult.Source),
			}
			if triageResult.AlertName != "" {
				signalLabels["severity_alert_name"] = triageResult.AlertName
			}
			if triageResult.RuleName != "" {
				signalLabels["severity_rule_name"] = triageResult.RuleName
			}
			spec["signalLabels"] = signalLabels
		}

		rr := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]interface{}{
					"name":      rrName,
					"namespace": args.Namespace,
					"labels": map[string]interface{}{
						"kubernaut.ai/target-kind": args.Kind,
						"kubernaut.ai/target-name": args.Name,
						"kubernaut.ai/reported-by": username,
					},
				},
				"spec": spec,
			},
		}

		created, createErr := client.Resource(rrGVR).Namespace(args.Namespace).Create(ctx, rr, metav1.CreateOptions{})
		if createErr != nil {
			return nil, ToUserFriendlyError(createErr)
		}

		return &CreateRRResult{
			RRID:    created.GetNamespace() + "/" + created.GetName(),
			Message: fmt.Sprintf("RemediationRequest created for %s/%s by %s", args.Kind, args.Name, username),
		}, nil
	})

	if err != nil {
		return CreateRRResult{}, err
	}
	res, ok := result.(*CreateRRResult)
	if !ok {
		return CreateRRResult{}, fmt.Errorf("unexpected singleflight result type")
	}
	return *res, nil
}

// NewCreateRRTool creates the af_create_rr tool.
func NewCreateRRTool(client dynamic.Interface, triager *severity.Triager) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_create_rr",
		Description: "Create a RemediationRequest for a target resource with deduplication. Checks for existing non-terminal RRs before creating.",
	}, func(ctx tool.Context, args CreateRRArgs) (CreateRRResult, error) {
		return HandleCreateRR(ctx, client, &args, usernameFromContext(ctx), triager)
	})
}
