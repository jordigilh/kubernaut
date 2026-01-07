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
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateDataStorageCluster creates a Kind cluster for Data Storage E2E tests
// This includes:
// - Kind cluster (2 nodes: control-plane + worker)
// - Data Storage Service Docker image (build + load)
func CreateDataStorageCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Data Storage E2E Cluster Setup (ONCE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	_, _ = fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Build Data Storage Docker image
	_, _ = fmt.Fprintln(writer, "🔨 Building Data Storage Docker image...")
	if err := buildDataStorageImage(writer); err != nil {
		return fmt.Errorf("failed to build Data Storage image: %w", err)
	}

	// 3. Load Data Storage image into Kind
	_, _ = fmt.Fprintln(writer, "📦 Loading Data Storage image into Kind cluster...")
	if err := loadDataStorageImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Data Storage image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy services per-namespace")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeleteCluster deletes a Kind cluster
func DeleteCluster(clusterName string, writer io.Writer) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to delete cluster: %s\n", output)
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	return nil
}

// SetupDataStorageInfrastructureParallel creates the full E2E infrastructure with parallel execution.
// This optimizes setup time by running independent tasks concurrently.
//
// Parallel Execution Strategy:
//
//	Phase 1 (Sequential): Create Kind cluster + namespace (~65s)
//	Phase 2 (PARALLEL):   Build/Load DS image | Deploy PostgreSQL | Deploy Redis (~60s)
//	Phase 3 (Sequential): Run migrations (~30s)
//	Phase 4 (Sequential): Deploy DataStorage service (~30s)
//	Phase 5 (Sequential): Wait for services ready (~30s)
//
// Total time: ~3.6 minutes (vs ~4.7 minutes sequential)
// Savings: ~1 minute per E2E run (~23% faster)
//
// Note: ImmuDB removed Jan 6, 2026 - PostgreSQL-only architecture per integration test decision
//
// Based on SignalProcessing reference implementation (test/infrastructure/signalprocessing.go:246)
func SetupDataStorageInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath, namespace, dataStorageImage string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 DataStorage E2E Infrastructure (PARALLEL MODE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Parallel optimization: ~1 min saved per E2E run (23% faster)")
	_, _ = fmt.Fprintln(writer, "  Reference: SignalProcessing implementation")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Create Kind cluster + namespace (Sequential - must be first)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Creating Kind cluster + namespace...")

	// Create Kind cluster
	if err := createKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Create namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Parallel infrastructure setup
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⚡ PHASE 2: Parallel infrastructure setup...")
	_, _ = fmt.Fprintln(writer, "  ├── Building + Loading DataStorage image")
	_, _ = fmt.Fprintln(writer, "  ├── Deploying PostgreSQL")
	_, _ = fmt.Fprintln(writer, "  └── Deploying Redis")

	type result struct {
		name      string
		err       error
		imageName string // For DS image: actual built image name with tag
	}

	results := make(chan result, 3) // PostgreSQL-only architecture (ImmuDB removed Jan 6, 2026)

	// Goroutine 1: Build and load DataStorage image (with dynamic tag from caller)
	// REFACTORED: Now uses consolidated BuildAndLoadImageToKind() (Phase 3)
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_PHASE3_PLAN_JAN07.md
	// BUG FIX: Capture returned image name to ensure deployment uses correct tag
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			KindClusterName:  clusterName,
			BuildContextPath: "", // Empty = use project root (default)
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		actualImageName, err := BuildAndLoadImageToKind(cfg, writer)
		if err != nil {
			err = fmt.Errorf("DS image build+load failed: %w", err)
		}
		results <- result{name: "DS image", err: err, imageName: actualImageName}
	}()

	// Goroutine 2: Deploy PostgreSQL
	go func() {
		err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
		if err != nil {
			err = fmt.Errorf("PostgreSQL deploy failed: %w", err)
		}
		results <- result{name: "PostgreSQL", err: err, imageName: ""}
	}()

	// Goroutine 3: Deploy Redis
	go func() {
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		if err != nil {
			err = fmt.Errorf("Redis deploy failed: %w", err)
		}
		results <- result{name: "Redis", err: err, imageName: ""}
	}()

	// Wait for all parallel tasks to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for parallel tasks to complete...")
	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("parallel setup failed (%s): %w", r.name, r.err)
		}
		// BUG FIX: Capture actual image name from DS image build
		if r.name == "DS image" && r.imageName != "" {
			dataStorageImage = r.imageName
			_, _ = fmt.Fprintf(writer, "  ✅ %s complete (image: %s)\n", r.name, r.imageName)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s complete\n", r.name)
		}
	}

	_, _ = fmt.Fprintln(writer, "✅ Phase 2 complete - all parallel tasks succeeded (PostgreSQL-only architecture)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3/4: Deploy migrations + DataStorage service in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3/4: Deploying migrations + DataStorage service in parallel...")
	_, _ = fmt.Fprintln(writer, "  (Kubernetes will handle dependencies - DataStorage retries until migrations complete)")

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 2)

	// Launch migrations and DataStorage deployment concurrently
	go func() {
		err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	go func() {
		err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImage, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 2; i++ {
		result := <-deployResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more deployments failed: %v", deployErrors)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Single wait for DataStorage to be ready (migrations complete first, then DataStorage connects)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for DataStorage to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "✅ DataStorage E2E infrastructure ready in namespace %s\n", namespace)
	_, _ = fmt.Fprintln(writer, "   Setup time optimized: ~23%% faster than sequential")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// DeployDataStorageTestServices deploys PostgreSQL, Redis, and Data Storage Service to a namespace
// This is used by E2E tests to create isolated test environments
// dataStorageImage: DD-TEST-001 compliant image tag (e.g., "datastorage:holmesgpt-api-a1b2c3d4")
//
// Note: ImmuDB removed Jan 6, 2026 - PostgreSQL-only architecture per integration test decision
func DeployDataStorageTestServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Data Storage Test Services in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  📦 Data Storage image: %s\n", dataStorageImage)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy PostgreSQL (V1.0: standard postgres, no pgvector)
	_, _ = fmt.Fprintf(writer, "🚀 Deploying PostgreSQL...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 3. Deploy Redis for DLQ
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Redis for DLQ...\n")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 4. Apply database migrations using shared migration library
	_, _ = fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Deploy Data Storage Service
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Data Storage Service...\n")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 6. Wait for all services ready
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Data Storage test services ready in namespace %s (PostgreSQL + Redis + DataStorage)\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// DeployDataStorageTestServicesWithNodePort deploys DataStorage with custom NodePort
// This variant allows E2E tests to specify NodePort to match Kind cluster port mappings
//
// Parameters:
//   - nodePort: NodePort to use for DataStorage service (e.g., 30081, 30090)
//
// Usage:
//
//	// Notification E2E: Uses NodePort 30090 (per kind-notification-config.yaml)
//	DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, image, 30090, writer)
//
//	// Gateway E2E: Uses NodePort 30081 (per kind-gateway-config.yaml)
//	DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, image, 30081, writer)
func DeployDataStorageTestServicesWithNodePort(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, nodePort int32, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Data Storage Test Services in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  📦 Data Storage image: %s\n", dataStorageImage)
	_, _ = fmt.Fprintf(writer, "  🔌 NodePort: %d\n", nodePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy PostgreSQL (V1.0: standard postgres, no pgvector)
	_, _ = fmt.Fprintf(writer, "🚀 Deploying PostgreSQL...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 3. Deploy Redis for DLQ
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Redis for DLQ...\n")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 4. Apply database migrations using shared migration library
	_, _ = fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Deploy Data Storage Service with custom NodePort
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Data Storage Service (NodePort %d)...\n", nodePort)
	if err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, nodePort, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 6. Wait for all services ready
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Data Storage test services ready in namespace %s (NodePort %d, PostgreSQL + Redis + DataStorage)\n", namespace, nodePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// CleanupDataStorageTestNamespace deletes a test namespace and all resources
func CleanupDataStorageTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🧹 Cleaning up namespace %s...\n", namespace)

	cmd := exec.Command("kubectl", "delete", "namespace", namespace,
		"--kubeconfig", kubeconfigPath,
		"--wait=true",
		"--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Failed to delete namespace: %s\n", output)
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Namespace %s deleted\n", namespace)
	return nil
}

func createTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"test": "datastorage-e2e",
			},
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		// Check for AlreadyExists error (case-insensitive for robustness)
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "alreadyexists") {
			_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s already exists (reusing)\n", namespace)
			return nil
		}
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s created\n", namespace)
	return nil
}

func getKubernetesClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

func deployPostgreSQLInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for init script
	initConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-init",
			Namespace: namespace,
		},
		Data: map[string]string{
			"init.sql": `-- V1.0: Standard PostgreSQL (no pgvector extension)

-- Create user if not exists (idempotent, race-proof)
-- Fix: PostgreSQL docker entrypoint may not create user before running init scripts
-- This handles Kubernetes secret loading delays and container startup timing
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'slm_user') THEN
        CREATE ROLE slm_user WITH LOGIN PASSWORD 'test_password';
    END IF;
END
$$;

-- Grant permissions to slm_user
GRANT ALL PRIVILEGES ON DATABASE action_history TO slm_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;`,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, initConfigMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL init ConfigMap: %w", err)
	}

	// 2. Create Secret for credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"POSTGRES_USER":     "slm_user",
			"POSTGRES_PASSWORD": "test_password",
			"POSTGRES_DB":       "action_history",
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL secret: %w", err)
	}

	// 3. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgresql",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					NodePort:   30432, // Mapped to localhost:5432 via Kind extraPortMappings
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "postgresql",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL service: %w", err)
	}

	// 4. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "postgresql",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "postgresql",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: "postgres:16-alpine", // V1.0: standard postgres, no pgvector
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgresql",
									ContainerPort: 5432,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "POSTGRES_USER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_USER",
										},
									},
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_PASSWORD",
										},
									},
								},
								{
									Name: "POSTGRES_DB",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_DB",
										},
									},
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgresql-data",
									MountPath: "/var/lib/postgresql/data",
								},
								{
									Name:      "postgresql-init",
									MountPath: "/docker-entrypoint-initdb.d",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "postgresql-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "postgresql-init",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "postgresql-init",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

func deployRedisInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "redis",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis service: %w", err)
	}

	// 2. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "redis",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "quay.io/jordigilh/redis:7-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: 6379,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Redis deployed (Service + Deployment)\n")
	return nil
}

