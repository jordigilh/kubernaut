package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// AgentExecutor defines the interface between the ACP server and the
// underlying agent runtime (open-agent-sdk-go in our case).
type AgentExecutor interface {
	// Manifest returns the ACP agent manifest for the named agent.
	Manifest(name string) (*AgentManifest, error)

	// ExecuteSync runs the agent synchronously and returns the completed run.
	ExecuteSync(ctx context.Context, req RunCreateRequest) (*Run, error)

	// ExecuteStream runs the agent and streams events to the channel.
	// The channel is closed when the run completes or fails.
	ExecuteStream(ctx context.Context, req RunCreateRequest) (<-chan Event, error)

	// Resume resumes an awaiting run with new input.
	Resume(ctx context.Context, runID string, req RunResumeRequest) (*Run, error)

	// Cancel cancels a running or awaiting run.
	Cancel(ctx context.Context, runID string) error

	// GetRun returns the current state of a run.
	GetRun(ctx context.Context, runID string) (*Run, error)
}

// Server implements the ACP v0.2.0 REST+SSE HTTP API.
type Server struct {
	executor AgentExecutor
	logger   *slog.Logger

	mu   sync.RWMutex
	runs map[string]*Run
}

func NewServer(executor AgentExecutor, logger *slog.Logger) *Server {
	return &Server{
		executor: executor,
		logger:   logger,
		runs:     make(map[string]*Run),
	}
}

// RegisterRoutes attaches ACP endpoints to a ServeMux.
// Compatible with any mux that supports Handle/HandleFunc.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /agents/{name}", s.handleGetAgent)
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("GET /runs/{run_id}", s.handleGetRun)
	mux.HandleFunc("POST /runs/{run_id}", s.handleResumeRun)
	mux.HandleFunc("DELETE /runs/{run_id}", s.handleCancelRun)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "agent name is required")
		return
	}

	manifest, err := s.executor.Manifest(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("agent %q not found", name))
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req RunCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid request body: "+err.Error())
		return
	}

	if req.AgentName == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "agent_name is required")
		return
	}
	if len(req.Input) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_input", "input messages are required")
		return
	}

	if req.Mode == "" {
		req.Mode = RunModeSync
	}

	ctx := r.Context()

	switch req.Mode {
	case RunModeStream:
		s.handleStreamRun(ctx, w, req)
	case RunModeAsync:
		s.handleAsyncRun(ctx, w, req)
	default:
		s.handleSyncRun(ctx, w, req)
	}
}

func (s *Server) handleSyncRun(ctx context.Context, w http.ResponseWriter, req RunCreateRequest) {
	run, err := s.executor.ExecuteSync(ctx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	s.storeRun(run)
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleAsyncRun(ctx context.Context, w http.ResponseWriter, req RunCreateRequest) {
	runID := uuid.New().String()
	run := &Run{
		AgentName: req.AgentName,
		RunID:     runID,
		Status:    RunStatusCreated,
		Output:    []Message{},
	}

	s.storeRun(run)

	go func() {
		result, err := s.executor.ExecuteSync(context.Background(), req)
		if err != nil {
			s.logger.Error("async run failed", "run_id", runID, "error", err)
			result = &Run{
				AgentName: req.AgentName,
				RunID:     runID,
				Status:    RunStatusFailed,
				Output:    []Message{},
				Error:     &Error{Code: "server_error", Message: err.Error()},
			}
		}
		result.RunID = runID
		s.storeRun(result)
	}()

	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleStreamRun(ctx context.Context, w http.ResponseWriter, req RunCreateRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	events, err := s.executor.ExecuteStream(ctx, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			s.logger.Error("failed to marshal event", "error", err)
			continue
		}

		eventType := string(event.Type)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
		flusher.Flush()

		if event.Run != nil {
			s.storeRun(event.Run)
		}
	}
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "run_id is required")
		return
	}

	run, err := s.executor.GetRun(r.Context(), runID)
	if err != nil {
		s.mu.RLock()
		stored, ok := s.runs[runID]
		s.mu.RUnlock()
		if ok {
			writeJSON(w, http.StatusOK, stored)
			return
		}
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("run %q not found", runID))
		return
	}

	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleResumeRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "run_id is required")
		return
	}

	var req RunResumeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input", "invalid request body: "+err.Error())
		return
	}
	req.RunID = runID

	run, err := s.executor.Resume(r.Context(), runID, req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		}
		return
	}

	s.storeRun(run)
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_input", "run_id is required")
		return
	}

	if err := s.executor.Cancel(r.Context(), runID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "server_error", err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) storeRun(run *Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.RunID] = run
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, Error{Code: code, Message: message})
}
