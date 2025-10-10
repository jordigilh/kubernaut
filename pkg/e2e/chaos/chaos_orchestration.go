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
//go:build e2e
// +build e2e

package chaos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// BR-E2E-002: LitmusChaos integration for controlled instability injection
// Business Impact: Validates kubernaut resilience under production failure scenarios
// Stakeholder Value: Operations teams gain confidence in system reliability under stress

// ChaosExperimentType defines types of chaos experiments
type ChaosExperimentType string

const (
	// Pod-level chaos experiments
	ChaosExperimentPodDelete     ChaosExperimentType = "pod-delete"
	ChaosExperimentPodKill       ChaosExperimentType = "pod-kill"
	ChaosExperimentContainerKill ChaosExperimentType = "container-kill"

	// Node-level chaos experiments
	ChaosExperimentNodeCPUHog    ChaosExperimentType = "node-cpu-hog"
	ChaosExperimentNodeMemoryHog ChaosExperimentType = "node-memory-hog"
	ChaosExperimentNodeDrain     ChaosExperimentType = "node-drain"

	// Network chaos experiments
	ChaosExperimentNetworkPartition ChaosExperimentType = "network-partition"
	ChaosExperimentNetworkLatency   ChaosExperimentType = "network-latency"
	ChaosExperimentNetworkLoss      ChaosExperimentType = "network-loss"

	// Resource exhaustion experiments
	ChaosExperimentDiskFill           ChaosExperimentType = "disk-fill"
	ChaosExperimentResourceExhaustion ChaosExperimentType = "resource-exhaustion"
)

// ChaosParameters defines structured parameters for chaos experiments
// Following type safety guidelines: avoid interface{} usage
type ChaosParameters struct {
	CPUPercentage    *int    `yaml:"cpu_percentage,omitempty" json:"cpu_percentage,omitempty"`
	MemoryPercentage *int    `yaml:"memory_percentage,omitempty" json:"memory_percentage,omitempty"`
	NetworkLatency   *string `yaml:"network_latency,omitempty" json:"network_latency,omitempty"`
	DiskFillSize     *string `yaml:"disk_fill_size,omitempty" json:"disk_fill_size,omitempty"`
	TargetPods       *int    `yaml:"target_pods,omitempty" json:"target_pods,omitempty"`
	Force            *bool   `yaml:"force,omitempty" json:"force,omitempty"`
}

// ChaosExperiment defines a chaos engineering experiment
// E2E Test Infrastructure: Used for programmatic Litmus chaos experiments
type ChaosExperiment struct {
	Name           string              `yaml:"name" json:"name"`
	Type           ChaosExperimentType `yaml:"type" json:"type"`
	TargetSelector map[string]string   `yaml:"target_selector" json:"target_selector"`
	Namespace      string              `yaml:"namespace" json:"namespace"`
	Duration       time.Duration       `yaml:"duration" json:"duration"`
	Parameters     ChaosParameters     `yaml:"parameters" json:"parameters"` // Structured instead of interface{}

	// Execution tracking
	Status    string    `yaml:"status" json:"status"`
	StartTime time.Time `yaml:"start_time" json:"start_time"`
	EndTime   time.Time `yaml:"end_time" json:"end_time"`
	LastError string    `yaml:"last_error,omitempty" json:"last_error,omitempty"`
}

// ChaosResult contains the result of a chaos experiment
type ChaosResult struct {
	ExperimentName  string             `json:"experiment_name"`
	Status          string             `json:"status"`
	Duration        time.Duration      `json:"duration"`
	TargetsAffected int                `json:"targets_affected"`
	Metrics         map[string]float64 `json:"metrics"`
	ErrorMessage    string             `json:"error_message,omitempty"`
}

// LitmusChaosEngine manages LitmusChaos-based chaos engineering experiments
type LitmusChaosEngine struct {
	client      kubernetes.Interface
	logger      *logrus.Logger
	namespace   string
	experiments map[string]*ChaosExperiment

	// Chaos engine state
	installed bool
	running   bool
}

// NewLitmusChaosEngine creates a new LitmusChaos engine
// Business Requirement: BR-E2E-002 - Chaos engineering for resilience testing
func NewLitmusChaosEngine(client kubernetes.Interface, logger *logrus.Logger) (*LitmusChaosEngine, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}

	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	engine := &LitmusChaosEngine{
		client:      client,
		logger:      logger,
		experiments: make(map[string]*ChaosExperiment),
	}

	logger.Info("LitmusChaos engine created")
	return engine, nil
}

