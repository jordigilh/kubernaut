#!/bin/bash

# HolmesGPT + Kubernaut Environment Stop Script
# This script stops all components of the hybrid environment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

log() {
    echo -e "${CYAN}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

echo -e "${CYAN}"
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║        Stopping HolmesGPT + Kubernaut Environment           ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Stop HolmesGPT container
log "🐳 Stopping HolmesGPT container..."
if podman stop holmesgpt-kubernaut-hybrid 2>/dev/null; then
    log_success "HolmesGPT container stopped"
    podman rm holmesgpt-kubernaut-hybrid 2>/dev/null || true
    log_success "HolmesGPT container removed"
else
    log_warning "HolmesGPT container was not running"
fi

# Stop Context API
log "🚀 Stopping Kubernaut Context API..."
if [ -f /tmp/kubernaut-context-api.pid ]; then
    local pid=$(cat /tmp/kubernaut-context-api.pid)
    if ps -p $pid > /dev/null 2>&1; then
        log "Stopping Context API (PID: $pid)..."
        kill $pid 2>/dev/null || true
        sleep 2

        # Force kill if still running
        if ps -p $pid > /dev/null 2>&1; then
            log_warning "Force stopping Context API..."
            kill -9 $pid 2>/dev/null || true
        fi

        rm -f /tmp/kubernaut-context-api.pid
        log_success "Context API stopped"
    else
        log_warning "Context API PID file exists but process not found"
        rm -f /tmp/kubernaut-context-api.pid
    fi
else
    # Try to find and stop context-api process
    local ctx_pids=$(pgrep -f "context-api-production" 2>/dev/null || true)
    if [ -n "$ctx_pids" ]; then
        log "Found Context API processes, stopping..."
        echo $ctx_pids | xargs kill 2>/dev/null || true
        log_success "Context API processes stopped"
    else
        log_warning "No Context API processes found"
    fi
fi

# Clean up any remaining containers
log "🧹 Cleaning up containers..."
podman container prune -f >/dev/null 2>&1 || true

# Note about Local LLM
log_warning "Local LLM service was not stopped automatically"
log "If you want to stop your local LLM:"
log "  • If using ramalama: Ctrl+C in the terminal where it's running"
log "  • If using ollama: ollama stop"
log "  • If using another service: stop it manually"

log_success "🛑 Environment shutdown complete!"
echo ""
echo -e "${YELLOW}📊 SUMMARY:${NC}"
echo "  • HolmesGPT container: Stopped and removed"
echo "  • Kubernaut Context API: Stopped"
echo "  • Local LLM: Left running (stop manually if needed)"
echo "  • Configuration files: Preserved in ~/.config/holmesgpt/"
echo ""
echo -e "${GREEN}🔄 TO RESTART:${NC}"
echo "  ./scripts/setup-holmesgpt-environment.sh"
