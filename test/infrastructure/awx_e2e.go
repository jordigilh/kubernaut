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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// AWXOperatorVersion is the pinned AWX Operator release (bundles AWX 24.6.1).
	AWXOperatorVersion = "2.19.1"
	AWXOperatorImage   = "quay.io/ansible/awx-operator:" + AWXOperatorVersion

	// AWXImageVersion is the pinned AWX container image version.
	AWXImageVersion = "24.6.1"
	AWXImage        = "quay.io/ansible/awx:" + AWXImageVersion
	AWXEEImage      = "quay.io/ansible/awx-ee:" + AWXImageVersion

	// AWXInstanceName is the AWX CR metadata.name; the operator derives service
	// and deployment names from it (e.g. awx-e2e-service, awx-e2e-web, etc.).
	AWXInstanceName = "awx-e2e"
	AWXServiceName  = AWXInstanceName + "-service"
	AWXServicePort  = 80

	// AWXDatabaseName is the database AWX uses in the shared PostgreSQL instance.
	AWXDatabaseName = "awx"
	AWXDatabaseUser = "awx"
	AWXDatabasePass = "awx_e2e_password"

	// AWXAdminUser is the initial AWX admin user.
	AWXAdminUser = "admin"
	AWXAdminPass = "admin_e2e_password"

	// AWXSecretKey is Django's SECRET_KEY for AWX (safe for E2E only).
	AWXSecretKey = "kubernaut-e2e-awx-secret-key-not-for-production"

	// AWXTokenSecretName is the K8s Secret where the AWX API token is stored.
	AWXTokenSecretName = "awx-api-token"

	// AWXTestPlaybooksRepo is the GitHub repo containing E2E test playbooks.
	AWXTestPlaybooksRepo   = "https://github.com/jordigilh/kubernaut-test-playbooks.git"
	AWXTestPlaybooksCommit = "b7e6a135be2019f995cb4875dbc0116dfda39d21"

	// AWXNodePort is the host-accessible port for AWX API in Kind.
	AWXNodePort = 30095
)

// DeployAWXInNamespace deploys AWX via the official AWX Operator into the
// given namespace. It shares the existing PostgreSQL (creates an "awx" database)
// while the operator manages its own Redis sidecar.
// Designed to run in parallel with other Phase 4 deployments.
func DeployAWXInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "\n--- Deploying AWX via Operator %s into %s (external PG)...\n", AWXOperatorVersion, namespace)

	// Step 1: Install AWX Operator CRDs + controller via kustomize.
	if err := installAWXOperator(namespace, kubeconfigPath, writer); err != nil {
		return err
	}

	// Step 2: Wait for operator controller to be ready.
	_, _ = fmt.Fprintf(writer, "  Waiting for AWX Operator controller...\n")
	waitCmd := exec.Command("kubectl", "rollout", "status",
		"deployment/awx-operator-controller-manager",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=180s",
	)
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("AWX Operator controller did not become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  AWX Operator controller ready\n")

	// Step 3: Apply DB init job (creates AWX database in shared PostgreSQL).
	if err := applyAWXDBInit(namespace, kubeconfigPath, writer); err != nil {
		return err
	}

	// Step 4: Create prerequisite secrets for the AWX CR.
	if err := createAWXSecrets(namespace, kubeconfigPath, writer); err != nil {
		return err
	}

	// Step 5: Apply AWX Custom Resource — the operator handles migrations,
	// admin user, web, task, Receptor, EE, Redis, instance registration, etc.
	if err := applyAWXCustomResource(namespace, kubeconfigPath, writer); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(writer, "AWX CR applied — operator will reconcile\n")
	return nil
}

