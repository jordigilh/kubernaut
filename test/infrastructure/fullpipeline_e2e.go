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
	"errors"
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
//  11. Prometheus in the dedicated monitoring namespace (metric comparison for EM, ADR-EM-001)
//  12. AlertManager in the dedicated monitoring namespace (alert resolution for EM, ADR-EM-001)
//  13. EffectivenessMonitor (CRD controller, watches EA CRDs)
//
// Test infrastructure:
//   - kubernetes-event-exporter: watches K8s Events, POSTs to Gateway webhook
//   - memory-eater: target pod that triggers OOMKill events
//
// Port Allocation (DD-TEST-001 v2.8, Issue #1737):
//   Gateway:      hostPort 30080, test-owned NodePort adapter (host-side signal POSTs, e.g. af_helpers_test.go)
//   DataStorage:  hostPort 30081, test-owned NodePort adapter (workflow seeding + audit queries)
//   API Frontend: hostPort 18443, NodePort 30443 (DD-TEST-001)
//   All three are allocated by the operator-exposure test fixture before cluster creation;
//   the operator-managed Services remain ClusterIP. NetworkPolicies admit the test boundary.
//   ipBlock rules to admit the NodePort-sourced (SNAT'd to node IP) traffic
//   through the default-deny NetworkPolicy. Host reachability additionally
//   requires the exact port to be pre-mapped in kind-fullpipeline-config.yaml's
//   extraPortMappings (fixed at cluster-creation time, before operator reconciliation
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

const mockLLMServiceName = "mock-llm"

// skipMockLLM returns true when the SKIP_MOCK_LLM environment variable is set
// to any non-empty value. When true, the Mock LLM service is NOT built, deployed,
// or checked for readiness. Use this for local development where Kubernaut Agent connects
// to a real LLM (e.g., Vertex AI). CI/CD pipelines leave this unset so Mock LLM
// provides a fully self-contained test environment.
func skipMockLLM() bool {
	return os.Getenv("SKIP_MOCK_LLM") != ""
}

func shouldDeployDexForAF(fleetProvisioner FleetProvisioner) bool {
	return fleetProvisioner == nil
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
	{ServiceName: mockLLMServiceName, ImageName: "kubernaut/mock-llm", DockerfilePath: "test/services/mock-llm/go.Dockerfile", BuildContextPath: ""},
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
// fleetProvisioner, when non-nil, is invoked once the target namespace
// exists (end of PHASE 5) but before operator reconciliation (PHASE 6) to deploy
// fleet-specific infrastructure (IdP, MCP Gateway, remote cluster) and
// return Fleet values to apply to THIS Kubernaut CR with --
// DD-TEST-015 "deploy correctly the first time" (see
// SetupFleetE2EInfrastructure). Nil (the FP suite's call) leaves fleet
// disabled, unchanged from pre-refactor behavior. See FleetProvisioner's
// doc comment for why this must be a callback rather than a pre-computed
// value.
//
// Returns:
//   - builtImages: Map of service name → full image reference (for cleanup)
//   - seededUUIDs: Map of "workflow_name:environment" → UUID (seeded in Phase 6b)
//   - error: First fatal error encountered
func fullPipelineWorkflowSeeds(fleetMode bool) []WorkflowSeedSpec {
	seeds := []WorkflowSeedSpec{
		{FixtureDir: "crashloop-config-fix-job", Environment: "production"},
		{FixtureDir: "oomkill-increase-memory-job", Environment: "production"},
	}
	if fleetMode {
		// Fleet E2E-012/014 and E2E-015 consume these job-backed fixtures.
		// The fleet-exec-cluster-override fixture is seeded separately by
		// SetupFleetE2EInfrastructure. Keeping unrelated local workflow fixtures
		// out of this list prevents Fleet setup from mutating their shared
		// workflow names in the same namespace.
		return seeds
	}

	return append(seeds,
		WorkflowSeedSpec{FixtureDir: "gitops-drift-2390", Environment: "production"},
		WorkflowSeedSpec{FixtureDir: "history-alternative-2496", Environment: "production"},
		WorkflowSeedSpec{FixtureDir: "standalone-exec-cluster-id", Environment: "production"},
		WorkflowSeedSpec{FixtureDir: "fix-certificate", Environment: "production"},
		WorkflowSeedSpec{FixtureDir: "generic-restart", Environment: "production"},
		// FullPipeline A2A targets are isolated in managed staging namespaces.
		// Seed matching catalog entries so discovery sees workflow IDs from the
		// target's environment while production system fixtures remain available.
		WorkflowSeedSpec{FixtureDir: "crashloop-config-fix-job", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "oomkill-increase-memory-job", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "gitops-drift-2390", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "history-alternative-2496", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "standalone-exec-cluster-id", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "generic-restart", Environment: "staging"},
		WorkflowSeedSpec{FixtureDir: "fullpipeline-consent-job", Environment: "staging"},
	)
}

type fullPipelineOperatorInputs struct {
	relatedImages       map[string]string
	operatorChartPath   string
	dependencyChartPath string
	operatorImage       string
	exposure            OperatorExposurePlan
	fixtures            OperatorFixtureFiles
	cleanup             func()
}

func prepareFullPipelineOperatorInputs(ctx context.Context, clusterName, namespace string, builtImages map[string]string, writer io.Writer) (fullPipelineOperatorInputs, error) {
	dbMigrateImage, err := ensureDBMigrateImage(ctx, "operator-"+uuid.New().String()[:8], writer)
	if err != nil {
		return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: db-migrate image failed: %w", err)
	}
	builtImages["db-migrate"] = dbMigrateImage
	relatedImages, err := OperatorOperandImagesFromBuiltImages(builtImages)
	if err != nil {
		return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: operator operand image map: %w", err)
	}
	operatorChartPath, dependencyChartPath, err := OperatorChartPathsFromEnvironment(ctx)
	if err != nil {
		return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: operator chart artifacts: %w", err)
	}
	operatorImage := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_IMAGE"))
	if operatorImage == "" {
		return fullPipelineOperatorInputs{}, errors.New("PHASE 1: KUBERNAUT_OPERATOR_IMAGE is required for operator qualification")
	}
	exposure, err := OperatorExposurePlanForCluster(ctx, clusterName, namespace, "localhost", []string{"gateway", "datastorage", "apifrontend", "kubernautagent"})
	if err != nil {
		return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: allocate operator exposure: %w", err)
	}
	fixtureDir, err := os.MkdirTemp("", "operator-fp-fixtures-*")
	if err != nil {
		return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: create operator fixture directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(fixtureDir) }
	files := OperatorFixtureFiles{LLMCredentials: filepath.Join(fixtureDir, "llm-credentials"), AIAnalysisPolicy: filepath.Join(fixtureDir, "ai-analysis.rego"), SignalProcessingPolicy: filepath.Join(fixtureDir, "signal-processing.rego")}
	for path, content := range map[string]string{files.LLMCredentials: "mock-llm-e2e-key", files.AIAnalysisPolicy: fullPipelineAIAnalysisPolicyRego, files.SignalProcessingPolicy: fullPipelineSignalProcessingPolicyRego} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			cleanup()
			return fullPipelineOperatorInputs{}, fmt.Errorf("PHASE 1: write operator fixture %s: %w", path, err)
		}
	}
	return fullPipelineOperatorInputs{relatedImages: relatedImages, operatorChartPath: operatorChartPath, dependencyChartPath: dependencyChartPath, operatorImage: operatorImage, exposure: exposure, fixtures: files, cleanup: cleanup}, nil
}

func SetupFullPipelineInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, fleetProvisioner FleetProvisioner, writer io.Writer) (builtImages map[string]string, seededUUIDs map[string]string, afRemediateNS map[string]string, err error) {
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

	operatorInputs, operatorErr := prepareFullPipelineOperatorInputs(ctx, clusterName, namespace, builtImages, writer)
	if operatorErr != nil {
		return builtImages, nil, nil, operatorErr
	}
	defer operatorInputs.cleanup()
	operatorImages := operatorInputs.relatedImages
	operatorChartPath := operatorInputs.operatorChartPath
	dependencyChartPath := operatorInputs.dependencyChartPath
	operatorImage := operatorInputs.operatorImage
	operatorExposure := operatorInputs.exposure
	operatorFixtures := operatorInputs.fixtures

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🏗️  PHASE 2: Creating Kind cluster...")
	phase2Start := time.Now()

	_, hostIngressCIDR, err := createFullPipelineCluster(ctx, projectRoot, clusterName, kubeconfigPath, operatorExposure, writer)
	if err != nil {
		return builtImages, nil, nil, err
	}
	_, _ = fmt.Fprintf(writer, "  🔐 Operator NetworkPolicy host ingress peer: %s\n", hostIngressCIDR)
	_, _ = fmt.Fprintf(writer, "✅ PHASE 2 complete: Kind cluster ready (%s)\n",
		time.Since(phase2Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load locally-built images into Kind (skip for registry images)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	phase3Start := time.Now()

	// Issue #1737: always load now -- the shared application image tag helper always produces
	// fresh localhost/*:<tag> references (regardless of the original source
	// mode), so LoadImageToKind must actually load them every time. mock-llm
	// follows its own pre-existing image ref, handled the same as before.
	if err := loadFullPipelineImages(ctx, builtImages, clusterName, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("PHASE 3 failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 3 complete: images loaded (%s)\n",
		time.Since(phase3Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: (removed, Issue #1737) -- config/crd/bases/ is installed by the
	// operator bootstrap and Kubernaut CR reconciliation in PHASE 6 below. Manual
	// `kubectl apply` of config/crd/bases/*.yaml is no longer needed for the
	// operator-managed services. The operator owns the CRD lifecycle.
	// ═══════════════════════════════════════════════════════════════════════

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Namespace + operator/dependency prerequisite Secrets
	//
	// Issue #1737: Phase 5 applies the DataStorage client RBAC, inter-service TLS,
	// and AU-9 signing certificate as administrator/test-managed prerequisites.
	// These resources are present before operator reconciliation and do not show
	// that the operator supplied them. Credential Secrets remain explicitly
	// user/test managed (original #239 audit finding #4).
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 5: Namespace + Helm prerequisite Secrets...")
	phase5Start := time.Now()

	if err := CreateTestNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("failed to create namespace: %w", err)
	}
	if err := CreateTestNamespace(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
		return builtImages, nil, nil, fmt.Errorf("failed to create monitoring namespace: %w", err)
	}
	crManifestPath := ""
	hostIngressPolicyCIDR := hostIngressCIDR
	if fleetProvisioner != nil {
		crManifestPath = filepath.Join(projectRoot, "test", "fixtures", "operator", "kubernaut-fleet.yaml")
	}
	operatorConfig := OperatorDeploymentConfig{
		ClusterName:         clusterName,
		Namespace:           namespace,
		KubeconfigPath:      kubeconfigPath,
		OperatorChartPath:   operatorChartPath,
		DependencyChartPath: dependencyChartPath,
		OperatorImage:       operatorImage,
		RelatedImages:       operatorImages,
		PostgreSQLImage:     "docker.io/library/postgres:16.4@sha256:e62fbf9d3e2b49816a32c400ed2dba83e3b361e6833e624024309c35d334b412",
		ValkeyImage:         "docker.io/valkey/valkey:8.0.2@sha256:57bcc49c6ade1813ef25206c571b65b66bb0094235ff7fb767941622892297d9",
		FixtureFiles:        operatorFixtures,
		CRManifestPath:      crManifestPath,
		HostIngressCIDR:     hostIngressPolicyCIDR,
		MonitoringNamespace: monitoringNamespace,
		Exposure:            &operatorExposure,
		DiagnosticsDir:      strings.TrimSpace(os.Getenv("KUBERNAUT_QUALIFICATION_DIR")),
		Provenance: OperatorArtifactProvenance{
			Repository: operatorRepository, RequestedRef: operatorRevision,
			ResolvedSHA: strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_SHA")),
			ImageRef:    operatorImage, Profile: "generic-kind-v1.6",
		},
		CR: OperatorCROptions{
			Name: operatorCRName, Namespace: namespace,
			PostgreSQLHost:    "postgresql." + namespace + ".svc.cluster.local",
			PostgreSQLSSLMode: "require",
			ValkeyHost:        "valkey." + namespace + ".svc.cluster.local",
			PostgreSQLSecret:  "postgresql-secret", ValkeySecret: "valkey-secret",
			ValkeyTLS:                        &OperatorValkeyTLSOptions{Enabled: true, CASecretName: "valkey-ca", ClientCertSecretName: "valkey-tls-client"},
			LLM:                              OperatorLLMOptions{Provider: "openai", Model: "mock-model", Endpoint: "http://mock-llm." + namespace + ".svc.cluster.local:8080", CredentialsSecret: "llm-credentials-primary"},
			MaxConcurrentInvestigations:      50,
			InteractiveRateLimitPerUser:      100,
			RemediationStabilizationWindow:   "10s",
			EffectivenessStabilizationWindow: "30s",
			EffectivenessValidityWindow:      "120s",
			TLS:                              OperatorTLSOptions{Mode: "manual"},
			PolicyConfigMaps:                 OperatorPolicyOptions{AIAnalysis: "aianalysis-policy", SignalProcessing: "signalprocessing-policy"},
			// Prometheus/AlertManager are test-owned endpoints signed by the
			// monitoring CA. Mount that CA into operator-managed clients instead
			// of the application inter-service trust bundle.
			Monitoring: OperatorMonitoringOptions{PrometheusURL: "https://prometheus-svc." + monitoringNamespace + ".svc.cluster.local:9090", AlertManagerURL: "https://alertmanager-svc." + monitoringNamespace + ".svc.cluster.local:9093", TLSCASecretName: monitoringClientCASecretName},
			// Kind host ingress is runtime test infrastructure. Apply the
			// test-owned policy after reconciliation instead of putting the
			// host bridge CIDR into the user CR.
			NetworkPolicies: OperatorNetworkPolicyOptions{},
			GatewayEnabled:  true, APIFrontendEnabled: true,
			IssuerURL: "https://dex:5556/dex", JWKSURL: "https://dex:5556/dex/keys", Audience: "kubernaut-apifrontend",
		},
	}
	if err := prepareFullPipelineOperatorResources(ctx, operatorConfig, kubeconfigPath, namespace, monitoringNamespace, fleetProvisioner != nil, writer); err != nil {
		return builtImages, nil, nil, err
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 5 complete (%s)\n", time.Since(phase5Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5b: Fleet infrastructure provisioning (DD-TEST-015)
	//
	// Runs the caller-supplied fleet provisioning (IdP, MCP Gateway, remote
	// cluster) now that the namespace exists but before PHASE 6's operator
	// reconciliation -- so every operator-managed pod that boots with Fleet
	// enabled already has a live, authenticated MCP Gateway to connect to,
	// instead of racing infrastructure that gets deployed afterward. Nil for the
	// FP suite (fleet stays disabled).
	// ═══════════════════════════════════════════════════════════════════════
	fleetOpts, err := provisionFullPipelineFleet(ctx, kubeconfigPath, namespace, fleetProvisioner, writer)
	if err != nil {
		return builtImages, nil, nil, err
	}

	// A preserved qualification cluster can retain CRs and fixture namespaces
	// after a prior parallel run. Clear that test-owned state after the
	// operator CRDs exist but before applying the next user-owned CR; otherwise
	// fixed-name InvestigationSessions and old target namespaces cross-contaminate
	// the next qualification run.
	_, _ = fmt.Fprintln(writer, "\n🧹 Clearing retained FullPipeline test state before CR apply...")
	CleanupFullPipelineTestResources(ctx, kubeconfigPath, writer)

	// ═══════════════════════════════════════════════════════════════════════
	if err := applyFullPipelineOperatorPhase(ctx, operatorConfig, fleetOpts, fleetProvisioner, kubeconfigPath, namespace, writer); err != nil {
		return builtImages, nil, nil, err
	}

	seededUUIDs, seedErr := seedFullPipelineWorkflowCatalog(ctx, namespace, kubeconfigPath, fleetProvisioner != nil, writer)
	if seedErr != nil {
		return builtImages, nil, nil, seedErr
	}

	afRemediateNS, err = runFullPipelineGoManagedInfrastructure(ctx, namespace, kubeconfigPath, monitoringNamespace, builtImages, seededUUIDs, fleetProvisioner, writer)
	if err != nil {
		return builtImages, seededUUIDs, nil, err
	}

	if err := verifyFullPipelineReadiness(ctx, namespace, kubeconfigPath, monitoringNamespace, fleetProvisioner, afRemediateNS, fleetOpts != nil, writer); err != nil {
		return builtImages, seededUUIDs, nil, err
	}

	// ═══════════════════════════════════════════════════════════════════════
	// DONE
	// ═══════════════════════════════════════════════════════════════════════
	totalDuration := time.Since(startTime).Round(time.Second)
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Full Pipeline E2E Infrastructure Ready!")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "  ⏱️  Total setup time: %s\n", totalDuration)
	_, _ = fmt.Fprintf(writer, "  🌐 Gateway:     https://localhost:30080 (chart-pinned NodePort)\n")
	_, _ = fmt.Fprintf(writer, "  🗄️  DataStorage: https://localhost:30081 (chart-pinned NodePort)\n")
	_, _ = fmt.Fprintf(writer, "  🖥️  APIFrontend: https://localhost:18443 (DD-TEST-001 host port)\n")
	_, _ = fmt.Fprintf(writer, "  📦 Namespace:   %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  🔑 Kubeconfig:  %s\n", kubeconfigPath)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return builtImages, seededUUIDs, afRemediateNS, nil
}

func provisionFullPipelineFleet(ctx context.Context, kubeconfigPath, namespace string, provisioner FleetProvisioner, writer io.Writer) (*FleetHelmOptions, error) {
	if provisioner == nil {
		return nil, nil
	}
	_, _ = fmt.Fprintln(writer, "\n🌐 PHASE 5b: Provisioning fleet infrastructure (IdP + MCP Gateway)...")
	started := time.Now()
	options, err := provisioner(ctx, kubeconfigPath, namespace, writer)
	if err != nil {
		return nil, fmt.Errorf("PHASE 5b: fleet infrastructure provisioning failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 5b complete (%s)\n", time.Since(started).Round(time.Second))
	return options, nil
}

func createFullPipelineCluster(ctx context.Context, projectRoot, clusterName, kubeconfigPath string, exposure OperatorExposurePlan, writer io.Writer) (string, string, error) {
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return "", "", fmt.Errorf("failed to create coverdata directory: %w", err)
	}
	extraMounts := []ExtraMount{}
	if os.Getenv("E2E_COVERAGE") == trueFixture {
		extraMounts = append(extraMounts, ExtraMount{HostPath: coverdataPath, ContainerPath: "/coverdata", ReadOnly: false})
	}
	if err := CreateKindClusterWithExtraMountsAndPortMappings(ctx, clusterName, kubeconfigPath, "test/infrastructure/kind-fullpipeline-config.yaml", extraMounts, OperatorPortMappings(exposure), writer); err != nil {
		return "", "", fmt.Errorf("PHASE 2 failed: %w", err)
	}
	hostIngressCIDR, err := KindHostIngressCIDR(ctx, clusterName)
	if err != nil {
		return "", "", fmt.Errorf("PHASE 2: resolve Kind host ingress CIDR: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  🔐 Operator NetworkPolicy host ingress peer: %s\n", hostIngressCIDR)
	return coverdataPath, hostIngressCIDR, nil
}

func seedFullPipelineWorkflowCatalog(ctx context.Context, namespace, kubeconfigPath string, fleetMode bool, writer io.Writer) (map[string]string, error) {
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
		return nil, fmt.Errorf("PHASE 6b: DataStorage HTTP not ready: %w", err)
	}

	// #1985 (BR-AUDIT-005 v2.0): AuthWebhook's own /readyz now blocks on
	// DataStorage's health endpoint being reachable, so it can no longer be
	// assumed ready merely because operator reconciliation returned or because
	// DataStorage itself just became ready --
	// AuthWebhook's readiness probe only starts succeeding some time *after*
	// DataStorage's. SeedE2EActionTypes below creates/deletes ActionType CRs,
	// which the actiontype.validate.kubernaut.ai ValidatingWebhookConfiguration
	// routes to AuthWebhook's :443 endpoint -- if its pod isn't Ready yet, the
	// Service has no endpoints and the apiserver's webhook call fails closed
	// with "dial tcp ...:443: connect: connection refused" (confirmed via CI
	// must-gather).
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for AuthWebhook rollout (validating webhook pre-condition)...")
	if err := waitForDeploymentRollout(ctx, namespace, kubeconfigPath, "authwebhook", 120*time.Second, writer); err != nil {
		return nil, fmt.Errorf("PHASE 6b: AuthWebhook rollout not ready: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ AuthWebhook deployment rolled out")

	// Deployment rollout completing does not guarantee the Service's own
	// Endpoints/kube-proxy DNAT are programmed yet (see
	// waitForServiceEndpointReady doc comment) -- close that remaining gap
	// before the apiserver's webhook call below can hit it.
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for AuthWebhook Service endpoint (kube-proxy programming)...")
	if err := waitForServiceEndpointReady(ctx, namespace, kubeconfigPath, "authwebhook-service", 60*time.Second, writer); err != nil {
		return nil, fmt.Errorf("PHASE 6b: AuthWebhook service endpoint not ready: %w", err)
	}

	// The pinned operator RC admits ActionType CRs successfully but does not
	// populate the legacy AuthWebhook registration status used by
	// SeedE2EActionTypes. Use the direct CRD seeder here; it applies the same
	// catalog status contract explicitly and keeps workflow seeding independent
	// of that legacy admission-side effect.
	if err := SeedActionTypesViaCRD(ctx, kubeconfigPath, namespace, writer); err != nil {
		return nil, fmt.Errorf("PHASE 6b: failed to seed action types: %w", err)
	}

	fpWorkflows := fullPipelineWorkflowSeeds(fleetMode)
	// The pinned RC admits RemediationWorkflow CRs but does not populate the
	// legacy AuthWebhook status.workflowId side effect. Stamp the same catalog
	// status through the direct CRD path so workflow discovery sees the seeded
	// catalog without depending on that legacy admission mutation.
	seededUUIDs, seedErr := SeedWorkflowsViaDirectCRDCreationFromKubeconfig(ctx, kubeconfigPath, namespace, fpWorkflows, writer)
	if seedErr != nil {
		return nil, fmt.Errorf("PHASE 6b: failed to seed workflows: %w", seedErr)
	}
	_, _ = fmt.Fprintf(writer, "✅ PHASE 6b complete: %d workflows seeded (%s)\n",
		len(seededUUIDs), time.Since(phase6bStart).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	return seededUUIDs, nil
}

func applyFullPipelineOperatorPhase(ctx context.Context, operatorConfig OperatorDeploymentConfig, fleetOpts *FleetHelmOptions, fleetProvisioner FleetProvisioner, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n🗄️  PHASE 6: applying Kubernaut CR through the operator...")
	phase6Start := time.Now()

	type deployResult struct {
		name string
		err  error
	}
	phase6Results := make(chan deployResult, 2)
	go func() {
		if fleetOpts != nil {
			operatorConfig.CR.Fleet = operatorFleetOptionsFromHelm(fleetOpts)
			operatorConfig.CR.IssuerURL = "https://keycloak:8443/realms/kubernaut-demo"
			operatorConfig.CR.JWKSURL = "https://keycloak." + namespace + ".svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs"
		}
		evidence, deployErr := ApplyOperatorCRAndWait(ctx, operatorConfig, writer)
		if deployErr == nil {
			RememberOperatorExposure(kubeconfigPath, evidence)
		}
		phase6Results <- deployResult{"OperatorCR", deployErr}
	}()
	go func() {
		phase6Results <- deployResult{"Mock-Slack",
			deployMockSlack(ctx, namespace, kubeconfigPath, writer)}
	}()
	for i := 0; i < 2; i++ {
		r := <-phase6Results
		if r.err != nil {
			return fmt.Errorf("%s deployment failed: %w", r.name, r.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", r.name)
	}
	// API Frontend keeps A2A session state in memory. A preserved cluster can
	// therefore retain fixed qualification session IDs (for example
	// ctx-fp-cg3-1) even after their CRs are deleted; restart the now-reconciled
	// operator-owned session service so the next run starts with a clean driver
	// pool.
	if err := runKubectl(ctx, kubeconfigPath, writer, "rollout", "restart", "deployment/apifrontend", "-n", namespace); err != nil {
		return fmt.Errorf("restart API Frontend session service: %w", err)
	}
	if err := waitForDeployment(ctx, "apifrontend", namespace, kubeconfigPath, 3*time.Minute, writer); err != nil {
		return fmt.Errorf("API Frontend session service restart failed: %w", err)
	}
	if fleetProvisioner != nil {
		if err := waitForAFPersonaRBAC(ctx, kubeconfigPath, writer); err != nil {
			return fmt.Errorf("AF persona RBAC verification failed: %w", err)
		}
	} else if err := BindAFPersonaRBAC(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("AF persona RBAC binding failed: %w", err)
	}

	// The manual TLS material was prepared before the CR was applied. The
	// operator now validates those administrator-owned Secrets instead of
	// generating runtime certificates from removed hook/self-signed fields.
	// DEX still needs its own leaf certificate before PHASE 7.
	if shouldDeployDexForAF(fleetProvisioner) {
		if err := ensureDexTLSFromOperatorCA(ctx, kubeconfigPath, namespace, writer); err != nil {
			return fmt.Errorf("PHASE 6: dex-tls provisioning failed: %w", err)
		}
	}

	// The host-accessed operator Services use test-owned NodePort adapters.
	// Their manual serving Secrets are signed with the pre-created operator CA,
	// so no post-reconciliation certificate rewrite or rollout is required.

	// Issue #785 + #1737: suite_test.go's NewTLSAwareTransport (host-side E2E
	// HTTP client) reads the inter-service CA from a local file that the
	// Go-native GenerateInterServiceTLS used to write as a side effect of
	// generating the CA itself; the chart generates the CA in-cluster now, so
	// fetch it back out to the same expected path.
	if err := writeInterServiceCAPEMFromOperator(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("PHASE 6: failed to write inter-service CA PEM: %w", err)
	}
	if err := ReplicateInterServiceCAConfigMap(ctx, kubeconfigPath, monitoringNamespace, writer); err != nil {
		return fmt.Errorf("PHASE 6: monitoring Gateway CA setup failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 6 complete (%s)\n", time.Since(phase6Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	return nil
}

func prepareFullPipelineOperatorResources(ctx context.Context, config OperatorDeploymentConfig, kubeconfigPath, namespace, monitoringNamespace string, fleetEnabled bool, writer io.Writer) error {
	steps := []struct {
		name string
		fn   func() error
	}{
		{"operator dependencies", func() error { return InstallOperatorDependencies(ctx, config, writer) }},
		{"operator bootstrap", func() error { return InstallOperatorBootstrap(ctx, config, writer) }},
		{"DataStorage client RBAC", func() error { return EnsureOperatorDataStorageClientRBAC(ctx, kubeconfigPath, writer) }},
		{"operator Fleet TLS", func() error { return EnsureOperatorFleetTLS(ctx, kubeconfigPath, namespace, writer) }},
		{"operator application TLS", func() error {
			return EnsureOperatorApplicationTLS(ctx, kubeconfigPath, namespace, fleetEnabled, writer)
		}},
		{"monitoring TLS", func() error { return EnsureMonitoringTLS(ctx, kubeconfigPath, namespace, monitoringNamespace, writer) }},
		{"Prometheus Operator dependency", func() error { return InstallPrometheusOperator(ctx, kubeconfigPath, writer) }},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			return fmt.Errorf("failed to prepare %s: %w", step.name, err)
		}
	}
	return nil
}

func verifyFullPipelineReadiness(ctx context.Context, namespace, kubeconfigPath, monitoringNamespace string, fleetProvisioner FleetProvisioner, afRemediateNS map[string]string, fleetEnabled bool, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "\n⏳ PHASE 8: Waiting for all services ready...")
	results := make(chan error, 2)
	go func() {
		results <- waitForFullPipelineServicesReady(ctx, namespace, kubeconfigPath, fleetProvisioner, writer)
	}()
	go func() {
		results <- waitForMonitoringDeploymentsReady(ctx, monitoringNamespace, kubeconfigPath, 3*time.Minute, writer)
	}()
	var errs []error
	for range 2 {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("PHASE 8 failed: %w", errors.Join(errs...))
	}
	if fleetEnabled {
		if err := waitForDeployment(ctx, "fleetmetadatacache", namespace, kubeconfigPath, 3*time.Minute, writer); err != nil {
			return fmt.Errorf("PHASE 8 failed: fleetmetadatacache not ready: %w", err)
		}
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 8 complete")
	if err := WaitForPrometheusCadvisorTarget(ctx, fmt.Sprintf("https://127.0.0.1:%d", PrometheusHostPort), 360*time.Second, writer, MonitoringCAPath(kubeconfigPath)); err != nil {
		return fmt.Errorf("PHASE 8b failed: %w", err)
	}
	if !skipMockLLM() {
		if err := SeedFullPipelineA2AGroundingRules(ctx, monitoringNamespace, kubeconfigPath, afRemediateNS, writer); err != nil {
			return fmt.Errorf("PHASE 8c failed: A2A severity grounding: %w", err)
		}
	}
	if err := createEnrichmentFixtures(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("PHASE 9 failed: enrichment fixtures: %w", err)
	}
	return nil
}

func runFullPipelineGoManagedInfrastructure(ctx context.Context, namespace, kubeconfigPath, monitoringNamespace string, builtImages, seededUUIDs map[string]string, fleetProvisioner FleetProvisioner, writer io.Writer) (map[string]string, error) {
	// PHASE 7: Go-managed test infrastructure (Issue #1737)
	//
	// Gateway, KubernautAgent, EffectivenessMonitor, AIAnalysis, SignalProcessing,
	// RemediationOrchestrator, WorkflowExecution, Notification, and APIFrontend
	// are all operator-managed now and already ready (PHASE 6 waits for the
	// operator lifecycle contract). No wave synchronization is needed here
	// anymore -- everything below is independent Go-managed test
	// infrastructure that only needs the operator-managed services to already
	// exist (Gateway for event-exporter/AlertManager signal delivery, DataStorage for
	// RBAC checks), which PHASE 6 guarantees by running before this phase.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n🚀 PHASE 7: Go-managed test infrastructure...")
	phase7Start := time.Now()

	// Gateway authentication is opt-in. FullPipeline exercises the default
	// TLS-only signal-source path; test/e2e/gateway covers BR-GATEWAY-036/037
	// with an explicitly authenticated Gateway deployment.
	gatewayToken := ""
	var afRemediateNS map[string]string

	type waveResult struct {
		name string
		err  error
	}
	// MockLLM + MockLLMShadow + Prometheus + AlertManager + event-exporter +
	// APIFrontend-DataStorage-RoleBinding + workflow-job-executor RBAC = 8,
	// plus DEX for the non-Fleet FullPipeline path.
	allResults := make(chan waveResult, 8)
	if shouldDeployDexForAF(fleetProvisioner) {
		allResults = make(chan waveResult, 9)
	}

	// Per-test namespace isolation: generate a unique namespace per remediate
	// scenario so parallel Ginkgo processes never cross-match RRs.
	afRemediateNS = map[string]string{
		"autonomous":  fmt.Sprintf("fp-auto-%s", uuid.New().String()[:8]),
		"interactive": fmt.Sprintf("fp-int-%s", uuid.New().String()[:8]),
		// The cross-namespace A2A target also needs namespace-scoped firing
		// Prometheus evidence before AF's fail-closed severity triage. A real
		// pod crash alone does not ensure its normal alert has fired yet.
		"cross-namespace": fmt.Sprintf("fp-e2e-1292-%s", uuid.New().String()[:8]),
		// interactive-streaming: dedicated namespace for E2E-FP-1189-005, kept
		// distinct from "interactive" above so its memory-eater Deployment's
		// signal fingerprint never collides with E2E-FP-1189-003/-004's. This
		// scenario replays the same 5-turn conversation as -003/-004 but over
		// message/stream (SSE) instead of message/send, closing the gap where
		// no single test combined the real Console-facing SSE transport with a
		// full RO->SP->AA->KA->WE pipeline run to completion (see session notes,
		// issue #1189).
		"interactive-streaming": fmt.Sprintf("fp-ints-%s", uuid.New().String()[:8]),
		// combined-investigate: dedicated namespace for E2E-FP-1853-001
		// (issue #1853 mode 2 — Interactive, single combined message):
		// "create and investigate remediation" triggers kubernaut_remediate
		// chained into kubernaut_investigate within one turn, stopping at RCA.
		"combined-investigate": fmt.Sprintf("fp-comb-%s", uuid.New().String()[:8]),
		// full-interactive: dedicated namespace for E2E-FP-1853-002 (issue
		// #1853 mode 3 — "Full Interactive Remediation" / autonomous-interactive
		// per pkg/apifrontend/agent/prompt.txt): "investigate and fix
		// remediation" triggers kubernaut_investigate chained all the way
		// through discover_workflows -> select_workflow -> watch with no
		// pause for manual workflow selection.
		"full-interactive": fmt.Sprintf("fp-full-%s", uuid.New().String()[:8]),
		// consent-phase2: dedicated namespace for E2E-FP-1899-001 (DD-AF-011,
		// issue #1899 Phase 1->2 consent gate): a single message declares
		// interaction_mode=interactive on kubernaut_investigate then attempts
		// a same-turn fire-and-forget kubernaut_discover_workflows -- the
		// structural gate must block it until a genuine follow-up turn.
		"consent-phase2": fmt.Sprintf("fp-cg2-%s", uuid.New().String()[:8]),
		// consent-phase3: dedicated namespace for E2E-FP-1899-002 (DD-AF-011,
		// issue #1899 Phase 2->3 consent gate): a single message declares
		// interaction_mode=full_remediation (authorizing discover_workflows
		// to auto-chain) then attempts a same-turn fire-and-forget
		// kubernaut_select_workflow -- the gate must block only the 3rd hop.
		"consent-phase3": fmt.Sprintf("fp-cg3-%s", uuid.New().String()[:8]),
		// terminal-1912: dedicated namespace for E2E-FP-1912-001 (issue
		// #1912): a single message declares interaction_mode=
		// full_remediation_autonomous (no checkpoint left blocking) then
		// completes the driver session in the same turn -- driverActive
		// must be cleared alongside it, or a stray text-only turn could be
		// misread as still-active and incorrectly reinvoked.
		"terminal-1912": fmt.Sprintf("fp-t1912-%s", uuid.New().String()[:8]),
		// not-actionable-1918: dedicated namespace for E2E-FP-1918-001 (issue
		// #1918): declares interaction_mode=full_remediation_autonomous (an
		// autonomy grant that would normally leave phase2_blocked=false)
		// against an RR whose target Deployment carries a synthetic Warning
		// K8s Event (reason MOCK_NOT_ACTIONABLE, injected by the 18_ test
		// itself) so deriveSignalName resolves a grounded SignalName that
		// surfaces in KA's prompt, matching the mock-LLM's built-in
		// "not_actionable" scenario -- KA's own RCA reasoning returns
		// is_actionable=false. The harness-enforced actionability gate must
		// force phase2_blocked=true regardless of the declared mode, and the
		// chained kubernaut_discover_workflows attempt that follows must be
		// hard-rejected before it ever reaches KA.
		"not-actionable-1918": fmt.Sprintf("fp-na1918-%s", uuid.New().String()[:8]),
		// standalone-exec-cluster-id: dedicated namespace for E2E-FP-2378-001
		// so its memory-eater signal fingerprint cannot collide with the shared
		// kubernaut-system fixture or other FullPipeline scenarios.
		"standalone-exec-cluster-id": fmt.Sprintf("fp-standalone-2378-%s", uuid.New().String()[:8]),
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
		err := DeployMockLLMInNamespace(ctx, namespace, kubeconfigPath, builtImages[mockLLMServiceName], seededUUIDs, afRemediateNS, writer)
		allResults <- waveResult{"MockLLM", err}
	}()

	// Mock LLM Shadow (alignment evaluation — KA config references mock-llm-shadow:8080)
	go func() {
		err := DeployMockLLMShadowInNamespace(ctx, namespace, kubeconfigPath, builtImages[mockLLMServiceName], writer)
		allResults <- waveResult{"MockLLMShadow", err}
	}()

	// Prometheus + AlertManager: chart-external test infrastructure (ADR-EM-001).
	go func() {
		if err := DeployPrometheusTLS(ctx, monitoringNamespace, kubeconfigPath, writer); err != nil {
			allResults <- waveResult{"Prometheus", err}
			return
		}
		allResults <- waveResult{"Prometheus", nil}
	}()
	go func() {
		if err := DeployAlertManagerTLS(ctx, monitoringNamespace, namespace, kubeconfigPath, gatewayToken, writer); err != nil {
			allResults <- waveResult{"AlertManager", err}
			return
		}
		allResults <- waveResult{"AlertManager", nil}
	}()

	// event-exporter: Gateway is already up (PHASE 6 operator readiness gate).
	go func() {
		err := deployKubernetesEventExporter(ctx, namespace, kubeconfigPath, gatewayToken, writer, true)
		allResults <- waveResult{"event-exporter", err}
	}()

	if shouldDeployDexForAF(fleetProvisioner) {
		// DEX OIDC provider (Issue #1189): test-only IdP that AF authenticates
		// against on the non-Fleet FullPipeline path.
		go func() {
			err := deployDexOIDCProviderForAF(ctx, kubeconfigPath, writer)
			allResults <- waveResult{"DEX", err}
		}()
	}

	// AF persona-to-DEX-group RBAC bindings (Issue #1737 regression fix): the
	// chart creates the kubernaut-tool-<persona> ClusterRoles (PHASE 6 helm
	// install) but deliberately not their bindings (see
	// bindAFPersonaToolClusterRoles doc comment). Only depends on those
	// ClusterRoles existing, not on DEX itself being up yet.
	go func() {
		if fleetProvisioner != nil {
			allResults <- waveResult{"AF-Persona-RBAC-Bindings", nil}
			return
		}
		err := bindAFPersonaToolClusterRoles(ctx, kubeconfigPath, writer)
		allResults <- waveResult{"AF-Persona-RBAC-Bindings", err}
	}()

	// APIFrontend -> DataStorage audit RoleBinding (Issue #1189): the chart's
	// own datastorage-rbac.yaml does not include the apifrontend SA (verified
	// against operator-managed DataStorage RBAC), so this
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
		if err := CreateTestNamespace(ctx, ExecutionNamespace, kubeconfigPath, writer); err != nil {
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
		return nil, fmt.Errorf("PHASE 7 deployments failed: %v", deployErrors)
	}
	if err := EnsureEventExporterGatewayNetworkPolicy(ctx, namespace, kubeconfigPath, writer); err != nil {
		return nil, err
	}

	_, _ = fmt.Fprintf(writer, "✅ PHASE 7 complete (%s)\n", time.Since(phase7Start).Round(time.Second))

	// ═══════════════════════════════════════════════════════════════════════
	return afRemediateNS, nil
}

// CleanupFullPipelineTestResources deletes all kubernaut CR instances and
// test namespaces, leaving the cluster with deployed services but no test
// state. This is called from SynchronizedAfterSuite so retries start clean.
func CleanupFullPipelineTestResources(ctx context.Context, kubeconfigPath string, writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "\n🧹 Cleaning up test-created resources...")
	if err := DeleteOperatorExposureAdapters(ctx, kubeconfigPath, kubernautSystem, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  operator exposure cleanup: %v\n", err)
	}

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
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"delete", kind, "--all-namespaces", "--all", "--ignore-not-found", "--wait=true", "--timeout=120s")
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

	// Delete all test-owned FP/history/phase namespaces. The application and
	// monitoring namespaces do not use these prefixes.
	nsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"get", "namespaces", "-o", "jsonpath={.items[*].metadata.name}")
	nsOut, err := nsCmd.Output()
	if err == nil {
		for _, ns := range strings.Fields(string(nsOut)) {
			if strings.HasPrefix(ns, "fp-") || strings.HasPrefix(ns, "history-2496-") || strings.HasPrefix(ns, "phase-2498-") {
				delCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
					"delete", "namespace", ns, "--ignore-not-found", "--wait=true", "--timeout=120s")
				if delOut, delErr := delCmd.CombinedOutput(); delErr != nil {
					_, _ = fmt.Fprintf(writer, "  ⚠️  namespace %s: %s\n", ns, strings.TrimSpace(string(delOut)))
				} else {
					_, _ = fmt.Fprintf(writer, "  🗑️  namespace %s deleted\n", ns)
				}
			}
		}
	}

	_, _ = fmt.Fprintln(writer, "✅ Test resource cleanup complete")
	deleteCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"delete", "secret", "e2e-2390-snapshot-secret", "-n", ExecutionNamespace, "--ignore-not-found", "--wait=true")
	if out, err := deleteCmd.CombinedOutput(); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  standalone execution fixture cleanup: %s\n", strings.TrimSpace(string(out)))
	} else if trimmed := strings.TrimSpace(string(out)); trimmed != "" && trimmed != "No resources found" {
		_, _ = fmt.Fprintf(writer, "  🗑️  standalone execution fixture: %s\n", trimmed)
	}
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
		if baseCfg.ServiceName == mockLLMServiceName && skipMockLLM() {
			_, _ = fmt.Fprintln(writer, "  ⏭️  Skipping mock-llm build (SKIP_MOCK_LLM is set)")
			continue
		}
		wg.Add(1)
		cfg := baseCfg // capture loop variable
		// Mock LLM doesn't need Go coverage instrumentation
		if cfg.ServiceName != mockLLMServiceName {
			cfg.EnableCoverage = enableCoverage
		}

		go func() {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot

			var imageName string
			var err error
			switch {
			case cfg.ServiceName == mockLLMServiceName && os.Getenv("KUBERNAUT_BUILD_MOCK_LLM") == trueFixture:
				imageName, err = buildMockLLMQualificationImage(ctx, writer)
			case cfg.ServiceName == mockLLMServiceName && strings.TrimSpace(os.Getenv("KUBERNAUT_MOCK_LLM_IMAGE")) != "":
				imageName = strings.TrimSpace(os.Getenv("KUBERNAUT_MOCK_LLM_IMAGE"))
			default:
				imageName, err = BuildImageForKind(ctx, cfg, writer)
			}

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

// buildMockLLMQualificationImage builds only the test-owned mock LLM when a
// qualification profile intentionally reuses published v1.6 RC operands for
// every application service. Mock LLM is not a release artifact and is never
// added to operator relatedImages.
func buildMockLLMQualificationImage(ctx context.Context, writer io.Writer) (string, error) {
	tag := strings.TrimSpace(os.Getenv("KUBERNAUT_MOCK_LLM_TAG"))
	if tag == "" {
		tag = "operator-2492-mock-llm"
	}
	image := "localhost/mock-llm:" + tag
	cmd := exec.CommandContext(ctx, "podman", "build", "-t", image, "-f", filepath.Join(getProjectRoot(), "test", "services", mockLLMServiceName, "go.Dockerfile"), getProjectRoot())
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build test-owned mock-llm image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ mock-llm test image → %s\n", image)
	return image, nil
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
func buildEventExporterManifest(namespace, gatewayToken string, tlsEnabled ...bool) string {
	// BR-GATEWAY-036/037: Add Authorization header to webhook when token is provided
	authHeaderYaml := ""
	if gatewayToken != "" {
		authHeaderYaml = fmt.Sprintf("            Authorization: \"Bearer %s\"\n", gatewayToken)
	}

	gatewayEventEndpoint := fmt.Sprintf("https://gateway-service.%s.svc.cluster.local:8080/api/v1/signals/kubernetes-event", namespace)
	if len(tlsEnabled) > 0 && tlsEnabled[0] {
		// Operator-managed Gateway serves the webhook on HTTPS :8443. Keep
		// the legacy :8080 default for the application-Helm compatibility
		// manifest used by the existing Fleet contract tests.
		gatewayEventEndpoint = fmt.Sprintf("https://gateway-service.%s.svc.cluster.local:8443/api/v1/signals/kubernetes-event", namespace)
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
# ConfigMap: route Warning events from FullPipeline fixture namespaces to Gateway
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
    # The event payload already contains the involved object; Gateway performs
    # authoritative owner resolution. Avoid cluster-wide metadata lookups here
    # so parallel FullPipeline specs do not backlog Warning events past the
    # Gateway freshness guard (BR-E2E-001).
    omitLookup: true
    maxEventAgeSeconds: 300
    kubeQPS: 50
    kubeBurst: 100
    route:
      routes:
        # Forward K8s events from the static and dynamically generated
        # FullPipeline fixture namespaces. The fp-fp-* family is emitted by
        # direct-RR helpers that prefix their test ID; the explicit families
        # cover autonomous, interactive, consent, and standalone journeys.
        # fp-am-* namespaces excluded to prevent K8s event duplication in AlertManager test.
        - match:
            - namespace: "fp-e2e-*"
              receiver: gateway-webhook
            - namespace: "fp-approval-*"
              receiver: gateway-webhook
            - namespace: "fp-mcp-*"
              receiver: gateway-webhook
            - namespace: "fp-fp-*"
              receiver: gateway-webhook
            - namespace: "fp-auto-*"
              receiver: gateway-webhook
            - namespace: "fp-int-*"
              receiver: gateway-webhook
            - namespace: "fp-ints-*"
              receiver: gateway-webhook
            - namespace: "fp-comb-*"
              receiver: gateway-webhook
            - namespace: "fp-full-*"
              receiver: gateway-webhook
            - namespace: "fp-cg2-*"
              receiver: gateway-webhook
            - namespace: "fp-cg3-*"
              receiver: gateway-webhook
            - namespace: "fp-t1912-*"
              receiver: gateway-webhook
            - namespace: "fp-na1918-*"
              receiver: gateway-webhook
            - namespace: "fp-standalone-2378-*"
              receiver: gateway-webhook
          drop:
            - type: "Normal"
    receivers:
      - name: gateway-webhook
        webhook:
          endpoint: "%[3]s"
          headers:
            Content-Type: application/json
          tls:
            caFile: /etc/tls-ca/ca.crt
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
        - name: inter-service-ca
          mountPath: /etc/tls-ca
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
      - name: inter-service-ca
        configMap:
          name: inter-service-ca
 `, namespace, authHeaderYaml, gatewayEventEndpoint)
	return manifest
}

func deployKubernetesEventExporter(ctx context.Context, namespace, kubeconfigPath, gatewayToken string, writer io.Writer, tlsEnabled ...bool) error {
	_, _ = fmt.Fprintln(writer, "  📡 Deploying kubernetes-event-exporter...")
	manifest := buildEventExporterManifest(namespace, gatewayToken, tlsEnabled...)
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// EnsureEventExporterGatewayNetworkPolicy adds the test-owned ingress edge
// omitted by the operator's default-deny Gateway policy. The event-exporter
// is Go-managed test infrastructure, so it does not carry the operator labels
// allowed by kubernaut-gateway. Keep this additive and limited to HTTPS 8443.
func EnsureEventExporterGatewayNetworkPolicy(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernaut-e2e-event-exporter-gateway
  namespace: %s
  labels:
    app: event-exporter
    component: test-infrastructure
    test: e2e
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: event-exporter
          component: test-infrastructure
    ports:
    - protocol: TCP
      port: 8443
`, namespace)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("ensure event-exporter Gateway NetworkPolicy: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ event-exporter Gateway ingress NetworkPolicy ready")
	return nil
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

func fullPipelineReadinessDeployments(fleetProvisioner FleetProvisioner) []string {
	deployments := []string{
		"data-storage",
		"kubernaut-agent",
		"gateway",
		"event-exporter",
		"mock-slack",  // Accepts Slack webhook POSTs so notifications reach terminal phase
		"apifrontend", // Issue #1189: AF as FP signal source
	}
	if shouldDeployDexForAF(fleetProvisioner) {
		deployments = append(deployments, "dex") // Issue #1189: OIDC provider for AF authentication
	}
	return deployments
}

// waitForMonitoringDeploymentsReady waits for the isolated monitoring stack in
// parallel. Prometheus and AlertManager are independent workloads; serial
// rollout waits unnecessarily add one startup duration to the other.
func waitForMonitoringDeploymentsReady(ctx context.Context, namespace, kubeconfigPath string, timeout time.Duration, writer io.Writer) error {
	type readyResult struct {
		name string
		err  error
	}
	deployments := []string{"prometheus", "alertmanager"}
	results := make(chan readyResult, len(deployments))
	for _, deployment := range deployments {
		go func(name string) {
			results <- readyResult{name: name, err: waitForDeployment(ctx, name, namespace, kubeconfigPath, timeout, writer)}
		}(deployment)
	}

	var readinessErrors []error
	for range deployments {
		result := <-results
		if result.err != nil {
			readinessErrors = append(readinessErrors, fmt.Errorf("%s: %w", result.name, result.err))
		}
	}
	if len(readinessErrors) > 0 {
		return errors.Join(readinessErrors...)
	}
	return nil
}

// waitForFullPipelineServicesReady waits for all services to be ready in the cluster.
// All readiness checks run in parallel for faster convergence.
func waitForFullPipelineServicesReady(ctx context.Context, namespace, kubeconfigPath string, fleetProvisioner FleetProvisioner, writer io.Writer) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// List of deployments that must be ready
	deployments := fullPipelineReadinessDeployments(fleetProvisioner)
	if !skipMockLLM() {
		deployments = append(deployments, mockLLMServiceName)
	}

	// Controller pods checked by label (may have different deployment names)
	type controllerCheck struct {
		name     string
		selector string
	}
	controllers := []controllerCheck{
		{"SignalProcessing", "app=signalprocessing"},
		{"RemediationOrchestrator", "app=remediationorchestrator"},
		{"AIAnalysis", "app=aianalysis"},
		{"WorkflowExecution", "app=workflowexecution"},
		{"Notification", "app=notification"},
		{"EffectivenessMonitor", "app=effectivenessmonitor"}, // ADR-EM-001
	}

	// Run all checks in parallel
	totalChecks := len(deployments) + len(controllers)
	results := make(chan struct {
		name string
		err  error
	}, totalChecks)

	// PR #1790 round-15 RCA (run 30633836502, first time fleet's Helm E2E
	// flow ever reached PHASE 8 -- round-14's DA17 fix eliminated the PHASE 6
	// bottleneck that previously masked this): "gateway not ready after 3m
	// RemediationOrchestrator controller not ready after 3m" while all 13
	// other services (including non-fleet-MCP-dependent controllers) became
	// ready well inside 3m. Both gateway's and RO's own /readyz handlers
	// legitimately block on wireFleetOwnerResolution's mcpclient.NewResilient
	// MCP Gateway connection succeeding (pkg/fleet/mcpclient/resilience.go's
	// DefaultResilienceConfig.MaxElapsedTime = 5 minutes) when
	// global.fleet.enabled=true -- fullpipeline never exercises this path at
	// all (fleetProvisioner is nil there, fleet stays disabled), which is why
	// it has never hit this. The NetworkPolicy egress this connection needs
	// was already fixed in #1755/DD-TEST-015
	// (kubernaut.np.mcpGatewayEgress) -- this is a plain timeout-ceiling
	// mismatch, not a missing rule: the harness's own 3-minute poll window
	// was simply never long enough to cover the dependency's own designed
	// 5-minute retry budget, structurally identical to DA17's
	// progressDeadlineSeconds-vs-kubectl-timeout mismatch. 6 minutes (+1min
	// margin over the 5min ceiling) applies uniformly to both checks below
	// (not just gateway/RO) to keep this function simple -- every other
	// service converges in seconds regardless, so this only affects the
	// failure-case ceiling, not typical-case latency.
	const serviceReadyTimeout = 6 * time.Minute

	// Deployment readiness checks
	for _, deplName := range deployments {
		go checkFullPipelineDeployment(ctx, clientset, namespace, deplName, serviceReadyTimeout, writer, results)
	}

	// Controller readiness checks
	for _, ctrl := range controllers {
		go checkFullPipelineController(ctx, clientset, namespace, ctrl.name, ctrl.selector, serviceReadyTimeout, writer, results)
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

func checkFullPipelineDeployment(ctx context.Context, clientset kubernetes.Interface, namespace, name string, timeout time.Duration, writer io.Writer, results chan<- struct {
	name string
	err  error
}) {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s...\n", name)
	pollErr := pollUntilReady(ctx, timeout, 2*time.Second, func() bool {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		return err == nil && deployment.Status.ReadyReplicas >= 1
	})
	if pollErr != nil {
		results <- struct {
			name string
			err  error
		}{name, fmt.Errorf("%s not ready after %s", name, timeout)}
		return
	}
	_, _ = fmt.Fprintf(writer, "  ✅ %s ready\n", name)
	results <- struct {
		name string
		err  error
	}{name, nil}
}

func checkFullPipelineController(ctx context.Context, clientset kubernetes.Interface, namespace, name, selector string, timeout time.Duration, writer io.Writer, results chan<- struct {
	name string
	err  error
}) {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for %s controller...\n", name)
	pollErr := pollUntilReady(ctx, timeout, 2*time.Second, func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		return err == nil && anyPodReady(pods)
	})
	if pollErr != nil {
		results <- struct {
			name string
			err  error
		}{name, fmt.Errorf("%s controller not ready after %s", name, timeout)}
		return
	}
	_, _ = fmt.Fprintf(writer, "  ✅ %s controller ready\n", name)
	results <- struct {
		name string
		err  error
	}{name, nil}
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
		"deployment/data-storage",
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
			LabelSelector: "app=data-storage",
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

// waitForDeploymentRollout blocks until the named Deployment's rollout
// completes (all replicas updated and available) or timeout elapses.
func waitForDeploymentRollout(ctx context.Context, namespace, kubeconfigPath, deploymentName string, timeout time.Duration, writer io.Writer) error {
	rolloutCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/"+deploymentName,
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		fmt.Sprintf("--timeout=%s", timeout))
	rolloutCmd.Stdout = writer
	rolloutCmd.Stderr = writer
	if err := rolloutCmd.Run(); err != nil {
		return fmt.Errorf("%s rollout not ready: %w", deploymentName, err)
	}
	return nil
}

// waitForServiceEndpointReady blocks until the named Service has at least
// one ready backing address, or timeout elapses.
//
// `kubectl rollout status` reports success the instant a Deployment's
// replica first passes its readiness probe -- but the Service's own
// Endpoints/EndpointSlice update (via the endpoint-controller watching that
// pod-readiness transition) and kube-proxy's iptables DNAT reprogramming
// are both separate, asynchronous steps with their own latency. Confirmed
// via CI (run 31842923960): waitForDeploymentRollout("authwebhook")
// returned success, yet the very next kubectl apply (routed through
// apiserver -> actiontype.validate.kubernaut.ai -> authwebhook Service)
// still failed with "dial tcp ...:443: connect: connection refused" --
// the Service had no programmed endpoint yet. Polling the Service's own
// Endpoints closes that specific gap instead of guessing at a fixed sleep.
func waitForServiceEndpointReady(ctx context.Context, namespace, kubeconfigPath, serviceName string, timeout time.Duration, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		slices, listErr := clientset.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "kubernetes.io/service-name=" + serviceName,
		})
		if listErr == nil {
			for _, slice := range slices.Items {
				readyAddresses := 0
				for _, endpoint := range slice.Endpoints {
					if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
						continue
					}
					readyAddresses += len(endpoint.Addresses)
				}
				if readyAddresses > 0 {
					_, _ = fmt.Fprintf(writer, "  ✅ Service %s has %d ready endpoint(s)\n", serviceName, readyAddresses)
					return nil
				}
			}
		}
		if ctxErr := sleepOrDone(ctx, 2*time.Second); ctxErr != nil {
			return ctxErr
		}
	}
	return fmt.Errorf("service %s had no ready endpoints within %s", serviceName, timeout)
}

// sleepOrDone sleeps for d, returning early with ctx.Err() if ctx is
// cancelled first.
func sleepOrDone(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
