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

	"github.com/google/uuid"
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
//   Event → Gateway → RO → SP → AA → KA(MockLLM) → WE(Job) → Notification → EA → EM
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
//  10. KA + Mock LLM (AI service)
//  11. Prometheus (metric comparison for EM, ADR-EM-001)
//  12. AlertManager (alert resolution for EM, ADR-EM-001)
//  13. EffectivenessMonitor (CRD controller, watches EA CRDs)
//
// Test infrastructure:
//   - kubernetes-event-exporter: watches K8s Events, POSTs to Gateway webhook
//   - memory-eater: target pod that triggers OOMKill events
//
// Port Allocation (DD-TEST-001 v2.8, Issue #1737):
//   Gateway:      hostPort 30080, chart-pinned NodePort 30080 (host-side signal POSTs, e.g. af_helpers_test.go)
//   DataStorage:  hostPort 30081, chart-pinned NodePort 30081 (workflow seeding + audit queries)
//   API Frontend: hostPort 30443, chart-pinned NodePort 30443 (AF E2E HTTP client)
//   All three are pinned via gateway/datastorage/apifrontend.service.{type,nodePort}
//   (default-off, production-safe chart knob) + networkPolicies.<svc>.ingressCIDRs
//   ipBlock rules to admit the NodePort-sourced (SNAT'd to node IP) traffic
//   through the default-deny NetworkPolicy. Host reachability additionally
//   requires the exact port to be pre-mapped in kind-fullpipeline-config.yaml's
//   extraPortMappings (fixed at cluster-creation time, before `helm install`
//   runs) -- an auto-assigned NodePort cannot be made host-reachable this way.
//   Mock LLM:     ClusterIP only (internal, accessed by Kubernaut Agent)
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
// or checked for readiness. Use this for local development where Kubernaut Agent connects
// to a real LLM (e.g., Vertex AI). CI/CD pipelines leave this unset so Mock LLM
// provides a fully self-contained test environment.
func skipMockLLM() bool {
	return os.Getenv("SKIP_MOCK_LLM") != ""
}

