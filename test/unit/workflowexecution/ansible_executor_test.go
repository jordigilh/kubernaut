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
	"k8s.io/apimachinery/pkg/runtime"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
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
	launchFunc                func(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error)
	getStatusFunc             func(ctx context.Context, jobID int) (*executor.AWXJobStatus, error)
	cancelFunc                func(ctx context.Context, jobID int) error
	findTemplateByNameFn      func(ctx context.Context, name string) (int, error)
	createCredentialTypeFn    func(ctx context.Context, name string, inputs, injectors map[string]interface{}) (int, error)
	findCredentialTypeByNameFn func(ctx context.Context, name string) (int, error)
	createCredentialFn        func(ctx context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error)
	deleteCredentialFn        func(ctx context.Context, credentialID int) error
	launchWithCredsFn         func(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int) (int, error)
	getTemplateCredsFn        func(ctx context.Context, templateID int) ([]int, error)
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

func (m *mockAWXClient) CreateCredentialType(ctx context.Context, name string, inputs, injectors map[string]interface{}) (int, error) {
	if m.createCredentialTypeFn != nil {
		return m.createCredentialTypeFn(ctx, name, inputs, injectors)
	}
	return 1, nil
}

func (m *mockAWXClient) FindCredentialTypeByName(ctx context.Context, name string) (int, error) {
	if m.findCredentialTypeByNameFn != nil {
		return m.findCredentialTypeByNameFn(ctx, name)
	}
	return 1, nil
}

func (m *mockAWXClient) CreateCredential(ctx context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error) {
	if m.createCredentialFn != nil {
		return m.createCredentialFn(ctx, name, credTypeID, orgID, inputs)
	}
	return 42, nil
}

func (m *mockAWXClient) DeleteCredential(ctx context.Context, credentialID int) error {
	if m.deleteCredentialFn != nil {
		return m.deleteCredentialFn(ctx, credentialID)
	}
	return nil
}

func (m *mockAWXClient) LaunchJobTemplateWithCreds(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int) (int, error) {
	if m.launchWithCredsFn != nil {
		return m.launchWithCredsFn(ctx, templateID, extraVars, credentialIDs)
	}
	return 42, nil
}

