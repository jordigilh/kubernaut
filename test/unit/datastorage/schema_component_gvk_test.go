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

package datastorage

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// TP-1051: ValidateMandatoryLabels GVK component format validation
// ========================================
// Authority: BR-WORKFLOW-004 (workflow schema format), Issue #1051 (GVK component labels)
// Tests that ValidateMandatoryLabels correctly accepts GVK-formatted component
// values and rejects plain-kind or malformed formats at the registration boundary.
// ========================================

var _ = Describe("TP-1051: ValidateMandatoryLabels component GVK format validation", func() {

	validLabels := func(component []string) *models.WorkflowSchemaLabels {
		return &models.WorkflowSchemaLabels{
			Severity:    []string{"critical"},
			Component:   component,
			Environment: []string{"production"},
			Priority:    "P1",
		}
	}

	Context("UT-DS-1051-001: valid GVK component values are accepted", func() {
		DescribeTable("should accept well-formed GVK component strings",
			func(component string) {
				labels := validLabels([]string{component})
				Expect(labels.ValidateMandatoryLabels()).ToNot(HaveOccurred(),
					"Issue #1051: valid component %q must be accepted", component)
			},
			Entry("core group: v1/Pod", "v1/Pod"),
			Entry("core group: v1/Node", "v1/Node"),
			Entry("core group: v1/Service", "v1/Service"),
			Entry("named group: apps/v1/Deployment", "apps/v1/Deployment"),
			Entry("named group: apps/v1/StatefulSet", "apps/v1/StatefulSet"),
			Entry("named group: apps/v1/DaemonSet", "apps/v1/DaemonSet"),
			Entry("named group: batch/v1/Job", "batch/v1/Job"),
			Entry("named group: batch/v1/CronJob", "batch/v1/CronJob"),
			Entry("named group: policy/v1/PodDisruptionBudget", "policy/v1/PodDisruptionBudget"),
			Entry("CRD: networking.k8s.io/v1/NetworkPolicy", "networking.k8s.io/v1/NetworkPolicy"),
			Entry("CRD: route.openshift.io/v1/Route", "route.openshift.io/v1/Route"),
			Entry("wildcard", "*"),
		)
	})

	Context("UT-DS-1051-002: plain-kind component values are rejected", func() {
		DescribeTable("should reject plain-kind component strings without apiVersion",
			func(component string) {
				labels := validLabels([]string{component})
				err := labels.ValidateMandatoryLabels()
				Expect(err).To(HaveOccurred(),
					"Issue #1051: plain-kind component %q must be rejected", component)
				Expect(err.Error()).To(ContainSubstring("must be in GVK format"),
					"Issue #1051: error message must guide operator to correct format")
			},
			Entry("lowercase deployment", "deployment"),
			Entry("PascalCase Pod (no apiVersion)", "Pod"),
			Entry("lowercase node", "node"),
			Entry("lowercase statefulset", "statefulset"),
		)
	})

	Context("UT-DS-1051-003: malformed component values are rejected", func() {
		DescribeTable("should reject structurally invalid component strings",
			func(component, reason string) {
				labels := validLabels([]string{component})
				err := labels.ValidateMandatoryLabels()
				Expect(err).To(HaveOccurred(),
					"Issue #1051: %s — component %q must be rejected", reason, component)
			},
			Entry("single segment", "Deployment", "single segment without slash"),
			Entry("4+ segments", "apps/v1/extra/Deployment", "too many segments"),
			Entry("lowercase kind after slash", "apps/v1/deployment", "kind must start uppercase"),
			Entry("empty kind segment", "apps/v1/", "empty kind after trailing slash"),
			Entry("empty string in array", "", "empty string component"),
			Entry("only slashes", "///", "only delimiter characters"),
			Entry("kind starts with number", "v1/1Pod", "kind starts with digit"),
		)
	})

	Context("UT-DS-1051-004: mixed valid and invalid components", func() {
		It("should reject when any component in the array is invalid (Issue #1051)", func() {
			labels := validLabels([]string{"apps/v1/Deployment", "pod"})
			err := labels.ValidateMandatoryLabels()
			Expect(err).To(HaveOccurred(),
				"Issue #1051: array with any invalid component must be rejected")
			Expect(err.Error()).To(ContainSubstring("pod"))
		})
	})

	Context("UT-DS-1051-005: wildcard alongside GVK values", func() {
		It("should accept wildcard mixed with valid GVK values (Issue #1051)", func() {
			labels := validLabels([]string{"*", "apps/v1/Deployment"})
			Expect(labels.ValidateMandatoryLabels()).ToNot(HaveOccurred(),
				"Issue #1051: wildcard alongside valid GVK must be accepted")
		})
	})
})
