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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// API Frontend E2E Infrastructure
//
// Deploys the AF E2E stack (KA + DS + PostgreSQL + Redis + mock-LLM + DEX + CRDs)
// in a single Kind cluster. Follows the same patterns as fullpipeline_e2e.go.
//
// Port Allocation (DD-TEST-001):
//   AF HTTPS:   NodePort 30443, host port 18443
//   AF Health:  NodePort 30081, host port 18081
//   AF Metrics: NodePort 9190
//   DEX:        host port 5556
//
// Kind Config: test/infrastructure/kind-kubernautagent-config.yaml
// Kubeconfig:  ~/.kube/apifrontend-e2e-config
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"gopkg.in/yaml.v3"
)

const (
	// AFDefaultClusterName is the Kind cluster name for apifrontend E2E tests.
	AFDefaultClusterName = "apifrontend-e2e"
	// AFDefaultNamespace is the Kubernetes namespace for AF E2E workloads.
	AFDefaultNamespace = kubernautSystem
)

// afE2EHostPortOffset returns the optional host-port offset used for isolated
// AF E2E runs. The offset changes only Kind's host bindings; service and
// NodePort values inside the cluster remain at their established defaults.
func afE2EHostPortOffset() (int, error) {
	raw := strings.TrimSpace(os.Getenv("AF_E2E_HOST_PORT_OFFSET"))
	if raw == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid AF_E2E_HOST_PORT_OFFSET %q: %w", raw, err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("AF_E2E_HOST_PORT_OFFSET must be non-negative")
	}
	return offset, nil
}

func afE2EHostPort(defaultPort int) int {
	offset, err := afE2EHostPortOffset()
	if err != nil {
		return defaultPort
	}
	return defaultPort + offset
}

// SetupAPIFrontendE2EInfrastructure builds the standalone AF image set once,
// deploys the local-mode AF cluster, and returns the images so the Fleet-mode
// cluster can reuse the same artifacts without rebuilding them.
func SetupAPIFrontendE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) (map[string]string, error) {
	hostPortOffset, err := afE2EHostPortOffset()
	if err != nil {
		return nil, err
	}
	images, err := BuildAPIFrontendE2EImages(ctx, writer)
	if err != nil {
		return nil, err
	}
	options := apiFrontendE2EOptions{
		KindConfigPath: "test/infrastructure/kind-kubernautagent-config.yaml",
		HostPortOffset: hostPortOffset,
	}
	if err := setupAPIFrontendE2EInfrastructure(ctx, clusterName, kubeconfigPath, namespace, images, options, writer); err != nil {
		return images, err
	}
	return images, nil
}

// SetupAPIFrontendFleetE2EInfrastructure deploys the standalone AF services
// plus the minimal hub-only Fleet core into a second isolated Kind cluster
// (DD-TEST-019).
// Images are shared with SetupAPIFrontendE2EInfrastructure; fmcImage is built
// or resolved separately because local-mode AF does not need FMC.
func SetupAPIFrontendFleetE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, images map[string]string, fmcImage string, writer io.Writer) error {
	return setupAPIFrontendE2EInfrastructure(ctx, clusterName, kubeconfigPath, namespace, images, apiFrontendE2EOptions{
		KindConfigPath: "test/infrastructure/kind-apifrontend-fleet-config.yaml",
		FleetEnabled:   true,
		FleetImage:     fmcImage,
	}, writer)
}

// BuildAPIFrontendE2EImages resolves the image set shared by the isolated
// local and Fleet AF clusters. Build the coverage-instrumented AF image after
// the three supporting images so Podman does not run four dependency-heavy Go
// image builds concurrently on local developers' machines.
func BuildAPIFrontendE2EImages(ctx context.Context, writer io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintln(writer, "\n📦 Resolving standalone AF E2E images...")
	type buildResult struct {
		name  string
		image string
		err   error
	}
	results := make(chan buildResult, 3)
	for _, svc := range []struct {
		name       string
		image      string
		dockerfile string
		buildCtx   string
	}{
		{"datastorage", "datastorage", "docker/data-storage.Dockerfile", ""},
		{"kubernautagent", "kubernautagent", "docker/kubernautagent.Dockerfile", ""},
		{"mock-llm", "mock-llm", "test/services/mock-llm/go.Dockerfile", ""},
	} {
		go func(name, image, dockerfile, buildCtx string) {
			cfg := E2EImageConfig{ServiceName: name, ImageName: image, DockerfilePath: dockerfile, BuildContextPath: buildCtx}
			img, err := BuildImageForKind(ctx, cfg, writer)
			results <- buildResult{name, img, err}
		}(svc.name, svc.image, svc.dockerfile, svc.buildCtx)
	}
	images := make(map[string]string, 4)
	for range 3 {
		result := <-results
		if result.err != nil {
			return nil, fmt.Errorf("failed to build %s: %w", result.name, result.err)
		}
		images[result.name] = result.image
		_, _ = fmt.Fprintf(writer, "  %s: %s\n", result.name, result.image)
	}
	// DD-TEST-007: AF is built locally with GOFLAGS=-cover after the supporting images.
	afImage, err := BuildAFImage(ctx, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to build apifrontend: %w", err)
	}
	images["apifrontend"] = afImage
	_, _ = fmt.Fprintf(writer, "  apifrontend: %s\n", afImage)
	return images, nil
}

type apiFrontendE2EOptions struct {
	KindConfigPath string
	HostPortOffset int
	FleetEnabled   bool
	FleetImage     string
}

