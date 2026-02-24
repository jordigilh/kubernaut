#!/usr/bin/env bash
# Inject invalid nginx configuration to trigger CrashLoopBackOff
set -euo pipefail

NAMESPACE="demo-remediation-retry"

echo "==> Injecting bad configuration into worker-config..."

kubectl apply -f - <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: worker-config-bad
  namespace: demo-remediation-retry
data:
  nginx.conf: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;

    events {
        worker_connections 1024;
    }

    http {
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

echo "==> Bad config injected. Pods will crash on startup."
echo "==> Watch: kubectl get pods -n ${NAMESPACE} -w"
