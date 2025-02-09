package loadbalancer

import (
	"errors"
	"sync"
)

// LoadBalancer struct
type LoadBalancer struct {
	servers  []string
	mu       sync.Mutex
	strategy Strategy
}

// NewLoadBalancer initializes a load balancer with a given strategy
func NewLoadBalancer(strategy Strategy, servers []string) *LoadBalancer {
	return &LoadBalancer{
		servers:  servers,
		strategy: strategy,
	}
}

// GetServer selects a server based on the strategy
func (lb *LoadBalancer) GetServer() (string, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.servers) == 0 {
		return "", errors.New("no servers available")
	}

	return lb.strategy.SelectServer(lb.servers), nil
}
