package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/integration/processor"
	"github.com/jordigilh/kubernaut/pkg/integration/webhook"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	// Load configuration
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		configFile = "config/development.yaml"
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
	}

	// Set log level
	level, err := logrus.ParseLevel(cfg.Logging.Level)
	if err != nil {
		logger.WithError(err).Warn("Invalid log level, using info")
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.WithFields(logrus.Fields{
		"service": "gateway-service",
		"version": cfg.App.Version,
		"port":    cfg.Webhook.Port,
	}).Info("Starting gateway service")

	// Create HTTP processor client
	processorServiceURL := os.Getenv("PROCESSOR_SERVICE_URL")
	if processorServiceURL == "" {
		processorServiceURL = "http://processor-service:8095"
	}

	processorClient := processor.NewHTTPProcessorClient(processorServiceURL, logger)

	logger.WithField("processor_url", processorServiceURL).Info("Created HTTP processor client")

	// Create webhook handler
	webhookHandler := webhook.NewHandler(processorClient, cfg.Webhook, logger)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/alerts", webhookHandler.HandleAlert)
	mux.HandleFunc("/health", webhookHandler.HealthCheck)
	mux.HandleFunc("/metrics", handleMetrics)

	server := &http.Server{
		Addr:         ":" + cfg.Webhook.Port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.WithField("port", cfg.Webhook.Port).Info("Starting gateway HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Fatal("Failed to start HTTP server")
		}
	}()

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.WithField("signal", sig).Info("Received shutdown signal")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.WithError(err).Error("Failed to shutdown server gracefully")
	} else {
		logger.Info("Server shutdown complete")
	}
}

// handleMetrics provides basic metrics endpoint
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	// Basic metrics - in production this would use Prometheus metrics
	metrics := `# HELP webhook_requests_total Total number of webhook requests
# TYPE webhook_requests_total counter
webhook_requests_total 0

# HELP webhook_service_up Service availability
# TYPE webhook_service_up gauge
webhook_service_up 1
`

	fmt.Fprint(w, metrics)
}
