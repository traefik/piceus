package plugin

import (
	"context"
	"fmt"
	"net/http"
	"os"
)

type Config struct{}

// New writes a fake result on stdout and stderr, and returns a valid handler.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	fmt.Println(`{"error":"fake error from stdout"}`)
	fmt.Fprintln(os.Stderr, `{"error":"fake error from stderr"}`)

	return next, nil
}

func CreateConfig() *Config {
	return &Config{}
}
