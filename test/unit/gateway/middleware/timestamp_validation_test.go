/*
Copyright 2025 Jordi Gil.

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

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/gateway/middleware"
)

var _ = Describe("Timestamp Validation", func() {
	var (
		recorder    *httptest.ResponseRecorder
		testHandler http.Handler
	)

	BeforeEach(func() {
		recorder = httptest.NewRecorder()

		// Create test handler that always returns 200 OK
		testHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		})
	})

	// TDD RED Phase - Test 1-3: Valid timestamps
	Context("Valid Timestamps", func() {
		It("should allow request with current timestamp", func() {
			// Arrange: Create validator with 5 minute tolerance
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with current timestamp
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			currentTime := time.Now().Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(currentTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Request should succeed
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})

		It("should allow request with timestamp within tolerance (4 minutes old)", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with timestamp 4 minutes in the past
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			pastTime := time.Now().Add(-4 * time.Minute).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(pastTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Request should succeed (within 5 minute tolerance)
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})

		It("should allow request with timestamp at exact tolerance boundary", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with timestamp just under 5 minutes old (4m59s)
			// This accounts for execution time between timestamp creation and validation
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			boundaryTime := time.Now().Add(-5*time.Minute + 1*time.Second).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(boundaryTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Request should succeed (at boundary)
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	// TDD RED Phase - Test 4-6: Invalid timestamps (too old)
	Context("Invalid Timestamps - Too Old", func() {
		It("should reject request with timestamp exceeding tolerance with 400", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with timestamp 10 minutes old (exceeds 5 min tolerance)
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			oldTime := time.Now().Add(-10 * time.Minute).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(oldTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Request should be rejected with 400 Bad Request
			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(recorder.Body.String()).To(ContainSubstring("timestamp too old"))
		})

		It("should reject request with very old timestamp (1 hour)", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			veryOldTime := time.Now().Add(-1 * time.Hour).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(veryOldTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(recorder.Body.String()).To(ContainSubstring("timestamp too old"))
		})
	})

	// TDD RED Phase - Test 7-8: Invalid timestamps (future)
	Context("Invalid Timestamps - Future", func() {
		It("should reject request with future timestamp with 400", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with timestamp 10 minutes in the future
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			futureTime := time.Now().Add(10 * time.Minute).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(futureTime, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Request should be rejected (prevents clock skew attacks)
			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(recorder.Body.String()).To(ContainSubstring("timestamp in future"))
		})

		It("should allow small clock skew (1 minute future) with tolerance", func() {
			// Arrange: Create validator with 2 minute clock skew tolerance
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with timestamp 1 minute in future (small clock skew)
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			slightlyFuture := time.Now().Add(1 * time.Minute).Unix()
			req.Header.Set("X-Timestamp", strconv.FormatInt(slightlyFuture, 10))
			handler.ServeHTTP(recorder, req)

			// Assert: Should allow small clock skew
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	// TDD RED Phase - Test 9-11: Malformed timestamps
	Context("Malformed Timestamps", func() {
		It("should allow request with missing timestamp header (optional validation)", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request without X-Timestamp header
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			handler.ServeHTTP(recorder, req)

			// Assert: Timestamp validation is optional - passes through without header
			Expect(recorder.Code).To(Equal(http.StatusOK))
			Expect(recorder.Body.String()).To(Equal("OK"))
		})

		It("should reject request with non-numeric timestamp with 400", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with invalid timestamp format
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req.Header.Set("X-Timestamp", "not-a-number")
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(recorder.Body.String()).To(ContainSubstring("invalid timestamp format"))
		})

		It("should reject request with negative timestamp with 400", func() {
			// Arrange
			validator := middleware.TimestampValidator(5 * time.Minute)
			handler := validator(testHandler)

			// Act: Send request with negative timestamp
			req := httptest.NewRequest("POST", "/webhook/prometheus", nil)
			req.Header.Set("X-Timestamp", "-12345")
			handler.ServeHTTP(recorder, req)

			// Assert
			Expect(recorder.Code).To(Equal(http.StatusBadRequest))
			Expect(recorder.Body.String()).To(ContainSubstring("invalid timestamp"))
		})
	})
})
