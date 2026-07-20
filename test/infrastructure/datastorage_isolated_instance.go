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
	"os/exec"
	"strings"
	"time"
)

// Issue #1661 Phase 56 (DD-WORKFLOW-018): a second, test-owned DataStorage +
// PostgreSQL + Redis + RBAC stack, deployed in its own namespace within the
// SAME Kind cluster as the shared datastorage-e2e instance. This exists
// specifically for the DS-restart-and-cache-recovery E2E case
// (test/e2e/datastorage/27_ds_restart_cache_recovery_test.go), which kills the
// DS pod mid-suite -- something that cannot safely be done against the shared
// instance without disrupting every other concurrently-running spec.
//
// Everything namespace-scoped below (ServiceAccount, ConfigMap, Secret,
// Service, Deployment) reuses the exact same literal names the shared
// instance uses (deployDataStorageServiceInNamespaceWithNodePort) -- safe,
// because namespaced resources are keyed by (namespace, name), and this stack
// lives in its own namespace. Redis is reused verbatim (deployRedisInNamespace,
// already ClusterIP). PostgreSQL gets its own deployIsolatedPostgreSQL instead
// of reusing deployPostgreSQLInNamespace: that helper's Service is a
// cluster-scoped NodePort (hardcoded 30432, DD-TEST-001), which collides with
// the shared instance's own Postgres Service. The two cluster-scoped RBAC
// objects (ClusterRole, ClusterRoleBinding) similarly get a unique
// "-resilience" suffix, since those are keyed by name alone and would
// otherwise collide with the shared instance's "data-storage-auth-middleware"
// (kubectl apply replaces a ClusterRoleBinding's subjects list wholesale
// rather than appending it).
const (
	// ResilienceDSNodePortAPI is the isolated instance's HTTPS API NodePort.
	// Mapped to host port 28093 via kind-datastorage-config.yaml's
	// extraPortMappings (DS's DD-TEST-001 block is 28090-28099; the shared
	// instance already owns 28090/28091/28092).
	ResilienceDSNodePortAPI int32 = 30082

	// ResilienceDSNodePortHealth is the isolated instance's plain-HTTP health
	// NodePort. Mapped to host port 28094.
	ResilienceDSNodePortHealth int32 = 30282

	// ResilienceDSHostPortAPI is the host-machine port for the isolated
	// instance's HTTPS API (localhost:28093).
	ResilienceDSHostPortAPI = 28093

	// ResilienceDSHostPortHealth is the host-machine port for the isolated
	// instance's health endpoint (localhost:28094).
	ResilienceDSHostPortHealth = 28094

	resilienceClusterRoleName = "data-storage-auth-middleware-resilience"
)

// DeployIsolatedDataStorageInstance deploys a second, fully isolated
// DataStorage + PostgreSQL + Redis stack into namespace, within the Kind
// cluster identified by kubeconfigPath. Mirrors
// SetupDataStorageInfrastructureParallel's build sequence (namespace ->
// RBAC -> TLS/signing certs -> Postgres+Redis in parallel -> migrations ->
// DataStorage -> readiness wait), reusing every namespace-scoped helper
// verbatim and substituting only the cluster-scoped RBAC names.
func DeployIsolatedDataStorageInstance(ctx context.Context, namespace, kubeconfigPath, dsImage string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "🏗️  Deploying ISOLATED DataStorage resilience instance in %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create isolated namespace: %w", err)
	}

	if err := deployIsolatedDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy isolated RBAC: %w", err)
	}

	// Deliberately does NOT call GenerateInterServiceTLS here: that helper's
	// host-side CA PEM path (InterServiceCAPath) is keyed only by
	// kubeconfigPath, not namespace -- calling it a second time for this
	// isolated namespace under the SAME kubeconfig would silently regenerate
	// and overwrite the CA PEM file the shared instance's already-running
	// clients depend on via NewTLSAwareTransport, breaking concurrently
	// running specs. datastorage-tls is optional (ConfigureConditionalTLS,
	// pkg/shared/tls/tls.go:71-94, falls back to plain HTTP when the cert
	// files are absent), so this isolated instance simply serves HTTP
	// instead of HTTPS -- irrelevant to what this test proves (cache
	// recovery), and avoided entirely rather than risk that collision.
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to generate signing certificate for isolated instance: %w", err)
	}

	type result struct {
		name string
		err  error
	}
	results := make(chan result, 2)
	go func() {
		err := deployIsolatedPostgreSQL(ctx, namespace, kubeconfigPath, writer)
		results <- result{"PostgreSQL", err}
	}()
	go func() {
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		results <- result{"Redis", err}
	}()
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("isolated %s deploy failed: %w", r.name, r.err)
		}
	}

	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("isolated instance migrations failed: %w", err)
	}

	if err := deployIsolatedDataStorageService(ctx, namespace, kubeconfigPath, dsImage, writer); err != nil {
		return fmt.Errorf("failed to deploy isolated DataStorage service: %w", err)
	}

	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("isolated instance services not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Isolated DataStorage resilience instance ready in %s\n", namespace)
	return nil
}

