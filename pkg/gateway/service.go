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

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/middleware"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Service defines the Gateway service interface
// Business Requirements: BR-WH-001, BR-WH-003, BR-WH-011, BR-WH-026, BR-WH-025
type Service interface {
	GetHTTPHandler() http.Handler
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetAccessLogs() []string
	GetSecurityLogs() []string
}

// AlertProcessorClient defines the interface for communicating with Alert Processor service
// Business Requirements: BR-WH-026 (Alert Processor integration)
type AlertProcessorClient interface {
	ForwardAlert(ctx context.Context, alert *types.Alert) error
	HealthCheck(ctx context.Context) error
}

// Config holds Gateway service configuration
type Config struct {
	Port           int                  `yaml:"port"`
	AlertProcessor AlertProcessorConfig `yaml:"alert_processor"`
	Authentication auth.AuthConfig      `yaml:"authentication,omitempty"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit,omitempty"`
	Logging        LoggingConfig        `yaml:"logging,omitempty"`
}

// AlertProcessorConfig holds Alert Processor client configuration
type AlertProcessorConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	BurstSize         int `yaml:"burst_size"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	AccessLog   bool   `yaml:"access_log"`
	SecurityLog bool   `yaml:"security_log"`
	Level       string `yaml:"level"`
}

// AlertManagerWebhook represents the AlertManager webhook payload structure
// Reusing existing types from pkg/integration/webhook/handler.go
type AlertManagerWebhook struct {
	Version           string              `json:"version"`
	GroupKey          string              `json:"groupKey"`
	TruncatedAlerts   int                 `json:"truncatedAlerts"`
	Status            string              `json:"status"`
	Receiver          string              `json:"receiver"`
	GroupLabels       map[string]string   `json:"groupLabels"`
	CommonLabels      map[string]string   `json:"commonLabels"`
	CommonAnnotations map[string]string   `json:"commonAnnotations"`
	ExternalURL       string              `json:"externalURL"`
	Alerts            []AlertManagerAlert `json:"alerts"`
}

// AlertManagerAlert represents a single alert from AlertManager
// Reusing existing types from pkg/integration/webhook/handler.go
type AlertManagerAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       *time.Time        `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

// WebhookResponse represents the response sent back to AlertManager
// Reusing existing types from pkg/integration/webhook/handler.go
type WebhookResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// service implements the Gateway Service interface
type service struct {
	config        *Config
	alertClient   AlertProcessorClient
	authenticator middleware.Authenticator
	logger        *logrus.Logger
	handler       http.Handler
	accessLogs    []string
	securityLogs  []string
	rateLimiter   *rate.Limiter
	// REFACTOR: Enhanced rate limiting with per-IP tracking
	ipRateLimiters map[string]*rate.Limiter
	mu             sync.RWMutex
}

// NewService creates a new Gateway service with dependency injection
// Authentication is pluggable via the Authenticator interface
func NewService(config *Config, alertClient AlertProcessorClient, authenticator middleware.Authenticator, logger *logrus.Logger) Service {
	s := &service{
		config:         config,
		alertClient:    alertClient,
		authenticator:  authenticator,
		logger:         logger,
		accessLogs:     make([]string, 0),
		securityLogs:   make([]string, 0),
		ipRateLimiters: make(map[string]*rate.Limiter),
	}

	// BR-WH-023: Initialize rate limiter (REFACTOR: enhanced with per-IP tracking)
	if config.RateLimit.RequestsPerMinute > 0 {
		ratePerSecond := rate.Limit(float64(config.RateLimit.RequestsPerMinute) / 60.0)
		burstSize := config.RateLimit.BurstSize
		if burstSize <= 0 {
			burstSize = 1
		}
		s.rateLimiter = rate.NewLimiter(ratePerSecond, burstSize)
	}

	// Create HTTP handler with minimal routing
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook/prometheus", s.handlePrometheusWebhook)
	mux.HandleFunc("/health", s.handleHealth)

	s.handler = mux
	return s
}

// GetHTTPHandler returns the HTTP handler for the service
func (s *service) GetHTTPHandler() http.Handler {
	return s.handler
}

