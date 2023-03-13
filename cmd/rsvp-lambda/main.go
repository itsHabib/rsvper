package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/lambda"

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

	status, err := s.PollRSVP(ctx, event.Schedule)
	if err != nil {
		return "", fmt.Errorf("unable to poll rsvp: %w", err)
	}
	fmt.Printf("rsvp status: %s\n", status.String())

	return status.String(), nil
}

func main() {
	lambda.Start(HandleLambdaEvent)
}
