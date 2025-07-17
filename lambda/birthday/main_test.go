package main

import (
	"fmt"
	"github.com/twilio/twilio-go"
	"os"
	"testing"
	"time"

	"bday-wisher/utils"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

func TestIsBirthday(t *testing.T) {
	today := time.Now()
	nonBirthday := today.AddDate(0, 0, 1) // Tomorrow

	if !isBirthday(today) {
		t.Errorf("Expected isBirthday to return true for today's date")
	}

	if isBirthday(nonBirthday) {
		t.Errorf("Expected isBirthday to return false for a non-birthday date")
	}
}

func TestGenerateMessage(t *testing.T) {
	godotenv.Load("../../.env") // Load .env file from root directory

	client := openai.NewClient(os.Getenv(utils.OPENAI_API_KEY))

	friend := utils.Friend{
		Name:        "Pena",
		PhoneNumber: "+1234567890",
		Birthday:    time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC),
		Interests:   "reading, hiking, cooking",
	}

	message, err := generateMessage(client, friend)
	if err != nil {
		t.Errorf("Error generating message: %v", err)
	}
	if len(message) == 0 {
		t.Errorf("Generated message is empty")
	}
	fmt.Println(message) // Print the generated message
}

func TestSendSmsMessage(t *testing.T) {
	godotenv.Load("../../.env") // Load .env file from root directory

	twilioClient := twilio.NewRestClient()

	// Set up a test phone number and message
	toNumber := "+358401234567" // Replace it with your own number
	message := "Test message"

	// Send the SMS message
	err := sendSmsMessage(twilioClient, toNumber, message)
	if err != nil {
		t.Errorf("error sending SMS message: %v", err)
	}
}

func TestHandleRequest(t *testing.T) {
	godotenv.Load("../../.env") // Load .env file from root directory

	// Send the SMS message
	err := handleRequest()
	if err != nil {
		t.Errorf("error sending SMS message: %v", err)
	}
}
