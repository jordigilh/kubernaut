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

package metrics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Server represents a metrics server
type Server struct {
	server *http.Server
	log    *logrus.Logger
}

// NewServer creates a new metrics server
func NewServer(port string, log *logrus.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	return &Server{
		server: &http.Server{
			Addr:    ":" + port,
			Handler: mux,
		},
		log: log,
	}
}

// Start starts the metrics server
func (s *Server) Start() error {
	s.log.WithField("addr", s.server.Addr).Info("Starting metrics server")

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	return nil
}

// Stop gracefully stops the metrics server
func (s *Server) Stop(ctx context.Context) error {
	s.log.Info("Stopping metrics server")

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to stop metrics server: %w", err)
	}

	return nil
}

// StartAsync starts the metrics server in a goroutine
func (s *Server) StartAsync() {
	go func() {
		if err := s.Start(); err != nil {
			s.log.WithError(err).Error("Metrics server error")
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)
}
