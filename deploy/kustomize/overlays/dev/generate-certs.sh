#!/usr/bin/env bash
set -euo pipefail

# Generates self-signed TLS certificates for LOCAL DEVELOPMENT ONLY.
# Do NOT use these certificates in production environments.
# Creates a CA and server certificate valid for the in-cluster service DNS names.

CERT_DIR="${1:-/tmp/apifrontend-dev-certs}"
DAYS=365
NAMESPACE="kubernaut-system"

umask 077
mkdir -p "${CERT_DIR}"

# Generate CA key and certificate
openssl genrsa -out "${CERT_DIR}/ca.key" 4096
openssl req -x509 -new -nodes \
  -key "${CERT_DIR}/ca.key" \
  -sha256 -days "${DAYS}" \
  -out "${CERT_DIR}/ca.crt" \
  -subj "/CN=kubernaut-dev-ca/O=kubernaut"

# Generate server key
openssl genrsa -out "${CERT_DIR}/tls.key" 4096

# Generate server CSR with SANs
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

# Sign server certificate with CA
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

# Enforce restrictive permissions on private keys
chmod 0600 "${CERT_DIR}/ca.key" "${CERT_DIR}/tls.key"
chmod 0644 "${CERT_DIR}/ca.crt" "${CERT_DIR}/tls.crt"

# Clean up intermediate artifacts
rm -f "${CERT_DIR}/tls.csr" "${CERT_DIR}/ca.srl"

echo "Certificates generated in ${CERT_DIR}:"
ls -la "${CERT_DIR}"

echo ""
echo "WARNING: These certificates are for LOCAL DEVELOPMENT ONLY."
echo ""
echo "To create Kubernetes secrets:"
echo "  kubectl create secret tls apifrontend-tls --cert=${CERT_DIR}/tls.crt --key=${CERT_DIR}/tls.key -n ${NAMESPACE}"
echo "  kubectl create secret generic apifrontend-ca --from-file=ca.crt=${CERT_DIR}/ca.crt -n ${NAMESPACE}"
