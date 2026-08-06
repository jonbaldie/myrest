package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
)

// Service is a running myrest HTTP listener.
type Service struct {
	server   *http.Server
	listener net.Listener
}

// Listen binds addr and returns a Service ready to Serve.
func Listen(addr string) (*Service, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)

	return &Service{
		server:   &http.Server{Handler: mux},
		listener: listener,
	}, nil
}

// Start listens on an ephemeral local port and serves in the background.
func Start() (*Service, error) {
	service, err := Listen("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		_ = service.Serve()
	}()
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

func handleRoot(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{
		"service": "myrest",
		"parity":  "none",
	})
}
