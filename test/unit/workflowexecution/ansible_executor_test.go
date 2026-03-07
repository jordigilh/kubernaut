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

package workflowexecution

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ========================================
// BR-WE-015: AnsibleExecutor Unit Tests
// ========================================
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/45/TEST_PLAN.md
// ========================================

// mockAWXClient implements executor.AWXClient for unit testing.
type mockAWXClient struct {
	launchFunc            func(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error)
	getStatusFunc         func(ctx context.Context, jobID int) (*executor.AWXJobStatus, error)
	cancelFunc            func(ctx context.Context, jobID int) error
	findTemplateByNameFn  func(ctx context.Context, name string) (int, error)
}

func (m *mockAWXClient) LaunchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error) {
	if m.launchFunc != nil {
		return m.launchFunc(ctx, templateID, extraVars)
	}
	return 42, nil
}

func (m *mockAWXClient) GetJobStatus(ctx context.Context, jobID int) (*executor.AWXJobStatus, error) {
	if m.getStatusFunc != nil {
		return m.getStatusFunc(ctx, jobID)
	}
	return &executor.AWXJobStatus{ID: jobID, Status: "successful"}, nil
}

func (m *mockAWXClient) CancelJob(ctx context.Context, jobID int) error {
	if m.cancelFunc != nil {
		return m.cancelFunc(ctx, jobID)
	}
	return nil
}

func (m *mockAWXClient) FindJobTemplateByName(ctx context.Context, name string) (int, error) {
	if m.findTemplateByNameFn != nil {
		return m.findTemplateByNameFn(ctx, name)
	}
	return 10, nil
}

func newAnsibleWFE(name, namespace string, engineConfigJSON []byte, params map[string]string) *workflowexecutionv1alpha1.WorkflowExecution {
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			ExecutionEngine: "ansible",
			TargetResource:  namespace + "/deployment/test-app",
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:      "ansible-restart",
				Version:         "1.0.0",
				ExecutionBundle: "https://github.com/kubernaut/playbooks.git",
				EngineConfig: &apiextensionsv1.JSON{
					Raw: engineConfigJSON,
				},
			},
			Parameters: params,
		},
	}
}

