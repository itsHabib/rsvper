package scheduler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/service/scheduler"
	"github.com/itsHabib/rsvper/internal/cfa"
)

const (
	chicagoTZ        = "America/Chicago"
	rsvperLambdaARN  = "arn:aws:lambda:us-east-2:273568070039:function:RSVPer"
	schedulerRoleARN = "arn:aws:iam::273568070039:role/service-role/Amazon_EventBridge_Scheduler_LAMBDA_9d397e263b"
)

type TaskRequest struct {
	Schedule  cfa.Schedule `json:"schedule"`
	CFACookie cfa.Cookie   `json:"cfaCookie"`
}

type Service struct {
	sess *session.Session
}

func NewService(sess *session.Session) (*Service, error) {
	if sess == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	return &Service{sess: sess}, nil
}

func (s *Service) ProcessRequests(cookie cfa.Cookie, schedules []cfa.Schedule, requests []cfa.ScheduleRequest) error {
	// sort schedules and requests by time

	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].Start.Before(*schedules[j].Start)
	})

	for i := range requests {
		fmt.Printf("request: %s, %s\n", requests[i].ClassName, requests[i].StartTime.Format(time.RFC3339))
		for j := range schedules {
			//	fmt.Printf("class schedules: %s, %s, %s\n", schedules[j].Title, schedules[j].Start, schedules[j].End)
			if strings.Contains(schedules[j].Title, requests[i].ClassName) &&
				equalTimes(*schedules[j].Start, *requests[i].StartTime) {
				fmt.Printf("Found class: %s, %s\n", strings.Replace(schedules[j].Title, "\n", " ", 1), schedules[j].Start)
				timeUntilClass := time.Until(*requests[i].StartTime)
				fmt.Printf("time until class: %s\n", timeUntilClass)

				var start time.Time
				if timeUntilClass < cfa.MinimumRSVPTime {
					start = time.Now().Add(5 * time.Minute)
				} else {
					start = schedules[j].Start.Add(-1 * (cfa.MinimumRSVPTime + 5*time.Minute))
				}

				// create scheduled event for class
				fmt.Printf("creating scheduled event for class at %s\n", start)
				req := TaskRequest{
					Schedule:  schedules[j],
					CFACookie: cookie,
				}
				arn, err := s.createScheduledEvent(req, start)
				if err != nil {
					return fmt.Errorf("unable to create scheduled event: %w", err)
				}
				fmt.Printf("created scheduled event, arn: %s\n", arn)
			}
		}
	}

	return nil
}

func (s *Service) createScheduledEvent(req TaskRequest, start time.Time) (string, error) {
	input, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("unable to marshal task request: %w", err)
	}
	client := scheduler.New(s.sess)

	event := scheduler.CreateScheduleInput{
		Description:                aws.String(formScheduledEventDescription(req.Schedule)),
		Name:                       aws.String(formScheduledEventName(start)),
		ScheduleExpression:         aws.String(formScheduleExpression(start)),
		ScheduleExpressionTimezone: aws.String(chicagoTZ),
		FlexibleTimeWindow: &scheduler.FlexibleTimeWindow{
			Mode: aws.String(scheduler.FlexibleTimeWindowModeOff),
		},
		Target: &scheduler.Target{
			Arn:     aws.String(rsvperLambdaARN),
			RoleArn: aws.String(schedulerRoleARN),
			Input:   aws.String(string(input)),
			RetryPolicy: &scheduler.RetryPolicy{
				MaximumRetryAttempts: aws.Int64(0),
			},
		},
	}

	resp, err := client.CreateSchedule(&event)
	if err != nil {
		return "", fmt.Errorf("unable to create scheduled event: %w", err)
	}

	return *resp.ScheduleArn, nil
}

func formScheduleExpression(start time.Time) string {
	return fmt.Sprintf(
		"at(%d-%02d-%02dT%02d:%02d:00)",
		start.Year(),
		start.Month(),
		start.Day(),
		start.Hour(),
		start.Minute(),
	)
}

func formScheduledEventDescription(schedule cfa.Schedule) string {
	return fmt.Sprintf("Scheduled trigger for class %s at %s", strings.Replace(schedule.Title, "\n", " ", 1), schedule.Start)
}
func formScheduledEventName(start time.Time) string {
	return fmt.Sprintf(
		"Schedule.%s.%02d-%02dT%02d.%02d",
		start.Weekday().String(),
		start.Month(),
		start.Day(),
		start.Hour(),
		start.Minute(),
	)
}

func equalTimes(t1, t2 time.Time) bool {
	return t1.Year() == t2.Year() && t1.Month() == t2.Month() && t1.Day() == t2.Day() && t1.Hour() == t2.Hour() && t1.Minute() == t2.Minute()
}
