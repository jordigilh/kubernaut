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

// TestKubernautAgentCmd is the Ginkgo bootstrap for package main's
// white-box test suite (unexported production functions such as
// buildLLMClientFromConfig, buildAnthropicNativeClient, and
// anthropicReasoningOptions require same-package test access, so this
// suite lives in `package main` rather than `main_test`). Per AGENTS.md
// ("Testing Requirements": Ginkgo/Gomega BDD is mandatory for business
// logic tests), this is the single RunSpecs entry point for every
// Describe/It block added under cmd/kubernautagent going forward.
func TestKubernautAgentCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubernautAgent cmd Suite")
}
