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
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const kubernautAgentServiceName = "kubernaut-agent"

// IsolatedKubernautAgentConfig describes a KA Deployment that owns only the
// test namespace's AgentSessions. The shared E2E infrastructure must already
// provide the referenced ClusterRoles and service endpoints.
type IsolatedKubernautAgentConfig struct {
	Namespace            string
	SharedNamespace      string
	DataStorageNamespace string
	TLSSourceNamespace   string
	CAPath               string
	DeploymentName       string
	ServiceName          string
	ServiceAccountName   string
	Image                string
	DataStorageURL       string
	DataStorageHealthURL string
	LLMEndpoint          string
	ShadowLLMEndpoint    string
	PrometheusURL        string
	AlertManagerURL      string
}

// IsolatedKubernautAgentFixture identifies the resources used by a dedicated
// KA E2E fixture. Its unique namespace and selector allow a pod restart while
// unrelated E2E specs continue using the shared KA Deployment.
type IsolatedKubernautAgentFixture struct {
	Namespace            string
	DeploymentName       string
	ServiceName          string
	PodSelector          string
	DataStorageNamespace string
	ServiceAccountName   string
}

// DeployIsolatedKubernautAgent creates a namespace-scoped KA Deployment with
// unique RBAC bindings and a ClusterIP Service. It deliberately exposes no
// fixed NodePorts, so parallel E2E specs can create independent fixtures.
//
// The shared E2E setup must have created these ClusterRoles first:
// data-storage-client, data-storage-auth-middleware, and
// kubernaut-agent-investigator.
func DeployIsolatedKubernautAgent(ctx context.Context, cfg IsolatedKubernautAgentConfig, kubeconfigPath string, writer io.Writer) (IsolatedKubernautAgentFixture, error) {
	cfg = normalizeIsolatedKubernautAgentConfig(cfg, kubeconfigPath)
	if err := validateIsolatedKubernautAgentConfig(cfg); err != nil {
		return IsolatedKubernautAgentFixture{}, err
	}

	if err := CreateTestNamespace(ctx, cfg.Namespace, kubeconfigPath, writer); err != nil {
		return IsolatedKubernautAgentFixture{}, err
	}

	// The isolated dispatcher has its own namespace, but its dependencies remain
	// in the shared E2E topology. Reuse that topology's CA and KA certificate;
	// generating a second CA here would make the shared DataStorage certificate
	// unverifiable and would race the shared host-side CA file.
	if err := ReplicateInterServiceCAConfigMapAtPath(ctx, kubeconfigPath, cfg.Namespace, cfg.CAPath, "inter-service-ca", writer); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("replicate isolated kubernaut-agent CA: %w", err)
	}
	if err := ReplicateTLSSecret(ctx, kubeconfigPath, cfg.TLSSourceNamespace, cfg.Namespace, "kubernautagent-tls", writer); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("replicate isolated kubernaut-agent TLS secret: %w", err)
	}

	rbacManifest := isolatedKubernautAgentRBACManifest(cfg)
	if err := applyIsolatedKubernautAgentManifest(ctx, kubeconfigPath, rbacManifest, writer); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("apply isolated kubernaut-agent RBAC: %w", err)
	}
	if err := grantIsolatedKubernautAgentDataStorageAccess(ctx, kubeconfigPath, cfg, writer); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("grant isolated kubernaut-agent data storage access: %w", err)
	}

	manifest := isolatedKubernautAgentManifest(cfg)
	if err := applyIsolatedKubernautAgentManifest(ctx, kubeconfigPath, manifest, writer); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("apply isolated kubernaut-agent deployment: %w", err)
	}

	waitCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/"+cfg.DeploymentName,
		"-n", cfg.Namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s",
	)
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return IsolatedKubernautAgentFixture{}, fmt.Errorf("wait for isolated kubernaut-agent deployment: %w", err)
	}

	return IsolatedKubernautAgentFixture{
		Namespace:            cfg.Namespace,
		DeploymentName:       cfg.DeploymentName,
		ServiceName:          cfg.ServiceName,
		PodSelector:          "app=" + cfg.DeploymentName,
		DataStorageNamespace: cfg.DataStorageNamespace,
		ServiceAccountName:   cfg.ServiceAccountName,
	}, nil
}

