package registry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// Only the Kubernetes HTTP boundary is controlled; the dynamic client, registry,
// prober and Gate run unchanged. A flushed WATCH intentionally sends no events.
type registryAPIFixture struct {
	mu       sync.Mutex
	items    []unstructured.Unstructured
	streams  []chan struct{}
	events   []chan []byte
	denied   atomic.Bool
	lists    atomic.Int32
	watches  atomic.Int32
	active   atomic.Int32
	listKind string
	version  string
}

func (f *registryAPIFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("labelSelector") != registry.ManagedLabel+"=true" {
		http.Error(w, "selector required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Query().Get("watch") != "true" {
		f.lists.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		list := unstructured.UnstructuredList{Items: append([]unstructured.Unstructured{}, f.items...)}
		list.SetAPIVersion(f.version)
		list.SetKind(f.listKind)
		list.SetResourceVersion("10")
		if err := json.NewEncoder(w).Encode(&list); err != nil {
			return // client cancellation is observable at the caller
		}
		return
	}
	f.watches.Add(1)
	if f.denied.Load() {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: 403})
		return
	}
	if r.URL.Query().Get("resourceVersion") != "10" {
		http.Error(w, "matching list resourceVersion required", http.StatusBadRequest)
		return
	}
	closed := make(chan struct{})
	events := make(chan []byte, 1)
	f.mu.Lock()
	f.streams = append(f.streams, closed)
	f.events = append(f.events, events)
	f.mu.Unlock()
	f.active.Add(1)
	defer f.active.Add(-1)
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case data := <-events:
			if _, err := w.Write(data); err != nil {
				return
			}
			w.(http.Flusher).Flush()
		}
	}
}

func (f *registryAPIFixture) disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, stream := range f.streams {
		close(stream)
	}
	f.streams = nil
	f.events = nil
}

func (f *registryAPIFixture) send(data string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	Expect(f.events).NotTo(BeEmpty())
	f.events[len(f.events)-1] <- []byte(data + "\n")
}

