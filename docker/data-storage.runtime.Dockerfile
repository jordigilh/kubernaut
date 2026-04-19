# Data Storage Service - Runtime-Only Dockerfile (no QEMU)
#
# Used by release.yml arm64 builds: binary is cross-compiled on the host,
# then packaged into this scratch image without QEMU emulation.
#
# Prerequisites: make cross-build-datastorage IMAGE_ARCH=arm64
# Usage: make image-runtime-datastorage IMAGE_ARCH=arm64

ARG BINARY=bin/data-storage-arm64

FROM --platform=linux/amd64 registry.access.redhat.com/ubi10/ubi-minimal:latest AS certs
RUN microdnf install -y ca-certificates tzdata && microdnf clean all

FROM scratch
COPY --from=certs /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=certs /etc/passwd /etc/passwd
ARG BINARY
COPY ${BINARY} /data-storage
COPY api/openapi/data-storage-v1.yaml /usr/local/share/kubernaut/api/openapi/data-storage-v1.yaml
USER 65534
EXPOSE 8080 9090
ENTRYPOINT ["/data-storage"]
CMD []

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-data-storage" \
	org.opencontainers.image.description="Persistent storage service for remediation audit trails with PostgreSQL dual-write support." \
	org.opencontainers.image.vendor="Kubernaut"
