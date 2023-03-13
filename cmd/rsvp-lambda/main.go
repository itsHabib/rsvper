package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/twilio/twilio-go"
	twilioApi "github.com/twilio/twilio-go/rest/api/v2010"

	"github.com/itsHabib/rsvper/internal/cfa"
	"github.com/itsHabib/rsvper/internal/scheduler"
)

func HandleLambdaEvent(ctx context.Context, event scheduler.TaskRequest) (string, error) {
	fmt.Printf("received event: %+v\n", event)
	c := &http.Client{
		Timeout: 10 * time.Second,
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	s, err := cfa.NewService(c)
	if err != nil {
		return "", fmt.Errorf("unable to create cfa service: %w", err)
	}
	s.SetCookie(event.CFACookie)

	twilioClient := twilio.NewRestClient()
	params := &twilioApi.CreateMessageParams{}
	params.SetTo("+18186247532")
	params.SetFrom("+15075017519")

	status, err := s.PollRSVP(ctx, event.Schedule)
	if err != nil {
		params.SetBody(fmt.Sprintf("unable to poll rsvp: %v", err))
		if _, smsErr := twilioClient.Api.CreateMessage(params); smsErr != nil {
			fmt.Printf("unable to send sms: %s", err)
		}
		return "", fmt.Errorf("unable to poll rsvp: %w", err)
	}

	fmt.Printf("rsvp status: %s\n", status.String())
	params.SetBody("successfully submitted rsvp request, with rsvp status: " + status.String())
	if _, smsErr := twilioClient.Api.CreateMessage(params); smsErr != nil {
		fmt.Printf("unable to send sms: %s", err)
	}

	return status.String(), nil
}

func main() {
	lambda.Start(HandleLambdaEvent)
}
