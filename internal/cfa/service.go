package cfa

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Service struct {
	c      *http.Client
	cookie *Cookie
}

func NewService(c *http.Client) (*Service, error) {
	if c == nil {
		return nil, fmt.Errorf("client cannot be nil")
	}
	return &Service{c: c}, nil
}

func (s *Service) SetCookie(cookie Cookie) {
	s.cookie = &cookie
}

func (s *Service) Login(username, password string) (*Cookie, error) {
	values := make(url.Values)
	values.Add("username", username)
	values.Add("password", password)

	req, err := http.NewRequest(http.MethodPost, loginEndpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.c.Do(req)
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

	var cookie Cookie
	cookie.CSRFToken = extractCookie(cookies[0])
	cookie.SessionID = extractCookie(cookies[1])
	s.cookie = &cookie

	return &cookie, nil
}

func (s *Service) PollRSVP(ctx context.Context, sched Schedule) (RSVPStatus, error) {
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
			until := time.Until(*sched.Start)
			if until >= MinimumRSVPTime {
				pollTime := calculatePollTime(until)
				fmt.Printf("still not in rsvp window, sleeping for %s time, time until class: %s, remaining: %s\n", pollTime, until, until-MinimumRSVPTime)
				time.Sleep(pollTime)
				continue
			}

			fmt.Println("polling done we are now in rsvp window, time to register")
			status, err := s.RSVP(sched)
			if err != nil {
				return 0, fmt.Errorf("unable to rsvp: %w", err)
			}
			switch status {
			case RSVPED, WAITLISTED:
				return status, nil
			case UNREGISTERED, UNREGISTERED_WAITLIST:
				registerAttempts++
				if registerAttempts >= registerRetries {
					return 0, fmt.Errorf("unable to register after %d attempts", registerAttempts)
				}
				fmt.Printf("failed to register, retrying shortly, attempts: %d\n", registerAttempts)
				continue
			default:
				registerAttempts++
				if registerAttempts >= registerRetries {
					return 0, fmt.Errorf("unable to register after %d attempts", registerAttempts)
				}
				fmt.Printf("failed to register, retrying shortly, attempts: %d\n", registerAttempts)
				continue
			}
		}
	}
}

func (s *Service) RSVP(schedule Schedule) (RSVPStatus, error) {
	endpoint := baseEndpoint + schedule.URL + registerPath + "/"
	fmt.Printf("submitting rsvp request to: %s\n", endpoint)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Add("Cookie", csrfTokenCookieName+"="+s.cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+s.cookie.SessionID)

	resp, err := s.c.Do(req)
	if err != nil {
		return 0, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusFound {
		return 0, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}
	fmt.Println("submitted rsvp request successfully, checking rsvp..")

	// make sure we rsvped for the class
	status, err := s.CheckRSVP(schedule)
	if err != nil {
		return 0, fmt.Errorf("unable to check rsvp: %w", err)
	}

	fmt.Printf("RSVP status code: %d: %s\n", status, status.String())

	return status, nil
}

func (s *Service) CheckRSVP(sched Schedule) (RSVPStatus, error) {
	endpoint := baseEndpoint + sched.URL
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.Header.Add("Cookie", csrfTokenCookieName+"="+s.cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+s.cookie.SessionID)

	resp, err := s.c.Do(req)
	if err != nil {
		return 0, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	// make sure we rsvped for the class
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("unable to read response body: %w", err)
	}
	bodyStr := string(body)
	fmt.Printf("Class page body: %s\n", string(body))
	resp.Body.Close()

	if strings.Contains(bodyStr, unregisteredMessage) {
		return UNREGISTERED, nil
	} else if strings.Contains(bodyStr, unregisteredWaitlistMessage) {
		return UNREGISTERED_WAITLIST, nil
	} else if strings.Contains(bodyStr, rsvpedMessage) {
		return RSVPED, nil
	} else if strings.Contains(bodyStr, waitlistMessage) {
		return WAITLISTED, nil
	}

	return -1, nil
}

func (s *Service) GetSchedule(params ScheduleParams) ([]Schedule, error) {
	values := make(url.Values)
	values.Add(classQueryName, params.Name)
	values.Add(startDateQueryName, params.StartDate)
	values.Add(endDateQueryName, params.EndDate)

	req, err := http.NewRequest(http.MethodGet, scheduleEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to generate new request: %w", err)
	}
	req.URL.RawQuery = values.Encode()
	req.Header.Add("Cookie", csrfTokenCookieName+"="+s.cookie.CSRFToken)
	req.Header.Add("Cookie", sessionIDCookieName+"="+s.cookie.SessionID)

	resp, err := s.c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to complete request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var schedules []Schedule
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

func calculatePollTime(untilClass time.Duration) time.Duration {
	remaining := untilClass - MinimumRSVPTime
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

func timePtr(t time.Time) *time.Time { return &t }
