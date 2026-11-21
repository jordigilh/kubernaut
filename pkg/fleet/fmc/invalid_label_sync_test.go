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

package fmc_test

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/fleet/scopecache"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// invalidLabelReader serves one complete source scan whose items carry
// explicit managed-label states (e.g. kubernaut.ai/managed: "banana"),
// exactly what observation.ObservedItem.LabelState preserves for the syncer
// (#2686). states[i] is the label state served for item i; an out-of-
// whitelist state exercises the write boundary's rejection.
type invalidLabelReader struct {
	states []scope.LabelState
}

func (r *invalidLabelReader) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return nil
}

func (r *invalidLabelReader) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

func (r *invalidLabelReader) ListObserved(_ context.Context, _ observation.ListRequest) (observation.ListObservation, error) {
	items := make([]observation.ObservedItem, 0, len(r.states))
	for index, state := range r.states {
		pod := unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("observed-%d", index),
				"namespace": "default",
				"labels":    map[string]interface{}{"kubernaut.ai/managed": "banana"},
			},
		}}
		items = append(items, observation.ObservedItem{
			Object: pod, LabelsPresent: true, LabelState: state,
		})
	}
	return observation.ListObservation{
		Items: items, Complete: true, PageCount: 1, ItemCount: len(items),
	}, nil
}

func invalidLabelReaderFactory(states ...scope.LabelState) fmc.ObservedReaderFactory {
	return fmc.ObservedReaderFactoryFunc(func(_ context.Context, _ registry.ClusterInfo) (fmc.ObservedReader, error) {
		return &invalidLabelReader{states: states}, nil
	})
}

// #2686: an invalid managed-label observation is a complete, decisive source
// observation. The syncer must carry it through both write lanes instead of
// failing the whole cycle, because discarding it retains the last committed
// authorization until TTL — a bounded fail-open window on an AC-6 control.
var _ = Describe("FMC syncer invalid-label observation (BR-SCOPE-001, #2686)", func() {
	ctx := context.Background()

	It("UT-FMC-2686-005 [BR-SCOPE-001, #2686]: the legacy write lane records an invalid label state instead of failing the sync", func() {
		writer := &stubWriter{}
		syncer := fmc.NewSyncerWithReaderFactory(
			newStubRegistry(registry.ClusterInfo{ID: "cluster-a"}),
			invalidLabelReaderFactory(scope.LabelStateInvalid),
			writer,
			fmc.Config{KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
			logr.Discard(),
			fmc.NewMetrics(prometheus.NewPedanticRegistry()),
		)

		Expect(syncer.SyncCluster(ctx, registry.ClusterInfo{ID: "cluster-a"})).
			To(Succeed(), "an invalid label value is a completed observation, not a failed sync")

		key, err := scopecache.BuildKey("cluster-a", "", "v1", "Pod", "default", "observed-0")
		Expect(err).ToNot(HaveOccurred())
		Expect(writer.observedStates[key]).To(Equal(scope.LabelStateInvalid),
			"the legacy cache value must carry the invalid state so readers fail closed")
	})

	It("UT-FMC-2686-006 [BR-SCOPE-001, #2686]: the repair lane stages and commits an invalid label observation", func() {
		info := registry.ClusterInfo{ID: "cluster-a", RegistrationUID: "registration-a", ToolPrefix: "cluster-a__"}
		statusStore := &lifecycleStatusStore{}
		lease := &lifecycleLease{fence: scopecache.WriterFence{HolderPodUID: "pod-a", HolderSessionUID: "session-a", LeaseEpoch: 1, Fence: 1}}
		syncer := newLifecycleSyncer(newFencedRegistry(info, 1), invalidLabelReaderFactory(scope.LabelStateInvalid), statusStore, lease, []string{"Pod"})

		Expect(syncer.SyncCluster(ctx, info)).To(Succeed(),
			"a complete source scan whose label value is invalid must still repair and commit")

		Expect(statusStore.repairRequest).ToNot(BeNil())
		Expect(statusStore.repairRequest.Observations.Items).To(HaveLen(1))
		Expect(statusStore.repairRequest.Observations.Items[0].LabelState).To(Equal(scope.LabelStateInvalid),
			"the staged repair must carry the invalid observation so the commit revokes the previous authorization")
		Expect(statusStore.repairRequest.Observations.Items[0].LabelsPresent).To(BeTrue())
	})

	It("UT-FMC-2686-009 [BR-SCOPE-001, #2686]: the legacy write lane rejects a label state outside the write boundary without partial writes", func() {
		writer := &stubWriter{}
		syncer := fmc.NewSyncerWithReaderFactory(
			newStubRegistry(registry.ClusterInfo{ID: "cluster-a"}),
			invalidLabelReaderFactory(scope.LabelStateTrue, scope.LabelState("bogus")),
			writer,
			fmc.Config{KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
			logr.Discard(),
			fmc.NewMetrics(prometheus.NewPedanticRegistry()),
		)

		// A valid item in front of the non-conforming one proves the whole
		// observation set is validated before any cache mutation: neither
		// the valid item nor the bogus one may be written.
		err := syncer.SyncCluster(ctx, registry.ClusterInfo{ID: "cluster-a"})
		Expect(err).To(MatchError(ContainSubstring("unsupported observation label state")),
			"both write lanes share knownLabelState as the single write-boundary rule")
		Expect(writer.keysWritten).To(BeEmpty(), "a rejected observation set must not reach the cache, not even its valid prefix")

		// The same syncer must recover on the next, conforming scan: the
		// rejection is a per-cycle outcome, not a poisoned writer.
		recovering := fmc.NewSyncerWithReaderFactory(
			newStubRegistry(registry.ClusterInfo{ID: "cluster-a"}),
			invalidLabelReaderFactory(scope.LabelStateTrue, scope.LabelStateFalse),
			writer,
			fmc.Config{KeyTTL: 30 * time.Second, ResourceKinds: []string{"Pod"}},
			logr.Discard(),
			fmc.NewMetrics(prometheus.NewPedanticRegistry()),
		)
		Expect(recovering.SyncCluster(ctx, registry.ClusterInfo{ID: "cluster-a"})).
			To(Succeed(), "a conforming scan after a rejected one must still write")
		Expect(writer.keysWritten).To(HaveLen(2), "both valid observations must be written after recovery")
	})
})
