package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		fmt.Println(req)

		switch req.Method {
		case http.MethodGet:
			http.Error(rw, `{"error": "not found"}`, http.StatusNotFound)
			return
		case http.MethodPost, http.MethodPut:
			_, _ = rw.Write([]byte(`{}`))
			return
		}
	})

	err := http.ListenAndServe(":8666", mux)
	if err != nil {
		log.Fatal(err)
	}
}
