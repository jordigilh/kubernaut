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

package assets_test

import (
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/assets"
)

// DD-4 (Issue #578): Shared assets via Go embed package for the kubernaut-operator.
var _ = Describe("Shared Embedded Assets", func() {

	Context("MigrationsFS", func() {
		It("UT-SHARED-ASSETS-001: contains at least one .sql migration file", func() {
			entries, err := fs.ReadDir(assets.MigrationsFS, "migrations")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty(), "MigrationsFS should contain migration files")

			var sqlFiles []string
			for _, e := range entries {
				if !e.IsDir() {
					sqlFiles = append(sqlFiles, e.Name())
				}
			}
			Expect(sqlFiles).NotTo(BeEmpty(), "MigrationsFS should contain .sql files")
		})

		It("UT-SHARED-ASSETS-002: 001_v1_schema.sql is readable and non-empty", func() {
			data, err := fs.ReadFile(assets.MigrationsFS, "migrations/001_v1_schema.sql")
			Expect(err).NotTo(HaveOccurred())
			Expect(data).NotTo(BeEmpty(), "001_v1_schema.sql should not be empty")
			Expect(string(data)).To(ContainSubstring("CREATE"), "migration should contain SQL DDL")
		})
	})

	Context("CRDsFS", func() {
		It("UT-SHARED-ASSETS-003: contains kubernaut.ai CRD YAML files", func() {
			entries, err := fs.ReadDir(assets.CRDsFS, "crds")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty(), "CRDsFS should contain CRD files")

			var yamlFiles []string
			for _, e := range entries {
				if !e.IsDir() {
					yamlFiles = append(yamlFiles, e.Name())
				}
			}
			Expect(yamlFiles).NotTo(BeEmpty(), "CRDsFS should contain .yaml files")
		})

		It("UT-SHARED-ASSETS-004: each CRD YAML is a valid CustomResourceDefinition", func() {
			entries, err := fs.ReadDir(assets.CRDsFS, "crds")
			Expect(err).NotTo(HaveOccurred())

			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				data, err := fs.ReadFile(assets.CRDsFS, "crds/"+e.Name())
				Expect(err).NotTo(HaveOccurred(), "should read CRD file %s", e.Name())
				Expect(data).NotTo(BeEmpty(), "CRD file %s should not be empty", e.Name())
				Expect(string(data)).To(ContainSubstring("kind: CustomResourceDefinition"),
					"CRD file %s should contain kind: CustomResourceDefinition", e.Name())
				Expect(string(data)).To(ContainSubstring("kubernaut.ai"),
					"CRD file %s should belong to kubernaut.ai domain", e.Name())
			}
		})

		It("UT-SHARED-ASSETS-005: contains all 9 expected CRDs", func() {
			expectedCRDs := []string{
				"kubernaut.ai_actiontypes.yaml",
				"kubernaut.ai_aianalyses.yaml",
				"kubernaut.ai_effectivenessassessments.yaml",
				"kubernaut.ai_notificationrequests.yaml",
				"kubernaut.ai_remediationapprovalrequests.yaml",
				"kubernaut.ai_remediationrequests.yaml",
				"kubernaut.ai_remediationworkflows.yaml",
				"kubernaut.ai_signalprocessings.yaml",
				"kubernaut.ai_workflowexecutions.yaml",
			}

			for _, name := range expectedCRDs {
				_, err := fs.ReadFile(assets.CRDsFS, "crds/"+name)
				Expect(err).NotTo(HaveOccurred(), "CRD %s should be embedded", name)
			}
		})
	})
})
