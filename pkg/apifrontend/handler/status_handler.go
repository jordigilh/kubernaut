package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

const defaultHeartbeatInterval = 15 * time.Second

// StatusHandler serves the POST /a2a/status endpoint (DD-AF-008).
type StatusHandler struct {
	client            crclient.WithWatch
	namespace         string
	logger            logr.Logger
	heartbeatInterval time.Duration
}

// NewStatusHandler constructs a StatusHandler with the default 15s heartbeat.
func NewStatusHandler(client crclient.WithWatch, namespace string, logger logr.Logger) *StatusHandler {
	return newStatusHandler(client, namespace, logger, defaultHeartbeatInterval)
}

// NewStatusHandlerForTest constructs a StatusHandler with a custom heartbeat
// interval. Production code should use NewStatusHandler.
func NewStatusHandlerForTest(client crclient.WithWatch, namespace string, logger logr.Logger, heartbeat time.Duration) *StatusHandler {
	return newStatusHandler(client, namespace, logger, heartbeat)
}

func newStatusHandler(client crclient.WithWatch, namespace string, logger logr.Logger, heartbeat time.Duration) *StatusHandler {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &StatusHandler{
		client:            client,
		namespace:         namespace,
		logger:            logger.WithName("status-handler"),
		heartbeatInterval: heartbeat,
	}
}

func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, rpcErr := parseStatusSubscribeRequest(r)
	if rpcErr != nil {
		writeJSONRPCError(w, nil, rpcErr.Code, rpcErr.Message)
		return
	}

	h.handleSubscribe(w, r, req)
}

// subscribeStreamState holds the mutable state threaded through the SSE
// event loop for a single status/subscribe connection (RR + EA watch
// progress, diff baseline, and terminal-phase tracking).
type subscribeStreamState struct {
	ea                *eav1alpha1.EffectivenessAssessment
	prevEA            *eav1alpha1.EffectivenessAssessment
	rrWatcher         watch.Interface
	eaWatcher         watch.Interface
	eaCh              <-chan watch.Event
	rrResourceVersion string
	eaResourceVersion string
	lastSeenPhase     string
	isFinal           bool
}

// subscribeCleanup accumulates resource-cleanup closures (watch.Interface.Stop,
// time.Ticker/Timer.Stop) registered while setting up or running the subscribe
// stream, and runs them in LIFO order when the connection ends — mirroring Go's
// own `defer` semantics. This indirection exists because the stream setup and
// event loop are split across several methods, so a literal `defer` in each
// cannot reach the single point (handleSubscribe's return) where all of them
// must fire together.
type subscribeCleanup struct {
	fns []func()
}

func (c *subscribeCleanup) add(fn func()) {
	if fn != nil {
		c.fns = append(c.fns, fn)
	}
}

func (c *subscribeCleanup) run() {
	for i := len(c.fns) - 1; i >= 0; i-- {
		c.fns[i]()
	}
}

// subscribeLoopCtx groups the request-scoped, immutable-for-the-life-of-the-
// connection dependencies threaded through the SSE event loop. Extracted
// per AGENTS.md's 8+-param Options-pattern rule (GO-ANTIPATTERN-AUDIT-2026-07-01
// Phase 4h) so runSubscribeLoop/handleRRWatchEvent/handleEAWatchEvent stay
// under the 7-argument limit. Mutable per-iteration state lives separately
// in subscribeStreamState.
type subscribeLoopCtx struct {
	W       http.ResponseWriter
	Flusher http.Flusher
	Req     *StatusSubscribeRequest
	RRList  *remediationv1.RemediationRequestList
	Key     crclient.ObjectKey
	Logger  logr.Logger
	Cleanup *subscribeCleanup
}

