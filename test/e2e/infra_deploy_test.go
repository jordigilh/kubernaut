package e2e_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
)

const (
	defaultKAImage      = "ghcr.io/jordigilh/kubernaut/kubernautagent:pr-828"
	defaultDSImage      = "ghcr.io/jordigilh/kubernaut/datastorage:pr-828"
	defaultMockLLMImage = "ghcr.io/jordigilh/kubernaut/mock-llm:pr-828"
)

func getImageOrDefault(envVar, defaultImage string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultImage
}

// deployPostgreSQL deploys a single-replica PostgreSQL 16 pod with credentials.
// Replicates the kubernaut test/infrastructure pattern (inline YAML + kubectl apply).
func deployPostgreSQL(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
  namespace: %[1]s
stringData:
  POSTGRES_USER: slm_user
  POSTGRES_PASSWORD: test_password
  POSTGRES_DB: action_history
  db-secrets.yaml: |
    username: slm_user
    password: test_password
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  namespace: %[1]s
  labels:
    app: postgresql
spec:
  ports:
  - name: postgresql
    port: 5432
    targetPort: 5432
    protocol: TCP
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  namespace: %[1]s
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
        image: docker.io/library/postgres:16-alpine
        args: ["-c", "max_connections=200"]
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
        resources:
          requests:
            memory: 256Mi
            cpu: 250m
          limits:
            memory: 512Mi
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: postgresql-data
        emptyDir: {}
`, namespace)

	return kubectlApply(ctx, kubeconfigPath, manifest, writer)
}

// deployRedis deploys a single-replica Redis pod.
func deployRedis(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: %[1]s
  labels:
    app: redis
spec:
  ports:
  - name: redis
    port: 6379
    targetPort: 6379
    protocol: TCP
  selector:
    app: redis
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: %[1]s
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
        image: docker.io/library/redis:7-alpine
        ports:
        - name: redis
          containerPort: 6379
        resources:
          requests:
            memory: 64Mi
            cpu: 100m
          limits:
            memory: 128Mi
        readinessProbe:
          exec:
            command: ["redis-cli", "ping"]
          initialDelaySeconds: 3
          periodSeconds: 3
`, namespace)

	return kubectlApply(ctx, kubeconfigPath, manifest, writer)
}

// deployDataStorage deploys the Data Storage service with TLS, pointing at PostgreSQL and Redis.
func deployDataStorage(ctx context.Context, namespace, kubeconfigPath, image string, writer io.Writer) error {
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
      tls:
        certDir: /etc/tls
    database:
      host: postgresql
      port: 5432
      user: slm_user
      password: test_password
      dbname: action_history
      sslmode: disable
    redis:
      address: redis:6379
    logging:
      level: debug
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: data-storage-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-storage
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: data-storage
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: data-storage
  template:
    metadata:
      labels:
        app.kubernetes.io/name: data-storage
    spec:
      serviceAccountName: data-storage-sa
      containers:
      - name: data-storage
        image: %[2]s
        imagePullPolicy: IfNotPresent
        ports:
        - name: https
          containerPort: 8080
        - name: health
          containerPort: 8081
        - name: metrics
          containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: tls-certs
          mountPath: /etc/tls
          readOnly: true
        - name: tls-ca
          mountPath: /etc/tls-ca
          readOnly: true
        env:
        - name: TLS_CA_FILE
          value: /etc/tls-ca/ca.crt
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: tls-certs
        secret:
          secretName: data-storage-tls
          optional: true
      - name: tls-ca
        configMap:
          name: inter-service-ca
          optional: true
---
apiVersion: v1
kind: Service
metadata:
  name: data-storage
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: data-storage
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8080
    protocol: TCP
  - name: health
    port: 8081
    targetPort: 8081
    protocol: TCP
  selector:
    app.kubernetes.io/name: data-storage
`, namespace, image)

	return kubectlApply(ctx, kubeconfigPath, manifest, writer)
}

// deployMockLLM deploys a mock LLM service that responds to OpenAI-compatible endpoints.
func deployMockLLM(ctx context.Context, namespace, kubeconfigPath, image string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-llm
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: mock-llm
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: mock-llm
  template:
    metadata:
      labels:
        app.kubernetes.io/name: mock-llm
    spec:
      containers:
      - name: mock-llm
        image: %[2]s
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8080
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 3
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: 64Mi
            cpu: 50m
          limits:
            memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: mock-llm
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: mock-llm
spec:
  ports:
  - name: http
    port: 8080
    targetPort: 8080
    protocol: TCP
  selector:
    app.kubernetes.io/name: mock-llm
`, namespace, image)

	return kubectlApply(ctx, kubeconfigPath, manifest, writer)
}

