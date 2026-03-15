#!/usr/bin/env bash
# Inject bad configuration via helm upgrade to trigger CrashLoopBackOff
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Upgrading Helm release with bad nginx config..."

# Create a temporary bad values file
TMPFILE=$(mktemp)
cat > "${TMPFILE}" <<'YAML'
nginx:
  config: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;

    events {
        worker_connections 1024;
    }

    http {
        # INVALID: causes nginx to fail on startup
        invalid_directive_that_breaks_nginx on;

        server {
            listen 8080;
            server_name _;

            location / {
                return 200 'healthy\n';
                add_header Content-Type text/plain;
            }
        }
    }
YAML

helm upgrade demo-crashloop-helm "${SCRIPT_DIR}/chart" \
  -f "${TMPFILE}" \
  -n demo-crashloop-helm

rm -f "${TMPFILE}"

echo "==> Bad config injected via helm upgrade. Pods will crash on startup with:"
echo "     nginx: [emerg] unknown directive \"invalid_directive_that_breaks_nginx\""
echo "==> Watch: kubectl get pods -n demo-crashloop-helm -w"
echo "==> Helm history: helm history demo-crashloop-helm -n demo-crashloop-helm"
