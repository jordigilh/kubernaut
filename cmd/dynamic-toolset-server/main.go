package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/api/server"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// DynamicToolsetServer demonstrates complete integration of dynamic toolset configuration
// Business Requirement: BR-HOLMES-025 - Runtime toolset management API
func main() {
	// Initialize logger
	log := logrus.New()
	log.SetFormatter(&logrus.JSONFormatter{})
	log.SetLevel(logrus.InfoLevel)

	log.Info("🚀 Starting Dynamic Toolset Configuration Server")

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("📡 Received shutdown signal")
		cancel()
	}()

	// Initialize components
	if err := runServer(ctx, log); err != nil {
		log.WithError(err).Fatal("❌ Server failed")
	}

	log.Info("✅ Dynamic Toolset Configuration Server shutdown complete")
}

func runServer(parentCtx context.Context, log *logrus.Logger) error {
	// Create a cancellable context for this function
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	// 1. Initialize Kubernetes client
	// Business Requirement: BR-HOLMES-016 - Dynamic service discovery in Kubernetes cluster
	kubeConfig := os.Getenv("KUBECONFIG")
	if kubeConfig == "" {
		kubeConfig = clientcmd.RecommendedHomeFile
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		// Fallback to in-cluster config
		config, err = clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return fmt.Errorf("failed to build kubernetes config: %w", err)
		}
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	log.Info("✅ Kubernetes client initialized")

	// 2. Initialize Service Discovery Configuration
	// Business Requirement: BR-HOLMES-017 - Automatic detection of well-known services
	serviceDiscoveryConfig := &k8s.ServiceDiscoveryConfig{
		DiscoveryInterval:   5 * time.Minute,
		CacheTTL:            10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		Enabled:             true,
		Namespaces:          []string{"monitoring", "observability", "kube-system"}, // BR-HOLMES-027
		ServicePatterns:     k8s.GetDefaultServicePatterns(),
	}

	// 3. Create Service Integration (wires ServiceDiscovery + DynamicToolsetManager)
	// Business Requirement: BR-HOLMES-022 - Generate appropriate toolset configurations
	serviceIntegration, err := holmesgpt.NewServiceIntegration(k8sClient, serviceDiscoveryConfig, log)
	if err != nil {
		return fmt.Errorf("failed to create service integration: %w", err)
	}

	log.Info("✅ Service Integration created")

	// 4. Start Service Integration (this starts both ServiceDiscovery and DynamicToolsetManager)
	// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
	if err := serviceIntegration.Start(ctx); err != nil {
		return fmt.Errorf("failed to start service integration: %w", err)
	}

	log.Info("✅ Service Integration started")

	// 5. Initialize AI Service Integrator (minimal setup for demonstration)
	// Note: In production, this would be properly configured with LLM clients, etc.
	// For this demo, we'll pass nil since Context API can work without AI integrator for toolset endpoints
	var aiIntegrator *engine.AIServiceIntegrator // Will be nil, but that's OK for toolset endpoints

	// 6. Create Context API Server with Service Integration
	// Business Requirement: BR-HAPI-022 - Provide /api/v1/toolsets endpoint
	contextAPIConfig := server.ContextAPIConfig{
		Host:    "0.0.0.0",
		Port:    8091,
		Timeout: 30 * time.Second,
	}

	// Architecture: Context API serves data TO HolmesGPT (Python service), no direct client needed
	contextAPIServer := server.NewContextAPIServer(contextAPIConfig, aiIntegrator, serviceIntegration, log)

	log.Info("✅ Context API Server created")

	// 7. Start Context API Server in goroutine
	go func() {
		log.WithField("address", "http://localhost:8091").Info("🌐 Starting Context API Server")
		if err := contextAPIServer.Start(); err != nil {
			log.WithError(err).Error("❌ Context API Server failed")
			cancel() // This should work now since cancel is defined in main()
		}
	}()

	// 8. Wait for initial toolset generation
	log.Info("⏳ Waiting for initial toolset generation...")
	time.Sleep(2 * time.Second)

	// 9. Display current status
	displayStatus(serviceIntegration, log)

	// 10. Wait for shutdown
	<-ctx.Done()

	// 11. Graceful shutdown
	log.Info("🛑 Shutting down components...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := contextAPIServer.Stop(shutdownCtx); err != nil {
		log.WithError(err).Error("❌ Context API Server shutdown error")
	}

	serviceIntegration.Stop()

	return nil
}

// displayStatus shows the current system status
func displayStatus(serviceIntegration holmesgpt.ServiceIntegrationInterface, log *logrus.Logger) {
	toolsets := serviceIntegration.GetAvailableToolsets()
	toolsetStats := serviceIntegration.GetToolsetStats()
	discoveryStats := serviceIntegration.GetServiceDiscoveryStats()
	healthStatus := serviceIntegration.GetHealthStatus()

	log.WithFields(logrus.Fields{
		"total_toolsets":      toolsetStats.TotalToolsets,
		"enabled_toolsets":    toolsetStats.EnabledCount,
		"discovered_services": discoveryStats.TotalServices,
		"available_services":  discoveryStats.AvailableServices,
		"system_healthy":      healthStatus.Healthy,
	}).Info("📊 Dynamic Toolset Configuration Status")

	log.Info("🛠️ Available Toolsets:")
	for _, toolset := range toolsets {
		log.WithFields(logrus.Fields{
			"name":         toolset.Name,
			"service_type": toolset.ServiceType,
			"enabled":      toolset.Enabled,
			"capabilities": len(toolset.Capabilities),
		}).Info("   - Toolset")
	}

	log.WithFields(logrus.Fields{
		"endpoint": "http://localhost:8091/api/v1/toolsets",
	}).Info("🌐 Toolsets available via API")

	log.WithFields(logrus.Fields{
		"endpoint": "http://localhost:8091/api/v1/service-discovery",
	}).Info("🔍 Service discovery status available via API")
}
