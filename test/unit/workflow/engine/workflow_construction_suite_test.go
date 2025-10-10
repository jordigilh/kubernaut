/*
Copyright 2025 Jordi Gil.

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

package engine

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WF-CONST-SUITE-001: Workflow Construction Business Test Suite
// Business Impact: Ensures workflow construction meets business requirements for structural integrity
// Stakeholder Value: Operations teams can trust workflow construction reliability

func TestWorkflowConstruction(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workflow Construction Suite")
}
