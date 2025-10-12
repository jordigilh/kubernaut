package toolset_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/toolset/server"
)

var _ = Describe("BR-TOOLSET-033: HTTP Server", func() {
	var (
		srv          *server.Server
		fakeClient   *fake.Clientset
		ctx          context.Context
		cancelFunc   context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancelFunc = context.WithCancel(context.Background())
		fakeClient = fake.NewSimpleClientset()

		// Create server with test configuration
		config := &server.Config{
			Port:              8080,
			MetricsPort:       9090,
			ShutdownTimeout:   5 * time.Second,
			DiscoveryInterval: 5 * time.Minute,
		}

		var err error
		srv, err = server.NewServer(config, fakeClient)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if cancelFunc != nil {
			cancelFunc()
		}
		if srv != nil {
			_ = srv.Shutdown(context.Background())
		}
	})

	Describe("Health Endpoints", func() {
		Context("GET /health", func() {
			It("should return 200 OK without authentication", func() {
				req := httptest.NewRequest("GET", "/health", nil)
				// No Authorization header - public endpoint

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Body.String()).To(ContainSubstring("ok"))
			})

			It("should include service status", func() {
				req := httptest.NewRequest("GET", "/health", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).ToNot(HaveOccurred())

				Expect(response["status"]).To(Equal("ok"))
			})
		})

		Context("GET /ready", func() {
			It("should return 200 OK without authentication", func() {
				req := httptest.NewRequest("GET", "/ready", nil)
				// No Authorization header - public endpoint

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
			})

			It("should check dependencies readiness", func() {
				req := httptest.NewRequest("GET", "/ready", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				var response map[string]interface{}
				json.Unmarshal(rr.Body.Bytes(), &response)

				Expect(response).To(HaveKey("kubernetes"))
			})
		})
	})

	Describe("BR-TOOLSET-034: Protected API Endpoints", func() {
		Context("GET /api/v1/toolset", func() {
			It("should require authentication", func() {
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				// No Authorization header

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return current toolset JSON with valid auth", func() {
				// Setup authenticated request
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/json"))
			})

			It("should handle empty toolset", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(rr.Body.Bytes(), &response)

				Expect(response).To(HaveKey("tools"))
			})
		})

		Context("GET /api/v1/services", func() {
			It("should require authentication", func() {
				req := httptest.NewRequest("GET", "/api/v1/services", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return discovered services with valid auth", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				// Create test services
				promSvc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "prometheus",
						Namespace: "monitoring",
						Labels:    map[string]string{"app": "prometheus"},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{Name: "web", Port: 9090}},
					},
				}
				fakeClient.CoreV1().Services("monitoring").Create(ctx, promSvc, metav1.CreateOptions{})

				req := httptest.NewRequest("GET", "/api/v1/services", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				json.Unmarshal(rr.Body.Bytes(), &response)

				Expect(response).To(HaveKey("services"))
			})

			It("should filter by service type query parameter", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("GET", "/api/v1/services?type=prometheus", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
			})
		})

		Context("POST /api/v1/discover", func() {
			It("should require authentication", func() {
				req := httptest.NewRequest("POST", "/api/v1/discover", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should trigger discovery with valid auth", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("POST", "/api/v1/discover", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusAccepted))

				var response map[string]interface{}
				json.Unmarshal(rr.Body.Bytes(), &response)

				Expect(response["message"]).To(ContainSubstring("Discovery triggered"))
			})
		})

		Context("GET /api/v1/services/:name", func() {
			It("should require authentication", func() {
				req := httptest.NewRequest("GET", "/api/v1/services/prometheus", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return service details with valid auth", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("GET", "/api/v1/services/prometheus", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				// Should return 404 if not found (expected for empty cluster)
				Expect(rr.Code).To(BeElementOf(http.StatusOK, http.StatusNotFound))
			})
		})
	})

	Describe("Metrics Endpoint", func() {
		Context("GET /metrics", func() {
			It("should require authentication", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return Prometheus metrics with valid auth", func() {
				fakeClient.PrependReactor("create", "tokenreviews", authenticatedTokenReactor())

				req := httptest.NewRequest("GET", "/metrics", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				srv.ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("text/plain"))
			})
		})
	})

	Describe("Server Lifecycle", func() {
		It("should start and shutdown gracefully", func() {
			config := &server.Config{
				Port:              8081, // Different port to avoid conflicts
				MetricsPort:       9091,
				ShutdownTimeout:   1 * time.Second,
				DiscoveryInterval: 5 * time.Minute,
			}

			testSrv, err := server.NewServer(config, fakeClient)
			Expect(err).ToNot(HaveOccurred())

			// Start server in background
			go func() {
				_ = testSrv.Start(ctx)
			}()

			// Give it time to start
			time.Sleep(100 * time.Millisecond)

			// Shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			err = testSrv.Shutdown(shutdownCtx)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

// Helper function to create authenticated TokenReview reactor
func authenticatedTokenReactor() func(action k8stesting.Action) (bool, runtime.Object, error) {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		tokenReview := createAction.GetObject().(*authenticationv1.TokenReview)

		tokenReview.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: "system:serviceaccount:kubernaut:dynamic-toolset-sa",
				UID:      "test-uid",
			},
		}
		return true, tokenReview, nil
	}
}

