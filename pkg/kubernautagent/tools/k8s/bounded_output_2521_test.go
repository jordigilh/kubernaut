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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/k8s"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/registry"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/metadata"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("Issue #2521 bounded Kubernetes tool output", func() {
	It("UT-KA-752-112: bulk lists return bounded metadata/count summaries without ConfigMap bodies", func() {
		const corpusSize = 600
		objects := make([]runtime.Object, 0, corpusSize)
		for i := 0; i < corpusSize; i++ {
			objects = append(objects, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("large-config-%03d", i), Namespace: "prod", Labels: map[string]string{"app": "api"}},
				Data:       map[string]string{"payload": strings.Repeat("private-bulk-body ", 100)},
			})
		}
		resolver := testResolverWithMetadataObjects(objects...)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
			json.RawMessage(`{"kind":"ConfigMap","namespace":"prod"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("large-config"))
		Expect(result).To(ContainSubstring(`"item_count":100`))
		Expect(result).To(ContainSubstring(`"total_count":600`))
		Expect(result).To(ContainSubstring(`"truncated":true`))
		Expect(result).NotTo(ContainSubstring("private-bulk-body"))
		Expect(result).NotTo(ContainSubstring(`"data":`))
	})

	It("UT-KA-752-121: bulk metadata lists send the server-side item limit", func() {
		metadataClient := newMetadataSummaryClient(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "bounded-config", Namespace: "prod"},
		})
		resolver := &testDynamicResolver{resolver: k8s.NewDynamicResolver(
			fake.NewSimpleDynamicClient(buildTestScheme()), buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(metadataClient),
		)}
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		_, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
			json.RawMessage(`{"kind":"ConfigMap","namespace":"prod"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(metadataClient.lastListOptions.Limit).To(Equal(k8s.DefaultResourceListLimit))
	})

	It("UT-KA-752-113: exact-name cluster lookup sends a server-side metadata.name selector", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "target-config", Namespace: "prod"},
			Data:       map[string]string{"payload": strings.Repeat("target-body ", 100)},
		}
		resolver := testResolverWithObjects(configMap)
		var capturedFields string
		resolver.client.PrependReactor("list", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
			capturedFields = action.(k8stesting.ListAction).GetListRestrictions().Fields.String()
			return false, nil, nil
		})
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubectl_get_by_name_in_cluster",
			json.RawMessage(`{"kind":"ConfigMap","name":"target-config"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("target-config"))
		Expect(capturedFields).To(ContainSubstring("metadata.name=target-config"))
	})

	It("UT-KA-752-114: cluster-scoped metadata lists avoid a namespace subresource", func() {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-1"}}
		resolver := testResolverWithMetadataObjects(node)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_cluster",
			json.RawMessage(`{"kind":"Node"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("worker-1"))
		Expect(result).To(ContainSubstring(`"total_count":1`))
	})

	It("UT-KA-752-115: jq cannot retrieve bulk ConfigMap data", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "private-config", Namespace: "prod"},
			Data:       map[string]string{"payload": "private-jq-body"},
		}
		resolver := testResolverWithMetadataObjects(configMap)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubernetes_jq_query",
			json.RawMessage(`{"kind":"ConfigMap","jq_expr":".items[].data"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(ContainSubstring("private-jq-body"))
	})

	It("UT-KA-752-116: count reports retained and total source items separately", func() {
		objects := make([]runtime.Object, 0, 600)
		for i := 0; i < 600; i++ {
			objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("count-config-%03d", i), Namespace: "prod",
			}})
		}
		resolver := testResolverWithMetadataObjects(objects...)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubernetes_count",
			json.RawMessage(`{"kind":"ConfigMap","jq_expr":".items[].metadata.name"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("retained 100 of 600 source items"))
		Expect(result).To(ContainSubstring("truncated=true"))
	})

	It("UT-KA-752-122: count reports truncation when the source total is unknown", func() {
		objects := make([]runtime.Object, 0, 600)
		for i := 0; i < 600; i++ {
			objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("unknown-total-config-%03d", i), Namespace: "prod",
			}})
		}
		metadataClient := newMetadataSummaryClient(objects...)
		metadataClient.unknownTotal = true
		resolver := &testDynamicResolver{resolver: k8s.NewDynamicResolver(
			fake.NewSimpleDynamicClient(buildTestScheme(), objects...), buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(metadataClient),
		)}
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubernetes_count",
			json.RawMessage(`{"kind":"ConfigMap","jq_expr":".items[].metadata.name"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("retained 100 source items; total unknown; truncated=true"))
	})

	It("UT-KA-752-117: bulk lists fail closed when metadata-only access is unavailable", func() {
		configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "page-1", Namespace: "prod"}}
		client := fake.NewSimpleDynamicClient(buildTestScheme(), configMap)
		dynamicListCalled := false
		client.PrependReactor("list", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
			dynamicListCalled = true
			return true, nil, fmt.Errorf("full-object dynamic list must not be attempted")
		})
		resolver := k8s.NewDynamicResolver(client, buildTestMapper(), buildTestKindIndex(), logr.Discard(),
			k8s.WithMetadataClient(newFailingMetadataSummaryClient(
				apierrors.NewMethodNotSupported(schema.GroupResource{Resource: "configmaps"}, "list"),
			)))
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		_, err := reg.Execute(context.Background(), "kubectl_get_by_kind_in_namespace",
			json.RawMessage(`{"kind":"ConfigMap","namespace":"prod"}`))
		Expect(err).To(MatchError(ContainSubstring("metadata-only list unavailable")))
		Expect(dynamicListCalled).To(BeFalse())
	})

	It("UT-KA-752-118: JQ fails closed when metadata-only access is unavailable", func() {
		resolver := testResolverWithObjects(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "no-metadata-client", Namespace: "prod"}})
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		_, err := reg.Execute(context.Background(), "kubernetes_jq_query",
			json.RawMessage(`{"kind":"ConfigMap","jq_expr":".items[].metadata.name"}`))
		Expect(err).To(MatchError(ContainSubstring("metadata-only query unavailable")))
	})

	It("UT-KA-752-119: find_resource reports truncation instead of a false empty result", func() {
		objects := make([]runtime.Object, 0, 600)
		for i := 0; i < 600; i++ {
			objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("find-config-%03d", i), Namespace: "prod",
			}})
		}
		resolver := testResolverWithMetadataObjects(objects...)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubectl_find_resource",
			json.RawMessage(`{"kind":"ConfigMap","keyword":"find-config-599"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("results are truncated"))
		Expect(result).To(ContainSubstring("600"))
		Expect(result).NotTo(Equal("[]"))
	})

	It("UT-KA-752-123: find_resource reports truncation when retained matches are partial", func() {
		objects := make([]runtime.Object, 0, 600)
		for i := 0; i < 600; i++ {
			objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("partial-find-config-%03d", i), Namespace: "prod",
			}})
		}
		resolver := testResolverWithMetadataObjects(objects...)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		result, err := reg.Execute(context.Background(), "kubectl_find_resource",
			json.RawMessage(`{"kind":"ConfigMap","keyword":"partial-find-config-000"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("partial-find-config-000"))
		Expect(result).To(ContainSubstring("results are truncated"))
	})

	It("UT-KA-2521-120: oversized exact-name results fail before downstream sanitization", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "large-config", Namespace: "prod"},
			Data:       map[string]string{"payload": strings.Repeat("x", 100_001)},
		}
		resolver := testResolverWithObjects(configMap)
		reg := registry.New()
		for _, tool := range k8s.NewAllTools(nil, resolver) {
			reg.Register(tool)
		}

		_, err := reg.Execute(context.Background(), "kubectl_get_by_name_in_cluster",
			json.RawMessage(`{"kind":"ConfigMap","name":"large-config"}`))
		Expect(err).To(MatchError(ContainSubstring("serialized resource exceeds")))
	})
})

type testDynamicResolver struct {
	client   *fake.FakeDynamicClient
	resolver k8s.ResourceResolver
}

func (r *testDynamicResolver) Get(ctx context.Context, kind, name, namespace, apiGroup string) (interface{}, error) {
	return r.resolver.Get(ctx, kind, name, namespace, apiGroup)
}

func (r *testDynamicResolver) List(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error) {
	return r.resolver.List(ctx, kind, namespace, apiGroup)
}

func (r *testDynamicResolver) ListForQuery(ctx context.Context, kind, namespace, apiGroup string) (interface{}, error) {
	return r.resolver.ListForQuery(ctx, kind, namespace, apiGroup)
}

func (r *testDynamicResolver) GetByNameInCluster(ctx context.Context, kind, name, apiGroup string) (interface{}, error) {
	return r.resolver.GetByNameInCluster(ctx, kind, name, apiGroup)
}

func testResolverWithObjects(objects ...runtime.Object) *testDynamicResolver {
	scheme := buildTestScheme()
	client := fake.NewSimpleDynamicClient(scheme, objects...)
	return &testDynamicResolver{
		client:   client,
		resolver: k8s.NewDynamicResolver(client, buildTestMapper(), buildTestKindIndex(), logr.Discard()),
	}
}

func testResolverWithMetadataObjects(objects ...runtime.Object) *testDynamicResolver {
	scheme := buildTestScheme()
	client := fake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := newMetadataSummaryClient(objects...)
	return &testDynamicResolver{
		client:   client,
		resolver: k8s.NewDynamicResolver(client, buildTestMapper(), buildTestKindIndex(), logr.Discard(), k8s.WithMetadataClient(metadataClient)),
	}
}

type metadataSummaryClient struct {
	itemsByResource map[string][]metav1.PartialObjectMetadata
	listErr         error
	lastListOptions metav1.ListOptions
	unknownTotal    bool
}

// newMetadataSummaryClient builds a GVR-aware metadata fake with pagination
// markers matching the bounded production contract.
func newMetadataSummaryClient(objects ...runtime.Object) *metadataSummaryClient {
	client := &metadataSummaryClient{itemsByResource: make(map[string][]metav1.PartialObjectMetadata)}
	for _, object := range objects {
		accessor, err := meta.Accessor(object)
		if err != nil {
			continue
		}
		item := metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Name: accessor.GetName(), Namespace: accessor.GetNamespace(), UID: accessor.GetUID(),
			ResourceVersion: accessor.GetResourceVersion(), Labels: accessor.GetLabels(),
		}}
		client.itemsByResource[metadataResourceForObject(object)] = append(client.itemsByResource[metadataResourceForObject(object)], item)
	}
	return client
}

func newFailingMetadataSummaryClient(err error) metadata.Interface {
	return &metadataSummaryClient{
		itemsByResource: make(map[string][]metav1.PartialObjectMetadata),
		listErr:         err,
	}
}

func (c *metadataSummaryClient) Resource(gvr schema.GroupVersionResource) metadata.Getter {
	return metadataSummaryGetter{client: c, resource: gvr.Resource}
}

type metadataSummaryGetter struct {
	client    *metadataSummaryClient
	resource  string
	namespace string
}

func (g metadataSummaryGetter) Namespace(namespace string) metadata.ResourceInterface {
	g.namespace = namespace
	return g
}

// List returns only the requested resource kind and simulates one bounded page.
func (g metadataSummaryGetter) List(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	g.client.lastListOptions = options
	if g.client.listErr != nil {
		return nil, g.client.listErr
	}
	seeded := g.client.itemsByResource[g.resource]
	items := make([]metav1.PartialObjectMetadata, 0, len(seeded))
	for _, item := range seeded {
		if g.namespace == "" || item.Namespace == g.namespace {
			items = append(items, item)
		}
	}
	remaining := int64(0)
	continuation := ""
	var remainingPtr *int64
	if len(items) > int(k8s.DefaultResourceListLimit) {
		remaining = int64(len(items) - int(k8s.DefaultResourceListLimit))
		items = items[:int(k8s.DefaultResourceListLimit)]
		continuation = "metadata-summary-next-page"
		if !g.client.unknownTotal {
			remainingPtr = &remaining
		}
	}
	return &metav1.PartialObjectMetadataList{ListMeta: metav1.ListMeta{
		Continue:           continuation,
		RemainingItemCount: remainingPtr,
	}, Items: items}, nil
}

// metadataResourceForObject maps test objects to the resource key requested by
// the resolver's REST mapper.
func metadataResourceForObject(object runtime.Object) string {
	gvk := object.GetObjectKind().GroupVersionKind()
	kind := gvk.Kind
	if kind == "" {
		t := reflect.TypeOf(object)
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		kind = t.Name()
	}
	kind = strings.ToLower(kind)
	switch kind {
	case "authorizationpolicy":
		return "authorizationpolicies"
	case "poddisruptionbudget":
		return "poddisruptionbudgets"
	case "horizontalpodautoscaler":
		return "horizontalpodautoscalers"
	case "networkpolicy":
		return "networkpolicies"
	default:
		return kind + "s"
	}
}

func (metadataSummaryGetter) Delete(context.Context, string, metav1.DeleteOptions, ...string) error {
	return fmt.Errorf("metadata summary fake does not support delete")
}

func (metadataSummaryGetter) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return fmt.Errorf("metadata summary fake does not support delete collection")
}

func (metadataSummaryGetter) Get(context.Context, string, metav1.GetOptions, ...string) (*metav1.PartialObjectMetadata, error) {
	return nil, fmt.Errorf("metadata summary fake does not support get")
}

func (metadataSummaryGetter) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, fmt.Errorf("metadata summary fake does not support watch")
}

func (metadataSummaryGetter) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*metav1.PartialObjectMetadata, error) {
	return nil, fmt.Errorf("metadata summary fake does not support patch")
}