// Start starts the Gateway service
func (s *service) Start(ctx context.Context) error {
	// Minimal implementation - will be enhanced in REFACTOR phase
	s.logger.Info("Gateway service started")
	return nil
}

// Stop stops the Gateway service
func (s *service) Stop(ctx context.Context) error {
	// Minimal implementation - will be enhanced in REFACTOR phase
	s.logger.Info("Gateway service stopped")
	return nil
}

// GetAccessLogs returns access logs for testing
func (s *service) GetAccessLogs() []string {
	return s.accessLogs
}

// GetSecurityLogs returns security logs for testing
func (s *service) GetSecurityLogs() []string {
	return s.securityLogs
}

// handlePrometheusWebhook handles Prometheus webhook requests
// Business Requirements: BR-WH-001, BR-WH-003, BR-WH-011, BR-WH-026
// REFACTOR: Enhanced implementation reusing existing webhook handler patterns
func (s *service) handlePrometheusWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Enhanced logging following existing patterns
	s.logger.WithFields(logrus.Fields{
		"method":     r.Method,
		"url":        r.URL.Path,
		"remote_ip":  r.RemoteAddr,
		"user_agent": r.UserAgent(),
	}).Debug("Received webhook request")

	// BR-WH-023: Rate limiting (REFACTOR: enhanced with per-IP tracking and better logging)
	if s.rateLimiter != nil {
		clientIP := s.extractClientIP(r)

		// REFACTOR: Check both global and per-IP rate limits
		if !s.checkRateLimit(clientIP) {
			s.logger.WithFields(logrus.Fields{
				"remote_ip":    r.RemoteAddr,
				"client_ip":    clientIP,
				"user_agent":   r.UserAgent(),
				"request_path": r.URL.Path,
			}).Warn("Rate limit exceeded")

			// REFACTOR: Enhanced security logging for rate limiting
			s.securityLogs = append(s.securityLogs, fmt.Sprintf("rate_limit_exceeded: %s from %s (UA: %s)",
				clientIP, r.RemoteAddr, r.UserAgent()))

			s.sendError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			metrics.RecordWebhookRequest("error")
			return
		}
	}

	// BR-WH-004: OAuth2/JWT Authentication using Kubernetes TokenReview API
	if s.authenticator != nil {
		authResult, err := s.authenticator.Authenticate(r.Context(), r)
		if err != nil || (authResult != nil && !authResult.Authenticated) {
			s.logger.WithError(err).WithField("remote_ip", r.RemoteAddr).Warn("OAuth2/JWT authentication failed")

			errorMsg := "OAuth2/JWT authentication failed"
			if err != nil {
				errorMsg = err.Error()
			}
			if authResult != nil && len(authResult.Errors) > 0 {
				errorMsg = strings.Join(authResult.Errors, "; ")
			}

			s.securityLogs = append(s.securityLogs, fmt.Sprintf("oauth2_authentication_failed: %s from %s", errorMsg, r.RemoteAddr))

			// Determine appropriate status code based on error type
			statusCode := http.StatusUnauthorized
			if strings.Contains(errorMsg, "namespace mismatch") || strings.Contains(errorMsg, "ServiceAccount mismatch") {
				statusCode = http.StatusForbidden
			} else if strings.Contains(errorMsg, "TokenReview API call failed") {
				statusCode = http.StatusInternalServerError
			}

			s.sendError(w, statusCode, "OAuth2/JWT authentication required")
			metrics.RecordWebhookRequest("error")
			return
		}

		// Log successful OAuth2/JWT authentication
		s.logger.WithFields(logrus.Fields{
			"auth_type": "oauth2",
			"username":  authResult.Username,
			"namespace": authResult.Namespace,
			"groups":    authResult.Groups,
			"remote_ip": r.RemoteAddr,
		}).Debug("OAuth2/JWT authentication successful")
	}

	// BR-WH-003: Validate HTTP method (reusing existing validation)
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		metrics.RecordWebhookRequest("error")
		return
	}

	// BR-WH-003: Enhanced content type validation (reusing existing pattern)
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		s.sendError(w, http.StatusBadRequest, "Content-Type must be application/json")
		metrics.RecordWebhookRequest("error")
		return
	}

	// BR-WH-003: Enhanced payload reading and validation (REFACTOR: improved error handling)
	body, err := s.readAndValidateBody(r)
	if err != nil {
		s.logger.WithError(err).WithField("content_length", r.ContentLength).Error("Failed to read request body")
		s.sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to read request body: %v", err))
		metrics.RecordWebhookRequest("error")
		return
	}

	// REFACTOR: Enhanced AlertManager webhook payload parsing with better error context
	webhook, err := s.parseWebhookPayload(body)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"body_length":  len(body),
			"body_preview": s.getBodyPreview(body),
		}).Error("Failed to parse webhook payload")
		s.sendError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON payload: %v", err))
		metrics.RecordWebhookRequest("error")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"version":    webhook.Version,
		"group_key":  webhook.GroupKey,
		"status":     webhook.Status,
		"receiver":   webhook.Receiver,
		"num_alerts": len(webhook.Alerts),
	}).Info("Received AlertManager webhook")

	// BR-WH-026: Enhanced Alert Processor forwarding with BR-WH-009: Timeout handling
	ctx := r.Context()

	// Apply timeout if configured
	if s.config.AlertProcessor.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.AlertProcessor.Timeout)
		defer cancel()
	}

	if err := s.processWebhookAlerts(ctx, webhook); err != nil {
		// Check if error is due to timeout
		if ctx.Err() == context.DeadlineExceeded {
			s.logger.WithError(err).WithField("timeout", s.config.AlertProcessor.Timeout).Warn("Request timed out")
			s.sendError(w, http.StatusGatewayTimeout, fmt.Sprintf("Request timed out after %v", s.config.AlertProcessor.Timeout))
			metrics.RecordWebhookRequest("timeout")
			return
		}

		s.logger.WithError(err).Error("Failed to process alerts")
		s.sendError(w, http.StatusInternalServerError, "Failed to process alerts")
		metrics.RecordWebhookRequest("error")
		return
	}

	// BR-WH-011: Enhanced success response (reusing existing pattern)
	response := WebhookResponse{
		Status:  "success",
		Message: fmt.Sprintf("Successfully processed %d alerts", len(webhook.Alerts)),
	}

	s.sendJSONResponse(w, http.StatusOK, response)
	metrics.RecordWebhookRequest("success")

	s.logger.WithFields(logrus.Fields{
		"num_alerts": len(webhook.Alerts),
		"duration":   time.Since(start),
	}).Info("Webhook request processed successfully")

	// BR-WH-025: Access logging
	if s.config.Logging.AccessLog {
		s.accessLogs = append(s.accessLogs, fmt.Sprintf("SUCCESS: %s %s from %s - %d alerts processed in %v",
			r.Method, r.URL.Path, r.RemoteAddr, len(webhook.Alerts), time.Since(start)))
	}
}

