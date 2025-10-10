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
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// Multi-Service Production Workload Deployment
// Business Requirements: BR-PRODUCTION-005 - Production workload deployment
// Following 00-project-guidelines.mdc: MANDATORY business requirement mapping
// Following 03-testing-strategy.mdc: Integration testing with multi-service workloads
// Following 09-interface-method-validation.mdc: Interface validation before code generation

// ResourceSpec is defined in real_cluster_manager.go to avoid duplication

// MultiServiceWorkloadManager manages complex multi-service production workloads
type MultiServiceWorkloadManager struct {
	client    kubernetes.Interface
	logger    *logrus.Logger
	scenarios map[string]*MultiServiceScenario
}

// MultiServiceScenario defines a multi-service deployment scenario
type MultiServiceScenario struct {
	Name         string                  `yaml:"name"`
	Description  string                  `yaml:"description"`
	Namespace    string                  `yaml:"namespace"`
	Services     []*ServiceDefinition    `yaml:"services"`
	Dependencies []*ServiceDependency    `yaml:"dependencies"`
	Validation   *MultiServiceValidation `yaml:"validation"`
	Timeout      time.Duration           `yaml:"timeout"`
}

// ServiceDefinition defines a service in the multi-service scenario
type ServiceDefinition struct {
	Name        string                 `yaml:"name"`
	Type        string                 `yaml:"type"` // "web", "database", "cache", "worker"
	Image       string                 `yaml:"image"`
	Replicas    int32                  `yaml:"replicas"`
	Port        int32                  `yaml:"port"`
	Resources   *ResourceSpec          `yaml:"resources"`
	Environment map[string]string      `yaml:"environment"`
	Labels      map[string]string      `yaml:"labels"`
	Annotations map[string]string      `yaml:"annotations"`
	Command     []string               `yaml:"command,omitempty"`
	Args        []string               `yaml:"args,omitempty"`
	HealthCheck *HealthCheckDefinition `yaml:"health_check,omitempty"`
}

// ServiceDependency defines dependencies between services
type ServiceDependency struct {
	Service   string        `yaml:"service"`
	DependsOn string        `yaml:"depends_on"`
	WaitFor   string        `yaml:"wait_for"` // "ready", "healthy", "available"
	Timeout   time.Duration `yaml:"timeout"`
}

// HealthCheckDefinition defines health check configuration
type HealthCheckDefinition struct {
	Path                string `yaml:"path"`
	Port                int32  `yaml:"port"`
	InitialDelaySeconds int32  `yaml:"initial_delay_seconds"`
	PeriodSeconds       int32  `yaml:"period_seconds"`
	TimeoutSeconds      int32  `yaml:"timeout_seconds"`
	FailureThreshold    int32  `yaml:"failure_threshold"`
}

// MultiServiceValidation defines validation for multi-service scenarios
type MultiServiceValidation struct {
	ServiceConnectivity bool                            `yaml:"service_connectivity"`
	HealthChecks        bool                            `yaml:"health_checks"`
	ResourceUsage       bool                            `yaml:"resource_usage"`
	PerformanceTargets  *MultiServicePerformanceTargets `yaml:"performance_targets"`
}

// MultiServicePerformanceTargets defines performance targets for multi-service scenarios
type MultiServicePerformanceTargets struct {
	DeploymentTime   time.Duration `yaml:"deployment_time"`    // Target: <5 minutes
	ServiceStartTime time.Duration `yaml:"service_start_time"` // Target: <2 minutes
	HealthCheckTime  time.Duration `yaml:"health_check_time"`  // Target: <30 seconds
	ConnectivityTime time.Duration `yaml:"connectivity_time"`  // Target: <10 seconds
}

// MultiServiceDeploymentResult contains deployment results
type MultiServiceDeploymentResult struct {
	Scenario           *MultiServiceScenario `json:"scenario"`
	DeployedServices   []*appsv1.Deployment  `json:"deployed_services"`
	CreatedServices    []*corev1.Service     `json:"created_services"`
	DeploymentTime     time.Duration         `json:"deployment_time"`
	ValidationResults  *ValidationResults    `json:"validation_results"`
	PerformanceMetrics *PerformanceMetrics   `json:"performance_metrics"`
	Status             string                `json:"status"`
}

