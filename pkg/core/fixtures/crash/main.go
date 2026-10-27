package plugin

import (
	"context"
	"net/http"
	"time"
)

type Config struct{}

// New starts a goroutine that panics: the panic stops the process.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	go func() {
		panic("crash")
	}()

	time.Sleep(5 * time.Second)

	return next, nil
}

func CreateConfig() *Config {
	return &Config{}
}