// handleHealth handles health check requests
// Business Requirements: BR-WH-011 (appropriate HTTP responses)
func (s *service) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":  "healthy",
		"message": "Gateway service is running",
	}
	s.sendJSONResponse(w, http.StatusOK, response)
}

// sendError sends an error response
func (s *service) sendError(w http.ResponseWriter, statusCode int, message string) {
	response := map[string]interface{}{
		"status": "error",
		"error":  message,
	}
	s.sendJSONResponse(w, statusCode, response)
}

// sendSuccess sends a success response
func (s *service) sendSuccess(w http.ResponseWriter, message string) {
	response := map[string]interface{}{
		"status":  "success",
		"message": message,
	}
	s.sendJSONResponse(w, http.StatusOK, response)
}

// sendJSONResponse sends a JSON response
// REFACTOR: Enhanced with proper JSON encoding and error handling
func (s *service) sendJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gateway-Version", "1.0")
	w.WriteHeader(statusCode)

	// REFACTOR: Use proper JSON marshaling instead of hardcoded strings
	jsonData, err := json.Marshal(data)
	if err != nil {
		s.logger.WithError(err).Error("Failed to marshal JSON response")
		// Fallback to simple error response
		fallbackResponse := `{"status":"error","error":"Internal server error"}`
		if _, writeErr := w.Write([]byte(fallbackResponse)); writeErr != nil {
			s.logger.WithError(writeErr).Error("Failed to write fallback response")
		}
		return
	}

	if _, err := w.Write(jsonData); err != nil {
		s.logger.WithError(err).Error("Failed to write JSON response")
	}
}

