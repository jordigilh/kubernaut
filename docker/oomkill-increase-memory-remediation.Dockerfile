# OOMKill Remediation Workflow Container Image
#
# Authority: DD-WORKFLOW-003 (Parameterized Remediation Actions)
# Authority: BR-WE-014 (Kubernetes Job Execution Backend)
#
# This image contains kubectl and a shell script that patches a Kubernetes
# resource's memory limits. It follows the Validate -> Action -> Verify
# pattern from DD-WORKFLOW-003.
#
# Parameters (passed as env vars by WE Job executor):
#   TARGET_RESOURCE_KIND  - Deployment, StatefulSet, DaemonSet
#   TARGET_RESOURCE_NAME  - Resource name
#   TARGET_NAMESPACE      - Resource namespace
#   MEMORY_LIMIT_NEW      - New memory limit (e.g., 256Mi, 1Gi)
#   TARGET_RESOURCE       - Fallback: namespace/kind/name
#
# Build:
#   podman build -f docker/oomkill-increase-memory-remediation.Dockerfile -t oomkill-increase-memory:latest .
#
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install kubectl from the official Kubernetes repository
# Use ARG for architecture so the image builds for both arm64 and amd64
ARG TARGETARCH
ARG KUBECTL_VERSION=v1.31.4
RUN curl -fsSL -o /usr/local/bin/kubectl \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

COPY docker/scripts/oomkill-increase-memory.sh /scripts/remediate.sh
RUN chmod +x /scripts/remediate.sh

USER 1001

ENTRYPOINT ["/scripts/remediate.sh"]
