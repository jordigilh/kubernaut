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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Handler UserDriving Status — #774, BR-INTERACTIVE-001", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
	)

	BeforeEach(func() {
		store = session.NewStore(1 * time.Hour)
		manager = session.NewManager(store, logr.Discard(), nil, nil)
		handler = server.NewHandler(manager, nil, logr.Discard(), nil)
	})

	Describe("UT-KA-774-006: mapSessionStatusToAPI maps StatusUserDriving to 'user_driving'", func() {
		It("should return status 'user_driving' from the session status endpoint", func() {
			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{})
			Expect(err).NotTo(HaveOccurred())

			err = manager.TransitionToUserDriving(id, "operator@example.com", []string{"sre"})
			Expect(err).NotTo(HaveOccurred())

			res, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
				context.Background(),
				agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{
					SessionID: id,
				},
			)
			Expect(err).NotTo(HaveOccurred())

			status, ok := res.(*agentclient.SessionStatus)
			Expect(ok).To(BeTrue(), "response should be *SessionStatus")
			Expect(status.Status).To(Equal("user_driving"),
				"mapSessionStatusToAPI must map StatusUserDriving to 'user_driving'")
		})
	})
})
