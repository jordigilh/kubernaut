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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SignalProcessing E2E Infrastructure
// Per IMPLEMENTATION_PLAN_V1.30.md Day 11 and DD-TEST-001 Port Allocation
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Port Allocation (from DD-TEST-001 - AUTHORITATIVE):
//   Internal Metrics: 9090 (container)
//   Internal Health: 8081 (container)
//   Host Port: 8082 (Kind extraPortMappings)
//   NodePort (API): 30082
//   NodePort (Metrics): 30182
//   Host Metrics: 9182 (localhost:9182 → 30182)
//
// Kubeconfig Convention (per TESTING_GUIDELINES.md):
//   Pattern: ~/.kube/{service}-e2e-config
//   Path: ~/.kube/signalprocessing-e2e-config
//   Cluster Name: signalprocessing-e2e
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateSignalProcessingCluster creates a Kind cluster for SignalProcessing E2E testing.
// This is called ONCE in BeforeSuite.
//
// Steps:
// 1. Create Kind cluster with SignalProcessing-specific configuration
// 2. Export kubeconfig to ~/.kube/signalprocessing-e2e-config
// 3. Install SignalProcessing CRD (cluster-wide resource)
// 4. Create kubernaut-system namespace
// 5. Deploy Rego policy ConfigMaps
// 6. Build and load SignalProcessing controller image
//
// Time: ~60 seconds
func CreateSignalProcessingCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "SignalProcessing E2E Cluster Setup (ONCE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install SignalProcessing CRD
	fmt.Fprintln(writer, "📋 Installing SignalProcessing CRD...")
	if err := installSignalProcessingCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install SignalProcessing CRD: %w", err)
	}

	// 2a. Install RemediationRequest CRD (parent of SignalProcessing)
	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	if err := installRemediationRequestCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// 3. Create kubernaut-system namespace
	fmt.Fprintln(writer, "📁 Creating kubernaut-system namespace...")
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 4. Deploy Rego policy ConfigMaps
	fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	// 5. Build SignalProcessing controller image
	fmt.Fprintln(writer, "🔨 Building SignalProcessing controller image...")
	if err := buildSignalProcessingImage(writer); err != nil {
		return fmt.Errorf("failed to build controller image: %w", err)
	}

	// 6. Load image into Kind cluster
	fmt.Fprintln(writer, "📦 Loading SignalProcessing image into Kind...")
	if err := loadSignalProcessingImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load controller image: %w", err)
	}

	fmt.Fprintln(writer, "✅ SignalProcessing cluster ready for E2E tests")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployDataStorageForSignalProcessing deploys DataStorage infrastructure for BR-SP-090 audit testing.
// This includes PostgreSQL, Redis, migrations, and DataStorage service in kubernaut-system namespace.
//
// Prerequisites:
// - Kind cluster must be created (CreateSignalProcessingCluster)
// - kubernaut-system namespace must exist
//
// This enables the SignalProcessing controller to write audit events to DataStorage.
func DeployDataStorageForSignalProcessing(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "BR-SP-090: Deploying DataStorage for Audit Testing")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// 1. Build DataStorage image (if not already done)
	fmt.Fprintln(writer, "🔨 Building DataStorage image...")
	if err := buildDataStorageImage(writer); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	// 2. Load DataStorage image into Kind
	fmt.Fprintln(writer, "📦 Loading DataStorage image into Kind...")
	if err := loadDataStorageImageForSP(writer); err != nil {
		return fmt.Errorf("failed to load DataStorage image: %w", err)
	}

	// 3. Deploy PostgreSQL
	fmt.Fprintln(writer, "🚀 Deploying PostgreSQL...")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 4. Deploy Redis (required by DataStorage)
	fmt.Fprintln(writer, "🚀 Deploying Redis...")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 5. Apply audit migrations using shared library
	fmt.Fprintln(writer, "📋 Applying audit migrations...")
	if err := ApplyAuditMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// 6. Deploy DataStorage service
	fmt.Fprintln(writer, "🚀 Deploying DataStorage service...")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// 7. Wait for DataStorage to be ready
	fmt.Fprintln(writer, "⏳ Waiting for DataStorage to be ready...")
	if err := waitForSPDataStorageReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage not ready: %w", err)
	}

	fmt.Fprintln(writer, "✅ DataStorage infrastructure ready for BR-SP-090 audit testing")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// loadDataStorageImageForSP loads the DataStorage image into the SP Kind cluster
