/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func TestMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Middleware Suite")
}

var _ = Describe("MaxBytesReaderMiddleware", func() {
	var (
		logger = kubelog.NewLogger(kubelog.DefaultOptions())
		limit  int64 = 1024 // 1 KB for tests
	)

	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if middleware.IsMaxBytesError(err) {
			middleware.WriteMaxBytesExceeded(w, logger)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})

	It("UT-DS-1048-MB-001: should pass through POST body within limit", func() {
		mw := middleware.MaxBytesReaderMiddleware(limit, logger)
		handler := mw(echoHandler)

		body := strings.Repeat("a", 512)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal(body))
	})

	It("UT-DS-1048-MB-002: should reject POST body exceeding limit with 413 (Content-Length fast-path)", func() {
		mw := middleware.MaxBytesReaderMiddleware(limit, logger)
		handler := mw(echoHandler)

		body := strings.Repeat("a", 2048)
		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		req.ContentLength = 2048
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge))
		Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))

		var problem map[string]interface{}
		Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
		Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/request-too-large"))
		Expect(problem["status"]).To(BeNumerically("==", 413))
	})

	It("UT-DS-1048-MB-003: should NOT limit GET requests", func() {
		mw := middleware.MaxBytesReaderMiddleware(limit, logger)
		handler := mw(echoHandler)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusOK))
	})

	It("UT-DS-1048-MB-004: should limit PATCH body exceeding limit", func() {
		mw := middleware.MaxBytesReaderMiddleware(limit, logger)
		handler := mw(echoHandler)

		body := strings.Repeat("x", 2048)
		req := httptest.NewRequest(http.MethodPatch, "/test", strings.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge))
	})

	It("UT-DS-1048-MB-005: should limit DELETE body exceeding limit", func() {
		mw := middleware.MaxBytesReaderMiddleware(limit, logger)
		handler := mw(echoHandler)

		body := strings.Repeat("y", 2048)
		req := httptest.NewRequest(http.MethodDelete, "/test", strings.NewReader(body))
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge))
	})
})

var _ = Describe("OpenAPIValidator", func() {
	It("UT-DS-1048-OV-001: should successfully create validator with embedded spec", func() {
		logger := kubelog.NewLogger(kubelog.DefaultOptions())
		validator, err := middleware.NewOpenAPIValidator(logger, nil)
		Expect(err).ToNot(HaveOccurred())

		mw := validator.Middleware
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		Expect(rr.Code).To(Equal(http.StatusOK))
	})
})

var _ = Describe("IsMaxBytesError", func() {
	It("should return true for http.MaxBytesError", func() {
		limit := int64(10)
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, 20)))
		r.Body = http.MaxBytesReader(nil, r.Body, limit)
		_, err := io.ReadAll(r.Body)
		Expect(middleware.IsMaxBytesError(err)).To(BeTrue())
	})

	It("should return false for non-MaxBytesError", func() {
		Expect(middleware.IsMaxBytesError(io.EOF)).To(BeFalse())
	})
})
