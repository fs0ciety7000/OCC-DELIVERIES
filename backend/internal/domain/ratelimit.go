package domain

import (
	"sync"
	"time"
)

// RateLimiter is a small in-memory sliding window limiter keyed by a string
// (an IP address). Safe for concurrent use; the clock is passed in (tests).
type RateLimiter struct {
	Max    int
	Window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
	ops  int
}

// NewRateLimiter allows max events per window and per key.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{Max: max, Window: window, hits: map[string][]time.Time{}}
}

// Allow records an event for key at now and reports whether it is allowed
// (a refused event is not recorded).
func (l *RateLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	l.ops++
	if l.ops%256 == 0 {
		l.gc(now)
	}
	recent := l.prune(l.hits[key], now)
	if len(recent) >= l.Max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func (l *RateLimiter) prune(list []time.Time, now time.Time) []time.Time {
	cut := 0
	for cut < len(list) && now.Sub(list[cut]) >= l.Window {
		cut++
	}
	return list[cut:]
}

// gc drops the keys without recent events (bounded memory).
func (l *RateLimiter) gc(now time.Time) {
	for k, v := range l.hits {
		if len(l.prune(v, now)) == 0 {
			delete(l.hits, k)
		}
	}
}