// installAWXOperator installs the AWX Operator CRDs and controller using the
// official kustomize-based installation method.
func installAWXOperator(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  Installing AWX Operator %s...\n", AWXOperatorVersion)

	tmpDir, err := os.MkdirTemp("", "awx-operator-kustomize-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for kustomize: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	kustomization := fmt.Sprintf(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - github.com/ansible/awx-operator/config/default?ref=%s
images:
  - name: quay.io/ansible/awx-operator
    newTag: %s
  - name: gcr.io/kubebuilder/kube-rbac-proxy
    newName: registry.k8s.io/kubebuilder/kube-rbac-proxy
    newTag: v0.15.0
namespace: %s
`, AWXOperatorVersion, AWXOperatorVersion, namespace)

	if writeErr := os.WriteFile(filepath.Join(tmpDir, "kustomization.yaml"), []byte(kustomization), 0600); writeErr != nil {
		return fmt.Errorf("failed to write kustomization.yaml: %w", writeErr)
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-k", tmpDir)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install AWX Operator: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  AWX Operator CRDs + controller applied\n")
	return nil
}

// applyAWXDBInit applies the DB init Job that creates the AWX database and
// user in the shared PostgreSQL instance.
func applyAWXDBInit(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  Applying AWX DB init job...\n")

	dbInitManifest := fmt.Sprintf(`---
apiVersion: batch/v1
kind: Job
metadata:
  name: awx-db-init
  namespace: %[1]s
spec:
  backoffLimit: 10
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: init
        image: docker.io/library/postgres:16-alpine
        command: ["sh", "-c"]
        args:
        - |
          until pg_isready -h postgresql -p 5432 -U slm_user; do
            echo "Waiting for PostgreSQL..."
            sleep 2
          done
          echo "Creating AWX database and user..."
          export PGPASSWORD=test_password
          psql -h postgresql -p 5432 -U slm_user -d action_history -c "CREATE ROLE %[2]s WITH LOGIN PASSWORD '%[3]s';" 2>&1 || true
          set -e
          psql -h postgresql -p 5432 -U slm_user -d action_history -tc "SELECT 1 FROM pg_database WHERE datname = '%[4]s'" | grep -q 1 || psql -h postgresql -p 5432 -U slm_user -d action_history -c "CREATE DATABASE %[4]s OWNER %[2]s;"
          psql -h postgresql -p 5432 -U slm_user -d action_history -c "GRANT ALL PRIVILEGES ON DATABASE %[4]s TO %[2]s;"
          echo "AWX database ready."
`, namespace, AWXDatabaseUser, AWXDatabasePass, AWXDatabaseName)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "--server-side", "--field-manager=e2e-test", "-f", "-")
	cmd.Stdin = strings.NewReader(dbInitManifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply AWX DB init job: %w", err)
	}
	return nil
}

// createAWXSecrets creates the K8s Secrets the AWX CR references:
// external PostgreSQL connection, admin password, and Django secret key.
func createAWXSecrets(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  Creating AWX prerequisite secrets...\n")

	secretsManifest := fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: awx-postgres-configuration
  namespace: %[1]s
stringData:
  host: postgresql
  port: "5432"
  database: "%[2]s"
  username: "%[3]s"
  password: "%[4]s"
  sslmode: prefer
  type: unmanaged
---
apiVersion: v1
kind: Secret
metadata:
  name: %[5]s-admin-password
  namespace: %[1]s
stringData:
  password: "%[6]s"
---
apiVersion: v1
kind: Secret
metadata:
  name: %[5]s-secret-key
  namespace: %[1]s
stringData:
  secret_key: "%[7]s"
`, namespace, AWXDatabaseName, AWXDatabaseUser, AWXDatabasePass,
		AWXInstanceName, AWXAdminPass, AWXSecretKey)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "--server-side", "--field-manager=e2e-test", "-f", "-")
	cmd.Stdin = strings.NewReader(secretsManifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create AWX prerequisite secrets: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  AWX secrets created (PG config, admin password, secret key)\n")
	return nil
}

// applyAWXCustomResource creates the AWX CR that the operator reconciles into
// a fully functional AWX deployment (web, task, Receptor, EE, Redis, RBAC, etc.).
func applyAWXCustomResource(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  Applying AWX Custom Resource (%s)...\n", AWXInstanceName)

	awxCR := fmt.Sprintf(`---
apiVersion: awx.ansible.com/v1beta1
kind: AWX
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  service_type: nodeport
  nodeport_port: %[3]d
  admin_user: %[4]s
  admin_password_secret: %[1]s-admin-password
  secret_key_secret: %[1]s-secret-key
  postgres_configuration_secret: awx-postgres-configuration
  image: quay.io/ansible/awx
  image_version: "%[5]s"
  ee_images:
    - name: "AWX EE (latest)"
      image: "quay.io/ansible/awx-ee:%[5]s"
  web_resource_requirements:
    requests:
      cpu: 100m
      memory: 512Mi
    limits:
      cpu: "1"
      memory: 2Gi
  task_resource_requirements:
    requests:
      cpu: 100m
      memory: 512Mi
    limits:
      cpu: "1"
      memory: 2Gi
  ee_resource_requirements:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 256Mi
  redis_resource_requirements:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 128Mi
  init_container_resource_requirements:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
`, AWXInstanceName, namespace, AWXNodePort, AWXAdminUser, AWXImageVersion)

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "--server-side", "--field-manager=e2e-test", "-f", "-")
	cmd.Stdin = strings.NewReader(awxCR)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply AWX Custom Resource: %w", err)
	}
	return nil
}

