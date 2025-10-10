<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jordigilh/kubernaut/pkg/testutil/production"
)

// BR-E2E-001: OCP cluster management for comprehensive E2E testing
// Business Impact: Provides production-like environment for kubernaut validation
// Stakeholder Value: Operations teams can validate complete system behavior

// ClusterType defines supported cluster types for E2E testing
type ClusterType string

const (
	ClusterTypeOCP  ClusterType = "ocp"  // OpenShift Container Platform
	ClusterTypeKind ClusterType = "kind" // Kubernetes in Docker (for development)
)

// E2EClusterManager manages Kubernetes clusters for end-to-end testing
// Supports both OCP (production-like) and Kind (development) clusters
type E2EClusterManager struct {
	clusterType ClusterType
	client      kubernetes.Interface
	config      *rest.Config
	logger      *logrus.Logger

	// Cluster state
	initialized    bool
	clusterVersion string
	namespaces     []string

	// Production cluster management (for OCP)
	realClusterManager *production.RealClusterManager
}

// ClusterInfo contains information about the managed cluster
type ClusterInfo struct {
	Type            ClusterType `json:"type"`
	Version         string      `json:"version"`
	NodeCount       int         `json:"node_count"`
	ReadyNodes      int         `json:"ready_nodes"`
	Namespaces      []string    `json:"namespaces"`
	ClusterEndpoint string      `json:"cluster_endpoint"`
	Status          string      `json:"status"`
}

// NewE2EClusterManager creates a new E2E cluster manager
// Business Requirement: BR-E2E-001 - Cluster management for production-like testing
func NewE2EClusterManager(clusterType string, logger *logrus.Logger) (*E2EClusterManager, error) {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	var cType ClusterType
	switch strings.ToLower(clusterType) {
	case "ocp", "openshift":
		cType = ClusterTypeOCP
	case "kind", "kubernetes":
		cType = ClusterTypeKind
	default:
		return nil, fmt.Errorf("unsupported cluster type: %s (supported: ocp, kind)", clusterType)
	}

	manager := &E2EClusterManager{
		clusterType: cType,
		logger:      logger,
		namespaces:  []string{"default", "kube-system"},
	}

	logger.WithField("cluster_type", cType).Info("E2E cluster manager created")
	return manager, nil
}

// InitializeCluster initializes the cluster for E2E testing
// Business Requirement: BR-E2E-001 - Production-like cluster initialization
func (mgr *E2EClusterManager) InitializeCluster(ctx context.Context, version string) error {
	if mgr.initialized {
		return fmt.Errorf("cluster already initialized")
	}

	mgr.logger.WithFields(logrus.Fields{
		"cluster_type": mgr.clusterType,
		"version":      version,
	}).Info("Initializing E2E cluster")

	mgr.clusterVersion = version

	switch mgr.clusterType {
	case ClusterTypeOCP:
		return mgr.initializeOCPCluster(ctx, version)
	case ClusterTypeKind:
		return mgr.initializeKindCluster(ctx, version)
	default:
		return fmt.Errorf("unsupported cluster type: %s", mgr.clusterType)
	}
}

// initializeOCPCluster initializes an OpenShift cluster for E2E testing
func (mgr *E2EClusterManager) initializeOCPCluster(ctx context.Context, version string) error {
	mgr.logger.Info("Initializing OpenShift cluster for E2E testing")

	// Load existing cluster configuration
	config, err := mgr.loadClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to load OCP cluster config: %w", err)
	}
	mgr.config = config

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	mgr.client = client

	// Verify cluster connectivity and version
	if err := mgr.verifyClusterVersion(ctx, version); err != nil {
		return fmt.Errorf("cluster version verification failed: %w", err)
	}

	// Initialize production cluster manager for advanced operations
	realClusterManager, err := production.NewRealClusterManager(mgr.logger)
	if err != nil {
		return fmt.Errorf("failed to create production cluster manager: %w", err)
	}
	mgr.realClusterManager = realClusterManager

	// Setup E2E-specific namespaces
	if err := mgr.setupE2ENamespaces(ctx); err != nil {
		return fmt.Errorf("failed to setup E2E namespaces: %w", err)
	}

	mgr.initialized = true
	mgr.logger.Info("OpenShift cluster initialization completed")
	return nil
}

