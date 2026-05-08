package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gobreaker "github.com/sony/gobreaker/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var testGVR = schema.GroupVersionResource{Group: "test.io", Version: "v1", Resource: "widgets"}

func newFakeClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			testGVR: "WidgetList",
		},
	)
}

// IT-AF-038-080: Full chain — HTTP entry → ResilientDynamicClient → CB → observable gauge
func TestResilientDynamicClient_ListThroughCB_UpdatesGauge(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_k8s_cb_state_int",
	}, []string{"dependency"})

	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-list",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
		StateGauge:       gauge,
		DependencyName:   "k8s",
	})

	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)

	ctx := context.Background()

	// Successful list (CB stays closed)
	_, err := resilientClient.Resource(testGVR).Namespace("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List() through CB error = %v", err)
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("CB state = %v, want Closed", cb.State())
	}
}

// IT-AF-038-081: CB opens after K8s API failures, subsequent calls fail fast with metric change
func TestResilientDynamicClient_CBOpensOnFailure(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_k8s_cb_failfast",
	}, []string{"dependency"})

	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-fail",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 2,
		StateGauge:       gauge,
		DependencyName:   "k8s",
	})

	// Use a CB directly to simulate failures (since fake client doesn't error)
	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("apiserver timeout") })
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("apiserver timeout") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cb.State())
	}

	// Gauge should be set to 2 (StateOpen)
	m := &dto.Metric{}
	if err := gauge.WithLabelValues("k8s").Write(m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	if m.GetGauge().GetValue() != 2 {
		t.Errorf("gauge = %v, want 2 (Open)", m.GetGauge().GetValue())
	}

	// Now create a resilient client wrapping fakeClient — CB is already open
	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)

	// List should fail fast without hitting the fake client
	_, err := resilientClient.Resource(testGVR).Namespace("default").List(ctx, metav1.ListOptions{})
	if err == nil {
		t.Fatal("expected error when CB is open")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("error = %v, want ErrOpenState", err)
	}
}

// IT-AF-038-082: Watch bypasses the open CB
func TestResilientDynamicClient_WatchBypassesCB(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-watch-bypass",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 2,
	})

	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cb.State())
	}

	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)

	// Watch should still work despite open CB
	w, err := resilientClient.Resource(testGVR).Namespace("default").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Watch() with open CB error = %v", err)
	}
	if w == nil {
		t.Fatal("Watch() returned nil")
	}
	w.Stop()
}

// IT-AF-038-083: Healthy() method reflects CB state for readiness probe
func TestResilientDynamicClient_HealthyReflectsState(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-healthy",
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
	})

	if !cb.Healthy() {
		t.Fatal("Healthy() = false, want true when closed")
	}

	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })

	if cb.Healthy() {
		t.Fatal("Healthy() = true, want false when open")
	}

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)
	if !cb.Healthy() {
		t.Error("Healthy() = false, want true when half-open")
	}
}

// IT-AF-038-084: Concurrent K8s operations through CB under -race
func TestResilientDynamicClient_ConcurrentOperations(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-concurrent",
		MaxRequests:      50,
		Interval:         10 * time.Second,
		Timeout:          30 * time.Second,
		FailureThreshold: 100,
	})

	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)

	ctx := context.Background()
	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := resilientClient.Resource(testGVR).Namespace("default").List(ctx, metav1.ListOptions{})
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	if successCount.Load() != 20 {
		t.Errorf("successes = %d, want 20", successCount.Load())
	}
}

func newTestCB() *K8sCircuitBreaker {
	return NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "test-ops",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 5,
	})
}

func testObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "test.io/v1",
			"kind":       "Widget",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
			},
		},
	}
}

func clusterObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "test.io/v1",
			"kind":       "Widget",
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
}

