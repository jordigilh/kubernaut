#!/usr/bin/env bash
# Apply SQL migrations to PostgreSQL in the Kind cluster
#
# Usage:
#   ./deploy/demo/scripts/apply-migrations.sh
#
# Expects: kubectl configured for the demo cluster

set -euo pipefail

NAMESPACE="kubernaut-system"
MIGRATIONS_DIR="$(cd "$(dirname "$0")/../../.." && pwd)/migrations"

echo "==> Applying SQL migrations from ${MIGRATIONS_DIR}"

# Wait for PostgreSQL to be ready
echo "  Waiting for PostgreSQL pod..."
kubectl wait --for=condition=ready pod -l app=postgresql -n "${NAMESPACE}" --timeout=120s

POSTGRES_POD=$(kubectl get pods -n "${NAMESPACE}" -l app=postgresql -o jsonpath='{.items[0].metadata.name}')
echo "  PostgreSQL pod: ${POSTGRES_POD}"

# Apply each migration file (goose Up section only)
FAILED=0
APPLIED=0
for migration in $(ls "${MIGRATIONS_DIR}"/*.sql 2>/dev/null | sort); do
    filename=$(basename "${migration}")
    echo -n "  Applying: ${filename}..."
    # Extract only the Up section (before -- +goose Down).
    # If the file has no "-- +goose Down" marker, sed prints the entire file.
    if sed -n '1,/^-- +goose Down/p' "${migration}" | grep -v '^-- +goose Down' | \
        kubectl exec -i -n "${NAMESPACE}" "${POSTGRES_POD}" -- \
        psql -U slm_user -d action_history -q 2>&1; then
        echo " OK"
        APPLIED=$((APPLIED + 1))
    else
        echo " FAILED"
        FAILED=$((FAILED + 1))
    fi
done

echo "  Applied: ${APPLIED}, Failed: ${FAILED}"
if [ "${FAILED}" -gt 0 ]; then
    echo "WARNING: ${FAILED} migration(s) failed. Check output above."
fi

# Grant privileges
echo "  Granting privileges..."
kubectl exec -i -n "${NAMESPACE}" "${POSTGRES_POD}" -- psql -U slm_user -d action_history -q <<'EOF'
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
EOF

echo "==> Migrations complete"
