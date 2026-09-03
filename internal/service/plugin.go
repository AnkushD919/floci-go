// Package service defines the core plugin contract every AWS service handler must satisfy.
// Adding a new service: implement ServicePlugin, register in main.go — router.go never changes.
package service

import "net/http"

// ServicePlugin is the contract every AWS service handler must satisfy.
// Each plugin self-declares its name and detects whether an incoming request
// belongs to it, decoupling service detection from the central router.
type ServicePlugin interface {
	// Name returns the canonical lowercase service identifier (e.g. "lambda", "s3").
	Name() string

	// Matches returns true if this plugin should handle the given request.
	// Plugins are tested in registration order; the first match wins.
	Matches(r *http.Request) bool

	// ServeHTTP handles the matched request.
	http.Handler
}
