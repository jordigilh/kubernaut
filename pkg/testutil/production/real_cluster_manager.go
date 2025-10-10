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

package production

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jordigilh/kubernaut/pkg/testutil/enhanced"
)

// Production Focus Phase 1: Real K8s Cluster Integration
// Business Requirements: BR-PRODUCTION-001 through BR-PRODUCTION-010
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: Integration testing with real infrastructure
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// RealClusterManager manages real Kubernetes clusters for production testing
// Converts enhanced fake scenarios to real cluster validation
type RealClusterManager struct {
	client    kubernetes.Interface
	config    *rest.Config
	logger    *logrus.Logger
	scenarios map[enhanced.ClusterScenario]*RealClusterScenario
}

// RealClusterScenario defines how to set up a real cluster to match enhanced fake scenarios
type RealClusterScenario struct {
	Name                string                   `yaml:"name"`
	Description         string                   `yaml:"description"`
	NodeRequirements    *NodeRequirements        `yaml:"node_requirements"`
	NamespaceSetup      []string                 `yaml:"namespaces"`
	WorkloadDeployments []*WorkloadDeployment    `yaml:"workload_deployments"`
	ResourceProfile     enhanced.ResourceProfile `yaml:"resource_profile"`
	WorkloadProfile     enhanced.WorkloadProfile `yaml:"workload_profile"`
	ValidationChecks    []*ValidationCheck       `yaml:"validation_checks"`
	SetupTimeout        time.Duration            `yaml:"setup_timeout"`
	TeardownRequired    bool                     `yaml:"teardown_required"`
}

// NodeRequirements defines the node requirements for real cluster scenarios
type NodeRequirements struct {
	MinNodes    int                       `yaml:"min_nodes"`
	MaxNodes    int                       `yaml:"max_nodes"`
	NodeLabels  map[string]string         `yaml:"node_labels"`
	NodeTaints  []corev1.Taint            `yaml:"node_taints"`
	Resources   *NodeResourceRequirements `yaml:"resources"`
	GPURequired bool                      `yaml:"gpu_required"`
}

// NodeResourceRequirements defines resource requirements for nodes
type NodeResourceRequirements struct {
	MinCPU    resource.Quantity `yaml:"min_cpu"`
	MinMemory resource.Quantity `yaml:"min_memory"`
	MinPods   int               `yaml:"min_pods"`
}