// processWebhookAlerts processes alerts from AlertManager webhook
// Reusing existing pattern from pkg/integration/webhook/handler.go
func (s *service) processWebhookAlerts(ctx context.Context, webhook *AlertManagerWebhook) error {
	var processingErrors []error

	for i, alert := range webhook.Alerts {
		slmAlert := s.convertToSLMAlert(alert, webhook)

		s.logger.WithFields(logrus.Fields{
			"alert_index": i,
			"alert_name":  slmAlert.Name,
			"status":      slmAlert.Status,
			"severity":    slmAlert.Severity,
		}).Debug("Processing alert")

		// Record alert processing (reusing existing metrics)
		metrics.RecordAlert()

		if err := s.alertClient.ForwardAlert(ctx, slmAlert); err != nil {
			s.logger.WithFields(logrus.Fields{
				"alert_name": slmAlert.Name,
				"error":      err,
			}).Error("Failed to process individual alert")
			processingErrors = append(processingErrors, fmt.Errorf("alert %s: %w", slmAlert.Name, err))
			// Continue processing other alerts even if one fails
			continue
		}
	}

	// Return error if any alerts failed to process
	if len(processingErrors) > 0 {
		return fmt.Errorf("failed to process %d/%d alerts: %v", len(processingErrors), len(webhook.Alerts), processingErrors)
	}

	return nil
}

// convertToSLMAlert converts AlertManager alert to SLM alert format
// Reusing existing conversion logic from pkg/integration/webhook/handler.go
func (s *service) convertToSLMAlert(alert AlertManagerAlert, webhook *AlertManagerWebhook) *types.Alert {
	// Extract alert name from labels
	alertName := "unknown"
	if name, ok := alert.Labels["alertname"]; ok {
		alertName = name
	}

	// Extract namespace
	namespace := "default"
	if ns, ok := alert.Labels["namespace"]; ok {
		namespace = ns
	}

	// Extract severity
	severity := "info"
	if sev, ok := alert.Labels["severity"]; ok {
		severity = sev
	}

	// Extract resource information
	resource := ""
	if pod, ok := alert.Labels["pod"]; ok {
		resource = pod
	} else if deployment, ok := alert.Labels["deployment"]; ok {
		resource = deployment
	}

	return &types.Alert{
		Name:        alertName,
		Status:      alert.Status,
		Namespace:   namespace,
		Severity:    severity,
		Resource:    resource,
		Labels:      alert.Labels,
		Annotations: alert.Annotations,
		StartsAt:    alert.StartsAt,
		EndsAt:      alert.EndsAt,
	}
}

// authenticate validates the request authentication
// Reusing existing authentication pattern from pkg/integration/webhook/handler.go
func (s *service) authenticate(r *http.Request) error {
	// Skip authentication if no authenticator is configured
	if s.authenticator == nil {
		return nil
	}

	// Use the pluggable authenticator interface
	authResult, err := s.authenticator.Authenticate(r.Context(), r)
	if err != nil {
		return fmt.Errorf("authentication error: %w", err)
	}

	if !authResult.Authenticated {
		if len(authResult.Errors) > 0 {
			return fmt.Errorf("authentication failed: %s", strings.Join(authResult.Errors, "; "))
		}
		return fmt.Errorf("authentication failed")
	}

	return nil
}

