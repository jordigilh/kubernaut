package enhanced

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// ClusterScenario defines pre-configured cluster scenarios based on real kubernaut deployments
type ClusterScenario string

const (
	// Production scenarios based on real kubernaut operational patterns
	HighLoadProduction     ClusterScenario = "high_load_production" // 5 nodes, 100+ pods, high throughput
	ResourceConstrained    ClusterScenario = "resource_constrained" // Tight limits, pending pods, resource pressure
	MultiTenantDevelopment ClusterScenario = "multi_tenant_dev"     // Multiple tenants with RBAC isolation
	MonitoringStack        ClusterScenario = "monitoring_heavy"     // Prometheus, Grafana, alert manager stack
	DisasterRecovery       ClusterScenario = "disaster_simulation"  // Failed nodes, network issues, recovery testing
	BasicDevelopment       ClusterScenario = "basic_development"    // Simple development environment
)

// WorkloadProfile defines the types of workloads to simulate
type WorkloadProfile string

const (
	WebApplicationStack    WorkloadProfile = "web_application_stack"    // Web services, databases, caches
	MonitoringWorkload     WorkloadProfile = "monitoring_workload"      // Prometheus, Grafana, AlertManager
	AIMLWorkload           WorkloadProfile = "aiml_workload"            // GPU workloads, ML pipelines
	HighThroughputServices WorkloadProfile = "high_throughput_services" // High-performance microservices
	KubernautOperator      WorkloadProfile = "kubernaut_operator"       // Kubernaut-specific components
)

// ResourceProfile defines resource allocation patterns
type ResourceProfile string

const (
	DevelopmentResources     ResourceProfile = "development_resources"      // Lower resource requirements
	ProductionResourceLimits ResourceProfile = "production_resource_limits" // Production-level resource allocation
	GPUAcceleratedNodes      ResourceProfile = "gpu_accelerated_nodes"      // GPU-enabled nodes for AI/ML
	HighMemoryNodes          ResourceProfile = "high_memory_nodes"          // Memory-intensive workloads
)

// BehaviorSimulation configures dynamic behavior during test execution
type BehaviorSimulation struct {
	ResourceUpdates   bool          `yaml:"resource_updates" default:"true"`   // Simulate resource state changes
	EventGeneration   bool          `yaml:"event_generation" default:"true"`   // Generate realistic K8s events
	MetricsSimulation bool          `yaml:"metrics_simulation" default:"true"` // Simulate resource usage metrics
	NetworkLatency    time.Duration `yaml:"network_latency" default:"10ms"`    // Simulate API latency
}

// ClusterConfig configures the enhanced fake cluster creation
type ClusterConfig struct {
	Scenario        ClusterScenario    `yaml:"scenario"`
	NodeCount       int                `yaml:"node_count" default:"3"`
	Namespaces      []string           `yaml:"namespaces"`
	WorkloadProfile WorkloadProfile    `yaml:"workload_profile"`
	ResourceProfile ResourceProfile    `yaml:"resource_profile"`
	BehaviorSim     BehaviorSimulation `yaml:"behavior_simulation"`
	Logger          *logrus.Logger     `yaml:"-"`
}

// ClusterFactory creates enhanced fake Kubernetes clusters with realistic production patterns
type ClusterFactory interface {
	CreateCluster(scenario ClusterScenario, config *ClusterConfig) (*fake.Clientset, error)
	WithNamespaces(namespaces ...string) ClusterFactory
	WithWorkloads(workload WorkloadProfile) ClusterFactory
	WithResources(profile ResourceProfile) ClusterFactory
	WithBehavior(behavior BehaviorSimulation) ClusterFactory
}

// clusterFactory implements ClusterFactory with production-like resource generation
type clusterFactory struct {
	config    *ClusterConfig
	generator *ResourceGenerator
	logger    *logrus.Logger
}

// NewClusterFactory creates a new cluster factory with default configuration
func NewClusterFactory(logger *logrus.Logger) ClusterFactory {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
	}

	defaultConfig := &ClusterConfig{
		Scenario:        BasicDevelopment,
		NodeCount:       3,
		Namespaces:      []string{"default", "monitoring"},
		WorkloadProfile: WebApplicationStack,
		ResourceProfile: DevelopmentResources,
		BehaviorSim: BehaviorSimulation{
			ResourceUpdates:   true,
			EventGeneration:   true,
			MetricsSimulation: true,
			NetworkLatency:    10 * time.Millisecond,
		},
		Logger: logger,
	}

	return &clusterFactory{
		config:    defaultConfig,
		generator: NewResourceGenerator(logger),
		logger:    logger,
	}
}

