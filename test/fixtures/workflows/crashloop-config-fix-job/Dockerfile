# CrashLoop Config Fix - Runnable Job Container (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017
#
# This image serves DUAL purposes:
#   1. DataStorage OCI extractor reads /workflow-schema.yaml for catalog registration
#   2. WorkflowExecution Job executor runs this image as a K8s Job container
#
# Unlike schema-only images (FROM scratch), Job-based workflows MUST be runnable.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:v1.0.0 .
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# ADR-043: Include workflow schema at root for catalog discovery
COPY workflow-schema.yaml /workflow-schema.yaml

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
