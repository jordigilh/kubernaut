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
//go:build e2e
// +build e2e

package webhook

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-WEBHOOK-E2E-001: Webhook Service E2E Test Suite Organization
// Business Impact: Ensures comprehensive validation of complete AlertManager → Kubernetes workflows
// Stakeholder Value: Executive confidence in end-to-end alert processing and automated remediation

func TestWebhookE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webhook Service E2E Business Suite")
}
