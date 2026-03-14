// File: backend/internal/loadbalancer/factory.go
package loadbalancer

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/souvik03-136/neurabalancer/backend/internal/config"
	"github.com/souvik03-136/neurabalancer/backend/internal/metrics"
)

// NewStrategy creates the correct Strategy implementation from the config string.
func NewStrategy(name string, mlCfg *config.MLConfig, col *metrics.Collector, logger *zap.Logger) (Strategy, error) {
	switch name {
	case "round_robin":
		logger.Info("using Round Robin strategy")
		return NewRoundRobin(), nil
	case "weighted_round_robin":
		logger.Info("using Weighted Round Robin strategy")
		return NewWeightedRoundRobin(), nil
	case "least_connections":
		logger.Info("using Least Connections strategy")
		return NewLeastConnections(), nil
	case "random":
		logger.Info("using Random strategy")
		return NewRandom(), nil
	case "ml":
		logger.Info("using ML strategy", zap.String("endpoint", mlCfg.Endpoint))
		return NewMLStrategy(mlCfg, col, logger), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q", name)
	}
}
