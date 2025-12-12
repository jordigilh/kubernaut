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
	"os/exec"
	"strings"
)

// ========================================
// GATEWAY E2E INFRASTRUCTURE
// ========================================
//
// Gateway E2E tests require:
// - Kind cluster (RemediationRequest CRD)
// - PostgreSQL (Data Storage dependency)
// - Redis (Data Storage dependency)
// - Data Storage service (audit events)
// - Gateway service (signal ingestion)
//
// Pattern: Follows AIAnalysis E2E infrastructure pattern
// Authority: test/infrastructure/aianalysis.go
// ========================================

// Gateway E2E service ports (DD-TEST-001 port allocation strategy)
const (
	GatewayE2EHostPort     = 8080  // Gateway API (NodePort mapping)
	GatewayE2EMetricsPort  = 9080  // Gateway metrics
	GatewayDataStoragePort = 8091  // Data Storage for audit events
)

// CreateGatewayCluster creates a Kind cluster for Gateway E2E tests
// This includes:
// 1. Kind cluster (2 nodes: control-plane + worker)
// 2. RemediationRequest CRD installation
// 3. Gateway Docker image build and load
func CreateGatewayCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Gateway E2E Cluster Setup")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Dependencies:")
	fmt.Fprintf(writer, "  • PostgreSQL (port 5433) - Data Storage persistence\n")
	fmt.Fprintf(writer, "  • Redis (port 6380) - Data Storage caching\n")
	fmt.Fprintf(writer, "  • Data Storage (port %d) - Audit trail\n", GatewayDataStoragePort)
	fmt.Fprintf(writer, "  • Gateway (port %d) - Signal ingestion\n", GatewayE2EHostPort)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createGatewayKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install RemediationRequest CRD (reuse from signalprocessing.go)
	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	crdPath := getProjectRoot() + "/config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml"
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// 3. Build and load Gateway Docker image
	fmt.Fprintln(writer, "🐳 Building Gateway Docker image...")
	if err := buildAndLoadGatewayImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to build Gateway image: %w", err)
	}

	fmt.Fprintln(writer, "✅ Gateway E2E cluster created successfully")
	return nil
}

// DeployTestServices deploys Gateway and its dependencies to the Kind cluster
// This includes:
// 1. PostgreSQL deployment
// 2. Redis deployment
// 3. Data Storage deployment
// 4. Gateway deployment
func DeployTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "📦 Deploying Gateway E2E services...")

	// 1. Deploy PostgreSQL (shared function from datastorage.go)
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 2. Deploy Redis (shared function from datastorage.go)
	fmt.Fprintln(writer, "🔴 Deploying Redis...")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 3. Wait for infrastructure to be ready
	fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL and Redis to be ready...")
	if err := waitForDataStorageInfraReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("infrastructure not ready: %w", err)
	}

	// 4. Deploy Data Storage service
	fmt.Fprintln(writer, "💾 Deploying Data Storage service...")
	if err := deployDataStorageToCluster(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	// 5. Deploy Gateway service
	fmt.Fprintln(writer, "🚪 Deploying Gateway service...")
	if err := deployGatewayService(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Gateway: %w", err)
	}

	fmt.Fprintln(writer, "✅ All services deployed successfully")
	return nil
}

// DeleteGatewayCluster deletes the Kind cluster
func DeleteGatewayCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🗑️  Deleting Gateway E2E cluster...")

	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	fmt.Fprintln(writer, "✅ Gateway E2E cluster deleted")
	return nil
}

// ========================================
// INTERNAL HELPERS
// ========================================

// createGatewayKindCluster creates a Kind cluster for Gateway E2E tests
func createGatewayKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Use test/e2e/kind-config.yaml (shared Kind configuration)
	kindConfigPath := getProjectRoot() + "/test/e2e/kind-config.yaml"

	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", kindConfigPath,
		"--kubeconfig", kubeconfigPath,
		"--wait", "5m",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	return nil
}

// buildAndLoadGatewayImage builds Gateway Docker image using Podman and loads it into Kind
func buildAndLoadGatewayImage(clusterName string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	// 1. Build Docker image using Podman
	fmt.Fprintln(writer, "   Building Docker image using Podman...")
	buildCmd := exec.Command("podman", "build",
		"-t", "gateway:e2e-test",
		"-f", projectRoot+"/Dockerfile.gateway",
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	// 2. Load image into Kind (Kind supports Podman with experimental provider)
	fmt.Fprintln(writer, "   Loading image into Kind cluster...")
	loadCmd := exec.Command("kind", "load", "docker-image",
		"gateway:e2e-test",
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("kind load image failed: %w", err)
	}

	return nil
}

// deployDataStorageToCluster deploys Data Storage service to the cluster
func deployDataStorageToCluster(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Deploy using Data Storage's shared deployment function
	// This is a simplified version - full deployment would include ConfigMap, Secrets, etc.
	// For now, Gateway E2E tests will use a basic deployment

	// Use the existing deployDataStorage function from aianalysis.go pattern
	// But for Gateway, we don't need all the complexity
	// Gateway only needs Data Storage to be available for audit events

	// Create Data Storage deployment YAML
	deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: %s
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
        image: datastorage:e2e-test
        ports:
        - containerPort: 8080
        env:
        - name: POSTGRES_HOST
          value: postgres
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_USER
          value: testuser
        - name: POSTGRES_PASSWORD
          value: testpass
        - name: POSTGRES_DB
          value: testdb
        - name: REDIS_ADDR
          value: redis:6379
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: %s
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: %d
  selector:
    app: datastorage
`, namespace, namespace, GatewayDataStoragePort)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deploymentYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply Data Storage failed: %w", err)
	}

	// Wait for Data Storage to be ready
	fmt.Fprintln(writer, "   Waiting for Data Storage pod...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=datastorage",
		"-n", namespace,
		"--timeout=120s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Data Storage pod not ready: %w", err)
	}

	return nil
}

// deployGatewayService deploys Gateway service to the cluster
func deployGatewayService(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Deploy Gateway using test/e2e/gateway/gateway-deployment.yaml
	deploymentPath := getProjectRoot() + "/test/e2e/gateway/gateway-deployment.yaml"

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", deploymentPath,
		"-n", namespace)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply Gateway deployment failed: %w", err)
	}

	// Wait for Gateway to be ready
	fmt.Fprintln(writer, "   Waiting for Gateway pod...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=gateway",
		"-n", namespace,
		"--timeout=120s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Gateway pod not ready: %w", err)
	}

	return nil
}

// waitForDataStorageInfraReady waits for PostgreSQL and Redis to be ready
// This is a simplified version for Gateway E2E tests
func waitForDataStorageInfraReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Wait for PostgreSQL
	fmt.Fprintln(writer, "   Waiting for PostgreSQL...")
	pgCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=postgres",
		"-n", namespace,
		"--timeout=120s")
	pgCmd.Stdout = writer
	pgCmd.Stderr = writer
	if err := pgCmd.Run(); err != nil {
		return fmt.Errorf("PostgreSQL not ready: %w", err)
	}

	// Wait for Redis
	fmt.Fprintln(writer, "   Waiting for Redis...")
	redisCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=redis",
		"-n", namespace,
		"--timeout=120s")
	redisCmd.Stdout = writer
	redisCmd.Stderr = writer
	if err := redisCmd.Run(); err != nil {
		return fmt.Errorf("Redis not ready: %w", err)
	}

	return nil
}

