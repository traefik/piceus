package client

import (
	"context"

	"golang.org/x/oauth2"
)

type AuthClient struct {
	token string
}

func (a AuthClient) Apply(ctx context.Context, c *Client) error {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: a.token},
	)

	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.client) // needed by oauth2
	c.client = oauth2.NewClient(ctx, ts)

	return nil
}
