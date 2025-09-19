package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

type Handler interface {
	HandleAlert(w http.ResponseWriter, r *http.Request)
}

type handler struct {
	processor processor.Processor
	config    config.WebhookConfig
	log       *logrus.Logger
}

// AlertManagerWebhook represents the AlertManager webhook payload structure
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
type WebhookResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewHandler(processor processor.Processor, cfg config.WebhookConfig, log *logrus.Logger) Handler {
	return &handler{
		processor: processor,
		config:    cfg,
		log:       log,
	}
}

func (h *handler) HandleAlert(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	h.log.WithFields(logrus.Fields{
		"method":     r.Method,
		"url":        r.URL.Path,
		"remote_ip":  r.RemoteAddr,
		"user_agent": r.UserAgent(),
	}).Debug("Received webhook request")

	// Validate HTTP method
	if r.Method != http.MethodPost {
		h.sendError(w, http.StatusMethodNotAllowed, "Only POST method is allowed")
		metrics.RecordWebhookRequest("error")
		return
	}

	// Validate content type
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		h.sendError(w, http.StatusBadRequest, "Content-Type must be application/json")
		metrics.RecordWebhookRequest("error")
		return
	}

	// Authenticate request
	if err := h.authenticate(r); err != nil {
		h.log.WithError(err).Warn("Authentication failed")
		h.sendError(w, http.StatusUnauthorized, "Authentication failed")
		metrics.RecordWebhookRequest("error")
		return
	}

	// Read and parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.WithError(err).Error("Failed to read request body")
		h.sendError(w, http.StatusBadRequest, "Failed to read request body")
		metrics.RecordWebhookRequest("error")
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			h.log.WithError(err).Error("Failed to close request body")
		}
	}()

	var webhook AlertManagerWebhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		h.log.WithError(err).Error("Failed to parse webhook payload")
		h.sendError(w, http.StatusBadRequest, "Invalid JSON payload")
		metrics.RecordWebhookRequest("error")
		return
	}

	h.log.WithFields(logrus.Fields{
		"version":    webhook.Version,
		"group_key":  webhook.GroupKey,
		"status":     webhook.Status,
		"receiver":   webhook.Receiver,
		"num_alerts": len(webhook.Alerts),
	}).Info("Received AlertManager webhook")

	// Process alerts
	ctx := r.Context()
	if err := h.processWebhookAlerts(ctx, &webhook); err != nil {
		h.log.WithError(err).Error("Failed to process alerts")
		h.sendError(w, http.StatusInternalServerError, "Failed to process alerts")
		metrics.RecordWebhookRequest("error")
		return
	}

	// Send success response
	response := WebhookResponse{
		Status:  "success",
		Message: fmt.Sprintf("Successfully processed %d alerts", len(webhook.Alerts)),
	}

	h.sendResponse(w, http.StatusOK, response)

	// Record successful webhook request
	metrics.RecordWebhookRequest("success")

	h.log.WithFields(logrus.Fields{
		"num_alerts": len(webhook.Alerts),
		"duration":   time.Since(start),
	}).Info("Webhook request processed successfully")
}

func (h *handler) authenticate(r *http.Request) error {
	if h.config.Auth.Type == "" || h.config.Auth.Token == "" {
		// No authentication configured
		return nil
	}

	switch h.config.Auth.Type {
	case "bearer":
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return fmt.Errorf("missing Authorization header")
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return fmt.Errorf("invalid Authorization header format")
		}

		if parts[1] != h.config.Auth.Token {
			return fmt.Errorf("invalid bearer token")
		}

		return nil

	default:
		return fmt.Errorf("unsupported authentication type: %s", h.config.Auth.Type)
	}
}

func (h *handler) processWebhookAlerts(ctx context.Context, webhook *AlertManagerWebhook) error {
	var processingErrors []error

	for i, alert := range webhook.Alerts {
		slmAlert := h.convertToSLMAlert(alert, webhook)

		h.log.WithFields(logrus.Fields{
			"alert_index": i,
			"alert_name":  slmAlert.Name,
			"status":      slmAlert.Status,
			"severity":    slmAlert.Severity,
		}).Debug("Processing alert")

		// Record alert processing
		metrics.RecordAlert()

		if err := h.processor.ProcessAlert(ctx, slmAlert); err != nil {
			h.log.WithFields(logrus.Fields{
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

func (h *handler) convertToSLMAlert(alert AlertManagerAlert, _ *AlertManagerWebhook) types.Alert {
	// Extract alert name
	alertName := alert.Labels["alertname"]
	if alertName == "" {
		alertName = "unknown"
	}

	// Extract severity
	severity := alert.Labels["severity"]
	if severity == "" {
		severity = "info"
	}

	// Extract namespace
	namespace := alert.Labels["namespace"]
	if namespace == "" {
		namespace = alert.Labels["exported_namespace"]
	}
	if namespace == "" {
		namespace = "default"
	}

	// Extract resource information
	resource := ""
	if pod := alert.Labels["pod"]; pod != "" {
		resource = pod
	} else if deployment := alert.Labels["deployment"]; deployment != "" {
		resource = deployment
	} else if service := alert.Labels["service"]; service != "" {
		resource = service
	}

	// Extract description
	description := alert.Annotations["description"]
	if description == "" {
		description = alert.Annotations["summary"]
	}
	if description == "" {
		description = fmt.Sprintf("Alert %s is %s", alertName, alert.Status)
	}

	return types.Alert{
		Name:        alertName,
		Status:      alert.Status,
		Severity:    severity,
		Description: description,
		Namespace:   namespace,
		Resource:    resource,
		Labels:      alert.Labels,
		Annotations: alert.Annotations,
		StartsAt:    alert.StartsAt,
		EndsAt:      alert.EndsAt,
	}
}

func (h *handler) sendError(w http.ResponseWriter, statusCode int, message string) {
	response := WebhookResponse{
		Status: "error",
		Error:  message,
	}
	h.sendResponse(w, statusCode, response)
}

func (h *handler) sendResponse(w http.ResponseWriter, statusCode int, response WebhookResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.WithError(err).Error("Failed to encode response")
	}
}

// HealthCheck provides a simple health check endpoint
func (h *handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := WebhookResponse{
		Status:  "healthy",
		Message: "Webhook handler is running",
	}
	h.sendResponse(w, http.StatusOK, response)
}

// ValidateWebhook validates webhook configuration
func ValidateWebhookConfig(cfg config.WebhookConfig) error {
	if cfg.Port == "" {
		return fmt.Errorf("webhook port is required")
	}

	if cfg.Path == "" {
		return fmt.Errorf("webhook path is required")
	}

	if cfg.Auth.Type != "" && cfg.Auth.Token == "" {
		return fmt.Errorf("authentication token is required when auth type is specified")
	}

	return nil
}
