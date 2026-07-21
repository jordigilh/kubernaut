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

package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/cert"
	"github.com/jordigilh/kubernaut/pkg/datastorage/dlq"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload" // Issue #756: FileWatcher for cert rotation
)

// Start/Shutdown lifecycle (DD-007 + DD-008 Kubernetes-aware graceful
// shutdown) and the signing-certificate loader. Split from server.go
// (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code motion, no behavior
// change); see server_construction.go for NewServer's dependency wiring and
// server_routes.go for the Handler() route table.

// Start starts the HTTP server, with conditional TLS (#493).
func (s *Server) Start() error {
	s.logger.Info("Starting Data Storage Service server",
		"addr", s.httpServer.Addr,
	)

	// DD-009 V1.0: Start DLQ retry worker before accepting HTTP traffic
	// Issue #667/M4: Use a server-scoped context instead of context.Background()
	s.dlqRetryWorker.Start(context.Background())
	s.retentionWorker.Start(context.Background())

	// Issue #756: Start cert file watcher for hot-reload before accepting connections
	if s.certReloader != nil {
		watcher, err := hotreload.NewFileWatcher(
			filepath.Join(s.tlsCertDir, "tls.crt"),
			s.certReloader.ReloadCallback,
			s.logger.WithName("cert-reloader"),
		)
		if err != nil {
			return fmt.Errorf("failed to create cert file watcher: %w", err)
		}
		if err := watcher.Start(context.Background()); err != nil {
			return fmt.Errorf("failed to start cert file watcher: %w", err)
		}
		s.certWatcher = watcher
	}

	if s.httpServer.TLSConfig != nil {
		s.logger.Info("Server TLS configured", "tls.enabled", true, "tls.certDir", s.tlsCertDir)
		return s.httpServer.ListenAndServeTLS("", "")
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server following DD-007 + DD-008 pattern.
//
// Steps executed (all steps run regardless of prior errors):
//  1. Set readiness flag (Kubernetes removes pod from endpoints)
//  2. Wait for endpoint removal propagation
//  3. Drain in-flight HTTP connections
//     3.5 Stop DLQ retry worker (DD-009)
//  4. Drain DLQ messages to PostgreSQL (DD-008)
//     4.5. Stop retention worker (AU-11) after DLQ drain, before closing DB
//  5. Close external resources (audit store, PostgreSQL)
//
// Returns a joined error if any step failed; individual step errors are
// logged at the point of failure. The returned error supports errors.Is/As.
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownID := uuid.New().String()
	s.logger.Info("Initiating DD-007 + DD-008 Kubernetes-aware graceful shutdown with DLQ drain",
		"shutdown_id", shutdownID)

	var shutdownErrors []error

	// STEP 1: Signal Kubernetes to remove pod from endpoints
	s.shutdownStep1SetFlag(shutdownID)

	// STEP 2: Wait for endpoint removal to propagate
	s.shutdownStep2WaitForPropagation(shutdownID)

	// STEP 3: Drain in-flight HTTP connections
	// #1048 Phase 3: Never skip subsequent cleanup steps. HTTP drain failure
	// must not prevent DLQ drain or DB close — that would cause data loss
	// (BR-AUDIT-001) and leak database connections.
	if err := s.shutdownStep3DrainConnections(ctx, shutdownID); err != nil {
		s.logger.Error(err, "HTTP connection drain failed, continuing with cleanup",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-3-error-non-fatal")
		shutdownErrors = append(shutdownErrors, err)
	}

	// Issue #756: Stop cert file watcher after HTTP server is down
	if s.certWatcher != nil {
		s.certWatcher.Stop()
	}

	// STEP 3.5: Stop DLQ retry worker before draining (DD-009 V1.0)
	s.dlqRetryWorker.Stop()

	// STEP 4: Drain DLQ messages (DD-008) — ARCH-M1: surface DLQ drain errors
	if err := s.shutdownStep4DrainDLQ(ctx, shutdownID); err != nil {
		s.logger.Error(err, "DLQ drain failed during shutdown, continuing with cleanup",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-error-non-fatal")
		shutdownErrors = append(shutdownErrors, err)
	}

	// STEP 4.5: Stop retention worker before closing PostgreSQL (#1048 Phase 5 / AU-11)
	s.retentionWorker.Stop()

	// STEP 5: Close external resources (database)
	if err := s.shutdownStep5CloseResources(shutdownID); err != nil {
		shutdownErrors = append(shutdownErrors, err)
	}

	if len(shutdownErrors) > 0 {
		s.logger.Info("DD-007 + DD-008 graceful shutdown completed with errors",
			"shutdown_id", shutdownID,
			"error_count", len(shutdownErrors),
			"dd", "DD-007-DD-008-complete-with-errors")
		return errors.Join(shutdownErrors...)
	}

	s.logger.Info("DD-007 + DD-008 graceful shutdown complete - all resources closed, DLQ drained",
		"shutdown_id", shutdownID,
		"dd", "DD-007-DD-008-complete-success")
	return nil
}

// shutdownStep1SetFlag sets the shutdown flag to signal readiness probe
// DD-007 STEP 1: This triggers Kubernetes endpoint removal
func (s *Server) shutdownStep1SetFlag(shutdownID string) {
	s.isShuttingDown.Store(true)
	s.logger.Info("Shutdown flag set - readiness probe now returns 503",
		"shutdown_id", shutdownID,
		"effect", "kubernetes_will_remove_from_endpoints",
		"dd", "DD-007-step-1")
}

// shutdownStep2WaitForPropagation waits for Kubernetes endpoint removal to propagate
// DD-007 STEP 2: Industry best practice is 5 seconds (Kubernetes typically takes 1-3s)
func (s *Server) shutdownStep2WaitForPropagation(shutdownID string) {
	delay := s.endpointPropagationDelay
	s.logger.Info("Waiting for Kubernetes endpoint removal to propagate",
		"shutdown_id", shutdownID,
		"delay", delay,
		"dd", "DD-007-step-2")
	if delay > 0 {
		time.Sleep(delay)
	}
	s.logger.Info("Endpoint propagation complete - now draining connections",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-2-complete")
}

// shutdownStep3DrainConnections drains in-flight HTTP connections
// DD-007 STEP 3: Gracefully close HTTP connections with timeout
func (s *Server) shutdownStep3DrainConnections(ctx context.Context, shutdownID string) error {
	s.logger.Info("Draining in-flight HTTP connections",
		"shutdown_id", shutdownID,
		"drain_timeout", drainTimeout,
		"dd", "DD-007-step-3")

	// Create timeout context for draining
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout) //nolint:contextcheck // drain uses a bounded shutdown context, deliberately independent of any request context already cancelled during teardown
	defer cancel()

	// Override parent context if it would timeout sooner
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < drainTimeout {
			drainCtx = ctx
		}
	}

	if err := s.httpServer.Shutdown(drainCtx); err != nil {
		s.logger.Error(err, "Error during HTTP connection drain",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-3-error")
		return fmt.Errorf("HTTP connection drain failed: %w", err)
	}

	s.logger.Info("HTTP connections drained successfully",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-3-complete")
	return nil
}

// shutdownStep4DrainDLQ drains pending DLQ messages before shutdown
// DD-008 STEP 4: Ensure audit messages in DLQ are not lost
func (s *Server) shutdownStep4DrainDLQ(ctx context.Context, shutdownID string) error {
	if s.dlqClient == nil {
		s.logger.Info("DLQ client not available, skipping DLQ drain",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-skipped")
		return nil
	}

	s.logger.Info("Draining DLQ messages before shutdown",
		"shutdown_id", shutdownID,
		"timeout", dlqDrainTimeout,
		"dd", "DD-008-step-4")

	// Create timeout context for DLQ drain.
	// Always start from context.Background() so the DLQ drain gets its full budget
	// even if the parent context expired during HTTP drain (ARCH-M2).
	drainCtx, cancel := context.WithTimeout(context.Background(), dlqDrainTimeout) //nolint:contextcheck // drain uses a bounded shutdown context, deliberately independent of any request context already cancelled during teardown
	defer cancel()

	// Use the parent context only if it has positive remaining time shorter than
	// the DLQ budget — this prevents an expired parent from starving the drain.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < dlqDrainTimeout {
			drainCtx = ctx
		}
	}

	// Drain DLQ with timeout
	stats, err := s.dlqClient.DrainWithTimeout(drainCtx, s.repository, s.auditEventsRepo)
	s.metrics.DLQDrainBatchTotal.Inc()

	if err != nil {
		s.metrics.ShutdownDLQDrainError.Inc()
		s.logger.Error(err, "Error during DLQ drain (non-fatal, continuing shutdown)",
			"shutdown_id", shutdownID,
			"dd", "DD-008-step-4-error")
		return fmt.Errorf("DLQ drain failed: %w", err)
	}

	// Log drain statistics
	s.logger.Info("DLQ drain complete",
		"shutdown_id", shutdownID,
		"notifications_processed", stats.NotificationsProcessed,
		"events_processed", stats.EventsProcessed,
		"total_processed", stats.TotalProcessed,
		"duration", stats.Duration,
		"timed_out", stats.TimedOut,
		"errors", len(stats.Errors),
		"dd", "DD-008-step-4-complete")

	// Log any errors encountered during drain (but don't fail shutdown)
	for i, drainErr := range stats.Errors {
		s.metrics.ShutdownDLQDrainError.Inc()
		s.logger.Error(drainErr, "Error during DLQ drain processing",
			"shutdown_id", shutdownID,
			"error_index", i,
			"dd", "DD-008-step-4-drain-error")
	}
	return nil
}

// shutdownStep5CloseResources closes external resources (database, audit store)
// DD-007 STEP 5 (previously step 4): Clean up database connections and flush audit events
func (s *Server) shutdownStep5CloseResources(shutdownID string) error {
	s.logger.Info("Closing external resources (PostgreSQL, audit store)",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-5")

	// BR-STORAGE-014: Flush remaining audit events before closing database
	// This ensures no audit traces are lost during graceful shutdown
	var step5Errors []error

	if s.auditStore != nil {
		s.logger.Info("Flushing remaining audit events (DD-STORAGE-012)",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-5-audit-flush")
		if err := s.auditStore.Close(); err != nil {
			s.logger.Error(err, "Failed to flush audit events",
				"shutdown_id", shutdownID,
				"dd", "DD-007-step-5-audit-error")
			step5Errors = append(step5Errors, fmt.Errorf("failed to flush audit events: %w", err))
		} else {
			s.logger.Info("Audit events flushed successfully",
				"shutdown_id", shutdownID,
				"dd", "DD-007-step-5-audit-complete")
		}
	}

	// GAP-09 (Issue #1505): Stop the per-IP rate limiter's eviction goroutine, if enabled.
	if s.ipLimiter != nil {
		s.ipLimiter.Stop()
	}

	// Issue #1661 Phase 29 / DD-WORKFLOW-018: stop the workflow cache's informers.
	// Always set as of Phase 55 (K8sRestConfig is mandatory); the nil guard
	// remains as defense-in-depth against a Server built by unusual test
	// helpers that bypass NewServer's validation.
	if s.cancelWorkflowCache != nil {
		s.cancelWorkflowCache()
	}

	// Close PostgreSQL connection
	if s.db == nil {
		s.logger.Info("No PostgreSQL connection to close — verify initialization",
			"shutdown_id", shutdownID,
			"severity", "warning",
			"dd", "DD-007-step-5-no-db")
	} else if err := s.db.Close(); err != nil {
		s.logger.Error(err, "Failed to close PostgreSQL connection",
			"shutdown_id", shutdownID,
			"dd", "DD-007-step-5-error")
		step5Errors = append(step5Errors, fmt.Errorf("failed to close PostgreSQL: %w", err))
	}

	s.logger.Info("All external resources closed",
		"shutdown_id", shutdownID,
		"dd", "DD-007-step-5-complete")
	return errors.Join(step5Errors...)
}

// GetDLQClient returns the DLQ client for testing purposes
// This allows integration tests to verify DD-008 DLQ drain behavior
func (s *Server) GetDLQClient() *dlq.Client {
	return s.dlqClient
}

// loadSigningCertificate loads the signing certificate from cert-manager managed Secret
// SOC2 Day 9.1: Digital signatures for audit exports
// BR-AUDIT-007: Tamper-evident audit logs
//
// Certificate files (under certDir, default /etc/certs from config):
// - tls.crt (PEM certificate)
// - tls.key (PEM private key)
//
// cert-manager Compatibility:
// - Managed by Certificate CRD (deploy/data-storage/certificate.yaml)
// - Auto-rotates 30 days before expiry
// - Self-signed via selfsigned-issuer ClusterIssuer
//
// #1048 Phase 5 / AU-9: Missing or invalid provisioning is a fatal startup error (no fallback).
func loadSigningCertificate(logger logr.Logger, certDir string) (*cert.Signer, error) {
	certFile := filepath.Join(certDir, "tls.crt")
	keyFile := filepath.Join(certDir, "tls.key")

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("signing certificate not found at %s: cert-manager Certificate must be provisioned (AU-9)", certFile)
	}

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("signing key not found at %s: cert-manager Certificate must be provisioned (AU-9)", keyFile)
	}

	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("signing certificate at %s is invalid or corrupt: %w", certFile, err)
	}

	signer, err := cert.NewSignerFromTLSCertificate(&tlsCert)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from certificate: %w", err)
	}

	logger.V(1).Info("Loaded signing certificate from cert-manager",
		"cert_file", certFile,
		"algorithm", signer.GetAlgorithm(),
		"fingerprint", signer.GetCertificateFingerprint())

	return signer, nil
}
