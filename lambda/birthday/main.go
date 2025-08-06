package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"bday-wisher/utils"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sashabaranov/go-openai"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

func generateMessage(client *openai.Client, friend utils.Friend) (string, error) {
	prompt := fmt.Sprintf(
		"Write a warm, personal birthday message for my friend %s. "+
			"Try to include references to their interests, first ones are the most important ones: %s. "+
			"Write a message that is funny and teasing, but not mean, keep it under 300 characters. "+
			"Don't mention that this is AI generated.",
		friend.Name,
		friend.Interests,
	)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			MaxTokens: 1000,
		},
	)

	if err != nil {
		return "", fmt.Errorf("error generating message: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

func sendSmsMessage(twilioClient *twilio.RestClient, to, message string) error {
	fromNumber := os.Getenv("TWILIO_PHONE_NUMBER")

	params := &twilioApi.CreateMessageParams{
		From: &fromNumber,
		To:   &to,
		Body: &message,
	}

	_, err := twilioClient.Api.CreateMessage(params)
	if err != nil {
		return fmt.Errorf("error sending SMS message: %w", err)
	}

	return nil
}

func isBirthday(date time.Time) bool {
	now := time.Now().Local()
	return now.Month() == date.Month() && now.Day() == date.Day()
}

func handleRequest() error {
	// Initialize clients
	openaiClient := openai.NewClient(os.Getenv(utils.OPENAI_API_KEY))
	twilioClient := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: os.Getenv(utils.TWILIO_ACCOUNT_SID),
		Password: os.Getenv(utils.TWILIO_AUTH_TOKEN),
	})

	// Read friends data
	isLocal := utils.GetIsLocal()

	// Read friends data from appropriate source
	friends, err := utils.ReadFriendsCSV(isLocal, "birthday-wisher", "friends.csv")
	if err != nil {
		return fmt.Errorf("error reading friends data: %w", err)
	}

	// Check for birthdays and send messages
	for _, friend := range friends {
		birthday := friend.Birthday
		if isBirthday(birthday) {
			// Generate personalized message
			message, err := generateMessage(openaiClient, friend)
			if err != nil {
				return fmt.Errorf("error generating message for %s: %w", friend.Name, err)
			}

			// Send message (without "whatsapp:" prefix)
			err = sendSmsMessage(twilioClient, friend.PhoneNumber, message)
			if err != nil {
				return fmt.Errorf("error sending message to %s: %w", friend.Name, err)
			}
		}
	}

	return nil
}

func main() {
	lambda.Start(handleRequest)
}
