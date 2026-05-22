package auth_test

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authorizationv1 "k8s.io/api/authorization/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

// rbacPolicy stores RBAC state outside the fake client to avoid reactor recursion.
type rbacPolicy struct {
	mu       sync.RWMutex
	roles    map[string]*rbacv1.ClusterRole
	bindings map[string]*rbacv1.ClusterRoleBinding
}

func newRBACPolicy() *rbacPolicy {
	return &rbacPolicy{
		roles:    make(map[string]*rbacv1.ClusterRole),
		bindings: make(map[string]*rbacv1.ClusterRoleBinding),
	}
}

func (p *rbacPolicy) addRole(role *rbacv1.ClusterRole) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.roles[role.Name] = role
}

func (p *rbacPolicy) addBinding(binding *rbacv1.ClusterRoleBinding) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bindings[binding.Name] = binding
}

func (p *rbacPolicy) removeBinding(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.bindings, name)
}

func (p *rbacPolicy) evaluate(sar *authorizationv1.SubjectAccessReview) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	userGroups := make(map[string]bool, len(sar.Spec.Groups))
	for _, g := range sar.Spec.Groups {
		userGroups[g] = true
	}

	for _, binding := range p.bindings {
		if !subjectMatchesPolicy(binding.Subjects, sar.Spec.User, userGroups) {
			continue
		}
		role, ok := p.roles[binding.RoleRef.Name]
		if !ok {
			continue
		}
		if roleGrantsAccess(role, sar.Spec.ResourceAttributes) {
			return true
		}
	}
	return false
}

func subjectMatchesPolicy(subjects []rbacv1.Subject, user string, groups map[string]bool) bool {
	for _, s := range subjects {
		if s.Kind == "User" && s.Name == user {
			return true
		}
		if s.Kind == "Group" && groups[s.Name] {
			return true
		}
	}
	return false
}

func roleGrantsAccess(role *rbacv1.ClusterRole, attrs *authorizationv1.ResourceAttributes) bool {
	if attrs == nil {
		return false
	}
	for _, rule := range role.Rules {
		if !matchesSlice(rule.APIGroups, attrs.Group) {
			continue
		}
		if !matchesSlice(rule.Resources, attrs.Resource) {
			continue
		}
		if !matchesSlice(rule.Verbs, attrs.Verb) {
			continue
		}
		if len(rule.ResourceNames) > 0 && !containsString(rule.ResourceNames, attrs.Name) {
			continue
		}
		return true
	}
	return false
}

func matchesSlice(allowed []string, value string) bool {
	for _, a := range allowed {
		if a == "*" || a == value {
			return true
		}
	}
	return false
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// sarPolicyReactor returns a reactor that evaluates SAR using the in-memory RBAC policy.
func sarPolicyReactor(policy *rbacPolicy) func(k8stesting.Action) (bool, runtime.Object, error) {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		sar := createAction.GetObject().(*authorizationv1.SubjectAccessReview)
		allowed := policy.evaluate(sar)
		sar.Status = authorizationv1.SubjectAccessReviewStatus{
			Allowed: allowed,
			Reason:  "simulated RBAC evaluation",
		}
		return true, sar, nil
	}
}

func addClusterRole(policy *rbacPolicy, name string, rules []rbacv1.PolicyRule) {
	policy.addRole(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Rules:      rules,
	})
}

func addClusterRoleBinding(policy *rbacPolicy, name, roleName, groupName string) {
	policy.addBinding(&rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{Kind: "Group", Name: groupName, APIGroup: "rbac.authorization.k8s.io"},
		},
	})
}

