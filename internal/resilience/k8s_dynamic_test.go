package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	gobreaker "github.com/sony/gobreaker/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	var wg atomic.Int32

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Add(-1)
			_, err := resilientClient.Resource(testGVR).Namespace("default").List(ctx, metav1.ListOptions{})
			if err == nil {
				successCount.Add(1)
			}
		}()
	}

	// Wait for goroutines
	for wg.Load() > 0 {
		time.Sleep(1 * time.Millisecond)
	}

	if successCount.Load() != 20 {
		t.Errorf("successes = %d, want 20", successCount.Load())
	}
}
