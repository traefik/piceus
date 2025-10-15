package client

import (
	"context"
	"net/http"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/hashicorp/go-retryablehttp"
)

// Option implements options for HTTP client.
type Option interface {
	Apply(ctx context.Context, client *Client) error
}

// Client holds Github client with its underlying HTTP client and all its middlewares.
type Client struct {
	gh     *github.Client
	client *http.Client
}

// New creates a new client with optional middleware.
func New(ctx context.Context, options ...Option) (*Client, error) {
	c := &Client{
		client: &http.Client{
			Transport: http.DefaultTransport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
             // Disable redirect following to allow go-github library to detect 302 responses
             // for archive links. The library expects 302 status codes but the default client
             // follows redirects and returns 200 instead.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}

	for _, opt := range options {
		err := opt.Apply(ctx, c)
		if err != nil {
			return nil, err
		}
	}

	c.gh = github.NewClient(c.client)

	return c, nil
}

// GithubClient provides Github client with underlying HTTP middleware set.
func (c *Client) GithubClient() *github.Client {
	return c.gh
}

// WithToken adds authentification middleware to HTTP client.
func WithToken(token string) Option {
	a := authClient{token: token}
	return a
}

// WithMetrics adds metrics middleware to HTTP client.
func WithMetrics(enable bool) Option {
	m := metricsClient{enabled: enable}
	return m
}

// WithRateLimiter adds adaptative rate limiter middleware to HTTP client.
func WithRateLimiter(remaining, safetyBuffer int, resetTime time.Time) Option {
	arl := &adaptiveRateLimiter{
		remaining:    remaining, // GitHub search API default limit
		resetTime:    resetTime,
		safetyBuffer: safetyBuffer, // requests to keep as safety buffer
	}
	return arl
}

// WithRetry adds retry middleware to HTTP client.
func WithRetry(retryMax int, retryWaitMin time.Duration) Option {
	r := retryClient{
		retryClient: retryablehttp.NewClient(),
	}

	r.retryClient.RetryMax = retryMax
	r.retryClient.RetryWaitMin = retryWaitMin

	return r
}
