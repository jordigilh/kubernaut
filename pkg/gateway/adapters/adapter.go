/*
Copyright 2025 Jordi Gil.

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

package adapters

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/gateway/types"
)

// SignalAdapter converts source-specific signal formats to NormalizedSignal
//
// This interface defines the contract for all signal source adapters (Prometheus,
// Kubernetes events, Grafana, etc.). Each adapter is responsible for:
// 1. Parsing source-specific payload formats
// 2. Extracting required fields (alertname, severity, resource info)
// 3. Generating fingerprints for deduplication
// 4. Converting to the unified NormalizedSignal format
//
// Design Decision: Adapter-specific endpoints (Design B)
// - Each adapter registers its own HTTP route (e.g., /api/v1/signals/prometheus)
// - No detection logic needed - HTTP router dispatches directly to adapter
// - ~70% less code, better security, better performance vs detection-based approach
type SignalAdapter interface {
	// Name returns the adapter identifier
	// Examples: "prometheus", "kubernetes-event", "grafana"
	// Used for logging, metrics, and adapter registration
	Name() string

	// Parse converts source-specific raw payload to NormalizedSignal
	//
	// This method must:
	// 1. Unmarshal the source-specific JSON/format
	// 2. Extract required fields (alertname, severity, namespace, resource)
	// 3. Generate fingerprint (SHA256 of key fields)
	// 4. Populate NormalizedSignal with all required fields
	//
	// Context is provided for cancellation/timeout (typically 5-10ms parse time)
	//
	// Returns:
	// - NormalizedSignal: Unified format for Gateway processing pipeline
	// - error: Parse errors (invalid JSON, missing required fields, etc.)
	Parse(ctx context.Context, rawData []byte) (*types.NormalizedSignal, error)

	// Validate checks if the parsed signal meets minimum requirements
	//
	// This method validates:
	// - Required fields are populated (fingerprint, alertName, severity)
	// - Field values are valid (severity must be critical/warning/info)
	// - Namespace is specified (required for Kubernetes-targeted signals)
	//
	// Validation happens AFTER Parse but BEFORE processing pipeline.
	// Failed validation returns HTTP 400 Bad Request to the caller.
	//
	// Returns:
	// - error: Validation errors (missing fields, invalid values, etc.)
	Validate(signal *types.NormalizedSignal) error

	// GetMetadata returns adapter information for observability
	//
	// Metadata is used for:
	// - Logging adapter registration at startup
	// - Metrics labels (adapter name, version)
	// - API documentation generation
	// - Troubleshooting (supported content types, required headers)
	GetMetadata() AdapterMetadata
}

// RoutableAdapter extends SignalAdapter with HTTP route registration
//
// ALL adapters MUST implement this interface to register their endpoints.
// The HTTP server iterates over registered adapters and calls GetRoute()
// to set up adapter-specific routes at server startup.
//
// Design Decision: Self-registered endpoints (configuration-driven)
// - Adapters define their own routes (explicit, not hardcoded in server)
// - Server dynamically registers routes from enabled adapters
// - Easy to add new adapters without modifying server code
type RoutableAdapter interface {
	SignalAdapter

	// GetRoute returns the HTTP route path for this adapter
	//
	// Route format: "/api/v1/signals/{source}"
	//
	// Examples:
	// - Prometheus: "/api/v1/signals/prometheus"
	// - Kubernetes: "/api/v1/signals/kubernetes-event"
	// - Grafana: "/api/v1/signals/grafana"
	//
	// The route is registered at server startup and dispatches HTTP requests
	// directly to this adapter's Parse() method (no detection logic).
	//
	// Returns:
	// - string: HTTP route path (must start with /api/v1/signals/)
	GetRoute() string
}

// AdapterMetadata provides adapter information for observability
//
// This struct is returned by GetMetadata() and used for:
// - Startup logging (adapter registration)
// - Prometheus metrics labels
// - API documentation
// - Client configuration guidance (supported content types, required headers)
type AdapterMetadata struct {
	// Name is the adapter identifier (same as Name() method)
	// Examples: "prometheus", "kubernetes-event"
	Name string

	// Version is the adapter implementation version
	// Format: "1.0", "1.1", etc.
	// Used to track adapter changes and compatibility
	Version string

	// Description is a human-readable description
	// Example: "Handles Prometheus AlertManager webhook notifications"
	Description string

	// SupportedContentTypes lists accepted Content-Type headers
	// Examples: ["application/json"], ["application/json", "application/xml"]
	// Used for API documentation and request validation
	SupportedContentTypes []string

	// RequiredHeaders lists mandatory HTTP headers (optional)
	// Examples: ["X-Prometheus-External-URL"], ["Authorization"]
	// Used for API documentation
	RequiredHeaders []string
}