func (h *StatusHandler) handleSubscribe(w http.ResponseWriter, r *http.Request, req *StatusSubscribeRequest) {
	ctx := r.Context()
	logger := h.logger.WithValues("rr_id", req.Params.RRID)

	if h.client == nil {
		logger.Info("no K8s client configured")
		http.Error(w, "no K8s client configured", http.StatusServiceUnavailable)
		return
	}

	var rr remediationv1.RemediationRequest
	key := crclient.ObjectKey{Namespace: h.namespace, Name: req.Params.RRID}
	if err := h.client.Get(ctx, key, &rr); err != nil {
		if apierrors.IsNotFound(err) {
			writeJSONRPCError(w, req.ID, errCodeRRNotFound, "rr_not_found")
			return
		}
		logger.Error(err, "failed to get RR")
		writeJSONRPCError(w, req.ID, errCodeRRNotFound, "rr_not_found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	writeSSEHeaders(w)

	var cleanup subscribeCleanup
	defer cleanup.run()

	st := h.initSubscribeState(ctx, &rr)
	h.writeStatusUpdate(w, flusher, req.Params.RRID, string(rr.Status.OverallPhase), st.isFinal, BuildPhaseMetadata(&rr, st.ea))
	if st.isFinal {
		return
	}

	rrList := &remediationv1.RemediationRequestList{}
	var err error
	st.rrWatcher, err = h.watchRR(ctx, rrList, req.Params.RRID)
	if err != nil {
		logger.Error(err, "failed to start RR watch")
		return
	}
	cleanup.add(st.rrWatcher.Stop)

	heartbeat := time.NewTicker(h.heartbeatInterval)
	cleanup.add(heartbeat.Stop)

	h.startInitialEAWatch(ctx, &rr, st, &cleanup)

	deadlineTimer := h.deadlineTimerChan(ctx, &cleanup)

	lc := &subscribeLoopCtx{
		W:       w,
		Flusher: flusher,
		Req:     req,
		RRList:  rrList,
		Key:     key,
		Logger:  logger,
		Cleanup: &cleanup,
	}
	h.runSubscribeLoop(ctx, lc, st, heartbeat.C, deadlineTimer)
}

// writeSSEHeaders sets the response headers required for a chunked
// Server-Sent Events stream.
func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

// initSubscribeState computes the initial phase/terminal status for the
// stream and, if the RR is already Verifying, fetches the EA snapshot used
// for the very first status/update write.
func (h *StatusHandler) initSubscribeState(ctx context.Context, rr *remediationv1.RemediationRequest) *subscribeStreamState {
	phase := string(rr.Status.OverallPhase)
	st := &subscribeStreamState{
		lastSeenPhase:     phase,
		rrResourceVersion: rr.ResourceVersion,
		isFinal:           tools.IsTerminalPhase(phase),
	}
	if phase == "Verifying" {
		st.ea = h.fetchEA(ctx, rr)
	}
	return st
}

// startInitialEAWatch begins watching the EA once the stream is confirmed to
// continue past the initial (possibly terminal) status update, seeding the
// diff baseline from the EA snapshot captured in initSubscribeState.
func (h *StatusHandler) startInitialEAWatch(ctx context.Context, rr *remediationv1.RemediationRequest, st *subscribeStreamState, cleanup *subscribeCleanup) {
	if string(rr.Status.OverallPhase) != "Verifying" || st.ea == nil {
		return
	}
	st.prevEA = st.ea.DeepCopy()
	st.eaResourceVersion = st.ea.ResourceVersion
	st.eaWatcher, st.eaCh = h.startEAWatch(ctx, tools.ResolveEAName(rr))
	if st.eaWatcher != nil {
		cleanup.add(st.eaWatcher.Stop)
	}
}

// watchRR starts a watch scoped to the single named RR.
func (h *StatusHandler) watchRR(ctx context.Context, rrList *remediationv1.RemediationRequestList, rrID string) (watch.Interface, error) {
	return h.client.Watch(ctx, rrList,
		crclient.InNamespace(h.namespace),
		crclient.MatchingFields{"metadata.name": rrID})
}

// deadlineTimerChan returns a channel that fires 5s before the request
// context's deadline (if any), giving clients a chance to reconnect before
// the transport forcibly closes the connection. Returns nil if there is no
// deadline or fewer than 5s remain.
func (h *StatusHandler) deadlineTimerChan(ctx context.Context, cleanup *subscribeCleanup) <-chan time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}
	preWarning := time.Until(deadline) - 5*time.Second
	if preWarning <= 0 {
		return nil
	}
	timer := time.NewTimer(preWarning)
	cleanup.add(func() { timer.Stop() })
	return timer.C
}

