package plugin

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type Config struct{}

// New returns an error of 200 KiB: it is larger than the result limit and than the pipe buffer.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return nil, errors.New(strings.Repeat("x", 200<<10))
}

func CreateConfig() *Config {
	return &Config{}
}
