# File: deployments/docker/Dockerfile.ml
# Multi-stage build for the ONNX ML model inference server.
# Downloads and bundles ONNX Runtime so the image is self-contained.

ARG ONNX_VERSION=1.16.3

# ── Stage 1: ONNX Runtime downloader ─────────────────────────────────────────
FROM debian:bookworm-slim AS onnx-downloader

ARG ONNX_VERSION
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-x64-${ONNX_VERSION}.tgz" \
    && tar -xzf "onnxruntime-linux-x64-${ONNX_VERSION}.tgz" \
    && mv "onnxruntime-linux-x64-${ONNX_VERSION}" /opt/onnxruntime

# ── Stage 2: Go builder ───────────────────────────────────────────────────────
FROM golang:1.21-bookworm AS builder

ARG ONNX_VERSION
COPY --from=onnx-downloader /opt/onnxruntime /opt/onnxruntime

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY ml/model-server/ ./ml/model-server/

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    CGO_CFLAGS="-I/opt/onnxruntime/include" \
    CGO_LDFLAGS="-L/opt/onnxruntime/lib -lonnxruntime" \
    go build \
    -ldflags="-w -s" \
    -trimpath \
    -o /bin/ml-server \
    ./ml/model-server

# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
FROM debian:bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgomp1 \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime shared libraries
COPY --from=onnx-downloader /opt/onnxruntime /opt/onnxruntime
ENV LD_LIBRARY_PATH=/opt/onnxruntime/lib:$LD_LIBRARY_PATH
ENV ONNX_LIB_PATH=/opt/onnxruntime/lib/libonnxruntime.so

RUN ldconfig

WORKDIR /app

COPY --from=builder /bin/ml-server /app/ml-server

# Model files are mounted as a volume at runtime (see docker-compose.yml)
RUN mkdir -p /app/models

EXPOSE 8081

HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://localhost:8081/health || exit 1

# Run as non-root
RUN useradd -u 1001 -r mluser
USER mluser

ENTRYPOINT ["/app/ml-server"]
