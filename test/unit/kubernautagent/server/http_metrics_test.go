package server_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
)

var _ = Describe("HTTP Metrics Middleware — BR-KA-OBSERVABILITY-001.5", func() {

	Describe("UT-KA-OBS-017: Nil metrics passes through without panic", func() {
		It("serves request normally when metrics is nil", func() {
			handler := kaserver.HTTPMetricsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/v1/incident/session/abc/result", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("UT-KA-OBS-018: normalizeEndpoint collapses session IDs", func() {
		It("records normalized endpoint in histogram", func() {
			reg := prometheus.NewRegistry()
			m := kametrics.NewMetricsWithRegistry(reg)

			handler := kaserver.HTTPMetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/v1/incident/session/abc-123-def/result", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			for _, f := range families {
				if f.GetName() == kametrics.MetricNameHTTPRequestDurationSeconds {
					Expect(f.GetMetric()).NotTo(BeEmpty())
					for _, metric := range f.GetMetric() {
						for _, lp := range metric.GetLabel() {
							if lp.GetName() == "endpoint" {
								Expect(lp.GetValue()).To(ContainSubstring("{id}"),
									"session ID should be normalized to {id}")
							}
						}
					}
				}
			}
		})
	})

	Describe("UT-KA-OBS-019: /stream path excluded from duration histogram", func() {
		It("does not observe duration for stream requests", func() {
			reg := prometheus.NewRegistry()
			m := kametrics.NewMetricsWithRegistry(reg)

			handler := kaserver.HTTPMetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/v1/incident/session/abc/stream", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			for _, f := range families {
				if f.GetName() == kametrics.MetricNameHTTPRequestDurationSeconds {
					Expect(f.GetMetric()).To(BeEmpty(),
						"stream requests should not generate histogram observations")
				}
			}
		})
	})

	Describe("UT-KA-OBS-020: In-flight gauge increments and decrements", func() {
		It("gauge is 0 after request completes", func() {
			reg := prometheus.NewRegistry()
			m := kametrics.NewMetricsWithRegistry(reg)

			handler := kaserver.HTTPMetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			families, err := reg.Gather()
			Expect(err).NotTo(HaveOccurred())
			for _, f := range families {
				if f.GetName() == kametrics.MetricNameHTTPRequestsInFlight {
					for _, metric := range f.GetMetric() {
						Expect(metric.GetGauge().GetValue()).To(BeNumerically("==", 0))
					}
				}
			}
		})
	})
})
