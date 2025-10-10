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

package enhanced

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ResourceGenerator creates realistic Kubernetes resources based on production patterns
type ResourceGenerator struct {
	logger *logrus.Logger
}

// NewResourceGenerator creates a new resource generator
func NewResourceGenerator(logger *logrus.Logger) *ResourceGenerator {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
	}
	return &ResourceGenerator{logger: logger}
}

// Specification structs for resource generation

// DeploymentSpec specifies deployment creation parameters
type DeploymentSpec struct {
	Name         string                      `yaml:"name"`
	Namespace    string                      `yaml:"namespace"`
	Replicas     int32                       `yaml:"replicas"`
	Image        string                      `yaml:"image"`
	Resources    corev1.ResourceRequirements `yaml:"resources"`
	Labels       map[string]string           `yaml:"labels"`
	Annotations  map[string]string           `yaml:"annotations"`
	AutoScale    bool                        `yaml:"auto_scale"`
	HealthChecks HealthCheckConfig           `yaml:"health_checks"`
}

// ServiceSpec specifies service creation parameters
type ServiceSpec struct {
	Name      string               `yaml:"name"`
	Namespace string               `yaml:"namespace"`
	Selector  map[string]string    `yaml:"selector"`
	Ports     []corev1.ServicePort `yaml:"ports"`
	Type      corev1.ServiceType   `yaml:"type"`
}

// PodSpec specifies pod creation parameters
type PodSpec struct {
	Name      string                      `yaml:"name"`
	Namespace string                      `yaml:"namespace"`
	Labels    map[string]string           `yaml:"labels"`
	Image     string                      `yaml:"image"`
	Resources corev1.ResourceRequirements `yaml:"resources"`
	Phase     corev1.PodPhase             `yaml:"phase"`
	NodeName  string                      `yaml:"node_name"`
}

// NodeSpec specifies node creation parameters
type NodeSpec struct {
	Name       string                 `yaml:"name"`
	Capacity   corev1.ResourceList    `yaml:"capacity"`
	Labels     map[string]string      `yaml:"labels"`
	Conditions []corev1.NodeCondition `yaml:"conditions"`
}

// HealthCheckConfig specifies health check configuration
type HealthCheckConfig struct {
	LivenessProbe  *corev1.Probe `yaml:"liveness_probe"`
	ReadinessProbe *corev1.Probe `yaml:"readiness_probe"`
}

// ResourceLimits specifies resource quota limits
type ResourceLimits struct {
	CPU    resource.Quantity `yaml:"cpu"`
	Memory resource.Quantity `yaml:"memory"`
	Pods   resource.Quantity `yaml:"pods"`
}

// HPAConfig specifies HPA configuration
type HPAConfig struct {
	DeploymentName string `yaml:"deployment_name"`
	Namespace      string `yaml:"namespace"`
	MinReplicas    int32  `yaml:"min_replicas"`
	MaxReplicas    int32  `yaml:"max_replicas"`
	CPUTarget      int32  `yaml:"cpu_target"`
	MemoryTarget   int32  `yaml:"memory_target"`
}

// GenerateDeployment creates a realistic deployment based on production patterns
func (g *ResourceGenerator) GenerateDeployment(spec DeploymentSpec) *appsv1.Deployment {
	if spec.Labels == nil {
		spec.Labels = make(map[string]string)
	}
	if spec.Annotations == nil {
		spec.Annotations = make(map[string]string)
	}

	// Add default labels for realistic production patterns
	spec.Labels["app"] = spec.Name
	spec.Labels["created-by"] = "enhanced-fake-client"
	spec.Labels["version"] = "v1.0.0"

	// Add production-like annotations
	spec.Annotations["deployment.kubernetes.io/revision"] = "1"
	spec.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = ""

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   spec.Namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": spec.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: spec.Labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      spec.Name,
							Image:     spec.Image,
							Resources: spec.Resources,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:        spec.Replicas,
			ReadyReplicas:   spec.Replicas - 1, // Realistic: not all replicas always ready
			UpdatedReplicas: spec.Replicas,
			Conditions: []appsv1.DeploymentCondition{
				{
					Type:   appsv1.DeploymentProgressing,
					Status: corev1.ConditionTrue,
					Reason: "NewReplicaSetAvailable",
				},
			},
		},
	}

	// Add health checks if specified
	if spec.HealthChecks.LivenessProbe != nil {
		deployment.Spec.Template.Spec.Containers[0].LivenessProbe = spec.HealthChecks.LivenessProbe
	} else {
		// Add default liveness probe for production realism
		deployment.Spec.Template.Spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
		}
	}

	if spec.HealthChecks.ReadinessProbe != nil {
		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = spec.HealthChecks.ReadinessProbe
	} else {
		// Add default readiness probe for production realism
		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ready",
					Port: intstr.FromInt(8080),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
		}
	}

	g.logger.WithFields(logrus.Fields{
		"deployment": spec.Name,
		"namespace":  spec.Namespace,
		"replicas":   spec.Replicas,
		"image":      spec.Image,
	}).Debug("Generated realistic deployment")

	return deployment
}

