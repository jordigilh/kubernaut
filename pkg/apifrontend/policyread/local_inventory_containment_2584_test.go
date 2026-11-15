package policyread_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/metadata"
)

var _ = Describe("BR-SAFETY-001: bounded local inventory reads", func() {
	It("UT-AF-2592-METADATA-001 [BR-SAFETY-001]: refuses construction without a metadata client instead of falling back to full objects", func() {
		reader, err := policyread.NewLocalInventoryReader(policyread.LocalInventoryConfig{
			ScopeResolver: &localScopeStub{},
		})

		Expect(reader).To(BeNil())
		Expect(err).To(MatchError("local inventory metadata client is required"))
	})

	It("UT-AF-2592-PAGE-001 [BR-API-001]: traverses every Kubernetes metadata page with Limit and Continue before sorting and paging", func() {
		client := &metadataPageClient{}
		client.list = func(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			switch options.Continue {
			case "":
				return metadataPage("alpha", "continue-a"), nil
			case "continue-a":
				return metadataPage("beta"), nil
			default:
				return nil, fmt.Errorf("unexpected continuation %q", options.Continue)
			}
		}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{
			BackendPageLimit: 1,
		})

		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateSuccess))
		Expect(page.Items).To(HaveLen(1))
		Expect(page.Items[0].Name).To(Equal("alpha"))
		Expect(page.NextCursor).NotTo(BeEmpty())
		Expect(client.options).To(HaveLen(2))
		Expect(client.options[0].Limit).To(Equal(int64(1)))
		Expect(client.options[0].Continue).To(BeEmpty())
		Expect(client.options[1].Limit).To(Equal(int64(1)))
		Expect(client.options[1].Continue).To(Equal("continue-a"))
	})

	It("UT-AF-2592-PAGE-002 [BR-SAFETY-001]: defaults metadata traversal to a positive internal item-count hint", func() {
		client := &metadataPageClient{list: func(context.Context, metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			return metadataPage("alpha"), nil
		}}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{})

		_, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1})

		Expect(err).NotTo(HaveOccurred())
		Expect(client.options).To(HaveLen(1))
		Expect(client.options[0].Limit).To(BeNumerically(">", int64(0)))
	})

	It("UT-AF-2592-KINDS-001 [BR-SAFETY-001]: excludes Secrets and ConfigMaps from default local inventory kinds", func() {
		for _, kind := range policyread.DefaultLocalResourceKinds {
			Expect(kind.Kind).NotTo(Equal("Secret"))
			Expect(kind.Kind).NotTo(Equal("ConfigMap"))
		}
	})

	DescribeTable("UT-AF-2592-CONTINUATION-001 [BR-SAFETY-001]: returns a state-only failure for unsafe continuation", func(listFunc metadataListFunc) {
		client := &metadataPageClient{list: listFunc}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{BackendPageLimit: 1})

		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 1})

		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	},
		Entry("repeated continuation", metadataListFunc(func(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			if options.Continue == "" || options.Continue == "same-token" {
				return metadataPage("alpha", "same-token"), nil
			}
			return nil, errors.New("unexpected continuation")
		})),
		Entry("expired continuation", metadataListFunc(func(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			if options.Continue == "" {
				return metadataPage("alpha", "expired-token"), nil
			}
			return nil, errors.New("continue token expired")
		})),
	)

	It("UT-AF-2592-CANCEL-001 [BR-SAFETY-001]: returns a state-only failure when cancellation occurs between backend pages", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		client := &metadataPageClient{}
		client.list = func(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			if options.Continue == "" {
				cancel()
				return metadataPage("alpha", "cancelled"), nil
			}
			return metadataPage("beta"), nil
		}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{BackendPageLimit: 1})

		page, err := reader.List(ctx, inventory.Filters{ClusterID: "local", Limit: 1})

		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("UT-AF-2592-PROJECT-001 [BR-SAFETY-001]: retains only canonical projections after each metadata page is discarded", func() {
		firstPage := metadataPage("alpha", "continue-a")
		client := &metadataPageClient{}
		client.list = func(_ context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			if options.Continue == "" {
				return firstPage, nil
			}
			return metadataPage("beta"), nil
		}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{BackendPageLimit: 1})

		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 10})

		Expect(err).NotTo(HaveOccurred())
		Expect(page.Items).To(HaveLen(2))
		Expect(page.Items[0].Name).To(Equal("alpha"))
		Expect(page.Items[1].Name).To(Equal("beta"))
		Expect(firstPage.Items).To(BeEmpty(), "backend page objects must be discarded after projection")
		encoded, err := json.Marshal(page.Items)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("kubernaut.ai/managed"))
	})

	It("UT-AF-2592-BUDGET-001 [BR-SAFETY-001]: fails closed before retaining a projected resource over the working-set budget", func() {
		client := &metadataPageClient{list: func(context.Context, metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			return metadataPage("alpha"), nil
		}}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{
			WorkingSetBytes: 1,
		})

		page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local", Limit: 10})

		Expect(err).To(HaveOccurred())
		Expect(page.State).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
	})

	It("UT-AF-2592-SEMAPHORE-001 [BR-SAFETY-001]: shares a cancellable per-reader semaphore between List and Get", func() {
		entered := make(chan struct{})
		release := make(chan struct{})
		client := &metadataPageClient{}
		client.list = func(ctx context.Context, _ metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
			select {
			case <-entered:
			default:
				close(entered)
			}
			select {
			case <-release:
				return metadataPage("alpha"), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		reader := newBoundedReader(client, policyread.LocalInventoryConfig{
			MaxConcurrentReads: 1,
			OperationTimeout:   time.Second,
		})

		firstDone := make(chan inventory.Page, 1)
		firstErr := make(chan error, 1)
		go func() {
			page, err := reader.List(context.Background(), inventory.Filters{ClusterID: "local"})
			firstDone <- page
			firstErr <- err
		}()
		Eventually(entered).Should(BeClosed())

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		secondPage, secondErr := reader.Get(cancelled, "local", "r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

		Expect(secondErr).To(HaveOccurred())
		Expect(errors.Is(secondErr, context.Canceled)).To(BeTrue())
		Expect(secondPage).To(Equal(inventory.Resource{}))
		close(release)
		Eventually(firstDone).Should(Receive())
		Eventually(firstErr).Should(Receive(BeNil()))
	})
})

type metadataListFunc func(context.Context, metav1.ListOptions) (*metav1.PartialObjectMetadataList, error)

type metadataPageClient struct {
	mu      sync.Mutex
	options []metav1.ListOptions
	list    metadataListFunc
}

func (c *metadataPageClient) Resource(schema.GroupVersionResource) metadata.Getter {
	return &metadataPageGetter{client: c}
}

type metadataPageGetter struct {
	client    *metadataPageClient
	namespace string
}

func (g *metadataPageGetter) Namespace(namespace string) metadata.ResourceInterface {
	getterCopy := *g
	getterCopy.namespace = namespace
	return &getterCopy
}

func (g *metadataPageGetter) List(ctx context.Context, options metav1.ListOptions) (*metav1.PartialObjectMetadataList, error) {
	g.client.mu.Lock()
	g.client.options = append(g.client.options, options)
	g.client.mu.Unlock()
	return g.client.list(ctx, options)
}

func (*metadataPageGetter) Delete(context.Context, string, metav1.DeleteOptions, ...string) error {
	return errors.New("metadata test client does not support delete")
}

func (*metadataPageGetter) DeleteCollection(context.Context, metav1.DeleteOptions, metav1.ListOptions) error {
	return errors.New("metadata test client does not support delete collection")
}

func (*metadataPageGetter) Get(context.Context, string, metav1.GetOptions, ...string) (*metav1.PartialObjectMetadata, error) {
	return nil, errors.New("metadata test client does not support get")
}

func (*metadataPageGetter) Watch(context.Context, metav1.ListOptions) (watch.Interface, error) {
	return nil, errors.New("metadata test client does not support watch")
}

func (*metadataPageGetter) Patch(context.Context, string, types.PatchType, []byte, metav1.PatchOptions, ...string) (*metav1.PartialObjectMetadata, error) {
	return nil, errors.New("metadata test client does not support patch")
}

func metadataPage(names ...string) *metav1.PartialObjectMetadataList {
	page := &metav1.PartialObjectMetadataList{}
	for _, name := range names {
		page.Items = append(page.Items, metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "payments", Labels: map[string]string{"kubernaut.ai/managed": "true"},
		}})
	}
	if len(names) == 2 {
		page.Continue = names[1]
		page.Items = page.Items[:1]
	}
	return page
}

func newBoundedReader(client metadata.Interface, cfg policyread.LocalInventoryConfig) *policyread.LocalInventoryReader {
	cfg.MetadataClient = client
	cfg.ScopeResolver = &localScopeStub{}
	cfg.Now = func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }
	cfg.Kinds = []policyread.LocalResourceKind{{
		Group: "apps", Version: "v1", Resource: "deployments", Kind: "Deployment", Namespaced: true,
	}}
	reader, err := policyread.NewLocalInventoryReader(cfg)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return reader
}