func (m *mockAWXClient) GetJobTemplateCredentials(ctx context.Context, templateID int) ([]int, error) {
	if m.getTemplateCredsFn != nil {
		return m.getTemplateCredsFn(ctx, templateID)
	}
	return nil, nil
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
			RemediationRequestRef: corev1.ObjectReference{
				APIVersion: "remediationorchestrator.kubernaut.ai/v1alpha1",
				Kind:       "RemediationRequest",
				Name:       "rr-" + name,
				Namespace:  namespace,
			},
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
		ansibleExec = executor.NewAnsibleExecutor(awxClient, nil, 1, ctrl.Log.WithName("test"))
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

		It("UT-WE-015-006: should inject WFE and RR context into extra_vars (#311, #313)", func() {
			var capturedExtraVars map[string]interface{}
			awxClient.findTemplateByNameFn = func(_ context.Context, name string) (int, error) {
				return 10, nil
			}
			awxClient.launchFunc = func(_ context.Context, _ int, extraVars map[string]interface{}) (int, error) {
				capturedExtraVars = extraVars
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/gitops-update-memory.yml",
				"jobTemplateName": "kubernaut-gitops-update-memory",
			})

			wfe := newAnsibleWFE("we-rr-abc123", "kubernaut-workflows", engineConfig, map[string]string{
				"TARGET_NAMESPACE": "demo-ns",
				"NEW_MEMORY_LIMIT": "512Mi",
			})

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedExtraVars).To(HaveKeyWithValue("WFE_NAME", "we-rr-abc123"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("WFE_NAMESPACE", "kubernaut-workflows"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("RR_NAME", "rr-we-rr-abc123"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("RR_NAMESPACE", "kubernaut-workflows"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("TARGET_NAMESPACE", "demo-ns"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("NEW_MEMORY_LIMIT", "512Mi"))
		})

		It("UT-WE-015-007: should inject empty RR context when RemediationRequestRef is unset (#313)", func() {
			var capturedExtraVars map[string]interface{}
			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) {
				return 10, nil
			}
			awxClient.launchFunc = func(_ context.Context, _ int, extraVars map[string]interface{}) (int, error) {
				capturedExtraVars = extraVars
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "wfe-no-rr", Namespace: "default"},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					ExecutionEngine: "ansible",
					TargetResource:  "default/deployment/app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:      "ansible-test",
						ExecutionBundle: "https://github.com/kubernaut/playbooks.git",
						EngineConfig:    &apiextensionsv1.JSON{Raw: engineConfig},
					},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "default", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedExtraVars).To(HaveKeyWithValue("WFE_NAME", "wfe-no-rr"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("WFE_NAMESPACE", "default"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("RR_NAME", ""))
			Expect(capturedExtraVars).To(HaveKeyWithValue("RR_NAMESPACE", ""))
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

// ========================================
// BR-WE-015: AnsibleExecutor dependencies.secrets injection
// ========================================
// Tests the canonical dependencies.secrets abstraction for the ansible engine:
// K8s Secret -> dynamic AWX credential type -> ephemeral credential -> launch with creds -> status
// ========================================

var _ = Describe("AnsibleExecutor dependencies.secrets injection (BR-WE-015)", func() {
	var (
		ctx         context.Context
		awxClient   *mockAWXClient
		fakeClient  client.Client
		ansibleExec *executor.AnsibleExecutor
	)

	newFakeClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
			Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockAWXClient{}
	})

	Context("Create with dependencies.secrets", func() {
		It("UT-WE-015-030: should read K8s Secret, create credential type + credential, and launch with creds", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/gitops-update-memory-limits.yml",
				"jobTemplateName": "kubernaut-gitops-update-memory",
			})

			wfe := newAnsibleWFE("we-rr-test-deps", "kubernaut-system", engineConfig, map[string]string{
				"TARGET_NAMESPACE": "demo-ns",
				"NEW_MEMORY_LIMIT": "512Mi",
			})

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			var createdCredTypeName string
			var createdCredName string
			var launchedWithCreds []int

			awxClient.findTemplateByNameFn = func(_ context.Context, name string) (int, error) {
				return 10, nil
			}
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				return 0, fmt.Errorf("AWX credential type %q not found", name)
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, name string, inputs, injectors map[string]interface{}) (int, error) {
				createdCredTypeName = name
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error) {
				createdCredName = name
				Expect(credTypeID).To(Equal(15))
				Expect(orgID).To(Equal(1))
				Expect(inputs).To(HaveKeyWithValue("username", "admin"))
				Expect(inputs).To(HaveKeyWithValue("password", "secret123"))
				return 42, nil
			}
			awxClient.launchWithCredsFn = func(_ context.Context, templateID int, extraVars map[string]interface{}, credIDs []int) (int, error) {
				launchedWithCreds = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{
						{Name: "gitea-repo-creds"},
					},
				},
			}

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(ContainSubstring("awx-job-"))
			Expect(createdCredTypeName).To(Equal("kubernaut-secret-gitea-repo-creds"))
			Expect(createdCredName).To(ContainSubstring("gitea-repo-creds"))
			Expect(launchedWithCreds).To(ContainElement(42))

			// Verify credential IDs were written to WFE status
			var updatedWFE workflowexecutionv1alpha1.WorkflowExecution
			Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(wfe), &updatedWFE)).To(Succeed())
			Expect(updatedWFE.Status.EphemeralCredentialIDs).To(Equal([]int{42}))
		})

		It("UT-WE-015-031: should reuse existing credential type when already registered", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("pass"),
				},
			}
			fakeClient = newFakeClient(secret)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				return 15, nil // Already exists
			}
			createTypeCalled := false
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				createTypeCalled = true
				return 0, fmt.Errorf("should not be called")
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, credTypeID, _ int, _ map[string]string) (int, error) {
				Expect(credTypeID).To(Equal(15))
				return 42, nil
			}
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-reuse-type", "kubernaut-system", engineConfig, nil)

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())
			Expect(createTypeCalled).To(BeFalse(), "should not create credential type when it already exists")
		})

		It("UT-WE-015-032: should skip credential injection when no dependencies", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			launchCalled := false
			launchWithCredsCalled := false
			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.launchFunc = func(_ context.Context, _ int, _ map[string]interface{}) (int, error) {
				launchCalled = true
				return 42, nil
			}
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
				launchWithCredsCalled = true
				return 42, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-no-deps", "kubernaut-system", engineConfig, nil)

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(launchCalled).To(BeTrue(), "should use standard LaunchJobTemplate when no deps")
			Expect(launchWithCredsCalled).To(BeFalse(), "should NOT use LaunchJobTemplateWithCreds when no deps")
		})

		It("UT-WE-015-033: should return error when K8s Secret not found", func() {
			fakeClient = newFakeClient() // No secret
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-missing-secret", "kubernaut-system", engineConfig, nil)

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "nonexistent-secret"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-secret"))
		})
	})

	Context("Cleanup with ephemeral credentials", func() {
		It("UT-WE-015-034: should delete ephemeral credentials from status before cancelling job", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			var deletedCredIDs []int
			awxClient.deleteCredentialFn = func(_ context.Context, credID int) error {
				deletedCredIDs = append(deletedCredIDs, credID)
				return nil
			}
			cancelCalled := false
			awxClient.cancelFunc = func(_ context.Context, jobID int) error {
				cancelCalled = true
				Expect(jobID).To(Equal(99))
				return nil
			}

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cleanup-with-creds",
					Namespace: "default",
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					EphemeralCredentialIDs: []int{42, 55},
					ExecutionRef:           &corev1.LocalObjectReference{Name: "awx-job-99"},
				},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(deletedCredIDs).To(ConsistOf(42, 55))
			Expect(cancelCalled).To(BeTrue())
		})

		It("UT-WE-015-035: should continue cleanup when credential deletion fails", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.deleteCredentialFn = func(_ context.Context, credID int) error {
				if credID == 42 {
					return fmt.Errorf("AWX delete credential returned 500")
				}
				return nil
			}
			awxClient.cancelFunc = func(_ context.Context, _ int) error { return nil }

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cleanup-partial-fail",
					Namespace: "default",
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					EphemeralCredentialIDs: []int{42, 55},
					ExecutionRef:           &corev1.LocalObjectReference{Name: "awx-job-100"},
				},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred(), "cleanup should succeed even if some credential deletions fail")
		})

		It("UT-WE-015-036: should skip credential cleanup when no credential IDs in status", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			deleteCredCalled := false
			awxClient.deleteCredentialFn = func(_ context.Context, _ int) error {
				deleteCredCalled = true
				return nil
			}
			awxClient.cancelFunc = func(_ context.Context, _ int) error { return nil }

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cleanup-no-annotation",
					Namespace: "default",
				},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					ExecutionRef: &corev1.LocalObjectReference{Name: "awx-job-101"},
				},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(deleteCredCalled).To(BeFalse())
		})
	})
})

