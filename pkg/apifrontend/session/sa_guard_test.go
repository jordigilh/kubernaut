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

package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// BR-INTERACTIVE-010 SC-5: SA guard and single-driver field selector tests.
var _ = Describe("SA Guard and Single-Driver [BR-INTERACTIVE-010 SC-5]", func() {
	var (
		ctx       context.Context
		decorator *session.ServiceDecorator
	)

	BeforeEach(func() {
		inner := adksession.InMemoryService()
		decorator = session.NewServiceDecorator(inner)
		ctx = context.Background()
	})

	Describe("ServiceDecorator.Create SA Guard", func() {
		It("UT-AF-1293-003: SA caller is rejected from creating IS", func() {
			identity := &auth.UserIdentity{
				Username:         "system:serviceaccount:ns:robot",
				Groups:           []string{"system:serviceaccounts"},
				IsServiceAccount: true,
			}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithCreateContext(ctx, &session.CreateContext{
				TaskID: "task-sa-attempt",
			})

			_, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "robot",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service accounts"))
		})

		It("UT-AF-1293-004: human caller is allowed to create IS", func() {
			identity := &auth.UserIdentity{
				Username:         "alice",
				Groups:           []string{"sre"},
				IsServiceAccount: false,
			}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithCreateContext(ctx, &session.CreateContext{
				TaskID: "task-human-ok",
			})

			resp, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "alice",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Session).NotTo(BeNil())
		})
	})

	Describe("MaterializeCRD Single-Driver Guard", func() {
		It("UT-AF-1293-005: field selector detects conflicting active session from different user", func() {
			scheme := newScheme()
			existingIS := &v1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "is-existing-001",
					Namespace: "test-ns",
				},
				Spec: v1alpha1.InvestigationSessionSpec{
					RemediationRequestRef: v1alpha1.ObjectRef{
						Name:      "rr-conflict",
						Namespace: "test-ns",
					},
					UserIdentity: v1alpha1.SessionUser{
						Username: "bob",
						Groups:   []string{"sre"},
					},
				},
				Status: v1alpha1.InvestigationSessionStatus{
					Phase: v1alpha1.SessionPhaseActive,
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingIS).
				WithStatusSubresource(&v1alpha1.InvestigationSession{}).
				WithIndex(&v1alpha1.InvestigationSession{}, session.FieldIndexRRName,
					func(obj client.Object) []string {
						is := obj.(*v1alpha1.InvestigationSession)
						if is.Spec.RemediationRequestRef.Name == "" {
							return nil
						}
						return []string{is.Spec.RemediationRequestRef.Name}
					}).
				Build()

			crdSvc := session.NewCRDSessionService(adksession.InMemoryService(), k8sClient, scheme, "test-ns")
			dec := session.NewServiceDecorator(crdSvc)

			identity := &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre"},
			}
			testCtx := auth.WithUserIdentity(ctx, identity)
			testCtx = session.WithCreateContext(testCtx, &session.CreateContext{
				TaskID: "task-conflict",
				RemediationRef: v1alpha1.ObjectRef{
					Name:      "rr-conflict",
					Namespace: "test-ns",
				},
			})

			resp, err := dec.Create(testCtx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "alice",
			})

			Expect(err).NotTo(HaveOccurred(), "decorator Create should succeed (deferred)")
			Expect(resp).NotTo(BeNil())

			err = crdSvc.MaterializeCRD(testCtx, resp.Session.ID(), v1alpha1.ObjectRef{
				Name:      "rr-conflict",
				Namespace: "test-ns",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session_active"))
		})
	})
})
