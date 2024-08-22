package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/mrdankuta/aws-devops-api/auth"
)

func NewCommand(command string, accounts []string, authModule *auth.AuthModule) func() (string, error) {
	return func() (string, error) {
		switch command {
		case "check_unused_buckets":
			ctx := context.Background()
			unusedBuckets, err := checkUnusedBuckets(ctx, authModule, accounts)
			if err != nil {
				return "", err
			}
			message := formatUnusedBucketsMessage(unusedBuckets)
			return message, nil
		default:
			return "", fmt.Errorf("unknown S3 command: %s", command)
		}
	}
}

func checkUnusedBuckets(ctx context.Context, authModule *auth.AuthModule, accounts []string) (map[string][]string, error) {
	unusedBuckets := make(map[string][]string)

	for _, account := range accounts {
		lastAccessTimes, err := getLastAccessTimes(ctx, authModule, account)
		if err != nil {
			return nil, fmt.Errorf("error getting last access times for account %s: %w", account, err)
		}

		for bucket, lastAccessTime := range lastAccessTimes {
			if time.Since(lastAccessTime) > 30*24*time.Hour {
				unusedBuckets[account] = append(unusedBuckets[account], bucket)
			}
		}
	}

	return unusedBuckets, nil
}

func getLastAccessTimes(ctx context.Context, authModule *auth.AuthModule, accountID string) (map[string]time.Time, error) {
	cfg, err := authModule.GetAWSConfig(ctx, accountID, fmt.Sprintf("arn:aws:iam::%s:role/ReadOnlyRole", accountID))
	if err != nil {
		return nil, fmt.Errorf("error getting AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)

	lastAccessTimes := make(map[string]time.Time)

	listBucketsOutput, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("error listing buckets for account %s: %w", accountID, err)
	}

	for _, bucket := range listBucketsOutput.Buckets {
		lastAccessTime, err := getLastAccessTime(ctx, cwClient, *bucket.Name)
		if err != nil {
			fmt.Printf("Error getting last access time for bucket %s: %v\n", *bucket.Name, err)
			continue
		}
		lastAccessTimes[*bucket.Name] = lastAccessTime
	}

	return lastAccessTimes, nil
}

func getLastAccessTime(ctx context.Context, cwClient *cloudwatch.Client, bucketName string) (time.Time, error) {
	getMetricDataInput := &cloudwatch.GetMetricDataInput{
		MetricDataQueries: []types.MetricDataQuery{
			{
				Id: aws.String("lastAccessTime"),
				MetricStat: &types.MetricStat{
					Metric: &types.Metric{
						Namespace:  aws.String("AWS/S3"),
						MetricName: aws.String("LastObjectAccessedTimestamp"),
						Dimensions: []types.Dimension{
							{
								Name:  aws.String("BucketName"),
								Value: aws.String(bucketName),
							},
						},
					},
					Period: aws.Int32(86400), // Last 24 hours
					Stat:   aws.String("Maximum"),
				},
			},
		},
		StartTime: aws.Time(time.Now().Add(-24 * time.Hour)),
		EndTime:   aws.Time(time.Now()),
	}

	getMetricDataOutput, err := cwClient.GetMetricData(ctx, getMetricDataInput)
	if err != nil {
		return time.Time{}, fmt.Errorf("error getting CloudWatch metric data: %w", err)
	}

	if len(getMetricDataOutput.MetricDataResults) == 0 || len(getMetricDataOutput.MetricDataResults[0].Timestamps) == 0 {
		return time.Time{}, fmt.Errorf("no last access time data found for bucket %s", bucketName)
	}

	// Return the most recent last access time
	return getMetricDataOutput.MetricDataResults[0].Timestamps[0], nil
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
