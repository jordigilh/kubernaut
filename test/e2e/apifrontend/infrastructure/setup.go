package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" //nolint:revive // registers pgx driver for database/sql
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

	// AF: IMAGE_REGISTRY + IMAGE_TAG => registry image; else local BuildAFImage
	go func() {
		if imageRegistry != "" && imageTag != "" {
			img := strings.TrimRight(imageRegistry, "/") + "/apifrontend:" + imageTag
			_, _ = fmt.Fprintf(writer, "  apifrontend: using registry image %s\n", img)
			results <- buildResult{"apifrontend", img, nil}
		} else {
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
	if imageRegistry != "" {
		_, _ = fmt.Fprintln(writer, "\nPHASE 3: Skipping image loading (CI/CD: IMAGE_REGISTRY set, Kind pulls from public GHCR)")
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

	fmt.Fprintln(writer, "Phase 5: Deploy AF (programmatic)")

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
	if err := deployAPIFrontendService(ctx, kubeconfigPath, namespace, afImage, writer); err != nil {
		return fmt.Errorf("failed to deploy AF service: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for rollouts + enable JWT on KA
	// DEX must be up before KA can validate JWT config, so we enable JWT
	// after PHASE 5 deploys DEX and wait for everything together.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 6: Waiting for deployments...")

	for _, deploy := range []string{"datastorage", "dex", "apifrontend"} {
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

// deployPostgreSQL deploys a minimal PostgreSQL instance for DataStorage.
func deployPostgreSQL(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
  namespace: %[1]s
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: slm_password
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  namespace: %[1]s
spec:
  type: NodePort
  ports:
  - port: 5432
    targetPort: 5432
    nodePort: 30439
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  namespace: %[1]s
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
        image: docker.io/library/postgres:16-alpine
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: action_history
        - name: POSTGRES_USER
          value: slm_user
        - name: POSTGRES_PASSWORD
          value: slm_password
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
          initialDelaySeconds: 5
          periodSeconds: 5
`, namespace)
	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

// deployRedis deploys a minimal Redis instance for session/DLQ support.
func deployRedis(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: %[1]s
spec:
  type: NodePort
  ports:
  - port: 6379
    targetPort: 6379
    nodePort: 30387
  selector:
    app: redis
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: %[1]s
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
        image: docker.io/library/redis:7-alpine
        ports:
        - containerPort: 6379
        readinessProbe:
          exec:
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 3
          periodSeconds: 3
`, namespace)
	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

// deployDataStorageInline deploys DataStorage with all required RBAC using
// inline manifests. This avoids kinfra's findWorkspaceRoot dependency.
func deployDataStorageInline(ctx context.Context, kubeconfigPath, namespace, dsImage string, writer io.Writer) error {
	pullPolicy := kinfra.GetImagePullPolicy()

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: %[1]s
data:
  config.yaml: |
    server:
      port: 8080
      host: "0.0.0.0"
      metricsPort: 9090
      healthPort: 8081
      readTimeout: 30s
      writeTimeout: 30s
      signerCertDir: /etc/signing-certs
      tls:
        certDir: /etc/tls
    database:
      host: postgresql.%[1]s.svc.cluster.local
      port: 5432
      name: action_history
      user: slm_user
      sslMode: disable
      maxOpenConns: 100
      maxIdleConns: 20
      connMaxLifetime: 1h
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
  name: redis-secret
  namespace: %[1]s
stringData:
  redis-secrets.yaml: |
    password: ""
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: data-storage-sa
  namespace: %[1]s
  labels:
    app: data-storage-service
    authorization: dd-auth-014
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-auth-middleware
  labels:
    app: data-storage-service
    authorization: dd-auth-014
rules:
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: data-storage-auth-middleware
  labels:
    app: data-storage-service
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-auth-middleware
subjects:
- kind: ServiceAccount
  name: data-storage-sa
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-client
  labels:
    app: data-storage-service
    authorization: dd-auth-014
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["data-storage-service"]
  verbs: ["create", "get", "list", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: apifrontend-ds-client
  labels:
    app: data-storage-service
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
- kind: ServiceAccount
  name: apifrontend
  namespace: %[1]s
---
apiVersion: v1
kind: Service
metadata:
  name: data-storage-service
  namespace: %[1]s
  labels:
    app: datastorage
spec:
  type: NodePort
  ports:
  - name: https
    port: 8080
    targetPort: 8080
    nodePort: 30089
    protocol: TCP
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30281
    protocol: TCP
  selector:
    app: datastorage
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: %[1]s
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
      serviceAccountName: data-storage-sa
      containers:
      - name: datastorage
        image: %[2]s
        imagePullPolicy: %[3]s
        ports:
        - name: https
          containerPort: 8080
        - name: health
          containerPort: 8081
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        - name: POD_NAMESPACE
          value: %[1]s
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        - name: tls-certs
          mountPath: /etc/tls
          readOnly: true
        - name: signing-certs
          mountPath: /etc/signing-certs
          readOnly: true
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 45
          periodSeconds: 15
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: tls-certs
        secret:
          secretName: datastorage-tls
          optional: true
      - name: signing-certs
        secret:
          secretName: datastorage-signing
      - name: secrets
        projected:
          sources:
          - secret:
              name: postgresql-secret
              items:
              - key: db-secrets.yaml
                path: db-secrets.yaml
          - secret:
              name: redis-secret
              items:
              - key: redis-secrets.yaml
                path: redis-secrets.yaml
`, namespace, dsImage, pullPolicy)
	return kubectlApplyStdin(ctx, kubeconfigPath, manifest, writer)
}

// findMigrationsDir locates the kubernaut migrations directory: first in a sibling
// checkout (local dev), then in the Go module cache (CI).
func findMigrationsDir(writer io.Writer) (string, error) {
	// Try sibling checkout (local dev: ../kubernaut/migrations/)
	kubernautRoot := filepath.Join(filepath.Dir(getAFProjectRoot()), "kubernaut")
	candidate := filepath.Join(kubernautRoot, "migrations")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		_, _ = fmt.Fprintf(writer, "    Using migrations from sibling checkout: %s\n", candidate)
		return candidate, nil
	}

	// Try Go module cache (CI: go list -m resolves the cached module)
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/jordigilh/kubernaut").Output()
	if err == nil {
		modDir := strings.TrimSpace(string(out))
		candidate = filepath.Join(modDir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			_, _ = fmt.Fprintf(writer, "    Using migrations from Go module cache: %s\n", candidate)
			return candidate, nil
		}
	}
	return "", fmt.Errorf("kubernaut migrations/ not found in sibling dir or Go module cache")
}

// applyDatabaseMigrations applies all goose migrations to PostgreSQL via port-forward.
// Matches kubernaut/test/infrastructure.ApplyAllMigrations: goose provider + version tracking.
// DS requires audit_events (and other tables) before it can create partitions on boot.
func applyDatabaseMigrations(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	migrationsDir, err := findMigrationsDir(writer)
	if err != nil {
		return err
	}

	// Get PostgreSQL pod name
	getPodCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "get", "pod", "-l", "app=postgresql",
		"-o", "jsonpath={.items[0].metadata.name}")
	podNameBytes, err := getPodCmd.Output()
	if err != nil {
		return fmt.Errorf("get postgresql pod name: %w", err)
	}
	podName := strings.TrimSpace(string(podNameBytes))

	// Start port-forward (same pattern as kinfra.startPortForward)
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("find available port: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port //nolint:errcheck // net.Listener.Addr never fails
	_ = listener.Close()

	_, _ = fmt.Fprintf(writer, "    Port-forwarding to %s (localhost:%d → 5432)...\n", podName, localPort)
	pfCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"port-forward", "-n", namespace, podName, fmt.Sprintf("%d:5432", localPort))
	if err := pfCmd.Start(); err != nil {
		return fmt.Errorf("start port-forward: %w", err)
	}
	defer func() {
		_ = pfCmd.Process.Kill()
		_ = pfCmd.Wait()
	}()

	// Wait for port-forward to be ready
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), time.Second)
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Open database connection via port-forward
	connStr := fmt.Sprintf("host=localhost port=%d user=slm_user password=slm_password dbname=action_history sslmode=disable", localPort)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("open postgres connection: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	// Run goose migrations (same as kinfra.RunGooseMigrations)
	if err := kinfra.RunGooseMigrations(ctx, db, migrationsDir, writer); err != nil {
		return fmt.Errorf("goose migrations: %w", err)
	}

	// Grant permissions (same as kinfra.applyGooseMigrationsE2E)
	grantSQL := `
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`
	if _, grantErr := db.ExecContext(ctx, grantSQL); grantErr != nil {
		_, _ = fmt.Fprintf(writer, "    ⚠️  Grant permissions (may already exist): %v\n", grantErr)
	}

	_, _ = fmt.Fprintln(writer, "    ✅ Database migrations applied (goose)")
	return nil
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
			fmt.Fprintf(writer, "WARNING: CRD file not found: %s\n", path)
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
func deployAPIFrontendService(ctx context.Context, kubeconfigPath, namespace, afImage string, writer io.Writer) error {
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
	if os.Getenv("IMAGE_REGISTRY") != "" {
		pullPolicy = "Always"
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
      automountServiceAccountToken: true
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
          volumeMounts:
            - name: config
              mountPath: /etc/apifrontend
              readOnly: true
            - name: tls-certs
              mountPath: /etc/apifrontend/tls
              readOnly: true
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
  selector:
    app: apifrontend
`, namespace, indentYAML(string(configData), 4),
		namespace, indentYAML(string(rbacRolesData), 4),
		namespace, afImage, pullPolicy, namespace)
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
