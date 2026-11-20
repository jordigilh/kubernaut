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

package main

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestAuthWebhookCmdAgentSessionReaderWiring is the Ginkgo bootstrap for the
// #2674 AgentSession existence-gate reader regression coverage under package
// main. agentSessionGateReader is the production wiring seam called by
// registerAuthWebhookWorkflowHandlers, so its proof belongs next to main.go.
// The pre-existing *_test.go files in this package use plain testing.T and
// predate the Ginkgo convention (see cmd/apifrontend/reload_identity_suite_test.go
// for the same pattern); new coverage in this package uses this suite.
func TestAuthWebhookCmdAgentSessionReaderWiring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AuthWebhook cmd AgentSession Reader Wiring Suite")
}
