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

package validation

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
)

// baseSpec returns a minimally-valid AgentSessionSpec, so each test below
// can zero out exactly the one field it's proving is rejected -- isolating
// the assertion to that field's own constraint.
func baseSpec(name string) agentsessionv1.AgentSessionSpec {
	return agentsessionv1.AgentSessionSpec{
		RemediationRequestRef: agentsessionv1.ObjectRef{Name: "rr-" + name, Namespace: "default"},
		IncidentID:            "incident-" + name,
		RemediationID:         "rem-" + name,
		SignalName:            "TestSignal",
		Severity:              "high",
	}
}

var _ = Describe("IT-AS-2190: AgentSession CRD schema rejects incomplete investigation-identity fields", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		ns        *AgentSessionTestNamespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		ns = newTestNamespace(ctx, k8sClient)
	})

	AfterEach(func() {
		ns.Delete(ctx, k8sClient)
	})

	It("IT-AS-2190-001: rejects an AgentSession with an empty IncidentID", func() {
		spec := baseSpec("empty-incident-id")
		spec.IncidentID = ""

		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{Name: "as-empty-incident-id", Namespace: ns.Name},
			Spec:       spec,
		}

		err := k8sClient.Create(ctx, as)
		Expect(err).To(HaveOccurred(), "real apiserver must reject an empty incidentID (SI-10: pkg/agentclient's retired HTTP schema previously enforced this)")
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection must be a schema-validation error, not e.g. a network/RBAC error")
	})

	It("IT-AS-2190-002: rejects an AgentSession with an empty RemediationID", func() {
		spec := baseSpec("empty-remediation-id")
		spec.RemediationID = ""

		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{Name: "as-empty-remediation-id", Namespace: ns.Name},
			Spec:       spec,
		}

		err := k8sClient.Create(ctx, as)
		Expect(err).To(HaveOccurred(), "real apiserver must reject an empty remediationID -- DD-WORKFLOW-002 v2.2 MANDATORY minLength:1, previously enforced by the retired agentclient.IncidentRequest OpenAPI schema")
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "rejection must be a schema-validation error, not e.g. a network/RBAC error")
	})

	It("IT-AS-2190-003: accepts an AgentSession with both fields populated (control case)", func() {
		spec := baseSpec("valid")
		as := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{Name: "as-valid", Namespace: ns.Name},
			Spec:       spec,
		}

		Expect(k8sClient.Create(ctx, as)).To(Succeed(), "a fully-populated AgentSession must not be rejected by the new constraint (no false positive)")
	})
})

// AgentSessionTestNamespace is a tiny per-test namespace helper, avoiding a
// dependency on any shared test/integration/aianalysis fixture package for
// this deliberately isolated suite.
type AgentSessionTestNamespace struct {
	Name string
}

func newTestNamespace(ctx context.Context, c client.Client) *AgentSessionTestNamespace {
	name := fmt.Sprintf("as-validation-it-%d", time.Now().UnixNano())
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	Expect(c.Create(ctx, ns)).To(Succeed())
	return &AgentSessionTestNamespace{Name: name}
}

func (n *AgentSessionTestNamespace) Delete(ctx context.Context, c client.Client) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: n.Name}}
	_ = c.Delete(ctx, ns)
}
