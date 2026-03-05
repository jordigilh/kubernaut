#!/usr/bin/env bash
# Apply SQL migrations to PostgreSQL in the Kind cluster using goose (DD-012).
#
# Goose tracks applied migrations in its goose_db_version table, preventing
# re-application of already-applied migrations (#278).
#
# Usage:
#   ./deploy/demo/scripts/apply-migrations.sh
#
# Expects: kubectl configured for the demo cluster
# Requires: goose CLI (auto-installed if missing)

set -euo pipefail

NAMESPACE="${NAMESPACE:-kubernaut-system}"
REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
MIGRATIONS_DIR="${REPO_ROOT}/migrations"
GOOSE_VERSION="${GOOSE_VERSION:-v3.24.1}"
PG_LOCAL_PORT="${PG_LOCAL_PORT:-15432}"

ensure_goose() {
    if command -v goose &>/dev/null; then
        echo "  goose found: $(goose --version 2>&1 | head -1)"
        return 0
    fi

    echo "  goose not found, installing ${GOOSE_VERSION}..."
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    case "$arch" in
        x86_64)  arch="x86_64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)       echo "ERROR: Unsupported architecture: $arch"; exit 1 ;;
    esac

    local url="https://github.com/pressly/goose/releases/download/${GOOSE_VERSION}/goose_${os}_${arch}"
    local goose_bin="${REPO_ROOT}/.goose-bin/goose"
    mkdir -p "$(dirname "$goose_bin")"
    curl -sL -o "$goose_bin" "$url"
    chmod +x "$goose_bin"
    export PATH="$(dirname "$goose_bin"):$PATH"
    echo "  goose installed to ${goose_bin}"
}

cleanup() {
    if [ -n "${PF_PID:-}" ] && kill -0 "$PF_PID" 2>/dev/null; then
        kill "$PF_PID" 2>/dev/null || true
        wait "$PF_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

echo "==> Applying SQL migrations from ${MIGRATIONS_DIR}"

ensure_goose

echo "  Waiting for PostgreSQL pod..."
kubectl wait --for=condition=ready pod -l app=postgresql -n "${NAMESPACE}" --timeout=120s

POSTGRES_POD=$(kubectl get pods -n "${NAMESPACE}" -l app=postgresql -o jsonpath='{.items[0].metadata.name}')
echo "  PostgreSQL pod: ${POSTGRES_POD}"

PG_PASSWORD=$(kubectl get secret kubernaut-pg-credentials -n "${NAMESPACE}" \
    -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d)

echo "  Port-forwarding PostgreSQL to localhost:${PG_LOCAL_PORT}..."
kubectl port-forward -n "${NAMESPACE}" "pod/${POSTGRES_POD}" "${PG_LOCAL_PORT}:5432" &>/dev/null &
PF_PID=$!
sleep 2

if ! kill -0 "$PF_PID" 2>/dev/null; then
    echo "ERROR: Port-forward failed to start"
    exit 1
fi

GOOSE_DBSTRING="host=localhost port=${PG_LOCAL_PORT} user=slm_user password=${PG_PASSWORD} dbname=action_history sslmode=disable"

echo "==> Running goose up (DD-012)..."
goose -dir "${MIGRATIONS_DIR}" postgres "${GOOSE_DBSTRING}" up

echo "==> Checking migration status..."
goose -dir "${MIGRATIONS_DIR}" postgres "${GOOSE_DBSTRING}" status

echo "  Granting privileges..."
kubectl exec -i -n "${NAMESPACE}" "${POSTGRES_POD}" -- psql -U slm_user -d action_history -q <<'EOF'
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
EOF

echo "==> Migrations complete"
