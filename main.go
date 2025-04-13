package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	// "time"
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

// validateChirpHandler validates that a chirp's body length is at most 140 characters.
func (cfg *apiConfig) validateChirpHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests.
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Define a structure for the request body.
	type chirpRequest struct {
		Body string `json:"body"`
	}

	// Decode the JSON request body.
	decoder := json.NewDecoder(r.Body)
	var req chirpRequest
	err := decoder.Decode(&req)
	if err != nil {
		log.Printf("Error decoding chirp: %s", err)
		http.Error(w, `{"error":"Something went wrong"}`, http.StatusInternalServerError)
		return
	}

	// Validate chirp length.
	if len(req.Body) > 140 {
		w.Header().Set("Content-Type", "application/json")
		// Respond with 400 status code since the chirp is too long.
		w.WriteHeader(http.StatusBadRequest)
		resp := map[string]string{"error": "Chirp is too long"}
		if respData, err := json.Marshal(resp); err == nil {
			w.Write(respData)
		} else {
			http.Error(w, `{"error":"Something went wrong"}`, http.StatusInternalServerError)
		}
		return
	}

	// If valid, return a valid response.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	resp := map[string]bool{"valid": true}
	if respData, err := json.Marshal(resp); err == nil {
		w.Write(respData)
	} else {
		http.Error(w, `{"error":"Something went wrong"}`, http.StatusInternalServerError)
	}
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

	// Chirp validation endpoint.
	mux.HandleFunc("/api/validate_chirp", apiCfg.validateChirpHandler)

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
