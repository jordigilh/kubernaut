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

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

// SetupE2EInfrastructure is the top-level orchestrator for AF E2E tests.
// It deploys the full kubernaut stack (KA+DS+PostgreSQL+Redis+mock-LLM+DEX+CRDs)
// then overlays AF's own image and config.
//
// Image strategy mirrors kubernaut's own E2E pattern:
//   - When IMAGE_REGISTRY + IMAGE_TAG are set, use registry references for DS, KA,
//     mock-LLM (kinfra BuildImageForKind fast-path) and apifrontend as
//     IMAGE_REGISTRY + "/apifrontend:" + IMAGE_TAG. When IMAGE_REGISTRY is set,
//     kind load is skipped so kubelet pulls public GHCR images directly.
//   - Otherwise, images are built locally and loaded into Kind (including AF via BuildAFImage).
func SetupE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Setup (kubernaut-aligned)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getAFProjectRoot()

	// Pre-create coverdata directory so Kind hostPath mount succeeds.
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
	// PHASE 1: Resolve/build images.
	// DS, KA, mock-LLM: use registry when IMAGE_REGISTRY+IMAGE_TAG set,
	// otherwise build from kubernaut source (same fallback pattern as kubernaut).
	// AF: always built locally from this repo.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 1: Resolving images...")

	type buildResult struct {
		name  string
		image string
		err   error
	}
	results := make(chan buildResult, 4)

	// Kubernaut stack images (registry fast-path when IMAGE_REGISTRY+IMAGE_TAG set)
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
			cfg := kinfra.E2EImageConfig{
				ServiceName:      name,
				ImageName:        image,
				DockerfilePath:   dockerfile,
				BuildContextPath: buildCtx,
			}
			img, err := kinfra.BuildImageForKind(cfg, writer)
			results <- buildResult{name, img, err}
		}(svc.name, svc.image, svc.dockerfile, svc.buildCtx)
	}

	// AF: IMAGE_REGISTRY + IMAGE_TAG => registry image; else local BuildAFImage.
	// DD-TEST-007: When E2E_COVERAGE=true, always build locally with GOFLAGS=-cover
	// so the binary writes coverage counters to GOCOVERDIR at runtime.
	enableCoverage := os.Getenv("E2E_COVERAGE") == "true"
	go func() {
		if imageRegistry != "" && imageTag != "" && !enableCoverage {
			img := strings.TrimRight(imageRegistry, "/") + "/apifrontend:" + imageTag
			_, _ = fmt.Fprintf(writer, "  apifrontend: using registry image %s\n", img)
			results <- buildResult{"apifrontend", img, nil}
		} else {
			if enableCoverage {
				_, _ = fmt.Fprintln(writer, "  apifrontend: building locally with coverage instrumentation (E2E_COVERAGE=true)")
			}
			img, err := BuildAFImage(writer)
			results <- buildResult{"apifrontend", img, err}
		}
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
	opts := kinfra.KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-kubernautagent-config.yaml",
		WaitTimeout:               "5m",
		DeleteExisting:            true,
		CleanupOrphanedContainers: true,
		UsePodman:                 true,
		ProjectRootAsWorkingDir:   true,
	}
	if err := kinfra.CreateKindClusterWithConfig(opts, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images into Kind
	// In registry mode (IMAGE_REGISTRY set): skip loading entirely.
	// All kubernaut GHCR packages are public — kubelet pulls directly
	// from the registry inside the Kind node (same as KA/fullpipeline E2E).
	// In local mode: load locally-built images via kind load.
	// ═══════════════════════════════════════════════════════════════════════
	if imageRegistry != "" && !enableCoverage {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Skipping image loading (CI/CD: IMAGE_REGISTRY set, Kind pulls from public GHCR)")
	} else if imageRegistry != "" && enableCoverage {
		// Coverage mode in CI: only AF was built locally; load AF, let others pull from registry.
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading coverage-instrumented AF image into Kind...")
		if err := kinfra.LoadImageToKind(images["apifrontend"], "apifrontend", clusterName, writer); err != nil {
			return fmt.Errorf("failed to load apifrontend image: %w", err)
		}
		_, _ = fmt.Fprintln(writer, "  apifrontend loaded (coverage build)")
	} else {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading images into Kind...")

		for name, img := range images {
			if err := kinfra.LoadImageToKind(img, name, clusterName, writer); err != nil {
				return fmt.Errorf("failed to load %s image: %w", name, err)
			}
			_, _ = fmt.Fprintf(writer, "  %s loaded\n", name)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy kubernaut stack (DS + KA + dependencies)
	// Uses kinfra exported functions (inline manifests) + AF-local manifests.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 4: Deploying kubernaut stack...")

	if err := CreateNamespace(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Inter-service TLS (ECDSA P-256 CA + leaf certs)
	_, _ = fmt.Fprintln(writer, "  Generating inter-service TLS...")
	if _, err := kinfra.GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}
	if err := kinfra.GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
	}

	// Deploy DS stack using kinfra canonical helpers: PostgreSQL, Redis,
	// migrations, RBAC, and DS service with NodePort 30089.
	_, _ = fmt.Fprintln(writer, "  Deploying DataStorage stack (PostgreSQL + Redis + migrations + DS)...")
	if err := kinfra.DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, images["datastorage"], 30089, writer); err != nil {
		return fmt.Errorf("DataStorage stack deploy failed: %w", err)
	}

	// Bind apifrontend SA to data-storage-client ClusterRole so AF can access DS
	_, _ = fmt.Fprintln(writer, "  Binding apifrontend SA to data-storage-client role...")
	if err := bindAFServiceAccountToDSClient(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("AF DS client RBAC failed: %w", err)
	}

	// Deploy mock-LLM (used by AF for LLM routing in E2E)
	_, _ = fmt.Fprintln(writer, "  Deploying mock-LLM...")
	if err := deployMockLLM(ctx, kubeconfigPath, namespace, images["mock-llm"], writer); err != nil {
		return fmt.Errorf("mock-LLM deploy failed: %w", err)
	}

	// Deploy KA RBAC + KA service (kinfra exported, inline manifests)
	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent RBAC...")
	if err := kinfra.DeployKubernautAgentServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("KA RBAC failed: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  Deploying Kubernaut Agent...")
	if err := kinfra.DeployKubernautAgentOnly(clusterName, kubeconfigPath, namespace, images["kubernautagent"], false, writer); err != nil {
		return fmt.Errorf("KA deploy failed: %w", err)
	}

	certDir := filepath.Join(os.TempDir(), "apifrontend-e2e-certs")
	if err := GenerateCerts(certDir, writer); err != nil {
		return fmt.Errorf("failed to generate AF certs: %w", err)
	}
	if err := CreateTLSSecrets(ctx, kubeconfigPath, namespace, certDir, writer); err != nil {
		return fmt.Errorf("failed to create AF TLS secrets: %w", err)
	}
	_ = os.Setenv("AF_E2E_CERT_DIR", certDir)
	_ = os.Setenv("CERT_DIR", certDir)
	_ = os.Setenv("AF_E2E_CA_CERT", filepath.Join(certDir, "ca.crt"))
	_ = os.Setenv("AF_E2E_DEX_URL", "http://localhost:5556/dex")
	_ = os.Setenv("KUBECONFIG", kubeconfigPath)

	_, _ = fmt.Fprintln(writer, "Phase 5: Deploy AF (programmatic)")

	if err := installAFCRDs(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	if err := deployDexForAF(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to deploy Dex: %w", err)
	}

	if err := deployAFE2ERBAC(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to deploy AF RBAC: %w", err)
	}

	afImage := images["apifrontend"]
	if err := deployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, enableCoverage, writer); err != nil {
		return fmt.Errorf("failed to deploy AF service: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for rollouts + enable JWT on KA
	// KA must be ready before AF because AF's readiness probe checks
	// KAClient.Healthy() via circuit breaker. If KA isn't up, AF's CB opens
	// and the readiness probe never passes (Issue #1184 fix).
	// DEX must be up before KA can validate JWT config, so we enable JWT
	// after the initial rollout wait.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 6: Waiting for deployments...")

	for _, deploy := range []string{"datastorage", "kubernaut-agent", "dex", "apifrontend"} {
		_, _ = fmt.Fprintf(writer, "  Waiting for %s...\n", deploy)
		timeout := 120 * time.Second
		if deploy == "datastorage" {
			timeout = 180 * time.Second
		}
		if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, deploy, timeout, writer); err != nil {
			return fmt.Errorf("%s not ready: %w", deploy, err)
		}
	}

	// Now that DEX is running, patch KA to enable JWT with AF's audience.
	_, _ = fmt.Fprintln(writer, "  Patching KA for JWT delegation (DEX is now available)...")
	if err := patchKAJWTAudience(ctx, kubeconfigPath, namespace, writer); err != nil {
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

// TeardownE2EInfrastructure cleans up the Kind cluster.
func TeardownE2EInfrastructure(writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "Tearing down Kind cluster: %s\n", DefaultClusterName)
	return DeleteCluster(writer)
}


// bindAFServiceAccountToDSClient creates a ClusterRoleBinding that grants the
// apifrontend service account access to the data-storage-client ClusterRole.
func bindAFServiceAccountToDSClient(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
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
	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

// deployMockLLM deploys the mock-LLM service with the AF keyword scenarios ConfigMap.
func deployMockLLM(ctx context.Context, kubeconfigPath, namespace, mockLLMImage string, writer io.Writer) error {
	projectRoot := getAFProjectRoot()
	mockLLMManifest := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "mock-llm.yaml")

	// Read the manifest and replace the image reference
	data, err := os.ReadFile(mockLLMManifest) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read mock-llm.yaml: %w", err)
	}

	manifest := strings.ReplaceAll(string(data), "ghcr.io/jordigilh/kubernaut/mock-llm:pr-1161", mockLLMImage)
	manifest = strings.ReplaceAll(manifest, "imagePullPolicy: Always", "imagePullPolicy: IfNotPresent")

	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

// patchKAJWTAudience patches the KA ConfigMap to add JWT provider config with
// AF's DEX audience and FQDN URLs. KA is initially deployed without JWT
// (enableJWT=false) because DEX is not yet available; this function injects
// the full jwtProviders block after DEX is running.
func patchKAJWTAudience(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	getCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "get", "configmap", "kubernaut-agent-config",
		"-o", "jsonpath={.data.config\\.yaml}")
	out, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("get KA config: %w", err)
	}
	currentConfig := string(out)

	jwtBlock := fmt.Sprintf(`  jwtProviders:
    - name: dex-e2e
      issuer: "http://dex.%s.svc:5556/dex"
      jwksURL: "http://dex.%s.svc:5556/dex/keys"
      audience: "kubernaut-apifrontend"
      claimMappings:
        username: "email"
        groups: "groups"`, namespace, namespace)

	// Insert jwtProviders after rateLimitPerUser (the last line of the interactive block)
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
	_, _ = fmt.Fprintln(writer, "  ✅ KA JWT audience patched to accept AF tokens")
	return nil
}

