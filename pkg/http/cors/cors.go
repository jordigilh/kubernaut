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

// Package cors provides shared CORS configuration for Kubernaut HTTP services.
//
// This package implements BR-HTTP-015 (CORS and security policy enforcement)
// and provides consistent CORS configuration across all stateless services.
//
// Design Decision: DD-HTTP-001 (HTTP Router Strategy)
// Services using chi router should use this shared CORS configuration.
//
// Usage:
//
//	import (
//	    "github.com/go-chi/chi/v5"
//	    kubecors "github.com/jordigilh/kubernaut/pkg/http/cors"
//	)
//
//	func setupRouter() chi.Router {
//	    r := chi.NewRouter()
//
//	    // Option 1: Use environment-based configuration
//	    corsOpts := kubecors.FromEnvironment()
//	    r.Use(kubecors.Handler(corsOpts))
//
//	    // Option 2: Use custom configuration
//	    corsOpts := &kubecors.Options{
//	        AllowedOrigins: []string{"https://app.example.com"},
//	        AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
//	    }
//	    r.Use(kubecors.Handler(corsOpts))
//
//	    return r
//	}
package cors

import (
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/cors"
)

// Options contains CORS configuration for HTTP services.
//
// This struct provides a consistent CORS configuration pattern across all
// Kubernaut services that expose HTTP APIs.
//
// Configuration fields:
//   - AllowedOrigins: List of origins that are allowed to access the resource.
//     Use ["*"] for development only. In production, specify exact origins.
//   - AllowedMethods: HTTP methods allowed for cross-origin requests.
//   - AllowedHeaders: HTTP headers that can be used in the actual request.
//   - ExposedHeaders: Headers that are safe to expose to the API of a CORS response.
//   - AllowCredentials: Whether the request can include user credentials.
//   - MaxAge: How long (in seconds) the results of a preflight request can be cached.
//
// Security considerations:
//   - Never use AllowedOrigins: ["*"] in production
//   - AllowCredentials should only be true if specific origins are whitelisted
//   - MaxAge should be set to reduce preflight request overhead
//
// Example YAML configuration:
//
//	cors:
//	  allowed_origins:
//	    - "https://app.kubernaut.io"
//	    - "https://dashboard.kubernaut.io"
//	  allowed_methods:
//	    - "GET"
//	    - "POST"
//	    - "PUT"
//	    - "DELETE"
//	    - "OPTIONS"
//	  allowed_headers:
//	    - "Accept"
//	    - "Authorization"
//	    - "Content-Type"
//	    - "X-Request-ID"
//	  exposed_headers:
//	    - "Link"
//	    - "X-Request-ID"
//	  allow_credentials: false
//	  max_age: 300
type Options struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

// DefaultOptions returns CORS options with sensible defaults for development.
//
// Default values:
//   - AllowedOrigins: ["*"] (DEVELOPMENT ONLY - configure in production)
//   - AllowedMethods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
//   - AllowedHeaders: ["Accept", "Authorization", "Content-Type", "X-Request-ID"]
//   - ExposedHeaders: ["Link", "X-Request-ID", "X-Total-Count"]
//   - AllowCredentials: false (safe default)
//   - MaxAge: 300 (5 minutes)
//
// WARNING: These defaults use AllowedOrigins: ["*"] which is NOT secure
// for production. Always configure specific origins in production.
//
// Returns:
//   - *Options: CORS options with default values
//
// Example:
//
//	opts := cors.DefaultOptions()
//	// Override for production
//	opts.AllowedOrigins = []string{"https://app.kubernaut.io"}
//	r.Use(cors.Handler(opts))
func DefaultOptions() *Options {
	return &Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID", "X-Total-Count"},
		AllowCredentials: false,
		MaxAge:           300,
	}
}

// ProductionOptions returns CORS options with restrictive defaults for production.
//
// Default values:
//   - AllowedOrigins: [] (empty - MUST be configured)
//   - AllowedMethods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
//   - AllowedHeaders: ["Accept", "Authorization", "Content-Type", "X-Request-ID"]
//   - ExposedHeaders: ["Link", "X-Request-ID", "X-Total-Count"]
//   - AllowCredentials: false
//   - MaxAge: 300 (5 minutes)
//
// IMPORTANT: AllowedOrigins is empty by default and MUST be configured
// before use. This prevents accidental deployment with insecure defaults.
//
// Returns:
//   - *Options: CORS options with restrictive production defaults
//
// Example:
//
//	opts := cors.ProductionOptions()
//	opts.AllowedOrigins = []string{
//	    "https://app.kubernaut.io",
//	    "https://dashboard.kubernaut.io",
//	}
//	r.Use(cors.Handler(opts))
func ProductionOptions() *Options {
	return &Options{
		AllowedOrigins:   []string{}, // MUST be configured
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link", "X-Request-ID", "X-Total-Count"},
		AllowCredentials: false,
		MaxAge:           300,
	}
}