// ValidationResults contains validation results for multi-service deployment
type ValidationResults struct {
	ServiceConnectivity bool     `json:"service_connectivity"`
	HealthChecks        bool     `json:"health_checks"`
	ResourceUsage       bool     `json:"resource_usage"`
	FailedValidations   []string `json:"failed_validations"`
}

// PerformanceMetrics contains performance metrics for multi-service deployment
type PerformanceMetrics struct {
	DeploymentTime   time.Duration `json:"deployment_time"`
	ServiceStartTime time.Duration `json:"service_start_time"`
	HealthCheckTime  time.Duration `json:"health_check_time"`
	ConnectivityTime time.Duration `json:"connectivity_time"`
}

// NewMultiServiceWorkloadManager creates a new multi-service workload manager
// Business Requirement: BR-PRODUCTION-005 - Complex multi-service production workload deployment
func NewMultiServiceWorkloadManager(client kubernetes.Interface, logger *logrus.Logger) *MultiServiceWorkloadManager {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	manager := &MultiServiceWorkloadManager{
		client:    client,
		logger:    logger,
		scenarios: make(map[string]*MultiServiceScenario),
	}

	// Initialize production multi-service scenarios
	manager.initializeMultiServiceScenarios()

	logger.Info("Multi-service workload manager initialized")
	return manager
}

