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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createHAPIKindCluster creates a Kind cluster using the HAPI/KA Kind config.
// Reused by both HAPI and Kubernaut Agent E2E suites (same port layout).
func createHAPIKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	if os.Getenv("E2E_COVERAGE") == "true" {
		projectRoot := getProjectRoot()
		coverdataPath := filepath.Join(projectRoot, "coverdata")
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Failed to create coverdata directory: %v\n", err)
		} else {
			if err := os.Chmod(coverdataPath, 0777); err != nil {
				_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to chmod coverdata directory: %v\n", err)
			}
			_, _ = fmt.Fprintf(writer, "  ✅ Created %s for coverage collection (mode=0777)\n", coverdataPath)
		}
	}

	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-holmesgpt-api-config.yaml",
		WaitTimeout:               "5m",
		DeleteExisting:            true,
		ReuseExisting:             false,
		CleanupOrphanedContainers: true,
		UsePodman:                 true,
		ProjectRootAsWorkingDir:   true,
	}
	return CreateKindClusterWithConfig(opts, writer)
}

// createHolmesGPTAPIE2EServiceAccount creates the E2E ServiceAccount with
// RBAC for calling the agent API and accessing DataStorage.
// Used by Kubernaut Agent and legacy HAPI E2E suites.
func createHolmesGPTAPIE2EServiceAccount(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	saName := "kubernaut-agent-e2e-sa"

	if err := CreateServiceAccount(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return fmt.Errorf("failed to create E2E ServiceAccount: %w", err)
	}

	agentRBACYAML := fmt.Sprintf(`---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-agent-e2e-client-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-014
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["kubernaut-agent"]
    verbs: ["create", "get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubernaut-agent-e2e-client-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-014
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kubernaut-agent-e2e-client-access
subjects:
  - kind: ServiceAccount
    name: %s
    namespace: %s
`, namespace, namespace, saName, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(agentRBACYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply agent E2E RBAC: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Agent client RBAC created\n")

	_, _ = fmt.Fprintf(writer, "  🔐 Creating DataStorage client RoleBinding for workflow seeding...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return fmt.Errorf("failed to create DataStorage client RoleBinding: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ E2E ServiceAccount created with agent + DataStorage client permissions\n")
	return nil
}

// deployMockLLMInNamespace deploys the Go Mock LLM service to a Kind namespace.
// Uses ClusterIP for internal access only (no NodePort needed for E2E).
func deployMockLLMInNamespace(ctx context.Context, namespace, kubeconfigPath, imageTag string, workflowUUIDs map[string]string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "   📦 Deploying Mock LLM service (image: %s)...\n", imageTag)

	var scenariosYAML string
	if len(workflowUUIDs) > 0 {
		scenariosYAML = "scenarios:\n"
		for _, key := range SortedWorkflowUUIDKeys(workflowUUIDs) {
			scenariosYAML += fmt.Sprintf("      %s:\n        workflow_id: \"%s\"\n", key, workflowUUIDs[key])
		}
	} else {
		scenariosYAML = "scenarios: {}"
	}

	configMap := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: mock-llm-scenarios
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
data:
  scenarios.yaml: |
    %s
---`, namespace, scenariosYAML)

	_, _ = fmt.Fprintf(writer, "   📦 Creating Mock LLM ConfigMap...\n")
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = strings.NewReader(configMap)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Mock LLM ConfigMap: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ ConfigMap created\n")

	deployment := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-llm
  template:
    metadata:
      labels:
        app: mock-llm
        component: test-infrastructure
    spec:
      containers:
      - name: mock-llm
        image: %s
        imagePullPolicy: %s
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        env:
        - name: MOCK_LLM_HOST
          value: "0.0.0.0"
        - name: MOCK_LLM_PORT
          value: "8080"
        - name: MOCK_LLM_FORCE_TEXT
          value: "false"
        - name: MOCK_LLM_CONFIG_PATH
          value: "/config/scenarios.yaml"
        volumeMounts:
        - name: scenarios-config
          mountPath: /config
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1001
          capabilities:
            drop:
            - ALL
      volumes:
      - name: scenarios-config
        configMap:
          name: mock-llm-scenarios
      securityContext:
        fsGroup: 1001
      restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: mock-llm
`, namespace, imageTag, GetImagePullPolicy(), namespace)

	cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = strings.NewReader(deployment)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM: %w", err)
	}

	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Mock LLM pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=mock-llm",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Mock LLM pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Mock LLM ready\n")

	return nil
}

// BuildKubernautAgentImage builds the Go Kubernaut Agent container image for
// integration and E2E tests. Replaces the deprecated BuildHAPIImage (Python).
//
// The Dockerfile at docker/kubernautagent.Dockerfile uses a multi-stage build:
//   - builder stage: compiles the Go binary from cmd/kubernautagent
//   - development stage: UBI10-minimal runtime with debug + coverage support
//
// Returns the full image name with tag for use in GenericContainerConfig.
func BuildKubernautAgentImage(ctx context.Context, serviceName string, writer io.Writer) (string, error) {
	projectRoot := getProjectRoot()

	imageTag := generateInfrastructureImageTag("kubernautagent", serviceName)
	localImageName := fmt.Sprintf("localhost/kubernautagent:%s", imageTag)

	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	_, _ = fmt.Fprintf(writer, "   🔍 Environment check: IMAGE_REGISTRY=%q IMAGE_TAG=%q\n", registry, tag)

	registryImage, pulled, err := tryPullFromRegistry(ctx, "kubernautagent", localImageName, writer)
	if err != nil {
		return "", fmt.Errorf("failed during registry pull attempt: %w", err)
	}
	if pulled {
		return registryImage, nil
	}

	checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", localImageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ KA image already exists: %s\n", localImageName)
		return localImageName, nil
	}

	_, _ = fmt.Fprintf(writer, "   🔨 Building Kubernaut Agent image (tag: %s)...\n", imageTag)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"--no-cache",
		"-t", localImageName,
		"--force-rm=false",
		"-f", filepath.Join(projectRoot, "docker", "kubernautagent.Dockerfile"),
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		checkAgain := exec.Command("podman", "image", "exists", localImageName)
		if checkAgain.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Build completed with warnings (image exists): %s\n", localImageName)
			return localImageName, nil
		}
		return "", fmt.Errorf("failed to build Kubernaut Agent image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Kubernaut Agent image built: %s\n", localImageName)
	return localImageName, nil
}
