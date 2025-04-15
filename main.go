package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/kameikojirou/boot-dev-server-course/internal/database"
)

// apiConfig holds our stateful in-memory data and our SQLC queries instance.
type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

// User represents the JSON payload we return for a user.
type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

// Chirp represents the JSON payload we return for a chirp.
type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

// respondWithError responds with a JSON error message.
func respondWithError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := map[string]string{"error": msg}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Failed to send error response: %v", err)
	}
}

// respondWithJSON responds with the provided payload as JSON.
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Failed to send JSON response: %v", err)
	}
}

// badWords is the list of forbidden words.
var badWords = []string{"kerfuffle", "sharbert", "fornax"}

// stripPunctuation splits a token into leading punctuation, core word, and trailing punctuation.
func stripPunctuation(word string) (leading, core, trailing string) {
	start := 0
	for i, r := range word {
		if !unicode.IsPunct(r) {
			start = i
			break
		}
	}
	end := len(word)
	for i := len(word) - 1; i >= 0; i-- {
		if !unicode.IsPunct(rune(word[i])) {
			end = i + 1
			break
		}
	}
	leading = word[:start]
	core = word[start:end]
	trailing = word[end:]
	return
}

// replaceBadWords returns a new string with any bad words replaced by "****".
func replaceBadWords(text string) string {
	words := strings.Fields(text)
	for i, word := range words {
		leading, core, trailing := stripPunctuation(word)
		if core == "" {
			continue
		}
		lower := strings.ToLower(core)
		for _, bad := range badWords {
			if lower == bad {
				words[i] = leading + "****" + trailing
				break
			}
		}
	}
	return strings.Join(words, " ")
}

// middlewareMetricsInc increments the fileserverHits counter on each request.
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
</html>`, hits)
	_, _ = w.Write([]byte(page))
}

// adminResetHandler resets the user database and fileserverHits counter.
// It only works if PLATFORM is set to "dev".
func (cfg *apiConfig) adminResetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, http.StatusForbidden,
			"Access forbidden: reset endpoint allowed only in dev mode")
		return
	}

	// Delete all users in the database.
	if err := cfg.db.DeleteAllUsers(r.Context()); err != nil {
		respondWithError(w, http.StatusInternalServerError,
			"Could not reset database")
		return
	}

	// Reset the fileserverHits counter.
	cfg.fileserverHits.Store(0)
	respondWithJSON(w, http.StatusOK, struct{}{})
}

// createUserHandler handles POST /api/users requests to create a new user.
func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type createUserRequest struct {
		Email string `json:"email"`
	}

	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding create user request: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Create user in the database.
	userDB, err := cfg.db.CreateUser(r.Context(), req.Email)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Could not create user")
		return
	}

	userResp := User{
		ID:        userDB.ID,
		CreatedAt: userDB.CreatedAt,
		UpdatedAt: userDB.UpdatedAt,
		Email:     userDB.Email,
	}

	respondWithJSON(w, http.StatusCreated, userResp)
}

// createChirpHandler handles POST /api/chirps requests to create a new chirp.
// It validates the chirp's length, replaces any profane words,
// and creates the chirp in the database.
func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	type createChirpRequest struct {
		Body   string `json:"body"`
		UserID string `json:"user_id"`
	}

	var req createChirpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding chirp request: %v", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// Validate chirp length.
	if len(req.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	// Replace any bad words in the chirp body.
	cleanedBody := replaceBadWords(req.Body)

	// Convert the userID to a UUID.
	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user_id provided")
		return
	}

	// Create chirp in the database. We assume the SQLC query CreateChirp accepts
	// a parameters struct similar to below.
	params := database.CreateChirpParams{
		ID:     uuid.New(), // new random UUID
		Body:   cleanedBody,
		UserID: userUUID,
		// SQLC might set CreatedAt and UpdatedAt automatically if defined in the query,
		// Otherwise, you may explicitly pass time.Now()
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	chirpDB, err := cfg.db.CreateChirp(r.Context(), params)
	if err != nil {
		log.Printf("Error creating chirp: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Could not create chirp")
		return
	}

	chirpResp := Chirp{
		ID:        chirpDB.ID,
		CreatedAt: chirpDB.CreatedAt,
		UpdatedAt: chirpDB.UpdatedAt,
		Body:      chirpDB.Body,
		UserID:    chirpDB.UserID,
	}

	respondWithJSON(w, http.StatusCreated, chirpResp)
}

func main() {
	// Load environment variables.
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM") // read the PLATFORM env variable

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize SQLC queries.
	dbQueries := database.New(db)
	fmt.Println("Database queries initialized:", dbQueries)

	mux := http.NewServeMux()

	// Initialize our apiConfig.
	apiCfg := &apiConfig{
		db:       dbQueries,
		platform: platform,
	}

	// FileServer handler for serving static files.
	fs := http.FileServer(http.Dir("."))
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", fs)))

	// Healthz endpoint.
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Admin metrics endpoint.
	mux.HandleFunc("/admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		apiCfg.adminMetricsHandler(w, r)
	})

	// Admin reset endpoint.
	mux.HandleFunc("/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		apiCfg.adminResetHandler(w, r)
	})

	// Create user endpoint.
	mux.HandleFunc("/api/users", apiCfg.createUserHandler)

	// Create chirp endpoint.
	mux.HandleFunc("/api/chirps", apiCfg.createChirpHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("Server is running on http://localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
