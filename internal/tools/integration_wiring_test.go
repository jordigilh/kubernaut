package tools_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/resilience"
	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("Full Wiring Integration", func() {
	It("IT-AF-038-090: tool handler → ResilientDynamicClient → CB opens on failures → Healthy() false", func() {
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_wiring_cb_state",
		}, []string{"dependency"})

		cb := resilience.NewK8sCircuitBreaker(resilience.K8sCBConfig{
			Name:             "it-wiring-k8s",
			MaxRequests:      1,
			Interval:         10 * time.Second,
			Timeout:          50 * time.Millisecond,
			FailureThreshold: 2,
			StateGauge:       gauge,
			DependencyName:   "k8s",
		})

		scheme := runtime.NewScheme()
		rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
		fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				rrGVR: "RemediationRequestList",
			},
		)

		fakeClient.PrependReactor("list", "remediationrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("simulated API server error")
		})

		resilientClient := resilience.NewResilientDynamicClient(fakeClient, cb)
		ctx := context.Background()

		Expect(cb.Healthy()).To(BeTrue(), "CB starts closed")

		for i := 0; i < 3; i++ {
			_, _ = tools.HandleListRemediations(ctx, resilientClient, tools.ListRemediationsArgs{Namespace: "test"})
		}

		Expect(cb.Healthy()).To(BeFalse(), "CB should be open after repeated failures")

		var m dto.Metric
		g, err := gauge.GetMetricWithLabelValues("k8s")
		Expect(err).NotTo(HaveOccurred())
		Expect(g.Write(&m)).To(Succeed())
		Expect(m.GetGauge().GetValue()).To(BeNumerically(">", 0), "gauge should reflect open state")
	})

	It("IT-AF-038-091: tool handler succeeds through ResilientDynamicClient when K8s API healthy", func() {
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "test_wiring_cb_state_ok",
		}, []string{"dependency"})

		cb := resilience.NewK8sCircuitBreaker(resilience.K8sCBConfig{
			Name:             "it-wiring-k8s-ok",
			MaxRequests:      1,
			Interval:         10 * time.Second,
			Timeout:          50 * time.Millisecond,
			FailureThreshold: 5,
			StateGauge:       gauge,
			DependencyName:   "k8s",
		})

		fakeClient := newDynamicFakeClient(newFakeRR("default", "rr-1", "Executing"))
		resilientClient := resilience.NewResilientDynamicClient(fakeClient, cb)

		ctx := context.Background()
		result, err := tools.HandleListRemediations(ctx, resilientClient, tools.ListRemediationsArgs{Namespace: "default"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Remediations[0].Name).To(Equal("rr-1"))
		Expect(cb.Healthy()).To(BeTrue())
	})
})
