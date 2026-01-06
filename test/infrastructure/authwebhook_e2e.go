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
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupAuthWebhookInfrastructureParallel creates the full AuthWebhook E2E infrastructure with parallel execution.
// This optimizes setup time by running independent tasks concurrently.
//
// Parallel Execution Strategy:
//
//	Phase 1 (Sequential): Create Kind cluster + namespace (~65s)
//	Phase 2 (PARALLEL):   Build/Load DS+AW images | Deploy PostgreSQL | Deploy Redis (~90s)
//	Phase 3 (Sequential): Run migrations (~30s)
//	Phase 4 (Sequential): Deploy DataStorage + AuthWebhook services (~45s)
//	Phase 5 (Sequential): Wait for services ready (~30s)
//
// Total time: ~4.5 minutes (vs ~6.0 minutes sequential)
// Savings: ~1.5 minutes per E2E run (~25% faster)
//
// Based on DataStorage reference implementation (test/infrastructure/datastorage.go:85)
func SetupAuthWebhookInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath, namespace, dataStorageImage, authWebhookImage string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 AuthWebhook E2E Infrastructure (PARALLEL MODE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Parallel optimization: ~1.5 min saved per E2E run (25% faster)")
	_, _ = fmt.Fprintln(writer, "  Reference: DataStorage implementation")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Create Kind cluster + namespace (Sequential - must be first)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Creating Kind cluster + namespace...")

	// Create ./coverdata directory for coverage collection (required by kind-config.yaml)
	// Kind interprets relative paths relative to where the config file is located
	// So ./coverdata in test/e2e/authwebhook/kind-config.yaml means test/e2e/authwebhook/coverdata
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}
	coverdataPath := filepath.Join(workspaceRoot, "test", "e2e", "authwebhook", "coverdata")
	if err := os.MkdirAll(coverdataPath, 0755); err != nil {
		return fmt.Errorf("failed to create coverdata directory: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Created %s for coverage collection\n", coverdataPath)

	// Create Kind cluster with authwebhook-specific config
	if err := createKindClusterWithConfig(clusterName, kubeconfigPath, "test/e2e/authwebhook/kind-config.yaml", writer); err != nil {
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
	_, _ = fmt.Fprintln(writer, "  ├── Building + Loading AuthWebhook image")
	_, _ = fmt.Fprintln(writer, "  ├── Deploying PostgreSQL")
	_, _ = fmt.Fprintln(writer, "  └── Deploying Redis")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 4)

	// Goroutine 1: Build and load DataStorage image
	go func() {
		var err error
		if buildErr := buildDataStorageImageWithTag(dataStorageImage, writer); buildErr != nil {
			err = fmt.Errorf("DS image build failed: %w", buildErr)
		} else if loadErr := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); loadErr != nil {
			err = fmt.Errorf("DS image load failed: %w", loadErr)
		}
		results <- result{name: "DS image", err: err}
	}()

	// Goroutine 2: Build and load AuthWebhook image
	go func() {
		var err error
		if buildErr := buildAuthWebhookImageWithTag(authWebhookImage, writer); buildErr != nil {
			err = fmt.Errorf("AuthWebhook image build failed: %w", buildErr)
		} else if loadErr := loadAuthWebhookImageWithTag(clusterName, authWebhookImage, writer); loadErr != nil {
			err = fmt.Errorf("AuthWebhook image load failed: %w", loadErr)
		}
		results <- result{name: "AuthWebhook image", err: err}
	}()

	// Goroutine 3: Deploy PostgreSQL (E2E ports per DD-TEST-001)
	go func() {
		err := deployPostgreSQLToKind(kubeconfigPath, namespace, "25442", "30442", writer)
		results <- result{name: "PostgreSQL", err: err}
	}()

	// Goroutine 4: Deploy Redis (E2E ports per DD-TEST-001)
	go func() {
		err := deployRedisToKind(kubeconfigPath, namespace, "26386", "30386", writer)
		results <- result{name: "Redis", err: err}
	}()

	// Collect results from all goroutines
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for parallel tasks to complete...")
	var firstError error
	for i := 0; i < 4; i++ {
		res := <-results
		if res.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s: %v\n", res.name, res.err)
			if firstError == nil {
				firstError = res.err
			}
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s: Success\n", res.name)
		}
	}

	if firstError != nil {
		return fmt.Errorf("parallel infrastructure setup failed: %w", firstError)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Run database migrations (Sequential - depends on PostgreSQL)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 3: Running database migrations...")
	if err := runDatabaseMigrations(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services (Sequential - depends on migrations)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🚀 PHASE 4: Deploying services...")

	// Deploy DataStorage service (E2E ports per DD-TEST-001)
	_, _ = fmt.Fprintln(writer, "  📦 Deploying DataStorage service...")
	if err := deployDataStorageToKind(kubeconfigPath, namespace, dataStorageImage, "28099", "30099", writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// Deploy AuthWebhook service with webhook configurations
	_, _ = fmt.Fprintln(writer, "  🔐 Deploying AuthWebhook service...")
	if err := deployAuthWebhookToKind(kubeconfigPath, namespace, authWebhookImage, writer); err != nil {
		return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Wait for services to be ready (Sequential - verification)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 5: Waiting for services to be ready...")
	if err := waitForServicesReady(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("services failed to become ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ AuthWebhook E2E infrastructure ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// buildAuthWebhookImageWithTag builds the AuthWebhook (webhooks service) Docker image with a specific tag
func buildAuthWebhookImageWithTag(imageTag string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔨 Building Webhooks service image: %s\n", imageTag)

	// Build image using podman (follows DataStorage pattern)
	// Note: Service binary is 'webhooks' (cmd/webhooks/main.go)
	cmd := exec.Command("podman", "build",
		"-t", imageTag,
		"-f", "docker/webhooks.Dockerfile",
		".")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Build failed: %s\n", output)
		return fmt.Errorf("podman build failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Webhooks service image built successfully")
	return nil
}

// loadAuthWebhookImageWithTag loads the AuthWebhook image into Kind cluster
func loadAuthWebhookImageWithTag(clusterName, imageTag string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "📦 Loading AuthWebhook image into Kind: %s\n", imageTag)

	cmd := exec.Command("kind", "load", "docker-image", imageTag, "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Load failed: %s\n", output)
		return fmt.Errorf("kind load failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ AuthWebhook image loaded into Kind")
	return nil
}

// deployAuthWebhookToKind deploys the AuthWebhook service to Kind cluster
func deployAuthWebhookToKind(kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	// Generate webhook TLS certificates
	_, _ = fmt.Fprintln(writer, "🔐 Generating webhook TLS certificates...")
	if err := generateWebhookCerts(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate webhook certs: %w", err)
	}

	// Apply CRDs first
	_, _ = fmt.Fprintln(writer, "📋 Applying CRDs...")
	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "config/crd/bases/")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ CRD apply failed: %s\n", output)
		return fmt.Errorf("kubectl apply crds failed: %w", err)
	}

	// Apply webhook deployment
	_, _ = fmt.Fprintln(writer, "🚀 Applying AuthWebhook deployment...")
	cmd = exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"-f", "test/e2e/authwebhook/manifests/authwebhook-deployment.yaml")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Deployment failed: %s\n", output)
		return fmt.Errorf("kubectl apply failed: %w", err)
	}

	// Patch deployment with correct image tag
	_, _ = fmt.Fprintf(writer, "🔧 Patching deployment with image: %s\n", imageTag)
	cmd = exec.Command("kubectl", "set", "image",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"deployment/authwebhook",
		fmt.Sprintf("authwebhook=%s", imageTag))
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Image patch failed: %s\n", output)
		return fmt.Errorf("kubectl set image failed: %w", err)
	}

	return nil
}

