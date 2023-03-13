package cfa

import (
	"time"
)

type RSVPStatus int

func (s RSVPStatus) String() string {
	switch s {
	case UNREGISTERED:
		return "unregistered"
	case UNREGISTERED_WAITLIST:
		return "unregistered waitlist"
	case RSVPED:
		return "rsvped"
	case WAITLISTED:
		return "waitlisted"
	default:
		return "unknown"
	}
}

const (
	UNREGISTERED RSVPStatus = iota
	UNREGISTERED_WAITLIST
	RSVPED
	WAITLISTED
)

type ScheduleParams struct {
	Name      string
	StartDate string
	EndDate   string
}

type Schedule struct {
	ID      int        `json:"id"`
	Coaches string     `json:"coaches"`
	Title   string     `json:"title"`
	Start   *time.Time `json:"start"`
	End     *time.Time `json:"end"`
	URL     string     `json:"url"`
}

type ScheduleRequest struct {
	ClassName string
	StartTime *time.Time
}
