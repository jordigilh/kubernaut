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

// Package geminifamily implements llm.Client for Gemini models across the
// Gemini model family's auth modes — Google Vertex AI (New) and the native
// Gemini API (NewWithAPIKey) — by wrapping eino-ext's actively-maintained
// agenticgemini leaf model component (DD-LLM-010), rather than
// reimplementing Gemini's request/response/tool/reasoning protocol
// in-house or depending on Google ADK's model/gemini (rejected in
// DD-LLM-010 for transitively coupling KA to ADK's agent/session/tool
// framework, violating DD-HAPI-019's Framework Isolation Pattern).
//
// No eino type is ever exposed outside this package — Chat/StreamChat
// translate to/from KA's own llm.ChatRequest/ChatResponse/Message types at
// this package's boundary (see conv.go), so
// internal/kubernautagent/investigator/* remains completely unaware of
// eino, exactly as it is unaware of the Anthropic SDK behind
// anthropicfamily.
//
// This package is deliberately separate from anthropicfamily (BR-AI-087,
// issue #1778): the two Anthropic implementations across AF and KA already
// diverge intentionally (DD-LLM-007), and this DD explicitly keeps
// anthropicfamily untouched rather than unifying it with eino's
// agenticclaude in the same change (tracked as a deferred follow-up).
//
// Reference: https://ai.google.dev/gemini-api/docs
// Reference: https://github.com/cloudwego/eino-ext/tree/main/components/model/agenticgemini
package geminifamily

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"cloud.google.com/go/auth/credentials"
	"cloud.google.com/go/auth/httptransport"
	"github.com/cloudwego/eino-ext/components/model/agenticgemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/go-logr/logr"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// gcpAuthScope is the OAuth2 scope requested for Vertex AI credentials,
// matching anthropicfamily's resolveVertexAuth.
const gcpAuthScope = "https://www.googleapis.com/auth/cloud-platform"

// Option configures the Client.
type Option func(*clientOpts)

type clientOpts struct {
	logger           logr.Logger
	httpTimeout      time.Duration
	baseTransport    http.RoundTripper
	httpOptions      *genai.HTTPOptions
	defaultReasoning *llm.ReasoningRequest
}

// WithLogger injects a logr.Logger for diagnostic messages. If not
// provided, logging is silently discarded.
func WithLogger(l logr.Logger) Option {
	return func(o *clientOpts) { o.logger = l }
}

// WithHTTPTimeout sets an explicit timeout on the underlying HTTP client
// used by the Gemini API client, preventing unbounded network calls
// (mirrors anthropicfamily.WithHTTPTimeout, #956).
func WithHTTPTimeout(d time.Duration) Option {
	return func(o *clientOpts) { o.httpTimeout = d }
}

// WithBaseTransport sets a custom base RoundTripper (e.g. mTLS, circuit
// breaker, custom headers). For Vertex auth it is wrapped with GCP OAuth2
// authentication; for native API-key auth it is used as-is with the
// x-goog-api-key header applied by the genai SDK. Mirrors
// anthropicfamily.WithBaseTransport (#1342 enterprise mTLS parity).
func WithBaseTransport(rt http.RoundTripper) Option {
	return func(o *clientOpts) { o.baseTransport = rt }
}

// WithHTTPOptions overrides the genai SDK's HTTP options (e.g. BaseURL).
// Test-only: production code has no operator-facing "endpoint override"
// concept for Gemini's own API surface (unlike openai_compatible).
func WithHTTPOptions(opts genai.HTTPOptions) Option {
	return func(o *clientOpts) { o.httpOptions = &opts }
}

// WithReasoning resolves the operator's reasoning/thinking configuration
// ONCE at client-construction time, per llm.ReasoningRequest's documented
// contract (DD-HAPI-019). Mirrors anthropicfamily.WithReasoning.
func WithReasoning(r llm.ReasoningRequest) Option {
	return func(o *clientOpts) { o.defaultReasoning = &r }
}

func newClientOpts(opts ...Option) *clientOpts {
	o := &clientOpts{logger: logr.Discard()}
	for _, fn := range opts {
		fn(o)
	}
	return o
}

// Client implements llm.Client for Gemini, wrapping eino-ext's
// agenticgemini.Model (itself wrapping a *genai.Client).
type Client struct {
	model            *agenticgemini.Model
	logger           logr.Logger
	defaultReasoning *llm.ReasoningRequest
}

// New creates a Client for Gemini on Vertex AI.
//
// credentialsJSON holds the GCP service account or authorized_user JSON,
// resolved at runtime from the Helm-mounted credentials directory. If
// empty, ambient Application Default Credentials (ADC) are used —
// credentials.DetectDefault handles both paths in a single call.
func New(ctx context.Context, modelName string, credentialsJSON []byte, project, location string, opts ...Option) (*Client, error) {
	if project == "" {
		return nil, fmt.Errorf("geminifamily: project is required (vertex_project config)")
	}
	if location == "" {
		location = "us-central1"
	}

	o := newClientOpts(opts...)

	cred, err := credentials.DetectDefault(&credentials.DetectOptions{
		CredentialsJSON: bytes.TrimSpace(credentialsJSON),
		Scopes:          []string{gcpAuthScope},
	})
	if err != nil {
		return nil, fmt.Errorf("geminifamily: resolving Vertex AI credentials: %w", err)
	}

	httpClient, err := httptransport.NewClient(&httptransport.Options{
		Credentials:      cred,
		BaseRoundTripper: o.baseTransport,
	})
	if err != nil {
		return nil, fmt.Errorf("geminifamily: building Vertex AI HTTP client: %w", err)
	}
	applyHTTPTimeout(httpClient, o)

	cc := &genai.ClientConfig{
		Backend:    genai.BackendVertexAI,
		Project:    project,
		Location:   location,
		HTTPClient: httpClient,
	}
	if o.httpOptions != nil {
		cc.HTTPOptions = *o.httpOptions
	}

	return newClientFromConfig(ctx, modelName, cc, o)
}

