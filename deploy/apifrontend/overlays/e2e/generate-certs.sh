#!/usr/bin/env bash
set -euo pipefail

# Generates self-signed TLS certificates for E2E testing.
# Creates a CA and per-service leaf certificates for AF, KA, and DS.

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

generate_leaf_cert() {
  local name="$1"
  local cn="$2"
  shift 2
  local sans="$*"

  openssl genrsa -out "${CERT_DIR}/${name}-tls.key" 2048
  openssl req -new \
    -key "${CERT_DIR}/${name}-tls.key" \
    -out "${CERT_DIR}/${name}-tls.csr" \
    -subj "/CN=${cn}/O=kubernaut" \
    -config <(printf '[req]\ndistinguished_name = req_distinguished_name\nreq_extensions = v3_req\n[req_distinguished_name]\n[v3_req]\nsubjectAltName = @alt_names\n[alt_names]\n%s\n' "${sans}")

  openssl x509 -req \
    -in "${CERT_DIR}/${name}-tls.csr" \
    -CA "${CERT_DIR}/ca.crt" \
    -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/${name}-tls.crt" \
    -days "${DAYS}" \
    -sha256 \
    -extensions v3_req \
    -extfile <(printf '[v3_req]\nsubjectAltName = @alt_names\n[alt_names]\n%s\n' "${sans}")

  rm -f "${CERT_DIR}/${name}-tls.csr"
}

# AF (apifrontend) leaf certificate
AF_SANS="DNS.1 = apifrontend
DNS.2 = apifrontend.${NAMESPACE}
DNS.3 = apifrontend.${NAMESPACE}.svc
DNS.4 = apifrontend.${NAMESPACE}.svc.cluster.local
DNS.5 = localhost
IP.1 = 127.0.0.1"

generate_leaf_cert "af" "apifrontend.${NAMESPACE}.svc" "${AF_SANS}"

# Keep legacy names for backward compatibility with existing CI
cp "${CERT_DIR}/af-tls.key" "${CERT_DIR}/tls.key"
cp "${CERT_DIR}/af-tls.crt" "${CERT_DIR}/tls.crt"

# KA (kubernaut-agent) leaf certificate
KA_SANS="DNS.1 = kubernaut-agent
DNS.2 = kubernaut-agent.${NAMESPACE}
DNS.3 = kubernaut-agent.${NAMESPACE}.svc
DNS.4 = kubernaut-agent.${NAMESPACE}.svc.cluster.local
DNS.5 = localhost
IP.1 = 127.0.0.1"

generate_leaf_cert "ka" "kubernaut-agent.${NAMESPACE}.svc" "${KA_SANS}"

# DS (data-storage) leaf certificate
DS_SANS="DNS.1 = data-storage
DNS.2 = data-storage.${NAMESPACE}
DNS.3 = data-storage.${NAMESPACE}.svc
DNS.4 = data-storage.${NAMESPACE}.svc.cluster.local
DNS.5 = data-storage-service
DNS.6 = data-storage-service.${NAMESPACE}
DNS.7 = data-storage-service.${NAMESPACE}.svc
DNS.8 = localhost
IP.1 = 127.0.0.1"

generate_leaf_cert "ds" "data-storage.${NAMESPACE}.svc" "${DS_SANS}"

chmod 0600 "${CERT_DIR}/ca.key" "${CERT_DIR}"/*-tls.key "${CERT_DIR}/tls.key"
chmod 0644 "${CERT_DIR}/ca.crt" "${CERT_DIR}"/*-tls.crt "${CERT_DIR}/tls.crt"
rm -f "${CERT_DIR}/ca.srl"

echo "E2E certificates generated in ${CERT_DIR}"
echo "  CA:  ${CERT_DIR}/ca.crt"
echo "  AF:  ${CERT_DIR}/af-tls.{crt,key} (also tls.{crt,key})"
echo "  KA:  ${CERT_DIR}/ka-tls.{crt,key}"
echo "  DS:  ${CERT_DIR}/ds-tls.{crt,key}"
