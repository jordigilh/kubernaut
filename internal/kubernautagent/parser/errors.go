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

package parser

// ErrEmptyContent is returned by Parse when the input content is empty.
type ErrEmptyContent struct{}

func (e *ErrEmptyContent) Error() string {
	return "empty JSON content"
}

// ErrNoJSON is returned by Parse when the input contains no extractable JSON.
// Content preserves the raw LLM text for investigator-level classification.
type ErrNoJSON struct {
	Content string
}

func (e *ErrNoJSON) Error() string {
	return "no JSON found in response"
}

// ErrNoRecognizedFields is returned by Parse when JSON was found but contained
// no recognized investigation fields (no RCASummary, WorkflowID, or confidence).
type ErrNoRecognizedFields struct {
	Raw string
}

func (e *ErrNoRecognizedFields) Error() string {
	return "no recognized fields in LLM JSON response"
}
