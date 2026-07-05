FROM golang:1.23@sha256:60deed95d3888cc5e4d9ff8a10c54e5edc008c6ae3fba6187be6fb592e19e8c0 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /oas-runtime .

FROM ubuntu:24.04@sha256:4fbb8e6a8395de5a7550b33509421a2bafbc0aab6c06ba2cef9ebffbc7092d90
RUN groupadd -f -g 1000 sandbox && \
    (id -u 1000 >/dev/null 2>&1 && usermod -l sandbox -g sandbox -d /home/sandbox -m $(getent passwd 1000 | cut -d: -f1) || useradd -m -u 1000 -g sandbox sandbox) && \
    apt-get update && apt-get install -y iproute2 ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /oas-runtime /usr/local/bin/oas-runtime
USER sandbox
EXPOSE 8080
