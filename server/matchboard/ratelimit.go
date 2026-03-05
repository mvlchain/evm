package matchboard

import (
	"sync"
	"time"
)

type fixedWindowRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]rateBucket
}

type rateBucket struct {
	windowStart time.Time
	count       int
}

func newFixedWindowRateLimiter(limit int, window time.Duration) *fixedWindowRateLimiter {
	return &fixedWindowRateLimiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]rateBucket),
	}
}

func (r *fixedWindowRateLimiter) allow(principal string, now time.Time) bool {
	if r.limit <= 0 {
		return true
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, ok := r.buckets[principal]
	if !ok || now.Sub(bucket.windowStart) >= r.window {
		r.buckets[principal] = rateBucket{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if bucket.count >= r.limit {
		return false
	}

	bucket.count++
	r.buckets[principal] = bucket
	return true
}
