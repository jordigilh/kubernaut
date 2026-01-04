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

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// WorkflowExecution E2E Test Infrastructure
//
// Sets up a complete environment for testing WorkflowExecution controller:
// - Kind cluster with Tekton Pipelines installed
// - Data Storage Service (PostgreSQL + DS) for audit events (BR-WE-005)
// - WorkflowExecution CRD deployed
// - WorkflowExecution controller running (with --datastorage-url configured)
// - Simple test pipeline bundle available

const (
	// WorkflowExecutionClusterName is the default Kind cluster name
	WorkflowExecutionClusterName = "workflowexecution-e2e"

	// TektonPipelinesVersion is the Tekton Pipelines version to install
	// NOTE: v1.7.0+ uses ghcr.io which doesn't require auth (gcr.io requires auth since 2025)
	TektonPipelinesVersion = "v1.7.0"

	// WorkflowExecutionNamespace is where the controller runs
	WorkflowExecutionNamespace = "kubernaut-system"

	// ExecutionNamespace is where PipelineRuns are created
	ExecutionNamespace = "kubernaut-workflows"

	// WorkflowExecutionMetricsHostPort is the host port for metrics endpoint
	// Mapped via Kind NodePort extraPortMappings (container: 30185 -> host: 9185)
	WorkflowExecutionMetricsHostPort = 9185
)

