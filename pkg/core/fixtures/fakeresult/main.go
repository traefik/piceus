package plugin

import (
	"context"
	"net/http"
	"os"
	"time"
)

type Config struct{}

// New writes a valid result on the result file descriptor, then starts a goroutine that panics.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	f := os.NewFile(3, "")
	_, _ = f.Write([]byte("{}"))

	go func() {
		panic("crash")
	}()

	time.Sleep(5 * time.Second)

	return next, nil
}

func CreateConfig() *Config {
	return &Config{}
}
