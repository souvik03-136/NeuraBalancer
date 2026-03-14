// File: go.mod
module github.com/souvik03-136/neurabalancer

go 1.21

require (
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/joho/godotenv v1.5.1
	github.com/labstack/echo/v4 v4.11.4
	github.com/lib/pq v1.10.9
	github.com/prometheus/client_golang v1.18.0
	github.com/prometheus/client_model v0.6.0
	github.com/redis/go-redis/v9 v9.4.0
	github.com/shirou/gopsutil/v3 v3.23.12
	github.com/yalue/onnxruntime_go v1.10.0
	go.opentelemetry.io/otel v1.22.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.22.0
	go.opentelemetry.io/otel/sdk v1.22.0
	go.opentelemetry.io/otel/trace v1.22.0
	go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho v0.47.0
	go.uber.org/zap v1.27.0
	google.golang.org/grpc v1.61.0
	github.com/gorilla/mux v1.8.1
	github.com/golang-migrate/migrate/v4 v4.17.0
)
