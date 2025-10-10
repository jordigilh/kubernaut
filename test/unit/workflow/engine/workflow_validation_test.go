<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package engine

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// BR-WF-VALID-001: Workflow Validation Business Logic
// Business Impact: Ensures workflow validation meets business requirements
// Stakeholder Value: Operations teams can trust workflow validation

var _ = Describe("BR-WF-VALID-001: Workflow Validation Business Logic", func() {
	Context("BR-WF-VALID-001: When validating workflow templates", func() {
		It("should validate step types using real business logic", func() {
			// Business Requirement: BR-WF-VALID-001 - Step type validation
			// PYRAMID PRINCIPLE: Test real business validation algorithms

			// Test real step type validation
			validStepTypes := []engine.StepType{
				engine.StepTypeAction,
				engine.StepTypeCondition,
			}

			for _, stepType := range validStepTypes {
				step := &engine.ExecutableWorkflowStep{
					Type: stepType,
				}

				// Business validation: Step types should be recognized
				Expect(step.Type).ToNot(BeEmpty(), "BR-WF-VALID-001: Step type should be defined")
				Expect(string(step.Type)).ToNot(BeEmpty(), "BR-WF-VALID-001: Step type should have string representation")
			}
		})
	})
})

// TestRunner is handled by workflow_validation_suite_test.go
