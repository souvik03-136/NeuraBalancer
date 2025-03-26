package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/yalue/onnxruntime_go"
)

var (
	session   *onnxruntime_go.Session
	scaler    Scaler
	modelLock sync.RWMutex
)

type Scaler struct {
	Mean  []float32 `json:"mean"`
	Scale []float32 `json:"scale"`
}

type PredictionRequest struct {
	Features []float32 `json:"features"`
}

type PredictionResponse struct {
	PredictedTime float32 `json:"predicted_time"`
	ServerID      int     `json:"server_id"`
}

func loadScaler() error {
	file, err := os.ReadFile("ml/models/scaler.json")
	if err != nil {
		return fmt.Errorf("scaler file error: %w", err)
	}
	return json.Unmarshal(file, &scaler)
}

func normalize(features []float32) []float32 {
	normalized := make([]float32, len(features))
	for i := range features {
		if i < len(scaler.Mean) && i < len(scaler.Scale) {
			normalized[i] = (features[i] - scaler.Mean[i]) / scaler.Scale[i]
		}
	}
	return normalized
}

func predictHandler(w http.ResponseWriter, r *http.Request) {
	var req PredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	modelLock.RLock()
	defer modelLock.RUnlock()

	// Normalize input
	normalized := normalize(req.Features)

	// Create tensor
	inputTensor, err := ort.NewTensor(normalized)
	if err != nil {
		http.Error(w, fmt.Sprintf("Tensor creation failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer inputTensor.Destroy()

	// Run inference
	outputs, err := session.Run([]*onnxruntime_go.Tensor{inputTensor})
	if err != nil {
		http.Error(w, fmt.Sprintf("Prediction failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer outputs[0].Destroy()

	response := PredictionResponse{
		PredictedTime: outputs[0].GetData().([]float32)[0],
		ServerID:      -1, // Replace with actual server selection logic
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func main() {
	_ = godotenv.Load()

	// Initialize ONNX runtime
	ort.SetSharedLibraryPath("./onnxruntime.so")
	if err := ort.InitializeEnvironment(); err != nil {
		log.Fatalf("ONNX init failed: %v", err)
	}
	defer ort.CleanupEnvironment()

	// Load scaler
	if err := loadScaler(); err != nil {
		log.Fatalf("Scaler load failed: %v", err)
	}

	// Load model
	var err error
	session, err = ort.NewSession("ml/models/load_balancer.onnx")
	if err != nil {
		log.Fatalf("Model load failed: %v", err)
	}
	defer session.Destroy()

	// Configure router
	router := mux.NewRouter()
	router.HandleFunc("/predict", predictHandler).Methods("POST")
	router.HandleFunc("/health", healthCheck).Methods("GET")

	port := os.Getenv("ML_SERVICE_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("ML Service running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
