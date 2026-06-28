/*
Copyright 2026 Jordi Gil.

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
)

// SetupKubernautAgentInfrastructure provisions a complete E2E environment for the
// Kubernaut Agent (Go), reusing the same DataStorage + Mock LLM stack as AIAnalysis E2E but
// deploying the Go binary from docker/kubernautagent.Dockerfile.
//
// Port allocations (same as AIAnalysis/KA E2E per DD-TEST-001 v2.9):
//   - Kubernaut Agent: 30088 (NodePort) → 8443 (container, Host Port 8088)
//   - Data Storage:    30089 (NodePort) → 8080 (container, Host Port 8089)
//   - PostgreSQL:      30439 (NodePort) → 5432 (container)
//   - Redis:           30387 (NodePort) → 6379 (container)
func SetupKubernautAgentInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Kubernaut Agent E2E Infrastructure Setup (#433)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getProjectRoot()

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images in parallel (per DD-TEST-002)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"datastorage", imageName, err}
	}()

	// ADR-027: Build KA from UBI10 go-toolset → ubi10-minimal (development stage)
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "kubernautagent",
			ImageName:        "kubernautagent",
			DockerfilePath:   "docker/kubernautagent.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"kubernautagent", imageName, err}
	}()

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "mock-llm",
			ImageName:        "mock-llm",
			DockerfilePath:   "test/services/mock-llm/go.Dockerfile",
			BuildContextPath: projectRoot,
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{"mock-llm", imageName, err}
	}()

	images := make(map[string]string, 3)
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("failed to build %s: %w", result.name, result.err)
		}
		images[result.name] = result.image
		_, _ = fmt.Fprintf(writer, "  ✅ %s: %s\n", result.name, result.image)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (reuse AIAnalysis E2E Kind config — same ports)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🏗️  PHASE 2: Creating Kind cluster...")
	if err := createKAKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into Kind
	// ═══════════════════════════════════════════════════════════════════════
	if os.Getenv("IMAGE_REGISTRY") != "" {
		_, _ = fmt.Fprintln(writer, "\n⏭️  PHASE 3: Skipping local image loading (CI/CD: IMAGE_REGISTRY set, Kind pulls from registry)")
	} else {
		_, _ = fmt.Fprintln(writer, "\n📤 PHASE 3: Loading images into Kind...")
		for name, image := range images {
			if err := loadImageToKind(clusterName, image, writer); err != nil {
				return fmt.Errorf("failed to load %s image: %w", name, err)
			}
			_, _ = fmt.Fprintf(writer, "  ✅ %s loaded\n", name)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy DataStorage stack (PostgreSQL + Redis + DS + Migrations)
	// Reuses the same inline pattern as CreateAIAnalysisClusterHybrid.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 4: Deploying DataStorage stack...")
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Issue #785: Generate inter-service TLS before deploying services
	_, _ = fmt.Fprintln(writer, "  🔐 Generating inter-service TLS certificates (Issue #785)...")
	if _, err := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}

	// AU-9: Generate RSA signing certificate for audit exports
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
	}

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 4)

	go func() {
		deployResults <- deployResult{"PostgreSQL", deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		deployResults <- deployResult{"Redis", deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		deployResults <- deployResult{"Migrations", ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		if rbacErr := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); rbacErr != nil {
			deployResults <- deployResult{"DataStorage", rbacErr}
			return
		}
		deployResults <- deployResult{"DataStorage",
			deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, images["datastorage"], 30089, writer)}
	}()

	for i := 0; i < 4; i++ {
		result := <-deployResults
		if result.err != nil {
			return fmt.Errorf("%s deployment failed: %w", result.name, result.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", result.name)
	}

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DataStorage to be ready...")
	if err := waitForDataStorageReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4.5: Deploy data-storage-client ClusterRole (DD-AUTH-014)
	// CRITICAL: Must exist BEFORE RoleBindings that reference it (Phase 5)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 4.5: Deploying data-storage-client ClusterRole...")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Seed workflows + deploy Mock LLM (same as AIAnalysis E2E Phase 4c/4d)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🌱 PHASE 5: Seeding workflows and deploying Mock LLM...")
	if err := CreateKAE2EServiceAccount(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create E2E service account: %w", err)
	}

	if err := createKAE2EClientRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create KA E2E client RBAC: %w", err)
	}

	e2eSAName := "kubernaut-agent-e2e-sa"
	saToken, err := GetServiceAccountToken(ctx, namespace, e2eSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get SA token: %w", err)
	}

	dsURL := "https://localhost:8089"
	seedClient, err := CreateTLSAuthenticatedDataStorageClient(dsURL, saToken, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create DS client: %w", err)
	}

	if err := SeedActionTypesViaAPI(seedClient, writer); err != nil {
		return fmt.Errorf("failed to seed action types: %w", err)
	}

	testWorkflows := GetKAE2ETestWorkflows()
	workflowUUIDs, err := SeedWorkflowsInDataStorage(seedClient, testWorkflows, "KA E2E (via infrastructure)", writer)
	if err != nil {
		return fmt.Errorf("failed to seed workflows: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Seeded %d workflows\n", len(workflowUUIDs))

	if err := DeployMockLLMInNamespace(ctx, namespace, kubeconfigPath, images["mock-llm"], workflowUUIDs, nil, writer); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM: %w", err)
	}

	// Deploy shadow alignment evaluation instance (same image, mode: shadow)
	if err := DeployMockLLMShadowInNamespace(ctx, namespace, kubeconfigPath, images["mock-llm"], writer); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM Shadow: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5.5: Install CRDs required by interactive tests (#703)
	// The RemediationRequest CRD is needed so createTestRemediationRequest()
	// in interactive E2E tests can provision RR fixtures for the
	// RRExistenceChecker (HARM-004). Without this, the K8s API rejects
	// CR creation with "no matches for kubernaut.ai/v1alpha1".
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📋 PHASE 5.5: Installing CRDs for interactive tests (#703)...")
	if err := installKAE2ECRDs(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5.6: Install ambiguous-kind CRDs for apiVersion gate E2E (#1044)
	// Two CRDs with Kind "TestWidget" in different API groups so the REST
	// mapper reports the kind as ambiguous. Must be installed BEFORE KA
	// starts so its mapper discovers both groups at init.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔧 PHASE 5.6: Installing ambiguous-kind CRDs (#1044)...")
	if err := createAmbiguousKindCRDs(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create ambiguous-kind CRDs: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5.7: Deploy Prometheus + AlertManager (#1507)
	// Required for get_alerts/get_silences/nodes_log/nodes_stats_summary
	// tool E2E validation. Uses existing DeployPrometheus/DeployAlertManager
	// helpers from prometheus_alertmanager_e2e.go.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📊 PHASE 5.7: Deploying Prometheus + AlertManager (#1507)...")
	if err := DeployPrometheus(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Prometheus: %w", err)
	}
	if err := DeployAlertManager(ctx, namespace, kubeconfigPath, "", writer); err != nil {
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5.8: Deploy DEX OIDC Provider for JWT E2E testing (#1009)
	// Must be ready BEFORE KA starts so JWKS pre-warm succeeds.
	// DD-AUTH-MCP-001 v2.0: Pattern B validation with real OIDC provider.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 5.8: Deploying DEX OIDC Provider (#1009)...")
	if err := PreloadDexImage(clusterName, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to preload DEX image (non-fatal, Kind may pull): %v\n", err)
	}
	if err := deployDexInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy DEX: %w", err)
	}
	if err := waitForDexReady(writer); err != nil {
		return fmt.Errorf("DEX not ready: %w", err)
	}
	if err := createDexUserRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create DEX user RBAC: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Deploy Kubernaut Agent
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🤖 PHASE 6: Deploying Kubernaut Agent...")
	if err := DeployKubernautAgentServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy KA RBAC: %w", err)
	}
	if err := DeployKubernautAgentOnly(clusterName, kubeconfigPath, namespace, images["kubernautagent"], true, writer); err != nil {
		return fmt.Errorf("failed to deploy Kubernaut Agent: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7: Create enrichment fixture resources (#704)
	// Mock LLM scenarios reference resources in production/staging namespaces.
	// These must exist so re-enrichment doesn't trigger HardFail (rca_incomplete)
	// for scenarios that are NOT rca_incomplete.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 7: Creating enrichment fixture resources (#704)...")
	if err := createEnrichmentFixtures(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create enrichment fixtures: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ Kubernaut Agent E2E infrastructure ready")
	return nil
}

// installKAE2ECRDs installs the Kubernaut CRDs required by interactive E2E tests.
// The RemediationRequest CRD is mandatory for createTestRemediationRequest() which
// provisions RR fixtures so the RRExistenceChecker (HARM-004) allows sessions to start.
func installKAE2ECRDs(kubeconfigPath string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	crdFiles := []string{
		"kubernaut.ai_remediationrequests.yaml",
	}
	for _, crdFile := range crdFiles {
		crdPath := filepath.Join(projectRoot, "config/crd/bases", crdFile)
		_, _ = fmt.Fprintf(writer, "  ├── Installing %s...\n", crdFile)
		crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
		crdCmd.Stdout = writer
		crdCmd.Stderr = writer
		if err := crdCmd.Run(); err != nil {
			return fmt.Errorf("failed to install CRD %s: %w", crdFile, err)
		}
	}
	_, _ = fmt.Fprintln(writer, "  ✅ CRDs installed")
	return nil
}

// createEnrichmentFixtures creates namespaces and minimal workloads that mock LLM
// scenarios reference as remediation_target. Without these, re-enrichment returns
// NotFound → HardFail → rca_incomplete, breaking tests that expect normal outcomes.
// Includes Pod/test-pod/default for the default fallback mock scenario.
// The rca_incomplete scenario targets unreachable-pod which is intentionally NOT
// created so that it triggers HardFail as expected.
//
// Resources created:
//   production: api-server (Deployment), failing-pod, recovered-pod, api-server-def456,
//               ambiguous-pod, failed-analysis-pod (Pods), batch-job-pvc-expired (PVC)
//   staging:    worker (Deployment), worker-pdb (PDB — required so CrashLoopBackOff
//               re-enrichment to worker/staging preserves pdbProtected detection)
//
// Note: an empty enrichment: {} YAML section in the KA ConfigMap will zero out the
// HAPI defaults (MaxRetries=3 → 0), silently disabling retry+fail-hard. The E2E
// ConfigMap intentionally omits the enrichment key so DefaultConfig() applies.
func createEnrichmentFixtures(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	manifest := `---
apiVersion: v1
kind: Namespace
metadata:
  name: production
---
apiVersion: v1
kind: Namespace
metadata:
  name: staging
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: production
spec:
  replicas: 1
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.9
        resources:
          requests:
            memory: "8Mi"
            cpu: "10m"
          limits:
            memory: "16Mi"
            cpu: "50m"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker
  namespace: staging
spec:
  replicas: 1
  selector:
    matchLabels:
      app: worker
  template:
    metadata:
      labels:
        app: worker
    spec:
      containers:
      - name: pause
        image: registry.k8s.io/pause:3.9
        resources:
          requests:
            memory: "8Mi"
            cpu: "10m"
          limits:
            memory: "16Mi"
            cpu: "50m"
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: worker-pdb
  namespace: staging
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: worker
---
apiVersion: v1
kind: Pod
metadata:
  name: recovered-pod
  namespace: production
  labels:
    app: recovered-pod
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
---
apiVersion: v1
kind: Pod
metadata:
  name: api-server-def456
  namespace: production
  labels:
    app: api-server-def456
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
---
apiVersion: v1
kind: Pod
metadata:
  name: ambiguous-pod
  namespace: production
  labels:
    app: ambiguous-pod
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
---
apiVersion: v1
kind: Pod
metadata:
  name: failing-pod
  namespace: production
  labels:
    app: failing-pod
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
---
apiVersion: v1
kind: Pod
metadata:
  name: failed-analysis-pod
  namespace: production
  labels:
    app: failed-analysis-pod
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: batch-job-pvc-expired
  namespace: production
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Mi
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
  labels:
    app: test-pod
spec:
  restartPolicy: Never
  containers:
  - name: pause
    image: registry.k8s.io/pause:3.9
    resources:
      requests:
        memory: "8Mi"
        cpu: "10m"
      limits:
        memory: "16Mi"
        cpu: "50m"
`
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply enrichment fixtures: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Enrichment fixtures created (2 namespaces + 10 resources)")
	return nil
}

// createAmbiguousKindCRDs installs two CRDs with the same Kind ("TestWidget")
// in different API groups. This makes the REST mapper report TestWidget as
// ambiguous, which is required for apiVersionValidationGate E2E tests (#1044).
// Must be called BEFORE the KA deployment so its mapper discovers both groups.
func createAmbiguousKindCRDs(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	manifest := `---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testwidgets.alpha.kubernaut-test.ai
spec:
  group: alpha.kubernaut-test.ai
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
  scope: Namespaced
  names:
    plural: testwidgets
    singular: testwidget
    kind: TestWidget
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testwidgets.beta.kubernaut-test.ai
spec:
  group: beta.kubernaut-test.ai
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
  scope: Namespaced
  names:
    plural: testwidgets
    singular: testwidget
    kind: TestWidget
`
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply ambiguous-kind CRDs: %w", err)
	}

	// Wait for both CRDs to be established before KA starts
	for _, crd := range []string{
		"testwidgets.alpha.kubernaut-test.ai",
		"testwidgets.beta.kubernaut-test.ai",
	} {
		waitCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"wait", "--for=condition=Established", "crd/"+crd, "--timeout=30s")
		waitCmd.Stdout = writer
		waitCmd.Stderr = writer
		if err := waitCmd.Run(); err != nil {
			return fmt.Errorf("CRD %s not established: %w", crd, err)
		}
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Ambiguous-kind CRDs installed (TestWidget in alpha + beta groups)")

	// Create a TestWidget CR in the alpha group so enrichment has a resource to resolve.
	crManifest := `---
apiVersion: alpha.kubernaut-test.ai/v1
kind: TestWidget
metadata:
  name: test-widget-instance
  namespace: default
spec: {}
`
	crCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	crCmd.Stdin = strings.NewReader(crManifest)
	crCmd.Stdout = writer
	crCmd.Stderr = writer
	if err := crCmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply TestWidget CR: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ TestWidget CR created in default namespace (alpha group)")
	return nil
}

// createKAE2EClientRBAC grants the E2E ServiceAccount permission to call
// the Kubernaut Agent API (DD-AUTH-014: SAR check on services/kubernaut-agent).
func createKAE2EClientRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	rbacYAML := fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-agent-e2e-client-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-014
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["kubernaut-agent"]
    verbs: ["create", "get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubernaut-agent-e2e-client-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kubernaut-agent-e2e-client-access
subjects:
  - kind: ServiceAccount
    name: kubernaut-agent-e2e-sa
    namespace: %s
`, namespace, namespace, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(rbacYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply KA E2E client RBAC: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Kubernaut Agent E2E client RBAC created")
	return nil
}

// DeployKubernautAgentServiceRBAC creates the ServiceAccount and RBAC for KA pods.
// Mirrors the KA RBAC pattern (DD-AUTH-014) with KA-specific names.
func DeployKubernautAgentServiceRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	rbacManifest := fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-agent-sa
  namespace: %s
  labels:
    app: kubernaut-agent
    component: auth
    authorization: dd-auth-014
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-client
  labels:
    app: data-storage-service
    component: rbac
    authorization: dd-auth-014
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["data-storage-service"]
    verbs: ["create", "get", "list", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubernaut-agent-data-storage-client
  namespace: %s
  labels:
    app: kubernaut-agent
    component: rbac
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
  - kind: ServiceAccount
    name: kubernaut-agent-sa
    namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-agent-auth-middleware
  labels:
    app: kubernaut-agent
    component: rbac
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-auth-middleware
subjects:
  - kind: ServiceAccount
    name: kubernaut-agent-sa
    namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-agent-investigator
  labels:
    app: kubernaut-agent
    component: investigation
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log", "events", "services", "configmaps", "nodes", "nodes/proxy", "namespaces", "replicationcontrollers", "persistentvolumeclaims"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["events.k8s.io"]
    resources: ["events"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "list"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list"]
  - apiGroups: ["cert-manager.io"]
    resources: ["certificates", "clusterissuers", "certificaterequests"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods", "nodes"]
    verbs: ["get", "list"]
  # Interactive mode: RR existence check (#703 HARM-004)
  - apiGroups: ["kubernaut.ai"]
    resources: ["remediationrequests"]
    verbs: ["get", "list"]
  # Interactive mode: Lease-based session management (#703)
  # list required by ReconcileOrphanedLeases startup loop
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "create", "update", "delete"]
  # #1288: Impersonation RBAC removed — KA uses its own SA for all K8s API
  # calls. User identity is passed in MCP tool arguments for audit only.
  - apiGroups: ["alpha.kubernaut-test.ai"]
    resources: ["testwidgets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["beta.kubernaut-test.ai"]
    resources: ["testwidgets"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-agent-investigator-binding
  labels:
    app: kubernaut-agent
    component: investigation
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-agent-investigator
subjects:
  - kind: ServiceAccount
    name: kubernaut-agent-sa
    namespace: %s
`, namespace, namespace, namespace, namespace, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(rbacManifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply KA RBAC: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Kubernaut Agent RBAC created")
	return nil
}

// DeployKubernautAgentOnly deploys the Go Kubernaut Agent as a Deployment + NodePort Service.
// Port mapping: 30088 → 8443 (container), host 8088. KA defaults to 8443 since the H1 GA finding.
// enableJWT controls whether jwtProviders are included in the config (requires DEX to be deployed).
func DeployKubernautAgentOnly(clusterName, kubeconfigPath, namespace, imageTag string, enableJWT bool, writer io.Writer) error {
	imagePullPolicy := GetImagePullPolicy()

	// DD-TEST-007: Build GOCOVERDIR YAML snippets for binary coverage instrumentation
	covEnv := coverageEnvYAML("kubernautagent")
	covMount := coverageVolumeMountYAML()
	covVol := coverageVolumeYAML()

	jwtConfigSection := ""
	if enableJWT {
		jwtConfigSection = `      jwtProviders:
        - name: dex-e2e
          issuer: "http://dex:5556/dex"
          jwksURL: "http://dex:5556/dex/keys"
          audience: "kubernaut-agent"
          claimMappings:
            username: "email"
            groups: "groups"`
	}

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-agent-config
  namespace: %s
data:
  config.yaml: |
    runtime:
      logging:
        level: "debug"
      session:
        ttl: "5m"
        maxConcurrentInvestigations: 50
      server:
        tls:
          certDir: /etc/tls
        rateLimit:
          requestsPerSecond: 50
          burst: 100
      audit:
        flushIntervalSeconds: 0.1
        bufferSize: 10000
        batchSize: 50
    ai:
      llm:
        provider: "openai"
        apiKeyFile: "/etc/kubernautagent/llm-runtime/api-key"
      alignmentCheck:
        enabled: true
        timeout: "10s"
        maxStepTokens: 500
        llm:
          provider: "openai"
          model: "shadow-eval"
          endpoint: "http://mock-llm-shadow:8080"
          apiKeyFile: "/etc/kubernautagent/llm-runtime/shadow-api-key"
    integrations:
      dataStorage:
        url: "https://data-storage-service:8080"
      tools:
        prometheus:
          url: "http://prometheus-svc.%s.svc.cluster.local:9090"
        alertmanager:
          url: "http://alertmanager-svc.%s.svc.cluster.local:9093"
    interactive:
      enabled: true
      sessionTTL: "5m"
      inactivityTimeout: "2m"
      maxConcurrentSessions: 10
      rateLimitPerUser: 20
%s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-agent-llm-runtime
  namespace: %s
data:
  api-key: "mock-api-key-for-e2e"
  shadow-api-key: "mock-shadow-key"
  llm-runtime.yaml: |
    model: "mock-model"
    endpoint: "http://mock-llm:8080"
    temperature: 0.7
    maxRetries: 3
    timeoutSeconds: 120
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-agent
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernaut-agent
  template:
    metadata:
      labels:
        app: kubernaut-agent
    spec:
      serviceAccountName: kubernaut-agent-sa
      containers:
      - name: kubernaut-agent
        image: %s
        imagePullPolicy: %s
        ports:
        - name: https
          containerPort: 8443
        - name: health
          containerPort: 8081
        - name: metrics
          containerPort: 9090
        args:
        - "-config"
        - "/etc/kubernautagent/config.yaml"
        - "-llm-runtime"
        - "/etc/kubernautagent/llm-runtime/llm-runtime.yaml"
        env:
        - name: TLS_CA_FILE
          value: /etc/tls-ca/ca.crt
        - name: LLM_ENDPOINT
          value: "http://mock-llm:8080"
        - name: LLM_MODEL
          value: "mock-model"
        - name: LLM_PROVIDER
          value: "openai"
        - name: DATA_STORAGE_URL
          value: "https://data-storage-service:8080"
        %s
        volumeMounts:
        - name: config
          mountPath: /etc/kubernautagent
          readOnly: true
        - name: llm-runtime
          mountPath: /etc/kubernautagent/llm-runtime
          readOnly: true
        - name: tls-certs
          mountPath: /etc/tls
          readOnly: true
        - name: tls-ca
          mountPath: /etc/tls-ca
          readOnly: true
        %s
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: kubernaut-agent-config
      - name: llm-runtime
        configMap:
          name: kubernaut-agent-llm-runtime
      - name: tls-certs
        secret:
          secretName: kubernautagent-tls
          optional: true
      - name: tls-ca
        configMap:
          name: inter-service-ca
      %s
---
apiVersion: v1
kind: Service
metadata:
  name: kubernaut-agent
  namespace: %s
spec:
  type: NodePort
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    nodePort: 30088
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30188
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30988
  selector:
    app: kubernaut-agent
`, namespace, namespace, namespace, jwtConfigSection, namespace, namespace, imageTag, imagePullPolicy, covEnv, covMount, covVol, namespace)

	cmd := exec.Command("kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Kubernaut Agent: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Kubernaut Agent deployed")

	// Wait for pod readiness
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for Kubernaut Agent pod to be ready...")
	waitCmd := exec.Command("kubectl", "rollout", "status", "deployment/kubernaut-agent",
		"-n", namespace, "--kubeconfig", kubeconfigPath, "--timeout=120s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("kubernaut-agent deployment rollout failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Kubernaut Agent pod ready")
	return nil
}