// installAFCRDs applies the three CRDs required by AF E2E tests from local files.
func installAFCRDs(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	projectRoot := getAFProjectRoot()
	crds := []string{
		"config/crd/bases/apifrontend.kubernaut.ai_investigationsessions.yaml",
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
		if err := kubectlApplyStdin(ctx, kubeconfigPath, string(data), writer); err != nil {
			return fmt.Errorf("failed to apply CRD %s: %w", crd, err)
		}
	}
	return nil
}

// deployDexForAF deploys AF's DEX OIDC provider with multi-role E2E users.
func deployDexForAF(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	_ = namespace // namespace set in dex.yaml manifests
	projectRoot := getAFProjectRoot()
	dexPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "dex.yaml")
	data, err := os.ReadFile(dexPath) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read dex.yaml: %w", err)
	}
	if err := kubectlApplyStdin(ctx, kubeconfigPath, string(data), writer); err != nil {
		return fmt.Errorf("failed to deploy dex: %w", err)
	}
	return nil
}

// deployAFE2ERBAC creates the AF ServiceAccount, ClusterRole, ClusterRoleBinding,
// and the E2E user RBAC (impersonation targets for multi-role testing).
func deployAFE2ERBAC(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	afRBAC := fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: apifrontend
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: apifrontend
rules:
  - apiGroups: ["apifrontend.kubernaut.ai"]
    resources: ["investigationsessions"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apifrontend.kubernaut.ai"]
    resources: ["investigationsessions/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
  - apiGroups: [""]
    resources: ["users", "groups", "serviceaccounts"]
    verbs: ["impersonate"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: apifrontend
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: apifrontend
subjects:
  - kind: ServiceAccount
    name: apifrontend
    namespace: %s
`, namespace, namespace)
	if err := kubectlApplyStdin(ctx, kubeconfigPath, afRBAC, writer); err != nil {
		return fmt.Errorf("failed to deploy AF RBAC: %w", err)
	}
	projectRoot := getAFProjectRoot()
	userRBACPath := filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "e2e-user-rbac.yaml")
	data, err := os.ReadFile(userRBACPath) //nolint:gosec // G304: path from test constants
	if err != nil {
		return fmt.Errorf("failed to read e2e-user-rbac.yaml: %w", err)
	}
	if err := kubectlApplyStdin(ctx, kubeconfigPath, string(data), writer); err != nil {
		return fmt.Errorf("failed to deploy E2E user RBAC: %w", err)
	}
	return nil
}

// deployAPIFrontendService deploys the AF ConfigMaps, Deployment, and NodePort Service (programmatic E2E manifest).
// When enableCoverage is true (E2E_COVERAGE=true), the pod is configured with
// GOCOVERDIR=/coverdata and a hostPath volume mount so the coverage-instrumented
// binary writes runtime counters that CollectE2EBinaryCoverage can harvest.
func deployAPIFrontendService(ctx context.Context, kubeconfigPath, namespace, afImage string, enableCoverage bool, writer io.Writer) error {
	projectRoot := getAFProjectRoot()
	configData, err := os.ReadFile(filepath.Join(projectRoot, "deploy", "apifrontend", "overlays", "e2e", "config.yaml")) //nolint:gosec // G304
	if err != nil {
		return fmt.Errorf("failed to read config.yaml: %w", err)
	}
	rbacRolesData, err := os.ReadFile(filepath.Join(projectRoot, "deploy", "apifrontend", "base", "rbac_roles.yaml")) //nolint:gosec // G304
	if err != nil {
		return fmt.Errorf("failed to read rbac_roles.yaml: %w", err)
	}
	pullPolicy := "IfNotPresent"
	if os.Getenv("IMAGE_REGISTRY") != "" && !enableCoverage {
		pullPolicy = "Always"
	}

	// DD-TEST-007: Coverage instrumentation YAML fragments
	coverageEnvYAML := ""
	coverageVolumeMountYAML := ""
	coverageVolumeYAML := ""
	coverageSecurityContextYAML := ""
	if enableCoverage {
		coverageEnvYAML = `
            - name: GOCOVERDIR
              value: /coverdata`
		coverageVolumeMountYAML = `
            - name: coverdata
              mountPath: /coverdata`
		coverageVolumeYAML = `
        - name: coverdata
          hostPath:
            path: /coverdata
            type: DirectoryOrCreate`
		coverageSecurityContextYAML = `
      securityContext:
        runAsUser: 0
        runAsGroup: 0`
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: apifrontend-config
  namespace: %s
data:
  config.yaml: |
%s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: apifrontend-rbac-roles
  namespace: %s
data:
  rbac_roles.yaml: |
%s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: apifrontend
  namespace: %s
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
      automountServiceAccountToken: true%s
      containers:
        - name: apifrontend
          image: %s
          imagePullPolicy: %s
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
            - name: LLM_API_KEY
              value: "mock-key"%s
          volumeMounts:
            - name: config
              mountPath: /etc/apifrontend
              readOnly: true
            - name: tls-certs
              mountPath: /etc/apifrontend/tls
              readOnly: true
            - name: inter-service-ca
              mountPath: /etc/apifrontend/inter-service-ca
              readOnly: true%s
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
          projected:
            sources:
              - configMap:
                  name: apifrontend-config
                  items:
                    - key: config.yaml
                      path: config.yaml
              - configMap:
                  name: apifrontend-rbac-roles
                  items:
                    - key: rbac_roles.yaml
                      path: rbac_roles.yaml
        - name: tls-certs
          secret:
            secretName: apifrontend-tls
            optional: false
        - name: inter-service-ca
          configMap:
            name: inter-service-ca%s
---
apiVersion: v1
kind: Service
metadata:
  name: apifrontend
  namespace: %s
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
      targetPort: health
      nodePort: 30081
  selector:
    app: apifrontend
`, namespace, indentYAML(string(configData), 4),
		namespace, indentYAML(string(rbacRolesData), 4),
		namespace, coverageSecurityContextYAML, afImage, pullPolicy,
		coverageEnvYAML, coverageVolumeMountYAML, coverageVolumeYAML, namespace)
	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

func indentYAML(s string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func kubectlApplyStdin(ctx context.Context, kubeconfigPath, manifest string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-") //nolint:gosec // G204: test infra
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