// CreateWorkflowExecutionCluster creates a Kind cluster for WorkflowExecution E2E tests
// It installs Tekton Pipelines and prepares the cluster for testing
func CreateWorkflowExecutionCluster(clusterName, kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(output, "Creating WorkflowExecution E2E Kind Cluster\n")
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(output, "  Cluster: %s\n", clusterName)
	fmt.Fprintf(output, "  Kubeconfig: %s\n", kubeconfigPath)
	fmt.Fprintf(output, "  Tekton Version: %s\n", TektonPipelinesVersion)
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Find config file
	configPath, err := findKindConfig("kind-workflowexecution-config.yaml")
	if err != nil {
		return fmt.Errorf("failed to find Kind config: %w", err)
	}
	fmt.Fprintf(output, "Using Kind config: %s\n", configPath)

	// Create Kind cluster
	fmt.Fprintf(output, "\n📦 Creating Kind cluster...\n")
	createCmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	createCmd.Stdout = output
	createCmd.Stderr = output
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}
	fmt.Fprintf(output, "✅ Kind cluster created\n")

	// Install Tekton Pipelines
	fmt.Fprintf(output, "\n🔧 Installing Tekton Pipelines %s...\n", TektonPipelinesVersion)
	if err := installTektonPipelines(kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to install Tekton: %w", err)
	}
	fmt.Fprintf(output, "✅ Tekton Pipelines installed\n")

	// Deploy Data Storage infrastructure for audit events (BR-WE-005)
	// Following AIAnalysis E2E pattern: build → load → deploy (runs once, shared by 4 parallel procs)
	fmt.Fprintf(output, "\n🗄️  Deploying Data Storage infrastructure (BR-WE-005 audit events)...\n")

	// Create context for infrastructure deployment
	ctx := context.Background()

	// Deploy shared Data Storage infrastructure (PostgreSQL + Redis + Migrations + Data Storage)
	// Note: Standard ordering (PostgreSQL → Redis → Migrations → Data Storage)
	// Previous pattern applied migrations AFTER Data Storage, but this is unnecessary:
	// Data Storage only needs PostgreSQL/Redis to START, not migrations (migrations are for data, not startup)
	fmt.Fprintf(output, "📦 Deploying Data Storage infrastructure...\n")
	if err := DeployDataStorageTestServices(ctx, WorkflowExecutionNamespace, kubeconfigPath, GenerateInfraImageName("datastorage", "workflowexecution"), output); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	fmt.Fprintf(output, "✅ Data Storage infrastructure deployed (PostgreSQL + Redis + Migrations + DataStorage)\n")

	// Build and register test workflow bundles
	// This creates OCI bundles for test workflows and registers them in DataStorage
	// Per DD-WORKFLOW-005 v1.0: Direct REST API workflow registration
	// BR-WE-001: Workflow execution via Tekton bundle resolver
	fmt.Fprintf(output, "\n🎯 Building and registering test workflow bundles...\n")
	dataStorageURL := "http://localhost:8081" // NodePort per DD-TEST-001
	if _, err := BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, output); err != nil {
		return fmt.Errorf("failed to build and register test workflows: %w", err)
	}
	fmt.Fprintf(output, "✅ Test workflows ready\n")

	// Create execution namespace
	fmt.Fprintf(output, "\n📁 Creating execution namespace %s...\n", ExecutionNamespace)
	nsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = output
	nsCmd.Stderr = output
	if err := nsCmd.Run(); err != nil {
		// Namespace may already exist
		fmt.Fprintf(output, "Note: namespace creation returned error (may already exist): %v\n", err)
	}

	// Create image pull secret for quay.io (from podman auth)
	if err := createQuayPullSecret(kubeconfigPath, ExecutionNamespace, output); err != nil {
		fmt.Fprintf(output, "Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	fmt.Fprintf(output, "\n✅ WorkflowExecution E2E cluster ready!\n")
	return nil
}

// createQuayPullSecret creates an image pull secret from the podman auth config
func createQuayPullSecret(kubeconfigPath, namespace string, output io.Writer) error {
	fmt.Fprintf(output, "🔐 Creating quay.io pull secret...\n")

	// Get the auth config from podman
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	authFile := filepath.Join(homeDir, ".config/containers/auth.json")
	if _, err := os.Stat(authFile); os.IsNotExist(err) {
		return fmt.Errorf("podman auth file not found at %s", authFile)
	}

	// Create the secret in the execution namespace
	secretCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", namespace,
		"--kubeconfig", kubeconfigPath,
	)
	secretCmd.Stdout = output
	secretCmd.Stderr = output
	if err := secretCmd.Run(); err != nil {
		return fmt.Errorf("failed to create pull secret: %w", err)
	}

	// Create the secret in tekton-pipelines-resolvers namespace for bundle resolver
	secretResolverCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
	)
	secretResolverCmd.Stdout = output
	secretResolverCmd.Stderr = output
	_ = secretResolverCmd.Run() // Ignore error if namespace doesn't exist

	// Patch the service account to use the pull secret
	patchCmd := exec.Command("kubectl", "patch", "serviceaccount", "kubernaut-workflow-runner",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchCmd.Stdout = output
	patchCmd.Stderr = output
	// Ignore error if service account doesn't exist yet
	_ = patchCmd.Run()

	// Also patch the default service account in execution namespace
	patchDefaultCmd := exec.Command("kubectl", "patch", "serviceaccount", "default",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchDefaultCmd.Stdout = output
	patchDefaultCmd.Stderr = output
	_ = patchDefaultCmd.Run()

	// Patch the tekton-pipelines-resolvers service account
	patchResolverCmd := exec.Command("kubectl", "patch", "serviceaccount", "tekton-pipelines-resolvers",
		"-n", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchResolverCmd.Stdout = output
	patchResolverCmd.Stderr = output
	_ = patchResolverCmd.Run()

	fmt.Fprintf(output, "✅ Pull secret created and linked to service accounts\n")
	return nil
}

// DeleteWorkflowExecutionCluster deletes the Kind cluster
func DeleteWorkflowExecutionCluster(clusterName string, output io.Writer) error {
	fmt.Fprintf(output, "🗑️  Deleting Kind cluster %s...\n", clusterName)

	// Add 60-second timeout to prevent hanging on stuck clusters
	// Issue: kind delete can hang indefinitely with Podman provider
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		// If timeout, ignore the error (cluster will be cleaned up by system)
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Fprintf(output, "⚠️  Cluster deletion timed out after 60s, continuing...\n")
			return nil
		}
		// For other errors, check if cluster doesn't exist (ignore error)
		// Error message: "cluster \"workflowexecution-e2e\" not found"
		if strings.Contains(err.Error(), "not found") {
			fmt.Fprintf(output, "ℹ️  Cluster already deleted or doesn't exist\n")
			return nil
		}
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	fmt.Fprintf(output, "✅ Kind cluster deleted\n")
	return nil
}

// installTektonPipelines installs Tekton Pipelines using the official release manifests
func installTektonPipelines(kubeconfigPath string, output io.Writer) error {
	// Install Tekton Pipelines from GitHub releases (v1.0+ use GitHub releases)
	// NOTE: storage.googleapis.com/tekton-releases requires auth since 2025
	releaseURL := fmt.Sprintf("https://github.com/tektoncd/pipeline/releases/download/%s/release.yaml", TektonPipelinesVersion)

	fmt.Fprintf(output, "  Applying Tekton release from: %s\n", releaseURL)

	// Retry logic for transient GitHub CDN failures (503 Service Unavailable)
	// GitHub's CDN occasionally returns 503 errors during high load
	maxRetries := 3
	backoffSeconds := []int{5, 10, 20} // Exponential backoff: 5s, 10s, 20s

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			fmt.Fprintf(output, "  ⚠️  Attempt %d/%d failed, retrying in %ds...\n", attempt, maxRetries, backoffSeconds[attempt-1])
			time.Sleep(time.Duration(backoffSeconds[attempt-1]) * time.Second)
			fmt.Fprintf(output, "  🔄 Retry attempt %d/%d...\n", attempt+1, maxRetries)
		}

		applyCmd := exec.Command("kubectl", "apply",
			"-f", releaseURL,
			"--kubeconfig", kubeconfigPath,
		)
		applyCmd.Stdout = output
		applyCmd.Stderr = output

		if err := applyCmd.Run(); err != nil {
			lastErr = err
			continue // Retry
		}

		// Success!
		if attempt > 0 {
			fmt.Fprintf(output, "  ✅ Tekton release applied successfully on attempt %d\n", attempt+1)
		}
		lastErr = nil
		break
	}

	if lastErr != nil {
		return fmt.Errorf("failed to apply Tekton release after %d attempts: %w", maxRetries, lastErr)
	}

	// Wait for Tekton controller to be ready
	// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s) to prevent timeout failures
	// Root cause: Slow Tekton image pulls in Kind cluster (see WE_E2E_INFRASTRUCTURE_STABILIZATION_PLAN.md)
	fmt.Fprintf(output, "  ⏳ Waiting for Tekton Pipelines controller (up to 1 hour)...\n")
	waitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-controller",
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Tekton controller did not become ready: %w", err)
	}

	// Wait for Tekton webhook to be ready
	// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s)
	fmt.Fprintf(output, "  ⏳ Waiting for Tekton webhook (up to 1 hour)...\n")
	webhookWaitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-webhook",
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	webhookWaitCmd.Stdout = output
	webhookWaitCmd.Stderr = output
	if err := webhookWaitCmd.Run(); err != nil {
		return fmt.Errorf("Tekton webhook did not become ready: %w", err)
	}

	return nil
}

