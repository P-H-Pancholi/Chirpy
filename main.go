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

	"github.com/P-H-Pancholi/Chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             database.Queries
	Platform       string
}

// wrapper function should return another function with logic intended included
func (cfg *apiConfig) middlewareMetricInc(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}
func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Println(err)
	}
	dbQueries := database.New(db)
	mux := http.NewServeMux()
	server := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	platform := os.Getenv("PLATFORM")
	cfg := apiConfig{
		fileserverHits: atomic.Int32{},
		DB:             *dbQueries,
		Platform:       platform,
	}

	cfg.fileserverHits.Store(0)

	mux.Handle("/app/", cfg.middlewareMetricInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	// mux.Handle("/app/", http.FileServer(http.Dir(".")))
	mux.HandleFunc("GET /api/healthz", HealthHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.NumRequestHandler)
	mux.HandleFunc("POST /admin/reset", cfg.ResetHandler)
	mux.HandleFunc("POST /api/validate_chirp", ValidateChirp)
	mux.HandleFunc("POST  /api/users", cfg.CreateUserHandler)

	if err := server.ListenAndServe(); err != nil {
		fmt.Println(err)
	}
}

func (cfg *apiConfig) CreateUserHandler(res http.ResponseWriter, req *http.Request) {
	UserEmail := struct {
		Email string `json:"email"`
	}{}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&UserEmail); err != nil {
		respondWithError(res, 500, err.Error())
	}
	user, err := cfg.DB.CreateUser(req.Context(), UserEmail.Email)
	if err != nil {
		respondWithError(res, 500, err.Error())
	}

	JsonUser := struct {
		ID        int32     `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}
	dat, err := json.Marshal(JsonUser)
	if err != nil {
		respondWithError(res, 500, err.Error())
	}
	res.WriteHeader(201)
	res.Write(dat)
}

func HealthHandler(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/plain; charset=utf-8")
	res.WriteHeader(200)
	if _, err := res.Write([]byte("OK")); err != nil {
		log.Fatal(err)
	}
}

func (cfg *apiConfig) NumRequestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	htmlcontent := fmt.Sprintf(`
	<html>
  		<body>
    		<h1>Welcome, Chirpy Admin</h1>
    		<p>Chirpy has been visited %d times!</p>
  		</body>
	</html>
	`, cfg.fileserverHits.Load())
	msg := []byte(htmlcontent)
	if _, err := w.Write(msg); err != nil {
		log.Fatal(err)
	}
}

func (cfg *apiConfig) ResetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.Platform != "dev" {
		w.WriteHeader(403)
		return
	}
	if err := cfg.DB.DeleteAllUsers(r.Context()); err != nil {
		respondWithError(w, 500, err.Error())
	}
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)

}

func ValidateChirp(w http.ResponseWriter, r *http.Request) {
	chirp := struct {
		ChirpBody string `json:"body"`
	}{}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&chirp); err != nil {
		w.WriteHeader(500)
		return
	}
	if len(chirp.ChirpBody) > 140 {
		respondWithError(w, 400, "Chirp is too long")
	} else {
		respondWithJson(w, 200, chirp.ChirpBody)
	}
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respBody := struct {
		ChirpError string `json:"error"`
	}{
		ChirpError: msg,
	}
	dat, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error while marshalling json: %s", err)
		w.WriteHeader(500)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJson(w http.ResponseWriter, code int, payload string) {
	respBody := struct {
		ChirpSuccess bool   `json:"valid"`
		CleanedBody  string `json:"cleaned_body"`
	}{
		ChirpSuccess: true,
		CleanedBody:  replaceProfane(payload),
	}
	dat, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error while marshalling json: %s", err)
		w.WriteHeader(500)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	w.Write(dat)

}

func replaceProfane(body string) string {
	words_array := strings.Split(body, " ")
	for idx, word := range words_array {
		if strings.ToLower(word) == "kerfuffle" ||
			strings.ToLower(word) == "sharbert" ||
			strings.ToLower(word) == "fornax" {
			words_array[idx] = "****"
		}
	}
	return strings.Join(words_array, " ")
}