// TeardownIsolatedKubernautAgent removes the fixture namespace and the
// cluster-scoped bindings that cannot be garbage-collected with it.
func TeardownIsolatedKubernautAgent(ctx context.Context, fixture IsolatedKubernautAgentFixture, kubeconfigPath string, writer io.Writer) error {
	roleBindingName := isolatedKubernautAgentDataStorageBindingName(fixture.Namespace)
	roleBindingCmd := exec.CommandContext(ctx, "kubectl", "delete", "rolebinding", roleBindingName,
		"-n", fixture.DataStorageNamespace, "--kubeconfig", kubeconfigPath, "--ignore-not-found")
	roleBindingCmd.Stdout = writer
	roleBindingCmd.Stderr = writer
	if err := roleBindingCmd.Run(); err != nil {
		return fmt.Errorf("delete isolated kubernaut-agent data storage role binding %q: %w", roleBindingName, err)
	}

	for _, name := range isolatedKubernautAgentClusterRoleBindingNames(fixture.Namespace) {
		cmd := exec.CommandContext(ctx, "kubectl", "delete", "clusterrolebinding", name,
			"--kubeconfig", kubeconfigPath, "--ignore-not-found")
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("delete isolated kubernaut-agent ClusterRoleBinding %q: %w", name, err)
		}
	}
	namespaceCmd := exec.CommandContext(ctx, "kubectl", "delete", "namespace", fixture.Namespace,
		"--kubeconfig", kubeconfigPath, "--ignore-not-found", "--wait=true", "--timeout=60s")
	namespaceCmd.Stdout = writer
	namespaceCmd.Stderr = writer
	if err := namespaceCmd.Run(); err != nil {
		return fmt.Errorf("delete isolated kubernaut-agent namespace: %w", err)
	}
	return nil
}

// RestartIsolatedKubernautAgentPod deletes exactly one fixture pod and waits
// for its Deployment to create a ready replacement. It never selects the
// shared kubernaut-agent Deployment.
func RestartIsolatedKubernautAgentPod(ctx context.Context, fixture IsolatedKubernautAgentFixture, kubeconfigPath string, writer io.Writer) error {
	getCmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
		"-n", fixture.Namespace,
		"--kubeconfig", kubeconfigPath,
		"-l", fixture.PodSelector,
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	podName, err := getCmd.Output()
	if err != nil {
		return fmt.Errorf("find isolated kubernaut-agent pod: %w", err)
	}
	pod := strings.TrimSpace(string(podName))
	if pod == "" {
		return fmt.Errorf("no isolated kubernaut-agent pod found in namespace %q", fixture.Namespace)
	}

	deleteCmd := exec.CommandContext(ctx, "kubectl", "delete", "pod", pod,
		"-n", fixture.Namespace,
		"--kubeconfig", kubeconfigPath,
		"--wait=false",
	)
	deleteCmd.Stdout = writer
	deleteCmd.Stderr = writer
	if err := deleteCmd.Run(); err != nil {
		return fmt.Errorf("delete isolated kubernaut-agent pod %q: %w", pod, err)
	}

	waitDeleteCmd := exec.CommandContext(ctx, "kubectl", "wait", "--for=delete", "pod/"+pod,
		"-n", fixture.Namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s",
	)
	waitDeleteCmd.Stdout = writer
	waitDeleteCmd.Stderr = writer
	if err := waitDeleteCmd.Run(); err != nil {
		return fmt.Errorf("wait for isolated kubernaut-agent pod %q deletion: %w", pod, err)
	}

	rolloutCmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		"deployment/"+fixture.DeploymentName,
		"-n", fixture.Namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s",
	)
	rolloutCmd.Stdout = writer
	rolloutCmd.Stderr = writer
	if err := rolloutCmd.Run(); err != nil {
		return fmt.Errorf("wait for isolated kubernaut-agent replacement: %w", err)
	}
	return nil
}

func validateIsolatedKubernautAgentConfig(cfg IsolatedKubernautAgentConfig) error {
	fields := map[string]string{
		"namespace":            cfg.Namespace,
		"sharedNamespace":      cfg.SharedNamespace,
		"dataStorageNamespace": cfg.DataStorageNamespace,
		"tlsSourceNamespace":   cfg.TLSSourceNamespace,
		"caPath":               cfg.CAPath,
		"deploymentName":       cfg.DeploymentName,
		"serviceName":          cfg.ServiceName,
		"serviceAccountName":   cfg.ServiceAccountName,
		"image":                cfg.Image,
		"dataStorageURL":       cfg.DataStorageURL,
		"dataStorageHealthURL": cfg.DataStorageHealthURL,
		"llmEndpoint":          cfg.LLMEndpoint,
	}
	for field, value := range fields {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("isolated kubernaut-agent %s must not be empty", field)
		}
	}
	return nil
}

func normalizeIsolatedKubernautAgentConfig(cfg IsolatedKubernautAgentConfig, kubeconfigPath string) IsolatedKubernautAgentConfig {
	if cfg.DataStorageNamespace == "" {
		cfg.DataStorageNamespace = cfg.SharedNamespace
	}
	if cfg.TLSSourceNamespace == "" {
		cfg.TLSSourceNamespace = cfg.SharedNamespace
	}
	if cfg.CAPath == "" {
		cfg.CAPath = InterServiceCAPath(kubeconfigPath)
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = kubernautAgentServiceName
	}
	return cfg
}

func applyIsolatedKubernautAgentManifest(ctx context.Context, kubeconfigPath, manifest string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func grantIsolatedKubernautAgentDataStorageAccess(ctx context.Context, kubeconfigPath string, cfg IsolatedKubernautAgentConfig, writer io.Writer) error {
	manifest := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-client
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, isolatedKubernautAgentDataStorageBindingName(cfg.Namespace), cfg.DataStorageNamespace,
		cfg.ServiceAccountName, cfg.Namespace)
	return applyIsolatedKubernautAgentManifest(ctx, kubeconfigPath, manifest, writer)
}

