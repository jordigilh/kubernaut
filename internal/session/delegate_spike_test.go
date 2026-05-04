package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// delegatingService wraps InMemoryService to prove the delegate pattern.
// In production this becomes CRDSessionService (PR4).
type delegatingService struct {
	delegate session.Service
	creates  int
	appends  int
}

func newDelegatingService() *delegatingService {
	return &delegatingService{delegate: session.InMemoryService()}
}

func (s *delegatingService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	s.creates++
	return s.delegate.Create(ctx, req)
}

func (s *delegatingService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	return s.delegate.Get(ctx, req)
}

func (s *delegatingService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	return s.delegate.List(ctx, req)
}

func (s *delegatingService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	return s.delegate.Delete(ctx, req)
}

func (s *delegatingService) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	s.appends++
	return s.delegate.AppendEvent(ctx, sess, event)
}

var _ session.Service = (*delegatingService)(nil)

var _ = Describe("ADK Session delegate pattern spike", func() {
	var (
		svc *delegatingService
		ctx context.Context
	)

	BeforeEach(func() {
		svc = newDelegatingService()
		ctx = context.Background()
	})

	It("SPIKE-001: delegate Create returns a Session usable by AppendEvent", func() {
		resp, err := svc.Create(ctx, &session.CreateRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "test-user",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Session).NotTo(BeNil())
		Expect(resp.Session.AppName()).To(Equal("kubernaut-apifrontend"))
		Expect(resp.Session.UserID()).To(Equal("test-user"))

		event := session.NewEvent("inv-1")
		event.Author = "kubernaut-apifrontend"
		event.Content = genai.NewContentFromText("test response", genai.RoleModel)

		err = svc.AppendEvent(ctx, resp.Session, event)
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.appends).To(Equal(1))
	})

	It("SPIKE-002: delegate Get returns a Session with events", func() {
		createResp, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "test-user",
			SessionID: "sess-1",
		})
		Expect(err).NotTo(HaveOccurred())

		event := session.NewEvent("inv-1")
		event.Author = "kubernaut-apifrontend"
		event.Content = genai.NewContentFromText("test response", genai.RoleModel)
		err = svc.AppendEvent(ctx, createResp.Session, event)
		Expect(err).NotTo(HaveOccurred())

		getResp, err := svc.Get(ctx, &session.GetRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "test-user",
			SessionID: "sess-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(getResp.Session.Events().Len()).To(Equal(1))
	})

	It("SPIKE-003: delegate List returns sessions", func() {
		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "test-user",
		})
		Expect(err).NotTo(HaveOccurred())
		_, err = svc.Create(ctx, &session.CreateRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "test-user",
		})
		Expect(err).NotTo(HaveOccurred())

		listResp, err := svc.List(ctx, &session.ListRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "test-user",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.Sessions).To(HaveLen(2))
	})

	It("SPIKE-004: delegate Delete removes session", func() {
		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "test-user",
			SessionID: "sess-del",
		})
		Expect(err).NotTo(HaveOccurred())

		err = svc.Delete(ctx, &session.DeleteRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "test-user",
			SessionID: "sess-del",
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.Get(ctx, &session.GetRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "test-user",
			SessionID: "sess-del",
		})
		Expect(err).To(HaveOccurred())
	})

	It("SPIKE-005: delegate tracks side-effects alongside InMemoryService ops", func() {
		_, err := svc.Create(ctx, &session.CreateRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "test-user",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(svc.creates).To(Equal(1))
	})
})
