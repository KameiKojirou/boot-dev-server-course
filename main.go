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

// adminMetricsHandler returns an HTML page with admin metrics.
func (cfg *apiConfig) adminMetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hits := cfg.fileserverHits.Load()
	page := fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
`, hits)
	_, _ = w.Write([]byte(page))
}

// adminResetHandler resets the fileserverHits counter to 0.
func (cfg *apiConfig) adminResetHandler(w http.ResponseWriter, r *http.Request) {
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

	// Readiness endpoint; only allow GET requests.
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Admin metrics endpoint to report the number of hits in HTML; only allow GET.
	mux.HandleFunc("GET /admin/metrics", apiCfg.adminMetricsHandler)

	// Admin reset endpoint to reset the counter; only allow POST.
	mux.HandleFunc("POST /admin/reset", apiCfg.adminResetHandler)

	// Create a new server using the ServeMux as its handler.
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Server is running on http://localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
