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
	"encoding/json"
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

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
		It("UT-SCHEMA-2582-001 [BR-AI-009, SI-10]: preserves the AgentSession recovery identity", func() {
			crd := readEmbeddedCRD("kubernaut.ai_aianalyses.yaml")
			agentSessionRef := schemaAt(crd, "status", "investigationSession", "agentSessionRef")

			Expect(agentSessionRef.Type).To(Equal("object"))
			Expect(agentSessionRef.Properties).To(HaveKey("apiVersion"))
			Expect(agentSessionRef.Properties).To(HaveKey("kind"))
			Expect(agentSessionRef.Properties).To(HaveKey("name"))
			Expect(agentSessionRef.Properties).To(HaveKey("namespace"))
			Expect(agentSessionRef.Properties).To(HaveKey("uid"))
			Expect(agentSessionRef.XMapType).NotTo(BeNil())
			Expect(*agentSessionRef.XMapType).To(Equal("atomic"))
		})

		It("UT-SCHEMA-2582-002 [BR-WE-014, BR-ORCH-026, BR-ORCH-036, SI-10]: includes each lifecycle enum in its owning field", func() {
			for _, testCase := range []struct {
				file   string
				path   []string
				values []string
			}{
				{file: "kubernaut.ai_aianalyses.yaml", path: []string{"status", "phase"}, values: []string{"Cancelled"}},
				{file: "kubernaut.ai_effectivenessassessments.yaml", path: []string{"status", "phase"}, values: []string{"Cancelled"}},
				{file: "kubernaut.ai_remediationapprovalrequests.yaml", path: []string{"status", "decision"}, values: []string{"Cancelled"}},
				{file: "kubernaut.ai_signalprocessings.yaml", path: []string{"status", "phase"}, values: []string{"Cancelled"}},
				{file: "kubernaut.ai_workflowexecutions.yaml", path: []string{"status", "phase"}, values: []string{"Cancelling", "Cancelled"}},
			} {
				field := schemaAt(readEmbeddedCRD(testCase.file), testCase.path...)
				for _, value := range testCase.values {
					expectEnum(field, value)
				}
			}

			failurePhase := schemaAt(readEmbeddedCRD("kubernaut.ai_remediationrequests.yaml"), "status", "completionStatus", "failurePhase")
			Expect(failurePhase.Type).To(Equal("string"))
			Expect(failurePhase.AllOf).To(HaveLen(2))
			for constraintIndex, constraint := range failurePhase.AllOf {
				Expect(enumValues(constraint)).To(ContainElement("EffectivenessAssessment"),
					"failurePhase allOf constraint %d must include EffectivenessAssessment", constraintIndex)
			}
		})
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
				crd := readEmbeddedCRD(e.Name())
				Expect(crd.Kind).To(Equal("CustomResourceDefinition"), "CRD file %s", e.Name())
				Expect(crd.Spec.Group).To(Equal("kubernaut.ai"), "CRD file %s", e.Name())
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

func readEmbeddedCRD(name string) apixv1.CustomResourceDefinition {
	data, err := fs.ReadFile(assets.CRDsFS, "crds/"+name)
	Expect(err).NotTo(HaveOccurred(), "should read CRD file %s", name)
	Expect(data).NotTo(BeEmpty(), "CRD file %s should not be empty", name)

	var crd apixv1.CustomResourceDefinition
	Expect(yaml.Unmarshal(data, &crd)).To(Succeed(), "CRD file %s should parse", name)
	Expect(crd.APIVersion).To(Equal("apiextensions.k8s.io/v1"), "CRD file %s", name)
	Expect(crd.Spec.Versions).NotTo(BeEmpty(), "CRD file %s should define a version", name)
	Expect(crd.Spec.Versions[0].Schema).NotTo(BeNil(), "CRD file %s should define an OpenAPI schema", name)
	Expect(crd.Spec.Versions[0].Schema.OpenAPIV3Schema).NotTo(BeNil(), "CRD file %s should define an OpenAPI root", name)
	return crd
}

func schemaAt(crd apixv1.CustomResourceDefinition, path ...string) apixv1.JSONSchemaProps {
	schema := *crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	for _, field := range path {
		child, found := schema.Properties[field]
		Expect(found).To(BeTrue(), "schema path %q should exist in %s", field, crd.Name)
		schema = child
	}
	return schema
}

func enumValues(schema apixv1.JSONSchemaProps) []string {
	values := make([]string, 0, len(schema.Enum))
	for _, value := range schema.Enum {
		var decoded string
		Expect(json.Unmarshal(value.Raw, &decoded)).To(Succeed())
		values = append(values, decoded)
	}
	return values
}

func expectEnum(schema apixv1.JSONSchemaProps, value string) {
	Expect(enumValues(schema)).To(ContainElement(value))
}
