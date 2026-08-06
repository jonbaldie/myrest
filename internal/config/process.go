package config

import (
	"errors"
	"fmt"
	"os"
)

// defaultListen is the address a myrest process binds when nobody sets one.
const defaultListen = "127.0.0.1:3000"

// ForProcess resolves the settings of a myrest process from its arguments and
// its environment, and passes the serve gate. The one optional argument is the
// path of the config file, as the parity target does it. A process that gets an
// error from ForProcess must not serve the API.
func ForProcess(args []string, env Environment) (Settings, error) {
	file, err := readConfigFile(args)
	if err != nil {
		return Settings{}, err
	}
	settings, err := Load(file, env)
	if err != nil {
		return Settings{}, err
	}
	if err := Gate(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// ListenAddress is the address the process binds. A bind address is process
// tuning, so it stays off the normative surface and out of the config file.
func ListenAddress(env Environment) string {
	if address := env["MYREST_LISTEN"]; address != "" {
		return address
	}
	return defaultListen
}

// readConfigFile reads the text of the one optional config file argument.
func readConfigFile(args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("usage: myrest [config-file]")
	}
	if len(args) == 0 {
		return "", nil
	}
	text, err := os.ReadFile(args[0])
	if err != nil {
		return "", fmt.Errorf("read config file: %w", err)
	}
	return string(text), nil
}
