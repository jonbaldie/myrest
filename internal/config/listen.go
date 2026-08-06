package config

// defaultListen is the address a myrest process binds when nobody sets one.
const defaultListen = "127.0.0.1:3000"

// ListenAddress is the address the process binds. A bind address is process
// tuning, so it stays off the normative surface: it has no knob, and a config
// file cannot hold it.
func ListenAddress(env Environment) string {
	if address := env["MYREST_LISTEN"]; address != "" {
		return address
	}
	return defaultListen
}
