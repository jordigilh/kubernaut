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
	"os"
	"os/signal"
	"syscall"

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
		dbName     = flag.String("db-name", getEnv("DB_NAME", "kubernaut"), "PostgreSQL database name")
		dbUser     = flag.String("db-user", getEnv("DB_USER", "kubernaut"), "PostgreSQL user")
		dbPassword = flag.String("db-password", getEnv("DB_PASSWORD", ""), "PostgreSQL password")
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
	)

	// Context management
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: Day 2 - Initialize database connection
	// dbConnStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=disable",
	//     *dbHost, *dbPort, *dbName, *dbUser, *dbPassword)
	// TODO: Day 2 - Initialize schema with schema.NewInitializer
	// TODO: Day 6 - Initialize Data Storage client
	// TODO: Day 11 - Start HTTP server

	_ = ctx        // Will be used in Day 2
	_ = dbPassword // Will be used in Day 2

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down Data Storage service")
}