// ========================================
// BR-WE-015: AnsibleExecutor dependencies.configMaps injection
// ========================================
// Tests the ConfigMap injection for the ansible engine via AWX extra_vars.
// ConfigMaps are non-sensitive and follow the standard AWX pattern of
// injecting configuration as extra_vars (not credentials).
// ========================================

var _ = Describe("AnsibleExecutor dependencies.configMaps injection (BR-WE-015)", func() {
	var (
		ctx         context.Context
		awxClient   *mockAWXClient
		fakeClient  client.Client
		ansibleExec *executor.AnsibleExecutor
	)

	newFakeClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
			Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockAWXClient{}
	})

	Context("Create with dependencies.configMaps", func() {
		It("UT-WE-015-040: should merge ConfigMap data into extra_vars with KUBERNAUT_CONFIGMAP_ prefix", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-config",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string]string{
					"log-level":  "debug",
					"max-retries": "5",
				},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-configmap-test", "kubernaut-system", engineConfig, map[string]string{
				"TARGET_NAMESPACE": "demo-ns",
			})

			fakeClient = newFakeClient(cm, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			var capturedExtraVars map[string]interface{}
			launchCalled := false
			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.launchFunc = func(_ context.Context, _ int, extraVars map[string]interface{}) (int, error) {
				launchCalled = true
				capturedExtraVars = extraVars
				return 42, nil
			}
			launchWithCredsCalled := false
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
				launchWithCredsCalled = true
				return 42, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					ConfigMaps: []models.ResourceDependency{{Name: "app-config"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())
			Expect(launchCalled).To(BeTrue(), "should use LaunchJobTemplate (no credentials needed for configMaps)")
			Expect(launchWithCredsCalled).To(BeFalse(), "should NOT use LaunchJobTemplateWithCreds for configMap-only deps")

			Expect(capturedExtraVars).To(HaveKeyWithValue("KUBERNAUT_CONFIGMAP_APP_CONFIG_LOG_LEVEL", "debug"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("KUBERNAUT_CONFIGMAP_APP_CONFIG_MAX_RETRIES", "5"))
			Expect(capturedExtraVars).To(HaveKeyWithValue("TARGET_NAMESPACE", "demo-ns"))
			Expect(capturedExtraVars).To(HaveKey("WFE_NAME"))
			Expect(capturedExtraVars).To(HaveKey("WFE_NAMESPACE"))
		})

		It("UT-WE-015-041: should inject both secrets as credentials AND configMaps as extra_vars", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-settings",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string]string{
					"timeout": "30s",
				},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-both-deps", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, cm, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, _ int, _ int, _ map[string]string) (int, error) {
				return 42, nil
			}

			var capturedExtraVars map[string]interface{}
			var capturedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, extraVars map[string]interface{}, credIDs []int) (int, error) {
				capturedExtraVars = extraVars
				capturedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets:    []models.ResourceDependency{{Name: "gitea-repo-creds"}},
					ConfigMaps: []models.ResourceDependency{{Name: "app-settings"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())

			Expect(capturedCredIDs).To(ContainElement(42), "should launch with ephemeral secret credential")
			Expect(capturedExtraVars).To(HaveKeyWithValue("KUBERNAUT_CONFIGMAP_APP_SETTINGS_TIMEOUT", "30s"),
				"configMap data should be merged into extra_vars")
			Expect(capturedExtraVars).To(HaveKey("WFE_NAME"))
		})

		It("UT-WE-015-042: should return error when ConfigMap not found", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-missing-cm", "kubernaut-system", engineConfig, nil)

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					ConfigMaps: []models.ResourceDependency{{Name: "nonexistent-configmap"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-configmap"))
		})
	})
})

