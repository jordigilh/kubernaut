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

package workflowexecution

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// ========================================
// ANSIBLE TOKENREQUEST + TTL VALIDATION (#501)
// ========================================
// Authority: BR-WE-015, DD-WE-005 v2.0, Issue #501
// Tests per-workflow SA token injection, fallback, and TTL validation.
// ========================================

var _ = Describe("Ansible TokenRequest Credential Injection [#501]", func() {

	var (
		ctx         context.Context
		awxClient   *mockTokenRequestAWXClient
		fakeCS      *kubefake.Clientset
		ansibleExec *executor.AnsibleExecutor
	)

	defaultEngineConfig := func() *apiextensionsv1.JSON {
		return &apiextensionsv1.JSON{Raw: []byte(`{"jobTemplateName":"test-template","playbookPath":"playbooks/restart.yml"}`)}
	}

	buildWFE := func(saName string, timeout time.Duration) *workflowexecutionv1alpha1.WorkflowExecution {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wfe-token-test",
				Namespace: "kubernaut-workflows",
			},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				ServiceAccountName: saName,
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowID:      "wf-123",
					ExecutionBundle: "quay.io/test:v1@sha256:abc123",
					EngineConfig:    defaultEngineConfig(),
				},
				TargetResource: "default/Deployment/nginx",
				RemediationRequestRef: corev1.ObjectReference{
					Name:      "rr-test",
					Namespace: "kubernaut-workflows",
				},
			},
			Status: workflowexecutionv1alpha1.WorkflowExecutionStatus{
				ExecutionEngine: "ansible",
			},
		}
		if timeout > 0 {
			wfe.Spec.ExecutionConfig = &workflowexecutionv1alpha1.ExecutionConfig{
				Timeout: &metav1.Duration{Duration: timeout},
			}
		}
		return wfe
	}

	BeforeEach(func() {
		ctx = context.Background()
		awxClient = &mockTokenRequestAWXClient{
			templateID:   99,
			credID:       55,
			credTypeID:   10,
			launchedJob:  42,
			templateCreds: []int{},
		}
		fakeCS = kubefake.NewSimpleClientset()
	})

	setupExecutor := func(cs *kubefake.Clientset, wfe *workflowexecutionv1alpha1.WorkflowExecution) {
		scheme := runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		fakeK8sClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
			WithObjects(wfe).
			Build()
		ansibleExec = executor.NewAnsibleExecutor(awxClient, fakeK8sClient, cs, 1, ctrl.Log.WithName("test-token"))
		ansibleExec.InClusterCredentialsFn = func() (*executor.InClusterCredentials, error) {
			return &executor.InClusterCredentials{
				Host:   "https://10.0.0.1:6443",
				Token:  "controller-token",
				CACert: "fake-ca-cert",
			}, nil
		}
	}

	// Helper: add a TokenRequest reactor that returns a token with given expiration
	addTokenReactor := func(cs *kubefake.Clientset, token string, expiration time.Time) {
		cs.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
			subresource := action.GetSubresource()
			if subresource != "token" {
				return false, nil, nil
			}
			return true, &authenticationv1.TokenRequest{
				Status: authenticationv1.TokenRequestStatus{
					Token:               token,
					ExpirationTimestamp: metav1.NewTime(expiration),
				},
			}, nil
		})
	}

	// Helper: add a TokenRequest reactor that returns an error
	addTokenErrorReactor := func(cs *kubefake.Clientset, err error) {
		cs.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.GetSubresource() != "token" {
				return false, nil, nil
			}
			return true, nil, err
		})
	}

	Context("TokenRequest injection (SA specified)", func() {

		It("UT-WE-501-004: AWX receives per-workflow SA credentials when workflow SA is specified", func() {
			addTokenReactor(fakeCS, "sa-token-xyz", time.Now().Add(2*time.Hour))
			wfe := buildWFE("workflow-sa", 30*time.Minute)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ResourceName).To(Equal("awx-job-42"))

			By("Verifying AWX received the TokenRequest token, not the controller token")
			Expect(awxClient.capturedCredInputs).To(HaveKeyWithValue("bearer_token", "sa-token-xyz"),
				"AWX must receive the per-workflow SA token from TokenRequest")
		})

		It("UT-WE-501-006: Create fails when TokenRequest returns an error", func() {
			notFoundErr := apierrors.NewNotFound(
				schema.GroupResource{Group: "", Resource: "serviceaccounts"},
				"nonexistent-sa",
			)
			addTokenErrorReactor(fakeCS, notFoundErr)
			wfe := buildWFE("nonexistent-sa", 0)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(result).To(BeNil(), "result should be nil on TokenRequest failure")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("TokenRequest"))
			Expect(err.Error()).To(ContainSubstring("nonexistent-sa"))

			By("Verifying no AWX job was launched")
			Expect(awxClient.launchCalled).To(BeFalse(),
				"AWX job must not be launched when TokenRequest fails")
		})
	})

	Context("Fallback to in-cluster credentials (no SA)", func() {

		It("UT-WE-501-005: AWX receives controller in-cluster credentials when no workflow SA", func() {
			wfe := buildWFE("", 0)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ResourceName).To(Equal("awx-job-42"))

			By("Verifying AWX received the in-cluster token")
			Expect(awxClient.capturedCredInputs).To(HaveKeyWithValue("bearer_token", "controller-token"),
				"AWX must receive the controller's in-cluster token as fallback")
		})

		It("UT-WE-501-003: Ansible Create returns CreateResult with ResourceName", func() {
			wfe := buildWFE("", 0)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.ResourceName).To(Equal("awx-job-42"),
				"ResourceName should match the AWX job ID format")
			Expect(result.Warnings).To(HaveLen(0),
				"no warnings expected when using in-cluster fallback")
		})
	})

	Context("TTL validation", func() {

		It("UT-WE-501-007: No warning when granted TTL meets execution timeout", func() {
			addTokenReactor(fakeCS, "sufficient-token", time.Now().Add(2*time.Hour))
			wfe := buildWFE("my-sa", 30*time.Minute)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Warnings).To(HaveLen(0),
				"no warning expected when granted TTL (2h) >= execution timeout (30m)")
		})

		It("UT-WE-501-008: Warning when API server shortens token TTL below timeout", func() {
			addTokenReactor(fakeCS, "short-token", time.Now().Add(10*time.Minute))
			wfe := buildWFE("my-sa", 60*time.Minute)
			setupExecutor(fakeCS, wfe)
			result, err := ansibleExec.Create(ctx, wfe, "kubernaut-workflows", executor.CreateOptions{})

			Expect(err).ToNot(HaveOccurred(),
				"Create should succeed even with TTL warning (soft warning, not hard failure)")
			Expect(result.ResourceName).To(Equal("awx-job-42"),
				"AWX job should still be launched")

			Expect(result.Warnings).To(HaveLen(1), "exactly one TTL warning expected")
			w := result.Warnings[0]
			Expect(w.Type).To(Equal(workflowexecutionv1alpha1.ConditionTokenTTLInsufficient))
			Expect(w.Reason).To(Equal(workflowexecutionv1alpha1.ReasonTokenTTLShortened))
			Expect(w.Message).To(ContainSubstring("shorter than execution timeout"),
				"warning message should describe the TTL mismatch")
		})
	})
})