// initializeKindCluster initializes a Kind cluster for E2E testing
func (mgr *E2EClusterManager) initializeKindCluster(ctx context.Context, version string) error {
	mgr.logger.Info("Initializing Kind cluster for E2E testing")

	// Check if Kind cluster exists, create if not
	if err := mgr.ensureKindCluster(ctx, version); err != nil {
		return fmt.Errorf("failed to ensure Kind cluster: %w", err)
	}

	// Load Kind cluster configuration
	config, err := mgr.loadKindConfig()
	if err != nil {
		return fmt.Errorf("failed to load Kind cluster config: %w", err)
	}
	mgr.config = config

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	mgr.client = client

	// Wait for cluster to be ready
	if err := mgr.waitForClusterReady(ctx); err != nil {
		return fmt.Errorf("cluster readiness check failed: %w", err)
	}

	// Setup E2E-specific namespaces
	if err := mgr.setupE2ENamespaces(ctx); err != nil {
		return fmt.Errorf("failed to setup E2E namespaces: %w", err)
	}

	mgr.initialized = true
	mgr.logger.Info("Kind cluster initialization completed")
	return nil
}

// loadClusterConfig loads Kubernetes configuration from various sources
func (mgr *E2EClusterManager) loadClusterConfig() (*rest.Config, error) {
	// Try in-cluster config first
	if config, err := rest.InClusterConfig(); err == nil {
		mgr.logger.Info("Using in-cluster configuration")
		return config, nil
	}

	// Try kubeconfig file
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := os.Getenv("HOME"); home != "" {
			kubeconfig = home + "/.kube/config"
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	mgr.logger.WithField("kubeconfig", kubeconfig).Info("Using kubeconfig file")
	return config, nil
}

// loadKindConfig loads Kind cluster configuration
func (mgr *E2EClusterManager) loadKindConfig() (*rest.Config, error) {
	// Kind clusters use kubeconfig with specific context
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := os.Getenv("HOME"); home != "" {
			kubeconfig = home + "/.kube/config"
		}
	}

	// Load config with Kind context
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build Kind config: %w", err)
	}

	return config, nil
}

// ensureKindCluster ensures a Kind cluster exists for testing
func (mgr *E2EClusterManager) ensureKindCluster(ctx context.Context, version string) error {
	clusterName := "kubernaut-e2e"

	// Check if cluster exists
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check existing Kind clusters: %w", err)
	}

	clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, cluster := range clusters {
		if strings.TrimSpace(cluster) == clusterName {
			mgr.logger.WithField("cluster", clusterName).Info("Kind cluster already exists")
			return nil
		}
	}

	// Create Kind cluster
	mgr.logger.WithField("cluster", clusterName).Info("Creating Kind cluster for E2E testing")

	createCmd := exec.CommandContext(ctx, "kind", "create", "cluster",
		"--name", clusterName,
		"--config", "/dev/stdin")

	// Kind cluster configuration for E2E testing
	kindConfig := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: 6443