// deployKubernautAgent deploys the Kubernaut Agent with TLS, pointing at DS and mock-LLM.
// This replicates kubernaut's unexported deployKubernautAgentOnly until kubernaut#1121 exports it.
func deployKubernautAgent(ctx context.Context, namespace, kubeconfigPath, image string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-agent-config
  namespace: %[1]s
data:
  config.yaml: |
    runtime:
      logging:
        level: "debug"
      server:
        tls:
          certDir: /etc/tls
    ai:
      llm:
        provider: "openai"
    integrations:
      dataStorage:
        url: "https://data-storage:8443"
    interactive:
      enabled: true
      sessionTTL: "5m"
      inactivityTimeout: "2m"
      maxConcurrentSessions: 3
      rateLimitPerUser: 20
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-agent-llm-runtime
  namespace: %[1]s
data:
  llm-runtime.yaml: |
    model: "mock-model"
    endpoint: "http://mock-llm:8080"
    apiKey: "mock-api-key-for-e2e"
    temperature: 0.7
    maxRetries: 3
    timeoutSeconds: 120
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-agent-sa
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernaut-agent
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: kubernaut-agent
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: kubernaut-agent
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kubernaut-agent
    spec:
      serviceAccountName: kubernaut-agent-sa
      containers:
      - name: kubernaut-agent
        image: %[2]s
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8080
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
          value: "https://data-storage:8443"
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
          secretName: kubernaut-agent-tls
          optional: true
      - name: tls-ca
        configMap:
          name: inter-service-ca
          optional: true
---
apiVersion: v1
kind: Service
metadata:
  name: kubernaut-agent
  namespace: %[1]s
  labels:
    app.kubernetes.io/name: kubernaut-agent
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8080
    protocol: TCP
  - name: health
    port: 8081
    targetPort: 8081
    protocol: TCP
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
  selector:
    app.kubernetes.io/name: kubernaut-agent
`, namespace, image)

	return kubectlApply(ctx, kubeconfigPath, manifest, writer)
}

// createInterServiceCA creates the CA ConfigMap that all services mount for TLS verification.
func createInterServiceCA(ctx context.Context, namespace, kubeconfigPath, certDir string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "Creating inter-service CA ConfigMap from %s/ca.crt\n", certDir)
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "configmap", "inter-service-ca",
		"--from-file=ca.crt="+certDir+"/ca.crt",
		"-n", namespace, "--dry-run=client", "-o", "yaml")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generate CA configmap: %w", err)
	}
	return kubectlApply(ctx, kubeconfigPath, string(out), writer)
}

// createTLSSecret creates a TLS secret from cert/key files.
func createTLSSecret(ctx context.Context, namespace, kubeconfigPath, secretName, certFile, keyFile string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "Creating TLS secret %s\n", secretName)
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "secret", "tls", secretName,
		"--cert="+certFile, "--key="+keyFile,
		"-n", namespace, "--dry-run=client", "-o", "yaml")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("generate TLS secret %s: %w", secretName, err)
	}
	return kubectlApply(ctx, kubeconfigPath, string(out), writer)
}

// waitForDeployment waits for a deployment to roll out successfully.
func waitForDeployment(ctx context.Context, namespace, name, kubeconfigPath string, timeout time.Duration) error {
	cmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		fmt.Sprintf("deployment/%s", name),
		"-n", namespace, "--kubeconfig", kubeconfigPath,
		fmt.Sprintf("--timeout=%s", timeout))
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	return cmd.Run()
}

// kubectlApply applies a YAML manifest via stdin.
func kubectlApply(ctx context.Context, kubeconfigPath, manifest string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply failed: %w", err)
	}
	return nil
}