// TeardownIsolatedDataStorageInstance deletes the isolated namespace (which
// cascades every namespace-scoped resource created above) plus the two
// cluster-scoped RBAC objects that outlive namespace deletion.
func TeardownIsolatedDataStorageInstance(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🧹 Tearing down isolated DataStorage resilience instance (%s)...\n", namespace)

	delCRB := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"delete", "clusterrolebinding", resilienceClusterRoleName, "--ignore-not-found")
	delCRB.Stdout = writer
	delCRB.Stderr = writer
	if err := delCRB.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  failed to delete ClusterRoleBinding %s: %v\n", resilienceClusterRoleName, err)
	}

	delCR := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"delete", "clusterrole", resilienceClusterRoleName, "--ignore-not-found")
	delCR.Stdout = writer
	delCR.Stderr = writer
	if err := delCR.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  failed to delete ClusterRole %s: %v\n", resilienceClusterRoleName, err)
	}

	if err := CleanupDataStorageTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to delete isolated namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Isolated resilience instance torn down\n")
	return nil
}

// deployIsolatedPostgreSQL deploys a namespace-scoped PostgreSQL instance for
// the isolated resilience stack. Deliberately does NOT reuse
// deployPostgreSQLInNamespace verbatim: that helper's Service is
// type:NodePort with a hardcoded nodePort:30432 (DD-TEST-001, for host-machine
// debugging access to the SHARED instance's Postgres) -- NodePorts are
// cluster-scoped, so a second Service reusing that same nodePort in a
// different namespace fails outright ("provided port is already allocated").
// This isolated instance's Postgres never needs host-machine access (only
// DS's own API/health need that, via ResilienceDSHostPortAPI/Health above),
// so ClusterIP avoids the collision entirely.
func deployIsolatedPostgreSQL(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := `---
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
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
  labels:
    app: postgresql
spec:
  type: ClusterIP
  ports:
  - name: postgresql
    port: 5432
    targetPort: 5432
    protocol: TCP
  selector:
    app: postgresql
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgresql-data
  labels:
    app: postgresql
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
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
            memory: 512Mi
            cpu: 250m
          limits:
            memory: 1Gi
            cpu: 500m
        readinessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
        livenessProbe:
          exec:
            command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
      volumes:
      - name: postgresql-data
        persistentVolumeClaim:
          claimName: postgresql-data
`

	if err := kubectlApply(ctx, kubeconfigPath, namespace, manifest, writer); err != nil {
		return fmt.Errorf("failed to deploy isolated PostgreSQL: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Isolated PostgreSQL deployed (ClusterIP, no host NodePort)\n")
	return nil
}

// deployIsolatedDataStorageServiceRBAC creates the ServiceAccount +
// cluster-scoped RBAC the isolated instance's auth middleware needs
// (TokenReview + SubjectAccessReview + RemediationWorkflow/ActionType
// list/watch, same rule set as deployDataStorageServiceRBAC), but with a
// uniquely-named ClusterRole/ClusterRoleBinding so it cannot collide with the
// shared instance's "data-storage-auth-middleware".
func deployIsolatedDataStorageServiceRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: data-storage-sa
  namespace: %[1]s
  labels:
    app: data-storage-service
    component: auth
    authorization: dd-auth-014
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %[2]s
  labels:
    app: data-storage-service
    component: auth
    authorization: dd-auth-014
rules:
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
- apiGroups: ["authorization.k8s.io"]
  resources: ["subjectaccessreviews"]
  verbs: ["create"]
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationworkflows", "actiontypes"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %[2]s
  labels:
    app: data-storage-service
    component: auth
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %[2]s
subjects:
- kind: ServiceAccount
  name: data-storage-sa
  namespace: %[1]s
`, namespace, resilienceClusterRoleName)

	return kubectlApply(ctx, kubeconfigPath, namespace, manifest, writer)
}

// deployIsolatedDataStorageService deploys the isolated instance's
// ConfigMap/Secret/Service/Deployment. Same shape as
// deployDataStorageServiceInNamespaceWithNodePort minus OAuth2/coverage
// options (not needed by this resilience test) and with fresh NodePort
// values so it doesn't collide with the shared instance's Service.
func deployIsolatedDataStorageService(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	pullPolicy := GetImagePullPolicy()

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
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
      maxOpenConns: 20
      maxIdleConns: 5
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
stringData:
  redis-secrets.yaml: |
    password: ""
---
apiVersion: v1
kind: Service
metadata:
  name: data-storage-service
  labels:
    app: datastorage
spec:
  type: NodePort
  ports:
  - name: https
    port: 8080
    targetPort: 8080
    nodePort: %[2]d
    protocol: TCP
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: %[3]d
    protocol: TCP
  selector:
    app: datastorage
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
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
        image: %[4]s
        imagePullPolicy: %[5]s
        ports:
        - name: https
          containerPort: 8080
        - name: metrics
          containerPort: 9090
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
        resources:
          requests:
            memory: 512Mi
            cpu: 250m
          limits:
            memory: 1Gi
            cpu: 500m
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 45
          periodSeconds: 15
          timeoutSeconds: 10
          failureThreshold: 5
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
`, namespace, ResilienceDSNodePortAPI, ResilienceDSNodePortHealth, dataStorageImage, pullPolicy)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy isolated DataStorage: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Isolated DataStorage deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

