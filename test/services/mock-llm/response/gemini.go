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
package response

import (
	"encoding/json"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// GeminiRequest represents a Gemini API generateContent request.
type GeminiRequest struct {
	Contents          []GeminiContent    `json:"contents"`
	SystemInstruction *GeminiContent     `json:"systemInstruction,omitempty"`
	Tools             []GeminiToolDecl   `json:"tools,omitempty"`
	GenerationConfig  *GeminiGenConfig   `json:"generationConfig,omitempty"`
}

// GeminiContent represents a content block with role and parts.
type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a single part within a content block.
type GeminiPart struct {
	Text             string               `json:"text,omitempty"`
	FunctionCall     *GeminiFunctionCall  `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResp  `json:"functionResponse,omitempty"`
}

// GeminiFunctionCall represents a function call issued by the model.
type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

// GeminiFunctionResp represents a function response sent by the client.
type GeminiFunctionResp struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

// GeminiToolDecl represents a tool declaration in the request.
type GeminiToolDecl struct {
	FunctionDeclarations []GeminiFunctionDecl `json:"functionDeclarations,omitempty"`
}

// GeminiFunctionDecl describes a function available to the model.
type GeminiFunctionDecl struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// GeminiGenConfig holds generation configuration parameters.
type GeminiGenConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

// GeminiResponse represents a Gemini API generateContent response.
type GeminiResponse struct {
	Candidates   []GeminiCandidate `json:"candidates"`
	ModelVersion string            `json:"modelVersion"`
}

// GeminiCandidate represents a single candidate in the response.
type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

const geminiModelVersion = "mock-model"

// BuildGeminiTextResponse creates a Gemini response with a text part.
func BuildGeminiTextResponse(cfg scenarios.MockScenarioConfig) GeminiResponse {
	text := buildAnalysisText(cfg)
	return GeminiResponse{
		Candidates: []GeminiCandidate{
			{
				Content: GeminiContent{
					Role:  "model",
					Parts: []GeminiPart{{Text: text}},
				},
				FinishReason: "STOP",
			},
		},
		ModelVersion: geminiModelVersion,
	}
}

// BuildGeminiToolCallResponse creates a Gemini response with a single function call.
func BuildGeminiToolCallResponse(toolName string, cfg scenarios.MockScenarioConfig) GeminiResponse {
	args := buildToolArguments(toolName, cfg)
	return GeminiResponse{
		Candidates: []GeminiCandidate{
			{
				Content: GeminiContent{
					Role: "model",
					Parts: []GeminiPart{
						{
							FunctionCall: &GeminiFunctionCall{
								Name: toolName,
								Args: args,
							},
						},
					},
				},
				FinishReason: "STOP",
			},
		},
		ModelVersion: geminiModelVersion,
	}
}

// BuildGeminiMultiToolCallResponse creates a Gemini response with multiple function calls.
func BuildGeminiMultiToolCallResponse(toolEntries []scenarios.MultiToolCallEntry) GeminiResponse {
	parts := make([]GeminiPart, len(toolEntries))
	for i, entry := range toolEntries {
		parts[i] = GeminiPart{
			FunctionCall: &GeminiFunctionCall{
				Name: entry.Name,
				Args: entry.Arguments,
			},
		}
	}
	return GeminiResponse{
		Candidates: []GeminiCandidate{
			{
				Content: GeminiContent{
					Role:  "model",
					Parts: parts,
				},
				FinishReason: "STOP",
			},
		},
		ModelVersion: geminiModelVersion,
	}
}

// BuildGeminiErrorResponse creates a Gemini-compatible error response body.
func BuildGeminiErrorResponse(message string) map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]interface{}{
			"code":    500,
			"message": message,
			"status":  "INTERNAL",
		},
	}
}

// HasFunctionResponse returns true if any content in the request contains a functionResponse part.
func HasFunctionResponse(contents []GeminiContent) bool {
	for _, c := range contents {
		for _, p := range c.Parts {
			if p.FunctionResponse != nil {
				return true
			}
		}
	}
	return false
}

// LastContentIsFunctionResponse returns true if the final content entry in the
// conversation is a FunctionResponse. This indicates the ADK just executed a
// tool in the current iteration; the mock-LLM should stop repeating tool calls
// to avoid an infinite loop (issue #1189).
func LastContentIsFunctionResponse(contents []GeminiContent) bool {
	if len(contents) == 0 {
		return false
	}
	last := contents[len(contents)-1]
	for _, p := range last.Parts {
		if p.FunctionResponse != nil {
			return true
		}
	}
	return false
}

// ExtractFieldFromFunctionResponse scans Gemini contents for a FunctionResponse
// with the given tool name and extracts a top-level string field from its JSON
// response object. Returns empty string if not found.
func ExtractFieldFromFunctionResponse(contents []GeminiContent, toolName, field string) string {
	for _, c := range contents {
		for _, p := range c.Parts {
			if p.FunctionResponse == nil || p.FunctionResponse.Name != toolName {
				continue
			}
			raw, err := json.Marshal(p.FunctionResponse.Response)
			if err != nil {
				continue
			}
			var obj map[string]interface{}
			if err := json.Unmarshal(raw, &obj); err != nil {
				continue
			}
			if val, ok := obj[field]; ok {
				if s, ok := val.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

// ExtractTextFromContents extracts all text parts from Gemini contents,
// returning the last user text (for Content field) and all text joined (for AllText field).
func ExtractTextFromContents(contents []GeminiContent, systemInstruction *GeminiContent) (lastUserText string, allText string) {
	var allParts []string

	if systemInstruction != nil {
		for _, p := range systemInstruction.Parts {
			if p.Text != "" {
				allParts = append(allParts, p.Text)
			}
		}
	}

	for _, c := range contents {
		for _, p := range c.Parts {
			if p.Text != "" {
				allParts = append(allParts, p.Text)
				if c.Role == "user" {
					lastUserText = p.Text
				}
			}
			if p.FunctionCall != nil {
				argsJSON, _ := json.Marshal(p.FunctionCall.Args)
				allParts = append(allParts, p.FunctionCall.Name+" "+string(argsJSON))
			}
			if p.FunctionResponse != nil {
				respJSON, _ := json.Marshal(p.FunctionResponse.Response)
				allParts = append(allParts, p.FunctionResponse.Name+" "+string(respJSON))
			}
		}
	}

	var joined string
	for i, s := range allParts {
		if i > 0 {
			joined += " "
		}
		joined += s
	}
	allText = joined
	return lastUserText, allText
}
