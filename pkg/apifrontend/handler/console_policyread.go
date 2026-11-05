package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/httputil"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/requestid"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

const (
	consolePolicyScopePath   = "/api/v1/console/policies/scope"
	consoleInventoryListPath = "/api/v1/console/inventory/managed-resources"
)

// ConsolePolicyReadHandler serves the authenticated, same-origin Console
// policy and managed-resource projection routes.
type ConsolePolicyReadHandler struct {
	service *policyread.Service
	logger  logr.Logger
}

// NewConsolePolicyReadHandler constructs the three Console read operations.
func NewConsolePolicyReadHandler(service *policyread.Service, logger logr.Logger) http.Handler {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	return &ConsolePolicyReadHandler{service: service, logger: logger.WithName("console-policy-read")}
}

func (h *ConsolePolicyReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	user := auth.UserIdentityFromContext(r.Context())
	if user == nil {
		h.writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "authentication required")
		return
	}
	if h.service == nil {
		h.writeState(w, r, inventory.StateUnavailable)
		return
	}

	eventType := consoleReadEventType(r.URL.Path)
	filters := inventory.Filters{}
	resourceID := r.PathValue("resourceID")
	state := inventory.StateUnavailable
	ctx := policyread.WithAuditBoundary(r.Context())
	ctx = policyread.WithRequestID(ctx, requestid.FromContext(r.Context()))
	defer func() {
		if eventType != "" {
			h.service.EmitReadAudit(ctx, user, eventType, filters, resourceID, state)
		}
	}()

	switch {
	case r.URL.Path == consolePolicyScopePath:
		var err error
		filters, err = parseConsoleFilters(r, false)
		if err != nil {
			h.writeInvalidFilter(w, r)
			return
		}
		result, err := h.service.ListPolicyScope(ctx, user, filters)
		if err != nil {
			state = unavailableState(err)
			h.writeServiceError(w, r, err)
			return
		}
		state = h.writeJSON(w, r, http.StatusOK, result, result.State)
	case r.URL.Path == consoleInventoryListPath:
		var err error
		filters, err = parseConsoleFilters(r, false)
		if err != nil {
			h.writeInvalidFilter(w, r)
			return
		}
		result, err := h.service.ListInventory(ctx, user, filters)
		if err != nil {
			state = unavailableState(err)
			h.writeServiceError(w, r, err)
			return
		}
		state = h.writeJSON(w, r, http.StatusOK, struct {
			State        inventory.State             `json:"state"`
			Completeness inventory.CompletenessState `json:"completeness"`
			Items        []inventory.Resource        `json:"items,omitempty"`
			NextCursor   string                      `json:"next_cursor,omitempty"`
		}{
			State:        result.State,
			Completeness: result.Completeness,
			Items:        result.Items,
			NextCursor:   result.NextCursor,
		}, result.State)
	case r.PathValue("resourceID") != "":
		var err error
		filters, err = parseConsoleFilters(r, true)
		if err != nil {
			h.writeInvalidFilter(w, r)
			return
		}
		result, err := h.service.GetInventory(ctx, user, filters, r.PathValue("resourceID"))
		if err != nil {
			state = unavailableState(err)
			h.writeServiceError(w, r, err)
			return
		}
		state = h.writeJSON(w, r, http.StatusOK, result, resultState(result))
	default:
		h.writeProblem(w, r, http.StatusNotFound, "Not Found", "Console read route not found")
	}
}

func parseConsoleFilters(r *http.Request, exact bool) (inventory.Filters, error) {
	values := r.URL.Query()
	allowed := map[string]bool{
		"cluster_id": true, "namespace": true, "group": true, "version": true,
		"kind": true, "name": true, "resource_id": true, "limit": true, "cursor": true,
	}
	if exact {
		for key := range values {
			if key != "cluster_id" {
				return inventory.Filters{}, fmt.Errorf("%w: exact resource reads accept only cluster_id", inventory.ErrInvalidFilter)
			}
		}
	}
	for key, items := range values {
		if !allowed[key] || len(items) != 1 {
			return inventory.Filters{}, fmt.Errorf("%w: unsupported or duplicated query parameter", inventory.ErrInvalidFilter)
		}
	}
	get := func(key string) string { return values.Get(key) }
	filters := inventory.Filters{
		ClusterID:  get("cluster_id"),
		Namespace:  get("namespace"),
		Group:      get("group"),
		Version:    get("version"),
		Kind:       get("kind"),
		Name:       get("name"),
		ResourceID: get("resource_id"),
		Cursor:     get("cursor"),
	}
	if rawLimit := get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return inventory.Filters{}, fmt.Errorf("%w: limit must be an integer", inventory.ErrInvalidFilter)
		}
		filters.Limit = limit
	}
	return filters, nil
}

