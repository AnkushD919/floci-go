package router

import (
	"log"
	"net/http"

	"github.com/floci-io/floci-go/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router manages all AWS services and routes incoming port 4566 traffic.
type Router struct {
	mux     *chi.Mux
	plugins []service.ServicePlugin
}

// New creates and initializes a new Router.
func New() *Router {
	r := &Router{
		mux:     chi.NewRouter(),
		plugins: make([]service.ServicePlugin, 0),
	}

	// Mount default middlewares
	r.mux.Use(middleware.RequestID)
	r.mux.Use(middleware.RealIP)
	r.mux.Use(middleware.Logger)
	r.mux.Use(middleware.Recoverer)

	// Wildcard route to handle custom protocol dispatching
	r.mux.HandleFunc("/*", r.dispatch)

	return r
}

// RegisterPlugin adds a ServicePlugin to the router.
func (r *Router) RegisterPlugin(p service.ServicePlugin) {
	r.plugins = append(r.plugins, p)
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func (r *Router) dispatch(w http.ResponseWriter, req *http.Request) {
	for _, p := range r.plugins {
		if p.Matches(req) {
			p.ServeHTTP(w, req)
			return
		}
	}

	log.Printf("No handler found for Request: Path: %s, Method: %s, Host: %s", req.URL.Path, req.Method, req.Host)
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("Floci-Go: service not found or not implemented yet"))
}

