package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/codeany-ai/open-agent-sdk-go/types"
	"github.com/jordigilh/kubernaut/spikes/oas-runtime/internal/acp"
	"github.com/jordigilh/kubernaut/spikes/oas-runtime/internal/runtime"
)

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	model := flag.String("model", envOrDefault("OAS_MODEL", "sonnet-4-6"), "LLM model name")
	apiKey := flag.String("api-key", os.Getenv("OAS_API_KEY"), "LLM provider API key")
	baseURL := flag.String("base-url", os.Getenv("OAS_BASE_URL"), "LLM provider base URL (for OpenAI-compatible)")
	llmEndpoint := flag.String("llm-endpoint", os.Getenv("OAS_LLM_ENDPOINT"), "inference.local endpoint for zero-secret LLM access")
	mcpEndpoints := flag.String("mcp-endpoints", os.Getenv("OAS_MCP_ENDPOINTS"), "comma-separated MCP server URLs (name=url)")
	maxTurns := flag.Int("max-turns", 25, "maximum agent reasoning turns")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	effectiveBaseURL := *baseURL
	if *llmEndpoint != "" {
		effectiveBaseURL = *llmEndpoint
		logger.Info("using inference.local endpoint for zero-secret LLM access", "endpoint", *llmEndpoint)
	}

	mcpServers := parseMCPEndpoints(*mcpEndpoints)
	if len(mcpServers) > 0 {
		names := make([]string, 0, len(mcpServers))
		for name := range mcpServers {
			names = append(names, name)
		}
		logger.Info("configured MCP servers", "servers", names)
	}

	executor := runtime.NewExecutor(runtime.Config{
		Model:        *model,
		APIKey:       *apiKey,
		BaseURL:      effectiveBaseURL,
		MCPServers:   mcpServers,
		MaxTurns:     *maxTurns,
		Logger:       logger,
		PermissionHook: func(toolName string, input map[string]interface{}) bool {
			logger.Info("permission request", "tool", toolName)
			return true
		},
	})

	server := acp.NewServer(executor, logger)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("OAS Runtime starting", "port", *port, "model", *model)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}

func parseMCPEndpoints(raw string) map[string]types.MCPServerConfig {
	if raw == "" {
		return nil
	}
	servers := make(map[string]types.MCPServerConfig)
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), "=", 2)
		if len(parts) != 2 {
			continue
		}
		name, url := parts[0], parts[1]
		servers[name] = types.MCPServerConfig{
			Type: types.MCPTransportHTTP,
			URL:  url,
		}
	}
	return servers
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
