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

// TestAPIFrontendCmdReloadIdentity is the Ginkgo bootstrap for the #1599
// restart-required LLM identity regression coverage under package main
// (configReloadCallback requires same-package test access). Per AGENTS.md
// ("Testing Requirements": Ginkgo/Gomega BDD is mandatory for business logic
// tests), new Describe/It coverage in this package should use this suite
// rather than plain testing.T (pre-existing *_test.go files in this package
// predate that convention and are out of scope to convert here).
func TestAPIFrontendCmdReloadIdentity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "APIFrontend cmd Reload Identity Suite")
}