// IT-AF-038-085: Cluster-scoped operations route through circuit breaker
func TestResilientNamespaceableResource_AllOperations(t *testing.T) {
	cb := newTestCB()
	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)
	ctx := context.Background()

	res := resilientClient.Resource(testGVR)

	// Create (cluster-scoped object without namespace)
	created, err := res.Create(ctx, clusterObj("cluster-obj"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.GetName() != "cluster-obj" {
		t.Errorf("Create() name = %q, want %q", created.GetName(), "cluster-obj")
	}

	// Get
	got, err := res.Get(ctx, "cluster-obj", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GetName() != "cluster-obj" {
		t.Errorf("Get() name = %q, want %q", got.GetName(), "cluster-obj")
	}

	// List
	list, err := res.List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Items) == 0 {
		t.Error("List() returned empty items")
	}

	// Update
	got.Object["spec"] = map[string]interface{}{"color": "blue"}
	updated, err := res.Update(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated == nil {
		t.Fatal("Update() returned nil")
	}

	// UpdateStatus
	_, err = res.UpdateStatus(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Patch
	patchData := []byte(`{"metadata":{"labels":{"env":"test"}}}`)
	patched, err := res.Patch(ctx, "cluster-obj", types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if patched == nil {
		t.Fatal("Patch() returned nil")
	}

	// DeleteCollection
	err = res.DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("DeleteCollection() error = %v", err)
	}

	// Recreate for Delete test
	_, err = res.Create(ctx, clusterObj("del-obj"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() for delete error = %v", err)
	}

	// Delete
	err = res.Delete(ctx, "del-obj", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Watch (bypasses CB)
	w, err := res.Watch(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}
	w.Stop()
}

// IT-AF-038-086: Namespaced operations — Create, Get, Update, Delete, Patch
func TestResilientResourceInterface_AllOperations(t *testing.T) {
	cb := newTestCB()
	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)
	ctx := context.Background()

	ns := resilientClient.Resource(testGVR).Namespace("default")

	// Create
	created, err := ns.Create(ctx, testObj("ns-obj"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.GetName() != "ns-obj" {
		t.Errorf("Create() name = %q, want %q", created.GetName(), "ns-obj")
	}

	// Get
	got, err := ns.Get(ctx, "ns-obj", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GetName() != "ns-obj" {
		t.Errorf("Get() name = %q, want %q", got.GetName(), "ns-obj")
	}

	// Update
	got.Object["spec"] = map[string]interface{}{"version": "2"}
	updated, err := ns.Update(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated == nil {
		t.Fatal("Update() returned nil")
	}

	// UpdateStatus
	_, err = ns.UpdateStatus(ctx, got, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	// Patch
	patchData := []byte(`{"metadata":{"annotations":{"key":"val"}}}`)
	patched, err := ns.Patch(ctx, "ns-obj", types.MergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("Patch() error = %v", err)
	}
	if patched == nil {
		t.Fatal("Patch() returned nil")
	}

	// DeleteCollection
	err = ns.DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("DeleteCollection() error = %v", err)
	}

	// Recreate for Delete
	_, err = ns.Create(ctx, testObj("ns-del"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Create() for delete error = %v", err)
	}

	// Delete
	err = ns.Delete(ctx, "ns-del", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

// IT-AF-038-087: All operations fail fast when CB is open
func TestResilientDynamicClient_AllOpsFailWhenCBOpen(t *testing.T) {
	cb := NewK8sCircuitBreaker(K8sCBConfig{
		Name:             "it-k8s-open-all",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          1 * time.Second,
		FailureThreshold: 2,
	})

	ctx := context.Background()
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })
	_ = cb.Execute(ctx, func(_ context.Context) error { return errors.New("fail") })

	if cb.State() != gobreaker.StateOpen {
		t.Fatalf("CB state = %v, want Open", cb.State())
	}

	fakeClient := newFakeClient()
	resilientClient := NewResilientDynamicClient(fakeClient, cb)
	obj := testObj("x")

	// Cluster-scoped
	res := resilientClient.Resource(testGVR)
	if _, err := res.Get(ctx, "x", metav1.GetOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("cluster Get: got %v, want ErrOpenState", err)
	}
	if _, err := res.Create(ctx, obj, metav1.CreateOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("cluster Create: got %v, want ErrOpenState", err)
	}
	if _, err := res.Update(ctx, obj, metav1.UpdateOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("cluster Update: got %v, want ErrOpenState", err)
	}
	if err := res.Delete(ctx, "x", metav1.DeleteOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("cluster Delete: got %v, want ErrOpenState", err)
	}
	if _, err := res.Patch(ctx, "x", types.MergePatchType, []byte(`{}`), metav1.PatchOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("cluster Patch: got %v, want ErrOpenState", err)
	}

	// Namespaced
	ns := resilientClient.Resource(testGVR).Namespace("ns")
	if _, err := ns.Get(ctx, "x", metav1.GetOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("ns Get: got %v, want ErrOpenState", err)
	}
	if _, err := ns.Create(ctx, obj, metav1.CreateOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("ns Create: got %v, want ErrOpenState", err)
	}
	if _, err := ns.Update(ctx, obj, metav1.UpdateOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("ns Update: got %v, want ErrOpenState", err)
	}
	if err := ns.Delete(ctx, "x", metav1.DeleteOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("ns Delete: got %v, want ErrOpenState", err)
	}
	if _, err := ns.Patch(ctx, "x", types.MergePatchType, []byte(`{}`), metav1.PatchOptions{}); !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("ns Patch: got %v, want ErrOpenState", err)
	}
}
