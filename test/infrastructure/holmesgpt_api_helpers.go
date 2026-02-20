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
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// waitForDataStorageReady waits for DataStorage pod to be ready and responsive
// Pattern: Matches AA E2E waitForDataStorageReady logic
func waitForDataStorageReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  ⏳ Waiting for DataStorage pod to be ready...\n")

	// Load kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Wait for DataStorage pod to be Ready (5 minutes timeout)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage", // Note: no hyphen (matches deployment label)
		})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if len(pods.Items) > 0 {
			pod := pods.Items[0]
			for _, cond := range pod.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					_, _ = fmt.Fprintf(writer, "  ✅ DataStorage pod ready: %s\n", pod.Name)

					// Also verify HTTP endpoint is responsive
					_, _ = fmt.Fprintf(writer, "  ⏳ Verifying DataStorage HTTP endpoint...\n")
					httpDeadline := time.Now().Add(2 * time.Minute)
					for time.Now().Before(httpDeadline) {
						resp, err := http.Get("http://localhost:8089/health/ready")
						if err == nil && resp.StatusCode == http.StatusOK {
							resp.Body.Close()
							_, _ = fmt.Fprintf(writer, "  ✅ DataStorage HTTP endpoint ready\n")
							return nil
						}
						if resp != nil {
							resp.Body.Close()
						}
						time.Sleep(2 * time.Second)
					}
					return fmt.Errorf("DataStorage pod ready but HTTP endpoint not responding after 2 minutes")
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("DataStorage pod not ready after 5 minutes")
}

// createAuthenticatedDataStorageClient creates an authenticated OpenAPI client for DataStorage
// Pattern: Matches AA E2E client creation (aianalysis_e2e.go lines 271-280)
// DD-AUTH-014: Uses ServiceAccount token for authentication
func createAuthenticatedDataStorageClient(dataStorageURL, saToken string) (*ogenclient.Client, error) {
	client, err := ogenclient.NewClient(
		dataStorageURL,
		ogenclient.WithClient(&http.Client{
			Transport: testauth.NewServiceAccountTransport(saToken),
			Timeout:   30 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataStorage client: %w", err)
	}

	return client, nil
}

// GetHAPIE2ETestWorkflows returns workflows for HAPI E2E tests
// Pattern: Inlined workflow definitions (CANNOT use test/e2e/holmesgpt-api - import cycle)
// Similar to AA E2E approach (aianalysis_e2e.go:287-296)
// Source of truth: test/e2e/holmesgpt-api/test_workflows.go:GetHAPIE2ETestWorkflows()
// Acceptable trade-off: Small duplication avoids architectural issues
func GetHAPIE2ETestWorkflows() []TestWorkflow {
	// BR-HAPI-191: SchemaParameters MUST match Mock LLM scenario parameters
	// HAPI validates LLM response parameters against workflow schema from DataStorage
	// DD-WORKFLOW-016: ActionType values MUST match Python fixtures (workflow_fixtures.py)
	// DD-WORKFLOW-017: SchemaImage references real OCI images at quay.io/kubernaut-cicd/test-workflows
	// These images contain /workflow-schema.yaml (BR-WORKFLOW-004) for pullspec-only registration
	const hapiWorkflowRegistry = "quay.io/kubernaut-cicd/test-workflows"
	baseWorkflows := []TestWorkflow{
		{WorkflowID: "oomkill-increase-memory-v1", Name: "OOMKill Remediation - Increase Memory Limits", Description: "Increases memory limits for pods experiencing OOMKilled events", ActionType: "IncreaseMemoryLimits", SignalType: "OOMKilled", Severity: "critical", Component: "pod", Priority: "P0", SchemaImage: hapiWorkflowRegistry + "/oomkill-increase-memory:v1.0.0",
			// DD-WORKFLOW-017: SchemaParameters mirror OCI image's /workflow-schema.yaml for documentation.
			// Actual schema comes from OCI image via pullspec-only registration.
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace containing the affected deployment"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to update memory limits"},
				{Name: "MEMORY_INCREASE_PERCENT", Type: "integer", Required: false, Description: "Percentage to increase memory limits by"},
			}},
		{WorkflowID: "memory-optimize-v1", Name: "OOMKill Remediation - Scale Down Replicas", Description: "Reduces replica count for deployments experiencing OOMKilled", ActionType: "ScaleReplicas", SignalType: "OOMKilled", Severity: "high", Component: "deployment", Priority: "P1", SchemaImage: hapiWorkflowRegistry + "/memory-optimize:v1.0.0",
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to scale"},
				{Name: "REPLICA_COUNT", Type: "integer", Required: false, Description: "Target number of replicas"},
			}},
		{WorkflowID: "crashloop-config-fix-v1", Name: "CrashLoopBackOff - Fix Configuration", Description: "Identifies and fixes configuration issues causing CrashLoopBackOff", ActionType: "RestartDeployment", SignalType: "CrashLoopBackOff", Severity: "high", Component: "pod", Priority: "P1", SchemaImage: hapiWorkflowRegistry + "/crashloop-config-fix:v1.0.0",
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "DEPLOYMENT_NAME", Type: "string", Required: true, Description: "Name of the deployment to restart"},
				{Name: "GRACE_PERIOD_SECONDS", Type: "integer", Required: false, Description: "Graceful shutdown period in seconds"},
			}},
		{WorkflowID: "node-drain-reboot-v1", Name: "NodeNotReady - Drain and Reboot", Description: "Safely drains and reboots nodes in NotReady state", ActionType: "DrainNode", SignalType: "NodeNotReady", Severity: "critical", Component: "node", Priority: "P0", SchemaImage: hapiWorkflowRegistry + "/node-drain-reboot:v1.0.0",
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NODE_NAME", Type: "string", Required: true, Description: "Name of the node to drain and reboot"},
				{Name: "DRAIN_TIMEOUT_SECONDS", Type: "integer", Required: false, Description: "Timeout for drain operation in seconds"},
			}},
		{WorkflowID: "image-pull-backoff-fix-credentials", Name: "ImagePullBackOff - Fix Registry Credentials", Description: "Fixes ImagePullBackOff errors by updating registry credentials", ActionType: "RollbackDeployment", SignalType: "ImagePullBackOff", Severity: "high", Component: "pod", Priority: "P1", SchemaImage: hapiWorkflowRegistry + "/imagepull-fix-creds:v1.0.0"},
		{WorkflowID: "generic-restart-v1", Name: "Generic Pod Restart", Description: "Generic pod restart for unknown issues", ActionType: "RestartPod", SignalType: "Unknown", Severity: "medium", Component: "deployment", Priority: "P2", SchemaImage: hapiWorkflowRegistry + "/generic-restart:v1.0.0",
			SchemaParameters: []models.WorkflowParameter{
				{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				{Name: "POD_NAME", Type: "string", Required: true, Description: "Name of the pod to restart"},
			}},
	}

	// Create workflow instances for BOTH staging AND production
	// (Matches AIAnalysis pattern - test/integration/aianalysis/test_workflows.go:114-136)
	var allWorkflows []TestWorkflow
	for _, wf := range baseWorkflows {
		// Staging version
		stagingWf := wf
		stagingWf.Environment = "staging"
		allWorkflows = append(allWorkflows, stagingWf)

		// Production version
		prodWf := wf
		prodWf.Environment = "production"
		allWorkflows = append(allWorkflows, prodWf)
	}

	return allWorkflows
}