var _ = Describe("SARChecker Integration", func() {
	var (
		ctx     context.Context
		fakeK8s *k8sfake.Clientset
		checker *auth.SARChecker
		policy  *rbacPolicy
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeK8s = k8sfake.NewSimpleClientset()
		policy = newRBACPolicy()
		fakeK8s.PrependReactor("create", "subjectaccessreviews", sarPolicyReactor(policy))
	})

	Describe("IT-AF-1221-001: ClusterRole + ClusterRoleBinding grants tool access via SAR", func() {
		It("should allow access when ClusterRole and binding exist for user's group", func() {
			addClusterRole(policy, "kubernaut-tool-sre", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"},
					Resources: []string{"tools"},
					Verbs:     []string{"use"},
				},
			})
			addClusterRoleBinding(policy, "sre-binding", "kubernaut-tool-sre", "sre")

			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			allowed, err := checker.Check(ctx, "alice@corp.com", []string{"sre", "system:authenticated"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue(), "AC 1, AC 6: SRE ClusterRole should grant access to all tools")
		})
	})

	Describe("IT-AF-1221-002: Missing ClusterRoleBinding denies tool access", func() {
		It("should deny access when no binding exists for user's groups", func() {
			addClusterRole(policy, "kubernaut-tool-sre", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"},
					Resources: []string{"tools"},
					Verbs:     []string{"use"},
				},
			})

			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())
			allowed, err := checker.Check(ctx, "alice@corp.com", []string{"viewer"}, "kubernaut_approve")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse(), "AC 1, AC 5: missing binding should deny access")
		})
	})

	Describe("IT-AF-1221-003: Multi-group membership grants union of tools", func() {
		It("should allow access when any of the user's groups has a matching binding", func() {
			addClusterRole(policy, "kubernaut-tool-cicd", []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"kubernaut.ai"},
					Resources:     []string{"tools"},
					Verbs:         []string{"use"},
					ResourceNames: []string{"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch"},
				},
			})
			addClusterRoleBinding(policy, "cicd-binding", "kubernaut-tool-cicd", "cicd")

			addClusterRole(policy, "kubernaut-tool-audit", []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"kubernaut.ai"},
					Resources:     []string{"tools"},
					Verbs:         []string{"use"},
					ResourceNames: []string{"kubernaut_get_audit_trail", "kubernaut_get_effectiveness", "kubernaut_get_remediation_history"},
				},
			})
			addClusterRoleBinding(policy, "audit-binding", "kubernaut-tool-audit", "l3-audit")

			checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

			allowed1, err1 := checker.Check(ctx, "bob@corp.com", []string{"cicd", "l3-audit"}, "kubernaut_watch")
			Expect(err1).NotTo(HaveOccurred())
			Expect(allowed1).To(BeTrue(), "AC 3: cicd group grants kubernaut_watch")

			allowed2, err2 := checker.Check(ctx, "bob@corp.com", []string{"cicd", "l3-audit"}, "kubernaut_get_audit_trail")
			Expect(err2).NotTo(HaveOccurred())
			Expect(allowed2).To(BeTrue(), "AC 3: l3-audit group grants kubernaut_get_audit_trail")

			allowed3, err3 := checker.Check(ctx, "bob@corp.com", []string{"cicd", "l3-audit"}, "kubernaut_approve")
			Expect(err3).NotTo(HaveOccurred())
			Expect(allowed3).To(BeFalse(), "AC 3: neither group grants kubernaut_approve")
		})
	})

	Describe("IT-AF-1221-004: Cache TTL expiry reflects permission changes", func() {
		It("should reflect permission changes after cache TTL expires", func() {
			addClusterRole(policy, "kubernaut-tool-sre", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"},
					Resources: []string{"tools"},
					Verbs:     []string{"use"},
				},
			})
			addClusterRoleBinding(policy, "sre-binding", "kubernaut-tool-sre", "sre")

			checker = auth.NewSARChecker(fakeK8s, 100*time.Millisecond, logr.Discard())

			allowed1, err1 := checker.Check(ctx, "alice@corp.com", []string{"sre"}, "kubernaut_approve")
			Expect(err1).NotTo(HaveOccurred())
			Expect(allowed1).To(BeTrue(), "AC 4: initial check should be allowed")

			policy.removeBinding("sre-binding")

			Eventually(func() bool {
				a, _ := checker.Check(ctx, "alice@corp.com", []string{"sre"}, "kubernaut_approve")
				return a
			}, 1*time.Second, 50*time.Millisecond).Should(BeFalse(),
				"AC 4: after binding deletion and TTL expiry, access should be denied")
		})
	})

	Describe("IT-AF-1221-005: All 6 ClusterRoles grant correct tool subsets", func() {
		type roleTestCase struct {
			roleName    string
			group       string
			allowedTool string
			deniedTool  string
		}

		DescribeTable("should grant/deny correct tools per ClusterRole",
			func(tc roleTestCase) {
				addClusterRoleBinding(policy, tc.group+"-binding", tc.roleName, tc.group)
				checker = auth.NewSARChecker(fakeK8s, 30*time.Second, logr.Discard())

				if tc.allowedTool != "" {
					allowed, err := checker.Check(ctx, tc.group+"-user@corp.com", []string{tc.group}, tc.allowedTool)
					Expect(err).NotTo(HaveOccurred())
					Expect(allowed).To(BeTrue(), "AC 6: %s should grant %s", tc.roleName, tc.allowedTool)
				}
				if tc.deniedTool != "" {
					denied, err := checker.Check(ctx, tc.group+"-user@corp.com", []string{tc.group}, tc.deniedTool)
					Expect(err).NotTo(HaveOccurred())
					Expect(denied).To(BeFalse(), "AC 6: %s should deny %s", tc.roleName, tc.deniedTool)
				}
			},
			Entry("SRE: all tools", roleTestCase{
				roleName: "kubernaut-tool-sre", group: "sre",
				allowedTool: "kubernaut_approve", deniedTool: "",
			}),
			Entry("Orchestrator: orchestration tools, denied audit", roleTestCase{
				roleName: "kubernaut-tool-orchestrator", group: "ai-orchestrator",
				allowedTool: "kubernaut_start_investigation", deniedTool: "kubernaut_get_audit_trail",
			}),
			Entry("Approver: approve + read, denied create_rr", roleTestCase{
				roleName: "kubernaut-tool-approver", group: "remediation-approver",
				allowedTool: "kubernaut_approve", deniedTool: "af_create_rr",
			}),
			Entry("Viewer: list/get/watch + events, denied approve", roleTestCase{
				roleName: "kubernaut-tool-viewer", group: "observability",
				allowedTool: "kubernaut_list_remediations", deniedTool: "kubernaut_approve",
			}),
			Entry("CICD: list/get/watch, denied approve", roleTestCase{
				roleName: "kubernaut-tool-cicd", group: "cicd",
				allowedTool: "kubernaut_watch", deniedTool: "kubernaut_approve",
			}),
			Entry("Audit: history/effectiveness/audit, denied approve", roleTestCase{
				roleName: "kubernaut-tool-audit", group: "l3-audit",
				allowedTool: "kubernaut_get_audit_trail", deniedTool: "kubernaut_approve",
			}),
		)

		BeforeEach(func() {
			addClusterRole(policy, "kubernaut-tool-sre", []rbacv1.PolicyRule{
				{APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"}},
			})
			addClusterRole(policy, "kubernaut-tool-orchestrator", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"},
					ResourceNames: []string{
						"kubernaut_list_remediations", "kubernaut_get_remediation",
						"kubernaut_approve", "kubernaut_cancel_remediation", "kubernaut_watch",
						"kubernaut_start_investigation", "kubernaut_poll_investigation", "kubernaut_stream_investigation",
						"kubernaut_discover_workflows", "kubernaut_select_workflow", "present_decision",
					},
				},
			})
			addClusterRole(policy, "kubernaut-tool-approver", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"},
					ResourceNames: []string{
						"kubernaut_approve", "kubernaut_list_remediations",
						"kubernaut_get_remediation", "kubernaut_watch",
					},
				},
			})
			addClusterRole(policy, "kubernaut-tool-viewer", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"},
					ResourceNames: []string{
						"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch",
						"kubernaut_get_effectiveness", "kubernaut_list_workflows",
					},
				},
			})
			addClusterRole(policy, "kubernaut-tool-cicd", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"},
					ResourceNames: []string{
						"kubernaut_list_remediations", "kubernaut_get_remediation", "kubernaut_watch",
					},
				},
			})
			addClusterRole(policy, "kubernaut-tool-audit", []rbacv1.PolicyRule{
				{
					APIGroups: []string{"kubernaut.ai"}, Resources: []string{"tools"}, Verbs: []string{"use"},
					ResourceNames: []string{
						"kubernaut_list_remediations", "kubernaut_get_remediation",
						"kubernaut_list_workflows", "kubernaut_get_remediation_history",
						"kubernaut_get_effectiveness", "kubernaut_get_audit_trail",
					},
				},
			})
		})
	})
})
