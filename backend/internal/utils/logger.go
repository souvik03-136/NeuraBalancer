package utils

import (
	"log"
)

// Logger struct
type Logger struct{}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	return &Logger{}
}

// Info logs info messages
func (l *Logger) Info(v ...interface{}) {
	log.Println("[INFO]", v)
}

// Fatal logs fatal errors
func (l *Logger) Fatal(v ...interface{}) {
	log.Fatal("[FATAL]", v)
}
