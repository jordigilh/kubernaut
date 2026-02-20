#!/usr/bin/env bash
# Inject invalid nginx configuration to trigger CrashLoopBackOff
# The broken config uses an invalid directive that causes nginx to exit on startup
set -euo pipefail

NAMESPACE="demo-crashloop"

echo "==> Injecting bad configuration into worker-config..."

kubectl apply -f - <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: worker-config-bad
  namespace: demo-crashloop
data:
  nginx.conf: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;

    events {
        worker_connections 1024;
    }

    http {
        # INVALID: this directive does not exist and causes nginx to fail on startup
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

echo "==> Patching deployment to reference broken ConfigMap..."
kubectl patch deployment worker -n "${NAMESPACE}" \
  --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/configMap/name","value":"worker-config-bad"}]'

echo "==> Bad config injected. Pods will crash on startup with:"
echo "     nginx: [emerg] unknown directive \"invalid_directive_that_breaks_nginx\""
echo "==> Watch: kubectl get pods -n ${NAMESPACE} -w"
