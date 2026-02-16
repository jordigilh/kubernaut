#!/usr/bin/env bash
# Generate TLS certificates for the AuthWebhook admission controller
# and patch the webhook configurations with the CA bundle
#
# Usage:
#   ./deploy/demo/scripts/generate-webhook-certs.sh

set -euo pipefail

NAMESPACE="kubernaut-system"
SERVICE="authwebhook"
TMPDIR=$(mktemp -d)

echo "==> Generating TLS certificates for AuthWebhook"

# Generate CA
openssl genrsa -out "${TMPDIR}/ca.key" 2048 2>/dev/null
openssl req -new -x509 -days 365 -key "${TMPDIR}/ca.key" \
    -out "${TMPDIR}/ca.crt" -subj "/CN=authwebhook-ca" 2>/dev/null

# Generate server certificate
openssl genrsa -out "${TMPDIR}/tls.key" 2048 2>/dev/null
openssl req -new -key "${TMPDIR}/tls.key" \
    -out "${TMPDIR}/tls.csr" \
    -subj "/CN=${SERVICE}.${NAMESPACE}.svc" \
    -addext "subjectAltName=DNS:${SERVICE},DNS:${SERVICE}.${NAMESPACE},DNS:${SERVICE}.${NAMESPACE}.svc,DNS:${SERVICE}.${NAMESPACE}.svc.cluster.local" \
    2>/dev/null

openssl x509 -req -in "${TMPDIR}/tls.csr" \
    -CA "${TMPDIR}/ca.crt" -CAkey "${TMPDIR}/ca.key" -CAcreateserial \
    -out "${TMPDIR}/tls.crt" -days 365 \
    -extfile <(echo "subjectAltName=DNS:${SERVICE},DNS:${SERVICE}.${NAMESPACE},DNS:${SERVICE}.${NAMESPACE}.svc,DNS:${SERVICE}.${NAMESPACE}.svc.cluster.local") \
    2>/dev/null

# Create TLS Secret
echo "  Creating TLS secret..."
kubectl create secret tls authwebhook-tls \
    --cert="${TMPDIR}/tls.crt" \
    --key="${TMPDIR}/tls.key" \
    -n "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -

# Patch webhook configurations with CA bundle
CA_BUNDLE=$(base64 < "${TMPDIR}/ca.crt" | tr -d '\n')
echo "  Patching MutatingWebhookConfiguration..."
kubectl patch mutatingwebhookconfiguration authwebhook-mutating --type='json' \
    -p "[
        {\"op\":\"replace\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
        {\"op\":\"replace\",\"path\":\"/webhooks/1/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"},
        {\"op\":\"replace\",\"path\":\"/webhooks/2/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}
    ]"

echo "  Patching ValidatingWebhookConfiguration..."
kubectl patch validatingwebhookconfiguration authwebhook-validating --type='json' \
    -p "[{\"op\":\"replace\",\"path\":\"/webhooks/0/clientConfig/caBundle\",\"value\":\"${CA_BUNDLE}\"}]"

# Cleanup
rm -rf "${TMPDIR}"

echo "==> AuthWebhook TLS setup complete"
