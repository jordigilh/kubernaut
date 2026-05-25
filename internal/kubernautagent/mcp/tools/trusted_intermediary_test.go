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

package tools_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

var _ = Describe("Trusted Intermediary Identity (#1287)", func() {

	Describe("UT-KA-1287-005: Tool input schemas accept acting_user fields", func() {
		It("InvestigateInput unmarshals acting_user and acting_user_groups", func() {
			raw := `{"rr_id":"rr-1","action":"start","acting_user":"alice@example.com","acting_user_groups":["sre","admin"]}`
			var input mcptools.InvestigateInput
			Expect(json.Unmarshal([]byte(raw), &input)).To(Succeed())
			Expect(input.ActingUser).To(Equal("alice@example.com"))
			Expect(input.ActingUserGroups).To(ConsistOf("sre", "admin"))
		})

		It("SelectWorkflowInput unmarshals acting_user and acting_user_groups", func() {
			raw := `{"rr_id":"rr-1","workflow_id":"wf-1","acting_user":"bob@example.com","acting_user_groups":["dev"]}`
			var input mcptools.SelectWorkflowInput
			Expect(json.Unmarshal([]byte(raw), &input)).To(Succeed())
			Expect(input.ActingUser).To(Equal("bob@example.com"))
			Expect(input.ActingUserGroups).To(ConsistOf("dev"))
		})

		It("CompleteNoActionInput unmarshals acting_user and acting_user_groups", func() {
			raw := `{"rr_id":"rr-1","acting_user":"charlie@example.com","acting_user_groups":["ops"]}`
			var input mcptools.CompleteNoActionInput
			Expect(json.Unmarshal([]byte(raw), &input)).To(Succeed())
			Expect(input.ActingUser).To(Equal("charlie@example.com"))
			Expect(input.ActingUserGroups).To(ConsistOf("ops"))
		})
	})

	Describe("UT-KA-1287-006: KA resolves acting_user from payload", func() {
		It("prefers acting_user from input over middleware identity", func() {
			middlewareUser := mcpinternal.UserInfo{Username: "system:serviceaccount:kubernaut:apifrontend"}
			resolved := mcptools.ResolveUser(middlewareUser, "alice@example.com", []string{"sre", "admin"})
			Expect(resolved.Username).To(Equal("alice@example.com"))
			Expect(resolved.Groups).To(ConsistOf("sre", "admin"))
		})
	})

	Describe("UT-KA-1287-007: Non-intermediary caller uses middleware identity", func() {
		It("falls back to middleware identity when acting_user is empty", func() {
			middlewareUser := mcpinternal.UserInfo{Username: "direct-user@example.com", Groups: []string{"viewers"}}
			resolved := mcptools.ResolveUser(middlewareUser, "", nil)
			Expect(resolved.Username).To(Equal("direct-user@example.com"))
			Expect(resolved.Groups).To(ConsistOf("viewers"))
		})
	})
})
