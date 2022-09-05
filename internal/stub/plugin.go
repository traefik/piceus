package main

import (
	"net/http"

	"github.com/rs/zerolog/log"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			http.Error(rw, `{"error": "plugin not found"}`, http.StatusNotFound)
			return
		case http.MethodPost, http.MethodPut:
			_, _ = rw.Write([]byte(`{}`))
			return
		}
	})

	//nolint:gosec // only for testing purpose.
	err := http.ListenAndServe(":8666", mux)
	if err != nil {
		log.Fatal().Err(err).Msg("error")
	}
}