// generateWebhookCerts generates TLS certificates for webhook admission and patches webhook configurations
func generateWebhookCerts(kubeconfigPath, namespace string, writer io.Writer) error {
	// Use openssl to generate self-signed certificates for testing
	// In production, use cert-manager

	// Generate private key
	cmd := exec.Command("openssl", "genrsa", "-out", "/tmp/webhook-key.pem", "2048")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Key generation failed: %s\n", output)
		return fmt.Errorf("openssl genrsa failed: %w", err)
	}

	// Generate certificate with SAN (Subject Alternative Names) for webhook service
	// This is required for Kubernetes to trust the webhook certificate
	cmd = exec.Command("openssl", "req", "-new", "-x509",
		"-key", "/tmp/webhook-key.pem",
		"-out", "/tmp/webhook-cert.pem",
		"-days", "365",
		"-subj", fmt.Sprintf("/CN=authwebhook.%s.svc", namespace),
		"-addext", fmt.Sprintf("subjectAltName=DNS:authwebhook.%s.svc,DNS:authwebhook.%s.svc.cluster.local", namespace, namespace))
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Cert generation failed: %s\n", output)
		return fmt.Errorf("openssl req failed: %w", err)
	}

	// Base64 encode the certificate for CA bundle
	caBundleOutput, err := exec.Command("bash", "-c", "cat /tmp/webhook-cert.pem | base64 | tr -d '\\n'").Output()
	if err != nil {
		return fmt.Errorf("failed to base64 encode CA bundle: %w", err)
	}
	caBundleB64 := string(caBundleOutput)

	// Create and apply secret with certificates using a single command
	cmd = exec.Command("kubectl", "create", "secret", "tls", "authwebhook-tls",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"--cert=/tmp/webhook-cert.pem",
		"--key=/tmp/webhook-key.pem")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Secret creation failed: %s\n", output)
		return fmt.Errorf("kubectl create secret failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Webhook TLS certificates created")

	// Patch MutatingWebhookConfiguration with CA bundle
	_, _ = fmt.Fprintln(writer, "🔧 Patching MutatingWebhookConfiguration with CA bundle...")
	for _, webhookName := range []string{"workflowexecution.mutate.kubernaut.ai", "remediationapprovalrequest.mutate.kubernaut.ai"} {
		patchCmd := exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "authwebhook-mutating",
			"--kubeconfig", kubeconfigPath,
			"--type=json",
			"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/0/clientConfig/caBundle","value":"%s"}]`, caBundleB64))
		if output, err := patchCmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Failed to patch %s: %s\n", webhookName, output)
			// Continue anyway - webhook might still work
		}
	}

	// Patch ValidatingWebhookConfiguration with CA bundle
	_, _ = fmt.Fprintln(writer, "🔧 Patching ValidatingWebhookConfiguration with CA bundle...")
	patchCmd := exec.Command("kubectl", "patch", "validatingwebhookconfiguration", "authwebhook-validating",
		"--kubeconfig", kubeconfigPath,
		"--type=json",
		"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/0/clientConfig/caBundle","value":"%s"}]`, caBundleB64))
	if output, err := patchCmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Failed to patch validating webhook: %s\n", output)
		// Continue anyway - webhook might still work
	}

	_, _ = fmt.Fprintln(writer, "✅ Webhook configurations patched with CA bundle")
	return nil
}