// ApplyMigrations is an exported wrapper for applying ALL migrations to a namespace.
// This is useful for re-applying migrations after PostgreSQL restarts (e.g., in DLQ tests).
//
// DEPRECATED: Use ApplyAllMigrations() for DS full schema, or ApplyAuditMigrations() for audit-only.
// This function is kept for backward compatibility.
func ApplyMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Delegate to shared migration library
	return ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
}

func deployDataStorageServiceInNamespace(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	return deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30081, writer)
}

func deployDataStorageServiceInNamespaceWithNodePort(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, nodePort int32, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for service configuration
	configYAML := fmt.Sprintf(`service:
  name: data-storage
  metricsPort: 9181
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: postgresql.%s.svc.cluster.local
  port: 5432
  name: action_history
  user: slm_user
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: redis.%s.svc.cluster.local:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json`, namespace, namespace)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": configYAML,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage ConfigMap: %w", err)
	}

	// 2. Create Secret for database and Redis credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"db-secrets.yaml": `username: slm_user
password: test_password`,
			"redis-secrets.yaml": `password: ""`,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Secret: %w", err)
	}

	// 3. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					NodePort:   nodePort, // Configurable per service (default: 30081)
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9181,
					TargetPort: intstr.FromInt(9181),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "datastorage",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Service: %w", err)
	}

	// 4. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datastorage",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "datastorage",
					},
				},
				Spec: corev1.PodSpec{
					// DD-TEST-001: Schedule on control-plane where images are loaded
					// Kind loads images to control-plane only by default
					NodeSelector: map[string]string{
						"node-role.kubernetes.io/control-plane": "",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node-role.kubernetes.io/control-plane",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					// DD-TEST-007: Run as root for E2E coverage (simplified permissions)
					// Per SP team guidance: non-root user may not have permission to write /coverdata
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
							Name:            "datastorage",
							Image:           dataStorageImage, // DD-TEST-001: service-specific tag
							ImagePullPolicy: corev1.PullNever, // DD-TEST-001: Use local Kind image (scheduled on control-plane where images are loaded)
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
								},
								{
									Name:          "metrics",
									ContainerPort: 9181,
								},
							},
							Env: func() []corev1.EnvVar {
								envVars := []corev1.EnvVar{
									{
										Name:  "CONFIG_PATH",
										Value: "/etc/datastorage/config.yaml",
									},
								}
								// DD-TEST-007: E2E Coverage Capture Standard
								// Only add GOCOVERDIR if E2E_COVERAGE=true
								// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
								coverageEnabled := os.Getenv("E2E_COVERAGE") == "true"
								_, _ = fmt.Fprintf(writer, "   🔍 DD-TEST-007: E2E_COVERAGE=%s (enabled=%v)\n", os.Getenv("E2E_COVERAGE"), coverageEnabled)
								if coverageEnabled {
									_, _ = fmt.Fprintf(writer, "   ✅ Adding GOCOVERDIR=/coverdata to DataStorage deployment\n")
									envVars = append(envVars, corev1.EnvVar{
										Name:  "GOCOVERDIR",
										Value: "/coverdata",
									})
								} else {
									_, _ = fmt.Fprintf(writer, "   ⚠️  E2E_COVERAGE not set, skipping GOCOVERDIR\n")
								}
								return envVars
							}(),
							VolumeMounts: func() []corev1.VolumeMount {
								mounts := []corev1.VolumeMount{
									{
										Name:      "config",
										MountPath: "/etc/datastorage",
										ReadOnly:  true,
									},
									{
										Name:      "secrets",
										MountPath: "/etc/datastorage/secrets",
										ReadOnly:  true,
									},
								}
								// DD-TEST-007: Add coverage volume mount if enabled
								// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
								if os.Getenv("E2E_COVERAGE") == "true" {
									mounts = append(mounts, corev1.VolumeMount{
										Name:      "coverage",
										MountPath: "/coverdata",
										ReadOnly:  false,
									})
								}
								return mounts
							}(),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "datastorage-config",
										},
									},
								},
							},
							{
								Name: "secrets",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: "datastorage-secret",
									},
								},
							},
						}
						// DD-TEST-007: Add hostPath volume for coverage if enabled
						// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
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

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage Service deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

func waitForDataStorageServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for PostgreSQL pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=postgresql",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "PostgreSQL pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL pod ready\n")

	// Wait for Redis pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=redis",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Redis pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Redis pod ready\n")

	// Wait for Data Storage Service pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Data Storage Service pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Data Storage Service pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage Service pod ready\n")

	return nil
}

func createKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
	opts := KindClusterOptions{
		ClusterName:    clusterName,
		KubeconfigPath: kubeconfigPath,
		ConfigPath:     "test/infrastructure/kind-datastorage-config.yaml",
		WaitTimeout:    "60s",
		DeleteExisting: true, // Original behavior: delete if exists
		ReuseExisting:  false,
	}
	return CreateKindClusterWithConfig(opts, writer)
}

func buildDataStorageImage(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Build Data Storage image using Podman (following Gateway pattern)
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildArgs := []string{
		"build",
		"--no-cache",                                                 // Force fresh build to include latest code changes
		"-t", "localhost/kubernaut-datastorage:e2e-test-datastorage", // DD-TEST-001: service-specific tag
		"-f", "docker/data-storage.Dockerfile",
	}

	// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
	// If E2E_COVERAGE=true, build with coverage instrumentation
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintln(writer, "   📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	}

	buildArgs = append(buildArgs, ".")

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	// Tag image for SP E2E compatibility (SP expects e2e-test tag)
	tagCmd := exec.Command("podman", "tag", "localhost/kubernaut-datastorage:e2e-test-datastorage", "localhost/kubernaut-datastorage:e2e-test")
	tagCmd.Stdout = writer
	tagCmd.Stderr = writer
	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("podman tag failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   Data Storage image built: localhost/kubernaut-datastorage:e2e-test-datastorage")
	_, _ = fmt.Fprintln(writer, "   Data Storage image tagged: localhost/kubernaut-datastorage:e2e-test (SP E2E compatibility)")

	// PROFILING: Get image size for optimization analysis
	sizeCmd := exec.Command("podman", "images", "--format", "{{.Size}}", "localhost/kubernaut-datastorage:e2e-test-datastorage")
	sizeOutput, err := sizeCmd.Output()
	if err == nil {
		_, _ = fmt.Fprintf(writer, "   📊 Image size: %s\n", string(sizeOutput))
	}

	return nil
}

func loadDataStorageImage(clusterName string, writer io.Writer) error {
	// Save image to tar (following Gateway pattern)
	// DD-TEST-001: Use service-specific tag
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-datastorage:e2e-test-datastorage", "-o", "/tmp/datastorage-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/datastorage-e2e.tar")

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintln(writer, "   🗑️  Removing Podman image to free disk space...")
	rmiCmd := exec.Command("podman", "rmi", "-f", "localhost/kubernaut-datastorage:e2e-test-datastorage")
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintln(writer, "   ✅ Podman image removed: localhost/kubernaut-datastorage:e2e-test-datastorage")
	}

	_, _ = fmt.Fprintln(writer, "   Data Storage image loaded into Kind cluster")
	return nil
}

// DataStorageInfrastructure manages the Data Storage Service test infrastructure
// This includes PostgreSQL, Redis, and the Data Storage Service itself
type DataStorageInfrastructure struct {
	PostgresContainer string
	RedisContainer    string
	ServiceContainer  string
	ConfigDir         string
	ServiceURL        string
	DB                *sql.DB
	RedisClient       *redis.Client
}

// DataStorageConfig contains configuration for the Data Storage Service
type DataStorageConfig struct {
	PostgresPort string // Default: "5433"
	RedisPort    string // Default: "6380"
	ServicePort  string // Default: "8085"
	DBName       string // Default: "action_history"
	DBUser       string // Default: "slm_user"
	DBPassword   string // Default: "test_password"
}

// DefaultDataStorageConfig returns default configuration
func DefaultDataStorageConfig() *DataStorageConfig {
	return &DataStorageConfig{
		PostgresPort: "5433",
		RedisPort:    "6380",
		ServicePort:  "8085",
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password",
	}
}

// StartDataStorageInfrastructure starts all Data Storage Service infrastructure
// Returns an infrastructure handle that can be used to stop the services
func StartDataStorageInfrastructure(cfg *DataStorageConfig, writer io.Writer) (*DataStorageInfrastructure, error) {
	if cfg == nil {
		cfg = DefaultDataStorageConfig()
	}

	infra := &DataStorageInfrastructure{
		PostgresContainer: "datastorage-postgres-test",
		RedisContainer:    "datastorage-redis-test",
		ServiceContainer:  "datastorage-service-test",
		ServiceURL:        fmt.Sprintf("http://localhost:%s", cfg.ServicePort),
	}

	_, _ = fmt.Fprintln(writer, "🔧 Setting up Data Storage Service infrastructure (ADR-016: Podman)")

	// 1. Start PostgreSQL
	_, _ = fmt.Fprintln(writer, "📦 Starting PostgreSQL container...")
	if err := startPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// 2. Start Redis
	_, _ = fmt.Fprintln(writer, "📦 Starting Redis container...")
	if err := startRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	// 3. Connect to PostgreSQL
	_, _ = fmt.Fprintln(writer, "🔌 Connecting to PostgreSQL...")
	if err := connectPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// 4. Apply migrations
	_, _ = fmt.Fprintln(writer, "📋 Applying schema migrations...")
	if err := applyMigrations(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Connect to Redis
	_, _ = fmt.Fprintln(writer, "🔌 Connecting to Redis...")
	if err := connectRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// 6. Create config files
	_, _ = fmt.Fprintln(writer, "📝 Creating ADR-030 config files...")
	if err := createConfigFiles(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to create config files: %w", err)
	}

	// 7. Build Data Storage Service image
	_, _ = fmt.Fprintln(writer, "🏗️  Building Data Storage Service image...")
	if err := buildDataStorageService(writer); err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	// 8. Start Data Storage Service
	_, _ = fmt.Fprintln(writer, "🚀 Starting Data Storage Service container...")
	if err := startDataStorageService(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	// 9. Wait for service to be ready
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for Data Storage Service to be ready...")
	if err := waitForServiceReady(infra, writer); err != nil {
		return nil, fmt.Errorf("service not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Data Storage Service infrastructure ready!")
	return infra, nil
}

// StopDataStorageInfrastructure stops all Data Storage Service infrastructure
func (infra *DataStorageInfrastructure) Stop(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "🧹 Cleaning up Data Storage Service infrastructure...")

	// Close connections
	if infra.DB != nil {
		_ = infra.DB.Close()
	}
	if infra.RedisClient != nil {
		_ = infra.RedisClient.Close()
	}

	// Stop and remove containers
	_ = exec.Command("podman", "stop", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "rm", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "stop", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "rm", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "stop", infra.RedisContainer).Run()
	_ = exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Remove config directory
	if infra.ConfigDir != "" {
		_ = os.RemoveAll(infra.ConfigDir)
	}

	_, _ = fmt.Fprintln(writer, "✅ Data Storage Service infrastructure cleanup complete")
}

// Helper functions

func startPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "rm", infra.PostgresContainer).Run()

	// Start PostgreSQL (V1.0: standard postgres, no pgvector)
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"-p", fmt.Sprintf("%s:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", cfg.DBName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", cfg.DBUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.DBPassword),
		"postgres:16-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to start PostgreSQL: %s\n", output)
		return fmt.Errorf("PostgreSQL container failed to start: %w", err)
	}

	// Wait for PostgreSQL ready
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL to be ready...")
	time.Sleep(3 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.PostgresContainer, "pg_isready", "-U", cfg.DBUser)
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	_, _ = fmt.Fprintln(writer, "  ✅ PostgreSQL started successfully")
	return nil
}

func startRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.RedisContainer).Run()
	_ = exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Start Redis
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.RedisContainer,
		"-p", fmt.Sprintf("%s:6379", cfg.RedisPort),
		"quay.io/jordigilh/redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to start Redis: %s\n", output)
		return fmt.Errorf("Redis container failed to start: %w", err)
	}

	// Wait for Redis ready
	time.Sleep(2 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.RedisContainer, "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	_, _ = fmt.Fprintln(writer, "  ✅ Redis started successfully")
	return nil
}

func connectPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	connStr := fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.PostgresPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	var err error
	infra.DB, err = sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Wait for connection
	Eventually(func() error {
		return infra.DB.Ping()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be connectable")

	_, _ = fmt.Fprintln(writer, "  ✅ PostgreSQL connection established")
	return nil
}

func applyMigrations(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Drop and recreate schema
	_, _ = fmt.Fprintln(writer, "  🗑️  Dropping existing schema...")
	_, err := infra.DB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	// V1.0: pgvector extension REMOVED (label-only architecture)
	// See: docs/handoff/RESPONSE_DS_PGVECTOR_CLEANUP_COMPLETE.md

	// Apply migrations (V1.0 label-only, no vector migrations)
	_, _ = fmt.Fprintln(writer, "  📜 Applying V1.0 migrations (label-only, no embeddings)...")
	// V1.0 Migration List (label-only architecture, no embeddings)
	// Removed vector-dependent migrations per TRIAGE_DS_MIGRATION_DEPENDENCIES_V1.0.md:
	// - 005_vector_schema.sql (creates action_patterns with embedding vector)
	// - 007_add_context_column.sql (depends on 005)
	// - 008_context_api_compatibility.sql (adds embedding column)
	// - 009_update_vector_dimensions.sql (updates vector dimensions)
	// - 010_audit_write_api_phase1.sql (creates tables with vector columns)
	// - 011_rename_alert_to_signal.sql (depends on 010)
	// - 015_create_workflow_catalog_table.sql (creates workflows with embedding)
	// - 016_update_embedding_dimensions.sql (updates to 768 dimensions)
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		// NOTE: Migration 006 moved to migrations/v1.1/ (v1.1 feature, removed 2026-01-07)
		"012_adr033_multidimensional_tracking.sql",
		"013_create_audit_events_table.sql",
		"017_add_workflow_schema_fields.sql",
		"018_rename_execution_bundle_to_container_image.sql",
		"019_uuid_primary_key.sql",
		"020_add_workflow_label_columns.sql", // DD-WORKFLOW-001 v1.6: custom_labels + detected_labels
		"1000_create_audit_events_partitions.sql",
	}

	// Find workspace root once (project root with go.mod)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	for _, migration := range migrations {
		// Use absolute path from project root (no relative path issues)
		migrationPath := filepath.Join(workspaceRoot, "migrations", migration)

		content, err := os.ReadFile(migrationPath)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ Migration file not found at %s: %v\n", migrationPath, err)
			return fmt.Errorf("migration file %s not found at %s: %w", migration, migrationPath, err)
		}

		// Remove CONCURRENTLY keyword for test environment
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		// Extract only the UP migration (ignore DOWN section)
		if strings.Contains(migrationSQL, "-- +goose Down") {
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		_, err = infra.DB.Exec(migrationSQL)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ Migration %s failed: %v\n", migration, err)
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ Applied %s\n", migration)
	}

	// Grant permissions
	_, _ = fmt.Fprintln(writer, "  🔐 Granting permissions...")
	_, err = infra.DB.Exec(`
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`)
	if err != nil {
		return fmt.Errorf("failed to grant permissions: %w", err)
	}

	// Wait for schema propagation
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL schema propagation (2s)...")
	time.Sleep(2 * time.Second)

	_, _ = fmt.Fprintln(writer, "  ✅ All migrations applied successfully")
	return nil
}

func connectRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	infra.RedisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("localhost:%s", cfg.RedisPort),
		DB:   0,
	})

	// Verify connection
	err := infra.RedisClient.Ping(context.Background()).Err()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Redis connection established")
	return nil
}

func createConfigFiles(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	var err error
	infra.ConfigDir, err = os.MkdirTemp("", "datastorage-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Get container IPs
	postgresIP := getContainerIP(infra.PostgresContainer)
	redisIP := getContainerIP(infra.RedisContainer)

	// Create config.yaml (ADR-030)
	configYAML := fmt.Sprintf(`
