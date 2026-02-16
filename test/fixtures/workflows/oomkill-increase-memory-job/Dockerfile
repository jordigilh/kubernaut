# OOMKill Increase Memory - Runnable Job Container (BR-WE-014)
# Authority: ADR-043, DD-WORKFLOW-017, DD-WORKFLOW-003
#
# This image serves DUAL purposes:
#   1. DataStorage OCI extractor reads /workflow-schema.yaml for catalog registration
#   2. WorkflowExecution Job executor runs this image as a K8s Job container
#
# The remediation script follows the Validate -> Action -> Verify pattern
# from DD-WORKFLOW-003. It uses kubectl to patch the target resource's
# memory limits.
#
# Build: podman build -t quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:v1.0.0 .
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install kubectl for Kubernetes API access
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4
RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

# ADR-043: Include workflow schema at root for catalog discovery
COPY workflow-schema.yaml /workflow-schema.yaml

# DD-WORKFLOW-003: Remediation script (Validate -> Action -> Verify)
COPY remediate.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