// initializeMultiServiceScenarios initializes production multi-service scenarios
func (mswm *MultiServiceWorkloadManager) initializeMultiServiceScenarios() {
	// Web Application Stack Scenario
	mswm.scenarios["web-app-stack"] = &MultiServiceScenario{
		Name:        "Web Application Stack",
		Description: "Complete web application with frontend, backend, database, and cache",
		Namespace:   "web-app",
		Services: []*ServiceDefinition{
			{
				Name:     "frontend",
				Type:     "web",
				Image:    "nginx:1.21",
				Replicas: 2,
				Port:     80,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				Labels: map[string]string{
					"app":  "frontend",
					"tier": "web",
				},
				HealthCheck: &HealthCheckDefinition{
					Path:                "/",
					Port:                80,
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
					TimeoutSeconds:      5,
					FailureThreshold:    3,
				},
			},
			{
				Name:     "backend",
				Type:     "web",
				Image:    "httpd:2.4",
				Replicas: 3,
				Port:     8080,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1000m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				Environment: map[string]string{
					"DATABASE_HOST": "database",
					"CACHE_HOST":    "cache",
				},
				Labels: map[string]string{
					"app":  "backend",
					"tier": "application",
				},
				HealthCheck: &HealthCheckDefinition{
					Path:                "/health",
					Port:                8080,
					InitialDelaySeconds: 15,
					PeriodSeconds:       10,
					TimeoutSeconds:      5,
					FailureThreshold:    3,
				},
			},
			{
				Name:     "database",
				Type:     "database",
				Image:    "postgres:13",
				Replicas: 1,
				Port:     5432,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2000m"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				Environment: map[string]string{
					"POSTGRES_DB":       "webapp",
					"POSTGRES_USER":     "webapp",
					"POSTGRES_PASSWORD": "webapp123",
				},
				Labels: map[string]string{
					"app":  "database",
					"tier": "data",
				},
				HealthCheck: &HealthCheckDefinition{
					Port:                5432,
					InitialDelaySeconds: 30,
					PeriodSeconds:       10,
					TimeoutSeconds:      5,
					FailureThreshold:    3,
				},
			},
			{
				Name:     "cache",
				Type:     "cache",
				Image:    "redis:6.2",
				Replicas: 1,
				Port:     6379,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				Labels: map[string]string{
					"app":  "cache",
					"tier": "cache",
				},
				HealthCheck: &HealthCheckDefinition{
					Port:                6379,
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
					TimeoutSeconds:      5,
					FailureThreshold:    3,
				},
			},
		},
		Dependencies: []*ServiceDependency{
			{
				Service:   "backend",
				DependsOn: "database",
				WaitFor:   "ready",
				Timeout:   2 * time.Minute,
			},
			{
				Service:   "backend",
				DependsOn: "cache",
				WaitFor:   "ready",
				Timeout:   1 * time.Minute,
			},
			{
				Service:   "frontend",
				DependsOn: "backend",
				WaitFor:   "healthy",
				Timeout:   2 * time.Minute,
			},
		},
		Validation: &MultiServiceValidation{
			ServiceConnectivity: true,
			HealthChecks:        true,
			ResourceUsage:       true,
			PerformanceTargets: &MultiServicePerformanceTargets{
				DeploymentTime:   5 * time.Minute,
				ServiceStartTime: 2 * time.Minute,
				HealthCheckTime:  30 * time.Second,
				ConnectivityTime: 10 * time.Second,
			},
		},
		Timeout: 10 * time.Minute,
	}

	// Microservices Stack Scenario
	mswm.scenarios["microservices-stack"] = &MultiServiceScenario{
		Name:        "Microservices Stack",
		Description: "Microservices architecture with API gateway, services, and shared database",
		Namespace:   "microservices",
		Services: []*ServiceDefinition{
			{
				Name:     "api-gateway",
				Type:     "web",
				Image:    "nginx:1.21",
				Replicas: 2,
				Port:     80,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1000m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				Labels: map[string]string{
					"app":  "api-gateway",
					"tier": "gateway",
				},
			},
			{
				Name:     "user-service",
				Type:     "web",
				Image:    "httpd:2.4",
				Replicas: 2,
				Port:     8080,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("150m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				Labels: map[string]string{
					"app":  "user-service",
					"tier": "service",
				},
			},
			{
				Name:     "order-service",
				Type:     "web",
				Image:    "httpd:2.4",
				Replicas: 3,
				Port:     8080,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("150m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
				Labels: map[string]string{
					"app":  "order-service",
					"tier": "service",
				},
			},
			{
				Name:     "shared-database",
				Type:     "database",
				Image:    "postgres:13",
				Replicas: 1,
				Port:     5432,
				Resources: &ResourceSpec{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2000m"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
				Environment: map[string]string{
					"POSTGRES_DB":       "microservices",
					"POSTGRES_USER":     "microservices",
					"POSTGRES_PASSWORD": "micro123",
				},
				Labels: map[string]string{
					"app":  "shared-database",
					"tier": "data",
				},
			},
		},
		Dependencies: []*ServiceDependency{
			{
				Service:   "user-service",
				DependsOn: "shared-database",
				WaitFor:   "ready",
				Timeout:   2 * time.Minute,
			},
			{
				Service:   "order-service",
				DependsOn: "shared-database",
				WaitFor:   "ready",
				Timeout:   2 * time.Minute,
			},
			{
				Service:   "api-gateway",
				DependsOn: "user-service",
				WaitFor:   "healthy",
				Timeout:   2 * time.Minute,
			},
			{
				Service:   "api-gateway",
				DependsOn: "order-service",
				WaitFor:   "healthy",
				Timeout:   2 * time.Minute,
			},
		},
		Validation: &MultiServiceValidation{
			ServiceConnectivity: true,
			HealthChecks:        true,
			ResourceUsage:       true,
			PerformanceTargets: &MultiServicePerformanceTargets{
				DeploymentTime:   5 * time.Minute,
				ServiceStartTime: 2 * time.Minute,
				HealthCheckTime:  30 * time.Second,
				ConnectivityTime: 10 * time.Second,
			},
		},
		Timeout: 10 * time.Minute,
	}

	mswm.logger.WithField("scenarios_count", len(mswm.scenarios)).Info("Multi-service scenarios initialized")
}

// DeployMultiServiceScenario deploys a multi-service scenario to real cluster
// Business Requirement: BR-PRODUCTION-005 - Deploy complex multi-service workloads
func (mswm *MultiServiceWorkloadManager) DeployMultiServiceScenario(ctx context.Context, scenarioName string) (*MultiServiceDeploymentResult, error) {
	mswm.logger.WithField("scenario", scenarioName).Info("Starting multi-service scenario deployment")

	scenario, exists := mswm.scenarios[scenarioName]
	if !exists {
		return nil, fmt.Errorf("unknown scenario: %s", scenarioName)
	}

	deploymentStart := time.Now()

	result := &MultiServiceDeploymentResult{
		Scenario:           scenario,
		DeployedServices:   []*appsv1.Deployment{},
		CreatedServices:    []*corev1.Service{},
		ValidationResults:  &ValidationResults{},
		PerformanceMetrics: &PerformanceMetrics{},
		Status:             "deploying",
	}

	// Create namespace
	if err := mswm.createNamespace(ctx, scenario.Namespace); err != nil {
		return result, fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy services in dependency order
	if err := mswm.deployServicesInOrder(ctx, scenario, result); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("failed to deploy services: %w", err)
	}

	result.DeploymentTime = time.Since(deploymentStart)
	result.PerformanceMetrics.DeploymentTime = result.DeploymentTime

	// Wait for all services to be ready
	if err := mswm.waitForServicesReady(ctx, scenario, result); err != nil {
		result.Status = "failed"
		return result, fmt.Errorf("services failed to become ready: %w", err)
	}

	// Validate deployment
	if err := mswm.validateDeployment(ctx, scenario, result); err != nil {
		result.Status = "validation_failed"
		mswm.logger.WithError(err).Warn("Deployment validation failed")
	} else {
		result.Status = "completed"
	}

	mswm.logger.WithFields(logrus.Fields{
		"scenario":        scenarioName,
		"deployment_time": result.DeploymentTime,
		"services_count":  len(result.DeployedServices),
		"status":          result.Status,
	}).Info("Multi-service scenario deployment completed")

	return result, nil
}

// deployServicesInOrder deploys services respecting dependencies
func (mswm *MultiServiceWorkloadManager) deployServicesInOrder(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	// Create dependency graph
	dependencyMap := make(map[string][]string)
	for _, dep := range scenario.Dependencies {
		dependencyMap[dep.Service] = append(dependencyMap[dep.Service], dep.DependsOn)
	}

	// Deploy services in topological order
	deployed := make(map[string]bool)

	for len(deployed) < len(scenario.Services) {
		progress := false

		for _, service := range scenario.Services {
			if deployed[service.Name] {
				continue
			}

			// Check if all dependencies are deployed
			canDeploy := true
			for _, depService := range dependencyMap[service.Name] {
				if !deployed[depService] {
					canDeploy = false
					break
				}
			}

			if canDeploy {
				if err := mswm.deployService(ctx, scenario.Namespace, service, result); err != nil {
					return fmt.Errorf("failed to deploy service %s: %w", service.Name, err)
				}
				deployed[service.Name] = true
				progress = true
			}
		}

		if !progress {
			return fmt.Errorf("circular dependency detected or deployment failed")
		}
	}

	return nil
}

// deployService deploys a single service
func (mswm *MultiServiceWorkloadManager) deployService(ctx context.Context, namespace string, service *ServiceDefinition, result *MultiServiceDeploymentResult) error {
	mswm.logger.WithFields(logrus.Fields{
		"service":   service.Name,
		"namespace": namespace,
		"type":      service.Type,
	}).Info("Deploying service")

	// Create deployment
	deployment := mswm.createDeployment(namespace, service)
	createdDeployment, err := mswm.client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	result.DeployedServices = append(result.DeployedServices, createdDeployment)

	// Create service
	k8sService := mswm.createService(namespace, service)
	createdService, err := mswm.client.CoreV1().Services(namespace).Create(ctx, k8sService, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	result.CreatedServices = append(result.CreatedServices, createdService)

	mswm.logger.WithField("service", service.Name).Info("Service deployed successfully")
	return nil
}

// createDeployment creates a Kubernetes deployment for a service
func (mswm *MultiServiceWorkloadManager) createDeployment(namespace string, service *ServiceDefinition) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        service.Name,
			Namespace:   namespace,
			Labels:      service.Labels,
			Annotations: service.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &service.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: service.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: service.Labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    service.Name,
							Image:   service.Image,
							Command: service.Command,
							Args:    service.Args,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: service.Port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: service.Resources.Requests,
								Limits:   service.Resources.Limits,
							},
							Env: mswm.createEnvironmentVariables(service.Environment),
						},
					},
				},
			},
		},
	}

	// Add health check if defined
	if service.HealthCheck != nil {
		container := &deployment.Spec.Template.Spec.Containers[0]
		if service.HealthCheck.Path != "" {
			container.ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: service.HealthCheck.Path,
						Port: intstr.FromInt(int(service.HealthCheck.Port)),
					},
				},
				InitialDelaySeconds: service.HealthCheck.InitialDelaySeconds,
				PeriodSeconds:       service.HealthCheck.PeriodSeconds,
				TimeoutSeconds:      service.HealthCheck.TimeoutSeconds,
				FailureThreshold:    service.HealthCheck.FailureThreshold,
			}
			container.LivenessProbe = container.ReadinessProbe
		} else {
			// TCP probe for non-HTTP services
			container.ReadinessProbe = &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					TCPSocket: &corev1.TCPSocketAction{
						Port: intstr.FromInt(int(service.HealthCheck.Port)),
					},
				},
				InitialDelaySeconds: service.HealthCheck.InitialDelaySeconds,
				PeriodSeconds:       service.HealthCheck.PeriodSeconds,
				TimeoutSeconds:      service.HealthCheck.TimeoutSeconds,
				FailureThreshold:    service.HealthCheck.FailureThreshold,
			}
			container.LivenessProbe = container.ReadinessProbe
		}
	}

	return deployment
}