func loadDataStorageImageForSP(writer io.Writer) error {
	// Get cluster name - should match what's used in CreateSignalProcessingCluster
	clusterName := "signalprocessing-e2e"

	// Save image to tar (following Gateway pattern - more reliable with Podman)
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-datastorage:e2e-test", "-o", "/tmp/datastorage-e2e-sp.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image archive into Kind cluster
	cmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e-sp.tar", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load DataStorage image into Kind: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ DataStorage image loaded into Kind cluster")
	return nil
}

// waitForSPDataStorageReady waits for DataStorage service to be ready in SP E2E tests
func waitForSPDataStorageReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if DataStorage pod is ready
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", namespace, "-l", "app=datastorage",
			"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")

		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "True" {
			// Also check if service is accessible
			healthCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"exec", "-n", namespace, "deploy/datastorage", "--",
				"curl", "-sf", "http://localhost:8080/health")
			if healthCmd.Run() == nil {
				fmt.Fprintln(writer, "  ✅ DataStorage is ready and healthy")
				return nil
			}
		}

		fmt.Fprintln(writer, "  ⏳ DataStorage not ready yet, waiting...")
		time.Sleep(interval)
	}

	return fmt.Errorf("DataStorage not ready after %v", timeout)
}

// DeleteSignalProcessingCluster deletes the Kind cluster and cleans up resources.
func DeleteSignalProcessingCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🗑️  Deleting SignalProcessing E2E cluster...")

	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	// Remove kubeconfig file
	if kubeconfigPath != "" {
		_ = os.Remove(kubeconfigPath)
	}

	fmt.Fprintln(writer, "✅ SignalProcessing cluster deleted")
	return nil
}

// DeploySignalProcessingController deploys the controller to the cluster.
// This should be called after CreateSignalProcessingCluster.
func DeploySignalProcessingController(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🚀 Deploying SignalProcessing controller...")

	// Deploy controller manager
	manifest := signalProcessingControllerManifest()
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy controller: %w", err)
	}

	// Wait for controller to be ready
	fmt.Fprintln(writer, "⏳ Waiting for controller to be ready...")
	return waitForSignalProcessingController(ctx, kubeconfigPath, writer)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal helper functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func createSignalProcessingKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Find Kind config file
	configPath := findSignalProcessingKindConfig()
	if configPath == "" {
		return fmt.Errorf("SignalProcessing Kind config not found")
	}

	// Check if cluster already exists
	checkCmd := exec.Command("kind", "get", "clusters")
	output, _ := checkCmd.Output()
	if strings.Contains(string(output), clusterName) {
		fmt.Fprintln(writer, "  Cluster already exists, reusing...")
		return exportSignalProcessingKubeconfig(clusterName, kubeconfigPath, writer)
	}

	// Create cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	// Wait for cluster to be ready
	return waitForSignalProcessingClusterReady(kubeconfigPath, writer)
}

func findSignalProcessingKindConfig() string {
	// Try to find config using various relative paths and also from GOPATH
	paths := []string{
		"test/infrastructure/kind-signalprocessing-config.yaml",
		"../infrastructure/kind-signalprocessing-config.yaml",
		"../../test/infrastructure/kind-signalprocessing-config.yaml",
		"../../../test/infrastructure/kind-signalprocessing-config.yaml",
	}

	// Also try from runtime.Caller to get the file's directory
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		thisDir := filepath.Dir(thisFile)
		paths = append(paths, filepath.Join(thisDir, "kind-signalprocessing-config.yaml"))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}
	return ""
}