// GenerateService creates a realistic service
func (g *ResourceGenerator) GenerateService(spec ServiceSpec) *corev1.Service {
	if spec.Type == "" {
		spec.Type = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app":        spec.Name,
				"created-by": "enhanced-fake-client",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: spec.Selector,
			Ports:    spec.Ports,
			Type:     spec.Type,
		},
	}

	g.logger.WithFields(logrus.Fields{
		"service":   spec.Name,
		"namespace": spec.Namespace,
		"type":      spec.Type,
		"ports":     len(spec.Ports),
	}).Debug("Generated realistic service")

	return service
}

// GeneratePod creates a realistic pod
func (g *ResourceGenerator) GeneratePod(spec PodSpec) *corev1.Pod {
	if spec.Labels == nil {
		spec.Labels = make(map[string]string)
	}
	spec.Labels["created-by"] = "enhanced-fake-client"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels:    spec.Labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:      spec.Name,
					Image:     spec.Image,
					Resources: spec.Resources,
				},
			},
			NodeName: spec.NodeName,
		},
		Status: corev1.PodStatus{
			Phase: spec.Phase,
			Conditions: []corev1.PodCondition{
				{
					Type: corev1.PodReady,
					Status: func() corev1.ConditionStatus {
						if spec.Phase == corev1.PodRunning {
							return corev1.ConditionTrue
						}
						return corev1.ConditionFalse
					}(),
				},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:  spec.Name,
					Ready: spec.Phase == corev1.PodRunning,
					State: corev1.ContainerState{
						Running: func() *corev1.ContainerStateRunning {
							if spec.Phase == corev1.PodRunning {
								return &corev1.ContainerStateRunning{
									StartedAt: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
								}
							}
							return nil
						}(),
					},
				},
			},
		},
	}

	g.logger.WithFields(logrus.Fields{
		"pod":       spec.Name,
		"namespace": spec.Namespace,
		"phase":     spec.Phase,
		"node":      spec.NodeName,
	}).Debug("Generated realistic pod")

	return pod
}

// GenerateNode creates a realistic node
func (g *ResourceGenerator) GenerateNode(spec NodeSpec) *corev1.Node {
	if spec.Labels == nil {
		spec.Labels = make(map[string]string)
	}

	// Add realistic node labels
	spec.Labels["kubernetes.io/hostname"] = spec.Name
	spec.Labels["kubernetes.io/os"] = "linux"
	spec.Labels["kubernetes.io/arch"] = "amd64"
	spec.Labels["node.kubernetes.io/instance-type"] = "m5.large"
	spec.Labels["topology.kubernetes.io/zone"] = "us-west-2a"

	// Default conditions if not specified
	if len(spec.Conditions) == 0 {
		spec.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
				Reason: "KubeletReady",
			},
			{
				Type:   corev1.NodeMemoryPressure,
				Status: corev1.ConditionFalse,
				Reason: "KubeletHasSufficientMemory",
			},
			{
				Type:   corev1.NodeDiskPressure,
				Status: corev1.ConditionFalse,
				Reason: "KubeletHasNoDiskPressure",
			},
		}
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   spec.Name,
			Labels: spec.Labels,
		},
		Spec: corev1.NodeSpec{
			PodCIDR: "10.244.0.0/24",
		},
		Status: corev1.NodeStatus{
			Capacity:    spec.Capacity,
			Allocatable: spec.Capacity, // Simplified: allocatable = capacity
			Conditions:  spec.Conditions,
			NodeInfo: corev1.NodeSystemInfo{
				MachineID:               fmt.Sprintf("machine-%s", spec.Name),
				SystemUUID:              fmt.Sprintf("system-%s", spec.Name),
				BootID:                  fmt.Sprintf("boot-%s", spec.Name),
				KernelVersion:           "5.4.0-74-generic",
				OSImage:                 "Ubuntu 20.04.2 LTS",
				ContainerRuntimeVersion: "containerd://1.4.6",
				KubeletVersion:          "v1.21.0",
				KubeProxyVersion:        "v1.21.0",
				OperatingSystem:         "linux",
				Architecture:            "amd64",
			},
		},
	}

	g.logger.WithFields(logrus.Fields{
		"node":     spec.Name,
		"capacity": spec.Capacity,
		"ready":    isNodeReady(spec.Conditions),
	}).Debug("Generated realistic node")

	return node
}

