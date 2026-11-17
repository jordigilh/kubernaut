package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("Owned registry publication [BR-INTEGRATION-065, BR-SCOPE-001]", func() {
	It("W01: consumes every page under the same namespace/selector/resourceVersion before publication", func() {
		var pages atomic.Int32
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/apis/gateway.envoyproxy.io/v1alpha1/namespaces/registrations/backends" || r.URL.Query().Get("labelSelector") != ManagedLabel+"=true" {
				http.Error(w, "incorrect list identity", http.StatusBadRequest)
				return
			}
			page := pages.Add(1)
			name := "east"
			if page == 2 {
				name = "west"
				if r.URL.Query().Get("continue") != "next-page" {
					http.Error(w, "missing continuation", http.StatusBadRequest)
					return
				}
			}
			obj := newBackendUnstructured(name, "https://mcp")
			obj.SetNamespace("registrations")
			list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*obj}}
			list.SetAPIVersion("gateway.envoyproxy.io/v1alpha1")
			list.SetKind("BackendList")
			list.SetResourceVersion("10")
			if page == 1 {
				list.SetContinue("next-page")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(list)
		}))
		DeferCleanup(api.Close)
		client, err := dynamic.NewForConfig(&rest.Config{Host: api.URL})
		Expect(err).NotTo(HaveOccurred())
		w := NewEAIGWRegistry(client, EAIGWRegistryConfig{Namespace: "registrations"}, nil, logr.Discard())
		DeferCleanup(w.Stop)
		cfg := registryWatchConfig{client: client, gvr: BackendGVR, config: w.config, trackable: w.trackableClusterInfo}
		clusters, version, err := cfg.list(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal("10"))
		Expect(clusters).To(HaveLen(2))
		Expect(pages.Load()).To(Equal(int32(2)))
		Expect(w.List()).To(BeEmpty(), "private LIST must not publish membership or readiness")
		Expect(w.Ready()).To(BeFalse())
	})

	DescribeTable("W01/W02: rejects incomplete or invalid LIST evidence", func(body string) {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		DeferCleanup(api.Close)
		client, err := dynamic.NewForConfig(&rest.Config{Host: api.URL})
		Expect(err).NotTo(HaveOccurred())
		w := NewEAIGWRegistry(client, EAIGWRegistryConfig{Namespace: "registrations"}, nil, logr.Discard())
		DeferCleanup(w.Stop)
		cfg := registryWatchConfig{client: client, gvr: BackendGVR, config: w.config, trackable: w.trackableClusterInfo}
		_, _, err = cfg.list(context.Background())
		Expect(err).To(HaveOccurred())
		Expect(w.List()).To(BeEmpty())
		Expect(w.Ready()).To(BeFalse())
	},
		Entry("missing resourceVersion", `{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","items":[]}`),
		Entry("incomplete final page", `{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","metadata":{"resourceVersion":"10","remainingItemCount":1},"items":[]}`),
		Entry("repeated continuation", `{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","metadata":{"resourceVersion":"10","continue":"same"},"items":[]}`),
		Entry("missing UID", `{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","metadata":{"resourceVersion":"10"},"items":[{"metadata":{"name":"east","namespace":"registrations","labels":{"kubernaut.ai/managed":"true"}}}]}`),
	)

	DescribeTable("W02/W03: an empty or nil WATCH can never establish health", func(nilWatch bool) {
		client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{BackendGVR: "BackendList"})
		client.PrependReactor("list", "backends", func(k8stesting.Action) (bool, runtime.Object, error) {
			list := &unstructured.UnstructuredList{}
			list.SetResourceVersion("10")
			return true, list, nil
		})
		client.PrependWatchReactor("backends", func(k8stesting.Action) (bool, watch.Interface, error) {
			if nilWatch {
				return true, nil, nil
			}
			return true, watch.NewEmptyWatch(), nil
		})
		w := NewEAIGWRegistry(client, EAIGWRegistryConfig{}, nil, logr.Discard())
		DeferCleanup(w.Stop)
		cfg := registryWatchConfig{client: client, gvr: BackendGVR, config: w.config, trackable: w.trackableClusterInfo,
			publish: watchPublication{begin: w.begin, install: w.install, invalidate: w.invalidate, apply: w.applyWatchEvent}}
		err := cfg.session(context.Background(), func() { Fail("closed WATCH cannot install a session") })
		Expect(err).To(HaveOccurred())
		Expect(w.Ready()).To(BeFalse())
	}, Entry("nil", true), Entry("already closed", false))
})
