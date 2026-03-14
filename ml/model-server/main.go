// File: ml/model-server/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	ort "github.com/yalue/onnxruntime_go"
)

// ─── Config ───────────────────────────────────────────────────────────────────

type serverConfig struct {
	Port            string
	ModelPath       string
	ScalerPath      string
	OnnxLibPath     string
	ShutdownTimeout time.Duration
}

func loadConfig() serverConfig {
	_ = godotenv.Load()
	return serverConfig{
		Port:            getEnv("ML_SERVICE_PORT", "8081"),
		ModelPath:       getEnv("MODEL_PATH", "ml/models/load_balancer.onnx"),
		ScalerPath:      getEnv("SCALER_PATH", "ml/models/scaler.json"),
		OnnxLibPath:     getEnv("ONNX_LIB_PATH", onnxLibDefault()),
		ShutdownTimeout: time.Duration(getEnvInt("SHUTDOWN_TIMEOUT_SECONDS", 10)) * time.Second,
	}
}

func onnxLibDefault() string {
	if runtime.GOOS == "windows" {
		return getEnv("ONNX_LIB_PATH_WIN", "onnxruntime.dll")
	}
	return "libonnxruntime.so"
}

// ─── Scaler ───────────────────────────────────────────────────────────────────

const expectedFeatures = 6

type scaler struct {
	Mean  []float32 `json:"mean"`
	Scale []float32 `json:"scale"`
}

func (sc *scaler) validate() error {
	if len(sc.Mean) != expectedFeatures {
		return fmt.Errorf("scaler mean length %d != expected %d", len(sc.Mean), expectedFeatures)
	}
	if len(sc.Scale) != expectedFeatures {
		return fmt.Errorf("scaler scale length %d != expected %d", len(sc.Scale), expectedFeatures)
	}
	return nil
}

func (sc *scaler) normalize(features []float32) []float32 {
	out := make([]float32, len(features))
	for i, f := range features {
		if sc.Scale[i] == 0 {
			out[i] = 0
			continue
		}
		out[i] = (f - sc.Mean[i]) / sc.Scale[i]
	}
	return out
}

// ─── Model Server ─────────────────────────────────────────────────────────────

type modelServer struct {
	cfg     serverConfig
	sc      scaler
	session *ort.DynamicAdvancedSession
	mu      sync.RWMutex
}

func newModelServer(cfg serverConfig) (*modelServer, error) {
	ms := &modelServer{cfg: cfg}

	// Validate files exist
	for _, path := range []string{cfg.ModelPath, cfg.ScalerPath} {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("required file missing %q: %w", path, err)
		}
	}

	// Load scaler
	raw, err := os.ReadFile(cfg.ScalerPath)
	if err != nil {
		return nil, fmt.Errorf("read scaler: %w", err)
	}
	if err := json.Unmarshal(raw, &ms.sc); err != nil {
		return nil, fmt.Errorf("parse scaler: %w", err)
	}
	if err := ms.sc.validate(); err != nil {
		return nil, fmt.Errorf("invalid scaler: %w", err)
	}

	// Init ONNX runtime
	ort.SetSharedLibraryPath(cfg.OnnxLibPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnx init: %w", err)
	}

	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("session options: %w", err)
	}
	defer func() {
		if err := opts.Destroy(); err != nil {
			log.Printf("opts destroy error: %v", err)
		}
	}()

	// FIX: input/output names now match what PyTorch onnx.export produces.
	// Training code exports with input_names=['features'], output_names=['predicted_score'].
	// If you retrain with TF, set MODEL_INPUT_NAME / MODEL_OUTPUT_NAME env vars.
	inputName := getEnv("MODEL_INPUT_NAME", "features")
	outputName := getEnv("MODEL_OUTPUT_NAME", "predicted_score")

	ms.session, err = ort.NewDynamicAdvancedSession(
		cfg.ModelPath,
		[]string{inputName},
		[]string{outputName},
		opts,
	)
	if err != nil {
		return nil, fmt.Errorf("load model: %w", err)
	}

	log.Printf("model server ready — model=%s input=%s output=%s",
		cfg.ModelPath, inputName, outputName)
	return ms, nil
}

