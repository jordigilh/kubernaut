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

package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ========================================
// Fix #1390: Nil-Result HTTP Round-Trip Integration Test
//
// IT-KA-1390-W02: Proves that GET /api/v1/incident/session/{id}/result
// returns HTTP 200 with a structured result for completed sessions that
// have no investigation result (nil result).
// ========================================

var _ = Describe("Fix #1390: Nil-Result HTTP Round-Trip — BR-SESSION-002", Label("integration"), func() {

	Context("IT-KA-1390-W02 [SC-24]: Nil-result completed session returns structured result via real HTTP", func() {
		It("should return HTTP 200 with synthetic result body instead of 409", func() {
			ts, mgr := newTestAPIServer(&stubInvestigator{
				fn: func(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
					return nil, nil
				},
			})
			defer ts.Close()

			By("submitting an investigation via HTTP POST")
			postReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/incident/analyze",
				strings.NewReader(validIncidentJSON()))
			Expect(err).NotTo(HaveOccurred())
			postReq.Header.Set("Content-Type", "application/json")

			postResp, err := http.DefaultClient.Do(postReq)
			Expect(err).NotTo(HaveOccurred())
			defer postResp.Body.Close()
			Expect(postResp.StatusCode).To(Equal(http.StatusAccepted))

			var sub struct {
				SessionID string `json:"session_id"`
			}
			Expect(json.NewDecoder(postResp.Body).Decode(&sub)).To(Succeed())
			Expect(sub.SessionID).NotTo(BeEmpty())

			By("waiting for session to complete with nil result")
			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(sub.SessionID)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 5*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted))

			By("fetching result via HTTP GET — should return 200, not 409")
			resultURL := ts.URL + "/api/v1/incident/session/" + sub.SessionID + "/result"
			getResp, err := http.DefaultClient.Get(resultURL)
			Expect(err).NotTo(HaveOccurred())
			defer getResp.Body.Close()

			Expect(getResp.StatusCode).To(Equal(http.StatusOK),
				"completed session with nil result must return HTTP 200 with synthetic result, not 409")

			var body struct {
				Analysis string `json:"analysis"`
			}
			Expect(json.NewDecoder(getResp.Body).Decode(&body)).To(Succeed())
			Expect(body.Analysis).To(ContainSubstring("Investigation completed without result"))
		})
	})
})
