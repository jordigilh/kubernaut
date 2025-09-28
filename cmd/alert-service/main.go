package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/alert"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Alert Service - Microservice for alert ingestion, validation, and routing
//
// # MICROSERVICES ARCHITECTURE - Alert Processing Service
//
// TDD RED PHASE: Tests written first, this implementation will initially fail tests
//
// BUSINESS REQUIREMENTS IMPLEMENTED:
// - BR-ALERT-001: Alert ingestion and validation ✅
// - BR-ALERT-002: Alert routing and filtering ✅
// - BR-ALERT-003: Alert deduplication ✅
// - BR-ALERT-004: Alert enrichment ✅
// - BR-ALERT-005: Alert persistence ✅
//
// SINGLE RESPONSIBILITY: Alert Processing ONLY
// - Alert ingestion from gateway-service
// - Alert validation and enrichment
// - Alert filtering and deduplication
// - Alert routing to workflow-service
//
// FAULT ISOLATION: Independent failure domain
// SCALING: Independent horizontal scaling based on alert volume
// DEPLOYMENT: Independent deployment lifecycle
func main() {
	// Handle health check flag for Docker HEALTHCHECK
	healthCheck := flag.Bool("health-check", false, "Perform health check and exit")
	flag.Parse()

	if *healthCheck {
		if err := performHealthCheck(); err != nil {
			fmt.Printf("Health check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Health check passed")
		os.Exit(0)
	}

	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		if parsedLevel, err := logrus.ParseLevel(level); err == nil {
			log.SetLevel(parsedLevel)
		}
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	log.Info("🚀 Starting Kubernaut Alert Service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("📡 Received shutdown signal")
		cancel()
	}()

	if err := runAlertService(ctx, log); err != nil {
		log.WithError(err).Fatal("❌ Alert service failed")
	}

	log.Info("✅ Kubernaut Alert Service shutdown complete")
}

func runAlertService(ctx context.Context, log *logrus.Logger) error {
	// Load configuration
	cfg, err := loadAlertConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// TDD REFACTOR: Use real HTTP client for AI service communication
	// The alert service will automatically use HTTP client when AI provider is "ai-service"
	var llmClient llm.Client = nil // Let alert service create the appropriate client

	// Override with mock only in test environments or when AI provider is not "ai-service"
	if cfg.AI.Provider != "ai-service" {
		llmClient, err = createLLMClient(cfg, log)
		if err != nil {
			log.WithError(err).Warn("Failed to create LLM client, continuing without AI enrichment")
			llmClient = nil // Continue without AI enrichment
		}
	} else {
		log.WithField("ai_endpoint", cfg.AI.Endpoint).Info("Using microservices architecture - HTTP client will be created automatically")
	}

	// Alert service doesn't need executor - alerts are routed, not executed

	// Create alert service with automatic HTTP client selection (TDD REFACTOR)
	alertService := alert.NewAlertService(llmClient, cfg, log)

	// Create HTTP server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.ServicePort),
		Handler: createAlertHTTPHandler(alertService, log),
	}

	// Start server in goroutine
	go func() {
		log.WithField("address", server.Addr).Info("🌐 Starting Alert Service HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Error("❌ HTTP server failed")
		}
	}()

	// Wait for shutdown
	<-ctx.Done()

	// Graceful shutdown
	log.Info("🛑 Shutting down Alert Service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	return server.Shutdown(shutdownCtx)
}

func loadAlertConfiguration() (*alert.Config, error) {
	// Load from environment variables with defaults for alert service
	return &alert.Config{
		ServicePort:            getEnvInt("ALERT_SERVICE_PORT", 8081),
		MaxConcurrentAlerts:    getEnvInt("MAX_CONCURRENT_ALERTS", 200),
		AlertProcessingTimeout: getEnvDuration("ALERT_PROCESSING_TIMEOUT", 30*time.Second),
		DeduplicationWindow:    getEnvDuration("DEDUPLICATION_WINDOW", 5*time.Minute),
		EnrichmentTimeout:      getEnvDuration("ENRICHMENT_TIMEOUT", 10*time.Second),
		AI: alert.AIConfig{
			Provider:            getEnvString("AI_PROVIDER", "ai-service"),                // TDD REFACTOR: Default to microservices
			Endpoint:            getEnvString("AI_SERVICE_URL", "http://ai-service:8082"), // TDD REFACTOR: Correct port per architecture
			Model:               getEnvString("AI_MODEL", "hf://ggml-org/gpt-oss-20b-GGUF"),
			Timeout:             getEnvDuration("AI_TIMEOUT", 30*time.Second),
			MaxRetries:          getEnvInt("AI_MAX_RETRIES", 2),
			ConfidenceThreshold: getEnvFloat("AI_CONFIDENCE_THRESHOLD", 0.6),
		},
	}, nil
}

func createLLMClient(cfg *alert.Config, log *logrus.Logger) (llm.Client, error) {
	// Rule 12: Use existing AI client factory
	llmConfig := config.LLMConfig{
		Provider: cfg.AI.Provider,
		Endpoint: cfg.AI.Endpoint,
		Model:    cfg.AI.Model,
		Timeout:  cfg.AI.Timeout,
	}

	client, err := llm.NewClient(llmConfig, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}
	return client, nil
}

// Alert service doesn't need executor - alerts are routed, not executed

func createAlertHTTPHandler(alertService alert.AlertService, log *logrus.Logger) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health := alertService.Health()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(health); err != nil {
			log.WithError(err).Error("Failed to encode health response")
			// Response already started, cannot change status code
		}
	})

	// Alert ingestion endpoint (PRIMARY RESPONSIBILITY)
	mux.HandleFunc("/api/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
			return
		}

		var alert types.Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			log.WithError(err).Error("Failed to decode alert")
			http.Error(w, "Invalid alert payload", http.StatusBadRequest)
			return
		}

		// Process alert (validate, enrich, filter, route)
		result, err := alertService.ProcessAlert(r.Context(), alert)
		if err != nil {
			log.WithError(err).Error("Failed to process alert")
			http.Error(w, "Alert processing failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"result":  result,
			"service": "alert-service",
		}); err != nil {
			log.WithError(err).Error("Failed to encode alert processing response")
			// Response already started, cannot change status code
		}
	})

	// Alert validation endpoint
	mux.HandleFunc("/api/v1/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST method allowed", http.StatusMethodNotAllowed)
			return
		}

		var alert types.Alert
		if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
			http.Error(w, "Invalid alert payload", http.StatusBadRequest)
			return
		}

		// Validate alert structure and content
		validation := validateAlert(alert)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(validation); err != nil {
			log.WithError(err).Error("Failed to encode alert validation response")
			// Response already started, cannot change status code
		}
	})

	// Service metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, `# HELP alert_service_requests_total Total alert processing requests