func (ms *modelServer) predict(features []float32) (float32, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	norm := ms.sc.normalize(features)
	shape := ort.NewShape(1, int64(len(norm)))

	inTensor, err := ort.NewTensor[float32](shape, norm)
	if err != nil {
		return 0, fmt.Errorf("input tensor: %w", err)
	}
	defer func() {
		if err := inTensor.Destroy(); err != nil {
			log.Printf("inTensor destroy error: %v", err)
		}
	}()

	outShape := ort.NewShape(1, 1)
	outTensor, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return 0, fmt.Errorf("output tensor: %w", err)
	}
	defer func() {
		if err := outTensor.Destroy(); err != nil {
			log.Printf("outTensor destroy error: %v", err)
		}
	}()

	if err := ms.session.Run(
		[]ort.ArbitraryTensor{inTensor},
		[]ort.ArbitraryTensor{outTensor},
	); err != nil {
		return 0, fmt.Errorf("inference: %w", err)
	}

	data := outTensor.GetData()
	if len(data) == 0 {
		return 0, errors.New("empty inference output")
	}
	return data[0], nil
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

type predictRequest struct {
	Servers []struct {
		ServerID    int     `json:"server_id"`
		CPUUsage    float32 `json:"cpu_usage"`
		MemoryUsage float32 `json:"memory_usage"`
		ActiveConns int     `json:"active_conns"`
		ErrorRate   float32 `json:"error_rate"`
		ResponseP95 float32 `json:"response_p95"`
		Weight      int     `json:"weight"`
		Capacity    int     `json:"capacity"`
	} `json:"servers"`
}

// FIX: response field is now "predictions" to match what the Go strategy expects.
type predictResponse struct {
	Predictions []float32 `json:"predictions"`
}

func (ms *modelServer) handlePredict(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	var req predictRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if len(req.Servers) == 0 {
		http.Error(w, "servers list is empty", http.StatusBadRequest)
		return
	}

	scores := make([]float32, len(req.Servers))
	for i, srv := range req.Servers {
		feats := []float32{
			srv.CPUUsage,
			srv.MemoryUsage,
			float32(srv.ActiveConns),
			srv.ErrorRate,
			srv.ResponseP95,
			float32(srv.Capacity),
		}
		if len(feats) != expectedFeatures {
			http.Error(w, fmt.Sprintf("server %d: wrong feature count", i), http.StatusBadRequest)
			return
		}
		score, err := ms.predict(feats)
		if err != nil {
			log.Printf("prediction error for server %d: %v", srv.ServerID, err)
			http.Error(w, "prediction failed", http.StatusInternalServerError)
			return
		}
		scores[i] = score
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(predictResponse{Predictions: scores})
}

func (ms *modelServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (ms *modelServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"model_version": getEnv("MODEL_VERSION", "1.0.0"),
		"onnx_version":  "1.16.3",
	})
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	os.Exit(run())
}

func run() int {
	cfg := loadConfig()

	var ms *modelServer
	defer func() {
		if ms != nil && ms.session != nil {
			if err := ms.session.Destroy(); err != nil {
				log.Printf("session destroy error: %v", err)
			}
		}
		if err := ort.DestroyEnvironment(); err != nil {
			log.Printf("ort destroy error: %v", err)
		}
	}()

	var err error
	ms, err = newModelServer(cfg)
	if err != nil {
		if err := ort.DestroyEnvironment(); err != nil {
			log.Printf("ort destroy error: %v", err)
		}
		log.Printf("model server init failed: %v", err)
		return 1
	}

	r := mux.NewRouter()
	r.HandleFunc("/health", ms.handleHealth).Methods(http.MethodGet)
	r.HandleFunc("/version", ms.handleVersion).Methods(http.MethodGet)
	r.HandleFunc("/predict", ms.handlePredict).Methods(http.MethodPost)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("ML model server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("signal %s received, shutting down", sig)
	case err := <-serverErr:
		log.Printf("server error: %v", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("ML model server stopped")
	return 0
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil {
		return fallback
	}
	return n
}
