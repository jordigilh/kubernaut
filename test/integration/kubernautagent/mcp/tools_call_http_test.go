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

package mcp_test

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("MCP tools/call over HTTP — PR6a BR-INTERACTIVE-001", func() {

	var (
		stack   *realMCPTestStack
		nsName  string
	)

	BeforeEach(func() {
		nsName = uniqueNamespace("pr6a")
		createNamespace(context.Background(), sharedK8sClient, nsName)
		stack = newRealMCPTestStack(sharedK8sClient, nsName, defaultRealStackOpts())
	})

	AfterEach(func() {
		stack.Close()
	})

	Describe("IT-KA-PR6A-TOOLS-001: tools/list returns registered tools via SDK client", func() {
		It("should list kubernaut_investigate via the MCP protocol", func() {
			ctx := context.Background()
			session, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			var toolNames []string
			for tool, err := range session.Tools(ctx, nil) {
				Expect(err).NotTo(HaveOccurred())
				toolNames = append(toolNames, tool.Name)
			}
			Expect(toolNames).To(ContainElement("kubernaut_investigate"))
		})
	})

	Describe("IT-KA-PR6A-TOOLS-002: tools/call invokes investigate start action", func() {
		It("should return session_id and status=started", func() {
			session, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			result, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-test-001",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse(), "tool call should not return error")

			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["session_id"]).NotTo(BeEmpty(), "real Lease-backed session_id should be a UUID")
			Expect(output["status"]).To(Equal("started"))
		})
	})

	Describe("IT-KA-PR6A-TOOLS-003: tools/call message action returns LLM response", func() {
		It("should return the LLM response for message action via real investigator", func() {
			session, err := connectMCP(stack.Server, "alice@example.com")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = session.Close() }()

			startResult, err := callInvestigate(session, map[string]any{
				"rr_id":  "rr-test-002",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred())
			startOutput, err := decodeOutput(startResult)
			Expect(err).NotTo(HaveOccurred())
			sessionID := startOutput["session_id"].(string)
			Expect(sessionID).NotTo(BeEmpty())

			result, err := callInvestigate(session, map[string]any{
				"rr_id":   "rr-test-002",
				"action":  "message",
				"message": "What caused this pod crash?",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsError).To(BeFalse())

			output, err := decodeOutput(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(output["status"]).To(Equal("message_received"))
			Expect(output["response"]).NotTo(BeEmpty(), "real investigator should return LLM response text")
			Expect(output["session_id"]).To(Equal(sessionID))
		})
	})

	Describe("IT-KA-PR6A-TOOLS-004: unauthenticated request returns 401", func() {
		It("should reject requests without Authorization header", func() {
			resp, err := http.Get(stack.Server.URL + "/mcp")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})
})

// fakeAuthMiddlewareWithContext injects the user into the request context
// via sharedauth.UserContextKey, which the MCP tool handlers use.
// Kept for any tests that need a fixed-user middleware variant.
func fakeAuthMiddlewareWithContext(user string) func(http.Handler) http.Handler {
	return fakeAuthMiddlewareForUser(user)
}

func fakeAuthMiddlewareForUser(user string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), sharedauth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
