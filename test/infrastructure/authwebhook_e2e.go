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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupAuthWebhookInfrastructureParallel creates the full AuthWebhook E2E infrastructure with hybrid pattern.
// This optimizes setup time by building images before cluster creation.
//
// Hybrid Execution Strategy:
//
//	Phase 1 (PARALLEL):   Build DataStorage + AuthWebhook images (BEFORE cluster) (~90s)
//	Phase 2 (Sequential): Create Kind cluster + namespace (~65s)
//	Phase 3 (PARALLEL):   Load images + Deploy PostgreSQL + Deploy Redis (~60s)
//	Phase 4 (Sequential): Run migrations (~30s)
//	Phase 5 (Sequential): Deploy DataStorage + AuthWebhook services (~45s)
//	Phase 6 (Sequential): Wait for services ready (~30s)
//
// Total time: ~4.3 minutes (eliminates cluster idle time during builds)
//
// PostgreSQL-only architecture (SOC2 audit storage)
// Authority: Gateway/DataStorage hybrid pattern migration (Jan 7, 2026)
func SetupAuthWebhookInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) (awImage, dsImage string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 AuthWebhook E2E Infrastructure (HYBRID PATTERN)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build images → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Optimization: Eliminates cluster idle time during image builds")
	_, _ = fmt.Fprintln(writer, "  Authority: Gateway/DataStorage hybrid pattern migration (Jan 7, 2026)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images in PARALLEL (BEFORE cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel (NO CLUSTER YET)...")
	_, _ = fmt.Fprintln(writer, "  ├── DataStorage image")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook image")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~1-2 minutes")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}
	buildResults := make(chan buildResult, 2)

	// Goroutine 1: Build DataStorage image
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		dsImageName, err := BuildImageForKind(cfg, writer)
		if err != nil {
			err = fmt.Errorf("DS image build failed: %w", err)
		}
		buildResults <- buildResult{name: "DataStorage", imageName: dsImageName, err: err}
	}()

	// Goroutine 2: Build AuthWebhook image using standardized BuildImageForKind (with registry pull fallback)
	// Registry Strategy: Attempts pull from ghcr.io first, falls back to local build
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "authwebhook",
			ImageName:        "authwebhook", // No repo prefix, just service name
			DockerfilePath:   "docker/authwebhook.Dockerfile",
			BuildContextPath: "", // Empty = project root
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		awImageName, err := BuildImageForKind(cfg, writer)
		if err != nil {
			err = fmt.Errorf("AuthWebhook image build failed: %w", err)
		}
		buildResults <- buildResult{name: "AuthWebhook", imageName: awImageName, err: err}
	}()

	// Collect build results
	var dsImageName, awImageName string
	var buildErrors []string
	for i := 0; i < 2; i++ {
		r := <-buildResults
		if r.err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("%s: %v", r.name, r.err))
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", r.name, r.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed\n", r.name)
			if r.name == "DataStorage" {
				dsImageName = r.imageName
			} else if r.name == "AuthWebhook" {
				awImageName = r.imageName
			}
		}
	}
	if len(buildErrors) > 0 {
		return "", "", fmt.Errorf("image builds failed: %v", buildErrors)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster + namespace
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster + namespace...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	// Create ./coverdata directory for coverage collection (required by kind-config.yaml)
	// Kind interprets relative paths relative to where the config file is located
	// So ./coverdata in test/e2e/authwebhook/kind-config.yaml means test/e2e/authwebhook/coverdata
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return "", "", fmt.Errorf("failed to find workspace root: %w", err)
	}
	coverdataPath := filepath.Join(workspaceRoot, "test", "e2e", "authwebhook", "coverdata")
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return "", "", fmt.Errorf("failed to create coverdata directory: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Created %s for coverage collection\n", coverdataPath)

	// Create Kind cluster with authwebhook-specific config
	if err := createKindClusterWithConfig(clusterName, kubeconfigPath, "test/e2e/authwebhook/kind-config.yaml", writer); err != nil {
		return "", "", fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Create namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return "", "", fmt.Errorf("failed to create namespace: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images + Deploy infrastructure in PARALLEL
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⚡ PHASE 3: Loading images + Deploying infrastructure in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Loading DataStorage image to Kind")
	_, _ = fmt.Fprintln(writer, "  ├── Loading AuthWebhook image to Kind")
	_, _ = fmt.Fprintln(writer, "  ├── Deploying PostgreSQL")
	_, _ = fmt.Fprintln(writer, "  └── Deploying Redis")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-60 seconds")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 4)

	// Goroutine 1: Load pre-built DataStorage image
	go func() {
		err := LoadImageToKind(dsImageName, "datastorage", clusterName, writer)
		if err != nil {
			err = fmt.Errorf("DS image load failed: %w", err)
		}
		results <- result{name: "DS image load", err: err}
	}()

	// Goroutine 2: Load pre-built AuthWebhook image
	go func() {
		err := loadAuthWebhookImageOnly(awImageName, clusterName, writer)
		if err != nil {
			err = fmt.Errorf("AuthWebhook image load failed: %w", err)
		}
		results <- result{name: "AuthWebhook image load", err: err}
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
	var errors []string
	for i := 0; i < 4; i++ {
		res := <-results
		if res.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", res.name, res.err))
			_, _ = fmt.Fprintf(writer, "  ❌ %s failed: %v\n", res.name, res.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s complete\n", res.name)
		}
	}

	if len(errors) > 0 {
		return "", "", fmt.Errorf("parallel load/deploy failed: %v", errors)
	}

	_, _ = fmt.Fprintln(writer, "✅ Phase 3 complete - images loaded + infrastructure deployed")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Run database migrations (Sequential - depends on PostgreSQL)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 4: Running database migrations...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~20-30 seconds")
	if err := runDatabaseMigrations(kubeconfigPath, namespace, writer); err != nil {
		return "", "", fmt.Errorf("failed to run migrations: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Deploy services (Sequential - depends on migrations)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🚀 PHASE 5: Deploying services...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-45 seconds")

	// Deploy DataStorage service (E2E ports per DD-TEST-001)
	_, _ = fmt.Fprintln(writer, "  📦 Deploying DataStorage service...")
	if err := deployDataStorageToKind(kubeconfigPath, namespace, dsImageName, "28099", "30099", writer); err != nil {
		return "", "", fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// Deploy AuthWebhook service with webhook configurations
	_, _ = fmt.Fprintln(writer, "  🔐 Deploying AuthWebhook service...")
	// Deploy AuthWebhook service using pre-built image from Phase 1
	if err := deployAuthWebhookToKind(kubeconfigPath, namespace, awImageName, writer); err != nil {
		return "", "", fmt.Errorf("failed to deploy AuthWebhook: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for services to be ready (Sequential - verification)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 6: Waiting for services to be ready...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~20-30 seconds")
	if err := waitForServicesReady(kubeconfigPath, namespace, writer); err != nil {
		return "", "", fmt.Errorf("services failed to become ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ AuthWebhook E2E infrastructure ready!")
	_, _ = fmt.Fprintf(writer, "  🖼️  AuthWebhook image: %s\n", awImageName)
	_, _ = fmt.Fprintf(writer, "  🖼️  DataStorage image: %s\n", dsImageName)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return awImageName, dsImageName, nil
}

// buildAuthWebhookImageWithTag builds the AuthWebhook Docker image with a specific tag
// buildAuthWebhookImageOnly builds AuthWebhook image without loading it to Kind.
// This is Phase 1 of the hybrid E2E pattern (build before cluster creation).
//
// Returns: Image name with localhost/ prefix for later loading
func buildAuthWebhookImageOnly(writer io.Writer) (string, error) {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Generate unique image tag using DD-TEST-001 pattern
	imageTag := GenerateInfraImageName("authwebhook", "e2e")
	_, _ = fmt.Fprintf(writer, "🔨 Building AuthWebhook image: %s\n", imageTag)

	cmd := exec.Command("podman", "build",
		"--no-cache",
		"-t", imageTag,
		"-f", "docker/authwebhook.Dockerfile",
		".")
	cmd.Dir = workspaceRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("podman build failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ AuthWebhook image built: %s\n", imageTag)
	return imageTag, nil
}

// loadAuthWebhookImageOnly loads a pre-built AuthWebhook image to Kind cluster.
// This is Phase 3 of the hybrid E2E pattern (load after cluster creation).
func loadAuthWebhookImageOnly(imageName, clusterName string, writer io.Writer) error {
	return LoadImageToKind(imageName, "authwebhook", clusterName, writer)
}

// deployAuthWebhookToKind deploys the AuthWebhook service to Kind cluster
func deployAuthWebhookToKind(kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	// Get workspace root for config paths
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// STEP 1: Generate webhook TLS certificates (but don't patch yet - webhook configs don't exist)
	_, _ = fmt.Fprintln(writer, "🔐 Generating webhook TLS certificates...")
	if err := generateWebhookCertsOnly(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate webhook certs: %w", err)
	}

	// STEP 2: Apply CRDs first
	_, _ = fmt.Fprintln(writer, "📋 Applying CRDs...")
	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "config/crd/bases/")
	cmd.Dir = workspaceRoot // Run from workspace root

	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ CRD apply failed: %s\n", output)
		return fmt.Errorf("kubectl apply crds failed: %w", err)
	}

	// STEP 3: Apply webhook deployment (creates webhook configurations with empty caBundle)
	_, _ = fmt.Fprintln(writer, "🚀 Applying AuthWebhook deployment...")
	// Read and substitute namespace in manifest
	manifestPath := filepath.Join(workspaceRoot, "test/e2e/authwebhook/manifests/authwebhook-deployment.yaml")
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Replace ${WEBHOOK_NAMESPACE} with actual namespace
	substitutedManifest := strings.ReplaceAll(string(manifestContent), "${WEBHOOK_NAMESPACE}", namespace)

	// Replace hardcoded imagePullPolicy with dynamic value
	// CI/CD mode (IMAGE_REGISTRY set): Use IfNotPresent (allows pulling from GHCR)
	// Local mode: Use Never (uses images loaded into Kind)
	substitutedManifest = strings.ReplaceAll(substitutedManifest,
		"imagePullPolicy: Never",
		fmt.Sprintf("imagePullPolicy: %s", GetImagePullPolicy()))

	// Apply substituted manifest via kubectl
	cmd = exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "-") // Read from stdin
	cmd.Stdin = strings.NewReader(substitutedManifest)
	cmd.Dir = workspaceRoot

	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Deployment failed: %s\n", output)
		return fmt.Errorf("kubectl apply failed: %w", err)
	}

	// STEP 4: Patch deployment with correct image tag
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

	// STEP 5: NOW patch webhook configurations with CA bundle (after they exist)
	_, _ = fmt.Fprintln(writer, "🔐 Patching webhook configurations with CA bundle...")
	if err := patchWebhookConfigurations(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to patch webhook configurations: %w", err)
	}

	return nil
}

// generateWebhookCertsOnly generates TLS certificates for webhook admission WITHOUT patching
// This must be called BEFORE the deployment (which creates the webhook configurations)
func generateWebhookCertsOnly(kubeconfigPath, namespace string, writer io.Writer) error {
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

	_, _ = fmt.Fprintln(writer, "   ✅ Webhook TLS secret created")
	return nil
}

// patchWebhookConfigurations patches webhook configurations with CA bundle
// This must be called AFTER the deployment is applied (webhook configurations must exist)
func patchWebhookConfigurations(kubeconfigPath string, writer io.Writer) error {
	// Base64 encode the certificate for CA bundle
	caBundleOutput, err := exec.Command("bash", "-c", "cat /tmp/webhook-cert.pem | base64 | tr -d '\\n'").Output()
	if err != nil {
		return fmt.Errorf("failed to base64 encode CA bundle: %w", err)
	}
	caBundleB64 := string(caBundleOutput)

	// Patch each webhook in MutatingWebhookConfiguration
	_, _ = fmt.Fprintln(writer, "   🔧 Patching MutatingWebhookConfiguration webhooks...")
	webhookNames := []string{"workflowexecution.mutate.kubernaut.ai", "remediationapprovalrequest.mutate.kubernaut.ai", "remediationrequest.mutate.kubernaut.ai"}
	for i, webhookName := range webhookNames {
		patchCmd := exec.Command("kubectl", "patch", "mutatingwebhookconfiguration", "authwebhook-mutating",
			"--kubeconfig", kubeconfigPath,
			"--type=json",
			"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/%d/clientConfig/caBundle","value":"%s"}]`, i, caBundleB64))
		if output, err := patchCmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(writer, "   ❌ Failed to patch %s: %s\n", webhookName, output)
			return fmt.Errorf("failed to patch mutating webhook %s: %w", webhookName, err)
		}
		_, _ = fmt.Fprintf(writer, "   ✅ Patched %s\n", webhookName)
	}

	// Patch ValidatingWebhookConfiguration
	_, _ = fmt.Fprintln(writer, "   🔧 Patching ValidatingWebhookConfiguration...")
	patchCmd := exec.Command("kubectl", "patch", "validatingwebhookconfiguration", "authwebhook-validating",
		"--kubeconfig", kubeconfigPath,
		"--type=json",
		"-p", fmt.Sprintf(`[{"op":"replace","path":"/webhooks/0/clientConfig/caBundle","value":"%s"}]`, caBundleB64))
	if output, err := patchCmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ Failed to patch validating webhook: %s\n", output)
		return fmt.Errorf("failed to patch validating webhook: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "   ✅ Patched validating webhook")

	_, _ = fmt.Fprintln(writer, "✅ All webhook configurations patched with CA bundle")
	return nil
}

// createKindClusterWithConfig creates a Kind cluster with a specific config file
// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
func createKindClusterWithConfig(clusterName, kubeconfigPath, configPath string, writer io.Writer) error {
	opts := KindClusterOptions{
		ClusterName:    clusterName,
		KubeconfigPath: kubeconfigPath,
		ConfigPath:     configPath,
		WaitTimeout:    "60s",
		DeleteExisting: true, // Original behavior: delete if exists
		ReuseExisting:  false,
	}
	return CreateKindClusterWithConfig(opts, writer)
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
		// Handle case where secret already exists (from previous test run)
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create PostgreSQL secret: %w", err)
		}
		// Secret exists, update it to ensure it has correct values
		if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to update existing PostgreSQL secret: %w", err)
		}
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
// Uses ConfigMap + Secret pattern (same as datastorage E2E) for proper service configuration
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

	// Create ConfigMap for service configuration (required by Data Storage)
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
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: redis.%s.svc.cluster.local:6379
  db: 0
  dlqStreamName: dlq-stream
  dlqMaxLen: 1000
  dlqConsumerGroup: dlq-group
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

	if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Data Storage ConfigMap: %w", err)
	}

	// Create Secret for database and Redis credentials
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

	if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create Data Storage Secret: %w", err)
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
							ImagePullPolicy: GetImagePullPolicyV1(), // Dynamic: IfNotPresent (CI/CD) or Never (local)
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 8080},
								{Name: "metrics", ContainerPort: 9181},
							},
							Env: []corev1.EnvVar{
								{Name: "CONFIG_PATH", Value: "/etc/datastorage/config.yaml"},
								{Name: "GOCOVERDIR", Value: "/coverdata"},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config", MountPath: "/etc/datastorage", ReadOnly: true},
								{Name: "secrets", MountPath: "/etc/datastorage/secrets", ReadOnly: true},
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
						{Name: "config", VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "datastorage-config",
								},
							},
						}},
						{Name: "secrets", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: "datastorage-secret",
							},
						}},
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
