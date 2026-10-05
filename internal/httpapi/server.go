// Package httpapi serves the myrest HTTP API: it maps a request to a resource
// of the schema cache and answers with JSON rows or with the error envelope.
package httpapi

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/jonbaldie/myrest/internal/config"
	"github.com/jonbaldie/myrest/internal/jwt"
	"github.com/jonbaldie/myrest/internal/prefer"
	"github.com/jonbaldie/myrest/internal/rpcexec"
	"github.com/jonbaldie/myrest/internal/schemacache"
)

// Options holds what a myrest listener needs: where to bind, the resolved
// settings, the schema cache, the reader that runs the read as the database
// role of the request, the writer that runs table writes, the routine executor
// that runs POST /rpc and GET /rpc (or the caller adapter), and the logger.
// Log takes what the operator must see and the client must not; it defaults
// to the logger of the log package.
type Options struct {
	Addr     string
	Settings config.Settings
	Cache    *schemacache.Cache
	Reader   Reader
	Writer   Writer
	Executor rpcexec.Executor
	Caller   Caller
	Log      *log.Logger
}

// Service is a running myrest HTTP listener.
type Service struct {
	server   *http.Server
	listener net.Listener
	settings config.Settings
	cache    *schemacache.Cache
	reader   Reader
	writer   Writer
	executor rpcexec.Executor
	verifier *jwt.Verifier
	log      *log.Logger
}

// Listen binds the address of the options and returns a Service ready to Serve.
func Listen(options Options) (*Service, error) {
	listener, err := net.Listen("tcp", options.Addr)
	if err != nil {
		return nil, err
	}

	logger := options.Log
	if logger == nil {
		logger = log.Default()
	}

	var verifier *jwt.Verifier
	if options.Settings.JWT.Secret != "" {
		built, err := jwt.New(jwt.Options{
			Secret:          options.Settings.JWT.Secret,
			SecretIsBase64:  options.Settings.JWT.SecretIsBase64,
			Aud:             options.Settings.JWT.Aud,
			RoleClaimKey:    options.Settings.JWT.RoleClaimKey,
			CacheMaxEntries: options.Settings.JWT.CacheMaxEntries,
		})
		if err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("jwt settings: %w", err)
		}
		verifier = built
	}

	executor := options.Executor
	if executor == nil && options.Caller != nil {
		executor = rpcexec.New(options.Caller, options.Settings.DB.TxEnd)
	}

	service := &Service{
		listener: listener,
		settings: options.Settings,
		cache:    options.Cache,
		reader:   options.Reader,
		writer:   options.Writer,
		executor: executor,
		verifier: verifier,
		log:      logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", withPrefer(service.writeRoot))
	mux.HandleFunc("GET /{table}", withPrefer(service.readTable))
	mux.HandleFunc("HEAD /{table}", withPrefer(service.readTable))
	mux.HandleFunc("POST /{table}", withPrefer(service.insertTable))
	mux.HandleFunc("PATCH /{table}", withPrefer(service.patchTable))
	mux.HandleFunc("PUT /{table}", withPrefer(service.putTable))
	mux.HandleFunc("DELETE /{table}", withPrefer(service.deleteTable))
	mux.HandleFunc("OPTIONS /{table}", withPrefer(service.optionsTable))
	mux.HandleFunc("POST /rpc/{name}", withPrefer(service.callRoutine))
	mux.HandleFunc("GET /rpc/{name}", withPrefer(service.getRoutine))
	mux.HandleFunc("OPTIONS /rpc/{name}", withPrefer(service.optionsRoutine))
	mux.HandleFunc("/", writeNoHandler)
	service.server = &http.Server{
		Handler: withCORS(options.Settings.Server.CORSAllowedOrigins, mux),
	}
	return service, nil
}

// preferHandler answers one route with the parsed Prefer header.
type preferHandler func(http.ResponseWriter, *http.Request, prefer.Preferences)

// withPrefer parses the Prefer header once per request, so every check of the
// route reads the same preferences.
func withPrefer(handler preferHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		handler(writer, request, prefer.Parse(request.Header.Values("Prefer")))
	}
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
