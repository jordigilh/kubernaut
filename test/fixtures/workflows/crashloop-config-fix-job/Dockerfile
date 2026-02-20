# CrashLoop Config Fix - Execution Image (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017
#
# Execution-only image for the WE Job executor. Does NOT contain workflow-schema.yaml.
# The schema is delivered separately via a schema-only OCI image (FROM scratch).
# See: test/fixtures/workflows/Dockerfile (shared schema image builder)
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:v1.0.0-exec .
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Job executor injects parameters as environment variables:
#   NAMESPACE           - target namespace
#   DEPLOYMENT_NAME     - deployment to restart
#   GRACE_PERIOD_SECONDS - graceful shutdown period
ENV NAMESPACE="default" \
    DEPLOYMENT_NAME="test-deployment" \
    GRACE_PERIOD_SECONDS="30"

ENTRYPOINT ["/bin/sh", "-c", "\
echo '============================================' && \
echo 'CrashLoop Config Fix - Remediation Job' && \
echo '============================================' && \
echo '' && \
echo \"Namespace: ${NAMESPACE}\" && \
echo \"Deployment: ${DEPLOYMENT_NAME}\" && \
echo \"Grace Period: ${GRACE_PERIOD_SECONDS}s\" && \
echo '' && \
echo 'Simulating deployment restart...' && \
sleep 2 && \
echo 'Remediation completed successfully!' && \
echo '============================================'"]
