#!/bin/bash
set -e

# Model Comparison Infrastructure Cleanup Script
# Stops all running ramallama and vLLM servers for model comparison

echo "🛑 Stopping Model Comparison Infrastructure..."

# Function to stop server by PID file
stop_server_by_pid() {
    local pid_file=$1
    local server_name=$2

    if [[ -f "$pid_file" ]]; then
        local pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            echo "🔄 Stopping $server_name (PID: $pid)..."
            kill "$pid"

            # Wait for graceful shutdown
            local count=0
            while kill -0 "$pid" 2>/dev/null && [ $count -lt 10 ]; do
                sleep 1
                count=$((count + 1))
            done

            # Force kill if still running
            if kill -0 "$pid" 2>/dev/null; then
                echo "   Force killing $server_name..."
                kill -9 "$pid" 2>/dev/null || true
            fi

            echo "✅ Stopped $server_name"
        else
            echo "ℹ️  $server_name was not running (stale PID file)"
        fi

        rm -f "$pid_file"
    else
        echo "ℹ️  No PID file found for $server_name"
    fi
}

# Function to stop servers by process name
stop_servers_by_name() {
    local process_pattern=$1
    local server_type=$2

    local pids=$(pgrep -f "$process_pattern" 2>/dev/null || true)

    if [[ -n "$pids" ]]; then
        echo "🔄 Stopping $server_type servers..."
        echo "$pids" | while read -r pid; do
            if [[ -n "$pid" ]]; then
                echo "   Stopping PID: $pid"
                kill "$pid" 2>/dev/null || true
            fi
        done

        # Wait for processes to terminate
        sleep 3

        # Force kill any remaining
        pids=$(pgrep -f "$process_pattern" 2>/dev/null || true)
        if [[ -n "$pids" ]]; then
            echo "   Force killing remaining $server_type processes..."
            echo "$pids" | while read -r pid; do
                if [[ -n "$pid" ]]; then
                    kill -9 "$pid" 2>/dev/null || true
                fi
            done
        fi

        echo "✅ Stopped $server_type servers"
    else
        echo "ℹ️  No $server_type servers running"
    fi
}

# Function to check if ports are free
check_ports_free() {
    local ports=(11434 11435 11436)
    local all_free=true

    echo "🔍 Checking if ports are free..."

    for port in "${ports[@]}"; do
        if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
            echo "⚠️  Port $port is still in use"
            all_free=false
        else
            echo "✅ Port $port is free"
        fi
    done

    if [[ "$all_free" = false ]]; then
        echo ""
        echo "⚠️  Some ports are still in use. You may need to manually kill processes:"
        for port in "${ports[@]}"; do
            local pid=$(lsof -ti :$port 2>/dev/null || true)
            if [[ -n "$pid" ]]; then
                echo "   Port $port: kill $pid"
            fi
        done
    fi
}

# Main cleanup function
main() {
    echo "==============================================="
    echo "🛑 Model Comparison Cleanup"
    echo "==============================================="

    # Move to logs directory if it exists
    if [[ -d "logs" ]]; then
        cd logs
    fi

    echo "🔍 Stopping servers by PID files..."

    # Stop servers by PID files
    stop_server_by_pid "ramallama_11434.pid" "Granite ramallama server"
    stop_server_by_pid "ramallama_11435.pid" "DeepSeek ramallama server"
    stop_server_by_pid "ramallama_11436.pid" "Steiner ramallama server"
    stop_server_by_pid "vllm_11434.pid" "Granite vLLM server"
    stop_server_by_pid "vllm_11435.pid" "DeepSeek vLLM server"
    stop_server_by_pid "vllm_11436.pid" "Steiner vLLM server"

    echo ""
    echo "🔍 Stopping any remaining servers by process name..."

    # Stop any remaining servers by process pattern
    stop_servers_by_name "ramallama serve" "ramallama"
    stop_servers_by_name "vllm.entrypoints.openai.api_server" "vLLM"

    echo ""
    check_ports_free

    echo ""
    echo "🧹 Cleaning up log files..."

    # Clean up old log files (optional - keep recent ones for debugging)
    find . -name "ramallama_*.log" -mtime +1 -delete 2>/dev/null || true
    find . -name "vllm_*.log" -mtime +1 -delete 2>/dev/null || true

    echo ""
    echo "✅ Model comparison infrastructure cleanup complete!"
    echo ""
    echo "📊 To restart the infrastructure:"
    echo "   ./scripts/setup_model_comparison.sh"
}

# Cleanup function for script interruption
cleanup() {
    echo ""
    echo "🛑 Cleanup interrupted."
    exit 1
}

trap cleanup INT TERM

# Run main function
main "$@"