// =====================================================================
// #365: AWX Credential Merging — Template creds merged with ephemeral
// =====================================================================
// Authority: BR-WE-015 (Ansible Execution Engine)
// Test Plan: docs/testing/365/TEST_PLAN.md
// Bug: Template credentials are dropped when ephemeral credentials are
//      injected because AWX treats the credentials list as a full
//      replacement, not an append.
// =====================================================================

var _ = Describe("AnsibleExecutor credential merging (#365, BR-WE-015)", func() {
	var (
		ctx         context.Context
		awxClient   *mockAWXClient
		fakeClient  client.Client
		ansibleExec *executor.AnsibleExecutor
	)

	newFakeClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
			Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockAWXClient{}
	})

	Context("Create merges template and ephemeral credentials", func() {
		It("UT-WE-365-001: should include template's pre-configured credentials alongside ephemeral credentials in AWX launch", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/remediate.yml",
				"jobTemplateName": "kubernaut-remediate",
			})
			wfe := newAnsibleWFE("we-merge-creds", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, _ int, _ int, _ map[string]string) (int, error) {
				return 42, nil
			}
			awxClient.getTemplateCredsFn = func(_ context.Context, templateID int) ([]int, error) {
				Expect(templateID).To(Equal(10))
				return []int{100, 200}, nil
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(Equal("awx-job-77"))

			By("Verifying template credentials 100, 200 are present in the launch")
			Expect(launchedCredIDs).To(ContainElement(100),
				"template credential 100 must be preserved in launch")
			Expect(launchedCredIDs).To(ContainElement(200),
				"template credential 200 must be preserved in launch")

			By("Verifying ephemeral credential 42 is also present")
			Expect(launchedCredIDs).To(ContainElement(42),
				"ephemeral credential 42 must be included in launch")

			By("Verifying total count is exactly 3 (no duplicates)")
			Expect(launchedCredIDs).To(HaveLen(3),
				"merged list should contain exactly template (2) + ephemeral (1) = 3 credentials")
		})

		It("UT-WE-365-002: should deduplicate when template and ephemeral credentials overlap", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{"token": []byte("abc")},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/remediate.yml",
				"jobTemplateName": "kubernaut-remediate",
			})
			wfe := newAnsibleWFE("we-dedup-creds", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, _ int, _ int, _ map[string]string) (int, error) {
				return 200, nil // Ephemeral gets ID 200, which overlaps with template
			}
			awxClient.getTemplateCredsFn = func(_ context.Context, _ int) ([]int, error) {
				return []int{100, 200}, nil
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying 200 appears only once (deduplicated)")
			count200 := 0
			for _, id := range launchedCredIDs {
				if id == 200 {
					count200++
				}
			}
			Expect(count200).To(Equal(1), "credential 200 must appear exactly once (deduplicated)")
			Expect(launchedCredIDs).To(ContainElement(100))
			Expect(launchedCredIDs).To(HaveLen(2), "merged list should be [100, 200] with no duplicates")
		})

		It("UT-WE-365-003: should preserve ephemeral-only behavior when template has no pre-configured credentials", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{"token": []byte("abc")},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/remediate.yml",
				"jobTemplateName": "kubernaut-remediate",
			})
			wfe := newAnsibleWFE("we-no-template-creds", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, _ int, _ int, _ map[string]string) (int, error) {
				return 42, nil
			}
			awxClient.getTemplateCredsFn = func(_ context.Context, _ int) ([]int, error) {
				return nil, nil // No template creds
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying only ephemeral credential 42 is sent")
			Expect(launchedCredIDs).To(Equal([]int{42}),
				"when template has no creds, only ephemeral cred should be sent")
		})

		It("UT-WE-365-005: should proceed with ephemeral-only when GetJobTemplateCredentials fails (non-fatal)", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{"token": []byte("abc")},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/remediate.yml",
				"jobTemplateName": "kubernaut-remediate",
			})
			wfe := newAnsibleWFE("we-cred-fetch-fail", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, _ string) (int, error) {
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, _ int, _ int, _ map[string]string) (int, error) {
				return 42, nil
			}
			awxClient.getTemplateCredsFn = func(_ context.Context, _ int) ([]int, error) {
				return nil, fmt.Errorf("AWX credentials returned 500: internal server error")
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)

			By("Verifying launch succeeded despite credential fetch failure")
			Expect(err).ToNot(HaveOccurred(),
				"Create must not fail when GetJobTemplateCredentials errors — degraded launch is acceptable")
			Expect(ref).To(Equal("awx-job-77"))

			By("Verifying only ephemeral credential was sent (template creds could not be fetched)")
			Expect(launchedCredIDs).To(Equal([]int{42}),
				"when template cred fetch fails, launch with ephemeral only")
		})
	})
})