func setupAPIFrontendE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, images map[string]string, options apiFrontendE2EOptions, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if options.FleetEnabled {
		_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Setup (hub-only Fleet mode)")
	} else {
		_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Setup (local mode)")
	}
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getProjectRoot()
	coverdataDir := filepath.Join(projectRoot, "coverdata")
	if err := os.MkdirAll(coverdataDir, 0o777); err != nil { //nolint:gosec // G301: world-readable dir needed for Kind volume mount
		_, _ = fmt.Fprintf(writer, "  WARNING: failed to create coverdata dir: %v\n", err)
	}
	imageRegistry := os.Getenv("IMAGE_REGISTRY")
	imageTag := os.Getenv("IMAGE_TAG")
	if imageRegistry != "" && imageTag != "" {
		_, _ = fmt.Fprintf(writer, "  Registry mode: %s/*:%s\n", imageRegistry, imageTag)
	} else {
		_, _ = fmt.Fprintln(writer, "  Reusing the shared AF E2E image set")
	}
	if options.KindConfigPath == "" {
		return fmt.Errorf("kind config path is required for AF E2E setup")
	}
	_, _ = fmt.Fprintf(writer, "  Cluster: %s (Kind config: %s, host port offset: %d)\n", clusterName, options.KindConfigPath, options.HostPortOffset)
	if len(images) == 0 {
		return fmt.Errorf("api frontend E2E image set is required")
	}
	for _, name := range []string{"datastorage", "kubernautagent", "mock-llm", "apifrontend"} {
		if images[name] == "" {
			return fmt.Errorf("api frontend E2E image %q is required", name)
		}
	}
	if options.FleetEnabled && options.FleetImage == "" {
		return fmt.Errorf("fleet metadata cache image is required for Fleet AF E2E setup")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 2: Creating Kind cluster...")
	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                options.KindConfigPath,
		WaitTimeout:               "5m",
		DeleteExisting:            true,
		CleanupOrphanedContainers: true,
		UsePodman:                 true,
		ProjectRootAsWorkingDir:   true,
		HostPortOffset:            options.HostPortOffset,
	}
	if err := CreateKindClusterWithConfig(ctx, opts, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into Kind
	// ═══════════════════════════════════════════════════════════════════════
	if imageRegistry != "" {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading AF image into Kind (coverage build); others pull from GHCR...")
		if err := LoadImageToKind(ctx, images["apifrontend"], "apifrontend", clusterName, writer); err != nil {
			return fmt.Errorf("failed to load apifrontend image: %w", err)
		}
		_, _ = fmt.Fprintln(writer, "  apifrontend loaded")
	} else {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading images into Kind...")
		for name, img := range images {
			if err := LoadImageToKind(ctx, img, name, clusterName, writer); err != nil {
				return fmt.Errorf("failed to load %s image: %w", name, err)
			}
			_, _ = fmt.Fprintf(writer, "  %s loaded\n", name)
		}
		if options.FleetEnabled {
			if err := LoadImageToKind(ctx, options.FleetImage, "fleetmetadatacache", clusterName, writer); err != nil {
				return fmt.Errorf("failed to load Fleet Metadata Cache image: %w", err)
			}
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy kubernaut stack (DS + KA + dependencies)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 4: Deploying kubernaut stack...")

	if err := CreateTestNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Generating inter-service TLS...")
	if _, err := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
	}

	var fleetOptions *FleetHelmOptions
	if options.FleetEnabled {
		_, _ = fmt.Fprintln(writer, "  🌐 Provisioning hub-only Fleet core from the FMC E2E setup...")
		fleetOpts, fleetErr := SetupFMCHubOnlyInfrastructure(ctx, clusterName, kubeconfigPath, namespace, options.FleetImage, writer)
		fleetOptions = fleetOpts
		if fleetErr != nil {
			return fmt.Errorf("hub-only Fleet core setup failed: %w", fleetErr)
		}
	}

	_, _ = fmt.Fprintln(writer, "  Deploying DataStorage stack (PostgreSQL + Redis + migrations + DS)...")
	if err := DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, images["datastorage"], 30089, writer); err != nil {
		return fmt.Errorf("DataStorage stack deploy failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Binding apifrontend SA to data-storage-client role...")
	if err := afBindServiceAccountToDSClient(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("AF DS client RBAC failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Deploying mock-LLM...")
	mockLLMClusterID := ""
	if options.FleetEnabled {
		mockLLMClusterID = fleetHubClusterID
	}
	if err := afDeployMockLLM(ctx, kubeconfigPath, images["mock-llm"], mockLLMClusterID, writer); err != nil {
		return fmt.Errorf("mock-LLM deploy failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent RBAC...")
	if err := DeployKubernautAgentServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("KA RBAC failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent...")
	if err := DeployKubernautAgentOnly(ctx, clusterName, kubeconfigPath, namespace, images["kubernautagent"], false, writer); err != nil {
		return fmt.Errorf("KA deploy failed: %w", err)
	}

	certDir := ""
	if !options.FleetEnabled {
		certDir = os.Getenv("AF_E2E_CERT_DIR")
	}
	if certDir == "" {
		certDir = filepath.Join(os.TempDir(), "apifrontend-e2e-certs", clusterName)
	}
	if err := AFGenerateCerts(ctx, certDir, writer); err != nil {
		return fmt.Errorf("failed to generate AF certs: %w", err)
	}
	if err := AFCreateTLSSecrets(ctx, kubeconfigPath, namespace, certDir, writer); err != nil {
		return fmt.Errorf("failed to create AF TLS secrets: %w", err)
	}
	_ = os.Setenv("AF_E2E_CERT_DIR", certDir)
	_ = os.Setenv("CERT_DIR", certDir)
	_ = os.Setenv("AF_E2E_CA_CERT", filepath.Join(certDir, "ca.crt"))
	if !options.FleetEnabled && os.Getenv("AF_E2E_DEX_URL") == "" {
		_ = os.Setenv("AF_E2E_DEX_URL", fmt.Sprintf("https://localhost:%d/dex", afE2EHostPort(5556)))
	}
	_ = os.Setenv("KUBECONFIG", kubeconfigPath)

	_, _ = fmt.Fprintln(writer, "Phase 5: Deploy AF (programmatic)")

	if err := afInstallCRDs(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	if !options.FleetEnabled {
		if err := afDeployDex(ctx, kubeconfigPath, namespace, writer); err != nil {
			return fmt.Errorf("failed to deploy Dex: %w", err)
		}
	}

	if err := afDeployE2ERBAC(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to deploy AF RBAC: %w", err)
	}
	if options.FleetEnabled {
		if err := afDeployFleetRBAC(ctx, kubeconfigPath, namespace, writer); err != nil {
			return fmt.Errorf("failed to deploy Fleet AF RBAC: %w", err)
		}
	}

	// Seed DS with action types + workflows so kubernaut_list_workflows returns
	// a non-empty catalog. Must run after afDeployE2ERBAC (creates the apifrontend SA).
	_, _ = fmt.Fprintln(writer, "  Seeding DS action types + workflows for AF E2E...")
	// #1661 Phase 55: DS's Postgres-backed POST /api/v1/action-types endpoint was
	// removed (DD-WORKFLOW-018); action types are now seeded exclusively as CRDs for
	// DS's informer-backed cache. Workflows here seed via SeedWorkflowsViaKubectlApply
	// (real AuthWebhook admission), so this file no longer touches DS's REST API at all.
	var seedErr error
	if seedErr = SeedActionTypesViaCRD(ctx, kubeconfigPath, namespace, writer); seedErr != nil {
		return fmt.Errorf("seed action types (CRD): %w", seedErr)
	}
	testWorkflows := GetKAE2ETestWorkflows()
	// #1661 Phase 56 (discovered gap): this suite runs with no live AuthWebhook
	// (unlike fullpipeline/fleet), so SeedWorkflowsViaKubectlApply's wait on
	// .status.workflowId can never resolve -- use the direct-CRD-creation path
	// instead, which computes the same deterministic UUID and stamps status
	// itself (pkg/shared/contenthash).
	if _, seedErr = SeedWorkflowsViaDirectCRDCreationFromKubeconfig(ctx, kubeconfigPath, namespace, testWorkflowsToSeedSpecs(testWorkflows), writer); seedErr != nil {
		return fmt.Errorf("seed workflows: %w", seedErr)
	}

	afImage := images["apifrontend"]
	var deployErr error
	if options.FleetEnabled {
		deployErr = deployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, true, fleetOptions, writer)
	} else {
		deployErr = DeployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, true, writer)
	}
	if deployErr != nil {
		return fmt.Errorf("failed to deploy AF service: %w", deployErr)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for rollouts + enable JWT on KA
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 6: Waiting for deployments...")

	deployments := []string{"datastorage", "kubernaut-agent", "mock-llm", "apifrontend"}
	if !options.FleetEnabled {
		deployments = append(deployments, "dex")
	}
	for _, deploy := range deployments {
		_, _ = fmt.Fprintf(writer, "  Waiting for %s...\n", deploy)
		timeout := 120 * time.Second
		if deploy == "datastorage" {
			timeout = 180 * time.Second
		}
		if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, deploy, timeout, writer); err != nil {
			return fmt.Errorf("%s not ready: %w", deploy, err)
		}
	}

	if options.FleetEnabled {
		_, _ = fmt.Fprintln(writer, "  Configuring KA for Keycloak identity and Fleet MCP routing...")
		if err := afPatchKubernautAgentFleetConfig(ctx, kubeconfigPath, namespace, fleetOptions, writer); err != nil {
			return fmt.Errorf("KA Fleet configuration failed: %w", err)
		}
	} else {
		_, _ = fmt.Fprintln(writer, "  Patching KA for JWT delegation (DEX is now available)...")
		if err := afPatchKAJWTAudience(ctx, kubeconfigPath, namespace, writer); err != nil {
			_, _ = fmt.Fprintf(writer, "  WARNING: KA JWT audience patch failed (non-fatal): %v\n", err)
		}
	}
	_, _ = fmt.Fprintln(writer, "  Waiting for kubernaut-agent restart...")
	if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, "kubernaut-agent", 120*time.Second, writer); err != nil {
		return fmt.Errorf("kubernaut-agent not ready after JWT patch: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if options.FleetEnabled {
		_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Ready: APIFrontend + Fleet core")
		_, _ = fmt.Fprintln(writer, "  Fleet cluster ID: hub (registered through the EAIGW Gateway)")
	} else {
		_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Ready: local APIFrontend stack")
	}
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// BuildAFImage builds the apifrontend container image locally with coverage
// instrumentation (GOFLAGS=-cover).
func BuildAFImage(ctx context.Context, writer io.Writer) (string, error) {
	cfg := E2EImageConfig{
		ServiceName:    "apifrontend",
		ImageName:      "apifrontend",
		DockerfilePath: "docker/apifrontend.Dockerfile",
		EnableCoverage: true,
	}
	return BuildImageForKind(ctx, cfg, writer)
}

// AFGenerateCerts runs the AF cert generation script.
func AFGenerateCerts(ctx context.Context, certDir string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	script := projectRoot + "/deploy/apifrontend/overlays/e2e/generate-certs.sh"
	cmd := exec.CommandContext(ctx, "bash", script, certDir) //nolint:gosec // G204: test infra, script path from project root
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate-certs.sh failed: %w", err)
	}
	return nil
}

// AFCreateTLSSecrets creates the TLS secrets required by AF from the cert directory.
func AFCreateTLSSecrets(ctx context.Context, kubeconfigPath, namespace, certDir string, writer io.Writer) error {
	secrets := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{"apifrontend-tls", "tls.crt", "tls.key"},
	}
	for _, s := range secrets {
		dryRunCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
			"create", "secret", "tls", s.name,
			"--cert="+filepath.Join(certDir, s.certFile),
			"--key="+filepath.Join(certDir, s.keyFile),
			"-n", namespace, "--dry-run=client", "-o", "yaml")
		yamlData, err := dryRunCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to generate TLS secret %s: %w", s.name, err)
		}
		applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(string(yamlData))
		applyCmd.Stdout = writer
		applyCmd.Stderr = writer
		if err := applyCmd.Run(); err != nil {
			return fmt.Errorf("failed to apply TLS secret %s: %w", s.name, err)
		}
	}

	dryRunCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"create", "secret", "generic", "apifrontend-ca",
		"--from-file=ca.crt="+filepath.Join(certDir, "ca.crt"),
		"-n", namespace, "--dry-run=client", "-o", "yaml")
	yamlData, err := dryRunCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate CA secret: %w", err)
	}
	applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(string(yamlData))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	return applyCmd.Run()
}

