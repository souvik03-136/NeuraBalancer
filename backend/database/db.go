package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Global DB instance
var DB *sql.DB

// LoadEnv loads environment variables from .env
func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Could not load .env file, using system environment variables")
	}
}

// InitDB initializes the PostgreSQL/TimescaleDB connection
func InitDB() error {
	LoadEnv()

	// Fetch environment variables with defaults
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "password")
	dbName := getEnv("DB_NAME", "neura_balancer")
	dbSSLMode := getEnv("DB_SSLMODE", "disable") // Ensuring SSL is disabled

	// DSN (Database Source Name)
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode,
	)

	log.Println("🔄 Connecting to database...")

	var err error
	DB, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("❌ Failed to connect to DB: %w", err)
	}

	// Verify connection
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("❌ Database ping failed: %w", err)
	}

	log.Println("✅ Database connected successfully")
	return nil
}

// CloseDB closes the database connection
func CloseDB() {
	if DB != nil {
		DB.Close()
		log.Println("🔌 Database connection closed")
	}
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
