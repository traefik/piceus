package plugin

import (
	"context"
	"net/http"
	"unsafe"
)

type Config struct{}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	_ = unsafe.Pointer(config)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
	}), nil
}

func CreateConfig() *Config {
	return &Config{}
}