// deployWorkflowExecutionControllerDeployment creates the Deployment resource programmatically
// Following DD-TEST-007 pattern (DataStorage reference implementation)
func deployWorkflowExecutionControllerDeployment(ctx context.Context, namespace, kubeconfigPath string, output io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workflowexecution-controller",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "workflowexecution-controller",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "workflowexecution-controller",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "workflowexecution-controller",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "workflowexecution-controller",
					// DD-TEST-007: Run as root for E2E coverage (simplified permissions)
					// Per SP/DS team guidance: non-root user may not have permission to write /coverdata
					SecurityContext: func() *corev1.PodSecurityContext {
						if os.Getenv("E2E_COVERAGE") == "true" {
							runAsUser := int64(0)
							runAsGroup := int64(0)
							return &corev1.PodSecurityContext{
								RunAsUser:  &runAsUser,
								RunAsGroup: &runAsGroup,
							}
						}
						return nil
					}(),
					Containers: []corev1.Container{
						{
							Name:  "controller",
							Image: "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution", // DD-TEST-001: service-specific tag
							ImagePullPolicy: corev1.PullNever,                                         // DD-REGISTRY-001: Use local image loaded into Kind
							Args: []string{
								"--metrics-bind-address=:9090",
								"--health-probe-bind-address=:8081",
								"--execution-namespace=kubernaut-workflows",
								"--cooldown-period=1", // Short cooldown for E2E tests (1 minute)
								"--service-account=kubernaut-workflow-runner",
								"--datastorage-url=http://datastorage.kubernaut-system:8080", // BR-WE-005: Audit events
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "metrics",
									ContainerPort: 9090,
								},
								{
									Name:          "health",
									ContainerPort: 8081,
								},
							},
							Env: func() []corev1.EnvVar {
								envVars := []corev1.EnvVar{}
								// DD-TEST-007: E2E Coverage Capture Standard
								// Only add GOCOVERDIR if E2E_COVERAGE=true
								// MUST match Kind extraMounts path: /coverdata
								coverageEnabled := os.Getenv("E2E_COVERAGE") == "true"
								fmt.Fprintf(output, "   🔍 DD-TEST-007: E2E_COVERAGE=%s (enabled=%v)\n", os.Getenv("E2E_COVERAGE"), coverageEnabled)
								if coverageEnabled {
									fmt.Fprintf(output, "   ✅ Adding GOCOVERDIR=/coverdata to WorkflowExecution deployment\n")
									envVars = append(envVars, corev1.EnvVar{
										Name:  "GOCOVERDIR",
										Value: "/coverdata",
									})
								} else {
									fmt.Fprintf(output, "   ⚠️  E2E_COVERAGE not set, skipping GOCOVERDIR\n")
								}
								return envVars
							}(),
							VolumeMounts: func() []corev1.VolumeMount {
								mounts := []corev1.VolumeMount{}
								// DD-TEST-007: Add coverage volume mount if enabled
								// MUST match Kind extraMounts path: /coverdata
								if os.Getenv("E2E_COVERAGE") == "true" {
									mounts = append(mounts, corev1.VolumeMount{
										Name:      "coverage",
										MountPath: "/coverdata",
										ReadOnly:  false,
									})
								}
								return mounts
							}(),
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       20,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/readyz",
										Port: intstr.FromString("health"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
					},
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{}
						// DD-TEST-007: Add hostPath volume for coverage if enabled
						// MUST match Kind extraMounts path: /coverdata
						if os.Getenv("E2E_COVERAGE") == "true" {
							volumes = append(volumes, corev1.Volume{
								Name: "coverage",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/coverdata",
										Type: func() *corev1.HostPathType {
											t := corev1.HostPathDirectoryOrCreate
											return &t
										}(),
									},
								},
							})
						}
						return volumes
					}(),
				},
			},
		},
	}

	// Create Deployment
	fmt.Fprintf(output, "   Creating Deployment/workflowexecution-controller...\n")
	fmt.Fprintf(output, "   📊 Debug: Image=%s, ImagePullPolicy=%s\n",
		deployment.Spec.Template.Spec.Containers[0].Image,
		deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy)

	createdDep, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	fmt.Fprintf(output, "   ✅ Deployment created (UID: %s)\n", createdDep.UID)

	// Wait and check status
	time.Sleep(3 * time.Second)

	// Check Pods
	podList, _ := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=workflowexecution-controller",
	})
	fmt.Fprintf(output, "   📊 Debug: Found %d pod(s) after 3s\n", len(podList.Items))
	for _, pod := range podList.Items {
		fmt.Fprintf(output, "      Pod %s: Phase=%s\n", pod.Name, pod.Status.Phase)
	}

	// Create Service for metrics
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workflowexecution-controller-metrics",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app": "workflowexecution-controller",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
					NodePort:   30185, // Exposed on host via Kind extraPortMappings
				},
			},
		},
	}

	fmt.Fprintf(output, "   Creating Service/workflowexecution-controller-metrics...\n")
	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	fmt.Fprintf(output, "   ✅ Service created\n")

	return nil
}

