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

package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

// OpenAPIValidator is a middleware that validates requests against OpenAPI spec
// BR-STORAGE-034: Automatic API request validation
// BR-STORAGE-019: Prometheus metrics for validation failures
// This middleware:
// - Validates required fields (including empty strings!)
// - Validates enum values
// - Validates field types and formats
// - Returns RFC 7807 Problem Details for validation errors
// - Emits Prometheus metrics for validation failures
type OpenAPIValidator struct {
	router            routers.Router
	logger            logr.Logger
	validationMetrics *prometheus.CounterVec // BR-STORAGE-019: Track validation failures
}

// NewOpenAPIValidator creates a new OpenAPI validation middleware from embedded spec
// DD-API-002: OpenAPI Spec Loading Standard
// BR-STORAGE-034: Automatic API request validation
//
// This function loads the OpenAPI spec from embedded bytes (compile-time inclusion).
// No file path configuration needed - spec is part of the binary.
//
// Parameters:
//   - logger: Logger for validation events
//   - validationMetrics: Prometheus counter for validation failures (optional)
//
// Returns:
//   - *OpenAPIValidator: Configured validator middleware
//   - error: Spec loading or validation error
func NewOpenAPIValidator(logger logr.Logger, validationMetrics *prometheus.CounterVec) (*OpenAPIValidator, error) {
	ctx := context.Background()
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// Load from embedded bytes (NO file path dependencies)
	// DD-API-002: Spec is embedded at compile time via //go:embed directive
	doc, err := loader.LoadFromData(embeddedOpenAPISpec)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded OpenAPI spec: %w", err)
	}

	// Validate spec structure
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("OpenAPI spec validation failed: %w", err)
	}

	// Create router for matching requests to operations
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAPI router: %w", err)
	}

	logger.Info("OpenAPI validator initialized from embedded spec",
		"api_version", doc.Info.Version,
		"paths_count", len(doc.Paths.Map()),
		"metrics_enabled", validationMetrics != nil)

	return &OpenAPIValidator{
		router:            router,
		logger:            logger,
		validationMetrics: validationMetrics,
	}, nil
}

// Middleware returns a Chi-compatible middleware handler
// Routes not in the OpenAPI spec (e.g., /health, /metrics) are passed through without validation
func (v *OpenAPIValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// kin-openapi router requires full URL when 'servers' are defined in spec
		// Convert path-only request to full URL for routing
		routingRequest := r.Clone(r.Context())
		if routingRequest.URL.Scheme == "" {
			// Path-only request - add scheme and host for routing
			routingRequest.URL.Scheme = "http"
			routingRequest.URL.Host = "localhost:8080" // Match spec server
		}

		// Find the operation for this request
		route, pathParams, err := v.router.FindRoute(routingRequest)
		if err != nil {
			// Route not in OpenAPI spec (e.g., /health, /metrics)
			// Pass through without validation
			v.logger.V(2).Info("Route not in OpenAPI spec, skipping validation",
				"method", r.Method,
				"path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		// Log validation attempt
		operationID := "unknown"
		if route.Operation != nil && route.Operation.OperationID != "" {
			operationID = route.Operation.OperationID
		}

		v.logger.V(1).Info("Validating request against OpenAPI spec",
			"method", r.Method,
			"path", r.URL.Path,
			"operation_id", operationID)

		// Configure validation options
		options := &openapi3filter.Options{
			// Enable strict validation
			ExcludeRequestBody:    false, // Validate request body
			ExcludeResponseBody:   true,  // Don't validate responses (performance)
			IncludeResponseStatus: false,

			// Collect all validation errors, not just the first one
			MultiError: true,
		}

		// Create validation input
		requestValidationInput := &openapi3filter.RequestValidationInput{
			Request:    r,
			PathParams: pathParams,
			Route:      route,
			Options:    options,
		}

		// Validate request against OpenAPI spec
		if err := openapi3filter.ValidateRequest(r.Context(), requestValidationInput); err != nil {
			// Validation failed - return RFC 7807 error
			v.logger.Info("Request validation failed",
				"method", r.Method,
				"path", r.URL.Path,
				"operation_id", operationID,
				"error", err.Error())

			// BR-STORAGE-019: Emit validation failure metric
			if v.validationMetrics != nil {
				v.validationMetrics.WithLabelValues("openapi_middleware", "validation_error").Inc()
			}

			v.writeValidationError(w, r, err)
			return
		}

		// Validation passed
		v.logger.V(1).Info("Request validation passed",
			"method", r.Method,
			"path", r.URL.Path,
			"operation_id", operationID)

		// Proceed to handler
		next.ServeHTTP(w, r)
	})
}

// writeValidationError writes an RFC 7807 error response for validation failures
func (v *OpenAPIValidator) writeValidationError(w http.ResponseWriter, r *http.Request, validationErr error) {
	// Parse validation errors
	var details string
	var validationErrors []string

	// kin-openapi may return multiple validation errors
	if multiErr, ok := validationErr.(interface{ Unwrap() []error }); ok {
		for _, err := range multiErr.Unwrap() {
			validationErrors = append(validationErrors, err.Error())
		}
		if len(validationErrors) > 0 {
			details = validationErrors[0] // Use first error as primary detail
			if len(validationErrors) > 1 {
				details = fmt.Sprintf("%s (and %d more errors)", details, len(validationErrors)-1)
			}
		}
	} else {
		details = validationErr.Error()
	}

	// RFC 7807 Problem Details
	problem := map[string]interface{}{
		"type":   "https://kubernaut.ai/problems/validation-error",
		"title":  "Validation Error",
		"status": http.StatusBadRequest,
		"detail": details,
	}

	// Add request context
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
		problem["instance"] = r.URL.Path
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)

	// Write JSON response
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		v.logger.Error(err, "Failed to encode validation error response")
	}
}
