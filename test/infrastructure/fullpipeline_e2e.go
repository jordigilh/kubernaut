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
	"sync"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Full Pipeline E2E Infrastructure (Issue #39)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Deploys ALL Kubernaut services in a single Kind cluster to test the complete
// remediation lifecycle end-to-end:
//
//   Event → Gateway → RO → SP → AA → HAPI(MockLLM) → WE(Job) → Notification
//
// Services deployed (10):
//   1. PostgreSQL + Redis (infrastructure)
//   2. DataStorage (audit trail, workflow catalog)
//   3. AuthWebhook (SOC2 CC8.1 user attribution)
//   4. Gateway (HTTP ingress for alerts)
//   5. SignalProcessing (CRD controller)
//   6. RemediationOrchestrator (CRD controller)
//   7. AIAnalysis (CRD controller)
//   8. WorkflowExecution (CRD controller, Job engine)
//   9. Notification (CRD controller, file-based delivery)
//  10. HolmesGPT API + Mock LLM (AI service)
//
// Test infrastructure:
//   - kubernetes-event-exporter: watches K8s Events, POSTs to Gateway webhook
//   - memory-eater: target pod that triggers OOMKill events
//
// Port Allocation (DD-TEST-001 v2.7):
//   Gateway:     NodePort 30080 (event-exporter webhook delivery)
//   DataStorage: NodePort 30081 (workflow seeding + audit queries)
//   Mock LLM:    ClusterIP only (internal, accessed by HAPI)
//
// Image Build Strategy:
//   CI/CD mode (IMAGE_REGISTRY+IMAGE_TAG set): Skip build+load, Kind pulls on-demand
//   Local mode: Build 3 at a time (concurrency limit), load into Kind after cluster creation
//
// Kind Config: test/infrastructure/kind-fullpipeline-config.yaml
// Kubeconfig:  ~/.kube/fullpipeline-e2e-config
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// fullPipelineImageConfig defines all images required for the full pipeline E2E.
// Each entry maps to a BuildImageForKind call.
var fullPipelineImageConfigs = []E2EImageConfig{
	{ServiceName: "gateway", ImageName: "gateway", DockerfilePath: "docker/gateway-ubi9.Dockerfile"},
	{ServiceName: "signalprocessing", ImageName: "kubernaut/signalprocessing", DockerfilePath: "docker/signalprocessing-controller.Dockerfile"},
	{ServiceName: "remediationorchestrator", ImageName: "kubernaut/remediationorchestrator", DockerfilePath: "docker/remediationorchestrator-controller.Dockerfile"},
	{ServiceName: "aianalysis", ImageName: "kubernaut/aianalysis", DockerfilePath: "docker/aianalysis.Dockerfile"},
	{ServiceName: "workflowexecution", ImageName: "kubernaut/workflowexecution", DockerfilePath: "docker/workflowexecution-controller.Dockerfile"},
	{ServiceName: "notification", ImageName: "kubernaut/notification", DockerfilePath: "docker/notification-controller-ubi9.Dockerfile"},
	{ServiceName: "datastorage", ImageName: "kubernaut/datastorage", DockerfilePath: "docker/data-storage.Dockerfile"},
	{ServiceName: "authwebhook", ImageName: "authwebhook", DockerfilePath: "docker/authwebhook.Dockerfile"},
	{ServiceName: "holmesgpt-api", ImageName: "kubernaut/holmesgpt-api", DockerfilePath: "holmesgpt-api/Dockerfile"},
	{ServiceName: "mock-llm", ImageName: "kubernaut/mock-llm", DockerfilePath: "test/services/mock-llm/Dockerfile"},
}

// SetupFullPipelineInfrastructure deploys the complete Kubernaut service pipeline
// in a single Kind cluster for end-to-end remediation lifecycle testing.
//
// This is the AUTHORITATIVE setup function for the full pipeline E2E test suite.
// It composes existing per-service deployment helpers into a unified orchestration.
//
// Parameters:
//   - ctx: Context for cancellation
//   - clusterName: Kind cluster name (e.g., "fullpipeline-e2e")
//   - kubeconfigPath: Isolated kubeconfig path (e.g., ~/.kube/fullpipeline-e2e-config)
//   - writer: Output writer for progress logging
//
// Returns:
//   - builtImages: Map of service name → full image reference (for cleanup)
//   - error: First fatal error encountered
func SetupFullPipelineInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Full Pipeline E2E Infrastructure (Issue #39)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Pipeline: Event → Gateway → RO → SP → AA → HAPI → WE(Job) → Notification")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build (3 parallel) → Cluster → Load → Deploy → Seed → Verify")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001 v2.7: Gateway :30080, DataStorage :30081")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"
	projectRoot := getProjectRoot()
	startTime := time.Now()

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build all 10 images (3 at a time for local, skip for CI/CD)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building service images...")

	builtImages, err := buildFullPipelineImages(writer)
	if err != nil {
		return nil, fmt.Errorf("PHASE 1 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 1 complete: %d images ready (%s)\n",
		len(builtImages), time.Since(startTime).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🏗️  PHASE 2: Creating Kind cluster...")
	phase2Start := time.Now()

	// Coverage + notification file output mounts
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return builtImages, fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	extraMounts := []ExtraMount{}
	if os.Getenv("E2E_COVERAGE") == "true" {
		extraMounts = append(extraMounts, ExtraMount{
			HostPath:      coverdataPath,
			ContainerPath: "/coverdata",
			ReadOnly:      false,
		})
	}

	kindConfigPath := "test/infrastructure/kind-fullpipeline-config.yaml"
	if err := CreateKindClusterWithExtraMounts(
		clusterName, kubeconfigPath, kindConfigPath, extraMounts, writer,
	); err != nil {
		return builtImages, fmt.Errorf("PHASE 2 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 2 complete: Kind cluster ready (%s)\n",
		time.Since(phase2Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load locally-built images into Kind (skip for registry images)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	phase3Start := time.Now()

	if err := loadFullPipelineImages(builtImages, clusterName, writer); err != nil {
		return builtImages, fmt.Errorf("PHASE 3 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 3 complete: images loaded (%s)\n",
		time.Since(phase3Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Install ALL CRDs
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📋 PHASE 4: Installing CRDs...")
	phase4Start := time.Now()

	crdFiles := []string{
		"kubernaut.ai_remediationrequests.yaml",
		"kubernaut.ai_remediationapprovalrequests.yaml",
		"kubernaut.ai_signalprocessings.yaml",
		"kubernaut.ai_aianalyses.yaml",
		"kubernaut.ai_workflowexecutions.yaml",
		"kubernaut.ai_notificationrequests.yaml",
	}
	for _, crdFile := range crdFiles {
		crdPath := filepath.Join(projectRoot, "config/crd/bases", crdFile)
		_, _ = fmt.Fprintf(writer, "  ├── %s\n", crdFile)
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return builtImages, fmt.Errorf("failed to install CRD %s: %w", crdFile, err)
		}
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 4 complete: %d CRDs installed (%s)\n",
		len(crdFiles), time.Since(phase4Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Create namespace + RBAC foundation
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 5: Namespace + RBAC...")
	phase5Start := time.Now()

	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("failed to create namespace: %w", err)
	}

	// DD-AUTH-014: Deploy DataStorage client ClusterRole (required for all SAR checks)
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// Create DataStorage access RoleBindings for all services that need audit trail
	auditServices := []string{
		"data-storage-service",
		"remediationorchestrator-controller",
		"authwebhook",
		"notification-controller",
		"workflowexecution-controller",
		"holmesgpt-api-sa",
	}
	for _, sa := range auditServices {
		if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, sa, writer); err != nil {
			return builtImages, fmt.Errorf("failed to create RoleBinding for %s: %w", sa, err)
		}
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 5 complete (%s)\n", time.Since(phase5Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Deploy infrastructure (PostgreSQL, Redis, DataStorage, AuthWebhook)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 6: Infrastructure services...")
	phase6Start := time.Now()

	// 6a: PostgreSQL + Redis in parallel
	type deployResult struct {
		name string
		err  error
	}
	infraResults := make(chan deployResult, 2)
	go func() {
		infraResults <- deployResult{"PostgreSQL", deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	go func() {
		infraResults <- deployResult{"Redis", deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)}
	}()
	for i := 0; i < 2; i++ {
		r := <-infraResults
		if r.err != nil {
			return builtImages, fmt.Errorf("%s deployment failed: %w", r.name, r.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s ready\n", r.name)
	}

	// 6b: Run database migrations (needs PostgreSQL ready)
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("database migrations failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Migrations applied")

	// 6c: DataStorage RBAC + deployment (needs PostgreSQL + Redis)
	if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("DataStorage RBAC failed: %w", err)
	}
	dsImage := builtImages["datastorage"]
	if err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dsImage, 30081, writer); err != nil {
		return builtImages, fmt.Errorf("DataStorage deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ DataStorage deployed (NodePort 30081)")

	// 6d: AuthWebhook (SOC2 CC8.1)
	awImage := builtImages["authwebhook"]
	if err := DeployAuthWebhookManifestsOnly(ctx, clusterName, namespace, kubeconfigPath, awImage, writer); err != nil {
		return builtImages, fmt.Errorf("AuthWebhook deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ AuthWebhook deployed")

	_, _ = fmt.Fprintf(writer, "✅ PHASE 6 complete (%s)\n", time.Since(phase6Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7: Deploy AI services (Mock LLM + HAPI)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🤖 PHASE 7: AI services...")
	phase7Start := time.Now()

	// 7a: Deploy HAPI RBAC (ServiceAccount + RoleBinding for DataStorage access)
	if err := deployHAPIServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("HAPI RBAC failed: %w", err)
	}

	// 7b: Mock LLM (no workflow UUIDs yet — will be updated after workflow seeding)
	mockLLMImage := builtImages["mock-llm"]
	if err := deployMockLLMInNamespace(ctx, namespace, kubeconfigPath, mockLLMImage, nil, writer); err != nil {
		return builtImages, fmt.Errorf("Mock LLM deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Mock LLM deployed (ClusterIP)")

	// 7c: HAPI (connects to Mock LLM + DataStorage)
	hapiImage := builtImages["holmesgpt-api"]
	if err := deployHAPIOnly(clusterName, kubeconfigPath, namespace, hapiImage, writer); err != nil {
		return builtImages, fmt.Errorf("HAPI deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ HAPI deployed")

	_, _ = fmt.Fprintf(writer, "✅ PHASE 7 complete (%s)\n", time.Since(phase7Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 8: Deploy CRD controllers
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⚙️  PHASE 8: CRD controllers...")
	phase8Start := time.Now()

	// Deploy all controllers in parallel — they'll reconcile once CRDs and
	// dependencies are available (Kubernetes handles dependency ordering)
	controllerResults := make(chan deployResult, 5)

	go func() {
		err := deployFullPipelineSPController(ctx, namespace, kubeconfigPath, builtImages["signalprocessing"], writer)
		controllerResults <- deployResult{"SignalProcessing", err}
	}()
	go func() {
		err := DeployROCoverageManifest(kubeconfigPath, builtImages["remediationorchestrator"], writer)
		controllerResults <- deployResult{"RemediationOrchestrator", err}
	}()
	go func() {
		err := deployFullPipelineAAController(ctx, namespace, kubeconfigPath, builtImages["aianalysis"], writer)
		controllerResults <- deployResult{"AIAnalysis", err}
	}()
	go func() {
		err := DeployWorkflowExecutionController(ctx, namespace, kubeconfigPath, builtImages["workflowexecution"], writer)
		controllerResults <- deployResult{"WorkflowExecution", err}
	}()
	go func() {
		err := DeployNotificationController(ctx, namespace, kubeconfigPath, builtImages["notification"], writer)
		controllerResults <- deployResult{"Notification", err}
	}()

	var ctrlErrors []error
	for i := 0; i < 5; i++ {
		r := <-controllerResults
		if r.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
			ctrlErrors = append(ctrlErrors, fmt.Errorf("%s: %w", r.name, r.err))
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", r.name)
		}
	}
	if len(ctrlErrors) > 0 {
		return builtImages, fmt.Errorf("PHASE 8 controller deployments failed: %v", ctrlErrors)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 8 complete (%s)\n", time.Since(phase8Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 9: Deploy Gateway with NodePort
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🌐 PHASE 9: Gateway ingress...")
	phase9Start := time.Now()

	gwImage := builtImages["gateway"]
	if err := deployFullPipelineGateway(ctx, namespace, kubeconfigPath, gwImage, writer); err != nil {
		return builtImages, fmt.Errorf("Gateway deployment failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 9 complete: Gateway ready at NodePort 30080 (%s)\n",
		time.Since(phase9Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 10: Deploy test infrastructure (event-exporter + memory-eater)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🧪 PHASE 10: Test infrastructure...")
	phase10Start := time.Now()

	if err := deployKubernetesEventExporter(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("event-exporter deployment failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ kubernetes-event-exporter deployed")

	_, _ = fmt.Fprintf(writer, "✅ PHASE 10 complete (%s)\n", time.Since(phase10Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 11: Wait for all services ready
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 11: Waiting for all services ready...")
	phase11Start := time.Now()

	if err := waitForFullPipelineServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("PHASE 11 failed: services not ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 11 complete (%s)\n", time.Since(phase11Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// DONE
	// ═══════════════════════════════════════════════════════════════════════
	totalDuration := time.Since(startTime).Round(time.Second)
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Full Pipeline E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Total setup time: %s\n", totalDuration)
	_, _ = fmt.Fprintf(writer, "  🌐 Gateway:     http://localhost:30080\n")
	_, _ = fmt.Fprintf(writer, "  🗄️  DataStorage: http://localhost:30081\n")
	_, _ = fmt.Fprintf(writer, "  📦 Namespace:   %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  🔑 Kubeconfig:  %s\n", kubeconfigPath)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, nil
}

// ============================================================================
// PHASE 1: Image Building (3 at a time concurrency for local builds)
// ============================================================================

// buildFullPipelineImages builds all service images with a concurrency limit of 3
// for local builds. In CI/CD mode (IMAGE_REGISTRY+IMAGE_TAG set), BuildImageForKind
// returns the registry reference immediately without building.
func buildFullPipelineImages(writer io.Writer) (map[string]string, error) {
	// In CI/CD mode, all builds are instant (return registry refs)
	isCI := IsRunningInCICD()
	if isCI {
		_, _ = fmt.Fprintln(writer, "  🔄 CI/CD mode: using registry images (no local builds)")
	} else {
		_, _ = fmt.Fprintln(writer, "  🔨 Local mode: building 3 images at a time")
	}

	builtImages := make(map[string]string)
	var mu sync.Mutex
	var buildErrors []error

	// Semaphore for concurrency limit (only matters for local builds)
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	enableCoverage := os.Getenv("E2E_COVERAGE") == "true"

	for _, baseCfg := range fullPipelineImageConfigs {
		wg.Add(1)
		cfg := baseCfg // capture loop variable
		// HAPI doesn't support Go coverage instrumentation (Python service)
		if cfg.ServiceName != "holmesgpt-api" && cfg.ServiceName != "mock-llm" {
			cfg.EnableCoverage = enableCoverage
		}

		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot

			imageName, err := BuildImageForKind(cfg, writer)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", cfg.ServiceName, err)
				buildErrors = append(buildErrors, fmt.Errorf("%s: %w", cfg.ServiceName, err))
			} else {
				_, _ = fmt.Fprintf(writer, "  ✅ %s → %s\n", cfg.ServiceName, imageName)
				builtImages[cfg.ServiceName] = imageName
			}
		}()
	}
	wg.Wait()

	if len(buildErrors) > 0 {
		return builtImages, fmt.Errorf("image builds failed: %v", buildErrors)
	}
	return builtImages, nil
}

// ============================================================================
// PHASE 3: Image Loading (only for locally-built images)
// ============================================================================

// loadFullPipelineImages loads locally-built images into the Kind cluster.
// Skipped automatically for registry images (LoadImageToKind checks internally).
func loadFullPipelineImages(builtImages map[string]string, clusterName string, writer io.Writer) error {
	// LoadImageToKind already checks if the image is a registry image and skips.
	// We still iterate all images — the no-op is cheap.
	var loadErrors []error
	for serviceName, imageName := range builtImages {
		if err := LoadImageToKind(imageName, serviceName, clusterName, writer); err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s load failed: %v\n", serviceName, err)
			loadErrors = append(loadErrors, fmt.Errorf("%s: %w", serviceName, err))
		}
	}
	if len(loadErrors) > 0 {
		return fmt.Errorf("image loads failed: %v", loadErrors)
	}
	return nil
}

// ============================================================================
// PHASE 8: Controller Deployment Helpers
// ============================================================================

// deployFullPipelineSPController deploys the SignalProcessing controller with
// Rego policy ConfigMap for the full pipeline E2E.
func deployFullPipelineSPController(ctx context.Context, namespace, kubeconfigPath, imageName string, writer io.Writer) error {
	// Install Rego policy ConfigMap (required by SP)
	if err := createInlineRegoPolicyConfigMap(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Rego policy ConfigMap: %w", err)
	}

	// Deploy SP controller using coverage manifest (handles both coverage and non-coverage modes)
	if err := DeploySignalProcessingControllerWithCoverage(kubeconfigPath, imageName, writer); err != nil {
		return fmt.Errorf("failed to deploy SP controller: %w", err)
	}
	return nil
}

// deployFullPipelineAAController deploys the AIAnalysis controller with
// Rego policy and proper RBAC for the full pipeline E2E.
func deployFullPipelineAAController(ctx context.Context, namespace, kubeconfigPath, imageName string, writer io.Writer) error {
	// Deploy AA controller using the manifest helper
	if err := deployAIAnalysisControllerManifestOnly(kubeconfigPath, imageName, writer); err != nil {
		return fmt.Errorf("failed to deploy AA controller: %w", err)
	}
	return nil
}

// ============================================================================
// PHASE 9: Gateway Deployment (with NodePort 30080)
// ============================================================================

// deployFullPipelineGateway deploys the Gateway service with NodePort 30080
// for the full pipeline E2E. Uses a customized deployment manifest that routes
// to the correct DataStorage and Redis services within the cluster.
func deployFullPipelineGateway(ctx context.Context, namespace, kubeconfigPath, gatewayImageName string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	// Read the standard Gateway deployment manifest
	deploymentPath := filepath.Join(projectRoot, "test/e2e/gateway/gateway-deployment.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return fmt.Errorf("failed to read Gateway deployment: %w", err)
	}

	// Replace image name and pull policy
	updatedContent := strings.ReplaceAll(string(deploymentContent),
		"localhost/kubernaut-gateway:e2e-test", gatewayImageName)
	updatedContent = strings.ReplaceAll(updatedContent,
		"imagePullPolicy: Never",
		fmt.Sprintf("imagePullPolicy: %s", GetImagePullPolicy()))

	// Replace the NodePort to use 30080 (DD-TEST-001 v2.7: full pipeline allocation)
	// The gateway-deployment.yaml may use a different NodePort for the gateway-only E2E
	updatedContent = strings.ReplaceAll(updatedContent,
		"nodePort: 30088", "nodePort: 30080")

	// Write temporary modified manifest
	tmpDeployment := filepath.Join(os.TempDir(), "fullpipeline-gateway-deployment.yaml")
	if err := os.WriteFile(tmpDeployment, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp deployment: %w", err)
	}
	defer func() { _ = os.Remove(tmpDeployment) }()

	// Apply
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", tmpDeployment, "-n", namespace)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Gateway deployment failed: %w", err)
	}

	// Wait for Gateway pod ready
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for Gateway pod ready...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=gateway", "-n", namespace, "--timeout=300s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Gateway pod not ready: %w", err)
	}

	return nil
}

// ============================================================================
// PHASE 10: Test Infrastructure (event-exporter + memory-eater)
// ============================================================================

// deployKubernetesEventExporter deploys the kubernetes-event-exporter that watches
// K8s Events and forwards them to the Gateway webhook endpoint.
//
// The event-exporter:
//   - Watches for Warning events (OOMKilled, CrashLoopBackOff, etc.)
//   - POSTs to Gateway's /api/v1/alerts/kubernetes-events endpoint
//   - Runs in kubernaut-system namespace with RBAC for cluster-wide event watching
//
// Image: ghcr.io/resmoio/kubernetes-event-exporter:latest
// (No local build needed — pulled directly by Kind's containerd)
func deployKubernetesEventExporter(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  📡 Deploying kubernetes-event-exporter...")

	manifest := fmt.Sprintf(`---
# ServiceAccount for event-exporter
apiVersion: v1
kind: ServiceAccount
metadata:
  name: event-exporter
  namespace: %[1]s
---
# ClusterRole: read events cluster-wide
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: event-exporter
rules:
- apiGroups: [""]
  resources: ["events"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get"]
---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: event-exporter
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: event-exporter
subjects:
- kind: ServiceAccount
  name: event-exporter
  namespace: %[1]s
---
# ConfigMap: route Warning events to Gateway
apiVersion: v1
kind: ConfigMap
metadata:
  name: event-exporter-config
  namespace: %[1]s
data:
  config.yaml: |
    logLevel: info
    logFormat: json
    route:
      routes:
        - match:
            - receiver: gateway-webhook
          drop:
            - type: "Normal"
    receivers:
      - name: gateway-webhook
        webhook:
          endpoint: "http://gateway-service.%[1]s.svc.cluster.local:8080/api/v1/alerts/kubernetes-events"
          headers:
            Content-Type: application/json
---
# Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: event-exporter
  namespace: %[1]s
  labels:
    app: event-exporter
    component: test-infrastructure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: event-exporter
  template:
    metadata:
      labels:
        app: event-exporter
        component: test-infrastructure
    spec:
      serviceAccountName: event-exporter
      containers:
      - name: event-exporter
        image: ghcr.io/resmoio/kubernetes-event-exporter:latest
        imagePullPolicy: IfNotPresent
        args:
        - -conf=/config/config.yaml
        volumeMounts:
        - name: config
          mountPath: /config
          readOnly: true
        resources:
          requests:
            memory: "32Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "200m"
      volumes:
      - name: config
        configMap:
          name: event-exporter-config
`, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// DeployMemoryEater deploys a memory-eater pod in the target namespace that will
// trigger an OOMKill event. The event-exporter picks up this event and forwards
// it to Gateway, starting the full remediation pipeline.
//
// Image: us-central1-docker.pkg.dev/genuine-flight-317411/devel/memory-eater:1.0
//
// Parameters:
//   - targetNamespace: Namespace with kubernaut.ai/managed=true label
//   - kubeconfigPath: Path to kubeconfig
//   - writer: Output writer for progress logging
func DeployMemoryEater(ctx context.Context, targetNamespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🐛 Deploying memory-eater in namespace %s...\n", targetNamespace)

	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: memory-eater
  namespace: %s
  labels:
    app: memory-eater
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: memory-eater
  template:
    metadata:
      labels:
        app: memory-eater
        kubernaut.ai/managed: "true"
    spec:
      containers:
      - name: memory-eater
        image: us-central1-docker.pkg.dev/genuine-flight-317411/devel/memory-eater:1.0
        imagePullPolicy: IfNotPresent
        args:
        - "--start-mb=80"
        - "--increment-mb=10"
        - "--interval-sec=1"
        resources:
          limits:
            memory: "100Mi"
          requests:
            memory: "50Mi"
`, targetNamespace)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// ============================================================================
// PHASE 11: Service Readiness Checks
// ============================================================================

// waitForFullPipelineServicesReady waits for all services to be ready in the cluster.
func waitForFullPipelineServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// List of deployments that must be ready
	deployments := []string{
		"data-storage-service",
		"mock-llm",
		"holmesgpt-api",
		"gateway",
		"event-exporter",
	}

	// Check each deployment
	for _, deplName := range deployments {
		_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s...\n", deplName)
		Eventually(func() bool {
			depl, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deplName, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return depl.Status.ReadyReplicas >= 1
		}, 3*time.Minute, 5*time.Second).Should(BeTrue(),
			fmt.Sprintf("%s should become ready", deplName))
		_, _ = fmt.Fprintf(writer, "  ✅ %s ready\n", deplName)
	}

	// Check controller pods by label (they may have different deployment names)
	controllerLabels := map[string]string{
		"SignalProcessing":        "app=signalprocessing-controller",
		"RemediationOrchestrator": "app=remediationorchestrator-controller",
		"AIAnalysis":              "app=aianalysis-controller",
		"WorkflowExecution":       "app=workflowexecution-controller",
		"Notification":            "app=notification-controller",
	}

	for name, selector := range controllerLabels {
		_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s controller...\n", name)
		Eventually(func() bool {
			pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil || len(pods.Items) == 0 {
				return false
			}
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					for _, c := range pod.Status.Conditions {
						if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
							return true
						}
					}
				}
			}
			return false
		}, 3*time.Minute, 5*time.Second).Should(BeTrue(),
			fmt.Sprintf("%s controller should become ready", name))
		_, _ = fmt.Fprintf(writer, "  ✅ %s controller ready\n", name)
	}

	return nil
}
