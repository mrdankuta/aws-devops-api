package auth

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	devconfig "github.com/mrdankuta/aws-devops-api/config"
	"golang.org/x/oauth2"
)

type AuthModule struct {
	oidcConfig *oauth2.Config
	tokenCache map[string]*oauth2.Token
}

func NewAuthModule(cfg *devconfig.OIDCConfig) *AuthModule {
	return &AuthModule{
		oidcConfig: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  cfg.ProviderURL + "/authorize",
				TokenURL: cfg.ProviderURL + "/token",
			},
		},
		tokenCache: make(map[string]*oauth2.Token),
	}
}

func (am *AuthModule) GetAWSConfig(ctx context.Context, accountID, roleARN string) (aws.Config, error) {
	token, err := am.getOIDCToken(ctx, accountID)
	if err != nil {
		return aws.Config{}, err
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, err
	}

	stsSvc := sts.NewFromConfig(cfg)
	creds := stscreds.NewAssumeRoleProvider(stsSvc, roleARN, func(o *stscreds.AssumeRoleOptions) {
		o.TokenProvider = func() (string, error) {
			return token.AccessToken, nil
		}
	})

	cfg.Credentials = aws.NewCredentialsCache(creds)

	return cfg, nil
}

func (am *AuthModule) getOIDCToken(ctx context.Context, accountID string) (*oauth2.Token, error) {
	if token, ok := am.tokenCache[accountID]; ok && token.Valid() {
		return token, nil
	}

	// In a real-world scenario, you'd implement the OIDC flow here
	// For this example, we'll just create a dummy token
	token := &oauth2.Token{
		AccessToken: "dummy_access_token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(1 * time.Hour),
	}

	am.tokenCache[accountID] = token
	return token, nil
}
