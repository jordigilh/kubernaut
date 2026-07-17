package tools

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	gwtypes "github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// unknownValue is the generic fallback used when a signal name, phase, or
// reason cannot be grounded in any available infrastructure signal.
const unknownValue = "unknown"

// ToolDeps groups infrastructure dependencies shared by tool handler functions.
// Constructed once at wiring time; per-call data (args, username) remains as
// separate parameters.
type ToolDeps struct {
	Client       crclient.Client
	DynClient    dynamic.Interface
	ControllerNS string
	Triager      *severity.Triager
	Auditor      audit.Emitter
}

// maxDescriptionLen is the maximum length for RR description (truncated, not rejected).
const maxDescriptionLen = 2048

// CreateRRArgs defines the input for RR creation (used by kubernaut_remediate and kubernaut_investigate).
// Namespace is the workload namespace where the target resource lives (LLM-provided).
// For cluster-scoped resources (e.g., Node), Namespace is empty and ClusterScoped is true.
// Severity is resolved by AF via the triage pipeline.
type CreateRRArgs struct {
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// APIVersion is the Kubernetes API group/version (e.g., "apps/v1", "v1").
	// Stored in targetResource.apiVersion of the RR CRD (#1372).
	APIVersion string `json:"api_version"`
	// ClusterScoped indicates the target resource is cluster-scoped (e.g., Node).
	// When true, Namespace may be empty. Callers set this after RESTMapper validation.
	ClusterScoped bool `json:"-"`
	// SignalNameOverride, when set, bypasses deriveSignalName and uses this value
	// directly as the RR spec.signalName. Used by kubernaut_investigate_alert
	// where the alert name is the definitive signal (#1372).
	SignalNameOverride string `json:"-"`
	// ClusterID is the cluster identifier from Thanos external_labels.
	// Empty string indicates local hub cluster (ADR-065).
	ClusterID string `json:"cluster_id,omitempty"`
	// ClusterName is the human-readable display name for the cluster.
	// Populated from the MCP Gateway Backend CRD displayName (ADR-065).
	ClusterName string `json:"cluster_name,omitempty"`
}

// CreateRRResult is the output of RR creation.
type CreateRRResult struct {
	RRID           string `json:"rr_id"`
	Message        string `json:"message"`
	AlreadyExists  bool   `json:"already_exists,omitempty"`
	Severity       string `json:"severity,omitempty"`
	SeveritySource string `json:"severity_source,omitempty"`
	SignalName     string `json:"signal_name,omitempty"`
}

// rrCreateGroup provides singleflight deduplication per fingerprint.
// Dedup is intentionally user-agnostic: concurrent RR creation for the same
// target resource is deduplicated regardless of which user initiated it.
// This is acceptable because RR ownership is tracked via labels (reported-by),
// and the check_existing_rr safety net prevents duplicate CRDs regardless.
// Note: parallel tests with the same fingerprint may share flights (by design).
var rrCreateGroup singleflight.Group

func rrFingerprint(namespace, kind, name string) string {
	return rrFingerprintWithCluster("", namespace, kind, name)
}

// rrFingerprintWithCluster generates a dedup fingerprint that includes the cluster
// context. Delegates to gwtypes.CalculateClusterAwareFingerprint to ensure GW and
// AF produce identical fingerprints for the same resource (CC4.2: audit trail consistency).
func rrFingerprintWithCluster(clusterID, namespace, kind, name string) string {
	return gwtypes.CalculateClusterAwareFingerprint(clusterID, gwtypes.ResourceIdentifier{
		Namespace: namespace,
		Kind:      kind,
		Name:      name,
	})
}

// checkExistingRRByFingerprint checks for an existing non-terminal RR by fingerprint.
// This is the internal dedup check used by HandleCreateRR that skips input validation
// (namespace validation already performed by caller).
func checkExistingRRByFingerprint(ctx context.Context, client crclient.Client, controllerNS, fingerprint string) (CheckExistingRRResult, error) {
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
				Exists:   true,
				RRID:     item.Name,
				Phase:    phase,
				Severity: item.Spec.Severity,
			}, nil
		}
	}
	return CheckExistingRRResult{Exists: false}, nil
}

