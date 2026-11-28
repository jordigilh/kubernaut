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

package infrastructure

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// UT-DOC-2716-001 [BR-SECURITY-001, AC-6]: the operator-facing
// operation -> capability grant catalog in the AF security model must stay in
// sync with the harness fixture tool contract (LoadPersonaTools). The fixture
// contract is itself pinned to the registered tool set by TC-A-RBAC-01, so
// this keeps the documentation answerable to "what do I grant for operation
// X" without drift (#2716: operators compose roles from these grants).
var _ = Describe("AF operation grant catalog documentation (UT-DOC-2716-001)", func() {
	const catalogPath = "docs/services/apifrontend/security/AUTHENTICATION_AND_RBAC.md"

	docCatalogToolNames := func() map[string]bool {
		raw, err := os.ReadFile(filepath.Join(getProjectRoot(), catalogPath))
		Expect(err).NotTo(HaveOccurred(), "AF security model doc must exist")
		content := string(raw)
		start := strings.Index(content, "<!-- tool-catalog:start -->")
		end := strings.Index(content, "<!-- tool-catalog:end -->")
		Expect(start).To(BeNumerically(">", -1), "catalog start marker missing in %s", catalogPath)
		Expect(end).To(BeNumerically(">", start), "catalog end marker missing in %s", catalogPath)
		section := content[start:end]

		// Every backticked token in the catalog section is a grant name.
		tokenRe := regexp.MustCompile("`([a-z_]+)`")
		names := map[string]bool{}
		for _, match := range tokenRe.FindAllStringSubmatch(section, -1) {
			names[match[1]] = true
		}
		Expect(names).NotTo(BeEmpty(), "catalog section must list grant names")
		return names
	}

	It("lists every tool in the fixture role contract as a grantable operation", func() {
		personaTools, err := LoadPersonaTools()
		Expect(err).NotTo(HaveOccurred(), "fixture role contract must load")
		docNames := docCatalogToolNames()

		var missing []string
		for _, tools := range personaTools {
			for _, tool := range tools {
				if !docNames[tool] {
					missing = append(missing, tool)
				}
			}
		}
		sort.Strings(missing)
		Expect(missing).To(BeEmpty(),
			"the operation catalog in %s must document every grantable tool (missing: %s) — operators compose roles from this catalog (AC-6)",
			catalogPath, strings.Join(missing, ", "))
	})

	It("documents no grant that is absent from the fixture role contract", func() {
		personaTools, err := LoadPersonaTools()
		Expect(err).NotTo(HaveOccurred())
		known := map[string]bool{}
		for _, tools := range personaTools {
			for _, tool := range tools {
				known[tool] = true
			}
		}
		docNames := docCatalogToolNames()

		var unknown []string
		for name := range docNames {
			if !known[name] {
				unknown = append(unknown, name)
			}
		}
		sort.Strings(unknown)
		Expect(unknown).To(BeEmpty(),
			"the operation catalog in %s references grants that are not part of the tool contract (unknown: %s)",
			catalogPath, strings.Join(unknown, ", "))
	})
})
