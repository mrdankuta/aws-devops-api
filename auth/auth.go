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

func (am *AuthModule) HandleCallback(ctx context.Context, code, state string) (*oauth2.Token, error) {
	// Verify state
	if _, ok := am.stateStore.Load(state); !ok {
		return nil, errors.New("invalid state parameter")
	}
	am.stateStore.Delete(state)

	// Exchange code for token
	token, err := am.oidcConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	// Verify ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token in token response")
	}
	idToken, err := am.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID token: %w", err)
	}

	// Extract claims
	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	// Store token in cache
	encryptedToken, err := am.encryptToken(token)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt token: %w", err)
	}
	am.tokenCache.Store(claims.Email, tokenCacheEntry{Token: encryptedToken, AccountID: claims.Email})

	return token, nil
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
