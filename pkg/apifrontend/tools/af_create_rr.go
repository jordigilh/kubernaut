package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// maxDescriptionLen is the maximum length for RR description (truncated, not rejected).
const maxDescriptionLen = 2048

// CreateRRArgs defines the LLM-supplied input for af_create_rr.
// Namespace and severity are resolved by AF, not the LLM.
type CreateRRArgs struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CreateRRResult is the output of af_create_rr.
type CreateRRResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
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
	return fmt.Sprintf("%x", h)
}

// HandleCreateRR creates a RemediationRequest CRD with singleflight deduplication.
// Namespace is resolved by AF (not the LLM). Severity is resolved via the triage
// pipeline when a triager is available, otherwise defaults to "medium".
func HandleCreateRR(ctx context.Context, client dynamic.Interface, namespace string, args *CreateRRArgs, username string, triager *severity.Triager, auditor audit.Emitter) (CreateRRResult, error) {
	if client == nil {
		return CreateRRResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(namespace); err != nil {
		return CreateRRResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if args.Kind == "" {
		return CreateRRResult{}, fmt.Errorf("%w: kind must not be empty", ErrInvalidInput)
	}
	if args.Name == "" {
		return CreateRRResult{}, fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}

	if len(args.Description) > maxDescriptionLen {
		args.Description = args.Description[:maxDescriptionLen]
	}

	resolvedSeverity := "medium"
	var triageResult *severity.TriageResult
	if triager != nil {
		input := severity.TriageInput{
			Namespace:   namespace,
			Kind:        args.Kind,
			Name:        args.Name,
			Description: args.Description,
			Labels:      map[string]string{"namespace": namespace, "kind": args.Kind, "name": args.Name},
		}
		result, err := triager.Triage(ctx, input)
		if err != nil {
			return CreateRRResult{}, fmt.Errorf("severity triage failed: %w", err)
		}
		if result.Severity != "" {
			resolvedSeverity = result.Severity
			triageResult = &result
		}
	}

	signalName := deriveSignalName(ctx, client, namespace, args, triageResult)
	fingerprint := rrFingerprint(namespace, args.Kind, args.Name)

	result, err, _ := rrCreateGroup.Do(fingerprint, func() (interface{}, error) {
		existing, checkErr := HandleCheckExistingRR(ctx, client, CheckExistingRRArgs{
			Namespace: namespace,
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

		rrName := fmt.Sprintf("rr-%s-%s-%d", strings.ToLower(args.Kind), strings.ToLower(args.Name), time.Now().UnixMilli())
		if len(rrName) > 63 {
			rrName = rrName[:63]
		}

		now := time.Now().UTC().Format(time.RFC3339)

		spec := map[string]interface{}{
			"signalName":        signalName,
			"signalSource":      "a2a-agent",
			"signalType":        "alert",
			"signalFingerprint": fingerprint,
			"severity":          resolvedSeverity,
			"firingTime":        now,
			"receivedTime":      now,
			"targetType":        "kubernetes",
			"targetResource": map[string]interface{}{
				"kind":      args.Kind,
				"name":      args.Name,
				"namespace": namespace,
			},
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

		rrObj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"metadata": map[string]interface{}{
					"name":      rrName,
					"namespace": namespace,
				},
				"spec": spec,
			},
		}

		created, createErr := client.Resource(rrGVR).Namespace(namespace).Create(ctx, rrObj, metav1.CreateOptions{})
		if createErr != nil {
			return nil, ToUserFriendlyError(createErr)
		}

		out := &CreateRRResult{
			RRID:    created.GetNamespace() + "/" + created.GetName(),
			Message: fmt.Sprintf("RemediationRequest created for %s/%s by %s", args.Kind, args.Name, username),
		}
		if triageResult != nil {
			out.Severity = triageResult.Severity
			out.SeveritySource = string(triageResult.Source)
		} else {
			out.Severity = resolvedSeverity
		}
		return out, nil
	})

	if err != nil {
		return CreateRRResult{}, err
	}
	res, ok := result.(*CreateRRResult)
	if !ok {
		return CreateRRResult{}, fmt.Errorf("unexpected singleflight result type")
	}

	if auditor != nil {
		if res.AlreadyExists {
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventRRDeduplicated,
				UserID: username,
				Detail: map[string]string{
					"namespace":   namespace,
					"kind":        args.Kind,
					"name":        args.Name,
					"existing_rr": res.RRID,
				},
			})
		} else {
			auditor.Emit(ctx, &audit.Event{
				Type:   audit.EventRRCreated,
				UserID: username,
				Detail: map[string]string{
					"namespace": namespace,
					"kind":      args.Kind,
					"name":      args.Name,
					"rr_id":     res.RRID,
					"severity":  resolvedSeverity,
				},
			})
		}
	}

	return *res, nil
}

// deriveSignalName selects a grounded signal name using a priority cascade:
//  1. Triager AlertName (from Prometheus firing/pending alert — most specific)
//  2. Triager RuleName (from inactive rule match — known rule, not yet firing)
//  3a. Dominant K8s event reason on the target resource (e.g., Deployment)
//  3b. Dominant K8s event reason on Pods owned by the target (name-prefix match)
//  4. Fallback: "unknown" (no grounded infrastructure signal found)
//
// The signal name is critical: KA uses it to drive investigation behavior.
// Every tier above the fallback provides a meaningful infrastructure signal.
//
// Tier 3a queries events on the target resource kind (e.g., Deployment).
// Tier 3b cascades to Pod-level events when 3a finds no operationally
// significant signal. This is necessary because failure events like BackOff,
// OOMKilled, and CrashLoopBackOff are emitted on Pods, not on the owning
// Deployment. Pod events are filtered by name prefix (e.g., "memory-eater-")
// to scope to pods belonging to the specific target resource.
//
// Both tiers use DominantEventReason which filters out Normal lifecycle
// events (F-SIG-08): ScalingReplicaSet, Scheduled, Pulled, Created, etc.
// are not failure signals and would mislead KA's scenario detection.
func deriveSignalName(ctx context.Context, client dynamic.Interface, namespace string, args *CreateRRArgs, triageResult *severity.TriageResult) string {
	if triageResult != nil {
		if triageResult.AlertName != "" {
			return triageResult.AlertName
		}
		if triageResult.RuleName != "" {
			return triageResult.RuleName
		}
	}

	if client != nil {
		// Tier 3a: events on the target resource itself (e.g., Deployment)
		evResult, err := HandleListEvents(ctx, client, ListEventsArgs{
			Namespace: namespace,
			Kind:      args.Kind,
		})
		if err == nil && len(evResult.Events) > 0 {
			if dominant := DominantEventReason(evResult.Events); dominant != "" {
				return dominant
			}
		}

		// Tier 3b: Pod-level events for pods owned by the target resource.
		// BackOff, OOMKilled, CrashLoopBackOff are emitted on Pods, not on
		// the owning Deployment/StatefulSet. Filter by name prefix to scope
		// to pods belonging to this specific owner.
		if args.Kind != "Pod" {
			podResult, err := HandleListEvents(ctx, client, ListEventsArgs{
				Namespace: namespace,
				Kind:      "Pod",
			})
			if err == nil && len(podResult.Events) > 0 {
				related := FilterRelatedPodEvents(podResult.Events, args.Name)
				if dominant := DominantEventReason(related); dominant != "" {
					return dominant
				}
			}
		}
	}

	return "unknown"
}

// NewCreateRRTool creates the af_create_rr tool. Namespace is injected by AF
// (resolved from downward API / config) and is not exposed to the LLM.
func NewCreateRRTool(client dynamic.Interface, namespace string, triager *severity.Triager, auditor audit.Emitter) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "af_create_rr",
		Description: "Create a RemediationRequest for a target resource with deduplication. Checks for existing non-terminal RRs before creating.",
	}, func(ctx tool.Context, args CreateRRArgs) (CreateRRResult, error) {
		return HandleCreateRR(ctx, client, namespace, &args, usernameFromContext(ctx), triager, auditor)
	})
}