func (h *ConsolePolicyReadHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, inventory.ErrInvalidFilter) {
		h.writeInvalidFilter(w, r)
		return
	}
	state := policyread.StateOf(err)
	if state == "" {
		state = inventory.StateUnavailable
	}
	status := projectionStatus(state)
	message := projectionMessage(state)
	h.writeJSON(w, r, status, consoleStateEnvelope{
		State: state, RequestID: requestid.FromContext(r.Context()), Message: message,
	}, state)
	if status >= http.StatusInternalServerError {
		h.logger.Error(err, "Console projection read failed", "state", state)
	}
}

func projectionStatus(state inventory.State) int {
	switch state {
	case inventory.StateForbidden:
		return http.StatusForbidden
	case inventory.StateNotFound:
		return http.StatusNotFound
	default:
		return http.StatusServiceUnavailable
	}
}

func projectionMessage(state inventory.State) string {
	switch state {
	case inventory.StateForbidden:
		return "access forbidden"
	case inventory.StateNotFound:
		return "resource not found"
	default:
		return "Console projection unavailable"
	}
}

func (h *ConsolePolicyReadHandler) writeInvalidFilter(w http.ResponseWriter, r *http.Request) {
	h.writeProblem(w, r, http.StatusBadRequest, "Invalid request", "invalid Console read filter")
}

func (h *ConsolePolicyReadHandler) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpProblem := &httputil.ProblemDetail{
		Type:      "about:blank",
		Title:     title,
		Status:    status,
		Detail:    detail,
		RequestID: requestid.FromContext(r.Context()),
	}
	httputil.WriteProblemFull(w, httpProblem)
}

func (h *ConsolePolicyReadHandler) writeState(w http.ResponseWriter, r *http.Request, state inventory.State) {
	_ = h.writeJSON(w, r, projectionStatus(state), consoleStateEnvelope{
		State: state, RequestID: requestid.FromContext(r.Context()), Message: projectionMessage(state),
	}, state)
}

type consoleStateEnvelope struct {
	State     inventory.State `json:"state"`
	RequestID string          `json:"request_id,omitempty"`
	Message   string          `json:"message"`
}

func (h *ConsolePolicyReadHandler) writeJSON(w http.ResponseWriter, r *http.Request, status int, value interface{}, state inventory.State) inventory.State {
	body, err := json.Marshal(value)
	if err != nil {
		h.logger.Error(err, "encode Console projection response")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, writeErr := w.Write([]byte(`{"state":"unavailable","message":"Console projection unavailable"}`)); writeErr != nil {
			h.logger.Error(writeErr, "write Console projection encoding failure")
		}
		return inventory.StateUnavailable
	}
	if int64(len(body)) > policyread.DefaultInventoryResponseBytes {
		// This is a provisional serialized-response safety guard, not a
		// Console item cap. The check happens before the first write so an
		// oversized projection cannot become a partial success response.
		h.logger.Error(fmt.Errorf("serialized Console projection exceeds provisional response budget"), "reject Console projection response", "bytes", len(body), "budget", policyread.DefaultInventoryResponseBytes)
		h.writeState(w, r, inventory.StateUnavailable)
		return inventory.StateUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		h.logger.Error(err, "write Console projection response")
		return inventory.StateUnavailable
	}
	return state
}

func consoleReadEventType(path string) audit.EventType {
	switch path {
	case consolePolicyScopePath:
		return audit.EventConsolePolicyScopeRead
	case consoleInventoryListPath:
		return audit.EventConsoleManagedResourceListRead
	case "":
		return ""
	default:
		if strings.HasPrefix(path, consoleInventoryListPath+"/") {
			return audit.EventConsoleManagedResourceGetRead
		}
		return ""
	}
}

func unavailableState(err error) inventory.State {
	state := policyread.StateOf(err)
	if state == "" {
		return inventory.StateUnavailable
	}
	return state
}

func resultState(resource inventory.Resource) inventory.State {
	if resource.ID == "" {
		return inventory.StateUnavailable
	}
	return inventory.StateSuccess
}
