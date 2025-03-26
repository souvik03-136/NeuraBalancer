FROM golang:1.21 as builder

WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -o model-server .

FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/model-server /app/model-server
COPY ml/models /app/models
COPY --from=builder /app/onnxruntime.so /usr/lib/

ENV LD_LIBRARY_PATH=/usr/lib
CMD ["/app/model-server"]