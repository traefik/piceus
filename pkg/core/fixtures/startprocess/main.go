package plugin

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

type Config struct{}

// New starts a shell with os.StartProcess.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	p, err := os.StartProcess("/bin/sh", []string{"sh", "-c", "true"}, &os.ProcAttr{})
	if err != nil {
		return nil, err
	}

	_, _ = p.Wait()

	return nil, fmt.Errorf("pwned")
}

func CreateConfig() *Config {
	return &Config{}
}
