/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

    http://www.apache.org/licenses/LICENSE-2.0
*/

package handlers_test

import (
	"encoding/json"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// BR-KA-016 / DD-KA-2511; FedRAMP SI-10/AU-3; SOC 2 CC7.2.
var _ = Describe("Generic recurrent RCA Mock LLM contract — #2511", func() {
	It("IT-MOCK-2511-001: continues RCA after server-injected history and permits a changed target", func() {
		registry := scenarios.DefaultRegistry()
		_, found := registry.Get("recurrent_rca_2511")
		Expect(found).To(BeTrue())
		server := httptest.NewServer(handlers.NewRouter(registry, false, config.ModeFull))
		DeferCleanup(server.Close)

		tools := []openai.Tool{{Type: "function", Function: openai.ToolDefinition{Name: openai.ToolSubmitResult}}}
		messages := []openai.Message{{Role: "user", Content: issue2482StringPtr("Signal Name: recurrentrca2511")}}
		first := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
		Expect(first.Choices[0].Message.ToolCalls).To(HaveLen(1))
		var firstArgs map[string]interface{}
		Expect(json.Unmarshal([]byte(first.Choices[0].Message.ToolCalls[0].Function.Arguments), &firstArgs)).To(Succeed())
		Expect(firstArgs["root_cause_analysis"].(map[string]interface{})["remediation_target"].(map[string]interface{})["name"]).To(Equal("api-a"))

		messages = append(messages, first.Choices[0].Message)
		messages = append(messages, openai.Message{Role: "user", Content: issue2482StringPtr("## PRIOR REMEDIATION HISTORY REVIEW\nTarget: prod/Deployment/api-a")})
		second := postIssue2482OpenAI(server.URL, openai.ChatCompletionRequest{Model: "mock-model", Messages: messages, Tools: tools})
		Expect(second.Choices[0].Message.ToolCalls).To(HaveLen(1))
		var secondArgs map[string]interface{}
		Expect(json.Unmarshal([]byte(second.Choices[0].Message.ToolCalls[0].Function.Arguments), &secondArgs)).To(Succeed())
		rca := secondArgs["root_cause_analysis"].(map[string]interface{})
		Expect(rca["remediation_target"].(map[string]interface{})["name"]).To(Equal("api-b"))
		Expect(rca["causal_mechanism_status"]).To(Equal("probable"))
	})
})
