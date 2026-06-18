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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	phase := string(rr.Status.OverallPhase)
	isFinal := tools.IsTerminalPhase(phase)

	var ea *eav1alpha1.EffectivenessAssessment
	if phase == "Verifying" {
		ea = h.fetchEA(ctx, &rr)
	}

	h.writeStatusUpdate(w, flusher, req.Params.RRID, phase, isFinal, BuildPhaseMetadata(&rr, ea))

	if isFinal {
		return
	}

	rrResourceVersion := rr.ResourceVersion

	rrList := &remediationv1.RemediationRequestList{}
	rrWatcher, err := h.client.Watch(ctx, rrList,
		crclient.InNamespace(h.namespace),
		crclient.MatchingFields{"metadata.name": req.Params.RRID})
	if err != nil {
		logger.Error(err, "failed to start RR watch")
		return
	}
	defer rrWatcher.Stop()

	heartbeat := time.NewTicker(h.heartbeatInterval)
	defer heartbeat.Stop()

	var eaCh <-chan watch.Event
	var eaWatcher watch.Interface
	var prevEA *eav1alpha1.EffectivenessAssessment
	var eaResourceVersion string
	lastSeenPhase := phase

	if phase == "Verifying" && ea != nil {
		prevEA = ea.DeepCopy()
		eaResourceVersion = ea.ResourceVersion
		eaName := tools.ResolveEAName(&rr)
		eaWatcher, eaCh = h.startEAWatch(ctx, eaName)
		if eaWatcher != nil {
			defer eaWatcher.Stop()
		}
	}

	var deadlineTimer <-chan time.Time
	if deadline, ok := ctx.Deadline(); ok {
		preWarning := time.Until(deadline) - 5*time.Second
		if preWarning > 0 {
			timer := time.NewTimer(preWarning)
			defer timer.Stop()
			deadlineTimer = timer.C
		}
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-heartbeat.C:
			_, _ = w.Write(streaming.HeartbeatFrame())
			flusher.Flush()

		case <-deadlineTimer:
			h.writeStatusClosing(w, flusher, "token_expiry", true)
			deadlineTimer = nil

		case evt, ok := <-rrWatcher.ResultChan():
			if !ok {
				logger.V(1).Info("RR watch closed, reconnecting", "resourceVersion", rrResourceVersion)
				rrWatcher.Stop()
				rrWatcher, err = h.client.Watch(ctx, rrList,
					crclient.InNamespace(h.namespace),
					crclient.MatchingFields{"metadata.name": req.Params.RRID})
				if err != nil {
					logger.Error(err, "failed to reconnect RR watch")
					return
				}
				continue
			}
			if evt.Type != watch.Modified && evt.Type != watch.Added {
				continue
			}
			rrObj, ok := evt.Object.(*remediationv1.RemediationRequest)
			if !ok {
				continue
			}
			rrResourceVersion = rrObj.ResourceVersion
			newPhase := string(rrObj.Status.OverallPhase)
			if newPhase == lastSeenPhase {
				continue
			}
			lastSeenPhase = newPhase

			if newPhase == "Verifying" && eaCh == nil {
				ea = h.fetchEA(ctx, rrObj)
				if ea != nil {
					prevEA = ea.DeepCopy()
					eaResourceVersion = ea.ResourceVersion
				}
				eaName := tools.ResolveEAName(rrObj)
				if eaWatcher != nil {
					eaWatcher.Stop()
				}
				eaWatcher, eaCh = h.startEAWatch(ctx, eaName)
				if eaWatcher != nil {
					defer eaWatcher.Stop()
				}
			}

			isFinal = tools.IsTerminalPhase(newPhase)
			meta := BuildPhaseMetadata(rrObj, ea)
			h.writeStatusUpdate(w, flusher, req.Params.RRID, newPhase, isFinal, meta)

			if isFinal {
				return
			}

		case eaEvt, ok := <-eaCh:
			if !ok {
				logger.V(1).Info("EA watch closed, reconnecting", "resourceVersion", eaResourceVersion)
				eaCh = nil
				if eaWatcher != nil {
					eaWatcher.Stop()
				}
				if err := h.client.Get(ctx, key, &rr); err == nil {
					eaName := tools.ResolveEAName(&rr)
					eaWatcher, eaCh = h.startEAWatch(ctx, eaName)
					if eaWatcher != nil {
						defer eaWatcher.Stop()
					}
				}
				continue
			}
			if eaEvt.Type != watch.Modified && eaEvt.Type != watch.Added {
				continue
			}
			eaObj, ok := eaEvt.Object.(*eav1alpha1.EffectivenessAssessment)
			if !ok {
				continue
			}
			eaResourceVersion = eaObj.ResourceVersion

			steps := tools.DiffEASteps(prevEA, eaObj)
			prevEA = eaObj.DeepCopy()
			ea = eaObj

			if len(steps) == 0 {
				continue
			}

			if err := h.client.Get(ctx, key, &rr); err != nil {
				logger.V(1).Info("failed to refresh RR for EA event", "error", err)
				continue
			}
			meta := BuildPhaseMetadata(&rr, ea)
			meta["verification_steps"] = steps
			h.writeStatusUpdate(w, flusher, req.Params.RRID, string(rr.Status.OverallPhase), false, meta)
		}
	}
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
	fmt.Fprintf(w, "event: status/update\ndata: %s\n\n", data)
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
	fmt.Fprintf(w, "event: status/closing\ndata: %s\n\n", data)
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