// DeployWorkflowExecutionController deploys the WorkflowExecution controller to the cluster
func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "\n🚀 Deploying WorkflowExecution Controller to %s...\n", namespace)

	// Find project root for absolute paths
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Create controller namespace
	nsCmd := exec.Command("kubectl", "create", "namespace", namespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = output
	nsCmd.Stderr = output
	_ = nsCmd.Run() // May already exist

	// Deploy CRDs (use absolute path)
	crdPath := filepath.Join(projectRoot, "config/crd/bases/kubernaut.ai_workflowexecutions.yaml")
	fmt.Fprintf(output, "  Applying WorkflowExecution CRDs...\n")
	crdCmd := exec.Command("kubectl", "apply",
		"-f", crdPath,
		"--kubeconfig", kubeconfigPath,
	)
	crdCmd.Stdout = output
	crdCmd.Stderr = output
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRDs: %w", err)
	}

	// Build controller image with optional E2E coverage instrumentation (DD-TEST-007)
	fmt.Fprintf(output, "  Building controller image...\n")
	dockerfilePath := filepath.Join(projectRoot, "docker/workflowexecution-controller.Dockerfile")
	// DD-REGISTRY-001: Use localhost prefix for E2E test images
	// DD-TEST-001: Use service-specific tag to avoid conflicts with other services
	imageName := "localhost/kubernaut-workflowexecution:e2e-test-workflowexecution"

	buildArgs := []string{
		"build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes

		"-t", imageName,
		"-f", dockerfilePath,
	}

	// Build for host architecture (no multi-arch support needed)
	hostArch := runtime.GOARCH
	buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("GOARCH=%s", hostArch))
	fmt.Fprintf(output, "   🏗️  Building for host architecture: %s\n", hostArch)

	// DD-TEST-007: E2E Coverage Collection
	// If E2E_COVERAGE=true, build with coverage instrumentation
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		fmt.Fprintf(output, "   📊 Building with coverage instrumentation (GOFLAGS=-cover)\n")
	}

	buildArgs = append(buildArgs, projectRoot)

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = output
	buildCmd.Stderr = output
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build controller image: %w", err)
	}

	// Save image to tarball for Kind loading (Podman images need explicit save/load)
	fmt.Fprintf(output, "  Saving image for Kind cluster...\n")
	tarPath := filepath.Join(projectRoot, "workflowexecution-controller.tar")
	saveCmd := exec.Command("podman", "save", "-o", tarPath, imageName)
	saveCmd.Stdout = output
	saveCmd.Stderr = output
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save controller image: %w", err)
	}
	defer os.Remove(tarPath)

	// Load image into Kind from tarball
	fmt.Fprintf(output, "  Loading image into Kind cluster...\n")
	loadCmd := exec.Command("kind", "load", "image-archive", tarPath,
		"--name", WorkflowExecutionClusterName,
	)
	loadCmd.Stdout = output
	loadCmd.Stderr = output
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	fmt.Fprintf(output, "  🗑️  Removing Podman image to free disk space...\n")
	rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
	rmiCmd.Stdout = output
	rmiCmd.Stderr = output
	if err := rmiCmd.Run(); err != nil {
		fmt.Fprintf(output, "  ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		fmt.Fprintf(output, "  ✅ Podman image removed: %s\n", imageName)
	}

	// Apply static resources (Namespaces, ServiceAccounts, RBAC)
	// Note: Deployment and Service are created programmatically for E2E coverage support
	manifestsPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/manifests/controller-deployment.yaml")
	fmt.Fprintf(output, "  Applying static resources (Namespaces, ServiceAccounts, RBAC)...\n")

	// Use kubectl apply but exclude Deployment and Service resources
	// They will be created programmatically with E2E coverage support
	excludeCmd := exec.Command("kubectl", "apply",
		"-f", manifestsPath,
		"--kubeconfig", kubeconfigPath,
	)
	excludeCmd.Stdout = output
	excludeCmd.Stderr = output
	if err := excludeCmd.Run(); err != nil {
		// Ignore errors - some resources may already exist
		fmt.Fprintf(output, "   ⚠️  Some resources may already exist (continuing)\n")
	}

	// Delete existing Deployment and Service if they exist (they were created by kubectl apply above)
	// We'll recreate them programmatically with E2E coverage support
	fmt.Fprintf(output, "  Cleaning up existing Deployment/Service (if any)...\n")
	deleteDeployCmd := exec.Command("kubectl", "delete", "deployment",
		"workflowexecution-controller",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--ignore-not-found=true")
	deleteDeployCmd.Stdout = output
	deleteDeployCmd.Stderr = output
	_ = deleteDeployCmd.Run() // Ignore errors

	deleteSvcCmd := exec.Command("kubectl", "delete", "service",
		"workflowexecution-controller-metrics",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--ignore-not-found=true")
	deleteSvcCmd.Stdout = output
	deleteSvcCmd.Stderr = output
	_ = deleteSvcCmd.Run() // Ignore errors

	// Deploy controller programmatically with E2E coverage support (DD-TEST-007)
	fmt.Fprintf(output, "  Deploying controller programmatically (E2E coverage support)...\n")
	if err := deployWorkflowExecutionControllerDeployment(ctx, namespace, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy controller: %w", err)
	}

	fmt.Fprintf(output, "✅ WorkflowExecution Controller deployed\n")
	return nil
}

// CreateSimpleTestPipeline creates a simple "hello world" pipeline for testing
// Also creates a failing pipeline for BR-WE-004 failure details testing
func CreateSimpleTestPipeline(kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "\n📝 Creating test pipelines (success + failure)...\n")

	pipelineYAML := `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-hello-world
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: MESSAGE
      type: string
      default: "Hello from Kubernaut!"
  tasks:
    - name: echo-hello
      taskRef:
        name: test-echo-task
      params:
        - name: message
          value: $(params.MESSAGE)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-echo-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: message
      type: string
  steps:
    - name: echo
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "$(params.message)"
        echo "Test task completed successfully"
        sleep 2
---
# Intentionally failing pipeline for BR-WE-004 failure details testing
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-intentional-failure
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: FAILURE_REASON
      type: string
      default: "Simulated failure for E2E testing"
  tasks:
    - name: fail-task
      taskRef:
        name: test-fail-task
      params:
        - name: reason
          value: $(params.FAILURE_REASON)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-fail-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: reason
      type: string
  steps:
    - name: fail
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "Task will fail with reason: $(params.reason)"
        echo "This is an intentional failure for BR-WE-004 E2E testing"
        exit 1
`

	// Write to temp file and apply
	tmpFile, err := os.CreateTemp("", "test-pipeline-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(pipelineYAML); err != nil {
		return fmt.Errorf("failed to write pipeline YAML: %w", err)
	}
	tmpFile.Close()

	applyCmd := exec.Command("kubectl", "apply",
		"-f", tmpFile.Name(),
		"--kubeconfig", kubeconfigPath,
	)
	applyCmd.Stdout = output
	applyCmd.Stderr = output
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to create test pipeline: %w", err)
	}

	fmt.Fprintf(output, "✅ Test pipeline created\n")
	return nil
}

