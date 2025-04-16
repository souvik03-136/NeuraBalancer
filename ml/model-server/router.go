package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// NewRouter creates and returns a new router with all endpoints registered.
func NewRouter() *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/predict", predictHandler).Methods("POST")
	router.HandleFunc("/health", healthCheck).Methods("GET")
	router.HandleFunc("/version", versionHandler).Methods("GET")
	return router
}

// versionHandler returns the model version and onnx version information.
func versionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"model_version": "1.2.0",
		"onnx_version":  "1.12.0",
	})
}