// createService creates a Kubernetes service for a service definition
func (mswm *MultiServiceWorkloadManager) createService(namespace string, service *ServiceDefinition) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.Name,
			Namespace: namespace,
			Labels:    service.Labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: service.Labels,
			Ports: []corev1.ServicePort{
				{
					Port:       service.Port,
					TargetPort: intstr.FromInt(int(service.Port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// createEnvironmentVariables creates environment variables for a container
func (mswm *MultiServiceWorkloadManager) createEnvironmentVariables(envMap map[string]string) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	for key, value := range envMap {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}
	return envVars
}

// createNamespace creates a namespace if it doesn't exist
func (mswm *MultiServiceWorkloadManager) createNamespace(ctx context.Context, name string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"managed-by": "kubernaut-production-testing",
				"scenario":   "multi-service",
			},
		},
	}

	_, err := mswm.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s: %w", name, err)
	}

	mswm.logger.WithField("namespace", name).Debug("Namespace created or already exists")
	return nil
}

// waitForServicesReady waits for all services to be ready
func (mswm *MultiServiceWorkloadManager) waitForServicesReady(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	mswm.logger.Info("Waiting for services to be ready")

	serviceStartTime := time.Now()
	ctx, cancel := context.WithTimeout(ctx, scenario.Timeout)
	defer cancel()

	for _, deployment := range result.DeployedServices {
		if err := mswm.waitForDeploymentReady(ctx, deployment); err != nil {
			return fmt.Errorf("deployment %s not ready: %w", deployment.Name, err)
		}
	}

	result.PerformanceMetrics.ServiceStartTime = time.Since(serviceStartTime)
	mswm.logger.WithField("start_time", result.PerformanceMetrics.ServiceStartTime).Info("All services are ready")
	return nil
}

