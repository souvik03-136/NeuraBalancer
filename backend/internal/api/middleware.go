package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware handles Cross-Origin Resource Sharing (CORS) settings.
func CORSMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		ctx.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
		ctx.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		ctx.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		ctx.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if ctx.Request.Method == "OPTIONS" {
			ctx.AbortWithStatus(http.StatusNoContent)
			return
		}

		ctx.Next()
	}
}

// RequestLogger logs details of each incoming request.
func RequestLogger() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		log.Printf("➡️  [%s] %s - %s", ctx.Request.Method, ctx.Request.URL.Path, ctx.ClientIP())
		ctx.Next()
		log.Printf("⬅️  [%d] %s", ctx.Writer.Status(), ctx.Request.URL.Path)
	}
}
