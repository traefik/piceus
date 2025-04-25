package unsafe

import (
	"context"
	"net/http"
)

type Config struct{}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {

	}), nil
}

func CreateConfig() *Config {
	return &Config{}
}
