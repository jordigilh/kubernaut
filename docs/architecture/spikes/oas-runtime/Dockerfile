FROM golang:1.26@sha256:2005724102f45917a63e9d092fc0e4ea56ea575048ce147caad5f5f61502c365 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /oas-runtime .

FROM ubuntu:26.04@sha256:678c6550cc43645e08669028bc177f50be4e7c5b8cca677067b1914d4afc7a03
RUN groupadd -f -g 1000 sandbox && \
    (id -u 1000 >/dev/null 2>&1 && usermod -l sandbox -g sandbox -d /home/sandbox -m $(getent passwd 1000 | cut -d: -f1) || useradd -m -u 1000 -g sandbox sandbox) && \
    apt-get update && apt-get install -y iproute2 ca-certificates && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /oas-runtime /usr/local/bin/oas-runtime
USER sandbox
EXPOSE 8080