// readAndValidateBody reads and validates the request body
// REFACTOR: Enhanced body reading with size limits and validation
func (s *service) readAndValidateBody(r *http.Request) ([]byte, error) {
	// REFACTOR: Add reasonable size limit for webhook payloads
	const maxBodySize = 1024 * 1024 // 1MB limit

	if r.ContentLength > maxBodySize {
		return nil, fmt.Errorf("request body too large: %d bytes (max %d)", r.ContentLength, maxBodySize)
	}

	// REFACTOR: Use LimitReader to prevent memory exhaustion
	limitedReader := io.LimitReader(r.Body, maxBodySize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// REFACTOR: Ensure body is closed properly
	if err := r.Body.Close(); err != nil {
		s.logger.WithError(err).Warn("Failed to close request body")
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	return body, nil
}

// parseWebhookPayload parses the AlertManager webhook payload
// REFACTOR: Enhanced parsing with better error context
func (s *service) parseWebhookPayload(body []byte) (*AlertManagerWebhook, error) {
	var webhook AlertManagerWebhook

	if err := json.Unmarshal(body, &webhook); err != nil {
		return nil, fmt.Errorf("JSON unmarshal failed: %w", err)
	}

	// REFACTOR: Basic payload validation
	if len(webhook.Alerts) == 0 {
		return nil, fmt.Errorf("webhook contains no alerts")
	}

	// REFACTOR: Validate alert structure
	for i, alert := range webhook.Alerts {
		if alert.Labels == nil {
			return nil, fmt.Errorf("alert %d missing labels", i)
		}
		if alert.Status == "" {
			return nil, fmt.Errorf("alert %d missing status", i)
		}
	}

	return &webhook, nil
}

// getBodyPreview returns a safe preview of the request body for logging
// REFACTOR: Safe logging helper to prevent log injection
func (s *service) getBodyPreview(body []byte) string {
	const maxPreviewLength = 100

	if len(body) == 0 {
		return "<empty>"
	}

	preview := string(body)
	if len(preview) > maxPreviewLength {
		preview = preview[:maxPreviewLength] + "..."
	}

	// REFACTOR: Sanitize for safe logging (remove newlines and control chars)
	preview = strings.ReplaceAll(preview, "\n", "\\n")
	preview = strings.ReplaceAll(preview, "\r", "\\r")
	preview = strings.ReplaceAll(preview, "\t", "\\t")

	return preview
}

// extractClientIP extracts the real client IP from the request
// REFACTOR: Enhanced IP extraction with X-Forwarded-For support
func (s *service) extractClientIP(r *http.Request) string {
	// REFACTOR: Check X-Forwarded-For header first (for load balancers/proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// REFACTOR: Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// REFACTOR: Fall back to RemoteAddr, strip port if present
	ip := r.RemoteAddr
	if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
		ip = ip[:colonIndex]
	}

	return ip
}

// checkRateLimit checks both global and per-IP rate limits
// REFACTOR: Enhanced rate limiting with per-IP tracking
func (s *service) checkRateLimit(clientIP string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// REFACTOR: Check global rate limit first
	if !s.rateLimiter.Allow() {
		return false
	}

	// REFACTOR: Get or create per-IP rate limiter
	ipLimiter, exists := s.ipRateLimiters[clientIP]
	if !exists {
		// REFACTOR: Create per-IP limiter with same settings as global
		ratePerSecond := rate.Limit(float64(s.config.RateLimit.RequestsPerMinute) / 60.0)
		burstSize := s.config.RateLimit.BurstSize
		if burstSize <= 0 {
			burstSize = 1
		}
		ipLimiter = rate.NewLimiter(ratePerSecond, burstSize)
		s.ipRateLimiters[clientIP] = ipLimiter

		// REFACTOR: Cleanup old IP limiters to prevent memory leak
		if len(s.ipRateLimiters) > 1000 { // Reasonable limit
			s.cleanupOldIPLimiters()
		}
	}

	// REFACTOR: Check per-IP rate limit
	return ipLimiter.Allow()
}

// cleanupOldIPLimiters removes inactive IP rate limiters to prevent memory leaks
// REFACTOR: Memory management for per-IP rate limiters
func (s *service) cleanupOldIPLimiters() {
	// REFACTOR: Simple cleanup - remove half of the limiters
	// In production, this could be more sophisticated with LRU or time-based cleanup
	count := 0
	maxToKeep := len(s.ipRateLimiters) / 2

	for ip := range s.ipRateLimiters {
		if count >= maxToKeep {
			delete(s.ipRateLimiters, ip)
		}
		count++
	}

	s.logger.WithFields(logrus.Fields{
		"cleaned_limiters": len(s.ipRateLimiters) - maxToKeep,
		"remaining":        len(s.ipRateLimiters),
	}).Debug("Cleaned up old IP rate limiters")
}
