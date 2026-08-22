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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("UserDriving Integration — #774, BR-INTERACTIVE-001", func() {

	Describe("IT-KA-774-001: TransitionToUserDriving full wiring with handler", func() {
		It("should transition session, cancel goroutine, and handler returns user_driving status with identity", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

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
		})
	})
})
