package policyread

import (
	"errors"
	"io"
	"net/http"

	"k8s.io/client-go/rest"
)

// ErrResponseBodyLimitExceeded indicates that a trusted HTTPS response was
// larger than the configured client-side decode budget.
var ErrResponseBodyLimitExceeded = errors.New("response body exceeds configured limit")

// NewResponseLimitedTransport wraps an existing HTTP transport without
// changing its scheme, TLS settings, authentication, or proxy behavior. The
// limit is enforced while the decrypted response body is read, before
// client-go decodes the payload.
func NewResponseLimitedTransport(base http.RoundTripper, maxBytes int64) http.RoundTripper {
	if base == nil || maxBytes <= 0 {
		return base
	}
	return &responseLimitedTransport{base: base, maxBytes: maxBytes}
}

// WithResponseBodyLimit returns a copy of config with a response-body limiter
// layered on the existing REST transport. Existing WrapTransport behavior is
// preserved, and the REST config continues to use its original HTTPS/TLS
// endpoint and credentials.
func WithResponseBodyLimit(config *rest.Config, maxBytes int64) *rest.Config {
	if config == nil || maxBytes <= 0 {
		return config
	}
	bounded := rest.CopyConfig(config)
	previous := bounded.WrapTransport
	bounded.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		if previous != nil {
			base = previous(base)
		}
		return NewResponseLimitedTransport(base, maxBytes)
	}
	return bounded
}

type responseLimitedTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *responseLimitedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &responseLimitedBody{ReadCloser: response.Body, remaining: t.maxBytes}
	return response, nil
}

type responseLimitedBody struct {
	io.ReadCloser
	remaining int64
}

func (b *responseLimitedBody) Read(buffer []byte) (int, error) {
	if b.remaining <= 0 {
		var extra [1]byte
		n, err := b.ReadCloser.Read(extra[:])
		if n > 0 {
			b.remaining = -1
			return 0, ErrResponseBodyLimitExceeded
		}
		return 0, err
	}

	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	n, err := b.ReadCloser.Read(buffer)
	b.remaining -= int64(n)
	if b.remaining == 0 && err == nil {
		var extra [1]byte
		extraN, extraErr := b.ReadCloser.Read(extra[:])
		if extraN > 0 {
			b.remaining = -1
			return n, ErrResponseBodyLimitExceeded
		}
		if extraErr != nil && !errors.Is(extraErr, io.EOF) {
			return n, extraErr
		}
		if errors.Is(extraErr, io.EOF) {
			return n, io.EOF
		}
	}
	return n, err
}
