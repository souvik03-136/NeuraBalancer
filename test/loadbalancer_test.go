package test

import (
	"testing"

	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
	"github.com/stretchr/testify/assert"
)

func TestRoundRobinStrategy(t *testing.T) {
	servers := []string{"http://server1", "http://server2", "http://server3"}
	lb := loadbalancer.NewRoundRobinStrategy()

	// Calls should return servers in a cyclic order
	assert.Equal(t, "http://server1", lb.SelectServer(servers))
	assert.Equal(t, "http://server2", lb.SelectServer(servers))
	assert.Equal(t, "http://server3", lb.SelectServer(servers))
	assert.Equal(t, "http://server1", lb.SelectServer(servers)) // Loop back
}

func TestLeastConnectionsStrategy(t *testing.T) {
	servers := []string{"http://server1", "http://server2", "http://server3"}
	lb := loadbalancer.NewLeastConnectionsStrategy()

	// Simulate active connections
	lb.UpdateConnections("http://server1", 3)
	lb.UpdateConnections("http://server2", 1) // Least connections
	lb.UpdateConnections("http://server3", 2)

	assert.Equal(t, "http://server2", lb.SelectServer(servers)) // Should return server2
}

func TestWeightedRoundRobinStrategy(t *testing.T) {
	servers := []string{"http://server1", "http://server2", "http://server3"}
	weights := map[string]int{
		"http://server1": 1,
		"http://server2": 2, // Higher weight
		"http://server3": 1,
	}
	lb := loadbalancer.NewWeightedRoundRobinStrategy(weights)

	serverCounts := map[string]int{}
	for i := 0; i < 4; i++ {
		serverCounts[lb.SelectServer(servers)]++
	}

	assert.GreaterOrEqual(t, serverCounts["http://server2"], 2) // Should be selected more often
}

func TestRandomSelectionStrategy(t *testing.T) {
	servers := []string{"http://server1", "http://server2", "http://server3"}
	lb := loadbalancer.NewRandomSelectionStrategy()

	selected := lb.SelectServer(servers)
	assert.Contains(t, servers, selected)
}
