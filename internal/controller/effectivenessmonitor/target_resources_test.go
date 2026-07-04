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

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	canonicalhash "github.com/jordigilh/kubernaut/pkg/shared/hash"
)

// Issue #1542 follow-up history: getTargetHealthStatus/listActivePodNames/
// resolveConfigMapHashes previously had to call SetGroupVersionKind
// explicitly on every object before Get/List, because pkg/fleet/mcpclient
// (used for fleet/cross-cluster reads) required it and had no scheme to
// infer it from -- unlike the cached controller-runtime client used for
// local reads, which always infers GVK from its own scheme. That per-call-
// site workaround was removed once mcpclient itself was hardened to infer
// GVK from a runtime.Scheme (see pkg/fleet/mcpclient/gvk.go's ensureGVK),
// so these functions now rely solely on the reader's own GVK handling,
// matching how every other client.Reader/client.Writer caller in the
// codebase already behaves. These tests exercise the functional behavior
// (not GVK plumbing, which is covered by pkg/fleet/mcpclient's own
// UT-FLEET-GVK regression suite) against a plain fake client.
var _ = Describe("target_resources: health/pod-name/hash resolution", func() {
	var (
		ctx    context.Context
		r      *Reconciler
		reader client.Reader
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
		reader = fake.NewClientBuilder().WithObjects(pod, cm).Build()
		r = &Reconciler{}
	})

	// UT-EM-060-001
	It("getTargetHealthStatus finds active pods for a Deployment target", func() {
		status := r.getTargetHealthStatus(ctx, reader, eav1.TargetResource{
			Kind: "Deployment", Name: "crashloop-app", Namespace: "target-ns",
		}, nil)
		Expect(status.TargetExists).To(BeTrue())
		Expect(status.ReadyReplicas).To(BeNumerically("==", 1))
	})

	// UT-EM-060-002
	It("getTargetHealthStatus finds a Pod target directly", func() {
		status := r.getTargetHealthStatus(ctx, reader, eav1.TargetResource{
			Kind: "Pod", Name: "crashloop-app-abc123", Namespace: "target-ns",
		}, nil)
		Expect(status.TargetExists).To(BeTrue())
	})

	// UT-EM-060-003
	It("listActivePodNames lists active pods for a workload target", func() {
		names := r.listActivePodNames(ctx, reader, eav1.TargetResource{
			Kind: "Deployment", Name: "crashloop-app", Namespace: "target-ns",
		})
		Expect(names).To(ConsistOf("crashloop-app-abc123"))
	})

	// UT-EM-060-004
	It("resolveConfigMapHashes fetches and hashes a referenced ConfigMap", func() {
		r.readerFactory = fleetReaderFactoryStub{}
		hashes := r.resolveConfigMapHashes(ctx, reader, map[string]interface{}{
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
		// resolveConfigMapHashes falls back to a sentinel hash on ANY Get
		// error (by design, to keep drift detection deterministic), so a
		// bare HaveKey assertion would pass even on a fetch failure. Assert
		// the real ConfigMap data hash to distinguish a successful Get from
		// the sentinel fallback path.
		wantHash, err := canonicalhash.ConfigMapDataHash(map[string]string{"key": "value"}, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(hashes).To(HaveKeyWithValue("crashloop-app-config", wantHash))
	})
})

// fleetReaderFactoryStub is a minimal non-nil fleet.ReaderFactory so
// resolveConfigMapHashes routes ConfigMap reads through the passed-in
// reader instead of r.apiReader (nil in this unit test).
type fleetReaderFactoryStub struct{}

func (fleetReaderFactoryStub) ReaderFor(_ context.Context, _ string) (client.Reader, error) {
	return nil, nil
}