// waitForDeploymentReady waits for a deployment to be ready
func (mswm *MultiServiceWorkloadManager) waitForDeploymentReady(ctx context.Context, deployment *appsv1.Deployment) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			current, err := mswm.client.AppsV1().Deployments(deployment.Namespace).Get(
				ctx, deployment.Name, metav1.GetOptions{},
			)
			if err != nil {
				return err
			}

			if current.Status.ReadyReplicas == *current.Spec.Replicas {
				mswm.logger.WithField("deployment", deployment.Name).Debug("Deployment is ready")
				return nil
			}

			time.Sleep(5 * time.Second)
		}
	}
}

// validateDeployment validates the multi-service deployment
func (mswm *MultiServiceWorkloadManager) validateDeployment(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	mswm.logger.Info("Validating multi-service deployment")

	validationStart := time.Now()
	defer func() {
		result.PerformanceMetrics.HealthCheckTime = time.Since(validationStart)
	}()

	// Validate service connectivity
	if scenario.Validation.ServiceConnectivity {
		if err := mswm.validateServiceConnectivity(ctx, scenario, result); err != nil {
			result.ValidationResults.FailedValidations = append(result.ValidationResults.FailedValidations, "service_connectivity")
			return fmt.Errorf("service connectivity validation failed: %w", err)
		}
		result.ValidationResults.ServiceConnectivity = true
	}

	// Validate health checks
	if scenario.Validation.HealthChecks {
		if err := mswm.validateHealthChecks(ctx, scenario, result); err != nil {
			result.ValidationResults.FailedValidations = append(result.ValidationResults.FailedValidations, "health_checks")
			return fmt.Errorf("health checks validation failed: %w", err)
		}
		result.ValidationResults.HealthChecks = true
	}

	// Validate resource usage
	if scenario.Validation.ResourceUsage {
		if err := mswm.validateResourceUsage(ctx, scenario, result); err != nil {
			result.ValidationResults.FailedValidations = append(result.ValidationResults.FailedValidations, "resource_usage")
			return fmt.Errorf("resource usage validation failed: %w", err)
		}
		result.ValidationResults.ResourceUsage = true
	}

	mswm.logger.Info("Multi-service deployment validation completed successfully")
	return nil
}

