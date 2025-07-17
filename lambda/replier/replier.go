package main

import (
	"bday-wisher/utils"
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sashabaranov/go-openai"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type IncomingMessage struct {
	From string `json:"From"`
	Body string `json:"Body"`
}

func sendReply(twilioClient *twilio.RestClient, to, message string) error {
	fromNumber := os.Getenv(utils.TWILIO_PHONE_NUMBER)

	fmt.Printf("Sending response: %s\n", message)
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

func generateReply(client *openai.Client, incomingMessage IncomingMessage, customPrompt string) (string, error) {
	prompt := customPrompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Respond to this message: \"%s\" in a friendly and casual way, as if you are a bit busy but still happy to reply. Don't make it sound too official, and don't reveal that you are AI.",
			incomingMessage.Body,
		)
	}

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
			MaxTokens: 3000,
		},
	)

	if err != nil {
		return "", fmt.Errorf("error generating reply: %w", err)
	}

	fmt.Printf("Response from openAI: %s", resp)

	return resp.Choices[0].Message.Content, nil
}

func handleReplyRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse the body to extract parameters
	bodyValues, err := url.ParseQuery(request.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "Error parsing request body"}, nil
	}

	// Extract 'From' and 'Body' from the parsed body
	from := bodyValues.Get("From")
	body := bodyValues.Get("Body")

	// Validate incoming message
	if from == "" || body == "" {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "Missing 'From' or 'Body' parameter"}, nil
	}

	incomingMessage := IncomingMessage{
		From: from,
		Body: body,
	}

	// Read friend data
	isLocal := utils.GetIsLocal()
	friends, err := utils.ReadFriendsCSV(isLocal, "birthday-wisher", "friends.csv")
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError, Body: "Error reading friends data"}, nil
	}

	// Find custom prompt for the incoming number
	var customPrompt string
	fmt.Printf("Looking for custom prompt for number: %s\n", incomingMessage.From)
	for _, friend := range friends {
		if friend.PhoneNumber == incomingMessage.From {
			customPrompt = friend.Prompt
			fmt.Printf("Found custom prompt: %s\n", customPrompt)
			break
		}
	}
	if customPrompt == "" {
		fmt.Printf("No custom prompt found for number: %s\n", incomingMessage.From)
	}

	// Initialize OpenAI client
	openaiClient := openai.NewClient(os.Getenv(utils.OPENAI_API_KEY))

	// Generate a reply using OpenAI
	replyMessage, err := generateReply(openaiClient, incomingMessage, customPrompt)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, fmt.Errorf("error generating reply for message from %s: %w", incomingMessage.From, err)
	}

	// Remove leading and trailing quotes if they exist
	replyMessage = strings.Trim(replyMessage, "\"")

	// Log the reply message
	fmt.Printf("Sending reply to: %s, message: %s\n", incomingMessage.From, replyMessage)

	// Initialize Twilio client
	twilioClient := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: os.Getenv(utils.TWILIO_ACCOUNT_SID),
		Password: os.Getenv(utils.TWILIO_AUTH_TOKEN),
	})

	// Send the reply
	err = sendReply(twilioClient, incomingMessage.From, replyMessage)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError}, fmt.Errorf("error sending reply to %s: %w", incomingMessage.From, err)
	}

	// Optionally log the context deadline
	deadline, ok := ctx.Deadline()
	if ok {
		fmt.Printf("Deadline for this request is: %v\n", deadline)
	}

	return events.APIGatewayProxyResponse{StatusCode: http.StatusOK}, nil
}

func main() {
	lambda.Start(handleReplyRequest)
}
