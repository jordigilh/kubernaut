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
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

func main() {
	cfg := config.LoadFromEnv()

	overrides, err := config.LoadYAMLOverrides(cfg.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load YAML overrides from %s: %v", cfg.ConfigPath, err)
	}

	registry := scenarios.DefaultRegistryWithOverrides(overrides)

	router := handlers.NewFullRouter(registry, cfg.ForceText, cfg.RecordHeaders, nil)

	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr())
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", cfg.ListenAddr(), err)
	}

	log.Printf("Mock LLM server starting on %s (force_text=%v)", cfg.ListenAddr(), cfg.ForceText)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down", sig)
	case err := <-errCh:
		log.Printf("Server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
	}
}
