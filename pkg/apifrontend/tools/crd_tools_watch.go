package tools

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/watch"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/go-logr/logr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// WatchArgs defines the input for kubernaut_watch.
type WatchArgs struct {
	Namespace string `json:"-"`
	RRID      string `json:"rr_id,omitempty"`
	Name      string `json:"name,omitempty"`
}

// WatchEvent represents a single status change event.
type WatchEvent struct {
	Timestamp string `json:"timestamp"`
	Resource  string `json:"resource"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
}

// WatchResult is the output of kubernaut_watch.
type WatchResult struct {
	Events  []WatchEvent `json:"events"`
	Status  string       `json:"status"`
	Outcome string       `json:"outcome,omitempty"`
	Message string       `json:"message,omitempty"`
}

// maxWatchDuration is the maximum time HandleWatch will block before returning.
const maxWatchDuration = 15 * time.Minute

// HandleWatch implements the kubernaut_watch logic with progressive SSE
// updates via EventBridge and RAR approval lifecycle tracking.
// typedClient is the controller-runtime client for typed CRD operations (EA);
// may be nil (graceful degradation — EA metadata omitted).
func HandleWatch(ctx context.Context, client crclient.WithWatch, args WatchArgs) (WatchResult, error) {
	if client == nil {
		return WatchResult{}, ErrK8sUnavailable
	}
	if err := normalizeAndValidateWatchArgs(&args); err != nil {
		return WatchResult{}, err
	}

	logger := logr.FromContextOrDiscard(ctx)

	var rrCheck remediationv1.RemediationRequest
	if err := client.Get(ctx, crclient.ObjectKey{Namespace: args.Namespace, Name: args.Name}, &rrCheck); err != nil {
		return WatchResult{}, ToUserFriendlyError(err)
	}

	watchCtx, cancel := context.WithTimeout(ctx, maxWatchDuration)
	defer cancel()

	rrWatcher, err := client.Watch(watchCtx, &remediationv1.RemediationRequestList{},
		crclient.InNamespace(args.Namespace),
		crclient.MatchingFields{"metadata.name": args.Name})
	if err != nil {
		return WatchResult{}, ToUserFriendlyError(err)
	}
	defer rrWatcher.Stop()

	rarName, rarCh, stopRARWatch := setupRARWatch(watchCtx, client, args, logger)
	defer stopRARWatch()

	state := &watchLoopState{startedAt: time.Now().UTC().Format(time.RFC3339)}
	defer state.stopEAWatcher()

	_ = launcher.EmitStatusSafe(ctx, "Watching remediation progress...\n")

	deps := watchDeps{Client: client, Args: args, Logger: logger, RARName: rarName}
	return state.run(ctx, watchCtx, deps, rrWatcher.ResultChan(), rarCh)
}

// watchDeps bundles the request-scoped dependencies shared by the
// watchLoopState methods, keeping their parameter counts within the
// argument-limit lint gate.
type watchDeps struct {
	Client  crclient.WithWatch
	Args    WatchArgs
	Logger  logr.Logger
	RARName string
}

// run drives the watch-loop's event-dispatch select statement until the
// caller's context is cancelled, the RR watch channel closes, or a
// terminal/awaiting-approval RR event is observed.
func (s *watchLoopState) run(ctx, watchCtx context.Context, deps watchDeps, rrCh <-chan watch.Event, rarCh <-chan watch.Event) (WatchResult, error) {
	for {
		select {
		case <-ctx.Done():
			return WatchResult{Events: s.events, Status: "cancelled"}, nil

		case evt, ok := <-rrCh:
			if !ok {
				return WatchResult{Events: s.events, Status: "completed"}, nil
			}
			done, result, herr := s.handleRREvent(ctx, watchCtx, deps, evt)
			if herr != nil {
				return WatchResult{}, herr
			}
			if done {
				return result, nil
			}

		case evt, ok := <-rarCh:
			if !ok {
				rarCh = nil
				continue
			}
			s.handleRAREvent(ctx, evt)

		case evt, ok := <-s.eaCh:
			if !ok {
				s.eaCh = nil
				continue
			}
			s.handleEAEvent(ctx, evt)
		}
	}
}

// normalizeAndValidateWatchArgs resolves args.Namespace/Name from RRID (if
// given) and validates the result in place.
func normalizeAndValidateWatchArgs(args *WatchArgs) error {
	ns, name, err := ParseRRID(args.RRID, args.Namespace, args.Name)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	args.Namespace = ns
	args.Name = name
	if err := validate.Namespace(args.Namespace); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if err := validate.ResourceName(args.Name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	return nil
}

// setupRARWatch starts a best-effort watch on the RemediationApprovalRequest
// named after args.Name. RAR watch failures are non-fatal (logged at V(1))
// since RAR only exists once the RR reaches AwaitingApproval; the returned
// channel is nil and the stop func is a no-op when unavailable.
func setupRARWatch(watchCtx context.Context, client crclient.WithWatch, args WatchArgs, logger logr.Logger) (string, <-chan watch.Event, func()) {
	rarName := fmt.Sprintf("rar-%s", args.Name)
	rarWatcher, rarErr := client.Watch(watchCtx, &remediationv1.RemediationApprovalRequestList{},
		crclient.InNamespace(args.Namespace),
		crclient.MatchingFields{"metadata.name": rarName})
	if rarErr != nil {
		logger.V(1).Info("RAR watch unavailable, continuing with RR-only watch",
			"rar_name", rarName, "error", rarErr)
		return rarName, nil, func() {}
	}
	return rarName, rarWatcher.ResultChan(), rarWatcher.Stop
}

// watchLoopState carries the mutable state shared across watch-loop
// iterations in HandleWatch (GO-ANTIPATTERN-AUDIT-2026-07-01 Phase 4e).
// eaWatcher is stopped via a single deferred call registered once in
// HandleWatch, since the EA watcher is created lazily on first entry into
// the "Verifying" phase (inside handleRREvent) rather than up front.
type watchLoopState struct {
	events             []WatchEvent
	lastSeenPhase      string
	lastRARDecision    string
	startedAt          string
	eaCh               <-chan watch.Event
	eaWatcher          watch.Interface
	prevEA             *eav1alpha1.EffectivenessAssessment
	verifyingStartedAt time.Time
}

// stopEAWatcher stops the lazily-created EA watcher, if one was started.
func (s *watchLoopState) stopEAWatcher() {
	if s.eaWatcher != nil {
		s.eaWatcher.Stop()
	}
}

// handleRREvent processes one RemediationRequest watch event. It returns
// done=true when the watch should terminate (terminal phase or
// awaiting-approval), along with the WatchResult to return to the caller.
func (s *watchLoopState) handleRREvent(ctx, watchCtx context.Context, deps watchDeps, evt watch.Event) (bool, WatchResult, error) {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return false, WatchResult{}, nil
	}
	rrObj, ok := evt.Object.(*remediationv1.RemediationRequest)
	if !ok {
		return false, WatchResult{}, nil
	}
	phase := string(rrObj.Status.OverallPhase)
	if phase == s.lastSeenPhase {
		return false, WatchResult{}, nil
	}
	s.lastSeenPhase = phase
	launcher.UpdatePhaseSafe(ctx, phase)
	msg := fmt.Sprintf("Phase changed to %s", phase)
	s.events = append(s.events, WatchEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Resource:  "RemediationRequest",
		Phase:     phase,
		Message:   msg,
	})
	if phase == "Verifying" {
		s.enterVerifyingPhase(ctx, watchCtx, deps, rrObj, phase)
	} else {
		_ = launcher.EmitStatusSafe(ctx, fmt.Sprintf("Remediation phase: %s\n", phase))
	}

	completedAt := ""
	if IsTerminalPhase(phase) {
		completedAt = time.Now().UTC().Format(time.RFC3339)
	}
	progressMeta := map[string]any{"type": "execution_progress"}
	if phase == "Verifying" {
		eaName := ResolveEAName(rrObj)
		timing := FetchEATimingMetadata(ctx, deps.Client, nil, deps.Args.Namespace, eaName)
		if timing.StabilizationWindow != "" {
			progressMeta["stabilization_window"] = timing.StabilizationWindow
		}
	}
	snapshot := BuildProgressSnapshot(phase, deps.Args.Name, s.startedAt, completedAt)
	_ = launcher.EmitArtifactSafe(ctx, snapshot, fmt.Sprintf("Progress: %s", phase), progressMeta)

	if IsTerminalPhase(phase) {
		return true, WatchResult{Events: s.events, Status: "completed", Outcome: rrObj.Status.Outcome, Message: rrObj.Status.Message}, nil
	}
	if phase == "AwaitingApproval" {
		emitApprovalRequestEvent(ctx, deps)
		return true, WatchResult{Events: s.events, Status: "awaiting_approval"}, nil
	}
	return false, WatchResult{}, nil
}

// enterVerifyingPhase emits the "Verifying" status update (with stabilization
// timing metadata when available) and lazily starts the EA watcher on first
// entry into this phase.
func (s *watchLoopState) enterVerifyingPhase(ctx, watchCtx context.Context, deps watchDeps, rrObj *remediationv1.RemediationRequest, phase string) {
	s.verifyingStartedAt = time.Now().UTC()
	eaName := ResolveEAName(rrObj)
	timing := FetchEATimingMetadata(ctx, deps.Client, nil, deps.Args.Namespace, eaName)

	statusMeta := map[string]any{"type": launcher.MetaTypeStatus}
	if timing.StabilizationWindow != "" {
		statusMeta["stabilization_window"] = timing.StabilizationWindow
		statusMeta["started_at"] = s.verifyingStartedAt.Format(time.RFC3339)
	}
	if timing.ValidityDeadline != "" {
		statusMeta["validity_deadline"] = timing.ValidityDeadline
	}
	_ = launcher.EmitStatusWithMetaSafe(ctx, fmt.Sprintf("Remediation phase: %s\n", phase), statusMeta)

	if s.eaCh != nil {
		return
	}
	eaList := &eav1alpha1.EffectivenessAssessmentList{}
	eaWatcher, eaErr := deps.Client.Watch(watchCtx, eaList,
		crclient.InNamespace(deps.Args.Namespace),
		crclient.MatchingFields{"metadata.name": eaName})
	if eaErr != nil {
		deps.Logger.V(1).Info("EA watch unavailable, verification_step events will not be emitted",
			"ea_name", eaName, "error", eaErr)
		return
	}
	s.eaWatcher = eaWatcher
	s.eaCh = eaWatcher.ResultChan()
}

// emitApprovalRequestEvent fetches the RemediationApprovalRequest named
// rarName and emits its structured payload (plus a resolved payload if
// already decided) for the SSE stream. Best-effort: a GET failure only logs
// and falls back to the text-only phase-change event already emitted by the
// caller.
func emitApprovalRequestEvent(ctx context.Context, deps watchDeps) {
	var rarObj remediationv1.RemediationApprovalRequest
	if getErr := deps.Client.Get(ctx, crclient.ObjectKey{Namespace: deps.Args.Namespace, Name: deps.RARName}, &rarObj); getErr != nil {
		deps.Logger.V(1).Info("RAR GET for structured event failed, continuing with text-only",
			"rar_name", deps.RARName, "error", getErr)
		return
	}
	if payload, mErr := MarshalApprovalRequestPayload(&rarObj); mErr == nil {
		_ = launcher.EmitStructuredMetaSafe(ctx, payload, map[string]any{"type": launcher.MetaTypeApprovalRequest})
	}
	if rarObj.Status.Decision == "" {
		return
	}
	if resolved, mErr := MarshalApprovalResolvedPayload(&rarObj); mErr == nil {
		_ = launcher.EmitStructuredMetaSafe(ctx, resolved, map[string]any{"type": launcher.MetaTypeApprovalRequestResolved})
	}
}

// handleRAREvent processes one RemediationApprovalRequest watch event.
func (s *watchLoopState) handleRAREvent(ctx context.Context, evt watch.Event) {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return
	}
	rarObj, ok := evt.Object.(*remediationv1.RemediationApprovalRequest)
	if !ok {
		return
	}
	decision := string(rarObj.Status.Decision)
	if decision == s.lastRARDecision {
		return
	}
	s.lastRARDecision = decision

	var rarMsg string
	switch decision {
	case "":
		rarMsg = "Approval requested — awaiting human decision"
	case "Approved":
		if rarObj.Status.DecidedBy != "" {
			rarMsg = fmt.Sprintf("Approval granted by %s", rarObj.Status.DecidedBy)
		} else {
			rarMsg = "Approval granted"
		}
	case "Rejected":
		if rarObj.Status.DecidedBy != "" {
			rarMsg = fmt.Sprintf("Approval rejected by %s", rarObj.Status.DecidedBy)
		} else {
			rarMsg = "Approval rejected"
		}
	case "Expired":
		rarMsg = "Approval expired"
	default:
		rarMsg = fmt.Sprintf("Approval status: %s", decision)
	}

	s.events = append(s.events, WatchEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Resource:  "RemediationApprovalRequest",
		Phase:     decision,
		Message:   rarMsg,
	})
	_ = launcher.EmitStatusSafe(ctx, rarMsg+"\n")
	if resolved, mErr := MarshalApprovalResolvedPayload(rarObj); mErr == nil {
		_ = launcher.EmitStructuredMetaSafe(ctx, resolved, map[string]any{"type": launcher.MetaTypeApprovalRequestResolved})
	}
}

// handleEAEvent processes one EffectivenessAssessment watch event, emitting
// a verification_step event for each step diff since the previous snapshot.
func (s *watchLoopState) handleEAEvent(ctx context.Context, evt watch.Event) {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return
	}
	currEA, ok := evt.Object.(*eav1alpha1.EffectivenessAssessment)
	if !ok {
		return
	}
	steps := DiffEASteps(s.prevEA, currEA)
	for _, step := range steps {
		stepMeta := map[string]any{
			"type": launcher.MetaTypeVerificationStep,
			"step": step.Step,
		}
		for k, v := range step.Data {
			stepMeta[k] = v
		}
		if !s.verifyingStartedAt.IsZero() {
			stepMeta["elapsed_s"] = int(time.Since(s.verifyingStartedAt).Seconds())
		}
		_ = launcher.EmitStatusWithMetaSafe(ctx, step.Message+"\n", stepMeta)
		s.events = append(s.events, WatchEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Resource:  "EffectivenessAssessment",
			Phase:     step.Step,
			Message:   step.Message,
		})
	}
	s.prevEA = currEA.DeepCopy()
}

// NewWatchTool creates the kubernaut_watch tool.
func NewWatchTool(client crclient.WithWatch, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_watch",
		Description: "Stream live status updates for a remediation and its related resources",
	}, func(ctx tool.Context, args WatchArgs) (WatchResult, error) {
		args.Namespace = controllerNS
		return HandleWatch(ctx, client, args)
	})
}
