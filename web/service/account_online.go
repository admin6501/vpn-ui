package service

import (
	"maps"
	"sync"
	"time"
)

// Keep the beginning of the currently observed online interval in memory. The
// collector is the writer, so opening or refreshing a page cannot reset durations.
var accountOnlineStart = onlineStarts{}

type onlineStarts struct {
	mu    sync.RWMutex
	since map[string]int64
}

func (s *onlineStarts) observe(emails []string, now int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := make(map[string]int64, len(emails))
	for _, email := range emails {
		key := accountKey(email)
		start := s.since[key]
		if start == 0 {
			start = now
		}
		current[key] = start
	}
	s.since = current
}

func (s *onlineStarts) snapshot() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maps.Clone(s.since)
}

func ObserveOnlineAccounts(emails []string) {
	accountOnlineStart.observe(emails, time.Now().UnixMilli())
}
