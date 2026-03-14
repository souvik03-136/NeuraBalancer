// File: backend/internal/loadbalancer/strategies_test.go
package loadbalancer_test

import (
	"sync"
	"testing"

	"github.com/souvik03-136/neurabalancer/backend/internal/loadbalancer"
)

// makeServers creates n alive servers with default weight and capacity.
func makeServers(n int) []*loadbalancer.Server {
	servers := make([]*loadbalancer.Server, n)
	for i := range servers {
		servers[i] = &loadbalancer.Server{
			ID:       i + 1,
			URL:      fmt.Sprintf("http://backend-%d:800%d", i+1, i+1),
			Alive:    true,
			Weight:   1,
			Capacity: 100,
		}
	}
	return servers
}

func TestRoundRobin_Distribution(t *testing.T) {
	rr := loadbalancer.NewRoundRobin()
	servers := makeServers(3)

	counts := make(map[int]int)
	for i := 0; i < 300; i++ {
		s := rr.Select(servers)
		if s == nil {
			t.Fatal("Select returned nil")
		}
		counts[s.ID]++
	}

	// Each server should receive exactly 100 requests
	for _, s := range servers {
		if counts[s.ID] != 100 {
			t.Errorf("server %d got %d requests, want 100", s.ID, counts[s.ID])
		}
	}
}

func TestRoundRobin_ConcurrentSafe(t *testing.T) {
	rr := loadbalancer.NewRoundRobin()
	servers := makeServers(5)

	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := rr.Select(servers)
			if s == nil {
				t.Error("nil server returned")
			}
		}()
	}
	wg.Wait()
}

func TestRoundRobin_EmptyServers(t *testing.T) {
	rr := loadbalancer.NewRoundRobin()
	if s := rr.Select(nil); s != nil {
		t.Errorf("expected nil, got %v", s)
	}
}

func TestLeastConnections_PicksLowest(t *testing.T) {
	lc := loadbalancer.NewLeastConnections()
	servers := makeServers(3)
	servers[0].Connections = 10
	servers[1].Connections = 2
	servers[2].Connections = 7

	s := lc.Select(servers)
	if s == nil || s.ID != servers[1].ID {
		t.Errorf("expected server %d (least conns), got %v", servers[1].ID, s)
	}
}

func TestWeightedRoundRobin_RespectWeights(t *testing.T) {
	wrr := loadbalancer.NewWeightedRoundRobin()
	servers := makeServers(2)
	servers[0].Weight = 3
	servers[1].Weight = 1

	counts := make(map[int]int)
	for i := 0; i < 400; i++ {
		s := wrr.Select(servers)
		counts[s.ID]++
	}

	// server[0] should get ~75% of traffic
	ratio := float64(counts[servers[0].ID]) / float64(counts[servers[1].ID])
	if ratio < 2.5 || ratio > 3.5 {
		t.Errorf("weight ratio %.2f out of expected range [2.5, 3.5]", ratio)
	}
}

func TestRandom_ReturnsValid(t *testing.T) {
	r := loadbalancer.NewRandom()
	servers := makeServers(10)

	seen := make(map[int]bool)
	for i := 0; i < 1000; i++ {
		s := r.Select(servers)
		if s == nil {
			t.Fatal("nil server returned")
		}
		seen[s.ID] = true
	}

	// Over 1000 draws all 10 servers should appear at least once
	if len(seen) < 8 {
		t.Errorf("random strategy only hit %d unique servers out of 10", len(seen))
	}
}