// Setup installs and configures LitmusChaos in the cluster
// Business Requirement: BR-E2E-002 - Chaos infrastructure setup
func (lce *LitmusChaosEngine) Setup(ctx context.Context, namespace string) error {
	if lce.installed {
		return fmt.Errorf("LitmusChaos already installed")
	}

	lce.namespace = namespace
	lce.logger.WithField("namespace", namespace).Info("Setting up LitmusChaos")

	// Create namespace
	if err := lce.createNamespace(ctx); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Setup RBAC
	if err := lce.setupRBAC(ctx); err != nil {
		return fmt.Errorf("failed to setup RBAC: %w", err)
	}

	// Install chaos operator
	if err := lce.installChaosOperator(ctx); err != nil {
		return fmt.Errorf("failed to install chaos operator: %w", err)
	}

	// Wait for operator to be ready
	if err := lce.waitForOperatorReady(ctx); err != nil {
		return fmt.Errorf("chaos operator not ready: %w", err)
	}

	lce.installed = true
	lce.logger.Info("LitmusChaos setup completed")
	return nil
}

// createNamespace creates the chaos engineering namespace
func (lce *LitmusChaosEngine) createNamespace(ctx context.Context) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: lce.namespace,
			Labels: map[string]string{
				"kubernaut.io/component": "chaos-engineering",
				"kubernaut.io/e2e":       "true",
			},
		},
	}

	_, err := lce.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create chaos namespace: %w", err)
	}

	lce.logger.WithField("namespace", lce.namespace).Info("Chaos namespace created")
	return nil
}

// setupRBAC creates necessary RBAC permissions for chaos experiments
func (lce *LitmusChaosEngine) setupRBAC(ctx context.Context) error {
	// Create service account
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "litmus-chaos",
			Namespace: lce.namespace,
		},
	}

	_, err := lce.client.CoreV1().ServiceAccounts(lce.namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	// Create cluster role
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "litmus-chaos",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods", "events", "configmaps", "secrets", "services"},
				Verbs:     []string{"get", "list", "patch", "create", "update", "delete"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments", "daemonsets", "replicasets", "statefulsets"},
				Verbs:     []string{"get", "list", "patch", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "patch"},
			},
		},
	}

	_, err = lce.client.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create cluster role: %w", err)
	}

	// Create cluster role binding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "litmus-chaos",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "litmus-chaos",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "litmus-chaos",
				Namespace: lce.namespace,
			},
		},
	}

	_, err = lce.client.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	lce.logger.Info("Chaos RBAC setup completed")
	return nil
}

