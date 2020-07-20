package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	setupLogger()
	code := m.Run()
	os.Exit(code)
}