// HandleCreateRR creates a RemediationRequest CRD with singleflight deduplication.
//
// controllerNS is where the RR CRD is placed (metadata.namespace) — injected at
// wiring time from AF's deployment context (ADR-057).
// args.Namespace is the workload namespace where the target resource lives — provided
// by the LLM. Severity is resolved via the triage pipeline when a triager is
// available, otherwise defaults to "warning".
func HandleCreateRR(ctx context.Context, d *ToolDeps, args *CreateRRArgs, username string) (CreateRRResult, error) {
	if d.Client == nil {
		return CreateRRResult{}, ErrK8sUnavailable
	}
	if err := validateCreateRRArgs(d, args); err != nil {
		return CreateRRResult{}, err
	}

	if len(args.Description) > maxDescriptionLen {
		args.Description = args.Description[:maxDescriptionLen]
	}

	resolvedSeverity, triageResult, err := resolveCreateRRSeverity(ctx, d, args)
	if err != nil {
		return CreateRRResult{}, err
	}

	signalName := args.SignalNameOverride
	if signalName == "" {
		signalName = deriveSignalName(ctx, d.DynClient, args.Namespace, args, triageResult)
	}
	fingerprint := rrFingerprintWithCluster(args.ClusterID, args.Namespace, args.Kind, args.Name)

	result, err, _ := rrCreateGroup.Do(fingerprint, func() (interface{}, error) {
		return createOrReuseRR(ctx, d, createRRRequest{
			Args: args, Username: username, Fingerprint: fingerprint,
			SignalName: signalName, ResolvedSeverity: resolvedSeverity, TriageResult: triageResult,
		})
	})
	if err != nil {
		return CreateRRResult{}, fmt.Errorf("create RR for %s/%s: %w", args.Kind, args.Name, err)
	}
	res, ok := result.(*CreateRRResult)
	if !ok {
		return CreateRRResult{}, fmt.Errorf("create RR: unexpected singleflight result type")
	}

	emitCreateRRAudit(ctx, d, args, username, res, resolvedSeverity)
	return *res, nil
}

// validateCreateRRArgs validates all HandleCreateRR inputs, returning a
// wrapped ErrInvalidInput on the first violation found.
func validateCreateRRArgs(d *ToolDeps, args *CreateRRArgs) error {
	if err := validate.Namespace(d.ControllerNS); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.ClusterScoped {
		if args.Namespace != "" {
			return fmt.Errorf("%w: cluster-scoped resources must have empty namespace", ErrInvalidInput)
		}
	} else if err := validate.Namespace(args.Namespace); err != nil {
		return fmt.Errorf("%w: workload namespace: %w", ErrInvalidInput, err)
	}
	if err := validate.Kind(args.Kind); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.Name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidInput)
	}
	if args.APIVersion != "" {
		if err := validate.APIVersion(args.APIVersion); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidInput, err)
		}
	}
	return nil
}

// resolveCreateRRSeverity runs the severity-triage pipeline when a Triager is
// configured, returning the resolved severity (defaulting to "warning") and
// the triage result (nil if no triager, or triage found no severity signal).
func resolveCreateRRSeverity(ctx context.Context, d *ToolDeps, args *CreateRRArgs) (string, *severity.TriageResult, error) {
	if d.Triager == nil {
		return "warning", nil, nil
	}
	input := severity.TriageInput{
		Namespace:   args.Namespace,
		Kind:        args.Kind,
		Name:        args.Name,
		Description: args.Description,
		Labels:      map[string]string{"namespace": args.Namespace, "kind": args.Kind, "name": args.Name},
	}
	result, err := d.Triager.Triage(ctx, input)
	if err != nil {
		return "", nil, fmt.Errorf("severity triage failed: %w", err)
	}
	if result.Severity == "" {
		return "warning", nil, nil
	}
	return result.Severity, &result, nil
}

