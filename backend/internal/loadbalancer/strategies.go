// File: backend/internal/loadbalancer/strategies.go
package loadbalancer

import (
	"math/rand"
	"sync"
	"sync/atomic"
)

// Strategy selects a server from the healthy pool.
type Strategy interface {
	Select(servers []*Server) *Server
}

// ─── Round Robin ──────────────────────────────────────────────────────────────

// RoundRobin cycles through servers in order using an atomic counter.
// Safe for concurrent use without a mutex.
type RoundRobin struct {
	counter uint64
}

func NewRoundRobin() *RoundRobin { return &RoundRobin{} }

func (r *RoundRobin) Select(servers []*Server) *Server {
	if len(servers) == 0 {
		return nil
	}
	idx := atomic.AddUint64(&r.counter, 1) - 1
	return servers[int(idx)%len(servers)]
}

// ─── Weighted Round Robin ─────────────────────────────────────────────────────

// WeightedRoundRobin expands weights into a virtual slot pool and cycles through it.
// Re-builds the pool only when the server list changes (detected via len).
type WeightedRoundRobin struct {
	mu           sync.Mutex
	current      int
	slots        []*Server
	lastPoolSize int
}

func NewWeightedRoundRobin() *WeightedRoundRobin { return &WeightedRoundRobin{} }

func (w *WeightedRoundRobin) Select(servers []*Server) *Server {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(servers) == 0 {
		return nil
	}

	// Rebuild slot pool if server list length changed
	if len(servers) != w.lastPoolSize {
		w.slots = buildSlots(servers)
		w.lastPoolSize = len(servers)
		w.current = 0
	}

	if len(w.slots) == 0 {
		return servers[0]
	}

	s := w.slots[w.current%len(w.slots)]
	w.current++
	return s
}

func buildSlots(servers []*Server) []*Server {
	var slots []*Server
	for _, s := range servers {
		w := s.Weight
		if w <= 0 {
			w = 1
		}
		for i := 0; i < w; i++ {
			slots = append(slots, s)
		}
	}
	return slots
}

// ─── Least Connections ────────────────────────────────────────────────────────

// LeastConnections picks the server with the fewest active connections.
type LeastConnections struct{}

func NewLeastConnections() *LeastConnections { return &LeastConnections{} }

func (lc *LeastConnections) Select(servers []*Server) *Server {
	var best *Server
	bestConns := -1

	for _, s := range servers {
		s.mu.Lock()
		conns := s.Connections
		s.mu.Unlock()

		if best == nil || conns < bestConns {
			best = s
			bestConns = conns
		}
	}
	return best
}

// ─── Random ───────────────────────────────────────────────────────────────────

// Random picks a backend uniformly at random.
type Random struct{}

func NewRandom() *Random { return &Random{} }

func (r *Random) Select(servers []*Server) *Server {
	if len(servers) == 0 {
		return nil
	}
	return servers[rand.Intn(len(servers))]
}
