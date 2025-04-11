package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

// apiConfig holds our stateful in-memory data.
type apiConfig struct {
	fileserverHits atomic.Int32
}

// middlewareMetricsInc is a middleware that increments the fileserverHits
// counter on each request before calling the next handler.
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

// metricsHandler writes the number of requests processed as plain text.
func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	hits := cfg.fileserverHits.Load()
	_, _ = fmt.Fprintf(w, "Hits: %d", hits)
}

// resetHandler resets the fileserverHits counter to 0.
func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	cfg.fileserverHits.Store(0)
	_, _ = w.Write([]byte("fileserverHits reset to 0"))
}

func main() {
	// Create a new ServeMux instance.
	mux := http.NewServeMux()

	// Create and configure our apiConfig.
	apiCfg := &apiConfig{}

	// Create a FileServer handler to serve files from the current directory.
	fs := http.FileServer(http.Dir("."))

	// Wrap the file server with our metrics middleware and register it for the "/app/" path.
	// Use http.StripPrefix to remove the "/app" prefix before handing off to the FileServer.
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fs)))

	// Readiness endpoint.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// Set the expected Content-Type header.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// Write the status code.
		w.WriteHeader(http.StatusOK)
		// Write the response body.
		_, _ = w.Write([]byte("OK"))
	})

	// Metrics endpoint to report the number of hits.
	mux.HandleFunc("/metrics", apiCfg.metricsHandler)
	// Reset endpoint to reset the counter.
	mux.HandleFunc("/reset", apiCfg.resetHandler)

	// Create a new server using the ServeMux as its handler.
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Server is running on http://localhost:8080")
	// Start the server. This call blocks until the server exits.
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
