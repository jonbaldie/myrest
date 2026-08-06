package config_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/config"
)

func TestListenAddressComesFromTheEnvironmentOrTheDefaultBind(t *testing.T) {
	t.Parallel()

	if address := config.ListenAddress(config.Environment{}); address != "127.0.0.1:3000" {
		t.Errorf("ListenAddress = %q, want 127.0.0.1:3000", address)
	}
	chosen := config.Environment{"MYREST_LISTEN": "0.0.0.0:8080"}
	if address := config.ListenAddress(chosen); address != "0.0.0.0:8080" {
		t.Errorf("ListenAddress = %q, want 0.0.0.0:8080", address)
	}
}