// FromEnvironment creates CORS options from environment variables.
//
// Environment variables:
//   - CORS_ALLOWED_ORIGINS: Comma-separated list of allowed origins
//     Example: "https://app.kubernaut.io,https://dashboard.kubernaut.io"
//     Default: "*" (development only)
//   - CORS_ALLOWED_METHODS: Comma-separated list of allowed HTTP methods
//     Example: "GET,POST,PUT,DELETE"
//     Default: "GET,POST,PUT,PATCH,DELETE,OPTIONS"
//   - CORS_ALLOWED_HEADERS: Comma-separated list of allowed headers
//     Example: "Accept,Authorization,Content-Type"
//     Default: "Accept,Authorization,Content-Type,X-Request-ID"
//   - CORS_ALLOW_CREDENTIALS: "true" or "false"
//     Default: "false"
//   - CORS_MAX_AGE: Preflight cache duration in seconds
//     Default: "300"
//
// This function is designed for Kubernetes deployments where configuration
// is passed via environment variables from ConfigMaps or Secrets.
//
// Returns:
//   - *Options: CORS options populated from environment variables
//
// Example Kubernetes ConfigMap:
//
//	apiVersion: v1
//	kind: ConfigMap
//	metadata:
//	  name: data-storage-config
//	data:
//	  CORS_ALLOWED_ORIGINS: "https://app.kubernaut.io"
//	  CORS_ALLOWED_METHODS: "GET,POST,PUT,DELETE,OPTIONS"
//	  CORS_ALLOW_CREDENTIALS: "false"
//
// Example usage:
//
//	opts := cors.FromEnvironment()
//	r.Use(cors.Handler(opts))
func FromEnvironment() *Options {
	opts := DefaultOptions()

	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		opts.AllowedOrigins = splitAndTrim(origins)
	}

	if methods := os.Getenv("CORS_ALLOWED_METHODS"); methods != "" {
		opts.AllowedMethods = splitAndTrim(methods)
	}

	if headers := os.Getenv("CORS_ALLOWED_HEADERS"); headers != "" {
		opts.AllowedHeaders = splitAndTrim(headers)
	}

	if exposed := os.Getenv("CORS_EXPOSED_HEADERS"); exposed != "" {
		opts.ExposedHeaders = splitAndTrim(exposed)
	}

	if creds := os.Getenv("CORS_ALLOW_CREDENTIALS"); creds == "true" {
		opts.AllowCredentials = true
	}

	if maxAge := os.Getenv("CORS_MAX_AGE"); maxAge != "" {
		// Parse max age, default to 300 if invalid
		var age int
		if _, err := parseMaxAge(maxAge, &age); err == nil {
			opts.MaxAge = age
		}
	}

	return opts
}

// Handler returns a chi-compatible CORS middleware handler.
//
// This function converts our Options to go-chi/cors.Options and returns
// a middleware handler that can be used with chi.Router.Use().
//
// Parameters:
//   - opts: CORS configuration options
//
// Returns:
//   - func(http.Handler) http.Handler: chi-compatible middleware
//
// Example:
//
//	r := chi.NewRouter()
//	opts := cors.FromEnvironment()
//	r.Use(cors.Handler(opts))
//	r.Get("/api/v1/health", healthHandler)
func Handler(opts *Options) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   opts.AllowedOrigins,
		AllowedMethods:   opts.AllowedMethods,
		AllowedHeaders:   opts.AllowedHeaders,
		ExposedHeaders:   opts.ExposedHeaders,
		AllowCredentials: opts.AllowCredentials,
		MaxAge:           opts.MaxAge,
	})
}

// IsProduction returns true if the CORS configuration appears to be
// production-ready (i.e., does not allow all origins).
//
// This function can be used for logging warnings or validation during
// service startup.
//
// Returns:
//   - bool: true if configuration appears production-ready
//
// Example:
//
//	opts := cors.FromEnvironment()
//	if !opts.IsProduction() {
//	    logger.Warn("CORS configuration allows all origins - not recommended for production")
//	}
func (o *Options) IsProduction() bool {
	for _, origin := range o.AllowedOrigins {
		if origin == "*" {
			return false
		}
	}
	return len(o.AllowedOrigins) > 0
}

// splitAndTrim splits a comma-separated string and trims whitespace.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseMaxAge parses a string to an integer for max age.
func parseMaxAge(s string, result *int) (int, error) {
	var age int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil // Invalid, return 0
		}
		age = age*10 + int(c-'0')
	}
	*result = age
	return age, nil
}
