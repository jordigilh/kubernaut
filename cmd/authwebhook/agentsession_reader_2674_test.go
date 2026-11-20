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

package main

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agentsessionv1alpha1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Issue #2674, BR-AA-KA-065.13 (SI-10 input validation at the trust boundary;
// FedRAMP AU-3 denial attribution): the AgentSession CREATE existence gate must
// resolve Spec.RemediationRequestRef through the manager's direct API reader,
// never through the manager's cache-backed client. Two cache windows make the
// cached read produce a wrong answer at exactly the moment the gate runs:
//
//	(a) the cache is not started yet -- controller-runtime starts webhook
//	    listeners before caches (pkg/manager/internal.go:429-446), and /readyz
//	    does not gate on cache sync, so an admission request can arrive while
//	    InformerCache.Get still returns &cache.ErrCacheNotStarted{};
//	(b) the cache is started but lagging -- a RemediationRequest created
//	    moments earlier reads as NotFound.
//
// Both make the gate deny a legitimate AgentSession, and in (a) it records the
// internal error string as the denial reason in the audit trail, which misleads
// operators reconstructing the denial (AU-3).
//
// These specs drive the production wiring seam (agentSessionGateReader) that
// registerAuthWebhookWorkflowHandlers uses, so reverting the wiring to
// mgr.GetClient() fails them.

// fakeGateManager is the minimal ctrl.Manager surface the AgentSession gate
// wiring needs. GetClient is deliberately part of the fake even though the
// fixed wiring never calls it: it is what a revert to the cache-backed client
// would silently pick up, and it is what makes that revert fail these specs.
type fakeGateManager struct {
	apiReader client.Reader
	cached    client.Client
}

func (f fakeGateManager) GetAPIReader() client.Reader { return f.apiReader }
func (f fakeGateManager) GetClient() client.Client    { return f.cached }

// cachedClientFailingWith models the manager cache in one of its two wrong
// states: Get returns the supplied cache-layer error instead of the object.
func cachedClientFailingWith(err error) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return err
		},
	}).Build()
}

// directReaderWith models the manager's direct API reader: an uncached read
// that returns whatever the API server currently has.
func directReaderWith(rr *remediationv1.RemediationRequest) client.Reader {
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
}

func gateAgentSession(namespace, rrName string) (*agentsessionv1alpha1.AgentSession, admission.Request) {
	as := &agentsessionv1alpha1.AgentSession{
		TypeMeta: metav1.TypeMeta{APIVersion: "kubernaut.ai/v1alpha1", Kind: "AgentSession"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "as-2674-gate",
			Namespace: namespace,
		},
		Spec: agentsessionv1alpha1.AgentSessionSpec{
			RemediationRequestRef: agentsessionv1alpha1.ObjectRef{Name: rrName, Namespace: namespace},
			IncidentID:            "incident-2674",
			RemediationID:         rrName,
			SignalName:            "OOMKilled",
			Severity:              "critical",
			Environment:           "test",
			Priority:              "P1",
			SignalMode:            "reactive",
		},
	}
	raw, err := json.Marshal(as)
	Expect(err).ToNot(HaveOccurred())

	return as, admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       types.UID("uid-2674-gate"),
			Kind:      metav1.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "AgentSession"},
			Name:      as.Name,
			Namespace: as.Namespace,
			Operation: admissionv1.Create,
			UserInfo: authv1.UserInfo{
				Username: "it-agentsession-user@kubernaut.ai",
				UID:      "it-agentsession-uid",
				Groups:   []string{"system:masters"},
			},
			Object: runtime.RawExtension{Raw: raw},
		},
	}
}

func gateRR(namespace, name string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		TypeMeta:   metav1.TypeMeta{APIVersion: "kubernaut.ai/v1alpha1", Kind: "RemediationRequest"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

var _ = Describe("#2674 AgentSession existence gate reads RemediationRequest through the direct API reader (BR-AA-KA-065.13, SI-10, AU-3)", func() {
	const namespace = "default"
	ctx := context.Background()

	It("IT-AS-2674-001: CREATE is admitted while the manager cache is not started, because the gate reads the API directly", func() {
		rr := gateRR(namespace, "rr-created-before-cache-start")
		mgr := fakeGateManager{
			apiReader: directReaderWith(rr),
			cached:    cachedClientFailingWith(&cache.ErrCacheNotStarted{}),
		}

		as, req := gateAgentSession(namespace, rr.Name)
		handler := authwebhook.NewAgentSessionHandler(nil, agentSessionGateReader(mgr))
		resp := handler.Handle(ctx, req)

		Expect(resp.Allowed).To(BeTrue(),
			"the gate must resolve RemediationRequestRef %q from the API server even though the manager cache has not started yet; "+
				"a cached read would deny this legitimate AgentSession with the internal cache error. Denial message: %v",
			as.Spec.RemediationRequestRef.Name, resp.Result)
		Expect(resp.Result.Message).ToNot(ContainSubstring("cache is not started"),
			"the admission response must never expose the cache-layer error for a RemediationRequest %q that exists in etcd",
			as.Spec.RemediationRequestRef.Name)
	})

	It("IT-AS-2674-002: CREATE is admitted when the manager cache lags a just-created RemediationRequest (no false 'does not exist')", func() {
		rr := gateRR(namespace, "rr-created-while-cache-lagging")
		mgr := fakeGateManager{
			apiReader: directReaderWith(rr),
			cached:    cachedClientFailingWith(apierrors.NewNotFound(remediationv1.GroupVersion.WithResource("remediationrequests").GroupResource(), rr.Name)),
		}

		as, req := gateAgentSession(namespace, rr.Name)
		handler := authwebhook.NewAgentSessionHandler(nil, agentSessionGateReader(mgr))
		resp := handler.Handle(ctx, req)

		Expect(resp.Allowed).To(BeTrue(),
			"a RemediationRequest that exists in etcd must admit the AgentSession even when the informer cache has not observed it yet; "+
				"a lagging cached read reports the same 'does not exist' denial as a genuinely dangling reference. Denial message: %v",
			resp.Result)
		Expect(resp.Result.Message).ToNot(ContainSubstring("does not exist"),
			"the gate must not attribute a cache staleness artifact to a missing RemediationRequest %q in the admission response",
			as.Spec.RemediationRequestRef.Name)
	})
})
