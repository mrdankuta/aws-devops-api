package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/coreos/go-oidc/v3/oidc"
	devconfig "github.com/mrdankuta/aws-devops-api/config"
	"golang.org/x/oauth2"
)

type AuthModule struct {
	oidcConfig    *oauth2.Config
	oidcProvider  *oidc.Provider
	tokenCache    sync.Map
	encryptionKey []byte
	stateStore    sync.Map
	verifier      *oidc.IDTokenVerifier
	httpClient    *http.Client
}

type tokenCacheEntry struct {
	Token     *oauth2.Token
	AccountID string
}

func NewAuthModule(cfg *devconfig.OIDCConfig) (*AuthModule, error) {
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, cfg.ProviderURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oidcConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	// Generate a random encryption key
	encryptionKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, encryptionKey); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	return &AuthModule{
		oidcConfig:    oidcConfig,
		oidcProvider:  provider,
		encryptionKey: encryptionKey,
		verifier:      provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (am *AuthModule) GetAuthorizationURL(state string) string {
	am.stateStore.Store(state, time.Now().Add(5*time.Minute))
	return am.oidcConfig.AuthCodeURL(state)
}

func (am *AuthModule) StartOIDCFlow(w http.ResponseWriter, r *http.Request) {
	state, _ := generateRandomState()
	nonce, _ := generateRandomNonce()

	// Store state and nonce (you might want to use a secure session store instead of the stateStore)
	am.stateStore.Store(state, nonce)

	// Redirect to Keycloak login
	authURL := am.oidcConfig.AuthCodeURL(state, oidc.Nonce(nonce))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (am *AuthModule) HandleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	storedNonce, ok := am.stateStore.Load(state)
	if !ok {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}
	am.stateStore.Delete(state)

	token, err := am.oidcConfig.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token in token response", http.StatusInternalServerError)
		return
	}

	idToken, err := am.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Failed to verify ID token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if idToken.Nonce != storedNonce.(string) {
		http.Error(w, "Invalid nonce", http.StatusBadRequest)
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Failed to extract claims: "+err.Error(), http.StatusInternalServerError)
		return
	}

	encryptedToken, err := am.encryptToken(token)
	if err != nil {
		http.Error(w, "Failed to encrypt token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	am.tokenCache.Store(claims.Email, tokenCacheEntry{Token: encryptedToken, AccountID: claims.Email})

	// Redirect to a success page or return a success message
	fmt.Fprintf(w, "Authentication successful for %s", claims.Email)
}

func (am *AuthModule) HasValidToken(accountID string) bool {
	// TODO: This is a simplified check. Implement proper token validation.
	_, err := am.getOIDCToken(context.Background(), accountID)
	return err == nil
}

// CustomTokenRetriever implements the stscreds.IdentityTokenRetriever interface
type CustomTokenRetriever struct {
	Token string
}

func (ctr CustomTokenRetriever) GetIdentityToken() ([]byte, error) {
	return []byte(ctr.Token), nil
}

func (am *AuthModule) GetAWSConfig(ctx context.Context, accountID, roleARN string) (aws.Config, error) {
	token, err := am.getOIDCToken(ctx, accountID)
	if err != nil {
		// TODO: Instead of returning an error, you might want to trigger the authentication flow here
		// For now, we'll just return the error
		return aws.Config{}, fmt.Errorf("failed to get OIDC token: %w", err)
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	stsSvc := sts.NewFromConfig(cfg)

	tokenRetriever := CustomTokenRetriever{Token: token.AccessToken}
	webIdentityProvider := stscreds.NewWebIdentityRoleProvider(stsSvc, roleARN, tokenRetriever)

	cfg.Credentials = aws.NewCredentialsCache(webIdentityProvider)

	return cfg, nil
}

func (am *AuthModule) getOIDCToken(ctx context.Context, accountID string) (*oauth2.Token, error) {
	entry, ok := am.tokenCache.Load(accountID)
	if !ok {
		return nil, fmt.Errorf("no token found for account %s", accountID)
	}

	tokenEntry := entry.(tokenCacheEntry)
	token, err := am.decryptToken(tokenEntry.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt token: %w", err)
	}

	if token.Valid() {
		return token, nil
	}

	// Token is expired, try to refresh
	newToken, err := am.refreshToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	encryptedToken, err := am.encryptToken(newToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt refreshed token: %w", err)
	}
	am.tokenCache.Store(accountID, tokenCacheEntry{Token: encryptedToken, AccountID: accountID})

	return newToken, nil
}

func (am *AuthModule) refreshToken(ctx context.Context, token *oauth2.Token) (*oauth2.Token, error) {
	src := am.oidcConfig.TokenSource(ctx, token)
	newToken, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	return newToken, nil
}

func (am *AuthModule) encryptToken(token *oauth2.Token) (*oauth2.Token, error) {
	plaintext, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal token: %w", err)
	}

	block, err := aes.NewCipher(am.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	encryptedToken := &oauth2.Token{
		AccessToken: base64.StdEncoding.EncodeToString(ciphertext),
		TokenType:   token.TokenType,
		Expiry:      token.Expiry,
	}

	return encryptedToken, nil
}

func (am *AuthModule) decryptToken(token *oauth2.Token) (*oauth2.Token, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(am.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}

	var decryptedToken oauth2.Token
	if err := json.Unmarshal(plaintext, &decryptedToken); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted token: %w", err)
	}

	return &decryptedToken, nil
}
