package loadbalancer

import (
	"math/rand"
	"sync"
)

// Strategy interface
type Strategy interface {
	SelectServer(servers []*Server) *Server
}

// RoundRobinStrategy struct
type RoundRobinStrategy struct {
	counter int
	mu      sync.Mutex
}

func (r *RoundRobinStrategy) SelectServer(servers []*Server) *Server {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(servers) == 0 {
		return nil
	}
	selected := servers[r.counter%len(servers)]
	r.counter++
	return selected
}

// LeastConnectionsStrategy struct
type LeastConnectionsStrategy struct{}

// SelectServer (Least Connections)
func (lc *LeastConnectionsStrategy) SelectServer(servers []*Server) *Server {
	var selected *Server
	minConns := -1

	for _, s := range servers {
		s.mu.Lock()
		conns := s.Connections
		s.mu.Unlock()

		if selected == nil || conns < minConns {
			minConns = conns
			selected = s
		}
	}
	return selected
}

// WeightedRoundRobinStrategy struct
type WeightedRoundRobinStrategy struct {
	currentIndex int
	mu           sync.Mutex
}

func (w *WeightedRoundRobinStrategy) SelectServer(servers []*Server) *Server {
	w.mu.Lock()
	defer w.mu.Unlock()

	totalWeight := 0
	for _, s := range servers {
		totalWeight += s.Weight
	}

	if totalWeight == 0 {
		return nil
	}

	w.currentIndex = (w.currentIndex + 1) % totalWeight
	current := 0

	for _, s := range servers {
		current += s.Weight
		if w.currentIndex < current {
			return s
		}
	}
	return nil
}

// Sum of all weights
func sumWeights(weights map[string]int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	return total
}

// RandomSelectionStrategy struct
type RandomSelectionStrategy struct{}

func (r *RandomSelectionStrategy) SelectServer(servers []*Server) *Server {
	if len(servers) == 0 {
		return nil
	}
	return servers[rand.Intn(len(servers))]
}
