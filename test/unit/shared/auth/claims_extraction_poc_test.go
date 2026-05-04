package auth_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("PoC: Dot-notation claim extraction — Keycloak-realistic payloads", func() {

	// Standard Keycloak JWT payload (simplified)
	keycloakStandard := map[string]interface{}{
		"sub":                "f:12345-abcde:jsmith",
		"preferred_username": "jsmith@corp.com",
		"email":              "jsmith@corp.com",
		"groups":             []interface{}{"sre-team", "platform-eng", "kubernaut-interactive-users"},
		"realm_access": map[string]interface{}{
			"roles": []interface{}{"offline_access", "uma_authorization", "kubernaut-user"},
		},
		"resource_access": map[string]interface{}{
			"kubernaut-agent": map[string]interface{}{
				"roles": []interface{}{"interactive-user", "investigate"},
			},
			"account": map[string]interface{}{
				"roles": []interface{}{"manage-account", "view-profile"},
			},
		},
		"iss": "https://sso.example.com/realms/kubernaut",
		"aud": []interface{}{"kubernaut-agent", "account"},
	}

	Describe("PoC-CLAIMS-001: Top-level string claim", func() {
		It("should extract preferred_username from top level", func() {
			val, err := auth.ExtractStringClaim(keycloakStandard, "preferred_username")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("jsmith@corp.com"))
		})
	})

	Describe("PoC-CLAIMS-002: Top-level array claim (groups)", func() {
		It("should extract groups as string slice", func() {
			val, err := auth.ExtractGroupsClaim(keycloakStandard, "groups")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(ConsistOf("sre-team", "platform-eng", "kubernaut-interactive-users"))
		})
	})

	Describe("PoC-CLAIMS-003: Nested claim — realm_access.roles", func() {
		It("should extract roles from nested realm_access", func() {
			val, err := auth.ExtractGroupsClaim(keycloakStandard, "realm_access.roles")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(ConsistOf("offline_access", "uma_authorization", "kubernaut-user"))
		})
	})

	Describe("PoC-CLAIMS-004: Double-nested claim — resource_access.kubernaut-agent.roles", func() {
		It("should extract roles from double-nested resource_access", func() {
			val, err := auth.ExtractGroupsClaim(keycloakStandard, "resource_access.kubernaut-agent.roles")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(ConsistOf("interactive-user", "investigate"))
		})
	})

	Describe("PoC-CLAIMS-005: Missing intermediate key", func() {
		It("should return error for missing intermediate map", func() {
			_, err := auth.ExtractStringClaim(keycloakStandard, "nonexistent.field")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("PoC-CLAIMS-006: Key exists but wrong type (string instead of array)", func() {
		It("should return error when expecting array but got string", func() {
			_, err := auth.ExtractGroupsClaim(keycloakStandard, "preferred_username")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected array"))
		})
	})

	Describe("PoC-CLAIMS-007: Key exists but wrong type (array instead of string)", func() {
		It("should return error when expecting string but got array", func() {
			_, err := auth.ExtractStringClaim(keycloakStandard, "groups")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected string"))
		})
	})

	Describe("PoC-CLAIMS-008: Intermediate is not a map", func() {
		It("should return error when traversing through non-map value", func() {
			_, err := auth.ExtractStringClaim(keycloakStandard, "preferred_username.nested")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not a map"))
		})
	})

	Describe("PoC-CLAIMS-009: Empty claims map", func() {
		It("should return error for any path on empty claims", func() {
			_, err := auth.ExtractStringClaim(map[string]interface{}{}, "sub")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("PoC-CLAIMS-010: Non-string element in groups array", func() {
		It("should return error when array contains non-string", func() {
			claims := map[string]interface{}{
				"groups": []interface{}{"valid-group", 42, "another-group"},
			}
			_, err := auth.ExtractGroupsClaim(claims, "groups")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("element 1"))
			Expect(err.Error()).To(ContainSubstring("expected string"))
		})
	})

	Describe("PoC-CLAIMS-011: sub claim (standard OIDC)", func() {
		It("should extract sub from top level", func() {
			val, err := auth.ExtractStringClaim(keycloakStandard, "sub")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("f:12345-abcde:jsmith"))
		})
	})
})
