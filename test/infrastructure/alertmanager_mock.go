package infrastructure

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// ============================================================================
// AlertManager Mock Server for Integration Tests (Tier 2)
// ============================================================================
//
// Provides a configurable httptest.NewServer that responds to AlertManager API
// endpoints with canned responses. Used in EM integration tests to simulate
// AlertManager alert queries without requiring a real AlertManager instance.
//
// This is a documented exception per TESTING_GUIDELINES.md v2.6.0 Section 4a:
// real AlertManager contract validation is deferred to E2E (Tier 3) tests.
//
// Each Ginkgo process gets its own mock server on an ephemeral OS port,
// so no port allocation or collision management is needed.
//
// Supported endpoints:
//   - GET  /api/v2/alerts   (list alerts)
//   - POST /api/v2/alerts   (create alerts -- for test setup)
//   - GET  /-/ready         (readiness check)
//   - GET  /-/healthy       (health check)
//
// References:
//   - ADR-EM-001: EM integration architecture
//   - TESTING_GUIDELINES.md v2.6.0 Section 4a: Prom/AM mocking policy
// ============================================================================

// AMAlert represents an alert in the AlertManager v2 API format.
type AMAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	StartsAt     string            `json:"startsAt,omitempty"`
	EndsAt       string            `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
	Status       *AMAlertStatus    `json:"status,omitempty"`
	Receivers    []AMReceiver      `json:"receivers,omitempty"`
}

// AMAlertStatus represents the status of an alert.
type AMAlertStatus struct {
	State       string   `json:"state"`       // "active", "suppressed", "unprocessed"
	SilencedBy  []string `json:"silencedBy"`  // IDs of silences
	InhibitedBy []string `json:"inhibitedBy"` // IDs of inhibiting alerts
}

// AMReceiver represents an AlertManager receiver.
type AMReceiver struct {
	Name string `json:"name"`
}

// MockAlertManagerConfig configures the mock AlertManager server behavior.
type MockAlertManagerConfig struct {
	// AlertsResponse is the list of alerts returned by GET /api/v2/alerts.
	// If nil, an empty list is returned.
	AlertsResponse []AMAlert

	// AlertsHandler overrides the default handler for GET /api/v2/alerts.
	// When set, AlertsResponse is ignored.
	AlertsHandler http.HandlerFunc

	// PostAlertsHandler overrides the default handler for POST /api/v2/alerts.
	// When nil, POST returns 200 OK (accepting alerts silently).
	PostAlertsHandler http.HandlerFunc

	// Ready controls the response of the /-/ready endpoint.
	// When false, the endpoint returns 503.
	Ready bool

	// Healthy controls the response of the /-/healthy endpoint.
	// When false, the endpoint returns 503.
	Healthy bool
}

// MockAlertManager wraps an httptest.Server with AlertManager-specific configuration.
type MockAlertManager struct {
	Server *httptest.Server
	mu     sync.Mutex
	config MockAlertManagerConfig

	// RequestLog tracks all requests received by the mock.
	RequestLog []MockHTTPRequest

	// PostedAlerts collects alerts received via POST /api/v2/alerts.
	PostedAlerts []AMAlert
}

// NewMockAlertManager creates a new mock AlertManager httptest server.
//
// Usage:
//
//	mock := NewMockAlertManager(MockAlertManagerConfig{
//	    AlertsResponse: []AMAlert{{Labels: map[string]string{"alertname": "HighCPU"}, ...}},
//	    Ready: true,
//	    Healthy: true,
//	})
//	defer mock.Close()
//	// Use mock.URL() as the AlertManager endpoint
func NewMockAlertManager(config MockAlertManagerConfig) *MockAlertManager {
	mam := &MockAlertManager{
		config: config,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v2/alerts", func(w http.ResponseWriter, r *http.Request) {
		mam.logRequest(r)

		switch r.Method {
		case http.MethodGet:
			mam.handleGetAlerts(w, r)
		case http.MethodPost:
			mam.handlePostAlerts(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
		mam.logRequest(r)
		mam.mu.Lock()
		ready := mam.config.Ready
		mam.mu.Unlock()

		if ready {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "OK\n")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "Service Unavailable\n")
		}
	})

	mux.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		mam.logRequest(r)
		mam.mu.Lock()
		healthy := mam.config.Healthy
		mam.mu.Unlock()

		if healthy {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "OK\n")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "Service Unavailable\n")
		}
	})

	mam.Server = httptest.NewServer(mux)
	return mam
}

func (mam *MockAlertManager) handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	mam.mu.Lock()
	handler := mam.config.AlertsHandler
	response := mam.config.AlertsResponse
	mam.mu.Unlock()

	if handler != nil {
		handler(w, r)
		return
	}

	alerts := response
	if alerts == nil {
		alerts = []AMAlert{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(alerts); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

func (mam *MockAlertManager) handlePostAlerts(w http.ResponseWriter, r *http.Request) {
	mam.mu.Lock()
	handler := mam.config.PostAlertsHandler
	mam.mu.Unlock()

	if handler != nil {
		handler(w, r)
		return
	}

	var alerts []AMAlert
	if err := json.NewDecoder(r.Body).Decode(&alerts); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	mam.mu.Lock()
	mam.PostedAlerts = append(mam.PostedAlerts, alerts...)
	mam.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// URL returns the base URL of the mock AlertManager server.
func (mam *MockAlertManager) URL() string {
	return mam.Server.URL
}

// Close shuts down the mock server.
func (mam *MockAlertManager) Close() {
	mam.Server.Close()
}

// SetAlertsResponse updates the canned response for GET /api/v2/alerts at runtime.
func (mam *MockAlertManager) SetAlertsResponse(alerts []AMAlert) {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.config.AlertsResponse = alerts
}

// SetReady updates the readiness state of the mock.
func (mam *MockAlertManager) SetReady(ready bool) {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.config.Ready = ready
}

// SetHealthy updates the health state of the mock.
func (mam *MockAlertManager) SetHealthy(healthy bool) {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.config.Healthy = healthy
}

// GetRequestLog returns a copy of all recorded requests.
func (mam *MockAlertManager) GetRequestLog() []MockHTTPRequest {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	log := make([]MockHTTPRequest, len(mam.RequestLog))
	copy(log, mam.RequestLog)
	return log
}

// GetPostedAlerts returns a copy of all alerts received via POST.
func (mam *MockAlertManager) GetPostedAlerts() []AMAlert {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	alerts := make([]AMAlert, len(mam.PostedAlerts))
	copy(alerts, mam.PostedAlerts)
	return alerts
}

// ResetRequestLog clears the request log.
func (mam *MockAlertManager) ResetRequestLog() {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.RequestLog = nil
}

// ResetPostedAlerts clears the posted alerts log.
func (mam *MockAlertManager) ResetPostedAlerts() {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.PostedAlerts = nil
}

func (mam *MockAlertManager) logRequest(r *http.Request) {
	mam.mu.Lock()
	defer mam.mu.Unlock()
	mam.RequestLog = append(mam.RequestLog, MockHTTPRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
	})
}

// ============================================================================
// Helper constructors for common AlertManager mock responses
// ============================================================================

// NewResolvedAlert creates an AMAlert in the "resolved" state for testing.
func NewResolvedAlert(name string, labels map[string]string) AMAlert {
	allLabels := make(map[string]string)
	for k, v := range labels {
		allLabels[k] = v
	}
	allLabels["alertname"] = name

	return AMAlert{
		Labels: allLabels,
		Status: &AMAlertStatus{
			State:       "suppressed",
			SilencedBy:  []string{},
			InhibitedBy: []string{},
		},
	}
}

// NewFiringAlert creates an AMAlert in the "firing" (active) state for testing.
func NewFiringAlert(name string, labels map[string]string) AMAlert {
	allLabels := make(map[string]string)
	for k, v := range labels {
		allLabels[k] = v
	}
	allLabels["alertname"] = name

	return AMAlert{
		Labels: allLabels,
		Status: &AMAlertStatus{
			State:       "active",
			SilencedBy:  []string{},
			InhibitedBy: []string{},
		},
	}
}
