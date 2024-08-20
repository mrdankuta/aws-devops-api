package iam

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/mrdankuta/aws-devops-api/auth"
)

func NewCommand(command string, accounts []string, authModule *auth.AuthModule) func() (string, error) {
	return func() (string, error) {
		switch command {
		case "list_iam_users":
			return listIAMUsers(accounts, authModule)
		default:
			return "", fmt.Errorf("unknown IAM command: %s", command)
		}
	}
}

func listIAMUsers(accounts []string, authModule *auth.AuthModule) (string, error) {
	ctx := context.Background()
	iamUsers := make(map[string][]string)

	for _, account := range accounts {
		cfg, err := authModule.GetAWSConfig(ctx, account, fmt.Sprintf("arn:aws:iam::%s:role/ReadOnlyRole", account))
		if err != nil {
			return "", fmt.Errorf("error getting AWS config for account %s: %v", account, err)
		}

		iamClient := iam.NewFromConfig(cfg)

		listUsersOutput, err := iamClient.ListUsers(ctx, &iam.ListUsersInput{})
		if err != nil {
			return "", fmt.Errorf("error listing IAM users for account %s: %v", account, err)
		}

		for _, user := range listUsersOutput.Users {
			iamUsers[account] = append(iamUsers[account], *user.UserName)
		}
	}

	return formatIAMUsersMessage(iamUsers), nil
}

func formatIAMUsersMessage(iamUsers map[string][]string) string {
	message := "IAM users that still exist in accounts:\n"
	for account, users := range iamUsers {
		message += fmt.Sprintf("Account %s:\n", account)
		for _, user := range users {
			message += fmt.Sprintf("- %s\n", user)
		}
	}
	message += "\nPlease delete these IAM users as the organization is moving to OIDC roles for AWS authentication."
	return message
}