// mockTokenRequestAWXClient is a minimal AWX client mock for TokenRequest tests.
type mockTokenRequestAWXClient struct {
	templateID    int
	credID        int
	credTypeID    int
	launchedJob   int
	templateCreds []int
	launchCalled  bool

	capturedCredInputs map[string]string
}

func (m *mockTokenRequestAWXClient) LaunchJobTemplate(_ context.Context, _ int, _ map[string]interface{}) (int, error) {
	m.launchCalled = true
	return m.launchedJob, nil
}

func (m *mockTokenRequestAWXClient) LaunchJobTemplateWithCreds(_ context.Context, _ int, _ map[string]interface{}, _ []int) (int, error) {
	m.launchCalled = true
	return m.launchedJob, nil
}

func (m *mockTokenRequestAWXClient) GetJobStatus(_ context.Context, _ int) (*executor.AWXJobStatus, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTokenRequestAWXClient) CancelJob(_ context.Context, _ int) error {
	return nil
}

func (m *mockTokenRequestAWXClient) FindJobTemplateByName(_ context.Context, _ string) (int, error) {
	return m.templateID, nil
}

func (m *mockTokenRequestAWXClient) CreateCredentialType(_ context.Context, _ string, _ executor.CredentialTypeInputs, _ executor.CredentialTypeInjectors) (int, error) {
	return m.credTypeID, nil
}

func (m *mockTokenRequestAWXClient) FindCredentialTypeByName(_ context.Context, _ string) (int, error) {
	return m.credTypeID, nil
}

func (m *mockTokenRequestAWXClient) FindCredentialTypeByKind(_ context.Context, _ string, _ bool) (int, error) {
	return 0, fmt.Errorf("not found")
}

func (m *mockTokenRequestAWXClient) CreateCredential(_ context.Context, _ string, _ int, _ int, inputs map[string]string) (int, error) {
	m.capturedCredInputs = inputs
	return m.credID, nil
}

func (m *mockTokenRequestAWXClient) DeleteCredential(_ context.Context, _ int) error {
	return nil
}

func (m *mockTokenRequestAWXClient) GetJobTemplateCredentials(_ context.Context, _ int) ([]int, error) {
	return m.templateCreds, nil
}
