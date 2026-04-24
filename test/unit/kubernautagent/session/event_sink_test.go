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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

var _ = Describe("Event Sink Context Helpers — #823 PR3", func() {

	Describe("UT-KA-823-C08: Event sink context round-trip", func() {
		It("event sink attached to context is retrievable by EventSinkFromContext", func() {
			ch := make(chan session.InvestigationEvent, 1)
			ctx := session.WithEventSink(context.Background(), ch)

			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).NotTo(BeNil(), "should retrieve the attached event sink")

			retrieved <- session.InvestigationEvent{Type: session.EventTypeComplete}
			Expect(ch).To(Receive(HaveField("Type", session.EventTypeComplete)))
		})
	})

	Describe("UT-KA-823-C09: Missing event sink returns nil", func() {
		It("EventSinkFromContext on plain context returns nil without panic", func() {
			ctx := context.Background()
			retrieved := session.EventSinkFromContext(ctx)
			Expect(retrieved).To(BeNil(), "no event sink attached — should return nil")
		})
	})
})
