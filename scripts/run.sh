#!/bin/bash

echo "🚀 Starting AI Load Balancer Backend..."
source .env

# Run backend
go run backend/cmd/api/main.go
