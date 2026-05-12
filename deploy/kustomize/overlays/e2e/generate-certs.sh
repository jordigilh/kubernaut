#!/usr/bin/env bash
set -euo pipefail

# Generates self-signed TLS certificates for E2E testing.
# Creates a CA and server certificate valid for AF in-cluster service DNS names.

CERT_DIR="${1:-/tmp/apifrontend-e2e-certs}"
DAYS=30
NAMESPACE="kubernaut-system"

umask 077
mkdir -p "${CERT_DIR}"

# Generate CA
openssl genrsa -out "${CERT_DIR}/ca.key" 2048
openssl req -x509 -new -nodes \
  -key "${CERT_DIR}/ca.key" \
  -sha256 -days "${DAYS}" \
  -out "${CERT_DIR}/ca.crt" \
  -subj "/CN=kubernaut-e2e-ca/O=kubernaut"

# Generate server key and cert for AF
openssl genrsa -out "${CERT_DIR}/tls.key" 2048
openssl req -new \
  -key "${CERT_DIR}/tls.key" \
  -out "${CERT_DIR}/tls.csr" \
  -subj "/CN=apifrontend.${NAMESPACE}.svc/O=kubernaut" \
  -config <(cat <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
[req_distinguished_name]
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = apifrontend
DNS.2 = apifrontend.${NAMESPACE}
DNS.3 = apifrontend.${NAMESPACE}.svc
DNS.4 = apifrontend.${NAMESPACE}.svc.cluster.local
DNS.5 = localhost
IP.1 = 127.0.0.1
EOF
)

openssl x509 -req \
  -in "${CERT_DIR}/tls.csr" \
  -CA "${CERT_DIR}/ca.crt" \
  -CAkey "${CERT_DIR}/ca.key" \
  -CAcreateserial \
  -out "${CERT_DIR}/tls.crt" \
  -days "${DAYS}" \
  -sha256 \
  -extensions v3_req \
  -extfile <(cat <<EOF
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = apifrontend
DNS.2 = apifrontend.${NAMESPACE}
DNS.3 = apifrontend.${NAMESPACE}.svc
DNS.4 = apifrontend.${NAMESPACE}.svc.cluster.local
DNS.5 = localhost
IP.1 = 127.0.0.1
EOF
)

chmod 0600 "${CERT_DIR}/ca.key" "${CERT_DIR}/tls.key"
chmod 0644 "${CERT_DIR}/ca.crt" "${CERT_DIR}/tls.crt"
rm -f "${CERT_DIR}/tls.csr" "${CERT_DIR}/ca.srl"

echo "E2E certificates generated in ${CERT_DIR}"