// WaitForAWXReady waits until the operator-managed AWX pod is running and
// all containers are ready (web, task, EE, redis, rsyslog).
// Uses manual polling instead of Gomega Eventually because this function
// is called from goroutines where Gomega panics crash the process.
func WaitForAWXReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "Waiting for AWX (operator-managed) to be ready...\n")

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	deadline := time.Now().Add(12 * time.Minute)
	for time.Now().Before(deadline) {
		pods, listErr := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/managed-by=awx-operator,app.kubernetes.io/name=" + AWXInstanceName,
		})
		if listErr == nil && len(pods.Items) > 0 {
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					for _, c := range pod.Status.Conditions {
						if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
							_, _ = fmt.Fprintf(writer, "AWX is ready (all containers running)\n")
							return nil
						}
					}
				}
			}
		}
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("AWX pod did not become ready within 12 minutes")
}

// awxAPIRequest makes an authenticated request to the AWX API.
func awxAPIRequest(method, url string, body interface{}, token string) (map[string]interface{}, int, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.SetBasicAuth(AWXAdminUser, AWXAdminPass)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, nil
	}
	return result, resp.StatusCode, nil
}

// AWXConfig holds the IDs created during AWX configuration.
type AWXConfig struct {
	APIURL              string
	OrganizationID      int
	ProjectID           int
	InventoryID         int
	SuccessTemplateID   int
	FailureTemplateID   int
	Token               string
}