// runSubscribeLoop drives the SSE event loop: heartbeats, deadline
// pre-warning, RR phase-change events, and (once Verifying) EA
// verification-step events. Returns when the context is done or a terminal
// phase update has been sent.
func (h *StatusHandler) runSubscribeLoop(
	ctx context.Context,
	lc *subscribeLoopCtx,
	st *subscribeStreamState,
	heartbeatC <-chan time.Time,
	deadlineTimer <-chan time.Time,
) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-heartbeatC:
			_, _ = lc.W.Write(streaming.HeartbeatFrame())
			lc.Flusher.Flush()

		case <-deadlineTimer:
			h.writeStatusClosing(lc.W, lc.Flusher, "token_expiry", true)
			deadlineTimer = nil

		case evt, ok := <-st.rrWatcher.ResultChan():
			if !h.handleRRWatchEvent(ctx, lc, st, evt, ok) {
				return
			}

		case eaEvt, ok := <-st.eaCh:
			h.handleEAWatchEvent(ctx, lc, st, eaEvt, ok)
		}
	}
}

// handleRRWatchEvent processes one event from the RR watch: reconnects on
// channel close, ignores non-modification events and unchanged phases, and
// on a phase change writes a status/update (bootstrapping the EA watch on
// first entry into Verifying). Returns false if the loop should return
// (reconnect failure or terminal phase reached).
func (h *StatusHandler) handleRRWatchEvent(
	ctx context.Context,
	lc *subscribeLoopCtx,
	st *subscribeStreamState,
	evt watch.Event,
	ok bool,
) bool {
	if !ok {
		lc.Logger.V(1).Info("RR watch closed, reconnecting", "resourceVersion", st.rrResourceVersion)
		st.rrWatcher.Stop()
		newWatcher, err := h.watchRR(ctx, lc.RRList, lc.Req.Params.RRID)
		if err != nil {
			lc.Logger.Error(err, "failed to reconnect RR watch")
			return false
		}
		st.rrWatcher = newWatcher
		return true
	}
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return true
	}
	rrObj, ok := evt.Object.(*remediationv1.RemediationRequest)
	if !ok {
		return true
	}
	st.rrResourceVersion = rrObj.ResourceVersion
	newPhase := string(rrObj.Status.OverallPhase)
	if newPhase == st.lastSeenPhase {
		return true
	}
	st.lastSeenPhase = newPhase

	if newPhase == "Verifying" && st.eaCh == nil {
		h.bootstrapEAWatch(ctx, rrObj, st, lc.Cleanup)
	}

	st.isFinal = tools.IsTerminalPhase(newPhase)
	meta := BuildPhaseMetadata(rrObj, st.ea)
	h.writeStatusUpdate(lc.W, lc.Flusher, lc.Req.Params.RRID, newPhase, st.isFinal, meta)

	return !st.isFinal
}

// bootstrapEAWatch starts the EA watch when the RR first transitions into
// the Verifying phase mid-stream, seeding the diff baseline from the current
// EA (if any) and replacing any stale EA watcher.
func (h *StatusHandler) bootstrapEAWatch(ctx context.Context, rrObj *remediationv1.RemediationRequest, st *subscribeStreamState, cleanup *subscribeCleanup) {
	st.ea = h.fetchEA(ctx, rrObj)
	if st.ea != nil {
		st.prevEA = st.ea.DeepCopy()
		st.eaResourceVersion = st.ea.ResourceVersion
	}
	if st.eaWatcher != nil {
		st.eaWatcher.Stop()
	}
	st.eaWatcher, st.eaCh = h.startEAWatch(ctx, tools.ResolveEAName(rrObj))
	if st.eaWatcher != nil {
		cleanup.add(st.eaWatcher.Stop)
	}
}