func exportSignalProcessingKubeconfig(clusterName, kubeconfigPath string, writer io.Writer) error {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", clusterName,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func waitForSignalProcessingClusterReady(kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "True") {
			fmt.Fprintln(writer, "  ✓ Cluster ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("cluster not ready after 120 seconds")
}

func installSignalProcessingCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPaths := []string{
		"config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml",
		"../../../config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml",
	}

	var crdPath string
	for _, p := range crdPaths {
		if _, err := os.Stat(p); err == nil {
			crdPath, _ = filepath.Abs(p)
			break
		}
	}

	if crdPath == "" {
		return fmt.Errorf("SignalProcessing CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install CRD: %w", err)
	}

	// Wait for CRD to be established
	fmt.Fprintln(writer, "  Waiting for CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "signalprocessings.signalprocessing.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ CRD established")
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("CRD not established after 30 seconds")
}

func installRemediationRequestCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPaths := []string{
		"config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml",
		"../../../config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml",
	}

	var crdPath string
	for _, p := range crdPaths {
		if _, err := os.Stat(p); err == nil {
			crdPath, _ = filepath.Abs(p)
			break
		}
	}

	if crdPath == "" {
		return fmt.Errorf("RemediationRequest CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Wait for CRD to be established
	fmt.Fprintln(writer, "  Waiting for RemediationRequest CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "remediationrequests.remediation.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ RemediationRequest CRD established")
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("RemediationRequest CRD not established after 30 seconds")
}

func createSignalProcessingNamespace(kubeconfigPath string, writer io.Writer) error {
	manifest := `
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: signalprocessing
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deploySignalProcessingPolicies(kubeconfigPath string, writer io.Writer) error {
	// Deploy environment classification policy
	envPolicy := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-environment-policy
  namespace: kubernaut-system
data:
  environment.rego: |
    package signalprocessing.environment

    # Default environment classification based on namespace
    default environment = "unknown"

    environment = "production" {
      input.namespace == "production"
    }
    environment = "production" {
      input.namespace == "prod"
    }
    environment = "staging" {
      input.namespace == "staging"
    }
    environment = "development" {
      input.namespace == "development"
    }
    environment = "development" {
      input.namespace == "dev"
    }
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(envPolicy)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy environment policy: %w", err)
	}

	// Deploy priority assignment policy
	priorityPolicy := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-priority-policy
  namespace: kubernaut-system
data:
  priority.rego: |
    package signalprocessing.priority

    # Priority assignment based on environment and severity
    default priority = "P3"

    priority = "P0" {
      input.environment == "production"
      input.severity == "critical"
    }
    priority = "P1" {
      input.environment == "production"
      input.severity == "warning"
    }
    priority = "P1" {
      input.environment == "staging"
      input.severity == "critical"
    }
    priority = "P2" {
      input.environment == "staging"
      input.severity == "warning"
    }
    priority = "P2" {
      input.environment == "development"
      input.severity == "critical"
    }
`
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(priorityPolicy)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy priority policy: %w", err)
	}

	fmt.Fprintln(writer, "  ✓ Rego policies deployed")
	return nil
}

func buildSignalProcessingImage(writer io.Writer) error {
	// Find project root
	projectRoot := findSignalProcessingProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	// Use the Dockerfile in docker/ directory
	dockerfilePath := filepath.Join(projectRoot, "docker", "signalprocessing.Dockerfile")

	// Check if Dockerfile exists
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("SignalProcessing Dockerfile not found at %s", dockerfilePath)
	}

	// Build controller image using podman (preferred) or docker
	containerCmd := "podman"
	if _, err := exec.LookPath("podman"); err != nil {
		containerCmd = "docker"
	}

	cmd := exec.Command(containerCmd, "build",
		"-t", "localhost/signalprocessing-controller:e2e",
		"-f", dockerfilePath,
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	return cmd.Run()
}

func loadSignalProcessingImage(clusterName string, writer io.Writer) error {
	// When using podman with Kind, we need to save the image to a tar file
	// and load it using kind load image-archive
	tmpFile := filepath.Join(os.TempDir(), "signalprocessing-controller-e2e.tar")

	// Save image to tar file
	fmt.Fprintln(writer, "  Saving image to tar file...")
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		"localhost/signalprocessing-controller:e2e",
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind using image-archive
	fmt.Fprintln(writer, "  Loading image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		// Cleanup tmp file
		os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Cleanup tmp file
	os.Remove(tmpFile)
	return nil
}

func findSignalProcessingProjectRoot() string {
	// Try to find go.mod to determine project root
	paths := []string{
		".",
		"..",
		"../..",
		"../../..",
	}
	for _, p := range paths {
		goMod := filepath.Join(p, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}
	return ""
}

func signalProcessingControllerManifest() string {
	return `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: signalprocessing-controller
rules:
- apiGroups: ["signalprocessing.kubernaut.ai"]
  resources: ["signalprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["signalprocessing.kubernaut.ai"]
  resources: ["signalprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["pods", "services", "configmaps", "namespaces", "nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: signalprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: signalprocessing-controller
subjects:
- kind: ServiceAccount
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: signalprocessing-controller
  template:
    metadata:
      labels:
        app: signalprocessing-controller
    spec:
      serviceAccountName: signalprocessing-controller
      containers:
      - name: controller
        image: localhost/signalprocessing-controller:e2e
        imagePullPolicy: Never
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        env:
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        # BR-SP-090: Point to DataStorage service in kubernaut-system namespace
        - name: DATA_STORAGE_URL
          value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 500m
            memory: 256Mi
          requests:
            cpu: 100m
            memory: 64Mi
        volumeMounts:
        - name: policies
          mountPath: /etc/signalprocessing/policies
          readOnly: true
      volumes:
      - name: policies
        projected:
          sources:
          - configMap:
              name: signalprocessing-environment-policy
          - configMap:
              name: signalprocessing-priority-policy
---
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller-metrics
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: signalprocessing-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30182
`
}

func waitForSignalProcessingController(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	timeout := 120 * time.Second
	interval := 2 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/signalprocessing-controller",
			"-n", "kubernaut-system", "--timeout=5s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ Controller ready")
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("controller not ready after %v", timeout)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SignalProcessing Integration Test Infrastructure (Podman Compose)
// Per DD-TEST-001: Port Allocation Strategy
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// SignalProcessing integration test ports (per DD-TEST-001)
	// NOTE: RO uses 15435/16381, so SP gets the next available
	SignalProcessingIntegrationPostgresPort    = 15436 // PostgreSQL for audit storage
	SignalProcessingIntegrationRedisPort       = 16382 // Redis for DataStorage DLQ
	SignalProcessingIntegrationDataStoragePort = 18094 // DataStorage API for audit events

	// Compose configuration
	SignalProcessingIntegrationComposeFile    = "test/integration/signalprocessing/podman-compose.signalprocessing.test.yml"
	SignalProcessingIntegrationComposeProject = "signalprocessing_integration_test"
)

// StartSignalProcessingIntegrationInfrastructure starts the full podman-compose stack for SignalProcessing integration tests
// This includes: PostgreSQL, Redis, and DataStorage API (for BR-SP-090 audit trail)
//
// Port Allocation (per DD-TEST-001):
//   - PostgreSQL:     15435
//   - Redis:          16381
//   - DataStorage:    18094
//
// This function is designed to be called from SynchronizedBeforeSuite (Process 1 only)
func StartSignalProcessingIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, SignalProcessingIntegrationComposeFile)

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting SignalProcessing Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d (RO:15435, SP:15436)\n", SignalProcessingIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d (RO:16381, SP:16382)\n", SignalProcessingIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", SignalProcessingIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  Compose File:   %s\n", SignalProcessingIntegrationComposeFile)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if podman-compose is available
	if err := exec.Command("podman-compose", "--version").Run(); err != nil {
		return fmt.Errorf("podman-compose not found: %w (install via: pip install podman-compose)", err)
	}

	// Start services
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", SignalProcessingIntegrationComposeProject,
		"up", "-d", "--build",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(writer, "⏳ Starting containers...\n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start podman-compose stack: %w", err)
	}

	// Wait for DataStorage to be healthy (includes PostgreSQL + Redis dependencies)
	fmt.Fprintf(writer, "⏳ Waiting for DataStorage to be healthy (includes PostgreSQL + Redis)...\n")
	if err := waitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", SignalProcessingIntegrationDataStoragePort),
		120*time.Second, // Longer timeout for migrations
	); err != nil {
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage is healthy (PostgreSQL + Redis ready)\n")

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ SignalProcessing Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopSignalProcessingIntegrationInfrastructure stops and cleans up the SignalProcessing integration test infrastructure
func StopSignalProcessingIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, SignalProcessingIntegrationComposeFile)

	fmt.Fprintf(writer, "🛑 Stopping SignalProcessing Integration Infrastructure...\n")

	// Stop and remove containers
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", SignalProcessingIntegrationComposeProject,
		"down", "-v",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Error stopping infrastructure: %v\n", err)
		return err
	}

	fmt.Fprintf(writer, "✅ SignalProcessing Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// waitForHTTPHealth waits for an HTTP health endpoint to respond with 200 OK
func waitForHTTPHealth(healthURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for health endpoint: %s", healthURL)
}

