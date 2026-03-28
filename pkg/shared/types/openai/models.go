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
package openai

// Usage represents token usage in a chat completion response.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelInfo represents a model entry in the models list response.
type ModelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// ModelsResponse represents the response from /v1/models or /api/tags.
type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

const (
	DefaultModel       = "mock-model"
	FixedCreatedTime   = int64(1701388800)
	ObjectChatCompletion = "chat.completion"
)
