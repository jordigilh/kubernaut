#!/bin/bash
# ========================================
# HolmesGPT API Integration Test Infrastructure Setup
# ========================================
#
# Purpose: Start Go-managed infrastructure for Python integration tests
# Pattern: DD-INTEGRATION-001 v2.0 (Go-bootstrapped services)
# Usage: ./setup-infrastructure.sh
#
# This script starts the infrastructure defined in test/infrastructure/holmesgpt_integration.go
# and keeps it running for Python tests to use.
#
# Services Started:
# - PostgreSQL (port 15439)
# - Redis (port 16387)
# - Data Storage (port 18098)
# - HAPI (port 18120)
#
# ========================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

echo "════════════════════════════════════════════════════════════════════════"
echo "🚀 Starting HolmesGPT API Integration Infrastructure (Go)"
echo "════════════════════════════════════════════════════════════════════════"
echo "📋 Pattern: DD-INTEGRATION-001 v2.0"
echo "📍 Location: test/infrastructure/holmesgpt_integration.go"
echo ""

# Create a simple Go program that starts infrastructure and waits
cat > /tmp/hapi_infra_runner.go <<'EOF'
package main

import (
    "fmt"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/jordigilh/kubernaut/test/infrastructure"
)

func main() {
    fmt.Println("Starting HAPI integration infrastructure...")
    
    if err := infrastructure.StartHolmesGPTAPIIntegrationInfrastructure(os.Stdout); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to start infrastructure: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Println("✅ Infrastructure started successfully")
    fmt.Println("   • PostgreSQL: localhost:15439")
    fmt.Println("   • Redis: localhost:16387")
    fmt.Println("   • Data Storage: localhost:18098")
    fmt.Println("   • HAPI: localhost:18120")
    fmt.Println("")
    fmt.Println("Infrastructure will remain running. Press Ctrl+C to stop.")
    
    // Wait for interrupt signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan
    
    fmt.Println("\n🧹 Stopping infrastructure...")
    if err := infrastructure.StopHolmesGPTAPIIntegrationInfrastructure(os.Stdout); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to stop infrastructure: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("✅ Infrastructure stopped successfully")
}
EOF

# Build and run the infrastructure runner
cd "$PROJECT_ROOT"
go run /tmp/hapi_infra_runner.go