// createRRRequest bundles the resolved signal identity/severity fields
// createOrReuseRR needs, keeping its parameter count within the argument-limit
// lint gate.
type createRRRequest struct {
	Args             *CreateRRArgs
	Username         string
	Fingerprint      string
	SignalName       string
	ResolvedSeverity string
	TriageResult     *severity.TriageResult
}

// createOrReuseRR is the singleflight-guarded body of HandleCreateRR: it
// returns the existing RR if one is already active for req's fingerprint,
// otherwise creates a new RemediationRequest CRD.
func createOrReuseRR(ctx context.Context, d *ToolDeps, req createRRRequest) (*CreateRRResult, error) {
	existing, checkErr := checkExistingRRByFingerprint(ctx, d.Client, d.ControllerNS, req.Fingerprint)
	if checkErr != nil {
		return nil, checkErr
	}
	if existing.Exists {
		return &CreateRRResult{
			RRID:          existing.RRID,
			Message:       fmt.Sprintf("RemediationRequest already exists (%s)", existing.Phase),
			AlreadyExists: true,
			Severity:      existing.Severity,
		}, nil
	}

	rrObj := buildRRObject(d.ControllerNS, req.Args, req.Fingerprint, req.SignalName, req.ResolvedSeverity, req.TriageResult)
	if createErr := d.Client.Create(ctx, rrObj); createErr != nil {
		return nil, ToUserFriendlyError(createErr)
	}

	out := &CreateRRResult{
		RRID:       rrObj.Name,
		Message:    fmt.Sprintf("RemediationRequest created for %s/%s by %s", req.Args.Kind, req.Args.Name, req.Username),
		SignalName: req.SignalName,
	}
	if req.TriageResult != nil {
		out.Severity = req.TriageResult.Severity
		out.SeveritySource = string(req.TriageResult.Source)
	} else {
		out.Severity = req.ResolvedSeverity
	}
	return out, nil
}

// buildRRObject constructs the RemediationRequest CRD to be created for a new
// (non-duplicate) signal.
func buildRRObject(controllerNS string, args *CreateRRArgs, fingerprint, signalName, resolvedSeverity string, triageResult *severity.TriageResult) *remediationv1.RemediationRequest {
	fpPrefix := fingerprint
	if len(fpPrefix) > 12 {
		fpPrefix = fpPrefix[:12]
	}
	rrName := fmt.Sprintf("rr-%s-%s", fpPrefix, uuid.New().String()[:8])
	nowTime := metav1.Now()

	rrObj := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rrName,
			Namespace: controllerNS,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalName:        signalName,
			SignalSource:      "a2a-agent",
			SignalType:        "alert",
			SignalFingerprint: fingerprint,
			Severity:          resolvedSeverity,
			FiringTime:        nowTime,
			ReceivedTime:      nowTime,
			TargetType:        "kubernetes",
			TargetResource:    buildTypedTargetResource(args),
			ClusterID:         args.ClusterID,
			ClusterName:       args.ClusterName,
		},
	}

	if triageResult != nil {
		rrObj.Spec.SignalLabels = map[string]string{
			"severity_source": string(triageResult.Source),
		}
		if triageResult.AlertName != "" {
			rrObj.Spec.SignalLabels["severity_alert_name"] = triageResult.AlertName
		}
		if triageResult.RuleName != "" {
			rrObj.Spec.SignalLabels["severity_rule_name"] = triageResult.RuleName
		}
	}
	return rrObj
}