// RestartIsolatedDataStoragePod deletes the isolated instance's DataStorage
// pod and waits for it to be fully gone, then waits for the replacement pod
// to become Ready. Deliberately mirrors the delete-and-verify-gone rigor of
// test/e2e/fleetmetadatacache/shared/resilience.go's outage induction (no
// scale-to-0/scale-to-1 dance, since there is no shared-resource contention
// to protect against: this instance is exclusively owned by one spec).
func RestartIsolatedDataStoragePod(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "💥 Restarting isolated DataStorage pod (namespace=%s)...\n", namespace)

	// Capture the exact pod name before deleting. Deployment's controller
	// schedules a replacement carrying the SAME "app=datastorage" label
	// almost immediately, so `kubectl wait --for=delete -l app=datastorage`
	// never observes zero matches -- it keeps finding the replacement pod and
	// hangs until timeout. Waiting on the specific pod/<name> object instead
	// sidesteps that entirely, with no need for fleetmetadatacache/shared/
	// resilience.go's scale-to-0/scale-to-1 dance (this instance has no
	// shared-resource contention to protect against).
	nameCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "get", "pod", "-l", "app=datastorage",
		"-o", "jsonpath={.items[0].metadata.name}")
	nameOut, err := nameCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to resolve isolated DataStorage pod name: %w", err)
	}
	podName := strings.TrimSpace(string(nameOut))
	if podName == "" {
		return fmt.Errorf("no isolated DataStorage pod found matching app=datastorage in namespace %s", namespace)
	}
	_, _ = fmt.Fprintf(writer, "   🎯 Target pod: %s\n", podName)

	delCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "delete", "pod", podName, "--wait=false")
	delCmd.Stdout = writer
	delCmd.Stderr = writer
	if err := delCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete isolated DataStorage pod %s: %w", podName, err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	waitCmd := exec.CommandContext(waitCtx, "kubectl", "--kubeconfig", kubeconfigPath,
		"-n", namespace, "wait", "--for=delete", "pod/"+podName, "--timeout=60s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("isolated DataStorage pod %s did not actually terminate: %w", podName, err)
	}

	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("isolated DataStorage replacement pod not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Isolated DataStorage pod restarted and ready\n")
	return nil
}
