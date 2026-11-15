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

package creator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
)

// BR-INTERACTIVE-010 SC-9 (#2214, DD-AA-KA-001 Amendment): AA's cascade-cancel
// path deletes the AgentSession instead of writing InvestigationSession
// directly, letting AF's AgentSession-watching reconciler close IS and KA's
// already-proven Dispatcher.cancelOnDelete stop the in-flight investigation.
var _ = Describe("AgentSessionCreator.DeleteForCascadeCancel [IR-4, AC-6]", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		_ = clientgoscheme.AddToScheme(scheme)
		_ = agentsessionv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
	})

	// UT-AA-2214-004: happy path -- deletes the deterministically-named
	// AgentSession (as-<rrName>) for the given RR/namespace.
	It("UT-AA-2214-004: deletes the AgentSession named as-<rrName> in the given namespace [IR-4(1)]", func() {
		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "as-rr-cascade-004",
				Namespace: "default",
			},
		}
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-rr-cascade-004", Namespace: "default", UID: types.UID("analysis-uid")},
			Spec:       aianalysisv1.AIAnalysisSpec{RemediationRequestRef: corev1.ObjectReference{Name: "rr-cascade-004", Namespace: "default"}},
		}
		Expect(controllerutil.SetControllerReference(analysis, as, scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(as).Build()
		c := creator.NewAgentSessionCreator(fakeClient, scheme)

		Expect(c.DeleteForCascadeCancel(ctx, analysis)).To(Succeed())

		var fetched agentsessionv1.AgentSession
		err := fakeClient.Get(ctx, types.NamespacedName{Name: "as-rr-cascade-004", Namespace: "default"}, &fetched)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	// UT-AA-2214-005: idempotency -- a second call (or a call after the
	// AgentSession was already deleted/never existed) is not an error,
	// mirroring DeleteForRetry's existing idempotent-on-NotFound contract.
	It("UT-AA-2214-005: is idempotent -- NotFound is not an error [AC-6]", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		c := creator.NewAgentSessionCreator(fakeClient, scheme)
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "aa-rr-never-existed", Namespace: "default"},
			Spec:       aianalysisv1.AIAnalysisSpec{RemediationRequestRef: corev1.ObjectReference{Name: "rr-never-existed", Namespace: "default"}},
		}

		Expect(c.DeleteForCascadeCancel(ctx, analysis)).To(Succeed())
	})

	It("UT-AA-2588-003: refuses to delete an AgentSession owned by a stale AIAnalysis UID", func() {
		controller := true
		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "as-rr-cascade-stale",
				Namespace: "default",
				UID:       types.UID("session-uid"),
				OwnerReferences: []metav1.OwnerReference{{
					Kind:       "AIAnalysis",
					Name:       "aa-rr-cascade-stale",
					UID:        types.UID("old-analysis-uid"),
					Controller: &controller,
				}},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(as).Build()
		c := creator.NewAgentSessionCreator(fakeClient, scheme)
		analysis := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-rr-cascade-stale",
				Namespace: "default",
				UID:       types.UID("current-analysis-uid"),
			},
			Spec: aianalysisv1.AIAnalysisSpec{RemediationRequestRef: corev1.ObjectReference{Name: "rr-cascade-stale", Namespace: "default"}},
		}

		Expect(c.DeleteForCascadeCancel(ctx, analysis)).To(MatchError(ContainSubstring("identity mismatch")))
		var fetched agentsessionv1.AgentSession
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: as.Name, Namespace: as.Namespace}, &fetched)).To(Succeed())
	})
})
