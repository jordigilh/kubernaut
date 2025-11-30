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

package log

// Standard field keys for structured logging.
//
// DD-005: Use these constants to ensure consistent field names across all services.
// This enables effective log aggregation and querying in production.
//
// Example:
//
//	logger.Info("Request processed",
//	    log.KeyRequestID, requestID,
//	    log.KeyDurationMS, durationMs,
//	    log.KeyStatusCode, statusCode,
//	)
const (
	// Request tracing fields
	KeyRequestID = "request_id"
	KeyTraceID   = "trace_id"
	KeySpanID    = "span_id"

	// Service identification
	KeyService     = "service"
	KeyComponent   = "component"
	KeyEnvironment = "environment"
	KeyVersion     = "version"
	KeyGitCommit   = "git_commit"
	KeyBuildDate   = "build_date"

	// Kubernetes context
	KeyNamespace  = "namespace"
	KeyPodName    = "pod_name"
	KeyNodeName   = "node_name"
	KeyCluster    = "cluster"
	KeyController = "controller"

	// HTTP request fields
	KeyMethod     = "method"
	KeyEndpoint   = "endpoint"
	KeyPath       = "path"
	KeyStatusCode = "status_code"
	KeySourceIP   = "source_ip"
	KeyUserAgent  = "user_agent"

	// Performance metrics
	KeyDurationMS      = "duration_ms"
	KeyDurationSeconds = "duration_seconds"
	KeyBytesProcessed  = "bytes_processed"
	KeyRetryCount      = "retry_count"

	// Business domain fields
	KeySignalName       = "signal_name"
	KeyFingerprint      = "fingerprint"
	KeySeverity         = "severity"
	KeyWorkflowID       = "workflow_id"
	KeyWorkflowName     = "workflow_name"
	KeyWorkflowVersion  = "workflow_version"
	KeyIncidentType     = "incident_type"
	KeyRemediationID    = "remediation_id"
	KeyCorrelationID    = "correlation_id"
	KeyAlertFingerprint = "alert_fingerprint"

	// Database fields
	KeyDatabase  = "database"
	KeyTable     = "table"
	KeyQuery     = "query"
	KeyRowCount  = "row_count"
	KeyOperation = "operation"

	// Error fields
	KeyError      = "error"
	KeyErrorCode  = "error_code"
	KeyErrorType  = "error_type"
	KeyStackTrace = "stack_trace"

	// Audit fields
	KeyEventType = "event_type"
	KeyOutcome   = "outcome"
	KeyActor     = "actor"
	KeyResource  = "resource"
	KeyAction    = "action"

	// Graceful shutdown fields (DD-007)
	KeyPhase  = "phase"
	KeyStatus = "status"
)

// Standard field values for common scenarios.
const (
	// Outcomes
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
	OutcomeSkipped = "skipped"

	// Phases (DD-007 graceful shutdown)
	PhaseStartup           = "startup"
	PhaseRunning           = "running"
	PhaseShutdown          = "shutdown"
	PhaseEndpointRemoval   = "endpoint_removal"
	PhaseDrainingConns     = "draining_connections"
	PhaseClosingResources  = "closing_resources"
	PhaseShutdownComplete  = "shutdown_complete"

	// Statuses
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
	StatusDegraded  = "degraded"
	StatusReady     = "ready"
	StatusNotReady  = "not_ready"
)

