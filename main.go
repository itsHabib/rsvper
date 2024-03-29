package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/itsHabib/rsvper/internal/cfa"
	scheduler2 "github.com/itsHabib/rsvper/internal/scheduler"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/scheduler"
)

const (
	loginEndpoint               = "https://crossfit-austin.triib.com/accounts/login/"
	scheduleEndpoint            = "https://crossfit-austin.triib.com/schedule/json-feed/"
	baseEndpoint                = "https://crossfit-austin.triib.com"
	registerPath                = "register"
	UnregisterPath              = "unregister"
	credsFile                   = "creds.txt"
	inHouseSessionsQueryName    = "In House Sessions"
	startDateQueryName          = "start"
	endDateQueryName            = "end"
	csrfTokenCookieName         = "csrftoken"
	sessionIDCookieName         = "sessionid"
	minimumRSVPTime             = time.Minute * 30
	timeLayout                  = "2006-01-02T15:04"
	rsvpedMessage               = "You are currently RSVP'd for this class"
	waitlistMessage             = "You are currently on the wait list for this class"
	unregisteredMessage         = "RSVP'ing for this class is still available"
	unregisteredWaitlistMessage = "This class is currently full, but you can sign up to be on the wait list"
	registerRetries             = 10
	awsRegion                   = "us-east-2"
	chicagoTZ                   = "America/Chicago"
)

var (
	username string
	password string
)

func main() {
	//if err := run(); err != nil {
	//	log.Fatalf("failed to run: %s", err)
	//}

	mockRun()
}

func mockRun() {
	var mockRequests = []cfa.ScheduleRequest{
		{
			ClassName: "CrossFit Small Group Session",
			StartTime: timePtr(time.Date(2023, 3, 12, 23, 10, 0, 0, time.Local)),
		},
	}
	var mockSchedule = []cfa.Schedule{
		{
			ID:      15289564,
			Coaches: "773522",
			Title:   "CrossFit Small Group Session\nSomeone",
			Start:   timePtr(time.Date(2023, 3, 12, 23, 10, 0, 0, time.Local)),
			End:     timePtr(time.Date(2023, 3, 17, 21, 30, 0, 0, time.Local)),
			URL:     "/schedule/15289564/",
		},
	}
	attemptRegister(&http.Client{}, cfa.Cookie{}, mockSchedule, mockRequests)
}

