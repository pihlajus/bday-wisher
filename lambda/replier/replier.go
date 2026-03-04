package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"bday-wisher/utils"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/sashabaranov/go-openai"
	"github.com/twilio/twilio-go"
	twilioClient "github.com/twilio/twilio-go/client"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"
)

type IncomingMessage struct {
	From string `json:"From"`
	Body string `json:"Body"`
}

func sendReply(tc *twilio.RestClient, to, message string) error {
	fromNumber := utils.GetSecret("SSM_TWILIO_PHONE_NUMBER")

	fmt.Printf("Sending response: %s\n", message)
	params := &twilioApi.CreateMessageParams{
		From: &fromNumber,
		To:   &to,
		Body: &message,
	}

	_, err := tc.Api.CreateMessage(params)
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
	// Validate Twilio request signature
	authToken := utils.GetSecret("SSM_TWILIO_AUTH_TOKEN")
	twilioSignature := request.Headers["X-Twilio-Signature"]
	if twilioSignature == "" {
		twilioSignature = request.Headers["x-twilio-signature"]
	}
	webhookURL := os.Getenv("TWILIO_WEBHOOK_URL")

	validator := twilioClient.NewRequestValidator(authToken)

	// Parse body params for signature validation
	bodyValues, err := url.ParseQuery(request.Body)
	if err != nil {
		return events.APIGatewayProxyResponse{StatusCode: http.StatusBadRequest, Body: "Error parsing request body"}, nil
	}

	params := make(map[string]string)
	for key, values := range bodyValues {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	if !validator.Validate(webhookURL, params, twilioSignature) {
		fmt.Println("Invalid Twilio signature - rejecting request")
		return events.APIGatewayProxyResponse{StatusCode: http.StatusForbidden, Body: "Invalid signature"}, nil
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
	openaiClient := openai.NewClient(utils.GetSecret("SSM_OPENAI_API_KEY"))

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
	tc := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: utils.GetSecret("SSM_TWILIO_ACCOUNT_SID"),
		Password: authToken,
	})

	// Send the reply
	err = sendReply(tc, incomingMessage.From, replyMessage)
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
