package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client combines basic and advanced Kubernetes operations
type Client interface {
	BasicClient
	AdvancedClient
}

func NewClient(cfg config.KubernetesConfig, log *logrus.Logger) (Client, error) {
	var k8sConfig *rest.Config
	var err error

	// Try to load config from the specified context or default locations
	if cfg.Context != "" {
		// Use specified context from default kubeconfig
		kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
		k8sConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
			&clientcmd.ConfigOverrides{CurrentContext: cfg.Context},
		).ClientConfig()
	} else {
		// Try in-cluster config first, then default kubeconfig
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			// Fallback to default kubeconfig
			kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	log.WithFields(logrus.Fields{
		"namespace": cfg.Namespace,
		"context":   cfg.Context,
	}).Info("Kubernetes client initialized")

	// Use unified client implementation
	client := NewUnifiedClient(clientset, cfg, log)
	return client, nil
}

// NewClientFromConfig creates a Kubernetes client based on configuration settings
// Business Requirement: BR-K8S-MAIN-001 - Configuration-driven client creation
// Following project principles: Configuration should determine environment settings, not business logic
func NewClientFromConfig(cfg config.KubernetesConfig, log *logrus.Logger) (Client, error) {
	// Determine client type based on configuration, not environment variables
	shouldUseFakeClient := determineShouldUseFakeClient(cfg)

	if shouldUseFakeClient {
		// Create fake client for testing/development
		fakeClientset := fake.NewSimpleClientset()
		client := NewUnifiedClient(fakeClientset, cfg, log)
		log.WithFields(logrus.Fields{
			"client_type": "fake",
			"namespace":   cfg.Namespace,
			"reason":      getClientTypeReason(cfg),
		}).Info("Created fake Kubernetes client")
		return client, nil
	}

	// Create real client for production/staging
	realClient, err := NewClient(cfg, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create real Kubernetes client: %w", err)
	}

	log.WithFields(logrus.Fields{
		"client_type": "real",
		"namespace":   cfg.Namespace,
		"context":     cfg.Context,
		"reason":      getClientTypeReason(cfg),
	}).Info("Created real Kubernetes client")

	return realClient, nil
}

// determineShouldUseFakeClient determines if fake client should be used based on configuration
// Following project principles: Configuration determines behavior, not environment variables
func determineShouldUseFakeClient(cfg config.KubernetesConfig) bool {
	// Direct override has highest priority
	if cfg.UseFakeClient {
		return true
	}

	// Explicit client type configuration
	switch cfg.ClientType {
	case "fake", "test", "mock":
		return true
	case "real", "production":
		return false
	case "auto", "":
		// Auto-detect based on environment context for backwards compatibility
		// This is the ONLY place where environment detection is acceptable
		// because it's in the configuration layer, not business logic
		environment := os.Getenv("ENVIRONMENT")
		return environment == "development" || environment == "testing" || environment == ""
	default:
		// Unknown client type, default to real client for safety
		return false
	}
}

// getClientTypeReason returns the reason why a particular client type was chosen
func getClientTypeReason(cfg config.KubernetesConfig) string {
	if cfg.UseFakeClient {
		return "use_fake_client=true"
	}
	if cfg.ClientType != "" {
		return fmt.Sprintf("client_type=%s", cfg.ClientType)
	}
	return "auto-detected"
}
