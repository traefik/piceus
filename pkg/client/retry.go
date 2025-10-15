package client

import (
	"context"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
)

type retryClient struct {
	retryClient *retryablehttp.Client
}

func (r retryClient) Apply(ctx context.Context, c *Client) error {
	r.retryClient.HTTPClient = &http.Client{
		Transport: c.client.Transport,
		// Disable redirect following to allow go-github library to detect 302 responses
		// for archive links. The library expects 302 status codes but the default client
		// follows redirects and returns 200 instead.
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	r.retryClient.Logger = log.Ctx(ctx)

	c.client.Transport = &retryablehttp.RoundTripper{Client: r.retryClient}

	return nil
}