func run() error {
	if err := readCreds(); err != nil {
		log.Fatalf("failed to read creds: %s", err)
	}

	c := &http.Client{
		Timeout: time.Second * 5,
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	var requests = []cfa.ScheduleRequest{
		{
			ClassName: "Range & Resilience",
			StartTime: timePtr(time.Date(2023, 3, 18, 8, 00, 0, 0, time.Local)),
		},
		{
			ClassName: "CrossFit Small Group Session",
			StartTime: timePtr(time.Date(2023, 3, 17, 16, 30, 0, 0, time.Local)),
		},
		{
			ClassName: "CrossFit Small Group Session",
			StartTime: timePtr(time.Date(2023, 3, 17, 12, 00, 0, 0, time.Local)),
		},
		{
			ClassName: "CrossFit Small Group Session",
			StartTime: timePtr(time.Date(2023, 3, 17, 7, 30, 0, 0, time.Local)),
		},
	}

	cfaCookie, err := login(c)
	if err != nil {
		log.Fatalf("failed to login: %s", err)
	}
	//	fmt.Printf("CFA COOKIE: %+v\n", cfaCookie)

	scheduleParams := cfa.ScheduleParams{
		Name:      inHouseSessionsQueryName,
		StartDate: "2023-03-13",
		EndDate:   "2023-03-17",
	}
	schedules, err := getSchedule(c, *cfaCookie, scheduleParams)
	if err != nil {
		log.Fatalf("failed to get schedule: %s", err)
	}
	fmt.Printf("SCHEDULE: %+v\n", len(schedules))

	attemptRegister(c, *cfaCookie, schedules, requests)

	return nil
}

func attemptRegister(c *http.Client, cookie cfa.Cookie, schedules []cfa.Schedule, requests []cfa.ScheduleRequest) {
	// sort schedules and requests by time
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].StartTime.Before(*requests[j].StartTime)
	})
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].Start.Before(*schedules[j].Start)
	})
	sess, err := getAWSSession()
	if err != nil {
		log.Fatalf("failed to get aws session: %s", err)
	}

	for i := range requests {
		fmt.Printf("request: %s, %s\n", requests[i].ClassName, requests[i].StartTime.Format(time.RFC3339))
		for j := range schedules {
			//	fmt.Printf("class schedules: %s, %s, %s\n", schedules[j].Title, schedules[j].Start, schedules[j].End)
			if strings.Contains(schedules[j].Title, requests[i].ClassName) &&
				equalTimes(*schedules[j].Start, *requests[i].StartTime) {
				fmt.Printf("Found class: %s, %s, %s\n", schedules[j].Title, schedules[j].Start, schedules[j].End)
				// if we're already in the 5 day window, try to rsvp right away
				timeUntilClass := time.Until(*requests[i].StartTime)
				fmt.Printf("time until class: %s\n", timeUntilClass)
				if timeUntilClass < minimumRSVPTime {
					fmt.Println("CLASS IN RSVP WINDOW, RSVPING NOW")
					//status, err := rsvp(c, cookie, schedules[j])
					//if err != nil {
					//	log.Fatalf("failed to rsvp: %s", err)
					//}
					//// send text message w/ status
					//fmt.Printf("RSVP STATUS: %s", status)

				} else {
					fmt.Println("CLASS NOT IN RSVP WINDOW, CREATING TASK")
					// create task for later polling to rsvp
					request := scheduler2.TaskRequest{
						Schedule:  schedules[j],
						CFACookie: cookie,
					}
					arn, err := createScheduledEvents(sess, request)
					if err != nil {
						log.Fatalf("failed to create scheduled events: %s", err)
					}
					fmt.Printf("ARN OF SCHEDULED EVENT: %s", arn)
					//status, err := pollRSVP(context.Background(), c, request)
					//if err != nil {
					//	log.Fatalf("failed to rsvp: %s", err)
					//}
					//// send text message w/ status
					//fmt.Printf("RSVP STATUS: %s", status)
				}
			}
		}
	}
}

