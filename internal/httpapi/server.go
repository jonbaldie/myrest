// Package httpapi serves the myrest HTTP API: it maps a request to a resource
// of the schema cache and answers with JSON rows or with the error envelope.
package httpapi

import (
	"net"
	"net/http"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Options holds what a myrest listener needs: where to bind, the resolved
// settings, the schema cache, and the reader that runs the read as the
// database role of the request.
type Options struct {
	Addr     string
	Settings config.Settings
	Cache    *schemacache.Cache
	Reader   Reader
}

// Service is a running myrest HTTP listener.
type Service struct {
	server   *http.Server
	listener net.Listener
	settings config.Settings
	cache    *schemacache.Cache
	reader   Reader
}

// Listen binds the address of the options and returns a Service ready to Serve.
func Listen(options Options) (*Service, error) {
	listener, err := net.Listen("tcp", options.Addr)
	if err != nil {
		return nil, err
	}

	service := &Service{
		listener: listener,
		settings: options.Settings,
		cache:    options.Cache,
		reader:   options.Reader,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", writeService)
	mux.HandleFunc("GET /{table}", service.readTable)
	service.server = &http.Server{Handler: mux}
	return service, nil
}

// Serve accepts connections until Close is called.
func (s *Service) Serve() error {
	return s.server.Serve(s.listener)
}

// URL returns the base URL of the running service.
func (s *Service) URL() string {
	return "http://" + s.listener.Addr().String()
}

// Close stops the service listener.
func (s *Service) Close() error {
	return s.server.Close()
}

// writeService answers the root path. The OpenAPI output of the parity target
// comes with the discovery ticket.
func writeService(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"service": "myrest"})
}