// installChaosOperator installs the Litmus chaos operator
func (lce *LitmusChaosEngine) installChaosOperator(ctx context.Context) error {
	// For E2E testing, we'll use a simplified chaos operator deployment
	// In production, this would use Helm charts or operators

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-operator",
			Namespace: lce.namespace,
			Labels: map[string]string{
				"app": "chaos-operator",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "chaos-operator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "chaos-operator",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "litmus-chaos",
					Containers: []corev1.Container{
						{
							Name:  "chaos-operator",
							Image: "litmuschaos/chaos-operator:latest",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
									Name:          "metrics",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CHAOS_RUNNER_IMAGE",
									Value: "litmuschaos/chaos-runner:latest",
								},
								{
									Name: "WATCH_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}

	_, err := lce.client.AppsV1().Deployments(lce.namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("failed to create chaos operator deployment: %w", err)
	}

	lce.logger.Info("Chaos operator deployment created")
	return nil
}

// waitForOperatorReady waits for the chaos operator to be ready
func (lce *LitmusChaosEngine) waitForOperatorReady(ctx context.Context) error {
	lce.logger.Info("Waiting for chaos operator to be ready")

	return wait.PollUntilContextTimeout(ctx, 10*time.Second, 300*time.Second, true, func(ctx context.Context) (bool, error) {
		deployment, err := lce.client.AppsV1().Deployments(lce.namespace).Get(ctx, "chaos-operator", metav1.GetOptions{})
		if err != nil {
			lce.logger.WithError(err).Debug("Failed to get chaos operator deployment")
			return false, nil
		}

		if deployment.Status.ReadyReplicas >= 1 {
			lce.logger.Info("Chaos operator is ready")
			return true, nil
		}

		lce.logger.WithFields(logrus.Fields{
			"ready_replicas":   deployment.Status.ReadyReplicas,
			"desired_replicas": deployment.Spec.Replicas,
		}).Debug("Waiting for chaos operator to be ready")

		return false, nil
	})
}

// RunExperiment runs a chaos experiment
// Business Requirement: BR-E2E-003 - AI decision-making validation under chaos conditions
func (lce *LitmusChaosEngine) RunExperiment(ctx context.Context, experiment *ChaosExperiment) (*ChaosResult, error) {
	if !lce.installed {
		return nil, fmt.Errorf("LitmusChaos not installed")
	}

	lce.logger.WithFields(logrus.Fields{
		"experiment": experiment.Name,
		"type":       experiment.Type,
		"namespace":  experiment.Namespace,
		"duration":   experiment.Duration,
	}).Info("Running chaos experiment")

	experiment.Status = "running"
	experiment.StartTime = time.Now()
	lce.experiments[experiment.Name] = experiment

	result := &ChaosResult{
		ExperimentName: experiment.Name,
		Status:         "running",
		Metrics:        make(map[string]float64),
	}

	// Execute experiment based on type
	switch experiment.Type {
	case ChaosExperimentPodDelete:
		err := lce.runPodDeleteExperiment(ctx, experiment, result)
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = err.Error()
			experiment.LastError = err.Error()
		}
	case ChaosExperimentNodeCPUHog:
		err := lce.runNodeCPUHogExperiment(ctx, experiment, result)
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = err.Error()
			experiment.LastError = err.Error()
		}
	case ChaosExperimentNodeMemoryHog:
		err := lce.runNodeMemoryHogExperiment(ctx, experiment, result)
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = err.Error()
			experiment.LastError = err.Error()
		}
	case ChaosExperimentNetworkPartition:
		err := lce.runNetworkPartitionExperiment(ctx, experiment, result)
		if err != nil {
			result.Status = "failed"
			result.ErrorMessage = err.Error()
			experiment.LastError = err.Error()
		}
	default:
		return nil, fmt.Errorf("unsupported experiment type: %s", experiment.Type)
	}

	experiment.EndTime = time.Now()
	experiment.Status = result.Status
	result.Duration = experiment.EndTime.Sub(experiment.StartTime)

	if result.Status != "failed" {
		result.Status = "completed"
		experiment.Status = "completed"
	}

	lce.logger.WithFields(logrus.Fields{
		"experiment":       experiment.Name,
		"status":           result.Status,
		"duration":         result.Duration,
		"targets_affected": result.TargetsAffected,
	}).Info("Chaos experiment completed")

	return result, nil
}

// runPodDeleteExperiment runs a pod deletion chaos experiment
func (lce *LitmusChaosEngine) runPodDeleteExperiment(ctx context.Context, experiment *ChaosExperiment, result *ChaosResult) error {
	// Get target pods based on selector
	pods, err := lce.client.CoreV1().Pods(experiment.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelsToSelector(experiment.TargetSelector),
	})
	if err != nil {
		return fmt.Errorf("failed to list target pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no target pods found for experiment")
	}

	// Delete pods (simulating pod failures)
	deletedPods := 0
	for _, pod := range pods.Items {
		if deletedPods >= 1 { // Limit impact for E2E testing
			break
		}

		err := lce.client.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
		if err != nil {
			lce.logger.WithError(err).WithField("pod", pod.Name).Warn("Failed to delete pod")
			continue
		}

		deletedPods++
		lce.logger.WithField("pod", pod.Name).Info("Pod deleted for chaos experiment")
	}

	result.TargetsAffected = deletedPods
	result.Metrics["pods_deleted"] = float64(deletedPods)

	// Wait for the experiment duration
	time.Sleep(experiment.Duration)

	return nil
}

// runNodeCPUHogExperiment runs a node CPU stress experiment
func (lce *LitmusChaosEngine) runNodeCPUHogExperiment(ctx context.Context, experiment *ChaosExperiment, result *ChaosResult) error {
	// For E2E testing, we'll create a CPU stress pod
	stressPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cpu-stress-%s", experiment.Name),
			Namespace: experiment.Namespace,
			Labels: map[string]string{
				"kubernaut.io/chaos-experiment": experiment.Name,
				"kubernaut.io/chaos-type":       string(experiment.Type),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "cpu-stress",
					Image: "busybox:latest",
					Command: []string{
						"sh", "-c",
						"while true; do dd if=/dev/zero of=/dev/null; done",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"cpu": *parseQuantity("100m"),
						},
						Limits: corev1.ResourceList{
							"cpu": *parseQuantity("500m"),
						},
					},
				},
			},
		},
	}

	_, err := lce.client.CoreV1().Pods(experiment.Namespace).Create(ctx, stressPod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create CPU stress pod: %w", err)
	}

	result.TargetsAffected = 1
	result.Metrics["stress_pods_created"] = 1

	// Wait for experiment duration
	time.Sleep(experiment.Duration)

	// Cleanup stress pod
	err = lce.client.CoreV1().Pods(experiment.Namespace).Delete(ctx, stressPod.Name, metav1.DeleteOptions{})
	if err != nil {
		lce.logger.WithError(err).Warn("Failed to cleanup CPU stress pod")
	}

	return nil
}

