package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func (am *AuthModule) SetupRoutes(router *mux.Router) {
	router.HandleFunc("/auth/login", am.handleLogin).Methods("GET")
	router.HandleFunc("/auth/callback", am.handleCallback).Methods("GET")
}

func (am *AuthModule) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := generateRandomState()
	if err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, am.GetAuthorizationURL(state), http.StatusFound)
}

func (am *AuthModule) handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	state := r.URL.Query().Get("state")
	if state == "" {
		http.Error(w, "Missing state parameter", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	token, err := am.HandleCallback(ctx, code, state)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to handle callback: %v", err), http.StatusInternalServerError)
		return
	}

	// In a real application, you would set a session cookie or token here
	// For this example, we'll just return the access token
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Authentication successful. Access token: %s", token.AccessToken)
}

func generateRandomState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
