package server

import (
	"net/http"
	"time"
)

// New builds an *http.Server with sane defaults for a public-facing
// service. ReadTimeout/WriteTimeout/MaxHeaderBytes exist specifically so a
// slow or malicious client can't hold a connection open indefinitely - the
// zero-value http.Server has none of these guards.
func New(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
}

// Run starts the server and blocks until it stops, returning whatever error
// caused that. It doesn't call log.Fatal itself - only main() gets to
// decide the process should exit (see fatal() in main.go); a library
// function killing the process takes that decision away from its caller.
func Run(addr string, handler http.Handler) error {
	return New(addr, handler).ListenAndServe()
}