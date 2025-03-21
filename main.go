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

	"github.com/P-H-Pancholi/Chirpy/internal/auth"
	"github.com/P-H-Pancholi/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type JsonChirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             database.Queries
	Platform       string
	JwtToken       string
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
		JwtToken:       os.Getenv("JWT_TOKEN"),
	}

	cfg.fileserverHits.Store(0)

	mux.Handle("/app/", cfg.middlewareMetricInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	// mux.Handle("/app/", http.FileServer(http.Dir(".")))
	mux.HandleFunc("GET /api/healthz", HealthHandler)
	mux.HandleFunc("GET /admin/metrics", cfg.NumRequestHandler)
	mux.HandleFunc("POST /admin/reset", cfg.ResetHandler)
	mux.HandleFunc("POST /api/validate_chirp", ValidateChirp)
	mux.HandleFunc("POST  /api/users", cfg.CreateUserHandler)
	mux.HandleFunc("POST /api/chirps", cfg.ChirpHandler)
	mux.HandleFunc("GET /api/chirps", cfg.GetAllChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirp_id}", cfg.GetChirpHandler)
	mux.HandleFunc("POST  /api/login", cfg.LoginHandler)
	mux.HandleFunc("POST /api/refresh", cfg.RefreshHandler)
	mux.HandleFunc("POST /api/revoke", cfg.RevokeHandler)

	if err := server.ListenAndServe(); err != nil {
		fmt.Println(err)
	}
}

func (cfg *apiConfig) RevokeHandler(res http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(res, 401, err.Error())
		return
	}
	err = cfg.DB.RevokeToken(req.Context(), database.RevokeTokenParams{
		RevokedAt: sql.NullTime{
			Time:  time.Now().UTC(),
			Valid: true,
		},
		Token: token,
	})

	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(res, 401, "invalid token")
			return
		}
		respondWithError(res, 401, err.Error())
		return
	}
	res.WriteHeader(204)
}

func (cfg *apiConfig) RefreshHandler(res http.ResponseWriter, req *http.Request) {
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(res, 401, err.Error())
		return
	}
	RefreshTokenResponse, err := cfg.DB.GetUserFromRefreshToken(req.Context(), token)
	if err != nil {
		respondWithError(res, 401, fmt.Sprintf("error in fetching user from DB : %v", err))
		return
	}
	if RefreshTokenResponse.ExpiresAt.Compare(time.Now().UTC()) <= 0 {
		respondWithError(res, 401, fmt.Sprintf("refresh token expired"))
		return
	}
	if RefreshTokenResponse.RevokedAt.Valid {
		respondWithError(res, 401, fmt.Sprintf("refresh token revoked"))
		return
	}
	accessToken, err := auth.MakeJWT(RefreshTokenResponse.UserID, cfg.JwtToken)

	ResBody := struct {
		Token string `json:"token"`
	}{
		Token: accessToken,
	}

	dat, err := json.Marshal(ResBody)
	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	res.WriteHeader(200)
	res.Write(dat)
}

func (cfg *apiConfig) LoginHandler(res http.ResponseWriter, req *http.Request) {
	ReqBody := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&ReqBody); err != nil {
		respondWithError(res, 500, fmt.Sprintf("error in decoding json body : %v", err))
		return
	}
	currUser, err := cfg.DB.GetUserByEmail(req.Context(), ReqBody.Email)
	if err != nil {
		respondWithError(res, 500, fmt.Sprintf("error in fetching user from DB : %v", err))
		return
	}
	if err := auth.CheckPasswordHash(ReqBody.Password, currUser.HashedPassword); err != nil {
		respondWithError(res, 401, "Incorrect email or password")
		return
	}

	refresh_token, _ := auth.MakeRefreshToken()

	Rt, err := cfg.DB.CreateRefereshToken(req.Context(), database.CreateRefereshTokenParams{
		Token:     refresh_token,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().AddDate(0, 0, 60),
		UserID:    currUser.ID,
	})
	if err != nil {
		respondWithError(res, 500, "unable to generate token")
		return
	}

	token, err := auth.MakeJWT(currUser.ID, cfg.JwtToken)
	if err != nil {
		respondWithError(res, 500, "unable to generate token")
		return
	}
	JsonUser := struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}{
		ID:           currUser.ID,
		CreatedAt:    currUser.CreatedAt,
		UpdatedAt:    currUser.UpdatedAt,
		Email:        currUser.Email,
		Token:        token,
		RefreshToken: Rt.Token,
	}
	dat, err := json.Marshal(JsonUser)
	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	res.WriteHeader(200)
	res.Write(dat)

}