var _ = Describe("AnsibleExecutor (BR-WE-015)", func() {
	var (
		ansibleExec *executor.AnsibleExecutor
		awxClient   *mockAWXClient
		ctx         context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockAWXClient{}
		ansibleExec = executor.NewAnsibleExecutor(awxClient, ctrl.Log.WithName("test"))
	})

	It("should return engine name 'ansible'", func() {
		Expect(ansibleExec.Engine()).To(Equal("ansible"))
	})

	Context("Create", func() {
		It("UT-WE-015-001: should launch AWX job with correct extra_vars from workflow parameters", func() {
			var capturedExtraVars map[string]interface{}
			var capturedTemplateID int
			awxClient.findTemplateByNameFn = func(_ context.Context, name string) (int, error) {
				Expect(name).To(Equal("restart-pod"))
				return 99, nil
			}
			awxClient.launchFunc = func(_ context.Context, templateID int, extraVars map[string]interface{}) (int, error) {
				capturedTemplateID = templateID
				capturedExtraVars = extraVars
				return 42, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
				"inventoryName":   "production",
			})

			wfe := newAnsibleWFE("test-wfe", "default", engineConfig, map[string]string{
				"NAMESPACE": "default",
				"REPLICAS":  "3",
			})

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(ContainSubstring("awx-job-"))
			Expect(capturedTemplateID).To(Equal(99))
			Expect(capturedExtraVars).To(HaveKeyWithValue("NAMESPACE", "default"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("REPLICAS", BeNumerically("==", 3)))
		})

		It("UT-WE-015-004: should reject WFE without engineConfig", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "no-config", Namespace: "default"},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "ansible",
					TargetResource:  "default/deployment/app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      "ansible-test",
						ExecutionBundle: "https://github.com/kubernaut/playbooks.git",
					},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "default", executor.CreateOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("engineConfig"))
		})

		It("UT-WE-015-005: should propagate AWX API errors", func() {
			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("AWX API 401: unauthorized")
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/restart.yml",
				"jobTemplateName": "restart-pod",
			})
			wfe := newAnsibleWFE("error-wfe", "default", engineConfig, nil)

			_, err := ansibleExec.Create(ctx, wfe, "default", executor.CreateOptions{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("AWX"))
		})
	})

	Context("GetStatus", func() {
		It("UT-WE-015-002: should map all 7 AWX states to correct WFE phases", func() {
			testCases := []struct {
				awxStatus     string
				expectedPhase string
			}{
				{"pending", workflowexecutionv1alpha1.PhasePending},
				{"waiting", workflowexecutionv1alpha1.PhasePending},
				{"running", workflowexecutionv1alpha1.PhaseRunning},
				{"successful", workflowexecutionv1alpha1.PhaseCompleted},
				{"failed", workflowexecutionv1alpha1.PhaseFailed},
				{"error", workflowexecutionv1alpha1.PhaseFailed},
				{"canceled", workflowexecutionv1alpha1.PhaseFailed},
			}

			for _, tc := range testCases {
				awxClient.getStatusFunc = func(_ context.Context, jobID int) (*executor.AWXJobStatus, error) {
					return &executor.AWXJobStatus{ID: jobID, Status: tc.awxStatus}, nil
				}

				wfe := &workflowexecutionv1alpha1.WorkflowExecution{
					ObjectMeta: metav1.ObjectMeta{Name: "status-wfe", Namespace: "default"},
					Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
						ExecutionRef: &corev1.LocalObjectReference{Name: "awx-job-42"},
					},
				}

				result, err := ansibleExec.GetStatus(ctx, wfe, "default")
				Expect(err).ToNot(HaveOccurred(), "AWX status %q should not error", tc.awxStatus)
				Expect(result.Phase).To(Equal(tc.expectedPhase),
					"AWX status %q should map to %q, got %q", tc.awxStatus, tc.expectedPhase, result.Phase)
				Expect(result.Summary.Status).To(Equal(tc.expectedPhase), "summary status should match phase")
			}
		})

		It("should return error when executionRef is missing", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "no-ref", Namespace: "default"},
			}

			_, err := ansibleExec.GetStatus(ctx, wfe, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("execution ref"))
		})
	})

	Context("Cleanup", func() {
		It("should cancel AWX job", func() {
			var cancelledJobID int
			awxClient.cancelFunc = func(_ context.Context, jobID int) error {
				cancelledJobID = jobID
				return nil
			}

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-wfe", Namespace: "default"},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "awx-job-99"},
				},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(cancelledJobID).To(Equal(99))
		})

		It("should be no-op when executionRef is nil", func() {
			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "no-ref", Namespace: "default"},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

var _ = Describe("BuildExtraVars (BR-WE-015)", func() {
	It("UT-WE-015-003: should coerce string parameters to typed values", func() {
		params := map[string]string{
			"REPLICAS":  "3",
			"ENABLED":   "true",
			"THRESHOLD": "0.75",
			"NAME":      "hello",
			"LIST":      `[1,2,3]`,
		}

		extraVars := executor.BuildExtraVars(params)
		Expect(extraVars).To(HaveLen(5), "all parameters should be converted to extra_vars")
		Expect(extraVars["REPLICAS"]).To(BeNumerically("==", 3))
		Expect(extraVars["ENABLED"]).To(BeTrue())
		Expect(extraVars["THRESHOLD"]).To(BeNumerically("~", 0.75, 0.001))
		Expect(extraVars["NAME"]).To(Equal("hello"))
		Expect(extraVars["LIST"]).To(Equal([]interface{}{float64(1), float64(2), float64(3)}))
	})

	It("should return nil for empty params", func() {
		extraVars := executor.BuildExtraVars(nil)
		Expect(extraVars).To(BeNil())
	})
})

var _ = Describe("MapAWXStatusToResult (BR-WE-015)", func() {
	It("UT-WE-015-002b: should include stdout in failure message", func() {
		status := &executor.AWXJobStatus{
			ID:           42,
			Status:       "failed",
			Failed:       true,
			ResultStdout: "TASK [restart] fatal: connection refused",
		}

		result := executor.MapAWXStatusToResult(status)
		Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhaseFailed))
		Expect(result.Message).To(ContainSubstring("connection refused"))
		Expect(result.Summary.Status).To(Equal(workflowexecutionv1alpha1.PhaseFailed), "summary should reflect failure phase")
	})

	It("should handle unknown AWX status gracefully", func() {
		status := &executor.AWXJobStatus{ID: 1, Status: "unknown-state"}

		result := executor.MapAWXStatusToResult(status)
		Expect(result.Phase).To(Equal(workflowexecutionv1alpha1.PhasePending))
		Expect(result.Message).To(ContainSubstring("unknown-state"))
	})
})