service:
  name: data-storage
  metricsPort: 9181
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: %s
  port: 5432
  name: %s
  user: %s
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: %s:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`, postgresIP, cfg.DBName, cfg.DBUser, redisIP)

	configPath := filepath.Join(infra.ConfigDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// Create database secrets file
	dbSecretsYAML := fmt.Sprintf(`
username: %s
password: %s
`, cfg.DBUser, cfg.DBPassword)
	dbSecretsPath := filepath.Join(infra.ConfigDir, "db-secrets.yaml")
	err = os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write db-secrets.yaml: %w", err)
	}

	// Create Redis secrets file
	redisSecretsYAML := `password: ""` // Redis without auth in test
	redisSecretsPath := filepath.Join(infra.ConfigDir, "redis-secrets.yaml")
	err = os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write redis-secrets.yaml: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Config files created in %s\n", infra.ConfigDir)
	return nil
}

func buildDataStorageService(writer io.Writer) error {
	// Find workspace root (go.mod location)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Cleanup any existing image
	_ = exec.Command("podman", "rmi", "-f", "data-storage:test").Run()

	// Build image for ARM64 (local testing on Apple Silicon)
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildCmd := exec.Command("podman", "build",
		"--no-cache", // Force fresh build to include latest code changes
		"--build-arg", "GOARCH=arm64",
		"-t", "data-storage:test",
		"-f", "docker/data-storage.Dockerfile",
		".")
	buildCmd.Dir = workspaceRoot // Run from workspace root

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Build output:\n%s\n", string(output))
		return fmt.Errorf("failed to build Data Storage Service image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Data Storage Service image built successfully")
	return nil
}

// findWorkspaceRoot finds the workspace root by looking for go.mod
func findWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func startDataStorageService(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "rm", infra.ServiceContainer).Run()

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", infra.ConfigDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", infra.ConfigDir)

	// Start service container with ADR-030 config
	startCmd := exec.Command("podman", "run", "-d",
		"--name", infra.ServiceContainer,
		"-p", fmt.Sprintf("%s:8080", cfg.ServicePort),
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:test")

	output, err := startCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Start output:\n%s\n", string(output))
		return fmt.Errorf("failed to start Data Storage Service container: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Data Storage Service container started")
	return nil
}

func waitForServiceReady(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Wait up to 30 seconds for service to be ready
	var lastStatusCode int
	var lastError error

	Eventually(func() int {
		resp, err := http.Get(infra.ServiceURL + "/health")
		if err != nil {
			lastError = err
			lastStatusCode = 0
			_, _ = fmt.Fprintf(writer, "    Health check attempt failed: %v\n", err)
			return 0
		}
		if resp == nil {
			lastStatusCode = 0
			return 0
		}
		defer func() { _ = resp.Body.Close() }()
		lastStatusCode = resp.StatusCode
		if lastStatusCode != 200 {
			_, _ = fmt.Fprintf(writer, "    Health check returned status %d (expected 200)\n", lastStatusCode)
		}
		return lastStatusCode
	}, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

	// If we got here and status is not 200, print diagnostics
	if lastStatusCode != 200 {
		_, _ = fmt.Fprintf(writer, "\n❌ Data Storage Service health check failed\n")
		_, _ = fmt.Fprintf(writer, "  Last status code: %d\n", lastStatusCode)
		if lastError != nil {
			_, _ = fmt.Fprintf(writer, "  Last error: %v\n", lastError)
		}

		// Print container logs for debugging
		logs, logErr := exec.Command("podman", "logs", "--tail", "200", infra.ServiceContainer).CombinedOutput()
		if logErr == nil {
			_, _ = fmt.Fprintf(writer, "\n📋 Data Storage Service logs (last 200 lines):\n%s\n", string(logs))
		}

		// Check if container is running
		statusCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", infra.ServiceContainer), "--format", "{{.Status}}")
		statusOutput, _ := statusCmd.CombinedOutput()
		_, _ = fmt.Fprintf(writer, "  Container status: %s\n", strings.TrimSpace(string(statusOutput)))
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Data Storage Service ready at %s\n", infra.ServiceURL)
	return nil
}

func getContainerIP(containerName string) string {
	cmd := exec.Command("podman", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to get IP for container %s: %v", containerName, err))
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		panic(fmt.Sprintf("Container %s has no IP address", containerName))
	}
	return ip
}

// ═══════════════════════════════════════════════════════════════════════
// SHARED E2E HELPER FUNCTIONS (Per DD-TEST-001: Fresh builds with dynamic tags)
// ═══════════════════════════════════════════════════════════════════════

// buildDataStorageImageWithTag builds DataStorage image with a specific dynamic tag
// This ensures each service gets a FRESH build with latest DataStorage code
// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
func buildDataStorageImageWithTag(imageTag string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  🔨 Building DataStorage with tag: %s\n", imageTag)

	// Build Data Storage image using Podman
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildArgs := []string{
		"build",
		"--no-cache",   // Force fresh build to include latest code changes
		"-t", imageTag, // Use dynamic tag for parallel isolation
		"-f", "docker/data-storage.Dockerfile",
	}

	// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
	// If E2E_COVERAGE=true, build with coverage instrumentation
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintln(writer, "     📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	}

	buildArgs = append(buildArgs, ".")

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "     ✅ DataStorage image built: %s\n", imageTag)
	return nil
}

// loadDataStorageImageWithTag loads DataStorage image into Kind cluster with specific tag
// Per DD-TEST-001: Each service loads its own fresh-built DataStorage image
func loadDataStorageImageWithTag(clusterName, imageTag string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  📦 Loading DataStorage image: %s\n", imageTag)

	// Save image to tar
	saveCmd := exec.Command("podman", "save", imageTag, "-o", "/tmp/datastorage-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = exec.Command("rm", "-f", "/tmp/datastorage-e2e.tar").Run()

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintf(writer, "     🗑️  Removing Podman image to free disk space...\n")
	rmiCmd := exec.Command("podman", "rmi", "-f", imageTag)
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "     ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "     ✅ Podman image removed: %s\n", imageTag)
	}

	_, _ = fmt.Fprintf(writer, "     ✅ DataStorage image loaded: %s\n", imageTag)
	return nil
}
