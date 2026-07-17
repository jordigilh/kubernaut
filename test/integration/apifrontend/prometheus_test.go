package apifrontend_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	afprom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
)

var _ = Describe("Prometheus Client Integration (prometheus/)", func() {

	Describe("AC-33: Prometheus client queries alerts", func() {
		It("IT-AF-1195-048: GetAlerts returns parsed alert list", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "success",
					"data": map[string]any{
						"alerts": []map[string]any{
							{
								"labels":      map[string]string{"alertname": "TestAlert", "namespace": defaultFixture},
								"annotations": map[string]string{"summary": "test alert"},
								"state":       "firing",
								"activeAt":    "2025-01-01T00:00:00Z",
							},
						},
					},
				})
			}))
			defer backend.Close()

			client := afprom.NewHTTPClient(backend.URL, http.DefaultClient)
			alerts, err := client.GetAlerts(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(alerts).To(HaveLen(1))
			Expect(alerts[0].Labels["alertname"]).To(Equal("TestAlert"))
			Expect(alerts[0].State).To(Equal("firing"))
		})

		It("IT-AF-1195-049: GetAlerts handles Prometheus error response", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer backend.Close()

			client := afprom.NewHTTPClient(backend.URL, http.DefaultClient)
			_, err := client.GetAlerts(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AC-34: Prometheus client queries rules", func() {
		It("IT-AF-1195-050: GetRules returns parsed rule groups", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "success",
					"data": map[string]any{
						"groups": []map[string]any{
							{
								"name": "test-group",
								"file": "test.yaml",
								"rules": []map[string]any{
									{
										"alert":    "TestRule",
										"expr":     "up == 0",
										"duration": 60.0,
										"state":    "firing",
										"type":     "alerting",
										"labels":   map[string]string{"severity": "critical"},
									},
								},
							},
						},
					},
				})
			}))
			defer backend.Close()

			client := afprom.NewHTTPClient(backend.URL, http.DefaultClient)
			groups, err := client.GetRules(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(1))
			Expect(groups[0].Name).To(Equal("test-group"))
			Expect(groups[0].Rules).To(HaveLen(1))
			Expect(groups[0].Rules[0].Name).To(Equal("TestRule"))
		})
	})

	Describe("AC-35: Prometheus instant query", func() {
		It("IT-AF-1195-051: InstantQuery parses vector result", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status": "success",
					"data": map[string]any{
						"resultType": "vector",
						"result": []map[string]any{
							{
								"metric": map[string]string{"__name__": "up", "instance": "localhost:9090"},
								"value":  []any{1609459200.0, "1"},
							},
						},
					},
				})
			}))
			defer backend.Close()

			client := afprom.NewHTTPClient(backend.URL, http.DefaultClient)
			result, err := client.InstantQuery(context.Background(), "up")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Samples).To(HaveLen(1))
			Expect(result.Samples[0].Value).To(Equal(1.0))
		})

		It("IT-AF-1195-052: InstantQuery handles invalid PromQL gracefully", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":    "error",
					"errorType": "bad_data",
					"error":     "invalid expression",
				})
			}))
			defer backend.Close()

			client := afprom.NewHTTPClient(backend.URL, http.DefaultClient)
			_, err := client.InstantQuery(context.Background(), "invalid{{{")
			Expect(err).To(HaveOccurred())
		})
	})
})
