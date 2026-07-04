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

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// gvkStrictReader wraps a delegate client.Reader and rejects Get/List calls
// for objects that don't carry an explicit GroupVersionKind, mirroring the
// contract of pkg/fleet/mcpclient.Client (used for cross-cluster fleet
// reads). The real controller-runtime cached/fake client infers GVK from its
// scheme, so it never surfaces this class of bug locally — this spy is what
// lets a fast unit test catch it (Issue #1542 follow-up: fleet EM health
// assessment failed with "list object GVK Kind must be set before calling
// List" because getTargetHealthStatus/listActivePodNames/
// resolveConfigMapHashes built plain typed objects without TypeMeta).
type gvkStrictReader struct {
	client.Reader
}

func (g *gvkStrictReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if obj.GetObjectKind().GroupVersionKind().Kind == "" {
		return fmt.Errorf("object GVK Kind must be set before calling Get")
	}
	return g.Reader.Get(ctx, key, obj, opts...)
}

func (g *gvkStrictReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if list.GetObjectKind().GroupVersionKind().Kind == "" {
		return fmt.Errorf("list object GVK Kind must be set before calling List")
	}
	return g.Reader.List(ctx, list, opts...)
}

var _ = Describe("target_resources: fleet (GVK-strict) reader compatibility", func() {
	var (
		ctx    context.Context
		r      *Reconciler
		strict *gvkStrictReader
	)

	BeforeEach(func() {
		ctx = context.Background()
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "crashloop-app-abc123",
				Namespace: "target-ns",
				Labels:    map[string]string{"app": "crashloop-app"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true},
				},
			},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop-app-config", Namespace: "target-ns"},
			Data:       map[string]string{"key": "value"},
		}
		fakeClient := fake.NewClientBuilder().WithObjects(pod, cm).Build()
		strict = &gvkStrictReader{Reader: fakeClient}
		r = &Reconciler{}
	})

	// UT-EM-060-001
	It("getTargetHealthStatus sets GVK before listing pods for a Deployment target", func() {
		status := r.getTargetHealthStatus(ctx, strict, eav1.TargetResource{
			Kind: "Deployment", Name: "crashloop-app", Namespace: "target-ns",
		}, nil)
		Expect(status.TargetExists).To(BeTrue(),
			"a GVK-strict reader (fleet MCP client contract) must not cause a false 'target not found'")
		Expect(status.ReadyReplicas).To(BeNumerically("==", 1))
	})

	// UT-EM-060-002
	It("getTargetHealthStatus sets GVK before getting a Pod target", func() {
		status := r.getTargetHealthStatus(ctx, strict, eav1.TargetResource{
			Kind: "Pod", Name: "crashloop-app-abc123", Namespace: "target-ns",
		}, nil)
		Expect(status.TargetExists).To(BeTrue())
	})

	// UT-EM-060-003
	It("listActivePodNames sets GVK before listing pods", func() {
		names := r.listActivePodNames(ctx, strict, eav1.TargetResource{
			Kind: "Deployment", Name: "crashloop-app", Namespace: "target-ns",
		})
		Expect(names).To(ConsistOf("crashloop-app-abc123"))
	})

	// UT-EM-060-004
	It("resolveConfigMapHashes sets GVK before getting a ConfigMap", func() {
		r.readerFactory = fleetReaderFactoryStub{}
		hashes := r.resolveConfigMapHashes(ctx, strict, map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"volumes": []interface{}{
						map[string]interface{}{
							"configMap": map[string]interface{}{"name": "crashloop-app-config"},
						},
					},
				},
			},
		}, eav1.TargetResource{Kind: "Deployment", Name: "crashloop-app", Namespace: "target-ns"})
		// resolveConfigMapHashes falls back to a sentinel hash on ANY Get error
		// (by design, to keep drift detection deterministic), so a bare
		// HaveKey assertion would pass even if the GVK-strict Get failed.
		// Assert the real ConfigMap data hash to actually distinguish a
		// successful strict-reader Get from the sentinel fallback path.
		wantHash, err := canonicalhash.ConfigMapDataHash(map[string]string{"key": "value"}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(hashes).To(HaveKeyWithValue("crashloop-app-config", wantHash))
	})
})

// fleetReaderFactoryStub is a minimal non-nil fleet.ReaderFactory so
// resolveConfigMapHashes routes ConfigMap reads through the passed-in
// (GVK-strict) reader instead of r.apiReader (nil in this unit test).
type fleetReaderFactoryStub struct{}

func (fleetReaderFactoryStub) ReaderFor(_ context.Context, _ string) (client.Reader, error) {
	return nil, nil
}
