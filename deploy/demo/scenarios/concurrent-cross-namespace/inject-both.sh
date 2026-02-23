#!/usr/bin/env bash
# Inject bad config into both team-alpha and team-beta namespaces
set -euo pipefail

for NS in demo-team-alpha demo-team-beta; do
  echo "==> Injecting bad config into ${NS}..."
  kubectl apply -f - <<YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: worker-config-bad
  namespace: ${NS}
data:
  nginx.conf: |
    worker_processes auto;
    error_log /var/log/nginx/error.log warn;
    pid /tmp/nginx.pid;
    events { worker_connections 1024; }
    http {
        invalid_directive_that_breaks_nginx on;
        server {
            listen 8080;
            server_name _;
            location / { return 200 'healthy\n'; add_header Content-Type text/plain; }
        }
    }
YAML

  kubectl patch deployment worker -n "${NS}" \
    --type=json \
    -p '[{"op":"replace","path":"/spec/template/spec/volumes/0/configMap/name","value":"worker-config-bad"}]'
done

echo "==> Bad config injected in both namespaces. All pods will crash."