var _ = Describe("Owned registry WATCH [BR-INTEGRATION-065, BR-SCOPE-001]", func() {
	It("W04 IT-FMC-WATCH-2637-002: a live MODIFIED event retires a disabled Kuadrant registration", func() {
		f := &registryAPIFixture{listKind: "MCPServerRegistrationList", version: "mcp.kuadrant.io/v1"}
		obj := unstructured.Unstructured{}
		obj.SetAPIVersion(f.version)
		obj.SetKind("MCPServerRegistration")
		obj.SetName("east")
		obj.SetNamespace("registrations")
		obj.SetUID("east-original")
		obj.SetResourceVersion("10")
		obj.SetLabels(map[string]string{registry.ManagedLabel: "true"})
		f.items = []unstructured.Unstructured{obj}
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
		Expect(err).NotTo(HaveOccurred())
		reg, err := registry.NewClusterRegistry(registry.GatewayKuadrant, client, registry.RegistryConfig{Namespace: "registrations"}, nil, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		DeferCleanup(reg.Stop)
		Expect(reg.Start(ctx)).To(Succeed())
		guard := reg.(registry.CurrentRegistrationGuard)
		old, found := guard.Snapshot("east")
		Expect(found).To(BeTrue())
		obj.SetResourceVersion("11")
		Expect(unstructured.SetNestedField(obj.Object, "Disabled", "spec", "state")).To(Succeed())
		data, err := json.Marshal(map[string]any{"type": "MODIFIED", "object": obj.Object})
		Expect(err).NotTo(HaveOccurred())
		f.send(string(data))
		Eventually(reg.List).Should(BeEmpty(), "a disabled registration cannot remain authorized until stream rotation")
		Expect(reg.Ready()).To(BeTrue(), "a valid membership update is not a stream failure")
		Expect(guard.WithCurrent(ctx, "east", old.Info.RegistrationUID, old.Revision, func(registry.ClusterInfo) error { return nil })).NotTo(Succeed())
	})

	DescribeTable("W01/W02/W04/W06/W07 IT-FMC-WATCH-2637-001/002/004: readiness requires WATCH, automatically repairs a missed deletion, and joins quiet streams", func(kuadrant bool) {
		f := &registryAPIFixture{listKind: "BackendList", version: "gateway.envoyproxy.io/v1alpha1"}
		if kuadrant {
			f.listKind, f.version = "MCPServerRegistrationList", "mcp.kuadrant.io/v1"
		}
		obj := unstructured.Unstructured{}
		obj.SetAPIVersion(f.version)
		obj.SetKind("Backend")
		if kuadrant {
			obj.SetKind("MCPServerRegistration")
		}
		obj.SetName("east")
		obj.SetNamespace("registrations")
		obj.SetUID("east-original")
		obj.SetResourceVersion("10")
		obj.SetLabels(map[string]string{registry.ManagedLabel: "true"})
		f.items = []unstructured.Unstructured{obj}
		f.denied.Store(true)
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
		Expect(err).NotTo(HaveOccurred())
		gateway := registry.GatewayEAIGW
		if kuadrant {
			gateway = registry.GatewayKuadrant
		}
		reg, err := registry.NewClusterRegistry(gateway, client, registry.RegistryConfig{Namespace: "registrations"}, nil, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		DeferCleanup(reg.Stop)
		started := make(chan error, 1)
		go func() { started <- reg.Start(ctx) }()
		gate := readiness.NewGate(10*time.Millisecond, logr.Discard(), &readiness.ClusterRegistryProber{Registry: reg})
		gate.Start(ctx)
		DeferCleanup(gate.Stop)
		Eventually(f.watches.Load).WithTimeout(3 * time.Second).Should(BeNumerically(">=", 1))
		Consistently(started).WithTimeout(150*time.Millisecond).ShouldNot(Receive(), "LIST is not initial WATCH success")
		Expect(reg.Ready()).To(BeFalse())
		Expect(gate.Ready()).To(BeFalse())
		f.denied.Store(false)
		Eventually(started).WithTimeout(5 * time.Second).Should(Receive(Succeed()))
		Eventually(gate.Ready).Should(BeTrue(), "quiet WATCH must recover without events/bookmarks or manual Probe")
		guard := reg.(registry.CurrentRegistrationGuard)
		old, found := guard.Snapshot("east")
		Expect(found).To(BeTrue())
		// Reconnecting a quiet stream must not manufacture a registration
		// metadata change or invalidate an otherwise unchanged proof revision.
		f.denied.Store(true)
		f.disconnect()
		Eventually(reg.Ready).Should(BeFalse())
		Eventually(gate.Ready).Should(BeFalse())
		f.denied.Store(false)
		Eventually(gate.Ready).WithTimeout(5 * time.Second).Should(BeTrue())
		unchanged, found := guard.Snapshot("east")
		Expect(found).To(BeTrue())
		Expect(unchanged.Revision).To(Equal(old.Revision))
		Expect(guard.WithCurrent(ctx, "east", old.Info.RegistrationUID, old.Revision, func(registry.ClusterInfo) error { return nil })).To(Succeed())
		f.denied.Store(true)
		f.disconnect()
		Eventually(reg.Ready).Should(BeFalse())
		Eventually(gate.Ready).Should(BeFalse())
		Expect(guard.WithCurrent(ctx, "east", old.Info.RegistrationUID, old.Revision, func(registry.ClusterInfo) error { return nil })).NotTo(Succeed())
		f.mu.Lock()
		f.items = nil // deletion happened during the lost interval
		f.mu.Unlock()
		Eventually(f.lists.Load).WithTimeout(5 * time.Second).Should(BeNumerically(">=", 3))
		Expect(gate.Ready()).To(BeFalse(), "a successful recovery LIST cannot mask denied WATCH")
		f.denied.Store(false)
		Eventually(gate.Ready).WithTimeout(8 * time.Second).Should(BeTrue())
		Expect(reg.List()).To(BeEmpty(), "fresh complete LIST retires the missed deletion")
		reg.Stop()
		Eventually(f.active.Load).Should(BeZero())
		attempts := f.watches.Load()
		Consistently(f.watches.Load).WithTimeout(1200 * time.Millisecond).Should(Equal(attempts))
	}, Entry("EAIGW", false), Entry("Kuadrant", true))

	DescribeTable("W03/W04 IT-FMC-WATCH-2637-001/002: observed stream failures retire readiness before same-name recreation recovery", func(kuadrant bool, failure string) {
		f := &registryAPIFixture{listKind: "BackendList", version: "gateway.envoyproxy.io/v1alpha1"}
		gateway := registry.GatewayEAIGW
		if kuadrant {
			gateway, f.listKind, f.version = registry.GatewayKuadrant, "MCPServerRegistrationList", "mcp.kuadrant.io/v1"
		}
		obj := unstructured.Unstructured{}
		obj.SetAPIVersion(f.version)
		obj.SetKind("Backend")
		if kuadrant {
			obj.SetKind("MCPServerRegistration")
		}
		obj.SetName("east")
		obj.SetNamespace("registrations")
		obj.SetUID("old")
		obj.SetResourceVersion("10")
		obj.SetLabels(map[string]string{registry.ManagedLabel: "true"})
		f.items = []unstructured.Unstructured{obj}
		server := httptest.NewServer(http.HandlerFunc(f.serve))
		DeferCleanup(server.Close)
		client, err := dynamic.NewForConfig(&rest.Config{Host: server.URL})
		Expect(err).NotTo(HaveOccurred())
		reg, err := registry.NewClusterRegistry(gateway, client, registry.RegistryConfig{Namespace: "registrations"}, nil, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		DeferCleanup(cancel)
		DeferCleanup(reg.Stop)
		Expect(reg.Start(ctx)).To(Succeed())
		guard := reg.(registry.CurrentRegistrationGuard)
		old, found := guard.Snapshot("east")
		Expect(found).To(BeTrue())
		f.denied.Store(true)
		f.send(failure)
		Eventually(reg.Ready).Should(BeFalse())
		Expect(guard.WithCurrent(ctx, "east", old.Info.RegistrationUID, old.Revision, func(registry.ClusterInfo) error { return nil })).NotTo(Succeed())
		f.mu.Lock()
		f.items[0].SetUID("recreated")
		f.mu.Unlock()
		f.denied.Store(false)
		Eventually(reg.Ready).WithTimeout(5 * time.Second).Should(BeTrue())
		current, found := guard.Snapshot("east")
		Expect(found).To(BeTrue())
		Expect(current.Info.RegistrationUID).To(Equal("recreated"))
		Expect(current.Revision).To(BeNumerically(">", old.Revision))
		Expect(guard.WithCurrent(ctx, "east", old.Info.RegistrationUID, old.Revision, func(registry.ClusterInfo) error { return nil })).NotTo(Succeed())
	},
		Entry("EAIGW expired", false, `{"type":"ERROR","object":{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Expired","code":410}}`),
		Entry("Kuadrant expired", true, `{"type":"ERROR","object":{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Expired","code":410}}`),
		Entry("EAIGW decode failure", false, `not-json`),
		Entry("Kuadrant decode failure", true, `not-json`),
	)
})
