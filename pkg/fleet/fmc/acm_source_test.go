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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/fleet/observation"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const acmSourceCluster = "prod-east"

// fakeACMSearch serves a scripted traversal. Each entry is one chunk reply, so
// a spec can reproduce a count transition, a duplicate identity or a short
// page without an ACM instance.
type fakeACMSearch struct {
	replies []fakeACMReply
	calls   []fmc.ACMSearchOptions
}

type fakeACMReply struct {
	count int
	items []map[string]json.RawMessage
	err   error
}

func (f *fakeACMSearch) Search(_ context.Context, options fmc.ACMSearchOptions) (fmc.ACMSearchPage, error) {
	f.calls = append(f.calls, options)
	if len(f.calls) > len(f.replies) {
		return fmc.ACMSearchPage{}, fmt.Errorf("unexpected extra search call at offset %d", options.Offset)
	}
	reply := f.replies[len(f.calls)-1]
	if reply.err != nil {
		return fmc.ACMSearchPage{}, reply.err
	}
	return fmc.ACMSearchPage{Count: reply.count, Items: reply.items}, nil
}

// chunked builds the reply sequence a healthy walk produces: chunks of at most
// acmSearchChunk items, a stable count, and a final short page.
func chunked(items []map[string]json.RawMessage) []fakeACMReply {
	const chunk = 50
	replies := make([]fakeACMReply, 0, len(items)/chunk+1)
	for start := 0; start < len(items); start += chunk {
		end := start + chunk
		if end > len(items) {
			end = len(items)
		}
		replies = append(replies, fakeACMReply{count: len(items), items: items[start:end]})
	}
	if len(replies) == 0 {
		replies = append(replies, fakeACMReply{count: 0})
	}
	return replies
}

func searchItem(uid, name, label string) map[string]json.RawMessage {
	const namespace = "payments"
	item := map[string]json.RawMessage{
		"_uid":       mustJSON(uid),
		"cluster":    mustJSON(acmSourceCluster),
		"apiversion": mustJSON("apps/v1"),
		"kind":       mustJSON("Deployment"),
		"name":       mustJSON(name),
	}
	if namespace != "" {
		item["namespace"] = mustJSON(namespace)
	}
	if label != "" {
		item["label"] = mustJSON(label)
	}
	return item
}

func mustJSON(value string) json.RawMessage {
	encoded, err := json.Marshal(value)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return encoded
}

var acmSourceKinds = map[string]schema.GroupVersionKind{
	"Deployment": {Group: "apps", Version: "v1", Kind: "Deployment"},
}

func newACMSource(search fmc.ACMSearch, configure ...func(*fmc.ACMSourceConfig)) *fmc.ACMSource {
	config := fmc.ACMSourceConfig{Search: search, Kinds: acmSourceKinds}
	for _, apply := range configure {
		apply(&config)
	}
	source, err := fmc.NewACMSource(config)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return source
}

func readAll(source *fmc.ACMSource) (observation.ListObservation, error) {
	return readAllKind(source, "apps/v1", "Deployment")
}

// readAllKind collects one specific kind, so specs about cluster-scoped kinds
// request the kind they actually assert against.
func readAllKind(source *fmc.ACMSource, apiVersion, kind string) (observation.ListObservation, error) {
	reader, err := source.ReaderFor(context.Background(), registry.ClusterInfo{ID: acmSourceCluster, RegistrationUID: "registration-1"})
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return reader.ListObserved(context.Background(), observation.ListRequest{
		APIVersion: apiVersion, Kind: kind, Limit: observation.ObservationPageLimit,
	})
}

