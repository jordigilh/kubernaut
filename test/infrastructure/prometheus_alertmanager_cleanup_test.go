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

package infrastructure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AlertManager alert cleanup", func() {
	It("UT-INFRA-FP-2579-001 [BR-E2E-001]: resolves only alerts matching the requested fixture labels", func() {
		type alertPayload struct {
			Labels map[string]string `json:"labels"`
			EndsAt string            `json:"endsAt"`
		}
		var resolved []alertPayload
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method == http.MethodGet {
				writer.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(writer).Encode([]alertPayload{
					{Labels: map[string]string{
						"alertname": "MemoryExceedsLimit",
						"namespace": "fp-am-owned",
						"pod":       "memory-eater-owned",
					}},
					{Labels: map[string]string{
						"alertname": "historyfirst2496-unrelated",
						"namespace": "history-2496-unrelated",
					}},
				})).To(Succeed())
				return
			}

			Expect(request.Method).To(Equal(http.MethodPost))
			Expect(json.NewDecoder(request.Body).Decode(&resolved)).To(Succeed())
			writer.WriteHeader(http.StatusOK)
		}))
		DeferCleanup(server.Close)

		Expect(ResolveActiveAlertsForLabels(server.URL, map[string]string{
			"alertname": "MemoryExceedsLimit",
			"namespace": "fp-am-owned",
			"pod":       "memory-eater-owned",
		})).To(Succeed())

		Expect(resolved).To(HaveLen(1))
		Expect(resolved[0].Labels).To(Equal(map[string]string{
			"alertname": "MemoryExceedsLimit",
			"namespace": "fp-am-owned",
			"pod":       "memory-eater-owned",
		}))
		Expect(resolved[0].EndsAt).NotTo(BeEmpty())
	})

	It("UT-INFRA-FP-2579-002 [BR-E2E-001]: requires an empty-valued label to be present", func() {
		Expect(alertLabelsMatch(
			map[string]string{"alertname": "MemoryExceedsLimit"},
			map[string]string{"namespace": ""},
		)).To(BeFalse())
	})
})
