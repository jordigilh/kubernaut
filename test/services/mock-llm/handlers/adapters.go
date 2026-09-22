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
package handlers

import (
	"encoding/json"
	"fmt"

	openai "github.com/jordigilh/kubernaut/pkg/shared/types/openai"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
)

const openAIToolMessageRole = "tool"

// NormalizeOpenAITranscript converts an OpenAI Chat Completions request into the
// provider-neutral transcript consumed by the discovery planner.
func NormalizeOpenAITranscript(req openai.ChatCompletionRequest) conversation.DiscoveryTranscript {
	transcript := conversation.DiscoveryTranscript{AdvertisedTools: openAIToolNames(req.Tools)}
	toolNamesByID := make(map[string]string)
	pendingNames := make([]string, 0)

	for _, message := range req.Messages {
		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			pendingNames = pendingNames[:0]
			for _, toolCall := range message.ToolCalls {
				toolNamesByID[toolCall.ID] = toolCall.Function.Name
				pendingNames = append(pendingNames, toolCall.Function.Name)
				transcript.Events = append(transcript.Events, conversation.DiscoveryEvent{
					Kind:       conversation.DiscoveryToolCallEvent,
					ToolName:   toolCall.Function.Name,
					ToolCallID: toolCall.ID,
				})
			}
			continue
		}
		if message.Role != openAIToolMessageRole {
			continue
		}

		toolName := toolNamesByID[message.ToolCallID]
		if message.ToolCallID == "" && len(pendingNames) > 0 {
			toolName = pendingNames[0]
			pendingNames = pendingNames[1:]
		}
		if toolName == "" {
			continue
		}
		payload := ""
		if message.Content != nil {
			payload = *message.Content
		}
		transcript.Events = append(transcript.Events, conversation.DiscoveryEvent{
			Kind:       conversation.DiscoveryToolResultEvent,
			ToolName:   toolName,
			ToolCallID: message.ToolCallID,
			Payload:    payload,
		})
	}

	return transcript
}

// NormalizeGeminiTranscript converts Gemini function-call content into the
// provider-neutral transcript consumed by the discovery planner.
func NormalizeGeminiTranscript(contents []response.GeminiContent, tools []response.GeminiToolDecl) (conversation.DiscoveryTranscript, error) {
	transcript := conversation.DiscoveryTranscript{AdvertisedTools: geminiToolNames(tools)}
	for _, content := range contents {
		for _, part := range content.Parts {
			if part.FunctionCall != nil {
				transcript.Events = append(transcript.Events, conversation.DiscoveryEvent{
					Kind:     conversation.DiscoveryToolCallEvent,
					ToolName: part.FunctionCall.Name,
				})
			}
			if part.FunctionResponse == nil {
				continue
			}
			payload, err := serializeGeminiFunctionResponse(part.FunctionResponse.Response)
			if err != nil {
				return conversation.DiscoveryTranscript{}, err
			}
			transcript.Events = append(transcript.Events, conversation.DiscoveryEvent{
				Kind:     conversation.DiscoveryToolResultEvent,
				ToolName: part.FunctionResponse.Name,
				Payload:  payload,
			})
		}
	}
	return transcript, nil
}

func openAIToolNames(tools []openai.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Function.Name != "" {
			names = append(names, tool.Function.Name)
		}
	}
	return names
}

func serializeGeminiFunctionResponse(value interface{}) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("serialize gemini function response: %w", err)
	}
	return string(data), nil
}
