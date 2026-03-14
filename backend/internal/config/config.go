// File: backend/internal/config/config.go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the complete application configuration loaded from environment.
// Every field has a documented env-var name and a safe default.
type Config struct {
	App       AppConfig
	Server    ServerConfig
	LB        LBConfig
	Database  DatabaseConfig
	Redis     RedisConfig
	ML        MLConfig
	Telemetry TelemetryConfig
	Health    HealthConfig
}

type AppConfig struct {
	Env       string // APP_ENV (default: development)
	LogLevel  string // LOG_LEVEL (default: info)
	LogFormat string // LOG_FORMAT (default: json)
}

type ServerConfig struct {
	Port int // APP_PORT (default: 8080)
}

type LBConfig struct {
	Strategy string   // LB_STRATEGY (default: least_connections)
	Servers  []string // SERVERS comma-separated URLs
	// Per-server weights keyed by "host:port"
	Weights map[string]int
}

type DatabaseConfig struct {
	Host               string        // DB_HOST
	Port               int           // DB_PORT
	Name               string        // DB_NAME
	User               string        // DB_USER
	Password           string        // DB_PASSWORD
	SSLMode            string        // DB_SSLMODE
	MaxOpenConns       int           // DB_MAX_OPEN_CONNS
	MaxIdleConns       int           // DB_MAX_IDLE_CONNS
	ConnMaxLifetime    time.Duration // DB_CONN_MAX_LIFETIME_MINUTES
}

type RedisConfig struct {
	Addr     string // REDIS_ADDR
	Password string // REDIS_PASSWORD
	DB       int    // REDIS_DB
}

type MLConfig struct {
	Endpoint              string        // ML_MODEL_ENDPOINT
	TimeoutMs             int           // ML_MODEL_TIMEOUT_MS
	CircuitBreakerResetSec int          // ML_CIRCUIT_BREAKER_RESET_SECONDS
	CacheSize             int           // ML_CACHE_SIZE
}

type TelemetryConfig struct {
	OTELEnabled       bool   // OTEL_ENABLED
	ServiceName       string // OTEL_SERVICE_NAME
	OTLPEndpoint      string // OTEL_EXPORTER_OTLP_ENDPOINT
	PrometheusEnabled bool   // PROMETHEUS_ENABLED
}

type HealthConfig struct {
	IntervalSeconds int // HEALTH_CHECK_INTERVAL_SECONDS
	TimeoutSeconds  int // HEALTH_CHECK_TIMEOUT_SECONDS
	Retries         int // HEALTH_CHECK_RETRIES
}

// Load reads configuration from environment variables.
// It attempts to load a .env file first (silently ignores absence in production).
func Load() (*Config, error) {
	// Load .env if present; ignore error (file may not exist in containers)
	_ = godotenv.Load()

	cfg := &Config{
		App: AppConfig{
			Env:       getEnv("APP_ENV", "development"),
			LogLevel:  getEnv("LOG_LEVEL", "info"),
			LogFormat: getEnv("LOG_FORMAT", "json"),
		},
		Server: ServerConfig{
			Port: getEnvInt("APP_PORT", 8080),
		},
		LB: LBConfig{
			Strategy: strings.ToLower(getEnv("LB_STRATEGY", "least_connections")),
			Servers:  parseServerList(getEnv("SERVERS", "")),
			Weights:  parseWeights(),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvInt("DB_PORT", 5432),
			Name:            getEnv("DB_NAME", "neurabalancer"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", ""),
			SSLMode:         getEnv("DB_SSLMODE", "disable"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 30)) * time.Minute,
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		ML: MLConfig{
			Endpoint:              getEnv("ML_MODEL_ENDPOINT", "http://ml-service:8081"),
			TimeoutMs:             getEnvInt("ML_MODEL_TIMEOUT_MS", 300),
			CircuitBreakerResetSec: getEnvInt("ML_CIRCUIT_BREAKER_RESET_SECONDS", 30),
			CacheSize:             getEnvInt("ML_CACHE_SIZE", 1000),
		},
		Telemetry: TelemetryConfig{
			OTELEnabled:       getEnvBool("OTEL_ENABLED", true),
			ServiceName:       getEnv("OTEL_SERVICE_NAME", "neurabalancer"),
			OTLPEndpoint:      getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel-collector:4317"),
			PrometheusEnabled: getEnvBool("PROMETHEUS_ENABLED", true),
		},
		Health: HealthConfig{
			IntervalSeconds: getEnvInt("HEALTH_CHECK_INTERVAL_SECONDS", 5),
			TimeoutSeconds:  getEnvInt("HEALTH_CHECK_TIMEOUT_SECONDS", 3),
			Retries:         getEnvInt("HEALTH_CHECK_RETRIES", 3),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// DSN returns the PostgreSQL connection string.
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// Addr returns "host:port" for the HTTP server.
func (c *ServerConfig) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func (c *Config) validate() error {
	if len(c.LB.Servers) == 0 {
		return fmt.Errorf("SERVERS must be set (comma-separated backend URLs)")
	}
	validStrategies := map[string]bool{
		"round_robin": true, "weighted_round_robin": true,
		"least_connections": true, "random": true, "ml": true,
	}
	if !validStrategies[c.LB.Strategy] {
		return fmt.Errorf("LB_STRATEGY %q is not valid (choose from: round_robin, weighted_round_robin, least_connections, random, ml)", c.LB.Strategy)
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func parseServerList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// parseWeights reads all env vars matching SERVER_WEIGHT_<key> and returns
// a map[<url-safe-host-port>]weight used by WeightedRoundRobin.
func parseWeights() map[string]int {
	weights := map[string]int{}
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "SERVER_WEIGHT_") {
			continue
		}
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], "SERVER_WEIGHT_")
		// Normalise: replace _ back to - or : as needed
		// Key format in env: backend-1_8001 → "backend-1:8001"
		hostPort := strings.ReplaceAll(key, "_", ":")
		w, err := strconv.Atoi(parts[1])
		if err == nil && w > 0 {
			weights[hostPort] = w
		}
	}
	return weights
}
