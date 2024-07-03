package goproxy

import (
	"fmt"
	"net/http"
	"time"
)

// BasicAuthTransport HTTP transport for API authentication
type BasicAuthTransport struct {
	username string
	password string

	// Transport is the underlying HTTP transport to use when making requests.
	// It will default to http.DefaultTransport if nil.
	Transport http.RoundTripper
}

// NewBasicAuthTransport Creates a  new BasicAuthTransport
func NewBasicAuthTransport(username, password string) (*BasicAuthTransport, error) {
	if username == "" || password == "" {
		return nil, fmt.Errorf("credentials missing")
	}

	return &BasicAuthTransport{username: username, password: password}, nil
}

// RoundTrip executes a single HTTP transaction
func (t *BasicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	enrichedReq := &http.Request{}
	*enrichedReq = *req

	enrichedReq.Header = make(http.Header, len(req.Header))
	for k, s := range req.Header {
		enrichedReq.Header[k] = append([]string(nil), s...)
	}

	if t.username != "" && t.password != "" {
		enrichedReq.SetBasicAuth(t.username, t.password)
	}

	return t.transport().RoundTrip(enrichedReq)
}

// Wrap Wrap a HTTP client Transport with the BasicAuthTransport
func (t *BasicAuthTransport) Wrap(client *http.Client) *http.Client {
	backup := client.Transport
	t.Transport = backup
	client.Transport = t
	return client
}

// Client Creates a new HTTP client
func (t *BasicAuthTransport) Client() *http.Client {
	return &http.Client{
		Transport: t,
		Timeout:   30 * time.Second,
	}
}

func (t *BasicAuthTransport) transport() http.RoundTripper {
	if t.Transport != nil {
		return t.Transport
	}
	return http.DefaultTransport
}
