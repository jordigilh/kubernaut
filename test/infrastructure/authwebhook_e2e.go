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
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			switch r.name {
			case "DataStorage":
				dsImageName = r.imageName
			case "AuthWebhook":
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
	// CRITICAL: os.MkdirAll applies umask (0022), resulting in 0755 (rwxr-xr-x).
	// Container processes (UID 1001) need write access to /coverdata via hostPath volume.
	// os.Chmod bypasses umask, ensuring world-writable permissions propagate through
	// the Kind bind mount → pod hostPath chain.
	if err := os.Chmod(coverdataPath, 0777); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to chmod coverdata directory: %v\n", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Created %s for coverage collection (mode=0777)\n", coverdataPath)

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

// loadAuthWebhookImageOnly loads a pre-built AuthWebhook image to Kind cluster.
// This is Phase 3 of the hybrid E2E pattern (load after cluster creation).
func loadAuthWebhookImageOnly(imageName, clusterName string, writer io.Writer) error {
	return LoadImageToKind(imageName, "authwebhook", clusterName, writer)
}

// authWebhookManifest generates the complete AuthWebhook multi-document YAML manifest.
// This is the single source of truth for all AuthWebhook E2E deployments.
// Includes: ServiceAccount, ClusterRole, ClusterRoleBinding, Service, ConfigMap,
// Deployment, MutatingWebhookConfiguration, ValidatingWebhookConfiguration.
func authWebhookManifest(namespace, imageTag string) string {
	pullPolicy := GetImagePullPolicy()

	return fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: authwebhook
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: authwebhook
    app.kubernetes.io/component: admission-webhook
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: authwebhook
  labels:
    app.kubernetes.io/name: authwebhook
    app.kubernetes.io/component: admission-webhook
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions", "remediationapprovalrequests", "notificationrequests", "remediationrequests"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["workflowexecutions/status", "remediationapprovalrequests/status", "remediationrequests/status"]
  verbs: ["update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: authwebhook
  labels:
    app.kubernetes.io/name: authwebhook
    app.kubernetes.io/component: admission-webhook
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: authwebhook
subjects:
- kind: ServiceAccount
  name: authwebhook
  namespace: %[1]s
---
apiVersion: v1
kind: Service
metadata:
  name: authwebhook
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: authwebhook
    app.kubernetes.io/component: admission-webhook
spec:
  selector:
    app.kubernetes.io/name: authwebhook
  ports:
  - name: https
    port: 443
    targetPort: webhook
    protocol: TCP
    nodePort: 30443
  type: NodePort
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: authwebhook-config
  namespace: %[1]s
data:
  authwebhook.yaml: |
    webhook:
      port: 9443
      certDir: /tmp/k8s-webhook-server/serving-certs
      healthProbeAddr: ":8081"
    datastorage:
      url: "http://data-storage-service:8080"
      timeout: 30s
      buffer:
        bufferSize: 1000
        batchSize: 100
        flushInterval: 5s
        maxRetries: 3
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: authwebhook
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: authwebhook
    app.kubernetes.io/component: admission-webhook
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app.kubernetes.io/name: authwebhook
  template:
    metadata:
      labels:
        app: authwebhook
        app.kubernetes.io/name: authwebhook
        app.kubernetes.io/component: admission-webhook
    spec:
      serviceAccountName: authwebhook
      containers:
      - name: authwebhook
        image: %[2]s
        imagePullPolicy: %[3]s
        args:
        - -config=/etc/authwebhook/authwebhook.yaml
        ports:
        - name: webhook
          containerPort: 9443
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        env:
        - name: LOG_LEVEL
          value: "debug"
        - name: GOCOVERDIR
          value: "/coverdata"
        volumeMounts:
        - name: webhook-certs
          mountPath: /tmp/k8s-webhook-server/serving-certs
          readOnly: true
        - name: config
          mountPath: /etc/authwebhook
          readOnly: true
        - name: coverdata
          mountPath: /coverdata
        resources:
          requests:
            memory: 128Mi
            cpu: 100m
          limits:
            memory: 256Mi
            cpu: 200m
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
            scheme: HTTP
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
            scheme: HTTP
          initialDelaySeconds: 15
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 6
      volumes:
      - name: webhook-certs
        secret:
          secretName: authwebhook-tls
      - name: config
        configMap:
          name: authwebhook-config
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: authwebhook-mutating
webhooks:
- name: workflowexecution.mutate.kubernaut.ai
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: authwebhook
      namespace: %[1]s
      path: /mutate-workflowexecution
    caBundle: ""
  failurePolicy: Fail
  matchPolicy: Equivalent
  rules:
  - apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    operations: ["UPDATE"]
    resources: ["workflowexecutions/status"]
    scope: "Namespaced"
  sideEffects: None
  timeoutSeconds: 10
- name: remediationapprovalrequest.mutate.kubernaut.ai
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: authwebhook
      namespace: %[1]s
      path: /mutate-remediationapprovalrequest
    caBundle: ""
  failurePolicy: Fail
  matchPolicy: Equivalent
  rules:
  - apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    operations: ["UPDATE"]
    resources: ["remediationapprovalrequests/status"]
    scope: "Namespaced"
  sideEffects: None
  timeoutSeconds: 10
- name: remediationrequest.mutate.kubernaut.ai
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: authwebhook
      namespace: %[1]s
      path: /mutate-remediationrequest
    caBundle: ""
  failurePolicy: Fail
  matchPolicy: Equivalent
  rules:
  - apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    operations: ["UPDATE"]
    resources: ["remediationrequests/status"]
    scope: "Namespaced"
  sideEffects: None
  timeoutSeconds: 10
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: authwebhook-validating
webhooks:
- name: notificationrequest.validate.kubernaut.ai
  admissionReviewVersions: ["v1"]
  clientConfig:
    service:
      name: authwebhook
      namespace: %[1]s
      path: /validate-notificationrequest-delete
    caBundle: ""
  failurePolicy: Fail
  matchPolicy: Equivalent
  rules:
  - apiGroups: ["kubernaut.ai"]
    apiVersions: ["v1alpha1"]
    operations: ["DELETE"]
    resources: ["notificationrequests"]
    scope: "Namespaced"
  sideEffects: None
  timeoutSeconds: 10
`, namespace, imageTag, pullPolicy)
}

// deployAuthWebhookToKind deploys the AuthWebhook service to Kind cluster.
// Standardized: uses authWebhookManifest() inline YAML template.
func deployAuthWebhookToKind(kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// STEP 1: Generate webhook TLS certificates
	_, _ = fmt.Fprintln(writer, "🔐 Generating webhook TLS certificates...")
	if err := generateWebhookCertsOnly(kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate webhook certs: %w", err)
	}

	// STEP 2: Apply CRDs first
	_, _ = fmt.Fprintln(writer, "📋 Applying CRDs...")
	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "config/crd/bases/")
	cmd.Dir = workspaceRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ CRD apply failed: %s\n", output)
		return fmt.Errorf("kubectl apply crds failed: %w", err)
	}

	// STEP 3: Apply AuthWebhook manifest (all resources including webhook configs)
	_, _ = fmt.Fprintln(writer, "🚀 Applying AuthWebhook deployment...")
	manifest := authWebhookManifest(namespace, imageTag)
	cmd = exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
	}

	// STEP 4: Patch webhook configurations with CA bundle
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
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                configPath,
		WaitTimeout:               "60s",
		DeleteExisting:            true,  // Original behavior: delete if exists
		ReuseExisting:             false,
		CleanupOrphanedContainers: true,  // Podman cleanup on macOS
		UsePodman:                 true,  // CRITICAL: Sets KIND_EXPERIMENTAL_PROVIDER=podman
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

// deployPostgreSQLToKind deploys PostgreSQL to Kind cluster with custom NodePort.
// Standardized: inline YAML template pattern.
func deployPostgreSQLToKind(kubeconfigPath, namespace, hostPort, nodePort string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgresql-init
data:
  init.sql: |
    -- AuthWebhook E2E PostgreSQL init script
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
    GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
stringData:
  POSTGRES_USER: slm_user
  POSTGRES_PASSWORD: test_password
  POSTGRES_DB: action_history
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  labels:
    app: postgresql
spec:
  type: NodePort
  ports:
  - name: postgresql
    port: 5432
    targetPort: 5432
    nodePort: %[1]s
    protocol: TCP
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  labels:
    app: postgresql
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
      - name: postgresql
        image: postgres:16-alpine
        ports:
        - name: postgresql
          containerPort: 5432
        env:
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgresql-secret
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgresql-secret
              key: POSTGRES_PASSWORD
        - name: POSTGRES_DB
          valueFrom:
            secretKeyRef:
              name: postgresql-secret
              key: POSTGRES_DB
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        volumeMounts:
        - name: postgresql-data
          mountPath: /var/lib/postgresql/data
        - name: postgresql-init
          mountPath: /docker-entrypoint-initdb.d
        resources:
          requests:
            memory: 256Mi
            cpu: 250m
          limits:
            memory: 512Mi
            cpu: 500m
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user"]
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
        livenessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user"]
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
      volumes:
      - name: postgresql-data
        emptyDir: {}
      - name: postgresql-init
        configMap:
          name: postgresql-init
`, nodePort)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL deployed (NodePort %s)\n", nodePort)
	return nil
}

