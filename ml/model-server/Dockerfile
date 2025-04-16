# Build stage
FROM golang:1.21-bookworm as builder

WORKDIR /app
COPY . .

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Download ONNX Runtime 1.16.3 Linux package
# (Already correct, but included for context)
RUN wget https://github.com/microsoft/onnxruntime/releases/download/v1.16.3/onnxruntime-linux-x64-1.16.3.tgz \
    && tar -xzf onnxruntime-linux-x64-1.16.3.tgz \
    && mv onnxruntime-linux-x64-1.16.3 /opt/onnxruntime

# Build the Go application
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    CGO_CFLAGS="-I/opt/onnxruntime/include" \
    CGO_LDFLAGS="-L/opt/onnxruntime/lib -lonnxruntime" \
    go build -o model-server ./ml/model-server

# Runtime stage
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgomp1 \
    libssl3 \
    && rm -rf /var/lib/apt/lists/*

# Copy ONNX Runtime files from builder
COPY --from=builder /opt/onnxruntime /opt/onnxruntime

# Add ONNX Runtime to library path
ENV LD_LIBRARY_PATH=/opt/onnxruntime/lib:$LD_LIBRARY_PATH

# Application setup
WORKDIR /app
COPY --from=builder /app/model-server /app/model-server
COPY ml/models /app/models

# Verify ONNX runtime installation
RUN ldconfig && \
    ldd /app/model-server && \
    /app/model-server -version

CMD ["/app/model-server"]
