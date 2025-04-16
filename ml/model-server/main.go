// main.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"

	"github.com/joho/godotenv"
	"github.com/yalue/onnxruntime_go"
)

var (
	session   *onnxruntime_go.DynamicAdvancedSession
	scaler    Scaler
	modelLock sync.RWMutex
)

const expectedFeatureCount = 6

type Scaler struct {
	Mean  []float32 `json:"mean"`
	Scale []float32 `json:"scale"`
}

type PredictionResponse struct {
	Scores []float32 `json:"scores"`
}

func loadScaler() error {
	data, err := os.ReadFile("ml/models/scaler.json")
	if err != nil {
		return fmt.Errorf("scaler file error: %w", err)
	}
	return json.Unmarshal(data, &scaler)
}

func normalize(features []float32) []float32 {
	out := make([]float32, len(features))
	for i := range features {
		if i < len(scaler.Mean) && i < len(scaler.Scale) {
			out[i] = (features[i] - scaler.Mean[i]) / scaler.Scale[i]
		}
	}
	return out
}

func predict(normalized []float32) (float32, error) {
	modelLock.RLock()
	defer modelLock.RUnlock()

	inShape := onnxruntime_go.NewShape(1, int64(len(normalized)))
	inTensor, err := onnxruntime_go.NewTensor[float32](inShape, normalized)
	if err != nil {
		return 0, fmt.Errorf("tensor creation failed: %w", err)
	}
	defer inTensor.Destroy()

	outShape := onnxruntime_go.NewShape(1, 1)
	outTensor, err := onnxruntime_go.NewEmptyTensor[float32](outShape)
	if err != nil {
		return 0, fmt.Errorf("output tensor alloc failed: %w", err)
	}
	defer outTensor.Destroy()

	inputs := []onnxruntime_go.ArbitraryTensor{inTensor}
	outputs := []onnxruntime_go.ArbitraryTensor{outTensor}

	if err := session.Run(inputs, outputs); err != nil {
		return 0, fmt.Errorf("inference failed: %w", err)
	}

	return outTensor.GetData()[0], nil
}

func predictHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Servers []struct {
			CPU         float32 `json:"cpu_usage"`
			Memory      float32 `json:"memory_usage"`
			Connections int     `json:"active_conns"`
			ErrorRate   float32 `json:"error_rate"`
			ResponseP95 float32 `json:"response_p95"`
			Capacity    int     `json:"capacity"`
		} `json:"servers"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Unable to read request body", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	scores := make([]float32, len(req.Servers))
	for i, srv := range req.Servers {
		feats := []float32{
			srv.CPU,
			srv.Memory,
			float32(srv.Connections),
			srv.ErrorRate,
			srv.ResponseP95,
			float32(srv.Capacity),
		}
		if len(feats) != expectedFeatureCount {
			http.Error(w,
				fmt.Sprintf("Invalid feature count: expected %d got %d", expectedFeatureCount, len(feats)),
				http.StatusBadRequest,
			)
			return
		}
		norm := normalize(feats)
		score, err := predict(norm)
		if err != nil {
			log.Printf("Prediction error: %v", err)
			http.Error(w, "Prediction failed", http.StatusInternalServerError)
			return
		}
		scores[i] = score
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(PredictionResponse{Scores: scores})
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func main() {
	_ = godotenv.Load()

	if runtime.GOOS == "windows" {
		onnxruntime_go.SetSharedLibraryPath("onnxruntime.dll")
	} else {
		onnxruntime_go.SetSharedLibraryPath("libonnxruntime.so.1.16.0")
	}

	if err := onnxruntime_go.InitializeEnvironment(); err != nil {
		log.Fatalf("ORT init failed: %v", err)
	}
	defer onnxruntime_go.DestroyEnvironment()

	if err := loadScaler(); err != nil {
		log.Fatalf("Scaler load failed: %v", err)
	}

	opts, err := onnxruntime_go.NewSessionOptions()
	if err != nil {
		log.Fatalf("SessionOptions creation failed: %v", err)
	}
	defer opts.Destroy()

	// Verify actual input/output names using Netron
	session, err = onnxruntime_go.NewDynamicAdvancedSession(
		"ml/models/load_balancer.onnx",
		[]string{"serving_default_input:0"},   // Actual input name
		[]string{"StatefulPartitionedCall:0"}, // Actual output name
		opts,
	)
	if err != nil {
		log.Fatalf("Model load failed: %v", err)
	}
	defer session.Destroy()

	router := NewRouter()
	port := os.Getenv("ML_SERVICE_PORT")
	if port == "" {
		port = "8081"
	}
	log.Printf("ML Service running on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
