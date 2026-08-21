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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
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
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(as).Build()
		c := creator.NewAgentSessionCreator(fakeClient, scheme)

		Expect(c.DeleteForCascadeCancel(ctx, "rr-cascade-004", "default")).To(Succeed())

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

		Expect(c.DeleteForCascadeCancel(ctx, "rr-never-existed", "default")).To(Succeed())
	})
})
