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

