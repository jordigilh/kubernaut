package launcher

import (
	"context"
	"net/http"

	"github.com/a2aproject/a2a-go/a2a"
	"google.golang.org/adk/server/adka2a"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// EnrichRRDetailForTest exports enrichRRDetail for unit testing.
func EnrichRRDetailForTest(ctx context.Context, detail map[string]string) {
	enrichRRDetail(ctx, detail)
}

// PartConverterFunc is the exported signature for testing the part converter.
type PartConverterFunc func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error)

// BuildPartConverterForTest exports buildPartConverter for unit testing.
func BuildPartConverterForTest() PartConverterFunc {
	fn := buildPartConverter()
	return func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error) {
		return fn(ctx, adkEvent, part)
	}
}

// BuildStreamingPartConverterForTest exports buildStreamingPartConverter for unit testing.
func BuildStreamingPartConverterForTest() PartConverterFunc {
	fn := buildStreamingPartConverter()
	return func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error) {
		return fn(ctx, adkEvent, part)
	}
}

// BuiltConverterIsNonNil verifies buildPartConverter returns a non-nil function.
func BuiltConverterIsNonNil() bool {
	return buildPartConverter() != nil
}

// ExpectedOutputMode returns the OutputMode constant wired in the ExecutorConfig.
func ExpectedOutputMode() adka2a.OutputMode {
	return adka2a.OutputArtifactPerEvent
}

// BuildTransportChainForTest exports buildTransportChain for unit testing.
func BuildTransportChainForTest(cfg config.LLMConfig) (http.RoundTripper, error) {
	return buildTransportChain(cfg)
}

// BuildLLMHTTPClientForTest exports buildLLMHTTPClient for unit testing.
func BuildLLMHTTPClientForTest(cfg config.LLMConfig) (*http.Client, error) {
	return buildLLMHTTPClient(cfg)
}

// SanitizeBridgeTextForTest exports sanitizeBridgeText for unit testing.
func SanitizeBridgeTextForTest(text string) string {
	return sanitizeBridgeText(text)
}

// ResolveA2AMethodForTest exports resolveA2AMethod for unit testing.
func ResolveA2AMethodForTest(ctx context.Context) string {
	return resolveA2AMethod(ctx)
}
