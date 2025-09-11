package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// ContextAPIServer provides HTTP server for HolmesGPT Context API integration
// Business Requirement: Enable HolmesGPT dynamic context orchestration
// Following development guideline: integrate with existing code
type ContextAPIServer struct {
	server            *http.Server
	contextController *contextapi.ContextController
	log               *logrus.Logger
}

// ContextAPIConfig holds configuration for the Context API server
type ContextAPIConfig struct {
	Host    string        `yaml:"host"`
	Port    int           `yaml:"port"`
	Timeout time.Duration `yaml:"timeout"`
}

// NewContextAPIServer creates a new Context API server using standard library
// Following development guideline: reuse existing patterns
func NewContextAPIServer(config ContextAPIConfig, aiIntegrator *engine.AIServiceIntegrator, log *logrus.Logger) *ContextAPIServer {
	contextController := contextapi.NewContextController(aiIntegrator, log)

	mux := http.NewServeMux()

	// Register context API routes
	contextController.RegisterRoutes(mux)

	// Wrap with middleware
	handler := corsMiddleware(loggingMiddleware(log)(mux))

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      handler,
		ReadTimeout:  config.Timeout,
		WriteTimeout: config.Timeout,
		IdleTimeout:  config.Timeout * 2,
	}

	return &ContextAPIServer{
		server:            server,
		contextController: contextController,
		log:               log,
	}
}

// Start starts the Context API server
func (s *ContextAPIServer) Start() error {
	s.log.WithFields(logrus.Fields{
		"address": s.server.Addr,
		"service": "context-api",
	}).Info("Starting HolmesGPT Context API server")

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start context API server: %w", err)
	}

	return nil
}

// Stop gracefully stops the Context API server
func (s *ContextAPIServer) Stop(ctx context.Context) error {
	s.log.Info("Stopping HolmesGPT Context API server")

	return s.server.Shutdown(ctx)
}

// corsMiddleware adds CORS headers for HolmesGPT integration
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow HolmesGPT to access the Context API
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware adds request logging using standard library
func loggingMiddleware(log *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			next.ServeHTTP(w, r)

			log.WithFields(logrus.Fields{
				"method":   r.Method,
				"path":     r.URL.Path,
				"duration": time.Since(start),
				"service":  "context-api",
			}).Debug("Context API request processed")
		})
	}
}