// ConfigureAWX creates the project, inventory, job templates, and API token in AWX.
// Must be called after AWX is ready. Returns the configuration needed by the WE controller.
func ConfigureAWX(ctx context.Context, awxBaseURL string, writer io.Writer) (*AWXConfig, error) {
	_, _ = fmt.Fprintf(writer, "\n🔧 Configuring AWX at %s...\n", awxBaseURL)

	cfg := &AWXConfig{APIURL: awxBaseURL}

	// 1. Get default organization (always exists as ID 1)
	_, _ = fmt.Fprintf(writer, "   Creating organization...\n")
	orgBody := map[string]interface{}{"name": "Kubernaut E2E", "description": "E2E test organization"}
	orgResult, orgStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/organizations/", orgBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}
	if orgStatus == http.StatusCreated {
		cfg.OrganizationID = int(orgResult["id"].(float64))
	} else {
		// Organization might already exist; use default (ID 1)
		cfg.OrganizationID = 1
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Organization ID: %d\n", cfg.OrganizationID)

	// 2. Create project (Git SCM pointing to test playbooks repo)
	_, _ = fmt.Fprintf(writer, "   Creating project (Git: %s)...\n", AWXTestPlaybooksRepo)
	projectBody := map[string]interface{}{
		"name":             "kubernaut-test-playbooks",
		"description":      "E2E test playbooks for Kubernaut Ansible engine",
		"organization":     cfg.OrganizationID,
		"scm_type":         "git",
		"scm_url":          AWXTestPlaybooksRepo,
		"scm_branch":       AWXTestPlaybooksCommit,
		"scm_update_on_launch": true,
	}
	projectResult, projectStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/projects/", projectBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	if projectStatus != http.StatusCreated {
		return nil, fmt.Errorf("failed to create project: HTTP %d", projectStatus)
	}
	cfg.ProjectID = int(projectResult["id"].(float64))
	_, _ = fmt.Fprintf(writer, "   ✅ Project ID: %d\n", cfg.ProjectID)

	// 3. Wait for project sync to complete (manual polling — called from goroutine)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for project sync...\n")
	syncDeadline := time.Now().Add(3 * time.Minute)
	projectSynced := false
	for time.Now().Before(syncDeadline) {
		result, _, _ := awxAPIRequest("GET", fmt.Sprintf("%s/api/v2/projects/%d/", awxBaseURL, cfg.ProjectID), nil, "")
		if result != nil {
			if status, _ := result["status"].(string); status == "successful" {
				projectSynced = true
				break
			} else if status == "failed" || status == "error" {
				return nil, fmt.Errorf("project sync failed with status: %s", status)
			}
		}
		time.Sleep(5 * time.Second)
	}
	if !projectSynced {
		return nil, fmt.Errorf("project sync did not complete within 3 minutes")
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Project synced\n")

	// 4. Create inventory
	_, _ = fmt.Fprintf(writer, "   Creating inventory...\n")
	invBody := map[string]interface{}{
		"name":         "localhost",
		"description":  "Local execution for E2E tests",
		"organization": cfg.OrganizationID,
	}
	invResult, invStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/inventories/", invBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create inventory: %w", err)
	}
	if invStatus != http.StatusCreated {
		return nil, fmt.Errorf("failed to create inventory: HTTP %d", invStatus)
	}
	cfg.InventoryID = int(invResult["id"].(float64))
	_, _ = fmt.Fprintf(writer, "   ✅ Inventory ID: %d\n", cfg.InventoryID)

	// Add localhost host to inventory
	hostBody := map[string]interface{}{
		"name":      "localhost",
		"variables": "ansible_connection: local\nansible_python_interpreter: /usr/bin/python3",
	}
	_, hostStatus, err := awxAPIRequest("POST", fmt.Sprintf("%s/api/v2/inventories/%d/hosts/", awxBaseURL, cfg.InventoryID), hostBody, "")
	if err != nil || (hostStatus != http.StatusCreated && hostStatus != http.StatusBadRequest) {
		return nil, fmt.Errorf("failed to add host to inventory: HTTP %d, err: %v", hostStatus, err)
	}

	// 5. Create success job template
	_, _ = fmt.Fprintf(writer, "   Creating success job template...\n")
	successBody := map[string]interface{}{
		"name":         "kubernaut-test-success",
		"description":  "E2E test: successful remediation playbook",
		"project":      cfg.ProjectID,
		"playbook":     "playbooks/test-success.yml",
		"inventory":    cfg.InventoryID,
		"ask_variables_on_launch": true,
	}
	successResult, successStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/job_templates/", successBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create success template: %w", err)
	}
	if successStatus != http.StatusCreated {
		return nil, fmt.Errorf("failed to create success template: HTTP %d", successStatus)
	}
	cfg.SuccessTemplateID = int(successResult["id"].(float64))
	_, _ = fmt.Fprintf(writer, "   ✅ Success Job Template ID: %d\n", cfg.SuccessTemplateID)

	// 6. Create failure job template
	_, _ = fmt.Fprintf(writer, "   Creating failure job template...\n")
	failureBody := map[string]interface{}{
		"name":         "kubernaut-test-failure",
		"description":  "E2E test: intentionally failing remediation playbook",
		"project":      cfg.ProjectID,
		"playbook":     "playbooks/test-failure.yml",
		"inventory":    cfg.InventoryID,
		"ask_variables_on_launch": true,
	}
	failureResult, failureStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/job_templates/", failureBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create failure template: %w", err)
	}
	if failureStatus != http.StatusCreated {
		return nil, fmt.Errorf("failed to create failure template: HTTP %d", failureStatus)
	}
	cfg.FailureTemplateID = int(failureResult["id"].(float64))
	_, _ = fmt.Fprintf(writer, "   ✅ Failure Job Template ID: %d\n", cfg.FailureTemplateID)

	// 7. Create API token for WE controller
	_, _ = fmt.Fprintf(writer, "   Creating API token...\n")
	tokenBody := map[string]interface{}{
		"description": "Kubernaut WE controller E2E token",
		"scope":       "write",
	}
	tokenResult, tokenStatus, err := awxAPIRequest("POST", awxBaseURL+"/api/v2/users/1/personal_tokens/", tokenBody, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create API token: %w", err)
	}
	if tokenStatus != http.StatusCreated {
		return nil, fmt.Errorf("failed to create API token: HTTP %d", tokenStatus)
	}
	cfg.Token = tokenResult["token"].(string)
	_, _ = fmt.Fprintf(writer, "   ✅ API token created\n")

	_, _ = fmt.Fprintf(writer, "✅ AWX configuration complete\n")
	return cfg, nil
}

