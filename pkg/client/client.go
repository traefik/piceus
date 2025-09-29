package client

import (
	"context"
	"net/http"
	"time"

	"github.com/google/go-github/v57/github"
	"github.com/hashicorp/go-retryablehttp"
)

type Option interface {
	Apply(context.Context, *Client) error
}
type Client struct {
	gh     *github.Client
	client *http.Client
}

func New(ctx context.Context, options ...Option) (*Client, error) {
	c := &Client{
		client: &http.Client{Transport: http.DefaultTransport},
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

func (c *Client) GithubClient() *github.Client {
	return c.gh
}

func WithToken(token string) Option {
	a := AuthClient{token: token}
	return a
}

func WithMetrics(enable bool) Option {
	m := MetricsClient{enabled: enable}
	return m
}

func WithRateLimiter(remaining, safetyBuffer int, resetTime time.Time) Option {
	arl := &AdaptiveRateLimiter{
		remaining:    remaining, // GitHub search API default limit
		resetTime:    resetTime,
		safetyBuffer: safetyBuffer, // requests to keep as safety buffer
	}
	return arl
}

func WithRetry(retryMax int, retryWaitMin time.Duration) Option {
	r := RetryClient{
		retryClient: retryablehttp.NewClient(),
	}

	r.retryClient.RetryMax = retryMax
	r.retryClient.RetryWaitMin = retryWaitMin

	return r
}
