# Stage 1: Build
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Copy dependencies & download modules
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go binary
RUN go build -o backend ./cmd/api

# Stage 2: Run the application
FROM alpine:latest
WORKDIR /root/

# Copy compiled binary
COPY --from=builder /app/backend .

# Expose the API port
EXPOSE 8080

# Run the backend server
CMD ["./backend"]
