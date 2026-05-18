package requestid

import "net/http"

// Transport is an http.RoundTripper that injects the X-Request-ID header
// from the request's context into outbound HTTP calls (DD-005 correlation).
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip injects X-Request-ID from the request context before delegating.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if rid := FromContext(req.Context()); rid != "" {
		req = req.Clone(req.Context())
		req.Header.Set("X-Request-ID", rid)
	}
	return t.base().RoundTrip(req)
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}