// WaitForDeploymentRollout waits for a deployment to become ready.
// On failure, it collects pod-level diagnostics (describe, logs, events) for triage.
func WaitForDeploymentRollout(ctx context.Context, kubeconfigPath, namespace, name string, timeout time.Duration, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"rollout", "status", "deployment/"+name, "-n", namespace,
		fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		collectDeploymentDiagnostics(ctx, kubeconfigPath, namespace, name, writer)
		return fmt.Errorf("deployment/%s not ready: %w", name, err)
	}
	return nil
}

// CollectAFE2EBinaryCoverage collects Go coverage data for the AF binary.
func CollectAFE2EBinaryCoverage(clusterName string, writer io.Writer) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir for kubeconfig: %w", err)
	}
	kcPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)

	return CollectE2EBinaryCoverage(E2ECoverageOptions{
		ServiceName:    "apifrontend",
		ClusterName:    clusterName,
		DeploymentName: "apifrontend",
		Namespace:      kubernautSystem,
		KubeconfigPath: kcPath,
	}, writer)
}

// DeployAPIFrontendService deploys the local-mode AF ConfigMaps, Deployment, and NodePort Service.
// When enableCoverage is true, the pod includes GOCOVERDIR=/coverdata env var and
// a hostPath volume mount (DD-TEST-007). When false (e.g., FP cluster), coverage
// instrumentation is omitted.
func DeployAPIFrontendService(ctx context.Context, kubeconfigPath, namespace, afImage string, enableCoverage bool, writer io.Writer) error {
	return deployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, enableCoverage, nil, writer)
}

