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

package k8s_test

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// ========================================
// GAP-13 (Issue #1505): Secret access observer on the K8s resource resolver
// ========================================
//
// KubernautAgent intentionally keeps broad read RBAC on Secrets for
// investigation completeness (missing RBAC degrades RCA quality — see
// docs/services/stateless/kubernaut-agent/security-configuration.md).
// SecretAccessObserver is the compensating detective control: every Get/List
// that resolves to the core Secret resource must invoke the observer exactly
// once, with the resolved verb/name/namespace/outcome — while accesses to
// non-Secret kinds (even ones argued with a misleading kind string) must
// never trigger it.
// ========================================

type secretAccessCall struct {
	verb      string
	name      string
	namespace string
	err       error
}

type secretAccessRecorder struct {
	mu    sync.Mutex
	calls []secretAccessCall
}

func (r *secretAccessRecorder) observe(_ context.Context, verb, name, namespace string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, secretAccessCall{verb: verb, name: name, namespace: namespace, err: err})
}

func (r *secretAccessRecorder) Calls() []secretAccessCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]secretAccessCall, len(r.calls))
	copy(out, r.calls)
	return out
}

var _ = Describe("GAP-13: K8s resolver SecretAccessObserver", func() {
	var (
		scheme   *runtime.Scheme
		mapper   *meta.DefaultRESTMapper
		kindIdx  map[string]schema.GroupKind
		recorder *secretAccessRecorder
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		mapper = buildTestMapper()
		kindIdx = buildTestKindIndex()
		recorder = &secretAccessRecorder{}
	})

	Context("when the resolved resource is the core Secret", func() {
		It("invokes the observer on a successful Get", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "prod"},
					Data:       map[string][]byte{"password": []byte("hunter2")},
				},
			)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard(),
				k8s.WithSecretAccessObserver(recorder.observe))

			_, err := resolver.Get(context.Background(), "Secret", "db-creds", "prod", "")
			Expect(err).NotTo(HaveOccurred())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].verb).To(Equal("get"))
			Expect(calls[0].name).To(Equal("db-creds"))
			Expect(calls[0].namespace).To(Equal("prod"))
			Expect(calls[0].err).NotTo(HaveOccurred())
		})

		It("invokes the observer with the error on a failed Get (not found)", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard(),
				k8s.WithSecretAccessObserver(recorder.observe))

			_, err := resolver.Get(context.Background(), "Secret", "missing", "prod", "")
			Expect(err).To(HaveOccurred())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].verb).To(Equal("get"))
			Expect(calls[0].err).To(HaveOccurred())
		})

		It("invokes the observer on List with an empty name", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "prod"}},
			)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard(),
				k8s.WithSecretAccessObserver(recorder.observe))

			_, err := resolver.List(context.Background(), "Secret", "prod", "")
			Expect(err).NotTo(HaveOccurred())

			calls := recorder.Calls()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].verb).To(Equal("list"))
			Expect(calls[0].name).To(BeEmpty())
			Expect(calls[0].namespace).To(Equal("prod"))
		})

		It("is case-insensitive on the requested kind string", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "prod"}},
			)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard(),
				k8s.WithSecretAccessObserver(recorder.observe))

			_, err := resolver.Get(context.Background(), "secret", "db-creds", "prod", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Calls()).To(HaveLen(1))
		})
	})

	Context("when the resolved resource is not the core Secret", func() {
		It("does not invoke the observer for ConfigMap reads", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "prod"}},
			)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard(),
				k8s.WithSecretAccessObserver(recorder.observe))

			_, err := resolver.Get(context.Background(), "ConfigMap", "cfg", "prod", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(recorder.Calls()).To(BeEmpty())
		})
	})

	Context("when no observer is configured", func() {
		It("does not panic on Secret access", func() {
			dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "prod"}},
			)
			resolver := k8s.NewDynamicResolver(dynClient, mapper, kindIdx, logr.Discard())

			Expect(func() {
				_, _ = resolver.Get(context.Background(), "Secret", "db-creds", "prod", "")
			}).NotTo(Panic())
		})
	})
})
