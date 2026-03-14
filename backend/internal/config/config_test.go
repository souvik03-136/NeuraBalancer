// File: backend/internal/config/config_test.go
package config_test

import (
	"os"
	"testing"

	"github.com/souvik03-136/neurabalancer/backend/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Provide the bare minimum required fields
	t.Setenv("SERVERS", "http://localhost:8001")
	t.Setenv("LB_STRATEGY", "round_robin")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.App.Env != "development" {
		t.Errorf("default env = %q, want development", cfg.App.Env)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("default MaxOpenConns = %d, want 25", cfg.Database.MaxOpenConns)
	}
}

func TestLoad_ValidationFailsOnEmptyServers(t *testing.T) {
	os.Unsetenv("SERVERS")
	_, err := config.Load()
	if err == nil {
		t.Error("expected validation error for missing SERVERS, got nil")
	}
}

func TestLoad_ValidationFailsOnBadStrategy(t *testing.T) {
	t.Setenv("SERVERS", "http://localhost:8001")
	t.Setenv("LB_STRATEGY", "invalid_strategy")
	_, err := config.Load()
	if err == nil {
		t.Error("expected validation error for invalid strategy, got nil")
	}
}

func TestDatabaseDSN(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		User:     "user",
		Password: "pass",
		Name:     "mydb",
		SSLMode:  "require",
	}
	want := "host=db.example.com port=5432 user=user password=pass dbname=mydb sslmode=require"
	if got := cfg.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := config.ServerConfig{Port: 9090}
	if got := cfg.Addr(); got != ":9090" {
		t.Errorf("Addr() = %q, want :9090", got)
	}
}
