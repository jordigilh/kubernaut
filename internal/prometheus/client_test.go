package prometheus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

var _ = Describe("Prometheus Client", func() {

	Describe("GetAlerts", func() {
		It("UT-AF-T-001: returns firing alerts filtered by labels", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/alerts"))
				resp := promAlertsResponse([]promAlert{
					{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "severity": "critical"}, State: "firing"},
					{Labels: map[string]string{"alertname": "HighMem", "namespace": "staging", "severity": "high"}, State: "firing"},
				})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			alerts, err := client.GetAlerts(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(alerts).To(HaveLen(2))
			Expect(alerts[0].Labels["alertname"]).To(Equal("HighCPU"))
			Expect(alerts[0].State).To(Equal("firing"))
		})

		It("UT-AF-T-002: returns empty when no alerts match", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := promAlertsResponse([]promAlert{})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			alerts, err := client.GetAlerts(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(alerts).To(BeEmpty())
		})

		It("UT-AF-T-003: handles HTTP 500 from Prometheus", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("internal error"))
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.GetAlerts(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring(srv.URL))
		})

		It("UT-AF-T-004: respects context cancellation", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.GetAlerts(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetRules", func() {
		It("UT-AF-T-005: returns rule groups with state", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/rules"))
				resp := promRulesResponse([]promRuleGroup{
					{
						Name: "test-group",
						Rules: []promRule{
							{Alert: "HighCPU", Expr: "cpu_usage > 0.9", State: "pending", Labels: map[string]string{"severity": "critical"}},
							{Alert: "HighMem", Expr: "mem_usage > 0.8", State: "inactive", Labels: map[string]string{"severity": "high"}},
						},
					},
				})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			groups, err := client.GetRules(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(1))
			Expect(groups[0].Rules).To(HaveLen(2))
			Expect(groups[0].Rules[0].State).To(Equal("pending"))
			Expect(groups[0].Rules[1].State).To(Equal("inactive"))
		})

		It("UT-AF-T-006: handles malformed JSON response", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("{invalid json"))
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.GetRules(context.Background())
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("InstantQuery", func() {
		It("UT-AF-T-007: returns vector result for valid expression", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Path).To(Equal("/api/v1/query"))
				Expect(r.URL.Query().Get("query")).To(Equal("up{job=\"test\"}"))
				resp := promQueryResponse("vector", []promSample{
					{Metric: map[string]string{"job": "test"}, Value: "1"},
				})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			result, err := client.InstantQuery(context.Background(), `up{job="test"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Samples).To(HaveLen(1))
			Expect(result.Samples[0].Value).To(BeNumerically("==", 1))
		})

		It("UT-AF-T-008: returns empty result for no-data expression", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := promQueryResponse("vector", []promSample{})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			result, err := client.InstantQuery(context.Background(), `nonexistent_metric`)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Samples).To(BeEmpty())
		})

		It("UT-AF-T-009: handles Prometheus HTTP 422 (bad query)", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`))
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.InstantQuery(context.Background(), `invalid{`)
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-T-012: client timeout cancels request", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(2 * time.Second)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.InstantQuery(ctx, "up")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("TLS and Auth", func() {
		It("UT-AF-T-010: client with TLS CA connects to TLS server", func() {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				resp := promAlertsResponse([]promAlert{})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, srv.Client())
			alerts, err := client.GetAlerts(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(alerts).To(BeEmpty())
		})

		It("UT-AF-T-011: client with bearer token sends Authorization header", func() {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				resp := promAlertsResponse([]promAlert{})
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(resp)
			}))
			defer srv.Close()

			transport := &bearerTokenTransport{token: "test-sa-token", base: http.DefaultTransport}
			client := prom.NewHTTPClient(srv.URL, &http.Client{Transport: transport, Timeout: 5 * time.Second})
			_, err := client.GetAlerts(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(gotAuth).To(Equal("Bearer test-sa-token"))
		})
	})

	Describe("Error redaction", func() {
		It("UT-AF-T-022: Prometheus error is redacted (no internal URLs)", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("connection to http://prometheus.monitoring:9090 failed"))
			}))
			defer srv.Close()

			client := prom.NewHTTPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
			_, err := client.GetAlerts(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("prometheus.monitoring"))
			Expect(err.Error()).NotTo(ContainSubstring("9090"))
		})
	})
})

// --- Test helpers ---

type bearerTokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

type promAlert struct {
	Labels map[string]string `json:"labels"`
	State  string            `json:"state"`
}

func promAlertsResponse(alerts []promAlert) []byte {
	data := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"alerts": alerts,
		},
	}
	b, _ := json.Marshal(data)
	return b
}

type promRule struct {
	Alert  string            `json:"alert"`
	Expr   string            `json:"expr"`
	State  string            `json:"state"`
	Labels map[string]string `json:"labels"`
}

type promRuleGroup struct {
	Name  string     `json:"name"`
	Rules []promRule `json:"rules"`
}

func promRulesResponse(groups []promRuleGroup) []byte {
	data := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"groups": groups,
		},
	}
	b, _ := json.Marshal(data)
	return b
}

type promSample struct {
	Metric map[string]string `json:"metric"`
	Value  string
}

func promQueryResponse(resultType string, samples []promSample) []byte {
	result := make([]interface{}, len(samples))
	for i, s := range samples {
		result[i] = map[string]interface{}{
			"metric": s.Metric,
			"value":  []interface{}{float64(time.Now().Unix()), s.Value},
		}
	}
	data := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": resultType,
			"result":     result,
		},
	}
	b, _ := json.Marshal(data)
	return b
}

// Verify unused import suppression
var _ = fmt.Sprintf