// NewWithAPIKey creates a Client for Gemini via the native Gemini API
// (generativelanguage.googleapis.com), authenticating with a static API
// key rather than GCP Vertex AI credentials. Distinct from New (Vertex-only)
// so that Vertex's required project/location parameters are never
// mistakenly requested for this auth mode — mirrors
// anthropicfamily.NewWithAPIKey.
func NewWithAPIKey(apiKey, modelName string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("geminifamily: apiKey is required")
	}

	o := newClientOpts(opts...)

	httpClient := &http.Client{}
	if o.baseTransport != nil {
		httpClient.Transport = o.baseTransport
	}
	applyHTTPTimeout(httpClient, o)

	cc := &genai.ClientConfig{
		Backend:    genai.BackendGeminiAPI,
		APIKey:     apiKey,
		HTTPClient: httpClient,
	}
	if o.httpOptions != nil {
		cc.HTTPOptions = *o.httpOptions
	}

	return newClientFromConfig(context.Background(), modelName, cc, o)
}

func applyHTTPTimeout(c *http.Client, o *clientOpts) {
	if o.httpTimeout > 0 {
		c.Timeout = o.httpTimeout
	}
}

func newClientFromConfig(ctx context.Context, modelName string, cc *genai.ClientConfig, o *clientOpts) (*Client, error) {
	genaiClient, err := genai.NewClient(ctx, cc)
	if err != nil {
		return nil, fmt.Errorf("geminifamily: creating genai client: %w", err)
	}

	m, err := agenticgemini.New(ctx, &agenticgemini.Config{
		Client: genaiClient,
		Model:  modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("geminifamily: creating agenticgemini model: %w", err)
	}

	return &Client{model: m, logger: o.logger, defaultReasoning: o.defaultReasoning}, nil
}

// Chat translates a Kubernaut ChatRequest to eino's AgenticModel.Generate,
// and maps the response back.
func (c *Client) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	messages, err := toEinoMessages(req.Messages)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("geminifamily: %w", err)
	}

	msg, err := c.model.Generate(ctx, messages, c.callOptions(req)...)
	if err != nil {
		return llm.ChatResponse{}, classifyErr(fmt.Errorf("geminifamily: %w", err))
	}

	return fromEinoMessage(msg), nil
}

// StreamChat uses eino's AgenticModel.Stream to deliver text/reasoning
// deltas incrementally. The final ChatResponse is built by concatenating
// all received chunks via schema.ConcatAgenticMessages, reusing the
// existing fromEinoMessage mapping.
func (c *Client) StreamChat(ctx context.Context, req llm.ChatRequest, callback func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	messages, err := toEinoMessages(req.Messages)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("geminifamily: %w", err)
	}

	sr, err := c.model.Stream(ctx, messages, c.callOptions(req)...)
	if err != nil {
		return llm.ChatResponse{}, classifyErr(fmt.Errorf("geminifamily: stream error: %w", err))
	}
	defer sr.Close()

	var chunks []*schema.AgenticMessage
	for {
		chunk, recvErr := sr.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return llm.ChatResponse{}, classifyErr(fmt.Errorf("geminifamily: stream recv: %w", recvErr))
		}
		chunks = append(chunks, chunk)
		if delta := extractStreamTextDelta(chunk); delta != "" {
			if cbErr := callback(llm.ChatStreamEvent{Delta: delta}); cbErr != nil {
				return llm.ChatResponse{}, cbErr
			}
		}
	}
	_ = callback(llm.ChatStreamEvent{Done: true})

	if len(chunks) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("geminifamily: empty stream response")
	}
	final, err := schema.ConcatAgenticMessages(chunks)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("geminifamily: concat stream chunks: %w", err)
	}
	return fromEinoMessage(final), nil
}

// callOptions builds the per-call eino model.Option list: tool definitions
// (when present) and the resolved reasoning/thinking configuration. A
// per-call req.Options.Reasoning always wins over the client's
// construction-time default (mirrors anthropicfamily.buildParams).
func (c *Client) callOptions(req llm.ChatRequest) []model.Option {
	var opts []model.Option
	if len(req.Tools) > 0 {
		opts = append(opts, model.WithTools(toEinoTools(req.Tools, c.logger)))
	}

	reasoning := req.Options.Reasoning
	if reasoning == nil {
		reasoning = c.defaultReasoning
	}
	if reasoning != nil && reasoning.Enabled {
		opts = append(opts, agenticgemini.WithThinkingConfig(llm.EffortToThinkingConfig(reasoning)))
	}
	return opts
}

// classifyErr marks err non-retryable (kubernaut#1585 parity) when it
// unwraps to the genai SDK's typed genai.APIError with a permanent
// (400/401/403/404-class) status code.
func classifyErr(err error) error {
	var apiErr genai.APIError
	if errors.As(err, &apiErr) && llm.IsNonRetryableHTTPStatus(apiErr.Code) {
		return llm.MarkNonRetryable(err)
	}
	return err
}

// Close is a no-op: the genai SDK client has no closeable resources.
// Satisfies llm.Client.
func (c *Client) Close() error { return nil }

var _ llm.Client = (*Client)(nil)