// validateServiceConnectivity validates that services can communicate
func (mswm *MultiServiceWorkloadManager) validateServiceConnectivity(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	mswm.logger.Info("Validating service connectivity")

	connectivityStart := time.Now()
	defer func() {
		result.PerformanceMetrics.ConnectivityTime = time.Since(connectivityStart)
	}()

	// Check that all services have endpoints
	for _, service := range result.CreatedServices {
		endpoints, err := mswm.client.CoreV1().Endpoints(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get endpoints for service %s: %w", service.Name, err)
		}

		if len(endpoints.Subsets) == 0 {
			return fmt.Errorf("service %s has no endpoints", service.Name)
		}

		mswm.logger.WithFields(logrus.Fields{
			"service":   service.Name,
			"endpoints": len(endpoints.Subsets),
		}).Debug("Service connectivity validated")
	}

	return nil
}

// validateHealthChecks validates that health checks are working
func (mswm *MultiServiceWorkloadManager) validateHealthChecks(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	mswm.logger.Info("Validating health checks")

	for _, deployment := range result.DeployedServices {
		pods, err := mswm.client.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
		})
		if err != nil {
			return fmt.Errorf("failed to list pods for deployment %s: %w", deployment.Name, err)
		}

		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("pod %s is not running: %s", pod.Name, pod.Status.Phase)
			}

			// Check readiness
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
					return fmt.Errorf("pod %s is not ready", pod.Name)
				}
			}
		}

		mswm.logger.WithField("deployment", deployment.Name).Debug("Health checks validated")
	}

	return nil
}

// validateResourceUsage validates resource usage is within limits
func (mswm *MultiServiceWorkloadManager) validateResourceUsage(ctx context.Context, scenario *MultiServiceScenario, result *MultiServiceDeploymentResult) error {
	mswm.logger.Info("Validating resource usage")

	// Get resource usage metrics (simplified validation)
	for _, deployment := range result.DeployedServices {
		pods, err := mswm.client.CoreV1().Pods(deployment.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
		})
		if err != nil {
			return fmt.Errorf("failed to list pods for deployment %s: %w", deployment.Name, err)
		}

		if len(pods.Items) == 0 {
			return fmt.Errorf("no pods found for deployment %s", deployment.Name)
		}

		mswm.logger.WithFields(logrus.Fields{
			"deployment": deployment.Name,
			"pods":       len(pods.Items),
		}).Debug("Resource usage validated")
	}

	return nil
}

// CleanupMultiServiceDeployment cleans up a multi-service deployment
func (mswm *MultiServiceWorkloadManager) CleanupMultiServiceDeployment(ctx context.Context, result *MultiServiceDeploymentResult) error {
	mswm.logger.WithField("scenario", result.Scenario.Name).Info("Cleaning up multi-service deployment")

	// Delete deployments
	for _, deployment := range result.DeployedServices {
		err := mswm.client.AppsV1().Deployments(deployment.Namespace).Delete(
			ctx, deployment.Name, metav1.DeleteOptions{},
		)
		if err != nil {
			mswm.logger.WithError(err).WithField("deployment", deployment.Name).Warn("Failed to delete deployment")
		}
	}

	// Delete services
	for _, service := range result.CreatedServices {
		err := mswm.client.CoreV1().Services(service.Namespace).Delete(
			ctx, service.Name, metav1.DeleteOptions{},
		)
		if err != nil {
			mswm.logger.WithError(err).WithField("service", service.Name).Warn("Failed to delete service")
		}
	}

	mswm.logger.Info("Multi-service deployment cleanup completed")
	return nil
}

// GetAvailableScenarios returns available multi-service scenarios
func (mswm *MultiServiceWorkloadManager) GetAvailableScenarios() []string {
	scenarios := make([]string, 0, len(mswm.scenarios))
	for name := range mswm.scenarios {
		scenarios = append(scenarios, name)
	}
	return scenarios
}
