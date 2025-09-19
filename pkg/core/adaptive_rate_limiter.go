package core

import (
	"sync"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/rs/zerolog/log"
)

// AdaptiveRateLimiter manages GitHub API rate limiting using response headers.
type AdaptiveRateLimiter struct {
	mu           sync.Mutex
	remaining    int
	resetTime    time.Time
	safetyBuffer int // Number of requests to keep as buffer
}

// NewAdaptiveRateLimiter creates a new adaptive rate limiter.
func NewAdaptiveRateLimiter() *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		remaining:    30, // GitHub search API default limit
		resetTime:    time.Now().Add(2 * time.Minute),
		safetyBuffer: 27, // Keep 27 requests as safety buffer
	}
}

// UpdateFromResponse updates rate limit state from GitHub API response.
func (rl *AdaptiveRateLimiter) UpdateFromResponse(resp *github.Response) {
	if resp == nil || resp.Response == nil {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Parse rate limit headers.
	rl.remaining = resp.Rate.Remaining
	rl.resetTime = resp.Rate.Reset.Time

	log.Debug().
		Int("remaining", rl.remaining).
		Time("reset", rl.resetTime).
		Msg("Updated rate limit info from GitHub response")
}

// ShouldWait determines if we should wait before making the next request.
func (rl *AdaptiveRateLimiter) ShouldWait() (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// If we have enough requests remaining, no need to wait
	if rl.remaining > rl.safetyBuffer {
		return false, 0
	}

	// Calculate time until reset
	now := time.Now()
	if now.After(rl.resetTime) {
		// Rate limit should have reset, update our state
		rl.remaining = 30 // Reset to default GitHub limit
		rl.resetTime = now.Add(time.Minute)
		return false, 0
	}

	// We're close to the limit, wait until reset
	waitTime := rl.resetTime.Sub(now)

	log.Debug().
		Int("remaining", rl.remaining).
		Dur("waitTime", waitTime).
		Msg("Rate limit approaching, waiting for reset")

	return true, waitTime
}
