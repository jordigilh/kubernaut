package k8s

import (
	"fmt"
	"path/filepath"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client combines basic and advanced Kubernetes operations
type Client interface {
	BasicClient
	AdvancedClient
}

// client implements the full Client interface using composition
type client struct {
	*basicClient
	*advancedClient
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

	// Set default namespace if not specified
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Create the basic client
	basic := &basicClient{
		clientset: clientset,
		namespace: namespace,
		log:       log,
	}

	// Create the advanced client (which embeds the basic client)
	advanced := &advancedClient{
		basicClient: basic,
	}

	log.WithFields(logrus.Fields{
		"namespace": namespace,
		"context":   cfg.Context,
	}).Info("Kubernetes client initialized")

	return &client{
		basicClient:    basic,
		advancedClient: advanced,
	}, nil
}

// Verify that client implements both interfaces
var _ BasicClient = (*client)(nil)
var _ AdvancedClient = (*client)(nil)
var _ Client = (*client)(nil)
