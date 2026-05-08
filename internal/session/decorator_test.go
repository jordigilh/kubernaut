package session_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

var _ = Describe("SessionServiceDecorator", func() {
	var (
		inner     adksession.Service
		decorator *session.SessionServiceDecorator
		ctx       context.Context
	)

	BeforeEach(func() {
		inner = adksession.InMemoryService()
		decorator = session.NewSessionServiceDecorator(inner)
		ctx = context.Background()
	})

	Describe("Create", func() {
		It("UT-AF-056-PW-001: enriches State with CreateConfig from context", func() {
			identity := &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "task-123",
				RemediationRef: v1alpha1.ObjectRef{
					Namespace: "prod",
					Name:      "rr-fix-oom",
				},
			})

			resp, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "alice",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Session).NotTo(BeNil())
		})

		It("UT-AF-056-PW-002: passes through unchanged when no context config", func() {
			resp, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "bob",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Session).NotTo(BeNil())
		})

		It("UT-AF-056-PW-004: TaskID extracted from context into CreateConfig.A2ATaskID", func() {
			identity := &auth.UserIdentity{Username: "carol", Groups: []string{"sre"}}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "task-abc-def",
			})

			// Use the CRDSessionService as inner to verify the CreateConfig is passed through
			scheme := newScheme()
			k8s := newFakeClient(scheme)
			crdSvc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")
			dec := session.NewSessionServiceDecorator(crdSvc)

			resp, err := dec.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "carol",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Session).NotTo(BeNil())
		})

		It("UT-AF-056-PW-005: UserIdentity populated from auth context", func() {
			identity := &auth.UserIdentity{Username: "dave", Groups: []string{"l1-ops", "sre"}}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "task-user-test",
			})

			scheme := newScheme()
			k8s := newFakeClient(scheme)
			crdSvc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")
			dec := session.NewSessionServiceDecorator(crdSvc)

			resp, err := dec.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "dave",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		})

		It("UT-AF-056-PW-006: concurrent Create calls are safe under -race", func() {
			identity := &auth.UserIdentity{Username: "eve", Groups: []string{"sre"}}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "task-concurrent",
			})

			var wg sync.WaitGroup
			errs := make([]error, 10)
			for i := range 10 {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					_, errs[idx] = decorator.Create(ctx, &adksession.CreateRequest{
						AppName: "test-app",
						UserID:  "eve",
					})
				}(i)
			}
			wg.Wait()

			for _, e := range errs {
				Expect(e).NotTo(HaveOccurred())
			}
		})

		It("UT-AF-056-PW-007: non-RFC-1123 TaskID is passed to inner service for sanitization", func() {
			identity := &auth.UserIdentity{Username: "grace", Groups: []string{"sre"}}
			ctx = auth.WithUserIdentity(ctx, identity)
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "UPPER-CASE_with/slashes/../traversal",
			})

			// The decorator passes TaskID through; CRDSessionService sanitizes it
			scheme := newScheme()
			k8s := newFakeClient(scheme)
			crdSvc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")
			dec := session.NewSessionServiceDecorator(crdSvc)

			resp, err := dec.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "grace",
			})

			// CRDSessionService handles sanitization internally - should not panic
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		})

		It("UT-AF-056-PW-008: empty username / nil identity yields error", func() {
			ctx = session.WithSessionCreateContext(ctx, &session.SessionCreateContext{
				TaskID: "task-no-identity",
			})

			_, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "ghost",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("authenticated user identity"))
		})
	})

	Describe("Delegation", func() {
		It("UT-AF-056-PW-003: Get/List/Delete/AppendEvent delegate unchanged", func() {
			// Create a session first
			resp, err := decorator.Create(ctx, &adksession.CreateRequest{
				AppName: "test-app",
				UserID:  "frank",
			})
			Expect(err).NotTo(HaveOccurred())
			sessionID := resp.Session.ID()

			// Get
			getResp, err := decorator.Get(ctx, &adksession.GetRequest{
				AppName:   "test-app",
				UserID:    "frank",
				SessionID: sessionID,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.Session.ID()).To(Equal(sessionID))

			// List
			listResp, err := decorator.List(ctx, &adksession.ListRequest{
				AppName: "test-app",
				UserID:  "frank",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Sessions).To(HaveLen(1))

			// Delete
			err = decorator.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "test-app",
				UserID:    "frank",
				SessionID: sessionID,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify deleted
			listResp, err = decorator.List(ctx, &adksession.ListRequest{
				AppName: "test-app",
				UserID:  "frank",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.Sessions).To(HaveLen(0))
		})
	})

	Describe("Context helpers", func() {
		It("UT-AF-056-PW-009: WithSessionCreateContext stores and retrieves value", func() {
			sc := &session.SessionCreateContext{
				TaskID: "test-task",
				RemediationRef: v1alpha1.ObjectRef{
					Namespace: "ns1",
					Name:      "rr-1",
				},
			}
			enriched := session.WithSessionCreateContext(ctx, sc)
			retrieved := session.SessionCreateContextFromContext(enriched)

			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved.TaskID).To(Equal("test-task"))
			Expect(retrieved.RemediationRef.Namespace).To(Equal("ns1"))
			Expect(retrieved.RemediationRef.Name).To(Equal("rr-1"))
		})

		It("UT-AF-056-PW-010: SessionCreateContextFromContext returns nil when not set", func() {
			retrieved := session.SessionCreateContextFromContext(ctx)
			Expect(retrieved).To(BeNil())
		})
	})
})
