/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF THE KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"go.uber.org/zap"
)

func main() {
	// Read from environment variables first, then flags
	getEnv := func(key, fallback string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return fallback
	}

	// Flag parsing with environment variable defaults
	var (
		addr       = flag.String("addr", getEnv("HTTP_PORT", ":8080"), "HTTP server address")
		dbHost     = flag.String("db-host", getEnv("DB_HOST", "localhost"), "PostgreSQL host")
		dbPort     = flag.Int("db-port", 5432, "PostgreSQL port")
		dbName     = flag.String("db-name", getEnv("DB_NAME", "action_history"), "PostgreSQL database name")
		dbUser     = flag.String("db-user", getEnv("DB_USER", "db_user"), "PostgreSQL user")
		dbPassword = flag.String("db-password", getEnv("DB_PASSWORD", ""), "PostgreSQL password")
		redisAddr  = flag.String("redis-addr", getEnv("REDIS_ADDR", "localhost:6379"), "Redis address for DLQ")
	)
	flag.Parse()

	// Logger setup
	logger, err := zap.NewProduction()
	if err != nil {
		panic("failed to create logger: " + err.Error())
	}
	defer func() {
		_ = logger.Sync() // Ignore sync errors on shutdown
	}()

	logger.Info("Starting Data Storage service",
		zap.String("addr", *addr),
		zap.String("db_host", *dbHost),
		zap.Int("db_port", *dbPort),
		zap.String("db_name", *dbName),
		zap.String("db_user", *dbUser),
		zap.String("redis_addr", *redisAddr),
	)

	// Context management for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Day 2: Initialize database connection
	dbConnStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
		*dbHost, *dbPort, *dbName, *dbUser, *dbPassword)

	// Day 2: Create HTTP server with database connection + Redis for DLQ
	serverCfg := &server.Config{
		Port:         getPortFromAddr(*addr),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	srv, err := server.NewServer(dbConnStr, *redisAddr, logger, serverCfg)
	if err != nil {
		logger.Fatal("Failed to create server",
			zap.Error(err),
		)
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting",
			zap.String("addr", *addr),
		)
		serverErrors <- srv.Start()
	}()

	// Wait for shutdown signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("Server error",
			zap.Error(err),
		)
	case sig := <-sigChan:
		logger.Info("Shutdown signal received",
			zap.String("signal", sig.String()),
		)

		// DD-007: Graceful shutdown with 35 second timeout
		// (5s propagation + 30s drain)
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 35*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Graceful shutdown failed",
				zap.Error(err),
			)
		}
	}

	logger.Info("Data Storage service stopped")
}

// getPortFromAddr extracts port number from address string (e.g., ":8080" -> 8080)
func getPortFromAddr(addr string) int {
	if addr == "" {
		return 8080
	}
	// Remove leading colon if present
	portStr := strings.TrimPrefix(addr, ":")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 8080 // Default port
	}
	return port
}