var _ = Describe("ACM source collector (DD-FLEET-017 D4)", func() {
	// UT-FMC-SRC-2661-014
	It("UT-FMC-SRC-2661-014 [BR-SAFETY-001]: walks ordered chunks and reports a complete observation", func() {
		items := make([]map[string]json.RawMessage, 0, 3)
		for index := 0; index < 3; index++ {
			items = append(items, searchItem(fmt.Sprintf("uid-%d", index), fmt.Sprintf("app-%d", index), "kubernaut.ai/managed=true"))
		}
		search := &fakeACMSearch{replies: chunked(items)}

		observed, err := readAll(newACMSource(search))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Complete).To(BeTrue())
		Expect(observed.Items).To(HaveLen(3))
		Expect(observed.ItemCount).To(Equal(3))
		Expect(observed.PageCount).To(Equal(1))
		Expect(search.calls[0].OrderBy).To(Equal("uid asc"),
			"ordering must be requested explicitly: an invalid order is silently ignored by Search")
		Expect(search.calls[0].ClusterID).To(Equal(acmSourceCluster))
		Expect(search.calls[0].Group).To(Equal("apps"))
		Expect(search.calls[0].Version).To(Equal("v1"))
		Expect(search.calls[0].Kind).To(Equal("Deployment"))
	})

	// UT-FMC-SRC-2661-011
	It("UT-FMC-SRC-2661-011 [BR-SCOPE-001]: collects every object with its projected label state", func() {
		items := []map[string]json.RawMessage{
			searchItem("uid-1", "managed", "kubernaut.ai/managed=true"),
			searchItem("uid-2", "refused", "kubernaut.ai/managed=false"),
			searchItem("uid-3", "plain", ""),
		}

		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked(items)}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items).To(HaveLen(3))
		Expect(observed.Items[0].LabelState).To(Equal(scope.LabelStateTrue))
		Expect(observed.Items[0].LabelsPresent).To(BeTrue())
		Expect(observed.Items[1].LabelState).To(Equal(scope.LabelStateFalse))
		Expect(observed.Items[2].LabelState).To(Equal(scope.LabelStateAbsent))
		Expect(observed.Items[2].LabelsPresent).To(BeFalse(),
			"Search omits the label field entirely when an object has no labels")
		Expect(observed.Items[0].Object.GetName()).To(Equal("managed"))
		Expect(observed.Items[0].Object.GetNamespace()).To(Equal("payments"))
		Expect(observed.Items[0].Object.GetKind()).To(Equal("Deployment"))
	})

	// UT-FMC-SRC-2661-018
	It("UT-FMC-SRC-2661-018 [BR-SCOPE-002]: never projects the Search uid into the object", func() {
		observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{
			searchItem("uid-secret", "api", "kubernaut.ai/managed=true"),
		})}))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Items).To(HaveLen(1))
		Expect(observed.Items[0].Object.Object).ToNot(HaveKey("_uid"))
		Expect(observed.Items[0].Object.GetUID()).To(BeEmpty(), "no target-UID claim (DD-FLEET-015)")
		Expect(fmt.Sprint(observed.Items[0].Object.Object)).ToNot(ContainSubstring("uid-secret"))
	})

	// UT-FMC-SRC-2661-013
	DescribeTable("UT-FMC-SRC-2661-013 [BR-SCOPE-002]: an unusable identity fails the kind, not just the item",
		func(mutate func(map[string]json.RawMessage)) {
			item := searchItem("uid-1", "api", "kubernaut.ai/managed=true")
			mutate(item)

			observed, err := readAll(newACMSource(&fakeACMSearch{replies: chunked([]map[string]json.RawMessage{item})}))

			Expect(err).To(HaveOccurred())
			Expect(observed.Complete).To(BeFalse())
			Expect(observation.FailureOf(err)).ToNot(Equal(observation.FailureNone))
		},
		Entry("missing apiversion", func(item map[string]json.RawMessage) { delete(item, "apiversion") }),
		Entry("cross-cluster item", func(item map[string]json.RawMessage) { item["cluster"] = mustJSON("prod-west") }),
		Entry("kind mismatch", func(item map[string]json.RawMessage) { item["kind"] = mustJSON("StatefulSet") }),
		Entry("group mismatch", func(item map[string]json.RawMessage) { item["apiversion"] = mustJSON("batch/v1") }),
		Entry("missing name", func(item map[string]json.RawMessage) { delete(item, "name") }),
		Entry("namespaced item without namespace", func(item map[string]json.RawMessage) { delete(item, "namespace") }),
	)

	// UT-FMC-SRC-2661-015
	It("UT-FMC-SRC-2661-015 [BR-SAFETY-001]: restarts once from offset 0 when the count moves, then completes", func() {
		// A full chunk keeps the walk going (a short page legitimately ends it),
		// so the transition is observed on a second page exactly as it happens
		// against Search.
		fullPage := make([]map[string]json.RawMessage, 0, 50)
		for index := 0; index < 50; index++ {
			fullPage = append(fullPage, searchItem(fmt.Sprintf("uid-%03d", index), fmt.Sprintf("app-%03d", index), "kubernaut.ai/managed=true"))
		}
		search := &fakeACMSearch{replies: []fakeACMReply{
			{count: 52, items: fullPage},
			{count: 50, items: fullPage[50:]},
			{count: 50, items: fullPage},
		}}

		observed, err := readAll(newACMSource(search))

		Expect(err).ToNot(HaveOccurred())
		Expect(observed.Complete).To(BeTrue(), "a single count transition is recoverable")
		Expect(observed.Items).To(HaveLen(50))
		Expect(search.calls).To(HaveLen(3))
		Expect(search.calls[2].Offset).To(Equal(0), "the restart re-walks from the beginning")
	})

	It("UT-FMC-SRC-2661-015 [BR-SAFETY-001]: a second count transition leaves the collection incomplete", func() {
		// A full chunk keeps the walk going (a short page legitimately ends
		// it), so both transitions are observed on continued pages and the
		// failure comes from the restart guard, not an items/count mismatch
		// on a terminal short page.
		fullPage := make([]map[string]json.RawMessage, 0, 50)
		for index := 0; index < 50; index++ {
			fullPage = append(fullPage, searchItem(fmt.Sprintf("uid-%03d", index), fmt.Sprintf("app-%03d", index), "kubernaut.ai/managed=true"))
		}
		shortTail := []map[string]json.RawMessage{searchItem("uid-050", "tail", "kubernaut.ai/managed=true")}
		search := &fakeACMSearch{replies: []fakeACMReply{
			{count: 51, items: fullPage},  // full page: the walk continues
			{count: 50, items: shortTail}, // first transition: restart from 0
			{count: 49, items: fullPage},  // second transition: the guard fires
		}}

		observed, err := readAll(newACMSource(search))

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("count changed twice"),
			"the failure must come from the restart guard, not a terminal short page")
		Expect(observed.Complete).To(BeFalse())
		offsets := make([]int, 0, len(search.calls))
		for _, call := range search.calls {
			offsets = append(offsets, call.Offset)
		}
		Expect(offsets).To(Equal([]int{0, 50, 0}),
			"the walk continues past the full page, then restarts from offset 0")
	})

	// UT-FMC-SRC-2661-016
	DescribeTable("UT-FMC-SRC-2661-016 [BR-SAFETY-001]: a traversal anomaly leaves the collection incomplete",
		func(replies []fakeACMReply) {
			observed, err := readAll(newACMSource(&fakeACMSearch{replies: replies}))

			Expect(err).To(HaveOccurred())
			Expect(observed.Complete).To(BeFalse())
			Expect(observed.Failure).ToNot(Equal(observation.FailureNone))
		},
		Entry("non-increasing uid", []fakeACMReply{
			{count: 2, items: []map[string]json.RawMessage{
				searchItem("uid-2", "two", "kubernaut.ai/managed=true"),
				searchItem("uid-1", "one", "kubernaut.ai/managed=true"),
			}},
		}),
		Entry("duplicate uid", []fakeACMReply{
			{count: 2, items: []map[string]json.RawMessage{
				searchItem("uid-1", "one", "kubernaut.ai/managed=true"),
				searchItem("uid-1", "one-again", "kubernaut.ai/managed=true"),
			}},
		}),
		Entry("missing uid", []fakeACMReply{
			{count: 1, items: []map[string]json.RawMessage{searchItem("", "one", "kubernaut.ai/managed=true")}},
		}),
		Entry("reported count exceeds the items returned", []fakeACMReply{
			{count: 3, items: []map[string]json.RawMessage{searchItem("uid-1", "one", "kubernaut.ai/managed=true")}},
		}),
	)

	// UT-FMC-SRC-2661-019
	It("UT-FMC-SRC-2661-019 [BR-SAFETY-001]: a completed zero-item traversal is a complete empty observation", func() {
		observed, err := readAll(newACMSource(&fakeACMSearch{replies: []fakeACMReply{{count: 0}}}))

		Expect(err).ToNot(HaveOccurred(),
			"the adapter never decides empty vs partial; a successful walk is complete")
		Expect(observed.Complete).To(BeTrue())
		Expect(observed.Items).To(BeEmpty())
		Expect(observed.ItemCount).To(Equal(0))
	})

	// UT-FMC-SRC-2661-017
	It("UT-FMC-SRC-2661-017 [BR-SAFETY-001]: a transport failure aborts before the collection is reported", func() {
		observed, err := readAll(newACMSource(&fakeACMSearch{replies: []fakeACMReply{{err: fmt.Errorf("search unavailable")}}}))

		Expect(err).To(HaveOccurred())
		Expect(observed.Complete).To(BeFalse())
		Expect(observed.Items).To(BeEmpty(), "a failed kind publishes no partial set")
		Expect(observation.FailureOf(err)).To(Equal(observation.FailureTransport))
	})

	// UT-FMC-SRC-2661-021
	It("UT-FMC-SRC-2661-021 [BR-SAFETY-001]: starts no request when the remaining budget is below the floor", func() {
		search := &fakeACMSearch{replies: chunked([]map[string]json.RawMessage{
			searchItem("uid-1", "one", "kubernaut.ai/managed=true"),
		})}
		now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
		source := newACMSource(search, func(config *fmc.ACMSourceConfig) {
			config.Now = func() time.Time { return now }
		})
		reader, err := source.ReaderFor(context.Background(), registry.ClusterInfo{ID: acmSourceCluster})
		Expect(err).ToNot(HaveOccurred())

		// 3 s left, 5 s commit reserve -> negative remaining, below the 2 s floor.
		ctx := fmc.WithCollectionDeadline(context.Background(), now.Add(3*time.Second))
		observed, err := reader.ListObserved(ctx, observation.ListRequest{APIVersion: "apps/v1", Kind: "Deployment"})

		Expect(err).To(HaveOccurred())
		Expect(observed.Complete).To(BeFalse())
		Expect(search.calls).To(BeEmpty(),
			"a request that cannot finish inside the window must not start")
	})

	// UT-FMC-SRC-2661-022
	It("UT-FMC-SRC-2661-022 [BR-SAFETY-001]: an unknown kind is a tool failure, never a silent empty collection", func() {
		source := newACMSource(&fakeACMSearch{}, func(config *fmc.ACMSourceConfig) {
			config.Kinds = map[string]schema.GroupVersionKind{"ConfigMap": {Version: "v1", Kind: "ConfigMap"}}
		})
		reader, err := source.ReaderFor(context.Background(), registry.ClusterInfo{ID: acmSourceCluster})
		Expect(err).ToNot(HaveOccurred())

		observed, err := reader.ListObserved(context.Background(), observation.ListRequest{APIVersion: "apps/v1", Kind: "Deployment"})

		Expect(err).To(HaveOccurred())
		Expect(observed.Complete).To(BeFalse())
		Expect(observed.Failure).To(Equal(observation.FailureTool))
	})
})