func isolatedKubernetesBindingName(prefix, namespace string) string {
	sum := sha256.Sum256([]byte(namespace))
	return fmt.Sprintf("%s-%x", prefix, sum[:5])
}

func isolatedKubernautAgentDataStorageBindingName(namespace string) string {
	return isolatedKubernetesBindingName("ka-ds", namespace)
}

func isolatedKubernautAgentRBACManifest(cfg IsolatedKubernautAgentConfig) string {
	authBinding := isolatedKubernetesBindingName("ka-auth", cfg.Namespace)
	investigatorBinding := isolatedKubernetesBindingName("ka-investigator", cfg.Namespace)
	return strings.ReplaceAll(fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: data-storage-auth-middleware
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-agent-investigator
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
	`, cfg.ServiceAccountName, cfg.Namespace,
		authBinding, cfg.ServiceAccountName, cfg.Namespace,
		investigatorBinding, cfg.ServiceAccountName, cfg.Namespace), "\t", "")
}

func isolatedKubernautAgentClusterRoleBindingNames(namespace string) []string {
	return []string{
		isolatedKubernetesBindingName("ka-auth", namespace),
		isolatedKubernetesBindingName("ka-investigator", namespace),
	}
}

func isolatedKubernautAgentManifest(cfg IsolatedKubernautAgentConfig) string {
	imagePullPolicy := GetImagePullPolicy()
	alignmentConfig := "        enabled: false"
	if cfg.ShadowLLMEndpoint != "" {
		alignmentConfig = fmt.Sprintf(`        enabled: true
        timeout: "10s"
        maxStepTokens: 500
        llm:
          provider: "openai"
          model: "shadow-eval"
          endpoint: "%s"
          apiKeyFile: "/etc/kubernautagent/llm-runtime/api-key"`, cfg.ShadowLLMEndpoint)
	}
	return strings.ReplaceAll(fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-config
  namespace: %s
data:
  config.yaml: |
    runtime:
      logging:
        level: "debug"
      session:
        ttl: "10m"
        maxConcurrentInvestigations: 10
      server:
        tls:
          certDir: /etc/tls
        tlsCaFile: /etc/tls-ca/ca.crt
    ai:
      llm:
        provider: "openai"
        apiKeyFile: "/etc/kubernautagent/llm-runtime/api-key"
        reasoning:
          enabled: true
      alignmentCheck:
%s
    integrations:
      dataStorage:
        url: "%s"
        healthUrl: "%s"
        tls:
          caFile: /etc/tls-ca/ca.crt
      tools:
        prometheus:
          url: "%s"
        alertmanager:
          url: "%s"
    interactive:
      enabled: false
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-llm-runtime
  namespace: %s
data:
  api-key: "mock-api-key-for-e2e"
  llm-runtime.yaml: |
    model: "mock-model"
    endpoint: "%s"
    temperature: 0.0
    maxRetries: 1
    timeoutSeconds: 120
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      serviceAccountName: %s
      containers:
      - name: kubernaut-agent
        image: %s
        imagePullPolicy: %s
        ports:
        - name: https
          containerPort: 8443
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
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: TLS_CA_FILE
          value: /etc/tls-ca/ca.crt
        - name: LLM_ENDPOINT
          value: "%s"
        - name: LLM_MODEL
          value: "mock-model"
        - name: LLM_PROVIDER
          value: "openai"
        - name: DATA_STORAGE_URL
          value: "%s"
        volumeMounts:
        - name: config
          mountPath: /etc/kubernautagent/config.yaml
          subPath: config.yaml
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
          name: %s-config
      - name: llm-runtime
        configMap:
          name: %s-llm-runtime
      - name: tls-certs
        secret:
          secretName: kubernautagent-tls
      - name: tls-ca
        configMap:
          name: inter-service-ca
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  type: ClusterIP
  ports:
  - name: https
    port: 8443
    targetPort: 8443
  - name: health
    port: 8081
    targetPort: 8081
  - name: metrics
    port: 9090
    targetPort: 9090
  selector:
    app: %s
	`, cfg.DeploymentName, cfg.Namespace, alignmentConfig,
		cfg.DataStorageURL, cfg.DataStorageHealthURL, cfg.PrometheusURL, cfg.AlertManagerURL,
		cfg.DeploymentName, cfg.Namespace, cfg.LLMEndpoint,
		cfg.DeploymentName, cfg.Namespace, cfg.DeploymentName, cfg.DeploymentName, cfg.DeploymentName,
		cfg.ServiceAccountName, cfg.Image, imagePullPolicy, cfg.LLMEndpoint, cfg.DataStorageURL,
		cfg.DeploymentName, cfg.DeploymentName, cfg.ServiceName, cfg.Namespace, cfg.DeploymentName), "\t", "")
}
