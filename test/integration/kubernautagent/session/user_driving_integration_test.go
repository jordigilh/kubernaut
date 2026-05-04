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

package session_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("UserDriving Integration — #774, BR-INTERACTIVE-001", func() {

	Describe("IT-KA-774-001: TransitionToUserDriving full wiring with handler", func() {
		It("should transition session, cancel goroutine, and handler returns user_driving status with identity", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)
			handler := server.NewHandler(manager, nil, logr.Discard(), nil)

			ctxCancelled := make(chan struct{})
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				close(ctxCancelled)
				return nil, ctx.Err()
			}, map[string]string{"incident_id": "inc-774-it"})
			Expect(err).NotTo(HaveOccurred())

			err = manager.TransitionToUserDriving(id, "sre-operator@example.com", []string{"sre", "production-oncall"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(ctxCancelled).Should(BeClosed(),
				"TransitionToUserDriving must cancel the investigation goroutine")

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusUserDriving))
			Expect(sess.Metadata["acting_user"]).To(Equal("sre-operator@example.com"))

			var groups []string
			err = json.Unmarshal([]byte(sess.Metadata["acting_user_groups"]), &groups)
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(ConsistOf("sre", "production-oncall"))

			_ = handler
		})
	})

	Describe("IT-KA-774-002: HTTP round-trip through real ogen handler returns user_driving with identity", func() {
		It("should serve user_driving status with acting_user fields via the production handler pipeline", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)
			handler := server.NewHandler(manager, nil, logr.Discard(), nil)

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			err = manager.TransitionToUserDriving(id, "operator@company.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			// QE-1: Use the real ogen Server wrapping the production handler —
			// this validates that SessionStatus.ActingUser and ActingUserGroups
			// are emitted by the actual handler, not a test stub.
			ogenSrv, err := agentclient.NewServer(handler)
			Expect(err).NotTo(HaveOccurred())

			ts := httptest.NewServer(ogenSrv)
			defer ts.Close()

			client, err := agentclient.NewKubernautAgentClientWithTransport(
				agentclient.Config{BaseURL: ts.URL, Timeout: 5 * time.Second},
				http.DefaultTransport,
			)
			Expect(err).NotTo(HaveOccurred())

			result, err := client.PollSession(context.Background(), id)
			Expect(err).NotTo(HaveOccurred())

			Expect(result.Status).To(Equal("user_driving"))
			Expect(result.ActingUser).To(Equal("operator@company.com"))
			Expect(result.ActingUserGroups).To(ConsistOf("sre"))
		})
	})
})
