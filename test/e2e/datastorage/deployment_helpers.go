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

package datastorage

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/gomega"
)

// DeploymentConfig contains configuration for E2E test deployments
type DeploymentConfig struct {
	Namespace           string
	PostgresPort        int
	RedisPort           int
	DataStoragePort     int
	DBName              string
	DBUser              string
	DBPassword          string
	WaitTimeout         time.Duration
	ApplyMigrations     bool
}

// DefaultDeploymentConfig returns default configuration for E2E tests
func DefaultDeploymentConfig(namespace string) *DeploymentConfig {
	return &DeploymentConfig{
		Namespace:           namespace,
		PostgresPort:        5432,
		RedisPort:           6379,
		DataStoragePort:     8080,
		DBName:              "action_history",
		DBUser:              "slm_user",
		DBPassword:          "test_password",
		WaitTimeout:         2 * time.Minute,
		ApplyMigrations:     true,
	}
}

// DeployPostgreSQL deploys PostgreSQL with pgvector to the specified namespace
func DeployPostgreSQL(cfg *DeploymentConfig) error {
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-init
  namespace: %s
data:
  init.sql: |
    CREATE EXTENSION IF NOT EXISTS vector;
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgresql
  namespace: %s
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
        image: pgvector/pgvector:pg16
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: "%s"
        - name: POSTGRES_USER
          value: "%s"
        - name: POSTGRES_PASSWORD
          value: "%s"
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        volumeMounts:
        - name: postgres-init
          mountPath: /docker-entrypoint-initdb.d
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - %s
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: postgres-init
        configMap:
          name: postgres-init
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  namespace: %s
spec:
  selector:
    app: postgresql
  ports:
  - port: 5432
    targetPort: 5432
`, cfg.Namespace, cfg.Namespace, cfg.DBName, cfg.DBUser, cfg.DBPassword, cfg.DBUser, cfg.Namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = []byte(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w, output: %s", err, output)
	}

	// Wait for PostgreSQL to be ready
	return waitForDeployment(cfg.Namespace, "postgresql", cfg.WaitTimeout)
}

// DeployRedis deploys Redis to the specified namespace
func DeployRedis(cfg *DeploymentConfig) error {
	manifest := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: %s
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
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: %s
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
`, cfg.Namespace, cfg.Namespace)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = []byte(manifest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to deploy Redis: %w, output: %s", err, output)
	}

	// Wait for Redis to be ready
	return waitForDeployment(cfg.Namespace, "redis", cfg.WaitTimeout)
}

// DeployDataStorageService deploys the Data Storage Service to the specified namespace
func DeployDataStorageService(cfg *DeploymentConfig) error {
	// Create ConfigMap for service configuration
	configMap := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: %s
data:
  config.yaml: |
    service:
      name: data-storage
      metricsPort: 9090
      logLevel: debug
      shutdownTimeout: 30s
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: 30s
      write_timeout: 30s
    database:
      host: postgresql
      port: 5432
      name: %s
      user: %s
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      conn_max_idle_time: 10m
      secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
      usernameKey: "username"
      passwordKey: "password"
    redis:
      addr: redis:6379
      db: 0
      dlq_stream_name: dlq-stream
      dlq_max_len: 1000
      dlq_consumer_group: dlq-group
      secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
      passwordKey: "password"
    logging:
      level: debug
      format: json
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secrets
  namespace: %s
type: Opaque
stringData:
  db-secrets.yaml: |
    username: %s
    password: %s
  redis-secrets.yaml: |
    password: ""
`, cfg.Namespace, cfg.DBName, cfg.DBUser, cfg.Namespace, cfg.DBUser, cfg.DBPassword)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = []byte(configMap)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w, output: %s", err, output)
	}

	// Deploy Data Storage Service
	deployment := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: %s
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
      containers:
      - name: datastorage
        image: data-storage:e2e
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 15
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secrets
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: %s
spec:
  selector:
    app: datastorage
  ports:
  - name: http
    port: 8080
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
`, cfg.Namespace, cfg.Namespace)

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = []byte(deployment)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w, output: %s", err, output)
	}

	// Wait for Data Storage Service to be ready
	return waitForDeployment(cfg.Namespace, "datastorage", cfg.WaitTimeout)
}

// waitForDeployment waits for a deployment to be ready
func waitForDeployment(namespace, deploymentName string, timeout time.Duration) error {
	Eventually(func() error {
		cmd := exec.Command("kubectl", "rollout", "status",
			fmt.Sprintf("deployment/%s", deploymentName),
			"-n", namespace,
			"--timeout=10s")
		return cmd.Run()
	}, timeout, 5*time.Second).Should(Succeed(),
		fmt.Sprintf("Deployment %s in namespace %s should be ready", deploymentName, namespace))
	return nil
}

// GetServiceURL returns the URL for accessing a service in the namespace
// For Kind clusters, this uses kubectl port-forward
func GetServiceURL(namespace, serviceName string, port int) string {
	// In E2E tests, we'll use port-forward or NodePort
	// For now, return the in-cluster service URL
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// PortForwardService creates a port-forward to a service and returns the local URL
// The caller is responsible for stopping the port-forward process
func PortForwardService(namespace, serviceName string, localPort, remotePort int) (*exec.Cmd, string, error) {
	cmd := exec.Command("kubectl", "port-forward",
		fmt.Sprintf("svc/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"-n", namespace)

	err := cmd.Start()
	if err != nil {
		return nil, "", fmt.Errorf("failed to start port-forward: %w", err)
	}

	// Wait a bit for port-forward to establish
	time.Sleep(2 * time.Second)

	localURL := fmt.Sprintf("http://localhost:%d", localPort)
	return cmd, localURL, nil
}