func deployAPIFrontendService(ctx context.Context, kubeconfigPath, namespace, afImage string, enableCoverage bool, fleetOptions *FleetHelmOptions, writer io.Writer) error {
	projectRoot := getProjectRoot()
	configData, err := os.ReadFile(filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "config.yaml")) //nolint:gosec // G304
	if err != nil {
		return fmt.Errorf("failed to read config.yaml: %w", err)
	}
	if fleetOptions != nil {
		configData, err = buildAPIFrontendFleetConfig(configData, namespace, fleetOptions)
		if err != nil {
			return fmt.Errorf("failed to configure Fleet-mode APIFrontend: %w", err)
		}
	}

	pullPolicy := "IfNotPresent"
	if os.Getenv("IMAGE_REGISTRY") != "" {
		pullPolicy = "Always"
	}

	securityCtx := ""
	if enableCoverage {
		securityCtx = `      securityContext:
        runAsUser: 0
        runAsGroup: 0`
	}

	coverageEnv := ""
	if enableCoverage {
		coverageEnv = `            - name: GOCOVERDIR
              value: /coverdata`
	}

	coverageMount := ""
	if enableCoverage {
		coverageMount = `            - name: coverdata
              mountPath: /coverdata`
	}

	healthNodePort := ""
	if enableCoverage {
		healthNodePort = "\n      nodePort: 30081"
	}

	coverageVolume := ""
	if enableCoverage {
		coverageVolume = `        - name: coverdata
          hostPath:
            path: /coverdata
            type: DirectoryOrCreate`
	}
	if fleetOptions != nil {
		if fleetOptions.OAuth2CredentialsSecret == "" {
			return fmt.Errorf("fleet OAuth2 credentials secret is required for APIFrontend")
		}
		coverageMount += fmt.Sprintf(`
            - name: fleet-oauth2-credentials
              mountPath: /etc/apifrontend/%s
              readOnly: true`, fleetOptions.OAuth2CredentialsSecret)
		coverageVolume += fmt.Sprintf(`
        - name: fleet-oauth2-credentials
          secret:
            secretName: %s`, fleetOptions.OAuth2CredentialsSecret)
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: apifrontend-llm-key
  namespace: %[1]s
type: Opaque
stringData:
  llm-api-key: "mock-key"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: apifrontend-config
  namespace: %[1]s
data:
  config.yaml: |
%[2]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: apifrontend
  namespace: %[1]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: apifrontend
  template:
    metadata:
      labels:
        app: apifrontend
    spec:
      serviceAccountName: apifrontend
      automountServiceAccountToken: true
%[3]s
      containers:
        - name: apifrontend
          image: %[4]s
          imagePullPolicy: %[5]s
          ports:
            - name: https
              containerPort: 8443
            - name: metrics
              containerPort: 9090
            - name: health
              containerPort: 8081
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
%[6]s
          volumeMounts:
            - name: config
              mountPath: /etc/apifrontend
              readOnly: true
            - name: tls-certs
              mountPath: /etc/apifrontend/tls
              readOnly: true
            - name: inter-service-ca
              mountPath: /etc/apifrontend/inter-service-ca
              readOnly: true
            - name: llm-credentials
              mountPath: /etc/apifrontend/llm-credentials
              readOnly: true
%[7]s
          readinessProbe:
            httpGet:
              path: /readyz
              port: health
            initialDelaySeconds: 10
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 30
            periodSeconds: 15
          resources:
            requests:
              memory: 64Mi
              cpu: 50m
            limits:
              memory: 256Mi
              cpu: 500m
      volumes:
        - name: config
          configMap:
            name: apifrontend-config
            items:
              - key: config.yaml
                path: config.yaml
        - name: tls-certs
          secret:
            secretName: apifrontend-tls
            optional: false
        - name: inter-service-ca
          configMap:
            name: inter-service-ca
        - name: llm-credentials
          secret:
            secretName: apifrontend-llm-key
%[8]s
---
apiVersion: v1
kind: Service
metadata:
  name: apifrontend
  namespace: %[1]s
spec:
  type: NodePort
  ports:
    - name: https
      port: 8443
      targetPort: https
      nodePort: 30443
    - name: metrics
      port: 9090
      targetPort: metrics
    - name: health
      port: 8081
      targetPort: health%[9]s
  selector:
    app: apifrontend
`, namespace, indentYAMLLines(string(configData), 4), securityCtx, afImage, pullPolicy,
		coverageEnv, coverageMount, coverageVolume, healthNodePort)
	return kubectlApplyStdinAF(ctx, kubeconfigPath, manifest, writer)
}

func buildAPIFrontendFleetConfig(baseConfig []byte, namespace string, fleetOptions *FleetHelmOptions) ([]byte, error) {
	if fleetOptions == nil {
		return baseConfig, nil
	}
	if fleetOptions.MCPGatewayEndpoint == "" || fleetOptions.OAuth2TokenURL == "" || fleetOptions.OAuth2CredentialsSecret == "" {
		return nil, fmt.Errorf("fleet MCP endpoint, token URL, and credentials secret are required")
	}
	var config map[string]any
	if err := yaml.Unmarshal(baseConfig, &config); err != nil {
		return nil, fmt.Errorf("parse AF E2E config: %w", err)
	}
	authConfig, ok := config["auth"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("api frontend E2E config must contain an auth mapping")
	}
	issuerURL := "https://keycloak:8443/realms/kubernaut-demo"
	authConfig["issuerURL"] = issuerURL
	authConfig["jwksURL"] = fmt.Sprintf("https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs", namespace)
	authConfig["audience"] = "kubernaut-apifrontend"

	fmcNamespace := fleetOptions.FleetMetadataCacheNamespace
	if fmcNamespace == "" {
		fmcNamespace = namespace
	}
	config["fleet"] = map[string]any{
		"enabled":            true,
		"backend":            string(fleet.BackendFMC),
		"endpoint":           fmt.Sprintf("https://fleetmetadatacache-service.%s.svc.cluster.local:8080", fmcNamespace),
		"mcpGatewayEndpoint": fleetOptions.MCPGatewayEndpoint,
		"mcpGatewayType":     fleetOptions.MCPGatewayType,
		"namespace":          namespace,
		"tlsCAFile":          "/etc/apifrontend/inter-service-ca/ca.crt",
		"oauth2": map[string]any{
			"enabled":              true,
			"tokenURL":             fleetOptions.OAuth2TokenURL,
			"credentialsSecretRef": fleetOptions.OAuth2CredentialsSecret,
			"scopes":               fleetOptions.OAuth2Scopes,
			"tlsCAFile":            "/etc/apifrontend/inter-service-ca/ca.crt",
		},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal Fleet-mode AF config: %w", err)
	}
	return encoded, nil
}

// personaOrder is the fixed, deterministic rendering order for the 6
// per-persona ClusterRoles/ClusterRoleBindings PersonaToolClusterRolesYAML
// generates. Must match afPersonaGroupNames (fullpipeline_e2e_helm.go) and
// charts/kubernaut/values.yaml's apifrontend.config.rbac.personas keys.
var personaOrder = []string{
	"sre", "ai-orchestrator", "cicd", "observability", "l3-audit", "remediation-approver",
}

// PersonaToolClusterRolesYAML generates the 6 per-persona ClusterRoles (with
// kubernaut.ai/tools verb=use resourceNames) and 6 ClusterRoleBindings
// (mapping DEX OIDC groups to the ClusterRoles).
//
// Tool lists are derived directly from charts/kubernaut/values.yaml's
// apifrontend.config.rbac.personas (via LoadPersonaToolsFromValuesYAML)
// rather than hand-copied literals. This function used to embed its own
// copy of each persona's tool list; that copy silently drifted out of sync
// for 5 of 6 personas as values.yaml gained tools across #1367/#1372/#1869
// with zero build/lint signal, breaking test/e2e/apifrontend's
// E2E-KA-1418-001/002 (kubernaut_complete_no_action as sre) on SAR denial
// (incident found 2026-08-03, regression-pinned by
// UT-INFRA-RBAC-002 in rbac_parity_test.go). Deriving from values.yaml
// eliminates this whole class of drift permanently.
//
// Used by AF-only E2E deployments (see afDeployE2ERBAC). Full-pipeline E2E
// does not call this: it deploys the real chart via `helm install` and only
// adds the ClusterRoleBindings (see bindAFPersonaToolClusterRoles,
// fullpipeline_e2e_helm.go), so it was never subject to this drift.
func PersonaToolClusterRolesYAML() (string, error) {
	personaTools, err := LoadPersonaToolsFromValuesYAML()
	if err != nil {
		return "", fmt.Errorf("failed to load persona tool RBAC from values.yaml: %w", err)
	}

	var b strings.Builder
	for _, name := range personaOrder {
		tools, ok := personaTools[name]
		if !ok {
			return "", fmt.Errorf(
				"persona %q (afPersonaGroupNames) not found in values.yaml apifrontend.config.rbac.personas", name)
		}

		fmt.Fprintf(&b, `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-tool-%s
rules:
  - apiGroups: ["kubernaut.ai"]
    resources: ["tools"]
    verbs: ["use"]
    resourceNames:
`, name)
		for _, t := range tools {
			fmt.Fprintf(&b, "      - %q\n", t)
		}

		fmt.Fprintf(&b, `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-tool-%s-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-tool-%s
subjects:
  - kind: Group
    name: %s
    apiGroup: rbac.authorization.k8s.io
`, name, name, name)
	}

	return b.String(), nil
}

// LoadPersonaToolsFromValuesYAML reads charts/kubernaut/values.yaml and
// returns apifrontend.config.rbac.personas as persona name -> tool list --
// the single source of truth PersonaToolClusterRolesYAML renders from.
// Exported for reuse by test/e2e/apifrontend's persona ACL-matrix test
// (#1827), so that test asserts against the same source of truth instead
// of re-declaring a third hand-copied tool list.
func LoadPersonaToolsFromValuesYAML() (map[string][]string, error) {
	path := filepath.Join(getProjectRoot(), "charts", "kubernaut", "values.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // G304: known project path
	if err != nil {
		return nil, fmt.Errorf("failed to read values.yaml: %w", err)
	}
	var parsed struct {
		APIFrontend struct {
			Config struct {
				RBAC struct {
					Personas map[string][]string `yaml:"personas"`
				} `yaml:"rbac"`
			} `yaml:"config"`
		} `yaml:"apifrontend"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse values.yaml: %w", err)
	}
	if len(parsed.APIFrontend.Config.RBAC.Personas) == 0 {
		return nil, fmt.Errorf("values.yaml apifrontend.config.rbac.personas is empty or missing")
	}
	return parsed.APIFrontend.Config.RBAC.Personas, nil
}