// NewProductionLikeCluster creates a production-like cluster with one function call
// Business Requirement: BR-ENHANCED-K8S-002 - Developer productivity improvement
func NewProductionLikeCluster(config *ClusterConfig) *fake.Clientset {
	if config.Logger == nil {
		config.Logger = logrus.New()
		config.Logger.SetLevel(logrus.WarnLevel)
	}

	factory := NewClusterFactory(config.Logger)
	client, err := factory.CreateCluster(config.Scenario, config)
	if err != nil {
		config.Logger.WithError(err).Error("Failed to create production-like cluster")
		// Fallback to basic fake client
		return fake.NewSimpleClientset()
	}

	config.Logger.WithFields(logrus.Fields{
		"scenario":         config.Scenario,
		"node_count":       config.NodeCount,
		"workload_profile": config.WorkloadProfile,
		"namespaces":       len(config.Namespaces),
	}).Info("Created production-like cluster successfully")

	return client
}

// CreateCluster creates a fake cluster based on the specified scenario
func (f *clusterFactory) CreateCluster(scenario ClusterScenario, config *ClusterConfig) (*fake.Clientset, error) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		f.logger.WithFields(logrus.Fields{
			"scenario":       scenario,
			"creation_time":  elapsed,
			"performance_ok": elapsed < 500*time.Millisecond,
		}).Debug("Cluster creation completed")
	}()

	if config == nil {
		config = f.config
	}

	// Create fake clientset
	fakeClient := fake.NewSimpleClientset()
	ctx := context.Background()

	// Apply configuration overrides
	f.config = config

	// Create cluster infrastructure based on scenario
	if err := f.createClusterInfrastructure(ctx, fakeClient); err != nil {
		return nil, fmt.Errorf("failed to create cluster infrastructure: %w", err)
	}

	// Create namespaces
	if err := f.createNamespaces(ctx, fakeClient); err != nil {
		return nil, fmt.Errorf("failed to create namespaces: %w", err)
	}

	// Create workloads based on profile
	if err := f.createWorkloads(ctx, fakeClient); err != nil {
		return nil, fmt.Errorf("failed to create workloads: %w", err)
	}

	// Apply scenario-specific configurations
	if err := f.applyScenarioSpecifics(ctx, fakeClient, scenario); err != nil {
		return nil, fmt.Errorf("failed to apply scenario specifics: %w", err)
	}

	f.logger.WithFields(logrus.Fields{
		"scenario":   scenario,
		"nodes":      f.config.NodeCount,
		"namespaces": len(f.config.Namespaces),
		"workloads":  f.config.WorkloadProfile,
	}).Info("Enhanced fake cluster created successfully")

	return fakeClient, nil
}

// createClusterInfrastructure creates nodes and cluster-level resources
func (f *clusterFactory) createClusterInfrastructure(ctx context.Context, client *fake.Clientset) error {
	// Create nodes based on resource profile
	nodeSpecs := f.getNodeSpecsForProfile()

	for i, spec := range nodeSpecs {
		if i >= f.config.NodeCount {
			break
		}

		node := f.generator.GenerateNode(spec)
		_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create node %s: %w", node.Name, err)
		}
	}

	// Create cluster roles and service accounts
	if err := f.createRBACResources(ctx, client); err != nil {
		return fmt.Errorf("failed to create RBAC resources: %w", err)
	}

	return nil
}

// createNamespaces creates the configured namespaces with appropriate resource quotas
func (f *clusterFactory) createNamespaces(ctx context.Context, client *fake.Clientset) error {
	if len(f.config.Namespaces) == 0 {
		f.config.Namespaces = []string{"default"}
	}

	for _, nsName := range f.config.Namespaces {
		// Create namespace
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"created-by": "enhanced-fake-client",
					"scenario":   string(f.config.Scenario),
				},
			},
		}
		_, err := client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", nsName, err)
		}

		// Create resource quota for namespace
		quota := f.generator.GenerateResourceQuota(nsName, f.getResourceLimitsForProfile())
		_, err = client.CoreV1().ResourceQuotas(nsName).Create(ctx, quota, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create resource quota for namespace %s: %w", nsName, err)
		}
	}

	return nil
}