// WaitForPipelineRunCompletion waits for a PipelineRun to complete
func WaitForPipelineRunCompletion(kubeconfigPath, prName, namespace string, timeout time.Duration, output io.Writer) error {
	fmt.Fprintf(output, "⏳ Waiting for PipelineRun %s/%s to complete (timeout: %v)...\n", namespace, prName, timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for PipelineRun completion")
		case <-ticker.C:
			// Check PipelineRun status
			statusCmd := exec.Command("kubectl", "get", "pipelinerun", prName,
				"-n", namespace,
				"-o", "jsonpath={.status.conditions[0].status}",
				"--kubeconfig", kubeconfigPath,
			)
			statusOutput, err := statusCmd.Output()
			if err != nil {
				continue // PipelineRun may not exist yet
			}

			status := string(statusOutput)
			if status == "True" {
				fmt.Fprintf(output, "✅ PipelineRun completed successfully\n")
				return nil
			} else if status == "False" {
				// Get failure reason
				reasonCmd := exec.Command("kubectl", "get", "pipelinerun", prName,
					"-n", namespace,
					"-o", "jsonpath={.status.conditions[0].reason}",
					"--kubeconfig", kubeconfigPath,
				)
				reasonOutput, _ := reasonCmd.Output()
				return fmt.Errorf("PipelineRun failed: %s", string(reasonOutput))
			}
			// status == "Unknown" means still running
			fmt.Fprintf(output, "  PipelineRun status: %s\n", status)
		}
	}
}