// =====================================================================
// #500: K8s credential injection for kubernetes.core playbooks
// =====================================================================
// Authority: BR-WE-015, #497, #500
// The WE Ansible executor injects the controller's in-cluster SA token
// as an AWX credential so kubernetes.core modules can authenticate.
// Ref: ansible/awx#5735, ansible/awx#7629
// =====================================================================

var _ = Describe("AnsibleExecutor K8s credential injection (#500, BR-WE-015)", func() {
	var (
		ctx         context.Context
		awxClient   *mockAWXClient
		fakeClient  client.Client
		ansibleExec *executor.AnsibleExecutor
	)

	newFakeClient := func(objs ...client.Object) client.Client {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(objs...).
			WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
			Build()
	}

	mockInClusterCreds := func() (*executor.InClusterCredentials, error) {
		return &executor.InClusterCredentials{
			Host:   "https://10.0.0.1:6443",
			Token:  "fake-sa-token-for-testing",
			CACert: "-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----",
		}, nil
	}

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockAWXClient{}
	})

	Context("Create injects K8s credential alongside dependency secrets", func() {
		It("UT-WE-500-001: should inject K8s credential when no dependency secrets exist", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))
			ansibleExec.InClusterCredentialsFn = mockInClusterCreds

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				if name == "OpenShift or Kubernetes API Bearer Token" {
					return 5, nil
				}
				return 0, fmt.Errorf("not found")
			}

			var capturedCredName string
			var capturedInputs map[string]string
			awxClient.createCredentialFn = func(_ context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error) {
				capturedCredName = name
				capturedInputs = inputs
				Expect(credTypeID).To(Equal(5))
				return 88, nil
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/scale-deployment.yml",
				"jobTemplateName": "kubernaut-scale-deployment",
			})
			wfe := newAnsibleWFE("we-k8s-cred-test", "kubernaut-system", engineConfig, nil)

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(ContainSubstring("awx-job-"))

			By("Verifying K8s credential was created with controller SA token")
			Expect(capturedCredName).To(Equal("kubernaut-k8s-we-k8s-cred-test"))
			Expect(capturedInputs).To(HaveKeyWithValue("host", "https://10.0.0.1:6443"))
			Expect(capturedInputs).To(HaveKeyWithValue("bearer_token", "fake-sa-token-for-testing"))
			Expect(capturedInputs).To(HaveKey("ssl_ca_cert"))

			By("Verifying K8s credential ID is included in launch")
			Expect(launchedCredIDs).To(ContainElement(88))
		})

		It("UT-WE-500-002: should inject K8s credential alongside dependency secret credentials", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gitea-repo-creds",
					Namespace: "kubernaut-workflows",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/gitops-update.yml",
				"jobTemplateName": "kubernaut-gitops-update",
			})
			wfe := newAnsibleWFE("we-k8s-plus-secrets", "kubernaut-system", engineConfig, nil)

			fakeClient = newFakeClient(secret, wfe)
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))
			ansibleExec.InClusterCredentialsFn = mockInClusterCreds

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }

			callCount := 0
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				if name == "OpenShift or Kubernetes API Bearer Token" {
					return 5, nil
				}
				return 0, fmt.Errorf("not found")
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				return 15, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, name string, credTypeID, _ int, _ map[string]string) (int, error) {
				callCount++
				if credTypeID == 5 {
					return 88, nil // K8s credential
				}
				return 42, nil // dependency secret credential
			}

			var launchedCredIDs []int
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, credIDs []int) (int, error) {
				launchedCredIDs = credIDs
				return 77, nil
			}

			opts := executor.CreateOptions{
				Dependencies: &models.WorkflowDependencies{
					Secrets: []models.ResourceDependency{{Name: "gitea-repo-creds"}},
				},
			}

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", opts)
			Expect(err).ToNot(HaveOccurred())

			By("Verifying both dependency secret cred and K8s cred are in launch")
			Expect(launchedCredIDs).To(ContainElement(42), "dependency secret credential")
			Expect(launchedCredIDs).To(ContainElement(88), "K8s credential")
			Expect(callCount).To(Equal(2), "should create 2 credentials: 1 dependency + 1 K8s")
		})

		It("UT-WE-500-003: should proceed without K8s credential when in-cluster creds unavailable", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))
			ansibleExec.InClusterCredentialsFn = func() (*executor.InClusterCredentials, error) {
				return nil, fmt.Errorf("in-cluster environment not detected: KUBERNETES_SERVICE_HOST or KUBERNETES_SERVICE_PORT not set")
			}

			launchCalled := false
			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.launchFunc = func(_ context.Context, _ int, _ map[string]interface{}) (int, error) {
				launchCalled = true
				return 42, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-no-incluster", "kubernaut-system", engineConfig, nil)

			ref, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			By("Verifying job still launches (degraded, without K8s cred)")
			Expect(err).ToNot(HaveOccurred())
			Expect(ref).To(ContainSubstring("awx-job-"))
			Expect(launchCalled).To(BeTrue(), "should fall back to standard launch without credentials")
		})

		It("UT-WE-500-004: should use custom fallback type when built-in type not found", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))
			ansibleExec.InClusterCredentialsFn = mockInClusterCreds

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }

			var createdTypeName string
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				return 0, fmt.Errorf("not found: %s", name)
			}
			awxClient.createCredentialTypeFn = func(_ context.Context, name string, inputs, injectors map[string]interface{}) (int, error) {
				createdTypeName = name

				By("Verifying custom type has correct injector structure")
				Expect(injectors).To(HaveKey("env"))
				envMap, ok := injectors["env"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(envMap).To(HaveKeyWithValue("K8S_AUTH_HOST", "{{host}}"))
				Expect(envMap).To(HaveKeyWithValue("K8S_AUTH_API_KEY", "{{bearer_token}}"))
				Expect(envMap).To(HaveKey("K8S_AUTH_SSL_CA_CERT"))
				Expect(envMap).To(HaveKeyWithValue("K8S_AUTH_VERIFY_SSL", "True"))

				return 99, nil
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, credTypeID, _ int, _ map[string]string) (int, error) {
				Expect(credTypeID).To(Equal(99))
				return 88, nil
			}
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-fallback-type", "kubernaut-system", engineConfig, nil)

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(createdTypeName).To(Equal("kubernaut-k8s-bearer-token"),
				"should create custom fallback type when built-in is absent")
		})

		It("UT-WE-500-005: should reuse existing custom fallback type", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))
			ansibleExec.InClusterCredentialsFn = mockInClusterCreds

			awxClient.findTemplateByNameFn = func(_ context.Context, _ string) (int, error) { return 10, nil }
			awxClient.findCredentialTypeByNameFn = func(_ context.Context, name string) (int, error) {
				if name == "OpenShift or Kubernetes API Bearer Token" {
					return 0, fmt.Errorf("not found")
				}
				if name == "kubernaut-k8s-bearer-token" {
					return 99, nil // Already exists
				}
				return 0, fmt.Errorf("not found")
			}

			createTypeCalled := false
			awxClient.createCredentialTypeFn = func(_ context.Context, _ string, _, _ map[string]interface{}) (int, error) {
				createTypeCalled = true
				return 0, fmt.Errorf("should not be called")
			}
			awxClient.createCredentialFn = func(_ context.Context, _ string, credTypeID, _ int, _ map[string]string) (int, error) {
				Expect(credTypeID).To(Equal(99))
				return 88, nil
			}
			awxClient.launchWithCredsFn = func(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
				return 77, nil
			}

			engineConfig, _ := json.Marshal(map[string]interface{}{
				"playbookPath":    "playbooks/test.yml",
				"jobTemplateName": "test-template",
			})
			wfe := newAnsibleWFE("we-reuse-fallback", "kubernaut-system", engineConfig, nil)

			_, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(createTypeCalled).To(BeFalse(), "should not create type when fallback already exists")
		})

		It("UT-WE-500-006: K8s credential should be cleaned up with other ephemeral credentials", func() {
			fakeClient = newFakeClient()
			ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeClient, 1, ctrl.Log.WithName("test"))

			var deletedCredIDs []int
			awxClient.deleteCredentialFn = func(_ context.Context, credID int) error {
				deletedCredIDs = append(deletedCredIDs, credID)
				return nil
			}
			awxClient.cancelFunc = func(_ context.Context, _ int) error { return nil }

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-k8s-cred", Namespace: "default"},
				Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
					EphemeralCredentialIDs: []int{42, 88}, // 42=dependency, 88=K8s
					ExecutionRef:           &corev1.LocalObjectReference{Name: "awx-job-99"},
				},
			}

			err := ansibleExec.Cleanup(ctx, wfe, "default")
			Expect(err).ToNot(HaveOccurred())
			Expect(deletedCredIDs).To(ConsistOf(42, 88),
				"both dependency and K8s ephemeral credentials should be deleted")
		})
	})
})
