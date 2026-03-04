package utils

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	TWILIO_PHONE_NUMBER = "TWILIO_PHONE_NUMBER"
	TWILIO_ACCOUNT_SID  = "TWILIO_ACCOUNT_SID"
	TWILIO_AUTH_TOKEN   = "TWILIO_AUTH_TOKEN"
	OPENAI_API_KEY      = "OPENAI_API_KEY"
)

var (
	secretCache   = make(map[string]string)
	secretCacheMu sync.Mutex
)

// GetSecret reads a secret value. In local mode it returns the env var directly.
// In Lambda it reads from SSM Parameter Store and caches the result.
func GetSecret(envKey string) string {
	if GetIsLocal() {
		return os.Getenv(envKey)
	}

	paramName := os.Getenv(envKey)
	if paramName == "" {
		return ""
	}

	secretCacheMu.Lock()
	defer secretCacheMu.Unlock()

	if val, ok := secretCache[paramName]; ok {
		return val
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		fmt.Printf("error loading AWS config for SSM: %v\n", err)
		return ""
	}

	client := ssm.NewFromConfig(cfg)
	result, err := client.GetParameter(context.TODO(), &ssm.GetParameterInput{
		Name:           aws.String(paramName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		fmt.Printf("error reading SSM parameter %s: %v\n", paramName, err)
		return ""
	}

	val := aws.ToString(result.Parameter.Value)
	secretCache[paramName] = val
	return val
}

// Friend represents a friend's information.
type Friend struct {
	Name        string
	Birthday    time.Time
	PhoneNumber string
	Interests   string
	Prompt      string
}

// GetIsLocal checks if the application is running in local mode.
func GetIsLocal() bool {
	return os.Getenv("IS_LOCAL") != ""
}

// ReadFriendsCSV reads friend data from either a local file or S3 based on the environment.
func ReadFriendsCSV(isLocal bool, bucketName, key string) ([]Friend, error) {
	data, err := ReadDataSource(isLocal, bucketName, key)
	if err != nil {
		return nil, fmt.Errorf("error reading data source: %w", err)
	}
	friends, err := ParseCSVData(data)
	if err != nil {
		return nil, fmt.Errorf("error parsing CSV data: %w", err)
	}
	return friends, nil
}

// ReadDataSource reads from a local file or S3 based on the environment.
func ReadDataSource(isLocal bool, bucketName, key string) ([]byte, error) {
	if isLocal {
		file, err := os.Open("../../data/friends.csv")
		if err != nil {
			return nil, fmt.Errorf("error opening CSV file: %w", err)
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("error reading file contents: %w", err)
		}
		return content, nil
	} else {
		cfg, err := config.LoadDefaultConfig(context.TODO())
		if err != nil {
			return nil, fmt.Errorf("unable to load SDK config, %w", err)
		}
		client := s3.NewFromConfig(cfg)
		input := &s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		}
		result, err := client.GetObject(context.TODO(), input)
		if err != nil {
			return nil, fmt.Errorf("failed to get object from S3, %w", err)
		}
		defer result.Body.Close()
		content, err := io.ReadAll(result.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading S3 object contents: %w", err)
		}
		return content, nil
	}
}

// ParseCSVData parses CSV data into a slice of Friend structs.
func ParseCSVData(data []byte) ([]Friend, error) {
	reader := csv.NewReader(bytes.NewBuffer(data))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("error reading CSV: %w", err)
	}
	var friends []Friend
	for i, record := range records {
		if i == 0 { // Skip header row
			continue
		}
		birthday, err := time.ParseInLocation("2006-01-02", record[1], time.Local)
		if err != nil {
			return nil, fmt.Errorf("error parsing birthday for row %d: %w", i+1, err)
		}
		friends = append(friends, Friend{
			Name:        record[0],
			Birthday:    birthday,
			PhoneNumber: record[2],
			Interests:   record[3],
			Prompt:      record[4],
		})
	}
	return friends, nil
}