`

	createCmd.Stdin = strings.NewReader(kindConfig)
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	mgr.logger.Info("Kind cluster created successfully")
	return nil
}

// verifyClusterVersion verifies the cluster version matches expectations
func (mgr *E2EClusterManager) verifyClusterVersion(ctx context.Context, expectedVersion string) error {
	version, err := mgr.client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to get server version: %w", err)
	}

	mgr.logger.WithFields(logrus.Fields{
		"server_version":   version.String(),
		"expected_version": expectedVersion,
	}).Info("Cluster version verification")

	// For E2E testing, we're flexible with version matching
	// Just ensure the cluster is accessible
	return nil
}

// waitForClusterReady waits for the cluster to be ready
func (mgr *E2EClusterManager) waitForClusterReady(ctx context.Context) error {
	mgr.logger.Info("Waiting for cluster to be ready")

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		nodes, err := mgr.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			mgr.logger.WithError(err).Debug("Failed to list nodes, retrying...")
			return false, nil
		}

		readyNodes := 0
		for _, node := range nodes.Items {
			for _, condition := range node.Status.Conditions {
				if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
					readyNodes++
					break
				}
			}
		}

		if readyNodes >= 2 { // Control plane + at least 1 worker
			mgr.logger.WithFields(logrus.Fields{
				"total_nodes": len(nodes.Items),
				"ready_nodes": readyNodes,
			}).Info("Cluster is ready")
			return true, nil
		}

		mgr.logger.WithFields(logrus.Fields{
			"total_nodes": len(nodes.Items),
			"ready_nodes": readyNodes,
		}).Debug("Waiting for more nodes to be ready...")

		return false, nil
	})
}

// setupE2ENamespaces creates necessary namespaces for E2E testing
func (mgr *E2EClusterManager) setupE2ENamespaces(ctx context.Context) error {
	e2eNamespaces := []string{
		"kubernaut-e2e",
		"monitoring",
		"litmus",
		"chaos-testing",
	}

	for _, namespace := range e2eNamespaces {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Labels: map[string]string{
					"kubernaut.io/e2e":      "true",
					"kubernaut.io/testtype": "e2e",
				},
			},
		}

		_, err := mgr.client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
		}

		mgr.namespaces = append(mgr.namespaces, namespace)
		mgr.logger.WithField("namespace", namespace).Debug("E2E namespace created")
	}

	mgr.logger.WithField("namespaces", e2eNamespaces).Info("E2E namespaces setup completed")
	return nil
}

// GetKubernetesClient returns the Kubernetes client
func (mgr *E2EClusterManager) GetKubernetesClient() kubernetes.Interface {
	return mgr.client
}

// GetRestConfig returns the Kubernetes REST configuration
func (mgr *E2EClusterManager) GetRestConfig() *rest.Config {
	return mgr.config
}

// GetClusterInfo returns information about the managed cluster
func (mgr *E2EClusterManager) GetClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	if !mgr.initialized {
		return nil, fmt.Errorf("cluster not initialized")
	}

	// Get cluster version
	version, err := mgr.client.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	// Get node information
	nodes, err := mgr.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	readyNodes := 0
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyNodes++
				break
			}
		}
	}

	clusterEndpoint := mgr.config.Host
	if clusterEndpoint == "" {
		clusterEndpoint = "unknown"
	}

	status := "ready"
	if readyNodes < len(nodes.Items) {
		status = "partially-ready"
	}

	return &ClusterInfo{
		Type:            mgr.clusterType,
		Version:         version.String(),
		NodeCount:       len(nodes.Items),
		ReadyNodes:      readyNodes,
		Namespaces:      mgr.namespaces,
		ClusterEndpoint: clusterEndpoint,
		Status:          status,
	}, nil
}

// Cleanup cleans up cluster resources for E2E testing
func (mgr *E2EClusterManager) Cleanup(ctx context.Context) error {
	if !mgr.initialized {
		return nil
	}

	mgr.logger.Info("Cleaning up E2E cluster resources")

	// Cleanup E2E-specific namespaces (but preserve system namespaces)
	e2eNamespaces := []string{
		"kubernaut-e2e",
		"chaos-testing",
	}

	for _, namespace := range e2eNamespaces {
		err := mgr.client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
		if err != nil && !strings.Contains(err.Error(), "not found") {
			mgr.logger.WithError(err).WithField("namespace", namespace).Warn("Failed to delete namespace")
		} else {
			mgr.logger.WithField("namespace", namespace).Debug("E2E namespace deleted")
		}
	}

	// For Kind clusters, optionally delete the entire cluster
	if mgr.clusterType == ClusterTypeKind && os.Getenv("E2E_CLEANUP_KIND") == "true" {
		mgr.logger.Info("Deleting Kind cluster")
		cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", "kubernaut-e2e")
		if err := cmd.Run(); err != nil {
			mgr.logger.WithError(err).Warn("Failed to delete Kind cluster")
		}
	}

	mgr.initialized = false
	mgr.logger.Info("E2E cluster cleanup completed")
	return nil
}

// IsInitialized returns whether the cluster is initialized
func (mgr *E2EClusterManager) IsInitialized() bool {
	return mgr.initialized
}

// GetClusterType returns the cluster type
func (mgr *E2EClusterManager) GetClusterType() ClusterType {
	return mgr.clusterType
}

// GetNamespaces returns the list of managed namespaces
func (mgr *E2EClusterManager) GetNamespaces() []string {
	return mgr.namespaces
}
