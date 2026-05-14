# Gateway Service - Runtime-Only Dockerfile (no QEMU)
#
# Prerequisites: make cross-build-gateway IMAGE_ARCH=arm64
# Usage: make image-runtime-gateway IMAGE_ARCH=arm64

ARG BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-minimal:latest
ARG BINARY=bin/gateway-arm64

# SECURITY: Pin to specific digest on release. Run: skopeo inspect --format '{{.Digest}}' docker://registry.access.redhat.com/ubi10/ubi-minimal:latest
# Best practice: pass --build-arg BASE_IMAGE=registry.access.redhat.com/ubi10/ubi-minimal@sha256:<digest> in CI; digests change with each image release.
FROM --platform=linux/amd64 ${BASE_IMAGE} AS certs
RUN microdnf install -y ca-certificates tzdata && microdnf clean all

FROM scratch
COPY --from=certs /etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem /etc/ssl/certs/ca-certificates.crt
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=certs /etc/passwd /etc/passwd
ARG BINARY
COPY ${BINARY} /gateway
USER 65534
EXPOSE 8080 9090
ENTRYPOINT ["/gateway"]
CMD []

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-gateway" \
	org.opencontainers.image.vendor="Kubernaut"
