// File: ml/model-server/main.go
package main

import (
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
	loaded  bool // true only when model + scaler are ready
}

// newModelServer attempts to load the model and scaler.
// Returns (nil, err) if files are missing — caller should start in degraded mode.
// Returns (nil, err) with a descriptive error for any other failure.
func newModelServer(cfg serverConfig) (*modelServer, error) {
	ms := &modelServer{cfg: cfg}

	// Check files exist before touching ONNX runtime
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
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("session options: %w", err)
	}
	defer opts.Destroy()

	inputName := getEnv("MODEL_INPUT_NAME", "features")
	outputName := getEnv("MODEL_OUTPUT_NAME", "predicted_score")

	ms.session, err = ort.NewDynamicAdvancedSession(
		cfg.ModelPath,
		[]string{inputName},
		[]string{outputName},
		opts,
	)
	if err != nil {
		_ = ort.DestroyEnvironment()
		return nil, fmt.Errorf("load model: %w", err)
	}

	ms.loaded = true
	log.Printf("model loaded — path=%s input=%s output=%s", cfg.ModelPath, inputName, outputName)
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
	defer inTensor.Destroy()

	outShape := ort.NewShape(1, 1)
	outTensor, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		return 0, fmt.Errorf("output tensor: %w", err)
	}
	defer outTensor.Destroy()

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

type predictResponse struct {
	Predictions []float32 `json:"predictions"`
}

// buildHandlers returns a mux.Router.
// ms may be nil (degraded mode — no model loaded).
func buildHandlers(ms *modelServer) *mux.Router {
	r := mux.NewRouter()

	// Health — always 200 so Docker/compose never marks the container unhealthy
	// due to missing model. Status field tells the load balancer the real state.
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		status := "ok"
		if ms == nil || !ms.loaded {
			status = "degraded - no model loaded. Run: task ml-train"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
	}).Methods(http.MethodGet)

	// Version
	r.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		version := "none - model not trained yet"
		if ms != nil && ms.loaded {
			version = getEnv("MODEL_VERSION", "1.0.0")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"model_version": version,
			"onnx_version":  "1.16.3",
		})
	}).Methods(http.MethodGet)

	// Predict — returns 503 when model is not loaded so the load balancer
	// circuit breaker trips and falls back to least_connections automatically.
	r.HandleFunc("/predict", func(w http.ResponseWriter, r *http.Request) {
		if ms == nil || !ms.loaded {
			http.Error(w, "model not loaded - train a model first with: task ml-train", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

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
	}).Methods(http.MethodPost)

	return r
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	cfg := loadConfig()

	// Attempt to load model — if it fails, start in degraded mode (no crash).
	ms, err := newModelServer(cfg)
	if err != nil {
		log.Printf("WARNING: starting in degraded mode — %v", err)
		log.Printf("The /predict endpoint will return 503 until a model is trained.")
		log.Printf("To train: task ml-train  then  docker compose restart ml-service")
		ms = nil
	}

	// Cleanup on exit
	defer func() {
		if ms != nil && ms.session != nil {
			ms.session.Destroy()
		}
		// Only destroy ONNX env if it was successfully initialised (i.e. ms != nil)
		if ms != nil {
			_ = ort.DestroyEnvironment()
		}
	}()

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      buildHandlers(ms),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("ML model server listening on :%s (loaded=%v)", cfg.Port, ms != nil)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Printf("signal %s — shutting down", sig)
	case err := <-serverErr:
		log.Fatalf("server error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("ML model server stopped")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