// ============================================================================
// AF-specific unexported helpers
// ============================================================================

func afBindServiceAccountToDSClient(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: apifrontend-ds-client
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
- kind: ServiceAccount
  name: apifrontend
  namespace: %s
`, namespace)
	return kubectlApplyStdinAF(ctx, kubeconfigPath, manifest, writer)
}

func afDeployMockLLM(ctx context.Context, kubeconfigPath, mockLLMImage, clusterID string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	mockLLMManifest := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "mock-llm.yaml")

	data, err := os.ReadFile(mockLLMManifest) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read mock-llm.yaml: %w", err)
	}

	manifest := strings.ReplaceAll(string(data), "ghcr.io/jordigilh/kubernaut/mock-llm:pr-1161", mockLLMImage)
	manifest = strings.ReplaceAll(manifest, "imagePullPolicy: Always", "imagePullPolicy: IfNotPresent")
	manifest = strings.ReplaceAll(manifest, "__HUB_CLUSTER_ID__", clusterID)

	return kubectlApplyStdinAF(ctx, kubeconfigPath, manifest, writer)
}

func afPatchKAJWTAudience(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	getCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "get", "configmap", "kubernaut-agent-config",
		"-o", "jsonpath={.data.config\\.yaml}")
	out, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("get KA config: %w", err)
	}
	currentConfig := string(out)

	jwtBlock := `  jwtProviders:
    - name: dex-e2e
      issuer: "https://dex:5556/dex"
      jwksURL: "https://dex:5556/dex/keys"
      audience: "kubernaut-apifrontend"
      tlsCaFile: /etc/tls-ca/ca.crt
      claimMappings:
        username: "email"
        groups: "groups"`

	anchor := "rateLimitPerUser: 100" // keep in sync with test/infrastructure/kubernautagent.go
	if !strings.Contains(currentConfig, anchor) {
		return fmt.Errorf("cannot find anchor %q in KA config", anchor)
	}
	newConfig := strings.Replace(currentConfig, anchor, anchor+"\n"+jwtBlock, 1)

	patchJSON := fmt.Sprintf(`{"data":{"config.yaml":%q}}`, newConfig)
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "patch", "configmap", "kubernaut-agent-config",
		"--type=merge", "-p", patchJSON)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("patch KA config: %w", err)
	}
	restartCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "rollout", "restart", "deployment/kubernaut-agent")
	restartCmd.Stdout = writer
	restartCmd.Stderr = writer
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("restart KA: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  KA JWT audience patched to accept AF tokens")
	return nil
}

func afPatchKubernautAgentFleetConfig(ctx context.Context, kubeconfigPath, namespace string, fleetOptions *FleetHelmOptions, writer io.Writer) error {
	if fleetOptions == nil || fleetOptions.OAuth2CredentialsSecret == "" {
		return fmt.Errorf("fleet gateway options and OAuth2 secret are required for Kubernaut Agent")
	}

	getCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infrastructure
		"-n", namespace, "get", "configmap", "kubernaut-agent-config", "-o", "jsonpath={.data.config\\.yaml}")
	currentConfig, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("get KA config for Fleet patch: %w", err)
	}

	encodedConfig, err := buildKubernautAgentFleetConfig(currentConfig, namespace, fleetOptions)
	if err != nil {
		return err
	}
	configPatch, err := json.Marshal(map[string]map[string]string{"data": {"config.yaml": string(encodedConfig)}})
	if err != nil {
		return fmt.Errorf("marshal KA Fleet ConfigMap patch: %w", err)
	}
	patchConfigCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infrastructure
		"-n", namespace, "patch", "configmap", "kubernaut-agent-config", "--type=merge", "-p", string(configPatch))
	patchConfigCmd.Stdout = writer
	patchConfigCmd.Stderr = writer
	if err := patchConfigCmd.Run(); err != nil {
		return fmt.Errorf("patch KA Fleet config: %w", err)
	}

	deploymentPatch, err := buildKubernautAgentFleetDeploymentPatch(fleetOptions)
	if err != nil {
		return err
	}
	patchDeploymentCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infrastructure
		"-n", namespace, "patch", "deployment", "kubernaut-agent", "--type=strategic", "-p", string(deploymentPatch))
	patchDeploymentCmd.Stdout = writer
	patchDeploymentCmd.Stderr = writer
	if err := patchDeploymentCmd.Run(); err != nil {
		return fmt.Errorf("patch KA Fleet OAuth2 credentials mount: %w", err)
	}

	for _, args := range [][]string{
		{"--kubeconfig", kubeconfigPath, "-n", namespace, "rollout", "restart", "deployment/kubernaut-agent"},
		{"--kubeconfig", kubeconfigPath, "-n", namespace, "rollout", "status", "deployment/kubernaut-agent", "--timeout=120s"},
	} {
		cmd := exec.CommandContext(ctx, "kubectl", args...) //nolint:gosec // G204: test infrastructure
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("restart KA with Fleet config: %w", err)
		}
	}
	_, _ = fmt.Fprintf(writer, "  ✅ KA Fleet OAuth2 configured for %s\n", fleetOptions.MCPGatewayEndpoint)
	return nil
}

func buildKubernautAgentFleetDeploymentPatch(fleetOptions *FleetHelmOptions) ([]byte, error) {
	if fleetOptions == nil || fleetOptions.OAuth2CredentialsSecret == "" {
		return nil, fmt.Errorf("fleet OAuth2 credentials secret is required for Kubernaut Agent")
	}
	return json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]any{{
						"name": "kubernaut-agent",
						"volumeMounts": []map[string]any{{
							"name":      "fleet-oauth2-credentials",
							"mountPath": "/etc/kubernaut-agent/" + fleetOptions.OAuth2CredentialsSecret,
							"readOnly":  true,
						}},
					}},
					"volumes": []map[string]any{{
						"name":   "fleet-oauth2-credentials",
						"secret": map[string]string{"secretName": fleetOptions.OAuth2CredentialsSecret},
					}},
				},
			},
		},
	})
}

func buildKubernautAgentFleetConfig(currentConfig []byte, namespace string, fleetOptions *FleetHelmOptions) ([]byte, error) {
	if fleetOptions == nil || fleetOptions.OAuth2CredentialsSecret == "" || fleetOptions.MCPGatewayEndpoint == "" || fleetOptions.OAuth2TokenURL == "" {
		return nil, fmt.Errorf("fleet gateway endpoint, OAuth2 token URL, and secret are required for Kubernaut Agent")
	}
	var config map[string]any
	if err := yaml.Unmarshal(currentConfig, &config); err != nil {
		return nil, fmt.Errorf("parse KA config for Fleet patch: %w", err)
	}
	integrations, ok := config["integrations"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("kubernaut agent config must contain an integrations mapping")
	}
	integrations["fleet"] = map[string]any{
		"endpoint":    fleetOptions.MCPGatewayEndpoint,
		"gatewayType": fleetOptions.MCPGatewayType,
		"oauth2": map[string]any{
			"enabled":              true,
			"tokenURL":             fleetOptions.OAuth2TokenURL,
			"credentialsSecretRef": fleetOptions.OAuth2CredentialsSecret,
			"scopes":               fleetOptions.OAuth2Scopes,
			"tlsCaFile":            "/etc/tls-ca/ca.crt",
		},
	}
	interactive, ok := config["interactive"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("kubernaut agent config must contain an interactive mapping")
	}
	interactive["jwtProviders"] = []map[string]any{{
		"name":      "keycloak-fleet-e2e",
		"issuer":    "https://keycloak:8443/realms/kubernaut-demo",
		"jwksURL":   fmt.Sprintf("https://keycloak.%s.svc.cluster.local:8443/realms/kubernaut-demo/protocol/openid-connect/certs", namespace),
		"audience":  "kubernaut-apifrontend",
		"tlsCaFile": "/etc/tls-ca/ca.crt",
		"claimMappings": map[string]string{
			"username": "preferred_username",
			"groups":   "groups",
		},
	}}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal KA Fleet config: %w", err)
	}
	return encoded, nil
}

func afInstallCRDs(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	crds := []string{
		"config/crd/bases/kubernaut.ai_investigationsessions.yaml",
		"config/crd/bases/kubernaut.ai_remediationrequests.yaml",
		"config/crd/bases/kubernaut.ai_remediationapprovalrequests.yaml",
	}
	for _, crd := range crds {
		path := filepath.Join(projectRoot, crd)
		data, err := os.ReadFile(path) //nolint:gosec // G304: path from known CRD list
		if err != nil {
			_, _ = fmt.Fprintf(writer, "WARNING: CRD file not found: %s\n", path)
			continue
		}
		if err := kubectlApplyStdinAF(ctx, kubeconfigPath, string(data), writer); err != nil {
			return fmt.Errorf("failed to apply CRD %s: %w", crd, err)
		}
	}
	return nil
}

func afDeployDex(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_ = namespace
	projectRoot := getProjectRoot()
	dexPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "dex.yaml")
	data, err := os.ReadFile(dexPath) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read dex.yaml: %w", err)
	}
	return kubectlApplyStdinAF(ctx, kubeconfigPath, string(data), writer)
}

func afDeployE2ERBAC(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	saManifest := fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: apifrontend
  namespace: %s
`, namespace)
	if err := kubectlApplyStdinAF(ctx, kubeconfigPath, saManifest, writer); err != nil {
		return fmt.Errorf("failed to create AF ServiceAccount: %w", err)
	}

	rbacPath := filepath.Join(projectRoot, "deploy", "apifrontend", "base", "02-rbac.yaml")
	rbacData, err := os.ReadFile(rbacPath) //nolint:gosec // G304: path from project constants
	if err != nil {
		return fmt.Errorf("failed to read 02-rbac.yaml: %w", err)
	}
	if err := kubectlApplyStdinAF(ctx, kubeconfigPath, string(rbacData), writer); err != nil {
		return fmt.Errorf("failed to deploy AF RBAC from base: %w", err)
	}

	personaToolRBAC, err := PersonaToolClusterRolesYAML()
	if err != nil {
		return fmt.Errorf("failed to generate persona tool ClusterRoles: %w", err)
	}
	if err := kubectlApplyStdinAF(ctx, kubeconfigPath, personaToolRBAC, writer); err != nil {
		return fmt.Errorf("failed to deploy persona tool ClusterRoles: %w", err)
	}

	userRBACPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "e2e-user-rbac.yaml")
	data, err := os.ReadFile(userRBACPath) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read e2e-user-rbac.yaml: %w", err)
	}
	if err := kubectlApplyStdinAF(ctx, kubeconfigPath, string(data), writer); err != nil {
		return fmt.Errorf("failed to deploy E2E user RBAC: %w", err)
	}
	return nil
}

