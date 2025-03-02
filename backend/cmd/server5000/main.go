package main

import (
	"github.com/souvik03-136/neurabalancer/backend/internal/api"
)

func main() {
	serverAddr := "http://localhost:5000" // Hardcoded for this binary
	api.StartBackendServer(serverAddr)
}