// runNodeMemoryHogExperiment runs a node memory stress experiment
func (lce *LitmusChaosEngine) runNodeMemoryHogExperiment(ctx context.Context, experiment *ChaosExperiment, result *ChaosResult) error {
	// Similar to CPU stress but for memory
	stressPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("memory-stress-%s", experiment.Name),
			Namespace: experiment.Namespace,
			Labels: map[string]string{
				"kubernaut.io/chaos-experiment": experiment.Name,
				"kubernaut.io/chaos-type":       string(experiment.Type),
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:  "memory-stress",
					Image: "busybox:latest",
					Command: []string{
						"sh", "-c",
						"while true; do head -c 100M < /dev/zero | tail; done",
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"memory": *parseQuantity("100Mi"),
						},
						Limits: corev1.ResourceList{
							"memory": *parseQuantity("200Mi"),
						},
					},
				},
			},
		},
	}

	_, err := lce.client.CoreV1().Pods(experiment.Namespace).Create(ctx, stressPod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create memory stress pod: %w", err)
	}

	result.TargetsAffected = 1
	result.Metrics["stress_pods_created"] = 1

	// Wait for experiment duration
	time.Sleep(experiment.Duration)

	// Cleanup stress pod
	err = lce.client.CoreV1().Pods(experiment.Namespace).Delete(ctx, stressPod.Name, metav1.DeleteOptions{})
	if err != nil {
		lce.logger.WithError(err).Warn("Failed to cleanup memory stress pod")
	}

	return nil
}

// runNetworkPartitionExperiment runs a network partition experiment
func (lce *LitmusChaosEngine) runNetworkPartitionExperiment(ctx context.Context, experiment *ChaosExperiment, result *ChaosResult) error {
	// For E2E testing, this is simplified
	// In real scenarios, this would use network policies or iptables rules
	lce.logger.Info("Simulating network partition (simplified for E2E)")

	// Wait for experiment duration (simulating network issues)
	time.Sleep(experiment.Duration)

	result.TargetsAffected = 1
	result.Metrics["network_partitions_simulated"] = 1

	return nil
}

// Cleanup cleans up LitmusChaos resources
func (lce *LitmusChaosEngine) Cleanup(ctx context.Context) error {
	if !lce.installed {
		return nil
	}

	lce.logger.Info("Cleaning up LitmusChaos resources")

	// Stop all running experiments
	for _, experiment := range lce.experiments {
		if experiment.Status == "running" {
			experiment.Status = "stopped"
			experiment.EndTime = time.Now()
		}
	}

	// Delete chaos operator deployment
	err := lce.client.AppsV1().Deployments(lce.namespace).Delete(ctx, "chaos-operator", metav1.DeleteOptions{})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		lce.logger.WithError(err).Warn("Failed to delete chaos operator deployment")
	}

	// Delete RBAC resources
	_ = lce.client.RbacV1().ClusterRoleBindings().Delete(ctx, "litmus-chaos", metav1.DeleteOptions{})
	_ = lce.client.RbacV1().ClusterRoles().Delete(ctx, "litmus-chaos", metav1.DeleteOptions{})
	_ = lce.client.CoreV1().ServiceAccounts(lce.namespace).Delete(ctx, "litmus-chaos", metav1.DeleteOptions{})

	lce.installed = false
	lce.running = false
	lce.logger.Info("LitmusChaos cleanup completed")

	return nil
}

// GetExperimentStatus returns the status of a chaos experiment
func (lce *LitmusChaosEngine) GetExperimentStatus(experimentName string) (*ChaosExperiment, bool) {
	experiment, exists := lce.experiments[experimentName]
	return experiment, exists
}

// ListExperiments returns all chaos experiments
func (lce *LitmusChaosEngine) ListExperiments() map[string]*ChaosExperiment {
	return lce.experiments
}

// IsInstalled returns whether LitmusChaos is installed
func (lce *LitmusChaosEngine) IsInstalled() bool {
	return lce.installed
}

// Helper functions

func int32Ptr(i int32) *int32 {
	return &i
}

func labelsToSelector(labels map[string]string) string {
	var selectors []string
	for key, value := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", key, value))
	}
	return strings.Join(selectors, ",")
}

func parseQuantity(quantity string) *resource.Quantity {
	// Simplified for E2E testing
	// In production, use resource.ParseQuantity
	q := resource.MustParse(quantity)
	return &q
}
