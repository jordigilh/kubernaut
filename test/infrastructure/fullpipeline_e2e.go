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
//   Event → Gateway → RO → SP → AA → HAPI(MockLLM) → WE(Job) → Notification → EA → EM
//
// Services deployed (13):
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
//  11. Prometheus (metric comparison for EM, ADR-EM-001)
//  12. AlertManager (alert resolution for EM, ADR-EM-001)
//  13. EffectivenessMonitor (CRD controller, watches EA CRDs)
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

// skipMockLLM returns true when the SKIP_MOCK_LLM environment variable is set
// to any non-empty value. When true, the Mock LLM service is NOT built, deployed,
// or checked for readiness. Use this for local development where HAPI connects
// to a real LLM (e.g., Vertex AI). CI/CD pipelines leave this unset so Mock LLM
// provides a fully self-contained test environment.
func skipMockLLM() bool {
	return os.Getenv("SKIP_MOCK_LLM") != ""
}

// getEnvOrDefault returns the value of the environment variable named by key,
// or fallback if the variable is unset or empty.
func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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
	{ServiceName: "mock-llm", ImageName: "kubernaut/mock-llm", DockerfilePath: "test/services/mock-llm/Dockerfile", BuildContextPath: "test/services/mock-llm"},
	{ServiceName: "effectivenessmonitor", ImageName: "kubernaut/effectivenessmonitor", DockerfilePath: "docker/effectivenessmonitor-controller.Dockerfile"},
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
		"kubernaut.ai_effectivenessassessments.yaml", // ADR-EM-001: EA CRD for EM
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
	// NOTE: Every ServiceAccount that writes audit events to DataStorage MUST be listed here.
	// Missing entries cause HTTP 403 from DataStorage's SAR check, silently dropping audit data.
	auditServices := []string{
		"data-storage-service",
		"gateway",
		"remediationorchestrator-controller",
		"authwebhook",
		"workflowexecution-controller",
		"holmesgpt-api-sa",
		"effectivenessmonitor-controller", // ADR-EM-001: EM needs DataStorage audit access
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
	// PHASE 7: Parallel service deployment (Wave A + Wave B)
	//
	// After DataStorage is ready, deploy everything that only depends on DS
	// in parallel (Wave A). Then deploy services that depend on Wave A outputs
	// (Wave B: HAPI → MockLLM, EM → Prometheus+AM, event-exporter → Gateway).
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🚀 PHASE 7: Parallel service deployment (Wave A + Wave B)...")
	phase7Start := time.Now()

	// Synchronization channels: Wave B services wait on these before deploying.
	mockLLMReady := make(chan struct{})
	promAMReady := make(chan struct{})
	gatewayReady := make(chan struct{})

	// Collect all errors from both waves via a single channel.
	type waveResult struct {
		name string
		err  error
	}
	// Wave A: 5 controllers + Gateway + HAPI RBAC + MockLLM + Prometheus + AlertManager = up to 11
	// Wave B: HAPI + EM + event-exporter = 3
	// Total capacity = 14 (generous upper bound)
	allResults := make(chan waveResult, 16)

	// ── Wave A: Deploy in parallel (no inter-dependency beyond DataStorage) ──
	_, _ = fmt.Fprintln(writer, "  Wave A: deploying services in parallel...")

	// A1: HAPI RBAC (prerequisite for HAPI, fast kubectl apply)
	go func() {
		allResults <- waveResult{"HAPI-RBAC", deployHAPIServiceRBAC(ctx, namespace, kubeconfigPath, writer)}
	}()

	// A2: Mock LLM (HAPI depends on this)
	go func() {
		defer close(mockLLMReady)
		if skipMockLLM() {
			_, _ = fmt.Fprintln(writer, "  ⏭️  Mock LLM skipped (SKIP_MOCK_LLM is set)")
			allResults <- waveResult{"MockLLM", nil}
			return
		}
		err := deployMockLLMInNamespace(ctx, namespace, kubeconfigPath, builtImages["mock-llm"], nil, writer)
		allResults <- waveResult{"MockLLM", err}
	}()

	// A3: Prometheus + AlertManager (EM depends on these)
	go func() {
		defer close(promAMReady)
		var promErr, amErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			promErr = DeployPrometheus(ctx, namespace, kubeconfigPath, writer)
		}()
		go func() {
			defer wg.Done()
			amErr = DeployAlertManager(ctx, namespace, kubeconfigPath, writer)
		}()
		wg.Wait()
		if promErr != nil {
			allResults <- waveResult{"Prometheus", promErr}
			return
		}
		if amErr != nil {
			allResults <- waveResult{"AlertManager", amErr}
			return
		}
		allResults <- waveResult{"Prometheus+AlertManager", nil}
	}()

	// A4: 5 CRD controllers (no Prometheus/AM dependency)
	for _, ctrl := range []struct {
		name    string
		deployF func() error
	}{
		{"SignalProcessing", func() error {
			return deployFullPipelineSPController(ctx, namespace, kubeconfigPath, builtImages["signalprocessing"], writer)
		}},
		{"RemediationOrchestrator", func() error {
			return DeployROCoverageManifest(kubeconfigPath, builtImages["remediationorchestrator"], writer)
		}},
		{"AIAnalysis", func() error {
			return deployFullPipelineAAController(ctx, namespace, kubeconfigPath, builtImages["aianalysis"], writer)
		}},
		{"WorkflowExecution", func() error {
			return DeployWorkflowExecutionController(ctx, namespace, kubeconfigPath, builtImages["workflowexecution"], writer)
		}},
		{"Notification", func() error {
			return DeployNotificationController(ctx, namespace, kubeconfigPath, builtImages["notification"], writer)
		}},
	} {
		ctrl := ctrl // capture
		go func() {
			allResults <- waveResult{ctrl.name, ctrl.deployF()}
		}()
	}

	// A5: Gateway (event-exporter depends on this)
	go func() {
		defer close(gatewayReady)
		err := deployFullPipelineGateway(ctx, namespace, kubeconfigPath, builtImages["gateway"], writer)
		allResults <- waveResult{"Gateway", err}
	}()

	// ── Wave B: Deploy after specific Wave A dependencies are ready ──

	// B1: HAPI — wait for Mock LLM
	go func() {
		<-mockLLMReady
		err := deployHAPIOnly(clusterName, kubeconfigPath, namespace, builtImages["holmesgpt-api"], writer)
		allResults <- waveResult{"HAPI", err}
	}()

	// B2: EM controller — wait for Prometheus + AlertManager
	go func() {
		<-promAMReady
		err := DeployEMController(ctx, namespace, kubeconfigPath, builtImages["effectivenessmonitor"], writer)
		allResults <- waveResult{"EffectivenessMonitor", err}
	}()

	// B3: event-exporter — wait for Gateway
	go func() {
		<-gatewayReady
		err := deployKubernetesEventExporter(ctx, namespace, kubeconfigPath, writer)
		allResults <- waveResult{"event-exporter", err}
	}()

	// ── Collect all results ──
	// Wave A: HAPI-RBAC + MockLLM + Prom+AM(1) + 5 controllers + Gateway = 9
	// Wave B: HAPI + EM + event-exporter = 3
	expectedResults := 12
	var deployErrors []error
	for i := 0; i < expectedResults; i++ {
		r := <-allResults
		if r.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
			deployErrors = append(deployErrors, fmt.Errorf("%s: %w", r.name, r.err))
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", r.name)
		}
	}
	if len(deployErrors) > 0 {
		return builtImages, fmt.Errorf("PHASE 7 deployments failed: %v", deployErrors)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 7 complete (%s)\n", time.Since(phase7Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 8: Wait for all services ready (parallel readiness checks)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 8: Waiting for all services ready...")
	phase8Start := time.Now()

	if err := waitForFullPipelineServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, fmt.Errorf("PHASE 8 failed: services not ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 8 complete (%s)\n", time.Since(phase8Start).Round(time.Second))

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
		// Skip mock-llm build when SKIP_MOCK_LLM is set (local dev with real LLM)
		if baseCfg.ServiceName == "mock-llm" && skipMockLLM() {
			_, _ = fmt.Fprintln(writer, "  ⏭️  Skipping mock-llm build (SKIP_MOCK_LLM is set)")
			continue
		}
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
	// Install all SP-specific Rego policy ConfigMaps and predictive signal mappings
	// (5 policies + 1 predictive mapping ConfigMap required by SP controller)
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy SP policies: %w", err)
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
	// Install AA-specific Rego policy ConfigMap (aianalysis-policies)
	if err := createInlineRegoPolicyConfigMap(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create AA Rego policy ConfigMap: %w", err)
	}

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
// deployFullPipelineGateway deploys Gateway using the unified inline YAML template.
// Standardized: uses gatewayManifest() instead of reading static YAML files.
func deployFullPipelineGateway(ctx context.Context, namespace, kubeconfigPath, gatewayImageName string, writer io.Writer) error {
	manifest := gatewayManifest(gatewayImageName, false)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Gateway deployment failed: %w", err)
	}

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
  resources: ["pods", "configmaps", "namespaces"]
  verbs: ["get", "list"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list"]
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
# ConfigMap: route Warning events from fp-e2e-* namespaces only to Gateway
# fp-am-* namespaces are intentionally excluded: the AlertManager E2E test uses
# Prometheus alerts (not K8s events) as signal source to prevent duplication.
apiVersion: v1
kind: ConfigMap
metadata:
  name: event-exporter-config
  namespace: %[1]s
data:
  config.yaml: |
    logLevel: debug
    logFormat: json
    maxEventAgeSeconds: 300
    kubeQPS: 50
    kubeBurst: 100
    route:
      routes:
        # Only forward K8s events from fp-e2e-* namespaces (K8s event signal source test)
        # fp-am-* namespaces excluded to prevent K8s event duplication in AlertManager test
        - match:
            - namespace: "fp-e2e-*"
              receiver: gateway-webhook
          drop:
            - type: "Normal"
    receivers:
      - name: gateway-webhook
        webhook:
          endpoint: "http://gateway-service.%[1]s.svc.cluster.local:8080/api/v1/signals/kubernetes-event"
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
        # Positional args: initial_memory initial_duration target_memory target_duration hold_duration
        # Consumes 40Mi initially for 1s, then grows to 60Mi over 1s, then exits (hold=0)
        # Limit deliberately lower than target+runtime to trigger OOMKill (exit 137)
        # The remediation workflow fixes this by increasing the memory limit
        args: ["40Mi", "1", "60Mi", "1", "0"]
        resources:
          limits:
            memory: "50Mi"
          requests:
            memory: "20Mi"
`, targetNamespace)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// DeployMemoryEaterHighUsage deploys a memory-eater pod that runs at high memory
// usage (>=90% of limit) WITHOUT triggering OOMKill. This is used by the AlertManager
// E2E test where the signal source is a Prometheus MemoryExceedsLimit alert, not a K8s event.
//
// The pod consumes 92Mi with a 100Mi limit (92% usage), staying above the 90% threshold
// in the Prometheus alert rule while remaining within the OOMKill boundary.
// Hold duration is 300s, giving Prometheus ample time to scrape and alert to fire.
func DeployMemoryEaterHighUsage(ctx context.Context, targetNamespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🐛 Deploying memory-eater (high usage, no OOMKill) in namespace %s...\n", targetNamespace)

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
        # Positional args: initial_memory initial_duration target_memory target_duration hold_duration
        # Consumes 50Mi initially for 2s, then grows to 92Mi over 5s, then holds for 300s.
        # With 100Mi limit, 92Mi = 92%% usage — above the 90%% Prometheus alert threshold
        # but safely below OOMKill. This gives Prometheus time to scrape and fire the alert.
        args: ["50Mi", "2", "92Mi", "5", "300"]
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
// All readiness checks run in parallel for faster convergence.
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
		"datastorage",
		"holmesgpt-api",
		"gateway",
		"event-exporter",
		"prometheus",   // ADR-EM-001: Prometheus for EM metric comparison
		"alertmanager", // ADR-EM-001: AlertManager for EM alert resolution
	}
	if !skipMockLLM() {
		deployments = append(deployments, "mock-llm")
	}

	// Controller pods checked by label (may have different deployment names)
	type controllerCheck struct {
		name     string
		selector string
	}
	controllers := []controllerCheck{
		{"SignalProcessing", "app=signalprocessing-controller"},
		{"RemediationOrchestrator", "app=remediationorchestrator-controller"},
		{"AIAnalysis", "app=aianalysis-controller"},
		{"WorkflowExecution", "app=workflowexecution-controller"},
		{"Notification", "app=notification-controller"},
		{"EffectivenessMonitor", "app=effectivenessmonitor-controller"}, // ADR-EM-001
	}

	// Run all checks in parallel
	type readyResult struct {
		name string
		err  error
	}
	totalChecks := len(deployments) + len(controllers)
	results := make(chan readyResult, totalChecks)

	// Deployment readiness checks
	for _, deplName := range deployments {
		deplName := deplName // capture
		go func() {
			_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s...\n", deplName)
			pollErr := pollUntilReady(ctx, 3*time.Minute, 5*time.Second, func() bool {
				depl, getErr := clientset.AppsV1().Deployments(namespace).Get(ctx, deplName, metav1.GetOptions{})
				if getErr != nil {
					return false
				}
				return depl.Status.ReadyReplicas >= 1
			})
			if pollErr != nil {
				results <- readyResult{deplName, fmt.Errorf("%s not ready after 3m", deplName)}
			} else {
				_, _ = fmt.Fprintf(writer, "  ✅ %s ready\n", deplName)
				results <- readyResult{deplName, nil}
			}
		}()
	}

	// Controller readiness checks
	for _, ctrl := range controllers {
		ctrl := ctrl // capture
		go func() {
			_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s controller...\n", ctrl.name)
			pollErr := pollUntilReady(ctx, 3*time.Minute, 5*time.Second, func() bool {
				pods, listErr := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
					LabelSelector: ctrl.selector,
				})
				if listErr != nil || len(pods.Items) == 0 {
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
			})
			if pollErr != nil {
				results <- readyResult{ctrl.name, fmt.Errorf("%s controller not ready after 3m", ctrl.name)}
			} else {
				_, _ = fmt.Fprintf(writer, "  ✅ %s controller ready\n", ctrl.name)
				results <- readyResult{ctrl.name, nil}
			}
		}()
	}

	// Collect all results
	var readyErrors []error
	for i := 0; i < totalChecks; i++ {
		r := <-results
		if r.err != nil {
			readyErrors = append(readyErrors, r.err)
		}
	}
	if len(readyErrors) > 0 {
		return fmt.Errorf("services not ready: %v", readyErrors)
	}

	return nil
}

// pollUntilReady polls condFn at the given interval until it returns true or
// the timeout expires. Unlike Gomega Eventually, this can be used outside a
// Ginkgo test context (e.g., from SynchronizedBeforeSuite Process 1).
func pollUntilReady(ctx context.Context, timeout, interval time.Duration, condFn func() bool) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Check once immediately
	if condFn() {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out after %s", timeout)
		case <-ticker.C:
			if condFn() {
				return nil
			}
		}
	}
}
