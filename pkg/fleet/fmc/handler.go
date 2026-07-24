/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fmc

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	ScopeCheckPath = "/api/v1/scope/check"
	ClustersPath   = "/api/v1/clusters"
	// HealthzPath is FMC's liveness/health endpoint, served on the same API
	// mux as ScopeCheckPath. Used by HTTPClient.Ping (readiness gate Wave 0)
	// to probe reachability without depending on scope-check semantics.
	HealthzPath = "/healthz"
)

// Handler serves the FMC REST API for federated scope checks and cluster listing.
// ADR-068: GW/RO query this API instead of connecting to Valkey directly.
type Handler struct {
	checker  scope.ScopeChecker
	registry registry.ClusterRegistry
	logger   logr.Logger
}

// NewHandler creates an FMC API handler with the given dependencies.
// The checker is typically a *scopecache.Client wrapping the Valkey backend.
func NewHandler(checker scope.ScopeChecker, reg registry.ClusterRegistry, logger logr.Logger) *Handler {
	return &Handler{
		checker:  checker,
		registry: reg,
		logger:   logger.WithName("fmc-api"),
	}
}

// RegisterRoutes adds the FMC API routes to the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc(ScopeCheckPath, h.handleScopeCheck)
	mux.HandleFunc(ClustersPath, h.handleListClusters)
}

// ScopeCheckResponse is the JSON response for scope check queries.
type ScopeCheckResponse struct {
	Managed bool `json:"managed"`
}

// handleScopeCheck checks whether a resource on a remote cluster is managed.
//
//	GET /api/v1/scope/check?cluster={clusterID}&group={g}&version={v}&kind={k}&namespace={ns}&name={n}
//	-> 200 {"managed": true} or {"managed": false}
//	-> 400 if required params missing
//	-> 405 if not GET
func (h *Handler) handleScopeCheck(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	q := r.URL.Query()
	resource := scope.ResourceIdentity{
		ClusterID: q.Get("cluster"),
		Group:     q.Get("group"),
		Version:   q.Get("version"),
		Kind:      q.Get("kind"),
		Namespace: q.Get("namespace"),
		Name:      q.Get("name"),
	}

	if resource.ClusterID == "" || resource.Kind == "" || resource.Name == "" {
		http.Error(w, "cluster, kind, and name are required query parameters", http.StatusBadRequest)
		return
	}

	if _, known := h.registry.Get(resource.ClusterID); !known {
		h.logger.V(1).Info("scope check rejected: unknown cluster",
			"cluster", resource.ClusterID, "kind", resource.Kind, "name", resource.Name)
		writeJSON(w, ScopeCheckResponse{Managed: false})
		return
	}

	managed, err := h.checker.IsManagedResource(r.Context(), resource)
	if err != nil {
		h.logger.Error(err, "scope check failed",
			"cluster", resource.ClusterID, "kind", resource.Kind, "name", resource.Name)
		writeJSON(w, ScopeCheckResponse{Managed: false})
		return
	}

	writeJSON(w, ScopeCheckResponse{Managed: managed})
}

// ClusterListResponse is the JSON response for cluster listing.
type ClusterListResponse struct {
	Clusters []ClusterInfoResponse `json:"clusters"`
}

// ClusterInfoResponse represents a single cluster in the list response.
// ID-only (issue #1651): cluster display names are non-unique and unsafe
// for disambiguation, so only the unique ID is surfaced.
type ClusterInfoResponse struct {
	ID string `json:"id"`
}

// handleListClusters returns all clusters known to the FMC cluster registry.
//
//	GET /api/v1/clusters
//	-> 200 {"clusters": [{"id": "..."}, ...]}
//	-> 405 if not GET
func (h *Handler) handleListClusters(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	clusters := h.registry.List()
	resp := ClusterListResponse{
		Clusters: make([]ClusterInfoResponse, 0, len(clusters)),
	}
	for _, c := range clusters {
		resp.Clusters = append(resp.Clusters, ClusterInfoResponse{
			ID: c.ID,
		})
	}

	writeJSON(w, resp)
}

// Pinger checks connectivity to a backend store.
type Pinger interface {
	Ping(ctx context.Context) error
}

// readyzPingTimeout bounds how long ReadyzHandler waits on the backend Ping
// before reporting 503. The incoming request context (a Kubernetes kubelet
// probe, or a direct client call) carries no deadline of its own, and
// Issue #1683 moved /readyz onto pkg/shared/health.NewHealthServer, whose
// WriteTimeout (10s) will forcibly reset the TCP connection if the handler
// is still blocked when it fires -- turning a detectable dependency outage
// (503) into an opaque "connection reset by peer" for the caller. Bounding
// the ping here keeps the probe's own failure mode observable (SI-4: no
// silent hang, matching "no silent false-healthy") regardless of server
// timeout configuration.
const readyzPingTimeout = 3 * time.Second

// ReadyzHandler returns an http.HandlerFunc that checks startup readiness
// and backend connectivity. Used for the Kubernetes readiness probe.
func ReadyzHandler(ready func() bool, pinger Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), readyzPingTimeout)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("valkey unreachable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// requireGET returns 405 and false if the request is not GET.
func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// writeJSON writes a 200 OK JSON response. All current call sites in this
// package are success paths; non-2xx responses use http.Error/WriteHeader
// directly (see requireGET, the readiness check above).
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