// emitCreateRRAudit emits the RR-created or RR-deduplicated audit event, when
// an auditor is configured.
func emitCreateRRAudit(ctx context.Context, d *ToolDeps, args *CreateRRArgs, username string, res *CreateRRResult, resolvedSeverity string) {
	if d.Auditor == nil {
		return
	}
	if res.AlreadyExists {
		d.Auditor.Emit(ctx, &audit.Event{
			Type:        audit.EventRRDeduplicated,
			UserID:      username,
			ClusterName: args.ClusterName,
			Detail: map[string]string{
				"namespace":   d.ControllerNS,
				"kind":        args.Kind,
				"name":        args.Name,
				"existing_rr": res.RRID,
			},
		})
		return
	}
	d.Auditor.Emit(ctx, &audit.Event{
		Type:        audit.EventRRCreated,
		UserID:      username,
		ClusterName: args.ClusterName,
		Detail: map[string]string{
			"namespace": d.ControllerNS,
			"kind":      args.Kind,
			"name":      args.Name,
			"rr_id":     res.RRID,
			"severity":  resolvedSeverity,
		},
	})
}

// buildTypedTargetResource constructs the typed ResourceIdentifier for the RR CRD spec.
// Includes apiVersion when available (#1372). Omits namespace for cluster-scoped resources.
func buildTypedTargetResource(args *CreateRRArgs) remediationv1.ResourceIdentifier {
	ri := remediationv1.ResourceIdentifier{
		Kind: args.Kind,
		Name: args.Name,
	}
	if args.Namespace != "" {
		ri.Namespace = args.Namespace
	}
	if args.APIVersion != "" {
		ri.APIVersion = args.APIVersion
	}
	return ri
}

// deriveSignalName selects a grounded signal name using a priority cascade:
//  1. Triager AlertName (from Prometheus firing/pending alert — most specific)
//  2. Triager RuleName (from inactive rule match — known rule, not yet firing)
//     3a. Dominant K8s event reason on the target resource (e.g., Deployment)
//     3b. Dominant K8s event reason on Pods owned by the target (name-prefix match)
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
	if name := signalNameFromTriage(triageResult); name != "" {
		return name
	}
	if client == nil {
		return unknownValue
	}
	if name := signalNameFromResourceEvents(ctx, client, namespace, args); name != "" {
		return name
	}
	if name := signalNameFromPodEvents(ctx, client, namespace, args); name != "" {
		return name
	}
	return unknownValue
}

// signalNameFromTriage returns the alert/rule name from an upstream
// severity-triage result, if any (highest-priority signal source).
func signalNameFromTriage(triageResult *severity.TriageResult) string {
	if triageResult == nil {
		return ""
	}
	if triageResult.AlertName != "" {
		return triageResult.AlertName
	}
	return triageResult.RuleName
}

// signalNameFromResourceEvents implements Tier 3a: events on the target
// resource itself (e.g., Deployment). Returns "" if none found.
func signalNameFromResourceEvents(ctx context.Context, client dynamic.Interface, namespace string, args *CreateRRArgs) string {
	evResult, err := HandleListEvents(ctx, client, ListEventsArgs{
		Namespace: namespace,
		Kind:      args.Kind,
	})
	if err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "deriveSignalName failed to list events", "kind", args.Kind, "namespace", namespace)
		return ""
	}
	return DominantEventReason(evResult.Events)
}

// signalNameFromPodEvents implements Tier 3b: Pod-level events for pods
// owned by the target resource. BackOff, OOMKilled, CrashLoopBackOff are
// emitted on Pods, not on the owning Deployment/StatefulSet, so this cascade
// is necessary when Tier 3a finds no operationally significant signal.
// Filtered by name prefix to scope to pods belonging to this specific owner.
// Skipped (returns "") when the target resource is already a Pod.
func signalNameFromPodEvents(ctx context.Context, client dynamic.Interface, namespace string, args *CreateRRArgs) string {
	if args.Kind == "Pod" {
		return ""
	}
	podResult, err := HandleListEvents(ctx, client, ListEventsArgs{
		Namespace: namespace,
		Kind:      "Pod",
	})
	if err != nil {
		logr.FromContextOrDiscard(ctx).Error(err, "deriveSignalName failed to list Pod events", "namespace", namespace)
		return ""
	}
	related := FilterRelatedPodEvents(podResult.Events, args.Name)
	return DominantEventReason(related)
}
