#!/bin/bash

# Port Forwarding Setup Script for Kind Integration
# Alternative to NodePort services using kubectl port-forward

set -euo pipefail

NAMESPACE="kubernaut-integration"
PIDS_FILE="/tmp/kubernaut-port-forwards.pids"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to start port forwarding
start_port_forwarding() {
    log_info "Starting port forwarding for kubernaut services..."

    # Clean up any existing port forwards
    stop_port_forwarding

    # Create PID file
    touch "$PIDS_FILE"

    # Port forward webhook service
    log_info "Port forwarding webhook service: localhost:8080 -> webhook-service:8080"
    kubectl port-forward -n "$NAMESPACE" service/webhook-service 8080:8080 &
    echo $! >> "$PIDS_FILE"

    # Port forward Prometheus
    log_info "Port forwarding Prometheus: localhost:9090 -> prometheus-service:9090"
    kubectl port-forward -n "$NAMESPACE" service/prometheus-service 9090:9090 &
    echo $! >> "$PIDS_FILE"

    # Port forward AlertManager
    log_info "Port forwarding AlertManager: localhost:9093 -> alertmanager-service:9093"
    kubectl port-forward -n "$NAMESPACE" service/alertmanager-service 9093:9093 &
    echo $! >> "$PIDS_FILE"

    # Port forward PostgreSQL
    log_info "Port forwarding PostgreSQL: localhost:5432 -> postgresql-service:5432"
    kubectl port-forward -n "$NAMESPACE" service/postgresql-service 5432:5432 &
    echo $! >> "$PIDS_FILE"

    # Port forward Redis
    log_info "Port forwarding Redis: localhost:6379 -> redis-service:6379"
    kubectl port-forward -n "$NAMESPACE" service/redis-service 6379:6379 &
    echo $! >> "$PIDS_FILE"

    # Port forward HolmesGPT
    log_info "Port forwarding HolmesGPT: localhost:8090 -> holmesgpt-service:8090"
    kubectl port-forward -n "$NAMESPACE" service/holmesgpt-service 8090:8090 &
    echo $! >> "$PIDS_FILE"

    # Wait a moment for port forwards to establish
    sleep 2

    log_success "Port forwarding established!"
    echo ""
    echo "📋 Service Access URLs:"
    echo "  • Webhook Service:  http://localhost:8080"
    echo "  • Prometheus:       http://localhost:9090"
    echo "  • AlertManager:     http://localhost:9093"
    echo "  • PostgreSQL:       localhost:5432"
    echo "  • Redis:            localhost:6379"
    echo "  • HolmesGPT:        http://localhost:8090"
    echo ""
    echo "💡 To stop port forwarding: $0 stop"
}

# Function to stop port forwarding
stop_port_forwarding() {
    if [ -f "$PIDS_FILE" ]; then
        log_info "Stopping existing port forwards..."
        while read -r pid; do
            if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
                kill "$pid" 2>/dev/null || true
            fi
        done < "$PIDS_FILE"
        rm -f "$PIDS_FILE"
        log_success "Port forwarding stopped"
    else
        log_info "No active port forwards found"
    fi
}

# Function to check port forwarding status
check_status() {
    log_info "Checking port forwarding status..."

    if [ ! -f "$PIDS_FILE" ]; then
        log_warning "No port forwarding active"
        return 1
    fi

    local active_count=0
    while read -r pid; do
        if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
            ((active_count++))
        fi
    done < "$PIDS_FILE"

    if [ $active_count -gt 0 ]; then
        log_success "$active_count port forwards active"
        echo ""
        echo "📊 Active Port Forwards:"
        netstat -an | grep LISTEN | grep -E ":(8080|9090|9093|5432|6379|8090)" || true
    else
        log_warning "Port forwarding processes not running"
        rm -f "$PIDS_FILE"
        return 1
    fi
}

# Main function
main() {
    case "${1:-start}" in
        start)
            start_port_forwarding
            ;;
        stop)
            stop_port_forwarding
            ;;
        status)
            check_status
            ;;
        restart)
            stop_port_forwarding
            sleep 1
            start_port_forwarding
            ;;
        --help|-h)
            echo "Usage: $0 [start|stop|status|restart]"
            echo ""
            echo "Port forwarding management for kubernaut Kind integration"
            echo ""
            echo "Commands:"
            echo "  start    - Start port forwarding (default)"
            echo "  stop     - Stop all port forwarding"
            echo "  status   - Check port forwarding status"
            echo "  restart  - Restart port forwarding"
            echo ""
            echo "Port Mappings:"
            echo "  localhost:8080  -> webhook-service:8080"
            echo "  localhost:9090  -> prometheus-service:9090"
            echo "  localhost:9093  -> alertmanager-service:9093"
            echo "  localhost:5432  -> postgresql-service:5432"
            echo "  localhost:6379  -> redis-service:6379"
            echo "  localhost:8090  -> holmesgpt-service:8090"
            ;;
        *)
            echo "Unknown command: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
}

# Execute main function
main "$@"