// CreateAWXTokenSecret creates a K8s Secret containing the AWX API token
// for the WE controller to read via tokenSecretRef.
func CreateAWXTokenSecret(ctx context.Context, namespace, kubeconfigPath, token string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔑 Creating AWX token Secret (%s)...\n", AWXTokenSecretName)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AWXTokenSecretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"token": token,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create AWX token Secret: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ AWX token Secret created\n")
	return nil
}

// PatchWEControllerWithAnsibleConfig patches the WE controller ConfigMap to add
// the ansible configuration and restarts the controller to pick it up.
func PatchWEControllerWithAnsibleConfig(ctx context.Context, namespace, kubeconfigPath string, awxCfg *AWXConfig, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔧 Patching WE controller config with ansible settings...\n")

	// Read current ConfigMap, append ansible section, and replace
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, "workflowexecution-config", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get WE ConfigMap: %w", err)
	}

	currentConfig := cm.Data["workflowexecution.yaml"]
	ansibleSection := fmt.Sprintf(`
    ansible:
      apiURL: "http://%s.%s:%d"
      tokenSecretRef:
        name: "%s"
        namespace: "%s"
        key: "token"
      insecure: true`, AWXServiceName, namespace, AWXServicePort, AWXTokenSecretName, namespace)

	cm.Data["workflowexecution.yaml"] = currentConfig + ansibleSection

	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update WE ConfigMap: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ ConfigMap updated with ansible config\n")

	// Rollout restart the WE controller to pick up the new config
	_, _ = fmt.Fprintf(writer, "   🔄 Restarting WE controller...\n")
	restartCmd := exec.Command("kubectl", "rollout", "restart",
		"deployment/workflowexecution-controller",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
	)
	restartCmd.Stdout = writer
	restartCmd.Stderr = writer
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("failed to restart WE controller: %w", err)
	}

	// Wait for rollout to complete
	waitCmd := exec.Command("kubectl", "rollout", "status",
		"deployment/workflowexecution-controller",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"--timeout=120s",
	)
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("WE controller rollout did not complete: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ WE controller restarted with ansible executor enabled\n")
	return nil
}

// SetupAWXPostDeployment waits for AWX to be ready, configures it (project,
// inventory, job templates), and creates the API token Secret in K8s.
// Designed to run in parallel with workflow seeding since they share no dependencies.
// The caller is responsible for calling PatchWEControllerWithAnsibleConfig afterwards.
func SetupAWXPostDeployment(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) (*AWXConfig, error) {
	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🤖 AWX Post-Deployment Configuration (BR-WE-015)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Step 1: Wait for AWX to be ready
	if err := WaitForAWXReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return nil, fmt.Errorf("AWX not ready: %w", err)
	}

	// Step 2: Configure AWX (project, inventory, job templates, token)
	awxBaseURL := fmt.Sprintf("http://localhost:%d", AWXNodePort)
	awxCfg, err := ConfigureAWX(ctx, awxBaseURL, writer)
	if err != nil {
		return nil, fmt.Errorf("AWX configuration failed: %w", err)
	}

	// Step 3: Create token Secret in K8s
	if err := CreateAWXTokenSecret(ctx, namespace, kubeconfigPath, awxCfg.Token, writer); err != nil {
		return nil, fmt.Errorf("failed to create AWX token Secret: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ AWX configured + token Secret created")
	_, _ = fmt.Fprintf(writer, "   AWX API: %s\n", awxCfg.APIURL)
	_, _ = fmt.Fprintf(writer, "   Success Template: kubernaut-test-success (ID: %d)\n", awxCfg.SuccessTemplateID)
	_, _ = fmt.Fprintf(writer, "   Failure Template: kubernaut-test-failure (ID: %d)\n", awxCfg.FailureTemplateID)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return awxCfg, nil
}
