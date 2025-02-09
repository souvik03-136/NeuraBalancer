package loadbalancer

import (
	"math/rand"
	"sync"
)

// Strategy interface
type Strategy interface {
	SelectServer(servers []string) string
}

// RoundRobinStrategy struct
type RoundRobinStrategy struct {
	counter int
	mu      sync.Mutex
}

// SelectServer (Round Robin)
func (r *RoundRobinStrategy) SelectServer(servers []string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	server := servers[r.counter%len(servers)]
	r.counter++
	return server
}

// LeastConnectionsStrategy struct
type LeastConnectionsStrategy struct {
	serverConnections map[string]int
	mu                sync.Mutex
}

// SelectServer (Least Connections)
func (lc *LeastConnectionsStrategy) SelectServer(servers []string) string {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// Initialize connection counts if empty
	if lc.serverConnections == nil {
		lc.serverConnections = make(map[string]int)
		for _, server := range servers {
			lc.serverConnections[server] = 0
		}
	}

	// Find the server with the least connections
	minServer := servers[0]
	minConnections := lc.serverConnections[minServer]

	for _, server := range servers {
		if lc.serverConnections[server] < minConnections {
			minServer = server
			minConnections = lc.serverConnections[server]
		}
	}

	// Simulate increasing connection count (in real-world, update dynamically)
	lc.serverConnections[minServer]++

	return minServer
}

// WeightedRoundRobinStrategy struct
type WeightedRoundRobinStrategy struct {
	serverWeights  map[string]int
	currentWeights map[string]int
	mu             sync.Mutex
}

// SelectServer (Weighted Round Robin)
func (w *WeightedRoundRobinStrategy) SelectServer(servers []string) string {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Initialize weights if empty
	if w.serverWeights == nil {
		w.serverWeights = map[string]int{
			"http://server1": 3,
			"http://server2": 1,
			"http://server3": 2,
		}
		w.currentWeights = make(map[string]int)
	}

	// Increase each server's weight
	for server, weight := range w.serverWeights {
		w.currentWeights[server] += weight
	}

	// Select server with highest current weight
	selectedServer := servers[0]
	for _, server := range servers {
		if w.currentWeights[server] > w.currentWeights[selectedServer] {
			selectedServer = server
		}
	}

	// Reduce the weight of the selected server
	w.currentWeights[selectedServer] -= sumWeights(w.serverWeights)

	return selectedServer
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

// SelectServer (Random Selection)
func (r *RandomSelectionStrategy) SelectServer(servers []string) string {
	return servers[rand.Intn(len(servers))]
}
