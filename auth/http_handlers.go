package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gorilla/mux"
)

func (am *AuthModule) SetupRoutes(router *mux.Router) {
	router.HandleFunc("/auth/login", am.handleLogin).Methods("GET")
	router.HandleFunc("/auth/callback", am.HandleCallback).Methods("GET")
}

func (am *AuthModule) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateRandomState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, am.GetAuthorizationURL(state), http.StatusFound)
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func generateRandomNonce() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