func afDeployFleetRBAC(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	manifest := buildAFleetRBACManifest(namespace)
	if err := kubectlApplyStdinAF(ctx, kubeconfigPath, manifest, writer); err != nil {
		return fmt.Errorf("apply least-privilege Fleet AF RBAC: %w", err)
	}
	return nil
}

func buildAFleetRBACManifest(namespace string) string {
	return fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: apifrontend-fleet-registry-reader
  namespace: %[1]s
rules:
- apiGroups: ["gateway.envoyproxy.io"]
  resources: ["backends"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["aigateway.envoyproxy.io"]
  resources: ["mcproutes"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: apifrontend-fleet-registry-reader
  namespace: %[1]s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: apifrontend-fleet-registry-reader
subjects:
- kind: ServiceAccount
  name: apifrontend
  namespace: %[1]s
`, namespace)
}

func collectDeploymentDiagnostics(ctx context.Context, kubeconfigPath, namespace, name string, writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "\n== DIAGNOSTICS for deployment/%s ==\n", name)

	run := func(args ...string) {
		c := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", kubeconfigPath, "-n", namespace}, args...)...) //nolint:gosec
		c.Stdout = writer
		c.Stderr = writer
		_ = c.Run()
	}

	_, _ = fmt.Fprintln(writer, "-- kubectl get pods --")
	run("get", "pods", "-l", "app="+name, "-o", "wide")

	_, _ = fmt.Fprintln(writer, "-- kubectl describe pod --")
	run("describe", "pods", "-l", "app="+name)

	_, _ = fmt.Fprintln(writer, "-- kubectl logs (last 50 lines) --")
	run("logs", "-l", "app="+name, "--tail=50", "--all-containers=true")

	_, _ = fmt.Fprintln(writer, "-- kubectl get events --")
	run("get", "events", "--sort-by=.lastTimestamp", "--field-selector", "involvedObject.kind=Pod")

	_, _ = fmt.Fprintf(writer, "== END DIAGNOSTICS for deployment/%s ==\n\n", name)
}

func kubectlApplyStdinAF(ctx context.Context, kubeconfigPath, manifest string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-") //nolint:gosec // G204: test infra
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
