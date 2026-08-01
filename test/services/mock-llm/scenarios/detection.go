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
package scenarios

// DetectionContext holds the input data used for scenario detection.
type DetectionContext struct {
	Content         string
	AllText         string
	SignalName      string
	IsProactive     bool
	LastUserContent string

	// AvailableTools lists the function-tool names the caller actually
	// advertised in this request (req.Tools function names, OpenAI handler
	// only -- always empty for the Ollama handler, which has no tools
	// field). Lets a scenario pick which of several candidate tool calls to
	// script strictly based on what was really offered, mirroring how a
	// real LLM can only ever call a tool it was told about -- e.g.
	// distinguishing a hub-local investigation (only kubectl_* tools
	// offered) from a fleet one (overlay tools like resources_get also
	// offered) without the test needing to tell the scenario which
	// environment it's in (E2E-FLEET-017, issue #1729).
	AvailableTools []string
}