// GenerateResourceQuota creates a resource quota for a namespace
func (g *ResourceGenerator) GenerateResourceQuota(namespace string, limits ResourceLimits) *corev1.ResourceQuota {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "resource-quota",
			Namespace: namespace,
			Labels: map[string]string{
				"created-by": "enhanced-fake-client",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    limits.CPU,
				corev1.ResourceMemory: limits.Memory,
				corev1.ResourcePods:   limits.Pods,
			},
		},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    limits.CPU,
				corev1.ResourceMemory: limits.Memory,
				corev1.ResourcePods:   limits.Pods,
			},
			Used: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("0"),
				corev1.ResourceMemory: resource.MustParse("0"),
				corev1.ResourcePods:   resource.MustParse("0"),
			},
		},
	}

	g.logger.WithFields(logrus.Fields{
		"namespace": namespace,
		"cpu_limit": limits.CPU.String(),
		"mem_limit": limits.Memory.String(),
		"pod_limit": limits.Pods.String(),
	}).Debug("Generated resource quota")

	return quota
}

// GenerateHPA creates a horizontal pod autoscaler
func (g *ResourceGenerator) GenerateHPA(config HPAConfig) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.DeploymentName + "-hpa",
			Namespace: config.Namespace,
			Labels: map[string]string{
				"created-by": "enhanced-fake-client",
				"app":        config.DeploymentName,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       config.DeploymentName,
			},
			MinReplicas: &config.MinReplicas,
			MaxReplicas: config.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &config.CPUTarget,
						},
					},
				},
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceMemory,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &config.MemoryTarget,
						},
					},
				},
			},
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: config.MinReplicas,
			DesiredReplicas: config.MinReplicas,
		},
	}

	g.logger.WithFields(logrus.Fields{
		"hpa":          hpa.Name,
		"namespace":    config.Namespace,
		"target":       config.DeploymentName,
		"min_replicas": config.MinReplicas,
		"max_replicas": config.MaxReplicas,
	}).Debug("Generated HPA")

	return hpa
}

// Helper functions for realistic resource allocation patterns

// Node capacity functions based on production instance types
func standardNodeCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2000m"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
		corev1.ResourcePods:   resource.MustParse("30"),
	}
}

func productionNodeCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4000m"),
		corev1.ResourceMemory: resource.MustParse("8Gi"),
		corev1.ResourcePods:   resource.MustParse("50"),
	}
}

func developmentNodeCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1000m"),
		corev1.ResourceMemory: resource.MustParse("2Gi"),
		corev1.ResourcePods:   resource.MustParse("20"),
	}
}

func gpuNodeCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("8000m"),
		corev1.ResourceMemory: resource.MustParse("32Gi"),
		corev1.ResourcePods:   resource.MustParse("40"),
		"nvidia.com/gpu":      resource.MustParse("2"),
	}
}

func highMemoryNodeCapacity() corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("16000m"),
		corev1.ResourceMemory: resource.MustParse("64Gi"),
		corev1.ResourcePods:   resource.MustParse("50"),
	}
}

// Node label functions for realistic production patterns
func standardNodeLabels() map[string]string {
	return map[string]string{
		"node.kubernetes.io/instance-type": "m5.large",
		"topology.kubernetes.io/zone":      "us-west-2a",
		"node-role.kubernetes.io/worker":   "",
	}
}

func productionNodeLabels() map[string]string {
	return map[string]string{
		"node.kubernetes.io/instance-type": "m5.xlarge",
		"topology.kubernetes.io/zone":      "us-west-2a",
		"node-role.kubernetes.io/worker":   "",
		"environment":                      "production",
	}
}

func developmentNodeLabels() map[string]string {
	return map[string]string{
		"node.kubernetes.io/instance-type": "t3.medium",
		"topology.kubernetes.io/zone":      "us-west-2a",
		"node-role.kubernetes.io/worker":   "",
		"environment":                      "development",
	}
}

func gpuNodeLabels() map[string]string {
	return map[string]string{
		"node.kubernetes.io/instance-type": "p3.2xlarge",
		"topology.kubernetes.io/zone":      "us-west-2a",
		"node-role.kubernetes.io/worker":   "",
		"accelerator":                      "nvidia-tesla-v100",
		"workload-type":                    "gpu",
	}
}

func highMemoryNodeLabels() map[string]string {
	return map[string]string{
		"node.kubernetes.io/instance-type": "r5.4xlarge",
		"topology.kubernetes.io/zone":      "us-west-2a",
		"node-role.kubernetes.io/worker":   "",
		"workload-type":                    "memory-intensive",
	}
}

// Helper functions
func isNodeReady(conditions []corev1.NodeCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
