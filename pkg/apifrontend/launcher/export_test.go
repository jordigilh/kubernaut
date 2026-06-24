package launcher

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/a2aproject/a2a-go/a2a"
	"google.golang.org/adk/server/adka2a"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
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
func BuildTransportChainForTest(cfg types.LLMConfig) (http.RoundTripper, error) {
	return buildTransportChain(cfg)
}

// BuildLLMHTTPClientForTest exports buildLLMHTTPClient for unit testing.
func BuildLLMHTTPClientForTest(cfg types.LLMConfig) (*http.Client, error) {
	return buildLLMHTTPClient(cfg)
}

// EnsureTrailingParagraphBreakForTest exports ensureTrailingParagraphBreak for unit testing.
func EnsureTrailingParagraphBreakForTest(s string) string {
	return ensureTrailingParagraphBreak(s)
}

// SanitizeBridgeTextForTest exports sanitizeBridgeText for unit testing.
func SanitizeBridgeTextForTest(text string) string {
	return sanitizeBridgeText(text)
}

// ResolveA2AMethodForTest exports resolveA2AMethod for unit testing.
func ResolveA2AMethodForTest(ctx context.Context) string {
	return resolveA2AMethod(ctx)
}

// LoggerForTest exports A2AConfig.logger for unit testing.
func LoggerForTest(cfg A2AConfig) logr.Logger {
	return cfg.logger()
}

// StreamingExecutorLoggerForTest returns the logger stored in a StreamingExecutor.
func StreamingExecutorLoggerForTest(se *StreamingExecutor) logr.Logger {
	return se.logger
}

// StripEmojiForTest exports stripEmoji for unit testing.
func StripEmojiForTest(s string) string {
	return stripEmoji(s)
}

// EmitArtifactForTest exports EmitArtifact via bridge from context for testing.
func EmitArtifactForTest(ctx context.Context, data map[string]any, textFallback string, meta map[string]any) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	return bridge.EmitArtifact(ctx, data, textFallback, meta)
}

// ValidatePayloadForTest exports ValidatePayload for testing.
func ValidatePayloadForTest(schemaName string, data map[string]any) error {
	return ValidatePayload(schemaName, data)
}
