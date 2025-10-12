package toolset_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/toolset/server/middleware"
)

var _ = Describe("BR-TOOLSET-032: Authentication Middleware", func() {
	var (
		authMW     *middleware.AuthMiddleware
		fakeClient *fake.Clientset
		handler    http.Handler
		testServer *httptest.Server
	)

	BeforeEach(func() {
		fakeClient = fake.NewSimpleClientset()
		authMW = middleware.NewAuthMiddleware(fakeClient)

		// Create test handler that auth middleware will wrap
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("authenticated"))
		})
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	Describe("Middleware", func() {
		Context("with valid Bearer token", func() {
			It("should allow request through", func() {
				// Setup fake TokenReview response (authenticated)
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					createAction := action.(k8stesting.CreateAction)
					tokenReview := createAction.GetObject().(*authenticationv1.TokenReview)

					// Return authenticated response
					tokenReview.Status = authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User: authenticationv1.UserInfo{
							Username: "system:serviceaccount:default:test-sa",
							UID:      "test-uid",
						},
					}
					return true, tokenReview, nil
				})

				// Create request with valid token
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Body.String()).To(Equal("authenticated"))
			})

			It("should pass for ServiceAccount tokens", func() {
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					createAction := action.(k8stesting.CreateAction)
					tokenReview := createAction.GetObject().(*authenticationv1.TokenReview)

					tokenReview.Status = authenticationv1.TokenReviewStatus{
						Authenticated: true,
						User: authenticationv1.UserInfo{
							Username: "system:serviceaccount:kubernaut:dynamic-toolset-sa",
							Groups:   []string{"system:serviceaccounts", "system:authenticated"},
						},
					}
					return true, tokenReview, nil
				})

				req := httptest.NewRequest("GET", "/api/v1/services", nil)
				req.Header.Set("Authorization", "Bearer sa-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusOK))
			})
		})

		Context("with invalid Bearer token", func() {
			It("should return 401 Unauthorized", func() {
				// Setup fake TokenReview response (NOT authenticated)
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					createAction := action.(k8stesting.CreateAction)
					tokenReview := createAction.GetObject().(*authenticationv1.TokenReview)

					tokenReview.Status = authenticationv1.TokenReviewStatus{
						Authenticated: false,
					}
					return true, tokenReview, nil
				})

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer invalid-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
				Expect(rr.Body.String()).To(ContainSubstring("Invalid or expired token"))
			})

			It("should return 401 for expired tokens", func() {
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					createAction := action.(k8stesting.CreateAction)
					tokenReview := createAction.GetObject().(*authenticationv1.TokenReview)

					tokenReview.Status = authenticationv1.TokenReviewStatus{
						Authenticated: false,
						Error:         "token has expired",
					}
					return true, tokenReview, nil
				})

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer expired-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("with missing Authorization header", func() {
			It("should return 401 Unauthorized", func() {
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				// No Authorization header

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
				Expect(rr.Body.String()).To(ContainSubstring("Bearer token required"))
			})
		})

		Context("with malformed Authorization header", func() {
			It("should return 401 for missing 'Bearer' prefix", func() {
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "token-without-bearer")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return 401 for empty token", func() {
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer ")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return 401 for wrong auth scheme", func() {
				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("when TokenReview API fails", func() {
			It("should return 500 Internal Server Error", func() {
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, context.DeadlineExceeded
				})

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("with request context cancellation", func() {
			It("should handle context timeout gracefully", func() {
				fakeClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
					// Simulate slow response
					return true, nil, context.DeadlineExceeded
				})

				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				req := httptest.NewRequest("GET", "/api/v1/toolset", nil).WithContext(ctx)
				req.Header.Set("Authorization", "Bearer valid-token")

				rr := httptest.NewRecorder()
				authMW.Middleware(handler).ServeHTTP(rr, req)

				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	Describe("ExtractUsername", func() {
		It("should extract ServiceAccount username", func() {
			username := "system:serviceaccount:kubernaut:dynamic-toolset-sa"
			namespace, saName := middleware.ExtractServiceAccount(username)

			Expect(namespace).To(Equal("kubernaut"))
			Expect(saName).To(Equal("dynamic-toolset-sa"))
		})

		It("should handle user accounts", func() {
			username := "admin@example.com"
			namespace, saName := middleware.ExtractServiceAccount(username)

			Expect(namespace).To(BeEmpty())
			Expect(saName).To(BeEmpty())
		})

		It("should handle malformed ServiceAccount format", func() {
			username := "system:serviceaccount:namespace-only"
			namespace, saName := middleware.ExtractServiceAccount(username)

			Expect(namespace).To(BeEmpty())
			Expect(saName).To(BeEmpty())
		})
	})
})