// deployRedisToKind deploys Redis to Kind cluster with custom NodePort.
// Standardized: inline YAML template pattern.
func deployRedisToKind(kubeconfigPath, namespace, hostPort, nodePort string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: redis
  labels:
    app: redis
spec:
  type: NodePort
  ports:
  - name: redis
    port: 6379
    targetPort: 6379
    nodePort: %[1]s
    protocol: TCP
  selector:
    app: redis
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  labels:
    app: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - name: redis
          containerPort: 6379
        resources:
          requests:
            memory: 128Mi
            cpu: 100m
          limits:
            memory: 256Mi
            cpu: 200m
        readinessProbe:
          exec:
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
        livenessProbe:
          exec:
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
`, nodePort)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Redis deployed (NodePort %s)\n", nodePort)
	return nil
}

// runDatabaseMigrations runs database migrations using ApplyMigrations
func runDatabaseMigrations(kubeconfigPath, namespace string, writer io.Writer) error {
	ctx := context.Background()
	return ApplyMigrations(ctx, namespace, kubeconfigPath, writer)
}

// deployDataStorageToKind deploys Data Storage service to Kind cluster with custom NodePort and image tag.
// Standardized: inline YAML template pattern.
func deployDataStorageToKind(kubeconfigPath, namespace, imageTag, hostPort, nodePort string, writer io.Writer) error {
	pullPolicy := GetImagePullPolicy()

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
data:
  config.yaml: |
    server:
      port: 8080
      host: "0.0.0.0"
      readTimeout: 30s
      writeTimeout: 30s
    database:
      host: postgresql.%[1]s.svc.cluster.local
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
      addr: redis.%[1]s.svc.cluster.local:6379
      db: 0
      dlqStreamName: dlq-stream
      dlqMaxLen: 1000
      dlqConsumerGroup: dlq-group
      secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
      passwordKey: "password"
    logging:
      level: debug
      format: json
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  labels:
    app: datastorage
spec:
  type: NodePort
  ports:
  - name: http
    port: 8080
    targetPort: 8080
    nodePort: %[2]s
    protocol: TCP
  selector:
    app: datastorage
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  labels:
    app: datastorage
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
        image: %[3]s
        imagePullPolicy: %[4]s
        ports:
        - name: http
          containerPort: 8080
        - name: metrics
          containerPort: 9181
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        - name: GOCOVERDIR
          value: /coverdata
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        - name: coverdata
          mountPath: /coverdata
        resources:
          requests:
            memory: 256Mi
            cpu: 250m
          limits:
            memory: 512Mi
            cpu: 500m
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
      - name: coverdata
        hostPath:
          path: /coverdata
`, namespace, nodePort, imageTag, pullPolicy)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
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
