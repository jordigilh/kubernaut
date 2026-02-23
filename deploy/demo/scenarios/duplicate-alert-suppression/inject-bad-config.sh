#!/usr/bin/env bash
# Inject invalid nginx config to cause all 5 pods to crash simultaneously
set -euo pipefail

NAMESPACE="demo-alert-storm"

echo "==> Injecting bad configuration into gateway-config..."

kubectl apply -f - <<'YAML'
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config-bad
  namespace: demo-alert-storm
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
kubectl patch deployment api-gateway -n "${NAMESPACE}" \
  --type=json \
  -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/configMap/name","value":"gateway-config-bad"}]'

echo "==> Bad config injected. All 5 pods will crash simultaneously."
echo "==> Watch: kubectl get pods -n ${NAMESPACE} -w"
