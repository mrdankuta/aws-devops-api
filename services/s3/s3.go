package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mrdankuta/aws-devops-api/auth"
)

func NewCommand(command string, accounts []string, authModule *auth.AuthModule) func() (string, error) {
	return func() (string, error) {
		switch command {
		case "check_unused_buckets":
			return checkUnusedBuckets(accounts, authModule)
		default:
			return "", fmt.Errorf("unknown S3 command: %s", command)
		}
	}
}

func checkUnusedBuckets(accounts []string, authModule *auth.AuthModule) (string, error) {
	ctx := context.Background()
	unusedBuckets := make(map[string][]string)

	for _, account := range accounts {
		cfg, err := authModule.GetAWSConfig(ctx, account, fmt.Sprintf("arn:aws:iam::%s:role/ReadOnlyRole", account))
		if err != nil {
			return "", fmt.Errorf("error getting AWS config for account %s: %v", account, err)
		}

		s3Client := s3.NewFromConfig(cfg)

		listBucketsOutput, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			return "", fmt.Errorf("error listing buckets for account %s: %v", account, err)
		}

		for _, bucket := range listBucketsOutput.Buckets {
			lastAccessTime, err := getLastAccessTime(ctx, s3Client, *bucket.Name)
			if err != nil {
				fmt.Printf("Error getting last access time for bucket %s: %v\n", *bucket.Name, err)
				continue
			}

			if time.Since(lastAccessTime) > 30*24*time.Hour {
				unusedBuckets[account] = append(unusedBuckets[account], *bucket.Name)
			}
		}
	}

	return formatUnusedBucketsMessage(unusedBuckets), nil
}

func getLastAccessTime(ctx context.Context, client *s3.Client, bucketName string) (time.Time, error) {
	// In a real implementation, you would check bucket metrics or logs to determine the last access time
	// For this example, we'll just return the current time minus a random duration
	return time.Now().Add(-time.Duration(30+time.Now().Unix()%60) * 24 * time.Hour), nil
}

func formatUnusedBucketsMessage(unusedBuckets map[string][]string) string {
	message := "Unused S3 buckets (not accessed in the last month):\n"
	for account, buckets := range unusedBuckets {
		message += fmt.Sprintf("Account %s:\n", account)
		for _, bucket := range buckets {
			message += fmt.Sprintf("- %s\n", bucket)
		}
	}
	return message
}