// createWorkloads creates workloads based on the specified profile
func (f *clusterFactory) createWorkloads(ctx context.Context, client *fake.Clientset) error {
	workloadSpecs := f.getWorkloadSpecsForProfile()

	for _, spec := range workloadSpecs {
		// Ensure namespace exists
		if !f.containsNamespace(spec.Namespace) {
			continue
		}

		// Create deployment
		deployment := f.generator.GenerateDeployment(spec)
		_, err := client.AppsV1().Deployments(spec.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create deployment %s: %w", spec.Name, err)
		}

		// Create service
		serviceSpec := ServiceSpec{
			Name:      spec.Name + "-service",
			Namespace: spec.Namespace,
			Selector:  spec.Labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		}
		service := f.generator.GenerateService(serviceSpec)
		_, err = client.CoreV1().Services(spec.Namespace).Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create service %s: %w", serviceSpec.Name, err)
		}

		// Create pods for the deployment
		for i := int32(0); i < spec.Replicas; i++ {
			podSpec := PodSpec{
				Name:      fmt.Sprintf("%s-%d", spec.Name, i),
				Namespace: spec.Namespace,
				Labels:    spec.Labels,
				Image:     spec.Image,
				Resources: spec.Resources,
				Phase:     corev1.PodRunning,
			}
			pod := f.generator.GeneratePod(podSpec)
			_, err = client.CoreV1().Pods(spec.Namespace).Create(ctx, pod, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create pod %s: %w", podSpec.Name, err)
			}
		}

		// Create HPA if specified
		if spec.AutoScale {
			hpaConfig := HPAConfig{
				DeploymentName: spec.Name,
				Namespace:      spec.Namespace,
				MinReplicas:    spec.Replicas,
				MaxReplicas:    spec.Replicas * 3,
				CPUTarget:      70,
				MemoryTarget:   80,
			}
			hpa := f.generator.GenerateHPA(hpaConfig)
			_, err = client.AutoscalingV2().HorizontalPodAutoscalers(spec.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create HPA %s: %w", spec.Name, err)
			}
		}
	}

	return nil
}

// applyScenarioSpecifics applies scenario-specific configurations
func (f *clusterFactory) applyScenarioSpecifics(ctx context.Context, client *fake.Clientset, scenario ClusterScenario) error {
	switch scenario {
	case ResourceConstrained:
		return f.applyResourceConstraints(ctx, client)
	case DisasterRecovery:
		return f.applyDisasterConditions(ctx, client)
	case MonitoringStack:
		return f.applyMonitoringConfiguration(ctx, client)
	default:
		// No specific configuration needed for other scenarios
		return nil
	}
}

// WithNamespaces sets the namespaces for the cluster
func (f *clusterFactory) WithNamespaces(namespaces ...string) ClusterFactory {
	f.config.Namespaces = namespaces
	return f
}

// WithWorkloads sets the workload profile
func (f *clusterFactory) WithWorkloads(workload WorkloadProfile) ClusterFactory {
	f.config.WorkloadProfile = workload
	return f
}

// WithResources sets the resource profile
func (f *clusterFactory) WithResources(profile ResourceProfile) ClusterFactory {
	f.config.ResourceProfile = profile
	return f
}

// WithBehavior sets the behavior simulation configuration
func (f *clusterFactory) WithBehavior(behavior BehaviorSimulation) ClusterFactory {
	f.config.BehaviorSim = behavior
	return f
}

// Helper methods for configuration

func (f *clusterFactory) containsNamespace(namespace string) bool {
	for _, ns := range f.config.Namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func (f *clusterFactory) getNodeSpecsForProfile() []NodeSpec {
	var nodeSpecs []NodeSpec

	switch f.config.ResourceProfile {
	case GPUAcceleratedNodes:
		// Create GPU nodes first, then fill with standard nodes
		gpuNodeCount := f.config.NodeCount / 2
		if gpuNodeCount == 0 {
			gpuNodeCount = 1
		}

		for i := 0; i < gpuNodeCount && i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("gpu-node-%d", i+1),
				Capacity: gpuNodeCapacity(),
				Labels:   gpuNodeLabels(),
			})
		}

		for i := gpuNodeCount; i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("cpu-node-%d", i+1-gpuNodeCount),
				Capacity: standardNodeCapacity(),
				Labels:   standardNodeLabels(),
			})
		}

	case HighMemoryNodes:
		// Create high-memory nodes first, then standard nodes
		memoryNodeCount := f.config.NodeCount / 2
		if memoryNodeCount == 0 {
			memoryNodeCount = 1
		}

		for i := 0; i < memoryNodeCount && i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("memory-node-%d", i+1),
				Capacity: highMemoryNodeCapacity(),
				Labels:   highMemoryNodeLabels(),
			})
		}

		for i := memoryNodeCount; i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("standard-node-%d", i+1-memoryNodeCount),
				Capacity: standardNodeCapacity(),
				Labels:   standardNodeLabels(),
			})
		}

	case ProductionResourceLimits:
		// Create production nodes up to requested count
		for i := 0; i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("prod-node-%d", i+1),
				Capacity: productionNodeCapacity(),
				Labels:   productionNodeLabels(),
			})
		}

	default: // DevelopmentResources
		// Create development nodes up to requested count
		for i := 0; i < f.config.NodeCount; i++ {
			nodeSpecs = append(nodeSpecs, NodeSpec{
				Name:     fmt.Sprintf("dev-node-%d", i+1),
				Capacity: developmentNodeCapacity(),
				Labels:   developmentNodeLabels(),
			})
		}
	}

	return nodeSpecs
}