func rsvp(c *http.Client, cookie cfa.Cookie, schedule cfa.Schedule) (cfa.RSVPStatus, error) {
	endpoint := baseEndpoint + schedule.URL + "register/"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Add("Cookie", csrfTokenCookieName+"="+cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+cookie.SessionID)

	resp, err := c.Do(req)
	if err != nil {
		return 0, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusFound {
		return 0, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	// make sure we rsvped for the class
	status, err := checkRSVP(c, cookie, schedule)
	if err != nil {
		return 0, fmt.Errorf("unable to check rsvp: %w", err)
	}

	fmt.Printf("RSVP status: %d\n", status)

	return status, nil
}

const (
	pollOverFiveMinutes    = time.Minute
	pollUnderTwoMinutes    = 30 * time.Second
	pollUnderOneMinute     = 10 * time.Second
	pollUnderThirtySeconds = 5 * time.Second
	pollUnderTenSeconds    = 1 * time.Second
	pollUnderFiveSeconds   = 500 * time.Millisecond
	pollUnderOneSecond     = 250 * time.Millisecond
)

func pollRSVP(ctx context.Context, c *http.Client, rsvpRequest scheduler2.TaskRequest) (cfa.RSVPStatus, error) {
	// cap this polling at 20 minute to reduce costs/memory etc
	const pollTimeout = 10 * time.Minute
	timeout := time.NewTimer(pollTimeout)
	var registerAttempts int
	for {
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("polling cancelled: %w", ctx.Err())
		case <-timeout.C:
			return 0, fmt.Errorf("polling timed out after %s", pollTimeout)
		default:
			// calculate poll time
			until := time.Until(*rsvpRequest.Schedule.Start)
			if until >= minimumRSVPTime {
				pollTime := calculatePollTime(until)
				fmt.Printf("still not in rsvp window, sleeping for %s time, time until class: %s, remaining: %s\n", pollTime, until, until-minimumRSVPTime)
				time.Sleep(pollTime)
				continue
			}
			// pretend rsvp
			fmt.Println("polling done we are now in rsvp window, time to register")
			var status cfa.RSVPStatus
			//status, err := rsvp(c, rsvpRequest.CFACookie, rsvpRequest.Schedule)
			//if err != nil {
			//	return 0, fmt.Errorf("unable to rsvp: %w", err)
			//}
			switch status {
			case cfa.RSVPED, cfa.WAITLISTED:
				return status, nil
			case cfa.UNREGISTERED, cfa.UNREGISTERED_WAITLIST:
				registerAttempts++
				if registerAttempts >= registerRetries {
					return 0, fmt.Errorf("unable to register after %d attempts", registerAttempts)
				}
				fmt.Println("failed to register, retrying shortly..")
				continue
			default:
				return 0, fmt.Errorf("unexpected rsvp status: %d", status)
			}
		}
	}
}

func calculatePollTime(untilClass time.Duration) time.Duration {
	remaining := untilClass - minimumRSVPTime
	if remaining >= time.Minute*2 {
		return pollOverFiveMinutes
	} else if remaining < time.Minute*2 && remaining >= time.Minute*1 {
		return pollUnderTwoMinutes
	} else if remaining < time.Minute*1 && remaining >= time.Second*30 {
		return pollUnderOneMinute
	} else if remaining < time.Second*30 && remaining >= time.Second*10 {
		return pollUnderThirtySeconds
	} else if remaining < time.Second*10 && remaining >= time.Second*5 {
		return pollUnderTenSeconds
	} else if remaining < time.Second*5 && remaining >= time.Second*1 {
		return pollUnderFiveSeconds
	} else if remaining < time.Second*1 {
		return pollUnderOneSecond
	}

	return pollUnderFiveSeconds
}

func checkRSVP(c *http.Client, cookie cfa.Cookie, sched cfa.Schedule) (cfa.RSVPStatus, error) {
	endpoint := baseEndpoint + sched.URL
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Add("Cookie", csrfTokenCookieName+"="+cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+cookie.SessionID)

	resp, err := c.Do(req)
	if err != nil {
		return 0, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}
	// make sure we rsvped for the class
	fmt.Printf("RSVP RESPONSE CODE: %d\n", resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("unable to read response body: %w", err)
	}
	resp.Body.Close()
	//	fmt.Printf("RSVP RESPONSE BODY: %s\n", string(body))

	bodyStr := string(body)
	if strings.Contains(bodyStr, unregisteredMessage) {
		return cfa.UNREGISTERED, nil
	} else if strings.Contains(bodyStr, unregisteredWaitlistMessage) {
		return cfa.UNREGISTERED_WAITLIST, nil
	} else if strings.Contains(bodyStr, rsvpedMessage) {
		return cfa.RSVPED, nil
	} else if strings.Contains(bodyStr, waitlistMessage) {
		return cfa.WAITLISTED, nil

	}

	return -1, nil
}

func login(c *http.Client) (*cfa.Cookie, error) {
	values := make(url.Values)
	values.Add("username", username)
	values.Add("password", password)

	req, err := http.NewRequest(http.MethodPost, loginEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to complete request: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	cookies := resp.Header.Values("Set-Cookie")
	if len(cookies) == 0 {
		return nil, fmt.Errorf("unable to get cookie from login request")
	}

	var cookie cfa.Cookie
	cookie.CSRFToken = extractCookie(cookies[0])
	cookie.SessionID = extractCookie(cookies[1])

	return &cookie, nil
}

func getSchedule(c *http.Client, cookie cfa.Cookie, params cfa.ScheduleParams) ([]cfa.Schedule, error) {
	values := make(url.Values)
	values.Add(inHouseSessionsQueryName, params.Name)
	values.Add(startDateQueryName, params.StartDate)
	values.Add(endDateQueryName, params.EndDate)

	req, err := http.NewRequest(http.MethodGet, scheduleEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.URL.RawQuery = values.Encode()
	req.Header.Add("Cookie", csrfTokenCookieName+"="+cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+cookie.SessionID)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var schedules []cfa.Schedule
	if err := json.NewDecoder(resp.Body).Decode(&schedules); err != nil {
		return nil, fmt.Errorf("unable to decode schedule response: %w", err)
	}
	resp.Body.Close()

	// clear out seconds from start time
	for i := range schedules {
		schedules[i].Start = timePtr(time.Date(schedules[i].Start.Year(), schedules[i].Start.Month(), schedules[i].Start.Day(), schedules[i].Start.Hour(), schedules[i].Start.Minute(), 0, 0, time.Local))
	}

	return schedules, nil
}

func equalTimes(t1, t2 time.Time) bool {
	return t1.Year() == t2.Year() && t1.Month() == t2.Month() && t1.Day() == t2.Day() && t1.Hour() == t2.Hour() && t1.Minute() == t2.Minute()
}

func extractCookie(cookieStr string) string {
	var (
		start int
		end   int
	)

	for i := range cookieStr {
		if cookieStr[i] == '=' {
			start = i + 1
		}
		if cookieStr[i] == ';' {
			end = i
			break
		}
	}

	return cookieStr[start:end]
}

func timePtr(t time.Time) *time.Time { return &t }

func readCreds() error {
	f, err := os.Open(credsFile)
	if err != nil {
		return fmt.Errorf("unable to open creds file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		username = scanner.Text()
	}
	if scanner.Scan() {
		password = scanner.Text()
	}

	if username == "" || password == "" {
		return fmt.Errorf("unable to read username or password")
	}

	return nil
}

func createScheduledEvents(sess *session.Session, task scheduler2.TaskRequest) (string, error) {
	input, err := json.Marshal(task)
	if err != nil {
		return "", fmt.Errorf("unable to marshal task request: %w", err)
	}

	s := scheduler.New(sess)

	// create scheduled event for class, time will be set to 5 minutes before
	// the RSVP window opens. assuming the class is outside the RSVP window
	// TODO: make sure start time of class is outside of the RSVP window
	// yyyy-mm-ddThh:mm:ss
	start := task.Schedule.Start.Add(-1 * (minimumRSVPTime + 5*time.Minute))
	event := scheduler.CreateScheduleInput{
		Description:                aws.String(formScheduledEventDescription(task.Schedule)),
		Name:                       aws.String(formScheduledEventName(start)),
		ScheduleExpression:         aws.String(formScheduleExpression(start)),
		ScheduleExpressionTimezone: aws.String(chicagoTZ),
		FlexibleTimeWindow: &scheduler.FlexibleTimeWindow{
			Mode: aws.String(scheduler.FlexibleTimeWindowModeOff),
		},
		Target: &scheduler.Target{
			Arn:     aws.String("arn:aws:lambda:us-east-2:273568070039:function:Tester"),
			RoleArn: aws.String("arn:aws:iam::273568070039:role/service-role/Amazon_EventBridge_Scheduler_LAMBDA_9d397e263b"),
			Input:   aws.String(string(input)),
			RetryPolicy: &scheduler.RetryPolicy{
				MaximumRetryAttempts: aws.Int64(0),
			},
		},
	}

	resp, err := s.CreateSchedule(&event)
	if err != nil {
		return "", fmt.Errorf("unable to create scheduled event: %w", err)
	}

	return *resp.ScheduleArn, nil
}

func formScheduleExpression(start time.Time) string {
	// we want to set the start to five minutes before the rsvp window begins.
	// to do this we get the start time of the class, subtract the rsvp window
	// and 7 additional minutes. AWS expects the expression to look like:
	// yyyy-mm-ddThh:mm:ss

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
	return fmt.Sprintf("Scheduled trigger for class %s at %s", schedule.Title, schedule.Start)
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

func getAWSSession() (*session.Session, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create new session: %w", err)
	}

	return sess, nil
}
