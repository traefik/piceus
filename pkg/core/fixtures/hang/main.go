package plugin

import (
	"context"
	"net/http"
	"time"
)

type Config struct{}

// New blocks for a long time.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	time.Sleep(time.Hour)

	return next, nil
}

func CreateConfig() *Config {
	return &Config{}
}