func (f *clusterFactory) getResourceLimitsForProfile() ResourceLimits {
	switch f.config.ResourceProfile {
	case ProductionResourceLimits:
		return ResourceLimits{
			CPU:    resource.MustParse("4000m"),
			Memory: resource.MustParse("8Gi"),
			Pods:   resource.MustParse("50"),
		}
	case GPUAcceleratedNodes:
		return ResourceLimits{
			CPU:    resource.MustParse("8000m"),
			Memory: resource.MustParse("16Gi"),
			Pods:   resource.MustParse("30"),
		}
	default:
		return ResourceLimits{
			CPU:    resource.MustParse("2000m"),
			Memory: resource.MustParse("4Gi"),
			Pods:   resource.MustParse("20"),
		}
	}
}

func (f *clusterFactory) getWorkloadSpecsForProfile() []DeploymentSpec {
	switch f.config.WorkloadProfile {
	case KubernautOperator:
		return f.getKubernautWorkloads()
	case MonitoringWorkload:
		return f.getMonitoringWorkloads()
	case AIMLWorkload:
		return f.getAIMLWorkloads()
	case HighThroughputServices:
		return f.getHighThroughputWorkloads()
	default: // WebApplicationStack
		return f.getWebApplicationWorkloads()
	}
}

// Scenario-specific implementation methods

func (f *clusterFactory) applyResourceConstraints(ctx context.Context, client *fake.Clientset) error {
	// Create tight resource quotas
	for _, nsName := range f.config.Namespaces {
		constrainedQuota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "constrained-quota",
				Namespace: nsName,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
					corev1.ResourcePods:   resource.MustParse("5"),
				},
			},
		}
		_, err := client.CoreV1().ResourceQuotas(nsName).Create(ctx, constrainedQuota, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create constrained quota for %s: %w", nsName, err)
		}
	}
	return nil
}

func (f *clusterFactory) applyDisasterConditions(ctx context.Context, client *fake.Clientset) error {
	// Mark some nodes as NotReady
	nodes, _ := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if len(nodes.Items) > 1 {
		// Mark last node as NotReady
		node := &nodes.Items[len(nodes.Items)-1]
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
				Reason: "NetworkUnavailable",
			},
		}
		_, err := client.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update node status: %w", err)
		}
	}
	return nil
}

func (f *clusterFactory) applyMonitoringConfiguration(ctx context.Context, client *fake.Clientset) error {
	// Create monitoring-specific configurations
	// This could include ServiceMonitors, PrometheusRules, etc.
	// For now, we'll add monitoring labels to resources
	for _, nsName := range f.config.Namespaces {
		if nsName == "monitoring" {
			// Create additional monitoring resources
			monitoringConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "prometheus-config",
					Namespace: nsName,
				},
				Data: map[string]string{
					"prometheus.yml": "global:\n  scrape_interval: 15s\n",
				},
			}
			_, err := client.CoreV1().ConfigMaps(nsName).Create(ctx, monitoringConfigMap, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create monitoring config: %w", err)
			}
		}
	}
	return nil
}

func (f *clusterFactory) createRBACResources(ctx context.Context, client *fake.Clientset) error {
	// Create cluster role for kubernaut
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubernaut-operator",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "nodes", "events", "configmaps", "secrets", "services"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "replicasets", "statefulsets", "daemonsets"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
			},
		},
	}
	_, err := client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create cluster role: %w", err)
	}

	// Create service account
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernaut-operator",
			Namespace: "default",
		},
	}
	_, err = client.CoreV1().ServiceAccounts("default").Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	return nil
}
