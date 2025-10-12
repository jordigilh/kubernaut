package k8s

import (
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Config contains Kubernetes client configuration
type Config struct {
	// KubeconfigPath is the path to kubeconfig file (optional, falls back to in-cluster config)
	KubeconfigPath string

	// Context is the kubeconfig context to use (optional)
	Context string

	// InCluster forces in-cluster configuration (for production deployments)
	InCluster bool
}

// NewClient creates a Kubernetes client using in-cluster config or kubeconfig
// Business Requirement: BR-TOOLSET-005 - Kubernetes API integration
//
// Pattern: Follows Gateway service Kubernetes client initialization pattern
// Priority: In-cluster config first (production), then kubeconfig (development)
func NewClient(cfg Config, logger *zap.Logger) (kubernetes.Interface, error) {
	var k8sConfig *rest.Config
	var err error

	// Try in-cluster config first (for running inside Kubernetes)
	if cfg.InCluster || cfg.KubeconfigPath == "" {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil && cfg.InCluster {
			// If in-cluster is explicitly requested but fails, return error
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}

		if err == nil {
			logger.Info("Using in-cluster Kubernetes configuration")
			clientset, err := kubernetes.NewForConfig(k8sConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
			}
			return clientset, nil
		}
	}

	// Fallback to kubeconfig file (for development/testing)
	if cfg.KubeconfigPath != "" {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeconfigPath)
	} else {
		// Use default kubeconfig path
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		configOverrides := &clientcmd.ConfigOverrides{}

		if cfg.Context != "" {
			configOverrides.CurrentContext = cfg.Context
		}

		k8sConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules,
			configOverrides,
		).ClientConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	logger.Info("Using kubeconfig file for Kubernetes configuration",
		zap.String("kubeconfig", cfg.KubeconfigPath),
		zap.String("context", cfg.Context))

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return clientset, nil
}

