package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
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
	mux.HandleFunc("GET /api/metrics", cfg.NumRequestHandler)
	mux.HandleFunc("POST /api/reset", cfg.ResetHandler)

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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	msg := []byte("Hits: " + strconv.Itoa(int(cfg.fileserverHits.Load())))
	if _, err := w.Write(msg); err != nil {
		log.Fatal(err)
	}
}

func (cfg *apiConfig) ResetHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	cfg.fileserverHits.Store(0)
}
