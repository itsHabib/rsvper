package cfa

import (
	"time"
)

const (
	loginEndpoint      = "https://crossfit-austin.triib.com/accounts/login/"
	baseEndpoint       = "https://crossfit-austin.triib.com"
	registerPath       = "register"
	scheduleEndpoint   = "https://crossfit-austin.triib.com/schedule/json-feed/"
	InHouseSessions    = "In House Sessions"
	classQueryName     = "name"
	startDateQueryName = "start"
	endDateQueryName   = "end"

	MinimumRSVPTime = time.Hour * 120
	registerRetries = 10

	csrfTokenCookieName = "csrftoken"
	sessionIDCookieName = "sessionid"

	rsvpedMessage               = "You are currently RSVP'd for this class"
	waitlistMessage             = "You are currently on the wait list for this class"
	unregisteredMessage         = "RSVP'ing for this class is still available"
	unregisteredWaitlistMessage = "This class is currently full, but you can sign up to be on the wait list"

	pollOverFiveMinutes    = time.Minute
	pollUnderTwoMinutes    = 30 * time.Second
	pollUnderOneMinute     = 10 * time.Second
	pollUnderThirtySeconds = 5 * time.Second
	pollUnderTenSeconds    = 1 * time.Second
	pollUnderFiveSeconds   = 500 * time.Millisecond
	pollUnderOneSecond     = 250 * time.Millisecond
)

type Cookie struct {
	CSRFToken string
	SessionID string
}
