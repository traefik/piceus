package core

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdaptiveRateLimiter(t *testing.T) {
	rl := NewAdaptiveRateLimiter()

	require.NotNil(t, rl)
	assert.Equal(t, 30, rl.remaining)
	assert.Equal(t, 27, rl.safetyBuffer)
	assert.True(t, rl.resetTime.After(time.Now()))
	assert.True(t, rl.resetTime.Before(time.Now().Add(2*time.Minute)))
}

func TestAdaptiveRateLimiter_UpdateFromResponse(t *testing.T) {
	tests := []struct {
		desc     string
		response *github.Response
	}{
		{
			desc:     "handles nil response gracefully",
			response: nil,
		},
		{
			desc: "handles response with nil http response",
			response: &github.Response{
				Response: nil,
			},
		},
		{
			desc: "updates rate limit from valid response",
			response: &github.Response{
				Response: &http.Response{},
				Rate: github.Rate{
					Remaining: 25,
					Reset:     github.Timestamp{Time: time.Now().Add(30 * time.Second)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			rl := NewAdaptiveRateLimiter()
			initialRemaining := rl.remaining
			initialResetTime := rl.resetTime

			rl.UpdateFromResponse(tt.response)

			if tt.response != nil && tt.response.Response != nil {
				// Should update values from response
				assert.Equal(t, tt.response.Rate.Remaining, rl.remaining)
				assert.Equal(t, tt.response.Rate.Reset.Time, rl.resetTime)
			} else {
				// Should not change values for nil/invalid responses
				assert.Equal(t, initialRemaining, rl.remaining)
				assert.Equal(t, initialResetTime, rl.resetTime)
			}
		})
	}
}

func TestAdaptiveRateLimiter_ShouldWait(t *testing.T) {
	tests := []struct {
		desc           string
		remaining      int
		resetTime      time.Time
		safetyBuffer   int
		expectWait     bool
		expectWaitTime bool // true if we should get a positive wait time
	}{
		{
			desc:         "continues when plenty of requests remaining",
			remaining:    20,
			resetTime:    time.Now().Add(30 * time.Second),
			safetyBuffer: 15,
			expectWait:   false,
		},
		{
			desc:           "waits when exactly at safety buffer",
			remaining:      15,
			resetTime:      time.Now().Add(30 * time.Second),
			safetyBuffer:   15,
			expectWait:     true,
			expectWaitTime: true,
		},
		{
			desc:           "waits when below safety buffer",
			remaining:      10,
			resetTime:      time.Now().Add(30 * time.Second),
			safetyBuffer:   15,
			expectWait:     true,
			expectWaitTime: true,
		},
		{
			desc:         "resets state when reset time passed",
			remaining:    5,
			resetTime:    time.Now().Add(-30 * time.Second),
			safetyBuffer: 15,
			expectWait:   false,
		},
		{
			desc:         "handles zero requests with expired reset time",
			remaining:    0,
			resetTime:    time.Now().Add(-1 * time.Minute),
			safetyBuffer: 15,
			expectWait:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			rl := NewAdaptiveRateLimiter()
			rl.remaining = tt.remaining
			rl.resetTime = tt.resetTime
			rl.safetyBuffer = tt.safetyBuffer

			shouldWait, waitTime := rl.ShouldWait()

			assert.Equal(t, tt.expectWait, shouldWait)

			if tt.expectWaitTime {
				assert.Positive(t, waitTime)
			} else {
				assert.Equal(t, time.Duration(0), waitTime)
			}

			// If reset time was in the past, check that state was reset
			if tt.resetTime.Before(time.Now()) && !tt.expectWait {
				assert.Equal(t, 30, rl.remaining)
				assert.True(t, rl.resetTime.After(time.Now()))
			}
		})
	}
}

func TestAdaptiveRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewAdaptiveRateLimiter()
	rl.remaining = 20 // Start with enough requests

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent UpdateFromResponse calls
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(i int) {
			defer wg.Done()
			resp := &github.Response{
				Response: &http.Response{},
				Rate: github.Rate{
					Remaining: 15 + i, // Different values
					Reset:     github.Timestamp{Time: time.Now().Add(time.Duration(i) * time.Second)},
				},
			}
			rl.UpdateFromResponse(resp)
		}(i)
	}
	wg.Wait()

	// Test concurrent ShouldWait calls
	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			_, _ = rl.ShouldWait()
		}()
	}
	wg.Wait()

	// Should not panic and should have valid state
	assert.GreaterOrEqual(t, rl.remaining, 0)
	assert.Positive(t, rl.safetyBuffer)
}

func TestAdaptiveRateLimiter_RealWorldScenario(t *testing.T) {
	rl := NewAdaptiveRateLimiter()

	// Simulate starting with fresh rate limit
	resp1 := &github.Response{
		Response: &http.Response{},
		Rate: github.Rate{
			Remaining: 30,
			Reset:     github.Timestamp{Time: time.Now().Add(1 * time.Minute)},
		},
	}
	rl.UpdateFromResponse(resp1)

	// Should not need to wait initially
	shouldWait, waitTime := rl.ShouldWait()
	require.False(t, shouldWait)
	assert.Equal(t, time.Duration(0), waitTime)

	// Simulate using up requests
	for i := 30; i > 15; i-- {
		resp := &github.Response{
			Response: &http.Response{},
			Rate: github.Rate{
				Remaining: i - 1,
				Reset:     github.Timestamp{Time: time.Now().Add(45 * time.Second)},
			},
		}
		rl.UpdateFromResponse(resp)
	}

	// Now should need to wait as we're at the safety buffer
	shouldWait, waitTime = rl.ShouldWait()
	require.True(t, shouldWait)
	assert.Positive(t, waitTime)
	assert.LessOrEqual(t, waitTime, 1*time.Minute) // Should be reasonable
}

func TestAdaptiveRateLimiter_SafetyBufferBehavior(t *testing.T) {
	tests := []struct {
		desc         string
		safetyBuffer int
		remaining    int
		expectWait   bool
	}{
		{
			desc:         "continues when above safety buffer",
			safetyBuffer: 5,
			remaining:    10,
			expectWait:   false,
		},
		{
			desc:         "waits when at safety buffer threshold",
			safetyBuffer: 5,
			remaining:    5,
			expectWait:   true,
		},
		{
			desc:         "waits when below safety buffer",
			safetyBuffer: 5,
			remaining:    3,
			expectWait:   true,
		},
		{
			desc:         "handles zero safety buffer edge case",
			safetyBuffer: 0,
			remaining:    1,
			expectWait:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			rl := NewAdaptiveRateLimiter()
			rl.safetyBuffer = tt.safetyBuffer
			rl.remaining = tt.remaining
			rl.resetTime = time.Now().Add(30 * time.Second)

			shouldWait, _ := rl.ShouldWait()
			assert.Equal(t, tt.expectWait, shouldWait)
		})
	}
}
