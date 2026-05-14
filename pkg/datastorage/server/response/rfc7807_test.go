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

package response_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

func TestResponse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Response Suite")
}

var _ = Describe("WriteRFC7807InternalError (#1048 Phase 4 / SI-11)", func() {

	It("UT-DS-1048-RE-001: should write 500 with generic detail, not the actual error", func() {
		logger := kubelog.NewLogger(kubelog.DefaultOptions())
		rr := httptest.NewRecorder()

		internalErr := fmt.Errorf("pq: connection refused to db-host-secret.internal:5432")
		response.WriteRFC7807InternalError(rr, "database-error", "Database Error", internalErr, logger)

		Expect(rr.Code).To(Equal(http.StatusInternalServerError))
		Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))

		var problem response.RFC7807Problem
		Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
		Expect(problem.Status).To(Equal(500))
		Expect(problem.Title).To(Equal("Database Error"))
		Expect(problem.Detail).To(Equal("An internal error occurred. Check server logs for details."))
		Expect(problem.Detail).ToNot(ContainSubstring("pq:"))
		Expect(problem.Detail).ToNot(ContainSubstring("db-host-secret"))
		Expect(problem.Detail).ToNot(ContainSubstring("5432"))
	})

	It("UT-DS-1048-RE-002: should set correct RFC 7807 type URI", func() {
		logger := kubelog.NewLogger(kubelog.DefaultOptions())
		rr := httptest.NewRecorder()

		response.WriteRFC7807InternalError(rr, "conversion_error", "Conversion Error",
			fmt.Errorf("some internal detail"), logger)

		var problem response.RFC7807Problem
		Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
		Expect(problem.Type).To(Equal("https://kubernaut.ai/problems/conversion_error"))
	})
})
