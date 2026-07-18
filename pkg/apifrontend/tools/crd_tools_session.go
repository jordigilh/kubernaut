package tools

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/watch"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// ========================================
// kubernaut_await_session: Wait for KA investigation session readiness
// BR-INTERACTIVE-010: AF waits for AA to submit to KA before connecting
// ========================================

// AwaitSessionArgs defines the input for kubernaut_await_session.
type AwaitSessionArgs struct {
	Namespace string `json:"-"`
	RRName    string `json:"rr_name"`
}

// AwaitSessionResult is the output of kubernaut_await_session.
type AwaitSessionResult struct {
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

// AwaitSessionTimeout is the maximum duration HandleAwaitSession waits for an
// AIAnalysis CRD with a session ID. In production the AA controller may take
// minutes to process an RR; in E2E tests this can be shortened.
// Exported so that tests can override it without modifying production code.
var AwaitSessionTimeout = 3 * time.Minute

const awaitSessionPollInterval = 3 * time.Second

// HandleAwaitSession waits for an AIAnalysis resource (matching the given RR) to have
// a non-empty status.investigationSession.id. Returns the session ID when ready, or
// times out after AwaitSessionTimeout.
func HandleAwaitSession(ctx context.Context, client crclient.Client, args AwaitSessionArgs) (AwaitSessionResult, error) {
	if client == nil {
		return AwaitSessionResult{}, ErrK8sUnavailable
	}
	if err := validate.Namespace(args.Namespace); err != nil {
		return AwaitSessionResult{}, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	if args.RRName == "" {
		return AwaitSessionResult{}, fmt.Errorf("%w: rr_name is required", ErrInvalidInput)
	}

	if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
		return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
	}

	watchCtx, cancel := context.WithTimeout(ctx, AwaitSessionTimeout)
	defer cancel()

	wc, ok := client.(crclient.WithWatch)
	if !ok {
		return pollForSessionID(watchCtx, client, args)
	}

	var aiaList aiav1alpha1.AIAnalysisList
	watcher, err := wc.Watch(watchCtx, &aiaList, crclient.InNamespace(args.Namespace))
	if err != nil {
		return pollForSessionID(watchCtx, client, args)
	}
	defer watcher.Stop()

	return watchForSessionID(watchCtx, watcher, args.RRName)
}

// watchForSessionID drains watcher's event channel until an AIAnalysis event
// matching rrName carries a non-empty KASession.ID, the watch closes, or
// watchCtx is done (timeout).
//
//nolint:unparam // error is always nil here; signature matches pollForSessionID's (AwaitSessionResult, error), the interchangeable sibling branch at the shared call site (Issue #1546 Tier 4)
func watchForSessionID(watchCtx context.Context, watcher watch.Interface, rrName string) (AwaitSessionResult, error) {
	for {
		select {
		case <-watchCtx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case evt, ok := <-watcher.ResultChan():
			if !ok {
				return AwaitSessionResult{Status: "timeout", Message: "watch closed unexpectedly"}, nil
			}
			if sessionID, matched := sessionIDFromEvent(evt, rrName); matched {
				return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
			}
		}
	}
}

// sessionIDFromEvent extracts the KA session ID from a watch event, if it is
// an Added/Modified event for the AIAnalysis matching rrName with a
// non-empty session ID already set. matched is true only in that case.
func sessionIDFromEvent(evt watch.Event, rrName string) (sessionID string, matched bool) {
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return "", false
	}
	aia, ok := evt.Object.(*aiav1alpha1.AIAnalysis)
	if !ok || aia.Spec.RemediationRequestRef.Name != rrName {
		return "", false
	}
	if aia.Status.KASession == nil || aia.Status.KASession.ID == "" {
		return "", false
	}
	return aia.Status.KASession.ID, true
}

// pollForSessionID is a fallback that polls AIAnalysis resources until session ID appears.
func pollForSessionID(ctx context.Context, client crclient.Client, args AwaitSessionArgs) (AwaitSessionResult, error) {
	ticker := time.NewTicker(awaitSessionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return AwaitSessionResult{Status: "timeout", Message: "KA session not ready within timeout"}, nil
		case <-ticker.C:
			if sessionID := findSessionIDByList(ctx, client, args); sessionID != "" {
				return AwaitSessionResult{SessionID: sessionID, Status: "ready"}, nil
			}
		}
	}
}

// findSessionIDByList lists AIAnalysis for the given RR and returns the first non-empty session ID.
func findSessionIDByList(ctx context.Context, client crclient.Client, args AwaitSessionArgs) string {
	var list aiav1alpha1.AIAnalysisList
	if err := client.List(ctx, &list, crclient.InNamespace(args.Namespace)); err != nil {
		return ""
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.RemediationRequestRef.Name != args.RRName {
			continue
		}
		if item.Status.KASession != nil && item.Status.KASession.ID != "" {
			return item.Status.KASession.ID
		}
	}
	return ""
}

// NewAwaitSessionTool creates the kubernaut_await_session tool.
func NewAwaitSessionTool(client crclient.Client, controllerNS string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "kubernaut_await_session",
		Description: "Wait for the AI investigation session to become ready for a given remediation request. Returns the KA session ID when available.",
	}, func(ctx tool.Context, args AwaitSessionArgs) (AwaitSessionResult, error) {
		args.Namespace = controllerNS
		return HandleAwaitSession(ctx, client, args)
	})
}

// ========================================
// AwaitISPhaseActive: Poll IS CRD until AA sets Phase=Active
// BR-INTERACTIVE-010: AF waits for AA to acknowledge the interactive session
// ========================================

const (
	isPhaseInitialInterval = 500 * time.Millisecond
	isPhaseMaxInterval     = 2 * time.Second
	isPhaseDefaultTimeout  = 30 * time.Second
)

// AwaitISPhaseActive polls the IS CRD for the given RR name until its phase
// becomes Active (set by the AA controller). Uses exponential backoff starting
// at 500ms, capping at 2s. Returns true when Active is detected, false on
// timeout. Errors from the API are silently retried (best-effort).
// The poll respects the parent context deadline, capping at isPhaseDefaultTimeout.
func AwaitISPhaseActive(ctx context.Context, client crclient.Client, namespace, rrName string) bool {
	if client == nil || namespace == "" || rrName == "" {
		return false
	}

	timeout := isPhaseDefaultTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	interval := isPhaseInitialInterval
	for {
		if isActivePhasePresent(pollCtx, client, namespace, rrName) {
			return true
		}

		select {
		case <-pollCtx.Done():
			return false
		case <-time.After(interval):
		}

		interval *= 2
		if interval > isPhaseMaxInterval {
			interval = isPhaseMaxInterval
		}
	}
}

// isActivePhasePresent lists IS CRDs in the namespace and returns true if any
// non-terminal IS for the given RR has Phase=Active.
func isActivePhasePresent(ctx context.Context, client crclient.Client, namespace, rrName string) bool {
	var list isv1alpha1.InvestigationSessionList
	if err := client.List(ctx, &list, crclient.InNamespace(namespace)); err != nil {
		return false
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.RemediationRequestRef.Name != rrName {
			continue
		}
		if item.Status.Phase == isv1alpha1.SessionPhaseActive {
			return true
		}
	}
	return false
}
