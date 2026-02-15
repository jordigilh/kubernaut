# OOMKill Increase Memory - Runnable Job Container (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017
#
# This image serves DUAL purposes:
#   1. DataStorage OCI extractor reads /workflow-schema.yaml for catalog registration
#   2. WorkflowExecution Job executor runs this image as a K8s Job container
#
# Unlike schema-only images (FROM scratch), Job-based workflows MUST be runnable.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0 .
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# ADR-043: Include workflow schema at root for catalog discovery
COPY workflow-schema.yaml /workflow-schema.yaml

# Job executor injects parameters as environment variables:
#   NAMESPACE               - target namespace
#   DEPLOYMENT_NAME         - deployment to update
#   MEMORY_INCREASE_PERCENT - percentage to increase memory by
ENV NAMESPACE="default" \
    DEPLOYMENT_NAME="test-deployment" \
    MEMORY_INCREASE_PERCENT="50"

ENTRYPOINT ["/bin/sh", "-c", "\
echo '============================================' && \
echo 'OOMKill Recovery - Increase Memory Limits' && \
echo '============================================' && \
echo '' && \
echo \"Namespace: ${NAMESPACE}\" && \
echo \"Deployment: ${DEPLOYMENT_NAME}\" && \
echo \"Memory Increase: ${MEMORY_INCREASE_PERCENT}%\" && \
echo '' && \
echo 'Simulating memory limit increase...' && \
sleep 2 && \
echo 'Memory limits updated successfully!' && \
echo '============================================'"]
