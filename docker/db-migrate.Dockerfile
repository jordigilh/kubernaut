# db-migrate: UBI10-minimal image bundling goose + psql for database migrations.
#
# Goose is built with `go install` at the version pinned in go.mod
# (github.com/pressly/goose/v3) so the CLI matches application dependencies
# without maintaining per-release binary SHA256 checksums.
#
# Postgres-only build (-tags): kubernaut's migrations (charts/kubernaut/templates/
# hooks/migration-job.yaml's `goose ... postgres ...` invocation, and
# test/infrastructure/migrations.go's goose.DialectPostgres) only ever use the
# postgres dialect. Excluding every other driver goose supports drops their
# transitive dependencies entirely from the compiled binary (verified via
# `go version -m`, which is what Trivy's gobinary scanner reads) -- notably
# github.com/ydb-platform/ydb-go-sdk (driver_ydb.go), whose grpc dependency
# repeatedly requires manual `go get google.golang.org/grpc@vX` overrides here
# every time a new gRPC-Go CVE lands (see git history for prior golang.org/x/
# crypto and x/net overrides, same root cause). None of kubernaut's own code
# ever exercises those drivers, so removing them removes the recurring CVE
# surface instead of chasing it version-by-version, and shrinks the binary
# ~3x (56MB -> 20MB).
#
# Build:
#   podman build --platform linux/amd64 \
#     --build-arg APP_VERSION=v3.27.2 \
#     -t quay.io/kubernaut-ai/db-migrate:v3.27.2-amd64 \
#     -f docker/db-migrate.Dockerfile .
#
# Multi-arch: TARGETARCH is set from --platform for the goose compile step.

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi10/go-toolset:10.2@sha256:40eb0e19d90700b02aa1055810a637f307af48c2d1cb376905bc53e3e583af6f AS goose-builder
USER root
ARG TARGETARCH
ENV CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH}
# no_clickhouse/no_mssql/no_mysql/no_sqlite3/no_turso/no_vertica/no_ydb: keep
# only the postgres driver (see comment above) -- libsql remains registered
# (compiled unconditionally by the core goose library, not gated by a
# cmd/goose build tag) but is SQLite-based with no grpc/CVE-prone transitive
# dependencies, so it's left as-is rather than chasing further.
RUN GOMODCACHE=$(mktemp -d) && \
    cd "$GOMODCACHE" && \
    go mod init tmp && \
    go get github.com/pressly/goose/v3/cmd/goose@v3.27.2 && \
    go build -tags 'no_clickhouse no_mssql no_mysql no_sqlite3 no_turso no_vertica no_ydb' \
      -o /go/bin/goose github.com/pressly/goose/v3/cmd/goose

FROM registry.access.redhat.com/ubi10/ubi-minimal:latest@sha256:af74bce19b9ab6446362310c9d18ffb4671ac11b2a4d36263047d9f57a653d80 AS production

ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

COPY --from=goose-builder /go/bin/goose /usr/local/bin/goose

RUN microdnf update -y && \
	microdnf install -y postgresql ca-certificates shadow-utils && \
	microdnf clean all

RUN useradd -r -u 1001 -g root db-migrate-user
USER 1001

LABEL name="kubernaut-db-migrate" \
	vendor="Kubernaut" \
	summary="Kubernaut Database Migration - goose + psql on UBI10-minimal" \
	description="Bundles goose SQL migration tool and PostgreSQL client for Helm post-install/post-upgrade database migrations in connected and disconnected environments." \
	maintainer="jgil@redhat.com" \
	component="db-migrate" \
	part-of="kubernaut" \
	io.k8s.description="Database migration image for Kubernaut Helm chart hooks" \
	io.k8s.display-name="Kubernaut DB Migrate" \
	io.openshift.tags="kubernaut,db-migrate,goose,postgresql,migration,helm-hook"

LABEL org.opencontainers.image.source="https://github.com/jordigilh/kubernaut" \
	org.opencontainers.image.version="${APP_VERSION}" \
	org.opencontainers.image.revision="${GIT_COMMIT}" \
	org.opencontainers.image.created="${BUILD_DATE}" \
	org.opencontainers.image.title="kubernaut-db-migrate" \
	org.opencontainers.image.description="Database migration image bundling goose and psql for Kubernaut Helm chart hooks in connected and disconnected environments." \
	org.opencontainers.image.vendor="Kubernaut"

ENTRYPOINT ["/usr/local/bin/goose"]
CMD ["--help"]
