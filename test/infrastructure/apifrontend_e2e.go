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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// AFDefaultClusterName is the Kind cluster name for apifrontend E2E tests.
	AFDefaultClusterName = "apifrontend-e2e"
	// AFDefaultNamespace is the Kubernetes namespace for AF E2E workloads.
	AFDefaultNamespace = "kubernaut-system"
)

// SetupAPIFrontendE2EInfrastructure is the top-level orchestrator for AF E2E tests.
// It deploys the full kubernaut stack (KA+DS+PostgreSQL+Redis+mock-LLM+DEX+CRDs)
// then overlays AF's own image and config.
//
// Image strategy mirrors kubernaut's own E2E pattern:
//   - When IMAGE_REGISTRY + IMAGE_TAG are set, use registry references for DS, KA,
//     mock-LLM (kinfra BuildImageForKind fast-path) and apifrontend as
//     IMAGE_REGISTRY + "/apifrontend:" + IMAGE_TAG. When IMAGE_REGISTRY is set,
//     kind load is skipped so kubelet pulls public GHCR images directly.
//   - Otherwise, images are built locally and loaded into Kind (including AF via BuildAFImage).
func SetupAPIFrontendE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Setup (kubernaut-aligned)")
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
		_, _ = fmt.Fprintln(writer, "  Local build mode (no IMAGE_REGISTRY/IMAGE_TAG set)")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Resolve/build images
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 1: Resolving images...")

	type buildResult struct {
		name  string
		image string
		err   error
	}
	results := make(chan buildResult, 4)

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
			cfg := E2EImageConfig{
				ServiceName:      name,
				ImageName:        image,
				DockerfilePath:   dockerfile,
				BuildContextPath: buildCtx,
			}
			img, err := BuildImageForKind(cfg, writer)
			results <- buildResult{name, img, err}
		}(svc.name, svc.image, svc.dockerfile, svc.buildCtx)
	}

	// DD-TEST-007: AF is always built locally with GOFLAGS=-cover
	go func() {
		img, err := BuildAFImage(writer)
		results <- buildResult{"apifrontend", img, err}
	}()

	images := make(map[string]string, 4)
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("failed to build %s: %w", r.name, r.err)
		}
		images[r.name] = r.image
		_, _ = fmt.Fprintf(writer, "  %s: %s\n", r.name, r.image)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 2: Creating Kind cluster...")
	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-kubernautagent-config.yaml",
		WaitTimeout:               "5m",
		DeleteExisting:            true,
		CleanupOrphanedContainers: true,
		UsePodman:                 true,
		ProjectRootAsWorkingDir:   true,
	}
	if err := CreateKindClusterWithConfig(opts, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into Kind
	// ═══════════════════════════════════════════════════════════════════════
	if imageRegistry != "" {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading AF image into Kind (coverage build); others pull from GHCR...")
		if err := LoadImageToKind(images["apifrontend"], "apifrontend", clusterName, writer); err != nil {
			return fmt.Errorf("failed to load apifrontend image: %w", err)
		}
		_, _ = fmt.Fprintln(writer, "  apifrontend loaded")
	} else {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading images into Kind...")
		for name, img := range images {
			if err := LoadImageToKind(img, name, clusterName, writer); err != nil {
				return fmt.Errorf("failed to load %s image: %w", name, err)
			}
			_, _ = fmt.Fprintf(writer, "  %s loaded\n", name)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy kubernaut stack (DS + KA + dependencies)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 4: Deploying kubernaut stack...")

	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Generating inter-service TLS...")
	if _, err := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
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
	if err := afDeployMockLLM(ctx, kubeconfigPath, namespace, images["mock-llm"], writer); err != nil {
		return fmt.Errorf("mock-LLM deploy failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent RBAC...")
	if err := DeployKubernautAgentServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("KA RBAC failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent...")
	if err := DeployKubernautAgentOnly(clusterName, kubeconfigPath, namespace, images["kubernautagent"], false, writer); err != nil {
		return fmt.Errorf("KA deploy failed: %w", err)
	}

	certDir := filepath.Join(os.TempDir(), "apifrontend-e2e-certs")
	if err := AFGenerateCerts(certDir, writer); err != nil {
		return fmt.Errorf("failed to generate AF certs: %w", err)
	}
	if err := AFCreateTLSSecrets(ctx, kubeconfigPath, namespace, certDir, writer); err != nil {
		return fmt.Errorf("failed to create AF TLS secrets: %w", err)
	}
	_ = os.Setenv("AF_E2E_CERT_DIR", certDir)
	_ = os.Setenv("CERT_DIR", certDir)
	_ = os.Setenv("AF_E2E_CA_CERT", filepath.Join(certDir, "ca.crt"))
	_ = os.Setenv("AF_E2E_DEX_URL", "https://localhost:5556/dex")
	_ = os.Setenv("KUBECONFIG", kubeconfigPath)

	_, _ = fmt.Fprintln(writer, "Phase 5: Deploy AF (programmatic)")

	if err := afInstallCRDs(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	if err := afDeployDex(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to deploy Dex: %w", err)
	}

	if err := afDeployE2ERBAC(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to deploy AF RBAC: %w", err)
	}

	// Seed DS with action types + workflows so kubernaut_list_workflows returns
	// a non-empty catalog. Must run after afDeployE2ERBAC (creates the apifrontend SA).
	_, _ = fmt.Fprintln(writer, "  Seeding DS action types + workflows for AF E2E...")
	saToken, seedErr := GetServiceAccountToken(ctx, namespace, "apifrontend", kubeconfigPath)
	if seedErr != nil {
		return fmt.Errorf("get apifrontend SA token for DS seeding: %w", seedErr)
	}
	seedClient, seedErr := CreateTLSAuthenticatedDataStorageClient("https://localhost:8089", saToken, kubeconfigPath)
	if seedErr != nil {
		return fmt.Errorf("create DS seed client: %w", seedErr)
	}
	// #1661 Phase 53: also seed as CRDs for DS's informer-backed cache. Workflows here
	// now seed via SeedWorkflowsViaKubectlApply (real AuthWebhook admission, Phase 55),
	// so this file no longer touches DS's Postgres inline-registration endpoint at all
	// -- but SeedActionTypesViaAPI's Postgres dual-seed below is retained regardless,
	// since Phase 55 hasn't yet dropped the action_type_taxonomy table/FK/handlers
	// (tracked separately; premature removal here would just be dead code until then).
	if seedErr = SeedActionTypesViaCRD(kubeconfigPath, namespace, writer); seedErr != nil {
		return fmt.Errorf("seed action types (CRD): %w", seedErr)
	}
	if seedErr = SeedActionTypesViaAPI(seedClient, writer); seedErr != nil {
		return fmt.Errorf("seed action types (Postgres): %w", seedErr)
	}
	testWorkflows := GetKAE2ETestWorkflows()
	// #1661 Phase 55: seed via kubectl apply (real AuthWebhook admission pipeline)
	// instead of DS's retired Postgres-backed inline endpoint.
	if _, seedErr = SeedWorkflowsViaKubectlApply(kubeconfigPath, namespace, testWorkflowsToSeedSpecs(testWorkflows), writer); seedErr != nil {
		return fmt.Errorf("seed workflows: %w", seedErr)
	}

	afImage := images["apifrontend"]
	if err := DeployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, true, writer); err != nil {
		return fmt.Errorf("failed to deploy AF service: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for rollouts + enable JWT on KA
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 6: Waiting for deployments...")

	for _, deploy := range []string{"datastorage", "kubernaut-agent", "mock-llm", "dex", "apifrontend"} {
		_, _ = fmt.Fprintf(writer, "  Waiting for %s...\n", deploy)
		timeout := 120 * time.Second
		if deploy == "datastorage" {
			timeout = 180 * time.Second
		}
		if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, deploy, timeout, writer); err != nil {
			return fmt.Errorf("%s not ready: %w", deploy, err)
		}
	}

	_, _ = fmt.Fprintln(writer, "  Patching KA for JWT delegation (DEX is now available)...")
	if err := afPatchKAJWTAudience(ctx, kubeconfigPath, namespace, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "  WARNING: KA JWT audience patch failed (non-fatal): %v\n", err)
	}
	_, _ = fmt.Fprintln(writer, "  Waiting for kubernaut-agent restart...")
	if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, "kubernaut-agent", 120*time.Second, writer); err != nil {
		return fmt.Errorf("kubernaut-agent not ready after JWT patch: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Ready: Full kubernaut stack + AF")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// BuildAFImage builds the apifrontend container image locally with coverage
// instrumentation (GOFLAGS=-cover).
func BuildAFImage(writer io.Writer) (string, error) {
	cfg := E2EImageConfig{
		ServiceName:    "apifrontend",
		ImageName:      "apifrontend",
		DockerfilePath: "docker/apifrontend.Dockerfile",
		EnableCoverage: true,
	}
	return BuildImageForKind(cfg, writer)
}

// AFGenerateCerts runs the AF cert generation script.
func AFGenerateCerts(certDir string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	script := projectRoot + "/deploy/apifrontend/overlays/e2e/generate-certs.sh"
	cmd := exec.Command("bash", script, certDir) //nolint:gosec // G204: test infra, script path from project root
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
		Namespace:      "kubernaut-system",
		KubeconfigPath: kcPath,
	}, writer)
}

// DeployAPIFrontendService deploys the AF ConfigMaps, Deployment, and NodePort Service.
// When enableCoverage is true, the pod includes GOCOVERDIR=/coverdata env var and
// a hostPath volume mount (DD-TEST-007). When false (e.g., FP cluster), coverage
// instrumentation is omitted.
func DeployAPIFrontendService(ctx context.Context, kubeconfigPath, namespace, afImage string, enableCoverage bool, writer io.Writer) error {
	projectRoot := getProjectRoot()
	configData, err := os.ReadFile(filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "config.yaml")) //nolint:gosec // G304
	if err != nil {
		return fmt.Errorf("failed to read config.yaml: %w", err)
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

// PersonaToolClusterRolesYAML generates the 6 per-persona ClusterRoles (with
// kubernaut.ai/tools verb=use resourceNames) and 6 ClusterRoleBindings (mapping
// DEX OIDC groups to the ClusterRoles). Mirrors the Helm chart template that
// generates kubernaut-tool-{persona} ClusterRoles from values.yaml personas.
//
// Used by both AF E2E and full-pipeline E2E deployments.
func PersonaToolClusterRolesYAML() string {
	type persona struct {
		name  string
		tools []string
	}
	personas := []persona{
		{"sre", []string{
			"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_approve",
			"kubernaut_cancel_remediation", "kubernaut_watch", "kubernaut_investigate",
			"kubernaut_discover_workflows", "kubernaut_select_workflow",
			"kubernaut_present_decision", "kubernaut_list_workflows", "kubernaut_get_remediation_history",
			"kubernaut_get_effectiveness", "kubernaut_get_audit_trail",
			"kubectl_get", "kubectl_list", "kubectl_list_events",
			"kubernaut_check_existing_remediation", "kubernaut_remediate",
		}},
		{"ai-orchestrator", []string{
			"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_approve",
			"kubernaut_cancel_remediation", "kubernaut_watch", "kubernaut_investigate",
			"kubernaut_discover_workflows", "kubernaut_select_workflow",
			"kubernaut_present_decision",
			"kubectl_get", "kubectl_list", "kubectl_list_events",
			"kubernaut_check_existing_remediation", "kubernaut_remediate",
		}},
		{"cicd", []string{
			"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch",
		}},
		{"observability", []string{
			"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch",
			"kubernaut_get_effectiveness", "kubernaut_list_workflows",
			"kubectl_get", "kubectl_list", "kubectl_list_events",
		}},
		{"l3-audit", []string{
			"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_list_workflows",
			"kubernaut_get_remediation_history", "kubernaut_get_effectiveness", "kubernaut_get_audit_trail",
		}},
		{"remediation-approver", []string{
			"kubernaut_approve", "kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch",
		}},
	}

	var b strings.Builder
	for _, p := range personas {
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
`, p.name)
		for _, t := range p.tools {
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
`, p.name, p.name, p.name)
	}

	return b.String()
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

func afDeployMockLLM(ctx context.Context, kubeconfigPath, namespace, mockLLMImage string, writer io.Writer) error {
	projectRoot := getProjectRoot()
	mockLLMManifest := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "mock-llm.yaml")

	data, err := os.ReadFile(mockLLMManifest) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read mock-llm.yaml: %w", err)
	}

	manifest := strings.ReplaceAll(string(data), "ghcr.io/jordigilh/kubernaut/mock-llm:pr-1161", mockLLMImage)
	manifest = strings.ReplaceAll(manifest, "imagePullPolicy: Always", "imagePullPolicy: IfNotPresent")

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

	anchor := "rateLimitPerUser: 20"
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

	personaToolRBAC := PersonaToolClusterRolesYAML()
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
