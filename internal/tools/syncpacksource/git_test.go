package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("GIT_CONFIG_COUNT", "2")
	os.Setenv("GIT_CONFIG_KEY_0", "gc.auto")
	os.Setenv("GIT_CONFIG_VALUE_0", "0")
	os.Setenv("GIT_CONFIG_KEY_1", "maintenance.auto")
	os.Setenv("GIT_CONFIG_VALUE_1", "false")
	os.Exit(m.Run())
}