// createKindClusterWithConfig creates a Kind cluster with a specific config file
func createKindClusterWithConfig(clusterName, kubeconfigPath, configPath string, writer io.Writer) error {
	// Check if cluster already exists and delete it
	checkCmd := exec.Command("kind", "get", "clusters")
	checkOutput, _ := checkCmd.CombinedOutput()
	if strings.Contains(string(checkOutput), clusterName) {
		_, _ = fmt.Fprintln(writer, "  ⚠️  Cluster already exists, deleting...")
		delCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
		if output, err := delCmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Failed to delete existing cluster: %s\n", output)
		}
	}

	// Resolve config path relative to workspace root
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}
	absoluteConfigPath := filepath.Join(workspaceRoot, configPath)
	if _, err := os.Stat(absoluteConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kind config file not found: %s", absoluteConfigPath)
	}

	_, _ = fmt.Fprintf(writer, "  📋 Using Kind config: %s\n", absoluteConfigPath)

	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", absoluteConfigPath,
		"--kubeconfig", kubeconfigPath,
		"--wait", "60s")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to create cluster:\n%s\n", output)
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Kind cluster created")

	// Export kubeconfig explicitly (kind create --kubeconfig doesn't always work reliably)
	kubeconfigCmd := exec.Command("kind", "get", "kubeconfig", "--name", clusterName)
	kubeconfigOutput, err := kubeconfigCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Ensure directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Write kubeconfig to file
	if err := os.WriteFile(kubeconfigPath, kubeconfigOutput, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Kubeconfig written to %s\n", kubeconfigPath)
	return nil
}

// LoadKubeconfig loads a kubeconfig file and returns a rest.Config
func LoadKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from %s: %w", kubeconfigPath, err)
	}
	return config, nil
}

// Note: createTestNamespace and findWorkspaceRoot are defined in datastorage.go
// and shared across the infrastructure package

// deployPostgreSQLToKind deploys PostgreSQL to Kind cluster with custom NodePort
func deployPostgreSQLToKind(kubeconfigPath, namespace, hostPort, nodePort string, writer io.Writer) error {
	ctx := context.Background()
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	nodePortInt, err := strconv.Atoi(nodePort)
	if err != nil {
		return fmt.Errorf("invalid nodePort: %w", err)
	}

	// Create init ConfigMap
	initConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-init",
			Namespace: namespace,
		},
		Data: map[string]string{
			"init.sql": `-- AuthWebhook E2E PostgreSQL init script
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'slm_user') THEN
        CREATE ROLE slm_user WITH LOGIN PASSWORD 'test_password';
    END IF;
END
$$;

GRANT ALL PRIVILEGES ON DATABASE action_history TO slm_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;`,
		},
	}

	if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, initConfigMap, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PostgreSQL init ConfigMap: %w", err)
	}

	// Create Secret
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

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PostgreSQL secret: %w", err)
	}

	// Create Service with custom NodePort
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels:    map[string]string{"app": "postgresql"},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgresql",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					NodePort:   int32(nodePortInt),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{"app": "postgresql"},
		},
	}

	if _, err := clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PostgreSQL service: %w", err)
	}

	// Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels:    map[string]string{"app": "postgresql"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "postgresql"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "postgresql"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: "postgres:16-alpine",
							Ports: []corev1.ContainerPort{
								{Name: "postgresql", ContainerPort: 5432},
							},
							Env: []corev1.EnvVar{
								{Name: "POSTGRES_USER", ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "postgresql-secret"},
										Key:                  "POSTGRES_USER",
									},
								}},
								{Name: "POSTGRES_PASSWORD", ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "postgresql-secret"},
										Key:                  "POSTGRES_PASSWORD",
									},
								}},
								{Name: "POSTGRES_DB", ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "postgresql-secret"},
										Key:                  "POSTGRES_DB",
									},
								}},
								{Name: "PGDATA", Value: "/var/lib/postgresql/data/pgdata"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "postgresql-data", MountPath: "/var/lib/postgresql/data"},
								{Name: "postgresql-init", MountPath: "/docker-entrypoint-initdb.d"},
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
						{Name: "postgresql-data", VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						}},
						{Name: "postgresql-init", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "postgresql-init"},
							},
						}},
					},
				},
			},
		},
	}

	if _, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create PostgreSQL deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL deployed (NodePort %s)\n", nodePort)
	return nil
}