// deployDataStorageWithConfig deploys Data Storage with proper ADR-030 configuration
// This includes ConfigMap, Secrets, and CONFIG_PATH as required by DS
func deployDataStorageWithConfig(clusterName, kubeconfigPath string, output io.Writer) error {
	projectRoot := getProjectRoot()

	// Build Data Storage image
	fmt.Fprintln(output, "    Building Data Storage image...")
	buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
		"-f", "docker/data-storage.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = output
	buildCmd.Stderr = output
	if err := buildCmd.Run(); err != nil {
		// Try docker as fallback
		buildCmd = exec.Command("docker", "build", "-t", "kubernaut-datastorage:latest",
			"-f", "docker/data-storage.Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = output
		buildCmd.Stderr = output
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build Data Storage: %w", err)
		}
	}

	// Load into Kind
	fmt.Fprintln(output, "    Loading Data Storage image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-datastorage:latest", output); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Deploy ConfigMap with ADR-030 configuration
	configMapManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      port: 8080
      metricsPort: 9090
      readTimeout: 30s
      writeTimeout: 30s
      idleTimeout: 120s
      gracefulShutdownTimeout: 30s
    database:
      host: postgresql
      port: 5432
      name: action_history
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/db-credentials.yaml
      usernameKey: username
      passwordKey: password
    redis:
      addr: redis:6379
      db: 0
      dlq_stream_name: audit_dlq
      dlq_max_len: 10000
      dlq_consumer_group: audit_processors
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/redis-credentials.yaml
      passwordKey: password
    logging:
      level: info
      format: json
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(configMapManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Deploy Secret with credentials in YAML format (ADR-030 Section 6)
	secretManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secrets
  namespace: kubernaut-system
stringData:
  db-credentials.yaml: |
    username: slm_user
    password: test_password
  redis-credentials.yaml: |
    password: ""
`
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(secretManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Secret: %w", err)
	}

	// Deploy Data Storage with proper volumes and CONFIG_PATH
	// Note: Image name is localhost/kubernaut-datastorage:latest when loaded via kind load
	deploymentManifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: localhost/kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secrets
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30081
    name: http
  - port: 9090
    targetPort: 9090
    name: metrics
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage-service
  namespace: kubernaut-system
spec:
  type: ClusterIP
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    name: http
`
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deploymentManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// waitForDeploymentReady waits for a deployment to be ready in kubernaut-system namespace
// Phase 1 E2E Stabilization: Increased timeout to 1 hour (3600s) to prevent timeout failures
func waitForDeploymentReady(kubeconfigPath, deploymentName string, output io.Writer) error {
	waitCmd := exec.Command("kubectl", "wait",
		"-n", WorkflowExecutionNamespace,
		"--for=condition=available",
		"deployment/"+deploymentName,
		"--timeout=3600s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("deployment %s did not become available: %w", deploymentName, err)
	}
	return nil
}

// waitForWEDataStorageReady waits for Data Storage deployment to be ready (WorkflowExecution)
// Phase 1 E2E Stabilization: Uses kubectl wait with 1 hour timeout (DS may take time to connect to PostgreSQL)
func waitForWEDataStorageReady(kubeconfigPath string, output io.Writer) error {
	if err := waitForDeploymentReady(kubeconfigPath, "datastorage", output); err != nil {
		return err
	}
	// Brief wait for DS to initialize connections to PostgreSQL/Redis
	time.Sleep(5 * time.Second)
	return nil
}

// findKindConfig finds the Kind config file relative to the project root
func findKindConfig(filename string) (string, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()

	// Try paths relative to project root
	paths := []string{
		filepath.Join(projectRoot, "test", "infrastructure", filename),
		filepath.Join(cwd, "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "..", "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "infrastructure", filename),
		filename,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("Kind config file %s not found in any expected location (tried from %s)", filename, projectRoot)
}