// fullPipelineImageConfig defines all images required for the full pipeline E2E.
// Each entry maps to a BuildImageForKind call.
var fullPipelineImageConfigs = []E2EImageConfig{
	{ServiceName: "gateway", ImageName: "gateway", DockerfilePath: "docker/gateway.Dockerfile"},
	{ServiceName: "signalprocessing", ImageName: "kubernaut/signalprocessing", DockerfilePath: "docker/signalprocessing-controller.Dockerfile"},
	{ServiceName: "remediationorchestrator", ImageName: "kubernaut/remediationorchestrator", DockerfilePath: "docker/remediationorchestrator-controller.Dockerfile"},
	{ServiceName: "aianalysis", ImageName: "kubernaut/aianalysis", DockerfilePath: "docker/aianalysis.Dockerfile"},
	{ServiceName: "workflowexecution", ImageName: "kubernaut/workflowexecution", DockerfilePath: "docker/workflowexecution-controller.Dockerfile"},
	{ServiceName: "notification", ImageName: "kubernaut/notification", DockerfilePath: "docker/notification-controller.Dockerfile"},
	{ServiceName: "datastorage", ImageName: "kubernaut/datastorage", DockerfilePath: "docker/data-storage.Dockerfile"},
	{ServiceName: "authwebhook", ImageName: "authwebhook", DockerfilePath: "docker/authwebhook.Dockerfile"},
	{ServiceName: "kubernautagent", ImageName: "kubernaut/kubernautagent", DockerfilePath: "docker/kubernautagent.Dockerfile"},
	{ServiceName: "mock-llm", ImageName: "kubernaut/mock-llm", DockerfilePath: "test/services/mock-llm/go.Dockerfile", BuildContextPath: ""},
	{ServiceName: "effectivenessmonitor", ImageName: "kubernaut/effectivenessmonitor", DockerfilePath: "docker/effectivenessmonitor-controller.Dockerfile"},
	{ServiceName: "apifrontend", ImageName: "kubernaut/apifrontend", DockerfilePath: "docker/apifrontend.Dockerfile"},
	{ServiceName: "fleetmetadatacache", ImageName: "kubernaut/fleetmetadatacache", DockerfilePath: "docker/fleetmetadatacache.Dockerfile"},
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
//   - seededUUIDs: Map of "workflow_name:environment" → UUID (seeded in Phase 6b)
//   - error: First fatal error encountered
func SetupFullPipelineInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (builtImages map[string]string, seededUUIDs map[string]string, afRemediateNS map[string]string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Full Pipeline E2E Infrastructure (Issue #39)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Pipeline: Event → Gateway → RO → SP → AA → KA → WE(Job) → Notification")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build (3 parallel) → Cluster → Load → Deploy → Seed → Verify")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001 v2.8: Gateway/DataStorage/AF via chart-pinned NodePort + ipBlock")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := kubernautSystem
	projectRoot := getProjectRoot()
	startTime := time.Now()

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build all 10 images (3 at a time for local, skip for CI/CD)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building service images...")

	var buildErr error
	builtImages, buildErr = buildFullPipelineImages(ctx, writer)
	if buildErr != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 1 failed: %w", buildErr)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 1 complete: %d images ready (%s)\n",
		len(builtImages), time.Since(startTime).Round(time.Second))

	// Issue #1737: normalize the 12 chart-managed images to one shared
	// localhost/<service>:<tag> reference for the chart's global.image.tag
	// (buildFullPipelineImages may hand back per-service registry refs,
	// a shared CI-artifact tag, or per-service random local-build tags --
	// this makes the downstream `helm install` image resolution uniform
	// regardless of source). mock-llm is untouched (stays Go-managed).
	chartImageRegistry, chartImageTag, retagErr := ensureSharedChartImageTag(ctx, builtImages, writer)
	if retagErr != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 1: image retag for Helm failed: %w", retagErr)
	}

	// db-migrate (charts/kubernaut/templates/hooks/migration-job.yaml) is a
	// standalone infra image, not one of fullPipelineImageConfigs -- build/
	// reuse it separately and fold it into builtImages so PHASE 3's
	// loadFullPipelineImages picks it up automatically (Issue #1737).
	dbMigrateImage, dbMigrateErr := ensureDBMigrateImage(ctx, chartImageTag, writer)
	if dbMigrateErr != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 1: db-migrate image failed: %w", dbMigrateErr)
	}
	builtImages["db-migrate"] = dbMigrateImage

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🏗️  PHASE 2: Creating Kind cluster...")
	phase2Start := time.Now()

	// Coverage + notification file output mounts
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return builtImages, nil, nil, fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	extraMounts := []ExtraMount{}
	if os.Getenv("E2E_COVERAGE") == trueFixture {
		extraMounts = append(extraMounts, ExtraMount{
			HostPath:      coverdataPath,
			ContainerPath: "/coverdata",
			ReadOnly:      false,
		})
	}

	kindConfigPath := "test/infrastructure/kind-fullpipeline-config.yaml"
	if err := CreateKindClusterWithExtraMounts(ctx,
		clusterName, kubeconfigPath, kindConfigPath, extraMounts, writer,
	); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 2 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 2 complete: Kind cluster ready (%s)\n",
		time.Since(phase2Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load locally-built images into Kind (skip for registry images)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	phase3Start := time.Now()

	// Issue #1737: always load now -- ensureSharedChartImageTag always produces
	// fresh localhost/*:<tag> references (regardless of the original source
	// mode), so LoadImageToKind must actually load them every time. mock-llm
	// follows its own pre-existing image ref, handled the same as before.
	if err := loadFullPipelineImages(ctx, builtImages, clusterName, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 3 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 3 complete: images loaded (%s)\n",
		time.Since(phase3Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: (removed, Issue #1737) -- charts/kubernaut/crds/ is installed
	// automatically by `helm install` in PHASE 6 below. Manual `kubectl apply`
	// of config/crd/bases/*.yaml is no longer needed for the chart-managed
	// services. investigationsessions CRD (chart-only, unused by FullPipeline)
	// is a harmless extra the chart also installs.
	// ═══════════════════════════════════════════════════════════════════════

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Namespace + Helm chart prerequisite Secrets
	//
	// Issue #1737: DataStorage client ClusterRole/RoleBindings, inter-service
	// TLS (Issue #785), and the AU-9 signing certificate are now provisioned
	// by the Helm chart itself (templates/rbac/datastorage-rbac.yaml,
	// templates/interservice/{ca,leaf-certs}.yaml, templates/hooks/tls-cert-job.yaml)
	// -- removed here to avoid duplicating/conflicting with chart-owned resources.
	// The chart does NOT generate credential Secrets (by design, original #239
	// audit finding #4), so those remain Go-managed.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 5: Namespace + Helm prerequisite Secrets...")
	phase5Start := time.Now()

	if err := createTestNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("failed to create namespace: %w", err)
	}

	if err := createFullPipelineHelmSecrets(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("failed to create Helm chart secrets: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 5 complete (%s)\n", time.Since(phase5Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: helm install charts/kubernaut (Issue #1737)
	//
	// Replaces the manual PostgreSQL + Redis + DB migrations + DataStorage +
	// AuthWebhook deployment (previously 6a-6c here) with a single Helm
	// install that also covers every chart-managed service deployed in the
	// old PHASE 7 below (Gateway, SignalProcessing, RemediationOrchestrator,
	// AIAnalysis, WorkflowExecution, Notification, KubernautAgent,
	// EffectivenessMonitor, APIFrontend, FleetMetadataCache). Mock-Slack stays
	// Go-managed test infrastructure (not part of the production chart).
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 6: helm install charts/kubernaut...")
	phase6Start := time.Now()

	type deployResult struct {
		name string
		err  error
	}
	phase6Results := make(chan deployResult, 2)
	go func() {
		phase6Results <- deployResult{"HelmChart",
			InstallFullPipelineHelmChart(ctx, kubeconfigPath, namespace, chartImageRegistry, chartImageTag, writer)}
	}()
	go func() {
		phase6Results <- deployResult{"Mock-Slack",
			deployMockSlack(ctx, namespace, kubeconfigPath, writer)}
	}()
	for i := 0; i < 2; i++ {
		r := <-phase6Results
		if r.err != nil {
			return builtImages, nil, nil, fmt.Errorf("%s deployment failed: %w", r.name, r.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", r.name)
	}

	// dex-tls (Issue #1737): the chart's pre-install tls-cert-gen hook has
	// already run by this point (helm install above blocks on hooks), so
	// authwebhook-tls's embedded ca.crt/ca.key are available to sign DEX's
	// leaf cert. Must happen before PHASE 7's deployDexOIDCProviderForAF.
	if err := ensureDexTLSFromChartCA(ctx, kubeconfigPath, namespace, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6: dex-tls provisioning failed: %w", err)
	}

	// Issue #1737: host-side E2E clients hit DataStorage, APIFrontend, and
	// KubernautAgent via their chart-pinned NodePorts ("https://localhost:30081",
	// "https://localhost:30443", "https://localhost:8088") with full hostname
	// verification -- the chart's own tls-cert-job.yaml hook omits
	// "localhost"/127.0.0.1 from these SANs (correct for real clusters, which
	// never use localhost), so those leaf certs must be re-signed here for
	// host access to work at all (confirmed via "connection reset by peer" on
	// the audit API during validation).
	if err := resignHostAccessedTLSCertsWithLocalhostSAN(ctx, kubeconfigPath, namespace, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6: gateway/datastorage/apifrontend/kubernautagent TLS re-sign failed: %w", err)
	}

	// Issue #785 + #1737: suite_test.go's NewTLSAwareTransport (host-side E2E
	// HTTP client) reads the inter-service CA from a local file that the
	// Go-native GenerateInterServiceTLS used to write as a side effect of
	// generating the CA itself; the chart generates the CA in-cluster now, so
	// fetch it back out to the same expected path.
	if err := writeInterServiceCAPEMFromCluster(ctx, kubeconfigPath, namespace, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6: failed to write inter-service CA PEM: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 6 complete (%s)\n", time.Since(phase6Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6b: Seed workflow catalog (must complete BEFORE KA deployment)
	//
	// KA's buildWorkflowValidator blocks the HTTP server until at least one
	// workflow exists in DataStorage. Seeding here — after DS + AuthWebhook
	// are ready but before KA deploys — ensures the catalog is populated
	// when KA starts, matching production deployment ordering.
	//
	// Pre-condition: DataStorage HTTP must be accepting connections.
	// The deployment rollout may complete before the HTTP server is ready.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🌱 PHASE 6b: Seeding workflow catalog (before KA deployment)...")
	phase6bStart := time.Now()

	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for DataStorage HTTP endpoint (pre-condition for seeding)...")
	if err := waitForDataStorageHTTP(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6b: DataStorage HTTP not ready: %w", err)
	}

	if err := SeedE2EActionTypes(ctx, kubeconfigPath, namespace, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6b: failed to seed action types: %w", err)
	}

	fpWorkflows := []WorkflowSeedSpec{
		{FixtureDir: "crashloop-config-fix-job", Environment: "production"},
		{FixtureDir: "oomkill-increase-memory-job", Environment: "production"},
		{FixtureDir: "fix-certificate", Environment: "production"},
		{FixtureDir: "generic-restart", Environment: "production"},
	}
	seededUUIDs, seedErr := SeedWorkflowsViaKubectlApply(ctx, kubeconfigPath, namespace, fpWorkflows, writer)
	if seedErr != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 6b: failed to seed workflows: %w", seedErr)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 6b complete: %d workflows seeded (%s)\n",
		len(seededUUIDs), time.Since(phase6bStart).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7: Go-managed test infrastructure (Issue #1737)
	//
	// Gateway, KubernautAgent, EffectivenessMonitor, AIAnalysis, SignalProcessing,
	// RemediationOrchestrator, WorkflowExecution, Notification, and APIFrontend
	// are all chart-managed now and already ready (PHASE 6's `helm install
	// --wait` blocks until they are). No wave synchronization is needed here
	// anymore -- everything below is independent Go-managed test
	// infrastructure that only needs the chart-managed services to already
	// exist (Gateway for event-exporter/AlertManager auth, DataStorage for
	// RBAC checks), which PHASE 6 guarantees by running before this phase.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🚀 PHASE 7: Go-managed test infrastructure...")
	phase7Start := time.Now()

	// BR-GATEWAY-036/037: Create Gateway SA and token for event-exporter and AlertManager webhooks
	// Signal sources (event-exporter, AlertManager) must send Bearer tokens to /api/v1/signals/* endpoints.
	_, _ = fmt.Fprintln(writer, "  🔐 Creating E2E ServiceAccount for Gateway signal ingestion (BR-GATEWAY-036/037)...")
	gatewaySAName := "fullpipeline-gateway-sa"
	if err := CreateE2EServiceAccountWithGatewayAccess(ctx, namespace, kubeconfigPath, gatewaySAName, writer); err != nil {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 7: failed to create Gateway SA: %w", err)
	}
	gatewayToken, err := GetServiceAccountToken(ctx, namespace, gatewaySAName, kubeconfigPath)
	if err != nil {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 7: failed to get Gateway SA token: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ Gateway auth token ready for event-exporter and AlertManager")

	type waveResult struct {
		name string
		err  error
	}
	// MockLLM + MockLLMShadow + Prometheus + AlertManager + event-exporter +
	// DEX + APIFrontend-DataStorage-RoleBinding + workflow-job-executor RBAC = 8
	allResults := make(chan waveResult, 9)

	// Per-test namespace isolation: generate a unique namespace per remediate
	// scenario so parallel Ginkgo processes never cross-match RRs.
	afRemediateNS = map[string]string{
		"autonomous":  fmt.Sprintf("fp-auto-%s", uuid.New().String()[:8]),
		"interactive": fmt.Sprintf("fp-int-%s", uuid.New().String()[:8]),
		// fleet: dedicated namespace for E2E-AF-1409-001 (#1409 ADR-065), which
		// exercises kubernaut_remediate with a fleet cluster_id argument chained
		// into kubernaut_present_decision to prove cluster_id reaches the
		// SSE-visible investigation_summary artifact end-to-end.
		"fleet": fmt.Sprintf("fp-fleet-%s", uuid.New().String()[:8]),
	}
	_, _ = fmt.Fprintln(writer, "  📌 AF remediate namespaces:")
	for key, ns := range afRemediateNS {
		_, _ = fmt.Fprintf(writer, "      %s → %s\n", key, ns)
	}

	_, _ = fmt.Fprintln(writer, "  Deploying Go-managed test infrastructure in parallel...")

	// Mock LLM (KubernautAgent connects lazily, confirmed by the Issue #1737
	// spike pilot -- no startup ordering dependency on this anymore).
	go func() {
		if skipMockLLM() {
			_, _ = fmt.Fprintln(writer, "  ⏭️  Mock LLM skipped (SKIP_MOCK_LLM is set)")
			allResults <- waveResult{"MockLLM", nil}
			return
		}
		err := DeployMockLLMInNamespace(ctx, namespace, kubeconfigPath, builtImages["mock-llm"], seededUUIDs, afRemediateNS, writer)
		allResults <- waveResult{"MockLLM", err}
	}()

	// Mock LLM Shadow (alignment evaluation — KA config references mock-llm-shadow:8080)
	go func() {
		err := DeployMockLLMShadowInNamespace(ctx, namespace, kubeconfigPath, builtImages["mock-llm"], writer)
		allResults <- waveResult{"MockLLMShadow", err}
	}()

	// Prometheus + AlertManager: chart-external test infrastructure (ADR-EM-001).
	go func() {
		if err := DeployPrometheus(ctx, namespace, kubeconfigPath, writer); err != nil {
			allResults <- waveResult{"Prometheus", err}
			return
		}
		allResults <- waveResult{"Prometheus", nil}
	}()
	go func() {
		if err := DeployAlertManager(ctx, namespace, kubeconfigPath, gatewayToken, writer); err != nil {
			allResults <- waveResult{"AlertManager", err}
			return
		}
		allResults <- waveResult{"AlertManager", nil}
	}()

	// event-exporter: Gateway is already up (PHASE 6 helm install --wait).
	go func() {
		err := deployKubernetesEventExporter(ctx, namespace, kubeconfigPath, gatewayToken, writer)
		allResults <- waveResult{"event-exporter", err}
	}()

	// DEX OIDC provider (Issue #1189): test-only IdP that AF authenticates
	// against. Not part of the production chart -- must stay Go-managed.
	go func() {
		err := deployDexOIDCProviderForAF(ctx, kubeconfigPath, writer)
		allResults <- waveResult{"DEX", err}
	}()

	// AF persona-to-DEX-group RBAC bindings (Issue #1737 regression fix): the
	// chart creates the kubernaut-tool-<persona> ClusterRoles (PHASE 6 helm
	// install) but deliberately not their bindings (see
	// bindAFPersonaToolClusterRoles doc comment). Only depends on those
	// ClusterRoles existing, not on DEX itself being up yet.
	go func() {
		err := bindAFPersonaToolClusterRoles(ctx, kubeconfigPath, writer)
		allResults <- waveResult{"AF-Persona-RBAC-Bindings", err}
	}()

	// APIFrontend -> DataStorage audit RoleBinding (Issue #1189): the chart's
	// own datastorage-rbac.yaml does not include the apifrontend SA (verified
	// against charts/kubernaut/templates/rbac/datastorage-rbac.yaml), so this
	// stays a Go-managed step, same as before Issue #1737.
	go func() {
		err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "apifrontend", writer)
		allResults <- waveResult{"APIFrontend-DataStorage-RoleBinding", err}
	}()

	// workflow-job-executor SA + RBAC (DD-WE-005): required by the
	// oomkill-increase-memory-job / crashloop-config-fix-job workflow fixtures
	// seeded in PHASE 6b, whose Job template sets
	// serviceAccountName: workflow-job-executor. Previously missing from this
	// setup function -- WorkflowExecution's dispatched Jobs would fail with
	// "serviceaccount 'workflow-job-executor' not found" (confirmed during the
	// Issue #1737 spike). Reuses the same helper as
	// workflowexecution_e2e_hybrid.go / fleet_e2e.go.
	//
	// ExecutionNamespace ("kubernaut-workflows"), NOT namespace
	// ("kubernaut-system"): WE's job executor dispatches Jobs into the
	// dedicated execution namespace (DD-WE-002), confirmed by the actual Job
	// spec's serviceAccountName lookup failing there during Issue #1737
	// validation ("serviceaccount kubernaut-workflows/workflow-job-executor
	// not found") when this was mistakenly pointed at namespace instead.
	// createTestNamespace first mirrors fleet_e2e.go's pattern -- idempotent
	// if the WE controller has already created it.
	go func() {
		if err := createTestNamespace(ctx, ExecutionNamespace, kubeconfigPath, writer); err != nil {
			allResults <- waveResult{"workflow-job-executor-RBAC", fmt.Errorf("failed to create %s namespace: %w", ExecutionNamespace, err)}
			return
		}
		err := createWorkflowJobExecutorRBAC(ctx, kubeconfigPath, ExecutionNamespace, writer)
		allResults <- waveResult{"workflow-job-executor-RBAC", err}
	}()

	var deployErrors []error
	for i := 0; i < cap(allResults); i++ {
		r := <-allResults
		if r.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
			deployErrors = append(deployErrors, fmt.Errorf("%s: %w", r.name, r.err))
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", r.name)
		}
	}
	if len(deployErrors) > 0 {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 7 deployments failed: %v", deployErrors)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 7 complete (%s)\n", time.Since(phase7Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 8: Wait for all services ready (parallel readiness checks)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 8: Waiting for all services ready...")
	phase8Start := time.Now()

	if err := waitForFullPipelineServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 8 failed: services not ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 8 complete (%s)\n", time.Since(phase8Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 8b: Verify Prometheus cadvisor scrape target is UP
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 8b: Verifying Prometheus cadvisor scrape target...")
	promURL := fmt.Sprintf("http://127.0.0.1:%d", PrometheusHostPort)
	if err := WaitForPrometheusCadvisorTarget(ctx, promURL, 60*time.Second, writer); err != nil {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 8b failed: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 9: Create enrichment fixture resources (#704)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 9: Creating enrichment fixture resources (#704)...")
	if err := createEnrichmentFixtures(ctx, kubeconfigPath, writer); err != nil {
		return builtImages, seededUUIDs, nil, fmt.Errorf("PHASE 9 failed: enrichment fixtures: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// DONE
	// ═══════════════════════════════════════════════════════════════════════
	totalDuration := time.Since(startTime).Round(time.Second)
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Full Pipeline E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Total setup time: %s\n", totalDuration)
	_, _ = fmt.Fprintf(writer, "  🌐 Gateway:     http://localhost:30080 (chart-pinned NodePort, TLS not yet wired -- tracked follow-up)\n")
	_, _ = fmt.Fprintf(writer, "  🗄️  DataStorage: https://localhost:30081 (chart-pinned NodePort)\n")
	_, _ = fmt.Fprintf(writer, "  🖥️  APIFrontend: https://localhost:30443 (chart-pinned NodePort)\n")
	_, _ = fmt.Fprintf(writer, "  📦 Namespace:   %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  🔑 Kubeconfig:  %s\n", kubeconfigPath)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, seededUUIDs, afRemediateNS, nil
}

// CleanupFullPipelineTestResources deletes all kubernaut CR instances and
// test namespaces, leaving the cluster with deployed services but no test
// state. This is called from SynchronizedAfterSuite so retries start clean.
func CleanupFullPipelineTestResources(kubeconfigPath string, writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "\n🧹 Cleaning up test-created resources...")

	crdKinds := []string{
		"remediationrequests.kubernaut.ai",
		"remediationapprovalrequests.kubernaut.ai",
		"aianalyses.kubernaut.ai",
		"signalprocessings.kubernaut.ai",
		"workflowexecutions.kubernaut.ai",
		"notificationrequests.kubernaut.ai",
		"effectivenessassessments.kubernaut.ai",
		"investigationsessions.kubernaut.ai",
	}

	for _, kind := range crdKinds {
		cmd := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
			"delete", kind, "--all-namespaces", "--all", "--ignore-not-found", "--wait=false")
		out, err := cmd.CombinedOutput()
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ⚠️  %s cleanup: %s\n", kind, strings.TrimSpace(string(out)))
		} else {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" && trimmed != "No resources found" {
				_, _ = fmt.Fprintf(writer, "  🗑️  %s: %s\n", kind, trimmed)
			}
		}
	}

	// Delete test namespaces matching known patterns (fp-am-*, fp-event-*)
	nsCmd := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
	nsOut, err := nsCmd.Output()
	if err == nil {
		for _, ns := range strings.Fields(string(nsOut)) {
			if strings.HasPrefix(ns, "fp-am-") || strings.HasPrefix(ns, "fp-event-") {
				delCmd := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
					"delete", "namespace", ns, "--ignore-not-found", "--wait=false")
				if delOut, delErr := delCmd.CombinedOutput(); delErr != nil {
					_, _ = fmt.Fprintf(writer, "  ⚠️  namespace %s: %s\n", ns, strings.TrimSpace(string(delOut)))
				} else {
					_, _ = fmt.Fprintf(writer, "  🗑️  namespace %s deleted\n", ns)
				}
			}
		}
	}

	_, _ = fmt.Fprintln(writer, "✅ Test resource cleanup complete")
}

// ============================================================================
// PHASE 1: Image Building (3 at a time concurrency for local builds)
// ============================================================================

// buildFullPipelineImages builds all service images with a concurrency limit of 3
// for local builds. In CI/CD mode (IMAGE_REGISTRY+IMAGE_TAG set), BuildImageForKind
// returns the registry reference immediately without building.
func buildFullPipelineImages(ctx context.Context, writer io.Writer) (map[string]string, error) {
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

	enableCoverage := os.Getenv("E2E_COVERAGE") == trueFixture

	for _, baseCfg := range fullPipelineImageConfigs {
		// Skip mock-llm build when SKIP_MOCK_LLM is set (local dev with real LLM)
		if baseCfg.ServiceName == "mock-llm" && skipMockLLM() {
			_, _ = fmt.Fprintln(writer, "  ⏭️  Skipping mock-llm build (SKIP_MOCK_LLM is set)")
			continue
		}
		wg.Add(1)
		cfg := baseCfg // capture loop variable
		// Mock LLM doesn't need Go coverage instrumentation
		if cfg.ServiceName != "mock-llm" {
			cfg.EnableCoverage = enableCoverage
		}

		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot

			imageName, err := BuildImageForKind(ctx, cfg, writer)

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
func loadFullPipelineImages(ctx context.Context, builtImages map[string]string, clusterName string, writer io.Writer) error {
	// LoadImageToKind already checks if the image is a registry image and skips.
	// We still iterate all images — the no-op is cheap.
	var loadErrors []error
	for serviceName, imageName := range builtImages {
		if err := LoadImageToKind(ctx, imageName, serviceName, clusterName, writer); err != nil {
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
// PHASE 10: Test Infrastructure (event-exporter + memory-eater)
// ============================================================================

// deployKubernetesEventExporter deploys the kubernetes-event-exporter that watches
// K8s Events and forwards them to the Gateway webhook endpoint.
//
// The event-exporter:
//   - Watches for Warning events (OOMKilled, CrashLoopBackOff, etc.)
//   - POSTs to Gateway's /api/v1/signals/kubernetes-event endpoint
//   - Runs in kubernaut-system namespace with RBAC for cluster-wide event watching
//   - BR-GATEWAY-036/037: When gatewayToken is non-empty, adds Bearer auth to webhook requests
//
// Image: ghcr.io/resmoio/kubernetes-event-exporter:latest
// (No local build needed — pulled directly by Kind's containerd)
func deployKubernetesEventExporter(ctx context.Context, namespace, kubeconfigPath, gatewayToken string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  📡 Deploying kubernetes-event-exporter...")

	// BR-GATEWAY-036/037: Add Authorization header to webhook when token is provided
	authHeaderYaml := ""
	if gatewayToken != "" {
		authHeaderYaml = fmt.Sprintf("            Authorization: Bearer %s\n", gatewayToken)
	}

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
# ConfigMap: route Warning events from fp-e2e-* and fp-approval-* namespaces to Gateway
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
        # Forward K8s events from fp-e2e-* (K8s event test), fp-approval-* (approval lifecycle),
        # and fp-mcp-* (MCP interactive lifecycle) namespaces to Gateway.
        # fp-am-* namespaces excluded to prevent K8s event duplication in AlertManager test.
        - match:
            - namespace: "fp-e2e-*"
              receiver: gateway-webhook
            - namespace: "fp-approval-*"
              receiver: gateway-webhook
            - namespace: "fp-mcp-*"
              receiver: gateway-webhook
          drop:
            - type: "Normal"
    receivers:
      - name: gateway-webhook
        webhook:
          endpoint: "http://gateway-service.%[1]s.svc.cluster.local:8080/api/v1/signals/kubernetes-event"
          headers:
            Content-Type: application/json
%[2]s
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
`, namespace, authHeaderYaml)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// deployMockSlack deploys a minimal HTTP service that accepts POST requests and
// returns 200 OK. Per-receiver Slack delivery services resolve webhook URLs from
// credentialRef files (#244). The mock-slack service acts as the webhook target
// for E2E tests. Without it, Slack delivery fails with connection errors.
func deployMockSlack(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  📨 Deploying mock-slack (webhook sink)...")

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: mock-slack-config
  namespace: %[1]s
data:
  default.conf: |
    server {
      listen 8080;
      location / {
        return 200 'ok';
        add_header Content-Type text/plain;
      }
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-slack
  namespace: %[1]s
  labels:
    app: mock-slack
    component: test-infrastructure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-slack
  template:
    metadata:
      labels:
        app: mock-slack
        component: test-infrastructure
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/nginx/conf.d
      volumes:
      - name: config
        configMap:
          name: mock-slack-config
---
apiVersion: v1
kind: Service
metadata:
  name: mock-slack
  namespace: %[1]s
  labels:
    app: mock-slack
    component: test-infrastructure
spec:
  selector:
    app: mock-slack
  ports:
  - port: 8080
    targetPort: 8080
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
	return DeployMemoryEaterWithLimits(ctx, targetNamespace, kubeconfigPath, "50Mi", "20Mi", writer)
}

// DeployMemoryEaterWithLimits deploys a memory-eater with configurable resource limits.
// Different limits produce a different spec_hash, isolating the deployment from
// ineffective chain detection triggered by other tests with the same spec.
func DeployMemoryEaterWithLimits(ctx context.Context, targetNamespace, kubeconfigPath, memLimit, memRequest string, writer io.Writer) error {
	return DeployMemoryEaterNamed(ctx, "memory-eater", targetNamespace, kubeconfigPath, memLimit, memRequest, writer)
}

// DeployMemoryEaterNamed deploys a memory-eater Deployment under a caller-chosen
// name/label, instead of the hardcoded "memory-eater" used by
// DeployMemoryEaterWithLimits. This lets a test stand up its own dedicated
// instance without colliding with a shared "memory-eater" fixture that other,
// unrelated tests may already depend on existing in the same namespace (e.g.
// the fleet suite's signal-routing tests -- see E2E-FLEET-015).
func DeployMemoryEaterNamed(ctx context.Context, name, targetNamespace, kubeconfigPath, memLimit, memRequest string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🐛 Deploying %s in namespace %s...\n", name, targetNamespace)

	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
  labels:
    app: %[1]s
    kubernaut.ai/managed: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
        kubernaut.ai/managed: "true"
    spec:
      automountServiceAccountToken: false
      containers:
      - name: %[1]s
        image: us-central1-docker.pkg.dev/genuine-flight-317411/devel/memory-eater:1.0
        imagePullPolicy: IfNotPresent
        # Positional args: initial_memory initial_duration target_memory target_duration hold_duration
        # Consumes 40Mi for 1s, then grows to 60Mi over 1s. With memLimit < 60Mi
        # (the default 50Mi caller), the OOM-kill happens during the growth
        # phase, before hold_duration is ever reached, so hold_duration has no
        # effect on triggering the initial OOMKill.
        #
        # Issue #1542 follow-up (E2E-FP-118-001): hold_duration was previously
        # "0", which -- once a real fix (e.g. oomkill-increase-memory-v1) lifts
        # memLimit above 60Mi -- makes the process exit successfully right
        # after reaching target memory (~2s lifecycle), so the container
        # perpetually restart-loops even though the underlying OOM is fixed.
        # EM's one-shot post-remediation health check then samples the pod at
        # an arbitrary point in that restart cycle, making "HealthScore > 0"
        # racy independent of whether remediation worked. A large
        # hold_duration lets the fixed pod actually reach and stay in a
        # stable Ready state, matching real-world workload behavior.
        args: ["40Mi", "1", "60Mi", "1", "999999"]
        resources:
          limits:
            memory: "%[3]s"
          requests:
            memory: "%[4]s"
`, name, targetNamespace, memLimit, memRequest)

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
      automountServiceAccountToken: false
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

// indentYAMLLines indents each non-empty line of s by the given number of spaces.
func indentYAMLLines(s string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
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
		"kubernaut-agent",
		"gateway",
		"event-exporter",
		"mock-slack",   // Accepts Slack webhook POSTs so notifications reach terminal phase
		"prometheus",   // ADR-EM-001: Prometheus for EM metric comparison
		"alertmanager", // ADR-EM-001: AlertManager for EM alert resolution
		"apifrontend",  // Issue #1189: AF as FP signal source
		"dex",          // Issue #1189: OIDC provider for AF authentication
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
			pollErr := pollUntilReady(ctx, 3*time.Minute, 2*time.Second, func() bool {
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
			pollErr := pollUntilReady(ctx, 3*time.Minute, 2*time.Second, func() bool {
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

// waitForDataStorageHTTP blocks until the DataStorage pod is Ready and the
// cluster-internal service is accepting HTTP connections. Phase 6 applies the
// manifest but doesn't wait for the readiness probe; this guard prevents
// Phase 6b's ActionType seeding from hitting "connection refused" when the
// AuthWebhook's validating webhook forwards to DataStorage.
func waitForDataStorageHTTP(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	rolloutCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/datastorage",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s")
	rolloutCmd.Stdout = writer
	rolloutCmd.Stderr = writer
	if err := rolloutCmd.Run(); err != nil {
		return fmt.Errorf("DataStorage rollout not ready: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ DataStorage deployment rolled out")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		pods, listErr := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage",
		})
		if listErr == nil && len(pods.Items) > 0 {
			for _, cond := range pods.Items[0].Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					_, _ = fmt.Fprintf(writer, "  ✅ DataStorage pod %s ready\n", pods.Items[0].Name)
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("DataStorage pod not ready within 60s")
}
