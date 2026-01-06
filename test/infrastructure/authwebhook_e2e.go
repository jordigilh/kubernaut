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

// buildAuthWebhookImageWithTag builds the AuthWebhook Docker image with a specific tag
func buildAuthWebhookImageWithTag(imageTag string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔨 Building AuthWebhook image: %s\n", imageTag)

	// Build image using podman (follows DataStorage pattern)
	cmd := exec.Command("podman", "build",
		"-t", imageTag,
		"-f", "cmd/authwebhook/Dockerfile",
		".")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Build failed: %s\n", output)
		return fmt.Errorf("podman build failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ AuthWebhook image built successfully")
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

// generateWebhookCerts generates TLS certificates for webhook admission
func generateWebhookCerts(kubeconfigPath, namespace string, writer io.Writer) error {
	// Use openssl to generate self-signed certificates for testing
	// In production, use cert-manager

	// Generate private key
	cmd := exec.Command("openssl", "genrsa", "-out", "/tmp/webhook-key.pem", "2048")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Key generation failed: %s\n", output)
		return fmt.Errorf("openssl genrsa failed: %w", err)
	}

	// Generate certificate
	cmd = exec.Command("openssl", "req", "-new", "-x509",
		"-key", "/tmp/webhook-key.pem",
		"-out", "/tmp/webhook-cert.pem",
		"-days", "365",
		"-subj", fmt.Sprintf("/CN=authwebhook.%s.svc", namespace))
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Cert generation failed: %s\n", output)
		return fmt.Errorf("openssl req failed: %w", err)
	}

	// Create/update secret with certificates
	cmd = exec.Command("kubectl", "create", "secret", "tls", "authwebhook-tls",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"--cert=/tmp/webhook-cert.pem",
		"--key=/tmp/webhook-key.pem",
		"--dry-run=client",
		"-o", "yaml")
	secretYaml, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Secret creation failed: %s\n", secretYaml)
		return fmt.Errorf("kubectl create secret failed: %w", err)
	}

	cmd = exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "-")
	cmd.Stdin = exec.Command("echo", string(secretYaml)).Stdout
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Secret apply failed: %s\n", output)
		return fmt.Errorf("kubectl apply secret failed: %w", err)
	}

	// Read CA bundle for webhook configuration
	// TODO: Patch webhook configurations with caBundle

	_, _ = fmt.Fprintln(writer, "✅ Webhook TLS certificates created")
	return nil
}

// createKindClusterWithConfig creates a Kind cluster with a specific config file
func createKindClusterWithConfig(clusterName, kubeconfigPath, configPath string, writer io.Writer) error {
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--kubeconfig", kubeconfigPath,
		"--config", configPath,
		"--wait", "60s")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to create cluster: %s\n", output)
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Kind cluster created")
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