// handleEAWatchEvent processes one event from the EA watch: reconnects (by
// re-fetching the RR to resolve the current EA name) on channel close,
// ignores non-modification events, and on a change computes and streams the
// verification-step diff against the previous EA snapshot.
func (h *StatusHandler) handleEAWatchEvent(
	ctx context.Context,
	lc *subscribeLoopCtx,
	st *subscribeStreamState,
	evt watch.Event,
	ok bool,
) {
	if !ok {
		lc.Logger.V(1).Info("EA watch closed, reconnecting", "resourceVersion", st.eaResourceVersion)
		st.eaCh = nil
		if st.eaWatcher != nil {
			st.eaWatcher.Stop()
		}
		var rr remediationv1.RemediationRequest
		if err := h.client.Get(ctx, lc.Key, &rr); err == nil {
			st.eaWatcher, st.eaCh = h.startEAWatch(ctx, tools.ResolveEAName(&rr))
			if st.eaWatcher != nil {
				lc.Cleanup.add(st.eaWatcher.Stop)
			}
		}
		return
	}
	if evt.Type != watch.Modified && evt.Type != watch.Added {
		return
	}
	eaObj, ok := evt.Object.(*eav1alpha1.EffectivenessAssessment)
	if !ok {
		return
	}
	st.eaResourceVersion = eaObj.ResourceVersion

	steps := tools.DiffEASteps(st.prevEA, eaObj)
	st.prevEA = eaObj.DeepCopy()
	st.ea = eaObj

	if len(steps) == 0 {
		return
	}

	var rr remediationv1.RemediationRequest
	if err := h.client.Get(ctx, lc.Key, &rr); err != nil {
		lc.Logger.V(1).Info("failed to refresh RR for EA event", "error", err)
		return
	}
	meta := BuildPhaseMetadata(&rr, st.ea)
	meta["verification_steps"] = steps
	h.writeStatusUpdate(lc.W, lc.Flusher, lc.Req.Params.RRID, string(rr.Status.OverallPhase), false, meta)
}

func (h *StatusHandler) startEAWatch(ctx context.Context, eaName string) (watch.Interface, <-chan watch.Event) {
	eaList := &eav1alpha1.EffectivenessAssessmentList{}
	watcher, err := h.client.Watch(ctx, eaList,
		crclient.InNamespace(h.namespace),
		crclient.MatchingFields{"metadata.name": eaName})
	if err != nil {
		h.logger.V(1).Info("EA watch unavailable", "ea_name", eaName, "error", err)
		return nil, nil
	}
	return watcher, watcher.ResultChan()
}

func (h *StatusHandler) fetchEA(ctx context.Context, rr *remediationv1.RemediationRequest) *eav1alpha1.EffectivenessAssessment {
	eaName := tools.ResolveEAName(rr)
	var ea eav1alpha1.EffectivenessAssessment
	if err := h.client.Get(ctx, crclient.ObjectKey{Namespace: h.namespace, Name: eaName}, &ea); err != nil {
		return nil
	}
	return &ea
}

func (h *StatusHandler) writeStatusUpdate(w http.ResponseWriter, flusher http.Flusher, rrID, phase string, final bool, metadata map[string]any) {
	params := StatusUpdateParams{
		RRID:      rrID,
		Phase:     phase,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Final:     final,
		Metadata:  metadata,
	}
	envelope := map[string]any{
		"jsonrpc": "2.0",
		"method":  "status/update",
		"params":  params,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		h.logger.Error(err, "failed to marshal status/update")
		return
	}
	if _, err = fmt.Fprintf(w, "event: status/update\ndata: %s\n\n", data); err != nil {
		h.logger.V(1).Info("failed to write status/update frame", "error", err)
	}
	flusher.Flush()
}

func (h *StatusHandler) writeStatusClosing(w http.ResponseWriter, flusher http.Flusher, reason string, reconnect bool) {
	params := StatusClosingParams{
		Reason:    reason,
		Reconnect: reconnect,
	}
	envelope := map[string]any{
		"jsonrpc": "2.0",
		"method":  "status/closing",
		"params":  params,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		h.logger.Error(err, "failed to marshal status/closing")
		return
	}
	if _, err = fmt.Fprintf(w, "event: status/closing\ndata: %s\n\n", data); err != nil {
		h.logger.V(1).Info("failed to write status/closing frame", "error", err)
	}
	flusher.Flush()
}

func parseStatusSubscribeRequest(r *http.Request) (*StatusSubscribeRequest, *jsonRPCError) {
	var req StatusSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, &jsonRPCError{Code: errCodeInvalidRequest, Message: "invalid_request"}
	}

	if req.Method != "status/subscribe" {
		return nil, &jsonRPCError{Code: errCodeMethodNotFound, Message: "method_not_found"}
	}

	if req.Params.RRID == "" {
		return nil, &jsonRPCError{Code: errCodeInvalidParams, Message: "invalid_params"}
	}

	return &req, nil
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, message string) {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode error response: %v", err), http.StatusInternalServerError)
	}
}