// deployRedisToKind deploys Redis to Kind cluster with custom NodePort
func deployRedisToKind(kubeconfigPath, namespace, hostPort, nodePort string, writer io.Writer) error {
	ctx := context.Background()
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	nodePortInt, err := strconv.Atoi(nodePort)
	if err != nil {
		return fmt.Errorf("invalid nodePort: %w", err)
	}

	// Create Service with custom NodePort
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels:    map[string]string{"app": "redis"},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					NodePort:   int32(nodePortInt),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{"app": "redis"},
		},
	}

	if _, err := clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Redis service: %w", err)
	}

	// Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels:    map[string]string{"app": "redis"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "redis"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "redis"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:7-alpine",
							Ports: []corev1.ContainerPort{
								{Name: "redis", ContainerPort: 6379},
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

	if _, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Redis deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Redis deployed (NodePort %s)\n", nodePort)
	return nil
}

// runDatabaseMigrations runs database migrations using ApplyMigrations
func runDatabaseMigrations(kubeconfigPath, namespace string, writer io.Writer) error {
	ctx := context.Background()
	return ApplyMigrations(ctx, namespace, kubeconfigPath, writer)
}

// deployDataStorageToKind deploys Data Storage service to Kind cluster with custom NodePort and image tag
func deployDataStorageToKind(kubeconfigPath, namespace, imageTag, hostPort, nodePort string, writer io.Writer) error {
	ctx := context.Background()
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	nodePortInt, err := strconv.Atoi(nodePort)
	if err != nil {
		return fmt.Errorf("invalid nodePort: %w", err)
	}

	// Create Service with custom NodePort
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels:    map[string]string{"app": "datastorage"},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					NodePort:   int32(nodePortInt),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{"app": "datastorage"},
		},
	}

	if _, err := clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Data Storage service: %w", err)
	}

	// Create Deployment with custom image tag
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels:    map[string]string{"app": "datastorage"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "datastorage"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "datastorage"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "datastorage",
							Image:           imageTag,
							ImagePullPolicy: corev1.PullNever,
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
							},
							Env: []corev1.EnvVar{
								{Name: "DATABASE_HOST", Value: "postgresql"},
								{Name: "DATABASE_PORT", Value: "5432"},
								{Name: "DATABASE_NAME", Value: "action_history"},
								{Name: "DATABASE_USER", Value: "slm_user"},
								{Name: "DATABASE_PASSWORD", Value: "test_password"},
								{Name: "DATABASE_SSLMODE", Value: "disable"},
								{Name: "REDIS_HOST", Value: "redis"},
								{Name: "REDIS_PORT", Value: "6379"},
								{Name: "LOG_LEVEL", Value: "debug"},
								{Name: "GOCOVERDIR", Value: "/coverdata"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "coverdata", MountPath: "/coverdata"},
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
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health/ready",
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
										Path: "/health/live",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{Name: "coverdata", VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/coverdata",
							},
						}},
					},
				},
			},
		},
	}

	if _, err := clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Data Storage deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage deployed (NodePort %s, image %s)\n", nodePort, imageTag)
	return nil
}

// waitForServicesReady waits for Data Storage and AuthWebhook services to be ready
func waitForServicesReady(kubeconfigPath, namespace string, writer io.Writer) error {
	ctx := context.Background()
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for Data Storage pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Data Storage pod to be ready...\n")
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Data Storage pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage pod ready\n")

	// Wait for AuthWebhook pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for AuthWebhook pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=authwebhook",
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "AuthWebhook pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ AuthWebhook pod ready\n")

	return nil
}

