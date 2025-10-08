package client

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	headerRateRemaining = "X-RateLimit-Remaining"
	headerRateReset     = "X-RateLimit-Reset"
)

// adaptiveRateLimiter manages GitHub API rate limiting using responses headers.
type adaptiveRateLimiter struct {
	remaining    int
	resetTime    time.Time
	safetyBuffer int // Number of requests to keep as buffer
}

func (arl adaptiveRateLimiter) Apply(_ context.Context, c *Client) error {
	c.client.Transport = &adaptiveRateLimiterTripper{
		remaining:    arl.remaining,
		resetTime:    arl.resetTime,
		safetyBuffer: arl.safetyBuffer,

		next: c.client.Transport,
	}
	return nil
}

type adaptiveRateLimiterTripper struct {
	mu           sync.Mutex
	remaining    int
	resetTime    time.Time
	safetyBuffer int // Number of requests to keep as buffer

	next http.RoundTripper
}

func (rl *adaptiveRateLimiterTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := rl.next.RoundTrip(req)

	if resp == nil {
		return nil, err
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if remaining := resp.Header.Get(headerRateRemaining); remaining != "" {
		rl.remaining, _ = strconv.Atoi(remaining)
	}

	if reset := resp.Header.Get(headerRateReset); reset != "" {
		if v, _ := strconv.ParseInt(reset, 10, 64); v != 0 {
			rl.resetTime = time.Unix(v, 0)
		}
	}

	log.Debug().
		Int("remaining", rl.remaining).
		Time("reset", rl.resetTime).
		Msg("Updated rate limit info from GitHub responses")

	if shouldWait, waitTime := rl.shouldWait(); shouldWait {
		log.Debug().
			Dur("waitTime", waitTime).
			Msg("Adaptive throttling: waiting for rate limit reset")

		select {
		case <-time.After(waitTime):
			// Continue after wait
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}

	return resp, err
}

// ShouldWait determines if we should wait before making the next request.
func (rl *adaptiveRateLimiterTripper) shouldWait() (bool, time.Duration) {
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