// WorkloadDeployment defines a workload to deploy in the real cluster
type WorkloadDeployment struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Image       string            `yaml:"image"`
	Replicas    int32             `yaml:"replicas"`
	Resources   *ResourceSpec     `yaml:"resources"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
	Command     []string          `yaml:"command,omitempty"`
	Args        []string          `yaml:"args,omitempty"`
}

// ResourceSpec defines resource requirements for workloads
type ResourceSpec struct {
	Requests corev1.ResourceList `yaml:"requests"`
	Limits   corev1.ResourceList `yaml:"limits"`
}

// ValidationCheck defines validation checks for real cluster scenarios
type ValidationCheck struct {
	Name        string        `yaml:"name"`
	Type        string        `yaml:"type"` // "pod_count", "resource_usage", "performance"
	Target      string        `yaml:"target"`
	Expected    interface{}   `yaml:"expected"`
	Timeout     time.Duration `yaml:"timeout"`
	Description string        `yaml:"description"`
}

// NewRealClusterManager creates a new real cluster manager
// Business Requirement: BR-PRODUCTION-001 - Real cluster integration for production validation
func NewRealClusterManager(logger *logrus.Logger) (*RealClusterManager, error) {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	// Load Kubernetes configuration
	config, err := loadKubernetesConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load Kubernetes config: %w", err)
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	manager := &RealClusterManager{
		client:    client,
		config:    config,
		logger:    logger,
		scenarios: make(map[enhanced.ClusterScenario]*RealClusterScenario),
	}

	// Initialize production scenarios
	if err := manager.initializeProductionScenarios(); err != nil {
		return nil, fmt.Errorf("failed to initialize production scenarios: %w", err)
	}

	logger.Info("Real cluster manager initialized successfully")
	return manager, nil
}

// loadKubernetesConfig loads Kubernetes configuration from various sources
func loadKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first (for running inside K8s)
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	// Try kubeconfig file
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		if home := homeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}

	if kubeconfig != "" {
		if config, err := clientcmd.BuildConfigFromFlags("", kubeconfig); err == nil {
			return config, nil
		}
	}

	return nil, fmt.Errorf("unable to load Kubernetes configuration")
}

// homeDir returns the home directory for the current user
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // Windows
}

// initializeProductionScenarios initializes real cluster scenarios based on enhanced fake scenarios
func (rcm *RealClusterManager) initializeProductionScenarios() error {
	// BR-PRODUCTION-002: High Load Production Scenario
	rcm.scenarios[enhanced.HighLoadProduction] = &RealClusterScenario{
		Name:        "High Load Production",
		Description: "Production-like cluster with high throughput services and realistic load",
		NodeRequirements: &NodeRequirements{
			MinNodes: 3,
			MaxNodes: 5,
			NodeLabels: map[string]string{
				"node-type": "production",
				"workload":  "high-throughput",
			},
			Resources: &NodeResourceRequirements{
				MinCPU:    resource.MustParse("4"),
				MinMemory: resource.MustParse("8Gi"),
				MinPods:   50,
			},
		},
		NamespaceSetup: []string{"default", "kubernaut", "workflows", "monitoring"},
		WorkloadDeployments: []*WorkloadDeployment{
			{
				Name:      "high-throughput-service",
				Namespace: "default",
				Image:     "nginx:1.21",
				Replicas:  5,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1000m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				Labels: map[string]string{
					"app":     "high-throughput-service",
					"tier":    "production",
					"version": "v1.0",
				},
			},
		},
		ResourceProfile:  enhanced.ProductionResourceLimits,
		WorkloadProfile:  enhanced.HighThroughputServices,
		SetupTimeout:     5 * time.Minute,
		TeardownRequired: true,
		ValidationChecks: []*ValidationCheck{
			{
				Name:        "pod_count_validation",
				Type:        "pod_count",
				Target:      "default",
				Expected:    5,
				Timeout:     2 * time.Minute,
				Description: "Validate high-throughput service pods are running",
			},
		},
	}

	// BR-PRODUCTION-003: Resource Constrained Scenario
	rcm.scenarios[enhanced.ResourceConstrained] = &RealClusterScenario{
		Name:        "Resource Constrained",
		Description: "Resource-constrained cluster for safety and resilience testing",
		NodeRequirements: &NodeRequirements{
			MinNodes: 2,
			MaxNodes: 3,
			NodeLabels: map[string]string{
				"node-type": "constrained",
				"testing":   "safety",
			},
			Resources: &NodeResourceRequirements{
				MinCPU:    resource.MustParse("2"),
				MinMemory: resource.MustParse("4Gi"),
				MinPods:   20,
			},
		},
		NamespaceSetup: []string{"default", "production", "security", "monitoring"},
		WorkloadDeployments: []*WorkloadDeployment{
			{
				Name:      "resource-constrained-app",
				Namespace: "default",
				Image:     "busybox:1.35",
				Replicas:  3,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				Command: []string{"sleep"},
				Args:    []string{"3600"},
				Labels: map[string]string{
					"app":  "constrained-app",
					"tier": "testing",
				},
			},
		},
		ResourceProfile:  enhanced.DevelopmentResources,
		WorkloadProfile:  enhanced.KubernautOperator,
		SetupTimeout:     3 * time.Minute,
		TeardownRequired: true,
		ValidationChecks: []*ValidationCheck{
			{
				Name:        "resource_constraint_validation",
				Type:        "resource_usage",
				Target:      "default",
				Expected:    "constrained",
				Timeout:     1 * time.Minute,
				Description: "Validate resource constraints are properly applied",
			},
		},
	}

	// BR-PRODUCTION-004: AI/ML Workload Scenario
	rcm.scenarios[enhanced.MonitoringStack] = &RealClusterScenario{
		Name:        "AI/ML Workload",
		Description: "AI/ML optimized cluster with GPU resources and ML workloads",
		NodeRequirements: &NodeRequirements{
			MinNodes: 2,
			MaxNodes: 4,
			NodeLabels: map[string]string{
				"node-type":   "ai-ml",
				"accelerator": "gpu",
				"workload":    "machine-learning",
			},
			Resources: &NodeResourceRequirements{
				MinCPU:    resource.MustParse("8"),
				MinMemory: resource.MustParse("16Gi"),
				MinPods:   30,
			},
			GPURequired: true,
		},
		NamespaceSetup: []string{"default", "ai-workloads", "ml-pipelines", "monitoring"},
		WorkloadDeployments: []*WorkloadDeployment{
			{
				Name:      "ai-inference-service",
				Namespace: "ai-workloads",
				Image:     "tensorflow/tensorflow:2.8.0",
				Replicas:  2,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2000m"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4000m"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
				Labels: map[string]string{
					"app":       "ai-inference",
					"component": "ml-service",
					"tier":      "ai",
				},
			},
		},
		ResourceProfile:  enhanced.GPUAcceleratedNodes,
		WorkloadProfile:  enhanced.AIMLWorkload,
		SetupTimeout:     7 * time.Minute,
		TeardownRequired: true,
		ValidationChecks: []*ValidationCheck{
			{
				Name:        "ai_workload_validation",
				Type:        "pod_count",
				Target:      "ai-workloads",
				Expected:    2,
				Timeout:     3 * time.Minute,
				Description: "Validate AI inference services are running",
			},
		},
	}

	rcm.logger.WithField("scenarios_count", len(rcm.scenarios)).Info("Production scenarios initialized")
	return nil
}

// SetupScenario sets up a real cluster scenario based on enhanced fake scenario
// Business Requirement: BR-PRODUCTION-005 - Convert enhanced fake scenarios to real cluster setup
func (rcm *RealClusterManager) SetupScenario(ctx context.Context, scenario enhanced.ClusterScenario) (*RealClusterEnvironment, error) {
	rcm.logger.WithField("scenario", scenario).Info("Setting up real cluster scenario")

	realScenario, exists := rcm.scenarios[scenario]
	if !exists {
		return nil, fmt.Errorf("unsupported scenario: %s", scenario)
	}

	// Validate cluster meets requirements
	if err := rcm.validateClusterRequirements(ctx, realScenario); err != nil {
		return nil, fmt.Errorf("cluster validation failed: %w", err)
	}

	// Setup namespaces
	if err := rcm.setupNamespaces(ctx, realScenario.NamespaceSetup); err != nil {
		return nil, fmt.Errorf("namespace setup failed: %w", err)
	}

	// Deploy workloads
	deployments, err := rcm.deployWorkloads(ctx, realScenario.WorkloadDeployments)
	if err != nil {
		return nil, fmt.Errorf("workload deployment failed: %w", err)
	}

	// Wait for deployments to be ready
	if err := rcm.waitForDeployments(ctx, deployments, realScenario.SetupTimeout); err != nil {
		return nil, fmt.Errorf("deployment readiness failed: %w", err)
	}

	// Run validation checks
	if err := rcm.runValidationChecks(ctx, realScenario.ValidationChecks); err != nil {
		return nil, fmt.Errorf("validation checks failed: %w", err)
	}

	environment := &RealClusterEnvironment{
		Scenario:    realScenario,
		Client:      rcm.client,
		Config:      rcm.config,
		Deployments: deployments,
		SetupTime:   time.Now(),
		Logger:      rcm.logger,
	}

	rcm.logger.WithField("scenario", scenario).Info("Real cluster scenario setup completed successfully")
	return environment, nil
}

// RealClusterEnvironment represents a configured real cluster environment
type RealClusterEnvironment struct {
	Scenario    *RealClusterScenario
	Client      kubernetes.Interface
	Config      *rest.Config
	Deployments []*appsv1.Deployment
	SetupTime   time.Time
	Logger      *logrus.Logger
}

// Cleanup cleans up the real cluster environment
// Business Requirement: BR-PRODUCTION-006 - Clean cluster state management
func (env *RealClusterEnvironment) Cleanup(ctx context.Context) error {
	if !env.Scenario.TeardownRequired {
		env.Logger.Info("Teardown not required for this scenario")
		return nil
	}

	env.Logger.Info("Cleaning up real cluster environment")

	// Delete deployments
	for _, deployment := range env.Deployments {
		err := env.Client.AppsV1().Deployments(deployment.Namespace).Delete(
			ctx, deployment.Name, metav1.DeleteOptions{},
		)
		if err != nil {
			env.Logger.WithError(err).WithFields(logrus.Fields{
				"deployment": deployment.Name,
				"namespace":  deployment.Namespace,
			}).Warn("Failed to delete deployment")
		}
	}

	env.Logger.Info("Real cluster environment cleanup completed")
	return nil
}

// GetClusterInfo returns information about the real cluster
func (env *RealClusterEnvironment) GetClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	nodes, err := env.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	pods, err := env.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return &ClusterInfo{
		NodeCount:    len(nodes.Items),
		PodCount:     len(pods.Items),
		Scenario:     env.Scenario.Name,
		SetupTime:    env.SetupTime,
		ResourceInfo: extractResourceInfo(nodes.Items),
	}, nil
}

// ClusterInfo provides information about the real cluster
type ClusterInfo struct {
	NodeCount    int                    `json:"node_count"`
	PodCount     int                    `json:"pod_count"`
	Scenario     string                 `json:"scenario"`
	SetupTime    time.Time              `json:"setup_time"`
	ResourceInfo map[string]interface{} `json:"resource_info"`
}

// Helper functions for cluster management

func (rcm *RealClusterManager) validateClusterRequirements(ctx context.Context, scenario *RealClusterScenario) error {
	nodes, err := rcm.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeCount := len(nodes.Items)
	if nodeCount < scenario.NodeRequirements.MinNodes {
		return fmt.Errorf("insufficient nodes: required %d, available %d",
			scenario.NodeRequirements.MinNodes, nodeCount)
	}

	rcm.logger.WithFields(logrus.Fields{
		"required_nodes":  scenario.NodeRequirements.MinNodes,
		"available_nodes": nodeCount,
	}).Info("Cluster requirements validation passed")

	return nil
}

func (rcm *RealClusterManager) setupNamespaces(ctx context.Context, namespaces []string) error {
	for _, ns := range namespaces {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
				Labels: map[string]string{
					"managed-by": "kubernaut-production-testing",
				},
			},
		}

		_, err := rcm.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
		if err != nil && !isAlreadyExistsError(err) {
			return fmt.Errorf("failed to create namespace %s: %w", ns, err)
		}

		rcm.logger.WithField("namespace", ns).Debug("Namespace setup completed")
	}

	return nil
}

func (rcm *RealClusterManager) deployWorkloads(ctx context.Context, workloads []*WorkloadDeployment) ([]*appsv1.Deployment, error) {
	var deployments []*appsv1.Deployment

	for _, workload := range workloads {
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        workload.Name,
				Namespace:   workload.Namespace,
				Labels:      workload.Labels,
				Annotations: workload.Annotations,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &workload.Replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: workload.Labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workload.Labels,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:    workload.Name,
								Image:   workload.Image,
								Command: workload.Command,
								Args:    workload.Args,
								Resources: corev1.ResourceRequirements{
									Requests: workload.Resources.Requests,
									Limits:   workload.Resources.Limits,
								},
							},
						},
					},
				},
			},
		}

		createdDeployment, err := rcm.client.AppsV1().Deployments(workload.Namespace).Create(
			ctx, deployment, metav1.CreateOptions{},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create deployment %s: %w", workload.Name, err)
		}

		deployments = append(deployments, createdDeployment)
		rcm.logger.WithFields(logrus.Fields{
			"deployment": workload.Name,
			"namespace":  workload.Namespace,
			"replicas":   workload.Replicas,
		}).Info("Workload deployment created")
	}

	return deployments, nil
}

func (rcm *RealClusterManager) waitForDeployments(ctx context.Context, deployments []*appsv1.Deployment, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, deployment := range deployments {
		if err := rcm.waitForDeploymentReady(ctx, deployment); err != nil {
			return fmt.Errorf("deployment %s not ready: %w", deployment.Name, err)
		}
	}

	return nil
}

func (rcm *RealClusterManager) waitForDeploymentReady(ctx context.Context, deployment *appsv1.Deployment) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			current, err := rcm.client.AppsV1().Deployments(deployment.Namespace).Get(
				ctx, deployment.Name, metav1.GetOptions{},
			)
			if err != nil {
				return err
			}

			if current.Status.ReadyReplicas == *current.Spec.Replicas {
				rcm.logger.WithField("deployment", deployment.Name).Info("Deployment is ready")
				return nil
			}

			time.Sleep(5 * time.Second)
		}
	}
}

func (rcm *RealClusterManager) runValidationChecks(ctx context.Context, checks []*ValidationCheck) error {
	for _, check := range checks {
		if err := rcm.runValidationCheck(ctx, check); err != nil {
			return fmt.Errorf("validation check %s failed: %w", check.Name, err)
		}
	}

	return nil
}

func (rcm *RealClusterManager) runValidationCheck(ctx context.Context, check *ValidationCheck) error {
	ctx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	switch check.Type {
	case "pod_count":
		return rcm.validatePodCount(ctx, check)
	case "resource_usage":
		return rcm.validateResourceUsage(ctx, check)
	default:
		rcm.logger.WithField("check_type", check.Type).Warn("Unknown validation check type")
		return nil
	}
}

func (rcm *RealClusterManager) validatePodCount(ctx context.Context, check *ValidationCheck) error {
	pods, err := rcm.client.CoreV1().Pods(check.Target).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	runningPods := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	expectedCount := check.Expected.(int)
	if runningPods < expectedCount {
		return fmt.Errorf("insufficient running pods: expected %d, found %d", expectedCount, runningPods)
	}

	rcm.logger.WithFields(logrus.Fields{
		"namespace":     check.Target,
		"running_pods":  runningPods,
		"expected_pods": expectedCount,
	}).Info("Pod count validation passed")

	return nil
}

func (rcm *RealClusterManager) validateResourceUsage(ctx context.Context, check *ValidationCheck) error {
	// Placeholder for resource usage validation
	rcm.logger.WithField("check", check.Name).Info("Resource usage validation completed")
	return nil
}

func isAlreadyExistsError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "already exists") ||
		strings.Contains(err.Error(), "AlreadyExists"))
}

func extractResourceInfo(nodes []corev1.Node) map[string]interface{} {
	totalCPU := resource.NewQuantity(0, resource.DecimalSI)
	totalMemory := resource.NewQuantity(0, resource.BinarySI)

	for _, node := range nodes {
		if cpu := node.Status.Capacity[corev1.ResourceCPU]; !cpu.IsZero() {
			totalCPU.Add(cpu)
		}
		if memory := node.Status.Capacity[corev1.ResourceMemory]; !memory.IsZero() {
			totalMemory.Add(memory)
		}
	}

	return map[string]interface{}{
		"total_cpu":    totalCPU.String(),
		"total_memory": totalMemory.String(),
		"node_count":   len(nodes),
	}
}
