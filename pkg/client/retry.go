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
	}
	r.retryClient.Logger = log.Ctx(ctx)

	c.client.Transport = &retryablehttp.RoundTripper{Client: r.retryClient}

	return nil
}
