package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

// wrapper function should return another function with logic intended included
func (cfg *apiConfig) middlewareMetricInc(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}
func main() {
	mux := http.NewServeMux()
	server := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	cfg := apiConfig{
		fileserverHits: atomic.Int32{},
	}

	cfg.fileserverHits.Store(0)

	mux.Handle("/app/", cfg.middlewareMetricInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	// mux.Handle("/app/", http.FileServer(http.Dir(".")))
	mux.HandleFunc("GET /api/healthz", HealthHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.NumRequestHandler)
	mux.HandleFunc("POST /admin/reset", cfg.ResetHandler)
	mux.HandleFunc("POST /api/validate_chirp", ValidateChirp)

	if err := server.ListenAndServe(); err != nil {
		fmt.Println(err)
	}
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
