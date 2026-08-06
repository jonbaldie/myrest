package config

import (
	"errors"
	"fmt"
	"os"
)

// ForProcess resolves the settings of a myrest process from its arguments and
// its environment, and passes the serve gate. The one optional argument is the
// path of the config file, as the parity target does it. A process that gets an
// error from ForProcess must not serve the API.
func ForProcess(args []string, env Environment) (Settings, error) {
	text, err := readConfigFile(args)
	if err != nil {
		return Settings{}, err
	}
	settings, err := Load(text, env)
	if err != nil {
		return Settings{}, err
	}
	if err := settings.ServeGate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
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
