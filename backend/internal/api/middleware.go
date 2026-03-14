// File: backend/internal/api/middleware.go
package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

// RequestID injects a unique request ID into every request.
func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			id := c.Request().Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = uuid.New().String()
			}
			c.Request().Header.Set(echo.HeaderXRequestID, id)
			c.Response().Header().Set(echo.HeaderXRequestID, id)
			return next(c)
		}
	}
}

// StructuredLogger logs each request with structured fields for Loki.
func StructuredLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			duration := time.Since(start)

			req := c.Request()
			res := c.Response()

			fields := []zap.Field{
				zap.String("request_id", req.Header.Get(echo.HeaderXRequestID)),
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.Int("status", res.Status),
				zap.Duration("duration", duration),
				zap.String("remote_ip", c.RealIP()),
				zap.Int64("response_bytes", res.Size),
			}

			if err != nil {
				fields = append(fields, zap.Error(err))
				logger.Error("request error", fields...)
			} else {
				switch {
				case res.Status >= 500:
					logger.Error("server error", fields...)
				case res.Status >= 400:
					logger.Warn("client error", fields...)
				default:
					logger.Info("request", fields...)
				}
			}

			return err
		}
	}
}

// CORS returns a permissive CORS middleware suitable for API services.
// In production, restrict AllowOrigins to your domain.
func CORS() echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodDelete, http.MethodOptions,
		},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", echo.HeaderXRequestID},
		ExposeHeaders:    []string{"Content-Length", echo.HeaderXRequestID},
		AllowCredentials: false,
		MaxAge:           86400,
	})
}

// RateLimiter provides basic per-IP rate limiting.
func RateLimiter() echo.MiddlewareFunc {
	return middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100))
}

// Recover returns Echo's panic recovery middleware configured to log via zap.
func Recover(logger *zap.Logger) echo.MiddlewareFunc {
	return middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			logger.Error("panic recovered",
				zap.Error(err),
				zap.ByteString("stack", stack),
			)
			return nil
		},
	})
}
