package stats

import (
	"sync"
	"time"
)

const historyLen = 60 // 60 samples = 60s at 1s intervals (or 30min at 30s)

type Sample struct {
	Time    time.Time `json:"time"`
	DLSpeed int64     `json:"dl"`
	ULSpeed int64     `json:"ul"`
}

type ServerStats struct {
	mu      sync.RWMutex
	samples []Sample
}

func (s *ServerStats) Push(dl, ul int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, Sample{
		Time:    time.Now(),
		DLSpeed: dl,
		ULSpeed: ul,
	})
	if len(s.samples) > historyLen {
		s.samples = s.samples[len(s.samples)-historyLen:]
	}
}

func (s *ServerStats) Get() []Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Sample, len(s.samples))
	copy(out, s.samples)
	return out
}

func (s *ServerStats) Latest() Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.samples) == 0 {
		return Sample{}
	}
	return s.samples[len(s.samples)-1]
}

// Collector holds stats for all servers
type Collector struct {
	mu      sync.RWMutex
	servers map[string]*ServerStats
}

func NewCollector() *Collector {
	return &Collector{servers: make(map[string]*ServerStats)}
}

func (c *Collector) Push(serverID string, dl, ul int64) {
	c.mu.Lock()
	s, ok := c.servers[serverID]
	if !ok {
		s = &ServerStats{}
		c.servers[serverID] = s
	}
	c.mu.Unlock()
	s.Push(dl, ul)
}

func (c *Collector) Get(serverID string) []Sample {
	c.mu.RLock()
	s, ok := c.servers[serverID]
	c.mu.RUnlock()
	if !ok {
		return nil
	}
	return s.Get()
}

func (c *Collector) Latest(serverID string) Sample {
	c.mu.RLock()
	s, ok := c.servers[serverID]
	c.mu.RUnlock()
	if !ok {
		return Sample{}
	}
	return s.Latest()
}