func (cfg *apiConfig) GetChirpHandler(res http.ResponseWriter, req *http.Request) {
	chirp, err := cfg.DB.GetChirpById(req.Context(), uuid.MustParse(req.PathValue("chirp_id")))
	if err == sql.ErrNoRows {
		res.WriteHeader(404)
		return
	}
	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	chirpResBody := JsonChirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	dat, err := json.Marshal(chirpResBody)
	if err != nil {
		fmt.Println(err)
		respondWithError(res, 500, err.Error())
		return
	}
	res.WriteHeader(200)
	res.Write(dat)

}

func (cfg *apiConfig) GetAllChirpsHandler(res http.ResponseWriter, req *http.Request) {
	chirps, err := cfg.DB.GetAllChirps(req.Context())
	if err != nil {
		fmt.Println(err)
		respondWithError(res, 500, err.Error())
		return
	}
	ChirpsResBody := []JsonChirp{}
	for _, chirp := range chirps {
		ChirpsResBody = append(ChirpsResBody, JsonChirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		})
	}
	dat, err := json.Marshal(ChirpsResBody)
	if err != nil {
		fmt.Println(err)
		respondWithError(res, 500, err.Error())
		return
	}
	res.WriteHeader(200)
	res.Write(dat)
}

func (cfg *apiConfig) ChirpHandler(res http.ResponseWriter, req *http.Request) {
	ChirpReqBody := struct {
		Body string `json:"body"`
	}{}
	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(res, 401, err.Error())
		return
	}

	decoder := json.NewDecoder(req.Body)
	defer req.Body.Close()
	if len(ChirpReqBody.Body) > 140 {
		respondWithError(res, 400, "Chirp is too long")
		return
	}
	if err := decoder.Decode(&ChirpReqBody); err != nil {
		respondWithError(res, 500, err.Error())
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.JwtToken)
	if err != nil {
		respondWithError(res, 401, err.Error())
		return
	}
	Chirp, err := cfg.DB.CreateChirp(req.Context(), database.CreateChirpParams{
		Body:   ChirpReqBody.Body,
		UserID: userId,
	})
	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	ChirpResBody := JsonChirp{
		ID:        Chirp.ID,
		CreatedAt: Chirp.CreatedAt,
		UpdatedAt: Chirp.UpdatedAt,
		Body:      Chirp.Body,
		UserID:    Chirp.UserID,
	}

	dat, err := json.Marshal(ChirpResBody)
	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	res.WriteHeader(201)
	res.Write(dat)
}

func (cfg *apiConfig) CreateUserHandler(res http.ResponseWriter, req *http.Request) {
	UserEmail := struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}{}
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&UserEmail); err != nil {
		respondWithError(res, 500, err.Error())
		return
	}
	hashed_password, err := auth.HashPassword(UserEmail.Password)
	if err != nil {
		respondWithError(res, 500, fmt.Sprintf("error in Hashing password: %v", err))
		return
	}

	user, err := cfg.DB.CreateUser(req.Context(), database.CreateUserParams{
		Email:          UserEmail.Email,
		HashedPassword: hashed_password,
	})

	if err != nil {
		respondWithError(res, 500, err.Error())
		return
	}

	JsonUser := struct {
		ID        uuid.UUID `json:"id"`
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
		return
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
		return
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