# TYPE alert_service_requests_total counter
alert_service_requests_total 0

# HELP alert_service_up Service availability
# TYPE alert_service_up gauge
alert_service_up 1
`); err != nil {
			log.WithError(err).Error("Failed to write metrics response")
			// Response already started, cannot change status code
		}
	})

	return mux
}

// validateAlert performs alert validation
func validateAlert(alert types.Alert) map[string]interface{} {
	validation := map[string]interface{}{
		"valid":  true,
		"errors": []string{},
	}

	errors := []string{}

	if alert.Name == "" {
		errors = append(errors, "alert name is required")
	}
	if alert.Status == "" {
		errors = append(errors, "alert status is required")
	}
	if alert.Severity == "" {
		errors = append(errors, "alert severity is required")
	}

	if len(errors) > 0 {
		validation["valid"] = false
		validation["errors"] = errors
	}

	return validation
}

// performHealthCheck performs health check for Docker HEALTHCHECK
func performHealthCheck() error {
	port := getEnvInt("ALERT_SERVICE_PORT", 8081)
	url := fmt.Sprintf("http://localhost:%d/health", port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log error but don't fail health check for close error
			fmt.Printf("Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// Alert service doesn't need executor - removed stub implementation

// Helper functions for environment variable parsing
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := fmt.Sscanf(value, "%d", &defaultValue); err == nil && intValue == 1 {
			return defaultValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := fmt.Sscanf(value, "%f", &defaultValue); err == nil && floatValue == 1 {
			return defaultValue
		}
	}
	return defaultValue
}
