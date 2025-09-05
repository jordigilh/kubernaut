package monitoring_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
)

var _ = Describe("AlertManagerClient", func() {
	var (
		client     *monitoring.AlertManagerClient
		mockServer *httptest.Server
		logger     *logrus.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)
		ctx = context.Background()
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	Describe("NewAlertManagerClient", func() {
		It("should create a new AlertManager client", func() {
			client := monitoring.NewAlertManagerClient("http://localhost:9093", 30*time.Second, logger)
			Expect(client).NotTo(BeNil())
		})

		It("should trim trailing slash from endpoint", func() {
			client := monitoring.NewAlertManagerClient("http://localhost:9093/", 30*time.Second, logger)
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("IsAlertResolved", func() {
		Context("when AlertManager returns no active alerts", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer GinkgoRecover()
					Expect(r.URL.Path).To(Equal("/api/v1/alerts"))
					filters := r.URL.Query()["filter"]
					Expect(filters).To(ContainElement(ContainSubstring("alertname=\"HighMemoryUsage\"")))
					Expect(filters).To(ContainElement(ContainSubstring("namespace=\"test-namespace\"")))

					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data:   []monitoring.AlertManagerAlert{},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return true (resolved) when no active alerts found", func() {
				since := time.Now().Add(-10 * time.Minute)
				resolved, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).NotTo(HaveOccurred())
				Expect(resolved).To(BeTrue())
			})
		})

		Context("when AlertManager returns active alerts", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					activeTime := time.Now().Add(-2 * time.Minute)
					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data: []monitoring.AlertManagerAlert{
							{
								Labels: map[string]string{
									"alertname": "HighMemoryUsage",
									"namespace": "test-namespace",
									"severity":  "warning",
								},
								State:    "active",
								ActiveAt: &activeTime,
								Value:    "0.95",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return false when alert is still active after reference time", func() {
				since := time.Now().Add(-5 * time.Minute)
				resolved, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).NotTo(HaveOccurred())
				Expect(resolved).To(BeFalse())
			})

			It("should return true when active alert started before reference time", func() {
				since := time.Now().Add(-1 * time.Minute)
				resolved, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).NotTo(HaveOccurred())
				Expect(resolved).To(BeTrue())
			})
		})

		Context("when AlertManager returns an error", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return an error", func() {
				since := time.Now().Add(-10 * time.Minute)
				_, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AlertManager API returned status 500"))
			})
		})
	})

	Describe("HasAlertRecurred", func() {
		Context("when no alerts recurred in time window", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return alert that started before the time window
					activeTime := time.Now().Add(-20 * time.Minute)
					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data: []monitoring.AlertManagerAlert{
							{
								Labels: map[string]string{
									"alertname": "HighMemoryUsage",
									"namespace": "test-namespace",
								},
								State:    "active",
								ActiveAt: &activeTime,
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return false when no recurrence detected", func() {
				from := time.Now().Add(-10 * time.Minute)
				to := time.Now()
				recurred, err := client.HasAlertRecurred(ctx, "HighMemoryUsage", "test-namespace", from, to)

				Expect(err).NotTo(HaveOccurred())
				Expect(recurred).To(BeFalse())
			})
		})

		Context("when alert recurred in time window", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return alert that started within the time window
					activeTime := time.Now().Add(-5 * time.Minute)
					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data: []monitoring.AlertManagerAlert{
							{
								Labels: map[string]string{
									"alertname": "HighMemoryUsage",
									"namespace": "test-namespace",
								},
								State:    "active",
								ActiveAt: &activeTime,
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return true when recurrence detected", func() {
				from := time.Now().Add(-10 * time.Minute)
				to := time.Now()
				recurred, err := client.HasAlertRecurred(ctx, "HighMemoryUsage", "test-namespace", from, to)

				Expect(err).NotTo(HaveOccurred())
				Expect(recurred).To(BeTrue())
			})
		})
	})

	Describe("GetAlertHistory", func() {
		Context("when AlertManager returns alert history", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					activeTime := time.Now().Add(-5 * time.Minute)
					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data: []monitoring.AlertManagerAlert{
							{
								Labels: map[string]string{
									"alertname": "HighMemoryUsage",
									"namespace": "test-namespace",
									"severity":  "warning",
								},
								Annotations: map[string]string{
									"description": "Memory usage is high",
								},
								State:    "active",
								ActiveAt: &activeTime,
								Value:    "0.95",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return alert events within time range", func() {
				from := time.Now().Add(-10 * time.Minute)
				to := time.Now()
				events, err := client.GetAlertHistory(ctx, "HighMemoryUsage", "test-namespace", from, to)

				Expect(err).NotTo(HaveOccurred())
				Expect(events).To(HaveLen(1))

				event := events[0]
				Expect(event.AlertName).To(Equal("HighMemoryUsage"))
				Expect(event.Namespace).To(Equal("test-namespace"))
				Expect(event.Severity).To(Equal("warning"))
				Expect(event.Status).To(Equal("firing"))
				Expect(event.Labels).To(HaveKeyWithValue("alertname", "HighMemoryUsage"))
				Expect(event.Annotations).To(HaveKeyWithValue("description", "Memory usage is high"))
			})
		})

		Context("when no alerts in time range", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return alert outside time range
					activeTime := time.Now().Add(-20 * time.Minute)
					response := monitoring.AlertManagerResponse{
						Status: "success",
						Data: []monitoring.AlertManagerAlert{
							{
								Labels: map[string]string{
									"alertname": "HighMemoryUsage",
									"namespace": "test-namespace",
								},
								State:    "active",
								ActiveAt: &activeTime,
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return empty event list", func() {
				from := time.Now().Add(-10 * time.Minute)
				to := time.Now()
				events, err := client.GetAlertHistory(ctx, "HighMemoryUsage", "test-namespace", from, to)

				Expect(err).NotTo(HaveOccurred())
				Expect(events).To(BeEmpty())
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("when AlertManager is healthy", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/-/healthy" {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("OK"))
					}
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return no error", func() {
				err := client.HealthCheck(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when AlertManager is unhealthy", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/-/healthy" {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should return an error", func() {
				err := client.HealthCheck(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("health check failed with status 503"))
			})
		})
	})

	Describe("Error Handling", func() {
		Context("when AlertManager is unreachable", func() {
			BeforeEach(func() {
				// Use invalid endpoint
				client = monitoring.NewAlertManagerClient("http://invalid-endpoint", 1*time.Second, logger)
			})

			It("should handle connection errors gracefully", func() {
				since := time.Now().Add(-10 * time.Minute)
				_, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute request"))
			})
		})

		Context("when AlertManager returns invalid JSON", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte("invalid json"))
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should handle JSON decode errors", func() {
				since := time.Now().Add(-10 * time.Minute)
				_, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to decode response"))
			})
		})

		Context("when AlertManager returns error status", func() {
			BeforeEach(func() {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := monitoring.AlertManagerResponse{
						Status: "error",
						Data:   []monitoring.AlertManagerAlert{},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(response)
				}))
				client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
			})

			It("should handle API error status", func() {
				since := time.Now().Add(-10 * time.Minute)
				_, err := client.IsAlertResolved(ctx, "HighMemoryUsage", "test-namespace", since)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AlertManager API returned status: error"))
			})
		})
	})

	Describe("Context Cancellation", func() {
		BeforeEach(func() {
			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Simulate slow response
				time.Sleep(100 * time.Millisecond)
				response := monitoring.AlertManagerResponse{
					Status: "success",
					Data:   []monitoring.AlertManagerAlert{},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			client = monitoring.NewAlertManagerClient(mockServer.URL, 30*time.Second, logger)
		})

		It("should respect context cancellation", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel() // Cancel immediately

			since := time.Now().Add(-10 * time.Minute)
			_, err := client.IsAlertResolved(cancelCtx, "HighMemoryUsage", "test-namespace", since)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})
